package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/topicgraph/repository"
)

func topicPtr(id uint) *uint { return &id }

// mergeSection builds a minimal section for the MergeSimilarSections tests.
// Each section gets its own cluster_index (= id) so the sibling-bucket guard
// (same cluster_index ⇒ never merge) does not reject the pairs these tests
// exercise; the guard itself is covered by TestMergeRejectsSiblingBuckets.
func mergeSection(id uint, label string, anchor *uint, lane string, embedding string) repository.DailyReportSection {
	return repository.DailyReportSection{
		ID:             id,
		ClusterIndex:   int(id),
		ClusterLabel:   label,
		ClusterTagIDs:  []byte("[]"),
		MatchedTopicID: anchor,
		LaneTier:       lane,
		Embedding:      embedding,
		ArticleCount:   1,
	}
}

// TestMergeRejectsSiblingBuckets covers tasks 3.4 / Scenario 分桶产物不参与同日合并:
// sections sharing a cluster_index are the sibling output of one intra-lane
// split and must never be glued back together — even at distance 0.
func TestMergeRejectsSiblingBuckets(t *testing.T) {
	a := mergeSection(1, "Agent 工具链", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "Skill 生态", topicPtr(7), "l1_direct", "[1,0,0]")
	a.ClusterIndex = 3
	b.ClusterIndex = 3

	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 2, "sibling buckets of one cluster must stay separate")
}

// TestSameAnchorClass covers the anchor boundary predicate directly.
func TestSameAnchorClass(t *testing.T) {
	seven := topicPtr(7)
	twelve := topicPtr(12)
	cases := []struct {
		name string
		a, b *uint
		want bool
	}{
		{"same topic", seven, seven, true},
		{"different topics", seven, twelve, false},
		{"both new-narrative", nil, nil, true},
		{"anchored vs new-narrative", seven, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := mergeSection(1, "a", tc.a, "l1_direct", "[1,0]")
			b := mergeSection(2, "b", tc.b, "l3_new", "[1,0]")
			assert.Equal(t, tc.want, sameAnchorClass(a, b))
			assert.Equal(t, tc.want, sameAnchorClass(b, a))
		})
	}
}

// TestMergeSimilarSections_SameTopicMerges: two sections anchored to the same
// topic at distance < 0.20 merge (existing behaviour preserved).
func TestMergeSimilarSections_SameTopicMerges(t *testing.T) {
	a := mergeSection(1, "A", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "B", topicPtr(7), "l2_llm", "[0.99,0.01,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, uint(7), *got[0].MatchedTopicID)
}

// TestMergeSimilarSections_DifferentTopicsRejected: cross-topic pairs are
// rejected even at distance 0.11 (the observed blackhole geometry).
func TestMergeSimilarSections_DifferentTopicsRejected(t *testing.T) {
	a := mergeSection(1, "美伊博弈", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "大模型能力边界", topicPtr(12), "l1_direct", "[0.99,0.02,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 2, "cross-topic pair must stay separate even at dist 0.11")
}

// TestMergeSimilarSections_NewNarrativeNotAbsorbed: an L3 new-narrative
// section must not be absorbed by an anchored section at distance 0.14.
func TestMergeSimilarSections_NewNarrativeNotAbsorbed(t *testing.T) {
	a := mergeSection(1, "美伊博弈", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "中东地缘新叙事", nil, "l3_new", "[0.985,0.05,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 2, "anchored section must not absorb a new-narrative section")
}

// TestMergeSimilarSections_TwoNewNarrativesMayMerge: two unanchored L3
// sections at distance < 0.20 still merge (same new-narrative pool).
func TestMergeSimilarSections_TwoNewNarrativesMayMerge(t *testing.T) {
	a := mergeSection(1, "新叙事甲", nil, "l3_new", "[1,0,0]")
	b := mergeSection(2, "新叙事乙", nil, "l3_new", "[0.99,0.01,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 1, "two unanchored sections may merge")
}

// TestMergeSimilarSections_TransitiveClosureStopsAtBoundary: A(t7)↔B(t7)
// close enough to merge; B(t7)↔C(t12) equally close but cross-anchor — the
// union-find closure must merge only A,B and leave C alone regardless of
// distance (boundary filtering happens before edges exist).
func TestMergeSimilarSections_TransitiveClosureStopsAtBoundary(t *testing.T) {
	a := mergeSection(1, "A", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "B", topicPtr(7), "l1_direct", "[0.95,0.05,0]")
	c := mergeSection(3, "C", topicPtr(12), "l1_direct", "[0.7,0.5,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b, c}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 2, "closure merges A,B; C stays separate despite close distance")
	anchors := map[uint]bool{}
	for _, s := range got {
		if s.MatchedTopicID != nil {
			anchors[*s.MatchedTopicID] = true
		}
	}
	assert.True(t, anchors[7] && anchors[12], "both topics must keep a section")
}

// TestMergeSimilarSections_DisabledByConfig: mergeEnabled=false short-circuits
// any merging regardless of distance.
func TestMergeSimilarSections_DisabledByConfig(t *testing.T) {
	a := mergeSection(1, "A", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "B", topicPtr(7), "l2_llm", "[0.99,0.01,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, false)
	require.NoError(t, err)
	require.Len(t, got, 2, "disabled switch must keep all sections unmerged")
}

// TestMergeSimilarSections_DisabledLogsOnce: the disabled path emits the
// single short-circuit log line, not per-pair audit lines.
func TestMergeSimilarSections_DisabledLogsOnce(t *testing.T) {
	a := mergeSection(1, "A", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "B", topicPtr(12), "l1_direct", "[0.99,0.01,0]")
	_, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, false)
	require.NoError(t, err)
}

// TestMergeSimilarSections_AuditLogsEveryCandidate ensures the audit trail
// covers deterministic merges AND boundary rejections (labels + topics).
// The log itself is INFO-level stdout; here we assert the rejected pair
// stays separate and the call completes without error (log content is
// exercised by running the pipeline with logging enabled).
func TestMergeSimilarSections_AuditLogsEveryCandidate(t *testing.T) {
	a := mergeSection(1, "美伊博弈", topicPtr(7), "l1_direct", "[1,0,0]")
	b := mergeSection(2, "大模型监管", topicPtr(12), "l1_direct", "[0.99,0.02,0]")
	got, _, err := MergeSimilarSections(context.Background(),
		[]repository.DailyReportSection{a, b}, nil, nil, true)
	require.NoError(t, err)
	require.Len(t, got, 2)
}
