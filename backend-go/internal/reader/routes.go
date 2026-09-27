package reader

import (
	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/reader/handler"
)

// RegisterRoutes registers all reader module routes under the given router group.
func RegisterRoutes(rg *gin.RouterGroup) {
	categories := rg.Group("/categories")
	{
		categories.GET("", handler.GetCategories)
		categories.POST("", handler.CreateCategory)
		categories.PUT("/:category_id", handler.UpdateCategory)
		categories.DELETE("/:category_id", handler.DeleteCategory)
	}

	feeds := rg.Group("/feeds")
	{
		feeds.GET("", handler.GetFeeds)
		// 静态段与 /:feed_id 同级共存（先例：articles 组的 /stats 与 /:article_id）。
		feeds.GET("/board-hit-stats", handler.GetBoardHitStats)
		feeds.GET("/:feed_id", handler.GetFeed)
		feeds.POST("", handler.CreateFeed)
		feeds.PUT("/:feed_id", handler.UpdateFeed)
		feeds.DELETE("/:feed_id", handler.DeleteFeed)
		feeds.POST("/:feed_id/refresh", handler.RefreshFeed)
		feeds.POST("/fetch", handler.FetchFeed)
		feeds.POST("/refresh-all", handler.RefreshAllFeeds)
	}

	articles := rg.Group("/articles")
	{
		articles.GET("/stats", handler.GetArticlesStats)
		articles.GET("", handler.GetArticles)
		articles.GET("/:article_id", handler.GetArticle)
		articles.POST("/:article_id/tags", handler.RetagArticleHandler)
		articles.PUT("/:article_id", handler.UpdateArticle)
		articles.PUT("/bulk-update", handler.BulkUpdateArticles)
	}

	opml := rg.Group("")
	{
		opml.POST("/import-opml", handler.ImportOPML)
		opml.GET("/export-opml", handler.ExportOPML)
	}

	contentCompletion := rg.Group("/content-completion")
	{
		contentCompletion.POST("/articles/:article_id/complete", handler.CompleteArticleContent)
		contentCompletion.POST("/feeds/:feed_id/complete-all", handler.CompleteFeedArticles)
		contentCompletion.GET("/articles/:article_id/status", handler.GetCompletionStatus)
		contentCompletion.GET("/overview", handler.GetCompletionOverview)
	}

	firecrawl := rg.Group("/firecrawl")
	{
		firecrawl.POST("/article/:id", handler.CrawlArticle)
		firecrawl.POST("/feed/:id/enable", handler.EnableFeedFirecrawl)
		firecrawl.GET("/status", handler.GetFirecrawlStatus)
		firecrawl.POST("/settings", handler.SaveFirecrawlSettings)
	}
}
