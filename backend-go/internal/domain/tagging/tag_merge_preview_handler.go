package tagging

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

// mergeGroupSuggestion is a single suggestion within a grouped merge preview.
type mergeGroupSuggestion struct {
	ID          uint    `json:"id"`
	NewTagID    uint    `json:"new_tag_id"`
	NewLabel    string  `json:"new_label"`
	NewSlug     string  `json:"new_slug"`
	Similarity  float64 `json:"similarity"`
	NewArticles int     `json:"new_articles"`
	LLMVerdict  string  `json:"llm_verdict"`
	Source      string  `json:"source"`
}

// mergeGroup is a target tag with its associated merge suggestions.
type mergeGroup struct {
	TargetTagID    uint                   `json:"target_tag_id"`
	TargetLabel    string                 `json:"target_label"`
	TargetSlug     string                 `json:"target_slug"`
	TargetArticles int                    `json:"target_articles"`
	Category       string                 `json:"category"`
	Suggestions    []mergeGroupSuggestion `json:"suggestions"`
}

// ScanMergePreviewHandler returns candidate tag pairs grouped by target tag.
// GET /api/topic-tags/merge-preview?limit=50
func ScanMergePreviewHandler(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Query pending suggestions from the table
	var suggestions []models.TagMergeSuggestion
	if err := database.DB.
		Where("status = ?", "pending").
		Order("similarity DESC").
		Limit(limit).
		Find(&suggestions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// Filter out suggestions where LLM verdict explicitly says should_merge=false
	filtered := make([]models.TagMergeSuggestion, 0, len(suggestions))
	for _, sug := range suggestions {
		if sug.LLMVerdict != "" {
			// Parse verdict: if it contains "should_merge":false, skip it
			if strings.Contains(sug.LLMVerdict, `"should_merge":false`) ||
				strings.Contains(sug.LLMVerdict, `"should_merge": false`) {
				continue
			}
		}
		filtered = append(filtered, sug)
	}

	// Group by existing_tag_id (target tag)
	type groupKey struct {
		TargetTagID uint
		Category    string
	}
	groupMap := make(map[groupKey]*mergeGroup)
	groupOrder := make([]groupKey, 0)

	for _, sug := range filtered {
		key := groupKey{TargetTagID: sug.ExistingTagID, Category: sug.Category}

		if _, exists := groupMap[key]; !exists {
			groupMap[key] = &mergeGroup{
				TargetTagID: sug.ExistingTagID,
				TargetLabel: sug.ExistingLabel,
				Category:    sug.Category,
				Suggestions: []mergeGroupSuggestion{},
			}
			groupOrder = append(groupOrder, key)
		}

		// Fetch new tag slug
		var newTag models.TopicTag
		newSlug := ""
		if err := database.DB.Select("slug").First(&newTag, sug.NewTagID).Error; err == nil {
			newSlug = newTag.Slug
		}

		// Count articles for new tag
		var newCount int64
		database.DB.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", sug.NewTagID).Count(&newCount)

		groupMap[key].Suggestions = append(groupMap[key].Suggestions, mergeGroupSuggestion{
			ID:          sug.ID,
			NewTagID:    sug.NewTagID,
			NewLabel:    sug.NewLabel,
			NewSlug:     newSlug,
			Similarity:  sug.Similarity,
			NewArticles: int(newCount),
			LLMVerdict:  sug.LLMVerdict,
			Source:      sug.Source,
		})
	}

	// Build ordered groups with target tag enrichment
	groups := make([]mergeGroup, 0, len(groupOrder))
	hasEvaluated := false
	for _, key := range groupOrder {
		g := groupMap[key]

		// Fetch target tag slug and article count
		var targetTag models.TopicTag
		if err := database.DB.Select("slug").First(&targetTag, g.TargetTagID).Error; err == nil {
			g.TargetSlug = targetTag.Slug
		}
		var targetCount int64
		database.DB.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", g.TargetTagID).Count(&targetCount)
		g.TargetArticles = int(targetCount)

		// Check if any suggestion has been evaluated
		for _, s := range g.Suggestions {
			if s.LLMVerdict != "" {
				hasEvaluated = true
			}
		}

		groups = append(groups, *g)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"groups":       groups,
			"total_groups": len(groups),
			"evaluated":    hasEvaluated,
		},
	})
}

// MergeTagsWithCustomNameHandler merges two tags and optionally renames the target.
// POST /api/topic-tags/merge-with-name
func MergeTagsWithCustomNameHandler(c *gin.Context) {
	var body struct {
		SourceTagID uint   `json:"source_tag_id"`
		TargetTagID uint   `json:"target_tag_id"`
		NewName     string `json:"new_name"`
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

	newName := strings.TrimSpace(body.NewName)
	if newName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new_name is required"})
		return
	}

	var resultTarget models.TopicTag

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Lock both tags for the duration of the check-rename-merge sequence
		var source, target models.TopicTag
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&source, body.SourceTagID).Error; err != nil {
			return fmt.Errorf("source tag not found: %w", err)
		}
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&target, body.TargetTagID).Error; err != nil {
			return fmt.Errorf("target tag not found: %w", err)
		}

		if newName != target.Label {
			newSlug := Slugify(newName)

			// Check for slug collision with other active tags
			var conflictCount int64
			if err := tx.Model(&models.TopicTag{}).
				Where("slug = ? AND id != ? AND (status = 'active' OR status = '' OR status IS NULL)", newSlug, target.ID).
				Count(&conflictCount).Error; err != nil {
				return fmt.Errorf("check slug collision: %w", err)
			}
			if conflictCount > 0 {
				return fmt.Errorf("CONFLICT:a tag with this name already exists")
			}

			if err := tx.Model(&target).Updates(map[string]interface{}{
				"label": newName,
				"slug":  newSlug,
			}).Error; err != nil {
				return fmt.Errorf("failed to rename target tag: %w", err)
			}
			target.Label = newName
			target.Slug = newSlug
		}

		// Perform merge directly on the outer transaction to avoid deadlock.
		// MergeTags/HardMergeTags uses database.DB which opens a separate transaction,
		// causing a self-deadlock against the FOR UPDATE locks held by this transaction.
		if err := HardMergeTags(tx, body.SourceTagID, body.TargetTagID); err != nil {
			return fmt.Errorf("merge failed: %w", err)
		}

		// NOTE: Enqueue must NOT be called inside this transaction.
		// It uses database.DB (a separate connection), and the FK constraints on
		// merge_reembedding_queues (source_tag_id, target_tag_id → topic_tags)
		// cause PostgreSQL to wait for this transaction to commit before the INSERT
		// can verify the FK — resulting in an application-level deadlock.
		// Enqueue is now called after the transaction commits (see below).

		resultTarget = target
		return nil
	})

	// Enqueue after transaction commits so the FK constraints can resolve.
	if err == nil {
		_ = mergeReembeddingQueueFactory().Enqueue(body.SourceTagID, body.TargetTagID)
	}

	if err != nil {
		errMsg := err.Error()
		status := http.StatusInternalServerError
		if strings.Contains(errMsg, "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(errMsg, "already merged") || strings.Contains(errMsg, "CONFLICT:") {
			status = http.StatusBadRequest
		}
		// Strip CONFLICT: prefix for clean error message
		cleanMsg := strings.TrimPrefix(errMsg, "CONFLICT:")
		c.JSON(status, gin.H{"success": false, "error": cleanMsg})
		return
	}

	// Mark related suggestions as merged
	database.DB.Model(&models.TagMergeSuggestion{}).
		Where("status = ? AND (new_tag_id = ? OR existing_tag_id = ? OR new_tag_id = ? OR existing_tag_id = ?)",
			"pending", body.SourceTagID, body.SourceTagID, body.TargetTagID, body.TargetTagID).
		Update("status", "merged")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"source_id": body.SourceTagID,
			"target_id": body.TargetTagID,
			"new_label": resultTarget.Label,
			"merged_at": time.Now().Format(time.RFC3339),
		},
	})
}

// DismissSuggestionHandler marks a suggestion as dismissed.
// POST /api/topic-tags/merge-preview/dismiss
func DismissSuggestionHandler(c *gin.Context) {
	var body struct {
		NewTagID      uint `json:"new_tag_id"`
		ExistingTagID uint `json:"existing_tag_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if body.NewTagID == 0 || body.ExistingTagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "new_tag_id and existing_tag_id are required"})
		return
	}

	result := database.DB.Model(&models.TagMergeSuggestion{}).
		Where("new_tag_id = ? AND existing_tag_id = ? AND status = ?",
			body.NewTagID, body.ExistingTagID, "pending").
		Update("status", "dismissed")

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "suggestion not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TriggerScanHandler starts an asynchronous full scan.
// POST /api/topic-tags/merge-preview/scan
func TriggerScanHandler(c *gin.Context) {
	if !StartFullScan() {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "scan already in progress",
		})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "scan started",
	})
}

// ScanStreamHandler streams scan progress via SSE.
// GET /api/topic-tags/merge-preview/scan/stream
func ScanStreamHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch := GetScanProgressChannel()
	if ch == nil {
		// No scan running — send idle status and close
		c.SSEvent("progress", ScanProgress{Status: "idle"})
		return
	}

	c.Stream(func(w io.Writer) bool {
		msg, ok := <-ch
		if !ok {
			return false
		}
		c.SSEvent("progress", msg)
		return true
	})
}

// TriggerEvaluateHandler starts an asynchronous LLM evaluation of merge candidates.
// POST /api/topic-tags/merge-preview/evaluate
func TriggerEvaluateHandler(c *gin.Context) {
	if !StartEvaluation() {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "evaluation already in progress",
		})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "evaluation started",
	})
}

// EvaluateStreamHandler streams evaluation progress via SSE.
// GET /api/topic-tags/merge-preview/evaluate/stream
func EvaluateStreamHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch := GetEvaluateProgressChannel()
	if ch == nil {
		c.SSEvent("progress", EvaluateProgress{Status: "idle"})
		return
	}

	c.Stream(func(w io.Writer) bool {
		msg, ok := <-ch
		if !ok {
			return false
		}
		c.SSEvent("progress", msg)
		return true
	})
}

// AddToGroupHandler manually adds a tag to an existing merge group.
// POST /api/topic-tags/merge-preview/add-to-group
func AddToGroupHandler(c *gin.Context) {
	var body struct {
		TargetTagID uint `json:"target_tag_id"`
		NewTagID    uint `json:"new_tag_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}
	if body.TargetTagID == 0 || body.NewTagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "target_tag_id and new_tag_id are required"})
		return
	}
	if body.TargetTagID == body.NewTagID {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "cannot add tag to itself"})
		return
	}

	// Verify both tags exist
	var target, newTag models.TopicTag
	if err := database.DB.Select("id, label, slug, category").First(&target, body.TargetTagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "target tag not found"})
		return
	}
	if err := database.DB.Select("id, label, slug, category").First(&newTag, body.NewTagID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "new tag not found"})
		return
	}

	suggestion := models.TagMergeSuggestion{
		NewTagID:      body.NewTagID,
		ExistingTagID: body.TargetTagID,
		NewLabel:      newTag.Label,
		ExistingLabel: target.Label,
		Category:      target.Category,
		Similarity:    0,
		Status:        "pending",
		Source:        "manual",
	}

	result := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "new_tag_id"}, {Name: "existing_tag_id"}},
		DoNothing: true,
	}).Create(&suggestion)

	if result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "suggestion already exists"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RegisterTagMergePreviewRoutes registers the preview and custom-name merge endpoints.
func RegisterTagMergePreviewRoutes(rg *gin.RouterGroup) {
	tags := rg.Group("/topic-tags")
	{
		tags.GET("/merge-preview", ScanMergePreviewHandler)
		tags.POST("/merge-with-name", MergeTagsWithCustomNameHandler)
		tags.POST("/merge-preview/dismiss", DismissSuggestionHandler)
		tags.POST("/merge-preview/scan", TriggerScanHandler)
		tags.GET("/merge-preview/scan/stream", ScanStreamHandler)
		tags.POST("/merge-preview/evaluate", TriggerEvaluateHandler)
		tags.GET("/merge-preview/evaluate/stream", EvaluateStreamHandler)
		tags.POST("/merge-preview/add-to-group", AddToGroupHandler)
	}
}
