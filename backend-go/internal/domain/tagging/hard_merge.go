package tagging

import (
	"fmt"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/logging"

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

		if err := tx.Where("topic_tag_id = ?", sourceID).Delete(&models.TopicTagEmbedding{}).Error; err != nil {
			return fmt.Errorf("delete source tag embeddings: %w", err)
		}

		if err := tx.Delete(&models.TopicTag{}, sourceID).Error; err != nil {
			return fmt.Errorf("delete source tag %d: %w", sourceID, err)
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
