package topicgraph

import (
	"fmt"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/domain/tagging"
	"syntopica-backend/internal/platform/database"
)

func collectAllChildTagIDs(parentTagID uint) map[uint]bool {
	result := map[uint]bool{parentTagID: true}
	queue := []uint{parentTagID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		var relations []models.TopicTagRelation
		database.DB.Where("parent_id = ? AND relation_type = ?", current, "abstract").
			Find(&relations)

		for _, r := range relations {
			if !result[r.ChildID] {
				result[r.ChildID] = true
				queue = append(queue, r.ChildID)
			}
		}
	}

	return result
}

type HotspotDigestCard struct {
	ID          uint                         `json:"id"`
	Title       string                       `json:"title"`
	Link        string                       `json:"link"`
	FeedName    string                       `json:"feed_name"`
	FeedIcon    string                       `json:"feed_icon,omitempty"`
	FeedColor   string                       `json:"feed_color,omitempty"`
	PublishedAt string                       `json:"published_at,omitempty"`
	Tags        []tagging.AggregatedTopicTag `json:"tags,omitempty"`
}

func GetDigestsByArticleTag(tagSlug string, windowKind string, anchor time.Time, limit int, tagKind string) ([]HotspotDigestCard, error) {
	windowStart, windowEnd, _, err := tagging.ResolveWindow(windowKind, anchor)
	if err != nil {
		return nil, err
	}

	var topicTag models.TopicTag
	query := database.DB.Where("slug = ?", tagSlug)
	if tagKind != "" {
		query = query.Where("kind = ?", tagKind)
	}
	err = query.First(&topicTag).Error
	if err != nil {
		return nil, fmt.Errorf("topic tag not found: %w", err)
	}

	tagIDSet := collectAllChildTagIDs(topicTag.ID)
	tagIDs := make([]uint, 0, len(tagIDSet))
	for id := range tagIDSet {
		tagIDs = append(tagIDs, id)
	}

	var articles []models.Article
	err = database.DB.
		Joins("JOIN article_topic_tags ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ?", tagIDs).
		Where("articles.created_at >= ? AND articles.created_at < ?", windowStart, windowEnd).
		Preload("Feed").
		Omit("tag_count", "relevance_score").
		Distinct().
		Order("articles.created_at DESC").
		Find(&articles).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get articles: %w", err)
	}

	if limit > 0 && len(articles) > limit {
		articles = articles[:limit]
	}

	result := make([]HotspotDigestCard, 0, len(articles))
	for _, article := range articles {
		card := HotspotDigestCard{
			ID:    article.ID,
			Title: article.Title,
			Link:  article.Link,
		}

		if article.PubDate != nil {
			card.PublishedAt = article.PubDate.In(tagging.TopicGraphCST).Format(time.RFC3339)
		}

		if article.Feed.ID != 0 {
			card.FeedName = article.Feed.Title
			card.FeedIcon = article.Feed.Icon
			card.FeedColor = article.Feed.Color
		} else {
			card.FeedName = "未知订阅源"
		}

		result = append(result, card)
	}

	return result, nil
}
