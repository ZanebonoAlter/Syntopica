package repository

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
