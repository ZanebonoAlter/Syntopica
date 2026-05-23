package content

import (
	"context"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func (s *ContentCompletionService) AutoCompleteCompletePendingArticles(limit int) ([]uint, []error) {
	var articles []models.Article

	err := database.DB.
		Joins("Feed").
		Where("articles.summary_status = ? AND feeds.article_summary_enabled = ?", "incomplete", true).
		Where("articles.completion_attempts < feeds.max_completion_retries").
		Omit("tag_count", "relevance_score").
		Limit(limit).
		Find(&articles).Error

	if err != nil {
		return nil, []error{err}
	}

	var completedIDs []uint
	var errors []error

	for _, article := range articles {
		if err := s.CompleteArticle(context.Background(), article.ID); err != nil {
			errors = append(errors, err)
		} else {
			completedIDs = append(completedIDs, article.ID)
		}
	}

	return completedIDs, errors
}
