// Package discovery: HTTP route registration, migrated verbatim from
// internal/admin/routes.go (decouple-backend-domains). Path strings are
// byte-identical to the pre-migration registration.
package discovery

import (
	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/discovery/handler"
)

// RegisterRoutes registers all discovery domain routes under /api.
func RegisterRoutes(rg *gin.RouterGroup) {
	preferenceProfile := rg.Group("/preference-profile")
	{
		preferenceProfile.GET("", handler.GetPreferenceProfile)
		preferenceProfile.POST("/recompute", handler.RecomputePreferenceProfile)
	}

	discovery := rg.Group("/discovery")
	{
		discovery.POST("/catalog/sync", handler.SyncCatalog)
		discovery.GET("/catalog/status", handler.GetCatalogStatus)
		discovery.GET("/recommendations", handler.GetRecommendations)
		discovery.POST("/recommendations/refresh", handler.RefreshRecommendations)
		discovery.POST("/recommendations/:id/accept", handler.AcceptRecommendation)
		discovery.POST("/recommendations/:id/dismiss", handler.DismissRecommendation)
		// 生命周期用户动作（4.4 / D5）：长期排除与恢复资格。
		discovery.POST("/recommendations/:id/exclude", handler.ExcludeRecommendation)
		discovery.POST("/recommendations/:id/restore", handler.RestoreRecommendation)
		discovery.POST("/ask", handler.Ask)
		// 手动查询 run 详情（improve-discovery-recommendations，design D2/D9）
		discovery.GET("/runs/:id", handler.GetDiscoveryRun)
		// 兴趣记录列表（improve-discovery-recommendations High 1 修复，design D9）
		discovery.GET("/interests", handler.GetInterests)

		// 候选源库（improve-discovery-recommendations，design D9）
		discovery.GET("/candidates", handler.ListCandidates)
		discovery.POST("/candidates", handler.CreateCandidate)
		discovery.GET("/candidates/:id", handler.GetCandidate)
		discovery.PATCH("/candidates/:id", handler.UpdateCandidate)
		// 可用性检查（3.3，design D7）：:id 同名子路由与 import/* 静态段并存（静态优先）。
		discovery.POST("/candidates/:id/check", handler.CheckCandidate)
		// 私网访问授权确认（Medium 7，design D9/C5）：private_pending → private_allowed。
		discovery.POST("/candidates/:id/access-confirm", handler.ConfirmCandidateAccess)
		// 目录导入导出（3.2，design D8）：export 与 :id 同段静态路由优先于参数路由。
		discovery.GET("/candidates/export", handler.ExportCandidates)
		discovery.POST("/candidates/import/preview", handler.PreviewCatalogImport)
		discovery.POST("/candidates/import/confirm", handler.ConfirmCatalogImport)
	}

	settings := rg.Group("/settings")
	{
		settings.GET("/rsshub", handler.GetRSSHubSettings)
		settings.POST("/rsshub", handler.SaveRSSHubSettings)
		settings.GET("/proxy", handler.GetProxySettings)
		settings.POST("/proxy", handler.SaveProxySettings)
		settings.GET("/bocha", handler.GetBochaSettings)
		settings.POST("/bocha", handler.SaveBochaSettings)
		settings.GET("/searxng", handler.GetSearxngSettings)
		settings.POST("/searxng", handler.SaveSearxngSettings)
	}

	// 路由参数可选值字典 CRUD（feed-param-options）
	routeParamOptions := rg.Group("/admin/route-param-options")
	{
		routeParamOptions.GET("", handler.ListRouteParamOptions)
		routeParamOptions.POST("", handler.CreateRouteParamOption)
		routeParamOptions.PUT("/:id", handler.UpdateRouteParamOption)
		routeParamOptions.DELETE("/:id", handler.DeleteRouteParamOption)
	}
}
