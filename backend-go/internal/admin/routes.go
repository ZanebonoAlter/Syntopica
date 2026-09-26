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
		ai.GET("/providers", ListProviders)
		ai.POST("/providers", UpsertProvider)
		ai.PUT("/providers/:provider_id", UpdateProvider)
		ai.DELETE("/providers/:provider_id", DeleteProvider)
		ai.GET("/routes", ListRoutes)
		ai.PUT("/routes/:capability", UpdateRoute)
		ai.GET("/settings", GetSettings)
		ai.POST("/settings", SaveSettings)
		ai.POST("/test", TestConnection)
		ai.GET("/call-logs", ListCallLogs)
		ai.GET("/sessions/:session_id", GetSession)
		ai.GET("/health", GetAIHealth)
		ai.PUT("/health/auto-start-models", SetAutoStartModels)
		ai.POST("/health/reprobe", ReprobeAIHealth)
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
