package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

// Categories remain user-owned data. Aliases group older names without rewriting notes.
type NoteType struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Aliases      []string `json:"aliases"`
	ContentLabel string   `json:"content_label"`
	StepsLabel   string   `json:"steps_label"`
	Count        int      `json:"count"`
}

func noteTypes() []NoteType {
	return []NoteType{
		{ID: "record", Title: "诊疗记录", Description: "回看当时的观察、判断与记录经过", Aliases: []string{"诊疗记录", "诊疗笔记", "临床记录", "病例记录"}, ContentLabel: "诊疗记录", StepsLabel: "已记录过程"},
		{ID: "tutorial", Title: "操作教程", Description: "按步骤学习，结合原件核对要点", Aliases: []string{"操作教程", "教程", "操作流程", "技能教程", "临床技能"}, ContentLabel: "教程原文", StepsLabel: "操作步骤"},
		{ID: "knowledge", Title: "知识要点", Description: "查概念、辨析与知识整理", Aliases: []string{"知识要点", "知识", "知识笔记", "理论知识"}, ContentLabel: "知识原文", StepsLabel: "学习要点"},
		{ID: "review", Title: "经验复盘", Description: "回顾经验、易错点与待确认事项", Aliases: []string{"经验复盘", "经验总结", "复盘", "学习心得"}, ContentLabel: "复盘原文", StepsLabel: "复盘条目"},
		{ID: "followup", Title: "随访记录", Description: "按时间核对变化与后续记录", Aliases: []string{"随访记录", "随访", "复诊记录"}, ContentLabel: "随访原文", StepsLabel: "随访过程"},
		{ID: "atlas", Title: "图谱资料", Description: "对照图片、标注与材料说明", Aliases: []string{"图谱资料", "图谱", "图片资料", "解剖图谱"}, ContentLabel: "资料说明", StepsLabel: "核对要点"},
		{ID: "other", Title: "其他资料", Description: "保留自定义分类的资料", Aliases: []string{}, ContentLabel: "原文内容", StepsLabel: "已记录条目"},
	}
}
func noteType(category string) NoteType { return typeIn(noteTypes(), category) }
func typeIn(types []NoteType, category string) NoteType {
	for _, t := range types {
		for _, name := range append([]string{t.Title}, t.Aliases...) {
			if strings.TrimSpace(category) == name {
				return t
			}
		}
	}
	for _, t := range types {
		if t.ID == "other" {
			return t
		}
	}
	return noteTypes()[6]
}

type taxonomyConfig struct {
	Types []NoteType `json:"note_types"`
	Tags  []string   `json:"tags"`
}

func loadTaxonomy(q queryer, userID int64) taxonomyConfig {
	var raw string
	err := q.QueryRow("SELECT config FROM user_taxonomy WHERE user_id=?", userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return taxonomyConfig{Types: noteTypes(), Tags: []string{}}
	}
	check(err)
	var c taxonomyConfig
	check(json.Unmarshal([]byte(raw), &c))
	return c
}
func taxonomyVersion(c taxonomyConfig) string { return digest(jsonText(c)) }

type tagCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func taxonomyTags(q queryer, userID int64, c taxonomyConfig) []tagCount {
	counts := map[string]int{}
	for _, name := range c.Tags {
		counts[name] = 0
	}
	rows, err := q.Query("SELECT tags FROM notes WHERE user_id=?", userID)
	check(err)
	defer rows.Close()
	for rows.Next() {
		var raw string
		check(rows.Scan(&raw))
		var tags []string
		check(json.Unmarshal([]byte(raw), &tags))
		for _, name := range unique(tags) {
			counts[name]++
		}
	}
	check(rows.Err())
	tags := []tagCount{}
	for name, count := range counts {
		tags = append(tags, tagCount{name, count})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Name < tags[j].Name
	})
	return tags
}
func taxonomyPayload(q queryer, userID int64) map[string]any {
	c := loadTaxonomy(q, userID)
	types := append([]NoteType{}, c.Types...)
	rows, err := q.Query("SELECT category,COUNT(*) FROM notes WHERE user_id=? AND NOT EXISTS(SELECT 1 FROM note_type_links l WHERE l.note_id=notes.id) GROUP BY category", userID)
	check(err)
	for rows.Next() {
		var category string
		var count int
		check(rows.Scan(&category, &count))
		id := typeIn(types, category).ID
		for i := range types {
			if types[i].ID == id {
				types[i].Count += count
			}
		}
	}
	check(rows.Err())
	rows.Close()
	linked, err := q.Query("SELECT l.type_id,COUNT(*) FROM note_type_links l JOIN notes n ON n.id=l.note_id WHERE n.user_id=? GROUP BY l.type_id", userID)
	check(err)
	for linked.Next() {
		var id string
		var count int
		check(linked.Scan(&id, &count))
		for i := range types {
			if types[i].ID == id {
				types[i].Count += count
			}
		}
	}
	check(linked.Err())
	linked.Close()
	return map[string]any{"note_types": types, "tags": taxonomyTags(q, userID, c), "version": taxonomyVersion(c), "editable": true}
}
func (a *App) taxonomy(w http.ResponseWriter, r *http.Request, u User) {
	respond(w, 200, taxonomyPayload(a.DB, u.ID))
}

// Mutations are serialized per account, including note writes. Names and note indexes
// change in one transaction; deleting a directory never deletes its notes.
func (a *App) mutateTaxonomy(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Kind            string `json:"kind"`
		Action          string `json:"action"`
		ID              string `json:"id"`
		Name            string `json:"name"`
		ExpectedVersion string `json:"expected_version"`
	}
	decode(r, &in)
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind != "type" && in.Kind != "tag" {
		fail(422, "请选择资料类型或标签")
	}
	if in.Action != "create" && in.Action != "rename" && in.Action != "delete" {
		fail(422, "操作无效")
	}
	if in.Action != "delete" {
		bounded("名称", in.Name, 1, 40)
		for _, ch := range in.Name {
			if unicode.IsControl(ch) || strings.ContainsRune(",，;；\n\r", ch) {
				fail(422, "名称不能包含逗号、分号或控制字符")
			}
		}
	}
	withTx(a, func(tx *sql.Tx) bool {
		var uid int64
		check(tx.QueryRow("SELECT id FROM users WHERE id=? FOR UPDATE", u.ID).Scan(&uid))
		c := loadTaxonomy(tx, u.ID)
		beforeTypes := append([]NoteType{}, c.Types...)
		if in.ExpectedVersion == "" || in.ExpectedVersion != taxonomyVersion(c) {
			fail(409, "分类与标签已更新，请重新打开后再修改")
		}
		tags := taxonomyTags(tx, u.ID, c)
		c.Tags = []string{}
		for _, tag := range tags {
			c.Tags = append(c.Tags, tag.Name)
		}
		oldName, newName := "", ""
		aliases := []string{}
		if in.Kind == "type" {
			index := -1
			for i, t := range c.Types {
				if t.ID == in.ID {
					index = i
				}
				if in.Action != "delete" && t.ID != in.ID {
					for _, name := range append([]string{t.Title}, t.Aliases...) {
						if strings.EqualFold(name, in.Name) {
							fail(409, "资料类型名称已存在")
						}
					}
				}
			}
			if in.Action == "create" {
				if len(c.Types) >= 100 {
					fail(422, "最多保存 100 个资料类型")
				}
				t := NoteType{ID: "custom_" + newID(), Title: in.Name, Aliases: []string{in.Name}, ContentLabel: "笔记正文", StepsLabel: "记录条目"}
				i := len(c.Types) - 1
				c.Types = append(c.Types[:i], append([]NoteType{t}, c.Types[i:]...)...)
			} else {
				if index < 0 {
					fail(404, "资料类型不存在")
				}
				t := c.Types[index]
				oldName = t.Title
				aliases = unique(append([]string{t.Title}, t.Aliases...))
				if in.Action == "delete" {
					if t.ID == "other" {
						fail(422, "其他资料用于保留未分类笔记，不能删除")
					}
					newName = typeIn(c.Types, "其他资料").Title
					for _, t := range c.Types {
						if t.ID == "other" {
							newName = t.Title
						}
					}
					c.Types = append(c.Types[:index], c.Types[index+1:]...)
				} else {
					newName = in.Name
					c.Types[index].Title = in.Name
					c.Types[index].Aliases = unique(append(aliases, in.Name))
				}
			}
		} else {
			index := -1
			for i, name := range c.Tags {
				if name == in.ID {
					index = i
				}
				if in.Action != "delete" && name != in.ID && strings.EqualFold(name, in.Name) {
					fail(409, "标签名称已存在")
				}
			}
			if in.Action == "create" {
				if len(c.Tags) >= 1000 {
					fail(422, "最多保存 1000 个标签")
				}
				if index >= 0 {
					fail(409, "标签名称已存在")
				}
				c.Tags = append(c.Tags, in.Name)
			} else {
				if index < 0 {
					fail(404, "标签不存在")
				}
				oldName = in.ID
				if in.Action == "delete" {
					c.Tags = append(c.Tags[:index], c.Tags[index+1:]...)
				} else {
					newName = in.Name
					c.Tags[index] = newName
				}
			}
		}
		if in.Action != "create" {
			rows, err := tx.Query("SELECT id,category,tags FROM notes WHERE user_id=? ORDER BY id", u.ID)
			check(err)

			type change struct {
				id, category string
				tags, ids    []string
			}
			rawNotes := []change{}
			for rows.Next() {
				var id, category, raw string
				check(rows.Scan(&id, &category, &raw))
				var tags []string
				check(json.Unmarshal([]byte(raw), &tags))
				rawNotes = append(rawNotes, change{id: id, category: category, tags: tags})
			}
			check(rows.Err())
			rows.Close()
			changes := []change{}
			for _, v := range rawNotes {
				changed := false
				if in.Kind == "type" {
					ids := noteTypeIDs(tx, v.id, v.category, beforeTypes)
					if contains(ids, in.ID) {
						changed = true
						if in.Action == "delete" {
							next := []string{}
							for _, key := range ids {
								if key != in.ID {
									next = append(next, key)
								}
							}
							v.ids = validTypeIDs(c.Types, next)
							if typeIn(beforeTypes, v.category).ID == in.ID {
								for _, t := range c.Types {
									if t.ID == v.ids[0] {
										v.category = t.Title
									}
								}
							}
						}
						if in.Action == "rename" && typeIn(beforeTypes, v.category).ID == in.ID {
							v.category = newName
						}
					}
				} else {
					next := []string{}
					for _, tag := range v.tags {
						if tag == oldName {
							changed = true
							if in.Action == "rename" {
								next = append(next, newName)
							}
						} else {
							next = append(next, tag)
						}
					}
					v.tags = unique(next)
				}
				if changed {
					changes = append(changes, v)
				}
			}
			for _, v := range changes {
				_, err = tx.Exec("UPDATE notes SET category=?,tags=?,updated_at=? WHERE id=? AND user_id=?", v.category, jsonText(v.tags), now(), v.id, u.ID)
				check(err)
				if v.ids != nil {
					writeNoteTypes(tx, v.id, v.ids)
				}
			}
			// Store the directory before reindexing, so canonical note types agree.
			_, err = tx.Exec("INSERT INTO user_taxonomy(user_id,config) VALUES (?,?) ON DUPLICATE KEY UPDATE config=VALUES(config)", u.ID, jsonText(c))
			check(err)
			for _, v := range changes {
				reindex(tx, v.id, u.ID)
			}
		} else {
			_, err := tx.Exec("INSERT INTO user_taxonomy(user_id,config) VALUES (?,?) ON DUPLICATE KEY UPDATE config=VALUES(config)", u.ID, jsonText(c))
			check(err)
		}
		return true
	})
	respond(w, 200, taxonomyPayload(a.DB, u.ID))
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func validTypeIDs(types []NoteType, ids []string) []string {
	ids = unique(ids)
	if len(ids) > 20 {
		fail(422, "每篇笔记最多选择 20 个资料类型")
	}
	valid := map[string]bool{}
	for _, t := range types {
		valid[t.ID] = true
	}
	out := []string{}
	for _, id := range ids {
		if !valid[id] {
			fail(422, "资料类型已变更，请重新选择")
		}
		if id != "other" || len(ids) == 1 {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return []string{"other"}
	}
	return out
}
func noteTypeIDs(q queryer, id, category string, types []NoteType) []string {
	rows, err := q.Query("SELECT type_id FROM note_type_links WHERE note_id=? ORDER BY type_id", id)
	check(err)
	linked := []string{}
	for rows.Next() {
		var key string
		check(rows.Scan(&key))
		for _, t := range types {
			if t.ID == key {
				linked = append(linked, key)
				break
			}
		}
	}
	check(rows.Err())
	rows.Close()
	if len(linked) == 0 {
		return []string{typeIn(types, category).ID}
	}
	primary := typeIn(types, category).ID
	out := []string{}
	if contains(linked, primary) {
		out = append(out, primary)
	}
	for _, t := range types {
		if contains(linked, t.ID) && !contains(out, t.ID) {
			out = append(out, t.ID)
		}
	}
	return out
}
func writeNoteTypes(tx *sql.Tx, id string, ids []string) {
	_, err := tx.Exec("DELETE FROM note_type_links WHERE note_id=?", id)
	check(err)
	for _, key := range ids {
		_, err = tx.Exec("INSERT INTO note_type_links(note_id,type_id) VALUES (?,?)", id, key)
		check(err)
	}
}

type noteFilter struct {
	Type, Tag     string
	TypeIDs, Tags []string
	Types         []NoteType
}
type noteFilterKey struct{}

func filterFromContext(ctx context.Context) noteFilter {
	f, _ := ctx.Value(noteFilterKey{}).(noteFilter)
	return f
}
func (f noteFilter) types() []NoteType {
	if f.Types != nil {
		return f.Types
	}
	return noteTypes()
}
func (f noteFilter) typeIDs() []string {
	v := append([]string{}, f.TypeIDs...)
	if f.Type != "" {
		v = append(v, f.Type)
	}
	return unique(v)
}
func (f noteFilter) tags() []string {
	v := append([]string{}, f.Tags...)
	if f.Tag != "" {
		v = append(v, f.Tag)
	}
	return unique(v)
}
func (f noteFilter) validate() {
	if len(f.typeIDs()) > 100 || len(f.tags()) > 20 {
		fail(422, "筛选条件过多")
	}
	for _, id := range f.typeIDs() {
		valid := false
		for _, t := range f.types() {
			if t.ID == id {
				valid = true
			}
		}
		if !valid {
			fail(422, "资料类型已变更，请重新选择")
		}
	}
	for _, tag := range f.tags() {
		bounded("标签", tag, 1, 2000)
	}
}

// Types are a union; all selected tags must be present. Constraints precede LIMIT.
func filterSQL(f noteFilter, prefix string) (string, []any) {
	statement := ""
	args := []any{}
	parts := []string{}
	for _, id := range f.typeIDs() {
		names := []string{}
		for _, t := range f.types() {
			if (id == "other" && t.ID != "other") || (id != "other" && id == t.ID) {
				names = append(names, t.Title)
				names = append(names, t.Aliases...)
			}
		}
		names = unique(names)
		expr := "1=1"
		if len(names) > 0 {
			marks := []string{}
			for range names {
				marks = append(marks, "?")
			}
			op := " IN "
			if id == "other" {
				op = " NOT IN "
			}
			expr = prefix + "category" + op + "(" + strings.Join(marks, ",") + ")"
		} else if id != "other" {
			expr = "1=0"
		}
		parts = append(parts, "(EXISTS (SELECT 1 FROM note_type_links nt WHERE nt.note_id="+prefix+"id AND nt.type_id=?) OR (NOT EXISTS (SELECT 1 FROM note_type_links nx WHERE nx.note_id="+prefix+"id) AND "+expr+"))")
		args = append(args, id)
		for _, name := range names {
			args = append(args, name)
		}
	}
	if len(parts) > 0 {
		statement += " AND (" + strings.Join(parts, " OR ") + ")"
	}
	if len(f.tags()) > 0 {
		statement += " AND JSON_CONTAINS(" + prefix + "tags,?)"
		args = append(args, jsonText(f.tags()))
	}
	return statement, args
}
func matchesFilter(n Note, f noteFilter) bool {
	ids := n.NoteTypes
	if len(ids) == 0 {
		ids = []string{typeIn(f.types(), n.Category).ID}
	}
	if len(f.typeIDs()) > 0 {
		match := false
		for _, id := range f.typeIDs() {
			if contains(ids, id) {
				match = true
			}
		}
		if !match {
			return false
		}
	}
	for _, tag := range f.tags() {
		if !contains(n.Tags, tag) {
			return false
		}
	}
	return true
}
