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
	"time"
	"unicode"
)

//go:embed schema.sql
var schema string

type App struct {
	DB     *sql.DB
	Config Config
	Client HTTPDoer
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
	rows, err := q.Query("SELECT "+attachmentColumns+" FROM attachments WHERE note_id=? ORDER BY created_at,id", id)
	check(err)
	defer rows.Close()
	for rows.Next() {
		a, err := scanAttachment(rows)
		check(err)
		n.Attachments = append(n.Attachments, a)
	}
	check(rows.Err())
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
	if !isNew {
		ownedNote(tx, id, userID, true)
	}
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
	for _, t := range tokens(n.Title + " " + n.Category + " " + strings.Join(n.Tags, " ")) {
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
	all := unique(tokens(query))
	terms := []string{}
	stop := map[string]bool{"之前": true, "步骤": true, "怎么": true, "什么": true, "一个": true, "我的": true, "请问": true, "帮我": true}
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
			warnings = append(warnings, a.Filename+" 尚无识别文本，请手动补充或配置识别服务")
		}
	}
	return strings.TrimSpace(strings.Join(texts, "\n")), warnings
}
