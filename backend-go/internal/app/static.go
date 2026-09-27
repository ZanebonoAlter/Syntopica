package app

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// Cache-Control contract (openspec: same-origin-deployment / 静态资源体积与缓存契约):
//   - /_nuxt/* files are content-hashed → cache for a year, immutable;
//   - index.html (incl. SPA fallback for deep routes and prerendered shell
//     copies 200.html/404.html) must revalidate on every load → no-cache;
//   - every other unhashed static file (favicon, textures, robots.txt, …)
//     gets a conservative one-day public cache.
const (
	cacheHashedImmutable = "public, max-age=31536000, immutable"
	cacheHTMLNoCache     = "no-cache"
	cacheUnhashedDaily   = "public, max-age=86400"
)

// prerenderedHTMLFiles are bare filenames served from the static root that
// are HTML shells, not hashed assets — they must revalidate like index.html.
var prerenderedHTMLFiles = map[string]bool{
	"/index.html": true,
	"/200.html":   true,
	"/404.html":   true,
}

func SetupStaticFiles(r *gin.Engine) {
	staticDir := "frontend"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		return
	}

	r.Use(spaFallback(staticDir))
	// /assets and /_nuxt are content-hashed builds: serve them with the
	// immutable header via a wrapped file server so every response carries
	// the Cache-Control contract. /assets stays registered unconditionally
	// (missing dir → 404), matching the previous r.Static behavior.
	for _, dir := range []string{"assets", "_nuxt"} {
		h := http.StripPrefix("/"+dir, http.FileServer(http.Dir(staticDir+"/"+dir)))
		r.GET("/"+dir+"/*filepath", gin.WrapH(withCacheHeader(h, cacheHashedImmutable)))
	}
	// favicon is unhashed but tiny: conservative daily cache instead of
	// immutable (a stale icon for ≤1 day is acceptable, forever is not).
	r.GET("/favicon.png", func(c *gin.Context) {
		c.Header("Cache-Control", cacheUnhashedDaily)
		c.File(staticDir + "/favicon.png")
	})
	// /icons is owned by the backend icon store (registered in SetupRoutes) —
	// it must not be re-registered here or gin would panic on the duplicate
	// route prefix.
}

// withCacheHeader stamps every response from the wrapped handler with a
// fixed Cache-Control value.
func withCacheHeader(h http.Handler, cacheControl string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", cacheControl)
		h.ServeHTTP(w, req)
	})
}

// iconsFileHandler serves the icon storage directory (registered at
// /icons/*filepath) with security headers that neutralize stored SVG XSS:
// X-Content-Type-Options: nosniff forces the browser to honor the served
// Content-Type instead of sniffing, and Content-Security-Policy: sandbox
// strips script execution from any SVG rendered outside an <img> context.
func iconsFileHandler(dir string) gin.HandlerFunc {
	fileServer := http.StripPrefix("/icons", http.FileServer(http.Dir(dir)))
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Content-Security-Policy", "sandbox")
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func spaFallback(staticDir string) gin.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticDir))
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || path == "/icons" || strings.HasPrefix(path, "/icons/") || path == "/ws" || path == "/health" ||
			strings.HasPrefix(path, "/_nuxt/") || strings.HasPrefix(path, "/assets/") {
			c.Next()
			return
		}
		f, err := os.Stat(staticDir + path)
		if err == nil && !f.IsDir() {
			// Real file under the static root: HTML shells revalidate, other
			// unhashed files get the conservative daily cache.
			if prerenderedHTMLFiles[path] {
				c.Header("Cache-Control", cacheHTMLNoCache)
			} else {
				c.Header("Cache-Control", cacheUnhashedDaily)
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		// SPA fallback: deep routes get index.html, which must revalidate —
		// the hashed asset URLs it references are what make long caches safe.
		c.Header("Cache-Control", cacheHTMLNoCache)
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
