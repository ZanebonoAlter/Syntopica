package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── EnrichBoard 简报编排（tasks 3.3 / M2）——testcontainer PG 全链路（mock LLM）──
//
// 契约（spec board-level-analysis + D2/D3）：
//   - 默认触发只发生 board_brief 的 Router.Chat（合法 1 次 / 坏 JSON 2 次），
//     旧 board operations（interpret/tool_use/analyze/review_judge）次数 0
//   - prompt 无工具说明、无方法卡全文、无作者画像
//   - 持久化不可变 board_brief 快照（tool_calls=[]，input_snapshot 可重建）
//   - 坏 JSON 两次后机械降级仍持久化；全 sparse 诚实简报、零研究问题
//   - 旧论文链已退场（tasks 6.1）：EnrichBoardLegacyAnalysis 入口删除，
//     新 trigger 调用链中 board_interpret / 旧 board directions（tool_use）/
//     analyze 次数恒为 0，落库只可能是 board_brief

// mockBoardResolver: canned board config by board ID.
type mockBoardResolver struct {
	enabled      bool
	relationAuto bool // auto relation discovery switch (7.1 tests)
}

func (m *mockBoardResolver) GetBoardConfigByBoardID(ctx context.Context, boardID uint) (*service.BoardEnrichmentConfig, error) {
	cfg := service.DefaultBoardConfig()
	cfg.EnrichmentEnabled = m.enabled
	cfg.RelationAutoDiscoveryEnabled = m.relationAuto
	return cfg, nil
}

// noopFreshnessRefresher disables the completeness gate in tests.
type noopFreshnessRefresher struct{}

func (noopFreshnessRefresher) RefreshGranularity(ctx context.Context, topicID uint, granularity string, now time.Time) error {
	return nil
}

func (noopFreshnessRefresher) RefreshPeriod(ctx context.Context, topicID uint, granularity, period string, now time.Time) error {
	return nil
}

func (noopFreshnessRefresher) SectionDates(ctx context.Context, topicID uint) ([]time.Time, error) {
	return nil, nil
}

// newEnrichBoardOrch wires the orchestrator against an isolated testcontainer
// Postgres (golden-schema style: AutoMigrate once per process-cached DB).
func newEnrichBoardOrch(t *testing.T, enabled bool) (*service.OrchestratorService, *mockAirRouter, *repository.Repository) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		t.Fatalf("create vector extension: %v", err)
	}
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	repo := repository.NewRepository(db)
	router := newMockAirRouter()
	orch := service.NewOrchestratorService(
		router, repo, &orchMockLifelineReader{}, service.NewLifelineRenderer(),
		service.NewRegistry(&nilFetcher{}), &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	orch.SetBoardConfigResolver(&mockBoardResolver{enabled: enabled})
	orch.SetFreshnessRefresher(&noopFreshnessRefresher{})
	return orch, router, repo
}

// seedBoardLane inserts one active lane (Postgres-flavoured seeds).
func seedBoardLane(t *testing.T, repo *repository.Repository, id uint, boardID uint, label string) {
	t.Helper()
	sql := fmt.Sprintf(
		`INSERT INTO board_persistent_topics (id, semantic_board_id, label, status, source, first_seen_date, last_seen_date, hit_count, consecutive_hits, created_at, updated_at)
		 VALUES (%d, %d, '%s', 'active', 'auto', '2026-08-01', '2026-08-26', 20, 8, now(), now())`,
		id, boardID, label)
	if err := repo.DB().Exec(sql).Error; err != nil {
		t.Fatalf("seed lane: %v", err)
	}
	t.Cleanup(func() { _ = repo.DB().Exec("DELETE FROM board_persistent_topics WHERE id = ?", id).Error })
}

// seedWeekLifelinePG inserts a week lifeline row for a lane (Postgres flavour).
func seedWeekLifelinePG(t *testing.T, repo *repository.Repository, topicID uint, period, content string, asOf time.Time) {
	t.Helper()
	sql := fmt.Sprintf(
		`INSERT INTO topic_lifeline_context (persistent_topic_id, granularity, period, content, as_of_date, source, created_at, updated_at)
		 VALUES (%d, 'week', '%s', '%s', '%s', 'manual', now(), now())`,
		topicID, period, content, asOf.Format("2006-01-02"))
	if err := repo.DB().Exec(sql).Error; err != nil {
		t.Fatalf("seed week lifeline: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.DB().Exec("DELETE FROM topic_lifeline_context WHERE persistent_topic_id = ?", topicID).Error
	})
}

// seedEnabledAnalysisMethod plants an enabled method card whose content must
// never reach the brief prompt (M7.5 stage isolation).
func seedEnabledAnalysisMethod(t *testing.T, repo *repository.Repository) string {
	t.Helper()
	const sentinel = "BRIEF_MUST_NOT_INJECT_METHOD_CARD_SENTINEL"
	m := &repository.AnalysisMethod{
		Name: "brief-isolation-probe", Title: "隔离探针", Content: sentinel, Enabled: true,
	}
	if err := repo.DB().Create(m).Error; err != nil {
		t.Fatalf("seed analysis method: %v", err)
	}
	t.Cleanup(func() { _ = repo.DB().Unscoped().Where("id = ?", m.ID).Delete(&repository.AnalysisMethod{}).Error })
	return sentinel
}

// validBriefLLM is a legal board_brief response (one ghost observation +
// ghost lane id inside a relationship to exercise the sanitizer end-to-end).
const validBriefLLM = `{"summary":"板块两条泳道有进展，暂未发现统一关系。","observations":[
	{"id":"o1","lane_id":901,"statement":"观察甲：产能落地","basis":"周摘要","as_of_date":"2026-08-26"},
	{"id":"o2","lane_id":9999,"statement":"幽灵观察","basis":"x","as_of_date":"2026-08-26"}],
	"relationships":[{"lane_ids":[901,902,9999],"type":"context_only","explanation":"同板块背景相关","confidence":"low","evidence_refs":["o1"]}],
	"uncertainties":[{"question":"后续节奏?","why_uncertain":"材料尚少","needed_evidence":"持续跟踪"}],
	"research_questions":[{"id":"q1","question":"产能落地节奏如何影响布局?","rationale":"决定下一步","related_lane_ids":[901]}],
	"lane_refs":[{"lane_id":901,"note":"主观察"}]}`

// countBriefCalls returns how many mock calls used the board_brief operation.
func countBriefCalls(router *mockAirRouter) int {
	n := 0
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.board_brief" {
			n++
		}
	}
	return n
}

// assertNoLegacyOps fails when any call used a legacy thesis-chain operation.
// review_judge is NOT here: since task 3.5 the same-kind board-brief review
// judge legitimately runs in the brief session (design D11 / M10.2). Tests
// that must not see it assert countJudgeCalls == 0 explicitly.
func assertNoLegacyOps(t *testing.T, router *mockAirRouter) {
	t.Helper()
	legacy := map[string]bool{
		"data_enrichment.board_interpret": true,
		"data_enrichment.interpret":       true,
		"data_enrichment.tool_use":        true,
		"data_enrichment.analyze":         true,
	}
	for i, c := range router.Calls {
		if legacy[c.Operation] {
			t.Fatalf("call %d used legacy operation %q — default trigger must not run the thesis chain", i, c.Operation)
		}
	}
}

// countJudgeCalls returns how many mock calls used the review_judge operation.
func countJudgeCalls(router *mockAirRouter) int {
	n := 0
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.review_judge" {
			n++
		}
	}
	return n
}

// judgeTrueLLM / judgeFalseLLM: canned review-judge responses.
const judgeTrueLLM = `{"should_review":true,"reason":"新增重要观察且一条关系置信上移","new_findings":["观察乙首次出现"],"overturned":["旧判断甲已消失"],"confidence_shift":[{"relation":"甲乙关系","from":"low","to":"medium"}],"affected_context":"","confidence":0.7}`

const judgeFalseLLM = `{"should_review":false,"reason":"见解层无实质认知更新","new_findings":[],"overturned":[],"confidence_shift":[],"affected_context":"","confidence":0.2}`

// lifelineSnapshotRow is one comparable lifeline row for the "review never
// writes back to table 1" red line: content-level before/after snapshot.
type lifelineSnapshotRow struct {
	Granularity string
	Period      string
	Content     string
	AsOfDate    string
}

func snapshotLifeline(t *testing.T, db *gorm.DB, topicID uint) []lifelineSnapshotRow {
	t.Helper()
	var rows []lifelineSnapshotRow
	if err := db.Raw(`SELECT granularity, period, content, to_char(as_of_date, 'YYYY-MM-DD') AS as_of_date
		FROM topic_lifeline_context WHERE persistent_topic_id = ? ORDER BY granularity, period`, topicID).Scan(&rows).Error; err != nil {
		t.Fatalf("snapshot lifeline: %v", err)
	}
	return rows
}

func requireSameLifeline(t *testing.T, before, after []lifelineSnapshotRow) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("lifeline row count changed: %d → %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("lifeline row %d changed: %+v → %+v", i, before[i], after[i])
		}
	}
}

// seedBoardResult inserts a board-scope result row directly (no LLM).
func seedBoardResult(t *testing.T, repo *repository.Repository, boardID uint, kind, sectors, sessionID string) *repository.TopicEnrichmentResult {
	t.Helper()
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: kind, Sectors: json.RawMessage(sectors), SessionID: sessionID,
	}
	if err := repo.CreateTopicEnrichmentResult(context.Background(), res); err != nil {
		t.Fatalf("seed board result (%s): %v", kind, err)
	}
	return res
}

// seedBoardReview inserts a board-scope review row directly (no LLM).
func seedBoardReview(t *testing.T, repo *repository.Repository, boardID, currResultID uint, summary string, applied bool) *repository.TopicEnrichmentReview {
	t.Helper()
	rv := &repository.TopicEnrichmentReview{
		SemanticBoardID: repository.BoardIDPtr(boardID), CurrResultID: currResultID,
		DeviationSummary: summary, Applied: applied, Source: "llm_assisted",
	}
	if err := repo.CreateTopicEnrichmentReview(context.Background(), rv); err != nil {
		t.Fatalf("seed board review: %v", err)
	}
	return rv
}

// seedBriefSectors is a valid board_brief-shaped sectors payload.
const seedBriefSectors = `{"scope":"board","result_kind":"board_brief","summary":"上一份简报概览：甲有产能落地","observations":[{"id":"o1","lane_id":901,"statement":"观察甲：产能落地","basis":"周摘要","as_of_date":"2026-08-26"}],"relationships":[],"uncertainties":[],"research_questions":[],"lane_refs":[]}`

// 版块未开启增强 → 拒绝且零 LLM 调用。
func TestEnrichBoard_DisabledBoardRejected(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, false)
	seedBoardLane(t, repo, 901, 8801, "泳道甲")

	_, err := orch.EnrichBoard(context.Background(), 8801)
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("want not-enabled rejection, got %v", err)
	}
	if len(router.Calls) != 0 {
		t.Fatal("no LLM calls before the gate")
	}
}

// 默认触发只生成简报：1 次 board_brief 调用、零旧链调用、零工具说明/方法卡，
// 持久化 board_brief 快照（幽灵 lane 清洗、tool_calls=[]、无 review 行）。
func TestEnrichBoard_DefaultTriggerIsBriefOnly(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8801, "泳道甲")
	seedBoardLane(t, repo, 902, 8801, "泳道乙")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	sentinel := seedEnabledAnalysisMethod(t, repo)
	router.addResponse(validBriefLLM)

	out, err := orch.EnrichBoard(context.Background(), 8801)
	if err != nil {
		t.Fatalf("EnrichBoard: %v", err)
	}

	// LLM 契约：恰好 1 次 board_brief，零旧 operations。
	if got := countBriefCalls(router); got != 1 {
		t.Fatalf("default trigger must make exactly 1 board_brief call, got %d (total %d)", got, len(router.Calls))
	}
	assertNoLegacyOps(t, router)

	// Prompt 契约：态势卡注入；无工具说明、无方法卡正文。
	prompt := router.Calls[0].Messages[0].Content
	for _, want := range []string{"泳道态势卡", "周内容"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("brief prompt missing %q", want)
		}
	}
	for _, banned := range []string{"可用工具", "web_search", sentinel, "而是", "thesis"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("brief prompt must not contain %q", banned)
		}
	}

	// 持久化契约：board_brief 快照。
	res := out.Result
	if res.AnalysisScope != "board" || res.ResultKind != repository.ResultKindBoardBrief {
		t.Fatalf("kind: want board/board_brief, got %s/%s", res.AnalysisScope, res.ResultKind)
	}
	if res.PersistentTopicID != nil || res.SemanticBoardID == nil || *res.SemanticBoardID != 8801 {
		t.Fatalf("board ownership wrong: topic=%v board=%v", res.PersistentTopicID, res.SemanticBoardID)
	}
	if res.ParentResultID != nil || res.QuestionKey != nil {
		t.Fatal("brief must have no parent/question_key")
	}
	if !strings.HasPrefix(res.SessionID, "data_enrichment_board_8801_") {
		t.Fatalf("session id: %s", res.SessionID)
	}
	if string(res.ToolCalls) != "[]" {
		t.Fatalf("brief tool_calls must be an empty array, got %s", res.ToolCalls)
	}
	if out.Review != nil {
		t.Fatal("first brief must not run the review judge (no previous same-kind brief)")
	}
	if got := countJudgeCalls(router); got != 0 {
		t.Fatalf("first brief must make 0 review_judge calls, got %d", got)
	}
	var nReview int
	if err := db.Raw("SELECT COUNT(*) FROM topic_enrichment_review WHERE semantic_board_id = 8801").Scan(&nReview).Error; err != nil {
		t.Fatalf("count reviews: %v", err)
	}
	if nReview != 0 {
		t.Fatalf("no review rows expected, got %d", nReview)
	}

	// Sectors 契约：六字段 + 幽灵清洗。
	var payload map[string]any
	if err := json.Unmarshal(res.Sectors, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["result_kind"] != "board_brief" {
		t.Fatalf("sectors result_kind: %v", payload["result_kind"])
	}
	for _, f := range []string{"summary", "observations", "relationships", "uncertainties", "research_questions", "lane_refs"} {
		if _, ok := payload[f]; !ok {
			t.Fatalf("sectors missing %q", f)
		}
	}
	obs := payload["observations"].([]any)
	if len(obs) != 1 || obs[0].(map[string]any)["lane_id"].(float64) != 901 {
		t.Fatalf("ghost observation must be scrubbed: %v", obs)
	}
	rel := payload["relationships"].([]any)[0].(map[string]any)
	laneIDs := rel["lane_ids"].([]any)
	if len(laneIDs) != 2 || laneIDs[0].(float64) != 901 || laneIDs[1].(float64) != 902 {
		t.Fatalf("relationship ghost lane id must be scrubbed: %v", laneIDs)
	}

	// input_snapshot 可重建：cards / freshness / prompt_inputs / generation。
	var snap map[string]any
	if err := json.Unmarshal(res.InputSnapshot, &snap); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	for _, k := range []string{"cards", "freshness", "prompt_inputs", "generation"} {
		if _, ok := snap[k]; !ok {
			t.Fatalf("input_snapshot missing %q", k)
		}
	}
	if cards := snap["cards"].([]any); len(cards) != 2 {
		t.Fatalf("snapshot must carry both cards, got %d", len(cards))
	}
	if gen := snap["generation"].(map[string]any); gen["attempts"].(float64) != 1 {
		t.Fatalf("generation meta attempts: %v", gen["attempts"])
	}
}

// 两连触发：两份独立不可变简报快照，互不覆盖；第二份跑同 kind review judge
// （3.5）；judge 判 false 不写任何 review 行；lifeline 内容级快照不变。
func TestEnrichBoard_SecondBriefIndependentSnapshot(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8802, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容", time.Now())

	router.addResponse(validBriefLLM)
	out1, err := orch.EnrichBoard(context.Background(), 8802)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if got := countJudgeCalls(router); got != 0 {
		t.Fatalf("first brief must make 0 judge calls, got %d", got)
	}

	before := snapshotLifeline(t, db, 901)

	router.addResponse(validBriefLLM)
	router.addResponse(judgeFalseLLM)
	out2, err := orch.EnrichBoard(context.Background(), 8802)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if out2.Result.ID == out1.Result.ID {
		t.Fatal("second brief must be an independent snapshot")
	}
	// 第二份 → judge 调用恰好 1 次（同一 board session）。 judge false → 无 review。
	if got := countJudgeCalls(router); got != 1 {
		t.Fatalf("second brief must run the review judge exactly once, got %d", got)
	}
	if out2.Review != nil {
		t.Fatal("should_review=false must not produce a review")
	}
	var kinds []string
	if err := db.Raw("SELECT result_kind FROM topic_enrichment_result WHERE semantic_board_id = 8802 ORDER BY id").Scan(&kinds).Error; err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(kinds) != 2 || kinds[0] != "board_brief" || kinds[1] != "board_brief" {
		t.Fatalf("two board_brief snapshots expected, got %v", kinds)
	}
	var nReview int
	_ = db.Raw("SELECT COUNT(*) FROM topic_enrichment_review WHERE semantic_board_id = 8802").Scan(&nReview).Error
	if nReview != 0 {
		t.Fatalf("judge false must write no review rows, got %d", nReview)
	}
	// 红线：简报 + review 全链不回写新闻记忆（内容级快照）。
	requireSameLifeline(t, before, snapshotLifeline(t, db, 901))
}

// 3.5 契约：第二份简报 judge 判 true → 写 review 行（BaseResultID=上一份 brief、
// NewResultID=当前 brief、board 所有权正确、topic id 为空），out2.Review 透出；
// judge prompt 只比较简报字段；lifeline 内容不变。
func TestEnrichBoard_SecondBriefReviewJudgeWritesReview(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8807, "泳道甲")
	seedBoardLane(t, repo, 902, 8807, "泳道乙")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())

	router.addResponse(validBriefLLM)
	out1, err := orch.EnrichBoard(context.Background(), 8807)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := snapshotLifeline(t, db, 901)

	router.addResponse(validBriefLLM)
	router.addResponse(judgeTrueLLM)
	out2, err := orch.EnrichBoard(context.Background(), 8807)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	// Judge 调用契约：1 次、review_judge operation、同一 board session。
	if got := countJudgeCalls(router); got != 1 {
		t.Fatalf("second brief must run the judge exactly once, got %d", got)
	}
	var judgePrompt string
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.review_judge" {
			judgePrompt = c.Messages[0].Content
			if c.SessionID != out2.Result.SessionID {
				t.Fatalf("judge session %q != brief session %q", c.SessionID, out2.Result.SessionID)
			}
		}
	}
	// 比较内容契约：两份简报的 observations/relationships 等字段进入 prompt；
	// thesis/argument/depth 字样不出现。
	for _, want := range []string{"observations", "relationships", "板块两条泳道有进展", "观察甲：产能落地"} {
		if !strings.Contains(judgePrompt, want) {
			t.Fatalf("judge prompt missing %q", want)
		}
	}
	for _, banned := range []string{"thesis", "argument", "depth"} {
		if strings.Contains(judgePrompt, banned) {
			t.Fatalf("judge prompt must not contain field %q", banned)
		}
	}

	// Review 行契约。
	if out2.Review == nil {
		t.Fatal("judge true must surface a review in the output")
	}
	var reviews []repository.TopicEnrichmentReview
	if err := db.Where("semantic_board_id = 8807").Order("id").Find(&reviews).Error; err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("exactly one review row expected, got %d", len(reviews))
	}
	rv := reviews[0]
	if rv.PrevResultID == nil || *rv.PrevResultID != out1.Result.ID {
		t.Fatalf("review prev must point at the previous brief %d, got %v", out1.Result.ID, rv.PrevResultID)
	}
	if rv.CurrResultID != out2.Result.ID {
		t.Fatalf("review curr must point at the current brief %d, got %d", out2.Result.ID, rv.CurrResultID)
	}
	if rv.PersistentTopicID != nil {
		t.Fatalf("board review must leave persistent_topic_id nil, got %v", rv.PersistentTopicID)
	}
	if rv.SemanticBoardID == nil || *rv.SemanticBoardID != 8807 {
		t.Fatalf("review board ownership wrong: %v", rv.SemanticBoardID)
	}
	if rv.Applied {
		t.Fatal("judge-written review starts unapplied")
	}
	if rv.DeviationSummary != "新增重要观察且一条关系置信上移" {
		t.Fatalf("deviation summary: %q", rv.DeviationSummary)
	}
	if !strings.Contains(string(rv.Verdict), "观察乙首次出现") {
		t.Fatalf("verdict must carry new_findings: %s", rv.Verdict)
	}
	if rv.ID != out2.Review.ID {
		t.Fatalf("output review must be the persisted row: %d vs %d", out2.Review.ID, rv.ID)
	}

	// 红线：lifeline 内容不变。
	requireSameLifeline(t, before, snapshotLifeline(t, db, 901))
}

// 3.5 契约：brief#1 与 brief#2 之间插入 legacy_board_analysis（id 更高）时，
// judge 仍只比较上一份 board_brief，不读 legacy thesis。
func TestEnrichBoard_ReviewJudgeSkipsLegacyPrev(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8808, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容", time.Now())

	router.addResponse(validBriefLLM)
	out1, err := orch.EnrichBoard(context.Background(), 8808)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Insert a legacy thesis-shaped result AFTER brief#1 (higher id).
	legacy := seedBoardResult(t, repo, 8808, repository.ResultKindLegacyBoardAnalysis,
		`{"scope":"board","thesis":"legacy 论文命题：一切都是底层传导","argument":{},"depth":{}}`, "seed-legacy-prev")

	router.addResponse(validBriefLLM)
	router.addResponse(judgeTrueLLM)
	out2, err := orch.EnrichBoard(context.Background(), 8808)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if got := countJudgeCalls(router); got != 1 {
		t.Fatalf("judge must run exactly once, got %d", got)
	}
	var judgePrompt string
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.review_judge" {
			judgePrompt = c.Messages[0].Content
		}
	}
	if !strings.Contains(judgePrompt, "观察甲：产能落地") {
		t.Fatal("judge must compare against the previous board_brief content")
	}
	if strings.Contains(judgePrompt, "legacy 论文命题") {
		t.Fatal("judge prompt must not read the legacy thesis")
	}

	var reviews []repository.TopicEnrichmentReview
	if err := db.Where("semantic_board_id = 8808").Find(&reviews).Error; err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("one review expected, got %d", len(reviews))
	}
	if reviews[0].PrevResultID == nil || *reviews[0].PrevResultID != out1.Result.ID || *reviews[0].PrevResultID == legacy.ID {
		t.Fatalf("review prev must be the previous brief %d (not legacy %d), got %v", out1.Result.ID, legacy.ID, reviews[0].PrevResultID)
	}
	if reviews[0].CurrResultID != out2.Result.ID {
		t.Fatalf("review curr: %d", reviews[0].CurrResultID)
	}
}

// 3.5 review 修复：brief#1 与 brief#2 之间插入合法 board_investigation
// （parent=brief#1、同 board、question_key 有效）时，第二份简报的 review prev
// 必须仍是 brief#1 —— kind 隔离不只挡 legacy，调查链（同 board 的直接子节点）
// 同样不参与 brief-vs-brief 比较，judge prompt 也不读其内容。
func TestEnrichBoard_ReviewJudgeSkipsInvestigationPrev(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8813, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())

	router.addResponse(validBriefLLM)
	out1, err := orch.EnrichBoard(context.Background(), 8813)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Legal investigation child of brief#1: same board, valid question_key,
	// id higher than brief#1 (inserted between the two briefs).
	questionKey := repository.ComputeQuestionKey("为什么产能落地节奏分化？")
	investigation := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(8813), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(out1.Result.ID), QuestionKey: &questionKey,
		SessionID: "seed-investigation-prev",
		Sectors:   json.RawMessage(`{"scope":"board","hypotheses":[],"investigation_marker":"调查链内容不应进入 judge"}`),
	}
	if err := repo.CreateBoardInvestigationResult(context.Background(), investigation); err != nil {
		t.Fatalf("seed investigation child: %v", err)
	}
	if investigation.ID <= out1.Result.ID {
		t.Fatal("investigation must sit between the two briefs (higher id than brief#1)")
	}

	router.addResponse(validBriefLLM)
	router.addResponse(judgeTrueLLM)
	out2, err := orch.EnrichBoard(context.Background(), 8813)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if got := countJudgeCalls(router); got != 1 {
		t.Fatalf("judge must run exactly once, got %d", got)
	}
	var judgePrompt string
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.review_judge" {
			judgePrompt = c.Messages[0].Content
		}
	}
	if strings.Contains(judgePrompt, "调查链内容不应进入 judge") {
		t.Fatal("judge prompt must not read the investigation payload")
	}

	var reviews []repository.TopicEnrichmentReview
	if err := db.Where("semantic_board_id = 8813").Find(&reviews).Error; err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("one review expected, got %d", len(reviews))
	}
	if reviews[0].PrevResultID == nil || *reviews[0].PrevResultID != out1.Result.ID {
		t.Fatalf("review prev must stay at brief#1 %d even with a legal investigation in between, got %v", out1.Result.ID, reviews[0].PrevResultID)
	}
	if reviews[0].CurrResultID != out2.Result.ID {
		t.Fatalf("review curr: %d", reviews[0].CurrResultID)
	}
}

// 3.5 review 修复：模型无视“留空”指令返回超长 affected_context 时，review 落库
// 不得因此失败（affected_context 是 varchar(10) 列，透传会炸整条 INSERT）——
// judge 写库前无条件清空该字段，review 行仍落库且字段为空。
func TestEnrichBoard_ReviewJudgeClampsAffectedContext(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8814, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容", time.Now())

	router.addResponse(validBriefLLM)
	if _, err := orch.EnrichBoard(context.Background(), 8814); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Malicious judge output: should_review=true + a >10-char affected_context
	// (varchar(10) overflow would lose the whole review row if passed through).
	router.addResponse(validBriefLLM)
	router.addResponse(`{"should_review":true,"reason":"有认知更新但 affected_context 恶意超长","new_findings":["观察新出现"],"overturned":[],"confidence_shift":[],"affected_context":"` + strings.Repeat("x", 120) + `","confidence":0.8}`)
	out2, err := orch.EnrichBoard(context.Background(), 8814)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if out2.Review == nil {
		t.Fatal("review must still surface despite the oversized affected_context")
	}

	var reviews []repository.TopicEnrichmentReview
	if err := db.Where("semantic_board_id = 8814").Find(&reviews).Error; err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("review row must survive, got %d", len(reviews))
	}
	if reviews[0].AffectedContext != "" {
		t.Fatalf("affected_context must be unconditionally emptied, got %q", reviews[0].AffectedContext)
	}
	if reviews[0].ID != out2.Review.ID {
		t.Fatalf("output review must be the persisted row: %d vs %d", out2.Review.ID, reviews[0].ID)
	}
}

// 3.5 契约：注入下一份 brief 的 applied review digest 必须 kind 隔离——
// board_brief 链的 applied review 进入“历史认知提醒”，legacy 链与他版块的
// applied review 不得进入；digest 写入 input_snapshot.prompt_inputs。
func TestEnrichBoard_ReviewDigestKindFiltered(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	seedBoardLane(t, repo, 901, 8810, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容", time.Now())

	// Seeded same-board brief + applied review → must enter the digest.
	prevBrief := seedBoardResult(t, repo, 8810, repository.ResultKindBoardBrief, seedBriefSectors, "seed-digest-prev-brief")
	seedBoardReview(t, repo, 8810, prevBrief.ID, "BRIEF-CHAIN-DIGEST：曾把共现误判为因果", true)
	// Same-board legacy result + applied review → must NOT enter.
	legacyRes := seedBoardResult(t, repo, 8810, repository.ResultKindLegacyBoardAnalysis,
		`{"scope":"board","thesis":"legacy","argument":{},"depth":{}}`, "seed-digest-legacy")
	seedBoardReview(t, repo, 8810, legacyRes.ID, "LEGACY-CHAIN-DIGEST：旧论文复盘", true)
	// Other-board brief + applied review → must NOT enter.
	otherBrief := seedBoardResult(t, repo, 8811, repository.ResultKindBoardBrief, seedBriefSectors, "seed-digest-other-brief")
	seedBoardReview(t, repo, 8811, otherBrief.ID, "OTHER-BOARD-DIGEST：他版块复盘", true)
	// Same-board pending (not applied) review → must NOT enter.
	seedBoardReview(t, repo, 8810, prevBrief.ID, "PENDING-DIGEST：未采纳复盘", false)

	router.addResponse(validBriefLLM)
	router.addResponse(judgeTrueLLM) // prev brief exists → judge also runs
	out, err := orch.EnrichBoard(context.Background(), 8810)
	if err != nil {
		t.Fatalf("EnrichBoard: %v", err)
	}

	briefPrompt := router.Calls[0].Messages[0].Content
	if !strings.Contains(briefPrompt, "BRIEF-CHAIN-DIGEST：曾把共现误判为因果") {
		t.Fatal("applied board_brief-chain review must enter the digest")
	}
	for _, banned := range []string{"LEGACY-CHAIN-DIGEST", "OTHER-BOARD-DIGEST", "PENDING-DIGEST"} {
		if strings.Contains(briefPrompt, banned) {
			t.Fatalf("brief prompt must not contain %q (kind/board/applied isolation broken)", banned)
		}
	}

	// Digest 回放：input_snapshot.prompt_inputs.review_digest 固化注入内容。
	var snap struct {
		PromptInputs struct {
			ReviewDigest string `json:"review_digest"`
		} `json:"prompt_inputs"`
	}
	if err := json.Unmarshal(out.Result.InputSnapshot, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if !strings.Contains(snap.PromptInputs.ReviewDigest, "BRIEF-CHAIN-DIGEST") {
		t.Fatalf("input_snapshot must replay the injected digest, got %q", snap.PromptInputs.ReviewDigest)
	}

	// Judge compares against the seeded same-kind prev brief.
	if got := countJudgeCalls(router); got != 1 {
		t.Fatalf("judge must run once against the seeded prev brief, got %d", got)
	}
	var reviews []repository.TopicEnrichmentReview
	if err := repo.DB().Where("semantic_board_id = 8810 AND curr_result_id = ?", out.Result.ID).Find(&reviews).Error; err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].PrevResultID == nil || *reviews[0].PrevResultID != prevBrief.ID {
		t.Fatalf("review must anchor prev=%d, got %+v", prevBrief.ID, reviews)
	}
}

// 坏 JSON 两次 → 机械降级仍持久化（不再像旧链 analyze 失败那样中断）；
// 降级观察只来自真实 cards，无关系、无研究问题。
func TestEnrichBoard_BadJSONDegradesButPersists(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8803, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())

	router.addResponse("这不是JSON")
	router.addResponse(`{"summary": 还是坏`)

	out, err := orch.EnrichBoard(context.Background(), 8803)
	if err != nil {
		t.Fatalf("degraded brief must still persist: %v", err)
	}
	if got := countBriefCalls(router); got != 2 {
		t.Fatalf("bad JSON must retry exactly once (2 board_brief calls), got %d", got)
	}
	assertNoLegacyOps(t, router)

	var payload map[string]any
	if err := json.Unmarshal(out.Result.Sectors, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["degraded"] != true {
		t.Fatal("degraded brief must be flagged in sectors")
	}
	if rels := payload["relationships"].([]any); len(rels) != 0 {
		t.Fatalf("mechanical fallback must not invent relationships: %v", rels)
	}
	if qs := payload["research_questions"].([]any); len(qs) != 0 {
		t.Fatalf("mechanical fallback must not invent questions: %v", qs)
	}
	obs := payload["observations"].([]any)
	if len(obs) != 1 || obs[0].(map[string]any)["lane_id"].(float64) != 901 {
		t.Fatalf("fallback observations from real cards only: %v", obs)
	}
	// 降级状态写进 input_snapshot.generation。
	var snap map[string]any
	_ = json.Unmarshal(out.Result.InputSnapshot, &snap)
	gen := snap["generation"].(map[string]any)
	if gen["degraded"] != true || gen["attempts"].(float64) != 2 {
		t.Fatalf("generation meta must record degradation: %v", gen)
	}
	var n int
	_ = db.Raw("SELECT COUNT(*) FROM topic_enrichment_result WHERE semantic_board_id = 8803").Scan(&n).Error
	if n != 1 {
		t.Fatalf("exactly one persisted brief, got %d", n)
	}
}

// 全 sparse：诚实素材不足简报（零观察/零关系/零研究问题），单次 LLM 调用。
func TestEnrichBoard_AllSparseHonestBrief(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	seedBoardLane(t, repo, 901, 8804, "泳道甲") // no lifeline/sections/description → FactsSource none

	router.addResponse(`{"summary":"该板块近期无可观察素材。","observations":[],"relationships":[],
		"uncertainties":[{"question":"动向如何","why_uncertain":"无素材","needed_evidence":"等待命中"}],
		"research_questions":[{"id":"q1","question":"不该生成","rationale":"sparse","related_lane_ids":[901]}]}`)

	out, err := orch.EnrichBoard(context.Background(), 8804)
	if err != nil {
		t.Fatalf("sparse brief: %v", err)
	}
	if got := countBriefCalls(router); got != 1 {
		t.Fatalf("valid sparse brief is a single call, got %d", got)
	}
	assertNoLegacyOps(t, router)

	var payload map[string]any
	_ = json.Unmarshal(out.Result.Sectors, &payload)
	if qs := payload["research_questions"].([]any); len(qs) != 0 {
		t.Fatalf("all-sparse must not carry research questions: %v", qs)
	}
	if obs := payload["observations"].([]any); len(obs) != 0 {
		t.Fatalf("all-sparse observations stay empty: %v", obs)
	}
	if payload["summary"] == "" {
		t.Fatal("honest summary required")
	}
}

// 无活跃泳道 → 拒绝，零 LLM 调用，零 result 行。
func TestEnrichBoard_NoActiveLanesRejected(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	if _, err := orch.EnrichBoard(context.Background(), 8805); err == nil {
		t.Fatal("board without active lanes must be rejected")
	}
	if len(router.Calls) != 0 {
		t.Fatal("no LLM calls for lane-less board")
	}
	var n int
	_ = repo.DB().Raw("SELECT COUNT(*) FROM topic_enrichment_result WHERE semantic_board_id = 8805").Scan(&n).Error
	if n != 0 {
		t.Fatalf("no result rows expected, got %d", n)
	}
}

// 6.1 旧写链退场契约：新 trigger 调用链中旧 Operation 计数恒 0，且
// 落库只可能是 board_brief —— 旧链入口（EnrichBoardLegacyAnalysis）已从
// 生产代码删除，handler 唯一能触达的编排就是 EnrichBoard。
func TestEnrichBoard_TriggerChainNeverRunsLegacyWritePath(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	// 注意独占 board id：本测试断言落库行数绝对值，不得与其他 EnrichBoard 测试
	// 共用（避免残留行污染计数）。
	const zeroOpBoardID = 8809
	seedBoardLane(t, repo, 901, zeroOpBoardID, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容：一期产能落地", time.Now())
	router.addResponse(validBriefLLM)

	if _, err := orch.EnrichBoard(context.Background(), zeroOpBoardID); err != nil {
		t.Fatalf("EnrichBoard: %v", err)
	}

	// 旧链三个 Operation 的计数必须为 0：board_interpret（命题生成）、
	// tool_use（旧 board 研究方向 agent loop）、analyze（论文式 board 分析）。
	ops := map[string]int{}
	for _, c := range router.Calls {
		ops[c.Operation]++
	}
	for _, legacyOp := range []string{
		"data_enrichment.board_interpret",
		"data_enrichment.tool_use",
		"data_enrichment.analyze",
	} {
		if ops[legacyOp] != 0 {
			t.Fatalf("legacy operation %q must never run on the trigger chain, got %d calls", legacyOp, ops[legacyOp])
		}
	}
	if got := countBriefCalls(router); got != 1 {
		t.Fatalf("exactly one board_brief call expected, got %d", got)
	}

	// 落库只可能是 board_brief：本板块不允许出现任何其它 result_kind。
	var nonBrief int
	if err := db.Raw(`SELECT COUNT(*) FROM topic_enrichment_result
		WHERE semantic_board_id = ? AND result_kind <> 'board_brief'`, zeroOpBoardID).Scan(&nonBrief).Error; err != nil {
		t.Fatalf("count non-brief rows: %v", err)
	}
	if nonBrief != 0 {
		t.Fatalf("trigger chain must only persist board_brief, found %d other-kind rows", nonBrief)
	}
	var briefRows int
	if err := db.Raw(`SELECT COUNT(*) FROM topic_enrichment_result
		WHERE semantic_board_id = ? AND result_kind = 'board_brief'`, zeroOpBoardID).Scan(&briefRows).Error; err != nil {
		t.Fatalf("count brief rows: %v", err)
	}
	if briefRows != 1 {
		t.Fatalf("exactly one board_brief row expected, got %d", briefRows)
	}
}

// 3.5 契约：judge 失败（LLM 输出坏 JSON）按 review non-fatal 纪律降级——只记
// 日志，当前简报仍已落库且返回成功，无 review 行，lifeline 不动。
func TestEnrichBoard_ReviewJudgeErrorKeepsBrief(t *testing.T) {
	orch, router, repo := newEnrichBoardOrch(t, true)
	db := repo.DB()
	seedBoardLane(t, repo, 901, 8812, "泳道甲")
	seedWeekLifelinePG(t, repo, 901, "2026-W34", "周内容", time.Now())

	router.addResponse(validBriefLLM)
	out1, err := orch.EnrichBoard(context.Background(), 8812)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := snapshotLifeline(t, db, 901)

	router.addResponse(validBriefLLM)
	router.addResponse("judge 输出根本不是 JSON") // judge parse error → non-fatal
	out2, err := orch.EnrichBoard(context.Background(), 8812)
	if err != nil {
		t.Fatalf("judge failure must not fail the brief run: %v", err)
	}
	if out2.Result == nil || out2.Result.ID == 0 {
		t.Fatal("current brief must survive the judge failure")
	}
	if out2.Review != nil {
		t.Fatal("no review may surface when the judge errors")
	}
	if got := countJudgeCalls(router); got != 1 {
		t.Fatalf("judge must have been attempted once, got %d", got)
	}
	var nReview int
	_ = db.Raw("SELECT COUNT(*) FROM topic_enrichment_review WHERE semantic_board_id = 8812").Scan(&nReview).Error
	if nReview != 0 {
		t.Fatalf("no review rows after judge error, got %d", nReview)
	}
	var nResults int
	_ = db.Raw("SELECT COUNT(*) FROM topic_enrichment_result WHERE semantic_board_id = 8812").Scan(&nResults).Error
	if nResults != 2 {
		t.Fatalf("both briefs must persist (immutable snapshots), got %d", nResults)
	}
	if out2.Result.ID <= out1.Result.ID {
		t.Fatal("second brief must be a new row")
	}
	requireSameLifeline(t, before, snapshotLifeline(t, db, 901))
}
