package main

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/topicgraph/repository"
)

type rebuildSignalStats struct {
	Selected, Rebuilt int
	Signals           int64
}

// The report window selects story IDs, not evidence: selected stories are
// rebuilt from ALL their threads, including dates outside the selection window.
// Each story commits atomically; rerunning after a partial failure is safe.
func rebuildStorySignals(ctx context.Context, db *gorm.DB, opts commandOptions) (rebuildSignalStats, error) {
	stats := rebuildSignalStats{}
	if opts.From.IsZero() || opts.To.IsZero() || opts.From.After(opts.To) {
		return stats, fmt.Errorf("rebuild signals: require nonzero dates and from <= to")
	}
	db = db.WithContext(ctx)
	reports := db.Model(&repository.BoardDailyReport{}).
		Select("id").Where("period_date >= ? AND period_date < ?", opts.From.Format(time.DateOnly), opts.To.AddDate(0, 0, 1).Format(time.DateOnly))
	if opts.BoardID != nil {
		reports = reports.Where("semantic_board_id = ?", *opts.BoardID)
	}
	ids := db.Model(&repository.DailyReportThread{}).Select("story_id").
		Where("report_id IN (?) AND story_id IS NOT NULL", reports)
	var stories []repository.BoardStory
	if err := db.Select("id", "merged_into").Where("id IN (?)", ids).Order("id").Find(&stories).Error; err != nil {
		return stats, fmt.Errorf("select stories to rebuild: %w", err)
	}
	stats.Selected = len(stories)
	if opts.DryRun {
		return stats, nil
	}
	for _, story := range stories {
		var signals int64
		err := db.Transaction(func(tx *gorm.DB) error {
			repo := repository.NewTopicGraphRepository(tx)
			if err := repo.RecomputeSignalsForStory(story.ID); err != nil {
				return err
			}
			if err := repo.RecomputeBoardStoryCounts(story.ID); err != nil {
				return err
			}
			// Protected threads can still point to merged-out stories. Refresh the
			// survivor's participant union too, without moving any thread or signal.
			if story.MergedInto != nil {
				if err := repo.RecomputeBoardStoryCounts(*story.MergedInto); err != nil {
					return err
				}
			}
			return tx.Model(&repository.StorySignal{}).Where("story_id = ?", story.ID).Count(&signals).Error
		})
		if err != nil {
			return stats, fmt.Errorf("rebuild story %d signals: %w", story.ID, err)
		}
		stats.Rebuilt++
		stats.Signals += signals
	}
	return stats, nil
}
