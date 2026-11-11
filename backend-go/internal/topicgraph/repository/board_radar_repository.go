package repository

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── 今日雷达聚合（rework-workshop-into-radar-and-desk 8.2）──────────────────
//
// 单次端点：活跃版块 × 最新日报锚点 × 近 14 天。口径复用既有
// GetBoardLaneDynamics（泳道/分区/线索）与 board_daily_reports（最新日报），
// 本文件只做窗口对齐与求和，不复制匹配/日报统计公式。

// RadarLane is one of the board's top lanes with its 14-day density.
type RadarLane struct {
	TopicID         uint   `json:"topic_id"`
	Label           string `json:"label"`
	SectionCount14d int    `json:"section_count_14d"`
	ThreadCount14d  int    `json:"thread_count_14d"`
	// Counts is the 14-day SECTION density (legacy口径, oldest → newest);
	// StoryCounts is the same axis in deduplicated event-line counts
	// (add-event-story-heat：密度标签换事件线口径， Counts 保留向后兼容).
	Counts      []int `json:"counts"`       // 14 days, oldest → newest
	StoryCounts []int `json:"story_counts"` // 14 days, oldest → newest
}

// RadarBoardRow is one /semantic-boards/overview row.
//
// Attention is Σ lane_heat over the board's lanes (72h/14d story-signal
// decay, 1 decimal) — section/thread counts must never feed it
// (add-event-story-heat spec：不得因分区数虚高). The *_sections fields stay
// on the legacy section口径 for compatibility; the *_stories fields carry
// the event-line count over the same axis windows, and the *_heat fields
// carry the day-end story_heat totals (48h/24h口径 via
// ComputeBoardStoryHeatByDay) used by the「今天可能升温」list.
type RadarBoardRow struct {
	BoardID       uint   `json:"board_id"`
	Label         string `json:"label"`
	TagCount      int64  `json:"tag_count"`
	ReportDate    string `json:"report_date"` // latest report period_date (YYYY-MM-DD), "" when none
	ReportTitle   string `json:"report_title"`
	ReportArticle int    `json:"report_article_count"`
	ReportEvent   int    `json:"report_event_count"`
	AnchorDate    string `json:"anchor_date"`
	// Legacy section-count windows (compat).
	TodaySections int `json:"today_sections"`
	Last3Sections int `json:"last3_sections"`
	Prev3Sections int `json:"prev3_sections"`
	// Event-line count windows over the same 14-day axis
	// (today = axis last day, last3 = last 3 days, prev3 = the 3 before).
	TodayStories int `json:"today_stories"`
	Last3Stories int `json:"last3_stories"`
	Prev3Stories int `json:"prev3_stories"`
	// Attention = Σ lane_heat（保留一位小数）。
	Attention float64 `json:"attention"`
	// Day-end story_heat windows (48h/24h口径，保留一位小数) + the full
	// 14-day axis for the升温 trend chart (与 days 同序， oldest → newest).
	TodayHeat       float64   `json:"today_heat"`
	Last3Heat       float64   `json:"last3_heat"`
	Prev3Heat       float64   `json:"prev3_heat"`
	HeatDays        []float64 `json:"heat_days"` // 14 days, oldest → newest
	HeatRuleVersion string    `json:"heat_rule_version"`
	HeatIncomplete  bool      `json:"heat_incomplete"` // 落后源：热度证据可能缺最新报道
	// StoriesBackfilled mirrors lane-dynamics stories_backfilled（窗口内是否有
	// thread 携带 story_id）：false ⇒ *_stories 窗口为非综合稿上界，雷达「今日
	// 该读」事件数须标注降级（workshop-radar 未回填口径，与 lane-dynamics 同源）。
	// 注意：lane-dynamics 的样本只覆盖 active ∪ watch-linked 话题锚定的
	// section——它**不能**用来判雷达事件线是否该上榜（候选话题锚定的版块
	// 事件线同样是真实的），雷达侧改用下面的 story_identity_state。
	StoriesBackfilled bool `json:"stories_backfilled"`
	// StoryIdentityState is the RADAR-side board-level story-identity state
	// （R-2 三态）：backfilled / unbackfilled / no_candidate（空串 = 无日报，
	// 未判定）。只看该版块自己的报告线程带不带 story_id，与话题是否锚定到
	// active 话题无关——话题是否锚定不该决定事件线是否上榜。
	StoryIdentityState string      `json:"story_identity_state"`
	DataDelayed        bool        `json:"data_delayed"`
	Days               []int       `json:"days"` // 14 days, oldest → newest (legacy section口径)
	Lanes              []RadarLane `json:"lanes"`
}

// RadarOverview is the single-shot radar payload.
type RadarOverview struct {
	Date   string          `json:"date"`
	Boards []RadarBoardRow `json:"boards"`
	// Stories is the cross-board event-line feed (today's reads / warming /
	// new). It travels with the same response — the client never merges or
	// re-sorts event lines itself.
	Stories *RadarStoryFeed `json:"stories,omitempty"`
}

// RadarStoryItem is one event line in the radar's story feed (wire shape).
type RadarStoryItem struct {
	BoardID    uint   `json:"board_id"`
	BoardLabel string `json:"board_label"`
	StoryID    uint   `json:"story_id"`
	// StoryTitle is the surviving event line's own title ("" = the client
	// falls back to「事件线 #ID」— the title never carries identity).
	StoryTitle   string  `json:"story_title"`
	Participants int     `json:"participants"`
	Heat         float64 `json:"heat"`
	BaselineHeat float64 `json:"baseline_heat"`
	// Ratio is heat / baseline_heat; 0 when there is no baseline (the entry
	// then carries the「新起」badge inline and shows no multiplier).
	Ratio            float64 `json:"ratio"`
	HasBaseline      bool    `json:"has_baseline"`
	HasTodayEvidence bool    `json:"has_today_evidence"`
	// AnchorDate is the entry's own board anchor day — a lagging board shows
	// its own date, never the global one.
	AnchorDate string `json:"anchor_date"`
	// ReportDate is the landing report; empty = no body thread to jump to.
	ReportDate string `json:"report_date"`
}

// RadarStoryFeed is the cross-board event-line feed of the overview endpoint.
// The merge, the sort and the truncation are done server-side; the three
// degradation markers (unbackfilled / lagging / partial) are honest state,
// never an empty-list business conclusion.
//
// matrix-clue-fallback-and-rail-merge D4：右栏收敛为单清单「今日该读」(≤8) +
// 行内徽标——独立的 warming/new_stories 清单数组与去 Top 计数字段
// （new_count/new_total_count）已移除：徽标判定所需 has_baseline/ratio 随
// top_stories 条目自带；WarmingCount 保留（报头「N 处出现升温」全量计数），
// CrossDayStoryCount 保留（右栏「跨天延续建设中」脚注判据）。
type RadarStoryFeed struct {
	// AnchorDate is the global anchor (the overview's date) — used for the
	// "N boards lag behind" note only.
	AnchorDate      string           `json:"anchor_date"`
	HeatRuleVersion string           `json:"heat_rule_version"`
	TopStories      []RadarStoryItem `json:"top_stories"`
	// WarmingCount is the FULL count of event lines whose own heat reached 2×
	// their own 3-day baseline with today evidence (masthead 「N 处出现升温」).
	// The trimmed list itself is gone with the merged rail (badges carry the
	// per-item verdict); only the count remains.
	WarmingCount int `json:"warming_count"`
	// CrossDayStoryCount 是**既在锚点日之前 3 日基线窗口内有热度证据、又在
	// 锚点日当天有证据**的事件线条数（全部候选，不受任何截断影响）。它是
	// 右栏「跨天延续建设中」脚注的服务端判据：0 = 当前根本不存在跨天延续
	// 的事件线（脚注出现）；>0 = 有跨天线，升温与否由清单行内徽标表达，
	// 不再有空态文案。前端 SHALL NOT 凭条目渲染结果反推这个结论
	// （条目会被截断）。
	//
	// 为什么不是单纯的「有基线」：锚点日之前就结束、只剩 48h 残热的旧线也有
	// 基线，但它们并没有延续到今天——把它们算作跨天会把「能力还没有」误报成
	// 「有跨天线只是没升温」。
	CrossDayStoryCount int `json:"cross_day_story_count"`
	// UnbackfilledBoardIDs are boards whose event identities are not
	// backfilled — they produce no entries at all (红线 30).
	UnbackfilledBoardIDs   []uint `json:"unbackfilled_board_ids"`
	UnbackfilledBoardCount int    `json:"unbackfilled_board_count"`
	// NoStoryBoardIDs are boards with no backfillable event identity at all
	// （无带 story_id 的线程，也没有可回填的候选）——它们**不计入**未回填数
	// （本来就没有事件身份可回填），也不产生条目；不得呈现为「今天没有事件」。
	NoStoryBoardIDs   []uint `json:"no_story_board_ids"`
	NoStoryBoardCount int    `json:"no_story_board_count"`
	// LaggingBoardIDs are boards whose own anchor day is behind the global
	// one (data lag, not missing data).
	LaggingBoardIDs []uint `json:"lagging_board_ids"`
	// Partial is true when some board missed the endpoint budget: the event
	// line ranking is then incomplete and must be labelled as such.
	Partial          bool   `json:"partial"`
	DegradedBoardIDs []uint `json:"degraded_board_ids"`
}

// 清单条数上限（右栏宽度固定，条数只影响首屏密度；口径见 change design D12。
// matrix-clue-fallback-and-rail-merge D4：5→8，单清单吃掉原「今日新起」档
// 屏位；warming/new 独立清单与其上限随徽标化移除）。
const (
	radarTopStoryLimit = 8
	// radarWarmingRatio：升温阈值（当日热度 ≥ 自身近 3 日基线的 2 倍）。
	radarWarmingRatio = 2
)

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
			Days:            make([]int, LaneDynamicsDefaultDays),
			HeatDays:        make([]float64, LaneDynamicsDefaultDays),
			HeatRuleVersion: HeatRuleVersion,
			Lanes:           []RadarLane{},
		}
	}
	if len(boards) == 0 {
		// 空版块也要走 finalize（date 回退今天，不冒空串——tasks 9.4 测试抓出
		// early return 曾绕过回退逻辑，雷达空态报头拿到空日期）。
		finalizeRadar(out)
		finalizeRadarStories(out, map[uint]radarBoardStoryResult{})
		return out, nil
	}

	type result struct {
		index int
		row   RadarBoardRow
		story radarBoardStoryResult
	}
	ch := make(chan result, len(boards))
	for i, b := range boards {
		go func(index int, brief boardBrief) {
			row := r.buildRadarRow(brief.ID)
			row.BoardID = brief.ID
			row.Label = brief.Label
			row.TagCount = brief.TagCount
			// 事件线明细与版块行同属一个 goroutine、共享同一个 5s 预算：
			// 算不完的版块整体降级（data_delayed），不延长预算。
			ch <- result{index: index, row: row, story: r.radarBoardStoryPoints(brief.ID, row)}
		}(i, b)
	}

	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	done := make(map[int]bool, len(boards))
	results := make(map[uint]radarBoardStoryResult, len(boards))
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for len(done) < len(boards) {
		select {
		case res := <-ch:
			done[res.index] = true
			out.Boards[res.index] = res.row
			results[res.row.BoardID] = res.story
		case <-timer.C:
			for i := range out.Boards {
				if !done[i] {
					out.Boards[i].DataDelayed = true
					out.Boards[i].Days = make([]int, LaneDynamicsDefaultDays)
					out.Boards[i].HeatDays = make([]float64, LaneDynamicsDefaultDays)
					out.Boards[i].Lanes = []RadarLane{}
				}
			}
			// drain in background; ch is buffered so senders never block
			finalizeRadar(out)
			finalizeRadarStories(out, results)
			return out, nil
		}
	}
	finalizeRadar(out)
	finalizeRadarStories(out, results)
	return out, nil
}

// radarBoardStoryResult is one board's event-line measurement outcome.
type radarBoardStoryResult struct {
	points []RadarStoryPoint
	// degraded = 该版块的事件线计算失败（局部失败）→ 该板不参与全局排序，
	// 且整体排序标记为不完整（partial）。与 DataDelayed（整板数据延迟超时）
	// 是两回事：不得复用后者（会把「计算失败」误标成「超时降级」）。
	degraded bool
}

// radarBoardStoryPoints measures one board's event lines at **that board's
// own report day**（该版块自己的最新日报日 = 它自己的「今天」，红线 30）——
// 不取 lane-dynamics 的轴末日（轴末日只覆盖 active ∪ watch-linked 话题锚定
// 的 section；最新一期日报没锚定到活跃话题的版块会因轴末日为空而整板缺席）。
//
// Boards whose event identities are not backfilled produce no entries at all
// （红线 30：不得按 0/空态冒充无事件）；失败降级为该版块无条目并回传
// degraded 标记（排序不完整必须明示），不阻断整页。
func (r *TopicGraphRepository) radarBoardStoryPoints(boardID uint, row RadarBoardRow) radarBoardStoryResult {
	// 未回填版块：事件身份未回填 → 不产生任何条目（红线 30）。
	if row.StoryIdentityState == RadarStoryIdentityUnbackfilled {
		return radarBoardStoryResult{}
	}
	// 判定不可判定（取数失败）→ 保守降级：不出条目，并标记排序不完整。
	if row.StoryIdentityState == RadarStoryIdentityUnknown {
		return radarBoardStoryResult{degraded: true}
	}
	// 无日报即无「自己的今天」→ 不产条目。
	if row.ReportDate == "" {
		return radarBoardStoryResult{}
	}
	anchor, ok := radarAnchorInstant(row.ReportDate)
	if !ok {
		return radarBoardStoryResult{}
	}
	points, err := r.RadarBoardStoryPoints(boardID, anchor)
	if err != nil {
		logging.Warnf("radar: board %d story points degraded: %v", boardID, err)
		return radarBoardStoryResult{degraded: true}
	}
	ids := make([]uint, 0, len(points))
	for _, point := range points {
		ids = append(ids, point.StoryID)
	}
	dates, err := r.RadarStoryReportDates(boardID, anchor, ids)
	if err != nil {
		logging.Warnf("radar: board %d story report dates degraded: %v", boardID, err)
		return radarBoardStoryResult{degraded: true}
	}
	for i := range points {
		points[i].ReportDate = dates[points[i].StoryID]
	}
	return radarBoardStoryResult{points: points}
}

func (r *TopicGraphRepository) buildRadarRow(boardID uint) RadarBoardRow {
	row := RadarBoardRow{
		Days:            make([]int, LaneDynamicsDefaultDays),
		HeatDays:        make([]float64, LaneDynamicsDefaultDays),
		HeatRuleVersion: HeatRuleVersion,
		Lanes:           []RadarLane{},
	}
	brief, err := r.GetLatestReportBrief(boardID)
	if err == nil && brief != nil {
		row.ReportDate = brief.PeriodDate
		row.ReportTitle = brief.Title
		row.ReportArticle = brief.ArticleCount
		row.ReportEvent = brief.EventCount
	}

	// 事件身份回填状态：**板级**判定（R-2）——只看该版块自己的报告线程带不带
	// story_id，不依赖话题是否锚定到 active 话题（候选话题锚定的版块同样有
	// 真实事件线，不得被误判成「未回填」而整板剔除——那是对用户说假话）。
	// 窗口 = 该版块最新日报日起的 14 天（与 lane-dynamics 同窗）。
	if row.ReportDate != "" {
		axis := radarAxis(row.ReportDate)
		if len(axis) > 0 {
			state, err := r.radarBoardStoryIdentityState(boardID, axis[0], axis[len(axis)-1])
			if err != nil {
				logging.Warnf("radar: board %d story identity state degraded: %v", boardID, err)
				row.StoryIdentityState = RadarStoryIdentityUnknown
			} else {
				row.StoryIdentityState = state
			}
		}
	}

	dynamics, err := r.GetBoardLaneDynamics(boardID, LaneDynamicsDefaultDays)
	if err != nil || dynamics == nil || !dynamics.HasReports {
		return row
	}
	// 覆盖标记直接复用 dynamics 内的 BoardHeatCoverage 结果（同板同一次
	// 评估；失败已在 dynamics 侧降级告警，不阻断雷达行）。
	row.HeatIncomplete = dynamics.HeatIncomplete
	row.StoriesBackfilled = dynamics.StoriesBackfilled
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

	storyAxis := make([]int, LaneDynamicsDefaultDays)
	for _, lane := range dynamics.Lanes {
		counts := make([]int, LaneDynamicsDefaultDays)
		storyCounts := make([]int, LaneDynamicsDefaultDays)
		for _, day := range lane.Timeline {
			i, ok := index[day.Date]
			if !ok {
				continue
			}
			counts[i] = len(day.Sections)
			storyCounts[i] = day.StoryCount
			row.Days[i] += counts[i]
			storyAxis[i] += storyCounts[i]
		}
		// Attention 求和取全部 lane（截前 3 之前），且只来自 lane_heat。
		row.Attention += lane.LaneHeat
		row.Lanes = append(row.Lanes, RadarLane{
			TopicID:         lane.TopicID,
			Label:           lane.Label,
			SectionCount14d: lane.SectionCount14d,
			ThreadCount14d:  lane.ThreadCount14d,
			Counts:          counts,
			StoryCounts:     storyCounts,
		})
	}
	// 「前 3 条泳道」筛选键 = 14 天事件线密度之和（specs/workshop-radar 行
	// 构成；lane-dynamics 的行序口径），不再按线索数——线索多但事件线少的
	// 泳道不得挤掉「线索少但事件线多」的泳道。同值保持传入稳定次序
	// （lane-dynamics 的 lane_heat→story→id 序），不另设 topic id 键。
	sort.SliceStable(row.Lanes, func(i, j int) bool {
		return sumRadar(row.Lanes[i].StoryCounts) > sumRadar(row.Lanes[j].StoryCounts)
	})
	if len(row.Lanes) > 3 {
		row.Lanes = row.Lanes[:3]
	}
	last := len(row.Days) - 1
	row.TodaySections = row.Days[last]
	row.Last3Sections = sumRadar(row.Days[last-2:])
	row.Prev3Sections = sumRadar(row.Days[last-5 : last-2])
	row.TodayStories = storyAxis[last]
	row.Last3Stories = sumRadar(storyAxis[last-2:])
	row.Prev3Stories = sumRadar(storyAxis[last-5 : last-2])
	row.Attention = math.Round(row.Attention*10) / 10

	// 逐日 story_heat（today/last3/prev3 升温口径，task 5.2）：一次取全轴，
	// 三窗从原始值求和后各自保留一位小数；失败降级全零并告警，不阻断。
	if heatByDay, err := r.ComputeBoardStoryHeatByDay(boardID, radarHeatDays(axis)); err != nil {
		logging.Warnf("radar: board %d heat-by-day degraded: %v", boardID, err)
	} else {
		dayHeat := func(i int) float64 { return heatByDay[axis[i]] }
		for i := range axis {
			row.HeatDays[i] = math.Round(dayHeat(i)*10) / 10
		}
		row.TodayHeat = math.Round(dayHeat(last)*10) / 10
		row.Last3Heat = math.Round((dayHeat(last)+dayHeat(last-1)+dayHeat(last-2))*10) / 10
		row.Prev3Heat = math.Round((dayHeat(last-3)+dayHeat(last-4)+dayHeat(last-5))*10) / 10
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

// radarHeatDays converts the axis date strings to CST-anchored instants —
// ComputeBoardStoryHeatByDay resolves each to its CST 23:59:59 day end, so
// parsing in CST keeps the day identity independent of the server timezone.
func radarHeatDays(axis []string) []time.Time {
	days := make([]time.Time, 0, len(axis))
	for _, date := range axis {
		t, err := time.ParseInLocation("2006-01-02", date, storyHeatZone)
		if err != nil {
			return nil
		}
		days = append(days, t)
	}
	return days
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

// finalizeRadarStories merges the per-board event-line measurements into the
// cross-board feed. Everything statistical happens here, server-side: one
// global sort key (heat desc → participants desc → story id asc, unrounded),
// the three trimmed lists and the degradation markers.
//
// Boards that missed the endpoint budget (data_delayed) or whose event-line
// computation failed contribute no entries and turn the feed into「partial」——
// an incomplete ranking must never be presented as a complete one (红线 30 的
// 诚实降级同一纪律；两类降级语义不同：前者是超时，后者是计算失败).
func finalizeRadarStories(out *RadarOverview, results map[uint]radarBoardStoryResult) {
	feed := &RadarStoryFeed{
		AnchorDate:           out.Date,
		HeatRuleVersion:      HeatRuleVersion,
		TopStories:           []RadarStoryItem{},
		UnbackfilledBoardIDs: []uint{},
		NoStoryBoardIDs:      []uint{},
		LaggingBoardIDs:      []uint{},
		DegradedBoardIDs:     []uint{},
	}
	items := make([]RadarStoryItem, 0, len(results))
	crossDayStories := 0
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
			// 单版块事件线计算失败 → 该板不参与全局排序 + 整体排序不完整。
			feed.Partial = true
			feed.DegradedBoardIDs = append(feed.DegradedBoardIDs, row.BoardID)
			continue
		}
		switch row.StoryIdentityState {
		case RadarStoryIdentityUnbackfilled:
			// 有候选线程却都没带 story_id → 事件身份未回填（红线 30：不产生
			// 条目、不冒充无事件）。
			feed.UnbackfilledBoardIDs = append(feed.UnbackfilledBoardIDs, row.BoardID)
		case RadarStoryIdentityNoCandidate:
			// 无可回填对象 → 不计入未回填数（本来就没有事件身份可回填）。
			feed.NoStoryBoardIDs = append(feed.NoStoryBoardIDs, row.BoardID)
		}
		for _, point := range res.points {
			// 跨天延续 = **既在锚点日之前有证据、又在锚点日当天有证据**。
			// 只数 HasBaseline 会把「锚点日之前就结束、只剩 48h 残热」的旧线
			// 也算进去（真库实测：只数基线会得到 388 条，实际跨天数为 0）。
			if point.HasBaseline && point.HasTodayEvidence {
				crossDayStories++
			}
			items = append(items, RadarStoryItem{
				BoardID:          row.BoardID,
				BoardLabel:       row.Label,
				StoryID:          point.StoryID,
				StoryTitle:       point.Title,
				Participants:     point.Participants,
				Heat:             point.Heat,
				BaselineHeat:     point.BaselineHeat,
				Ratio:            point.Ratio,
				HasBaseline:      point.HasBaseline,
				HasTodayEvidence: point.HasTodayEvidence,
				AnchorDate:       point.AnchorDate,
				ReportDate:       point.ReportDate,
			})
		}
	}
	// 排序键与日报「今日重点」候选排序同一口径（heat desc → participants desc
	// → story id asc）；排序前不四舍五入。story_id 是全局主键，跨版块可比。
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Heat != b.Heat {
			return a.Heat > b.Heat
		}
		if a.Participants != b.Participants {
			return a.Participants > b.Participants
		}
		return a.StoryID < b.StoryID
	})
	for i, item := range items {
		if i >= radarTopStoryLimit {
			break
		}
		feed.TopStories = append(feed.TopStories, item)
	}
	// 徽标化后的计数（matrix-clue-fallback-and-rail-merge D4）：WarmingCount 是
	// 全量升温计数（不截断，报头消费）——判定与被删的升温清单、前端徽标、spec、
	// API 文档同口径（自身基线 ratio ≥ 2 **且当日有证据**），只是不再产出清单
	// 本身；新起/去 Top 互斥计数随单清单移除（徽标信息随 top_stories 条目自带，
	// 同一事件线在单清单里结构性不重复）。
	warmingCount := 0
	for _, item := range items {
		if item.HasBaseline && item.Ratio >= radarWarmingRatio && item.HasTodayEvidence {
			warmingCount++
		}
	}
	feed.WarmingCount = warmingCount
	feed.CrossDayStoryCount = crossDayStories
	feed.UnbackfilledBoardCount = len(feed.UnbackfilledBoardIDs)
	feed.NoStoryBoardCount = len(feed.NoStoryBoardIDs)
	sort.Slice(feed.UnbackfilledBoardIDs, func(i, j int) bool { return feed.UnbackfilledBoardIDs[i] < feed.UnbackfilledBoardIDs[j] })
	sort.Slice(feed.NoStoryBoardIDs, func(i, j int) bool { return feed.NoStoryBoardIDs[i] < feed.NoStoryBoardIDs[j] })
	sort.Slice(feed.LaggingBoardIDs, func(i, j int) bool { return feed.LaggingBoardIDs[i] < feed.LaggingBoardIDs[j] })
	sort.Slice(feed.DegradedBoardIDs, func(i, j int) bool { return feed.DegradedBoardIDs[i] < feed.DegradedBoardIDs[j] })
	out.Stories = feed
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
