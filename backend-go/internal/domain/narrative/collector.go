package narrative

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

type TagInput struct {
	ID           uint   `json:"id"`
	Label        string `json:"label"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	ArticleCount int    `json:"article_count"`
	IsAbstract   bool   `json:"is_abstract"`
	Source       string `json:"source"`
	ParentLabel  string `json:"parent_label,omitempty"`
	IsWatched    bool   `json:"is_watched,omitempty"`
}

type AbstractTreeNode struct {
	ID           uint               `json:"id"`
	Label        string             `json:"label"`
	Category     string             `json:"category"`
	Description  string             `json:"description"`
	ArticleCount int                `json:"article_count"`
	IsAbstract   bool               `json:"is_abstract"`
	Children     []AbstractTreeNode `json:"children,omitempty"`
}

type PreviousNarrative struct {
	ID         uint64 `json:"id"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
	Generation int    `json:"generation"`
}

func CollectTagInputs(date time.Time) ([]TagInput, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var inputs []TagInput

	abstractTreeTags, err := collectAbstractTreeTags(startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("collect abstract tree tags: %w", err)
	}
	inputs = append(inputs, abstractTreeTags...)

	unclassifiedTags, err := collectUnclassifiedTags(startOfDay, endOfDay)
	if err != nil {
		return nil, fmt.Errorf("collect unclassified tags: %w", err)
	}
	inputs = append(inputs, unclassifiedTags...)

	return inputs, nil
}

func collectAbstractTreeTags(since, until time.Time) ([]TagInput, error) {
	type relationRow struct {
		ParentID uint
		ChildID  uint
	}
	var relations []relationRow
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Select("parent_id, child_id").
		Scan(&relations)

	if len(relations) == 0 {
		return nil, nil
	}

	tagIDSet := make(map[uint]bool)
	parentOf := make(map[uint]uint)
	for _, r := range relations {
		tagIDSet[r.ParentID] = true
		tagIDSet[r.ChildID] = true
		parentOf[r.ChildID] = r.ParentID
	}

	allIDs := make([]uint, 0, len(tagIDSet))
	for id := range tagIDSet {
		allIDs = append(allIDs, id)
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ?", allIDs, "active").Find(&tags)
	if len(tags) == 0 {
		return nil, nil
	}

	tagMap := make(map[uint]models.TopicTag, len(tags))
	for _, t := range tags {
		tagMap[t.ID] = t
	}

	tagIDs := make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagIDs, since, until).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	var inputs []TagInput
	for _, tag := range tags {
		parentLabel := ""
		if pid, ok := parentOf[tag.ID]; ok {
			if p, found := tagMap[pid]; found {
				parentLabel = p.Label
			}
		}

		inputs = append(inputs, TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: countMap[tag.ID],
			IsAbstract:   tag.Source == "abstract",
			Source:       tag.Source,
			ParentLabel:  parentLabel,
		})
	}
	return inputs, nil
}

func collectUnclassifiedTags(since, until time.Time) ([]TagInput, error) {
	var allRelated []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("parent_id", &allRelated)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Pluck("child_id", &childIDs)
	allRelated = append(allRelated, childIDs...)
	relatedSet := make(map[uint]bool, len(allRelated))
	for _, id := range allRelated {
		relatedSet[id] = true
	}

	activeSubquery := database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.topic_tag_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.pub_date >= ? AND articles.pub_date < ?", since, until)

	baseQuery := database.DB.Model(&models.TopicTag{}).
		Where("status = ? AND source != ?", "active", "abstract").
		Where("id IN (?)", activeSubquery)

	if len(relatedSet) > 0 {
		excl := make([]uint, 0, len(relatedSet))
		for id := range relatedSet {
			excl = append(excl, id)
		}
		baseQuery = baseQuery.Where("id NOT IN ?", excl)
	}

	var watchedTags []models.TopicTag
	watchedQ := baseQuery.Session(&gorm.Session{}).
		Where("is_watched = ?", true).
		Order("quality_score DESC, feed_count DESC")
	watchedQ.Find(&watchedTags)

	var topTags []models.TopicTag
	topQ := baseQuery.Session(&gorm.Session{}).
		Where("is_watched = ?", false).
		Order("quality_score DESC, feed_count DESC").
		Limit(10)
	topQ.Find(&topTags)

	tags := watchedTags
	tags = append(tags, topTags...)
	if len(tags) == 0 {
		return nil, nil
	}

	tagIDs := make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", tagIDs, since, until).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	var inputs []TagInput
	for _, tag := range tags {
		inputs = append(inputs, TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: countMap[tag.ID],
			IsAbstract:   false,
			Source:       tag.Source,
			IsWatched:    tag.IsWatched,
		})
	}
	return inputs, nil
}

type CategoryNarrativeBrief struct {
	ID          uint       `json:"id"`
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	RelatedTags []TagBrief `json:"related_tags"`
}

type CategoryInput struct {
	CategoryID   uint                     `json:"category_id"`
	CategoryName string                   `json:"category_name"`
	CategoryIcon string                   `json:"category_icon"`
	Narratives   []CategoryNarrativeBrief `json:"narratives"`
}

type ActiveCategory struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Icon         string `json:"icon"`
	Color        string `json:"color"`
	ArticleCount int    `json:"article_count"`
	TagCount     int    `json:"tag_count"`
}

func CollectActiveCategories(date time.Time) ([]ActiveCategory, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var categories []models.Category
	database.DB.Find(&categories)

	var result []ActiveCategory
	for _, cat := range categories {
		var feedIDs []uint
		database.DB.Model(&models.Feed{}).Where("category_id = ?", cat.ID).Pluck("id", &feedIDs)
		if len(feedIDs) == 0 {
			continue
		}

		var articleCount int64
		database.DB.Model(&models.Article{}).
			Where("feed_id IN ? AND pub_date >= ? AND pub_date < ?", feedIDs, startOfDay, endOfDay).
			Count(&articleCount)

		if articleCount == 0 {
			continue
		}

		var tagCount int64
		database.DB.Model(&models.ArticleTopicTag{}).
			Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
			Where("articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", feedIDs, startOfDay, endOfDay).
			Distinct("article_topic_tags.topic_tag_id").
			Count(&tagCount)

		result = append(result, ActiveCategory{
			ID:           cat.ID,
			Name:         cat.Name,
			Icon:         cat.Icon,
			Color:        cat.Color,
			ArticleCount: int(articleCount),
			TagCount:     int(tagCount),
		})
	}
	return result, nil
}

func CollectPreviousNarratives(date time.Time, scopeType string, categoryID *uint) ([]PreviousNarrative, error) {
	yesterday := date.AddDate(0, 0, -1)
	query := database.DB.
		Where("period = ? AND period_date >= ? AND period_date < ?", "daily", yesterday, date)

	if scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
		if categoryID != nil {
			query = query.Where("scope_category_id = ?", *categoryID)
		}
	}

	var narratives []models.NarrativeSummary
	if err := query.Order("id ASC").Find(&narratives).Error; err != nil {
		return nil, err
	}

	var result []PreviousNarrative
	for _, n := range narratives {
		result = append(result, PreviousNarrative{
			ID:         uint64(n.ID),
			Title:      n.Title,
			Summary:    n.Summary,
			Status:     n.Status,
			Generation: n.Generation,
		})
	}
	return result, nil
}

func CollectCategoryNarrativeSummaries(date time.Time) ([]CategoryInput, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var narratives []models.NarrativeSummary
	if err := database.DB.
		Where("scope_type = ? AND status != ? AND period_date >= ? AND period_date < ?",
			models.NarrativeScopeTypeFeedCategory, models.NarrativeStatusEnding, startOfDay, endOfDay).
		Order("generation DESC, id DESC").
		Find(&narratives).Error; err != nil {
		return nil, fmt.Errorf("query category narratives: %w", err)
	}

	if len(narratives) == 0 {
		return nil, nil
	}

	grouped := make(map[uint][]models.NarrativeSummary)
	for _, n := range narratives {
		if n.ScopeCategoryID != nil {
			grouped[*n.ScopeCategoryID] = append(grouped[*n.ScopeCategoryID], n)
		}
	}

	for catID, ns := range grouped {
		if len(ns) > 5 {
			grouped[catID] = ns[:5]
		}
	}

	type catWithCount struct {
		CategoryID   uint
		Narratives   []models.NarrativeSummary
		ArticleCount int
	}

	var buckets []catWithCount
	for catID, ns := range grouped {
		totalArticles := 0
		for _, n := range ns {
			var ids []interface{}
			if n.RelatedArticleIDs != "" {
				_ = json.Unmarshal([]byte(n.RelatedArticleIDs), &ids)
			}
			totalArticles += len(ids)
		}
		buckets = append(buckets, catWithCount{
			CategoryID:   catID,
			Narratives:   ns,
			ArticleCount: totalArticles,
		})
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].ArticleCount > buckets[j].ArticleCount
	})

	totalCap := 30
	var selected []catWithCount
	total := 0
	for _, b := range buckets {
		if total >= totalCap {
			break
		}
		take := b.Narratives
		if total+len(take) > totalCap {
			take = take[:totalCap-total]
		}
		selected = append(selected, catWithCount{
			CategoryID:   b.CategoryID,
			Narratives:   take,
			ArticleCount: b.ArticleCount,
		})
		total += len(take)
	}

	catIDs := make([]uint, len(selected))
	for i, s := range selected {
		catIDs[i] = s.CategoryID
	}

	var categories []models.Category
	if len(catIDs) > 0 {
		database.DB.Where("id IN ?", catIDs).Find(&categories)
	}
	catMap := make(map[uint]models.Category, len(categories))
	for _, c := range categories {
		catMap[c.ID] = c
	}

	tagIDSet := make(map[uint]bool)
	for _, b := range selected {
		for _, n := range b.Narratives {
			var tagIDs []uint
			if n.RelatedTagIDs != "" {
				_ = json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
			}
			for _, id := range tagIDs {
				tagIDSet[id] = true
			}
		}
	}

	tagBriefMap := make(map[uint]TagBrief)
	if len(tagIDSet) > 0 {
		tagIDs := make([]uint, 0, len(tagIDSet))
		for id := range tagIDSet {
			tagIDs = append(tagIDs, id)
		}
		var tags []models.TopicTag
		database.DB.Where("id IN ?", tagIDs).Find(&tags)
		for _, t := range tags {
			tagBriefMap[t.ID] = TagBrief{ID: t.ID, Slug: t.Slug, Label: t.Label, Category: t.Category, Kind: t.Kind}
		}
	}

	var result []CategoryInput
	for _, b := range selected {
		cat, ok := catMap[b.CategoryID]
		if !ok {
			continue
		}

		briefs := make([]CategoryNarrativeBrief, 0, len(b.Narratives))
		for _, n := range b.Narratives {
			var tagIDs []uint
			if n.RelatedTagIDs != "" {
				_ = json.Unmarshal([]byte(n.RelatedTagIDs), &tagIDs)
			}
			relatedTags := make([]TagBrief, 0, len(tagIDs))
			for _, tid := range tagIDs {
				if brief, ok := tagBriefMap[tid]; ok {
					relatedTags = append(relatedTags, brief)
				}
			}

			briefs = append(briefs, CategoryNarrativeBrief{
				ID:          uint(n.ID),
				Title:       n.Title,
				Summary:     n.Summary,
				RelatedTags: relatedTags,
			})
		}

		result = append(result, CategoryInput{
			CategoryID:   b.CategoryID,
			CategoryName: cat.Name,
			CategoryIcon: cat.Icon,
			Narratives:   briefs,
		})
	}

	logging.Infof("narrative: collected %d category inputs with %d total narratives for %s",
		len(result), total, date.Format("2006-01-02"))

	return result, nil
}

func CollectAbstractTreeInputsByCategory(date time.Time, categoryID uint) ([]AbstractTreeNode, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var feedIDs []uint
	if err := database.DB.Model(&models.Feed{}).
		Where("category_id = ?", categoryID).
		Pluck("id", &feedIDs).Error; err != nil || len(feedIDs) == 0 {
		return nil, nil
	}

	var tagIDs []uint
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.topic_tag_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", feedIDs, startOfDay, endOfDay).
		Pluck("article_topic_tags.topic_tag_id", &tagIDs)

	if len(tagIDs) == 0 {
		return nil, nil
	}

	var relations []models.TopicTagRelation
	database.DB.Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)",
		"abstract", tagIDs, tagIDs).Find(&relations)

	if len(relations) == 0 {
		return nil, nil
	}

	allIDs := make(map[uint]bool)
	parentOf := make(map[uint]uint)
	childrenOf := make(map[uint][]uint)
	for _, r := range relations {
		allIDs[r.ParentID] = true
		allIDs[r.ChildID] = true
		parentOf[r.ChildID] = r.ParentID
		childrenOf[r.ParentID] = append(childrenOf[r.ParentID], r.ChildID)
	}

	var allTagIDs []uint
	for id := range allIDs {
		allTagIDs = append(allTagIDs, id)
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ?", allTagIDs, "active").Find(&tags)
	tagMap := make(map[uint]models.TopicTag, len(tags))
	for _, t := range tags {
		tagMap[t.ID] = t
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?",
			allTagIDs, feedIDs, startOfDay, endOfDay).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	visited := make(map[uint]bool)
	var roots []uint
	for id := range allIDs {
		if _, hasParent := parentOf[id]; !hasParent {
			roots = append(roots, id)
		}
	}

	var result []AbstractTreeNode
	for _, rootID := range roots {
		if visited[rootID] {
			continue
		}
		tree := buildTree(rootID, tagMap, countMap, childrenOf, visited)
		if tree != nil {
			result = append(result, *tree)
		}
	}

	return result, nil
}

func buildTree(id uint, tagMap map[uint]models.TopicTag, countMap map[uint]int, childrenOf map[uint][]uint, visited map[uint]bool) *AbstractTreeNode {
	if visited[id] {
		return nil
	}
	visited[id] = true

	tag, ok := tagMap[id]
	if !ok {
		return nil
	}

	node := &AbstractTreeNode{
		ID:           tag.ID,
		Label:        tag.Label,
		Category:     tag.Category,
		Description:  tag.Description,
		ArticleCount: countMap[tag.ID],
		IsAbstract:   tag.Source == "abstract",
	}

	for _, childID := range childrenOf[id] {
		child := buildTree(childID, tagMap, countMap, childrenOf, visited)
		if child != nil {
			node.Children = append(node.Children, *child)
		}
	}

	return node
}

func CountTreeNodes(tree AbstractTreeNode) int {
	count := 1
	for _, child := range tree.Children {
		count += CountTreeNodes(child)
	}
	return count
}

func CollectUnclassifiedEventTagsByCategory(date time.Time, categoryID uint) ([]TagInput, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var feedIDs []uint
	if err := database.DB.Model(&models.Feed{}).
		Where("category_id = ?", categoryID).
		Pluck("id", &feedIDs).Error; err != nil || len(feedIDs) == 0 {
		return nil, nil
	}

	var tagIDs []uint
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("DISTINCT article_topic_tags.topic_tag_id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", feedIDs, startOfDay, endOfDay).
		Pluck("article_topic_tags.topic_tag_id", &tagIDs)

	if len(tagIDs) == 0 {
		return nil, nil
	}

	var relatedIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)", "abstract", tagIDs, tagIDs).
		Pluck("parent_id", &relatedIDs)
	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("relation_type = ? AND (parent_id IN ? OR child_id IN ?)", "abstract", tagIDs, tagIDs).
		Pluck("child_id", &childIDs)
	relatedIDs = append(relatedIDs, childIDs...)
	relatedSet := make(map[uint]bool, len(relatedIDs))
	for _, id := range relatedIDs {
		relatedSet[id] = true
	}

	var tags []models.TopicTag
	database.DB.Where("id IN ? AND status = ? AND category = ? AND source != ?",
		tagIDs, "active", "event", "abstract").
		Order("quality_score DESC, feed_count DESC").
		Limit(50).
		Find(&tags)

	if len(tags) == 0 {
		return nil, nil
	}

	var filtered []models.TopicTag
	for _, t := range tags {
		if !relatedSet[t.ID] {
			filtered = append(filtered, t)
		}
	}
	tags = filtered

	if len(tags) == 0 {
		return nil, nil
	}

	tagIDs = make([]uint, len(tags))
	for i, t := range tags {
		tagIDs[i] = t.ID
	}

	type countRow struct {
		TopicTagID uint `json:"topic_tag_id"`
		Cnt        int  `json:"cnt"`
	}
	var counts []countRow
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.feed_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?",
			tagIDs, feedIDs, startOfDay, endOfDay).
		Group("article_topic_tags.topic_tag_id").
		Scan(&counts)

	countMap := make(map[uint]int, len(counts))
	for _, c := range counts {
		countMap[c.TopicTagID] = c.Cnt
	}

	var inputs []TagInput
	for _, tag := range tags {
		inputs = append(inputs, TagInput{
			ID:           tag.ID,
			Label:        tag.Label,
			Category:     tag.Category,
			Description:  tag.Description,
			ArticleCount: countMap[tag.ID],
			Source:       tag.Source,
		})
	}
	return inputs, nil
}
