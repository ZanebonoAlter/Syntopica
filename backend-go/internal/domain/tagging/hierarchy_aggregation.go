package tagging

import (
	"context"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

// AggregateOrphanTags finds abstract tags at depth < maxDepth that have children
// but no parent (orphans), groups by concept, and places them upward.
func AggregateOrphanTags(ctx context.Context, category string) (int, error) {
	maxDepth := getMaxDepthForCategory(category)

	// Find abstract tags with children (at least one child relation where they are parent)
	// but no parent relation where they are child (orphans)
	var orphans []models.TopicTag
	database.DB.Where(`category = ? AND source = 'abstract' AND status = 'active'
		AND id IN (SELECT parent_id FROM topic_tag_relations WHERE relation_type = 'abstract')
		AND id NOT IN (SELECT child_id FROM topic_tag_relations WHERE relation_type = 'abstract')`,
		category).Find(&orphans)

	placed := 0
	for _, tag := range orphans {
		depth := getTagDepthFromRoot(tag.ID)
		if depth >= maxDepth {
			continue
		}

		tmpl := GetHierarchyManager().GetTemplate(tag.Category, tag.SubType)
		if tmpl == nil {
			continue
		}

		logging.Infof("AggregateOrphanTags: attempting to place orphan abstract %d (%s) depth=%d",
			tag.ID, tag.Label, depth)

		aggregateToUpperLevel(ctx, &tag, tmpl, depth)
		placed++
	}
	return placed, nil
}

// aggregateToUpperLevel places an abstract tag upward in the hierarchy.
func aggregateToUpperLevel(ctx context.Context, parentTag *models.TopicTag, tmpl *CategoryHierarchyTemplate, currentDepth int) {
	if currentDepth <= 0 {
		return
	}

	targetDepth := currentDepth
	levelDef := tmpl.Levels[targetDepth]

	result := &PlacementResult{TagID: parentTag.ID}

	es := NewEmbeddingService()
	candidates, err := es.FindSimilarAbstractTags(ctx, parentTag.ID, parentTag.Category, placementCandidateLimit)
	if err != nil {
		logging.Warnf("aggregateToUpperLevel: find candidates for %d failed: %v", parentTag.ID, err)
		return
	}

	// Filter to tags at targetDepth-1
	var filtered []TagCandidate
	for _, c := range candidates {
		if c.Tag == nil || c.Tag.ID == parentTag.ID {
			continue
		}
		if c.Tag.Source != "abstract" {
			continue
		}
		depth := getTagDepthFromRoot(c.Tag.ID)
		if depth != targetDepth-1 {
			continue
		}
		filtered = append(filtered, c)
	}

	// Step 1: Direct attach if high similarity
	if len(filtered) > 0 && filtered[0].Similarity >= 0.85 {
		linkTagToParent(ctx, parentTag.ID, filtered[0].Tag.ID, result)
		logging.Infof("aggregateToUpperLevel: attached %d under %d via high similarity", parentTag.ID, filtered[0].Tag.ID)
		return
	}

	// Step 2: LLM match for mid-range
	if len(filtered) > 0 && filtered[0].Similarity >= 0.65 {
		selected, err := callLLMForMatch(ctx, parentTag, filtered, tmpl, &levelDef, targetDepth)
		if err != nil {
			logging.Warnf("aggregateToUpperLevel: LLM match failed for %d: %v", parentTag.ID, err)
		} else if selected != nil {
			linkTagToParent(ctx, parentTag.ID, selected.ID, result)
			logging.Infof("aggregateToUpperLevel: attached %d under %d via LLM", parentTag.ID, selected.ID)
			return
		}
	}

	logging.Infof("aggregateToUpperLevel: skipped node creation for orphan %d; PlaceTagInHierarchy owns node creation", parentTag.ID)
}
