package server

import (
	"errors"
	"net/http"
	"testing"
)

type scopeNoModel struct{ t *testing.T }

func (s scopeNoModel) Do(*http.Request) (*http.Response, error) {
	s.t.Error("scope-only search called a cloud model")
	return nil, errors.New("cloud call forbidden in scope-only test")
}

func TestScopeTagsWithoutText(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "scope_tags", Password: "scope-only-test-pass"}, 201)
	c.token = getString(login, "token")
	a.Config.AIProvider = "deepseek"
	a.Config.DeepSeekKey = "fixture-only"
	a.Config.DeepSeekURL = "https://example.invalid/v1"
	a.Client = scopeNoModel{t: t} // Scope-only browsing must never call the model.
	c.request("POST", "/api/notes", NoteInput{Title: "双标签", NoteTypes: []string{"record", "knowledge"}, Tags: []string{"主题", "复习"}, Content: "原文保留"}, 201)
	c.request("POST", "/api/notes", NoteInput{Title: "单标签", NoteTypes: []string{"knowledge"}, Tags: []string{"主题"}}, 201)
	c.request("POST", "/api/notes", NoteInput{Title: "另一类型", NoteTypes: []string{"atlas"}, Tags: []string{"主题", "复习"}}, 201)
	c.request("POST", "/api/search", SearchInput{}, 422)
	got := c.request("POST", "/api/search", SearchInput{NoteTypes: []string{"record", "knowledge"}, Tags: []string{"主题", "复习"}, Limit: 1}, 200)
	if got["total"] != float64(1) || len(got["items"].([]any)) != 1 || got["engine"] != "资料类型与标签筛选" {
		t.Fatal(got)
	}
	evidence := got["items"].([]any)[0].(map[string]any)["search_match"].(map[string]any)
	for _, p := range evidence["passages"].([]any) {
		if p.(map[string]any)["matched"] == true {
			t.Fatal("scope-only browsing must not invent a text match")
		}
	}
	got = c.request("POST", "/api/search", SearchInput{Tags: []string{"主题", "复习"}}, 200)
	if got["total"] != float64(2) {
		t.Fatal(got)
	}
	got = c.request("POST", "/api/search", SearchInput{NoteTypes: []string{"knowledge"}}, 200)
	if got["total"] != float64(2) {
		t.Fatal(got)
	}
	got = c.request("POST", "/api/search", SearchInput{Tags: []string{"无匹配"}}, 200)
	if got["total"] != float64(0) {
		t.Fatal(got)
	}
	c.request("POST", "/api/search", SearchInput{NoteTypes: []string{"invalid"}}, 422)
}
