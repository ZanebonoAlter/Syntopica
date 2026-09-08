package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// resolverTestSearcher returns fixed hits per concept (nil → empty).
type resolverTestSearcher struct {
	hits []InternalContextHit
}

func (r *resolverTestSearcher) SearchInternalContext(_ context.Context, _ string, _ int) ([]InternalContextHit, error) {
	return r.hits, nil
}

func resolverLaneHit(board, lane uint, label, summary string) InternalContextHit {
	id := lane
	return InternalContextHit{Kind: "lane", BoardID: board, LaneID: &id, Label: label, Summary: summary}
}

const (
	resolverTestThreshold = 0.62
	resolverTestMargin    = 0.08
)

func resolverTestInput(hits []InternalContextHit) ResolveTargetInput {
	return ResolveTargetInput{
		Concept:   "日债收益率",
		Searcher:  &resolverTestSearcher{hits: hits},
		Threshold: resolverTestThreshold,
		Margin:    resolverTestMargin,
		TopK:      5,
	}
}

func TestResolveTarget_NoMatch(t *testing.T) {
	// Empty index: no candidates at all.
	res, err := ResolveTarget(context.Background(), resolverTestInput(nil))
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeNoMatch, res.Outcome)
	require.Nil(t, res.Best)

	// Candidates exist but lexical scores are all below threshold.
	res2, err := ResolveTarget(context.Background(), resolverTestInput([]InternalContextHit{
		resolverLaneHit(1, 10, "卡塔尔航空", "航班动态"),
	}))
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeNoMatch, res2.Outcome, "zero lexical score must stay no_match")
}

func TestResolveTarget_AmbiguousMargin(t *testing.T) {
	// Two lanes both fully contain the concept → 1.0 vs 1.0, margin 0 → ambiguous.
	res, err := ResolveTarget(context.Background(), resolverTestInput([]InternalContextHit{
		resolverLaneHit(1, 10, "日债收益率走高", ""),
		resolverLaneHit(2, 20, "日债收益率下行", ""),
	}))
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeAmbiguous, res.Outcome)
	require.Nil(t, res.Best, "ambiguous must not bind top-1")
	require.Len(t, res.Candidates, 2)
}

func TestResolveTarget_Resolved(t *testing.T) {
	res, err := ResolveTarget(context.Background(), resolverTestInput([]InternalContextHit{
		resolverLaneHit(5, 77, "日债收益率", "日本国债收益率动态"),
		resolverLaneHit(9, 88, "中东局势", "油价与避险"),
	}))
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeResolved, res.Outcome)
	require.NotNil(t, res.Best)
	require.Equal(t, uint(5), res.Best.BoardID)
	require.NotNil(t, res.Best.LaneID)
	require.Equal(t, uint(77), *res.Best.LaneID)
	require.Equal(t, "lexical", res.ScoreKind)
	require.InDelta(t, 1.0, res.Best.Score, 1e-9)
	require.Equal(t, relationResolverVersion, res.ResolverVersion)
}

func TestResolveTarget_ScoreBoundary(t *testing.T) {
	// threshold-ε (label partial 0.4 + summary 0.6 = 0.6 < 0.62) → no_match.
	mk := func(label string) ResolveTargetInput {
		return ResolveTargetInput{
			Concept: "日债收益率", Searcher: &resolverTestSearcher{hits: []InternalContextHit{
				resolverLaneHit(1, 10, label, ""),
			}}, Threshold: resolverTestThreshold, Margin: resolverTestMargin,
		}
	}
	// Summary containment alone (0.6) is below 0.62 → no_match.
	res, err := ResolveTarget(context.Background(), ResolveTargetInput{
		Concept: "日债收益率", Searcher: &resolverTestSearcher{hits: []InternalContextHit{
			resolverLaneHit(1, 10, "全球债券市场", "日债收益率走高引发关注"),
		}}, Threshold: resolverTestThreshold, Margin: resolverTestMargin,
	})
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeNoMatch, res.Outcome)

	// Label containment (1.0) clears the threshold → resolved (single candidate).
	res2, err := ResolveTarget(context.Background(), mk("日债收益率创新高"))
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeResolved, res2.Outcome)
}

func TestResolveTarget_MarginBoundary(t *testing.T) {
	// cosine mode: top1 0.9, top2 0.82 → gap 0.08 == margin → resolved
	// (gap must MEET the margin, i.e. >= margin).
	in := ResolveTargetInput{
		Concept: "中东油价",
		Searcher: &resolverTestSearcher{hits: []InternalContextHit{
			resolverLaneHit(1, 10, "原油市场", ""),
			resolverLaneHit(2, 20, "航运指数", ""),
		}},
		Cosine: func(_ context.Context, concept string, cand ResolveCandidate) (float64, bool) {
			if cand.Label == "原油市场" {
				return 0.90, true
			}
			return 0.82, true
		},
		Threshold: resolverTestThreshold,
		Margin:    resolverTestMargin,
	}
	res, err := ResolveTarget(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeResolved, res.Outcome, "gap == margin resolves (>= semantics)")
	require.Equal(t, "cosine", res.ScoreKind)

	// gap 0.079 < margin 0.08 → ambiguous.
	in2 := in
	in2.Cosine = func(_ context.Context, concept string, cand ResolveCandidate) (float64, bool) {
		if cand.Label == "原油市场" {
			return 0.899, true
		}
		return 0.82, true
	}
	res2, err := ResolveTarget(context.Background(), in2)
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeAmbiguous, res2.Outcome)
}

func TestResolveTarget_StableOrderingAndValidation(t *testing.T) {
	// Equal scores sort by board id (deterministic beyond ties).
	res, err := ResolveTarget(context.Background(), resolverTestInput([]InternalContextHit{
		resolverLaneHit(9, 90, "日债收益率A", ""),
		resolverLaneHit(2, 20, "日债收益率B", ""),
	}))
	require.NoError(t, err)
	require.Len(t, res.Candidates, 2)
	require.Equal(t, uint(2), res.Candidates[0].BoardID, "tie breaks by board id ascending")

	// Whitespace/emoji concept normalizes; empty concept errors.
	_, err = ResolveTarget(context.Background(), ResolveTargetInput{Concept: "   ", Searcher: &resolverTestSearcher{}})
	require.Error(t, err)
	res3, err := ResolveTarget(context.Background(), ResolveTargetInput{
		Concept: "  日债收益率 📈 ", Searcher: &resolverTestSearcher{hits: []InternalContextHit{
			resolverLaneHit(5, 77, "日债收益率", ""),
		}}, Threshold: resolverTestThreshold, Margin: resolverTestMargin,
	})
	require.NoError(t, err)
	require.Equal(t, ResolveOutcomeResolved, res3.Outcome, "emoji/space concept still resolves via label containment")

	// nil searcher errors (wiring guard).
	_, err = ResolveTarget(context.Background(), ResolveTargetInput{Concept: "x"})
	require.Error(t, err)

	// Repeated identical input → identical outcome (idempotent resolution).
	again, _ := ResolveTarget(context.Background(), resolverTestInput([]InternalContextHit{
		resolverLaneHit(5, 77, "日债收益率", "日本国债"),
	}))
	first, _ := ResolveTarget(context.Background(), resolverTestInput([]InternalContextHit{
		resolverLaneHit(5, 77, "日债收益率", "日本国债"),
	}))
	require.Equal(t, first.Outcome, again.Outcome)
	require.Equal(t, first.Best.BoardID, again.Best.BoardID)
}
