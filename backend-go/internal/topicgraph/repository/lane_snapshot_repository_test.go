package repository

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

// TestGetBoardLaneDynamics_FullMatrix covers tasks 2.1: 活跃排序、沉寂排除、
// watch 标识、待结算 null、候选门槛、时间线折叠计数。
func TestGetBoardLaneDynamics_FullMatrix(t *testing.T) {
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

// TestGetBoardLaneDynamics_WindowBoundary: 14 天窗口含 13 天前、不含 14 天前
// （锚定 MAX(period_date)，与前端 14 列日期轴同口径）。
func TestGetBoardLaneDynamics_WindowBoundary(t *testing.T) {
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

// TestGetBoardLaneDynamics_NoReports: 无日报 → lanes+candidates 空数组非
// null + has_reports=false（tasks 2.2 第一态）。
func TestGetBoardLaneDynamics_NoReports(t *testing.T) {
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

// TestGetBoardLaneDynamics_ReportsNoActiveLanes: 有日报无活跃泳道 →
// has_reports=true + lanes 空（tasks 2.2 第二态；候选栏照常返回）。
func TestGetBoardLaneDynamics_ReportsNoActiveLanes(t *testing.T) {
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

// TestGetBoardLaneDynamics_SnapshotDetailLevels: 快照 detail 的两级缺失语义
// （lane-trend-overview design D3，test-cases AG-1~AG-4）：
// AG-1 detail 入库 → 聚合携带长版全文；
// AG-2 存量空串 → Detail=nil（空串归一缺失）；
// AG-3 无快照 → snapshot=null（既有语义不变）；
// AG-4 detail=nil 与 snapshot=null 同板共存，JSON 形状可区分。
func TestGetBoardLaneDynamics_SnapshotDetailLevels(t *testing.T) {
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

// TestLaneDynamicsSortsByDistinctDays covers tasks 6.1 / Scenarios 活跃排序按天计
// 与 同日多条只计一天：ordering uses DISTINCT in-window anchor DAYS, never the
// section count (intra-lane theme bucketing would otherwise inflate a lane that
// simply got split more that day). Ties break on the most recent hit date.
func TestLaneDynamicsSortsByDistinctDays(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reports := make(map[int]uint) // daysAgo → report id
	for d := 0; d <= 5; d++ {
		reports[d] = seedTestReport(t, db, boardID, now.AddDate(0, 0, -d))
	}

	assign := func(topicID, reportID uint, labels ...string) {
		for _, label := range labels {
			sec := seedTestSection(t, db, reportID, label)
			assignSection(t, db, sec, topicID)
		}
	}

	// Lane A: 6 distinct days; day 0 is split into 3 sibling sections → 8
	// sections but only 6 days.
	tA := seedLaneDynamicsTopic(t, db, boardID, "六天", TopicStatusActive, 10, now)
	assign(tA.ID, reports[0], "a0-1", "a0-2", "a0-3")
	for d := 1; d <= 5; d++ {
		assign(tA.ID, reports[d], fmt.Sprintf("a%d", d))
	}

	// Lane B: 3 days, one section each.
	tB := seedLaneDynamicsTopic(t, db, boardID, "三天", TopicStatusActive, 10, now)
	for d := 0; d <= 2; d++ {
		assign(tB.ID, reports[d], fmt.Sprintf("b%d", d))
	}

	// Lane C: a single day split into 4 sections → count 4, days 1.
	tC := seedLaneDynamicsTopic(t, db, boardID, "一天四块", TopicStatusActive, 10, now)
	assign(tC.ID, reports[0], "c1", "c2", "c3", "c4")

	// Lane D: 2 older days (last hit 3 days ago).
	tD := seedLaneDynamicsTopic(t, db, boardID, "两天较旧", TopicStatusActive, 10, now)
	assign(tD.ID, reports[3], "d3")
	assign(tD.ID, reports[4], "d4")

	// Lane E: 2 days, last hit today; Lane F: 2 days, last hit yesterday.
	tE := seedLaneDynamicsTopic(t, db, boardID, "两天今天", TopicStatusActive, 10, now)
	assign(tE.ID, reports[0], "e0")
	assign(tE.ID, reports[5], "e5")
	tF := seedLaneDynamicsTopic(t, db, boardID, "两天昨天", TopicStatusActive, 10, now)
	assign(tF.ID, reports[1], "f1")
	assign(tF.ID, reports[2], "f2")

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 6)

	order := make([]uint, len(resp.Lanes))
	for i, lane := range resp.Lanes {
		order[i] = lane.TopicID
	}
	assert.Equal(t, []uint{tA.ID, tB.ID, tE.ID, tF.ID, tD.ID, tC.ID}, order,
		"6d > 3d > (2d,last=today) > (2d,last=yesterday) > (2d,older) > 1d — never by section count")

	byID := make(map[uint]LaneDynamicsLane, len(resp.Lanes))
	for _, lane := range resp.Lanes {
		byID[lane.TopicID] = lane
	}
	assert.Equal(t, 8, byID[tA.ID].SectionCount14d, "section_count_14d stays the raw section count for display")
	assert.Equal(t, 3, byID[tB.ID].SectionCount14d)
	assert.Equal(t, 4, byID[tC.ID].SectionCount14d, "C has more sections than D but fewer days")
}
