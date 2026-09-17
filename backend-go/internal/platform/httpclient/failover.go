// Failover transport for the globally configured outbound proxy.
//
// A dead proxy (process down, port refusing) used to fail EVERY outbound
// request at the dial step — feed fetch, Firecrawl, LLM calls alike — even
// when the target is reachable directly. The failover transport adds two
// behaviours on top of the plain configured proxy:
//
//   - Circuit breaker: a dial to the proxy address itself failing marks the
//     proxy down for proxyFailoverWindow; the failing request is retried
//     once, directly (safe for every method: a dial failure means nothing
//     was sent). While the breaker is open, requests skip the proxy
//     entirely — no proxy dial latency.
//   - Lazy probe recovery: when the window expires, exactly one in-flight
//     request (single-flight under the breaker mutex) retries the proxy.
//     Any non-dial outcome (target 404/502/timeout through a reachable
//     proxy) proves the proxy path alive and closes the breaker; another
//     dial failure extends the window. Proxy reachability — not target
//     reachability — is the health signal.
//
// The proxy address is stored atomically (see SetProxy), so every runtime
// change is visible to all clients built via New immediately, without a
// restart; changing or clearing the address resets the breaker.

package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// proxyFailoverWindow is how long the breaker stays open (all requests go
// direct) after a dial-to-proxy failure, before a single probe request is
// allowed to retry the proxy. Package var so tests can shorten it.
var proxyFailoverWindow = 60 * time.Second

// failoverTransport routes requests through the configured proxy while a
// breaker watches proxy health; it is the package-level transport shared by
// every client built via New without an explicit transport.
type failoverTransport struct {
	// proxy dials through the currently configured proxy URL. Its Proxy
	// function is a pure routing decision; breaker state transitions live
	// in RoundTrip (single writer, see tryAcquireProxy).
	proxy *http.Transport
	// direct dials without the configured proxy. It keeps the standard
	// ProxyFromEnvironment so the unconfigured behaviour (HTTP_PROXY /
	// HTTPS_PROXY env fallback, loopback never proxied) is preserved, both
	// for the no-proxy route and for breaker fallback retries.
	direct *http.Transport

	breaker proxyBreaker
}

// newFailoverTransport clones the default transport settings so both routes
// keep the stdlib TLS/idle defaults; the proxy route's dialer tags failures
// addressed to the proxy itself (see proxyDialContext).
func newFailoverTransport() *failoverTransport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// Unreachable with the stdlib DefaultTransport; defensive clone
		// target so the construction below cannot nil-panic.
		base = &http.Transport{}
	}
	proxy := base.Clone()
	proxy.Proxy = proxyRouteFunc
	proxy.DialContext = proxyDialContext((&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext)
	direct := base.Clone() // keeps ProxyFromEnvironment and the default dialer
	return &failoverTransport{proxy: proxy, direct: direct}
}

var (
	transportOnce   sync.Once
	sharedTransport *failoverTransport
)

// packageTransport returns the process-wide failover transport. All clients
// built via New without an explicit transport share it, which is what makes
// SetProxy changes effective for already-built clients.
func packageTransport() *failoverTransport {
	transportOnce.Do(func() { sharedTransport = newFailoverTransport() })
	return sharedTransport
}

// proxyRouteFunc is the Proxy function of the proxy route: loopback targets
// are never proxied (local model servers must not be intercepted, see
// isLoopbackHost); otherwise the currently configured proxy URL wins. The
// ProxyFromEnvironment fallback only applies when no proxy is configured,
// mirroring the previous DefaultTransport behaviour.
func proxyRouteFunc(req *http.Request) (*url.URL, error) {
	if isLoopbackHost(req.URL.Hostname()) {
		return nil, nil
	}
	if u := currentProxyURL(); u != nil {
		return u, nil
	}
	return http.ProxyFromEnvironment(req)
}

// RoundTrip implements http.RoundTripper with the breaker state machine:
//
//	healthy ──dial-to-proxy failure──► open (downUntil=now+window)
//	    ▲                                  │
//	    │ probe result: proxy path alive   │ window expired: single-flight
//	    └──────────────────────────────────┘ probe (others go direct)
//
// Target-side failures through a reachable proxy (502, CONNECT refused,
// target timeout) never open the breaker: such targets may need the proxy,
// and a direct retry would not do better.
func (f *failoverTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return f.direct.RoundTrip(req)
	}
	proxyURL := currentProxyURL()
	if proxyURL == nil || isLoopbackHost(req.URL.Hostname()) || !f.breaker.tryAcquireProxy() {
		return f.direct.RoundTrip(req)
	}
	resp, err := f.proxy.RoundTrip(req)
	if err == nil {
		f.breaker.proxyAlive()
		return resp, nil
	}
	if isProxyDialFailure(err) {
		f.breaker.proxyDown()
		// Dial-level failure: the request never left, so a direct retry is
		// safe for every method. The retry result is the final result.
		return f.direct.RoundTrip(req)
	}
	// Reachable proxy, target-side failure: the proxy path is alive.
	f.breaker.proxyAlive()
	return resp, err
}

// proxyBreaker is the three-state breaker (healthy / open / probing). All
// transitions happen under one mutex; the cost is one lock per request.
type proxyBreaker struct {
	mu        sync.Mutex
	downUntil time.Time // zero = healthy
	probing   bool      // a window-expiry probe is in flight
}

// tryAcquireProxy reports whether this attempt may use the proxy. In the
// healthy state every attempt may. In the open state none may. In the
// expired state exactly one attempt becomes the prober (single-flight):
// concurrent requests go direct instead of stampeding the dead proxy.
func (b *proxyBreaker) tryAcquireProxy() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.downUntil.IsZero() {
		return true
	}
	if time.Now().Before(b.downUntil) {
		return false
	}
	if b.probing {
		return false
	}
	b.probing = true
	return true
}

// proxyDown records a dial-to-proxy failure: the breaker opens (or extends)
// for a full window and the probe slot is released. In-flight probes that
// raced a concurrent reset may leave probing set until their own result
// arrives; both result handlers clear it, so no state can stick.
func (b *proxyBreaker) proxyDown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.downUntil = time.Now().Add(proxyFailoverWindow)
	b.probing = false
}

// proxyAlive records evidence that the proxy path is reachable (any dial to
// the proxy succeeded): the breaker closes.
func (b *proxyBreaker) proxyAlive() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.downUntil = time.Time{}
	b.probing = false
}

// reset re-arms the breaker to healthy, used when the proxy configuration
// changes: a new address is treated as healthy.
func (b *proxyBreaker) reset() {
	b.proxyAlive()
}

// proxyDialError marks a failed dial whose target address was the proxy
// address. Tagging at the dial layer — instead of inspecting error chains —
// is scheme-independent and immune to how net/http nests proxy dial errors
// (all proxy dials surface as `proxyconnect tcp: ...`, hiding the inner
// dial OpError from a first-match errors.As).
type proxyDialError struct {
	addr string
	err  error
}

func (e *proxyDialError) Error() string { return "proxy dial " + e.addr + ": " + e.err.Error() }
func (e *proxyDialError) Unwrap() error { return e.err }

// proxyDialContext wraps a dialer so failures whose dial target is the
// currently configured proxy are tagged with proxyDialError.
func proxyDialContext(next func(ctx context.Context, network, addr string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := next(ctx, network, addr)
		if err != nil {
			if pu := currentProxyURL(); pu != nil && dialAddrMatchesProxy(addr, pu) {
				return conn, &proxyDialError{addr: addr, err: err}
			}
		}
		return conn, err
	}
}

// isProxyDialFailure reports whether err is a network-layer dial failure
// whose target was the proxy address itself (connection refused, dial
// timeout). Failures past the dial — CONNECT refusals, target errors, TLS
// alerts from a listening proxy — are proxy-alive signals, not proxy-down
// signals.
func isProxyDialFailure(err error) bool {
	var pde *proxyDialError
	return errors.As(err, &pde)
}

// dialAddrMatchesProxy compares the pre-resolution dial address against the
// proxy URL (host case-insensitive, port with scheme defaults applied for
// URLs given without an explicit port).
func dialAddrMatchesProxy(addr string, proxy *url.URL) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return strings.EqualFold(host, proxy.Hostname()) && port == defaultProxyPort(proxy)
}

func defaultProxyPort(proxy *url.URL) string {
	if p := proxy.Port(); p != "" {
		return p
	}
	switch proxy.Scheme {
	case "https":
		return "443"
	case "socks5":
		return "1080"
	default:
		return "80"
	}
}
