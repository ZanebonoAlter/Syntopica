package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

// ── 分析异步执行器（fix-board-analysis-material 8.x）────────────────────────
//
// 背景：循环 B 分析（含补全门备料）是 10 分钟级长任务。同步 HTTP 下客户端
// 断连（离开页面/关标签页/网络抖动）会把 request-context 的 cancel 传播进
// 整条分析链——2026-08-27 实锤：跑满 10 分钟的 analyze 环节 "context canceled"，
// 全部 LLM 调用作废、无报告落库。
//
// 模型：trigger 立即返回 202，分析在独立 context 的 goroutine 里跑完落库；
// 前端轮询 status 接口拿 running/finished/error/result_id。单实例单用户产品，
// 内存态足够（进程重启 = 分析本就死了，无状态可恢复）。

const (
	// analysisJobTimeout 是单次分析（含补全门 40 次 LLM 上限 + agent loop）的
	// 宽松上限；到点强杀防止 goroutine 泄漏。
	analysisJobTimeout = 30 * time.Minute

	AnalysisScopeBoard = "board"
	AnalysisScopeTopic = "topic"

	// Job kinds（D9）：同一 runner 承载三种异步分析，前端按 kind 分派
	// 状态语义（brief 与 investigation 视觉/轮询隔离），topic 档保持旧行为。
	AnalysisJobKindTopic              = "topic_analysis"
	AnalysisJobKindBoardBrief         = "board_brief"
	AnalysisJobKindBoardInvestigation = "board_investigation"
)

// ErrAlreadyRunning marks a rejected duplicate trigger (sentinel; the
// concrete rejection carries the running job's identity via RunningJobError).
var ErrAlreadyRunning = errors.New("analysis already running")

// RunningJobError is returned by Start when the same (scope, target) already
// has a running job. Current carries the full identity (job_id/job_kind/
// scope/target_id/running) so 409 responses can tell pollers WHICH job is
// running — a board brief must not be mistaken for an investigation.
type RunningJobError struct {
	Current AnalysisStatus
}

func (e *RunningJobError) Error() string {
	return "analysis already running: job " + e.Current.JobID + " (" + e.Current.JobKind + ")"
}

func (e *RunningJobError) Unwrap() error { return ErrAlreadyRunning }

// AnalysisStatus is the poller-facing snapshot of one analysis job.
type AnalysisStatus struct {
	JobID     string    `json:"job_id"`
	JobKind   string    `json:"job_kind"`
	Scope     string    `json:"scope"`
	TargetID  uint      `json:"target_id"`
	Running   bool      `json:"running"`
	StartedAt time.Time `json:"started_at"`
	Finished  bool      `json:"finished"`
	Error     string    `json:"error,omitempty"`
	ResultID  uint      `json:"result_id,omitempty"`
}

type analysisJob struct {
	jobID     string
	kind      string
	scope     string
	targetID  uint
	startedAt time.Time
	done      bool
	err       string
	resultID  uint
}

func (j *analysisJob) snapshot() AnalysisStatus {
	return AnalysisStatus{
		JobID:     j.jobID,
		JobKind:   j.kind,
		Scope:     j.scope,
		TargetID:  j.targetID,
		Running:   !j.done,
		StartedAt: j.startedAt,
		Finished:  j.done,
		Error:     j.err,
		ResultID:  j.resultID,
	}
}

func jobKey(scope string, id uint) string {
	return scope + ":" + strconv.FormatUint(uint64(id), 10)
}

// analysisRunner serializes analysis jobs per (scope, id): a target already
// running rejects re-trigger (409 carries the running job's identity); every
// job — finished, errored, timed out or panicked — stays queryable by its
// unique job_id; the last job per target is kept for the board/topic status
// entry (re-entry recovery).
type analysisRunner struct {
	mu   sync.Mutex
	jobs map[string]*analysisJob // active/current slot per (scope, id)
	byID map[string]*analysisJob // every job ever started, keyed by job_id
}

func newAnalysisRunner() *analysisRunner {
	return &analysisRunner{
		jobs: map[string]*analysisJob{},
		byID: map[string]*analysisJob{},
	}
}

// Start launches fn in a detached goroutine and returns the new job's
// identity snapshot. If the same (scope, id) job is still running it returns
// a *RunningJobError carrying that job's identity (errors.Is(err,
// ErrAlreadyRunning) stays true). fn receives a context that survives the
// triggering HTTP request (client disconnects no longer kill the run) and
// returns the persisted result id for pollers.
func (r *analysisRunner) Start(scope string, id uint, kind string, timeout time.Duration, fn func(ctx context.Context) (uint, error)) (AnalysisStatus, error) {
	k := jobKey(scope, id)
	r.mu.Lock()
	if job, exists := r.jobs[k]; exists && !job.done {
		cur := job.snapshot()
		r.mu.Unlock()
		return AnalysisStatus{}, &RunningJobError{Current: cur}
	}
	job := &analysisJob{
		jobID:     r.newJobIDLocked(),
		kind:      kind,
		scope:     scope,
		targetID:  id,
		startedAt: time.Now(),
	}
	r.jobs[k] = job
	r.byID[job.jobID] = job
	st := job.snapshot()
	r.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		resultID, err := safeRun(ctx, fn)
		r.mu.Lock()
		defer r.mu.Unlock()
		if cur := r.jobs[k]; cur == job && !cur.done {
			cur.done = true
			cur.err = errString(err)
			cur.resultID = resultID
		}
	}()
	return st, nil
}

// Status returns the current (or last finished) job snapshot for a target —
// the re-entry recovery entry: after a page reload the frontend asks "what is
// this board doing right now" and gets the newest job for it.
func (r *analysisRunner) Status(scope string, id uint) (AnalysisStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[jobKey(scope, id)]
	if !ok {
		return AnalysisStatus{}, false
	}
	return j.snapshot(), true
}

// StatusByJobID returns one job by its unique id — running or already
// finished/errored/timed out/panicked (all terminal states stay queryable).
// Unknown ids report ok=false (process restart wipes the in-memory table).
func (r *analysisRunner) StatusByJobID(jobID string) (AnalysisStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.byID[jobID]
	if !ok {
		return AnalysisStatus{}, false
	}
	return j.snapshot(), true
}

// newJobIDLocked mints a unique non-empty job id (crypto/rand 24 hex chars);
// collisions are re-rolled so uniqueness holds even across many triggers.
func (r *analysisRunner) newJobIDLocked() string {
	for {
		id := randomJobID()
		if _, exists := r.byID[id]; !exists {
			return id
		}
	}
}

func randomJobID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand for all practical purposes cannot fail; fall back to a
		// time-based id rather than an empty one.
		return fmt.Sprintf("job%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// safeRun guards fn against panics so a crashing analysis surfaces as an error
// status instead of taking down the process.
func safeRun(ctx context.Context, fn func(ctx context.Context) (uint, error)) (resultID uint, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			resultID, err = 0, fmt.Errorf("analysis panic: %v", rec)
		}
	}()
	return fn(ctx)
}
