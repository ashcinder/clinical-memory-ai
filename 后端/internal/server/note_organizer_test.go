package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrganizerDraftApprovalAndPartSearch(t *testing.T) {
	a := integrationApp(t)
	const reusable = "仅保留明确的时间信息"
	requests := make(chan string, 2)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 2 {
			w.WriteHeader(400)
			return
		}
		requests <- req.Messages[1]["content"]
		model := organizerOutput{Title: "虚构整理", Items: []organizerItem{
			{Section: "观察记录", Text: "上午出现不适", Source: 1, Quote: "上午不适"},
			{Section: "观察记录", Text: "Word 中有局部记录", Source: 2, Quote: "左肩外侧有标注"},
			{Section: "观察记录", Text: "虚构结果不得入稿", Source: 2, Quote: "原件里不存在"},
		}, Mentions: []organizerMention{{Source: 2, Term: "左肩外侧"}, {Source: 1, Term: "肩部"}}}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{
			"message": map[string]string{"content": jsonText(model)},
		}}})
	}))
	defer provider.Close()
	a.Config.AIProvider, a.Config.VisionProvider = "deepseek", "none"
	a.Config.DeepSeekURL, a.Config.DeepSeekKey = provider.URL, "organizer-test"
	c := testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", Credentials{Username: "organizer_test", Password: "organizer-test-only"}, 201), "token")
	config := `{"templates":{"record":{"sections":["观察记录","待补充"]}},"parts":[
		{"id":"LEFT1","name":"左肩区域","aliases":["左肩外侧","肩部"]},
		{"id":"RIGHT1","name":"右肩区域","aliases":["肩部"]}]}`
	if err := os.WriteFile(filepath.Join(a.Config.DataDir, "note-organizer.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	var doc bytes.Buffer
	archive := zip.NewWriter(&doc)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>左肩外侧有标注</w:t></w:r></w:p></w:document>`))
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	file := c.upload("虚构材料.docx", doc.Bytes())
	if file.Status != "ready" || file.Text != "左肩外侧有标注" {
		t.Fatalf("word extraction failed: %#v", file)
	}
	input := organizerInput{Type: "record", Text: "上午不适。提到肩部。", AttachmentIDs: []string{file.ID}, Rule: reusable}
	draft := c.request("POST", "/api/notes/organize", input, 200)
	<-requests
	if len(draft["evidence"].([]any)) != 2 || len(draft["parts"].([]any)) != 1 ||
		!strings.Contains(jsonText(draft["warnings"]), "部位待确认：肩部") ||
		!strings.Contains(jsonText(draft["missing"]), "待补充") ||
		strings.Contains(getString(draft, "content"), "虚构结果不得入稿") {
		t.Fatal(draft)
	}
	note := NoteInput{
		OrganizerDraftID: getString(draft, "id"), RememberOrganizerRule: true,
		Title: "用户修改后的标题", Content: getString(draft, "content"), NoteTypes: []string{"record"},
		Details: map[string]string{partField: "肩部"},
	}
	c.request("POST", "/api/notes", note, 422)
	note.Details[partField] = "LEFT1"
	c.request("POST", "/api/notes", note, 409)
	note.AttachmentIDs = []string{file.ID}
	approved := c.request("POST", "/api/notes", note, 201)
	if approved["details"].(map[string]any)[partField] != "左肩区域" {
		t.Fatal(approved)
	}
	for _, term := range []string{"LEFT1", "左肩区域", "左肩外侧"} {
		hits := c.request("POST", "/api/search", SearchInput{Query: term}, 200)
		if getString(hits, "engine") != "标准部位精确检索" || len(hits["items"].([]any)) != 1 {
			t.Fatal(term, hits)
		}
	}
	input.Rule = ""
	c.request("POST", "/api/notes/organize", input, 200)
	prompt := <-requests
	if !strings.Contains(prompt, reusable) || strings.Contains(prompt, note.Title) {
		t.Fatal("the approved reusable rule should be sent without the prior final note")
	}
}
