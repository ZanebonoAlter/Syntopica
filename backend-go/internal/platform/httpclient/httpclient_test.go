package httpclient

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TestNew_DefaultClientHasTransport 验证默认构造的 client 有非空 transport。
func TestNew_DefaultClientHasTransport(t *testing.T) {
	SetInstrumentation(true)
	c := New()
	if c == nil {
		t.Fatal("New returned nil client")
	}
	if c.Transport == nil {
		t.Fatal("default transport is nil")
	}
}

// TestNew_InstrumentedWrapsWithOtelHTTP 验证 InstrumentHTTP=true 时
// transport 被 otelhttp 包装（对应 spec: LLM 调用出现在 trace）。
func TestNew_InstrumentedWrapsWithOtelHTTP(t *testing.T) {
	SetInstrumentation(true)
	defer SetInstrumentation(true)
	c := New()
	if _, ok := c.Transport.(*otelhttp.Transport); !ok {
		t.Fatalf("expected *otelhttp.Transport when instrumented, got %T", c.Transport)
	}
}

// TestNew_DisabledReturnsPlainTransport 验证 InstrumentHTTP=false 时
// transport 不含 otelhttp 包装（对应 spec: 可按配置关闭）。
func TestNew_DisabledReturnsPlainTransport(t *testing.T) {
	SetInstrumentation(false)
	defer SetInstrumentation(true)
	c := New()
	if _, ok := c.Transport.(*otelhttp.Transport); ok {
		t.Fatalf("expected plain transport when disabled, got *otelhttp.Transport")
	}
}

// TestWithTimeout 验证 Timeout option 生效（对应 spec: 保留各调用点自定义）。
func TestWithTimeout(t *testing.T) {
	c := New(WithTimeout(5 * time.Second))
	if c.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %v", c.Timeout)
	}
}

// TestWithTransport 验证自定义 transport 被保留并（启用时）被 otelhttp 包装。
func TestWithTransport(t *testing.T) {
	SetInstrumentation(true)
	defer SetInstrumentation(true)
	custom := &http.Transport{}
	c := New(WithTransport(custom))
	if _, ok := c.Transport.(*otelhttp.Transport); !ok {
		t.Fatalf("expected *otelhttp.Transport wrapping custom base, got %T", c.Transport)
	}
}

// TestSetInstrumentation_TogglesGlobally 验证开关全局生效。
func TestSetInstrumentation_TogglesGlobally(t *testing.T) {
	SetInstrumentation(false)
	c1 := New()
	if _, ok := c1.Transport.(*otelhttp.Transport); ok {
		t.Fatal("disabled should not wrap")
	}
	SetInstrumentation(true)
	defer SetInstrumentation(true)
	c2 := New()
	if _, ok := c2.Transport.(*otelhttp.Transport); !ok {
		t.Fatal("enabled should wrap")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// TestSetProxy_AppliesToNewClients 验证 SetProxy 后路由函数按当前配置返回代理。
// 注：New 现在返回包级 failover transport（所有 client 共享、每请求动态读 URL），
// 断言对象从裸 *http.Transport 改为 wrapper 的 proxy 路由 transport；
// 对已构造 client 的即时生效行为由 failover_test.go 专门覆盖。
func TestSetProxy_AppliesToNewClients(t *testing.T) {
	SetInstrumentation(false) // 裸 transport，方便断言 base
	t.Cleanup(func() {
		SetProxy("")
		SetInstrumentation(true)
	})
	if err := SetProxy("http://proxy.example.com:8080"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	c := New()
	w, ok := c.Transport.(*failoverTransport)
	if !ok {
		t.Fatalf("expected *failoverTransport, got %T", c.Transport)
	}
	got, err := w.proxy.Proxy(&http.Request{URL: mustParseURL(t, "http://target.example.com/")})
	if err != nil {
		t.Fatalf("Proxy() error: %v", err)
	}
	if got == nil || got.Host != "proxy.example.com:8080" {
		t.Fatalf("expected proxy host proxy.example.com:8080, got %v", got)
	}
}

// TestSetProxy_EmptyClearsProxy 验证空串清除代理。注：清空后 New 不再返回
// http.DefaultTransport 本体，而是返回同一个包级 failover transport（其路由
// 函数回落 ProxyFromEnvironment，行为等价直连）；这里断言已构造 client 与新
// client 共享同一实例——这是运行时改配置对既有 client 即时生效的根基，
// 行为级断言见 failover_test.go 的 TestRuntimeClearProxyTakesEffect。
func TestSetProxy_EmptyClearsProxy(t *testing.T) {
	SetInstrumentation(false)
	t.Cleanup(func() {
		SetProxy("")
		SetInstrumentation(true)
	})
	if err := SetProxy("http://proxy.example.com:8080"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	c1 := New()
	if err := SetProxy(""); err != nil {
		t.Fatalf("SetProxy empty: %v", err)
	}
	c2 := New()
	if c1.Transport != c2.Transport {
		t.Fatalf("expected the shared failover transport to be reused after clearing proxy, got %T vs %T", c1.Transport, c2.Transport)
	}
	if currentProxyURL() != nil {
		t.Fatal("expected proxy URL to be cleared")
	}
}

// TestSetProxy_RejectsUnsupportedScheme 验证非法 scheme 报错且不污染全局状态。
// 注：全局状态从 transport 实例改为原子代理 URL，断言对象同步调整。
func TestSetProxy_RejectsUnsupportedScheme(t *testing.T) {
	t.Cleanup(func() { SetProxy("") })
	if err := SetProxy("ftp://proxy.example.com:21"); err == nil {
		t.Fatal("expected error for ftp scheme, got nil")
	}
	if currentProxyURL() != nil {
		t.Fatal("proxy URL should remain unset after rejected scheme")
	}
}

// TestNew_WithTransportOverridesProxy 验证显式 WithTransport 覆盖全局代理。
func TestNew_WithTransportOverridesProxy(t *testing.T) {
	SetInstrumentation(false)
	t.Cleanup(func() {
		SetProxy("")
		SetInstrumentation(true)
	})
	if err := SetProxy("http://proxy.example.com:8080"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	custom := &http.Transport{}
	c := New(WithTransport(custom))
	if c.Transport != custom {
		t.Fatalf("expected custom transport to override proxy, got %T", c.Transport)
	}
}
