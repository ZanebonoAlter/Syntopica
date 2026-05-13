package tagging

import (
	"context"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

func checkDepthLimit(tx *gorm.DB, parentID, childID uint, category string) error {
	childSubtreeDepth := getAbstractSubtreeDepth(tx, childID)
	parentAncestryDepth := getTagDepthFromRootDB(tx, parentID)
	maxDepth := getMaxDepthForCategory(category)
	if childSubtreeDepth+parentAncestryDepth+1 > maxDepth {
		return fmt.Errorf("depth limit: placing subtree(depth=%d) under parent(ancestry=%d) would exceed max depth %d", childSubtreeDepth, parentAncestryDepth, maxDepth)
	}
	return nil
}

func MatchAbstractTagHierarchy(ctx context.Context, abstractTagID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("MatchAbstractTagHierarchy panic for tag %d: %v", abstractTagID, r)
		}
	}()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: tag %d not found: %v", abstractTagID, err)
		return
	}

	es := NewEmbeddingService()
	thresholds := es.GetThresholds()

	treeDuplicates, err := findCrossLayerDuplicateCandidatesFn(ctx, abstractTagID, abstractTag.Category)
	if err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: cross-layer dedup search failed: %v", err)
	} else {
		for _, dup := range treeDuplicates {
			if dup.Tag == nil {
				continue
			}
			shouldMerge, reason, judgeErr := judgeCrossLayerDuplicateFn(ctx, abstractTagID, dup.Tag.ID)
			if judgeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: cross-layer judge failed for %d vs %d: %v", abstractTagID, dup.Tag.ID, judgeErr)
				continue
			}
			if !shouldMerge {
				logging.Infof("MatchAbstractTagHierarchy: candidate %d rejected by cross-layer judge for %d: %s", dup.Tag.ID, abstractTagID, reason)
				continue
			}
			if mergeErr := mergeTagsFn(abstractTagID, dup.Tag.ID); mergeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: cross-layer merge failed for %d into %d: %v", abstractTagID, dup.Tag.ID, mergeErr)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: merged %d into %d (cross-layer dedup, reason=%s)", abstractTagID, dup.Tag.ID, reason)
			return
		}
	}

	candidates, err := es.FindSimilarAbstractTags(ctx, abstractTagID, abstractTag.Category, 0)
	if err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: failed to find similar abstract tags for %d: %v", abstractTagID, err)
		return
	}
	if len(candidates) == 0 {
		return
	}

	var highSimilars []TagCandidate
	var mediumSimilars []TagCandidate
	for _, candidate := range candidates {
		if candidate.Tag == nil {
			continue
		}
		if candidate.Similarity >= thresholds.HighSimilarity {
			highSimilars = append(highSimilars, candidate)
		} else if candidate.Similarity >= thresholds.LowSimilarity {
			mediumSimilars = append(mediumSimilars, candidate)
		}
	}

	for _, candidate := range highSimilars {
		if err := mergeOrLinkSimilarAbstract(ctx, abstractTagID, candidate.Tag.ID); err != nil {
			logging.Warnf("MatchAbstractTagHierarchy: merge/link failed for %d vs %d: %v", abstractTagID, candidate.Tag.ID, err)
			continue
		}
		return
	}

	if len(mediumSimilars) == 0 {
		return
	}

	batchResults, err := batchJudgeAbstractRelationships(ctx, &abstractTag, mediumSimilars)
	if err != nil {
		logging.Warnf("MatchAbstractTagHierarchy: batch AI judgment failed for %d: %v", abstractTagID, err)
		return
	}

	for _, r := range batchResults {
		if r.Action == "merge" {
			sourceID, targetID := abstractTagID, r.TagID
			if strings.EqualFold(r.Target, "A") {
				sourceID, targetID = r.TagID, abstractTagID
			}
			if mergeErr := MergeTags(sourceID, targetID); mergeErr != nil {
				logging.Warnf("MatchAbstractTagHierarchy: merge failed for %d into %d: %v", sourceID, targetID, mergeErr)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: merged %d into %d (batch AI judged, reason=%s)", sourceID, targetID, r.Reason)
			return
		}
	}

	for _, r := range batchResults {
		switch r.Action {
		case "parent_A":
			if err := linkAbstractParentChild(r.TagID, abstractTagID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: failed to link %d under %d: %v", r.TagID, abstractTagID, err)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: %d is child of %d (batch AI judged, reason=%s)", r.TagID, abstractTagID, r.Reason)
		case "parent_B":
			if err := linkAbstractParentChild(abstractTagID, r.TagID); err != nil {
				logging.Warnf("MatchAbstractTagHierarchy: failed to link %d under %d: %v", abstractTagID, r.TagID, err)
				continue
			}
			logging.Infof("MatchAbstractTagHierarchy: %d is child of %d (batch AI judged, reason=%s)", abstractTagID, r.TagID, r.Reason)
		case "skip":
			logging.Infof("MatchAbstractTagHierarchy: skipped %d vs %d (batch AI judged, reason=%s)", abstractTagID, r.TagID, r.Reason)
		}
	}
}

func adoptNarrowerAbstractChildren(ctx context.Context, abstractTagID uint) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("adoptNarrowerAbstractChildren panic for tag %d: %v", abstractTagID, r)
			retErr = fmt.Errorf("panic: %v", r)
		}
	}()

	var abstractTag models.TopicTag
	if err := database.DB.First(&abstractTag, abstractTagID).Error; err != nil {
		return fmt.Errorf("tag %d not found: %w", abstractTagID, err)
	}

	es := NewEmbeddingService()
	candidates, err := es.FindSimilarAbstractTags(ctx, abstractTagID, abstractTag.Category, 0)
	if err != nil {
		return fmt.Errorf("similarity search failed for %d: %w", abstractTagID, err)
	}
	if len(candidates) == 0 {
		return nil
	}

	thresholds := es.GetThresholds()
	var eligible []TagCandidate
	for _, c := range candidates {
		if c.Tag != nil && c.Similarity >= thresholds.LowSimilarity {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	narrowerIDs, err := batchJudgeNarrowerConceptsFn(ctx, &abstractTag, eligible)
	if err != nil {
		return fmt.Errorf("batch judge narrower failed for %d: %w", abstractTagID, err)
	}

	adopted := 0
	for _, cid := range narrowerIDs {
		if err := reparentOrLinkAbstractChild(ctx, cid, abstractTagID); err != nil {
			logging.Warnf("adoptNarrowerAbstractChildren: failed to link %d under %d: %v", cid, abstractTagID, err)
			continue
		}
		adopted++
	}

	if adopted > 0 {
		logging.Infof("adoptNarrowerAbstractChildren: abstract tag %d (%s) adopted %d narrower abstract tags", abstractTagID, abstractTag.Label, adopted)
		EnqueueAbstractTagUpdate(abstractTagID, "adopted_narrower_children")
	}
	return nil
}

func linkAbstractParentChild(childID, parentID uint) error {
	var parent, child models.TopicTag
	if err := database.DB.First(&parent, parentID).Error; err != nil {
		return fmt.Errorf("load parent tag %d: %w", parentID, err)
	}
	if err := database.DB.First(&child, childID).Error; err != nil {
		return fmt.Errorf("load child tag %d: %w", childID, err)
	}
	if parent.Kind != "abstract" && parent.Source != "abstract" {
		return fmt.Errorf("linkAbstractParentChild: parent %d (%q) is not abstract (kind=%s source=%s)", parentID, parent.Label, parent.Kind, parent.Source)
	}
	if child.Kind != "abstract" && child.Source != "abstract" {
		return fmt.Errorf("linkAbstractParentChild: child %d (%q) is not abstract (kind=%s source=%s)", childID, child.Label, child.Kind, child.Source)
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		wouldCycle, err := wouldCreateCycle(tx, parentID, childID)
		if err != nil {
			return fmt.Errorf("cycle check: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("would create cycle: parent=%d, child=%d", parentID, childID)
		}

		childSubtreeDepth := getAbstractSubtreeDepth(tx, childID)
		parentAncestryDepth := getTagDepthFromRootDB(tx, parentID)
		maxDepth := getMaxDepthForCategory(parent.Category)
		if childSubtreeDepth+parentAncestryDepth+1 > maxDepth {
			return fmt.Errorf("depth limit: placing subtree(depth=%d) under parent(ancestry=%d) would exceed max depth %d", childSubtreeDepth, parentAncestryDepth, maxDepth)
		}

		var count int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", parentID, childID).
			Count(&count)
		if count > 0 {
			return nil
		}

		var existingParents []models.TopicTagRelation
		if err := tx.Where("child_id = ? AND relation_type = ?", childID, "abstract").Find(&existingParents).Error; err != nil {
			return fmt.Errorf("check existing parents for child %d: %w", childID, err)
		}

		// Multi-parent prevention: if child already has an active abstract parent,
		// check that the new parent covers a meaningfully different article set.
		for _, old := range existingParents {
			if old.ParentID == parentID {
				continue
			}
			var oldParentTag models.TopicTag
			if tx.First(&oldParentTag, old.ParentID).Error == nil && oldParentTag.Status == "active" {
				articleSets, collectErr := collectArticleIDsForTags(tx, []uint{old.ParentID, parentID})
				if collectErr == nil {
					jaccard := jaccardSimilarity(articleSets[old.ParentID], articleSets[parentID])
					if jaccard > 0.3 {
						return fmt.Errorf("multi-parent prevention: new parent %d (%q) overlaps (Jaccard=%.2f) with existing parent %d (%q), rejecting",
							parentID, parent.Label, jaccard, old.ParentID, oldParentTag.Label)
					}
				}
			}
		}

		for _, old := range existingParents {
			if old.ParentID == parentID {
				continue
			}
			logging.Infof("linkAbstractParentChild: removing old parent %d from child %d before linking to new parent %d", old.ParentID, childID, parentID)
			if err := tx.Delete(&models.TopicTagRelation{}, old.ID).Error; err != nil {
				return fmt.Errorf("remove old parent relation %d for child %d: %w", old.ID, childID, err)
			}
		}

		relation := models.TopicTagRelation{
			ParentID:     parentID,
			ChildID:      childID,
			RelationType: "abstract",
		}
		return tx.Create(&relation).Error
	})
	if err != nil {
		return err
	}

	go enqueueEmbeddingsForNormalChildren(parentID)
	go EnqueueAbstractTagUpdate(parentID, "hierarchy_linked")

	return nil
}

type parentWithInfo struct {
	RelationID      uint
	Parent          *models.TopicTag
	SimilarityScore float64
}

type multiParentConflict struct {
	ChildID uint
	Parents []parentWithInfo
	Child   *models.TopicTag
}

// batchResolveMultiParentConflicts resolves multiple multi-parent conflicts in a single LLM call.
// First attempts to remove redundant ancestor parents without LLM, then batches remaining conflicts.
func batchResolveMultiParentConflicts(conflicts []multiParentConflict) (int, []string) {
	if len(conflicts) == 0 {
		return 0, nil
	}

	// Phase 1: Remove redundant ancestor parents (no LLM needed)
	resolved := 0
	var errors []string
	var remaining []multiParentConflict

	for _, c := range conflicts {
		txResolved := false
		if err := database.DB.Transaction(func(tx *gorm.DB) error {
			ok, err := removeRedundantAncestorParentsTx(tx, c.ChildID, c.Parents)
			if err != nil {
				return err
			}
			txResolved = ok
			return nil
		}); err != nil {
			errors = append(errors, fmt.Sprintf("child %d: ancestor check: %v", c.ChildID, err))
			continue
		}
		if txResolved {
			resolved++
			continue
		}
		remaining = append(remaining, c)
	}

	if len(remaining) == 0 {
		return resolved, errors
	}

	// Phase 2: Resolve by highest similarity score (no LLM)
	for _, c := range remaining {
		if len(c.Parents) == 0 {
			continue
		}
		bestIdx := 0
		bestScore := c.Parents[0].SimilarityScore
		bestChildrenCount := countAbstractChildren(c.Parents[0].Parent.ID)
		for i := 1; i < len(c.Parents); i++ {
			score := c.Parents[i].SimilarityScore
			childrenCount := countAbstractChildren(c.Parents[i].Parent.ID)
			if score > bestScore || (score == bestScore && childrenCount > bestChildrenCount) {
				bestIdx = i
				bestScore = score
				bestChildrenCount = childrenCount
			}
		}

		childID := c.ChildID
		parents := c.Parents
		if err := database.DB.Transaction(func(tx *gorm.DB) error {
			for i, p := range parents {
				if i == bestIdx {
					continue
				}
				if err := tx.Delete(&models.TopicTagRelation{}, p.RelationID).Error; err != nil {
					return fmt.Errorf("remove relation %d from child %d: %w", p.RelationID, childID, err)
				}
			}
			return nil
		}); err != nil {
			errors = append(errors, err.Error())
			continue
		}

		logging.Infof("batchResolveMultiParentConflicts: resolved child %d, kept parent %d (%s) (score=%.4f)",
			childID, parents[bestIdx].Parent.ID, parents[bestIdx].Parent.Label, bestScore)
		resolved++
	}

	return resolved, errors
}

func resolveMultiParentConflict(childID uint) (bool, error) {
	if database.DB == nil {
		return false, nil
	}
	var result bool
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var relations []models.TopicTagRelation
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("child_id = ? AND relation_type = ?", childID, "abstract").
			Preload("Parent").
			Find(&relations).Error; err != nil {
			return err
		}
		if len(relations) <= 1 {
			return nil
		}

		var childTag models.TopicTag
		if err := tx.First(&childTag, childID).Error; err != nil {
			return fmt.Errorf("load child tag %d: %w", childID, err)
		}

		var parents []parentWithInfo
		for _, r := range relations {
			if r.Parent == nil {
				continue
			}
			parents = append(parents, parentWithInfo{RelationID: r.ID, Parent: r.Parent, SimilarityScore: r.SimilarityScore})
		}
		if len(parents) <= 1 {
			return nil
		}
		if resolved, err := removeRedundantAncestorParentsTx(tx, childID, parents); err != nil {
			return err
		} else if resolved {
			result = true
			return nil
		}

		// Pick parent with highest similarity score; if tied, prefer the one with more children.
		bestIdx := 0
		bestScore := parents[0].SimilarityScore
		bestChildrenCount := countAbstractChildren(parents[0].Parent.ID)
		for i := 1; i < len(parents); i++ {
			score := parents[i].SimilarityScore
			childrenCount := countAbstractChildren(parents[i].Parent.ID)
			if score > bestScore || (score == bestScore && childrenCount > bestChildrenCount) {
				bestIdx = i
				bestScore = score
				bestChildrenCount = childrenCount
			}
		}

		removed := 0
		for i, p := range parents {
			if i == bestIdx {
				continue
			}
			if delErr := tx.Delete(&models.TopicTagRelation{}, p.RelationID).Error; delErr != nil {
				return fmt.Errorf("remove relation %d: %w", p.RelationID, delErr)
			} else {
				removed++
				logging.Infof("resolveMultiParentConflict: removed parent %d (%s) from child %d (%s), keeping parent %d (%s) (score=%.4f)",
					p.Parent.ID, p.Parent.Label, childID, childTag.Label,
					parents[bestIdx].Parent.ID, parents[bestIdx].Parent.Label, bestScore)
			}
		}

		keptParent := parents[bestIdx].Parent
		go EnqueueAbstractTagUpdate(keptParent.ID, "multi_parent_resolved")

		result = removed > 0
		return nil
	})
	return result, err
}

func countAbstractChildren(tagID uint) int {
	var count int64
	database.DB.Model(&models.TopicTag{}).
		Joins("JOIN topic_tag_relations ON topic_tag_relations.child_id = topic_tags.id").
		Where("topic_tag_relations.parent_id = ? AND topic_tag_relations.relation_type = ?", tagID, "abstract").
		Count(&count)
	return int(count)
}

func removeRedundantAncestorParentsTx(tx *gorm.DB, childID uint, parents []parentWithInfo) (bool, error) {
	removed := 0
	for _, maybeAncestor := range parents {
		for _, maybeDescendant := range parents {
			if maybeAncestor.Parent.ID == maybeDescendant.Parent.ID {
				continue
			}
			ancestor, err := isAbstractAncestorOf(maybeAncestor.Parent.ID, maybeDescendant.Parent.ID)
			if err != nil {
				return false, err
			}
			if !ancestor {
				continue
			}

			if delErr := tx.Delete(&models.TopicTagRelation{}, maybeAncestor.RelationID).Error; delErr != nil {
				return false, fmt.Errorf("remove redundant ancestor relation %d: %w", maybeAncestor.RelationID, delErr)
			}
			removed++
			logging.Infof("resolveMultiParentConflict: removed redundant ancestor parent %d (%s) from child %d, keeping narrower parent %d (%s)",
				maybeAncestor.Parent.ID, maybeAncestor.Parent.Label, childID,
				maybeDescendant.Parent.ID, maybeDescendant.Parent.Label)
			break
		}
	}

	return removed > 0, nil
}

func isAbstractAncestorOf(ancestorID, descendantID uint) (bool, error) {
	if ancestorID == 0 || descendantID == 0 || ancestorID == descendantID {
		return false, nil
	}

	visited := map[uint]bool{descendantID: true}
	queue := []uint{descendantID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		var relations []models.TopicTagRelation
		if err := database.DB.Where("child_id = ? AND relation_type = ?", current, "abstract").Find(&relations).Error; err != nil {
			return false, fmt.Errorf("load parents for ancestor check %d: %w", current, err)
		}
		for _, relation := range relations {
			if relation.ParentID == ancestorID {
				return true, nil
			}
			if !visited[relation.ParentID] {
				visited[relation.ParentID] = true
				queue = append(queue, relation.ParentID)
			}
		}
	}

	return false, nil
}
