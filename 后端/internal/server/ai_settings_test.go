package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestUserAISettings(t *testing.T) {
	a := &App{Config: Config{DataDir: t.TempDir(), AIProvider: "deepseek", VisionProvider: "none", DeepSeekKey: "legacy-key", DeepSeekURL: "https://legacy.example", DeepSeekModel: "legacy-model"}}
	call := func(user int64, method string, body string) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, "/api/ai-config", strings.NewReader(body))
		w := httptest.NewRecorder()
		if method == http.MethodPut {
			a.putAISettings(w, r, User{ID: user})
		} else {
			a.getAISettings(w, r, User{ID: user})
		}
		if w.Code != 200 {
			t.Fatalf("status %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "sensitive-key") || strings.Contains(w.Body.String(), "legacy-key") {
			t.Fatal("secret returned")
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	get := call(10, "GET", "")
	if get["chat_provider"] != "deepseek" {
		t.Fatal(get)
	}
	call(10, "PUT", `{"chat_provider":"glm","vision_provider":"kimi","providers":{"glm":{"api_key":"sensitive-key","chat_model":"glm-custom"},"kimi":{"api_key":"vision-secret","vision_model":"kimi-custom"}},"asr":{"api_key":"audio-secret","base_url":"https://asr.example/v1","model":"speech-model"}}`)
	s, err := a.loadAISettings(10)
	if err != nil {
		t.Fatal(err)
	}
	c := a.runtimeAIConfig(s)
	if c.DeepSeekKey != "sensitive-key" || c.DeepSeekModel != "glm-custom" || c.VisionKey != "vision-secret" || c.VisionModel != "kimi-custom" {
		t.Fatal("runtime selection incorrect")
	}
	if c.ASRKey != "audio-secret" || c.ASRURL != "https://asr.example/v1" || c.ASRModel != "speech-model" {
		t.Fatal("audio configuration not applied")
	}
	if strings.Contains(jsonText(call(10, "GET", "")), "audio-secret") {
		t.Fatal("audio key returned")
	}
	st, err := os.Stat(a.aiSettingsPath(10))
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("incorrect file permissions", err)
	}
	req := a.aiSettingsForRequest(httptest.NewRequest("GET", "/", nil), User{ID: 10})
	if got := a.aiConfig(req.Context()); got.DeepSeekModel != "glm-custom" {
		t.Fatal("saved config not active")
	}
	other, err := a.loadAISettings(11)
	if err != nil || other.Providers["glm"].APIKey != "" {
		t.Fatal("config not isolated")
	}
	call(10, "PUT", `{"chat_provider":"glm","vision_provider":"none","providers":{"glm":{"api_key":"","chat_model":"new-model"}}}`)
	s, err = a.loadAISettings(10)
	if err != nil || s.Providers["glm"].APIKey != "sensitive-key" {
		t.Fatal("blank key did not preserve saved key")
	}
	call(10, "PUT", `{"chat_provider":"none","vision_provider":"none","providers":{"glm":{"clear_key":true}}}`)
	s, err = a.loadAISettings(10)
	if err != nil || s.Providers["glm"].APIKey != "" || a.runtimeAIConfig(s).DeepSeekKey != "" {
		t.Fatal("clear failed")
	}
	fresh := &App{Config: a.Config}
	s, err = fresh.loadAISettings(10)
	if err != nil || s.ChatProvider != "none" {
		t.Fatal("settings did not persist")
	}
	if a.aiConfig(context.Background()).DeepSeekURL != "https://legacy.example" {
		t.Fatal("legacy config changed")
	}
}

func TestAISettingsAuthenticatedFlow(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	c.request("GET", "/api/ai-config", nil, 401)
	created := c.request("POST", "/api/auth/setup", map[string]any{"username": "first", "password": "test-pass-123"}, 201)
	c.token = getString(created, "token")
	result := c.request("PUT", "/api/ai-config", map[string]any{"chat_provider": "glm", "vision_provider": "kimi", "providers": map[string]any{"glm": map[string]any{"api_key": "test-only-secret", "chat_model": "test-model"}}}, 200)
	if strings.Contains(jsonText(result), "test-only-secret") {
		t.Fatal("key returned")
	}
	got := c.request("GET", "/api/settings", nil, 200)
	if got["ai_provider"] != "glm" || got["ai_model"] != "test-model" || got["ai_enabled"] != true {
		t.Fatal(got)
	}
	c.request("PUT", "/api/ai-config", map[string]any{"chat_provider": "glm", "vision_provider": "kimi", "asr": map[string]any{"api_key": "test-asr-key", "base_url": "http://remote.invalid/v1", "model": "whisper-1"}}, 422)
	c.request("PUT", "/api/ai-config", map[string]any{"chat_provider": "glm", "vision_provider": "kimi", "asr": map[string]any{"api_key": "test-asr-key", "base_url": "https://asr.example/v1", "model": "whisper-1"}}, 200)
	got = c.request("GET", "/api/settings", nil, 200)
	if got["asr_enabled"] != true {
		t.Fatal("audio config not active")
	}
	firstToken := c.token
	_, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "second", hashPassword("second-pass"), "Second", now())
	if err != nil {
		t.Fatal(err)
	}
	second := c.request("POST", "/api/auth/login", map[string]any{"username": "second", "password": "second-pass"}, 200)
	c.token = getString(second, "token")
	other := c.request("GET", "/api/ai-config", nil, 200)
	if other["chat_provider"] == "glm" {
		t.Fatal("other account received settings")
	}
	c.token = firstToken
	cfg := c.request("GET", "/api/ai-config", nil, 200)
	if cfg["chat_provider"] != "glm" {
		t.Fatal(cfg)
	}
}
