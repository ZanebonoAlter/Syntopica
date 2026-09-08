package board

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
)

func setupSemanticBoardMatchingTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	InvalidateBoardCache() // 避免包级缓存跨测试残留（ResetTestData 清 db 但不清内存缓存）
	return db
}

func TestSemanticBoardMatchingDirectHit(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "openai-gpt-5")
	auxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	secondaryAuxiliary := createMatchLabel(t, db, "GPT", "gpt", "auxiliary", "active", []float64{0, 1, 0})
	board := createMatchLabel(t, db, "AI Board", "ai-board", "board", "active", nil)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: secondaryAuxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: secondaryAuxiliary.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board.ID, results[0].SemanticBoardID)
	require.Equal(t, "direct_hit", results[0].MatchReason)
	// add-composite-labels: direct_hit downgraded to DirectHitScoreFactor (default 0.7).
	require.InDelta(t, 0.7, results[0].Score, 0.0001)
	// No tag identity embedding → direction check skipped → mismatch=false.
	require.False(t, results[0].DirectionMismatch)

	var rows []models.TopicTagBoardLabel
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, tag.ID, rows[0].TopicTagID)
	require.Equal(t, board.ID, rows[0].SemanticBoardID)
	require.Equal(t, "direct_hit", rows[0].MatchReason)
	require.InDelta(t, 0.7, rows[0].Score, 0.0001)
	require.False(t, rows[0].DirectionMismatch)
}

// TestSemanticBoardMatchingDirectHitDirectionMismatch covers the forced
// direction check on downgraded direct_hit: identity embedding present and
// cosine below threshold → still mounted but flagged direction_mismatch=true.
func TestSemanticBoardMatchingDirectHitDirectionMismatch(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "yield-direction")
	auxA := createMatchLabel(t, db, "YieldA", "yield-a", "auxiliary", "active", []float64{1, 0, 0})
	auxB := createMatchLabel(t, db, "YieldB", "yield-b", "auxiliary", "active", []float64{0, 1, 0})
	board := createMatchLabel(t, db, "Yield Board", "yield-board", "board", "active", []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxB.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxB.ID}).Error)
	// tag identity embedding [1,0,0] vs board embedding [0,0,1] → cosine 0 < 0.5.
	pgVector := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	require.NoError(t, db.Create(&models.TopicTagEmbedding{TopicTagID: tag.ID, EmbeddingType: "identity", EmbeddingVec: pgVector, Dimension: testutil.TestEmbeddingDim, Model: "test", TextHash: fmt.Sprintf("hash-%d", tag.ID)}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "direct_hit", results[0].MatchReason)
	require.InDelta(t, 0.7, results[0].Score, 0.0001)
	require.True(t, results[0].DirectionMismatch, "direction cosine below threshold must flag mismatch")
}

// TestSemanticBoardMatchingDirectHitScoreFactorConfigured covers S5 变体3:
// direct_hit_score_factor from ai_settings (0.5) takes effect.
func TestSemanticBoardMatchingDirectHitScoreFactorConfigured(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_score_factor", "0.5")
	tag := createMatchTag(t, db, "factor-config")
	auxA := createMatchLabel(t, db, "FactorA", "factor-a", "auxiliary", "active", []float64{1, 0, 0})
	auxB := createMatchLabel(t, db, "FactorB", "factor-b", "auxiliary", "active", []float64{0, 1, 0})
	board := createMatchLabel(t, db, "Factor Board", "factor-board", "board", "active", nil)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxB.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxA.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxB.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "direct_hit", results[0].MatchReason)
	require.InDelta(t, 0.5, results[0].Score, 0.0001)
}

// createMatchComposite creates an active composite label with an embedding.
func createMatchComposite(t *testing.T, db *gorm.DB, label string, slug string, vector []float64) models.SemanticLabel {
	t.Helper()
	return createMatchLabel(t, db, label, slug, "composite", "active", vector)
}

// TestSemanticBoardMatchingCompositeHit covers S5 步1 + S6 步1 + 变体4/5:
// tag composite ∩ board composite → composite_hit score=1.0, exempt from
// direction check even when identity embedding direction disagrees.
func TestSemanticBoardMatchingCompositeHit(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "us-treasury-yield")
	composite := createMatchComposite(t, db, "美债收益率", "us-treasury-yield-comp", []float64{1, 0, 0})
	board := createMatchLabel(t, db, "美债观察", "us-treasury-watch", "board", "active", []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: composite.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: composite.ID}).Error)
	// tag identity embedding [1,0,0] vs board embedding [0,0,1] → cosine 0;
	// composite_hit must stay exempt from the direction check.
	pgVector := core.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	require.NoError(t, db.Create(&models.TopicTagEmbedding{TopicTagID: tag.ID, EmbeddingType: "identity", EmbeddingVec: pgVector, Dimension: testutil.TestEmbeddingDim, Model: "test", TextHash: fmt.Sprintf("hash-%d", tag.ID)}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board.ID, results[0].SemanticBoardID)
	require.Equal(t, "composite_hit", results[0].MatchReason)
	require.InDelta(t, 1.0, results[0].Score, 0.0001)
	require.False(t, results[0].DirectionMismatch, "composite_hit is exempt from direction check")
}

// TestSemanticBoardMatchingCompositeVariants covers S5 变体4/5 + S6 变体1:
// disabled composites and NULL-embedding composites never participate in
// composite_hit; multiple composites with partial intersection still hit.
func TestSemanticBoardMatchingCompositeVariants(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "composite-variants")
	// Case A: board composite is disabled → no composite_hit.
	tagActive := createMatchComposite(t, db, "组合甲", "comp-a", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: tagActive.ID}).Error)
	boardDisabled := createMatchLabel(t, db, "禁用组合板", "disabled-comp-board", "board", "active", nil)
	disabledComposite := createMatchLabel(t, db, "组合甲旧", "comp-a-old", "composite", "disabled", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardDisabled.ID, AuxiliaryLabelID: disabledComposite.ID}).Error)
	// tag composite with NULL embedding (active but missing vector) → skipped.
	boardNullVector := createMatchLabel(t, db, "无向量组合板", "null-vector-board", "board", "active", nil)
	nullComposite := createMatchLabel(t, db, "组合乙", "comp-b-null", "composite", "active", nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardNullVector.ID, AuxiliaryLabelID: nullComposite.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: nullComposite.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Empty(t, results, "disabled / NULL-vector composites must not produce composite_hit")
}

// TestSemanticBoardMatchingCompositeOverridesDirectHit covers S5 步4 + 变体6:
// board with both composite and single-label overlap records only
// composite_hit (priority), and composite_hit outranks direct_hit in sorting.
func TestSemanticBoardMatchingCompositeOverridesDirectHit(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "priority-tag")
	aux := createMatchLabel(t, db, "SharedAux", "shared-aux", "auxiliary", "active", []float64{1, 0, 0})
	aux2 := createMatchLabel(t, db, "SharedAux2", "shared-aux-2", "auxiliary", "active", []float64{0, 1, 0})
	composite := createMatchComposite(t, db, "共享组合", "shared-comp", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: aux.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: aux2.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: composite.ID}).Error)

	boardA := createMatchLabel(t, db, "Both Board", "both-board", "board", "active", nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardA.ID, AuxiliaryLabelID: aux.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardA.ID, AuxiliaryLabelID: aux2.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardA.ID, AuxiliaryLabelID: composite.ID}).Error)

	boardB := createMatchLabel(t, db, "Aux Only Board", "aux-only-board", "board", "active", nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardB.ID, AuxiliaryLabelID: aux.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardB.ID, AuxiliaryLabelID: aux2.ID}).Error)

	service := NewSemanticBoardMatchingService(db)
	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, boardA.ID, results[0].SemanticBoardID, "composite_hit (1.0) sorts before direct_hit (0.7)")
	require.Equal(t, "composite_hit", results[0].MatchReason)
	require.InDelta(t, 1.0, results[0].Score, 0.0001)
	require.Equal(t, boardB.ID, results[1].SemanticBoardID)
	require.Equal(t, "direct_hit", results[1].MatchReason)
	require.InDelta(t, 0.7, results[1].Score, 0.0001)
}

// TestSemanticBoardMatchingCompositeOnlyBoard covers boards mounted with only
// composites (no auxiliaries): still eligible for composite_hit — the aux-count
// pre-filter must not drop them.
func TestSemanticBoardMatchingCompositeOnlyBoard(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "composite-only")
	composite := createMatchComposite(t, db, "纯组合", "pure-comp", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: composite.ID}).Error)
	board := createMatchLabel(t, db, "纯组合板", "pure-comp-board", "board", "active", nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: composite.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board.ID, results[0].SemanticBoardID)
	require.Equal(t, "composite_hit", results[0].MatchReason)
	require.InDelta(t, 1.0, results[0].Score, 0.0001)
}

// TestSemanticBoardMatchingCompositeCache covers S6 步2/3: board composites
// enter the board cache; composition changes are only visible after
// InvalidateBoardCache.
func TestSemanticBoardMatchingCompositeCache(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "cache-comp")
	compositeA := createMatchComposite(t, db, "缓存组合甲", "cache-comp-a", []float64{1, 0, 0})
	compositeB := createMatchComposite(t, db, "缓存组合乙", "cache-comp-b", []float64{0, 1, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: compositeA.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: compositeB.ID}).Error)
	boardA := createMatchLabel(t, db, "缓存板甲", "cache-board-a", "board", "active", nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardA.ID, AuxiliaryLabelID: compositeA.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	first, err := service.MatchTopicTag(context.Background(), tag.ID)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, boardA.ID, first[0].SemanticBoardID)

	// Mount compositeB on a new board WITHOUT invalidating the cache — the
	// next match must still come from cache and not see the new board.
	boardB := createMatchLabel(t, db, "缓存板乙", "cache-board-b", "board", "active", nil)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: boardB.ID, AuxiliaryLabelID: compositeB.ID}).Error)
	cached, err := service.MatchTopicTag(context.Background(), tag.ID)
	require.NoError(t, err)
	require.Len(t, cached, 1, "stale cache must hide the newly mounted composite board")
	require.Equal(t, boardA.ID, cached[0].SemanticBoardID)

	// Invalidate → the new board becomes visible.
	InvalidateBoardCache()
	refreshed, err := service.MatchTopicTag(context.Background(), tag.ID)
	require.NoError(t, err)
	require.Len(t, refreshed, 2)
	reasons := map[uint]string{refreshed[0].SemanticBoardID: refreshed[0].MatchReason, refreshed[1].SemanticBoardID: refreshed[1].MatchReason}
	require.Equal(t, "composite_hit", reasons[boardA.ID])
	require.Equal(t, "composite_hit", reasons[boardB.ID])
}

func TestSemanticBoardMatchingThreeRules(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	upsertMatchSetting(t, db, "semantic_board_match_sim_threshold", "0.6")
	upsertMatchSetting(t, db, "semantic_board_match_direct_max_sim_min_hits", "1")
	upsertMatchSetting(t, db, "semantic_board_match_direct_max_sim_min_hit_rate", "0")
	upsertMatchSetting(t, db, "semantic_board_match_min_effective_sample", "1")
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_min_overlap", "1")
	tag := createMatchTag(t, db, "model-release")
	tagAuxA := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	tagAuxB := createMatchLabel(t, db, "Release", "release", "auxiliary", "active", []float64{0, 1, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: tagAuxA.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: tagAuxB.ID}).Error)

	hitRateBoard := createMatchBoardWithAuxiliaries(t, db, "hit-rate", [][]float64{{0.7, 0.5, 0.509901951359279}, {0.5, 0.7, 0.509901951359279}})
	maxSimBoard := createMatchBoardWithAuxiliaries(t, db, "max-sim", [][]float64{{1, 0, 0}})
	weightedBoard := createMatchBoardWithAuxiliaries(t, db, "weighted", [][]float64{{0.7, 0.5, 0.509901951359279}})
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 3)
	byBoard := map[uint]SemanticBoardMatchResult{}
	for _, result := range results {
		byBoard[result.SemanticBoardID] = result
	}
	require.Equal(t, "hit_rate", byBoard[hitRateBoard.ID].MatchReason)
	require.InDelta(t, 0.79, byBoard[hitRateBoard.ID].Score, 0.01)
	require.Equal(t, "max_sim", byBoard[maxSimBoard.ID].MatchReason)
	require.InDelta(t, 1.0, byBoard[maxSimBoard.ID].Score, 0.0001)
	require.Equal(t, "weighted", byBoard[weightedBoard.ID].MatchReason)
	require.InDelta(t, 0.62, byBoard[weightedBoard.ID].Score, 0.0001)
}

func TestSemanticBoardMatchingMaxBoardsTruncation(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_rate", "1")
	upsertMatchSetting(t, db, "semantic_board_match_max_boards", "2")
	upsertMatchSetting(t, db, "semantic_board_match_min_effective_sample", "1")
	upsertMatchSetting(t, db, "semantic_board_match_direct_hit_min_overlap", "1")
	tag := createMatchTag(t, db, "ranked-boards")
	tagAux := createMatchLabel(t, db, "GPU", "gpu", "auxiliary", "active", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: tagAux.ID}).Error)
	top := createMatchBoardWithAuxiliaries(t, db, "top", [][]float64{{0.95, 0.31224989991992, 0}})
	second := createMatchBoardWithAuxiliaries(t, db, "second", [][]float64{{0.9, 0.435889894354067, 0}})
	third := createMatchBoardWithAuxiliaries(t, db, "third", [][]float64{{0.85, 0.526782687642637, 0}})
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, top.ID, results[0].SemanticBoardID)
	require.Equal(t, second.ID, results[1].SemanticBoardID)
	for _, result := range results {
		require.NotEqual(t, third.ID, result.SemanticBoardID)
	}
	var rows []models.TopicTagBoardLabel
	require.NoError(t, db.Order("score desc").Find(&rows).Error)
	require.Len(t, rows, 2)
}

func TestSemanticBoardMatchingNoMatchReplacesExistingLabels(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "no-match")
	otherTag := createMatchTag(t, db, "other")
	oldBoard := createMatchLabel(t, db, "Old Board", "old-board", "board", "active", nil)
	otherBoard := createMatchLabel(t, db, "Other Board", "other-board", "board", "active", nil)
	tagAux := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	boardAux := createMatchLabel(t, db, "Hardware", "hardware", "auxiliary", "active", []float64{0, 1, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: tagAux.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: oldBoard.ID, AuxiliaryLabelID: boardAux.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: oldBoard.ID, Score: 0.8, MatchReason: "existing"}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: otherTag.ID, SemanticBoardID: otherBoard.ID, Score: 0.9, MatchReason: "existing"}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Empty(t, results)
	var tagRows int64
	require.NoError(t, db.Model(&models.TopicTagBoardLabel{}).Where("topic_tag_id = ?", tag.ID).Count(&tagRows).Error)
	require.Zero(t, tagRows)
	var otherRows int64
	require.NoError(t, db.Model(&models.TopicTagBoardLabel{}).Where("topic_tag_id = ?", otherTag.ID).Count(&otherRows).Error)
	require.Equal(t, int64(1), otherRows)
}

func TestSemanticBoardMatchingColdStartNoBoard(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "cold-start")
	oldBoard := createMatchLabel(t, db, "Old Board", "old-board", "board", "active", nil)
	auxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: oldBoard.ID, Score: 0.8, MatchReason: "existing"}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Empty(t, results)
	var rows int64
	require.NoError(t, db.Model(&models.TopicTagBoardLabel{}).Where("topic_tag_id = ?", tag.ID).Count(&rows).Error)
	require.Zero(t, rows)
}

func TestSemanticBoardMatchingIgnoresDisabledLabels(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "disabled-labels")
	activeAux := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	disabledAux := createMatchLabel(t, db, "Disabled", "disabled", "auxiliary", "disabled", []float64{0, 1, 0})
	activeBoard := createMatchLabel(t, db, "Active Board", "active-board", "board", "active", nil)
	disabledBoard := createMatchLabel(t, db, "Disabled Board", "disabled-board", "board", "disabled", nil)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: activeAux.ID}).Error)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: disabledAux.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: activeBoard.ID, AuxiliaryLabelID: disabledAux.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: disabledBoard.ID, AuxiliaryLabelID: activeAux.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Empty(t, results)
}

func createMatchTag(t *testing.T, db *gorm.DB, slug string) models.TopicTag {
	tag := models.TopicTag{Label: slug, Slug: slug, Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)
	return tag
}

func createMatchLabel(t *testing.T, db *gorm.DB, label string, slug string, labelType string, status string, vector []float64) models.SemanticLabel {
	t.Helper()
	semanticLabel := models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: status}
	if vector != nil {
		pgVector := core.FloatsToPgVector(testutil.PadVector(vector, testutil.TestEmbeddingDim))
		semanticLabel.Embedding = &pgVector
	}
	require.NoError(t, db.Create(&semanticLabel).Error)
	return semanticLabel
}

func createMatchBoardWithAuxiliaries(t *testing.T, db *gorm.DB, slug string, vectors [][]float64) models.SemanticLabel {
	t.Helper()
	board := createMatchLabel(t, db, slug, slug, "board", "active", nil)
	for index, vector := range vectors {
		auxiliary := createMatchLabel(t, db, fmt.Sprintf("%s-aux-%d", slug, index), fmt.Sprintf("%s-aux-%d", slug, index), "auxiliary", "active", vector)
		require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	}
	return board
}

// TestSemanticBoardMatchingDerivedCompositeHit（8.2 真实库发现的链路缺口修复）：
// tag 未显式挂组合（topic_tag_semantic_labels 无 composite 行），但挂齐了某
// active 组合的全部组件 aux ⇒ 推导视为挂该组合 → composite_hit。
// 缺任一组件不推导；disabled 组合不推导；空组件组合不推导。
func TestSemanticBoardMatchingDerivedCompositeHit(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)

	auxFed := createMatchLabel(t, db, "美联储", "derived-fed", "auxiliary", "active", []float64{1, 0, 0})
	auxHike := createMatchLabel(t, db, "加息", "derived-hike", "auxiliary", "active", []float64{0, 1, 0})
	fullTag := createMatchTag(t, db, "derived-full")
	partialTag := createMatchTag(t, db, "derived-partial")
	for _, tid := range []uint{fullTag.ID, partialTag.ID} {
		require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tid, SemanticLabelID: auxFed.ID}).Error)
	}
	// fullTag 挂齐美联储+加息；partialTag 只挂美联储
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: fullTag.ID, SemanticLabelID: auxHike.ID}).Error)

	derived := createMatchComposite(t, db, "美联储加息", "derived-fed-hike", []float64{1, 0.5, 0})
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: derived.ID, ComponentLabelID: auxFed.ID, Position: 1}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: derived.ID, ComponentLabelID: auxHike.ID, Position: 2}).Error)
	board := createMatchLabel(t, db, "宏观观察", "derived-macro-board", "board", "active", []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: derived.ID}).Error)
	// 干扰项：disabled 组合组件与 fullTag 的 aux 集完全一致 → 不得推导
	disabledComposite := createMatchLabel(t, db, "旧美联储加息", "derived-fed-hike-old", "composite", "disabled", []float64{1, 0.4, 0})
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: disabledComposite.ID, ComponentLabelID: auxFed.ID, Position: 1}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: disabledComposite.ID, ComponentLabelID: auxHike.ID, Position: 2}).Error)

	service := NewSemanticBoardMatchingService(db)

	fullResults, err := service.MatchTopicTag(context.Background(), fullTag.ID)
	require.NoError(t, err)
	require.Len(t, fullResults, 1)
	require.Equal(t, board.ID, fullResults[0].SemanticBoardID)
	require.Equal(t, "composite_hit", fullResults[0].MatchReason)
	require.InDelta(t, 1.0, fullResults[0].Score, 0.0001)

	partialResults, err := service.MatchTopicTag(context.Background(), partialTag.ID)
	require.NoError(t, err)
	require.Empty(t, partialResults, "缺一个组件不得推导组合，也不得被间接规则挂载（无 board composition aux）")
}
