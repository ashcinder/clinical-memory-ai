package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBasePathDeployment(t *testing.T) {
	a := integrationApp(t)
	a.Config.BasePath = "/clinical-memory"
	h := a.Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = "example.test"
		r.Header.Set("Origin", "http://example.test")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/clinical-memory", ""); w.Code != 308 || w.Header().Get("Location") != "/clinical-memory/" {
		t.Fatal("canonical URL", w.Code, w.Header())
	}
	if w := request("GET", "/clinical-memory/api/health", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, path := range []string{"/clinical-memory/", "/clinical-memory/app.js", "/clinical-memory/styles.css", "/clinical-memory/demo-assets.json"} {
		if w := request("GET", path, ""); w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	if w := request("GET", "/api/health", ""); w.Code != 404 {
		t.Fatal("root API must remain free for other projects", w.Code)
	}
	w := request("POST", "/clinical-memory/api/auth/setup", `{"username":"doctor","password":"test-password-123"}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := w.Result().Cookies()[0].Path; got != "/clinical-memory/" {
		t.Fatal("cookie scope", got)
	}
	cookie := w.Result().Cookies()[0]
	r := httptest.NewRequest("GET", "/clinical-memory/api/auth/me", nil)
	r.AddCookie(cookie)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, r)
	if response.Code != 200 {
		t.Fatal("cookie login failed", response.Code)
	}
	w = request("POST", "/clinical-memory/api/auth/logout", "")
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestReverseProxyClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Real-IP", "203.0.113.4")
	if remoteClientIP(r) != "203.0.113.4" {
		t.Fatal("loopback proxy address not honored")
	}
	r.RemoteAddr = "198.51.100.7:1234"
	if remoteClientIP(r) != "198.51.100.7" {
		t.Fatal("untrusted forwarded IP accepted")
	}
}
