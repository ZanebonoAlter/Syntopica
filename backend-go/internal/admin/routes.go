package admin

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all admin module routes under the given router group.
func RegisterRoutes(rg *gin.RouterGroup) {
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

	preferenceProfile := rg.Group("/preference-profile")
	{
		preferenceProfile.GET("", GetPreferenceProfile)
		preferenceProfile.POST("/recompute", RecomputePreferenceProfile)
	}

	discovery := rg.Group("/discovery")
	{
		discovery.POST("/catalog/sync", SyncCatalog)
		discovery.GET("/catalog/status", GetCatalogStatus)
		discovery.GET("/recommendations", GetRecommendations)
		discovery.POST("/recommendations/refresh", RefreshRecommendations)
		discovery.POST("/recommendations/:id/accept", AcceptRecommendation)
		discovery.POST("/recommendations/:id/dismiss", DismissRecommendation)
		// 生命周期用户动作（4.4 / D5）：长期排除与恢复资格。
		discovery.POST("/recommendations/:id/exclude", ExcludeRecommendation)
		discovery.POST("/recommendations/:id/restore", RestoreRecommendation)
		discovery.POST("/ask", Ask)
		// 手动查询 run 详情（improve-discovery-recommendations，design D2/D9）
		discovery.GET("/runs/:id", GetDiscoveryRun)
		// 兴趣记录列表（improve-discovery-recommendations High 1 修复，design D9）
		discovery.GET("/interests", GetInterests)

		// 候选源库（improve-discovery-recommendations，design D9）
		discovery.GET("/candidates", ListCandidates)
		discovery.POST("/candidates", CreateCandidate)
		discovery.GET("/candidates/:id", GetCandidate)
		discovery.PATCH("/candidates/:id", UpdateCandidate)
		// 可用性检查（3.3，design D7）：:id 同名子路由与 import/* 静态段并存（静态优先）。
		discovery.POST("/candidates/:id/check", CheckCandidate)
		// 私网访问授权确认（Medium 7，design D9/C5）：private_pending → private_allowed。
		discovery.POST("/candidates/:id/access-confirm", ConfirmCandidateAccess)
		// 目录导入导出（3.2，design D8）：export 与 :id 同段静态路由优先于参数路由。
		discovery.GET("/candidates/export", ExportCandidates)
		discovery.POST("/candidates/import/preview", PreviewCatalogImport)
		discovery.POST("/candidates/import/confirm", ConfirmCatalogImport)
	}

	settings := rg.Group("/settings")
	{
		settings.GET("/rsshub", GetRSSHubSettings)
		settings.POST("/rsshub", SaveRSSHubSettings)
		settings.GET("/proxy", GetProxySettings)
		settings.POST("/proxy", SaveProxySettings)
		settings.GET("/bocha", GetBochaSettings)
		settings.POST("/bocha", SaveBochaSettings)
	}

	notifications := rg.Group("/notifications")
	{
		notifications.GET("", ListNotifications)
		notifications.GET("/unread-count", GetUnreadCount)
		notifications.POST("/:id/read", MarkNotificationRead)
		notifications.POST("/read-all", MarkAllNotificationsRead)
		notifications.DELETE("", ClearNotifications)
	}

	// 路由参数可选值字典 CRUD（feed-param-options）
	routeParamOptions := rg.Group("/admin/route-param-options")
	{
		routeParamOptions.GET("", ListRouteParamOptions)
		routeParamOptions.POST("", CreateRouteParamOption)
		routeParamOptions.PUT("/:id", UpdateRouteParamOption)
		routeParamOptions.DELETE("/:id", DeleteRouteParamOption)
	}
}
