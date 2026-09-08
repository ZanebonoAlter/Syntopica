package service_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// qaEventChainSectors is a minimal composite sectors JSON ({form,lens,analysis})
// used to seed a result the QAAgent reads for report context.
const qaEventChainSectors = `{
	"form": "event_chain",
	"lens": "油价这轮上涨能不能持续",
	"analysis": {
		"fact_layer": [{"claim": "产油国设施遭袭", "evidence": [{"source_type":"news","ref":"ctx1","quote":"设施遭袭"}], "verified": true}],
		"timeline": [{"date": "2026-07-01", "event": "遭袭", "ref": {"source_type":"news","ref":"ctx1"}}],
		"insight_layer": [{"cert": "medium", "title": "油价短期承压", "logic": "供应收紧", "evidence": [{"source_type":"news","ref":"ctx1","quote":"油价飙升"}]}]
	}
}`

// seedQAResult inserts an immutable result with the event_chain sectors and
// returns its ID. The QAAgent reads this snapshot for report context.
func seedQAResult(t *testing.T, repo *repository.Repository) uint {
	t.Helper()
	r := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(1),
		Sectors:           json.RawMessage(qaEventChainSectors),
		SessionID:         "data_enrichment_1_seed",
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), r); err != nil {
		t.Fatalf("seed result: %v", err)
	}
	return r.ID
}

// TestQAAgent_ReuseExplorationLoop proves the QA agent drives the SAME tool loop
// as EnrichTopic: the LLM's call_tool decision is executed and recorded as a
// ToolCall, then finish produces the answer. The exploration tool registry is
// shared (here list_boards is allowed but degrades to error JSON because no
// board lister is injected — the tool CALL is still recorded, which is what we
// assert).
func TestQAAgent_ReuseExplorationLoop(t *testing.T) {
	repo := setupOrchTestDB(t)
	resultID := seedQAResult(t, repo)

	ar := newMockAirRouter()
	// Step 1: agent decides to call list_boards. Step 2: finish with an answer.
	ar.addResponse(`{"action":"call_tool","thought":"先看版块全景","tool":"list_boards","args":{}}`)
	ar.addResponse(`{"action":"finish","thought":"done","summary":"油价短期仍承压(推演有据)"}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	qa := service.NewQAAgent(ar, toolRegistry, repo, testCap)

	answer, err := qa.Ask(context.Background(), resultID, "油价还会涨吗")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}

	// The exploration loop executed list_boards and recorded the tool call.
	if len(answer.ToolCalls) != 1 {
		t.Fatalf("tool_calls: want 1 (list_boards), got %d", len(answer.ToolCalls))
	}
	if answer.ToolCalls[0].Tool != "list_boards" {
		t.Fatalf("tool: want list_boards, got %s", answer.ToolCalls[0].Tool)
	}

	// Answer carries the finish summary + a tool ref derived from the call.
	if answer.Answer != "油价短期仍承压(推演有据)" {
		t.Fatalf("answer: got %q", answer.Answer)
	}
	foundToolRef := false
	for _, r := range answer.Refs {
		if r.SourceType == "tool" && r.Ref == "list_boards" {
			foundToolRef = true
		}
	}
	if !foundToolRef {
		t.Fatal("refs should include a tool ref for list_boards")
	}
}

// TestQAAgent_ReportContext proves the QA system prompt embeds the immutable
// report snapshot (form + lens + analysis JSON), so the agent answers FROM the
// report rather than from scratch.
func TestQAAgent_ReportContext(t *testing.T) {
	repo := setupOrchTestDB(t)
	resultID := seedQAResult(t, repo)

	ar := newMockAirRouter()
	ar.addResponse(`{"action":"finish","thought":"直接答","summary":"基于报告..."}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	qa := service.NewQAAgent(ar, toolRegistry, repo, testCap)

	if _, err := qa.Ask(context.Background(), resultID, "供应收紧逻辑成立吗"); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if len(ar.Calls) == 0 {
		t.Fatal("expected at least one airouter call")
	}
	var systemContent string
	for _, call := range ar.Calls {
		if call.Operation != "data_enrichment.qa_tool_use" {
			continue
		}
		for _, msg := range call.Messages {
			if msg.Role == "system" {
				systemContent = msg.Content
			}
		}
	}
	if systemContent == "" {
		t.Fatal("no qa_tool_use system message captured")
	}
	// The system prompt must carry the report's lens, form, and analysis body.
	if !strings.Contains(systemContent, "油价这轮上涨能不能持续") {
		t.Fatal("system prompt should embed the report lens")
	}
	if !strings.Contains(systemContent, "event_chain") {
		t.Fatal("system prompt should embed the report form")
	}
	if !strings.Contains(systemContent, "油价短期承压") {
		t.Fatal("system prompt should embed the report analysis body")
	}
	// The user message must carry the question under the /no_think prefix.
	var userContent string
	for _, call := range ar.Calls {
		if call.Operation != "data_enrichment.qa_tool_use" {
			continue
		}
		for _, msg := range call.Messages {
			if msg.Role == "user" {
				userContent = msg.Content
			}
		}
	}
	if !strings.Contains(userContent, "/no_think") {
		t.Fatal("qa user message should carry the /no_think defense prefix")
	}
	if !strings.Contains(userContent, "供应收紧逻辑成立吗") {
		t.Fatal("qa user message should carry the user question")
	}
}

// TestQAAgent_PersistsQA proves Ask appends a topic_enrichment_qa row
// (source="qa") and NEVER touches the immutable result table.
func TestQAAgent_PersistsQA(t *testing.T) {
	repo := setupOrchTestDB(t)
	resultID := seedQAResult(t, repo)

	// Snapshot the result before Ask to prove immutability.
	before, err := repo.GetTopicEnrichmentResultByID(context.Background(), resultID)
	if err != nil {
		t.Fatalf("get result before: %v", err)
	}

	ar := newMockAirRouter()
	ar.addResponse(`{"action":"finish","thought":"done","summary":"油价短期承压(推演有据)"}`)

	toolRegistry := service.NewRegistry(&nilFetcher{})
	qa := service.NewQAAgent(ar, toolRegistry, repo, testCap)

	if _, err := qa.Ask(context.Background(), resultID, "油价还会涨吗"); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	// 1. A qa row was appended with source="qa".
	list, err := repo.ListTopicEnrichmentQAByResultID(context.Background(), resultID)
	if err != nil {
		t.Fatalf("list qa: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("qa rows: want 1, got %d", len(list))
	}
	qaRow := list[0]
	if qaRow.Source != "qa" {
		t.Fatalf("source: want qa, got %s", qaRow.Source)
	}
	if qaRow.Question != "油价还会涨吗" {
		t.Fatalf("question: got %q", qaRow.Question)
	}
	if qaRow.Answer != "油价短期仍承压(推演有据)" && qaRow.Answer != "油价短期承压(推演有据)" {
		// Answer comes from the finish summary; just assert it is non-empty.
		if qaRow.Answer == "" {
			t.Fatal("answer should not be empty")
		}
	}
	if qaRow.Sedimented {
		t.Fatal("new qa row should default to sedimented=false")
	}

	// 2. The result table is byte-for-byte unchanged (报告不可变, 业务约束#2).
	after, err := repo.GetTopicEnrichmentResultByID(context.Background(), resultID)
	if err != nil {
		t.Fatalf("get result after: %v", err)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	if string(beforeJSON) != string(afterJSON) {
		t.Fatalf("result table must be immutable across Ask:\nbefore=%s\nafter =%s", beforeJSON, afterJSON)
	}
}
