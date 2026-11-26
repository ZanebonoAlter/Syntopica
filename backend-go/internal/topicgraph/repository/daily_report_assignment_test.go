package repository

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/testutil"
)

// helper: build a 3-dim unit-ish vector pgvector string.
func vecStr(v ...float64) string { return FloatsToPgVector(v) }

func TestSelectAnchorableTopics_PreservesActiveAndFiltersStaleCandidate(t *testing.T) {
	reportDate := time.Date(2026, 6, 26, 15, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	cfg := PersistentTopicConfig{CandidateDecayWindow: 7, CandidatePromptLimit: 20}
	topics := []BoardPersistentTopic{
		{ID: 1, Status: TopicStatusActive, LastSeenDate: reportDate.AddDate(0, 0, -90)},
		{ID: 2, Status: TopicStatusCandidate, LastSeenDate: reportDate.AddDate(0, 0, -7)},
		{ID: 3, Status: TopicStatusCandidate, LastSeenDate: reportDate.AddDate(0, 0, -8)},
	}

	selected, stats := selectAnchorableTopics(topics, reportDate, cfg)

	require.Equal(t, []uint{1, 2}, []uint{selected[0].ID, selected[1].ID})
	assert.Equal(t, 1, stats.ActiveCount)
	assert.Equal(t, 2, stats.CandidateCount)
	assert.Equal(t, 1, stats.FilteredByWindow)
}

func TestSelectAnchorableTopics_OrdersAndLimitsCandidates(t *testing.T) {
	reportDate := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	cfg := PersistentTopicConfig{CandidateDecayWindow: 7, CandidatePromptLimit: 3}
	topics := []BoardPersistentTopic{
		{ID: 9, Status: TopicStatusCandidate, LastSeenDate: reportDate.AddDate(0, 0, -2), HitCount: 10},
		{ID: 4, Status: TopicStatusCandidate, LastSeenDate: reportDate.AddDate(0, 0, -1), HitCount: 2},
		{ID: 2, Status: TopicStatusCandidate, LastSeenDate: reportDate.AddDate(0, 0, -1), HitCount: 5},
		{ID: 1, Status: TopicStatusCandidate, LastSeenDate: reportDate.AddDate(0, 0, -1), HitCount: 5},
	}

	selected, stats := selectAnchorableTopics(topics, reportDate, cfg)

	require.Len(t, selected, 3)
	assert.Equal(t, []uint{1, 2, 4}, []uint{selected[0].ID, selected[1].ID, selected[2].ID})
	assert.Equal(t, 1, stats.TruncatedByLimit)
}

// TestSelectAnchorableTopics_AgreementWithAssignment verifies the collection
// contract under the lane-driven model: only topics that survive
// selectAnchorableTopics can be the MatchedTopicID target that
// planTopicAssignments accepts as anchor_hit. A candidate filtered by window
// or truncated by limit is absent from the anchorable set, so a section whose
// MatchedTopicID points at it cannot anchor — planTopicAssignments opens a new
// candidate instead. This guarantees bucketing injection and assignment share
// the same topic set.
func TestSelectAnchorableTopics_AgreementWithAssignment(t *testing.T) {
	reportDate := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	cfg := PersistentTopicConfig{MatchThreshold: 0.30, CandidateDecayWindow: 7, CandidatePromptLimit: 2}

	topics := []BoardPersistentTopic{
		{ID: 1, Status: TopicStatusActive, Centroid: vecStr(1, 0, 0), LastSeenDate: reportDate.AddDate(0, 0, -90)},
		{ID: 2, Status: TopicStatusCandidate, Centroid: vecStr(0.9, 0.43, 0), LastSeenDate: reportDate.AddDate(0, 0, -5)},
		{ID: 3, Status: TopicStatusCandidate, Centroid: vecStr(0.8, 0.5, 0), LastSeenDate: reportDate.AddDate(0, 0, -1)},
	}

	selected, stats := selectAnchorableTopics(topics, reportDate, cfg)
	require.Equal(t, 3, len(selected)) // active + 2 in-window candidates
	require.Equal(t, 0, stats.FilteredByWindow)
	require.Equal(t, 0, stats.TruncatedByLimit)

	anchorableIDs := make(map[uint]bool, len(selected))
	for _, t := range selected {
		anchorableIDs[t.ID] = true
	}

	// L2 sections anchor to their MatchedTopicID when that topic is anchorable.
	id2 := uint(2)
	id3 := uint(3)
	sections := []DailyReportSection{
		{ClusterLabel: "hits in-window-2", Embedding: vecStr(0.89, 0.42, 0), MatchedTopicID: &id2, LaneTier: "l2_llm"},
		{ClusterLabel: "hits in-window-3", Embedding: vecStr(0.79, 0.49, 0), MatchedTopicID: &id3, LaneTier: "l2_llm"},
	}

	decisions := planTopicAssignments(sections, selected, cfg, reportDate)
	require.Len(t, decisions, 2)

	for _, d := range decisions {
		assert.True(t, anchorableIDs[d.topicID], "topic %d not in anchorable set", d.topicID)
		assert.Equal(t, TopicConfAnchorHit, d.confidence, "L2 section should anchor to anchorable topic")
		assert.Equal(t, "l2_llm", d.laneTier)
		assert.NotNil(t, d.topicStatusAtReport)
	}

	// Negative: a section whose MatchedTopicID points at a filtered candidate
	// (id=4, gap > window) cannot anchor because id=4 is not in the anchorable
	// set. planTopicAssignments opens a new candidate (lane degrades to l3_new).
	staleTopics := []BoardPersistentTopic{
		{ID: 1, Status: TopicStatusActive, Centroid: vecStr(1, 0, 0), LastSeenDate: reportDate.AddDate(0, 0, -5)},
	}
	filteredSelected, filteredStats := selectAnchorableTopics(append(staleTopics, BoardPersistentTopic{
		ID: 4, Status: TopicStatusCandidate, Centroid: vecStr(0.9, 0.4, 0), LastSeenDate: reportDate.AddDate(0, 0, -9),
	}), reportDate, cfg)
	assert.Equal(t, 1, filteredStats.FilteredByWindow, "candidate with gap=9 should be filtered")
	require.Len(t, filteredSelected, 1)

	id4 := uint(4)
	sectionForStale := []DailyReportSection{{
		ClusterLabel: "misses stale", Embedding: vecStr(0.89, 0.39, 0), MatchedTopicID: &id4, LaneTier: "l2_llm",
	}}
	decisionsForStale := planTopicAssignments(sectionForStale, filteredSelected, cfg, reportDate)
	require.Len(t, decisionsForStale, 1)
	assert.Equal(t, TopicConfAutoNew, decisionsForStale[0].confidence,
		"pointing at a filtered candidate must trigger auto_new, not anchor hit")
	assert.Equal(t, "l3_new", decisionsForStale[0].laneTier,
		"lane degrades to l3_new when the anchor target is absent")
}

func TestCosineDistance_OrthogonalIsOne(t *testing.T) {
	// [1,0,0] vs [0,1,0] → similarity 0 → distance 1.
	d := cosineDistance([]float64{1, 0, 0}, []float64{0, 1, 0})
	assert.InDelta(t, 1.0, d, 1e-9)
}

func TestCosineDistance_IdenticalIsZero(t *testing.T) {
	d := cosineDistance([]float64{0.3, 0.7, -0.2}, []float64{0.3, 0.7, -0.2})
	assert.InDelta(t, 0.0, d, 1e-9)
}

func TestCosineDistance_MismatchedLengthIsInf(t *testing.T) {
	assert.True(t, math.IsInf(cosineDistance([]float64{1, 0}, []float64{1, 0, 0}), 1) || math.IsInf(cosineDistance([]float64{1, 0}, []float64{1, 0, 0}), 0) || cosineDistance([]float64{1, 0}, []float64{1, 0, 0}) == math.MaxFloat64)
}

// ── Lane-driven planTopicAssignments tests (daily-report-lane-driven-clustering) ──
//
// Attribution is decided upstream by bucketing (section.LaneTier +
// section.MatchedTopicID); planTopicAssignments only maps the lane outcome onto
// confidence + distance. The old dual-confirmation AND-gate is gone.

func TestPlanTopicAssignments_LaneDriven_AnchorHit_L1(t *testing.T) {
	// L1 section anchors to its MatchedTopicID at the centroid distance.
	mit := uint(12)
	cfg := DefaultPersistentTopicConfig()
	topics := []BoardPersistentTopic{{ID: 12, Centroid: vecStr(1, 0, 0), Status: TopicStatusActive}}
	sections := []DailyReportSection{{
		ClusterLabel: "AI 编程竞争", Embedding: vecStr(0.99, 0.01, 0),
		MatchedTopicID: &mit, LaneTier: "l1_direct",
	}}
	dec := planTopicAssignments(sections, topics, cfg, time.Now())
	require.Len(t, dec, 1)
	assert.Equal(t, TopicConfAnchorHit, dec[0].confidence)
	assert.Equal(t, uint(12), dec[0].topicID)
	assert.Equal(t, "l1_direct", dec[0].laneTier)
	assert.Nil(t, dec[0].newCandidate)
	require.NotNil(t, dec[0].topicStatusAtReport)
	assert.Equal(t, TopicStatusActive, *dec[0].topicStatusAtReport)
	assert.InDelta(t, 0.0, dec[0].distance, 0.01, "distance = section embedding to centroid anchor")
}

func TestPlanTopicAssignments_LaneDriven_AnchorHit_L2(t *testing.T) {
	// L2 section anchors to its MatchedTopicID (not the nearest topic) at the
	// real centroid distance — the lane-driven equivalent of the old drift case.
	mit := uint(13)
	cfg := DefaultPersistentTopicConfig()
	topics := []BoardPersistentTopic{
		{ID: 12, Centroid: vecStr(1, 0, 0), Status: TopicStatusActive},      // nearest (~0.0001)
		{ID: 13, Centroid: vecStr(0.9, 0.43, 0), Status: TopicStatusActive}, // chosen (~0.093)
	}
	sections := []DailyReportSection{{
		ClusterLabel:   "AI 编程竞争",
		Embedding:      vecStr(1, 0.01, 0),
		MatchedTopicID: &mit,
		LaneTier:       "l2_llm",
	}}
	dec := planTopicAssignments(sections, topics, cfg, time.Now())
	require.Len(t, dec, 1)
	assert.Equal(t, TopicConfAnchorHit, dec[0].confidence, "must anchor, not open new candidate")
	assert.Equal(t, uint(13), dec[0].topicID, "anchor to the lane-chosen topic, not the nearest")
	assert.Equal(t, "l2_llm", dec[0].laneTier)
	assert.InDelta(t, 0.093, dec[0].distance, 0.01, "distance is the anchored topic's centroid distance")
}

func TestPlanTopicAssignments_LaneDriven_AnchorUsesEmbeddingFallback(t *testing.T) {
	// Topic has no centroid → anchor degrades to the 首义 embedding.
	mit := uint(12)
	cfg := DefaultPersistentTopicConfig()
	topics := []BoardPersistentTopic{{ID: 12, Embedding: vecStr(1, 0, 0), Status: TopicStatusActive}}
	sections := []DailyReportSection{{
		ClusterLabel: "AI", Embedding: vecStr(0.99, 0.01, 0),
		MatchedTopicID: &mit, LaneTier: "l1_direct",
	}}
	dec := planTopicAssignments(sections, topics, cfg, time.Now())
	require.Len(t, dec, 1)
	assert.Equal(t, TopicConfAnchorHit, dec[0].confidence)
	assert.Equal(t, uint(12), dec[0].topicID)
}

func TestPlanTopicAssignments_LaneDriven_AnchorTopicAbsentBecomesNew(t *testing.T) {
	// Lane says anchor but MatchedTopicID points at a topic not in the
	// anchorable set (e.g. filtered) → degrade to auto_new, lane l3_new, so the
	// persisted row keeps confidence + lane consistent.
	cfg := DefaultPersistentTopicConfig()
	topics := []BoardPersistentTopic{{ID: 12, Centroid: vecStr(1, 0, 0), Status: TopicStatusActive}}
	missing := uint(99)
	sections := []DailyReportSection{{
		ClusterLabel: "ghost", Embedding: vecStr(0.99, 0.01, 0),
		MatchedTopicID: &missing, LaneTier: "l2_llm",
	}}
	dec := planTopicAssignments(sections, topics, cfg, time.Now())
	require.Len(t, dec, 1)
	assert.Equal(t, TopicConfAutoNew, dec[0].confidence)
	assert.Equal(t, "l3_new", dec[0].laneTier)
	assert.NotNil(t, dec[0].newCandidate)
}

func TestPlanTopicAssignments_LaneDriven_L3New(t *testing.T) {
	// L3 section → auto_new, lane l3_new, distance = nearest anchor (diag).
	cfg := DefaultPersistentTopicConfig()
	topics := []BoardPersistentTopic{{ID: 12, Centroid: vecStr(1, 0, 0), Status: TopicStatusActive}}
	sections := []DailyReportSection{{
		ClusterLabel: "量子计算商用", Embedding: vecStr(0, 1, 0), LaneTier: "l3_new",
	}}
	dec := planTopicAssignments(sections, topics, cfg, time.Now())
	require.Len(t, dec, 1)
	assert.Equal(t, TopicConfAutoNew, dec[0].confidence)
	assert.Equal(t, "l3_new", dec[0].laneTier)
	assert.NotNil(t, dec[0].newCandidate)
	assert.Equal(t, "量子计算商用", dec[0].newCandidate.label)
	assert.InDelta(t, 1.0, dec[0].distance, 1e-9, "nearest anchor distance (orthogonal)")
}

func TestPlanTopicAssignments_LaneDriven_Unmatched_EmptyEmbedding(t *testing.T) {
	cfg := DefaultPersistentTopicConfig()
	sections := []DailyReportSection{{ClusterLabel: "no embedding", Embedding: "", LaneTier: "l1_direct"}}
	dec := planTopicAssignments(sections, nil, cfg, time.Now())
	require.Len(t, dec, 1)
	assert.Equal(t, TopicConfUnmatched, dec[0].confidence)
	assert.Equal(t, "l1_direct", dec[0].laneTier)
	assert.Nil(t, dec[0].newCandidate)
	assert.Nil(t, dec[0].topicStatusAtReport)
	assert.Equal(t, 0.0, dec[0].distance)
}

func TestPlanLifecycle_EligibleCandidateStillRequiresManualConfirmation(t *testing.T) {
	// Reaching the occurrence threshold makes a candidate eligible for manual
	// confirmation, but the daily pipeline must not publish it automatically.
	topics := []BoardPersistentTopic{{
		ID: 1, Status: TopicStatusCandidate, ConsecutiveHits: 2, HitCount: 2,
		LastSeenDate: time.Now().AddDate(0, 0, -1),
	}}
	changes := planLifecycle(topics, time.Now(), map[uint]bool{1: true})
	require.Len(t, changes, 1)
	assert.Equal(t, TopicStatusCandidate, changes[0].status)
	assert.Equal(t, 3, changes[0].consecutiveHits)
	assert.Equal(t, 3, changes[0].hitCount)
}

func TestPlanLifecycle_HitKeepsCandidateBelowThreshold(t *testing.T) {
	// candidate at consecutive=1, hit today → stays candidate (2 < 3).
	topics := []BoardPersistentTopic{{
		ID: 1, Status: TopicStatusCandidate, ConsecutiveHits: 1, HitCount: 1,
		LastSeenDate: time.Now().AddDate(0, 0, -1),
	}}
	changes := planLifecycle(topics, time.Now(), map[uint]bool{1: true})
	require.Len(t, changes, 1)
	assert.Equal(t, TopicStatusCandidate, changes[0].status)
	assert.Equal(t, 2, changes[0].consecutiveHits)
}

func TestPlanLifecycle_MissResetsConsecutive(t *testing.T) {
	// candidate not hit today → consecutive resets to 0; stays candidate.
	topics := []BoardPersistentTopic{{
		ID: 1, Status: TopicStatusCandidate, ConsecutiveHits: 2, HitCount: 2,
		LastSeenDate: time.Now().AddDate(0, 0, -1),
	}}
	changes := planLifecycle(topics, time.Now(), map[uint]bool{})
	require.Len(t, changes, 1)
	assert.Equal(t, TopicStatusCandidate, changes[0].status)
	assert.Equal(t, 0, changes[0].consecutiveHits)
	assert.Equal(t, 2, changes[0].hitCount)
}

func TestPlanLifecycle_LongGapNeverArchives(t *testing.T) {
	// candidate 60 days behind last_seen, active 35 days behind last_seen —
	// neither must be archived because archiving is manual-only. Both miss
	// today, so both reset consecutive_hits to 0.
	today := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC)
	topics := []BoardPersistentTopic{
		{ID: 1, Status: TopicStatusCandidate, ConsecutiveHits: 2, HitCount: 4, LastSeenDate: today.AddDate(0, 0, -60)},
		{ID: 2, Status: TopicStatusActive, ConsecutiveHits: 5, HitCount: 10, LastSeenDate: today.AddDate(0, 0, -35)},
	}
	changes := planLifecycle(topics, today, map[uint]bool{})

	require.Len(t, changes, 2)
	assert.Equal(t, uint(1), changes[0].topicID)
	assert.Equal(t, TopicStatusCandidate, changes[0].status)
	assert.Equal(t, 0, changes[0].consecutiveHits)
	assert.Equal(t, uint(2), changes[1].topicID)
	assert.Equal(t, TopicStatusActive, changes[1].status)
	assert.Equal(t, 0, changes[1].consecutiveHits)
}

// TestPlanLifecycle_SameDayMultipleSectionsCountOnce covers the persistent-topic
// Scenario「同日多条 section 只记一次命中」(add-intra-lane-theme-bucketing):
// three sibling buckets of one cluster count as ONE hit for the day.
func TestPlanLifecycle_SameDayMultipleSectionsCountOnce(t *testing.T) {
	db := testutil.SetupTestDB(t)
	Repo = NewTopicGraphRepository(db)
	t.Cleanup(func() { Repo = nil })

	boardID := seedTestBoard(t, db)
	today := NormalizeReportDate(time.Now())
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "AGENT编程工具相关",
		Embedding:       FloatsToPgVector([]float64{1, 0, 0}),
		Status:          TopicStatusActive,
		Source:          TopicSourceManual,
		FirstSeenDate:   today.AddDate(0, 0, -3),
		LastSeenDate:    today.AddDate(0, 0, -1),
		HitCount:        4,
		ConsecutiveHits: 2,
	}
	require.NoError(t, db.Create(&topic).Error)
	topicID := topic.ID

	report := &BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      today,
		Title:           "three siblings, one hit",
		Status:          ReportStatusCompleted,
	}
	sections := []DailyReportSection{
		{ClusterIndex: 0, BucketIndex: 0, BucketTitle: "A", ClusterLabel: "A", ClusterTagIDs: JSON("[1]"),
			Embedding: FloatsToPgVector([]float64{1, 0, 0}), LaneTier: "l1_direct", MatchedTopicID: &topicID},
		{ClusterIndex: 0, BucketIndex: 1, BucketTitle: "B", ClusterLabel: "B", ClusterTagIDs: JSON("[2]"),
			Embedding: FloatsToPgVector([]float64{0, 1, 0}), LaneTier: "l1_direct", MatchedTopicID: &topicID},
		{ClusterIndex: 0, BucketIndex: 2, BucketTitle: "C", ClusterLabel: "C", ClusterTagIDs: JSON("[3]"),
			Embedding: FloatsToPgVector([]float64{0, 0, 1}), LaneTier: "l1_direct", MatchedTopicID: &topicID},
	}
	batches := [][]DailyReportThread{{{Title: "a"}}, {{Title: "b"}}, {{Title: "c"}}}
	require.NoError(t, Repo.SaveReport(report, sections, batches))

	var reloaded BoardPersistentTopic
	require.NoError(t, db.First(&reloaded, topicID).Error)
	assert.Equal(t, 5, reloaded.HitCount, "3 sibling sections must add exactly 1 hit")
	assert.Equal(t, 3, reloaded.ConsecutiveHits, "consecutive_hits also advances only once")
	assert.Equal(t, NormalizeReportDate(today), NormalizeReportDate(reloaded.LastSeenDate))
}

// ── 人工归属变更（移除 / 加回）（lane-manual-correction D4/D5） ─────────────
// PostgreSQL 集成测试（testcontainer 黄金 schema）。repository 层禁 SQLite。

// seedAssignmentReport 建某一天的日报（period_date 是 DATE 列，按 YYYY-MM-DD
// 精确落位，便于按日批量移除断言「无越日」）。
func seedAssignmentReport(t *testing.T, db *gorm.DB, boardID uint, day string) uint {
	t.Helper()
	report := BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC).AddDate(0, 0, dayOffset(day)),
		Title:           "report " + day,
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

// dayOffset 把 "YYYY-MM-DD" 解析成相对 2026-11-10 的天数偏移。
func dayOffset(day string) int {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return 0
	}
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	return int(t.Sub(base).Hours() / 24)
}

// seedAssignmentSection 建一条带完整归属快照的节点（算法 anchor_hit 后的状态）。
// topicID 为 nil 时建的是「无归属」节点：归属快照列全部为空。
func seedAssignmentSection(t *testing.T, db *gorm.DB, reportID uint, label string, embedding []float64, topicID *uint) DailyReportSection {
	t.Helper()
	sec := DailyReportSection{
		ReportID:      reportID,
		ClusterLabel:  label,
		ClusterTagIDs: JSON("[]"),
		ArticleCount:  1,
		Embedding:     FloatsToPgVector(embedding),
		LaneTier:      "l1_direct",
	}
	if topicID != nil {
		status := TopicStatusActive
		sec.PersistentTopicID = topicID
		sec.TopicMatchDistance = 0.12
		sec.TopicMatchConfidence = TopicConfAnchorHit
		sec.TopicStatusAtReport = &status
	}
	require.NoError(t, db.Create(&sec).Error)
	return sec
}

// seedAssignmentTopic 建一条泳道话题，并写入一条质心（用于断言「归属变更不动
// 质心列」与「加回距离按质心锚点实算」）。
func seedAssignmentTopic(t *testing.T, db *gorm.DB, boardID uint, label string, status string, embedding []float64, centroid []float64) BoardPersistentTopic {
	t.Helper()
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       FloatsToPgVector(embedding),
		Status:          status,
		Source:          TopicSourceAuto,
		FirstSeenDate:   NormalizeReportDate(time.Now()),
		LastSeenDate:    NormalizeReportDate(time.Now()),
		HitCount:        5,
		ConsecutiveHits: 2,
	}
	require.NoError(t, db.Create(&topic).Error)
	if centroid != nil {
		require.NoError(t, db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
			Update("centroid", FloatsToPgVector(centroid)).Error)
	}
	return topic
}

// TestSetSectionTopicAssignment_DetachClearsSnapshotAndKeepsLaneTier covers
// 故事 B2 / 白盒 B1（移除方向逐列断言）: the four snapshot columns become
// NULL, lane_tier keeps its value, and the audit columns record a detach with
// the topic the node came from.
func TestSetSectionTopicAssignment_DetachClearsSnapshotAndKeepsLaneTier(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, []float64{0, 0, 1})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	result, err := repo.SetSectionTopicAssignment(sec.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, sec.ID, result.SectionID)
	assert.Nil(t, result.TopicID)
	require.NotNil(t, result.PreviousTopicID)
	assert.Equal(t, topic.ID, *result.PreviousTopicID)
	assert.Equal(t, ManualAssignmentActionDetach, result.Action)
	assert.True(t, result.Changed)

	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, sec.ID).Error)
	assert.Nil(t, reloaded.PersistentTopicID, "persistent_topic_id → NULL")
	assert.Zero(t, reloaded.TopicMatchDistance, "topic_match_distance → NULL")
	assert.Empty(t, reloaded.TopicMatchConfidence, "topic_match_confidence → NULL")
	assert.Nil(t, reloaded.TopicStatusAtReport, "topic_status_at_report → NULL")
	assert.Equal(t, "l1_direct", reloaded.LaneTier, "lane_tier 保留")
	require.NotNil(t, reloaded.ManualAssignmentAt)
	require.NotNil(t, reloaded.ManualAssignmentFromTopicID)
	assert.Equal(t, topic.ID, *reloaded.ManualAssignmentFromTopicID)
	assert.Equal(t, ManualAssignmentActionDetach, reloaded.ManualAssignmentAction)

	// 四列必须是真 NULL（不是零值）——否则读侧会看到「无归属却有距离/置信/
	// 状态快照」的自相矛盾行。
	var snapshot struct {
		TopicID    *uint
		Distance   *float64
		Confidence *string
		Status     *string
	}
	require.NoError(t, repo.db.Raw(`
		SELECT persistent_topic_id AS topic_id,
		       topic_match_distance AS distance,
		       topic_match_confidence AS confidence,
		       topic_status_at_report AS status
		FROM daily_report_sections WHERE id = ?
	`, sec.ID).Scan(&snapshot).Error)
	assert.Nil(t, snapshot.TopicID)
	assert.Nil(t, snapshot.Distance)
	assert.Nil(t, snapshot.Confidence)
	assert.Nil(t, snapshot.Status)
}

// TestSetSectionTopicAssignment_AttachWritesManualSnapshot covers 故事 B5 /
// 白盒 B2（加回方向逐列断言）: the target lane id is written, the confidence
// becomes "manual", the distance is recomputed against the target's anchor
// (centroid first), topic_status_at_report is NULL and lane_tier survives.
func TestSetSectionTopicAssignment_AttachWritesManualSnapshot(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	// 锚点 = 质心 [0,0,1]（embedding 是 [1,0,0]）；节点向量 [1,0,0] → 余弦 0 →
	// 距离 1.0。距离按质心算（不是按 embedding），否则会是 0。
	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, []float64{0, 0, 1})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, nil)

	result, err := repo.SetSectionTopicAssignment(sec.ID, &topic.ID)
	require.NoError(t, err)
	assert.Equal(t, ManualAssignmentActionAttach, result.Action)
	assert.True(t, result.Changed)
	require.NotNil(t, result.TopicID)
	assert.Equal(t, topic.ID, *result.TopicID)
	require.NotNil(t, result.Distance)
	assert.InDelta(t, 1.0, *result.Distance, 1e-6)

	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
	assert.Equal(t, TopicConfManual, reloaded.TopicMatchConfidence)
	assert.InDelta(t, 1.0, reloaded.TopicMatchDistance, 1e-6)
	assert.Nil(t, reloaded.TopicStatusAtReport)
	assert.Equal(t, "l1_direct", reloaded.LaneTier, "lane_tier 保留")
	require.NotNil(t, reloaded.ManualAssignmentAt)
	assert.Nil(t, reloaded.ManualAssignmentFromTopicID, "加回的变更前话题 id 为 NULL")
	assert.Equal(t, ManualAssignmentActionAttach, reloaded.ManualAssignmentAction)

	// 加回后的距离是真值（非 NULL）：实算到目标锚点的距离。
	var snapshot struct {
		Distance   *float64
		Confidence *string
		Status     *string
	}
	require.NoError(t, repo.db.Raw(`
		SELECT topic_match_distance AS distance,
		       topic_match_confidence AS confidence,
		       topic_status_at_report AS status
		FROM daily_report_sections WHERE id = ?
	`, sec.ID).Scan(&snapshot).Error)
	require.NotNil(t, snapshot.Distance)
	assert.InDelta(t, 1.0, *snapshot.Distance, 1e-6)
	require.NotNil(t, snapshot.Confidence)
	assert.Equal(t, TopicConfManual, *snapshot.Confidence)
	assert.Nil(t, snapshot.Status, "topic_status_at_report → NULL")
}

// TestSetSectionTopicAssignment_FlagsAffectedTopicForRecompute covers task 3.3
// + spec「加回后质心窗口重新纳入」: the affected lane (the one the node left /
// the one it joined) is flagged centroid_pending_recompute, and the centroid
// column itself is NOT written (task 3.7 / spec「归属变更不立即重算质心」).
func TestSetSectionTopicAssignment_FlagsAffectedTopicForRecompute(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	oldTopic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, []float64{0, 0, 1})
	newTopic := seedAssignmentTopic(t, repo.db, boardID, "lane B", TopicStatusActive, []float64{0, 1, 0}, []float64{0, 1, 0})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &oldTopic.ID)

	// 移除 → 被移除时所属话题被置位。
	_, err := repo.SetSectionTopicAssignment(sec.ID, nil)
	require.NoError(t, err)

	var reloadedOld BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloadedOld, oldTopic.ID).Error)
	assert.True(t, reloadedOld.CentroidPendingRecompute, "移除 → 原话题置待重算")
	assertVecEqual(t, FloatsToPgVector([]float64{0, 0, 1}), reloadedOld.Centroid)

	var reloadedNew BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloadedNew, newTopic.ID).Error)
	assert.False(t, reloadedNew.CentroidPendingRecompute, "未受影响的话题不置位")

	// 加回 → 目标话题被置位。
	_, err = repo.SetSectionTopicAssignment(sec.ID, &newTopic.ID)
	require.NoError(t, err)

	require.NoError(t, repo.db.First(&reloadedNew, newTopic.ID).Error)
	assert.True(t, reloadedNew.CentroidPendingRecompute, "加回 → 目标话题置待重算")
	assertVecEqual(t, FloatsToPgVector([]float64{0, 1, 0}), reloadedNew.Centroid)
}

// TestSetSectionTopicAssignment_LeavesHitCountersAndStatusUntouched covers
// 故事 B4 / spec「归属变更不动命中计数与状态」: hit_count / consecutive_hits /
// status belong to the algorithm and to explicit user archiving.
func TestSetSectionTopicAssignment_LeavesHitCountersAndStatusUntouched(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, []float64{0, 0, 1})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	_, err := repo.SetSectionTopicAssignment(sec.ID, nil)
	require.NoError(t, err)

	var reloaded BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloaded, topic.ID).Error)
	assert.Equal(t, 5, reloaded.HitCount)
	assert.Equal(t, 2, reloaded.ConsecutiveHits)
	assert.Equal(t, TopicStatusActive, reloaded.Status)
}

// TestSetSectionTopicAssignment_RebuildsRelationsAndRollsBackOnFailure covers
// 故事 B3 / 白盒 E1: the transaction ends with RebuildBoardRelations, and a
// failing rebuild rolls back both the membership change and the audit trail.
func TestSetSectionTopicAssignment_RebuildsRelationsAndRollsBackOnFailure(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, []float64{0, 0, 1})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	// 关系重建被调用（整板重建，非增量）。
	var rebuilt []uint
	repo.rebuildRelations = func(tx *gorm.DB, id uint) error {
		rebuilt = append(rebuilt, id)
		return RebuildBoardRelations(tx, id)
	}
	_, err := repo.SetSectionTopicAssignment(sec.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, []uint{boardID}, rebuilt, "事务末尾必须重建关系")

	// 重建失败 → 整体回滚：归属与留痕一并还原。
	rollbackTopic := seedAssignmentTopic(t, repo.db, boardID, "rollback lane", TopicStatusActive, []float64{0, 0, 1}, nil)
	rollbackSec := seedAssignmentSection(t, repo.db, reportID, "rollback", []float64{1, 0, 0}, &rollbackTopic.ID)
	repo.rebuildRelations = func(tx *gorm.DB, id uint) error {
		return errors.New("rebuild boom")
	}
	_, err = repo.SetSectionTopicAssignment(rollbackSec.ID, nil)
	require.Error(t, err)

	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, rollbackSec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID, "回滚后归属不变")
	assert.Equal(t, rollbackTopic.ID, *reloaded.PersistentTopicID)
	assert.Equal(t, TopicConfAnchorHit, reloaded.TopicMatchConfidence)
	assert.Nil(t, reloaded.ManualAssignmentAt, "回滚后不留痕")
	assert.Empty(t, reloaded.ManualAssignmentAction)

	var rollbackTopicRow BoardPersistentTopic
	require.NoError(t, repo.db.First(&rollbackTopicRow, rollbackTopic.ID).Error)
	assert.False(t, rollbackTopicRow.CentroidPendingRecompute, "回滚后待重算标记也还原")
}

// TestSetSectionTopicAssignment_RejectsInvalidTargets covers the validation
// branches (spec「加回目标校验」/「加回不接受改挂」): cross-board target,
// archived target, re-parenting and detaching an unassigned node are all
// rejected without writing a single column.
// TestSetSectionTopicAssignment_ReassignFromCandidateLane covers the
// 改挂 permission narrowed by the lane-correction loop: a node may be moved
// out of a **candidate**（动态）lane into the target lane (its material is
// still fluid), but never out of an **active** lane. Both lanes get the
// pending-recompute mark and the assignment override records the source.
func TestSetSectionTopicAssignment_ReassignFromCandidateLane(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	target := seedAssignmentTopic(t, repo.db, boardID, "target lane", TopicStatusActive, []float64{1, 0, 0}, []float64{1, 0, 0})
	source := seedAssignmentTopic(t, repo.db, boardID, "dynamic lane", TopicStatusCandidate, []float64{0, 1, 0}, []float64{0, 1, 0})
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &source.ID)

	res, err := repo.SetSectionTopicAssignment(sec.ID, &target.ID)
	require.NoError(t, err)
	assert.True(t, res.Changed, "改挂 → 发生变化")
	require.NotNil(t, res.TopicID)
	assert.Equal(t, target.ID, *res.TopicID)
	require.NotNil(t, res.PreviousTopicID)
	assert.Equal(t, source.ID, *res.PreviousTopicID, "留痕记录来源泳道")

	// 节点归属已改到目标泳道。
	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, target.ID, *reloaded.PersistentTopicID)

	// 两条泳道的滚动窗口都变了 → 都置待重算。
	var reloadedTarget, reloadedSource BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloadedTarget, target.ID).Error)
	require.NoError(t, repo.db.First(&reloadedSource, source.ID).Error)
	assert.True(t, reloadedTarget.CentroidPendingRecompute, "目标泳道置待重算")
	assert.True(t, reloadedSource.CentroidPendingRecompute, "来源泳道置待重算")

	// 留痕行：来源 → 目标。
	history, err := repo.ListSectionAssignmentHistory(sec.ID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.NotNil(t, history[0].TopicIDFrom)
	require.NotNil(t, history[0].TopicIDTo)
	assert.Equal(t, source.ID, *history[0].TopicIDFrom)
	assert.Equal(t, target.ID, *history[0].TopicIDTo)
}

// TestSetSectionTopicAssignment_ReassignFromActiveLaneRejected pins the
// remaining half of the rule: an **active** lane keeps its nodes.
func TestSetSectionTopicAssignment_ReassignFromActiveLaneRejected(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	target := seedAssignmentTopic(t, repo.db, boardID, "target lane", TopicStatusActive, []float64{1, 0, 0}, nil)
	source := seedAssignmentTopic(t, repo.db, boardID, "active lane", TopicStatusActive, []float64{0, 1, 0}, nil)
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &source.ID)

	_, err := repo.SetSectionTopicAssignment(sec.ID, &target.ID)
	assert.ErrorIs(t, err, ErrReassignFromActiveLane)

	// 拒绝后不得改任何列。
	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, source.ID, *reloaded.PersistentTopicID, "拒绝后归属不变")
}

func TestSetSectionTopicAssignment_RejectsInvalidTargets(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	otherBoardID := seedTestBoardNamed(t, repo.db, "assignment-other-board")
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, nil)
	other := seedAssignmentTopic(t, repo.db, boardID, "lane B", TopicStatusActive, []float64{0, 1, 0}, nil)
	archived := seedAssignmentTopic(t, repo.db, boardID, "archived lane", TopicStatusArchived, []float64{1, 0, 0}, nil)
	foreign := seedAssignmentTopic(t, repo.db, otherBoardID, "foreign lane", TopicStatusActive, []float64{1, 0, 0}, nil)
	assigned := seedAssignmentSection(t, repo.db, reportID, "assigned", []float64{1, 0, 0}, &topic.ID)
	unassigned := seedAssignmentSection(t, repo.db, reportID, "unassigned", []float64{1, 0, 0}, nil)

	// 加回到别的版块 → 拒绝。
	_, err := repo.SetSectionTopicAssignment(unassigned.ID, &foreign.ID)
	assert.ErrorIs(t, err, ErrTopicOnOtherBoard)

	// 加回到已归档泳道 → 拒绝。
	_, err = repo.SetSectionTopicAssignment(unassigned.ID, &archived.ID)
	assert.ErrorIs(t, err, ErrTopicArchived)

	// 加回到不存在的泳道 → 拒绝。
	unknownTopicID := uint(999999)
	_, err = repo.SetSectionTopicAssignment(unassigned.ID, &unknownTopicID)
	assert.ErrorIs(t, err, ErrTopicNotFound)

	// 已归属 **active** 泳道的节点改挂到另一条泳道 → 拒绝（已转正泳道不挪）。
	_, err = repo.SetSectionTopicAssignment(assigned.ID, &other.ID)
	assert.ErrorIs(t, err, ErrReassignFromActiveLane)

	// 移除无归属的节点 → 拒绝。
	_, err = repo.SetSectionTopicAssignment(unassigned.ID, nil)
	assert.ErrorIs(t, err, ErrSectionAlreadyUnassigned)

	// 未知节点 → 拒绝。
	unknownSectionID := uint(999999)
	_, err = repo.SetSectionTopicAssignment(unknownSectionID, nil)
	assert.ErrorIs(t, err, ErrSectionNotFound)

	// 什么都没写。
	var unassignedRow DailyReportSection
	require.NoError(t, repo.db.First(&unassignedRow, unassigned.ID).Error)
	assert.Nil(t, unassignedRow.PersistentTopicID)
	assert.Empty(t, unassignedRow.TopicMatchConfidence)
	assert.Nil(t, unassignedRow.ManualAssignmentAt)

	var assignedRow DailyReportSection
	require.NoError(t, repo.db.First(&assignedRow, assigned.ID).Error)
	require.NotNil(t, assignedRow.PersistentTopicID)
	assert.Equal(t, topic.ID, *assignedRow.PersistentTopicID)
	assert.Equal(t, TopicConfAnchorHit, assignedRow.TopicMatchConfidence)
}

// TestSetSectionTopicAssignment_AttachToSameLaneIsIdempotent covers the
// idempotency variant「section 已属目标时加回 → 幂等 200 不重复留痕」: no
// column is rewritten and no second audit row is appended.
func TestSetSectionTopicAssignment_AttachToSameLaneIsIdempotent(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, nil)
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	result, err := repo.SetSectionTopicAssignment(sec.ID, &topic.ID)
	require.NoError(t, err)
	assert.False(t, result.Changed, "幂等加回不写任何列")
	assert.Equal(t, ManualAssignmentActionAttach, result.Action)

	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, sec.ID).Error)
	assert.Equal(t, TopicConfAnchorHit, reloaded.TopicMatchConfidence, "算法判定的置信度不被人工改写")
	assert.Nil(t, reloaded.ManualAssignmentAt, "幂等加回不追加留痕")
	assert.Empty(t, reloaded.ManualAssignmentAction)
}

// TestDetachTopicSectionsByDate_ReleasesOnlyThatDay covers task 3.2 / spec
// 「批量移除某一天」: every node of that lane on that day is released, other
// days and other lanes are untouched, and one relation rebuild happens.
func TestDetachTopicSectionsByDate_ReleasesOnlyThatDay(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	day1 := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")
	day2 := seedAssignmentReport(t, repo.db, boardID, "2026-11-11")

	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, nil)
	other := seedAssignmentTopic(t, repo.db, boardID, "lane B", TopicStatusActive, []float64{0, 1, 0}, nil)

	s1 := seedAssignmentSection(t, repo.db, day1, "d1-a", []float64{1, 0, 0}, &topic.ID)
	s2 := seedAssignmentSection(t, repo.db, day1, "d1-b", []float64{1, 0, 0}, &topic.ID)
	s3 := seedAssignmentSection(t, repo.db, day1, "d1-c", []float64{1, 0, 0}, &topic.ID)
	s4 := seedAssignmentSection(t, repo.db, day2, "d2-a", []float64{1, 0, 0}, &topic.ID)
	s5 := seedAssignmentSection(t, repo.db, day2, "d2-b", []float64{1, 0, 0}, &topic.ID)
	otherDay := seedAssignmentSection(t, repo.db, day1, "other-lane", []float64{0, 1, 0}, &other.ID)
	free := seedAssignmentSection(t, repo.db, day1, "free", []float64{0, 0, 1}, nil)

	var rebuilt []uint
	repo.rebuildRelations = func(tx *gorm.DB, id uint) error {
		rebuilt = append(rebuilt, id)
		return RebuildBoardRelations(tx, id)
	}

	result, err := repo.DetachTopicSectionsByDate(topic.ID, time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 3, result.Detached, "当天 3 条全部解除")
	assert.Equal(t, []uint{s1.ID, s2.ID, s3.ID}, result.SectionIDs)
	assert.Equal(t, "2026-11-10", result.PeriodDate)
	assert.Equal(t, []uint{boardID}, rebuilt, "整板关系只重建一次")

	for _, id := range []uint{s1.ID, s2.ID, s3.ID} {
		var reloaded DailyReportSection
		require.NoError(t, repo.db.First(&reloaded, id).Error)
		assert.Nil(t, reloaded.PersistentTopicID)
		assert.Empty(t, reloaded.TopicMatchConfidence)
		assert.Equal(t, "l1_direct", reloaded.LaneTier, "lane_tier 保留")
		require.NotNil(t, reloaded.ManualAssignmentAt)
		require.NotNil(t, reloaded.ManualAssignmentFromTopicID)
		assert.Equal(t, topic.ID, *reloaded.ManualAssignmentFromTopicID)
		assert.Equal(t, ManualAssignmentActionDetach, reloaded.ManualAssignmentAction)
	}

	// 无遗漏（当天 3 条都写了留痕）。
	var detached int64
	require.NoError(t, repo.db.Model(&DailyReportSection{}).
		Where("manual_assignment_action = ? AND manual_assignment_from_topic_id = ?", ManualAssignmentActionDetach, topic.ID).
		Count(&detached).Error)
	assert.Equal(t, int64(3), detached)

	// 无越日：另一天的节点不动。
	for _, id := range []uint{s4.ID, s5.ID} {
		var reloaded DailyReportSection
		require.NoError(t, repo.db.First(&reloaded, id).Error)
		require.NotNil(t, reloaded.PersistentTopicID)
		assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
		assert.Nil(t, reloaded.ManualAssignmentAt)
	}

	// 别的泳道 / 无归属的节点不动。
	var reloadedOther, reloadedFree DailyReportSection
	require.NoError(t, repo.db.First(&reloadedOther, otherDay.ID).Error)
	require.NoError(t, repo.db.First(&reloadedFree, free.ID).Error)
	require.NotNil(t, reloadedOther.PersistentTopicID)
	assert.Equal(t, other.ID, *reloadedOther.PersistentTopicID)
	assert.Nil(t, reloadedFree.PersistentTopicID)
	assert.Nil(t, reloadedFree.ManualAssignmentAt)

	// 受影响话题置待重算。
	var reloadedTopic BoardPersistentTopic
	require.NoError(t, repo.db.First(&reloadedTopic, topic.ID).Error)
	assert.True(t, reloadedTopic.CentroidPendingRecompute)
}

// TestDetachTopicSectionsByDate_UnknownTopicAndEmptyDay covers the
// precondition variants: an unknown lane is rejected, a day without nodes is
// a no-op (nothing written, no relation rebuild).
func TestDetachTopicSectionsByDate_UnknownTopicAndEmptyDay(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	reportID := seedAssignmentReport(t, repo.db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, repo.db, boardID, "lane A", TopicStatusActive, []float64{1, 0, 0}, nil)
	sec := seedAssignmentSection(t, repo.db, reportID, "s1", []float64{1, 0, 0}, &topic.ID)

	_, err := repo.DetachTopicSectionsByDate(uint(999999), time.Now())
	assert.ErrorIs(t, err, ErrTopicNotFound)

	var rebuilt []uint
	repo.rebuildRelations = func(tx *gorm.DB, id uint) error {
		rebuilt = append(rebuilt, id)
		return RebuildBoardRelations(tx, id)
	}
	result, err := repo.DetachTopicSectionsByDate(topic.ID, time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 0, result.Detached)
	assert.Empty(t, rebuilt)

	var reloaded DailyReportSection
	require.NoError(t, repo.db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
}
