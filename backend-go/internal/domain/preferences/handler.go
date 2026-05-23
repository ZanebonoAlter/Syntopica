package preferences

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/app/runtimeinfo"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type TrackBehaviorRequest struct {
	ArticleID   uint   `json:"article_id" binding:"required"`
	FeedID      uint   `json:"feed_id" binding:"required"`
	CategoryID  *uint  `json:"category_id"`
	SessionID   string `json:"session_id" binding:"required"`
	EventType   string `json:"event_type" binding:"required"`
	ScrollDepth int    `json:"scroll_depth"`
	ReadingTime int    `json:"reading_time"`
}

type BatchTrackBehaviorRequest struct {
	Events []TrackBehaviorRequest `json:"events" binding:"required"`
}

func TrackReadingBehavior(c *gin.Context) {
	var req TrackBehaviorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	categoryID := req.CategoryID

	if categoryID == nil || *categoryID == 0 {
		var feed models.Feed
		if err := database.DB.Select("category_id").First(&feed, req.FeedID).Error; err == nil {
			categoryID = feed.CategoryID
		}
	}

	behavior := models.ReadingBehavior{
		ArticleID:   req.ArticleID,
		FeedID:      req.FeedID,
		CategoryID:  categoryID,
		SessionID:   req.SessionID,
		EventType:   req.EventType,
		ScrollDepth: req.ScrollDepth,
		ReadingTime: req.ReadingTime,
		CreatedAt:   time.Now(),
	}

	if err := database.DB.Create(&behavior).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    behavior.ToDict(),
	})
}

func BatchTrackReadingBehavior(c *gin.Context) {
	var req BatchTrackBehaviorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	feedCategoryMap := make(map[uint]*uint)

	behaviors := make([]models.ReadingBehavior, len(req.Events))
	for i, event := range req.Events {
		categoryID := event.CategoryID

		if categoryID == nil || *categoryID == 0 {
			if cachedCategoryID, exists := feedCategoryMap[event.FeedID]; exists {
				categoryID = cachedCategoryID
			} else {
				var feed models.Feed
				if err := database.DB.Select("category_id").First(&feed, event.FeedID).Error; err == nil {
					categoryID = feed.CategoryID
					feedCategoryMap[event.FeedID] = categoryID
				}
			}
		}

		behaviors[i] = models.ReadingBehavior{
			ArticleID:   event.ArticleID,
			FeedID:      event.FeedID,
			CategoryID:  categoryID,
			SessionID:   event.SessionID,
			EventType:   event.EventType,
			ScrollDepth: event.ScrollDepth,
			ReadingTime: event.ReadingTime,
			CreatedAt:   time.Now(),
		}
	}

	if err := database.DB.Create(&behaviors).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": len(behaviors),
	})
}

func GetReadingStats(c *gin.Context) {
	var stats struct {
		TotalArticles      int64   `json:"total_articles"`
		TotalReadingTime   int     `json:"total_reading_time"`
		AvgReadingTime     float64 `json:"avg_reading_time"`
		AvgScrollDepth     float64 `json:"avg_scroll_depth"`
		MostActiveFeedID   uint    `json:"most_active_feed_id"`
		MostActiveCategory uint    `json:"most_active_category"`
	}

	database.DB.Model(&models.ReadingBehavior{}).
		Distinct("article_id").
		Count(&stats.TotalArticles)

	database.DB.Model(&models.ReadingBehavior{}).
		Select("COALESCE(SUM(reading_time), 0)").
		Scan(&stats.TotalReadingTime)

	var avgTime sql.NullFloat64
	database.DB.Model(&models.ReadingBehavior{}).
		Where("reading_time > 0").
		Select("AVG(reading_time)").
		Scan(&avgTime)
	if avgTime.Valid {
		stats.AvgReadingTime = avgTime.Float64
	}

	var avgDepth sql.NullFloat64
	database.DB.Model(&models.ReadingBehavior{}).
		Where("scroll_depth > 0").
		Select("AVG(scroll_depth)").
		Scan(&avgDepth)
	if avgDepth.Valid {
		stats.AvgScrollDepth = avgDepth.Float64
	}

	type FeedCount struct {
		FeedID uint
		Count  int64
	}
	var feedCounts []FeedCount
	database.DB.Model(&models.ReadingBehavior{}).
		Select("feed_id, COUNT(*) as count").
		Group("feed_id").
		Order("count DESC").
		Limit(1).
		Scan(&feedCounts)
	if len(feedCounts) > 0 {
		stats.MostActiveFeedID = feedCounts[0].FeedID
	}

	type CategoryCount struct {
		CategoryID *uint
		Count      int64
	}
	var categoryCounts []CategoryCount
	database.DB.Model(&models.ReadingBehavior{}).
		Select("category_id, COUNT(*) as count").
		Where("category_id IS NOT NULL").
		Group("category_id").
		Order("count DESC").
		Limit(1).
		Scan(&categoryCounts)
	if len(categoryCounts) > 0 && categoryCounts[0].CategoryID != nil {
		stats.MostActiveCategory = *categoryCounts[0].CategoryID
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

func GetUserPreferences(c *gin.Context) {
	preferenceType := c.Query("type")

	var preferences []models.UserPreference
	query := database.DB.Model(&models.UserPreference{})

	switch preferenceType {
	case "feed":
		query = query.Joins("JOIN feeds ON feeds.id = user_preferences.feed_id").
			Where("user_preferences.feed_id IS NOT NULL")
	case "category":
		query = query.Joins("JOIN categories ON categories.id = user_preferences.category_id").
			Where("user_preferences.category_id IS NOT NULL")
	default:
		query = query.Where(`
			(user_preferences.feed_id IS NOT NULL AND EXISTS (
				SELECT 1 FROM feeds WHERE feeds.id = user_preferences.feed_id
			)) OR
			(user_preferences.category_id IS NOT NULL AND EXISTS (
				SELECT 1 FROM categories WHERE categories.id = user_preferences.category_id
			))
		`)
	}

	if err := query.
		Preload("Feed").
		Preload("Category").
		Order("preference_score DESC").
		Find(&preferences).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	data := make([]map[string]interface{}, 0, len(preferences))
	for _, pref := range preferences {
		prefData := pref.ToDict()
		if prefData["feed_title"] == "" && prefData["category_name"] == "" {
			continue
		}
		data = append(data, prefData)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func TriggerPreferenceUpdate(c *gin.Context) {
	if runtimeinfo.PreferenceUpdateSchedulerInterface != nil {
		if scheduler, ok := runtimeinfo.PreferenceUpdateSchedulerInterface.(interface{ TriggerNow() map[string]interface{} }); ok {
			result := scheduler.TriggerNow()
			message, _ := result["message"].(string)
			accepted, _ := result["accepted"].(bool)
			if accepted {
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": message,
					"data":    result,
				})
				return
			}

			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   message,
				"data":    result,
			})
			return
		}
	}

	go func() {
		service := NewPreferenceService(database.DB)
		if err := service.UpdateAllPreferences(); err != nil {
			logging.Warnf("UpdateAllPreferences failed: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Preference update triggered",
	})
}
