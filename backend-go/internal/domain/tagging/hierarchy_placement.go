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
	anchorHighThreshold            = 0.85
	anchorMidThreshold             = 0.70
	placementCandidateLimit        = 15
	orphanPlacementMinAge          = 10 * time.Minute
	maxArticleJaccardForCreation   = 0.70
	minLeafToDepthRatioForCreation = 1.5
)

type PlacementResult struct {
	TagID            uint   `json:"tag_id"`
	ParentID         *uint  `json:"parent_id,omitempty"`
	ParentLabel      string `json:"parent_label,omitempty"`
	CreatedParents   []uint `json:"created_parents,omitempty"`
	ConceptID        *uint  `json:"concept_id,omitempty"`
	Action           string `json:"action"`
	BlockerReason    string `json:"blocker_reason,omitempty"`
	DiagnosticAction string `json:"diagnostic_action,omitempty"`
}

type nodeCreationContext struct {
	CandidateChildIDs []uint
	MaxArticleJaccard float64
}

var createAbstractAtLevelFn = createAbstractAtLevel

func markPlacementBlocker(result *PlacementResult, action, reason, diagnostic string) *PlacementResult {
	result.Action = action
	result.BlockerReason = reason
	result.DiagnosticAction = diagnostic
	return result
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
		return markPlacementBlocker(&PlacementResult{TagID: tag.ID}, "pending_embedding", "missing_embedding", "generate_embedding"), nil
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
		return markPlacementBlocker(&PlacementResult{TagID: tag.ID}, "concept_match_failed", "concept_match_failed", "inspect_concept_matching"), nil
	}
	if conceptMatch == nil {
		logging.Infof("PlaceTagInHierarchy: tag %d has no matching concept, waiting for concept bootstrap", tag.ID)
		return markPlacementBlocker(&PlacementResult{TagID: tag.ID}, "no_matching_concept", "no_matching_concept", "bootstrap_concept"), nil
	}

	// Determine target depth
	tagDepth := getTagDepthFromRoot(tag.ID)
	maxDepth := getMaxDepthForCategory(tag.Category)
	if tagDepth >= maxDepth {
		return markPlacementBlocker(&PlacementResult{TagID: tag.ID}, "already_at_max_depth", "max_depth_reached", "review_hierarchy_template"), nil
	}
	targetDepth := tagDepth + 1

	// Non-abstract tags must be placed at leaf-appropriate levels per template
	if tag.Source != "abstract" {
		for targetDepth < len(tmpl.Levels) && !tmpl.Levels[targetDepth].IsLeaf {
			targetDepth++
		}
		if targetDepth >= len(tmpl.Levels) || !tmpl.Levels[targetDepth].IsLeaf {
			return markPlacementBlocker(&PlacementResult{TagID: tag.ID}, "no_suitable_level", "no_leaf_level", "review_hierarchy_template"), nil
		}
	}

	// Place at target depth within concept
	return placeTagAtLevel(ctx, tag, tmpl, targetDepth, conceptMatch)
}

// placeTagAtLevel places a tag at the given depth within a concept.
// Flow: anchor search → abstract embedding match → resolveParent
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

	// Step 5: No matching abstract found, decide whether a guarded node can be created.
	logging.Infof("placeTagAtLevel: tag=%d no parent at depth=%d category=%s, evaluating node creation",
		tag.ID, targetDepth, tag.Category)
	return decideNodeCreation(ctx, tag, tmpl, &levelDef, targetDepth, conceptMatch.ConceptID, anchors, candidates, result)
}

func decideNodeCreation(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, levelDef *AbstractionLevel, targetDepth int, conceptID uint, anchors []Anchor, candidates []TagCandidate, result *PlacementResult) (*PlacementResult, error) {
	creationCtx := collectNodeCreationContext(ctx, tag, conceptID, anchors, candidates)
	if reason, diagnostic := validateNodeCreationContext(creationCtx, targetDepth, len(anchors), len(candidates)); reason != "" {
		logging.Infof("decideNodeCreation: tag=%d blocked reason=%s candidates=%d max_jaccard=%.4f",
			tag.ID, reason, len(creationCtx.CandidateChildIDs), creationCtx.MaxArticleJaccard)
		return markPlacementBlocker(result, "unplaced", reason, diagnostic), nil
	}

	parentID, parentLabel, err := createAbstractAtLevelFn(ctx, tag, tmpl, levelDef, conceptID)
	if err != nil {
		logging.Warnf("decideNodeCreation: create abstract for tag %d failed: %v", tag.ID, err)
		return markPlacementBlocker(result, "unplaced", "node_creation_failed", "inspect_hierarchy_create"), nil
	}

	result.Action = "created_node"
	result.ParentID = &parentID
	result.ParentLabel = parentLabel
	result.CreatedParents = append(result.CreatedParents, parentID)
	return triggerUpwardAggregation(ctx, parentID, tmpl, targetDepth, result)
}

func collectNodeCreationContext(ctx context.Context, tag *models.TopicTag, conceptID uint, anchors []Anchor, candidates []TagCandidate) nodeCreationContext {
	_ = ctx
	childIDs := map[uint]bool{tag.ID: true}
	anchorParentIDs := map[uint]bool{}
	for _, anchor := range anchors {
		anchorParentIDs[anchor.ParentID] = true
	}
	for _, candidate := range candidates {
		if candidate.Tag != nil {
			anchorParentIDs[candidate.Tag.ID] = true
		}
	}

	if len(anchorParentIDs) > 0 {
		parentIDs := make([]uint, 0, len(anchorParentIDs))
		for id := range anchorParentIDs {
			parentIDs = append(parentIDs, id)
		}

		var relations []models.TopicTagRelation
		database.DB.Where("parent_id IN ? AND relation_type = ?", parentIDs, "abstract").Find(&relations)
		for _, relation := range relations {
			if relation.ChildID != tag.ID && isActiveTagInConcept(relation.ChildID, tag.Category, conceptID) {
				childIDs[relation.ChildID] = true
			}
		}
	}

	ids := make([]uint, 0, len(childIDs))
	for id := range childIDs {
		ids = append(ids, id)
	}

	return nodeCreationContext{
		CandidateChildIDs: ids,
		MaxArticleJaccard: maxArticleJaccard(tag.ID, ids),
	}
}

func validateNodeCreationContext(creationCtx nodeCreationContext, targetDepth int, anchorCount int, candidateCount int) (string, string) {
	if anchorCount == 0 && candidateCount == 0 {
		return "no_anchor_context", "generate_anchor_context"
	}
	if len(creationCtx.CandidateChildIDs) < 2 {
		return "insufficient_siblings", "wait_for_more_siblings"
	}
	if creationCtx.MaxArticleJaccard > maxArticleJaccardForCreation {
		return "low_information_gain", "review_article_overlap"
	}
	depth := targetDepth
	if depth < 1 {
		depth = 1
	}
	if float64(len(creationCtx.CandidateChildIDs))/float64(depth) < minLeafToDepthRatioForCreation {
		return "low_information_gain", "wait_for_more_leaf_tags"
	}
	return "", ""
}

func isActiveTagInConcept(tagID uint, category string, conceptID uint) bool {
	var tag models.TopicTag
	if err := database.DB.Select("id", "concept_id").Where("id = ? AND category = ? AND status = ?", tagID, category, "active").First(&tag).Error; err != nil {
		return false
	}
	return tag.ConceptID != nil && *tag.ConceptID == conceptID
}

func maxArticleJaccard(tagID uint, candidateIDs []uint) float64 {
	base := articleSetForTag(tagID)
	maxJaccard := 0.0
	for _, candidateID := range candidateIDs {
		if candidateID == tagID {
			continue
		}
		jaccard := articleJaccard(base, articleSetForTag(candidateID))
		if jaccard > maxJaccard {
			maxJaccard = jaccard
		}
	}
	return maxJaccard
}

func articleSetForTag(tagID uint) map[uint]bool {
	var rows []models.ArticleTopicTag
	database.DB.Select("article_id").Where("topic_tag_id = ?", tagID).Find(&rows)
	set := make(map[uint]bool, len(rows))
	for _, row := range rows {
		set[row.ArticleID] = true
	}
	return set
}

func articleJaccard(left, right map[uint]bool) float64 {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	intersection := 0
	for id := range left {
		if right[id] {
			intersection++
		}
	}
	union := len(left) + len(right) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
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

	var activeSignals []models.HierarchyAnchorSignal
	if err := database.DB.Where("category = ? AND expires_at > ?", tag.Category, time.Now()).Find(&activeSignals).Error; err == nil {
		for _, signal := range activeSignals {
			containsTag := false
			for _, memberID := range signal.MemberTagIDs {
				if memberID == tag.ID {
					containsTag = true
					break
				}
			}
			if !containsTag {
				continue
			}

			for _, memberID := range signal.MemberTagIDs {
				if memberID == tag.ID {
					continue
				}

				var rel models.TopicTagRelation
				if err := database.DB.Where("child_id = ? AND relation_type = ?", memberID, "abstract").First(&rel).Error; err != nil {
					continue
				}
				if seen[rel.ParentID] {
					continue
				}

				var parentTag models.TopicTag
				if err := database.DB.First(&parentTag, rel.ParentID).Error; err != nil {
					continue
				}
				if parentTag.ConceptID == nil || *parentTag.ConceptID != conceptID {
					continue
				}

				seen[rel.ParentID] = true
				anchors = append(anchors, Anchor{
					ParentID:    rel.ParentID,
					ParentLabel: parentTag.Label,
					Similarity:  0.82,
					Source:      "anchor_signal",
				})
			}
		}
	}

	// Cotag search: find tags co-occurring with this tag in articles
	type cotagResult struct {
		ParentID    uint
		ParentLabel string
		TagID       uint
	}
	var cotagParents []cotagResult
	database.DB.Raw(`
		SELECT DISTINCT tr.parent_id, pt.label as parent_label, att2.topic_tag_id as tag_id
		FROM article_topic_tags att
		JOIN article_topic_tags att2 ON att.article_id = att2.article_id AND att2.topic_tag_id != att.topic_tag_id
		JOIN topic_tag_relations tr ON tr.child_id = att2.topic_tag_id AND tr.relation_type = 'abstract'
		JOIN topic_tags pt ON pt.id = tr.parent_id
		WHERE att.topic_tag_id = ? AND tr.parent_id IN (
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
				if c.Tag == nil {
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
				if seen[rel.ParentID] {
					continue
				}
				if parentTag.ConceptID == nil || *parentTag.ConceptID != conceptID {
					continue
				}
				seen[rel.ParentID] = true
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
	return retryOrphanPlacements(ctx, "")
}

func RetryOrphanPlacementsForCategory(ctx context.Context, category string) (int, error) {
	return retryOrphanPlacements(ctx, category)
}

func retryOrphanPlacements(ctx context.Context, category string) (int, error) {
	cutoff := time.Now().Add(-orphanPlacementMinAge)

	var tags []models.TopicTag
	query := database.DB.Where("status = 'active' AND source IN ('llm', 'heuristic') AND created_at < ?", cutoff)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	query.
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
		if result.Action != "pending_embedding" && result.Action != "no_matching_concept" &&
			result.Action != "unplaced" && result.Action != "no_suitable_level" {
			placed++
		}
	}
	return placed, nil
}
