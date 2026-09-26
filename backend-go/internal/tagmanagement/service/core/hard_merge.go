package core

import (
	"context"
	"fmt"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"

	"gorm.io/gorm"
)

func HardMergeTags(db *gorm.DB, sourceID, targetID uint) error {
	if sourceID == targetID {
		return fmt.Errorf("cannot merge tag into itself (id=%d)", sourceID)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var source, target models.TopicTag
		if err := tx.First(&source, sourceID).Error; err != nil {
			return fmt.Errorf("source tag %d not found: %w", sourceID, err)
		}
		if err := tx.First(&target, targetID).Error; err != nil {
			return fmt.Errorf("target tag %d not found: %w", targetID, err)
		}

		var sourceLinks []models.ArticleTopicTag
		if err := tx.Where("topic_tag_id = ?", sourceID).Find(&sourceLinks).Error; err != nil {
			return fmt.Errorf("find source article_topic_tags: %w", err)
		}
		for _, link := range sourceLinks {
			var existingCount int64
			if err := tx.Model(&models.ArticleTopicTag{}).
				Where("article_id = ? AND topic_tag_id = ?", link.ArticleID, targetID).
				Count(&existingCount).Error; err != nil {
				return fmt.Errorf("check existing article_topic_tag for article %d: %w", link.ArticleID, err)
			}
			if existingCount > 0 {
				if err := tx.Delete(&link).Error; err != nil {
					return fmt.Errorf("delete duplicate article_topic_tag %d: %w", link.ID, err)
				}
			} else {
				if err := tx.Model(&link).Update("topic_tag_id", targetID).Error; err != nil {
					return fmt.Errorf("update article_topic_tag %d to target: %w", link.ID, err)
				}
			}
		}

		if err := tx.Where("topic_tag_id = ?", sourceID).Delete(&tagmodels.TopicTagEmbedding{}).Error; err != nil {
			return fmt.Errorf("delete source tag embeddings: %w", err)
		}

		// Collect aux label IDs before CASCADE deletes topic_tag_semantic_labels
		var affectedAuxLabelIDs []uint
		if err := tx.Model(&tagmodels.TopicTagSemanticLabel{}).
			Where("topic_tag_id = ?", sourceID).
			Distinct("semantic_label_id").
			Pluck("semantic_label_id", &affectedAuxLabelIDs).Error; err != nil {
			return fmt.Errorf("collect affected aux label ids: %w", err)
		}

		// Clean up queue records that reference the source tag before deletion
		// to avoid foreign key constraint violations.
		if err := tx.Where("tag_id = ?", sourceID).Delete(&models.EmbeddingQueue{}).Error; err != nil {
			return fmt.Errorf("delete source tag embedding queue entries: %w", err)
		}
		if err := tx.Where("source_tag_id = ? OR target_tag_id = ?", sourceID, sourceID).Delete(&tagmodels.MergeReembeddingQueue{}).Error; err != nil {
			return fmt.Errorf("delete source tag merge re-embedding queue entries: %w", err)
		}

		if err := tx.Delete(&models.TopicTag{}, sourceID).Error; err != nil {
			return fmt.Errorf("delete source tag %d: %w", sourceID, err)
		}

		// Recount refs for affected auxiliary labels after CASCADE
		if len(affectedAuxLabelIDs) > 0 {
			auxService := AuxServiceFactory(tx, nil)
			if err := auxService.RecountRefs(context.Background(), affectedAuxLabelIDs); err != nil {
				return fmt.Errorf("recount refs after hard merge: %w", err)
			}
		}

		if err := tx.Model(&models.TopicTag{}).
			Where("id = ?", targetID).
			Update("feed_count", tx.Model(&models.ArticleTopicTag{}).
				Select("COUNT(DISTINCT a.feed_id)").
				Joins("JOIN articles a ON a.id = article_topic_tags.article_id").
				Where("article_topic_tags.topic_tag_id = ?", targetID),
			).Error; err != nil {
			return fmt.Errorf("recalculate target feed_count: %w", err)
		}

		logging.Infof("HardMergeTags: hard-deleted tag %d into %d", sourceID, targetID)
		return nil
	})
}
