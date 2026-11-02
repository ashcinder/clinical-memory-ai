package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Initialize once, preserving legacy catalogs and all original note ownership.
// The app-state lock serializes initialization across multiple server processes.
func (a *App) initializeSharedLibrary() error {
	tx, err := a.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id, count int
	if err = tx.QueryRow("SELECT id FROM app_state WHERE id=1 FOR UPDATE").Scan(&id); err != nil {
		return err
	}
	if err = tx.QueryRow("SELECT COUNT(*) FROM shared_library WHERE id=1").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return tx.Commit()
	}
	rows, err := tx.Query("SELECT config FROM user_taxonomy ORDER BY user_id")
	if err != nil {
		return err
	}
	catalogs := []taxonomyConfig{}
	for rows.Next() {
		var raw string
		var c taxonomyConfig
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			rows.Close()
			return err
		}
		catalogs = append(catalogs, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	c := mergeLegacyCatalogs(catalogs)
	if _, err = tx.Exec("INSERT INTO shared_library(id,revision,taxonomy) VALUES (1,0,?)", jsonText(c)); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeLegacyCatalogs(catalogs []taxonomyConfig) taxonomyConfig {
	if len(catalogs) == 0 {
		return taxonomyConfig{Types: noteTypes(), Tags: []string{}}
	}
	merged := taxonomyConfig{Types: []NoteType{}, Tags: []string{}}
	positions := map[string]int{}
	for _, c := range catalogs {
		merged.Tags = unique(append(merged.Tags, c.Tags...))
		for _, item := range c.Types {
			if i, exists := positions[item.ID]; exists {
				merged.Types[i].Aliases = unique(append(merged.Types[i].Aliases, append([]string{item.Title}, item.Aliases...)...))
			} else {
				item.Count = 0
				positions[item.ID] = len(merged.Types)
				merged.Types = append(merged.Types, item)
			}
		}
	}
	other := noteTypes()[6]
	types := []NoteType{}
	for _, item := range merged.Types {
		if item.ID == "other" {
			other = item
		} else {
			types = append(types, item)
		}
	}
	merged.Types = append(types, other)
	return merged
}

func isLibraryMutation(r *http.Request) bool {
	if r.URL.Path == "/api/anatomy" && r.Method == "PUT" {
		return true
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return false
	}
	p := r.URL.Path
	return p == "/api/taxonomy" || p == "/api/demo" || p == "/api/notes" || strings.HasPrefix(p, "/api/notes/") || p == "/api/attachments" || strings.HasPrefix(p, "/api/attachments/")
}
