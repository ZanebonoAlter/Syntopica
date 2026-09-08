package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

func TestAnalysisMethodAssembly_ZeroSelected(t *testing.T) {
	assembled := AssembleSelectedAnalysisMethods(nil, 4000)
	require.Empty(t, assembled.Prompt)
	require.Empty(t, assembled.MethodRefs)
	require.Empty(t, assembled.Dropped)
}

func TestAnalysisMethodAssembly_MaxTwoAndWholeCardBudget(t *testing.T) {
	methods := []repository.AnalysisMethod{
		{ID: 1, Name: "first", Title: "第一张", Content: strings.Repeat("甲", 10)},
		{ID: 2, Name: "second", Title: "第二张", Content: strings.Repeat("乙", 100)},
		{ID: 3, Name: "third", Title: "第三张", Content: "丙"},
	}
	// Enough for the first card only. Input order is relevance order supplied by
	// the future selector; this helper does not select or call an LLM.
	assembled := AssembleSelectedAnalysisMethods(methods, 30)
	require.Len(t, assembled.MethodRefs, 1)
	require.Equal(t, uint(1), assembled.MethodRefs[0].ID)
	require.Len(t, assembled.MethodRefs[0].ContentHash, 64)
	require.Contains(t, assembled.Prompt, strings.Repeat("甲", 10))
	require.NotContains(t, assembled.Prompt, "乙")
	require.NotContains(t, assembled.Prompt, "丙")
	require.Len(t, assembled.Dropped, 2)
	require.Equal(t, "budget_exceeded", assembled.Dropped[0].Reason)
	require.Equal(t, "selection_limit", assembled.Dropped[1].Reason)
	// 每张输入卡都有 trace（1 注入 + 2 舍弃），不含原文敏感内容限制。
	require.Len(t, assembled.Cards, 3)
	require.True(t, assembled.Cards[0].Injected)
	require.Equal(t, "budget_exceeded", assembled.Cards[1].DroppedReason)
	require.Equal(t, "selection_limit", assembled.Cards[2].DroppedReason)
}

// M7.7：部分清洗 → hash 仍取原始 Content 字节，预算按清洗后整卡正文计算，
// 被过滤原文不进 prompt/trace。
func TestAnalysisMethodAssembly_PartialRhetoricFilteredHashOriginalBudgetCleaned(t *testing.T) {
	personaLine := "模仿《内部看美国》作者的口吻写作"
	goldenLine := "每段结尾插入一句金句升华主题"
	stepLine := "步骤一：列出传导链每一环的证据来源"
	original := stepLine + "\n" + personaLine + "\n" + goldenLine
	methods := []repository.AnalysisMethod{
		{ID: 7, Name: "causal", Title: "因果链检验", Content: original},
	}
	assembled := AssembleSelectedAnalysisMethods(methods, 4000)
	require.Len(t, assembled.MethodRefs, 1)
	// method_ref hash = 原始 Content 字节（部分清洗不变，快照可回放对原文校验）。
	require.Equal(t, AnalysisMethodContentHash(original), assembled.MethodRefs[0].ContentHash)
	// prompt 只含清洗后正文：证据步骤在，修辞指令不在。
	require.Contains(t, assembled.Prompt, stepLine)
	require.NotContains(t, assembled.Prompt, personaLine)
	require.NotContains(t, assembled.Prompt, goldenLine)
	// 按卡 trace：原始 hash + 行数 + 原因码 + 实际注入正文，且不携带被删原文。
	require.Len(t, assembled.Cards, 1)
	card := assembled.Cards[0]
	require.Equal(t, uint(7), card.ID)
	require.Equal(t, "因果链检验", card.Title)
	require.Equal(t, AnalysisMethodContentHash(original), card.ContentHash)
	require.Equal(t, 2, card.FilteredLines)
	require.Equal(t, []string{methodFilterFixedPhrase, methodFilterPersona}, card.ReasonCodes)
	require.True(t, card.Injected)
	require.Equal(t, stepLine, card.InjectedContent)
	traceJSON, err := json.Marshal(assembled.Cards)
	require.NoError(t, err)
	require.NotContains(t, string(traceJSON), personaLine)
	require.NotContains(t, string(traceJSON), goldenLine)

	// 预算按清洗后正文：原文超限但清洗后入预算 → 仍整卡注入。
	heavyRhetoric := strings.Repeat("模仿专栏作者的口吻写作，", 40)
	cleanableOriginal := stepLine + "\n" + heavyRhetoric
	budget := len([]rune("## 因果链检验\n" + stepLine + "\n"))
	methods = []repository.AnalysisMethod{
		{ID: 8, Name: "causal2", Title: "因果链检验", Content: cleanableOriginal},
	}
	assembled = AssembleSelectedAnalysisMethods(methods, budget)
	require.Len(t, assembled.MethodRefs, 1, "budget must be computed over cleaned content, not the original")
	require.Contains(t, assembled.Prompt, stepLine)
	require.NotContains(t, assembled.Prompt, "模仿专栏作者")
	// 反例：清洗后仍超限 → 整卡舍弃照旧。
	assembled = AssembleSelectedAnalysisMethods(methods, budget-1)
	require.Empty(t, assembled.MethodRefs)
	require.Len(t, assembled.Dropped, 1)
	require.Equal(t, "budget_exceeded", assembled.Dropped[0].Reason)
}

// M7.7：全违规卡整卡舍弃，无 method_ref，trace 只有原因码/行数。
func TestAnalysisMethodAssembly_FullyNoncompliantCardDroppedWhole(t *testing.T) {
	personaLine := "模仿《内部看美国》作者的口吻写作"
	goldenLine := "每段结尾插入一句金句升华主题"
	methods := []repository.AnalysisMethod{
		{ID: 9, Name: "ghost", Title: "文风卡", Content: personaLine + "\n" + goldenLine},
	}
	assembled := AssembleSelectedAnalysisMethods(methods, 4000)
	require.Empty(t, assembled.MethodRefs, "fully noncompliant card must not be referenced")
	require.Empty(t, assembled.Prompt)
	require.Len(t, assembled.Dropped, 1)
	require.Equal(t, "content_noncompliant", assembled.Dropped[0].Reason)
	require.Len(t, assembled.Cards, 1)
	card := assembled.Cards[0]
	require.False(t, card.Injected)
	require.Equal(t, 2, card.FilteredLines)
	require.Equal(t, []string{methodFilterFixedPhrase, methodFilterPersona}, card.ReasonCodes)
	require.Equal(t, "content_noncompliant", card.DroppedReason)
	require.Empty(t, card.InjectedContent)
	// 被过滤原文不落在任何产出结构里。
	traceJSON, err := json.Marshal(assembled)
	require.NoError(t, err)
	require.NotContains(t, string(traceJSON), personaLine)
	require.NotContains(t, string(traceJSON), goldenLine)
}

// 空白/全空 Content 卡与全违规卡同待遇：content_noncompliant、0 ref/
// 0 prompt，不注入只有标题的空卡；trace 照留（0 行过滤、无原因码）。
func TestAnalysisMethodAssembly_BlankContentCardIsNoncompliant(t *testing.T) {
	for name, content := range map[string]string{
		"empty":      "",
		"whitespace": "\n  \n\t\n",
	} {
		t.Run(name, func(t *testing.T) {
			methods := []repository.AnalysisMethod{
				{ID: 12, Name: "blank", Title: "空卡", Content: content},
			}
			assembled := AssembleSelectedAnalysisMethods(methods, 4000)
			require.Empty(t, assembled.MethodRefs, "blank card must not be referenced")
			require.Empty(t, assembled.Prompt, "blank card must inject nothing")
			require.Len(t, assembled.Dropped, 1)
			require.Equal(t, "content_noncompliant", assembled.Dropped[0].Reason)
			require.Len(t, assembled.Cards, 1)
			require.False(t, assembled.Cards[0].Injected)
			require.Zero(t, assembled.Cards[0].FilteredLines)
			require.Empty(t, assembled.Cards[0].ReasonCodes)
			require.Equal(t, "content_noncompliant", assembled.Cards[0].DroppedReason)
			require.Empty(t, assembled.Cards[0].InjectedContent)
		})
	}
}

// runeBudget<=0 是「无预算」语义：所有卡一律 budget_exceeded 全弃，0 ref/
// 0 prompt（行为不变，注释/测试钉住语义）。
func TestAnalysisMethodAssembly_NonPositiveBudgetDropsAllCards(t *testing.T) {
	methods := []repository.AnalysisMethod{
		{ID: 21, Name: "a", Title: "甲卡", Content: "甲步骤"},
		{ID: 22, Name: "b", Title: "乙卡", Content: "乙步骤"},
	}
	for _, budget := range []int{0, -1} {
		assembled := AssembleSelectedAnalysisMethods(methods, budget)
		require.Empty(t, assembled.MethodRefs)
		require.Empty(t, assembled.Prompt)
		require.Len(t, assembled.Dropped, 2)
		for _, d := range assembled.Dropped {
			require.Equal(t, "budget_exceeded", d.Reason)
		}
		for _, c := range assembled.Cards {
			require.False(t, c.Injected)
			require.Equal(t, "budget_exceeded", c.DroppedReason)
		}
	}
}

// M7.7：否定性边界说明卡（禁止金句/禁用句式）不得被反向误删。
func TestAnalysisMethodAssembly_NegativeBoundaryCardNotDropped(t *testing.T) {
	content := "禁止使用固定金句与套话。\n不要使用「不是…而是…」的对仗句式。\n结论必须能被证据削弱。"
	methods := []repository.AnalysisMethod{
		{ID: 10, Name: "discipline", Title: "证据纪律", Content: content},
	}
	assembled := AssembleSelectedAnalysisMethods(methods, 4000)
	require.Len(t, assembled.MethodRefs, 1)
	require.Equal(t, AnalysisMethodContentHash(content), assembled.MethodRefs[0].ContentHash)
	require.Contains(t, assembled.Prompt, "禁止使用固定金句与套话")
	require.Contains(t, assembled.Prompt, "不要使用「不是…而是…」的对仗句式")
	require.Len(t, assembled.Cards, 1)
	require.Zero(t, assembled.Cards[0].FilteredLines)
	require.Empty(t, assembled.Cards[0].ReasonCodes)
	require.True(t, assembled.Cards[0].Injected)
}

func TestAnalysisMethodSelectionMeta_AvoidWhenJSONRoundTrip(t *testing.T) {
	meta := repository.AnalysisMethodSelectionMeta{
		ApplicableWhen:   []string{"存在可比较机制"},
		AvoidWhen:        []string{"只有单条未经核实消息"},
		RequiredEvidence: []string{"至少两个独立来源"},
		FailureModes:     []string{"把时间先后误当因果"},
	}
	raw, err := json.Marshal(meta)
	require.NoError(t, err)
	var decoded repository.AnalysisMethodSelectionMeta
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, meta, decoded)
	require.Contains(t, string(raw), "avoid_when")
}

// Enabled legacy reference roles remain queryable for one compatibility
// version, but must not reach any topic/board production prompt entry.
func TestAnalysisMethodLegacyRoleNoLongerInjectedIntoAnyPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	repo := repository.NewRepository(db)
	const sentinel = "LEGACY_AUTHOR_STYLE_MUST_NEVER_REACH_PROMPTS"
	const roleName = "analysis-method-legacy-prompt-test"
	t.Cleanup(func() { _ = db.Unscoped().Where("name = ?", roleName).Delete(&repository.ReferenceRole{}).Error })
	require.NoError(t, db.Unscoped().Where("name = ?", roleName).Delete(&repository.ReferenceRole{}).Error)
	require.NoError(t, repo.CreateReferenceRole(context.Background(), &repository.ReferenceRole{
		Name: roleName, Title: "旧作者画像", Content: sentinel, Enabled: true,
	}))

	router := &internalMockRouter{responses: []*airouter.ChatResult{
		{Content: `{"form":"sparse","form_reason":"素材不足","topics":[]}`},
		{Content: `{"action":"finish","summary":"done"}`},
		{Content: `{"form":"sparse","lens":"测试","analysis":{"notice":"不足","summary":"不足"}}`},
		{Content: `{"form":"board","candidates":[{"thesis":"T","hook":"H","angle":"A"}],"chosen_index":0,"reason":"R"}`},
	}}
	orch := &OrchestratorService{
		airouter: router, repo: repo, capability: internalTestCap,
		toolRegistry: NewRegistry(nil),
	}
	ctx := context.Background()

	_, err := orch.interpret(ctx, interpretContext{SessionID: "method-isolation", LifelineText: "脉络"})
	require.NoError(t, err)
	_, err = orch.runAgentLoop(ctx, "method-isolation", "主题", "视角", "脉络", nil)
	require.NoError(t, err)
	_, err = orch.analyze(ctx, "method-isolation", FormSparse, "视角", "脉络", "", nil)
	require.NoError(t, err)
	_, err = orch.boardInterpret(ctx, boardInterpretInput{
		SessionID: "method-isolation", CardsMD: "泳道事实", TopCard: &LaneSituationCard{LaneID: 1, Label: "泳道"},
	})
	require.NoError(t, err)

	boardPrompt := orch.assembleBoardAnalyzePrompt(ctx, "命题", "视角", "泳道事实", nil)
	topicHelperPrompt := orch.AssembleSingleLaneAnalyzePromptForTest(ctx, FormSparse, "视角", "脉络", "", nil)
	for i, call := range router.calls {
		require.NotEmpty(t, call.Messages)
		require.NotContainsf(t, call.Messages[0].Content, sentinel, "LLM call %d leaked legacy role", i)
	}
	require.NotContains(t, boardPrompt, sentinel)
	require.NotContains(t, topicHelperPrompt, sentinel)
}
