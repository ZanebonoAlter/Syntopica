package datasources

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FetchBudget bounds a single HTTP request (design decision 3). Values follow
// the energy-mcp field experience: JODI annual CSV is ~4.76MB, so 32MB leaves
// ample headroom; slow upstreams get 45s wall clock.
type FetchBudget struct {
	Timeout      time.Duration // total wall-clock budget per request (redirects included)
	MaxBytes     int64         // streamed response body cap; exceeding aborts
	MaxRedirects int           // manual redirect hops (same-host https only)
}

// DefaultFetchBudget is the production budget. Timeout is 120s: the JODI
// annual CSV (~4.76MB) exceeded the original 45s ceiling on the Raspberry Pi
// behind a proxy (2026-09-19 live smoke); small responses are unaffected
// (the timeout is a ceiling, not added latency).
func DefaultFetchBudget() FetchBudget {
	return FetchBudget{Timeout: 120 * time.Second, MaxBytes: 32 << 20, MaxRedirects: 3}
}

// FetchedDoc is a successfully fetched upstream document. It is what the
// cache stores; every field must survive cache hits unchanged (spec: 缓存命中
// 保留原元数据: retrieved_at/sha256/last_modified).
type FetchedDoc struct {
	URL          string
	RetrievedAt  time.Time
	LastModified string // normalized from Last-Modified header, "" when absent
	SHA256       string
	ContentType  string
	Bytes        int64
	Payload      []byte // raw body

	// FromCache is true when the document came from the TTL cache (never
	// persisted; set by the cache on hits).
	FromCache bool
}

// Fetcher performs budgeted GETs restricted to an allowlist of official HTTPS
// hosts. No arbitrary-URL parameter is ever accepted from callers; callers
// pass paths relative to a definition's fixed host.
type Fetcher struct {
	allowlist map[string]struct{}
	budget    FetchBudget
	client    *http.Client
}

// NewFetcher builds a fetcher for the given allowlisted hosts.
func NewFetcher(allowlist []string, budget FetchBudget) *Fetcher {
	al := make(map[string]struct{}, len(allowlist))
	for _, h := range allowlist {
		al[normalizeHost(h)] = struct{}{}
	}
	return &Fetcher{
		allowlist: al,
		budget:    budget,
		// Manual redirect control: never follow automatically; hops are
		// validated against the allowlist + https before following.
		client: &http.Client{
			Timeout: budget.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// SetClient replaces the HTTP client (production never calls this; tests
// inject a client that trusts the httptest TLS cert).
func (f *Fetcher) SetClient(c *http.Client) { f.client = c }

// normalizeHost lower-cases and strips an optional :port so allowlist entries
// and request hosts compare consistently.
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		host = host[:i]
	}
	return host
}

// allowedHost reports whether host (lower-cased, port stripped) is allowlisted.
func (f *Fetcher) allowedHost(host string) bool {
	_, ok := f.allowlist[normalizeHost(host)]
	return ok
}

// Get fetches url under budget. HTML masquerading as data (200 + text/html or
// html sniff) is rejected so it never reaches parsing or the cache.
func (f *Fetcher) Get(ctx context.Context, rawURL string, headers map[string]string) (*FetchedDoc, error) {
	if err := f.checkURL(rawURL); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(f.budget.Timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	hops := 0
	current := rawURL
	for {
		if hops > f.budget.MaxRedirects {
			return nil, Unavailable("fetch", "重定向跳数超预算")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return nil, InvalidArg("fetch", "构造请求失败: "+err.Error())
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := f.client.Do(req)
		if err != nil {
			return nil, Unavailable("fetch", "网络请求失败")
		}
		// Manual redirect handling: validate target then loop.
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			_ = resp.Body.Close()
			if loc == "" {
				return nil, UnavailableDetail("fetch",
					fmt.Sprintf("重定向缺少 Location（status=%d）", resp.StatusCode), current)
			}
			next, err := url.Parse(loc)
			if err != nil {
				return nil, Unavailable("fetch", "重定向地址非法")
			}
			current = req.URL.ResolveReference(next).String()
			if err := f.checkURL(current); err != nil {
				return nil, err
			}
			hops++
			continue
		}
		return f.readBody(resp, current)
	}
}

// checkURL enforces https + allowlist. Callers never pass arbitrary URLs in
// production (paths are built from fixed catalog definitions), this is the
// belt-and-braces gate.
func (f *Fetcher) checkURL(rawURL string) *SourceError {
	u, err := url.Parse(rawURL)
	if err != nil {
		return InvalidArg("fetch", "URL 非法")
	}
	if u.Scheme != "https" {
		return UnavailableDetail("fetch", "仅允许 HTTPS", rawURL)
	}
	if !f.allowedHost(u.Host) {
		return UnavailableDetail("fetch", "目标 host 不在核定白名单内", u.Host)
	}
	return nil
}

// readBody drains the response under the byte budget, rejects non-200 and
// HTML content, and returns the document with retrieval metadata.
func (f *Fetcher) readBody(resp *http.Response, finalURL string) (*FetchedDoc, error) {
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, UnavailableStatus("fetch", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	doc := &FetchedDoc{
		RetrievedAt:  time.Now().UTC(),
		ContentType:  ct,
		LastModified: normalizeHTTPDate(resp.Header.Get("Last-Modified")),
		URL:          finalURL,
	}

	buf := make([]byte, 0, 1<<16)
	tmp := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			doc.Bytes += int64(n)
			if doc.Bytes > f.budget.MaxBytes {
				return nil, UnavailableDetail("fetch",
					fmt.Sprintf("响应体超过大小预算 %d 字节", f.budget.MaxBytes), finalURL)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, Unavailable("fetch", "读取响应体失败")
		}
		// HTML sniff on the head of the payload (Content-Type can be absent
		// for some official CSV endpoints).
		if len(buf) >= 16 && looksLikeHTML(string(buf[:64]), ct) {
			return nil, UnavailableDetail("fetch", "上游返回 HTML 而非预期数据格式", finalURL)
		}
	}
	if looksLikeHTML(string(buf), ct) {
		return nil, UnavailableDetail("fetch", "上游返回 HTML 而非预期数据格式", finalURL)
	}
	sum := sha256.Sum256(buf)
	doc.SHA256 = hex.EncodeToString(sum[:])
	doc.Bytes = int64(len(buf))
	doc.Payload = append([]byte(nil), buf...)
	return doc, nil
}

// normalizeHTTPDate parses an HTTP date header into RFC3339 UTC; unparseable
// values pass through unchanged, absent → "".
func normalizeHTTPDate(v string) string {
	if v == "" {
		return ""
	}
	if t, err := http.ParseTime(v); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	return v
}

// looksLikeHTML detects HTML by content-type or by a leading html/doctype tag.
func looksLikeHTML(head, contentType string) bool {
	if strings.HasPrefix(contentType, "text/html") {
		return true
	}
	h := strings.TrimSpace(strings.ToLower(head))
	return strings.HasPrefix(h, "<!doctype html") || strings.HasPrefix(h, "<html")
}
