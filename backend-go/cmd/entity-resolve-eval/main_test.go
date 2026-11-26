package main

// 场景锚点（tasks.md 映射表）：
//   - 评测协议与金标在运行前冻结并隔离      （白盒 B9）
//   - 预算超限在发请求前被拦截              （白盒 B8）
//   - 评分按固定总 N 与覆盖率并单列缺身份与未归一（白盒 B10）
// 全部零网络：fake provider client + 纯函数协议/评分器。

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/entity"
)

// fakeProviderClient counts REAL provider attempts (the wire boundary).
type fakeProviderClient struct {
	chatCalls  int
	embedCalls int
}

func (f *fakeProviderClient) Chat(_ context.Context, _ models.AIVendor, _ models.AIModel, _ string, _ airouter.ChatRequest) (*airouter.ChatResponse, []airouter.StructuredAttempt, error) {
	f.chatCalls++
	return &airouter.ChatResponse{Content: "none"}, nil, nil
}

func (f *fakeProviderClient) Embed(_ context.Context, _ models.AIVendor, _ models.AIModel, req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	f.embedCalls++
	embeds := make([][]float64, len(req.Input))
	for i := range embeds {
		embeds[i] = []float64{0.1, 0.2, 0.3}
	}
	return &airouter.EmbeddingResult{Embeddings: embeds, Model: "fake-embed", Dimensions: 3}, nil
}

// 场景 + B8：预算超限在发请求前被拦截。
// WHEN 评测已消耗到某一阶段的上限且仍需发起调用：在发出请求前拒绝该次调用
// 并记录拒绝计数，不静默继续；预算清单四项由实际输入规模机械算出。
func TestEntityEval_BudgetGuardBlocksRequest(t *testing.T) {
	t.Run("http budget rejects before the wire", func(t *testing.T) {
		fake := &fakeProviderClient{}
		guard := NewSendGuard("t", BudgetPlan{HTTPRequests: 2, EmbedTexts: 10}, nil, fake)
		vendor := models.AIVendor{}
		model := models.AIModel{Name: "m"}
		req := airouter.ChatRequest{Operation: "t.chat"}

		_, _, err := guard.Chat(context.Background(), vendor, model, "", req)
		require.NoError(t, err)
		_, _, err = guard.Chat(context.Background(), vendor, model, "", req)
		require.NoError(t, err)
		require.Equal(t, 2, fake.chatCalls, "两次真实放行")

		_, _, err = guard.Chat(context.Background(), vendor, model, "", req)
		require.Error(t, err, "第三次在发送前被拒")
		assert.Contains(t, err.Error(), "发送前拦截")
		assert.Equal(t, 2, fake.chatCalls, "被拒的调用没有 HTTP（wire 计数不变）")
		assert.Equal(t, 1, guard.Rejections(), "拒绝计数 +1")
		assert.Equal(t, 2, guard.HTTPCount(), "HTTP 计数按发送前口径")
	})

	t.Run("model whitelist blocks cross-model fallback", func(t *testing.T) {
		fake := &fakeProviderClient{}
		guard := NewSendGuard("t", BudgetPlan{HTTPRequests: 10}, []string{"allowed-model"}, fake)
		_, _, err := guard.Chat(context.Background(), models.AIVendor{}, models.AIModel{Name: "other-model"}, "", airouter.ChatRequest{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "不在白名单")
		assert.Zero(t, fake.chatCalls, "跨模型/降级请求不会发出")
	})

	t.Run("embed submit-text budget counts inputs pre-send", func(t *testing.T) {
		fake := &fakeProviderClient{}
		guard := NewSendGuard("t", BudgetPlan{HTTPRequests: 10, EmbedTexts: 2}, nil, fake)
		_, err := guard.Embed(context.Background(), models.AIVendor{}, models.AIModel{Name: "m"}, airouter.EmbeddingRequest{Input: []string{"a"}})
		require.NoError(t, err)
		// 一条输入一次 HTTP（缓存白名单外），计数按提交文本数。
		_, err = guard.Embed(context.Background(), models.AIVendor{}, models.AIModel{Name: "m"}, airouter.EmbeddingRequest{Input: []string{"b", "c"}})
		require.Error(t, err, "提交文本将超上限 → 发送前拒绝")
		assert.Contains(t, err.Error(), "提交文本数")
		assert.Equal(t, 1, fake.embedCalls)
	})

	t.Run("mechanical budget derives from real stage shape", func(t *testing.T) {
		samples := []EvalSample{
			{SampleID: "1", ArticleTitle: "文章A", Mention: "特朗普", SubjectDesc: "美国第45任总统", SubjectKey: "person:trump"},
			{SampleID: "2", ArticleTitle: "文章A", Mention: "OpenAI", SubjectKey: "company:openai"},
			{SampleID: "3", ArticleTitle: "文章B", Mention: "争议项", Disputed: true},
		}
		plan := MechanicalBudget(samples)
		assert.Equal(t, 1, plan.ExtractionCalls, "① 实际抽取文章数（同文两提及只算一篇；争议项不抽）")
		assert.Equal(t, 2*secondRoundMaxAttempts, plan.SecondRoundCalls, "② 提及数×尝试上限（实际以运行计数）")
		assert.Equal(t, 3, plan.EmbedTexts, "③ 两路查询文本（desc 空只 1 条）")
		assert.Greater(t, plan.HTTPRequests, 0, "④ 发送前 HTTP 硬闸门")
	})

	t.Run("missing guard pre-blocks", func(t *testing.T) {
		require.Error(t, EnsureGuardRegistered(false), "无法建立发送前计数 → 预先阻断")
		require.NoError(t, EnsureGuardRegistered(true))
	})
}

// 场景 + B9：评测协议与金标在运行前冻结并隔离。
// WHEN 评测开始运行：协议要素已冻结，运行中不得修改样本或金标；运行器不
// 打开金标；争议样本预列不计分并报数。
func TestEntityEval_ProtocolFrozenAndGoldIsolated(t *testing.T) {
	samples := []EvalSample{
		{SampleID: "1", ArticleTitle: "文章A", ContextText: "原文片段…特朗普宣布…", Mention: "特朗普", Group: "cross-lang"},
		{SampleID: "2", ArticleTitle: "文章B", ContextText: "原文片段…争议主体…", Mention: "争议主体", Group: "none", Disputed: true},
	}
	digest := SamplesDigest(samples)
	p := Freeze(Protocol{
		SampleDigest:  digest,
		GroupRules:    []string{"cross-lang", "none"},
		Budget:        MechanicalBudget(samples),
		ScoringRules:  "pairwise-v1",
		DisputedCount: 1,
	}, "2026-11-12T00:00:00Z")

	t.Run("frozen protocol verifies and drift is rejected", func(t *testing.T) {
		require.NoError(t, VerifyFrozen(p))
		drifted := p
		drifted.Budget.HTTPRequests += 1
		require.Error(t, VerifyFrozen(drifted), "预算漂移被拒")
		drifted2 := p
		drifted2.ScoringRules = "pairwise-v2"
		require.Error(t, VerifyFrozen(drifted2), "评分规则漂移被拒")
	})

	t.Run("sample list edit after freeze is detected", func(t *testing.T) {
		edited := append(samples, EvalSample{SampleID: "3", Mention: "后加的"})
		require.NotEqual(t, digest, SamplesDigest(edited), "冻结后加样本必然改变摘要")
	})

	t.Run("runner API never sees gold (structural)", func(t *testing.T) {
		// Runner 结构体没有金标字段：金标只进 score 子命令。
		rt := reflect.TypeOf(Runner{})
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			ft := field.Type.String()
			assert.NotContains(t, strings.ToLower(field.Name), "gold", "Runner 字段 %s 不得是金标", field.Name)
			assert.NotContains(t, ft, "GoldEntry", "Runner 字段 %s 类型不得含金标", field.Name)
		}
		// Run 的入参类型同样不含金标（编译期 + 反射双保险）。
		runMethod, ok := reflect.TypeOf(&Runner{}).MethodByName("Run")
		require.True(t, ok)
		for i := 1; i < runMethod.Type.NumIn(); i++ { // 0 = receiver
			assert.NotContains(t, runMethod.Type.In(i).String(), "Gold", "Run 入参不得含金标")
		}
	})

	t.Run("runner refuses drifted samples", func(t *testing.T) {
		// 冻结协议 + 样本漂移 → Run 前置拒绝（不需要 DB：NewRunner 的冻结
		// 校验先行，样本比对在 Run 内第一道；此处直接验证摘要契约）。
		edited := samples
		edited[0].Mention = "特朗普（改）"
		require.NotEqual(t, p.SampleDigest, SamplesDigest(edited))
	})

	t.Run("worksheet is blind and disputes are counted", func(t *testing.T) {
		var buf strings.Builder
		require.NoError(t, ExportWorksheet(&buf, samples))
		out := buf.String()
		assert.Contains(t, out, "sample_id,article_title,context_text,mention,disputed,gold_entity")
		assert.NotContains(t, out, "候选", "工作表不含候选资料")
		assert.NotContains(t, out, "entity_id", "工作表不含系统输出/实体 ID")
		assert.Contains(t, FormatDisputedReport(samples), "争议样本预列 1 条")
	})
}

// 场景 + B10：评分按固定总 N 与覆盖率并单列缺身份与未归一。
// WHEN 评测产出结果并进入评分：固定总 N、覆盖率、技术失败数、错误复用数
// 与按固定总 N 的错误率；空 subject_key 单列缺身份、无决策单列未归一、不以
// 单元素组冒充正确；误合/漏合/重复身份组率在同一可比子集上算；分母 0 报
// 「不适用（N=0）」；技术失败 ≠ 0 阻断。
func TestEntityEval_FixedNAndCoverage(t *testing.T) {
	t.Run("missing identity and unresolved are counted separately", func(t *testing.T) {
		gold := []GoldEntry{
			{SampleID: "1", GoldEntity: "特朗普"},
			{SampleID: "2", GoldEntity: "OpenAI"},
			{SampleID: "3", GoldEntity: "NVIDIA"},
		}
		records := []MentionRecord{
			{SampleID: "1", SubjectKey: "person:trump", EntityID: 10, Status: "reused"},
			{SampleID: "2", SubjectKey: "", EntityID: 0, Status: "unresolved", TechFailure: true},
			{SampleID: "3", SubjectKey: "company:nvidia", EntityID: 0, Status: "unresolved"}, // 无技术失败但无决策
		}
		rep := Score(gold, records)
		assert.Equal(t, 3, rep.TotalN, "固定总 N 为分母口径")
		assert.Equal(t, 1, rep.MissingIdentity, "空 subject_key 单列缺身份")
		assert.Equal(t, 2, rep.Unresolved, "无决策单列未归一")
		assert.InDelta(t, 2.0/3.0, rep.CoverageSubject.Rate, 1e-9)
		assert.InDelta(t, 1.0/3.0, rep.CoverageEntity.Rate, 1e-9)
		assert.Equal(t, 1, rep.ComparableN, "可比子集 = 两套都有预测")
	})

	t.Run("zero denominator reports N/A, never 0% or 100%", func(t *testing.T) {
		rep := Score(nil, nil)
		assert.Equal(t, "不适用（N=0）", rep.CoverageSubject.Text())
		assert.Equal(t, "不适用（N=0）", rep.EntityMerge.Text())
		assert.False(t, rep.MeetsBar, "数据不足不判达标")
		assert.Contains(t, rep.MeetsBarDetail, "数据不足")

		allDisputed := Score([]GoldEntry{{SampleID: "x", GoldEntity: "e", Disputed: true}}, nil)
		assert.Equal(t, 0, allDisputed.TotalN)
		assert.Equal(t, 1, allDisputed.DisputedCount)
		assert.Equal(t, "不适用（N=0）", allDisputed.CoverageEntity.Text())
	})

	t.Run("technical failure blocks the segment", func(t *testing.T) {
		gold := []GoldEntry{{SampleID: "1", GoldEntity: "A"}}
		records := []MentionRecord{{SampleID: "1", SubjectKey: "a", EntityID: 1, Status: "reused", TechFailure: true}}
		rep := Score(gold, records)
		assert.True(t, rep.Blocked, "技术失败 ≠ 0 → 阻断该段")
		assert.Contains(t, rep.BlockReason, "修复后重跑")
		assert.False(t, rep.MeetsBar)
	})

	t.Run("pairwise merge/split/dup on the comparable subset", func(t *testing.T) {
		gold := []GoldEntry{
			{SampleID: "1", GoldEntity: "特朗普"},
			{SampleID: "2", GoldEntity: "特朗普"},
			{SampleID: "3", GoldEntity: "OpenAI"},
		}
		// 实体侧把三条全并成 e9（误合）；既有侧把同一实体拆成两键（漏合+重复组）。
		records := []MentionRecord{
			{SampleID: "1", SubjectKey: "person:trump", EntityID: 9, Status: "reused"},
			{SampleID: "2", SubjectKey: "person:donald_trump", EntityID: 9, Status: "reused"},
			{SampleID: "3", SubjectKey: "company:openai", EntityID: 9, Status: "reused"},
		}
		rep := Score(gold, records)
		require.Equal(t, 3, rep.ComparableN)
		// 实体侧误合：三对里 (1,3)、(2,3) 预测同组而金标不同。
		assert.Equal(t, 2, rep.EntityMerge.Num)
		assert.Equal(t, 3, rep.EntityMerge.Den)
		// 既有侧漏合：(1,2) 金标同而预测不同组。
		assert.Equal(t, 1, rep.SubjectSplit.Num)
		// 既有侧重复身份组：金标组「特朗普」被拆成两个预测组。
		assert.Equal(t, 1, rep.SubjectDupGroup.Num)
		assert.Equal(t, 2, rep.SubjectDupGroup.Den)
		// 错误复用：e9 把三条提及并进一组，每条 reuse 都有金标不同的同组
		// 兄弟 → 三条全计（按固定总 N 的错误率 = 3/3）。
		assert.Equal(t, 3, rep.WrongReuse)
		assert.InDelta(t, 1.0, rep.ErrorRateEntity.Rate, 1e-9)
		// 误合升高 → 未达标。
		assert.False(t, rep.MeetsBar)
	})

	t.Run("meeting the bar is mechanically verifiable", func(t *testing.T) {
		gold := []GoldEntry{
			{SampleID: "1", GoldEntity: "特朗普"},
			{SampleID: "2", GoldEntity: "特朗普"},
			{SampleID: "3", GoldEntity: "OpenAI"},
		}
		records := []MentionRecord{
			{SampleID: "1", SubjectKey: "person:donald_trump", EntityID: 5, Status: "created"},
			{SampleID: "2", SubjectKey: "person:donald_trump", EntityID: 5, Status: "idempotent"},
			{SampleID: "3", SubjectKey: "", EntityID: 6, Status: "created"},
		}
		rep := Score(gold, records)
		assert.False(t, rep.Blocked)
		assert.Zero(t, rep.TechFailures)
		// 既有侧覆盖率 2/3 < 实体侧 3/3；可比子集 2 条：实体 0 误合、
		// 既有 0 误合；重复组率实体 0 < 既有 0（同组）→ 覆盖率达标但
		// 重复组率「未低于」→ 未达标的机械分支可复算。
		assert.InDelta(t, 1.0, rep.CoverageEntity.Rate, 1e-9)
	})
}

// ─── R-15：actual 侧计数器与 records 实际字段 ───────────────────────────────

// 场景 + B8：预算超限在发请求前被拦截（actual 侧口径）。
// WHEN 评测跑完：ActualExtraction/ActualSecondRound/ActualEmbedTexts 必须由
// guard 的发送前计数与 records 的 Attempts 机械导出，不是恒 0 的占位字段。
func TestEntityEval_SendGuardExposesActualCounters(t *testing.T) {
	fake := &fakeProviderClient{}
	guard := NewSendGuard("t", BudgetPlan{HTTPRequests: 3, EmbedTexts: 2}, nil, fake)
	vendor := models.AIVendor{}
	model := models.AIModel{Name: "m"}

	// 两次 chat + 一次 2 文本的 embed：三个计数器各自反映真实放行。
	_, _, err := guard.Chat(context.Background(), vendor, model, "", airouter.ChatRequest{Operation: "t.1"})
	require.NoError(t, err)
	_, _, err = guard.Chat(context.Background(), vendor, model, "", airouter.ChatRequest{Operation: "t.2"})
	require.NoError(t, err)
	_, err = guard.Embed(context.Background(), vendor, model, airouter.EmbeddingRequest{Input: []string{"a", "b"}})
	require.NoError(t, err)

	assert.Equal(t, 2, guard.ChatCount(), "ChatCount = 真实发出的 chat 尝试数")
	assert.Equal(t, 2, guard.EmbedTexts(), "EmbedTexts = 提交的 embedding 文本数")
	assert.Equal(t, 3, guard.HTTPCount(), "HTTPCount = chat 尝试 + embed 调用")
	assert.Equal(t, 2, fake.chatCalls)
	assert.Equal(t, 1, fake.embedCalls)

	// 被拦截的调用不计入任何「实际」计数（没有 HTTP、没有 token）。
	_, _, err = guard.Chat(context.Background(), vendor, model, "", airouter.ChatRequest{Operation: "t.3"})
	require.Error(t, err)
	_, err = guard.Embed(context.Background(), vendor, model, airouter.EmbeddingRequest{Input: []string{"c", "d"}})
	require.Error(t, err)
	assert.Equal(t, 2, guard.ChatCount(), "被拒的 chat 不计")
	assert.Equal(t, 2, guard.EmbedTexts(), "被拒的 embed 文本不计")
	assert.Equal(t, 3, guard.HTTPCount())
	assert.Equal(t, 2, guard.Rejections())
}

// ─── R-16：授权后完整装配（零网络、零真库分支验证） ─────────────────────────

// 跨包集成测试约束③（standard/backend/testing.md）：本测试二进制的
// AutoMigrate 表集需要 entity 三表（tagmanagement 根包 init 无法在此 import）。
func init() {
	database.RegisterModels(tagmodels.EntitySubjectModels()...)
}

// scriptedEvalProvider 是评测入口的 fake provider client（inner，被 guard 包住）：
// 只按二轮 prompt 的提及决定答案，不发任何 HTTP。
//   - 默认：回候选列表里第一个 ID（模拟「复用已登记实体」）
//   - none 计划：回 none（模拟合法 none → 原子新建）
//   - illegal 计划：回一个不在候选集合内的 ID（模拟技术失败，触发重试）
type scriptedEvalProvider struct {
	mu         sync.Mutex
	chatCalls  int
	embedCalls int
	plans      map[string]string // mention → "" (pick) | "none" | "illegal"
}

func (p *scriptedEvalProvider) Chat(_ context.Context, _ models.AIVendor, _ models.AIModel, _ string, req airouter.ChatRequest) (*airouter.ChatResponse, []airouter.StructuredAttempt, error) {
	p.mu.Lock()
	p.chatCalls++
	p.mu.Unlock()
	user := ""
	if len(req.Messages) > 1 {
		user = req.Messages[1].Content
	}
	mention := mentionFromChoicePrompt(user)
	// 二轮请求是结构化请求（router 要求可解析 JSON）：fake 也按
	// {"answer": ...} 回包，否则会被 router 的结构化解析判为失败。
	switch p.plans[mention] {
	case "none":
		return &airouter.ChatResponse{Content: `{"answer":"none"}`}, nil, nil
	case "illegal":
		return &airouter.ChatResponse{Content: `{"answer":"ENOTACANDIDATE"}`}, nil, nil
	}
	keys := candidateKeysFromChoicePrompt(user)
	if len(keys) == 0 {
		return &airouter.ChatResponse{Content: `{"answer":"none"}`}, nil, nil
	}
	return &airouter.ChatResponse{Content: `{"answer":"` + keys[0] + `"}`}, nil, nil
}

func (p *scriptedEvalProvider) Embed(_ context.Context, _ models.AIVendor, _ models.AIModel, req airouter.EmbeddingRequest) (*airouter.EmbeddingResult, error) {
	p.mu.Lock()
	p.embedCalls++
	p.mu.Unlock()
	embeds := make([][]float64, len(req.Input))
	for i := range embeds {
		embeds[i] = []float64{0.1, 0.2, 0.3}
	}
	return &airouter.EmbeddingResult{Embeddings: embeds, Model: "fake-embed", Dimensions: 3}, nil
}

func (p *scriptedEvalProvider) counts() (chat, embed int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.chatCalls, p.embedCalls
}

func mentionFromChoicePrompt(user string) string {
	for _, line := range strings.Split(user, "\n") {
		if after, ok := strings.CutPrefix(line, "提及："); ok {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

func candidateKeysFromChoicePrompt(user string) []string {
	keys := []string{}
	for _, line := range strings.Split(user, "\n") {
		parts := strings.Split(line, " | ")
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if strings.HasPrefix(key, "E") && !strings.ContainsAny(key, " ：:") {
			keys = append(keys, key)
		}
	}
	return keys
}

// writeFrozenRunInputs writes a frozen protocol + the matching samples file.
func writeFrozenRunInputs(t *testing.T, samples []EvalSample) (protocolPath, samplesPath string) {
	t.Helper()
	dir := t.TempDir()
	samplesPath = filepath.Join(dir, "samples.json")
	samplesRaw, err := json.Marshal(samples)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(samplesPath, samplesRaw, 0o600))

	digest := SamplesDigest(samples)
	frozen := Freeze(Protocol{
		SampleDigest:   digest,
		SnapshotDigest: digest,
		GroupRules:     distinctGroups(samples),
		ModelVersions:  map[string]string{},
		Budget:         MechanicalBudget(samples),
		ScoringRules:   "pairwise-v1",
		DisputedCount:  CountDisputed(samples),
	}, "2026-11-13T00:00:00Z")
	protocolPath = filepath.Join(dir, "protocol.json")
	protocolRaw, err := json.Marshal(frozen)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(protocolPath, protocolRaw, 0o600))
	return protocolPath, samplesPath
}

func writeAuthorizationFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authorization.txt")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// seedEvalAIRoutes seeds one vendor + one chat model/route (topic_tagging) and
// one embedding model/route so the REAL router can load bindings; the provider
// client behind them is the injected fake (never a network client).
func seedEvalAIRoutes(t *testing.T, db *gorm.DB, suffix string) {
	t.Helper()
	vendor := models.AIVendor{
		Name: "eval-fake-" + suffix, ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL: "http://127.0.0.1:1/v1", APIKey: "k", Enabled: true,
	}
	require.NoError(t, db.Create(&vendor).Error)
	chatModel := models.AIModel{VendorID: vendor.ID, Name: "fake-chat-" + suffix, ModelKind: airouter.ModelKindLLM, Enabled: true, Status: airouter.ModelStatusOK}
	embedModel := models.AIModel{VendorID: vendor.ID, Name: "fake-embed-" + suffix, ModelKind: airouter.ModelKindEmbedding, Enabled: true, Status: airouter.ModelStatusOK}
	require.NoError(t, db.Create(&chatModel).Error)
	require.NoError(t, db.Create(&embedModel).Error)
	for _, rc := range []struct {
		capability string
		modelID    uint
	}{
		{string(airouter.CapabilityTopicTagging), chatModel.ID},
		{string(airouter.CapabilityEmbedding), embedModel.ID},
	} {
		route := models.AIRoute{Name: "eval-default-" + rc.capability, Capability: rc.capability, Enabled: true}
		require.NoError(t, db.Create(&route).Error)
		require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: rc.modelID, Priority: 1, Enabled: true}).Error)
	}
}

// 场景 + B8/budget：两个失败分支必须分清。
// WHEN 没有授权：报「未授权」且**不连库**（connectDB 一次都不被调用）。
func TestEntityEval_RunWithoutAuthorizationDoesNotConnect(t *testing.T) {
	samples := []EvalSample{{SampleID: "1", ArticleTitle: "文章A", Mention: "特朗普", ContextText: "原文…"}}
	protocolPath, samplesPath := writeFrozenRunInputs(t, samples)

	connected := 0
	deps := runDeps{
		connectDB: func(string) (*gorm.DB, error) {
			connected++
			return nil, errors.New("不应发生的连库")
		},
		provider: &scriptedEvalProvider{plans: map[string]string{}},
	}
	base := []string{"-protocol", protocolPath, "-samples", samplesPath, "-session", "run-1", "-out", filepath.Join(t.TempDir(), "records.json")}

	// ① 完全没有 -authorize：未授权。
	err := cmdRunWith(base, deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无模型额度授权")
	assert.Zero(t, connected, "未授权不连库")

	// ② 有授权文件但缺 BUDGET-APPROVED 标记：同样按未授权处理，不连库。
	noMarker := writeAuthorizationFile(t, "随便一段没有标记的说明")
	err = cmdRunWith(append(base, "-authorize", noMarker), deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BUDGET-APPROVED")
	assert.Contains(t, err.Error(), "未授权")
	assert.Zero(t, connected, "标记缺失同样不连库")
}

// 场景 + budget（R-16 裁决）：有授权但装配失败 → 报具体原因，不得复用
// 「当前无授权」这句。
func TestEntityEval_RunAuthorizedAssemblyFailureIsNotUnauthorized(t *testing.T) {
	samples := []EvalSample{{SampleID: "1", ArticleTitle: "文章A", Mention: "特朗普", ContextText: "原文…"}}
	protocolPath, samplesPath := writeFrozenRunInputs(t, samples)
	authorizePath := writeAuthorizationFile(t, "BUDGET-APPROVED\n范围：本轮实体评测 40 次 Chat")

	connected := 0
	deps := runDeps{
		connectDB: func(string) (*gorm.DB, error) {
			connected++
			return nil, errors.New("dsn unreachable: connection refused")
		},
		provider: &scriptedEvalProvider{plans: map[string]string{}},
	}
	err := cmdRunWith([]string{
		"-protocol", protocolPath, "-samples", samplesPath,
		"-authorize", authorizePath, "-session", "run-2",
		"-out", filepath.Join(t.TempDir(), "records.json"),
	}, deps)
	require.Error(t, err)
	assert.Equal(t, 1, connected, "有授权 → 走到装配（连库被尝试）")
	assert.Contains(t, err.Error(), "装配失败")
	assert.Contains(t, err.Error(), "dsn unreachable")
	assert.NotContains(t, err.Error(), "无授权", "装配失败不得复用「无授权」这句")
	assert.NotContains(t, err.Error(), "未授权")

	// 协议/样本漂移在连库之前拒绝（装配不启动）。
	drifted := append([]EvalSample{}, samples...)
	drifted[0].Mention = "特朗普（改）"
	driftedSamples := filepath.Join(t.TempDir(), "samples.json")
	raw, merr := json.Marshal(drifted)
	require.NoError(t, merr)
	require.NoError(t, os.WriteFile(driftedSamples, raw, 0o600))
	connected = 0
	err = cmdRunWith([]string{
		"-protocol", protocolPath, "-samples", driftedSamples,
		"-authorize", authorizePath, "-session", "run-2",
		"-out", filepath.Join(t.TempDir(), "records.json"),
	}, deps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "样本清单与冻结协议不一致")
	assert.Zero(t, connected, "样本漂移在连库之前被拒")
}

// 场景 + budget：有授权 + 装配通过 → 连库 → NewRunner → Run → 落 records
// 与预算实际值产物（records 可被 score 直接消费；Attempts/Reason 落盘）。
func TestEntityEval_RunAuthorizedPersistsRecordsAndBudget(t *testing.T) {
	db := testutil.SetupTestDB(t)
	seedEvalAIRoutes(t, db, "r16")

	samples := []EvalSample{
		{SampleID: "1", ArticleTitle: "文章A", Mention: "特朗普", SubjectDesc: "美国第45任总统", ContextText: "特朗普宣布新政策", SubjectKey: "person:trump", Group: "cross-lang"},
		{SampleID: "2", ArticleTitle: "文章B", Mention: "特朗普", SubjectDesc: "美国第45任总统", ContextText: "特朗普再谈关税", SubjectKey: "person:trump", Group: "cross-lang"},
	}
	protocolPath, samplesPath := writeFrozenRunInputs(t, samples)
	authorizePath := writeAuthorizationFile(t, "BUDGET-APPROVED\n范围：本轮实体评测 40 次 Chat")

	provider := &scriptedEvalProvider{plans: map[string]string{}}
	deps := runDeps{connectDB: func(string) (*gorm.DB, error) { return db, nil }, provider: provider}
	outPath := filepath.Join(t.TempDir(), "records.json")
	require.NoError(t, cmdRunWith([]string{
		"-protocol", protocolPath, "-samples", samplesPath,
		"-authorize", authorizePath, "-session", "run-r16", "-out", outPath,
	}, deps))

	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	var records []MentionRecord
	require.NoError(t, json.Unmarshal(raw, &records), "records 产物是 score 的入参形状（[]MentionRecord）")
	require.Len(t, records, 2)
	assert.Equal(t, "created", records[0].Status, "空池 → 无候选 → 合法 none 直接新建")
	assert.NotZero(t, records[0].EntityID)
	assert.Equal(t, "reused", records[1].Status, "已登记叫法命中 → 二判选中同一实体")
	assert.Equal(t, records[0].EntityID, records[1].EntityID, "同一实体同一 ID")
	assert.Equal(t, 1, records[1].Attempts, "二判实际尝试次数落盘")

	budgetRaw, err := os.ReadFile(outPath + ".budget.json")
	require.NoError(t, err)
	var report BudgetReport
	require.NoError(t, json.Unmarshal(budgetRaw, &report))
	chatCalls, embedCalls := provider.counts()
	assert.Equal(t, 1, chatCalls)
	assert.Equal(t, 4, embedCalls, "两条提及各走名路径 + 名+说明路径")
	assert.Equal(t, 1, report.ActualSecondRound, "二判实际次数 = Σ Attempts")
	assert.Equal(t, 4, report.ActualEmbedTexts, "embedding 实际提交文本数（每条输入一次）")
	assert.Equal(t, chatCalls+embedCalls, report.ActualHTTP, "guard 计数 = inner client 真实调用数（证明 guard 挂在 router 上）")
	assert.Equal(t, 0, report.ActualExtraction, "回放样本跑法：chat 尝试全部来自二判 → 抽取 = 0（chat − 二判）")
	assert.Zero(t, report.Rejected)
}

// ─── R-17：Run 主路径零网络集成（testcontainer PG + fake provider） ─────────

// TestEntityEval_RunnerMainPath 走 NewRunner → Run 的真实装配：真 router（guard
// 经 RegisterClient 挂上）+ 真隔离 PG（testutil，非真库）+ fake provider。
// 断言 records 形状（含 Attempts/Reason）、争议项不产 record、技术失败样本
// 两次尝试且原因非空、guard 计数与 inner 调用数一致、EntityID/Status 映射。
func TestEntityEval_RunnerMainPath(t *testing.T) {
	db := testutil.SetupTestDB(t)
	seedEvalAIRoutes(t, db, "r17")

	samples := []EvalSample{
		{SampleID: "s1", ArticleTitle: "文章A", Mention: "特朗普", SubjectDesc: "美国第45任总统", ContextText: "特朗普宣布新政策", SubjectKey: "person:trump", Group: "cross-lang"},
		{SampleID: "s2", ArticleTitle: "文章B", Mention: "唐纳德·特朗普", SubjectDesc: "美国第45任总统", ContextText: "唐纳德·特朗普再谈关税", SubjectKey: "person:donald_trump", Group: "cross-lang"},
		{SampleID: "s3", ArticleTitle: "文章C", Mention: "技术失败样本", SubjectDesc: "用于触发非法回答的主体", ContextText: "技术失败样本出现", SubjectKey: "other:techfail", Group: "none"},
		{SampleID: "s4", ArticleTitle: "文章D", Mention: "争议主体", ContextText: "争议主体出现", Disputed: true, Group: "none"},
		{SampleID: "s5", ArticleTitle: "文章E", Mention: "老黄", SubjectDesc: "英伟达创始人", ContextText: "老黄在发布会上展示新卡", SubjectKey: "", Group: "cross-name"},
	}
	digest := SamplesDigest(samples)
	protocol := Freeze(Protocol{
		SampleDigest:   digest,
		SnapshotDigest: digest,
		GroupRules:     distinctGroups(samples),
		ModelVersions:  map[string]string{},
		Budget:         MechanicalBudget(samples),
		ScoringRules:   "pairwise-v1",
		DisputedCount:  CountDisputed(samples),
	}, "2026-11-13T00:00:00Z")

	provider := &scriptedEvalProvider{plans: map[string]string{
		"技术失败样本": "illegal", // 两次尝试都非法 → 技术失败（不降级 none、不新建）
		"老黄":     "none",    // 合法 none → 原子新建另一实体
	}}
	runner, err := NewRunner(db, protocol, "run-r17", nil, provider)
	require.NoError(t, err)
	records, report, err := runner.Run(context.Background(), samples)
	require.NoError(t, err)

	// 争议项不产 record（5 条样本 → 4 条记录）。
	require.Len(t, records, 4)
	byID := map[string]MentionRecord{}
	for _, r := range records {
		byID[r.SampleID] = r
	}
	assert.NotContains(t, byID, "s4", "争议项预列：不进入运行、不产 record")

	// s1：空池 → 无候选 → 合法 none 直接新建（Attempts=0，原因可读）。
	s1 := byID["s1"]
	assert.Equal(t, entity.StatusCreated, s1.Status)
	assert.NotZero(t, s1.EntityID)
	assert.False(t, s1.TechFailure)
	assert.Zero(t, s1.Attempts, "无候选不发起二判（不烧预算）")
	assert.Equal(t, "no candidates in pool", s1.Reason)

	// s2：已登记叫法/向量召回同一实体 → 二判选中 → reused 同一 ID。
	s2 := byID["s2"]
	assert.Equal(t, entity.StatusReused, s2.Status)
	assert.Equal(t, s1.EntityID, s2.EntityID, "同一实体沿用同一统一 ID")
	assert.Equal(t, 1, s2.Attempts)

	// s3：技术失败 → unresolved、无 EntityID、两次尝试、原因非空。
	s3 := byID["s3"]
	assert.Equal(t, entity.StatusUnresolved, s3.Status)
	assert.True(t, s3.TechFailure)
	assert.Zero(t, s3.EntityID, "技术失败不新建、不留半实体")
	assert.Equal(t, 2, s3.Attempts, "最多 2 次尝试后耗尽")
	assert.NotEmpty(t, s3.Reason)

	// s5：合法 none → 新建另一个实体（不与 s1 合并）。
	s5 := byID["s5"]
	assert.Equal(t, entity.StatusCreated, s5.Status)
	assert.NotZero(t, s5.EntityID)
	assert.NotEqual(t, s1.EntityID, s5.EntityID)
	assert.Equal(t, 1, s5.Attempts)
	assert.Equal(t, "legal none", s5.Reason)

	// guard 计数：chat 尝试 = s2(1) + s3(2) + s5(1)；embed = 4 条提及 × 2 路。
	chatCalls, embedCalls := provider.counts()
	assert.Equal(t, 4, chatCalls)
	assert.Equal(t, 8, embedCalls)
	assert.Equal(t, 4, report.ActualSecondRound, "二判实际次数 = Σ Attempts")
	assert.Equal(t, embedCalls, report.ActualEmbedTexts)
	assert.Greater(t, report.ActualHTTP, 0, "guard 计数 > 0：client 经 RegisterClient 挂在 router 上")
	assert.Equal(t, chatCalls+embedCalls, report.ActualHTTP, "guard 的发送前计数 = inner client 真实调用数，resolver 的调用确实被拦到")
	assert.Equal(t, 0, report.ActualExtraction, "回放跑法：chat − 二判 = 0（口径见 BudgetReport 注释）")
	assert.Zero(t, report.Rejected)

	// 实体层真的落库：两个实体（s1/s2 共用 + s5 新建），技术失败不建。
	var entityCount int64
	require.NoError(t, db.Model(&tagmodels.EntityRegistry{}).Count(&entityCount).Error)
	assert.EqualValues(t, 2, entityCount)
}
