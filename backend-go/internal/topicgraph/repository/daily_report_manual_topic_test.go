package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// ── CreateManualTopic / GetComposeCandidates integration tests (PostgreSQL) ──
// These tests run against a testcontainer PostgreSQL DB (golden schema) via
// testutil.SetupTestDB. The CreateManualTopic happy-path — including the
// RebuildBoardRelations call that uses PG-specific SQL (period_date::date) — is
// covered by TestManualTopic_CreateAndReassign in
// daily_report_topic_integration_test.go. The cases here cover input
// validation, embedding usability, and the compose-candidate query.
//
// Pure-function tests (aggregateEmbeddings / detectOutliers / FloatsToPgVector)
// live in daily_report_manual_topic_unit_test.go and run under `go test -short`.

// setupManualTopicTestDB provisions a testcontainer PostgreSQL DB (golden
// schema) and returns a fresh repository. The production AutoMigrate run inside
// testutil.SetupTestDB already creates every table these tests touch
// (SemanticLabel / BoardDailyReport / DailyReportSection / BoardPersistentTopic
// / SectionRelation), so no manual AutoMigrate is needed. The package-global
// Repo is set because CreateManualTopic → RebuildBoardRelations reaches the
// package global via tx helpers (same convention as the other PG integration
// tests in this package).
func setupManualTopicTestDB(t *testing.T) *TopicGraphRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repo := NewTopicGraphRepository(db)
	Repo = repo
	return repo
}

func TestCreateManualTopic_NoSections(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	_, skipped, err := repo.CreateManualTopic(0, "test", nil)
	assert.Error(t, err)
	assert.Nil(t, skipped)
	assert.Contains(t, err.Error(), "no sections")
}

func TestCreateManualTopic_EmptySectionIDs(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	_, skipped, err := repo.CreateManualTopic(0, "test", []uint{})
	assert.Error(t, err)
	assert.Nil(t, skipped)
	assert.Contains(t, err.Error(), "no sections")
}

func TestCreateManualTopic_NoUsableEmbeddings(t *testing.T) {
	repo := setupManualTopicTestDB(t)

	boardID := seedTestBoard(t, repo.db)
	reportID := seedTestReport(t, repo.db, boardID, time.Now())
	// A section with no usable embedding. On PostgreSQL the vector column
	// rejects the empty string (SQLSTATE 22P02), so a "no embedding" section is
	// represented as NULL — the production-realistic state, and what a
	// non-pointer string column scans back to as "" (which repoParsePgVector
	// then rejects). Use Omit so embedding is NULL rather than "".
	sec := DailyReportSection{ReportID: reportID, ClusterLabel: "no-emb", ArticleCount: 1}
	require.NoError(t, repo.db.Omit("embedding").Create(&sec).Error)

	_, skipped, err := repo.CreateManualTopic(boardID, "test", []uint{sec.ID})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usable vector", "must reject when no section has a usable embedding")
	assert.Len(t, skipped, 1, "section without embedding must be skipped")
}

// ── GetComposeCandidates PostgreSQL tests ──────────────────────────────────

func TestGetComposeCandidates_ParsesEmbeddingsAndExcludesEmpty(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	today := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reportID := seedTestReport(t, repo.db, boardID, today)

	// Three sections: two with usable embeddings, one with no embedding. On
	// PostgreSQL the vector column rejects "" (SQLSTATE 22P02), so the
	// "no embedding" section is seeded as NULL (Omit); GetComposeCandidates'
	// `embedding IS NOT NULL` filter then excludes it, mirroring how SQLite
	// excluded it via the Go-level repoParsePgVector skip.
	s1 := DailyReportSection{ReportID: reportID, ClusterLabel: "s1", Embedding: vecStr(0.1, 0.2, 0.3), ArticleCount: 1}
	s2 := DailyReportSection{ReportID: reportID, ClusterLabel: "s2", Embedding: vecStr(0.4, 0.5, 0.6), ArticleCount: 1}
	s3 := DailyReportSection{ReportID: reportID, ClusterLabel: "no-emb", ArticleCount: 1}
	require.NoError(t, repo.db.Create(&s1).Error)
	require.NoError(t, repo.db.Create(&s2).Error)
	require.NoError(t, repo.db.Omit("embedding").Create(&s3).Error)

	resp, err := repo.GetComposeCandidates(boardID, 14)
	require.NoError(t, err)
	// Default MatchThreshold from DefaultPersistentTopicConfig (no ai_settings row).
	assert.InDelta(t, 0.30, resp.MatchThreshold, 1e-9)
	// Only the two sections with a usable embedding are returned (empty excluded).
	require.Len(t, resp.Sections, 2)
	assert.Equal(t, "s1", resp.Sections[0].ClusterLabel)
	assert.Len(t, resp.Sections[0].Embedding, 3)
	assert.InDeltaSlice(t, []float64{0.1, 0.2, 0.3}, resp.Sections[0].Embedding, 1e-9)
	assert.Equal(t, "s2", resp.Sections[1].ClusterLabel)
	// report_id 随 section 返回（编排态查线索复用，task 11.1）。
	assert.Equal(t, reportID, resp.Sections[0].ReportID)
	assert.Equal(t, reportID, resp.Sections[1].ReportID)
}

func TestGetComposeCandidates_AttachesTopicBrief(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	today := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	reportID := seedTestReport(t, repo.db, boardID, today)

	// A topic the section is currently assigned to. board_persistent_topics.embedding
	// is a non-nullable vector column, so the topic MUST carry a valid embedding
	// (production always sets it before CreateTopic; SQLite tolerated the zero value).
	topic := BoardPersistentTopic{
		SemanticBoardID: boardID, Label: "中东局势", Status: TopicStatusActive, Source: TopicSourceAuto,
		Embedding:     vecStr(0.2, 0.1),
		FirstSeenDate: today, LastSeenDate: today,
	}
	require.NoError(t, repo.CreateTopic(repo.db, &topic))

	tid := topic.ID
	sec := DailyReportSection{
		ReportID: reportID, ClusterLabel: "霍尔木兹", Embedding: vecStr(0.2, 0.1),
		PersistentTopicID: &tid, TopicMatchConfidence: TopicConfAnchorHit, ArticleCount: 1,
	}
	require.NoError(t, repo.db.Create(&sec).Error)

	resp, err := repo.GetComposeCandidates(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	got := resp.Sections[0]
	assert.Equal(t, sec.ID, got.ID)
	require.NotNil(t, got.PersistentTopicID)
	assert.Equal(t, tid, *got.PersistentTopicID)
	assert.Equal(t, TopicConfAnchorHit, got.TopicMatchConfidence)
	// Topic brief attached (label carried for the candidate pool display).
	require.NotNil(t, got.PersistentTopic)
	assert.Equal(t, "中东局势", got.PersistentTopic.Label)
}

func TestGetComposeCandidates_DaysWindowOutOfScope(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	// Latest report is recent; an older report falls outside the window.
	latest := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	recentReport := seedTestReport(t, repo.db, boardID, latest)
	oldReport := seedTestReport(t, repo.db, boardID, old)

	sRecent := DailyReportSection{ReportID: recentReport, ClusterLabel: "recent", Embedding: vecStr(0.1, 0.2), ArticleCount: 1}
	sOld := DailyReportSection{ReportID: oldReport, ClusterLabel: "old", Embedding: vecStr(0.9, 0.8), ArticleCount: 1}
	require.NoError(t, repo.db.Create(&sRecent).Error)
	require.NoError(t, repo.db.Create(&sOld).Error)

	// 14-day window anchored to the latest report (06-29) → the 2025 section is out.
	resp, err := repo.GetComposeCandidates(boardID, 14)
	require.NoError(t, err)
	require.Len(t, resp.Sections, 1)
	assert.Equal(t, "recent", resp.Sections[0].ClusterLabel)
}

func TestGetComposeCandidates_EmptyBoard(t *testing.T) {
	repo := setupManualTopicTestDB(t)
	boardID := seedTestBoard(t, repo.db)
	// No reports at all → graceful fallback (zero latest → cutoff far in past →
	// returns empty slice, never errors).
	resp, err := repo.GetComposeCandidates(boardID, 14)
	require.NoError(t, err)
	assert.Empty(t, resp.Sections)
	assert.InDelta(t, 0.30, resp.MatchThreshold, 1e-9)
}
