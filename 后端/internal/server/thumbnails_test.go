package server

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestThumbnailAndSearchEvidence(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", map[string]any{"username": "doctor", "password": "test-pass-123"}, 201)
	c.token = getString(login, "token")
	var raw bytes.Buffer
	_ = png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 800, 400)))
	file := c.upload("diagram.png", raw.Bytes())
	note := c.request("POST", "/api/notes", map[string]any{"title": "合谷 LI4", "content": "合谷的历史资料", "attachment_ids": []string{file.ID}}, 201)
	found := c.request("POST", "/api/search", map[string]any{"query": "合谷"}, 200)
	items := found["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["search_match"] == nil {
		t.Fatal("missing evidence")
	}
	if items[0].(map[string]any)["version"] != note["version"] {
		t.Fatal("evidence must not change note version")
	}
	request := func(token string, expected int) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/attachments/"+file.ID+"/thumbnail", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		c.handler.ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("thumbnail status %d", w.Code)
		}
		return w
	}
	request("", 401)
	w := request(c.token, 200)
	if w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal(w.Header())
	}
	decoded, err := jpeg.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
	if err != nil || decoded.Width != 384 || decoded.Height != 192 {
		t.Fatal(decoded, err)
	}
	_, err = a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "other", hashPassword("other-pass"), "Other", now())
	if err != nil {
		t.Fatal(err)
	}
	other := c.request("POST", "/api/auth/login", map[string]any{"username": "other", "password": "other-pass"}, 200)
	request(getString(other, "token"), 200)
}
