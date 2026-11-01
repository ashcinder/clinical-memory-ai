package server

import (
	"testing"
)

func TestConversationDeleteOwnershipAndCascade(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "delete_owner", Password: "delete-test-password"}, 201)
	ownerToken := getString(login, "token")
	c.token = ownerToken
	first := c.request("POST", "/api/chat", map[string]any{"message": "第一份会话", "mode": "knowledge"}, 200)
	second := c.request("POST", "/api/chat", map[string]any{"message": "第二份会话", "mode": "knowledge"}, 200)
	target, keep := getString(first, "conversation_id"), getString(second, "conversation_id")
	_, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "delete_other", hashPassword("other-delete-test"), "其他账户", now())
	if err != nil {
		t.Fatal(err)
	}
	other := c.request("POST", "/api/auth/login", Credentials{Username: "delete_other", Password: "other-delete-test"}, 200)
	c.token = getString(other, "token")
	c.request("DELETE", "/api/conversations/"+target, nil, 200)
	c.token = ownerToken
	c.request("GET", "/api/conversations/"+target, nil, 200)
	c.request("DELETE", "/api/conversations/"+target, nil, 200)
	c.request("GET", "/api/conversations/"+target, nil, 404)
	c.request("GET", "/api/conversations/"+keep, nil, 200)
	var n int
	if err := a.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id=?", target).Scan(&n); err != nil || n != 0 {
		t.Fatalf("deleted conversation retained messages: %d %v", n, err)
	}
	if err := a.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id=?", keep).Scan(&n); err != nil || n != 2 {
		t.Fatalf("another conversation changed: %d %v", n, err)
	}
	c.request("DELETE", "/api/conversations/"+target, nil, 200)
}
