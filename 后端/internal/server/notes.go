package server

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

func (a *App) createNote(w http.ResponseWriter, r *http.Request, u User) {
	var in NoteInput
	decode(r, &in)
	n := withTx(a, func(tx *sql.Tx) Note { return a.saveNote(tx, in, newID(), u.ID, true) })
	respond(w, 201, n)
}
func (a *App) updateNote(w http.ResponseWriter, r *http.Request, u User) {
	var in NoteInput
	decode(r, &in)
	n := withTx(a, func(tx *sql.Tx) Note { return a.saveNote(tx, in, r.PathValue("id"), u.ID, false) })
	respond(w, 200, n)
}
func (a *App) getNote(w http.ResponseWriter, r *http.Request, u User) {
	respond(w, 200, ownedNote(a.DB, r.PathValue("id"), u.ID, false))
}
func (a *App) deleteNote(w http.ResponseWriter, r *http.Request, u User) {
	withTx(a, func(tx *sql.Tx) bool {
		id := r.PathValue("id")
		ownedNote(tx, id, u.ID, true)
		_, err := tx.Exec("UPDATE attachments SET note_id=NULL WHERE note_id=?", id)
		check(err)
		_, err = tx.Exec("DELETE FROM notes WHERE id=?", id)
		check(err)
		return true
	})
	respond(w, 200, map[string]bool{"ok": true})
}
func (a *App) noteList(userID int64, category string, limit, offset int) ([]Note, int) {
	return a.noteListFiltered(userID, category, limit, offset, noteFilter{})
}
func (a *App) noteListFiltered(userID int64, category string, limit, offset int, filter noteFilter) ([]Note, int) {
	if filter.Types == nil {
		filter.Types = loadTaxonomy(a.DB, userID).Types
	}
	where := "1=1"
	args := []any{}
	if category != "" {
		where += " AND category=?"
		args = append(args, category)
	}
	constraint, filterArgs := filterSQL(filter, "notes.")
	where += constraint
	args = append(args, filterArgs...)
	var total int
	check(a.DB.QueryRow("SELECT COUNT(*) FROM notes WHERE "+where, args...).Scan(&total))
	rows, err := a.DB.Query("SELECT id FROM notes WHERE "+where+" ORDER BY occurred_on DESC,updated_at DESC,id LIMIT ? OFFSET ?", append(args, limit, offset)...)
	check(err)
	ids := []string{}
	for rows.Next() {
		var id string
		check(rows.Scan(&id))
		ids = append(ids, id)
	}
	check(rows.Err())
	rows.Close()
	result := []Note{}
	for _, id := range ids {
		result = append(result, ownedNote(a.DB, id, userID, false))
	}
	return result, total
}
func (a *App) listNotes(w http.ResponseWriter, r *http.Request, u User) {
	limit := intParam(r, "limit", 100, 1, 100)
	offset := intParam(r, "offset", 0, 0, 10000000)
	filter := noteFilter{Type: r.URL.Query().Get("note_type"), Tag: r.URL.Query().Get("tag"), TypeIDs: r.URL.Query()["note_types"], Tags: r.URL.Query()["tags"], Types: loadTaxonomy(a.DB, u.ID).Types}
	filter.validate()
	items, total := a.noteListFiltered(u.ID, r.URL.Query().Get("category"), limit, offset, filter)
	respond(w, 200, map[string]any{"total": total, "items": items})
}
func (a *App) stats(w http.ResponseWriter, r *http.Request, u User) {
	var notes, attachments, conversations int
	check(a.DB.QueryRow("SELECT COUNT(*) FROM notes").Scan(&notes))
	check(a.DB.QueryRow("SELECT COUNT(*) FROM attachments WHERE user_id=? OR note_id IS NOT NULL", u.ID).Scan(&attachments))
	check(a.DB.QueryRow("SELECT COUNT(*) FROM conversations WHERE user_id=?", u.ID).Scan(&conversations))
	rows, err := a.DB.Query("SELECT category,COUNT(*) FROM notes GROUP BY category ORDER BY COUNT(*) DESC,category")
	check(err)
	defer rows.Close()
	categories := []map[string]any{}
	for rows.Next() {
		var category string
		var count int
		check(rows.Scan(&category, &count))
		categories = append(categories, map[string]any{"category": category, "count": count})
	}
	check(rows.Err())
	respond(w, 200, map[string]any{"notes": notes, "attachments": attachments, "conversations": conversations, "categories": categories})
}
func (a *App) search(w http.ResponseWriter, r *http.Request, u User) {
	var in SearchInput
	decode(r, &in)
	bounded("关键词", in.Query, 0, 2000)
	bounded("分类", in.Category, 0, 40)
	filter := noteFilter{Type: in.NoteType, Tag: in.Tag, TypeIDs: in.NoteTypes, Tags: in.Tags, Types: loadTaxonomy(a.DB, u.ID).Types}
	filter.validate()
	r = r.WithContext(context.WithValue(r.Context(), noteFilterKey{}, filter))
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		fail(422, "limit 应介于 1 和 100 之间")
	}
	query, warnings := queryText(a.DB, in.Query, in.AttachmentIDs, u.ID)
	if query == "" && len(in.AttachmentIDs) == 0 {
		if len(filter.typeIDs()) == 0 && len(filter.tags()) == 0 {
			fail(422, "请输入关键词、选择类型或标签，或添加图片、录音")
		}
		notes, total := a.noteListFiltered(u.ID, in.Category, in.Limit, 0, filter)
		for i := range notes {
			notes[i].SearchMatch = searchEvidence(notes[i], "")
		}
		respond(w, 200, map[string]any{"query": "", "warnings": warnings, "items": notes, "total": total, "engine": "资料类型与标签筛选"})
		return
	}

	imageCtx := context.WithValue(r.Context(), imageIntentKey{}, in.Query)
	notes, enhanced, extraWarnings, dinoUsed := a.retrieveInput(imageCtx, query, in.AttachmentIDs, u.ID, in.Category, in.Limit)
	warnings = append(warnings, extraWarnings...)
	engine := "本地图片指纹 + 图片/音频文字索引"
	if enhanced {
		engine += " + AI 相关性筛选"
		for _, id := range in.AttachmentIDs {
			if strings.HasPrefix(ownedAttachment(a.DB, id, u.ID, false).MIME, "image/") && a.aiConfig(r.Context()).VisionKey != "" {
				engine = "图片指纹 + 结构化索引 + AI 原图视觉比对"
				break
			}
		}
	}
	if dinoUsed {
		engine = "DINOv2 候选召回 + 图片指纹与文字索引 + AI 原图视觉比对"
	}
	respond(w, 200, map[string]any{"query": query, "warnings": warnings, "items": notes, "engine": engine})
}
func (a *App) export(w http.ResponseWriter, r *http.Request, u User) {
	items, _ := a.noteList(u.ID, "", 100000000, 0)
	w.Header().Set("Content-Disposition", `attachment; filename="clinical-notes.json"`)
	respond(w, 200, map[string]any{"exported_at": now(), "notes": items})
}
func (a *App) demo(w http.ResponseWriter, r *http.Request, u User) {
	examples := []NoteInput{
		{Title: "咳嗽随访 · 治疗过程复盘", Category: "随访记录", Tags: []string{"咳嗽", "复盘", "虚构示例"}, OccurredOn: "2026-10-21", Content: "【虚构演示资料，不构成诊疗建议】\n演示编号 DEMO-001，不含真实患者信息。复盘时把首次记录、复诊变化和原始材料按时间对齐；具体诊断和处置请核对医生的实际记录。", Steps: []string{"核对首次就诊笔记与日期", "按时间整理既往处置记录，关联原始材料", "比对随访时记录的症状变化", "记录复盘结论与待补充信息"}},
		{Title: "皮疹资料 · 图片归档与观察记录", Category: "诊疗笔记", Tags: []string{"皮肤", "图片", "虚构示例"}, OccurredOn: "2026-10-18", Content: "【虚构演示资料】演示编号 DEMO-002。图片归档应记录拍摄日期和医生自己的观察，识别文本需要人工核对。", Steps: []string{"整理观察笔记", "按日期关联图片", "核对图片描述并补充原始观察"}},
		{Title: "门诊记录整理 · 复诊前的回顾清单", Category: "经验总结", Tags: []string{"复诊", "归档", "虚构示例"}, OccurredOn: "2026-10-12", Content: "【虚构演示资料】演示编号 DEMO-003。复诊前回看既往治疗步骤、随访时间点。每项回顾需能找到原始记录。", Steps: []string{"找到最近一次就诊笔记", "回看治疗步骤和随访记录", "列出仍缺少的材料"}},
	}
	withTx(a, func(tx *sql.Tx) bool {
		var id int64
		check(tx.QueryRow("SELECT id FROM shared_library WHERE id=1 FOR UPDATE").Scan(&id))
		var count int
		check(tx.QueryRow("SELECT COUNT(*) FROM notes").Scan(&count))
		if count > 0 {
			fail(409, "仅可在空资料库中导入虚构示例")
		}
		for _, in := range examples {
			a.saveNote(tx, in, newID(), u.ID, true)
		}
		return true
	})
	respond(w, 201, map[string]any{"ok": true, "count": len(examples)})
}
func (a *App) chat(w http.ResponseWriter, r *http.Request, u User) {
	var in ChatInput
	decode(r, &in)
	in.Message = strings.TrimSpace(in.Message)
	bounded("问题", in.Message, 1, 5000)
	if in.Mode == "" {
		in.Mode = "knowledge"
	}
	if in.Mode != "knowledge" && in.Mode != "general" {
		fail(422, "对话模式无效")
	}
	bounded("引用笔记", in.NoteID, 0, 32)
	if in.NoteID != "" && in.Mode != "knowledge" {
		fail(422, "引用笔记仅用于笔记回顾")
	}
	id := newID()
	history := []Message{}
	if in.ConversationID != nil {
		id = *in.ConversationID
		convo := a.ownedConversation(id, u.ID)
		if convo.Mode != in.Mode {
			fail(409, "请新建对话后切换模式")
		}
		if in.NoteID != "" && in.NoteID != convo.NoteID {
			fail(409, "请新建对话后更换回顾笔记")
		}
		in.NoteID = convo.NoteID
		rows, err := a.DB.Query("SELECT role,content FROM messages WHERE conversation_id=? ORDER BY id DESC LIMIT 12", id)
		check(err)
		for rows.Next() {
			m := Message{}
			check(rows.Scan(&m.Role, &m.Content))
			history = append(history, m)
		}
		check(rows.Err())
		rows.Close()
		for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
			history[i], history[j] = history[j], history[i]
		}
	}
	query, warnings := queryText(a.DB, in.Message, in.AttachmentIDs, u.ID)
	notes := []Note{}
	if in.Mode == "knowledge" && in.NoteID != "" {
		// An explicit selection is authoritative: do not replace it with similarly titled notes.
		notes = append(notes, ownedNote(a.DB, in.NoteID, u.ID, false))
	} else if in.Mode == "knowledge" {
		var extraWarnings []string
		notes, _, extraWarnings, _ = a.retrieveInput(context.WithValue(r.Context(), imageIntentKey{}, in.Message), query, in.AttachmentIDs, u.ID, "", 5)
		warnings = append(warnings, extraWarnings...)
		if len(notes) == 0 && strings.Contains("|继续|继续回顾|然后呢|还有呢|详细说说|", "|"+in.Message+"|") {
			for i := len(history) - 1; i >= 0; i-- {
				if history[i].Role == "user" {
					notes = a.retrieve(history[i].Content, u.ID, "", 5)
					break
				}
			}
		}
	}
	for i := range notes {
		for _, at := range notes[i].Attachments {
			notes[i].AttachmentText += at.Text + "\n"
		}
	}
	content, provider := a.answer(r.Context(), query, notes, history, in.Mode)
	sources := []Source{}
	for _, n := range notes {
		sources = append(sources, Source{n.ID, n.Title, n.OccurredOn})
	}
	userSources := []Source{}
	if in.NoteID != "" {
		userSources = sources
	}
	withTx(a, func(tx *sql.Tx) bool {
		if in.ConversationID == nil {
			_, err := tx.Exec("INSERT INTO conversations(id,user_id,mode,title,created_at) VALUES (?,?,?,?,?)", id, u.ID, in.Mode, cut(in.Message, 60), now())
			check(err)
		} else {
			var existing string
			err := tx.QueryRow("SELECT id FROM conversations WHERE id=? AND user_id=? FOR UPDATE", id, u.ID).Scan(&existing)
			if err == sql.ErrNoRows {
				fail(404, "对话已删除，请新建对话")
			}
			check(err)
		}
		_, err := tx.Exec("INSERT INTO messages(conversation_id,role,content,sources,created_at) VALUES (?,?,?,?,?),(?,?,?,?,?)", id, "user", query, jsonText(userSources), now(), id, "assistant", content, jsonText(sources), now())
		check(err)
		return true
	})
	respond(w, 200, map[string]any{"conversation_id": id, "note_id": in.NoteID, "content": content, "sources": sources, "provider": provider, "warnings": warnings})
}
func (a *App) ownedConversation(id string, userID int64) Conversation {
	c := Conversation{}
	err := a.DB.QueryRow("SELECT id,mode,title,created_at FROM conversations WHERE id=? AND user_id=?", id, userID).Scan(&c.ID, &c.Mode, &c.Title, &c.CreatedAt)
	if err == sql.ErrNoRows {
		fail(404, "对话不存在")
	}
	check(err)
	// Only explicitly selected notes are recorded in the first user message's sources.
	// Existing conversations have an empty user-source list and keep their normal search behavior.
	var raw string
	err = a.DB.QueryRow("SELECT sources FROM messages WHERE conversation_id=? AND role='user' ORDER BY id LIMIT 1", id).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		check(err)
	}
	if err == nil {
		var sources []Source
		check(jsonUnmarshal(raw, &sources))
		if len(sources) == 1 {
			c.NoteID = sources[0].ID
		}
	}
	return c
}
func (a *App) conversations(w http.ResponseWriter, r *http.Request, u User) {
	rows, err := a.DB.Query("SELECT id,mode,title,created_at FROM conversations WHERE user_id=? ORDER BY created_at DESC,id", u.ID)
	check(err)
	defer rows.Close()
	items := []Conversation{}
	for rows.Next() {
		c := Conversation{}
		check(rows.Scan(&c.ID, &c.Mode, &c.Title, &c.CreatedAt))
		items = append(items, c)
	}
	check(rows.Err())
	respond(w, 200, items)
}
func (a *App) conversation(w http.ResponseWriter, r *http.Request, u User) {
	id := r.PathValue("id")
	c := a.ownedConversation(id, u.ID)
	rows, err := a.DB.Query("SELECT id,role,content,sources,created_at FROM messages WHERE conversation_id=? ORDER BY id", id)
	check(err)
	defer rows.Close()
	items := []Message{}
	for rows.Next() {
		m := Message{Sources: []Source{}}
		var sources string
		check(rows.Scan(&m.ID, &m.Role, &m.Content, &sources, &m.CreatedAt))
		check(jsonUnmarshal(sources, &m.Sources))
		items = append(items, m)
	}
	check(rows.Err())
	respond(w, 200, map[string]any{"conversation": c, "messages": items})
}
func (a *App) deleteConversation(w http.ResponseWriter, r *http.Request, u User) {
	_, err := a.DB.Exec("DELETE FROM conversations WHERE id=? AND user_id=?", r.PathValue("id"), u.ID)
	check(err)
	respond(w, 200, map[string]bool{"ok": true})
}
