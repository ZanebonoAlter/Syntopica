package tagging

import (
	"context"
	"fmt"
	"sort"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm"
)

const (
	coTagBaseTopN          = 5
	coTagDepthBonus        = 2
	coTagMaxTopN           = 15
	coTagMaxArticlesPerTag = 10
	coTagMaxCandidates     = 20
)

func calculateCoTagTopN(subtreeDepth int) int {
	if subtreeDepth <= 1 {
		return coTagBaseTopN
	}

	n := coTagBaseTopN + (subtreeDepth-1)*coTagDepthBonus
	if n > coTagMaxTopN {
		return coTagMaxTopN
	}

	return n
}

type coTagCandidate struct {
	KeywordTagID uint
	KeywordLabel string
	Coverage     int
	ArticleCount int
}

type articleKeyword struct {
	TagID uint
	Score float64
}

// ExpandEventCandidatesByArticleCoTags expands event-tag candidates with article co-tag signals.
// Pass articleID for raw article-based expansion, or abstractTagID for abstract-tag subtree expansion.
func ExpandEventCandidatesByArticleCoTags(ctx context.Context, articleID uint, abstractTagID uint, existingCandidateIDs []uint) ([]TagCandidate, error) {
	if database.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	tx := database.DB.WithContext(ctx)

	var keywordTagIDs []uint
	topN := coTagBaseTopN

	switch {
	case articleID > 0:
		keywords := getTopArticleKeywords(tx, articleID, coTagBaseTopN)
		for _, kw := range keywords {
			keywordTagIDs = append(keywordTagIDs, kw.TagID)
		}
	case abstractTagID > 0:
		subtreeDepth := getAbstractSubtreeDepth(tx, abstractTagID)
		topN = calculateCoTagTopN(subtreeDepth)
		keywords := aggregateKeywordsByChildCoverage(tx, abstractTagID, topN)
		for _, kw := range keywords {
			keywordTagIDs = append(keywordTagIDs, kw.KeywordTagID)
		}
	default:
		return nil, nil
	}

	if len(keywordTagIDs) == 0 {
		return nil, nil
	}

	existingSet := make(map[uint]bool, len(existingCandidateIDs))
	for _, id := range existingCandidateIDs {
		existingSet[id] = true
	}

	eventTagArticleMap := findEventTagsViaKeywords(tx, keywordTagIDs, existingSet, coTagMaxCandidates)
	if len(eventTagArticleMap) == 0 {
		return nil, nil
	}

	eventTagIDs := make([]uint, 0, len(eventTagArticleMap))
	for id := range eventTagArticleMap {
		eventTagIDs = append(eventTagIDs, id)
	}

	var eventTags []models.TopicTag
	if err := tx.Where("id IN ? AND category = ? AND status = ?", eventTagIDs, "event", "active").Find(&eventTags).Error; err != nil {
		return nil, fmt.Errorf("load event tags: %w", err)
	}

	logging.Infof("co-tag expansion: found %d additional event candidates (topN=%d, source=article:%d/abstract:%d)",
		len(eventTags), topN, articleID, abstractTagID)

	result := make([]TagCandidate, 0, len(eventTags))
	for _, tag := range eventTags {
		tagCopy := tag
		result = append(result, TagCandidate{
			Tag:        &tagCopy,
			Similarity: 0.80,
		})
	}

	return result, nil
}

func getTopArticleKeywords(tx *gorm.DB, articleID uint, topN int) []articleKeyword {
	var links []models.ArticleTopicTag
	tx.Where("article_id = ?", articleID).
		Order("score DESC").
		Limit(topN).
		Find(&links)

	result := make([]articleKeyword, 0, len(links))
	for _, link := range links {
		result = append(result, articleKeyword{TagID: link.TopicTagID, Score: link.Score})
	}

	return result
}

func aggregateKeywordsByChildCoverage(tx *gorm.DB, abstractTagID uint, topN int) []coTagCandidate {
	childEventTagIDs := getDescendantEventTagIDs(tx, abstractTagID)
	if len(childEventTagIDs) == 0 {
		return nil
	}

	type coverageRow struct {
		KeywordTagID uint `gorm:"column:keyword_tag_id"`
		Coverage     int  `gorm:"column:coverage"`
		ArticleCount int  `gorm:"column:article_count"`
	}

	var rows []coverageRow
	query := `
		SELECT att2.topic_tag_id AS keyword_tag_id,
		       COUNT(DISTINCT att1.topic_tag_id) AS coverage,
		       COUNT(DISTINCT att2.article_id) AS article_count
		FROM article_topic_tags att1
		JOIN article_topic_tags att2 ON att1.article_id = att2.article_id
		                            AND att2.topic_tag_id != att1.topic_tag_id
		JOIN topic_tags tt ON tt.id = att2.topic_tag_id
		WHERE att1.topic_tag_id IN ?
		  AND tt.category = 'keyword'
		  AND (tt.status = 'active' OR tt.status = '' OR tt.status IS NULL)
		GROUP BY att2.topic_tag_id
		ORDER BY coverage DESC, article_count DESC
		LIMIT ?
	`
	if err := tx.Raw(query, childEventTagIDs, topN).Scan(&rows).Error; err != nil {
		logging.Warnf("aggregateKeywordsByChildCoverage: query failed: %v", err)
		return nil
	}

	tagIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		tagIDs = append(tagIDs, row.KeywordTagID)
	}

	labelMap := make(map[uint]string, len(tagIDs))
	if len(tagIDs) > 0 {
		var tags []models.TopicTag
		tx.Where("id IN ?", tagIDs).Find(&tags)
		for _, tag := range tags {
			labelMap[tag.ID] = tag.Label
		}
	}

	result := make([]coTagCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, coTagCandidate{
			KeywordTagID: row.KeywordTagID,
			KeywordLabel: labelMap[row.KeywordTagID],
			Coverage:     row.Coverage,
			ArticleCount: row.ArticleCount,
		})
	}

	return result
}

func getDescendantEventTagIDs(tx *gorm.DB, abstractTagID uint) []uint {
	query := `
		WITH RECURSIVE descendants AS (
			SELECT child_id FROM topic_tag_relations WHERE parent_id = ? AND relation_type = 'abstract'
			UNION
			SELECT r.child_id FROM topic_tag_relations r
			JOIN descendants d ON r.parent_id = d.child_id
			WHERE r.relation_type = 'abstract'
		)
		SELECT d.child_id FROM descendants d
		JOIN topic_tags t ON t.id = d.child_id
		WHERE t.category = 'event' AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
	`

	var ids []uint
	if err := tx.Raw(query, abstractTagID).Scan(&ids).Error; err != nil {
		logging.Warnf("getDescendantEventTagIDs: query failed for %d: %v", abstractTagID, err)
		return nil
	}

	return ids
}

func getAbstractSubtreeDepth(tx *gorm.DB, tagID uint) int {
	if tx == nil || tagID == 0 {
		return 0
	}

	maxDepth := 0
	visited := map[uint]bool{tagID: true}
	type queueItem struct {
		id    uint
		depth int
	}
	queue := []queueItem{{id: tagID, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		var childIDs []uint
		if err := tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND relation_type = ?", current.id, "abstract").
			Pluck("child_id", &childIDs).Error; err != nil {
			logging.Warnf("getAbstractSubtreeDepth: query failed for %d: %v", current.id, err)
			continue
		}

		for _, childID := range childIDs {
			if visited[childID] {
				continue
			}
			visited[childID] = true
			childDepth := current.depth + 1
			if childDepth > maxDepth {
				maxDepth = childDepth
			}
			queue = append(queue, queueItem{id: childID, depth: childDepth})
		}
	}
	return maxDepth
}

func findEventTagsViaKeywords(tx *gorm.DB, keywordTagIDs []uint, excludeIDs map[uint]bool, maxCandidates int) map[uint]int {
	type eventRow struct {
		EventTagID uint `gorm:"column:event_tag_id"`
		HitCount   int  `gorm:"column:hit_count"`
	}

	var rows []eventRow
	query := `
		SELECT att2.topic_tag_id AS event_tag_id, COUNT(DISTINCT att2.article_id) AS hit_count
		FROM article_topic_tags att1
		JOIN article_topic_tags att2 ON att1.article_id = att2.article_id
		JOIN topic_tags tt ON tt.id = att2.topic_tag_id
		WHERE att1.topic_tag_id IN ?
		  AND tt.category = 'event'
		  AND (tt.status = 'active' OR tt.status = '' OR tt.status IS NULL)
		GROUP BY att2.topic_tag_id
		ORDER BY hit_count DESC
		LIMIT ?
	`
	if err := tx.Raw(query, keywordTagIDs, maxCandidates).Scan(&rows).Error; err != nil {
		logging.Warnf("findEventTagsViaKeywords: query failed: %v", err)
		return nil
	}

	result := make(map[uint]int, len(rows))
	for _, row := range rows {
		if excludeIDs[row.EventTagID] {
			continue
		}
		result[row.EventTagID] = row.HitCount
	}

	return result
}

func MergeCandidateLists(embeddingCandidates, coTagCandidates []TagCandidate) []TagCandidate {
	seen := make(map[uint]bool, len(embeddingCandidates)+len(coTagCandidates))
	result := make([]TagCandidate, 0, len(embeddingCandidates)+len(coTagCandidates))

	for _, candidate := range embeddingCandidates {
		if candidate.Tag == nil || seen[candidate.Tag.ID] {
			continue
		}
		seen[candidate.Tag.ID] = true
		result = append(result, candidate)
	}

	for _, candidate := range coTagCandidates {
		if candidate.Tag == nil || seen[candidate.Tag.ID] {
			continue
		}
		seen[candidate.Tag.ID] = true
		result = append(result, candidate)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Similarity > result[j].Similarity
	})

	return result
}
