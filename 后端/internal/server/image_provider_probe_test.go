package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Explicit quota probe; on unavailable balance, attempts a minimal completion
// to verify the upstream status. Never prints keys or monetary balance amounts.
func TestImageProviderAvailability(t *testing.T) {
	if os.Getenv("LINJI_RUN_IMAGE_PROBE") != "1" {
		t.Skip("explicit quota probe only")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	owner := &App{Config: cfg}
	s, err := owner.loadAISettings(1)
	if err != nil {
		t.Fatal(err)
	}
	c := owner.runtimeAIConfig(s)
	endpoint, parseErr := url.Parse(c.VisionURL)
	if parseErr != nil || c.VisionProvider != "deepseek" || endpoint.Hostname() != "api.deepseek.com" {
		t.Skip("DeepSeek quota endpoint only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.deepseek.com/user/balance", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.VisionKey)
	response, err := newAIClient().Do(req)
	if err != nil {
		t.Fatal("quota probe request failed")
	}
	defer response.Body.Close()
	var value struct {
		Available bool `json:"is_available"`
		Error     struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err = json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal("invalid quota response")
	}
	completionStatus := 0
	if response.StatusCode == 200 && !value.Available {
		payload := jsonText(map[string]any{"model": c.VisionModel, "messages": []map[string]string{{"role": "user", "content": "1"}}, "max_tokens": 1, "stream": false, "thinking": map[string]string{"type": "disabled"}})
		checkRequest, e := http.NewRequestWithContext(ctx, "POST", c.VisionURL+"/chat/completions", strings.NewReader(payload))
		if e == nil {
			checkRequest.Header.Set("Authorization", "Bearer "+c.VisionKey)
			checkRequest.Header.Set("Content-Type", "application/json")
			checkResponse, e := newAIClient().Do(checkRequest)
			if e == nil {
				completionStatus = checkResponse.StatusCode
				checkResponse.Body.Close()
			}
		}
	}
	result := map[string]any{"completion_http_status": completionStatus, "http_status": response.StatusCode, "is_available": value.Available, "error_type": value.Error.Type, "model": c.VisionModel, "timestamp": now()}
	raw, _ := json.MarshalIndent(result, "", "  ")
	if dir := os.Getenv("LINJI_IMAGE_SCALE_DIR"); dir != "" {
		if err = os.WriteFile(filepath.Join(dir, "provider-availability.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("provider http_status=%d is_available=%v", response.StatusCode, value.Available)
}
