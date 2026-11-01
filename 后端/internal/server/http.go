package server

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

const maxUpload int64 = 20 << 20

func Headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			// Revalidate mutable frontend files so HTML, CSS and JavaScript stay in sync.
			if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") || strings.HasSuffix(r.URL.Path, ".css") || strings.HasSuffix(r.URL.Path, ".js") {
				w.Header().Set("Cache-Control", "no-cache")
			}
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' blob: data:; media-src 'self' blob:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && r.Header.Get("Origin") != "" {
			origin, err := url.Parse(r.Header.Get("Origin"))
			if err != nil || origin.Host != r.Host {
				respond(w, 403, map[string]string{"detail": "不允许跨站请求"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				switch err := value.(type) {
				case apiError:
					respond(w, err.Status, map[string]string{"detail": err.Detail})
				default:
					var large *http.MaxBytesError
					if e, ok := value.(error); ok && errors.As(e, &large) {
						respond(w, 413, map[string]string{"detail": "单份文件不能超过 20MB"})
					} else {
						log.Printf("请求处理失败 %s %s: %v", r.Method, r.URL.Path, value)
						respond(w, 500, map[string]string{"detail": "服务处理失败，请查看后端日志后重试"})
					}
				}
			}
		}()
		if r.ContentLength > maxUpload+(1<<20) {
			fail(413, "请求过大，单份文件限 20MB")
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
		next.ServeHTTP(w, r)
	})
}

type authed func(http.ResponseWriter, *http.Request, User)

func (a *App) guard(fn authed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := a.currentUser(r)
		sw := &syncResponseWriter{ResponseWriter: w}
		fn(sw, a.aiSettingsForRequest(r, u), u)
		if sw.status >= 200 && sw.status < 300 && isLibraryMutation(r) {
			if _, err := a.DB.Exec("UPDATE shared_library SET revision=revision+1 WHERE id=1"); err != nil {
				log.Print("共享资料同步版本更新失败，请检查数据库连接")
			}
		}
		if sw.status >= 200 && sw.status < 300 && isSyncedMutation(r) {
			if _, err := a.DB.Exec("INSERT INTO user_sync(user_id,revision) VALUES (?,1) ON DUPLICATE KEY UPDATE revision=revision+1", u.ID); err != nil {
				log.Print("同步版本更新失败，请检查数据库连接")
			}
		}
	}
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("POST /api/auth/setup", a.setup)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("GET /api/auth/me", a.guard(func(w http.ResponseWriter, r *http.Request, u User) { respond(w, 200, u) }))
	mux.HandleFunc("POST /api/auth/logout", a.guard(func(w http.ResponseWriter, r *http.Request, u User) {
		_, err := a.DB.Exec("DELETE FROM sessions WHERE token_hash=?", digest(requestToken(r)))
		check(err)
		http.SetCookie(w, &http.Cookie{Name: "linji_session", Path: a.cookiePath(), MaxAge: -1, HttpOnly: true, Secure: a.Config.CookieSecure, SameSite: http.SameSiteStrictMode})
		respond(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("PUT /api/auth/me", a.guard(a.updateProfile))
	mux.HandleFunc("GET /api/sync", a.guard(a.syncState))
	mux.HandleFunc("GET /api/settings", a.guard(a.settings))
	mux.HandleFunc("PUT /api/search-settings", a.guard(a.putSearchSettings))
	mux.HandleFunc("GET /api/ai-config", a.guard(a.getAISettings))
	mux.HandleFunc("PUT /api/ai-config", a.guard(a.putAISettings))
	mux.HandleFunc("GET /api/stats", a.guard(a.stats))
	mux.HandleFunc("GET /api/taxonomy", a.guard(a.taxonomy))
	mux.HandleFunc("POST /api/taxonomy", a.guard(a.mutateTaxonomy))
	mux.HandleFunc("GET /api/notes", a.guard(a.listNotes))
	mux.HandleFunc("POST /api/notes", a.guard(a.createNote))
	mux.HandleFunc("GET /api/notes/{id}", a.guard(a.getNote))
	mux.HandleFunc("PUT /api/notes/{id}", a.guard(a.updateNote))
	mux.HandleFunc("DELETE /api/notes/{id}", a.guard(a.deleteNote))
	mux.HandleFunc("GET /api/attachments", a.guard(a.listAttachments))
	mux.HandleFunc("POST /api/attachments", a.guard(a.upload))
	mux.HandleFunc("GET /api/attachments/{id}/file", a.guard(a.attachmentFile))
	mux.HandleFunc("GET /api/attachments/{id}/thumbnail", a.guard(a.attachmentThumbnail))
	mux.HandleFunc("PUT /api/attachments/{id}/text", a.guard(a.editExtraction))
	mux.HandleFunc("POST /api/attachments/{id}/extract", a.guard(a.retryExtraction))
	mux.HandleFunc("DELETE /api/attachments/{id}", a.guard(a.deleteAttachment))
	mux.HandleFunc("POST /api/search", a.guard(a.search))
	mux.HandleFunc("POST /api/chat", a.guard(a.chat))
	mux.HandleFunc("GET /api/conversations", a.guard(a.conversations))
	mux.HandleFunc("GET /api/conversations/{id}", a.guard(a.conversation))
	mux.HandleFunc("DELETE /api/conversations/{id}", a.guard(a.deleteConversation))
	mux.HandleFunc("GET /api/export", a.guard(a.export))
	mux.HandleFunc("POST /api/demo", a.guard(a.demo))
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(apiDocs))
	})
	mux.Handle("/", http.FileServer(http.Dir(a.Config.WebDir)))
	return WithBasePath(Headers(a.recover(mux)), a.Config.BasePath)
}
func (a *App) health(w http.ResponseWriter, r *http.Request) {
	var count int
	check(a.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count))
	respond(w, 200, map[string]any{"status": "ok", "initialized": count > 0, "database": "mysql", "backend": "go"})
}
func (a *App) settings(w http.ResponseWriter, r *http.Request, u User) {
	c := a.aiConfig(r.Context())
	_, pdfErr := exec.LookPath("pdftotext")
	respond(w, 200, map[string]any{"cloud_search_enabled": !c.CloudSearchDisabled, "local_asr": a.localASRReady(), "local_ocr_languages": a.localOCRReady(), "image_fingerprint": true, "ai_enabled": c.DeepSeekKey != "", "ai_provider": c.AIProvider, "ai_model": c.DeepSeekModel, "deepseek_enabled": c.DeepSeekKey != "", "deepseek_model": c.DeepSeekModel, "vision_provider": c.VisionProvider, "vision_model": c.VisionModel, "vision_enabled": c.VisionKey != "", "asr_enabled": c.ASRKey != "" && c.ASRURL != "", "local_ocr": a.localOCRReady(), "pdf_extraction": pdfErr == nil, "max_upload_mb": 20, "database": "MySQL", "backend": "Go", "external_data_notice": "启用云端 AI 后，检索内容、命中笔记片段或上传材料将发送到配置的第三方服务，产生费用；请先脱敏。"})
}
func intParam(r *http.Request, key string, defaultValue, minValue, maxValue int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue || value > maxValue {
		fail(422, key+" 参数无效")
	}
	return value
}

const apiDocs = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><title>临记 Go API</title><h1>临记 Go + MySQL API</h1><p>Web 使用 HttpOnly Cookie，Android 使用 Authorization: Bearer TOKEN。JSON 错误格式为 {"detail":"说明"}。</p><pre>
GET  /api/health                         服务状态与初始化状态
POST /api/auth/setup                    创建首个账户 {username,password,display_name}
POST /api/auth/login                    登录 {username,password}
POST /api/auth/logout                   退出
GET  /api/auth/me                       当前账户
GET  /api/notes?limit=100&amp;offset=0        笔记列表
POST /api/notes                         创建 {title,category,tags,content,steps,occurred_on,attachment_ids}
GET/PUT/DELETE /api/notes/{id}           查阅、编辑、删除笔记
GET/POST /api/attachments               材料列表、上传 multipart file（20MB）
POST /api/taxonomy                     管理资料类型与标签
GET /api/attachments/{id}/file           鉴权原件
PUT /api/attachments/{id}/text           修正识别文本 {text}
POST /api/attachments/{id}/extract       重新提取
DELETE /api/attachments/{id}             删除材料
PUT /api/search-settings               同步云端搜索开关 {cloud_search_enabled}
POST /api/search                        图片指纹与文字检索 {query,attachment_ids,category,limit}
POST /api/chat                          对话 {message,mode,conversation_id,attachment_ids}
GET /api/conversations                  对话列表
GET/DELETE /api/conversations/{id}       对话历史、删除对话
GET /api/stats                          资料统计
GET /api/settings                       服务能力（不返回密钥）
GET /api/export                         导出笔记 JSON
POST /api/demo                          空库导入虚构示例
</pre><p>mode: knowledge 或 general。occurred_on: YYYY-MM-DD。完整启动说明请看项目 README。</p></html>`

func (a *App) cookiePath() string { return a.Config.BasePath + "/" }

// Keep every project's API and session cookie within its own URL namespace.
func WithBasePath(handler http.Handler, base string) http.Handler {
	if base == "" {
		return handler
	}
	mux := http.NewServeMux()
	mux.Handle(base+"/", http.StripPrefix(base, handler))
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, base+"/", http.StatusPermanentRedirect)
	})
	return mux
}
