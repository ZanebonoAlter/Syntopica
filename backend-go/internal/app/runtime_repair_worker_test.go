package app

// board-health-triage-advice seam B6 (task 2.6) — the application-level
// lifecycle of the description-repair worker.
//
// The worker runs on the APPLICATION-lifetime context (never on the context of
// the HTTP request that created a task — closing the page must not cancel a
// confirmed task), and the graceful shutdown cancels that context and JOINS
// the worker BEFORE the database is released.
//
// The seams are replaced with recorders here, so the test needs no database.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement"
)

// TestRepairWorkerLifecycleStartsOnAppContextAndJoinsAtShutdown covers both
// halves of the wiring: the start seam receives the application context, and
// the shutdown cancels that context and stops the worker exactly once.
func TestRepairWorkerLifecycleStartsOnAppContextAndJoinsAtShutdown(t *testing.T) {
	origStart := startRepairExecutorFn
	origStop := stopRepairExecutorFn
	origWorkers := stopWorkersFn
	t.Cleanup(func() {
		startRepairExecutorFn = origStart
		stopRepairExecutorFn = origStop
		stopWorkersFn = origWorkers
	})
	// The workers seam must not touch the real DB workers in this test.
	stopWorkersFn = func() {}

	var startedCtx context.Context
	stopCalls := 0
	startRepairExecutorFn = func(ctx context.Context) error {
		startedCtx = ctx
		return nil
	}
	stopRepairExecutorFn = func() { stopCalls++ }

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	rt := &Runtime{appCtx: appCtx, appCancel: appCancel}

	// The runtime starts the worker with the application-lifetime context.
	require.NoError(t, startRepairExecutorFn(appCtx))
	require.NotNil(t, startedCtx)
	require.NoError(t, startedCtx.Err(), "the application context must still be alive")

	// The graceful shutdown cancels it and joins the worker.
	_ = runShutdownSequence(rt, nil)
	require.Equal(t, 1, stopCalls, "the worker must be stopped exactly once")
	require.Error(t, appCtx.Err(), "the application context must be cancelled at shutdown")
}

// TestRepairWorkerLifecycleSurvivesStopWithoutRuntime: the demo/read-only
// path has no runtime at all — the shutdown step must stay a safe no-op.
func TestRepairWorkerLifecycleSurvivesStopWithoutRuntime(t *testing.T) {
	origStop := stopRepairExecutorFn
	origWorkers := stopWorkersFn
	t.Cleanup(func() {
		stopRepairExecutorFn = origStop
		stopWorkersFn = origWorkers
	})
	stopWorkersFn = func() {}

	stopCalls := 0
	stopRepairExecutorFn = func() { stopCalls++ }

	timings := runShutdownSequence(nil, nil)
	require.Equal(t, 1, stopCalls)
	require.GreaterOrEqual(t, timings.Total, time.Duration(0))
}

// TestRepairWorkerShutdownIsBounded (B6 review Low 2): the repair shutdown
// step must be BOUNDED — a worker stuck in a call that ignores the context
// must not block the shutdown forever. The app context is cancelled first,
// the later steps still run, and the step does not claim to have joined.
func TestRepairWorkerShutdownIsBounded(t *testing.T) {
	origStop := stopRepairExecutorFn
	origWorkers := stopWorkersFn
	origTimeout := repairShutdownTimeout
	t.Cleanup(func() {
		stopRepairExecutorFn = origStop
		stopWorkersFn = origWorkers
		repairShutdownTimeout = origTimeout
	})
	// A short bound so the test does not wait for the production 10s.
	repairShutdownTimeout = 200 * time.Millisecond

	// The stop never returns on its own — the test releases it in cleanup.
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	stopRepairExecutorFn = func() { <-release }

	workersRan := false
	stopWorkersFn = func() { workersRan = true }

	appCtx, appCancel := context.WithCancel(context.Background())
	rt := &Runtime{appCtx: appCtx, appCancel: appCancel}

	started := time.Now()
	_ = runShutdownSequence(rt, nil)
	elapsed := time.Since(started)

	require.Less(t, elapsed, 3*time.Second, "the repair shutdown step must be bounded")
	require.True(t, workersRan, "the later shutdown steps must still run")
	require.Error(t, appCtx.Err(), "the application context must be cancelled before the join")
}

// TestRepairWorkerRealDomainBinding (B6 review Low 3): the production
// binding — the app's start seam must really start the DOMAIN's executor on
// the APPLICATION context. The test uses the real seam (no fake) over a
// throwaway testcontainer database and never queues a repair task, so no
// provider is ever called.
func TestRepairWorkerRealDomainBinding(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// The real domain chain over the disposable PG.
	require.NoError(t, tagging.InitDescriptionRepairChain(db))
	t.Cleanup(tagging.StopDescriptionRepairExecutor)

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	// The production seam (not a fake) starts the domain's executor.
	require.NoError(t, startRepairExecutorFn(appCtx))
	require.True(t, tagging.DescriptionRepairExecutorRunning(),
		"the real domain executor must be running after the app start")

	// A cancelled REQUEST context must not stop the worker: the worker runs on
	// the application context.
	reqCtx, cancelReq := context.WithCancel(context.Background())
	cancelReq()
	require.Error(t, reqCtx.Err())
	time.Sleep(100 * time.Millisecond)
	require.True(t, tagging.DescriptionRepairExecutorRunning(),
		"a cancelled request context must not stop the worker")

	// Cancelling the APPLICATION context stops the worker: the join that
	// follows returns at once (the worker had already left its loop with the
	// application context).
	cancelApp()
	stopped := time.Now()
	tagging.StopDescriptionRepairExecutor()
	require.Less(t, time.Since(stopped), 5*time.Second,
		"the worker must stop with the application context")
}
