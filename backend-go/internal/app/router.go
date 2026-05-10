package app

import (
	"github.com/gin-gonic/gin"
	aiadmindomain "my-robot-backend/internal/domain/aiadmin"
	articlesdomain "my-robot-backend/internal/domain/articles"
	categoriesdomain "my-robot-backend/internal/domain/categories"
	contentprocessingdomain "my-robot-backend/internal/domain/contentprocessing"
	feedsdomain "my-robot-backend/internal/domain/feeds"
	narrativedomain "my-robot-backend/internal/domain/narrative"
	preferencesdomain "my-robot-backend/internal/domain/preferences"
	topicanalysisdomain "my-robot-backend/internal/domain/topicanalysis"
	topicextractiondomain "my-robot-backend/internal/domain/topicextraction"
	topicgraphdomain "my-robot-backend/internal/domain/topicgraph"
	"my-robot-backend/internal/jobs"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/tracing"
	"my-robot-backend/internal/platform/ws"
)

func SetupRoutes(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"name":    "RSS Reader API (Go)",
			"version": "1.0.0",
			"endpoints": gin.H{
				"categories": "/api/categories",
				"feeds":      "/api/feeds",
				"articles":   "/api/articles",
				"ai":         "/api/ai",
				"opml": gin.H{
					"import": "POST /api/import-opml",
					"export": "GET /api/export-opml",
				},
				"schedulers": "/api/schedulers",
			},
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":   "healthy",
			"database": "connected",
		})
	})

	r.GET("/api/tasks/status", jobs.GetTasksStatus)

	r.GET("/ws", ws.HandleWebSocket)

	api := r.Group("/api")
	{
		categories := api.Group("/categories")
		{
			categories.GET("", categoriesdomain.GetCategories)
			categories.POST("", categoriesdomain.CreateCategory)
			categories.PUT("/:category_id", categoriesdomain.UpdateCategory)
			categories.DELETE("/:category_id", categoriesdomain.DeleteCategory)
		}

		feeds := api.Group("/feeds")
		{
			feeds.GET("", feedsdomain.GetFeeds)
			feeds.GET("/:feed_id", feedsdomain.GetFeed)
			feeds.POST("", feedsdomain.CreateFeed)
			feeds.PUT("/:feed_id", feedsdomain.UpdateFeed)
			feeds.DELETE("/:feed_id", feedsdomain.DeleteFeed)
			feeds.POST("/:feed_id/refresh", feedsdomain.RefreshFeed)
			feeds.POST("/fetch", feedsdomain.FetchFeed)
			feeds.POST("/refresh-all", feedsdomain.RefreshAllFeeds)
		}

		articles := api.Group("/articles")
		{
			articles.GET("/stats", articlesdomain.GetArticlesStats)
			articles.GET("", articlesdomain.GetArticles)
			articles.GET("/:article_id", articlesdomain.GetArticle)
			articles.POST("/:article_id/tags", articlesdomain.RetagArticleHandler)
			articles.PUT("/:article_id", articlesdomain.UpdateArticle)
			articles.PUT("/bulk-update", articlesdomain.BulkUpdateArticles)
		}

		ai := api.Group("/ai")
		{
			ai.GET("/providers", aiadmindomain.ListProviders)
			ai.POST("/providers", aiadmindomain.UpsertProvider)
			ai.PUT("/providers/:provider_id", aiadmindomain.UpdateProvider)
			ai.DELETE("/providers/:provider_id", aiadmindomain.DeleteProvider)
			ai.GET("/routes", aiadmindomain.ListRoutes)
			ai.PUT("/routes/:capability", aiadmindomain.UpdateRoute)
			ai.GET("/settings", aiadmindomain.GetSettings)
			ai.POST("/settings", aiadmindomain.SaveSettings)
		}

		opml := api.Group("")
		{
			opml.POST("/import-opml", feedsdomain.ImportOPML)
			opml.GET("/export-opml", feedsdomain.ExportOPML)
		}

		schedulers := api.Group("/schedulers")
		{
			schedulers.GET("/status", jobs.GetSchedulersStatus)
			schedulers.GET("/:name/status", jobs.GetSchedulerStatus)
			schedulers.POST("/:name/trigger", jobs.TriggerScheduler)
			schedulers.POST("/:name/reset", jobs.ResetSchedulerStats)
			schedulers.PUT("/:name/interval", jobs.UpdateSchedulerInterval)
		}

		readingBehavior := api.Group("/reading-behavior")
		{
			readingBehavior.POST("/track", preferencesdomain.TrackReadingBehavior)
			readingBehavior.POST("/track-batch", preferencesdomain.BatchTrackReadingBehavior)
			readingBehavior.GET("/stats", preferencesdomain.GetReadingStats)
		}

		preferences := api.Group("/user-preferences")
		{
			preferences.GET("", preferencesdomain.GetUserPreferences)
			preferences.POST("/update", preferencesdomain.TriggerPreferenceUpdate)
		}

		contentCompletion := api.Group("/content-completion")
		{
			contentCompletion.POST("/articles/:article_id/complete", contentprocessingdomain.CompleteArticleContent)
			contentCompletion.POST("/feeds/:feed_id/complete-all", contentprocessingdomain.CompleteFeedArticles)
			contentCompletion.GET("/articles/:article_id/status", contentprocessingdomain.GetCompletionStatus)
			contentCompletion.GET("/overview", contentprocessingdomain.GetCompletionOverview)
		}

		firecrawl := api.Group("/firecrawl")
		{
			firecrawl.POST("/article/:id", contentprocessingdomain.CrawlArticle)
			firecrawl.POST("/feed/:id/enable", contentprocessingdomain.EnableFeedFirecrawl)
			firecrawl.GET("/status", contentprocessingdomain.GetFirecrawlStatus)
			firecrawl.POST("/settings", contentprocessingdomain.SaveFirecrawlSettings)
		}

		topicGraph := api.Group("/topic-graph")
		{
			topicGraph.GET("/:type", topicgraphdomain.GetTopicGraph)
			topicGraph.GET("/topic/:slug", topicgraphdomain.GetTopicDetail)
			topicGraph.GET("/by-category", topicgraphdomain.GetTopicsByCategory)
			topicGraph.GET("/tag/:slug/digests", topicgraphdomain.GetDigestsByArticleTagHandler)
			topicGraph.GET("/tag/:slug/pending-articles", topicgraphdomain.GetPendingArticlesByTagHandler)
			topicGraph.GET("/topic/:slug/articles", topicgraphdomain.GetTopicArticles)
		}
		topicanalysisdomain.RegisterAnalysisRoutes(topicGraph, topicanalysisdomain.GetAnalysisService(database.DB))
		topicanalysisdomain.RegisterEmbeddingConfigRoutes(api)
		topicanalysisdomain.RegisterEmbeddingQueueRoutes(api)
		topicanalysisdomain.RegisterMergeReembeddingQueueRoutes(api)
		topicanalysisdomain.RegisterAbstractTagUpdateQueueRoutes(api)
		topicanalysisdomain.RegisterAdoptNarrowerQueueRoutes(api)
		topicextractiondomain.RegisterTagQueueRoutes(api)
		topicanalysisdomain.RegisterTagManagementRoutes(api)
		topicanalysisdomain.RegisterWatchedTagsRoutes(api)
		topicanalysisdomain.RegisterTagMergePreviewRoutes(api)
		topicanalysisdomain.RegisterAbstractTagRoutes(api)
		topicanalysisdomain.RegisterHierarchyRoutes(api.Group("/hierarchy"))

		narrativedomain.RegisterNarrativeRoutes(api)

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
