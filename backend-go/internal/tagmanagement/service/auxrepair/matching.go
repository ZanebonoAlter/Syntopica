package auxrepair

// Matching phase of the auxiliary-label description-repair chain
// (board-health-triage-advice seam B4 / task 2.4): the FROZEN scope is
// processed with the EXISTING board matching engine and every item result is
// persisted together with the job counters.
//
// Boundaries of this file:
//   - the matching algorithm is NOT copied and NOT changed — the existing
//     engine (board.SemanticBoardMatchingService.MatchTopicTag) is called;
//     the engine owns its own writes to the board match table;
//   - only the frozen items are processed (pending / failed). Succeeded and
//     not_applicable items are final and never run again; tags created after
//     the freeze are NOT part of this job (they ride the existing real-time
//     matching path);
//   - no goroutine, no dispatcher, no heartbeat, no HTTP route in this batch —
//     the phase is entered explicitly (2.5 will drive it).
//
// Counting (implementation-contract.md §1): processed counts the items the
// engine actually succeeded on (a legal zero-hit IS a success), failed the
// items still failing, not_applicable the frozen items that became
// inapplicable (deleted / disabled / merged) — never a success, never a
// shrinkage of the frozen total. remaining = total - processed - failed -
// not_applicable. The counters come from the persisted items; nothing is
// counted in memory.

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/board"
)

// Matcher is the seam onto the existing board matching engine. The default
// is the production engine bound to this service's DB; tests inject a stub.
type Matcher interface {
	MatchTopicTag(ctx context.Context, topicTagID uint) ([]board.SemanticBoardMatchResult, error)
}

// WithMatcher replaces the matching engine. The stub lets the tests drive
// failures and legal zero-hits without touching the real matcher.
func WithMatcher(m Matcher) Option {
	return func(s *Service) {
		if m != nil {
			s.matcher = m
		}
	}
}

// Stage errors of the matching phase.
var (
	// ErrStageNotCommitted: the description was not committed yet — the
	// matching phase must not run (the engine would match the OLD vector).
	ErrStageNotCommitted = errors.New("repair job has not committed the description yet")
	// ErrStageAlreadyCompleted: the job is already finished.
	ErrStageAlreadyCompleted = errors.New("repair job is already completed")
)

// ExecuteMatchingPhase runs the matching phase of a repair task over the
// FROZEN scope.
//
// Preconditions (all checked BEFORE the first engine call, so a refused run
// touches nothing): the executor owns the job with a live lease, the
// description was committed, and the job is not already finished.
//
// Per item: a tag that no longer exists (deleted) or left the active
// population (disabled / merged) is recorded as no longer applicable — the
// engine is NOT called for it, it counts neither as success nor as failure,
// and the frozen total does not shrink. An applicable tag is handed to the
// existing engine; a nil error is a success (including a legal zero-hit), an
// error is a failure that does NOT stop the sibling items.
//
// The per-item result and the job counters are written in ONE short
// transaction, guarded by the ownership and the database clock; a lost
// ownership stops the phase without touching any further item, counter or
// final state.
func (s *Service) ExecuteMatchingPhase(ctx context.Context, jobID uint, owner string) (view *JobView, err error) {
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
	if !s.ownsJob(job, owner) {
		return nil, fmt.Errorf("%w: %w", ErrStageOwnerLost, repository.ErrRepairJobOwnerLost)
	}
	if !job.DescriptionCommitted {
		return nil, fmt.Errorf("%w: %w", ErrStageNotCommitted, repository.ErrRepairJobNotCommitted)
	}
	switch job.Status {
	case tagmodels.AuxiliaryDescriptionRepairStatusCompleted:
		return nil, fmt.Errorf("%w: %w", ErrStageAlreadyCompleted, repository.ErrRepairJobAlreadyCommitted)
	case tagmodels.AuxiliaryDescriptionRepairStatusConflict,
		tagmodels.AuxiliaryDescriptionRepairStatusUnavailable:
		return nil, fmt.Errorf("%w: job is in the terminal state %s", ErrStageAlreadyCompleted, job.Status)
	}

	// Claim the execution right of the matching phase (atomic CAS): a second
	// attempt under the same owner is refused as busy instead of running the
	// engine twice; a takeover after an expired lease starts with a fresh
	// token, so the stale in-flight attempt can no longer commit.
	token, err := newRepairPhaseToken()
	if err != nil {
		logging.Errorf("[auxiliary-description-repair] mint the phase token of job %d: %v", jobID, err)
		return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
	}
	if _, err := s.repo.AcquireRepairPhase(ctx, repository.AcquireRepairPhaseParams{
		JobID:        jobID,
		Owner:        owner,
		Token:        token,
		TargetStage:  tagmodels.AuxiliaryDescriptionRepairStageMatching,
		TargetStatus: tagmodels.AuxiliaryDescriptionRepairStatusMatching,
		AllowedSourceStatuses: []string{
			tagmodels.AuxiliaryDescriptionRepairStatusMatching,
			tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed,
		},
		RequireCommitted: true,
	}); err != nil {
		return nil, s.mapMatchingError(err)
	}

	// Panic fence: a panic inside the matching engine must not take the
	// process down. The description was ALREADY committed in this stage — the
	// saved description and vector STAY (the database is the truth), and the
	// task is marked matching_failed (a retryable state; the scanner never
	// picks a failed task up on its own).
	defer func() {
		if rec := recover(); rec != nil {
			logging.Errorf("[auxiliary-description-repair] panic in the matching phase of job %d: %v\n%s",
				jobID, rec, debug.Stack())
			s.markStagePanic(ctx, jobID, owner, token)
			view, err = nil, fmt.Errorf("%w", ErrStageAttemptPanicked)
		}
	}()

	// The board match cache has no TTL — drop it before every run, including
	// every resume/retry, so the new storage vector is the one being matched.
	s.invalidateMatchCache()

	items, err := s.repo.ListRepairScopeItems(ctx, jobID)
	if err != nil {
		logging.Errorf("[auxiliary-description-repair] load scope items of job %d: %v", jobID, err)
		return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
	}

	for _, item := range items {
		state, err := s.repo.GetScopeTagState(ctx, item.TopicTagID)
		if err != nil {
			logging.Errorf("[auxiliary-description-repair] load scope tag %d state: %v", item.TopicTagID, err)
			return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
		}
		if !state.Exists || !state.Active {
			// Deleted / disabled / merged → no longer applicable. The engine is
			// not called, the item is not a success and not a failure, and the
			// frozen total stays as it is.
			if err := s.recordItem(ctx, jobID, owner, token, item.TopicTagID,
				tagmodels.AuxiliaryDescriptionRepairItemNotApplicable,
				"the frozen tag is no longer applicable"); err != nil {
				return nil, err
			}
			continue
		}

		_, matchErr := s.matcher.MatchTopicTag(ctx, item.TopicTagID)
		status := tagmodels.AuxiliaryDescriptionRepairItemSucceeded
		message := ""
		if matchErr != nil {
			logging.Errorf("[auxiliary-description-repair] match topic tag %d (job %d): %v", item.TopicTagID, jobID, matchErr)
			// A tag that vanished DURING the call is only "not applicable" when
			// the database confirms it — a plain engine failure stays a failure.
			after, stateErr := s.repo.GetScopeTagState(ctx, item.TopicTagID)
			if stateErr != nil {
				logging.Errorf("[auxiliary-description-repair] load scope tag %d state after failure: %v", item.TopicTagID, stateErr)
				return nil, fmt.Errorf("%w", ErrStageStateNotPersisted)
			}
			if !after.Exists || !after.Active {
				status = tagmodels.AuxiliaryDescriptionRepairItemNotApplicable
				message = "the frozen tag is no longer applicable"
			} else {
				status = tagmodels.AuxiliaryDescriptionRepairItemFailed
				message = "match execution failed"
			}
		}
		if err := s.recordItem(ctx, jobID, owner, token, item.TopicTagID, status, message); err != nil {
			return nil, err
		}
	}

	// Close the phase: the persisted counters decide completed / matching_failed.
	final, _, err := s.repo.FinishMatchingPhase(ctx, jobID, owner, token)
	if err != nil {
		return nil, s.mapMatchingError(err)
	}
	return jobView(final), nil
}

// recordItem persists one item result (item row + job counters in one short
// transaction) and maps the guard's rejection onto the stage error.
func (s *Service) recordItem(ctx context.Context, jobID uint, owner, token string, topicTagID uint, status, message string) error {
	if _, err := s.repo.RecordMatchingItemResult(ctx, repository.RepairItemOutcome{
		JobID:      jobID,
		Owner:      owner,
		PhaseToken: token,
		TopicTagID: topicTagID,
		Status:     status,
		Error:      message,
	}); err != nil {
		return s.mapMatchingError(err)
	}
	return nil
}

// mapMatchingError turns a storage failure into a stage error. Raw database
// details are logged, never returned.
func (s *Service) mapMatchingError(err error) error {
	switch {
	case errors.Is(err, repository.ErrRepairJobBusy):
		return fmt.Errorf("%w: %w", ErrStageBusy, err)
	case errors.Is(err, repository.ErrRepairJobOwnerLost):
		return fmt.Errorf("%w: %w", ErrStageOwnerLost, err)
	case errors.Is(err, repository.ErrRepairJobNotCommitted):
		return fmt.Errorf("%w: %w", ErrStageNotCommitted, err)
	case errors.Is(err, repository.ErrRepairJobAlreadyCommitted):
		return fmt.Errorf("%w: %w", ErrStageAlreadyCompleted, err)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return err
	default:
		logging.Errorf("[auxiliary-description-repair] matching phase storage error: %v", err)
		return fmt.Errorf("%w", ErrStageStateNotPersisted)
	}
}

// invalidateMatchCache drops the TTL-less board match cache. It is separate
// from the save-phase invalidation because the matching phase must invalidate
// again before EVERY run (including a resume/retry).
func (s *Service) invalidateMatchCache() {
	if s.invalidateCache != nil {
		s.invalidateCache()
	}
}
