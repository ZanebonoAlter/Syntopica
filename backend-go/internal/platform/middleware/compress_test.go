package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// compressCase maps one row of test-cases.md §5.1 (压缩中间件分支表, rows 1-8).
type compressCase struct {
	name           string
	acceptEncoding string // request header value ("" = header absent)
	contentType    string // response Content-Type set by the handler
	body           string // response body produced by the handler
	wantGzip       bool
	wantVary       bool // Vary: Accept-Encoding must exist even on identity responses
}

func TestCompress(t *testing.T) {
	bigHTML := strings.Repeat("<p>syntopica 公网链路压缩分支验证</p>", 256)                                                        // ~8 KB text/html
	smallHTML := strings.Repeat("<p>tiny</p>", 60)                                                                       // ~900 B (< 1 KiB threshold)
	bigJSON := `{"items":[` + strings.Repeat(`{"title":"article","lede":"lead"},`, 640) + `{"title":"end","lede":"x"}]}` // ~50 KB application/json
	bigFont := strings.Repeat("WOFF2FONTDATA", 640)                                                                      // ~8 KB font/woff2
	bigPNG := strings.Repeat("\x89PNG-fake-bytes", 640)                                                                  // ~8 KB image/png

	cases := []compressCase{
		// §5.1 row 1: gzip + text/html 8 KB → compressed, Vary present.
		{name: "gzip text/html 8KB compressed", acceptEncoding: "gzip", contentType: "text/html; charset=utf-8", body: bigHTML, wantGzip: true, wantVary: true},
		// §5.1 row 2: gzip + text/html 900 B → identity, Vary still present.
		{name: "gzip text/html 900B below threshold", acceptEncoding: "gzip", contentType: "text/html; charset=utf-8", body: smallHTML, wantGzip: false, wantVary: true},
		// §5.1 row 3: gzip + application/json 50 KB → compressed, decompresses byte-equal.
		{name: "gzip application/json 50KB compressed", acceptEncoding: "gzip", contentType: "application/json; charset=utf-8", body: bigJSON, wantGzip: true, wantVary: true},
		// §5.1 row 4: font/woff2 is already compressed → never re-compressed.
		{name: "woff2 passthrough", acceptEncoding: "gzip", contentType: "font/woff2", body: bigFont, wantGzip: false, wantVary: true},
		// §5.1 row 5: image/png is already compressed → never re-compressed.
		{name: "png passthrough", acceptEncoding: "gzip", contentType: "image/png", body: bigPNG, wantGzip: false, wantVary: true},
		// §5.1 row 6: no Accept-Encoding → identity, Vary still present.
		{name: "no accept-encoding identity", acceptEncoding: "", contentType: "text/html; charset=utf-8", body: bigHTML, wantGzip: false, wantVary: true},
		// §5.1 row 7: br-only offer (br unimplemented) → identity, Vary present.
		{name: "br-only identity", acceptEncoding: "br", contentType: "text/html; charset=utf-8", body: bigHTML, wantGzip: false, wantVary: true},
		// §5.1 row 8: deflate with downgraded gzip q-value → gzip still negotiable.
		{name: "deflate with gzip q=0.5 compressed", acceptEncoding: "deflate, gzip;q=0.5", contentType: "text/html; charset=utf-8", body: bigHTML, wantGzip: true, wantVary: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newCompressTestRouter(tc.contentType, tc.body)
			req := httptest.NewRequest(http.MethodGet, "/_nuxt/probe", nil)
			if tc.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tc.acceptEncoding)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			gotEncoding := w.Header().Get("Content-Encoding")
			if tc.wantGzip && gotEncoding != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip (wire %d bytes, original %d)", gotEncoding, w.Body.Len(), len(tc.body))
			}
			if !tc.wantGzip && gotEncoding != "" {
				t.Fatalf("Content-Encoding = %q, want identity (no header)", gotEncoding)
			}
			gotVary := w.Header().Get("Vary")
			if tc.wantVary && !strings.Contains(gotVary, "Accept-Encoding") {
				t.Fatalf("Vary = %q, want it to contain Accept-Encoding", gotVary)
			}
			if tc.wantGzip {
				zr, err := gzip.NewReader(w.Body)
				if err != nil {
					t.Fatalf("body is not valid gzip: %v", err)
				}
				defer zr.Close()
				decoded, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("gzip decode failed: %v", err)
				}
				if string(decoded) != tc.body {
					t.Fatalf("decompressed body differs from original: decoded %d bytes, want %d", len(decoded), len(tc.body))
				}
				if w.Body.Len() >= len(tc.body) {
					t.Fatalf("wire %d bytes >= original %d bytes: compression gained nothing", w.Body.Len(), len(tc.body))
				}
			} else if w.Body.String() != tc.body {
				t.Fatalf("identity body was altered: got %d bytes, want %d", w.Body.Len(), len(tc.body))
			}
		})
	}
}

// newCompressTestRouter builds a gin engine with Compress() mounted and a
// single probe route emitting the given Content-Type/body — the minimal
// harness for the §5.1 branch table.
func newCompressTestRouter(contentType, body string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Compress())
	r.GET("/_nuxt/probe", func(c *gin.Context) {
		c.Header("Content-Type", contentType)
		_, _ = c.Writer.WriteString(body)
	})
	return r
}

// TestCompressSkip covers §5.1 rows 9-13: requests/responses that must bypass
// compression entirely.
func TestCompressSkip(t *testing.T) {
	big := strings.Repeat("skip-compression-body-", 400) // ~8 KB

	t.Run("websocket upgrade untouched", func(t *testing.T) {
		r := newCompressTestRouter("text/plain; charset=utf-8", big)
		req := httptest.NewRequest(http.MethodGet, "/_nuxt/probe", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		req.Header.Set("Sec-WebSocket-Version", "13")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if enc := w.Header().Get("Content-Encoding"); enc != "" {
			t.Fatalf("WS upgrade got Content-Encoding %q, want untouched", enc)
		}
		if w.Body.String() != big {
			t.Fatalf("WS upgrade body altered")
		}
	})

	t.Run("streaming path untouched", func(t *testing.T) {
		for path := range streamingPaths {
			r := newCompressTestRouter("text/event-stream", big)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if enc := w.Header().Get("Content-Encoding"); enc != "" {
				t.Fatalf("streaming path %s got Content-Encoding %q", path, enc)
			}
		}
	})

	t.Run("existing content-encoding not double-compressed", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(Compress())
		r.GET("/_nuxt/probe", func(c *gin.Context) {
			c.Header("Content-Encoding", "gzip")
			c.Header("Content-Type", "text/html; charset=utf-8")
			_, _ = c.Writer.WriteString(big)
		})
		req := httptest.NewRequest(http.MethodGet, "/_nuxt/probe", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Fatalf("Content-Encoding = %q, want handler-set gzip kept", w.Header().Get("Content-Encoding"))
		}
		if w.Body.String() != big {
			t.Fatalf("handler-provided body was re-encoded")
		}
	})

	t.Run("range request identity", func(t *testing.T) {
		r := newCompressTestRouter("application/javascript", big)
		req := httptest.NewRequest(http.MethodGet, "/_nuxt/probe", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Range", "bytes=0-99")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if enc := w.Header().Get("Content-Encoding"); enc != "" {
			t.Fatalf("Range request got Content-Encoding %q, want identity", enc)
		}
	})

	t.Run("HEAD request safe", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(Compress())
		r.HEAD("/_nuxt/probe", func(c *gin.Context) {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Header("Content-Length", "8192")
			c.Status(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodHead, "/_nuxt/probe", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("HEAD status = %d, want %d", w.Code, http.StatusOK)
		}
	})
}

// TestCompressStreaming asserts that a streaming endpoint on the whitelist
// still delivers its events chunk by chunk (≥2 observed writes) — the
// compression layer must not buffer them into one blob.
func TestCompressStreaming(t *testing.T) {
	path := "/api/topic-tags/merge-preview/scan/stream"
	proceed := make(chan struct{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Compress())
	r.GET(path, func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		_, _ = c.Writer.WriteString("data: chunk-0\n\n")
		c.Writer.Flush()
		<-proceed
		_, _ = c.Writer.WriteString("data: chunk-1\n\n")
		c.Writer.Flush()
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()
	if enc := resp.Header.Get("Content-Encoding"); enc != "" {
		t.Fatalf("streaming response got Content-Encoding %q", enc)
	}
	first := make([]byte, len("data: chunk-0\n\n"))
	if _, err := io.ReadFull(resp.Body, first); err != nil {
		t.Fatalf("first chunk read failed: %v", err)
	}
	if string(first) != "data: chunk-0\n\n" {
		t.Fatalf("first chunk = %q, want exactly chunk-0 (not merged)", first)
	}
	close(proceed)
	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("rest read failed: %v", err)
	}
	if string(rest) != "data: chunk-1\n\n" {
		t.Fatalf("second chunk = %q, want chunk-1", rest)
	}
}

// TestCompressNegotiation covers the Accept-Encoding parsing variants from
// test-cases §2 (输入组): q=0 suppresses, case-insensitive token matches,
// unknown codings fall back to identity.
func TestCompressNegotiation(t *testing.T) {
	bigHTML := strings.Repeat("<p>negotiation</p>", 512)
	cases := []struct {
		accept   string
		wantGzip bool
	}{
		{"gzip;q=0", false},
		{"GZIP", true},
		{"deflate", false},
		{"gzip;q=0.001", true},
		{"br, gzip", true},
		{"identity", false},
	}
	for _, tc := range cases {
		t.Run(tc.accept, func(t *testing.T) {
			r := newCompressTestRouter("text/html; charset=utf-8", bigHTML)
			req := httptest.NewRequest(http.MethodGet, "/_nuxt/probe", nil)
			req.Header.Set("Accept-Encoding", tc.accept)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			got := w.Header().Get("Content-Encoding")
			if tc.wantGzip && got != "gzip" {
				t.Fatalf("Accept-Encoding %q: Content-Encoding = %q, want gzip", tc.accept, got)
			}
			if !tc.wantGzip && got != "" {
				t.Fatalf("Accept-Encoding %q: Content-Encoding = %q, want identity", tc.accept, got)
			}
		})
	}
}

// TestCompressThreshold covers §5.2 boundary values: exactly-at-threshold is
// NOT compressed (min = "compress from minBytes upward" → below min stays
// identity; here threshold semantics are `body >= minBytes` compressible).
func TestCompressThreshold(t *testing.T) {
	withEnv(t, "COMPRESS_MIN_BYTES", "1025") // shrink/grow via env override
	for _, tc := range []struct {
		size     int
		wantGzip bool
	}{
		{1023, false},
		{1024, false},
		{1025, true},
	} {
		t.Run(strconv.Itoa(tc.size), func(t *testing.T) {
			body := strings.Repeat("x", tc.size)
			r := newCompressTestRouter("text/plain", body)
			req := httptest.NewRequest(http.MethodGet, "/_nuxt/probe", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			got := w.Header().Get("Content-Encoding")
			if tc.wantGzip && got != "gzip" {
				t.Fatalf("size %d: Content-Encoding = %q, want gzip", tc.size, got)
			}
			if !tc.wantGzip && got != "" {
				t.Fatalf("size %d: Content-Encoding = %q, want identity", tc.size, got)
			}
		})
	}
}
