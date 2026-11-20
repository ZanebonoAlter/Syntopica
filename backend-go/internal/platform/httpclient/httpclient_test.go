package httpclient

import (
	"net/http"
	"net/url"
	"reflect"
	"sync/atomic"
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
// 注：WithCrawlScope 标记的 client 返回包级 failover transport（所有 scope
// client 共享、每请求动态读 URL）；未标记 scope 的默认 client 强制直连，
// 由 TestNew_DefaultAlwaysDirect 覆盖。对已构造 client 的即时生效行为由
// failover_test.go 专门覆盖。
func TestSetProxy_AppliesToNewClients(t *testing.T) {
	SetInstrumentation(false) // 裸 transport，方便断言 base
	t.Cleanup(func() {
		SetProxy("")
		SetInstrumentation(true)
	})
	if err := SetProxy("http://proxy.example.com:8080"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	c := New(WithCrawlScope())
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

// TestSetProxy_EmptyClearsProxy 验证空串清除代理。注：清空后 scope client
// 不再返回 http.DefaultTransport 本体，而是返回同一个包级 failover transport
// （其路由函数回落 ProxyFromEnvironment，行为等价直连）；这里断言已构造
// client 与新 client 共享同一实例——这是运行时改配置对既有 client 即时生效
// 的根基，行为级断言见 failover_test.go 的 TestRuntimeClearProxyTakesEffect。
func TestSetProxy_EmptyClearsProxy(t *testing.T) {
	SetInstrumentation(false)
	t.Cleanup(func() {
		SetProxy("")
		SetInstrumentation(true)
	})
	if err := SetProxy("http://proxy.example.com:8080"); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	c1 := New(WithCrawlScope())
	if err := SetProxy(""); err != nil {
		t.Fatalf("SetProxy empty: %v", err)
	}
	c2 := New(WithCrawlScope())
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

// TestNew_DefaultAlwaysDirect — 收窄契约：未标记 WithCrawlScope 的默认 client
// 强制直连——即使全局代理已配置且环境变量在场，也既不经代理也不读环境变量。
// airouter/discovery/dataenrichment 等调用点均为 unscoped ⇒ 由本契约直接得到
// 「AI 调用、目录同步等直连」的 spec ADDED 场景（推导链，不逐包重复断言）。
func TestNew_DefaultAlwaysDirect(t *testing.T) {
	resetProxyState(t)
	var proxyHits atomic.Int32
	targetURL := serveOnAllInterfaces(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	proxy := forwardingProxy(t, &proxyHits)
	if err := SetProxy(proxy.URL); err != nil {
		t.Fatalf("SetProxy: %v", err)
	}
	// 环境变量也在场：默认 client 连它都不该读。
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	client := New(WithTimeout(2 * time.Second))
	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("direct request: %v", err)
	}
	_ = resp.Body.Close()
	if proxyHits.Load() != 0 {
		t.Fatalf("unscoped client hit the proxy %d time(s), want 0", proxyHits.Load())
	}
	// 直闭口子：行为断言可能被 ProxyFromEnvironment 的 once 缓存定型为无代理而恒真，
	// 这里直接断言 transport 的 Proxy 已被禁用（对「不读环境变量」的最强直接断言）。
	if sharedDirect().Proxy != nil {
		t.Fatal("sharedDirect must have Proxy disabled")
	}
	// 默认 client 共享同一进程级 direct transport（关 otel 包装后新建两个比 base）。
	SetInstrumentation(false)
	c1, c2 := New(), New()
	SetInstrumentation(true)
	if c1.Transport != c2.Transport {
		t.Fatal("expected unscoped clients to share the direct transport")
	}
}

// TestWithCrawlScope_EnvFallbackRoute — 作用域 client 的 direct 通道保留
// stdlib ProxyFromEnvironment 引用（SetProxy 未配置时的 env 兜底由它承担）。
// 行为级 env 验证不在单测做：stdlib ProxyFromEnvironment 有 sync.Once 环境
// 缓存，进程内 t.Setenv 无法驱动；返回值断言在无环境代理时会退化为 nil==nil，
// 故用函数指针断言（不依赖环境、恒有区分度）；部署后由验证清单确认行为面。
func TestWithCrawlScope_EnvFallbackRoute(t *testing.T) {
	resetProxyState(t)
	if err := SetProxy(""); err != nil {
		t.Fatalf("SetProxy empty: %v", err)
	}
	SetInstrumentation(false)
	defer SetInstrumentation(true)
	client := New(WithCrawlScope())
	ft, ok := client.Transport.(*failoverTransport)
	if !ok {
		t.Fatalf("expected *failoverTransport, got %T", client.Transport)
	}
	if reflect.ValueOf(ft.direct.Proxy).Pointer() != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() {
		t.Fatal("direct.Proxy must be the stdlib ProxyFromEnvironment (env fallback contract)")
	}
}
