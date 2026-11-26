package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/testutil"
)

// Candidate ignore/restore disposition (redesign-enrichment-research-workbench
// 2.1 / S9): an independent marker row per candidate — candidate content,
// score, snapshot and reports never change; re-ignore keeps the first
// ignored_at; restore deletes the marker (idempotent); cross-board/unknown
// candidates return ErrSignalCandidateNotFound. Isolated testcontainer
// Postgres via testutil — no SQLite, no business database.

func setupSignalDispositionDB(t *testing.T) *repository.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t) // golden schema: AutoMigrate + all versioned migrations (disposition FK in place)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_disposition WHERE candidate_id IN
		(SELECT id FROM board_signal_candidate WHERE semantic_board_id BETWEEN 94000 AND 94999)`).Error)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 94000 AND 94999`).Error)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 94000 AND 94999`).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM board_signal_disposition WHERE candidate_id IN
			(SELECT id FROM board_signal_candidate WHERE semantic_board_id BETWEEN 94000 AND 94999)`).Error
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 94000 AND 94999`).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 94000 AND 94999`).Error
	})
	return repository.NewRepository(db)
}

func seedDispositionCandidate(t *testing.T, repo *repository.Repository, ctx context.Context, boardID uint, period, signal string) *repository.BoardSignalCandidate {
	t.Helper()
	discovery := newSignalDiscovery(boardID, period, "signal-disp-seed")
	persisted, _, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, signal, []string{"article:1"}),
	})
	require.NoError(t, err)
	require.Len(t, persisted, 1)
	return persisted[0]
}

func TestSignalDispositionIgnoreRestorePersistsAcrossReentry(t *testing.T) {
	repo := setupSignalDispositionDB(t)
	ctx := context.Background()
	boardID := uint(94101)
	candidate := seedDispositionCandidate(t, repo, ctx, boardID, "2026-08", "开工与库存背离")

	// Ignore: marker row appears; the candidate raw row is byte-identical
	// (S9: 原内容/报告/依据不变，零 LLM).
	before := *candidate
	firstIgnoredAt, err := repo.IgnoreSignalCandidate(ctx, boardID, candidate.ID)
	require.NoError(t, err)
	require.False(t, firstIgnoredAt.IsZero())
	stored, err := repo.GetSignalCandidateByID(ctx, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, before.Signal, stored.Signal)
	require.Equal(t, before.WhyItMatters, stored.WhyItMatters)
	require.Equal(t, before.ResearchQuestion, stored.ResearchQuestion)
	require.JSONEq(t, string(before.EvidenceRefs), string(stored.EvidenceRefs))
	require.Equal(t, before.Score, stored.Score)
	require.Equal(t, before.Rationale, stored.Rationale)
	// gorm autoCreateTime carries ns; PG stores µs — compare at storage precision.
	require.Equal(t, before.CreatedAt.Truncate(time.Microsecond).UTC(), stored.CreatedAt.UTC())

	// Re-ignore keeps the first time (幂等，不刷新 ignored_at).
	secondIgnoredAt, err := repo.IgnoreSignalCandidate(ctx, boardID, candidate.ID)
	require.NoError(t, err)
	require.Equal(t, firstIgnoredAt.UTC(), secondIgnoredAt.UTC())

	// Visibility filter: default index hides it, ignored view shows it,
	// all keeps the old client semantics (省略参数兼容).
	visible, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20, repository.SignalVisibilityVisible, repository.SignalCandidateViewAll, 0)
	require.NoError(t, err)
	require.Empty(t, visible)
	ignored, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20, repository.SignalVisibilityIgnored, repository.SignalCandidateViewAll, 0)
	require.NoError(t, err)
	require.Len(t, ignored, 1)
	require.NotNil(t, ignored[0].IgnoredAt)
	all, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20, repository.SignalVisibilityAll, repository.SignalCandidateViewAll, 0)
	require.NoError(t, err)
	require.Len(t, all, 1)

	// Restore: marker gone, candidate back in the visible index; the report
	// surface (GetLatestSignalReportIDForCandidate) is untouched by any of it.
	require.NoError(t, repo.RestoreSignalCandidate(ctx, boardID, candidate.ID))
	require.NoError(t, repo.RestoreSignalCandidate(ctx, boardID, candidate.ID), "restore is idempotent")
	marker, err := repo.GetSignalDisposition(ctx, candidate.ID)
	require.NoError(t, err)
	require.Nil(t, marker)
	visible, err = repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20, repository.SignalVisibilityVisible, repository.SignalCandidateViewAll, 0)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Nil(t, visible[0].IgnoredAt)
}

// TestSignalDispositionCrossBoardAndUnknownRejected locks the ownership
// backstop (处理写入故障与越权): another board's candidate and unknown ids
// both map to ErrSignalCandidateNotFound (handler → 404), and no marker row
// is written for them.
func TestSignalDispositionCrossBoardAndUnknownRejected(t *testing.T) {
	repo := setupSignalDispositionDB(t)
	ctx := context.Background()
	boardID := uint(94201)
	candidate := seedDispositionCandidate(t, repo, ctx, boardID, "2026-08", "他板候选不可越权忽略")

	_, err := repo.IgnoreSignalCandidate(ctx, boardID+1, candidate.ID)
	require.ErrorIs(t, err, repository.ErrSignalCandidateNotFound)
	err = repo.RestoreSignalCandidate(ctx, boardID+1, candidate.ID)
	require.ErrorIs(t, err, repository.ErrSignalCandidateNotFound)
	_, err = repo.IgnoreSignalCandidate(ctx, boardID, 999999999)
	require.ErrorIs(t, err, repository.ErrSignalCandidateNotFound)

	marker, err := repo.GetSignalDisposition(ctx, candidate.ID)
	require.NoError(t, err)
	require.Nil(t, marker, "rejected writes must not leave a marker row")
}

// TestSignalDispositionOtherBatchesUnaffected locks S9's scope promise:
// ignoring one candidate neither hides nor restores any other candidate in
// the same or later batches.
func TestSignalDispositionOtherBatchesUnaffected(t *testing.T) {
	repo := setupSignalDispositionDB(t)
	ctx := context.Background()
	boardID := uint(94301)
	first := seedDispositionCandidate(t, repo, ctx, boardID, "2026-08", "第一批被忽略的线索")
	second := seedDispositionCandidate(t, repo, ctx, boardID, "2026-09", "第二批不受影响")

	_, err := repo.IgnoreSignalCandidate(ctx, boardID, first.ID)
	require.NoError(t, err)
	visible, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-09", 0, 20, repository.SignalVisibilityVisible, repository.SignalCandidateViewAll, 0)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Equal(t, second.ID, visible[0].ID)
}
