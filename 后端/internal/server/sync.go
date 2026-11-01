package server

import (
	"net/http"
	"strings"
)

type syncResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *syncResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *syncResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	return w.ResponseWriter.Write(p)
}
func isSyncedMutation(r *http.Request) bool {
	if r.Method == "GET" || r.Method == "HEAD" {
		return false
	}
	p := r.URL.Path
	return p == "/api/taxonomy" || p == "/api/auth/me" || p == "/api/ai-config" || p == "/api/search-settings" || p == "/api/chat" || p == "/api/demo" || p == "/api/notes" || strings.HasPrefix(p, "/api/notes/") || p == "/api/attachments" || strings.HasPrefix(p, "/api/attachments/") || strings.HasPrefix(p, "/api/conversations/")
}
func (a *App) syncState(w http.ResponseWriter, r *http.Request, u User) {
	var revision uint64
	check(a.DB.QueryRow("SELECT COALESCE((SELECT revision FROM user_sync WHERE user_id=?),0)", u.ID).Scan(&revision))
	// The file fingerprint also detects configuration changes made outside the UI.
	config, err := a.loadAISettings(u.ID)
	check(err)
	version := digest(jsonText(map[string]any{"revision": revision, "user": u, "config": config}))
	respond(w, 200, map[string]any{"version": version, "revision": revision, "user": u, "note_format": "plain-text-v1", "poll_seconds": 5})
}
func (a *App) updateProfile(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		DisplayName string `json:"display_name"`
	}
	decode(r, &in)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	bounded("显示名称", in.DisplayName, 1, 40)
	_, err := a.DB.Exec("UPDATE users SET display_name=? WHERE id=?", in.DisplayName, u.ID)
	check(err)
	u.DisplayName = in.DisplayName
	respond(w, 200, u)
}
