package repository

import (
	"encoding/json"
	"fmt"
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

// TestLaneDynamicsSortsByLaneHeatThenStoryCountThenID covers task 5.1 行序：
// lane_heat 降序 → story_count_14d 降序 → topic id 升序（add-event-story-heat
// spec；旧「活跃天数→最近命中日→id」排序已退役，section/天几何不得决定
// 行序）。热度泳道（1 section）必须排在多 section 无热度泳道之前。
func TestLaneDynamicsSortsByLaneHeatThenStoryCountThenID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)

	// Heat morning instant (CST 08:00 today): fixed distance to the CST
	// 23:59:59 anchor end, so the expected lane_heat is deterministic
	// regardless of when the test runs.
	todayCST := now.In(storyHeatZone)
	morning := time.Date(todayCST.Year(), todayCST.Month(), todayCST.Day(), 8, 0, 0, 0, storyHeatZone)

	// Lane HOT: 1 section, 1 story, 2 feed signals → lane_heat > 0.
	tHot := seedLaneDynamicsTopic(t, db, boardID, "热度泳道", TopicStatusActive, 10, now)
	secHot := seedTestSection(t, db, reportToday, "hot-sec")
	assignSection(t, db, secHot, tHot.ID)
	storyHot := seedLaneStoryLine(t, db, boardID, "hot", 2, &tHot.ID)
	seedLaneStoryThread(t, db, reportToday, secHot, "热度事件", &storyHot, ThreadScopeSingle, nil)
	seedHeatSignal(t, db, storyHot, 51, morning)
	seedHeatSignal(t, db, storyHot, 52, morning)

	// Lane RICH: 2 sections, 2 distinct stories, no signals → heat 0,
	// story_count_14d = 2. More sections than HOT — must NOT outrank it.
	tRich := seedLaneDynamicsTopic(t, db, boardID, "双事件泳道", TopicStatusActive, 10, now)
	storyR1 := seedLaneStoryLine(t, db, boardID, "r1", 1, &tRich.ID)
	storyR2 := seedLaneStoryLine(t, db, boardID, "r2", 1, &tRich.ID)
	for i, story := range []uint{storyR1, storyR2} {
		sec := seedTestSection(t, db, reportToday, fmt.Sprintf("rich-sec-%d", i))
		assignSection(t, db, sec, tRich.ID)
		seedLaneStoryThread(t, db, reportToday, sec, "rich事件", &story, ThreadScopeSingle, nil)
	}

	// Lane POOR: 1 section, 1 story → story_count_14d = 1.
	tPoor := seedLaneDynamicsTopic(t, db, boardID, "单事件泳道", TopicStatusActive, 10, now)
	secPoor := seedTestSection(t, db, reportToday, "poor-sec")
	assignSection(t, db, secPoor, tPoor.ID)
	storyPoor := seedLaneStoryLine(t, db, boardID, "p1", 1, &tPoor.ID)
	seedLaneStoryThread(t, db, reportToday, secPoor, "单事件", &storyPoor, ThreadScopeSingle, nil)

	// Two zero-story lanes (sections without threads) → tie → id ASC.
	tZeroA := seedLaneDynamicsTopic(t, db, boardID, "零甲", TopicStatusActive, 10, now)
	secZeroA := seedTestSection(t, db, reportToday, "zero-a")
	assignSection(t, db, secZeroA, tZeroA.ID)
	tZeroB := seedLaneDynamicsTopic(t, db, boardID, "零乙", TopicStatusActive, 10, now)
	secZeroB := seedTestSection(t, db, reportToday, "zero-b")
	assignSection(t, db, secZeroB, tZeroB.ID)

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 5)

	order := make([]uint, len(resp.Lanes))
	for i, lane := range resp.Lanes {
		order[i] = lane.TopicID
	}
	assert.Equal(t, []uint{tHot.ID, tRich.ID, tPoor.ID, tZeroA.ID, tZeroB.ID}, order,
		"lane_heat 高者在前；无热度按 story_count_14d；再按 id —— 与 section 数无关")

	byID := make(map[uint]LaneDynamicsLane, len(resp.Lanes))
	for _, lane := range resp.Lanes {
		byID[lane.TopicID] = lane
	}
	// lane_heat：热度泳道 > 0 且不超过两位参与者当日贡献；其余为 0。
	assert.Greater(t, byID[tHot.ID].LaneHeat, 0.0)
	assert.LessOrEqual(t, byID[tHot.ID].LaneHeat, 2.0)
	for _, id := range []uint{tRich.ID, tPoor.ID, tZeroA.ID, tZeroB.ID} {
		assert.Zero(t, byID[id].LaneHeat, "无 signals 的泳道 lane_heat 为 0")
	}
	assert.Equal(t, 1, byID[tHot.ID].StoryCount14d)
	assert.Equal(t, 2, byID[tRich.ID].StoryCount14d)
	assert.Equal(t, 1, byID[tPoor.ID].StoryCount14d)
	// section 计数仅展示：RICH(2) > HOT(1) 但排名靠后。
	assert.Equal(t, 1, byID[tHot.ID].SectionCount14d)
	assert.Equal(t, 2, byID[tRich.ID].SectionCount14d)
}

// seedLaneStoryLine creates a board_stories row with explicit participant
// count and lane attribution (task 5.1 fixtures).
func seedLaneStoryLine(t *testing.T, db *gorm.DB, boardID uint, subjectKey string, participants int, laneID *uint) uint {
	t.Helper()
	today := NormalizeReportDate(time.Now())
	story := BoardStory{
		BoardID:          boardID,
		LaneID:           laneID,
		Title:            "story-" + subjectKey,
		Subject:          subjectKey,
		SubjectKey:       subjectKey,
		Status:           StoryStatusActive,
		FirstSeenDate:    today,
		LastSeenDate:     today,
		Origin:           StoryOriginRule,
		ParticipantCount: participants,
	}
	require.NoError(t, db.Omit("embedding", "subject_embedding", "occurred_at").Create(&story).Error)
	return story.ID
}

// seedLaneStoryThread appends a thread carrying story-identity fields
// (story_id / scope / related_article_ids) — the projection the day-level
// story aggregates read.
func seedLaneStoryThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, title string, storyID *uint, scope string, articleIDs []uint) uint {
	t.Helper()
	thread := DailyReportThread{
		ReportID:          reportID,
		SectionID:         sectionID,
		Title:             title,
		Embedding:         FloatsToPgVector([]float64{0}),
		StoryID:           storyID,
		Scope:             scope,
		RelatedArticleIDs: mustStoryJSON(articleIDs),
	}
	require.NoError(t, db.Create(&thread).Error)
	return thread.ID
}

func seedLaneArticle(t *testing.T, db *gorm.DB, feedID uint) uint {
	t.Helper()
	article := models.Article{FeedID: feedID, Title: "lane-evidence"}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}

func uintPtr(v uint) *uint { return &v }

// TestGetBoardLaneDynamics_StoryCountDedupAndParticipants（task 5.1 / test-cases
// 步 7）：同日多 section 的事件线去重合计（非桶峰值）、merged_into 一跳解析后
// 同一 survivor 只计一次、max_participant_count 取 survivor 的
// board_stories.participant_count 最大值、unassigned 只数非综合稿且
// story_id NULL、stories_backfilled=true。
func TestGetBoardLaneDynamics_StoryCountDedupAndParticipants(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)
	report1d := seedTestReport(t, db, boardID, now.AddDate(0, 0, -1))

	topic := seedLaneDynamicsTopic(t, db, boardID, "事件泳道", TopicStatusActive, 9, now)
	story1 := seedLaneStoryLine(t, db, boardID, "s1", 3, &topic.ID)
	story2 := seedLaneStoryLine(t, db, boardID, "s2", 6, &topic.ID)
	storyMerged := seedLaneStoryLine(t, db, boardID, "sm", 1, &topic.ID)
	// storyMerged 合入 story2：线程指向 merged 行 → 一跳解析到 story2。
	require.NoError(t, db.Model(&BoardStory{}).Where("id = ?", storyMerged).
		Updates(map[string]any{"merged_into": story2, "status": StoryStatusMerged}).Error)

	// 今日两个 section：s1 桶(story1 + story2)、s2 桶(story2 重复 + merged 行)。
	sec1 := seedTestSection(t, db, reportToday, "桶一")
	sec2 := seedTestSection(t, db, reportToday, "桶二")
	assignSection(t, db, sec1, topic.ID)
	assignSection(t, db, sec2, topic.ID)
	seedLaneStoryThread(t, db, reportToday, sec1, "事件甲", uintPtr(story1), ThreadScopeSingle, nil)
	seedLaneStoryThread(t, db, reportToday, sec1, "事件乙", uintPtr(story2), ThreadScopeSingle, nil)
	seedLaneStoryThread(t, db, reportToday, sec2, "事件乙叉桶", uintPtr(story2), ThreadScopeSingle, nil)
	seedLaneStoryThread(t, db, reportToday, sec2, "事件乙合并侧", uintPtr(storyMerged), ThreadScopeSingle, nil)
	// 未归组：非综合稿 1 条计入 unassigned；综合稿不计。
	seedLaneStoryThread(t, db, reportToday, sec2, "未归组事件", nil, ThreadScopeSingle, nil)
	seedLaneStoryThread(t, db, reportToday, sec2, "综合稿", nil, ThreadScopeComposite, nil)
	// 昨日单事件：验证逐日独立计数与 story_count_14d 求和。
	sec1d := seedTestSection(t, db, report1d, "昨日桶")
	assignSection(t, db, sec1d, topic.ID)
	seedLaneStoryThread(t, db, report1d, sec1d, "昨日事件", uintPtr(story1), ThreadScopeSingle, nil)

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	lane := resp.Lanes[0]

	assert.True(t, resp.StoriesBackfilled, "窗口内存在带 story_id 的线程")
	assert.Equal(t, HeatRuleVersion, resp.HeatRuleVersion)
	assert.False(t, resp.HeatIncomplete)
	assert.Empty(t, resp.HeatStaleFeedIDs)

	// Timeline 倒序：今日在前。
	require.Len(t, lane.Timeline, 2)
	today := lane.Timeline[0]
	assert.Equal(t, dateStr(now), today.Date)
	assert.Equal(t, 2, today.StoryCount, "去重后 {story1, story2} = 2，非桶峰值 1，也非桶内合计 4")
	assert.Equal(t, 6, today.MaxParticipantCount, "最热事件线 story2 的 participant_count")
	assert.Equal(t, 1, today.UnassignedEventCount, "仅非综合稿且 story_id NULL 的线程计入")

	yesterday := lane.Timeline[1]
	assert.Equal(t, dateStr(now.AddDate(0, 0, -1)), yesterday.Date)
	assert.Equal(t, 1, yesterday.StoryCount)
	assert.Equal(t, 3, yesterday.MaxParticipantCount)
	assert.Zero(t, yesterday.UnassignedEventCount)

	assert.Equal(t, 3, lane.StoryCount14d, "窗口内各日 story_count 求和：2+1")
}

// TestGetBoardLaneDynamics_FallbackWhenNotBackfilled（design §8.3 降级 2）：
// 窗口内无任何 story_id → story_count 用当日非综合稿线程数作上界、
// max_participant_count 按当日非综合稿线程证据文章的 distinct feed 计算、
// unassigned 与 story_count 同值、stories_backfilled=false（不得显示 0 冒充
// 无事件）。
func TestGetBoardLaneDynamics_FallbackWhenNotBackfilled(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)

	topic := seedLaneDynamicsTopic(t, db, boardID, "未回填泳道", TopicStatusActive, 9, now)
	a1 := seedLaneArticle(t, db, 11)
	a2 := seedLaneArticle(t, db, 12)
	a3 := seedLaneArticle(t, db, 11)
	a4 := seedLaneArticle(t, db, 13)
	a5 := seedLaneArticle(t, db, 99) // 综合稿的证据文章：不参与降级参与者

	sec1 := seedTestSection(t, db, reportToday, "桶一")
	sec2 := seedTestSection(t, db, reportToday, "桶二")
	assignSection(t, db, sec1, topic.ID)
	assignSection(t, db, sec2, topic.ID)
	seedLaneStoryThread(t, db, reportToday, sec1, "未归组一", nil, ThreadScopeSingle, []uint{a1, a2})
	seedLaneStoryThread(t, db, reportToday, sec1, "未归组二", nil, ThreadScopeSingle, []uint{a3, a4})
	seedLaneStoryThread(t, db, reportToday, sec2, "综合稿", nil, ThreadScopeComposite, []uint{a5})

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	lane := resp.Lanes[0]

	assert.False(t, resp.StoriesBackfilled)
	require.Len(t, lane.Timeline, 1)
	day := lane.Timeline[0]
	assert.Equal(t, 2, day.StoryCount, "降级上界：非综合稿线程数（同日两桶合并不取峰值）")
	assert.Equal(t, 2, day.MaxParticipantCount, "降级参与者：每条事件自身 distinct feed 后取最大（{11,12}=2 / {11,13}=2），不是当天并集 3")
	assert.Equal(t, 2, day.UnassignedEventCount)
	assert.Equal(t, 2, lane.StoryCount14d)
	assert.Zero(t, lane.LaneHeat, "无 signals 时 lane_heat 为 0")
	assert.Equal(t, HeatRuleVersion, resp.HeatRuleVersion)
}

// TestGetBoardLaneDynamics_HeatMetaFields（task 5.1）：heat_rule_version /
// heat_incomplete / heat_stale_feed_ids 透出——落后源（从未刷新的 feed）
// 把窗口热度标为不完整。
func TestGetBoardLaneDynamics_HeatMetaFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)

	topic := seedLaneDynamicsTopic(t, db, boardID, "覆盖泳道", TopicStatusActive, 9, now)
	story := seedLaneStoryLine(t, db, boardID, "cov", 1, &topic.ID)
	sec := seedTestSection(t, db, reportToday, "cov-sec")
	assignSection(t, db, sec, topic.ID)
	seedLaneStoryThread(t, db, reportToday, sec, "覆盖事件", uintPtr(story), ThreadScopeSingle, nil)
	staleFeed := seedCoverageFeed(t, db, "lane-stale", 60, "idle", nil)
	seedHeatSignal(t, db, story, staleFeed, now)

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	assert.Equal(t, HeatRuleVersion, resp.HeatRuleVersion)
	assert.True(t, resp.HeatIncomplete, "从未刷新的 feed 在贡献热度证据 → incomplete")
	assert.Equal(t, []uint{staleFeed}, resp.HeatStaleFeedIDs)
	require.Len(t, resp.Lanes, 1)
	assert.Greater(t, resp.Lanes[0].LaneHeat, 0.0, "signal 仍在窗口内参与 lane_heat")
}

// seedLaneArticleWithLink 落一篇带链接的证据文章（降级模式的参与者去重用）。
func seedLaneArticleWithLink(t *testing.T, db *gorm.DB, feedID uint, link string) uint {
	t.Helper()
	article := models.Article{FeedID: feedID, Title: "lane-evidence", Link: link}
	require.NoError(t, db.Create(&article).Error)
	return article.ID
}

// TestGetBoardLaneDynamics_FallbackParticipantUsesContentIdentity（fix-duplicate-article-heat
// 2.4）：未回填降级模式的参与者按内容身份去重（与 story_signals 同一口径）——
// 同一篇内容的两个 feed 副本算 1 个参与者；两篇独立内容算 2 个（取事件最大值）。
func TestGetBoardLaneDynamics_FallbackParticipantUsesContentIdentity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)

	boardID := seedTestBoard(t, db)
	now := time.Now()
	reportToday := seedTestReport(t, db, boardID, now)

	topic := seedLaneDynamicsTopic(t, db, boardID, "未回填泳道", TopicStatusActive, 9, now)
	// 同一篇内容的两个 feed 副本（只在 query / fragment 上不同）→ 1 个参与者。
	dup1 := seedLaneArticleWithLink(t, db, 11, "https://example.com/p/1?utm=x")
	dup2 := seedLaneArticleWithLink(t, db, 12, "https://example.com/p/1#comments")
	// 两篇独立内容 → 2 个参与者。
	other1 := seedLaneArticleWithLink(t, db, 13, "https://example.com/p/2")
	other2 := seedLaneArticleWithLink(t, db, 14, "https://example.com/p/3")

	sec1 := seedTestSection(t, db, reportToday, "桶一")
	assignSection(t, db, sec1, topic.ID)
	seedLaneStoryThread(t, db, reportToday, sec1, "副本事件", nil, ThreadScopeSingle, []uint{dup1, dup2})
	seedLaneStoryThread(t, db, reportToday, sec1, "独立事件", nil, ThreadScopeSingle, []uint{other1, other2})

	resp, err := repo.GetBoardLaneDynamics(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Lanes, 1)
	lane := resp.Lanes[0]
	assert.False(t, resp.StoriesBackfilled)
	require.Len(t, lane.Timeline, 1)
	day := lane.Timeline[0]
	assert.Equal(t, 2, day.StoryCount, "降级上界：非综合稿线程数")
	assert.Equal(t, 2, day.MaxParticipantCount, "副本事件 1 个参与者 / 独立事件 2 个 → 取最大 2")
}
