package repository

// 版块贴题信号查询（rank-report-highlights-by-board-relevance 1.1）——repository
// 包禁 SQLite（standard/backend/testing.md 红线），故走 testcontainer PG 真跑这条
// SQL：cluster_tag_ids 是 jsonb 数字数组，lane_tier / topic_status_at_report 在
// 历史行上是 NULL（PG 严格类型，SQLite 测了等于没测）。

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// TestListReportSectionSignals：按 report id 读本期板块贴题信号——字段齐全、
// (cluster_index, bucket_index) 定序、jsonb 数字数组解成 []uint、历史 NULL 读成
// 空串（不伪造状态，也不回填）。
func TestListReportSectionSignals(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	day := NormalizeReportDate(time.Now())
	reportID := seedTestReport(t, db, boardID, day)
	otherReportID := seedTestReport(t, db, boardID, day.AddDate(0, 0, -1))

	// 故意乱序插入，验证查询自己定序。
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_index, bucket_index, cluster_label, cluster_tag_ids, article_count, best_tier, avg_score, embedding, lane_tier, topic_status_at_report)
		VALUES (?, 1, 1, 'b', '[304968, 304637]', 2, 1, 0.5, '[0]', 'l1_direct', 'active')
	`, reportID).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_index, bucket_index, cluster_label, cluster_tag_ids, article_count, best_tier, avg_score, embedding, lane_tier, topic_status_at_report)
		VALUES (?, 0, 0, 'a', '[304290]', 6, 3, 0.6, '[0]', 'l2_llm', 'candidate')
	`, reportID).Error)
	// 历史行：无泳道档、无话题状态快照、标签为空数组。
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_index, bucket_index, cluster_label, cluster_tag_ids, article_count, best_tier, avg_score, embedding)
		VALUES (?, 1, 0, 'legacy', '[]', 1, 1, 0.5, '[0]')
	`, reportID).Error)
	// 别的日报的板块：不得串进来。
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_index, bucket_index, cluster_label, cluster_tag_ids, article_count, best_tier, avg_score, embedding, lane_tier, topic_status_at_report)
		VALUES (?, 0, 0, 'other', '[999]', 1, 1, 0.5, '[0]', 'l1_direct', 'active')
	`, otherReportID).Error)

	signals, err := repo.ListReportSectionSignals(reportID)
	require.NoError(t, err)
	require.Len(t, signals, 3, "只读本报告，且返回全部板块（不截断）")
	require.NotEmpty(t, signals[0].ID)

	// (cluster_index, bucket_index) 升序：(0,0) → (1,0) → (1,1)。
	require.Equal(t, []int{0, 1, 1}, []int{signals[0].ClusterIndex, signals[1].ClusterIndex, signals[2].ClusterIndex})
	require.Equal(t, []int{0, 0, 1}, []int{signals[0].BucketIndex, signals[1].BucketIndex, signals[2].BucketIndex})

	// jsonb 数字数组 → []uint（不是字符串，也不依赖 jsonb `?` 文本比较）。
	require.Equal(t, []uint{304290}, signals[0].TagIDs)
	require.Equal(t, "l2_llm", signals[0].LaneTier)
	require.Equal(t, "candidate", signals[0].TopicStatusAtReport)
	require.Equal(t, []uint{304968, 304637}, signals[2].TagIDs, "标签顺序按存库原样，不做重排")
	require.Equal(t, "l1_direct", signals[2].LaneTier)

	// 历史 NULL：读成空串（缺失不惩罚），不回填、不报错。
	require.Empty(t, signals[1].TagIDs)
	require.Empty(t, signals[1].LaneTier)
	require.Empty(t, signals[1].TopicStatusAtReport)

	// 这些读出来的信号能直接定贴题档：候选话题落中档候选、近档落 0、无对应板块落 5
	// （档位映射的表驱动测试在 service 层的 daily_report_highlight_relevance_test.go）。
	require.Equal(t, "candidate", signals[0].TopicStatusAtReport, "候选话题状态要能读出")
	require.Equal(t, "l1_direct", signals[2].LaneTier, "近档泳道档位要能读出")

	// 没有板块的日报：空列表（非 nil 错误），调用方据此维持既有口径。
	emptyReportID := seedTestReport(t, db, boardID, day.AddDate(0, 0, -2))
	empty, err := repo.ListReportSectionSignals(emptyReportID)
	require.NoError(t, err)
	require.Empty(t, empty)
}
