package server

import "testing"

func TestRecallPinsSelectedNoteAcrossHistory(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "recall_test", Password: "recall-test-password"}, 201)
	c.token = getString(login, "token")
	selected := c.request("POST", "/api/notes", NoteInput{Title: "相同标题", Content: "选中笔记的唯一原文", Steps: []string{"选中的步骤"}}, 201)
	other := c.request("POST", "/api/notes", NoteInput{Title: "相同标题", Content: "其他笔记不可混入"}, 201)
	id := getString(selected, "id")
	checkSource := func(reply map[string]any) {
		t.Helper()
		sources := reply["sources"].([]any)
		if len(sources) != 1 || sources[0].(map[string]any)["id"] != id {
			t.Fatal("recall escaped selected note", reply)
		}
	}
	reply := c.request("POST", "/api/chat", map[string]any{"message": "请回顾这篇记录", "mode": "knowledge", "note_id": id}, 200)
	checkSource(reply)
	conversation := getString(reply, "conversation_id")
	history := c.request("GET", "/api/conversations/"+conversation, nil, 200)
	if history["conversation"].(map[string]any)["note_id"] != id {
		t.Fatal("history lost selected note")
	}
	reply = c.request("POST", "/api/chat", map[string]any{"message": "继续", "mode": "knowledge", "conversation_id": conversation}, 200)
	checkSource(reply)
	c.request("POST", "/api/chat", map[string]any{"message": "换一篇", "mode": "knowledge", "conversation_id": conversation, "note_id": other["id"]}, 409)
	c.request("POST", "/api/chat", map[string]any{"message": "回顾", "mode": "general", "note_id": id}, 422)
	c.request("DELETE", "/api/notes/"+id, nil, 200)
	c.request("POST", "/api/chat", map[string]any{"message": "继续", "mode": "knowledge", "conversation_id": conversation}, 404)
}
