package tagging

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
	"my-robot-backend/internal/platform/ws"

	"gorm.io/gorm"
)

type TagHierarchyNode struct {
	ID              uint               `json:"id"`
	Label           string             `json:"label"`
	Slug            string             `json:"slug"`
	Category        string             `json:"category"`
	Icon            string             `json:"icon"`
	FeedCount       int                `json:"feed_count"`
	ArticleCount    int                `json:"article_count"`
	SimilarityScore float64            `json:"similarity_score,omitempty"`
	IsActive        bool               `json:"is_active"`
	QualityScore    float64            `json:"quality_score,omitempty"`
	IsLowQuality    bool               `json:"is_low_quality,omitempty"`
	Children        []TagHierarchyNode `json:"children"`
}

type OrganizeResult struct {
	TotalUnclassified int             `json:"total_unclassified"`
	Processed         int             `json:"processed"`
	Groups            []OrganizeGroup `json:"groups,omitempty"`
}

type OrganizeGroup struct {
	NewLabel       string `json:"new_label"`
	CandidateCount int    `json:"candidate_count"`
	Action         string `json:"action"`
}

func GetTagHierarchy(category string, scopeFeedID uint, scopeCategoryID uint, timeRange string) ([]TagHierarchyNode, error) {
	var scopeTagIDs map[uint]bool
	if scopeFeedID > 0 || scopeCategoryID > 0 {
		var err error
		scopeTagIDs, err = resolveScopeTagIDs(scopeFeedID, scopeCategoryID)
		if err != nil {
			return nil, err
		}
		if len(scopeTagIDs) == 0 {
			return []TagHierarchyNode{}, nil
		}
	}

	query := database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract")

	var relations []models.TopicTagRelation
	if err := query.Find(&relations).Error; err != nil {
		return nil, fmt.Errorf("query tag relations: %w", err)
	}

	if len(relations) == 0 {
		return []TagHierarchyNode{}, nil
	}

	if scopeTagIDs != nil {
		filtered := make([]models.TopicTagRelation, 0, len(relations))
		for _, r := range relations {
			if scopeTagIDs[r.ParentID] || scopeTagIDs[r.ChildID] {
				filtered = append(filtered, r)
			}
		}
		relations = filtered
		if len(relations) == 0 {
			return []TagHierarchyNode{}, nil
		}
	}

	tagIDSet := make(map[uint]bool)
	for _, r := range relations {
		tagIDSet[r.ParentID] = true
		tagIDSet[r.ChildID] = true
	}

	activeTagIDs := resolveActiveTagIDs(timeRange, tagIDSet)

	if timeRange != "" {
		propagateActiveAncestors(activeTagIDs, relations)
		prunedRelations := make([]models.TopicTagRelation, 0, len(relations))
		for _, r := range relations {
			if activeTagIDs[r.ChildID] {
				prunedRelations = append(prunedRelations, r)
			}
		}
		relations = prunedRelations
		if len(relations) == 0 {
			return []TagHierarchyNode{}, nil
		}
		tagIDSet = make(map[uint]bool)
		for _, r := range relations {
			tagIDSet[r.ParentID] = true
			tagIDSet[r.ChildID] = true
		}
	}

	tagIDs := make([]uint, 0, len(tagIDSet))
	for id := range tagIDSet {
		tagIDs = append(tagIDs, id)
	}

	var tags []models.TopicTag
	if err := database.DB.Where("id IN ? AND status = ?", tagIDs, "active").Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("load tags: %w", err)
	}
	tagMap := make(map[uint]*models.TopicTag, len(tags))
	for i := range tags {
		tagMap[tags[i].ID] = &tags[i]
	}

	articleCounts := countArticlesByTag(tagIDs, timeRange)

	if category != "" {
		filteredRelations := make([]models.TopicTagRelation, 0, len(relations))
		for _, r := range relations {
			parent, ok := tagMap[r.ParentID]
			if ok && parent.Category == category {
				filteredRelations = append(filteredRelations, r)
			}
		}
		relations = filteredRelations
	}

	childrenMap := make(map[uint][]TagHierarchyNode)
	parentSet := make(map[uint]bool)
	for _, r := range relations {
		child, ok := tagMap[r.ChildID]
		if !ok {
			continue
		}
		childrenMap[r.ParentID] = append(childrenMap[r.ParentID], TagHierarchyNode{
			ID:              child.ID,
			Label:           child.Label,
			Slug:            child.Slug,
			Category:        child.Category,
			Icon:            child.Icon,
			FeedCount:       child.FeedCount,
			ArticleCount:    articleCounts[child.ID],
			SimilarityScore: r.SimilarityScore,
			IsActive:        activeTagIDs[child.ID],
			QualityScore:    child.QualityScore,
			IsLowQuality:    child.Source != "abstract" && child.QualityScore < 0.3,
			Children:        []TagHierarchyNode{},
		})
		parentSet[r.ParentID] = true
	}

	childSet := make(map[uint]bool)
	for _, r := range relations {
		childSet[r.ChildID] = true
	}

	var roots []TagHierarchyNode
	for parentID := range parentSet {
		parent, ok := tagMap[parentID]
		if !ok {
			continue
		}
		if childSet[parentID] {
			continue
		}
		children := buildHierarchy(childrenMap, parentID)
		roots = append(roots, TagHierarchyNode{
			ID:           parent.ID,
			Label:        parent.Label,
			Slug:         parent.Slug,
			Category:     parent.Category,
			Icon:         parent.Icon,
			FeedCount:    parent.FeedCount,
			ArticleCount: articleCounts[parent.ID],
			IsActive:     activeTagIDs[parent.ID],
			QualityScore: parent.QualityScore,
			Children:     children,
		})
	}

	if len(parentSet) > 0 {
		childToParent := make(map[uint]uint, len(relations))
		for _, r := range relations {
			childToParent[r.ChildID] = r.ParentID
		}
		cycleRoots := make(map[uint]bool)
		globalVisited := make(map[uint]bool)
		for pid := range parentSet {
			if globalVisited[pid] {
				continue
			}
			path := make(map[uint]bool)
			current := pid
			for {
				if path[current] {
					cycleRoots[current] = true
					break
				}
				if globalVisited[current] {
					break
				}
				path[current] = true
				p, ok := childToParent[current]
				if !ok {
					break
				}
				current = p
			}
			for id := range path {
				globalVisited[id] = true
			}
		}
		existingRoots := make(map[uint]bool)
		for _, r := range roots {
			existingRoots[r.ID] = true
		}
		for rootID := range cycleRoots {
			if existingRoots[rootID] {
				continue
			}
			parent, ok := tagMap[rootID]
			if !ok {
				continue
			}
			children := buildHierarchy(childrenMap, rootID)
			roots = append(roots, TagHierarchyNode{
				ID:           parent.ID,
				Label:        parent.Label,
				Slug:         parent.Slug,
				Category:     parent.Category,
				Icon:         parent.Icon,
				FeedCount:    parent.FeedCount,
				ArticleCount: articleCounts[parent.ID],
				IsActive:     activeTagIDs[parent.ID],
				QualityScore: parent.QualityScore,
				Children:     children,
			})
		}
	}

	return roots, nil
}

func buildHierarchy(childrenMap map[uint][]TagHierarchyNode, parentID uint) []TagHierarchyNode {
	return buildHierarchyWithVisited(childrenMap, parentID, make(map[uint]bool))
}

func buildHierarchyWithVisited(childrenMap map[uint][]TagHierarchyNode, parentID uint, visited map[uint]bool) []TagHierarchyNode {
	if visited[parentID] {
		return []TagHierarchyNode{}
	}
	visited[parentID] = true
	defer delete(visited, parentID)

	children, ok := childrenMap[parentID]
	if !ok {
		return []TagHierarchyNode{}
	}
	result := make([]TagHierarchyNode, len(children))
	for i, child := range children {
		result[i] = child
		grandChildren := buildHierarchyWithVisited(childrenMap, child.ID, visited)
		result[i].Children = grandChildren
	}
	return result
}

func UpdateAbstractTagName(tagID uint, newName string) error {
	if tagID == 0 {
		return fmt.Errorf("tag ID must be > 0")
	}
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("new name must not be empty")
	}
	if len(newName) > maxAbstractNameLen {
		return fmt.Errorf("new name exceeds %d characters", maxAbstractNameLen)
	}

	var count int64
	database.DB.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND relation_type = ?", tagID, "abstract").
		Count(&count)
	if count == 0 {
		return fmt.Errorf("tag %d is not an abstract tag (no child relations)", tagID)
	}

	newSlug := Slugify(newName)
	if newSlug == "" {
		return fmt.Errorf("generated empty slug for name %q", newName)
	}

	var conflictCount int64
	database.DB.Model(&models.TopicTag{}).
		Where("slug = ? AND id != ? AND status = ?", newSlug, tagID, "active").
		Count(&conflictCount)
	if conflictCount > 0 {
		return fmt.Errorf("slug %q already in use by another active tag", newSlug)
	}

	if err := database.DB.Model(&models.TopicTag{}).
		Where("id = ?", tagID).
		Updates(map[string]interface{}{
			"label": newName,
			"slug":  newSlug,
		}).Error; err != nil {
		return fmt.Errorf("update abstract tag name: %w", err)
	}

	go func(tid uint) {
		es := NewEmbeddingService()
		var tag models.TopicTag
		if err := database.DB.First(&tag, tid).Error; err != nil {
			logging.Warnf("Failed to load tag %d for re-embedding: %v", tid, err)
			return
		}
		for _, embType := range []string{EmbeddingTypeIdentity, EmbeddingTypeSemantic} {
			emb, err := es.GenerateEmbedding(context.Background(), &tag, embType)
			if err != nil {
				logging.Warnf("Failed to generate %s embedding for renamed tag %d: %v", embType, tid, err)
				continue
			}
			emb.TopicTagID = tid
			if err := es.SaveEmbedding(emb); err != nil {
				logging.Warnf("Failed to save %s embedding for renamed tag %d: %v", embType, tid, err)
			}
		}
	}(tagID)

	return nil
}

func DetachChildTag(parentID, childID uint) error {
	if parentID == 0 || childID == 0 {
		return fmt.Errorf("parent_id and child_id must be > 0")
	}
	if parentID == childID {
		return fmt.Errorf("parent_id and child_id must be different")
	}

	result := database.DB.Where("parent_id = ? AND child_id = ?", parentID, childID).
		Delete(&models.TopicTagRelation{})
	if result.Error != nil {
		return fmt.Errorf("detach child tag: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no relation found between parent %d and child %d", parentID, childID)
	}

	return nil
}

func ReassignTagParent(tagID, newParentID uint) error {
	if tagID == 0 || newParentID == 0 {
		return fmt.Errorf("tag_id and new_parent_id must be > 0")
	}
	if tagID == newParentID {
		return fmt.Errorf("tag_id and new_parent_id must be different")
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		var parent models.TopicTag
		if err := tx.First(&parent, newParentID).Error; err != nil {
			return fmt.Errorf("parent tag not found: %w", err)
		}

		var childCount int64
		tx.Model(&models.TopicTagRelation{}).Where("parent_id = ?", tagID).Count(&childCount)
		if childCount > 0 {
			return fmt.Errorf("cannot reassign an abstract tag that has children")
		}

		wouldCycle, err := wouldCreateCycle(tx, newParentID, tagID)
		if err != nil {
			return fmt.Errorf("check cycle for reassignment: %w", err)
		}
		if wouldCycle {
			return fmt.Errorf("reassigning tag %d to parent %d would create a cycle", tagID, newParentID)
		}

		tx.Where("child_id = ?", tagID).Delete(&models.TopicTagRelation{})

		relation := models.TopicTagRelation{
			ParentID:     newParentID,
			ChildID:      tagID,
			RelationType: "abstract",
		}
		if err := tx.Create(&relation).Error; err != nil {
			return fmt.Errorf("create reassignment relation: %w", err)
		}

		return nil
	})
}

func GetUnclassifiedTags(category string, scopeFeedID uint, scopeCategoryID uint, timeRange string) ([]TagHierarchyNode, error) {
	var relatedTagIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("parent_id", &relatedTagIDs)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("child_id", &childIDs)
	relatedTagIDs = append(relatedTagIDs, childIDs...)
	relatedSet := make(map[uint]bool, len(relatedTagIDs))
	for _, id := range relatedTagIDs {
		relatedSet[id] = true
	}

	query := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND source != ?", "active", "abstract").
		Where("id IN (SELECT DISTINCT topic_tag_id FROM article_topic_tags)")

	if len(relatedSet) > 0 {
		allRelated := make([]uint, 0, len(relatedSet))
		for id := range relatedSet {
			allRelated = append(allRelated, id)
		}
		query = query.Where("id NOT IN ?", allRelated)
	}

	if category != "" {
		query = query.Where("category = ?", category)
	}

	if scopeFeedID > 0 || scopeCategoryID > 0 {
		scopeTagIDs, err := resolveScopeTagIDs(scopeFeedID, scopeCategoryID)
		if err != nil {
			return nil, err
		}
		if len(scopeTagIDs) == 0 {
			return []TagHierarchyNode{}, nil
		}
		scopeSlice := make([]uint, 0, len(scopeTagIDs))
		for id := range scopeTagIDs {
			scopeSlice = append(scopeSlice, id)
		}
		query = query.Where("id IN ?", scopeSlice)
	}

	var tags []models.TopicTag
	if err := query.Order("quality_score DESC, feed_count DESC, label ASC").Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("query unclassified tags: %w", err)
	}

	tagIDSet := make(map[uint]bool, len(tags))
	for _, tag := range tags {
		tagIDSet[tag.ID] = true
	}
	activeTagIDs := resolveActiveTagIDs(timeRange, tagIDSet)

	if timeRange != "" {
		filteredTags := make([]models.TopicTag, 0, len(tags))
		for _, tag := range tags {
			if activeTagIDs[tag.ID] {
				filteredTags = append(filteredTags, tag)
			}
		}
		tags = filteredTags
	}

	nodes := make([]TagHierarchyNode, 0, len(tags))
	for _, tag := range tags {
		nodes = append(nodes, TagHierarchyNode{
			ID:           tag.ID,
			Label:        tag.Label,
			Slug:         tag.Slug,
			Category:     tag.Category,
			Icon:         tag.Icon,
			FeedCount:    tag.FeedCount,
			IsActive:     true,
			QualityScore: tag.QualityScore,
			IsLowQuality: tag.Source != "abstract" && tag.QualityScore < 0.3,
			Children:     []TagHierarchyNode{},
		})
	}
	return nodes, nil
}

func OrganizeUnclassifiedTags(ctx context.Context, category string, _ int) (*OrganizeResult, error) {
	tags, err := GetUnclassifiedTags(category, 0, 0, "")
	if err != nil {
		return nil, fmt.Errorf("query unclassified tags: %w", err)
	}
	if len(tags) == 0 {
		return &OrganizeResult{TotalUnclassified: 0, Processed: 0}, nil
	}

	totalUnclassified := len(tags)

	es := NewEmbeddingService()

	tagModels := loadTagModels(tags)

	processed := 0
	var groups []OrganizeGroup
	used := make(map[uint]bool)

	broadcastOrganizeProgress("processing", totalUnclassified, processed, nil, 0, category)

	for i, tag := range tags {
		if used[tag.ID] {
			continue
		}

		if tagModels[tag.ID] == nil {
			continue
		}

		currentTag := tagModels[tag.ID]
		matchCategory := organizeMatchCategory(category, currentTag)

		similarCandidates, err := es.FindSimilarTags(ctx, currentTag, matchCategory, 5, EmbeddingTypeSemantic)
		if err != nil {
			logging.Warnf("OrganizeUnclassifiedTags: FindSimilarTags failed for tag %d: %v", tag.ID, err)
			continue
		}

		var groupCandidates []TagCandidate
		groupCandidates = append(groupCandidates, TagCandidate{
			Tag:        currentTag,
			Similarity: 1.0,
		})

		for _, sc := range similarCandidates {
			if !shouldUseOrganizeCandidate(sc, tag.ID, used) {
				continue
			}
			groupCandidates = append(groupCandidates, sc)
		}

		if len(groupCandidates) < 2 {
			continue
		}

		logging.Infof("OrganizeUnclassifiedTags: processing group for %q with %d candidates", tag.Label, len(groupCandidates))

		result, err := ExtractAbstractTag(ctx, groupCandidates, tag.Label, matchCategory, WithCaller("OrganizeUnclassifiedTags"))
		if err != nil {
			logging.Warnf("OrganizeUnclassifiedTags: ExtractAbstractTag failed for %q: %v", tag.Label, err)
			continue
		}

		action := "none"
		mergedSources := applyOrganizeMerge(result, currentTag)
		hasMergeAction := len(mergedSources) > 0
		hasAbstractAction := result != nil && result.HasAbstract()
		if hasMergeAction || hasAbstractAction {
			processed++
			switch {
			case hasMergeAction && hasAbstractAction:
				action = "merge+abstract"
			case hasMergeAction:
				action = "merge"
			case hasAbstractAction:
				action = "abstract"
			}

			if hasMergeAction {
				used[result.Merge.Target.ID] = true
				for _, c := range mergedSources {
					used[c.ID] = true
				}
			}
			if hasAbstractAction {
				used[result.Abstract.Tag.ID] = true
				for _, c := range result.Abstract.Children {
					used[c.ID] = true
				}
			}
		} else if result != nil && result.HasMerge() {
			logging.Infof("OrganizeUnclassifiedTags: merge judgment for %q had no mergeable sources", tag.Label)
		}

		g := OrganizeGroup{
			NewLabel:       tag.Label,
			CandidateCount: len(groupCandidates),
			Action:         action,
		}
		groups = append(groups, g)

		broadcastOrganizeProgress("processing", totalUnclassified, processed, &g, i+1, category)

		_ = i
	}

	broadcastOrganizeProgress("completed", totalUnclassified, processed, nil, 0, category)

	return &OrganizeResult{
		TotalUnclassified: totalUnclassified,
		Processed:         processed,
		Groups:            groups,
	}, nil
}

func broadcastOrganizeProgress(status string, total, processed int, currentGroup *OrganizeGroup, currentIndex int, category string) {
	hub := ws.GetHub()
	msg := ws.OrganizeProgressMessage{
		Type:              "organize_progress",
		Status:            status,
		TotalUnclassified: total,
		Processed:         processed,
		Category:          category,
	}
	if currentGroup != nil {
		msg.CurrentGroup = &ws.OrganizeGroupInfo{
			NewLabel:       currentGroup.NewLabel,
			CandidateCount: currentGroup.CandidateCount,
			Action:         currentGroup.Action,
		}
	}
	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("broadcastOrganizeProgress: marshal failed: %v", err)
		return
	}
	hub.BroadcastRaw(data)
}

func loadTagModels(nodes []TagHierarchyNode) map[uint]*models.TopicTag {
	result := make(map[uint]*models.TopicTag, len(nodes))
	for _, n := range nodes {
		var tag models.TopicTag
		if err := database.DB.First(&tag, n.ID).Error; err == nil {
			result[n.ID] = &tag
		}
	}
	return result
}

func propagateActiveAncestors(activeTagIDs map[uint]bool, relations []models.TopicTagRelation) {
	childToParents := make(map[uint][]uint)
	for _, r := range relations {
		childToParents[r.ChildID] = append(childToParents[r.ChildID], r.ParentID)
	}

	queue := make([]uint, 0, len(activeTagIDs))
	for id := range activeTagIDs {
		queue = append(queue, id)
	}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, parentID := range childToParents[id] {
			if !activeTagIDs[parentID] {
				activeTagIDs[parentID] = true
				queue = append(queue, parentID)
			}
		}
	}
}

func resolveActiveTagIDs(timeRange string, candidateIDs map[uint]bool) map[uint]bool {
	result := make(map[uint]bool, len(candidateIDs))

	if timeRange == "" {
		for id := range candidateIDs {
			result[id] = true
		}
		return result
	}

	var activeIDs []uint
	switch {
	case timeRange == "1d":
		since := time.Now().AddDate(0, 0, -1)
		database.DB.Model(&models.ArticleTopicTag{}).
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ?", since).
			Where("article_topic_tags.topic_tag_id IN ?", candidateIDSetToSlice(candidateIDs)).
			Pluck("DISTINCT article_topic_tags.topic_tag_id", &activeIDs)
	case timeRange == "7d":
		since := time.Now().AddDate(0, 0, -7)
		database.DB.Model(&models.ArticleTopicTag{}).
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ?", since).
			Where("article_topic_tags.topic_tag_id IN ?", candidateIDSetToSlice(candidateIDs)).
			Pluck("DISTINCT article_topic_tags.topic_tag_id", &activeIDs)
	case timeRange == "30d":
		since := time.Now().AddDate(0, 0, -30)
		database.DB.Model(&models.ArticleTopicTag{}).
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ?", since).
			Where("article_topic_tags.topic_tag_id IN ?", candidateIDSetToSlice(candidateIDs)).
			Pluck("DISTINCT article_topic_tags.topic_tag_id", &activeIDs)
	case strings.HasPrefix(timeRange, "custom:"):
		parts := strings.SplitN(timeRange, ":", 3)
		if len(parts) != 3 {
			for id := range candidateIDs {
				result[id] = true
			}
			return result
		}
		startDate := parts[1]
		endDate := parts[2]
		if _, err := time.Parse("2006-01-02", startDate); err != nil {
			for id := range candidateIDs {
				result[id] = true
			}
			return result
		}
		if _, err := time.Parse("2006-01-02", endDate); err != nil {
			for id := range candidateIDs {
				result[id] = true
			}
			return result
		}
		database.DB.Model(&models.ArticleTopicTag{}).
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ? AND articles.pub_date <= ?", startDate+" 00:00:00", endDate+" 23:59:59").
			Where("article_topic_tags.topic_tag_id IN ?", candidateIDSetToSlice(candidateIDs)).
			Pluck("DISTINCT article_topic_tags.topic_tag_id", &activeIDs)
	default:
		for id := range candidateIDs {
			result[id] = true
		}
		return result
	}

	for _, id := range activeIDs {
		result[id] = true
	}
	return result
}

func candidateIDSetToSlice(m map[uint]bool) []uint {
	result := make([]uint, 0, len(m))
	for id := range m {
		result = append(result, id)
	}
	return result
}

func countArticlesByTag(tagIDs []uint, timeRange string) map[uint]int {
	result := make(map[uint]int)
	if len(tagIDs) == 0 {
		return result
	}

	query := database.DB.Model(&models.ArticleTopicTag{}).
		Select("topic_tag_id, count(*) as cnt").
		Where("topic_tag_id IN ?", tagIDs)

	switch {
	case timeRange == "1d":
		since := time.Now().AddDate(0, 0, -1)
		query = query.Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ?", since)
	case timeRange == "7d":
		since := time.Now().AddDate(0, 0, -7)
		query = query.Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ?", since)
	case timeRange == "30d":
		since := time.Now().AddDate(0, 0, -30)
		query = query.Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.pub_date >= ?", since)
	case strings.HasPrefix(timeRange, "custom:"):
		parts := strings.SplitN(timeRange, ":", 3)
		if len(parts) == 3 {
			if _, err1 := time.Parse("2006-01-02", parts[1]); err1 == nil {
				if _, err2 := time.Parse("2006-01-02", parts[2]); err2 == nil {
					query = query.Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
						Where("articles.pub_date >= ? AND articles.pub_date <= ?", parts[1]+" 00:00:00", parts[2]+" 23:59:59")
				}
			}
		}
	}

	var rows []struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	query.Group("topic_tag_id").Scan(&rows)

	for _, row := range rows {
		result[row.TopicTagID] = row.Cnt
	}
	return result
}

func resolveScopeTagIDs(feedID uint, categoryID uint) (map[uint]bool, error) {
	if feedID == 0 && categoryID == 0 {
		return nil, nil
	}

	query := database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT topic_tag_id")

	if feedID > 0 {
		query = query.Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.feed_id = ?", feedID)
	} else if categoryID > 0 {
		query = query.Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Joins("JOIN feeds ON feeds.id = articles.feed_id").
			Where("feeds.category_id = ?", categoryID)
	}

	var tagIDs []uint
	if err := query.Pluck("topic_tag_id", &tagIDs).Error; err != nil {
		return nil, fmt.Errorf("resolve scope tag IDs: %w", err)
	}

	result := make(map[uint]bool, len(tagIDs))
	for _, id := range tagIDs {
		result[id] = true
	}
	return result, nil
}

func enqueueEmbeddingsForNormalChildren(parentID uint) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("enqueueEmbeddingsForNormalChildren panic: %v", r)
		}
	}()

	var normalChildIDs []uint
	database.DB.Model(&models.TopicTag{}).
		Joins("JOIN topic_tag_relations ON topic_tag_relations.child_id = topic_tags.id").
		Where("topic_tag_relations.parent_id = ? AND topic_tag_relations.relation_type = ? AND topic_tags.source != ?",
			parentID, "abstract", "abstract").
		Pluck("topic_tags.id", &normalChildIDs)

	if len(normalChildIDs) == 0 {
		return
	}

	var existingEmbTagIDs []uint
	database.DB.Model(&models.TopicTagEmbedding{}).
		Where("topic_tag_id IN ?", normalChildIDs).
		Pluck("topic_tag_id", &existingEmbTagIDs)
	existingSet := make(map[uint]bool, len(existingEmbTagIDs))
	for _, id := range existingEmbTagIDs {
		existingSet[id] = true
	}

	qs := NewEmbeddingQueueService(nil)
	for _, id := range normalChildIDs {
		if !existingSet[id] {
			if err := qs.Enqueue(id); err != nil {
				logging.Warnf("Failed to enqueue embedding for normal child %d under abstract %d: %v", id, parentID, err)
			}
		}
	}
}
