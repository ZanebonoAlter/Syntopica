package imageproxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/platform/httpclient"
)

const (
	// browserUA 避免上游把默认 Go-http-client UA 拦掉。
	browserUA = "Mozilla/5.0 (X11; Linux aarch64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	// upstreamTimeout 覆盖整个上游交换（含读体，ctx 到 handler 返回才 cancel）。
	upstreamTimeout = 15 * time.Second

	cacheHeader = "X-Image-Proxy-Cache"
)

// refererOverrides 是 design D1 的 per-host Referer 覆盖表（初始为空；
// 实测遇到要求主域 Referer 的图床再配置）。
var (
	overrideMu       sync.RWMutex
	refererOverrides = map[string]string{}
)

// SetRefererOverride 设置/清除（referer 为空串即删除）某 host 的 Referer 覆盖。
func SetRefererOverride(host, referer string) {
	host = strings.ToLower(strings.TrimSpace(host))
	overrideMu.Lock()
	defer overrideMu.Unlock()
	if referer == "" {
		delete(refererOverrides, host)
		return
	}
	refererOverrides[host] = referer
}

// refererFor 默认注入图片自身 origin（scheme://host/），命中覆盖表则用覆盖值。
func refererFor(u *url.URL) string {
	overrideMu.RLock()
	v, ok := refererOverrides[strings.ToLower(u.Host)]
	overrideMu.RUnlock()
	if ok && v != "" {
		return v
	}
	return u.Scheme + "://" + u.Host + "/"
}

// Handler 实现 GET /api/image-proxy?url=...（spec：代理转发与 Referer 注入）。
type Handler struct {
	cache   *diskCache
	client  *http.Client
	timeout time.Duration
}

// newHandler 供测试注入缓存目录/客户端/超时；生产走 RegisterRoutes。
func newHandler(cache *diskCache, client *http.Client, timeout time.Duration) *Handler {
	if client == nil {
		client = httpclient.New()
	}
	return &Handler{cache: cache, client: client, timeout: timeout}
}

// NewHandler 按生产配置构造（data/image-cache + IMAGE_CACHE_MAX_MB + 出站代理 failover 客户端）。
func NewHandler() *Handler {
	return newHandler(newDiskCache(defaultCacheDir, maxBytesFromEnv()), nil, upstreamTimeout)
}

// RegisterRoutes 挂载代理路由并做启动清扫。挂载于 /api 组，与其余接口
// 同样经过 ReadOnly 中间件（GET 不受影响）。
func RegisterRoutes(rg *gin.RouterGroup) {
	h := NewHandler()
	h.cache.sweep()
	registerWith(rg, h)
}

// registerWith 用给定 handler 挂路由（RegisterRoutes 的可测拆分，测试注入
// 临时缓存目录避免触碰真实 data/image-cache）。
func registerWith(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/image-proxy", h.Serve)
}

// Serve 校验 url → 查缓存 → 注入 Referer/UA 转发上游 → 按规则落缓存。
func (h *Handler) Serve(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("url"))
	if raw == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "missing url parameter"})
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}
	// url.Parse 保留 scheme 原始大小写，而 http.Client 对 "HTTP://" 判非法定 scheme
	// ——归一后再转发（test-cases 边界：大小写混合 scheme）。
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "only http/https urls are proxied"})
		return
	}
	if u.Host == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "url has no host"})
		return
	}
	if strings.EqualFold(u.Host, c.Request.Host) {
		// 防循环：代理自己的 host 会把请求再打回自己。
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "refusing to proxy own host"})
		return
	}

	if path, ok := h.cache.lookup(raw); ok {
		if h.serveCached(c, path) {
			return
		}
		// 文件在 lookup 与 open 之间被清/删：当作未命中继续走上游。
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", refererFor(u))

	resp, err := h.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{"error": "upstream timeout"})
		} else {
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "upstream unreachable"})
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	c.Header(cacheHeader, "MISS")

	if resp.StatusCode != http.StatusOK {
		// 上游拒绝（spec：状态透传 + 零缓存写入）。
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			c.Header("Content-Type", ct)
		}
		c.Status(resp.StatusCode)
		if _, copyErr := io.Copy(c.Writer, resp.Body); copyErr != nil {
			// 头已发出，中途断流无从补救；丢弃即可。
			_ = copyErr
		}
		return
	}

	h.writeUpstreamOK(c, raw, resp)
}

// writeUpstreamOK 处理上游 200：确定 content-type 后写出头；image/* 走
// 「临时文件 tee → rename 落缓存」，其余类型直通不落盘。
func (h *Handler) writeUpstreamOK(c *gin.Context, rawURL string, resp *http.Response) {
	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	var prefix []byte
	if ct == "" {
		// 上游没给 Content-Type：先嗅探前 512 字节再发头。
		prefix = make([]byte, 512)
		n, rerr := io.ReadFull(resp.Body, prefix)
		if rerr != nil && !errors.Is(rerr, io.ErrUnexpectedEOF) && !errors.Is(rerr, io.EOF) {
			prefix = nil
		} else {
			prefix = prefix[:n]
		}
		ct = detectContentType(prefix)
	}

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", ct)
	if resp.ContentLength >= 0 && len(prefix) == 0 {
		hdr.Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	c.Status(http.StatusOK)

	isImage := strings.HasPrefix(strings.ToLower(ct), "image/")
	if !isImage || !h.cache.enabled() {
		h.drain(c, prefix, resp.Body)
		return
	}

	tmp, tmpPath, terr := h.cache.beginWrite()
	if terr != nil {
		// 缓存目录不可写：降级为直通，响应不受影响（design Risks）。
		h.drain(c, prefix, resp.Body)
		return
	}
	if len(prefix) > 0 {
		_, _ = tmp.Write(prefix)
		_, _ = c.Writer.Write(prefix)
	}
	// tee：同一份字节既给客户端又进临时文件。
	_, copyErr := io.Copy(io.MultiWriter(c.Writer, tmp), resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		return
	}
	h.cache.commit(rawURL, tmpPath)
}

// drain 把 prefix + 剩余 body 写给客户端（不落缓存路径共用）。
func (h *Handler) drain(c *gin.Context, prefix []byte, body io.Reader) {
	if len(prefix) > 0 {
		_, _ = c.Writer.Write(prefix)
	}
	if _, err := io.Copy(c.Writer, body); err != nil {
		_ = err
	}
}

// serveCached 命中路径：读 512 字节嗅探 content-type 后返回文件内容。
// 打开失败返回 false，调用方回落上游。
func (h *Handler) serveCached(c *gin.Context, path string) bool {
	// path = filepath.Join(cacheDir, sha256hex)：目录与文件名均非用户明文可控。
	f, err := os.Open(path) //nolint:gosec // G304 路径由本包哈希构造，限定在缓存目录内
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	head := make([]byte, 512)
	n, _ := f.Read(head)
	body := io.MultiReader(bytes.NewReader(head[:n]), f)
	c.DataFromReader(http.StatusOK, st.Size(), detectContentType(head[:n]), body, map[string]string{
		cacheHeader: "HIT",
	})
	return true
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
