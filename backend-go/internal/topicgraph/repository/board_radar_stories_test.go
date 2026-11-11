package repository

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/testutil"
)

// ── 雷达事件线清单（surface-story-events-in-radar）─────────────────────────
//
// 覆盖：基线不含锚点日、基线为 0 → 新起、当日无证据不判升温、跨版块归并与
// 排序键（热度 → 来源数 → 事件线 id）、每版块用自己锚点日、滞后版块仍出条目、
// 未回填版块 0 条目、降级版块 → partial、落点报告日期解析。
//
// 仓储层只用隔离 PostgreSQL（testutil.SetupTestDB），不碰真库。

// radarStoryAnchorDay 返回今天 CST 日历日的零点——版块行与事件线清单共用
// 的锚点日 instant（各版块用自己的锚点日，见 design D2）。
func radarStoryAnchorDay() time.Time {
	local := time.Now().In(storyHeatZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, storyHeatZone)
}

// seedRadarStoryBoard 建一个可进雷达清单的版块：锚点日的日报 + 活跃泳道 +
// 带 story_id 的正文线程（事件身份已回填）。
func seedRadarStoryBoard(t *testing.T, db *gorm.DB, slug string, anchor time.Time) (boardID, storyID, reportID uint) {
	t.Helper()
	boardID = seedTestBoardNamed(t, db, slug)
	reportID = seedTestReport(t, db, boardID, anchor)
	topic := seedLaneDynamicsTopic(t, db, boardID, slug+"-泳道", TopicStatusActive, 9, anchor)
	storyID = seedLaneStoryLine(t, db, boardID, slug+"-线", 2, &topic.ID)
	section := seedTestSection(t, db, reportID, slug+"-sec")
	assignSection(t, db, section, topic.ID)
	seedLaneStoryThread(t, db, reportID, section, slug+"-线程", uintPtr(storyID), ThreadScopeSingle, nil)
	return boardID, storyID, reportID
}

// seedRadarStoryBoardOnCandidateTopic 建一个「日报的 section 锚定在 candidate
// 话题上」的版块：lane-dynamics 的样本（active ∪ watch-linked 话题锚定的
// section）取不到它 → 版块行 anchor 为空、lane 侧 stories_backfilled=false；
// 但它的线程带 story_id（事件身份已回填）→ 雷达侧板级判定为 backfilled，
// 事件线必须照常上榜（review R-2 回归护栏）。
func seedRadarStoryBoardOnCandidateTopic(t *testing.T, db *gorm.DB, slug string, anchor time.Time) (boardID, storyID uint) {
	t.Helper()
	boardID = seedTestBoardNamed(t, db, slug)
	reportID := seedTestReport(t, db, boardID, anchor)
	topic := seedLaneDynamicsTopic(t, db, boardID, slug+"-候选泳道", TopicStatusCandidate, 9, anchor)
	storyID = seedLaneStoryLine(t, db, boardID, slug+"-线", 2, &topic.ID)
	section := seedTestSection(t, db, reportID, slug+"-sec")
	assignSection(t, db, section, topic.ID)
	seedLaneStoryThread(t, db, reportID, section, slug+"-线程", uintPtr(storyID), ThreadScopeSingle, nil)
	return boardID, storyID
}

// radarWeight 是某个 observed_at 相对日终的热度权重（0.5^(Δt/24h)）。
func radarWeight(hours float64) float64 {
	return math.Pow(0.5, hours/StoryHeatHalfLifeHours)
}

// TestRadarBoardStoryPointsBaselineExcludesAnchorDay：基线 = 锚点日**之前** 3
// 个日终的均值（不含锚点日）。含锚点日的分母会把倍率压到 2 倍以下（「今日
// 新起」档也就永远不可达）。
func TestRadarBoardStoryPointsBaselineExcludesAnchorDay(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	boardID, storyID, _ := seedRadarStoryBoard(t, db, "radar-baseline", anchor)

	// 今天 08:00 一位来源，昨天 08:00 另一位来源。
	today := anchor.Add(8 * time.Hour)
	yesterday := anchor.AddDate(0, 0, -1).Add(8 * time.Hour)
	seedHeatSignal(t, db, storyID, 11, today)
	seedHeatSignal(t, db, storyID, 12, yesterday)

	points, err := repo.RadarBoardStoryPoints(boardID, anchor)
	require.NoError(t, err)
	require.Len(t, points, 1)
	point := points[0]
	assert.Equal(t, storyID, point.StoryID)
	assert.Equal(t, anchor.Format("2006-01-02"), point.AnchorDate)
	assert.True(t, point.HasTodayEvidence, "锚点日当天有报道证据")

	// 当日热度 = 今天来源（Δ16h）+ 昨天来源（Δ40h）。
	dayEnd := anchor.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	w16 := radarWeight(dayEnd.Sub(today).Hours())
	w40 := radarWeight(dayEnd.Sub(yesterday).Hours())
	assert.InDelta(t, w16+w40, point.Heat, 1e-9)
	// 基线只含 E1（昨天日终）：mean(E1, E2, E3) = w16/3。
	assert.InDelta(t, w16/3, point.BaselineHeat, 1e-9)
	assert.True(t, point.HasBaseline)
	// 倍率 = 当日热度 ÷ 自身近 3 日基线（≈4.5×；含锚点日的分母只有 1.8×）。
	assert.InDelta(t, 4.5, point.Ratio, 1e-6)
	// 来源数 = 锚点日日终 48h 窗内去重来源数（今天 + 昨天两位）。
	assert.Equal(t, 2, point.Participants)
}

// TestRadarBoardStoryPointsBaselineZeroMeansNewStory：近 3 日无任何热度证据
// → 无基线、倍率不硬凑（0），条目归「今日新起」档。
func TestRadarBoardStoryPointsBaselineZeroMeansNewStory(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	boardID, storyID, _ := seedRadarStoryBoard(t, db, "radar-newstory", anchor)

	seedHeatSignal(t, db, storyID, 21, anchor.Add(8*time.Hour))

	points, err := repo.RadarBoardStoryPoints(boardID, anchor)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.False(t, points[0].HasBaseline, "近 3 日无证据 → 无基线")
	assert.Zero(t, points[0].BaselineHeat)
	assert.Zero(t, points[0].Ratio, "无基线不得硬凑倍率（0 也不是 0.0× 的展示依据）")
	assert.True(t, points[0].HasTodayEvidence)
	assert.Equal(t, 1, points[0].Participants)
}

// TestRadarBoardStoryPointsWithoutTodayEvidence：只有前一日证据的事件线不得
// 进升温/新起档（对「今天」的断言必须有今天的报道）。
func TestRadarBoardStoryPointsWithoutTodayEvidence(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	boardID, storyID, _ := seedRadarStoryBoard(t, db, "radar-noevidence", anchor)

	// 只有前一日 08:00 的证据：当日热度靠 48h 窗内的余温（>0），但当日无报道。
	seedHeatSignal(t, db, storyID, 31, anchor.AddDate(0, 0, -1).Add(8*time.Hour))

	points, err := repo.RadarBoardStoryPoints(boardID, anchor)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.False(t, points[0].HasTodayEvidence, "前一日有证据不算今日证据")
	assert.True(t, points[0].HasBaseline)

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	assert.Zero(t, overview.Stories.WarmingCount, "当日无证据不得计入升温（升温判定要求当日有报道证据）")
	assert.Len(t, overview.Stories.TopStories, 1, "但仍是「今日该看」条目（热度 > 0；单清单不设独立升温/新起档）")
}

// TestRadarOverviewStoryFeedMergesBoardsAndSorts：跨版块归并 + 排序键
// （原始热度降序 → 来源数降序 → 存活事件线 id 升序；排序前不四舍五入）。
func TestRadarOverviewStoryFeedMergesBoardsAndSorts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	// 版块一：两条同热度同来源数的事件线（id 小的在前）+ 一条低热度事件线。
	boardA, storyA1, _ := seedRadarStoryBoard(t, db, "radar-merge-a", anchor)
	topicA := seedLaneDynamicsTopic(t, db, boardA, "radar-merge-a-泳道2", TopicStatusActive, 9, anchor)
	storyA2 := seedLaneStoryLine(t, db, boardA, "radar-merge-a-线2", 2, &topicA.ID)
	storyA3 := seedLaneStoryLine(t, db, boardA, "radar-merge-a-线3", 1, &topicA.ID)
	storyA4 := seedLaneStoryLine(t, db, boardA, "radar-merge-a-线4", 1, &topicA.ID)
	for _, feed := range []uint{41, 42} {
		seedHeatSignal(t, db, storyA1, feed, anchor.AddDate(0, 0, -1).Add(23*time.Hour+59*time.Minute+59*time.Second))
	}
	for _, feed := range []uint{43, 44} {
		seedHeatSignal(t, db, storyA2, feed, anchor.AddDate(0, 0, -1).Add(23*time.Hour+59*time.Minute+59*time.Second))
	}
	seedHeatSignal(t, db, storyA3, 45, anchor.Add(12*time.Hour))
	seedHeatSignal(t, db, storyA4, 48, anchor.AddDate(0, 0, -1).Add(23*time.Hour+59*time.Minute+59*time.Second))

	// 版块二：热度相同但来源数少（1 位来源，锚点日日终当刻）→ 排在同热度两位
	// 来源之后；版块二另一条热度更低 → 同样参与跨版块混排。
	boardB, storyB1, _ := seedRadarStoryBoard(t, db, "radar-merge-b", anchor)
	topicB := seedLaneDynamicsTopic(t, db, boardB, "radar-merge-b-泳道2", TopicStatusActive, 9, anchor)
	storyB2 := seedLaneStoryLine(t, db, boardB, "radar-merge-b-线2", 1, &topicB.ID)
	seedHeatSignal(t, db, storyB1, 46, anchor.Add(23*time.Hour+59*time.Minute+59*time.Second))
	seedHeatSignal(t, db, storyB2, 47, anchor.Add(12*time.Hour))

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories
	// 6 条候选 ≤ 上限 8 → 全部入单清单（matrix-clue-fallback-and-rail-merge：
	// 5→8，不再有独立新起档承接被截条目）。
	require.Len(t, feed.TopStories, 6, "候选 6 条 ≤ 上限 8 → 全部入清单")

	order := make([]uint, 0, len(feed.TopStories))
	for _, item := range feed.TopStories {
		order = append(order, item.StoryID)
	}
	// 热度：storyA1/storyA2/storyB1 = 1.0（Δ24h×2 位来源 / Δ0×1 位来源），
	// storyA3/storyB2 = 0.707（Δ12h），storyA4 = 0.5（Δ24h×1 位）。同热度先比
	// 来源数（2 位在前），再比事件线 id 升序。
	assert.Equal(t, []uint{storyA1, storyA2, storyB1, storyA3, storyB2, storyA4}, order,
		"热度降序；同热度按来源数降序、再按事件线 id 升序")
	assert.Equal(t, boardA, feed.TopStories[0].BoardID)
	assert.Equal(t, "radar-merge-a", feed.TopStories[0].BoardLabel)
	assert.Equal(t, 2, feed.TopStories[0].Participants)
	assert.Equal(t, boardB, feed.TopStories[2].BoardID, "同热度但来源数少 → 排在后面")
	assert.Equal(t, 1, feed.TopStories[2].Participants)
	assert.Equal(t, boardA, feed.TopStories[3].BoardID)
	assert.Equal(t, boardB, feed.TopStories[4].BoardID, "跨版块混排（低热度的版块二条目在列）")
}

// TestRadarOverviewStoryFeedUnbackfilledBoardHasNoEntries：事件身份未回填的
// 版块 0 条目 + 进未回填清单（红线 30：不得冒充「今天没有事件」）。
func TestRadarOverviewStoryFeedUnbackfilledBoardHasNoEntries(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	// 回填版块（线程带 story_id）。
	filledBoard, filledStory, _ := seedRadarStoryBoard(t, db, "radar-filled", anchor)
	seedHeatSignal(t, db, filledStory, 51, anchor.Add(8*time.Hour))

	// 未回填版块：线程不带 story_id。
	plainBoard := seedTestBoardNamed(t, db, "radar-unfilled")
	plainReport := seedTestReport(t, db, plainBoard, anchor)
	plainTopic := seedLaneDynamicsTopic(t, db, plainBoard, "radar-unfilled-泳道", TopicStatusActive, 9, anchor)
	plainSection := seedTestSection(t, db, plainReport, "radar-unfilled-sec")
	assignSection(t, db, plainSection, plainTopic.ID)
	seedLaneStoryThread(t, db, plainReport, plainSection, "未回填线程", nil, ThreadScopeSingle, nil)

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	for _, item := range feed.TopStories {
		assert.NotEqual(t, plainBoard, item.BoardID, "未回填版块不得产生条目")
	}
	assert.Contains(t, feed.UnbackfilledBoardIDs, plainBoard)
	assert.Equal(t, 1, feed.UnbackfilledBoardCount)
	assert.NotContains(t, feed.UnbackfilledBoardIDs, filledBoard)
}

// TestRadarOverviewStoryFeedLaggingBoardKeepsOwnAnchorDate：滞后版块（自己的
// 锚点日 < 全局日）仍出条目，且条目显示自己的锚点日（不冒充全局日）。
func TestRadarOverviewStoryFeedLaggingBoardKeepsOwnAnchorDate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	// 领先版块（锚点日 = 今天）。
	leadBoard, leadStory, _ := seedRadarStoryBoard(t, db, "radar-lead", anchor)
	seedHeatSignal(t, db, leadStory, 61, anchor.Add(8*time.Hour))

	// 滞后版块（锚点日 = 昨天）：自己的日报在昨天。
	lagAnchor := anchor.AddDate(0, 0, -1)
	lagBoard, lagStory, _ := seedRadarStoryBoard(t, db, "radar-lag", lagAnchor)
	seedHeatSignal(t, db, lagStory, 62, lagAnchor.Add(8*time.Hour))

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	assert.Equal(t, anchor.Format("2006-01-02"), feed.AnchorDate)
	assert.Contains(t, feed.LaggingBoardIDs, lagBoard, "锚点日落后于全局日的版块要标出来")
	assert.NotContains(t, feed.LaggingBoardIDs, leadBoard)

	byBoard := map[uint]RadarStoryItem{}
	for _, item := range feed.TopStories {
		byBoard[item.BoardID] = item
	}
	require.Contains(t, byBoard, lagBoard, "滞后版块仍出条目，不得整板缺席")
	assert.Equal(t, lagAnchor.Format("2006-01-02"), byBoard[lagBoard].AnchorDate)
	assert.Equal(t, anchor.Format("2006-01-02"), byBoard[leadBoard].AnchorDate)
}

// TestRadarOverviewStoryFeedPartialOnDegradedBoard：降级（超时）版块 0 条目 +
// partial=true + 降级版块 id 在列（排序不完整必须明示）。
func TestRadarOverviewStoryFeedPartialOnDegradedBoard(t *testing.T) {
	out := &RadarOverview{
		Date: "2026-11-06",
		Boards: []RadarBoardRow{
			{BoardID: 1, Label: "正常版块", ReportDate: "2026-11-06", AnchorDate: "2026-11-06", StoriesBackfilled: true},
			{BoardID: 2, Label: "超时版块", ReportDate: "2026-11-06", AnchorDate: "2026-11-06", StoriesBackfilled: true, DataDelayed: true},
		},
	}
	points := map[uint]radarBoardStoryResult{
		1: {points: []RadarStoryPoint{{StoryID: 11, Title: "甲", Heat: 3, Participants: 2, AnchorDate: "2026-11-06"}}},
		2: {points: []RadarStoryPoint{{StoryID: 22, Title: "乙", Heat: 9, Participants: 5, AnchorDate: "2026-11-06"}}},
	}
	finalizeRadarStories(out, points)

	feed := out.Stories
	require.NotNil(t, feed)
	assert.True(t, feed.Partial)
	assert.Equal(t, []uint{2}, feed.DegradedBoardIDs)
	require.Len(t, feed.TopStories, 1)
	assert.Equal(t, uint(11), feed.TopStories[0].StoryID, "降级版块的条目不参与排序")
	assert.NotContains(t, feed.UnbackfilledBoardIDs, uint(2), "超时版块不是「未回填」")
}

// TestRadarStoryReportDatesResolvesLandingReport：落点报告日期解析（命中 /
// 未命中 / 合并前旧 id 命中）。
func TestRadarStoryReportDatesResolvesLandingReport(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	boardID, storyID, reportID := seedRadarStoryBoard(t, db, "radar-landing", anchor)

	// 合并前的旧 id：线程写旧 id，落点仍命中存活事件线。
	mergedID := seedLaneStoryLine(t, db, boardID, "radar-landing-旧线", 1, nil)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", mergedID).Update("merged_into", storyID).Error)
	section := seedTestSection(t, db, reportID, "radar-landing-sec2")
	seedLaneStoryThread(t, db, reportID, section, "旧 id 线程", uintPtr(mergedID), ThreadScopeSingle, nil)

	dates, err := repo.RadarStoryReportDates(boardID, anchor, []uint{storyID})
	require.NoError(t, err)
	assert.Equal(t, anchor.Format("2006-01-02"), dates[storyID], "正文落点 = 含该事件线 body thread 的最近一期日报")

	// 没有任何正文线程的事件线 → 不造落点。
	orphan := seedLaneStoryLine(t, db, boardID, "radar-landing-孤儿线", 1, nil)
	dates, err = repo.RadarStoryReportDates(boardID, anchor, []uint{orphan})
	require.NoError(t, err)
	assert.Empty(t, dates[orphan], "无正文落点不得编造日期")
}

// TestRadarStoryPointsBudgetOnBusyBoard：实测最坏版块（多条事件线 × 多来源）
// 的事件线计算耗时——雷达端点 5s 预算内必须跑得完（超预算按 partial 降级）。
func TestRadarStoryPointsBudgetOnBusyBoard(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	boardID, _, _ := seedRadarStoryBoard(t, db, "radar-budget", anchor)

	const stories = 60
	const participants = 8
	topic := seedLaneDynamicsTopic(t, db, boardID, "radar-budget-泳道2", TopicStatusActive, 9, anchor)
	feed := uint(1000)
	for s := 0; s < stories; s++ {
		story := seedLaneStoryLine(t, db, boardID, "radar-budget-线", participants, &topic.ID)
		for p := 0; p < participants; p++ {
			seedHeatSignal(t, db, story, feed, anchor.Add(time.Duration(p)*time.Hour))
			feed++
		}
	}

	start := time.Now()
	points, err := repo.RadarBoardStoryPoints(boardID, anchor)
	require.NoError(t, err)
	ids := make([]uint, 0, len(points))
	for _, point := range points {
		ids = append(ids, point.StoryID)
	}
	_, err = repo.RadarStoryReportDates(boardID, anchor, ids)
	require.NoError(t, err)
	elapsed := time.Since(start)

	// 同板对比：版块行（既有开销）与新增事件线开销，验证 5s 端点预算不被击穿。
	rowStart := time.Now()
	row := repo.buildRadarRow(boardID)
	rowElapsed := time.Since(rowStart)

	t.Logf("radar story budget: %d stories × %d participants → %d points；事件线计算 %s，版块行 %s（合计 %s）",
		stories, participants, len(points), elapsed, rowElapsed, elapsed+rowElapsed)
	assert.Len(t, points, stories)
	assert.Less(t, elapsed, 2*time.Second, "单版块事件线计算必须远低于 5s 端点预算")
	assert.Less(t, elapsed+rowElapsed, 4*time.Second, "单版块总开销必须留在 5s 端点预算内")
	assert.NotEmpty(t, row.AnchorDate)
}

// TestRadarOverviewStoryFeedBoardLevelIdentityState（review R-2）：事件身份
// 回填状态改用**板级**判定（三态），不依赖话题是否锚定到 active 话题：
//   - 已回填：板内有带 story_id 的线程 → 照常出条目。
//   - 未回填：有非综合稿候选线程却都没带 story_id → 0 条目 + 进未回填清单。
//   - 无可回填对象：有日报但没有任何线程 → 不计入未回填数（本来就没有事件
//     身份可回填），也不得被呈现为「今天没有事件」。
func TestRadarOverviewStoryFeedBoardLevelIdentityState(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	// 板一：已回填（线程带 story_id）。
	filledBoard, filledStory, _ := seedRadarStoryBoard(t, db, "radar-state-filled", anchor)
	seedHeatSignal(t, db, filledStory, 71, anchor.Add(8*time.Hour))

	// 板二：未回填（候选线程不带 story_id）。
	plainBoard := seedTestBoardNamed(t, db, "radar-state-unfilled")
	plainReport := seedTestReport(t, db, plainBoard, anchor)
	plainTopic := seedLaneDynamicsTopic(t, db, plainBoard, "radar-state-unfilled-泳道", TopicStatusActive, 9, anchor)
	plainSection := seedTestSection(t, db, plainReport, "radar-state-unfilled-sec")
	assignSection(t, db, plainSection, plainTopic.ID)
	seedLaneStoryThread(t, db, plainReport, plainSection, "未回填线程", nil, ThreadScopeSingle, nil)

	// 板三：无可回填对象（有日报、无任何线程）。
	emptyBoard := seedTestBoardNamed(t, db, "radar-state-empty")
	seedTestReport(t, db, emptyBoard, anchor)

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	states := map[uint]string{}
	for _, row := range overview.Boards {
		states[row.BoardID] = row.StoryIdentityState
	}
	assert.Equal(t, RadarStoryIdentityBackfilled, states[filledBoard], "带 story_id 的线程 → 已回填")
	assert.Equal(t, RadarStoryIdentityUnbackfilled, states[plainBoard], "有候选线程但没带 story_id → 未回填")
	assert.Equal(t, RadarStoryIdentityNoCandidate, states[emptyBoard], "无线程 → 无可回填对象")

	assert.Contains(t, feed.UnbackfilledBoardIDs, plainBoard, "未回填版块进未回填清单")
	assert.NotContains(t, feed.UnbackfilledBoardIDs, filledBoard)
	assert.NotContains(t, feed.UnbackfilledBoardIDs, emptyBoard, "无可回填对象不计入未回填数")
	assert.Contains(t, feed.NoStoryBoardIDs, emptyBoard, "无可回填对象单列（不冒充未回填）")
	assert.NotContains(t, feed.NoStoryBoardIDs, plainBoard)
	assert.NotContains(t, feed.NoStoryBoardIDs, filledBoard)

	for _, item := range feed.TopStories {
		assert.NotEqual(t, plainBoard, item.BoardID, "未回填版块不得产生条目")
		assert.NotEqual(t, emptyBoard, item.BoardID, "无可回填对象的版块没有事件线可上榜")
	}
	require.NotEmpty(t, feed.TopStories, "已回填版块照常出条目")
	assert.Equal(t, filledBoard, feed.TopStories[0].BoardID)
}

// TestRadarOverviewStoryFeedCandidateTopicBoardStillListed（review R-2 回归
// 护栏）：日报的 section 锚定在 **candidate 话题**上的版块，lane-dynamics 的
// 样本取不到它（版块行 anchor 为空、lane 侧 stories_backfilled=false），但
// 它的线程带 story_id（事件身份已回填）→ 必须照常进清单，且不得被判成
// 「未回填」（那是对用户说假话）。
func TestRadarOverviewStoryFeedCandidateTopicBoardStillListed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	boardID, storyID := seedRadarStoryBoardOnCandidateTopic(t, db, "radar-candidate-topic", anchor)
	seedHeatSignal(t, db, storyID, 91, anchor.Add(8*time.Hour))

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	var row *RadarBoardRow
	for i := range overview.Boards {
		if overview.Boards[i].BoardID == boardID {
			row = &overview.Boards[i]
		}
	}
	require.NotNil(t, row)
	assert.Empty(t, row.AnchorDate, "lane-dynamics 轴末日：无活跃泳道")
	assert.False(t, row.StoriesBackfilled, "lane-dynamics 样本不含候选话题的 section")
	assert.Equal(t, RadarStoryIdentityBackfilled, row.StoryIdentityState, "板级判定：线程带 story_id → 已回填")

	ids := make([]uint, 0, len(feed.TopStories))
	for _, item := range feed.TopStories {
		ids = append(ids, item.StoryID)
	}
	assert.Contains(t, ids, storyID, "候选话题锚定但事件线已回填的版块必须能进清单")
	assert.NotContains(t, feed.UnbackfilledBoardIDs, boardID, "不得被判成未回填")
	assert.NotContains(t, feed.NoStoryBoardIDs, boardID)
}

// TestRadarOverviewStoryFeedMeasuresAtBoardOwnReportDate（review R-3）：事件线
// 的测量时刻 = 该版块**自己的最新日报日**（不是 lane-dynamics 的轴末日）。
// 最新一期日报的 section 没锚定到 active 话题时版块行 anchor 为空，但事件线
// 仍以该板最新日报日测量并出条目。
func TestRadarOverviewStoryFeedMeasuresAtBoardOwnReportDate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	today := radarStoryAnchorDay()
	// 该板的最新日报在昨天（比全局锚点日滞后一天）。
	boardDay := today.AddDate(0, 0, -1)

	boardID, storyID := seedRadarStoryBoardOnCandidateTopic(t, db, "radar-own-report-day", boardDay)
	seedHeatSignal(t, db, storyID, 92, boardDay.Add(8*time.Hour))

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	var item *RadarStoryItem
	for i := range feed.TopStories {
		if feed.TopStories[i].BoardID == boardID {
			item = &feed.TopStories[i]
		}
	}
	require.NotNil(t, item, "滞后版块仍出条目（不得整板缺席）")
	assert.Equal(t, boardDay.Format("2006-01-02"), item.AnchorDate, "条目带该板自己的锚点日（最新日报日）")
	// 测量时刻 = 该板最新日报日的 CST 日终：信号在当日 08:00 → Δ≈16h → 热度
	// ≈0.63；若错按自然今天测量（Δ≈40h）则只剩 ≈0.31。
	assert.Greater(t, item.Heat, 0.5, "热度必须按该板自己的最新日报日测量")
}

// TestRadarOverviewStoryFeedPartialOnStoryComputationFailure（review R-4）：
// 单版块事件线计算失败 → 该板不参与排序 + partial=true + 进降级版块清单
// （不完整排序必须明示；且不得复用 data_delayed 的「整板数据延迟」语义）。
func TestRadarOverviewStoryFeedPartialOnStoryComputationFailure(t *testing.T) {
	out := &RadarOverview{
		Date: "2026-11-06",
		Boards: []RadarBoardRow{
			{BoardID: 1, Label: "正常版块", ReportDate: "2026-11-06", StoryIdentityState: RadarStoryIdentityBackfilled},
			{BoardID: 2, Label: "计算失败版块", ReportDate: "2026-11-06", StoryIdentityState: RadarStoryIdentityBackfilled},
		},
	}
	results := map[uint]radarBoardStoryResult{
		1: {points: []RadarStoryPoint{{StoryID: 11, Title: "甲", Heat: 3, Participants: 2, AnchorDate: "2026-11-06"}}},
		2: {degraded: true},
	}
	finalizeRadarStories(out, results)

	feed := out.Stories
	require.NotNil(t, feed)
	assert.True(t, feed.Partial, "单版块事件线计算失败 → 本次排序不完整")
	assert.Equal(t, []uint{2}, feed.DegradedBoardIDs, "计算失败的版块进降级清单")
	require.Len(t, feed.TopStories, 1)
	assert.Equal(t, uint(11), feed.TopStories[0].StoryID, "计算失败版块的条目不参与排序")
	assert.NotContains(t, feed.UnbackfilledBoardIDs, uint(2), "计算失败不是「未回填」")
}

// TestRadarBoardStoryPointsKeepsMultiHopSurvivor（review R-5）：多跳 merge 链
// X→Y→Z 时，只剔除链上的**前驱**（它不是独立身份），**不得连坐最终存活者**
// ——Z 本身是干净身份（有标题、有热度、有信号），丢了就是凭空消失一条事件线。
func TestRadarBoardStoryPointsKeepsMultiHopSurvivor(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	boardID, survivor, _ := seedRadarStoryBoard(t, db, "radar-multihop", anchor)

	// 三跳链：X→Y→Z（Z = seedRadarStoryBoard 建的那条存活线）。
	y := seedLaneStoryLine(t, db, boardID, "radar-multihop-Y", 1, nil)
	x := seedLaneStoryLine(t, db, boardID, "radar-multihop-X", 1, nil)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", y).Update("merged_into", survivor).Error)
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", x).Update("merged_into", y).Error)
	seedHeatSignal(t, db, survivor, 81, anchor.Add(8*time.Hour))

	points, err := repo.RadarBoardStoryPoints(boardID, anchor)
	require.NoError(t, err)
	ids := make([]uint, 0, len(points))
	for _, point := range points {
		ids = append(ids, point.StoryID)
	}
	assert.Contains(t, ids, survivor, "多跳链的最终存活者必须照常上榜（不得连坐丢弃）")
	assert.NotContains(t, ids, x, "链上前驱不是独立身份，不作为条目")
	assert.NotContains(t, ids, y, "链上前驱不是独立身份，不作为条目")
}

// ── 三档互斥（deconflict-radar-warming-and-new-stories）─────────────────────
//
// 覆盖：「今日新起」不得重复「今日该读」的条目、新起计数去 Top 后仍为完整数、
// 跨天延续事件线计数（升温空态两成因的服务端判据）、该读不按来源数硬过滤。

// seedRadarExtraStory 在同一版块上追加一条事件线，并灌入 feeds 条当日热度
// 信号（不同 feed = 不同参与者，热度随条数递增）。
func seedRadarExtraStory(t *testing.T, db *gorm.DB, boardID uint, key string, feeds int, at time.Time) uint {
	t.Helper()
	storyID := seedLaneStoryLine(t, db, boardID, key, feeds, nil)
	for i := 0; i < feeds; i++ {
		seedHeatSignal(t, db, storyID, uint(9000+i)+storyID*100, at)
	}
	return storyID
}

// storyIDsOf 把清单压成 id 切片，便于断言集合关系。
func storyIDsOf(items []RadarStoryItem) []uint {
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.StoryID)
	}
	return ids
}

// 空态「没有跨天线 / 有但未升温」的服务端判据。
func TestRadarStoryFeedCrossDayStoryCount(t *testing.T) {
	t.Run("有基线事件线", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		repo := NewTopicGraphRepository(db)
		anchor := radarStoryAnchorDay()

		boardID, firstStory, _ := seedRadarStoryBoard(t, db, "radar-cross-yes", anchor)
		// 昨日有证据 → 有基线（跨天延续）；今日也有证据。
		seedHeatSignal(t, db, firstStory, 401, anchor.AddDate(0, 0, -1).Add(8*time.Hour))
		seedHeatSignal(t, db, firstStory, 402, anchor.Add(8*time.Hour))
		// 另一条同样跨天。
		cross2 := seedLaneStoryLine(t, db, boardID, "radar-cross-2", 1, nil)
		seedHeatSignal(t, db, cross2, 403, anchor.AddDate(0, 0, -1).Add(9*time.Hour))
		seedHeatSignal(t, db, cross2, 404, anchor.Add(9*time.Hour))
		// 一条纯当日（无基线）。
		seedRadarExtraStory(t, db, boardID, "radar-cross-new", 1, anchor.Add(10*time.Hour))
		// 一条锚点日之前就结束的旧线（只剩 48h 残热）——它有基线但没延续到今天，
		// 不得被算作跨天延续（真库实测：只数 HasBaseline 会多报 388 条）。
		stale := seedLaneStoryLine(t, db, boardID, "radar-cross-stale", 1, nil)
		seedHeatSignal(t, db, stale, 405, anchor.AddDate(0, 0, -1).Add(8*time.Hour))

		overview, err := repo.GetBoardRadarOverview(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, overview.Stories.CrossDayStoryCount,
			"只数「既跨天又今天还活着」：两条真正延续的线，不含昨日已结束的旧线")
	})

	t.Run("全部无基线", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		repo := NewTopicGraphRepository(db)
		anchor := radarStoryAnchorDay()

		boardID, firstStory, _ := seedRadarStoryBoard(t, db, "radar-cross-no", anchor)
		seedHeatSignal(t, db, firstStory, 501, anchor.Add(8*time.Hour))
		seedRadarExtraStory(t, db, boardID, "radar-cross-no-2", 2, anchor.Add(8*time.Hour))

		overview, err := repo.GetBoardRadarOverview(context.Background())
		require.NoError(t, err)
		assert.Zero(t, overview.Stories.CrossDayStoryCount, "无跨天延续事件线 → 0")
	})
}

// TestRadarStoryFeedTopStoriesIgnoreParticipantFloor：「今日该读」不按来源数
// 设硬门槛——1 个来源的事件线照样按热度进 Top N。
func TestRadarStoryFeedTopStoriesIgnoreParticipantFloor(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	boardID, firstStory, _ := seedRadarStoryBoard(t, db, "radar-floor", anchor)
	for i := 0; i < 3; i++ {
		seedHeatSignal(t, db, firstStory, uint(600+i), anchor.Add(8*time.Hour))
	}
	for i := 0; i < 4; i++ {
		seedRadarExtraStory(t, db, boardID, fmt.Sprintf("radar-floor-%d", i), 1, anchor.Add(8*time.Hour))
	}

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	feed := overview.Stories
	require.Len(t, feed.TopStories, 5, "造 5 条候选 → 5 条全部入榜（上限 8，未触顶）")

	singles := 0
	for _, item := range feed.TopStories {
		if item.Participants == 1 {
			singles++
		}
	}
	assert.Equal(t, 4, singles, "4 条单来源事件线不得因来源数为 1 被剔除")
}

// TestRadarStoryFeedTopStoriesLimitEight：「今日该读」单清单上限 8（原 5+新起 3
// 合并，matrix-clue-fallback-and-rail-merge D4）：9 条热度递减的候选只列前 8，
// 第 9 条不展示、不附「另有 N 条」类摘要；清单内无重复条目（单清单结构性互斥）。
func TestRadarStoryFeedTopStoriesLimitEight(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	boardID, _, _ := seedRadarStoryBoard(t, db, "radar-top8", anchor)
	topic := seedLaneDynamicsTopic(t, db, boardID, "radar-top8-泳道", TopicStatusActive, 9, anchor)
	// 9 条事件线，热度随序号递减（锚点日内 evidence 越早热度越低）。
	stories := make([]uint, 0, 9)
	for i := 0; i < 9; i++ {
		story := seedLaneStoryLine(t, db, boardID, fmt.Sprintf("radar-top8-线%d", i), 1, &topic.ID)
		stories = append(stories, story)
		seedHeatSignal(t, db, story, uint(6060+i), anchor.Add(time.Duration(22-i)*time.Hour))
	}

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	require.Len(t, feed.TopStories, 8, "上限 8：只列热度前 8 条")
	seen := make(map[uint]bool, len(feed.TopStories))
	for _, item := range feed.TopStories {
		assert.False(t, seen[item.StoryID], "单清单不得重复条目")
		seen[item.StoryID] = true
	}
	assert.Equal(t, stories[:8], storyIDsOf(feed.TopStories), "热度降序取前 8")
	assert.NotContains(t, seen, stories[8], "第 9 条（最冷）不展示")
}

// TestRadarStoryFeedWarmingCountWithoutList：升温清单数组随本变更移除，但
// 全量计数保留（报头「N 处出现升温」消费）；计数不受截断影响、判据仍是
// 事件线自身 ratio ≥ 2 且当日有证据。
func TestRadarStoryFeedWarmingCountWithoutList(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()

	boardID, _, _ := seedRadarStoryBoard(t, db, "radar-warmcnt", anchor)
	// 三条升温线：昨日 1 位来源（基线低）+ 今日 2 位来源（热度高）→ ratio ≥ 2。
	for i := 0; i < 3; i++ {
		story := seedLaneStoryLine(t, db, boardID, fmt.Sprintf("radar-warmcnt-升%d", i), 1, nil)
		seedHeatSignal(t, db, story, uint(7100+i), anchor.AddDate(0, 0, -1).Add(8*time.Hour))
		seedHeatSignal(t, db, story, uint(7200+i), anchor.Add(8*time.Hour))
		seedHeatSignal(t, db, story, uint(7300+i), anchor.Add(9*time.Hour))
	}
	// 两条不升温：一条纯当日（无基线 → 归新起口径，非升温）；一条只剩昨日残热
	// （当日无证据 → 不参与升温判定；heat 是 48h 窗，昨日两位来源今日仍有
	// 残热——昨日今日各一位会因基线取 3 日均而 ratio≈3，勿用该形状）。
	newStory := seedLaneStoryLine(t, db, boardID, "radar-warmcnt-新起", 1, nil)
	seedHeatSignal(t, db, newStory, 7400, anchor.Add(8*time.Hour))
	flat := seedLaneStoryLine(t, db, boardID, "radar-warmcnt-平", 1, nil)
	seedHeatSignal(t, db, flat, 7500, anchor.AddDate(0, 0, -1).Add(8*time.Hour))
	seedHeatSignal(t, db, flat, 7502, anchor.AddDate(0, 0, -1).Add(9*time.Hour))

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.NotNil(t, overview.Stories)
	feed := overview.Stories

	assert.Equal(t, 3, feed.WarmingCount, "升温全量计数（事件线自身 ratio≥2 且当日有证据）")
}

// TestFinalizeRadarStoriesWarmingCountRequiresTodayEvidence：warming_count 与徽标、
// spec、API 文档同口径——有基线且 ratio≥2 **但当日无证据**的事件线不得计入（R-4）。
// 纯逻辑用例（不依赖 DB）：直接构造 finalizeRadarStories 的输入。
func TestFinalizeRadarStoriesWarmingCountRequiresTodayEvidence(t *testing.T) {
	out := &RadarOverview{
		Date:   "2026-11-09",
		Boards: []RadarBoardRow{{BoardID: 1, Label: "板一", ReportDate: "2026-11-09"}},
	}
	results := map[uint]radarBoardStoryResult{
		1: {points: []RadarStoryPoint{
			// 有基线 + ratio≥2，但当日无报道证据 → 不算「今天在升温」
			{StoryID: 1, Heat: 4, BaselineHeat: 1, Ratio: 4, HasBaseline: true, HasTodayEvidence: false},
			// 有基线 + ratio≥2 + 当日有证据 → 计入
			{StoryID: 2, Heat: 4, BaselineHeat: 1, Ratio: 4, HasBaseline: true, HasTodayEvidence: true},
			// 无基线（新起口径）→ 不计入升温，即使 ratio 非零
			{StoryID: 3, Heat: 4, BaselineHeat: 0, Ratio: 4, HasBaseline: false, HasTodayEvidence: true},
		}},
	}

	finalizeRadarStories(out, results)

	require.NotNil(t, out.Stories)
	assert.Equal(t, 1, out.Stories.WarmingCount, "只有「有基线 + ratio≥2 + 当日有证据」计入 warming_count")
	assert.Len(t, out.Stories.TopStories, 3, "计数口径不影响清单条目数（单清单仍收录全部）")
}

// TestRadarStoryParticipantsMatchParticipantCount（fix-duplicate-article-heat
// 2.4）：同一篇内容被两个 feed 各抓一份 → 雷达「N 个来源」与
// board_stories.participant_count 同值（都是 1，不是 2）——雷达来源数与
// participant_count 必须是同一份去重口径。
func TestRadarStoryParticipantsMatchParticipantCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	anchor := radarStoryAnchorDay()
	observed := anchor.Add(8 * time.Hour)
	feedA := seedCoverageFeed(t, db, "radar-dup-a", 60, "ok", &observed)
	feedB := seedCoverageFeed(t, db, "radar-dup-b", 60, "ok", &observed)
	// 同一篇内容的两个 feed 副本（只在 query / fragment 上不同）。
	articleA := seedSignalArticleWithLink(t, db, feedA, "https://example.com/p/1?utm=x", observed)
	articleB := seedSignalArticleWithLink(t, db, feedB, "https://example.com/p/1#comments", observed)

	boardID := seedTestBoardNamed(t, db, "radar-dup")
	reportID := seedTestReport(t, db, boardID, anchor)
	topic := seedLaneDynamicsTopic(t, db, boardID, "radar-dup-泳道", TopicStatusActive, 9, anchor)
	storyID := seedLaneStoryLine(t, db, boardID, "radar-dup-线", 2, &topic.ID)
	section := seedTestSection(t, db, reportID, "radar-dup-sec")
	assignSection(t, db, section, topic.ID)
	seedLaneStoryThread(t, db, reportID, section, "radar-dup-线程", uintPtr(storyID), ThreadScopeSingle, []uint{articleA, articleB})
	require.NoError(t, repo.RecomputeSignalsForStory(storyID))
	assertStoryParticipants(t, db, storyID, 1)

	points, err := repo.RadarBoardStoryPoints(boardID, anchor)
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, 1, points[0].Participants, "雷达来源数 = participant_count = 1")
	// 热度：锚点日日终回看 08:00 的证据 → 0.5^(Δt/24h)，只按 1 个参与者计。
	dayEnd := anchor.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	assert.InDelta(t, radarWeight(dayEnd.Sub(observed).Hours()), points[0].Heat, 1e-9, "热度只按 1 个参与者计")
}
