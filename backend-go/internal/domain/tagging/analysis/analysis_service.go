package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/domain/tagging"
	"syntopica-backend/internal/platform/logging"
)

const (
	analysisWindowDaily  = "daily"
	analysisWindowWeekly = "weekly"
)

type AnalysisService interface {
	GetOrCreateAnalysis(tagID uint64, analysisType, windowType string, anchorDate time.Time) (*models.TopicTagAnalysis, error)
	GetAnalysis(tagID uint64, analysisType, windowType string, anchorDate time.Time) (*models.TopicTagAnalysis, error)
	RebuildAnalysis(tagID uint64, analysisType, windowType string, anchorDate time.Time) error
	GetAnalysisStatus(tagID uint64, analysisType, windowType string, anchorDate time.Time) (status string, progress float64, err error)
}

type analysisService struct {
	db         *gorm.DB
	queue      AnalysisQueueWithSnapshot
	aiService  AIAnalyzer
	logger     *zap.Logger
	workerOnce sync.Once
}

var (
	analysisQueueInstance AnalysisQueueWithSnapshot
	analysisQueueOnce     sync.Once
	analysisServiceGlobal AnalysisService
	analysisServiceOnce   sync.Once
)

func getAnalysisQueue(db *gorm.DB, logger *zap.Logger) AnalysisQueueWithSnapshot {
	analysisQueueOnce.Do(func() {
		analysisQueueInstance = NewAnalysisQueue(db, logger)
	})
	return analysisQueueInstance
}

func NewAnalysisService(db *gorm.DB) AnalysisService {
	logger := zap.NewNop()
	svc := &analysisService{
		db:        db,
		queue:     getAnalysisQueue(db, logger),
		aiService: NewAIAnalysisService(logger),
		logger:    logger,
	}
	svc.startWorker()
	return svc
}

func GetAnalysisService(db *gorm.DB) AnalysisService {
	analysisServiceOnce.Do(func() {
		analysisServiceGlobal = NewAnalysisService(db)
	})
	return analysisServiceGlobal
}

func (s *analysisService) GetOrCreateAnalysis(tagID uint64, analysisType, windowType string, anchorDate time.Time) (*models.TopicTagAnalysis, error) {
	if err := validateAnalysisParams(analysisType, windowType); err != nil {
		return nil, err
	}

	anchor, err := normalizeAnalysisAnchor(windowType, anchorDate)
	if err != nil {
		return nil, err
	}

	return s.GetAnalysis(tagID, analysisType, windowType, anchor)
}

func (s *analysisService) GetAnalysis(tagID uint64, analysisType, windowType string, anchorDate time.Time) (*models.TopicTagAnalysis, error) {
	if err := validateAnalysisParams(analysisType, windowType); err != nil {
		return nil, err
	}

	anchor, err := normalizeAnalysisAnchor(windowType, anchorDate)
	if err != nil {
		return nil, err
	}

	var analysis models.TopicTagAnalysis
	err = s.db.Where("topic_tag_id = ? AND analysis_type = ? AND window_type = ? AND anchor_date = ?", tagID, analysisType, windowType, anchor).First(&analysis).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query analysis: %w", err)
	}
	return &analysis, nil
}

func (s *analysisService) RebuildAnalysis(tagID uint64, analysisType, windowType string, anchorDate time.Time) error {
	if err := validateAnalysisParams(analysisType, windowType); err != nil {
		return err
	}
	anchor, err := normalizeAnalysisAnchor(windowType, anchorDate)
	if err != nil {
		return err
	}
	return s.enqueue(tagID, analysisType, windowType, anchor, AnalysisPriorityHigh)
}

func (s *analysisService) GetAnalysisStatus(tagID uint64, analysisType, windowType string, anchorDate time.Time) (string, float64, error) {
	anchor, err := normalizeAnalysisAnchor(windowType, anchorDate)
	if err != nil {
		return "error", 0, err
	}

	if snapshot, ok := s.queue.GetLatestByKey(tagID, analysisType, windowType, anchor); ok {
		switch snapshot.Job.Status {
		case AnalysisStatusPending, AnalysisStatusProcessing:
			return snapshot.Job.Status, float64(snapshot.Progress) / 100, nil
		case AnalysisStatusFailed:
			return AnalysisStatusFailed, float64(snapshot.Progress) / 100, nil
		}
	}

	analysis, err := s.GetAnalysis(tagID, analysisType, windowType, anchor)
	if err != nil {
		return "error", 0, err
	}
	if analysis == nil {
		return "missing", 0, nil
	}
	return "ready", 1, nil
}

func (s *analysisService) enqueue(tagID uint64, analysisType, windowType string, anchorDate time.Time, priority int) error {
	return s.queue.Enqueue(&AnalysisJob{
		TopicTagID:   tagID,
		AnalysisType: analysisType,
		WindowType:   windowType,
		AnchorDate:   anchorDate,
		Priority:     normalizePriority(priority),
	})
}

func (s *analysisService) startWorker() {
	s.workerOnce.Do(func() {
		go s.runQueueWorker()
	})
}

func (s *analysisService) runQueueWorker() {
	for {
		job, err := s.queue.Dequeue()
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "closed") {
				return
			}
			logging.Warnf("analysis dequeue failed: %v", err)
			continue
		}
		if job == nil {
			time.Sleep(150 * time.Millisecond)
			continue
		}

		_ = s.queue.MarkProgress(job.ID, 25)
		if _, err := s.buildAndPersist(job.TopicTagID, job.AnalysisType, job.WindowType, job.AnchorDate); err != nil {
			_ = s.queue.UpdateStatus(job.ID, AnalysisStatusFailed, err.Error())
			continue
		}
		_ = s.queue.Complete(job.ID)
	}
}

func (s *analysisService) buildAndPersist(tagID uint64, analysisType, windowType string, anchorDate time.Time) (*models.TopicTagAnalysis, error) {
	if err := validateAnalysisParams(analysisType, windowType); err != nil {
		return nil, err
	}

	anchor, err := normalizeAnalysisAnchor(windowType, anchorDate)
	if err != nil {
		return nil, err
	}

	var tag models.TopicTag
	if err := s.db.First(&tag, tagID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("topic tag %d not found", tagID)
		}
		return nil, fmt.Errorf("failed to load topic tag: %w", err)
	}

	windowStart, windowEnd, _, err := tagging.ResolveWindow(windowType, anchor)
	if err != nil {
		return nil, err
	}

	summaries, err := s.fetchArticlesByTag(tagID, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}

	lookup := models.TopicTagAnalysis{TopicTagID: tagID, AnalysisType: analysisType, WindowType: windowType, AnchorDate: anchor}
	var analysis models.TopicTagAnalysis
	err = s.db.Where(&lookup).First(&analysis).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		analysis = lookup
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load existing analysis: %w", err)
	}

	maxID := maxArticleID(summaries)
	cursor, err := s.getCursor(tagID, analysisType, windowType)
	if err != nil {
		return nil, err
	}
	if analysis.ID != 0 && cursor != nil && cursor.LastArticleID >= maxID {
		return &analysis, nil
	}

	payloadJSON, source, err := s.buildPayloadJSON(tag, analysisType, summaries, windowType, anchor)
	if err != nil {
		return nil, err
	}

	analysis.ArticleCount = len(summaries)
	analysis.PayloadJSON = payloadJSON
	analysis.Source = source
	if analysis.Version <= 0 {
		analysis.Version = 1
	} else {
		analysis.Version++
	}

	if analysis.ID == 0 {
		if err := s.db.Create(&analysis).Error; err != nil {
			return nil, fmt.Errorf("failed to create analysis: %w", err)
		}
	} else {
		if err := s.db.Save(&analysis).Error; err != nil {
			return nil, fmt.Errorf("failed to update analysis: %w", err)
		}
	}

	if err := s.updateCursor(tagID, analysisType, windowType, summaries); err != nil {
		return nil, err
	}
	return &analysis, nil
}

const maxArticlesPerTag = 20

func (s *analysisService) buildPayloadJSON(tag models.TopicTag, analysisType string, articles []models.Article, windowType string, anchor time.Time) (string, string, error) {
	aiParams := AnalysisParams{
		TopicTagID:   uint64(tag.ID),
		TopicLabel:   tag.Label,
		AnalysisType: analysisType,
		WindowType:   windowType,
		AnchorDate:   anchor,
		Summaries:    mapArticleInfos(articles),
	}
	result, err := s.aiService.Analyze(context.TODO(), aiParams)
	if err == nil && result != nil {
		bytes, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			raw := strings.TrimSpace(string(bytes))
			if raw != "" {
				return raw, "ai", nil
			}
		}
	}

	payload, fallbackErr := s.buildPayload(tag, analysisType, articles, windowType)
	if fallbackErr != nil {
		if err != nil {
			return "", "", fmt.Errorf("ai analysis failed: %w; fallback failed: %v", err, fallbackErr)
		}
		return "", "", fallbackErr
	}
	bytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", "", fmt.Errorf("failed to marshal payload: %w", marshalErr)
	}
	return string(bytes), "heuristic", nil
}

func (s *analysisService) fetchArticlesByTag(tagID uint64, windowStart, windowEnd time.Time) ([]models.Article, error) {
	var articles []models.Article
	err := s.db.Model(&models.Article{}).
		Joins("JOIN article_topic_tags att ON att.article_id = articles.id").
		Where("att.topic_tag_id = ?", tagID).
		Where("articles.created_at >= ? AND articles.created_at < ?", windowStart, windowEnd).
		Preload("Feed").
		Order("articles.created_at DESC").
		Limit(maxArticlesPerTag).
		Find(&articles).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query articles for tag: %w", err)
	}
	return articles, nil
}

func (s *analysisService) getCursor(tagID uint64, analysisType string, windowType string) (*models.TopicAnalysisCursor, error) {
	var cursor models.TopicAnalysisCursor
	err := s.db.Where("topic_tag_id = ? AND analysis_type = ? AND window_type = ?", tagID, analysisType, windowType).First(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query analysis cursor: %w", err)
	}
	return &cursor, nil
}

func maxArticleID(articles []models.Article) uint64 {
	var maxID uint64
	for _, article := range articles {
		if uint64(article.ID) > maxID {
			maxID = uint64(article.ID)
		}
	}
	return maxID
}

func mapArticleInfos(articles []models.Article) []SummaryInfo {
	out := make([]SummaryInfo, 0, len(articles))
	for _, article := range articles {
		summary := article.Description
		if article.AIContentSummary != "" {
			summary = article.AIContentSummary
		}
		item := SummaryInfo{
			ArticleID: uint64(article.ID),
			Title:     article.Title,
			Summary:   summary,
			CreatedAt: article.CreatedAt.In(tagging.TopicGraphCST).Format("2006-01-02"),
		}
		if article.Feed.ID != 0 {
			item.FeedName = article.Feed.Title
		}
		out = append(out, item)
	}
	return out
}

func (s *analysisService) buildPayload(tag models.TopicTag, analysisType string, articles []models.Article, windowType string) (map[string]any, error) {
	switch analysisType {
	case AnalysisTypeEvent:
		return s.buildEventPayload(tag, articles), nil
	case AnalysisTypePerson:
		return s.buildPersonPayload(tag, articles, windowType), nil
	case AnalysisTypeKeyword:
		return s.buildKeywordPayload(tag, articles, windowType), nil
	default:
		return nil, fmt.Errorf("unsupported analysis type: %s", analysisType)
	}
}

func (s *analysisService) buildEventPayload(tag models.TopicTag, articles []models.Article) map[string]any {
	timeline := make([]map[string]any, 0, len(articles))
	keyMoments := make([]string, 0, minInt(len(articles), 5))

	for i, article := range articles {
		sources := []map[string]any{
			{"article_id": article.ID, "title": article.Title, "link": article.Link},
		}

		summary := article.Description
		if article.AIContentSummary != "" {
			summary = article.AIContentSummary
		}

		timeline = append(timeline, map[string]any{
			"date":            article.CreatedAt.In(tagging.TopicGraphCST).Format("2006-01-02"),
			"title":           article.Title,
			"summary":         truncateText(summary, 240),
			"source_articles": sources,
		})
		if i < 5 {
			keyMoments = append(keyMoments, article.Title)
		}
	}

	return map[string]any{
		"timeline":       timeline,
		"key_moments":    keyMoments,
		"related_topics": s.fetchRelatedTopicLabels(uint64(tag.ID), 8),
	}
}

func (s *analysisService) buildPersonPayload(tag models.TopicTag, articles []models.Article, windowType string) map[string]any {
	appearances := make([]map[string]any, 0, len(articles))

	for _, article := range articles {
		summary := article.Description
		if article.AIContentSummary != "" {
			summary = article.AIContentSummary
		}

		item := map[string]any{
			"date":  article.CreatedAt.In(tagging.TopicGraphCST).Format("2006-01-02"),
			"scene": truncateText(article.Title, 96),
			"quote": firstSentence(summary),
		}
		if article.Title != "" {
			item["article_title"] = article.Title
			item["article_link"] = article.Link
		}
		appearances = append(appearances, item)
	}

	background := "暂无背景资料"
	if len(articles) > 0 {
		bg := articles[0].Description
		if articles[0].AIContentSummary != "" {
			bg = articles[0].AIContentSummary
		}
		background = truncateText(bg, 280)
	}

	return map[string]any{
		"profile": map[string]any{
			"name":       tag.Label,
			"role":       "相关人物",
			"background": background,
		},
		"appearances": appearances,
		"trend_data":  buildTrendData(articles, windowType),
	}
}

func (s *analysisService) buildKeywordPayload(tag models.TopicTag, articles []models.Article, windowType string) map[string]any {
	contextExamples := make([]map[string]any, 0, minInt(len(articles), 6))
	for i, article := range articles {
		if i >= 6 {
			break
		}
		summary := article.Description
		if article.AIContentSummary != "" {
			summary = article.AIContentSummary
		}
		contextExamples = append(contextExamples, map[string]any{
			"text":   truncateText(summary, 220),
			"source": article.Title,
		})
	}

	coOccurrence := s.fetchCoOccurrence(uint64(tag.ID), 12)
	relatedTopics := make([]string, 0, minInt(len(coOccurrence), 8))
	for i, item := range coOccurrence {
		if i >= 8 {
			break
		}
		name, _ := item["keyword"].(string)
		relatedTopics = append(relatedTopics, name)
	}

	return map[string]any{
		"trend_data":       buildTrendData(articles, windowType),
		"related_topics":   relatedTopics,
		"co_occurrence":    coOccurrence,
		"context_examples": contextExamples,
	}
}

func (s *analysisService) fetchRelatedTopicLabels(tagID uint64, limit int) []string {
	type row struct{ Label string }
	rows := make([]row, 0)
	_ = s.db.Table("article_topic_tags att").
		Select("tt.label").
		Joins("JOIN article_topic_tags att2 ON att2.article_id = att.article_id").
		Joins("JOIN topic_tags tt ON tt.id = att2.topic_tag_id").
		Where("att.topic_tag_id = ?", tagID).
		Where("att2.topic_tag_id <> ?", tagID).
		Group("tt.id, tt.label").
		Order("COUNT(*) DESC").
		Limit(limit).
		Scan(&rows).Error

	result := make([]string, 0, len(rows))
	for _, r := range rows {
		if strings.TrimSpace(r.Label) != "" {
			result = append(result, r.Label)
		}
	}
	return result
}

func (s *analysisService) fetchCoOccurrence(tagID uint64, limit int) []map[string]any {
	type row struct {
		Keyword string
		Count   int
	}
	rows := make([]row, 0)
	_ = s.db.Table("article_topic_tags att").
		Select("tt.label AS keyword, COUNT(*) AS count").
		Joins("JOIN article_topic_tags att2 ON att2.article_id = att.article_id").
		Joins("JOIN topic_tags tt ON tt.id = att2.topic_tag_id").
		Where("att.topic_tag_id = ?", tagID).
		Where("att2.topic_tag_id <> ?", tagID).
		Group("tt.id, tt.label").
		Order("COUNT(*) DESC").
		Limit(limit).
		Scan(&rows).Error

	maxCount := 0
	for _, r := range rows {
		if r.Count > maxCount {
			maxCount = r.Count
		}
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		score := 0.0
		if maxCount > 0 {
			score = float64(r.Count) / float64(maxCount)
		}
		out = append(out, map[string]any{"keyword": r.Keyword, "score": score})
	}
	return out
}

func (s *analysisService) updateCursor(tagID uint64, analysisType, windowType string, articles []models.Article) error {
	lookup := models.TopicAnalysisCursor{TopicTagID: tagID, AnalysisType: analysisType, WindowType: windowType}
	var cursor models.TopicAnalysisCursor
	err := s.db.Where(&lookup).First(&cursor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cursor = lookup
	} else if err != nil {
		return fmt.Errorf("failed to query analysis cursor: %w", err)
	}

	cursor.LastArticleID = maxArticleID(articles)
	cursor.LastUpdatedAt = time.Now().In(tagging.TopicGraphCST)

	if cursor.ID == 0 {
		if err := s.db.Create(&cursor).Error; err != nil {
			return fmt.Errorf("failed to create analysis cursor: %w", err)
		}
		return nil
	}
	if err := s.db.Save(&cursor).Error; err != nil {
		return fmt.Errorf("failed to update analysis cursor: %w", err)
	}
	return nil
}

func validateAnalysisParams(analysisType, windowType string) error {
	switch analysisType {
	case AnalysisTypeEvent, AnalysisTypePerson, AnalysisTypeKeyword:
	default:
		return fmt.Errorf("unsupported analysis type: %s", analysisType)
	}
	switch windowType {
	case analysisWindowDaily, analysisWindowWeekly:
	default:
		return fmt.Errorf("unsupported window type: %s", windowType)
	}
	return nil
}

func normalizeAnalysisAnchor(windowType string, anchorDate time.Time) (time.Time, error) {
	windowStart, _, _, err := tagging.ResolveWindow(windowType, anchorDate)
	if err != nil {
		return time.Time{}, err
	}
	return windowStart, nil
}

func buildTrendData(articles []models.Article, windowType string) []map[string]any {
	counter := map[string]int{}
	for _, article := range articles {
		key := article.CreatedAt.In(tagging.TopicGraphCST).Format("2006-01-02")
		counter[key]++
	}
	trend := make([]map[string]any, 0, len(counter))
	for date, count := range counter {
		trend = append(trend, map[string]any{"date": date, "count": count})
	}
	sort.SliceStable(trend, func(i, j int) bool {
		di, _ := trend[i]["date"].(string)
		dj, _ := trend[j]["date"].(string)
		return di < dj
	})
	if windowType == analysisWindowWeekly && len(trend) > 7 {
		return trend[len(trend)-7:]
	}
	return trend
}

func truncateText(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if maxLen <= 0 || len(trimmed) <= maxLen {
		return trimmed
	}
	if maxLen <= 3 {
		return trimmed[:maxLen]
	}
	return trimmed[:maxLen-3] + "..."
}

func firstSentence(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	for _, sep := range []string{"。", "!", "?", "."} {
		if idx := strings.Index(trimmed, sep); idx > 0 {
			return strings.TrimSpace(trimmed[:idx+len(sep)])
		}
	}
	return truncateText(trimmed, 120)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
