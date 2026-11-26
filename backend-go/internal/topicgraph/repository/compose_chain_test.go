package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Candidate-chain algorithm (lane-compose contract §2.1, whitebox C1–C15) ──
//
// The chain semantics are a pure function: no DB, no clock. The cases below
// mirror the whitebox branch table of
// openspec/changes/rework-workshop-into-radar-and-desk/test-cases-lane-compose-implementation.md §3.1.

func chainDay(t *testing.T, s string) time.Time {
	t.Helper()
	day, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return day
}

func chainSection(t *testing.T, id uint, day string, topicID *uint, status string) ComposeChainSection {
	t.Helper()
	return ComposeChainSection{
		ID:                id,
		PeriodDate:        chainDay(t, day),
		PersistentTopicID: topicID,
		TopicLabel:        "topic-label",
		TopicStatus:       status,
	}
}

func topicIDPtr(id uint) *uint { return &id }

func identityRelation(from, to uint) ComposeRelation {
	return ComposeRelation{FromID: from, ToID: to, Distance: 0.1, RelationType: "identity"}
}

func similarityRelation(from, to uint) ComposeRelation {
	return ComposeRelation{FromID: from, ToID: to, Distance: 0.42, RelationType: "similarity"}
}

// C1 — empty input returns an empty result and never panics.
func TestBuildComposeCandidateChains_C1EmptyInput(t *testing.T) {
	chains := BuildComposeCandidateChains(nil, nil)
	assert.NotNil(t, chains)
	assert.Empty(t, chains)

	chains = BuildComposeCandidateChains([]ComposeChainSection{}, []ComposeRelation{})
	assert.NotNil(t, chains)
	assert.Empty(t, chains)
}

// C2 — sections without a persistent topic never enter a chain.
func TestBuildComposeCandidateChains_C2NoTopicAssignment(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", nil, ""),
		chainSection(t, 2, "2026-10-30", nil, ""),
	}
	chains := BuildComposeCandidateChains(sections, []ComposeRelation{identityRelation(1, 2)})
	assert.Empty(t, chains)
}

// C3 — only candidate topics qualify; active / archived topics are excluded.
func TestBuildComposeCandidateChains_C3NonCandidateStatus(t *testing.T) {
	for _, status := range []string{TopicStatusActive, TopicStatusArchived} {
		sections := []ComposeChainSection{
			chainSection(t, 1, "2026-10-27", topicIDPtr(7), status),
			chainSection(t, 2, "2026-10-30", topicIDPtr(7), status),
		}
		chains := BuildComposeCandidateChains(sections, []ComposeRelation{identityRelation(1, 2)})
		assert.Empty(t, chains, "status %s must not produce a candidate chain", status)
	}
}

// C4 — a single section (no edge) never qualifies.
func TestBuildComposeCandidateChains_C4SingleNodeComponent(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
	}
	chains := BuildComposeCandidateChains(sections, nil)
	assert.Empty(t, chains)
}

// C5 — two nodes three days apart qualify (span_days = 3).
func TestBuildComposeCandidateChains_C5TwoNodesSpanThreeDays(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 7345, "2026-10-27", topicIDPtr(1984), TopicStatusCandidate),
		chainSection(t, 7409, "2026-10-30", topicIDPtr(1984), TopicStatusCandidate),
	}
	chains := BuildComposeCandidateChains(sections, []ComposeRelation{identityRelation(7345, 7409)})
	require.Len(t, chains, 1)
	assert.Equal(t, uint(1984), chains[0].TopicID)
	assert.Equal(t, []uint{7345, 7409}, chains[0].SectionIDs)
	assert.Equal(t, "2026-10-27", chains[0].FirstDate)
	assert.Equal(t, "2026-10-30", chains[0].LastDate)
	assert.Equal(t, 3, chains[0].SpanDays)
	assert.Equal(t, "1984:7345", chains[0].Key)
	assert.Equal(t, TopicStatusCandidate, chains[0].TopicStatus)
}

// C6 — two nodes two days apart do not qualify.
func TestBuildComposeCandidateChains_C6TwoNodesSpanTwoDays(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-29", topicIDPtr(7), TopicStatusCandidate),
	}
	chains := BuildComposeCandidateChains(sections, []ComposeRelation{identityRelation(1, 2)})
	assert.Empty(t, chains)
}

// C7 — gaps in the middle of a chain are allowed.
func TestBuildComposeCandidateChains_C7GapInTheMiddle(t *testing.T) {
	// 10-28 and 10-29 hold no section of this topic — the chain still spans
	// three days and qualifies.
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(7), TopicStatusCandidate),
	}
	chains := BuildComposeCandidateChains(sections, []ComposeRelation{identityRelation(1, 2)})
	require.Len(t, chains, 1)
	assert.Equal(t, 3, chains[0].SpanDays)
	assert.Equal(t, "2026-10-27", chains[0].FirstDate)
	assert.Equal(t, "2026-10-30", chains[0].LastDate)
}

// C8 — a similarity edge connects just like an identity edge.
func TestBuildComposeCandidateChains_C8SimilarityEdgeConnects(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(7), TopicStatusCandidate),
	}
	chains := BuildComposeCandidateChains(sections, []ComposeRelation{similarityRelation(1, 2)})
	require.Len(t, chains, 1)
	assert.Equal(t, 3, chains[0].SpanDays)
}

// C9 — a similarity edge across two topics never merges them; each topic is
// judged by its own components.
func TestBuildComposeCandidateChains_C9CrossTopicEdgeDoesNotMerge(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(11), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(11), TopicStatusCandidate),
		chainSection(t, 3, "2026-10-28", topicIDPtr(22), TopicStatusCandidate),
		chainSection(t, 4, "2026-10-31", topicIDPtr(22), TopicStatusCandidate),
	}
	relations := []ComposeRelation{
		identityRelation(1, 2),
		identityRelation(3, 4),
		similarityRelation(2, 3), // cross-topic edge — must not merge
	}
	chains := BuildComposeCandidateChains(sections, relations)
	require.Len(t, chains, 2)

	byTopic := map[uint][]uint{}
	for _, c := range chains {
		byTopic[c.TopicID] = c.SectionIDs
	}
	assert.Equal(t, []uint{1, 2}, byTopic[11])
	assert.Equal(t, []uint{3, 4}, byTopic[22])
}

// C10 — two disconnected components of the same topic become two chains and
// are never mixed.
func TestBuildComposeCandidateChains_C10DisconnectedComponentsStaySeparate(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 3, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 4, "2026-10-31", topicIDPtr(7), TopicStatusCandidate),
	}
	relations := []ComposeRelation{
		identityRelation(1, 2),
		identityRelation(3, 4),
	}
	chains := BuildComposeCandidateChains(sections, relations)
	require.Len(t, chains, 2)

	keys := []string{chains[0].Key, chains[1].Key}
	assert.ElementsMatch(t, []string{"7:1", "7:3"}, keys)
	for _, c := range chains {
		assert.Len(t, c.SectionIDs, 2)
	}
}

// C11 — the span is a UTC calendar-day difference, not a month-boundary guess.
func TestBuildComposeCandidateChains_C11SpanUsesUTCDays(t *testing.T) {
	// 09-30 → 10-02 is two days — below the threshold, so no chain.
	short := []ComposeChainSection{
		chainSection(t, 1, "2026-09-30", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-02", topicIDPtr(7), TopicStatusCandidate),
	}
	chains := BuildComposeCandidateChains(short, []ComposeRelation{identityRelation(1, 2)})
	assert.Empty(t, chains)

	// 09-28 → 10-02 is four days — qualifies and reports the UTC span.
	qualified := []ComposeChainSection{
		chainSection(t, 1, "2026-09-28", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-02", topicIDPtr(7), TopicStatusCandidate),
	}
	chains = BuildComposeCandidateChains(qualified, []ComposeRelation{identityRelation(1, 2)})
	require.Len(t, chains, 1)
	assert.Equal(t, "2026-09-28", chains[0].FirstDate)
	assert.Equal(t, "2026-10-02", chains[0].LastDate)
	assert.Equal(t, 4, chains[0].SpanDays)
}

// C12 — the key is stable across calls and equals "{topic_id}:{min section id}".
func TestBuildComposeCandidateChains_C12KeyIsStable(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 7409, "2026-10-27", topicIDPtr(1984), TopicStatusCandidate),
		chainSection(t, 7345, "2026-10-30", topicIDPtr(1984), TopicStatusCandidate),
	}
	relations := []ComposeRelation{identityRelation(7409, 7345)}

	first := BuildComposeCandidateChains(sections, relations)
	second := BuildComposeCandidateChains(sections, relations)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, first[0].Key, second[0].Key)
	// The smallest section id wins — not the earliest date, not the first row.
	assert.Equal(t, "1984:7345", first[0].Key)
}

// C13 — ordering: last_date desc → span_days desc → topic_id asc.
func TestBuildComposeCandidateChains_C13Ordering(t *testing.T) {
	sections := []ComposeChainSection{
		// chain A: topic 30, ends 2026-10-30, span 3
		chainSection(t, 1, "2026-10-27", topicIDPtr(30), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(30), TopicStatusCandidate),
		// chain B: topic 10, ends 2026-10-30, span 5 (same last_date, longer span)
		chainSection(t, 3, "2026-10-25", topicIDPtr(10), TopicStatusCandidate),
		chainSection(t, 4, "2026-10-30", topicIDPtr(10), TopicStatusCandidate),
		// chain C: topic 20, ends 2026-10-29, span 4 (older last_date)
		chainSection(t, 5, "2026-10-25", topicIDPtr(20), TopicStatusCandidate),
		chainSection(t, 6, "2026-10-29", topicIDPtr(20), TopicStatusCandidate),
		// chain D: topic 5, ends 2026-10-30, span 5 (same last_date + span as B,
		// smaller topic id wins)
		chainSection(t, 7, "2026-10-25", topicIDPtr(5), TopicStatusCandidate),
		chainSection(t, 8, "2026-10-30", topicIDPtr(5), TopicStatusCandidate),
	}
	relations := []ComposeRelation{
		identityRelation(1, 2),
		identityRelation(3, 4),
		identityRelation(5, 6),
		identityRelation(7, 8),
	}
	chains := BuildComposeCandidateChains(sections, relations)
	require.Len(t, chains, 4)

	topics := make([]uint, len(chains))
	for i, c := range chains {
		topics[i] = c.TopicID
	}
	// B and D share last_date + span → topic_id asc puts 5 first; C is older.
	assert.Equal(t, []uint{5, 10, 30, 20}, topics)
}

// C14 — a relation pointing outside the visible window is ignored and never
// panics.
func TestBuildComposeCandidateChains_C14RelationWithMissingEndpoint(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(7), TopicStatusCandidate),
	}
	relations := []ComposeRelation{
		identityRelation(1, 2),
		identityRelation(1, 9999), // 9999 is outside the window
		identityRelation(9999, 2),
	}
	chains := BuildComposeCandidateChains(sections, relations)
	require.Len(t, chains, 1)
	assert.Equal(t, []uint{1, 2}, chains[0].SectionIDs)
}

// C15 — duplicated edges do not change the connectivity verdict.
func TestBuildComposeCandidateChains_C15DuplicateEdgesAreHarmless(t *testing.T) {
	sections := []ComposeChainSection{
		chainSection(t, 1, "2026-10-27", topicIDPtr(7), TopicStatusCandidate),
		chainSection(t, 2, "2026-10-30", topicIDPtr(7), TopicStatusCandidate),
	}
	relations := []ComposeRelation{
		identityRelation(1, 2),
		identityRelation(1, 2),
		similarityRelation(1, 2),
	}
	chains := BuildComposeCandidateChains(sections, relations)
	require.Len(t, chains, 1)
	assert.Equal(t, []uint{1, 2}, chains[0].SectionIDs)
	assert.Equal(t, 3, chains[0].SpanDays)
}
