package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLearningTemplateSelectionAndHistory(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", Credentials{Username: "learning_template", Password: "learning-test-password"}, 201), "token")
	makeNote := func(title, role string) string {
		result := c.request("POST", "/api/notes", NoteInput{Title: title, NoteTypes: []string{"record"},
			Content: "旧病人秘密：历史患者只用于确认范本。", LibraryRole: role}, 201)
		return getString(result, "id")
	}
	id := makeNote("只作原稿展示的姓名", "reference")
	raw := makeNote("未整理原件", "raw")
	if got := c.request("GET", "/api/notes?library_role=raw", nil, 200); len(got["items"].([]any)) != 1 ||
		getString(got["items"].([]any)[0].(map[string]any), "id") != raw {
		t.Fatal("raw role filter incorrect")
	}
	profile := learningProfile{Sections: []string{"自定查体"}, Rules: "先查体后复查。"}
	save := func(action string, revision int, rules learningProfile, def bool, restore int, status int) map[string]any {
		return c.request("PUT", "/api/notes/"+id+"/learning", map[string]any{"action": action, "revision": revision,
			"profile": rules, "is_default": def, "restore_version": restore}, status)
	}
	if catalog := c.request("GET", "/api/learning-templates", nil, 200); len(catalog["items"].([]any)) != 0 {
		t.Fatal("candidate should not participate")
	}
	save("approve", 1, profile, true, 0, 200)
	save("approve", 1, profile, false, 0, 409)

	prompts := make(chan string, 3)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 2 {
			w.WriteHeader(400)
			return
		}
		prompts <- req.Messages[1]["content"]
		result := organizerOutput{Title: "本次记录", Items: []organizerItem{
			{Section: "自定查体", Text: "左臂抬举受限。", Source: 1, Quote: "左臂抬举受限"},
		}}
		respond(w, 200, map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": jsonText(result)}}}})
	}))
	defer provider.Close()
	a.Config.AIProvider, a.Config.VisionProvider = "deepseek", "none"
	a.Config.DeepSeekURL, a.Config.DeepSeekKey = provider.URL, "local-mock-test"
	in := organizerInput{Type: "record", Text: "本次查体：左臂抬举受限。"}
	draft := c.request("POST", "/api/notes/organize", in, 200)
	prompt := <-prompts
	if strings.Contains(prompt, "历史患者") || strings.Contains(prompt, "只作原稿展示的姓名") ||
		!strings.Contains(prompt, profile.Rules) || !strings.Contains(prompt, "左臂抬举受限") {
		t.Fatal("template and input evidence were not isolated")
	}
	ref := draft["template_reference"].(map[string]any)
	if getString(ref, "note_id") != id || ref["version"] != float64(1) {
		t.Fatal(ref)
	}
	saved := c.request("POST", "/api/notes", NoteInput{Title: "本次记录", NoteTypes: []string{"record"},
		Content: getString(draft, "content"), OrganizerDraftID: getString(draft, "id"), AttachmentIDs: []string{}}, 201)
	savedID := getString(saved, "id")
	newRules := learningProfile{Sections: []string{"自定查体"}, Rules: "第二版规则。"}
	save("approve", 2, newRules, true, 0, 200)
	save("disable", 3, learningProfile{}, false, 0, 200)
	in.TemplateID = id
	c.request("POST", "/api/notes/organize", in, 422)
	save("restore", 4, learningProfile{}, true, 1, 200)
	in.TemplateID = ""
	newDraft := c.request("POST", "/api/notes/organize", in, 200)
	if !strings.Contains(<-prompts, profile.Rules) || newDraft["template_reference"].(map[string]any)["version"] != float64(3) {
		t.Fatal("previous profile was not restored as a new version")
	}
	persisted := c.request("GET", "/api/notes/"+savedID, nil, 200)
	if persisted["template_reference"].(map[string]any)["version"] != float64(1) ||
		getString(persisted["learning"].(map[string]any), "role") != "generated" {
		t.Fatal("saved note lost original provenance")
	}
}
