package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This opt-in test uses published case fixtures and a locally installed model.
func TestOfflineLearningPublishedCase(t *testing.T) {
	model, fixtures := os.Getenv("LINJI_LEARNING_MODEL"), os.Getenv("LINJI_LEARNING_FIXTURES")
	if model == "" || fixtures == "" {
		t.Skip("set LINJI_LEARNING_MODEL and LINJI_LEARNING_FIXTURES for local-model acceptance")
	}
	a := integrationApp(t)
	a.Config.LocalOrganizerModel, a.Config.LocalOrganizerCLI = model, env("LINJI_LEARNING_CLI", "llama-cli")
	a.Config.AIProvider, a.Config.VisionProvider = "none", "none"
	c := testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", Credentials{Username: "offline_learning", Password: "learning-test-password"}, 201), "token")
	template := c.request("POST", "/api/notes", NoteInput{Title: "验收用结构范本", Content: "历史病例专用标记：鹭桐甲，不作为新病例事实。",
		NoteTypes: []string{"record"}, LibraryRole: "reference"}, 201)
	id := getString(template, "id")
	profile := learningProfile{
		Sections: []string{"病例基本信息", "初诊检查", "初诊处置", "四周随访", "八周复查", "末次复诊及结局"},
		Rules:    "按六节组织，明确四次实际到访。三个检查图分别紧跟初诊检查、初诊处置、八周复查，四周口述紧跟四周随访。记录影像变化和最终转归。材料中真实图号和日期用于配图定位。",
		Example:  "初诊检查：［日期］。［查体所见］。随后插入对应影像。",
	}
	c.request("PUT", "/api/notes/"+id+"/learning", map[string]any{"action": "approve", "revision": 1,
		"profile": profile, "is_default": true}, 200)
	raw, err := os.ReadFile(filepath.Join(fixtures, "执行记录", "uploads.json"))
	if err != nil {
		t.Fatal(err)
	}
	var original []Attachment
	if err := json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	ids := []string{}
	for _, item := range original {
		data, err := os.ReadFile(filepath.Join(fixtures, "原始材料", item.Filename))
		if err != nil {
			t.Fatal(err)
		}
		file := c.upload(item.Filename, data)
		ids = append(ids, file.ID)
		names[file.ID] = file.Filename
		if !strings.HasPrefix(item.MIME, "image/") && item.Text != "" {
			c.request("PUT", "/api/attachments/"+file.ID+"/text", map[string]string{"text": item.Text}, 200)
		}
	}
	input := organizerInput{Type: "record", AttachmentIDs: ids,
		Text: "公开论文配图索引（日期按工作流模拟）：2026-06-01 初诊左肩 X 线对应图1。2026-06-01 超声引导下注射记录对应图2。2026-07-27 第八周复查左肩 X 线对应图3。病例编号 PUB-68934。",
		Rule: "以接诊工作单为就诊时间来源。检查所见以影像报告为依据。第八周影像变化和末次转归应保留。四周随访口述与病例号 PUB-68934 是同一案例。"}
	draft := c.request("POST", "/api/notes/organize", input, 200)
	content := getString(draft, "content")
	if !strings.Contains(content, "明显减少") || !strings.Contains(content, "完全缓解") ||
		strings.Contains(content, "鹭桐甲") || getString(draft["template_reference"].(map[string]any), "note_id") != id {
		t.Fatal(content)
	}
	for _, section := range profile.Sections {
		if !strings.Contains(content, "【"+section+"】") {
			t.Fatalf("missing section %s: %s", section, content)
		}
	}
	heading := ""
	placements := map[string]string{}
	for _, item := range draft["body_blocks"].([]any) {
		block := item.(map[string]any)
		if h := getString(block, "heading"); h != "" {
			heading = h
		}
		for _, aid := range valueSlice(block["media"]) {
			placements[names[aid.(string)]] = heading
		}
	}
	for name, want := range map[string]string{"图1_初诊左肩X线.jpg": "初诊检查", "图2_超声引导操作.jpg": "初诊处置",
		"图3_八周复查左肩X线.jpg": "八周复查", "四周随访口述_合成.wav": "四周随访"} {
		if placements[name] != want {
			t.Fatalf("wrong media placement: %v", placements)
		}
	}
	note := c.request("POST", "/api/notes", NoteInput{Title: getString(draft, "title"), Content: content, NoteTypes: []string{"record"},
		AttachmentIDs: ids, OrganizerDraftID: getString(draft, "id")}, 201)
	saved := c.request("GET", "/api/notes/"+getString(note, "id"), nil, 200)
	if jsonText(saved["body_blocks"]) != jsonText(draft["body_blocks"]) ||
		jsonText(saved["template_reference"]) != jsonText(draft["template_reference"]) {
		t.Fatal("saved note differs from confirmed preview")
	}
}

func valueSlice(v any) []any {
	x, _ := v.([]any)
	return x
}
