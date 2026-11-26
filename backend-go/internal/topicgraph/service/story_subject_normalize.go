package service

import (
	"strings"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/textutil"
)

// Subject normalization — three-tier matching (add-event-story-heat design
// §4.1, specs/event-story「主体归一化」, task 2.5). Pure decision logic over
// candidate story lines; the caller (2c) supplies the candidates and the
// embedding function. Order is fixed and first-hit-wins:
//
//	① subject_key exact (NormalizeSubjectKey, non-empty keys only)
//	② embedding cosine ≥ 0.92, only when at least one key is absent
//	③ subject literal (NormalizeEntityText exact)
//	④ miss → new subject candidate (input's normalized key)
//
// The 0.92 threshold deliberately dwarfs the event-recall 0.5: merging two
// DIFFERENT subjects (A 公司的事件挂到 B 公司) costs far more than a split
// (next run merges it back). Embedding only bridges spelling variants of the
// same entity — it never participates in event assignment (that is
// story_assign.go's rule layers).
//
// Explicit keys are authoritative: neither embedding nor literal matching may
// overwrite a different non-empty key. Embedding is a missing-key fallback,
// using the display subject when the input key is absent. It is called at
// most once, only if an eligible candidate already has an embedding.

// SubjectInput is the new event's identity pair.
type SubjectInput struct {
	Subject    string
	SubjectKey string
}

// SubjectCandidate is an existing story line's subject identity
// (board_stories row, StoryID for 2c's linking).
type SubjectCandidate struct {
	StoryID uint
	Subject string
	// SubjectKey as stored in board_stories (normalized here before compare).
	SubjectKey string
	// SubjectEmbedding is the story's subject_key embedding (empty → the
	// embedding tier skips this candidate).
	SubjectEmbedding []float64
}

// SubjectMatch is ResolveSubjectKey's outcome.
type SubjectMatch struct {
	// SubjectKey is the winning key: the matched candidate's normalized key,
	// or the input's normalized key on a miss (may be empty for keyless
	// input — callers treat that as "no identity", not as a match basis).
	SubjectKey string
	// Method is one of SubjectMethod*.
	Method string
	// Score: exact → 1, embedding → cosine, literal/new → 0.
	Score float64
	// Embedding echoes the input key (or keyless display subject) embedding
	// when computed; nil for exact hits, ineligible candidates, or failure.
	Embedding []float64
}

// SubjectMatch methods.
const (
	SubjectMethodExact     = "exact"
	SubjectMethodEmbedding = "embedding"
	SubjectMethodLiteral   = "literal"
	SubjectMethodNew       = "new"
)

// SubjectEmbedFunc embeds a subject key or a keyless display name; injected so
// tests never touch the AI router.
type SubjectEmbedFunc func(subjectKey string) ([]float64, error)

// StorySubjectEmbeddingThreshold is the cosine threshold for the embedding
// tier (design §4.2: 0.92, inclusive — 0.919 does NOT merge).
const StorySubjectEmbeddingThreshold = 0.92

// ResolveSubjectKey runs the three-tier match. See the file comment for the
// ordering and the embed-call budget; an empty input key never takes part in
// key equality (test-cases §5.2 #5), and candidates with empty keys are
// never matched by key equality (embedding/literal may still hit them).
func ResolveSubjectKey(input SubjectInput, candidates []SubjectCandidate, embed SubjectEmbedFunc) SubjectMatch {
	inputKey := textutil.NormalizeSubjectKey(input.SubjectKey)
	candKeys := make([]string, len(candidates))
	hasEmbeddingCandidate := false
	for i, c := range candidates {
		candKeys[i] = textutil.NormalizeSubjectKey(c.SubjectKey)
		if (inputKey == "" || candKeys[i] == "") && len(c.SubjectEmbedding) > 0 {
			hasEmbeddingCandidate = true
		}
	}

	// Tier ① exact — no embed call on this path.
	if inputKey != "" {
		for i := range candidates {
			if candKeys[i] != "" && candKeys[i] == inputKey {
				return SubjectMatch{SubjectKey: inputKey, Method: SubjectMethodExact, Score: 1}
			}
		}
	}

	// Tier ② cannot reconcile conflicting explicit keys. Missing input keys
	// use the display name as embedding evidence, never an invented type.
	embedText := inputKey
	if embedText == "" {
		embedText = strings.TrimSpace(input.Subject)
	}
	var embedding []float64
	if embedText != "" && hasEmbeddingCandidate && embed != nil {
		if vec, err := embed(embedText); err == nil && len(vec) > 0 {
			embedding = vec
			bestCos := 0.0
			bestIdx := -1
			for i, c := range candidates {
				if (inputKey != "" && candKeys[i] != "") || len(c.SubjectEmbedding) == 0 {
					continue // different explicit keys cannot merge here
				}
				cos, err := airouter.CosineSimilarity(vec, c.SubjectEmbedding)
				if err != nil || cos < StorySubjectEmbeddingThreshold || cos <= bestCos {
					continue
				}
				bestCos, bestIdx = cos, i
			}
			if bestIdx >= 0 {
				key := candKeys[bestIdx]
				if key == "" {
					key = inputKey
				}
				return SubjectMatch{SubjectKey: key, Method: SubjectMethodEmbedding, Score: bestCos, Embedding: embedding}
			}
		}
	}

	// Tier ③ literal — display-name equality after NormalizeEntityText.
	normSubject := textutil.NormalizeEntityText(input.Subject)
	if normSubject != "" {
		for i, c := range candidates {
			if inputKey != "" && candKeys[i] != "" && inputKey != candKeys[i] {
				continue
			}
			if c.Subject == "" || textutil.NormalizeEntityText(c.Subject) != normSubject {
				continue
			}
			key := candKeys[i]
			if key == "" {
				key = inputKey // literal-matched keyless candidate: adopt the input key
			}
			return SubjectMatch{SubjectKey: key, Method: SubjectMethodLiteral, Score: 0, Embedding: embedding}
		}
	}

	// Tier ④ new subject candidate.
	return SubjectMatch{SubjectKey: inputKey, Method: SubjectMethodNew, Score: 0, Embedding: embedding}
}
