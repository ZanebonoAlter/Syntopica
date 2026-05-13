package tagging

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

func getRootAbstractTagID(tagID uint) uint {
	if database.DB == nil || tagID == 0 {
		return 0
	}

	current := tagID
	visited := map[uint]bool{}
	for current != 0 && !visited[current] {
		visited[current] = true

		var relation models.TopicTagRelation
		err := database.DB.Where("child_id = ? AND relation_type = ?", current, "abstract").Order("id ASC").First(&relation).Error
		if err != nil {
			break
		}
		current = relation.ParentID
	}

	return current
}

func getAllTreeTagIDs(tagID uint) []uint {
	if database.DB == nil || tagID == 0 {
		return nil
	}

	rootID := getRootAbstractTagID(tagID)
	if rootID == 0 {
		rootID = tagID
	}

	visited := make(map[uint]bool)
	queue := []uint{rootID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == 0 || visited[current] {
			continue
		}
		visited[current] = true

		var childIDs []uint
		if err := database.DB.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND relation_type = ?", current, "abstract").
			Order("id ASC").
			Pluck("child_id", &childIDs).Error; err != nil {
			logging.Warnf("getAllTreeTagIDs: load children for %d failed: %v", current, err)
			continue
		}
		queue = append(queue, childIDs...)
	}

	ids := make([]uint, 0, len(visited))
	for id := range visited {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func getTagDepthFromRootDB(db *gorm.DB, tagID uint) int {
	if db == nil || tagID == 0 {
		return 0
	}

	type node struct {
		id    uint
		depth int
	}
	maxDepth := 0
	visited := map[uint]bool{tagID: true}
	queue := []node{{id: tagID, depth: 0}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		var relations []models.TopicTagRelation
		if err := db.Where("child_id = ? AND relation_type = ?", current.id, "abstract").Find(&relations).Error; err != nil {
			continue
		}
		for _, r := range relations {
			if visited[r.ParentID] {
				continue
			}
			visited[r.ParentID] = true
			d := current.depth + 1
			if d > maxDepth {
				maxDepth = d
			}
			queue = append(queue, node{id: r.ParentID, depth: d})
		}
	}
	return maxDepth
}

func getTagDepthFromRoot(tagID uint) int {
	return getTagDepthFromRootDB(database.DB, tagID)
}

// GetTagDepthFromRoot is the exported version for external packages.
func GetTagDepthFromRoot(tagID uint) int {
	return getTagDepthFromRoot(tagID)
}

func loadTagPathLabels(tagID uint, maxDepth int) []string {
	if database.DB == nil || tagID == 0 {
		return nil
	}

	labels := make([]string, 0, maxDepth)
	current := tagID
	visited := map[uint]bool{}
	for current != 0 && !visited[current] && (maxDepth <= 0 || len(labels) < maxDepth) {
		visited[current] = true

		var tag models.TopicTag
		if err := database.DB.Select("id", "label").First(&tag, current).Error; err != nil {
			break
		}
		labels = append(labels, tag.Label)

		var relation models.TopicTagRelation
		err := database.DB.Where("child_id = ? AND relation_type = ?", current, "abstract").Order("id ASC").First(&relation).Error
		if err != nil {
			break
		}
		current = relation.ParentID
	}

	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	return labels
}

func loadTagPathString(tagID uint, maxDepth int) string {
	labels := loadTagPathLabels(tagID, maxDepth)
	if len(labels) == 0 {
		return "(无路径)"
	}
	return strings.Join(labels, " > ")
}

type supportedEventTag struct {
	TagID       uint
	Support     int
	DirectScore float64
}

func findRelatedEventTagsFromAnchors(anchorIDs []uint, excludeIDs map[uint]struct{}, directHitScores map[uint]float64) ([]supportedEventTag, error) {
	if database.DB == nil || len(anchorIDs) == 0 {
		return nil, nil
	}

	keywordSet := make(map[uint]struct{})
	for _, anchorID := range anchorIDs {
		var articleIDs []uint
		if err := database.DB.Model(&models.ArticleTopicTag{}).
			Where("topic_tag_id = ?", anchorID).
			Order("article_id DESC").
			Limit(coTagMaxArticlesPerTag).
			Pluck("article_id", &articleIDs).Error; err != nil {
			return nil, fmt.Errorf("load articles for anchor %d: %w", anchorID, err)
		}

		for _, articleID := range articleIDs {
			keywords := getTopArticleKeywords(database.DB, articleID, coTagBaseTopN)
			if len(keywords) == 0 {
				continue
			}

			tagIDs := make([]uint, 0, len(keywords))
			for _, keyword := range keywords {
				tagIDs = append(tagIDs, keyword.TagID)
			}

			var keywordTags []models.TopicTag
			if err := database.DB.Where("id IN ? AND category = ? AND (status = 'active' OR status = '' OR status IS NULL)", tagIDs, "keyword").Find(&keywordTags).Error; err != nil {
				return nil, fmt.Errorf("load keyword tags for article %d: %w", articleID, err)
			}
			for _, keywordTag := range keywordTags {
				keywordSet[keywordTag.ID] = struct{}{}
			}
		}
	}

	if len(keywordSet) == 0 {
		return nil, nil
	}

	keywordIDs := make([]uint, 0, len(keywordSet))
	for id := range keywordSet {
		keywordIDs = append(keywordIDs, id)
	}
	sort.Slice(keywordIDs, func(i, j int) bool { return keywordIDs[i] < keywordIDs[j] })

	excludeMap := make(map[uint]bool, len(excludeIDs))
	for id := range excludeIDs {
		excludeMap[id] = true
	}

	eventSupport := findEventTagsViaKeywords(database.DB, keywordIDs, excludeMap, coTagMaxCandidates)
	if len(eventSupport) == 0 {
		return nil, nil
	}

	result := make([]supportedEventTag, 0, len(eventSupport))
	for tagID, support := range eventSupport {
		result = append(result, supportedEventTag{
			TagID:       tagID,
			Support:     support,
			DirectScore: directHitScores[tagID],
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Support != result[j].Support {
			return result[i].Support > result[j].Support
		}
		if result[i].DirectScore != result[j].DirectScore {
			return result[i].DirectScore > result[j].DirectScore
		}
		return result[i].TagID < result[j].TagID
	})

	return result, nil
}

func buildTreeDedupCandidate(_ context.Context, eventID uint, supportCount int, directScore float64) (TagCandidate, error) {
	var tag models.TopicTag
	if err := database.DB.First(&tag, eventID).Error; err != nil {
		return TagCandidate{}, fmt.Errorf("load event tag %d: %w", eventID, err)
	}

	score := 0.72
	if supportCount > 0 {
		score += float64(min(supportCount, 5)) * 0.04
	}
	if directScore > 0 {
		score += directScore * 0.08
	}
	if score > 0.99 {
		score = 0.99
	}

	return TagCandidate{Tag: &tag, Similarity: score}, nil
}

func findCrossLayerDuplicateCandidates(ctx context.Context, abstractTagID uint, category string) ([]TagCandidate, error) {
	if database.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	es := NewEmbeddingService()
	thresholds := es.GetThresholds()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		return nil, err
	}

	treeTagIDs := getAllTreeTagIDs(abstractTagID)
	if len(treeTagIDs) == 0 {
		return nil, nil
	}
	treeSet := make(map[uint]struct{}, len(treeTagIDs))
	for _, id := range treeTagIDs {
		treeSet[id] = struct{}{}
	}

	keywordAnchors, err := es.FindSimilarTags(ctx, &abstractTag, category, 20, EmbeddingTypeSemantic)
	if err != nil {
		return nil, err
	}

	anchorIDs := make([]uint, 0, len(keywordAnchors))
	directHitScores := make(map[uint]float64)
	for _, candidate := range keywordAnchors {
		if candidate.Tag == nil || candidate.Tag.ID == abstractTagID {
			continue
		}
		if candidate.Similarity < thresholds.HighSimilarity {
			continue
		}
		anchorIDs = append(anchorIDs, candidate.Tag.ID)
		directHitScores[candidate.Tag.ID] = candidate.Similarity
	}
	if len(anchorIDs) == 0 {
		return nil, nil
	}

	excludeIDs := map[uint]struct{}{abstractTagID: {}}
	for _, anchorID := range anchorIDs {
		if _, inTree := treeSet[anchorID]; inTree {
			excludeIDs[anchorID] = struct{}{}
		}
	}

	relatedEventTags, err := findRelatedEventTagsFromAnchors(anchorIDs, excludeIDs, directHitScores)
	if err != nil {
		return nil, err
	}

	result := make([]TagCandidate, 0, 3)
	for _, supportedTag := range relatedEventTags {
		if _, inTree := treeSet[supportedTag.TagID]; !inTree {
			continue
		}
		candidate, buildErr := buildTreeDedupCandidate(ctx, supportedTag.TagID, supportedTag.Support, supportedTag.DirectScore)
		if buildErr != nil {
			logging.Warnf("findCrossLayerDuplicateCandidates: build candidate for %d failed: %v", supportedTag.TagID, buildErr)
			continue
		}
		result = append(result, candidate)
		if len(result) == 3 {
			break
		}
	}

	return result, nil
}

func wouldCreateCycle(tx *gorm.DB, parentID, childID uint) (bool, error) {
	visited := make(map[uint]bool)
	queue := []uint{childID}
	visited[childID] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == parentID {
			return true, nil
		}

		var relations []models.TopicTagRelation
		if err := tx.Where("child_id = ? AND relation_type = ?", current, "abstract").Find(&relations).Error; err != nil {
			return false, fmt.Errorf("query relations for cycle check: %w", err)
		}

		for _, r := range relations {
			if !visited[r.ParentID] {
				visited[r.ParentID] = true
				queue = append(queue, r.ParentID)
			}
		}
	}

	return false, nil
}

func loadAbstractChildLabels(tagID uint, limit int) []string {
	var labels []string
	database.DB.Model(&models.TopicTag{}).
		Joins("JOIN topic_tag_relations ON topic_tag_relations.child_id = topic_tags.id").
		Where("topic_tag_relations.parent_id = ? AND topic_tag_relations.relation_type = ?", tagID, "abstract").
		Order("topic_tag_relations.similarity_score DESC").
		Limit(limit).
		Pluck("topic_tags.label", &labels)
	if labels == nil {
		labels = []string{}
	}
	return labels
}

func formatChildLabels(labels []string) string {
	if len(labels) == 0 {
		return "(无子标签)"
	}
	return strings.Join(labels, ", ")
}
