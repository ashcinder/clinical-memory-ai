package server

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestCuratedModelLibrary(t *testing.T) {
	a := &App{Config: Config{WebDir: filepath.Join("..", "..", "..", "前端Web")}}
	read := func(q string) anatomyCatalog {
		t.Helper()
		response := httptest.NewRecorder()
		a.modelLibrary(response, httptest.NewRequest("GET", "/api/anatomy/models?q="+url.QueryEscape(q), nil), User{})
		var catalog anatomyCatalog
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &catalog) != nil {
			t.Fatalf("unexpected catalog response: %d", response.Code)
		}
		return catalog
	}
	catalog := read("")
	if catalog.Provider != "bodyparts3d" || len(catalog.Items) == 0 || catalog.Credit == "" {
		t.Fatal("catalog source attribution missing")
	}
	for _, model := range catalog.Items {
		raw, err := os.ReadFile(filepath.Join(a.Config.WebDir, "anatomy", "library", model.Asset))
		if err != nil || len(raw) < 20 || fmt.Sprintf("%x", sha256.Sum256(raw)) != model.SHA256 {
			t.Fatalf("%s: missing or changed GLB asset", model.ID)
		}
		if string(raw[:4]) != "glTF" {
			t.Fatalf("%s: invalid GLB header", model.ID)
		}
		size := int(binary.LittleEndian.Uint32(raw[12:16]))
		if size > len(raw)-20 {
			t.Fatalf("%s: invalid GLB JSON chunk size", model.ID)
		}
		var gltf struct {
			Nodes []struct{ Name string }
		}
		if err := json.Unmarshal(raw[20:20+size], &gltf); err != nil {
			t.Fatal(err)
		}
		if len(gltf.Nodes) != len(model.Parts) {
			t.Fatalf("%s: structure count differs from model", model.ID)
		}
		for i, part := range model.Parts {
			if part.ID != gltf.Nodes[i].Name || part.Name == "" || len(part.Elements) == 0 {
				t.Fatalf("%s: invalid source mapping for %s", model.ID, part.ID)
			}
		}
	}
	if items := read("肺").Items; len(items) == 0 || items[0].ID != "airways" {
		t.Fatal("exact keyword must outrank a partial structure match")
	}
	for query, want := range map[string]string{"fma3768": "heart", "右支气管树": "airways", "髌骨": "knee", "acupuncture shop": ""} {
		items := read(query).Items
		if want == "" && len(items) != 0 || want != "" && (len(items) != 1 || items[0].ID != want) {
			t.Fatalf("unexpected search results for %q", query)
		}
	}
}
