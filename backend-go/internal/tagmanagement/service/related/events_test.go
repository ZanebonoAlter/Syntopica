package related

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// 用例来源：openspec/changes/enable-story-cross-day-continuation/test-cases.md
// 故事 S1（主链路步 1/2/4/5 + 变体 + 白盒附加）。SQL/向量路径一律走 testutil
// 临时 PG（标准/backend/testing.md：repository/向量语义禁 SQLite）。

func setupRelatedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t)
	// 空间解析读的是 ai_routes/ai_models/ai_vendors：先种一条可用的 embedding 路由。
	vendor := models.AIVendor{Name: "stub-vendor", ProviderType: "openai_compatible", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	model := models.AIModel{VendorID: vendor.ID, Name: "stub-model", ModelKind: "embedding", Enabled: true, Status: "ok"}
	require.NoError(t, db.Create(&model).Error)
	route := models.AIRoute{Name: "default", Capability: "embedding", Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: model.ID, Priority: 1, Enabled: true}).Error)
	return db
}

type seedTag struct {
	label       string
	description string
	category    string
	status      string
	createdAt   time.Time
	metadata    models.MetadataMap
}

func createTag(t *testing.T, db *gorm.DB, seed seedTag) *models.TopicTag {
	t.Helper()
	if seed.category == "" {
		seed.category = models.TagCategoryEvent
	}
	if seed.status == "" {
		seed.status = "active"
	}
	if seed.createdAt.IsZero() {
		seed.createdAt = time.Now().UTC()
	}
	tag := &models.TopicTag{
		Label:       seed.label,
		Description: seed.description,
		Slug:        fmt.Sprintf("related-test-%d-%d", time.Now().UnixNano(), len(seed.label)),
		Category:    seed.category,
		Status:      seed.status,
		Metadata:    seed.metadata,
		IsCanonical: true,
		CreatedAt:   seed.createdAt,
		UpdatedAt:   seed.createdAt,
	}
	require.NoError(t, db.Create(tag).Error)
	return tag
}

func createBoard(t *testing.T, db *gorm.DB, label string) *models.SemanticLabel {
	t.Helper()
	board := &models.SemanticLabel{
		Label:     label,
		Slug:      "related-board-" + fmt.Sprint(time.Now().UnixNano()),
		LabelType: "board",
		Status:    "active",
	}
	require.NoError(t, db.Create(board).Error)
	return board
}

func mountTag(t *testing.T, db *gorm.DB, boardID, tagID uint, directionMismatch bool) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID:        tagID,
		SemanticBoardID:   boardID,
		Score:             0.9,
		MatchReason:       "direct_hit",
		DirectionMismatch: directionMismatch,
	}).Error)
}

func linkAux(t *testing.T, db *gorm.DB, tagID uint, labels ...string) {
	t.Helper()
	for _, label := range labels {
		aux := &models.SemanticLabel{
			Label:     label,
			Slug:      "related-aux-" + fmt.Sprint(time.Now().UnixNano()) + "-" + fmt.Sprint(len(labels)),
			LabelType: "auxiliary",
			Status:    "active",
		}
		require.NoError(t, db.Create(aux).Error)
		require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagID, SemanticLabelID: aux.ID}).Error)
	}
}

// putVector writes a cache row directly (bypassing the refresher) so the query
// tests can control exact vectors and text hashes.
func putVector(t *testing.T, db *gorm.DB, tag *models.TopicTag, space Space, aux []string, vector []float64) {
	t.Helper()
	text, err := BuildRepresentation(tag.Label, tag.Description, aux)
	require.NoError(t, err)
	require.NoError(t, db.Create(&tagmodels.RelatedEventEmbedding{
		TopicTagID:            tag.ID,
		RepresentationVersion: RepresentationVersion,
		SpaceKey:              space.Key(),
		TextHash:              HashText(text),
		Dimension:             len(vector),
		EmbeddingVec:          core.FloatsToPgVector(vector),
	}).Error)
}

func currentSpace(t *testing.T, db *gorm.DB) Space {
	t.Helper()
	space, err := ResolveSpace(context.Background(), db, DefaultGeneration)
	require.NoError(t, err)
	return space
}

// stubEmbedder maps text → vector. It never touches a provider.
type stubEmbedder struct {
	vectors  map[string][]float64
	err      error
	provider string
	model    string
	calls    [][]string
}

func (s *stubEmbedder) Embed(_ context.Context, req airouter.EmbeddingRequest, _ airouter.Capability) (*airouter.EmbeddingResult, error) {
	s.calls = append(s.calls, append([]string(nil), req.Input...))
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float64, len(req.Input))
	dims := 0
	for i, text := range req.Input {
		vec, ok := s.vectors[text]
		if !ok {
			vec = []float64{0.5, 0.5}
		}
		out[i] = vec
		dims = len(vec)
	}
	return &airouter.EmbeddingResult{
		Embeddings: out,
		Model:      s.model,
		Dimensions: dims,
		Provider:   s.provider,
	}, nil
}

// ── Scenario: 文本表示稳定且辅助标签可缺失 ────────────────────────────────────

func TestBuildRepresentationIsStableAndAuxOptional(t *testing.T) {
	base, err := BuildRepresentation("DeepSeek 推出 Creator Mode", "  DeepSeek\t发布   新功能  ", []string{"大模型", "开源"})
	require.NoError(t, err)

	reordered, err := BuildRepresentation(" DeepSeek 推出 Creator Mode ", "DeepSeek 发布 新功能", []string{"开源", "大模型", "开源"})
	require.NoError(t, err)
	require.Equal(t, base, reordered, "辅助标签顺序/重复与字段空白不得改变规范化文本")
	require.Equal(t, HashText(base), HashText(reordered))

	require.Equal(t, "名称\nDeepSeek 推出 Creator Mode\n\n描述\nDeepSeek 发布 新功能\n\n辅助标签\n大模型\n开源", base)

	withoutAux, err := BuildRepresentation("DeepSeek 推出 Creator Mode", "DeepSeek 发布 新功能", nil)
	require.NoError(t, err)
	require.NotContains(t, withoutAux, "辅助标签", "缺辅助标签时降级为名称+描述，不生成空块")

	onlyName, err := BuildRepresentation("仅有名称", "", []string{"  "})
	require.NoError(t, err)
	require.Equal(t, "名称\n仅有名称", onlyName)

	_, err = BuildRepresentation("  \u3000\t", "描述", []string{"大模型"})
	require.ErrorIs(t, err, ErrNotIndexable, "空白名称不可索引，不得生成虚构向量")

	_, err = BuildRepresentation("名称", strings.Repeat("长", maxInputRunes), nil)
	require.ErrorIs(t, err, ErrTextTooLong, "超预算必须拒绝而非静默截断")
}

// ── 主链路步 1：显式刷新（写入 / 复用 / 失效 / 失败不覆盖） ────────────────────

func TestRefreshIndexesReusesAndInvalidates(t *testing.T) {
	db := setupRelatedTestDB(t)
	board := createBoard(t, db, "AI 版块")
	tagA := createTag(t, db, seedTag{label: "DeepSeek 推出 Creator Mode", description: "发布新功能"})
	blank := createTag(t, db, seedTag{label: "   "})
	mountTag(t, db, board.ID, tagA.ID, false)
	mountTag(t, db, board.ID, blank.ID, false)

	space := currentSpace(t, db)
	embedder := &stubEmbedder{
		provider: space.Provider,
		model:    space.Model,
		vectors: map[string][]float64{
			"名称\nDeepSeek 推出 Creator Mode\n\n描述\n发布新功能": {1, 0},
		},
	}
	refresher := NewRefresherWithEmbedder(db, embedder)

	first, err := refresher.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, 2, first.TagCount)
	require.Equal(t, 1, first.Indexed)
	require.Equal(t, 0, first.Reused)
	require.Equal(t, 1, first.Failed, "空白名称记为不可索引，不生成向量")
	require.Equal(t, ReasonInvalidSource, first.Failures[0].Reason)

	var stored tagmodels.RelatedEventEmbedding
	require.NoError(t, db.Where("topic_tag_id = ?", tagA.ID).First(&stored).Error)
	require.Equal(t, space.Key(), stored.SpaceKey)
	require.Equal(t, RepresentationVersion, stored.RepresentationVersion)
	require.Equal(t, 2, stored.Dimension)

	// 重跑相同版本：复用有效缓存，不再调模型。
	second, err := refresher.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, 0, second.Indexed)
	require.Equal(t, 1, second.Reused)
	require.Len(t, embedder.calls, 1, "第二次刷新不得重新计算")

	// 文本变化（新增辅助标签）→ 旧向量失效，重算并写入新签名。
	linkAux(t, db, tagA.ID, "大模型")
	updatedText := "名称\nDeepSeek 推出 Creator Mode\n\n描述\n发布新功能\n\n辅助标签\n大模型"
	embedder.vectors[updatedText] = []float64{0, 1}
	third, err := refresher.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, 1, third.Indexed)
	require.Len(t, embedder.calls, 2)

	require.NoError(t, db.Where("topic_tag_id = ?", tagA.ID).First(&stored).Error)
	require.Equal(t, HashText(updatedText), stored.TextHash)
	require.Equal(t, "[0,1]", stored.EmbeddingVec, "重算后的向量已落库")
}

func TestRefreshFailuresDoNotOverwriteValidCache(t *testing.T) {
	db := setupRelatedTestDB(t)
	board := createBoard(t, db, "AI 版块")
	tag := createTag(t, db, seedTag{label: "事件甲", description: "描述甲"})
	mountTag(t, db, board.ID, tag.ID, false)
	space := currentSpace(t, db)
	// 先写一条「文本过期」的旧行（hash 与当前文本不符）：刷新会尝试重算，
	// 失败时这条旧行不得被覆盖。
	putVector(t, db, tag, space, []string{"旧辅助标签"}, []float64{1, 0})

	var before tagmodels.RelatedEventEmbedding
	require.NoError(t, db.Where("topic_tag_id = ?", tag.ID).First(&before).Error)

	failing := NewRefresherWithEmbedder(db, &stubEmbedder{provider: space.Provider, model: space.Model, err: errors.New("provider down")})
	result, err := failing.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, 0, result.Indexed)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, ReasonEmbedFailed, result.Failures[0].Reason)

	var after tagmodels.RelatedEventEmbedding
	require.NoError(t, db.Where("topic_tag_id = ?", tag.ID).First(&after).Error)
	require.Equal(t, before.EmbeddingVec, after.EmbeddingVec, "失败项不得覆盖旧有效缓存")

	// 零向量 / 空向量 / 空间不符一律拒绝写库（校验分支的白盒变体）。
	zero := NewRefresherWithEmbedder(db, &stubEmbedder{
		provider: space.Provider, model: space.Model,
		vectors: map[string][]float64{"名称\n事件甲\n\n描述\n描述甲": {0, 0}},
	})
	zeroResult, err := zero.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, 0, zeroResult.Indexed)
	require.Equal(t, ReasonZeroNorm, zeroResult.Failures[0].Reason)

	empty := NewRefresherWithEmbedder(db, &stubEmbedder{
		provider: space.Provider, model: space.Model,
		vectors: map[string][]float64{"名称\n事件甲\n\n描述\n描述甲": {}},
	})
	emptyResult, err := empty.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, ReasonEmptyVector, emptyResult.Failures[0].Reason)

	fallback := NewRefresherWithEmbedder(db, &stubEmbedder{
		provider: "other-vendor", model: "other-model",
		vectors: map[string][]float64{"名称\n事件甲\n\n描述\n描述甲": {1, 0}},
	})
	fallbackResult, err := fallback.RefreshBoard(context.Background(), board.ID, DefaultGeneration)
	require.NoError(t, err)
	require.Equal(t, 0, fallbackResult.Indexed)
	require.Equal(t, ReasonSpaceMismatch, fallbackResult.Failures[0].Reason,
		"后备供应商换了模型时必须拒绝写入，避免同维不同空间混算")

	require.NoError(t, db.Where("topic_tag_id = ?", tag.ID).First(&after).Error)
	require.Equal(t, before.EmbeddingVec, after.EmbeddingVec)
}

// ── 主链路步 2：同版块窗口、阈值、稳定排序、覆盖状态 ──────────────────────────

func TestRelatedEventsWindowThresholdAndOrdering(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	otherBoard := createBoard(t, db, "另一个版块")
	space := currentSpace(t, db)

	source := createTag(t, db, seedTag{label: "源事件", description: "源描述", createdAt: now.Add(-10 * 24 * time.Hour)})
	mountTag(t, db, board.ID, source.ID, false)
	putVector(t, db, source, space, nil, []float64{1, 0})

	// 同分数：id 升序决定展示顺序（先建的两个标签 id 更小）。
	sameA := createTag(t, db, seedTag{label: "同分甲", createdAt: now.Add(-2 * 24 * time.Hour)})
	sameB := createTag(t, db, seedTag{label: "同分乙", createdAt: now.Add(-2 * 24 * time.Hour)})
	// 恰好达到默认阈值 0.86。
	exact := createTag(t, db, seedTag{label: "恰好阈值", createdAt: now.Add(-1 * 24 * time.Hour)})
	// 窗口两端：恰好在边界上算，边界前 1 秒不算。
	boundary := createTag(t, db, seedTag{label: "窗口边界", createdAt: now.Add(-WindowDays * 24 * time.Hour)})
	outside := createTag(t, db, seedTag{label: "窗口外", createdAt: now.Add(-WindowDays*24*time.Hour - time.Second)})
	// 跨版块 / 自身 / mismatch / 非 event / 已合并 均不得返回。
	crossBoard := createTag(t, db, seedTag{label: "跨版块", createdAt: now.Add(-1 * 24 * time.Hour)})
	mismatch := createTag(t, db, seedTag{label: "方向不符", createdAt: now.Add(-1 * 24 * time.Hour)})
	personTag := createTag(t, db, seedTag{label: "人物标签", category: models.TagCategoryPerson, createdAt: now.Add(-1 * 24 * time.Hour)})
	merged := createTag(t, db, seedTag{label: "已合并标签", status: "merged", createdAt: now.Add(-1 * 24 * time.Hour)})

	for _, tag := range []*models.TopicTag{sameA, sameB, exact, boundary, outside} {
		mountTag(t, db, board.ID, tag.ID, false)
	}
	mountTag(t, db, otherBoard.ID, crossBoard.ID, false)
	mountTag(t, db, board.ID, mismatch.ID, true)
	mountTag(t, db, board.ID, personTag.ID, false)
	mountTag(t, db, board.ID, merged.ID, false)

	putVector(t, db, sameA, space, nil, []float64{0.9, 0.4358898943540674})
	putVector(t, db, sameB, space, nil, []float64{0.9, 0.4358898943540674})
	putVector(t, db, exact, space, nil, []float64{0.86, 0.5102941176470588})
	putVector(t, db, boundary, space, nil, []float64{0.95, 0.3122498999199199})
	putVector(t, db, outside, space, nil, []float64{1, 0})
	putVector(t, db, crossBoard, space, nil, []float64{1, 0})
	putVector(t, db, mismatch, space, nil, []float64{1, 0})
	putVector(t, db, personTag, space, nil, []float64{1, 0})
	putVector(t, db, merged, space, nil, []float64{1, 0})

	query := NewQuery(db)
	query.now = func() time.Time { return now }
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusReady, result.Status)
	require.Equal(t, DefaultThreshold, result.Threshold)
	require.Equal(t, WindowDays, result.WindowDays)
	require.Equal(t, 4, result.CandidateCount, "候选=同版块有效 event 标签 ∩ 窗口（不含自身/跨版块/mismatch/非 event/已合并）")
	require.Equal(t, 4, result.IndexedCount)

	var ids []uint
	for _, item := range result.Items {
		ids = append(ids, item.TagID)
	}
	require.Equal(t, boundary.ID, ids[0], "最高分在前")
	require.Equal(t, sameA.ID, ids[1], "同分按 id 升序稳定排序")
	require.Equal(t, sameB.ID, ids[2])
	require.Equal(t, exact.ID, ids[3], "恰好达到阈值的候选必须包含")
	require.NotContains(t, ids, outside.ID, "窗口外一毫秒都不得返回")
	require.NotContains(t, ids, crossBoard.ID, "跨版块不返回")
	require.NotContains(t, ids, mismatch.ID, "仅 mismatch 入板的标签不返回")
	require.NotContains(t, ids, source.ID, "自身不返回")
}

func TestRelatedEventsTopTenWithStableOrder(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)
	source := createTag(t, db, seedTag{label: "源事件", createdAt: now.Add(-1 * time.Hour)})
	mountTag(t, db, board.ID, source.ID, false)
	putVector(t, db, source, space, nil, []float64{1, 0})

	var created []uint
	for i := 0; i < 12; i++ {
		tag := createTag(t, db, seedTag{label: fmt.Sprintf("同分事件 %02d", i), createdAt: now.Add(-2 * time.Hour)})
		mountTag(t, db, board.ID, tag.ID, false)
		putVector(t, db, tag, space, nil, []float64{1, 0})
		created = append(created, tag.ID)
	}

	query := NewQuery(db)
	query.now = func() time.Time { return now }
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Len(t, result.Items, MaxResults, "阈值过滤后只截断展示列表")
	require.Equal(t, 12, result.CandidateCount)
	require.Equal(t, created[0], result.Items[0].TagID)
	require.Equal(t, created[9], result.Items[9].TagID, "同分数按 id 升序稳定排序")
}

func TestRelatedEventsCoverageStatus(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)
	source := createTag(t, db, seedTag{label: "源事件", createdAt: now.Add(-1 * time.Hour)})
	mountTag(t, db, board.ID, source.ID, false)

	query := NewQuery(db)
	query.now = func() time.Time { return now }

	// 源未索引 + 无候选 → not_ready（缺索引不得伪装成空结果）。
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusNotReady, result.Status)
	require.Equal(t, ReasonSourceNotIndexed, result.Reason)
	require.Empty(t, result.Items)

	// 源就绪 + 无候选 → ready 空集。
	putVector(t, db, source, space, nil, []float64{1, 0})
	result, err = query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusReady, result.Status)
	require.Equal(t, 0, result.CandidateCount)
	require.Empty(t, result.Items)

	// 源就绪 + 部分候选缺向量 → partial（即使命中为空也如实上报覆盖）。
	indexed := createTag(t, db, seedTag{label: "已索引候选", createdAt: now.Add(-2 * time.Hour)})
	missing := createTag(t, db, seedTag{label: "未索引候选", createdAt: now.Add(-2 * time.Hour)})
	unrelated := createTag(t, db, seedTag{label: "未索引且不相关", createdAt: now.Add(-2 * time.Hour)})
	for _, tag := range []*models.TopicTag{indexed, missing, unrelated} {
		mountTag(t, db, board.ID, tag.ID, false)
	}
	putVector(t, db, indexed, space, nil, []float64{1, 0})
	result, err = query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusPartial, result.Status)
	require.Equal(t, 3, result.CandidateCount)
	require.Equal(t, 1, result.IndexedCount)
	require.Len(t, result.Items, 1)
	require.Equal(t, indexed.ID, result.Items[0].TagID)

	// 索引后补齐 → ready。
	putVector(t, db, missing, space, nil, []float64{0, 1})
	putVector(t, db, unrelated, space, nil, []float64{0, 1})
	result, err = query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusReady, result.Status)
	require.Equal(t, 3, result.IndexedCount)
}

// ── 主链路步 4：不传递、不改身份 ──────────────────────────────────────────────

func TestRelatedEventsDoNotPropagateOrMutateIdentity(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)

	source := createTag(t, db, seedTag{label: "事件 A", createdAt: now.Add(-1 * time.Hour)})
	middle := createTag(t, db, seedTag{label: "事件 B", createdAt: now.Add(-2 * time.Hour)})
	far := createTag(t, db, seedTag{label: "事件 C", createdAt: now.Add(-3 * time.Hour)})
	for _, tag := range []*models.TopicTag{source, middle, far} {
		mountTag(t, db, board.ID, tag.ID, false)
	}
	// A≈B(0.87)、B≈C(0.91)、A⟂C(0.57) → A 的结果不得因 B 展开出 C。
	putVector(t, db, source, space, nil, []float64{1, 0, 0})
	putVector(t, db, middle, space, nil, []float64{0.8660254037844387, 0.5, 0})
	putVector(t, db, far, space, nil, []float64{0.5735764363510462, 0.8191520442889918, 0})

	var beforeTags []models.TopicTag
	require.NoError(t, db.Order("id ASC").Find(&beforeTags).Error)
	var beforeLinks []tagmodels.TopicTagSemanticLabel
	require.NoError(t, db.Find(&beforeLinks).Error)

	query := NewQuery(db)
	query.now = func() time.Time { return now }
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, middle.ID, result.Items[0].TagID, "只返回直接相关，不传递聚类")

	// B 的查询能看到 C —— 说明 C 确实在库里达标，只是不因 A 被展开。
	result, err = query.RelatedEvents(context.Background(), board.ID, middle.ID)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)

	var afterTags []models.TopicTag
	require.NoError(t, db.Order("id ASC").Find(&afterTags).Error)
	require.Equal(t, beforeTags, afterTags, "查询不得改写标签（身份/别名/描述/状态）")
	var afterLinks []tagmodels.TopicTagSemanticLabel
	require.NoError(t, db.Find(&afterLinks).Error)
	require.Equal(t, beforeLinks, afterLinks, "查询不得写辅助标签关联")

	var relations int64
	require.NoError(t, db.Model(&models.TopicTagRelation{}).Count(&relations).Error)
	require.Zero(t, relations, "不写任何关系边")
}

// ── Scenario: 关联不依赖旧身份字段 ────────────────────────────────────────────

func TestRelatedEventsIgnoreLegacyIdentityFields(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)

	source := createTag(t, db, seedTag{
		label:     "DeepSeek 推出 Creator Mode",
		createdAt: now.Add(-1 * time.Hour),
		metadata:  models.MetadataMap{"event_fingerprint": map[string]any{"subject_key": "product:deepseekharness", "fingerprint_trusted": false}},
	})
	candidate := createTag(t, db, seedTag{
		label:     "DeepSeek 发布创作模式教程",
		createdAt: now.Add(-2 * time.Hour),
		// 不同 subject_key、无实体 ID、指纹不可信 —— 全部不得否决相关查询。
		metadata: models.MetadataMap{"event_fingerprint": map[string]any{"subject_key": "other:deepseek_harness", "fingerprint_trusted": false}},
	})
	for _, tag := range []*models.TopicTag{source, candidate} {
		mountTag(t, db, board.ID, tag.ID, false)
	}
	putVector(t, db, source, space, nil, []float64{1, 0})
	putVector(t, db, candidate, space, nil, []float64{0.97, 0.24})

	query := NewQuery(db)
	query.now = func() time.Time { return now }
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, candidate.ID, result.Items[0].TagID)
}

// ── Scenario: 模型空间或文本变更后旧向量失效 ──────────────────────────────────

func TestRelatedEventsInvalidateOnSpaceAndTextChange(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)
	source := createTag(t, db, seedTag{label: "源事件", createdAt: now.Add(-1 * time.Hour)})
	candidate := createTag(t, db, seedTag{label: "候选事件", createdAt: now.Add(-2 * time.Hour)})
	for _, tag := range []*models.TopicTag{source, candidate} {
		mountTag(t, db, board.ID, tag.ID, false)
	}
	putVector(t, db, source, space, nil, []float64{1, 0})
	putVector(t, db, candidate, space, nil, []float64{1, 0})

	query := NewQuery(db)
	query.now = func() time.Time { return now }
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusReady, result.Status)
	require.Len(t, result.Items, 1)

	// 文本变化但未刷新 → 该候选的旧向量不参与计算。
	require.NoError(t, db.Model(&models.TopicTag{}).Where("id = ?", candidate.ID).Update("description", "描述变了").Error)
	result, err = query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusPartial, result.Status, "文本过期的向量不参与，覆盖数如实下降")
	require.Empty(t, result.Items)

	// 显式换 generation（同维换模型）→ 旧空间行全部不参与。
	require.NoError(t, SetGeneration(db, "g2"))
	result, err = query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Equal(t, StatusNotReady, result.Status)
	require.Equal(t, ReasonSourceNotIndexed, result.Reason)
	require.Contains(t, result.SpaceKey, "gen=g2")
}

// ── 主链路步 3：来源文章（老文章仍可读，取 pub_date 最新一条） ──────────────────

func TestRelatedEventsSourceArticle(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)
	feed := &models.Feed{Title: "来源", URL: fmt.Sprintf("https://example.com/%d", time.Now().UnixNano())}
	require.NoError(t, db.Create(feed).Error)

	source := createTag(t, db, seedTag{label: "源事件", createdAt: now.Add(-1 * time.Hour)})
	older := createTag(t, db, seedTag{label: "有来源事件", createdAt: now.Add(-2 * time.Hour)})
	bare := createTag(t, db, seedTag{label: "无来源事件", createdAt: now.Add(-2 * time.Hour)})
	for _, tag := range []*models.TopicTag{source, older, bare} {
		mountTag(t, db, board.ID, tag.ID, false)
	}
	putVector(t, db, source, space, nil, []float64{1, 0})
	putVector(t, db, older, space, nil, []float64{1, 0})
	putVector(t, db, bare, space, nil, []float64{1, 0})

	pubOld := now.Add(-48 * time.Hour)
	pubNew := now.Add(-2 * time.Hour)
	archived := &models.Article{FeedID: feed.ID, Title: "较新的归档文章", PubDate: &pubNew, Archived: true}
	old := &models.Article{FeedID: feed.ID, Title: "较旧文章", PubDate: &pubOld}
	require.NoError(t, db.Create(archived).Error)
	require.NoError(t, db.Create(old).Error)
	for _, article := range []*models.Article{archived, old} {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: older.ID}).Error)
	}

	query := NewQuery(db)
	query.now = func() time.Time { return now }
	result, err := query.RelatedEvents(context.Background(), board.ID, source.ID)
	require.NoError(t, err)
	require.Len(t, result.Items, 2)

	byTag := map[uint]RelatedEvent{}
	for _, item := range result.Items {
		byTag[item.TagID] = item
	}
	withSource := byTag[older.ID]
	require.NotNil(t, withSource.SourceArticle)
	require.Equal(t, archived.ID, withSource.SourceArticle.ID, "归档文章仍可读，取 pub_date 最新一条")
	require.Nil(t, byTag[bare.ID].SourceArticle, "无来源如实为空，不伪造 ID")
}

// ── 4xx 语义 ──────────────────────────────────────────────────────────────────

func TestRelatedEventsRejectsUnknownBoardAndForeignTag(t *testing.T) {
	db := setupRelatedTestDB(t)
	board := createBoard(t, db, "AI 版块")
	otherBoard := createBoard(t, db, "别的版块")
	tag := createTag(t, db, seedTag{label: "只挂别的版块"})
	mountTag(t, db, otherBoard.ID, tag.ID, false)

	query := NewQuery(db)
	_, err := query.RelatedEvents(context.Background(), 999999, tag.ID)
	require.ErrorIs(t, err, ErrBoardNotFound)
	_, err = query.RelatedEvents(context.Background(), board.ID, tag.ID)
	require.ErrorIs(t, err, ErrTagNotInBoard)
}

// ── 故事 S3：历史只读试验（不写缓存/身份/关系） ───────────────────────────────

func TestExperimentIsReadOnly(t *testing.T) {
	db := setupRelatedTestDB(t)
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	board := createBoard(t, db, "AI 版块")
	space := currentSpace(t, db)

	near := createTag(t, db, seedTag{label: "事件甲", createdAt: now.Add(-1 * time.Hour)})
	far := createTag(t, db, seedTag{label: "事件乙", createdAt: now.Add(-2 * time.Hour)})
	blank := createTag(t, db, seedTag{label: "  ", createdAt: now.Add(-3 * time.Hour)})
	for _, tag := range []*models.TopicTag{near, far, blank} {
		mountTag(t, db, board.ID, tag.ID, false)
	}

	embedder := &stubEmbedder{
		provider: space.Provider,
		model:    space.Model,
		vectors: map[string][]float64{
			"名称\n事件甲": {1, 0},
			"名称\n事件乙": {0.9, 0.4358898943540674},
		},
	}
	experiment := NewExperimentWithEmbedder(db, embedder)
	experiment.now = func() time.Time { return now }
	result, err := experiment.Run(context.Background(), ExperimentOptions{
		BoardID: board.ID, WindowDays: 30, Threshold: 0.82, SampleLimit: 10, NearLimit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, 3, result.TagCount)
	require.Equal(t, 2, result.Embedded)
	require.Equal(t, 1, result.Failed, "空白名称计入失败，不当成不相关")
	require.Equal(t, ReasonInvalidSource, result.Failures[0].Reason)
	require.Equal(t, 1, result.PairCount)
	require.Equal(t, 1, result.AboveCount)
	require.Len(t, result.Direct, 1)
	require.InDelta(t, 0.9, result.Direct[0].Score, 1e-6)
	require.Empty(t, result.Near)
	require.Zero(t, result.CacheWrites)
	require.Equal(t, RepresentationVersion, result.RepresentationVersion)
	require.Equal(t, space.Model, result.Model)

	// 只读：缓存/标签/关系零写入。
	var cacheRows int64
	require.NoError(t, db.Model(&tagmodels.RelatedEventEmbedding{}).Count(&cacheRows).Error)
	require.Zero(t, cacheRows)
	var relations int64
	require.NoError(t, db.Model(&models.TopicTagRelation{}).Count(&relations).Error)
	require.Zero(t, relations)
	var tags []models.TopicTag
	require.NoError(t, db.Order("id ASC").Find(&tags).Error)
	require.Len(t, tags, 3)
	require.Equal(t, "事件甲", tags[0].Label)
}

// ── 阈值读取（缺项/坏值回退） ─────────────────────────────────────────────────

func TestLoadThresholdFallsBackAndHonoursStoredValue(t *testing.T) {
	db := setupRelatedTestDB(t)
	require.Equal(t, DefaultThreshold, LoadThreshold(db), "缺项回退默认 0.86")

	require.NoError(t, db.Create(&models.AISettings{Key: ThresholdKey, Value: "0.66"}).Error)
	require.Equal(t, 0.66, LoadThreshold(db))

	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", ThresholdKey).Update("value", "NaN").Error)
	require.Equal(t, DefaultThreshold, LoadThreshold(db), "非有限存量值告警并回退默认")

	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", ThresholdKey).Update("value", "1.5").Error)
	require.Equal(t, DefaultThreshold, LoadThreshold(db))
}
