package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Paid, opt-in evaluation against the configured real vision API. The entire
// database and upload directory are disposable. No patient records are involved.
// These smoke cases do NOT certify accuracy on clinical photographs.
func TestLiveImageComparison(t *testing.T) {
	if os.Getenv("LINJI_RUN_IMAGE_LIVE") != "1" {
		t.Skip("set LINJI_RUN_IMAGE_LIVE=1 for paid image evaluation")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := (&App{Config: cfg}).loadAISettings(1)
	if err != nil {
		t.Fatal(err)
	}
	runtime := (&App{Config: cfg}).runtimeAIConfig(settings)
	if runtime.CloudSearchDisabled || runtime.VisionKey == "" {
		t.Fatal("configured vision API required")
	}
	a := integrationApp(t)
	runtime.DataDir = a.Config.DataDir
	a.Config = runtime
	c := &testAPI{t: t, handler: a.Handler()}
	c.token = getString(c.request("POST", "/api/auth/setup", map[string]any{"username": "image_live_qa", "password": "image-live-test-123"}, 201), "token")
	read := func(path string) []byte {
		b, e := os.ReadFile(filepath.Join(cfg.Root, path))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	lung := read("数据库数据/测试资料/实测/lungs-anatomy.png")
	hegu := read("数据库数据/测试资料/实测/hegu.png")
	zusanli := read("数据库数据/测试资料/实测/zusanli.png")
	gallery := map[string][]byte{"lung": lung, "hegu": hegu, "zusanli": zusanli}
	ids := map[string]string{}
	for _, name := range []string{"lung", "hegu", "zusanli"} {
		f := c.upload(name+".png", gallery[name])
		if f.Status != "ready" {
			t.Fatalf("real image analysis failed: %s", f.Status)
		}
		n := c.request("POST", "/api/notes", map[string]any{"title": "QA " + name, "content": "公开解剖示意图或测试标签卡片", "attachment_ids": []string{f.ID}}, 201)
		ids[name] = n["id"].(string)
	}
	jpegData := func(raw []byte) []byte {
		im, _, e := image.Decode(bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		var b bytes.Buffer
		jpeg.Encode(&b, im, &jpeg.Options{Quality: 65})
		return b.Bytes()
	}
	im, _, _ := image.Decode(bytes.NewReader(lung))
	b := im.Bounds()
	crop := image.NewRGBA(image.Rect(0, 0, b.Dx()*7/10, b.Dy()*7/10))
	draw.Draw(crop, crop.Bounds(), im, image.Pt(b.Min.X+b.Dx()*15/100, b.Min.Y+b.Dy()*15/100), draw.Src)
	var cropped bytes.Buffer
	png.Encode(&cropped, crop)
	blank := image.NewRGBA(image.Rect(0, 0, 640, 400))
	draw.Draw(blank, blank.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	var empty bytes.Buffer
	png.Encode(&empty, blank)
	// Same broad body part, but an explicitly different named acupoint.
	card := image.NewRGBA(image.Rect(0, 0, 640, 400))
	draw.Draw(card, card.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	d := font.Drawer{Dst: card, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13, Dot: fixed.P(55, 80)}
	d.DrawString("HOUXI SI3 - HAND - ACUPOINT LABEL TEST ONLY")
	d.Dot = fixed.P(55, 115)
	d.DrawString("Not HEGU LI4. No corresponding record in the library.")
	var wrong bytes.Buffer
	png.Encode(&wrong, card)
	cases := []struct {
		name, file, want string
		data             []byte
	}{
		{"exact_lung", "exact.png", "lung", lung},
		{"compressed_lung", "compressed.jpg", "lung", read("数据库数据/测试资料/实测/lungs-query.jpg")},
		{"cropped_lung", "crop.png", "lung", cropped.Bytes()},
		{"compressed_hegu", "hegu.jpg", "hegu", jpegData(hegu)},
		{"compressed_zusanli", "zusanli.jpg", "zusanli", jpegData(zusanli)},
		{"unrelated_skin", "skin.png", "", read("前端Web/demo-files/皮肤结构示意图.png")},
		{"blank_no_answer", "blank.png", "", empty.Bytes()},
		{"different_acupoint_same_body_part", "si3.png", "", wrong.Bytes()},
	}
	report := []map[string]any{}
	correct, positive, top1, top5, negative, rejected, baselineCorrect := 0, 0, 0, 0, 0, 0, 0
	for _, test := range cases {
		f := c.upload(test.file, test.data)
		start := time.Now()
		baseline, _, _ := a.imageMatches(context.Background(), []string{f.ID}, 1, "", 20)
		baselineOK := test.want == "" && len(baseline) == 0 || test.want != "" && len(baseline) > 0 && baseline[0].ID == ids[test.want]
		if baselineOK {
			baselineCorrect++
		}
		result := c.request("POST", "/api/search", map[string]any{"attachment_ids": []string{f.ID}}, 200)
		items := result["items"].([]any)
		pass := false
		method := ""
		reasons := []string{}
		if test.want == "" {
			negative++
			pass = len(items) == 0
			if pass {
				rejected++
			}
		} else {
			positive++
			for i, item := range items {
				if item.(map[string]any)["id"] == ids[test.want] {
					if i == 0 {
						top1++
						pass = true
					}
					if i < 5 {
						top5++
					}
				}
			}
		}
		if len(items) > 0 {
			ev := items[0].(map[string]any)["search_match"].(map[string]any)
			for _, m := range ev["image_matches"].([]any) {
				v := m.(map[string]any)
				method, _ = v["method"].(string)
				if s, ok := v["reason"].(string); ok {
					reasons = append(reasons, s)
				}
			}
		}
		if pass {
			correct++
		}
		report = append(report, map[string]any{"case": test.name, "pass": pass, "fingerprint_only_pass": baselineOK, "returned": len(items), "method": method, "reasons": reasons, "warnings": result["warnings"], "seconds": time.Since(start).Seconds()})
		t.Logf("%s pass=%v method=%s returned=%d", test.name, pass, method, len(items))
	}
	data := map[string]any{"scope": "8 smoke cases; 3 source images; public anatomy and synthetic label cards; not a clinical benchmark", "model": runtime.VisionModel, "cases": report, "correct": correct, "total": len(cases), "positive_queries": positive, "top1_hits": top1, "top5_hits": top5, "negative_queries": negative, "correct_rejections": rejected, "fingerprint_only_correct": baselineCorrect, "timestamp": now()}
	if path := os.Getenv("LINJI_IMAGE_REPORT"); path != "" {
		b, _ := json.MarshalIndent(data, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if correct != len(cases) {
		t.Fatalf("real image smoke cases passed %d/%d; inspect report", correct, len(cases))
	}
}
