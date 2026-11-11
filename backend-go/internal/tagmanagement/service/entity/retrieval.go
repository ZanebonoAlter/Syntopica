package entity

import (
	"sort"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
	repository "syntopica-backend/internal/tagmanagement/repository"
)

// Two-path vector retrieval (design D3/D6 spec「主体提及的完整解析链」).
//
// Retrieval ONLY recalls candidates — no similarity value here ever decides
// identity (that is the second-round model's single job). Both representations
// compare strictly INSIDE one (model, version, dim) space: rows whose triplet
// differs from the query's are SKIPPED, never compared cross-space and never
// fallen back to the other representation. The NAME path always exists — an
// entity without a stable description simply has no second representation,
// it never becomes invisible.

// SpaceTriplet identifies one embedding space: rows and queries must match
// on all three fields to be comparable.
type SpaceTriplet struct {
	Model   string
	Version string
	Dim     int
}

// QueryVector is one embedded query text plus its space triplet (the triplet
// comes from the embedder that produced the vector — dim = len(Vec)).
type QueryVector struct {
	Vec     []float64
	Triplet SpaceTriplet
}

// ScoredEntity is one retrieval hit.
type ScoredEntity struct {
	EntityID uint
	Score    float64
}

// searchBySpace is the shared one-path retrieval: same-space filter →
// cosine → top-K with ties broken by entity ID ascending (mechanically
// deterministic). vecOf/rowSpace are per-path accessors.
func searchBySpace(pool []tagmodels.EntityRegistry, query QueryVector, k int,
	vecOf func(*tagmodels.EntityRegistry) ([]float64, bool),
	rowSpace func(*tagmodels.EntityRegistry) SpaceTriplet) []ScoredEntity {
	if k <= 0 || len(query.Vec) == 0 {
		return nil
	}
	wanted := SpaceTriplet{Model: query.Triplet.Model, Version: query.Triplet.Version, Dim: len(query.Vec)}
	hits := make([]ScoredEntity, 0, len(pool))
	for i := range pool {
		row := &pool[i]
		vec, ok := vecOf(row)
		if !ok || len(vec) == 0 {
			continue
		}
		rs := rowSpace(row)
		if rs != wanted {
			continue // cross-space row: skipped, never compared, never fallback
		}
		score, err := repository.CosineSimilarity(query.Vec, vec)
		if err != nil {
			continue // dimension guard already ensured equality; belt & braces
		}
		hits = append(hits, ScoredEntity{EntityID: row.ID, Score: score})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].EntityID < hits[j].EntityID // ties → entity ID ascending
	})
	if len(hits) > k {
		hits = hits[:k]
	}
	return hits
}

// SearchNamePath retrieves by the entity-NAME representation (canonical name
// vector). This path ALWAYS runs — it is the only one guaranteed to exist.
func SearchNamePath(pool []tagmodels.EntityRegistry, query QueryVector, k int) []ScoredEntity {
	return searchBySpace(pool, query, k,
		func(r *tagmodels.EntityRegistry) ([]float64, bool) {
			if r.NameEmbedding == nil || *r.NameEmbedding == "" {
				return nil, false
			}
			vec, err := repository.EntityParsePgVector(*r.NameEmbedding)
			if err != nil {
				return nil, false
			}
			return vec, true
		},
		func(r *tagmodels.EntityRegistry) SpaceTriplet {
			return SpaceTriplet{Model: r.NameEmbeddingModel, Version: r.NameEmbeddingVersion, Dim: r.NameEmbeddingDim}
		})
}

// SearchNameDescPath retrieves by the NAME+DESCRIPTION representation. Only
// called when the mention carries a stable description AND the query vector
// for that text exists; rows lacking the second representation are simply
// absent from this path (they remain reachable via the name path).
func SearchNameDescPath(pool []tagmodels.EntityRegistry, query QueryVector, k int) []ScoredEntity {
	return searchBySpace(pool, query, k,
		func(r *tagmodels.EntityRegistry) ([]float64, bool) {
			if r.NameDescEmbedding == nil || *r.NameDescEmbedding == "" {
				return nil, false
			}
			vec, err := repository.EntityParsePgVector(*r.NameDescEmbedding)
			if err != nil {
				return nil, false
			}
			return vec, true
		},
		func(r *tagmodels.EntityRegistry) SpaceTriplet {
			return SpaceTriplet{Model: r.NameDescEmbeddingModel, Version: r.NameDescEmbeddingVersion, Dim: r.NameDescEmbeddingDim}
		})
}

// EmbedQueryTexts are the canonical query texts for the two paths
// (same-templates as the research runner):
//   - name path:     the verbatim mention
//   - name+desc path: mention + " " + stable description (only when present)
func EmbedQueryTexts(mentionText, subjectDesc string) (nameText string, nameDescText string) {
	nameText = mentionText
	if subjectDesc != "" {
		nameDescText = mentionText + " " + subjectDesc
	}
	return nameText, nameDescText
}
