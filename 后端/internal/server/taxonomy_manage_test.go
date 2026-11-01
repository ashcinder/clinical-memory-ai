package server

import (
	"strings"
	"testing"
)

func TestEditableTaxonomyLifecycle(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	c.request("POST", "/api/taxonomy", map[string]any{}, 401)
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "catalog_owner", Password: "catalog-only-test-pass"}, 201)
	c.token = getString(login, "token")
	catalog := c.request("GET", "/api/taxonomy", nil, 200)
	mutate := func(kind, action, id, name string, status int) {
		result := c.request("POST", "/api/taxonomy", map[string]any{"kind": kind, "action": action, "id": id, "name": name, "expected_version": catalog["version"]}, status)
		if status == 200 {
			catalog = result
		}
	}
	oldVersion := catalog["version"]
	mutate("tag", "create", "", "待核对", 200)
	if catalog["version"] == oldVersion {
		t.Fatal("unused tag was not persisted")
	}
	c.request("POST", "/api/taxonomy", map[string]any{"kind": "tag", "action": "create", "name": "覆盖风险", "expected_version": oldVersion}, 409)
	mutate("tag", "create", "", "待核对", 409)
	mutate("tag", "create", "", "bad,name", 422)
	mutate("type", "create", "", "操作教程", 409)
	mutate("type", "create", "", "入科手册", 200)
	custom := ""
	for _, v := range catalog["note_types"].([]any) {
		m := v.(map[string]any)
		if m["title"] == "入科手册" {
			custom = m["id"].(string)
		}
	}
	if custom == "" {
		t.Fatal("missing custom type")
	}
	attachment := c.upload("目录验证.txt", []byte("附件原文保留 CATALOG908"))
	first := c.request("POST", "/api/notes", NoteInput{Title: "目录检索", Category: "入科手册", Tags: []string{"待核对", "保留标签"}, Content: "目录核对 CATALOG908", Steps: []string{"步骤不变"}, AttachmentIDs: []string{attachment.ID}}, 201)
	if first["note_type"] != custom {
		t.Fatal("custom type not returned", first)
	}
	noteID := getString(first, "id")
	list := c.request("GET", "/api/notes?note_type="+custom+"&tag=待核对&limit=1", nil, 200)
	if list["total"] != float64(1) {
		t.Fatal(list)
	}
	hits := c.request("POST", "/api/search", SearchInput{Query: "CATALOG908", NoteType: custom, Tag: "待核对", Limit: 1}, 200)
	if len(hits["items"].([]any)) != 1 {
		t.Fatal("custom type not searched")
	}
	before := c.request("GET", "/api/sync", nil, 200)
	mutate("type", "rename", custom, "轮转手册", 200)
	mutate("tag", "rename", "待核对", "待复核", 200)
	note := c.request("GET", "/api/notes/"+noteID, nil, 200)
	if note["category"] != "轮转手册" || note["note_type"] != custom || !strings.Contains(jsonText(note["tags"]), "待复核") {
		t.Fatal(note)
	}
	if note["version"] == first["version"] {
		t.Fatal("note concurrency version unchanged")
	}
	c.request("PUT", "/api/notes/"+noteID, NoteInput{Title: "旧客户端覆盖", Category: "入科手册", ExpectedVersion: getString(first, "version")}, 409)
	hits = c.request("POST", "/api/search", SearchInput{Query: "待复核", NoteType: custom}, 200)
	if len(hits["items"].([]any)) != 1 {
		t.Fatal("renamed tag not reindexed")
	}
	after := c.request("GET", "/api/sync", nil, 200)
	if before["version"] == after["version"] {
		t.Fatal("catalog not synchronized")
	}
	mutate("tag", "delete", "待复核", "", 200)
	note = c.request("GET", "/api/notes/"+noteID, nil, 200)
	if strings.Contains(jsonText(note["tags"]), "待复核") || !strings.Contains(jsonText(note["tags"]), "保留标签") {
		t.Fatal(note)
	}
	hits = c.request("POST", "/api/search", SearchInput{Query: "待复核"}, 200)
	if len(hits["items"].([]any)) != 0 {
		t.Fatal("deleted tag remains indexed")
	}
	mutate("type", "delete", custom, "", 200)
	note = c.request("GET", "/api/notes/"+noteID, nil, 200)
	if note["category"] != "其他资料" || note["note_type"] != "other" || note["content"] != first["content"] || jsonText(note["steps"]) != jsonText(first["steps"]) || len(note["attachments"].([]any)) != 1 {
		t.Fatal("delete damaged note", note)
	}
	c.request("GET", "/api/notes?note_type="+custom, nil, 422)
	if ownedAttachment(a.DB, attachment.ID, 1, false).NoteID == nil {
		t.Fatal("attachment detached")
	}
	mutate("type", "delete", "other", "", 422)
	legacy := c.request("POST", "/api/notes", NoteInput{Title: "旧版教程", Category: "教程", Content: "原文保留"}, 201)
	mutate("type", "rename", "tutorial", "操作手册", 200)
	list = c.request("GET", "/api/notes?note_type=tutorial&limit=1", nil, 200)
	if list["total"] != float64(1) {
		t.Fatal(list)
	}
	note = c.request("GET", "/api/notes/"+getString(legacy, "id"), nil, 200)
	if note["category"] != "操作手册" {
		t.Fatal("alias rename ignored")
	}
	mutate("type", "delete", "tutorial", "", 200)
	note = c.request("GET", "/api/notes/"+getString(legacy, "id"), nil, 200)
	if note["note_type"] != "other" {
		t.Fatal(note)
	}
	_, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "catalog_other", hashPassword("other-catalog-pass"), "其他账户", now())
	if err != nil {
		t.Fatal(err)
	}
	ownerToken := c.token
	other := c.request("POST", "/api/auth/login", Credentials{Username: "catalog_other", Password: "other-catalog-pass"}, 200)
	c.token = getString(other, "token")
	otherCatalog := c.request("GET", "/api/taxonomy", nil, 200)
	if jsonText(otherCatalog) != jsonText(catalog) {
		t.Fatal("shared catalog differs between accounts")
	}
	c.request("POST", "/api/taxonomy", map[string]any{"kind": "type", "action": "delete", "id": "knowledge", "expected_version": otherCatalog["version"]}, 200)
	catalog = c.request("GET", "/api/taxonomy", nil, 200)
	c.token = ownerToken
	for _, v := range catalog["note_types"].([]any) {
		id := v.(map[string]any)["id"].(string)
		if id != "other" {
			mutate("type", "delete", id, "", 200)
		}
	}
	list = c.request("GET", "/api/notes?note_type=other", nil, 200)
	if list["total"] != float64(2) {
		t.Fatal(list)
	}
	mutate("type", "rename", "other", "未分类", 200)
	note = c.request("GET", "/api/notes/"+noteID, nil, 200)
	if note["category"] != "未分类" {
		t.Fatal(note)
	}
	if !matchesFilter(Note{Category: "未分类"}, noteFilter{Type: "other", Types: loadTaxonomy(a.DB, 1).Types}) {
		t.Fatal("fingerprint type filtering ignored catalog")
	}
}
