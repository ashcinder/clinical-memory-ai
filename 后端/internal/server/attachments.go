package server

import (
	"database/sql"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var extensions = map[string]string{".txt": "text/plain", ".md": "text/markdown", ".pdf": "application/pdf", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".wav": "audio/wav", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".mp4": "audio/mp4", ".ogg": "audio/ogg", ".webm": "audio/webm", ".aac": "audio/aac", ".3gp": "audio/3gpp"}

func (a *App) upload(w http.ResponseWriter, r *http.Request, u User) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			panic(err)
		}
		fail(400, "上传格式无效，请使用 multipart file 字段")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	src, header, err := r.FormFile("file")
	if err != nil {
		fail(422, "请选择材料文件")
	}
	defer src.Close()
	filename := cut(filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/")), 180)
	extension := strings.ToLower(filepath.Ext(filename))
	mimetype, ok := extensions[extension]
	if !ok {
		fail(415, "支持 JPG/PNG/WebP、音频、PDF、TXT 和 Markdown")
	}
	id := newID()
	stored := id + extension
	path := filepath.Join(a.Config.DataDir, "uploads", stored)
	dst, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	check(err)
	saved := false
	defer func() {
		if !saved {
			_ = os.Remove(path)
		}
	}()
	size, copyErr := io.Copy(dst, io.LimitReader(src, maxUpload+1))
	closeErr := dst.Close()
	check(copyErr)
	check(closeErr)
	if size > maxUpload {
		fail(413, "文件不能超过 20MB")
	}
	if size == 0 {
		fail(400, "不能上传空文件")
	}
	text, status, extractionErr := a.extract(r.Context(), path, mimetype)
	item := Attachment{ID: id, Filename: filename, StoredName: stored, MIME: mimetype, Size: size, Text: text, Status: status, CreatedAt: now()}
	if extractionErr != nil {
		if e, ok := extractionErr.(apiError); ok && e.Status == 400 {
			panic(e)
		}
		item.Status = "extraction_failed"
		detail := "模型提取失败，可重新提取或手动填写文本"
		if e, ok := extractionErr.(apiError); ok {
			detail = e.Detail
		}
		item.ExtractionError = &detail
	}
	_, err = a.DB.Exec("INSERT INTO attachments(id,user_id,note_id,filename,stored_name,mime,size,extracted_text,status,created_at) VALUES (?,?,NULL,?,?,?,?,?,?,?)", id, u.ID, filename, stored, mimetype, size, text, item.Status, item.CreatedAt)
	check(err)
	saved = true
	respond(w, 201, item)
}
func (a *App) listAttachments(w http.ResponseWriter, r *http.Request, u User) {
	rows, err := a.DB.Query("SELECT "+attachmentColumns+" FROM attachments WHERE (user_id=? OR note_id IS NOT NULL) ORDER BY created_at DESC,id", u.ID)
	check(err)
	defer rows.Close()
	items := []Attachment{}
	for rows.Next() {
		item, err := scanAttachment(rows)
		check(err)
		items = append(items, item)
	}
	check(rows.Err())
	respond(w, 200, items)
}
func (a *App) attachmentFile(w http.ResponseWriter, r *http.Request, u User) {
	item := ownedAttachment(a.DB, r.PathValue("id"), u.ID, false)
	path := filepath.Join(a.Config.DataDir, "uploads", item.StoredName)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		fail(404, "原始文件不存在，请从备份恢复")
	}
	check(err)
	defer file.Close()
	stat, err := file.Stat()
	check(err)
	w.Header().Set("Content-Type", item.MIME)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": item.Filename}))
	http.ServeContent(w, r, item.Filename, stat.ModTime(), file)
}
func (a *App) saveExtraction(id string, userID int64, text, status string) Attachment {
	return withTx(a, func(tx *sql.Tx) Attachment {
		item := ownedAttachment(tx, id, userID, true)
		_, err := tx.Exec("UPDATE attachments SET extracted_text=?,status=? WHERE id=?", text, status, id)
		check(err)
		if item.NoteID != nil {
			reindex(tx, *item.NoteID, userID)
		}
		return ownedAttachment(tx, id, userID, false)
	})
}
func (a *App) editExtraction(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Text string `json:"text"`
	}
	decode(r, &in)
	bounded("识别文本", in.Text, 0, 50000)
	status := "ready"
	if strings.TrimSpace(in.Text) == "" {
		status = "needs_transcript"
	}
	respond(w, 200, a.saveExtraction(r.PathValue("id"), u.ID, in.Text, status))
}
func (a *App) retryExtraction(w http.ResponseWriter, r *http.Request, u User) {
	item := ownedAttachment(a.DB, r.PathValue("id"), u.ID, false)
	text, status, err := a.extract(r.Context(), filepath.Join(a.Config.DataDir, "uploads", item.StoredName), item.MIME)
	check(err)
	respond(w, 200, a.saveExtraction(item.ID, u.ID, text, status))
}
func (a *App) deleteAttachment(w http.ResponseWriter, r *http.Request, u User) {
	item := withTx(a, func(tx *sql.Tx) Attachment {
		item := ownedAttachment(tx, r.PathValue("id"), u.ID, true)
		_, err := tx.Exec("DELETE FROM attachments WHERE id=?", item.ID)
		check(err)
		if item.NoteID != nil {
			reindex(tx, *item.NoteID, u.ID)
		}
		return item
	})
	if err := os.Remove(filepath.Join(a.Config.DataDir, "uploads", item.StoredName)); err != nil && !os.IsNotExist(err) {
		panic(err)
	}
	respond(w, 200, map[string]bool{"ok": true})
}
