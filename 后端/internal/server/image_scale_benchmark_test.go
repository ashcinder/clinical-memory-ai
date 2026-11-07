package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type imageBenchmarkMeter struct {
	quotaStopped                         bool
	skippedAfterQuota                    int
	statuses                             map[int]int
	pairs                                int
	coverage                             map[string]map[string]bool
	invalid                              int
	client                               HTTPDoer
	mu                                   sync.Mutex
	calls, comparisons, analyses, errors int
	prompt, cached, output               int64
}

func (m *imageBenchmarkMeter) Do(r *http.Request) (*http.Response, error) {
	m.mu.Lock()
	if m.quotaStopped {
		m.skippedAfterQuota++
		m.mu.Unlock()
		return nil, fmt.Errorf("benchmark stopped after upstream HTTP 402")
	}
	m.mu.Unlock()
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	comparison := bytes.Contains(raw, []byte("clinical_image_compare_v1"))
	var message struct {
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &message)
	queries, candidates := []string{}, []string{}
	for _, message := range message.Messages {
		for _, part := range message.Content {
			if strings.HasPrefix(part.Text, "查询图 ID=") {
				queries = append(queries, strings.TrimPrefix(part.Text, "查询图 ID="))
			}
			if strings.HasPrefix(part.Text, "候选图 ID=") {
				candidates = append(candidates, strings.TrimPrefix(part.Text, "候选图 ID="))
			}
		}
	}
	m.mu.Lock()
	m.pairs += len(queries) * len(candidates)
	if m.coverage == nil {
		m.coverage = map[string]map[string]bool{}
	}
	for _, q := range queries {
		if m.coverage[q] == nil {
			m.coverage[q] = map[string]bool{}
		}
		for _, c := range candidates {
			m.coverage[q][c] = true
		}
	}
	m.mu.Unlock()
	response, err := m.client.Do(r)
	m.mu.Lock()
	m.calls++
	if comparison {
		m.comparisons++
	} else {
		m.analyses++
	}
	if err != nil {
		m.errors++
	}
	m.mu.Unlock()
	if err != nil {
		return response, err
	}
	m.mu.Lock()
	if m.statuses == nil {
		m.statuses = map[int]int{}
	}
	m.statuses[response.StatusCode]++
	if response.StatusCode == 402 {
		m.quotaStopped = true
	}
	m.mu.Unlock()
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	var usage struct {
		Usage struct {
			Prompt int64 `json:"prompt_tokens"`
			Cached int64 `json:"prompt_cache_hit_tokens"`
			Output int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &usage)
	m.mu.Lock()
	m.prompt += usage.Usage.Prompt
	m.cached += usage.Usage.Cached
	m.output += usage.Usage.Output
	if response.StatusCode >= 400 || readErr != nil {
		m.errors++
	}
	m.mu.Unlock()
	return response, readErr
}

// Runs only on explicit opt-in. Uses the existing account's vision configuration,
// an isolated linji_test_* database and temporary files; never seeds real notes.
func TestLargeImageBenchmark(t *testing.T) {
	if os.Getenv("LINJI_RUN_IMAGE_SCALE") != "1" {
		t.Skip("set LINJI_RUN_IMAGE_SCALE=1 for paid large image evaluation")
	}
	dir := os.Getenv("LINJI_IMAGE_SCALE_DIR")
	if dir == "" {
		t.Fatal("LINJI_IMAGE_SCALE_DIR is required")
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
	if runtime.CloudSearchDisabled || runtime.VisionKey == "" {
		t.Fatal("vision model is not configured")
	}
	benchmarkQuotaPreflight(t, runtime, dir)
	a := integrationApp(t)
	runtime.DataDir = a.Config.DataDir
	a.Config = runtime
	meter := &imageBenchmarkMeter{client: newAIClient()}
	a.Client = meter
	restoring := os.Getenv("LINJI_IMAGE_BENCH_RESTORE") == "1"
	if restoring {
		benchmarkRestoreState(t, a, dir)
	}
	c := &testAPI{t: t, handler: a.Handler()}
	password := randomHex(16)
	var created map[string]any
	if restoring {
		if _, e := a.DB.Exec("UPDATE users SET password_hash=? WHERE username='scale_benchmark'", hashPassword(password)); e != nil {
			t.Fatal(e)
		}
		created = c.request("POST", "/api/auth/login", map[string]any{"username": "scale_benchmark", "password": password}, 200)
	} else {
		created = c.request("POST", "/api/auth/setup", map[string]any{"username": "scale_benchmark", "password": password}, 201)
	}
	mux := http.NewServeMux()
	mux.Handle("/", a.Handler())
	token := getString(created, "token")
	mux.HandleFunc("POST /benchmark/fingerprint", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		var in struct {
			IDs   []string `json:"ids"`
			Scope string   `json:"scope"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			w.WriteHeader(400)
			return
		}
		ctx := context.WithValue(r.Context(), noteFilterKey{}, noteFilter{Tags: []string{"BENCH-" + in.Scope}})
		if in.Scope == "" {
			ctx = r.Context()
		}
		notes, _, warnings := a.imageMatches(ctx, in.IDs, 1, "", 20)
		respond(w, 200, map[string]any{"items": notes, "warnings": warnings})
	})
	mux.HandleFunc("GET /benchmark/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		meter.mu.Lock()
		defer meter.mu.Unlock()
		respond(w, 200, map[string]any{"quota_exhausted": meter.quotaStopped, "upstream_statuses": meter.statuses})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	before := time.Now()
	script := filepath.Join(dir, "run_benchmark.py")
	python := filepath.Join(dir, "venv/bin/python")
	if err = benchmarkArchiveRun(dir); err != nil {
		t.Fatal("不能归档上一轮评测结果")
	}
	child := exec.Command(python, script)
	child.Env = append(os.Environ(), "LINJI_BENCH_URL="+srv.URL, "LINJI_BENCH_TOKEN="+getString(created, "token"), "LINJI_BENCH_MODEL="+runtime.VisionModel)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	runErr := child.Run()
	meter.mu.Lock()
	report := map[string]any{"model": runtime.VisionModel, "provider": runtime.VisionProvider, "api_calls": meter.calls, "comparison_calls": meter.comparisons, "analysis_calls": meter.analyses, "api_errors": meter.errors, "upstream_http_statuses": meter.statuses, "stopped_on_quota": meter.quotaStopped, "skipped_after_quota": meter.skippedAfterQuota, "candidate_image_pairs": meter.pairs, "prompt_tokens": meter.prompt, "cache_hit_tokens": meter.cached, "completion_tokens": meter.output, "elapsed_seconds": time.Since(before).Seconds(), "timestamp": now(), "isolated_database": true, "production_records_modified": false}
	coverage, _ := json.MarshalIndent(meter.coverage, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "candidate-coverage.json"), coverage, 0600)
	meter.mu.Unlock()
	data, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(filepath.Join(dir, "api-usage.json"), data, 0600); err != nil {
		t.Error(err)
	}
	// Test result reflects infrastructure execution, not an arbitrary accuracy gate.
	// Accuracy, abstentions, timeouts and every failed example remain in the report.
	benchmarkSaveState(t, a, dir)
	if runErr != nil {
		t.Fatal(strings.TrimSpace(runErr.Error()))
	}
}
