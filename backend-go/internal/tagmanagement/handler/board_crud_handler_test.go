package handler

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service"
)

// ── 版块编辑诚实化修复（lane-manual-correction D9 / board-match-cache） ─────
// 改版块 label/description 后必须让 board embeddings 缓存失效（缓存无 TTL，
// 不失效则方向校验继续用旧向量）；且**不**自动重跑匹配（topic_tag_board_labels
// 保持旧结果，回填是用户的显式动作）。

// boardEmbedderStub 返回一个固定方向的向量（测试可切换方向以模拟「改版块名
// 后向量变了」）。
func boardEmbedderStub(direction []float64) func(context.Context, string, service.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	return func(_ context.Context, _ string, _ service.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		vec := testutil.PadVector(direction, testutil.TestEmbeddingDim)
		return service.FloatsToPgVector(vec), vec, nil
	}
}

// TestUpdateSemanticBoard_LabelChangeInvalidatesMatchCache covers
// board-match-cache「Cache invalidates on board label change」: the board
// embeddings cache has no TTL, so the edit path MUST invalidate it — otherwise
// the direction gate keeps judging against the old vector.
func TestUpdateSemanticBoard_LabelChangeInvalidatesMatchCache(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)
	service.InvalidateMatchCache()
	t.Cleanup(service.InvalidateMatchCache)

	// 计数失效调用（seam 包住真实实现，行为不变）。
	var invalidations int
	original := invalidateMatchCache
	invalidateMatchCache = func() {
		invalidations++
		original()
	}
	t.Cleanup(func() { invalidateMatchCache = original })

	// tag + 两个辅助标签（构成 direct_hit 的重叠证据）。
	tag := createHandlerTopicTag(t, db, "方向校验标签", models.TagCategoryEvent)
	auxA := createHandlerSemanticLabel(t, db, "AuxA", "aux-a", "auxiliary", "active", 1, []float64{1, 0, 0})
	auxB := createHandlerSemanticLabel(t, db, "AuxB", "aux-b", "auxiliary", "active", 1, []float64{0, 1, 0})
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxB.ID}).Error)
	// tag identity 向量 [1,0,0]。
	require.NoError(t, db.Create(&tagmodels.TopicTagEmbedding{
		TopicTagID:    tag.ID,
		EmbeddingType: "identity",
		EmbeddingVec:  service.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)),
		Dimension:     testutil.TestEmbeddingDim,
		Model:         "test",
		TextHash:      fmt.Sprintf("hash-%d", tag.ID),
	}).Error)
	// 参照版块（不在候选集内，只提供「全库最优方向」参照系）。
	createHandlerSemanticLabel(t, db, "参照版块", "ref-board", "board", "active", 0, []float64{1, 0, 0})

	// 被编辑的版块：初始向量与 tag 同向（stub 默认 [1,0,0]）。
	created := performJSON(t, router, http.MethodPost, "/api/semantic-boards", map[string]any{
		"label":       "方向版块",
		"description": "初始描述",
	})
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	boardID := uint(responseDataFloat(t, created, "id"))
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: boardID, AuxiliaryLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&tagmodels.BoardComposition{BoardID: boardID, AuxiliaryLabelID: auxB.ID}).Error)

	// 先跑一次匹配：缓存里装的是旧向量（同向 → 不判方向不符）。
	matcher := service.NewSemanticBoardMatchingService(db)
	matches, err := matcher.MatchTopicTag(context.Background(), tag.ID)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, "direct_hit", matches[0].MatchReason)
	require.False(t, matches[0].DirectionMismatch, "旧向量与 tag 同向 → 不判方向不符")

	var matchRowsBefore []tagmodels.TopicTagBoardLabel
	require.NoError(t, db.Find(&matchRowsBefore).Error)
	require.Len(t, matchRowsBefore, 1)

	// 改版块名：stub 换成反向向量（模拟新 label 的新 embedding）。
	semanticBoardLabelEmbedder = boardEmbedderStub([]float64{0, 0, 1})
	updated := performJSON(t, router, http.MethodPut, fmt.Sprintf("/api/semantic-boards/%d", boardID), map[string]any{
		"label":       "改名后的方向版块",
		"description": "初始描述",
	})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Equal(t, 1, invalidations, "label 变化必须失效板级匹配缓存")

	// 匹配表不被自动重跑（回填是用户的显式动作）。
	var matchRowsAfter []tagmodels.TopicTagBoardLabel
	require.NoError(t, db.Find(&matchRowsAfter).Error)
	require.Len(t, matchRowsAfter, 1)
	assert.Equal(t, matchRowsBefore[0].TopicTagID, matchRowsAfter[0].TopicTagID)
	assert.Equal(t, matchRowsBefore[0].SemanticBoardID, matchRowsAfter[0].SemanticBoardID)
	assert.Equal(t, matchRowsBefore[0].Score, matchRowsAfter[0].Score)
	assert.Equal(t, matchRowsBefore[0].MatchReason, matchRowsAfter[0].MatchReason)
	assert.Equal(t, matchRowsBefore[0].DirectionMismatch, matchRowsAfter[0].DirectionMismatch)

	// 方向校验现在用新向量：与 tag 反向 → 判方向不符（缓存没失效的话这里仍是
	// 旧结果，测试会红）。
	matches, err = matcher.MatchTopicTag(context.Background(), tag.ID)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.True(t, matches[0].DirectionMismatch, "方向校验必须使用改版块后的新向量")
}

// TestUpdateSemanticBoard_DescriptionChangeInvalidatesMatchCache covers
// board-match-cache「Cache invalidates on board description change」.
func TestUpdateSemanticBoard_DescriptionChangeInvalidatesMatchCache(t *testing.T) {
	db, router := setupSemanticBoardHandlerRouter(t)
	service.InvalidateMatchCache()
	t.Cleanup(service.InvalidateMatchCache)

	var invalidations int
	original := invalidateMatchCache
	invalidateMatchCache = func() {
		invalidations++
		original()
	}
	t.Cleanup(func() { invalidateMatchCache = original })

	created := performJSON(t, router, http.MethodPost, "/api/semantic-boards", map[string]any{
		"label":       "描述版块",
		"description": "旧描述",
	})
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	boardID := uint(responseDataFloat(t, created, "id"))

	updated := performJSON(t, router, http.MethodPut, fmt.Sprintf("/api/semantic-boards/%d", boardID), map[string]any{
		"label":       "描述版块",
		"description": "新描述",
	})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Equal(t, 1, invalidations, "description 变化必须失效板级匹配缓存")

	var board models.SemanticLabel
	require.NoError(t, db.First(&board, boardID).Error)
	assert.Equal(t, "新描述", board.Description)
}

// TestUpdateSemanticBoard_NoFieldChangeKeepsCache covers 白盒 D2「PUT board
// 无字段变化 → 不失效」: nothing about the board vector changed, so the cache
// stays warm (no needless invalidation).
func TestUpdateSemanticBoard_NoFieldChangeKeepsCache(t *testing.T) {
	_, router := setupSemanticBoardHandlerRouter(t)
	service.InvalidateMatchCache()
	t.Cleanup(service.InvalidateMatchCache)

	var invalidations int
	original := invalidateMatchCache
	invalidateMatchCache = func() {
		invalidations++
		original()
	}
	t.Cleanup(func() { invalidateMatchCache = original })

	created := performJSON(t, router, http.MethodPost, "/api/semantic-boards", map[string]any{
		"label":       "不变版块",
		"description": "描述",
	})
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	boardID := uint(responseDataFloat(t, created, "id"))

	updated := performJSON(t, router, http.MethodPut, fmt.Sprintf("/api/semantic-boards/%d", boardID), map[string]any{
		"label":       "不变版块",
		"description": "描述",
	})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	assert.Zero(t, invalidations, "字段没变就不该失效缓存")
}
