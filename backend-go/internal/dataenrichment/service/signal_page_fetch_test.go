package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/platform/safefetch"
)

// TestQuestionSafePageFetch：signal 专用安全页面适配（S8「有界来源访问与
// 时间语义」）。经 safefetch 逐跳验证后由 readability 从 bytes 解析——先校
// 验后抓取；危险目标断言「内网服务没有收到请求」。复用 safefetch 自身的
// 地址分类测试，此处只补 signal 适配确实经过它的集成证明与 typed gap。

// qFakePageFetcher 是 SignalPageFetchAdapter 的假实现：不走网络，直接把
// 预置 HTML 交给真实解析纯函数（parseSignalFetchedPage），调用计数供
// 「危险目标请求为0」断言。
type qFakePageFetcher struct {
	body   string
	status int
	calls  int
}

func (f *qFakePageFetcher) FetchPage(_ context.Context, _ string) (*SignalPageFetchResult, error) {
	f.calls++
	if f.status != 0 && f.status != http.StatusOK {
		return nil, &SignalPageFetchError{Kind: signalPageGapHTTPStatus, Detail: fmt.Sprintf("http status %d", f.status)}
	}
	return parseSignalFetchedPage(f.body, "https://x.example.com/final", time.Now())
}

func qSafeOptions() safefetch.Options {
	// 测试允许 loopback：httptest 服务器绑定 127.0.0.1。AllowedIPs 是
	// 显式白名单（safefetch 契约：授权按请求、绝不全局放开私网）。
	_, loop, _ := net.ParseCIDR("127.0.0.0/8")
	return safefetch.Options{AllowedIPs: []*net.IPNet{loop}, Timeout: 5 * time.Second}
}

func TestQuestionSafePageFetch(t *testing.T) {
	t.Run("URL门：userinfo与非http拒绝", func(t *testing.T) {
		for _, raw := range []string{
			"https://user:pass@example.com/a",
			"ftp://example.com/a",
			"file:///etc/passwd",
			"notaurl",
		} {
			if _, err := normalizeSignalWebURL(raw); err == nil {
				t.Fatalf("%q must be rejected by the URL gate", raw)
			}
		}
		// 合法 URL：去 fragment、保留 query、host 归一。
		n, err := normalizeSignalWebURL("HTTPS://Example.COM/a?x=1#frag")
		if err != nil {
			t.Fatalf("legit url rejected: %v", err)
		}
		if n != "https://example.com/a?x=1" {
			t.Fatalf("normalized url wrong: %q", n)
		}
	})

	t.Run("未登记URL请求为0", func(t *testing.T) {
		v1 := qFixtureMaterial(SignalResearchContractQuestionV1)
		v1.Sources = qFixtureSources()
		fetcher := &qFakePageFetcher{body: "<html><body><p>x</p></body></html>"}
		rt, _ := newSignalQuestionRuntime(v1, &SignalResearchBrief{SourceIDs: []string{"n1"}}, nil, fetcher)
		if _, err := rt.readSource("n1"); err != nil {
			t.Fatalf("read first: %v", err)
		}
		// 未登记 URL：拒绝且不触网（模型自造 URL 不放行）。
		if _, err := rt.fetchPage(context.Background(), "https://evil.example.com/selfmade"); err == nil {
			t.Fatalf("unregistered URL must be rejected")
		}
		if fetcher.calls != 0 {
			t.Fatalf("no request may reach the network for unregistered URLs, got %d", fetcher.calls)
		}
		// 网页内链接不递归授权：登记 A、请求 B（同站不同路径）仍拒绝。
		rt.registerSearchHits([]WebSearchResult{{Title: "t", URL: "https://x.example.com/registered", Snippet: "s"}})
		if _, err := rt.fetchPage(context.Background(), "https://x.example.com/other"); err == nil {
			t.Fatalf("in-page link must not inherit authorization")
		}
		if fetcher.calls != 0 {
			t.Fatalf("unrelated URL must not reach network, got %d calls", fetcher.calls)
		}
	})

	t.Run("safefetch集成与重定向私网拒绝", func(t *testing.T) {
		// 公网语义服务器（httptest 绑 loopback，经 AllowedIPs 显式授权）。
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><meta property="article:published_time" content="2026-09-15T08:00:00+08:00"></head><body><article><h1>标题</h1><p>正文第一段。</p></article></body></html>`))
		}))
		defer srv.Close()

		fetcher := NewSignalSafePageFetcherWithOptions(qSafeOptions())
		res, err := fetcher.Fetch(context.Background(), srv.URL)
		if err != nil {
			t.Fatalf("safe fetch through safefetch must succeed: %v", err)
		}
		if !strings.Contains(res.Text, "正文第一段") {
			t.Fatalf("readability text missing, got %q", res.Text)
		}
		if res.PublishedAt == nil || res.PublishedFrom == "" {
			t.Fatalf("machine metadata date must be extracted with provenance, got %+v", res.PublishedAt)
		}

		// 重定向到未授权私网地址：safefetch 拒绝（10/8 不在白名单）。
		redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://10.255.255.1/internal", http.StatusFound)
		}))
		defer redirect.Close()
		if _, err := fetcher.Fetch(context.Background(), redirect.URL); err == nil {
			t.Fatalf("redirect into private range must be rejected by safefetch")
		}
	})

	t.Run("私网直接拒绝且无请求", func(t *testing.T) {
		fetcher := NewSignalSafePageFetcherWithOptions(safefetch.Options{Timeout: 2 * time.Second})
		for _, raw := range []string{
			"http://127.0.0.1:5100/api/health",
			"http://169.254.169.254/latest/meta-data/",
			"http://10.0.0.1/x",
			"http://[::1]/x",
		} {
			if _, err := fetcher.Fetch(context.Background(), raw); err == nil {
				t.Fatalf("%s must be rejected (private/loopback/metadata)", raw)
			}
		}
	})

	t.Run("超体积typed gap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>" + strings.Repeat("a", 3<<20) + "</body></html>"))
		}))
		defer srv.Close()
		fetcher := NewSignalSafePageFetcherWithOptions(qSafeOptions())
		_, err := fetcher.Fetch(context.Background(), srv.URL)
		var gap *SignalPageFetchError
		if err == nil {
			t.Fatalf("oversize body must fail")
		}
		if g, ok := err.(*SignalPageFetchError); !ok {
			t.Fatalf("oversize must surface a typed gap, got %T: %v", err, err)
		} else {
			gap = g
		}
		if gap.Kind != signalPageGapTooLarge {
			t.Fatalf("oversize gap kind wrong: %s", gap.Kind)
		}
	})

	t.Run("超时typed gap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body><p>late</p></body></html>"))
		}))
		defer srv.Close()
		fetcher := NewSignalSafePageFetcherWithOptions(safefetch.Options{Timeout: 300 * time.Millisecond, AllowedIPs: qSafeOptions().AllowedIPs})
		_, err := fetcher.Fetch(context.Background(), srv.URL)
		var gap *SignalPageFetchError
		if err == nil {
			t.Fatalf("timeout must fail")
		}
		if g, ok := err.(*SignalPageFetchError); !ok {
			t.Fatalf("timeout must surface a typed gap, got %T: %v", err, err)
		} else {
			gap = g
		}
		if gap.Kind != signalPageGapTimeout {
			t.Fatalf("timeout gap kind wrong: %s", gap.Kind)
		}
	})

	t.Run("非HTML与非2xx拒绝", func(t *testing.T) {
		jsonSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"x":1}`))
		}))
		defer jsonSrv.Close()
		fetcher := NewSignalSafePageFetcherWithOptions(qSafeOptions())
		if _, err := fetcher.Fetch(context.Background(), jsonSrv.URL); err == nil {
			t.Fatalf("non-HTML body must be a typed gap")
		} else if g, ok := err.(*SignalPageFetchError); !ok || g.Kind != signalPageGapNotHTML {
			t.Fatalf("non-HTML gap kind wrong: %v", err)
		}
		// HTTP 404 → 非 2xx 拒绝。
		notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer notFound.Close()
		if _, err := fetcher.Fetch(context.Background(), notFound.URL); err == nil {
			t.Fatalf("404 must be a typed gap")
		} else if g, ok := err.(*SignalPageFetchError); !ok || g.Kind != signalPageGapHTTPStatus {
			t.Fatalf("404 gap kind wrong: %v", err)
		}
	})

	t.Run("日期提取：meta与JSONLD", func(t *testing.T) {
		pub, mod, from := extractSignalPageDates(`<html><head>
			<meta property="article:published_time" content="2026-09-15T08:00:00+08:00">
			</head><body><p>x</p></body></html>`)
		if pub == nil || from == "" {
			t.Fatalf("meta published_time must be extracted with provenance")
		}
		_ = mod
		pub2, mod2, _ := extractSignalPageDates(`<html><head><script type="application/ld+json">
			{"@type":"NewsArticle","datePublished":"2026-09-10T00:00:00Z","dateModified":"2026-10-02T00:00:00Z"}
			</script></head><body><p>x</p></body></html>`)
		if pub2 == nil || mod2 == nil {
			t.Fatalf("JSON-LD datePublished/dateModified must be extracted")
		}
		_ = mod
		// 无日期页面 → nil（调用方记 gap，不猜日期）。
		pub3, _, _ := extractSignalPageDates(`<html><body><p>无日期</p></body></html>`)
		if pub3 != nil {
			t.Fatalf("date-less page must yield nil published, got %v", pub3)
		}
	})
}
