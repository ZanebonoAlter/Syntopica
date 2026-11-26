package repository

// Auxiliary-description repair — EXECUTOR primitives (task 2.5).
//
// This file owns the SQL of the safe execution loop: the claimable-job scan
// (a short transaction with FOR UPDATE SKIP LOCKED, so several executor
// instances never claim the same job), the lease renewal (heartbeat) and the
// explicit re-queue of a failed or interrupted task.
//
// Boundaries:
//   - the queue is the DATABASE. There is no in-memory map, no process-local
//     job list and no persisted "queue" table — the jobs themselves are the
//     queue;
//   - the scanner claims ONLY a fresh task (pending) or an unfinished phase
//     whose lease has expired (the previous attempt died). The explicit
//     failure states (embedding_failed / save_failed / matching_failed) are
//     NOT claimable — they wait for the user's explicit retry, so a failure
//     never burns AI quota in a loop;
//   - the terminal states (completed / conflict / unavailable) are never
//     scanned;
//   - every lease decision uses the DATABASE's real-time clock
//     (clock_timestamp()) — never now()/statement_timestamp(), which are
//     frozen at the transaction/statement start.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// ErrRepairNoClaimableJob: the scan found nothing to do (idle, not an error).
var ErrRepairNoClaimableJob = errors.New("no claimable description-repair job")

// RepairRequeueOutcome is the result of an explicit retry.
type RepairRequeueOutcome string

const (
	// RepairRequeueRequeued: the task was re-queued for another attempt.
	RepairRequeueRequeued RepairRequeueOutcome = "requeued"
	// RepairRequeueNoop: the task is running or already completed — the retry
	// changed NOTHING (no re-queue, no new worker, no counter/time change).
	RepairRequeueNoop RepairRequeueOutcome = "noop"
	// RepairRequeueConflict: the task ended in conflict/unavailable — it
	// cannot be retried as-is; the user must re-check the latest version.
	RepairRequeueConflict RepairRequeueOutcome = "conflict"
)

// claimableRepairJobsPredicate is the scanner's filter: a fresh task, or an
// unfinished phase whose lease has expired. The lease comparison uses the
// database's real-time clock.
const claimableRepairJobsPredicate = `(
	(status = ? AND (claim_owner IS NULL OR lease_expires_at IS NULL OR lease_expires_at <= clock_timestamp()))
	OR
	(status IN ? AND (lease_expires_at IS NULL OR lease_expires_at <= clock_timestamp()))
)`

// leaseExpiryExpr builds the SQL expression of a lease deadline computed by
// the DATABASE clock: clock_timestamp() + the given duration.
//
// The application clock must never decide the lease deadline: a lagging
// application clock would write a lease that is already expired (the guards
// judge with clock_timestamp()), and the worker would then re-claim the same
// task in a hot loop (review fix 2). The duration is passed as a parameter —
// no string concatenation of user input.
func leaseExpiryExpr(lease time.Duration) clause.Expr {
	return gorm.Expr("clock_timestamp() + make_interval(secs => ?)", lease.Seconds())
}

// ClaimNextRepairJob atomically claims the next claimable job for an
// executor.
//
// The scan and the claim share ONE short transaction with FOR UPDATE SKIP
// LOCKED: with several executor instances only one of them can take a given
// job, and the others skip it instead of blocking.
//
// The claim writes a FRESH owner token (the caller mints a unique one per
// execution) and voids the previous attempt's phase token — a takeover after
// an expired lease must not leave the stale attempt able to commit.
//
// Returns ErrRepairNoClaimableJob when there is nothing to do.
func (r *AuxiliaryDescriptionRepairRepository) ClaimNextRepairJob(ctx context.Context, owner string, lease time.Duration) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, ErrRepairOwnerRequired
	}
	var job tagmodels.AuxiliaryDescriptionRepairJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidate tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(claimableRepairJobsPredicate,
				tagmodels.AuxiliaryDescriptionRepairStatusPending,
				[]string{
					tagmodels.AuxiliaryDescriptionRepairStatusEmbedding,
					tagmodels.AuxiliaryDescriptionRepairStatusSaving,
					tagmodels.AuxiliaryDescriptionRepairStatusMatching,
				}).
			Order("id ASC").
			Limit(1).
			Find(&candidate).Error; err != nil {
			return fmt.Errorf("scan claimable repair jobs: %w", err)
		}
		if candidate.ID == 0 {
			return ErrRepairNoClaimableJob
		}
		if err := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
			Where("id = ?", candidate.ID).
			Updates(map[string]any{
				"claim_owner": owner,
				"claimed_at":  gorm.Expr("now()"),
				// The DATABASE clock decides the deadline (review fix 2).
				"lease_expires_at": leaseExpiryExpr(lease),
				"phase_token":      nil,
				"updated_at":       gorm.Expr("now()"),
			}).Error; err != nil {
			return fmt.Errorf("claim repair job %d: %w", candidate.ID, err)
		}
		return tx.Where("id = ?", candidate.ID).First(&job).Error
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// RenewRepairLease extends the lease of the current owner (the heartbeat).
//
// The extension is allowed ONLY while this owner still holds the lease and
// the lease has not expired yet — an expired lease is never revived, and the
// phase token, the status and the counters are never touched. A lost
// ownership is reported as ErrRepairJobOwnerLost.
func (r *AuxiliaryDescriptionRepairRepository) RenewRepairLease(ctx context.Context, jobID uint, owner string, lease time.Duration) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return ErrRepairOwnerRequired
	}
	res := r.db.WithContext(ctx).Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
		Where("id = ? AND claim_owner = ? AND lease_expires_at IS NOT NULL AND lease_expires_at > clock_timestamp()",
			jobID, owner).
		// The DATABASE clock decides the deadline (review fix 2).
		Update("lease_expires_at", leaseExpiryExpr(lease))
	if res.Error != nil {
		return fmt.Errorf("renew lease of repair job %d: %w", jobID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRepairJobOwnerLost
	}
	return nil
}

// RequeueRepairJob re-queues a task for another explicit attempt.
//
// The decision is taken in ONE short transaction (the job row is locked):
//
//   - not found → gorm.ErrRecordNotFound (404);
//   - conflict / unavailable → RepairRequeueConflict (409, the user must
//     re-check the latest version);
//   - completed, or running with a LIVE lease → RepairRequeueNoop: NOTHING
//     is written — no re-queue, no new worker, no counter or time change;
//   - an explicit failure state, or an unfinished phase whose lease expired
//     (an interrupted attempt) → RepairRequeueRequeued.
//
// The re-queue re-uses the SAME task: the confirmed description, the
// expected version and the frozen scope are untouched; only the phase state
// changes — the old error is cleared, the old execution right (phase token
// and claim) is voided, and the task is put back into the stage it must
// continue from: the matching stage when the description was already
// committed, otherwise the embedding stage.
func (r *AuxiliaryDescriptionRepairRepository) RequeueRepairJob(ctx context.Context, jobID uint) (*tagmodels.AuxiliaryDescriptionRepairJob, RepairRequeueOutcome, error) {
	var job tagmodels.AuxiliaryDescriptionRepairJob
	outcome := RepairRequeueNoop
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", jobID).First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return fmt.Errorf("lock repair job %d: %w", jobID, err)
		}
		switch locked.Status {
		case tagmodels.AuxiliaryDescriptionRepairStatusConflict,
			tagmodels.AuxiliaryDescriptionRepairStatusUnavailable:
			outcome = RepairRequeueConflict
			job = locked
			return nil
		case tagmodels.AuxiliaryDescriptionRepairStatusCompleted,
			tagmodels.AuxiliaryDescriptionRepairStatusPending:
			// Done or already queued — the retry changes NOTHING.
			outcome = RepairRequeueNoop
			job = locked
			return nil
		case tagmodels.AuxiliaryDescriptionRepairStatusEmbedding,
			tagmodels.AuxiliaryDescriptionRepairStatusSaving,
			tagmodels.AuxiliaryDescriptionRepairStatusMatching:
			// A running task is a no-op; only an interrupted one (its lease
			// expired) may be re-queued.
			var dbNow time.Time
			if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&dbNow); err != nil {
				return fmt.Errorf("read database clock: %w", err)
			}
			if locked.LeaseExpiresAt != nil && locked.LeaseExpiresAt.After(dbNow) {
				outcome = RepairRequeueNoop
				job = locked
				return nil
			}
		case tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed,
			tagmodels.AuxiliaryDescriptionRepairStatusSaveFailed,
			tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed:
			// An explicit failure waits for the user's retry.
		default:
			outcome = RepairRequeueNoop
			job = locked
			return nil
		}

		// Re-queue: the SAME task, the stage it must continue from.
		targetStatus := tagmodels.AuxiliaryDescriptionRepairStatusPending
		targetStage := ""
		if locked.DescriptionCommitted {
			// The description is committed — only the matching continues.
			targetStatus = tagmodels.AuxiliaryDescriptionRepairStatusMatching
			targetStage = tagmodels.AuxiliaryDescriptionRepairStageMatching
		}
		if err := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
			Where("id = ?", locked.ID).
			Updates(map[string]any{
				"status":           targetStatus,
				"stage":            targetStage,
				"error":            "",
				"phase_token":      nil,
				"claim_owner":      nil,
				"claimed_at":       nil,
				"lease_expires_at": nil,
				"updated_at":       gorm.Expr("now()"),
			}).Error; err != nil {
			return fmt.Errorf("re-queue repair job %d: %w", locked.ID, err)
		}
		outcome = RepairRequeueRequeued
		return tx.Where("id = ?", locked.ID).First(&job).Error
	})
	if err != nil {
		return nil, RepairRequeueNoop, err
	}
	return &job, outcome, nil
}
