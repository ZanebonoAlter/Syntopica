package tagging

import (
	"context"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

func CleanupZombieTagsV2(db *gorm.DB, category string) (int, error) {
	cutoff := time.Now().AddDate(0, 0, -7)
	var ids []uint
	err := db.Model(&models.TopicTag{}).
		Where("status = ? AND category = ?", "active", category).
		Where("created_at < ?", cutoff).
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_relations r WHERE (r.parent_id = topic_tags.id OR r.child_id = topic_tags.id) AND r.relation_type = ?)", "abstract").
		Where("NOT EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id)").
		Pluck("topic_tags.id", &ids).Error
	if err != nil {
		return 0, fmt.Errorf("CleanupZombieTagsV2: pluck ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := hardDeleteTags(db, ids); err != nil {
		return 0, fmt.Errorf("CleanupZombieTagsV2: delete: %w", err)
	}
	logging.Infof("CleanupZombieTagsV2(%s): hard-deleted %d zombie tags", category, len(ids))
	return len(ids), nil
}

func CleanupLowQualityTagsV2(db *gorm.DB, category string) (int, error) {
	var ids []uint
	err := db.Model(&models.TopicTag{}).
		Where("status = ? AND category = ? AND kind != ? AND source != ?", "active", category, "abstract", "abstract").
		Where("quality_score < ?", 0.15).
		Where("(SELECT COUNT(*) FROM article_topic_tags att WHERE att.topic_tag_id = topic_tags.id) = 1").
		Pluck("topic_tags.id", &ids).Error
	if err != nil {
		return 0, fmt.Errorf("CleanupLowQualityTagsV2: pluck ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := hardDeleteTags(db, ids); err != nil {
		return 0, fmt.Errorf("CleanupLowQualityTagsV2: delete: %w", err)
	}
	logging.Infof("CleanupLowQualityTagsV2(%s): hard-deleted %d low-quality tags", category, len(ids))
	return len(ids), nil
}

func CleanupEmptyNodesV2(db *gorm.DB, category string) (int, error) {
	var ids []uint
	err := db.Model(&models.TopicTag{}).
		Where("source = ? AND status = ? AND category = ?", "abstract", "active", category).
		Where("NOT EXISTS (SELECT 1 FROM topic_tag_relations r WHERE r.parent_id = topic_tags.id AND r.relation_type = ?)", "abstract").
		Pluck("topic_tags.id", &ids).Error
	if err != nil {
		return 0, fmt.Errorf("CleanupEmptyNodesV2: pluck ids: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := hardDeleteTags(db, ids); err != nil {
		return 0, fmt.Errorf("CleanupEmptyNodesV2: delete: %w", err)
	}
	logging.Infof("CleanupEmptyNodesV2(%s): hard-deleted %d empty abstract nodes", category, len(ids))
	return len(ids), nil
}

func CleanupSameLevelDuplicates(db *gorm.DB, category string) (int, error) {
	type abstractInfo struct {
		ID        uint
		ConceptID uint
	}
	var tags []abstractInfo
	err := db.Model(&models.TopicTag{}).
		Select("topic_tags.id, topic_tags.concept_id").
		Where("source = ? AND status = ? AND category = ? AND concept_id IS NOT NULL AND concept_id != 0",
			"abstract", "active", category).
		Find(&tags).Error
	if err != nil {
		return 0, fmt.Errorf("CleanupSameLevelDuplicates: load abstracts: %w", err)
	}
	if len(tags) < 2 {
		return 0, nil
	}

	type depthInfo struct {
		ID    uint
		Depth int
	}
	depths := make([]depthInfo, len(tags))
	for i, t := range tags {
		depths[i] = depthInfo{ID: t.ID, Depth: getTagDepthFromRootDB(db, t.ID)}
	}

	type groupKey struct {
		ConceptID uint
		Depth     int
	}
	groups := make(map[groupKey][]uint)
	for i, t := range tags {
		key := groupKey{ConceptID: t.ConceptID, Depth: depths[i].Depth}
		groups[key] = append(groups[key], t.ID)
	}

	merged := 0
	mergedSet := make(map[uint]bool)
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		var activeIDs []uint
		for _, id := range ids {
			if !mergedSet[id] {
				activeIDs = append(activeIDs, id)
			}
		}
		if len(activeIDs) < 2 {
			continue
		}

		edges, err := NewEmbeddingService().FindSimilarTagsAmongSet(context.Background(), activeIDs, 0.90)
		if err != nil {
			logging.Warnf("CleanupSameLevelDuplicates: similarity query failed: %v", err)
			continue
		}

		for _, edge := range edges {
			if mergedSet[edge.TagAID] || mergedSet[edge.TagBID] {
				continue
			}
			sourceID, targetID := pickMergeOrderByChildCount(db, edge.TagAID, edge.TagBID)
			if err := HardMergeTags(db, sourceID, targetID); err != nil {
				logging.Warnf("CleanupSameLevelDuplicates: merge %d -> %d failed: %v", sourceID, targetID, err)
				continue
			}
			mergedSet[sourceID] = true
			merged++
		}
	}

	logging.Infof("CleanupSameLevelDuplicates(%s): merged %d same-level duplicates", category, merged)
	return merged, nil
}

func pickMergeOrderByChildCount(db *gorm.DB, idA, idB uint) (sourceID, targetID uint) {
	var countA, countB int64
	db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND relation_type = ?", idA, "abstract").Count(&countA)
	db.Model(&models.TopicTagRelation{}).Where("parent_id = ? AND relation_type = ?", idB, "abstract").Count(&countB)
	if countA >= countB {
		return idB, idA
	}
	return idA, idB
}

func CleanupTemplateViolationsV2(db *gorm.DB, category string) (int, error) {
	var relations []models.TopicTagRelation
	if err := db.Where("relation_type = 'abstract'").
		Preload("Parent").Preload("Child").
		Find(&relations).Error; err != nil {
		return 0, fmt.Errorf("CleanupTemplateViolationsV2: load relations: %w", err)
	}

	count := 0
	for _, r := range relations {
		if r.Parent == nil || r.Child == nil {
			continue
		}
		if r.Parent.Category != category {
			continue
		}

		tmpl := GetHierarchyManager().GetTemplate(r.Parent.Category, r.Parent.SubType)
		if tmpl == nil {
			continue
		}

		violated := false
		reason := ""

		childDepth := getTagDepthFromRoot(r.ChildID)
		if childDepth+1 > tmpl.MaxLevel {
			violated = true
			reason = fmt.Sprintf("Depth %d exceeds max %d for template %s", childDepth+1, tmpl.MaxLevel, tmpl.TemplateKey())
		}

		if r.Parent.Category != r.Child.Category {
			violated = true
			if reason != "" {
				reason += "; "
			}
			reason += fmt.Sprintf("Parent category %s != child category %s", r.Parent.Category, r.Child.Category)
		}

		if violated {
			change := models.HierarchyPendingChange{
				TagID:              r.ChildID,
				TagLabel:           r.Child.Label,
				ChangeType:         "template_violation",
				CurrentParentID:    &r.ParentID,
				CurrentParentLabel: r.Parent.Label,
				Reason:             reason,
				Status:             "pending",
			}
			if err := db.Create(&change).Error; err != nil {
				logging.Warnf("CleanupTemplateViolationsV2: failed to create pending change for tag %d: %v", r.ChildID, err)
				continue
			}
			count++
		}
	}

	logging.Infof("CleanupTemplateViolationsV2(%s): created %d pending changes", category, count)
	return count, nil
}

type AnchorSignal struct {
	TagIDs      []uint
	CenterTagID uint
}

const anchorSignalTTL = 24 * time.Hour

func cleanupExpiredAnchorSignals(db *gorm.DB) error {
	return db.Where("expires_at <= ?", time.Now()).Delete(&models.HierarchyAnchorSignal{}).Error
}

func GenerateAnchorSignals(ctx context.Context, db *gorm.DB, category string) ([]AnchorSignal, error) {
	if err := cleanupExpiredAnchorSignals(db); err != nil {
		return nil, fmt.Errorf("GenerateAnchorSignals: cleanup expired anchor signals: %w", err)
	}

	var tagIDs []uint
	err := db.Model(&models.TopicTag{}).
		Where("category = ? AND status = ? AND concept_id IS NULL", category, "active").
		Limit(100).
		Pluck("id", &tagIDs).Error
	if err != nil {
		return nil, fmt.Errorf("GenerateAnchorSignals: load unplaced tags: %w", err)
	}
	if len(tagIDs) == 0 {
		if err := db.Where("category = ? AND expires_at > ?", category, time.Now()).Delete(&models.HierarchyAnchorSignal{}).Error; err != nil {
			return nil, fmt.Errorf("GenerateAnchorSignals: clear anchor signals: %w", err)
		}
		return nil, nil
	}

	es := NewEmbeddingService()
	edges, err := es.FindSimilarTagsAmongSet(ctx, tagIDs, 0.75)
	if err != nil {
		return nil, fmt.Errorf("GenerateAnchorSignals: find similar: %w", err)
	}

	adj := make(map[uint][]uint)
	for _, e := range edges {
		adj[e.TagAID] = append(adj[e.TagAID], e.TagBID)
		adj[e.TagBID] = append(adj[e.TagBID], e.TagAID)
	}

	visited := make(map[uint]bool)
	var signals []AnchorSignal
	for _, start := range tagIDs {
		if visited[start] {
			continue
		}
		if len(adj[start]) == 0 {
			visited[start] = true
			continue
		}

		component := []uint{start}
		visited[start] = true
		queue := []uint{start}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, nb := range adj[cur] {
				if !visited[nb] {
					visited[nb] = true
					component = append(component, nb)
					queue = append(queue, nb)
				}
			}
		}

		if len(component) >= 3 {
			center := component[0]
			signals = append(signals, AnchorSignal{
				TagIDs:      component,
				CenterTagID: center,
			})
		}
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("category = ? AND expires_at > ?", category, time.Now()).Delete(&models.HierarchyAnchorSignal{}).Error; err != nil {
			return fmt.Errorf("delete current anchor signals: %w", err)
		}

		now := time.Now()
		for _, signal := range signals {
			row := models.HierarchyAnchorSignal{
				Category:     category,
				CenterTagID:  signal.CenterTagID,
				MemberTagIDs: signal.TagIDs,
				ExpiresAt:    now.Add(anchorSignalTTL),
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("create anchor signal: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("GenerateAnchorSignals: persist anchor signals: %w", err)
	}

	logging.Infof("GenerateAnchorSignals(%s): %d anchor signals from %d unplaced tags", category, len(signals), len(tagIDs))
	return signals, nil
}

func hardDeleteTags(db *gorm.DB, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("topic_tag_id IN ?", ids).Delete(&models.TopicTagEmbedding{}).Error; err != nil {
			return fmt.Errorf("delete embeddings: %w", err)
		}
		if err := tx.Where("parent_id IN ? OR child_id IN ?", ids, ids).
			Where("relation_type = ?", "abstract").Delete(&models.TopicTagRelation{}).Error; err != nil {
			return fmt.Errorf("delete relations: %w", err)
		}
		if err := tx.Where("topic_tag_id IN ?", ids).Delete(&models.ArticleTopicTag{}).Error; err != nil {
			return fmt.Errorf("delete article links: %w", err)
		}
		if err := tx.Where("id IN ?", ids).Delete(&models.TopicTag{}).Error; err != nil {
			return fmt.Errorf("delete tags: %w", err)
		}
		return nil
	})
}
