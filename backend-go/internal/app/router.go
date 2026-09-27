package app

import (
	"os"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/admin"
	"syntopica-backend/internal/dataenrichment"
	wiring "syntopica-backend/internal/datasources/wiring"
	"syntopica-backend/internal/discovery"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/imageproxy"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/middleware"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/platform/ws"
	"syntopica-backend/internal/reader"
	tagmanagement "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph"
)

func SetupRoutes(r *gin.Engine) {
	// Serve locally downloaded feed icons at /icons — independent of the
	// frontend build output (static.go) so dev and production both reach them
	// through the backend origin (port 5000).
	iconDir := reader.IconStorageDir()
	if err := os.MkdirAll(iconDir, 0o750); err != nil {
		logging.Warnf("Failed to create icon storage dir %q: %v", iconDir, err)
	}
	// Serve /icons through a wrapped FileServer (not r.Static) so every
	// response carries the security headers that neutralize stored SVG XSS.
	r.GET("/icons/*filepath", iconsFileHandler(iconDir))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":   "healthy",
			"database": "connected",
		})
	})

	r.GET("/api/tasks/status", admin.GetTasksStatus)

	// In public read-only demo mode, the WebSocket endpoint is unused (the
	// frontend degrades silently) and background schedulers are disabled, so we
	// skip registering it to avoid an unused connection surface.
	if !middleware.IsDemoReadOnly() {
		r.GET("/ws", ws.HandleWebSocket)
	}

	api := r.Group("/api")
	// Enforce read-only access when DEMO_READ_ONLY=1; no-op in production.
	api.Use(middleware.ReadOnly())
	{
		reader.RegisterRoutes(api)
		topicgraph.RegisterRoutes(api)
		admin.RegisterRoutes(api)
		discovery.RegisterRoutes(api)
		imageproxy.RegisterRoutes(api)

		tagmanagement.RegisterRoutes(api)
		dataenrichment.RegisterRoutes(api)
		wiring.RegisterRoutes(api)

		traceHandler := tracing.NewTraceHandler(database.DB)
		traces := api.Group("/traces")
		{
			traces.GET("", traceHandler.GetTraceByTraceID)
			traces.GET("/recent", traceHandler.GetRecentTraces)
			traces.GET("/search", traceHandler.SearchTraces)
			traces.GET("/stats", traceHandler.GetTraceStats)
			traces.GET("/:trace_id/timeline", traceHandler.GetTraceTimeline)
			traces.GET("/:trace_id/otlp", traceHandler.ExportTraceOTLP)
		}
	}
}
