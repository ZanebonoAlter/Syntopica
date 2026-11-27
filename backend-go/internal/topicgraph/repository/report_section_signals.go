package repository

// 本期正文板块的「贴题信号」只读投影
// （rank-report-highlights-by-board-relevance 1.1）。
//
// 日报尾部生成「今日重点」时需要知道「这一簇标签落在本期哪些正文板块、那些
// 板块是哪个泳道产出的、话题当期是什么状态」——这些快照在 SaveReport 事务内已经
// 落库（lane_tier + topic_status_at_report），这里只做一次轻量只读投影供服务层
// 定贴题档：不新算向量、不写任何快照、不回填历史（flow 红线 5）。
//
// 刻意不走 GetReportByID：它会 Preload threads、折叠文章副本并当场改写
// report.Highlights，副作用不可接受（design D8）。

import (
	"fmt"
)

// ReportSectionSignal is one section's read-only relevance projection: the tags
// it holds, the lane that produced its topic assignment, and the topic's status
// at report time. Both snapshots are read exactly as stored — never derived,
// never backfilled.
type ReportSectionSignal struct {
	ID           uint
	ClusterIndex int
	BucketIndex  int
	TagIDs       []uint
	// LaneTier is l1_direct / l2_llm / l3_new / watch_keyword / watch_sentence;
	// historical NULL reads as "" (no lane tier).
	LaneTier string
	// TopicStatusAtReport is active / candidate at report time; historical NULL
	// reads as "" and is treated as non-candidate (missing must not punish).
	TopicStatusAtReport string
}

// ListReportSectionSignals reads this report's section relevance signals: one row
// per section, ordered by (cluster_index, bucket_index) so the projection is
// deterministic. cluster_tag_ids is a jsonb NUMBER array, decoded as []uint (never
// compared as jsonb text: `?` on numeric elements is always false).
func (r *TopicGraphRepository) ListReportSectionSignals(reportID uint) ([]ReportSectionSignal, error) {
	var rows []struct {
		ID                  uint
		ClusterIndex        int
		BucketIndex         int
		ClusterTagIDs       JSON
		LaneTier            *string
		TopicStatusAtReport *string
	}
	if err := r.db.Table("daily_report_sections").
		Select("id, cluster_index, bucket_index, cluster_tag_ids, lane_tier, topic_status_at_report").
		Where("report_id = ?", reportID).
		Order("cluster_index ASC, bucket_index ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("highlight relevance: report %d section signals: %w", reportID, err)
	}
	out := make([]ReportSectionSignal, 0, len(rows))
	for _, row := range rows {
		signal := ReportSectionSignal{
			ID:           row.ID,
			ClusterIndex: row.ClusterIndex,
			BucketIndex:  row.BucketIndex,
			TagIDs:       jsonUintIDs(row.ClusterTagIDs),
		}
		if row.LaneTier != nil {
			signal.LaneTier = *row.LaneTier
		}
		if row.TopicStatusAtReport != nil {
			signal.TopicStatusAtReport = *row.TopicStatusAtReport
		}
		out = append(out, signal)
	}
	return out, nil
}
