package repository

// Auxiliary-description repair — EXECUTION stage primitives
// (board-health-triage-advice seam B3 / task 2.3, hardened by the B3 review).
//
// This file owns every SQL statement and transaction of the execution chain:
// the claim/lease CAS, the atomic acquisition of the phase execution right,
// the owner+token guarded stage transitions, the failure / terminal markers
// and the ATOMIC save of the confirmed description plus its new storage
// embedding. The service layer only orchestrates and calls the embedding unit
// — no SQL there.
//
// Scope note: the full claim / heartbeat / restart-recovery workflow belongs
// to the later recovery seam (task 2.5). What lives here are the primitives
// that workflow will use (a CAS claim, an atomic phase acquisition) plus the
// atomic save of task 2.3.
//
// Guard model (review R1 + R2):
//   - the lease is judged by the DATABASE's REAL-TIME clock
//     (clock_timestamp()) — never by now()/statement_timestamp(), which are
//     frozen at the transaction/statement start and would let a transaction
//     that waited on a row lock save AFTER its lease expired;
//   - the lease is re-checked AFTER all row locks are held;
//   - every write carries (claim_owner, phase_token) and the live-lease
//     condition, and checks RowsAffected — a lost ownership rolls the whole
//     transaction back, including an already written label/vector;
//   - the phase execution right is claimed by an atomic CAS: a second attempt
//     (even under the same owner) is refused as busy instead of running the
//     engine twice, and a takeover after an expired lease uses a NEW token so
//     the stale in-flight attempt can no longer commit.

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

// Errors of the execution stage. The service layer maps them onto
// stage-distinguishable outcomes; none of them carries raw SQL text.
var (
	// ErrRepairOwnerRequired: no executor owner was supplied — the stage
	// refuses to run unowned (every write is owner-guarded).
	ErrRepairOwnerRequired = errors.New("repair executor owner is required")
	// ErrRepairPhaseTokenRequired: no phase token was supplied — every write
	// of an attempt is guarded by its token.
	ErrRepairPhaseTokenRequired = errors.New("repair phase token is required")
	// ErrRepairJobNotClaimable: the job cannot be claimed by this executor
	// (already committed, terminal, or held by a live lease of somebody else).
	ErrRepairJobNotClaimable = errors.New("repair job cannot be claimed by this executor")
	// ErrRepairJobOwnerLost: the job is not (or no longer) owned by this
	// executor — missing owner, expired lease, replaced token, or a newer
	// owner took over. The job must NOT be touched in this case.
	ErrRepairJobOwnerLost = errors.New("repair job is not owned by this executor")
	// ErrRepairJobAlreadyCommitted: the description was already committed by
	// an earlier run — re-entering the embedding stage must not embed or
	// save again.
	ErrRepairJobAlreadyCommitted = errors.New("repair job already committed the confirmed description")
	// ErrRepairJobBusy: another attempt currently holds the execution right of
	// this phase. The caller must return without touching the job — in
	// particular it must NOT write a failure state over the running attempt.
	ErrRepairJobBusy = errors.New("repair job is already being executed by another attempt")
)

// AcquireRepairPhaseParams claims the execution right of one phase.
type AcquireRepairPhaseParams struct {
	JobID uint
	Owner string
	// Token is the attempt's phase token (fresh per attempt).
	Token string
	// TargetStage / TargetStatus are the values the phase starts with.
	TargetStage  string
	TargetStatus string
	// AllowedSourceStatuses are the job states the phase may start from.
	// A state left behind by a DEAD attempt (its token was voided by the
	// claim takeover) is allowed too — the token, not the status, decides
	// whether a runner is active.
	AllowedSourceStatuses []string
	// RequireCommitted selects the phase: false = the saving phase (the
	// description is NOT committed yet), true = the matching phase (the
	// description IS committed).
	RequireCommitted bool
}

// AcquireRepairPhase atomically claims the execution right of a phase.
//
// It succeeds only when: the job exists, the description was not committed
// yet, the caller holds the claim with a LIVE lease, NO other attempt holds
// the phase token, and the job status is one of the allowed source states.
//
// The CAS writes the fresh token, the target stage/status and CLEARS the old
// error (a new attempt must not inherit the previous failure message).
func (r *AuxiliaryDescriptionRepairRepository) AcquireRepairPhase(ctx context.Context, params AcquireRepairPhaseParams) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	owner := strings.TrimSpace(params.Owner)
	if owner == "" {
		return nil, ErrRepairOwnerRequired
	}
	token := strings.TrimSpace(params.Token)
	if token == "" {
		return nil, ErrRepairPhaseTokenRequired
	}
	var job tagmodels.AuxiliaryDescriptionRepairJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", params.JobID).First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return fmt.Errorf("lock repair job %d: %w", params.JobID, err)
		}
		if locked.DescriptionCommitted != params.RequireCommitted {
			if params.RequireCommitted {
				return ErrRepairJobNotCommitted
			}
			return ErrRepairJobAlreadyCommitted
		}
		if locked.ClaimOwner == nil || *locked.ClaimOwner != owner {
			return ErrRepairJobOwnerLost
		}
		// The lease is judged by the database's real-time clock.
		var dbNow time.Time
		if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&dbNow); err != nil {
			return fmt.Errorf("read database clock: %w", err)
		}
		if locked.LeaseExpiresAt == nil || !locked.LeaseExpiresAt.After(dbNow) {
			return ErrRepairJobOwnerLost
		}
		// Another attempt holds the phase right → busy (and it must NOT be
		// overwritten with a failure state).
		if locked.PhaseToken != nil && *locked.PhaseToken != "" {
			return ErrRepairJobBusy
		}
		if !repairStatusAllowed(locked.Status, params.AllowedSourceStatuses) {
			switch locked.Status {
			case tagmodels.AuxiliaryDescriptionRepairStatusCompleted,
				tagmodels.AuxiliaryDescriptionRepairStatusConflict,
				tagmodels.AuxiliaryDescriptionRepairStatusUnavailable:
				return ErrRepairJobNotClaimable
			default:
				return ErrRepairJobBusy
			}
		}
		res := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
			Where("id = ? AND phase_token IS NULL", params.JobID).
			Updates(map[string]any{
				"phase_token": token,
				"stage":       params.TargetStage,
				"status":      params.TargetStatus,
				"error":       "",
				"updated_at":  gorm.Expr("now()"),
			})
		if res.Error != nil {
			return fmt.Errorf("acquire repair phase of job %d: %w", params.JobID, res.Error)
		}
		if res.RowsAffected == 0 {
			// Somebody holds the phase right after all (the token was not NULL
			// when the row was locked, or it was set concurrently): the attempt
			// must be refused — never "granted" without an actual write.
			return ErrRepairJobBusy
		}
		return tx.Where("id = ?", params.JobID).First(&job).Error
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// repairStatusAllowed reports whether the job status may start the phase.
func repairStatusAllowed(status string, allowed []string) bool {
	for _, s := range allowed {
		if s == status {
			return true
		}
	}
	return false
}

// ClaimRepairJob claims the execution lease of a job for the given owner.
//
// The CAS accepts a job that is unclaimed, already owned by the same owner,
// or whose previous lease has expired (the DATABASE's real-time clock
// decides). A job in a terminal status can never be claimed.
//
// A TAKEOVER (a different owner, or a lease that had expired) VOIDS the
// previous attempt's phase token: the dead attempt can no longer commit, and
// the new attempt starts from a clean phase right. A plain re-claim by the
// same owner with a live lease keeps the token (the attempt is still running).
//
// The claim is about OWNERSHIP, not about the stage: a committed job can be
// claimed too (the matching stage of task 2.4 needs exactly that). Whether a
// given stage may run on a claimed job is decided by the stage itself.
//
// The lease is written as clock_timestamp() + the given duration (the DATABASE
// clock is the authority); a negative duration produces an already-expired
// lease, which the guard then rejects.
func (r *AuxiliaryDescriptionRepairRepository) ClaimRepairJob(ctx context.Context, jobID uint, owner string, lease time.Duration) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, ErrRepairOwnerRequired
	}
	var job tagmodels.AuxiliaryDescriptionRepairJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", jobID).First(&locked).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return fmt.Errorf("lock repair job %d: %w", jobID, err)
		}
		if repairStatusAllowed(locked.Status, tagmodels.AuxiliaryDescriptionRepairTerminalStatuses) {
			return ErrRepairJobNotClaimable
		}
		var dbNow time.Time
		if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&dbNow); err != nil {
			return fmt.Errorf("read database clock: %w", err)
		}
		heldByOther := locked.ClaimOwner != nil && *locked.ClaimOwner != owner &&
			locked.LeaseExpiresAt != nil && locked.LeaseExpiresAt.After(dbNow)
		if heldByOther {
			return ErrRepairJobNotClaimable
		}
		updates := map[string]any{
			"claim_owner": owner,
			"claimed_at":  gorm.Expr("now()"),
			// The lease is written by the DATABASE clock: an application clock
			// behind the database would otherwise write a lease that is already
			// expired (review fix 2).
			"lease_expires_at": leaseExpiryExpr(lease),
			"updated_at":       gorm.Expr("now()"),
		}
		// A takeover voids the previous attempt's phase right.
		takeover := locked.ClaimOwner == nil || *locked.ClaimOwner != owner ||
			locked.LeaseExpiresAt == nil || !locked.LeaseExpiresAt.After(dbNow)
		if takeover {
			updates["phase_token"] = nil
		}
		if err := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
			Where("id = ?", jobID).Updates(updates).Error; err != nil {
			return fmt.Errorf("claim repair job %d: %w", jobID, err)
		}
		return tx.Where("id = ?", jobID).First(&job).Error
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetRepairJobByID returns the job row by its id (no ownership filter — the
// caller is the executor and checks the owner itself).
func (r *AuxiliaryDescriptionRepairRepository) GetRepairJobByID(ctx context.Context, jobID uint) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	var job tagmodels.AuxiliaryDescriptionRepairJob
	if err := r.db.WithContext(ctx).Where("id = ?", jobID).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("load repair job %d: %w", jobID, err)
	}
	return &job, nil
}

// GetLiveAuxiliaryLabel returns the live auxiliary label row (zero-write).
// A missing id or a non-auxiliary label is ErrRepairLabelNotFound — the
// caller maps that onto the terminal "unavailable" state.
func (r *AuxiliaryDescriptionRepairRepository) GetLiveAuxiliaryLabel(ctx context.Context, auxiliaryLabelID uint) (*models.SemanticLabel, error) {
	var label models.SemanticLabel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND label_type = ?", auxiliaryLabelID, "auxiliary").
		First(&label).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRepairLabelNotFound
		}
		return nil, fmt.Errorf("load auxiliary label %d: %w", auxiliaryLabelID, err)
	}
	return &label, nil
}

// MarkRepairStage advances the job to the given stage/status. Owner- and
// token-guarded: an executor that lost the lease or the phase right must not
// advance the job.
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairStage(ctx context.Context, jobID uint, owner, token, stage, status string) error {
	return r.updateJobGuarded(ctx, jobID, owner, token, false, map[string]any{
		"stage":  stage,
		"status": status,
	})
}

// MarkRepairFailure records a retryable stage failure (embedding_failed /
// save_failed) and RELEASES the phase right (the attempt ended). These
// statuses are NOT terminal — the job keeps holding the per-label creation
// mutex until it is finished, abandoned or released (implementation-contract
// §1, migration 20261028_0001).
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairFailure(ctx context.Context, jobID uint, owner, token, status, message string) error {
	// The guard follows the phase the failure belongs to: the embedding and
	// saving stages run BEFORE the description is committed, the matching
	// stage runs AFTER it (a matching failure must not roll the committed
	// description back).
	var requireCommitted bool
	switch status {
	case tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed, tagmodels.AuxiliaryDescriptionRepairStatusSaveFailed:
		requireCommitted = false
	case tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed:
		requireCommitted = true
	default:
		return fmt.Errorf("mark repair failure: %q is not a retryable failure status", status)
	}
	return r.updateJobGuarded(ctx, jobID, owner, token, requireCommitted, map[string]any{
		"status":      status,
		"error":       message,
		"phase_token": nil,
	})
}

// MarkRepairTerminal records a terminal outcome (conflict / unavailable) and
// releases the phase right. Terminal statuses release the per-label creation
// mutex so the user can start a new repair for the same label.
func (r *AuxiliaryDescriptionRepairRepository) MarkRepairTerminal(ctx context.Context, jobID uint, owner, token, status, message string) error {
	switch status {
	case tagmodels.AuxiliaryDescriptionRepairStatusConflict, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable:
	default:
		return fmt.Errorf("mark repair terminal: %q is not a terminal status", status)
	}
	return r.updateJobGuarded(ctx, jobID, owner, token, false, map[string]any{
		"status":      status,
		"error":       message,
		"phase_token": nil,
	})
}

// updateJobGuarded applies the owner+token guarded UPDATE. The guard is the
// whole point: an executor without a live lease (missing, expired,
// replaced) or without the phase right must not write anything, and a
// committed job must never be walked back.
//
// requireCommitted selects the phase: false = the saving phase (the
// description is not committed yet), true = the matching phase.
func (r *AuxiliaryDescriptionRepairRepository) updateJobGuarded(ctx context.Context, jobID uint, owner, token string, requireCommitted bool, updates map[string]any) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return ErrRepairOwnerRequired
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrRepairPhaseTokenRequired
	}
	updates["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
		Where("id = ? AND claim_owner = ? AND phase_token = ? AND lease_expires_at IS NOT NULL AND lease_expires_at > clock_timestamp() AND description_committed = ?",
			jobID, owner, token, requireCommitted).
		Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update repair job %d: %w", jobID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRepairJobOwnerLost
	}
	return nil
}

// CommitDescriptionParams is the atomic save request of task 2.3: the
// confirmed (already trimmed) description and its freshly generated storage
// embedding, applied to the job's auxiliary label under the executor's lease.
type CommitDescriptionParams struct {
	JobID       uint
	Owner       string
	PhaseToken  string
	Description string
	Embedding   string
}

// CommitDescription atomically writes the confirmed description AND its new
// storage embedding, and advances the job to the matching stage.
//
// One transaction, four locks/checks, one write each:
//
//  1. lock the job row (FOR UPDATE) — the description must not be committed
//     twice, and the executor must still own it with the phase right;
//  2. lock the auxiliary label row (FOR UPDATE) — the label must not change
//     between the check and the write;
//  3. re-verify: label still exists / is auxiliary / is active, and its
//     source version still equals the version the user confirmed;
//  4. write description + storage embedding on the label, then mark the job
//     committed (description_committed, stage/status=matching,
//     committed_version = the label's version token AFTER the write,
//     committed_at = DB clock) — the marker UPDATE carries the owner, the
//     phase token and the REAL-TIME lease condition, and its RowsAffected is
//     checked: a lost ownership or an expired lease rolls the WHOLE
//     transaction back, including the label write.
//
// Nothing else is touched: label text, aliases and merge_embedding keep their
// values, and no match row, upgrade suggestion, snapshot or report is written.
//
// Business conflicts (version changed / label gone / label disabled) are
// returned as the matching sentinel — the transaction ROLLS BACK, so the job
// row keeps its previous status. The caller must therefore record the
// terminal status in a SEPARATE transaction (rolling back with the conflict
// would erase the very status the user has to see).
func (r *AuxiliaryDescriptionRepairRepository) CommitDescription(ctx context.Context, params CommitDescriptionParams) (*tagmodels.AuxiliaryDescriptionRepairJob, error) {
	owner := strings.TrimSpace(params.Owner)
	if owner == "" {
		return nil, ErrRepairOwnerRequired
	}
	token := strings.TrimSpace(params.PhaseToken)
	if token == "" {
		return nil, ErrRepairPhaseTokenRequired
	}
	if strings.TrimSpace(params.Description) == "" {
		return nil, fmt.Errorf("commit repair description: description is empty")
	}
	if strings.TrimSpace(params.Embedding) == "" {
		return nil, fmt.Errorf("commit repair description: embedding is empty")
	}
	var committed tagmodels.AuxiliaryDescriptionRepairJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. lock the job row and re-check ownership + phase right.
		var job tagmodels.AuxiliaryDescriptionRepairJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", params.JobID).First(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return fmt.Errorf("lock repair job %d: %w", params.JobID, err)
		}
		if job.DescriptionCommitted {
			return ErrRepairJobAlreadyCommitted
		}
		if job.ClaimOwner == nil || *job.ClaimOwner != owner {
			return ErrRepairJobOwnerLost
		}
		if job.PhaseToken == nil || *job.PhaseToken != token {
			return ErrRepairJobOwnerLost
		}

		// 2. lock the auxiliary label row.
		var label models.SemanticLabel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND label_type = ?", job.AuxiliaryLabelID, "auxiliary").
			First(&label).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRepairLabelNotFound
			}
			return fmt.Errorf("lock auxiliary label %d: %w", job.AuxiliaryLabelID, err)
		}

		// 3. re-verify the live state against the confirmed request.
		if label.Status != "active" {
			return ErrRepairLabelNotActive
		}
		if AuxiliaryVersionToken(&label) != job.ExpectedVersion {
			return ErrRepairVersionConflict
		}

		// 3b. ALL locks are held now: re-check the lease against the
		// database's REAL-TIME clock. now()/statement_timestamp() are frozen
		// at the transaction start and would let a transaction that waited on
		// the label lock save after its lease had expired.
		var dbNow time.Time
		if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&dbNow); err != nil {
			return fmt.Errorf("read database clock: %w", err)
		}
		if job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.After(dbNow) {
			return ErrRepairJobOwnerLost
		}

		// 4. write description + storage embedding (and nothing else).
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", label.ID).
			Updates(map[string]any{
				"description": params.Description,
				"embedding":   params.Embedding,
			}).Error; err != nil {
			return fmt.Errorf("update auxiliary label %d: %w", label.ID, err)
		}
		var saved models.SemanticLabel
		if err := tx.Where("id = ?", label.ID).First(&saved).Error; err != nil {
			return fmt.Errorf("reload auxiliary label %d: %w", label.ID, err)
		}

		// 5. mark the job committed and hand it to the matching stage. The
		// marker carries the owner, the phase token and the real-time lease
		// condition; a lost ownership rolls everything back.
		res := tx.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).
			Where("id = ? AND claim_owner = ? AND phase_token = ? AND lease_expires_at IS NOT NULL AND lease_expires_at > clock_timestamp() AND description_committed = ?",
				job.ID, owner, token, false).
			Updates(map[string]any{
				"description_committed": true,
				"stage":                 tagmodels.AuxiliaryDescriptionRepairStageMatching,
				"status":                tagmodels.AuxiliaryDescriptionRepairStatusMatching,
				"committed_version":     AuxiliaryVersionToken(&saved),
				"committed_at":          gorm.Expr("now()"),
				"phase_token":           nil,
				"error":                 "",
				"updated_at":            gorm.Expr("now()"),
			})
		if res.Error != nil {
			return fmt.Errorf("mark repair job %d committed: %w", job.ID, res.Error)
		}
		if res.RowsAffected == 0 {
			// The ownership or the lease was lost while the transaction ran —
			// nothing may be committed, not even the label write.
			return ErrRepairJobOwnerLost
		}
		if err := tx.Where("id = ?", job.ID).First(&committed).Error; err != nil {
			return fmt.Errorf("reload repair job %d: %w", job.ID, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &committed, nil
}
