package topicanalysis

import (
	"encoding/json"
	"net/http"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"github.com/gin-gonic/gin"
)

func GetHierarchyConfig(c *gin.Context) {
	mgr := GetHierarchyManager()

	templates := mgr.AllTemplates()
	if templates == nil {
		templates = []*CategoryHierarchyTemplate{}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"templates": templates,
			"version":   mgr.GetVersion(),
		},
	})
}

func UpdateHierarchyConfig(c *gin.Context) {
	var req struct {
		Templates []CategoryHierarchyTemplate `json:"templates" binding:"required"`
		ChangeLog string                      `json:"change_log"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	mgr := GetHierarchyManager()
	existing := mgr.AllTemplates()
	existingKeys := make(map[string]bool)
	for _, t := range existing {
		existingKeys[t.TemplateKey()] = true
	}
	for _, t := range req.Templates {
		if !existingKeys[t.TemplateKey()] {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "不能新增模板: " + t.TemplateKey(),
			})
			return
		}
	}
	if len(req.Templates) != len(existing) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "不能删除模板",
		})
		return
	}

	impact, err := previewConfigImpact(&req.Templates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to preview impact: " + err.Error()})
		return
	}

	if err := generatePendingChanges(impact); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to generate pending changes: " + err.Error()})
		return
	}

	templatesJSON := make(map[string]json.RawMessage)
	for _, t := range req.Templates {
		data, err := json.Marshal(t)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to marshal template"})
			return
		}
		templatesJSON[t.TemplateKey()] = data
	}

	changeLog := req.ChangeLog
	if changeLog == "" {
		changeLog = "Configuration update"
	}
	if err := mgr.SaveConfig(templatesJSON, changeLog); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to save config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version": mgr.GetVersion(),
			"impact":  impact,
		},
	})
}

func GetHierarchyPending(c *gin.Context) {
	statusFilter := c.DefaultQuery("status", "")

	var changes []models.HierarchyPendingChange
	query := database.DB.Preload("Tag").Preload("Parent").Order("created_at DESC")
	if statusFilter != "" {
		query = query.Where("status = ?", statusFilter)
	}
	if err := query.Find(&changes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load pending changes"})
		return
	}

	if changes == nil {
		changes = []models.HierarchyPendingChange{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": changes})
}

func TriggerHierarchyRebuild(c *gin.Context) {
	var req struct {
		Category string `json:"category"`
		DryRun   bool   `json:"dry_run"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	var changes []models.HierarchyPendingChange
	query := database.DB.Where("status = 'pending'").Preload("Tag")
	if req.Category != "" {
		query = query.Where("tag_label IN (SELECT label FROM topic_tags WHERE category = ?)", req.Category)
	}
	if err := query.Find(&changes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load pending changes"})
		return
	}

	if req.DryRun {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"dry_run":            true,
				"pending_count":      len(changes),
				"pending_changes":    changes,
			},
		})
		return
	}

	processed := 0
	for _, change := range changes {
		if change.Tag == nil {
			continue
		}
		_, err := PlaceTagInHierarchy(c.Request.Context(), change.Tag)
		if err != nil {
			logging.Warnf("Rebuild: failed to place tag %d: %v", change.TagID, err)
			continue
		}
		database.DB.Model(&change).Updates(map[string]interface{}{
			"status":      "resolved",
			"resolved_at": database.DB.NowFunc(),
		})
		processed++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_pending":     len(changes),
			"processed":         processed,
		},
	})
}

func RegisterHierarchyRoutes(rg *gin.RouterGroup) {
	rg.GET("/config", GetHierarchyConfig)
	rg.PUT("/config", UpdateHierarchyConfig)
	rg.GET("/pending", GetHierarchyPending)
	rg.POST("/rebuild", TriggerHierarchyRebuild)
}
