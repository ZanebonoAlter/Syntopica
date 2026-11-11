package repository

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

func TestRadarAxisEndsAtAnchor(t *testing.T) {
	axis := radarAxis("2026-10-24")
	if len(axis) != LaneDynamicsDefaultDays {
		t.Fatalf("axis len=%d", len(axis))
	}
	if axis[len(axis)-1] != "2026-10-24" {
		t.Fatalf("axis end=%q", axis[len(axis)-1])
	}
	if axis[0] != "2026-10-11" {
		t.Fatalf("axis start=%q", axis[0])
	}
}

func TestFinalizeRadarSortsByAttentionAndKeepsZeroRows(t *testing.T) {
	out := finalizeRadar(&RadarOverview{Boards: []RadarBoardRow{
		{BoardID: 3, Attention: 0, ReportDate: "2026-10-24"},
		{BoardID: 1, Attention: 30, ReportDate: "2026-10-24"},
		{BoardID: 2, Attention: 12, ReportDate: "2026-10-23"},
	}})
	if out.Date != "2026-10-24" {
		t.Fatalf("date=%q", out.Date)
	}
	got := []uint{out.Boards[0].BoardID, out.Boards[1].BoardID, out.Boards[2].BoardID}
	want := []uint{1, 2, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order=%v want=%v", got, want)
		}
	}
}

// tasks 9.4：所有版块均无日报时顶层 date 回退今天（不冒空串），
// 雷达报头不会拿到空日期。
func TestFinalizeRadarDateFallsBackToToday(t *testing.T) {
	out := finalizeRadar(&RadarOverview{Boards: []RadarBoardRow{
		{BoardID: 1, Attention: 0},
	}})
	today := NormalizeReportDate(time.Now()).Format("2006-01-02")
	if out.Date != today {
		t.Fatalf("date=%q want=%q", out.Date, today)
	}
}

// cstMorning returns 08:00 CST on `now`'s CST calendar day minus daysAgo —
// a fixed distance to the 23:59:59 CST day end so expected heats are
// deterministic regardless of when the test runs.
func cstMorning(now time.Time, daysAgo int) time.Time {
	day := now.In(storyHeatZone).AddDate(0, 0, -daysAgo)
	return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, storyHeatZone)
}

// TestRadarAttentionFollowsLaneHeatNotSectionCount（task 5.2 / test-cases
// 步 8 / spec「关注度不因分区数虚高」）：A 8 sections/1 事件线/2 源，
// B 2 sections/2 事件线/6 源 → B 的 lane_heat 高于 A，雷达排序 B 前 A 后，
// 不得因 A 的 section 数更多而反转；Attention 为一位小数。
func TestRadarAttentionFollowsLaneHeatNotSectionCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()
	morning := cstMorning(now, 0)

	// Board A：8 sections 全挂同一事件线（2 源）。
	boardA := seedTestBoardNamed(t, db, "radar-heat-a")
	reportA := seedTestReport(t, db, boardA, now)
	topicA := seedLaneDynamicsTopic(t, db, boardA, "A泳道", TopicStatusActive, 9, now)
	storyA := seedLaneStoryLine(t, db, boardA, "a", 2, &topicA.ID)
	for i := 0; i < 8; i++ {
		sec := seedTestSection(t, db, reportA, fmt.Sprintf("a-sec-%d", i))
		assignSection(t, db, sec, topicA.ID)
		seedLaneStoryThread(t, db, reportA, sec, "A事件", uintPtr(storyA), ThreadScopeSingle, nil)
	}
	seedHeatSignal(t, db, storyA, 101, morning)
	seedHeatSignal(t, db, storyA, 102, morning)

	// Board B：2 sections、2 条事件线（共 6 源）。
	boardB := seedTestBoardNamed(t, db, "radar-heat-b")
	reportB := seedTestReport(t, db, boardB, now)
	topicB := seedLaneDynamicsTopic(t, db, boardB, "B泳道", TopicStatusActive, 9, now)
	storyB1 := seedLaneStoryLine(t, db, boardB, "b1", 4, &topicB.ID)
	storyB2 := seedLaneStoryLine(t, db, boardB, "b2", 2, &topicB.ID)
	secB1 := seedTestSection(t, db, reportB, "b-sec-1")
	secB2 := seedTestSection(t, db, reportB, "b-sec-2")
	assignSection(t, db, secB1, topicB.ID)
	assignSection(t, db, secB2, topicB.ID)
	seedLaneStoryThread(t, db, reportB, secB1, "B事件一", uintPtr(storyB1), ThreadScopeSingle, nil)
	seedLaneStoryThread(t, db, reportB, secB2, "B事件二", uintPtr(storyB2), ThreadScopeSingle, nil)
	for feed := uint(201); feed <= 204; feed++ {
		seedHeatSignal(t, db, storyB1, feed, morning)
	}
	for feed := uint(205); feed <= 206; feed++ {
		seedHeatSignal(t, db, storyB2, feed, morning)
	}

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.Len(t, overview.Boards, 2)

	// 排序：B（lane_heat 高）在前，不因 A 的 8 个 section 反转。
	assert.Equal(t, boardB, overview.Boards[0].BoardID, "B 关注度更高必须排前")
	assert.Equal(t, boardA, overview.Boards[1].BoardID)
	rowB, rowA := overview.Boards[0], overview.Boards[1]
	assert.Greater(t, rowB.Attention, rowA.Attention)

	// Attention = Σ lane_heat（同一衰减权重 w=0.5^(Δt/72)，A=2w，B=6w），
	// 保留一位小数。
	weight := math.Pow(0.5, laneHeatAnchorEnd(NormalizeReportDate(now)).Sub(morning).Hours()/72)
	assert.InDelta(t, math.Round(2*weight*10)/10, rowA.Attention, 1e-9)
	assert.InDelta(t, math.Round(6*weight*10)/10, rowB.Attention, 1e-9)
	assert.Equal(t, rowA.Attention, math.Round(rowA.Attention*10)/10, "Attention 一位小数")

	// 事件线窗口与 legacy section 窗口并存。
	assert.Equal(t, 8, rowA.TodaySections)
	assert.Equal(t, 2, rowB.TodaySections)
	assert.Equal(t, 1, rowA.TodayStories)
	assert.Equal(t, 2, rowB.TodayStories)
	assert.Equal(t, HeatRuleVersion, rowA.HeatRuleVersion)
	assert.False(t, rowA.HeatIncomplete)
	assert.True(t, rowA.StoriesBackfilled, "窗口内线程带 story_id：事件线口径，雷达行无降级标注")

	// RadarLane.story_counts 与 14 天轴对齐：最后一天 = 当日事件线数。
	require.Len(t, rowA.Lanes, 1)
	require.Len(t, rowA.Lanes[0].StoryCounts, 14)
	assert.Equal(t, 1, rowA.Lanes[0].StoryCounts[13])
	assert.Equal(t, 2, rowB.Lanes[0].StoryCounts[13])
	// legacy counts（section 口径）保留。
	assert.Equal(t, 8, rowA.Lanes[0].Counts[13])
	assert.Equal(t, 2, rowB.Lanes[0].Counts[13])
}

// TestRadarStoryHeatByDayWindowsAndCoverage（task 5.2）：today/last3/prev3
// 热度与 ComputeBoardStoryHeatByDay 逐日求和一致、heat_days 与 14 天轴同
// 序、无 signals 的版块热度全 0 不报错、落后源透出 heat_incomplete。
func TestRadarStoryHeatByDayWindowsAndCoverage(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	board := seedTestBoardNamed(t, db, "radar-heatday")
	reportToday := seedTestReport(t, db, board, now)
	report1d := seedTestReport(t, db, board, now.AddDate(0, 0, -1))
	report2d := seedTestReport(t, db, board, now.AddDate(0, 0, -2))

	topic := seedLaneDynamicsTopic(t, db, board, "热度轴泳道", TopicStatusActive, 9, now)
	s1 := seedLaneStoryLine(t, db, board, "d1", 2, &topic.ID)
	s2 := seedLaneStoryLine(t, db, board, "d2", 1, &topic.ID)
	s3 := seedLaneStoryLine(t, db, board, "d3", 1, &topic.ID)

	secToday := seedTestSection(t, db, reportToday, "today-sec")
	assignSection(t, db, secToday, topic.ID)
	seedLaneStoryThread(t, db, reportToday, secToday, "今日甲", uintPtr(s1), ThreadScopeSingle, nil)
	seedLaneStoryThread(t, db, reportToday, secToday, "今日乙", uintPtr(s2), ThreadScopeSingle, nil)
	sec1d := seedTestSection(t, db, report1d, "d1-sec")
	assignSection(t, db, sec1d, topic.ID)
	seedLaneStoryThread(t, db, report1d, sec1d, "昨日甲", uintPtr(s1), ThreadScopeSingle, nil)
	sec2d := seedTestSection(t, db, report2d, "d2-sec")
	assignSection(t, db, sec2d, topic.ID)
	seedLaneStoryThread(t, db, report2d, sec2d, "前日丙", uintPtr(s3), ThreadScopeSingle, nil)

	// 信号：s1 今晨 08:00（新鲜）；s3 两天前 08:00 + 从未刷新的 feed →
	// 热度不完整标记。
	seedHeatSignal(t, db, s1, 301, cstMorning(now, 0))
	staleFeed := seedCoverageFeed(t, db, "radar-stale", 60, "idle", nil)
	seedHeatSignal(t, db, s3, staleFeed, cstMorning(now, 2))

	row := repo.buildRadarRow(board)
	require.NotEmpty(t, row.AnchorDate)
	assert.Equal(t, dateStr(now), row.AnchorDate)

	// 事件线三窗。
	assert.Equal(t, 2, row.TodayStories)
	assert.Equal(t, 4, row.Last3Stories)
	assert.Zero(t, row.Prev3Stories)

	// 热度三窗 + heat_days：与 ComputeBoardStoryHeatByDay 同轴逐日一致。
	axis := radarAxis(dateStr(now))
	byDay, err := repo.ComputeBoardStoryHeatByDay(board, radarHeatDays(axis))
	require.NoError(t, err)
	require.Len(t, row.HeatDays, 14)
	for i, date := range axis {
		assert.Equal(t, math.Round(byDay[date]*10)/10, row.HeatDays[i], "heat_days[%d] (%s)", i, date)
	}
	assert.Equal(t, math.Round(byDay[axis[13]]*10)/10, row.TodayHeat)
	last3 := byDay[axis[13]] + byDay[axis[12]] + byDay[axis[11]]
	assert.Equal(t, math.Round(last3*10)/10, row.Last3Heat)
	prev3 := byDay[axis[10]] + byDay[axis[9]] + byDay[axis[8]]
	assert.Equal(t, math.Round(prev3*10)/10, row.Prev3Heat)
	assert.Greater(t, row.TodayHeat, 0.0)

	// 落后源透出。
	assert.True(t, row.HeatIncomplete)
	assert.Equal(t, HeatRuleVersion, row.HeatRuleVersion)

	// RadarLane.story_counts 与 timeline story_count 对齐（轴最后三天
	// = 今日 2、昨日 1、前日 1）。
	require.Len(t, row.Lanes, 1)
	counts := row.Lanes[0].StoryCounts
	assert.Equal(t, 2, counts[13])
	assert.Equal(t, 1, counts[12])
	assert.Equal(t, 1, counts[11])
	assert.Zero(t, counts[10])

	// 无 signals 的版块：热度全 0 不报错。
	coldBoard := seedTestBoardNamed(t, db, "radar-cold")
	coldReport := seedTestReport(t, db, coldBoard, now)
	coldTopic := seedLaneDynamicsTopic(t, db, coldBoard, "冷泳道", TopicStatusActive, 9, now)
	coldSec := seedTestSection(t, db, coldReport, "cold-sec")
	assignSection(t, db, coldSec, coldTopic.ID)
	seedLaneStoryThread(t, db, coldReport, coldSec, "冷事件", nil, ThreadScopeSingle, nil)
	coldRow := repo.buildRadarRow(coldBoard)
	for i, v := range coldRow.HeatDays {
		assert.Zero(t, v, "heat_days[%d] 无 signals 应为 0", i)
	}
	assert.Zero(t, coldRow.TodayHeat)
	assert.Zero(t, coldRow.Last3Heat)
	assert.Zero(t, coldRow.Prev3Heat)
	assert.Zero(t, coldRow.Attention)
	assert.False(t, coldRow.HeatIncomplete)
	assert.Equal(t, 1, coldRow.TodayStories, "未回填降级：非综合稿线程数作上界")
	assert.False(t, coldRow.StoriesBackfilled, "未回填降级：today_stories 为非综合稿上界，UI 须标注")
}

// TestRadarTopLanesFilterByStoryCountNotThreadCount（review R-3 /
// specs/workshop-radar 行构成「前 3 条泳道的 14 天事件线密度」）：泳道
// 筛选键是事件线数（StoryCounts 之和），不再是线索数——线索多但事件线
// 少的泳道不得把「线索少但事件线多」的泳道挤出前 3。
// 泳道 A：4 线索全挂同一条事件线（thread=4、story=1）；泳道 B：2 线索
// 挂 2 条事件线（thread=2、story=2）——按线索数 B 排第 4 被截，按事件线
// 数 B 排第 1 在列；C/D 各 3 线索 1 事件线填充截断位。
func TestRadarTopLanesFilterByStoryCountNotThreadCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()
	board := seedTestBoardNamed(t, db, "radar-toplane")
	report := seedTestReport(t, db, board, now)

	// seedTopLane 建 `sections` 条 section 各 1 线索，threads 轮流挂
	// stories——线索数=sections、事件线数=len(stories)。
	seedTopLane := func(label string, sections int, stories ...uint) BoardPersistentTopic {
		topic := seedLaneDynamicsTopic(t, db, board, label, TopicStatusActive, 9, now)
		for i := 0; i < sections; i++ {
			sec := seedTestSection(t, db, report, fmt.Sprintf("%s-sec-%d", label, i))
			assignSection(t, db, sec, topic.ID)
			seedLaneStoryThread(t, db, report, sec, label, uintPtr(stories[i%len(stories)]), ThreadScopeSingle, nil)
		}
		return topic
	}

	// 创建顺序 A→B→C→D ⇒ topic id ASC，同值稳定次序可预期。
	storyA := seedLaneStoryLine(t, db, board, "top-a", 1, nil)
	topicA := seedTopLane("A泳道", 4, storyA)
	storyB1 := seedLaneStoryLine(t, db, board, "top-b1", 1, nil)
	storyB2 := seedLaneStoryLine(t, db, board, "top-b2", 1, nil)
	topicB := seedTopLane("B泳道", 2, storyB1, storyB2)
	storyC := seedLaneStoryLine(t, db, board, "top-c", 1, nil)
	seedTopLane("C泳道", 3, storyC)
	storyD := seedLaneStoryLine(t, db, board, "top-d", 1, nil)
	topicD := seedTopLane("D泳道", 3, storyD)

	row := repo.buildRadarRow(board)
	require.Len(t, row.Lanes, 3, "4 条泳道截前 3")

	// B（线索 2 事件线 2）必须在列且居首；A 在列；D（线索 3 事件线 1）被截。
	assert.Equal(t, topicB.ID, row.Lanes[0].TopicID, "事件线最多的泳道居首")
	assert.Equal(t, 2, sumRadar(row.Lanes[0].StoryCounts), "B 泳道 14 天事件线密度 = 2")
	inLane := func(topicID uint) bool {
		for _, lane := range row.Lanes {
			if lane.TopicID == topicID {
				return true
			}
		}
		return false
	}
	assert.True(t, inLane(topicA.ID), "A（线索多事件线少）仍在列")
	assert.False(t, inLane(topicD.ID), "D 被截：同事件线数时保持传入稳定次序")

	// 交叉验证线索数关系：A 线索 4 > B 线索 2——旧 ThreadCount14d 排序
	// 会把 B 挤出前 3，本测试的排序键不得再是它。
	dynamics, err := repo.GetBoardLaneDynamics(board, LaneDynamicsDefaultDays)
	require.NoError(t, err)
	var threadA, threadB int
	for _, lane := range dynamics.Lanes {
		switch lane.TopicID {
		case topicA.ID:
			threadA = lane.ThreadCount14d
		case topicB.ID:
			threadB = lane.ThreadCount14d
		}
	}
	assert.Equal(t, 4, threadA, "fixture：A 泳道 4 线索")
	assert.Equal(t, 2, threadB, "fixture：B 泳道仅 2 线索（旧排序会把它挤出前 3）")
}
