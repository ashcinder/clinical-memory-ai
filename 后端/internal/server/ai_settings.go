package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type providerSettings struct {
	APIKey      string `json:"api_key,omitempty"`
	ChatModel   string `json:"chat_model"`
	VisionModel string `json:"vision_model"`
	ClearKey    bool   `json:"clear_key,omitempty"`
}
type userAISettings struct {
	CloudSearchEnabled *bool                       `json:"cloud_search_enabled,omitempty"`
	ExpectedVersion    string                      `json:"expected_version,omitempty"`
	ChatProvider       string                      `json:"chat_provider"`
	VisionProvider     string                      `json:"vision_provider"`
	Providers          map[string]providerSettings `json:"providers"`
	ASR                asrSettings                 `json:"asr"`
}
type asrSettings struct {
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	ClearKey bool   `json:"clear_key,omitempty"`
}

var aiProviderURLs = map[string]string{
	"deepseek": "https://api.deepseek.com", "glm": "https://open.bigmodel.cn/api/paas/v4",
	"kimi": "https://api.moonshot.cn/v1", "minimax": "https://api.minimax.cn/v1",
}

func (a *App) defaultAISettings() userAISettings {
	s := userAISettings{ChatProvider: a.Config.AIProvider, VisionProvider: a.Config.VisionProvider, Providers: map[string]providerSettings{
		"deepseek": {ChatModel: "deepseek-flash", VisionModel: "deepseek-flash"},
		"glm":      {ChatModel: "glm-4.5", VisionModel: "glm-4.6v"},
		"kimi":     {ChatModel: "kimi-k2.5", VisionModel: "kimi-k2.5"},
		"minimax":  {ChatModel: "MiniMax-M3.1-Flash", VisionModel: "MiniMax-M3.1-Flash"},
	}}
	s.ASR = asrSettings{APIKey: a.Config.ASRKey, BaseURL: a.Config.ASRURL, Model: a.Config.ASRModel}
	if s.ASR.Model == "" {
		s.ASR.Model = "whisper-1"
	}
	if _, ok := s.Providers[s.ChatProvider]; ok {
		p := s.Providers[s.ChatProvider]
		p.APIKey = a.Config.DeepSeekKey
		if a.Config.DeepSeekModel != "" {
			p.ChatModel = a.Config.DeepSeekModel
		}
		s.Providers[s.ChatProvider] = p
	} else if s.ChatProvider != "none" {
		s.ChatProvider = "deepseek"
	}
	if _, ok := s.Providers[s.VisionProvider]; ok {
		p := s.Providers[s.VisionProvider]
		if p.APIKey == "" {
			p.APIKey = a.Config.VisionKey
		}
		if a.Config.VisionModel != "" {
			p.VisionModel = a.Config.VisionModel
		}
		s.Providers[s.VisionProvider] = p
	} else if s.VisionProvider != "none" {
		s.VisionProvider = "none"
	}
	return s
}
func (a *App) aiSettingsPath(userID int64) string {
	return filepath.Join(a.Config.DataDir, "ai-settings", strconv.FormatInt(userID, 10)+".json")
}
func (a *App) loadAISettings(userID int64) (userAISettings, error) {
	s := a.defaultAISettings()
	b, err := os.ReadFile(a.aiSettingsPath(userID))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return s, nil
}

// Endpoint routing stays under server administration even after account settings
// are saved. Intranet gateways must not silently revert to public provider URLs.
func (a *App) providerURL(provider string, vision bool) string {
	if vision && provider == a.Config.VisionProvider && a.Config.VisionURL != "" {
		return a.Config.VisionURL
	}
	if provider == a.Config.AIProvider && a.Config.DeepSeekURL != "" {
		return a.Config.DeepSeekURL
	}
	if base := a.Config.ProviderURLs[provider]; base != "" {
		return base
	}
	return aiProviderURLs[provider]
}

func (a *App) runtimeAIConfig(s userAISettings) Config {
	c := a.Config
	if s.CloudSearchEnabled != nil {
		c.CloudSearchDisabled = !*s.CloudSearchEnabled
	}
	c.AIProvider = s.ChatProvider
	c.VisionProvider = s.VisionProvider
	c.DeepSeekKey = ""
	c.DeepSeekURL = ""
	c.DeepSeekModel = ""
	c.VisionKey = ""
	c.VisionURL = ""
	c.VisionModel = ""
	c.ASRKey = s.ASR.APIKey
	c.ASRURL = s.ASR.BaseURL
	c.ASRModel = s.ASR.Model
	if p, ok := s.Providers[s.ChatProvider]; ok {
		c.DeepSeekKey = p.APIKey
		c.DeepSeekURL = a.providerURL(s.ChatProvider, false)
		c.DeepSeekModel = p.ChatModel
	}
	if p, ok := s.Providers[s.VisionProvider]; ok {
		c.VisionKey = p.APIKey
		c.VisionURL = a.providerURL(s.VisionProvider, true)
		c.VisionModel = p.VisionModel
	}
	return c
}

type aiConfigContextKey struct{}

func (a *App) aiConfig(ctx context.Context) Config {
	if c, ok := ctx.Value(aiConfigContextKey{}).(Config); ok {
		return c
	}
	return a.Config
}
func (a *App) publicAISettings(s userAISettings) map[string]any {
	providers := map[string]any{}
	for name, p := range s.Providers {
		providers[name] = map[string]any{"has_key": p.APIKey != "", "chat_model": p.ChatModel, "vision_model": p.VisionModel}
	}
	return map[string]any{"cloud_search_enabled": s.CloudSearchEnabled == nil || *s.CloudSearchEnabled, "version": digest(jsonText(s)), "chat_provider": s.ChatProvider, "vision_provider": s.VisionProvider, "providers": providers, "asr": map[string]any{"has_key": s.ASR.APIKey != "", "base_url": s.ASR.BaseURL, "model": s.ASR.Model}}
}
func (a *App) getAISettings(w http.ResponseWriter, r *http.Request, u User) {
	s, err := a.loadAISettings(u.ID)
	if err != nil {
		fail(500, "无法读取模型设置，请检查后端数据目录")
	}
	respond(w, 200, a.publicAISettings(s))
}
func (a *App) putAISettings(w http.ResponseWriter, r *http.Request, u User) {
	var in userAISettings
	decode(r, &in)
	validProvider := func(v string) bool { _, ok := aiProviderURLs[v]; return ok || v == "none" }
	if !validProvider(in.ChatProvider) || !validProvider(in.VisionProvider) {
		fail(422, "请选择有效的模型服务")
	}
	a.aiSettingsMu.Lock()
	defer a.aiSettingsMu.Unlock()
	s, err := a.loadAISettings(u.ID)
	if err != nil {
		fail(500, "无法读取模型设置，请检查后端数据目录")
	}
	if in.ExpectedVersion != "" && in.ExpectedVersion != digest(jsonText(s)) {
		fail(409, "模型设置已在其他设备更新，请重新载入后修改")
	}
	if in.CloudSearchEnabled != nil {
		s.CloudSearchEnabled = in.CloudSearchEnabled
	}
	s.ChatProvider = in.ChatProvider
	s.VisionProvider = in.VisionProvider
	for name, v := range in.Providers {
		old, ok := s.Providers[name]
		if !ok {
			fail(422, "未知模型服务")
		}
		v.APIKey = strings.TrimSpace(v.APIKey)
		v.ChatModel = strings.TrimSpace(v.ChatModel)
		v.VisionModel = strings.TrimSpace(v.VisionModel)
		if len(v.APIKey) > 4096 || strings.ContainsAny(v.APIKey, "\r\n") {
			fail(422, "API 密钥格式无效")
		}
		if len(v.ChatModel) > 200 || len(v.VisionModel) > 200 || strings.ContainsAny(v.ChatModel+v.VisionModel, "\r\n") {
			fail(422, "模型名称格式无效")
		}
		if v.ClearKey && v.APIKey != "" {
			fail(422, "不能同时替换和清除密钥")
		}
		if v.ClearKey {
			old.APIKey = ""
		} else if v.APIKey != "" {
			old.APIKey = v.APIKey
		}
		if v.ChatModel != "" {
			old.ChatModel = v.ChatModel
		}
		if v.VisionModel != "" {
			old.VisionModel = v.VisionModel
		}
		s.Providers[name] = old
	}
	if in.ASR.ClearKey && in.ASR.APIKey != "" {
		fail(422, "不能同时替换和清除音频服务密钥")
	}
	if len(in.ASR.APIKey) > 4096 || strings.ContainsAny(in.ASR.APIKey, "\r\n") || len(in.ASR.Model) > 200 || strings.ContainsAny(in.ASR.Model, "\r\n") {
		fail(422, "音频服务配置格式无效")
	}
	if in.ASR.ClearKey {
		s.ASR.APIKey = ""
	} else if strings.TrimSpace(in.ASR.APIKey) != "" {
		s.ASR.APIKey = strings.TrimSpace(in.ASR.APIKey)
	}
	if in.ASR.Model != "" {
		s.ASR.Model = strings.TrimSpace(in.ASR.Model)
	}
	if in.ASR.BaseURL != "" {
		u, parseErr := url.Parse(in.ASR.BaseURL)
		if parseErr != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
			fail(422, "音频服务地址需为 HTTPS 基础地址（本机可使用 HTTP）")
		}
		s.ASR.BaseURL = strings.TrimRight(u.String(), "/")
	}
	a.saveAISettings(u.ID, s)
	respond(w, 200, a.publicAISettings(s))
}
func (a *App) saveAISettings(userID int64, s userAISettings) {
	dir := filepath.Dir(a.aiSettingsPath(userID))
	if err := os.MkdirAll(dir, 0700); err != nil {
		fail(500, "无法保存模型设置")
	}
	tmp, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		fail(500, "无法保存模型设置")
	}
	name := tmp.Name()
	defer os.Remove(name)
	err = tmp.Chmod(0600)
	if err == nil {
		err = json.NewEncoder(tmp).Encode(s)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, a.aiSettingsPath(userID))
	}
	if err != nil {
		fail(500, "无法保存模型设置")
	}
}
func (a *App) aiSettingsForRequest(r *http.Request, u User) *http.Request {
	if _, err := os.Stat(a.aiSettingsPath(u.ID)); os.IsNotExist(err) {
		return r
	}
	s, err := a.loadAISettings(u.ID)
	if err != nil {
		panic(apiError{500, "无法读取模型设置，请检查后端数据目录"})
	}
	return r.WithContext(context.WithValue(r.Context(), aiConfigContextKey{}, a.runtimeAIConfig(s)))
}

func (a *App) putSearchSettings(w http.ResponseWriter, r *http.Request, u User) {
	var in struct {
		Enabled         *bool  `json:"cloud_search_enabled"`
		ExpectedVersion string `json:"expected_version,omitempty"`
	}
	decode(r, &in)
	if in.Enabled == nil {
		fail(422, "请选择是否启用云端 AI 搜索")
	}
	a.aiSettingsMu.Lock()
	defer a.aiSettingsMu.Unlock()
	s, err := a.loadAISettings(u.ID)
	check(err)
	if in.ExpectedVersion != "" && in.ExpectedVersion != digest(jsonText(s)) {
		fail(409, "搜索设置已在其他设备更新，请重试")
	}
	s.CloudSearchEnabled = in.Enabled
	a.saveAISettings(u.ID, s)
	respond(w, 200, a.publicAISettings(s))
}
