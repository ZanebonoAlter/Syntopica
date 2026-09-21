package scheduler

import (
	"context"
	"fmt"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

const blockedArticleThreshold = 50

// BlockedArticleRecoveryJob recovers articles stuck in blocked/firecrawl waiting states.
func BlockedArticleRecoveryJob(ctx context.Context) (*JobResult, error) {
	var blockedArticles []models.Article
	err := repository.Repo.DB().
		Joins("JOIN feeds ON feeds.id = articles.feed_id").
		Where("articles.firecrawl_status IN ?", []string{"waiting_for_firecrawl", "blocked"}).
		Find(&blockedArticles).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query blocked articles: %w", err)
	}

	recoveredCount := 0
	for _, article := range blockedArticles {
		var feed models.Feed
		if err := repository.Repo.DB().First(&feed, article.FeedID).Error; err != nil {
			// feed deleted, skip
			continue
		}

		if feed.FirecrawlEnabled {
			result := repository.Repo.DB().Model(&models.Article{}).
				Where("id = ?", article.ID).
				Update("firecrawl_status", "pending")

			if result.Error == nil && result.RowsAffected > 0 {
				recoveredCount++
				logging.Infof("BlockedArticleRecovery: recovered article %d from feed %d", article.ID, feed.ID)
			}
		}
	}

	// STAT-05: Blocked count warning
	var blockedCount int64
	err = repository.Repo.DB().Model(&models.Article{}).
		Joins("JOIN feeds ON feeds.id = articles.feed_id").
		Where("articles.summary_status = ?", "incomplete").
		Where("feeds.article_summary_enabled = ?", true).
		Where("feeds.completion_on_refresh = ?", true).
		Where("articles.firecrawl_status <> ?", "completed").
		Count(&blockedCount).Error
	if err != nil {
		logging.Errorf("BlockedArticleRecovery: failed to count blocked articles: %v", err)
	} else if blockedCount > blockedArticleThreshold {
		logging.Warnf("ContentCompletion blocked articles exceeded threshold: %d > %d", blockedCount, blockedArticleThreshold)
	}

	return &JobResult{
		Data: map[string]interface{}{
			"recovered_count": recoveredCount,
		},
		Summary: fmt.Sprintf("recovered %d articles", recoveredCount),
	}, nil
}
