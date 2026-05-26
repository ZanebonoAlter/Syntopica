package tagging

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/domain/models"
)

func TestComputeMatchDetailPairsAndMetrics(t *testing.T) {
	config := SemanticBoardMatchConfig{SimThreshold: 0.72, MinEffectiveSample: 3}
	tagAuxiliaries := []models.SemanticLabel{
		{ID: 1, Label: "Tag A", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		{ID: 2, Label: "Tag B", Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
		{ID: 3, Label: "Tag C", Embedding: ptrStr(floatsToPgVector([]float64{0, 0, 1}))},
	}
	boardAuxiliaries := []boardAuxiliaryLabel{
		{BoardID: 100, AuxiliaryLabelID: 10, Label: "Board A", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
		{BoardID: 100, AuxiliaryLabelID: 11, Label: "Board B", Embedding: ptrStr(floatsToPgVector([]float64{0, 1, 0}))},
		{BoardID: 100, AuxiliaryLabelID: 12, Label: "Board Fallback", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
	}

	detail := computeMatchDetail(tagAuxiliaries, boardAuxiliaries, config)

	require.Equal(t, 2, detail.Hits)
	require.InDelta(t, 2.0/3.0, detail.HitRate, 0.0001)
	require.InDelta(t, 1.0, detail.MaxSimilarity, 0.0001)
	require.Len(t, detail.Pairs, 3)

	require.Equal(t, uint(1), detail.Pairs[0].TagAuxiliaryID)
	require.Equal(t, "Tag A", detail.Pairs[0].TagAuxiliaryLabel)
	require.Equal(t, uint(10), detail.Pairs[0].BoardAuxiliaryID)
	require.Equal(t, "Board A", detail.Pairs[0].BoardAuxiliaryLabel)
	require.InDelta(t, 1.0, detail.Pairs[0].Similarity, 0.0001)
	require.True(t, detail.Pairs[0].IsHit)

	require.Equal(t, uint(2), detail.Pairs[1].TagAuxiliaryID)
	require.Equal(t, "Tag B", detail.Pairs[1].TagAuxiliaryLabel)
	require.Equal(t, uint(11), detail.Pairs[1].BoardAuxiliaryID)
	require.Equal(t, "Board B", detail.Pairs[1].BoardAuxiliaryLabel)
	require.InDelta(t, 1.0, detail.Pairs[1].Similarity, 0.0001)
	require.True(t, detail.Pairs[1].IsHit)

	require.Equal(t, uint(3), detail.Pairs[2].TagAuxiliaryID)
	require.Equal(t, "Tag C", detail.Pairs[2].TagAuxiliaryLabel)
	require.Equal(t, uint(10), detail.Pairs[2].BoardAuxiliaryID)
	require.Equal(t, "Board A", detail.Pairs[2].BoardAuxiliaryLabel)
	require.InDelta(t, 0.0, detail.Pairs[2].Similarity, 0.0001)
	require.False(t, detail.Pairs[2].IsHit)
}

func TestComputeMatchDetailUsesEffectiveSampleDenominator(t *testing.T) {
	config := SemanticBoardMatchConfig{SimThreshold: 0.72, MinEffectiveSample: 3}
	tagAuxiliaries := []models.SemanticLabel{
		{ID: 1, Label: "Tag A", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
	}
	boardAuxiliaries := []boardAuxiliaryLabel{
		{BoardID: 100, AuxiliaryLabelID: 10, Label: "Board A", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))},
	}

	detail := computeMatchDetail(tagAuxiliaries, boardAuxiliaries, config)

	require.Equal(t, 1, detail.Hits)
	require.InDelta(t, 1.0/3.0, detail.HitRate, 0.0001)
}

func TestComputeMatchDetailEmptyInputs(t *testing.T) {
	config := SemanticBoardMatchConfig{SimThreshold: 0.72, MinEffectiveSample: 3}

	detail := computeMatchDetail(nil, []boardAuxiliaryLabel{{BoardID: 100, AuxiliaryLabelID: 10, Label: "Board A", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))}}, config)
	require.Zero(t, detail.Hits)
	require.Zero(t, detail.HitRate)
	require.Zero(t, detail.MaxSimilarity)
	require.Empty(t, detail.Pairs)

	detail = computeMatchDetail([]models.SemanticLabel{{ID: 1, Label: "Tag A", Embedding: ptrStr(floatsToPgVector([]float64{1, 0, 0}))}}, nil, config)
	require.Zero(t, detail.Hits)
	require.Zero(t, detail.HitRate)
	require.Zero(t, detail.MaxSimilarity)
	require.Empty(t, detail.Pairs)
}

func TestDirectHitAuxiliaryDetection(t *testing.T) {
	tagAuxiliaries := []models.SemanticLabel{
		{ID: 1, Label: "Tag A"},
		{ID: 2, Label: "Tag B"},
	}
	boardAuxiliaries := []boardAuxiliaryLabel{
		{BoardID: 100, AuxiliaryLabelID: 2, Label: "Board B"},
		{BoardID: 100, AuxiliaryLabelID: 3, Label: "Board C"},
	}

	directHits := buildDirectHitAuxiliaryDTOs(tagAuxiliaries, boardAuxiliaries)

	require.Equal(t, []directHitAuxiliaryDTO{{
		TagAuxiliaryID:   2,
		TagLabel:         "Tag B",
		BoardAuxiliaryID: 2,
		BoardLabel:       "Board B",
	}}, directHits)
}
