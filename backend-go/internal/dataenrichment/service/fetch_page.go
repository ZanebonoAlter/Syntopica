package service

import (
	"context"
	"encoding/json"

	reader "syntopica-backend/internal/reader"
)

// maxFetchPageRunes caps the main_text length returned by fetch_page so a single
// huge page cannot blow up the agent's context window. Matches the design's
// "{main_text(截断 N 字符)}" note; 4000 runes is plenty for a verifiable quote.
const maxFetchPageRunes = 4000

// PageFetcher abstracts article-body fetching for the fetch_page tool. The
// production impl reuses the reader domain's readability crawler; tests inject
// a stub. Returning an error degrades to a tool error JSON (registry
// convention) without aborting the agent loop.
type PageFetcher interface {
	FetchPage(ctx context.Context, url string) (title, mainText string, err error)
}

// ReaderPageFetcher adapts reader.ReadabilityCrawler to the PageFetcher
// interface. It is the only implementation wired in production.
type ReaderPageFetcher struct {
	crawler *reader.ReadabilityCrawler
}

// NewReaderPageFetcher builds a ReaderPageFetcher over a fresh readability crawler.
func NewReaderPageFetcher() *ReaderPageFetcher {
	return &ReaderPageFetcher{crawler: reader.NewReadabilityCrawler()}
}

// FetchPage scrapes the page and returns its title plus the readability-extracted
// body (truncated to maxFetchPageRunes).
func (f *ReaderPageFetcher) FetchPage(ctx context.Context, url string) (string, string, error) {
	res, err := f.crawler.ScrapePage(ctx, url)
	if err != nil {
		return "", "", err
	}
	return res.Title, truncateRunes(res.Markdown, maxFetchPageRunes), nil
}

// truncateRunes returns the first n runes of s (full s if shorter). Rune-aware so
// CJK content is cut on a character boundary, not mid-byte.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ── Tool: fetch_page ───────────────────────────────────────────────────────

// executeFetchPage runs the fetch_page tool: empty url → param error; nil
// pageFetcher → "not configured" degradation; otherwise it delegates to the
// injected PageFetcher. A scrape failure (timeout / anti-bot) is surfaced as an
// error JSON with a nil Go error so the agent loop keeps running (registry
// single-tool-failure convention). main_text is already truncated by FetchPage.
func (r *Registry) executeFetchPage(ctx context.Context, args map[string]any) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return jsonError("参数错误: url 必须为非空字符串"), nil
	}
	if r.pageFetcher == nil {
		return jsonError("fetch_page 未配置"), nil
	}
	title, mainText, err := r.pageFetcher.FetchPage(ctx, url)
	if err != nil {
		// Degrade: surface the scrape error to the agent without aborting the loop.
		return jsonError("fetch_page 失败: " + err.Error()), nil
	}
	b, _ := json.Marshal(map[string]any{
		"title":     title,
		"url":       url,
		"main_text": mainText,
	})
	return string(b), nil
}
