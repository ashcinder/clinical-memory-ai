package server

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

//go:embed schema.sql
var schema string

type App struct {
	DB           *sql.DB
	Config       Config
	Client       HTTPDoer
	aiSettingsMu sync.Mutex
}

func New(c Config) (*App, error) {
	db, err := sql.Open("mysql", c.MySQLConfig().FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("MySQL 配置无效，请检查后端 .env")
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("无法连接 MySQL：请先启动 MySQL，创建数据库，并检查后端/.env 中 DB_HOST、DB_PORT、DB_NAME、DB_USER、DB_PASSWORD（不会自动修改 root 密码）")
	}
	a := &App{DB: db, Config: c, Client: newAIClient()}
	if err = a.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("MySQL 建表失败，请确认应用账户拥有本项目数据库的建表权限：%w", err)
	}
	if err = os.MkdirAll(filepath.Join(c.DataDir, "uploads"), 0700); err != nil {
		db.Close()
		return nil, err
	}
	return a, nil
}
func (a *App) Migrate() error {
	for _, s := range strings.Split(schema, ";") {
		if strings.TrimSpace(s) != "" {
			if _, err := a.DB.Exec(s); err != nil {
				return err
			}
		}
	}
	return nil
}

const noteColumns = "id,title,category,tags,content,steps,DATE_FORMAT(occurred_on,'%Y-%m-%d'),created_at,updated_at"
const attachmentColumns = "id,note_id,filename,stored_name,mime,size,extracted_text,status,created_at"

func scanAttachment(row scanner) (Attachment, error) {
	a := Attachment{}
	err := row.Scan(&a.ID, &a.NoteID, &a.Filename, &a.StoredName, &a.MIME, &a.Size, &a.Text, &a.Status, &a.CreatedAt)
	return a, err
}
func scanNote(row scanner) (Note, error) {
	n := Note{Tags: []string{}, Steps: []string{}, Attachments: []Attachment{}}
	var tags, steps string
	err := row.Scan(&n.ID, &n.Title, &n.Category, &tags, &n.Content, &steps, &n.OccurredOn, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return n, err
	}
	if err = jsonUnmarshal(tags, &n.Tags); err != nil {
		return n, err
	}
	err = jsonUnmarshal(steps, &n.Steps)
	n.NoteType = noteType(n.Category).ID
	return n, err
}
func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }
func ownedNote(q queryer, id string, userID int64, lock bool) Note {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	n, err := scanNote(q.QueryRow("SELECT "+noteColumns+" FROM notes WHERE id=? AND user_id=?"+suffix, id, userID))
	if err == sql.ErrNoRows {
		fail(404, "笔记不存在")
	}
	check(err)
	n.NoteTypes = noteTypeIDs(q, id, n.Category, loadTaxonomy(q, userID).Types)
	n.NoteType = n.NoteTypes[0]
	rows, err := q.Query("SELECT "+attachmentColumns+" FROM attachments WHERE note_id=? ORDER BY created_at,id", id)
	check(err)
	defer rows.Close()
	for rows.Next() {
		a, err := scanAttachment(rows)
		check(err)
		n.Attachments = append(n.Attachments, a)
	}
	check(rows.Err())
	n.Version = digest(jsonText(n))
	return n
}
func ownedAttachment(q queryer, id string, userID int64, lock bool) Attachment {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	a, err := scanAttachment(q.QueryRow("SELECT "+attachmentColumns+" FROM attachments WHERE id=? AND user_id=?"+suffix, id, userID))
	if err == sql.ErrNoRows {
		fail(404, "材料不存在")
	}
	check(err)
	return a
}
func (a *App) saveNote(tx *sql.Tx, in NoteInput, id string, userID int64, isNew bool) Note {
	in.validate()
	var owner int64
	check(tx.QueryRow("SELECT id FROM users WHERE id=? FOR UPDATE", userID).Scan(&owner))
	catalog := loadTaxonomy(tx, userID).Types
	var current Note
	if !isNew {
		current = ownedNote(tx, id, userID, true)
		if in.ExpectedVersion != "" && in.ExpectedVersion != current.Version {
			fail(409, "这篇笔记已在其他设备更新，请保留当前输入并重新打开最新笔记")
		}
	}
	selected := in.NoteTypes
	if selected == nil {
		selected = []string{typeIn(catalog, in.Category).ID}
		if !isNew {
			for _, key := range current.NoteTypes {
				if key != current.NoteType && !contains(selected, key) {
					selected = append(selected, key)
				}
			}
		}
	} else {
		selected = validTypeIDs(catalog, selected)
		for _, t := range catalog {
			if t.ID == selected[0] {
				in.Category = t.Title
			}
		}
	}
	selected = validTypeIDs(catalog, selected)
	ids := unique(in.AttachmentIDs)
	sort.Strings(ids)
	for _, aid := range ids {
		item := ownedAttachment(tx, aid, userID, true)
		if item.NoteID != nil && *item.NoteID != id {
			fail(409, "材料已关联到其他笔记")
		}
	}
	values := []any{in.Title, in.Category, jsonText(in.Tags), in.Content, jsonText(in.Steps), in.OccurredOn, now()}
	if isNew {
		_, err := tx.Exec("INSERT INTO notes(title,category,tags,content,steps,occurred_on,updated_at,id,user_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)", append(values, id, userID, now())...)
		check(err)
	} else {
		_, err := tx.Exec("UPDATE notes SET title=?,category=?,tags=?,content=?,steps=?,occurred_on=?,updated_at=? WHERE id=? AND user_id=?", append(values, id, userID)...)
		check(err)
		_, err = tx.Exec("UPDATE attachments SET note_id=NULL WHERE note_id=?", id)
		check(err)
	}
	for _, aid := range ids {
		_, err := tx.Exec("UPDATE attachments SET note_id=? WHERE id=?", id, aid)
		check(err)
	}
	writeNoteTypes(tx, id, selected)
	reindex(tx, id, userID)
	return ownedNote(tx, id, userID, false)
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Chinese characters + bigrams are indexed explicitly, independent of MySQL ngram server settings.
func tokens(text string) []string {
	result := []string{}
	word := []rune{}
	han := []rune{}
	flushWord := func() {
		if len(word) > 0 {
			result = append(result, string(word[:min(len(word), 64)]))
			word = nil
		}
	}
	flushHan := func() {
		for i, r := range han {
			result = append(result, string(r))
			if i+1 < len(han) {
				result = append(result, string(han[i:i+2]))
			}
		}
		han = nil
	}
	for _, r := range strings.ToLower(text) {
		if unicode.Is(unicode.Han, r) {
			flushWord()
			han = append(han, r)
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			flushHan()
			word = append(word, r)
		} else {
			flushWord()
			flushHan()
		}
	}
	flushWord()
	flushHan()
	return result
}
func reindex(tx *sql.Tx, id string, userID int64) {
	n := ownedNote(tx, id, userID, false)
	_, err := tx.Exec("DELETE FROM note_terms WHERE note_id=?", id)
	check(err)
	weights := map[string]int{}
	for _, t := range tokens(n.Content + " " + strings.Join(n.Steps, " ")) {
		weights[t]++
	}
	for _, a := range n.Attachments {
		for _, t := range tokens(a.Text) {
			weights[t]++
		}
	}
	typeNames := []string{n.Category}
	for _, t := range loadTaxonomy(tx, userID).Types {
		if contains(n.NoteTypes, t.ID) {
			typeNames = append(typeNames, t.Title)
		}
	}
	for _, t := range tokens(n.Title + " " + strings.Join(typeNames, " ") + " " + strings.Join(n.Tags, " ")) {
		weights[t] += 4
	}
	terms := make([]string, 0, len(weights))
	for t := range weights {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	for start := 0; start < len(terms); start += 500 {
		end := min(start+500, len(terms))
		args := []any{}
		marks := []string{}
		for _, t := range terms[start:end] {
			marks = append(marks, "(?,?,?)")
			args = append(args, id, t, min(weights[t], 100))
		}
		_, err = tx.Exec("INSERT INTO note_terms(note_id,term,weight) VALUES "+strings.Join(marks, ","), args...)
		check(err)
	}
}
func (a *App) retrieve(query string, userID int64, category string, limit int) []Note {
	return a.retrieveFiltered(query, userID, category, limit, noteFilter{})
}
func (a *App) retrieveFiltered(query string, userID int64, category string, limit int, filter noteFilter) []Note {
	all := unique(tokens(query))
	terms := []string{}
	stop := map[string]bool{}
	for _, term := range strings.Fields("之前 步骤 怎么 什么 一个 我的 请问 帮我 记录 资料 图片 查询 查找 历史 内容 虚构 测试 个人 相关 这个 笔记 观察 材料 文字 识别 可见 可能 包括 描述 页面 标记 无法 确认 可以 进行 这张 说明 没有 文件 标签 编号 检索 原始 关联") {
		stop[term] = true
	}
	for _, t := range all {
		if len([]rune(t)) > 1 && !stop[t] {
			terms = append(terms, t)
		}
	}
	if len(terms) == 0 {
		terms = all
	}
	terms = terms[:min(100, len(terms))]
	result := []Note{}
	if len(terms) == 0 {
		return result
	}
	marks := make([]string, len(terms))
	args := []any{}
	for i, t := range terms {
		marks[i] = "?"
		args = append(args, t)
	}
	args = append(args, userID)
	statement := "SELECT n.id, SUM(t.weight) AS relevance FROM note_terms t JOIN notes n ON n.id=t.note_id WHERE t.term IN (" + strings.Join(marks, ",") + ") AND n.user_id=?"
	if category != "" {
		statement += " AND n.category=?"
		args = append(args, category)
	}
	if filter.Types == nil {
		filter.Types = loadTaxonomy(a.DB, userID).Types
	}
	constraint, filterArgs := filterSQL(filter, "n.")
	statement += constraint
	args = append(args, filterArgs...)
	statement += " GROUP BY n.id ORDER BY relevance DESC,n.id LIMIT ?"
	args = append(args, limit)
	rows, err := a.DB.Query(statement, args...)
	check(err)
	ids := []string{}
	for rows.Next() {
		var id string
		var score int
		check(rows.Scan(&id, &score))
		ids = append(ids, id)
	}
	check(rows.Err())
	rows.Close()
	for _, id := range ids {
		result = append(result, ownedNote(a.DB, id, userID, false))
	}
	return result
}
func queryText(q queryer, query string, ids []string, userID int64) (string, []string) {
	if len(ids) > 5 {
		fail(422, "搜索和对话最多添加 5 份材料")
	}
	texts := []string{strings.TrimSpace(query)}
	warnings := []string{}
	for _, id := range unique(ids) {
		a := ownedAttachment(q, id, userID, false)
		if strings.TrimSpace(a.Text) != "" {
			texts = append(texts, cut(a.Text, 4000))
		} else {
			if strings.HasPrefix(a.MIME, "image/") {
				warnings = append(warnings, a.Filename+" 未识别到文字，本次使用图片指纹匹配")
			} else {
				warnings = append(warnings, a.Filename+" 尚无转写文字，可重新提取或手动补充")
			}
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n")), warnings
}
