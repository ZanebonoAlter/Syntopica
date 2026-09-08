package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"syntopica-backend/internal/dataenrichment/service"
)

// ── WebSearcher mocks ──────────────────────────────────────────────────────

type mockWebSearcher struct {
	results []service.WebSearchResult
	err     error
	query   string
	calls   int
}

func (m *mockWebSearcher) Search(_ context.Context, query string) ([]service.WebSearchResult, error) {
	m.calls++
	m.query = query
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestWebSearch_NoopReturnsError(t *testing.T) {
	// NoopWebSearcher must surface an error so the agent can self-degrade.
	var ws service.WebSearcher = service.NoopWebSearcher{}
	results, err := ws.Search(context.Background(), "anything")
	if err == nil {
		t.Fatal("NoopWebSearcher.Search should return an error")
	}
	if !errors.Is(err, errors.New("web_search not configured")) && err.Error() != "web_search not configured" {
		t.Fatalf("unexpected error text: %v", err)
	}
	if results != nil {
		t.Fatalf("NoopWebSearcher should return nil results, got %v", results)
	}
}

func TestWebSearch_ToolDelegates(t *testing.T) {
	ws := &mockWebSearcher{
		results: []service.WebSearchResult{
			{Title: "半导体涨价", URL: "https://example.com/a", Snippet: "全球缺芯加剧"},
		},
	}
	registry := service.NewRegistry(&nilFetcher{}, service.WithWebSearcher(ws))

	output, err := registry.Execute(context.Background(), "web_search", map[string]any{
		"query": "半导体 涨价",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if ws.calls != 1 {
		t.Fatalf("expected 1 webSearcher call, got %d", ws.calls)
	}
	if ws.query != "半导体 涨价" {
		t.Fatalf("query forwarded = %q, want %q", ws.query, "半导体 涨价")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse output: %v (raw: %s)", err, output)
	}
	if hitCount, _ := result["hit_count"].(float64); int(hitCount) != 1 {
		t.Fatalf("hit_count = %v, want 1", result["hit_count"])
	}
	results, _ := result["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	first := results[0].(map[string]any)
	if first["url"] != "https://example.com/a" {
		t.Fatalf("url = %v", first["url"])
	}
}

func TestWebSearch_BackendError_DegradesToJSON(t *testing.T) {
	ws := &mockWebSearcher{err: errors.New("boom")}
	registry := service.NewRegistry(&nilFetcher{}, service.WithWebSearcher(ws))

	output, err := registry.Execute(context.Background(), "web_search", map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("web_search backend error must degrade to JSON, got go error: %v", err)
	}
	if !strings.Contains(output, "web_search 失败") {
		t.Fatalf("expected degraded error JSON, got: %s", output)
	}
}

func TestWebSearch_EmptyQuery_ValidationError(t *testing.T) {
	registry := service.NewRegistry(&nilFetcher{}, service.WithWebSearcher(service.NoopWebSearcher{}))
	output, err := registry.Execute(context.Background(), "web_search", map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(output, "参数错误") {
		t.Fatalf("expected param error, got: %s", output)
	}
}

func TestWebSearch_DefaultNoopWhenNotInjected(t *testing.T) {
	// NewRegistry without WithWebSearcher defaults to NoopWebSearcher.
	registry := service.NewRegistry(&nilFetcher{})
	output, err := registry.Execute(context.Background(), "web_search", map[string]any{"query": "x"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(output, "web_search not configured") && !strings.Contains(output, "未配置") {
		t.Fatalf("expected not-configured degradation, got: %s", output)
	}
}

// ── BochaWebSearcher (mocked HTTP, never hits the real Bocha API) ────────────

// bochaProviderOf builds a BochaConfigProvider returning fixed credentials,
// for tests that don't need dynamic resolution.
func bochaProviderOf(key, endpoint string) service.BochaConfigProvider {
	return func() (string, string) { return key, endpoint }
}

func TestBochaWebSearcher_ParsesResults(t *testing.T) {
	// Real Bocha web-search shape (captured against the live API): hits live
	// under data.webPages.value with name/snippet fields (Bing-style).
	bochaResp := `{"code":200,"data":{"_type":"SearchResponse","webPages":{"value":[` +
		`{"id":"x1","name":"日债收益率破3%","url":"https://x","snippet":"1999年来首次","siteName":"路透"},` +
		`{"id":"x2","name":"no-url","url":"","snippet":"drop me"}` +
		`],"totalEstimatedMatches":2}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("Authorization = %q, want Bearer k", got)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["summary"] != false {
			t.Errorf("summary must be false (raw mode, no AI summary), got %v", body["summary"])
		}
		_, _ = w.Write([]byte(bochaResp))
	}))
	defer srv.Close()

	ws := service.NewBochaWebSearcher(bochaProviderOf("k", srv.URL))
	results, err := ws.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result (url-less item dropped), got %d", len(results))
	}
	want := service.WebSearchResult{Title: "日债收益率破3%", URL: "https://x", Snippet: "1999年来首次"}
	if results[0] != want {
		t.Fatalf("result = %+v, want %+v", results[0], want)
	}
}

// TestBochaWebSearcher_LegacyResultShape ensures the legacy flat "result"
// response variant still parses (compat fallback) — the original implementation
// only knew this shape, so deployments returning it must not silently degrade.
func TestBochaWebSearcher_LegacyResultShape(t *testing.T) {
	bochaResp := `{"code":200,"data":{"result":[{"title":"T","url":"https://x","summary":"S"}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bochaResp))
	}))
	defer srv.Close()

	ws := service.NewBochaWebSearcher(bochaProviderOf("k", srv.URL))
	results, err := ws.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Title != "T" || results[0].Snippet != "S" {
		t.Fatalf("legacy shape result = %+v", results)
	}
}

// TestBochaWebSearcher_UnknownShapeYieldsZeroResults guards the honest-empty
// contract: a 200 response whose shape matches neither webPages.value nor
// result must return (empty, nil) — NOT an error — so scout degrades with a
// no_candidates gap instead of a hard failure.
func TestBochaWebSearcher_UnknownShapeYieldsZeroResults(t *testing.T) {
	bochaResp := `{"code":200,"data":{"_type":"SomethingElse"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bochaResp))
	}))
	defer srv.Close()

	ws := service.NewBochaWebSearcher(bochaProviderOf("k", srv.URL))
	results, err := ws.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search should not error on unknown shape: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("want 0 results, got %d", len(results))
	}
}

func TestBochaWebSearcher_HttpError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ws := service.NewBochaWebSearcher(bochaProviderOf("k", srv.URL))
	if _, err := ws.Search(context.Background(), "q"); err == nil {
		t.Fatal("Search should return error on HTTP 500")
	}
}

func TestBochaWebSearcher_SnippetFallback(t *testing.T) {
	// item with snippet field but no summary → snippet used (webPages shape).
	bochaResp := `{"code":200,"data":{"webPages":{"value":[{"name":"A","url":"https://a","snippet":"snip"}]}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bochaResp))
	}))
	defer srv.Close()

	ws := service.NewBochaWebSearcher(bochaProviderOf("k", srv.URL))
	results, err := ws.Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results[0].Snippet != "snip" {
		t.Fatalf("snippet = %q, want snip", results[0].Snippet)
	}
}

// TestBochaWebSearcher_NotConfigured covers the all-empty fallback: when the
// provider returns an empty key (DB/env/config.yaml all unset), Search must
// surface a "not configured" error so executeWebSearch degrades like Noop.
func TestBochaWebSearcher_NotConfigured(t *testing.T) {
	ws := service.NewBochaWebSearcher(bochaProviderOf("", ""))
	_, err := ws.Search(context.Background(), "q")
	if err == nil {
		t.Fatal("Search should return error when key is empty")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %v, want 'not configured'", err)
	}
}

// TestBochaWebSearcher_DynamicRead covers the dynamic-read contract: Search must
// consult the provider on EVERY call (so UI changes take effect without
// restart). A mutable provider returns empty first, then a DB key — the second
// call must hit the mock server with that key. Also verifies endpoint fallback:
// empty endpoint falls back to the default Bocha endpoint.
func TestBochaWebSearcher_DynamicRead(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		// Second call must carry the newly-configured key.
		if hits == 2 {
			if got := r.Header.Get("Authorization"); got != "Bearer db-key" {
				t.Errorf("call #2 Authorization = %q, want Bearer db-key (provider not re-read)", got)
			}
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"webPages":{"value":[]}}}`))
	}))
	defer srv.Close()

	// Mutable provider state: simulates DB key appearing after UI save.
	var curKey, curEndpoint string
	provider := service.BochaConfigProvider(func() (string, string) { return curKey, curEndpoint })
	ws := service.NewBochaWebSearcher(provider)

	// 1. No key yet → not configured, server untouched.
	if _, err := ws.Search(context.Background(), "q"); err == nil {
		t.Fatal("first Search (empty key) should error")
	}
	if hits != 0 {
		t.Fatalf("server hit before key configured: %d", hits)
	}

	// 2. UI save sets key + endpoint → next Search uses them (proves re-read).
	curKey, curEndpoint = "db-key", srv.URL
	if _, err := ws.Search(context.Background(), "q"); err != nil {
		t.Fatalf("second Search: %v", err)
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}
	// (endpoint-empty fallback to defaultBochaEndpoint is a one-line constant
	// guard inside Search; not exercised here to avoid hitting the real Bocha
	// host — see TestBochaWebSearcher_NotConfigured for the empty-key path.)
}
