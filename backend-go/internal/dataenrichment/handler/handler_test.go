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
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/database"
)

// Test-only source type: the source_type enum is extensible (spec "板块数据源
// 绑定"); built-in financial types were removed, so tests register a neutral
// dummy to exercise the data-source CRUD endpoints without coupling to any
// built-in type.
func init() {
	repository.RegisterSourceType("test_source")
	repository.RegisterSourceType("test_source_2")
}

// ── Test helpers ────────────────────────────────────────────────────────────

func setupHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.DB = db
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repo := repository.NewRepository(db)
	repository.SetRepo(repo)
	return db
}

// alwaysEnabledBoardConfig returns enrichment enabled for any topic.
type alwaysEnabledBoardConfig struct{}

func (c *alwaysEnabledBoardConfig) GetBoardConfig(ctx context.Context, topicID uint) (*service.BoardEnrichmentConfig, error) {
	return &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        14,
		ContextLayers:     []string{"week", "month", "year", "all"},
	}, nil
}

// alwaysDisabledBoardConfig returns enrichment disabled for any topic.
type alwaysDisabledBoardConfig struct{}

func (c *alwaysDisabledBoardConfig) GetBoardConfig(ctx context.Context, topicID uint) (*service.BoardEnrichmentConfig, error) {
	return service.DefaultBoardConfig(), nil
}

// mockLifelineService is a mock for handler.LifelineService that records calls.
type mockLifelineService struct {
	lastTopicID     uint
	lastGranularity string
	lastPeriod      string
	shouldFail      bool
}

func (m *mockLifelineService) RefreshGranularity(ctx context.Context, topicID uint, granularity string, now time.Time) error {
	m.lastTopicID = topicID
	m.lastGranularity = granularity
	if m.shouldFail {
		return fmt.Errorf("mock refresh error")
	}
	return nil
}

func (m *mockLifelineService) RefreshPeriod(ctx context.Context, topicID uint, granularity, period string, now time.Time) error {
	m.lastTopicID = topicID
	m.lastGranularity = granularity
	m.lastPeriod = period
	if m.shouldFail {
		return fmt.Errorf("mock refresh error")
	}
	return nil
}

// mockOrchestrator is a mock for handler.Orchestrator.
type mockOrchestrator struct {
	lastTopicID uint
	lastLens    string
	shouldFail  bool
	output      *service.EnrichmentOutput
	boardOut    *service.BoardEnrichmentOutput
	boardErr    error
	block       chan struct{} // non-nil → EnrichBoard waits until closed (re-entry tests)

	// investigation mock (D9 5.x): investBlock gates the background run;
	// investCalls counts entries (0-call assertions for sync rejections).
	investOut        *service.BoardInvestigationOutput
	investErr        error
	investBlock      chan struct{}
	investCalls      int
	lastInvestBoard  uint
	lastInvestParent uint
	lastInvestQ      *service.BoardInvestigationQuestion

	// relation discovery mock (add-evidence-backed-cross-board-relations):
	// relationBlock gates the background run; relationCalls counts entries.
	relationOut     *service.RelationDiscoveryOutput
	relationErr     error
	relationBlock   chan struct{}
	relationCalls   int
	lastRelationIn  *service.RelationDiscoveryInput
	relationInputs  []service.RelationDiscoveryInput
	reResolveOut    *service.RelationReResolveOutput
	reResolveErr    error
	reResolveCalls  int
	lastReResolveID uint
}

func (m *mockOrchestrator) BoardEnrichmentEnabled(ctx context.Context, boardID uint) error {
	return m.boardErr
}

func (m *mockOrchestrator) EnrichTopicLens(ctx context.Context, topicID uint, prefillLens string) (*service.EnrichmentOutput, error) {
	m.lastLens = prefillLens
	return m.EnrichTopic(ctx, topicID)
}

func (m *mockOrchestrator) EnrichBoard(ctx context.Context, boardID uint) (*service.BoardEnrichmentOutput, error) {
	if m.block != nil {
		<-m.block
	}
	if m.boardErr != nil {
		return nil, m.boardErr
	}
	if m.boardOut != nil {
		return m.boardOut, nil
	}
	return nil, fmt.Errorf("mock EnrichBoard: not configured")
}

func (m *mockOrchestrator) EnrichTopic(ctx context.Context, topicID uint) (*service.EnrichmentOutput, error) {
	m.lastTopicID = topicID
	if m.shouldFail {
		return nil, fmt.Errorf("mock enrich error")
	}
	return m.output, nil
}

func (m *mockOrchestrator) InvestigateBoardQuestion(ctx context.Context, boardID uint, parentBriefID uint, question service.BoardInvestigationQuestion) (*service.BoardInvestigationOutput, error) {
	m.investCalls++
	m.lastInvestBoard = boardID
	m.lastInvestParent = parentBriefID
	q := question
	m.lastInvestQ = &q
	if m.investBlock != nil {
		<-m.investBlock
	}
	if m.investErr != nil {
		return nil, m.investErr
	}
	if m.investOut != nil {
		return m.investOut, nil
	}
	return nil, fmt.Errorf("mock InvestigateBoardQuestion: not configured")
}

func newTestHandler(db *gorm.DB, lifelineSvc handler.LifelineService, orch handler.Orchestrator, cfg service.BoardConfigReader) *handler.EnrichmentHandler {
	return handler.NewHandler(repository.Repo, lifelineSvc, orch, cfg, nil, nil, db)
}

// newTestRouter creates a Gin engine with the handler routes registered.
func newTestRouter(h *handler.EnrichmentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h.RegisterRoutes(engine.Group("/api"))
	return engine
}

// doRequest is a convenience helper for test HTTP requests.
func doRequest(t *testing.T, r *gin.Engine, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// expectJSONSuccess asserts the response has success=true and parses data into v.
func expectJSONSuccess(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	expectJSONStatus(t, w, http.StatusOK, v)
}

// expectJSONAccepted asserts the async-trigger envelope: 202 + success payload
// (D9: trigger endpoints are "started", not "done").
func expectJSONAccepted(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	expectJSONStatus(t, w, http.StatusAccepted, v)
}

func expectJSONStatus(t *testing.T, w *httptest.ResponseRecorder, status int, v any) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, w.Code, w.Body.String())
	}
	var resp struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success=true, got error=%q", resp.Error)
	}
	if v != nil {
		if err := json.Unmarshal(resp.Data, v); err != nil {
			t.Fatalf("parse data: %v", err)
		}
	}
}

// expectJSONError asserts the response has success=false with the given status.
func expectJSONError(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	if w.Code != status {
		t.Fatalf("expected %d, got %d: %s", status, w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if s, ok := resp["success"].(bool); ok && s {
		t.Fatal("expected success=false")
	}
	return resp
}

// ── Route smoke test ────────────────────────────────────────────────────────

func TestRoutesRegisterWithoutPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// Use a handler with all nil dependencies — route registration shouldn't
	// call any service method, it just registers path patterns.
	h := &handler.EnrichmentHandler{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterRoutes panicked: %v", r)
		}
	}()
	h.RegisterRoutes(engine.Group("/api"))
}

// ── Context CRUD tests ──────────────────────────────────────────────────────

func TestListContexts(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Seed a context.
	lc := &repository.TopicLifelineContext{
		PersistentTopicID: 1, Granularity: "week", Period: "2026-W27", Content: "week summary",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/contexts", "")
	var list []repository.TopicLifelineContext
	expectJSONSuccess(t, w, &list)
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1", len(list))
	}
	if list[0].Granularity != "week" {
		t.Fatalf("granularity = %q, want week", list[0].Granularity)
	}
}

func TestGetContext(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	lc := &repository.TopicLifelineContext{
		PersistentTopicID: 1, Granularity: "month", Period: "2026-07", Content: "month summary",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/contexts/month/2026-07", "")
	var got repository.TopicLifelineContext
	expectJSONSuccess(t, w, &got)
	if got.Content != "month summary" {
		t.Fatalf("content = %q, want 'month summary'", got.Content)
	}
}

func TestGetContextNotFound(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/contexts/week/2026-W27", "")
	expectJSONError(t, w, http.StatusNotFound)
}

func TestGetContextInvalidGranularity(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/contexts/fake/2026", "")
	expectJSONError(t, w, http.StatusBadRequest)
}

func TestUpdateContext(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	lc := &repository.TopicLifelineContext{
		PersistentTopicID: 1, Granularity: "week", Period: "2026-W27", Content: "original content",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	body := `{"content": "edited content"}`
	w := doRequest(t, r, "PUT", "/api/persistent-topics/1/enrichment/contexts/week/2026-W27", body)
	var got repository.TopicLifelineContext
	expectJSONSuccess(t, w, &got)
	if got.Content != "edited content" {
		t.Fatalf("content = %q, want 'edited content'", got.Content)
	}
	if got.Source != "manual" {
		t.Fatalf("source = %q, want 'manual'", got.Source)
	}
}

func TestRegenerateContext(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Seed a context for the regenerate to overwrite.
	lc := &repository.TopicLifelineContext{
		PersistentTopicID: 1, Granularity: "week", Period: "2026-W27", Content: "old content",
	}
	if err := repository.Repo.UpsertTopicLifelineContext(ctx, lc); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mockLifeline := &mockLifelineService{}
	h := newTestHandler(db, mockLifeline, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "POST", "/api/persistent-topics/1/enrichment/contexts/week/regenerate?period=2026-W27", "")
	expectJSONSuccess(t, w, nil)

	if mockLifeline.lastTopicID != 1 || mockLifeline.lastGranularity != "week" {
		t.Fatalf("RefreshGranularity called with (%d, %s), want (1, week)",
			mockLifeline.lastTopicID, mockLifeline.lastGranularity)
	}
}

// ── Result CRUD tests ───────────────────────────────────────────────────────

func TestListResults(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	r1 := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "first", SessionID: "s1",
	}
	r2 := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1), EvolutionAssessment: "second", SessionID: "s2",
	}
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r1)
	_ = repository.Repo.CreateTopicEnrichmentResult(ctx, r2)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/results", "")
	var list []map[string]any
	expectJSONSuccess(t, w, &list)
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
	// First item should have the largest ID (newest first).
	id1, _ := list[0]["id"].(float64)
	id2, _ := list[1]["id"].(float64)
	if id1 <= id2 {
		t.Fatal("expected descending order by id")
	}
}

func TestGetResult(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	result := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(1),
		EvolutionAssessment: "test assessment",
		SessionID:           "session-123",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d", result.ID), "")
	var got map[string]any
	expectJSONSuccess(t, w, &got)
	if id, _ := got["id"].(float64); uint(id) != result.ID {
		t.Fatalf("id = %v, want %d", id, result.ID)
	}
	if sid, _ := got["session_id"].(string); sid != "session-123" {
		t.Fatalf("session_id = %q, want 'session-123'", sid)
	}
}

func TestGetResultNotFound(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/results/99999", "")
	expectJSONError(t, w, http.StatusNotFound)
}

func TestGetResultIDORProtection(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Create result for topic 1.
	result1 := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(1),
		EvolutionAssessment: "topic 1 result",
		SessionID:           "session-1",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result1); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	// Cross-topic access: topic 2 tries to read topic 1's result → 404.
	w := doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/2/enrichment/results/%d", result1.ID), "")
	expectJSONError(t, w, http.StatusNotFound)

	// Same-topic access → 200.
	w2 := doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d", result1.ID), "")
	var got map[string]any
	expectJSONSuccess(t, w2, &got)
	if id, _ := got["id"].(float64); uint(id) != result1.ID {
		t.Fatalf("id = %v, want %d", id, result1.ID)
	}
}

func TestTriggerEnrichmentSuccess(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Seed a result that the orchestrator mock will "create".
	result := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(1),
		EvolutionAssessment: "mock enrichment result",
		SessionID:           "mock-session",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mockOrch := &mockOrchestrator{
		output: &service.EnrichmentOutput{
			Result: result,
		},
	}
	h := newTestHandler(db, nil, mockOrch, &alwaysEnabledBoardConfig{})
	r := newTestRouter(h)

	w := doRequest(t, r, "POST", "/api/persistent-topics/1/enrichment/results/trigger", "")
	var started struct {
		JobID   string `json:"job_id"`
		JobKind string `json:"job_kind"`
		Status  string `json:"status"`
		Scope   string `json:"scope"`
	}
	expectJSONAccepted(t, w, &started)
	if started.JobID == "" || started.JobKind != "topic_analysis" || started.Status != "started" || started.Scope != "topic" {
		t.Fatalf("trigger envelope: %+v", started)
	}

	// Async: poll status until finished, then assert the persisted id and
	// that EnrichTopic really ran (with the right topic).
	st := pollAnalysisStatus(t, r, "topic", 1)
	if errStr, _ := st["error"].(string); errStr != "" {
		t.Fatalf("analysis failed: %v", errStr)
	}
	if got, ok := st["result_id"].(float64); !ok || uint(got) != result.ID {
		t.Fatalf("result_id = %v, want %d", st["result_id"], result.ID)
	}
	if mockOrch.lastTopicID != 1 {
		t.Fatalf("EnrichTopic called with %d, want 1", mockOrch.lastTopicID)
	}
}

// pollAnalysisStatus hits GET /enrichment/analysis-status until the job is
// finished (or times out — background goroutine must complete quickly in tests).
func pollAnalysisStatus(t *testing.T, r *gin.Engine, scope string, id uint) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := doRequest(t, r, "GET", fmt.Sprintf("/api/enrichment/analysis-status?scope=%s&id=%d", scope, id), "")
		var resp struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if finished, _ := resp.Data["finished"].(bool); finished {
			return resp.Data
		}
		if time.Now().After(deadline) {
			t.Fatalf("analysis not finished in 5s: %v", resp.Data)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTriggerEnrichmentDisabled(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, &alwaysDisabledBoardConfig{})
	r := newTestRouter(h)

	w := doRequest(t, r, "POST", "/api/persistent-topics/1/enrichment/results/trigger", "")
	resp := expectJSONError(t, w, http.StatusBadRequest)
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "not enabled") {
		t.Fatalf("expected 'not enabled' error, got %q", errMsg)
	}
}

// ── Review CRUD tests ───────────────────────────────────────────────────────

func TestListReviews(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	rv1 := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 10, DeviationSummary: "review 1",
	}
	rv2 := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 20, DeviationSummary: "review 2",
	}
	_ = repository.Repo.CreateTopicEnrichmentReview(ctx, rv1)
	_ = repository.Repo.CreateTopicEnrichmentReview(ctx, rv2)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/persistent-topics/1/enrichment/reviews", "")
	var list []repository.TopicEnrichmentReview
	expectJSONSuccess(t, w, &list)
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
}

func TestUpdateReviewDeviation(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	rv := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 10, DeviationSummary: "original",
	}
	if err := repository.Repo.CreateTopicEnrichmentReview(ctx, rv); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	body := `{"deviation_summary": "updated deviation"}`
	w := doRequest(t, r, "PUT", fmt.Sprintf("/api/persistent-topics/1/enrichment/reviews/%d", rv.ID), body)
	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w, &got)
	if got.DeviationSummary != "updated deviation" {
		t.Fatalf("deviation = %q, want 'updated deviation'", got.DeviationSummary)
	}
}

func TestApplyReview(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	rv := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 10, DeviationSummary: "needs apply", Applied: false,
	}
	if err := repository.Repo.CreateTopicEnrichmentReview(ctx, rv); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/1/enrichment/reviews/%d/apply", rv.ID), "")
	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w, &got)
	if !got.Applied {
		t.Fatal("expected applied=true after apply")
	}

	// Verify context table was NOT modified (design §4.3: no write-back to table 1).
	// This is an architectural invariant tested here.
	// Context table is empty — if apply wrote back, there'd be content.
	ctxList, _ := repository.Repo.ListTopicLifelineContextsByTopic(ctx, 1)
	if len(ctxList) > 0 {
		t.Fatal("applyReview should NOT write back to table 1 (topic_lifeline_context)")
	}
}

func TestCreateManualReview(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	body := `{"curr_result_id": 1, "deviation_summary": "manual annotation"}`
	w := doRequest(t, r, "POST", "/api/persistent-topics/1/enrichment/reviews", body)

	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w, &got)
	if got.Source != "manual" {
		t.Fatalf("source = %q, want 'manual'", got.Source)
	}
	if !got.Applied {
		t.Fatal("expected applied=true for manual reviews")
	}
	if got.DeviationSummary != "manual annotation" {
		t.Fatalf("deviation = %q, want 'manual annotation'", got.DeviationSummary)
	}
}

func TestCreateManualReviewWithPrevResultID(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	prevID := uint(5)
	body := fmt.Sprintf(`{"curr_result_id": 2, "deviation_summary": "with prev", "prev_result_id": %d}`, prevID)
	w := doRequest(t, r, "POST", "/api/persistent-topics/1/enrichment/reviews", body)

	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w, &got)
	if got.PrevResultID == nil || *got.PrevResultID != 5 {
		t.Fatalf("prev_result_id = %v, want 5", got.PrevResultID)
	}
}

func TestCreateManualReviewPrevResultIDOmittable(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	body := `{"curr_result_id": 3, "deviation_summary": "no prev"}`
	w := doRequest(t, r, "POST", "/api/persistent-topics/1/enrichment/reviews", body)

	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w, &got)
	if got.PrevResultID != nil {
		t.Fatalf("prev_result_id should be nil when omitted, got %v", *got.PrevResultID)
	}
}

// ── Board data source tests ─────────────────────────────────────────────────

func TestListDataSources(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	ds1 := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source", Enabled: true}
	ds2 := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source_2", Enabled: true}
	_ = repository.Repo.CreateBoardDataSource(ctx, ds1)
	_ = repository.Repo.CreateBoardDataSource(ctx, ds2)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", "/api/semantic-boards/1/data-sources", "")
	var list []repository.BoardDataSource
	expectJSONSuccess(t, w, &list)
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
}

func TestUpsertDataSource(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	body := `{"source_type": "test_source", "config": {"keywords": ["半导体"]}, "enabled": true}`
	w := doRequest(t, r, "PUT", "/api/semantic-boards/1/data-sources", body)
	var ds repository.BoardDataSource
	expectJSONSuccess(t, w, &ds)

	// Verify persisted.
	got, err := repository.Repo.GetBoardDataSourceByBoardAndType(ctx, 1, "test_source")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
}

func TestUpsertDataSourceUniqueConstraint(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Pre-create.
	ds := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source", Enabled: true}
	_ = repository.Repo.CreateBoardDataSource(ctx, ds)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	// Upsert should succeed (update not insert).
	body := `{"source_type": "test_source", "enabled": false}`
	w := doRequest(t, r, "PUT", "/api/semantic-boards/1/data-sources", body)
	var updated repository.BoardDataSource
	expectJSONSuccess(t, w, &updated)
	if updated.Enabled {
		t.Fatal("expected enabled=false after upsert update")
	}
}

func TestDeleteDataSource(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	ds := &repository.BoardDataSource{SemanticBoardID: 1, SourceType: "test_source", Enabled: true}
	if err := repository.Repo.CreateBoardDataSource(ctx, ds); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "DELETE", "/api/semantic-boards/1/data-sources/test_source", "")
	var got map[string]any
	expectJSONSuccess(t, w, &got)
	if d, _ := got["deleted"].(bool); !d {
		t.Fatal("expected deleted=true")
	}

	// Verify deleted.
	_, err := repository.Repo.GetBoardDataSourceByBoardAndType(ctx, 1, "test_source")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteDataSourceNotFound(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "DELETE", "/api/semantic-boards/1/data-sources/nonexistent", "")
	expectJSONError(t, w, http.StatusNotFound)
}

// ── Invalid param tests ─────────────────────────────────────────────────────

func TestInvalidTopicID(t *testing.T) {
	db := setupHandlerTestDB(t)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	tests := []struct{ path string }{
		{"/api/persistent-topics/abc/enrichment/contexts"},
		{"/api/persistent-topics/0/enrichment/contexts"},
		{"/api/persistent-topics/abc/enrichment/results"},
		{"/api/persistent-topics/abc/enrichment/reviews"},
	}
	for _, tt := range tests {
		w := doRequest(t, r, "GET", tt.path, "")
		expectJSONError(t, w, http.StatusBadRequest)
	}
}

// ── Report follow-up Q&A handler tests (causal-analysis-agent 阶段2b) ─────────

// mockQARunner is a mock for handler.QARunner.
type mockQARunner struct {
	lastResultID uint
	lastQuestion string
	answer       *service.QAAnswer
	shouldFail   bool
}

func (m *mockQARunner) Ask(ctx context.Context, resultID uint, question string) (*service.QAAnswer, error) {
	m.lastResultID = resultID
	m.lastQuestion = question
	if m.shouldFail {
		return nil, fmt.Errorf("mock qa error")
	}
	return m.answer, nil
}

// newTestHandlerWithQA builds a handler wired with a QA runner (other deps nil).
func newTestHandlerWithQA(db *gorm.DB, qa handler.QARunner) *handler.EnrichmentHandler {
	return handler.NewHandler(repository.Repo, nil, nil, nil, nil, qa, db)
}

func TestAskQA(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	result := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1),
		SessionID:         "session-qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}

	canned := &service.QAAnswer{
		Answer: "油价短期承压(推演有据)",
		Refs:   []service.Ref{{SourceType: "tool", Ref: "list_boards"}},
	}
	mockQA := &mockQARunner{answer: canned}
	h := newTestHandlerWithQA(db, mockQA)
	r := newTestRouter(h)

	body := `{"question": "油价还会涨吗"}`
	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d/qa", result.ID), body)
	var got service.QAAnswer
	expectJSONSuccess(t, w, &got)
	if got.Answer != "油价短期承压(推演有据)" {
		t.Fatalf("answer: got %q", got.Answer)
	}

	if mockQA.lastResultID != result.ID {
		t.Fatalf("Ask called with resultID %d, want %d", mockQA.lastResultID, result.ID)
	}
	if mockQA.lastQuestion != "油价还会涨吗" {
		t.Fatalf("Ask called with question %q", mockQA.lastQuestion)
	}
}

func TestAskQA_IDORProtection(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Result belongs to topic 1.
	result := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), SessionID: "s"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}

	mockQA := &mockQARunner{answer: &service.QAAnswer{Answer: "x"}}
	h := newTestHandlerWithQA(db, mockQA)
	r := newTestRouter(h)

	// Topic 2 tries to ask against topic 1's result → 404, Ask never called.
	body := `{"question": "x"}`
	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/2/enrichment/results/%d/qa", result.ID), body)
	expectJSONError(t, w, http.StatusNotFound)
	if mockQA.lastQuestion != "" {
		t.Fatal("QARunner.Ask should NOT be called on cross-topic access")
	}
}

func TestAskQA_MissingQuestion(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	result := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), SessionID: "s"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandlerWithQA(db, &mockQARunner{})
	r := newTestRouter(h)

	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d/qa", result.ID), `{}`)
	expectJSONError(t, w, http.StatusBadRequest)
}

func TestListQA(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	result := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), SessionID: "s"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}

	older := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 18, 10, 5, 0, 0, time.UTC)

	qa1 := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: result.ID, Question: "第一轮", Source: "qa", CreatedAt: older,
	}
	qa2 := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: result.ID, Question: "第二轮", Source: "qa", CreatedAt: newer,
	}
	// Insert out of order to prove the handler returns created_at ASC.
	_ = repository.Repo.CreateTopicEnrichmentQA(ctx, qa2)
	_ = repository.Repo.CreateTopicEnrichmentQA(ctx, qa1)

	h := newTestHandler(db, nil, nil, nil) // qa GET needs no QA runner
	r := newTestRouter(h)

	w := doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d/qa", result.ID), "")
	var list []repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &list)
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}
	if list[0].Question != "第一轮" || list[1].Question != "第二轮" {
		t.Fatalf("order = [%q, %q], want [第一轮, 第二轮] (created_at ASC)",
			list[0].Question, list[1].Question)
	}
}

func TestSedimentQA(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	result := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), SessionID: "s"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}
	qa := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: result.ID, Question: "油价还会涨吗", Answer: "短期承压", Source: "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa); err != nil {
		t.Fatalf("seed qa: %v", err)
	}

	// Snapshot the result to prove sediment never rewrites it.
	resultBefore, _ := repository.Repo.GetTopicEnrichmentResultByID(ctx, result.ID)

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/1/enrichment/qa/%d/sediment", qa.ID), "")
	var got repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &got)
	if !got.Sedimented {
		t.Fatal("expected sedimented=true after POST sediment")
	}

	// The result table is unchanged (报告不可变).
	resultAfter, _ := repository.Repo.GetTopicEnrichmentResultByID(ctx, result.ID)
	beforeJSON, _ := json.Marshal(resultBefore)
	afterJSON, _ := json.Marshal(resultAfter)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("result table must be immutable across sediment")
	}
}

// ── IDOR protection tests for review / qa-sediment (H1) ──────────────────────
//
// updateReviewDeviation, applyReview, sedimentQA accept :topicId in the path
// but historically never validated the resource belongs to that topic.
// These tests reproduce the cross-topic access; the fix must return 404.

func TestUpdateReviewDeviation_IDORProtection(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Review belongs to topic 1.
	rv := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 10, DeviationSummary: "original",
	}
	if err := repository.Repo.CreateTopicEnrichmentReview(ctx, rv); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	// Topic 2 tries to edit topic 1's review → 404, deviation unchanged.
	body := `{"deviation_summary": "hijacked"}`
	w := doRequest(t, r, "PUT", fmt.Sprintf("/api/persistent-topics/2/enrichment/reviews/%d", rv.ID), body)
	expectJSONError(t, w, http.StatusNotFound)

	// Confirm the deviation was NOT modified.
	unchanged, _ := repository.Repo.GetTopicEnrichmentReviewByID(ctx, rv.ID)
	if unchanged.DeviationSummary != "original" {
		t.Fatalf("deviation should be unchanged, got %q", unchanged.DeviationSummary)
	}

	// Same-topic access → 200.
	w2 := doRequest(t, r, "PUT", fmt.Sprintf("/api/persistent-topics/1/enrichment/reviews/%d", rv.ID), body)
	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w2, &got)
	if got.DeviationSummary != "hijacked" {
		t.Fatalf("deviation = %q, want 'hijacked'", got.DeviationSummary)
	}
}

func TestApplyReview_IDORProtection(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	rv := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1), CurrResultID: 10, DeviationSummary: "x", Applied: false,
	}
	if err := repository.Repo.CreateTopicEnrichmentReview(ctx, rv); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	// Topic 2 tries to apply topic 1's review → 404, not applied.
	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/2/enrichment/reviews/%d/apply", rv.ID), "")
	expectJSONError(t, w, http.StatusNotFound)

	unchanged, _ := repository.Repo.GetTopicEnrichmentReviewByID(ctx, rv.ID)
	if unchanged.Applied {
		t.Fatal("review should remain applied=false on cross-topic access")
	}

	// Same-topic access → 200, applied.
	w2 := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/1/enrichment/reviews/%d/apply", rv.ID), "")
	var got repository.TopicEnrichmentReview
	expectJSONSuccess(t, w2, &got)
	if !got.Applied {
		t.Fatal("expected applied=true after same-topic apply")
	}
}

func TestSedimentQA_IDORProtection(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	// Result (and its QA) belong to topic 1.
	result := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), SessionID: "s"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed result: %v", err)
	}
	qa := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: result.ID, Question: "q", Answer: "a", Source: "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa); err != nil {
		t.Fatalf("seed qa: %v", err)
	}

	h := newTestHandler(db, nil, nil, nil)
	r := newTestRouter(h)

	// Topic 2 tries to sediment topic 1's qa → 404, flag unchanged.
	w := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/2/enrichment/qa/%d/sediment", qa.ID), "")
	expectJSONError(t, w, http.StatusNotFound)

	unchanged, _ := repository.Repo.GetTopicEnrichmentQAByID(ctx, qa.ID)
	if unchanged.Sedimented {
		t.Fatal("qa should remain sedimented=false on cross-topic access")
	}

	// Same-topic access → 200, sedimented.
	w2 := doRequest(t, r, "POST", fmt.Sprintf("/api/persistent-topics/1/enrichment/qa/%d/sediment", qa.ID), "")
	var got repository.TopicEnrichmentQA
	expectJSONSuccess(t, w2, &got)
	if !got.Sedimented {
		t.Fatal("expected sedimented=true after same-topic sediment")
	}
}

// ── Reference roles read-only compatibility (design D6) ────────────────────

func TestReferenceRoleRoutes(t *testing.T) {
	db := setupHandlerTestDB(t)
	h := newTestHandler(db, &mockLifelineService{}, &mockOrchestrator{}, &alwaysEnabledBoardConfig{})
	role := &repository.ReferenceRole{Name: "compat-role", Title: "旧画像", Content: "原文", Enabled: true}
	if err := repository.Repo.CreateReferenceRole(context.Background(), role); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	r := newTestRouter(h)

	if w := doRequest(t, r, http.MethodGet, "/api/reference-roles", ""); w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	if w := doRequest(t, r, http.MethodGet, fmt.Sprintf("/api/reference-roles/%d", role.ID), ""); w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/reference-roles", `{"name":"new","content":"x"}`},
		{http.MethodPut, fmt.Sprintf("/api/reference-roles/%d", role.ID), `{"enabled":false}`},
		{http.MethodDelete, fmt.Sprintf("/api/reference-roles/%d", role.ID), ""},
	} {
		if w := doRequest(t, r, tc.method, tc.path, tc.body); w.Code != http.StatusGone {
			t.Fatalf("%s %s: want 410, got %d", tc.method, tc.path, w.Code)
		}
	}
	unchanged, err := repository.Repo.GetReferenceRoleByID(context.Background(), role.ID)
	if err != nil || !unchanged.Enabled || unchanged.Content != "原文" {
		t.Fatalf("legacy write changed stored role: role=%+v err=%v", unchanged, err)
	}
}

// RunRelationDiscovery implements the relation-discovery arm of the mock
// (blocks on relationBlock when set, mirroring investBlock semantics).
func (m *mockOrchestrator) RunRelationDiscovery(ctx context.Context, in service.RelationDiscoveryInput) (*service.RelationDiscoveryOutput, error) {
	m.relationCalls++
	snapshot := in
	m.lastRelationIn = &snapshot
	m.relationInputs = append(m.relationInputs, in)
	if m.relationBlock != nil {
		<-m.relationBlock
	}
	if m.relationErr != nil {
		return nil, m.relationErr
	}
	if m.relationOut != nil {
		return m.relationOut, nil
	}
	return &service.RelationDiscoveryOutput{Status: "succeeded"}, nil
}

// ReResolveRelation implements the relation re-resolve arm of the mock.
func (m *mockOrchestrator) ReResolveRelation(ctx context.Context, boardID, relationID uint) (*service.RelationReResolveOutput, error) {
	m.reResolveCalls++
	m.lastReResolveID = relationID
	if m.reResolveErr != nil {
		return nil, m.reResolveErr
	}
	if m.reResolveOut != nil {
		return m.reResolveOut, nil
	}
	return &service.RelationReResolveOutput{RelationID: relationID, NewStatus: "unresolved", Outcome: "no_match"}, nil
}
