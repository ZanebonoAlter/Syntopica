package entity

import (
	"sort"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// Candidate merge (design D4): the three candidate paths —
//
//	① entity-name vector recall
//	② entity-name+short-desc vector recall
//	③ registered-name hit (entity_mentions.mention_key == normalized mention)
//
// are merged and DEDUPED BY entity_id: one entity, one option. Same-name
// DIFFERENT entities stay separate options (ambiguity is legal; the
// second-round model decides with context — a name hit is NEVER an identity
// decision, never a hard reuse).

// Candidate is one merged option for the second-round choice question.
type Candidate struct {
	EntityID   uint
	EntityKey  string // opaque handle the model echoes back
	Name       string // canonical name
	StableDesc string // stable short identity description (may be empty)
	// Sources marks which paths recalled this entity ("name", "name_desc",
	// "registered_name") — eval-report material, never a decision input.
	Sources []string
	// BestScore is the max cosine across the vector paths (0 when only the
	// registered-name path hit). Ordering material only.
	BestScore float64
}

// MergeCandidates merges the three paths. poolByID supplies entity_key /
// canonical name / stable desc for hit IDs (registered-name hits may reference
// entities absent from poolByID — defensive skip). maxOptions caps the merged
// list (a tunable EVALUATION configuration, never an identity threshold).
func MergeCandidates(namePath, nameDescPath []ScoredEntity, registeredHits []uint, poolByID map[uint]*tagmodels.EntityRegistry, maxOptions int) []Candidate {
	type acc struct {
		candidate Candidate
	}
	merged := make(map[uint]*acc)

	add := func(id uint, score float64, source string) {
		row, ok := poolByID[id]
		if !ok {
			return // pool row vanished between read and merge — skip
		}
		entry, exists := merged[id]
		if !exists {
			entry = &acc{candidate: Candidate{
				EntityID:   id,
				EntityKey:  row.EntityKey,
				Name:       row.CanonicalName,
				StableDesc: row.StableDesc,
				Sources:    []string{source},
				BestScore:  score,
			}}
			merged[id] = entry
			return
		}
		if score > entry.candidate.BestScore {
			entry.candidate.BestScore = score
		}
		for _, s := range entry.candidate.Sources {
			if s == source {
				return
			}
		}
		entry.candidate.Sources = append(entry.candidate.Sources, source)
	}

	for _, h := range namePath {
		add(h.EntityID, h.Score, "name")
	}
	for _, h := range nameDescPath {
		add(h.EntityID, h.Score, "name_desc")
	}
	for _, id := range registeredHits {
		add(id, 0, "registered_name")
	}

	out := make([]Candidate, 0, len(merged))
	for _, entry := range merged {
		out = append(out, entry.candidate)
	}
	// Deterministic order: score desc, ties → entity_id asc (same tiebreak
	// as each retrieval path).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].BestScore != out[j].BestScore {
			return out[i].BestScore > out[j].BestScore
		}
		return out[i].EntityID < out[j].EntityID
	})
	if maxOptions > 0 && len(out) > maxOptions {
		out = truncateKeepingRegistered(out, maxOptions)
	}
	return out
}

// truncateKeepingRegistered caps the merged list while keeping EVERY
// registered-name hit. A registered hit enters with score 0 (no vector
// recall), so a plain score-desc truncation would let a growing vector pool
// push out the only entity that owns the mention's exact spelling (e.g. the
// nickname「老黄」 whose name vector may score low — registered-name is its
// only recall path). Losing it means an empty candidate list → none → a
// duplicate new entity. The reserved seats keep the cap total unchanged: the
// vector-path remainder is filled by score desc (same tiebreak), and relative
// order of the kept rows is preserved.
func truncateKeepingRegistered(sorted []Candidate, maxOptions int) []Candidate {
	reserved := make([]Candidate, 0, maxOptions)
	remainder := make([]Candidate, 0, len(sorted))
	for _, c := range sorted {
		if hasSource(c, "registered_name") {
			reserved = append(reserved, c)
			continue
		}
		remainder = append(remainder, c)
	}
	if len(reserved) >= maxOptions {
		// 叫法命中自身已占满席位：不因分数而丢掉它们（它们才是唯一召回）。
		return reserved[:maxOptions]
	}
	kept := make(map[uint]bool, maxOptions)
	for _, c := range reserved {
		kept[c.EntityID] = true
	}
	for _, c := range remainder {
		if len(kept) >= maxOptions {
			break
		}
		kept[c.EntityID] = true
	}
	// 保持排序（分数降序 + ID 升序）不变，只筛掉未保留的行。
	out := make([]Candidate, 0, maxOptions)
	for _, c := range sorted {
		if kept[c.EntityID] {
			delete(kept, c.EntityID)
			out = append(out, c)
		}
	}
	return out
}

// hasSource reports whether a candidate was recalled by the given path.
func hasSource(c Candidate, source string) bool {
	for _, s := range c.Sources {
		if s == source {
			return true
		}
	}
	return false
}
