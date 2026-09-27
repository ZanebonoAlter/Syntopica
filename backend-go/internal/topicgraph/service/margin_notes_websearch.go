package service

import (
	"context"
	"errors"
	"strings"

	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/searxng"
)

// ── 页边注问答联网后端（daily-report-margin-notes design D7）──
//
// 触发策略：每次必搜 + 失败静默降级——搜索失败/超时/未配置只影响是否携带联网
// 参考块，绝不阻断问答（对齐 D6 失败隔离精神）。客户端在
// internal/platform/searxng（独立通用包，避免 topicgraph → dataenrichment
// 循环引用）；配置 searxng_config 对齐 bocha_config 语义：
// DB(界面) > env(SEARXNG_URL，config 加载时折入 AppConfig) > config.yaml >
// 空 = 禁用；每次提问现读，界面改即时生效。

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
// disabled (DB absent-or-disabled AND env/config.yaml empty).
func marginNoteSearxngEndpoint() (string, bool) {
	// 1. DB (UI) first. enabled 缺省视为启用（仅显式 false 才跳过 DB）。
	if cfg, _, err := aisettings.LoadSearxngConfig(); err == nil && cfg != nil {
		if v, ok := cfg["enabled"].(bool); !ok || v {
			if ep, _ := cfg["endpoint"].(string); strings.TrimSpace(ep) != "" {
				return strings.TrimSpace(ep), true
			}
		}
	}
	// 2. env / config.yaml 兑底（SEARXNG_URL 已在 config 加载时折入）。
	if c := config.AppConfig; c != nil {
		if ep := strings.TrimSpace(c.Searxng.Endpoint); ep != "" {
			return ep, true
		}
	}
	return "", false
}
