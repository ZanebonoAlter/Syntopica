package httpclient

// Tests for the proxy failover behaviour: circuit breaker on dial-to-proxy
// failure with direct fallback, single-flight probe recovery, error
// classification (only dial-to-proxy failures trip the breaker) and runtime
// proxy reconfiguration taking effect on already-built clients.
//
// All tests are sequential (no t.Parallel): they white-box the shared
// package transport (dial interception, breaker inspection) and mutate
// global proxy state; every test isolates itself via resetProxyState.

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// resetProxyState isolates a test from package-level proxy state: the
// cleanup clears the configured proxy (which also resets the breaker) and
// restores the default failover window.
func resetProxyState(t *testing.T) {
	t.Helper()
	oldWindow := proxyFailoverWindow
	t.Cleanup(func() {
		proxyFailoverWindow = oldWindow
		_ = SetProxy("")
	})
}

// freePortAddr returns a 127.0.0.1:<free port> address: nothing listens
// there, so dialing it fails with a genuine connection refusal.
func freePortAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// nonLoopbackLocalIP returns a local IPv4 address that is not loopback.
// Proxy routing bypasses loopback targets by design, so exercising the
// proxy path (and its failover) requires a non-loopback target host.
func nonLoopbackLocalIP(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatalf("interface addrs: %v", err)
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() {
			continue
		}
		return ipnet.IP.String()
	}
	t.Skip("no non-loopback IPv4 interface address available")
	return ""
}

// serveOnAllInterfaces starts an httptest server bound to all interfaces
// (httptest's default binds 127.0.0.1 only, which proxy routing bypasses)
// and returns its URL rewritten to the non-loopback local address.
func serveOnAllInterfaces(t *testing.T, handler http.Handler) string {
	t.Helper()
	return rewriteToNonLoopback(t, startServer(t, handler, false))
}

// serveTLSOnAllInterfaces is serveOnAllInterfaces for TLS (https) targets.
func serveTLSOnAllInterfaces(t *testing.T, handler http.Handler) string {
	t.Helper()
	return rewriteToNonLoopback(t, startServer(t, handler, true))
}

func startServer(t *testing.T, handler http.Handler, useTLS bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		srv.Close()
		t.Fatalf("listen: %v", err)
	}
	srv.Listener = l
	if useTLS {
		srv.StartTLS()
	} else {
		srv.Start()
	}
	t.Cleanup(srv.Close)
	return srv
}

func rewriteToNonLoopback(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse %q: %v", srv.URL, err)
	}
	u.Host = net.JoinHostPort(nonLoopbackLocalIP(t), u.Port())
	return u.String()
}

// forwardingProxy is a minimal forward proxy for absolute-form http
// requests; every accepted request bumps hits so tests can assert how often
// the proxy was actually used.
func forwardingProxy(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method == http.MethodConnect || !r.URL.IsAbs() {
			http.Error(w, "test forward proxy only relays absolute-form http", http.StatusBadRequest)
			return
		}
		outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		resp, err := http.DefaultTransport.RoundTrip(outReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// countProxyDials wraps the shared proxy dialer so tests can assert exactly
// how many dial attempts were made towards the proxy (real dials preserved).
func countProxyDials(t *testing.T) *atomic.Int32 {
	t.Helper()
	pt := packageTransport()
	var n atomic.Int32
	orig := pt.proxy.DialContext
	if orig == nil {
		t.Fatal("proxy transport has no DialContext")
	}
	pt.proxy.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		n.Add(1)
		return orig(ctx, network, addr)
	}
	t.Cleanup(func() { pt.proxy.DialContext = orig })
	return &n
}

// failProxyDials swaps the shared proxy dialer for one that fails every dial
// to the proxy address with a dial-shaped *net.OpError — a deterministic
// outage without close-then-relisten port races. Dials to any other address
// (e.g. after a runtime SetProxy swap to a different proxy) pass through to
// the original dialer. It counts dial attempts so tests can assert
// single-flight probing ("exactly one probe") and zero-dial windows.
func failProxyDials(t *testing.T) (dials *atomic.Int32, restore func()) {
	t.Helper()
	pt := packageTransport()
	pu := currentProxyURL()
	if pu == nil {
		t.Fatal("configure the proxy before failing its dials")
	}
	host, port := pu.Hostname(), defaultProxyPort(pu)
	tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, port))
	if err != nil {
		t.Fatalf("resolve proxy addr: %v", err)
	}
	var n atomic.Int32
	orig := pt.proxy.DialContext
	if orig == nil {
		t.Fatal("proxy transport has no DialContext")
	}
	pt.proxy.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		n.Add(1)
		aHost, aPort, splitErr := net.SplitHostPort(addr)
		if splitErr == nil && strings.EqualFold(aHost, host) && aPort == port {
			// Same tagging as the production dialer wrapper so the
			// classifier sees the exact shape it is meant to catch.
			return nil, &proxyDialError{
				addr: addr,
				err: &net.OpError{
					Op:   "dial",
					Net:  network,
					Addr: tcpAddr,
					Err:  errors.New("connection refused (test outage)"),
				},
			}
		}
		return orig(ctx, network, addr)
	}
	var once sync.Once
	restore = func() { once.Do(func() { pt.proxy.DialContext = orig }) }
	t.Cleanup(restore)
	return &n, restore
}

// allowInsecureTLS lets the shared transports accept httptest self-signed
// certificates (needed for https/CONNECT targets).
func allowInsecureTLS(t *testing.T) {
	t.Helper()
	pt := packageTransport()
	for _, tr := range []*http.Transport{pt.proxy, pt.direct} {
		old := tr.TLSClientConfig
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // self-signed httptest cert
		t.Cleanup(func() { tr.TLSClientConfig = old })
	}
}

// breakerOpen reads breaker state (white-box, same package): true while
// requests bypass the proxy.
func breakerOpen() bool {
	b := &packageTransport().breaker
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.downUntil.IsZero() && time.Now().Before(b.downUntil)
}

// TestProxyDialFailureFallsBackToDirect — spec「代理不可达时熔断降级直连 /
// 代理挂掉时直连可达源正常抓取」：端到端真拒绝（空闲端口），首个请求必须
// 无感直连重试并成功；失败分类正确（OpError dial → 代理地址）打开熔断。
func TestProxyDialFailureFallsBackToDirect(t *testing.T) {
	resetProxyState(t)
	var targetHits atomic.Int32
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits.Add(1)
		_, _ = w.Write([]byte("direct-ok"))
	}))
	if err := SetProxy("http://" + freePortAddr(t)); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}

	client := New(WithTimeout(5 * time.Second))
	resp, err := client.Get(targetURL + "/feed.xml")
	if err != nil {
		t.Fatalf("GET through dead proxy must fall back to direct: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "direct-ok" {
		t.Fatalf("status=%d body=%q, want 200/direct-ok", resp.StatusCode, body)
	}
	if targetHits.Load() != 1 {
		t.Fatalf("target hits = %d, want 1", targetHits.Load())
	}
	if !breakerOpen() {
		t.Fatal("breaker must be open after a proxy dial failure")
	}
}

// TestCircuitOpenSkipsProxyDial — spec「熔断窗口内零代理拨号」：熔断打开后
// 窗口内的请求不得再向代理发起任何拨号（机械断言：拨号计数冻结在 1）。
func TestCircuitOpenSkipsProxyDial(t *testing.T) {
	resetProxyState(t)
	proxyFailoverWindow = 500 * time.Millisecond
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err := SetProxy("http://" + freePortAddr(t)); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}

	dials := countProxyDials(t)
	client := New(WithTimeout(2 * time.Second))
	for i := 0; i < 3; i++ {
		resp, err := client.Get(targetURL)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i, resp.StatusCode)
		}
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("proxy dial attempts = %d, want 1 (only the first request may dial the dead proxy)", got)
	}
	if !breakerOpen() {
		t.Fatal("breaker should still be open inside the window")
	}
}

// TestProbeSuccessReopensCircuit — spec「代理恢复后自动接回」：窗口到期后
// 试探请求成功即关闭熔断，后续流量回到代理。
func TestProbeSuccessReopensCircuit(t *testing.T) {
	resetProxyState(t)
	proxyFailoverWindow = 100 * time.Millisecond
	var hits atomic.Int32
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("via-proxy"))
	}))
	proxy := forwardingProxy(t, &hits)
	if err := SetProxy(proxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	_, restore := failProxyDials(t)

	client := New(WithTimeout(2 * time.Second))
	resp1, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("outage-phase request: %v", err)
	}
	_ = resp1.Body.Close()
	if hits.Load() != 0 || !breakerOpen() {
		t.Fatalf("outage phase: proxy hits=%d breakerOpen=%v, want 0/true", hits.Load(), breakerOpen())
	}

	restore() // 「代理进程回来了」：同一地址恢复可达
	time.Sleep(120 * time.Millisecond)

	resp2, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("post-recovery request: %v", err)
	}
	_ = resp2.Body.Close()
	if hits.Load() == 0 {
		t.Fatal("request after recovery must go through the proxy again")
	}
	if breakerOpen() {
		t.Fatal("breaker must close after a successful probe")
	}
}

// TestProbeFailureExtendsWindow — spec「试探失败续期」：窗口到期试探仍拨号
// 失败 → 该请求直连成功不报错，熔断续满一个窗口。
func TestProbeFailureExtendsWindow(t *testing.T) {
	resetProxyState(t)
	proxyFailoverWindow = 100 * time.Millisecond
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	proxy := forwardingProxy(t, &atomic.Int32{})
	if err := SetProxy(proxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	dials, _ := failProxyDials(t)

	client := New(WithTimeout(2 * time.Second))
	resp1, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	_ = resp1.Body.Close()
	if got := dials.Load(); got != 1 {
		t.Fatalf("dials after first request = %d, want 1", got)
	}

	time.Sleep(120 * time.Millisecond)
	resp2, err := client.Get(targetURL) // probe fails → extend + direct retry
	if err != nil {
		t.Fatalf("probe request must not surface the proxy failure: %v", err)
	}
	_ = resp2.Body.Close()
	if got := dials.Load(); got != 2 {
		t.Fatalf("dials after probe = %d, want 2 (initial + exactly one probe)", got)
	}
	if !breakerOpen() {
		t.Fatal("breaker must be re-armed after a failed probe")
	}

	resp3, err := client.Get(targetURL) // inside extended window: direct, no dial
	if err != nil {
		t.Fatalf("post-extend request: %v", err)
	}
	_ = resp3.Body.Close()
	if got := dials.Load(); got != 2 {
		t.Fatalf("dials after extended-window request = %d, want 2 (zero proxy dials inside window)", got)
	}
}

// TestProbeSingleFlightConcurrentDirect — spec「试探期间并发请求直连」：
// 窗口到期并发 N 请求、代理不可达 → 恰一次试探拨号，N 个请求全部成功。
func TestProbeSingleFlightConcurrentDirect(t *testing.T) {
	resetProxyState(t)
	proxyFailoverWindow = 200 * time.Millisecond
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	proxy := forwardingProxy(t, &atomic.Int32{})
	if err := SetProxy(proxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	dials, _ := failProxyDials(t)

	client := New(WithTimeout(2 * time.Second))
	resp1, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	_ = resp1.Body.Close()

	time.Sleep(220 * time.Millisecond) // window expires

	const n = 10
	errs := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(targetURL)
			if err != nil {
				errs <- err
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errs <- errors.New("status not ok")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent request failed: %v", err)
	}
	if got := dials.Load(); got != 2 {
		t.Fatalf("proxy dial attempts = %d, want 2 (first request + exactly one single-flight probe)", got)
	}
	if !breakerOpen() {
		t.Fatal("breaker must be extended after the failed probe")
	}
}

// TestProxyAliveTargetErrorsNoBreaker — spec「熔断仅由拨代理失败触发」四形态：
// 代理可达时的目标侧失败（502 / 404 不直连重试 / CONNECT 拒绝 / 目标超时）
// 不开熔断、不重试，调用方按现状收到错误或状态码。
func TestProxyAliveTargetErrorsNoBreaker(t *testing.T) {
	t.Run("proxy 502 keeps breaker closed and keeps using proxy", func(t *testing.T) {
		resetProxyState(t)
		var proxyHits atomic.Int32
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyHits.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(proxy.Close)
		if err := SetProxy(proxy.URL); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		client := New(WithTimeout(2 * time.Second))
		for i := 0; i < 2; i++ {
			resp, err := client.Get("http://example.com/") // non-loopback; proxy answers, target never contacted
			if err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusBadGateway {
				t.Fatalf("request %d status = %d, want 502 from proxy", i, resp.StatusCode)
			}
		}
		if proxyHits.Load() != 2 {
			t.Fatalf("proxy hits = %d, want 2 (breaker must stay closed, proxy stays in use)", proxyHits.Load())
		}
		if breakerOpen() {
			t.Fatal("target-side 502 through a reachable proxy must not open the breaker")
		}
	})

	t.Run("target 404 is not direct-retried", func(t *testing.T) {
		resetProxyState(t)
		var targetHits, proxyHits atomic.Int32
		targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targetHits.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}))
		proxy := forwardingProxy(t, &proxyHits)
		if err := SetProxy(proxy.URL); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		client := New(WithTimeout(2 * time.Second))
		resp, err := client.Get(targetURL)
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
		if targetHits.Load() != 1 {
			t.Fatalf("target hits = %d, want 1 (a direct retry would double it)", targetHits.Load())
		}
		if breakerOpen() {
			t.Fatal("target 404 through a reachable proxy must not open the breaker")
		}
	})

	t.Run("CONNECT refused by reachable proxy", func(t *testing.T) {
		resetProxyState(t)
		tlsURL := serveTLSOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		allowInsecureTLS(t)
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Error(w, "CONNECT-only proxy stub", http.StatusNotFound)
		}))
		t.Cleanup(proxy.Close)
		if err := SetProxy(proxy.URL); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		client := New(WithTimeout(2 * time.Second))
		if _, err := client.Get(tlsURL); err == nil {
			t.Fatal("expected the CONNECT refusal to surface to the caller")
		}
		if breakerOpen() {
			t.Fatal("CONNECT refused by a reachable proxy must not open the breaker")
		}
	})

	t.Run("target timeout through reachable proxy", func(t *testing.T) {
		resetProxyState(t)
		release := make(chan struct{})
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(func() { close(release); proxy.Close() })
		if err := SetProxy(proxy.URL); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		client := New(WithTimeout(100 * time.Millisecond))
		if _, err := client.Get("http://example.com/"); err == nil {
			t.Fatal("expected the timeout to surface to the caller")
		}
		if breakerOpen() {
			t.Fatal("target timeout through a reachable proxy must not open the breaker")
		}
	})
}

// TestRuntimeClearProxyTakesEffect — spec「运行时清空代理立即直连」：清空前
// 构造的 client（启动单例的替身）在 SetProxy("") 后必须立即直连。
func TestRuntimeClearProxyTakesEffect(t *testing.T) {
	resetProxyState(t)
	var hits atomic.Int32
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	proxy := forwardingProxy(t, &hits)
	if err := SetProxy(proxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	client := New(WithTimeout(2 * time.Second)) // built BEFORE the clear

	resp1, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("proxied request: %v", err)
	}
	_ = resp1.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("proxy hits after first request = %d, want 1", hits.Load())
	}

	if err := SetProxy(""); err != nil {
		t.Fatalf("SetProxy empty: %v", err)
	}
	resp2, err := client.Get(targetURL) // same client instance as before the clear
	if err != nil {
		t.Fatalf("request after clearing proxy: %v", err)
	}
	_ = resp2.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("proxy hits after clearing = %d, want 1 (frozen: request must go direct)", hits.Load())
	}
	if currentProxyURL() != nil {
		t.Fatal("proxy URL should be cleared")
	}
}

// TestRuntimeSwapProxyResetsBreaker — spec「运行时换址立即生效且熔断重置」：
// 代理 A 熔断中换到可达的代理 B → 立即经 B 成功，不受 A 的熔断窗口影响。
func TestRuntimeSwapProxyResetsBreaker(t *testing.T) {
	resetProxyState(t)
	var hitsB atomic.Int32
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	proxyB := forwardingProxy(t, &hitsB)
	if err := SetProxy("http://" + freePortAddr(t)); err != nil {
		t.Fatalf("SetProxy A: %v", err)
	}
	_, _ = failProxyDials(t)

	client := New(WithTimeout(2 * time.Second))
	resp1, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("outage request: %v", err)
	}
	_ = resp1.Body.Close()
	if !breakerOpen() {
		t.Fatal("precondition: breaker open for proxy A")
	}

	if err := SetProxy(proxyB.URL); err != nil {
		t.Fatalf("SetProxy B: %v", err)
	}
	if breakerOpen() {
		t.Fatal("swapping proxy must reset the breaker")
	}
	resp2, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("request after swap: %v", err)
	}
	_ = resp2.Body.Close()
	if hitsB.Load() != 1 {
		t.Fatalf("proxy B hits = %d, want 1 (traffic must move to B immediately)", hitsB.Load())
	}
}

// TestFailoverMatrixHTTPSConnectDialFailure — scheme 矩阵（https/CONNECT）：
// http 代理端口不通时，CONNECT 隧道建立前的代理拨号失败同样触发直连回退。
func TestFailoverMatrixHTTPSConnectDialFailure(t *testing.T) {
	resetProxyState(t)
	tlsURL := serveTLSOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	allowInsecureTLS(t)
	if err := SetProxy("http://" + freePortAddr(t)); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	client := New(WithTimeout(3 * time.Second))
	resp, err := client.Get(tlsURL)
	if err != nil {
		t.Fatalf("https via dead proxy must fall back to direct: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !breakerOpen() {
		t.Fatal("CONNECT-path proxy dial failure must open the breaker")
	}
}

// TestFailoverMatrixSocks5DialFailure — scheme 矩阵（socks5）：socks5 代理
// 端口不通时的拨号失败必须被分类器识别并直连回退。
func TestFailoverMatrixSocks5DialFailure(t *testing.T) {
	resetProxyState(t)
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err := SetProxy("socks5://" + freePortAddr(t)); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	client := New(WithTimeout(3 * time.Second))
	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("socks5 dead proxy must fall back to direct: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !breakerOpen() {
		t.Fatal("socks5 proxy dial failure must open the breaker")
	}
}

// TestFailoverMatrixDefaultPortClassification — 错误分类边界：无显式端口的
// 代理 URL（http 默认 80）与实际拨号地址（host:80）必须能匹配上；匹配的是
// 解析前的拨号地址字符串（生产上拨号层可比对，不受 DNS 解析影响）。
func TestFailoverMatrixDefaultPortClassification(t *testing.T) {
	proxy, err := url.Parse("http://proxy.example.com")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !dialAddrMatchesProxy("proxy.example.com:80", proxy) {
		t.Fatal("dial address proxy.example.com:80 must match scheme-default proxy port")
	}
	if dialAddrMatchesProxy("proxy.example.com:8080", proxy) {
		t.Fatal("dial address proxy.example.com:8080 must not match scheme-default proxy port 80")
	}
	ipProxy, err := url.Parse("http://93.184.216.34:7897")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !dialAddrMatchesProxy("93.184.216.34:7897", ipProxy) {
		t.Fatal("dial address 93.184.216.34:7897 must match the ip proxy")
	}
}
