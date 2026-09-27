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

	// signalResearchJobTimeout 是 signal_research job（board-signal-reports
	// tasks 4.6）的独立超时，与 40 轮预算对齐：实测单轮 108~380s（均值
	// ~200s），150 分钟覆盖 40 轮 + compose 3 稿的余量。2026-09-22 真实验收
	// 两次 30 分钟超时均砍在第 9/12 轮、已付费轮次成果丢失，促成独立化。
	// discovery 保持 analysisJobTimeout（30 分钟）不动。
	signalResearchJobTimeout = 150 * time.Minute

	AnalysisScopeBoard = "board"
	AnalysisScopeTopic = "topic"

	// Job kinds（D9）：同一 runner 承载三种异步分析，前端按 kind 分派
	// 状态语义（brief 与 investigation 视觉/轮询隔离），topic 档保持旧行为。
	AnalysisJobKindTopic              = "topic_analysis"
	AnalysisJobKindBoardBrief         = "board_brief"
	AnalysisJobKindBoardInvestigation = "board_investigation"

	// Signal kinds（board-signal-reports design §7）：与上面三种 kind 共享
	// board 级 202/409 互斥；report 的触发 handler 属研究/成文批次（2b），
	// 常量先声明以保证状态契约完整（tasks 4.4 discovery 部分）。
	AnalysisJobKindBoardSignalDiscovery = "board_signal_discovery"
	AnalysisJobKindBoardSignalReport    = "board_signal_report"
)

// Signal job phases（design §7：phase=prepare|detect|research|compose；
// 不新增 review phase）。research/compose 由 2b 的 research job 使用。
const (
	SignalPhasePrepare  = "prepare"
	SignalPhaseDetect   = "detect"
	SignalPhaseResearch = "research"
	SignalPhaseCompose  = "compose"
)

// Signal job outcomes（JB-1/JB-2）：discovery 终态 discovered|no_signal|failed，
// research 终态 succeeded|failed；result_id 只在成功时存在（零信号无数值、
// 失败无半成品）。超时/panic 等未显式报告终态的信号任务由 runner 兑底 failed。
const (
	SignalOutcomeDiscovered = "discovered"
	SignalOutcomeNoSignal   = "no_signal"
	SignalOutcomeSucceeded  = "succeeded"
	SignalOutcomeFailed     = "failed"
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
	// Signal-job extensions（design §7，tasks 4.4 discovery 部分）。全部
	// omitempty 且只经 SignalJobPatch 写入——旧 kind 永远不碰这些字段，
	// 序列化输出逐字节不变（旧 kind 输出零变化）。
	Phase          string `json:"phase,omitempty"`
	Outcome        string `json:"outcome,omitempty"`
	ErrorStage     string `json:"error_stage,omitempty"`
	Granularity    string `json:"granularity,omitempty"`
	Period         string `json:"period,omitempty"`
	DiscoveryID    uint   `json:"discovery_id,omitempty"`
	CandidateID    uint   `json:"candidate_id,omitempty"`
	CandidateCount int    `json:"candidate_count,omitempty"`
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
	// signal-job extensions (mirror of AnalysisStatus, unexported side).
	phase          string
	outcome        string
	errorStage     string
	granularity    string
	period         string
	discoveryID    uint
	candidateID    uint
	candidateCount int
}

func (j *analysisJob) snapshot() AnalysisStatus {
	return AnalysisStatus{
		JobID:          j.jobID,
		JobKind:        j.kind,
		Scope:          j.scope,
		TargetID:       j.targetID,
		Running:        !j.done,
		StartedAt:      j.startedAt,
		Finished:       j.done,
		Error:          j.err,
		ResultID:       j.resultID,
		Phase:          j.phase,
		Outcome:        j.outcome,
		ErrorStage:     j.errorStage,
		Granularity:    j.granularity,
		Period:         j.period,
		DiscoveryID:    j.discoveryID,
		CandidateID:    j.candidateID,
		CandidateCount: j.candidateCount,
	}
}

// SignalJobPatch selectively updates one signal job's progress fields; zero
// values leave the corresponding field untouched（candidate_count 为 0 时由
// outcome=no_signal 表达，不需单独补 0）。只能经 StartSignal 发放的 reporter
// 调用；终态（done）后的 patch 一律忽略——终态不可变。
//
// ResultID（board-signal-reports 2b 增量）：research job 成功时由 job fn 经
// patch 上报保存后的 result id（StartSignal 的 fn 签名只返 error，无法像旧
// Start 那样经返回值带出 result_id——JB-2 要求 result_id 仅成功存在且出现在
// job 状态里）。旧 kind 从不调用 report，序列化输出不受影响。终态回写时
// fn 返回值仍优先，patch 值只在 fn 返 0 时保留。
type SignalJobPatch struct {
	Phase          string
	Outcome        string
	ErrorStage     string
	Granularity    string
	Period         string
	DiscoveryID    uint
	CandidateID    uint
	CandidateCount int
	ResultID       uint
}

func isSignalJobKind(kind string) bool {
	return kind == AnalysisJobKindBoardSignalDiscovery || kind == AnalysisJobKindBoardSignalReport
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
	return r.launch(scope, id, kind, timeout, func(ctx context.Context, _ func(SignalJobPatch)) (uint, error) {
		return fn(ctx)
	})
}

// StartSignal launches a signal job (board_signal_discovery；2b 起含
// board_signal_report). Locking/timeout/409 semantics identical to Start;
// fn additionally receives a progress reporter bound to the new job's
// identity — the goroutine-safe way to publish phase/outcome/counters while
// running（job fn 不能闭包捕获 Start 的返回值：goroutine 先于赋值启动）.
func (r *analysisRunner) StartSignal(scope string, id uint, kind string, timeout time.Duration, fn func(ctx context.Context, report func(SignalJobPatch)) error) (AnalysisStatus, error) {
	return r.launch(scope, id, kind, timeout, func(ctx context.Context, report func(SignalJobPatch)) (uint, error) {
		return 0, fn(ctx, report)
	})
}

// launch is the shared spawn path: slot conflict check → job registration →
// detached run. The legacy Start keeps its exact signature and fn shape; the
// reporter parameter is the only addition on this path.
func (r *analysisRunner) launch(scope string, id uint, kind string, timeout time.Duration, fn func(ctx context.Context, report func(SignalJobPatch)) (uint, error)) (AnalysisStatus, error) {
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
	// Reporter bound to THIS job object: patches lock internally and are
	// rejected once the job reaches a terminal state.
	report := func(patch SignalJobPatch) { r.patchJob(job, patch) }
	r.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		ctx = context.WithValue(ctx, analysisJobIDContextKey{}, job.jobID)
		resultID, err := safeRun(ctx, fn, report)
		r.mu.Lock()
		defer r.mu.Unlock()
		if cur := r.jobs[k]; cur == job && !cur.done {
			cur.done = true
			cur.err = errString(err)
			// fn 返回值优先；signal job 经 patch 预上报的 ResultID 只在 fn
			// 返 0（StartSignal 路径恒为 0）时保留。旧路径行为不变。
			if resultID != 0 {
				cur.resultID = resultID
			}
			// 信号任务兑底（JB-1）：超时/panic/未报告终态的错误必须以
			// outcome=failed 收尾，不能停留在无 outcome 的中间态；旧 kind
			// 永不写 outcome，序列化不受影响。
			if err != nil && cur.outcome == "" && isSignalJobKind(kind) {
				cur.outcome = SignalOutcomeFailed
			}
		}
	}()
	return st, nil
}

// patchJob applies a progress patch under the runner lock. Terminal jobs are
// immutable: late patches after completion/timeout/panic are dropped.
func (r *analysisRunner) patchJob(job *analysisJob, patch SignalJobPatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if job == nil || job.done {
		return
	}
	if patch.Phase != "" {
		job.phase = patch.Phase
	}
	if patch.Outcome != "" {
		job.outcome = patch.Outcome
	}
	if patch.ErrorStage != "" {
		job.errorStage = patch.ErrorStage
	}
	if patch.Granularity != "" {
		job.granularity = patch.Granularity
	}
	if patch.Period != "" {
		job.period = patch.Period
	}
	if patch.DiscoveryID != 0 {
		job.discoveryID = patch.DiscoveryID
	}
	if patch.CandidateID != 0 {
		job.candidateID = patch.CandidateID
	}
	if patch.CandidateCount != 0 {
		job.candidateCount = patch.CandidateCount
	}
	if patch.ResultID != 0 {
		job.resultID = patch.ResultID
	}
}

// RunningSignalReportForCandidate reports the LIVE running research job
// (kind board_signal_report) targeting candidateID on a board — the derived
// 「研究中」 half of the candidate status（design §3：running 位不持久化，
// 重启不可能卡死；board 槽位同板最多一个 running job，O(1) 查询）.
func (r *analysisRunner) RunningSignalReportForCandidate(boardID, candidateID uint) (AnalysisStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobKey(AnalysisScopeBoard, boardID)]
	if !ok || job.done || job.kind != AnalysisJobKindBoardSignalReport || job.candidateID != candidateID {
		return AnalysisStatus{}, false
	}
	return job.snapshot(), true
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

// analysisJobIDContextKey carries the job's identity into the job fn via
// ctx（board-signal-reports tasks 4.7）：StartSignal 的 fn 闭包拿不到 mint 后的
// job_id（goroutine 先于返回值启动，捕获会有竞态），改由 launch 在启动前注入
// context —— fn 用 jobIDFromContext 读取，进度持久化以它为滚动行主键。
type analysisJobIDContextKey struct{}

// jobIDFromContext returns the running job's id, or "" outside a launched job
// (e.g. direct service calls in tests without the runner).
func jobIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(analysisJobIDContextKey{}).(string)
	return id
}

// safeRun guards fn against panics so a crashing analysis surfaces as an error
// status instead of taking down the process.
func safeRun(ctx context.Context, fn func(ctx context.Context, report func(SignalJobPatch)) (uint, error), report func(SignalJobPatch)) (resultID uint, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			resultID, err = 0, fmt.Errorf("analysis panic: %v", rec)
		}
	}()
	return fn(ctx, report)
}
