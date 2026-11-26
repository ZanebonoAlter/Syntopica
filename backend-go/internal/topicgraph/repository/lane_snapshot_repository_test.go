package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── 泳道动态（overview-lane-dynamics）repository 集成测试 ──────────────────
// testcontainer PG via testutil.SetupTestDB（repository 包禁 SQLite —
// standard/backend/testing.md 红线）。

// seedLaneDynamicsTopic creates a persistent topic with the given lifecycle
// fields (mirrors seedLandscapeTopic, local so this file stays readable).
func seedLaneDynamicsTopic(t *testing.T, db *gorm.DB, boardID uint, label, status string, hitCount int, lastSeen time.Time) BoardPersistentTopic {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector([]float64{0}),
		Status:          status,
		Source:          TopicSourceAuto,
		FirstSeenDate:   NormalizeReportDate(lastSeen),
		LastSeenDate:    NormalizeReportDate(lastSeen),
		HitCount:        hitCount,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

// seedLaneThread appends one thread row to a section (insertion order = id
// ASC = the deterministic top-N order the aggregation relies on).
// embedding is a non-nullable vector column — mirror production with a 1-dim
// vector (same as the daily_report_backfill_embeddings_test seeds).
func seedLaneThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, title string) {
	t.Helper()
	require.NoError(t, db.Create(&DailyReportThread{
		ReportID:  reportID,
		SectionID: sectionID,
		Title:     title,
		Embedding: FloatsToPgVector([]float64{0}),
	}).Error)
}

func dateStr(d time.Time) string {
	return NormalizeReportDate(d).Format("2006-01-02")
}

// seedLaneCompositeThread appends one composite (综合稿) thread to a section —
// the 口径 requires composite threads to stay out of every clue count and heat.
func seedLaneCompositeThread(t *testing.T, db *gorm.DB, reportID, sectionID uint) {
	t.Helper()
	require.NoError(t, db.Create(&DailyReportThread{
		ReportID: reportID, SectionID: sectionID, Title: "综合稿",
		Scope: ThreadScopeComposite, Embedding: FloatsToPgVector([]float64{0}),
	}).Error)
}

// seedLaneReportWithStatus creates a report with an explicit status (the
// default in the model is 'generating' — the completed anchor cases need both).
func seedLaneReportWithStatus(t *testing.T, db *gorm.DB, boardID uint, date time.Time, status string) uint {
	t.Helper()
	day := NormalizeReportDate(date)
	for {
		var taken int64
		require.NoError(t, db.Model(&BoardDailyReport{}).
			Where("semantic_board_id = ? AND period_date = ?", boardID, day.Format("2006-01-02")).
			Count(&taken).Error)
		if taken == 0 {
			break
		}
		day = day.AddDate(0, 0, 1)
	}
	report := BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      day,
		Title:           "Test Report",
		Status:          status,
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

// TestLaneDynamicsFullMatrix covers tasks 2.1: 活跃排序、沉寂排除、
// watch 标识、待结算 null、候选门槛、时间线折叠计数。
func TestLaneDynamicsFullMatrix(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)
	report1d := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))
	report20d := seedTestReport(t, db, boardID, now.AddDate(0, 0, -20))

	// T_A active: 2 sections today (one with 7 threads → events=5 folded=2)
	// + 1 section yesterday → count 3. Carries the only settled snapshot.
	tA := seedLaneDynamicsTopic(t, db, boardID, "芯片战", TopicStatusActive, 47, now)
	sA1 := seedTestSection(t, db, reportToday, "chips-today")
	sA2 := seedTestSection(t, db, reportToday, "chips-today-2")
	sA3 := seedTestSection(t, db, report1d, "chips-1d")
	assignSection(t, db, sA1, tA.ID)
	assignSection(t, db, sA2, tA.ID)
	assignSection(t, db, sA3, tA.ID)
	for i := 1; i <= 7; i++ {
		seedLaneThread(t, db, reportToday, sA1, "线索"+string(rune('A'+i-1)))
	}
	seedLaneThread(t, db, reportToday, sA2, "第二节的线索")
	seedLaneThread(t, db, report1d, sA3, "昨天的线索")

	// T_B active + linked by an ACTIVE sentence_topic watch → watch_linked.
	tB := seedLaneDynamicsTopic(t, db, boardID, "我的追踪", TopicStatusActive, 9, now)
	sB1 := seedTestSection(t, db, reportToday, "watch-today")
	assignSection(t, db, sB1, tB.ID)
	require.NoError(t, db.Create(&BoardTopicWatch{
		SemanticBoardID:   boardID,
		Label:             "我的追踪",
		Type:              WatchTypeSentenceTopic,
		Status:            WatchStatusActive,
		PersistentTopicID: &tB.ID,
	}).Error)

	// T_C active but silent: section only 20 days ago → excluded (沉寂).
	tC := seedLaneDynamicsTopic(t, db, boardID, "沉寂话题", TopicStatusActive, 12, now.AddDate(0, 0, -20))
	sC := seedTestSection(t, db, report20d, "silent-old")
	assignSection(t, db, sC, tC.ID)

	// T_D candidate at the default threshold (3) → candidate bar with hint.
	tD := seedLaneDynamicsTopic(t, db, boardID, "待激活", TopicStatusCandidate, 3, now)
	sD := seedTestSection(t, db, reportToday, "candidate-latest")
	assignSection(t, db, sD, tD.ID)

	// T_E candidate below threshold → hidden everywhere.
	tE := seedLaneDynamicsTopic(t, db, boardID, "观察中", TopicStatusCandidate, 1, now)
	sE := seedTestSection(t, db, reportToday, "emerging-section")
	assignSection(t, db, sE, tE.ID)

	// Snapshot only for T_A (T_B stays null → 待结算).
	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: tA.ID,
		RollingSummary:    "过去两周芯片议题持续升温。",
		AsOfDate:          NormalizeReportDate(now),
	}))

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.Equal(t, 14, resp.WindowDays)
	assert.True(t, resp.HasReports)

	// Lane set + ordering: T_A (3 sections) before T_B (1); T_C/E/D absent.
	require.Len(t, resp.Lanes, 2)
	assert.Equal(t, tA.ID, resp.Lanes[0].TopicID)
	assert.Equal(t, tB.ID, resp.Lanes[1].TopicID)

	// watch badge.
	assert.False(t, resp.Lanes[0].WatchLinked)
	assert.True(t, resp.Lanes[1].WatchLinked)

	// Snapshot: settled vs 待结算 null.
	require.NotNil(t, resp.Lanes[0].Snapshot)
	assert.Equal(t, "过去两周芯片议题持续升温。", resp.Lanes[0].Snapshot.Summary)
	assert.Equal(t, dateStr(now), resp.Lanes[0].Snapshot.AsOf)
	assert.Nil(t, resp.Lanes[1].Snapshot)

	// Timeline: newest day first (descending); today has 2 sections;
	// 7-thread section folds.
	tA_lane := resp.Lanes[0]
	require.Len(t, tA_lane.Timeline, 2)
	assert.Equal(t, dateStr(now), tA_lane.Timeline[0].Date)
	assert.Equal(t, dateStr(now.AddDate(0, 0, -1)), tA_lane.Timeline[1].Date)
	require.Len(t, tA_lane.Timeline[0].Sections, 2)
	var foldSection *LaneDynamicsTimelineSection
	for i := range tA_lane.Timeline[0].Sections {
		if tA_lane.Timeline[0].Sections[i].SectionID == sA1 {
			foldSection = &tA_lane.Timeline[0].Sections[i]
		}
	}
	require.NotNil(t, foldSection, "7-thread section must appear in today's timeline")
	require.Len(t, foldSection.Events, 5)
	assert.Equal(t, 2, foldSection.FoldedCount)
	assert.Equal(t, []string{"线索A", "线索B", "线索C", "线索D", "线索E"}, foldSection.Events)
	assert.Equal(t, 3, tA_lane.SectionCount14d)

	// Candidates: T_D visible (hit=3 >= default threshold) with hint;
	// T_E hidden (hit=1 < 3).
	require.Len(t, resp.Candidates, 1)
	assert.Equal(t, tD.ID, resp.Candidates[0].TopicID)
	assert.Equal(t, "待激活", resp.Candidates[0].Label)
	assert.Equal(t, "candidate-latest", resp.Candidates[0].RecentHint)
	assert.Equal(t, dateStr(now), resp.Candidates[0].LastSeenDate)
}

// TestLaneDynamicsSectionCompositeCount（fix-lane-metric-provenance 1.1/1.2）：
// 桶级 composite_count 下发——同桶 5 条线索（2 条综合稿）时，section 的
// thread_count = 5（含综合稿的 thread 总数）、composite_count = 2，
// 该日 clue_count 仍 = 3（综合稿不计入线索合计）。
func TestLaneDynamicsSectionCompositeCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)

	topic := seedLaneDynamicsTopic(t, db, boardID, "综合稿泳道", TopicStatusActive, 9, now)
	bucket := seedTestSection(t, db, reportToday, "综合稿桶")
	assignSection(t, db, bucket, topic.ID)
	for i := 0; i < 3; i++ {
		seedLaneThread(t, db, reportToday, bucket, "普通线索")
	}
	seedLaneCompositeThread(t, db, reportToday, bucket)
	seedLaneCompositeThread(t, db, reportToday, bucket)

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	day := resp.Lanes[0].Timeline[0]
	require.Len(t, day.Sections, 1)
	assert.Equal(t, 5, day.Sections[0].ThreadCount, "thread 总数含综合稿")
	assert.Equal(t, 2, day.Sections[0].CompositeCount, "桶内综合稿数")
	assert.Equal(t, 3, day.ClueCount, "当日线索合计仍排除综合稿")

	// JSON 形状：字段名与 snake_case 约定一致。
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"composite_count":2`)
}

// TestLaneDynamicsAnchorIgnoresUnfinishedReport（fix-lane-metric-provenance
// 1.3/1.4）：「最新日报」锚点统一为 completed 口径——版块有 completed(11-10)
// 与 generating(11-11) 两份日报时，窗口末日 = completed 日，generating
// 报告的分区不进时间线，响应 anchor_date = completed 日。
func TestLaneDynamicsAnchorIgnoresUnfinishedReport(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	completedDay := NormalizeReportDate(now).AddDate(0, 0, -1)
	unfinishedDay := NormalizeReportDate(now)

	completed := seedLaneReportWithStatus(t, db, boardID, completedDay, ReportStatusCompleted)
	unfinished := seedLaneReportWithStatus(t, db, boardID, unfinishedDay, "generating")

	topic := seedLaneDynamicsTopic(t, db, boardID, "锚点泳道", TopicStatusActive, 9, now)
	finishedSection := seedTestSection(t, db, completed, "已完成分区")
	assignSection(t, db, finishedSection, topic.ID)
	seedLaneThread(t, db, completed, finishedSection, "已完成线索")

	pendingSection := seedTestSection(t, db, unfinished, "未完成分区")
	assignSection(t, db, pendingSection, topic.ID)
	seedLaneThread(t, db, unfinished, pendingSection, "未完成线索")

	anchor, ok, err := repo.GetBoardReportAnchor(boardID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, dateStr(completedDay), anchor.Format("2006-01-02"), "锚点只认 completed 日报")

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.Equal(t, dateStr(completedDay), resp.AnchorDate, "响应显式下发 completed 锚点日")
	require.Len(t, resp.Lanes, 1)
	require.Len(t, resp.Lanes[0].Timeline, 1)
	assert.Equal(t, dateStr(completedDay), resp.Lanes[0].Timeline[0].Date, "未完成日报的分区不进时间线")
}

// TestLaneDynamicsWindowBoundary: 14 天窗口含 13 天前、不含 14 天前
// （锚定 MAX(period_date)，与前端 14 列日期轴同口径）。
func TestLaneDynamicsWindowBoundary(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportAnchor := seedTestReport(t, db, boardID, now)
	report13 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -13))
	report14 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -14))

	topic := seedLaneDynamicsTopic(t, db, boardID, "边界话题", TopicStatusActive, 9, now)
	for _, r := range []uint{reportAnchor, report13, report14} {
		s := seedTestSection(t, db, r, "boundary-section")
		assignSection(t, db, s, topic.ID)
	}

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	assert.Equal(t, 2, resp.Lanes[0].SectionCount14d, "13天前含、14天前不含")
	require.Len(t, resp.Lanes[0].Timeline, 2)
	assert.Equal(t, dateStr(now), resp.Lanes[0].Timeline[0].Date, "最新日在最前（倒序）")
	assert.Equal(t, dateStr(now.AddDate(0, 0, -13)), resp.Lanes[0].Timeline[1].Date)
}

// TestLaneDynamicsNoReports: 无日报 → lanes+candidates 空数组非
// null + has_reports=false（tasks 2.2 第一态）。
func TestLaneDynamicsNoReports(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	// A visible candidate exists but no reports at all → still the no-report
	// empty state (candidates also empty per tasks 2.2).
	seedLaneDynamicsTopic(t, db, boardID, "无报候选", TopicStatusCandidate, 5, time.Now())

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.Equal(t, 14, resp.WindowDays)
	assert.False(t, resp.HasReports)
	require.NotNil(t, resp.Lanes)
	assert.Empty(t, resp.Lanes)
	require.NotNil(t, resp.Candidates)
	assert.Empty(t, resp.Candidates)

	// JSON shape: empty arrays, not null.
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"lanes":[]`)
	assert.Contains(t, string(data), `"candidates":[]`)
	assert.Contains(t, string(data), `"has_reports":false`)
}

// TestLaneDynamicsReportsNoActiveLanes: 有日报无活跃泳道 →
// has_reports=true + lanes 空（tasks 2.2 第二态；候选栏照常返回）。
func TestLaneDynamicsReportsNoActiveLanes(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := time.Now()

	// (a) report + no topics at all.
	reportID := seedTestReport(t, db, boardID, now)
	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.True(t, resp.HasReports)
	require.NotNil(t, resp.Lanes)
	assert.Empty(t, resp.Lanes)
	assert.Empty(t, resp.Candidates)

	// (b) visible candidate with an anchored section → lanes stay empty,
	// candidates non-empty.
	cand := seedLaneDynamicsTopic(t, db, boardID, "唯一候选", TopicStatusCandidate, 3, now)
	s := seedTestSection(t, db, reportID, "hint-section")
	assignSection(t, db, s, cand.ID)

	resp2, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.True(t, resp2.HasReports)
	assert.Empty(t, resp2.Lanes)
	require.Len(t, resp2.Candidates, 1)
	assert.Equal(t, "hint-section", resp2.Candidates[0].RecentHint)
}

// TestListLaneSnapshotMaterial_Top3ThreadsAndWindow: 素材查询按 thread id 序
// 取前 3 条，窗口 [from, anchor] 闭区间。
func TestListLaneSnapshotMaterial_Top3ThreadsAndWindow(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportAnchor := seedTestReport(t, db, boardID, now)
	report14 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -14))
	report15 := seedTestReport(t, db, boardID, now.AddDate(0, 0, -15))

	topic := seedLaneDynamicsTopic(t, db, boardID, "素材话题", TopicStatusActive, 9, now)
	sAnchor := seedTestSection(t, db, reportAnchor, "素材-anchor")
	s14 := seedTestSection(t, db, report14, "素材-14d")
	s15 := seedTestSection(t, db, report15, "素材-15d")
	for _, s := range []uint{sAnchor, s14, s15} {
		assignSection(t, db, s, topic.ID)
	}
	for i := 1; i <= 4; i++ {
		seedLaneThread(t, db, reportAnchor, sAnchor, "线索"+string(rune('0'+i)))
	}

	anchor, ok, err := repo.GetBoardReportAnchor(boardID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, dateStr(now), anchor.Format("2006-01-02"))

	rows, err := repo.ListLaneSnapshotMaterial(topic.ID, anchor.AddDate(0, 0, -14), anchor, 3)
	require.NoError(t, err)
	require.Len(t, rows, 2, "15天前不含、14天前与 anchor 含")
	// date ASC ordering.
	assert.Equal(t, "素材-14d", rows[0].SectionLabel)
	assert.Equal(t, "素材-anchor", rows[1].SectionLabel)
	// Top-3 threads only, insertion order.
	require.Len(t, rows[1].ThreadTitles, 3)
	assert.Equal(t, []string{"线索1", "线索2", "线索3"}, rows[1].ThreadTitles)
}

// TestListDailyThreadCounts: 量级信号计数（slim-lane-snapshot-trend D1）——
// 单日多 section 求和、窗口边界日（from/to 闭区间）计入、窗口外排除、
// 无数据空集；同日另一 topic 的 section 不计入。
func TestListDailyThreadCounts(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportAnchor := seedTestReport(t, db, boardID, now)
	reportMid := seedTestReport(t, db, boardID, now.AddDate(0, 0, -3))
	reportFrom := seedTestReport(t, db, boardID, now.AddDate(0, 0, -14))
	reportBefore := seedTestReport(t, db, boardID, now.AddDate(0, 0, -15))

	topic := seedLaneDynamicsTopic(t, db, boardID, "量级话题", TopicStatusActive, 9, now)
	other := seedLaneDynamicsTopic(t, db, boardID, "邻居话题", TopicStatusActive, 5, now)

	// anchor 日：2 sections × (3+2) threads → 5条·2节。
	sA1 := seedTestSection(t, db, reportAnchor, "量级-anchor-1")
	sA2 := seedTestSection(t, db, reportAnchor, "量级-anchor-2")
	assignSection(t, db, sA1, topic.ID)
	assignSection(t, db, sA2, topic.ID)
	for i := 1; i <= 3; i++ {
		seedLaneThread(t, db, reportAnchor, sA1, "线索"+string(rune('0'+i)))
	}
	for i := 1; i <= 2; i++ {
		seedLaneThread(t, db, reportAnchor, sA2, "线索b"+string(rune('0'+i)))
	}
	// 窗首日（from 当日，闭区间边界）：单 section 单 thread。
	sFrom := seedTestSection(t, db, reportFrom, "量级-from")
	assignSection(t, db, sFrom, topic.ID)
	seedLaneThread(t, db, reportFrom, sFrom, "窗首线索")
	// 窗外日（from 前 1 天）：不计入。
	sBefore := seedTestSection(t, db, reportBefore, "量级-out")
	assignSection(t, db, sBefore, topic.ID)
	seedLaneThread(t, db, reportBefore, sBefore, "窗外线索")
	// 同日同 board 的邻居 topic section：不计入本 topic。
	sOther := seedTestSection(t, db, reportMid, "量级-other-topic")
	assignSection(t, db, sOther, other.ID)
	seedLaneThread(t, db, reportMid, sOther, "邻居线索")

	anchor, ok, err := repo.GetBoardReportAnchor(boardID)
	require.NoError(t, err)
	require.True(t, ok)
	from := anchor.AddDate(0, 0, -14)

	counts, err := repo.ListDailyThreadCounts(topic.ID, from, anchor)
	require.NoError(t, err)
	require.Len(t, counts, 2, "仅窗口内两个有 section 的报告日返回行")
	// date ASC：窗首日在前。
	assert.Equal(t, dateStr(now.AddDate(0, 0, -14)), counts[0].PeriodDate.Format("2006-01-02"))
	assert.Equal(t, 1, counts[0].SectionCnt)
	assert.Equal(t, 1, counts[0].ThreadCnt, "窗首日（闭区间边界）计入")
	assert.Equal(t, dateStr(now), counts[1].PeriodDate.Format("2006-01-02"))
	assert.Equal(t, 2, counts[1].SectionCnt, "单日多 section 计数求和")
	assert.Equal(t, 5, counts[1].ThreadCnt)
	// 邻居 topic 独占的报告日（-3d）不在本 topic 计数内。
	for _, c := range counts {
		assert.NotEqual(t, dateStr(now.AddDate(0, 0, -3)), c.PeriodDate.Format("2006-01-02"), "邻居 topic 的 section 不计入本 topic")
	}

	// 无数据 topic：空集非 nil 语义（len==0），调用方补零。
	empty, err := repo.ListDailyThreadCounts(other.ID+9999, from, anchor)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

// TestUpsertLaneSnapshot_IdempotentOverwriteAndUnique: 结算幂等（同行覆盖）
// + unique index 拒绝重复行。
func TestUpsertLaneSnapshot_IdempotentOverwriteAndUnique(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := time.Now()
	topic := seedLaneDynamicsTopic(t, db, boardID, "幂等话题", TopicStatusActive, 9, now)

	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "第一版态势。",
		AsOfDate:          NormalizeReportDate(now),
	}))
	nextDay := NormalizeReportDate(now).AddDate(0, 0, 1)
	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "第二版态势。",
		AsOfDate:          nextDay,
	}))

	var snaps []TopicLaneSnapshot
	require.NoError(t, db.Where("persistent_topic_id = ?", topic.ID).Find(&snaps).Error)
	require.Len(t, snaps, 1, "重复结算覆盖同一行")
	assert.Equal(t, "第二版态势。", snaps[0].RollingSummary)
	assert.Equal(t, nextDay, NormalizeReportDate(snaps[0].AsOfDate))

	// Unique index: a direct second Create must fail (no duplicate rows).
	err := db.Create(&TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "重复行",
		AsOfDate:          nextDay,
	}).Error
	require.Error(t, err, "unique index on persistent_topic_id must reject duplicates")

	// Batch loader.
	m, err := repo.GetLaneSnapshotsByTopicIDs([]uint{topic.ID, topic.ID + 999})
	require.NoError(t, err)
	require.Len(t, m, 1)
	assert.Equal(t, "第二版态势。", m[topic.ID].RollingSummary)
}

// TestLaneSnapshotDeleteCascades: 物理删除话题时快照经 FK ON DELETE CASCADE
// 级联删除（golden schema 的 20260910_0001 迁移产物）。
func TestLaneSnapshotDeleteCascades(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := time.Now()
	topic := seedLaneDynamicsTopic(t, db, boardID, "级联话题", TopicStatusActive, 9, now)
	other := seedLaneDynamicsTopic(t, db, boardID, "旁观话题", TopicStatusActive, 9, now)

	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: topic.ID,
		RollingSummary:    "将被级联删除",
		AsOfDate:          NormalizeReportDate(now),
	}))
	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: other.ID,
		RollingSummary:    "保留",
		AsOfDate:          NormalizeReportDate(now),
	}))

	require.NoError(t, db.Exec("DELETE FROM board_persistent_topics WHERE id = ?", topic.ID).Error)

	var count int64
	require.NoError(t, db.Model(&TopicLaneSnapshot{}).Where("persistent_topic_id = ?", topic.ID).Count(&count).Error)
	assert.Zero(t, count, "snapshot must cascade-delete with its topic")
	require.NoError(t, db.Model(&TopicLaneSnapshot{}).Where("persistent_topic_id = ?", other.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "sibling snapshot untouched")
}

// TestLaneDynamicsSnapshotDetailLevels: 快照 detail 的两级缺失语义
// （lane-trend-overview design D3，test-cases AG-1~AG-4）：
// AG-1 detail 入库 → 聚合携带长版全文；
// AG-2 存量空串 → Detail=nil（空串归一缺失）；
// AG-3 无快照 → snapshot=null（既有语义不变）；
// AG-4 detail=nil 与 snapshot=null 同板共存，JSON 形状可区分。
func TestLaneDynamicsSnapshotDetailLevels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportID := seedTestReport(t, db, boardID, now)

	tFull := seedLaneDynamicsTopic(t, db, boardID, "全长泳道", TopicStatusActive, 9, now)
	tLegacy := seedLaneDynamicsTopic(t, db, boardID, "存量泳道", TopicStatusActive, 9, now)
	tNone := seedLaneDynamicsTopic(t, db, boardID, "无快照泳道", TopicStatusActive, 9, now)
	for _, tp := range []BoardPersistentTopic{tFull, tLegacy, tNone} {
		s := seedTestSection(t, db, reportID, tp.Label+"-section")
		assignSection(t, db, s, tp.ID)
	}

	detailText := "近两周该议题持续升温，多家厂商相继发声，监管层面亦有新动向。"
	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: tFull.ID,
		RollingSummary:    "全长泳道短版。",
		RollingDetail:     detailText,
		AsOfDate:          NormalizeReportDate(now),
	}))
	// 存量行：detail 空串（旧数据只有短版）。
	require.NoError(t, repo.UpsertLaneSnapshot(&TopicLaneSnapshot{
		PersistentTopicID: tLegacy.ID,
		RollingSummary:    "存量泳道仅短版。",
		AsOfDate:          NormalizeReportDate(now),
	}))

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 3)

	byTopic := make(map[uint]LaneDynamicsLane, len(resp.Lanes))
	for _, l := range resp.Lanes {
		byTopic[l.TopicID] = l
	}

	// AG-1: detail 入库 → 聚合响应携带长版全文。
	lFull := byTopic[tFull.ID]
	require.NotNil(t, lFull.Snapshot)
	require.NotNil(t, lFull.Snapshot.Detail, "非空 detail 必须携带全文")
	assert.Equal(t, detailText, *lFull.Snapshot.Detail)

	// AG-2: 存量空串 → Detail=nil（缺失语义），短版照常。
	lLegacy := byTopic[tLegacy.ID]
	require.NotNil(t, lLegacy.Snapshot)
	assert.Nil(t, lLegacy.Snapshot.Detail, "空串必须归一为 nil")
	assert.Equal(t, "存量泳道仅短版。", lLegacy.Snapshot.Summary)

	// AG-3: 无快照泳道 → snapshot=null（既有语义不变）。
	assert.Nil(t, byTopic[tNone.ID].Snapshot)

	// AG-4: 两级缺失同板共存且 JSON 形状可区分：detail=nil 被omitempty
	// 隐去（有 snapshot 对象、无 detail 键），无快照泳道是字面 null。
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	payload := string(data)
	assert.Contains(t, payload, `"summary":"存量泳道仅短版。"`, "detail=nil 泳道有 snapshot 对象")
	assert.NotContains(t, payload, `"detail":null`, "nil detail 必须被 omitempty 隐去而非输出 null")
	assert.Contains(t, payload, `"snapshot":null`, "无快照泳道是字面 null")
	assert.Contains(t, payload, `"detail":"`+detailText+`"`, "AG-1 泳道 JSON 携带 detail 键")
}

// TestLaneDynamicsSortsByLaneHeatThenClueCountThenID（task 1.4 行序）：
// 泳道当天热度降序 → 全窗线索合计降序 → topic id 升序。section 条数、
// 出现天数、事件线数一律不参与排序。
func TestLaneDynamicsSortsByLaneHeatThenClueCountThenID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)

	// HOT：当天 5 条线索（多点热度）；RICH：当天 2 条线索但 1 条由两个桶
	// 共同贡献（section 多、线索少）；POOR：当天 1 条线索。
	tHot := seedLaneDynamicsTopic(t, db, boardID, "热搜泳道", TopicStatusActive, 10, now)
	secHot := seedTestSection(t, db, reportToday, "hot-sec")
	assignSection(t, db, secHot, tHot.ID)
	for i := 0; i < 5; i++ {
		seedLaneThread(t, db, reportToday, secHot, "热度线索")
	}

	// 4 个 section 但只有 2 条线索（section 多 ≠ 线索多）。
	tRich := seedLaneDynamicsTopic(t, db, boardID, "多桶泳道", TopicStatusActive, 10, now)
	for i := 0; i < 4; i++ {
		sec := seedTestSection(t, db, reportToday, fmt.Sprintf("rich-sec-%d", i))
		assignSection(t, db, sec, tRich.ID)
		if i < 2 {
			seedLaneThread(t, db, reportToday, sec, "多桶线索")
		}
	}

	tPoor := seedLaneDynamicsTopic(t, db, boardID, "单线泳道", TopicStatusActive, 10, now)
	secPoor := seedTestSection(t, db, reportToday, "poor-sec")
	assignSection(t, db, secPoor, tPoor.ID)
	seedLaneThread(t, db, reportToday, secPoor, "单线索")

	// 无 thread 的泳道：热度 0、全窗线索 0 → 同值按 id 升序（先建者在前）。
	tZeroA := seedLaneDynamicsTopic(t, db, boardID, "零甲", TopicStatusActive, 10, now)
	assignSection(t, db, seedTestSection(t, db, reportToday, "zero-a"), tZeroA.ID)
	tZeroB := seedLaneDynamicsTopic(t, db, boardID, "零乙", TopicStatusActive, 10, now)
	assignSection(t, db, seedTestSection(t, db, reportToday, "zero-b"), tZeroB.ID)

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 5)

	order := make([]uint, len(resp.Lanes))
	for i, lane := range resp.Lanes {
		order[i] = lane.TopicID
	}
	assert.Equal(t, []uint{tHot.ID, tRich.ID, tPoor.ID, tZeroA.ID, tZeroB.ID}, order,
		"热度降序 → 全窗线索降序 → id；section 数不参与（RICH 有 4 个 section 仍排第二）")

	byID := make(map[uint]LaneDynamicsLane, len(resp.Lanes))
	for _, lane := range resp.Lanes {
		byID[lane.TopicID] = lane
	}
	assert.InDelta(t, 5.0, byID[tHot.ID].LaneHeat, 1e-9)
	assert.InDelta(t, 2.0, byID[tRich.ID].LaneHeat, 1e-9)
	assert.InDelta(t, 1.0, byID[tPoor.ID].LaneHeat, 1e-9)
	assert.Zero(t, byID[tZeroA.ID].LaneHeat)
	assert.Equal(t, 5, byID[tHot.ID].ThreadCount14d)
	assert.Equal(t, 2, byID[tRich.ID].ThreadCount14d)
	assert.Equal(t, 4, byID[tRich.ID].SectionCount14d)
	assert.Equal(t, 1, byID[tPoor.ID].ThreadCount14d)
}

// TestLaneDynamicsDayClueCountAndCandidateHeat（task 1.4 口径）：
// 逐日 clue_count = 各主题桶非综合稿 thread 数之和（不取桶峰值）；
// 候选条目带锚点日线索数与其加权热度；候选权重在线可调即时生效；
// 事件线数 / 最热事件线 / 未归组事件字段彻底不再下发。
func TestLaneDynamicsDayClueCountAndCandidateHeat(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)
	reportYesterday := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	topic := seedLaneDynamicsTopic(t, db, boardID, "线索泳道", TopicStatusActive, 9, now)
	bucketA := seedTestSection(t, db, reportToday, "桶A")
	bucketB := seedTestSection(t, db, reportToday, "桶B")
	assignSection(t, db, bucketA, topic.ID)
	assignSection(t, db, bucketB, topic.ID)
	for i := 0; i < 3; i++ {
		seedLaneThread(t, db, reportToday, bucketA, "线索")
	}
	for i := 0; i < 4; i++ {
		seedLaneThread(t, db, reportToday, bucketB, "线索")
	}
	// 综合稿不计入线索合计。
	seedLaneCompositeThread(t, db, reportToday, bucketB)
	// 昨日：2 条线索（历史日可回算）。
	oldBucket := seedTestSection(t, db, reportYesterday, "昨日桶")
	assignSection(t, db, oldBucket, topic.ID)
	seedLaneThread(t, db, reportYesterday, oldBucket, "昨日线索")
	seedLaneThread(t, db, reportYesterday, oldBucket, "昨日线索")

	// 可见候选（hit 达阈值）带锚点日线索数。
	cand := seedLaneDynamicsTopic(t, db, boardID, "候选泳道", TopicStatusCandidate, 3, now)
	candSec := seedTestSection(t, db, reportToday, "候选桶")
	assignSection(t, db, candSec, cand.ID)
	for i := 0; i < 4; i++ {
		seedLaneThread(t, db, reportToday, candSec, "候选线索")
	}

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.Equal(t, LaneHeatRuleVersion, resp.HeatRuleVersion, "口径版本另立（heat-v2-lane-clues-1d）")

	require.Len(t, resp.Lanes, 1)
	lane := resp.Lanes[0]
	require.Len(t, lane.Timeline, 2)
	assert.Equal(t, dateStr(now), lane.Timeline[0].Date)
	assert.Equal(t, 7, lane.Timeline[0].ClueCount, "3+4 合并计数，不取桶峰值 4；综合稿不计")
	assert.Equal(t, 2, lane.Timeline[1].ClueCount)
	assert.Equal(t, 9, lane.ThreadCount14d, "全窗线索合计 = 7+2")
	assert.InDelta(t, 7.0, lane.LaneHeat, 1e-9, "活跃泳道当天线索数 × 1.0")

	// 逐日载荷不再携带事件线字段（JSON 形状层面确认退役）。
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	payload := string(data)
	for _, retired := range []string{"story_count", "hottest_story_id", "hottest_story_title", "unassigned_event_count", "max_participant_count", "stories_backfilled", "heat_incomplete", "heat_stale_feed_ids"} {
		assert.NotContains(t, payload, retired, "退役字段不得再下发：%s", retired)
	}
	assert.Contains(t, payload, `"clue_count":7`)

	// 候选条目：锚点日线索数 + 候选权重热度。
	require.Len(t, resp.Candidates, 1)
	candidate := resp.Candidates[0]
	assert.Equal(t, dateStr(now), candidate.AnchorDate)
	assert.Equal(t, 4, candidate.ClueCount)
	assert.InDelta(t, 2.0, candidate.LaneHeat, 1e-9, "4 × 默认权重 0.5")

	// 权重在线调整后立即生效；活跃泳道不变。
	require.NoError(t, db.Create(&models.AISettings{Key: LaneHeatCandidateWeightKey, Value: "0.2"}).Error)
	resp, err = repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Candidates, 1)
	assert.InDelta(t, 0.8, resp.Candidates[0].LaneHeat, 1e-9, "4 × 0.2 = 0.8")
	assert.InDelta(t, 7.0, resp.Lanes[0].LaneHeat, 1e-9, "活跃泳道热度不变")
}

// ── 「未入泳道」逐日聚合（lane-matrix-coverage-and-others-row）──────────────
//
// other_topics 口径：窗口内每个 completed 日报日一条；值 = 该日未入泳道的分区数
// 与这些分区的非综合稿线索数。排除集 = 板块 active 话题 ∪ watch 关联话题；
// candidate / archived / 没有 persistent topic 的分区计入（与前端「当日其他
// 话题」组的 A 名单排除同口径）。综合稿不计线索数；未完成日报整天不参与。

// TestOtherTopicsDailyAggregate 覆盖 other_topics 的四条口径：
//  1. 真实 0 值日：有 completed 日报但全部分区已归属 active → 仍出条目 0/0；
//  2. 排除口径：active / watch 关联话题的分区不计入，candidate / archived /
//     persistent_topic_id 为空的分区计入；
//  3. 综合稿不计线索数（分区数照算）；
//  4. 未完成日报整天不参与（连其分区一起不参与）；窗口外的 completed 日报同样不参与。
func TestOtherTopicsDailyAggregate(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	anchorDay := NormalizeReportDate(now)
	prevDay := anchorDay.AddDate(0, 0, -1)
	unfinishedDay := anchorDay.AddDate(0, 0, -2)
	firstWindowDay := anchorDay.AddDate(0, 0, -13) // 窗口首日（锚点前 13 天，闭区间）
	outsideDay := anchorDay.AddDate(0, 0, -14)     // 窗口外（锚点前 14 天）

	// 报告：锚点日 / 前一天（真实 0 值日）/ 未完成 / 窗口外（锚点前 14 天）。
	reportAnchor := seedLaneReportWithStatus(t, db, boardID, anchorDay, ReportStatusCompleted)
	reportPrev := seedLaneReportWithStatus(t, db, boardID, prevDay, ReportStatusCompleted)
	reportUnfinished := seedLaneReportWithStatus(t, db, boardID, unfinishedDay, "generating")
	reportFirst := seedLaneReportWithStatus(t, db, boardID, firstWindowDay, ReportStatusCompleted)
	reportOutside := seedLaneReportWithStatus(t, db, boardID, outsideDay, ReportStatusCompleted)

	// 话题：active（泳道）/ candidate / archived / watch 关联的 candidate。
	activeTopic := seedLaneDynamicsTopic(t, db, boardID, "活跃泳道", TopicStatusActive, 9, anchorDay)
	candidateTopic := seedLaneDynamicsTopic(t, db, boardID, "候选话题", TopicStatusCandidate, 2, anchorDay)
	archivedTopic := seedLaneDynamicsTopic(t, db, boardID, "归档话题", TopicStatusArchived, 2, anchorDay)
	watchedTopic := seedLaneDynamicsTopic(t, db, boardID, "追踪话题", TopicStatusCandidate, 1, anchorDay)
	require.NoError(t, db.Create(&BoardTopicWatch{
		SemanticBoardID:   boardID,
		Label:             "追踪话题",
		Type:              WatchTypeSentenceTopic,
		Status:            WatchStatusActive,
		PersistentTopicID: &watchedTopic.ID,
	}).Error)

	// 锚点日的分区：active（不计）/ candidate（计）/ archived（计）/ 无归属（计）/ watch 关联（不计）。
	sActive := seedTestSection(t, db, reportAnchor, "活跃分区")
	assignSection(t, db, sActive, activeTopic.ID)
	seedLaneThread(t, db, reportAnchor, sActive, "活跃线索")

	sCandidate := seedTestSection(t, db, reportAnchor, "候选分区")
	assignSection(t, db, sCandidate, candidateTopic.ID)
	seedLaneThread(t, db, reportAnchor, sCandidate, "候选线索")
	seedLaneCompositeThread(t, db, reportAnchor, sCandidate) // 综合稿不计线索数

	sArchived := seedTestSection(t, db, reportAnchor, "归档分区")
	assignSection(t, db, sArchived, archivedTopic.ID)
	seedLaneThread(t, db, reportAnchor, sArchived, "归档线索")

	seedTestSection(t, db, reportAnchor, "无归属分区") // persistent_topic_id 为空

	sWatched := seedTestSection(t, db, reportAnchor, "追踪分区")
	assignSection(t, db, sWatched, watchedTopic.ID)
	seedLaneThread(t, db, reportAnchor, sWatched, "追踪线索")

	// 前一天：分区全归属 active → 真实 0 值日。
	sPrev := seedTestSection(t, db, reportPrev, "昨日活跃分区")
	assignSection(t, db, sPrev, activeTopic.ID)
	seedLaneThread(t, db, reportPrev, sPrev, "昨日线索")

	// 未完成日报的分区：不参与（分区数与线索数都不算）。
	sUnfinished := seedTestSection(t, db, reportUnfinished, "未完成分区")
	seedLaneThread(t, db, reportUnfinished, sUnfinished, "未完成线索")

	// 窗口首日（锚点前 13 天）：在窗内 —— 与泳道时间线同一边界（闭区间 14 天）。
	sFirst := seedTestSection(t, db, reportFirst, "窗口首日分区")
	seedLaneThread(t, db, reportFirst, sFirst, "窗口首日线索")

	// 窗口外的 completed 日报：不参与。
	sOutside := seedTestSection(t, db, reportOutside, "窗口外分区")
	seedLaneThread(t, db, reportOutside, sOutside, "窗口外线索")

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.OtherTopics, 3, "窗口内三个 completed 日报日各一条（含窗口首日）")

	// 窗口首日（锚点前 13 天）在窗内 —— 与泳道时间线同一边界。
	assert.Equal(t, LaneDynamicsOtherDay{Date: dateStr(firstWindowDay), Sections: 1, Clues: 1}, resp.OtherTopics[0])

	// 前一天：真实 0 值日（有日报、无未入泳道分区）——不得省略成缺失。
	assert.Equal(t, LaneDynamicsOtherDay{Date: dateStr(prevDay), Sections: 0, Clues: 0}, resp.OtherTopics[1])

	// 锚点日：candidate / archived / 无归属三个分区计入；active / watch 关联不计；
	// 综合稿不计线索数（候选分区 1 普通 + 1 综合稿 → 只算 1 条线索）。
	assert.Equal(t, LaneDynamicsOtherDay{Date: dateStr(anchorDay), Sections: 3, Clues: 2}, resp.OtherTopics[2])

	// 未完成日报 / 窗口外日报都不出条目。
	for _, day := range resp.OtherTopics {
		assert.NotEqual(t, dateStr(unfinishedDay), day.Date, "未完成日报不参与")
		assert.NotEqual(t, dateStr(outsideDay), day.Date, "窗口外日报不参与")
	}

	// 排除集为空（板块没有任何 active/watch 话题）→ 不加 NOT IN 条件，
	// 全部分区都算「未入泳道」。
	emptyBoard := seedTestBoardNamed(t, db, "other-topics-empty-board")
	emptyReport := seedLaneReportWithStatus(t, db, emptyBoard, anchorDay, ReportStatusCompleted)
	sEmpty := seedTestSection(t, db, emptyReport, "无泳道分区")
	seedLaneThread(t, db, emptyReport, sEmpty, "无泳道线索")

	emptyResp, err := repo.GetBoardLaneDynamics(emptyBoard, 14)
	require.NoError(t, err)
	require.Len(t, emptyResp.OtherTopics, 1)
	assert.Equal(t, LaneDynamicsOtherDay{Date: dateStr(anchorDay), Sections: 1, Clues: 1}, emptyResp.OtherTopics[0])
}

// TestOtherTopicsNoAnchorEmptyArray：无锚点（无 completed 日报）→ other_topics
// 是空数组而非 null（与 lanes/candidates 同一空态契约）。
func TestOtherTopicsNoAnchorEmptyArray(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	seedLaneDynamicsTopic(t, db, boardID, "无报话题", TopicStatusActive, 9, time.Now())

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.False(t, resp.HasReports)
	require.NotNil(t, resp.OtherTopics)
	assert.Empty(t, resp.OtherTopics)

	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"other_topics":[]`)
}

// failOnMarkerConnPool 只在 SQL 命中 marker 时失败——把某一条聚合查询的失败
// 精确注入 repository 的取数路径（其余查询照常成功）。
type failOnMarkerConnPool struct {
	gorm.ConnPool
	marker string
}

func (p *failOnMarkerConnPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if strings.Contains(query, p.marker) {
		return nil, errors.New("injected other-topics aggregation failure")
	}
	return p.ConnPool.QueryContext(ctx, query, args...)
}

// TestOtherTopicsAggregationFailurePropagates：other_topics 聚合失败必须向上
// 返回错误（端点 500），不得静默补 0 —— 「算不出来」不是「当天没有其他话题」。
func TestOtherTopicsAggregationFailurePropagates(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportID := seedLaneReportWithStatus(t, db, boardID, NormalizeReportDate(now), ReportStatusCompleted)
	topic := seedLaneDynamicsTopic(t, db, boardID, "活跃泳道", TopicStatusActive, 9, now)
	s := seedTestSection(t, db, reportID, "活跃分区")
	assignSection(t, db, s, topic.ID)

	// 只让 other_topics 的窗口聚合查询失败（marker 是该聚合独有的 SELECT 片段）。
	// Session(Context) 会克隆 Statement——包裹 ConnPool 不会污染共享句柄。
	sess := db.Session(&gorm.Session{Context: context.Background()})
	sess.Statement.ConnPool = &failOnMarkerConnPool{
		ConnPool: sess.Statement.ConnPool,
		marker:   "COUNT(DISTINCT s.id) AS sections",
	}
	repo.db = sess

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.Error(t, err, "聚合失败必须向上返回错误，不得静默 0")
	assert.ErrorContains(t, err, "injected other-topics aggregation failure")
	assert.Nil(t, resp)
}
