package tagging

import (
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

// TagMergeCandidate represents a pair of similar tags proposed for merging.
type TagMergeCandidate struct {
	SourceTagID    uint    `json:"source_tag_id"`
	SourceLabel    string  `json:"source_label"`
	SourceSlug     string  `json:"source_slug"`
	TargetTagID    uint    `json:"target_tag_id"`
	TargetLabel    string  `json:"target_label"`
	TargetSlug     string  `json:"target_slug"`
	Category       string  `json:"category"`
	Similarity     float64 `json:"similarity"`
	SourceArticles int     `json:"source_articles"`
	TargetArticles int     `json:"target_articles"`
}

// CandidateArticle holds minimal article info for preview display.
type CandidateArticle struct {
	ArticleID uint   `json:"article_id"`
	Title     string `json:"title"`
	Link      string `json:"link"`
}

// ScanSimilarTagPairs finds tag pairs with high embedding similarity without merging.
// It reuses the same pgvector cross-join logic from tag_hierarchy_cleanup.go (Phase 2: flat merge)
// but returns candidates sorted by similarity descending instead of auto-executing merges.
func ScanSimilarTagPairs(limit int, scopeFeedID uint, scopeCategoryID uint) ([]TagMergeCandidate, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	distanceThreshold := 1.0 - DefaultThresholds.HighSimilarity

	type similarPair struct {
		SourceID    uint    `gorm:"column:source_id"`
		SourceLabel string  `gorm:"column:source_label"`
		TargetID    uint    `gorm:"column:target_id"`
		TargetLabel string  `gorm:"column:target_label"`
		Category    string  `gorm:"column:category"`
		Distance    float64 `gorm:"column:distance"`
	}

	args := []interface{}{distanceThreshold}
	scopeJoin := ""

	if scopeFeedID > 0 {
		scopeJoin = `
			JOIN article_topic_tags at1 ON at1.topic_tag_id = e1.topic_tag_id
			JOIN articles a1 ON a1.id = at1.article_id
			JOIN article_topic_tags at2 ON at2.topic_tag_id = e2.topic_tag_id
			JOIN articles a2 ON a2.id = at2.article_id`
		args = append(args, scopeFeedID, scopeFeedID)
	} else if scopeCategoryID > 0 {
		scopeJoin = `
			JOIN article_topic_tags at1 ON at1.topic_tag_id = e1.topic_tag_id
			JOIN articles a1 ON a1.id = at1.article_id
			JOIN feeds f1 ON f1.id = a1.feed_id
			JOIN feed_categories fc1 ON fc1.id = f1.category_id
			JOIN article_topic_tags at2 ON at2.topic_tag_id = e2.topic_tag_id
			JOIN articles a2 ON a2.id = at2.article_id
			JOIN feeds f2 ON f2.id = a2.feed_id
			JOIN feed_categories fc2 ON fc2.id = f2.category_id`
		args = append(args, scopeCategoryID, scopeCategoryID)
	}

	args = append(args, limit)

	var pairs []similarPair
	query := `
		SELECT
			t1.id AS source_id, t1.label AS source_label,
			t2.id AS target_id, t2.label AS target_label,
			t1.category,
			e1.embedding <=> e2.embedding AS distance
		FROM topic_tag_embeddings e1
		JOIN topic_tags t1 ON t1.id = e1.topic_tag_id
		JOIN topic_tag_embeddings e2 ON e2.topic_tag_id > e1.topic_tag_id
		JOIN topic_tags t2 ON t2.id = e2.topic_tag_id
		` + scopeJoin + `
		WHERE (t1.status = 'active' OR t1.status = '' OR t1.status IS NULL)
		  AND (t2.status = 'active' OR t2.status = '' OR t2.status IS NULL)
		  AND t1.category = t2.category
		  AND e1.embedding <=> e2.embedding < ?`

	if scopeFeedID > 0 {
		query += `
		  AND a1.feed_id = ?
		  AND a2.feed_id = ?`
	} else if scopeCategoryID > 0 {
		query += `
		  AND fc1.id = ?
		  AND fc2.id = ?`
	}

	query += `
		ORDER BY e1.embedding <=> e2.embedding ASC
		LIMIT ?`

	if err := database.DB.Raw(query, args...).Scan(&pairs).Error; err != nil {
		return nil, fmt.Errorf("query similar tag pairs: %w", err)
	}

	candidates := make([]TagMergeCandidate, 0, len(pairs))
	skipped := 0
	for _, pair := range pairs {
		var tag1, tag2 models.TopicTag
		if err := database.DB.First(&tag1, pair.SourceID).Error; err != nil {
			skipped++
			continue
		}
		if err := database.DB.First(&tag2, pair.TargetID).Error; err != nil {
			skipped++
			continue
		}

		var count1, count2 int64
		database.DB.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", tag1.ID).Count(&count1)
		database.DB.Model(&models.ArticleTopicTag{}).Where("topic_tag_id = ?", tag2.ID).Count(&count2)

		var sourceID, targetID uint
		var sourceLabel, sourceSlug, targetLabel, targetSlug string
		var sourceArticles, targetArticles int

		switch {
		case count2 > count1:
			sourceID = tag1.ID
			sourceLabel = tag1.Label
			sourceSlug = tag1.Slug
			targetID = tag2.ID
			targetLabel = tag2.Label
			targetSlug = tag2.Slug
			sourceArticles = int(count1)
			targetArticles = int(count2)
		case count1 > count2:
			sourceID = tag2.ID
			sourceLabel = tag2.Label
			sourceSlug = tag2.Slug
			targetID = tag1.ID
			targetLabel = tag1.Label
			targetSlug = tag1.Slug
			sourceArticles = int(count2)
			targetArticles = int(count1)
		default:
			// Equal article count: smaller ID = source (deterministic)
			if tag1.ID < tag2.ID {
				sourceID = tag1.ID
				sourceLabel = tag1.Label
				sourceSlug = tag1.Slug
				targetID = tag2.ID
				targetLabel = tag2.Label
				targetSlug = tag2.Slug
			} else {
				sourceID = tag2.ID
				sourceLabel = tag2.Label
				sourceSlug = tag2.Slug
				targetID = tag1.ID
				targetLabel = tag1.Label
				targetSlug = tag1.Slug
			}
			sourceArticles = int(count1)
			targetArticles = int(count2)
		}

		candidates = append(candidates, TagMergeCandidate{
			SourceTagID:    sourceID,
			SourceLabel:    sourceLabel,
			SourceSlug:     sourceSlug,
			TargetTagID:    targetID,
			TargetLabel:    targetLabel,
			TargetSlug:     targetSlug,
			Category:       pair.Category,
			Similarity:     1.0 - pair.Distance,
			SourceArticles: sourceArticles,
			TargetArticles: targetArticles,
		})
	}

	if skipped > 0 {
		logging.Warnf("ScanSimilarTagPairs: skipped %d pairs due to DB lookup errors", skipped)
	}

	return candidates, nil
}

// GetCandidateArticleTitles returns the most recent articles associated with a tag.
func GetCandidateArticleTitles(tagID uint, limit int) ([]CandidateArticle, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	var articles []CandidateArticle
	query := `
		SELECT a.id AS article_id, a.title, a.link
		FROM articles a
		JOIN article_topic_tags at ON a.id = at.article_id
		WHERE at.topic_tag_id = ?
		ORDER BY a.pub_date DESC NULLS LAST, a.created_at DESC
		LIMIT ?
	`
	if err := database.DB.Raw(query, tagID, limit).Scan(&articles).Error; err != nil {
		return nil, fmt.Errorf("query candidate articles for tag %d: %w", tagID, err)
	}
	return articles, nil
}
