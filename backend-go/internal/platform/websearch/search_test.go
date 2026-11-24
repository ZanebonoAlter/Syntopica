package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"syntopica-backend/internal/platform/httpclient"
)

// ── 测试夹具 ────────────────────────────────────────────────────────────────
// 包级变量（resolveCredentials / searchHTTPClient）是测试替换点，替换即全包
// 生效，故本包测试一律不加 t.Parallel()。

// stubCredentials 把凭据解析指向 httptest 端点（不碰真库、不出外网）。
func stubCredentials(t *testing.T, baseURL string) {
	t.Helper()
	orig := resolveCredentials
	resolveCredentials = func(context.Context) (Credentials, error) {
		return Credentials{
			VendorID:   7,
			VendorName: "DeepSeek",
			BaseURL:    baseURL,
			APIKey:     "test-key",
		}, nil
	}
	t.Cleanup(func() { resolveCredentials = orig })
}

// stubNotConfigured 模拟「AI 供应商注册表无可用 DeepSeek 供应商」。
func stubNotConfigured(t *testing.T) {
	t.Helper()
	orig := resolveCredentials
	resolveCredentials = func(context.Context) (Credentials, error) {
		return Credentials{}, ErrNotConfigured
	}
	t.Cleanup(func() { resolveCredentials = orig })
}

// TestSearch_OK 覆盖请求形状（POST 路径/鉴权头/请求体）、字段映射
// （summary→snippet 回退、空 URL 剔除）、摘要截断与服务商顺序保留。
func TestSearch_OK(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotCT string
	var gotBody searchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth, gotCT = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{
					"title":        " 逆回购_财经百科 ",
					"url":          " https://money.163.com/baike/x ",
					"summary":      strings.Repeat("流", MaxSummaryRunes+1200),
					"rerank_score": 1.49,
					"site_name":    "网易财经",
					"date":         "2026-11-20T08:00:00+08:00",
					"authority":    3,
				},
				{"title": "仅有 snippet", "url": "https://example.com/snippet", "snippet": " 简短摘要 ", "rerank_score": 1.43},
				{"title": "无 URL 项应被剔除", "url": "   ", "summary": "orphan"},
				{"title": "两者都有", "url": "https://example.com/both", "summary": " 用 summary ", "snippet": "不用 snippet", "rerank_score": 1.2},
			},
		})
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	results, err := Search(context.Background(), "  央行逆回购  ")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != searchPath {
		t.Fatalf("request = %s %s, want POST %s", gotMethod, gotPath, searchPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if gotBody.Query != "央行逆回购" || gotBody.Count != DefaultResultCount || gotBody.NeedContent {
		t.Fatalf("body = %+v", gotBody)
	}

	if len(results) != 3 {
		t.Fatalf("want 3 results (empty-URL dropped), got %d", len(results))
	}
	r := results[0]
	if r.Title != "逆回购_财经百科" || r.URL != "https://money.163.com/baike/x" {
		t.Fatalf("trim failed: %+v", r)
	}
	if got := len([]rune(r.Content)); got != MaxSummaryRunes {
		t.Fatalf("content runes = %d, want %d", got, MaxSummaryRunes)
	}
	if r.Score != 1.49 || r.SiteName != "网易财经" || r.Authority != 3 {
		t.Fatalf("signals not mapped: %+v", r)
	}
	if r.PublishedAt != "2026-11-20T08:00:00+08:00" {
		t.Fatalf("PublishedAt = %q", r.PublishedAt)
	}
	if results[1].Content != "简短摘要" {
		t.Fatalf("snippet fallback = %q", results[1].Content)
	}
	if results[2].Content != "用 summary" {
		t.Fatalf("summary should win over snippet, got %q", results[2].Content)
	}
	// 顺序保留（不排序：重排是调用方合并池的职责）。
	if results[0].URL != "https://money.163.com/baike/x" || results[1].URL != "https://example.com/snippet" {
		t.Fatalf("order changed: %+v", results)
	}
}

// TestSearch_EmptyResults covers the "200 + results: []" contract: empty slice,
// nil error（无结果不算错误、不计费）。
func TestSearch_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	results, err := Search(context.Background(), "q")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results == nil || len(results) != 0 {
		t.Fatalf("want non-nil empty slice, got %#v", results)
	}
}

// TestSearch_StatusClassification 是错误分类的契约表（test-cases 故事四）：
// 401/402/403-quota/403-other/429/5xx 各自落到唯一类别。
func TestSearch_StatusClassification(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantKind   ErrorKind
		wantStatus int
	}{
		{"401 auth", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`, KindAuth, 401},
		{"402 quota", http.StatusPaymentRequired, `{"error":{"message":"payment required"}}`, KindQuota, 402},
		{"403 quota wording", http.StatusForbidden, `You do not have enough money or package quota`, KindQuota, 403},
		{"403 chinese balance wording", http.StatusForbidden, `{"message":"余额不足"}`, KindQuota, 403},
		{"403 other", http.StatusForbidden, `{"message":"key has no access to tools/search"}`, KindAuth, 403},
		{"429 rate limited", http.StatusTooManyRequests, `{"message":"rate limit exceeded"}`, KindRateLimited, 429},
		{"500 unavailable", http.StatusInternalServerError, `internal error`, KindUnavailable, 500},
		{"503 unavailable empty body", http.StatusServiceUnavailable, ``, KindUnavailable, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			stubCredentials(t, srv.URL)

			_, err := Search(context.Background(), "q")
			if err == nil {
				t.Fatalf("want error for status %d", tc.status)
			}
			if got := KindOf(err); got != tc.wantKind {
				t.Fatalf("KindOf = %q, want %q (err: %v)", got, tc.wantKind, err)
			}
			var typed *Error
			if !errors.As(err, &typed) {
				t.Fatalf("want *Error, got %T", err)
			}
			if typed.Status != tc.wantStatus {
				t.Fatalf("Status = %d, want %d", typed.Status, tc.wantStatus)
			}
		})
	}
}

// TestSearch_BadJSON 覆盖「响应体无法解析 → unavailable」。
func TestSearch_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	_, err := Search(context.Background(), "q")
	if err == nil {
		t.Fatal("want parse error")
	}
	if got := KindOf(err); got != KindUnavailable {
		t.Fatalf("KindOf = %q, want %q", got, KindUnavailable)
	}
}

// TestSearch_InvalidQueryRejectedLocally 覆盖本地拒绝：空/纯空白/超 128 runes
// 一律不发请求（含不解析凭据），Kind=invalid_query；恰好 128 runes 放行。
func TestSearch_InvalidQueryRejectedLocally(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	for _, q := range []string{"", "   ", strings.Repeat("字", MaxQueryRunes+1)} {
		if _, err := Search(context.Background(), q); err == nil {
			t.Fatalf("query %q: want rejection", q)
		} else if got := KindOf(err); got != KindInvalidQuery {
			t.Fatalf("query %q: KindOf = %q, want %q", q, got, KindInvalidQuery)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("invalid queries must not hit the network, got %d requests", got)
	}

	if _, err := Search(context.Background(), strings.Repeat("字", MaxQueryRunes)); err != nil {
		t.Fatalf("boundary query (128 runes) must be accepted: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("hits = %d, want 1", got)
	}
}

// TestSearch_ClientTimeout 覆盖 HTTP 客户端超时 → timeout。
func TestSearch_ClientTimeout(t *testing.T) {
	orig := searchHTTPClient
	searchHTTPClient = httpclient.New(httpclient.WithTimeout(50 * time.Millisecond))
	t.Cleanup(func() { searchHTTPClient = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	if _, err := Search(context.Background(), "q"); err == nil {
		t.Fatal("want timeout error")
	} else if got := KindOf(err); got != KindTimeout {
		t.Fatalf("KindOf = %q, want %q", got, KindTimeout)
	}
}

// TestSearch_ContextDeadline 覆盖调用方 ctx 截止 → timeout。
func TestSearch_ContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	stubCredentials(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Search(ctx, "q"); err == nil {
		t.Fatal("want deadline error")
	} else if got := KindOf(err); got != KindTimeout {
		t.Fatalf("KindOf = %q, want %q", got, KindTimeout)
	}
}

// TestSearch_Unreachable 覆盖连接失败 → unavailable。
func TestSearch_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := srv.URL
	srv.Close() // 端口已关，连接必失败
	stubCredentials(t, deadURL)

	_, err := Search(context.Background(), "q")
	if err == nil {
		t.Fatal("want connection error")
	}
	if got := KindOf(err); got != KindUnavailable {
		t.Fatalf("KindOf = %q, want %q (err: %v)", got, KindUnavailable, err)
	}
}

// TestSearch_NotConfigured 覆盖凭据缺失：errors.Is 可判哨兵，类别为 not_configured。
func TestSearch_NotConfigured(t *testing.T) {
	stubNotConfigured(t)

	_, err := Search(context.Background(), "q")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured chain, got %v", err)
	}
	if got := KindOf(err); got != KindNotConfigured {
		t.Fatalf("KindOf = %q, want %q", got, KindNotConfigured)
	}
}

// TestKindOf_NilAndBareErrors 固化 KindOf 的兜底口径。
func TestKindOf_NilAndBareErrors(t *testing.T) {
	if got := KindOf(nil); got != "" {
		t.Fatalf("KindOf(nil) = %q, want empty", got)
	}
	if got := KindOf(context.DeadlineExceeded); got != KindTimeout {
		t.Fatalf("KindOf(DeadlineExceeded) = %q, want timeout", got)
	}
	if got := KindOf(errors.New("boom")); got != KindUnavailable {
		t.Fatalf("KindOf(bare) = %q, want unavailable", got)
	}
}
