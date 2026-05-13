package feed

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	otelCodes "go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
	"my-robot-backend/internal/domain/content"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/tagging"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type FeedService struct {
	rssParser *RSSParser
}

func NewFeedService() *FeedService {
	return &FeedService{
		rssParser: NewRSSParser(),
	}
}

func (s *FeedService) RefreshFeed(ctx context.Context, feedID uint) (err error) {
	_, span := otel.Tracer("rss-reader-backend").Start(ctx, "FeedService.RefreshFeed")
	defer span.End()
	defer func() {
		if err != nil {
			span.SetStatus(otelCodes.Error, "error")
			span.RecordError(err)
		}
	}()
	/*line backend-go/internal/domain/feeds/service.go:26:2*/ var feed models.Feed
	if err := database.DB.First(&feed, feedID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("feed not found")
		}
		return err
	}

	parsed, err := s.rssParser.ParseFeedURL(feed.URL)
	if err != nil {
		s.updateFeedError(&feed, err)
		return err
	}

	now := time.Now().In(time.FixedZone("CST", 8*3600))
	feed.Title = parsed.Title
	feed.Description = parsed.Description
	feed.LastUpdated = &now
	feed.LastRefreshAt = &now
	feed.RefreshStatus = "success"
	feed.RefreshError = ""

	var firstArticleImage string
	for _, entry := range parsed.Entries {
		if entry.ImageURL != "" && firstArticleImage == "" {
			firstArticleImage = entry.ImageURL
			break
		}
	}

	if feed.Icon == "" || feed.Icon == "rss" || feed.Icon == "mdi:rss" {
		switch {
		case parsed.Image != "":
			feed.Icon = parsed.Image
		case firstArticleImage != "":
			feed.Icon = firstArticleImage
		default:
			feed.Icon = s.rssParser.FetchFaviconURL(feed.URL)
		}
	}

	var existingTitles []string
	database.DB.Model(&models.Article{}).
		Where("feed_id = ?", feed.ID).
		Pluck("title", &existingTitles)
	titleSet := make(map[string]bool, len(existingTitles))
	for _, t := range existingTitles {
		titleSet[t] = true
	}

	articlesAdded := 0
	for _, entry := range parsed.Entries {
		if entry.Link == "" {
			continue
		}

		if titleSet[entry.Title] {
			continue
		}

		article := s.buildArticleFromEntry(feed, entry)

		if article.PubDate == nil {
			now := time.Now().In(time.FixedZone("CST", 8*3600))
			article.PubDate = &now
		}

		if err := database.DB.Create(&article).Error; err != nil {
			continue
		}

		titleSet[entry.Title] = true

		if err := s.enqueueArticleProcessing(feed, article); err != nil {
			logging.Errorf("Error enqueueing processing for article %d (feed %d): %v", article.ID, feed.ID, err)
		}

		articlesAdded++
		if feed.MaxArticles > 0 && articlesAdded >= feed.MaxArticles {
			break
		}
	}

	s.CleanupOldArticles(&feed)

	if err := database.DB.Save(&feed).Error; err != nil {
		return err
	}

	return nil
}

func (s *FeedService) enqueueArticleProcessing(feed models.Feed, article models.Article) error {
	if feed.FirecrawlEnabled {
		return content.NewFirecrawlJobQueue(database.DB).Enqueue(article)
	}

	if !feed.TaggingEnabled {
		return nil
	}

	return tagging.NewTagJobQueue(database.DB).Enqueue(tagging.TagJobRequest{
		ArticleID:    article.ID,
		FeedName:     feed.Title,
		CategoryName: tagging.FeedCategoryName(feed),
		Reason:       "article_created",
	})
}

func (s *FeedService) updateFeedError(feed *models.Feed, err error) {
	now := time.Now().In(time.FixedZone("CST", 8*3600))
	feed.RefreshStatus = "error"
	feed.RefreshError = err.Error()
	feed.LastRefreshAt = &now
	database.DB.Save(feed)
}

func (s *FeedService) CleanupOldArticles(feed *models.Feed) {
	maxArticles := feed.MaxArticles
	if maxArticles <= 0 || maxArticles >= 9999 {
		return
	}

	var articleCount int64
	database.DB.Model(&models.Article{}).Where("feed_id = ?", feed.ID).Count(&articleCount)

	logging.Infof("[cleanup] feed %d: max=%d, current=%d", feed.ID, maxArticles, articleCount)

	if int(articleCount) <= maxArticles {
		logging.Infof("[cleanup] feed %d: skip, article count within limit", feed.ID)
		return
	}

	var allArticles []struct {
		ID              uint
		Favorite        bool
		FirecrawlStatus string
		SummaryStatus   string
	}
	database.DB.Model(&models.Article{}).
		Select("id, favorite, firecrawl_status, summary_status").
		Where("feed_id = ?", feed.ID).
		Order("pub_date DESC").
		Find(&allArticles)

	var keepIDs []uint
	candidates := make([]uint, 0)

	for _, a := range allArticles {
		if a.Favorite {
			keepIDs = append(keepIDs, a.ID)
		} else {
			candidates = append(candidates, a.ID)
		}
	}

	logging.Infof("[cleanup] feed %d: keep=%d (favorite), candidates=%d", feed.ID, len(keepIDs), len(candidates))

	remaining := maxArticles - len(keepIDs)
	if len(candidates) > 0 {
		toDelete := candidates
		if remaining > 0 && len(candidates) > remaining {
			toDelete = candidates[remaining:]
		}
		if len(toDelete) > 0 {
			logging.Infof("[cleanup] feed %d: deleting %d articles, IDs=%v", feed.ID, len(toDelete), toDelete)
			database.DB.Where("article_id IN (SELECT id FROM articles WHERE feed_id = ? AND id IN ?)", feed.ID, toDelete).Delete(&models.ReadingBehavior{})
			database.DB.Where("feed_id = ? AND id IN ?", feed.ID, toDelete).Delete(&models.Article{})
		} else {
			logging.Infof("[cleanup] feed %d: no articles to delete", feed.ID)
		}
	}
}

func (s *FeedService) FetchFeedPreview(feedURL string) (title, description string, err error) {
	return s.rssParser.FetchFeedMetadata(feedURL)
}

func (s *FeedService) buildArticleFromEntry(feed models.Feed, entry ParsedEntry) models.Article {
	article := models.Article{
		FeedID:          feed.ID,
		Title:           entry.Title,
		Description:     entry.Description,
		Content:         entry.Content,
		Link:            entry.Link,
		ImageURL:        entry.ImageURL,
		PubDate:         entry.PubDate,
		Author:          entry.Author,
		SummaryStatus:   "complete",
		FirecrawlStatus: "complete",
	}

	if feed.FirecrawlEnabled {
		article.FirecrawlStatus = "pending"
		if feed.ArticleSummaryEnabled {
			article.SummaryStatus = "incomplete"
		}
	} else if feed.ArticleSummaryEnabled {
		article.SummaryStatus = "pending"
	}

	return article
}
