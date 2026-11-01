package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
)

type imageFingerprint struct {
	SHA    string `json:"sha256"`
	PHash  uint64 `json:"phash"`
	DHash  uint64 `json:"dhash"`
	Mean   [3]int `json:"mean"`
	Detail bool   `json:"detail"`
}
type ImageMatch struct {
	AttachmentID      string `json:"attachment_id"`
	QueryAttachmentID string `json:"query_attachment_id"`
	Method            string `json:"method"`
	Distance          int    `json:"distance"`
}

func fingerprint(path string) (imageFingerprint, error) {
	var result imageFingerprint
	raw, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(raw)
	result.SHA = hex.EncodeToString(sum[:])
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return result, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 50000000 {
		return result, fmt.Errorf("invalid image size")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return result, err
	}
	source, _, err := image.Decode(f)
	if err != nil {
		return result, err
	}
	small := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(small, small.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(small, small.Bounds(), source, source.Bounds(), draw.Over, nil)
	var values [32][32]float64
	var total, totalSq float64
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			p := small.RGBAAt(x, y)
			v := 0.299*float64(p.R) + 0.587*float64(p.G) + 0.114*float64(p.B)
			values[y][x] = v
			total += v
			totalSq += v * v
			result.Mean[0] += int(p.R)
			result.Mean[1] += int(p.G)
			result.Mean[2] += int(p.B)
		}
	}
	for i := range result.Mean {
		result.Mean[i] /= 1024
	}
	result.Detail = totalSq/1024-math.Pow(total/1024, 2) > 30
	var coeff [64]float64
	for v := 0; v < 8; v++ {
		for u := 0; u < 8; u++ {
			var sum float64
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					sum += values[y][x] * math.Cos(float64(2*x+1)*float64(u)*math.Pi/64) * math.Cos(float64(2*y+1)*float64(v)*math.Pi/64)
				}
			}
			coeff[v*8+u] = sum
		}
	}
	ordered := append([]float64{}, coeff[1:]...)
	sort.Float64s(ordered)
	median := ordered[len(ordered)/2]
	for i := 1; i < 64; i++ {
		if coeff[i] > median {
			result.PHash |= uint64(1) << i
		}
	}
	diff := image.NewRGBA(image.Rect(0, 0, 9, 8))
	draw.Draw(diff, diff.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(diff, diff.Bounds(), source, source.Bounds(), draw.Over, nil)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			l := color.GrayModel.Convert(diff.At(x, y)).(color.Gray).Y
			r := color.GrayModel.Convert(diff.At(x+1, y)).(color.Gray).Y
			if l > r {
				result.DHash |= uint64(1) << uint(y*8+x)
			}
		}
	}
	return result, nil
}
func fingerprintMatch(a, b imageFingerprint) (string, int, bool) {
	if a.SHA == b.SHA {
		return "exact", 0, true
	}
	p, d := bits.OnesCount64(a.PHash^b.PHash), bits.OnesCount64(a.DHash^b.DHash)
	colorDist := 0
	for i := 0; i < 3; i++ {
		colorDist += int(math.Abs(float64(a.Mean[i] - b.Mean[i])))
	}
	return "perceptual", p, a.Detail && b.Detail && p <= 6 && d <= 10 && colorDist <= 75
}
func (a *App) ensureFingerprint(ctx context.Context, item Attachment) (imageFingerprint, error) {
	var f imageFingerprint
	var raw string
	err := a.DB.QueryRowContext(ctx, "SELECT fingerprint FROM image_fingerprints WHERE attachment_id=?", item.ID).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &f)
		return f, err
	}
	if err != sql.ErrNoRows {
		return f, err
	}
	f, err = fingerprint(filepath.Join(a.Config.DataDir, "uploads", item.StoredName))
	if err != nil {
		return f, err
	}
	_, err = a.DB.ExecContext(ctx, "INSERT IGNORE INTO image_fingerprints(attachment_id,fingerprint) VALUES (?,?)", item.ID, jsonText(f))
	return f, err
}
func (a *App) imageMatches(ctx context.Context, queryIDs []string, userID int64, category string, limit int) ([]Note, map[string][]ImageMatch, []string) {
	found := map[string][]ImageMatch{}
	warnings := []string{}
	queries := map[string]imageFingerprint{}
	for _, id := range unique(queryIDs) {
		item := ownedAttachment(a.DB, id, userID, false)
		if !strings.HasPrefix(item.MIME, "image/") {
			continue
		}
		f, err := a.ensureFingerprint(ctx, item)
		if err != nil {
			warnings = append(warnings, item.Filename+" 无法建立图片指纹")
			continue
		}
		queries[id] = f
	}
	if len(queries) == 0 {
		return []Note{}, found, warnings
	}
	rows, err := a.DB.QueryContext(ctx, "SELECT "+attachmentColumns+" FROM attachments WHERE user_id=? AND note_id IS NOT NULL AND mime LIKE 'image/%' ORDER BY id", userID)
	check(err)
	candidates := []Attachment{}
	for rows.Next() {
		item, err := scanAttachment(rows)
		check(err)
		candidates = append(candidates, item)
	}
	check(rows.Err())
	rows.Close()
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			warnings = append(warnings, "图片指纹检索超时，请缩小资料范围后重试")
			break
		}
		f, err := a.ensureFingerprint(ctx, candidate)
		if err != nil {
			warnings = append(warnings, candidate.Filename+" 原件缺失或无法建立图片指纹")
			continue
		}
		for id, q := range queries {
			method, distance, ok := fingerprintMatch(q, f)
			if ok {
				found[*candidate.NoteID] = append(found[*candidate.NoteID], ImageMatch{candidate.ID, id, method, distance})
			}
		}
	}
	type hit struct {
		id       string
		distance int
	}
	hits := []hit{}
	for id, matches := range found {
		score := 100
		for _, m := range matches {
			score = min(score, m.Distance)
		}
		hits = append(hits, hit{id, score})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].distance != hits[j].distance {
			return hits[i].distance < hits[j].distance
		}
		return hits[i].id < hits[j].id
	})
	notes := []Note{}
	for _, h := range hits {
		n := ownedNote(a.DB, h.id, userID, false)
		if !matchesFilter(n, filterFromContext(ctx)) || (category != "" && n.Category != category) {
			continue
		}
		notes = append(notes, n)
		if len(notes) == limit {
			break
		}
	}
	return notes, found, unique(warnings)
}

// Image candidates remain independent of cloud re-ranking: a valid image match is
// useful even when no text could be recognized. Both modalities are de-duplicated.
func (a *App) retrieveInput(ctx context.Context, query string, ids []string, userID int64, category string, limit int) ([]Note, bool, []string) {
	if len(ids) > 5 {
		fail(422, "搜索和对话最多添加 5 份材料")
	}
	images, matches, warnings := a.imageMatches(ctx, ids, userID, category, limit)
	text, enhanced, extra := a.retrieveRanked(ctx, query, userID, category, limit)
	warnings = append(warnings, extra...)
	notes := []Note{}
	seen := map[string]bool{}
	for _, n := range append(images, text...) {
		if seen[n.ID] || len(notes) == limit {
			continue
		}
		seen[n.ID] = true
		n.SearchMatch = searchEvidence(n, query)
		n.SearchMatch.ImageMatches = matches[n.ID]
		if len(matches[n.ID]) > 0 {
			n.SearchMatch.Fields = append(n.SearchMatch.Fields, "image")
			for _, m := range matches[n.ID] {
				n.SearchMatch.AttachmentIDs = append(n.SearchMatch.AttachmentIDs, m.AttachmentID)
			}
		}
		notes = append(notes, n)
	}
	return notes, enhanced, warnings
}
