package server

import (
	"strings"
	"testing"
)

func TestCrossDeviceSync(t *testing.T) {
	a := integrationApp(t)
	web := &testAPI{t: t, handler: a.Handler()}
	app := &testAPI{t: t, handler: a.Handler()}
	created := web.request("POST", "/api/auth/setup", map[string]any{"username": "sync_doctor", "password": "test-pass-123", "display_name": "Initial"}, 201)
	web.token = getString(created, "token")
	logged := app.request("POST", "/api/auth/login", map[string]any{"username": "sync_doctor", "password": "test-pass-123"}, 200)
	app.token = getString(logged, "token")
	before := app.request("GET", "/api/sync", nil, 200)
	note := web.request("POST", "/api/notes", map[string]any{"title": "跨端记录", "content": "第一行\n第二行", "category": "随访记录", "tags": []string{"标签一", "标签二"}, "steps": []string{"步骤一", "步骤二"}}, 201)
	id := getString(note, "id")
	got := app.request("GET", "/api/notes/"+id, nil, 200)
	if jsonText(note) != jsonText(got) {
		t.Fatal("note fields differ")
	}
	after := app.request("GET", "/api/sync", nil, 200)
	if after["version"] == before["version"] {
		t.Fatal("note creation not signaled")
	}
	update := map[string]any{"title": "APP编辑", "content": "APP正文\n第二行", "expected_version": got["version"]}
	app.request("PUT", "/api/notes/"+id, update, 200)
	web.request("PUT", "/api/notes/"+id, update, 409)
	web.request("PUT", "/api/auth/me", map[string]string{"display_name": "New display"}, 200)
	profile := app.request("GET", "/api/auth/me", nil, 200)
	if profile["display_name"] != "New display" {
		t.Fatal(profile)
	}
	config := web.request("GET", "/api/ai-config", nil, 200)
	payload := map[string]any{"chat_provider": "glm", "vision_provider": "none", "expected_version": config["version"], "providers": map[string]any{"glm": map[string]string{"api_key": "private-test-key"}}}
	app.request("PUT", "/api/ai-config", payload, 200)
	web.request("PUT", "/api/ai-config", payload, 409)
	current := web.request("GET", "/api/ai-config", nil, 200)
	if current["chat_provider"] != "glm" || strings.Contains(jsonText(current), "private-test-key") {
		t.Fatal("settings sync or key redaction failed")
	}
}
