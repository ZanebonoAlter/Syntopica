package auxrepair

// Explicit retry of an auxiliary-label description-repair task
// (board-health-triage-advice seam B5 / task 2.5).
//
// The retry re-uses the SAME task — it never creates a second one and never
// changes the confirmed description, the expected version or the frozen
// scope. What it does depend on the task's state:
//
//   - running (a live lease) or completed → 202 with the SAME task and NO
//     change at all: no re-queue, no new worker, no counter or time change;
//   - conflict / unavailable → 409: the user must re-check the latest version
//     (a retry must not reset the source version of a terminal task);
//   - an explicit failure (embedding_failed / save_failed / matching_failed)
//     or an interrupted attempt (its lease expired) → the SAME task is
//     re-queued for the stage it must continue from: the matching stage when
//     the description was already committed, otherwise the embedding stage.
//
// The retry does NOT look at the live auxiliary label: a historical task whose
// label changed since must stay retryable — the stage's own version check
// (inside the save transaction) refuses the write if the source changed, and
// the retry must not be a way around it.

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"syntopica-backend/internal/tagmanagement/repository"
)

// ErrStageRetryNotAllowed: the task ended in conflict/unavailable — it cannot
// be retried as-is; the user must load the latest version and confirm again.
var ErrStageRetryNotAllowed = errors.New("repair task cannot be retried in its current state")

// RetryJob re-queues a repair task for another explicit attempt.
//
// The ownership is checked first: a job id that does not exist or belongs to
// another auxiliary label is gorm.ErrRecordNotFound (404). The task is then
// re-queued by the database (a short locked transaction) — a concurrent retry
// and the scanner are arbitrated by the database, so the same task can never
// run twice.
//
// The call never runs a phase and never starts a worker: it only makes the
// task claimable again (and wakes the running worker, if any).
func (s *Service) RetryJob(ctx context.Context, auxiliaryLabelID, jobID uint) (*JobView, error) {
	// The ownership check (the job must belong to this auxiliary label).
	if _, err := s.repo.GetRepairJob(ctx, auxiliaryLabelID, jobID); err != nil {
		return nil, err
	}
	job, outcome, err := s.repo.RequeueRepairJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	switch outcome {
	case repository.RepairRequeueConflict:
		return nil, fmt.Errorf("%w: %w", ErrStageRetryNotAllowed, repository.ErrRepairJobNotClaimable)
	default:
		// Requeued or a no-op — in both cases the task is returned as it is.
		// The wakeup only wakes an already running worker.
		s.notifyWakeup()
		return jobView(job), nil
	}
}

// attachWakeup registers a wakeup callback (the executor registers itself).
// The callback may only WAKE an existing worker.
func (s *Service) attachWakeup(fn func()) {
	if fn == nil {
		return
	}
	s.wakeupMu.Lock()
	defer s.wakeupMu.Unlock()
	s.wakeups = append(s.wakeups, fn)
}

// notifyWakeup wakes the registered workers (best effort, never blocking).
func (s *Service) notifyWakeup() {
	s.wakeupMu.Lock()
	hooks := make([]func(), len(s.wakeups))
	copy(hooks, s.wakeups)
	s.wakeupMu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

// ensure the retry error keeps the not-found semantics for the handler.
var _ = gorm.ErrRecordNotFound
