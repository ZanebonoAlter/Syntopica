package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 板块简报/调查 API 契约（board-level-deep-analysis 5.1 / M9）──────────────
//
// 覆盖：D9 的 trigger=board_brief 语义、investigations/trigger（generated/
// custom）、job_id/job_kind 轮询、同 board 跨 kind 409 携当前身份、防断连、
// kind 过滤、父结果校验（404/400 且 0 后台调用）、legacy 详情兼容。

// briefResultRow seeds one board_brief row; researchQuestions is the raw JSON
// array for sectors.research_questions ("" → empty list).
func briefResultRow(t *testing.T, db *gorm.DB, boardID uint, researchQuestions string) *repository.TopicEnrichmentResult {
	t.Helper()
	if researchQuestions == "" {
		researchQuestions = `[]`
	}
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardBrief,
		Sectors: json.RawMessage(`{"scope":"board","result_kind":"board_brief","summary":"供需变化主导",` +
			`"research_questions":` + researchQuestions + `}`),
		SessionID: fmt.Sprintf("data_enrichment_board_%d_brief", boardID),
	}
	if err := repository.NewRepository(db).CreateTopicEnrichmentResult(context.Background(), res); err != nil {
		t.Fatalf("seed board brief: %v", err)
	}
	return res
}

// investigationResultRow seeds one board_investigation child of parentBrief.
func investigationResultRow(t *testing.T, db *gorm.DB, boardID uint, parentBrief *repository.TopicEnrichmentResult, question string) *repository.TopicEnrichmentResult {
	t.Helper()
	key := repository.ComputeQuestionKey(question)
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardInvestigation,
		ParentResultID:  &parentBrief.ID,
		QuestionKey:     &key,
		Sectors: json.RawMessage(`{"scope":"board","result_kind":"board_investigation",` +
			`"question":{"text":"` + question + `","source":"custom"},` +
			`"hypotheses":[],"conclusion":{"summary":"c"},"evidence_chain":[],"lane_refs":[],"method_refs":[]}`),
		SessionID: "data_enrichment_board_inv",
	}
	if err := repository.NewRepository(db).CreateTopicEnrichmentResult(context.Background(), res); err != nil {
		t.Fatalf("seed board investigation: %v", err)
	}
	return res
}

func legacyResultRow(t *testing.T, db *gorm.DB, boardID uint, thesis string) *repository.TopicEnrichmentResult {
	t.Helper()
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindLegacyBoardAnalysis,
		Sectors:         json.RawMessage(`{"scope":"board","thesis":"` + thesis + `"}`),
		SessionID:       "legacy_" + thesis,
	}
	if err := repository.NewRepository(db).CreateTopicEnrichmentResult(context.Background(), res); err != nil {
		t.Fatalf("seed legacy result: %v", err)
	}
	return res
}

// postInvestigation fires the investigation trigger and returns (code, parsed body).
func postInvestigation(t *testing.T, r *gin.Engine, boardID uint, body string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", fmt.Sprintf("/semantic-boards/%d/enrichment/analysis/investigations/trigger", boardID), bodyReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var parsed map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	return w.Code, parsed
}

// triggerEnvelope extracts data from a 202 trigger response.
func triggerEnvelope(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, _ := body["data"].(map[string]any)
	if data == nil {
		t.Fatalf("trigger response missing data: %v", body)
	}
	return data
}

// M9.2 generated 问题：question_id → source=generated，文本从父简报候选解析；
// 202 携 board_investigation 身份；轮询拿 result_id 且 kind 不串。
func TestBoardInvestigationTrigger_Generated(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief := briefResultRow(t, db, 5, `[{"id":"q1","question":"三条泳道是否共享同一驱动？","rationale":"r","related_lane_ids":[1,2]}]`)
	inv := investigationResultRow(t, db, 5, brief, "三条泳道是否共享同一驱动？")
	orch := &mockOrchestrator{investOut: &service.BoardInvestigationOutput{Result: inv}}
	r := newBoardAnalysisRouter(t, orch, db)

	code, body := postInvestigation(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"question_id":"q1"}`, brief.ID))
	if code != http.StatusAccepted {
		t.Fatalf("generated trigger: want 202, got %d body=%v", code, body)
	}
	data := triggerEnvelope(t, body)
	if data["job_kind"] != "board_investigation" || data["job_id"] == "" || data["target_id"] != float64(5) {
		t.Fatalf("generated envelope: %v", data)
	}
	jobID, _ := data["job_id"].(string)

	st := pollJobStatus(t, r, jobID)
	if st["job_kind"] != "board_investigation" {
		t.Fatalf("status job_kind: %v", st)
	}
	if errStr, _ := st["error"].(string); errStr != "" {
		t.Fatalf("investigation failed: %s", errStr)
	}
	if got, _ := st["result_id"].(float64); uint(got) != inv.ID {
		t.Fatalf("result_id = %v, want %d", st["result_id"], inv.ID)
	}

	// 服务层收到的问题：generated + 父简报候选原文（不是 body 里的文本）。
	if orch.lastInvestQ == nil {
		t.Fatal("InvestigateBoardQuestion must run in background")
	}
	if orch.lastInvestBoard != 5 || orch.lastInvestParent != brief.ID {
		t.Fatalf("investigation target: board=%d parent=%d", orch.lastInvestBoard, orch.lastInvestParent)
	}
	if orch.lastInvestQ.Source != service.QuestionSourceGenerated || orch.lastInvestQ.ID != "q1" {
		t.Fatalf("question source/id: %+v", orch.lastInvestQ)
	}
	if orch.lastInvestQ.Text != "三条泳道是否共享同一驱动？" {
		t.Fatalf("generated text must resolve from parent brief: %q", orch.lastInvestQ.Text)
	}

	// 详情：result_kind/parent_result_id/question_key 齐备，不串成 brief。
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(inv.ID), nil)
	r.ServeHTTP(w, req)
	detail := w.Body.String()
	for _, want := range []string{`"board_investigation"`, `"parent_result_id"`, `"question_key"`} {
		if !jsonContains(detail, want) {
			t.Fatalf("investigation detail missing %s: %s", want, detail)
		}
	}
}

// M9.3 custom 问题：question 文本 trim 后进入调查链；空白拒绝 400 且 0 后台调用。
func TestBoardInvestigationTrigger_Custom(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief := briefResultRow(t, db, 5, `[{"id":"q1","question":"候选问题","rationale":"r","related_lane_ids":[]}]`)
	inv := investigationResultRow(t, db, 5, brief, "自填问题全文")
	orch := &mockOrchestrator{investOut: &service.BoardInvestigationOutput{Result: inv}}
	r := newBoardAnalysisRouter(t, orch, db)

	code, body := postInvestigation(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"question":"  自填问题全文  "}`, brief.ID))
	if code != http.StatusAccepted {
		t.Fatalf("custom trigger: want 202, got %d body=%v", code, body)
	}
	st := pollJobStatus(t, r, triggerEnvelope(t, body)["job_id"].(string))
	if errStr, _ := st["error"].(string); errStr != "" {
		t.Fatalf("custom investigation failed: %s", errStr)
	}
	if orch.lastInvestQ == nil || orch.lastInvestQ.Source != service.QuestionSourceCustom {
		t.Fatalf("custom source: %+v", orch.lastInvestQ)
	}
	if orch.lastInvestQ.Text != "自填问题全文" {
		t.Fatalf("custom text must be trimmed: %q", orch.lastInvestQ.Text)
	}

	// 空白变体：空串/纯空白/全空格+缺 question_id → 400，且 0 后台调用。
	for _, bad := range []string{
		fmt.Sprintf(`{"briefing_result_id":%d}`, brief.ID),
		fmt.Sprintf(`{"briefing_result_id":%d,"question":"   "}`, brief.ID),
		fmt.Sprintf(`{"briefing_result_id":%d,"question_id":"  "}`, brief.ID),
		`{"briefing_result_id":0,"question":"x"}`,
		`not-json`,
	} {
		code, body := postInvestigation(t, r, 5, bad)
		if code != http.StatusBadRequest {
			t.Fatalf("blank/missing question %q: want 400, got %d body=%v", bad, code, body)
		}
	}
	if orch.investCalls != 1 {
		t.Fatalf("invalid payloads must start 0 background jobs, got %d calls", orch.investCalls-1)
	}
}

// M6.8/M9 父结果校验：不存在 → 404；跨板块 → 404；legacy/调查类 parent → 400；
// generated question_id 不在父简报候选里 → 400。全部 0 后台调用。
func TestBoardInvestigationTrigger_ParentValidation(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief5 := briefResultRow(t, db, 5, `[{"id":"q1","question":"候选","rationale":"r","related_lane_ids":[]}]`)
	brief5NoQ := briefResultRow(t, db, 5, `[]`)
	brief6 := briefResultRow(t, db, 6, `[]`)
	legacy5 := legacyResultRow(t, db, 5, "旧命题")
	orch := &mockOrchestrator{}
	r := newBoardAnalysisRouter(t, orch, db)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"missing parent", `{"briefing_result_id":99999,"question":"q"}`, http.StatusNotFound},
		{"cross-board parent", fmt.Sprintf(`{"briefing_result_id":%d,"question":"q"}`, brief6.ID), http.StatusNotFound},
		{"legacy parent", fmt.Sprintf(`{"briefing_result_id":%d,"question":"q"}`, legacy5.ID), http.StatusBadRequest},
		{"unknown question_id", fmt.Sprintf(`{"briefing_result_id":%d,"question_id":"nope"}`, brief5.ID), http.StatusBadRequest},
		{"brief without questions", fmt.Sprintf(`{"briefing_result_id":%d,"question_id":"q1"}`, brief5NoQ.ID), http.StatusBadRequest},
	}
	for _, tc := range cases {
		code, body := postInvestigation(t, r, 5, tc.body)
		if code != tc.want {
			t.Fatalf("%s: want %d, got %d body=%v", tc.name, tc.want, code, body)
		}
	}
	if orch.investCalls != 0 {
		t.Fatalf("sync rejections must make 0 background calls, got %d", orch.investCalls)
	}
}

// 停用板块：investigation 触发同步 400（“not enabled”可区分），0 后台调用。
func TestBoardInvestigationTrigger_DisabledBoard(t *testing.T) {
	db := setupHandlerTestDB(t)
	orch := &mockOrchestrator{boardErr: fmt.Errorf("enrich board 5: enrichment not enabled for this board")}
	r := newBoardAnalysisRouter(t, orch, db)

	code, body := postInvestigation(t, r, 5, `{"briefing_result_id":1,"question":"q"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("disabled board: want 400, got %d body=%v", code, body)
	}
	if !jsonContains(fmt.Sprint(body), "not enabled") {
		t.Fatalf("error must be distinguishable: %v", body)
	}
	if orch.investCalls != 0 {
		t.Fatalf("disabled board must make 0 background calls, got %d", orch.investCalls)
	}
}

// M9.4 同 board 跨 kind：brief 在跑时 investigation → 409 携 brief 身份；
// 反之 investigation 在跑时 brief → 409 携 investigation 身份。
func TestBoardInvestigationTrigger_CrossKindConflict(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief := briefResultRow(t, db, 5, `[]`)
	block := make(chan struct{})
	orch := &mockOrchestrator{block: block, investBlock: block}
	r := newBoardAnalysisRouter(t, orch, db)
	defer close(block)

	// brief 先跑。
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/semantic-boards/5/enrichment/analysis/trigger", nil)
	r.ServeHTTP(w, req)
	var briefEnv struct {
		Data struct {
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	if w.Code != http.StatusAccepted {
		t.Fatalf("brief trigger: %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &briefEnv)

	code, body := postInvestigation(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"question":"q"}`, brief.ID))
	if code != http.StatusConflict {
		t.Fatalf("investigation during brief: want 409, got %d body=%v", code, body)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil || data["job_id"] != briefEnv.Data.JobID || data["job_kind"] != "board_brief" || data["running"] != true {
		t.Fatalf("409 must carry running brief identity, got %v (want job %s)", data, briefEnv.Data.JobID)
	}
}

// 反向：investigation 在跑时 brief trigger → 409 携 investigation 身份。
func TestBoardBriefTrigger_ConflictsWithRunningInvestigation(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief := briefResultRow(t, db, 5, `[]`)
	block := make(chan struct{})
	inv := investigationResultRow(t, db, 5, brief, "重跑问题")
	orch := &mockOrchestrator{investBlock: block, investOut: &service.BoardInvestigationOutput{Result: inv}}
	r := newBoardAnalysisRouter(t, orch, db)
	defer close(block)

	code, body := postInvestigation(t, r, 5, fmt.Sprintf(`{"briefing_result_id":%d,"question":"重跑问题"}`, brief.ID))
	if code != http.StatusAccepted {
		t.Fatalf("investigation trigger: want 202, got %d body=%v", code, body)
	}
	invJob := triggerEnvelope(t, body)["job_id"].(string)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/semantic-boards/5/enrichment/analysis/trigger", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("brief during investigation: want 409, got %d", w.Code)
	}
	var conflict struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &conflict)
	if conflict.Data["job_id"] != invJob || conflict.Data["job_kind"] != "board_investigation" {
		t.Fatalf("409 must carry investigation identity: %v", conflict.Data)
	}
}

// 不同 board 并行：board 5 的 brief 阻塞时 board 6 的 investigation 照常完成。
func TestBoardInvestigation_DifferentBoardsParallel(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief6 := briefResultRow(t, db, 6, `[]`)
	inv6 := investigationResultRow(t, db, 6, brief6, "六号问题")
	block := make(chan struct{})
	orch := &mockOrchestrator{
		block:     block, // board 5 brief blocks (EnrichBoard)
		investOut: &service.BoardInvestigationOutput{Result: inv6},
	}
	r := newBoardAnalysisRouter(t, orch, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/semantic-boards/5/enrichment/analysis/trigger", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("board5 brief trigger: %d", w.Code)
	}

	code, body := postInvestigation(t, r, 6, fmt.Sprintf(`{"briefing_result_id":%d,"question":"六号问题"}`, brief6.ID))
	if code != http.StatusAccepted {
		t.Fatalf("board6 investigation must start in parallel: %d body=%v", code, body)
	}
	st := pollJobStatus(t, r, triggerEnvelope(t, body)["job_id"].(string))
	if errStr, _ := st["error"].(string); errStr != "" {
		t.Fatalf("parallel investigation failed: %s", errStr)
	}
	close(block)
	pollBoardAnalysisStatus(t, r, 5)
}

// M9.5 防断连：触发成功后取消 request ctx，后台仍完成；同步预检阶段取消
// 只要不启动 job 即可（此处验证 202 后断连不杀后台）。
func TestBoardInvestigation_ClientDisconnect(t *testing.T) {
	db := setupHandlerTestDB(t)
	brief := briefResultRow(t, db, 5, `[]`)
	inv := investigationResultRow(t, db, 5, brief, "断连问题")
	orch := &mockOrchestrator{investOut: &service.BoardInvestigationOutput{Result: inv}}
	r := newBoardAnalysisRouter(t, orch, db)

	// 可取消的客户端请求 ctx：响应返回后立刻 cancel。
	clientCtx, cancelClient := context.WithCancel(context.Background())
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(clientCtx, "POST",
		"/semantic-boards/5/enrichment/analysis/investigations/trigger",
		bodyReader(fmt.Sprintf(`{"briefing_result_id":%d,"question":"断连问题"}`, brief.ID)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger: want 202, got %d", w.Code)
	}
	var env struct {
		Data struct {
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	cancelClient() // 客户端断线

	st := pollJobStatus(t, r, env.Data.JobID)
	if errStr, _ := st["error"].(string); errStr != "" {
		t.Fatalf("background investigation must survive client disconnect: %s", errStr)
	}
	if got, _ := st["result_id"].(float64); uint(got) != inv.ID {
		t.Fatalf("result_id = %v, want %d", st["result_id"], inv.ID)
	}
}

// M8.5/M9 kind 过滤：?kind= 三选一精确过滤；非法 kind → 400；缺省 = 全部；
// 严格 board 隔离；legacy 详情可读。
func TestBoardResultsKindFilterAndLegacyDetail(t *testing.T) {
	db := setupHandlerTestDB(t)
	briefA := briefResultRow(t, db, 5, `[]`)
	invA := investigationResultRow(t, db, 5, briefA, "调查一")
	legacyA := legacyResultRow(t, db, 5, "旧版命题")
	briefResultRow(t, db, 6, `[]`) // 其他 board，绝不泄漏
	r := newBoardAnalysisRouter(t, &mockOrchestrator{}, db)

	list := func(kind string) (int, string) {
		t.Helper()
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results"+kind, nil)
		r.ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	listIDs := func(kind string) map[uint]bool {
		t.Helper()
		code, body := list(kind)
		if code != http.StatusOK {
			t.Fatalf("list %q: want 200, got %d body=%s", kind, code, body)
		}
		var resp struct {
			Data []struct {
				ID uint `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("parse list %q: %v", kind, err)
		}
		ids := map[uint]bool{}
		for _, it := range resp.Data {
			ids[it.ID] = true
		}
		return ids
	}

	// 全部（board 5 三行，不含其他 board）。
	if ids := listIDs(""); len(ids) != 3 || !ids[briefA.ID] || !ids[invA.ID] || !ids[legacyA.ID] {
		t.Fatalf("list all must return exactly board5's three rows: %v", ids)
	}

	// kind 精确过滤：每档只含自己的 id。
	if ids := listIDs("?kind=board_brief"); len(ids) != 1 || !ids[briefA.ID] {
		t.Fatalf("kind=board_brief: %v", ids)
	}
	if ids := listIDs("?kind=board_investigation"); len(ids) != 1 || !ids[invA.ID] {
		t.Fatalf("kind=board_investigation: %v", ids)
	}
	if ids := listIDs("?kind=legacy_board_analysis"); len(ids) != 1 || !ids[legacyA.ID] {
		t.Fatalf("kind=legacy_board_analysis: %v", ids)
	}

	// 非法 kind → 400。
	if code, body := list("?kind=topic_analysis"); code != http.StatusBadRequest {
		t.Fatalf("kind=topic_analysis must be 400 (board-only enum), got %d body=%s", code, body)
	}
	if code, body := list("?kind=bogus"); code != http.StatusBadRequest {
		t.Fatalf("kind=bogus must be 400, got %d body=%s", code, body)
	}

	// legacy 详情可读且标注 result_kind=legacy_board_analysis。
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(legacyA.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy detail: %d", w.Code)
	}
	if !jsonContains(w.Body.String(), `"legacy_board_analysis"`) {
		t.Fatalf("legacy detail must carry result_kind: %s", w.Body.String())
	}
}

// D9 状态入口：未知 job_id → 404；scope/id 兼容入口从未触发 → idle。
func TestAnalysisStatus_ByJobIDAndIdle(t *testing.T) {
	db := setupHandlerTestDB(t)
	r := newBoardAnalysisRouter(t, &mockOrchestrator{}, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/enrichment/analysis-status?job_id=nope", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown job_id: want 404, got %d", w.Code)
	}

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/enrichment/analysis-status?scope=board&id=5", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("idle status: want 200, got %d", w2.Code)
	}
	if !jsonContains(w2.Body.String(), `"running":false`) {
		t.Fatalf("idle status body: %s", w2.Body.String())
	}
}

// genPayloadGuard：确认测试自身没拼坏 JSON（快速失败，避免误报 400 归因）。
func TestBoardInvestigationTestPayloads(t *testing.T) {
	for _, s := range []string{
		`{"briefing_result_id":1,"question_id":"q1"}`,
		`{"briefing_result_id":1,"question":"文本"}`,
	} {
		if !json.Valid([]byte(s)) {
			t.Fatalf("invalid test payload: %s", s)
		}
	}
	if strings.TrimSpace("  x  ") != "x" {
		t.Fatal("sanity")
	}
}
