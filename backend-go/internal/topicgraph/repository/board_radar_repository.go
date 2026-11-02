package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/models"
)

// ── 今日雷达聚合（rework-workshop-into-radar-and-desk 8.2）──────────────────
//
// 单次端点：活跃版块 × 最新日报锚点 × 近 14 天。口径复用既有
// GetBoardLaneDynamics（泳道/分区/线索）与 board_daily_reports（最新日报），
// 本文件只做窗口对齐与求和，不复制匹配/日报统计公式。

// RadarLane is one of the board's top lanes with its 14-day section density.
type RadarLane struct {
	TopicID         uint   `json:"topic_id"`
	Label           string `json:"label"`
	SectionCount14d int    `json:"section_count_14d"`
	ThreadCount14d  int    `json:"thread_count_14d"`
	Counts          []int  `json:"counts"` // 14 days, oldest → newest
}

// RadarBoardRow is one /semantic-boards/overview row.
type RadarBoardRow struct {
	BoardID       uint        `json:"board_id"`
	Label         string      `json:"label"`
	TagCount      int64       `json:"tag_count"`
	ReportDate    string      `json:"report_date"` // latest report period_date (YYYY-MM-DD), "" when none
	ReportTitle   string      `json:"report_title"`
	ReportArticle int         `json:"report_article_count"`
	ReportEvent   int         `json:"report_event_count"`
	AnchorDate    string      `json:"anchor_date"`
	TodaySections int         `json:"today_sections"`
	Last3Sections int         `json:"last3_sections"`
	Prev3Sections int         `json:"prev3_sections"`
	Attention     int         `json:"attention"`
	DataDelayed   bool        `json:"data_delayed"`
	Days          []int       `json:"days"` // 14 days, oldest → newest
	Lanes         []RadarLane `json:"lanes"`
}

// RadarOverview is the single-shot radar payload.
type RadarOverview struct {
	Date   string          `json:"date"`
	Boards []RadarBoardRow `json:"boards"`
}

// GetBoardRadarOverview aggregates all active boards. A board whose lane
// dynamics query fails degrades to zero counts instead of failing the page
// (spec: 无数据版块返回零值). ctx carries the 5s handler budget; rows that
// do not make it are still returned as zero with data_delayed=true.
func (r *TopicGraphRepository) GetBoardRadarOverview(ctx context.Context) (*RadarOverview, error) {
	var labels []models.SemanticLabel
	if err := r.db.WithContext(ctx).
		Where("label_type = ? AND status = ?", "board", "active").
		Order("display_order ASC, id ASC").
		Find(&labels).Error; err != nil {
		return nil, fmt.Errorf("radar: list active boards: %w", err)
	}
	type boardBrief struct {
		ID       uint
		Label    string
		TagCount int64
	}
	boards := make([]boardBrief, 0, len(labels))
	ids := make([]uint, 0, len(labels))
	for _, label := range labels {
		boards = append(boards, boardBrief{ID: label.ID, Label: label.Label})
		ids = append(ids, label.ID)
	}
	if len(ids) > 0 {
		var counts []struct {
			SemanticBoardID uint  `gorm:"column:semantic_board_id"`
			Count           int64 `gorm:"column:count"`
		}
		if err := r.db.WithContext(ctx).Table("topic_tag_board_labels").
			Select("semantic_board_id, COUNT(DISTINCT topic_tag_id) AS count").
			Where("semantic_board_id IN ?", ids).
			Group("semantic_board_id").
			Scan(&counts).Error; err != nil {
			return nil, fmt.Errorf("radar: board tag counts: %w", err)
		}
		byID := make(map[uint]int64, len(counts))
		for _, c := range counts {
			byID[c.SemanticBoardID] = c.Count
		}
		for i := range boards {
			boards[i].TagCount = byID[boards[i].ID]
		}
	}

	out := &RadarOverview{Boards: make([]RadarBoardRow, len(boards))}
	for i, b := range boards {
		out.Boards[i] = RadarBoardRow{
			BoardID:  b.ID,
			Label:    b.Label,
			TagCount: b.TagCount,
			Days:     make([]int, LaneDynamicsDefaultDays),
			Lanes:    []RadarLane{},
		}
	}
	if len(boards) == 0 {
		// 空版块也要走 finalize（date 回退今天，不冒空串——tasks 9.4 测试抓出
		// early return 曾绕过回退逻辑，雷达空态报头拿到空日期）。
		return finalizeRadar(out), nil
	}

	type result struct {
		index int
		row   RadarBoardRow
	}
	ch := make(chan result, len(boards))
	for i, b := range boards {
		go func(index int, brief boardBrief) {
			row := r.buildRadarRow(brief.ID)
			row.BoardID = brief.ID
			row.Label = brief.Label
			row.TagCount = brief.TagCount
			ch <- result{index: index, row: row}
		}(i, b)
	}

	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	done := make(map[int]bool, len(boards))
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for len(done) < len(boards) {
		select {
		case res := <-ch:
			done[res.index] = true
			out.Boards[res.index] = res.row
		case <-timer.C:
			for i := range out.Boards {
				if !done[i] {
					out.Boards[i].DataDelayed = true
					out.Boards[i].Days = make([]int, LaneDynamicsDefaultDays)
					out.Boards[i].Lanes = []RadarLane{}
				}
			}
			// drain in background; ch is buffered so senders never block
			return finalizeRadar(out), nil
		}
	}
	return finalizeRadar(out), nil
}

func (r *TopicGraphRepository) buildRadarRow(boardID uint) RadarBoardRow {
	row := RadarBoardRow{
		Days:  make([]int, LaneDynamicsDefaultDays),
		Lanes: []RadarLane{},
	}
	brief, err := r.GetLatestReportBrief(boardID)
	if err == nil && brief != nil {
		row.ReportDate = brief.PeriodDate
		row.ReportTitle = brief.Title
		row.ReportArticle = brief.ArticleCount
		row.ReportEvent = brief.EventCount
	}

	dynamics, err := r.GetBoardLaneDynamics(boardID, LaneDynamicsDefaultDays)
	if err != nil || dynamics == nil || !dynamics.HasReports {
		return row
	}
	anchor := dynamicsAnchor(dynamics)
	if anchor == "" {
		return row
	}
	row.AnchorDate = anchor
	axis := radarAxis(anchor)
	index := make(map[string]int, len(axis))
	for i, date := range axis {
		index[date] = i
	}

	for _, lane := range dynamics.Lanes {
		counts := make([]int, LaneDynamicsDefaultDays)
		for _, day := range lane.Timeline {
			i, ok := index[day.Date]
			if !ok {
				continue
			}
			counts[i] = len(day.Sections)
			row.Days[i] += counts[i]
		}
		row.Lanes = append(row.Lanes, RadarLane{
			TopicID:         lane.TopicID,
			Label:           lane.Label,
			SectionCount14d: lane.SectionCount14d,
			ThreadCount14d:  lane.ThreadCount14d,
			Counts:          counts,
		})
	}
	sort.SliceStable(row.Lanes, func(i, j int) bool {
		return row.Lanes[i].ThreadCount14d > row.Lanes[j].ThreadCount14d
	})
	if len(row.Lanes) > 3 {
		row.Lanes = row.Lanes[:3]
	}
	last := len(row.Days) - 1
	row.TodaySections = row.Days[last]
	row.Last3Sections = sumRadar(row.Days[last-2:])
	row.Prev3Sections = sumRadar(row.Days[last-5 : last-2])
	// 关注度 = 今日×2 + 近3日 + 近3日相对前3日的净增（可为负，地板 0）。
	row.Attention = row.TodaySections*2 + row.Last3Sections + (row.Last3Sections - row.Prev3Sections)
	if row.Attention < 0 {
		row.Attention = 0
	}
	return row
}

func dynamicsAnchor(resp *LaneDynamicsResponse) string {
	var max string
	for _, lane := range resp.Lanes {
		for _, day := range lane.Timeline {
			if day.Date > max {
				max = day.Date
			}
		}
	}
	return max
}

func radarAxis(anchor string) []string {
	t, err := time.ParseInLocation("2006-01-02", anchor, time.Local)
	if err != nil {
		return []string{}
	}
	axis := make([]string, LaneDynamicsDefaultDays)
	for i := 0; i < LaneDynamicsDefaultDays; i++ {
		axis[i] = t.AddDate(0, 0, i-(LaneDynamicsDefaultDays-1)).Format("2006-01-02")
	}
	return axis
}

func sumRadar(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

func finalizeRadar(out *RadarOverview) *RadarOverview {
	sort.SliceStable(out.Boards, func(i, j int) bool {
		if out.Boards[i].Attention != out.Boards[j].Attention {
			return out.Boards[i].Attention > out.Boards[j].Attention
		}
		return out.Boards[i].BoardID < out.Boards[j].BoardID
	})
	latest := ""
	for _, row := range out.Boards {
		if row.ReportDate > latest {
			latest = row.ReportDate
		}
	}
	if latest == "" {
		latest = NormalizeReportDate(time.Now()).Format("2006-01-02")
	}
	out.Date = latest
	return out
}

// RadarReportBrief is the light latest-report projection used by radar rows.
type RadarReportBrief struct {
	PeriodDate   string
	Title        string
	ArticleCount int
	EventCount   int
}

// GetLatestReportBrief returns the board's newest report without sections.
func (r *TopicGraphRepository) GetLatestReportBrief(boardID uint) (*RadarReportBrief, error) {
	var row struct {
		PeriodDate    time.Time
		Title         string
		ArticleCount  int
		EventTagCount int
	}
	err := r.db.Model(&BoardDailyReport{}).
		Select("period_date, title, article_count, event_tag_count").
		Where("semantic_board_id = ?", boardID).
		Order("period_date DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.PeriodDate.IsZero() {
		return nil, nil
	}
	return &RadarReportBrief{
		PeriodDate:   NormalizeReportDate(row.PeriodDate).Format("2006-01-02"),
		Title:        row.Title,
		ArticleCount: row.ArticleCount,
		EventCount:   row.EventTagCount,
	}, nil
}
