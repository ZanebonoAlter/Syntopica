package admin

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all admin module routes under the given router group.
func RegisterRoutes(rg *gin.RouterGroup) {
	// Single reconciliation endpoint for the frontend's resident status data
	// (schedulers + tag queue + unread count) — client-poll-budget 契约;
	// legacy per-domain endpoints stay registered for already-open tabs.
	rg.GET("/poll", GetPollBundle)

	ai := rg.Group("/ai")
	{
		// vendor/model 两层管理端点（ai-provider-model-overhaul design D7，
		// 旧 /providers* 端点原子删除，不留兼容窗口）。
		ai.GET("/vendors", ListAIVendors)
		ai.POST("/vendors", CreateAIVendor)
		ai.POST("/vendors/test", TestAIVendorInline) // 静态段优先于 :id（gin 同段静/参兄弟）
		ai.GET("/vendors/:id", GetAIVendor)
		ai.PUT("/vendors/:id", UpdateAIVendor)
		ai.DELETE("/vendors/:id", DeleteAIVendor)
		ai.POST("/vendors/:id/models/refresh", RefreshAIVendorModels)
		ai.POST("/vendors/:id/test", TestAIVendor)
		ai.GET("/models", ListAIModels)
		ai.PUT("/models/:id", UpdateAIModel)
		ai.GET("/routes", ListAIRoutes)
		ai.PUT("/routes/:capability", UpdateAIRoute)
		ai.GET("/settings", GetSettings)
		ai.POST("/settings", SaveSettings)
		ai.GET("/call-logs", ListCallLogs)
		ai.GET("/sessions/:session_id", GetSession)
		ai.GET("/health", GetAIHealth)
		ai.PUT("/health/auto-start-models", SetAutoStartModels)
		ai.POST("/health/reprobe", ReprobeAIHealth)
		ai.GET("/observability/degradations", QueryDegradations)
	}

	schedulers := rg.Group("/schedulers")
	{
		schedulers.GET("/status", GetSchedulersStatus)
		schedulers.GET("/:name/status", GetSchedulerStatus)
		schedulers.POST("/:name/trigger", TriggerScheduler)
		schedulers.POST("/:name/reset", ResetSchedulerStats)
		schedulers.PUT("/:name/interval", UpdateSchedulerInterval)
		schedulers.PUT("/:name/schedule-time", UpdateSchedulerScheduleTime)
	}

	analysis := rg.Group("/analysis")
	{
		analysis.GET("/pause", GetAnalysisPause)
		analysis.POST("/pause", SetAnalysisPause)
	}

	readingBehavior := rg.Group("/reading-behavior")
	{
		readingBehavior.POST("/track", TrackReadingBehavior)
		readingBehavior.POST("/track-batch", BatchTrackReadingBehavior)
		readingBehavior.GET("/stats", GetReadingStats)
	}

	// discovery 域 settings 路由（rsshub/proxy/bocha/searxng）已迁
	// internal/discovery/routes.go（decouple-backend-domains）。
	settings := rg.Group("/settings")
	{
		settings.GET("/comtrade", GetComtradeSettings)
		settings.POST("/comtrade", SaveComtradeSettings)
	}

	notifications := rg.Group("/notifications")
	{
		notifications.GET("", ListNotifications)
		notifications.GET("/unread-count", GetUnreadCount)
		notifications.POST("/:id/read", MarkNotificationRead)
		notifications.POST("/read-all", MarkAllNotificationsRead)
		notifications.DELETE("", ClearNotifications)
	}

}
