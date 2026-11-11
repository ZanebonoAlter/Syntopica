package repository

import (
	"fmt"
	"sort"
	"time"

	"syntopica-backend/internal/platform/logging"
)

// ── 雷达右栏事件线清单（surface-story-events-in-radar）─────────────────────
//
// 条目单位是**事件线**（board_stories），不是版块日报标题、也不是分区（红线 28：
// section 数 / thread 条数 MUST NOT 参与任何热度或排序计算）。热度只复用既有
// story_heat 口径（loadBoardHeatEvidence + boardHeatEvidence.compute，
// heat-v1-48h-hl24h）——本文件不新增指数衰减公式、不改 ComputeBoardStoryHeatByDay
// 的返回值语义（既有调用方与测试零改动）。

const (
	// radarStoryBaselineDays：基线取锚点日**之前** 3 个 CST 日终（不含锚点日）。
	radarStoryBaselineDays = 3
	// radarStoryEvidenceDays：证据窗天数——E3 日终的 48h 窗下界落在 E0-5d。
	radarStoryEvidenceDays = 5
)

// 事件身份回填状态的**板级**三态（雷达侧专用，R-2）。判定只看该版块自己的
// 报告线程**带不带 story_id**，与话题是否锚定到 active 话题无关——候选话题
// 锚定的版块同样有真实事件线，不得被误判成「未回填」而整板剔除。
//
// lane-dynamics 的 stories_backfilled 不能直接复用：它的样本只覆盖
// active ∪ watch-linked 话题锚定的 section（空板与候选话题板都取不到），
// 拿来判雷达事件线会把有事件线的版块说成「未回填」（对用户是假话）。
const (
	// RadarStoryIdentityBackfilled：板内有带 story_id 的线程（事件身份已回填）。
	RadarStoryIdentityBackfilled = "backfilled"
	// RadarStoryIdentityUnbackfilled：板内有非综合稿候选线程但都没带 story_id
	// （确有可回填对象、尚未回填）→ 不产生任何条目，并计入未回填版块数。
	RadarStoryIdentityUnbackfilled = "unbackfilled"
	// RadarStoryIdentityNoCandidate：板内没有可回填对象（无线程，或只有不建
	// 事件身份的综合稿）→ 不计入未回填数（本来就没有事件身份可回填），
	// 也不产生条目；不得呈现为「今天没有事件」。
	RadarStoryIdentityNoCandidate = "no_candidate"
	// RadarStoryIdentityUnknown：判定取数失败（不可判定）→ 保守降级：该板
	// 不出条目，并进降级清单（排序不完整必须明示）。
	RadarStoryIdentityUnknown = "unknown"
)

// radarBoardStoryIdentityCounts counts the board's report threads inside the
// radar window: those carrying a story identity (withIdentity) and those that
// are event candidates without one (candidates — non-composite threads with
// story_id IS NULL; 综合稿永不建事件身份，不算候选).
func (r *TopicGraphRepository) radarBoardStoryIdentityCounts(boardID uint, from, to string) (withIdentity int, candidates int, err error) {
	var row struct {
		WithIdentity int64 `gorm:"column:with_identity"`
		Candidates   int64 `gorm:"column:candidates"`
	}
	if err := r.db.Table("daily_report_threads AS t").
		Select("COALESCE(SUM(CASE WHEN t.story_id IS NOT NULL THEN 1 ELSE 0 END), 0) AS with_identity, "+
			"COALESCE(SUM(CASE WHEN t.story_id IS NULL AND (t.scope IS NULL OR t.scope <> ?) THEN 1 ELSE 0 END), 0) AS candidates",
			ThreadScopeComposite).
		Joins("JOIN board_daily_reports AS r ON r.id = t.report_id").
		Where("r.semantic_board_id = ? AND r.period_date >= ? AND r.period_date <= ?", boardID, from, to).
		Scan(&row).Error; err != nil {
		return 0, 0, fmt.Errorf("radar stories: board %d story identity counts: %w", boardID, err)
	}
	return int(row.WithIdentity), int(row.Candidates), nil
}

// radarBoardStoryIdentityState returns the board-level story-identity state
// (RadarStoryIdentity* 三态) of one board inside the radar window
// [from, to]（YYYY-MM-DD，两端闭区间）.
func (r *TopicGraphRepository) radarBoardStoryIdentityState(boardID uint, from, to string) (string, error) {
	withIdentity, candidates, err := r.radarBoardStoryIdentityCounts(boardID, from, to)
	if err != nil {
		return "", err
	}
	switch {
	case withIdentity > 0:
		return RadarStoryIdentityBackfilled, nil
	case candidates > 0:
		return RadarStoryIdentityUnbackfilled, nil
	default:
		return RadarStoryIdentityNoCandidate, nil
	}
}

// RadarStoryPoint is one event line's radar measurement at a board's own
// anchor day (A_b): the day-end heat, the preceding 3-day baseline, the
// source count, whether the event line has a report ON the anchor day and the
// report the entry can deep-link to.
type RadarStoryPoint struct {
	StoryID      uint
	Title        string
	Participants int
	// Heat is the anchor day's day-end story_heat at full precision (the sort
	// key must not be rounded; the display rounds to one decimal).
	Heat float64
	// BaselineHeat is the mean of the 3 days BEFORE the anchor day (the anchor
	// day itself is not part of its own baseline).
	BaselineHeat float64
	// Ratio is Heat / BaselineHeat — 0 when there is no baseline (the entry
	// then belongs to「今日新起」and must not show a multiplier).
	Ratio float64
	// HasBaseline is false when the event line had no heat evidence in the 3
	// days before the anchor day.
	HasBaseline bool
	// HasTodayEvidence is true when the event line has at least one signal on
	// the anchor day itself (「今天在升温」is a claim about today).
	HasTodayEvidence bool
	// AnchorDate is the board's own anchor day (YYYY-MM-DD) — a lagging board
	// shows its own date, never the global one.
	AnchorDate string
	// ReportDate is the landing report (YYYY-MM-DD); empty = no body thread
	// to deep-link to (the entry is then not clickable).
	ReportDate string
}

// radarDayEnd returns the CST 23:59:59 day end of the given instant's day.
func radarDayEnd(at time.Time) time.Time {
	local := at.In(storyHeatZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 23, 59, 59, 0, storyHeatZone)
}

// radarAnchorInstant parses a radar anchor date (YYYY-MM-DD) as a CST instant.
func radarAnchorInstant(date string) (time.Time, bool) {
	at, err := time.ParseInLocation("2006-01-02", date, storyHeatZone)
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}

// RadarBoardStoryPoints measures every event line of one board at **that
// board's own anchor day** (A_b). The measurement instant is the anchor day's
// CST day end, so a board's event lines and its own today_heat / today_stories
// share one instant.
//
// The baseline is mean(heat(E1), heat(E2), heat(E3)) — the three days BEFORE
// the anchor day. The anchor day is deliberately NOT part of its own baseline:
// including it would make every event line have a baseline and「今日新起」
// unreachable.
//
// Only the existing evidence loader and the existing per-story heat
// computation are used — no second heat formula, no new decay constants.
func (r *TopicGraphRepository) RadarBoardStoryPoints(boardID uint, anchor time.Time) ([]RadarStoryPoint, error) {
	local := anchor.In(storyHeatZone)
	ends := make([]time.Time, 0, radarStoryBaselineDays+1)
	for i := 0; i <= radarStoryBaselineDays; i++ {
		ends = append(ends, radarDayEnd(local.AddDate(0, 0, -i)))
	}
	// Evidence window: heat(E3)'s 48h window reaches back to E0-5d.
	since := ends[radarStoryBaselineDays].AddDate(0, 0, -(radarStoryEvidenceDays - radarStoryBaselineDays))
	evidence, err := r.loadBoardHeatEvidence(boardID, since, ends[0])
	if err != nil {
		return nil, err
	}
	heats := make([]map[uint]float64, len(ends))
	for i, end := range ends {
		heat, _ := evidence.compute(end)
		heats[i] = heat
	}
	participants, todayEvidence := radarStorySources(evidence, ends[0], local)
	// 多跳 merge 链（异常，合并不变式是一跳）：链上**前驱**的身份无法一跳
	// 确定，但它的**最终存活者**本身是干净身份（有标题、有热度、有信号）——
	// 不得连坐丢弃（否则一条真实事件线凭空消失，review R-5）。这里只把前驱
	// 自身标脏（它的热度已归并到存活者，前驱不会成为条目），并留日志标注。
	tainted := make(map[uint]bool)
	ambiguous := make([]uint, 0)
	for id := range evidence.survivors {
		if _, ok := evidence.strict[id]; !ok {
			tainted[id] = true
			ambiguous = append(ambiguous, id)
		}
	}
	if len(ambiguous) > 0 {
		sort.Slice(ambiguous, func(i, j int) bool { return ambiguous[i] < ambiguous[j] })
		logging.Warnf("radar: board %d has %d multi-hop merge predecessor(s) %v — 前驱不作为条目，其存活者照常上榜", boardID, len(ambiguous), ambiguous)
	}
	base1, base2, base3 := heats[1], heats[2], heats[3]
	points := make([]RadarStoryPoint, 0, len(heats[0]))
	for id, heat := range heats[0] {
		if heat <= 0 || tainted[id] {
			continue
		}
		baseline := (base1[id] + base2[id] + base3[id]) / 3
		hasBaseline := baseline > 0
		ratio := 0.0
		if hasBaseline {
			ratio = heat / baseline
		}
		points = append(points, RadarStoryPoint{
			StoryID:          id,
			Title:            evidence.titles[id],
			Participants:     participants[id],
			Heat:             heat,
			BaselineHeat:     baseline,
			Ratio:            ratio,
			HasBaseline:      hasBaseline,
			HasTodayEvidence: todayEvidence[id],
			AnchorDate:       local.Format("2006-01-02"),
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].StoryID < points[j].StoryID })
	return points, nil
}

// radarStorySources counts the distinct participants inside the anchor day's
// 48h window and flags the event lines that have evidence ON the anchor day
// itself. The participant rule mirrors rankReportStoryCandidates: one
// participant counts once per event line. The dedup key is content-identity
// first, feed as fallback (heatParticipantKey) — the same rule as
// board_stories.participant_count.
func radarStorySources(evidence boardHeatEvidence, dayEnd time.Time, dayStart time.Time) (map[uint]int, map[uint]bool) {
	participants := make(map[uint]int)
	today := make(map[uint]bool)
	seen := make(map[storyParticipant]bool)
	windowStart := dayEnd.Add(-StoryHeatWindowHours * time.Hour)
	for _, signal := range evidence.signals {
		id := evidence.survivors[signal.StoryID]
		if id == 0 || signal.ObservedAt.After(dayEnd) || signal.ObservedAt.Before(windowStart) {
			continue
		}
		key := storyParticipant{id, heatParticipantKey(signal.ContentKey, signal.ParticipantKey)}
		if !seen[key] {
			seen[key] = true
			participants[id]++
		}
		if !signal.ObservedAt.Before(dayStart) {
			today[id] = true
		}
	}
	return participants, today
}

// RadarStoryReportDates resolves the landing report of every candidate event
// line: the newest completed report (at or before the board's anchor day)
// that carries a body thread (scope='single', display_tier='body') of that
// event line. Threads written with a pre-merge story id still hit — the id is
// resolved through the one-hop survivor map.
//
// Candidates without such a report get no landing point at all (the client
// renders them as static rows — never a fake jump target).
func (r *TopicGraphRepository) RadarStoryReportDates(boardID uint, anchor time.Time, candidates []uint) (map[uint]string, error) {
	out := make(map[uint]string, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}
	survivorOf := make(map[uint]uint, len(candidates))
	ids := make([]uint, 0, len(candidates))
	for _, id := range candidates {
		survivorOf[id] = id
		ids = append(ids, id)
	}
	// Merged-away ids that resolve one hop to a candidate still count.
	var merged []struct {
		ID         uint
		MergedInto uint
	}
	if err := r.db.Table("board_stories").
		Select("id, merged_into").
		Where("board_id = ? AND merged_into IS NOT NULL AND merged_into IN ?", boardID, candidates).
		Scan(&merged).Error; err != nil {
		return nil, fmt.Errorf("radar stories: board %d merged ids: %w", boardID, err)
	}
	for _, row := range merged {
		if row.ID == row.MergedInto {
			continue
		}
		survivorOf[row.ID] = row.MergedInto
		ids = append(ids, row.ID)
	}
	var threads []struct {
		StoryID    uint
		PeriodDate time.Time
	}
	anchorDate := anchor.In(storyHeatZone).Format("2006-01-02")
	if err := r.db.Table("daily_report_threads AS t").
		Select("t.story_id, r.period_date").
		Joins("JOIN board_daily_reports AS r ON r.id = t.report_id").
		Where("r.semantic_board_id = ? AND r.period_date <= ? AND r.status IN ?", boardID, anchorDate, []string{"completed", "done"}).
		Where("t.scope = ? AND t.display_tier = ? AND t.story_id IN ?", ThreadScopeSingle, ThreadDisplayTierBody, ids).
		Order("r.period_date DESC").
		Scan(&threads).Error; err != nil {
		return nil, fmt.Errorf("radar stories: board %d report threads: %w", boardID, err)
	}
	for _, thread := range threads {
		survivor, ok := survivorOf[thread.StoryID]
		if !ok {
			continue
		}
		if _, done := out[survivor]; done {
			continue
		}
		out[survivor] = NormalizeReportDate(thread.PeriodDate).Format("2006-01-02")
	}
	return out, nil
}
