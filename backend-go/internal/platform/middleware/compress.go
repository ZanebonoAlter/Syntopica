package middleware

import (
	"bytes"
	"compress/gzip"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// compressMinBytesDefault is the minimum response body size (bytes) worth
// compressing — below it the gzip framing overhead exceeds the savings.
const compressMinBytesDefault = 1024

// streamingPaths are endpoints whose response bodies must arrive chunk by
// chunk (SSE progress streams). Buffering them behind a compressor would
// destroy the "each event arrives as it happens" semantics, so compression is
// skipped entirely (spec: http-response-compression / 压缩边界与幂等).
// Derived 2026-09-24 by grepping c.Stream / text/event-stream across the repo
// — the only streaming HTTP endpoints are these two SSE handlers; /ws is the
// lone WebSocket upgrade and is handled separately below.
var streamingPaths = map[string]bool{
	"/api/topic-tags/merge-preview/scan/stream":     true,
	"/api/topic-tags/merge-preview/evaluate/stream": true,
}

// compressibleTypes are the media types worth gzipping, as a whitelist —
// anything not listed is passed through untouched (safe default for unknown
// binary formats). Mirrors the spec wording: text/*, application/json,
// application/javascript, image/svg+xml (+ the XML/JS aliases Nuxt actually
// emits, and the favicon-adjacent text formats).
var compressibleExact = map[string]bool{
	"application/json":          true,
	"application/javascript":    true,
	"application/x-javascript":  true,
	"text/javascript":           true,
	"text/plain":                true,
	"image/svg+xml":             true,
	"application/xml":           true,
	"text/xml":                  true,
	"application/atom+xml":      true,
	"application/rss+xml":       true,
	"application/manifest+json": true,
	"application/ld+json":       true,
}

// precompressedTypes are media formats that are already compressed —
// re-compressing them wastes CPU and can even grow the payload.
var precompressedExact = map[string]bool{
	"font/woff2":               true,
	"font/woff":                true,
	"font/x-woff":              true,
	"image/png":                true,
	"image/jpeg":               true,
	"image/webp":               true,
	"image/gif":                true,
	"image/avif":               true,
	"image/x-icon":             true,
	"image/vnd.microsoft.icon": true,
	"application/zip":          true,
	"application/gzip":         true,
	"application/x-gzip":       true,
	"application/wasm":         true,
	"application/pdf":          true,
	"application/octet-stream": true,
}

// gzipWriterPool recycles gzip writers across requests to keep the
// per-response allocation cost down.
var gzipWriterPool = sync.Pool{
	New: func() interface{} { return gzip.NewWriter(nil) },
}

// Compress returns a Gin middleware that negotiates gzip compression for
// text-like responses based on the request's Accept-Encoding header.
//
// Contract (openspec: http-response-compression):
//   - covers API routes and static files alike — compression lives in the app,
//     never delegated to a reverse proxy;
//   - writes `Vary: Accept-Encoding` on every negotiable response (compressed
//     or not) so shared caches never serve the wrong variant;
//   - skips already-compressed media types (woff2/png/jpg/webp/gif/zip/gz…),
//     responses that already carry Content-Encoding, bodies below the
//     minimum-size threshold (default 1 KiB, COMPRESS_MIN_BYTES overrides);
//   - passes WebSocket upgrades, Range requests and whitelisted streaming
//     endpoints through untouched so chunked delivery semantics survive;
//   - never keeps the original Content-Length on a compressed response.
func Compress() gin.HandlerFunc {
	return func(c *gin.Context) {
		req := c.Request
		// WebSocket upgrades and SSE streams: hands off, not even Vary — the
		// 101/SSE response is protocol traffic, not a cacheable variant.
		if isWebSocketUpgrade(req) || streamingPaths[req.URL.Path] {
			c.Next()
			return
		}
		// Range requests: 206 partial-content semantics are byte-exact offsets
		// into the identity representation; compressing would break them.
		if req.Header.Get("Range") != "" {
			c.Header("Vary", "Accept-Encoding")
			c.Next()
			return
		}

		c.Header("Vary", "Accept-Encoding")
		w := &compressWriter{ResponseWriter: c.Writer, req: req, minBytes: compressMinBytes()}
		c.Writer = w
		c.Next()
		w.finish()
	}
}

// compressWriter buffers the response until it can decide between gzip and
// identity, then streams through the chosen representation. The decision is
// deferred to the first Write because Content-Type/Content-Length may be set
// late by the handler (http.ServeContent, c.JSON, …) and gin only commits the
// header on first underlying write.
type compressWriter struct {
	gin.ResponseWriter
	req      *http.Request
	minBytes int
	decided  bool
	useGzip  bool
	gz       *gzip.Writer
	buf      bytes.Buffer
}

// Write implements io.Writer, intercepting the body to run the compression
// decision exactly once.
func (w *compressWriter) Write(b []byte) (int, error) {
	if !w.decided {
		if !w.shouldAttempt() {
			w.decided = true
		} else {
			// Buffer until the body proves itself above the threshold.
			w.buf.Write(b)
			if w.buf.Len() >= w.minBytes {
				w.startGzip()
			}
			return len(b), nil
		}
	}
	if w.useGzip {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// WriteString mirrors Write so gin's string helpers keep single-write
// semantics under the interceptor.
func (w *compressWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// Flush pushes buffered bytes to the client. If compression is active the
// gzip stream is flushed too, keeping SSE-style liveness for any endpoint
// that flushes explicitly (whitelisted streams bypass this middleware).
func (w *compressWriter) Flush() {
	if !w.decided {
		// Small body flushed early: commit to identity and ship the buffer
		// first so bytes keep their order.
		w.decided = true
		if w.buf.Len() > 0 && !w.Written() {
			_, _ = w.ResponseWriter.Write(w.buf.Bytes())
		}
		w.buf.Reset()
	}
	if w.useGzip {
		_ = w.gz.Flush()
	}
	w.ResponseWriter.Flush()
}

// shouldAttempt inspects response headers only (no body): it reports whether
// the response is even a compression candidate. Body size is judged later,
// via the threshold buffer.
func (w *compressWriter) shouldAttempt() bool {
	h := w.Header()
	// Response already carries an encoding (handler-level compression) —
	// never encode twice (§5.1 row 11).
	if h.Get("Content-Encoding") != "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil || mediaType == "" {
		return false // unknown body type: do not gamble
	}
	if precompressedExact[mediaType] ||
		strings.HasPrefix(mediaType, "video/") ||
		strings.HasPrefix(mediaType, "audio/") {
		return false
	}
	if !compressibleExact[mediaType] && !strings.HasPrefix(mediaType, "text/") {
		return false
	}
	if !acceptsGzip(w.req.Header.Get("Accept-Encoding")) {
		return false
	}
	// Static files advertise their size up front: a small file skips the
	// buffer dance entirely.
	if cl := h.Get("Content-Length"); cl != "" {
		if n, err := strconv.Atoi(cl); err == nil && n < w.minBytes {
			return false
		}
	}
	return true
}

// startGzip commits to the gzip representation: swap the headers, wire the
// pooled gzip writer to the underlying response writer, and drain the buffer
// through it.
func (w *compressWriter) startGzip() {
	w.decided = true
	w.useGzip = true
	h := w.Header()
	// Compressed length differs from the original; keeping the stale value
	// would truncate the body on the client side.
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	w.gz = gzipWriterPool.Get().(*gzip.Writer)
	// Reset onto a passthrough shim so gzip output hits the wire without
	// re-entering this interceptor (which would recurse).
	w.gz.Reset(passthroughWriter{w.ResponseWriter})
	if w.buf.Len() > 0 {
		_, _ = w.gz.Write(w.buf.Bytes())
		w.buf.Reset()
	}
}

// finish runs after the handler chain returns: close the gzip stream (emit
// trailer), or ship the sub-threshold identity body that never made it out
// of the buffer. Safe to call after a panic-abort — it never invents a body.
func (w *compressWriter) finish() {
	defer func() {
		if w.gz != nil {
			_ = w.gz.Close()
			gzipWriterPool.Put(w.gz)
			w.gz = nil
		}
	}()
	// Body still buffered when the handler returned: the total stayed under
	// the threshold — ship it as identity. Skip if the response was already
	// committed upstream (e.g. recovery middleware aborting a panic: its 500
	// must stay empty, not inherit a partial body).
	if w.buf.Len() > 0 && !w.useGzip {
		if !w.Written() {
			_, _ = w.ResponseWriter.Write(w.buf.Bytes())
		}
		w.buf.Reset()
		w.decided = true
		return
	}
	if !w.decided {
		// No body was ever written (204/304/HEAD-without-body): commit the
		// recorded status, nothing to encode.
		w.WriteHeaderNow()
		return
	}
	// useGzip: trailer emitted by the deferred Close; identity-passthrough:
	// nothing pending.
}

// passthroughWriter forwards writes straight to the underlying response
// writer, bypassing the interceptor — the gzip writer's sink.
type passthroughWriter struct {
	gin.ResponseWriter
}

func (p passthroughWriter) Write(b []byte) (int, error) {
	return p.ResponseWriter.Write(b)
}

// isWebSocketUpgrade reports whether the request is a WebSocket handshake;
// the 101 response is not an HTTP body and must bypass compression entirely.
func isWebSocketUpgrade(req *http.Request) bool {
	if !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, v := range req.Header.Values("Connection") {
		if strings.Contains(strings.ToLower(v), "upgrade") {
			return true
		}
	}
	return false
}

// acceptsGzip parses an Accept-Encoding header (RFC 7231): any gzip entry —
// case-insensitive, with a q-value above zero — counts as an offer. Unknown
// codings and br-only offers leave the response identity.
func acceptsGzip(header string) bool {
	if strings.TrimSpace(header) == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		token := part
		q := 1.0
		if i := strings.IndexByte(part, ';'); i >= 0 {
			token = part[:i]
			if v := strings.TrimSpace(part[i+1:]); strings.HasPrefix(v, "q=") {
				if f, err := strconv.ParseFloat(v[2:], 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 && strings.EqualFold(strings.TrimSpace(token), "gzip") {
			return true
		}
	}
	return false
}

// compressMinBytes reads the compression threshold; COMPRESS_MIN_BYTES
// overrides the 1 KiB default (tests shrink/grow it via withEnv).
func compressMinBytes() int {
	if v := strings.TrimSpace(os.Getenv("COMPRESS_MIN_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return compressMinBytesDefault
}
