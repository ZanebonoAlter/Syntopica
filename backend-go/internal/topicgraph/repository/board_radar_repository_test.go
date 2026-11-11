package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

func TestBoardRadarAxisEndsAtAnchor(t *testing.T) {
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

func TestBoardRadarSortsByAttentionAndKeepsZeroRows(t *testing.T) {
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
func TestBoardRadarDateFallsBackToToday(t *testing.T) {
	out := finalizeRadar(&RadarOverview{Boards: []RadarBoardRow{
		{BoardID: 1, Attention: 0},
	}})
	today := NormalizeReportDate(time.Now()).Format("2006-01-02")
	if out.Date != today {
		t.Fatalf("date=%q want=%q", out.Date, today)
	}
}

// TestBoardRadarAttentionFollowsLaneHeat（task 1.5 / spec「关注度不因
// 分区数虚高」）：A 8 个 section 但只 2 条线索，B 2 个 section 但有 6 条线索 →
// B 的关注度（泳道当天热度之和）必须高于 A，雷达排序 B 前 A 后；候选泳道按
// lane_heat_candidate_weight 计入关注度。
func TestBoardRadarAttentionFollowsLaneHeat(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	// Board A：8 sections，其中只有 2 条线索（4 条 section 无 thread）。
	boardA := seedTestBoardNamed(t, db, "radar-clue-a")
	reportA := seedTestReport(t, db, boardA, now)
	topicA := seedLaneDynamicsTopic(t, db, boardA, "A泳道", TopicStatusActive, 9, now)
	for i := 0; i < 8; i++ {
		sec := seedTestSection(t, db, reportA, "a-sec")
		assignSection(t, db, sec, topicA.ID)
		if i < 2 {
			seedLaneThread(t, db, reportA, sec, "A线索")
		}
	}
	// 候选泳道 A：当天 4 条线索 → 关注度 +4×0.5 = 2.0。
	topicACand := seedLaneDynamicsTopic(t, db, boardA, "A候选", TopicStatusCandidate, 3, now)
	candSec := seedTestSection(t, db, reportA, "a-cand-sec")
	assignSection(t, db, candSec, topicACand.ID)
	for i := 0; i < 4; i++ {
		seedLaneThread(t, db, reportA, candSec, "候选线索")
	}

	// Board B：2 sections，各 3 条线索 = 6。
	boardB := seedTestBoardNamed(t, db, "radar-clue-b")
	reportB := seedTestReport(t, db, boardB, now)
	topicB := seedLaneDynamicsTopic(t, db, boardB, "B泳道", TopicStatusActive, 9, now)
	for i := 0; i < 2; i++ {
		sec := seedTestSection(t, db, reportB, "b-sec")
		assignSection(t, db, sec, topicB.ID)
		for j := 0; j < 3; j++ {
			seedLaneThread(t, db, reportB, sec, "B线索")
		}
	}

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.Len(t, overview.Boards, 2)

	rowB, rowA := overview.Boards[0], overview.Boards[1]
	assert.Equal(t, boardB, rowB.BoardID, "B 关注度更高必须排前，不因 A 的 8 个 section 反转")
	assert.Equal(t, boardA, rowA.BoardID)
	assert.InDelta(t, 6.0, rowB.Attention, 1e-9)
	assert.InDelta(t, 4.0, rowA.Attention, 1e-9, "活跃 2 条线索 + 候选 4×0.5 = 4.0")
	assert.Equal(t, LaneHeatRuleVersion, rowA.HeatRuleVersion)

	// 当天 / 近 3 日 / 前三日线索窗与 14 天轴。
	assert.Equal(t, 6, rowB.TodayClues)
	assert.Equal(t, 2, rowA.TodayClues, "候选泳道不计入版块线索窗（只有活跃泳道进 lane 轴）")
	assert.Equal(t, 2, rowA.Last3Clues, "近 3 日窗含当天")
	assert.Zero(t, rowA.Prev3Clues)
	require.Len(t, rowA.ClueDays, LaneDynamicsDefaultDays)
	assert.Equal(t, 2, rowA.ClueDays[LaneDynamicsDefaultDays-1])

	// RadarLane 逐日线索密度与 14 天轴对齐。
	require.Len(t, rowB.Lanes, 1)
	require.Len(t, rowB.Lanes[0].ClueCounts, LaneDynamicsDefaultDays)
	assert.Equal(t, 6, rowB.Lanes[0].ClueCounts[LaneDynamicsDefaultDays-1])
	assert.Equal(t, 6, rowB.Lanes[0].ThreadCount14d)

	// 顶层清单：只有活跃泳道条目，带各自版块锚点日与当天线索数。
	require.NotNil(t, overview.TopLanes)
	require.Len(t, overview.TopLanes.Lanes, 2)
	assert.Equal(t, boardB, overview.TopLanes.Lanes[0].BoardID, "线索多的泳道排前")
	assert.Equal(t, 6, overview.TopLanes.Lanes[0].ClueCount)
	assert.Equal(t, 2, overview.TopLanes.Lanes[1].ClueCount)
	assert.Equal(t, dateStr(now), overview.TopLanes.Lanes[0].AnchorDate)
	assert.False(t, overview.TopLanes.Partial)

	// 权重在线调整立即生效：候选 4 × 0.2 = 0.8 → A 关注度 2.8。
	require.NoError(t, db.Create(&models.AISettings{Key: LaneHeatCandidateWeightKey, Value: "0.2"}).Error)
	overview, err = repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.Len(t, overview.Boards, 2)
	assert.InDelta(t, 2.8, overview.Boards[1].Attention, 1e-9)
}

// TestBoardRadarClueWindowsAndHeatRule（task 1.5 口径）：三窗线索数来自逐日线索
// 合计（不掺事件线数），14 天轴与泳道密度同源；事件线热度字段整体退役。
func TestBoardRadarClueWindowsAndHeatRule(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	board := seedTestBoardNamed(t, db, "radar-clue-window")
	reportToday := seedTestReport(t, db, board, now)
	report1d := seedTestReport(t, db, board, now.AddDate(0, 0, -1))
	report2d := seedTestReport(t, db, board, now.AddDate(0, 0, -2))

	topic := seedLaneDynamicsTopic(t, db, board, "轴泳道", TopicStatusActive, 9, now)
	seedClues := func(reportID uint, label string, clues int) {
		sec := seedTestSection(t, db, reportID, label)
		assignSection(t, db, sec, topic.ID)
		for i := 0; i < clues; i++ {
			seedLaneThread(t, db, reportID, sec, "线索")
		}
	}
	seedClues(reportToday, "today-sec", 5)
	seedClues(report1d, "d1-sec", 3)
	seedClues(report2d, "d2-sec", 2)

	result := repo.buildRadarRow(board)
	require.False(t, result.degraded)
	row := result.row
	assert.Equal(t, dateStr(now), row.AnchorDate)
	assert.Equal(t, 5, row.TodayClues)
	assert.Equal(t, 10, row.Last3Clues, "5+3+2")
	assert.Zero(t, row.Prev3Clues)
	assert.Equal(t, LaneHeatRuleVersion, row.HeatRuleVersion)
	assert.InDelta(t, 5.0, row.Attention, 1e-9, "关注度 = 泳道当天热度（5 条线索）")

	require.Len(t, row.Lanes, 1)
	counts := row.Lanes[0].ClueCounts
	assert.Equal(t, 5, counts[13])
	assert.Equal(t, 3, counts[12])
	assert.Equal(t, 2, counts[11])
	assert.Zero(t, counts[10])
	assert.Equal(t, 10, row.Lanes[0].ThreadCount14d)

	// 退役字段不再出现在雷达响应里。
	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	data, err := json.Marshal(overview)
	require.NoError(t, err)
	payload := string(data)
	for _, retired := range []string{"today_stories", "last3_stories", "prev3_stories", "today_heat", "last3_heat", "prev3_heat", "heat_days", "story_counts", "stories_backfilled", "story_identity_state", "heat_incomplete", "top_stories", "warming_count", "cross_day_story_count", "\"stories\""} {
		assert.NotContains(t, payload, retired, "退役字段不得再下发：%s", retired)
	}
}

// TestBoardRadarTopLanesFilterByClueCount（task 1.5 / specs/workshop-radar 行构成）：
// 前 3 条泳道按 14 天线索合计筛选（与 lane-dynamics 次键同口径），section 数
// 不参与——section 多但线索少的泳道不得挤掉线索多的泳道。
func TestBoardRadarTopLanesFilterByClueCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()
	board := seedTestBoardNamed(t, db, "radar-toplane")
	report := seedTestReport(t, db, board, now)

	seedTopLane := func(label string, sections, threadsPerSection int) BoardPersistentTopic {
		topic := seedLaneDynamicsTopic(t, db, board, label, TopicStatusActive, 9, now)
		for i := 0; i < sections; i++ {
			sec := seedTestSection(t, db, report, label+"-sec")
			assignSection(t, db, sec, topic.ID)
			for j := 0; j < threadsPerSection; j++ {
				seedLaneThread(t, db, report, sec, "线索")
			}
		}
		return topic
	}

	topicA := seedTopLane("A泳道", 4, 1) // 4 线索 / 4 section
	topicB := seedTopLane("B泳道", 2, 3) // 6 线索 / 2 section
	topicC := seedTopLane("C泳道", 3, 1) // 3 线索
	topicD := seedTopLane("D泳道", 1, 2) // 2 线索

	result := repo.buildRadarRow(board)
	require.False(t, result.degraded)
	require.Len(t, result.row.Lanes, 3, "4 条泳道截前 3")
	assert.Equal(t, topicB.ID, result.row.Lanes[0].TopicID, "线索最多的泳道居首")
	assert.Equal(t, topicA.ID, result.row.Lanes[1].TopicID)
	assert.Equal(t, topicC.ID, result.row.Lanes[2].TopicID)
	inLane := func(topicID uint) bool {
		for _, lane := range result.row.Lanes {
			if lane.TopicID == topicID {
				return true
			}
		}
		return false
	}
	assert.False(t, inLane(topicD.ID), "线索最少的泳道被截")
}

// TestBoardRadarTopLanesSortLimitAndDegradation 覆盖清单段的纯函数规则：
// 线索数降序 → 稳定键（版块名 → 泳道名 → 泳道 id）→ 上限 8；滞后版块保留
// 自己的锚点日；延迟 / 计算失败的版块缺席并把清单标记为不完整。
func TestBoardRadarTopLanesSortLimitAndDegradation(t *testing.T) {
	rows := []RadarBoardRow{
		{BoardID: 1, Label: "乙版块", ReportDate: "2026-11-06", AnchorDate: "2026-11-06"},
		{BoardID: 2, Label: "甲版块", ReportDate: "2026-11-05", AnchorDate: "2026-11-05"}, // 滞后
		{BoardID: 3, Label: "丙版块", ReportDate: "2026-11-06", DataDelayed: true},
		{BoardID: 4, Label: "丁版块", ReportDate: "2026-11-06"},
	}
	results := map[uint]radarBoardLanes{
		1: {anchorDate: "2026-11-06", lanes: []RadarLane{
			{TopicID: 11, Label: "乙一", ClueCounts: append(make([]int, 13), 8)},
			{TopicID: 12, Label: "乙二", ClueCounts: append(make([]int, 13), 8)},
		}},
		2: {anchorDate: "2026-11-05", lanes: []RadarLane{
			{TopicID: 21, Label: "甲一", ClueCounts: append(make([]int, 13), 12)},
		}},
		3: {degraded: true},
		4: {anchorDate: "2026-11-06", degraded: true},
	}
	out := finalizeRadar(&RadarOverview{Boards: rows})
	out.Boards = rows
	finalizeRadarTopLanes(out, results)

	require.NotNil(t, out.TopLanes)
	assert.True(t, out.TopLanes.Partial, "延迟 / 计算失败的版块让清单不完整")
	assert.Equal(t, []uint{3, 4}, out.TopLanes.DegradedBoardIDs)
	assert.Equal(t, []uint{2}, out.TopLanes.LaggingBoardIDs)

	require.Len(t, out.TopLanes.Lanes, 3)
	assert.Equal(t, uint(21), out.TopLanes.Lanes[0].TopicID, "12 条线索最多")
	assert.Equal(t, "甲版块", out.TopLanes.Lanes[0].BoardLabel)
	assert.Equal(t, "2026-11-05", out.TopLanes.Lanes[0].AnchorDate, "滞后版块显示自己的锚点日")
	// 同为 8 条线索：稳定键按版块名（同为乙版块）→ 泳道名升序（乙一 < 乙二）。
	assert.Equal(t, uint(11), out.TopLanes.Lanes[1].TopicID)
	assert.Equal(t, uint(12), out.TopLanes.Lanes[2].TopicID)

	// 上限 8：第 9 名不展示。
	many := make([]RadarLane, 0, 9)
	for i := 1; i <= 9; i++ {
		clues := make([]int, 14)
		clues[13] = 20 - i
		many = append(many, RadarLane{TopicID: uint(i), Label: "泳道", ClueCounts: clues})
	}
	out = finalizeRadar(&RadarOverview{Boards: []RadarBoardRow{{BoardID: 9, Label: "版块", ReportDate: "2026-11-06"}}})
	out.Boards = []RadarBoardRow{{BoardID: 9, Label: "版块", ReportDate: "2026-11-06"}}
	finalizeRadarTopLanes(out, map[uint]radarBoardLanes{9: {anchorDate: "2026-11-06", lanes: many}})
	require.Len(t, out.TopLanes.Lanes, radarTopLaneLimit)
	assert.Equal(t, uint(1), out.TopLanes.Lanes[0].TopicID)
	assert.Equal(t, 19, out.TopLanes.Lanes[0].ClueCount)
	assert.False(t, out.TopLanes.Partial)
}
