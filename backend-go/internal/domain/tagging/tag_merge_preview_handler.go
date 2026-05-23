package tagging

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

// mergePreviewResponse wraps a single candidate with optional article lists.
type mergePreviewCandidate struct {
	TagMergeCandidate
	SourceArticleList []CandidateArticle `json:"source_article_list,omitempty"`
	TargetArticleList []CandidateArticle `json:"target_article_list,omitempty"`
}

// ScanMergePreviewHandler returns candidate tag pairs for manual review.
// GET /api/topic-tags/merge-preview?limit=50&include_articles=false&feed_id=&category_id=
func ScanMergePreviewHandler(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	includeArticles := c.DefaultQuery("include_articles", "false") == "true"

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

	candidates, err := ScanSimilarTagPairs(limit, scopeFeedID, scopeCategoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	result := make([]mergePreviewCandidate, 0, len(candidates))
	for _, cand := range candidates {
		item := mergePreviewCandidate{TagMergeCandidate: cand}
		if includeArticles {
			if arts, err := GetCandidateArticleTitles(cand.SourceTagID, 5); err == nil {
				item.SourceArticleList = arts
			}
			if arts, err := GetCandidateArticleTitles(cand.TargetTagID, 5); err == nil {
				item.TargetArticleList = arts
			}
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"candidates": result,
			"total":      len(result),
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

		// Perform merge within the same transaction scope.
		// MergeTags opens its own database.DB.Transaction which GORM promotes
		// to a savepoint when already inside a transaction.
		if err := MergeTags(body.SourceTagID, body.TargetTagID); err != nil {
			return fmt.Errorf("merge failed: %w", err)
		}

		resultTarget = target
		return nil
	})

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

// RegisterTagMergePreviewRoutes registers the preview and custom-name merge endpoints.
func RegisterTagMergePreviewRoutes(rg *gin.RouterGroup) {
	tags := rg.Group("/topic-tags")
	{
		tags.GET("/merge-preview", ScanMergePreviewHandler)
		tags.POST("/merge-with-name", MergeTagsWithCustomNameHandler)
	}
}
