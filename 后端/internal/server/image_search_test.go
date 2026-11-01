package server

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	xdraw "golang.org/x/image/draw"
)

func fingerprintFixture() image.Image {
	im := image.NewRGBA(image.Rect(0, 0, 320, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 320; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x / 2), uint8(y), uint8((x*y)%80 + 80), 255})
		}
	}
	for y := 60; y < 145; y++ {
		for x := 65; x < 220; x++ {
			im.SetRGBA(x, y, color.RGBA{30, 120, 80, 255})
		}
	}
	return im
}
func TestImageFingerprint(t *testing.T) {
	dir := t.TempDir()
	im := fingerprintFixture()
	var original bytes.Buffer
	png.Encode(&original, im)
	p := filepath.Join(dir, "a.png")
	os.WriteFile(p, original.Bytes(), 0600)
	a, e := fingerprint(p)
	if e != nil {
		t.Fatal(e)
	}
	small := image.NewRGBA(image.Rect(0, 0, 240, 150))
	xdraw.CatmullRom.Scale(small, small.Bounds(), im, im.Bounds(), xdraw.Src, nil)
	var compressed bytes.Buffer
	jpeg.Encode(&compressed, small, &jpeg.Options{Quality: 72})
	q := filepath.Join(dir, "b.jpg")
	os.WriteFile(q, compressed.Bytes(), 0600)
	b, e := fingerprint(q)
	if e != nil {
		t.Fatal(e)
	}
	if method, _, ok := fingerprintMatch(a, a); !ok || method != "exact" {
		t.Fatal("exact match failed")
	}
	if method, d, ok := fingerprintMatch(a, b); !ok || method != "perceptual" {
		t.Fatalf("resized jpeg failed distance %d", d)
	}
	blank := imageFingerprint{SHA: "other", Detail: false}
	if _, _, ok := fingerprintMatch(a, blank); ok {
		t.Fatal("unrelated/blank image matched")
	}
}

type denyCloud struct{ calls atomic.Int32 }

func (d *denyCloud) Do(r *http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return nil, apiError{502, "cloud must not be called"}
}
func TestLocalSearchAndCloudSwitch(t *testing.T) {
	a := integrationApp(t)
	c := &testAPI{t: t, handler: a.Handler()}
	login := c.request("POST", "/api/auth/setup", map[string]any{"username": "localdoctor", "password": "local-test-123"}, 201)
	c.token = getString(login, "token")
	// Keys are deliberately present: disabling the switch must prevent every cloud call.
	a.Config.AIProvider = "deepseek"
	a.Config.VisionProvider = "deepseek"
	a.Config.DeepSeekKey = "test-key"
	a.Config.VisionKey = "test-key"
	a.Config.ASRKey = "test-key"
	a.Config.ASRURL = "https://no-cloud.example"
	d := &denyCloud{}
	a.Client = d
	c.request("PUT", "/api/search-settings", map[string]any{"cloud_search_enabled": false}, 200)
	cfg := c.request("GET", "/api/ai-config", nil, 200)
	if !cfg["providers"].(map[string]any)["deepseek"].(map[string]any)["has_key"].(bool) {
		t.Fatal("测试必须保留已配置密钥")
	}
	if cfg["cloud_search_enabled"] != false {
		t.Fatal(cfg)
	}
	var raw bytes.Buffer
	png.Encode(&raw, fingerprintFixture())
	original := c.upload("history.png", raw.Bytes())
	note := c.request("POST", "/api/notes", map[string]any{"title": "独立归档条目", "content": "此处无照片关键词", "attachment_ids": []string{original.ID}}, 201)
	query := c.upload("copy.png", raw.Bytes())
	result := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != note["id"] {
		t.Fatal("image-only search", result)
	}
	evidence := items[0].(map[string]any)["search_match"].(map[string]any)
	if len(evidence["image_matches"].([]any)) != 1 {
		t.Fatal("missing image evidence")
	}
	// Reuse cached hashes as well as first-time generation.
	c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
	var rawJPEG bytes.Buffer
	jpeg.Encode(&rawJPEG, fingerprintFixture(), &jpeg.Options{Quality: 75})
	similar := c.upload("compressed.jpg", rawJPEG.Bytes())
	match := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{similar.ID}}, 200)
	if len(match["items"].([]any)) != 1 {
		t.Fatal("near duplicate", match)
	}
	audio := c.upload("speech.wav", []byte("not real audio"))
	c.request("PUT", "/api/attachments/"+audio.ID+"/text", map[string]any{"text": "音频特有词青竹档案"}, 200)
	audioNote := c.request("POST", "/api/notes", map[string]any{"title": "录音档案", "content": "文字来源于转写", "attachment_ids": []string{audio.ID}}, 201)
	textResult := c.request("POST", "/api/search", map[string]any{"query": "青竹档案"}, 200)
	if len(textResult["items"].([]any)) != 1 {
		t.Fatal(textResult)
	}
	queryAudio := c.upload("query.wav", []byte("not real audio"))
	c.request("PUT", "/api/attachments/"+queryAudio.ID+"/text", map[string]any{"text": "青竹档案"}, 200)
	byAudio := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{queryAudio.ID}}, 200)
	if byAudio["items"].([]any)[0].(map[string]any)["id"] != audioNote["id"] {
		t.Fatal(byAudio)
	}
	chat := c.request("POST", "/api/chat", map[string]any{"message": "回顾资料", "attachment_ids": []string{query.ID}, "mode": "knowledge"}, 200)
	if chat["provider"] != "local" || len(chat["sources"].([]any)) == 0 {
		t.Fatal(chat)
	}
	before := c.request("GET", "/api/sync", nil, 200)["version"]
	c.request("PUT", "/api/search-settings", map[string]any{"cloud_search_enabled": true}, 200)
	if before == c.request("GET", "/api/sync", nil, 200)["version"] {
		t.Fatal("toggle not synchronized")
	}
	c.request("PUT", "/api/search-settings", map[string]any{"cloud_search_enabled": false}, 200)
	// Another user's query attachment must never be usable.
	_, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", "otherlocal", hashPassword("other-password"), "other", now())
	if err != nil {
		t.Fatal(err)
	}
	other := c.request("POST", "/api/auth/login", map[string]any{"username": "otherlocal", "password": "other-password"}, 200)
	c.token = getString(other, "token")
	c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 404)
	if d.calls.Load() != 0 {
		t.Fatalf("cloud called %d times while disabled", d.calls.Load())
	}
}
func TestLiveOfflineExtraction(t *testing.T) {
	if os.Getenv("LINJI_RUN_OFFLINE") != "1" {
		t.Skip("set LINJI_RUN_OFFLINE=1 for actual local engines")
	}
	c, e := LoadConfig()
	if e != nil {
		t.Fatal(e)
	}
	c.CloudSearchDisabled = true
	a := &App{Config: c}
	d := &denyCloud{}
	a.Client = d
	for _, fixture := range []struct{ name, mime, term string }{{"hegu.png", "image/png", "LI4"}, {"hegu.wav", "audio/wav", "合谷"}} {
		text, status, err := a.extract(context.Background(), filepath.Join(c.Root, "数据库数据", "测试资料", "实测", fixture.name), fixture.mime)
		if err != nil || status != "ready" || !strings.Contains(strings.ToLower(text), strings.ToLower(fixture.term)) {
			t.Fatalf("%s status=%s text=%s err=%v", fixture.name, status, text, err)
		}
		t.Logf("%s 本地提取: %s", fixture.name, text)
	}
	if d.calls.Load() != 0 {
		t.Fatal("offline extraction called cloud")
	}
}
