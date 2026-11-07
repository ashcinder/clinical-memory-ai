package server

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFuseImageCandidatesKeepsFingerprintAndLexicalResults(t *testing.T) {
	var items []Attachment
	for _, id := range []string{"text-1", "exact", "text-2", "visual-1", "visual-2", "visual-3", "visual-4", "visual-5", "visual-6"} {
		items = append(items, Attachment{ID: id})
	}
	ranks := []string{"visual-1", "exact", "visual-2", "visual-1", "visual-3", "visual-4", "visual-5", "visual-6"}
	got := fuseImageCandidates(items, ranks, map[string]bool{"exact": true}, 5)
	var ids []string
	for _, item := range got {
		ids = append(ids, item.ID)
	}
	want := []string{"exact", "visual-1", "visual-2", "visual-3", "text-1"}
	if !slices.Equal(ids, want) {
		t.Fatalf("candidates = %v, want %v", ids, want)
	}
}

func TestOfflineDINOWithCloudOff(t *testing.T) {
	model := os.Getenv("IMAGE_DINO_MODEL")
	if model == "" {
		t.Skip("set IMAGE_DINO_MODEL and IMAGE_DINO_PYTHON for actual local inference")
	}
	a := integrationApp(t)
	a.Config.ImageDINOModel = model
	a.Config.ImageDINOPython = os.Getenv("IMAGE_DINO_PYTHON")
	a.Config.AIProvider, a.Config.VisionProvider = "deepseek", "deepseek"
	a.Config.DeepSeekKey, a.Config.VisionKey = "configured-test-key", "configured-test-key"
	cloud := &denyCloud{}
	a.Client = cloud
	c := &testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", map[string]any{"username": "dino_offline", "password": "local-test-123"}, 201), "token")
	c.request("PUT", "/api/search-settings", map[string]any{"cloud_search_enabled": false}, 200)

	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(a.Config.Root, "数据库数据", "测试资料", "实测", name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	lung := read("lungs-anatomy.png")
	var target string
	for _, fixture := range []struct {
		name string
		raw  []byte
	}{
		{"lungs.png", lung}, {"hegu.png", read("hegu.png")},
	} {
		file := c.upload(fixture.name, fixture.raw)
		note := c.request("POST", "/api/notes", map[string]any{"title": fixture.name, "attachment_ids": []string{file.ID}}, 201)
		if fixture.name == "lungs.png" {
			target = getString(note, "id")
		}
	}
	original, _, err := image.Decode(bytes.NewReader(lung))
	if err != nil {
		t.Fatal(err)
	}
	size := original.Bounds()
	crop := image.NewRGBA(image.Rect(0, 0, size.Dx()*2/3, size.Dy()*2/3))
	draw.Draw(crop, crop.Bounds(), original, image.Pt(size.Min.X+size.Dx()/6, size.Min.Y+size.Dy()/6), draw.Src)
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, crop); err != nil {
		t.Fatal(err)
	}
	query := c.upload("crop.png", encoded.Bytes())
	result := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{query.ID}}, 200)
	items, _ := result["items"].([]any)
	if len(items) == 0 || getString(items[0].(map[string]any), "id") != target || !strings.Contains(getString(result, "engine"), "DINOv2") {
		t.Fatalf("local DINOv2 failed: %v", result)
	}
	evidence := items[0].(map[string]any)["search_match"].(map[string]any)["image_matches"].([]any)[0].(map[string]any)
	if evidence["method"] != "dino" || evidence["query_attachment_id"] != query.ID {
		t.Fatalf("missing local candidate evidence: %v", evidence)
	}
	if cloud.calls.Load() != 0 {
		t.Fatalf("unexpected cloud requests: %d", cloud.calls.Load())
	}
	t.Log("offline DINOv2 recovered a cropped image with 0 cloud requests")
}
