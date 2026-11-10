package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
type Attachment struct {
	ID              string  `json:"id"`
	NoteID          *string `json:"note_id"`
	Filename        string  `json:"filename"`
	StoredName      string  `json:"-"`
	MIME            string  `json:"mime"`
	Size            int64   `json:"size"`
	Text            string  `json:"extracted_text"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	ExtractionError *string `json:"extraction_error,omitempty"`
}
type Note struct {
	Learning          learningMeta       `json:"learning"`
	TemplateReference *learningReference `json:"template_reference,omitempty"`
	BodyBlocks        []organizerBlock   `json:"body_blocks,omitempty"`
	Details           map[string]string  `json:"details"`
	NoteType          string             `json:"note_type"`
	NoteTypes         []string           `json:"note_types"`
	SearchMatch       *SearchEvidence    `json:"search_match,omitempty"`
	Version           string             `json:"version"`
	ID                string             `json:"id"`
	Title             string             `json:"title"`
	Category          string             `json:"category"`
	Tags              []string           `json:"tags"`
	Content           string             `json:"content"`
	Steps             []string           `json:"steps"`
	OccurredOn        string             `json:"occurred_on"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
	Attachments       []Attachment       `json:"attachments"`
	AttachmentText    string             `json:"attachment_text,omitempty"`
}
type NoteInput struct {
	LibraryRole           string            `json:"library_role,omitempty"`
	OrganizerDraftID      string            `json:"organizer_draft_id,omitempty"`
	RememberOrganizerRule bool              `json:"remember_organizer_rule,omitempty"`
	Details               map[string]string `json:"details"`
	NoteTypes             []string          `json:"note_types"`
	ExpectedVersion       string            `json:"expected_version,omitempty"`
	Title                 string            `json:"title"`
	Category              string            `json:"category"`
	Tags                  []string          `json:"tags"`
	Content               string            `json:"content"`
	Steps                 []string          `json:"steps"`
	OccurredOn            string            `json:"occurred_on"`
	AttachmentIDs         []string          `json:"attachment_ids"`
}
type Source struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	OccurredOn string `json:"occurred_on"`
}
type Message struct {
	ID        int64    `json:"id,omitempty"`
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Sources   []Source `json:"sources"`
	CreatedAt string   `json:"created_at,omitempty"`
}
type Conversation struct {
	NoteID    string `json:"note_id,omitempty"`
	ID        string `json:"id"`
	Mode      string `json:"mode"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
}
type SearchInput struct {
	NoteTypes     []string `json:"note_types,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	NoteType      string   `json:"note_type,omitempty"`
	Tag           string   `json:"tag,omitempty"`
	Query         string   `json:"query"`
	AttachmentIDs []string `json:"attachment_ids"`
	Category      string   `json:"category"`
	Limit         int      `json:"limit"`
}
type ChatInput struct {
	NoteID         string   `json:"note_id,omitempty"`
	Message        string   `json:"message"`
	Mode           string   `json:"mode"`
	ConversationID *string  `json:"conversation_id"`
	AttachmentIDs  []string `json:"attachment_ids"`
}
type Credentials struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}
type apiError struct {
	Status int
	Detail string
}

func fail(status int, detail string) { panic(apiError{status, detail}) }
func check(err error) {
	if err != nil {
		panic(err)
	}
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func jsonText(v any) string { b, err := json.Marshal(v); check(err); return string(b) }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	check(json.NewEncoder(w).Encode(v))
}
func decode(r *http.Request, v any) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			panic(err)
		}
		fail(422, "输入 JSON 格式不正确，请检查字段与内容")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		fail(422, "请求只允许一个 JSON 对象")
	}
}
func bounded(name, value string, min, max int) {
	n := utf8.RuneCountInString(value)
	if n < min || n > max {
		fail(422, name+"长度不符合要求")
	}
}
func cleanItems(items []string, max int) []string {
	if len(items) > max {
		fail(422, "项目数量过多")
	}
	result := []string{}
	for _, s := range items {
		bounded("单项", s, 0, 2000)
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
func (n *NoteInput) validate() {
	validateDetails(n.Details)
	n.Title = strings.TrimSpace(n.Title)
	if n.Category == "" {
		n.Category = "诊疗笔记"
	}
	n.Category = strings.TrimSpace(n.Category)
	bounded("标题", n.Title, 1, 160)
	bounded("分类", n.Category, 1, 40)
	bounded("正文", n.Content, 0, 50000)
	n.Tags = cleanItems(n.Tags, 20)
	n.Steps = cleanItems(n.Steps, 50)
	if len(n.AttachmentIDs) > maxNoteMaterials {
		fail(422, "最多关联 60 份材料")
	}
	if n.OccurredOn == "" {
		n.OccurredOn = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", n.OccurredOn); err != nil {
		fail(422, "日期请使用 YYYY-MM-DD 格式")
	}
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)

func (c *Credentials) validate() {
	if !usernamePattern.MatchString(c.Username) {
		fail(422, "用户名需要 3–64 位字母、数字或下划线")
	}
	bounded("密码", c.Password, 8, 128)
	if c.DisplayName == "" {
		c.DisplayName = "医生"
	}
	bounded("显示名称", c.DisplayName, 1, 40)
}

type queryer interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}
type scanner interface{ Scan(...any) error }

func (e apiError) Error() string { return e.Detail }
