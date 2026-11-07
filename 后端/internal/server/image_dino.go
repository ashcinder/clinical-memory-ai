package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Serial inference bounds CPU and memory when several local users search.
var dinoSlot = make(chan struct{}, 1)

type dinoImage struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	SHA  string `json:"sha"`
}

type dinoRanking struct {
	IDs      []string          `json:"ids"`
	QueryIDs map[string]string `json:"query_ids"`
	Skipped  int               `json:"skipped"`
}

func (a *App) dinoRanks(ctx context.Context, queries, gallery []dinoImage) (dinoRanking, error) {
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	select {
	case dinoSlot <- struct{}{}:
		defer func() { <-dinoSlot }()
	case <-ctx.Done():
		return dinoRanking{}, ctx.Err()
	}

	python := a.Config.ImageDINOPython
	if python == "" {
		python = "python3"
	}
	worker := filepath.Join(a.Config.Root, "后端", "tools", "open_image_eval", "retrieve.py")
	cache := filepath.Join(a.Config.DataDir, "image-embeddings")
	cmd := exec.CommandContext(ctx, python, worker, "--model", a.Config.ImageDINOModel, "--cache", cache)
	cmd.Env = append(os.Environ(), "HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "OPENBLAS_NUM_THREADS=1", "VECLIB_MAXIMUM_THREADS=1")
	input, err := json.Marshal(map[string][]dinoImage{"queries": queries, "gallery": gallery})
	if err != nil {
		return dinoRanking{}, err
	}
	cmd.Stdin = bytes.NewReader(input)
	raw, err := cmd.Output()
	if err != nil {
		return dinoRanking{}, err
	}
	var result dinoRanking
	if err := json.Unmarshal(raw, &result); err != nil {
		return dinoRanking{}, err
	}
	if len(result.IDs) == 0 || result.Skipped < 0 || len(result.IDs)+result.Skipped != len(gallery) {
		return dinoRanking{}, fmt.Errorf("incomplete DINOv2 ranking")
	}
	ids := make(map[string]bool, len(gallery))
	for _, item := range gallery {
		ids[item.ID] = true
	}
	queryIDs := make(map[string]bool, len(queries))
	for _, query := range queries {
		queryIDs[query.ID] = true
	}
	for _, id := range result.IDs {
		if !ids[id] || !queryIDs[result.QueryIDs[id]] {
			return dinoRanking{}, fmt.Errorf("invalid DINOv2 candidate")
		}
		delete(ids, id)
	}
	return result, nil
}

// Preserve strong fingerprints, then reserve space for both visual and lexical
// recall. The returned order only controls which images get verified.
func fuseImageCandidates(items []Attachment, ranks []string, strong map[string]bool, limit int) []Attachment {
	byID := make(map[string]Attachment, len(items))
	chosen := make(map[string]bool, limit)
	out := make([]Attachment, 0, limit)
	add := func(item Attachment) {
		if len(out) < limit && !chosen[item.ID] {
			chosen[item.ID] = true
			out = append(out, item)
		}
	}
	for _, item := range items {
		byID[item.ID] = item
		if strong[item.ID] {
			add(item)
		}
	}
	visualEnd := len(out) + (3*(limit-len(out))+3)/4
	for _, id := range ranks {
		if len(out) >= visualEnd {
			break
		}
		if item, ok := byID[id]; ok {
			add(item)
		}
	}
	for _, item := range items {
		add(item)
		if len(out) == limit {
			break
		}
	}
	return out
}

// Offline results are ranked suggestions. Exact/perceptual evidence stays ahead
// of these candidates, which do not receive the cloud verifier's verdict.
func (a *App) offlineDINO(ctx context.Context, queries []Attachment, userID int64, category string, limit int, found map[string][]ImageMatch) ([]Note, []string, bool) {
	candidates, warnings := a.imageCandidateGallery(ctx, userID, category)
	if len(candidates) == 0 {
		return nil, warnings, false
	}
	path := func(item Attachment) string {
		return filepath.Join(a.Config.DataDir, "uploads", item.StoredName)
	}
	dinoQueries := make([]dinoImage, 0, len(queries))
	for _, item := range queries {
		dinoQueries = append(dinoQueries, dinoImage{ID: item.ID, Path: path(item)})
	}
	gallery := make([]dinoImage, 0, len(candidates))
	byID := make(map[string]Attachment, len(candidates))
	for _, item := range candidates {
		f, err := a.ensureFingerprint(ctx, item)
		if err == nil {
			gallery = append(gallery, dinoImage{ID: item.ID, Path: path(item), SHA: f.SHA})
			byID[item.ID] = item
		}
	}
	ranking, err := a.dinoRanks(ctx, dinoQueries, gallery)
	if err != nil {
		return nil, append(warnings, "本地 DINOv2 检索未完成，已使用图片指纹和文字匹配"), false
	}
	if unavailable := len(candidates) - len(gallery) + ranking.Skipped; unavailable > 0 {
		warnings = append(warnings, fmt.Sprintf("%d 张图片未能参与本地 DINOv2 检索", unavailable))
	}
	var notes []Note
	for _, id := range ranking.IDs {
		item := byID[id]
		noteID := *item.NoteID
		if len(found[noteID]) > 0 {
			continue
		}
		found[noteID] = []ImageMatch{{AttachmentID: id, QueryAttachmentID: ranking.QueryIDs[id], Method: "dino", Reason: "本地 DINOv2 外观相似候选，请核对原图与标记位置"}}
		notes = append(notes, ownedNote(a.DB, noteID, userID, false))
		if len(notes) >= min(5, limit) {
			break
		}
	}
	return notes, warnings, true
}
