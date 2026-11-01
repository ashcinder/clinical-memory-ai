package server

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"
)

func TestNoteTypeCompatibility(t *testing.T) {
	for category, want := range map[string]string{"教程": "tutorial", "操作教程": "tutorial", "知识": "knowledge", "经验总结": "review", "诊疗笔记": "record", "随访记录": "followup", "解剖图谱": "atlas", "我的自定义分类": "other"} {
		if got := noteType(category).ID; got != want {
			t.Fatalf("%s: %s", category, got)
		}
	}
	n := Note{Category: "教程", Tags: []string{"待复习", "手部"}}
	if !matchesFilter(n, noteFilter{Type: "tutorial", Tag: "待复习"}) || matchesFilter(n, noteFilter{Type: "knowledge"}) || matchesFilter(n, noteFilter{Tag: "不存在"}) {
		t.Fatal("type/tag intersection must be exact")
	}
}

func TestReadingPassagesKeepActualEvidence(t *testing.T) {
	attachment := "材料中的目标条目：LI4 原始标注"
	n := Note{Content: "笔记介绍，不含本次检索词", Attachments: []Attachment{{ID: "original", Filename: "标注文字.txt", Text: attachment}}}
	e := searchEvidence(n, "LI4")
	if len(e.Passages) != 1 || e.Passages[0].Text != attachment || e.Passages[0].Field != "attachment" || e.Passages[0].AttachmentID != "original" || !e.Passages[0].Matched {
		t.Fatal("must show the matched attachment, not unrelated opening prose", e)
	}
	n.Content = strings.Repeat("前段资料。", 180) + "目标 LI4 内容需要保留。" + strings.Repeat("后段资料。", 180)
	e = searchEvidence(n, "LI4")
	if !strings.Contains(e.Passages[0].Text, "目标 LI4") || !e.Passages[0].Truncated || len([]rune(e.Passages[0].Text)) > 482 {
		t.Fatal("long source must focus on the actual match", e)
	}
	short := searchEvidence(Note{Content: strings.Repeat("完整短笔记。", 60)}, "无关名称")
	if short.Passages[0].Matched || short.Passages[0].Truncated || short.Passages[0].Text != strings.Repeat("完整短笔记。", 60) {
		t.Fatal("do not invent a match or shorten a readable short note")
	}
}

func TestMySQLTypeAndTagBeforeLimit(t *testing.T) {
	a := integrationApp(t)
	res, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES ('typeqa','not-a-login-hash','学习测试',?)", now())
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := res.LastInsertId()
	for _, c := range []string{"诊疗笔记", "经验总结", "教程", "操作教程", "我的分类"} {
		in := NoteInput{Title: "资料整理 " + c, Category: c, Content: "学习资料核对", Tags: []string{"待复习"}, OccurredOn: "2026-10-29"}
		withTx(a, func(tx *sql.Tx) Note { return a.saveNote(tx, in, newID(), uid, true) })
	}
	for _, typeID := range []string{"record", "review", "tutorial", "other"} {
		items, total := a.noteListFiltered(uid, "", 1, 0, noteFilter{Type: typeID, Tag: "待复习"})
		want := 1
		if typeID == "tutorial" {
			want = 2
		}
		if total != want || len(items) != 1 || items[0].NoteType != typeID {
			t.Fatalf("pagination before type filter: %s %d %+v", typeID, total, items)
		}
		hits := a.retrieveFiltered("资料", uid, "", 1, noteFilter{Type: typeID, Tag: "待复习"})
		if len(hits) != 1 || hits[0].NoteType != typeID {
			t.Fatalf("recall before type filter: %s %+v", typeID, hits)
		}
	}
	if hits := a.retrieveFiltered("资料", uid, "", 20, noteFilter{Type: "tutorial", Tag: "已掌握"}); len(hits) != 0 {
		t.Fatal("tag intersection was ignored")
	}
}

func TestLearningCatalogueHTTP(t *testing.T) {
	a := integrationApp(t)
	c := testAPI{t: t, handler: a.Handler()}
	c.request("GET", "/api/taxonomy", nil, 401)
	login := c.request("POST", "/api/auth/setup", Credentials{Username: "learning_http", Password: "learning-test-only-password"}, 201)
	c.token = getString(login, "token")
	for _, category := range []string{"教程", "操作教程", "知识", "自定义目录"} {
		n := c.request("POST", "/api/notes", NoteInput{Title: "学习资料核对", Category: category, Content: "已有原文", Tags: []string{"待复习"}}, 201)
		if n["category"] != category || n["note_type"] != noteType(category).ID {
			t.Fatal("original category must be preserved", n)
		}
	}
	other, err := a.DB.Exec("INSERT INTO users(username,password_hash,display_name,created_at) VALUES ('other_learning_owner','no-login','其他测试账户',?)", now())
	if err != nil {
		t.Fatal(err)
	}
	otherID, _ := other.LastInsertId()
	withTx(a, func(tx *sql.Tx) Note {
		return a.saveNote(tx, NoteInput{Title: "他人学习记录", Category: "教程", Tags: []string{"共享标签"}}, newID(), otherID, true)
	})
	catalogue := c.request("GET", "/api/taxonomy", nil, 200)
	if len(catalogue["note_types"].([]any)) != 7 || len(catalogue["tags"].([]any)) != 2 {
		t.Fatal(catalogue)
	}
	for _, row := range catalogue["note_types"].([]any) {
		v := row.(map[string]any)
		if v["id"] == "tutorial" && v["count"] != float64(3) {
			t.Fatal("aliases must share a directory count", v)
		}
	}
	list := c.request("GET", "/api/notes?note_type=tutorial&limit=1", nil, 200)
	if list["total"] != float64(3) || len(list["items"].([]any)) != 1 {
		t.Fatal(list)
	}
	result := c.request("POST", "/api/search", SearchInput{Query: "学习", NoteType: "tutorial", Tag: "待复习", Limit: 1}, 200)
	if len(result["items"].([]any)) != 1 || result["items"].([]any)[0].(map[string]any)["note_type"] != "tutorial" {
		t.Fatal(result)
	}
	c.request("GET", "/api/notes?note_type=unknown", nil, 422)
	c.request("POST", "/api/search", SearchInput{Query: "学习", NoteType: "unknown"}, 422)
	result = c.request("POST", "/api/search", SearchInput{Query: "学习", Tag: "不存在的标签"}, 200)
	if len(result["items"].([]any)) != 0 {
		t.Fatal("tag filter ignored", result)
	}
	at := c.upload("命中材料.txt", []byte("只有附件才有的 TARGET908 原文，必须展示。"))
	c.request("POST", "/api/notes", NoteInput{Title: "附件来源", Category: "知识要点", Content: "不含编号的普通介绍", AttachmentIDs: []string{at.ID}}, 201)
	result = c.request("POST", "/api/search", SearchInput{Query: "TARGET908"}, 200)
	n := result["items"].([]any)[0].(map[string]any)
	passage := n["search_match"].(map[string]any)["passages"].([]any)[0].(map[string]any)
	if passage["attachment_id"] != at.ID || !strings.Contains(passage["text"].(string), "TARGET908") {
		t.Fatal("source text is not traceable", passage)
	}
	// A searchable picker must also reach tags below the previous top-40 cutoff.
	for batch := 0; batch < 3; batch++ {
		tags := []string{}
		for i := 0; i < 15; i++ {
			tags = append(tags, "标签目录-"+strconv.Itoa(batch*15+i))
		}
		c.request("POST", "/api/notes", NoteInput{Title: "标签目录验证", Category: "知识要点", Content: "标签筛选测试", Tags: tags}, 201)
	}
	full := c.request("GET", "/api/taxonomy", nil, 200)
	if len(full["tags"].([]any)) != 47 {
		t.Fatal("all shared tags must be available to the picker", full)
	}

}
