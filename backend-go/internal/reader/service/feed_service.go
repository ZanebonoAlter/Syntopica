package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	otelCodes "go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/reader/repository"
	tagging "syntopica-backend/internal/tagmanagement"
)

type FeedService struct {
	rssParser *RSSParser
	iconStore *IconStore
}

func NewFeedService() *FeedService {
	return &FeedService{
		rssParser: NewRSSParser(),
		iconStore: DefaultIconStore(),
	}
}

func (s *FeedService) RefreshFeed(ctx context.Context, feedID uint) (err error) {
	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "FeedService.RefreshFeed")
	defer span.End()
	defer func() {
		if err != nil {
			span.SetStatus(otelCodes.Error, "error")
			span.RecordError(err)
		}
	}()
	/*line backend-go/internal/domain/feeds/service.go:26:2*/ var feed models.Feed
	if err := repository.Repo.DB().First(&feed, feedID).Error; err != nil {
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

	now := time.Now().In(models.ShanghaiTZ)
	feed.Title = parsed.Title
	feed.Description = parsed.Description
	feed.LastUpdated = &now
	feed.LastRefreshAt = &now
	feed.RefreshStatus = "success"
	feed.RefreshError = ""

	if newIcon, newSource, ok := s.resolveFeedIcon(feed.ID, feed.Icon, feed.IconSource, parsed.Image, parsed.Link); ok {
		feed.Icon = newIcon
		feed.IconSource = newSource
	}

	var existingTitles []string
	repository.Repo.DB().Model(&models.Article{}).
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
			now := time.Now().In(models.ShanghaiTZ)
			article.PubDate = &now
		}

		if err := repository.Repo.DB().Create(&article).Error; err != nil {
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

	if err := repository.Repo.DB().Save(&feed).Error; err != nil {
		return err
	}

	return nil
}

func (s *FeedService) enqueueArticleProcessing(feed models.Feed, article models.Article) error {
	if feed.FirecrawlEnabled {
		return repository.NewFirecrawlJobQueue(repository.Repo.DB()).Enqueue(article)
	}

	if !feed.TaggingEnabled {
		return nil
	}

	return tagging.NewTagJobQueue(repository.Repo.DB()).Enqueue(tagging.TagJobRequest{
		ArticleID:    article.ID,
		FeedName:     feed.Title,
		CategoryName: tagging.FeedCategoryName(feed),
		Reason:       "article_created",
	})
}

func (s *FeedService) updateFeedError(feed *models.Feed, err error) {
	now := time.Now().In(models.ShanghaiTZ)
	feed.RefreshStatus = "error"
	feed.RefreshError = err.Error()
	feed.LastRefreshAt = &now
	repository.Repo.DB().Save(feed)
}

func (s *FeedService) CleanupOldArticles(feed *models.Feed) {
	maxArticles := feed.MaxArticles
	if maxArticles <= 0 || maxArticles >= 9999 {
		return
	}

	// Only active articles count against the window — archived rows must not
	// erode it (design D4), otherwise every refresh would archive fresh
	// articles while the feed fills up with historical archived rows.
	var articleCount int64
	repository.Repo.DB().Model(&models.Article{}).
		Where("feed_id = ? AND archived = ?", feed.ID, false).
		Count(&articleCount)

	logging.Infof("[cleanup] feed %d: max=%d, active=%d", feed.ID, maxArticles, articleCount)

	if int(articleCount) <= maxArticles {
		logging.Infof("[cleanup] feed %d: skip, active article count within limit", feed.ID)
		return
	}

	var allArticles []struct {
		ID       uint
		Favorite bool
	}
	repository.Repo.DB().Model(&models.Article{}).
		Select("id, favorite").
		Where("feed_id = ? AND archived = ?", feed.ID, false).
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

	logging.Infof("[cleanup] feed %d: keep=%d (favorite), archive candidates=%d", feed.ID, len(keepIDs), len(candidates))

	remaining := maxArticles - len(keepIDs)
	if len(candidates) > 0 {
		toArchive := candidates
		if remaining > 0 && len(candidates) > remaining {
			toArchive = candidates[remaining:]
		}
		if len(toArchive) > 0 {
			logging.Infof("[cleanup] feed %d: archiving %d articles, IDs=%v", feed.ID, len(toArchive), toArchive)

			// Behaviors go away; the row, its text fields and its
			// article_topic_tags edges stay (edges are reclaimed by the
			// time-window GC, not by archiving).
			repository.Repo.DB().Where("article_id IN ?", toArchive).Delete(&models.ReadingBehavior{})

			// Archive the rows. search_vector is Postgres-only (tsvector) —
			// sqlite test DBs lack the column, so guard the NULL assignment.
			updates := map[string]interface{}{"archived": true}
			if repository.Repo.DB().Migrator().HasColumn(&models.Article{}, "search_vector") {
				updates["search_vector"] = nil
			}
			repository.Repo.DB().Model(&models.Article{}).Where("id IN ?", toArchive).Updates(updates)
		} else {
			logging.Infof("[cleanup] feed %d: no articles to archive", feed.ID)
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
		FirecrawlStatus: "completed",
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

// localIconPresent reports whether the local icon file backing a DB-stored
// "/icons/..." path is still on disk. An unwired store (partial construction)
// keeps the legacy freeze semantics rather than risking a nil dereference in
// the download pipeline.
func (s *FeedService) localIconPresent(localIconPath string) bool {
	if s.iconStore == nil {
		return true
	}
	return s.iconStore.LocalIconExists(localIconPath)
}

// resolveFeedIcon applies the icon source state machine and the candidate
// download pipeline to decide whether and how to recompute a feed's icon.
//
// Skip rules:
//   - custom (or any non-empty non-auto/fallback source) → frozen, never touched;
//   - auto + icon already a local /icons/ path **whose file still exists on
//     disk** → frozen (a good downloaded icon must not be clobbered by a
//     transient remote failure: no download, no homepage probe);
//   - auto + local /icons/ path whose file is gone (icon dir wiped, or the DB
//     was restored from a dump without the runtime files) → the pipeline runs
//     again to heal the dangling path;
//   - auto + still-remote icon (legacy unlocalized data) → pipeline runs to
//     complete localization;
//   - fallback / empty (legacy rows) → pipeline runs.
//
// Candidate order (each verified by actually downloading): parsed.Image (RSS
// <image>) → homepage <link rel="icon"> href → {host}/favicon.ico guess. The
// homepage probe only runs when the RSS image is absent or failed to download.
// The first candidate that downloads successfully is stored locally and
// returned as icon=/icons/feeds/<id>.<ext>, icon_source=auto. When every
// candidate fails the feed stays fallback (mdi:rss) — icon failures never fail
// the refresh.
//
// Returns (icon, iconSource, ok): ok=false means the icon must be left
// untouched (custom, or auto with a still-present localized icon).
func (s *FeedService) resolveFeedIcon(feedID uint, currentIcon, currentSource, parsedImage, siteLink string) (icon, source string, ok bool) {
	if currentSource != "auto" && currentSource != "fallback" && currentSource != "" {
		return "", "", false // custom (or unknown): do not touch
	}
	if currentSource == "auto" && strings.HasPrefix(currentIcon, "/icons/") && s.localIconPresent(currentIcon) {
		return "", "", false // auto + already-localized (file on disk): skip the whole pipeline
	}
	if parsedImage != "" {
		if local, err := s.iconStore.SaveFeedIcon(feedID, parsedImage); err == nil {
			return local, "auto", true
		} else {
			logging.Infof("icon: RSS image download failed for feed %d, trying next candidate: %v", feedID, err)
		}
	}
	// RSS image absent or failed: probe the site homepage once, then guess.
	// Reached both by fallback/legacy rows and by auto rows whose local file is
	// missing (self-healing the dangling /icons/ path).
	for _, candidate := range s.rssParser.ProbeFaviconCandidates(siteLink) {
		if candidate == "" {
			continue
		}
		if local, err := s.iconStore.SaveFeedIcon(feedID, candidate); err == nil {
			return local, "auto", true
		} else {
			logging.Infof("icon: candidate %q download failed for feed %d: %v", candidate, feedID, err)
		}
	}
	return "mdi:rss", "fallback", true
}
