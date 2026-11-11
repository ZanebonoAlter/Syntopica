package repository

import (
	"context"
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── 今日雷达聚合（rework-workshop-into-radar-and-desk 8.2）──────────────────
//
// 单次端点：活跃版块 × 最新日报锚点 × 近 14 天。口径复用既有
// GetBoardLaneDynamics（泳道/线索/当天热度）与 board_daily_reports（最新日报），
// 本文件只做窗口对齐与求和，不复制匹配/日报统计公式。
//
// retire-story-adopt-event-metrics D4/D5/D10：矩阵量纲改「当日线索合计」，
// 事件线量纲整体退役；顶层故事线条目段替换为「当天线索最多的泳道」清单。

// RadarLane is one of the board's top lanes with its 14-day clue density.
type RadarLane struct {
	TopicID uint   `json:"topic_id"`
	Label   string `json:"label"`
	// ClueCounts is the 14-day clue density (per day: the sum of the lane's
	// theme buckets' non-composite thread counts), oldest → newest. The mini
	// matrix displays exactly these numbers (no event-line count, no section
	// count fallback — a missing day is "unknown", never a fabricated 0).
	ClueCounts []int `json:"clue_counts"`
	// ThreadCount14d is the in-window clue total (ordering key of the top-3
	// selection, same as lane-dynamics' secondary sort key).
	ThreadCount14d int `json:"thread_count_14d"`
}

// RadarBoardRow is one /semantic-boards/overview row.
//
// Attention is Σ lane heat over the board's lanes + Σ candidate lane heat
// (1 decimal) — lane heat is the same-day clue count weighted by topic status
// (lane-heat-metric). Section counts, thread counts, event counts and bucket
// counts MUST NOT feed it (spec: 不得因分区数虚高).
type RadarBoardRow struct {
	BoardID       uint   `json:"board_id"`
	Label         string `json:"label"`
	TagCount      int64  `json:"tag_count"`
	ReportDate    string `json:"report_date"` // latest report period_date (YYYY-MM-DD), "" when none
	ReportTitle   string `json:"report_title"`
	ReportArticle int    `json:"report_article_count"`
	ReportEvent   int    `json:"report_event_count"`
	AnchorDate    string `json:"anchor_date"`
	// Same-day / 3-day / previous-3-day clue windows (oldest → newest axis).
	TodayClues int `json:"today_clues"`
	Last3Clues int `json:"last3_clues"`
	Prev3Clues int `json:"prev3_clues"`
	// Attention = Σ 泳道当天热度（含候选按权重计入），保留一位小数。
	Attention float64 `json:"attention"`
	// HeatRuleVersion pins the same-day heat formula (heat-v2-lane-clues-1d).
	HeatRuleVersion string `json:"heat_rule_version"`
	// ClueDays is the full 14-day clue axis (oldest → newest).
	ClueDays    []int       `json:"clue_days"`
	DataDelayed bool        `json:"data_delayed"`
	Lanes       []RadarLane `json:"lanes"`
}

// RadarTopLanes is the cross-board 「当天线索最多的泳道」 list that replaces the
// retired event-line feed (design D10). The merge, the sort and the truncation
// happen server-side; the honest degradation markers (partial / lagging) travel
// with it so an incomplete ranking is never presented as a complete one.
type RadarTopLanes struct {
	// AnchorDate is the global anchor (the overview's date) — used for the
	// "N boards lag behind" note only.
	AnchorDate string             `json:"anchor_date"`
	Lanes      []RadarTopLaneItem `json:"lanes"`
	// Partial is true when some board missed the endpoint budget or its lane
	// data could not be computed: the ranking is then incomplete.
	Partial          bool   `json:"partial"`
	DegradedBoardIDs []uint `json:"degraded_board_ids"`
	// LaggingBoardIDs are boards whose own anchor day is behind the global
	// one (data lag, not missing data).
	LaggingBoardIDs []uint `json:"lagging_board_ids"`
}

// RadarTopLaneItem is one lane of the cross-board list: board + lane identity,
// the lane's same-day clue count and the board's own anchor day.
type RadarTopLaneItem struct {
	BoardID    uint   `json:"board_id"`
	BoardLabel string `json:"board_label"`
	TopicID    uint   `json:"topic_id"`
	TopicLabel string `json:"topic_label"`
	// ClueCount is the lane's clue total on ITS OWN board anchor day.
	ClueCount int `json:"clue_count"`
	// AnchorDate is the entry's own board anchor day — a lagging board shows
	// its own date, never the global one.
	AnchorDate string `json:"anchor_date"`
}

// RadarOverview is the single-shot radar payload.
type RadarOverview struct {
	Date   string          `json:"date"`
	Boards []RadarBoardRow `json:"boards"`
	// TopLanes is the cross-board lane list. It travels with the same response
	// — the client never merges or re-sorts lanes itself, and it never issues a
	// second request for it.
	TopLanes *RadarTopLanes `json:"top_lanes,omitempty"`
}

// radarTopLaneLimit caps the cross-board lane list (right rail width; the
// count only affects first-screen density — 上限 8，第 9 名不展示).
const radarTopLaneLimit = 8

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
			BoardID:         b.ID,
			Label:           b.Label,
			TagCount:        b.TagCount,
			ClueDays:        make([]int, LaneDynamicsDefaultDays),
			HeatRuleVersion: LaneHeatRuleVersion,
			Lanes:           []RadarLane{},
		}
	}
	if len(boards) == 0 {
		// 空版块也要走 finalize（date 回退今天，不冒空串——tasks 9.4 测试抓出
		// early return 曾绕过回退逻辑，雷达空态报头拿到空日期）。
		finalizeRadar(out)
		finalizeRadarTopLanes(out, map[uint]radarBoardLanes{})
		return out, nil
	}

	type result struct {
		index int
		board radarBoardResult
	}
	ch := make(chan result, len(boards))
	for i, b := range boards {
		go func(index int, brief boardBrief) {
			ch <- result{index: index, board: r.buildRadarRow(brief.ID)}
		}(i, b)
	}

	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	done := make(map[int]bool, len(boards))
	lanes := make(map[uint]radarBoardLanes, len(boards))
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for len(done) < len(boards) {
		select {
		case res := <-ch:
			done[res.index] = true
			row := res.board.row
			row.BoardID = out.Boards[res.index].BoardID
			row.Label = out.Boards[res.index].Label
			row.TagCount = out.Boards[res.index].TagCount
			out.Boards[res.index] = row
			lanes[row.BoardID] = radarBoardLanes{
				anchorDate: row.AnchorDate,
				lanes:      res.board.lanes,
				degraded:   res.board.degraded,
			}
		case <-timer.C:
			for i := range out.Boards {
				if !done[i] {
					out.Boards[i].DataDelayed = true
					out.Boards[i].ClueDays = make([]int, LaneDynamicsDefaultDays)
					out.Boards[i].Lanes = []RadarLane{}
				}
			}
			// drain in background; ch is buffered so senders never block
			finalizeRadar(out)
			finalizeRadarTopLanes(out, lanes)
			return out, nil
		}
	}
	finalizeRadar(out)
	finalizeRadarTopLanes(out, lanes)
	return out, nil
}

// radarBoardLanes is one board's lane projection for the cross-board list.
type radarBoardLanes struct {
	anchorDate string
	lanes      []RadarLane
	// degraded = 该版块的泳道数据取数失败（局部失败）→ 该板不参与全局排序，
	// 且整体排序标记为不完整（partial）。与 DataDelayed（整板数据延迟超时）
	// 是两回事：不得复用后者（会把「计算失败」误标成「超时降级」）。
	degraded bool
}

// radarBoardResult is one board's full row plus the untruncated lane list the
// cross-board rail needs (the row itself keeps only the top 3 lanes).
type radarBoardResult struct {
	row      RadarBoardRow
	lanes    []RadarLane
	degraded bool
}

func (r *TopicGraphRepository) buildRadarRow(boardID uint) radarBoardResult {
	row := RadarBoardRow{
		ClueDays:        make([]int, LaneDynamicsDefaultDays),
		HeatRuleVersion: LaneHeatRuleVersion,
		Lanes:           []RadarLane{},
	}
	brief, err := r.GetLatestReportBrief(boardID)
	if err == nil && brief != nil {
		row.ReportDate = brief.PeriodDate
		row.ReportTitle = brief.Title
		row.ReportArticle = brief.ArticleCount
		row.ReportEvent = brief.EventCount
	}

	dynamics, err := r.GetBoardLaneDynamics(boardID, LaneDynamicsDefaultDays)
	if err != nil {
		logging.Warnf("radar: board %d lane dynamics failed: %v", boardID, err)
		return radarBoardResult{row: row, degraded: true}
	}
	if dynamics == nil || !dynamics.HasReports {
		return radarBoardResult{row: row}
	}
	anchor := dynamicsAnchor(dynamics)
	if anchor == "" {
		// 报告存在但没有任何锚定 section：无泳道可展示，仍是完整的零值行。
		return radarBoardResult{row: row}
	}
	row.AnchorDate = anchor
	axis := radarAxis(anchor)
	index := make(map[string]int, len(axis))
	for i, date := range axis {
		index[date] = i
	}

	// Attention = Σ 泳道当天热度 + Σ 候选泳道热度（候选按 lane_heat_candidate_weight
	// 计入，spec: 版块关注度 SHALL 能据此按候选权重计入该泳道）。
	heats := make([]float64, 0, len(dynamics.Lanes)+len(dynamics.Candidates))
	full := make([]RadarLane, 0, len(dynamics.Lanes))
	for _, lane := range dynamics.Lanes {
		clues := make([]int, LaneDynamicsDefaultDays)
		for _, day := range lane.Timeline {
			i, ok := index[day.Date]
			if !ok {
				continue
			}
			clues[i] = day.ClueCount
			row.ClueDays[i] += day.ClueCount
		}
		heats = append(heats, lane.LaneHeat)
		full = append(full, RadarLane{
			TopicID:        lane.TopicID,
			Label:          lane.Label,
			ClueCounts:     clues,
			ThreadCount14d: lane.ThreadCount14d,
		})
	}
	for _, candidate := range dynamics.Candidates {
		heats = append(heats, candidate.LaneHeat)
	}
	row.Attention = ComputeBoardAttention(heats)

	// 「前 3 条泳道」筛选键 = 14 天线索合计（与 lane-dynamics 行序同一口径，
	// 传入次序已是 lane_heat → clue total → id）。section 数 / 事件线数不参与。
	sort.SliceStable(full, func(i, j int) bool {
		return full[i].ThreadCount14d > full[j].ThreadCount14d
	})
	row.Lanes = full
	if len(row.Lanes) > 3 {
		row.Lanes = row.Lanes[:3]
	}
	last := len(row.ClueDays) - 1
	row.TodayClues = row.ClueDays[last]
	row.Last3Clues = sumRadar(row.ClueDays[last-2:])
	row.Prev3Clues = sumRadar(row.ClueDays[last-5 : last-2])
	return radarBoardResult{row: row, lanes: full}
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

// finalizeRadarTopLanes merges the per-board lane lists into the cross-board
// 「当天线索最多的泳道」 list (design D10):
//   - one entry per lane of every active board, each carrying its OWN board's
//     anchor day (a lagging board shows its own date);
//   - sorted in two levels (2026-11-11 看版后修订, tasks 3.6): ① anchor-day
//     freshness — entries whose anchor day IS the global anchor day come first
//     and lagging entries sink to the bottom; ② same-day clue count desc,
//     tie-broken by a stable key (board label → lane label → lane id) so
//     rendering order / map iteration can never flip two equal entries.
//     Sinking is NOT a discount: a lagging entry keeps its raw clue count in the
//     secondary key (no decay / no half-life — heat only ever expresses one day);
//   - truncated to radarTopLaneLimit (the 9th entry is not shown);
//   - boards that missed the endpoint budget (data_delayed) or whose lane data
//     failed contribute NO entries and turn the list into「partial」— an
//     incomplete ranking must never be presented as a complete one.
func finalizeRadarTopLanes(out *RadarOverview, results map[uint]radarBoardLanes) {
	feed := &RadarTopLanes{
		AnchorDate:       out.Date,
		Lanes:            []RadarTopLaneItem{},
		DegradedBoardIDs: []uint{},
		LaggingBoardIDs:  []uint{},
	}
	items := make([]RadarTopLaneItem, 0)
	for _, row := range out.Boards {
		// 滞后：版块自己的「今天」（= 它的最新日报日）落后于全局锚点日。
		if row.ReportDate != "" && out.Date != "" && row.ReportDate < out.Date {
			feed.LaggingBoardIDs = append(feed.LaggingBoardIDs, row.BoardID)
		}
		if row.DataDelayed {
			feed.Partial = true
			feed.DegradedBoardIDs = append(feed.DegradedBoardIDs, row.BoardID)
			continue
		}
		res := results[row.BoardID]
		if res.degraded {
			feed.Partial = true
			feed.DegradedBoardIDs = append(feed.DegradedBoardIDs, row.BoardID)
			continue
		}
		for _, lane := range res.lanes {
			if len(lane.ClueCounts) == 0 {
				continue
			}
			clues := lane.ClueCounts[len(lane.ClueCounts)-1]
			if clues <= 0 {
				continue // 当天无线索的泳道不进清单（空态由前端占位）
			}
			items = append(items, RadarTopLaneItem{
				BoardID:    row.BoardID,
				BoardLabel: row.Label,
				TopicID:    lane.TopicID,
				TopicLabel: lane.Label,
				ClueCount:  clues,
				AnchorDate: res.anchorDate,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		// 一级：锚点日新鲜度——与全局锚点日一致的条目优先，滞后条目整体沉底。
		// 「今天」不同的数据不该与今天的数据排在同一序列里，但也不打折：
		// 滞后条目仍进清单，其 clue_count 原值参与二级排序。
		aFresh := a.AnchorDate == out.Date
		bFresh := b.AnchorDate == out.Date
		if aFresh != bFresh {
			return aFresh
		}
		if a.ClueCount != b.ClueCount {
			return a.ClueCount > b.ClueCount
		}
		if a.BoardLabel != b.BoardLabel {
			return a.BoardLabel < b.BoardLabel
		}
		if a.TopicLabel != b.TopicLabel {
			return a.TopicLabel < b.TopicLabel
		}
		return a.TopicID < b.TopicID
	})
	for i, item := range items {
		if i >= radarTopLaneLimit {
			break
		}
		feed.Lanes = append(feed.Lanes, item)
	}
	sort.Slice(feed.DegradedBoardIDs, func(i, j int) bool { return feed.DegradedBoardIDs[i] < feed.DegradedBoardIDs[j] })
	sort.Slice(feed.LaggingBoardIDs, func(i, j int) bool { return feed.LaggingBoardIDs[i] < feed.LaggingBoardIDs[j] })
	out.TopLanes = feed
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
