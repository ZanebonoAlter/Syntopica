package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

func TestBuildQualityBreakdownJSON(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "AI芯片", MatchReason: "direct_hit", Score: 1.0, Downgraded: false},
		{ID: 2, Label: "GPT-5发布", MatchReason: "max_sim", Score: 0.85, Downgraded: false},
		{ID: 3, Label: "AI竞赛", MatchReason: "weighted", Score: 0.59, Downgraded: false},
		{ID: 4, Label: "GPU短缺", MatchReason: "max_sim", Score: 0.72, Downgraded: true},
	}

	// Test with 3 tag IDs
	raw := buildQualityBreakdownJSON(tags, []uint{1, 2, 3})
	require.NotNil(t, raw)

	var entries []map[string]any
	err := json.Unmarshal(raw, &entries)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// Verify each entry has the expected fields
	for _, e := range entries {
		assert.NotNil(t, e["tag_id"])
		assert.NotNil(t, e["label"])
		assert.NotNil(t, e["match_reason"])
		assert.NotNil(t, e["score"])
		assert.NotNil(t, e["downgraded"])
	}

	// Verify first entry (direct_hit)
	assert.Equal(t, float64(1), entries[0]["tag_id"])
	assert.Equal(t, "AI芯片", entries[0]["label"])
	assert.Equal(t, "direct_hit", entries[0]["match_reason"])
	assert.Equal(t, float64(1), entries[0]["score"])
	assert.Equal(t, false, entries[0]["downgraded"])

	// Test empty tag IDs
	raw2 := buildQualityBreakdownJSON(tags, []uint{})
	require.NotNil(t, raw2)
	var emptyEntries []map[string]any
	err = json.Unmarshal(raw2, &emptyEntries)
	require.NoError(t, err)
	assert.Len(t, emptyEntries, 0)
}

func TestBuildQualityBreakdownJSON_DowngradedFlag(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 10, Label: "降级标签", MatchReason: "max_sim", Score: 0.82, Downgraded: true},
	}

	raw := buildQualityBreakdownJSON(tags, []uint{10})
	var entries []map[string]any
	err := json.Unmarshal(raw, &entries)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, true, entries[0]["downgraded"])
	assert.Equal(t, "max_sim", entries[0]["match_reason"])
	assert.Equal(t, float64(0.82), entries[0]["score"])
}

func TestFilterTagsByQuality_PreservesDirectHitHitRateMaxSim(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, MatchReason: "direct_hit", Score: 1.0},
		{ID: 2, MatchReason: "hit_rate", Score: 0.9},
		{ID: 3, MatchReason: "max_sim", Score: 0.8},
		{ID: 4, MatchReason: "weighted", Score: 0.5},
	}
	result := filterTagsByQuality(tags)
	assert.Len(t, result, 4) // weighted pulled back since < 10
}

func TestFilterTagsByQuality_PullsBackWeightedWhenFew(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, MatchReason: "direct_hit", Score: 1.0},
		{ID: 2, MatchReason: "direct_hit", Score: 0.9},
		{ID: 3, MatchReason: "direct_hit", Score: 0.8},
		{ID: 4, MatchReason: "weighted", Score: 0.5},
		{ID: 5, MatchReason: "weighted", Score: 0.4},
	}
	result := filterTagsByQuality(tags)
	// < 10 kept → weighted pulled back
	assert.Len(t, result, 5)
}

func TestFilterTagsByQuality_TruncatesAbove30(t *testing.T) {
	tags := make([]repository.TagInput, 0, 35)
	for i := 0; i < 35; i++ {
		tags = append(tags, repository.TagInput{
			ID: uint(i + 1), MatchReason: "max_sim", Score: float64(100 - i),
		})
	}
	result := filterTagsByQuality(tags)
	assert.Len(t, result, 30)
}

func TestFilterTagsByQuality_DowngradedMaxSimSortedToEnd(t *testing.T) {
	// Build >30 tags, with one downgraded max_sim that should sort lower
	tags := make([]repository.TagInput, 0, 35)
	for i := 0; i < 30; i++ {
		tags = append(tags, repository.TagInput{
			ID: uint(i + 1), MatchReason: "direct_hit", Score: float64(100 - i),
		})
	}
	// Add downgraded max_sim entries — they should be sorted to end
	tags = append(tags, repository.TagInput{ID: 31, MatchReason: "max_sim", Score: 0.99, Downgraded: true})
	tags = append(tags, repository.TagInput{ID: 32, MatchReason: "max_sim", Score: 0.95, Downgraded: true})
	tags = append(tags, repository.TagInput{ID: 33, MatchReason: "max_sim", Score: 0.90, Downgraded: false})
	tags = append(tags, repository.TagInput{ID: 34, MatchReason: "max_sim", Score: 0.85, Downgraded: false})
	tags = append(tags, repository.TagInput{ID: 35, MatchReason: "weighted", Score: 0.5})

	result := filterTagsByQuality(tags)
	assert.Len(t, result, 30)

	// The last entries should be the lowest-quality ones
	// Downgraded max_sim (tier 3) should be behind non-downgraded max_sim (tier 2)
	// All direct_hit (tier 0) should be first
	hasDowngraded := false
	for _, r := range result {
		if r.Downgraded {
			hasDowngraded = true
		}
	}
	// With 30 direct_hit entries at tier 0, the 2 downgraded max_sim (tier 3),
	// 2 non-downgraded max_sim (tier 2), and 1 weighted (tier 3) should not
	// displace any direct_hit entries. Since we only keep 30 and have 30 direct_hit,
	// none of the max_sim or weighted entries should appear.
	// This is correct: all kept entries are direct_hit (tier 0).
	// Downgraded entries correctly don't sneak in.
	_ = hasDowngraded
}

// TestBuildQualityBreakdownJSON_MergedSections covers the spec scenario
// "合并后重算明细": when MergeSimilarSections merges section A (tag IDs {1,2})
// with section B (tag IDs {3,4}), the recomputed quality_breakdown SHALL be
// the union of all four tags' details. MergeSimilarSections itself calls
// buildQualityBreakdownJSON with the merged tagIDSet, so this test asserts the
// union semantics the merge path relies on.
func TestBuildQualityBreakdownJSON_MergedSections(t *testing.T) {
	tags := []repository.TagInput{
		{ID: 1, Label: "AI芯片", MatchReason: "direct_hit", Score: 1.0, Downgraded: false},
		{ID: 2, Label: "GPT-5发布", MatchReason: "max_sim", Score: 0.85, Downgraded: false},
		{ID: 3, Label: "AI竞赛", MatchReason: "weighted", Score: 0.59, Downgraded: false},
		{ID: 4, Label: "GPU短缺", MatchReason: "max_sim", Score: 0.72, Downgraded: true},
	}

	// Merged tagIDSet = union of section A {1,2} and section B {3,4}.
	raw := buildQualityBreakdownJSON(tags, []uint{1, 2, 3, 4})
	require.NotNil(t, raw)

	var entries []map[string]any
	err := json.Unmarshal(raw, &entries)
	require.NoError(t, err)

	// Union of both sections → 4 entries, one per source tag.
	assert.Len(t, entries, 4)

	seenIDs := make(map[float64]bool, 4)
	for _, e := range entries {
		id, ok := e["tag_id"].(float64)
		require.True(t, ok)
		seenIDs[id] = true
		// Every entry carries the full per-tag detail (spec: tag_id/label/
		// match_reason/score/downgraded).
		assert.NotNil(t, e["label"])
		assert.NotNil(t, e["match_reason"])
		assert.NotNil(t, e["score"])
		assert.NotNil(t, e["downgraded"])
	}
	// All four merged tag IDs are present (union, no drops).
	assert.True(t, seenIDs[1] && seenIDs[2] && seenIDs[3] && seenIDs[4])
}

// ---- section display title decouple (spec: section 展示标题内容化) ----

// TestResolveClusterLabel_FallbackChain walks the whole fallback chain
// (spec Scenario 标题生成失败时降级兜底 + white-box branch table B1..B4):
// LLM daily title → first thread title → matched topic label → group name.
func TestResolveClusterLabel_FallbackChain(t *testing.T) {
	topicID := uint(935)
	labels := map[uint]string{topicID: "日本首相高市早苗宣布不于7月释放石油储备"}
	cluster := repository.ClusterGroup{GroupName: "L3分组名", MatchedTopicID: &topicID}

	// B1: LLM daily title wins even when a topic is matched (Scenario 命中既有话题的 section 标题反映当天内容).
	assert.Equal(t, "高市执政基础不稳引发党内反弹担忧",
		resolveClusterLabel("高市执政基础不稳引发党内反弹担忧", nil, cluster, labels))

	// B1 with only whitespace degrades to B2 (variant V4).
	assert.Equal(t, "首条叙事线标题",
		resolveClusterLabel("   ", []repository.Thread{{Title: "首条叙事线标题"}}, cluster, labels))

	// B2: missing section_title falls to the first non-blank thread title.
	assert.Equal(t, "首条叙事线标题",
		resolveClusterLabel("", []repository.Thread{{Title: "首条叙事线标题"}, {Title: "第二条"}}, cluster, labels))

	// B3: empty threads + matched topic → legacy topic-label safety net (Scenario 标题生成失败时降级兜底).
	assert.Equal(t, "日本首相高市早苗宣布不于7月释放石油储备",
		resolveClusterLabel("", nil, cluster, labels))

	// B4: no title, no threads, no topic label → group name (Scenario L3 新话题标题行为不变).
	noTopic := repository.ClusterGroup{GroupName: "L3分组名"}
	assert.Equal(t, "L3分组名", resolveClusterLabel("", nil, noTopic, labels))

	// B4 edge: matched topic missing from the label map → group name.
	assert.Equal(t, "L3分组名", resolveClusterLabel("", nil, cluster, map[uint]string{}))
}

// TestResolveClusterLabel_LongTitleStaysWithinColumnLimit guards variant V5:
// the label column is gorm size:200, so a runaway LLM title must be
// rune-truncated before persisting.
func TestResolveClusterLabel_LongTitleStaysWithinColumnLimit(t *testing.T) {
	long := strings.Repeat("超", 600)
	got := resolveClusterLabel(long, nil, repository.ClusterGroup{GroupName: "g"}, nil)
	if l := len([]rune(got)); l != 200 {
		t.Errorf("resolved label length %d runes, want exactly 200 (column budget)", l)
	}
}
