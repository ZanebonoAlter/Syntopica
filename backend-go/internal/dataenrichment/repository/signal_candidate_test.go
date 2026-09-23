package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/testutil"
)

// Signal discovery batch/candidate persistence (board-signal-reports DB-1/2,
// S1 persistence beats). Isolated testcontainer Postgres via testutil — no
// SQLite, no business database.

func setupSignalCandidateDB(t *testing.T) *repository.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t) // golden schema: AutoMigrate + all versioned migrations (CHECK/FK in place)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 93000 AND 93999`).Error)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 93000 AND 93999`).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 93000 AND 93999`).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 93000 AND 93999`).Error
	})
	return repository.NewRepository(db)
}

func testSignalCutoff() time.Time { return time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC) }

func signalRefs(raw []string) json.RawMessage {
	b, err := json.Marshal(raw)
	if err != nil {
		panic(err)
	}
	return b
}

func signalCandidate(boardID uint, signal string, refs []string) *repository.BoardSignalCandidate {
	return &repository.BoardSignalCandidate{
		SemanticBoardID:  boardID,
		Signal:           signal,
		WhyItMatters:     "它改变对供给过剩的判断",
		ResearchQuestion: "库存降而开工降，是谁在去库？",
		EvidenceRefs:     signalRefs(refs),
		Score:            7,
		Rationale:        "多篇报道同时提及炼厂开工异常",
	}
}

func newSignalDiscovery(boardID uint, period, sessionID string) *repository.BoardSignalDiscovery {
	return &repository.BoardSignalDiscovery{
		SemanticBoardID: boardID,
		Granularity:     repository.SignalGranularityMonth,
		Period:          period,
		AnalysisMode:    repository.SignalAnalysisModeCurrent,
		Cutoff:          testSignalCutoff(),
		InputSnapshot:   []byte(`{"lanes":[1,2]}`),
		SessionID:       sessionID,
	}
}

func TestCreateSignalDiscoveryBatchAtomicSave(t *testing.T) {
	repo := setupSignalCandidateDB(t)
	ctx := context.Background()
	boardID := uint(93101)

	discovery := newSignalDiscovery(boardID, "2026-08", "signal-cand-atomic")
	discovery.AnalysisMode = repository.SignalAnalysisModeRetrospective
	persisted, deduped, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "炼厂开工与库存背离", []string{"article:1", "article:2"}),
		signalCandidate(boardID, "出口放量但油价滞涨", []string{"article:3"}),
	})
	require.NoError(t, err)
	require.Equal(t, 0, deduped)
	require.Len(t, persisted, 2)
	require.Equal(t, 2, discovery.CandidateCount)
	require.NotZero(t, discovery.ID)

	// Rows exist with owner/period stamped from the batch; discovery time
	// comes from the batch row (S1: 发现时间来自批次).
	stored, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	require.Equal(t, discovery.ID, stored[0].DiscoveryID)
	require.Equal(t, boardID, stored[0].SemanticBoardID)
	require.Equal(t, "2026-08", stored[0].Period)
	require.False(t, stored[0].DiscoveryCreatedAt.IsZero())
	// Newest batch first, candidate id descending within the batch.
	require.Greater(t, stored[0].ID, stored[1].ID)

	// DB-1 backstop: a direct SQL candidate whose owner/period disagrees with
	// its batch is rejected by the composite FK (PostgreSQL, bypassing GORM).
	var discoveryID uint
	require.NoError(t, repo.DB().Raw(`SELECT id FROM board_signal_discovery WHERE session_id = ?`, "signal-cand-atomic").Scan(&discoveryID).Error)
	require.NotZero(t, discoveryID)
	err = repo.DB().Exec(`INSERT INTO board_signal_candidate
		(discovery_id, semantic_board_id, granularity, period, signal, why_it_matters, research_question, evidence_refs, score, rationale)
		VALUES (?, ?, 'month', '2026-08', 'x', 'y', 'z', '["a"]'::jsonb, 6, 'r')`,
		discoveryID, boardID+100).Error
	require.Error(t, err, "candidate on a different board must be rejected")
	err = repo.DB().Exec(`INSERT INTO board_signal_candidate
		(discovery_id, semantic_board_id, granularity, period, signal, why_it_matters, research_question, evidence_refs, score, rationale)
		VALUES (?, ?, 'month', '2026-09', 'x', 'y', 'z', '["a"]'::jsonb, 6, 'r')`,
		discoveryID, boardID).Error
	require.Error(t, err, "candidate on a different period must be rejected")
}

func TestCreateSignalDiscoveryBatchZeroCandidates(t *testing.T) {
	repo := setupSignalCandidateDB(t)
	ctx := context.Background()
	boardID := uint(93102)

	discovery := newSignalDiscovery(boardID, "2026-09", "signal-cand-zero")
	persisted, deduped, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, nil)
	require.NoError(t, err)
	require.Empty(t, persisted)
	require.Equal(t, 0, deduped)
	require.Equal(t, 0, discovery.CandidateCount)

	// A zero-candidate batch is a legal persisted outcome (安静空态) and must
	// NOT clear older candidates of the same board (other period here).
	older := newSignalDiscovery(boardID, "2026-08", "signal-cand-zero-older")
	_, _, err = repo.CreateSignalDiscoveryBatch(ctx, older, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "更早批次的候选", []string{"article:9"}),
	})
	require.NoError(t, err)
	count, err := repo.CountSignalDiscoveriesByPeriod(ctx, boardID, "month", "2026-08")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	list, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20)
	require.NoError(t, err)
	require.Len(t, list, 1, "empty new batch must not clear existing candidates")
}

func TestCreateSignalDiscoveryBatchDedupesWithinBatchOnly(t *testing.T) {
	repo := setupSignalCandidateDB(t)
	ctx := context.Background()
	boardID := uint(93103)

	discovery := newSignalDiscovery(boardID, "2026-08", "signal-cand-dedupe")
	persisted, deduped, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "同一信号标题", []string{"article:2", "article:1"}),
		signalCandidate(boardID, "同一信号标题", []string{"article:1", "article:2"}), // identical title + evidence set (order-insensitive)
		signalCandidate(boardID, "同一信号标题", []string{"article:1", "article:3"}), // different evidence set → kept
		signalCandidate(boardID, "另一条信号", []string{"article:1"}),               // different title → kept
	})
	require.NoError(t, err)
	require.Equal(t, 1, deduped)
	require.Len(t, persisted, 3)
	require.Equal(t, 3, discovery.CandidateCount)

	// Cross-batch duplicates are never merged: a second batch with the same
	// candidate appends (DB-2: 成功追加、不覆盖旧).
	second := newSignalDiscovery(boardID, "2026-08", "signal-cand-dedupe-2")
	_, deduped2, err := repo.CreateSignalDiscoveryBatch(ctx, second, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "同一信号标题", []string{"article:2", "article:1"}),
	})
	require.NoError(t, err)
	require.Equal(t, 0, deduped2)
	list, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20)
	require.NoError(t, err)
	require.Len(t, list, 4, "re-discovery appends a new batch instead of overwriting")
}

func TestCreateSignalDiscoveryBatchRejectsInvalidWithoutHalfBatch(t *testing.T) {
	repo := setupSignalCandidateDB(t)
	ctx := context.Background()
	boardID := uint(93104)

	// Repo-level shape validation rejects before any write.
	discovery := newSignalDiscovery(boardID, "2026-08", "signal-cand-invalid")
	bad := signalCandidate(boardID, "分数越界", []string{"article:1"})
	bad.Score = 11
	_, _, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{bad})
	require.Error(t, err)

	emptyEvidence := signalCandidate(boardID, "证据为空", nil)
	emptyEvidence.EvidenceRefs = json.RawMessage(`[]`)
	_, _, err = repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{emptyEvidence})
	require.Error(t, err)

	badPeriod := newSignalDiscovery(boardID, "2026-08", "signal-cand-invalid-period")
	badPeriod.Granularity = repository.SignalGranularityYear
	_, _, err = repo.CreateSignalDiscoveryBatch(ctx, badPeriod, nil)
	require.Error(t, err)

	// DB-2: a mid-transaction failure rolls the WHOLE batch back — no batch
	// row, no candidates. ResetTestData truncates tables between tests, so a
	// prior batch is created HERE to obtain an existing candidate id; the
	// conflicting candidate then forces its INSERT to fail after the first
	// insert of the doomed batch succeeded.
	seed := newSignalDiscovery(boardID, "2026-08", "signal-cand-rollback-seed")
	seeded, _, err := repo.CreateSignalDiscoveryBatch(ctx, seed, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "先行合法批次", []string{"article:6"}),
	})
	require.NoError(t, err)
	require.Len(t, seeded, 1)
	conflict := signalCandidate(boardID, "主键冲突候选", []string{"article:8"})
	conflict.ID = seeded[0].ID
	disco := newSignalDiscovery(boardID, "2026-08", "signal-cand-rollback")
	_, _, err = repo.CreateSignalDiscoveryBatch(ctx, disco, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "会随事务回滚的候选", []string{"article:7"}),
		conflict,
	})
	require.Error(t, err, "primary-key conflict must fail the batch")
	var batchCount int64
	require.NoError(t, repo.DB().Raw(`SELECT count(*) FROM board_signal_discovery WHERE session_id = ?`, "signal-cand-rollback").Scan(&batchCount).Error)
	require.Zero(t, batchCount, "a failed batch must not leave a discovery row (no half batch)")
	var candCount int64
	require.NoError(t, repo.DB().Raw(`SELECT count(*) FROM board_signal_candidate c JOIN board_signal_discovery d ON d.id = c.discovery_id WHERE d.session_id = ?`, "signal-cand-rollback").Scan(&candCount).Error)
	require.Zero(t, candCount, "no orphan candidates may survive the rollback")
}

func TestListSignalCandidatesByPeriodCursorAndFilter(t *testing.T) {
	repo := setupSignalCandidateDB(t)
	ctx := context.Background()
	boardID := uint(93105)
	otherBoard := uint(93106)

	sessions := []string{"signal-cand-page-a", "signal-cand-page-b", "signal-cand-page-c"}
	periods := []string{"2026-07", "2026-08", "2026-08"}
	for i, period := range periods {
		disco := newSignalDiscovery(boardID, period, sessions[i])
		_, _, err := repo.CreateSignalDiscoveryBatch(ctx, disco, []*repository.BoardSignalCandidate{
			signalCandidate(boardID, "信号 "+period, []string{"article:1"}),
		})
		require.NoError(t, err)
	}
	// Another board's candidates must stay invisible.
	disco := newSignalDiscovery(otherBoard, "2026-08", "signal-cand-page-other")
	_, _, err := repo.CreateSignalDiscoveryBatch(ctx, disco, []*repository.BoardSignalCandidate{
		signalCandidate(otherBoard, "别的板块的信号", []string{"article:1"}),
	})
	require.NoError(t, err)

	list, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", 0, 20)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Greater(t, list[0].ID, list[1].ID, "newest batch first")

	// Cursor: before_id = head candidate's id drops it.
	page1, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", list[0].ID, 20)
	require.NoError(t, err)
	require.Len(t, page1, 1)
	// Cursor past the tail → empty page.
	page2, err := repo.ListSignalCandidatesByPeriod(ctx, boardID, "month", "2026-08", list[1].ID, 20)
	require.NoError(t, err)
	require.Empty(t, page2)

	// Derived status source: no report yet → nil (待研究).
	reportID, err := repo.GetLatestSignalReportIDForCandidate(ctx, list[0].ID)
	require.NoError(t, err)
	require.Nil(t, reportID)
}

func TestSignalEvidenceRefHelpers(t *testing.T) {
	refs := repository.SignalEvidenceRefs(json.RawMessage(`["article:1", "article:2"]`))
	require.Equal(t, []string{"article:1", "article:2"}, refs)
	require.Empty(t, repository.SignalEvidenceRefs(json.RawMessage(`{"not":"an array"}`)))
	require.Empty(t, repository.SignalEvidenceRefs(nil))

	// Normalization: whitespace-folded title + sorted unique refs.
	a := repository.ComputeSignalCandidateDedupeKey("  同一\u3000标题 ", []string{"b:2", "a:1", "a:1"})
	b := repository.ComputeSignalCandidateDedupeKey("同一 标题", []string{"a:1", "b:2"})
	require.Equal(t, a, b)
	require.NotEqual(t, a, repository.ComputeSignalCandidateDedupeKey("同一 标题", []string{"a:1"}))
	require.Len(t, a, 64)
}
