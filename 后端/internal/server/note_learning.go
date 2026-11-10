package server

import (
	"database/sql"
	"net/http"
	"strings"
)

type learningMeta struct {
	Role     string `json:"role"`
	Scope    string `json:"scope,omitempty"`
	Revision int    `json:"revision"`
	Version  int    `json:"template_version"`
	Enabled  bool   `json:"enabled"`
	Default  bool   `json:"is_default"`
}

type learningProfile struct {
	Sections []string `json:"sections"`
	Rules    string   `json:"rules"`
	Example  string   `json:"example,omitempty"`
}

type learningReference struct {
	NoteID  string `json:"note_id,omitempty"`
	Title   string `json:"title"`
	Scope   string `json:"scope"`
	Version int    `json:"version"`
	Default bool   `json:"is_default,omitempty"`
}

const libraryRoleSQL = `COALESCE((SELECT role FROM note_learning WHERE note_id=notes.id),
	CASE WHEN title LIKE '已整理｜%' AND JSON_CONTAINS(tags,JSON_QUOTE('已整理')) THEN 'reference'
	WHEN (title LIKE '原始材料｜%' AND JSON_CONTAINS(tags,JSON_QUOTE('原始材料'))) OR
		(title LIKE '原始转写｜%' AND JSON_CONTAINS(tags,JSON_QUOTE('原始转写'))) THEN 'raw'
	WHEN EXISTS (SELECT 1 FROM note_organizer_drafts WHERE note_id=notes.id) THEN 'generated'
	ELSE 'note' END)`

func loadLearningMeta(q queryer, note Note) learningMeta {
	m := learningMeta{}
	var scope string
	err := q.QueryRow(`SELECT role,scope,revision,template_version,enabled,COALESCE(default_scope,'')
		FROM note_learning WHERE note_id=?`, note.ID).Scan(&m.Role, &m.Scope, &m.Revision, &m.Version, &m.Enabled, &scope)
	if err == sql.ErrNoRows {
		check(q.QueryRow("SELECT "+libraryRoleSQL+" FROM notes WHERE id=?", note.ID).Scan(&m.Role))
		return m
	}
	check(err)
	m.Enabled = m.Enabled && m.Scope == note.NoteType
	m.Default = m.Enabled && scope == m.Scope
	return m
}

func writeLearningMeta(tx *sql.Tx, id string, m learningMeta) {
	var defaultScope any
	if m.Default && m.Enabled {
		defaultScope = m.Scope
	}
	_, err := tx.Exec(`INSERT INTO note_learning(note_id,role,scope,revision,template_version,enabled,default_scope)
		VALUES(?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE role=VALUES(role),scope=VALUES(scope),
		revision=VALUES(revision),template_version=VALUES(template_version),enabled=VALUES(enabled),default_scope=VALUES(default_scope)`,
		id, m.Role, m.Scope, m.Revision, m.Version, m.Enabled, defaultScope)
	check(err)
}

func (a *App) listLearningTemplates(w http.ResponseWriter, r *http.Request, u User) {
	rows, err := a.DB.Query(`SELECT n.id,n.title,l.scope,l.template_version,l.default_scope IS NOT NULL
		FROM note_learning l JOIN notes n ON n.id=l.note_id
		WHERE l.role='reference' AND l.enabled=1
		ORDER BY l.default_scope IS NOT NULL DESC,n.title,n.id`)
	check(err)
	defer rows.Close()
	items := []learningReference{}
	for rows.Next() {
		var item learningReference
		check(rows.Scan(&item.NoteID, &item.Title, &item.Scope, &item.Version, &item.Default))
		items = append(items, item)
	}
	check(rows.Err())
	respond(w, 200, map[string]any{"items": items})
}

func readLearningProfile(q queryer, id string, version int) (learningProfile, string, error) {
	var raw, scope string
	err := q.QueryRow("SELECT profile,scope FROM note_learning_versions WHERE note_id=? AND version=?", id, version).Scan(&raw, &scope)
	if err != nil {
		return learningProfile{}, "", err
	}
	var profile learningProfile
	return profile, scope, jsonUnmarshal(raw, &profile)
}

func suggestLearningProfile(note Note, cfg organizerConfig) learningProfile {
	baseline := defaultOrganizerTemplate(note.NoteType, cfg)
	p := learningProfile{Sections: baseline.Sections, Rules: baseline.Rules}
	if p.Rules == "" {
		p.Rules = "按章节归纳本批材料，保持原始顺序和引用。把原件放在相关段落。"
	}
	if note.NoteType != "record" {
		return p
	}
	// These are known headings, so no names, dates or case sentences enter the proposal.
	var compact []string
	for _, line := range strings.Split(note.Content, "\n") {
		name := strings.Trim(strings.TrimSpace(line), "【】：:")
		switch name {
		case "基本信息", "一、患者主述", "二、体征检查及诊疗思路":
			compact = append(compact, name)
		}
	}
	if contains(compact, "一、患者主述") && contains(compact, "二、体征检查及诊疗思路") {
		p.Sections = unique(compact)
		p.Rules = "按患者主述、体征检查及记录者的诊疗思路分段。仅写本批原件已有的事实和判断，保留否定、侧别、时间及不确定性。图像紧跟对应查体或处置，录音放在对应的就诊段落。"
	}
	return p
}

func (a *App) getLearningNote(w http.ResponseWriter, r *http.Request, u User) {
	note := ownedNote(a.DB, r.PathValue("id"), u.ID, false)
	cfg, err := a.loadOrganizerConfig()
	check(err)
	profile := suggestLearningProfile(note, cfg)
	m := note.Learning
	if m.Version > 0 {
		var scope string
		var stored learningProfile
		stored, scope, err = readLearningProfile(a.DB, note.ID, m.Version)
		check(err)
		if scope == note.NoteType {
			profile = stored
		}
	}
	rows, err := a.DB.Query(`SELECT version FROM note_learning_versions WHERE note_id=? AND scope=?
		ORDER BY version DESC LIMIT 30`, note.ID, note.NoteType)
	check(err)
	defer rows.Close()
	versions := []int{}
	for rows.Next() {
		var version int
		check(rows.Scan(&version))
		versions = append(versions, version)
	}
	check(rows.Err())
	respond(w, 200, map[string]any{"learning": m, "profile": profile, "versions": versions})
}

func (a *App) updateLearningNote(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Action   string          `json:"action"`
		Revision int             `json:"revision"`
		Profile  learningProfile `json:"profile"`
		Default  bool            `json:"is_default"`
		Restore  int             `json:"restore_version"`
	}
	decode(r, &in)
	m := withTx(a, func(tx *sql.Tx) learningMeta {
		var locked int
		check(tx.QueryRow("SELECT id FROM shared_library WHERE id=1 FOR UPDATE").Scan(&locked))
		note := ownedNote(tx, r.PathValue("id"), u.ID, true)
		meta := note.Learning
		if in.Revision != meta.Revision {
			fail(409, "资料用途或范本已修改，请重新打开")
		}
		meta.Revision++
		switch in.Action {
		case "approve", "restore":
			if strings.TrimSpace(note.Content) == "" {
				fail(422, "学习范本需要可阅读的成品正文")
			}
			profile := in.Profile
			if in.Action == "restore" {
				var scope string
				var err error
				profile, scope, err = readLearningProfile(tx, note.ID, in.Restore)
				if err == sql.ErrNoRows || (err == nil && scope != note.NoteType) {
					fail(422, "该范本版本不适用于当前资料类型")
				}
				check(err)
			}
			profile.Rules, profile.Example = strings.TrimSpace(profile.Rules), strings.TrimSpace(profile.Example)
			bounded("范本规则", profile.Rules, 1, 4000)
			bounded("脱敏短例", profile.Example, 0, 1000)
			for i, section := range profile.Sections {
				profile.Sections[i] = strings.TrimSpace(section)
			}
			if _, ok := organizerSections(profile.Sections); !ok {
				fail(422, "范本需有 1 至 30 个不重复的章节，每行一个")
			}
			if in.Default {
				_, err := tx.Exec(`UPDATE note_learning SET default_scope=NULL,revision=revision+1
					WHERE default_scope=? AND note_id<>?`, note.NoteType, note.ID)
				check(err)
			}
			meta.Version++
			_, err := tx.Exec("INSERT INTO note_learning_versions(note_id,version,scope,profile) VALUES (?,?,?,?)",
				note.ID, meta.Version, note.NoteType, jsonText(profile))
			check(err)
			meta.Role, meta.Scope, meta.Enabled, meta.Default = "reference", note.NoteType, true, in.Default
		case "reference":
			if meta.Role != "reference" {
				meta.Role, meta.Enabled, meta.Default = "reference", false, false
			}
		case "raw", "note":
			meta.Role, meta.Enabled, meta.Default = in.Action, false, false
		case "disable":
			if meta.Role != "reference" {
				fail(422, "请选择一份学习范本")
			}
			meta.Enabled, meta.Default = false, false
		default:
			fail(422, "资料用途操作无效")
		}
		writeLearningMeta(tx, note.ID, meta)
		return meta
	})
	respond(w, 200, m)
}

func (a *App) resolveLearningTemplate(ctxType, explicit string) (learningReference, learningProfile) {
	fallback := learningReference{Title: "内置通用范本", Scope: ctxType, Version: 1}
	if ctxType == "record" {
		fallback.Title = "内置诊疗记录范本"
	}
	where, args := "(l.default_scope=? OR l.default_scope='other') ORDER BY l.default_scope=? DESC LIMIT 1", []any{ctxType, ctxType}
	if explicit != "" {
		if explicit == "builtin" {
			return fallback, learningProfile{}
		}
		where, args = "l.note_id=? AND l.scope IN (?,'other')", []any{explicit, ctxType}
	}
	var ref learningReference
	var raw string
	err := a.DB.QueryRow(`SELECT n.id,n.title,l.scope,l.template_version,v.profile
		FROM note_learning l JOIN notes n ON n.id=l.note_id JOIN note_learning_versions v
		ON v.note_id=l.note_id AND v.version=l.template_version AND v.scope=l.scope
		WHERE l.enabled=1 AND l.role='reference' AND `+where, args...).
		Scan(&ref.NoteID, &ref.Title, &ref.Scope, &ref.Version, &raw)
	if err == sql.ErrNoRows {
		if explicit != "" {
			fail(422, "所选范本不可用或资料类型不匹配，请重新选择")
		}
		return fallback, learningProfile{}
	}
	check(err)
	var profile learningProfile
	check(jsonUnmarshal(raw, &profile))
	return ref, profile
}
