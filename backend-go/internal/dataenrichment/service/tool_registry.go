package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/tracing"
)

// HTTPFetcher abstracts HTTP GET requests for testability.
type HTTPFetcher interface {
	Fetch(ctx context.Context, url string, headers map[string]string) ([]byte, error)
}

// DefaultHTTPFetcher is the production HTTP client with 5s timeout.
type DefaultHTTPFetcher struct {
	client *http.Client
}

// NewDefaultHTTPFetcher creates a fetcher with 5s timeout.
func NewDefaultHTTPFetcher() *DefaultHTTPFetcher {
	return &DefaultHTTPFetcher{
		client: httpclient.New(httpclient.WithTimeout(5 * time.Second)),
	}
}

// Fetch performs an HTTP GET with custom headers.
func (f *DefaultHTTPFetcher) Fetch(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "DefaultHTTPFetcher.Fetch")
	defer span.End()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// ── Tool definition ─────────────────────────────────────────────────────────

// Tool represents a callable data source tool.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Execute     func(ctx context.Context, args map[string]any) (string, error)
}

// Registry holds all registered data source tools.
type Registry struct {
	tools   map[string]*Tool
	fetcher HTTPFetcher

	// Exploration tools (阶段2a-ii). Each is optional; when nil the tool returns
	// a graceful error JSON. webSearcher defaults to NoopWebSearcher so the
	// web_search tool degrades cleanly until a real backend is configured.
	webSearcher             WebSearcher
	boardLister             BoardLister
	laneLister              LaneLister
	laneDetailRenderer      LaneDetailRenderer
	pageFetcher             PageFetcher             // fetch_page backend (reader readability); nil → degrade
	internalContextSearcher InternalContextSearcher // search_internal_context backend; nil → degrade
}

// RegistryOption configures optional exploration dependencies on a Registry.
// PageFetcher / webSearcher / listers are each optional; when not injected the
// corresponding tool degrades to a graceful error JSON.
type RegistryOption func(*Registry)

// WithWebSearcher injects a web search backend (default NoopWebSearcher).
func WithWebSearcher(ws WebSearcher) RegistryOption {
	return func(r *Registry) { r.webSearcher = ws }
}

// WithBoardLister injects the board-panorama data source for list_boards.
func WithBoardLister(bl BoardLister) RegistryOption {
	return func(r *Registry) { r.boardLister = bl }
}

// WithLaneLister injects the persistent-topic data source for list_lanes.
func WithLaneLister(ll LaneLister) RegistryOption {
	return func(r *Registry) { r.laneLister = ll }
}

// WithLaneDetailRenderer injects the lifeline renderer for get_lane_detail.
func WithLaneDetailRenderer(ldr LaneDetailRenderer) RegistryOption {
	return func(r *Registry) { r.laneDetailRenderer = ldr }
}

// WithPageFetcher injects the readability-backed page fetcher for fetch_page.
// When not injected, fetch_page degrades to a "not configured" error JSON.
func WithPageFetcher(pf PageFetcher) RegistryOption {
	return func(r *Registry) { r.pageFetcher = pf }
}

// WithInternalContextSearcher injects the compact internal-knowledge search
// backend for search_internal_context (add-evidence-backed-cross-board-relations).
func WithInternalContextSearcher(s InternalContextSearcher) RegistryOption {
	return func(r *Registry) { r.internalContextSearcher = s }
}

// NewRegistry creates a tool registry with the given HTTP fetcher and any
// optional exploration dependencies. Existing callers that pass only the
// fetcher keep working (exploration tools degrade to error JSON).
func NewRegistry(fetcher HTTPFetcher, opts ...RegistryOption) *Registry {
	r := &Registry{
		tools:       make(map[string]*Tool),
		fetcher:     fetcher,
		webSearcher: NoopWebSearcher{},
	}
	for _, opt := range opts {
		opt(r)
	}
	r.register()
	return r
}

// Tools returns the registered tools map (read-only).
func (r *Registry) Tools() map[string]*Tool {
	return r.tools
}

// Execute runs a tool by name with the given arguments.
// Returns JSON string on success or error JSON on failure.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) (string, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "Registry.Execute")
	defer span.End()
	tool := r.tools[name]
	if tool == nil {
		names := make([]string, 0, len(r.tools))
		for k := range r.tools {
			names = append(names, k)
		}
		errJSON, _ := json.Marshal(map[string]any{
			"error":     fmt.Sprintf("未知工具: %s", name),
			"available": names,
		})
		return string(errJSON), fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, args)
}

func (r *Registry) register() {
	// ── Exploration entry points + web_search (阶段2a-ii) ──────────────────
	// All tools are always-on; EnrichTopic always allows them for the agent so
	// it has multi-level navigation + web_search (+ fetch_page once wired)
	// regardless of board source_type. The A-share financial tools were removed
	// when the data-enrichment direction shifted to structured depth analysis.

	r.tools["list_boards"] = &Tool{
		Name:        "list_boards",
		Description: "列出全部语义版块(全景),返回 [{id,name,active_lanes 活跃泳道数}]。无参。用于从顶层选择版块。",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: r.executeListBoards,
	}

	r.tools["list_lanes"] = &Tool{
		Name:        "list_lanes",
		Description: "列出某版块下的全部持久话题泳道,返回 [{lane_id,label,status,hit_count,consecutive_hits}]。先用 list_boards 拿到 board_id。",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"board_id": map[string]any{"type": "integer"}},
			"required":   []string{"board_id"},
		},
		Execute: r.executeListLanes,
	}

	r.tools["get_lane_detail"] = &Tool{
		Name:        "get_lane_detail",
		Description: "查看某泳道(持久话题)的近期演进详情(复用 lifeline 渲染)。先用 list_lanes 拿到 lane_id。window_days 默认 14。",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"lane_id":     map[string]any{"type": "integer"},
				"window_days": map[string]any{"type": "integer"},
			},
			"required": []string{"lane_id"},
		},
		Execute: r.executeGetLaneDetail,
	}

	r.tools["web_search"] = &Tool{
		Name:        "web_search",
		Description: "网络搜索(外部知识补充),返回 [{title,url,snippet}]。当内部数据源不足时用。若未配置会返回错误,你可基于已有数据继续或跳过。",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
			"required":   []string{"query"},
		},
		Execute: r.executeWebSearch,
	}

	r.tools["fetch_page"] = &Tool{
		Name:        "fetch_page",
		Description: "抓取网页正文(readability),返回 {title,url,main_text}。用于给深度层取一手可核查原文。失败返回错误 JSON,可基于已有数据继续。",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"url": map[string]any{"type": "string"}},
			"required":   []string{"url"},
		},
		Execute: r.executeFetchPage,
	}

	r.tools["search_internal_context"] = &Tool{
		Name:        "search_internal_context",
		Description: "在系统内部知识中检索相关版块/泳道(紧凑概要,无时间线),返回 {hits:[{kind,board_id,lane_id,label,status,hit_count,summary,score}]}。命中 lane 本次调查即获临时授权,随后可用 get_lane_detail 下钻。跨版块调查优先用本工具发现相关泳道。",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":       map[string]any{"type": "string"},
				"max_results": map[string]any{"type": "integer"},
			},
			"required": []string{"query"},
		},
		Execute: r.executeSearchInternalContext,
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func jsonError(msg string) string {
	b, _ := json.Marshal(map[string]any{"error": msg})
	return string(b)
}

// toUint coerces a JSON-decoded numeric arg to uint. Handles both float64
// (the default json.Unmarshal type for numbers) and json.Number.
func toUint(v any) (uint, bool) {
	switch n := v.(type) {
	case float64:
		if n < 0 || n != float64(uint(n)) {
			return 0, false
		}
		return uint(n), true
	case int:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return uint(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil || i < 0 {
			return 0, false
		}
		return uint(i), true
	}
	return 0, false
}

// jsonMarshal is a thin wrapper over json.Marshal that never returns an error
// for the simple map[string]any payloads the tools build (it panics only on
// truly impossible inputs, which the static shapes here preclude).
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
