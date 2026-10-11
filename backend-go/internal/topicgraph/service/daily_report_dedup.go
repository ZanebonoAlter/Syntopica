package service

import (
	"context"
	"fmt"
	"sort"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/topicgraph/repository"
)

// DedupRelevance carries the board-relevance signals for the dedup
// three-level tiebreak (fix-digest-tag-leak): article_count >
// identity×board cosine > id. A nil *DedupRelevance — or a duel where either
// embedding is missing — falls back to the legacy ID tiebreak.
type DedupRelevance struct {
	// TagIdentityEmbeddings maps tag ID → identity vector
	// (topic_tag_embeddings WHERE embedding_type='identity').
	TagIdentityEmbeddings map[uint][]float64
	// BoardEmbedding is the current report board's vector
	// (semantic_labels WHERE label_type='board').
	BoardEmbedding []float64
}

// tagCosine returns cos(tag identity, board) when both sides are available
// and non-degenerate. Missing entry, empty/all-zero vector, dimension
// mismatch — all report "unavailable" so the duel falls back to the ID rule.
func (r *DedupRelevance) tagCosine(tagID uint) (float64, bool) {
	if r == nil || len(r.BoardEmbedding) == 0 || !hasNonZeroNorm(r.BoardEmbedding) {
		return 0, false
	}
	emb, ok := r.TagIdentityEmbeddings[tagID]
	if !ok || len(emb) == 0 || !hasNonZeroNorm(emb) {
		return 0, false
	}
	sim, err := airouter.CosineSimilarity(emb, r.BoardEmbedding)
	if err != nil {
		return 0, false
	}
	return sim, true
}

func hasNonZeroNorm(v []float64) bool {
	sum := 0.0
	for _, x := range v {
		sum += x * x
	}
	return sum > 0
}

// loadDedupRelevance loads the tiebreak signals after collect: every
// collected tag's identity embedding plus the reporting board's embedding.
// Any load failure (or missing board embedding) degrades to nil — the legacy
// ID tiebreak — because the report pipeline must not break over an
// enrichment signal.
func loadDedupRelevance(boardID uint, tags []repository.TagInput) *DedupRelevance {
	tagIDs := make([]uint, 0, len(tags))
	for _, t := range tags {
		tagIDs = append(tagIDs, t.ID)
	}
	embStr, err := repository.Repo.ListTagIdentityEmbeddings(tagIDs)
	if err != nil {
		logging.Warnf("daily-report: load identity embeddings for board %d dedup tiebreak failed (falling back to ID order): %v", boardID, err)
		return nil
	}
	identity := make(map[uint][]float64, len(embStr))
	for id, s := range embStr {
		if v, perr := parsePgVector(s); perr == nil && len(v) > 0 {
			identity[id] = v
		}
	}
	boardEmbStr, err := repository.Repo.GetBoardEmbedding(boardID)
	if err != nil {
		logging.Warnf("daily-report: load board %d embedding for dedup tiebreak failed (falling back to ID order): %v", boardID, err)
		return nil
	}
	if boardEmbStr == "" {
		return nil
	}
	boardEmb, perr := parsePgVector(boardEmbStr)
	if perr != nil || len(boardEmb) == 0 || !hasNonZeroNorm(boardEmb) {
		return nil
	}
	return &DedupRelevance{TagIdentityEmbeddings: identity, BoardEmbedding: boardEmb}
}

// DeduplicateTags removes duplicate tags based on article ID overlap.
// Two rules:
//  1. Tags with identical article ID sets → merge (keep the better one).
//  2. Tags with article_count=1 sharing the same article → merge.
//
// The survivor is picked by a three-level tiebreak (fix-digest-tag-leak):
// higher article_count, then higher cos(tag identity, board) when both
// embeddings are available, then lower ID. A nil relevance keeps the legacy
// two-level behavior (article_count > id).
//
// The function takes a slice of repository.TagInput plus a parallel slice of article ID sets
// (one per tag, in the same order) and returns the deduplicated slice along with
// the surviving indices.
func DeduplicateTags(ctx context.Context, tags []repository.TagInput, articleIDSets [][]uint, relevance *DedupRelevance) []repository.TagInput {
	_, span := tracing.Tracer(tracing.ServiceName).Start(ctx, "workflow.daily_report.dedup")
	defer span.End()

	if len(tags) == 0 {
		return nil
	}
	if len(articleIDSets) != len(tags) {
		return tags
	}

	n := len(tags)
	// Build a set-key for each tag: sorted, deduplicated article IDs → compact string key.
	// We use index-based maps to track which tags survive.
	alive := make([]bool, n)
	for i := range alive {
		alive[i] = true
	}

	// Map from article set signature to the "best" tag index.
	// For single-article tags, also track by single article ID.
	setSigToIdx := make(map[string]int)  // full set signature → best index
	singleArtToIdx := make(map[uint]int) // single article ID → best index

	for i, tag := range tags {
		arts := dedupAndSort(articleIDSets[i])

		if len(arts) == 0 {
			alive[i] = false
			continue
		}

		sig := uintSliceKey(arts)

		if existingIdx, ok := setSigToIdx[sig]; ok {
			// Rule 1: identical article sets → keep the better one.
			if shouldReplace(tags[existingIdx], tag, relevance) {
				alive[existingIdx] = false
				setSigToIdx[sig] = i
			} else {
				alive[i] = false
			}
			continue
		}
		setSigToIdx[sig] = i

		// Rule 2: single-article tags sharing the same article.
		if len(arts) == 1 {
			artID := arts[0]
			if existingIdx, ok := singleArtToIdx[artID]; ok {
				if shouldReplace(tags[existingIdx], tag, relevance) {
					alive[existingIdx] = false
					singleArtToIdx[artID] = i
					// Also update setSigToIdx if it still pointed to old index.
					if setSigToIdx[sig] == existingIdx {
						setSigToIdx[sig] = i
					}
				} else {
					alive[i] = false
					if setSigToIdx[sig] == i {
						delete(setSigToIdx, sig)
					}
				}
				continue
			}
			singleArtToIdx[artID] = i
		}
	}

	var result []repository.TagInput
	for i, a := range alive {
		if a {
			result = append(result, tags[i])
		}
	}
	return result
}

// shouldReplace returns true if challenger should replace incumbent.
// Three levels (fix-digest-tag-leak): higher article_count, then higher
// identity×board cosine (only when both embeddings resolve — any missing
// side falls straight through), then lower ID.
func shouldReplace(incumbent, challenger repository.TagInput, relevance *DedupRelevance) bool {
	if challenger.ArticleCount != incumbent.ArticleCount {
		return challenger.ArticleCount > incumbent.ArticleCount
	}
	cosIncumbent, okIncumbent := relevance.tagCosine(incumbent.ID)
	cosChallenger, okChallenger := relevance.tagCosine(challenger.ID)
	if okIncumbent && okChallenger && cosChallenger != cosIncumbent {
		return cosChallenger > cosIncumbent
	}
	return challenger.ID < incumbent.ID
}

func dedupAndSort(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]bool, len(ids))
	var result []uint
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uintSliceKey(ids []uint) string {
	return fmt.Sprintf("%v", ids)
}
