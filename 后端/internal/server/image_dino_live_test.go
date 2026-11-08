package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Both modes share one hard cap. Requests already sent count even if they fail.
type dinoLiveBudget struct {
	mu      sync.Mutex
	client  HTTPDoer
	calls   int
	stopped bool
}

func (b *dinoLiveBudget) Do(r *http.Request) (*http.Response, error) {
	b.mu.Lock()
	if b.calls >= 12 || b.stopped {
		b.mu.Unlock()
		return nil, errors.New("paid benchmark stopped")
	}
	b.calls++
	b.mu.Unlock()
	response, err := b.client.Do(r)
	if response != nil && (response.StatusCode == 402 || response.StatusCode == 429) {
		b.mu.Lock()
		b.stopped = true
		b.mu.Unlock()
	}
	return response, err
}

func TestDINODeepSeekSmallLive(t *testing.T) {
	if os.Getenv("LINJI_RUN_DINO_LIVE") != "1" {
		t.Skip("explicit opt-in for up to 12 paid image comparisons")
	}
	benchmark := os.Getenv("LINJI_DINO_BENCHMARK")
	out := os.Getenv("LINJI_DINO_REPORT_DIR")
	if benchmark == "" || out == "" {
		t.Fatal("set LINJI_DINO_BENCHMARK and LINJI_DINO_REPORT_DIR")
	}
	benchmark, err := filepath.Abs(benchmark)
	if err != nil {
		t.Fatal(err)
	}
	out, err = filepath.Abs(out)
	if err != nil {
		t.Fatal(err)
	}
	if out == benchmark || strings.HasPrefix(out, benchmark+string(filepath.Separator)) {
		t.Fatal("write the live report outside the frozen benchmark")
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	owner := &App{Config: cfg}
	settings, err := owner.loadAISettings(1)
	if err != nil {
		t.Fatal(err)
	}
	runtime := owner.runtimeAIConfig(settings)
	if runtime.CloudSearchDisabled || runtime.VisionProvider != "deepseek" || runtime.VisionKey == "" || runtime.ImageDINOModel == "" {
		t.Fatal("configure DeepSeek vision and IMAGE_DINO_MODEL")
	}
	if os.Getenv("MYSQL_TEST_DSN") == "" {
		t.Fatal("set MYSQL_TEST_DSN for a disposable database")
	}

	type frozenCase struct {
		Name    string   `json:"name"`
		QueryID string   `json:"query_attachment_id"`
		Notes   []string `json:"expected_note_ids"`
		Scope   string   `json:"scope"`
	}
	var previous struct {
		Results []frozenCase `json:"results"`
	}
	raw, err := os.ReadFile(filepath.Join(benchmark, "final-selected-results.json"))
	if err != nil || json.Unmarshal(raw, &previous) != nil {
		t.Fatal("cannot read frozen query results")
	}
	type outcome struct {
		Case           string      `json:"case"`
		Mode           string      `json:"mode"`
		Engine         string      `json:"engine"`
		HasAnswer      bool        `json:"has_answer"`
		Top1           bool        `json:"top1"`
		Rejected       bool        `json:"rejected"`
		TargetCompared bool        `json:"target_compared"`
		Returned       int         `json:"returned"`
		Warnings       []string    `json:"warnings"`
		PromptTokens   int64       `json:"prompt_tokens"`
		CachedTokens   int64       `json:"cached_tokens"`
		OutputTokens   int64       `json:"output_tokens"`
		UpstreamStatus map[int]int `json:"upstream_status"`
		ElapsedMS      int64       `json:"elapsed_ms"`
	}
	var results []outcome
	budget := &dinoLiveBudget{client: newAIClient()}
	defer func() {
		report := map[string]any{"calls": budget.calls, "max_calls": 12, "model": runtime.VisionModel, "candidate_limit_per_mode": 4, "results": results}
		raw, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "results.json"), raw, 0600); err != nil {
			t.Error(err)
		}
	}()
	benchmarkQuotaPreflight(t, runtime, out)
	for _, suite := range []struct {
		state, manifest string
		cases           []string
	}{
		{"large-gallery", "large-gallery", []string{"busi-train-279-crop", "busi-train-086-jpeg", "busi-val-059-absent"}},
		{"main-supplement", "", []string{"58cb47a2a5ac41d0-crop", "4206d5f648b861b0-semantic-jpeg", "c9f30d79272bb54c-absent-crop"}},
	} {
		a := integrationApp(t)
		cfg := runtime
		cfg.Root, cfg.WebDir, cfg.DataDir = a.Config.Root, a.Config.WebDir, a.Config.DataDir
		a.Config = cfg
		a.Config.ImageVisualCandidates = 4
		benchmarkRestoreState(t, a, filepath.Join(benchmark, suite.state))
		c := &testAPI{t: t, handler: a.Handler()}
		password := randomHex(16)
		if _, err := a.DB.Exec("UPDATE users SET password_hash=? WHERE username='scale_benchmark'", hashPassword(password)); err != nil {
			t.Fatal(err)
		}
		c.token = getString(c.request("POST", "/api/auth/login", map[string]any{"username": "scale_benchmark", "password": password}, 200), "token")
		var manifest struct {
			Cases []struct {
				Name, Query string
			}
		}
		raw, err := os.ReadFile(filepath.Join(benchmark, suite.manifest, "manifest.json"))
		if err != nil || json.Unmarshal(raw, &manifest) != nil {
			t.Fatal("cannot read frozen manifest")
		}
		for _, name := range suite.cases {
			caseIndex := slices.IndexFunc(previous.Results, func(i frozenCase) bool { return i.Name == name })
			query := ""
			for _, row := range manifest.Cases {
				if row.Name == name {
					query = row.Query
				}
			}
			if caseIndex < 0 || query == "" {
				t.Fatal("missing frozen case: " + name)
			}
			record := previous.Results[caseIndex]
			var available int
			if err := a.DB.QueryRow("SELECT COUNT(*) FROM attachments WHERE id=?", record.QueryID).Scan(&available); err != nil || available != 1 {
				t.Fatal("missing cached query image: " + name)
			}
			for _, mode := range []string{"baseline", "dino+deepseek"} {
				if budget.stopped {
					t.Fatal("provider rate or quota limit reached")
				}
				a.Config.ImageDINOModel = ""
				if mode == "dino+deepseek" {
					a.Config.ImageDINOModel = runtime.ImageDINOModel
				}
				meter := &imageBenchmarkMeter{client: budget}
				a.Client = meter
				started := time.Now()
				r := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{record.QueryID}, "query": query, "tags": []string{"BENCH-" + record.Scope}, "limit": 20}, 200)
				row := outcome{Case: name, Mode: mode, Engine: getString(r, "engine"), HasAnswer: len(record.Notes) > 0, PromptTokens: meter.prompt, CachedTokens: meter.cached, OutputTokens: meter.output, UpstreamStatus: meter.statuses, ElapsedMS: time.Since(started).Milliseconds()}
				items, _ := r["items"].([]any)
				row.Returned = len(items)
				row.Rejected = len(items) == 0
				if len(items) > 0 {
					row.Top1 = slices.Contains(record.Notes, getString(items[0].(map[string]any), "id"))
				}
				for id := range meter.coverage[record.QueryID] {
					var note string
					if a.DB.QueryRow("SELECT note_id FROM attachments WHERE id=?", id).Scan(&note) == nil && slices.Contains(record.Notes, note) {
						row.TargetCompared = true
					}
				}
				warnings, _ := r["warnings"].([]any)
				for _, warning := range warnings {
					row.Warnings = append(row.Warnings, warning.(string))
				}
				results = append(results, row)
				t.Logf("%s %s target_compared=%v top1=%v returned=%d upstream=%v", name, mode, row.TargetCompared, row.Top1, row.Returned, row.UpstreamStatus)
				if mode == "dino+deepseek" && !strings.Contains(row.Engine, "DINOv2") {
					t.Fatal("DINOv2 not used for the selected gallery")
				}
			}
		}
	}
}
