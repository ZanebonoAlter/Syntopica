package topicextraction

import (
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"strings"
)

// BackfillMissingDescriptions finds active tags without descriptions and generates
// descriptions via LLM using article context from associated articles.
// Intended to be called from a scheduled task (e.g. TagHierarchyCleanupScheduler).
// Returns the number of tags processed (not necessarily succeeded).
func BackfillMissingDescriptions() (int, error) {
	var tags []models.TopicTag
	if err := database.DB.
		Where("status = ? AND (description IS NULL OR description = '')", "active").
		Limit(50).
		Find(&tags).Error; err != nil {
		return 0, err
	}

	if len(tags) == 0 {
		return 0, nil
	}

	logging.Infof("description backfill: found %d tags without description", len(tags))

	batchSize := 10
	processed := 0
	for i := 0; i < len(tags); i += batchSize {
		end := i + batchSize
		if end > len(tags) {
			end = len(tags)
		}
		batch := tags[i:end]

		results := batchGenerateTagDescriptions(batch)
		for _, tag := range batch {
			if desc, ok := results[tag.ID]; ok {
				if desc == "" {
					// Single-tag path already saved to DB + enqueued embedding
					processed++
					continue
				}
				if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).
					Update("description", desc).Error; err != nil {
					logging.Warnf("description backfill: failed to update tag %d: %v", tag.ID, err)
				} else {
					processed++
					// Re-embed after description update, matching single-tag path (tagger.go:690)
					if qs := getEmbeddingQueueService(); qs != nil {
						if err := qs.Enqueue(tag.ID); err != nil {
							logging.Warnf("description backfill: failed to enqueue re-embedding for tag %d: %v", tag.ID, err)
						}
					}
				}
			}
		}
	}

	logging.Infof("description backfill: updated %d/%d tags", processed, len(tags))
	return processed, nil
}

// buildArticleContextForTag queries the most recent articles associated with a tag
// and builds a context string (title + summary) for description generation.
func buildArticleContextForTag(tagID uint) string {
	type articleRow struct {
		Title       string
		Description string
	}

	var rows []articleRow
	err := database.DB.Model(&models.ArticleTopicTag{}).
		Select("articles.title, articles.description").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id = ?", tagID).
		Order("articles.pub_date DESC").
		Limit(3).
		Scan(&rows).Error
	if err != nil {
		logging.Warnf("description backfill: failed to query articles for tag %d: %v", tagID, err)
		return ""
	}

	if len(rows) == 0 {
		return ""
	}

	var parts []string
	for _, row := range rows {
		if row.Title != "" {
			parts = append(parts, row.Title)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	context := strings.Join(parts, "; ")
	runes := []rune(context)
	if len(runes) > 800 {
		context = string(runes[:800])
	}
	return context
}
