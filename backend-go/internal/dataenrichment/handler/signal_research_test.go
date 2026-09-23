package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 深入研究 API（board-signal-reports tasks 4.3：API-3..6、S2、JB-1..4
// research 侧、NR）。handler 层轻量测试（内存 SQLite + stub runner）：HTTP 契约
// 与 job 状态机；service 内部行为由 service 层测试覆盖。──────────────────────

// stubSignalResearch records calls and replays one canned outcome/error.
type stubSignalResearch struct {
	resultID    uint
	err         error
	errStage    string // non-empty → err wrapped in *service.SignalStageError
	block       chan struct{}
	entered     chan struct{} // optional: closed once on first call, for mid-flight assertions (avoids goroutine-schedule race)
	enteredOnce sync.Once
	calls       int
	lastBoard   uint
	lastCand    uint
	lastJobID   string
}

func (s *stubSignalResearch) ResearchCandidate(ctx context.Context, boardID, candidateID uint, jobID string, hook func(stage string)) (uint, error) {
	s.lastJobID = jobID
	s.calls++
	s.lastBoard = boardID
	s.lastCand = candidateID
	// entered 在计数/身份记录之后才关阉：channel close 的 happens-before
	// 保证接收方醒来时必能读到本次写入。
	if s.entered != nil {
		s.enteredOnce.Do(func() { close(s.entered) })
	}
	if s.block != nil {
		<-s.block
	}
	if hook != nil {
		hook(service.SignalStageResearch)
		hook(service.SignalStageCompose)
	}
	if s.err != nil {
		if s.errStage != "" {
			return 0, &service.SignalStageError{Stage: s.errStage, Err: s.err}
		}
		return 0, s.err
	}
	return s.resultID, nil
}

func newResearchRouter(t *testing.T, orch handler.Orchestrator, db *gorm.DB, research handler.SignalResearchRunner) (*gin.Engine, *handler.EnrichmentHandler) {
	t.Helper()
	repo := repository.NewRepository(db)
	gin.SetMode(gin.TestMode)
	h := handler.NewHandler(repo, nil, orch, &enabledBoardConfigReader{}, nil, nil, db)
	if research != nil {
		h.SetSignalResearch(research)
	}
	r := gin.New()
	h.RegisterRoutes(&r.RouterGroup)
	return r, h
}

// decodeArrayEnvelope 解码 data 为数组的响应。
func decodeArrayEnvelope(t *testing.T, body []byte) []any {
	t.Helper()
	var resp struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode array envelope: %v body=%s", err, body)
	}
	return resp.Data
}

// ── API-3：跨板块 candidate/result 一律 404 ─────────────────────────────────

func TestSignalResearch_CrossBoardCandidateAndReport404(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 6, "2026-08", "别的板块的信号") // board 6
	stub := &stubSignalResearch{resultID: 11}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	// POST research 到板块 5：候选属板块 6 → 404，不产生 job。
	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-board candidate: want 404, got %d body=%s", w.Code, w.Body.String())
	}
	if stub.calls != 0 {
		t.Fatalf("rejected request must not reach the service")
	}
	// 不存在的候选同样 404。
	w = postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals/999/research", `{}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown candidate: want 404, got %d", w.Code)
	}
	// 报告详情：别的板块的报告 → 404。
	report := seedSignalReportResult(t, db, 6, candidate.ID, "2026-08")
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signal-reports/%d", report.ID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-board report: want 404, got %d", w.Code)
	}
	// kind 不匹配（board_brief）→ 404。
	brief := boardResultRow(t, db, 5, "命题甲")
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signal-reports/%d", brief.ID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("kind mismatch: want 404, got %d", w.Code)
	}
}

// ── API-4：浏览器自带材料/cutoff 字段被忽略（服务端快照权威）────────────────

func TestSignalResearch_ClientSuppliedFieldsIgnored(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	stub := &stubSignalResearch{resultID: 11}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	body := fmt.Sprintf(`{"regenerate":true,"cutoff":"2099-01-01T00:00:00Z","granularity":"year","period":"2099","signal":"伪造标题","candidate_id":%d}`, candidate.ID+1)
	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), body)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d body=%s", w.Code, w.Body.String())
	}
	pollJobStatus(t, r, decodeEnvelope(t, w.Body.Bytes())["job_id"].(string))
	if stub.calls != 1 || stub.lastCand != candidate.ID || stub.lastBoard != 5 {
		t.Fatalf("service must receive only the server-side candidate identity: calls=%d board=%d cand=%d", stub.calls, stub.lastBoard, stub.lastCand)
	}
}

// ── API-5：重复点击 → 单 job + 409；成功后默认重试 → 200 复用，零新 LLM ──────

func TestSignalResearch_ConcurrentDuplicateAndIdempotentReuse(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	block := make(chan struct{})
	stub := &stubSignalResearch{resultID: 11, block: block, entered: make(chan struct{})}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	// 第一个点击：202。
	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("first click: want 202, got %d body=%s", w.Code, w.Body.String())
	}
	first := decodeEnvelope(t, w.Body.Bytes())
	// 同候选重复点击（running）→ 409 携 running 身份。
	w = postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate click: want 409, got %d", w.Code)
	}
	<-stub.entered // 第一次 job 已真正进入 service（消除调度竞态，此刻 calls 必为 1）
	if stub.calls != 1 {
		t.Fatalf("duplicate must not start a second run, calls=%d", stub.calls)
	}
	conflict := decodeEnvelope(t, w.Body.Bytes())
	if conflict["job_kind"] != "board_signal_report" {
		t.Fatalf("409 payload must identify the running research: %v", conflict)
	}
	// 释放第一个 job。
	close(block)
	st := pollJobStatus(t, r, first["job_id"].(string))
	if st["outcome"] != "succeeded" || st["result_id"] != float64(11) {
		t.Fatalf("job status: %v", st)
	}
	// stub 不落库：派生「已有报告」以真实 result 行为准，补种子行。
	reported := seedSignalReportResult(t, db, 5, candidate.ID, "2026-08")
	// 成功后再点（regenerate 缺省 false）→ 200 已有 result_id，零新 LLM。
	w = postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusOK {
		t.Fatalf("reuse: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	reuse := decodeEnvelope(t, w.Body.Bytes())
	if reuse["status"] != "already_reported" || reuse["result_id"] != float64(reported.ID) {
		t.Fatalf("reuse payload: %v", reuse)
	}
	if stub.calls != 1 {
		t.Fatalf("reuse must not call the service again, calls=%d", stub.calls)
	}
}

// ── API-6：显式 regenerate=true 且无 running → 202 新任务（共享 board 锁）────

func TestSignalResearch_RegenerateStartsNewJob(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	seedSignalReportResult(t, db, 5, candidate.ID, "2026-08")
	stub := &stubSignalResearch{resultID: 12}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{"regenerate":true}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("regenerate: want 202, got %d body=%s", w.Code, w.Body.String())
	}
	started := decodeEnvelope(t, w.Body.Bytes())
	if started["job_kind"] != "board_signal_report" || started["candidate_id"] != float64(candidate.ID) {
		t.Fatalf("202 envelope: %v", started)
	}
	st := pollJobStatus(t, r, started["job_id"].(string))
	if st["outcome"] != "succeeded" || st["result_id"] != float64(12) {
		t.Fatalf("regenerate job: %v", st)
	}
	// 非法 body（regenerate 非布尔）→ 400 无新 job。
	w = postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{"regenerate":"yes"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad regenerate body: want 400, got %d", w.Code)
	}
}

// ── S2：只研究我选的一条 ────────────────────────────────────────────────────

func TestSignalResearch_OnlySelectedCandidateRuns(t *testing.T) {
	db := setupHandlerTestDB(t)
	candA := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	candB := seedSignalCandidate(t, db, 5, "2026-08", "信号B")
	stub := &stubSignalResearch{resultID: 21}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candA.ID), `{}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", w.Code)
	}
	pollJobStatus(t, r, decodeEnvelope(t, w.Body.Bytes())["job_id"].(string))
	if stub.calls != 1 || stub.lastCand != candA.ID {
		t.Fatalf("only the clicked candidate must be researched: calls=%d last=%d (B=%d)", stub.calls, stub.lastCand, candB.ID)
	}
	// stub 不落库；派生状态查询以真实 result 行为准，补种子行。
	reported := seedSignalReportResult(t, db, 5, candA.ID, "2026-08")
	// 列表派生状态：A=reported，B 仍 pending。
	w = getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08")
	rows := decodeArrayEnvelope(t, w.Body.Bytes())
	statusBySignal := map[string]map[string]any{}
	for _, row := range rows {
		rowMap := row.(map[string]any)
		statusBySignal[rowMap["signal"].(string)] = rowMap
	}
	if statusBySignal["信号A"]["status"] != "reported" || statusBySignal["信号A"]["latest_result_id"] != float64(reported.ID) {
		t.Fatalf("A must be reported: %v", statusBySignal["信号A"])
	}
	if statusBySignal["信号B"]["status"] != "pending" {
		t.Fatalf("B must stay pending: %v", statusBySignal["信号B"])
	}
}

// ── JB-2：失败 job → outcome=failed + error_stage，无 result_id；候选可重试 ──

func TestSignalResearch_FailedJobCarriesErrorStageAndNoResultID(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	stub := &stubSignalResearch{err: fmt.Errorf("compose keeps failing"), errStage: "compose"}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", w.Code)
	}
	jobID := decodeEnvelope(t, w.Body.Bytes())["job_id"].(string)
	st := pollJobStatus(t, r, jobID)
	if st["outcome"] != "failed" || st["error_stage"] != "compose" {
		t.Fatalf("failed job status: %v", st)
	}
	if _, has := st["result_id"]; has {
		t.Fatalf("failed job must not carry result_id: %v", st)
	}
	// 候选保留可重试：列表回到 pending（running 位不持久化，重启不卡死）。
	w = getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08")
	rows := decodeArrayEnvelope(t, w.Body.Bytes())
	row := rows[0].(map[string]any)
	if row["status"] != "pending" {
		t.Fatalf("failed candidate must be retryable (pending): %v", row)
	}
	// 重试（新点击）→ 新 job。
	w = postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("retry: want 202, got %d", w.Code)
	}
	pollJobStatus(t, r, decodeEnvelope(t, w.Body.Bytes())["job_id"].(string))
	if stub.calls != 2 {
		t.Fatalf("retry must call the service again: %d", stub.calls)
	}
}

// ── JB-4：旧 brief 在跑 → research 同样 409（共享 board 互斥）────────────────

func TestSignalResearch_OldBriefRunningBlocksResearch(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	orch := &mockOrchestrator{
		boardOut: &service.BoardEnrichmentOutput{Result: boardResultRow(t, db, 5, "命题甲")},
		block:    make(chan struct{}),
	}
	r, _ := newResearchRouter(t, orch, db, &stubSignalResearch{resultID: 1})

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/trigger", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("brief trigger: want 202, got %d", w.Code)
	}
	w2 := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w2.Code != http.StatusConflict {
		t.Fatalf("brief running: research must 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	conflict := decodeEnvelope(t, w2.Body.Bytes())
	if conflict["job_kind"] != "board_brief" {
		t.Fatalf("409 must identify the running brief: %v", conflict)
	}
	close(orch.block)
}

// ── 报告列表/详情：仅成功报告、游标分页、不可变 payload、无 review 字段 ──────

func TestSignalReports_ListAndDetail(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	r1 := seedSignalReportResult(t, db, 5, candidate.ID, "2026-08")
	r2 := seedSignalReportResult(t, db, 5, candidate.ID, "2026-08")
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, nil)

	// 列表：id 倒序。
	w := getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-reports?granularity=month&period=2026-08")
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", w.Code, w.Body.String())
	}
	rows := decodeArrayEnvelope(t, w.Body.Bytes())
	if len(rows) != 2 {
		t.Fatalf("list rows=%d", len(rows))
	}
	first := rows[0].(map[string]any)
	if first["id"] != float64(r2.ID) || first["source_signal_id"] != float64(candidate.ID) {
		t.Fatalf("list order/fields: %v", first)
	}
	// before_id 游标（排他）。
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signal-reports?granularity=month&period=2026-08&before_id=%d", r2.ID))
	rows = decodeArrayEnvelope(t, w.Body.Bytes())
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != float64(r1.ID) {
		t.Fatalf("cursor page: %v", rows)
	}
	// source_signal_id 过滤（归属合法）。
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signal-reports?granularity=month&period=2026-08&source_signal_id=%d", candidate.ID))
	rows = decodeArrayEnvelope(t, w.Body.Bytes())
	if len(rows) != 2 {
		t.Fatalf("candidate versions: %v", rows)
	}
	// source_signal_id 不存在/跨板块 → 404。
	w = getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-reports?granularity=month&period=2026-08&source_signal_id=999")
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign source_signal_id: want 404, got %d", w.Code)
	}
	// 非法参数 400。
	for _, q := range []string{
		"granularity=week&period=2026-W27",
		"granularity=month&period=2026-13",
		"granularity=month&period=2026-08&limit=0",
		"granularity=month&period=2026-08&before_id=x",
		"granularity=month&period=2026-08&source_signal_id=abc",
	} {
		w = getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-reports?"+q)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("bad params %q: want 400, got %d", q, w.Code)
		}
	}

	// 详情：不可变 payload + 工具日志 + 输入快照，零 review 键。
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signal-reports/%d", r1.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("detail: %d", w.Code)
	}
	detail := decodeEnvelope(t, w.Body.Bytes())
	if detail["result_kind"] != "signal_report" || detail["granularity"] != "month" || detail["period"] != "2026-08" {
		t.Fatalf("detail fields: %v", detail)
	}
	if _, has := detail["tool_calls"]; !has {
		t.Fatalf("detail must carry the tool log (OB-2)")
	}
	lower := strings.ToLower(w.Body.String())
	for _, banned := range []string{"review", "judge", "approved", "digest"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("signal report responses must not carry review vocabulary %q: %s", banned, w.Body.String())
		}
	}
}

// ── NR：旧 review 路由保持注册（兼容不破坏；行为回归由既有用例守护）──────────

func TestSignalResearch_NR_OldReviewRoutesStillRegistered(t *testing.T) {
	db := setupHandlerTestDB(t)
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, nil)
	w := getJSON(t, r, "/persistent-topics/501/enrichment/reviews")
	if w.Code == http.StatusNotFound {
		t.Fatalf("legacy review routes must stay registered")
	}
}

// ── 研究进展持久化 API（board-signal-reports tasks 4.7）──────────────────────

// job_id 从 runner 注入的 ctx 传入 service：202 信封的 job_id 与 stub 收到的
// jobID 一致（进展滚动行的主键来源）。
func TestSignalResearch_PassesJobIDToService(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	stub := &stubSignalResearch{resultID: 11, entered: make(chan struct{})}
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research", candidate.ID), `{}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d body=%s", w.Code, w.Body.String())
	}
	jobID := decodeEnvelope(t, w.Body.Bytes())["job_id"].(string)
	<-stub.entered // fn 已启动（计数/身份记录后才关阉，happens-before 保证可见）
	if stub.lastJobID != jobID {
		t.Fatalf("service must receive the runner job id: got %q want %q", stub.lastJobID, jobID)
	}
}

// 候选列表行携带 last_research_progress 摘要：有进展行 → 计数+状态+失败原因；
// 无进展 → null。批量一次 IN 查询（不逐行 N+1）。
func TestSignalList_CarriesLastResearchProgressSummary(t *testing.T) {
	db := setupHandlerTestDB(t)
	withProgress := seedSignalCandidate(t, db, 5, "2026-08", "有进展的信号")
	withoutProgress := seedSignalCandidate(t, db, 5, "2026-08", "没研究过的信号")
	_ = withoutProgress
	repo := repository.NewRepository(db)
	require.NoError(t, repo.UpsertSignalResearchProgress(context.Background(), &repository.BoardSignalResearchProgress{
		JobID: "job-list-1", SemanticBoardID: 5, CandidateID: withProgress.ID,
		Granularity: repository.SignalGranularityMonth, Period: "2026-08",
		RoundsDone: 8, SourceCalls: 9, CalculationCalls: 2,
		Ledger:     json.RawMessage(`{"calls":[],"calculations":[],"gaps":[]}`),
		Status:     repository.SignalResearchProgressAbandoned,
		StopReason: "timeout",
		Error:      "研究被取消或超时: context deadline exceeded",
	}))

	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, nil)
	w := getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08")
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	rows := decodeArrayEnvelope(t, w.Body.Bytes())
	if len(rows) != 2 {
		t.Fatalf("want 2 candidate rows, got %d", len(rows))
	}
	bySignal := map[string]map[string]any{}
	for _, row := range rows {
		m := row.(map[string]any)
		bySignal[m["signal"].(string)] = m
	}
	p, ok := bySignal["有进展的信号"]["last_research_progress"].(map[string]any)
	if !ok {
		t.Fatalf("progress row must be an object, got %v", bySignal["有进展的信号"]["last_research_progress"])
	}
	if p["rounds_done"].(float64) != 8 || p["source_calls"].(float64) != 9 || p["calculation_calls"].(float64) != 2 {
		t.Fatalf("progress counters: %v", p)
	}
	if p["status"] != "abandoned" || p["stop_reason"] != "timeout" {
		t.Fatalf("progress status/stop_reason: %v", p)
	}
	if _, has := p["updated_at"]; !has {
		t.Fatalf("progress summary must carry updated_at")
	}
	if got := bySignal["没研究过的信号"]["last_research_progress"]; got != nil {
		t.Fatalf("candidate without progress must serialize null, got %v", got)
	}
}

// 进展端点：候选最近进展全量（含 ledger jsonb）；跨板块/不存在 404；候选
// 存在但从未研究 → 200 data=null。
func TestSignalResearchProgressEndpoint(t *testing.T) {
	db := setupHandlerTestDB(t)
	candidate := seedSignalCandidate(t, db, 5, "2026-08", "信号A")
	otherBoard := seedSignalCandidate(t, db, 6, "2026-08", "别的板块")
	neverResearched := seedSignalCandidate(t, db, 5, "2026-08", "从未研究")
	repo := repository.NewRepository(db)
	require.NoError(t, repo.UpsertSignalResearchProgress(context.Background(), &repository.BoardSignalResearchProgress{
		JobID: "job-ep-1", SemanticBoardID: 5, CandidateID: candidate.ID,
		Granularity: repository.SignalGranularityMonth, Period: "2026-08",
		RoundsDone: 12, SourceCalls: 13, CalculationCalls: 1,
		Ledger:     json.RawMessage(`{"calls":[{"call_id":"c1","question":"q"}],"calculations":[],"gaps":[{"reason":"gap"}]}`),
		Status:     repository.SignalResearchProgressAbandoned,
		StopReason: "research",
		Error:      "signal research: 第13轮 LLM 调用失败",
	}))
	r, _ := newResearchRouter(t, &mockOrchestrator{}, db, nil)

	// 全量行：计数 + 账本三段 + 终态语义。
	w := getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research-progress", candidate.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("progress: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	data := decodeEnvelope(t, w.Body.Bytes())
	if data["job_id"] != "job-ep-1" || data["candidate_id"].(float64) != float64(candidate.ID) {
		t.Fatalf("identity: %v", data)
	}
	if data["rounds_done"].(float64) != 12 || data["source_calls"].(float64) != 13 || data["status"] != "abandoned" || data["stop_reason"] != "research" {
		t.Fatalf("counters/status: %v", data)
	}
	ledger, ok := data["ledger"].(map[string]any)
	if !ok || len(ledger["calls"].([]any)) != 1 || len(ledger["gaps"].([]any)) != 1 {
		t.Fatalf("full ledger must be exposed: %v", data["ledger"])
	}
	// 从未研究：200 + data=null（不是 404——候选存在，只是没有进展）。
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research-progress", neverResearched.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("no progress: want 200, got %d", w.Code)
	}
	if data := decodeEnvelope(t, w.Body.Bytes()); data != nil {
		t.Fatalf("no progress must be null, got %v", data)
	}
	// 跨板块 / 不存在：404（同款不暴露存在性）。
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals/%d/research-progress", otherBoard.ID))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-board: want 404, got %d", w.Code)
	}
	w = getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals/999/research-progress")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown candidate: want 404, got %d", w.Code)
	}
}
