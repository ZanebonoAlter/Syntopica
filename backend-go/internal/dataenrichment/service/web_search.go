package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"syntopica-backend/internal/platform/httpclient"
)

// WebSearchResult is a single hit from a web search.
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// WebSearcher abstracts a web search backend (Tavily/Serper/etc.).
// The agent loop calls it via the web_search tool. Implementations are injected
// at wiring time; when none is configured, NoopWebSearcher is used and the tool
// returns a graceful error JSON so the agent can self-degrade.
//
// NOTE(阶段2a-ii): no real search API is wired yet — users must configure a key
// and inject a real implementation. The default is NoopWebSearcher.
type WebSearcher interface {
	Search(ctx context.Context, query string) ([]WebSearchResult, error)
}

// NoopWebSearcher is the default WebSearcher used when no search backend is
// configured. It always returns an error, signalling the agent to degrade.
type NoopWebSearcher struct{}

// Search implements WebSearcher by returning a "not configured" error.
func (NoopWebSearcher) Search(_ context.Context, _ string) ([]WebSearchResult, error) {
	return nil, errors.New("web_search not configured")
}

// defaultBochaEndpoint is the Bocha general-search endpoint fallback used when
// the provider returns an empty endpoint (config.yaml / DB both unset).
const defaultBochaEndpoint = "https://api.bochaai.com/v1/web-search"

// BochaConfigProvider returns the current Bocha credentials on each call. The
// provider is consulted by BochaWebSearcher.Search at request time (not at
// wiring time) so that UI changes take effect immediately without a restart —
// mirroring how Firecrawl reads DB on every job. Resolution priority is
// DB(ui) > env > config.yaml > empty (empty → Search returns a
// "not configured" error so executeWebSearch degrades like NoopWebSearcher).
type BochaConfigProvider func() (apiKey, endpoint string)

// BochaWebSearcher is the Bocha (bochaai.com) general-search implementation of
// WebSearcher. It uses the raw-web-results mode (summary:false) and NEVER the
// AI-summary mode — AI summaries carry hallucination risk and are not usable as
// verifiable evidence (spec "web 搜索与正文抓取数据源"). Failures surface as an
// error so executeWebSearch degrades to an error JSON and the agent loop keeps
// running.
type BochaWebSearcher struct {
	config BochaConfigProvider
	client *http.Client
}

// NewBochaWebSearcher builds a BochaWebSearcher that reads credentials from the
// given provider on every Search call (dynamic; UI/config changes take effect
// without restart).
func NewBochaWebSearcher(provider BochaConfigProvider) *BochaWebSearcher {
	return &BochaWebSearcher{
		config: provider,
		client: httpclient.New(httpclient.WithTimeout(10 * time.Second)),
	}
}

// Search calls the Bocha general-search endpoint and maps the raw web_page
// results to WebSearchResult. Only items with a non-empty url are kept (the
// evidence_chain needs clickable URLs). summary:false forces raw results and
// disables AI summarisation.
//
// Credentials are read fresh from the provider each call. When no key is
// configured (DB/env/config.yaml all empty) it returns a "not configured"
// error so the caller can degrade to an error JSON (same semantics as
// NoopWebSearcher).
func (b *BochaWebSearcher) Search(ctx context.Context, query string) ([]WebSearchResult, error) {
	apiKey, endpoint := b.config()
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("web_search not configured")
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultBochaEndpoint
	}
	reqBody, _ := json.Marshal(map[string]any{
		"query":     query,
		"summary":   false,
		"count":     10,
		"freshness": "noLimit",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("bocha request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bocha fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bocha status %d", resp.StatusCode)
	}

	var parsed struct {
		Code int `json:"code"`
		Data struct {
			// Real Bocha shape (verified against the live API): hits live under
			// data.webPages.value and each item uses name/snippet (Bing-style).
			WebPages struct {
				Value []map[string]any `json:"value"`
			} `json:"webPages"`
			// Legacy "result" shape kept as a fallback so older deployments /
			// response variants keep parsing instead of silently degrading.
			Result []map[string]any `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("bocha decode: %w", err)
	}

	items := parsed.Data.WebPages.Value
	if len(items) == 0 {
		items = parsed.Data.Result
	}

	out := make([]WebSearchResult, 0, len(items))
	for _, item := range items {
		url, _ := item["url"].(string)
		if url == "" {
			continue // drop items without a verifiable URL
		}
		// Bocha names the hit "name" in the webPages shape; older/legacy
		// responses used "title" — accept both.
		title, _ := item["name"].(string)
		if title == "" {
			title, _ = item["title"].(string)
		}
		// Bocha fields vary; accept summary ?? snippet ?? description.
		snippet, _ := item["summary"].(string)
		if snippet == "" {
			snippet, _ = item["snippet"].(string)
		}
		if snippet == "" {
			snippet, _ = item["description"].(string)
		}
		out = append(out, WebSearchResult{Title: title, URL: url, Snippet: snippet})
	}
	return out, nil
}

// ── Tool: web_search ────────────────────────────────────────────────────────

// executeWebSearch delegates to the registry's WebSearcher and returns JSON.
// On any failure (unconfigured/backend error) it returns an error JSON string
// and a nil Go error, matching the registry's graceful-degradation convention
// for single-tool failures (the agent loop already handles these via its three
// defenses: dedup / no-truncation / thinking-off).
func (r *Registry) executeWebSearch(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return jsonError("参数错误: query 必须为非空字符串"), nil
	}
	if r.webSearcher == nil {
		return jsonError("web_search 未配置"), nil
	}
	results, err := r.webSearcher.Search(ctx, query)
	if err != nil {
		return jsonError("web_search 失败: " + err.Error()), nil
	}
	if len(results) == 0 {
		b, _ := json.Marshal(map[string]any{
			"query":     query,
			"hit_count": 0,
			"hint":      "没有命中网页，可换更具体或更宽泛的关键词重试",
			"results":   []WebSearchResult{},
		})
		return string(b), nil
	}
	b, _ := json.Marshal(map[string]any{
		"query":     query,
		"hit_count": len(results),
		"results":   results,
	})
	return string(b), nil
}
