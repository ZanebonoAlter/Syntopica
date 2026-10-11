package service

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/topicgraph/repository"
)

func TestDeduplicateTags_Empty(t *testing.T) {
	result := DeduplicateTags(context.Background(), nil, nil, nil)
	assert.Nil(t, result)
}

func TestDeduplicateTags_IdenticalArticleSets(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "Tag A", ArticleCount: 3},
		{ID: 2, Label: "Tag B", ArticleCount: 3},
		{ID: 3, Label: "Tag C", ArticleCount: 5},
	}
	articleSets := [][]uint{
		{10, 20, 30},
		{10, 20, 30},
		{40, 50},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, nil)
	assert.Len(t, result, 2)
	// Tag B has same set as Tag A but same article count and higher ID → Tag A survives.
	// Tag A (ID:1) vs Tag B (ID:2): same article set, same count, nil relevance → keep lower ID (Tag A)
	assert.Equal(t, uint(1), result[0].ID)
	assert.Equal(t, uint(3), result[1].ID)
}

// TestDeduplicateTags_SingleArticleOverlap（fix-digest-tag-leak 改造，故事 B-1）：
// 同一篇文章拆出的两个事件标签 article_count 平手时，与当前版块 identity
// cosine 更高者存活——digest 案例里贴版块的「柴油协议」标签（ID 更大但
// cos 0.80）顶掉串味的「OpenAI 营收」标签（ID 更小但 cos 0.60）。
func TestDeduplicateTags_SingleArticleOverlap(t *testing.T) {
	boardVec := []float64{1, 0, 0}
	cosVec := func(c float64) []float64 { return []float64{c, math.Sqrt(1 - c*c), 0} }
	relevance := &DedupRelevance{
		TagIdentityEmbeddings: map[uint][]float64{
			10: cosVec(0.60),
			20: cosVec(0.80),
		},
		BoardEmbedding: boardVec,
	}
	tags := []repository.TagInput{
		{ID: 10, Label: "Tag X", ArticleCount: 1},
		{ID: 20, Label: "Tag Y", ArticleCount: 1},
		{ID: 30, Label: "Tag Z", ArticleCount: 1},
	}
	articleSets := [][]uint{
		{100},
		{100},
		{200},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
	assert.Len(t, result, 2)
	ids := tagIDs(result)
	assert.Contains(t, ids, uint(20), "cos 0.80 survives despite the higher ID")
	assert.NotContains(t, ids, uint(10), "cos 0.60 loses the duel")
	assert.Contains(t, ids, uint(30), "different article unaffected")
}

// B-1 镜像：先到者（incumbent）更贴版块 → 不被后到的串味者顶替。
func TestDeduplicateTags_SingleArticleOverlap_CloserIncumbentSurvives(t *testing.T) {
	boardVec := []float64{1, 0, 0}
	cosVec := func(c float64) []float64 { return []float64{c, math.Sqrt(1 - c*c), 0} }
	relevance := &DedupRelevance{
		TagIdentityEmbeddings: map[uint][]float64{
			10: cosVec(0.80),
			20: cosVec(0.60),
		},
		BoardEmbedding: boardVec,
	}
	tags := []repository.TagInput{
		{ID: 10, Label: "Tag X", ArticleCount: 1},
		{ID: 20, Label: "Tag Y", ArticleCount: 1},
	}
	articleSets := [][]uint{{100}, {100}}

	result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(10), result[0].ID, "incumbent cos 0.80 keeps the seat")
}

// B-2 embedding 缺失回退 ID 顺序（现行为兜底）。
func TestDeduplicateTags_EmbeddingMissingFallsBackToID(t *testing.T) {
	boardVec := []float64{1, 0, 0}
	cosVec := func(c float64) []float64 { return []float64{c, math.Sqrt(1 - c*c), 0} }
	tags := []repository.TagInput{
		{ID: 10, Label: "Tag X", ArticleCount: 1},
		{ID: 20, Label: "Tag Y", ArticleCount: 1},
	}
	articleSets := [][]uint{{100}, {100}}

	t.Run("nil relevance keeps legacy ID order", func(t *testing.T) {
		result := DeduplicateTags(context.Background(), tags, articleSets, nil)
		assert.Len(t, result, 1)
		assert.Equal(t, uint(10), result[0].ID)
	})

	t.Run("challenger identity embedding missing", func(t *testing.T) {
		relevance := &DedupRelevance{
			TagIdentityEmbeddings: map[uint][]float64{10: cosVec(0.80)},
			BoardEmbedding:        boardVec,
		}
		result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
		assert.Len(t, result, 1)
		assert.Equal(t, uint(10), result[0].ID, "duel with a missing side falls back to ID")
	})

	t.Run("board embedding missing", func(t *testing.T) {
		relevance := &DedupRelevance{
			TagIdentityEmbeddings: map[uint][]float64{10: cosVec(0.80), 20: cosVec(0.10)},
		}
		result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
		assert.Len(t, result, 1)
		assert.Equal(t, uint(10), result[0].ID, "no board embedding → every duel falls back to ID")
	})

	t.Run("all-zero identity vector treated as missing", func(t *testing.T) {
		relevance := &DedupRelevance{
			TagIdentityEmbeddings: map[uint][]float64{10: cosVec(0.80), 20: {0, 0, 0}},
			BoardEmbedding:        boardVec,
		}
		result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
		assert.Len(t, result, 1)
		assert.Equal(t, uint(10), result[0].ID, "zero-norm vector is not a valid signal")
	})
}

// B-3 文章集合完全相同的标签去重也走三级 tiebreak。
func TestDeduplicateTags_IdenticalArticleSets_BoardRelevanceTiebreak(t *testing.T) {
	boardVec := []float64{1, 0, 0}
	cosVec := func(c float64) []float64 { return []float64{c, math.Sqrt(1 - c*c), 0} }
	relevance := &DedupRelevance{
		TagIdentityEmbeddings: map[uint][]float64{
			1: cosVec(0.70),
			2: cosVec(0.85),
		},
		BoardEmbedding: boardVec,
	}
	tags := []repository.TagInput{
		{ID: 1, Label: "Tag A", ArticleCount: 3},
		{ID: 2, Label: "Tag B", ArticleCount: 3},
	}
	articleSets := [][]uint{
		{10, 20, 30},
		{10, 20, 30},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(2), result[0].ID, "identical sets: closer-to-board tag survives")
}

// 白盒（分支表 #4）：count 平手 + 双 emb 齐 + cos 浮点精确相等 → ID 小者胜。
func TestDeduplicateTags_ExactEqualCosineFallsBackToID(t *testing.T) {
	boardVec := []float64{1, 0, 0}
	sameVec := []float64{0.7, 0.714142842854285, 0} // 两次经同一函数得到逐位相同的 cosine
	relevance := &DedupRelevance{
		TagIdentityEmbeddings: map[uint][]float64{10: sameVec, 20: sameVec},
		BoardEmbedding:        boardVec,
	}
	tags := []repository.TagInput{
		{ID: 10, Label: "Tag X", ArticleCount: 1},
		{ID: 20, Label: "Tag Y", ArticleCount: 1},
	}
	articleSets := [][]uint{{100}, {100}}

	result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(10), result[0].ID, "exact-equal cosine → ID tiebreak")
}

// 白盒（分支表 #1）：article_count 不等时第一级短路，cosine 不参与。
func TestDeduplicateTags_ArticleCountShortCircuitsCosine(t *testing.T) {
	boardVec := []float64{1, 0, 0}
	cosVec := func(c float64) []float64 { return []float64{c, math.Sqrt(1 - c*c), 0} }
	relevance := &DedupRelevance{
		TagIdentityEmbeddings: map[uint][]float64{
			1: cosVec(0.90),
			5: cosVec(0.10),
		},
		BoardEmbedding: boardVec,
	}
	tags := []repository.TagInput{
		{ID: 1, Label: "Small", ArticleCount: 2},
		{ID: 5, Label: "Big", ArticleCount: 5},
	}
	articleSets := [][]uint{
		{10, 20},
		{10, 20},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, relevance)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(5), result[0].ID, "higher article_count wins regardless of cosine")
}

// B-4（旧用例保留）：第一级 article_count 大者存活。
func TestDeduplicateTags_HigherArticleCountWins(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "Small", ArticleCount: 2},
		{ID: 5, Label: "Big", ArticleCount: 5},
	}
	articleSets := [][]uint{
		{10, 20},
		{10, 20},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(5), result[0].ID) // Higher article_count wins
}

func TestDeduplicateTags_MismatchedLength(t *testing.T) {
	tags := []repository.TagInput{{ID: 1, Label: "A"}}
	articleSets := [][]uint{} // mismatch

	result := DeduplicateTags(context.Background(), tags, articleSets, nil)
	assert.Len(t, result, 1) // Returns original
}

// B-5（旧用例保留）：无重复时原样返回——去重仅内存操作，不改原始数据。
func TestDeduplicateTags_NoDuplicates(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "A", ArticleCount: 2},
		{ID: 2, Label: "B", ArticleCount: 3},
		{ID: 3, Label: "C", ArticleCount: 1},
	}
	articleSets := [][]uint{
		{10, 20},
		{30, 40, 50},
		{60},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, nil)
	assert.Len(t, result, 3)
}

func TestDeduplicateTags_EmptyArticleSet(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "Empty", ArticleCount: 0},
		{ID: 2, Label: "Valid", ArticleCount: 2},
	}
	articleSets := [][]uint{
		{},
		{10, 20},
	}

	result := DeduplicateTags(context.Background(), tags, articleSets, nil)
	assert.Len(t, result, 1)
	assert.Equal(t, uint(2), result[0].ID)
}

// TestLoadDedupRelevance（故事 B-6）：orchestrator 接线的信号加载——
// 成功路径返回解析后的 identity/board 向量；版块缺 embedding 或查询失败
// 降级 nil（去重回退 ID 顺序，管线不中断）。
func TestLoadDedupRelevance(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.Repo = repository.NewTopicGraphRepository(db)

	pgVec := repository.FloatsToPgVector(testutil.PadVector([]float64{0.1, 0.2, 0.3}, testutil.TestEmbeddingDim))
	board := models.SemanticLabel{Label: "dedup-wire-board", Slug: "dedup-wire-board", LabelType: "board", Status: "active", Embedding: &pgVec}
	require.NoError(t, db.Create(&board).Error)
	boardNoEmb := models.SemanticLabel{Label: "dedup-wire-noemb", Slug: "dedup-wire-noemb", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&boardNoEmb).Error)

	tag := models.TopicTag{Slug: "dedup-wire-tag", Label: "dedup-wire-tag", Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID: tag.ID, EmbeddingType: "identity", EmbeddingVec: pgVec,
		Dimension: testutil.TestEmbeddingDim, Model: "test", TextHash: fmt.Sprintf("hash-%d", tag.ID),
	}).Error)

	tags := []repository.TagInput{{ID: tag.ID, Label: "dedup-wire-tag"}}

	t.Run("loads identity and board embeddings", func(t *testing.T) {
		relevance := loadDedupRelevance(board.ID, tags)
		require.NotNil(t, relevance)
		assert.Len(t, relevance.BoardEmbedding, testutil.TestEmbeddingDim)
		assert.InDelta(t, 0.1, relevance.BoardEmbedding[0], 1e-9)
		vec, ok := relevance.TagIdentityEmbeddings[tag.ID]
		require.True(t, ok)
		assert.Len(t, vec, testutil.TestEmbeddingDim)
	})

	t.Run("board without embedding degrades to nil", func(t *testing.T) {
		assert.Nil(t, loadDedupRelevance(boardNoEmb.ID, tags))
	})

	t.Run("query failure degrades to nil without breaking", func(t *testing.T) {
		// 借用本包 unreachableGormDB 模式：首次查询即失败，不碰共享容器连接。
		// 临时换全局 Repo，用完立刻还原。
		origRepo := repository.Repo
		repository.Repo = repository.NewTopicGraphRepository(unreachableGormDB(t))
		defer func() { repository.Repo = origRepo }()
		assert.Nil(t, loadDedupRelevance(board.ID, tags), "load failure degrades to the legacy ID tiebreak")
	})
}

func tagIDs(tags []repository.TagInput) []uint {
	ids := make([]uint, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	return ids
}
