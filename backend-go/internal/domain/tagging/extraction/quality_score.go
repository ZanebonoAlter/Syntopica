package extraction

import (
	"fmt"
	"sort"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

const defaultSemanticQualityScore = 0.7

type tagMetricRow struct {
	TagID            uint    `gorm:"column:tag_id"`
	ArticleCount     int     `gorm:"column:article_count"`
	FeedDiversity    int     `gorm:"column:feed_diversity"`
	AvgCooccurrence  float64 `gorm:"column:avg_cooccurrence"`
	SemanticMatchAvg float64 `gorm:"column:semantic_match_avg"`
}

func percentileRank(values map[uint]float64, tagID uint) float64 {
	if len(values) == 0 || len(values) < 3 {
		return 0.5
	}

	target, ok := values[tagID]
	if !ok {
		return 0.5
	}

	sorted := make([]float64, 0, len(values))
	for _, value := range values {
		sorted = append(sorted, value)
	}
	sort.Float64s(sorted)

	rank := 0
	for _, value := range sorted {
		if value <= target {
			rank++
		}
	}

	return float64(rank) / float64(len(sorted))
}

func computeQualityScore(freqPct, coocPct, feedDivPct, semanticPct float64) float64 {
	return 0.4*freqPct + 0.2*coocPct + 0.2*feedDivPct + 0.2*semanticPct
}

func ComputeAllQualityScores() error {
	if database.DB == nil {
		return fmt.Errorf("database not initialized")
	}

	var rows []tagMetricRow
	err := database.DB.Raw(`
		SELECT
			t.id AS tag_id,
			COUNT(DISTINCT att.article_id) AS article_count,
			COUNT(DISTINCT a.feed_id) AS feed_diversity,
			COALESCE(AVG(cooc.cooc_count), 0) AS avg_cooccurrence,
			0 AS semantic_match_avg
		FROM topic_tags t
		LEFT JOIN article_topic_tags att ON att.topic_tag_id = t.id
		LEFT JOIN articles a ON a.id = att.article_id
		LEFT JOIN (
			SELECT article_id, COUNT(DISTINCT topic_tag_id) - 1 AS cooc_count
			FROM article_topic_tags
			GROUP BY article_id
		) cooc ON cooc.article_id = att.article_id
		WHERE t.status = 'active'
		GROUP BY t.id
	`).Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("query tag metrics: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	var activeTags []models.TopicTag
	if err := database.DB.Where("status = ?", "active").Find(&activeTags).Error; err != nil {
		return fmt.Errorf("load active tags: %w", err)
	}

	parentIDs := make(map[uint]bool)
	childIDs := make(map[uint]bool)
	var relations []models.TopicTagRelation
	if err := database.DB.Where("relation_type = ?", "abstract").Find(&relations).Error; err != nil {
		return fmt.Errorf("load abstract relations: %w", err)
	}
	for _, relation := range relations {
		parentIDs[relation.ParentID] = true
		childIDs[relation.ChildID] = true
	}

	freqMap := make(map[uint]float64)
	coocMap := make(map[uint]float64)
	feedMap := make(map[uint]float64)
	semanticMap := make(map[uint]float64)
	articleCountMap := make(map[uint]int)

	for _, row := range rows {
		articleCountMap[row.TagID] = row.ArticleCount
		if parentIDs[row.TagID] {
			continue
		}
		freqMap[row.TagID] = float64(row.ArticleCount)
		coocMap[row.TagID] = row.AvgCooccurrence
		feedMap[row.TagID] = float64(row.FeedDiversity)
		if row.SemanticMatchAvg > 0 {
			semanticMap[row.TagID] = row.SemanticMatchAvg
		} else {
			semanticMap[row.TagID] = defaultSemanticQualityScore
		}
	}

	for _, tag := range activeTags {
		if parentIDs[tag.ID] {
			continue
		}
		if tag.Kind == "abstract" || tag.Source == "abstract" {
			continue
		}

		score := 0.0
		if articleCountMap[tag.ID] > 0 {
			score = computeQualityScore(
				percentileRank(freqMap, tag.ID),
				percentileRank(coocMap, tag.ID),
				percentileRank(feedMap, tag.ID),
				semanticMap[tag.ID],
			)
		}

		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Update("quality_score", score).Error; err != nil {
			return fmt.Errorf("update quality_score for tag %d: %w", tag.ID, err)
		}
	}

	for _, tag := range activeTags {
		if !parentIDs[tag.ID] {
			continue
		}

		totalWeight := 0.0
		weightedScore := 0.0
		for _, relation := range relations {
			if relation.ParentID != tag.ID {
				continue
			}

			var child models.TopicTag
			if err := database.DB.First(&child, relation.ChildID).Error; err != nil {
				return fmt.Errorf("load child tag %d: %w", relation.ChildID, err)
			}
			weight := float64(articleCountMap[relation.ChildID])
			if weight <= 0 && childIDs[relation.ChildID] {
				weight = 1
			}
			weightedScore += child.QualityScore * weight
			totalWeight += weight
		}

		score := 0.0
		if totalWeight > 0 {
			score = weightedScore / totalWeight
		}

		if err := database.DB.Model(&models.TopicTag{}).Where("id = ?", tag.ID).Update("quality_score", score).Error; err != nil {
			return fmt.Errorf("update abstract quality_score for tag %d: %w", tag.ID, err)
		}
	}

	return nil
}
