package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

func TestIntranetGatewaySurvivesAccountSettings(t *testing.T) {
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("AI_PROVIDER", "deepseek")
	t.Setenv("VISION_PROVIDER", "glm")
	t.Setenv("DEEPSEEK_API_KEY", "test-key")
	t.Setenv("GLM_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_BASE_URL", "https://chat.intranet.example/v1/")
	t.Setenv("GLM_BASE_URL", "https://glm.intranet.example/v1/")
	t.Setenv("KIMI_BASE_URL", "https://kimi.intranet.example/v1/")
	t.Setenv("VISION_BASE_URL", "https://vision.intranet.example/v1/")
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.DataDir = t.TempDir()
	a := &App{Config: c}
	// This mutation creates the account settings file even without visiting the
	// model-settings screen. It previously reset endpoint routing to public URLs.
	r := httptest.NewRequest("PUT", "/api/search-settings", strings.NewReader(`{"cloud_search_enabled":false}`))
	a.putSearchSettings(httptest.NewRecorder(), r, User{ID: 1})
	r = a.aiSettingsForRequest(httptest.NewRequest("GET", "/api/settings", nil), User{ID: 1})
	got := a.aiConfig(r.Context())
	if got.DeepSeekURL != "https://chat.intranet.example/v1" || got.VisionURL != "https://vision.intranet.example/v1" || !got.CloudSearchDisabled {
		t.Fatal("account mutation changed administrator gateway routing")
	}
	s, err := a.loadAISettings(1)
	if err != nil {
		t.Fatal(err)
	}
	s.ChatProvider, s.VisionProvider = "kimi", "kimi"
	got = a.runtimeAIConfig(s)
	if got.DeepSeekURL != "https://kimi.intranet.example/v1" || got.VisionURL != "https://kimi.intranet.example/v1" {
		t.Fatal("switching providers ignored configured gateway")
	}
	// If vision switches to the chat provider, it should use that provider URL,
	// not an unrelated vision-specific override.
	s.VisionProvider = "deepseek"
	if got = a.runtimeAIConfig(s); got.VisionURL != "https://chat.intranet.example/v1" {
		t.Fatal("vision provider route incorrect")
	}
}

func TestIntranetHTTPSWebAndApp(t *testing.T) {
	a := integrationApp(t)
	a.Config.BasePath, a.Config.CookieSecure = "/clinical-memory", true
	backend := httptest.NewServer(a.Handler())
	defer backend.Close()
	target, _ := url.Parse(backend.URL)
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	base := proxy.URL + a.Config.BasePath
	web := proxy.Client()
	web.Jar, _ = cookiejar.New(nil)
	app := proxy.Client()
	// New independent clients: browser Cookie vs native App Bearer token.
	app = &http.Client{Transport: app.Transport}
	request := func(client *http.Client, method, path, body, token string, expected int) (*http.Response, map[string]any) {
		t.Helper()
		r, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", proxy.URL)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != expected {
			t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, data)
		}
		out := map[string]any{}
		if strings.Contains(response.Header.Get("Content-Type"), "application/json") {
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatal(err)
			}
		}
		return response, out
	}
	for _, path := range []string{"/", "/app.js", "/styles.css"} {
		request(web, "GET", path, "", "", 200)
	}
	credentials := `{"username":"intranet_doctor","password":"test-password-123"}`
	response, _ := request(web, "POST", "/api/auth/setup", credentials, "", 201)
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Path != "/clinical-memory/" {
		t.Fatal("HTTPS session cookie incorrectly configured")
	}
	request(app, "GET", "/api/auth/me", "", "", 401)
	_, logged := request(app, "POST", "/api/auth/login", credentials, "", 200)
	token := getString(logged, "token")
	if token == "" {
		t.Fatal("native client token missing")
	}
	_, before := request(app, "GET", "/api/sync", "", token, 200)
	_, note := request(web, "POST", "/api/notes", `{"title":"内网网页记录","content":"手机可读取"}`, "", 201)
	id := getString(note, "id")
	_, read := request(app, "GET", "/api/notes/"+id, "", token, 200)
	if read["content"] != "手机可读取" {
		t.Fatal("App could not read browser note")
	}
	_, after := request(app, "GET", "/api/sync", "", token, 200)
	if before["version"] == after["version"] {
		t.Fatal("browser changes not signaled to native client")
	}
	update := jsonText(map[string]any{"title": "内网手机修改", "content": "网页可读取", "expected_version": read["version"]})
	request(app, "PUT", "/api/notes/"+id, update, token, 200)
	_, read = request(web, "GET", "/api/notes/"+id, "", "", 200)
	if read["content"] != "网页可读取" {
		t.Fatal("browser could not read App update")
	}
	request(web, "PUT", "/api/notes/"+id, update, "", 409)
}
