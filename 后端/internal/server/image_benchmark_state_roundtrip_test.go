package server

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBenchmarkSnapshotRoundTrip(t *testing.T) {
	a := integrationApp(t)
	cloud := &denyCloud{}
	a.Client = cloud
	c := &testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", map[string]any{"username": "snapshot_test", "password": "snapshot-test-123"}, 201), "token")
	note := c.request("POST", "/api/notes", map[string]any{"title": "Isolated snapshot fixture"}, 201)
	id := newID()
	stored := id + ".png"
	var image bytes.Buffer
	if err := png.Encode(&image, fingerprintFixture()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.Config.DataDir, "uploads", stored)
	if err := os.WriteFile(path, image.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := a.DB.Exec("INSERT INTO attachments(id,user_id,note_id,filename,stored_name,mime,size,extracted_text,status,created_at) VALUES (?,1,?,'snapshot.png',?,'image/png',?,'人工修订文本','ready',?)", id, note["id"], stored, image.Len(), now())
	if err != nil {
		t.Fatal(err)
	}
	item := ownedAttachment(a.DB, id, 1, false)
	f, err := a.ensureFingerprint(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.saveImageProfile(context.Background(), id, f.SHA, imageProfile{Kind: "fixture", Quality: "clear"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	benchmarkSaveState(t, a, dir)
	if _, err = a.DB.Exec("DELETE FROM attachments"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.DB.Exec("DELETE FROM notes"); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	benchmarkRestoreState(t, a, dir)
	restored := ownedAttachment(a.DB, id, 1, false)
	if restored.Text != "人工修订文本" || restored.NoteID == nil || *restored.NoteID != note["id"] {
		t.Fatal("snapshot changed source records")
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, image.Bytes()) {
		t.Fatal("snapshot upload bytes changed")
	}
	var profiles int
	if err = a.DB.QueryRow("SELECT COUNT(*) FROM image_profiles WHERE attachment_id=?", id).Scan(&profiles); err != nil || profiles != 1 {
		t.Fatal("profile cache not restored")
	}
	if cloud.calls.Load() != 0 {
		t.Fatal("state roundtrip made cloud calls")
	}
}
