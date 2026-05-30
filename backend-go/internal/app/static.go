package app

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func SetupStaticFiles(r *gin.Engine) {
	staticDir := "frontend"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		return
	}

	r.Use(spaFallback(staticDir))
	r.Static("/assets", staticDir+"/assets")
	r.StaticFile("/favicon.png", staticDir+"/favicon.png")
	for _, dir := range []string{"_nuxt", "icons"} {
		if _, err := os.Stat(staticDir + "/" + dir); err == nil {
			r.Static("/"+dir, staticDir+"/"+dir)
		}
	}
}

func spaFallback(staticDir string) gin.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticDir))
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || path == "/ws" || path == "/health" {
			c.Next()
			return
		}
		f, err := os.Stat(staticDir + path)
		if err == nil && !f.IsDir() {
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}
