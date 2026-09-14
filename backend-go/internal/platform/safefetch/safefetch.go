// Package safefetch provides an SSRF-hardened HTTP fetcher for untrusted URLs
// (RSS feed availability checks, catalog probes). It is deliberately separate
// from internal/platform/httpclient: that factory serves trusted first-party
// outbound calls (proxies, tracing), while this package enforces the
// private-source safety contract of openspec change improve-discovery-
// recommendations (design.md D7):
//
//   - only http/https schemes;
//   - every hop (initial URL and each redirect target) re-resolves DNS and ALL
//     resolved IPs must pass IsAllowed — loopback/RFC1918/link-local (incl.
//     cloud metadata 169.254.169.254)/multicast/unspecified and their IPv6
//     counterparts are rejected unless explicitly whitelisted via AllowedIPs;
//   - the TCP connection dials the exact validated IP (not the hostname),
//     closing the DNS-rebinding TOCTOU window between validation and dial;
//   - environment proxies are ignored (Transport.Proxy = nil) so a proxy
//     cannot bypass the IP checks;
//   - redirects are validated and counted per hop, exceeding MaxRedirects
//     fails with ErrTooManyRedirects;
//   - response bodies are capped at MaxBytes (ErrBodyTooLarge beyond);
//   - timeouts surface as ErrTimeout (errors.Is-identifiable).
package safefetch

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 默认限制（design.md D7：单次超时 10 秒、响应上限 2MiB、重定向最多 3 次）。
const (
	DefaultTimeout      = 10 * time.Second
	DefaultMaxBytes     = int64(2 << 20) // 2 MiB
	DefaultMaxRedirects = 3
)

// Options configures a single Fetch call. Zero values select the defaults
// above; AllowedIPs is the explicit opt-in for otherwise-blocked ranges
// (e.g. a user-authorized self-hosted intranet endpoint). Authorization is
// per-request, never global — private/loopback addresses stay rejected by
// default.
type Options struct {
	// Timeout bounds the whole exchange (connect + response + body read).
	Timeout time.Duration
	// MaxBytes caps the response body; bodies larger than this fail with
	// ErrBodyTooLarge.
	MaxBytes int64
	// MaxRedirects is the maximum number of redirects followed.
	MaxRedirects int
	// AllowedIPs explicitly whitelists network ranges that the default policy
	// would reject (loopback, RFC1918, link-local, ...).
	AllowedIPs []*net.IPNet
	// TLSClientConfig, when non-nil, replaces the default TLS config
	// (used by tests to trust self-signed httptest certificates).
	TLSClientConfig *tls.Config
}

// Result is the bounded response of a successful fetch. Any HTTP status —
// including 4xx/5xx — is returned as a Result with nil error: transport-level
// safety violations and size/timeout violations are errors, HTTP semantics
// are the caller's business.
type Result struct {
	StatusCode  int
	Body        []byte
	FinalURL    string
	ContentType string
	Redirects   int
	// RetryAfterSeconds is the Retry-After header parsed as delta-seconds
	// (0 when the header is absent, not an integer, or negative). §429 handling
	// of the discovery availability state machine consumes it to bound the
	// next-check delay (design.md D7: 429 按 Retry-After 有界推迟).
	RetryAfterSeconds int
}

// parseRetryAfter parses the delta-seconds form of Retry-After. The HTTP-date
// form is intentionally not parsed — callers fall back to their default
// delay when the value is not a positive integer.
func parseRetryAfter(value string) int {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0
	}
	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

func (o Options) withDefaults() Options {
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = DefaultMaxRedirects
	}
	return o
}

// IsAllowed reports whether ip may be fetched. It returns true only when ip
// falls into one of the explicitly allowed ranges, or when ip is a public
// unicast address. Everything else is rejected: loopback, RFC1918 private,
// ULA (fc00::/7), link-local unicast/multicast (169.254.0.0/16 covers the
// cloud metadata address 169.254.169.254, fe80::/10), multicast, unspecified,
// 0.0.0.0/8, 100.64.0.0/10 (CGNAT), broadcast and other reserved ranges.
// IPv4-mapped IPv6 addresses are unwrapped and judged as IPv4.
func IsAllowed(ip net.IP, allowed []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, n := range allowed {
		if n != nil && n.Contains(ip) {
			return true
		}
	}
	if v4 := ip.To4(); v4 != nil {
		switch {
		case v4[0] == 0: // 0.0.0.0/8 "this network"
			return false
		case v4[0] == 100 && v4[1]&0xc0 == 0x40: // 100.64.0.0/10 CGNAT shared range
			return false
		case v4[0]&0xf0 == 0xf0: // 240.0.0.0/4 class E reserved (incl. 255.255.255.255)
			return false
		}
	}
	switch {
	case ip.IsLoopback():
		return false
	case ip.IsPrivate():
		return false
	case ip.IsLinkLocalUnicast():
		return false
	case ip.IsLinkLocalMulticast():
		return false
	case ip.IsInterfaceLocalMulticast():
		return false
	case ip.IsMulticast():
		return false
	case ip.IsUnspecified():
		return false
	}
	return true
}

// Fetch performs a GET against rawURL under the Options safety policy.
// See the package comment for the enforced guarantees.
func Fetch(ctx context.Context, rawURL string, opts Options) (*Result, error) {
	cfg := opts.withDefaults()

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("safefetch: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrUnsupportedScheme
	}

	redirects := 0
	transport := &http.Transport{
		// 显式 Proxy(nil)：置 nil 覆盖默认的 ProxyFromEnvironment，环境代理
		// 不得把连接转交代理服务器而绕过本包的 IP 校验（design.md D7）。
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialValidated(ctx, network, addr, cfg)
		},
		TLSClientConfig: cfg.TLSClientConfig,
	}

	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrUnsupportedScheme
			}
			if len(via) > cfg.MaxRedirects {
				return ErrTooManyRedirects
			}
			redirects = len(via)
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("safefetch: build request: %w", err)
	}
	req.Header.Set("User-Agent", "Syntopica-SafeFetch/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, normalizeError(err, cfg.Timeout)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxBytes+1))
	if err != nil {
		return nil, normalizeError(err, cfg.Timeout)
	}
	if int64(len(body)) > cfg.MaxBytes {
		return nil, ErrBodyTooLarge
	}

	return &Result{
		StatusCode:  resp.StatusCode,
		Body:        body,
		FinalURL:    resp.Request.URL.String(),
		ContentType: resp.Header.Get("Content-Type"),
		Redirects:   redirects,

		RetryAfterSeconds: parseRetryAfter(resp.Header.Get("Retry-After")),
	}, nil
}

// dialValidated implements the per-hop safety gate: resolve the hostname,
// require every resolved IP to pass IsAllowed, then dial one of the validated
// IPs directly. Dialing the already-validated IP (instead of the hostname,
// which would re-resolve DNS at connect time) closes the classic DNS-rebinding
// race where the first lookup returns a public IP and the second a private one.
// TLS handshake/SNI/certificate validation stay keyed on the original hostname
// because the http.Transport still believes it dialed the hostname.
func dialValidated(ctx context.Context, network, addr string, cfg Options) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("safefetch: invalid dial address: %w", err)
	}

	resolver := net.DefaultResolver
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("safefetch: DNS lookup %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("safefetch: no addresses resolved for %q", host)
	}
	for _, ia := range ips {
		if !IsAllowed(ia.IP, cfg.AllowedIPs) {
			return nil, &PrivateAddressError{IP: ia.IP}
		}
	}

	dialer := &net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ia := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ia.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

// normalizeError unwraps *url.Error (and friends) into the typed sentinel
// errors. PrivateAddressError is returned bare on purpose: url.Error's message
// embeds the full request URL, which for a blocked private target is exactly
// what must not leak.
func normalizeError(err error, timeout time.Duration) error {
	if err == nil {
		return nil
	}
	var pae *PrivateAddressError
	if errors.As(err, &pae) {
		return pae
	}
	if errors.Is(err, ErrTooManyRedirects) {
		return ErrTooManyRedirects
	}
	if errors.Is(err, ErrUnsupportedScheme) {
		return ErrUnsupportedScheme
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w (after %s)", ErrTimeout, timeout)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w (after %s)", ErrTimeout, timeout)
	}
	return err
}
