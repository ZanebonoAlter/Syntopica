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
//
// 「未取到 ≠ 0」（fix-radar-overview-false-zero）：计数类字段用指针表达两种语义——
// nil = **未知**（本次没取到该版块的数据，JSON 落 null），指针值 = **已取到的数**
// （可以是真实的 0）。JSON 刻意不加 omitempty：未取到必须落成 null 而不是被省略
// 或写成 0，否则前端会把降级渲染成「今天没有热点」这类业务结论。
type RadarBoardRow struct {
	BoardID  uint   `json:"board_id"`
	Label    string `json:"label"`
	TagCount int64  `json:"tag_count"`
	// ReportDate / ReportTitle 是字符串事实：取到了就填，没取到是空串。
	ReportDate  string `json:"report_date"` // latest report period_date (YYYY-MM-DD), "" when none
	ReportTitle string `json:"report_title"`
	// ReportArticle / ReportEvent：nil = 未知（没取到），0 = 取到了且确实没有。
	ReportArticle *int   `json:"report_article_count"`
	ReportEvent   *int   `json:"report_event_count"`
	AnchorDate    string `json:"anchor_date"`
	// Same-day / 3-day / previous-3-day clue windows (oldest → newest axis).
	// nil = 未知（见 ReportArticle）。
	TodayClues *int `json:"today_clues"`
	Last3Clues *int `json:"last3_clues"`
	Prev3Clues *int `json:"prev3_clues"`
	// Attention = Σ 泳道当天热度（含候选按权重计入），保留一位小数。
	// nil = 未知（见 ReportArticle）。
	Attention *float64 `json:"attention"`
	// HeatRuleVersion pins the same-day heat formula (heat-v2-lane-clues-1d).
	HeatRuleVersion string `json:"heat_rule_version"`
	// ClueDays is the full 14-day clue axis (oldest → newest).
	ClueDays []int `json:"clue_days"`
	// DataDelayed = 本次聚合超时未完成该版块（端点预算耗尽）。
	DataDelayed bool `json:"data_delayed"`
	// DataUnavailable = 本次**没取到**该版块的数据（超时未完成 / 泳道动态计算
	// 失败）：该行计数类字段是未知（null），不是 0。取数成功但确实没数据的版块
	// 仍是完整零值行（计数为 0）且不标本字段。
	DataUnavailable bool        `json:"data_unavailable"`
	Lanes           []RadarLane `json:"lanes"`
}

// radarIntPtr / radarFloatPtr 把已知的数包成指针：区分「已知的 0」与「未知」。
func radarIntPtr(v int) *int { return &v }

func radarFloatPtr(v float64) *float64 { return &v }

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

// RadarOverviewBudget is the aggregation budget for one overview call. The
// handler wraps the request context with the same 5s bound; this caps it
// inside the repository so late rows degrade instead of hanging. It is a
// package-level var (not a const) so tests can inject a zero budget and
// deterministically reproduce the timeout-degradation row — real slow queries
// can't be reproduced reliably in a unit test.
var RadarOverviewBudget = 5 * time.Second

// GetBoardRadarOverview aggregates all active boards. A board whose lane
// dynamics query fails degrades to unknown counts instead of failing the page
// (spec: 无数据版块返回零值). ctx carries the 5s handler budget; rows that
// do not make it are returned as unknown with data_delayed=true +
// data_unavailable=true — a degradation must never look like a business
// conclusion («今天 0 关注»).
func (r *TopicGraphRepository) GetBoardRadarOverview(ctx context.Context) (*RadarOverview, error) {
	// ctx 感知副本：查询句柄带上请求 ctx —— 预算到期后在飞的查询立刻被取消，
	// 连接马上归还池子（否则超时后 goroutine 还在跑完查询、白占连接与 IO，
	// 拖慢后续请求）。
	rctx := r.withContext(ctx)
	var labels []models.SemanticLabel
	if err := rctx.db.
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
		if err := rctx.db.Table("topic_tag_board_labels").
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

	// 批量板级取数：所有版块的锚点日 / 非归档话题 / watch 关联话题（3 条）+
	// 各版块最新日报摘要（1 条）—— 取代「每版块各查一遍」的 N 倍往返。
	// 取数口径与单版块路径逐字一致（同一张表、同一过滤、同一「取最新」语义），
	// 聚合口径代码则完全复用（computeBoardLaneDynamics / radarRowFrom）。
	levels, levelErr := rctx.loadBoardLevels(ids)
	if levelErr != nil {
		// 批量板级取数失败 → 各版块的泳道数据「没取到」（降级），不报错挂端点。
		logging.Warnf("radar: board level batch failed: %v", levelErr)
	}
	briefs, briefErr := rctx.loadLatestReportBriefs(ids)
	if briefErr != nil {
		// 批量日报摘要失败 → 各版块的日报计数「没取到」（null），不冒充 0。
		logging.Warnf("radar: report brief batch failed: %v", briefErr)
	}
	batch := &radarBatch{
		levels:   levels,
		briefs:   briefs,
		levelErr: levelErr,
		briefErr: briefErr,
		settings: rctx.loadLaneDynamicsSettings(),
	}

	type result struct {
		index int
		board radarBoardResult
	}
	ch := make(chan result, len(boards))
	for i, b := range boards {
		go func(index int, brief boardBrief) {
			ch <- result{index: index, board: rctx.buildRadarRowBatch(brief.ID, batch)}
		}(i, b)
	}

	deadline := time.Now().Add(RadarOverviewBudget)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	done := make(map[int]bool, len(boards))
	lanes := make(map[uint]radarBoardLanes, len(boards))
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for len(done) < len(boards) {
		// 先判 deadline 再等结果：预算耗尽即刻收口（不再等缓冲区里已算完的行），
		// 超时因此可复现（测试可把预算注入为 0）。
		if time.Now().Before(deadline) {
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
				continue
			case <-timer.C:
			}
		}
		// 预算耗尽：没算完的版块是「没取到」——计数落未知（null），不是 0。
		markRadarRowsUnavailable(out, done)
		// drain in background; ch is buffered so senders never block
		finalizeRadar(out)
		finalizeRadarTopLanes(out, lanes)
		return out, nil
	}
	finalizeRadar(out)
	finalizeRadarTopLanes(out, lanes)
	return out, nil
}

// markRadarRowsUnavailable degrades the boards that didn't make it into the
// budget: their count fields become unknown (nil → JSON null), never 0 —
// rendering a degradation as a business conclusion is the bug this protocol
// change removes.
func markRadarRowsUnavailable(out *RadarOverview, done map[int]bool) {
	for i := range out.Boards {
		if done[i] {
			continue
		}
		out.Boards[i].DataDelayed = true
		out.Boards[i].DataUnavailable = true
		out.Boards[i].Attention = nil
		out.Boards[i].TodayClues = nil
		out.Boards[i].Last3Clues = nil
		out.Boards[i].Prev3Clues = nil
		out.Boards[i].ReportArticle = nil
		out.Boards[i].ReportEvent = nil
		out.Boards[i].ClueDays = []int{}
		out.Boards[i].Lanes = []RadarLane{}
	}
}

// markRadarRowUnavailable 把「本次没取到」写到一行上：计数落未知（null）而不是 0。
// 调用前 row 已带 BoardID/报表摘要，这里只清计数与泳道轴。
func markRadarRowUnavailable(row RadarBoardRow, boardID uint, cause error) RadarBoardRow {
	logging.Warnf("radar: board %d unavailable: %v", boardID, cause)
	row.DataUnavailable = true
	row.Attention = nil
	row.TodayClues = nil
	row.Last3Clues = nil
	row.Prev3Clues = nil
	row.ReportArticle = nil
	row.ReportEvent = nil
	row.ClueDays = []int{}
	row.Lanes = []RadarLane{}
	return row
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

// radarBatch is the batch-loaded input of one radar overview call: every
// board's board-level data + its latest report brief, loaded with a handful of
// queries instead of one set per board.
type radarBatch struct {
	// levels = per-board board-level data (anchor / topics / watch-linked).
	levels map[uint]*boardLevelData
	// briefs = per-board latest report brief (absent = the board has no report).
	briefs map[uint]*RadarReportBrief
	// levelErr = the board-level batch failed → 各版块的泳道数据「没取到」。
	levelErr error
	// briefErr = the report brief batch failed → 各版块的日报计数「没取到」。
	briefErr error
	// settings = request-scoped lane settings (read once per request).
	settings laneDynamicsSettings
}

// buildRadarRowBatch assembles one board's row from the batch-loaded data.
// It runs the SAME aggregation as the single-board path (radarRowFrom) ——
// 批量路径只换「取数」，不改任何口径。
func (r *TopicGraphRepository) buildRadarRowBatch(boardID uint, batch *radarBatch) radarBoardResult {
	var brief *RadarReportBrief
	briefErr := batch.briefErr
	if briefErr == nil {
		brief = batch.briefs[boardID]
	}
	level, hasLevel := batch.levels[boardID]
	if batch.levelErr != nil || !hasLevel {
		dynErr := batch.levelErr
		if dynErr == nil {
			dynErr = fmt.Errorf("radar: board %d level data missing", boardID)
		}
		return radarRowFrom(boardID, brief, briefErr, nil, dynErr)
	}
	dynamics, dynErr := r.radarLaneDynamics(boardID, LaneDynamicsDefaultDays, level, batch.settings)
	return radarRowFrom(boardID, brief, briefErr, dynamics, dynErr)
}

// radarLaneDynamics returns a board's lane dynamics for the radar row: the
// test override seam (repo.laneDynamics) wins; otherwise the shared
// aggregation runs on the batch-loaded board-level data.
func (r *TopicGraphRepository) radarLaneDynamics(boardID uint, days int, level *boardLevelData, settings laneDynamicsSettings) (*LaneDynamicsResponse, error) {
	if r.laneDynamics != nil {
		return r.laneDynamics(boardID, days)
	}
	return r.computeBoardLaneDynamics(level, boardID, days, settings)
}

// buildRadarRow assembles one board's row on the SINGLE-BOARD path (each
// board loads its own data). The batch path (buildRadarRowBatch) and this one
// share the same aggregation code —— radarRowFrom。
//
//nolint:unused // 单版块口径基准：口径回归测试拿它与批量路径逐字段对照；端点走批量路径
func (r *TopicGraphRepository) buildRadarRow(boardID uint) radarBoardResult {
	brief, briefErr := r.GetLatestReportBrief(boardID)
	var dynamics *LaneDynamicsResponse
	var dynErr error
	if r.laneDynamics != nil {
		// 测试注入的失败闸门（原签名不变）。
		dynamics, dynErr = r.laneDynamics(boardID, LaneDynamicsDefaultDays)
	} else {
		dynamics, dynErr = r.GetBoardLaneDynamics(boardID, LaneDynamicsDefaultDays)
	}
	return radarRowFrom(boardID, brief, briefErr, dynamics, dynErr)
}

// radarRowFrom assembles a board's row from the loaded data —— 纯聚合（不碰
// 数据库）：单版块路径与批量路径共用这一份，口径不会分叉。
func radarRowFrom(boardID uint, brief *RadarReportBrief, briefErr error, dynamics *LaneDynamicsResponse, dynErr error) radarBoardResult {
	row := RadarBoardRow{
		ClueDays:        make([]int, LaneDynamicsDefaultDays),
		HeatRuleVersion: LaneHeatRuleVersion,
		Lanes:           []RadarLane{},
		// 取数成功但确实没数据 → 计数是「已知的 0」，不是未知；未知只在降级路径出现。
		Attention:  radarFloatPtr(0),
		TodayClues: radarIntPtr(0),
		Last3Clues: radarIntPtr(0),
		Prev3Clues: radarIntPtr(0),
	}
	switch {
	case briefErr != nil:
		// 日报 brief 取不到 → 报告相关计数是未知（null）；写 0 等于把「没取到」
		// 冒充成「这个版块没有文章」。
		logging.Warnf("radar: board %d report brief failed: %v", boardID, briefErr)
	case brief != nil:
		row.ReportDate = brief.PeriodDate
		row.ReportTitle = brief.Title
		row.ReportArticle = radarIntPtr(brief.ArticleCount)
		row.ReportEvent = radarIntPtr(brief.EventCount)
	default:
		// 没有日报（取数成功）→ 真实零值：0 篇文章 / 0 个事件。
		row.ReportArticle = radarIntPtr(0)
		row.ReportEvent = radarIntPtr(0)
	}

	if dynErr != nil {
		return radarBoardResult{row: markRadarRowUnavailable(row, boardID, dynErr), degraded: true}
	}
	// 当日线索聚合失败时泳道热度是「算不出来的 0」——单版块端点可以降级为 0 热，
	// 但雷达把 0 当结论就是假 0（用户看到的「关注强度 0.0」）。按「未取到」处理。
	if dynamics != nil && dynamics.CluesDegraded {
		return radarBoardResult{
			row:      markRadarRowUnavailable(row, boardID, fmt.Errorf("clue counts degraded")),
			degraded: true,
		}
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
	attention := ComputeBoardAttention(heats)
	row.Attention = &attention

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
	row.TodayClues = radarIntPtr(row.ClueDays[last])
	row.Last3Clues = radarIntPtr(sumRadar(row.ClueDays[last-2:]))
	row.Prev3Clues = radarIntPtr(sumRadar(row.ClueDays[last-5 : last-2]))
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
		a, b := out.Boards[i], out.Boards[j]
		// 关注度未知的行（没取到）沉到末尾：没取到的行不得靠「假装 0」占排名位次。
		if (a.Attention == nil) != (b.Attention == nil) {
			return b.Attention == nil
		}
		if a.Attention == nil {
			return a.BoardID < b.BoardID
		}
		if *a.Attention != *b.Attention {
			return *a.Attention > *b.Attention
		}
		return a.BoardID < b.BoardID
	})
	latest := ""
	unavailable := false
	for _, row := range out.Boards {
		if row.ReportDate > latest {
			latest = row.ReportDate
		}
		if row.DataUnavailable {
			unavailable = true
		}
	}
	if latest == "" && !unavailable {
		// 确实没有日报（取数成功）→ 回退今天，报头不拿空日期。
		// 但存在没取到的行时不得回退：那是「冒充全局锚点日」，保持未知（空串）。
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

// loadLatestReportBriefs loads the newest report brief of MANY boards in ONE
// query —— GetLatestReportBrief 的批量版。
//
// 口径逐字一致：**不限 status**（GetLatestReportBrief 就不限，取最新一期日报
// 摘要，不分是否已发布），只取 period_date 最新的一行。取「每个版块最新一行」
// 用 ROW_NUMBER() 窗口函数（PG 与 SQLite 都支持——本包的 service 层测试跑
// 内存 SQLite，DISTINCT ON 只有 PG 能用）。board_daily_reports 的
// (semantic_board_id, period_date) 有唯一索引，同日不会有并列行，取舍是确定的。
//
// 没有日报的版块不在结果里（调用方读到 nil = 「没有日报」→ 真实零值）。
func (r *TopicGraphRepository) loadLatestReportBriefs(boardIDs []uint) (map[uint]*RadarReportBrief, error) {
	out := make(map[uint]*RadarReportBrief, len(boardIDs))
	if len(boardIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		SemanticBoardID uint
		PeriodDate      time.Time
		Title           string
		ArticleCount    int
		EventTagCount   int
	}
	if err := r.db.Raw(
		"SELECT semantic_board_id, period_date, title, article_count, event_tag_count FROM ("+
			"SELECT semantic_board_id, period_date, title, article_count, event_tag_count, "+
			"ROW_NUMBER() OVER (PARTITION BY semantic_board_id ORDER BY period_date DESC) AS rn "+
			"FROM board_daily_reports WHERE semantic_board_id IN (?)"+
			") ranked WHERE rn = 1",
		boardIDs).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("radar: latest report briefs: %w", err)
	}
	for _, row := range rows {
		if _, seen := out[row.SemanticBoardID]; seen {
			continue
		}
		if row.PeriodDate.IsZero() {
			continue
		}
		out[row.SemanticBoardID] = &RadarReportBrief{
			PeriodDate:   NormalizeReportDate(row.PeriodDate).Format("2006-01-02"),
			Title:        row.Title,
			ArticleCount: row.ArticleCount,
			EventCount:   row.EventTagCount,
		}
	}
	return out, nil
}
