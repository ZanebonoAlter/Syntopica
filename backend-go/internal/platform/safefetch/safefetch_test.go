package safefetch

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// loopbackAllowed returns Options that explicitly allow 127.0.0.0/8 — the
// default policy rejects loopback, so tests against httptest servers (which
// listen on 127.0.0.1) must opt in via AllowedIPs.
func loopbackAllowed(t *testing.T) Options {
	t.Helper()
	_, cidr, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	return Options{AllowedIPs: []*net.IPNet{cidr}}
}

func TestFetchSuccess200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		fmt.Fprint(w, "<rss>hello</rss>")
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL+"/feed.xml", loopbackAllowed(t))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "<rss>hello</rss>", string(res.Body))
	require.Equal(t, srv.URL+"/feed.xml", res.FinalURL)
	require.Equal(t, "application/rss+xml; charset=utf-8", res.ContentType)
	require.Equal(t, 0, res.Redirects)
}

func TestFetchHTTPStatusIsResultNotError(t *testing.T) {
	// 非 2xx 属于 HTTP 语义，由上层按 StatusCode 判定，不是 Fetch 错误。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL+"/missing", loopbackAllowed(t))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestFetchRetryAfterHeader（improve-discovery-recommendations 4.6）：429 的
// Retry-After 按 delta-seconds 透出，非整数/负数/缺失一律 0——上层据此回退默认
// 复查周期，不得把不可解析的头当成 0 秒立即重试。
func TestFetchRetryAfterHeader(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{name: "seconds", header: "120", want: 120},
		{name: "surrounding spaces", header: " 7 ", want: 7},
		{name: "http-date form falls back to zero", header: "Wed, 21 Oct 2026 07:28:00 GMT", want: 0},
		{name: "zero is not a usable delay", header: "0", want: 0},
		{name: "negative", header: "-5", want: 0},
		{name: "absent", header: "", want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.header != "" {
					w.Header().Set("Retry-After", tc.header)
				}
				w.WriteHeader(http.StatusTooManyRequests)
			}))
			defer srv.Close()

			res, err := Fetch(context.Background(), srv.URL+"/limited", loopbackAllowed(t))
			require.NoError(t, err)
			require.Equal(t, http.StatusTooManyRequests, res.StatusCode)
			require.Equal(t, tc.want, res.RetryAfterSeconds)
		})
	}
}

func TestFetchRedirectToLinkLocalMetadataBlocked(t *testing.T) {
	// 第一跳（127.0.0.1）已被 AllowedIPs 放行；重定向到云元数据地址
	// 169.254.169.254 必须在第二跳被拒——证明校验逐跳生效（S12 主链路步 3）。
	var redirectCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/public") {
			redirectCount++
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
			return
		}
		fmt.Fprint(w, "unexpected target reached")
	}))
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL+"/public/feed", loopbackAllowed(t))
	require.Error(t, err)
	require.Nil(t, res)
	require.ErrorIs(t, err, ErrPrivateAddress)

	var pae *PrivateAddressError
	require.True(t, errors.As(err, &pae))
	require.Equal(t, "169.254.169.254", pae.IP.String())

	// 脱敏：错误信息只含被拒 IP，不含完整原 URL 的 path/query。
	require.Contains(t, err.Error(), "169.254.169.254")
	require.NotContains(t, err.Error(), "meta-data")
	require.NotContains(t, err.Error(), srv.URL+"/public/feed")
	require.Equal(t, 1, redirectCount, "first hop must have been allowed and fetched")
}

func TestFetchPrivateURLErrorSanitized(t *testing.T) {
	// 直接访问未授权私网地址：错误可识别且脱敏（含 IP，不含 path/query）。
	_, err := Fetch(context.Background(), "http://10.0.0.7/internal/feed?token=abc123", Options{})
	require.ErrorIs(t, err, ErrPrivateAddress)
	require.Contains(t, err.Error(), "10.0.0.7")
	require.NotContains(t, err.Error(), "token=abc123")
	require.NotContains(t, err.Error(), "/internal/feed")
}

func TestFetchIPv6LoopbackBlocked(t *testing.T) {
	_, err := Fetch(context.Background(), "http://[::1]:8080/feed", Options{})
	require.ErrorIs(t, err, ErrPrivateAddress)
}

func redirectChainServer(t *testing.T, hops int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 路径 /r<N>：N < hops 时跳到 /r<N+1>，否则 200。
		n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/r"))
		if err != nil || n >= hops {
			fmt.Fprint(w, "done")
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/r%d", n+1), http.StatusFound)
	}))
}

func TestFetchRedirectsWithinLimit(t *testing.T) {
	// 3 跳（默认上限内）应成功，Redirects 计数为 3。
	srv := redirectChainServer(t, 3)
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL+"/r0", loopbackAllowed(t))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "done", string(res.Body))
	require.Equal(t, 3, res.Redirects)
	require.True(t, strings.HasSuffix(res.FinalURL, "/r3"))
}

func TestFetchTooManyRedirects(t *testing.T) {
	// 4 跳超过默认 MaxRedirects=3，第 4 跳前被拒绝。
	srv := redirectChainServer(t, 4)
	defer srv.Close()

	res, err := Fetch(context.Background(), srv.URL+"/r0", loopbackAllowed(t))
	require.Error(t, err)
	require.Nil(t, res)
	require.ErrorIs(t, err, ErrTooManyRedirects)
}

func TestFetchBodyTooLarge(t *testing.T) {
	bodyLen := 40 // handler 闭包捕获，子场景下调
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Repeat("x", bodyLen))
	}))
	defer srv.Close()

	opts := loopbackAllowed(t)
	opts.MaxBytes = 16

	// 超限：返回 ErrBodyTooLarge，不返回部分内容。
	res, err := Fetch(context.Background(), srv.URL+"/big", opts)
	require.ErrorIs(t, err, ErrBodyTooLarge)
	require.Nil(t, res)

	// 恰好等于上限：不超限（边界值）。
	bodyLen = 16
	res, err = Fetch(context.Background(), srv.URL+"/exact", opts)
	require.NoError(t, err)
	require.Len(t, res.Body, 16)
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		fmt.Fprint(w, "late")
	}))
	defer srv.Close()

	opts := loopbackAllowed(t)
	opts.Timeout = 100 * time.Millisecond

	res, err := Fetch(context.Background(), srv.URL+"/slow", opts)
	require.Error(t, err)
	require.Nil(t, res)
	require.ErrorIs(t, err, ErrTimeout)
}

func TestFetchUnsupportedScheme(t *testing.T) {
	for _, raw := range []string{
		"ftp://example.com/feed.xml",
		"gopher://example.com/feed",
		"file:///etc/passwd",
		"example.com/feed",
	} {
		res, err := Fetch(context.Background(), raw, Options{})
		require.ErrorIs(t, err, ErrUnsupportedScheme, "scheme must be rejected: %s", raw)
		require.Nil(t, res)
	}
}

func TestFetchHTTPS(t *testing.T) {
	// 拨号目标是校验过的 IP，但 TLS SNI/证书校验仍锚定原 hostname：
	// httptest TLS 证书签给 127.0.0.1，信任它后握手必须成功。
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		fmt.Fprint(w, "<feed/>")
	}))
	defer srv.Close()

	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	opts := loopbackAllowed(t)
	opts.TLSClientConfig = &tls.Config{RootCAs: pool}

	res, err := Fetch(context.Background(), srv.URL+"/atom", opts)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "application/atom+xml", res.ContentType)
}

func TestIsAllowed(t *testing.T) {
	_, loopback, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)

	tests := []struct {
		name    string
		ip      string
		allowed []*net.IPNet
		want    bool
	}{
		{"public v4", "8.8.8.8", nil, true},
		{"public v6", "2606:4700::1111", nil, true},
		{"loopback v4", "127.0.0.1", nil, false},
		{"loopback v4 high", "127.255.255.254", nil, false},
		{"rfc1918 10/8", "10.1.2.3", nil, false},
		{"rfc1918 172.16/12", "172.16.0.1", nil, false},
		{"rfc1918 172.31 upper", "172.31.255.254", nil, false},
		{"rfc1918 192.168/16", "192.168.1.1", nil, false},
		{"cloud metadata link-local", "169.254.169.254", nil, false},
		{"link-local other", "169.254.0.1", nil, false},
		{"unspecified v4", "0.0.0.0", nil, false},
		{"this-network 0/8", "0.1.2.3", nil, false},
		{"multicast v4", "224.0.0.1", nil, false},
		{"ssdp multicast", "239.255.255.250", nil, false},
		{"broadcast", "255.255.255.255", nil, false},
		{"class E reserved", "240.0.0.1", nil, false},
		{"cgnat 100.64/10", "100.64.0.1", nil, false},
		{"cgnat upper bound", "100.127.255.254", nil, false},
		{"public 100.128 above cgnat", "100.128.0.1", nil, true},
		{"loopback v6", "::1", nil, false},
		{"ula fc00", "fc00::1", nil, false},
		{"ula fd00 (ec2 ipv6 metadata)", "fd00:ec2::254", nil, false},
		{"link-local v6", "fe80::1", nil, false},
		{"multicast v6", "ff02::1", nil, false},
		{"v4-mapped loopback", "::ffff:127.0.0.1", nil, false},
		{"v4-mapped private", "::ffff:10.0.0.1", nil, false},
		{"whitelisted loopback", "127.0.0.1", []*net.IPNet{loopback}, true},
		{"whitelist does not open others", "10.0.0.1", []*net.IPNet{loopback}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsAllowed(net.ParseIP(tt.ip), tt.allowed))
		})
	}
}

func TestOptionsDefaults(t *testing.T) {
	// 零值 Options 必须落到包默认（10s / 2MiB / 3 跳）。
	got := Options{}.withDefaults()
	require.Equal(t, DefaultTimeout, got.Timeout)
	require.Equal(t, DefaultMaxBytes, got.MaxBytes)
	require.Equal(t, DefaultMaxRedirects, got.MaxRedirects)
}
