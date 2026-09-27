package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newStaticTestEngine builds a gin engine serving a throwaway static root
// with the same layout SetupStaticFiles expects (frontend/ as cwd-relative
// root). Returns the engine so tests can hit routes without a real build.
func newStaticTestEngine(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	dir := t.TempDir()
	mustWrite := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mustWrite("_nuxt/entry.ABC123.js", "console.log('hashed-entry')")
	mustWrite("robots.txt", "User-agent: *\nAllow: /")
	mustWrite("index.html", "<!doctype html><html><body>spa-shell</body></html>")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Mirror SetupStaticFiles wiring against the temp root: the production
	// function hardcodes "frontend" as its cwd-relative root, so tests bind
	// the same handlers to the temp dir explicitly.
	r.Use(spaFallback(dir))
	h := http.StripPrefix("/_nuxt", http.FileServer(http.Dir(filepath.Join(dir, "_nuxt"))))
	r.GET("/_nuxt/*filepath", gin.WrapH(withCacheHeader(h, cacheHashedImmutable)))
	r.GET("/favicon.png", func(c *gin.Context) {
		c.Header("Cache-Control", cacheUnhashedDaily)
		c.File(filepath.Join(dir, "favicon.png"))
	})
	return r, dir
}

func get(r http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHashedAssetsImmutableCache(t *testing.T) {
	r, _ := newStaticTestEngine(t)
	w := get(r, "/_nuxt/entry.ABC123.js")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=31536000") {
		t.Fatalf("Cache-Control = %q, want immutable + max-age=31536000", cc)
	}
}

func TestHTMLNoLongCache(t *testing.T) {
	r, _ := newStaticTestEngine(t)
	// /index.html is 301-redirected to / by http.FileServer itself; shells
	// served through the fallback (/ , /tags) are the paths that must carry
	// no-cache.
	for _, path := range []string{"/", "/tags"} {
		w := get(r, path)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, w.Code)
		}
		cc := w.Header().Get("Cache-Control")
		if cc != cacheHTMLNoCache {
			t.Fatalf("GET %s Cache-Control = %q, want %q", path, cc, cacheHTMLNoCache)
		}
	}
}

func TestUnhashedStaticFilesDailyCache(t *testing.T) {
	r, _ := newStaticTestEngine(t)
	w := get(r, "/robots.txt")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != cacheUnhashedDaily {
		t.Fatalf("Cache-Control = %q, want %q", cc, cacheUnhashedDaily)
	}
}

func TestFaviconCacheHeader(t *testing.T) {
	r, dir := newStaticTestEngine(t)
	if err := os.WriteFile(filepath.Join(dir, "favicon.png"), []byte("fake-png"), 0o600); err != nil {
		t.Fatalf("seed favicon: %v", err)
	}
	w := get(r, "/favicon.png")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	cc := w.Header().Get("Cache-Control")
	if !strings.Contains(cc, "max-age=86400") {
		t.Fatalf("favicon Cache-Control = %q, want daily max-age=86400", cc)
	}
}
