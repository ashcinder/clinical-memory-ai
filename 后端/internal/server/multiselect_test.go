package server

import (
	"strings"
	"testing"
)

func TestMultipleNoteTypesAndFilters(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "multi_owner", Password: "multi-only-test-pass"}, 201)
	c.token = getString(login, "token")
	create := func(title string, types, tags []string) map[string]any {
		return c.request("POST", "/api/notes", NoteInput{Title: title, NoteTypes: types, Tags: tags, Content: "MultiNeedle 原文保留", Steps: []string{"步骤保留"}}, 201)
	}
	n := create("多类型笔记", []string{"record", "tutorial", "tutorial"}, []string{"主题甲", "待复习"})
	if jsonText(n["note_types"]) != `["record","tutorial"]` {
		t.Fatal(n)
	}
	create("知识甲", []string{"knowledge"}, []string{"主题甲"})
	create("知识复盘", []string{"knowledge", "review"}, []string{"主题甲", "待复习"})
	// A union across types must not duplicate records, and tags are an intersection.
	list := c.request("GET", "/api/notes?note_types=record&note_types=tutorial&tags=主题甲&tags=待复习&limit=1", nil, 200)
	if list["total"] != float64(1) || len(list["items"].([]any)) != 1 {
		t.Fatal(list)
	}
	list = c.request("GET", "/api/notes?note_types=record&note_types=knowledge&tags=主题甲&tags=待复习&limit=1", nil, 200)
	if list["total"] != float64(2) || len(list["items"].([]any)) != 1 {
		t.Fatal("filter must precede pagination", list)
	}
	hits := c.request("POST", "/api/search", SearchInput{Query: "MultiNeedle", NoteTypes: []string{"tutorial", "knowledge"}, Tags: []string{"主题甲", "待复习"}}, 200)
	if len(hits["items"].([]any)) != 2 {
		t.Fatal(hits)
	}
	c.request("POST", "/api/search", SearchInput{Query: "MultiNeedle", NoteTypes: []string{"missing"}}, 422)
	c.request("POST", "/api/notes", NoteInput{Title: "越权类型", NoteTypes: []string{"someone_elses_type"}}, 422)
	// Older clients must retain extra types when saving a note with no array field.
	id := getString(n, "id")
	legacy := c.request("PUT", "/api/notes/"+id, NoteInput{Title: "旧客户端修改", Category: "诊疗记录", Content: "MultiNeedle 原文保留", Tags: []string{"主题甲", "待复习"}}, 200)
	if len(legacy["note_types"].([]any)) != 2 {
		t.Fatal("legacy save lost secondary types", legacy)
	}
	// Catalog counts include every type, but the overall note count stays distinct.
	cat := c.request("GET", "/api/taxonomy", nil, 200)
	for _, v := range cat["note_types"].([]any) {
		r := v.(map[string]any)
		if r["id"] == "tutorial" && r["count"] != float64(1) {
			t.Fatal(r)
		}
	}
	before := c.request("GET", "/api/sync", nil, 200)
	mutate := func(action, key, name string) {
		cat = c.request("POST", "/api/taxonomy", map[string]any{"kind": "type", "action": action, "id": key, "name": name, "expected_version": cat["version"]}, 200)
	}
	mutate("rename", "tutorial", "新版操作教程")
	hits = c.request("POST", "/api/search", SearchInput{Query: "新版操作教程"}, 200)
	if len(hits["items"].([]any)) != 1 {
		t.Fatal("secondary type name not indexed", hits)
	}
	mutate("delete", "record", "")
	saved := c.request("GET", "/api/notes/"+id, nil, 200)
	if saved["note_type"] != "tutorial" || saved["category"] != "新版操作教程" || len(saved["note_types"].([]any)) != 1 || saved["content"] != legacy["content"] {
		t.Fatal("deleting primary type lost remaining type", saved)
	}
	mutate("delete", "tutorial", "")
	saved = c.request("GET", "/api/notes/"+id, nil, 200)
	if saved["note_type"] != "other" {
		t.Fatal(saved)
	}
	// Deleting a secondary assignment preserves the primary directory.
	mutate("delete", "review", "")
	list = c.request("GET", "/api/notes?note_type=knowledge", nil, 200)
	if list["total"] != float64(2) {
		t.Fatal(list)
	}
	after := c.request("GET", "/api/sync", nil, 200)
	if before["version"] == after["version"] {
		t.Fatal("multi-type writes did not synchronize")
	}
	c.request("DELETE", "/api/notes/"+id, nil, 200)
	var count int
	check(a.DB.QueryRow("SELECT COUNT(*) FROM note_type_links WHERE note_id=?", id).Scan(&count))
	if count != 0 {
		t.Fatal("type links not cascaded")
	}
	// Unknown legacy categories are still readable as fallback without a data rewrite.
	_, err := a.DB.Exec("INSERT INTO notes(id,user_id,title,category,tags,content,steps,occurred_on,created_at,updated_at) VALUES (?,1,'旧笔记','个人旧类别','[]','旧正文','[]','2026-10-30',?,?)", newID(), now(), now())
	check(err)
	list = c.request("GET", "/api/notes?note_types=other", nil, 200)
	if list["total"] != float64(1) {
		t.Fatal(list)
	}
	if !matchesFilter(Note{NoteTypes: []string{"knowledge", "atlas"}, Tags: []string{"甲", "乙"}}, noteFilter{TypeIDs: []string{"atlas", "record"}, Tags: []string{"甲", "乙"}}) || matchesFilter(Note{NoteTypes: []string{"knowledge"}, Tags: []string{"甲"}}, noteFilter{Tags: []string{"甲", "乙"}}) {
		t.Fatal("image recall filter differs from SQL filter")
	}
	// Validating filters must reject large selections without accepting a partial query.
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = strings.Repeat("x", i+1)
	}
	c.request("POST", "/api/search", SearchInput{Query: "MultiNeedle", Tags: tooMany}, 422)
}
