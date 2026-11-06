package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestVisualResponseValidation(t *testing.T) {
	valid := visualDecision{AttachmentID: "a", QueryID: "q", Verdict: "match", Relevance: 90, Reason: "原图可见同一结构"}
	for _, test := range []struct {
		name          string
		decision      visualDecision
		valid, accept bool
	}{
		{"accept", valid, true, true},
		{"conflict", func() visualDecision { d := valid; d.Conflict = true; return d }(), true, false},
		{"uncertain", func() visualDecision { d := valid; d.Verdict = "uncertain"; return d }(), true, false},
		{"weak", func() visualDecision { d := valid; d.Relevance = 79; return d }(), true, false},
		{"unknown_candidate", func() visualDecision { d := valid; d.AttachmentID = "invented"; return d }(), false, false},
		{"unknown_query", func() visualDecision { d := valid; d.QueryID = "invented"; return d }(), false, false},
		{"missing_evidence", func() visualDecision { d := valid; d.Reason = ""; return d }(), false, false},
		{"bad_score", func() visualDecision { d := valid; d.Relevance = 101; return d }(), false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateVisualDecisions(jsonText(map[string]any{"results": []visualDecision{test.decision}}), map[string]bool{"a": true}, map[string]bool{"q": true})
			if (err == nil) != test.valid {
				t.Fatalf("validation: %v", err)
			}
			if test.valid && acceptVisualDecision(test.decision) != test.accept {
				t.Fatal("incorrect acceptance")
			}
		})
	}
	for _, raw := range []string{`{"results":[]}`, `{"results":null}`, `garbage`, jsonText(map[string]any{"results": []visualDecision{valid, valid}})} {
		if _, err := validateVisualDecisions(raw, map[string]bool{"a": true}, map[string]bool{"q": true}); err == nil {
			t.Fatal("accepted missing/duplicate/malformed decision")
		}
	}
}

func TestImagePayloadBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.png")
	f, _ := os.Create(path)
	_ = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 3200, 1800)))
	f.Close()
	block, hash, err := imageBlock(path)
	if err != nil {
		t.Fatal(err)
	}
	url := block["image_url"].(map[string]string)["url"]
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/jpeg;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != 2000 || cfg.Height != 1125 || len(hash) != 64 {
		t.Fatal(cfg, err)
	}
	original, _ := os.ReadFile(path)
	cfg, _, _ = image.DecodeConfig(bytes.NewReader(original))
	if cfg.Width != 3200 {
		t.Fatal("source overwritten")
	}
	os.WriteFile(path, []byte("invalid"), 0600)
	if _, _, err := imageBlock(path); err == nil {
		t.Fatal("invalid image accepted")
	}
}

func TestVisualImagePipeline(t *testing.T) {
	a := integrationApp(t)
	var calls atomic.Int32
	var mode atomic.Value
	mode.Store("match")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req struct {
			Messages []struct {
				Content []map[string]any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			return
		}
		parts := req.Messages[0].Content
		response := `{"kind":"解剖图","body_parts":["肺"],"laterality":"双侧","visible_text":"肺","identifiers":[],"features":["双肺轮廓"],"uncertainties":[],"quality":"清晰"}`
		if strings.Contains(parts[0]["text"].(string), "clinical_image_compare_v1") {
			q := ""
			decisions := []visualDecision{}
			imageCount := 0
			for _, p := range parts {
				if p["type"] == "image_url" {
					imageCount++
				}
				label, _ := p["text"].(string)
				if strings.HasPrefix(label, "查询图 ID=") {
					q = strings.TrimPrefix(label, "查询图 ID=")
				}
				if strings.HasPrefix(label, "候选图 ID=") {
					d := visualDecision{AttachmentID: strings.TrimPrefix(label, "候选图 ID="), QueryID: q, Verdict: "match", Relevance: 95, Reason: "原图中具体结构一致"}
					switch mode.Load().(string) {
					case "conflict":
						d.Conflict = true
					case "uncertain":
						d.Verdict = "uncertain"
					case "invented":
						d.AttachmentID = "invented"
					case "failure":
						w.WriteHeader(503)
						return
					}
					decisions = append(decisions, d)
				}
			}
			if imageCount != 1+len(decisions) {
				t.Error("query and candidate original images not supplied")
			}
			response = jsonText(map[string]any{"results": decisions})
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": response}}}})
	}))
	defer upstream.Close()
	a.Config.AIProvider = "deepseek"
	a.Config.VisionProvider = "deepseek"
	a.Config.VisionURL = upstream.URL
	a.Config.VisionModel = "test"
	a.Config.VisionKey = "test"
	c := &testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", map[string]any{"username": "visualtester", "password": "visual-test-123"}, 201), "token")
	var raw bytes.Buffer
	png.Encode(&raw, fingerprintFixture())
	history := c.upload("history.png", raw.Bytes())
	if !strings.Contains(history.Text, "原图可辨文字") {
		t.Fatal("missing structured recognition")
	}
	note := c.request("POST", "/api/notes", map[string]any{"title": "肺部资料", "content": "肺部结构", "attachment_ids": []string{history.ID}}, 201)
	other := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			other.Set(x, y, color.RGBA{240, 30, 10, 255})
		}
	}
	raw.Reset()
	png.Encode(&raw, other)
	query := c.upload("different-view.png", raw.Bytes())
	for _, m := range []string{"match", "conflict", "uncertain", "invented", "failure"} {
		mode.Store(m)
		result := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
		items := result["items"].([]any)
		if m == "match" {
			if len(items) != 1 || items[0].(map[string]any)["id"] != note["id"] {
				t.Fatal(result)
			}
			ev := items[0].(map[string]any)["search_match"].(map[string]any)
			if ev["image_matches"].([]any)[0].(map[string]any)["method"] != "visual" {
				t.Fatal(ev)
			}
		} else if len(items) != 0 {
			t.Fatalf("%s promoted rejected/unknown/unverified match", m)
		}
	}
	// Backfill must reuse cached observations and retain manual corrections.
	c.request("PUT", "/api/attachments/"+history.ID+"/text", map[string]any{"text": "医生手动修订"}, 200)
	before := calls.Load()
	c.request("POST", "/api/image-index", map[string]any{"limit": 2}, 200)
	if calls.Load() != before {
		t.Fatal("cache not reused")
	}
	item := ownedAttachment(a.DB, history.ID, 1, false)
	if item.Text != "医生手动修订" {
		t.Fatal("manual correction overwritten")
	}
	// Changing the model invalidates the structured index.
	a.Config.VisionModel = "test-new"
	c.request("POST", "/api/image-index", map[string]any{"limit": 2}, 200)
	if calls.Load() == before {
		t.Fatal("stale model cache reused")
	}
}

func TestExactImageOutsideRecentWindow(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", map[string]any{"username": "exact_old", "password": "old-image-test-123"}, 201), "token")
	var raw bytes.Buffer
	png.Encode(&raw, fingerprintFixture())
	old := c.upload("old.png", raw.Bytes())
	target := c.request("POST", "/api/notes", map[string]any{"title": "Older original", "attachment_ids": []string{old.ID}}, 201)
	other := c.request("POST", "/api/notes", map[string]any{"title": "Recent unrelated images"}, 201)
	_, err := a.DB.Exec("UPDATE attachments SET created_at='2000-01-01T00:00:00Z' WHERE id=?", old.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Seed valid cached fingerprints, without making 2001 real cloud uploads.
	tx, err := a.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2001; i++ {
		id := fmt.Sprintf("%032d", i)
		_, err = tx.Exec("INSERT INTO attachments(id,user_id,note_id,filename,stored_name,mime,size,extracted_text,status,created_at) VALUES (?,1,?,'unrelated.png','missing.png','image/png',1,'','ready','2026-01-01T00:00:00Z')", id, other["id"])
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec("INSERT INTO image_fingerprints(attachment_id,fingerprint) VALUES (?,?)", id, jsonText(imageFingerprint{Version: 2, SHA: "unrelated"}))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	query := c.upload("same.png", raw.Bytes())
	a.Config.AIProvider = "deepseek"
	a.Config.VisionProvider = "deepseek"
	a.Config.VisionKey = "test"
	a.Config.VisionURL = "http://127.0.0.1:1"
	a.Config.VisionModel = "test"
	result := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != target["id"] {
		t.Fatal("exact match lost outside visual window")
	}
}
