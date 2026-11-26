package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── 泳道内主题分桶：确定性校验器（add-intra-lane-theme-bucketing design D5）───
//
// repairBuckets 的「最近组」是 tag 集合 Jaccard 重叠（无 embedding 时的确定性
// 语义代理）；无重叠时才退回「行数最小组」，这与 test-cases B9「无嵌入时按组
// 大小最小者」一致。

// lineTags builds n synthetic threads, line i carrying tag id i+1.
func lineTags(n int) [][]uint {
	out := make([][]uint, n)
	for i := range out {
		out[i] = []uint{uint(i + 1)}
	}
	return out
}

func repairNames(titles ...string) []bucketThemeName {
	out := make([]bucketThemeName, len(titles))
	for i, t := range titles {
		out[i] = bucketThemeName{Title: t}
	}
	return out
}

// TestRepairBuckets covers tasks 2.4 / B9-B15: uncovered merge, duplicate first,
// singleton merge, empty-group drop, >6 merge, degenerate fallback.
func TestRepairBuckets(t *testing.T) {
	t.Run("uncovered line with no tag overlap joins the smallest group", func(t *testing.T) {
		// 5 lines / 5 distinct tags; line 4 is uncovered and shares no tag.
		tags := lineTags(5)
		_, groups, degenerate := repairBuckets(repairNames("A", "B"), [][]int{{0, 1}, {2, 3}}, tags)
		require.False(t, degenerate)
		require.Len(t, groups, 2)
		// Both groups have 2 lines; ties go to the earliest group.
		assert.Equal(t, []int{0, 1, 4}, groups[0])
		assert.Equal(t, []int{2, 3}, groups[1])
	})

	t.Run("uncovered line joins the semantically nearest group, not the smallest", func(t *testing.T) {
		tags := [][]uint{{1, 2}, {1, 2}, {3, 4, 5}, {3, 4, 5}, {3, 4, 5}, {3}}
		_, groups, degenerate := repairBuckets(repairNames("A", "B"), [][]int{{0, 1}, {2, 3, 4}}, tags)
		require.False(t, degenerate)
		require.Len(t, groups, 2)
		assert.Equal(t, []int{0, 1}, groups[0])
		assert.Equal(t, []int{2, 3, 4, 5}, groups[1],
			"line 5 shares a tag with B → merged there even though A is smaller")
	})

	t.Run("duplicate line keeps the first occurrence", func(t *testing.T) {
		_, groups, degenerate := repairBuckets(repairNames("A", "B"), [][]int{{0, 1}, {0, 2, 3}}, lineTags(4))
		require.False(t, degenerate)
		assert.Equal(t, []int{0, 1}, groups[0])
		assert.Equal(t, []int{2, 3}, groups[1], "line 0 stays in its first group instead of being counted twice")
	})

	t.Run("singleton group merges into the semantically nearest group", func(t *testing.T) {
		tags := [][]uint{{1, 2}, {1, 2}, {3, 4}, {3, 4}, {1}}
		_, groups, degenerate := repairBuckets(repairNames("A", "B", "C"), [][]int{{0, 1}, {2, 3}, {4}}, tags)
		require.False(t, degenerate)
		require.Len(t, groups, 2, "the singleton direction is absorbed")
		assert.Equal(t, []int{0, 1, 4}, groups[0], "line 4 (tag 1) overlaps A")
		assert.Equal(t, []int{2, 3}, groups[1])
	})

	t.Run("empty group is dropped and produces no section", func(t *testing.T) {
		names, groups, degenerate := repairBuckets(repairNames("A", "B", "C"), [][]int{{0, 1}, {}, {2, 3}}, lineTags(4))
		require.False(t, degenerate)
		require.Len(t, groups, 2)
		assert.Equal(t, []string{"A", "C"}, []string{names[0].Title, names[1].Title})
	})

	t.Run("degenerate when only one group survives", func(t *testing.T) {
		_, groups, degenerate := repairBuckets(repairNames("A", "B"), [][]int{{0, 1, 2, 3}, {}}, lineTags(4))
		assert.True(t, degenerate)
		require.Len(t, groups, 1)
	})

	t.Run("more than six groups merge down to six", func(t *testing.T) {
		many := make([][]int, 8)
		titles := make([]string, 8)
		for i := range many {
			many[i] = []int{i * 2, i*2 + 1}
			titles[i] = string(rune('A' + i))
		}
		_, groups, degenerate := repairBuckets(repairNames(titles...), many, lineTags(16))
		require.False(t, degenerate)
		assert.Len(t, groups, bucketMaxThemeGroups)
	})

	t.Run("no usable assignment degenerates to a single group", func(t *testing.T) {
		_, groups, degenerate := repairBuckets(repairNames("A"), [][]int{{}}, lineTags(3))
		assert.True(t, degenerate)
		require.Len(t, groups, 1)
		assert.Equal(t, []int{0, 1, 2}, groups[0])
	})
}
