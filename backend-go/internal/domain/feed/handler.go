package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

type CreateFeedRequest struct {
	Title                 string `json:"title"`
	Description           string `json:"description"`
	URL                   string `json:"url" binding:"required"`
	CategoryID            *uint  `json:"category_id"`
	Icon                  string `json:"icon"`
	Color                 string `json:"color"`
	MaxArticles           int    `json:"max_articles"`
	RefreshInterval       int    `json:"refresh_interval"`
	ArticleSummaryEnabled bool   `json:"article_summary_enabled"`
	CompletionOnRefresh   bool   `json:"completion_on_refresh"`
	MaxCompletionRetries  int    `json:"max_completion_retries"`
	FirecrawlEnabled      bool   `json:"firecrawl_enabled"`
	TaggingEnabled        bool   `json:"tagging_enabled"`
}

type UpdateFeedRequest struct {
	Title                 string `json:"title"`
	Description           string `json:"description"`
	URL                   string `json:"url"`
	CategoryID            *uint  `json:"category_id"`
	Icon                  string `json:"icon"`
	Color                 string `json:"color"`
	MaxArticles           int    `json:"max_articles"`
	RefreshInterval       int    `json:"refresh_interval"`
	ArticleSummaryEnabled *bool  `json:"article_summary_enabled"`
	CompletionOnRefresh   *bool  `json:"completion_on_refresh"`
	MaxCompletionRetries  *int   `json:"max_completion_retries"`
	FirecrawlEnabled      *bool  `json:"firecrawl_enabled"`
	TaggingEnabled        *bool  `json:"tagging_enabled"`
}

func GetFeeds(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	categoryID, _ := strconv.Atoi(c.Query("category_id"))
	uncategorized := c.Query("uncategorized") == "true"

	query := database.DB.Model(&models.Feed{})

	if categoryID > 0 {
		query = query.Where("category_id = ?", categoryID)
	}

	if uncategorized {
		query = query.Where("category_id IS NULL")
	}

	var total int64
	query.Count(&total)

	var feeds []models.Feed
	if err := query.Order("title ASC").Find(&feeds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	feedIDs := make([]uint, len(feeds))
	for i, f := range feeds {
		feedIDs[i] = f.ID
	}

	type FeedStatRow struct {
		FeedID       uint
		ArticleCount int
		UnreadCount  int
	}
	var statRows []FeedStatRow
	if len(feedIDs) > 0 {
		database.DB.Model(&models.Article{}).
			Select("feed_id, COUNT(*) as article_count, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread_count").
			Where("feed_id IN ?", feedIDs).
			Group("feed_id").
			Scan(&statRows)
	}

	statMap := make(map[uint]models.FeedStats, len(statRows))
	for _, row := range statRows {
		statMap[row.FeedID] = models.FeedStats{
			ArticleCount: row.ArticleCount,
			UnreadCount:  row.UnreadCount,
		}
	}

	data := make([]map[string]interface{}, 0, len(feeds))
	start := 0
	if perPage < 10000 {
		start = (page - 1) * perPage
		if start >= len(feeds) {
			start = len(feeds)
		}
	}
	end := len(feeds)
	if perPage < 10000 {
		end = start + perPage
		if end > len(feeds) {
			end = len(feeds)
		}
	}

	for i := start; i < end; i++ {
		stats := statMap[feeds[i].ID]
		data = append(data, feeds[i].ToDict(&stats))
	}

	resultPage := page
	resultPerPage := perPage
	if perPage >= 10000 {
		resultPage = 1
		resultPerPage = len(feeds)
	}
	if resultPerPage == 0 {
		resultPerPage = 1
	}

	pages := int(total) / resultPerPage
	if int(total)%resultPerPage > 0 {
		pages++
	}
	if perPage >= 10000 {
		pages = 1
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
		"pagination": gin.H{
			"page":     resultPage,
			"per_page": resultPerPage,
			"total":    total,
			"pages":    pages,
		},
	})
}

func GetFeed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("feed_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid feed ID",
		})
		return
	}

	var feed models.Feed
	if err := database.DB.First(&feed, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Feed not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	var stats models.FeedStats
	database.DB.Model(&models.Article{}).
		Select("COUNT(*) as article_count, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread_count").
		Where("feed_id = ?", feed.ID).
		Group("feed_id").
		Scan(&stats)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    feed.ToDict(&stats),
	})
}

func CreateFeed(c *gin.Context) {
	var req CreateFeedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Feed URL is required",
		})
		return
	}

	var existing models.Feed
	if err := database.DB.Where("url = ?", req.URL).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "Feed with this URL already exists",
		})
		return
	}

	now := time.Now()
	feed := models.Feed{
		Title:                 req.Title,
		Description:           req.Description,
		URL:                   req.URL,
		CategoryID:            req.CategoryID,
		Icon:                  req.Icon,
		Color:                 req.Color,
		MaxArticles:           req.MaxArticles,
		RefreshInterval:       req.RefreshInterval,
		ArticleSummaryEnabled: req.ArticleSummaryEnabled,
		CompletionOnRefresh:   req.CompletionOnRefresh,
		MaxCompletionRetries:  req.MaxCompletionRetries,
		FirecrawlEnabled:      req.FirecrawlEnabled,
		TaggingEnabled:        req.TaggingEnabled,
		LastUpdated:           &now,
	}

	if feed.Title == "" {
		feed.Title = "Untitled Feed"
	}
	if feed.Icon == "" {
		feed.Icon = "mdi:rss"
	}
	if feed.Color == "" {
		feed.Color = "#8b5cf6"
	}
	if feed.MaxArticles == 0 {
		feed.MaxArticles = 100
	}
	if feed.RefreshInterval == 0 {
		feed.RefreshInterval = 60
	}

	if err := database.DB.Create(&feed).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    feed.ToDict(&models.FeedStats{}),
		"message": "Feed created successfully",
	})
}

func UpdateFeed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("feed_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid feed ID",
		})
		return
	}

	var feed models.Feed
	if err := database.DB.First(&feed, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Feed not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	// Read raw body first to preserve it for later field checking
	rawBody, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to read request body",
		})
		return
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(rawBody, &bodyMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Failed to parse JSON",
		})
		return
	}

	var req UpdateFeedRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	updates := make(map[string]interface{})
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.URL != "" {
		updates["url"] = req.URL
	}
	// Only update category_id if explicitly provided (not zero/nil)
	if req.CategoryID != nil {
		updates["category_id"] = *req.CategoryID
	}
	if req.Icon != "" {
		updates["icon"] = req.Icon
	}
	if req.Color != "" {
		updates["color"] = req.Color
	}
	if _, exists := bodyMap["max_articles"]; exists {
		updates["max_articles"] = req.MaxArticles
	}
	if req.RefreshInterval >= 0 {
		updates["refresh_interval"] = req.RefreshInterval
	}
	if _, exists := bodyMap["article_summary_enabled"]; exists && req.ArticleSummaryEnabled != nil {
		updates["article_summary_enabled"] = *req.ArticleSummaryEnabled
	}
	if _, exists := bodyMap["completion_on_refresh"]; exists && req.CompletionOnRefresh != nil {
		updates["completion_on_refresh"] = *req.CompletionOnRefresh
	}
	if _, exists := bodyMap["max_completion_retries"]; exists && req.MaxCompletionRetries != nil {
		updates["max_completion_retries"] = *req.MaxCompletionRetries
	}
	if _, exists := bodyMap["firecrawl_enabled"]; exists && req.FirecrawlEnabled != nil {
		updates["firecrawl_enabled"] = *req.FirecrawlEnabled
	}
	if _, exists := bodyMap["tagging_enabled"]; exists && req.TaggingEnabled != nil {
		updates["tagging_enabled"] = *req.TaggingEnabled
	}

	if err := database.DB.Model(&feed).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Trigger cleanup if MaxArticles was updated
	if _, exists := bodyMap["max_articles"]; exists {
		database.DB.First(&feed, feed.ID)
		feed.MaxArticles = req.MaxArticles
		NewFeedService().CleanupOldArticles(&feed)
	}

	var stats models.FeedStats
	database.DB.Model(&models.Article{}).
		Select("COUNT(*) as article_count, SUM(CASE WHEN NOT read THEN 1 ELSE 0 END) as unread_count").
		Where("feed_id = ?", feed.ID).
		Group("feed_id").
		Scan(&stats)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    feed.ToDict(&stats),
	})
}

func DeleteFeed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("feed_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid feed ID",
		})
		return
	}

	var feed models.Feed
	if err := database.DB.First(&feed, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Feed not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   err.Error(),
			})
		}
		return
	}

	if err := database.DB.Where("feed_id = ?", feed.ID).Delete(&models.ReadingBehavior{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if err := database.DB.Delete(&feed).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Feed deleted successfully",
	})
}

func RefreshFeed(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("feed_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid feed ID",
		})
		return
	}

	var feed models.Feed
	if err := database.DB.First(&feed, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Feed not found",
		})
		return
	}

	now := time.Now()
	feed.RefreshStatus = "refreshing"
	feed.LastRefreshAt = &now
	feed.RefreshError = ""
	database.DB.Save(&feed)

	go func() {
		refreshFeedWorker(uint(id))
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "Started refreshing feed in background",
	})
}

func refreshFeedWorker(feedID uint) {
	feedService := NewFeedService()
	if err := feedService.RefreshFeed(context.Background(), feedID); err != nil {
		return
	}
}

func FetchFeed(c *gin.Context) {
	var req struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Feed URL is required",
		})
		return
	}

	feedService := NewFeedService()
	title, description, err := feedService.FetchFeedPreview(req.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"title":       title,
			"description": description,
		},
	})
}

func RefreshAllFeeds(c *gin.Context) {
	var feeds []models.Feed
	if err := database.DB.Find(&feeds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	if len(feeds) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "No feeds to refresh",
		})
		return
	}

	feedIDs := make([]uint, len(feeds))
	for i, feed := range feeds {
		feedIDs[i] = feed.ID
	}

	now := time.Now()
	database.DB.Model(&models.Feed{}).Where("id IN ?", feedIDs).
		Updates(map[string]interface{}{
			"refresh_status":  "refreshing",
			"last_refresh_at": &now,
			"refresh_error":   "",
		})

	go func(ids []uint) {
		refreshAllFeedsWorker(ids)
	}(feedIDs)

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "Started refreshing all feeds in background",
		"data": gin.H{
			"total_feeds": len(feeds),
		},
	})
}

func refreshAllFeedsWorker(feedIDs []uint) {
	feedService := NewFeedService()
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for _, id := range feedIDs {
		wg.Add(1)
		sem <- struct{}{}
		go func(feedID uint) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					logging.Errorf("[refresh-all] PANIC refreshing feed %d: %v", feedID, r)
					resetFeedStatus(feedID, fmt.Sprintf("panic: %v", r))
				}
			}()
			if err := feedService.RefreshFeed(context.Background(), feedID); err != nil {
				logging.Errorf("[refresh-all] Error refreshing feed %d: %v", feedID, err)
				// RefreshFeed may not have updated status for some error paths (e.g. feed not found, Save failure)
				resetFeedStatus(feedID, err.Error())
			}
		}(id)
	}
	wg.Wait()
}

// resetFeedStatus sets a feed to "error" status, but only if it's still in "refreshing" state.
// This avoids overwriting a concurrent status change (e.g. from auto-refresh completing it).
func resetFeedStatus(feedID uint, errMsg string) {
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	database.DB.Model(&models.Feed{}).Where("id = ? AND refresh_status = ?", feedID, "refreshing").Updates(map[string]interface{}{
		"refresh_status": "error",
		"refresh_error":  errMsg,
		"last_refresh_at": &now,
	})
}
