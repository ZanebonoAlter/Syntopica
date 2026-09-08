package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// ── 版块报告追问 QA（board-level-deep-analysis tasks 6.2 QA 缺口修复）───────
//
// SQLite handler 层覆盖（快反馈，-short 可跑）：
//   - 三 kind（brief/investigation/legacy）均可 ask/list——QA 按 result id 工作
//   - 所有权矩阵：跨 board / topic 档经 board 路由 / 不存在 → 404；
//     sediment 的 qa 属另一 result（同 board）→ 404
//   - legacy ask/list/sediment 全链路 + result 行不可变（字节级）
//
// 种子 helper（briefResultRow / investigationResultRow / legacyResultRow）复用
// board_investigation_api_test.go 的定义——含各自 kind 的完整落库校验。
// PG 集成版（jsonb 原样透传 + 真实 legacy fixture）见 board_legacy_read_compat_test.go。

// boardQASeedAnswer is the canned answer shared by board QA handler tests.
var boardQASeedAnswer = &service.QAAnswer{
	Answer: "判断仍成立（已验证）",
	Refs:   []service.Ref{{SourceType: "tool", Ref: "list_boards"}},
}

// boardQAFingerprint snapshots the immutable bytes of one result row.
type boardQAFingerprint struct {
	Sectors   string
	ToolCalls string
	Snapshot  string
}

func boardQAFingerprintOf(t *testing.T, id uint) boardQAFingerprint {
	t.Helper()
	row, err := repository.Repo.GetTopicEnrichmentResultByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load result %d: %v", id, err)
	}
	return boardQAFingerprint{
		Sectors:   string(row.Sectors),
		ToolCalls: string(row.ToolCalls),
		Snapshot:  string(row.InputSnapshot),
	}
}

func TestBoardQA_AllKindsCanAskAndList(t *testing.T) {
	db := setupHandlerTestDB(t)
	const boardID = uint(701)

	// 三 kind 各一份（brief 先种，investigation 挂它作 parent）：
	// QA 按 result id 工作，与 kind 无关（design D5）。
	brief := briefResultRow(t, db, boardID, `[{"id":"q1","question":"节奏?","rationale":"r","related_lane_ids":[]}]`)
	inv := investigationResultRow(t, db, boardID, brief, "产能纪律是否延续")
	legacy := legacyResultRow(t, db, boardID, "旧命题")
	results := []*repository.TopicEnrichmentResult{brief, inv, legacy}

	mockQA := &mockQARunner{answer: boardQASeedAnswer}
	h := newTestHandlerWithQA(db, mockQA)
	r := newTestRouter(h)

	for i, res := range results {
		w := doRequest(t, r, "POST",
			fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, res.ID),
			`{"question": "这条报告的判断现在还成立吗"}`)
		var got service.QAAnswer
		expectJSONSuccess(t, w, &got)
		if got.Answer != boardQASeedAnswer.Answer {
			t.Fatalf("kind[%d] ask answer = %q", i, got.Answer)
		}
		if mockQA.lastResultID != res.ID {
			t.Fatalf("kind[%d] Ask resultID = %d, want %d", i, mockQA.lastResultID, res.ID)
		}
	}

	// GET list：无 QA 行 → 空列表（mock runner 不落库；落库行为在 PG 集成验证）。
	w := doRequest(t, r, "GET",
		fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, legacy.ID), "")
	var list []repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &list)
	if len(list) != 0 {
		t.Fatalf("legacy list = %d rows, want 0 (mock runner persists nothing)", len(list))
	}
}

// TestBoardQA_LegacyAskListSedimentImmutable — legacy kind 全链路：
// ask（persisting runner 模拟 QAAgent 副作用，真实落一行 QA）→ list 返回
// → sediment 只翻 QA flag；result 行 sectors/tool_calls/input_snapshot 字节不变。
func TestBoardQA_LegacyAskListSedimentImmutable(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()
	const boardID = uint(702)
	legacy := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindLegacyBoardAnalysis,
		Sectors:         json.RawMessage(`{"scope":"board","thesis":"旧命题"}`),
		ToolCalls:       json.RawMessage(`[{"tool":"web_search","args":{"query":"q"},"result_preview":"p"}]`),
		InputSnapshot:   json.RawMessage(`{"gate":{"refreshed":1}}`),
		SessionID:       "qa_legacy_immutable",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, legacy); err != nil {
		t.Fatalf("seed legacy result: %v", err)
	}

	before := boardQAFingerprintOf(t, legacy.ID)

	// persisting QA runner：模拟 QAAgent.Ask 的可观察副作用（读 result + 追加
	// 一行 source="qa" 的 QA），不跑 LLM——handler 路由/所有权是本测试主角。
	persist := &persistingQARunner{repo: repository.Repo}
	h := newTestHandlerWithQA(db, persist)
	r := newTestRouter(h)

	// 1. ask → 200 + 答案。
	w := doRequest(t, r, "POST",
		fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, legacy.ID),
		`{"question": "旧结论后来怎样了"}`)
	var ans service.QAAnswer
	expectJSONSuccess(t, w, &ans)
	if ans.Answer == "" {
		t.Fatal("ask must return an answer")
	}

	// 2. list → 恰一行，sedimented=false。
	w = doRequest(t, r, "GET",
		fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardID, legacy.ID), "")
	var list []repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &list)
	if len(list) != 1 {
		t.Fatalf("qa rows after ask = %d, want 1", len(list))
	}
	if list[0].Sedimented {
		t.Fatal("fresh qa row must not be sedimented")
	}
	qaID := list[0].ID

	// 3. sediment → 只翻 flag，回写完整行。
	w = doRequest(t, r, "POST",
		fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardID, legacy.ID, qaID), "")
	var got repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &got)
	if !got.Sedimented || got.ID != qaID {
		t.Fatalf("sediment: got id=%d sedimented=%v", got.ID, got.Sedimented)
	}

	// 4. result 不可变：指纹逐字节不变。
	after := boardQAFingerprintOf(t, legacy.ID)
	if after != before {
		t.Fatalf("result mutated by board qa flow:\nbefore=%+v\nafter=%+v", before, after)
	}
}

// TestBoardQA_OwnershipMatrix — 每个 QA 入口的所有权：
// 跨 board ask/list/sediment、topic 档 result 经 board 路由、不存在 rid/qid、
// qa 属同 board 另一 result —— 一律 404。
func TestBoardQA_OwnershipMatrix(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()
	const (
		boardA = uint(703)
		boardB = uint(704)
	)
	legacyA := legacyResultRow(t, db, boardA, "旧命题A")

	// topic 档 result（PersistentTopicID 挂 55）：board 路由不得放行。
	topicRes := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(55),
		AnalysisScope:     "topic",
		ResultKind:        repository.ResultKindTopicAnalysis,
		Sectors:           json.RawMessage(`{"lens":"l"}`),
		SessionID:         "s",
	}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, topicRes); err != nil {
		t.Fatalf("seed topic result: %v", err)
	}

	// QA 行：属 boardA 的 legacy；同 board 另一份 brief 也备一份 QA（跨 result 用）。
	qaA := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: legacyA.ID, Question: "q-a", Answer: "a-a", Source: "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qaA); err != nil {
		t.Fatalf("seed qa: %v", err)
	}
	briefA2 := briefResultRow(t, db, boardA, "")
	qaA2 := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: briefA2.ID, Question: "q-a2", Answer: "a-a2", Source: "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qaA2); err != nil {
		t.Fatalf("seed qa2: %v", err)
	}

	h := newTestHandlerWithQA(db, &mockQARunner{answer: boardQASeedAnswer})
	r := newTestRouter(h)

	assert404 := func(method, path, body string) {
		t.Helper()
		w := doRequest(t, r, method, path, body)
		if w.Code != 404 {
			t.Fatalf("%s %s: want 404, got %d (%s)", method, path, w.Code, w.Body.String())
		}
	}

	// ask：跨 board / topic 档 / 不存在。
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardB, legacyA.ID), `{"question":"q"}`)
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardA, topicRes.ID), `{"question":"q"}`)
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardA, 999999), `{"question":"q"}`)

	// list：跨 board / topic 档 / 不存在。
	assert404("GET", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardB, legacyA.ID), "")
	assert404("GET", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardA, topicRes.ID), "")
	assert404("GET", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa", boardA, 999999), "")

	// sediment：跨 board / 跨 result（同 board）/ 不存在 qa / 不存在 result。
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardB, legacyA.ID, qaA.ID), "")
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardA, legacyA.ID, qaA2.ID), "")
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardA, legacyA.ID, 999999), "")
	assert404("POST", fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardA, 999999, qaA.ID), "")

	// 语义完好性：上面 404 的所有权组合不翻任何 flag。
	var sedimented int
	if err := repository.Repo.DB().Raw(`SELECT COUNT(*) FROM topic_enrichment_qa WHERE sedimented`).Scan(&sedimented).Error; err != nil {
		t.Fatalf("count sedimented: %v", err)
	}
	if sedimented != 0 {
		t.Fatalf("rejected sediment attempts must not flip any flag, got %d", sedimented)
	}

	// 同 board 正路 sediment 仍可用（对照组）。
	w := doRequest(t, r, "POST",
		fmt.Sprintf("/api/semantic-boards/%d/enrichment/analysis/results/%d/qa/%d/sediment", boardA, legacyA.ID, qaA.ID), "")
	var ok2 repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &ok2)
	if !ok2.Sedimented {
		t.Fatal("same-board same-result sediment must succeed")
	}
}

// TestListQA_IDORProtection — topic 档 listQA 补上属主校验（6.2 顺手修复）：
// 跨 topic 读取（含 board 档 result 借 topic 路由）→ 404，不泄漏行。
func TestListQA_IDORProtection(t *testing.T) {
	db := setupHandlerTestDB(t)
	ctx := context.Background()

	result := &repository.TopicEnrichmentResult{PersistentTopicID: repository.TopicIDPtr(1), SessionID: "s"}
	if err := repository.Repo.CreateTopicEnrichmentResult(ctx, result); err != nil {
		t.Fatalf("seed: %v", err)
	}
	qa := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: result.ID, Question: "q", Answer: "a", Source: "qa",
	}
	if err := repository.Repo.CreateTopicEnrichmentQA(ctx, qa); err != nil {
		t.Fatalf("seed qa: %v", err)
	}
	boardRes := legacyResultRow(t, db, 705, "旧命题B")

	h := newTestHandler(db, nil, nil, nil) // list GET needs no QA runner
	r := newTestRouter(h)

	// 正主可读。
	w := doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d/qa", result.ID), "")
	var list []repository.TopicEnrichmentQA
	expectJSONSuccess(t, w, &list)
	if len(list) != 1 {
		t.Fatalf("owner list = %d, want 1", len(list))
	}

	// 跨 topic → 404。
	w = doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/2/enrichment/results/%d/qa", result.ID), "")
	if w.Code != 404 {
		t.Fatalf("cross-topic list: want 404, got %d", w.Code)
	}
	// board 档 result 借 topic 路由 → 404（board QA 走 board 路由）。
	w = doRequest(t, r, "GET", fmt.Sprintf("/api/persistent-topics/1/enrichment/results/%d/qa", boardRes.ID), "")
	if w.Code != 404 {
		t.Fatalf("board result via topic route: want 404, got %d", w.Code)
	}
}

// persistingQARunner mimics QAAgent.Ask's observable side effects without the
// LLM loop: loads the result (same as the real agent's report-context read)
// and appends one topic_enrichment_qa row (source="qa").
type persistingQARunner struct {
	repo *repository.Repository
}

func (p *persistingQARunner) Ask(ctx context.Context, resultID uint, question string) (*service.QAAnswer, error) {
	if _, err := p.repo.GetTopicEnrichmentResultByID(ctx, resultID); err != nil {
		return nil, err
	}
	qaRow := &repository.TopicEnrichmentQA{
		TopicEnrichmentResultID: resultID,
		Question:                question,
		Answer:                  "基于报告分析的回答（已验证）",
		ToolCalls:               json.RawMessage(`[]`),
		Source:                  "qa",
	}
	if err := p.repo.CreateTopicEnrichmentQA(ctx, qaRow); err != nil {
		return nil, err
	}
	return &service.QAAnswer{Answer: qaRow.Answer}, nil
}
