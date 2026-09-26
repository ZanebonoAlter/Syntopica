package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	discmodels "syntopica-backend/internal/discovery/models"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// setupPreferenceFixture 构造偏好重算所需的最小数据集：
//   - 1 个版块 boardA（semantic_labels）
//   - 3 个 active 标签（topic_tags）+ semantic 向量（topic_tag_embeddings）
//   - 标签→版块归属（topic_tag_board_labels，3 标签同属 boardA → 满足 minTags=3）
//   - 1 篇文章收藏行为（reading_behaviors，event_type=favorite）
//
// 返回插入的标签向量（用于期望值校验）。
func setupPreferenceFixture(t *testing.T, db *gorm.DB) (boardID uint, tagIDs []uint) {
	t.Helper()
	now := time.Now()

	board := models.SemanticLabel{Label: "AI 前沿", Slug: "ai-front", Status: "active", Embedding: strPtr(floatsToPgVector(padVec([]float64{1, 0, 0})))}
	require.NoError(t, db.Create(&board).Error)
	boardID = board.ID

	tags := []models.TopicTag{
		{Label: "芯片", Slug: "chip", Category: "keyword", Status: "active", IsCanonical: true},
		{Label: "大模型", Slug: "llm", Category: "keyword", Status: "active", IsCanonical: true},
		{Label: "算力", Slug: "compute", Category: "keyword", Status: "active", IsCanonical: true},
	}
	for i := range tags {
		require.NoError(t, db.Create(&tags[i]).Error)
		tagIDs = append(tagIDs, tags[i].ID)
		emb := tagmodels.TopicTagEmbedding{
			TopicTagID:    tags[i].ID,
			EmbeddingType: "semantic",
			EmbeddingVec:  floatsToPgVector(padVec([]float64{float64(i + 1), 1, 0})),
			Dimension:     testutil.TestEmbeddingDim,
			Model:         "test-embed",
			TextHash:      "hash-" + tags[i].Slug,
		}
		require.NoError(t, db.Create(&emb).Error)
		require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
			TopicTagID: tags[i].ID, SemanticBoardID: boardID, Score: 0.9,
		}).Error)
	}

	// 文章收藏了上述 3 标签（reading_behaviors / article_topic_tags 不强 FK 且查询不 JOIN articles，
	// 直接用固定 article_id 即可，无需创建真实文章）。
	artID := uint(1)
	for _, tid := range tagIDs {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: artID, TopicTagID: tid, Source: "llm"}).Error)
	}
	require.NoError(t, db.Create(&models.ReadingBehavior{
		ArticleID: artID, EventType: "favorite", ScrollDepth: 0, ReadingTime: 0, CreatedAt: now,
	}).Error)
	return
}

// TestPreferenceRecomputeProducesBoardVector：有行为+标签+向量 → 版块产出 behavior 向量。
func TestPreferenceRecomputeProducesBoardVector(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID, _ := setupPreferenceFixture(t, db)
	svc := NewPreferenceProfileService(db)

	summary, err := svc.RecomputeAll(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, summary.BoardsComputed, "应至少产出一个版块向量")

	var pv discmodels.PreferenceVector
	require.NoError(t, db.Where("board_id = ? AND source = ?", boardID, PreferenceSourceBehavior).First(&pv).Error)
	require.NotEqual(t, "", pv.EmbeddingVec, "embedding 应非空")
	require.Equal(t, "test-embed", pv.Model)
	require.Equal(t, testutil.TestEmbeddingDim, pv.Dimension)
	require.NotEmpty(t, pv.TagWeights, "tag_weights 应记录 top 标签")
}

// TestPreferenceRecomputeIdempotent：连续两次重算，向量一致且不产生重复行。
func TestPreferenceRecomputeIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID, _ := setupPreferenceFixture(t, db)
	svc := NewPreferenceProfileService(db)

	_, err := svc.RecomputeAll(context.Background())
	require.NoError(t, err)
	var first discmodels.PreferenceVector
	require.NoError(t, db.Where("board_id = ? AND source = ?", boardID, PreferenceSourceBehavior).First(&first).Error)

	_, err = svc.RecomputeAll(context.Background())
	require.NoError(t, err)

	var rows []discmodels.PreferenceVector
	require.NoError(t, db.Where("board_id = ? AND source = ?", boardID, PreferenceSourceBehavior).Find(&rows).Error)
	require.Len(t, rows, 1, "幂等：同 board+source 不应产生重复行")
	require.Equal(t, first.EmbeddingVec, rows[0].EmbeddingVec, "两次重算向量应一致")
}

// TestPreferenceRecomputePreservesSeed：行为重算 MUST NOT 覆盖 source=seed 行。
func TestPreferenceRecomputePreservesSeed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	boardID, _ := setupPreferenceFixture(t, db)

	// 预置一行 seed（模拟问答写入）。
	seed := discmodels.PreferenceVector{
		BoardID:      &boardID,
		Source:       PreferenceSourceSeed,
		EmbeddingVec: floatsToPgVector(padVec([]float64{9, 9, 9})),
		Dimension:    testutil.TestEmbeddingDim,
		Model:        "seed-embed",
	}
	require.NoError(t, db.Create(&seed).Error)

	svc := NewPreferenceProfileService(db)
	_, err := svc.RecomputeAll(context.Background())
	require.NoError(t, err)

	var got discmodels.PreferenceVector
	require.NoError(t, db.Where("board_id = ? AND source = ?", boardID, PreferenceSourceSeed).First(&got).Error)
	require.Equal(t, seed.EmbeddingVec, got.EmbeddingVec, "重算不得修改 seed 行")
	require.Equal(t, "seed-embed", got.Model)
}

// TestPreferenceWriteSeedAccumulates：同版块多次 WriteSeed 加权合并累积（D7/A）。
func TestPreferenceWriteSeedAccumulates(t *testing.T) {
	db := testutil.SetupTestDB(t)
	board := models.SemanticLabel{Label: "量子", Slug: "quantum", Status: "active",
		Embedding: strPtr(floatsToPgVector(padVec([]float64{0, 1, 0})))}
	require.NoError(t, db.Create(&board).Error)

	boardVecs := map[uint][]float64{board.ID: padVec([]float64{0, 1, 0})}
	svc := NewPreferenceProfileService(db)
	ctx := context.Background()

	// 第一次：incoming=[1,0,0]，与 board=[0,1,0] 余弦=0 < 0.5 阈值 → 落全局桶。
	require.NoError(t, svc.WriteSeed(ctx, padVec([]float64{1, 0, 0}), testutil.TestEmbeddingDim, "test-embed", boardVecs))
	var global1 discmodels.PreferenceVector
	require.NoError(t, db.Where("board_id IS NULL AND source = ?", PreferenceSourceSeed).First(&global1).Error)
	require.NotEqual(t, "", global1.EmbeddingVec)

	// 第二次：incoming=[1,0,0] 再写 → 加权合并（α=0.4），单行不新增。
	require.NoError(t, svc.WriteSeed(ctx, padVec([]float64{1, 0, 0}), testutil.TestEmbeddingDim, "test-embed", boardVecs))
	var count int64
	db.Model(&discmodels.PreferenceVector{}).Where("board_id IS NULL AND source = ?", PreferenceSourceSeed).Count(&count)
	require.EqualValues(t, 1, count, "同版块多次问答应合并为单行（保 UNIQUE）")
}

// padVec 把短向量零填充到 TestEmbeddingDim（pgvector 列拒绝短向量）。
func padVec(v []float64) []float64 {
	return testutil.PadVector(v, testutil.TestEmbeddingDim)
}

// strPtr 辅助：取 string 指针（SemanticLabel.Embedding 是 *string）。
func strPtr(s string) *string { return &s }
