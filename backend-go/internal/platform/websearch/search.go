// Package websearch 是统一网页搜索入口：给定搜索词，取回原始网页结果。
//
// 目前的唯一后端是 DeepSeek Search API（POST {vendor.base_url}/tools/search），
// 凭据复用 AI 供应商注册表（见 credentials.go）；调用方（页边注问答联网参考、
// 设置面板连通性探测）只依赖本包的 Search / ProbeHealth，换搜索后端不扩散到
// 业务代码。
//
// 契约要点（daily-report 红线 25 / 27 与 web-search-provider spec）：
//   - 只取原始网页结果，不含服务商 AI 总结段；不做域名白名单、不跨调用缓存、
//     不重试——联网是问答的增益不是依赖，失败一律由调用方静默降级。
//   - 失败按 ErrorKind 分类（见 errors.go），使探测面板能区分「密钥错」与
//     「余额没」，而不是把一切揉成一句字符串。
//   - 每次调用现解析凭据（AI 路由里改配置即时生效，无需重启）。
//
// 调用方只依赖本包的 Search / ProbeHealth：无 endpoint 参数（凭据内置），
// Result 带相关度分供多 query 合并池跨 query 重排。
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"syntopica-backend/internal/platform/httpclient"
)

const (
	// DefaultTimeout 是单次搜索的墙钟上限。DeepSeek 直连实测 0.46s（2026-11-23）；
	// 8s 只为坏路径兜底，失败会静默降级，不拖长问答延迟。
	DefaultTimeout = 8 * time.Second
	// MaxSummaryRunes 限制单条摘要进 prompt 的长度。DeepSeek summary 实测
	// 239~1596 字，故上限取 400（比按 snippet 设计的 200 更宽）；400 runes ×
	// 前 5 条仍是可控的 prompt 增量（design D4 / 红线 25）。
	MaxSummaryRunes = 400
	// MaxQueryRunes 是搜索词的上限（DeepSeek 契约 1~128 字符，超出本地拒绝）。
	MaxQueryRunes = 128
	// DefaultResultCount 单次请求索取条数：取 10 让跨 query 重排有足够候选池
	// （design Open Questions，可上线后调）。
	DefaultResultCount = 10
	// searchPath 拼在 vendor.base_url（含 /v1）之后。
	searchPath = "/tools/search"
	// maxResponseBytes 是响应体读取上限（4MB）。
	maxResponseBytes = 4 << 20
)

// Result 是一条原始网页结果。Title/URL/Content 三字段是既有消费方
// （margin_notes_websearch.go）直接沿用的形状；Score 是服务商相关度分
// （rerank_score，0~3），供多 query 合并池跨 query 重排；其余为可信度信号，
// 服务商提供时透传、缺失时为零值。
type Result struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Content     string  `json:"content"`
	Score       float64 `json:"score"`
	SiteName    string  `json:"site_name,omitempty"`
	PublishedAt string  `json:"published_at,omitempty"` // RFC3339 原值透传，不解析不丢弃
	Authority   float64 `json:"authority,omitempty"`    // 站点权威度 0~4
}

// searchHTTPClient 可在同包测试中替换（改超时 / 指 httptest）；生产即共享
// 直连客户端。
var searchHTTPClient = httpclient.New(httpclient.WithTimeout(DefaultTimeout))

// resolveCredentials 是凭据解析的替换点：生产即 ResolveCredentials，同包测试
// 脚本化「已配置 vendor / 未配置」两态并指向 httptest，不碰真库。
var resolveCredentials = ResolveCredentials

// searchRequest 是 DeepSeek Search 请求体（need_content=false：只要摘要，
// 不要正文，省带宽与延迟）。
type searchRequest struct {
	Query       string `json:"query"`
	Count       int    `json:"count"`
	NeedContent bool   `json:"need_content"`
}

// apiResult 是 DeepSeek 响应 results[] 的元素。summary 为空时回退 snippet。
type apiResult struct {
	RerankScore float64 `json:"rerank_score"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Summary     string  `json:"summary"`
	SiteName    string  `json:"site_name"`
	Date        string  `json:"date"`
	Authority   float64 `json:"authority"`
}

type searchResponse struct {
	Results []apiResult `json:"results"`
}

// Search 对当前凭据解析出的 DeepSeek 端点发起一次搜索。query trim 后为空或
// 超过 MaxQueryRunes → 本地拒绝（不发请求）；服务商 200 且 results 为空 →
// 返回空切片 + nil error（无结果不是错误，也不计费）。
//
// 结果保留服务商顺序，URL trim 后为空的条目丢弃（证据必须可点击），摘要取
// summary、为空回退 snippet，截 MaxSummaryRunes。网络/协议失败按 ErrorKind
// 分类返回；调用方预期静默降级。本函数不做缓存、不重试、不做域名过滤。
func Search(ctx context.Context, query string) ([]Result, error) {
	q := strings.TrimSpace(query)
	if q == "" || utf8.RuneCountInString(q) > MaxQueryRunes {
		return nil, &Error{
			Kind: KindInvalidQuery,
			Msg:  fmt.Sprintf("query must be 1~%d runes after trim", MaxQueryRunes),
		}
	}

	creds, err := resolveCredentials(ctx)
	if err != nil {
		return nil, err
	}
	return searchWithCredentials(ctx, creds, q)
}

// searchWithCredentials 是请求与解析主体（凭据已解析）。
func searchWithCredentials(ctx context.Context, creds Credentials, query string) ([]Result, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(creds.BaseURL), "/") + searchPath

	payload, err := json.Marshal(searchRequest{
		Query:       query,
		Count:       DefaultResultCount,
		NeedContent: false,
	})
	if err != nil {
		return nil, &Error{Kind: KindUnavailable, Msg: "build request body", Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, &Error{Kind: KindUnavailable, Msg: "build request", Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(creds.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := searchHTTPClient.Do(req)
	if err != nil {
		return nil, transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, &Error{
			Kind:   KindUnavailable,
			Status: resp.StatusCode,
			Msg:    "read response body",
			Err:    err,
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, statusError(resp.StatusCode, body)
	}

	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &Error{
			Kind:   KindUnavailable,
			Status: resp.StatusCode,
			Msg:    "parse response body",
			Err:    err,
		}
	}
	return toResults(parsed.Results), nil
}

// toResults 把服务商条目映射成本包 Result：URL 为空的丢弃，摘要走
// summary→snippet 回退并截断，其余信号透传。顺序严格保留（重排是调用方
// 合并池的职责，本包不做）。
func toResults(raw []apiResult) []Result {
	out := make([]Result, 0, len(raw))
	for _, r := range raw {
		url := strings.TrimSpace(r.URL)
		if url == "" {
			continue // 证据必须可点击
		}
		content := strings.TrimSpace(r.Summary)
		if content == "" {
			content = strings.TrimSpace(r.Snippet)
		}
		out = append(out, Result{
			Title:       strings.TrimSpace(r.Title),
			URL:         url,
			Content:     truncateRunes(content, MaxSummaryRunes),
			Score:       r.RerankScore,
			SiteName:    strings.TrimSpace(r.SiteName),
			PublishedAt: strings.TrimSpace(r.Date),
			Authority:   r.Authority,
		})
	}
	return out
}

// transportError 把 http.Client.Do 的失败分类：截止时间/网络超时 → timeout，
// 其余（连不上、TLS 失败、连接被重置）→ unavailable。
func transportError(err error) error {
	if isTimeoutError(err) {
		return &Error{Kind: KindTimeout, Msg: "request timed out or deadline exceeded", Err: err}
	}
	return &Error{Kind: KindUnavailable, Msg: "request failed", Err: err}
}

// statusError 把非 2xx 映射成类别（design D3）：401→auth；402→quota；403 按
// 响应体是否表额度语义分 quota/auth（DeepSeek 余额耗尽实测 403
// "You do not have enough money or package quota"，Bocha 同款文案）；429→
// rate_limited；5xx 与其余→unavailable。
func statusError(status int, body []byte) error {
	e := &Error{Status: status, Msg: summarizeBody(body)}
	switch status {
	case http.StatusUnauthorized:
		e.Kind = KindAuth
		e.Msg = "鉴权失败（DeepSeek 密钥无效或无权限）: " + e.Msg
	case http.StatusPaymentRequired:
		e.Kind = KindQuota
		e.Msg = "余额或额度不足: " + e.Msg
	case http.StatusForbidden:
		if containsQuotaSignal(body) {
			e.Kind = KindQuota
			e.Msg = "余额或额度不足: " + e.Msg
		} else {
			e.Kind = KindAuth
			e.Msg = "鉴权失败（密钥无权访问搜索接口）: " + e.Msg
		}
	case http.StatusTooManyRequests:
		e.Kind = KindRateLimited
		e.Msg = "请求被限速: " + e.Msg
	default:
		e.Kind = KindUnavailable
	}
	return e
}

// quotaSignals 是「余额/额度不足」的响应体关键词（大小写不敏感）。
var quotaSignals = []string{
	"quota",
	"balance",
	"余额",
	"arrears",
	"insufficient",
	"money",
}

func containsQuotaSignal(body []byte) bool {
	lower := strings.ToLower(string(body))
	for _, kw := range quotaSignals {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// summarizeBody 把响应体裁成一行可读摘要（明细只进日志，不透传原始 JSON；
// 截 200 runes 防止错误信息把日志撑爆）。
func summarizeBody(body []byte) string {
	compact := strings.Join(strings.Fields(string(body)), " ")
	if compact == "" {
		return "empty response body"
	}
	return truncateRunes(compact, 200)
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
