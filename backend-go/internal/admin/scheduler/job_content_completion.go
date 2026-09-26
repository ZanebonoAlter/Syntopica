package scheduler

import (
	"context"
	"fmt"
	"time"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/scheduler"
	content "syntopica-backend/internal/reader"
)

// ContentCompletionJob runs the content completion cycle: fetches pending
// articles and completes them one by one using the AI service.
func ContentCompletionJob(completionService *content.ContentCompletionService) scheduler.JobFunc {
	return func(ctx context.Context) (*scheduler.JobResult, error) {
		startTime := time.Now()

		// Check if AI is configured
		overview, err := completionService.GetOverview()
		if err != nil {
			return nil, fmt.Errorf("failed to get completion overview: %w", err)
		}
		_ = overview

		articles, err := completionService.ListReadyArticles(50)
		if err != nil {
			return nil, fmt.Errorf("failed to list ready articles: %w", err)
		}

		if len(articles) == 0 {
			return &scheduler.JobResult{
				Data:    map[string]interface{}{},
				Summary: "no pending content to complete",
			}, nil
		}

		completedCount := 0
		failedCount := 0

		for _, article := range articles {
			if err := completionService.CompleteArticle(ctx, article.ID); err != nil {
				logging.Warnf("ContentCompletion: article %d failed: %v", article.ID, err)
				failedCount++
			} else {
				completedCount++
			}
		}

		return &scheduler.JobResult{
			Data: map[string]interface{}{
				"completed_count": completedCount,
				"failed_count":    failedCount,
				"total":           len(articles),
				"started_at":      startTime.Format(time.RFC3339),
				"finished_at":     time.Now().Format(time.RFC3339),
				"reason":          "completed",
			},
			Summary: fmt.Sprintf("completed=%d failed=%d total=%d", completedCount, failedCount, len(articles)),
		}, nil
	}
}
