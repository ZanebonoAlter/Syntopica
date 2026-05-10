package topicanalysis

import (
	"context"
	"fmt"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

const (
	PlacementL2HighThreshold = 0.85
	PlacementL2LowThreshold  = 0.60
	PlacementL1HighThreshold = 0.80
	PlacementL1LowThreshold  = 0.55
	PlacementCandidateLimit  = 15
)

type PlacementResult struct {
	TagID          uint
	ParentID       *uint
	ParentLabel    string
	CreatedParents []uint
	Action         string
}

func PlaceTagInHierarchy(ctx context.Context, tag *models.TopicTag) (*PlacementResult, error) {
	tmpl := GetHierarchyManager().GetTemplate(tag.Category, "")
	if tmpl == nil {
		return nil, fmt.Errorf("no hierarchy template for category=%s", tag.Category)
	}

	leafLevel := tmpl.GetLeafLevel()
	tagLevel := GetTagLevel(tag)

	if tagLevel >= leafLevel {
		return &PlacementResult{TagID: tag.ID, Action: "already_leaf"}, nil
	}

	return placeTagUpward(ctx, tag, tmpl, tagLevel+1)
}

func placeTagUpward(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, targetLevel int) (*PlacementResult, error) {
	result := &PlacementResult{TagID: tag.ID}

	switch targetLevel {
	case 2:
		return placeTagAtL2(ctx, tag, tmpl, result)
	case 1:
		return placeTagAtL1(ctx, tag, tmpl, result)
	default:
		return result, fmt.Errorf("unsupported target level %d", targetLevel)
	}
}

func placeTagAtL2(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, result *PlacementResult) (*PlacementResult, error) {
	es := NewEmbeddingService()

	candidates, err := es.FindSimilarAbstractTags(ctx, tag.ID, tag.Category, PlacementCandidateLimit)
	if err != nil {
		return result, fmt.Errorf("find L2 candidates: %w", err)
	}

	parentID, _, err := resolveL2Parent(ctx, tag, candidates, tmpl)
	if err != nil {
		return result, err
	}

	if parentID != 0 {
		if err := database.DB.Create(&models.TopicTagRelation{
			ParentID:     parentID,
			ChildID:      tag.ID,
			RelationType: "abstract",
		}).Error; err != nil {
			return result, fmt.Errorf("create L2 relation: %w", err)
		}
		result.ParentID = &parentID
		result.Action = "placed_at_L2"

		if tmpl.MaxLevel > 2 {
			var parentTag models.TopicTag
			if err := database.DB.First(&parentTag, parentID).Error; err == nil {
				l1Result, err := placeTagAtL1ForParent(ctx, &parentTag, tmpl, result)
				if err != nil {
					logging.Warnf("Failed to place L2 tag %d at L1: %v", parentID, err)
				}
				return l1Result, err
			}
		}
	}

	return result, nil
}

func placeTagAtL1(ctx context.Context, tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, result *PlacementResult) (*PlacementResult, error) {
	return placeTagAtL1ForParent(ctx, tag, tmpl, result)
}

func placeTagAtL1ForParent(ctx context.Context, l2Tag *models.TopicTag, tmpl *CategoryHierarchyTemplate, result *PlacementResult) (*PlacementResult, error) {
	currentLevel := GetTagLevel(l2Tag)
	if currentLevel >= 1 {
		return result, nil
	}

	existingL1s, err := loadExistingL1Tags(l2Tag.Category, tmpl)
	if err != nil {
		return result, fmt.Errorf("load existing L1 tags: %w", err)
	}

	es := NewEmbeddingService()

	candidates, err := es.FindSimilarAbstractTags(ctx, l2Tag.ID, l2Tag.Category, PlacementCandidateLimit)
	if err != nil {
		return result, fmt.Errorf("find L1 candidates for L2 tag: %w", err)
	}

	var l1Candidates []TagCandidate
	for _, c := range candidates {
		if isL1Tag(c.Tag, tmpl) {
			l1Candidates = append(l1Candidates, c)
		}
	}

	parentID, _, err := resolveL1Parent(ctx, l2Tag, l1Candidates, existingL1s, tmpl)
	if err != nil {
		return result, fmt.Errorf("resolve L1 parent: %w", err)
	}

	if parentID != 0 {
		if err := database.DB.Create(&models.TopicTagRelation{
			ParentID:     parentID,
			ChildID:      l2Tag.ID,
			RelationType: "abstract",
		}).Error; err != nil {
			return result, fmt.Errorf("create L1 relation: %w", err)
		}
		result.CreatedParents = append(result.CreatedParents, parentID)
		result.Action = "placed_at_L1"
	}

	return result, nil
}

func resolveL2Parent(ctx context.Context, tag *models.TopicTag, candidates []TagCandidate, tmpl *CategoryHierarchyTemplate) (parentID uint, parentLabel string, err error) {
	if len(candidates) == 0 {
		return createL2TagForChild(ctx, tag, tmpl)
	}

	top := candidates[0]
	if top.Similarity >= PlacementL2HighThreshold {
		logging.Infof("L2 placement: tag=%d label=%s similarity=%.4f >= %.2f, direct attach to %d(%s)",
			tag.ID, tag.Label, top.Similarity, PlacementL2HighThreshold, top.Tag.ID, top.Tag.Label)
		return top.Tag.ID, top.Tag.Label, nil
	}

	if top.Similarity < PlacementL2LowThreshold {
		logging.Infof("L2 placement: tag=%d label=%s similarity=%.4f < %.2f, creating new L2",
			tag.ID, tag.Label, top.Similarity, PlacementL2LowThreshold)
		return createL2TagForChild(ctx, tag, tmpl)
	}

	logging.Infof("L2 placement: tag=%d label=%s similarity=%.4f in [%.2f, %.2f], LLM judgment needed",
		tag.ID, tag.Label, top.Similarity, PlacementL2LowThreshold, PlacementL2HighThreshold)

	selected, err := callLLMForL2Match(ctx, tag, filterL2Candidates(candidates))
	if err != nil {
		logging.Warnf("LLM L2 match failed for tag=%d, falling back to create: %v", tag.ID, err)
		return createL2TagForChild(ctx, tag, tmpl)
	}

	if selected != nil {
		return selected.ID, selected.Label, nil
	}

	return createL2TagForChild(ctx, tag, tmpl)
}

func resolveL1Parent(ctx context.Context, l2Tag *models.TopicTag, candidates []TagCandidate, existingL1s []*models.TopicTag, tmpl *CategoryHierarchyTemplate) (parentID uint, parentLabel string, err error) {
	if len(candidates) == 0 {
		return createL1ForL2Tag(ctx, l2Tag, existingL1s, tmpl)
	}

	top := candidates[0]
	if top.Similarity >= PlacementL1HighThreshold {
		logging.Infof("L1 placement: l2=%d label=%s similarity=%.4f >= %.2f, direct attach to %d(%s)",
			l2Tag.ID, l2Tag.Label, top.Similarity, PlacementL1HighThreshold, top.Tag.ID, top.Tag.Label)
		return top.Tag.ID, top.Tag.Label, nil
	}

	if top.Similarity < PlacementL1LowThreshold {
		logging.Infof("L1 placement: l2=%d label=%s similarity=%.4f < %.2f, creating new L1",
			l2Tag.ID, l2Tag.Label, top.Similarity, PlacementL1LowThreshold)
		return createL1ForL2Tag(ctx, l2Tag, existingL1s, tmpl)
	}

	logging.Infof("L1 placement: l2=%d label=%s similarity=%.4f in [%.2f, %.2f], LLM judgment needed",
		l2Tag.ID, l2Tag.Label, top.Similarity, PlacementL1LowThreshold, PlacementL1HighThreshold)

	selected, err := callLLMForL1Match(ctx, l2Tag, filterL1Candidates(candidates), existingL1s, tmpl)
	if err != nil {
		logging.Warnf("LLM L1 match failed for l2=%d, falling back to create: %v", l2Tag.ID, err)
		return createL1ForL2Tag(ctx, l2Tag, existingL1s, tmpl)
	}

	if selected != nil {
		return selected.ID, selected.Label, nil
	}

	return createL1ForL2Tag(ctx, l2Tag, existingL1s, tmpl)
}

func createL2TagForChild(ctx context.Context, childTag *models.TopicTag, tmpl *CategoryHierarchyTemplate) (uint, string, error) {
	l2Label, l2Desc, err := callLLMForL2Creation(ctx, childTag, tmpl)
	if err != nil {
		return 0, "", fmt.Errorf("LLM create L2: %w", err)
	}

	parentID, parentLabel, err := createAbstractTag(ctx, l2Label, l2Desc, childTag.Category, childTag.ID)
	if err != nil {
		return 0, "", err
	}

	var newTag models.TopicTag
	if err := database.DB.First(&newTag, parentID).Error; err == nil {
		go dedupL2(context.Background(), &newTag)
	}

	return parentID, parentLabel, nil
}

func createL1ForL2Tag(ctx context.Context, l2Tag *models.TopicTag, existingL1s []*models.TopicTag, tmpl *CategoryHierarchyTemplate) (uint, string, error) {
	l1Label, l1Desc, err := callLLMForL1Creation(ctx, l2Tag, existingL1s, tmpl)
	if err != nil {
		return 0, "", fmt.Errorf("LLM create L1: %w", err)
	}

	parentID, parentLabel, err := createAbstractTag(ctx, l1Label, l1Desc, l2Tag.Category, l2Tag.ID)
	if err != nil {
		return 0, "", err
	}

	var newTag models.TopicTag
	if err := database.DB.First(&newTag, parentID).Error; err == nil {
		go dedupL1(context.Background(), &newTag)
	}

	return parentID, parentLabel, nil
}

func createAbstractTag(ctx context.Context, label, description, category string, childID uint) (uint, string, error) {
	tag := models.TopicTag{
		Label:       label,
		Category:    category,
		Source:      "abstract",
		Description: description,
		Status:      "active",
		IsCanonical: true,
	}

	if err := database.DB.Create(&tag).Error; err != nil {
		return 0, "", fmt.Errorf("create abstract tag: %w", err)
	}

	if childID != 0 {
		relation := models.TopicTagRelation{
			ParentID:     tag.ID,
			ChildID:      childID,
			RelationType: "abstract",
		}
		if err := database.DB.Create(&relation).Error; err != nil {
			logging.Warnf("Failed to link abstract tag %d to child %d: %v", tag.ID, childID, err)
		}
	}

	go func() {
		es := NewEmbeddingService()
		bgCtx := context.Background()
		_, err := es.GenerateEmbedding(bgCtx, &tag, EmbeddingTypeIdentity)
		if err != nil {
			logging.Warnf("Failed to generate identity embedding for abstract tag %d: %v", tag.ID, err)
		}
		_, err = es.GenerateEmbedding(bgCtx, &tag, EmbeddingTypeSemantic)
		if err != nil {
			logging.Warnf("Failed to generate semantic embedding for abstract tag %d: %v", tag.ID, err)
		}
	}()

	logging.Infof("Created abstract tag: %d label=%s category=%s", tag.ID, tag.Label, tag.Category)
	return tag.ID, tag.Label, nil
}

func loadExistingL1Tags(category string, tmpl *CategoryHierarchyTemplate) ([]*models.TopicTag, error) {
	var tags []models.TopicTag
	if err := database.DB.Where("category = ? AND source = ? AND status = 'active'", category, "abstract").Find(&tags).Error; err != nil {
		return nil, err
	}

	var l1Tags []*models.TopicTag
	for i := range tags {
		if isL1Tag(&tags[i], tmpl) {
			l1Tags = append(l1Tags, &tags[i])
		}
	}
	return l1Tags, nil
}

func isL1Tag(tag *models.TopicTag, tmpl *CategoryHierarchyTemplate) bool {
	if tag.Source != "abstract" {
		return false
	}
	depth := getTagDepthFromRoot(tag.ID)
	return depth == 0
}

func filterL2Candidates(candidates []TagCandidate) []TagCandidate {
	var filtered []TagCandidate
	for _, c := range candidates {
		if c.Tag != nil && c.Tag.Source == "abstract" {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func filterL1Candidates(candidates []TagCandidate) []TagCandidate {
	return filterL2Candidates(candidates)
}
