package server

import (
	"strings"
	"testing"
)

func TestSearchEvidence(t *testing.T) {
	n := Note{Title: "合谷 LI4 记录", Content: strings.Repeat("其他文字", 80) + "合谷是本条资料的关键词。" + strings.Repeat("其他文字", 80), Tags: []string{"合谷"}, Steps: []string{"核对资料"}, Attachments: []Attachment{{ID: "image-1", Filename: "手背.png", Text: "合谷 LI4 手背图片"}}}
	e := searchEvidence(n, "合谷 LI4")
	if !strings.Contains(e.Excerpt, "合谷") || e.Source != "正文" || len([]rune(e.Excerpt)) > 192 {
		t.Fatal(e)
	}
	if len(e.AttachmentIDs) != 1 || e.AttachmentIDs[0] != "image-1" {
		t.Fatal(e)
	}
	plain := searchEvidence(Note{Title: "独立条目", Content: "只保留原始内容"}, "没有字面相同的词")
	if len(plain.Terms) != 0 || plain.Excerpt != "只保留原始内容" {
		t.Fatal(plain)
	}
	if len(searchEvidence(Note{Title: "足三里 ST36"}, "足三里").Terms) != 1 {
		t.Fatal("phrase split unnecessarily")
	}
	short := searchEvidence(Note{Title: "Bite note", Content: "1"}, "Bite")
	if short.Excerpt != "1" || len(short.Fields) != 1 || short.Fields[0] != "title" {
		t.Fatal("do not replace sparse original content with a title or invented summary")
	}
}
