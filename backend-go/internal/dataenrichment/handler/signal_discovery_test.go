package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	dataenrichment "syntopica-backend/internal/dataenrichment"
	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
	topicgraphrepo "syntopica-backend/internal/topicgraph/repository"
)

// ── 信号发现 API（board-signal-reports tasks 2.4：API-1/2、S1、JB-1/4 discovery 部分）──
//
// handler 层轻量测试（内存 SQLite + stub runner/stub LLM，不起真模型）：
//   - API 契约：400 无 job / 409 互斥 / 202 job 帧
//   - job 状态机：phase/outcome/error_stage/candidate_count，无 result_id
//   - 列表：分页游标、派生状态（researching|reported|pending）、空新批不清旧
//   - 真实 service 的端到端发现路径（材料装配 → detect → 原子保存）

// ── test doubles ────────────────────────────────────────────────────────────

// stubSignalDiscovery records calls and replays one canned outcome/error.
type stubSignalDiscovery struct {
	outcome      *service.SignalDiscoveryOutcome
	err          error
	errStage     string // non-empty → err is wrapped in *service.SignalStageError
	block        chan struct{}
	calls        int
	lastBoard    uint
	lastGran     string
	lastPeriod   string
	hookedStages []string
}

func (s *stubSignalDiscovery) DiscoverSignals(ctx context.Context, boardID uint, granularity, period string, hook func(stage string)) (*service.SignalDiscoveryOutcome, error) {
	s.calls++
	s.lastBoard = boardID
	s.lastGran, s.lastPeriod = granularity, period
	if s.block != nil {
		<-s.block
	}
	s.record(hook, "prepare")
	if s.err != nil {
		if s.errStage != "" {
			s.record(hook, s.errStage)
			return nil, &service.SignalStageError{Stage: s.errStage, Err: s.err}
		}
		return nil, s.err
	}
	s.record(hook, "detect")
	return s.outcome, nil
}

func (s *stubSignalDiscovery) record(hook func(stage string), stage string) {
	s.hookedStages = append(s.hookedStages, stage)
	hook(stage)
}

// signalLLMStub is the service-level AirRouter double for the end-to-end path.
type signalLLMStub struct {
	content string
	err     error
	calls   int
}

func (s *signalLLMStub) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &airouter.ChatResult{Content: s.content}, nil
}

func newSignalRouter(t *testing.T, orch handler.Orchestrator, db *gorm.DB, runner handler.SignalDiscoveryRunner) (*gin.Engine, *handler.EnrichmentHandler) {
	t.Helper()
	repo := repository.NewRepository(db)
	gin.SetMode(gin.TestMode)
	h := handler.NewHandler(repo, nil, orch, &enabledBoardConfigReader{}, nil, nil, db)
	if runner != nil {
		h.SetSignalDiscovery(runner)
	}
	r := gin.New()
	h.RegisterRoutes(&r.RouterGroup)
	return r, h
}

func postJSON(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", path, bodyReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func getJSON(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", path, nil)
	r.ServeHTTP(w, req)
	return w
}

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, body)
	}
	return resp.Data
}

func seedSignalCandidate(t *testing.T, db *gorm.DB, boardID uint, period, signal string) *repository.BoardSignalCandidate {
	t.Helper()
	repo := repository.NewRepository(db)
	batch := &repository.BoardSignalDiscovery{
		SemanticBoardID: boardID,
		Granularity:     repository.SignalGranularityMonth,
		Period:          period,
		AnalysisMode:    repository.SignalAnalysisModeCurrent,
		Cutoff:          time.Now().Add(-time.Hour),
		SessionID:       "board_signal_discovery_seed",
	}
	candidate := &repository.BoardSignalCandidate{
		Signal:           signal,
		WhyItMatters:     "w:" + signal,
		ResearchQuestion: "q:" + signal,
		EvidenceRefs:     json.RawMessage(`["1","2"]`),
		Score:            8,
		Rationale:        "r:" + signal,
	}
	saved, _, err := repo.CreateSignalDiscoveryBatch(context.Background(), batch, []*repository.BoardSignalCandidate{candidate})
	if err != nil || len(saved) != 1 {
		t.Fatalf("seed candidate: err=%v saved=%d", err, len(saved))
	}
	return saved[0]
}

func seedSignalReportResult(t *testing.T, db *gorm.DB, boardID, candidateID uint, period string) *repository.TopicEnrichmentResult {
	t.Helper()
	repo := repository.NewRepository(db)
	granularity := repository.SignalGranularityMonth
	result := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindSignalReport,
		Sectors:         json.RawMessage(`{"schema_version":2}`),
		Granularity:     &granularity,
		Period:          &period,
		SourceSignalID:  &candidateID,
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), result); err != nil {
		t.Fatalf("seed signal report result: %v", err)
	}
	return result
}

// ── API-1：POST /signal-discoveries ─────────────────────────────────────────

func TestSignalDiscoveryTrigger_LegalPeriodAccepted(t *testing.T) {
	db := setupHandlerTestDB(t)
	stub := &stubSignalDiscovery{outcome: &service.SignalDiscoveryOutcome{DiscoveryID: 42, CandidateCount: 2, SessionID: "board_signal_discovery_5_x"}}
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("legal discovery: want 202, got %d body=%s", w.Code, w.Body.String())
	}
	started := decodeEnvelope(t, w.Body.Bytes())
	jobID, _ := started["job_id"].(string)
	if started["status"] != "started" || jobID == "" || started["job_kind"] != "board_signal_discovery" ||
		started["scope"] != "board" || started["target_id"] != float64(5) {
		t.Fatalf("202 envelope: %v", started)
	}

	st := pollJobStatus(t, r, jobID)
	if st["outcome"] != "discovered" || st["phase"] != "detect" {
		t.Fatalf("job status outcome/phase: %v", st)
	}
	if stub.lastBoard != 5 || stub.lastGran != "month" || stub.lastPeriod != "2026-08" {
		t.Fatalf("runner must receive the parsed request: %d %s %s", stub.lastBoard, stub.lastGran, stub.lastPeriod)
	}
	if st["discovery_id"] != float64(42) || st["candidate_count"] != float64(2) {
		t.Fatalf("job counters: %v", st)
	}
	if _, has := st["result_id"]; has {
		t.Fatalf("discovery jobs never carry result_id: %v", st)
	}
	if _, has := st["error"]; has {
		t.Fatalf("successful job must not carry error: %v", st)
	}
}

func TestSignalDiscoveryTrigger_InvalidPeriodRejectedWithoutJob(t *testing.T) {
	db := setupHandlerTestDB(t)
	stub := &stubSignalDiscovery{}
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, stub)

	cases := []struct{ name, body string }{
		{"month 13", `{"granularity":"month","period":"2026-13"}`},
		{"week granularity", `{"granularity":"week","period":"2026-W27"}`},
		{"empty period", `{"granularity":"month","period":""}`},
		{"missing body", `{"granularity":"month"}`},
		{"future month", `{"granularity":"month","period":"2099-01"}`},
		{"future year", `{"granularity":"year","period":"2099"}`},
		{"bad shape", `{"granularity":"month","period":"2026/08"}`},
	}
	for _, tc := range cases {
		w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", tc.body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d body=%s", tc.name, w.Code, w.Body.String())
		}
	}
	if stub.calls != 0 {
		t.Fatalf("rejected requests must never reach the service, got %d calls", stub.calls)
	}
	// 无 job：board 状态查询回到 idle 兜底帧。
	w := getJSON(t, r, "/enrichment/analysis-status?scope=board&id=5")
	idle := decodeEnvelope(t, w.Body.Bytes())
	if idle["running"] != false || idle["finished"] != false {
		t.Fatalf("no job may be created by rejected triggers: %v", idle)
	}
}

func TestSignalDiscoveryTrigger_DisabledBoardRejected(t *testing.T) {
	db := setupHandlerTestDB(t)
	stub := &stubSignalDiscovery{}
	orch := &mockOrchestrator{boardErr: fmt.Errorf("enrich board 5: enrichment not enabled for this board")}
	r, _ := newSignalRouter(t, orch, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w.Code != http.StatusBadRequest || !jsonContains(w.Body.String(), "not enabled") {
		t.Fatalf("disabled board: want 400 not-enabled, got %d body=%s", w.Code, w.Body.String())
	}
	if stub.calls != 0 {
		t.Fatalf("disabled board must not start a job, got %d service calls", stub.calls)
	}
}

func TestSignalDiscoveryTrigger_RunningDiscoveryConflicts(t *testing.T) {
	db := setupHandlerTestDB(t)
	block := make(chan struct{})
	stub := &stubSignalDiscovery{outcome: &service.SignalDiscoveryOutcome{DiscoveryID: 1, CandidateCount: 1}, block: block}
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("first trigger: want 202, got %d", w.Code)
	}
	w2 := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate trigger: want 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	conflict := decodeEnvelope(t, w2.Body.Bytes())
	if conflict["job_kind"] != "board_signal_discovery" {
		t.Fatalf("409 must carry the running job identity: %v", conflict)
	}
	close(block)
}

// JB-4（discovery 部分）：旧 brief 在跑 → 新发现同样 409，等待人工时不持锁。
func TestSignalDiscoveryTrigger_RunningBriefConflicts(t *testing.T) {
	db := setupHandlerTestDB(t)
	res := boardResultRow(t, db, 5, "命题甲")
	orch := &mockOrchestrator{boardOut: &service.BoardEnrichmentOutput{Result: res}, block: make(chan struct{})}
	stub := &stubSignalDiscovery{}
	r, _ := newSignalRouter(t, orch, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/trigger", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("brief trigger: want 202, got %d", w.Code)
	}
	w2 := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w2.Code != http.StatusConflict {
		t.Fatalf("discovery during brief: want 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	conflict := decodeEnvelope(t, w2.Body.Bytes())
	if conflict["job_kind"] != "board_brief" {
		t.Fatalf("409 must identify the running brief, not the discovery: %v", conflict)
	}
	close(orch.block)
}

// JB-1：失败 → outcome=failed + error_stage，不伪装零信号。
func TestSignalDiscoveryJob_FailedOutcomeCarriesStage(t *testing.T) {
	db := setupHandlerTestDB(t)
	stub := &stubSignalDiscovery{err: fmt.Errorf("detect LLM unavailable"), errStage: "detect"}
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger: want 202, got %d", w.Code)
	}
	jobID, _ := decodeEnvelope(t, w.Body.Bytes())["job_id"].(string)
	st := pollJobStatus(t, r, jobID)
	if st["outcome"] != "failed" || st["error_stage"] != "detect" {
		t.Fatalf("failed job status: %v", st)
	}
	if msg, _ := st["error"].(string); !strings.Contains(msg, "detect LLM unavailable") {
		t.Fatalf("job error must surface the cause: %v", st)
	}
	if _, has := st["result_id"]; has {
		t.Fatalf("failed discovery carries no result_id: %v", st)
	}
}

// JB-1 兜底：未显式报告 outcome 的失败（超时/panic 路径）由 runner 兜底 failed。
func TestSignalDiscoveryJob_FailedOutcomeBackstopWithoutStage(t *testing.T) {
	db := setupHandlerTestDB(t)
	stub := &stubSignalDiscovery{err: fmt.Errorf("boom")}
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	jobID, _ := decodeEnvelope(t, w.Body.Bytes())["job_id"].(string)
	st := pollJobStatus(t, r, jobID)
	if st["outcome"] != "failed" {
		t.Fatalf("runner backstop must set outcome=failed: %v", st)
	}
	if _, has := st["error_stage"]; has {
		t.Fatalf("no stage reported → no error_stage: %v", st)
	}
}

// ── S1：零信号不清旧记录；真实 service 端到端发现路径 ────────────────────────

func TestSignalDiscovery_NoSignalKeepsOldCandidates(t *testing.T) {
	db := setupHandlerTestDB(t)
	old := seedSignalCandidate(t, db, 5, "2026-08", "旧信号保留")
	stub := &stubSignalDiscovery{outcome: &service.SignalDiscoveryOutcome{DiscoveryID: 77, CandidateCount: 0}}
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, stub)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2026-08"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger: want 202, got %d", w.Code)
	}
	jobID, _ := decodeEnvelope(t, w.Body.Bytes())["job_id"].(string)
	st := pollJobStatus(t, r, jobID)
	if st["outcome"] != "no_signal" {
		t.Fatalf("zero candidates must end as no_signal: %v", st)
	}
	if _, has := st["candidate_count"]; has {
		t.Fatalf("no_signal omits candidate_count (0): %v", st)
	}
	if _, has := st["result_id"]; has {
		t.Fatalf("no_signal has no result_id: %v", st)
	}

	// 旧候选仍在，状态待研究（空新批不清旧记录——列表只查候选表）。
	list := getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08")
	if list.Code != http.StatusOK {
		t.Fatalf("list: %d body=%s", list.Code, list.Body.String())
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0]["id"] != float64(old.ID) || resp.Data[0]["status"] != "pending" {
		t.Fatalf("old candidates must survive a no_signal batch: %v", resp.Data)
	}
}

// 真实 SignalDiscoveryService（材料装配 → detect → 原子保存）+ stub LLM。
// S1 主链路步 3：detect 达标 → 事务保存批次/候选；无取数/计算/成文。
func TestSignalDiscovery_EndToEndDiscoveredPath(t *testing.T) {
	db := setupHandlerTestDB(t)

	// 板块 5：一条 2020-01 窗口内的泳道切片（泳道创建早于 cutoff）。
	lane := &topicgraphrepo.BoardPersistentTopic{
		SemanticBoardID: 5,
		Label:           "原油",
		Status:          "active",
		Source:          "auto",
		FirstSeenDate:   time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSeenDate:    time.Date(2020, 1, 20, 0, 0, 0, 0, time.UTC),
		HitCount:        1,
		CreatedAt:       time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := db.Create(lane).Error; err != nil {
		t.Fatalf("seed lane: %v", err)
	}
	report := &topicgraphrepo.BoardDailyReport{
		SemanticBoardID: 5,
		PeriodDate:      time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC),
		Status:          "completed",
	}
	if err := db.Create(report).Error; err != nil {
		t.Fatalf("seed report: %v", err)
	}
	topicID := lane.ID
	section := &topicgraphrepo.DailyReportSection{
		ReportID:             report.ID,
		ClusterLabel:         "炼厂开工异动",
		ArticleCount:         5,
		PersistentTopicID:    &topicID,
		TopicMatchConfidence: "high",
	}
	if err := db.Create(section).Error; err != nil {
		t.Fatalf("seed section: %v", err)
	}

	llm := &signalLLMStub{content: fmt.Sprintf(
		`{"signals":[{"signal":"炼厂开工与库存背离","why_it_matters":"改变补库判断","research_question":"开工下滑为何库存未降","evidence_refs":["%d"],"score":8,"rationale":"异动强度高"}]}`,
		section.ID)}
	svc := service.NewSignalDiscoveryService(
		llm,
		"data_enrichment_analysis",
		service.NewSignalMaterialBuilder(db, dataenrichment.NewDBLifelineReader(db)),
		repository.NewRepository(db),
	)
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, svc)

	w := postJSON(t, r, "/semantic-boards/5/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2020-01"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger: want 202, got %d body=%s", w.Code, w.Body.String())
	}
	jobID, _ := decodeEnvelope(t, w.Body.Bytes())["job_id"].(string)
	st := pollJobStatus(t, r, jobID)
	if st["outcome"] != "discovered" || st["candidate_count"] != float64(1) {
		t.Fatalf("discovered job status: %v", st)
	}
	if llm.calls != 1 {
		t.Fatalf("detect is exactly one LLM call（取数=0/计算=0/compose=0）, got %d", llm.calls)
	}
	discoveryID, _ := st["discovery_id"].(float64)
	if discoveryID == 0 {
		t.Fatalf("discovery_id must be set: %v", st)
	}

	// 列表：候选可见，证据引用指向材料切片，状态待研究，批次时间可追溯。
	list := getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2020-01")
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("want 1 candidate, got %v", resp.Data)
	}
	row := resp.Data[0]
	if row["signal"] != "炼厂开工与库存背离" || row["score"] != float64(8) || row["status"] != "pending" {
		t.Fatalf("candidate row: %v", row)
	}
	refs, ok := row["evidence_refs"].([]any)
	if !ok || len(refs) != 1 || refs[0] != fmt.Sprintf("%d", section.ID) {
		t.Fatalf("evidence refs must reference the material slice: %v", row["evidence_refs"])
	}
	if row["discovery_id"] != discoveryID {
		t.Fatalf("candidate must link its batch: %v vs %v", row["discovery_id"], discoveryID)
	}
	if _, has := row["discovery_created_at"]; !has {
		t.Fatalf("discovery_created_at (发现时间来自批次) must be present: %v", row)
	}
	if row["latest_result_id"] != nil {
		t.Fatalf("pending candidate has no result: %v", row["latest_result_id"])
	}

	// 对照：无材料板块跳过 LLM 直接零候选（PC-4），job no_signal。
	w2 := postJSON(t, r, "/semantic-boards/6/enrichment/analysis/signal-discoveries", `{"granularity":"month","period":"2020-01"}`)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("empty-board trigger: want 202, got %d", w2.Code)
	}
	job2, _ := decodeEnvelope(t, w2.Body.Bytes())["job_id"].(string)
	st2 := pollJobStatus(t, r, job2)
	if st2["outcome"] != "no_signal" {
		t.Fatalf("empty material must end no_signal: %v", st2)
	}
	if llm.calls != 1 {
		t.Fatalf("empty material must skip the LLM, total calls now %d", llm.calls)
	}
}

// ── API-2：GET /signals 分页与派生状态 ───────────────────────────────────────

func TestSignalList_PaginationAndValidation(t *testing.T) {
	db := setupHandlerTestDB(t)
	c1 := seedSignalCandidate(t, db, 5, "2026-08", "信号一")
	c2 := seedSignalCandidate(t, db, 5, "2026-08", "信号二")
	c3 := seedSignalCandidate(t, db, 5, "2026-08", "信号三")
	seedSignalCandidate(t, db, 5, "2026-07", "别的周期") // 不混入 2026-08
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, &stubSignalDiscovery{})

	w := getJSON(t, r, "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08")
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("want 3 rows, got %d", len(resp.Data))
	}
	// id 倒序：最新批次/候选在前。
	want := []uint{c3.ID, c2.ID, c1.ID}
	for i, id := range want {
		if resp.Data[i]["id"] != float64(id) {
			t.Fatalf("row %d: want id %d, got %v", i, id, resp.Data[i]["id"])
		}
	}

	// 游标 + limit。
	w = getJSON(t, r, fmt.Sprintf("/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08&before_id=%d&limit=2", c3.ID))
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode page2: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0]["id"] != float64(c2.ID) || resp.Data[1]["id"] != float64(c1.ID) {
		t.Fatalf("cursor page: %v", resp.Data)
	}

	// 非法参数 400。
	for _, path := range []string{
		"/semantic-boards/5/enrichment/analysis/signals?granularity=week&period=2026-W27",
		"/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-13",
		"/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08&limit=abc",
		"/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08&limit=0",
		"/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08&before_id=x",
	} {
		if w := getJSON(t, r, path); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: want 400, got %d", path, w.Code)
		}
	}
}

func TestSignalList_DerivedStatus(t *testing.T) {
	db := setupHandlerTestDB(t)
	pending := seedSignalCandidate(t, db, 5, "2026-08", "待研究")
	reported := seedSignalCandidate(t, db, 5, "2026-08", "已有报告")
	researching := seedSignalCandidate(t, db, 5, "2026-08", "研究中")
	seedSignalReportResult(t, db, 5, reported.ID, "2026-08")
	r, _ := newSignalRouter(t, &mockOrchestrator{}, db, &stubSignalDiscovery{})

	statusOf := func(rows []map[string]any, id uint) map[string]any {
		for _, row := range rows {
			if row["id"] == float64(id) {
				return row
			}
		}
		t.Fatalf("candidate %d missing from list", id)
		return nil
	}

	// 研究中：live running report job（不持久化 running 位）。挂在同一 handler
	// 实例上——派生状态读的就是它的 live job 表。
	block := make(chan struct{})
	r, h := newSignalRouter(t, &mockOrchestrator{}, db, &stubSignalDiscovery{})
	h.StartHangingSignalReportJob(5, researching.ID, block)

	listPath := "/semantic-boards/5/enrichment/analysis/signals?granularity=month&period=2026-08"
	w := getJSON(t, r, listPath)
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if row := statusOf(resp.Data, researching.ID); row["status"] != "researching" {
		t.Fatalf("running research job → researching: %v", row)
	}
	reportedRow := statusOf(resp.Data, reported.ID)
	if reportedRow["status"] != "reported" {
		t.Fatalf("successful report → reported: %v", reportedRow)
	}
	if id, _ := reportedRow["latest_result_id"].(float64); reportedRow["latest_result_id"] == nil || id == 0 {
		t.Fatalf("reported candidate carries its latest result id: %v", reportedRow)
	}
	if row := statusOf(resp.Data, pending.ID); row["status"] != "pending" || row["latest_result_id"] != nil {
		t.Fatalf("no report, no job → pending with null result id: %v", row)
	}

	// job 结束（含失败路径：无成功报告）→ 研究中回落待研究，不永久卡死（JB-3）。
	close(block)
	deadline := time.Now().Add(2 * time.Second)
	for {
		w := getJSON(t, r, listPath)
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if statusOf(resp.Data, researching.ID)["status"] != "researching" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("candidate stuck on researching after job finished")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
