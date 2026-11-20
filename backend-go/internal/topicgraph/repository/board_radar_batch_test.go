package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/testutil"
)

// ── 批量取数 vs 单版块取数的口径回归（fix-radar-overview-false-zero T1.2）──
//
// 批量路径只换「取数方式」（每版块 N 条 → 所有版块 1 条），口径代码必须完全
// 复用：下面两条用例把两条路径钉在同一批数据上逐字段对比。

// seedRadarBatchBoard 建一块带日报 / 活跃泳道 / 候选泳道 / watch 追踪泳道的版块。
func seedRadarBatchBoard(t *testing.T, db *gorm.DB, slug string, now time.Time) uint {
	t.Helper()
	board := seedTestBoardNamed(t, db, slug)
	reportToday := seedTestReport(t, db, board, now)
	report1d := seedTestReport(t, db, board, now.AddDate(0, 0, -1))

	// 活跃泳道：今天 3 条线索、昨天 2 条。
	active := seedLaneDynamicsTopic(t, db, board, "活跃泳道", TopicStatusActive, 9, now)
	secToday := seedTestSection(t, db, reportToday, "活跃-今天")
	assignSection(t, db, secToday, active.ID)
	for i := 0; i < 3; i++ {
		seedLaneThread(t, db, reportToday, secToday, "活跃线索")
	}
	secYesterday := seedTestSection(t, db, report1d, "活跃-昨天")
	assignSection(t, db, secYesterday, active.ID)
	for i := 0; i < 2; i++ {
		seedLaneThread(t, db, report1d, secYesterday, "昨天的活跃线索")
	}

	// 候选泳道（命中数达门槛 → 进候选条）：今天 4 条线索 → 关注度按权重计入。
	candidate := seedLaneDynamicsTopic(t, db, board, "候选泳道", TopicStatusCandidate, 3, now)
	secCandidate := seedTestSection(t, db, reportToday, "候选-今天")
	assignSection(t, db, secCandidate, candidate.ID)
	for i := 0; i < 4; i++ {
		seedLaneThread(t, db, reportToday, secCandidate, "候选线索")
	}

	// watch 追踪泳道：ACTIVE sentence_topic watch → 进 lane 集合。
	watched := seedLaneDynamicsTopic(t, db, board, "追踪泳道", TopicStatusActive, 9, now)
	secWatched := seedTestSection(t, db, reportToday, "追踪-今天")
	assignSection(t, db, secWatched, watched.ID)
	seedLaneThread(t, db, reportToday, secWatched, "追踪线索")
	require.NoError(t, db.Create(&BoardTopicWatch{
		SemanticBoardID:   board,
		Label:             "追踪泳道",
		Type:              WatchTypeSentenceTopic,
		Status:            WatchStatusActive,
		PersistentTopicID: &watched.ID,
	}).Error)

	// 归档话题：两条路径都必须排除。
	archived := seedLaneDynamicsTopic(t, db, board, "归档泳道", TopicStatusArchived, 9, now)
	secArchived := seedTestSection(t, db, reportToday, "归档-今天")
	assignSection(t, db, secArchived, archived.ID)
	seedLaneThread(t, db, reportToday, secArchived, "归档线索")

	return board
}

// TestBoardLevelBatchMatchesSingleBoard：批量板级取数与单版块取数逐项一致
// —— 锚点日（只认 completed）、非归档话题、watch 关联话题，三者都必须相同。
func TestBoardLevelBatchMatchesSingleBoard(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	boardA := seedRadarBatchBoard(t, db, "radar-batch-a", now)
	boardB := seedRadarBatchBoard(t, db, "radar-batch-b", now)
	// 没有日报的版块：无锚点。
	boardEmpty := seedTestBoardNamed(t, db, "radar-batch-empty")

	ids := []uint{boardA, boardB, boardEmpty}
	levels, err := repo.loadBoardLevels(ids)
	require.NoError(t, err)
	require.Len(t, levels, 3)

	for _, boardID := range ids {
		level := levels[boardID]
		require.NotNil(t, level, "每个请求的版块都要有条目")

		// ① 锚点日 vs GetBoardReportAnchor（completed 口径）。
		anchor, ok, err := repo.GetBoardReportAnchor(boardID)
		require.NoError(t, err)
		assert.Equal(t, ok, level.hasAnchor, "版块 %d 的有无锚点必须一致", boardID)
		if ok {
			assert.Equal(t, anchor, level.anchor, "版块 %d 的锚点日必须一致", boardID)
		}

		// ② watch 关联话题 vs ListActiveWatchLinkedTopicIDs。
		watchLinked, err := repo.ListActiveWatchLinkedTopicIDs(boardID)
		require.NoError(t, err)
		assert.Equal(t, watchLinked, level.watchLinked, "版块 %d 的 watch 关联话题必须一致", boardID)

		// ③ 非归档话题 vs 单版块查询（逐字段比：向量列不参与泳道口径，
		// 批量取数刻意不取——board_persistent_topics 的向量列 41MB/2074 行）。
		var singleTopics []BoardPersistentTopic
		require.NoError(t, db.Where("semantic_board_id = ? AND status != ?", boardID, TopicStatusArchived).
			Find(&singleTopics).Error)
		require.Len(t, level.topics, len(singleTopics), "版块 %d 的话题条数必须一致", boardID)
		for _, want := range singleTopics {
			got, ok := level.topicByID[want.ID]
			require.True(t, ok, "版块 %d 的话题 %d 必须批量取到", boardID, want.ID)
			assert.Equal(t, want.Label, got.Label)
			assert.Equal(t, want.Status, got.Status)
			assert.Equal(t, want.HitCount, got.HitCount)
			assert.Equal(t, want.SemanticBoardID, got.SemanticBoardID)
			assert.True(t, want.LastSeenDate.Equal(got.LastSeenDate))
		}
	}
}

// TestBoardRadarBatchMatchesSingleBoardRow：同一批数据下，批量路径与单版块
// 路径产出的雷达行必须**完全一致**——关注度 / 三窗线索 / 14 天轴 / 泳道清单 /
// 日报摘要一个都不许差（批量路径只换取数方式，不换任何口径）。
func TestBoardRadarBatchMatchesSingleBoardRow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	boardA := seedRadarBatchBoard(t, db, "radar-batch-row-a", now)
	boardB := seedRadarBatchBoard(t, db, "radar-batch-row-b", now)
	boardEmpty := seedTestBoardNamed(t, db, "radar-batch-row-empty")

	ids := []uint{boardA, boardB, boardEmpty}
	levels, err := repo.loadBoardLevels(ids)
	require.NoError(t, err)
	briefs, err := repo.loadLatestReportBriefs(ids)
	require.NoError(t, err)
	batch := &radarBatch{
		levels:   levels,
		briefs:   briefs,
		settings: repo.loadLaneDynamicsSettings(),
	}

	for _, boardID := range ids {
		single := repo.buildRadarRow(boardID)
		multi := repo.buildRadarRowBatch(boardID, batch)

		assert.Equal(t, single.degraded, multi.degraded, "版块 %d 的降级标记必须一致", boardID)
		assert.Equal(t, single.row.AnchorDate, multi.row.AnchorDate, "版块 %d 的锚点日必须一致", boardID)
		assert.Equal(t, single.row.ReportDate, multi.row.ReportDate, "版块 %d 的日报日期必须一致", boardID)
		assert.Equal(t, single.row.ReportTitle, multi.row.ReportTitle)
		assert.Equal(t, single.row.ReportArticle, multi.row.ReportArticle, "版块 %d 的文章数必须一致", boardID)
		assert.Equal(t, single.row.ReportEvent, multi.row.ReportEvent)
		assert.Equal(t, single.row.Attention, multi.row.Attention, "版块 %d 的关注度必须逐位一致", boardID)
		assert.Equal(t, single.row.TodayClues, multi.row.TodayClues, "版块 %d 的当天线索必须一致", boardID)
		assert.Equal(t, single.row.Last3Clues, multi.row.Last3Clues)
		assert.Equal(t, single.row.Prev3Clues, multi.row.Prev3Clues)
		assert.Equal(t, single.row.ClueDays, multi.row.ClueDays, "版块 %d 的 14 天轴必须一致", boardID)
		assert.Equal(t, single.row.Lanes, multi.row.Lanes, "版块 %d 的泳道清单必须一致", boardID)
		assert.Equal(t, single.row.DataUnavailable, multi.row.DataUnavailable)
		assert.Equal(t, single.row.DataDelayed, multi.row.DataDelayed)
		assert.Equal(t, single.row.HeatRuleVersion, multi.row.HeatRuleVersion)
	}

	// 断言值本身不是「全零」——否则上面对比就是空转。
	multiA := repo.buildRadarRowBatch(boardA, batch)
	require.NotNil(t, multiA.row.Attention)
	assert.InDelta(t, 6.0, *multiA.row.Attention, 1e-9,
		"活跃 3 + 追踪 1 + 候选 4×0.5 = 6.0")
	require.NotNil(t, multiA.row.TodayClues)
	assert.Equal(t, 4, *multiA.row.TodayClues, "候选泳道不进线索窗（只有活跃 + 追踪泳道进）")
	require.NotNil(t, multiA.row.Last3Clues)
	assert.Equal(t, 6, *multiA.row.Last3Clues, "近 3 日窗含当天与昨天")
	require.Len(t, multiA.row.ClueDays, LaneDynamicsDefaultDays)
	assert.Equal(t, 4, multiA.row.ClueDays[LaneDynamicsDefaultDays-1])
	assert.Equal(t, 2, multiA.row.ClueDays[LaneDynamicsDefaultDays-2])
	require.Len(t, multiA.row.Lanes, 2, "候选泳道不进 lane 清单，归档泳道被排除")
}

// TestBoardRadarOverviewRowMatchesSingleBoardPath：端点（批量路径）返回的行，
// 与单版块路径算出的行逐字段一致——端点对外的数字没有因为批量化而变。
func TestBoardRadarOverviewRowMatchesSingleBoardPath(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	boardA := seedRadarBatchBoard(t, db, "radar-overview-a", now)
	boardB := seedRadarBatchBoard(t, db, "radar-overview-b", now)

	overview, err := repo.GetBoardRadarOverview(context.Background())
	require.NoError(t, err)
	require.Len(t, overview.Boards, 2)

	byID := make(map[uint]RadarBoardRow, len(overview.Boards))
	for _, row := range overview.Boards {
		byID[row.BoardID] = row
	}
	for _, boardID := range []uint{boardA, boardB} {
		single := repo.buildRadarRow(boardID).row
		got, ok := byID[boardID]
		require.True(t, ok, "端点必须返回版块 %d 的行", boardID)
		assert.Equal(t, single.Attention, got.Attention, "版块 %d 的关注度必须一致", boardID)
		assert.Equal(t, single.TodayClues, got.TodayClues)
		assert.Equal(t, single.Last3Clues, got.Last3Clues)
		assert.Equal(t, single.Prev3Clues, got.Prev3Clues)
		assert.Equal(t, single.ClueDays, got.ClueDays)
		assert.Equal(t, single.Lanes, got.Lanes)
		assert.Equal(t, single.ReportDate, got.ReportDate)
		assert.Equal(t, single.ReportArticle, got.ReportArticle)
		assert.Equal(t, single.ReportEvent, got.ReportEvent)
		assert.Equal(t, single.AnchorDate, got.AnchorDate)
	}
}

// TestBoardLevelBatchAnchorIgnoresUnfinishedReport（review R-7）：存在**更新的
// generating 日报**时，批量锚点必须仍只认 completed 那天——否则批量 SQL 的
// status 过滤一旦漂移，现有夹具（恒 completed）看不出差别。
// 同时钉住 brief 侧的不对称口径：最新日报摘要**不限 status**（与单版块
// GetLatestReportBrief 一致），所以它取 generating 那份。
func TestBoardLevelBatchAnchorIgnoresUnfinishedReport(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	now := time.Now()

	board := seedRadarBatchBoard(t, db, "radar-batch-mixed-status", now)

	// 比已有日报更新一天的 generating 日报（未完成 → 不作锚点）。
	generatingDay := NormalizeReportDate(now).AddDate(0, 0, 1)
	generating := BoardDailyReport{
		SemanticBoardID: board,
		PeriodDate:      generatingDay,
		Title:           "Unfinished Report",
		Summary:         "Unfinished Summary",
		Status:          "generating",
	}
	require.NoError(t, db.Create(&generating).Error)

	levels, err := repo.loadBoardLevels([]uint{board})
	require.NoError(t, err)
	level := levels[board]
	require.NotNil(t, level)

	anchor, ok, err := repo.GetBoardReportAnchor(board)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, level.hasAnchor)
	assert.Equal(t, anchor, level.anchor, "批量锚点必须与单版块一致（只认 completed）")
	assert.NotEqual(t, generatingDay.Format("2006-01-02"), level.anchor.Format("2006-01-02"),
		"generating 日报不得成为锚点")

	briefs, err := repo.loadLatestReportBriefs([]uint{board})
	require.NoError(t, err)
	single, err := repo.GetLatestReportBrief(board)
	require.NoError(t, err)
	require.NotNil(t, single)
	require.NotNil(t, briefs[board])
	assert.Equal(t, single.PeriodDate, briefs[board].PeriodDate, "最新日报摘要两路一致（不限 status）")
	assert.Equal(t, generatingDay.Format("2006-01-02"), briefs[board].PeriodDate,
		"最新日报摘要取最新那份，不筛 completed")
}
