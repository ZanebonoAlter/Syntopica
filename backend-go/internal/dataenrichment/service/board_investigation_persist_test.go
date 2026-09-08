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

// ── InvestigateBoardQuestion 编排 + 持久化（tasks 4.6，test-cases M6）────────
//
// testcontainer PG 全链路（mock LLM + stub 工具）：
//   - 正常落库：kind=board_investigation、parent、question_key、调查 schema、
//     tool_calls 完整有序、input_snapshot 可回放；ops/session 一致
//   - 跨 board / legacy / 不存在 parent / 停用板块：error 且 0 LLM
//   - 一 brief 多调查（不同问题独立、同题重跑允许）
//   - custom 问题：原文保真 + 规范化 question_key
//   - synth 失败 = 0 行、父简报 sectors 字节不变
//   - 方法软删除后历史可回放；budget/修辞舍弃机码落快照且不泄原文

// invLaneRenderer / invWebSearcher: 调查研究循环的 stub 工具后端。
type invLaneRenderer struct{}

func (invLaneRenderer) RenderLaneDetail(_ context.Context, laneID uint, _ int) (string, error) {
	return fmt.Sprintf("泳道%d近期演进：招标公告与产能进展（周摘要）", laneID), nil
}

type invWebSearcher struct{}

func (invWebSearcher) Search(_ context.Context, _ string) ([]service.WebSearchResult, error) {
	return []service.WebSearchResult{{Title: "基金公告", URL: "https://example.com/a", Snippet: "基金公告原文摘录ABC"}}, nil
}

func newInvestigationOrch(t *testing.T, enabled bool) (*service.OrchestratorService, *mockAirRouter, *repository.Repository) {
	router := newMockAirRouter()
	orch, repo := newInvestigationOrchWithRouter(t, enabled, router)
	return orch, router, repo
}

// newInvestigationOrchWithRouter builds the investigation orchestrator around
// a caller-supplied router (ctx-cancel tests wrap the scripted mock with a
// ctx-respecting decorator instead of mutating the shared fixture).
func newInvestigationOrchWithRouter(t *testing.T, enabled bool, router service.AirRouter) (*service.OrchestratorService, *repository.Repository) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	repo := repository.NewRepository(db)
	registry := service.NewRegistry(&nilFetcher{},
		service.WithLaneDetailRenderer(invLaneRenderer{}),
		service.WithWebSearcher(invWebSearcher{}))
	orch := service.NewOrchestratorService(
		router, repo, &orchMockLifelineReader{}, service.NewLifelineRenderer(),
		registry, &mockBoardConfigReader{cfg: service.DefaultBoardConfig()}, testCap,
	)
	orch.SetBoardConfigResolver(&mockBoardResolver{enabled: enabled})
	return orch, repo
}

// invBriefSectors：lanes 901/902 + 候选问题 q1 的合法 board_brief sectors。
const invBriefSectors = `{"scope":"board","result_kind":"board_brief","summary":"两条泳道各有进展，暂未发现统一关系。","observations":[{"id":"o1","lane_id":901,"statement":"产能落地","basis":"周摘要","as_of_date":"2026-08-26"},{"id":"o2","lane_id":902,"statement":"招标启动","basis":"月摘要","as_of_date":"2026-08-25"}],"relationships":[],"uncertainties":[{"question":"资金是否同源","why_uncertain":"明细缺失","needed_evidence":"资金数据"}],"research_questions":[{"id":"q1","question":"两条泳道是否由同一资金驱动","rationale":"若同源将改变优先级","related_lane_ids":[901,902]}],"lane_refs":[{"lane_id":901,"note":"泳道甲"},{"lane_id":902,"note":"泳道乙"}]}`

func seedInvBrief(t *testing.T, repo *repository.Repository, boardID uint) *repository.TopicEnrichmentResult {
	t.Helper()
	res := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief,
		Sectors:    json.RawMessage(invBriefSectors), SessionID: fmt.Sprintf("inv-seed-brief-%d", boardID),
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(context.Background(), res))
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE parent_result_id = ?`, res.ID).Error
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE id = ?`, res.ID).Error
	})
	return res
}

// invHypothesesLLM：h0 零假设 + h1/h2 非零（研究纪律要求 h1/h2 counter）。
const invHypothesesLLM = `{"hypotheses":[
 {"id":"h0","label":"无统一机制，可由各自独立因素分别解释","is_null":true,"support_needed":["共同机制证据"],"disconfirm_needed":["独立解释"],"scope":"板块"},
 {"id":"h1","label":"同一产业基金推动产能与招标","is_null":false,"support_needed":["基金公告同时提及两泳道"],"disconfirm_needed":["资金来源明细独立"],"scope":"近三个月"},
 {"id":"h2","label":"政策补贴周期同步带动","is_null":false,"support_needed":["补贴文本覆盖两泳道"],"disconfirm_needed":["时间线不重合"],"scope":"政策周期"}]}`

// invSynthesisLLM：h0 plausible 最可信、h1 supported(medium)、h2 insufficient；
// e1 web（quote 摘自 invWebSearcher snippet）、e2 lane 901（白名单内）。
const invSynthesisLLM = `{"hypotheses":[
 {"id":"h0","label":"无统一机制，变化可分别解释","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e2"],"counter_evidence":[],"gaps":[]},
 {"id":"h1","label":"产业基金推动两泳道","is_null":false,"assessment":"supported","confidence":"medium","scope":"近三月","support_evidence":["e1"],"counter_evidence":[],"gaps":[]},
 {"id":"h2","label":"政策补贴同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":["缺补贴时间线"]}],
 "conclusion":{"summary":"h1 获基金公告支持但仍有缺口","confidence":"medium","scope":"两条泳道","boundary":"资金明细未完全核实"},
 "evidence_chain":[
  {"id":"e1","source_type":"web","url":"https://example.com/a","quote":"基金公告原文摘录ABC","institution":"示例研究所","date":"2026-08-20","supports":["h1"],"counters":[]},
  {"id":"e2","source_type":"lane","ref":"901","lane_note":"产能与招标详情","supports":["h0"],"counters":[]}],
 "lane_refs":[{"lane_id":901,"note":"主泳道"},{"lane_id":902,"note":"对照泳道"}]}`

// addInvChain appends one full investigation chain (empty method library):
// hypothesize → research (neutral + counter h1 + counter h2 + finish) → synthesize.
func addInvChain(router *mockAirRouter, synthResp string) {
	router.addResponse(invHypothesesLLM)
	router.addResponse(`{"action":"call_tool","thought":"内部核查","tool":"get_lane_detail","args":{"lane_id":901},"purpose":"neutral","hypothesis_ids":[]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h1","tool":"web_search","args":{"query":"产业基金 独立资金来源明细"},"purpose":"counter","hypothesis_ids":["h1"]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h2","tool":"web_search","args":{"query":"补贴政策 时间线对照"},"purpose":"counter","hypothesis_ids":["h2"]}`)
	router.addResponse(`{"action":"finish","thought":"纪律已补齐","summary":"h1 有基金公告线索；h2 缺补贴时间线；中性核查完成。"}`)
	router.addResponse(synthResp)
}

// addInvChainWithSelector appends a chain whose method selector picks the
// given method ids (selector → hypothesize → research → synthesize).
func addInvChainWithSelector(router *mockAirRouter, synthResp string, idReasons ...[2]string) {
	entries := make([]string, 0, len(idReasons))
	for _, ir := range idReasons {
		entries = append(entries, fmt.Sprintf(`{"id":%s,"reason":"%s"}`, ir[0], ir[1]))
	}
	router.addResponse(`{"selected":[` + strings.Join(entries, ",") + `]}`)
	addInvChain(router, synthResp)
}

func invCallOps(router *mockAirRouter) []string {
	ops := make([]string, 0, len(router.Calls))
	for _, c := range router.Calls {
		ops = append(ops, c.Operation)
	}
	return ops
}

func invSectorsMap(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// ── 正常落库：行字段、调查 schema、tool_calls、input_snapshot、ops/session ──

func TestInvestigateBoardQuestion_PersistsFullInvestigation(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95101)
	brief := seedInvBrief(t, repo, boardID)
	parentBefore := string(brief.Sectors)
	addInvChain(router, invSynthesisLLM)

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.NotNil(t, out.Result)

	// ops 顺序与 session 一致：hypothesize → tool_use×4 → synthesize；
	// 不跑 freshness/cards/board_brief/review（不重复父简报链）。
	wantOps := []string{
		"data_enrichment.board_hypothesize",
		"data_enrichment.tool_use", "data_enrichment.tool_use", "data_enrichment.tool_use", "data_enrichment.tool_use",
		"data_enrichment.board_synthesize",
	}
	require.Equal(t, wantOps, invCallOps(router))
	// 6.1 旧写链退场契约（显式计数，独立于上方的精确序列断言）：调查链
	// 不得触达旧论文链任何环节 —— board_interpret（命题生成）与
	// analyze（论文式 board 分析）恒 0。调查自己的共享研究循环用的是
	// data_enrichment.tool_use（spec 4.4 契约），不在禁用集内。
	legacyOps := map[string]int{
		"data_enrichment.board_interpret": 0,
		"data_enrichment.analyze":         0,
	}
	for _, c := range router.Calls {
		if _, banned := legacyOps[c.Operation]; banned {
			legacyOps[c.Operation]++
		}
	}
	for op, n := range legacyOps {
		require.Zero(t, n, "investigation chain must never run legacy op %s", op)
	}
	sessions := map[string]bool{}
	for i, c := range router.Calls {
		require.True(t, strings.HasPrefix(c.SessionID, fmt.Sprintf("data_enrichment_board_%d_", boardID)),
			"call %d session prefix: %q", i, c.SessionID)
		sessions[c.SessionID] = true
	}
	require.Len(t, sessions, 1, "one investigation = one session")

	// 重新读库断言持久化形状。
	var row repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", out.Result.ID).First(&row).Error)
	require.Equal(t, "board", row.AnalysisScope)
	require.Equal(t, repository.ResultKindBoardInvestigation, row.ResultKind)
	require.NotNil(t, row.ParentResultID)
	require.Equal(t, brief.ID, *row.ParentResultID)
	require.NotNil(t, row.QuestionKey)
	require.Equal(t, repository.ComputeQuestionKey("两条泳道是否由同一资金驱动"), *row.QuestionKey)

	// sectors 调查 schema。
	sectors := invSectorsMap(t, row.Sectors)
	require.Equal(t, "board", sectors["scope"])
	require.Equal(t, "board_investigation", sectors["result_kind"])
	require.Equal(t, float64(brief.ID), sectors["parent_briefing_id"])
	question := sectors["question"].(map[string]any)
	require.Equal(t, "两条泳道是否由同一资金驱动", question["text"])
	require.Equal(t, "generated", question["source"])
	require.Len(t, sectors["hypotheses"], 3)
	require.Contains(t, sectors, "conclusion")
	require.Len(t, sectors["evidence_chain"], 2)
	require.Len(t, sectors["lane_refs"], 2)
	require.Empty(t, sectors["method_refs"])
	concl := sectors["conclusion"].(map[string]any)
	for _, k := range []string{"summary", "confidence", "scope", "boundary"} {
		require.Contains(t, concl, k)
	}

	// tool_calls：共享研究完整有序记录（含 purpose/outcome/result_full）。
	var toolCalls []map[string]any
	require.NoError(t, json.Unmarshal(row.ToolCalls, &toolCalls))
	require.Len(t, toolCalls, 3)
	first := toolCalls[0]
	require.Equal(t, "neutral", first["purpose"])
	require.Equal(t, "ok", first["outcome"])
	require.NotEmpty(t, first["result_full"])
	require.Equal(t, []any{"h1"}, toolCalls[1]["hypothesis_ids"])

	// input_snapshot 回放要素齐全。
	snap := invSectorsMap(t, row.InputSnapshot)
	require.Equal(t, float64(brief.ID), snap["parent_brief_id"])
	parentRaw, err := json.Marshal(snap["parent_sectors"])
	require.NoError(t, err)
	wantRaw, err := json.Marshal(json.RawMessage(invBriefSectors))
	require.NoError(t, err)
	require.JSONEq(t, string(wantRaw), string(parentRaw), "snapshot parent sectors must be the raw parent bytes")
	require.Contains(t, snap["parent_projection"], "两条泳道各有进展")
	require.Equal(t, repository.ComputeQuestionKey("两条泳道是否由同一资金驱动"), snap["question_key"])
	lanes := snap["lane_whitelist"].([]any)
	require.ElementsMatch(t, []any{float64(901), float64(902)}, lanes)
	require.Contains(t, snap, "methods")
	require.Contains(t, snap, "method_prompt")
	require.Contains(t, snap, "method_cards")
	require.Contains(t, snap, "method_refs")
	require.Contains(t, snap, "evidence_needs")
	initial := snap["initial_hypotheses"].(map[string]any)
	require.Len(t, initial["hypotheses"], 3)
	research := snap["research"].(map[string]any)
	require.Equal(t, true, research["coverage"].(map[string]any)["neutral_attempted"])
	require.NotEmpty(t, research["final_data"])
	synth := snap["synthesis"].(map[string]any)
	require.Equal(t, float64(1), synth["attempts"])

	// 父简报语义不变（PG jsonb 读取会重新序列化字节，逐字节比较不可行；
	// 调查链不写父行，语义等价即“不变”）。
	var parentAfter repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", brief.ID).First(&parentAfter).Error)
	wantParent, err := json.Marshal(json.RawMessage(parentBefore))
	require.NoError(t, err)
	gotParent, err := json.Marshal(parentAfter.Sectors)
	require.NoError(t, err)
	require.JSONEq(t, string(wantParent), string(gotParent))
}

// ── 预检失败：0 LLM ─────────────────────────────────────────────────────────

func TestInvestigateBoardQuestion_InvalidParentOrBoardRejectedZeroLLM(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95111)
	const otherBoardID = uint(95112)
	brief := seedInvBrief(t, repo, boardID)
	legacy := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindLegacyBoardAnalysis, SessionID: "inv-seed-legacy",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(context.Background(), legacy))
	t.Cleanup(func() { _ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE id = ?`, legacy.ID).Error })

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	cases := []struct {
		name     string
		boardID  uint
		parentID uint
		question service.BoardInvestigationQuestion
	}{
		{"cross-board parent", otherBoardID, brief.ID, q},
		{"legacy parent", boardID, legacy.ID, q},
		{"nonexistent parent", boardID, 99999999, q},
		{"illegal question source", boardID, brief.ID, service.BoardInvestigationQuestion{Text: "x", Source: "bogus"}},
	}
	for _, tc := range cases {
		_, err := orch.InvestigateBoardQuestion(context.Background(), tc.boardID, tc.parentID, tc.question)
		require.Error(t, err, tc.name)
	}
	require.Empty(t, router.Calls, "all pre-flight failures must happen with zero LLM calls")

	// 停用板块同样 0 LLM（同一进程缓存库，直接用同一 repo 播种）。
	orchDisabled, router2, _ := newInvestigationOrch(t, false)
	brief2 := seedInvBrief(t, repo, 95113)
	_, err := orchDisabled.InvestigateBoardQuestion(context.Background(), 95113, brief2.ID, q)
	require.Error(t, err)
	require.Empty(t, router2.Calls)
}

// ── 一 brief 多调查：不同问题独立、同题重跑允许 ──────────────────────────────

func TestInvestigateBoardQuestion_MultipleInvestigationsPerBrief(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95121)
	brief := seedInvBrief(t, repo, boardID)

	q1 := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	q2 := service.BoardInvestigationQuestion{ID: "q2", Text: "招标节奏是否影响产能排期", Source: "generated"}
	addInvChain(router, invSynthesisLLM)
	addInvChain(router, invSynthesisLLM)
	out1, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q1)
	require.NoError(t, err)
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q2)
	require.NoError(t, err)
	require.NotEqual(t, out1.Result.ID, out2.Result.ID)
	require.Equal(t, brief.ID, *out1.Result.ParentResultID)
	require.Equal(t, brief.ID, *out2.Result.ParentResultID)
	require.Equal(t, repository.ComputeQuestionKey(q1.Text), *out1.Result.QuestionKey)
	require.Equal(t, repository.ComputeQuestionKey(q2.Text), *out2.Result.QuestionKey)
	require.NotEqual(t, *out1.Result.QuestionKey, *out2.Result.QuestionKey)

	// 同题重跑：允许第三行、question_key 相同。
	addInvChain(router, invSynthesisLLM)
	out3, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q1)
	require.NoError(t, err)
	require.Equal(t, *out1.Result.QuestionKey, *out3.Result.QuestionKey)
	require.NotEqual(t, out1.Result.ID, out3.Result.ID)

	var children []repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("parent_result_id = ?", brief.ID).Find(&children).Error)
	require.Len(t, children, 3)
}

// ── custom 问题：原文保真 + 规范化 key ───────────────────────────────────────

func TestInvestigateBoardQuestion_CustomQuestionNormalizedKeyAndRawText(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95131)
	brief := seedInvBrief(t, repo, boardID)
	addInvChain(router, invSynthesisLLM)

	q := service.BoardInvestigationQuestion{Text: "  自填  问题：资金是否 同源？ ", Source: "custom"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)

	// Normalize 只 trim 两端：原文（含中间连续空白）完整保存。
	require.Equal(t, "自填  问题：资金是否 同源？", invSectorsMap(t, out.Result.Sectors)["question"].(map[string]any)["text"])
	// key = 折叠空白后文本的 hash（generated/custom 同一算法）。
	require.Equal(t, repository.ComputeQuestionKey("自填 问题：资金是否 同源？"), *out.Result.QuestionKey)
}

// ── synth 失败：0 行、父简报不变 ────────────────────────────────────────────

func TestInvestigateBoardQuestion_SynthesisFailureLeavesZeroRows(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95141)
	brief := seedInvBrief(t, repo, boardID)
	parentBefore := string(brief.Sectors)

	router.addResponse(invHypothesesLLM)
	router.addResponse(`{"action":"call_tool","thought":"内部核查","tool":"get_lane_detail","args":{"lane_id":901},"purpose":"neutral","hypothesis_ids":[]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h1","tool":"web_search","args":{"query":"产业基金 独立资金"},"purpose":"counter","hypothesis_ids":["h1"]}`)
	router.addResponse(`{"action":"call_tool","thought":"反证h2","tool":"web_search","args":{"query":"补贴时间线"},"purpose":"counter","hypothesis_ids":["h2"]}`)
	router.addResponse(`{"action":"finish","thought":"完成","summary":"素材已汇总。"}`)
	router.addResponse("坏JSON不是调查结论")
	router.addResponse(`{"hypotheses": 还是坏`)

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	_, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.Error(t, err)

	var count int64
	require.NoError(t, repo.DB().Model(&repository.TopicEnrichmentResult{}).
		Where("parent_result_id = ?", brief.ID).Count(&count).Error)
	require.Zero(t, count, "synthesis failure must leave zero investigation rows")

	var parentAfter repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", brief.ID).First(&parentAfter).Error)
	wantParent, err := json.Marshal(json.RawMessage(parentBefore))
	require.NoError(t, err)
	gotParent, err := json.Marshal(parentAfter.Sectors)
	require.NoError(t, err)
	require.JSONEq(t, string(wantParent), string(gotParent), "synthesis failure must not touch the parent brief")
}

// ── 方法软删除后历史调查可回放（M7.10）──────────────────────────────────────

func TestInvestigateBoardQuestion_MethodSoftDeletedHistoryReplayable(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95151)
	brief := seedInvBrief(t, repo, boardID)
	const methodContent = "检查传导链每一环：时间先后、独立来源、替代解释逐一核对。"
	m := seedInvMethod(t, repo, "inv-replayable-method", "因果链检验", methodContent)
	addInvChainWithSelector(router, invSynthesisLLM, [2]string{fmt.Sprintf("%d", m.ID), "适配"})

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)

	// 方法正文进入 hypothesize prompt（ai_call_logs 可回放）。
	hypothesizePrompt := ""
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.board_hypothesize" {
			hypothesizePrompt = c.Messages[0].Content
		}
	}
	require.Contains(t, hypothesizePrompt, methodContent)

	// 软删除方法卡。
	require.NoError(t, repo.DeleteAnalysisMethod(context.Background(), m.ID))

	var row repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", out.Result.ID).First(&row).Error)
	sectors := invSectorsMap(t, row.Sectors)
	refs := sectors["method_refs"].([]any)
	require.Len(t, refs, 1)
	ref := refs[0].(map[string]any)
	require.Equal(t, "因果链检验", ref["title"])
	require.Equal(t, service.AnalysisMethodContentHash(methodContent), ref["content_hash"])

	// snapshot 仍带实际注入正文与逐卡 trace（读取不依赖方法表）。
	snap := invSectorsMap(t, row.InputSnapshot)
	require.Contains(t, snap["method_prompt"], methodContent)
	cards := snap["method_cards"].([]any)
	require.Len(t, cards, 1)
	card := cards[0].(map[string]any)
	require.Equal(t, true, card["injected"])
	require.Equal(t, methodContent, card["injected_content"])
}

// ── budget/修辞舍弃机码落快照且不泄原文 ─────────────────────────────────────

func TestInvestigateBoardQuestion_MethodDropTraceInSnapshotNoLeak(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95161)
	brief := seedInvBrief(t, repo, boardID)

	// Run 1：选中 [超预算卡B, 正常卡A] → B 整卡 budget_exceeded，A 注入。
	normalContent := "常规步骤：逐环核对时间先后与独立来源。"
	oversizedMarker := strings.Repeat("OVERSIZED-CONTENT-MARKER-", 200) // 5200 runes > 4000 预算
	a := seedInvMethod(t, repo, "inv-budget-normal", "常规检验", normalContent)
	b := seedInvMethod(t, repo, "inv-budget-oversized", "超预算卡", oversizedMarker)
	addInvChainWithSelector(router, invSynthesisLLM,
		[2]string{fmt.Sprintf("%d", b.ID), "次适配"}, [2]string{fmt.Sprintf("%d", a.ID), "最适配"})

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out1, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	snapRaw1 := string(out1.Result.InputSnapshot)
	require.Contains(t, snapRaw1, "budget_exceeded")
	require.NotContains(t, snapRaw1, "OVERSIZED-CONTENT-MARKER-", "oversized card content must not leak into the snapshot")
	require.Contains(t, snapRaw1, normalContent)
	// 舍弃机码进 synthesize prompt（→ ai_call_logs）。
	synthPrompt := ""
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.board_synthesize" {
			synthPrompt = c.Messages[0].Content
		}
	}
	require.Contains(t, synthPrompt, "budget_exceeded")
	require.Contains(t, synthPrompt, normalContent)
	require.NotContains(t, synthPrompt, "OVERSIZED-CONTENT-MARKER-")

	// Run 2：纯修辞卡（清洗后为空）→ content_noncompliant，被过滤原文不落快照。
	rhetoricMarker := "每段结尾都要用金句收尾"
	rhetoricContent := rhetoricMarker + "\n写作时必须保持冷嘲的语气"
	c := seedInvMethod(t, repo, "inv-rhetoric-card", "修辞卡", rhetoricContent)
	addInvChainWithSelector(router, invSynthesisLLM, [2]string{fmt.Sprintf("%d", c.ID), "适配"})
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID,
		service.BoardInvestigationQuestion{ID: "q2", Text: "招标节奏是否影响产能排期", Source: "generated"})
	require.NoError(t, err)
	snapRaw2 := string(out2.Result.InputSnapshot)
	require.Contains(t, snapRaw2, "content_noncompliant")
	require.NotContains(t, snapRaw2, rhetoricMarker, "filtered rhetoric lines must not leak into the snapshot")
	require.NotContains(t, snapRaw2, "冷嘲")
	cards := invSectorsMap(t, out2.Result.InputSnapshot)["method_cards"].([]any)
	require.Len(t, cards, 1)
	card := cards[0].(map[string]any)
	require.Equal(t, false, card["injected"])
	require.Equal(t, "content_noncompliant", card["dropped_reason"])
	require.GreaterOrEqual(t, int(card["filtered_lines"].(float64)), 2)
	require.Empty(t, card["injected_content"])
	require.NotEmpty(t, card["reason_codes"])
}

func seedInvMethod(t *testing.T, repo *repository.Repository, name, title, content string) *repository.AnalysisMethod {
	t.Helper()
	m := &repository.AnalysisMethod{
		Name: name, Title: title, Summary: "适配传导检验", Content: content, Enabled: true,
		SelectionMeta: repository.AnalysisMethodSelectionMeta{
			ApplicableWhen:   []string{"怀疑存在跨泳道传导"},
			RequiredEvidence: []string{"两个独立来源"},
		},
	}
	require.NoError(t, repo.CreateAnalysisMethod(context.Background(), m))
	t.Cleanup(func() { _ = repo.DB().Unscoped().Where("name = ?", name).Delete(&repository.AnalysisMethod{}).Error })
	return m
}
