package contentprocessing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	otelCodes "go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicextraction"
	platformai "my-robot-backend/internal/platform/ai"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
)

type ContentCompletionService struct {
	crawlClient *Crawl4AIClient
	aiService   *platformai.AIService
	router      *airouter.Router
}

type ContentCompletionArticleRef struct {
	ID     uint   `json:"id"`
	FeedID uint   `json:"feed_id"`
	Title  string `json:"title"`
}

type ContentCompletionOverview struct {
	PendingCount           int                             `json:"pending_count"`
	ProcessingCount        int                             `json:"processing_count"`
	LiveProcessingCount    int                             `json:"live_processing_count"`
	StaleProcessingCount   int                             `json:"stale_processing_count"`
	CompletedCount         int                             `json:"completed_count"`
	FailedCount            int                             `json:"failed_count"`
	BlockedCount           int                             `json:"blocked_count"`
	TotalCount             int                             `json:"total_count"`
	AIConfigured           bool                            `json:"ai_configured"`
	BlockedReasons         ContentCompletionBlockedReasons `json:"blocked_reasons"`
	StaleProcessingArticle *ContentCompletionArticleRef    `json:"stale_processing_article"`
}

type ContentCompletionBlockedReasons struct {
	WaitingForFirecrawlCount    int `json:"waiting_for_firecrawl_count"`
	FeedDisabledCount           int `json:"feed_disabled_count"`
	AIUnconfiguredCount         int `json:"ai_unconfigured_count"`
	ReadyButMissingContentCount int `json:"ready_but_missing_content_count"`
}

const (
	contentCompletionProcessingLease      = 30 * time.Minute
	contentCompletionClockSkewGracePeriod = 2 * time.Minute
)

func NewContentCompletionService(crawlBaseURL string) *ContentCompletionService {
	return &ContentCompletionService{
		crawlClient: NewCrawl4AIClient(crawlBaseURL),
		aiService:   platformai.NewAIService("", "", ""),
		router:      airouter.NewRouter(),
	}
}

func (s *ContentCompletionService) SetAICredentials(baseURL, apiKey, model string) {
	s.aiService = platformai.NewAIService(baseURL, apiKey, model)
}

func (s *ContentCompletionService) SetCrawlAPIToken(token string) {
	s.crawlClient.SetAPIToken(token)
}

func (s *ContentCompletionService) IsContentIncomplete(article *models.Article) bool {
	if article.SummaryStatus == "complete" || article.SummaryStatus == "failed" {
		return false
	}

	content := strings.TrimSpace(article.Content)
	if content == "" || content == strings.TrimSpace(article.Description) {
		return true
	}

	if len(content) < 200 {
		return true
	}

	return false
}

func (s *ContentCompletionService) CompleteArticle(ctx context.Context, articleID uint) error {
	ctx, span := otel.Tracer("rss-reader-backend").Start(ctx, "ContentCompletionService.CompleteArticle")
	defer span.End()
	/*line backend-go/internal/domain/contentprocessing/content_completion_service.go:89:2*/ return s.CompleteArticleWithMetadata(ctx, articleID, false, nil)
}

func (s *ContentCompletionService) CompleteArticleWithForce(ctx context.Context, articleID uint, force bool) error {
	ctx, span := otel.Tracer("rss-reader-backend").Start(ctx, "ContentCompletionService.CompleteArticleWithForce")
	defer span.End()
	/*line backend-go/internal/domain/contentprocessing/content_completion_service.go:93:2*/ return s.CompleteArticleWithMetadata(ctx, articleID, force, nil)
}

func (s *ContentCompletionService) CompleteArticleWithMetadata(ctx context.Context, articleID uint, force bool, metadata map[string]any) (err error) {
	ctx, span := otel.Tracer("rss-reader-backend").Start(ctx, "ContentCompletionService.CompleteArticleWithMetadata")
	defer span.End()
	defer func() {
		if err != nil {
			span.SetStatus(otelCodes.Error, "error")
			span.RecordError(err)
		}
	}()
	/*line backend-go/internal/domain/contentprocessing/content_completion_service.go:97:2*/ var article models.Article
	if err := database.DB.First(&article, articleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("article not found")
		}
		return fmt.Errorf("failed to fetch article: %w", err)
	}

	if article.SummaryStatus == "complete" && !force {
		return nil
	}

	var feed models.Feed
	if err := database.DB.First(&feed, article.FeedID).Error; err != nil {
		return fmt.Errorf("failed to fetch feed: %w", err)
	}

	if !feed.ArticleSummaryEnabled {
		return fmt.Errorf("AI summary not enabled for this feed")
	}

	if article.FirecrawlStatus != "completed" {
		return fmt.Errorf("firecrawl not completed for this article")
	}

	if article.CompletionAttempts >= completionRetryLimit(&feed) && !force {
		article.SummaryStatus = "failed"
		article.CompletionError = "Max retries exceeded"
		article.SummaryProcessingStartedAt = nil
		database.DB.Save(&article)
		return fmt.Errorf("max completion retries exceeded")
	}

	if force {
		article.AIContentSummary = ""
		article.SummaryGeneratedAt = nil
	}

	now := currentCompletionTime()
	claimed, err := s.claimArticleForCompletion(article.ID, force, now)
	if err != nil {
		return fmt.Errorf("claim article for completion: %w", err)
	}
	if !claimed {
		return nil
	}

	if err := database.DB.First(&article, articleID).Error; err != nil {
		return fmt.Errorf("failed to reload claimed article: %w", err)
	}

	if s.aiService == nil || s.aiService.BaseURL == "" || s.aiService.APIKey == "" {
		if !s.hasRouteConfig() {
			if err := s.persistCompletionFailure(&article, &feed, "AI service not configured"); err != nil {
				return fmt.Errorf("persist completion failure: %w", err)
			}
			return fmt.Errorf("AI service not configured")
		}
	}

	contentToSummarize := article.FirecrawlContent
	if contentToSummarize == "" {
		if err := s.persistCompletionFailure(&article, &feed, "No firecrawl content available"); err != nil {
			return fmt.Errorf("persist completion failure: %w", err)
		}
		return fmt.Errorf("no firecrawl content available")
	}

	summary, err := s.summarizeContent(article.ID, article.FeedID, article.Title, contentToSummarize, metadata)
	if err != nil {
		if err := s.persistCompletionFailure(&article, &feed, err.Error()); err != nil {
			return fmt.Errorf("persist completion failure: %w", err)
		}
		return fmt.Errorf("AI summarization failed: %w", err)
	}

	now = currentCompletionTime()
	article.AIContentSummary = formatAISummary(summary)
	article.SummaryStatus = "complete"
	article.CompletionError = ""
	article.SummaryGeneratedAt = &now
	article.SummaryProcessingStartedAt = nil

	if err := database.DB.Save(&article).Error; err != nil {
		return fmt.Errorf("failed to save article: %w", err)
	}

	if feed.TaggingEnabled {
		if err := topicextraction.NewTagJobQueue(database.DB).Enqueue(topicextraction.TagJobRequest{
			ArticleID:    article.ID,
			FeedName:     feed.Title,
			CategoryName: topicextraction.FeedCategoryName(feed),
			ForceRetag:   false,
			Reason:       "summary_completed",
		}); err != nil {
			return fmt.Errorf("enqueue retag job after completion: %w", err)
		}
	}

	return nil
}

func (s *ContentCompletionService) AutoCompletePendingArticles(limit int) ([]uint, []error) {
	articles, err := s.ListReadyArticles(limit)

	if err != nil {
		return nil, []error{fmt.Errorf("failed to fetch articles: %w", err)}
	}

	var completedIDs []uint
	var errors []error

	for _, article := range articles {
		if article.CompletionAttempts >= article.Feed.MaxCompletionRetries {
			continue
		}

		if err := s.CompleteArticle(context.Background(), article.ID); err != nil {
			errors = append(errors, fmt.Errorf("article %d: %w", article.ID, err))
		} else {
			completedIDs = append(completedIDs, article.ID)
		}
	}

	return completedIDs, errors
}

func (s *ContentCompletionService) ListReadyArticles(limit int) ([]models.Article, error) {
	var articles []models.Article
	staleBefore := staleCompletionStartedBefore(currentCompletionTime())

	query := database.DB.
		Joins("JOIN feeds ON feeds.id = articles.feed_id").
		Where("articles.firecrawl_status = ?", "completed").
		Where("feeds.article_summary_enabled = ?", true).
		Where("articles.summary_status = ? OR (articles.summary_status = ? AND (articles.summary_processing_started_at IS NULL OR articles.summary_processing_started_at <= ?))", "incomplete", "pending", staleBefore).
		Omit("tag_count", "relevance_score").
		Preload("Feed")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&articles).Error; err != nil {
		return nil, err
	}

	return articles, nil
}

func (s *ContentCompletionService) CheckAndMarkIncompleteArticles(feedID uint) (int, error) {
	var articles []models.Article
	if err := database.DB.Omit("tag_count", "relevance_score").Where("feed_id = ?", feedID).Find(&articles).Error; err != nil {
		return 0, err
	}

	count := 0
	for _, article := range articles {
		if s.IsContentIncomplete(&article) && article.SummaryStatus != "failed" {
			article.SummaryStatus = "incomplete"
			database.DB.Save(&article)
			count++
		}
	}

	return count, nil
}

func (s *ContentCompletionService) GetOverview() (*ContentCompletionOverview, error) {
	overview := &ContentCompletionOverview{}
	overview.AIConfigured = (s.aiService != nil && s.aiService.BaseURL != "" && s.aiService.APIKey != "") || s.hasRouteConfig()

	countQuery := []struct {
		assign func(int64)
		query  func(*gorm.DB) *gorm.DB
	}{
		{
			assign: func(count int64) { overview.PendingCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).
					Joins("JOIN feeds ON feeds.id = articles.feed_id").
					Where("articles.firecrawl_status = ? AND articles.summary_status = ?", "completed", "incomplete").
					Where("feeds.article_summary_enabled = ?", true)
			},
		},
		{
			assign: func(count int64) { overview.ProcessingCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).Where("summary_status = ?", "pending")
			},
		},
		{
			assign: func(count int64) { overview.CompletedCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).Where("summary_status = ?", "complete")
			},
		},
		{
			assign: func(count int64) { overview.FailedCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).Where("summary_status = ?", "failed")
			},
		},
		{
			assign: func(count int64) { overview.BlockedCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).
					Joins("JOIN feeds ON feeds.id = articles.feed_id").
					Where("articles.summary_status = ?", "incomplete").
					Where("articles.firecrawl_status <> ? OR feeds.article_summary_enabled = ?", "completed", false)
			},
		},
		{
			assign: func(count int64) { overview.TotalCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{})
			},
		},
	}

	for _, item := range countQuery {
		var count int64
		if err := item.query(database.DB).Count(&count).Error; err != nil {
			return nil, err
		}
		item.assign(count)
	}

	staleBefore := staleCompletionStartedBefore(currentCompletionTime())
	var staleCount int64
	if err := database.DB.Model(&models.Article{}).
		Where("summary_status = ?", "pending").
		Where("summary_processing_started_at IS NULL OR summary_processing_started_at <= ?", staleBefore).
		Count(&staleCount).Error; err != nil {
		return nil, err
	}
	overview.StaleProcessingCount = int(staleCount)
	overview.LiveProcessingCount = 0

	var staleArticle models.Article
	if err := database.DB.Where("summary_status = ?", "pending").Where("summary_processing_started_at IS NULL OR summary_processing_started_at <= ?", staleBefore).Order("summary_processing_started_at ASC").Order("created_at ASC").First(&staleArticle).Error; err == nil {
		overview.StaleProcessingArticle = ToArticleRef(staleArticle)
	}

	blockedQueries := []struct {
		assign func(int64)
		query  func(*gorm.DB) *gorm.DB
	}{
		{
			assign: func(count int64) { overview.BlockedReasons.WaitingForFirecrawlCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).
					Joins("JOIN feeds ON feeds.id = articles.feed_id").
					Where("articles.summary_status = ?", "incomplete").
					Where("feeds.article_summary_enabled = ?", true).
					Where("articles.firecrawl_status <> ?", "completed")
			},
		},
		{
			assign: func(count int64) { overview.BlockedReasons.FeedDisabledCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).
					Joins("JOIN feeds ON feeds.id = articles.feed_id").
					Where("articles.summary_status = ?", "incomplete").
					Where("feeds.article_summary_enabled = ?", false)
			},
		},
		{
			assign: func(count int64) { overview.BlockedReasons.ReadyButMissingContentCount = int(count) },
			query: func(db *gorm.DB) *gorm.DB {
				return db.Model(&models.Article{}).
					Joins("JOIN feeds ON feeds.id = articles.feed_id").
					Where("articles.firecrawl_status = ? AND articles.summary_status = ?", "completed", "incomplete").
					Where("feeds.article_summary_enabled = ?", true).
					Where("TRIM(COALESCE(articles.firecrawl_content, '')) = ''")
			},
		},
	}

	for _, item := range blockedQueries {
		var count int64
		if err := item.query(database.DB).Count(&count).Error; err != nil {
			return nil, err
		}
		item.assign(count)
	}

	if !overview.AIConfigured {
		overview.BlockedReasons.AIUnconfiguredCount = overview.PendingCount
	}

	return overview, nil
}

func (s *ContentCompletionService) hasRouteConfig() bool {
	if s.router == nil {
		return false
	}
	provider, _, err := s.router.ResolvePrimaryProvider(airouter.CapabilityArticleCompletion)
	return err == nil && provider != nil && strings.TrimSpace(provider.APIKey) != ""
}

func (s *ContentCompletionService) summarizeContent(articleID uint, feedID uint, title, content string, metadata map[string]any) (*platformai.AISummaryResponse, error) {
	requestMeta := map[string]any{
		"article_id": articleID,
		"feed_id":    feedID,
		"title":      title,
	}
	for key, value := range metadata {
		requestMeta[key] = value
	}

	if s.router != nil {
		maxTokens := 16000
		result, err := s.router.Chat(context.Background(), airouter.ChatRequest{
			Capability: airouter.CapabilityArticleCompletion,
			Messages: []airouter.Message{
				{Role: "system", Content: s.aiService.GetSystemPrompt("zh")},
				{Role: "user", Content: s.aiService.PrepareArticleContent(title, content)},
			},
			MaxTokens: &maxTokens,
			Metadata:  requestMeta,
		})
		if err == nil {
			return platformai.ParseSummaryMarkdown(result.Content), nil
		}
		if s.aiService == nil || s.aiService.BaseURL == "" || s.aiService.APIKey == "" {
			return nil, err
		}
	}

	return s.aiService.SummarizeArticle(title, content, "zh")
}

func (s *ContentCompletionService) persistCompletionFailure(article *models.Article, feed *models.Feed, message string) error {
	article.CompletionError = message
	article.SummaryProcessingStartedAt = nil
	if article.CompletionAttempts >= completionRetryLimit(feed) {
		article.SummaryStatus = "failed"
	} else {
		article.SummaryStatus = "incomplete"
	}
	return database.DB.Save(article).Error
}

func completionRetryLimit(feed *models.Feed) int {
	if feed == nil || feed.MaxCompletionRetries <= 0 {
		return 1
	}
	return feed.MaxCompletionRetries
}

func currentCompletionTime() time.Time {
	return time.Now().In(time.FixedZone("CST", 8*3600))
}

func staleCompletionStartedBefore(now time.Time) time.Time {
	return now.Add(-(contentCompletionProcessingLease + contentCompletionClockSkewGracePeriod))
}

func (s *ContentCompletionService) claimArticleForCompletion(articleID uint, force bool, now time.Time) (bool, error) {
	updates := map[string]any{
		"summary_status":                "pending",
		"completion_error":              "",
		"summary_processing_started_at": now,
		"completion_attempts":           gorm.Expr("completion_attempts + 1"),
	}
	if force {
		updates["ai_content_summary"] = ""
		updates["summary_generated_at"] = nil
	}

	query := database.DB.Model(&models.Article{}).Where("id = ?", articleID)
	if force {
		query = query.Where("summary_status <> ? OR summary_processing_started_at IS NULL OR summary_processing_started_at <= ?", "pending", staleCompletionStartedBefore(now))
	} else {
		query = query.Where("summary_status = ? OR (summary_status = ? AND (summary_processing_started_at IS NULL OR summary_processing_started_at <= ?))", "incomplete", "pending", staleCompletionStartedBefore(now))
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected > 0, nil
}

func ToArticleRef(article models.Article) *ContentCompletionArticleRef {
	return &ContentCompletionArticleRef{
		ID:     article.ID,
		FeedID: article.FeedID,
		Title:  article.Title,
	}
}

func formatAISummary(summary *platformai.AISummaryResponse) string {
	if summary == nil {
		return ""
	}

	if strings.TrimSpace(summary.Markdown) != "" {
		return strings.TrimSpace(summary.Markdown)
	}

	var result strings.Builder
	result.WriteString("# 内容整理\n\n")

	if summary.OneSentence != "" {
		result.WriteString(fmt.Sprintf("> %s\n\n", summary.OneSentence))
	}

	if len(summary.KeyPoints) > 0 {
		result.WriteString("## 关键点\n\n")
		for _, point := range summary.KeyPoints {
			result.WriteString(fmt.Sprintf("- %s\n", point))
		}
		result.WriteString("\n")
	}

	if len(summary.Takeaways) > 0 {
		result.WriteString("## 补充说明\n\n")
		for i, takeaway := range summary.Takeaways {
			result.WriteString(fmt.Sprintf("%d. %s\n", i+1, takeaway))
		}
		result.WriteString("\n")
	}

	if len(summary.Tags) > 0 {
		result.WriteString("## 标签\n\n")
		for _, tag := range summary.Tags {
			result.WriteString(fmt.Sprintf("- %s\n", tag))
		}
	}

	return result.String()
}
