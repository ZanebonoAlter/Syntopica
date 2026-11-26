package entity

// 场景锚点（tasks.md 映射表）：
//   - 新主体首次出现走完抽取到原子登记
//   - 合法 none 与技术失败是不同结论
//   - 同来源同提及重放幂等
// 白盒 B1：TestEntityResolver_TechnicalFailureDistinctFromNone

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/textutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	repository "syntopica-backend/internal/tagmanagement/repository"
)

// ---- fakes ----

type fakeChat struct {
	responses []string // consumed in order; last one repeats
	errs      []error  // parallel error override; nil = use response
	calls     int
	lastReq   airouter.ChatRequest
	delay     time.Duration // simulate latency (timeout variants)
	pickFirst bool          // answer with the first non-none enum value of the request schema
}

func (f *fakeChat) chat(ctx context.Context, req airouter.ChatRequest) (string, error) {
	f.calls++
	f.lastReq = req
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	idx := f.calls - 1
	if idx < len(f.errs) && f.errs[idx] != nil {
		return "", f.errs[idx]
	}
	if f.pickFirst {
		if prop, ok := req.JSONSchema.Properties["answer"]; ok {
			for _, opt := range prop.Enum {
				if opt != "none" {
					return opt, nil
				}
			}
		}
		return "none", nil
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	if len(f.responses) > 0 {
		return f.responses[len(f.responses)-1], nil
	}
	return "", nil
}

type fakeEmbed struct {
	byText map[string][]float64
	calls  int
}

func (f *fakeEmbed) embed(_ context.Context, text string) (QueryVector, error) {
	f.calls++
	vec, ok := f.byText[text]
	if !ok {
		vec = []float64{1, 0, 0}
	}
	return QueryVector{Vec: vec, Triplet: SpaceTriplet{Model: "fake-embed", Version: "fake-embed", Dim: len(vec)}}, nil
}

func fastConfig() ResolverConfig {
	return ResolverConfig{
		TopK:            10,
		MaxOptions:      12,
		MaxAttempts:     2,
		CallTimeout:     500 * time.Millisecond,
		GateWaitTimeout: 3 * time.Second,
		TotalTimeout:    10 * time.Second,
	}
}

func newTestResolver(t *testing.T, chat ChatFunc, embed EmbedFunc) (*Resolver, *repository.EntityRepository) {
	t.Helper()
	repo, _ := setupEntityDB(t)
	return NewResolver(repo, chat, embed, fastConfig()), repo
}

// seedEntity pre-registers an entity with a registered mention so later
// resolutions see a non-empty pool.
func seedEntity(t *testing.T, repo *repository.EntityRepository, key, name, mentionText string, nameVec []float64) *tagmodels.EntityRegistry {
	t.Helper()
	reg := &tagmodels.EntityRegistry{
		EntityKey:            key,
		CanonicalName:        name,
		NameEmbedding:        strPtr(repository.EntityFloatsToPgVector(nameVec)),
		NameEmbeddingModel:   "fake-embed",
		NameEmbeddingVersion: "fake-embed", // 与 fakeEmbed 三元组一致（model==version）
		NameEmbeddingDim:     len(nameVec),
	}
	m := &tagmodels.EntityMention{MentionText: mentionText, MentionKey: textutil.NormalizeEntityText(mentionText)}
	require.NoError(t, repo.CreateEntityAtomic(context.Background(), reg, m, nil))
	return reg
}

// 场景：新主体首次出现走完抽取到原子登记。
// WHEN 影子态下某篇真实文章的一次主体提及在当前池内无候选：
// 用该次第一轮真实产出的主体原文与稳定短身份说明走完检索 → 二次选择 →
// 登记全过程；模型输出合法 none 时新建实体并把该提及归到它，不生成待人工
// 确认条目；三行在同一短事务内写成，无半实体。
func TestEntityResolver_FirstMentionAtomicRegistration(t *testing.T) {
	chat := &fakeChat{responses: []string{"none"}}
	embed := &fakeEmbed{byText: map[string][]float64{}}
	rs, repo := newTestResolver(t, chat.chat, embed.embed)

	tagID := uint(1001)
	articleID := uint(77)
	res := rs.ResolveMention(context.Background(), MentionRequest{
		MentionText:     "特朗普",
		SubjectDesc:     "美国第45任总统",
		ContextSnippet:  "特朗普宣布…",
		SourceTagID:     &tagID,
		SourceArticleID: &articleID,
	})

	// 空池 → 无候选 → 直接合法 none（无二轮调用），原子新建。
	require.NotNil(t, res)
	assert.Equal(t, StatusCreated, res.Status)
	assert.NotZero(t, res.EntityID)
	assert.Equal(t, 0, res.Attempts, "空候选集不烧二轮调用")
	assert.Equal(t, 0, chat.calls)

	// 身份行 + 证据行同事务写成；规范名 = 提及原文，短说明真实留存。
	reg, err := repo.FindEntityByID(context.Background(), res.EntityID)
	require.NoError(t, err)
	assert.Equal(t, "特朗普", reg.CanonicalName)
	assert.Equal(t, "美国第45任总统", reg.StableDesc)
	assert.Equal(t, "fake-embed", reg.NameEmbeddingModel)
	assert.Equal(t, 3, reg.NameEmbeddingDim)

	// 原子性（无半实体）：事务中途失败（aux FK 违反）→ 全回滚。
	embed2 := &fakeEmbed{byText: map[string][]float64{}}
	chat2 := &fakeChat{responses: []string{"none"}}
	rs2 := NewResolver(repo, chat2.chat, embed2.embed, fastConfig())
	tagID2 := uint(1002)
	res2 := rs2.ResolveMention(context.Background(), MentionRequest{
		MentionText: "新主体",
		SourceTagID: &tagID2,
		AuxLabelIDs: []uint{987654321}, // 不存在的 aux id → 溯源行插入失败
	})
	require.NotNil(t, res2)
	assert.Equal(t, StatusUnresolved, res2.Status, "任一步失败 → 未归一（整体回滚）")
	assert.Contains(t, res2.Reason, "rolled back")
	var cnt int64
	require.NoError(t, repo.DB().Model(&tagmodels.EntityRegistry{}).Where("canonical_name = ?", "新主体").Count(&cnt).Error)
	assert.EqualValues(t, 0, cnt, "无半实体：回滚后身份行不存在")
}

// 场景：合法 none 与技术失败是不同结论（白盒 B1）。
// WHEN 模型返回合法 none → 视为「池内无对应实体」进入新建，不记技术失败；
// WHEN 返回不在候选集合内的 ID / 不可解析响应 / 服务失败 / 超时 → 判技术
// 失败，不降级 none、不新建、不半写，重试耗尽后保持「未归一」并记录原因，
// 不阻断、不进业务等待队列。
func TestEntityResolver_TechnicalFailureDistinctFromNone(t *testing.T) {
	t.Run("legal none creates, not a tech failure", func(t *testing.T) {
		chat := &fakeChat{responses: []string{"none"}}
		embed := &fakeEmbed{byText: map[string][]float64{}}
		rs, repo := newTestResolver(t, chat.chat, embed.embed)
		seedEntity(t, repo, "EDDDD0001", "OpenAI", "OpenAI", []float64{1, 0, 0})

		tagID := uint(2001)
		res := rs.ResolveMention(context.Background(), MentionRequest{
			MentionText: "特朗普", SourceTagID: &tagID,
		})
		require.Equal(t, StatusCreated, res.Status)
		assert.Equal(t, "legal none", res.Reason)
		assert.Equal(t, 1, chat.calls, "候选非空时走一次二轮")
		// 二轮请求经统一路由入口：capability 与 operation 契约。
		assert.Equal(t, airouter.CapabilityTopicTagging, chat.lastReq.Capability)
		assert.Equal(t, secondRoundOperation, chat.lastReq.Operation)
	})

	t.Run("illegal id exhausts attempts without creating", func(t *testing.T) {
		chat := &fakeChat{responses: []string{"E_NOT_IN_POOL", "E_ALSO_BAD"}}
		embed := &fakeEmbed{byText: map[string][]float64{}}
		rs, repo := newTestResolver(t, chat.chat, embed.embed)
		seeded := seedEntity(t, repo, "EDDDD0011", "OpenAI", "OpenAI", []float64{1, 0, 0})

		tagID := uint(2002)
		res := rs.ResolveMention(context.Background(), MentionRequest{
			MentionText: "OpenAI", SourceTagID: &tagID, // registered-name hit → candidate exists
		})
		require.Equal(t, StatusUnresolved, res.Status, "非法 ID = 技术失败，不降级 none")
		assert.Equal(t, 2, chat.calls, "最多 2 次尝试")
		assert.Equal(t, 2, res.Attempts)

		// 不新建、不半写：池里只有预置实体，无提及行落在任何新实体上。
		var mentionRows int64
		require.NoError(t, repo.DB().Model(&tagmodels.EntityMention{}).
			Where("mention_key = ?", "openai").Count(&mentionRows).Error)
		assert.EqualValues(t, 1, mentionRows, "只有 seed 的证据行，技术失败不落任何行")
		var entities int64
		require.NoError(t, repo.DB().Model(&tagmodels.EntityRegistry{}).Count(&entities).Error)
		assert.EqualValues(t, 1, entities)
		assert.NotContains(t, res.Reason, "none")
		_ = seeded
	})

	t.Run("unparsable response is a tech failure", func(t *testing.T) {
		// 解析分流（候选集非法产物不进系统）；不需要 DB。
		legal := map[string]bool{"EOK1": true}
		_, _, err := ParseSecondRoundAnswer("我认为是第一个候选", legal)
		require.Error(t, err, "不可解析 = 技术失败")
		_, _, err = ParseSecondRoundAnswer("E_NOPE", legal)
		require.Error(t, err, "不在候选集合 = 技术失败")
		key, isNone, err := ParseSecondRoundAnswer("EOK1", legal)
		require.NoError(t, err)
		assert.Equal(t, "EOK1", key)
		assert.False(t, isNone)
		_, isNone, err = ParseSecondRoundAnswer("none", legal)
		require.NoError(t, err)
		assert.True(t, isNone)
	})

	t.Run("service failure and timeout stay unresolved", func(t *testing.T) {
		chat := &fakeChat{errs: []error{errors.New("provider down"), errors.New("provider down")}}
		embed := &fakeEmbed{byText: map[string][]float64{}}
		rs, repo := newTestResolver(t, chat.chat, embed.embed)
		seedEntity(t, repo, "EDDDD0021", "OpenAI", "OpenAI", []float64{1, 0, 0})
		tagID := uint(2003)
		res := rs.ResolveMention(context.Background(), MentionRequest{MentionText: "OpenAI", SourceTagID: &tagID})
		require.Equal(t, StatusUnresolved, res.Status)
		assert.Contains(t, res.Reason, "exhausted")

		// 超时变体：单次调用延迟超过 CallTimeout → ctx 截断 → 技术失败。
		cfg := fastConfig()
		cfg.CallTimeout = 60 * time.Millisecond
		slow := &fakeChat{responses: []string{"none"}, delay: 500 * time.Millisecond}
		rs2 := NewResolver(repo, slow.chat, embed.embed, cfg)
		tagID2 := uint(2004)
		res2 := rs2.ResolveMention(context.Background(), MentionRequest{MentionText: "OpenAI", SourceTagID: &tagID2})
		require.Equal(t, StatusUnresolved, res2.Status, "超时 = 技术失败，不降级 none")
	})

	t.Run("empty or blank mention is a no-op not an error", func(t *testing.T) {
		chat := &fakeChat{responses: []string{"none"}}
		embed := &fakeEmbed{byText: map[string][]float64{}}
		rs, _ := newTestResolver(t, chat.chat, embed.embed)
		for _, in := range []string{"", "   ", "\t\n"} {
			res := rs.ResolveMention(context.Background(), MentionRequest{MentionText: in})
			require.Equal(t, StatusUnresolved, res.Status)
			assert.Contains(t, res.Reason, "empty or blank")
		}
		assert.Equal(t, 0, chat.calls)
		assert.Equal(t, 0, embed.calls, "空提及不烧 embedding")
	})
}

// 场景：同来源同提及重放幂等。
// WHEN 同一文章同一标签的同一提及被处理两次：两次返回同一实体 ID，实体层
// 不多出一个实体；第二轮模型调用不再发起（审计只有一次判定尝试）。
func TestEntityResolver_IdempotentReplay(t *testing.T) {
	chat := &fakeChat{responses: []string{"none"}}
	embed := &fakeEmbed{byText: map[string][]float64{}}
	rs, repo := newTestResolver(t, chat.chat, embed.embed)

	tagID := uint(3001)
	req := MentionRequest{MentionText: "特朗普", SourceTagID: &tagID, SubjectDesc: "美国第45任总统"}
	first := rs.ResolveMention(context.Background(), req)
	require.Equal(t, StatusCreated, first.Status)

	callsAfterFirst := chat.calls
	embedsAfterFirst := embed.calls

	second := rs.ResolveMention(context.Background(), req)
	require.Equal(t, StatusIdempotent, second.Status, "同来源同提及重放复用先前决策")
	assert.Equal(t, first.EntityID, second.EntityID)
	assert.Equal(t, 0, second.Attempts, "不再发起二轮调用")
	assert.Equal(t, callsAfterFirst, chat.calls, "模型零重放")
	assert.Equal(t, embedsAfterFirst, embed.calls, "embedding 零重放")

	var entities int64
	require.NoError(t, repo.DB().Model(&tagmodels.EntityRegistry{}).Count(&entities).Error)
	assert.EqualValues(t, 1, entities, "实体层不多出一个实体")

	// 跨来源不复用：另一来源标签的同提及要按当次上下文重新判定（D8）。
	// pickFirst 让二轮「选中池中已登记实体」→ reused 同一 ID。
	smartChat := &fakeChat{pickFirst: true}
	rsCross := NewResolver(repo, smartChat.chat, embed.embed, fastConfig())
	otherTag := uint(3002)
	third := rsCross.ResolveMention(context.Background(), MentionRequest{
		MentionText: "特朗普", SubjectDesc: "美国第45任总统", SourceTagID: &otherTag,
	})
	require.Equal(t, StatusReused, third.Status, "跨来源重判后按最新候选选中已登记实体")
	assert.Equal(t, first.EntityID, third.EntityID, "选中已有实体 → 同一统一 ID")
	assert.Equal(t, 1, smartChat.calls, "跨来源重放确实再次判定（不因叫法相同跳过判别）")
}
