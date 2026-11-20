// Package httpclient provides a factory for outbound *http.Client instances
// whose transport is wrapped with OpenTelemetry (otelhttp) so every outbound
// call becomes a SpanKind=Client span with trace context (traceparent)
// propagated to downstream services. Business code constructs clients via
// httpclient.New instead of bare &http.Client{}.
package httpclient

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// instrumentHTTP toggles otelhttp transport wrapping. Set once at startup
// from main.go via SetInstrumentation based on config.Tracing.InstrumentHTTP.
var instrumentHTTP = true

// SetInstrumentation toggles otelhttp wrapping globally. When false, New
// returns a plain client (current behaviour, for troubleshooting).
func SetInstrumentation(enabled bool) {
	instrumentHTTP = enabled
}

// proxyURLValue holds the globally configured outbound proxy as a *url.URL
// (typed nil when unset). Stored atomically and read per request by the
// package-level failover transport, so SetProxy changes reach every crawl-
// scoped client built via New immediately — including long-lived service
// singletons built at startup — without a restart. Unscoped clients are
// always direct and never consult this value.
var proxyURLValue atomic.Value // *url.URL or (*url.URL)(nil)

// currentProxyURL returns the configured proxy URL, or nil when no proxy is
// configured (direct / env-var fallback behaviour).
func currentProxyURL() *url.URL {
	u, _ := proxyURLValue.Load().(*url.URL)
	return u
}

// SetProxy configures the global outbound proxy applied to crawl-scoped
// clients (WithCrawlScope: feed fetching, article extraction, upstream image
// pulls) — unless the caller overrides Transport via WithTransport. An empty
// URL clears the proxy (restores direct behaviour with the standard
// HTTP_PROXY/HTTPS_PROXY env fallback). Accepted schemes: http, https,
// socks5. Returns an error for malformed or unsupported URLs so the settings
// API can surface bad input without mutating global state.
//
// The change is effective immediately for already-built scoped clients (they
// share one failover transport that reads the URL per request); switching or
// clearing the address also resets the proxy health breaker. Unscoped clients
// (the New default) never route through the proxy.
func SetProxy(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		proxyURLValue.Store((*url.URL)(nil))
		packageTransport().breaker.reset()
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL %q: %w", rawURL, err)
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return fmt.Errorf("unsupported proxy scheme %q: only http/https/socks5 are allowed", u.Scheme)
	}
	proxyURLValue.Store(u)
	packageTransport().breaker.reset()
	return nil
}

// isLoopbackHost reports whether host is a loopback destination: "localhost",
// any 127.0.0.0/8 address, "::1", or empty (same-host). IP parsing handles
// both plain and bracketed IPv6 forms.
func isLoopbackHost(host string) bool {
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// sharedDirect is the process-wide default transport for unscoped clients:
// stdlib defaults with the proxy explicitly disabled, so neither the
// configured outbound proxy nor the HTTP(S)_PROXY environment variables
// apply. AI calls, enrichment, catalog sync, icon fetching etc. stay on the
// direct path regardless of proxy configuration (scope narrowing).
var sharedDirect = sync.OnceValue(func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return t
})

// Option configures the *http.Client returned by New.
type Option func(*http.Client)

// WithTimeout sets the client Timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *http.Client) {
		c.Timeout = d
	}
}

// WithTransport sets a custom base RoundTripper. When instrumentation is on,
// this is wrapped by otelhttp; otherwise used as-is. An explicit transport
// overrides both the crawling scope and the global proxy (caller takes
// responsibility for proxying).
func WithTransport(rt http.RoundTripper) Option {
	return func(c *http.Client) {
		if rt != nil {
			c.Transport = rt
		}
	}
}

// WithCrawlScope marks the client as belonging to the external-content
// crawling scope (feed fetching, article extraction, upstream image pulls):
// it routes through the global outbound proxy (SetProxy / http_proxy_url)
// with circuit-breaker failover, falling back to the HTTP(S)_PROXY env vars
// when no proxy is configured. Without this option New returns an always-
// direct client that never consults the proxy or the env vars.

func WithCrawlScope() Option {
	return func(c *http.Client) {
		if c.Transport == nil {
			c.Transport = packageTransport()
		}
	}
}

// New returns an *http.Client. When instrumentation is enabled, its transport
// is wrapped with otelhttp.NewTransport so outbound calls produce
// SpanKind=Client spans and propagate traceparent.
//
// Default (no WithCrawlScope): an always-direct transport — stdlib defaults
// with proxying disabled entirely, regardless of SetProxy or the
// HTTP(S)_PROXY env vars. This is the scope narrowing contract: AI calls,
// enrichment, catalog sync etc. never touch the outbound proxy.
//
// With WithCrawlScope: the package-level failover transport is used — it
// routes through the globally configured proxy (SetProxy) with circuit-
// breaker failover to direct when the proxy is unreachable, and honours
// http.ProxyFromEnvironment when no proxy is configured. Because the
// transport is shared and reads the proxy URL per request, SetProxy changes
// affect every crawl-scoped client built earlier as well.
func New(opts ...Option) *http.Client {
	c := &http.Client{}
	for _, opt := range opts {
		opt(c)
	}
	base := c.Transport
	if base == nil {
		base = sharedDirect()
	}
	if instrumentHTTP {
		c.Transport = otelhttp.NewTransport(base)
	} else {
		c.Transport = base
	}
	return c
}
