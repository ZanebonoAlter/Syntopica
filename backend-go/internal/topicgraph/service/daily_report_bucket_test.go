package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 泳道内主题分桶（add-intra-lane-theme-bucketing）───────────────────────────
//
// 分桶是纯逻辑 + 两次可替换的 LLM 调用（bucketChatFn），本文件全部用 fake
// LLM，不触网、不落库。

func bucketConfig() repository.PersistentTopicConfig {
	cfg := repository.DefaultPersistentTopicConfig()
	cfg.IntraBucketEnabled = true
	cfg.IntraBucketMinThreads = 6
	cfg.IntraBucketMinTags = 10
	return cfg
}

// fakeBucketChat answers phase 1 / phase 2 from a fixed (operation → content)
// map, recording every call.
func fakeBucketChat(t *testing.T, byOperation map[string]string) *[]string {
	t.Helper()
	var calls []string
	prev := bucketChatFn
	bucketChatFn = func(_ context.Context, _ string, _ string, _ *airouter.JSONSchema, operation string) (string, error) {
		calls = append(calls, operation)
		content, ok := byOperation[operation]
		if !ok {
			return "", errors.New("unexpected operation " + operation)
		}
		return content, nil
	}
	t.Cleanup(func() { bucketChatFn = prev })
	return &calls
}

func fakeBucketChatError(t *testing.T) {
	t.Helper()
	prev := bucketChatFn
	bucketChatFn = func(_ context.Context, _ string, _ string, _ *airouter.JSONSchema, operation string) (string, error) {
		return "", errors.New("llm down")
	}
	t.Cleanup(func() { bucketChatFn = prev })
}

// bucketThreads builds n threads with one tag each (ids 1..n) plus titles.
func bucketThreads(n int) []repository.Thread {
	threads := make([]repository.Thread, 0, n)
	for i := 0; i < n; i++ {
		threads = append(threads, repository.Thread{
			Title:      "线索" + string(rune('A'+i)),
			Summary:    "摘要",
			TagIDs:     []uint{uint(i + 1)},
			Confidence: 0.9,
		})
	}
	return threads
}

func bucketTags(n int) []repository.TagInput {
	tags := make([]repository.TagInput, 0, n)
	for i := 0; i < n; i++ {
		tags = append(tags, repository.TagInput{ID: uint(i + 1), Label: "标签" + string(rune('A'+i))})
	}
	return tags
}

// TestPlanBucketCount covers tasks 2.1 / 白盒 B1-B4: k = clamp(round(lines/2.5),
// 2, 6) plus the kill switch and the OR-threshold boundaries.
func TestPlanBucketCount(t *testing.T) {
	cfg := bucketConfig()

	cases := []struct {
		name       string
		lines      int
		tags       int
		cfg        repository.PersistentTopicConfig
		wantK      int
		wantBucket bool
	}{
		{"2 lines 10 tags triggers (tag branch)", 2, 10, cfg, 2, true},
		{"7 lines → k=3", 7, 0, cfg, 3, true},
		{"9 lines → k=4", 9, 0, cfg, 4, true},
		{"10 lines → k=4", 10, 0, cfg, 4, true},
		{"17 lines → k clamped to 6", 17, 0, cfg, 6, true},
		{"25 lines → k clamped to 6", 25, 0, cfg, 6, true},
		{"6 lines hits the thread boundary exactly", 6, 0, cfg, 2, true},
		{"10 tags hits the tag boundary exactly", 3, 10, cfg, 2, true},
		{"9 tags and 3 lines stays below both", 3, 9, cfg, 1, false},
		{"4 lines 5 tags (narrative lane) never triggers", 4, 5, cfg, 1, false},
		{"kill switch off keeps a single section", 17, 18, repository.PersistentTopicConfig{IntraBucketEnabled: false}, 1, false},
		{"single line cannot be split", 1, 12, cfg, 1, false},
		{"zero lines never triggers", 0, 20, cfg, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k, trigger := planBucketCount(tc.lines, tc.tags, tc.cfg)
			assert.Equal(t, tc.wantBucket, trigger)
			assert.Equal(t, tc.wantK, k)
		})
	}

	// Zero-valued thresholds fall back to the seed defaults instead of
	// "everything triggers".
	empty := repository.PersistentTopicConfig{IntraBucketEnabled: true}
	k, trigger := planBucketCount(5, 5, empty)
	assert.False(t, trigger)
	assert.Equal(t, 1, k)
}

// TestBucketPhase1Parse covers tasks 2.2 / B5-B7: title validation, duplicate
// merge, out-of-range line ids, and the deterministic >k / <k convergence.
func TestBucketPhase1Parse(t *testing.T) {
	t.Run("parses names and section titles", func(t *testing.T) {
		names, err := parseBucketNames(`{"groups":[
			{"bucket_title":"工具调用","section_title":"Agent 工具链更新","line_ids":[0,1]},
			{"bucket_title":"Skills","section_title":"Skill 生态","line_ids":[2]}
		]}`, 3)
		require.NoError(t, err)
		require.Len(t, names, 2)
		assert.Equal(t, "工具调用", names[0].Title)
		assert.Equal(t, "Agent 工具链更新", names[0].SectionTitle)
		assert.Equal(t, 2, names[0].LineCount)
	})

	t.Run("drops blank titles and merges duplicates", func(t *testing.T) {
		names, err := parseBucketNames(`{"groups":[
			{"bucket_title":"  ","section_title":"x","line_ids":[0]},
			{"bucket_title":"工具调用","section_title":"A","line_ids":[0]},
			{"bucket_title":"工具调用","section_title":"B","line_ids":[1,2]},
			{"bucket_title":"安全","line_ids":[3]}
		]}`, 4)
		require.NoError(t, err)
		require.Len(t, names, 2)
		assert.Equal(t, "工具调用", names[0].Title)
		assert.Equal(t, 3, names[0].LineCount, "duplicate titles merge their line counts")
		assert.Equal(t, "A", names[0].SectionTitle, "first non-empty section title wins")
		assert.Equal(t, "安全", names[1].Title)
	})

	t.Run("ignores out-of-range line ids", func(t *testing.T) {
		names, err := parseBucketNames(`{"groups":[{"bucket_title":"A","line_ids":[0,99,-1]}]}`, 2)
		require.NoError(t, err)
		require.Len(t, names, 1)
		assert.Equal(t, 1, names[0].LineCount)
	})

	t.Run("invalid JSON errors", func(t *testing.T) {
		_, err := parseBucketNames("not json", 3)
		require.Error(t, err)
	})

	t.Run("more groups than k merges down to k", func(t *testing.T) {
		names := []bucketThemeName{
			{Title: "A", LineCount: 5},
			{Title: "B", LineCount: 4},
			{Title: "C", LineCount: 3},
			{Title: "D", LineCount: 2},
			{Title: "E", LineCount: 1},
			{Title: "F", LineCount: 1},
		}
		got := convergeBucketNames(names, 4)
		require.Len(t, got, 4)
		titles := make([]string, len(got))
		for i, n := range got {
			titles[i] = n.Title
		}
		assert.Equal(t, []string{"A", "B", "C", "D"}, titles, "the two smallest (F together with E) are merged away")
	})

	t.Run("fewer groups than k keeps the actual count", func(t *testing.T) {
		names := []bucketThemeName{{Title: "A", LineCount: 3}, {Title: "B", LineCount: 2}}
		got := convergeBucketNames(names, 4)
		require.Len(t, got, 2)
	})
}

// TestBucketPhase2Parse covers tasks 2.3: per-line assignment, duplicates keep
// the first occurrence, invalid indexes are dropped (→ repaired later).
func TestBucketPhase2Parse(t *testing.T) {
	groups, err := parseBucketAssignments(`{"assignments":[
		{"line_id":0,"bucket_index":0},
		{"line_id":1,"bucket_index":1},
		{"line_id":1,"bucket_index":0},
		{"line_id":9,"bucket_index":0},
		{"line_id":2,"bucket_index":7}
	]}`, 2, 4)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, []int{0}, groups[0], "duplicate line 1 keeps its first placement (bucket 1)")
	assert.Equal(t, []int{1}, groups[1])
	// line 2 has an out-of-range bucket index and is treated as uncovered.
	covered := append(append([]int{}, groups[0]...), groups[1]...)
	assert.NotContains(t, covered, 2)

	_, err = parseBucketAssignments("nope", 2, 4)
	require.Error(t, err)
}

// TestBucketCluster covers tasks 2.5 / B16: 3 sibling buckets sharing the
// cluster identity, tag-union derivation, section-title fallback chain.
func TestBucketCluster(t *testing.T) {
	calls := fakeBucketChat(t, map[string]string{
		bucketNamesOperation: `{"groups":[
			{"bucket_title":"工具调用","section_title":"Agent 工具链更新","line_ids":[0,1]},
			{"bucket_title":"Skill 生态","section_title":"","line_ids":[2,3]},
			{"bucket_title":"安全风控","section_title":"","line_ids":[4,5,6]}
		]}`,
		bucketAssignOperation: `{"assignments":[
			{"line_id":0,"bucket_index":0},{"line_id":1,"bucket_index":0},
			{"line_id":2,"bucket_index":1},{"line_id":3,"bucket_index":1},
			{"line_id":4,"bucket_index":2},{"line_id":5,"bucket_index":2},{"line_id":6,"bucket_index":2}
		]}`,
	})

	cluster := repository.ClusterGroup{GroupName: "AGENT编程工具相关", TagIDs: []uint{1, 2, 3, 4, 5, 6, 7}, Lane: "l1"}
	buckets, err := BucketCluster(context.Background(), cluster, bucketThreads(7), bucketTags(7), "AGENT编程工具相关", bucketConfig())
	require.NoError(t, err)
	require.Len(t, buckets, 3)
	assert.Equal(t, []string{bucketNamesOperation, bucketAssignOperation}, *calls)

	assert.Equal(t, "工具调用", buckets[0].Title)
	assert.Equal(t, "Agent 工具链更新", buckets[0].SectionTitle, "phase-1 section title is used when present")
	assert.Equal(t, []uint{1, 2}, buckets[0].TagIDs)
	require.Len(t, buckets[0].Threads, 2)

	assert.Equal(t, "Skill 生态", buckets[1].Title)
	assert.Equal(t, "线索C", buckets[1].SectionTitle, "missing section title falls back to the bucket's first thread title")
	assert.Equal(t, []uint{3, 4}, buckets[1].TagIDs)

	assert.Equal(t, []int{4, 5, 6}, buckets[2].LineIndexes)
	assert.Equal(t, []uint{5, 6, 7}, buckets[2].TagIDs)
}

// TestBucketClusterDegrade covers tasks 2.5 failure paths: any LLM error must
// return an error (caller keeps the single section) — never a partial split.
func TestBucketClusterDegrade(t *testing.T) {
	t.Run("phase 1 error", func(t *testing.T) {
		fakeBucketChatError(t)
		buckets, err := BucketCluster(context.Background(),
			repository.ClusterGroup{GroupName: "g", TagIDs: []uint{1}}, bucketThreads(7), bucketTags(7), "AGENT编程工具相关", bucketConfig())
		require.Error(t, err)
		assert.Nil(t, buckets)
	})

	t.Run("phase 2 error", func(t *testing.T) {
		prev := bucketChatFn
		bucketChatFn = func(_ context.Context, _ string, _ string, _ *airouter.JSONSchema, operation string) (string, error) {
			if operation == bucketNamesOperation {
				return `{"groups":[{"bucket_title":"A","line_ids":[0,1]},{"bucket_title":"B","line_ids":[2,3]}]}`, nil
			}
			return "", errors.New("phase 2 down")
		}
		t.Cleanup(func() { bucketChatFn = prev })

		buckets, err := BucketCluster(context.Background(),
			repository.ClusterGroup{GroupName: "g", TagIDs: []uint{1}}, bucketThreads(7), bucketTags(7), "AGENT编程工具相关", bucketConfig())
		require.Error(t, err)
		assert.Nil(t, buckets)
	})

	t.Run("converged to one group keeps the single section", func(t *testing.T) {
		fakeBucketChat(t, map[string]string{
			bucketNamesOperation:  `{"groups":[{"bucket_title":"唯一方向","line_ids":[0,1,2]},{"bucket_title":"空方向","line_ids":[]}]}`,
			bucketAssignOperation: `{"assignments":[{"line_id":0,"bucket_index":0},{"line_id":1,"bucket_index":0},{"line_id":2,"bucket_index":0},{"line_id":3,"bucket_index":0},{"line_id":4,"bucket_index":0},{"line_id":5,"bucket_index":0},{"line_id":6,"bucket_index":0}]}`,
		})
		buckets, err := BucketCluster(context.Background(),
			repository.ClusterGroup{GroupName: "g", TagIDs: []uint{1}}, bucketThreads(7), bucketTags(7), "AGENT编程工具相关", bucketConfig())
		require.NoError(t, err)
		assert.Nil(t, buckets, "single surviving group is a legal degradation, not a split")
	})
}

// TestBucketClusterFallsBackToLaneLabel pins the last link of the title
// fallback chain: an empty phase-1 title AND untitled threads fall back to the
// lane label instead of borrowing another bucket's content.
func TestBucketClusterFallsBackToLaneLabel(t *testing.T) {
	fakeBucketChat(t, map[string]string{
		bucketNamesOperation: `{"groups":[{"bucket_title":"A","line_ids":[0,1]},{"bucket_title":"B","line_ids":[2,3]}]}`,
		bucketAssignOperation: `{"assignments":[
			{"line_id":0,"bucket_index":0},{"line_id":1,"bucket_index":0},
			{"line_id":2,"bucket_index":1},{"line_id":3,"bucket_index":1},
			{"line_id":4,"bucket_index":0},{"line_id":5,"bucket_index":1},{"line_id":6,"bucket_index":0}
		]}`,
	})
	threads := bucketThreads(7)
	for i := range threads {
		threads[i].Title = ""
	}
	buckets, err := BucketCluster(context.Background(),
		repository.ClusterGroup{GroupName: "AGENT编程工具相关", TagIDs: []uint{1}}, threads, bucketTags(7), "AGENT编程工具相关", bucketConfig())
	require.NoError(t, err)
	require.Len(t, buckets, 2)
	assert.Equal(t, "AGENT编程工具相关", buckets[0].SectionTitle)
	assert.Equal(t, "AGENT编程工具相关", buckets[1].SectionTitle)
	assert.False(t, strings.Contains(buckets[0].SectionTitle, "线索"))
}

// TestBucketClusterTagUnionAndFallback covers the 前置 variants: duplicate tags
// across a bucket's threads are deduped, and a bucket whose threads carry no tag
// ids falls back to the cluster's tag set (so the section stays assignable
// instead of becoming an embedding-less orphan).
func TestBucketClusterTagUnionAndFallback(t *testing.T) {
	fakeBucketChat(t, map[string]string{
		bucketNamesOperation: `{"groups":[{"bucket_title":"有标签","line_ids":[0,2,4]},{"bucket_title":"无标签","line_ids":[1,3,5]}]}`,
		bucketAssignOperation: `{"assignments":[
			{"line_id":0,"bucket_index":0},{"line_id":1,"bucket_index":1},
			{"line_id":2,"bucket_index":0},{"line_id":3,"bucket_index":1},
			{"line_id":4,"bucket_index":0},{"line_id":5,"bucket_index":1}
		]}`,
	})
	threads := []repository.Thread{
		{Title: "t0", TagIDs: []uint{1, 2}},
		{Title: "t1", TagIDs: nil},
		{Title: "t2", TagIDs: []uint{2}},
		{Title: "t3", TagIDs: []uint{}},
		{Title: "t4", TagIDs: []uint{3}},
		{Title: "t5", TagIDs: nil},
	}
	cluster := repository.ClusterGroup{GroupName: "泳道", TagIDs: []uint{1, 2, 3, 4}}
	buckets, err := BucketCluster(context.Background(), cluster, threads, bucketTags(4), "泳道", bucketConfig())
	require.NoError(t, err)
	require.Len(t, buckets, 2)

	assert.Equal(t, []uint{1, 2, 3}, buckets[0].TagIDs, "tag union dedupes across the bucket's threads")
	assert.Equal(t, []uint{1, 2, 3, 4}, buckets[1].TagIDs, "tag-less bucket falls back to the cluster tag set")
}

// TestBucketTitlesAreNotTruncated pins the 超长 variant: an overlong direction
// name is stored as returned (column budget 200); the frontend ellipsizes it.
func TestBucketTitlesAreNotTruncated(t *testing.T) {
	long := strings.Repeat("长", 20)
	names, err := parseBucketNames(`{"groups":[{"bucket_title":"`+long+`","section_title":"`+strings.Repeat("题", 40)+`"}]}`, 3)
	require.NoError(t, err)
	require.Len(t, names, 1)
	assert.Equal(t, long, names[0].Title)
	assert.Equal(t, 40, len([]rune(names[0].SectionTitle)), "section title is not cut at the prompt budget")
}
