package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var testCap = airouter.Capability("data_enrichment_analysis")

// setupOrchDBSeq uniquifies shared-cache memory DB names across -count re-runs.
var setupOrchDBSeq int64

// ── Test helpers ────────────────────────────────────────────────────────────

func setupOrchTestDB(t *testing.T) *repository.Repository {
	t.Helper()
	// Unique suffix: shared-cache memory DBs keyed only by t.Name() collide
	// across -count>1 re-runs (same name → same DB → UNIQUE constraint on
	// re-seeded fixture rows).
	setupOrchDBSeq++
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), setupOrchDBSeq)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.DB = db
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repo := repository.NewRepository(db)
	repository.SetRepo(repo)
	return repo
}

// mockAirRouter records calls and returns canned ChatResults in order.
type mockAirRouter struct {
	responses []*airouter.ChatResult
	callCount int
	Calls     []airouter.ChatRequest
}

func newMockAirRouter() *mockAirRouter {
	return &mockAirRouter{}
}

func (m *mockAirRouter) addResponse(content string) {
	m.responses = append(m.responses, &airouter.ChatResult{Content: content})
}

func (m *mockAirRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	m.Calls = append(m.Calls, req)
	if m.callCount >= len(m.responses) {
		return &airouter.ChatResult{Content: "{}"}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

// mockBoardConfigReader returns canned board config.
type mockBoardConfigReader struct {
	cfg *service.BoardEnrichmentConfig
	err error
}

func (m *mockBoardConfigReader) GetBoardConfig(ctx context.Context, topicID uint) (*service.BoardEnrichmentConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.cfg, nil
}

func newEnabledConfig() *service.BoardEnrichmentConfig {
	return &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        14,
		ContextLayers:     []string{"week", "month", "year", "all"},
		AllowedTools:      []string{"list_etf_by_keyword", "get_etf_quote", "list_sectors"},
	}
}

// orchMockLifelineReader returns canned lifeline data for orchestrator tests.
// (Avoids collision with mockLifelineReader in lifeline_renderer_test.go.)
type orchMockLifelineReader struct {
	data service.SectionTimelineData
	err  error
}

func (m *orchMockLifelineReader) GetTopicLifeline(topicID uint) (service.SectionTimelineData, error) {
	if m.err != nil {
		return service.SectionTimelineData{}, m.err
	}
	return m.data, nil
}

func (m *orchMockLifelineReader) GetTopicLifelineArchive(topicID uint) ([]service.LifelineArchiveRow, error) {
	return nil, nil
}

func mustParseTime(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// nilFetcher returns empty ETF data to avoid real HTTP calls in tests.
type nilFetcher struct{}

func (n *nilFetcher) Fetch(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	// Return empty ETF list so list_etf_by_keyword returns 0 hits.
	return []byte(`{"data":{"diff":[]}}`), nil
}

// ── canned response builders (new schema) ───────────────────────────────────
//
// EnrichTopic LLM call order: interpret → lens(propose) → tool_use(per topic)
// → analyze → [review_judge if prev result has form]. These helpers produce the
// interpret/lens responses so each test stays readable.

// interpretResp builds an interpret-phase response (form + research topics).
func interpretResp(form, formReason string, topics ...[2]string) string {
	ts := make([]string, 0, len(topics))
	for _, t := range topics {
		ts = append(ts, fmt.Sprintf(`{"topic":%q,"reason":%q}`, t[0], t[1]))
	}
	return fmt.Sprintf(`{"form":%q,"form_reason":%q,"topics":[%s]}`, form, formReason, strings.Join(ts, ","))
}

// lensResp builds a lens-source response with >=2 concrete candidates.
func lensResp(candidates ...[2]string) string {
	cs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		cs = append(cs, fmt.Sprintf(`{"name":%q,"description":%q}`, c[0], c[1]))
	}
	return fmt.Sprintf(`{"lens_candidates":[%s]}`, strings.Join(cs, ","))
}

// defaultInterpretLens adds the standard interpret + lens pair (event_chain, 1 topic).
func defaultInterpretLens(ar *mockAirRouter) {
	ar.addResponse(interpretResp("event_chain", "线性因果", [2]string{"石油", "油价波动"}))
	ar.addResponse(lensResp(
		[2]string{"油价这轮上涨能不能持续", "供需与地缘"},
		[2]string{"产油国博弈谁占上风", "OPEC+博弈"},
	))
}

// defaultAnalyzeEventChain adds a minimal event_chain analyze response
// (including the required depth block — non-sparse forms must carry depth).
func defaultAnalyzeEventChain(ar *mockAirRouter) {
	ar.addResponse(`{
		"form": "event_chain",
		"lens": "油价这轮上涨能不能持续",
		"analysis": {
			"fact_layer": [{"claim": "产油国设施遭袭", "evidence": [{"source_type":"news","ref":"ctx1","quote":"设施遭袭"}], "verified": true}],
			"timeline": [{"date": "2026-07-01", "event": "遭袭", "ref": {"source_type":"news","ref":"ctx1"}}],
			"insight_layer": [{"cert": "medium", "title": "油价短期承压", "logic": "供应收紧", "evidence": [{"source_type":"news","ref":"ctx1","quote":"油价飙升"}]}],
			"depth": {
				"system_reframe": "放进全球能源供应链系统看",
				"mechanism_layers": [{"layer":"供给冲击","deep_logic":"产能骤降→供需缺口","basis":"遭袭报道"}],
				"historical_analogy": [{"case":"2019沙特油田遭袭","mechanism":"短期供应中断→油价跳涨","diff":"本次波及范围待证实"}],
				"regime_shift": null,
				"boundary": "目前还不能确认补产时序，不宜外推长期趋势",
				"evidence_chain": [{"source_type":"news","ref":"ctx1","quote":"设施遭袭","date":"2026-07-01"}]
			}
		}
	}`)
}

// ── parseJSONResponse tests ─────────────────────────────────────────────────

func TestParseJSONResponse_CleanJSON(t *testing.T) {
	input := `{"action":"call_tool","tool":"list_etf_by_keyword","args":{"keyword":"半导体"}}`
	result, err := service.ParseJSONResponse(input)
	if err != nil {
		t.Fatalf("clean JSON should parse: %v", err)
	}
	if result["action"] != "call_tool" {
		t.Fatalf("action: want call_tool, got %v", result["action"])
	}
	if result["tool"] != "list_etf_by_keyword" {
		t.Fatalf("tool: want list_etf_by_keyword, got %v", result["tool"])
	}
}

func TestParseJSONResponse_MarkdownCodeBlock(t *testing.T) {
	input := "```json\n{\"action\":\"finish\",\"summary\":\"done\"}\n```"
	result, err := service.ParseJSONResponse(input)
	if err != nil {
		t.Fatalf("markdown-wrapped JSON should parse: %v", err)
	}
	if result["action"] != "finish" {
		t.Fatalf("action: want finish, got %v", result["action"])
	}
}

func TestParseJSONResponse_PlainFence(t *testing.T) {
	input := "```\n{\"topics\":[{\"topic\":\"石油\"}]}\n```"
	result, err := service.ParseJSONResponse(input)
	if err != nil {
		t.Fatalf("plain-fence JSON should parse: %v", err)
	}
	topics, ok := result["topics"].([]any)
	if !ok || len(topics) == 0 {
		t.Fatal("topics should be present")
	}
}

func TestParseJSONResponse_EmbeddedBrackets(t *testing.T) {
	input := "sure, here's the json:\n{\"action\":\"call_tool\",\"args\":{\"keyword\":\"芯片\"}} and some extra text"
	result, err := service.ParseJSONResponse(input)
	if err != nil {
		t.Fatalf("embedded JSON should parse: %v", err)
	}
	if result["action"] != "call_tool" {
		t.Fatalf("action: want call_tool, got %v", result["action"])
	}
}

func TestParseJSONResponse_PlainTextFail(t *testing.T) {
	input := "这是纯文本，没有JSON"
	_, err := service.ParseJSONResponse(input)
	if err == nil {
		t.Fatal("plain text without JSON should fail")
	}
}

func TestParseJSONResponse_EmptyFail(t *testing.T) {
	_, err := service.ParseJSONResponse("")
	if err == nil {
		t.Fatal("empty string should fail")
	}
}

// ── BoardConfig tests ───────────────────────────────────────────────────────

func TestDefaultBoardConfig_DisabledByDefault(t *testing.T) {
	cfg := service.DefaultBoardConfig()
	if cfg.EnrichmentEnabled {
		t.Fatal("default should have enrichment_enabled=false")
	}
	if cfg.WindowDays != 14 {
		t.Fatalf("default window_days: want 14, got %d", cfg.WindowDays)
	}
}

func TestBoardEnrichmentConfig_Validate(t *testing.T) {
	cfg := &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        0,
		ContextLayers:     []string{"week", "month"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("window_days=0 should fail validation")
	}
}

// ── EnrichTopic: enrichment_enabled guard ───────────────────────────────────

func TestEnrichTopic_DisabledBoardRejected(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()
	toolRegistry := service.NewRegistry(&nilFetcher{})

	disabledCfg := service.DefaultBoardConfig() // EnrichmentEnabled=false
	boardReader := &mockBoardConfigReader{cfg: disabledCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试话题"},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	_, err := orch.EnrichTopic(context.Background(), 1)
	if err == nil {
		t.Fatal("disabled board should reject EnrichTopic")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("error should mention 'not enabled', got: %v", err)
	}
}

// ── EnrichTopic: end-to-end test with mock LLM ──────────────────────────────

func TestEnrichTopicE2E_FullFlow(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	// Sequence: interpret → lens → tool_use(finish) → analyze (no review, first run)
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","thought":"done","summary":"油价涨3%"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID:              1,
				Label:           "中东地缘紧张与能源连锁反应",
				Description:     "产油国设施遭袭 → 油价飙升",
				Status:          "active",
				FirstSeenDate:   mustParseTime("2026-07-01"),
				LastSeenDate:    mustParseTime("2026-07-05"),
				HitCount:        5,
				ConsecutiveHits: 5,
			},
			Sections: []service.TimelineSectionNode{
				{
					SectionID: 101, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "产油国遭袭", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 5, ThreadCount: 2,
					ThreadTitles: []string{"布伦特原油飙升", "中东局势升级"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	// Verify result
	result := output.Result
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if repository.TopicIDMatches(result.PersistentTopicID, 1) == false {
		t.Fatalf("topic ID: want 1, got %d", result.PersistentTopicID)
	}
	// EvolutionAssessment is now vestigial (empty) — new schema lives in Sectors.
	if result.EvolutionAssessment != "" {
		t.Fatalf("evolution_assessment should be empty (vestigial), got: %q", result.EvolutionAssessment)
	}
	if result.SessionID == "" {
		t.Fatal("session_id should not be empty")
	}
	if !strings.HasPrefix(result.SessionID, "data_enrichment_1_") {
		t.Fatalf("session_id should start with data_enrichment_1_, got: %s", result.SessionID)
	}

	// Verify sectors JSON now stores {form, lens, analysis}
	var sectorsJSON map[string]any
	if err := json.Unmarshal(result.Sectors, &sectorsJSON); err != nil {
		t.Fatalf("sectors should be valid JSON: %v", err)
	}
	if sectorsJSON["form"] != "event_chain" {
		t.Fatalf("sectors.form: want event_chain, got %v", sectorsJSON["form"])
	}
	if sectorsJSON["lens"] == nil {
		t.Fatal("sectors JSON should have lens")
	}
	analysis, ok := sectorsJSON["analysis"].(map[string]any)
	if !ok {
		t.Fatal("sectors JSON should have analysis object")
	}
	if analysis["fact_layer"] == nil {
		t.Fatal("analysis should have fact_layer")
	}
	if analysis["insight_layer"] == nil {
		t.Fatal("analysis should have insight_layer")
	}

	// Verify tool_calls JSON
	var toolCalls []map[string]any
	if err := json.Unmarshal(result.ToolCalls, &toolCalls); err != nil {
		t.Fatalf("tool_calls should be valid JSON: %v", err)
	}

	// Verify input_snapshot JSON
	var inputSnap map[string]any
	if err := json.Unmarshal(result.InputSnapshot, &inputSnap); err != nil {
		t.Fatalf("input_snapshot should be valid JSON: %v", err)
	}
	if _, ok := inputSnap["context_layers"]; !ok {
		t.Fatal("input_snapshot should have context_layers")
	}
	if _, ok := inputSnap["window_days"]; !ok {
		t.Fatal("input_snapshot should have window_days")
	}
	if _, ok := inputSnap["config_context_layers"]; !ok {
		t.Fatal("input_snapshot should have config_context_layers")
	}
	if _, ok := inputSnap["review_ids"]; !ok {
		t.Fatal("input_snapshot should have review_ids")
	}

	// First run: no prev result → no review
	if output.Review != nil {
		t.Fatal("first run should have no review (no prev result)")
	}

	// Verify airouter calls: interpret, lens(interpret op), tool_use, analyze.
	if len(airouter.Calls) < 4 {
		t.Fatalf("expected at least 4 airouter calls, got %d", len(airouter.Calls))
	}

	// Check Capability on all calls
	for i, call := range airouter.Calls {
		if string(call.Capability) != "data_enrichment_analysis" {
			t.Fatalf("call %d: Capability = %s, want data_enrichment_analysis", i, call.Capability)
		}
	}

	// Call order: Calls[0]=interpret, Calls[1]=lens(propose), Calls[2]=tool_use, Calls[3]=analyze.
	if airouter.Calls[0].Operation != "data_enrichment.interpret" {
		t.Fatalf("call 0: want interpret, got %s", airouter.Calls[0].Operation)
	}
	if airouter.Calls[1].Operation != "data_enrichment.interpret" {
		t.Fatalf("call 1 (lens): want interpret op, got %s", airouter.Calls[1].Operation)
	}
	if airouter.Calls[2].Operation != "data_enrichment.tool_use" {
		t.Fatalf("call 2: want tool_use, got %s", airouter.Calls[2].Operation)
	}
	if airouter.Calls[3].Operation != "data_enrichment.analyze" {
		t.Fatalf("call 3: want analyze, got %s", airouter.Calls[3].Operation)
	}

	// Check SessionID on all calls
	sid := result.SessionID
	for i, call := range airouter.Calls {
		if call.SessionID != sid {
			t.Fatalf("call %d: SessionID = %s, want %s", i, call.SessionID, sid)
		}
	}

	// Verify agent loop result
	if len(output.AgentLoops) != 1 {
		t.Fatalf("expected 1 agent loop result, got %d", len(output.AgentLoops))
	}
	if output.AgentLoops[0].FinalData != "油价涨3%" {
		t.Fatalf("agent loop final_data: want 油价涨3%%, got %s", output.AgentLoops[0].FinalData)
	}
}

// ── EnrichTopic: review judge test (new findings / overturned) ──────────────

func TestEnrichTopic_ReviewJudgeOnSecondRun(t *testing.T) {
	repo := setupOrchTestDB(t)

	// Pre-populate a previous result (new-format sectors with form field) to
	// trigger review judge.
	prevResult := &repository.TopicEnrichmentResult{
		PersistentTopicID:   repository.TopicIDPtr(1),
		EvolutionAssessment: "",
		Sectors:             json.RawMessage(`{"form":"event_chain","lens":"L","analysis":{"insight_layer":[{"title":"油价将回落","cert":"medium"}]}}`),
		SessionID:           "data_enrichment_1_prev0001",
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), prevResult); err != nil {
		t.Fatalf("create prev result: %v", err)
	}

	airouter := newMockAirRouter()
	// interpret → lens → tool_use → analyze → review_judge
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"油价涨4.2%"}`)
	defaultAnalyzeEventChain(airouter)
	// review_judge: new schema with new_findings/overturned/confidence_shift.
	airouter.addResponse(`{
		"should_review": true,
		"reason": "出现新见解并推翻旧判断",
		"new_findings": ["供应收紧或持续整季"],
		"overturned": ["油价将回落"],
		"confidence_shift": [{"insight": "油价短期承压", "from": "medium", "to": "high"}],
		"affected_context": "week",
		"confidence": 0.8
	}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "中东地缘"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 201, PeriodDate: mustParseTime("2026-07-05"),
					ClusterLabel: "局势升级", Status: "continuing",
					TopicMatchConfidence: "anchor_hit", ArticleCount: 8, ThreadCount: 3,
					ThreadTitles: []string{"以军威胁", "海峡争议"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	// Should have a review
	if output.Review == nil {
		t.Fatal("second run should produce a review")
	}
	if output.Review.Applied {
		t.Fatal("new review should have applied=false by default")
	}
	if output.Review.Source != "llm_assisted" {
		t.Fatalf("review source: want llm_assisted, got %s", output.Review.Source)
	}
	if output.Review.DeviationSummary == "" {
		t.Fatal("review deviation_summary (reason) should not be empty")
	}
	if output.Review.PrevResultID == nil || *output.Review.PrevResultID != prevResult.ID {
		t.Fatalf("review prev_result_id should match prev result")
	}

	// Verify Verdict stores {new_findings, overturned, confidence_shift}.
	if output.Review.Verdict == nil {
		t.Fatal("review verdict should store new_findings/overturned/confidence_shift")
	}
	var verdict map[string]any
	if err := json.Unmarshal(output.Review.Verdict, &verdict); err != nil {
		t.Fatalf("review verdict unmarshal: %v", err)
	}
	nf, _ := verdict["new_findings"].([]any)
	if len(nf) != 1 || nf[0] != "供应收紧或持续整季" {
		t.Fatalf("new_findings: got %v", nf)
	}
	ov, _ := verdict["overturned"].([]any)
	if len(ov) != 1 || ov[0] != "油价将回落" {
		t.Fatalf("overturned: got %v", ov)
	}
	cs, _ := verdict["confidence_shift"].([]any)
	if len(cs) != 1 {
		t.Fatalf("confidence_shift: got %v", cs)
	}

	// Verify airouter calls include review_judge
	foundReviewJudge := false
	for _, call := range airouter.Calls {
		if call.Operation == "data_enrichment.review_judge" {
			foundReviewJudge = true
			break
		}
	}
	if !foundReviewJudge {
		t.Fatal("expected review_judge airouter call on second run")
	}
}

// ── EnrichTopic: review judge skipped on should_review=false ────────────────

func TestEnrichTopic_ReviewJudgeFalseSkips(t *testing.T) {
	repo := setupOrchTestDB(t)

	prevResult := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(2),
		Sectors:           json.RawMessage(`{"form":"event_chain","lens":"L","analysis":{}}`),
		SessionID:         "data_enrichment_2_prev0001",
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), prevResult); err != nil {
		t.Fatalf("create prev result: %v", err)
	}

	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"油价微涨0.5%"}`)
	defaultAnalyzeEventChain(airouter)
	// review_judge: should_review=false → should NOT write review
	airouter.addResponse(`{"should_review":false,"reason":"见解层无实质变化","new_findings":[],"overturned":[],"confidence_shift":[],"affected_context":"","confidence":0.3}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 2, Label: "中东地缘"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 301, PeriodDate: mustParseTime("2026-07-05"),
					ClusterLabel: "稳定", Status: "continuing",
					TopicMatchConfidence: "anchor_hit", ArticleCount: 3, ThreadCount: 1,
					ThreadTitles: []string{"油价微涨"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 2)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	if output.Review != nil {
		t.Fatal("should_review=false should not produce a review")
	}
}

// ── EnrichTopic: agent loop /no_think prefix ────────────────────────────────

func TestEnrichTopic_AgentLoopNoThinkPrefix(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic:    service.TopicBrief{ID: 1, Label: "测试"},
			Sections: []service.TimelineSectionNode{},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	if _, err := orch.EnrichTopic(context.Background(), 1); err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	// Find the tool_use call and verify /no_think prefix in user message.
	for _, call := range airouter.Calls {
		if call.Operation != "data_enrichment.tool_use" {
			continue
		}
		for _, msg := range call.Messages {
			if msg.Role == "user" {
				if !strings.Contains(msg.Content, "/no_think") {
					t.Fatal("tool_use user message should contain /no_think prefix")
				}
				return
			}
		}
	}
	t.Fatal("no tool_use call found in airouter calls")
}

// ── EnrichTopic: input_snapshot fields ──────────────────────────────────────

func TestEnrichTopic_InputSnapshotFields(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        14,
		ContextLayers:     []string{"week", "month"},
	}
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试话题"},
			Sections: []service.TimelineSectionNode{
				{
					SectionID: 401, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "测试", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"测试线索"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	var snap map[string]any
	if err := json.Unmarshal(output.Result.InputSnapshot, &snap); err != nil {
		t.Fatalf("input_snapshot parse: %v", err)
	}

	configLayers, ok := snap["config_context_layers"].([]any)
	if !ok {
		t.Fatal("input_snapshot should have config_context_layers")
	}
	if len(configLayers) != 2 {
		t.Fatalf("config_context_layers: want 2, got %d", len(configLayers))
	}

	if wd, ok := snap["window_days"].(float64); !ok || int(wd) != 14 {
		t.Fatalf("window_days: want 14")
	}

	reviewIDs, ok := snap["review_ids"].([]any)
	if !ok {
		t.Fatal("input_snapshot should have review_ids")
	}
	if len(reviewIDs) != 0 {
		t.Fatalf("review_ids: want 0 (no prev review), got %d", len(reviewIDs))
	}
}

// ── EnrichTopic: review does NOT write to topic_lifeline_context ────────────

func TestEnrichTopic_ReviewDoesNotWriteToTable1(t *testing.T) {
	repo := setupOrchTestDB(t)

	prevResult := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1),
		Sectors:           json.RawMessage(`{"form":"event_chain","lens":"L","analysis":{}}`),
		SessionID:         "prev",
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), prevResult); err != nil {
		t.Fatalf("create prev result: %v", err)
	}

	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)
	airouter.addResponse(`{"should_review":true,"reason":"变了","new_findings":["x"],"overturned":[],"confidence_shift":[],"affected_context":"week","confidence":0.8}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试"},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	existingCtxs, _ := repo.ListTopicLifelineContextsByTopic(context.Background(), 1)
	beforeCount := len(existingCtxs)

	if _, err := orch.EnrichTopic(context.Background(), 1); err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	// Count contexts after — should be unchanged (no write to table1).
	afterCtxs, _ := repo.ListTopicLifelineContextsByTopic(context.Background(), 1)
	if len(afterCtxs) != beforeCount {
		t.Fatalf("review write should not modify topic_lifeline_context: before=%d, after=%d", beforeCount, len(afterCtxs))
	}
}

// ── randomHex ──────────────────────────────────────────────────────────────

func TestRandomHex_NonEmptyCorrectLength(t *testing.T) {
	for _, n := range []int{0, 1, 8, 16, 32} {
		got := service.RandomHex(n)
		if len(got) != n {
			t.Fatalf("randomHex(%d): len=%d, want %d", n, len(got), n)
		}
		// All-zeros only meaningful for longer strings: each char is one of 16
		// hex digits, so n=1 has a legitimate 1/16 chance of "0" (pre-existing
		// flake). At n>=8 the odds are ~4e-10 — effectively impossible.
		if n >= 8 && got == strings.Repeat("0", n) {
			t.Fatalf("randomHex(%d): got all zeros (extremely unlikely)", n)
		}
	}
}

func TestRandomHex_PanicsOnError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("randomHex panicked: %v", r)
		}
	}()
	got := service.RandomHex(8)
	if len(got) != 8 {
		t.Fatalf("randomHex(8): len=%d, want 8", len(got))
	}
}

// ── Dedup: canonical JSON test ──────────────────────────────────────────────

func TestCanonicalJSON_SameKeysDifferentOrder(t *testing.T) {
	a := map[string]any{"keyword": "半导体", "limit": 10}
	b := map[string]any{"limit": 10, "keyword": "半导体"}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("canonical JSON should be same regardless of map insert order: %s vs %s", ja, jb)
	}
}

// ── EnrichTopic: agent loop dedup intercept ─────────────────────────────────

func TestEnrichTopic_AgentLoopDedupIntercept(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	// interpret → lens → r1(call 半导体) → r2(call 半导体重复, dedup) → r3(finish) → analyze
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"call_tool","thought":"查半导体ETF","tool":"list_etf_by_keyword","args":{"keyword":"半导体"}}`)
	airouter.addResponse(`{"action":"call_tool","thought":"再查一次(重复)","tool":"list_etf_by_keyword","args":{"keyword":"半导体"}}`)
	airouter.addResponse(`{"action":"finish","thought":"done","summary":"完成"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "半导体话题"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 501, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "芯片", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 3, ThreadCount: 1,
					ThreadTitles: []string{"芯片"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	if len(output.AgentLoops) != 1 {
		t.Fatalf("expected 1 agent loop result")
	}
	al := output.AgentLoops[0]

	// 2 tool_calls: r1 executed, r2 dedup-intercepted (finish is not a tool call).
	if len(al.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls (1 exec + 1 dedup), got %d", len(al.ToolCalls))
	}
	if al.ToolCalls[0].ResultFull == "" {
		t.Fatal("first tool call should have result")
	}
	if !strings.Contains(al.ToolCalls[1].Thought, "被拦:重复") {
		t.Fatalf("second tool call should be dedup-intercepted, got thought: %s", al.ToolCalls[1].Thought)
	}
}

// ── EnrichTopic: agent loop max_loops guard ─────────────────────────────────

func TestEnrichTopic_AgentLoopMaxLoops(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	for i := 0; i < 6; i++ {
		airouter.addResponse(fmt.Sprintf(`{"action":"call_tool","thought":"step%d","tool":"list_etf_by_keyword","args":{"keyword":"石油%d"}}`, i, i))
	}
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic:    service.TopicBrief{ID: 1, Label: "石油话题"},
			Sections: []service.TimelineSectionNode{},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	al := output.AgentLoops[0]
	if al.Loops != 6 {
		t.Fatalf("agent loop should exhaust maxLoops=6, got %d", al.Loops)
	}
	if al.Error == "" {
		t.Fatal("agent loop should have error on maxLoops exhaustion")
	}
	if !strings.Contains(al.Error, "达到最大循环数") {
		t.Fatalf("error should mention max loops, got: %s", al.Error)
	}
}

// ── EnrichTopic: agent loop history not truncated ───────────────────────────

func TestEnrichTopic_AgentLoopHistoryFullResult(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"call_tool","thought":"查","tool":"list_etf_by_keyword","args":{"keyword":"石油"}}`)
	airouter.addResponse(`{"action":"finish","thought":"done","summary":"完成"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "石油话题"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 601, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "石油", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"石油"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	al := output.AgentLoops[0]
	for _, tc := range al.ToolCalls {
		if tc.ResultFull == "" {
			t.Fatal("tool call should have ResultFull")
		}
		if len(tc.ResultPreview) > len(tc.ResultFull) {
			t.Fatal("ResultPreview should not be longer than ResultFull")
		}
	}
}

// ── EnrichTopic: interpret includes context layers and applied reviews ──────

func TestEnrichTopic_InterpretIncludesContextLayers(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 701, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "测试", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"测试线索"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	ctxLayer := &repository.TopicLifelineContext{
		PersistentTopicID: 1,
		Granularity:       "week",
		Period:            "2026-W27",
		Content:           "本周油价上涨，地缘局势紧张",
		AsOfDate:          mustParseTime("2026-07-06"),
		Source:            "llm_assisted",
	}
	if err := repo.UpsertTopicLifelineContext(context.Background(), ctxLayer); err != nil {
		t.Fatalf("create context layer: %v", err)
	}

	review := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(1),
		CurrResultID:      999,
		DeviationSummary:  "上次因X误判黄金跌",
		Applied:           true,
		Source:            "llm_assisted",
	}
	if err := repo.CreateTopicEnrichmentReview(context.Background(), review); err != nil {
		t.Fatalf("create review: %v", err)
	}

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	if _, err := orch.EnrichTopic(context.Background(), 1); err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	// The FIRST interpret call (Calls[0]) is the form-classification call; it
	// must include context layer content AND applied review text. (The lens
	// propose call that follows includes context but not review text.)
	if len(airouter.Calls) == 0 {
		t.Fatal("no airouter calls")
	}
	first := airouter.Calls[0]
	if first.Operation != "data_enrichment.interpret" {
		t.Fatalf("first call should be interpret, got %s", first.Operation)
	}
	for _, msg := range first.Messages {
		if !strings.Contains(msg.Content, "本周油价上涨") {
			t.Fatal("interpret prompt should contain context layer content")
		}
		if !strings.Contains(msg.Content, "层级上下文") {
			t.Fatal("interpret prompt should contain context layer header")
		}
		if !strings.Contains(msg.Content, "误判黄金跌") {
			t.Fatal("interpret prompt should contain applied review text")
		}
		if !strings.Contains(msg.Content, "历史认知记录") {
			t.Fatal("interpret prompt should contain review section header")
		}
	}
}

// ── analyze includes context layers ─────────────────────────────────────────

func TestEnrichTopic_AnalyzeIncludesContextLayers(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"油价涨3%"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 801, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "测试", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"测试"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	for _, gran := range []string{"week", "month"} {
		ctxLayer := &repository.TopicLifelineContext{
			PersistentTopicID: 1,
			Granularity:       gran,
			Period:            periodByGran(gran),
			Content:           fmt.Sprintf("%s 层内容", gran),
			AsOfDate:          mustParseTime("2026-07-06"),
			Source:            "llm_assisted",
		}
		if err := repo.UpsertTopicLifelineContext(context.Background(), ctxLayer); err != nil {
			t.Fatalf("create context layer %s: %v", gran, err)
		}
	}

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	if _, err := orch.EnrichTopic(context.Background(), 1); err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	foundAnalyze := false
	for _, call := range airouter.Calls {
		if call.Operation != "data_enrichment.analyze" {
			continue
		}
		foundAnalyze = true
		for _, msg := range call.Messages {
			if !strings.Contains(msg.Content, "分层新闻上下文") {
				t.Fatal("analyze prompt should contain context layer header")
			}
			if !strings.Contains(msg.Content, "week 层内容") {
				t.Fatal("analyze prompt should contain week context")
			}
			if !strings.Contains(msg.Content, "各主题实时数据") {
				t.Fatal("analyze prompt should contain topics data section")
			}
			if !strings.Contains(msg.Content, "油价涨3%") {
				t.Fatal("analyze prompt should contain agent loop data")
			}
		}
	}
	if !foundAnalyze {
		t.Fatal("no analyze call found")
	}
}

// periodByGran returns a canonical period string for testing.
func periodByGran(g string) string {
	switch g {
	case "week":
		return "2026-W27"
	case "month":
		return "2026-07"
	case "year":
		return "2026"
	default:
		return "all"
	}
}

// ── EnrichTopic: context_layers filtering (skip ungenerated layers) ─────────

func TestEnrichTopic_ContextLayersSkippedWhenMissing(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 901, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "测试", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"测试"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	ctxLayer := &repository.TopicLifelineContext{
		PersistentTopicID: 1,
		Granularity:       "week",
		Period:            "2026-W27",
		Content:           "week 内容",
		AsOfDate:          mustParseTime("2026-07-06"),
		Source:            "llm_assisted",
	}
	if err := repo.UpsertTopicLifelineContext(context.Background(), ctxLayer); err != nil {
		t.Fatalf("create context: %v", err)
	}

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	var snap map[string]any
	if err := json.Unmarshal(output.Result.InputSnapshot, &snap); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	layers, _ := snap["context_layers"].(map[string]any)
	if len(layers) != 1 {
		t.Fatalf("context_layers in snapshot: want 1 (only week), got %d: %v", len(layers), layers)
	}
	if _, ok := layers["week"]; !ok {
		t.Fatal("snapshot should include week")
	}
}

// ── EnrichTopic: interpret with limited context_layers ──────────────────────

func TestEnrichTopic_ContextLayersConfigFilter(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        14,
		ContextLayers:     []string{"week"},
	}
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "测试"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 1001, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "测试", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"测试"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	for _, gran := range []string{"week", "month"} {
		ctxLayer := &repository.TopicLifelineContext{
			PersistentTopicID: 1,
			Granularity:       gran,
			Period:            periodByGran(gran),
			Content:           fmt.Sprintf("%s 内容", gran),
			AsOfDate:          mustParseTime("2026-07-06"),
			Source:            "llm_assisted",
		}
		if err := repo.UpsertTopicLifelineContext(context.Background(), ctxLayer); err != nil {
			t.Fatalf("create %s: %v", gran, err)
		}
	}

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	var snap map[string]any
	_ = json.Unmarshal(output.Result.InputSnapshot, &snap)
	layers, _ := snap["context_layers"].(map[string]any)
	if len(layers) != 1 {
		t.Fatalf("context_layers in snapshot: want 1 (config filtered to week), got %d", len(layers))
	}
	if _, ok := layers["week"]; !ok {
		t.Fatal("snapshot should include week")
	}
	if _, ok := layers["month"]; ok {
		t.Fatal("snapshot should NOT include month (filtered by config)")
	}
}

// ── Analyze: layered insight schema (event_chain end-to-end) ────────────────

func TestAnalyze_LayeredInsightSchema(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","thought":"done","summary":"油价涨3%"}`)
	airouter.addResponse(`{
		"form": "event_chain",
		"lens": "油价这轮上涨能不能持续",
		"analysis": {
			"fact_layer": [
				{"claim": "霍尔木兹海峡局势升级", "evidence": [{"source_type":"news","ref":"ctx_week_1","quote":"海峡局势升级"}], "verified": true},
				{"claim": "布伦特原油+3.2%", "evidence": [{"source_type":"tool","ref":"tool_resp_1","quote":"+3.2%"}], "verified": true}
			],
			"timeline": [
				{"date": "2026-07-01", "event": "产油国遭袭", "ref": {"source_type":"news","ref":"ctx_week_1"}}
			],
			"insight_layer": [
				{"cert": "high", "title": "短期供应收紧", "logic": "遭袭→产能下降→供应收紧", "evidence": [{"source_type":"news","ref":"ctx_week_1","quote":"设施遭袭"}]},
				{"cert": "medium", "title": "能源板块受益", "logic": "油价涨→能源股估值抬升", "evidence": [{"source_type":"tool","ref":"tool_resp_1"}]},
				{"cert": "question", "title": "下季能否补产", "logic": "取决于其他产油国是否释放储备", "evidence": [{"source_type":"news","ref":"ctx_week_1"}]}
			],
			"depth": {
				"system_reframe": "放进全球能源供应链系统看",
				"mechanism_layers": [{"layer":"供给冲击","deep_logic":"产能骤降→供需缺口","basis":"遭袭报道"}],
				"boundary": "还不能确认补产时序，不宜外推长期趋势",
				"evidence_chain": [{"source_type":"news","ref":"ctx_week_1","quote":"设施遭袭","date":"2026-07-01"}]
			}
		}
	}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{
				ID: 1, Label: "中东地缘紧张", Status: "active",
				FirstSeenDate: mustParseTime("2026-07-01"), LastSeenDate: mustParseTime("2026-07-05"),
				HitCount: 5, ConsecutiveHits: 5,
			},
			Sections: []service.TimelineSectionNode{
				{SectionID: 10001, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "中东局势", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 5, ThreadCount: 2,
					ThreadTitles: []string{"油价飙升"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	var sectors map[string]any
	if err := json.Unmarshal(output.Result.Sectors, &sectors); err != nil {
		t.Fatalf("sectors unmarshal: %v", err)
	}
	if sectors["form"] != "event_chain" {
		t.Fatalf("form: want event_chain, got %v", sectors["form"])
	}
	analysis := sectors["analysis"].(map[string]any)

	// 事实层与见解层分离
	factLayer := analysis["fact_layer"].([]any)
	if len(factLayer) != 2 {
		t.Fatalf("fact_layer: want 2, got %d", len(factLayer))
	}
	insightLayer := analysis["insight_layer"].([]any)
	if len(insightLayer) != 3 {
		t.Fatalf("insight_layer: want 3, got %d", len(insightLayer))
	}
	// 见解挂依据
	for _, ins := range insightLayer {
		im := ins.(map[string]any)
		ev, _ := im["evidence"].([]any)
		if len(ev) == 0 {
			t.Fatalf("insight %q must carry evidence", im["title"])
		}
	}
	// 确定性分级存在
	certs := map[string]bool{}
	for _, ins := range insightLayer {
		certs[ins.(map[string]any)["cert"].(string)] = true
	}
	for _, c := range []string{"high", "medium", "question"} {
		if !certs[c] {
			t.Fatalf("cert %q missing", c)
		}
	}
}

// ── Review Judge: new_findings / overturned / confidence_shift schema ───────

func TestRunReviewJudge_NewFindingsOverturned(t *testing.T) {
	repo := setupOrchTestDB(t)

	prevResult := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1),
		Sectors:           json.RawMessage(`{"form":"event_chain","lens":"L","analysis":{"insight_layer":[{"title":"油价将回落","cert":"medium"}]}}`),
		SessionID:         "data_enrichment_1_prev_pc",
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), prevResult); err != nil {
		t.Fatalf("create prev result: %v", err)
	}

	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)
	airouter.addResponse(`{
		"should_review": true,
		"reason": "新见解出现且推翻旧判断",
		"new_findings": ["供应收紧或持续整季", "航运成本上升"],
		"overturned": ["油价将回落"],
		"confidence_shift": [{"insight": "油价短期承压", "from": "medium", "to": "high"}],
		"affected_context": "week",
		"confidence": 0.85
	}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "中东地缘"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 401, PeriodDate: mustParseTime("2026-07-05"),
					ClusterLabel: "局势升级", Status: "continuing",
					TopicMatchConfidence: "anchor_hit", ArticleCount: 5, ThreadCount: 2,
					ThreadTitles: []string{"冲突"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	if output.Review == nil {
		t.Fatal("should produce a review")
	}
	// Verdict stores new_findings/overturned/confidence_shift.
	var pc map[string]any
	if err := json.Unmarshal(output.Review.Verdict, &pc); err != nil {
		t.Fatalf("verdict unmarshal: %v", err)
	}
	nf, _ := pc["new_findings"].([]any)
	if len(nf) != 2 {
		t.Fatalf("new_findings: want 2, got %d", len(nf))
	}
	ov, _ := pc["overturned"].([]any)
	if len(ov) != 1 || ov[0] != "油价将回落" {
		t.Fatalf("overturned: got %v", ov)
	}
	cs, _ := pc["confidence_shift"].([]any)
	if len(cs) != 1 {
		t.Fatalf("confidence_shift: want 1, got %d", len(cs))
	}
	// DeviationSummary stores reason.
	if output.Review.DeviationSummary == "" {
		t.Fatal("deviation_summary (reason) should not be empty")
	}
	if output.Review.Source != "llm_assisted" {
		t.Fatalf("review source: want llm_assisted, got %s", output.Review.Source)
	}
}

// ── Review judge skipped when prev result has no form (old-format/empty) ─────

func TestEnrichTopic_SkipReviewOnOldFormatPrev(t *testing.T) {
	repo := setupOrchTestDB(t)

	// Seed prev result with OLD-format sectors (bare array, no form field).
	prevResult := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(3),
		Sectors:           json.RawMessage(`[{"sector":"能源","direction":"up"}]`),
		SessionID:         "data_enrichment_3_prev_old",
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), prevResult); err != nil {
		t.Fatalf("create prev result: %v", err)
	}

	airouter := newMockAirRouter()
	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","summary":"done"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := newEnabledConfig()
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 3, Label: "测试"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 501, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "test", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 3)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	// Old-format prev (no form) → guard skips review judge entirely.
	if output.Review != nil {
		t.Fatal("old-format prev should NOT trigger review judge")
	}
	for _, call := range airouter.Calls {
		if call.Operation == "data_enrichment.review_judge" {
			t.Fatal("review_judge should NOT have been called for old-format prev")
		}
	}
}

// ── ToolsForSourceType mapping unit test ──────────────────────────────────

func TestToolsForSourceType(t *testing.T) {
	// After the A-share financial removal there are no built-in source types,
	// so every source_type maps to nil tools (the mechanism is retained as an
	// extension point for future structured external sources).
	for _, st := range []string{"etf_quote", "exchange_rate", "gdelt_event", "bogus"} {
		if tools := service.ToolsForSourceType(st); len(tools) != 0 {
			t.Fatalf("%s: want 0 tools (no built-in source_type mapping), got %d (%v)", st, len(tools), tools)
		}
	}
}

// ── AllowedTools: only permitted tools are advertised and executable ──────

func TestEnrichTopic_OnlyAllowedToolsAdvertised(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"call_tool","thought":"查股价","tool":"get_stock_price","args":{}}`)
	airouter.addResponse(`{"action":"call_tool","thought":"搜背景","tool":"web_search","args":{"query":"半导体"}}`)
	airouter.addResponse(`{"action":"finish","thought":"done","summary":"半导体供需背景已查"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        14,
		ContextLayers:     []string{"week"},
		AllowedTools:      nil, // exploration set always-on; no per-source_type tools exist anymore
	}
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 1, Label: "半导体话题"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 2001, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "芯片", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 3, ThreadCount: 1,
					ThreadTitles: []string{"芯片"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 1)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	al := output.AgentLoops[0]
	if len(al.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls (1 blocked + 1 executed), got %d", len(al.ToolCalls))
	}
	tc1 := al.ToolCalls[0]
	if tc1.Tool != "get_stock_price" {
		t.Fatalf("first tool call: want get_stock_price (unregistered), got %s", tc1.Tool)
	}
	if !strings.Contains(tc1.Thought, "被拦:工具不可用") {
		t.Fatalf("first tool call should be blocked (not in allowed set), got thought: %s", tc1.Thought)
	}
	if !strings.Contains(tc1.ResultFull, "该工具当前不可用") {
		t.Fatalf("first tool result should say 该工具当前不可用, got: %s", tc1.ResultFull)
	}

	tc2 := al.ToolCalls[1]
	if tc2.Tool != "web_search" {
		t.Fatalf("second tool call: want web_search (always-on), got %s", tc2.Tool)
	}
	if strings.Contains(tc2.Thought, "被拦") {
		t.Fatal("second tool call should NOT be blocked")
	}

	// System prompt advertises the always-on exploration + web_search tools, and
	// never the hallucinated/removed financial tool names.
	for _, call := range airouter.Calls {
		if call.Operation != "data_enrichment.tool_use" {
			continue
		}
		for _, msg := range call.Messages {
			if msg.Role == "system" {
				if !strings.Contains(msg.Content, "**web_search**") || !strings.Contains(msg.Content, "**list_boards**") {
					t.Fatal("system prompt should advertise web_search + list_boards (always-on)")
				}
				if strings.Contains(msg.Content, "**get_stock_price**") {
					t.Fatal("system prompt must NOT advertise the hallucinated tool get_stock_price")
				}
				if strings.Contains(msg.Content, "**list_etf_by_keyword**") {
					t.Fatal("system prompt must NOT advertise removed financial tool list_etf_by_keyword")
				}
			}
		}
	}
}

// ── AllowedTools: empty list → no tools advertised, agent finishes directly ─

func TestEnrichTopic_EmptyAllowedToolsNoToolsAdvertised(t *testing.T) {
	repo := setupOrchTestDB(t)
	airouter := newMockAirRouter()

	defaultInterpretLens(airouter)
	airouter.addResponse(`{"action":"finish","thought":"无可用工具，直接完成","summary":"(无可查数据)"}`)
	defaultAnalyzeEventChain(airouter)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	boardCfg := &service.BoardEnrichmentConfig{
		EnrichmentEnabled: true,
		WindowDays:        14,
		ContextLayers:     []string{"week"},
		AllowedTools:      []string{},
	}
	boardReader := &mockBoardConfigReader{cfg: boardCfg}

	lifelineReader := &orchMockLifelineReader{
		data: service.SectionTimelineData{
			Topic: service.TopicBrief{ID: 2, Label: "Rust发布"},
			Sections: []service.TimelineSectionNode{
				{SectionID: 3001, PeriodDate: mustParseTime("2026-07-01"),
					ClusterLabel: "Rust", Status: "emerging",
					TopicMatchConfidence: "auto_new", ArticleCount: 2, ThreadCount: 1,
					ThreadTitles: []string{"Rust 1.90"},
				},
			},
		},
	}
	renderer := service.NewLifelineRenderer()

	orch := service.NewOrchestratorService(
		airouter, repo, lifelineReader, renderer, toolRegistry, boardReader,
		testCap,
	)

	output, err := orch.EnrichTopic(context.Background(), 2)
	if err != nil {
		t.Fatalf("EnrichTopic: %v", err)
	}

	al := output.AgentLoops[0]
	if al.Error != "" {
		t.Fatalf("agent loop should have no error, got: %s", al.Error)
	}
	if al.FinalData != "(无可查数据)" {
		t.Fatalf("agent should finish with canned summary, got: %s", al.FinalData)
	}
	if len(al.ToolCalls) != 0 {
		t.Fatalf("no tool calls should have been made, got %d", len(al.ToolCalls))
	}

	for _, call := range airouter.Calls {
		if call.Operation != "data_enrichment.tool_use" {
			continue
		}
		for _, msg := range call.Messages {
			if msg.Role == "system" {
				if strings.Contains(msg.Content, "**list_etf_by_keyword**") {
					t.Fatal("system prompt should NOT have **list_etf_by_keyword** tool desc (no tools allowed)")
				}
				if strings.Contains(msg.Content, "**get_etf_quote**") {
					t.Fatal("system prompt should NOT have **get_etf_quote** tool desc (no tools allowed)")
				}
			}
		}
	}
}
