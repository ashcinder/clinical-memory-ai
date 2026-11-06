package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestPasswordAndTokens(t *testing.T) {
	stored := hashPassword("test-pass-123")
	if !verifyPassword("test-pass-123", stored) || verifyPassword("wrong", stored) {
		t.Fatal("password verification")
	}
	if len(newID()) != 32 {
		t.Fatal("id size")
	}
	items := tokens("咳嗽 followup")
	if !strings.Contains(strings.Join(items, " "), "咳嗽") {
		t.Fatal(items)
	}
}
func TestSchemaMirror(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "数据库数据", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != schema {
		t.Fatal("数据库数据/schema.sql must match embedded migration")
	}
}
func TestBodyLimitAndOrigin(t *testing.T) {
	app := &App{}
	handler := Headers(app.recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { var in any; decode(r, &in); respond(w, 200, in) })))
	r := httptest.NewRequest("POST", "http://localhost/api/test", strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://foreign.example")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "http://localhost/api/test", strings.NewReader(`{}`))
	r.ContentLength = maxUpload + (2 << 20)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "http://localhost/api/test", strings.NewReader(`{} {}`))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 422 {
		t.Fatal(w.Code)
	}
}

func TestFrontendCacheRevalidation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"index.html", "styles.css", "app.js"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("test asset"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler := Headers(http.FileServer(http.Dir(root)))
	for _, path := range []string{"/", "/index.html", "/styles.css?v=current", "/app.js?v=current"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		expected := http.StatusOK
		if path == "/index.html" {
			expected = http.StatusMovedPermanently
		}
		if w.Code != expected || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: status=%d cache=%q", path, w.Code, w.Header().Get("Cache-Control"))
		}
	}
	w := httptest.NewRecorder()
	Headers(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, httptest.NewRequest("GET", "/api/settings", nil))
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API must not be cached")
	}
}
func TestCompletionAdapter(t *testing.T) {
	var payload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing key")
		}
		if r.URL.Path != "/chat/completions" {
			t.Error(r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"测试回复"}}]}`))
	}))
	defer upstream.Close()
	app := &App{Config: Config{DeepSeekKey: "test-key", DeepSeekURL: upstream.URL, DeepSeekModel: "test-model"}, Client: newAIClient()}
	content, err := app.completion(context.Background(), []map[string]any{{"role": "user", "content": "测试"}}, false)
	if err != nil || content != "测试回复" || payload["model"] != "test-model" {
		t.Fatalf("%s %v %v", content, err, payload)
	}
	rejected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"do-not-leak-this"}`))
	}))
	defer rejected.Close()
	app.Config.DeepSeekURL = rejected.URL
	_, err = app.completion(context.Background(), nil, false)
	if err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatal(err)
	}
}

func TestExpandQuery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "<think>内部内容</think>```json\n{\"keywords\":[\"合谷\",\"LI4\"]}\n```"}}}})
	}))
	defer upstream.Close()
	a := &App{Config: Config{DeepSeekKey: "key", DeepSeekURL: upstream.URL}, Client: newAIClient()}
	query, enhanced, err := a.expandQuery(context.Background(), "手背资料")
	if err != nil || !enhanced || !strings.Contains(query, "合谷") || !strings.Contains(query, "手背资料") {
		t.Fatal(query, enhanced, err)
	}
	a.Config.DeepSeekURL = "http://127.0.0.1:1"
	query, enhanced, err = a.expandQuery(context.Background(), "手背资料")
	if err == nil || enhanced || query != "手背资料" {
		t.Fatal(query, enhanced, err)
	}
}

func TestProviderConfiguration(t *testing.T) {
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("AI_PROVIDER", "minimax")
	t.Setenv("MINIMAX_API_KEY", "minimax-test")
	t.Setenv("MINIMAX_MODEL", "text-model")
	t.Setenv("VISION_PROVIDER", "glm")
	t.Setenv("GLM_API_KEY", "glm-test")
	t.Setenv("VISION_MODEL", "vision-model")
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.AIProvider != "minimax" || c.DeepSeekKey != "minimax-test" || c.DeepSeekModel != "text-model" || c.VisionProvider != "glm" || c.VisionKey != "glm-test" || c.VisionModel != "vision-model" {
		t.Fatal("provider selection failed")
	}
	t.Setenv("AI_PROVIDER", "bad-provider")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
func TestExtractAndASR(t *testing.T) {
	dir := t.TempDir()
	app := &App{Client: newAIClient()}
	txt := filepath.Join(dir, "text.txt")
	_ = os.WriteFile(txt, []byte("中文测试"), 0600)
	s, status, err := app.extract(context.Background(), txt, "text/plain")
	if err != nil || s != "中文测试" || status != "ready" {
		t.Fatal(s, status, err)
	}
	imagePath := filepath.Join(dir, "test.png")
	f, _ := os.Create(imagePath)
	_ = png.Encode(f, image.NewRGBA(image.Rect(0, 0, 10, 10)))
	_ = f.Close()
	_, status, err = app.extract(context.Background(), imagePath, "image/png")
	if status != "extraction_failed" || err == nil {
		t.Fatal("缺少本地 OCR 应明确提示而非伪造文字", status, err)
	}
	asr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Error(r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		if r.FormValue("model") != "test-asr" {
			t.Error("missing model")
		}
		_, _ = w.Write([]byte(`{"text":"咳嗽随访"}`))
	}))
	defer asr.Close()
	app.Config.ASRURL = asr.URL
	app.Config.ASRKey = "key"
	app.Config.ASRModel = "test-asr"
	audio := filepath.Join(dir, "test.wav")
	_ = os.WriteFile(audio, []byte("RIFFtest"), 0600)
	s, status, err = app.extract(context.Background(), audio, "audio/wav")
	if err != nil || s != "咳嗽随访" || status != "ready" {
		t.Fatal(s, status, err)
	}
}

func TestAIImageAndKeywordSearch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid request")
		}
		content := `{"keywords":["合谷","LI4"]}`
		if request.Model == "vision-test" {
			if !strings.Contains(string(request.Messages[0].Content), "data:image/jpeg;base64,") {
				t.Error("image missing")
			}
			content = `{"kind":"穴位图","body_parts":["手部"],"laterality":"无法确认","visible_text":"合谷 LI4","identifiers":["LI4"],"features":["标记"],"uncertainties":[],"quality":"清晰"}`
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	}))
	defer upstream.Close()
	a := integrationApp(t)
	a.Config.DeepSeekKey, a.Config.DeepSeekURL, a.Config.DeepSeekModel = "test", upstream.URL, "text-test"
	a.Config.VisionKey, a.Config.VisionURL, a.Config.VisionModel = "test", upstream.URL, "vision-test"
	c := &testAPI{t: t, handler: a.Handler()}
	created := c.request("POST", "/api/auth/setup", map[string]any{"username": "doctor", "password": "test-pass-123"}, 201)
	c.token = getString(created, "token")
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 10, 10)))
	history := c.upload("history.png", picture.Bytes())
	if history.Status != "ready" || !strings.Contains(history.Text, "合谷") {
		t.Fatal(history)
	}
	note := c.request("POST", "/api/notes", map[string]any{"title": "穴位记录", "content": "合谷观察记录", "attachment_ids": []string{history.ID}}, 201)
	query := c.upload("query.png", picture.Bytes())
	result := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != note["id"] || !strings.Contains(result["engine"].(string), "AI") {
		t.Fatal(result)
	}
	if len(items[0].(map[string]any)["attachments"].([]any)) != 1 {
		t.Fatal("missing original image")
	}
	// Exact matches do not depend on text enhancement or another cloud call.
	a.Config.DeepSeekURL = "http://127.0.0.1:1"
	result = c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
	if len(result["items"].([]any)) != 1 {
		t.Fatal(result)
	}
}
func integrationApp(t *testing.T) *App {
	t.Helper()
	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("设置 MYSQL_TEST_DSN 才运行真实 MySQL 集成测试；测试只创建和删除 linji_test_ 前缀的独立库")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	name := "linji_test_" + randomHex(6)
	if _, err = db.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	cfg.DBName = name
	cfg.ParseTime = true
	appDB, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	app := &App{DB: appDB, Config: Config{Root: root, WebDir: filepath.Join(root, "前端Web"), DataDir: t.TempDir(), DeepSeekModel: "deepseek-flash"}, Client: newAIClient()}
	if err = app.Migrate(); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(app.Config.DataDir, "uploads"), 0700)
	t.Cleanup(func() { appDB.Close(); _, _ = db.Exec("DROP DATABASE `" + name + "`"); db.Close() })
	return app
}

type testAPI struct {
	t       *testing.T
	handler http.Handler
	token   string
}

func (c *testAPI) request(method, path string, body any, expected int) map[string]any {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(jsonText(body))
	}
	r := httptest.NewRequest(method, "http://localhost"+path, reader)
	r.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if w.Code != expected {
		c.t.Fatalf("%s %s expected %d got %d %s", method, path, expected, w.Code, w.Body.String())
	}
	result := map[string]any{}
	if expected != 204 && strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil { // List endpoints are decoded separately.
			return map[string]any{"raw": w.Body.String()}
		}
	}
	return result
}
func (c *testAPI) upload(name string, data []byte) Attachment {
	c.t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", name)
	_, _ = part.Write(data)
	_ = writer.Close()
	r := httptest.NewRequest("POST", "http://localhost/api/attachments", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+c.token)
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, r)
	if w.Code != 201 {
		c.t.Fatal(w.Code, w.Body.String())
	}
	item := Attachment{}
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		c.t.Fatal(err)
	}
	return item
}
func getString(m map[string]any, key string) string { v, _ := m[key].(string); return v }
func TestMySQLFullWorkflow(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	c.request("GET", "/api/notes", nil, 401)
	created := c.request("POST", "/api/auth/setup", map[string]any{"username": "doctor", "password": "test-pass-123", "display_name": "测试医生"}, 201)
	c.token = getString(created, "token")
	c.request("POST", "/api/auth/setup", map[string]any{"username": "other", "password": "test-pass-123"}, 409)
	c.request("POST", "/api/auth/login", map[string]any{"username": "doctor", "password": "wrong-pass"}, 401)
	attachment := c.upload("notes.txt", []byte("独特材料线索 蓝莓观察"))
	if attachment.Status != "ready" {
		t.Fatal(attachment)
	}
	note := c.request("POST", "/api/notes", map[string]any{"title": "虚构测试", "steps": []string{"核对", "回顾"}, "attachment_ids": []string{attachment.ID}}, 201)
	id := getString(note, "id")
	found := c.request("POST", "/api/search", map[string]any{"query": "蓝莓观察"}, 200)
	items := found["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != id {
		t.Fatal(found)
	}
	c.request("PUT", "/api/attachments/"+attachment.ID+"/text", map[string]string{"text": "新线索 橙子"}, 200)
	found = c.request("POST", "/api/search", map[string]any{"query": "蓝莓"}, 200)
	if len(found["items"].([]any)) != 0 {
		t.Fatal("old term not removed")
	}
	c.request("POST", "/api/notes", map[string]any{"title": "不应重复绑定", "attachment_ids": []string{attachment.ID}}, 409)
	c.request("PUT", "/api/notes/"+id, map[string]any{"title": "修改标题", "steps": []string{"步骤甲"}, "attachment_ids": []string{attachment.ID}}, 200)
	reply := c.request("POST", "/api/chat", map[string]any{"message": "橙子", "mode": "knowledge"}, 200)
	if reply["provider"] != "local" || !strings.Contains(reply["content"].(string), "新线索") {
		t.Fatal(reply)
	}
	conversation := getString(reply, "conversation_id")
	continued := c.request("POST", "/api/chat", map[string]any{"message": "继续回顾", "mode": "knowledge", "conversation_id": conversation}, 200)
	if len(continued["sources"].([]any)) != 1 {
		t.Fatal(continued)
	}
	history := c.request("GET", "/api/conversations/"+conversation, nil, 200)
	if len(history["messages"].([]any)) != 4 {
		t.Fatal(history)
	}
	c.request("POST", "/api/chat", map[string]any{"message": "你好", "mode": "general", "conversation_id": conversation}, 409)
	c.request("GET", "/api/export", nil, 200)
	var owner int64
	_ = a.DB.QueryRow("SELECT id FROM users WHERE username='doctor'").Scan(&owner)
	// A second account is inserted only in this isolated test database to test all ownership paths.
	_, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "second", hashPassword("second-pass"), "另一个账户", now())
	if err != nil {
		t.Fatal(err)
	}
	other := c.request("POST", "/api/auth/login", map[string]any{"username": "second", "password": "second-pass"}, 200)
	ownerToken := c.token
	c.token = getString(other, "token")
	c.request("GET", "/api/notes/"+id, nil, 200)
	c.request("GET", "/api/attachments/"+attachment.ID+"/file", nil, 200)
	c.request("GET", "/api/conversations/"+conversation, nil, 404)
	c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{attachment.ID}}, 200)
	found = c.request("POST", "/api/search", map[string]any{"query": "橙子"}, 200)
	if len(found["items"].([]any)) != 1 {
		t.Fatal("shared retrieval failed")
	}
	c.request("POST", "/api/auth/logout", nil, 200)
	c.request("GET", "/api/auth/me", nil, 401)
	c.token = ownerToken
	c.request("DELETE", "/api/notes/"+id, nil, 200)
	item := ownedAttachment(a.DB, attachment.ID, owner, false)
	if item.NoteID != nil {
		t.Fatal("original should remain detached")
	}
	c.request("DELETE", "/api/attachments/"+attachment.ID, nil, 200)
	c.request("GET", "/api/attachments/"+attachment.ID+"/file", nil, 404)
	c.request("POST", "/api/notes", map[string]any{"title": " ", "occurred_on": "bad"}, 422)
	// Upstream extraction failure must not discard the original.
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer broken.Close()
	a.Config.VisionKey = "test-key"
	a.Config.VisionURL = broken.URL
	var picture bytes.Buffer
	_ = png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 10, 10)))
	failed := c.upload("image.png", picture.Bytes())
	if failed.Status != "extraction_failed" || failed.ExtractionError == nil {
		t.Fatal(failed)
	}
}
func TestMySQLConcurrentSetup(t *testing.T) {
	a := integrationApp(t)
	h := a.Handler()
	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			r := httptest.NewRequest("POST", "http://localhost/api/auth/setup", strings.NewReader(jsonText(map[string]any{"username": fmt.Sprintf("doctor%d", i), "password": "test-pass-123"})))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			statuses <- w.Code
		}(i)
	}
	group.Wait()
	close(statuses)
	counts := map[int]int{}
	for s := range statuses {
		counts[s]++
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatal(counts)
	}
}
func TestNoKeysLeakInSettings(t *testing.T) {
	a := integrationApp(t)
	a.Config.DeepSeekKey = "SECRET-MUST-NOT-LEAK"
	c := &testAPI{t: t, handler: a.Handler()}
	created := c.request("POST", "/api/auth/setup", map[string]any{"username": "doctor", "password": "test-pass-123"}, 201)
	c.token = getString(created, "token")
	settings := c.request("GET", "/api/settings", nil, 200)
	if strings.Contains(jsonText(settings), "SECRET-MUST-NOT-LEAK") {
		t.Fatal("key leaked")
	}
}
