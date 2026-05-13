package watched

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"my-robot-backend/internal/platform/database"
)

// ListWatchedTagsHandler returns all watched tags with abstract-tag metadata.
func ListWatchedTagsHandler(c *gin.Context) {
	tags, err := ListWatchedTags(database.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": tags})
}

// WatchTagHandler marks a tag as watched.
func WatchTagHandler(c *gin.Context) {
	tagID, err := strconv.ParseUint(c.Param("tag_id"), 10, 32)
	if err != nil || tagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid tag_id"})
		return
	}

	tag, err := WatchTag(database.DB, uint(tagID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "tag not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":         tag.ID,
			"is_watched": tag.IsWatched,
			"watched_at": tag.WatchedAt,
		},
	})
}

// UnwatchTagHandler removes the watched status from a tag.
func UnwatchTagHandler(c *gin.Context) {
	tagID, err := strconv.ParseUint(c.Param("tag_id"), 10, 32)
	if err != nil || tagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid tag_id"})
		return
	}

	tag, err := UnwatchTag(database.DB, uint(tagID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "tag not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":         tag.ID,
			"is_watched": tag.IsWatched,
		},
	})
}

// RegisterWatchedTagsRoutes registers the watched tag routes under the /topic-tags group.
func RegisterWatchedTagsRoutes(rg *gin.RouterGroup) {
	tags := rg.Group("/topic-tags")
	{
		tags.GET("/watched", ListWatchedTagsHandler)
		tags.POST("/:tag_id/watch", WatchTagHandler)
		tags.POST("/:tag_id/unwatch", UnwatchTagHandler)
	}
}
