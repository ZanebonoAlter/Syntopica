package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
)

// ── InvestigateBoardQuestion 研究/综合韧性全链（tasks 4.4/4.5/4.6 补验收）────
//
// 研究层单元测试（board_investigation_research_test.go）已覆盖 partial result
// 的 gap 分类与 ctx 取消；这里补「编排 + PG 落库」链路的三块缺口：
//
//  1. 研究循环耗尽 maxAgentLoops=6 未 finish → 编排仍调用 board_synthesize
//     并成功落 board_investigation；input_snapshot.research 明确携带
//     max_loops + 按实际的 missing 纪律 gap + coverage；result.ToolCalls
//     保全完整顺序与 ResultFull。
//  2. 研究 LLM 非法输出 → partial + llm_error gap 仍可 synthesize 落库；
//     内部错误串（LLM 原始垃圾输出前缀）不得泄入 input_snapshot 的
//     gap/retry 字段。
//  3. 全链 ctx 取消（research 前 / synth 前）→ InvestigateBoardQuestion
//     返回 context 错误、0 investigation 行、父简报不动。用尊重 ctx 的
//     router wrapper 模拟真实 HTTP 客户端语义，不靠忽略 ctx 的 mock
//     造成假阳性。

// invKnownResearchGapReasons is the closed reason enum persisted in
// input_snapshot.research.gaps (stable machine codes, never error text).
var invKnownResearchGapReasons = map[string]bool{
	"tool_unavailable": true,
	"tool_error":       true,
	"missing_neutral":  true,
	"missing_counter":  true,
	"max_loops":        true,
	"llm_error":        true,
}

// invSynthesisLLMDowngraded（M13）：llm_error 场景纠错重试的正确响应——
// 零工具调用下 e1（web）不可核验被剔，h1 不得再 supported，降级 plausible
// 并只保留可存活的 e2（lane 白名单内）。
const invSynthesisLLMDowngraded = `{"hypotheses":[
 {"id":"h0","label":"无统一机制，变化可分别解释","is_null":true,"assessment":"plausible","confidence":"medium","scope":"板块","support_evidence":["e2"],"counter_evidence":[],"gaps":[]},
 {"id":"h1","label":"产业基金推动两泳道","is_null":false,"assessment":"plausible","confidence":"low","scope":"近三月","support_evidence":[],"counter_evidence":[],"gaps":["外部证据不可核查"]},
 {"id":"h2","label":"政策补贴同步带动","is_null":false,"assessment":"insufficient","confidence":"low","scope":"政策","support_evidence":[],"counter_evidence":[],"gaps":["缺补贴时间线"]}],
 "conclusion":{"summary":"无可核查外部证据，基金假设仅初步成立","confidence":"low","scope":"两条泳道","boundary":"资金明细未核实"},
 "evidence_chain":[
  {"id":"e2","source_type":"lane","ref":"901","lane_note":"产能与招标详情","supports":["h0"],"counters":[]}],
 "lane_refs":[{"lane_id":901,"note":"主泳道"},{"lane_id":902,"note":"对照泳道"}]}`

// invResearchSnapshot re-reads the persisted research digest fields a chain
// resilience assertion needs.
type invResearchSnapshot struct {
	Loops     float64 `json:"loops"`
	FinalData string  `json:"final_data"`
	Coverage  struct {
		NeutralAttempted             bool     `json:"neutral_attempted"`
		CounterAttemptedByHypothesis []string `json:"counter_attempted_by_hypothesis"`
	} `json:"coverage"`
	Gaps []struct {
		Reason        string   `json:"reason"`
		HypothesisIDs []string `json:"hypothesis_ids"`
	} `json:"gaps"`
}

func invGapReasons(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var top struct {
		Research invResearchSnapshot `json:"research"`
	}
	require.NoError(t, json.Unmarshal(raw, &top))
	out := make([]string, 0, len(top.Research.Gaps))
	for _, g := range top.Research.Gaps {
		require.True(t, invKnownResearchGapReasons[g.Reason],
			"gap reason must be a stable enum, got %q", g.Reason)
		out = append(out, g.Reason)
	}
	return out
}

func invHasGap(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

// invCountChildren counts persisted investigations under the parent brief.
func invCountChildren(t *testing.T, repo *repository.Repository, parentID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, repo.DB().Model(&repository.TopicEnrichmentResult{}).
		Where("parent_result_id = ?", parentID).Count(&count).Error)
	return count
}

// invAssertParentUnchanged re-reads the parent brief and compares its sectors
// semantically (PG jsonb round-trip re-serializes bytes).
func invAssertParentUnchanged(t *testing.T, repo *repository.Repository, parentID uint, before string) {
	t.Helper()
	var parent repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", parentID).First(&parent).Error)
	want, err := json.Marshal(json.RawMessage(before))
	require.NoError(t, err)
	got, err := json.Marshal(parent.Sectors)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got), "parent brief must stay untouched")
}

// ── 1. max_loops：研究耗尽后仍综合、落库、快照如实 ───────────────────────────

func TestInvestigateBoardQuestion_ResearchMaxLoopsStillSynthesizesAndPersists(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95171)
	brief := seedInvBrief(t, repo, boardID)

	// hypothesize 后接 6 轮全部 support(h1) 的不同查询、永不 finish：
	// 循环按 maxAgentLoops=6 耗尽退出 → partial + max_loops。
	router.addResponse(invHypothesesLLM)
	for i := 0; i < 6; i++ {
		router.addResponse(fmt.Sprintf(
			`{"action":"call_tool","thought":"继续检索","tool":"web_search","args":{"query":"资金来源线索 第%d条"},"purpose":"support","hypothesis_ids":["h1"]}`, i))
	}
	router.addResponse(invSynthesisLLM) // partial 研究之后 board_synthesize 仍被调用

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.NotNil(t, out.Result)

	// 编排契约：hypothesize → tool_use×6 → board_synthesize（无 finish）。
	wantOps := []string{"data_enrichment.board_hypothesize"}
	for i := 0; i < 6; i++ {
		wantOps = append(wantOps, "data_enrichment.tool_use")
	}
	wantOps = append(wantOps, "data_enrichment.board_synthesize")
	require.Equal(t, wantOps, invCallOps(router), "max-loops partial research must still reach board_synthesize")

	// 落库行存在且为调查档。
	var row repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", out.Result.ID).First(&row).Error)
	require.Equal(t, repository.ResultKindBoardInvestigation, row.ResultKind)
	require.NotNil(t, row.ParentResultID)
	require.Equal(t, brief.ID, *row.ParentResultID)

	// tool_calls：完整有序（step 1..6）、purpose/hypothesis_ids/outcome 齐全、
	// ResultFull 保全 stub 搜索结果原文（供 review/回放核对 quote）。
	var toolCalls []map[string]any
	require.NoError(t, json.Unmarshal(row.ToolCalls, &toolCalls))
	require.Len(t, toolCalls, 6)
	for i, tc := range toolCalls {
		require.Equal(t, float64(i+1), tc["step"], "tool call order must be preserved")
		require.Equal(t, "support", tc["purpose"])
		require.Equal(t, []any{"h1"}, tc["hypothesis_ids"])
		require.Equal(t, "ok", tc["outcome"])
		require.Contains(t, tc["result_full"], "基金公告原文摘录ABC")
	}

	// input_snapshot.research：max_loops + 按实际的 missing 纪律 gap + coverage。
	reasons := invGapReasons(t, row.InputSnapshot)
	require.True(t, invHasGap(reasons, "max_loops"), "gaps must carry max_loops: %v", reasons)
	require.True(t, invHasGap(reasons, "missing_neutral"), "6 support-only loops never attempt a neutral check: %v", reasons)
	require.True(t, invHasGap(reasons, "missing_counter"), "h1/h2 never get a counter attempt: %v", reasons)
	var top struct {
		Research invResearchSnapshot `json:"research"`
	}
	require.NoError(t, json.Unmarshal(row.InputSnapshot, &top))
	res := top.Research
	require.Equal(t, float64(6), res.Loops)
	require.Empty(t, res.FinalData, "no finish accepted → final_data stays empty")
	require.False(t, res.Coverage.NeutralAttempted)
	require.Empty(t, res.Coverage.CounterAttemptedByHypothesis)

	// sectors：partial 研究照样产出合法综合（e1 web 证据绑定 6 次真实
	// web_search 调用、e2 lane 在白名单内）。
	sectors := invSectorsMap(t, row.Sectors)
	require.Equal(t, "board_investigation", sectors["result_kind"])
	require.Len(t, sectors["hypotheses"], 3)
	require.Len(t, sectors["evidence_chain"], 2)
}

// ── 2. llm_error：非法研究输出 → partial + gap，仍综合落库，内部串不泄 ────────

func TestInvestigateBoardQuestion_ResearchLLMErrorPartialStillPersistsNoLeak(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95172)
	brief := seedInvBrief(t, repo, boardID)

	// 研究第 1 轮即输出无法解析的垃圾：parse error → partial（0 次工具
	// 调用）+ llm_error gap。垃圾文本带敏感标记：它只允许出现在
	// ai_call_logs 的 prompt/response 留痕里，绝不进落库快照。
	const garbage = "研究输出已损坏SENSITIVE-INTERNAL-ERR-勿入快照"
	router.addResponse(invHypothesesLLM)
	router.addResponse(garbage)
	// M13 一致性门：0 次工具调用 → e1（web）绑不到任何真实 web_search
	// 被剔 → h1 supported 无存活 support = 结构失败；纠错重试后模型把 h1
	// 降级 plausible（诚实评估：外部证据不可核查）→ 落库。
	router.addResponse(invSynthesisLLM)
	router.addResponse(invSynthesisLLMDowngraded)

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.NotNil(t, out.Result)

	// 编排仍走到 board_synthesize 并成功落库。
	require.Equal(t, "data_enrichment.board_synthesize", invCallOps(router)[len(invCallOps(router))-1])
	var row repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", out.Result.ID).First(&row).Error)
	require.Equal(t, repository.ResultKindBoardInvestigation, row.ResultKind)

	// input_snapshot.research.gaps：llm_error + missing 纪律（0 次执行调用），
	// 全部为稳定枚举；内部错误串（垃圾输出前缀）不得出现在快照任何位置。
	reasons := invGapReasons(t, row.InputSnapshot)
	require.True(t, invHasGap(reasons, "llm_error"), "gaps must carry llm_error: %v", reasons)
	require.True(t, invHasGap(reasons, "missing_neutral"))
	require.True(t, invHasGap(reasons, "missing_counter"))
	require.NotContains(t, string(row.InputSnapshot), garbage,
		"internal LLM garbage must never leak into input_snapshot")
	require.NotContains(t, string(row.Sectors), garbage,
		"internal LLM garbage must never leak into sectors")

	// retry 字段稳定：M13 后综合成功于第 2 次（第 1 次一致性门失败）→
	// retry 码为稳定机码 invalid_structure；整体快照里的 retry_reason
	// （hypotheses/synthesis）只能是稳定机码或空。
	snap := invSectorsMap(t, row.InputSnapshot)
	synth, ok := snap["synthesis"].(map[string]any)
	require.True(t, ok, "input_snapshot must persist synthesis generation metadata")
	retry, _ := synth["retry_reason"].(string)
	require.Equal(t, "invalid_structure", retry)
	require.NotContains(t, retry, garbage)
	require.NotContains(t, retry, "第1轮", "retry must be a stable code, not loop error text")
	if initial, ok := snap["initial_hypotheses"].(map[string]any); ok {
		retry, _ := initial["retry_reason"].(string)
		require.NotContains(t, retry, garbage)
	}

	// 研究快照：0 次循环推进由 loops=1 + final_data 空 + coverage 全假如实记录。
	var top struct {
		Research invResearchSnapshot `json:"research"`
	}
	require.NoError(t, json.Unmarshal(row.InputSnapshot, &top))
	require.Empty(t, top.Research.FinalData)
	require.False(t, top.Research.Coverage.NeutralAttempted)

	// sectors：e1（web）绑定不到任何真实 web_search 调用被机械剔除，
	// e2（lane 白名单内）保留——partial 研究不伪造外部证据；纠错重试后
	// h1 降级 plausible（supported 定论不可无存活支撑，M13）。
	sectors := invSectorsMap(t, row.Sectors)
	require.Len(t, sectors["evidence_chain"], 1)
	ev := sectors["evidence_chain"].([]any)[0].(map[string]any)
	require.Equal(t, "lane", ev["source_type"])
	hyps := sectors["hypotheses"].([]any)
	require.Len(t, hyps, 3)
	h1 := hyps[1].(map[string]any)
	require.Equal(t, "plausible", h1["assessment"])
	require.Empty(t, h1["support_evidence"])

	// 综合 prompt（→ ai_call_logs）只含稳定 gap 码，不含垃圾原文。
	for _, c := range router.Calls {
		if c.Operation == "data_enrichment.board_synthesize" {
			require.Contains(t, c.Messages[0].Content, "llm_error")
			require.NotContains(t, c.Messages[0].Content, garbage)
		}
	}

	// 落库 tool_calls 为空数组（非 null）。
	require.JSONEq(t, "[]", string(row.ToolCalls))
}

// ── 3. ctx 取消：research 前 / synth 前 → context 错误、0 行、父简报不动 ────

// invCtxCancelRouter gives the scripted mock real-router ctx semantics:
// Chat fails with ctx.Err() once the context is dead, and the cancel hook
// fires when the configured operation is first requested — pinning「取消发生
// 在 research/synth 之前」to an exact chain position without timing races.
// A mock that ignores ctx would let a canceled chain finish "successfully"
// (false positive); this wrapper refuses that.
type invCtxCancelRouter struct {
	inner    *mockAirRouter
	cancelOn string
	cancel   context.CancelFunc
	fired    bool
}

func (r *invCtxCancelRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if !r.fired && req.Operation == r.cancelOn {
		r.fired = true
		r.cancel()
	}
	if err := ctx.Err(); err != nil {
		// 与真实 HTTP 客户端一致：调用被记账（ops 断言可查）但以 ctx 错误失败。
		r.inner.Calls = append(r.inner.Calls, req)
		return nil, err
	}
	return r.inner.Chat(ctx, req)
}

func TestInvestigateBoardQuestion_CtxCancelZeroRowsParentUntouched(t *testing.T) {
	cases := []struct {
		name     string
		cancelOn string // operation whose first request triggers the cancel
	}{
		{"cancel before research", "data_enrichment.tool_use"},
		{"cancel before synth", "data_enrichment.board_synthesize"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			boardID := uint(95180 + i)
			inner := newMockAirRouter()
			// synth-cancel 子链需要研究正常走完（纪律齐备的 finish）。
			inner.addResponse(invHypothesesLLM)
			if tc.cancelOn == "data_enrichment.board_synthesize" {
				inner.addResponse(`{"action":"call_tool","thought":"内部核查","tool":"get_lane_detail","args":{"lane_id":901},"purpose":"neutral","hypothesis_ids":[]}`)
				inner.addResponse(`{"action":"call_tool","thought":"反证h1","tool":"web_search","args":{"query":"产业基金 独立资金来源明细"},"purpose":"counter","hypothesis_ids":["h1"]}`)
				inner.addResponse(`{"action":"call_tool","thought":"反证h2","tool":"web_search","args":{"query":"补贴政策 时间线对照"},"purpose":"counter","hypothesis_ids":["h2"]}`)
				inner.addResponse(`{"action":"finish","thought":"纪律已补齐","summary":"素材已汇总。"}`)
			}
			inner.addResponse(invSynthesisLLM) // 只在链未被取消时才会被消费

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			router := &invCtxCancelRouter{inner: inner, cancelOn: tc.cancelOn, cancel: cancel}
			orch, repo := newInvestigationOrchWithRouter(t, true, router)
			brief := seedInvBrief(t, repo, boardID)
			parentBefore := string(brief.Sectors)

			q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
			out, err := orch.InvestigateBoardQuestion(ctx, boardID, brief.ID, q)

			// 取消必须以 context 错误浮出（链内包装不吞掉 errors.Is）。
			require.Error(t, err)
			require.Nil(t, out)
			require.True(t, errors.Is(err, context.Canceled),
				"canceled chain must surface context.Canceled, got: %v", err)

			// board_synthesize 只允许在 synth-cancel 子链出现（且必以 ctx
			// 错误失败）；research-cancel 子链根本走不到综合。
			ops := invCallOps(inner)
			synthCalls := 0
			for _, op := range ops {
				if op == "data_enrichment.board_synthesize" {
					synthCalls++
				}
			}
			if tc.cancelOn == "data_enrichment.tool_use" {
				require.Zero(t, synthCalls, "cancel before research must never reach synthesis")
			} else {
				require.Positive(t, synthCalls, "cancel hook fires on the synth call itself")
			}

			// 0 investigation 行；父简报字节级语义不变。
			require.Zero(t, invCountChildren(t, repo, brief.ID), "canceled investigation must leave zero rows")
			invAssertParentUnchanged(t, repo, brief.ID, parentBefore)
		})
	}
}
