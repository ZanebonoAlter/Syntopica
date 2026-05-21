package tagging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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
	var req hierarchyConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	if err := validateHierarchyConfigTemplates(req.Templates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	impact, err := previewConfigImpact(&req.Templates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to preview impact: " + err.Error()})
		return
	}

	previewOnly := !req.Apply && req.Mode != "apply"
	if previewOnly {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"version":      GetHierarchyManager().GetVersion(),
				"impact":       impact,
				"preview_only": true,
			},
		})
		return
	}

	result, err := applyHierarchyConfig(req.Templates, req.ChangeLog, impact)
	if err != nil {
		if _, ok := err.(*hierarchyConfigConflictError); ok {
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func PreviewHierarchyConfig(c *gin.Context) {
	var req hierarchyConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if err := validateHierarchyConfigTemplates(req.Templates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	impact, err := previewConfigImpact(&req.Templates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to preview impact: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version":      GetHierarchyManager().GetVersion(),
			"impact":       impact,
			"preview_only": true,
		},
	})
}

type hierarchyConfigRequest struct {
	Templates []CategoryHierarchyTemplate `json:"templates" binding:"required"`
	ChangeLog string                      `json:"change_log"`
	Mode      string                      `json:"mode"`
	Apply     bool                        `json:"apply"`
}

type hierarchyConfigApplyResult struct {
	Version           int                 `json:"version"`
	Impact            *ConfigImpact       `json:"impact"`
	PreviewOnly       bool                `json:"preview_only"`
	ChangedCategories []string            `json:"changed_categories"`
	RebuildJobs       []models.RebuildJob `json:"rebuild_jobs"`
}

type hierarchyConfigConflictError struct {
	message string
}

func (e *hierarchyConfigConflictError) Error() string {
	return e.message
}

func validateHierarchyConfigTemplates(templates []CategoryHierarchyTemplate) error {
	mgr := GetHierarchyManager()
	existing := mgr.AllTemplates()
	existingKeys := make(map[string]bool)
	for _, t := range existing {
		existingKeys[t.TemplateKey()] = true
	}
	for _, t := range templates {
		if !existingKeys[t.TemplateKey()] {
			return &hierarchyConfigConflictError{message: "不能新增模板: " + t.TemplateKey()}
		}
	}
	if len(templates) != len(existing) {
		return &hierarchyConfigConflictError{message: "不能删除模板"}
	}
	return nil
}

func applyHierarchyConfig(templates []CategoryHierarchyTemplate, changeLog string, impact *ConfigImpact) (*hierarchyConfigApplyResult, error) {
	mgr := GetHierarchyManager()
	changedCategories, err := changedTemplateCategories(mgr.AllTemplates(), templates)
	if err != nil {
		return nil, err
	}
	for _, category := range changedCategories {
		active, err := hasActiveRebuildJob(category)
		if err != nil {
			return nil, err
		}
		if active {
			return nil, &hierarchyConfigConflictError{message: "active rebuild job already exists for category " + category}
		}
	}

	if len(changedCategories) == 0 {
		return &hierarchyConfigApplyResult{
			Version:           mgr.GetVersion(),
			Impact:            impact,
			PreviewOnly:       false,
			ChangedCategories: []string{},
			RebuildJobs:       []models.RebuildJob{},
		}, nil
	}

	templatesJSON := make(map[string]json.RawMessage)
	for _, t := range templates {
		data, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		templatesJSON[t.TemplateKey()] = data
	}

	if changeLog == "" {
		changeLog = "Configuration update"
	}
	if err := mgr.SaveConfig(templatesJSON, changeLog); err != nil {
		return nil, err
	}

	changed := make(map[string]bool)
	for _, category := range changedCategories {
		changed[category] = true
	}
	jobs := make([]models.RebuildJob, 0, len(changedCategories))
	svc := GetRebuildService()
	for _, t := range templates {
		if !changed[t.Category] {
			continue
		}
		snapshot := make(models.MetadataMap)
		data, err := json.Marshal(t)
		if err != nil {
			logging.Warnf("rebuild-trigger: marshal template for category %q: %v", t.Category, err)
			continue
		}
		if err := json.Unmarshal(data, &snapshot); err != nil {
			logging.Warnf("rebuild-trigger: unmarshal snapshot for category %q: %v", t.Category, err)
			continue
		}
		if err := svc.TriggerTemplateRebuild(t.Category, snapshot); err != nil {
			return nil, err
		}
		var job models.RebuildJob
		if err := database.DB.Where("category = ? AND status = ?", t.Category, models.RebuildJobStatusPending).
			Order("created_at DESC").First(&job).Error; err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
		go func(jobID uint) {
			if err := svc.ExecuteJob(jobID); err != nil {
				logging.Warnf("rebuild-trigger: execute job %d: %v", jobID, err)
			}
		}(job.ID)
	}

	return &hierarchyConfigApplyResult{
		Version:           mgr.GetVersion(),
		Impact:            impact,
		PreviewOnly:       false,
		ChangedCategories: changedCategories,
		RebuildJobs:       jobs,
	}, nil
}

func changedTemplateCategories(existing []*CategoryHierarchyTemplate, incoming []CategoryHierarchyTemplate) ([]string, error) {
	existingByKey := make(map[string]*CategoryHierarchyTemplate)
	for _, t := range existing {
		existingByKey[t.TemplateKey()] = t
	}
	seen := make(map[string]bool)
	var categories []string
	for _, t := range incoming {
		old, ok := existingByKey[t.TemplateKey()]
		if !ok {
			return nil, &hierarchyConfigConflictError{message: "不能新增模板: " + t.TemplateKey()}
		}
		oldJSON, err := json.Marshal(old)
		if err != nil {
			return nil, err
		}
		newJSON, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(oldJSON, newJSON) || seen[t.Category] {
			continue
		}
		seen[t.Category] = true
		categories = append(categories, t.Category)
	}
	return categories, nil
}

func hasActiveRebuildJob(category string) (bool, error) {
	var count int64
	if err := database.DB.Model(&models.RebuildJob{}).
		Where("category = ? AND status IN ?", category, []string{
			models.RebuildJobStatusPending,
			models.RebuildJobStatusRunning,
			models.RebuildJobStatusPaused,
		}).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
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

func GetHierarchyClosureStatus(c *gin.Context) {
	category := c.DefaultQuery("category", "event")
	status, err := GetHierarchyOrchestrationService().InspectCategoryClosureStatus(c.Request.Context(), category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
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
				"dry_run":         true,
				"pending_count":   len(changes),
				"pending_changes": changes,
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
			"total_pending": len(changes),
			"processed":     processed,
		},
	})
}

func RegisterHierarchyRoutes(rg *gin.RouterGroup) {
	rg.GET("/config", GetHierarchyConfig)
	rg.POST("/config/preview", PreviewHierarchyConfig)
	rg.PUT("/config", UpdateHierarchyConfig)
	rg.GET("/pending", GetHierarchyPending)
	rg.GET("/closure-status", GetHierarchyClosureStatus)
	rg.POST("/rebuild", TriggerHierarchyRebuild)
	rg.POST("/rebuild/start", startRebuildHandler)
	rg.GET("/rebuild/:id", getRebuildStatusHandler)
	rg.POST("/pending/approve", approvePendingChangesHandler)
}

type startRebuildRequest struct {
	Category string `json:"category"`
	Trigger  string `json:"trigger"`
}

func startRebuildHandler(c *gin.Context) {
	var req startRebuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if req.Category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "category is required"})
		return
	}
	if req.Trigger == "" {
		req.Trigger = models.RebuildJobTriggerManual
	}

	job, err := GetRebuildService().CreateJob(req.Category, req.Trigger, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	go func() {
		if execErr := GetRebuildService().ExecuteJob(job.ID); execErr != nil {
			logging.Warnf("rebuild-start: execute job %d: %v", job.ID, execErr)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"success": true, "data": job})
}

func getRebuildStatusHandler(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	job, err := GetRebuildJobByID(database.DB, uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "rebuild job not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": job})
}

type approvePendingChangesRequest struct {
	IDs        []uint `json:"ids"`
	ApproveAll bool   `json:"approve_all"`
	Category   string `json:"category"`
}

type pendingChangeApprovalResult struct {
	ID         uint   `json:"id"`
	TagID      uint   `json:"tag_id"`
	ChangeType string `json:"change_type"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

func approvePendingChangesHandler(c *gin.Context) {
	var req approvePendingChangesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	var changes []models.HierarchyPendingChange
	query := database.DB.Where("status = ?", "pending").Preload("Tag")

	if req.ApproveAll {
		if req.Category != "" {
			query = query.Where("tag_label IN (SELECT label FROM topic_tags WHERE category = ?)", req.Category)
		}
	} else {
		if len(req.IDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "ids or approve_all required"})
			return
		}
		query = query.Where("id IN ?", req.IDs)
	}

	if err := query.Find(&changes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load pending changes"})
		return
	}

	approved := 0
	failed := 0
	results := make([]pendingChangeApprovalResult, 0, len(changes))

	for _, change := range changes {
		result := pendingChangeApprovalResult{ID: change.ID, TagID: change.TagID, ChangeType: change.ChangeType}
		if err := executePendingChange(change); err != nil {
			logging.Warnf("approve-pending: failed change id=%d type=%s tag=%d: %v", change.ID, change.ChangeType, change.TagID, err)
			failureReason := err.Error()
			database.DB.Model(&change).Updates(map[string]interface{}{
				"status":      "failed",
				"reason":      failureReason,
				"resolved_at": database.DB.NowFunc(),
			})
			result.Status = "failed"
			result.Reason = failureReason
			results = append(results, result)
			failed++
			continue
		}

		database.DB.Model(&change).Updates(map[string]interface{}{
			"status":      "resolved",
			"resolved_at": database.DB.NowFunc(),
		})
		result.Status = "resolved"
		results = append(results, result)
		approved++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"approved": approved,
			"failed":   failed,
			"results":  results,
		},
	})
}

func executePendingChange(change models.HierarchyPendingChange) error {
	if change.TagID == 0 {
		return fmt.Errorf("tag_id is required")
	}

	switch change.ChangeType {
	case "template_violation", "depth_exceeded", "cross_category", "level_mismatch", "new_leaf_violation", "new_leaf_violations":
		if change.CurrentParentID == nil || *change.CurrentParentID == 0 {
			return fmt.Errorf("current_parent_id is required for %s", change.ChangeType)
		}
		result := database.DB.Where(
			"parent_id = ? AND child_id = ? AND relation_type = ?",
			*change.CurrentParentID,
			change.TagID,
			"abstract",
		).Delete(&models.TopicTagRelation{})
		if result.Error != nil {
			return fmt.Errorf("delete relation: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("relation not found for parent_id=%d child_id=%d", *change.CurrentParentID, change.TagID)
		}
		return nil
	case "move", "reparent":
		return fmt.Errorf("target parent payload is required for %s", change.ChangeType)
	case "create":
		return fmt.Errorf("create payload is required")
	case "delete":
		return fmt.Errorf("delete payload is required")
	default:
		return fmt.Errorf("unsupported change_type %q", change.ChangeType)
	}
}
