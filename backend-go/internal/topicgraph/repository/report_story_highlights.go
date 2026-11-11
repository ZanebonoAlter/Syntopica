package repository

import (
	"encoding/json"
	"time"

	"syntopica-backend/internal/models"
)

// 今日重点（highlights）的读/写与聚簇快照解析
// （retire-story-adopt-event-metrics D13/D14）。
//
// 候选源已从「事件线 Top 8」换成「当天同事件聚簇 Top 8」：聚簇在 service 层
// 现算（不落库），本文件只负责快照落库、数组归一与 cluster_id 的同版块一跳
// 解析。事件线候选 / 昨日重点徽标 / story_heat 快照整体退役。

// resolveReportHighlightClusters validates every highlight's cluster_id with ONE
// hop in the same board: the id must resolve to an event tag that is a member of
// this board. Broken / cross-board / retired references are nulled out (never
// rewritten in the DB — the stored JSON keeps its generation-time snapshot).
func (r *TopicGraphRepository) resolveReportHighlightClusters(report *BoardDailyReport) error {
	normalizeHighlightsArray(&report.Highlights)
	var highlights []Highlight
	if err := json.Unmarshal(report.Highlights, &highlights); err != nil {
		return nil // unparseable historical payload: keep the normalized [] as-is
	}
	ids := make([]uint, 0, len(highlights))
	for _, highlight := range highlights {
		if highlight.ClusterID != nil {
			ids = appendUniqueUint(ids, *highlight.ClusterID)
		}
	}
	valid := make(map[uint]bool, len(ids))
	if len(ids) > 0 {
		var rows []struct {
			ID uint
		}
		if err := r.db.Table("topic_tags AS tt").
			Select("tt.id").
			Joins("JOIN topic_tag_board_labels AS tbl ON tbl.topic_tag_id = tt.id AND tbl.semantic_board_id = ?", report.SemanticBoardID).
			Where("tt.id IN ? AND tt.category = ?", ids, models.TagCategoryEvent).
			Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			valid[row.ID] = true
		}
	}
	for i := range highlights {
		if highlights[i].ClusterID != nil && !valid[*highlights[i].ClusterID] {
			highlights[i].ClusterID = nil
		}
	}
	if highlights == nil {
		highlights = []Highlight{}
	}
	payload, err := json.Marshal(highlights)
	if err != nil {
		return err
	}
	report.Highlights = payload
	return nil
}

// FinalizeReportHighlights is one conditional SQL write (an implicit short
// transaction). No external calls occur inside it. The revision is read back
// by SaveReport, including PostgreSQL timestamp precision.
func (r *TopicGraphRepository) FinalizeReportHighlights(report *BoardDailyReport, highlights []Highlight) (bool, error) {
	if highlights == nil {
		highlights = []Highlight{}
	}
	payload, err := json.Marshal(highlights)
	if err != nil {
		return false, err
	}
	title := "日报 " + report.PeriodDate.Format("2006-01-02")
	summary := ""
	if len(highlights) > 0 {
		title = highlights[0].Title
		summary = highlights[0].Reason
	}
	result := r.db.Model(&BoardDailyReport{}).Where("id = ? AND updated_at = ?", report.ID, report.UpdatedAt).Updates(map[string]any{"highlights": JSON(payload), "title": title, "summary": summary})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	report.Highlights = payload
	report.Title = title
	report.Summary = summary
	return true, nil
}

// EventHighlight builds the persisted highlight snapshot of one cluster:
// cluster_id (representative tag id), content-identity article count, the real
// clustering mode label, the data cutoff day and the research instant. All of
// these values are server-computed — the model never supplies numbers.
func EventHighlight(cluster EventCluster, title, reason string, tags []uint) Highlight {
	id := cluster.ClusterID
	count := cluster.ArticleCount
	researchAt := cluster.ResearchAt
	return Highlight{
		ClusterID:    &id,
		ArticleCount: &count,
		ClusterMode:  EventClusterModeLabel(cluster.Mode),
		DataAsOf:     cluster.DataAsOf.Format(time.DateOnly),
		ResearchAt:   &researchAt,
		Title:        title,
		Reason:       reason,
		TagIDs:       tags,
	}
}
