package auxrepair

// board-health-triage-advice seam B5 (task 2.5) — the safe execution loop
// (claim/lease, heartbeat, restart recovery) and the explicit retry, against
// a real testcontainer Postgres.
//
// Covered: concurrent executors — only one claims a task; a blocking embedder
// that outlives the INITIAL lease still succeeds because the heartbeat renews
// it; a lost ownership is never revived and the stale actor cannot commit; a
// committed task is resumed by a NEW executor without another embedding and
// without touching the committed version; an interrupted (unsaved) task is
// safely recovered; an explicit failure waits for the USER's retry (the
// scanner never burns AI quota in a loop); the retry semantics (no-op for a
// running/completed task without touching the row, 409 for a terminal task);
// repeated Start/Stop without leaks.
//
// The tests never touch the production library: every run is against the
// throwaway testcontainer.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/board"
)

// auxRepairTestExecutorConfig is a fast, deterministic configuration: the
// lease is short enough that a test can outlive it, the heartbeat is well
// below it.
func auxRepairTestExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		PollInterval:      20 * time.Millisecond,
		LeaseDuration:     300 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		HeartbeatTimeout:  200 * time.Millisecond,
		MaxBatch:          1,
	}
}

// waitForCondition waits for a condition with a bounded timeout (the tests
// never wait forever).
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", message)
}

// ── claim arbitration ───────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecutorConcurrentClaimSingleWinner: several
// executor instances scan the same queue — exactly ONE of them claims the
// task, and the embedding runs once.
func TestAuxiliaryDescriptionRepairExecutorConcurrentClaimSingleWinner(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "上海", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	stubA := &stubEmbedder{vector: auxRepairTestVectorB()}
	stubB := &stubEmbedder{vector: auxRepairTestVectorB()}
	svcA, err := NewService(db, WithEmbedder(stubA.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	svcB, err := NewService(db, WithEmbedder(stubB.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	execA, err := NewExecutor(svcA, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	execB, err := NewExecutor(svcB, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	job := createAuxExecJob(t, svcA, aux.ID, "concurrent-claim", "中国东部沿海的直辖市")

	var wg sync.WaitGroup
	counts := make([]int, 2)
	for i, exec := range []*Executor{execA, execB} {
		wg.Add(1)
		go func(i int, exec *Executor) {
			defer wg.Done()
			n, _ := exec.RunOnce(context.Background())
			counts[i] = n
		}(i, exec)
	}
	wg.Wait()

	require.Equal(t, 1, counts[0]+counts[1], "only one executor may claim the task")
	require.Equal(t, 1, stubA.callCount()+stubB.callCount(), "only one embedding call")
	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted)
}

// ── heartbeat ───────────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecutorHeartbeatKeepsLeaseAcrossTTL: the
// embedding blocks LONGER than the initial lease — only the heartbeat can
// keep the ownership alive, and the save then succeeds.
func TestAuxiliaryDescriptionRepairExecutorHeartbeatKeepsLeaseAcrossTTL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "广州", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")
	before := loadAuxExecLabel(t, db, aux.ID)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	stub := &stubEmbedder{
		vector: auxRepairTestVectorB(),
		hook: func() {
			once.Do(func() {
				entered <- struct{}{}
				<-release
			})
		},
	}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	cfg := auxRepairTestExecutorConfig()
	exec, err := NewExecutor(svc, cfg)
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "hb-keeps-lease", "南方的中心城市")

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = exec.RunOnce(context.Background())
	}()
	<-entered

	// Block past the INITIAL lease: only the heartbeat can keep it alive.
	time.Sleep(cfg.LeaseDuration + 300*time.Millisecond)
	close(release)

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("the executor round did not finish")
	}

	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted, "the heartbeat must have kept the lease alive")
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "南方的中心城市", after.Description)
	require.NotEqual(t, before.Embedding == nil, true)
}

// TestAuxiliaryDescriptionRepairExecutorHeartbeatLossDoesNotReviveLease: the
// ownership is taken over while the embedding runs — the heartbeat must NOT
// revive the (already expired) lease, and the stale actor must not commit.
func TestAuxiliaryDescriptionRepairExecutorHeartbeatLossDoesNotReviveLease(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "深圳", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")
	before := loadAuxExecLabel(t, db, aux.ID)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	stub := &stubEmbedder{
		vector: auxRepairTestVectorB(),
		hook: func() {
			once.Do(func() {
				entered <- struct{}{}
				<-release
			})
		},
	}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	cfg := auxRepairTestExecutorConfig()
	exec, err := NewExecutor(svc, cfg)
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "hb-lost", "南方的窗口城市")

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = exec.RunOnce(context.Background())
	}()
	<-entered

	// Another executor takes the job over with an ALREADY expired lease.
	expired := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{"claim_owner": "other-owner", "lease_expires_at": expired}).Error)

	// Give the heartbeat time to try (and fail).
	time.Sleep(4 * cfg.HeartbeatInterval)

	var lease time.Time
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Pluck("lease_expires_at", &lease).Error)
	require.False(t, lease.After(time.Now()), "the heartbeat must not revive an expired lease")

	close(release)
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("the executor round did not finish")
	}

	persisted := loadAuxExecJob(t, db, job.ID)
	require.False(t, persisted.DescriptionCommitted, "the stale actor must not commit")
	require.Equal(t, "other-owner", *persisted.ClaimOwner, "the new owner must not be stepped on")
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, after.Description)
	require.Equal(t, before.Embedding, after.Embedding)
}

// ── restart recovery ────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecutorRecoversCommittedTaskWithoutReembedding:
// a task whose description was committed but whose matching attempt died is
// resumed by a NEW executor instance — without another embedding and without
// touching the committed version.
func TestAuxiliaryDescriptionRepairExecutorRecoversCommittedTaskWithoutReembedding(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "成都", "active", "旧描述")
	seedAuxExecTag(t, db, "标签一", "active")
	seedAuxExecTag(t, db, "标签二", "active")

	saveStub := &stubEmbedder{vector: auxRepairTestVectorB()}
	saveSvc, err := NewService(db, WithEmbedder(saveStub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	job := prepareCommittedRepairJob(t, saveSvc, aux.ID, "recover-committed", "西部的中心城市")
	require.Equal(t, 1, saveStub.callCount())

	// The matching attempt died: the lease expired, the description stays
	// committed.
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{
			"status":           tagmodels.AuxiliaryDescriptionRepairStatusMatching,
			"stage":            tagmodels.AuxiliaryDescriptionRepairStageMatching,
			"lease_expires_at": time.Now().Add(-time.Minute),
		}).Error)
	committedBefore := loadAuxExecJob(t, db, job.ID)
	require.NotNil(t, committedBefore.CommittedVersion)

	// A NEW executor (a new service instance over the same DB) resumes it.
	recoverStub := &stubEmbedder{vector: auxRepairTestVectorB()}
	recoverSvc, err := NewService(db, WithEmbedder(recoverStub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	exec, err := NewExecutor(recoverSvc, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 0, recoverStub.callCount(), "a committed task must never be embedded again")

	after := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, *committedBefore.CommittedVersion, *after.CommittedVersion,
		"the committed version must not change")
	require.Equal(t, committedBefore.CommittedAt.UTC(), after.CommittedAt.UTC(),
		"the committed timestamp must not change")
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusCompleted, after.Status)
	require.Equal(t, 2, after.Processed)
	require.Equal(t, 0, jobView(&after).Remaining, "nothing may remain after the recovery")
}

// TestAuxiliaryDescriptionRepairExecutorRecoversInterruptedUncommittedTask: an
// attempt that died BEFORE the description was committed is safely recovered —
// the embedding runs again (nothing was saved) and the save succeeds.
func TestAuxiliaryDescriptionRepairExecutorRecoversInterruptedUncommittedTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "武汉", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")
	before := loadAuxExecLabel(t, db, aux.ID)

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "recover-uncommitted", "江城的别称")

	// The attempt died inside the embedding phase: the phase state is left
	// behind and the lease expired.
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{
			"status":           tagmodels.AuxiliaryDescriptionRepairStatusEmbedding,
			"stage":            tagmodels.AuxiliaryDescriptionRepairStageEmbedding,
			"lease_expires_at": time.Now().Add(-time.Minute),
		}).Error)

	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 1, stub.callCount(), "the interrupted attempt is embedded again")

	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted)
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "江城的别称", after.Description)
	require.NotEqual(t, before.Embedding == nil, true)
}

// ── the automatic boundary: failures wait for the user's retry ──────────────

// TestAuxiliaryDescriptionRepairExecutorDoesNotAutoRetryFailedTask: a failed
// task is NOT picked up again by the scanner (no AI burn in a loop); only the
// explicit retry re-queues it — and then exactly ONE new attempt runs.
func TestAuxiliaryDescriptionRepairExecutorDoesNotAutoRetryFailedTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "南京", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	stub := &stubEmbedder{
		vector: auxRepairTestVectorB(),
		failCalls: map[int]error{
			0: errors.New("provider exploded"),
		},
	}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "no-auto-retry", "六朝古都")

	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	// The first attempt fails.
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	failed := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed, failed.Status)
	require.Equal(t, 1, stub.callCount())

	// The scanner must NOT run it again — an empty queue.
	for i := 0; i < 3; i++ {
		n, err = exec.RunOnce(context.Background())
		require.NoError(t, err)
		require.Equal(t, 0, n, "a failed task must not be picked up automatically")
	}
	require.Equal(t, 1, stub.callCount(), "a failure must not burn AI quota in a loop")

	// The explicit retry re-queues the SAME task.
	requeued, err := svc.RetryJob(context.Background(), aux.ID, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.ID, requeued.ID, "the retry re-uses the same task")
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, requeued.Status)
	require.Empty(t, requeued.Error, "the old failure message must be cleared")
	require.Equal(t, job.ExpectedVersion, requeued.ExpectedVersion, "the source version must not change")
	require.Equal(t, job.NewDescription, requeued.NewDescription, "the confirmed description must not change")
	require.Equal(t, job.Total, requeued.Total, "the frozen scope must not change")

	// Now exactly ONE new attempt runs — and it succeeds.
	n, err = exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 2, stub.callCount())
	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusCompleted, persisted.Status,
		"one attempt continues from the embedding into the matching stage")
}

// ── retry semantics ─────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairRetrySemantics: a running or completed task
// is a NO-OP (not even the timestamp changes), a terminal task is refused and
// an unknown/foreign task is not found.
func TestAuxiliaryDescriptionRepairRetrySemantics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// Each scenario needs its OWN auxiliary label: the per-label creation
	// mutex allows only one unfinished task per label.
	runningLabel := seedAuxExecLabel(t, db, "杭州", "active", "旧描述")
	completedLabel := seedAuxExecLabel(t, db, "苏州", "active", "旧描述")
	conflictLabel := seedAuxExecLabel(t, db, "南京", "active", "旧描述")
	unavailableLabel := seedAuxExecLabel(t, db, "青岛", "active", "旧描述")
	other := seedAuxExecLabel(t, db, "别的标签", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)

	// A RUNNING task (a live lease) → the retry changes nothing.
	running := createAuxExecJob(t, svc, runningLabel.ID, "retry-running", "新描述")
	_, err = svc.ClaimJobForExecution(context.Background(), running.ID, "executor-1", time.Minute)
	require.NoError(t, err)
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", running.ID).
		Update("status", tagmodels.AuxiliaryDescriptionRepairStatusEmbedding).Error)
	beforeRunning := loadAuxExecJob(t, db, running.ID)
	view, err := svc.RetryJob(context.Background(), runningLabel.ID, running.ID)
	require.NoError(t, err)
	require.Equal(t, running.ID, view.ID)
	afterRunning := loadAuxExecJob(t, db, running.ID)
	require.Equal(t, beforeRunning.UpdatedAt.UTC(), afterRunning.UpdatedAt.UTC(),
		"a running task must not even be touched")
	require.Equal(t, beforeRunning.Status, afterRunning.Status)
	require.Equal(t, beforeRunning.Total, afterRunning.Total)

	// A COMPLETED task → the retry changes nothing.
	completed := createAuxExecJob(t, svc, completedLabel.ID, "retry-completed", "另一个描述")
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", completed.ID).
		Update("status", tagmodels.AuxiliaryDescriptionRepairStatusCompleted).Error)
	beforeCompleted := loadAuxExecJob(t, db, completed.ID)
	view, err = svc.RetryJob(context.Background(), completedLabel.ID, completed.ID)
	require.NoError(t, err)
	require.Equal(t, completed.ID, view.ID)
	afterCompleted := loadAuxExecJob(t, db, completed.ID)
	require.Equal(t, beforeCompleted.UpdatedAt.UTC(), afterCompleted.UpdatedAt.UTC(),
		"a completed task must not even be touched")

	// A TERMINAL task (conflict / unavailable) → refused.
	for _, tc := range []struct {
		status string
		label  models.SemanticLabel
	}{
		{tagmodels.AuxiliaryDescriptionRepairStatusConflict, conflictLabel},
		{tagmodels.AuxiliaryDescriptionRepairStatusUnavailable, unavailableLabel},
	} {
		terminalJob := createAuxExecJob(t, svc, tc.label.ID, "retry-"+tc.status, "终态描述")
		require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", terminalJob.ID).
			Update("status", tc.status).Error)
		_, err = svc.RetryJob(context.Background(), tc.label.ID, terminalJob.ID)
		require.ErrorIs(t, err, ErrStageRetryNotAllowed, "status=%s must not be retryable", tc.status)
	}

	// An unknown task → not found; a task of ANOTHER label → not found.
	_, err = svc.RetryJob(context.Background(), runningLabel.ID, 999999)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = svc.RetryJob(context.Background(), other.ID, running.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "a task of another label must not be retryable")
}

// ── lifecycle ───────────────────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecutorLifecycle: Start is idempotent (no
// second worker), Stop is idempotent and the worker really stops.
func TestAuxiliaryDescriptionRepairExecutorLifecycle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "西安", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	require.False(t, exec.Running(), "a new executor does not start itself")
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	require.NoError(t, exec.Start(appCtx))
	require.NoError(t, exec.Start(appCtx), "Start must be idempotent")
	require.True(t, exec.Running())

	job := createAuxExecJob(t, svc, aux.ID, "lifecycle", "十三朝古都")
	waitForCondition(t, 10*time.Second, func() bool {
		return loadAuxExecJob(t, db, job.ID).DescriptionCommitted
	}, "the running worker to pick the task up")
	require.Equal(t, 1, stub.callCount(), "the task runs exactly once")

	// Stop is idempotent and really stops the worker.
	exec.Stop()
	exec.Stop()
	require.False(t, exec.Running())

	// A task queued AFTER the stop must stay untouched (the per-label mutex
	// needs its own label here).
	afterStopLabel := seedAuxExecLabel(t, db, "另一个标签", "active", "旧描述")
	second := createAuxExecJob(t, svc, afterStopLabel.ID, "lifecycle-after-stop", "停止之后的描述")
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, 1, stub.callCount(), "the stopped worker must not pick anything up")
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending,
		loadAuxExecJob(t, db, second.ID).Status)
}

// ── the submitted task survives the request context ────────────────────────

// TestAuxiliaryDescriptionRepairExecutorTaskSurvivesRequestContext: the
// worker runs on the APPLICATION context — a cancelled request context must
// not cancel the confirmed task.
func TestAuxiliaryDescriptionRepairExecutorTaskSurvivesRequestContext(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "苏州", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)

	// The task is created with a request context that is cancelled right
	// after the response.
	reqCtx, cancelReq := context.WithCancel(context.Background())
	job := createAuxExecJob(t, svc, aux.ID, "req-ctx", "江南水乡")
	cancelReq()
	require.Error(t, reqCtx.Err())

	// The worker uses the application context.
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	require.NoError(t, exec.Start(appCtx))
	defer exec.Stop()

	waitForCondition(t, 10*time.Second, func() bool {
		return loadAuxExecJob(t, db, job.ID).DescriptionCommitted
	}, "the task to run despite the cancelled request context")
	require.Equal(t, 1, stub.callCount())
}

// ── nothing else is written ─────────────────────────────────────────────────

// TestAuxiliaryDescriptionRepairExecutorWritesOnlyJobAndItems: the executor
// touches the repair tables and the match table (the engine's own writes) —
// never a second task, the label's identity, the upgrade suggestions or the
// health snapshots.
func TestAuxiliaryDescriptionRepairExecutorWritesOnlyJobAndItems(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "天津", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")
	before := loadAuxExecLabel(t, db, aux.ID)

	boardLabel := models.SemanticLabel{Label: "华北", Slug: "exec-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardLabel).Error)
	require.NoError(t, db.Create(&tagmodels.BoardUpgradeSuggestion{
		BatchID: "b1", Mode: "create_new", Decision: "create_new", BoardLabel: "华北",
		Description: "旧建议", SuggestionHash: "exec-hash", Status: "pending",
	}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID: boardLabel.ID, CheckedAt: time.Now(),
		Metrics: tagmodels.BoardHealthMetrics{
			Board:  tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}},
			Labels: []tagmodels.BoardHealthLabelMetrics{},
		},
	}).Error)

	stub := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db, WithEmbedder(stub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "only-job", "北方的港口城市")
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var jobs int64
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Count(&jobs).Error)
	require.Equal(t, int64(1), jobs, "exactly one persisted task")
	var suggestions, snapshots int64
	require.NoError(t, db.Model(&tagmodels.BoardUpgradeSuggestion{}).Count(&suggestions).Error)
	require.Equal(t, int64(1), suggestions)
	require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&snapshots).Error)
	require.Equal(t, int64(1), snapshots)

	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Label, after.Label)
	require.Equal(t, before.Aliases, after.Aliases)
	require.Equal(t, before.MergeEmbedding, after.MergeEmbedding)
	require.Equal(t, "北方的港口城市", after.Description)
	require.NotNil(t, after.Embedding)
	_ = fmt.Sprintf("%v", job)
}

// ── configuration & notification (pure, no DB) ───────────────────────────────

// TestAuxiliaryDescriptionRepairExecutorConfigValidation: the configuration
// must have positive durations, and the heartbeat must be shorter than the
// lease (otherwise a live attempt would look expired between two beats).
func TestAuxiliaryDescriptionRepairExecutorConfigValidation(t *testing.T) {
	valid := DefaultExecutorConfig()
	require.NoError(t, valid.Validate())

	broken := []struct {
		name   string
		mutate func(c *ExecutorConfig)
	}{
		{name: "poll interval zero", mutate: func(c *ExecutorConfig) { c.PollInterval = 0 }},
		{name: "poll interval negative", mutate: func(c *ExecutorConfig) { c.PollInterval = -time.Second }},
		{name: "lease zero", mutate: func(c *ExecutorConfig) { c.LeaseDuration = 0 }},
		{name: "heartbeat zero", mutate: func(c *ExecutorConfig) { c.HeartbeatInterval = 0 }},
		{name: "heartbeat timeout zero", mutate: func(c *ExecutorConfig) { c.HeartbeatTimeout = 0 }},
		{name: "heartbeat not shorter than the lease", mutate: func(c *ExecutorConfig) { c.HeartbeatInterval = c.LeaseDuration }},
		{name: "batch below one", mutate: func(c *ExecutorConfig) { c.MaxBatch = 0 }},
	}
	for _, tc := range broken {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			require.Error(t, cfg.Validate())
		})
	}
}

// TestAuxiliaryDescriptionRepairExecutorNotifyWithoutWorker: the wakeup is a
// non-blocking hint — with no worker running it is simply dropped, it never
// starts a worker and never blocks the caller.
func TestAuxiliaryDescriptionRepairExecutorNotifyWithoutWorker(t *testing.T) {
	exec, err := NewExecutor(&Service{}, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	require.False(t, exec.Running())

	done := make(chan struct{})
	go func() {
		defer close(done)
		exec.Notify()
		exec.Notify()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("Notify must never block")
	}
	require.False(t, exec.Running(), "a notification must not start a worker")
}

// TestAuxiliaryDescriptionRepairExecutorResumesTaskWithNullLease (B6
// pre-flight): a SAVED matching task whose claim and lease were RELEASED
// (owner/lease = NULL — the state the retry path and the end of a phase leave
// behind) must still be picked up by the scanner. A predicate that only
// accepted an EXPIRED (non-NULL) lease would strand it forever.
func TestAuxiliaryDescriptionRepairExecutorResumesTaskWithNullLease(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "青岛", "active", "旧描述")
	seedAuxExecTag(t, db, "标签一", "active")
	seedAuxExecTag(t, db, "标签二", "active")

	saveStub := &stubEmbedder{vector: auxRepairTestVectorB()}
	saveSvc, err := NewService(db, WithEmbedder(saveStub.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	job := prepareCommittedRepairJob(t, saveSvc, aux.ID, "null-lease", "海上的花园城市")

	// The phase ended / was released: the claim and the lease are NULL.
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{
			"status":           tagmodels.AuxiliaryDescriptionRepairStatusMatching,
			"stage":            tagmodels.AuxiliaryDescriptionRepairStageMatching,
			"claim_owner":      nil,
			"claimed_at":       nil,
			"lease_expires_at": nil,
		}).Error)
	var released tagmodels.AuxiliaryDescriptionRepairJob
	require.NoError(t, db.Where("id = ?", job.ID).First(&released).Error)
	require.Nil(t, released.ClaimOwner)
	require.Nil(t, released.LeaseExpiresAt)

	// The REAL executor path (RunOnce → the scanner) must pick it up.
	exec, err := NewExecutor(saveSvc, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n, "a released (NULL lease) task must be claimable")
	require.Equal(t, 1, saveStub.callCount(), "a committed task is never embedded again")

	after := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusCompleted, after.Status)
	require.Equal(t, 2, after.Processed)
	require.Equal(t, 0, jobView(&after).Remaining)
}

// ── review fixes: panic isolation, database-clock leases, phase CAS ────────

// panicEmbedder is a controlled panic stub: it panics instead of returning an
// error (the worst case for a background worker).
type panicEmbedder struct {
	mu       sync.Mutex
	panicked bool
}

func (p *panicEmbedder) embed(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	p.mu.Lock()
	// Only the FIRST call panics — the following task must be processed
	// normally (the worker survived the panic).
	if p.panicked {
		p.mu.Unlock()
		return auxRepairTestVectorB(), nil, nil
	}
	p.panicked = true
	p.mu.Unlock()
	panic("embedding unit exploded")
}

// panicMatcher is a controlled panic stub for the matching engine.
type panicMatcher struct {
	mu       sync.Mutex
	panicked bool
}

func (m *panicMatcher) MatchTopicTag(ctx context.Context, topicTagID uint) ([]board.SemanticBoardMatchResult, error) {
	m.mu.Lock()
	m.panicked = true
	m.mu.Unlock()
	panic("matching engine exploded")
}

// TestAuxiliaryDescriptionRepairPanicInEmbeddingKeepsTaskUncommitted (review
// Medium): a panic in the embedding unit must not take the process down; the
// task stays UNCOMMITTED and records the retryable embedding_failed state.
func TestAuxiliaryDescriptionRepairPanicInEmbeddingKeepsTaskUncommitted(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "上海", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")
	before := loadAuxExecLabel(t, db, aux.ID)

	embedder := &panicEmbedder{}
	svc, err := NewService(db, WithEmbedder(embedder.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "panic-embedding", "中国东部沿海的直辖市")

	// The panic must be contained: the worker lives, the attempt is recorded
	// as a stage failure (the task state is the truth).
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n, "the attempt ran — its failure is recorded in the task state")

	persisted := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed, persisted.Status)
	require.False(t, persisted.DescriptionCommitted, "a panic before the save must not commit anything")
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, before.Description, after.Description)
	require.Equal(t, before.Embedding, after.Embedding)

	// The failed task is NOT picked up again automatically (no panic loop).
	n, err = exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

// TestAuxiliaryDescriptionRepairPanicInMatchingKeepsCommittedData (review
// Medium): a panic in the matching engine must not roll the ALREADY COMMITTED
// description back — the database is the truth: the saved description and
// vector stay, the task becomes matching_failed (a retryable state the scanner
// never picks up on its own).
func TestAuxiliaryDescriptionRepairPanicInMatchingKeepsCommittedData(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "广州", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	embedder := &stubEmbedder{vector: auxRepairTestVectorB()}
	matcher := &panicMatcher{}
	svc, err := NewService(db,
		WithEmbedder(embedder.embed),
		WithMatcher(matcher),
		WithCacheInvalidator(func() {}),
	)
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	job := createAuxExecJob(t, svc, aux.ID, "panic-matching", "南方的中心城市")
	// ONE attempt: the embedding phase commits the description and the
	// matching phase (the same attempt) hits the panicking engine.
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	persisted := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusMatchingFailed, persisted.Status,
		"the committed description means the MATCHING stage failed")
	require.True(t, persisted.DescriptionCommitted, "the committed description must not be rolled back")
	after := loadAuxExecLabel(t, db, aux.ID)
	require.Equal(t, "南方的中心城市", after.Description, "the saved description must stay")
	require.NotNil(t, after.Embedding, "the saved storage vector must stay")
	require.Equal(t, 1, embedder.callCount(), "the embedding must not run again")

	// The failed task is not picked up automatically.
	n, err = exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

// TestAuxiliaryDescriptionRepairExecutorKeepsWorkingAfterPanic (review
// Medium): a panic in one task must not stop the worker — the next task is
// still processed.
func TestAuxiliaryDescriptionRepairExecutorKeepsWorkingAfterPanic(t *testing.T) {
	db := testutil.SetupTestDB(t)
	panicking := seedAuxExecLabel(t, db, "会崩的标签", "active", "旧描述")
	healthy := seedAuxExecLabel(t, db, "正常的标签", "active", "旧描述")
	seedAuxExecTag(t, db, "标签一", "active")
	seedAuxExecTag(t, db, "标签二", "active")

	embedder := &panicEmbedder{}
	svc, err := NewService(db, WithEmbedder(embedder.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)

	panickingJob := createAuxExecJob(t, svc, panicking.ID, "panic-first", "会崩的描述")
	healthyJob := createAuxExecJob(t, svc, healthy.ID, "panic-second", "正常的描述")

	// The first task panics — the worker survives it.
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusEmbeddingFailed,
		loadAuxExecJob(t, db, panickingJob.ID).Status)

	// The next task is still processed (the worker is alive).
	n, err = exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.True(t, loadAuxExecJob(t, db, healthyJob.ID).DescriptionCommitted,
		"the worker must keep processing the other tasks")
}

// TestAuxiliaryDescriptionRepairExecutorLeaseUsesDatabaseClock (review Low 2):
// the lease deadline is computed by the DATABASE clock — an application clock
// far behind must not produce a lease that is already expired (which would
// make the worker re-claim the same task in a hot loop).
func TestAuxiliaryDescriptionRepairExecutorLeaseUsesDatabaseClock(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "深圳", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	// The application clock is one hour behind the database.
	skewed := time.Now().Add(-time.Hour)
	embedder := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db,
		WithEmbedder(embedder.embed),
		WithCacheInvalidator(func() {}),
		WithClock(func() time.Time { return skewed }),
	)
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "skewed-clock", "南方的窗口城市")

	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// The lease the database wrote is in the future (the DB clock), so the
	// attempt was NOT refused and the task finished.
	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted,
		"a skewed application clock must not expire the fresh lease")
	require.Equal(t, 1, embedder.callCount(), "the task must not be re-claimed in a hot loop")

	// The lease really comes from the DATABASE clock: a probe claim on another
	// task writes a deadline that is in the future even though the
	// application clock is an hour behind.
	probeLabel := seedAuxExecLabel(t, db, "租约探针", "active", "旧描述")
	probeJob := createAuxExecJob(t, svc, probeLabel.ID, "skewed-probe", "探针描述")
	_, err = svc.repo.ClaimRepairJob(context.Background(), probeJob.ID, "probe-owner", time.Minute)
	require.NoError(t, err)
	var lease time.Time
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", probeJob.ID).
		Pluck("lease_expires_at", &lease).Error)
	require.True(t, lease.After(time.Now()), "the lease must come from the database clock")
}

// TestAuxiliaryDescriptionRepairPhaseAcquireRejectsForeignToken (review
// Low 3): the phase CAS must check the affected rows — a job whose phase token
// is a non-NULL empty string is NOT free, the attempt is refused as busy and
// nothing is touched.
func TestAuxiliaryDescriptionRepairPhaseAcquireRejectsForeignToken(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "杭州", "active", "旧描述")
	seedAuxExecTag(t, db, "标签", "active")

	embedder := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db, WithEmbedder(embedder.embed), WithCacheInvalidator(func() {}))
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "foreign-token", "江南水乡")
	// The claim comes first (a takeover would void the planted token).
	_, err = svc.ClaimJobForExecution(context.Background(), job.ID, "executor-1", time.Minute)
	require.NoError(t, err)

	// A non-NULL empty phase token (defensive case).
	require.NoError(t, db.Model(&tagmodels.AuxiliaryDescriptionRepairJob{}).Where("id = ?", job.ID).
		Update("phase_token", "").Error)

	_, err = svc.ExecuteEmbeddingPhase(context.Background(), job.ID, "executor-1")
	require.ErrorIs(t, err, ErrStageBusy, "a foreign phase token must refuse the attempt")
	require.Equal(t, 0, embedder.callCount(), "a refused attempt must not embed")

	persisted := loadAuxExecJob(t, db, job.ID)
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusPending, persisted.Status,
		"the refused attempt must not change the task state")
	require.NotNil(t, persisted.PhaseToken)
	require.Equal(t, "", *persisted.PhaseToken)
}

// TestAuxiliaryDescriptionRepairExecutorLeaseClockAheadStillCompletes (B6
// review Low 1): the lease belongs to the DATABASE clock. An application clock
// that runs AHEAD of the database must not veto a live lease — the attempt has
// to complete instead of being re-claimed forever.
func TestAuxiliaryDescriptionRepairExecutorLeaseClockAheadStillCompletes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	aux := seedAuxExecLabel(t, db, "合肥", "active", "旧描述")
	seedAuxExecTag(t, db, "标签一", "active")
	seedAuxExecTag(t, db, "标签二", "active")

	// The application clock is ONE HOUR AHEAD of the database.
	ahead := time.Now().Add(time.Hour)
	embedder := &stubEmbedder{vector: auxRepairTestVectorB()}
	svc, err := NewService(db,
		WithEmbedder(embedder.embed),
		WithCacheInvalidator(func() {}),
		WithClock(func() time.Time { return ahead }),
	)
	require.NoError(t, err)
	exec, err := NewExecutor(svc, auxRepairTestExecutorConfig())
	require.NoError(t, err)
	job := createAuxExecJob(t, svc, aux.ID, "clock-ahead", "江淮之间的城市")

	// The whole chain (embedding → save → matching) completes in ONE attempt:
	// the database clock considers the lease live.
	n, err := exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	persisted := loadAuxExecJob(t, db, job.ID)
	require.True(t, persisted.DescriptionCommitted, "a clock ahead must not refuse the attempt")
	require.Equal(t, tagmodels.AuxiliaryDescriptionRepairStatusCompleted, persisted.Status)
	require.Equal(t, 2, persisted.Processed)
	require.Equal(t, 1, embedder.callCount(), "no hot re-claim: the embedding runs once")

	// And the task is not picked up again (no loop).
	n, err = exec.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
}
