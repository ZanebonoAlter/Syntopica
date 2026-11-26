package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/testutil"
)

// 旧研究历史只读列表（redesign-enrichment-research-workbench D2 GET /history，
// spec「旧研究历史只读访问」）：按 board/topic 可验证归属汇总旧 result，id
// DESC 排他游标分页；含已归档话题、跨板块不泄露、signal_report 不混入。
// 隔离 testcontainer Postgres（testutil）——不连 5432 开发库。板块号段
// 96600-96999 为本文件专用（signal 系用 94000-94999，962xx 被 legacy-read-
// compat handler 测试占用）。

const (
	legacyHistoryBoardFloor = 96600
	legacyHistoryBoardCeil  = 96999
)

func setupLegacyHistoryDB(t *testing.T) *repository.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t) // golden schema：AutoMigrate + 全部版本化迁移
	cleanup := func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE semantic_board_id BETWEEN ? AND ?
			OR persistent_topic_id IN (SELECT id FROM board_persistent_topics WHERE semantic_board_id BETWEEN ? AND ?)`,
			legacyHistoryBoardFloor, legacyHistoryBoardCeil, legacyHistoryBoardFloor, legacyHistoryBoardCeil).Error
		_ = db.Exec(`DELETE FROM board_signal_disposition WHERE candidate_id IN
			(SELECT id FROM board_signal_candidate WHERE semantic_board_id BETWEEN ? AND ?)`,
			legacyHistoryBoardFloor, legacyHistoryBoardCeil).Error
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN ? AND ?`,
			legacyHistoryBoardFloor, legacyHistoryBoardCeil).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN ? AND ?`,
			legacyHistoryBoardFloor, legacyHistoryBoardCeil).Error
		_ = db.Exec(`DELETE FROM board_persistent_topics WHERE semantic_board_id BETWEEN ? AND ?`,
			legacyHistoryBoardFloor, legacyHistoryBoardCeil).Error
	}
	cleanup()
	t.Cleanup(cleanup)
	return repository.NewRepository(db)
}

// seedLegacyTopicRow inserts one persistent topic via raw SQL (embedding 等
// 列走 NULL/默认值), returns the assigned id.
func seedLegacyTopicRow(t *testing.T, repo *repository.Repository, boardID uint, label, status string) uint {
	t.Helper()
	var id uint
	err := repo.DB().Raw(`INSERT INTO board_persistent_topics
		(semantic_board_id, label, status, source, first_seen_date, last_seen_date)
		VALUES (?, ?, ?, 'auto', CURRENT_DATE, CURRENT_DATE) RETURNING id`,
		boardID, label, status).Scan(&id).Error
	require.NoError(t, err)
	require.NotZero(t, id)
	return id
}

// seedLegacyBoardRow inserts one board-scope result of the given kind
// (board_brief / legacy_board_analysis need no parent).
func seedLegacyBoardRow(t *testing.T, repo *repository.Repository, ctx context.Context, boardID uint, kind string) *repository.TopicEnrichmentResult {
	t.Helper()
	row := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      kind,
		Sectors:         []byte(`{"legacy":"fixture"}`),
		SessionID:       "legacy_history_seed",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, row))
	return row
}

// seedLegacyInvestigationRow inserts one board_investigation child of a brief
// (shape validation requires a same-board board_brief parent + question key).
func seedLegacyInvestigationRow(t *testing.T, repo *repository.Repository, ctx context.Context, boardID, parentBriefID uint) *repository.TopicEnrichmentResult {
	t.Helper()
	qk := repository.ComputeQuestionKey("后续节奏如何？")
	row := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindBoardInvestigation,
		ParentResultID:  &parentBriefID,
		QuestionKey:     &qk,
		Sectors:         []byte(`{"legacy":"investigation"}`),
		SessionID:       "legacy_history_seed",
	}
	require.NoError(t, repo.CreateBoardInvestigationResult(ctx, row))
	return row
}

// seedLegacyTopicAnalysisRow inserts one topic-scope analysis for the topic.
func seedLegacyTopicAnalysisRow(t *testing.T, repo *repository.Repository, ctx context.Context, topicID uint) *repository.TopicEnrichmentResult {
	t.Helper()
	row := &repository.TopicEnrichmentResult{
		PersistentTopicID: repository.TopicIDPtr(topicID),
		AnalysisScope:     "topic",
		ResultKind:        repository.ResultKindTopicAnalysis,
		Sectors:           []byte(`{"legacy":"topic"}`),
		SessionID:         "legacy_history_seed",
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, row))
	return row
}

// seedLegacySignalReportRow inserts one signal_report the same way the
// production write chain does (candidate FK backstop satisfied) — the list
// must never surface it.
func seedLegacySignalReportRow(t *testing.T, repo *repository.Repository, ctx context.Context, boardID uint, period string) *repository.TopicEnrichmentResult {
	t.Helper()
	discovery := newSignalDiscovery(boardID, period, "legacy-history-signal-seed")
	persisted, _, err := repo.CreateSignalDiscoveryBatch(ctx, discovery, []*repository.BoardSignalCandidate{
		signalCandidate(boardID, "历史混入防御用候选", []string{"article:1"}),
	})
	require.NoError(t, err)
	require.Len(t, persisted, 1)
	candidateID := persisted[0].ID
	granularity := repository.SignalGranularityMonth
	row := &repository.TopicEnrichmentResult{
		SemanticBoardID: repository.BoardIDPtr(boardID),
		AnalysisScope:   "board",
		ResultKind:      repository.ResultKindSignalReport,
		Sectors:         []byte(`{"schema_version":2}`),
		Granularity:     &granularity,
		Period:          &period,
		SourceSignalID:  &candidateID,
	}
	require.NoError(t, repo.CreateTopicEnrichmentResult(ctx, row))
	return row
}

func legacyHistoryIDs(rows []repository.LegacyHistoryRow) []uint {
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

// TestLegacyHistoryPaginationCursorAndLimit locks the shared pagination
// contract: exclusive before_id cursor, id DESC order, limit default 20 /
// clamp 100 (与 ListSignalReportResults 同规范化).
func TestLegacyHistoryPaginationCursorAndLimit(t *testing.T) {
	repo := setupLegacyHistoryDB(t)
	ctx := context.Background()
	boardID := uint(96601)
	const total = 23
	seeded := make(map[uint]bool, total)
	for i := 0; i < total; i++ {
		row := seedLegacyBoardRow(t, repo, ctx, boardID, repository.ResultKindBoardBrief)
		seeded[row.ID] = true
	}

	// limit 缺省（0）与超上限（500）都规范化为 20。
	page1, err := repo.ListLegacyResults(ctx, boardID, "", 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page1, 20)
	page1b, err := repo.ListLegacyResults(ctx, boardID, "", 0, 0, 500)
	require.NoError(t, err)
	require.Len(t, page1b, 20)
	require.Equal(t, legacyHistoryIDs(page1), legacyHistoryIDs(page1b))
	// id 严格倒序。
	for i := 1; i < len(page1); i++ {
		require.Greater(t, page1[i-1].ID, page1[i].ID)
	}

	// 排他游标：before_id=第 20 行 id → 只回第 21-23 行，无重叠。
	cursor := page1[19].ID
	page2, err := repo.ListLegacyResults(ctx, boardID, "", 0, cursor, 20)
	require.NoError(t, err)
	require.Len(t, page2, 3)
	for _, r := range page2 {
		require.Less(t, r.ID, cursor, "before_id is exclusive")
		require.False(t, containsID(page1, r.ID), "cursor page must not repeat page 1 rows")
	}

	// 全量走查：游标翻到底，恰好收齐 23 行不重不漏。
	seen := make([]uint, 0, total)
	before := uint(0)
	for {
		page, err := repo.ListLegacyResults(ctx, boardID, "", 0, before, 10)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		require.LessOrEqual(t, len(page), 10)
		for _, r := range page {
			seen = append(seen, r.ID)
		}
		before = page[len(page)-1].ID
	}
	require.Len(t, seen, total)
	require.Len(t, uniqueIDs(seen), total)
}

func containsID(rows []repository.LegacyHistoryRow, id uint) bool {
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}

func uniqueIDs(ids []uint) []uint {
	seen := make(map[uint]bool, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// TestLegacyHistoryArchivedTopicIncluded locks spec「旧研究历史只读访问」:
// 已归档话题的结果仍在列表（归属按 topic→board 验证，不过滤 status）。
func TestLegacyHistoryArchivedTopicIncluded(t *testing.T) {
	repo := setupLegacyHistoryDB(t)
	ctx := context.Background()
	boardID := uint(96602)
	topicID := seedLegacyTopicRow(t, repo, boardID, "原油", "active")
	row := seedLegacyTopicAnalysisRow(t, repo, ctx, topicID)

	rows, err := repo.ListLegacyResults(ctx, boardID, "", 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, row.ID, rows[0].ID)
	require.Equal(t, repository.ResultKindTopicAnalysis, rows[0].ResultKind)
	require.NotNil(t, rows[0].PersistentTopicID)
	require.Equal(t, topicID, *rows[0].PersistentTopicID)
	require.Nil(t, rows[0].SemanticBoardID, "topic rows own no board id")

	// 归档后仍在（只读历史不受生命周期状态影响）。
	require.NoError(t, repo.DB().Exec(`UPDATE board_persistent_topics SET status = 'archived' WHERE id = ?`, topicID).Error)
	rows, err = repo.ListLegacyResults(ctx, boardID, repository.ResultKindTopicAnalysis, 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, row.ID, rows[0].ID)
}

// TestLegacyHistoryOwnershipAndSignalReportExcluded locks the ownership union
// and the never-mix guard: another board's rows never appear; a real
// signal_report row is not mixed into the legacy collection; the defensive
// kind=signal_report request is rejected.
func TestLegacyHistoryOwnershipAndSignalReportExcluded(t *testing.T) {
	repo := setupLegacyHistoryDB(t)
	ctx := context.Background()
	boardA := uint(96603)
	boardB := uint(96604)

	briefA := seedLegacyBoardRow(t, repo, ctx, boardA, repository.ResultKindBoardBrief)
	seedLegacyInvestigationRow(t, repo, ctx, boardA, briefA.ID)
	seedLegacyBoardRow(t, repo, ctx, boardA, repository.ResultKindLegacyBoardAnalysis)
	topicA := seedLegacyTopicRow(t, repo, boardA, "航运", "active")
	seedLegacyTopicAnalysisRow(t, repo, ctx, topicA)
	signal := seedLegacySignalReportRow(t, repo, ctx, boardA, "2026-08")

	briefB := seedLegacyBoardRow(t, repo, ctx, boardB, repository.ResultKindBoardBrief)
	topicB := seedLegacyTopicRow(t, repo, boardB, "他板话题", "active")
	topicRowB := seedLegacyTopicAnalysisRow(t, repo, ctx, topicB)

	rows, err := repo.ListLegacyResults(ctx, boardA, "", 0, 0, 20)
	require.NoError(t, err)
	// 3 种 board 旧 kind + 1 条 topic_analysis；signal_report 与他板行不出现。
	kinds := map[string]int{}
	for _, r := range rows {
		kinds[r.ResultKind]++
		require.NotEqual(t, repository.ResultKindSignalReport, r.ResultKind)
		require.NotEqual(t, briefB.ID, r.ID)
		require.NotEqual(t, topicRowB.ID, r.ID)
	}
	require.Len(t, rows, 4)
	require.Equal(t, 1, kinds[repository.ResultKindBoardBrief])
	require.Equal(t, 1, kinds[repository.ResultKindBoardInvestigation])
	require.Equal(t, 1, kinds[repository.ResultKindLegacyBoardAnalysis])
	require.Equal(t, 1, kinds[repository.ResultKindTopicAnalysis])

	// signal_report 行确实存在（排除的是列表视角，不是没造数）。
	got, err := repo.GetTopicEnrichmentResultByID(ctx, signal.ID)
	require.NoError(t, err)
	require.Equal(t, repository.ResultKindSignalReport, got.ResultKind)

	// B 板块视角：只看到自己的 2 行。
	rowsB, err := repo.ListLegacyResults(ctx, boardB, "", 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, rowsB, 2)

	// 防御层：signal_report 不是合法历史 kind。
	_, err = repo.ListLegacyResults(ctx, boardA, repository.ResultKindSignalReport, 0, 0, 20)
	require.ErrorContains(t, err, "invalid legacy history kind")
}

// TestLegacyHistoryKindAndTopicFilters locks the optional filters: kind narrows
// to one kind; topic_id narrows to one topic's rows; both compose; an empty
// board yields an empty list, not an error.
func TestLegacyHistoryKindAndTopicFilters(t *testing.T) {
	repo := setupLegacyHistoryDB(t)
	ctx := context.Background()
	boardID := uint(96605)
	brief := seedLegacyBoardRow(t, repo, ctx, boardID, repository.ResultKindBoardBrief)
	topic1 := seedLegacyTopicRow(t, repo, boardID, "话题一", "active")
	topic2 := seedLegacyTopicRow(t, repo, boardID, "话题二", "active")
	t1a := seedLegacyTopicAnalysisRow(t, repo, ctx, topic1)
	t1b := seedLegacyTopicAnalysisRow(t, repo, ctx, topic1)
	t2a := seedLegacyTopicAnalysisRow(t, repo, ctx, topic2)

	briefs, err := repo.ListLegacyResults(ctx, boardID, repository.ResultKindBoardBrief, 0, 0, 20)
	require.NoError(t, err)
	require.Len(t, briefs, 1)
	require.Equal(t, brief.ID, briefs[0].ID)

	topicRows, err := repo.ListLegacyResults(ctx, boardID, repository.ResultKindTopicAnalysis, 0, 0, 20)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{t1a.ID, t1b.ID, t2a.ID}, legacyHistoryIDs(topicRows))

	t1Rows, err := repo.ListLegacyResults(ctx, boardID, "", topic1, 0, 20)
	require.NoError(t, err)
	require.ElementsMatch(t, []uint{t1a.ID, t1b.ID}, legacyHistoryIDs(t1Rows))
	for _, r := range t1Rows {
		require.NotNil(t, r.PersistentTopicID)
		require.Equal(t, topic1, *r.PersistentTopicID)
	}

	combined, err := repo.ListLegacyResults(ctx, boardID, repository.ResultKindTopicAnalysis, topic2, 0, 20)
	require.NoError(t, err)
	require.Equal(t, []uint{t2a.ID}, legacyHistoryIDs(combined))

	// 空板块：空列表不报错。
	empty, err := repo.ListLegacyResults(ctx, 96606, "", 0, 0, 20)
	require.NoError(t, err)
	require.Empty(t, empty)
}
