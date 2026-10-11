package auxlabel

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
)

func setupAuxiliaryLabelTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	InvalidateAuxLabelCache() // 避免包级缓存跨测试残留（ResetTestData 清 db 但不清内存缓存）
	return db
}

type recordingAuxiliaryEmbedder struct {
	calls      []string
	modes      []AuxiliaryLabelEmbeddingMode
	vectors    map[string][]float64
	failInputs map[string]bool
}

func (e *recordingAuxiliaryEmbedder) embed(ctx context.Context, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	if e.failInputs != nil && e.failInputs[input] {
		return "", nil, fmt.Errorf("embedder forced failure for %q", input)
	}
	e.calls = append(e.calls, input)
	e.modes = append(e.modes, mode)
	if e.vectors != nil {
		if vec, ok := e.vectors[input]; ok {
			padded := testutil.PadVector(vec, testutil.TestEmbeddingDim)
			return core.FloatsToPgVector(padded), padded, nil
		}
	}
	padded := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
	return core.FloatsToPgVector(padded), padded, nil
}

func TestAuxiliaryLabelServiceL1SlugAndAliasExactMatch(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existing := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active", Aliases: []string{"Open AI"}}
	require.NoError(t, db.Create(&existing).Error)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "Open AI", "")

	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID)
	require.Empty(t, embedder.calls)
}

func TestAuxiliaryLabelServiceL1NormalizeKeyMatchesTextVariant(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existing := models.SemanticLabel{Label: "SK海力士", Slug: "sk海力士", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&existing).Error)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	// "SK 海力士" slug differs from "SK海力士" (Slugify keeps a single space),
	// and there is no alias — only the whitespace-stripped normalize key matches.
	label, err := service.ResolveAuxiliaryLabel(context.Background(), "SK 海力士", "")

	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID, "L1 normalize-key match should reuse existing text-variant label, not create new")
	require.Empty(t, embedder.calls, "L1 match must short-circuit before any embedding call")
}

func TestAuxiliaryLabelServiceL1NormalizeKeyNoMatchCreatesNew(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existing := models.SemanticLabel{Label: "SK海力士", Slug: "sk海力士", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&existing).Error)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	// "量子计算" normalize key differs from "SK海力士" → no L1 hit → create new.
	label, err := service.ResolveAuxiliaryLabel(context.Background(), "量子计算", "")

	require.NoError(t, err)
	require.NotEqual(t, existing.ID, label.ID, "distinct normalize key should create a new label")
	require.NotZero(t, label.ID)
	require.Equal(t, "量子计算", label.Label)
}

func TestAuxiliaryLabelServiceExcludesDisabledLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	disabled := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "disabled", Aliases: []string{"Open AI"}}
	require.NoError(t, db.Create(&disabled).Error)
	disabledVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	disabledCandidate := models.SemanticLabel{Label: "OpenAI Candidate", Slug: "openai-candidate", LabelType: "auxiliary", Status: "disabled", Embedding: &disabledVec}
	require.NoError(t, db.Create(&disabledCandidate).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"OpenAI": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "OpenAI", "")

	require.NoError(t, err)
	require.NotEqual(t, disabled.ID, label.ID)
	require.Equal(t, "active", label.Status)
	require.Equal(t, "auxiliary", label.LabelType)
	require.Equal(t, []string{"OpenAI", "OpenAI"}, embedder.calls)
}

func TestAuxiliaryLabelServiceDisableAuxiliaryLabelNullsVectors(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	label := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active", Embedding: &vec, MergeEmbedding: &vec, Aliases: []string{"Open AI"}}
	require.NoError(t, db.Create(&label).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.DisableAuxiliaryLabel(context.Background(), label.ID))

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, label.ID).Error)
	require.Equal(t, "disabled", reloaded.Status)
	require.Nil(t, reloaded.Embedding)
	require.Nil(t, reloaded.MergeEmbedding)
	// 行本体与 aliases 保留：重新启用后可由 llm_extract 重算向量
	require.Equal(t, "OpenAI", reloaded.Label)
	require.Equal(t, []string{"Open AI"}, reloaded.Aliases)
}

func TestAuxiliaryLabelServiceDisableAuxiliaryLabelMarksOnlyAuxiliaryLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	auxiliary := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&auxiliary).Error)
	board := models.SemanticLabel{Label: "AI Board", Slug: "ai-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.DisableAuxiliaryLabel(context.Background(), auxiliary.ID))
	require.Error(t, service.DisableAuxiliaryLabel(context.Background(), board.ID))

	var reloadedAuxiliary models.SemanticLabel
	require.NoError(t, db.First(&reloadedAuxiliary, auxiliary.ID).Error)
	require.Equal(t, "disabled", reloadedAuxiliary.Status)

	var reloadedBoard models.SemanticLabel
	require.NoError(t, db.First(&reloadedBoard, board.ID).Error)
	require.Equal(t, "active", reloadedBoard.Status)
}

func TestAuxiliaryLabelServiceMergeAuxiliaryLabelAliasMigratesLinksAndPreservesBoardLabels(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tagA := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	tagB := models.TopicTag{Label: "GPT-5 模型评测", Slug: "gpt-5-review", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tagA).Error)
	require.NoError(t, db.Create(&tagB).Error)
	target := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active", Aliases: []string{"Open AI"}, RefCount: 1}
	require.NoError(t, db.Create(&target).Error)
	source := models.SemanticLabel{Label: "Open AI", Slug: "open-ai", LabelType: "auxiliary", Status: "active", Aliases: []string{"openai", "ChatGPT"}, RefCount: 2}
	require.NoError(t, db.Create(&source).Error)
	board := models.SemanticLabel{Label: "AI Board", Slug: "ai-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagA.ID, SemanticLabelID: target.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagA.ID, SemanticLabelID: source.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tagB.ID, SemanticLabelID: source.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tagA.ID, SemanticBoardID: board.ID, Score: 0.8, MatchReason: "existing"}).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.MergeAuxiliaryLabelAlias(context.Background(), source.ID, target.ID))

	var targetLinks int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", target.ID).Count(&targetLinks).Error)
	require.Equal(t, int64(2), targetLinks)
	var sourceLinks int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", source.ID).Count(&sourceLinks).Error)
	require.Zero(t, sourceLinks)

	var reloadedTarget models.SemanticLabel
	require.NoError(t, db.First(&reloadedTarget, target.ID).Error)
	require.Equal(t, 2, reloadedTarget.RefCount)
	require.Contains(t, reloadedTarget.Aliases, "ChatGPT")
	require.Len(t, reloadedTarget.Aliases, 2)

	var reloadedSource models.SemanticLabel
	require.NoError(t, db.First(&reloadedSource, source.ID).Error)
	require.Equal(t, "disabled", reloadedSource.Status)
	require.Zero(t, reloadedSource.RefCount)

	var boardLabelCount int64
	require.NoError(t, db.Model(&tagmodels.TopicTagBoardLabel{}).Count(&boardLabelCount).Error)
	require.Equal(t, int64(1), boardLabelCount)
}

func TestAuxiliaryLabelServiceRemoveBoardCompositionDeletesOnlyRequestedRow(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	board := models.SemanticLabel{Label: "AI Board", Slug: "ai-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	otherBoard := models.SemanticLabel{Label: "Hardware Board", Slug: "hardware-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&otherBoard).Error)
	auxiliary := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&auxiliary).Error)
	otherAuxiliary := models.SemanticLabel{Label: "GPU", Slug: "gpu", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&otherAuxiliary).Error)
	tag := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: otherAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: otherBoard.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID, Score: 0.8, MatchReason: "existing"}).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.RemoveBoardComposition(context.Background(), board.ID, auxiliary.ID))

	var requested int64
	require.NoError(t, db.Model(&tagmodels.BoardComposition{}).Where("board_id = ? AND auxiliary_label_id = ?", board.ID, auxiliary.ID).Count(&requested).Error)
	require.Zero(t, requested)
	var remaining int64
	require.NoError(t, db.Model(&tagmodels.BoardComposition{}).Count(&remaining).Error)
	require.Equal(t, int64(2), remaining)
	var boardLabelCount int64
	require.NoError(t, db.Model(&tagmodels.TopicTagBoardLabel{}).Count(&boardLabelCount).Error)
	require.Equal(t, int64(1), boardLabelCount)
}

func TestAuxiliaryLabelServiceL2MergeKeepsHigherRefCountAndAppendsAlias(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existingVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	lowRef := models.SemanticLabel{Label: "GPT-5", Slug: "gpt-5", LabelType: "auxiliary", Status: "active", RefCount: 1, MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&lowRef).Error)
	highRef := models.SemanticLabel{Label: "GPT-5 High Ref", Slug: "gpt-5-high-ref", LabelType: "auxiliary", Status: "active", RefCount: 9, MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&highRef).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"GPT 5": {0.999, 0.001, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "GPT 5", "")

	require.NoError(t, err)
	require.Equal(t, highRef.ID, label.ID)
	require.Contains(t, label.Aliases, "GPT 5")

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, highRef.ID).Error)
	require.Contains(t, reloaded.Aliases, "GPT 5")
}

func TestAuxiliaryLabelServiceL2ThresholdConfigurable(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	// Override the seeded default (0.95, set by migration 20260717_0001) to 0.80.
	setv := models.AISettings{Key: "auxiliary_label_dedupe_sim", Value: "0.80"}
	require.NoError(t, db.Where("key = ?", "auxiliary_label_dedupe_sim").Assign(setv).FirstOrCreate(&setv).Error)

	existingVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	existing := models.SemanticLabel{Label: "Prompt工程", Slug: "prompt工程", LabelType: "auxiliary", Status: "active", MergeEmbedding: &existingVec}
	require.NoError(t, db.Create(&existing).Error)
	// Candidate whose merge embedding has cosine ~0.85 to existing.
	// At the default 0.95 threshold this would NOT merge; at the configured 0.80 it should.
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"提示词工程": {0.85, 0.53, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "提示词工程", "")

	require.NoError(t, err)
	require.Equal(t, existing.ID, label.ID, "with configured threshold 0.80, cosine ~0.85 should merge into existing as alias")
	require.Contains(t, label.Aliases, "提示词工程")
}

func TestAuxiliaryLabelServiceL3CreatesAuxiliaryLabelWithEmbedding(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"多模态模型": {0, 1, 0}, "多模态模型: AI技术术语": {0, 0.9, 0.1}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	label, err := service.ResolveAuxiliaryLabel(context.Background(), "多模态模型", "AI技术术语")

	require.NoError(t, err)
	require.NotZero(t, label.ID)
	require.Equal(t, "多模态模型", label.Label)
	require.Equal(t, "auxiliary", label.LabelType)
	require.Equal(t, "llm_extract", label.Source)
	require.Equal(t, "active", label.Status)
	require.Equal(t, "AI技术术语", label.Description)
	require.NotNil(t, label.Embedding)
	require.NotNil(t, label.MergeEmbedding)
	require.NotEqual(t, *label.Embedding, *label.MergeEmbedding)
}

func TestAuxiliaryLabelServiceAttachAuxiliaryLabelsIncrementsRefCountOnce(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "OpenAI 发布 GPT-5", Slug: "openai-gpt-5", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	existing := models.SemanticLabel{Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&existing).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	labels := []core.AuxiliaryLabel{{Label: "OpenAI", Description: "人工智能公司"}, {Label: "GPT-5", Description: "大语言模型"}, {Label: "模型发布", Description: "新产品发布"}}
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))
	require.NoError(t, service.AttachAuxiliaryLabels(context.Background(), tag.ID, labels))

	var count int64
	require.NoError(t, db.Model(&tagmodels.TopicTagSemanticLabel{}).Where("topic_tag_id = ? AND semantic_label_id = ?", tag.ID, existing.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	require.Equal(t, 1, reloaded.RefCount)
}

// --- fix-alias-save-vector-wipe：alias 写路径向量保持 + 死向量回填 ---

// requireSameVector 断言两个 pgvector 字符串是同一个向量（按元素 float32 比对）：
// pgvector 列存 float4，回读文本是它的最短表示（写入 "1.000000" 回读 "1"），
// 字符串逐字节比较会被格式归一化误伤，语义上的「向量不变」应按值比较。
func requireSameVector(t *testing.T, expected, actual string, msgAndArgs ...interface{}) {
	t.Helper()
	expVec, err := ParsePgVector(expected)
	require.NoError(t, err, msgAndArgs...)
	actVec, err := ParsePgVector(actual)
	require.NoError(t, err, msgAndArgs...)
	require.Len(t, actVec, len(expVec), msgAndArgs...)
	for i := range expVec {
		require.Equal(t, float32(expVec[i]), float32(actVec[i]), "vector element %d differs: %v", i, msgAndArgs)
	}
}

// 主链路步1+步3：建行（带双向量）→ 三个别名变体串行 L2 归并 → 向量逐字节原样、
// aliases 追加、终态 active ⇒ 双向量非 NULL（归并路径不产生死向量行）。
func TestAuxiliaryLabelServiceAddAliasPreservesVectors(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	storageVec := core.FloatsToPgVector(testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim))
	mergeVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	existing := models.SemanticLabel{
		Label: "高市早苗", Slug: "takaichi-sanae", LabelType: "auxiliary", Status: "active",
		Embedding: &storageVec, MergeEmbedding: &mergeVec, RefCount: 47,
	}
	require.NoError(t, db.Create(&existing).Error)

	// 三个别名变体的 merge 向量与既有 merge 向量 cosine ~0.999 ≥ 0.95 → 依次 L2 归并
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{
		"高市早苗政府":   {0.999, 0.001, 0},
		"日本首相高市早苗": {0.998, 0.002, 0},
		"高市早苗内阁":   {0.997, 0.003, 0},
	}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	for _, alias := range []string{"高市早苗政府", "日本首相高市早苗", "高市早苗内阁"} {
		label, err := service.ResolveAuxiliaryLabel(context.Background(), alias, "")
		require.NoError(t, err)
		require.Equal(t, existing.ID, label.ID, "alias %q should L2-merge into existing label", alias)
		require.Contains(t, label.Aliases, alias)
	}

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	// 核心断言：归并后向量逐字节原样保留（历史 bug：缓存结构体全量 Save 把向量写 NULL）
	require.NotNil(t, reloaded.Embedding)
	require.NotNil(t, reloaded.MergeEmbedding)
	requireSameVector(t, storageVec, *reloaded.Embedding)
	requireSameVector(t, mergeVec, *reloaded.MergeEmbedding)
	// active ⇒ 向量非 NULL 不变量 + aliases 累积
	require.Equal(t, "active", reloaded.Status)
	require.Len(t, reloaded.Aliases, 3)
}

// 主链路步2：手动 alias 合并后目标标签向量保持不变；source 侧按既有语义 disabled+NULL。
func TestAuxiliaryLabelServiceMergeAuxiliaryAliasPreservesTargetVectors(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	targetVec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	targetMerge := core.FloatsToPgVector(testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim))
	target := models.SemanticLabel{
		Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active",
		Embedding: &targetVec, MergeEmbedding: &targetMerge, Aliases: []string{"Open AI"},
	}
	require.NoError(t, db.Create(&target).Error)
	source := models.SemanticLabel{Label: "ChatGPT", Slug: "chatgpt", LabelType: "auxiliary", Status: "active", Aliases: []string{"GPT"}}
	require.NoError(t, db.Create(&source).Error)
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)

	require.NoError(t, service.MergeAuxiliaryLabelAlias(context.Background(), source.ID, target.ID))

	var reloadedTarget models.SemanticLabel
	require.NoError(t, db.First(&reloadedTarget, target.ID).Error)
	require.NotNil(t, reloadedTarget.Embedding)
	requireSameVector(t, targetVec, *reloadedTarget.Embedding, "merge-alias must not touch target embedding")
	require.NotNil(t, reloadedTarget.MergeEmbedding)
	requireSameVector(t, targetMerge, *reloadedTarget.MergeEmbedding, "merge-alias must not touch target merge embedding")
	require.Contains(t, reloadedTarget.Aliases, "ChatGPT")
	require.Contains(t, reloadedTarget.Aliases, "GPT")

	var reloadedSource models.SemanticLabel
	require.NoError(t, db.First(&reloadedSource, source.ID).Error)
	require.Equal(t, "disabled", reloadedSource.Status)
	require.Nil(t, reloadedSource.Embedding)
	require.Nil(t, reloadedSource.MergeEmbedding)
}

// 主链路步4：回填补齐死向量行——L3 同款生成规则（merge←label、storage←"label: description"）。
func TestAuxiliaryLabelServiceBackfillMissingEmbeddingsFillsNulls(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	dead := models.SemanticLabel{Label: "高市早苗", Slug: "takaichi", LabelType: "auxiliary", Status: "active", Description: "日本首相"}
	require.NoError(t, db.Create(&dead).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{
		"高市早苗":       {0, 1, 0}, // merge ← label
		"高市早苗: 日本首相": {1, 0, 0}, // storage ← "label: description"
	}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	summary, err := service.BackfillMissingEmbeddings(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, summary.Scanned)
	require.Equal(t, 1, summary.Fixed)
	require.Zero(t, summary.Skipped)
	require.Equal(t, []string{"高市早苗", "高市早苗: 日本首相"}, embedder.calls)
	require.Equal(t, []AuxiliaryLabelEmbeddingMode{AuxiliaryLabelEmbeddingModeMerge, AuxiliaryLabelEmbeddingModeStorage}, embedder.modes)

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, dead.ID).Error)
	require.NotNil(t, reloaded.Embedding)
	require.NotNil(t, reloaded.MergeEmbedding)
	expectedMerge := core.FloatsToPgVector(testutil.PadVector([]float64{0, 1, 0}, testutil.TestEmbeddingDim))
	expectedStorage := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	requireSameVector(t, expectedMerge, *reloaded.MergeEmbedding)
	requireSameVector(t, expectedStorage, *reloaded.Embedding)
}

// 主链路步5：只补 NULL 列——已有列值不变且不为该列发起生成。
func TestAuxiliaryLabelServiceBackfillSkipsExistingColumns(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	existingStorage := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	row := models.SemanticLabel{
		Label: "日本央行", Slug: "boj", LabelType: "auxiliary", Status: "active",
		Embedding: &existingStorage, Description: "中央银行",
	}
	require.NoError(t, db.Create(&row).Error)
	embedder := &recordingAuxiliaryEmbedder{vectors: map[string][]float64{"日本央行": {0, 1, 0}}}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	summary, err := service.BackfillMissingEmbeddings(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, summary.Fixed)
	// storage 列已在 → 只调 merge 模式一次，不为已有列发起生成
	require.Equal(t, []string{"日本央行"}, embedder.calls)
	require.Equal(t, []AuxiliaryLabelEmbeddingMode{AuxiliaryLabelEmbeddingModeMerge}, embedder.modes)

	var reloaded models.SemanticLabel
	require.NoError(t, db.First(&reloaded, row.ID).Error)
	requireSameVector(t, existingStorage, *reloaded.Embedding, "existing storage embedding must stay unchanged")
	require.NotNil(t, reloaded.MergeEmbedding)
}

// 主链路步6 + 幂等变体：单行生成失败跳过不阻塞；重触发 pending 只含残余行。
func TestAuxiliaryLabelServiceBackfillRowFailureSkips(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	badRow := models.SemanticLabel{Label: "坏标签", Slug: "bad", LabelType: "auxiliary", Status: "active"}
	goodA := models.SemanticLabel{Label: "好标签A", Slug: "good-a", LabelType: "auxiliary", Status: "active"}
	goodB := models.SemanticLabel{Label: "好标签B", Slug: "good-b", LabelType: "auxiliary", Status: "active"}
	require.NoError(t, db.Create(&badRow).Error)
	require.NoError(t, db.Create(&goodA).Error)
	require.NoError(t, db.Create(&goodB).Error)
	embedder := &recordingAuxiliaryEmbedder{
		vectors:    map[string][]float64{"好标签A": {0, 1, 0}, "好标签B": {0, 0, 1}},
		failInputs: map[string]bool{"坏标签": true},
	}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	summary, err := service.BackfillMissingEmbeddings(context.Background())

	require.NoError(t, err, "row-level failure must not bubble up")
	require.Equal(t, 3, summary.Scanned)
	require.Equal(t, 2, summary.Fixed)
	require.Equal(t, 1, summary.Skipped)

	var reloadedBad models.SemanticLabel
	require.NoError(t, db.First(&reloadedBad, badRow.ID).Error)
	require.Nil(t, reloadedBad.Embedding, "failed row stays NULL for retry")
	require.Nil(t, reloadedBad.MergeEmbedding)

	// 幂等重触发：pending 只含失败行；二次回填（仍失败）无 error、残余不变
	pending, err := service.CountMissingEmbeddings(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), pending)

	summary2, err := service.BackfillMissingEmbeddings(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, summary2.Scanned)
	require.Equal(t, 0, summary2.Fixed)
	require.Equal(t, 1, summary2.Skipped)
}

// 主链路步7（service 半）：无死向量时回填为空操作——零行处理、零 LLM 调用。
func TestAuxiliaryLabelServiceBackfillNoopHealthy(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	vec := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	healthy := models.SemanticLabel{
		Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active",
		Embedding: &vec, MergeEmbedding: &vec,
	}
	require.NoError(t, db.Create(&healthy).Error)
	embedder := &recordingAuxiliaryEmbedder{}
	service := NewAuxiliaryLabelService(db, embedder.embed)

	pending, err := service.CountMissingEmbeddings(context.Background())
	require.NoError(t, err)
	require.Zero(t, pending)

	summary, err := service.BackfillMissingEmbeddings(context.Background())
	require.NoError(t, err)
	require.Zero(t, summary.Scanned)
	require.Zero(t, summary.Fixed)
	require.Empty(t, embedder.calls, "healthy DB must trigger zero LLM calls")
}
