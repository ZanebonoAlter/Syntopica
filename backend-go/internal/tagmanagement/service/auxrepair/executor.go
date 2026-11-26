package auxrepair

// Executor of the auxiliary-label description-repair chain
// (board-health-triage-advice seam B5 / task 2.5): the safe execution loop
// with claim/lease, heartbeat and restart recovery.
//
// Lifecycle contract:
//   - NewService NEVER starts anything. The executor is started EXPLICITLY
//     with Start(appCtx) — the context is the APPLICATION lifetime, never the
//     context of the HTTP request that created the task (closing the page must
//     not cancel a confirmed task);
//   - Start is idempotent: calling it twice does NOT run two workers;
//   - Stop cancels the loop AND the heartbeat of the running attempt, joins
//     them and is idempotent;
//   - RunOnce performs exactly one scan-and-execute round — the controllable
//     seam for tests and for a future manual trigger.
//
// The queue is the DATABASE (the jobs themselves). There is no in-memory map
// and no process-local queue: a restart simply finds the unfinished jobs
// again.
//
// Automatic boundary (implementation-contract.md §2 / spec「失败等待明确重试」):
// the scanner claims ONLY a fresh task (pending) or an unfinished phase whose
// lease has expired. The explicit failure states (embedding_failed /
// save_failed / matching_failed) wait for the USER's retry — a failure never
// burns AI quota in a loop.
//
// Honesty boundary: the engine call cannot be interrupted. If the process dies
// during a matching call, the item stays pending and is replayed at least once
// after the restart (the engine replaces its mounts, so the replay is
// idempotent). Exactly-once is NOT promised.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"syntopica-backend/internal/platform/logging"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// ExecutorConfig is the tunables of the execution loop. The defaults are the
// production ones; the tests override them.
type ExecutorConfig struct {
	// PollInterval is the idle wait between scans (never busy-looping).
	PollInterval time.Duration
	// LeaseDuration is how long a claimed task stays owned without a
	// heartbeat.
	LeaseDuration time.Duration
	// HeartbeatInterval is how often the lease is renewed. It MUST be
	// shorter than the lease, so a live attempt is never seen as expired.
	HeartbeatInterval time.Duration
	// HeartbeatTimeout bounds one heartbeat database call.
	HeartbeatTimeout time.Duration
	// MaxBatch is how many tasks one round may execute (the default is a
	// single execution slot).
	MaxBatch int
}

// DefaultExecutorConfig returns the production defaults.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		PollInterval:      5 * time.Second,
		LeaseDuration:     60 * time.Second,
		HeartbeatInterval: 20 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxBatch:          1,
	}
}

// Validate checks the configuration: every duration must be positive and the
// heartbeat must be strictly shorter than the lease (otherwise a live attempt
// would be seen as expired between two heartbeats).
func (c ExecutorConfig) Validate() error {
	if c.PollInterval <= 0 {
		return fmt.Errorf("auxrepair: poll interval must be positive")
	}
	if c.LeaseDuration <= 0 {
		return fmt.Errorf("auxrepair: lease duration must be positive")
	}
	if c.HeartbeatInterval <= 0 {
		return fmt.Errorf("auxrepair: heartbeat interval must be positive")
	}
	if c.HeartbeatTimeout <= 0 {
		return fmt.Errorf("auxrepair: heartbeat timeout must be positive")
	}
	if c.HeartbeatInterval >= c.LeaseDuration {
		return fmt.Errorf("auxrepair: heartbeat interval (%s) must be shorter than the lease duration (%s)",
			c.HeartbeatInterval, c.LeaseDuration)
	}
	if c.MaxBatch < 1 {
		return fmt.Errorf("auxrepair: max batch must be at least 1")
	}
	return nil
}

// Executor runs the description-repair phases against the database.
type Executor struct {
	svc    *Service
	config ExecutorConfig

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// notify wakes the running worker (buffered, non-blocking) — a
	// notification can only WAKE an existing worker; it never starts one.
	notify chan struct{}

	instanceID string
}

// NewExecutor builds an executor for the given service. It starts nothing.
func NewExecutor(svc *Service, config ExecutorConfig) (*Executor, error) {
	if svc == nil {
		return nil, fmt.Errorf("auxrepair: service is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	id, err := randomHex(8)
	if err != nil {
		return nil, err
	}
	e := &Executor{
		svc:        svc,
		config:     config,
		notify:     make(chan struct{}, 1),
		instanceID: id,
	}
	// A queued task wakes the (already running) worker; the wakeup is
	// best-effort and never starts a worker.
	svc.attachWakeup(e.Notify)
	return e, nil
}

// Config returns the executor's configuration.
func (e *Executor) Config() ExecutorConfig { return e.config }

// Start starts the worker with the APPLICATION lifetime context. It is
// idempotent: a second Start while running does NOT create a second worker.
func (e *Executor) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("auxrepair: executor needs an application-lifetime context")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.running = true
	e.wg.Add(1)
	go e.loop(runCtx)
	return nil
}

// Stop cancels the loop and the heartbeat of the running attempt, joins them
// and is idempotent.
func (e *Executor) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	cancel := e.cancel
	e.cancel = nil
	e.running = false
	e.mu.Unlock()

	cancel()
	e.wg.Wait()
}

// Running reports whether the worker loop is active.
func (e *Executor) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Notify wakes the running worker so it scans immediately. It is a
// best-effort, non-blocking hint: with no worker running the notification is
// dropped — it NEVER starts a worker and never spawns a goroutine.
func (e *Executor) Notify() {
	select {
	case e.notify <- struct{}{}:
	default:
	}
}

// RunOnce performs one scan-and-execute round: it claims at most one task
// (the configuration's execution slot) and runs its phase. It returns the
// number of tasks it executed (0 = the queue was empty).
//
// It is the controllable seam for tests and for a future manual trigger; the
// running worker uses the same code path.
func (e *Executor) RunOnce(ctx context.Context) (int, error) {
	owner, err := e.newOwnerToken()
	if err != nil {
		return 0, err
	}
	// The lease DURATION is handed to the repository: the DATABASE clock
	// computes the deadline (review fix 2).
	job, err := e.svc.repo.ClaimNextRepairJob(ctx, owner, e.config.LeaseDuration)
	if errors.Is(err, repository.ErrRepairNoClaimableJob) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	e.runJob(ctx, job, owner)
	return 1, nil
}

// runJob executes one claimed task with a heartbeat next to it.
//
// The phase context is derived from the CALLER's context (the application
// lifetime for the worker loop) and is cancelled as soon as the heartbeat
// loses the ownership — the stale attempt can then neither commit nor report
// progress.
func (e *Executor) runJob(ctx context.Context, job *tagmodels.AuxiliaryDescriptionRepairJob, owner string) {
	// Safety net: a panic anywhere in the attempt must not escape the worker
	// goroutine (it would take the whole backend down). The deferred cleanup
	// below still joins the heartbeat and releases the slot; the task keeps
	// its persisted state and is recovered by the lost-lease path.
	defer func() {
		if rec := recover(); rec != nil {
			logging.Errorf("[auxiliary-description-repair] panic while executing job %d (stage %q): %v\n%s",
				job.ID, job.Stage, rec, debug.Stack())
		}
	}()

	phaseCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	hbDone := make(chan struct{})
	go func() {
		defer close(hbDone)
		e.heartbeat(phaseCtx, job.ID, owner, cancel)
	}()
	defer func() {
		cancel(context.Canceled)
		<-hbDone
	}()

	var err error
	if job.DescriptionCommitted {
		// The description is committed — only the matching continues, never
		// another embedding or save.
		_, err = e.svc.ExecuteMatchingPhase(phaseCtx, job.ID, owner)
	} else {
		_, err = e.svc.ExecuteEmbeddingPhase(phaseCtx, job.ID, owner)
		if err == nil {
			// The embedding phase committed the description → the matching
			// continues in the SAME attempt (same lease, same heartbeat),
			// instead of waiting for the lease to expire. A committed task is
			// never embedded or saved again.
			if updated, rerr := e.svc.repo.GetRepairJobByID(phaseCtx, job.ID); rerr == nil && updated.DescriptionCommitted {
				_, err = e.svc.ExecuteMatchingPhase(phaseCtx, job.ID, owner)
			}
		}
	}
	if err != nil {
		if phaseCtx.Err() != nil {
			// The attempt was cancelled (stop or a lost ownership) — the phase
			// writes nothing in that case.
			logging.Warnf("[auxiliary-description-repair] job %d attempt cancelled: %v", job.ID, err)
			return
		}
		logging.Errorf("[auxiliary-description-repair] job %d attempt failed: %v", job.ID, err)
	}
}

// heartbeat renews the lease of the running attempt.
//
// The renewal is allowed only while this owner still holds a LIVE lease: an
// expired lease is never revived, and the phase token, the status and the
// counters are never touched. A lost ownership (or an uncertain database
// failure) cancels the phase context — the stale actor must not commit or
// report progress afterwards.
func (e *Executor) heartbeat(ctx context.Context, jobID uint, owner string, cancel context.CancelCauseFunc) {
	// The heartbeat runs in its own goroutine — a panic here must not escape.
	defer func() {
		if rec := recover(); rec != nil {
			logging.Errorf("[auxiliary-description-repair] panic in the heartbeat of job %d: %v\n%s",
				jobID, rec, debug.Stack())
			cancel(fmt.Errorf("%w: heartbeat panicked", ErrStageAttemptPanicked))
		}
	}()
	ticker := time.NewTicker(e.config.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hbCtx, hbCancel := context.WithTimeout(ctx, e.config.HeartbeatTimeout)
			err := e.svc.repo.RenewRepairLease(hbCtx, jobID, owner, e.config.LeaseDuration)
			hbCancel()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logging.Warnf("[auxiliary-description-repair] heartbeat of job %d lost the ownership: %v", jobID, err)
				cancel(err)
				return
			}
		}
	}
}

// loop is the worker: it scans, executes and waits for the next tick (or a
// wakeup). An empty queue or a database error leads to a WAIT — never to a
// busy loop.
func (e *Executor) loop(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.config.PollInterval)
	defer ticker.Stop()
	for {
		worked := false
		for i := 0; i < e.config.MaxBatch; i++ {
			n, err := e.runRound(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logging.Warnf("[auxiliary-description-repair] executor round failed: %v", err)
				break
			}
			if n == 0 {
				break
			}
			worked = true
		}
		if worked {
			// Keep draining while there is work — the queue is bounded by the
			// execution slot, never by a busy loop.
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-e.notify:
		case <-ticker.C:
		}
	}
}

// runRound is one bounded scan-execute round with its own panic fence: a
// panic in one round must not kill the worker loop (the next round simply
// continues).
func (e *Executor) runRound(ctx context.Context) (n int, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			logging.Errorf("[auxiliary-description-repair] panic in the executor round: %v\n%s", rec, debug.Stack())
			n, err = 0, fmt.Errorf("%w: executor round panicked", ErrStageAttemptPanicked)
		}
	}()
	return e.RunOnce(ctx)
}

// newOwnerToken mints the UNIQUE owner identity of ONE execution. It is not a
// fixed process name: every execution gets its own token, so a takeover can
// never be confused with the previous attempt.
func (e *Executor) newOwnerToken() (string, error) {
	suffix, err := randomHex(8)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("auxrepair-%s-%s", e.instanceID, suffix), nil
}

// randomHex returns n random bytes as a hex string.
func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auxrepair: read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
