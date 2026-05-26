package narrative

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

type TagInput struct {
	ID           uint   `json:"id"`
	Label        string `json:"label"`
	Category     string `json:"category"`
	Description  string `json:"description"`
	ArticleCount int    `json:"article_count"`
	Source       string `json:"source"`
	ParentLabel  string `json:"parent_label,omitempty"`
	IsWatched    bool   `json:"is_watched,omitempty"`
}

type PreviousNarrative struct {
	ID         uint64 `json:"id"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
	Generation int    `json:"generation"`
}

type SemanticBoardNarrativeInput struct {
	Board        models.SemanticLabel
	EventTags    []TagInput
	PrevBoardIDs []uint
}

func CollectSemanticBoardNarrativeInputs(date time.Time) ([]SemanticBoardNarrativeInput, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var boards []models.SemanticLabel
	if err := database.DB.Where("label_type = ? AND status = ?", "board", "active").
		Order("display_order ASC, id ASC").
		Find(&boards).Error; err != nil {
		return nil, fmt.Errorf("collect active semantic boards: %w", err)
	}
	if len(boards) == 0 {
		return nil, nil
	}

	boardIDs := make([]uint, 0, len(boards))
	boardByID := make(map[uint]models.SemanticLabel, len(boards))
	for _, board := range boards {
		boardIDs = append(boardIDs, board.ID)
		boardByID[board.ID] = board
	}

	type tagRow struct {
		SemanticBoardID uint
		ID              uint
		Label           string
		Category        string
		Description     string
		Source          string
		ArticleCount    int
	}

	query := database.DB.Model(&models.TopicTag{}).
		Select(`topic_tag_board_labels.semantic_board_id AS semantic_board_id,
			topic_tags.id AS id,
			topic_tags.label AS label,
			topic_tags.category AS category,
			topic_tags.description AS description,
			topic_tags.source AS source,
			COUNT(DISTINCT articles.id) AS article_count`).
		Joins("JOIN topic_tag_board_labels ON topic_tag_board_labels.topic_tag_id = topic_tags.id").
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("topic_tag_board_labels.semantic_board_id IN ?", boardIDs).
		Where("topic_tags.status = ? AND topic_tags.category = ?", "active", models.TagCategoryEvent).
		Where("articles.pub_date >= ? AND articles.pub_date < ?", startOfDay, endOfDay)

	var rows []tagRow
	if err := query.Group(`topic_tag_board_labels.semantic_board_id,
			topic_tags.id,
			topic_tags.label,
			topic_tags.category,
			topic_tags.description,
			topic_tags.source`).
		Order("topic_tag_board_labels.semantic_board_id ASC, article_count DESC, topic_tags.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("collect semantic board event tags: %w", err)
	}

	eventsByBoard := make(map[uint][]TagInput)
	for _, row := range rows {
		if _, ok := boardByID[row.SemanticBoardID]; !ok {
			continue
		}
		eventsByBoard[row.SemanticBoardID] = append(eventsByBoard[row.SemanticBoardID], TagInput{
			ID:           row.ID,
			Label:        row.Label,
			Category:     row.Category,
			Description:  row.Description,
			ArticleCount: row.ArticleCount,
			Source:       row.Source,
		})
	}

	inputs := make([]SemanticBoardNarrativeInput, 0, len(eventsByBoard))
	for _, board := range boards {
		eventTags := eventsByBoard[board.ID]
		if len(eventTags) == 0 {
			continue
		}
		inputs = append(inputs, SemanticBoardNarrativeInput{
			Board:        board,
			EventTags:    eventTags,
			PrevBoardIDs: matchPreviousSemanticBoard(board.ID, date),
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
