// Package searxng is a minimal client for a local SearXNG instance's JSON API.
//
// SearXNG aggregates raw web results from multiple engines and has no
// AI-summary mode, which aligns with the data-enrichment red line 10
// 「web 搜索只用原始网页结果，禁 AI 总结」. The first consumer is the daily
// report margin-note QA (design D7); future consumers may adapt it to the
// dataenrichment WebSearcher interface via a thin adapter.
//
// The client is intentionally dependency-free: it lives in platform (not
// dataenrichment) because dataenrichment/service reserves an import direction
// toward topicgraph — a topicgraph → dataenrichment dependency would risk a
// cycle. Endpoints are passed per call so config changes take effect
// immediately (same semantics as bocha_config's read-on-every-Search).
package searxng

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"syntopica-backend/internal/platform/httpclient"
)

const (
	// DefaultTimeout bounds one search round-trip. Multi-engine aggregation on
	// a local instance measures 1~5.2s depending on query newsiness (2026-09-25
	// 实测：「外交部否认…」类新闻词 5.2s，短查询 2.8s；首次上线 5s 实跑超界后上调)；
	// 8s leaves headroom while keeping QA latency bounded. Failures degrade
	// silently, so the bound only matters for the happy path.
	DefaultTimeout = 8 * time.Second
	// MaxContentRunes caps each result snippet fed into prompts (design D7:
	// top 5 × ≤200 runes keeps the extra prompt block bounded).
	MaxContentRunes = 200
)

// Result is one raw web hit (title + clickable URL + snippet). Snippets are
// reference material for LLM prompts only — they are never rendered as
// evidence quotes.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// searchHTTPClient is overridable in tests (same-package tests swap the
// timeout to exercise the deadline path without real waiting).
var searchHTTPClient = httpclient.New(httpclient.WithTimeout(DefaultTimeout))

// Search queries the SearXNG JSON API at endpoint (e.g. http://localhost:8889)
// and returns raw results with non-empty URLs, preserving engine order.
// content snippets are truncated to MaxContentRunes. A non-2xx status, bad
// JSON, or unreachable instance returns an error — callers are expected to
// degrade silently (fail-open without the web context).
func Search(ctx context.Context, endpoint, query string) ([]Result, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	query = strings.TrimSpace(query)
	if endpoint == "" {
		return nil, fmt.Errorf("searxng: empty endpoint")
	}
	if query == "" {
		return nil, fmt.Errorf("searxng: empty query")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/search", nil)
	if err != nil {
		return nil, fmt.Errorf("searxng request: %w", err)
	}
	q := req.URL.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("language", "zh-CN")
	req.URL.RawQuery = q.Encode()

	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("searxng status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("searxng read body: %w", err)
	}

	var parsed struct {
		Results []Result `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("searxng parse: %w", err)
	}

	out := make([]Result, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		r.Title = strings.TrimSpace(r.Title)
		r.URL = strings.TrimSpace(r.URL)
		r.Content = truncateRunes(strings.TrimSpace(r.Content), MaxContentRunes)
		if r.URL == "" {
			continue // evidence must stay clickable
		}
		out = append(out, r)
	}
	return out, nil
}

// BuildQuery trims the search query and caps it at maxRunes runes (design D7:
// quoted_text 截前 80 runes，长划词直接搜效果差).
func BuildQuery(raw string, maxRunes int) string {
	return truncateRunes(strings.TrimSpace(raw), maxRunes)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
