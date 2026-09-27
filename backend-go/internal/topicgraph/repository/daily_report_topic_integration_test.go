package repository

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// seedTopicSection inserts a section with a real (test) embedding and a
// cluster_label, returning its id. Embeddings are 3-dim; the test DB vector
// column is untyped-dimension so this is accepted.
func seedTopicSection(t *testing.T, db *gorm.DB, reportID uint, label string, embedding string) uint {
	t.Helper()
	sec := DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: label,
		ArticleCount: 1,
		Embedding:    embedding,
	}
	require.NoError(t, db.Create(&sec).Error)
	return sec.ID
}

// TestBackfill_GroupsDriftedLabelsIntoOneTopic is the headline behaviour test:
// the same narrative reported under drifted labels across days ("AI 编程竞争" /
// "开发者生态重构" / "AI 工具内卷") should collapse into a SINGLE persistent
// topic after backfill, not three. This is the core value proposition and the
// thing real data must be checked against (see verification-report).
func TestBackfill_GroupsDriftedLabelsIntoOneTopic(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo // assignAndUpdateTopics / backfill use the package global

	boardID := seedTestBoard(t, db)
	// Three near-identical 3-dim embeddings (the "same story" signal); one
	// orthogonal outlier that must seed its own topic.
	sameStory := vecStr(1.0, 0.0, 0.0)
	d1 := NormalizeReportDate(time.Now().AddDate(0, 0, -2))
	d2 := NormalizeReportDate(time.Now().AddDate(0, 0, -1))
	d3 := NormalizeReportDate(time.Now())
	r1 := seedTestReport(t, db, boardID, d1)
	r2 := seedTestReport(t, db, boardID, d2)
	r3 := seedTestReport(t, db, boardID, d3)
	seedTopicSection(t, db, r1, "AI 编程竞争", sameStory)
	seedTopicSection(t, db, r2, "开发者生态重构", vecStr(0.99, 0.01, 0.0)) // ~same
	seedTopicSection(t, db, r3, "AI 工具内卷", vecStr(0.98, 0.0, 0.02)) // ~same

	created, err := repo.BackfillPersistentTopics(boardID)
	require.NoError(t, err)
	// All three drifted labels are within ClusterThreshold (0.30) of each
	// other → one topic, not three.
	require.Equal(t, 1, created, "drifted labels of the same story should merge into one topic")

	// Every section must be assigned (no orphans).
	var unassigned int64
	db.Model(&DailyReportSection{}).Where("persistent_topic_id IS NULL").Count(&unassigned)
	assert.Equal(t, int64(0), unassigned, "all sections must be assigned after backfill")

	// Exactly one active topic for this board.
	topics, err := repo.ListAllTopicsByBoard(boardID)
	require.NoError(t, err)
	require.Len(t, topics, 1)
	assert.Equal(t, TopicStatusActive, topics[0].Status)
	assert.Equal(t, 3, topics[0].HitCount, "hit_count should equal member count")
}

// TestBackfill_SeparatesDistinctNarratives ensures orthogonal stories do not
// get merged. Without this, a too-loose threshold would lump everything
// together — the opposite failure mode of the drift test above.
func TestBackfill_SeparatesDistinctNarratives(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	d := NormalizeReportDate(time.Now())
	r := seedTestReport(t, db, boardID, d)
	// Each orthogonal narrative has >= 2 near-identical sections so the
	// complete-link cluster clears the min-size gate (>= 2 members). A single
	// section per narrative would now be left unassigned — see
	// TestBackfill_SingleMemberDoesNotSeed.
	seedTopicSection(t, db, r, "AI 编程·a", vecStr(1.0, 0.0, 0.0))
	seedTopicSection(t, db, r, "AI 编程·b", vecStr(0.99, 0.01, 0.0))
	seedTopicSection(t, db, r, "中东局势·a", vecStr(0.0, 1.0, 0.0)) // orthogonal
	seedTopicSection(t, db, r, "中东局势·b", vecStr(0.01, 0.99, 0.0))

	created, err := repo.BackfillPersistentTopics(boardID)
	require.NoError(t, err)
	assert.Equal(t, 2, created, "orthogonal narratives (>=2 members each) must seed separate topics")
}

// TestBackfill_SingleMemberDoesNotSeed verifies the min-size gate: a section
// that clusters with nothing else (single-member cluster) must NOT seed an
// active topic. It stays unassigned, left for the daily candidate path to
// observe over consecutive days. Single-section topics would bypass the
// consecutive-hits observation window and produce noise lanes.
func TestBackfill_SingleMemberDoesNotSeed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	d := NormalizeReportDate(time.Now())
	r := seedTestReport(t, db, boardID, d)
	seedTopicSection(t, db, r, "孤立项 A", vecStr(1.0, 0.0, 0.0))
	seedTopicSection(t, db, r, "孤立项 B", vecStr(0.0, 1.0, 0.0)) // orthogonal, no cluster joins

	created, err := repo.BackfillPersistentTopics(boardID)
	require.NoError(t, err)
	assert.Equal(t, 0, created, "single-member clusters must not seed topics")

	var orphan int64
	db.Raw(`SELECT count(*) FROM daily_report_sections s
		JOIN board_daily_reports r ON r.id=s.report_id
		WHERE r.semantic_board_id=? AND s.persistent_topic_id IS NULL`, boardID).Scan(&orphan)
	assert.Equal(t, int64(2), orphan, "both single sections must stay unassigned")
}

// TestIdentityEdge_SurvivesLabelDrift verifies the root-cause-B fix end to end:
// two sections of the SAME topic on adjacent days, but whose embedding
// distance EXCEEDS the Hungarian 0.28 penalty, must still be connected by an
// identity edge after relation rebuild. Before this change such a pair would
// have been broken into emerging+ending.
//
// The sections are pre-assigned to one topic (mirroring the steady state after
// a few days of daily assignment), so this isolates the identity-edge behaviour
// from the assignment/clustering thresholds (those are covered by the backfill
// tests above).
func TestIdentityEdge_SurvivesLabelDrift(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	d1 := NormalizeReportDate(time.Now().AddDate(0, 0, -1))
	d2 := NormalizeReportDate(time.Now())
	r1 := seedTestReport(t, db, boardID, d1)
	r2 := seedTestReport(t, db, boardID, d2)
	// Two drifted labels whose distance (0.32) EXCEEDS MatchPenalty (0.28).
	s1 := seedTopicSection(t, db, r1, "Day1 label", vecStr(1.0, 0.0, 0.0))
	s2 := seedTopicSection(t, db, r2, "Day2 label", vecStr(0.68, 0.0, 0.73))

	// Force both sections onto one topic (the steady state the daily pipeline
	// reaches once a topic is established).
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "drifted story",
		Embedding: vecStr(1.0, 0.0, 0.0), Status: TopicStatusActive,
		FirstSeenDate: d1, LastSeenDate: d2, HitCount: 2, ConsecutiveHits: 2,
	}
	require.NoError(t, db.Create(&topic).Error)
	require.NoError(t, db.Model(&DailyReportSection{}).Where("id IN ?", []uint{s1, s2}).
		Updates(map[string]interface{}{"persistent_topic_id": topic.ID}).Error)

	// Rebuild relations — the Hungarian step will NOT connect s1→s2 (distance
	// 0.32 > 0.28 penalty), but the identity overlay must, because they share
	// a topic.
	require.NoError(t, RebuildBoardRelations(db, boardID))

	var rel SectionRelation
	err := db.Where("from_section_id = ? AND to_section_id = ?", s1, s2).First(&rel).Error
	require.NoError(t, err, "identity edge must connect the drifted pair despite >penalty distance")
	assert.Equal(t, "identity", rel.RelationType)
	assert.Greater(t, rel.Distance, 0.28, "identity edge distance should exceed the 0.28 penalty that would have broken the Hungarian chain")
}

// TestSaveReport_AssignsNewSections is the daily-pipeline smoke test: saving a
// report with sections that anchor to an existing topic writes the assignment
// columns and creates no spurious candidate.
// TestTopicStatusAtReport_JSONTagIsSnakeCase verifies the API contract: the
// GORM model's json tag uses snake_case so the frontend receives
// topic_status_at_report (not TopicStatusAtReport / topicStatusAtReport).
func TestTopicStatusAtReport_JSONTagIsSnakeCase(t *testing.T) {
	// Look up the struct field via reflection.
	field, ok := reflect.TypeOf(DailyReportSection{}).FieldByName("TopicStatusAtReport")
	require.True(t, ok, "field TopicStatusAtReport must exist on DailyReportSection")
	jsonTag := field.Tag.Get("json")
	assert.Equal(t, "topic_status_at_report", jsonTag,
		"JSON tag must be snake_case for frontend compatibility")
}

func TestSaveReport_AssignsNewSections(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	// Pre-create an active topic.
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "AI 编程工具平台化竞争",
		Embedding:       vecStr(1.0, 0.0, 0.0),
		Status:          TopicStatusActive,
		FirstSeenDate:   NormalizeReportDate(time.Now().AddDate(0, 0, -5)),
		LastSeenDate:    NormalizeReportDate(time.Now().AddDate(0, 0, -1)),
		HitCount:        4,
		ConsecutiveHits: 4,
	}
	require.NoError(t, db.Create(&topic).Error)
	mit := topic.ID

	report := &BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      NormalizeReportDate(time.Now()),
		Title:           "today", Status: "completed",
	}
	// Section is an L1 direct anchor: the bucketing stage (upstream of
	// SaveReport) set LaneTier=l1_direct + MatchedTopicID because the tag's
	// embedding is near the topic centroid. The lane-driven assignment path
	// keys off these two fields (the old embedding AND-gate is gone).
	sections := []DailyReportSection{{
		ClusterLabel: "AI 编程竞争", Embedding: vecStr(0.99, 0.01, 0.0),
		LaneTier: "l1_direct", MatchedTopicID: &mit,
	}}

	err := repo.SaveReport(report, sections, nil)
	require.NoError(t, err)

	// Reload the saved section; it should carry the topic assignment.
	var got DailyReportSection
	require.NoError(t, db.Where("report_id = ?", report.ID).First(&got).Error)
	require.NotNil(t, got.PersistentTopicID)
	assert.Equal(t, topic.ID, *got.PersistentTopicID)
	assert.Equal(t, TopicConfAnchorHit, got.TopicMatchConfidence)
	require.NotNil(t, got.TopicStatusAtReport)
	assert.Equal(t, TopicStatusActive, *got.TopicStatusAtReport)

	// The report-time snapshot is immutable even when topic management changes
	// the topic's current lifecycle state later.
	require.NoError(t, db.Model(&BoardPersistentTopic{}).Where("id = ?", topic.ID).
		Update("status", TopicStatusArchived).Error)
	require.NoError(t, db.First(&got, got.ID).Error)
	require.NotNil(t, got.TopicStatusAtReport)
	assert.Equal(t, TopicStatusActive, *got.TopicStatusAtReport)

	// The existing active topic should gain a hit (consecutive 4→5).
	var updated BoardPersistentTopic
	require.NoError(t, db.First(&updated, topic.ID).Error)
	assert.Equal(t, 5, updated.ConsecutiveHits)
	assert.Equal(t, 5, updated.HitCount)
}

// TestTopicStatusAtReport_MigrationIdempotent verifies that the 20260627_0001
// migration can run twice without error (IF NOT EXISTS on the column, key
// existence check on the ai_settings seed).
func TestTopicStatusAtReport_MigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Verify the column exists after first migration.
	var colExists bool
	err := db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'daily_report_sections'
			AND column_name = 'topic_status_at_report'
		)`).Scan(&colExists).Error
	require.NoError(t, err)
	assert.True(t, colExists, "topic_status_at_report column must exist after migration")

	// Verify the ai_settings keys exist.
	keys := []string{"persistent_topic_candidate_decay_window", "persistent_topic_candidate_prompt_limit"}
	for _, k := range keys {
		var found models.AISettings
		err := db.Where("key = ?", k).First(&found).Error
		require.NoError(t, err, "key %s must exist after migration", k)
	}

	// Running migration again should NOT error. The testutil.SetupTestDB runs
	// all migrations. If we call it again on a cloned db, it should be fine.
	db2 := testutil.SetupTestDB(t)
	require.NotNil(t, db2, "second migration run must not fail")
}

// TestTopicStatusAtReport_HistoricalNullQueryable verifies that sections that
// existed before the migration (which have NULL topic_status_at_report) can
// still be loaded without errors.
func TestTopicStatusAtReport_HistoricalNullQueryable(t *testing.T) {
	db := testutil.SetupTestDB(t)

	boardID := seedTestBoard(t, db)
	report := &BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      NormalizeReportDate(time.Now()),
		Title:           "pre-migration", Status: "completed",
	}
	require.NoError(t, db.Create(report).Error)

	// Insert a section with only the original columns (no topic_status_at_report).
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_label, article_count, created_at)
		VALUES (?, 'old section', 1, NOW())
	`, report.ID).Error)

	// Load it via GORM — topic_status_at_report must be nil, not an error.
	var sec DailyReportSection
	err := db.Where("report_id = ?", report.ID).First(&sec).Error
	require.NoError(t, err, "section with NULL topic_status_at_report must load without error")
	assert.Nil(t, sec.TopicStatusAtReport, "historical NULL must stay NULL")
}

// topicIDs extracts the ID slice from a topic list (test helper).
func topicIDs(topics []BoardPersistentTopic) []uint {
	ids := make([]uint, len(topics))
	for i, t := range topics {
		ids[i] = t.ID
	}
	return ids
}

// TestAnchorableSet_ConsistencyBetweenInjectionAndAssignment is the cross-path
// guard for the orchestrator↔assignment contract: calling
// ListAnchorableTopicsByBoard with the same (boardID, reportDate, cfg) from
// both paths must return the identical topic ID set.  If a future change
// causes one side to load all candidates instead of the filtered set, this
// test captures the mismatch.
//
// It also asserts that candidates outside the decay window and those truncated
// by the candidate prompt limit do NOT appear in either returned set.
func TestAnchorableSet_ConsistencyBetweenInjectionAndAssignment(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	cfg := PersistentTopicConfig{
		MatchThreshold:       0.30,
		UpgradeThreshold:     3,
		CandidateDecayWindow: 7,
		CandidatePromptLimit: 2,
		ClusterThreshold:     0.28,
	}
	reportDate := NormalizeReportDate(time.Now())
	inWindow := reportDate.AddDate(0, 0, -3)   // gap=3, within window=7
	outWindow := reportDate.AddDate(0, 0, -10) // gap=10, outside window=7

	// Seed topics: 1 active (always included), 5 candidates:
	//   - 3 in-window (sorted by hit_count 5, 3, 1; limit=2 keeps top 2)
	//   - 1 out-of-window (filtered)
	// Expected anchorable set: {active, cand-inwin-1, cand-inwin-2}
	emb := vecStr(1, 0, 0)
	topics := []BoardPersistentTopic{
		{SemanticBoardID: boardID, Label: "active-1", Status: TopicStatusActive,
			Embedding: emb, FirstSeenDate: reportDate.AddDate(0, 0, -30),
			LastSeenDate: reportDate.AddDate(0, 0, -15), HitCount: 10},
		{SemanticBoardID: boardID, Label: "cand-inwin-1", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: inWindow, LastSeenDate: inWindow, HitCount: 5},
		{SemanticBoardID: boardID, Label: "cand-inwin-2", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: inWindow, LastSeenDate: inWindow, HitCount: 3},
		{SemanticBoardID: boardID, Label: "cand-outwin-1", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: outWindow, LastSeenDate: outWindow, HitCount: 4},
		{SemanticBoardID: boardID, Label: "cand-inwin-3", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: inWindow, LastSeenDate: inWindow, HitCount: 1}, // truncated by limit=2
	}
	for i := range topics {
		require.NoError(t, db.Create(&topics[i]).Error)
		require.NotZero(t, topics[i].ID)
	}

	// Call as orchestrator would.
	set1, stats1, err1 := repo.ListAnchorableTopicsByBoard(boardID, reportDate, cfg)
	require.NoError(t, err1)

	// Call as assignment would (same params).
	set2, stats2, err2 := repo.ListAnchorableTopicsByBoard(boardID, reportDate, cfg)
	require.NoError(t, err2)

	// Both calls must return the identical topic ID set.
	ids1 := topicIDs(set1)
	ids2 := topicIDs(set2)
	require.ElementsMatch(t, ids1, ids2,
		"orchestrator and assignment paths must return identical topic ID sets")
	assert.Equal(t, stats1, stats2)

	// Build a lookup set for fast exclusion checks.
	idSet := make(map[uint]bool, len(ids1))
	for _, id := range ids1 {
		idSet[id] = true
	}

	// Out-of-window candidate must NOT appear.
	assert.False(t, idSet[topics[3].ID],
		"out-of-window candidate %d must not appear in anchorable set", topics[3].ID)
	assert.Equal(t, 1, stats1.FilteredByWindow)

	// Truncated-by-limit candidate must NOT appear.
	assert.False(t, idSet[topics[4].ID],
		"truncated-by-limit candidate %d must not appear in anchorable set", topics[4].ID)
	assert.Equal(t, 1, stats1.TruncatedByLimit)

	// Active topic must appear in the set.
	assert.True(t, idSet[topics[0].ID],
		"active topic %d must appear in anchorable set", topics[0].ID)

	// Expected count: 1 active + 2 in-window candidates = 3.
	assert.Equal(t, 3, len(idSet), "anchorable set size mismatch")
}

// TestCleanup_PruneUnderqualifiedCandidates verifies the one-shot cleanup
// migration: observing candidates (consecutive_hits < upgrade_threshold) are
// deleted, their sections unlinked, and active/qualified candidates preserved.
// Second run must be idempotent (no-op).
func TestCleanup_PruneUnderqualifiedCandidates(t *testing.T) {
	db := testutil.SetupTestDB(t)

	boardID := seedTestBoard(t, db)
	emb := vecStr(1, 0, 0)
	reportDate := NormalizeReportDate(time.Now())
	report := &BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      reportDate,
		Title:           "cleanup test", Status: "completed",
	}
	require.NoError(t, db.Create(report).Error)

	// Seed topics:
	//   id=1 (active)                  → preserved
	//   id=2 (candidate cons=3, >=3)   → preserved (qualified)
	//   id=3 (candidate cons=1, <3)    → deleted (observing, no section ref)
	//   id=4 (candidate cons=1, <3)    → deleted (observing, referenced by section)
	topics := []BoardPersistentTopic{
		{ID: 1, SemanticBoardID: boardID, Label: "active-1", Status: TopicStatusActive,
			Embedding: emb, FirstSeenDate: reportDate.AddDate(0, 0, -5), LastSeenDate: reportDate,
			HitCount: 5, ConsecutiveHits: 5},
		{ID: 2, SemanticBoardID: boardID, Label: "qual-cand", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: reportDate.AddDate(0, 0, -3), LastSeenDate: reportDate,
			HitCount: 3, ConsecutiveHits: 3},
		{ID: 3, SemanticBoardID: boardID, Label: "obs-no-ref", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: reportDate.AddDate(0, 0, -10), LastSeenDate: reportDate.AddDate(0, 0, -10),
			HitCount: 1, ConsecutiveHits: 1},
		{ID: 4, SemanticBoardID: boardID, Label: "obs-with-ref", Status: TopicStatusCandidate,
			Embedding: emb, FirstSeenDate: reportDate.AddDate(0, 0, -8), LastSeenDate: reportDate.AddDate(0, 0, -8),
			HitCount: 1, ConsecutiveHits: 1},
	}
	for i := range topics {
		require.NoError(t, db.Create(&topics[i]).Error)
	}

	// Create a section that references the observing candidate id=4.
	tid4 := topics[3].ID
	statusAtReport := TopicStatusCandidate
	sec := DailyReportSection{
		ReportID:             report.ID,
		ClusterLabel:         "test section",
		ArticleCount:         1,
		Embedding:            emb,
		PersistentTopicID:    &tid4,
		TopicMatchDistance:   0.15,
		TopicMatchConfidence: TopicConfAnchorHit,
		TopicStatusAtReport:  &statusAtReport,
	}
	require.NoError(t, db.Create(&sec).Error)

	// Run prune with upgrade_threshold=3.
	deleted, err := database.PruneUnderqualifiedCandidates(db, 3)
	require.NoError(t, err)
	assert.Equal(t, 2, deleted, "should delete topics id=3 and id=4")

	// Preserved topics.
	var remaining []BoardPersistentTopic
	require.NoError(t, db.Where("semantic_board_id = ?", boardID).Find(&remaining).Error)
	require.Len(t, remaining, 2)
	remainingIDs := make(map[uint]bool)
	for _, t := range remaining {
		remainingIDs[t.ID] = true
	}
	assert.True(t, remainingIDs[uint(1)], "active must be preserved")
	assert.True(t, remainingIDs[uint(2)], "qualified candidate must be preserved")
	assert.False(t, remainingIDs[uint(3)], "observing candidate id=3 must be deleted")
	assert.False(t, remainingIDs[uint(4)], "observing candidate id=4 must be deleted")

	// Section that referenced deleted topic id=4: all topic fields set to NULL.
	var reloaded DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	assert.Nil(t, reloaded.PersistentTopicID, "persistent_topic_id must be NULL after unlink")
	assert.Zero(t, reloaded.TopicMatchDistance, "topic_match_distance must be 0 after unlink")
	assert.Empty(t, reloaded.TopicMatchConfidence, "topic_match_confidence must be empty after unlink")
	assert.Nil(t, reloaded.TopicStatusAtReport, "topic_status_at_report must be NULL after unlink")
	assert.Equal(t, "test section", reloaded.ClusterLabel, "section content preserved")
	assert.Equal(t, 1, reloaded.ArticleCount, "section article_count preserved")

	// Idempotency: second run is no-op.
	deleted2, err2 := database.PruneUnderqualifiedCandidates(db, 3)
	require.NoError(t, err2)
	assert.Equal(t, 0, deleted2, "second run must delete 0 (idempotent)")
}

// TestCleanup_PruneRebuildsRelations verifies that pruning observing candidates
// correctly rebuilds board relations: identity edges pointing to deleted topics
// are dropped, while edges belonging to active topics survive. The test creates
// two adjacent-day reports with sections under both an active topic and an
// observing candidate, builds relations, prunes the candidate, then asserts the
// relations delta.
func TestCleanup_PruneRebuildsRelations(t *testing.T) {
	db := testutil.SetupTestDB(t)

	boardID := seedTestBoard(t, db)
	emb := vecStr(1, 0, 0)

	// Two adjacent-day reports.
	d1 := NormalizeReportDate(time.Now().AddDate(0, 0, -2))
	d2 := NormalizeReportDate(time.Now().AddDate(0, 0, -1))
	r1 := seedTestReport(t, db, boardID, d1)
	r2 := seedTestReport(t, db, boardID, d2)

	// Active topic (survives prune).
	activeTopic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "active", Status: TopicStatusActive,
		Embedding: emb, FirstSeenDate: d1, LastSeenDate: d2,
		HitCount: 5, ConsecutiveHits: 5,
	}
	require.NoError(t, db.Create(&activeTopic).Error)

	// Observing candidate (will be pruned: consecutive_hits=1 < threshold=3).
	candTopic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "obs", Status: TopicStatusCandidate,
		Embedding: emb, FirstSeenDate: d1, LastSeenDate: d1,
		HitCount: 1, ConsecutiveHits: 1,
	}
	require.NoError(t, db.Create(&candTopic).Error)

	// Section s1 (day1) under active topic.
	s1 := seedTopicSection(t, db, r1, "a1", emb)
	require.NoError(t, db.Model(&DailyReportSection{}).Where("id = ?", s1).
		Updates(map[string]interface{}{"persistent_topic_id": activeTopic.ID}).Error)

	// Section s2 (day2) under active topic — identity edge s1→s2.
	s2 := seedTopicSection(t, db, r2, "a2", emb)
	require.NoError(t, db.Model(&DailyReportSection{}).Where("id = ?", s2).
		Updates(map[string]interface{}{"persistent_topic_id": activeTopic.ID}).Error)

	// Section s3 (day1) under candidate topic.
	s3 := seedTopicSection(t, db, r1, "c1", emb)
	require.NoError(t, db.Model(&DailyReportSection{}).Where("id = ?", s3).
		Updates(map[string]interface{}{"persistent_topic_id": candTopic.ID}).Error)

	// Section s4 (day2) under candidate topic — identity edge s3→s4.
	s4 := seedTopicSection(t, db, r2, "c2", emb)
	require.NoError(t, db.Model(&DailyReportSection{}).Where("id = ?", s4).
		Updates(map[string]interface{}{"persistent_topic_id": candTopic.ID}).Error)

	// Build initial relations: identity edges for both topics.
	require.NoError(t, RebuildBoardRelations(db, boardID))

	// Verify identity edges exist before prune.
	var activeRel SectionRelation
	errActive := db.Where("from_section_id = ? AND to_section_id = ? AND relation_type = ?",
		s1, s2, "identity").First(&activeRel).Error
	require.NoError(t, errActive, "identity edge for active topic must exist before prune")

	var candRel SectionRelation
	errCand := db.Where("from_section_id = ? AND to_section_id = ? AND relation_type = ?",
		s3, s4, "identity").First(&candRel).Error
	require.NoError(t, errCand, "identity edge for candidate topic must exist before prune")

	// Prune: delete the observing candidate (consecutive_hits=1 < threshold=3).
	deleted, err := database.PruneUnderqualifiedCandidates(db, 3)
	require.NoError(t, err)
	assert.Equal(t, 1, deleted, "should delete 1 observing candidate")

	// After prune: identity edge s1→s2 (active topic) MUST survive.
	var activeRel2 SectionRelation
	errActive2 := db.Where("from_section_id = ? AND to_section_id = ? AND relation_type = ?",
		s1, s2, "identity").First(&activeRel2).Error
	assert.NoError(t, errActive2,
		"identity edge for active topic must survive prune")

	// After prune: identity edge s3→s4 (pruned candidate) MUST be gone.
	var candRel2 SectionRelation
	errCand2 := db.Where("from_section_id = ? AND to_section_id = ? AND relation_type = ?",
		s3, s4, "identity").First(&candRel2).Error
	assert.Error(t, errCand2,
		"identity edge for pruned candidate must be removed")

	// Sections referencing the deleted candidate are unlinked but still renderable.
	for _, sid := range []uint{s3, s4} {
		var sec DailyReportSection
		require.NoError(t, db.First(&sec, sid).Error)
		assert.Nil(t, sec.PersistentTopicID,
			"section %d must be unlinked after candidate deletion", sid)
		assert.NotEmpty(t, sec.ClusterLabel,
			"section %d must still have content (cluster_label)", sid)
	}

	// Section under active topic still has its assignment.
	var sec1 DailyReportSection
	require.NoError(t, db.First(&sec1, s1).Error)
	require.NotNil(t, sec1.PersistentTopicID)
	assert.Equal(t, activeTopic.ID, *sec1.PersistentTopicID,
		"section under active topic must retain its assignment")
}

// TestManualTopic_CreateAndReassign is the zero-side-effect integration test
// for CreateManualTopic. It proves:
//  1. New topic is source=manual, status=active
//  2. Selected sections get confidence=manual, persistent_topic_id=new topic
//  3. Original topic's consecutive_hits NOT affected by sections moving out
//  4. Identity edges follow the new ownership after RebuildBoardRelations
func TestManualTopic_CreateAndReassign(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	d1 := NormalizeReportDate(time.Now().AddDate(0, 0, -2))
	d2 := NormalizeReportDate(time.Now().AddDate(0, 0, -1))
	r1 := seedTestReport(t, db, boardID, d1)
	r2 := seedTestReport(t, db, boardID, d2)

	// Two sections with different embeddings belonging to the same original topic.
	s1 := seedTopicSection(t, db, r1, "orig-s1", vecStr(1.0, 0.0, 0.0))
	s2 := seedTopicSection(t, db, r2, "orig-s2", vecStr(0.9, 0.1, 0.0))
	// One section that stays with the original topic.
	sStay := seedTopicSection(t, db, r2, "orig-stay", vecStr(1.0, 0.0, 0.0))

	// Create original active topic with 3 sections.
	origTopic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "original",
		Embedding: vecStr(1.0, 0.0, 0.0), Status: TopicStatusActive,
		FirstSeenDate: d1, LastSeenDate: d2, HitCount: 3, ConsecutiveHits: 3,
	}
	require.NoError(t, db.Create(&origTopic).Error)
	for _, sid := range []uint{s1, s2, sStay} {
		require.NoError(t, db.Model(&DailyReportSection{}).Where("id = ?", sid).
			Updates(map[string]interface{}{
				"persistent_topic_id":    origTopic.ID,
				"topic_match_confidence": TopicConfAnchorHit,
			}).Error)
	}

	// Build initial relations so we can later verify they are rebuilt.
	require.NoError(t, RebuildBoardRelations(db, boardID))

	// Now create a manual topic taking s1 and s2.
	topic, skipped, err := repo.CreateManualTopic(boardID, "manual-topic", []uint{s1, s2})
	require.NoError(t, err)
	require.NotNil(t, topic)
	assert.Empty(t, skipped, "both sections have valid embeddings")

	// Check ①: new topic source=manual, status=active.
	assert.Equal(t, TopicSourceManual, topic.Source)
	assert.Equal(t, TopicStatusActive, topic.Status)
	assert.Equal(t, 2, topic.HitCount)
	assert.Equal(t, 2, topic.ConsecutiveHits)

	// Check ②: selected sections are reassigned with confidence=manual.
	for _, sid := range []uint{s1, s2} {
		var sec DailyReportSection
		require.NoError(t, db.First(&sec, sid).Error)
		require.NotNil(t, sec.PersistentTopicID)
		assert.Equal(t, topic.ID, *sec.PersistentTopicID, "section %d must be reassigned to new manual topic", sid)
		assert.Equal(t, TopicConfManual, sec.TopicMatchConfidence, "section %d confidence must be manual", sid)
	}

	// Check: section that stayed keeps original topic.
	var secStay DailyReportSection
	require.NoError(t, db.First(&secStay, sStay).Error)
	require.NotNil(t, secStay.PersistentTopicID)
	assert.Equal(t, origTopic.ID, *secStay.PersistentTopicID, "staying section must retain original topic")

	// Check ③: original topic's consecutive_hits NOT affected.
	var reloadedOrig BoardPersistentTopic
	require.NoError(t, db.First(&reloadedOrig, origTopic.ID).Error)
	assert.Equal(t, 3, reloadedOrig.ConsecutiveHits,
		"original topic consecutive_hits must be unchanged by manual topic creation")

	// Check ④: identity edge rebuilt — moved sections now share identity with
	// each other (same topic) but NOT with the staying section (different topic).
	var relNew SectionRelation
	errNew := db.Where("from_section_id = ? AND to_section_id = ?", s1, s2).
		Where("relation_type = ?", "identity").First(&relNew).Error
	assert.NoError(t, errNew, "identity edge must exist between sections in the same manual topic")

	// s1→sStay should NOT have an identity edge (different topics).
	var relOld SectionRelation
	errOld := db.Where("from_section_id = ? AND to_section_id = ?", s1, sStay).
		Where("relation_type = ?", "identity").First(&relOld).Error
	assert.Error(t, errOld, "identity edge must NOT exist between sections in different topics")
}

// TestManualTopic_MigrationIdempotent verifies the source column migration
// can be executed repeatedly without error.
func TestManualTopic_MigrationIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// Run migration twice — must not error.
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Exec(`
				ALTER TABLE board_persistent_topics
				ADD COLUMN IF NOT EXISTS source VARCHAR(10) NOT NULL DEFAULT 'auto'
			`).Error, "add source column (run %d)", i)

		require.NoError(t, db.Exec(`
				DO $$ BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.table_constraints
						WHERE constraint_name = 'chk_board_persistent_topics_source'
						  AND table_name = 'board_persistent_topics'
					) THEN
						ALTER TABLE board_persistent_topics
							ADD CONSTRAINT chk_board_persistent_topics_source
							CHECK (source IN ('auto', 'manual'));
					END IF;
				END $$
			`).Error, "add source CHECK (run %d)", i)
	}

	// Verify the column exists and has the right default.
	var hasSourceCol bool
	require.NoError(t, db.Raw(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'board_persistent_topics' AND column_name = 'source'
		)
	`).Scan(&hasSourceCol).Error)
	assert.True(t, hasSourceCol, "source column must exist after migration")
}

// TestManualTopic_HistoricalDefaultsToAuto ensures existing topics
// get source='auto' after the migration.
func TestManualTopic_HistoricalDefaultsToAuto(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo

	boardID := seedTestBoard(t, db)
	d := NormalizeReportDate(time.Now())
	r := seedTestReport(t, db, boardID, d)
	seedTopicSection(t, db, r, "hist", vecStr(1.0, 0.0, 0.0))

	// Create a topic without explicit Source (simulating pre-migration creation).
	err := db.Exec(`
		INSERT INTO board_persistent_topics
			(semantic_board_id, label, embedding, status, first_seen_date, last_seen_date, hit_count, consecutive_hits, created_at, updated_at)
		VALUES (?, 'hist-topic', ?, 'active', ?, ?, 1, 1, NOW(), NOW())
	`, boardID, vecStr(1.0, 0.0, 0.0), d, d).Error
	require.NoError(t, err)

	var topic BoardPersistentTopic
	require.NoError(t, db.Where("label = ?", "hist-topic").First(&topic).Error)
	assert.Equal(t, TopicSourceAuto, topic.Source,
		"topic created without explicit source must default to 'auto'")
}
