package server

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestSharedLibraryAcrossAccounts(t *testing.T) {
	a := integrationApp(t)
	first := &testAPI{t: t, handler: a.Handler()}
	second := &testAPI{t: t, handler: a.Handler()}
	login := first.request("POST", "/api/auth/setup", Credentials{Username: "author", Password: "test-pass-123"}, 201)
	first.token = getString(login, "token")
	_, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "reader", hashPassword("second-pass-123"), "Reader", now())
	if err != nil {
		t.Fatal(err)
	}
	login = second.request("POST", "/api/auth/login", Credentials{Username: "reader", Password: "second-pass-123"}, 200)
	second.token = getString(login, "token")
	before := second.request("GET", "/api/sync", nil, 200)
	var imageData bytes.Buffer
	if err = png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 40, 40))); err != nil {
		t.Fatal(err)
	}
	file := first.upload("shared.png", imageData.Bytes())
	private := first.upload("private-query.txt", []byte("private query"))
	note := first.request("POST", "/api/notes", NoteInput{Title: "共享穴位图谱", Category: "图谱资料", Content: "共享检索测试原文", Tags: []string{"共享标签"}, AttachmentIDs: []string{file.ID}}, 201)
	id := getString(note, "id")
	got := second.request("GET", "/api/notes/"+id, nil, 200)
	if jsonText(got) != jsonText(note) {
		t.Fatal("shared note fields differ")
	}
	list := second.request("GET", "/api/notes?tags=共享标签", nil, 200)
	if list["total"] != float64(1) {
		t.Fatal("shared filter failed")
	}
	second.request("GET", "/api/attachments/"+file.ID+"/file", nil, 200)
	second.request("GET", "/api/attachments/"+private.ID+"/file", nil, 404)
	second.request("POST", "/api/search", SearchInput{AttachmentIDs: []string{private.ID}}, 404)
	query := second.upload("query.png", imageData.Bytes())
	found := second.request("POST", "/api/search", SearchInput{AttachmentIDs: []string{query.ID}}, 200)
	if len(found["items"].([]any)) != 1 {
		t.Fatal("cross-account image retrieval failed")
	}
	found = second.request("POST", "/api/search", SearchInput{Query: "共享穴位"}, 200)
	if len(found["items"].([]any)) != 1 {
		t.Fatal("cross-account text retrieval failed")
	}
	after := second.request("GET", "/api/sync", nil, 200)
	if after["version"] == before["version"] || after["library_scope"] != "shared" {
		t.Fatal("shared change not signaled")
	}
	updated := second.request("PUT", "/api/notes/"+id, NoteInput{Title: "协作编辑", Category: "图谱资料", Content: "更新原文", Tags: []string{"共享标签"}, AttachmentIDs: []string{file.ID}, ExpectedVersion: getString(got, "version")}, 200)
	if first.request("GET", "/api/notes/"+id, nil, 200)["content"] != "更新原文" {
		t.Fatal("cross-account edit not visible")
	}
	first.request("PUT", "/api/notes/"+id, NoteInput{Title: "过期覆盖", ExpectedVersion: getString(note, "version")}, 409)
	catalog := second.request("GET", "/api/taxonomy", nil, 200)
	second.request("POST", "/api/taxonomy", map[string]any{"kind": "tag", "action": "rename", "id": "共享标签", "name": "共同标签", "expected_version": catalog["version"]}, 200)
	got = first.request("GET", "/api/notes/"+id, nil, 200)
	if jsonText(got["tags"]) != `["共同标签"]` || got["version"] == updated["version"] {
		t.Fatal("shared tag mutation failed")
	}
	if jsonText(first.request("GET", "/api/taxonomy", nil, 200)) != jsonText(second.request("GET", "/api/taxonomy", nil, 200)) {
		t.Fatal("catalogs differ")
	}
	chat := first.request("POST", "/api/chat", ChatInput{Message: "共享资料", Mode: "knowledge"}, 200)
	second.request("GET", "/api/conversations/"+getString(chat, "conversation_id"), nil, 404)
	first.request("PUT", "/api/auth/me", map[string]string{"display_name": "Author changed"}, 200)
	if second.request("GET", "/api/auth/me", nil, 200)["display_name"] != "Reader" {
		t.Fatal("profile leaked")
	}
	second.request("DELETE", "/api/notes/"+id, nil, 200)
	first.request("GET", "/api/notes/"+id, nil, 404)
	first.request("GET", "/api/attachments/"+file.ID+"/file", nil, 200)
	second.request("GET", "/api/attachments/"+file.ID+"/file", nil, 404)
	(&testAPI{t: t, handler: a.Handler()}).request("GET", "/api/notes", nil, 401)
}

func TestSharedCatalogMigrationPreservesLegacy(t *testing.T) {
	a := integrationApp(t)
	for _, name := range []string{"legacy_one", "legacy_two"} {
		if _, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,'unused',?,?)", name, name, now()); err != nil {
			t.Fatal(err)
		}
	}
	left := taxonomyConfig{Types: noteTypes(), Tags: []string{"原标签"}}
	left.Types[0].Title = "原诊疗"
	right := taxonomyConfig{Types: append(noteTypes(), NoteType{ID: "custom_legacy", Title: "原自定义", Aliases: []string{"原自定义"}}), Tags: []string{"另一个标签"}}
	for i, c := range []taxonomyConfig{left, right} {
		if _, err := a.DB.Exec("INSERT INTO user_taxonomy(user_id,config) VALUES (?,?)", i+1, jsonText(c)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.DB.Exec("DELETE FROM shared_library WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(); err != nil {
		t.Fatal(err)
	}
	c := loadTaxonomy(a.DB, 2)
	if len(c.Types) != 8 || len(c.Tags) != 2 || c.Types[0].Title != "原诊疗" || !contains(c.Types[0].Aliases, "诊疗记录") {
		t.Fatal("legacy definitions lost", c)
	}
	before := jsonText(c)
	if _, err := a.DB.Exec("UPDATE shared_library SET revision=42 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := a.Migrate(); err != nil {
		t.Fatal(err)
	}
	var revision int
	if err := a.DB.QueryRow("SELECT revision FROM shared_library WHERE id=1").Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 42 || before != jsonText(loadTaxonomy(a.DB, 1)) {
		t.Fatal("migration reset shared state")
	}
}
