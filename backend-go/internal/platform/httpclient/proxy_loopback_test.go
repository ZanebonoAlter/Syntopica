package httpclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestIsLoopbackHost covers the loopback classification used by the proxy
// bypass: localhost, 127.0.0.0/8, ::1 and empty host are direct; anything
// else goes through the proxy.
func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"127.0.0.1", true},
		{"127.8.8.8", true},
		{"::1", true},
		{"[::1]", true},
		{"", true},
		{"example.com", false},
		{"192.168.5.20", false},
		{"0.0.0.0", false},
	}
	for _, c := range cases {
		if got := isLoopbackHost(c.host); got != c.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

// TestProxyBypass_LoopbackTargetDirect verifies that with a global proxy
// configured, requests to loopback targets (127.0.0.1, as used by local
// llama-server) connect directly: the proxy server must receive zero requests.
func TestProxyBypass_LoopbackTargetDirect(t *testing.T) {
	var proxyHits atomic.Int32
	fakeProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		w.WriteHeader(http.StatusBadGateway) // 模拟 Clash 对本地地址的 502
	}))
	defer fakeProxy.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer target.Close()

	if err := SetProxy(fakeProxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	defer func() { _ = SetProxy("") }()

	client := New(WithTimeout(5*time.Second), WithCrawlScope())
	resp, err := client.Get(target.URL + "/v1/models") // 127.0.0.1 -> loopback
	if err != nil {
		t.Fatalf("GET loopback target: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (proxy must be bypassed for loopback)", resp.StatusCode)
	}
	if got := proxyHits.Load(); got != 0 {
		t.Fatalf("proxy received %d request(s), want 0 for loopback target", got)
	}
	_ = body
}

// TestProxyBypass_ExternalTargetUsesProxy verifies non-loopback targets still
// go through the configured proxy (the proxy server receives the request).
func TestProxyBypass_ExternalTargetUsesProxy(t *testing.T) {
	var proxyHits atomic.Int32
	fakeProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer fakeProxy.Close()

	if err := SetProxy(fakeProxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	defer func() { _ = SetProxy("") }()

	client := New(WithTimeout(5*time.Second), WithCrawlScope())
	// example.com is not loopback; the request must be forwarded to the proxy
	// (which answers 502 — we only assert the request reached it).
	resp, err := client.Get("http://example.com/v1/models")
	if err != nil {
		t.Fatalf("GET external target: %v", err)
	}
	defer resp.Body.Close()

	if got := proxyHits.Load(); got == 0 {
		t.Fatal("external target must go through the proxy")
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 from proxy", resp.StatusCode)
	}
}

// TestProxyBypass_NoProxyConfigured_Direct verifies unconfigured state stays
// direct for every target (no proxy logic involved).
func TestProxyBypass_NoProxyConfigured_Direct(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	if err := SetProxy(""); err != nil {
		t.Fatalf("SetProxy(\"\"): %v", err)
	}

	client := New(WithTimeout(5*time.Second), WithCrawlScope())
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
