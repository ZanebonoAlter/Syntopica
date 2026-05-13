package tagging

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/platform/logging"
)

// GetTagHierarchyHandler returns the tag hierarchy tree.
// GET /api/topic-tags/hierarchy?category=&feed_id=&category_id=&unclassified=&time_range=
func GetTagHierarchyHandler(c *gin.Context) {
	category := strings.TrimSpace(c.Query("category"))
	unclassified := c.Query("unclassified") == "true"
	timeRange := strings.TrimSpace(c.Query("time_range"))

	var scopeFeedID uint
	var scopeCategoryID uint
	if fid := c.Query("feed_id"); fid != "" {
		if v, e := strconv.ParseUint(fid, 10, 32); e == nil {
			scopeFeedID = uint(v)
		}
	}
	if cid := c.Query("category_id"); cid != "" {
		if v, e := strconv.ParseUint(cid, 10, 32); e == nil {
			scopeCategoryID = uint(v)
		}
	}

	var nodes []TagHierarchyNode
	var err error
	if unclassified {
		nodes, err = GetUnclassifiedTags(category, scopeFeedID, scopeCategoryID, timeRange)
	} else {
		nodes, err = GetTagHierarchy(category, scopeFeedID, scopeCategoryID, timeRange)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"nodes": nodes,
			"total": len(nodes),
		},
	})
}

// UpdateAbstractTagNameHandler renames an abstract tag.
// PUT /api/topic-tags/:tag_id/abstract-name
func UpdateAbstractTagNameHandler(c *gin.Context) {
	tagIDStr := c.Param("tag_id")
	tagID, err := strconv.ParseUint(tagIDStr, 10, 32)
	if err != nil || tagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid tag id"})
		return
	}

	var body struct {
		NewName string `json:"new_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	newName := strings.TrimSpace(body.NewName)
	if newName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new_name is required"})
		return
	}
	if len(newName) > 160 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new_name exceeds 160 characters"})
		return
	}

	if err := UpdateAbstractTagName(uint(tagID), newName); err != nil {
		status := http.StatusInternalServerError
		errMsg := err.Error()
		if strings.Contains(errMsg, "not an abstract tag") || strings.Contains(errMsg, "must be") {
			status = http.StatusBadRequest
		} else if strings.Contains(errMsg, "already in use") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"success": false, "error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":       tagID,
			"new_name": newName,
		},
	})
}

// DetachChildTagHandler removes a child tag from its abstract parent.
// POST /api/topic-tags/:tag_id/detach
func DetachChildTagHandler(c *gin.Context) {
	parentIDStr := c.Param("tag_id")
	parentID, err := strconv.ParseUint(parentIDStr, 10, 32)
	if err != nil || parentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid parent tag id"})
		return
	}

	var body struct {
		ChildID uint `json:"child_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if body.ChildID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "child_id is required"})
		return
	}

	if err := DetachChildTag(uint(parentID), body.ChildID); err != nil {
		status := http.StatusInternalServerError
		errMsg := err.Error()
		if strings.Contains(errMsg, "must be") || strings.Contains(errMsg, "no relation found") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"success": false, "error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "child detached",
	})
}

// ReassignTagHandler moves a tag to a new abstract parent.
// POST /api/topic-tags/:tag_id/reassign
func ReassignTagHandler(c *gin.Context) {
	tagIDStr := c.Param("tag_id")
	tagID, err := strconv.ParseUint(tagIDStr, 10, 32)
	if err != nil || tagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid tag id"})
		return
	}

	var body struct {
		ParentID uint `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if body.ParentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "parent_id is required"})
		return
	}

	if err := ReassignTagParent(uint(tagID), body.ParentID); err != nil {
		status := http.StatusInternalServerError
		errMsg := err.Error()
		if strings.Contains(errMsg, "must be") || strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "cannot reassign") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"success": false, "error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "tag reassigned",
	})
}

// RegisterAbstractTagRoutes registers the abstract tag management endpoints.
func RegisterAbstractTagRoutes(rg *gin.RouterGroup) {
	tags := rg.Group("/topic-tags")
	{
		tags.GET("/hierarchy", GetTagHierarchyHandler)
		tags.PUT("/:tag_id/abstract-name", UpdateAbstractTagNameHandler)
		tags.POST("/:tag_id/detach", DetachChildTagHandler)
		tags.POST("/:tag_id/reassign", ReassignTagHandler)
		tags.POST("/organize", OrganizeUnclassifiedTagsHandler)
	}
}

// OrganizeUnclassifiedTagsHandler triggers abstract tag extraction for unclassified tags.
// POST /api/topic-tags/organize?category=
// Runs asynchronously; progress is broadcast via WebSocket (type "organize_progress").
func OrganizeUnclassifiedTagsHandler(c *gin.Context) {
	category := strings.TrimSpace(c.Query("category"))

	if !organizeMu.TryLock() {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "整理任务正在运行中，请稍后再试"})
		return
	}

	broadcastOrganizeProgress("processing", 0, 0, nil, 0, category)

	go func() {
		defer organizeMu.Unlock()
		_, err := OrganizeUnclassifiedTags(context.Background(), category, 0)
		if err != nil {
			logging.Errorf("OrganizeUnclassifiedTags async error: %v", err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "整理任务已启动",
	})
}

var organizeMu sync.Mutex
