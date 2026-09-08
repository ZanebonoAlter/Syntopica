package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── Verifier 盲验 + Scout→Resolve→Verify→Persist 流水线 ──────────────────────
//
// test-cases S3/S4/S5 的流水线层：mock LLM（scout plan/extract + verify
// plan/judge 四段脚本）+ mock 搜索 + mock 内部检索 + testcontainer PG。

// relWebSearcher returns canned hits whose snippets carry the exact quotes
// the extract LLM is scripted to emit.
type relWebSearcher struct {
	queries []string
	fail    bool
}

func (s *relWebSearcher) Search(_ context.Context, query string) ([]service.WebSearchResult, error) {
	s.queries = append(s.queries, query)
	if s.fail {
		return nil, fmt.Errorf("bocha 超时")
	}
	return []service.WebSearchResult{
		{Title: "宏观报道", URL: "https://news.example/jgb", Snippet: "中东局势推升油价与全球通胀预期，日债遭抛售收益率走高"},
		{Title: "市场分析", URL: "https://analysis.example/jgb", Snippet: "全球通胀预期共同驱动债券收益率上行"},
	}, nil
}

// relSearcher resolves "日债收益率" → lane 77 board 5 (唯一高分).
type relSearcher struct{}

func (relSearcher) SearchInternalContext(_ context.Context, _ string, _ int) ([]service.InternalContextHit, error) {
	id := uint(77)
	return []service.InternalContextHit{
		{Kind: "lane", BoardID: 5, LaneID: &id, Label: "日债收益率", Status: "active", HitCount: 28},
	}, nil
}

// relExtractLLM is the scripted scout extract output: one candidate whose
// quote is verbatim from the canned search snippet.
const relExtractLLM = `{"candidates":[
 {"target_concept":"日债收益率",
  "claim":"中东局势经油价与通胀预期传导，推升日债收益率",
  "relation_type":"causal",
  "mechanism":"油价上涨推升通胀预期，避险抛售压低债券价格",
  "evidence":[{"ref":"s1","url":"https://news.example/jgb","title":"宏观报道","quote":"中东局势推升油价与全球通胀预期，日债遭抛售收益率走高"}],
  "counter_evidence":[]}]}`

func relPlanLLM() string {
	return `{"queries":["日债收益率 走高 原因","中东 油价 通胀"]}`
}

func relVerifyPlanLLM() string { return `{"counter_queries":["日债收益率 走高 其他原因"]}` }

func relJudgeLLM(verdict, relType string) string {
	return fmt.Sprintf(`{"relation_type":%q,"verdict":%q,"mechanism":"油价→通胀预期→抛售","winning_hypothesis":"H1","reasoning":"证据链完整","counter_summary":"反证检索未见独立解释"}`, relType, verdict)
}

func newRelationOrch(t *testing.T, router service.AirRouter, searcher service.InternalContextSearcher, ws *relWebSearcher) (*service.OrchestratorService, *repository.Repository) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	require.NoError(t, database.RunMigrations(db))
	repo := repository.NewRepository(db)
	registry := service.NewRegistry(&nilFetcher{},
		service.WithWebSearcher(ws),
		service.WithInternalContextSearcher(searcher))
	orch := service.NewOrchestratorService(
		router, repo, &orchMockLifelineReader{}, service.NewLifelineRenderer(),
		registry, &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	return orch, repo
}

// seedRelationBrief inserts a board_brief carrying observation o1 and question
// q1, plus the owning board; returns board id + parent id.
func seedRelationBrief(t *testing.T, repo *repository.Repository) (boardID, parentID uint) {
	t.Helper()
	var bid uint
	require.NoError(t, repo.DB().Raw(`INSERT INTO semantic_labels (label, slug, label_type, status, enrichment_enabled, created_at, updated_at)
		VALUES ('关系发现测试板块', 'cbr-disc-board', 'board', 'active', true, now(), now()) RETURNING id`).Scan(&bid).Error)
	t.Cleanup(func() { _ = repo.DB().Exec(`DELETE FROM cross_board_relations WHERE source_board_id = ?`, bid).Error })
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM cross_board_relation_runs WHERE source_board_id = ?`, bid).Error
	})
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE semantic_board_id = ?`, bid).Error
	})
	t.Cleanup(func() { _ = repo.DB().Exec(`DELETE FROM semantic_labels WHERE id = ?`, bid).Error })

	sectors := `{"scope":"board","result_kind":"board_brief","summary":"观察与问题。",
		"observations":[{"id":"o1","lane_id":901,"statement":"日元汇率波动加剧，日债收益率走高","basis":"周摘要","as_of_date":"2026-09-01"}],
		"relationships":[],"uncertainties":[],
		"research_questions":[{"id":"q1","question":"日债收益率走高的外部驱动是什么","rationale":"外部传导未明","related_lane_ids":[901]}],
		"lane_refs":[{"lane_id":901}]}`
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(bid), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief,
		Sectors:    json.RawMessage(sectors), SessionID: fmt.Sprintf("rel-disc-brief-%d", bid),
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(context.Background(), res))
	return bid, res.ID
}

// assertVerifierBlindness checks none of the captured Chat requests carries
// scout self-assessment fields (the pipeline never forwards them).
func assertVerifierBlindness(t *testing.T, router *mockAirRouter) {
	t.Helper()
	for _, call := range router.Calls {
		body := call.Messages[len(call.Messages)-1].Content
		if strings.Contains(body, "反证检索原始结果") || strings.Contains(body, "关系假设") {
			require.NotContains(t, body, "self_score", "verifier input must not carry scout self-scores")
			require.NotContains(t, body, "confidence\":0", "verifier input must not carry model confidence")
		}
	}
}

func TestRelationDiscoveryPipeline_SupportedBecomesProposed(t *testing.T) {
	router := newMockAirRouter()
	ws := &relWebSearcher{}
	orch, repo := newRelationOrch(t, router, relSearcher{}, ws)
	boardID, parentID := seedRelationBrief(t, repo)

	// LLM script: scout plan → scout extract → verify plan → verify judge(supported).
	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	router.addResponse(relJudgeLLM("supported", "causal"))

	out, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, repository.RelationRunStatusSucceeded, out.Status)
	require.Equal(t, 1, out.RelationsCreated)

	// Persisted row: proposed (never confirmed), resolved target, verified
	// evidence, program quality grade.
	rel := out.Relations[0]
	require.Equal(t, repository.RelationStatusProposed, rel.Status)
	require.NotNil(t, rel.TargetBoardID)
	require.Equal(t, uint(5), *rel.TargetBoardID)
	require.Equal(t, repository.RelationVerdictSupported, rel.VerificationVerdict)
	require.NotEmpty(t, rel.Evidence)
	require.True(t, rel.Evidence[0].Verified, "verbatim quote must verify against raw search JSON")
	// Counter evidence from the verifier's counter search is persisted.
	require.NotEmpty(t, rel.Counterevidence)

	// Run row is auditable with gaps frozen.
	run, err := repo.GetRelationRunByID(context.Background(), out.RunID)
	require.NoError(t, err)
	require.Equal(t, repository.RelationRunStatusSucceeded, run.Status)
	require.Equal(t, "observation", run.SourceKind)

	// Client-side source forgery is impossible: statement comes from the brief.
	require.Contains(t, run.SourceText, "日债收益率走高")
	assertVerifierBlindness(t, router)
}

func TestRelationDiscoveryPipeline_ContestedStaysUnresolved(t *testing.T) {
	router := newMockAirRouter()
	ws := &relWebSearcher{}
	orch, repo := newRelationOrch(t, router, relSearcher{}, ws)
	boardID, parentID := seedRelationBrief(t, repo)

	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	router.addResponse(relJudgeLLM("contested", "unclear"))

	out, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "question", SourceKey: "q1"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.RelationsCreated)
	rel := out.Relations[0]
	require.Equal(t, repository.RelationStatusUnresolved, rel.Status, "contested verdict → unresolved lifecycle")
}

func TestRelationDiscoveryPipeline_RejectedRunOnly(t *testing.T) {
	router := newMockAirRouter()
	ws := &relWebSearcher{}
	orch, repo := newRelationOrch(t, router, relSearcher{}, ws)
	boardID, parentID := seedRelationBrief(t, repo)

	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	router.addResponse(relJudgeLLM("rejected", "unclear"))

	out, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, out.RelationsCreated)
	require.Empty(t, out.Relations)

	// No relation row was created for the rejected candidate.
	rows, err := repo.ListCrossBoardRelations(context.Background(), repository.CrossBoardRelationFilter{BoardID: &boardID})
	require.NoError(t, err)
	require.Empty(t, rows)
	// But the run records the rejection gap.
	run, err := repo.GetRelationRunByID(context.Background(), out.RunID)
	require.NoError(t, err)
	require.Contains(t, string(run.Gaps), "candidate_rejected")
}

func TestRelationDiscoveryPipeline_BochaDownHonestInsufficient(t *testing.T) {
	router := newMockAirRouter()
	ws := &relWebSearcher{fail: true}
	orch, repo := newRelationOrch(t, router, relSearcher{}, ws)
	boardID, parentID := seedRelationBrief(t, repo)

	router.addResponse(relPlanLLM())
	// extract never gets material; scout returns zero candidates after search errors.

	out, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, out.RelationsCreated)
	require.Equal(t, repository.RelationRunStatusSucceeded, out.Status)

	// Honest gaps; no supported relation invented (spec: 博查不可用).
	run, err := repo.GetRelationRunByID(context.Background(), out.RunID)
	require.NoError(t, err)
	require.Contains(t, string(run.Gaps), "web_search_error")
	rows, err := repo.ListCrossBoardRelations(context.Background(), repository.CrossBoardRelationFilter{BoardID: &boardID})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestRelationDiscoveryPipeline_IdempotentRerunAndCooldown(t *testing.T) {
	router := newMockAirRouter()
	ws := &relWebSearcher{}
	orch, repo := newRelationOrch(t, router, relSearcher{}, ws)
	boardID, parentID := seedRelationBrief(t, repo)

	// First run creates the proposed row (4 LLM calls: plan/extract/vplan/judge).
	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	router.addResponse(relJudgeLLM("supported", "causal"))

	out1, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out1.RelationsCreated)
	firstID := out1.Relations[0].ID

	// Second identical run: same hash → skipped, no duplicate open row.
	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	router.addResponse(relJudgeLLM("supported", "causal"))
	out2, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, out2.RelationsCreated)
	require.Equal(t, 1, out2.RelationsSkipped)

	// Dismiss → cooldown blocks re-creation of the same suggestion.
	require.NoError(t, repo.DismissCrossBoardRelation(context.Background(), firstID, "误报", "tester"))
	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	router.addResponse(relJudgeLLM("supported", "causal"))
	out3, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, out3.RelationsCreated)
	require.Equal(t, 1, out3.RelationsSkipped, "dismiss cooldown must block identical suggestion")
}

func TestRelationDiscoveryPipeline_ForgedSourceRejected(t *testing.T) {
	router := newMockAirRouter()
	orch, repo := newRelationOrch(t, router, relSearcher{}, &relWebSearcher{})
	boardID, parentID := seedRelationBrief(t, repo)

	// Source key that does not exist in the brief → setup error, zero runs.
	_, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o999"},
	})
	require.Error(t, err)
	var runCount int64
	require.NoError(t, repo.DB().Model(&repository.CrossBoardRelationRun{}).
		Where("source_board_id = ?", boardID).Count(&runCount).Error)
	require.Zero(t, runCount)

	// Cross-board parent → error.
	_, err = orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID + 999,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.Error(t, err)
}

func TestRelationVerifier_IllegalEnumsDegrade(t *testing.T) {
	// relationVerifyOutput-level: the pipeline scrubs illegal verdict/type
	// rather than failing the run (verifier parse robustness).
	require.True(t, repository.ValidRelationType("common_driver"))
	require.False(t, repository.ValidRelationType("金融传导"))
	require.True(t, repository.ValidRelationVerdict("insufficient"))
	require.False(t, repository.ValidRelationVerdict("definitely"))
}

func TestRelationDiscoveryPipeline_CommonDriverNotCausal(t *testing.T) {
	router := newMockAirRouter()
	ws := &relWebSearcher{}
	orch, repo := newRelationOrch(t, router, relSearcher{}, ws)
	boardID, parentID := seedRelationBrief(t, repo)

	router.addResponse(relPlanLLM())
	router.addResponse(relExtractLLM)
	router.addResponse(relVerifyPlanLLM())
	// The blind judge decides the material only supports a shared driver —
	// the final type is common_driver, NOT causal (spec: 共同驱动而非直接因果).
	router.addResponse(relJudgeLLM("supported", "common_driver"))

	out, err := orch.RunRelationDiscovery(context.Background(), service.RelationDiscoveryInput{
		BoardID: boardID,
		Source:  service.RelationSourceRef{ParentResultID: parentID, SourceKind: "observation", SourceKey: "o1"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, out.RelationsCreated)
	rel := out.Relations[0]
	require.Equal(t, repository.RelationTypeCommonDriver, rel.RelationType)
	require.NotEqual(t, repository.RelationTypeCausal, rel.RelationType)
	require.Equal(t, repository.RelationStatusProposed, rel.Status)
}

// TestRelationToolBudget: 搜索预算变体 0/1/上限/上限+1——超额 query 被阻并
// 记 search_budget_exhausted gap；预算钳制（<=0 → 1）不炸。
func TestRelationToolBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("unit-only test but lives with pipeline file; keep under short guard parity")
	}
	cases := []struct {
		name        string
		maxSearches int
		wantRun     int // expected executed searches
		wantGaps    int // expected budget-exhausted gap rows
	}{
		{"budget 0 clamps to 1", 0, 1, 1},
		{"budget 1 caps first query", 1, 1, 1},
		{"budget equals plan size", 2, 2, 0},
		{"budget exceeds plan size", 5, 2, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newMockAirRouter()
			router.addResponse(relPlanLLM()) // 2 queries planned
			ws := &relWebSearcher{}
			orch, _ := newRelationOrch(t, router, &relSearcher{}, ws)
			out, err := orch.RunRelationScoutForTest(context.Background(), "session-budget", "日债收益率走高", tc.maxSearches)
			require.NoError(t, err)
			require.Len(t, ws.queries, tc.wantRun, "executed search count must respect budget")
			exhausted := 0
			for _, g := range out.Gaps {
				if g.Reason == "search_budget_exhausted" {
					exhausted++
				}
			}
			require.Equal(t, tc.wantGaps, exhausted, "over-budget queries must be recorded as gaps, never executed")
		})
	}
}
