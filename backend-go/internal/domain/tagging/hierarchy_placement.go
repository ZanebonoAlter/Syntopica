package tagging

import (
	"context"
	"fmt"
	"time"

	"my-robot-backend/internal/domain/concept"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const (
	anchorHighThreshold     = 0.85
	anchorMidThreshold      = 0.70
	placementCandidateLimit = 15
	orphanPlacementMinAge   = 10 * time.Minute
)

type PlacementResult struct {
	TagID          uint   `json:"tag_id"`
	ParentID       *uint  `json:"parent_id,omitempty"`
	ParentLabel    string `json:"parent_label,omitempty"`
	CreatedParents []uint `json:"created_parents,omitempty"`
	ConceptID      *uint  `json:"concept_id,omitempty"`
	Action         string `json:"action"`
}

// PlaceTagInHierarchy places a leaf tag into the hierarchy.
// New flow: embedding check → MatchTagToConcept → depth check → placeTagAtLevel
func PlaceTagInHierarchy(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
	tmpl := GetHierarchyManager().GetTemplate(tag.Category, tag.SubType)
	if tmpl == nil {
		return nil, fmt.Errorf("no hierarchy template for category=%s sub_type=%s", tag.Category, tag.SubType)
	}

	// Check if tag has semantic embedding ready
	var embCount int64
	database.DB.Model(&models.TopicTagEmbedding{}).
		Where("topic_tag_id = ? AND embedding_type = ?", tag.ID, "semantic").
		Count(&embCount)
	if embCount == 0 {
		return &PlacementResult{TagID: tag.ID, Action: "pending_embedding"}, nil
	}

	// Check if already placed
	if tag.Source != "abstract" {
		depth := getTagDepthFromRoot(tag.ID)
		if depth > 0 {
			return &PlacementResult{TagID: tag.ID, Action: "already_placed"}, nil
		}
	}

	// Match tag to concept
	conceptMatch, err := concept.MatchTagToConcept(ctx, tag.Label, tag.Description, tag.Category, tag.ID)
	if err != nil {
		logging.Warnf("PlaceTagInHierarchy: concept match failed for tag %d: %v", tag.ID, err)
		return &PlacementResult{TagID: tag.ID, Action: "concept_match_failed"}, nil
	}
	if conceptMatch == nil {
		logging.Infof("PlaceTagInHierarchy: tag %d has no matching concept, waiting for concept bootstrap", tag.ID)
		return &PlacementResult{TagID: tag.ID, Action: "no_matching_concept"}, nil
	}

	// Determine target depth
	tagDepth := getTagDepthFromRoot(tag.ID)
	maxDepth := getMaxDepthForCategory(tag.Category)
	if tagDepth >= maxDepth {
		return &PlacementResult{TagID: tag.ID, Action: "already_at_max_depth"}, nil
	}
	targetDepth := tagDepth + 1

	// Place at target depth within concept
	return placeTagAtLevel(ctx, tag, tmpl, targetDepth, conceptMatch)
}

// placeTagAtLevel places a tag at the given depth within a concept.
// Flow: anchor search → abstract embedding match → resolveParent → createAbstractAtLevel
func placeTagAtLevel(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, targetDepth int, conceptMatch *concept.ConceptMatchResult) (*PlacementResult, error) {
	result := &PlacementResult{TagID: tag.ID, ConceptID: &conceptMatch.ConceptID}

	if targetDepth >= len(tmpl.Levels) {
		return result, fmt.Errorf("target depth %d exceeds template levels %d", targetDepth, len(tmpl.Levels))
	}
	levelDef := tmpl.Levels[targetDepth]

	// Step 1: Anchor search (cotag + embedding)
	anchors, err := searchAnchors(ctx, tag, conceptMatch.ConceptID, targetDepth)
	if err != nil {
		logging.Warnf("placeTagAtLevel: anchor search failed for tag %d: %v", tag.ID, err)
	}

	// Step 2: Anchor-based placement decision
	if len(anchors) > 0 {
		bestAnchor := anchors[0]
		if bestAnchor.Similarity >= anchorHighThreshold {
			// Direct follow - high confidence
			linkTagToParent(ctx, tag.ID, bestAnchor.ParentID, result)
			result.Action = "placed_via_anchor_high"
			result.ParentLabel = bestAnchor.ParentLabel
			return triggerUpwardAggregation(ctx, bestAnchor.ParentID, tmpl, targetDepth, result)
		} else if bestAnchor.Similarity >= anchorMidThreshold && len(anchors) >= 2 {
			// LLM voting for anchor consensus
			parentIDs := collectUniqueParents(anchors[:min(3, len(anchors))])
			if len(parentIDs) == 1 {
				// Consensus - all top anchors point to same parent
				linkTagToParent(ctx, tag.ID, parentIDs[0], result)
				result.Action = "placed_via_anchor_consensus"
				return triggerUpwardAggregation(ctx, parentIDs[0], tmpl, targetDepth, result)
			}
			// Divergent - LLM vote
			selected := callLLMForAnchorVote(ctx, tag, anchors[:min(3, len(anchors))], tmpl, &levelDef)
			if selected != nil {
				linkTagToParent(ctx, tag.ID, selected.ParentID, result)
				result.Action = "placed_via_anchor_llm"
				result.ParentLabel = selected.ParentLabel
				return triggerUpwardAggregation(ctx, selected.ParentID, tmpl, targetDepth, result)
			}
		}
	}

	// Step 3: Fall through to abstract embedding matching
	es := NewEmbeddingService()
	candidates, err := es.FindSimilarAbstractTags(ctx, tag.ID, tag.Category, placementCandidateLimit)
	if err != nil {
		return result, fmt.Errorf("find abstract candidates: %w", err)
	}

	// Filter by concept
	candidates = filterByConcept(candidates, conceptMatch.ConceptID, targetDepth)

	// Step 4: Resolve parent (3-threshold: high=0.85, low=0.65)
	parent, err := resolveParent(ctx, tag, candidates, tmpl, &levelDef, targetDepth)
	if err != nil {
		return result, err
	}

	if parent != nil {
		linkTagToParent(ctx, tag.ID, parent.ID, result)
		result.Action = "placed_via_abstract_match"
		result.ParentLabel = parent.Label
		return triggerUpwardAggregation(ctx, parent.ID, tmpl, targetDepth, result)
	}

	// Step 5: Create new abstract
	parentID, parentLabel, err := createAbstractAtLevel(ctx, tag, tmpl, &levelDef, conceptMatch.ConceptID)
	if err != nil {
		return result, fmt.Errorf("create abstract at level %d: %w", targetDepth, err)
	}
	result.ParentID = &parentID
	result.ParentLabel = parentLabel
	result.CreatedParents = append(result.CreatedParents, parentID)
	result.Action = "placed_via_new_abstract"

	// Trigger async embedding generation for new abstract
	//nolint:gosec // intentional background task
	go generateAbstractEmbedding(parentID)

	return triggerUpwardAggregation(ctx, parentID, tmpl, targetDepth, result)
}

// Anchor represents a cotag/embedding anchor signal
type Anchor struct {
	ParentID    uint    `json:"parent_id"`
	ParentLabel string  `json:"parent_label"`
	Similarity  float64 `json:"similarity"`
	Source      string  `json:"source"` // "cotag" or "embedding"
	Depth       int     `json:"depth"`
}

// searchAnchors finds anchors for a tag: cotag first, then embedding supplement
func searchAnchors(ctx context.Context, tag *models.TopicTag, conceptID uint, targetDepth int) ([]Anchor, error) {
	var anchors []Anchor
	seen := make(map[uint]bool)

	// Cotag search: find tags co-occurring with this tag in articles
	type cotagResult struct {
		ParentID    uint
		ParentLabel string
		TagID       uint
	}
	var cotagParents []cotagResult
	database.DB.Raw(`
		SELECT DISTINCT tr.parent_id, pt.label as parent_label, at.tag_id
		FROM article_topic_tags att
		JOIN article_topic_tags att2 ON att.article_id = att2.article_id AND att2.tag_id != att.tag_id
		JOIN topic_tag_relations tr ON tr.child_id = att2.tag_id AND tr.relation_type = 'abstract'
		JOIN topic_tags pt ON pt.id = tr.parent_id
		JOIN article_topic_tags at ON at.article_id = att.article_id
		WHERE att.tag_id = ? AND tr.parent_id IN (
			SELECT id FROM topic_tags WHERE concept_id = ? AND status = 'active'
		)
		LIMIT 20
	`, tag.ID, conceptID).Scan(&cotagParents)

	for _, cp := range cotagParents {
		if seen[cp.ParentID] {
			continue
		}
		seen[cp.ParentID] = true
		anchors = append(anchors, Anchor{
			ParentID:    cp.ParentID,
			ParentLabel: cp.ParentLabel,
			Similarity:  0.80, // default cotag confidence
			Source:      "cotag",
		})
	}

	// Embedding supplement if < 2 anchors found
	if len(anchors) < 2 {
		es := NewEmbeddingService()
		embCandidates, err := es.FindSimilarAbstractTags(ctx, tag.ID, tag.Category, 10)
		if err == nil {
			for _, c := range embCandidates {
				if c.Tag == nil || seen[c.Tag.ID] {
					continue
				}
				// Only include tags that have a parent
				parentDepth := getTagDepthFromRoot(c.Tag.ID)
				if parentDepth+1 != targetDepth {
					continue
				}
				// Find the tag's parent
				var rel models.TopicTagRelation
				if err := database.DB.Where("child_id = ? AND relation_type = 'abstract'", c.Tag.ID).
					First(&rel).Error; err != nil {
					continue
				}
				var parentTag models.TopicTag
				if err := database.DB.First(&parentTag, rel.ParentID).Error; err != nil {
					continue
				}
				if parentTag.ConceptID == nil || *parentTag.ConceptID != conceptID {
					continue
				}
				seen[c.Tag.ID] = true
				anchors = append(anchors, Anchor{
					ParentID:    rel.ParentID,
					ParentLabel: parentTag.Label,
					Similarity:  c.Similarity,
					Source:      "embedding",
				})
			}
		}
	}

	// Sort by similarity descending
	sortAnchors(anchors)
	return anchors, nil
}

func sortAnchors(anchors []Anchor) {
	for i := 0; i < len(anchors); i++ {
		for j := i + 1; j < len(anchors); j++ {
			if anchors[j].Similarity > anchors[i].Similarity {
				anchors[i], anchors[j] = anchors[j], anchors[i]
			}
		}
	}
}

func collectUniqueParents(anchors []Anchor) []uint {
	seen := make(map[uint]bool)
	var result []uint
	for _, a := range anchors {
		if !seen[a.ParentID] {
			seen[a.ParentID] = true
			result = append(result, a.ParentID)
		}
	}
	return result
}

func linkTagToParent(ctx context.Context, childID, parentID uint, result *PlacementResult) {
	// Prevent cycles
	if childID == parentID {
		return
	}
	// Check if relation already exists
	var existing models.TopicTagRelation
	if err := database.DB.Where("parent_id = ? AND child_id = ? AND relation_type = 'abstract'",
		parentID, childID).First(&existing).Error; err == nil {
		result.ParentID = &parentID
		return
	}
	if err := database.DB.Create(&models.TopicTagRelation{
		ParentID:     parentID,
		ChildID:      childID,
		RelationType: "abstract",
	}).Error; err != nil {
		logging.Warnf("linkTagToParent: failed to link child %d to parent %d: %v", childID, parentID, err)
	} else {
		result.ParentID = &parentID
	}
}

func filterByConcept(candidates []TagCandidate, conceptID uint, targetDepth int) []TagCandidate {
	var filtered []TagCandidate
	for _, c := range candidates {
		if c.Tag == nil || c.Tag.ConceptID == nil {
			continue
		}
		if *c.Tag.ConceptID != conceptID {
			continue
		}
		if c.Tag.Source != "abstract" {
			continue
		}
		// Check depth matches target
		depth := getTagDepthFromRoot(c.Tag.ID)
		if depth != targetDepth {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// resolveParent determines the best parent for a tag using 3-threshold decision
func resolveParent(ctx context.Context, tag *models.TopicTag, candidates []TagCandidate, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel, targetDepth int) (*models.TopicTag, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	top := candidates[0]
	highThreshold := 0.85
	lowThreshold := 0.65

	if top.Similarity >= highThreshold {
		logging.Infof("resolveParent: tag=%d high sim=%.4f, direct attach to %d(%s)",
			tag.ID, top.Similarity, top.Tag.ID, top.Tag.Label)
		return top.Tag, nil
	}

	if top.Similarity < lowThreshold {
		return nil, nil // Create new abstract
	}

	// Mid-range: LLM judgment
	logging.Infof("resolveParent: tag=%d mid sim=%.4f, LLM judgment needed", tag.ID, top.Similarity)
	selected, err := callLLMForMatch(ctx, tag, candidates, tmpl, levelDef, targetDepth)
	if err != nil {
		logging.Warnf("resolveParent: LLM match failed for tag=%d: %v", tag.ID, err)
		return nil, nil
	}
	return selected, nil
}

// createAbstractAtLevel creates a new abstract tag at the given level
func createAbstractAtLevel(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel, conceptID uint) (uint, string, error) {
	label, desc, err := callLLMForCreation(ctx, tag, tmpl, levelDef)
	if err != nil {
		return 0, "", fmt.Errorf("LLM create abstract: %w", err)
	}

	abstract := models.TopicTag{
		Label:       label,
		Category:    tag.Category,
		Source:      "abstract",
		Description: desc,
		Status:      "active",
		IsCanonical: true,
		ConceptID:   &conceptID,
	}

	if err := database.DB.Create(&abstract).Error; err != nil {
		return 0, "", fmt.Errorf("create abstract tag: %w", err)
	}

	// Link child to parent
	if err := database.DB.Create(&models.TopicTagRelation{
		ParentID:     abstract.ID,
		ChildID:      tag.ID,
		RelationType: "abstract",
	}).Error; err != nil {
		logging.Warnf("createAbstractAtLevel: failed to link abstract %d to child %d: %v", abstract.ID, tag.ID, err)
	}

	// Async embedding and dedup
	//nolint:gosec // intentional background task
	go func() {
		generateAbstractEmbedding(abstract.ID)
		bgCtx := context.Background()
		dedupAtDepth(bgCtx, &abstract, getTagDepthFromRoot(abstract.ID))
	}()

	logging.Infof("createAbstractAtLevel: created abstract %d label=%s category=%s depth=%d concept=%d",
		abstract.ID, abstract.Label, abstract.Category, getTagDepthFromRoot(abstract.ID), conceptID)
	return abstract.ID, abstract.Label, nil
}

func generateAbstractEmbedding(tagID uint) {
	var tag models.TopicTag
	if err := database.DB.First(&tag, tagID).Error; err != nil {
		return
	}
	es := NewEmbeddingService()
	bgCtx := context.Background()
	_, _ = es.GenerateEmbedding(bgCtx, &tag, EmbeddingTypeIdentity)
	_, _ = es.GenerateEmbedding(bgCtx, &tag, EmbeddingTypeSemantic)
}

// triggerUpwardAggregation checks if the parent should be aggregated upward
func triggerUpwardAggregation(ctx context.Context, parentID uint, tmpl *CategoryHierarchyTemplate, currentDepth int, result *PlacementResult) (*PlacementResult, error) {
	maxDepth := getMaxDepthForCategory("") // category from template
	_ = maxDepth                           // Will be used when aggregation is implemented fully
	if currentDepth >= maxDepth {
		return result, nil
	}

	// Count children
	var childCount int64
	database.DB.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND relation_type = 'abstract'", parentID).
		Count(&childCount)

	if childCount >= 3 {
		var parentTag models.TopicTag
		if err := database.DB.First(&parentTag, parentID).Error; err == nil {
			logging.Infof("triggerUpwardAggregation: parent %d has %d children, triggering aggregation to depth %d",
				parentID, childCount, currentDepth+1)
			//nolint:gosec // intentional background task
			go aggregateToUpperLevel(context.Background(), &parentTag, tmpl, currentDepth)
		}
	}
	return result, nil
}

// RetryOrphanPlacements retries placement for leaf tags without parents created > orphanPlacementMinAge ago
func RetryOrphanPlacements(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-orphanPlacementMinAge)

	var tags []models.TopicTag
	database.DB.Where("status = 'active' AND source IN ('llm', 'heuristic') AND created_at < ?", cutoff).
		Where("id NOT IN (SELECT child_id FROM topic_tag_relations WHERE relation_type = 'abstract')").
		Limit(100).
		Find(&tags)

	placed := 0
	for _, tag := range tags {
		result, err := PlaceTagInHierarchy(ctx, &tag)
		if err != nil {
			logging.Warnf("RetryOrphanPlacements: failed for tag %d: %v", tag.ID, err)
			continue
		}
		if result.Action != "pending_embedding" && result.Action != "no_matching_concept" {
			placed++
		}
	}
	return placed, nil
}
