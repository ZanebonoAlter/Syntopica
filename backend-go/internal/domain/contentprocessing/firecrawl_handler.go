package contentprocessing

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicextraction"
	"my-robot-backend/internal/platform/aisettings"
	"my-robot-backend/internal/platform/database"
)

type SaveFirecrawlSettingsRequest struct {
	Enabled          bool   `json:"enabled"`
	APIUrl           string `json:"api_url"`
	APIKey           string `json:"api_key"`
	Mode             string `json:"mode"`
	Timeout          int    `json:"timeout"`
	MaxContentLength int    `json:"max_content_length"`
}

func CrawlArticle(c *gin.Context) {
	articleID := c.Param("id")

	var article models.Article
	if err := database.DB.First(&article, articleID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Article not found",
		})
		return
	}

	var feed models.Feed
	if err := database.DB.First(&feed, article.FeedID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Feed not found",
		})
		return
	}

	if !feed.FirecrawlEnabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Firecrawl not enabled for this feed",
		})
		return
	}

	config, err := GetFirecrawlConfig()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if !config.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Firecrawl is not enabled globally",
		})
		return
	}

	firecrawlService := NewFirecrawlService(config)

	article.FirecrawlStatus = "processing"
	database.DB.Save(&article)

	result, err := firecrawlService.ScrapePage(c.Request.Context(), article.Link)
	if err != nil {
		article.FirecrawlStatus = "failed"
		article.FirecrawlError = err.Error()
		database.DB.Save(&article)

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	article.FirecrawlStatus = "completed"
	article.FirecrawlContent = result.Data.Markdown
	article.FirecrawlError = ""
	article.SummaryStatus = "incomplete"
	now := time.Now()
	article.FirecrawlCrawledAt = &now
	database.DB.Save(&article)
	if feed.TaggingEnabled {
		_ = topicextraction.NewTagJobQueue(database.DB).Enqueue(topicextraction.TagJobRequest{
			ArticleID:    article.ID,
			FeedName:     feed.Title,
			CategoryName: topicextraction.FeedCategoryName(feed),
			ForceRetag:   true,
			Reason:       "manual_firecrawl_completed",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"firecrawl_content": result.Data.Markdown,
			"firecrawl_status":  "completed",
			"summary_status":    "incomplete",
		},
	})
}

func EnableFeedFirecrawl(c *gin.Context) {
	feedID := c.Param("id")

	var req struct {
		Enabled bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	var feed models.Feed
	if err := database.DB.First(&feed, feedID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Feed not found",
		})
		return
	}

	feed.FirecrawlEnabled = req.Enabled
	database.DB.Save(&feed)

	if req.Enabled {
		database.DB.Model(&models.Article{}).
			Where("feed_id = ?", feed.ID).
			Where("(firecrawl_content IS NULL OR firecrawl_content = '') AND firecrawl_status <> ?", "processing").
			Updates(map[string]interface{}{
				"firecrawl_status": "pending",
				"firecrawl_error":  "",
			})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"firecrawl_enabled": feed.FirecrawlEnabled,
		},
	})
}

func GetFirecrawlStatus(c *gin.Context) {
	config, err := GetFirecrawlConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"enabled": false,
				"error":   err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":            config.Enabled,
			"api_url":            config.APIUrl,
			"mode":               config.Mode,
			"timeout":            config.Timeout,
			"max_content_length": config.MaxContentLength,
			"api_key_configured": config.APIKey != "",
		},
	})
}

func SaveFirecrawlSettings(c *gin.Context) {
	var req SaveFirecrawlSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	if req.Mode == "" {
		req.Mode = "scrape"
	}
	if req.Timeout <= 0 {
		req.Timeout = 60
	}
	if req.MaxContentLength <= 0 {
		req.MaxContentLength = 50000
	}

	configJSON, _, err := aisettings.LoadFirecrawlConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	apiKey := req.APIKey
	if apiKey == "" {
		apiKey = GetFirecrawlAPIKey(configJSON)
	}

	configJSON = map[string]interface{}{
		"enabled":            req.Enabled,
		"api_url":            req.APIUrl,
		"api_key":            apiKey,
		"mode":               req.Mode,
		"timeout":            req.Timeout,
		"max_content_length": req.MaxContentLength,
	}

	if err := aisettings.SaveFirecrawlConfig(configJSON, "Firecrawl configuration"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Firecrawl settings saved successfully",
		"data": gin.H{
			"enabled":            req.Enabled,
			"api_url":            req.APIUrl,
			"mode":               req.Mode,
			"timeout":            req.Timeout,
			"max_content_length": req.MaxContentLength,
			"api_key_configured": apiKey != "",
		},
	})
}
