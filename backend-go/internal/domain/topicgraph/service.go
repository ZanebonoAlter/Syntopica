package topicgraph

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topictypes"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

func BuildTopicGraph(kind string, anchor time.Time, categoryID, feedID *uint) (*topictypes.TopicGraphResponse, error) {
	windowStart, windowEnd, periodLabel, err := topictypes.ResolveWindow(kind, anchor)
	if err != nil {
		return nil, err
	}

	articleTags, err := fetchArticleTagsData(windowStart, windowEnd, categoryID, feedID)
	if err != nil {
		return nil, err
	}

	nodes, edges, topTopics, articleCount := buildGraphPayloadFromArticles(database.DB, articleTags)
	feedCount := 0
	for _, node := range nodes {
		if node.Kind == "feed" {
			feedCount++
		}
	}

	return &topictypes.TopicGraphResponse{
		Type:         kind,
		AnchorDate:   windowStart.Format("2006-01-02"),
		PeriodLabel:  periodLabel,
		Nodes:        nodes,
		Edges:        edges,
		TopicCount:   len(topTopics),
		ArticleCount: articleCount,
		FeedCount:    feedCount,
		TopTopics:    topTopics,
	}, nil
}

func BuildTopicDetail(kind string, slug string, anchor time.Time, categoryID, feedID *uint) (*topictypes.TopicDetail, error) {
	windowStart, windowEnd, _, err := topictypes.ResolveWindow(kind, anchor)
	if err != nil {
		return nil, err
	}

	var topic models.TopicTag
	err = database.DB.Where("slug = ?", slug).First(&topic).Error
	if err != nil {
		topic = models.TopicTag{
			ID:       0,
			Slug:     slug,
			Label:    toTitle(slug),
			Category: models.TagCategoryKeyword,
		}
	}

	tagIDs := collectAllChildTagIDs(topic.ID)
	ids := make([]uint, 0, len(tagIDs))
	for id := range tagIDs {
		ids = append(ids, id)
	}

	articles, total, err := getTopicArticles(ids, windowStart, windowEnd, 1, 15, categoryID, feedID)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic articles: %w", err)
	}

	relatedTags, err := getRelatedTags(topic.ID, 20)
	if err != nil {
		logging.Warnf("Warning: failed to get related tags: %v", err)
	}

	canonical := topictypes.TopicTag{
		ID:          topic.ID,
		Label:       topic.Label,
		Slug:        topic.Slug,
		Category:    topictypes.NormalizeDisplayCategory(topic.Kind, topic.Category),
		Icon:        topic.Icon,
		Kind:        topictypes.NormalizeTopicKind(topic.Kind, topic.Category),
		Description: topic.Description,
		Score:       0,
	}

	history, err := buildTopicHistory(kind, slug, anchor, categoryID, feedID)
	if err != nil {
		return nil, err
	}

	related := buildRelatedTopicsFromTags(relatedTags, 8)

	return &topictypes.TopicDetail{
		Topic:         canonical,
		Articles:      articles,
		TotalArticles: total,
		RelatedTags:   relatedTags,
		History:       history,
		RelatedTopics: related,
		SearchLinks: map[string]string{
			"youtube_videos": "https://www.youtube.com/results?search_query=" + url.QueryEscape(canonical.Label),
			"youtube_live":   "https://www.youtube.com/results?search_query=" + url.QueryEscape(canonical.Label+" live"),
		},
		AppLinks: map[string]string{
			"digest_view": "/digest/" + kind,
			"topic_graph": "/topics",
		},
	}, nil
}

// getTopicArticles retrieves articles associated with one or more topic tags
func getTopicArticles(tagIDs []uint, startDate, endDate time.Time, page, pageSize int, categoryID, feedID *uint) ([]topictypes.TopicArticleCard, int64, error) {
	if len(tagIDs) == 0 {
		return []topictypes.TopicArticleCard{}, 0, nil
	}

	var articles []models.Article
	var total int64

	offset := (page - 1) * pageSize

	base := database.DB.Model(&models.Article{}).
		Joins("JOIN article_topic_tags ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ?", tagIDs).
		Where("articles.created_at >= ? AND articles.created_at < ?", startDate, endDate)

	countQuery := base
	dataQuery := database.DB.Model(&models.Article{}).
		Joins("JOIN article_topic_tags ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ?", tagIDs).
		Where("articles.created_at >= ? AND articles.created_at < ?", startDate, endDate)

	if feedID != nil {
		countQuery = countQuery.Where("articles.feed_id = ?", *feedID)
		dataQuery = dataQuery.Where("articles.feed_id = ?", *feedID)
	} else if categoryID != nil {
		countQuery = countQuery.Joins("JOIN feeds ON feeds.id = articles.feed_id").Where("feeds.category_id = ?", *categoryID)
		dataQuery = dataQuery.Joins("JOIN feeds ON feeds.id = articles.feed_id").Where("feeds.category_id = ?", *categoryID)
	}

	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count articles: %w", err)
	}

	err := dataQuery.
		Preload("Feed").
		Order("articles.created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Omit("tag_count", "relevance_score").
		Find(&articles).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query articles: %w", err)
	}

	// Batch fetch tags for all returned articles
	articleIDs := make([]uint, 0, len(articles))
	for _, a := range articles {
		articleIDs = append(articleIDs, a.ID)
	}

	tagMap := make(map[uint][]topictypes.TopicTagSummary)
	if len(articleIDs) > 0 {
		type tagRow struct {
			ArticleID uint
			Slug      string
			Label     string
			Category  string
		}
		var rows []tagRow
		dbErr := database.DB.Raw(`
			SELECT att.article_id, tt.slug, tt.label, tt.category
			FROM article_topic_tags att
			JOIN topic_tags tt ON att.topic_tag_id = tt.id
			WHERE att.article_id IN ?
		`, articleIDs).Scan(&rows).Error
		if dbErr != nil {
			return nil, 0, fmt.Errorf("failed to fetch article tags: %w", dbErr)
		}
		for _, r := range rows {
			tagMap[r.ArticleID] = append(tagMap[r.ArticleID], topictypes.TopicTagSummary{
				Slug:     r.Slug,
				Label:    r.Label,
				Category: r.Category,
			})
		}
	}

	// Convert to cards
	cards := make([]topictypes.TopicArticleCard, 0, len(articles))
	for _, article := range articles {
		card := topictypes.TopicArticleCard{
			ID:       article.ID,
			Title:    article.Title,
			Link:     article.Link,
			FeedID:   article.FeedID,
			ImageURL: article.ImageURL,
			Summary:  article.AIContentSummary,
			Content:  article.Content,
		}

		if article.PubDate != nil {
			card.PubDate = article.PubDate
		}

		if article.Feed.ID != 0 {
			card.FeedName = article.Feed.Title
		}

		if t, ok := tagMap[article.ID]; ok {
			card.Tags = t
		} else {
			card.Tags = []topictypes.TopicTagSummary{}
		}

		cards = append(cards, card)
	}

	return cards, total, nil
}

// getRelatedTags retrieves tags that co-occur with the given topic
func getRelatedTags(topicID uint, limit int) ([]topictypes.RelatedTag, error) {
	var relatedTags []topictypes.RelatedTag

	// Query tags that co-occur with the current topic in articles
	// Ordered by co-occurrence count
	err := database.DB.Raw(`
		SELECT 
			t.id,
			t.label,
			t.slug,
			t.category,
			t.kind,
			COUNT(*) as cooccurrence
		FROM topic_tags t
		JOIN article_topic_tags at1 ON t.id = at1.topic_tag_id
		JOIN article_topic_tags at2 ON at1.article_id = at2.article_id
		WHERE at2.topic_tag_id = ?
		  AND t.id != ?
		GROUP BY t.id, t.label, t.slug, t.category
		ORDER BY cooccurrence DESC
		LIMIT ?
	`, topicID, topicID, limit).Scan(&relatedTags).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get related tags: %w", err)
	}

	for i := range relatedTags {
		relatedTags[i].Category = topictypes.NormalizeDisplayCategory(relatedTags[i].Kind, relatedTags[i].Category)
		relatedTags[i].Kind = topictypes.NormalizeTopicKind(relatedTags[i].Kind, relatedTags[i].Category)
	}

	return relatedTags, nil
}

func buildRelatedTopicsFromTags(relatedTags []topictypes.RelatedTag, limit int) []topictypes.TopicTag {
	result := make([]topictypes.TopicTag, 0, len(relatedTags))
	for _, rt := range relatedTags {
		result = append(result, topictypes.TopicTag{
			ID:       rt.ID,
			Label:    rt.Label,
			Slug:     rt.Slug,
			Category: rt.Category,
			Kind:     rt.Kind,
			Score:    float64(rt.Cooccurrence),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score == result[j].Score {
			return result[i].Label < result[j].Label
		}
		return result[i].Score > result[j].Score
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// FetchTopicArticles is the public API for fetching topic articles with pagination
func FetchTopicArticles(slug string, kind string, anchor time.Time, page, pageSize int) ([]topictypes.TopicArticleCard, int64, error) {
	windowStart, windowEnd, _, err := topictypes.ResolveWindow(kind, anchor)
	if err != nil {
		return nil, 0, err
	}

	// Get topic
	var topic models.TopicTag
	err = database.DB.Where("slug = ?", slug).First(&topic).Error
	if err != nil {
		return nil, 0, fmt.Errorf("topic not found: %w", err)
	}

	tagIDs := collectAllChildTagIDs(topic.ID)
	ids := make([]uint, 0, len(tagIDs))
	for id := range tagIDs {
		ids = append(ids, id)
	}

	return getTopicArticles(ids, windowStart, windowEnd, page, pageSize, nil, nil)
}

func buildTopicHistory(kind string, slug string, anchor time.Time, categoryID, feedID *uint) ([]topictypes.TopicHistoryPoint, error) {
	history := make([]topictypes.TopicHistoryPoint, 0, 7)
	for i := 6; i >= 0; i-- {
		var pointAnchor time.Time
		if kind == "weekly" {
			pointAnchor = anchor.AddDate(0, 0, -7*i)
		} else {
			pointAnchor = anchor.AddDate(0, 0, -i)
		}

		start, end, label, err := topictypes.ResolveWindow(kind, pointAnchor)
		if err != nil {
			return nil, err
		}

		articleTags, err := fetchArticleTagsData(start, end, categoryID, feedID)
		if err != nil {
			return nil, err
		}

		count := 0
		articleSet := make(map[uint]bool)
		for _, at := range articleTags {
			if at.TopicTag != nil && at.TopicTag.Slug == slug {
				articleSet[at.ArticleID] = true
			}
		}
		count = len(articleSet)

		history = append(history, topictypes.TopicHistoryPoint{
			AnchorDate: start.Format("2006-01-02"),
			Count:      count,
			Label:      label,
		})
	}

	return history, nil
}

// GetCategoryColor returns the color for a given category
func GetCategoryColor(category string) string {
	switch category {
	case "event":
		return "#f59e0b" // amber
	case "person":
		return "#10b981" // emerald
	case "keyword":
		return "#6366f1" // indigo
	default:
		return "#6366f1" // default to indigo for unknown categories
	}
}

// BuildTopicsByCategory builds topic lists grouped by category from article tags
// Only includes tags extracted by LLM (not heuristic feed/category names)
func BuildTopicsByCategory(kind string, anchor time.Time, categoryID, feedID *uint) (*topictypes.TopicsByCategoryResult, error) {
	windowStart, windowEnd, _, err := topictypes.ResolveWindow(kind, anchor)
	if err != nil {
		return nil, err
	}

	// Get articles from the time window with their LLM-extracted tags
	// Filter by source='llm' to exclude heuristic tags (feed names, category names)
	var articleTags []models.ArticleTopicTag
	query := database.DB.
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Joins("JOIN topic_tags ON topic_tags.id = article_topic_tags.topic_tag_id").
		Where("articles.created_at >= ? AND articles.created_at < ?", windowStart, windowEnd).
		Where("article_topic_tags.source = ?", "llm")

	if feedID != nil {
		query = query.Where("articles.feed_id = ?", *feedID)
	} else if categoryID != nil {
		query = query.Joins("JOIN feeds ON feeds.id = articles.feed_id").
			Where("feeds.category_id = ?", *categoryID)
	}

	err = query.Preload("TopicTag").
		Find(&articleTags).Error
	if err != nil {
		return nil, err
	}

	// Group tags by category and aggregate scores
	eventScores := make(map[string]*topictypes.TopicTag)
	personScores := make(map[string]*topictypes.TopicTag)
	keywordScores := make(map[string]*topictypes.TopicTag)

	for _, at := range articleTags {
		if at.TopicTag == nil {
			continue
		}

		tag := topictypes.TopicTag{
			ID:           at.TopicTag.ID,
			Label:        at.TopicTag.Label,
			Slug:         at.TopicTag.Slug,
			Category:     topictypes.NormalizeDisplayCategory(at.TopicTag.Kind, at.TopicTag.Category),
			Icon:         at.TopicTag.Icon,
			Kind:         topictypes.NormalizeTopicKind(at.TopicTag.Kind, at.TopicTag.Category),
			Description:  at.TopicTag.Description,
			Score:        at.Score,
			QualityScore: at.TopicTag.QualityScore,
			IsLowQuality: at.TopicTag.Source != "abstract" && at.TopicTag.QualityScore < 0.3,
		}

		switch tag.Category {
		case models.TagCategoryEvent:
			if existing, ok := eventScores[tag.Slug]; ok {
				existing.Score += tag.Score
			} else {
				eventScores[tag.Slug] = &tag
			}
		case models.TagCategoryPerson:
			if existing, ok := personScores[tag.Slug]; ok {
				existing.Score += tag.Score
			} else {
				personScores[tag.Slug] = &tag
			}
		default: // keyword
			if existing, ok := keywordScores[tag.Slug]; ok {
				existing.Score += tag.Score
			} else {
				keywordScores[tag.Slug] = &tag
			}
		}
	}

	// Include abstract parent tags that have no direct article_topic_tags associations
	includeAbstractParents(database.DB, eventScores, personScores, keywordScores)

	enrichAbstractTags(database.DB, eventScores, personScores, keywordScores)
	finalizeTopicTagQuality(eventScores, personScores, keywordScores)

	result := &topictypes.TopicsByCategoryResult{
		Events:   sortTagsByScoreMap(eventScores),
		People:   sortTagsByScoreMap(personScores),
		Keywords: sortTagsByScoreMap(keywordScores),
	}

	return result, nil
}

// sortTagsByScoreMap converts a map of tags to a sorted slice
func sortTagsByScoreMap(tagMap map[string]*topictypes.TopicTag) []topictypes.TopicTag {
	result := make([]topictypes.TopicTag, 0, len(tagMap))
	for _, tag := range tagMap {
		result = append(result, *tag)
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].QualityScore == result[j].QualityScore {
			if result[i].Score == result[j].Score {
				return result[i].Label < result[j].Label
			}
			return result[i].Score > result[j].Score
		}
		return result[i].QualityScore > result[j].QualityScore
	})

	return result
}

func finalizeTopicTagQuality(tagMaps ...map[string]*topictypes.TopicTag) {
	for _, tagMap := range tagMaps {
		for _, tag := range tagMap {
			tag.IsLowQuality = !tag.IsAbstract && tag.QualityScore < 0.3
		}
	}
}

// ArticleTagData represents aggregated data from article_topic_tags for graph building
type ArticleTagData struct {
	ArticleID uint
	FeedID    uint
	FeedTitle string
	FeedColor string
	TopicTag  *models.TopicTag
	Score     float64
}

// fetchArticleTagsData retrieves article-topic associations with feed info for graph building
func fetchArticleTagsData(start, end time.Time, categoryID, feedID *uint) ([]ArticleTagData, error) {
	var articleTags []models.ArticleTopicTag
	query := database.DB.
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Joins("JOIN topic_tags ON topic_tags.id = article_topic_tags.topic_tag_id").
		Where("articles.created_at >= ? AND articles.created_at < ?", start, end).
		Where("article_topic_tags.source = ?", "llm")

	if feedID != nil {
		query = query.Where("articles.feed_id = ?", *feedID)
	} else if categoryID != nil {
		query = query.Joins("JOIN feeds ON feeds.id = articles.feed_id").
			Where("feeds.category_id = ?", *categoryID)
	}

	err := query.
		Preload("TopicTag").
		Preload("Article.Feed").
		Find(&articleTags).Error
	if err != nil {
		return nil, err
	}

	data := make([]ArticleTagData, 0, len(articleTags))
	for _, at := range articleTags {
		if at.TopicTag == nil || at.Article == nil {
			continue
		}
		feedTitle := "未知订阅源"
		feedColor := "#3b6b87"
		if at.Article.Feed.ID != 0 {
			if strings.TrimSpace(at.Article.Feed.Title) != "" {
				feedTitle = at.Article.Feed.Title
			}
			if strings.TrimSpace(at.Article.Feed.Color) != "" {
				feedColor = at.Article.Feed.Color
			}
		}
		data = append(data, ArticleTagData{
			ArticleID: at.ArticleID,
			FeedID:    at.Article.FeedID,
			FeedTitle: feedTitle,
			FeedColor: feedColor,
			TopicTag:  at.TopicTag,
			Score:     at.Score,
		})
	}

	return data, nil
}

// buildGraphPayloadFromArticles builds graph nodes and edges from article tag data
func buildGraphPayloadFromArticles(db *gorm.DB, data []ArticleTagData) ([]topictypes.GraphNode, []topictypes.GraphEdge, []topictypes.TopicTag, int) {
	topicNodes := map[string]*topictypes.GraphNode{}
	feedNodes := map[string]*topictypes.GraphNode{}
	edgeMap := map[string]*topictypes.GraphEdge{}
	topicScores := map[string]topictypes.TopicTag{}
	articleSet := make(map[uint]bool)

	for _, item := range data {
		articleSet[item.ArticleID] = true

		feedNodeID := fmt.Sprintf("feed-%d", item.FeedID)
		if _, exists := feedNodes[feedNodeID]; !exists {
			feedNodes[feedNodeID] = &topictypes.GraphNode{
				ID:       feedNodeID,
				Label:    item.FeedTitle,
				Kind:     "feed",
				Weight:   1,
				Color:    item.FeedColor,
				FeedName: item.FeedTitle,
			}
		}
		feedNodes[feedNodeID].Weight += 0.35

		topicSlug := item.TopicTag.Slug
		topicLabel := item.TopicTag.Label
		topicCategory := topictypes.NormalizeDisplayCategory(item.TopicTag.Kind, item.TopicTag.Category)

		if _, exists := topicNodes[topicSlug]; !exists {
			topicNodes[topicSlug] = &topictypes.GraphNode{
				ID:           topicSlug,
				Label:        topicLabel,
				Slug:         topicSlug,
				Kind:         "topic",
				Category:     topicCategory,
				Icon:         item.TopicTag.Icon,
				Color:        GetCategoryColor(topicCategory),
				Weight:       0,
				ArticleCount: 0,
			}
		}
		topicNodes[topicSlug].Weight += item.Score
		topicNodes[topicSlug].ArticleCount++

		merged := topicScores[topicSlug]
		if merged.Label == "" || merged.Score < item.Score {
			topicScores[topicSlug] = topictypes.TopicTag{
				ID:           item.TopicTag.ID,
				Label:        topicLabel,
				Slug:         topicSlug,
				Category:     topicCategory,
				Icon:         item.TopicTag.Icon,
				Kind:         topictypes.NormalizeTopicKind(item.TopicTag.Kind, item.TopicTag.Category),
				Score:        item.Score,
				QualityScore: item.TopicTag.QualityScore,
				IsLowQuality: item.TopicTag.Source != "abstract" && item.TopicTag.QualityScore < 0.3,
				Description:  item.TopicTag.Description,
			}
		}

		edgeKey := topicSlug + "::" + feedNodeID
		if _, exists := edgeMap[edgeKey]; !exists {
			edgeMap[edgeKey] = &topictypes.GraphEdge{ID: edgeKey, Source: topicSlug, Target: feedNodeID, Kind: "topic_feed", Weight: 0}
		}
		edgeMap[edgeKey].Weight += item.Score
	}

	// Build topic-topic edges from co-occurrence in same article
	articleTopics := make(map[uint][]string)
	for _, item := range data {
		articleTopics[item.ArticleID] = append(articleTopics[item.ArticleID], item.TopicTag.Slug)
	}
	for _, slugs := range articleTopics {
		for i := 0; i < len(slugs); i++ {
			for j := i + 1; j < len(slugs); j++ {
				if slugs[i] == slugs[j] {
					continue
				}
				left, right := slugs[i], slugs[j]
				if left > right {
					left, right = right, left
				}
				edgeKey := left + "::" + right
				if _, exists := edgeMap[edgeKey]; !exists {
					edgeMap[edgeKey] = &topictypes.GraphEdge{ID: edgeKey, Source: left, Target: right, Kind: "topic_topic", Weight: 0}
				}
				edgeMap[edgeKey].Weight += 0.5
			}
		}
	}

	// Identify abstract tags (parent tags in topic_tag_relations)
	findAbstractSlugs(db, topicNodes)

	nodes := make([]topictypes.GraphNode, 0, len(topicNodes)+len(feedNodes))
	for _, node := range topicNodes {
		nodes = append(nodes, *node)
	}
	for _, node := range feedNodes {
		nodes = append(nodes, *node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Weight == nodes[j].Weight {
			return nodes[i].Label < nodes[j].Label
		}
		return nodes[i].Weight > nodes[j].Weight
	})

	edges := make([]topictypes.GraphEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, *edge)
	}
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].Weight > edges[j].Weight })

	topTopics := make([]topictypes.TopicTag, 0, len(topicScores))
	for _, topic := range topicScores {
		topic.Score = topicNodes[topic.Slug].Weight
		topTopics = append(topTopics, topic)
	}
	markTopicTagsQuality(topTopics)
	sort.SliceStable(topTopics, func(i, j int) bool {
		if topTopics[i].QualityScore == topTopics[j].QualityScore {
			if topTopics[i].Score == topTopics[j].Score {
				return topTopics[i].Label < topTopics[j].Label
			}
			return topTopics[i].Score > topTopics[j].Score
		}
		return topTopics[i].QualityScore > topTopics[j].QualityScore
	})

	return nodes, edges, topTopics, len(articleSet)
}

func markTopicTagsQuality(tags []topictypes.TopicTag) {
	for i := range tags {
		tags[i].IsLowQuality = !tags[i].IsAbstract && tags[i].QualityScore < 0.3
	}
}

// GetPendingArticlesByTag retrieves articles that have the given tag but are not in any digest
func GetPendingArticlesByTag(tagSlug string, kind string, anchor time.Time) (*topictypes.PendingArticlesResponse, error) {
	windowStart, windowEnd, _, err := topictypes.ResolveWindow(kind, anchor)
	if err != nil {
		return nil, err
	}

	var topicTag models.TopicTag
	err = database.DB.Where("slug = ?", tagSlug).First(&topicTag).Error
	if err != nil {
		return nil, fmt.Errorf("topic tag not found: %w", err)
	}

	tagIDSet := collectAllChildTagIDs(topicTag.ID)
	tagIDs := make([]uint, 0, len(tagIDSet))
	for id := range tagIDSet {
		tagIDs = append(tagIDs, id)
	}

	var taggedArticles []models.Article
	err = database.DB.
		Joins("JOIN article_topic_tags ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ?", tagIDs).
		Where("articles.created_at >= ? AND articles.created_at < ?", windowStart, windowEnd).
		Preload("Feed").
		Omit("tag_count", "relevance_score").
		Distinct().
		Find(&taggedArticles).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tagged articles: %w", err)
	}

	pendingArticles := make([]topictypes.PendingArticle, 0, len(taggedArticles))
	for _, article := range taggedArticles {
		pa := topictypes.PendingArticle{
			ID:    article.ID,
			Title: article.Title,
			Link:  article.Link,
		}

		if article.PubDate != nil {
			pa.PubDate = article.PubDate.In(topictypes.TopicGraphCST).Format(time.RFC3339)
		}

		if article.Feed.ID != 0 {
			pa.FeedName = article.Feed.Title
			pa.FeedIcon = article.Feed.Icon
			pa.FeedColor = article.Feed.Color
		} else {
			pa.FeedName = "未知订阅源"
		}

		pendingArticles = append(pendingArticles, pa)
	}

	return &topictypes.PendingArticlesResponse{
		Articles: pendingArticles,
		Total:    len(pendingArticles),
	}, nil
}

// findAbstractSlugs queries topic_tag_relations to identify which tag slugs are abstract parents.
// It annotates matching nodes in the topicNodes map with IsAbstract=true.
func findAbstractSlugs(db *gorm.DB, topicNodes map[string]*topictypes.GraphNode) {
	var abstractParentIDs []uint
	db.Model(&models.TopicTagRelation{}).
		Select("DISTINCT parent_id").
		Pluck("parent_id", &abstractParentIDs)

	if len(abstractParentIDs) == 0 {
		return
	}

	var parentTags []models.TopicTag
	db.Where("id IN ?", abstractParentIDs).Find(&parentTags)

	abstractSlugs := make(map[string]bool, len(parentTags))
	for _, t := range parentTags {
		abstractSlugs[t.Slug] = true
	}

	for slug, node := range topicNodes {
		if abstractSlugs[slug] {
			node.IsAbstract = true
		}
	}
}

// includeAbstractParents adds abstract parent tags that have no direct source='llm'
// article_topic_tags associations into the category maps, so enrichAbstractTags can
// set IsAbstract=true and ChildSlugs on them.
func includeAbstractParents(db *gorm.DB, tagMaps ...map[string]*topictypes.TopicTag) {
	var parentIDs []uint
	db.Model(&models.TopicTagRelation{}).
		Where("relation_type = ?", "abstract").
		Select("DISTINCT parent_id").Pluck("parent_id", &parentIDs)
	if len(parentIDs) == 0 {
		return
	}

	for _, pid := range parentIDs {
		var pt models.TopicTag
		if err := db.First(&pt, pid).Error; err != nil {
			continue
		}
		if pt.Status != "" && pt.Status != "active" {
			continue
		}

		cat := topictypes.NormalizeDisplayCategory(pt.Kind, pt.Category)

		var targetMap map[string]*topictypes.TopicTag
		switch cat {
		case "event":
			if len(tagMaps) > 0 {
				targetMap = tagMaps[0]
			}
		case "person":
			if len(tagMaps) > 1 {
				targetMap = tagMaps[1]
			}
		default:
			if len(tagMaps) > 2 {
				targetMap = tagMaps[2]
			}
		}
		if targetMap == nil {
			continue
		}

		if _, exists := targetMap[pt.Slug]; !exists {
			targetMap[pt.Slug] = &topictypes.TopicTag{
				ID:           pt.ID,
				Label:        pt.Label,
				Slug:         pt.Slug,
				Category:     cat,
				Kind:         topictypes.NormalizeTopicKind(pt.Kind, pt.Category),
				Icon:         pt.Icon,
				Description:  pt.Description,
				Score:        0,
				QualityScore: pt.QualityScore,
				IsLowQuality: pt.Source != "abstract" && pt.QualityScore < 0.3,
			}
		}
	}
}

// enrichAbstractTags queries topic_tag_relations and enriches tags in the category maps
// with IsAbstract flag and ChildSlugs for parent tags.
func enrichAbstractTags(db *gorm.DB, tagMaps ...map[string]*topictypes.TopicTag) {
	var relations []models.TopicTagRelation
	db.Preload("Parent").Preload("Child").Find(&relations)
	if len(relations) == 0 {
		return
	}

	parentToChildren := make(map[uint][]string)
	parentByID := make(map[uint]*models.TopicTag)
	for _, rel := range relations {
		if rel.Parent != nil && rel.Child != nil {
			parentToChildren[rel.ParentID] = append(parentToChildren[rel.ParentID], rel.Child.Slug)
			parentByID[rel.ParentID] = rel.Parent
		}
	}

	childToParents := make(map[uint]uint)
	for _, rel := range relations {
		childToParents[rel.ChildID] = rel.ParentID
	}

	abstractSlugs := make(map[string]bool, len(parentByID))
	for _, parent := range parentByID {
		abstractSlugs[parent.Slug] = true
	}

	for _, m := range tagMaps {
		for slug, tag := range m {
			if abstractSlugs[slug] {
				tag.IsAbstract = true
			}
		}
	}

	allSlugs := make(map[string]*topictypes.TopicTag)
	for _, m := range tagMaps {
		for slug, tag := range m {
			allSlugs[slug] = tag
		}
	}

	for parentID, childSlugs := range parentToChildren {
		parent, ok := parentByID[parentID]
		if !ok {
			continue
		}
		tag, exists := allSlugs[parent.Slug]
		if !exists {
			continue
		}
		tag.IsAbstract = true
		tag.ChildSlugs = childSlugs
	}
}

func toTitle(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	if len(s) == 0 {
		return s
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	result := string(r)
	return strings.ReplaceAll(result, "Ai", "AI")
}
