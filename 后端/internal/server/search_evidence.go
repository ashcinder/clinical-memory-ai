package server

import (
	"sort"
	"strings"
	"unicode"
)

type SearchEvidence struct {
	Passages      []SearchPassage `json:"passages"`
	ImageMatches  []ImageMatch    `json:"image_matches,omitempty"`
	Terms         []string        `json:"terms"`
	Fields        []string        `json:"fields"`
	Excerpt       string          `json:"excerpt"`
	Source        string          `json:"source"`
	AttachmentIDs []string        `json:"attachment_ids"`
}

// A passage is copied from saved source text; it is never an AI-written summary.
type SearchPassage struct {
	Field        string `json:"field"`
	Source       string `json:"source"`
	Text         string `json:"text"`
	AttachmentID string `json:"attachment_id,omitempty"`
	Matched      bool   `json:"matched"`
	Truncated    bool   `json:"truncated"`
}

func readingPassages(n Note, terms []string) []SearchPassage {
	type source struct{ field, label, text, id string }
	sources := []source{{"content", "笔记正文", n.Content, ""}, {"details", "专属信息", detailsText(n.Details), ""}}
	for _, f := range n.Attachments {
		sources = append(sources, source{"attachment", f.Filename, f.Text, f.ID})
	}
	sources = append(sources, source{"steps", "已记录步骤", strings.Join(n.Steps, "\n"), ""})
	out := []SearchPassage{}
	for _, s := range sources {
		for _, term := range terms {
			if strings.Contains(strings.ToLower(s.text), strings.ToLower(term)) {
				text := s.text
				if len([]rune(text)) > 800 {
					text = excerptAround(text, term, 480)
				}
				out = append(out, SearchPassage{Field: s.field, Source: s.label, Text: text, AttachmentID: s.id, Matched: true, Truncated: text != s.text})
				break
			}
		}
		if len(out) == 3 {
			break
		}
	}
	if len(out) == 0 {
		for _, s := range sources {
			if strings.TrimSpace(s.text) != "" {
				text := s.text
				if len([]rune(text)) > 800 {
					text = cut(text, 480) + "…"
				}
				out = append(out, SearchPassage{Field: s.field, Source: s.label, Text: text, AttachmentID: s.id, Truncated: text != s.text})
				break
			}
		}
	}
	return out
}

func searchEvidence(n Note, query string) *SearchEvidence {
	stop := map[string]bool{}
	for _, v := range strings.Fields("记录 资料 图片 查询 查找 历史 内容 虚构 测试 个人 请问 一下 相关 一个 这个 笔记 观察 步骤 材料 文字 识别 可见 可能 包括 描述 页面 标记 无法 确认 可以 进行 这张 说明 没有 文件 标签 编号 检索 原始") {
		stop[v] = true
	}
	terms := []string{}
	for _, term := range unique(tokens(cut(query, 1000))) {
		if len([]rune(term)) > 1 && !stop[term] {
			terms = append(terms, strings.ToLower(term))
		}
	}
	// Longer Han phrases make visible highlights more useful than isolated bigrams.
	phraseQuery := query
	if len([]rune(phraseQuery)) > 80 {
		phraseQuery = ""
	}
	for _, chunk := range strings.FieldsFunc(phraseQuery, func(r rune) bool { return !unicode.Is(unicode.Han, r) }) {
		r := []rune(chunk)
		for length := min(6, len(r)); length >= 3; length-- {
			for i := 0; i+length <= len(r); i++ {
				terms = append(terms, string(r[i:i+length]))
			}
		}
	}
	terms = unique(terms)
	sort.SliceStable(terms, func(i, j int) bool {
		if len([]rune(terms[i])) != len([]rune(terms[j])) {
			return len([]rune(terms[i])) > len([]rune(terms[j]))
		}
		return terms[i] < terms[j]
	})
	type field struct{ key, text, source, id string }
	fields := []field{{"title", n.Title, "标题", ""}, {"tags", strings.Join(n.Tags, " · "), "标签", ""}, {"content", n.Content, "正文", ""}, {"details", detailsText(n.Details), "专属信息", ""}, {"steps", strings.Join(n.Steps, "\n"), "步骤", ""}}
	for _, f := range n.Attachments {
		fields = append(fields, field{"attachment", f.Text, f.Filename, f.ID})
	}
	evidence := &SearchEvidence{Terms: []string{}, Fields: []string{}, AttachmentIDs: []string{}}
	defer func() { evidence.Passages = readingPassages(n, evidence.Terms) }()
	lowerFields := make([]string, len(fields))
	for i, f := range fields {
		lowerFields[i] = strings.ToLower(f.text)
	}
	for _, term := range terms {
		found := false
		for _, text := range lowerFields {
			if strings.Contains(text, term) {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		contained := false
		for _, old := range evidence.Terms {
			if strings.Contains(old, term) {
				contained = true
				break
			}
		}
		if !contained {
			evidence.Terms = append(evidence.Terms, term)
		}
		if len(evidence.Terms) == 8 {
			break
		}
	}
	for _, f := range fields {
		for _, term := range evidence.Terms {
			if strings.Contains(strings.ToLower(f.text), term) {
				evidence.Fields = append(evidence.Fields, f.key)
				if f.id != "" {
					evidence.AttachmentIDs = append(evidence.AttachmentIDs, f.id)
				}
				break
			}
		}
	}
	evidence.Fields = unique(evidence.Fields)
	// Prefer the content/step/material passage over repeating an already visible title.
	for _, key := range []string{"content", "steps", "attachment"} {
		for _, f := range fields {
			if f.key != key {
				continue
			}
			for _, term := range evidence.Terms {
				if strings.Contains(strings.ToLower(f.text), term) {
					evidence.Excerpt = excerptAround(f.text, term, 190)
					evidence.Source = f.source
					return evidence
				}
			}
		}
	}
	evidence.Excerpt = cut(n.Content, 190)
	evidence.Source = "正文摘录"
	if strings.TrimSpace(evidence.Excerpt) == "" && len(n.Steps) > 0 {
		evidence.Excerpt = cut(strings.Join(n.Steps, "\n"), 190)
		evidence.Source = "已记录步骤"
	}
	return evidence
}
func excerptAround(text, term string, limit int) string {
	lower := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower(term))
	position := 0
	for i := 0; i+len(needle) <= len(lower); i++ {
		if string(lower[i:i+len(needle)]) == string(needle) {
			position = i
			break
		}
	}
	r := []rune(text)
	start := max(0, position-45)
	end := min(len(r), start+limit)
	result := string(r[start:end])
	if start > 0 {
		result = "…" + result
	}
	if end < len(r) {
		result += "…"
	}
	return result
}
