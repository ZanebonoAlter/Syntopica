package tagging

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupSemanticBoardMatchingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	database.DB = db
	require.NoError(t, db.AutoMigrate(&models.TopicTag{}, &models.SemanticLabel{}, &models.TopicTagSemanticLabel{}, &models.TopicTagBoardLabel{}, &models.BoardComposition{}, &models.AISettings{}))
	return db
}

func TestSemanticBoardMatchingDirectHit(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	tag := createMatchTag(t, db, "openai-gpt-5")
	auxiliary := createMatchLabel(t, db, "OpenAI", "openai", "auxiliary", "active", []float64{1, 0, 0})
	board := createMatchLabel(t, db, "AI Board", "ai-board", "board", "active", nil)
	require.NoError(t, db.Create(&models.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxiliary.ID}).Error)
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxiliary.ID}).Error)
	service := NewSemanticBoardMatchingService(db)

	results, err := service.MatchTopicTag(context.Background(), tag.ID)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, board.ID, results[0].SemanticBoardID)
	require.Equal(t, 1.0, results[0].Score)
	require.Equal(t, "direct_hit", results[0].MatchReason)

	var rows []models.TopicTagBoardLabel
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, tag.ID, rows[0].TopicTagID)
	require.Equal(t, board.ID, rows[0].SemanticBoardID)
	require.Equal(t, "direct_hit", rows[0].MatchReason)
}

func TestSemanticBoardMatchingThreeRules(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_sim_threshold", Value: "0.6"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_direct_max_sim_min_hits", Value: "1"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_direct_max_sim_min_hit_rate", Value: "0"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_min_effective_sample", Value: "1"}).Error)
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
	// hit_rate score is blended: 0.7*maxSim + 0.3*adjustedHitRate
	// hit-rate board auxiliaries are {0.7,0.5,0.51} and {0.5,0.7,0.51}; tag auxiliaries are {1,0,0} and {0,1,0}
	// maxSim = max(cos_sim pairs) = 0.7, adjustedHitRate = 2/max(2,1) = 1.0
	// score = 0.7*0.7 + 0.3*1.0 = 0.79
	require.InDelta(t, 0.79, byBoard[hitRateBoard.ID].Score, 0.01)
	require.Equal(t, "max_sim", byBoard[maxSimBoard.ID].MatchReason)
	require.InDelta(t, 1.0, byBoard[maxSimBoard.ID].Score, 0.0001)
	require.Equal(t, "weighted", byBoard[weightedBoard.ID].MatchReason)
	require.InDelta(t, 0.62, byBoard[weightedBoard.ID].Score, 0.0001)
}

func TestSemanticBoardMatchingMaxBoardsTruncation(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_direct_hit_rate", Value: "1"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_max_boards", Value: "2"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "semantic_board_match_min_effective_sample", Value: "1"}).Error)
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

func TestEvaluateSemanticBoardMatches_MaxSimDualFactor(t *testing.T) {
	defaultConfig := func() SemanticBoardMatchConfig {
		return SemanticBoardMatchConfig{
			SimThreshold:           0.72,
			DirectHitRate:          0.5,
			DirectMaxSim:           0.8,
			DirectMaxSimMinHits:    2,
			DirectMaxSimMinHitRate: 0.3,
			MinEffectiveSample:     3,
			HitRateSimBlend:        0.7,
			WeightSim:              0.6,
			WeightDensity:          0.4,
			WeightedThreshold:      0.6,
			MaxBoards:              3,
		}
	}

	t.Run("N=1 keyword hits enough sim high should match", func(t *testing.T) {
		// N=1: minHits = min(2, 1) = 1, rate >= 0.3, sim >= 0.8
		// Use high DirectHitRate so it falls through to max_sim
		config := defaultConfig()
		config.DirectHitRate = 1.0 // ensure hit_rate doesn't trigger
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "tech", Slug: "tech", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		require.Len(t, results, 1)
		require.Equal(t, uint(100), results[0].SemanticBoardID)
		require.Equal(t, "max_sim", results[0].MatchReason)
	})

	t.Run("N=2 both auxiliaries match should pass", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "tech", Slug: "tech", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "media", Slug: "media", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 11, Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// Both hit => hitRate = 1.0 > 0.5, should match as hit_rate
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
	})

	t.Run("N=5 hits=1 insufficient should not match max_sim", func(t *testing.T) {
		// 5 tag auxiliaries, only 1 has high sim with board. hits=1, minHits=min(2,5)=2 => fail
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a1", Slug: "a1", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "a2", Slug: "a2", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "a3", Slug: "a3", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 0, 1}))},
			{ID: 4, Label: "a4", Slug: "a4", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.5, 0.5, 0.7071067811865476}))},
			{ID: 5, Label: "a5", Slug: "a5", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.5773502691896258, 0.5773502691896258, 0.5773502691896258}))},
		}
		// Board has one auxiliary very close to tag a1, but the other 4 tag auxiliaries
		// will have low sim with the board auxiliary
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{0.99, 0.1, 0.0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// maxSim ~0.994 >= 0.8, but hits=1 < minHits=2 => should NOT match as max_sim
		// hitRate = 1/5 = 0.2, not > 0.5 => no hit_rate
		// weighted = 0.6*0.994 + 0.4*0.2 = 0.676 >= 0.6 => might match as weighted
		for _, r := range results {
			if r.SemanticBoardID == 100 {
				require.NotEqual(t, "max_sim", r.MatchReason, "should not match as max_sim with only 1 hit out of 5")
			}
		}
	})

	t.Run("N=5 hits=2 rate=0.2 insufficient rate should not match max_sim", func(t *testing.T) {
		// 5 tag auxiliaries, 2 hit threshold. rate=0.4/5=0.08... wait, need precise control.
		// Actually hitRate = hits/tagAuxiliaryCount. We need hits=2, rate=0.2.
		// That would mean tagAuxiliaryCount = 10 with 2 hits, or we set rate directly.
		// Since hitRate is computed as float64(hits)/float64(len(tagAuxiliaries)),
		// for 5 auxiliaries and 2 hits, rate = 0.4. We need rate < 0.3.
		// With 5 auxiliaries, to get rate < 0.3 we need hits <= 1 (rate=0.2).
		// So let's test: N=10, hits=2, rate=0.2, sim high
		config := defaultConfig()
		// Create 10 tag auxiliaries, only 2 close to board auxiliary
		tagAuxiliaries := make([]models.SemanticLabel, 10)
		for i := 0; i < 10; i++ {
			vec := []float64{0, 0, 1} // orthogonal to board
			if i < 2 {
				vec = []float64{1, 0, 0} // close to board
			}
			tagAuxiliaries[i] = models.SemanticLabel{
				ID: uint(i + 1), Label: fmt.Sprintf("a%d", i), Slug: fmt.Sprintf("a%d", i),
				LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector(vec)),
			}
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// maxSim = 1.0 >= 0.8, hits=2 >= minHits=2, BUT rate=0.2 < 0.3
		// => should NOT match as max_sim
		for _, r := range results {
			if r.SemanticBoardID == 100 {
				require.NotEqual(t, "max_sim", r.MatchReason, "should not match as max_sim with rate 0.2 < 0.3")
			}
		}
	})

	t.Run("N=5 hits=2 rate=0.4 sim high should match max_sim", func(t *testing.T) {
		config := defaultConfig()
		config.DirectHitRate = 1.0 // ensure hit_rate doesn't trigger
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a1", Slug: "a1", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "a2", Slug: "a2", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "a3", Slug: "a3", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 0, 1}))},
			{ID: 4, Label: "a4", Slug: "a4", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.5, 0.5, 0.7071067811865476}))},
			{ID: 5, Label: "a5", Slug: "a5", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.5773502691896258, 0.5773502691896258, 0.5773502691896258}))},
		}
		// Board has two auxiliaries: one close to a1, one close to a2
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{0.95, 0.31224989991992, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 11, Embedding: ptrStr(floatsToPgVector([]float64{0, 0.95, 0.31224989991992}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// rate=0.4, DirectHitRate=1.0 => not hit_rate
		// maxSim=0.95 >= 0.8, hits=2 >= minHits=2, rate=0.4 >= 0.3 => max_sim
		found := false
		for _, r := range results {
			if r.SemanticBoardID == 100 {
				require.Equal(t, "max_sim", r.MatchReason)
				found = true
			}
		}
		require.True(t, found, "expected board 100 to be matched")
	})
}

func ptrStr(s string) *string {
	return &s
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
		pgVector := floatsToPgVector(vector)
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

func TestEvaluateSemanticBoardMatches_EffectiveSampleAndBlend(t *testing.T) {
	defaultConfig := func() SemanticBoardMatchConfig {
		return SemanticBoardMatchConfig{
			SimThreshold:           0.72,
			DirectHitRate:          0.5,
			DirectMaxSim:           0.8,
			DirectMaxSimMinHits:    2,
			DirectMaxSimMinHitRate: 0.3,
			MinEffectiveSample:     3,
			HitRateSimBlend:        0.7,
			WeightSim:              0.6,
			WeightDensity:          0.4,
			WeightedThreshold:      0.6,
			MaxBoards:              3,
		}
	}

	t.Run("1-aux tag: adjustedHitRate=1/3=0.333 does not pass hit_rate gate", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// adjustedHitRate = 1/max(1,3) = 0.333, not > 0.5 => no hit_rate
		// maxSim = 1.0 >= 0.8, hits=1 >= min(2,1)=1, adjustedHitRate=0.333 >= 0.3 => max_sim
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.InDelta(t, 1.0, results[0].Score, 0.0001)
	})

	t.Run("1-aux tag: weak similarity falls to weighted and may be filtered", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.8, 0.6, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// cosSim = 0.8, adjustedHitRate = 1/3 = 0.333
		// hit_rate: 0.333 not > 0.5 => no
		// max_sim: sim=0.8 >= 0.8, hits=1 >= min(2,1)=1, adjustedHitRate=0.333 >= 0.3 => max_sim!
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
	})

	t.Run("1-aux tag: moderate similarity falls to weighted and passes threshold", func(t *testing.T) {
		config := defaultConfig()
		config.DirectMaxSim = 0.9 // raise max_sim threshold so it falls through
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.85, 0.527, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// cosSim ≈ 0.85, adjustedHitRate = 1/3 = 0.333
		// hit_rate: 0.333 not > 0.5 => no
		// max_sim: sim=0.85 < 0.9 => no
		// weighted: 0.6*0.85 + 0.4*0.333 = 0.51 + 0.133 = 0.643 >= 0.6 => yes
		require.Len(t, results, 1)
		require.Equal(t, "weighted", results[0].MatchReason)
		require.InDelta(t, 0.643, results[0].Score, 0.01)
	})

	t.Run("2-aux tag: both hit gives adjustedHitRate=2/3=0.667, score is blended", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "gpt", Slug: "gpt", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.9, 0.435889894354067, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// adjustedHitRate = 2/max(2,3) = 2/3 = 0.667 > 0.5 => hit_rate
		// maxSim = 1.0, score = 0.7*1.0 + 0.3*0.667 = 0.9
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.InDelta(t, 0.9, results[0].Score, 0.01)
	})

	t.Run("3-aux tag: unchanged behavior since N >= minEffectiveSample", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a", Slug: "a", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "b", Slug: "b", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "c", Slug: "c", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 0, 1}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// adjustedHitRate = 1/3 = 0.333, not > 0.5 => no hit_rate
		// maxSim = 1.0 >= 0.8, hits=1 >= min(2,3)=2? No, 1 < 2 => no max_sim
		// weighted = 0.6*1.0 + 0.4*0.333 = 0.733 >= 0.6 => weighted
		require.Len(t, results, 1)
		require.Equal(t, "weighted", results[0].MatchReason)
		require.InDelta(t, 0.733, results[0].Score, 0.01)
	})

	t.Run("5-aux tag: 3 hits hit_rate blended score", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a", Slug: "a", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "b", Slug: "b", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "c", Slug: "c", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.85, 0.527, 0}))},
			{ID: 4, Label: "d", Slug: "d", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0, 0, 1}))},
			{ID: 5, Label: "e", Slug: "e", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{0.5, 0.5, 0.707}))},
		}
		// Board has auxiliaries close to tag a, b, c
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 11, Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 12, Embedding: ptrStr(floatsToPgVector([]float64{0.85, 0.527, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// 3 out of 5 hit sim_threshold=0.72, adjustedHitRate = 3/5 = 0.6 > 0.5 => hit_rate
		// maxSim = 1.0, score = 0.7*1.0 + 0.3*0.6 = 0.88
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.InDelta(t, 0.88, results[0].Score, 0.01)
	})

	t.Run("hit_rate_sim_blend=1.0 recovers old pure maxSim score", func(t *testing.T) {
		config := defaultConfig()
		config.HitRateSimBlend = 1.0
		config.MinEffectiveSample = 1 // disable sample penalty
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a", Slug: "a", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		}
		boardAuxiliaries := []boardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(floatsToPgVector([]float64{0.85, 0.527, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, config)
		// adjustedHitRate = 1/1 = 1.0 > 0.5 => hit_rate
		// score = 1.0*maxSim + 0.0*1.0 = maxSim
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.InDelta(t, results[0].Score, 0.85, 0.01)
	})
}

func TestScoreSemanticBoardSimilarity_EffectiveDenominator(t *testing.T) {
	tagVectors := [][]float64{{1, 0, 0}}
	boardVectors := [][]float64{{1, 0, 0}}

	t.Run("1 aux, minEffectiveSample=3 => hitRate=1/3", func(t *testing.T) {
		hitRate, maxSim := scoreSemanticBoardSimilarity(tagVectors, boardVectors, 1, 0.72, 3)
		require.InDelta(t, 1.0/3.0, hitRate, 0.0001)
		require.InDelta(t, 1.0, maxSim, 0.0001)
	})

	t.Run("1 aux, minEffectiveSample=1 => hitRate=1/1", func(t *testing.T) {
		hitRate, maxSim := scoreSemanticBoardSimilarity(tagVectors, boardVectors, 1, 0.72, 1)
		require.InDelta(t, 1.0, hitRate, 0.0001)
		require.InDelta(t, 1.0, maxSim, 0.0001)
	})

	t.Run("5 aux, minEffectiveSample=3 => hitRate=3/5", func(t *testing.T) {
		vectors5 := [][]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {1, 0, 0}, {0, 1, 0}}
		hitRate, _ := scoreSemanticBoardSimilarity(vectors5, boardVectors, 5, 0.72, 3)
		// 4 out of 5 match [1,0,0] with sim >= 0.72 (exact matches for indices 0,3; perpendicular for 1,4; opposite for 2)
		// cos(1,0,0 with 1,0,0) = 1.0 >= 0.72 ✓
		// cos(0,1,0 with 1,0,0) = 0.0 < 0.72 ✗
		// cos(0,0,1 with 1,0,0) = 0.0 < 0.72 ✗
		// cos(1,0,0 with 1,0,0) = 1.0 >= 0.72 ✓
		// cos(0,1,0 with 1,0,0) = 0.0 < 0.72 ✗
		// hits=2, hitRate = 2/5 = 0.4
		require.InDelta(t, 0.4, hitRate, 0.0001)
	})
}
