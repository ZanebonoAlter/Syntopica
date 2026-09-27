package searxng

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/platform/httpclient"
)

func TestSearch_OK(t *testing.T) {
	var gotQuery, gotFormat, gotLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotQuery, gotFormat, gotLang = q.Get("q"), q.Get("format"), q.Get("language")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query": gotQuery,
			"results": []map[string]any{
				{"title": " 逆回购_财经百科 ", "url": " https://money.163.com/baike/x ", "content": strings.Repeat("流", MaxContentRunes+50)},
				{"title": "无 URL 项应被剔除", "url": "", "content": "orphan"},
			},
			"answers": []any{},
		})
	}))
	defer srv.Close()

	results, err := Search(context.Background(), srv.URL, "央行逆回购")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotQuery != "央行逆回购" || gotFormat != "json" || gotLang != "zh-CN" {
		t.Fatalf("query params: q=%q format=%q lang=%q", gotQuery, gotFormat, gotLang)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result (empty-URL dropped), got %d", len(results))
	}
	r := results[0]
	if r.Title != "逆回购_财经百科" || r.URL != "https://money.163.com/baike/x" {
		t.Fatalf("trim failed: %+v", r)
	}
	if got := len([]rune(r.Content)); got != MaxContentRunes {
		t.Fatalf("content runes = %d, want %d", got, MaxContentRunes)
	}
}

func TestSearch_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()

	results, err := Search(context.Background(), srv.URL, "q")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("want empty, got %d", len(results))
	}
}

func TestSearch_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := Search(context.Background(), srv.URL, "q"); err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("want status error, got %v", err)
	}
}

func TestSearch_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	defer srv.Close()

	if _, err := Search(context.Background(), srv.URL, "q"); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("want parse error, got %v", err)
	}
}

func TestSearch_Timeout(t *testing.T) {
	orig := searchHTTPClient
	searchHTTPClient = httpclient.New(httpclient.WithTimeout(50 * time.Millisecond))
	defer func() { searchHTTPClient = orig }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	if _, err := Search(context.Background(), srv.URL, "q"); err == nil {
		t.Fatalf("want timeout error, got nil")
	}
}

func TestSearch_EmptyEndpointOrQuery(t *testing.T) {
	if _, err := Search(context.Background(), "", "q"); err == nil {
		t.Fatalf("want empty-endpoint error")
	}
	if _, err := Search(context.Background(), "http://x", "  "); err == nil {
		t.Fatalf("want empty-query error")
	}
}

func TestBuildQuery(t *testing.T) {
	if got := BuildQuery("  央行 逆回购  ", 80); got != "央行 逆回购" {
		t.Fatalf("BuildQuery trim = %q", got)
	}
	long := strings.Repeat("字", 100)
	if got := BuildQuery(long, 80); len([]rune(got)) != 80 {
		t.Fatalf("BuildQuery cap = %d runes", len([]rune(got)))
	}
}
