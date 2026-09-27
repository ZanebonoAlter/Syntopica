package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/testutil"
)

// Signal report results (board-signal-reports DB-3..DB-5): kind/shape CHECKs,
// candidate composite FK, append-only versions. Isolated testcontainer
// Postgres; illegal shapes are written DIRECTLY via SQL so PostgreSQL itself
// must reject them (no SQLite, no business database).

func setupSignalReportDB(t *testing.T) *repository.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t) // golden schema: AutoMigrate + all versioned migrations (CHECK/FK in place)
	for _, stmt := range []string{
		`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'signal-report-%'`,
		`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 94000 AND 94999`,
		`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 94000 AND 94999`,
	} {
		require.NoError(t, db.Exec(stmt).Error, stmt)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'signal-report-%'`).Error
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 94000 AND 94999`).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 94000 AND 94999`).Error
	})
	return repository.NewRepository(db)
}

// seedSignalCandidate creates one discovery batch with one candidate and
// returns the candidate id.
func seedSignalCandidate(t *testing.T, repo *repository.Repository, boardID uint, period string) uint {
	t.Helper()
	discovery := newSignalDiscovery(boardID, period, "signal-report-disc")
	persisted, _, err := repo.CreateSignalDiscoveryBatch(context.Background(), discovery, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "库存与开工背离", []string{"article:11"}),
	})
	require.NoError(t, err)
	require.Len(t, persisted, 1)
	return persisted[0].ID
}

func signalReportResult(boardID, candidateID uint, granularity, period, sessionID string) *repository.TopicEnrichmentResult {
	return &repository.TopicEnrichmentResult{
		SemanticBoardID: &boardID,
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindSignalReport,
		Sectors:         []byte(`{"schema_version":2,"report":{"title":"判断","sections":[],"charts":[]}}`),
		SessionID:       sessionID,
		Granularity:     stringPtr(granularity),
		Period:          stringPtr(period),
		SourceSignalID:  &candidateID,
	}
}

func stringPtrUInt(v uint) *uint { return &v }

func TestCreateSignalReportResultValidatesShape(t *testing.T) {
	repo := setupSignalReportDB(t)
	ctx := context.Background()
	boardID := uint(94101)
	candidateID := seedSignalCandidate(t, repo, boardID, "2026-08")

	// Valid: board scope + same-board candidate + matching period.
	result := signalReportResult(boardID, candidateID, "month", "2026-08", "signal-report-valid")
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, result))
	require.NotZero(t, result.ID)

	// Candidate from another board → repository-level rejection.
	otherBoard := uint(94102)
	otherCandidate := seedSignalCandidate(t, repo, otherBoard, "2026-08")
	crossBoard := signalReportResult(boardID, otherCandidate, "month", "2026-08", "signal-report-cross-board")
	err := repo.CreateTopicEnrichmentResult(ctx, crossBoard)
	require.Error(t, err, "candidate from another board must be rejected")

	// Period mismatch between result and candidate → rejected.
	wrongPeriod := signalReportResult(boardID, candidateID, "month", "2026-09", "signal-report-wrong-period")
	err = repo.CreateTopicEnrichmentResult(ctx, wrongPeriod)
	require.Error(t, err)

	// Missing period → rejected.
	missingPeriod := signalReportResult(boardID, candidateID, "month", "2026-08", "signal-report-missing-period")
	missingPeriod.Period = nil
	err = repo.CreateTopicEnrichmentResult(ctx, missingPeriod)
	require.Error(t, err)

	// Topic scope is not a legal signal_report shape.
	topicScope := signalReportResult(boardID, candidateID, "month", "2026-08", "signal-report-topic-scope")
	topicScope.SemanticBoardID = nil
	topicScope.PersistentTopicID = stringPtrUInt(42)
	topicScope.AnalysisScope = "topic"
	err = repo.CreateTopicEnrichmentResult(ctx, topicScope)
	require.Error(t, err)
}

func TestSignalReportDBConstraintsRejectIllegalShapes(t *testing.T) {
	repo := setupSignalReportDB(t)
	boardID := uint(94103)
	candidateID := seedSignalCandidate(t, repo, boardID, "2026-08")

	insert := func(sessionID, kind, scope string, board *uint, granularity, period string, source *uint) error {
		return repo.DB().Exec(`INSERT INTO topic_enrichment_result
			(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
			VALUES (NULL, ?, ?, ?, '{}'::jsonb, ?, NULLIF(?, '')::varchar, NULLIF(?, '')::varchar, ?)`,
			board, scope, kind, sessionID, granularity, period, source).Error
	}
	boardPtr := &boardID

	// Unknown kind → kind CHECK.
	require.Error(t, insert("signal-report-ck-1", "signal_weird", "board", boardPtr, "", "", nil))
	// signal_report without granularity → shape CHECK (NULL must fail: CHECK
	// treats UNKNOWN as satisfied, hence the explicit IS NOT NULL guards).
	require.Error(t, insert("signal-report-ck-2", repository.ResultKindSignalReport, "board", boardPtr, "", "2026-08", &candidateID))
	// signal_report without period → shape CHECK.
	require.Error(t, insert("signal-report-ck-2b", repository.ResultKindSignalReport, "board", boardPtr, "month", "", &candidateID))
	// signal_report with an impossible month → shape CHECK.
	require.Error(t, insert("signal-report-ck-3", repository.ResultKindSignalReport, "board", boardPtr, "month", "2026-13", &candidateID))
	// signal_report on topic scope → shape CHECK.
	require.Error(t, insert("signal-report-ck-4", repository.ResultKindSignalReport, "topic", nil, "month", "2026-08", &candidateID))
	// A legacy kind carrying signal columns → shape CHECK (三列仅 signal_report 可用).
	require.Error(t, insert("signal-report-ck-5", repository.ResultKindBoardBrief, "board", boardPtr, "month", "2026-08", nil))
	// signal_report with a NULL source candidate → shape CHECK (source_signal_id 必填).
	require.Error(t, insert("signal-report-ck-6", repository.ResultKindSignalReport, "board", boardPtr, "month", "2026-08", nil))
	// Legal row still passes.
	require.NoError(t, insert("signal-report-ck-ok", repository.ResultKindSignalReport, "board", boardPtr, "month", "2026-08", &candidateID))

	// Cross-board candidate reference: the composite FK pins the result to a
	// candidate of the SAME board AND period — a candidate owned by another
	// board can never satisfy it.
	otherBoard := uint(94106)
	otherCandidate := seedSignalCandidate(t, repo, otherBoard, "2026-08")
	err := repo.DB().Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', ?, '{}'::jsonb, 'signal-report-ck-7', 'month', '2026-08', ?)`,
		boardID, repository.ResultKindSignalReport, otherCandidate).Error
	require.Error(t, err, "result must reference a same-board candidate")
	// Result period must match the candidate period.
	err = repo.DB().Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', ?, '{}'::jsonb, 'signal-report-ck-8', 'month', '2026-07', ?)`,
		boardID, repository.ResultKindSignalReport, candidateID).Error
	require.Error(t, err, "result period must match the candidate period")
}

func TestSignalReportVersionsAppendNotOverwrite(t *testing.T) {
	repo := setupSignalReportDB(t)
	ctx := context.Background()
	boardID := uint(94104)
	candidateID := seedSignalCandidate(t, repo, boardID, "2026-08")

	first := signalReportResult(boardID, candidateID, "month", "2026-08", "signal-report-v1")
	first.Sectors = []byte(`{"schema_version":2,"report":{"title":"第一版"}}`)
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, first))

	// DB-5: explicit re-research appends a NEW version; the old row is never
	// touched (第二版≠覆盖第一版).
	second := signalReportResult(boardID, candidateID, "month", "2026-08", "signal-report-v2")
	second.Sectors = []byte(`{"schema_version":2,"report":{"title":"第二版"}}`)
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, second))
	require.Greater(t, second.ID, first.ID)

	var firstSectors string
	require.NoError(t, repo.DB().Raw(`SELECT sectors::text FROM topic_enrichment_result WHERE id = ?`, first.ID).Scan(&firstSectors).Error)
	require.Contains(t, firstSectors, "第一版", "old version must stay byte-intact")

	versions, err := repo.ListSignalReportVersionsByCandidate(ctx, boardID, candidateID, 0, 20)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	require.Equal(t, second.ID, versions[0].ID, "newest version first")

	latest, err := repo.GetLatestSignalReportIDForCandidate(ctx, candidateID)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.Equal(t, second.ID, *latest)
}

func TestListSignalReportResultsIsKindAndPeriodIsolated(t *testing.T) {
	repo := setupSignalReportDB(t)
	ctx := context.Background()
	boardID := uint(94105)
	candidateID := seedSignalCandidate(t, repo, boardID, "2026-08")

	// A legacy board_brief shares the board but must never appear.
	brief := &repository.TopicEnrichmentResult{
		SemanticBoardID: &boardID,
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardBrief,
		Sectors:         []byte(`{}`),
		SessionID:       "signal-report-legacy-brief",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, brief))

	report := signalReportResult(boardID, candidateID, "month", "2026-08", "signal-report-listed")
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, report))
	// A different period needs its own same-board candidate (repo-level
	// candidate identity check mirrors the DB FK).
	septemberCandidate := seedSignalCandidate(t, repo, boardID, "2026-09")
	otherPeriod := signalReportResult(boardID, septemberCandidate, "month", "2026-09", "signal-report-listed-2")
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, otherPeriod))

	august, err := repo.ListSignalReportResults(ctx, boardID, "month", "2026-08", 0, 20)
	require.NoError(t, err)
	require.Len(t, august, 1)
	require.Equal(t, report.ID, august[0].ID)
	require.NotNil(t, august[0].Granularity)
	require.Equal(t, "2026-08", *august[0].Period)

	// Legacy kind queries keep their semantics and never surface signal
	// reports; the signal kind is not part of the legacy board-kind set.
	legacy, err := repo.ListBoardEnrichmentResultsByKind(ctx, boardID, repository.ResultKindBoardBrief)
	require.NoError(t, err)
	require.Len(t, legacy, 1)
	_, err = repo.ListBoardEnrichmentResultsByKind(ctx, boardID, repository.ResultKindSignalReport)
	require.Error(t, err, "signal_report must not be listable through the legacy kind API")

	// Legacy rows keep the three signal columns NULL.
	var gran, period *string
	var source *uint
	require.NoError(t, repo.DB().Raw(`SELECT granularity, period, source_signal_id FROM topic_enrichment_result WHERE id = ?`, brief.ID).Row().Scan(&gran, &period, &source))
	require.Nil(t, gran)
	require.Nil(t, period)
	require.Nil(t, source)
}
