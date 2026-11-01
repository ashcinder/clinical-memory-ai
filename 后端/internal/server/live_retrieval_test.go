package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt-in live checks use a disposable account and only synthetic materials.
func TestLiveRetrievalAndSync(t *testing.T) {
	if os.Getenv("LINJI_RUN_LIVE") != "1" {
		t.Skip("set LINJI_RUN_LIVE=1 for paid API checks")
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	root := c.Root
	sourceID, err := strconv.ParseInt(env("LINJI_LIVE_SOURCE_USER", "1"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	config, err := (&App{Config: c}).loadAISettings(sourceID)
	if err != nil {
		t.Fatal("cannot read source configuration")
	}
	c.DataDir = t.TempDir()
	a, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.DB.Close() })
	username, password := "qa_live_"+randomHex(5), randomHex(16)
	res, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES (?,?,?,?)", username, hashPassword(password), "跨端测试医生", now())
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	t.Cleanup(func() {
		for _, q := range []string{"DELETE FROM sessions WHERE user_id=?", "DELETE FROM conversations WHERE user_id=?", "DELETE FROM attachments WHERE user_id=?", "DELETE FROM notes WHERE user_id=?", "DELETE FROM users WHERE id=?"} {
			if _, e := a.DB.Exec(q, uid); e != nil {
				t.Error("QA cleanup failed")
			}
		}
	})
	if err = os.MkdirAll(filepath.Dir(a.aiSettingsPath(uid)), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(a.aiSettingsPath(uid), []byte(jsonText(config)), 0600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	web := &http.Client{Jar: jar, Timeout: 180 * time.Second}
	android := &http.Client{Timeout: 180 * time.Second}
	token := ""
	results := []map[string]any{}
	failures := 0
	record := func(name, status string, details any, start time.Time) {
		if status == "FAIL" {
			failures++
		}
		results = append(results, map[string]any{"name": name, "status": status, "details": details, "seconds": time.Since(start).Seconds()})
	}
	verdict := func(pass bool) string {
		if pass {
			return "PASS"
		}
		return "FAIL"
	}
	call := func(client *http.Client, method, path string, body any) (int, map[string]any) {
		var reader io.Reader
		if body != nil {
			reader = strings.NewReader(jsonText(body))
		}
		req, _ := http.NewRequest(method, srv.URL+"/api"+path, reader)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if client == android && token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, e := client.Do(req)
		if e != nil {
			return 0, map[string]any{"detail": "network error"}
		}
		defer res.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&out)
		return res.StatusCode, out
	}
	upload := func(name string) (int, map[string]any) {
		data, e := os.ReadFile(filepath.Join(root, "数据库数据/测试资料/实测", name))
		if e != nil {
			t.Fatal(e)
		}
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		p, _ := w.CreateFormFile("file", name)
		_, _ = p.Write(data)
		_ = w.Close()
		req, _ := http.NewRequest("POST", srv.URL+"/api/attachments", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		res, e := android.Do(req)
		if e != nil {
			return 0, map[string]any{"detail": "network error"}
		}
		defer res.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	_, _ = call(web, "POST", "/auth/login", map[string]string{"username": username, "password": password})
	_, login := call(android, "POST", "/auth/login", map[string]string{"username": username, "password": password})
	token = getString(login, "token")
	if token == "" {
		t.Fatal("QA login failed")
	}
	start := time.Now()
	_, initial := call(android, "GET", "/sync", nil)
	status, profile := call(web, "PUT", "/auth/me", map[string]string{"display_name": "同步测试医生"})
	_, other := call(android, "GET", "/sync", nil)
	otherUser, _ := other["user"].(map[string]any)
	record("账户信息跨端同步", verdict(status == 200 && getString(profile, "display_name") == getString(otherUser, "display_name") && other["version"] != initial["version"]), map[string]any{"revision": other["revision"]}, start)
	start = time.Now()
	_, wc := call(web, "GET", "/ai-config", nil)
	_, ac := call(android, "GET", "/ai-config", nil)
	record("模型配置一致且密钥不回显", verdict(jsonText(wc) == jsonText(ac) && !strings.Contains(jsonText(wc), "api_key")), map[string]any{"chat": config.ChatProvider, "vision": config.VisionProvider}, start)
	input := func(title, body string, tags []string) map[string]any {
		return map[string]any{"title": title, "category": "经验总结", "occurred_on": "2026-10-27", "content": body, "tags": tags, "steps": []string{"核对原始记录", "整理关联图片", "标记需要复核的信息"}, "attachment_ids": []string{}}
	}
	_, hegu := call(web, "POST", "/notes", input("【虚构测试】合谷 LI4 资料回顾", "虚构检索测试。合谷 LI4，手背虎口；仅记录资料整理，不含患者或处方。", []string{"合谷", "LI4", "虎口"}))
	_, zusanli := call(web, "POST", "/notes", input("【虚构测试】足三里 ST36 资料归档", "虚构检索测试。足三里 ST36，小腿资料整理。", []string{"足三里", "ST36", "下肢"}))
	_, neiguan := call(web, "POST", "/notes", input("【虚构测试】内关 PC6 材料归档", "虚构检索测试。内关 PC6，前臂资料整理。", []string{"内关", "PC6", "前臂"}))
	id := getString(hegu, "id")
	if id == "" {
		t.Fatal("QA notes not created")
	}
	start = time.Now()
	_, same := call(android, "GET", "/notes/"+id, nil)
	record("Web笔记完整字段同步到APP", verdict(jsonText(hegu) == jsonText(same)), []string{"title", "category", "tags", "content", "steps", "occurred_on", "attachments", "version"}, start)
	start = time.Now()
	updated := input(getString(hegu, "title"), getString(hegu, "content")+"\nAPP端补充：归档编号 SYNC-2710。", []string{"合谷", "LI4", "虎口"})
	updated["expected_version"] = hegu["version"]
	status, hegu = call(android, "PUT", "/notes/"+id, updated)
	_, same = call(web, "GET", "/notes/"+id, nil)
	record("APP编辑同步到Web", verdict(status == 200 && jsonText(hegu) == jsonText(same)), nil, start)
	start = time.Now()
	status, _ = call(web, "PUT", "/notes/"+id, updated)
	record("旧笔记版本冲突保护", verdict(status == 409), status, start)
	search := func(name, q, expected string, files []string) {
		start := time.Now()
		status, result := call(android, "POST", "/search", map[string]any{"query": q, "attachment_ids": files})
		items, _ := result["items"].([]any)
		ids := []string{}
		for _, n := range items {
			ids = append(ids, getString(n.(map[string]any), "id"))
		}
		pass := status == 200
		if expected == "" {
			pass = pass && len(ids) == 0
		} else {
			pass = pass && len(ids) == 1 && ids[0] == expected
		}
		record(name, verdict(pass), map[string]any{"http_status": status, "query": q, "expected_note": expected, "ranked_note_ids": ids, "engine": result["engine"], "warnings": result["warnings"], "detail": result["detail"]}, start)
	}
	search("文字-合谷", "合谷 LI4", id, nil)
	search("文字-足三里", "足三里 ST36", getString(zusanli, "id"), nil)
	search("文字-内关", "内关 PC6", getString(neiguan, "id"), nil)
	search("自然表述-虎口", "找手背虎口的历史记录", id, nil)
	search("无关内容负例", "火星殖民地量子推进器 ZXQ9846", "", nil)
	for _, f := range []struct{ file, term, target string }{{"hegu.png", "合谷", id}, {"zusanli.png", "足三里", getString(zusanli, "id")}} {
		start = time.Now()
		status, img := upload(f.file)
		text := getString(img, "extracted_text")
		ready := status == 201 && img["status"] == "ready" && strings.Contains(text, f.term)
		record("真实图片识别-"+f.term, verdict(ready), map[string]any{"status": img["status"], "text": text, "error": img["extraction_error"]}, start)
		if ready {
			search("图片检索-"+f.term, "", f.target, []string{getString(img, "id")})
		}
	}
	// Store a real public-domain anatomical diagram, then search with a separately encoded JPEG.
	start = time.Now()
	status, anatomy := upload("lungs-anatomy.png")
	record("真实肺部解剖图识别", verdict(status == 201 && anatomy["status"] == "ready" && strings.Contains(getString(anatomy, "extracted_text"), "肺")), map[string]any{"status": anatomy["status"], "text": anatomy["extracted_text"], "error": anatomy["extraction_error"]}, start)
	if anatomy["status"] == "ready" {
		note := input("【虚构测试】肺部结构图片资料", "肺部、肺、气管、支气管与呼吸系统的资料归档，仅用于检索测试。", []string{"肺部", "支气管", "解剖图片"})
		note["attachment_ids"] = []string{getString(anatomy, "id")}
		_, lungNote := call(web, "POST", "/notes", note)
		lungID := getString(lungNote, "id")
		_, fromApp := call(android, "GET", "/notes/"+lungID, nil)
		start = time.Now()
		images, _ := fromApp["attachments"].([]any)
		record("图文混合笔记跨端同步", verdict(jsonText(lungNote) == jsonText(fromApp) && len(images) == 1), map[string]any{"attachment_count": len(images)}, start)
		start = time.Now()
		_, pictureQuery := upload("lungs-query.jpg")
		ready := pictureQuery["status"] == "ready"
		record("另一编码肺部查询图识别", verdict(ready), map[string]any{"status": pictureQuery["status"], "text": pictureQuery["extracted_text"], "error": pictureQuery["extraction_error"]}, start)
		if ready {
			search("图像检索返回对应图文笔记", "", lungID, []string{getString(pictureQuery, "id")})
		}
	}
	start = time.Now()
	status, txt := upload("query.txt")
	record("TXT提取", verdict(status == 201 && txt["status"] == "ready"), txt["status"], start)
	if status == 201 {
		search("TXT检索", "", id, []string{getString(txt, "id")})
	}
	start = time.Now()
	status, audio := upload("hegu.wav")
	aid := getString(audio, "id")
	if audio["status"] == "ready" {
		record("真实音频自动转写", verdict(strings.Contains(getString(audio, "extracted_text"), "合谷")), audio["extracted_text"], start)
		search("音频检索", "", id, []string{aid})
	} else {
		record("真实音频自动转写", "BLOCKED", map[string]any{"status": audio["status"], "configured": config.ASR.APIKey != "" && config.ASR.BaseURL != "", "error": audio["extraction_error"]}, start)
		start = time.Now()
		status, result := call(android, "POST", "/search", map[string]any{"attachment_ids": []string{aid}})
		record("无转写音频拒绝伪识别", verdict(status == 422), result, start)
		_, _ = call(web, "PUT", "/attachments/"+aid+"/text", map[string]string{"text": "【人工测试转写】请查找合谷 LI4 的历史记录。"})
		search("音频人工补充后检索", "", id, []string{aid})
	}
	start = time.Now()
	status, reply := call(web, "POST", "/chat", map[string]any{"message": "仅回顾合谷 LI4 那篇笔记的资料整理步骤，标出笔记来源。", "mode": "knowledge"})
	record("真实AI回顾与引用", verdict(status == 200 && reply["provider"] == "deepseek" && strings.Contains(getString(reply, "content"), id)), map[string]any{"http_status": status, "content": reply["content"], "sources": reply["sources"], "provider": reply["provider"], "detail": reply["detail"]}, start)
	report := map[string]any{"date": "2026-10-27", "account": "disposable", "chat": config.ChatProvider, "vision": config.VisionProvider, "results": results, "failed_checks": failures, "fixture_notice": "自制带文字标签卡片；不代表无标签真实穴位照片定位准确率。录音为中文合成语音，未走固定样例哈希匹配。"}
	b, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "后端/output/live-verification.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("checks=%d failed=%d", len(results), failures)
	if failures > 0 {
		t.Errorf("%d live checks failed; see output/live-verification.json", failures)
	}
}
