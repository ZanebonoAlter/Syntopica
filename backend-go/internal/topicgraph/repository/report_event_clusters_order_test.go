package repository

// 完整候选序（不截断）与既有候选序共用同一比较器
// （rank-report-highlights-by-board-relevance 2.1 的仓储侧）。

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOrderEventClusterCandidatesIsFullOrder：组池用的是完整候选序（不截断）——
// 10 个簇全返回，排序键与 RankEventClusterCandidates 完全一致（截断版只是它的
// 前 8，既有候选序的行为未变）。
func TestOrderEventClusterCandidatesIsFullOrder(t *testing.T) {
	clusters := make([]EventCluster, 0, 10)
	for i := 1; i <= 10; i++ {
		clusters = append(clusters, EventCluster{ClusterID: uint(i), TagIDs: []uint{uint(i)}, ArticleCount: 100 - i})
	}
	ordered := OrderEventClusterCandidates(clusters)
	require.Len(t, ordered, 10, "不截断")
	ids := make([]uint, 0, len(ordered))
	for _, cluster := range ordered {
		ids = append(ids, cluster.ClusterID)
	}
	require.Equal(t, []uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, ids)

	ranked := RankEventClusterCandidates(clusters, 8)
	require.Len(t, ranked, 8)
	require.Equal(t, ids[:8], []uint{
		ranked[0].ClusterID, ranked[1].ClusterID, ranked[2].ClusterID, ranked[3].ClusterID,
		ranked[4].ClusterID, ranked[5].ClusterID, ranked[6].ClusterID, ranked[7].ClusterID,
	})
}
