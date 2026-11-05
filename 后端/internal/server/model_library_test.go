package server

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type libraryDoer func(*http.Request) (*http.Response, error)

func (f libraryDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }
func TestModelLibraryProviderParsing(t *testing.T) {
	client := libraryDoer(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.sketchfab.com" || r.URL.Query().Get("q") != "heart anatomy" {
			t.Fatalf("unexpected provider request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"results":[{"uid":"b018e3f215c14be6ab5c52e5371c3ba5","name":"Head","user":{"displayName":"Author"},"license":{"label":"CC BY"}},{"uid":"https://untrusted.test/model","name":"invalid"}]}`))}, nil
	})
	items, err := readModelLibrary(context.Background(), "heart anatomy", client)
	if err != nil || len(items) != 1 || items[0].Author != "Author" || items[0].License != "CC BY" {
		t.Fatalf("%+v %v", items, err)
	}
	failing := libraryDoer(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
	})
	if _, err := readModelLibrary(context.Background(), "heart", failing); err == nil {
		t.Fatal("provider failure must be reported")
	}
}
