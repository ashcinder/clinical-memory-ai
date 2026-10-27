package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type sampleFile struct {
	Path   string `json:"path"`
	Text   string `json:"text"`
	SHA256 string `json:"sha256"`
}
type sampleCase struct {
	Slug       string       `json:"slug"`
	Title      string       `json:"title"`
	Category   string       `json:"category"`
	Tags       []string     `json:"tags"`
	Content    string       `json:"content"`
	Steps      []string     `json:"steps"`
	OccurredOn string       `json:"occurred_on"`
	Files      []sampleFile `json:"files"`
}
type SampleImportResult struct {
	Created  int
	Skipped  int
	Username string
}

// ImportSamples adds explicitly fictitious fixtures only to the named account (or the sole account).
// It never replaces existing notes; the transaction and new files are rolled back on failure.
func (a *App) ImportSamples(username string) (result SampleImportResult, err error) {
	root := filepath.Join(a.Config.DataDir, "测试资料")
	raw, err := os.ReadFile(filepath.Join(root, "案例.json"))
	if err != nil {
		return result, fmt.Errorf("读取测试资料：%w", err)
	}
	var cases []sampleCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		return result, err
	}
	if len(cases) == 0 || len(cases) > 20 {
		return result, errors.New("测试案例数量无效")
	}
	var userID int64
	if username == "" {
		var total int
		if err = a.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&total); err != nil {
			return result, err
		}
		if total != 1 {
			return result, fmt.Errorf("当前有 %d 个账户，请使用 -user 明确指定演示资料所属账户", total)
		}
		err = a.DB.QueryRow("SELECT id,username FROM users LIMIT 1").Scan(&userID, &result.Username)
	} else {
		err = a.DB.QueryRow("SELECT id,username FROM users WHERE username=?", username).Scan(&userID, &result.Username)
	}
	if err != nil {
		return result, fmt.Errorf("未找到指定账户：%w", err)
	}
	// Verify all source bytes and paths before writing to the database.
	for _, c := range cases {
		if c.Slug == "" || len(c.Files) == 0 {
			return result, errors.New("测试案例缺少编号或材料")
		}
		for _, file := range c.Files {
			full := filepath.Join(root, filepath.Clean(file.Path))
			rel, e := filepath.Rel(root, full)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return result, errors.New("测试文件路径无效")
			}
			bytes, e := os.ReadFile(full)
			if e != nil {
				return result, fmt.Errorf("读取 %s：%w", file.Path, e)
			}
			sum := sha256.Sum256(bytes)
			if hex.EncodeToString(sum[:]) != file.SHA256 {
				return result, fmt.Errorf("测试文件校验失败：%s", file.Path)
			}
		}
	}
	createdFiles := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, p := range createdFiles {
				_ = os.Remove(p)
			}
		}
		if value := recover(); value != nil {
			switch v := value.(type) {
			case error:
				err = v
			case apiError:
				err = v
			default:
				err = fmt.Errorf("导入失败：%v", v)
			}
		}
	}()
	result = withTx(a, func(tx *sql.Tx) SampleImportResult {
		var locked int64
		check(tx.QueryRow("SELECT id FROM users WHERE id=? FOR UPDATE", userID).Scan(&locked))
		imported := SampleImportResult{Username: result.Username}
		for _, c := range cases {
			identity := sha256.Sum256([]byte(fmt.Sprintf("linji-sample-v1:%d:%s", userID, c.Slug)))
			noteID := hex.EncodeToString(identity[:16])
			var existing int
			check(tx.QueryRow("SELECT COUNT(*) FROM notes WHERE id=? AND user_id=?", noteID, userID).Scan(&existing))
			if existing != 0 {
				imported.Skipped++
				continue
			}
			ids := []string{}
			for _, file := range c.Files {
				ext := strings.ToLower(filepath.Ext(file.Path))
				mimetype, ok := extensions[ext]
				if !ok {
					panic(fmt.Errorf("不支持的测试文件：%s", file.Path))
				}
				id := newID()
				stored := id + ext
				destination := filepath.Join(a.Config.DataDir, "uploads", stored)
				src, e := os.Open(filepath.Join(root, file.Path))
				check(e)
				dst, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
				if e != nil {
					src.Close()
					panic(e)
				}
				createdFiles = append(createdFiles, destination)
				size, e := io.Copy(dst, src)
				src.Close()
				closeErr := dst.Close()
				check(e)
				check(closeErr)
				_, e = tx.Exec("INSERT INTO attachments(id,user_id,note_id,filename,stored_name,mime,size,extracted_text,status,created_at) VALUES (?,?,NULL,?,?,?,?,?,?,?)", id, userID, filepath.Base(file.Path), stored, mimetype, size, file.Text, "ready", now())
				check(e)
				ids = append(ids, id)
			}
			a.saveNote(tx, NoteInput{Title: c.Title, Category: c.Category, Tags: c.Tags, Content: c.Content, Steps: c.Steps, OccurredOn: c.OccurredOn, AttachmentIDs: ids}, noteID, userID, true)
			imported.Created++
		}
		return imported
	})
	committed = true
	return result, nil
}
