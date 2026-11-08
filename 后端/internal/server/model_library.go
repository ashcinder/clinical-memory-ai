package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type libraryPart struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	English   string          `json:"english"`
	Color     string          `json:"color"`
	Cavity    bool            `json:"cavity,omitempty"`
	Elements  json.RawMessage `json:"elements,omitempty"`
	Hierarchy string          `json:"hierarchy,omitempty"`
}

type libraryModel struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	English  string        `json:"english"`
	Category string        `json:"category"`
	Summary  string        `json:"summary"`
	Keywords []string      `json:"keywords"`
	Author   string        `json:"author"`
	URL      string        `json:"url"`
	License  string        `json:"license"`
	Asset    string        `json:"asset"`
	SHA256   string        `json:"sha256"`
	Parts    []libraryPart `json:"parts"`
}

type anatomyCatalog struct {
	Provider   string         `json:"provider"`
	Version    string         `json:"version"`
	Credit     string         `json:"credit"`
	LicenseURL string         `json:"license_url"`
	Items      []libraryModel `json:"items"`
}

func (m libraryModel) matchRank(q string) int {
	values := append([]string{m.Name, m.English, m.Category, m.ID}, m.Keywords...)
	for _, part := range m.Parts {
		if strings.EqualFold(part.ID, q) {
			return 2
		}
		values = append(values, part.Name, part.English)
	}
	rank := 0
	for _, value := range values {
		value = strings.ToLower(value)
		if value == q {
			return 2
		}
		if strings.Contains(value, q) {
			rank = 1
		}
	}
	return rank
}

func (a *App) modelLibrary(w http.ResponseWriter, r *http.Request, _ User) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	bounded("模型关键词", q, 0, 100)
	raw, err := os.ReadFile(filepath.Join(a.Config.WebDir, "anatomy", "library", "catalog.json"))
	var catalog anatomyCatalog
	if err != nil || json.Unmarshal(raw, &catalog) != nil || len(catalog.Items) == 0 {
		respond(w, http.StatusServiceUnavailable, map[string]string{"detail": "解剖目录暂未安装，请更新部署包后重试"})
		return
	}
	if q != "" {
		exact := make([]libraryModel, 0, len(catalog.Items))
		partial := make([]libraryModel, 0, len(catalog.Items))
		for _, model := range catalog.Items {
			switch model.matchRank(q) {
			case 2:
				exact = append(exact, model)
			case 1:
				partial = append(partial, model)
			}
		}
		catalog.Items = append(exact, partial...)
	}
	respond(w, http.StatusOK, catalog)
}
