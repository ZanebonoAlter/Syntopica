package board

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// expandDeterministicEmbedder 是挂载确认测试用的确定性 embedder。
var expandDeterministicEmbedder = func(ctx context.Context, input string, mode auxlabel.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
	return core.FloatsToPgVector(vec), vec, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 召回（spec: 扩充候选召回）
// ─────────────────────────────────────────────────────────────────────────────

func TestRecallExpandAuxCandidatesSimAndCooccur(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	// 版块「美债」：embedding 沿 (1,0,0)，构成标签 seed。
	board := createUpgradeLabel(t, db, "美债", "us-treasury", "board", "active", 0, []float64{1, 0, 0})
	boardAux := createUpgradeLabel(t, db, "美国国债", "us-treasury-aux", "auxiliary", "active", 2, []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	// 相似路命中：距离 0.2 ≤ 默认 0.35。
	near := createUpgradeLabel(t, db, "美债拍卖", "us-treasury-auction", "auxiliary", "active", 8, []float64{0.9, 0.4358898943, 0})
	// 相似路未命中：正交向量距离 1.0。
	far := createUpgradeLabel(t, db, "日本央行", "boj", "auxiliary", "active", 8, []float64{0, 1, 0})
	// 已挂载进目标版块：不召回（即使相似）。
	_ = far
	// 共现路命中：与构成标签「美国国债」同文章共现 ≥3（默认阈值）。
	cooccurAux := createUpgradeLabel(t, db, "国债期货", "treasury-futures", "auxiliary", "active", 6, []float64{0, 1, 0})
	tag := createComposeEventTag(t, db, "ex-event", boardAux.ID, cooccurAux.ID)
	createComposeArticles(t, db, 5, 0, tag.ID)
	// 共现不足：仅 1 篇共现。
	weakAux := createUpgradeLabel(t, db, "弱关联", "weak-aux", "auxiliary", "active", 6, []float64{0, 1, 0})
	weakTag := createComposeEventTag(t, db, "weak-event", boardAux.ID, weakAux.ID)
	createComposeArticles(t, db, 1, 0, weakTag.ID)

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	profile, err := svc.loadBoardExpandProfile(context.Background(), board.ID)
	require.NoError(t, err)
	config := svc.LoadUpgradeConfig(context.Background())

	candidates, validIDs, err := svc.recallExpandAuxCandidates(context.Background(), config, profile)
	require.NoError(t, err)

	ids := map[uint]bool{}
	for _, c := range candidates {
		ids[c.ID] = true
	}
	require.True(t, ids[near.ID], "相似路命中召回")
	require.True(t, ids[cooccurAux.ID], "共现路命中召回")
	require.False(t, ids[weakAux.ID], "共现不足不召回")
	require.False(t, ids[boardAux.ID], "已挂载进目标版块不召回")
	require.False(t, ids[far.ID], "相似度未达且无共现不召回")
	_, ok := validIDs[near.ID]
	require.True(t, ok, "valid IDs 集与候选一致")
}

func TestRecallExpandAuxCandidatesEmpty(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	board := createUpgradeLabel(t, db, "孤立版块", "lone-board", "board", "active", 0, []float64{1, 0, 0})
	createUpgradeLabel(t, db, "无关标签", "unrelated", "auxiliary", "active", 8, []float64{0, 1, 0})

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	profile, err := svc.loadBoardExpandProfile(context.Background(), board.ID)
	require.NoError(t, err)
	candidates, _, err := svc.recallExpandAuxCandidates(context.Background(), svc.LoadUpgradeConfig(context.Background()), profile)
	require.NoError(t, err)
	require.Empty(t, candidates, "召回为空是正常结果非错误")
}

func TestRecallExpandAuxCandidatesDisabledExcluded(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	board := createUpgradeLabel(t, db, "美债D", "us-treasury-d", "board", "active", 0, []float64{1, 0, 0})
	disabled := createUpgradeLabel(t, db, "已禁用", "disabled-aux", "auxiliary", "disabled", 8, []float64{1, 0, 0})

	svc := NewSemanticBoardUpgradeService(db, nil, nil)
	profile, err := svc.loadBoardExpandProfile(context.Background(), board.ID)
	require.NoError(t, err)
	candidates, _, err := svc.recallExpandAuxCandidates(context.Background(), svc.LoadUpgradeConfig(context.Background()), profile)
	require.NoError(t, err)
	for _, c := range candidates {
		require.NotEqual(t, disabled.ID, c.ID, "disabled aux 不召回")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 画像 prompt 与二分类裁决（spec: 版块画像上下文与二分类裁决 / 扩充方向锁定单版块）
// ─────────────────────────────────────────────────────────────────────────────

func TestGenerateExpandSuggestionsMergeBinary(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	board := createUpgradeLabel(t, db, "美债", "us-treasury-b", "board", "active", 0, []float64{1, 0, 0})
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("id = ?", board.ID).Update("description", "美国国债相关主题").Error)
	boardAux := createUpgradeLabel(t, db, "美国国债", "us-treasury-aux-b", "auxiliary", "active", 2, []float64{1, 0, 0})
	compositeComp := createUpgradeLabel(t, db, "组合件", "comp-x", "auxiliary", "active", 2, []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	composite := createUpgradeLabel(t, db, "美债收益率组合", "us-treasury-comp", "composite", "active", 0, nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: composite.ID}).Error)
	_ = compositeComp
	topic := createUpgradePersistentTopic(t, db, board.ID, "active")
	report := createUpgradeBoardDailyReport(t, db, board.ID, daysAgo(1))
	createUpgradeReportSection(t, db, report.ID, topic.ID, "美债拍卖热度攀升", []float64{1, 0, 0})

	inAux := createUpgradeLabel(t, db, "美联储利率", "fed-rate", "auxiliary", "active", 8, []float64{0.9, 0.4358898943, 0})
	fakeLLM := &fakeSemanticBoardUpgradeLLM{suggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionMergeIntoExisting, AuxiliaryLabelIDs: []uint{inAux.ID}, Reason: "属于美债"},
		{Decision: SemanticBoardUpgradeDecisionSkip, Reason: "不相关"},
		{Decision: SemanticBoardUpgradeDecisionCreateNew, BoardLabel: "越权", AuxiliaryLabelIDs: []uint{inAux.ID}},
	}}
	svc := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	suggestions, _, err := svc.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionExpand, Source: UpgradeSourceAux, TargetBoardID: board.ID})
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "仅有效 merge 保留；skip 与越权 create_new 丢弃")
	require.Equal(t, SemanticBoardUpgradeDecisionMergeIntoExisting, suggestions[0].Decision)
	require.NotNil(t, suggestions[0].TargetBoardID)
	require.Equal(t, board.ID, *suggestions[0].TargetBoardID, "target 由服务端注入锁定版块")
	require.Equal(t, "llm", suggestions[0].Confidence)

	// 画像进 prompt：描述 + 构成（组合标记）+ 近期内容标题。
	require.Contains(t, fakeLLM.prompt, "美国国债相关主题")
	require.Contains(t, fakeLLM.prompt, "美债收益率组合（组合）")
	require.Contains(t, fakeLLM.prompt, "美债拍卖热度攀升")
}

func TestGenerateExpandSuggestionsComposeTarget(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	board := createUpgradeLabel(t, db, "美债", "us-treasury-c", "board", "active", 0, []float64{1, 0, 0})
	boardAux := createUpgradeLabel(t, db, "美国国债", "us-treasury-aux-c", "auxiliary", "active", 8, []float64{1, 0, 0})
	yieldAux := createUpgradeLabel(t, db, "收益率", "yield-c", "auxiliary", "active", 6, []float64{0.9, 0.4358898943, 0})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	// 相关共现对：美国国债（构成内）× 收益率，共现 ≥10。
	tag := createComposeEventTag(t, db, "ex-compose-event", boardAux.ID, yieldAux.ID)
	createComposeArticles(t, db, 12, 0, tag.ID)
	// 无关共现对：两标签都不在（召回集 ∪ 构成集）。
	offA := createUpgradeLabel(t, db, "日本", "jp-x", "auxiliary", "active", 8, []float64{0, 1, 0})
	offB := createUpgradeLabel(t, db, "市场", "market-x", "auxiliary", "active", 6, []float64{0, 0.9, 0.4358898943})
	offTag := createComposeEventTag(t, db, "off-event", offA.ID, offB.ID)
	createComposeArticles(t, db, 12, 0, offTag.ID)

	fakeLLM := &composeAwareLLM{composeSuggestions: []SemanticBoardUpgradeSuggestion{
		{Decision: SemanticBoardUpgradeDecisionCompose, BoardLabel: "美债收益率", Description: "组合", AuxiliaryLabelIDs: []uint{boardAux.ID, yieldAux.ID}},
	}}
	svc := NewSemanticBoardUpgradeService(db, fakeLLM, nil)

	suggestions, _, err := svc.GenerateSuggestions(context.Background(), UpgradeGenerateRequest{Direction: UpgradeDirectionExpand, Source: UpgradeSourceComposite, TargetBoardID: board.ID})
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	require.Equal(t, SemanticBoardUpgradeDecisionCompose, suggestions[0].Decision)
	require.NotNil(t, suggestions[0].TargetBoardID)
	require.Equal(t, board.ID, *suggestions[0].TargetBoardID, "组合建议携带锁定版块 target")

	// 无关对被相关性过滤：prompt 不含日本×市场候选。
	require.Contains(t, fakeLLM.lastComposePrompt, "美债收益率")
	require.NotContains(t, fakeLLM.lastComposePrompt, "日本(ID:", "无关组合对被过滤不送裁")
}

// ─────────────────────────────────────────────────────────────────────────────
// create×aux prompt：全量版块清单与截断（design D2）
// ─────────────────────────────────────────────────────────────────────────────

func TestBuildCreateAuxPromptBoardList(t *testing.T) {
	cluster := SemanticBoardUpgradeCluster{
		Candidates: []SemanticBoardUpgradeCandidate{{ID: 1, Label: "光伏", RefCount: 5}},
		Centroid:   testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim),
	}
	boards := []activeBoardBrief{
		{BoardID: 1, BoardLabel: "新能源", BoardDescription: "新能源主题", Embedding: testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)},
		{BoardID: 2, BoardLabel: "远版块", Embedding: testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim)},
	}
	prompt := buildCreateAuxPrompt([]SemanticBoardUpgradeCluster{cluster}, boards)
	require.Contains(t, prompt, "create_new（创建新板块）或 skip")
	require.Contains(t, prompt, "新能源")
	require.Contains(t, prompt, "新能源主题")
	require.Contains(t, prompt, "光伏")
}

func TestBuildCreateAuxPromptBoardListTruncated(t *testing.T) {
	cluster := SemanticBoardUpgradeCluster{
		Candidates: []SemanticBoardUpgradeCandidate{{ID: 1, Label: "光伏", RefCount: 5}},
		Centroid:   testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim),
	}
	boards := make([]activeBoardBrief, 0, UpgradeBoardListLimit+1)
	near := activeBoardBrief{BoardID: 999, BoardLabel: "最近版块", Embedding: testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)}
	boards = append(boards, near)
	for i := 0; i < UpgradeBoardListLimit; i++ {
		boards = append(boards, activeBoardBrief{BoardID: uint(i + 1), BoardLabel: fmt.Sprintf("远版块%03d", i), Embedding: testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim)})
	}
	prompt := buildCreateAuxPrompt([]SemanticBoardUpgradeCluster{cluster}, boards)
	require.Contains(t, prompt, "最近版块", "与簇质心最近的版块保留在截断后的清单")
	// 截断后 ≤ UpgradeBoardListLimit 条（1 最近 + 59 远 = 60）：等距远版块按稳定
	// 序挤出最后一个（ID=60 的远版块059），最近版块与 ID=1 的远版块000 保留。
	require.NotContains(t, prompt, "远版块059", "超出上限的远版块被截断")
	require.Contains(t, prompt, "远版块058", "恰在上限内的远版块保留")
	require.Contains(t, prompt, "远版块000", "稳定排序下首个远版块保留")
}

// ─────────────────────────────────────────────────────────────────────────────
// compose 确认挂载（spec: compose 建议确认执行——扩充方向分支）
// ─────────────────────────────────────────────────────────────────────────────

func TestConfirmComposeSuggestionMountsToTargetBoard(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	board := createUpgradeLabel(t, db, "美债M", "us-treasury-m", "board", "active", 0, nil)
	auxA := createUpgradeLabel(t, db, "美国国债M", "us-treasury-aux-m", "auxiliary", "active", 8, []float64{1, 0, 0})
	auxB := createUpgradeLabel(t, db, "收益率M", "yield-m", "auxiliary", "active", 6, []float64{1, 0, 0})
	svc := NewSemanticBoardUpgradeService(db, nil, expandDeterministicEmbedder)

	target := board.ID
	result, err := svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "美债收益率M",
		Description:       "组合测试",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		TargetBoardID:     &target,
	})
	require.NoError(t, err)
	require.NotNil(t, result.CompositeLabelID)

	var count int64
	require.NoError(t, db.Model(&models.BoardComposition{}).Where("board_id = ? AND auxiliary_label_id = ?", board.ID, *result.CompositeLabelID).Count(&count).Error)
	require.Equal(t, int64(1), count, "组合标签挂载进目标版块 board_composition")
}

func TestConfirmComposeSuggestionDisabledTargetFails(t *testing.T) {
	db := setupSemanticBoardUpgradeTestDB(t)
	board := createUpgradeLabel(t, db, "已禁用版块", "disabled-board", "board", "disabled", 0, nil)
	auxA := createUpgradeLabel(t, db, "组件A", "comp-a", "auxiliary", "active", 8, []float64{1, 0, 0})
	auxB := createUpgradeLabel(t, db, "组件B", "comp-b", "auxiliary", "active", 6, []float64{1, 0, 0})
	svc := NewSemanticBoardUpgradeService(db, nil, expandDeterministicEmbedder)

	target := board.ID
	_, err := svc.ConfirmSuggestion(context.Background(), ConfirmSemanticBoardUpgradeRequest{
		Decision:          SemanticBoardUpgradeDecisionCompose,
		BoardLabel:        "禁用目标组合",
		AuxiliaryLabelIDs: []uint{auxA.ID, auxB.ID},
		TargetBoardID:     &target,
	})
	require.Error(t, err, "目标版块被禁用后确认失败（spec: 目标版块被禁用后确认失败）")

	var compositeCount int64
	require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ? AND label = ?", "composite", "禁用目标组合").Count(&compositeCount).Error)
	require.Zero(t, compositeCount, "确认失败整体回滚：组合标签不落库")
}
