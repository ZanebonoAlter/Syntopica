package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/jsonutil"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/searxng"
)

// ── 页边注问答联网后端（daily-report-margin-notes design D7；margin-note-
// llm-search-query 翻案 D7 搜索策略：query 由 LLM 自由生成，替代锚文本机械截
// 断）──
//
// 触发策略：每次必搜 + 失败静默降级——搜索失败/超时/未配置只影响是否携带联网
// 参考块，绝不阻断问答（对齐 D6 失败隔离精神）。客户端在
// internal/platform/searxng（独立通用包，避免 topicgraph → dataenrichment
// 循环引用）；配置 searxng_config 对齐 bocha_config 语义：
// DB(界面) > env(SEARXNG_URL，config 加载时折入 AppConfig) > config.yaml >
// 空 = 禁用；每次提问现读，界面改即时生效。
//
// 搜索词来源（本 change）：前置一次轻量 LLM 调用（marginNoteQueryGenFn，
// operation daily_report.margin_note_search_query）自由生成 1~3 条中文搜索
// 词；生成失败/超时/输出空 → 静默回退锚文本 query（MarginNoteSearchQuery 现状
// 规则）单 query 搜索。

// errMarginNoteSearchDisabled signals "SearXNG disabled/unconfigured" — the
// QA flow treats it as the silent-degrade path (Warn log, no web context).
var errMarginNoteSearchDisabled = errors.New("searxng not configured")

// marginNoteSearxngQueryRunes caps the search query (design D7: quoted_text
// 截前 80 runes，为空回退问题文本).
const marginNoteSearxngQueryRunes = 80

// marginNoteSearxngResultLimit is how many raw results feed the prompt.
const marginNoteSearxngResultLimit = 5

// marginNoteWebSearchFn is the swappable web-search hook (marginNoteChatFn
// precedent): tests replace it with a stub; the default resolves config on
// every call (UI changes take effect without restart).
var marginNoteWebSearchFn = func(ctx context.Context, query string) ([]searxng.Result, error) {
	endpoint, ok := marginNoteSearxngEndpoint()
	if !ok {
		return nil, errMarginNoteSearchDisabled
	}
	return searxng.Search(ctx, endpoint, query)
}

// MarginNoteSearchQuery derives the SearXNG query for one question: the
// anchored text first (capped at 80 runes — long selections search poorly),
// falling back to the question itself when the anchor is blank.
func MarginNoteSearchQuery(quotedText, question string) string {
	if q := searxng.BuildQuery(quotedText, marginNoteSearxngQueryRunes); q != "" {
		return q
	}
	return searxng.BuildQuery(question, marginNoteSearxngQueryRunes)
}

// marginNoteSearxngEndpoint resolves the current SearXNG endpoint. ok=false →
// disabled (DB absent-or-disabled AND env/config.yaml empty)。实现委托
// searxng.ResolveConfiguredEndpoint（单一事实源：与本包探测下沉的设置面板
// 健康探测同一解析路径，界面改两侧同时生效）。
func marginNoteSearxngEndpoint() (string, bool) {
	return searxng.ResolveConfiguredEndpoint()
}

// marginNoteSearxngEndpointFn is the swappable endpoint precheck hook (D3:
// AskMarginNote 发起 query 生成之前先预检，未配置/禁用跳过整个联网链路)。测
// 试替换它控制「已配置/未配置」两态（默认钩子读 aisettings 全局 DB，本包单测
// 未设会空指针，先例见 swapMarginNoteChat 注释）。
var marginNoteSearxngEndpointFn = marginNoteSearxngEndpoint

// ── LLM 自由选词搜索（margin-note-llm-search-query design D1~D8）──────────

// MarginNoteQueryGenOperation is the audit operation for the search-query
// generation call（与回答调用 MarginNoteQAOperation 区分，效果可单独回放）。
const MarginNoteQueryGenOperation = "daily_report.margin_note_search_query"

const (
	// marginNoteQueryGenMaxTokens caps the query-generation output (D1:
	// 输出仅 1~3 个查询词，压缩成本)。
	marginNoteQueryGenMaxTokens = 128
	// marginNoteQueryGenTemperature favors deterministic short outputs (D1).
	marginNoteQueryGenTemperature = 0.1
	// marginNoteQueryMaxCount caps generated queries（超过截前 N，D1）。
	marginNoteQueryMaxCount = 3
	// marginNoteQueryRunes caps each generated query（单条超长截 ≤60 runes）。
	marginNoteQueryRunes = 60
	// marginNoteQueryGenQuotedRunes caps the quoted text fed into the
	// query-generation prompt（锚文本截 ≤200 runes，变体表）。
	marginNoteQueryGenQuotedRunes = 200
)

// marginNoteQueryGenFn is the swappable query-generation hook (D8,
// marginNoteChatFn precedent): tests stub it to script generated queries;
// the default routes through airouter open_notebook（Operation 必填、每次尝
// 试落 ai_call_logs 审计，ai-summary 红线由 airouter 内部强制）。
var marginNoteQueryGenFn = func(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	return airouter.NewRouter().Chat(ctx, req)
}

// generateMarginNoteSearchQueries runs the D1 pre-call: LLM 依据问题语义
// 自由生成 1~3 条中文搜索词。任何失败（调用错误/超时/非 JSON/空/全空白）返
// 回 nil，由调用方静默回退锚文本 query（D5 降级树）。
func generateMarginNoteSearchQueries(ctx context.Context, quotedText, question, threadTitle, threadSummary string) []string {
	temperature := float64(marginNoteQueryGenTemperature)
	maxTokens := marginNoteQueryGenMaxTokens
	system, user := buildMarginNoteQueryGenPrompt(quotedText, question, threadTitle, threadSummary)
	result, err := marginNoteQueryGenFn(ctx, airouter.ChatRequest{
		Operation:  MarginNoteQueryGenOperation,
		SessionID:  SessionIDFromContext(ctx),
		Capability: airouter.CapabilityOpenNotebook,
		Messages: []airouter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		JSONMode:    true,
		JSONSchema:  marginNoteQueryGenSchema(),
		Metadata:    map[string]any{"operation": MarginNoteQueryGenOperation},
	})
	if err != nil {
		logging.Warnf("margin-notes: search query generation failed, falling back to quoted text: %v", err)
		return nil
	}
	queries := parseMarginNoteQueries(result.Content)
	if len(queries) == 0 {
		logging.Warnf("margin-notes: search query generation returned no usable queries, falling back to quoted text")
		return nil
	}
	return queries
}

// buildMarginNoteQueryGenPrompt assembles the query-generation prompt (D6:
// 问题语义优先、精炼词组、不输出解释)。测试断言不锁文案，只锁锚文本/问题/
// 线索上下文确实拼入。
func buildMarginNoteQueryGenPrompt(quotedText, question, threadTitle, threadSummary string) (string, string) {
	var sys strings.Builder
	sys.WriteString("你是个人知识系统的搜索查询生成器，为日报页边注问答准备联网搜索词。")
	sys.WriteString("依据读者的问题与划选原文，生成 1~3 条适合网页搜索的中文查询词：")
	sys.WriteString("问题与划选原文主题一致时，围绕原文核心概念组织搜索词；问题延伸到原文之外时（如问选举而原文讲民调），以问题语义为准；")
	sys.WriteString("每条是精炼的搜索词组（关键词组合），不是完整句子，不带解释。")
	sys.WriteString(`输出严格 JSON 对象：{"queries": string[]}，queries 含 1~3 条搜索词。`)

	var u strings.Builder
	if quoted := strings.TrimSpace(quotedText); quoted != "" {
		u.WriteString("【读者划选的原文】\n" + searxng.BuildQuery(quoted, marginNoteQueryGenQuotedRunes) + "\n\n")
	}
	u.WriteString("【读者的问题】\n" + question + "\n")
	if threadTitle != "" || threadSummary != "" {
		u.WriteString("\n【该段所在线索】\n")
		if threadTitle != "" {
			u.WriteString("标题：" + threadTitle + "\n")
		}
		if threadSummary != "" {
			u.WriteString("摘要：" + threadSummary + "\n")
		}
	}
	u.WriteString("\n请生成 1~3 条搜索词。")
	return sys.String(), u.String()
}

// marginNoteQueryGenSchema is the query-generation JSON schema {queries[]}.
func marginNoteQueryGenSchema() *airouter.JSONSchema {
	return &airouter.JSONSchema{
		Type: "object",
		Properties: map[string]airouter.SchemaProperty{
			"queries": {Type: "array", Items: &airouter.SchemaProperty{Type: "string"}, Description: "1~3 条中文搜索词组，非完整句子"},
		},
		Required: []string{"queries"},
	}
}

// parseMarginNoteQueries normalizes the query-generation payload with
// field-level tolerance (D1 解析容错)：非 JSON → nil；逐条 trim、空白丢弃、
// 单条截 ≤60 runes；过滤后空 → nil；超 3 条截前 3（Warn 不抛）。
func parseMarginNoteQueries(content string) []string {
	content = jsonutil.SanitizeLLMJSON(content)
	var raw struct {
		Queries []string `json:"queries"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		logging.Warnf("margin-notes: query generation response unparseable: %v", err)
		return nil
	}
	queries := make([]string, 0, len(raw.Queries))
	for _, q := range raw.Queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if r := []rune(q); len(r) > marginNoteQueryRunes {
			q = string(r[:marginNoteQueryRunes])
		}
		queries = append(queries, q)
	}
	if len(queries) == 0 {
		return nil
	}
	if len(queries) > marginNoteQueryMaxCount {
		logging.Warnf("margin-notes: query generation returned %d queries, capping at %d", len(queries), marginNoteQueryMaxCount)
		queries = queries[:marginNoteQueryMaxCount]
	}
	return queries
}

// ── 联网搜索状态三态（add-searxng-health-indicator design D3/D4）────────
//
// 落库三态（面板四态的落库投影：unreachable/degraded 对读者语义同为「联网
// 没帮上」，合并为 empty；面板才需细分排障）。枚举值即字符串常量，可读性好、
// 日志友好；旧行 NULL=历史数据（前端不渲染标记，不回填）。
const (
	// MarginNoteWebSearchStatusDisabled: SearXNG 未配置或未启用（预检不过）。
	MarginNoteWebSearchStatusDisabled = "disabled"
	// MarginNoteWebSearchStatusOK: 链路启用且有结果进入参考池。
	MarginNoteWebSearchStatusOK = "ok"
	// MarginNoteWebSearchStatusEmpty: 链路启用但全部搜索失败或结果为空。
	MarginNoteWebSearchStatusEmpty = "empty"
)

// searchWebForMarginNoteWithQueryGen runs the D5 degrade tree for the
// web-search step: endpoint 预检（D3）→ LLM query 生成（D1）→ 并行搜索合并
// （D4）→ 各级静默降级。返回的合并结果池直接作为回答调用的联网参考块与
// web_sources 白名单候选集；status 与降级树同函数采集（单一采集点，不会漏
// 分支）：预检不过→disabled；链路启用且池非空→ok；链路启用但全部失败/空
// →empty。静默降级行为不变（联网是增益不是依赖）。
func searchWebForMarginNoteWithQueryGen(ctx context.Context, quotedText, question, threadTitle, threadSummary string) ([]searxng.Result, string) {
	// D3 端点预检前置：未配置/禁用直接跳过整个联网链路（省 query 生成调用）。
	if _, ok := marginNoteSearxngEndpointFn(); !ok {
		return nil, MarginNoteWebSearchStatusDisabled
	}
	queries := generateMarginNoteSearchQueries(ctx, quotedText, question, threadTitle, threadSummary)
	var results []searxng.Result
	if len(queries) == 0 {
		// 生成失败/空 → 静默回退锚文本 query（现状规则）单 query 搜索。
		if fallback := MarginNoteSearchQuery(quotedText, question); fallback != "" {
			results = searchWebForMarginNote(ctx, fallback)
		}
	} else {
		results = searchWebParallelForMarginNote(ctx, queries)
	}
	if len(results) > 0 {
		return results, MarginNoteWebSearchStatusOK
	}
	return nil, MarginNoteWebSearchStatusEmpty
}

// searchWebParallelForMarginNote fans out one search per query (D4 并行)：各
// goroutine 独立调 marginNoteWebSearchFn（searxng 客户端单次超时 8s 不变，最
// 坏墙钟 8s）；单个 query 失败/空不拖垮其他；全部失败/空 → 无联网参考块
// （静默，不再回退锚文本——SearXNG 同轮已挂，二跳只加延迟）。
func searchWebParallelForMarginNote(ctx context.Context, queries []string) []searxng.Result {
	perQuery := make([][]searxng.Result, len(queries))
	var wg sync.WaitGroup
	for i, q := range queries {
		wg.Add(1)
		go func(idx int, query string) {
			defer wg.Done()
			results, err := marginNoteWebSearchFn(ctx, query)
			if err != nil {
				logging.Warnf("margin-notes: web search failed (query %q), skipping: %v", query, err)
				return
			}
			perQuery[idx] = results
		}(i, q)
	}
	wg.Wait()
	merged := mergeMarginNoteSearchResults(perQuery)
	if len(merged) == 0 {
		logging.Warnf("margin-notes: all web searches empty/failed, skipping web context")
		return nil
	}
	return merged
}

// mergeMarginNoteSearchResults merges per-query results into the whitelist
// pool (D4 合并)：按 query 顺序 × 各自结果顺序遍历，URL（trim 精确匹配）去
// 重保序（首次出现位次），截前 marginNoteSearxngResultLimit 条。
func mergeMarginNoteSearchResults(perQuery [][]searxng.Result) []searxng.Result {
	seen := make(map[string]struct{})
	merged := make([]searxng.Result, 0, marginNoteSearxngResultLimit)
	for _, results := range perQuery {
		for _, r := range results {
			url := strings.TrimSpace(r.URL)
			if url == "" {
				continue
			}
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			merged = append(merged, r)
			if len(merged) >= marginNoteSearxngResultLimit {
				return merged
			}
		}
	}
	return merged
}
