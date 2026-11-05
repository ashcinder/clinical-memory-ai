package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type libraryModel struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	License string `json:"license"`
}
type sketchModel struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
	User struct {
		DisplayName string `json:"displayName"`
	} `json:"user"`
	License *struct {
		Label string `json:"label"`
	} `json:"license"`
}

var modelIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
var libraryCache = struct {
	sync.Mutex
	data map[string]struct {
		items   []libraryModel
		expires time.Time
	}
}{data: make(map[string]struct {
	items   []libraryModel
	expires time.Time
})}
var modelLibraryHTTP HTTPDoer = &http.Client{Timeout: 12 * time.Second}

func readModelLibrary(ctx context.Context, q string, client HTTPDoer) ([]libraryModel, error) {
	endpoint := "https://api.sketchfab.com/v3/search?type=models&count=12&q=" + url.QueryEscape(q)
	if q == "" {
		endpoint = "https://api.sketchfab.com/v3/models/b018e3f215c14be6ab5c52e5371c3ba5"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClinicalMemory-ModelLibrary/1.0")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("provider status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	rows := []sketchModel{}
	if q == "" {
		var m sketchModel
		if err = json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		rows = append(rows, m)
	} else {
		var data struct {
			Results []sketchModel `json:"results"`
		}
		if err = json.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		rows = data.Results
	}
	result := []libraryModel{}
	for _, m := range rows {
		if !modelIDPattern.MatchString(m.UID) {
			continue
		}
		license := "按原站嵌入规则展示"
		if m.License != nil && m.License.Label != "" {
			license = m.License.Label
		}
		result = append(result, libraryModel{m.UID, m.Name, m.User.DisplayName, "https://sketchfab.com/3d-models/" + m.UID, license})
	}
	return result, nil
}
func (a *App) modelLibrary(w http.ResponseWriter, r *http.Request, u User) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	bounded("模型关键词", q, 0, 100)
	translations := map[string]string{"心脏": "heart anatomy", "头部": "head anatomy", "头颈": "head anatomy", "骨骼": "skeleton anatomy", "肌肉": "muscle anatomy", "穴位": "acupuncture", "经络": "acupuncture", "神经": "nervous system anatomy", "人体": "human anatomy"}
	if v, ok := translations[q]; ok {
		q = v
	}
	libraryCache.Lock()
	cached, ok := libraryCache.data[q]
	libraryCache.Unlock()
	if ok && time.Now().Before(cached.expires) {
		respond(w, 200, map[string]any{"provider": "sketchfab", "items": cached.items})
		return
	}
	items, err := readModelLibrary(r.Context(), q, modelLibraryHTTP)
	if err != nil {
		respond(w, 502, map[string]string{"detail": "暂时无法连接模型库，请重试；在线模型需要互联网连接"})
		return
	}
	libraryCache.Lock()
	if len(libraryCache.data) > 100 {
		libraryCache.data = make(map[string]struct {
			items   []libraryModel
			expires time.Time
		})
	}
	libraryCache.data[q] = struct {
		items   []libraryModel
		expires time.Time
	}{items, time.Now().Add(5 * time.Minute)}
	libraryCache.Unlock()
	respond(w, 200, map[string]any{"provider": "sketchfab", "items": items})
}
