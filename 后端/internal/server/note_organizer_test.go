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
			{Section: "观察记录", Text: "左肩外侧有标注", Source: 2, Quote: "左肩外侧有标注"},
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
		!strings.Contains(getString(draft, "content"), "左肩区域有标注") ||
		!strings.Contains(jsonText(draft["evidence"]), "左肩外侧有标注") ||
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
	read := c.request("GET", "/api/notes/"+getString(approved, "id"), nil, 200)
	if strings.Contains(getString(read, "content"), "〔") ||
		!strings.Contains(jsonText(read["body_blocks"]), "左肩外侧有标注") {
		t.Fatal("saved note did not retain inline source", read)
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

func TestOrganizerBodyKeepsMediaWithSupportedParagraph(t *testing.T) {
	files := []Attachment{
		{ID: "scan", MIME: "image/jpeg", Filename: "图1_初诊.jpg"},
		{ID: "audio", MIME: "audio/wav", Filename: "随访.wav"},
		{ID: "dated", MIME: "image/jpeg", Filename: "随访-20261108.jpg"},
		{ID: "unrelated", MIME: "image/jpeg", Filename: "图10_其他检查.jpg"},
	}
	sources := []organizerSource{
		{Name: "检查记录", Text: "2026-11-07 初诊检查已完成。\n2026年11月8日复诊。\n2026-11-07 初诊对应图1。"},
		{Name: files[0].Filename, AttachmentID: files[0].ID},
		{Name: files[1].Filename, AttachmentID: files[1].ID, Text: "患者自述症状缓解。"},
		{Name: files[2].Filename, AttachmentID: files[2].ID},
		{Name: files[3].Filename, AttachmentID: files[3].ID},
	}
	draft := organizerDraft{Content: "【初诊】\n检查已完成。\n\n【复诊】\n症状缓解。", Evidence: []organizerItem{
		{Section: "初诊", Text: "检查已完成。", Source: 1, Quote: "2026-11-07 初诊检查已完成。", Media: []int{2, 4, 5, 99}},
		{Section: "复诊", Text: "症状缓解。", Source: 3, Quote: "患者自述症状缓解。", Support: []organizerReference{{1, "2026年11月8日复诊。"}}, Media: []int{4}},
	}}
	body := organizerBody(draft.Content, draft, sources, files)
	if len(body) != 4 || strings.Join(body[1].Media, ",") != "scan" ||
		strings.Join(body[3].Media, ",") != "dated,audio" || len(body[3].Citations) != 2 {
		t.Fatalf("incorrect placement: %+v", body)
	}
	edited := organizerBody(strings.Replace(draft.Content, "检查已完成。", "检查待完成。", 1), draft, sources, files)
	if len(edited[1].Citations) != 0 || len(edited[1].Media) != 0 || len(edited[3].Media) != 2 {
		t.Fatalf("edited paragraph inherited stale sources: %+v", edited)
	}
	legacy := organizerDraft{Content: "【复诊】\n症状缓解。 〔1〕\n\n【材料来源】\n〔1〕随访.wav", Evidence: []organizerItem{draft.Evidence[1]}}
	old := organizerBody(legacy.Content, legacy, sources, files)
	if len(old) != 2 || old[1].Text != "症状缓解。" || !contains(old[1].Media, "audio") {
		t.Fatalf("old citations are not displayed inline: %+v", old)
	}
}
