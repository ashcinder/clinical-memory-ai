package server

import (
	"strings"
	"testing"
)

func TestNoteDetailsLifecycle(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "details_owner", Password: "details-only-test-pass"}, 201)
	c.token = getString(login, "token")
	catalog := c.request("GET", "/api/taxonomy", nil, 200)
	if len(catalog["detail_templates"].(map[string]any)) != 7 {
		t.Fatal(catalog)
	}
	input := NoteInput{Title: "专属字段测试", NoteTypes: []string{"record", "followup"}, Details: map[string]string{"患者姓名": "虚构患者", "疾病 / 诊断": "DetailNeedle", "自定义字段": "保留值"}}
	n := c.request("POST", "/api/notes", input, 201)
	id := getString(n, "id")
	n = c.request("GET", "/api/notes/"+id, nil, 200)
	if n["details"].(map[string]any)["自定义字段"] != "保留值" {
		t.Fatal(n)
	}
	hits := c.request("POST", "/api/search", SearchInput{Query: "DetailNeedle"}, 200)
	if len(hits["items"].([]any)) != 1 {
		t.Fatal(hits)
	}
	// An old client omitting details must not destroy structured information.
	input.Details = nil
	input.NoteTypes = []string{"knowledge"}
	n = c.request("PUT", "/api/notes/"+id, input, 200)
	if n["details"].(map[string]any)["患者姓名"] != "虚构患者" {
		t.Fatal(n)
	}
	exported := c.request("GET", "/api/export", nil, 200)
	if !strings.Contains(jsonText(exported), "DetailNeedle") {
		t.Fatal(exported)
	}
	input.Details = map[string]string{}
	c.request("PUT", "/api/notes/"+id, input, 200)
	hits = c.request("POST", "/api/search", SearchInput{Query: "DetailNeedle"}, 200)
	if len(hits["items"].([]any)) != 0 {
		t.Fatal("stale field search index", hits)
	}
	input.Details = map[string]string{"过长": strings.Repeat("字", 4001)}
	c.request("PUT", "/api/notes/"+id, input, 422)
	c.request("DELETE", "/api/notes/"+id, nil, 200)
	var count int
	check(a.DB.QueryRow("SELECT COUNT(*) FROM note_details WHERE note_id=?", id).Scan(&count))
	if count != 0 {
		t.Fatal("orphaned details")
	}
}
