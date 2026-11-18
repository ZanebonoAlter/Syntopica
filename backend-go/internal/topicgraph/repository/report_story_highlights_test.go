package repository

// 「今日重点」候选排序与快照（retire-story-adopt-event-metrics 2.2）。
// 事件线候选 / 热度排序整体退役，候选源改为当天同事件聚簇。

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRankEventClusterCandidatesOrderAndLimit：簇内文章数降序 → 簇内 event
// 标签数降序 → 代表标签 id 升序；不按四舍五入后的篇数排序；最多 8 条。
func TestRankEventClusterCandidatesOrderAndLimit(t *testing.T) {
	clusters := []EventCluster{
		{ClusterID: 30, TagIDs: []uint{30}, ArticleCount: 5},
		{ClusterID: 10, TagIDs: []uint{10}, ArticleCount: 5},
		{ClusterID: 20, TagIDs: []uint{20, 21, 22}, ArticleCount: 5},
		{ClusterID: 40, TagIDs: []uint{40}, ArticleCount: 9},
	}
	ranked := RankEventClusterCandidates(clusters, 8)
	require.Len(t, ranked, 4)
	require.Equal(t, uint(40), ranked[0].ClusterID, "篇数最多者居首")
	// 同篇数：标签数多者优先（20 有 3 个标签），其余按代表标签 id 升序。
	require.Equal(t, uint(20), ranked[1].ClusterID)
	require.Equal(t, uint(10), ranked[2].ClusterID)
	require.Equal(t, uint(30), ranked[3].ClusterID)

	// 上限 8：第 9 名不进候选，且不按 section 数 / 泳道热度替代；
	// 第 9/10 簇标签数最多（4 标签 vs 其余 1 标签）也不进——标签数只是
	// 同篇数时的次级键，不能凭标签数挤进候选（delta spec Scenario
	// 「候选按簇内文章数截断」的字面覆盖）。
	many := make([]EventCluster, 0, 10)
	for i := 1; i <= 10; i++ {
		tags := []uint{uint(i)}
		if i >= 9 {
			tags = []uint{uint(i), uint(i) + 100, uint(i) + 200, uint(i) + 300}
		}
		many = append(many, EventCluster{ClusterID: uint(i), TagIDs: tags, ArticleCount: 100 - i})
	}
	ranked = RankEventClusterCandidates(many, 8)
	require.Len(t, ranked, 8)
	require.Equal(t, uint(1), ranked[0].ClusterID)
	require.Equal(t, uint(8), ranked[7].ClusterID)

	// 零候选：空列表（调用方据此走标签回退）。
	require.Empty(t, RankEventClusterCandidates(nil, 8))
}

// TestEventHighlightSnapshot：快照字段由服务端补齐——cluster_id、篇数、
// 口径中文标签、数据截至日与研究时点；模型不参与这些数值。
func TestEventHighlightSnapshot(t *testing.T) {
	at := time.Date(2026, 11, 6, 9, 30, 0, 0, time.UTC)
	cluster := EventCluster{
		ClusterID: 11, TagIDs: []uint{11, 12}, ArticleCount: 4,
		Mode: EventClusterModeSubjectApprox, DataAsOf: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC), ResearchAt: at,
	}
	highlight := EventHighlight(cluster, "标题", "理由", []uint{11})
	require.NotNil(t, highlight.ClusterID)
	require.Equal(t, uint(11), *highlight.ClusterID)
	require.NotNil(t, highlight.ArticleCount)
	require.Equal(t, 4, *highlight.ArticleCount)
	require.Equal(t, "主体近似", highlight.ClusterMode, "降级口径必须如实标注，不得冒充向量聚簇")
	require.Equal(t, "2026-11-06", highlight.DataAsOf)
	require.NotNil(t, highlight.ResearchAt)
	require.Equal(t, at, *highlight.ResearchAt)
	require.Equal(t, []uint{11}, highlight.TagIDs)

	// 0 篇是合法快照值（不是缺省）。
	zero := EventHighlight(EventCluster{ClusterID: 12, Mode: EventClusterModeVector, DataAsOf: at}, "t", "r", []uint{12})
	require.NotNil(t, zero.ArticleCount)
	require.Zero(t, *zero.ArticleCount)
	require.Equal(t, "向量聚簇", zero.ClusterMode)

	// 口径标签映射。
	for mode, want := range map[string]string{
		EventClusterModeVector:           "向量聚簇",
		EventClusterModeSemanticFallback: "语义向量兜底",
		EventClusterModeSubjectApprox:    "主体近似",
	} {
		require.Equal(t, want, EventClusterModeLabel(mode), fmt.Sprintf("mode=%s", mode))
	}
}
