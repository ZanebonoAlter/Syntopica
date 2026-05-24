package content

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/app/runtimeinfo"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

var completionService *ContentCompletionService

func InitContentCompletionHandler() {
	completionService = NewContentCompletionService()
	loadCompletionAISettings()
}

func loadCompletionAISettings() {
	if completionService == nil {
		return
	}

	provider, _, err := airouter.NewRouter().ResolvePrimaryProvider(airouter.CapabilityArticleCompletion)
	if err == nil && provider != nil && provider.BaseURL != "" && provider.APIKey != "" && provider.Model != "" {
		completionService.SetAICredentials(provider.BaseURL, provider.APIKey, provider.Model)
	}
}

func SetCompletionAICredentials(baseURL, apiKey, model string) {
	if completionService != nil {
		completionService.SetAICredentials(baseURL, apiKey, model)
	}
}

func CompleteArticleContent(c *gin.Context) {
	id := c.Param("article_id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Article ID is required"})
		return
	}

	var articleID uint
	if _, err := fmt.Sscanf(id, "%d", &articleID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid article ID"})
		return
	}

	if completionService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Content completion service not initialized"})
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)

	if err := completionService.CompleteArticleWithForce(c.Request.Context(), articleID, req.Force); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Content completion initiated"})
}

func CompleteFeedArticles(c *gin.Context) {
	id := c.Param("feed_id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Feed ID is required"})
		return
	}

	var feedID uint
	if _, err := fmt.Sscanf(id, "%d", &feedID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid feed ID"})
		return
	}

	if completionService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Content completion service not initialized"})
		return
	}

	var articles []models.Article
	if err := database.DB.Omit("tag_count", "relevance_score").Where("feed_id = ? AND summary_status IN ?", feedID, []string{"incomplete", "failed"}).Find(&articles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	completed := 0
	failed := 0

	for _, article := range articles {
		if err := completionService.CompleteArticleWithForce(c.Request.Context(), article.ID, true); err != nil {
			failed++
		} else {
			completed++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"completed": completed,
		"failed":    failed,
		"total":     len(articles),
	})
}

func GetCompletionStatus(c *gin.Context) {
	id := c.Param("article_id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Article ID is required"})
		return
	}

	var article models.Article
	if err := database.DB.First(&article, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Article not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"summary_status":       article.SummaryStatus,
			"attempts":             article.CompletionAttempts,
			"error":                article.CompletionError,
			"summary_generated_at": article.SummaryGeneratedAt,
			"ai_content_summary":   article.AIContentSummary,
			"firecrawl_content":    article.FirecrawlContent,
			"firecrawl_status":     article.FirecrawlStatus,
			"firecrawl_error":      article.FirecrawlError,
			"firecrawl_crawled_at": article.FirecrawlCrawledAt,
		},
	})
}

func GetCompletionOverview(c *gin.Context) {
	if completionService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Content completion service not initialized"})
		return
	}

	overview, err := completionService.GetOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	data := gin.H{
		"pending_count":    overview.PendingCount,
		"processing_count": overview.ProcessingCount,
		"completed_count":  overview.CompletedCount,
		"failed_count":     overview.FailedCount,
		"blocked_count":    overview.BlockedCount,
		"total_count":      overview.TotalCount,
		"ai_configured":    overview.AIConfigured,
		"blocked_reasons": gin.H{
			"waiting_for_firecrawl_count":     overview.BlockedReasons.WaitingForFirecrawlCount,
			"feed_disabled_count":             overview.BlockedReasons.FeedDisabledCount,
			"ai_unconfigured_count":           overview.BlockedReasons.AIUnconfiguredCount,
			"ready_but_missing_content_count": overview.BlockedReasons.ReadyButMissingContentCount,
		},
	}

	if scheduler, ok := runtimeinfo.ContentCompletionSchedulerInterface.(interface{ GetStatus() map[string]interface{} }); ok {
		status := scheduler.GetStatus()
		for _, key := range []string{"is_executing", "current_article", "last_processed", "next_run", "last_error", "database_state", "overview"} {
			if value, exists := status[key]; exists {
				data[key] = value
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func GetContentCompletionService() *ContentCompletionService {
	return completionService
}
