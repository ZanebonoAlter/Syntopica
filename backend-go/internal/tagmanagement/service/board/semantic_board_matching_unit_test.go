package board

import (
	"fmt"
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
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}}
		results := evaluateSemanticBoardMatches(tagAux, boardAux, nil, nil, config, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "max_sim", results[0].MatchReason)
		require.True(t, results[0].DirectionMismatch)
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
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}}
		results := evaluateSemanticBoardMatches(tagAuxMulti, boardAuxMulti, nil, nil, hrConfig, tagEmb, boardEmbs)
		require.Len(t, results, 1)
		require.Equal(t, "hit_rate", results[0].MatchReason)
		require.True(t, results[0].DirectionMismatch)
	})

	t.Run("weighted match with orthogonal embeddings", func(t *testing.T) {
		wtConfig := config
		wtConfig.DirectHitRate = 1.0
		wtConfig.DirectMaxSim = 1.0
		wtConfig.WeightedThreshold = 0.01
		tagEmb := []float64{0.1, 0.9, 0.0}
		boardEmbs := map[uint][]float64{10: {0.9, 0.1, 0.0}}
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
