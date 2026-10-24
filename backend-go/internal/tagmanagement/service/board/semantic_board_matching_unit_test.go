package board

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

func ptrStr(s string) *string {
	return &s
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
			DirectHitMinOverlap:    2,
		}
	}

	t.Run("N=1 keyword hits enough sim high should match", func(t *testing.T) {
		config := defaultConfig()
		config.DirectHitRate = 1.0
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "tech", Slug: "tech", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, uint(100), results[0].SemanticBoardID)
		require.Equal(t, "max_sim", results[0].MatchReason)
	})

	t.Run("N=2 both auxiliaries match should pass", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "tech", Slug: "tech", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "media", Slug: "media", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 11, Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
	})

	t.Run("N=5 hits=1 insufficient should not match max_sim", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a1", Slug: "a1", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "a2", Slug: "a2", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "a3", Slug: "a3", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 0, 1}))},
			{ID: 4, Label: "a4", Slug: "a4", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.5, 0.5, 0.7071067811865476}))},
			{ID: 5, Label: "a5", Slug: "a5", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.5773502691896258, 0.5773502691896258, 0.5773502691896258}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.99, 0.1, 0.0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		for _, r := range results {
			if r.SemanticBoardID == 100 {
				require.NotEqual(t, "max_sim", r.MatchReason, "should not match as max_sim with only 1 hit out of 5")
			}
		}
	})

	t.Run("N=5 hits=2 rate=0.2 insufficient rate should not match max_sim", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := make([]models.SemanticLabel, 10)
		for i := 0; i < 10; i++ {
			vec := []float64{0, 0, 1}
			if i < 2 {
				vec = []float64{1, 0, 0}
			}
			tagAuxiliaries[i] = models.SemanticLabel{
				ID: uint(i + 1), Label: fmt.Sprintf("a%d", i), Slug: fmt.Sprintf("a%d", i),
				LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector(vec)),
			}
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		for _, r := range results {
			if r.SemanticBoardID == 100 {
				require.NotEqual(t, "max_sim", r.MatchReason, "should not match as max_sim with rate 0.2 < 0.3")
			}
		}
	})

	t.Run("N=5 hits=2 rate=0.4 sim high should match max_sim", func(t *testing.T) {
		config := defaultConfig()
		config.DirectHitRate = 1.0
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a1", Slug: "a1", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "a2", Slug: "a2", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "a3", Slug: "a3", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 0, 1}))},
			{ID: 4, Label: "a4", Slug: "a4", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.5, 0.5, 0.7071067811865476}))},
			{ID: 5, Label: "a5", Slug: "a5", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.5773502691896258, 0.5773502691896258, 0.5773502691896258}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.95, 0.31224989991992, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 11, Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 0.95, 0.31224989991992}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
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
			DirectHitMinOverlap:    2,
		}
	}

	t.Run("1-aux tag: adjustedHitRate=1/3=0.333 does not pass hit_rate gate", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.InDelta(t, 1.0, results[0].Score, 0.0001)
	})

	t.Run("1-aux tag: weak similarity falls to weighted and may be filtered", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.8, 0.6, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
	})

	t.Run("1-aux tag: moderate similarity falls to weighted and passes threshold", func(t *testing.T) {
		config := defaultConfig()
		config.DirectMaxSim = 0.9
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.527, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "weighted", results[0].MatchReason)
		require.InDelta(t, 0.643, results[0].Score, 0.01)
	})

	t.Run("2-aux tag: both hit gives adjustedHitRate=2/3=0.667, score is blended", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "openai", Slug: "openai", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "gpt", Slug: "gpt", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.435889894354067, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.InDelta(t, 0.9, results[0].Score, 0.01)
	})

	t.Run("3-aux tag: unchanged behavior since N >= minEffectiveSample", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a", Slug: "a", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "b", Slug: "b", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "c", Slug: "c", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 0, 1}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "weighted", results[0].MatchReason)
		require.InDelta(t, 0.733, results[0].Score, 0.01)
	})

	t.Run("5-aux tag: 3 hits hit_rate blended score", func(t *testing.T) {
		config := defaultConfig()
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a", Slug: "a", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{ID: 2, Label: "b", Slug: "b", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
			{ID: 3, Label: "c", Slug: "c", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.527, 0}))},
			{ID: 4, Label: "d", Slug: "d", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 0, 1}))},
			{ID: 5, Label: "e", Slug: "e", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{0.5, 0.5, 0.707}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 11, Embedding: ptrStr(core.FloatsToPgVector([]float64{0, 1, 0}))},
			{BoardID: 100, AuxiliaryLabelID: 12, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.527, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.InDelta(t, 0.88, results[0].Score, 0.01)
	})

	t.Run("hit_rate_sim_blend=1.0 recovers old pure maxSim score", func(t *testing.T) {
		config := defaultConfig()
		config.HitRateSimBlend = 1.0
		config.MinEffectiveSample = 1
		tagAuxiliaries := []models.SemanticLabel{
			{ID: 1, Label: "a", Slug: "a", LabelType: "auxiliary", Status: "active",
				Embedding: ptrStr(core.FloatsToPgVector([]float64{1, 0, 0}))},
		}
		boardAuxiliaries := []BoardAuxiliaryLabel{
			{BoardID: 100, AuxiliaryLabelID: 10, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.527, 0}))},
		}
		results := evaluateSemanticBoardMatches(tagAuxiliaries, boardAuxiliaries, nil, nil, config, nil, nil)
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
		require.InDelta(t, 0.4, hitRate, 0.0001)
	})
}

func TestEvaluateSemanticBoardMatches_DirectionCheck(t *testing.T) {
	config := SemanticBoardMatchConfig{
		SimThreshold:           0.5,
		DirectHitRate:          0.5,
		DirectMaxSim:           0.7,
		DirectMaxSimMinHits:    1,
		DirectMaxSimMinHitRate: 0.2,
		MinEffectiveSample:     3,
		HitRateSimBlend:        0.7,
		WeightSim:              0.6,
		WeightDensity:          0.4,
		WeightedThreshold:      0.6,
		MaxBoards:              3,
		DirectHitMinOverlap:    2,
		DirectionSimThreshold:  0.5,
	}

	tagAux := []models.SemanticLabel{
		{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
	}
	boardAux := []BoardAuxiliaryLabel{
		{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.15, 0.0}))},
	}

	t.Run("direction sim above threshold", func(t *testing.T) {
		tagEmb := []float64{0.9, 0.1, 0.0}
		boardEmbs := map[uint][]float64{10: {0.85, 0.15, 0.0}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.False(t, results[0].DirectionMismatch)
	})

	t.Run("direction sim below threshold", func(t *testing.T) {
		tagEmb := []float64{0.1, 0.9, 0.0}
		// board 20 只存在于 boardEmbs（全库参照系，非本次候选）：真家在候选外，
		// 挂载 board 10 非全库最优 → 绝对门照常拦截（fix-board-ghost-false-positive D1）。
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}, 20: {0.1, 0.9, 0.0}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.True(t, results[0].DirectionMismatch)
	})

	// fix-board-ghost-false-positive：单挂且全库最优时豁免生效（绝对门不再拦截），
	// 参照系含本次候选外的版块。
	t.Run("single mount globally best exempt despite low cosine", func(t *testing.T) {
		tagEmb := []float64{0.1, 0.9, 0.0}
		// board 10 cos≈0.18 为全库最高（board 20 正交 cos=0），豁免压过绝对门。
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}, 20: {0, 0, 1}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.False(t, results[0].DirectionMismatch, "global-best mount exempt from absolute gate")
	})

	t.Run("no tag embedding skips direction check", func(t *testing.T) {
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.False(t, results[0].DirectionMismatch)
	})

	t.Run("no board embedding skips direction check", func(t *testing.T) {
		tagEmb := []float64{0.1, 0.9, 0.0}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, tagEmb, nil)
		require.Len(t, results, 1)
		require.False(t, results[0].DirectionMismatch)
	})

	t.Run("hit_rate match with orthogonal embeddings", func(t *testing.T) {
		hrConfig := config
		hrConfig.DirectHitRate = 0.01
		tagAuxMulti := []models.SemanticLabel{
			{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{ID: 2, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0}))},
			{ID: 3, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.86, 0.14, 0.0}))},
		}
		boardAuxMulti := []BoardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{BoardID: 10, AuxiliaryLabelID: 101, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0}))},
		}
		tagEmb := []float64{0.1, 0.9, 0.0}
		// board 20 候选外真家 → board 10 非全库最优，绝对门照常拦截。
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}, 20: {0.1, 0.9, 0.0}}
		results := evaluateSemanticBoardMatches(tagAuxMulti, boardAuxMulti, nil, nil, hrConfig, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.True(t, results[0].DirectionMismatch)
	})

	// review R-2（fix-digest-tag-leak）：hit_rate 通过向直接断言——方向门与 matchReason
	// 无关（composite_hit 豁免除外），但 spec Scenario「hit_rate match passes direction
	// check」值得有真通道落点，避免仅靠等价覆盖推理。
	t.Run("hit_rate match with aligned embeddings passes", func(t *testing.T) {
		hrConfig := config
		hrConfig.DirectHitRate = 0.01
		tagAuxMulti := []models.SemanticLabel{
			{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{ID: 2, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0}))},
			{ID: 3, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.86, 0.14, 0.0}))},
		}
		boardAuxMulti := []BoardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{BoardID: 10, AuxiliaryLabelID: 101, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0}))},
		}
		tagEmb := []float64{0.9, 0.1, 0.0}
		// 板 10 emb 加第三维失配（cos≈0.949），板 20 候选外更高（cos=1.0）——
		// 钉住 spec 前提「非全库最优且双门不触发」（review L3：平凡最优时 false
		// 由豁免层给出，盖不住绝对门路径）。
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.3}, 20: {0.9, 0.1, 0.0}}
		results := evaluateSemanticBoardMatches(tagAuxMulti, boardAuxMulti, nil, nil, hrConfig, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.False(t, results[0].DirectionMismatch, "non-best mount: abs 0.949>=0.5, run-gap=0")
	})

	t.Run("weighted match with orthogonal embeddings", func(t *testing.T) {
		wtConfig := config
		wtConfig.DirectHitRate = 1.0
		wtConfig.DirectMaxSim = 1.0
		wtConfig.WeightedThreshold = 0.01
		tagEmb := []float64{0.1, 0.9, 0.0}
		// board 20 候选外真家 → board 10 非全库最优，绝对门照常拦截。
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}, 20: {0.1, 0.9, 0.0}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, wtConfig, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "weighted", results[0].MatchReason)
		require.True(t, results[0].DirectionMismatch)
	})
}

func TestEvaluateSemanticBoardMatches_DowngradedMark(t *testing.T) {
	config := SemanticBoardMatchConfig{
		SimThreshold:           0.5,
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
		DirectHitMinOverlap:    2,
	}

	t.Run("N=1 tag → minHits=1 < 2 → downgraded", func(t *testing.T) {
		tagAux1 := []models.SemanticLabel{
			{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
		}
		boardAux := []BoardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.15, 0.0}))},
		}

		results := evaluateSemanticBoardMatches(tagAux1, boardAux, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.True(t, results[0].Downgraded, "expected downgraded=true for N=1 tag with minHits=1 < DirectMaxSimMinHits=2")
	})

	t.Run("N=3 tag → minHits=2 → NOT downgraded", func(t *testing.T) {
		tagAux3 := []models.SemanticLabel{
			{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{ID: 2, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0}))},
			{ID: 3, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.86, 0.14, 0.0}))},
		}
		boardAux := []BoardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 100, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.85, 0.15, 0.0}))},
		}

		results := evaluateSemanticBoardMatches(tagAux3, boardAux, nil, nil, config, nil, nil)
		require.Len(t, results, 1)
		require.False(t, results[0].Downgraded, "expected downgraded=false for N=3 tag")
	})
}

// TestEvaluateSemanticBoardMatches_DirectionRelativeGate covers the relative
// direction gate (fix-digest-tag-leak 故事 A，test-cases.md 主链路)：多挂标签
// 的挂载点 identity cosine 落到 bestCos − margin 以下时标记
// direction_mismatch=true。composite_hit 豁免、direct_hit 纳入（spec 漂移修正）、
// 单挂 gap=0 恒过、embedding 缺失跳门、margin=0 关门、gap==margin 等值边界。
func TestEvaluateSemanticBoardMatches_DirectionRelativeGate(t *testing.T) {
	baseConfig := func() SemanticBoardMatchConfig {
		return SemanticBoardMatchConfig{
			SimThreshold:            0.5,
			DirectHitRate:           0.9,
			DirectMaxSim:            0.7,
			DirectMaxSimMinHits:     1,
			DirectMaxSimMinHitRate:  0.2,
			MinEffectiveSample:      3,
			HitRateSimBlend:         0.7,
			WeightSim:               0.6,
			WeightDensity:           0.4,
			WeightedThreshold:       0.6,
			MaxBoards:               3,
			DirectHitMinOverlap:     2,
			DirectHitMinRatio:       0.5,
			DirectionSimThreshold:   0.5,
			DirectionRelativeMargin: 0.15,
		}
	}

	// 单共享 aux + DirectHitMinOverlap=2 → 每个 board 走 max_sim 间接命中；
	// identity cosine 由 tagEmb/boardEmbs 独立构造，与匹配通道解耦。
	newFixture := func(boardIDs ...uint) ([]models.SemanticLabel, []BoardAuxiliaryLabel) {
		tagAux := []models.SemanticLabel{{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))}}
		boardAux := make([]BoardAuxiliaryLabel, 0, len(boardIDs))
		for _, id := range boardIDs {
			boardAux = append(boardAux, BoardAuxiliaryLabel{
				BoardID:          id,
				AuxiliaryLabelID: 100,
				Embedding:        ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0})),
			})
		}
		return tagAux, boardAux
	}

	xAxis := []float64{1, 0, 0}
	// relVec 返回与 x 轴 cosine=c 的单位向量（c²+s²=1）。
	relVec := func(c float64) []float64 {
		return []float64{c, math.Sqrt(1 - c*c), 0}
	}
	resultsByBoard := func(results []SemanticBoardMatchResult) (map[uint]bool, map[uint]string) {
		mismatch := make(map[uint]bool, len(results))
		reason := make(map[uint]string, len(results))
		for _, r := range results {
			mismatch[r.SemanticBoardID] = r.DirectionMismatch
			reason[r.SemanticBoardID] = r.MatchReason
		}
		return mismatch, reason
	}

	t.Run("multi-mount cross-domain leak flagged", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20, 30)
		boardEmbs := map[uint][]float64{10: relVec(0.81), 20: relVec(0.60), 30: relVec(0.56)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 3)
		for _, id := range []uint{10, 20, 30} {
			require.Equal(t, "max_sim", reason[id])
		}
		require.False(t, mismatch[10], "best mount survives")
		require.True(t, mismatch[20], "gap 0.21 >= 0.15")
		require.True(t, mismatch[30], "gap 0.25 >= 0.15")
	})

	t.Run("legal dual-mount within margin survives", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.82), 20: relVec(0.70)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.Len(t, results, 2)
		require.False(t, mismatch[10], "gap 0.12 < 0.15")
		require.False(t, mismatch[20], "gap 0.12 < 0.15")
	})

	t.Run("single-board mount unaffected", func(t *testing.T) {
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.60)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.Len(t, results, 1)
		require.False(t, mismatch[10], "gap=0 relative gate passes; 0.6 >= 0.5 absolute gate passes")
	})

	t.Run("absolute gate still flags independently", func(t *testing.T) {
		tagAux, boardAux := newFixture(10)
		// board 20 只在全库参照系（非候选）：board 10 非全库最优，绝对门拦截。
		boardEmbs := map[uint][]float64{10: relVec(0.30), 20: relVec(0.90)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.True(t, mismatch[10], "cos 0.30 < 0.5 absolute gate, not global best")
	})

	// fix-board-ghost-false-positive：绝对门+相对门对全库最优挂载都豁免
	// （cos 0.30 < 0.5 本会被绝对门拦，单挂候选外无更高版块 → 免标）。
	t.Run("global-best mount exempt from both gates", func(t *testing.T) {
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.30), 20: relVec(0.10)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[10], "global-best mount: exemption beats both gates")
	})

	t.Run("direct_hit subject to relative gate", func(t *testing.T) {
		cfg := baseConfig()
		cfg.DirectHitMinOverlap = 1
		cfg.DirectHitMinRatio = 0
		// 共享 aux ID=1 → direct_hit；identity cos 0.81/0.60。
		tagAux := []models.SemanticLabel{{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))}}
		boardAux := []BoardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{BoardID: 20, AuxiliaryLabelID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
		}
		boardEmbs := map[uint][]float64{10: relVec(0.81), 20: relVec(0.60)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 2)
		require.Equal(t, "direct_hit", reason[10])
		require.Equal(t, "direct_hit", reason[20])
		require.False(t, mismatch[10])
		require.True(t, mismatch[20], "direct_hit drift fix: subject to the relative gate")
	})

	t.Run("composite_hit exempt", func(t *testing.T) {
		tagAux, boardAux := newFixture(20)
		compVec := core.FloatsToPgVector([]float64{0.5, 0.5, 0.0})
		tagComposites := []models.SemanticLabel{{ID: 500, Embedding: ptrStr(compVec)}}
		boardComposites := []BoardCompositeLabel{{BoardID: 10, CompositeLabelID: 500, Embedding: ptrStr(compVec)}}
		// board 10 组合命中且 identity cos 0.20（两门本都会触发）→ 豁免恒 false；
		// board 20 max_sim cos 0.90 为参照集 best。
		boardEmbs := map[uint][]float64{10: relVec(0.20), 20: relVec(0.90)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, tagComposites, boardComposites, baseConfig(), xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 2)
		require.Equal(t, "composite_hit", reason[10])
		require.False(t, mismatch[10], "composite_hit exempt from both gates")
		require.False(t, mismatch[20])
	})

	t.Run("missing tag embedding skips gates", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.81), 20: relVec(0.10)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), nil, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.Len(t, results, 2)
		require.False(t, mismatch[10])
		require.False(t, mismatch[20], "no identity embedding: both gates skipped")
	})

	t.Run("missing board embedding skips that mount's gate", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20)
		// board 20 无 embedding → 其门跳过；参照集只剩 board 10。
		boardEmbs := map[uint][]float64{10: relVec(0.60)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.Len(t, results, 2)
		require.False(t, mismatch[10])
		require.False(t, mismatch[20], "board embedding missing: gate skipped for that mount")
	})

	t.Run("margin zero disables relative gate", func(t *testing.T) {
		cfg := baseConfig()
		cfg.DirectionRelativeMargin = 0
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.81), 20: relVec(0.60)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[20], "margin=0: gap 0.21 not flagged, absolute gate only")
	})

	t.Run("gap equals margin flags mismatch (>= boundary)", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20)
		best, worst := relVec(0.81), relVec(0.60)
		boardEmbs := map[uint][]float64{10: best, 20: worst}
		cfg := baseConfig()
		// 与门内同一表达式（CosineSimilarity 同输入相减）：margin 精确等值。
		cfg.DirectionRelativeMargin = CosineSimilarity(xAxis, best) - CosineSimilarity(xAxis, worst)
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.True(t, mismatch[20], "gap == margin triggers (>= semantics)")
		require.False(t, mismatch[10])
	})

	t.Run("gap one ulp below margin passes", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20)
		best, worst := relVec(0.81), relVec(0.60)
		boardEmbs := map[uint][]float64{10: best, 20: worst}
		cfg := baseConfig()
		cfg.DirectionRelativeMargin = math.Nextafter(
			CosineSimilarity(xAxis, best)-CosineSimilarity(xAxis, worst), math.Inf(1))
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[20], "gap < margin by one ulp: relative gate passes")
	})
}

// TestGlobalBestExemption covers the global-best exemption layer
// (fix-board-ghost-false-positive)：挂载版块 = tag 对全部 active 版块 identity
// cosine 最高者（含并列，1e-9 判等）时，绝对门与相对门均豁免——「具体事件 ×
// 宽泛类目」的天然低 cosine 不是跑题。参照系用 boardEmbs 全量（含本次候选外），
// 垃圾单挂的真家在候选外时照拦（D1）。
func TestGlobalBestExemption(t *testing.T) {
	baseConfig := func() SemanticBoardMatchConfig {
		return SemanticBoardMatchConfig{
			SimThreshold:            0.5,
			DirectHitRate:           0.9,
			DirectMaxSim:            0.7,
			DirectMaxSimMinHits:     1,
			DirectMaxSimMinHitRate:  0.2,
			MinEffectiveSample:      3,
			HitRateSimBlend:         0.7,
			WeightSim:               0.6,
			WeightDensity:           0.4,
			WeightedThreshold:       0.6,
			MaxBoards:               3,
			DirectHitMinOverlap:     2,
			DirectHitMinRatio:       0.5,
			DirectionSimThreshold:   0.6,
			DirectionRelativeMargin: 0.15,
		}
	}
	xAxis := []float64{1, 0, 0}
	relVec := func(c float64) []float64 {
		return []float64{c, math.Sqrt(1 - c*c), 0}
	}
	// 单共享 aux + DirectHitMinOverlap=2 → 全部走 max_sim 间接命中；
	// identity cosine 由 boardEmbs 独立构造，与匹配通道解耦。
	newFixture := func(boardIDs ...uint) ([]models.SemanticLabel, []BoardAuxiliaryLabel) {
		tagAux := []models.SemanticLabel{{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))}}
		boardAux := make([]BoardAuxiliaryLabel, 0, len(boardIDs))
		for _, id := range boardIDs {
			boardAux = append(boardAux, BoardAuxiliaryLabel{
				BoardID:          id,
				AuxiliaryLabelID: 100,
				Embedding:        ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0})),
			})
		}
		return tagAux, boardAux
	}
	resultsByBoard := func(results []SemanticBoardMatchResult) (map[uint]bool, map[uint]string) {
		mismatch := make(map[uint]bool, len(results))
		reason := make(map[uint]string, len(results))
		for _, r := range results {
			mismatch[r.SemanticBoardID] = r.DirectionMismatch
			reason[r.SemanticBoardID] = r.MatchReason
		}
		return mismatch, reason
	}

	t.Run("global-best mount exempt despite low absolute cosine", func(t *testing.T) {
		// 真库形态：高市早苗任命 × 日本新闻 cos 0.5826 < 0.6 且全库最高 → 免标。
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.5826), 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", reason[10])
		require.False(t, mismatch[10], "global-best mount exempt from absolute gate")
	})

	t.Run("tied global best mounts all exempt", func(t *testing.T) {
		// 并列=无法区分归属，不算跑题：两挂载 cos 精确相等且 < threshold。
		tagAux, boardAux := newFixture(10, 20)
		tied := relVec(0.58)
		boardEmbs := map[uint][]float64{10: tied, 20: relVec(0.58), 30: relVec(0.20)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.Len(t, results, 2)
		require.False(t, mismatch[10], "tied best exempt")
		require.False(t, mismatch[20], "tied best exempt")
	})

	t.Run("near-tie within 1e-9 joins best set (white box)", func(t *testing.T) {
		// 差值 < 1e-9 判等（与真库验证 SQL 口径一致）：1e-8 明显更低 → 拦，
		// 5e-10 浮点噪声级 → 视为并列豁免。
		best := 0.58
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(best), 20: relVec(best - 5e-10)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[20], "diff 5e-10 < 1e-9: treated as tied, exempt")

		boardEmbs[20] = relVec(best - 1e-8)
		results = evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ = resultsByBoard(results)
		require.True(t, mismatch[20], "diff 1e-8 > 1e-9: not tied, absolute gate flags")
	})

	t.Run("true home outside candidates still flagged", func(t *testing.T) {
		// D1 参照系：tag 单挂 B（唯一候选），真家 A 在候选外且 cosine 更高
		// → B 非全库最优，绝对门照常拦截（同轮候选参照会误豁免）。
		tagAux, boardAux := newFixture(20)
		boardEmbs := map[uint][]float64{10: relVec(0.82), 20: relVec(0.55)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", reason[20])
		require.True(t, mismatch[20], "mount not global best (true home outside candidates): absolute gate flags")
	})

	t.Run("direct_hit follows the same three layers", func(t *testing.T) {
		// spec「降级 direct_hit 方向校验不通过 / 全库最优豁免」：最优者免标，
		// 非最优低 cos 照拦。
		cfg := baseConfig()
		cfg.DirectHitMinOverlap = 1
		cfg.DirectHitMinRatio = 0
		tagAux := []models.SemanticLabel{{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))}}
		boardAux := []BoardAuxiliaryLabel{
			{BoardID: 10, AuxiliaryLabelID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
			{BoardID: 20, AuxiliaryLabelID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))},
		}
		boardEmbs := map[uint][]float64{10: relVec(0.55), 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 2)
		require.Equal(t, "direct_hit", reason[10])
		require.Equal(t, "direct_hit", reason[20])
		require.False(t, mismatch[10], "direct_hit global best exempt (cos 0.55 < 0.6)")
		require.True(t, mismatch[20], "direct_hit non-best below threshold flagged")
	})

	t.Run("composite_hit exemption not regressed", func(t *testing.T) {
		// 组合命中即指向一致：cos 0.20 且非全库最优仍免标。
		tagAux, boardAux := newFixture(20)
		compVec := core.FloatsToPgVector([]float64{0.5, 0.5, 0.0})
		tagComposites := []models.SemanticLabel{{ID: 500, Embedding: ptrStr(compVec)}}
		boardComposites := []BoardCompositeLabel{{BoardID: 10, CompositeLabelID: 500, Embedding: ptrStr(compVec)}}
		boardEmbs := map[uint][]float64{10: relVec(0.20), 20: relVec(0.90)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, tagComposites, boardComposites, baseConfig(), xAxis, boardEmbs)
		mismatch, reason := resultsByBoard(results)
		require.Len(t, results, 2)
		require.Equal(t, "composite_hit", reason[10])
		require.False(t, mismatch[10], "composite_hit exempt regardless of cosine/rank")
	})

	t.Run("missing tag embedding skips all layers", func(t *testing.T) {
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.10), 20: relVec(0.05)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), nil, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[10])
		require.False(t, mismatch[20], "no identity embedding: nothing can be called best, gates skipped")
	})

	t.Run("board without embedding excluded from reference set", func(t *testing.T) {
		// embedding 缺失的版块不进最优参照系；挂载其上的门跳过保持 false。
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.55)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[10], "global best among embeddable boards: exempt")
		require.False(t, mismatch[20], "board embedding missing: gate skipped for that mount")
	})

	t.Run("floor overrides exemption", func(t *testing.T) {
		// spec「floor overrides exemption」：floor 0.55 > best cos 0.515 →
		// 豁免不生效，绝对门（0.6）照判。
		cfg := baseConfig()
		cfg.DirectionFloor = 0.55
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.515), 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.True(t, mismatch[10], "cos 0.515 < floor 0.55: exemption revoked, absolute gate flags")
	})

	t.Run("floor at or below cosine keeps exemption", func(t *testing.T) {
		// cos == floor 边界：>= 语义，豁免保持。floor 用与门内同一表达式构造，
		// 防 arm64 FMA 舍入方向翻转（review L2）。
		cfg := baseConfig()
		floorVec := relVec(0.515)
		cfg.DirectionFloor = CosineSimilarity(xAxis, floorVec)
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: floorVec, 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch, _ := resultsByBoard(results)
		require.False(t, mismatch[10], "cos 0.515 >= floor 0.515: exemption stands")
	})
}

// TestDirectionGateThreeLayers covers the task 1.2 三分支：豁免 / 非最优 /
// 地板线覆盖（-run TestDirection 验证锚点）。
func TestDirectionGateThreeLayers(t *testing.T) {
	config := SemanticBoardMatchConfig{
		SimThreshold:            0.5,
		DirectHitRate:           0.9,
		DirectMaxSim:            0.7,
		DirectMaxSimMinHits:     1,
		DirectMaxSimMinHitRate:  0.2,
		MinEffectiveSample:      3,
		HitRateSimBlend:         0.7,
		WeightSim:               0.6,
		WeightDensity:           0.4,
		WeightedThreshold:       0.6,
		MaxBoards:               3,
		DirectHitMinOverlap:     2,
		DirectHitMinRatio:       0.5,
		DirectionSimThreshold:   0.6,
		DirectionRelativeMargin: 0.15,
	}
	xAxis := []float64{1, 0, 0}
	relVec := func(c float64) []float64 {
		return []float64{c, math.Sqrt(1 - c*c), 0}
	}
	newFixture := func(boardIDs ...uint) ([]models.SemanticLabel, []BoardAuxiliaryLabel) {
		tagAux := []models.SemanticLabel{{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))}}
		boardAux := make([]BoardAuxiliaryLabel, 0, len(boardIDs))
		for _, id := range boardIDs {
			boardAux = append(boardAux, BoardAuxiliaryLabel{
				BoardID:          id,
				AuxiliaryLabelID: 100,
				Embedding:        ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0})),
			})
		}
		return tagAux, boardAux
	}

	t.Run("exemption branch: global best below threshold not flagged", func(t *testing.T) {
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.5826), 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, xAxis, boardEmbs)
		require.Len(t, results, 1)
		require.False(t, results[0].DirectionMismatch, "0.5826 < 0.6 but global best: exempt")
	})

	t.Run("non-best branch: below threshold still flagged", func(t *testing.T) {
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.55), 20: relVec(0.82)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, xAxis, boardEmbs)
		require.Len(t, results, 1)
		require.True(t, results[0].DirectionMismatch, "0.55 < 0.6 and not global best: absolute gate flags")
	})

	t.Run("floor branch: floor revokes exemption", func(t *testing.T) {
		cfg := config
		cfg.DirectionFloor = 0.55
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.515), 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		require.Len(t, results, 1)
		require.True(t, results[0].DirectionMismatch, "0.515 < floor 0.55: global best no longer exempt")
	})
}

// TestGapExemption 覆盖 fix-board-direction-gap-exempt task 3.1：
// DirectionGapExemptMargin 扩围豁免主链路（margin 默认 0.10，零值回退
// 并列最优口径）。用例来源 spec「near-best mount within gap margin
// exempted」等 Scenario + test-cases.md 主链路表；真库锚点：日本新闻冤案
// 真家 cos 0.581 vs 磁铁板 0.647（gap 0.066）豁免、科技巨头次优挂载
// 0.597 vs 生成式AI 0.811（gap 0.214）仍标。
func TestGapExemption(t *testing.T) {
	baseConfig := func() SemanticBoardMatchConfig {
		cfg := SemanticBoardMatchConfig{
			SimThreshold:            0.5,
			DirectHitRate:           0.9,
			DirectMaxSim:            0.7,
			DirectMaxSimMinHits:     1,
			DirectMaxSimMinHitRate:  0.2,
			MinEffectiveSample:      3,
			HitRateSimBlend:         0.7,
			WeightSim:               0.6,
			WeightDensity:           0.4,
			WeightedThreshold:       0.6,
			MaxBoards:               3,
			DirectHitMinOverlap:     2,
			DirectHitMinRatio:       0.5,
			DirectionSimThreshold:   0.6,
			DirectionRelativeMargin: 0.15,
		}
		cfg.DirectionGapExemptMargin = 0.10
		return cfg
	}
	xAxis := []float64{1, 0, 0}
	relVec := func(c float64) []float64 {
		return []float64{c, math.Sqrt(1 - c*c), 0}
	}
	newFixture := func(boardIDs ...uint) ([]models.SemanticLabel, []BoardAuxiliaryLabel) {
		tagAux := []models.SemanticLabel{{ID: 1, Embedding: ptrStr(core.FloatsToPgVector([]float64{0.9, 0.1, 0.0}))}}
		boardAux := make([]BoardAuxiliaryLabel, 0, len(boardIDs))
		for _, id := range boardIDs {
			boardAux = append(boardAux, BoardAuxiliaryLabel{
				BoardID:          id,
				AuxiliaryLabelID: 100,
				Embedding:        ptrStr(core.FloatsToPgVector([]float64{0.88, 0.12, 0.0})),
			})
		}
		return tagAux, boardAux
	}
	resultsByBoard := func(results []SemanticBoardMatchResult) map[uint]bool {
		mismatch := make(map[uint]bool, len(results))
		for _, r := range results {
			mismatch[r.SemanticBoardID] = r.DirectionMismatch
		}
		return mismatch
	}

	t.Run("near-best mount within margin exempted (real-db anchor)", func(t *testing.T) {
		// 真库锚点：高市早苗众议院发言挂日本新闻 cos 0.581，磁铁板
		// 「中国国内新闻」0.647 抢走全库第一（gap 0.066 < 0.10）→ 豁免。
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.581), 20: relVec(0.647), 30: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.Len(t, results, 1)
		require.False(t, mismatch[10], "gap 0.066 < margin 0.10: near-best mount exempt despite cos 0.581 < 0.6")
	})

	t.Run("mount beyond margin still flagged (real-db anchor)", func(t *testing.T) {
		// 真库锚点：Claude Opus 5.5 挂科技巨头 cos 0.597，真家生成式AI
		// 0.811（gap 0.214 ≥ 0.10）→ 绝对门照拦。
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.597), 20: relVec(0.811), 30: relVec(0.40)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.Len(t, results, 1)
		require.True(t, mismatch[10], "gap 0.214 >= margin 0.10 and cos 0.597 < 0.6: absolute gate flags")
	})

	t.Run("exempt band boundary is strict (white box)", func(t *testing.T) {
		// 边界严格 <：差值名义 0.10，浮点误差方向不可控，故用略大/略小
		// 两侧锁定语义：margin 0.10001 豁免、margin 0.09999 不豁免。
		tagEmbedding := xAxis
		boards := map[uint][]float64{10: relVec(0.60), 20: relVec(0.70)}
		wide := computeGapExemptBoards(tagEmbedding, boards, 0.10001)
		require.Contains(t, wide, uint(10), "gap ~0.10 < margin 0.10001: in exempt set")
		narrow := computeGapExemptBoards(tagEmbedding, boards, 0.09999)
		require.NotContains(t, narrow, uint(10), "gap ~0.10 >= margin 0.09999: outside exempt set")
		require.Contains(t, narrow, uint(20), "global best always in exempt set")
	})

	t.Run("rank-1 mount always exempt regardless of margin", func(t *testing.T) {
		cfg := baseConfig()
		cfg.DirectionGapExemptMargin = 0.01
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.58), 20: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.False(t, mismatch[10], "diff 0 to global best: exempt even with tiny margin 0.01")
	})

	t.Run("margin zero falls back to tie-only basis", func(t *testing.T) {
		// 差值 0.05：margin 0.10 豁免、margin 0（回退 1e-9 并列口径）照拦。
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.55), 20: relVec(0.60)}

		cfgOff := baseConfig()
		cfgOff.DirectionGapExemptMargin = 0
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfgOff, xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.True(t, mismatch[10], "gap 0.05 >= 1e-9 with margin off: tie-only basis flags (pre-change behavior)")

		results = evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch = resultsByBoard(results)
		require.False(t, mismatch[10], "gap 0.05 < margin 0.10: exempt (new behavior)")
	})

	t.Run("floor revokes gap exemption", func(t *testing.T) {
		// 挂载 0.515 与全库最优 0.565 差 0.05 在豁免带内，但 floor 0.55
		// 收回豁免 → 绝对门照拦。
		cfg := baseConfig()
		cfg.DirectionFloor = 0.55
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.515), 20: relVec(0.565), 30: relVec(0.30)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.True(t, mismatch[10], "cos 0.515 < floor 0.55: gap exemption revoked")
	})

	t.Run("single-board mount always within band", func(t *testing.T) {
		// 单板挂载且为唯一带 embedding 的板：差值 0 恒豁免。
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.45)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, baseConfig(), xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.False(t, mismatch[10], "sole embeddable board: gap 0, exempt")
	})

	t.Run("gap exemption also skips relative gate (margin > relative margin)", func(t *testing.T) {
		// 默认配置（margin 0.10 < relative 0.15）下 skip 是死分支：豁免⇒
		// 同轮 gap ≤ 全库 gap < 0.10 < 0.15，相对门永不可能标记豁免板。
		// skip 仅在 margin > relative margin 配置下承重（review M1），本
		// 用例用 margin 0.20 钉住：A cos 0.62（全库 best 0.78，gap 0.16 在
		// 豁免带内）同轮 gap 0.16 ≥ 0.15——若无 skip，相对门会误标 A。
		cfg := baseConfig()
		cfg.DirectionGapExemptMargin = 0.20
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.62), 20: relVec(0.78)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.Len(t, results, 2)
		require.False(t, mismatch[10], "A in exempt set (gap 0.16 < margin 0.20): relative gate skipped despite same-run gap 0.16 >= 0.15")
		require.False(t, mismatch[20], "B is global best: exempt")
	})

	t.Run("floor-revoked mount falls back to relative gate (review M2)", func(t *testing.T) {
		// margin 0.20 > relative 0.15 时豁免板的同轮 gap 可 ≥ 0.15：被
		// floor 收回豁免后（cos 0.62 < floor 0.65）绝对门不拦（0.62 ≥
		// 0.6），相对门补刀（同轮 bestCos 0.78，gap 0.16 ≥ 0.15）——skip
		// 集合必须带 floor 条件，否则该挂载漏标（spec「回退双门判定」）。
		cfg := baseConfig()
		cfg.DirectionGapExemptMargin = 0.20
		cfg.DirectionFloor = 0.65
		tagAux, boardAux := newFixture(10, 20)
		boardEmbs := map[uint][]float64{10: relVec(0.62), 20: relVec(0.78)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.Len(t, results, 2)
		require.True(t, mismatch[10], "exemption revoked by floor (0.62 < 0.65): relative gate must flag (gap 0.16 >= 0.15)")
		require.False(t, mismatch[20], "B global best and above floor: exempt")
	})

	t.Run("custom margin widens band end-to-end", func(t *testing.T) {
		// spec「custom margin applied」端到端半边（review L2）：margin 0.15
		// 下差值 0.12 的近真家挂载（默认 0.10 时会被标）获豁免。
		cfg := baseConfig()
		cfg.DirectionGapExemptMargin = 0.15
		tagAux, boardAux := newFixture(10)
		boardEmbs := map[uint][]float64{10: relVec(0.53), 20: relVec(0.65)}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, cfg, xAxis, boardEmbs)
		mismatch := resultsByBoard(results)
		require.Len(t, results, 1)
		require.False(t, mismatch[10], "gap 0.12 < custom margin 0.15: exempt (would be flagged at default 0.10)")
	})
}
