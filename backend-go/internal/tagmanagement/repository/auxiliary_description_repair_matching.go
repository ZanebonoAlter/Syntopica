package repository

// Auxiliary-description repair — MATCHING stage primitives
// (board-health-triage-advice seam B4 / task 2.4).
//
// This file owns the SQL of the matching phase: the frozen-scope reads, the
// per-item result write (item row + job counters in ONE short transaction),
// the aggregated counters (the single source: the frozen item rows) and the
// final phase state.
//
// Guard model: the matching phase runs AFTER the description was committed,
// so its guard is description_committed = TRUE (the saving-phase guard of
// task 2.3 is description_committed = false and must NOT be reused here).
// Every write is additionally guarded by (claim_owner = owner AND
// lease_expires_at > now()) — the lease is judged by the DATABASE clock.
//
// Counter model (implementation-contract.md §1): the job's processed /
// failed / not_applicable are a PROJECTION of the frozen item rows, recomputed
// inside the same transaction that writes an item result. remaining =
// total - processed - failed - not_applicable, so the four counts always add
// up to the frozen total. Nothing is ever incremented in memory.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// ErrRepairJobNotCommitted: the description was not committed yet — the
// matching phase must not run (the engine would match against the OLD
// vector).
var ErrRepairJobNotCommitted = errors.New("repair job has not committed the description yet")

// RepairScopeTagState is the live state of one frozen scope tag.
type RepairScopeTagState struct {
	// Exists: the topic_tag row is still there.
	Exists bool
	// Active: the tag is part of the active population (the SAME predicate
	// the frozen scope used — active / legacy '' / NULL; disabled and merged
	// are out).
	Active bool
}

// RepairCounters is the aggregated view of one job's frozen scope.
type RepairCounters struct {
	Total         int
	Processed     int
	Failed        int
	NotApplicable int
	Remaining     int
}

// RepairItemOutcome is the persisted result of ONE frozen scope item.
type RepairItemOutcome struct {
	JobID      uint
	Owner      string
	PhaseToken string
	TopicTagID uint
	// Status: succeeded | failed | not_applicable.
	Status string
	// Error is a short, non-sensitive reason (the raw cause is logged).
	Error string
}

// ListRepairScopeItems returns the frozen scope items that still need work:
// pending (never processed) and failed (retryable). Items already succeeded
// or recorded as not applicable are never returned — they are final.
func (r *AuxiliaryDescriptionRepairRepository) ListRepairScopeItems(ctx context.Context, jobID uint) ([]tagmodels.AuxiliaryDescriptionRepairItem, error) {
	var items []tagmodels.AuxiliaryDescriptionRepairItem
	if err := r.db.WithContext(ctx).
		Where("job_id = ? AND status IN ?", jobID, []string{
			tagmodels.AuxiliaryDescriptionRepairItemPending,
			tagmodels.AuxiliaryDescriptionRepairItemFailed,
		}).
		Order("topic_tag_id ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("load repair scope items of job %d: %w", jobID, err)
	}
	return items, nil
}

// GetScopeTagState reports whether a frozen scope tag still exists and is
// still part of the active population. The active predicate is the SAME one
// the frozen scope was built from (activeTopicTagsPredicate) — a second,
// divergent definition would silently shrink the scope.
func (r *AuxiliaryDescriptionRepairRepository) GetScopeTagState(ctx context.Context, topicTagID uint) (RepairScopeTagState, error) {
	var counts struct {
		Total  int64
		Active int64
	}
	if err := r.db.WithContext(ctx).Model(&models.TopicTag{}).
		Select("count(*) AS total, count(*) FILTER (WHERE "+activeTopicTagsPredicate+") AS active").
		Where("id = ?", topicTagID).
		Scan(&counts).Error; err != nil {
		return RepairScopeTagState{}, fmt.Errorf("load scope tag %d state: %w", topicTagID, err)
	}
	return RepairScopeTagState{Exists: counts.Total > 0, Active: counts.Active > 0}, nil
}

// RecordMatchingItemResult persists the result of one frozen scope item and
// refreshes the job counters — item row and counters in ONE short
// transaction, guarded by the executor's ownership and a live lease.
//
// The item update only touches items that are still pending or failed: a
// succeeded or not_applicable item is final and is never rewritten, so no
// item can carry two results and no counter can be counted twice.
func (r *AuxiliaryDescriptionRepairRepository) RecordMatchingItemResult(ctx context.Context, outcome RepairItemOutcome) (*RepairCounters, error) {
	switch outcome.Status {
	case tagmodels.AuxiliaryDescriptionRepairItemSucceeded,
		tagmodels.AuxiliaryDescriptionRepairItemFailed,
		tagmodels.AuxiliaryDescriptionRepairItemNotApplicable:
	default:
		return nil, fmt.Errorf("record matching item result: %q is not a final item status", outcome.Status)
	}
	var counters RepairCounters
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		job, err := r.lockOwnedMatchingJob(tx, outcome.JobID, outcome.Owner, outcome.PhaseToken)
		if err != nil {
			return err
		}
		res := tx.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).
			Where("job_id = ? AND topic_tag_id = ? AND status IN ?", outcome.JobID, outcome.TopicTagID,
				[]string{
					tagmodels.AuxiliaryDescriptionRepairItemPending,
					tagmodels.AuxiliaryDescriptionRepairItemFailed,
				}).
			Updates(map[string]any{
				"status":     outcome.Status,
				"error":      outcome.Error,
				"updated_at": gorm.Expr("now()"),
			})
		if res.Error != nil {
			return fmt.Errorf("update repair item %d/%d: %w", outcome.JobID, outcome.TopicTagID, res.Error)
		}
		counters, err = r.aggregateRepairCounters(tx, outcome.JobID, job.Total)
		if err != nil {
			return err
		}
		return r.updateJobCounters(tx, job.ID, outcome.Owner, outcome.PhaseToken, counters, nil, nil)
	})
	if err != nil {
		return nil, err
	}
	return &counters, nil
}

// FinishMatchingPhase closes the matching phase: it recomputes the counters
// from the frozen items and sets the final state.
//
//   - no failure and nothing remaining → completed (the error is cleared);
//   - at least one failure → matching_failed (retryable: only the failed and
//     unfinished items are processed again, never the embedding/save);
//   - otherwise the phase stays matching.
//
// The execution claim is released when the phase ends, so an explicitly
// failed job can be claimed again immediately. The release is owner-guarded —
// another executor's claim is never dropped.
func (r *AuxiliaryDescriptionRepairRepository) FinishMatchingPhase(ctx context.Context, jobID uint, owner, token string) (*tagmodels.AuxiliaryDescriptionRepairJob, *RepairCounters, error) {
	var (
		job      tagmodels.AuxiliaryDescriptionRepairJob
		counters RepairCounters
	)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := r.lockOwnedMatchingJob(tx, jobID, owner, token)
		if err != nil {
			return err
		}
		counters, err = r.aggregateRepairCounters(tx, locked.ID, locked.Total)
		if err != nil {
			return err
		}
		status := tagmodels.AuxiliaryDescriptionRepairStatusMatching
		message := ""
		switch {
		case counters.Failed == 0 && counters.Remaining == 0:
			status = tagmodels.AuxiliaryDescriptionRepairStatusCompleted
		case counters.Failed > 0:
			status = tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed
			message = fmt.Sprintf("matching failed for %d of %d items", counters.Failed, counters.Total)
		}
		if err := r.updateJobCounters(tx, locked.ID, owner, token, counters, &status, &message); err != nil {
			return err
		}
		// The phase right is released: the attempt ended.
		if err := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
			Where("id = ? AND claim_owner = ? AND phase_token = ?", locked.ID, owner, token).
			Update("phase_token", nil).Error; err != nil {
			return fmt.Errorf("release repair job %d phase right: %w", locked.ID, err)
		}
		if status == tagmodels.AuxiliaryDescriptionRepairStatusCompleted ||
			status == tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed {
			// Release the execution claim (owner-guarded) so the job can be
			// claimed again for a retry.
			if err := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
				Where("id = ? AND claim_owner = ?", locked.ID, owner).
				Updates(map[string]any{
					"claim_owner":      nil,
					"claimed_at":       nil,
					"lease_expires_at": nil,
					"updated_at":       gorm.Expr("now()"),
				}).Error; err != nil {
				return fmt.Errorf("release repair job %d claim: %w", locked.ID, err)
			}
		}
		if err := tx.Where("id = ?", locked.ID).First(&job).Error; err != nil {
			return fmt.Errorf("reload repair job %d: %w", locked.ID, err)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &job, &counters, nil
}

// lockOwnedMatchingJob locks the job row (FOR UPDATE) and verifies that this
// executor still owns it with a live lease AND that the description was
// already committed (the matching phase runs on the NEW vector).
func (r *AuxiliaryDescriptionRepairRepository) lockOwnedMatchingJob(tx *gorm.DB, jobID uint, owner, token string) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, ErrRepairOwnerRequired
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrRepairPhaseTokenRequired
	}
	var job tagmodels.AuxiliaryDescriptionRepairJob
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", jobID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("lock repair job %d: %w", jobID, err)
	}
	if !job.DescriptionCommitted {
		return nil, ErrRepairJobNotCommitted
	}
	if job.ClaimOwner == nil || *job.ClaimOwner != owner {
		return nil, ErrRepairJobOwnerLost
	}
	if job.PhaseToken == nil || *job.PhaseToken != token {
		return nil, ErrRepairJobOwnerLost
	}
	// The lease is judged by the database's REAL-TIME clock (review R1):
	// now() is frozen at the transaction start and would let a transaction
	// that waited on a row lock act after its lease had expired.
	var dbNow time.Time
	if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&dbNow); err != nil {
		return nil, fmt.Errorf("read database clock: %w", err)
	}
	if job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.After(dbNow) {
		return nil, ErrRepairJobOwnerLost
	}
	return &job, nil
}

// aggregateRepairCounters recomputes the counters from the frozen item rows —
// the single source of truth. remaining is derived from the FROZEN total, so
// processed + failed + not_applicable + remaining always equals total.
func (r *AuxiliaryDescriptionRepairRepository) aggregateRepairCounters(tx *gorm.DB, jobID uint, total int) (RepairCounters, error) {
	var agg struct {
		Succeeded     int64
		Failed        int64
		NotApplicable int64
	}
	if err := tx.Model(&tagmodels.AuxiliaryDescriptionRepairItem{}).
		Select("count(*) FILTER (WHERE status = ?) AS succeeded, "+
			"count(*) FILTER (WHERE status = ?) AS failed, "+
			"count(*) FILTER (WHERE status = ?) AS not_applicable",
			tagmodels.AuxiliaryDescriptionRepairItemSucceeded,
			tagmodels.AuxiliaryDescriptionRepairItemFailed,
			tagmodels.AuxiliaryDescriptionRepairItemNotApplicable).
		Where("job_id = ?", jobID).
		Scan(&agg).Error; err != nil {
		return RepairCounters{}, fmt.Errorf("aggregate repair counters: %w", err)
	}
	counters := RepairCounters{
		Total:         total,
		Processed:     int(agg.Succeeded),
		Failed:        int(agg.Failed),
		NotApplicable: int(agg.NotApplicable),
	}
	remaining := total - counters.Processed - counters.Failed - counters.NotApplicable
	if remaining < 0 {
		remaining = 0
	}
	counters.Remaining = remaining
	return counters, nil
}

// updateJobCounters writes the aggregated counters onto the job row.
// status nil leaves the column untouched; a non-nil pointer sets it.
// errorMessage nil leaves the column untouched; a non-nil pointer sets it (an
// empty string CLEARS the previous error — a successful phase must not keep
// an old failure message).
//
// The write is guarded by the owner, the phase token and the REAL-TIME lease
// condition (review R1): a lost ownership or an expired lease rolls the whole
// transaction back — including the item result written next to it.
func (r *AuxiliaryDescriptionRepairRepository) updateJobCounters(tx *gorm.DB, jobID uint, owner, token string, counters RepairCounters, status *string, errorMessage *string) error {
	updates := map[string]any{
		"processed":      counters.Processed,
		"failed":         counters.Failed,
		"not_applicable": counters.NotApplicable,
		"stage":          tagmodels.AuxiliaryDescriptionRepairStageMatching,
		"updated_at":     gorm.Expr("now()"),
	}
	if status != nil {
		updates["status"] = *status
	}
	if errorMessage != nil {
		updates["error"] = *errorMessage
	}
	res := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
		Where("id = ? AND claim_owner = ? AND phase_token = ? AND lease_expires_at IS NOT NULL AND lease_expires_at > clock_timestamp() AND description_committed = ?",
			jobID, owner, token, true).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update repair job %d counters: %w", jobID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRepairJobOwnerLost
	}
	return nil
}
