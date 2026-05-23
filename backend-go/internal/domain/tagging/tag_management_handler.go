package tagging

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func SearchTagsHandler(c *gin.Context) {
	q := c.Query("q")
	category := c.Query("category")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	if q == "" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []models.TopicTag{}})
		return
	}

	var tags []models.TopicTag
	query := database.DB.Where("(status = 'active' OR status = '' OR status IS NULL)").
		Where("label ILIKE ?", "%"+q+"%").
		Order("feed_count DESC, id DESC").
		Limit(limit)

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tags})
}

func MergeTagsHandler(c *gin.Context) {
	var body struct {
		SourceTagID uint `json:"source_tag_id"`
		TargetTagID uint `json:"target_tag_id"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	if body.SourceTagID == 0 || body.TargetTagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "source_tag_id and target_tag_id are required"})
		return
	}

	if body.SourceTagID == body.TargetTagID {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cannot merge tag into itself"})
		return
	}

	var source, target models.TopicTag
	if err := database.DB.First(&source, body.SourceTagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "source tag not found"})
		return
	}
	if err := database.DB.First(&target, body.TargetTagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "target tag not found"})
		return
	}

	if err := MergeTags(body.SourceTagID, body.TargetTagID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "tags merged",
		"data": gin.H{
			"source_id":    body.SourceTagID,
			"target_id":    body.TargetTagID,
			"target_label": target.Label,
		},
	})
}

func RegisterTagManagementRoutes(rg *gin.RouterGroup) {
	tags := rg.Group("/topic-tags")
	{
		tags.GET("/search", SearchTagsHandler)
		tags.POST("/merge", MergeTagsHandler)
	}
}
