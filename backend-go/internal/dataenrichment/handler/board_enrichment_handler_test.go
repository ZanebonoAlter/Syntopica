package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 版块级分析 API（tasks 4.1 / M6）──────────────────────────────────────────

func newBoardAnalysisRouter(t *testing.T, orch handler.Orchestrator, db *gorm.DB) *gin.Engine {
	t.Helper()
	repo := repository.NewRepository(db)
	gin.SetMode(gin.TestMode)
	h := handler.NewHandler(repo, nil, orch, &enabledBoardConfigReader{}, nil, nil, db)
	r := gin.New()
	h.RegisterRoutes(&r.RouterGroup)
	return r
}

// enabledBoardConfigReader always reports enrichment enabled (topic trigger
// gate passes; board trigger has its own resolver inside the orchestrator).
type enabledBoardConfigReader struct{}

func (enabledBoardConfigReader) GetBoardConfig(ctx context.Context, topicID uint) (*service.BoardEnrichmentConfig, error) {
	cfg := service.DefaultBoardConfig()
	cfg.EnrichmentEnabled = true
	return cfg, nil
}

func jsonContains(body, needle string) bool { return strings.Contains(body, needle) }

func itoa(v uint) string { return strconv.FormatUint(uint64(v), 10) }

func bodyReader(s string) io.Reader { return strings.NewReader(s) }

func boardResultRow(t *testing.T, db *gorm.DB, boardID uint, thesis string) *repository.TopicEnrichmentResult {
	t.Helper()
	repo := repository.NewRepository(db)
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		Sectors:         json.RawMessage(`{"scope":"board","thesis":"` + thesis + `"}`),
		SessionID:       "data_enrichment_board_" + thesis,
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), res); err != nil {
		t.Fatalf("seed board result: %v", err)
	}
	return res
}

// M6.1 未开启板块 → 4xx 可区分错误。
func TestBoardEnrichmentRoutes_DisabledBoardRejected(t *testing.T) {
	db := setupHandlerTestDB(t)
	orch := &mockOrchestrator{boardErr: fmt.Errorf("enrich board 5: enrichment not enabled for this board")}
	r := newBoardAnalysisRouter(t, orch, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/semantic-boards/5/enrichment/analysis/trigger", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("disabled board: want 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !jsonContains(w.Body.String(), "not enabled") {
		t.Fatalf("error must be distinguishable: %s", w.Body.String())
	}
}

// M9.1 现有 trigger = 简报（board_brief）：202 帧携唯一 job_id + kind；
// 后台跑完落库，status 轮询（按 job_id 与按 board 双入口）拿 result_id，
// 详情携带 sectors 五字段（fix-board-analysis-material 8.x + D9）。
func TestBoardEnrichmentRoutes_Trigger(t *testing.T) {
	db := setupHandlerTestDB(t)
	res := boardResultRow(t, db, 5, "命题甲")
	orch := &mockOrchestrator{boardOut: &service.BoardEnrichmentOutput{Result: res}}
	r := newBoardAnalysisRouter(t, orch, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/semantic-boards/5/enrichment/analysis/trigger", nil)
	r.ServeHTTP(w, req)
	var started struct {
		Status   string `json:"status"`
		JobID    string `json:"job_id"`
		JobKind  string `json:"job_kind"`
		Scope    string `json:"scope"`
		TargetID uint   `json:"target_id"`
	}
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger: want 202, got %d body=%s", w.Code, w.Body.String())
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("parse trigger body: %v", err)
	}
	if err := json.Unmarshal(envelope.Data, &started); err != nil {
		t.Fatalf("parse trigger data: %v", err)
	}
	if started.Status != "started" || started.JobID == "" || started.JobKind != "board_brief" || started.Scope != "board" || started.TargetID != 5 {
		t.Fatalf("trigger envelope: %+v", started)
	}

	// 按 job_id 精确轮询与按 board 兼容入口均能拿到终态。
	st := pollJobStatus(t, r, started.JobID)
	if errStr, _ := st["error"].(string); errStr != "" {
		t.Fatalf("background analysis failed: %s", errStr)
	}
	if kind, _ := st["job_kind"].(string); kind != "board_brief" {
		t.Fatalf("status job_kind = %v, want board_brief", st["job_kind"])
	}
	stBoard := pollBoardAnalysisStatus(t, r, 5)
	gotID, _ := st["result_id"].(float64)
	if uint(gotID) != res.ID {
		t.Fatalf("result_id = %v, want %d", st["result_id"], res.ID)
	}
	if bID, _ := stBoard["result_id"].(float64); uint(bID) != res.ID {
		t.Fatalf("board-entry result_id = %v, want %d", stBoard["result_id"], res.ID)
	}

	// Detail carries the five-sector payload.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(res.ID), nil)
	r.ServeHTTP(w2, req2)
	for _, want := range []string{`"analysis_scope"`, `"sectors"`, `"thesis"`, `"session_id"`} {
		if !jsonContains(w2.Body.String(), want) {
			t.Fatalf("detail missing %s: %s", want, w2.Body.String())
		}
	}
}

// M9.4 防重入：同板块在跑时重复触发 → 409 且 data 携当前 job 身份；跑完后可再触发。
func TestBoardEnrichmentRoutes_AlreadyRunning(t *testing.T) {
	db := setupHandlerTestDB(t)
	block := make(chan struct{})
	orch := &mockOrchestrator{block: block}
	r := newBoardAnalysisRouter(t, orch, db)

	do := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/semantic-boards/5/enrichment/analysis/trigger", nil)
		r.ServeHTTP(w, req)
		return w
	}
	first := do()
	if first.Code != http.StatusAccepted {
		t.Fatalf("first trigger: want 202, got %d", first.Code)
	}
	var env struct {
		Data struct {
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &env)

	second := do()
	if second.Code != http.StatusConflict {
		t.Fatalf("second trigger while running: want 409, got %d", second.Code)
	}
	var conflict struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &conflict); err != nil {
		t.Fatalf("parse 409 body: %v", err)
	}
	if id, _ := conflict.Data["job_id"].(string); id != env.Data.JobID {
		t.Fatalf("409 data.job_id = %v, want the running job %q", conflict.Data["job_id"], env.Data.JobID)
	}
	if kind, _ := conflict.Data["job_kind"].(string); kind != "board_brief" {
		t.Fatalf("409 data.job_kind = %v, want board_brief", conflict.Data["job_kind"])
	}
	if running, _ := conflict.Data["running"].(bool); !running {
		t.Fatalf("409 data.running must be true: %v", conflict.Data)
	}

	close(block)
	pollBoardAnalysisStatus(t, r, 5)
	if code := do().Code; code != http.StatusAccepted {
		t.Fatalf("re-trigger after finish: want 202, got %d", code)
	}
}

// pollJobStatus polls the by-job_id status endpoint until finished (D9).
func pollJobStatus(t *testing.T, r *gin.Engine, jobID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/enrichment/analysis-status?job_id="+jobID, nil)
		r.ServeHTTP(w, req)
		var resp struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if finished, _ := resp.Data["finished"].(bool); finished {
			return resp.Data
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s not finished in 5s: %v", jobID, resp.Data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// pollBoardAnalysisStatus polls the async status endpoint (no /api prefix in
// this router) until the board job finishes.
func pollBoardAnalysisStatus(t *testing.T, r *gin.Engine, boardID uint) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/enrichment/analysis-status?scope=board&id=%d", boardID), nil)
		r.ServeHTTP(w, req)
		var resp struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if finished, _ := resp.Data["finished"].(bool); finished {
			return resp.Data
		}
		if time.Now().After(deadline) {
			t.Fatalf("board analysis not finished in 5s: %v", resp.Data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// M6.3 列表 scope 隔离：board 档列表不含 topic 档；详情拒绝他板块 result。
func TestBoardEnrichmentRoutes_ListAndDetail(t *testing.T) {
	db := setupHandlerTestDB(t)
	b1 := boardResultRow(t, db, 5, "命题一")
	boardResultRow(t, db, 6, "命题二") // other board — must not leak
	// topic-scope row for board 5's lane — must not leak either.
	topicRes := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(501),
		AnalysisScope:     "topic",
		Sectors:           json.RawMessage(`{"form":"sparse"}`),
		SessionID:         "topic-scope",
	}
	if err := repository.NewRepository(db).CreateTopicEnrichmentResult(context.Background(), topicRes); err != nil {
		t.Fatalf("seed topic result: %v", err)
	}
	r := newBoardAnalysisRouter(t, &mockOrchestrator{}, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d", w.Code)
	}
	body := w.Body.String()
	if !jsonContains(body, "命题一") {
		t.Fatal("own board result must be listed")
	}
	if jsonContains(body, "命题二") || jsonContains(body, "topic-scope") {
		t.Fatalf("foreign/topic-scope results leaked: %s", body)
	}

	// Detail: own result OK.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(b1.ID), nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("detail own: %d", w2.Code)
	}

	// Detail: foreign board's result → 404.
	foreign := boardResultRow(t, db, 6, "命题二b")
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(foreign.ID), nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("detail foreign: want 404, got %d", w3.Code)
	}
}

// 6.x review hardening：详情读路径显式要求 analysis_scope=board。造一条脏行
// （semantic_board_id=5 但 analysis_scope='topic'，直接 GORM Create 绕过
// repository.CreateTopicEnrichmentResult 的形状校验，模拟历史/手工写入的
// 不一致数据）：属主校验通过但 scope 不一致 → 统一 404，不得当作 board 档
// 报告透出；列表（repository scope 过滤）同样不泄漏，不变量未被削弱。
func TestBoardEnrichmentRoutes_DetailScopeMismatch(t *testing.T) {
	db := setupHandlerTestDB(t)
	b1 := boardResultRow(t, db, 5, "命题一")
	// 脏 fixture：板块属主 + topic scope（repository 写入路径不可能产生，
	// 但原始/历史行可能；读路径必须自防御）。
	dirty := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(5),
		AnalysisScope:   "topic",
		ResultKind:      "topic_analysis",
		Sectors:         json.RawMessage(`{"form":"sparse"}`),
		SessionID:       "scope-mismatch-fixture",
	}
	if err := db.Create(dirty).Error; err != nil {
		t.Fatalf("seed dirty scope row: %v", err)
	}
	r := newBoardAnalysisRouter(t, &mockOrchestrator{}, db)

	// 详情：同板块属主但 scope 不一致 → 404（与跨板块/不存在统一，不区分）。
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(dirty.ID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("scope-mismatch detail: want 404, got %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "scope-mismatch-fixture") {
		t.Fatalf("scope-mismatch row must not leak in body: %s", w.Body.String())
	}

	// 列表：脏行不泄漏（repository scope 过滤不变量保持）。
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("list: %d", w2.Code)
	}
	body := w2.Body.String()
	if jsonContains(body, "scope-mismatch-fixture") {
		t.Fatalf("scope-mismatch row leaked into list: %s", body)
	}
	if !jsonContains(body, "命题一") {
		t.Fatal("clean board result must still be listed")
	}

	// 对照：正常 board 档行仍 200（守卫不误伤）。
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/semantic-boards/5/enrichment/analysis/results/"+itoa(b1.ID), nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("clean board detail: want 200, got %d", w3.Code)
	}
}

// M6.4 prefill_lens：单泳道 trigger body 可选字段透传（异步化重写：错误进
// status，透传断言不变）。
func TestTopicTrigger_PrefillLens(t *testing.T) {
	db := setupHandlerTestDB(t)
	orch := &mockOrchestrator{shouldFail: true} // failure lands in status.error
	r := newBoardAnalysisRouter(t, orch, db)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/persistent-topics/501/enrichment/results/trigger", bodyReader(`{"prefill_lens":"供需错配"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("trigger: want 202 started, got %d body=%s", w.Code, w.Body.String())
	}

	st := pollTopicAnalysisStatus(t, r, 501)
	if errStr, _ := st["error"].(string); errStr == "" {
		t.Fatal("failing orchestrator must surface in status.error")
	}
	if orch.lastTopicID != 501 {
		t.Fatalf("prefill_lens body must not break topic resolution, got %d", orch.lastTopicID)
	}
	if orch.lastLens != "供需错配" {
		t.Fatalf("prefill_lens must reach the orchestrator, got %q", orch.lastLens)
	}
}

func pollTopicAnalysisStatus(t *testing.T, r *gin.Engine, topicID uint) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/enrichment/analysis-status?scope=topic&id=%d", topicID), nil)
		r.ServeHTTP(w, req)
		var resp struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if finished, _ := resp.Data["finished"].(bool); finished {
			return resp.Data
		}
		if time.Now().After(deadline) {
			t.Fatalf("topic analysis not finished in 5s: %v", resp.Data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
