package imageproxy

import (
	"bytes"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// png1x1 是合法 1x1 PNG，用作上游返回的图片字节。
var png1x1 = mustPNG()

func mustPNG() []byte {
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		panic(err)
	}
	return b
}

// newTestEngine 把被测 handler 挂到 /api 组，与生产挂载形态一致。
func newTestEngine(t *testing.T, h *Handler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerWith(r.Group("/api"), h)
	return r
}

func newTestHandler(t *testing.T, maxBytes int64, timeout time.Duration) *Handler {
	t.Helper()
	return newHandler(newDiskCache(t.TempDir(), maxBytes), nil, timeout)
}

// doGet 请求代理；target 为图片 URL（空串表示不带 url 参数）；mutate 可改请求。
func doGet(t *testing.T, r *gin.Engine, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/image-proxy"
	if target != "" {
		path += "?url=" + url.QueryEscape(target)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if mutate != nil {
		mutate(req)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// countingUpstream 返回记录命中数与最后一个请求的上游 stub。
func countingUpstream(t *testing.T, fn http.HandlerFunc) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var hits atomic.Int32
	var lastURI atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		lastURI.Store(r.URL.RequestURI())
		if fn != nil {
			fn(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &lastURI
}

func imageUpstream() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png1x1)
	}
}

// —— spec Scenario：非法 url 被拒绝（缺失/空白/非 http(s)/无 host）——

func TestInvalidURLRejectedWithZeroUpstream(t *testing.T) {
	srv, hits, _ := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	for _, target := range []string{
		"",    // 缺失
		"   ", // 纯空白
		"ftp://cdn.example/a.png",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"//cdn.example/a.png",                   // 缺 scheme
		"http:///no-host.png",                   // 有 scheme 无 host
		srv.URL[:len(srv.URL)-len("/")] + " @x", // 解析失败的非法字符
	} {
		w := doGet(t, r, target, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("target %q: status = %d, want 400", target, w.Code)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("upstream hits = %d, want 0（非法请求零上游）", n)
	}
}

// —— spec Scenario：非法 url 被拒绝（自指防循环）——

func TestLoopGuardRejectsOwnHost(t *testing.T) {
	srv, hits, _ := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	// 图片 host == 代理自身 Host（大小写不敏感）。
	w := doGet(t, r, "http://LOOP.Test/img.png", func(req *http.Request) {
		req.Host = "loop.test"
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("upstream hits = %d, want 0", n)
	}
	_ = srv
}

// —— spec Scenario：禁空 Referer 图床经代理成功（Referer/UA 注入 + 透传）——

func TestInjectsRefererAndUA(t *testing.T) {
	var gotReferer, gotUA atomic.Value
	srv, _, _ := countingUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotReferer.Store(r.Header.Get("Referer"))
		gotUA.Store(r.Header.Get("User-Agent"))
		imageUpstream()(w, r)
	})
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	w := doGet(t, r, srv.URL+"/2026/09/img.png", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	wantReferer := srv.URL + "/"
	if got := gotReferer.Load(); got != wantReferer {
		t.Errorf("Referer = %q, want %q（图片自身 origin）", got, wantReferer)
	}
	if got := gotUA.Load(); got != browserUA {
		t.Errorf("UA = %q, want browserUA", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if got := w.Header().Get(cacheHeader); got != "MISS" {
		t.Errorf("%s = %q, want MISS", cacheHeader, got)
	}
	if !bytes.Equal(w.Body.Bytes(), png1x1) {
		t.Errorf("body 不等于上游图片字节")
	}
}

// —— 白盒 H5：per-host Referer 覆盖表 ——

func TestRefererOverride(t *testing.T) {
	srv, _, _ := countingUpstream(t, imageUpstream())
	host := strings.TrimPrefix(srv.URL, "http://")
	SetRefererOverride(host, "https://sspai.com/")
	t.Cleanup(func() { SetRefererOverride(host, "") })

	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)
	if w := doGet(t, r, srv.URL+"/a.png", nil); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// 覆盖生效：请求重新打一次并读最近一次 Referer。
	var gotReferer atomic.Value
	srv2, _, _ := countingUpstream(t, func(w http.ResponseWriter, req *http.Request) {
		gotReferer.Store(req.Header.Get("Referer"))
		imageUpstream()(w, req)
	})
	host2 := strings.TrimPrefix(srv2.URL, "http://")
	SetRefererOverride(host2, "https://sspai.com/")
	t.Cleanup(func() { SetRefererOverride(host2, "") })
	if w := doGet(t, r, srv2.URL+"/a.png", nil); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := gotReferer.Load(); got != "https://sspai.com/" {
		t.Errorf("Referer = %q, want override value", got)
	}
}

// —— 边界：大小写混合 scheme 归一后可转发 ——

func TestUppercaseSchemeAccepted(t *testing.T) {
	srv, _, _ := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	w := doGet(t, r, "HTTP://"+strings.TrimPrefix(srv.URL, "http://")+"/a.png", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（scheme 大小写归一）", w.Code)
	}
}

// —— 边界：上游查询串原样转发不吞参 ——

func TestUpstreamQueryPreserved(t *testing.T) {
	srv, _, lastURI := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	query := "imageView2/2/w/1120/q/90/interlace/1/ignore-error/1/format/webp"
	w := doGet(t, r, srv.URL+"/img.png?"+query, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got, _ := lastURI.Load().(string); got != "/img.png?"+query {
		t.Errorf("上游收到 %q, want %q", got, "/img.png?"+query)
	}
}

// —— 边界：8KB 超长 URL 有界处理不 panic ——

func TestLongURLBounded(t *testing.T) {
	srv, _, _ := countingUpstream(t, imageUpstream())
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	longPath := "/" + strings.Repeat("a", 8*1024) + ".png"
	w := doGet(t, r, srv.URL+longPath, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// —— spec Scenario：上游拒绝时透传状态码（403 + 零缓存）——

func TestUpstream403PassThroughNoCache(t *testing.T) {
	srv, _, _ := countingUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html>deny by referer access rule</html>"))
	})
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	w := doGet(t, r, srv.URL+"/a.png", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 透传", w.Code)
	}
	if got := w.Header().Get(cacheHeader); got != "MISS" {
		t.Errorf("%s = %q, want MISS", cacheHeader, got)
	}
	if files, err := os.ReadDir(h.cache.dir); err == nil && len(files) != 0 {
		t.Fatalf("缓存目录有 %d 个文件, want 0（非 200 零写入）", len(files))
	}
}

// —— 外部依赖失败有答案：超时 → 504，不可达 → 502 ——

func TestUpstreamTimeoutReturns504(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		imageUpstream()(w, nil)
	}))
	t.Cleanup(srv.Close)

	h := newTestHandler(t, 1<<20, 50*time.Millisecond)
	r := newTestEngine(t, h)
	w := doGet(t, r, srv.URL+"/slow.png", nil)
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", w.Code)
	}
}

func TestUpstreamUnreachableReturns502(t *testing.T) {
	// 占一个端口立即释放，得到确定无监听的地址。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := ln.Addr().String()
	_ = ln.Close()

	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)
	w := doGet(t, r, "http://"+deadAddr+"/a.png", nil)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
}

// —— 路由级：挂载路径与校验可达（registerWith = RegisterRoutes 的挂载拆分）——

func TestRouteMountedOnAPIGroup(t *testing.T) {
	h := newTestHandler(t, 1<<20, 2*time.Second)
	r := newTestEngine(t, h)

	w := doGet(t, r, "", nil) // 无 url 参数
	if w.Code != http.StatusBadRequest {
		t.Errorf("/api/image-proxy 无参数 status = %d, want 400（路由已挂载并进入校验）", w.Code)
	}
	if !strings.Contains(w.Body.String(), "missing url") {
		t.Errorf("body = %q, want 含 missing url", w.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/image-proxy-typo", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusNotFound {
		t.Errorf("邻近路径 status = %d, want 404", w2.Code)
	}
}
