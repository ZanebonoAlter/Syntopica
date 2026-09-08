package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// ── Target concept → internal object resolver (design D2) ───────────────────
//
// Conservative, program-only binding: the LLM may name a concept, but the
// resolved internal id must come from this resolver's candidate search. A
// concept resolves ONLY when the top candidate clears the threshold AND leads
// the runner-up by the configured margin; ties and near-ties stay ambiguous
// (unresolved). Model-supplied ids are never accepted.

const relationResolverVersion = "resolver-v1"

// ResolveOutcome values.
const (
	ResolveOutcomeResolved  = "resolved"
	ResolveOutcomeAmbiguous = "ambiguous"
	ResolveOutcomeNoMatch   = "no_match"
)

// ResolveCandidate is one internal-object candidate with its scores.
type ResolveCandidate struct {
	Kind    string  `json:"kind"` // board | lane
	BoardID uint    `json:"board_id"`
	LaneID  *uint   `json:"lane_id,omitempty"`
	Label   string  `json:"label"`
	Lexical float64 `json:"lexical"`
	Cosine  float64 `json:"cosine"`
	Score   float64 `json:"score"` // effective ranking score
}

// ResolveTargetResult is the versioned mapping snapshot (frozen into the
// relation row's mapping_snapshot).
type ResolveTargetResult struct {
	Outcome         string             `json:"outcome"`
	Best            *ResolveCandidate  `json:"best,omitempty"`
	Candidates      []ResolveCandidate `json:"candidates"`
	ScoreKind       string             `json:"score_kind"` // cosine | lexical
	Threshold       float64            `json:"threshold"`
	Margin          float64            `json:"margin"`
	ResolverVersion string             `json:"resolver_version"`
}

// ResolveTargetInput carries the resolver's dependencies and knobs.
type ResolveTargetInput struct {
	Concept string
	// Searcher retrieves internal candidates (lexical pre-filter).
	Searcher InternalContextSearcher
	// Cosine optionally computes concept↔candidate semantic similarity
	// (0..1). nil → lexical-only scoring with stricter acceptance.
	Cosine func(ctx context.Context, concept string, cand ResolveCandidate) (float64, bool)
	TopK   int
	// Threshold: minimum top-1 score (cosine mode: similarity; lexical mode:
	// lexical score where 1.0 = label fully contains the concept tokens).
	Threshold float64
	// Margin: required top1-top2 gap.
	Margin float64
}

// lexicalConceptScore rates how strongly a candidate's label/summary matches
// the concept: 1.0 label token containment, 0.6 summary containment, 0.4
// partial token overlap on the label, 0 otherwise.
func lexicalConceptScore(concept, label, summary string) float64 {
	cNorm := normalizeForLexical(concept)
	lNorm := normalizeForLexical(label)
	sNorm := normalizeForLexical(summary)
	if cNorm == "" || lNorm == "" {
		return 0
	}
	if strings.Contains(lNorm, cNorm) || strings.Contains(cNorm, lNorm) {
		return 1.0
	}
	if sNorm != "" && (strings.Contains(sNorm, cNorm) || strings.Contains(cNorm, sNorm)) {
		return 0.6
	}
	cTokens := tokenizeForLexical(cNorm)
	lTokens := tokenizeForLexical(lNorm)
	if len(cTokens) == 0 || len(lTokens) == 0 {
		return 0
	}
	hit := 0
	for _, ct := range cTokens {
		for _, lt := range lTokens {
			if ct == lt || (len(ct) >= 2 && len(lt) >= 2 && (strings.Contains(ct, lt) || strings.Contains(lt, ct))) {
				hit++
				break
			}
		}
	}
	overlap := float64(hit) / float64(len(cTokens))
	if overlap >= 0.5 {
		return 0.4
	}
	return 0
}

func normalizeForLexical(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		switch r {
		case '　', '\t', '\n', '\r':
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func tokenizeForLexical(s string) []string {
	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, `，。、；：“”‘’()（）[]【】!?！？.,;:`)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ResolveTarget runs the conservative resolution. Pure over its inputs (the
// searcher/cosine are injected), so contract tests need no DB.
func ResolveTarget(ctx context.Context, in ResolveTargetInput) (*ResolveTargetResult, error) {
	concept := strings.TrimSpace(in.Concept)
	if concept == "" {
		return nil, fmt.Errorf("resolve target: empty concept")
	}
	if in.Searcher == nil {
		return nil, fmt.Errorf("resolve target: searcher not wired")
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 5
	}
	hits, err := in.Searcher.SearchInternalContext(ctx, concept, topK*2)
	if err != nil {
		return nil, fmt.Errorf("resolve target: search: %w", err)
	}

	res := &ResolveTargetResult{
		Outcome:         ResolveOutcomeNoMatch,
		Threshold:       in.Threshold,
		Margin:          in.Margin,
		ResolverVersion: relationResolverVersion,
	}
	cosineAvailable := false
	for _, h := range hits {
		cand := ResolveCandidate{
			Kind: h.Kind, BoardID: h.BoardID, LaneID: h.LaneID, Label: h.Label,
			Lexical: lexicalConceptScore(concept, h.Label, h.Summary),
		}
		if in.Cosine != nil {
			if cos, ok := in.Cosine(ctx, concept, cand); ok && cos > 0 {
				cand.Cosine = cos
				cosineAvailable = true
			}
		}
		res.Candidates = append(res.Candidates, cand)
	}
	if len(res.Candidates) == 0 {
		res.ScoreKind = "lexical"
		return res, nil
	}

	if cosineAvailable {
		// Cosine ranks; lexical breaks near-ties (a label that literally
		// contains the concept outranks a mere semantic neighbour).
		res.ScoreKind = "cosine"
		for i := range res.Candidates {
			res.Candidates[i].Score = res.Candidates[i].Cosine + 0.1*res.Candidates[i].Lexical
		}
	} else {
		// Lexical-only mode is stricter: only a real label/summary hit may
		// resolve (threshold enforced on the lexical score itself).
		res.ScoreKind = "lexical"
		for i := range res.Candidates {
			res.Candidates[i].Score = res.Candidates[i].Lexical
		}
	}

	sort.SliceStable(res.Candidates, func(i, j int) bool {
		if res.Candidates[i].Score != res.Candidates[j].Score {
			return res.Candidates[i].Score > res.Candidates[j].Score
		}
		return res.Candidates[i].BoardID < res.Candidates[j].BoardID
	})
	if len(res.Candidates) > topK {
		res.Candidates = res.Candidates[:topK]
	}

	top := res.Candidates[0]
	if top.Score < in.Threshold {
		return res, nil // no_match (below threshold)
	}
	if len(res.Candidates) > 1 {
		runnerUp := res.Candidates[1]
		if top.Score-runnerUp.Score < in.Margin {
			res.Outcome = ResolveOutcomeAmbiguous
			return res, nil
		}
	}
	res.Outcome = ResolveOutcomeResolved
	best := top
	res.Best = &best
	return res, nil
}
