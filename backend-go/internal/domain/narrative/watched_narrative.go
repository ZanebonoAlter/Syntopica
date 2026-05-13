package narrative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/tagging/watched"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/jsonutil"
	"my-robot-backend/internal/platform/logging"
)

type WatchedTagNarrativeOutput struct {
	TagID     uint   `json:"tag_id"`
	TagLabel  string `json:"tag_label"`
	Summary   string `json:"summary"`
	DateRange string `json:"date_range"`
}

// Deprecated: GenerateWatchedTagNarratives is removed from the main narrative generation flow.
// Kept for potential future use or manual invocation.
func GenerateWatchedTagNarratives(date time.Time) {
	watchedIDs, childIDs, err := watched.GetWatchedTagIDsExpanded(database.DB)
	if err != nil {
		logging.Warnf("watched-narrative: failed to get watched tags: %v", err)
		return
	}
	if len(watchedIDs) == 0 {
		return
	}

	allTagIDs := watchedIDs
	allTagIDs = append(allTagIDs, childIDs...)
	watchedSet := make(map[uint]bool, len(watchedIDs))
	for _, id := range watchedIDs {
		watchedSet[id] = true
	}

	since := date.AddDate(0, 0, -2)
	endOfDay := date.Add(24 * time.Hour)

	type tagActivity struct {
		TopicTagID uint
		Cnt        int
	}
	var activities []tagActivity
	database.DB.Model(&models.ArticleTopicTag{}).
		Select("article_topic_tags.topic_tag_id, COUNT(DISTINCT article_topic_tags.article_id) as cnt").
		Joins("JOIN articles ON articles.id = article_topic_tags.article_id").
		Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", allTagIDs, since, endOfDay).
		Group("article_topic_tags.topic_tag_id").
		Having("COUNT(DISTINCT article_topic_tags.article_id) >= 2").
		Scan(&activities)

	if len(activities) == 0 {
		return
	}

	activeWatchedMap := make(map[uint]int)
	for _, act := range activities {
		if watchedSet[act.TopicTagID] {
			activeWatchedMap[act.TopicTagID] = act.Cnt
		}
	}
	if len(childIDs) > 0 {
		var relations []models.TopicTagRelation
		database.DB.Where("child_id IN ? AND parent_id IN ?", childIDs, watchedIDs).Find(&relations)
		for _, rel := range relations {
			for _, act := range activities {
				if act.TopicTagID == rel.ChildID {
					activeWatchedMap[rel.ParentID] += act.Cnt
				}
			}
		}
	}

	if len(activeWatchedMap) == 0 {
		return
	}

	sem := make(chan struct{}, 5)
	for tagID, articleCount := range activeWatchedMap {
		if articleCount < 2 {
			continue
		}
		sem <- struct{}{}
		go func(tid uint) {
			defer func() { <-sem }()
			generateSingleWatchedNarrative(tid, since, endOfDay)
		}(tagID)
	}
	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}
}

func generateSingleWatchedNarrative(watchedTagID uint, since, until time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logging.Warnf("generateSingleWatchedNarrative panic for tag %d: %v", watchedTagID, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var tag models.TopicTag
	if err := database.DB.First(&tag, watchedTagID).Error; err != nil {
		logging.Warnf("watched-narrative: tag %d not found: %v", watchedTagID, err)
		return
	}

	var articles []models.Article
	database.DB.Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
		Where("article_topic_tags.topic_tag_id = ? AND articles.pub_date >= ? AND articles.pub_date < ?", watchedTagID, since, until).
		Order("articles.pub_date DESC").
		Limit(20).
		Find(&articles)

	var childIDs []uint
	database.DB.Model(&models.TopicTagRelation{}).
		Where("parent_id = ? AND relation_type = ?", watchedTagID, "abstract").
		Pluck("child_id", &childIDs)
	if len(childIDs) > 0 {
		var childArticles []models.Article
		database.DB.Joins("JOIN article_topic_tags ON article_topic_tags.article_id = articles.id").
			Where("article_topic_tags.topic_tag_id IN ? AND articles.pub_date >= ? AND articles.pub_date < ?", childIDs, since, until).
			Order("articles.pub_date DESC").
			Limit(20).
			Find(&childArticles)
		articles = append(articles, childArticles...)
	}

	if len(articles) == 0 {
		return
	}

	seen := make(map[uint]bool)
	var uniqueArticles []models.Article
	for _, a := range articles {
		if !seen[a.ID] {
			seen[a.ID] = true
			uniqueArticles = append(uniqueArticles, a)
		}
	}
	articles = uniqueArticles

	var articleParts []string
	for _, a := range articles {
		part := fmt.Sprintf("- [%s] %s", a.PubDate.Format("01-02"), a.Title)
		if a.AIContentSummary != "" {
			summary := a.AIContentSummary
			if len([]rune(summary)) > 200 {
				summary = string([]rune(summary)[:200]) + "..."
			}
			part += fmt.Sprintf(": %s", summary)
		}
		articleParts = append(articleParts, part)
	}

	prompt := fmt.Sprintf(`你是一个新闻分析助手。以下是与标签 "%s" 相关的近期文章列表。
请基于这些文章，生成一段该标签的近期发展脉络总结。

时间范围: %s ~ %s
相关文章:
%s

要求:
- 中文，300-800 字
- 按时间线梳理关键进展
- 客观总结事实，不要评价
- 如果文章间有因果或递进关系，明确指出
- 如果信息不足以生成有意义的发展脉络，直接说明

返回 JSON: {"summary": "你的总结"}`, tag.Label, since.Format("2006-01-02"), until.Format("2006-01-02"), strings.Join(articleParts, "\n"))

	router := airouter.NewRouter()
	result, err := router.Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是一个新闻分析助手，只输出合法JSON。"},
			{Role: "user", Content: prompt},
		},
		JSONMode: true,
		JSONSchema: &airouter.JSONSchema{
			Type: "object",
			Properties: map[string]airouter.SchemaProperty{
				"summary": {Type: "string", Description: "该关注标签的近期发展脉络总结"},
			},
			Required: []string{"summary"},
		},
		Temperature: func() *float64 { f := 0.4; return &f }(),
		Metadata: map[string]any{
			"operation": "watched_narrative_summary",
			"tag_id":    tag.ID,
			"tag_label": tag.Label,
		},
	})
	if err != nil {
		logging.Warnf("watched-narrative: LLM call failed for tag %d (%s): %v", tag.ID, tag.Label, err)
		return
	}

	var parsed struct {
		Summary string `json:"summary"`
	}
	content := jsonutil.SanitizeLLMJSON(result.Content)
	if err := json.Unmarshal([]byte(content), &parsed); err != nil || parsed.Summary == "" {
		logging.Warnf("watched-narrative: failed to parse response for tag %d: %v", tag.ID, err)
		return
	}

	startOfDay := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
	record := models.NarrativeSummary{
		Title:         fmt.Sprintf("关注标签：%s 近期动态", tag.Label),
		Summary:       parsed.Summary,
		Status:        models.NarrativeStatusContinuing,
		Period:        "watched_tag",
		PeriodDate:    startOfDay,
		Generation:    1,
		RelatedTagIDs: fmt.Sprintf("[%d]", tag.ID),
		Source:        "ai",
	}

	if err := database.DB.Create(&record).Error; err != nil {
		logging.Warnf("watched-narrative: failed to save for tag %d (%s): %v", tag.ID, tag.Label, err)
		return
	}

	logging.Infof("watched-narrative: saved narrative for watched tag %d (%s), articles=%d", tag.ID, tag.Label, len(articles))
}
