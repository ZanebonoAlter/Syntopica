package auxrepair

// Execution phase of the auxiliary-label description-repair chain
// (board-health-triage-advice seam B3 / task 2.3): the storage embedding is
// generated OUTSIDE any transaction, then the confirmed description and its
// new storage embedding are saved ATOMICALLY together with the task's
// committed state.
//
// Boundary of this file:
//   - it orchestrates and calls the embedding unit — all SQL lives in the
//     repository layer;
//   - it starts NO goroutine, no worker, no scheduler, no polling;
//   - it exposes NO HTTP route (the three 2.2 endpoints stay untouched) and
//     CreateJob still only enqueues a task.
//
// Guard model: an executor may only work on a job it has claimed; every write
// is guarded by (owner + live lease + not yet committed). A missing, expired
// or replaced owner can neither save a description nor step on the new
// owner's job.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
)

// Embedder is the storage-embedding unit of the repair chain. It is the
// existing auxiliary-label embedder (auxlabel.AuxiliaryLabelEmbedder): one
// airouter.Embed call per invocation, already logged under the
// tagmanagement.auxlabel_embedding operation. Production uses
// auxlabel.DefaultAuxiliaryLabelEmbedder; tests inject a stub.
type Embedder = auxlabel.AuxiliaryLabelEmbedder

// Option customizes a Service at construction time. NewService(db) keeps
// working unchanged (all options are optional).
type Option func(*Service)

// WithEmbedder replaces the storage-embedding unit. The stub lets the tests
// count calls and assert the exact input and mode without any AI call.
func WithEmbedder(e Embedder) Option {
	return func(s *Service) {
		if e != nil {
			s.embedder = e
		}
	}
}

// WithCacheInvalidator replaces the board match-cache invalidation. The
// default is board.InvalidateMatchCache (the board cache has no TTL, so the
// new vector must not be served from it). Tests inject an observer to prove
// the invalidation happens AFTER the commit.
func WithCacheInvalidator(fn func()) Option {
	return func(s *Service) {
		if fn != nil {
			s.invalidateCache = fn
		}
	}
}

// WithClock replaces the clock used for the lease computation and the
// ownership pre-check. The authoritative lease expiry check runs against the
// DATABASE clock inside the repository.
func WithClock(fn func() time.Time) Option {
	return func(s *Service) {
		if fn != nil {
			s.now = fn
		}
	}
}

// Stage-distinguishable errors of the execution phase. None of them carries
// raw SQL, provider payloads or secrets — the underlying cause is logged
// server-side.
var (
	// ErrStageOwnerRequired: no executor owner was supplied.
	ErrStageOwnerRequired = errors.New("repair execution requires an executor owner")
	// ErrStageOwnerLost: the job is not owned by this executor (missing,
	// expired or replaced lease) — nothing was written.
	ErrStageOwnerLost = errors.New("repair execution lost the ownership of the job")
	// ErrStageAlreadyCommitted: the description was already committed — the
	// embedding stage must not run again.
	ErrStageAlreadyCommitted = errors.New("repair job already committed the confirmed description")
	// ErrStageEmbeddingFailed: the storage embedding could not be generated.
	ErrStageEmbeddingFailed = errors.New("storage embedding generation failed")
	// ErrStageSaveFailed: the atomic save failed; the old description and the
	// old vectors are untouched.
	ErrStageSaveFailed = errors.New("saving the confirmed description failed")
	// ErrStageConflict: the source version changed before the save — the
	// newer description wins, the task ends as conflict.
	ErrStageConflict = errors.New("the auxiliary label changed before the description could be saved")
	// ErrStageUnavailable: the label was deleted, disabled or is no longer an
	// auxiliary label — the task ends as unavailable.
	ErrStageUnavailable = errors.New("the auxiliary label is no longer available for repair")
	// ErrStageClaimFailed: the execution lease could not be claimed (the
	// underlying database error is logged, not returned).
	ErrStageClaimFailed = errors.New("the repair task could not be claimed")
	// ErrStageStateNotPersisted: the task state could not be advanced (the
	// underlying database error is logged, not returned).
	ErrStageStateNotPersisted = errors.New("the repair task state could not be advanced")
	// ErrStageBusy: another attempt currently holds the execution right of
	// this phase (the same owner, or a takeover). The caller must return
	// WITHOUT touching the task — the running attempt keeps its state.
	ErrStageBusy = errors.New("repair task is already being executed by another attempt")
	// ErrStageAttemptPanicked: an attempt panicked. The message is
	// deliberately generic — the panic detail (which may carry provider or
	// configuration data) stays in the server log.
	ErrStageAttemptPanicked = errors.New("the repair attempt failed unexpectedly")
)

// newRepairPhaseToken mints the token of ONE attempt. It is minted per call:
// a repeated call under the same owner gets a DIFFERENT token and is refused
// as busy while the first attempt is running; a takeover after an expired
// lease gets a fresh token, so the stale in-flight attempt can no longer
// commit.
func newRepairPhaseToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("mint repair phase token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ClaimJobForExecution claims the execution lease of a job for the given
// owner. It is the minimal precondition primitive of task 2.3 — the full
// claim / heartbeat / restart-recovery workflow belongs to task 2.5 and is
// NOT implemented here.
func (s *Service) ClaimJobForExecution(ctx context.Context, jobID uint, owner string, lease time.Duration) (*JobView, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, fmt.Errorf("%w: %w", ErrStageOwnerRequired, repository.ErrRepairOwnerRequired)
	}
	// The lease DURATION is handed to the repository: the DATABASE clock
	// computes the deadline (an application clock behind the database would
	// write a lease that is already expired — review fix 2).
	job, err := s.repo.ClaimRepairJob(ctx, jobID, owner, lease)
	if err != nil {
		if errors.Is(err, repository.ErrRepairJobNotClaimable) {
			return nil, err
		}
		logging.Errorf("[auxiliary-description-repair] claim job %d for %s: %v", jobID, owner, err)
		return nil, fmt.Errorf("%w", ErrStageClaimFailed)
	}
	return jobView(job), nil
}

// ExecuteEmbeddingPhase runs the embedding + atomic-save phase of a repair
// task:
//
//	pre-check (no AI)  → claim/owner/version/live state
//	embed (no tx)      → ONE storage embedding of "label: new description"
//	atomic commit (tx) → description + storage embedding + task committed
//	after commit       → invalidate the board match cache
//
// The source version is verified twice: before the (wasted) AI call and again
// inside the commit transaction against the locked rows. A label edited while
// the embedding was generated ends as conflict; a deleted / disabled /
// non-auxiliary label ends as unavailable. In both cases the label keeps its
// current values and the task's terminal status is recorded in a SEPARATE
// transaction — a business conflict returned from inside the transaction
// rolls the transaction back, including the status write.
func (s *Service) ExecuteEmbeddingPhase(ctx context.Context, jobID uint, owner string) (view *JobView, err error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, fmt.Errorf("%w: %w", ErrStageOwnerRequired, repository.ErrRepairOwnerRequired)
	}

	job, err := s.repo.GetRepairJobByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		logging.Errorf("[auxiliary-description-repair] load job %d: %v", jobID, err)
		return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
	}
	// Ownership first: an executor that does not own the job learns nothing
	// about its state.
	if !s.ownsJob(job, owner) {
		return nil, fmt.Errorf("%w: %w", ErrStageOwnerLost, repository.ErrRepairJobOwnerLost)
	}
	if job.DescriptionCommitted {
		return nil, fmt.Errorf("%w: %w", ErrStageAlreadyCommitted, repository.ErrRepairJobAlreadyCommitted)
	}

	// Claim the execution right of this phase (atomic CAS): a second attempt
	// under the same owner is refused as busy instead of running the engine
	// twice, and a takeover after an expired lease starts with a fresh token.
	token, err := newRepairPhaseToken()
	if err != nil {
		logging.Errorf("[auxiliary-description-repair] mint the phase token of job %d: %v", jobID, err)
		return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
	}
	if _, err := s.repo.AcquireRepairPhase(ctx, repository.AcquireRepairPhaseParams{
		JobID:                 jobID,
		Owner:                 owner,
		Token:                 token,
		TargetStage:           tagmodels.AuxiliaryDescriptionRepairStageEmbedding,
		TargetStatus:          tagmodels.AuxiliaryDescriptionRepairStatusEmbedding,
		AllowedSourceStatuses: s.embeddingPhaseSourceStatuses(),
		RequireCommitted:      false,
	}); err != nil {
		return nil, s.mapOwnerError(err)
	}

	// Panic fence: a panic inside the embedding unit (or anywhere below) must
	// not take the process down, and it must not fake a committed description.
	// The stage failure is recorded under the owner/token/lease guard; the task
	// stays UNCOMMITTED (the save transaction rolls back on its own).
	defer func() {
		if rec := recover(); rec != nil {
			logging.Errorf("[auxiliary-description-repair] panic in the embedding phase of job %d: %v\n%s",
				jobID, rec, debug.Stack())
			s.markStagePanic(ctx, jobID, owner, token)
			view, err = nil, fmt.Errorf("%w", ErrStageAttemptPanicked)
		}
	}()

	// Pre-check the live state — an unusable label must not burn an AI call.
	label, err := s.repo.GetLiveAuxiliaryLabel(ctx, job.AuxiliaryLabelID)
	if err != nil {
		if errors.Is(err, repository.ErrRepairLabelNotFound) {
			if !s.markTerminal(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable,
				"the auxiliary label no longer exists") {
				return nil, fmt.Errorf("%w: the terminal state could not be persisted", ErrStageUnavailable)
			}
			return nil, fmt.Errorf("%w: %w", ErrStageUnavailable, err)
		}
		logging.Errorf("[auxiliary-description-repair] load auxiliary label %d: %v", job.AuxiliaryLabelID, err)
		return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
	}
	if label.Status != "active" {
		if !s.markTerminal(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable,
			"the auxiliary label is disabled") {
			return nil, fmt.Errorf("%w: the terminal state could not be persisted", ErrStageUnavailable)
		}
		return nil, fmt.Errorf("%w: %w", ErrStageUnavailable, repository.ErrRepairLabelNotActive)
	}
	if repository.AuxiliaryVersionToken(label) != job.ExpectedVersion {
		if !s.markTerminal(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusConflict,
			"the auxiliary label changed before the embedding was generated") {
			return nil, fmt.Errorf("%w: the terminal state could not be persisted", ErrStageConflict)
		}
		return nil, fmt.Errorf("%w: %w", ErrStageConflict, repository.ErrRepairVersionConflict)
	}

	// Stage: embedding (owner-guarded).
	// The embedding is generated OUTSIDE any transaction: the transaction must
	// never wait for a slow provider round trip (idle-in-transaction).
	input := label.Label + ": " + strings.TrimSpace(job.NewDescription)
	pgVector, _, err := s.embedder(ctx, input, auxlabel.AuxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		logging.Errorf("[auxiliary-description-repair] generate storage embedding for job %d: %v", jobID, err)
		if !s.markFailure(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed,
			"storage embedding generation failed") {
			return nil, fmt.Errorf("%w: the failure state could not be persisted", ErrStageEmbeddingFailed)
		}
		return nil, ErrStageEmbeddingFailed
	}
	if strings.TrimSpace(pgVector) == "" {
		logging.Errorf("[auxiliary-description-repair] generate storage embedding for job %d: empty vector", jobID)
		if !s.markFailure(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed,
			"storage embedding generation returned an empty vector") {
			return nil, fmt.Errorf("%w: the failure state could not be persisted", ErrStageEmbeddingFailed)
		}
		return nil, ErrStageEmbeddingFailed
	}

	// Stage: saving (owner-guarded) — a crash inside the commit then leaves
	// an honest stage behind.
	if err := s.repo.MarkRepairStage(ctx, jobID, owner, token,
		tagmodels.AuxiliaryDescriptionRepairStageSaving,
		tagmodels.AuxiliaryDescriptionRepairStatusSaving); err != nil {
		return nil, s.mapOwnerError(err)
	}

	// The atomic save: description + storage embedding + task committed, or
	// nothing at all.
	committed, err := s.repo.CommitDescription(ctx, repository.CommitDescriptionParams{
		JobID:       jobID,
		Owner:       owner,
		PhaseToken:  token,
		Description: strings.TrimSpace(job.NewDescription),
		Embedding:   pgVector,
	})
	if err != nil {
		return nil, s.handleCommitError(ctx, jobID, owner, token, err)
	}

	// AFTER the commit: the board match cache has no TTL, so the new vector
	// must not be served from it. The matching stage (2.4) invalidates again
	// before it matches.
	if s.invalidateCache != nil {
		s.invalidateCache()
	}
	return jobView(committed), nil
}

// handleCommitError maps a failed atomic commit onto the stage outcome. The
// business conflicts roll the transaction back, so the terminal status has to
// be written in a separate transaction — otherwise the status the user has to
// see would be rolled back with it.
func (s *Service) handleCommitError(ctx context.Context, jobID uint, owner, token string, err error) error {
	switch {
	case errors.Is(err, repository.ErrRepairVersionConflict):
		if !s.markTerminal(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusConflict,
			"the auxiliary label changed before the description was saved") {
			return fmt.Errorf("%w: the terminal state could not be persisted", ErrStageConflict)
		}
		return fmt.Errorf("%w: %w", ErrStageConflict, err)
	case errors.Is(err, repository.ErrRepairLabelNotFound):
		if !s.markTerminal(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable,
			"the auxiliary label no longer exists") {
			return fmt.Errorf("%w: the terminal state could not be persisted", ErrStageUnavailable)
		}
		return fmt.Errorf("%w: %w", ErrStageUnavailable, err)
	case errors.Is(err, repository.ErrRepairLabelNotActive):
		if !s.markTerminal(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusUnavailable,
			"the auxiliary label is disabled") {
			return fmt.Errorf("%w: the terminal state could not be persisted", ErrStageUnavailable)
		}
		return fmt.Errorf("%w: %w", ErrStageUnavailable, err)
	case errors.Is(err, repository.ErrRepairJobOwnerLost):
		// Another executor owns the job now — do not touch its state.
		return fmt.Errorf("%w: %w", ErrStageOwnerLost, err)
	case errors.Is(err, repository.ErrRepairJobAlreadyCommitted):
		return fmt.Errorf("%w: %w", ErrStageAlreadyCommitted, err)
	default:
		logging.Errorf("[auxiliary-description-repair] commit description for job %d: %v", jobID, err)
		if !s.markFailure(ctx, jobID, owner, token, tagmodels.AuxiliaryDescriptionRepairStatusSaveFailed,
			"saving the confirmed description failed") {
			// The database (or the context) is no longer writable — say so
			// instead of pretending the failure state was persisted.
			return fmt.Errorf("%w: the failure state could not be persisted", ErrStageSaveFailed)
		}
		return ErrStageSaveFailed
	}
}

// mapOwnerError turns the owner guard's rejection into the stage error. Any
// other failure (a database error) is logged and reported as a generic stage
// error — raw SQL must not reach a public error.
func (s *Service) mapOwnerError(err error) error {
	if errors.Is(err, repository.ErrRepairJobBusy) {
		// Another attempt holds the phase right — do NOT write anything over
		// its state.
		return fmt.Errorf("%w: %w", ErrStageBusy, err)
	}
	if errors.Is(err, repository.ErrRepairJobAlreadyCommitted) {
		return fmt.Errorf("%w: %w", ErrStageAlreadyCommitted, err)
	}
	if errors.Is(err, repository.ErrRepairJobOwnerLost) || errors.Is(err, repository.ErrRepairJobNotClaimable) {
		return fmt.Errorf("%w: %w", ErrStageOwnerLost, err)
	}
	logging.Errorf("[auxiliary-description-repair] advance the repair task state: %v", err)
	return fmt.Errorf("%w", ErrStageStateNotPersisted)
}

// markFailure records a retryable stage failure. It reports whether the
// state could actually be persisted: the write is owner-guarded and the
// database may be gone — the caller must not claim a persisted failure in
// that case.
func (s *Service) markFailure(ctx context.Context, jobID uint, owner, token, status, message string) bool {
	if err := s.repo.MarkRepairFailure(ctx, jobID, owner, token, status, message); err != nil {
		logging.Warnf("[auxiliary-description-repair] mark job %d as %s failed: %v", jobID, status, err)
		return false
	}
	return true
}

// markTerminal records a terminal outcome (conflict / unavailable) — these
// release the per-label creation mutex so the user can start a new repair.
// It reports whether the state could actually be persisted.
func (s *Service) markTerminal(ctx context.Context, jobID uint, owner, token, status, message string) bool {
	if err := s.repo.MarkRepairTerminal(ctx, jobID, owner, token, status, message); err != nil {
		logging.Warnf("[auxiliary-description-repair] mark job %d as %s failed: %v", jobID, status, err)
		return false
	}
	return true
}

// embeddingPhaseSourceStatuses are the job states the embedding phase may
// start from: a fresh task, an explicit retry after a failure, and the
// leftovers of a DEAD attempt (its phase right was voided by the claim
// takeover, so the token — not the status — decides whether a runner is
// active).
func (s *Service) embeddingPhaseSourceStatuses() []string {
	return []string{
		tagmodels.AuxiliaryDescriptionRepairStatusPending,
		tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed,
		tagmodels.AuxiliaryDescriptionRepairStatusSaveFailed,
		tagmodels.AuxiliaryDescriptionRepairStatusEmbedding,
		tagmodels.AuxiliaryDescriptionRepairStatusSaving,
	}
}

// markStagePanic records the stage failure of a panicking attempt.
//
// The PERSISTED state decides the status — the database is the truth:
//   - a COMMITTED description → the MATCHING stage failed; the saved
//     description and vector STAY (a panic must not roll committed data back);
//   - an uncommitted task in the saving stage → save_failed;
//   - an uncommitted task in the embedding stage → embedding_failed.
//
// The write is guarded by the owner, the phase token and the real-time lease:
// a lost ownership or an unwritable database leaves the task alone (the
// lost-lease path recovers it) instead of stepping on a new owner or
// pretending the failure was recorded.
func (s *Service) markStagePanic(ctx context.Context, jobID uint, owner, token string) {
	status := tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed
	message := "the repair attempt failed unexpectedly"
	if persisted, err := s.repo.GetRepairJobByID(ctx, jobID); err == nil {
		switch {
		case persisted.DescriptionCommitted:
			status = tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed
		case persisted.Stage == tagmodels.AuxiliaryDescriptionRepairStageSaving:
			status = tagmodels.AuxiliaryDescriptionRepairStatusSaveFailed
		}
	}
	s.markFailure(ctx, jobID, owner, token, status, message)
}

// ownsJob reports whether the executor still owns the job.
//
// ONLY the owner is decided here. The LEASE belongs to the DATABASE clock:
// the repository's phase acquisition (AcquireRepairPhase) and the matching
// write guard both judge it with clock_timestamp(), and they run BEFORE any
// engine call. The application clock must therefore NOT veto a lease the
// database considers live — an application clock ahead of the database would
// refuse every attempt and the task would be re-claimed forever without ever
// advancing (the database is the authority, review fix 1).
//
// The application clock is kept only as a diagnostic: a divergence between the
// two clocks is worth seeing in the log.
func (s *Service) ownsJob(job *tagmodels.AuxiliaryDescriptionRepairJob, owner string) bool {
	if job.ClaimOwner == nil || *job.ClaimOwner != owner {
		return false
	}
	if job.LeaseExpiresAt == nil {
		return false
	}
	if s.now().After(*job.LeaseExpiresAt) {
		logging.Warnf("[auxiliary-description-repair] the application clock is ahead of the database clock: " +
			"the database clock decides the lease")
	}
	return true
}
