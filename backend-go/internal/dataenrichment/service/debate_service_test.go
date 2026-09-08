package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ── Test helpers ────────────────────────────────────────────────────────────

func setupDebateTestDB(t *testing.T) *repository.Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.DB = db
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewRepository(db)
	repository.SetRepo(repo)
	return repo
}

// ── Mock FinGeniusClient ────────────────────────────────────────────────────

type mockFinGeniusClient struct {
	submitFn   func(ctx context.Context, symbols []service.DebateSymbol) ([]service.DebateTask, error)
	getTaskFn  func(ctx context.Context, taskID string) (*service.DebateTaskResult, error)
	pollTaskFn func(ctx context.Context, taskID string) (*service.DebateTaskResult, error)
	healthFn   func(ctx context.Context) error

	pollCalls map[string]int
	mu        sync.Mutex
}

func newMockFinGeniusClient() *mockFinGeniusClient {
	return &mockFinGeniusClient{
		pollCalls: make(map[string]int),
	}
}

func (m *mockFinGeniusClient) Submit(ctx context.Context, symbols []service.DebateSymbol) ([]service.DebateTask, error) {
	if m.submitFn != nil {
		return m.submitFn(ctx, symbols)
	}
	tasks := make([]service.DebateTask, len(symbols))
	for i, s := range symbols {
		tasks[i] = service.DebateTask{
			TaskID:    "task-" + s.Code,
			StockCode: s.Code,
			Name:      s.Name,
			Sector:    s.Sector,
		}
	}
	return tasks, nil
}

func (m *mockFinGeniusClient) GetTask(ctx context.Context, taskID string) (*service.DebateTaskResult, error) {
	if m.getTaskFn != nil {
		return m.getTaskFn(ctx, taskID)
	}
	return m.defaultDoneResult(taskID), nil
}

func (m *mockFinGeniusClient) PollTask(ctx context.Context, taskID string) (*service.DebateTaskResult, error) {
	m.mu.Lock()
	m.pollCalls[taskID]++
	m.mu.Unlock()

	if m.pollTaskFn != nil {
		return m.pollTaskFn(ctx, taskID)
	}
	return m.defaultDoneResult(taskID), nil
}

func (m *mockFinGeniusClient) Health(ctx context.Context) error {
	if m.healthFn != nil {
		return m.healthFn(ctx)
	}
	return nil
}

func (m *mockFinGeniusClient) pollCallCount(taskID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pollCalls[taskID]
}

func (m *mockFinGeniusClient) defaultDoneResult(taskID string) *service.DebateTaskResult {
	return &service.DebateTaskResult{
		TaskID:    taskID,
		StockCode: "000001",
		Name:      "平安银行",
		Sector:    "金融",
		Status:    "done",
		Result: &service.DebateResultData{
			StockCode:   "000001",
			HTMLContent: "<html>辩论内容</html>",
			Name:        "平安银行",
			Sector:      "金融",
			Research: map[string]any{
				"sentiment_agent": "积极看多,资金持续流入",
				"risk_agent":      "风险可控,估值合理",
				"macro_agent":     "宏观向好,政策支持",
				"tech_agent":      "技术突破,平台突破",
				"fund_agent":      "机构增持,北向流入",
				"flow_agent":      "主力净流入,散户跟进",
			},
			Battle: map[string]any{
				"final_decision": "bullish",
				"final_votes": map[string]any{
					"sentiment_agent": "bullish",
					"risk_agent":      "bullish",
					"macro_agent":     "bullish",
					"tech_agent":      "bullish",
					"fund_agent":      "bearish",
					"flow_agent":      "bearish",
				},
			},
		},
	}
}

// ── Mock AirRouter for Distill ──────────────────────────────────────────────

type serviceMockAirouter struct {
	content string
	err     error
}

func (m *serviceMockAirouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &airouter.ChatResult{Content: m.content}, nil
}

// distillJSON builds a standard distill response.
func distillUpResponse() string {
	return `{"agents":[
		{"role":"sentiment_agent","stance":"up","note":"情绪积极","raw_vote":"bullish"},
		{"role":"risk_agent","stance":"up","note":"风险可控","raw_vote":"bullish"},
		{"role":"macro_agent","stance":"up","note":"宏观向好","raw_vote":"bullish"},
		{"role":"tech_agent","stance":"up","note":"技术突破","raw_vote":"bullish"},
		{"role":"fund_agent","stance":"down","note":"机构分歧","raw_vote":"bearish"},
		{"role":"flow_agent","stance":"down","note":"流出压力","raw_vote":"bearish"}
	],"verdict":"up","consensus":"4/6","votes":{"up":4,"flat":0,"down":2}}`
}

// ── RunDebate: normal path (2 symbols both done) ───────────────────────────

func TestRunDebate_Success(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))
	client := newMockFinGeniusClient()

	svc := service.NewDebateService(client, distiller, repo)

	results, err := svc.RunDebate(context.Background(), 1, 1, "sess-run", []service.DebateSymbol{
		{Code: "000001", Name: "平安银行", Sector: "金融"},
		{Code: "600519", Name: "贵州茅台", Sector: "消费"},
	})
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("results count: want 2, got %d", len(results))
	}

	for i, dr := range results {
		if dr.DistillStatus != "done" {
			t.Fatalf("result[%d].distill_status: want done, got %s", i, dr.DistillStatus)
		}
		if dr.Verdict != "up" {
			t.Fatalf("result[%d].verdict: want up, got %s", i, dr.Verdict)
		}
		if dr.Consensus != "4/6" {
			t.Fatalf("result[%d].consensus: want 4/6, got %s", i, dr.Consensus)
		}
		if dr.Agents == nil {
			t.Fatalf("result[%d].agents should not be nil", i)
		}
		if dr.Votes == nil {
			t.Fatalf("result[%d].votes should not be nil", i)
		}
		if dr.HTMLContent == "" {
			t.Fatalf("result[%d].html_content should not be empty", i)
		}
		if dr.FingeniusResearch == nil {
			t.Fatalf("result[%d].fingenius_research should not be nil", i)
		}
		if dr.FingeniusBattle == nil {
			t.Fatalf("result[%d].fingenius_battle should not be nil", i)
		}
		if dr.FingeniusTaskID == "" {
			t.Fatalf("result[%d].task_id should not be empty", i)
		}
	}

	// Verify persisted in DB.
	stored, err := repo.ListStockDebateResultsByResult(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListStockDebateResultsByResult: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored count: want 2, got %d", len(stored))
	}
}

// ── RunDebate: PollTask error → distill_status=failed, raw data saved ──────

func TestRunDebate_PollTaskError(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))

	client := newMockFinGeniusClient()
	// PollTask returns an error but also a result (simulating partial data).
	client.pollTaskFn = func(ctx context.Context, taskID string) (*service.DebateTaskResult, error) {
		return &service.DebateTaskResult{
			TaskID:    taskID,
			Status:    "running",
			StockCode: "000001",
			Name:      "平安银行",
			Sector:    "金融",
			Result: &service.DebateResultData{
				HTMLContent: "<html>partial</html>",
				Research:    map[string]any{"sentiment_agent": "分析中"},
				Battle:      map[string]any{"final_decision": "pending"},
			},
		}, fmt.Errorf("poll timed out")
	}

	svc := service.NewDebateService(client, distiller, repo)
	results, err := svc.RunDebate(context.Background(), 1, 1, "sess-poll-err", []service.DebateSymbol{
		{Code: "000001", Name: "平安银行", Sector: "金融"},
	})
	if err != nil {
		t.Fatalf("RunDebate should not fail overall on individual poll error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("results: want 1, got %d", len(results))
	}

	dr := results[0]
	if dr.DistillStatus != "failed" {
		t.Fatalf("distill_status: want failed, got %s", dr.DistillStatus)
	}
	if dr.Verdict != "flat" {
		t.Fatalf("verdict: want flat (fallback), got %s", dr.Verdict)
	}
	if dr.HTMLContent != "<html>partial</html>" {
		t.Fatalf("html_content should be saved despite poll error")
	}
	if dr.FingeniusResearch == nil {
		t.Fatal("fingenius_research should be saved despite poll error")
	}
	if dr.FingeniusBattle == nil {
		t.Fatal("fingenius_battle should be saved despite poll error")
	}
	// Agents/votes should be nil (distill never ran)
	if dr.Agents != nil {
		t.Fatal("agents should be nil when distill never ran")
	}
	if dr.Votes != nil {
		t.Fatal("votes should be nil when distill never ran")
	}
}

// ── RunDebate: Distill error → distill_status=failed, raw data preserved ───

func TestRunDebate_DistillError(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{err: fmt.Errorf("LLM timeout")}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))
	client := newMockFinGeniusClient()

	svc := service.NewDebateService(client, distiller, repo)
	results, err := svc.RunDebate(context.Background(), 1, 1, "sess-distill-err", []service.DebateSymbol{
		{Code: "000001", Name: "平安银行", Sector: "金融"},
	})
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}

	dr := results[0]
	if dr.DistillStatus != "failed" {
		t.Fatalf("distill_status: want failed, got %s", dr.DistillStatus)
	}
	if dr.Verdict != "flat" {
		t.Fatalf("verdict: want flat (fallback), got %s", dr.Verdict)
	}
	// Raw data preserved.
	if dr.HTMLContent == "" {
		t.Fatal("html_content should be preserved")
	}
	if dr.FingeniusResearch == nil {
		t.Fatal("fingenius_research should be preserved")
	}
	// Agents/votes should be nil (distill failed).
	if dr.Agents != nil {
		t.Fatal("agents should be nil when distill fails")
	}
	if dr.Votes != nil {
		t.Fatal("votes should be nil when distill fails")
	}
}

// ── RunDebate: Submit failure → error ───────────────────────────────────────

func TestRunDebate_SubmitError(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))

	client := newMockFinGeniusClient()
	client.submitFn = func(ctx context.Context, symbols []service.DebateSymbol) ([]service.DebateTask, error) {
		return nil, fmt.Errorf("finGenius service unavailable")
	}

	svc := service.NewDebateService(client, distiller, repo)
	_, err := svc.RunDebate(context.Background(), 1, 1, "sess-submit-err", []service.DebateSymbol{
		{Code: "000001", Name: "平安银行", Sector: "金融"},
	})
	if err == nil {
		t.Fatal("RunDebate should return error when Submit fails")
	}
}

// ── RunDebate: empty symbols → error ────────────────────────────────────────

func TestRunDebate_EmptySymbols(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))
	client := newMockFinGeniusClient()

	svc := service.NewDebateService(client, distiller, repo)
	_, err := svc.RunDebate(context.Background(), 1, 1, "sess-empty", nil)
	if err == nil {
		t.Fatal("RunDebate should return error on empty symbols")
	}
}

// ── RunDebate: concurrency check ────────────────────────────────────────────

func TestRunDebate_Concurrency(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))

	client := newMockFinGeniusClient()

	// Track if PollTask calls overlap (concurrency check via time stamps).
	var callTimesMu sync.Mutex
	callTimes := make([]time.Time, 0)
	client.pollTaskFn = func(ctx context.Context, taskID string) (*service.DebateTaskResult, error) {
		callTimesMu.Lock()
		callTimes = append(callTimes, time.Now())
		n := len(callTimes)
		callTimesMu.Unlock()

		// Simulate a short delay so concurrent goroutines overlap on later tasks.
		time.Sleep(30 * time.Millisecond)

		if n == 1 {
			// First task returns done. Second and third will be running.
		}
		return &service.DebateTaskResult{
			TaskID: taskID,
			Status: "done",
			Result: &service.DebateResultData{
				Research:    map[string]any{"a": "b"},
				Battle:      map[string]any{"c": "d"},
				HTMLContent: "<html>x</html>",
			},
		}, nil
	}

	svc := service.NewDebateService(client, distiller, repo)

	start := time.Now()
	results, err := svc.RunDebate(context.Background(), 1, 1, "sess-concur", []service.DebateSymbol{
		{Code: "000001", Name: "A", Sector: "金融"},
		{Code: "000002", Name: "B", Sector: "科技"},
		{Code: "000003", Name: "C", Sector: "消费"},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results: want 3, got %d", len(results))
	}

	// Serial execution would take ~3*30ms = 90ms+.
	// Concurrent should finish well under 80ms (some overhead).
	if elapsed > 80*time.Millisecond {
		t.Fatalf("RunDebate took %v — expected concurrent (<80ms) but looks serial", elapsed)
	}

	// Verify all tasks were polled.
	for i, tc := range []string{"task-000001", "task-000002", "task-000003"} {
		if client.pollCallCount(tc) == 0 {
			t.Fatalf("task[%d] %s was not polled", i, tc)
		}
	}
}

// ── RunDebate: one failed, one succeeded in same batch ─────────────────────

func TestRunDebate_MixedResults(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))

	client := newMockFinGeniusClient()
	client.pollTaskFn = func(ctx context.Context, taskID string) (*service.DebateTaskResult, error) {
		if taskID == "task-000002" {
			// Second symbol: poll error.
			return nil, fmt.Errorf("network error")
		}
		return &service.DebateTaskResult{
			TaskID: taskID,
			Status: "done",
			Result: &service.DebateResultData{
				HTMLContent: "<html>ok</html>",
				Research:    map[string]any{"a": "b"},
				Battle:      map[string]any{"c": "d"},
			},
		}, nil
	}

	svc := service.NewDebateService(client, distiller, repo)
	results, err := svc.RunDebate(context.Background(), 1, 1, "sess-mixed", []service.DebateSymbol{
		{Code: "000001", Name: "成功", Sector: "金融"},
		{Code: "000002", Name: "失败", Sector: "消费"},
	})
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("results: want 2, got %d", len(results))
	}

	// First: succeeded.
	if results[0].DistillStatus != "done" {
		t.Fatalf("result[0].distill_status: want done, got %s", results[0].DistillStatus)
	}
	if results[0].Verdict != "up" {
		t.Fatalf("result[0].verdict: want up, got %s", results[0].Verdict)
	}

	// Second: failed.
	if results[1].DistillStatus != "failed" {
		t.Fatalf("result[1].distill_status: want failed, got %s", results[1].DistillStatus)
	}
	if results[1].Verdict != "flat" {
		t.Fatalf("result[1].verdict: want flat, got %s", results[1].Verdict)
	}
}

// ── processOneTask: task failed status → failed ─────────────────────────────

func TestRunDebate_TaskFailedStatus(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))

	client := newMockFinGeniusClient()
	client.pollTaskFn = func(ctx context.Context, taskID string) (*service.DebateTaskResult, error) {
		return &service.DebateTaskResult{
			TaskID:    taskID,
			Status:    "failed",
			StockCode: "000001",
			Name:      "失败股",
			Sector:    "金融",
			Error:     "analysis pipeline crashed",
			Result: &service.DebateResultData{
				HTMLContent: "<html>crash</html>",
				Research:    map[string]any{"partial": "data"},
				Battle:      map[string]any{"partial": "vote"},
			},
		}, fmt.Errorf("task failed: analysis pipeline crashed")
	}

	svc := service.NewDebateService(client, distiller, repo)
	results, err := svc.RunDebate(context.Background(), 1, 1, "sess-failed-status", []service.DebateSymbol{
		{Code: "000001", Name: "失败股", Sector: "金融"},
	})
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}

	dr := results[0]
	if dr.DistillStatus != "failed" {
		t.Fatalf("distill_status: want failed, got %s", dr.DistillStatus)
	}
	if dr.Verdict != "flat" {
		t.Fatalf("verdict: want flat, got %s", dr.Verdict)
	}
	if dr.HTMLContent != "<html>crash</html>" {
		t.Fatalf("html_content should be saved even on task failure")
	}
	if dr.FingeniusResearch == nil {
		t.Fatal("fingenius_research should be saved")
	}
	if dr.FingeniusBattle == nil {
		t.Fatal("fingenius_battle should be saved")
	}
}

// ── DB integrity: verify all fields persisted correctly ────────────────────

func TestRunDebate_DBIntegrity(t *testing.T) {
	repo := setupDebateTestDB(t)
	ar := &serviceMockAirouter{content: distillUpResponse()}
	distiller := service.NewDebateDistiller(ar, airouter.Capability("data_enrichment.debate_distill"))
	client := newMockFinGeniusClient()

	svc := service.NewDebateService(client, distiller, repo)
	results, err := svc.RunDebate(context.Background(), 42, 77, "sess-db-int", []service.DebateSymbol{
		{Code: "000001", Name: "平安银行", Sector: "金融"},
	})
	if err != nil {
		t.Fatalf("RunDebate: %v", err)
	}

	dr := results[0]

	// Verify result IDs.
	if dr.TopicEnrichmentResultID != 42 {
		t.Fatalf("result_id: want 42, got %d", dr.TopicEnrichmentResultID)
	}
	if dr.PersistentTopicID != 77 {
		t.Fatalf("topic_id: want 77, got %d", dr.PersistentTopicID)
	}
	if dr.Sector != "金融" {
		t.Fatalf("sector: want 金融, got %s", dr.Sector)
	}
	if dr.Code != "000001" {
		t.Fatalf("code: want 000001, got %s", dr.Code)
	}
	if dr.Name != "平安银行" {
		t.Fatalf("name: want 平安银行, got %s", dr.Name)
	}
	if dr.FingeniusTaskID != "task-000001" {
		t.Fatalf("task_id: want task-000001, got %s", dr.FingeniusTaskID)
	}

	// Verify JSON fields are valid.
	var agents []map[string]any
	if err := json.Unmarshal(dr.Agents, &agents); err != nil {
		t.Fatalf("agents JSON parse: %v", err)
	}
	if len(agents) != 6 {
		t.Fatalf("agents count: want 6, got %d", len(agents))
	}

	var votes map[string]any
	if err := json.Unmarshal(dr.Votes, &votes); err != nil {
		t.Fatalf("votes JSON parse: %v", err)
	}
	if up, ok := votes["up"].(float64); !ok || int(up) != 4 {
		t.Fatalf("votes.up: want 4")
	}

	// Verify stored in DB.
	stored, err := repo.ListStockDebateResultsByResult(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListStockDebateResultsByResult: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored count: want 1, got %d", len(stored))
	}
	if stored[0].DistillStatus != "done" {
		t.Fatalf("stored distill_status: want done, got %s", stored[0].DistillStatus)
	}
	if stored[0].ID == 0 {
		t.Fatal("stored ID should be >0")
	}
}
