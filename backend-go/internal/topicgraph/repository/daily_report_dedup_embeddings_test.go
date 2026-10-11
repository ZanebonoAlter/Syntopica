package repository

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 用例来源：fix-digest-tag-leak tasks 2.1 —— 日报去重 tiebreak 的两条
// embedding 查询：标签 identity 向量批量加载（只取 identity 轨）与
// 版块 embedding 单行加载（active board、缺失返回空串）。
func TestListTagIdentityEmbeddings(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	seedTag := func(slug string) models.TopicTag {
		tag := models.TopicTag{Slug: slug, Label: slug, Category: models.TagCategoryEvent, Status: "active"}
		require.NoError(t, db.Create(&tag).Error)
		return tag
	}
	seedEmbedding := func(tagID uint, embType, textHash string) {
		pgVec := FloatsToPgVector(testutil.PadVector([]float64{0.1, 0.2, 0.3}, testutil.TestEmbeddingDim))
		require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
			TopicTagID: tagID, EmbeddingType: embType, EmbeddingVec: pgVec,
			Dimension: testutil.TestEmbeddingDim, Model: "test", TextHash: textHash,
		}).Error)
	}

	tagA := seedTag("dedup-emb-a")
	tagB := seedTag("dedup-emb-b")
	tagC := seedTag("dedup-emb-c")
	seedEmbedding(tagA.ID, "identity", fmt.Sprintf("hash-id-%d", tagA.ID))
	// tagB 只有 semantic 轨 → 不得混入 identity 结果。
	seedEmbedding(tagB.ID, "semantic", fmt.Sprintf("hash-sem-%d", tagB.ID))
	// tagC 无任何 embedding。

	out, err := repo.ListTagIdentityEmbeddings([]uint{tagA.ID, tagB.ID, tagC.ID})
	require.NoError(t, err)
	require.Len(t, out, 1, "only the identity row is returned")
	require.Contains(t, out[tagA.ID], "0.1", "identity vector content preserved")

	empty, err := repo.ListTagIdentityEmbeddings(nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestGetBoardEmbedding(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	pgVec := FloatsToPgVector(testutil.PadVector([]float64{0.4, 0.5, 0.6}, testutil.TestEmbeddingDim))
	boardWithEmb := models.SemanticLabel{
		Label: "dedup-emb-board", Slug: "dedup-emb-board", LabelType: "board",
		Status: "active", Embedding: &pgVec,
	}
	require.NoError(t, db.Create(&boardWithEmb).Error)
	boardNoEmb := models.SemanticLabel{
		Label: "dedup-noemb-board", Slug: "dedup-noemb-board", LabelType: "board", Status: "active",
	}
	require.NoError(t, db.Create(&boardNoEmb).Error)
	boardDisabled := models.SemanticLabel{
		Label: "dedup-disabled-board", Slug: "dedup-disabled-board", LabelType: "board",
		Status: "disabled", Embedding: &pgVec,
	}
	require.NoError(t, db.Create(&boardDisabled).Error)

	got, err := repo.GetBoardEmbedding(boardWithEmb.ID)
	require.NoError(t, err)
	require.Contains(t, got, "0.4", "board embedding content preserved")

	missing, err := repo.GetBoardEmbedding(boardNoEmb.ID)
	require.NoError(t, err)
	require.Empty(t, missing, "board without embedding → empty string")

	disabled, err := repo.GetBoardEmbedding(boardDisabled.ID)
	require.NoError(t, err)
	require.Empty(t, disabled, "disabled board is not matched")

	absent, err := repo.GetBoardEmbedding(999999)
	require.NoError(t, err)
	require.Empty(t, absent, "nonexistent board → empty string")
}
