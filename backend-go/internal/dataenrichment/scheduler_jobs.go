package dataenrichment

import (
	"context"
	"fmt"
	"time"

	"syntopica-backend/internal/admin/scheduler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// WeeklyLifelineJob creates a scheduler JobFunc that:
// 1. Runs HealMissing for week granularity (fill missing weeks).
// 2. Refreshes all active topics' week context for the current week.
// 3. Prunes week rows older than 8 weeks.
func WeeklyLifelineJob(svc *service.LifelineContextService, lister ActiveTopicLister) scheduler.JobFunc {
	return func(ctx context.Context) (*scheduler.JobResult, error) {
		gran := string(repository.GranularityWeek)
		now := time.Now()
		startedAt := now

		// Step 1: self-heal missing periods.
		if err := svc.HealMissing(ctx, gran, now, lister); err != nil {
			return nil, fmt.Errorf("weekly lifeline: heal missing: %w", err)
		}

		// Step 2: refresh all active topics for current week.
		topics, err := lister.ListActiveTopicIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("weekly lifeline: list active topics: %w", err)
		}

		refreshed, failed := 0, 0
		for _, tid := range topics {
			if err := svc.RefreshWeek(ctx, tid, now); err != nil {
				failed++
				continue
			}
			refreshed++
		}

		// Step 3: archive prune.
		if err := svc.ArchivePrune(ctx, gran, now); err != nil {
			return nil, fmt.Errorf("weekly lifeline: archive prune: %w", err)
		}

		return &scheduler.JobResult{
			Data: map[string]interface{}{
				"refreshed":    refreshed,
				"failed":       failed,
				"total_topics": len(topics),
				"granularity":  gran,
				"started_at":   startedAt.Format(time.RFC3339),
				"finished_at":  time.Now().Format(time.RFC3339),
			},
			Summary: fmt.Sprintf("lifeline week: healed + refreshed %d/%d topics + pruned", refreshed, len(topics)),
		}, nil
	}
}

// MonthlyLifelineJob creates a scheduler JobFunc for monthly lifeline context.
func MonthlyLifelineJob(svc *service.LifelineContextService, lister ActiveTopicLister) scheduler.JobFunc {
	return func(ctx context.Context) (*scheduler.JobResult, error) {
		gran := string(repository.GranularityMonth)
		now := time.Now()
		startedAt := now

		if err := svc.HealMissing(ctx, gran, now, lister); err != nil {
			return nil, fmt.Errorf("monthly lifeline: heal missing: %w", err)
		}

		topics, err := lister.ListActiveTopicIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("monthly lifeline: list active topics: %w", err)
		}

		refreshed, failed := 0, 0
		for _, tid := range topics {
			if err := svc.RefreshMonth(ctx, tid, now); err != nil {
				failed++
				continue
			}
			refreshed++
		}

		if err := svc.ArchivePrune(ctx, gran, now); err != nil {
			return nil, fmt.Errorf("monthly lifeline: archive prune: %w", err)
		}

		return &scheduler.JobResult{
			Data: map[string]interface{}{
				"refreshed":    refreshed,
				"failed":       failed,
				"total_topics": len(topics),
				"granularity":  gran,
				"started_at":   startedAt.Format(time.RFC3339),
				"finished_at":  time.Now().Format(time.RFC3339),
			},
			Summary: fmt.Sprintf("lifeline month: healed + refreshed %d/%d topics + pruned", refreshed, len(topics)),
		}, nil
	}
}

// YearlyLifelineJob creates a scheduler JobFunc for yearly lifeline context.
func YearlyLifelineJob(svc *service.LifelineContextService, lister ActiveTopicLister) scheduler.JobFunc {
	return func(ctx context.Context) (*scheduler.JobResult, error) {
		gran := string(repository.GranularityYear)
		now := time.Now()
		startedAt := now

		if err := svc.HealMissing(ctx, gran, now, lister); err != nil {
			return nil, fmt.Errorf("yearly lifeline: heal missing: %w", err)
		}

		topics, err := lister.ListActiveTopicIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("yearly lifeline: list active topics: %w", err)
		}

		refreshed, failed := 0, 0
		for _, tid := range topics {
			if err := svc.RefreshYear(ctx, tid, now); err != nil {
				failed++
				continue
			}
			refreshed++
		}

		// Year is not pruned per design §2.1.

		return &scheduler.JobResult{
			Data: map[string]interface{}{
				"refreshed":    refreshed,
				"failed":       failed,
				"total_topics": len(topics),
				"granularity":  gran,
				"started_at":   startedAt.Format(time.RFC3339),
				"finished_at":  time.Now().Format(time.RFC3339),
			},
			Summary: fmt.Sprintf("lifeline year: healed + refreshed %d/%d topics", refreshed, len(topics)),
		}, nil
	}
}

// RelationExpireJob batch-marks confirmed cross-board relations whose
// expires_at has passed as expired (add-evidence-backed-cross-board-relations
// 4.3). Read paths already treat stale rows as expired — this only persists
// the state; failures are logged and never fail the scheduler round.
func RelationExpireJob(repo *repository.Repository) scheduler.JobFunc {
	return func(ctx context.Context) (*scheduler.JobResult, error) {
		n, err := repo.ExpireConfirmedRelations(ctx)
		if err != nil {
			return nil, fmt.Errorf("relation expire: %w", err)
		}
		return &scheduler.JobResult{
			Data: map[string]interface{}{
				"expired":     n,
				"finished_at": time.Now().Format(time.RFC3339),
			},
			Summary: fmt.Sprintf("expired %d stale confirmed cross-board relations", n),
		}, nil
	}
}
