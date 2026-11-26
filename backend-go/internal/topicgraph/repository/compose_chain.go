package repository

import (
	"fmt"
	"sort"
	"time"
)

// Candidate-chain qualification thresholds (lane-compose contract §2.1).
const (
	// composeChainMinMembers — a candidate chain needs at least two sections.
	composeChainMinMembers = 2
	// composeChainMinSpanDays — the first and last section of a chain must be
	// at least this many UTC days apart (gaps in between are allowed).
	composeChainMinSpanDays = 3
)

// ComposeRelation is a section-to-section edge of the compose workbench.
// Both relation types are carried verbatim: identity (same persistent topic)
// and similarity (Hungarian bipartite match). The canvas draws them with
// different line styles; the candidate-chain algorithm treats both as
// connectivity edges.
type ComposeRelation struct {
	FromID       uint    `json:"from_id"`
	ToID         uint    `json:"to_id"`
	Distance     float64 `json:"distance"`
	RelationType string  `json:"relation_type"`
}

// ComposeChainSection is the minimal section view the candidate-chain
// algorithm needs: the section identity, its report day and the persistent
// topic it belongs to (id + label + status).
type ComposeChainSection struct {
	ID                uint
	PeriodDate        time.Time
	PersistentTopicID *uint
	TopicLabel        string
	TopicStatus       string
}

// ComposeCandidateChain is one qualified candidate chain of the compose
// workbench — a connected run of sections that all belong to the same
// candidate topic and stretch over at least composeChainMinSpanDays days.
type ComposeCandidateChain struct {
	// Key is "{topic_id}:{smallest section id in the chain}" — stable across
	// calls so the frontend can keep its selection when the window shifts.
	Key         string `json:"key"`
	TopicID     uint   `json:"topic_id"`
	TopicLabel  string `json:"topic_label"`
	TopicStatus string `json:"topic_status"`
	// SectionIDs are ordered by (period_date, id) ascending.
	SectionIDs []uint `json:"section_ids"`
	FirstDate  string `json:"first_date"`
	LastDate   string `json:"last_date"`
	// SpanDays is the UTC calendar-day distance between FirstDate and
	// LastDate (gaps in between are allowed).
	SpanDays int `json:"span_days"`
}

// BuildComposeCandidateChains computes the candidate chains of the compose
// workbench. It is the server-side single source of truth for the chain
// semantics (lane-compose contract §2.1); the frontend only renders them.
//
// Rules:
//  1. Only sections whose persistent topic has status "candidate" and a
//     non-null persistent_topic_id are considered.
//  2. Sections are grouped by persistent_topic_id — sections of different
//     topics never merge, not even when a similarity edge connects them.
//  3. Inside a group the original relations (identity and similarity alike)
//     define the undirected connectivity; a relation pointing outside the
//     visible window is ignored.
//  4. Every connected component is a chain candidate on its own; components of
//     the same topic are never mixed.
//  5. A component qualifies when it holds at least composeChainMinMembers
//     sections and spans at least composeChainMinSpanDays UTC days.
//
// The function is pure (no DB access) so it can be unit-tested directly.
func BuildComposeCandidateChains(sections []ComposeChainSection, relations []ComposeRelation) []ComposeCandidateChain {
	chains := make([]ComposeCandidateChain, 0)
	if len(sections) == 0 {
		return chains
	}

	// 1. Keep candidate-topic sections only, grouped by persistent topic.
	byTopic := make(map[uint][]ComposeChainSection)
	topicOf := make(map[uint]uint) // section id → persistent topic id
	for _, s := range sections {
		if s.PersistentTopicID == nil || s.TopicStatus != TopicStatusCandidate {
			continue
		}
		tid := *s.PersistentTopicID
		byTopic[tid] = append(byTopic[tid], s)
		topicOf[s.ID] = tid
	}
	if len(byTopic) == 0 {
		return chains
	}

	// 2. Undirected neighbour index over the visible sections.
	neighbors := make(map[uint][]uint)
	for _, rel := range relations {
		if rel.FromID == rel.ToID {
			continue
		}
		neighbors[rel.FromID] = append(neighbors[rel.FromID], rel.ToID)
		neighbors[rel.ToID] = append(neighbors[rel.ToID], rel.FromID)
	}

	// 3. Connected components inside each topic group.
	for tid, group := range byTopic {
		member := make(map[uint]ComposeChainSection, len(group))
		for _, s := range group {
			member[s.ID] = s
		}
		visited := make(map[uint]bool, len(group))
		for _, seed := range group {
			if visited[seed.ID] {
				continue
			}
			visited[seed.ID] = true
			component := []ComposeChainSection{seed}
			queue := []uint{seed.ID}
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				for _, nb := range neighbors[cur] {
					if visited[nb] {
						continue
					}
					// A neighbour only connects when it belongs to the same
					// topic — cross-topic similarity edges never merge groups.
					if topicOf[nb] != tid {
						continue
					}
					visited[nb] = true
					queue = append(queue, nb)
					component = append(component, member[nb])
				}
			}

			// 4. Order the component by (period_date, id) and qualify it.
			sort.Slice(component, func(i, j int) bool {
				if !component[i].PeriodDate.Equal(component[j].PeriodDate) {
					return component[i].PeriodDate.Before(component[j].PeriodDate)
				}
				return component[i].ID < component[j].ID
			})
			if len(component) < composeChainMinMembers {
				continue
			}
			first := component[0].PeriodDate
			last := component[len(component)-1].PeriodDate
			span := utcDayDiff(first, last)
			if span < composeChainMinSpanDays {
				continue
			}
			sectionIDs := make([]uint, len(component))
			minID := component[0].ID
			for i, c := range component {
				sectionIDs[i] = c.ID
				if c.ID < minID {
					minID = c.ID
				}
			}
			chains = append(chains, ComposeCandidateChain{
				Key:         fmt.Sprintf("%d:%d", tid, minID),
				TopicID:     tid,
				TopicLabel:  component[0].TopicLabel,
				TopicStatus: component[0].TopicStatus,
				SectionIDs:  sectionIDs,
				FirstDate:   first.UTC().Format("2006-01-02"),
				LastDate:    last.UTC().Format("2006-01-02"),
				SpanDays:    span,
			})
		}
	}

	// 5. Stable ordering: last_date desc → span_days desc → topic_id asc
	//    (key asc as the final tiebreaker for full determinism).
	sort.Slice(chains, func(i, j int) bool {
		if chains[i].LastDate != chains[j].LastDate {
			return chains[i].LastDate > chains[j].LastDate
		}
		if chains[i].SpanDays != chains[j].SpanDays {
			return chains[i].SpanDays > chains[j].SpanDays
		}
		if chains[i].TopicID != chains[j].TopicID {
			return chains[i].TopicID < chains[j].TopicID
		}
		return chains[i].Key < chains[j].Key
	})
	return chains
}

// utcDayDiff returns the number of UTC calendar days between two instants
// (b − a). The compose window is day-granular, so the difference is computed
// on the UTC calendar date instead of the raw duration.
func utcDayDiff(a, b time.Time) int {
	au := time.Date(a.UTC().Year(), a.UTC().Month(), a.UTC().Day(), 0, 0, 0, 0, time.UTC)
	bu := time.Date(b.UTC().Year(), b.UTC().Month(), b.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(bu.Sub(au) / (24 * time.Hour))
}
