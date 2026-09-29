package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/service"
)

var completionService *service.ContentCompletionService

// schedulerLookup is set by StartRuntime to provide scheduler access.
var schedulerLookup func(name string) interface{}

// SetSchedulerLookup sets the function used to look up schedulers by name.
func SetSchedulerLookup(fn func(name string) interface{}) {
	schedulerLookup = fn
}

func InitContentCompletionHandler() {
	completionService = service.NewContentCompletionService()
	loadCompletionAISettings()
}

func loadCompletionAISettings() {
	if completionService == nil {
		return
	}

	model, vendor, _, err := airouter.NewRouter().ResolvePrimaryModel(airouter.CapabilitySummary)
	if err == nil && model != nil && vendor != nil && vendor.BaseURL != "" && model.Name != "" {
		completionService.SetAICredentials(vendor.BaseURL, vendor.APIKey, model.Name)
		return
	}

	// Fallback: use any enabled model (under an enabled vendor) when no
	// summary-specific route exists. This handles the common case where the
	// user configured an AI vendor/model but did not create a dedicated route
	// for the summary capability.
	store := airouter.NewStore(nil)
	vendors, err := store.ListVendors()
	if err != nil {
		return
	}
	enabledVendor := make(map[uint]models.AIVendor, len(vendors))
	for _, v := range vendors {
		if v.Enabled {
			enabledVendor[v.ID] = v
		}
	}
	modelList, err := store.ListModels()
	if err != nil {
		return
	}
	for _, m := range modelList {
		if m.Enabled && m.Status == airouter.ModelStatusOK {
			if v, ok := enabledVendor[m.VendorID]; ok && v.BaseURL != "" && m.Name != "" {
				completionService.SetAICredentials(v.BaseURL, v.APIKey, m.Name)
				return
			}
		}
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

	// Reload AI settings to pick up configuration changes made after startup.
	loadCompletionAISettings()

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

	// Reload AI settings to pick up configuration changes made after startup.
	loadCompletionAISettings()

	articles, err := repository.Repo.ListArticlesByFeedAndStatuses(feedID, []string{"incomplete", "failed"})
	if err != nil {
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

	articleID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid article ID"})
		return
	}

	article, err := repository.Repo.GetArticle(uint(articleID))
	if err != nil {
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

	if schedulerLookup != nil {
		if scheduler, ok := schedulerLookup("content_completion").(interface{ GetStatus() map[string]interface{} }); ok {
			status := scheduler.GetStatus()
			for _, key := range []string{"is_executing", "current_article", "last_processed", "next_run", "last_error", "database_state", "overview"} {
				if value, exists := status[key]; exists {
					data[key] = value
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func GetContentCompletionService() *service.ContentCompletionService {
	return completionService
}
