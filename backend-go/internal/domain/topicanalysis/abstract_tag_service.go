package topicanalysis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topictypes"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

const (
	maxAbstractNameLen = 160
)

var errInsufficientAbstractChildren = errors.New("abstract tag needs enough children")

var (
	findSimilarExistingAbstractFn       = findSimilarExistingAbstract
	aiJudgeNarrowerConceptFn            = aiJudgeNarrowerConcept
	batchJudgeNarrowerConceptsFn        = batchJudgeNarrowerConcepts
	aiJudgeBestParentFn                 = aiJudgeBestParent
	findCrossLayerDuplicateCandidatesFn = findCrossLayerDuplicateCandidates
	judgeCrossLayerDuplicateFn          = judgeCrossLayerDuplicate
	aiJudgeAlternativePlacementFn       = aiJudgeAlternativePlacement
	mergeTagsFn                         = MergeTags
	callTreeReviewLLMFn                 = callTreeReviewLLM
)

type TagExtractionResult struct {
	Merge           *MergeResult       `json:"merge,omitempty"`
	Abstract        *AbstractResult    `json:"abstract,omitempty"`
	MergeChildren   []*models.TopicTag `json:"merge_children,omitempty"`
	LLMExplicitNone bool               `json:"llm_explicit_none,omitempty"`
}

type MergeResult struct {
	Target *models.TopicTag `json:"target"`
	Label  string           `json:"label"`
}

type AbstractResult struct {
	Tag      *models.TopicTag   `json:"tag"`
	Children []*models.TopicTag `json:"children"`
}

func (r *TagExtractionResult) HasMerge() bool    { return r != nil && r.Merge != nil }
func (r *TagExtractionResult) HasAbstract() bool { return r != nil && r.Abstract != nil }
func (r *TagExtractionResult) HasAction() bool   { return r.HasMerge() || r.HasAbstract() }

type ExtractAbstractTagOption func(*extractAbstractTagConfig)

type extractAbstractTagConfig struct {
	narrativeContext string
	caller           string
}

func WithNarrativeContext(ctx string) ExtractAbstractTagOption {
	return func(c *extractAbstractTagConfig) {
		c.narrativeContext = ctx
	}
}

func WithCaller(caller string) ExtractAbstractTagOption {
	return func(c *extractAbstractTagConfig) {
		c.caller = caller
	}
}

func ExtractAbstractTag(ctx context.Context, candidates []TagCandidate, newLabel string, category string, opts ...ExtractAbstractTagOption) (*TagExtractionResult, error) {
	if len(candidates) < 1 {
		return nil, fmt.Errorf("need at least 1 candidate for abstract tag extraction, got %d", len(candidates))
	}

	cfg := &extractAbstractTagConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if category == "" && len(candidates) > 0 && candidates[0].Tag != nil {
		category = candidates[0].Tag.Category
	}
	if category == "" {
		category = "keyword"
	}

	judgment, err := callLLMForTagJudgment(ctx, candidates, newLabel, category, cfg.narrativeContext, cfg.caller)
	if err != nil {
		logging.Warnf("Tag judgment LLM call failed: %v", err)
		return nil, err
	}

	return ProcessJudgment(ctx, judgment, candidates, newLabel, category)
}

func ProcessJudgment(ctx context.Context, judgment *tagJudgment, candidates []TagCandidate, newLabel string, category string) (*TagExtractionResult, error) {
	result := &TagExtractionResult{}

	// Process all merges
	for _, mergeJudgment := range judgment.Merges {
		mergeTarget := selectMergeTarget(candidates, mergeJudgment.Target, mergeJudgment.Label)
		if mergeTarget == nil {
			logging.Warnf("No suitable merge target found for label %q (target=%q), skipping", mergeJudgment.Label, mergeJudgment.Target)
			continue
		}

		var topSim float64
		for _, c := range candidates {
			if c.Tag != nil && c.Tag.ID == mergeTarget.ID {
				topSim = c.Similarity
				break
			}
		}
		if topSim > 0 && topSim < mergeMinSimilarity {
			logging.Warnf("Tag judgment: rejecting merge for %q — top candidate %q similarity %.4f < %.2f", newLabel, mergeTarget.Label, topSim, mergeMinSimilarity)
			continue
		}

		logging.Infof("Tag judgment: merge into existing tag %q (id=%d), label=%q", mergeTarget.Label, mergeTarget.ID, mergeJudgment.Label)

		if result.Merge == nil {
			result.Merge = &MergeResult{
				Target: mergeTarget,
				Label:  mergeJudgment.Label,
			}
		}

		for _, childLabel := range mergeJudgment.Children {
			for _, c := range candidates {
				if c.Tag != nil && c.Tag.Label == childLabel && c.Tag.ID != mergeTarget.ID {
					result.MergeChildren = append(result.MergeChildren, c.Tag)
				}
			}
		}
	}

	// Process all abstracts
	for _, abstractJudgment := range judgment.Abstracts {
		ensureNewLabelCandidateInAbstractJudgment(judgment, candidates, newLabel)
		abstractResult, err := processAbstractJudgment(ctx, candidates, &abstractJudgment, newLabel, category)
		if err != nil {
			if result.HasMerge() {
				logging.Warnf("Abstract judgment failed but merge succeeded, skipping: %v", err)
				continue
			}
			return nil, err
		}
		if abstractResult != nil {
			// Multiple abstracts are supported
			if result.Abstract == nil {
				result.Abstract = abstractResult
			} else {
				// Merge children into existing abstract result
				result.Abstract.Children = append(result.Abstract.Children, abstractResult.Children...)
			}
		}
	}

	if !result.HasAction() {
		result.LLMExplicitNone = true
		logging.Infof("Tag judgment: all candidates independent for %q", newLabel)
	}

	if len(judgment.None) > 0 {
		logging.Infof("Tag judgment: %d candidates classified as none for %q: %v", len(judgment.None), newLabel, judgment.None)
	}

	return result, nil
}

func processAbstractJudgment(ctx context.Context, candidates []TagCandidate, judgment *tagJudgmentAbstract, newLabel string, category string) (*AbstractResult, error) {
	abstractName := judgment.Name
	abstractDesc := judgment.Description
	newLabelIsCandidate := candidateLabelForNewLabel(candidates, newLabel) != ""

	slug := topictypes.Slugify(abstractName)
	if slug == "" {
		return nil, fmt.Errorf("generated empty slug for abstract name %q", abstractName)
	}

	candidateSlugs := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c.Tag != nil {
			candidateSlugs[c.Tag.Slug] = true
		}
	}

	if candidateSlugs[slug] {
		logging.Infof("Abstract name %q (slug=%s) collides with a candidate tag, skipping abstract creation", abstractName, slug)
		return nil, nil
	}

	var abstractTag *models.TopicTag
	if existingAbstract := findSimilarExistingAbstractFn(ctx, abstractName, abstractDesc, category, candidates); existingAbstract != nil {
		logging.Infof("processAbstractJudgment: reusing existing abstract tag %d (%q) instead of creating new %q",
			existingAbstract.ID, existingAbstract.Label, abstractName)
		abstractTag = existingAbstract
	}

	abstractChildSet := make(map[string]bool, len(judgment.Children))
	for _, ch := range judgment.Children {
		abstractChildSet[ch] = true
	}

	// Pre-validate candidate children for information gain (only for new abstracts, not reuse)
	if abstractTag == nil {
		var candidateChildren []*models.TopicTag
		for _, candidate := range candidates {
			if candidate.Tag == nil {
				continue
			}
			if abstractChildSet[candidate.Tag.Label] {
				candidateChildren = append(candidateChildren, candidate.Tag)
			}
		}
		if validateErr := validateAbstractCreation(database.DB, candidateChildren); validateErr != nil {
			logging.Infof("Abstract tag creation rejected for %q: %v", abstractName, validateErr)
			return nil, nil
		}
	}

	var createdNewAbstract bool
	var abstractChildren []*models.TopicTag

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if abstractTag == nil {
			var existing models.TopicTag
			if err := tx.Where("slug = ? AND category = ? AND status = ?", slug, category, "active").First(&existing).Error; err == nil {
				if existing.Kind == "abstract" || existing.Source == "abstract" {
					abstractTag = &existing
				} else {
					logging.Infof("processAbstractJudgment: slug match %q found non-abstract tag %d (%q, kind=%s), skipping reuse",
						slug, existing.ID, existing.Label, existing.Kind)
				}
			}
		}

		if abstractTag == nil {
			abstractTag = &models.TopicTag{
				Slug:        slug,
				Label:       abstractName,
				Category:    category,
				Kind:        category,
				Source:      "abstract",
				Status:      "active",
				Description: abstractDesc,
			}
			if err := tx.Create(abstractTag).Error; err != nil {
				return fmt.Errorf("create abstract tag: %w", err)
			}
			createdNewAbstract = true

			go func(tagID uint, name, cat string) {
				es := NewEmbeddingService()
				tag := &models.TopicTag{ID: tagID, Label: name, Category: cat}
				for _, embType := range []string{EmbeddingTypeIdentity, EmbeddingTypeSemantic} {
					emb, genErr := es.GenerateEmbedding(context.Background(), tag, embType)
					if genErr != nil {
						logging.Warnf("Failed to generate %s embedding for abstract tag %d: %v", embType, tagID, genErr)
						continue
					}
					emb.TopicTagID = tagID
					if saveErr := es.SaveEmbedding(emb); saveErr != nil {
						logging.Warnf("Failed to save %s embedding for abstract tag %d: %v", embType, tagID, saveErr)
					}
				}
				MatchAbstractTagHierarchy(context.Background(), tagID)
				EnqueueAdoptNarrower(tagID, "processAbstractJudgment")
			}(abstractTag.ID, abstractName, category)
		}

		for _, candidate := range candidates {
			if candidate.Tag == nil {
				continue
			}
			if candidate.Tag.ID == abstractTag.ID {
				continue
			}
			if !abstractChildSet[candidate.Tag.Label] {
				continue
			}

			wouldCycle, err := wouldCreateCycle(tx, abstractTag.ID, candidate.Tag.ID)
			if err != nil {
				return fmt.Errorf("check cycle for candidate %d: %w", candidate.Tag.ID, err)
			}
			if wouldCycle {
				logging.Warnf("Skipping cyclic relation: abstract tag %d -> candidate %d", abstractTag.ID, candidate.Tag.ID)
				continue
			}

			if err := checkDepthLimit(tx, abstractTag.ID, candidate.Tag.ID); err != nil {
				logging.Warnf("Skipping depth overflow relation: abstract %d -> candidate %d: %v", abstractTag.ID, candidate.Tag.ID, err)
				continue
			}

			var count int64
			tx.Model(&models.TopicTagRelation{}).
				Where("parent_id = ? AND child_id = ? AND relation_type = ?", abstractTag.ID, candidate.Tag.ID, "abstract").
				Count(&count)
			if count > 0 {
				abstractChildren = append(abstractChildren, candidate.Tag)
				continue
			}

			relation := models.TopicTagRelation{
				ParentID:        abstractTag.ID,
				ChildID:         candidate.Tag.ID,
				RelationType:    "abstract",
				SimilarityScore: candidate.Similarity,
			}
			if err := tx.Create(&relation).Error; err != nil {
				return fmt.Errorf("create tag relation: %w", err)
			}
			abstractChildren = append(abstractChildren, candidate.Tag)
		}

		minChildren := 1
		if newLabelIsCandidate {
			minChildren = 2
		}
		if len(abstractChildren) < minChildren {
			return errInsufficientAbstractChildren
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, errInsufficientAbstractChildren) {
			logging.Infof("Skipping abstract tag %q: only %d child relation(s) could be linked", abstractName, len(abstractChildren))
			return nil, nil
		}
		logging.Warnf("Abstract tag transaction failed: %v", err)
		return nil, err
	}

	logging.Infof("Abstract tag extracted: %s (id=%d) with children [%s]",
		abstractTag.Label, abstractTag.ID, strings.Join(judgment.Children, ", "))

	if len(abstractChildren) > 0 {
		if !createdNewAbstract && abstractTag.Source == "abstract" {
			go EnqueueAdoptNarrower(abstractTag.ID, "processAbstractJudgment_reuse")
		}
		go EnqueueAbstractTagUpdate(abstractTag.ID, "new_child_added")
		for _, child := range abstractChildren {
			go func(childID uint) {
				_, _ = resolveMultiParentConflict(childID)
			}(child.ID)
		}
	}

	return &AbstractResult{
		Tag:      abstractTag,
		Children: abstractChildren,
	}, nil
}

func organizeMatchCategory(requestCategory string, tag *models.TopicTag) string {
	if strings.TrimSpace(requestCategory) != "" {
		return strings.TrimSpace(requestCategory)
	}
	if tag != nil && strings.TrimSpace(tag.Category) != "" {
		return strings.TrimSpace(tag.Category)
	}
	return "keyword"
}

func shouldUseOrganizeCandidate(candidate TagCandidate, currentTagID uint, used map[uint]bool) bool {
	if candidate.Tag == nil {
		return false
	}
	if candidate.Tag.ID == currentTagID {
		return false
	}
	if candidate.Similarity < DefaultThresholds.LowSimilarity {
		return false
	}
	return !used[candidate.Tag.ID]
}

func collectOrganizeMergeSources(result *TagExtractionResult, currentTag *models.TopicTag) []*models.TopicTag {
	if result == nil || result.Merge == nil || result.Merge.Target == nil {
		return nil
	}

	sourceByID := make(map[uint]*models.TopicTag)
	if currentTag != nil && currentTag.ID != 0 && currentTag.ID != result.Merge.Target.ID {
		sourceByID[currentTag.ID] = currentTag
	}
	for _, child := range result.MergeChildren {
		if child == nil || child.ID == 0 || child.ID == result.Merge.Target.ID {
			continue
		}
		sourceByID[child.ID] = child
	}

	sources := make([]*models.TopicTag, 0, len(sourceByID))
	for _, source := range sourceByID {
		sources = append(sources, source)
	}
	return sources
}

func applyOrganizeMerge(result *TagExtractionResult, currentTag *models.TopicTag) []*models.TopicTag {
	sources := collectOrganizeMergeSources(result, currentTag)
	if len(sources) == 0 {
		return nil
	}

	merged := make([]*models.TopicTag, 0, len(sources))
	for _, source := range sources {
		if err := MergeTags(source.ID, result.Merge.Target.ID); err != nil {
			logging.Warnf("OrganizeUnclassifiedTags: merge %d (%s) into %d (%s) failed: %v",
				source.ID, source.Label, result.Merge.Target.ID, result.Merge.Target.Label, err)
			continue
		}
		merged = append(merged, source)
	}
	return merged
}

func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

func buildCandidateSummary(candidates []TagCandidate) []string {
	summaries := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Tag != nil {
			summaries = append(summaries, fmt.Sprintf("%s(id=%d,sim=%.2f,src=%s)", c.Tag.Label, c.Tag.ID, c.Similarity, c.Tag.Source))
		}
	}
	return summaries
}

// collectArticleIDsForTags batch-fetches article IDs associated with the given tag IDs.
// Returns a map from tag ID to a set of article IDs.
func collectArticleIDsForTags(db *gorm.DB, tagIDs []uint) (map[uint]map[uint]bool, error) {
	if len(tagIDs) == 0 {
		return make(map[uint]map[uint]bool), nil
	}

	var rows []struct {
		TopicTagID uint `gorm:"column:topic_tag_id"`
		ArticleID  uint `gorm:"column:article_id"`
	}
	if err := db.Model(&models.ArticleTopicTag{}).
		Where("topic_tag_id IN ?", tagIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("collect article IDs for tags: %w", err)
	}

	result := make(map[uint]map[uint]bool, len(tagIDs))
	for _, tagID := range tagIDs {
		result[tagID] = make(map[uint]bool)
	}
	for _, row := range rows {
		result[row.TopicTagID][row.ArticleID] = true
	}
	return result, nil
}

// jaccardSimilarity computes the Jaccard similarity coefficient between two article ID sets.
// Jaccard = |intersection| / |union|. Returns 0 if both sets are empty.
func jaccardSimilarity(setA, setB map[uint]bool) float64 {
	if len(setA) == 0 && len(setB) == 0 {
		return 0
	}

	intersection := 0
	for id := range setA {
		if setB[id] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// computeLeafToDepthRatio calculates the leaf-to-depth ratio that would result if the
// given candidate children were placed under a new abstract tag.
// Returns the ratio and an error.
func computeLeafToDepthRatio(tx *gorm.DB, childIDs []uint) (float64, error) {
	if len(childIDs) == 0 {
		return 0, nil
	}

	totalLeaves := 0
	maxDepth := 0

	for _, childID := range childIDs {
		leaves, depth := countLeavesAndDepth(tx, childID, 0)
		totalLeaves += leaves
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	// The new abstract adds one level of depth
	treeDepth := maxDepth + 1
	if treeDepth == 0 {
		return float64(totalLeaves), nil
	}
	return float64(totalLeaves) / float64(treeDepth), nil
}

// countLeavesAndDepth recursively counts leaf tags and max depth in a subtree.
func countLeavesAndDepth(tx *gorm.DB, tagID uint, currentDepth int) (leaves int, maxDepth int) {
	visited := make(map[uint]bool)
	return countLeavesAndDepthVisited(tx, tagID, currentDepth, visited)
}

func countLeavesAndDepthVisited(tx *gorm.DB, tagID uint, currentDepth int, visited map[uint]bool) (leaves int, maxDepth int) {
	if visited[tagID] {
		return 0, currentDepth
	}
	visited[tagID] = true

	var childIDs []uint
	tx.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND relation_type = ?", tagID, "abstract").
		Pluck("child_id", &childIDs)

	if len(childIDs) == 0 {
		return 1, currentDepth
	}

	totalLeaves := 0
	deepest := currentDepth
	for _, childID := range childIDs {
		l, d := countLeavesAndDepthVisited(tx, childID, currentDepth+1, visited)
		totalLeaves += l
		if d > deepest {
			deepest = d
		}
	}
	return totalLeaves, deepest
}

const (
	minAbstractChildren = 2
	maxPairwiseJaccard  = 0.7
	minLeafDepthRatio   = 1.5
)

// validateAbstractCreation checks whether creating a new abstract tag for the given
// candidate children meets information gain thresholds. Returns nil if acceptable,
// or an error describing why creation should be rejected.
func validateAbstractCreation(tx *gorm.DB, children []*models.TopicTag) error {
	if len(children) < minAbstractChildren {
		return fmt.Errorf("insufficient children: need at least %d, got %d", minAbstractChildren, len(children))
	}

	// Collect article IDs for all candidate children
	tagIDs := make([]uint, len(children))
	for i, child := range children {
		tagIDs[i] = child.ID
	}

	articleSets, err := collectArticleIDsForTags(tx, tagIDs)
	if err != nil {
		return fmt.Errorf("collect article IDs: %w", err)
	}

	// Check max pairwise Jaccard similarity
	for i := 0; i < len(children); i++ {
		for j := i + 1; j < len(children); j++ {
			jaccard := jaccardSimilarity(articleSets[children[i].ID], articleSets[children[j].ID])
			if jaccard > maxPairwiseJaccard {
				return fmt.Errorf("children %q (id=%d) and %q (id=%d) share too many articles (Jaccard=%.2f > %.2f), suggest merge instead",
					children[i].Label, children[i].ID,
					children[j].Label, children[j].ID,
					jaccard, maxPairwiseJaccard)
			}
		}
	}

	// Check leaf-to-depth ratio
	childIDs := make([]uint, len(children))
	for i, child := range children {
		childIDs[i] = child.ID
	}
	ratio, err := computeLeafToDepthRatio(tx, childIDs)
	if err != nil {
		return fmt.Errorf("compute leaf-to-depth ratio: %w", err)
	}
	if ratio < minLeafDepthRatio {
		return fmt.Errorf("degenerate tree: leaf-to-depth ratio %.2f < %.2f for %d children",
			ratio, minLeafDepthRatio, len(children))
	}

	return nil
}
