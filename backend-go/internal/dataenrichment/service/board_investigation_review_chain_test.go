package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
)

// ── 调查 review 隔离链路（task 4.7，test-cases M8.7/M8.8）────────────────────
//
// 契约（调度指令 + design D11）：
//   - 首份（同 parent+question_key 无前份）→ 0 judge、0 review。
//   - 同 parent+key 重跑 → judge 复用 data_enrichment.review_judge operation、
//     同一调查 session；judge true 写 review 行（board 所有权、topic nil、
//     prev/curr 正确、applied=false、affected_context 强制空）。
//   - generated/custom 规范化文本相同（trim+空白折叠）→ 同链可比。
//   - 不同问题/不同 parent/别版块/legacy/brief 夹行 → 不互比、不污染。
//   - judge chat/parse 失败 non-fatal：当前调查照常返回、0 review、不动
//     父简报与当前行、lifeline 永不回写。

// invJudgeTrueLLM：should_review=true，且 affected_context 恶意超长（600
// rune，远超 varchar(10)）——若未强制清空，INSERT 会整体失败丢 review 行。
var invJudgeTrueLLM = `{"should_review":true,"reason":"h1 评估从 plausible 迁移到 supported，且新增反证","new_findings":["基金公告直接支持 h1"],"overturned":["政策补贴同步假设被降级为 insufficient"],"confidence_shift":[{"insight":"h1","from":"plausible","to":"supported"}],"affected_context":"` +
	strings.Repeat("越界", 300) + `","confidence":0.82}`

// invReviewRows 清点某版块的 review 行。
func invReviewRows(t *testing.T, repo *repository.Repository, boardID uint) []repository.TopicEnrichmentReview {
	t.Helper()
	var rows []repository.TopicEnrichmentReview
	require.NoError(t, repo.DB().Where("semantic_board_id = ?", boardID).Find(&rows).Error)
	return rows
}

func invCleanupReview(t *testing.T, repo *repository.Repository, boardID uint) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.DB().Exec(`DELETE FROM topic_enrichment_review WHERE semantic_board_id = ?`, boardID).Error
	})
}

// invLifelineCount：lifeline 表行数（调查+review 全链不得回写表1）。
func invLifelineCount(t *testing.T, repo *repository.Repository) int64 {
	t.Helper()
	var n int64
	require.NoError(t, repo.DB().Model(&repository.TopicLifelineContext{}).Count(&n).Error)
	return n
}

// ── 首份：0 judge、0 review ────────────────────────────────────────────────

func TestBoardInvestigationReview_FirstRunZeroJudge(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95201)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupReview(t, repo, boardID)
	addInvChain(router, invSynthesisLLM)

	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	out, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.NotNil(t, out.Result)
	require.Nil(t, out.Review, "first investigation of the chain must produce no review")

	for _, c := range router.Calls {
		require.NotEqual(t, "data_enrichment.review_judge", c.Operation,
			"first run must not call the review judge")
	}
	require.Empty(t, invReviewRows(t, repo, boardID))
}

// ── 同 parent+key 重跑：judge + review 行（含恶意 affected_context 强制空）──

func TestBoardInvestigationReview_SameQuestionRerunJudgedAndReviewWritten(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95211)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupReview(t, repo, boardID)
	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}

	addInvChain(router, invSynthesisLLM)
	out1, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.Nil(t, out1.Review)

	addInvChain(router, invSynthesisLLM)
	router.addResponse(invJudgeTrueLLM)
	parentBefore := string(brief.Sectors)
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.NotNil(t, out2.Result)
	require.NotNil(t, out2.Review, "same parent+key rerun with judge=true must yield a review")

	// ops：run2 末尾是 review_judge，且与 run2 的 synthesize 共用同一 session。
	var synthSession, judgeSession, judgePrompt string
	sawJudge := false
	for _, c := range router.Calls {
		switch c.Operation {
		case "data_enrichment.board_synthesize":
			synthSession = c.SessionID
		case "data_enrichment.review_judge":
			sawJudge = true
			judgeSession = c.SessionID
			judgePrompt = c.Messages[0].Content
		}
	}
	require.True(t, sawJudge, "rerun must invoke the review judge")
	require.NotEmpty(t, synthSession)
	require.Equal(t, synthSession, judgeSession, "judge must share the investigation session")

	// prompt 投影禁区：问题与假设评估在场；父简报全文/证据逐字摘录/
	// legacy 词汇/方法正文不在场；两份载荷各带一次问题文本。
	require.Contains(t, judgePrompt, "两条泳道是否由同一资金驱动")
	require.Equal(t, 2, strings.Count(judgePrompt, "两条泳道是否由同一资金驱动"),
		"question text must appear exactly twice (prev + current payloads)")
	require.Contains(t, judgePrompt, "assessment")
	require.Contains(t, judgePrompt, "上一份调查")
	require.Contains(t, judgePrompt, "本次调查")
	for _, banned := range []string{
		"两条泳道各有进展",                    // 父简报 summary（parent brief 全文禁区）
		"基金公告原文摘录ABC",                 // evidence quote（投影丢弃）
		"thesis", "argument", "depth", // legacy 论文词汇
	} {
		require.NotContains(t, judgePrompt, banned, "judge prompt leaked forbidden content %q", banned)
	}

	// review 行：board 所有权、topic nil、prev/curr 正确、applied=false、
	// affected_context 强制空（恶意超长被硬挡）。
	rows := invReviewRows(t, repo, boardID)
	require.Len(t, rows, 1)
	row := rows[0]
	require.NotNil(t, row.SemanticBoardID)
	require.Equal(t, boardID, *row.SemanticBoardID)
	require.Nil(t, row.PersistentTopicID, "board review must not claim a topic owner")
	require.NotNil(t, row.PrevResultID)
	require.Equal(t, out1.Result.ID, *row.PrevResultID)
	require.Equal(t, out2.Result.ID, row.CurrResultID)
	require.False(t, row.Applied)
	require.Empty(t, row.AffectedContext, "affected_context must be forced empty (varchar(10) hard guard)")
	require.Equal(t, "h1 评估从 plausible 迁移到 supported，且新增反证", row.DeviationSummary)
	require.NotNil(t, row.Confidence)
	var verdict map[string]any
	require.NoError(t, json.Unmarshal(row.Verdict, &verdict))
	require.Equal(t, []any{"基金公告直接支持 h1"}, verdict["new_findings"])
	require.Equal(t, "llm_assisted", row.Source)
	require.Equal(t, out2.Review.ID, row.ID)

	// 不可变性：父简报语义不变；当前行 sectors/input_snapshot 落库后未被
	// review 改写；lifeline 全程 0 写入。
	var parentAfter repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", brief.ID).First(&parentAfter).Error)
	wantParent, err := json.Marshal(json.RawMessage(parentBefore))
	require.NoError(t, err)
	gotParent, err := json.Marshal(parentAfter.Sectors)
	require.NoError(t, err)
	require.JSONEq(t, string(wantParent), string(gotParent), "review must not touch the parent brief")

	var currAfter repository.TopicEnrichmentResult
	require.NoError(t, repo.DB().Where("id = ?", out2.Result.ID).First(&currAfter).Error)
	wantSectors, err := json.Marshal(out2.Result.Sectors)
	require.NoError(t, err)
	gotSectors, err := json.Marshal(currAfter.Sectors)
	require.NoError(t, err)
	require.JSONEq(t, string(wantSectors), string(gotSectors), "review must not rewrite the current investigation sectors")
	wantSnap, err := json.Marshal(out2.Result.InputSnapshot)
	require.NoError(t, err)
	gotSnap, err := json.Marshal(currAfter.InputSnapshot)
	require.NoError(t, err)
	require.JSONEq(t, string(wantSnap), string(gotSnap), "review must not rewrite the current input_snapshot")

	require.Zero(t, invLifelineCount(t, repo), "investigation review must never write topic_lifeline_context")
}

// ── generated/custom 规范化同 key：跨来源重跑可比（M8.8）───────────────────

func TestBoardInvestigationReview_GeneratedCustomSameNormalizedKey(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95221)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupReview(t, repo, boardID)

	addInvChain(router, invSynthesisLLM)
	out1, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID,
		service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由 同一资金驱动", Source: "generated"})
	require.NoError(t, err)
	require.Equal(t, repository.ComputeQuestionKey("两条泳道是否由 同一资金驱动"), *out1.Result.QuestionKey)

	// custom：全角空格开头 + 半角空白结尾 + 中间双空格（基准是单空格）——
	// trim+折叠后与 generated 文本同 key，必须进入同一条比较链。
	addInvChain(router, invSynthesisLLM)
	router.addResponse(invJudgeTrueLLM)
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID,
		service.BoardInvestigationQuestion{Text: "　两条泳道是否由  同一资金驱动 \t ", Source: "custom"})
	require.NoError(t, err)
	require.Equal(t, *out1.Result.QuestionKey, *out2.Result.QuestionKey,
		"normalized whitespace variants must share one question key")
	require.NotNil(t, out2.Review, "custom rerun with the same normalized key must be judged")

	rows := invReviewRows(t, repo, boardID)
	require.Len(t, rows, 1)
	require.Equal(t, out1.Result.ID, *rows[0].PrevResultID)
	require.Equal(t, out2.Result.ID, rows[0].CurrResultID)
}

// ── 不同问题/不同 parent/别版块：不互比 ───────────────────────────────────

func TestBoardInvestigationReview_DifferentQuestionParentBoardNoJudge(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardA = uint(95231)
	const boardB = uint(95232)
	briefA1 := seedInvBrief(t, repo, boardA)
	briefA2 := seedInvBrief(t, repo, boardA)
	briefB := seedInvBrief(t, repo, boardB)
	invCleanupReview(t, repo, boardA)
	invCleanupReview(t, repo, boardB)

	q1 := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	q2 := service.BoardInvestigationQuestion{ID: "q2", Text: "招标节奏是否影响产能排期", Source: "generated"}
	for i := 0; i < 4; i++ {
		addInvChain(router, invSynthesisLLM)
	}
	// 同 board 同 parent 同 key（首份）。
	_, err := orch.InvestigateBoardQuestion(context.Background(), boardA, briefA1.ID, q1)
	require.NoError(t, err)
	// 不同问题（同 parent）。
	_, err = orch.InvestigateBoardQuestion(context.Background(), boardA, briefA1.ID, q2)
	require.NoError(t, err)
	// 不同 parent（同 board 同问题文本）。
	_, err = orch.InvestigateBoardQuestion(context.Background(), boardA, briefA2.ID, q1)
	require.NoError(t, err)
	// 别版块（同问题文本）。
	_, err = orch.InvestigateBoardQuestion(context.Background(), boardB, briefB.ID, q1)
	require.NoError(t, err)

	for _, c := range router.Calls {
		require.NotEqual(t, "data_enrichment.review_judge", c.Operation,
			"different question/parent/board must never trigger the judge")
	}
	require.Empty(t, invReviewRows(t, repo, boardA))
	require.Empty(t, invReviewRows(t, repo, boardB))
}

// ── legacy/brief/异链调查夹行不污染 prev 选择 ──────────────────────────────

func TestBoardInvestigationReview_InterleavedForeignRowsDoNotPollute(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95241)
	briefA := seedInvBrief(t, repo, boardID)
	briefB := seedInvBrief(t, repo, boardID)
	invCleanupReview(t, repo, boardID)

	q1 := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}
	key1 := repository.ComputeQuestionKey(q1.Text)
	keyOther := repository.ComputeQuestionKey("别的问题文本？")

	addInvChain(router, invSynthesisLLM)
	out1, err := orch.InvestigateBoardQuestion(context.Background(), boardID, briefA.ID, q1)
	require.NoError(t, err)

	// 夹行（id 介于 run1 与 run2 之间）：legacy（无 key）、同 board 新简报、
	// 别 parent 的同 key 调查、同 parent 的异 key 调查。
	legacy := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindLegacyBoardAnalysis, SessionID: "inv-review-pollute-legacy",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(context.Background(), legacy))
	briefNoise := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ResultKind: repository.ResultKindBoardBrief, SessionID: "inv-review-pollute-brief",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(context.Background(), briefNoise))
	invOtherParent := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(briefB.ID), QuestionKey: &key1,
		SessionID: "inv-review-pollute-other-parent",
	}
	require.NoError(t, repo.CreateBoardInvestigationResult(context.Background(), invOtherParent))
	invOtherKey := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID), AnalysisScope: "board",
		ParentResultID: repository.TopicIDPtr(briefA.ID), QuestionKey: &keyOther,
		SessionID: "inv-review-pollute-other-key",
	}
	require.NoError(t, repo.CreateBoardInvestigationResult(context.Background(), invOtherKey))
	t.Cleanup(func() {
		ids := []uint{legacy.ID, briefNoise.ID, invOtherParent.ID, invOtherKey.ID}
		for _, id := range ids {
			_ = repo.DB().Exec(`DELETE FROM topic_enrichment_result WHERE id = ?`, id).Error
		}
	})

	addInvChain(router, invSynthesisLLM)
	router.addResponse(invJudgeTrueLLM)
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, briefA.ID, q1)
	require.NoError(t, err)
	require.NotNil(t, out2.Review)

	rows := invReviewRows(t, repo, boardID)
	require.Len(t, rows, 1)
	require.Equal(t, out1.Result.ID, *rows[0].PrevResultID,
		"prev must stay on the same parent+key chain, skipping interleaved foreign rows")
}

// ── judge false：不写行 ────────────────────────────────────────────────────

func TestBoardInvestigationReview_JudgeFalseNoReviewRow(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95251)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupReview(t, repo, boardID)
	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}

	addInvChain(router, invSynthesisLLM)
	_, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)

	addInvChain(router, invSynthesisLLM)
	router.addResponse(`{"should_review":false,"reason":"仅措辞调整","new_findings":[],"overturned":[],"confidence_shift":[],"affected_context":"","confidence":0.9}`)
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)
	require.Nil(t, out2.Review, "judge=false must not yield a review")
	require.Empty(t, invReviewRows(t, repo, boardID))
}

// ── judge chat/parse 失败 non-fatal：当前调查仍在、0 review ────────────────

// invJudgeFailRouter 只让 review_judge operation 报错（chat 失败路径）。
type invJudgeFailRouter struct {
	inner      *mockAirRouter
	judgeCalls int
}

func (r *invJudgeFailRouter) Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error) {
	if req.Operation == "data_enrichment.review_judge" {
		r.judgeCalls++
		return nil, errors.New("judge backend down")
	}
	return r.inner.Chat(ctx, req)
}

func TestBoardInvestigationReview_JudgeFailuresNonFatal(t *testing.T) {
	orch, router, repo := newInvestigationOrch(t, true)
	const boardID = uint(95261)
	brief := seedInvBrief(t, repo, boardID)
	invCleanupReview(t, repo, boardID)
	q := service.BoardInvestigationQuestion{ID: "q1", Text: "两条泳道是否由同一资金驱动", Source: "generated"}

	addInvChain(router, invSynthesisLLM)
	_, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err)

	// parse 失败：judge 返回坏 JSON → 当前调查照常返回、0 review。
	addInvChain(router, invSynthesisLLM)
	router.addResponse("坏JSON不是评审结论")
	out2, err := orch.InvestigateBoardQuestion(context.Background(), boardID, brief.ID, q)
	require.NoError(t, err, "judge parse failure must be non-fatal")
	require.NotNil(t, out2.Result)
	require.Nil(t, out2.Review)
	var n int64
	require.NoError(t, repo.DB().Model(&repository.TopicEnrichmentResult{}).
		Where("id = ?", out2.Result.ID).Count(&n).Error)
	require.Equal(t, int64(1), n, "current investigation must stay persisted")
	require.Empty(t, invReviewRows(t, repo, boardID))

	// chat 失败：review_judge 调用报错 → 同样 non-fatal。同一 brief 跑两遍：
	// 第二遍才命中 judge 路径（首遍无 prev）。
	inner := newMockAirRouter()
	addInvChain(inner, invSynthesisLLM)
	addInvChain(inner, invSynthesisLLM)
	wrapped := &invJudgeFailRouter{inner: inner}
	orch2, repo2 := newInvestigationOrchWithRouter(t, true, wrapped)
	brief2 := seedInvBrief(t, repo2, boardID)
	invCleanupReview(t, repo2, boardID)
	_, err = orch2.InvestigateBoardQuestion(context.Background(), boardID, brief2.ID, q)
	require.NoError(t, err)
	out3, err := orch2.InvestigateBoardQuestion(context.Background(), boardID, brief2.ID, q)
	require.NoError(t, err, "judge chat failure must be non-fatal")
	require.NotNil(t, out3.Result)
	require.Nil(t, out3.Review)
	require.Equal(t, 1, wrapped.judgeCalls, "judge path must have been reached and failed")
	require.Empty(t, invReviewRows(t, repo2, boardID))
}
