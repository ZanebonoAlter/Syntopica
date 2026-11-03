package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

const boardSignalMigrationVersion = "20260922_0001"

func boardSignalMigration(t *testing.T) database.Migration {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == boardSignalMigrationVersion {
			return migration
		}
	}
	t.Fatalf("migration %s not found", boardSignalMigrationVersion)
	return database.Migration{}
}

// replaySignalMigrations 把两条 board-signal 迁移的 Up 按序重放一遍（均幂等），
// 把 golden 约束形状还原回去（cleanup 用）。重放前必须先摘
// fk_board_signal_research_progress_candidate：它钉住 0001 要重建的
// uq_board_signal_candidate_id_owner，而 0001 的 Up 只自己摘
// fk_topic_enrichment_result_signal_candidate，不摘它就 2BP01。
func replaySignalMigrations(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`ALTER TABLE board_signal_research_progress
		DROP CONSTRAINT IF EXISTS fk_board_signal_research_progress_candidate`).Error)
	for _, version := range []string{"20260922_0001", "20260922_0002"} {
		var replay *database.Migration
		for _, migration := range database.ExportedPostgresMigrations() {
			if migration.Version == version {
				replay = &migration
				break
			}
		}
		require.NotNil(t, replay, "migration %s not found", version)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return replay.Up(tx) }))
	}
}

// prepareBoardSignalMigration rewinds exactly this migration's constraints and
// rows so every test can exercise its Up closure independently on the shared
// testcontainer (same pattern as result_kind_migration_test.go).
//
// 整包跑注意：字母序靠前的测试（annotation_qas）经 SetupTestDB 落好全量迁移链，
// 20260922_0002 的 fk_board_signal_research_progress_candidate 依赖本迁移要
// 重建的 uq_board_signal_candidate_id_owner，不先摘会 2BP01；cleanup 里重放
// 20260922_0002 的 Up 恢复（幂等）。
func prepareBoardSignalMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	for _, statement := range []string{
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS fk_topic_enrichment_result_signal_candidate`,
		`ALTER TABLE board_signal_research_progress DROP CONSTRAINT IF EXISTS fk_board_signal_research_progress_candidate`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_kind`,
		`ALTER TABLE topic_enrichment_result DROP CONSTRAINT IF EXISTS chk_topic_enrichment_result_parent_shape`,
		`ALTER TABLE board_signal_candidate DROP CONSTRAINT IF EXISTS fk_board_signal_candidate_discovery`,
		`ALTER TABLE board_signal_candidate DROP CONSTRAINT IF EXISTS uq_board_signal_candidate_id_owner`,
		`ALTER TABLE board_signal_discovery DROP CONSTRAINT IF EXISTS uq_board_signal_discovery_id_owner`,
		`DROP INDEX IF EXISTS idx_topic_enrichment_result_signal_board_period`,
		`DROP INDEX IF EXISTS idx_topic_enrichment_result_source_signal`,
		`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'signal-mig-%'`,
		`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 95000 AND 95999`,
		`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 95000 AND 95999`,
	} {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM topic_enrichment_result WHERE session_id LIKE 'signal-mig-%'`).Error
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 95000 AND 95999`).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 95000 AND 95999`).Error
		// 恢复 prepare 摘掉的 20260922_0002 约束（本迁移的 Up 不负责它）：
		// 重放 0001+0002 的 Up（幂等，含表存在守卫），保证共享容器对包内
		// 后续测试仍是 golden 形状。
		replaySignalMigrations(t, db)
	})
}

func runBoardSignalMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	migration := boardSignalMigration(t)
	require.Nil(t, migration.Down, "migration framework has no Down executor; migration must stay forward-only")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
}

func seedBoardSignalDiscovery(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO board_signal_discovery
		(semantic_board_id, granularity, period, analysis_mode, cutoff, input_snapshot, session_id, candidate_count)
		VALUES (?, 'month', '2026-08', 'current', now(), '{}'::jsonb, 'signal-mig-disc', 0)`, boardID).Error)
	var id uint
	require.NoError(t, db.Raw(`SELECT id FROM board_signal_discovery WHERE session_id = 'signal-mig-disc' AND semantic_board_id = ?`, boardID).Scan(&id).Error)
	require.NotZero(t, id)
	require.NoError(t, db.Exec(`INSERT INTO board_signal_candidate
		(discovery_id, semantic_board_id, granularity, period, signal, why_it_matters, research_question, evidence_refs, score, rationale)
		VALUES (?, ?, 'month', '2026-08', '库存与开工背离', '判断依据', '研究问题', '["a:1"]'::jsonb, 7, '理由')`, id, boardID).Error)
	var candidateID uint
	require.NoError(t, db.Raw(`SELECT id FROM board_signal_candidate WHERE discovery_id = ? LIMIT 1`, id).Scan(&candidateID).Error)
	require.NotZero(t, candidateID)
	return candidateID
}

func TestBoardSignalMigrationLegacyRowsKeepNullColumns(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareBoardSignalMigration(t, db)

	// Simulate a pre-signal database: drop the three columns, then store
	// legacy rows exactly as the old writer would have (DB-4).
	for _, statement := range []string{
		`ALTER TABLE topic_enrichment_result DROP COLUMN IF EXISTS granularity`,
		`ALTER TABLE topic_enrichment_result DROP COLUMN IF EXISTS period`,
		`ALTER TABLE topic_enrichment_result DROP COLUMN IF EXISTS source_signal_id`,
	} {
		require.NoError(t, db.Exec(statement).Error, statement)
	}
	topicSectors := `{"form":"event_chain","legacy":[1,2]}`
	boardSectors := `{"scope":"board","thesis":"旧论文原样"}`
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, sectors, session_id)
		VALUES
		(95101, NULL, 'topic', 'topic_analysis', ?::jsonb, 'signal-mig-legacy-topic'),
		(NULL, 95102, 'board', 'board_brief', ?::jsonb, 'signal-mig-legacy-board')`, topicSectors, boardSectors).Error)

	runBoardSignalMigration(t, db)

	// Columns exist again and legacy rows keep NULL (旧行新列全 NULL 不回填).
	var gran, period *string
	var source *uint
	var storedTopic, storedBoard string
	require.NoError(t, db.Raw(`SELECT granularity, period, source_signal_id, sectors::text FROM topic_enrichment_result WHERE session_id = 'signal-mig-legacy-topic'`).Row().Scan(&gran, &period, &source, &storedTopic))
	require.Nil(t, gran)
	require.Nil(t, period)
	require.Nil(t, source)
	require.JSONEq(t, topicSectors, storedTopic, "sectors payload must stay byte-equivalent")
	require.NoError(t, db.Raw(`SELECT granularity, period, source_signal_id, sectors::text FROM topic_enrichment_result WHERE session_id = 'signal-mig-legacy-board'`).Row().Scan(&gran, &period, &source, &storedBoard))
	require.Nil(t, gran)
	require.Nil(t, period)
	require.Nil(t, source)
	require.JSONEq(t, boardSectors, storedBoard)

	// Legacy shapes must still be insertable after the CHECK re-ADD.
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(persistent_topic_id, semantic_board_id, analysis_scope, result_kind, sectors, session_id)
		VALUES (95103, NULL, 'topic', 'topic_analysis', '{}'::jsonb, 'signal-mig-post-topic')`).Error)
}

func TestBoardSignalMigrationShapeAndFKRejectIllegalRows(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareBoardSignalMigration(t, db)
	runBoardSignalMigration(t, db)

	boardID := uint(95201)
	candidateID := seedBoardSignalDiscovery(t, db, boardID)

	// Legal signal_report row passes.
	require.NoError(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-ok', 'month', '2026-08', ?)`,
		boardID, candidateID).Error)

	// Unknown kind → kind CHECK (re-ADDed with signal_report included).
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id)
		VALUES (?, 'board', 'signal_weird', '{}'::jsonb, 'signal-mig-bad-kind')`, boardID).Error)

	// signal_report without granularity → shape CHECK (NULL must fail).
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-no-gran', '2026-08', ?)`,
		boardID, candidateID).Error)

	// signal_report with an impossible month → shape CHECK.
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-bad-month', 'month', '2026-13', ?)`,
		boardID, candidateID).Error)

	// A legacy kind carrying signal columns → shape CHECK.
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period)
		VALUES (?, 'board', 'board_brief', '{}'::jsonb, 'signal-mig-brief-gran', 'month', '2026-08')`, boardID).Error)

	// Result period disagreeing with its candidate → composite FK.
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-bad-period', 'month', '2026-07', ?)`,
		boardID, candidateID).Error)

	// Result pointing at a candidate of another board → composite FK.
	otherCandidate := seedBoardSignalDiscovery(t, db, boardID+1)
	require.Error(t, db.Exec(`INSERT INTO topic_enrichment_result
		(semantic_board_id, analysis_scope, result_kind, sectors, session_id, granularity, period, source_signal_id)
		VALUES (?, 'board', 'signal_report', '{}'::jsonb, 'signal-mig-bad-board', 'month', '2026-08', ?)`,
		boardID, otherCandidate).Error)
}

func TestBoardSignalMigrationIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	prepareBoardSignalMigration(t, db)
	runBoardSignalMigration(t, db)
	// A second full run (restart replay) must be a no-op, not an error.
	runBoardSignalMigration(t, db)

	// Constraints and indexes are in place exactly once.
	var checkCount int
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_constraint
		WHERE conname IN ('chk_topic_enrichment_result_kind', 'chk_topic_enrichment_result_parent_shape',
			'uq_board_signal_discovery_id_owner', 'uq_board_signal_candidate_id_owner',
			'fk_board_signal_candidate_discovery', 'fk_topic_enrichment_result_signal_candidate')`).Scan(&checkCount).Error)
	require.Equal(t, 6, checkCount)
	var indexCount int
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes
		WHERE indexname IN ('idx_board_signal_discovery_board_period', 'idx_board_signal_candidate_board_period',
			'idx_board_signal_candidate_discovery', 'idx_topic_enrichment_result_signal_board_period',
			'idx_topic_enrichment_result_source_signal')`).Scan(&indexCount).Error)
	require.Equal(t, 5, indexCount)
}

const signalDispositionMigrationVersion = "20261015_0001"

func signalDispositionMigration(t *testing.T) database.Migration {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == signalDispositionMigrationVersion {
			return migration
		}
	}
	t.Fatalf("migration %s not found", signalDispositionMigrationVersion)
	return database.Migration{}
}

func runSignalDispositionMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	migration := signalDispositionMigration(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
}

// TestSignalDispositionMigrationFKAndIdempotent asserts the disposition
// migration shape (redesign-enrichment-research-workbench 2.1): the marker
// table itself is AutoMigrate's job; the migration owns only the candidate
// FK — dangling candidate ids are rejected, repeated Up runs are a no-op,
// and candidate/report rows are never touched (no backfill, no column).
// TestQuestionBriefMigrationLegacyNullNoBackfill 验证 20261019_0001
// (add-question-driven-news-research D1)：research_brief 列存在、旧行 NULL
// 不回填、迁移幂等、新行可写合法 brief。验证只在隔离 testcontainer（红线：
// 禁真库试 DDL）。
func TestQuestionBriefMigrationLegacyNullNoBackfill(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	boardID := uint(95510)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id = ?`, boardID).Error)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id = ?`, boardID).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id = ?`, boardID).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id = ?`, boardID).Error
	})

	// 模拟旧行：先摘列，写入旧形状候选，再跑迁移。
	require.NoError(t, db.Exec(`ALTER TABLE board_signal_candidate DROP COLUMN IF EXISTS research_brief`).Error)
	legacyCandidate := seedBoardSignalDiscovery(t, db, boardID)

	var migration *database.Migration
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261019_0001" {
			mm := m
			migration = &mm
			break
		}
	}
	require.NotNil(t, migration, "migration 20261019_0001 not found")
	require.Nil(t, migration.Down, "forward-only")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
	// 幂等：二次 Up 不报错。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))

	// 旧行 research_brief 仍 NULL（不猜填、不回填）。
	var brief *string
	require.NoError(t, db.Raw(`SELECT research_brief::text FROM board_signal_candidate WHERE id = ?`, legacyCandidate).Row().Scan(&brief))
	require.Nil(t, brief, "legacy candidate must keep NULL research_brief (never backfilled)")

	// 新行可写合法 brief（question-v1 形状），非法 owner 仍被复合 FK 拒绝。
	require.NoError(t, db.Exec(`UPDATE board_signal_candidate SET research_brief = ?::jsonb WHERE id = ?`,
		`{"unknown":"口径未确认","verification":"先读原文","source_ids":["n1"]}`, legacyCandidate).Error)
	require.Error(t, db.Exec(`INSERT INTO board_signal_candidate
		(discovery_id, semantic_board_id, granularity, period, signal, why_it_matters, research_question, evidence_refs, score, rationale)
		VALUES (?, 99999, 'month', '2026-08', 'x', 'y', 'z', '[]'::jsonb, 7, 'r')`, legacyCandidate+1).Error,
		"illegal owner/period must still be rejected by the composite FK")
}

func TestSignalDispositionMigrationFKAndIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	// Rewind to the pre-migration shape so this Up closure is exercised on
	// the shared testcontainer (the golden chain may already have applied it).
	require.NoError(t, db.Exec(`ALTER TABLE board_signal_disposition
		DROP CONSTRAINT IF EXISTS fk_board_signal_disposition_candidate`).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM board_signal_disposition WHERE candidate_id IN
			(SELECT id FROM board_signal_candidate WHERE semantic_board_id BETWEEN 95000 AND 95999)`).Error
		_ = db.Exec(`DELETE FROM board_signal_candidate WHERE semantic_board_id BETWEEN 95000 AND 95999`).Error
		_ = db.Exec(`DELETE FROM board_signal_discovery WHERE semantic_board_id BETWEEN 95000 AND 95999`).Error
	})

	runSignalDispositionMigration(t, db)
	// Restart replay: a second run must be a no-op, not an error.
	runSignalDispositionMigration(t, db)

	var fkCount int
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_constraint WHERE conname = 'fk_board_signal_disposition_candidate'`).Scan(&fkCount).Error)
	require.Equal(t, 1, fkCount)

	// FK: a marker may only point at an existing candidate.
	require.Error(t, db.Exec(`INSERT INTO board_signal_disposition (candidate_id, ignored_at)
		VALUES (999999999, now())`).Error, "dangling candidate_id must be rejected")

	// CASCADE: deleting the candidate removes its marker (marker is
	// meaningless without its candidate).
	candidateID := seedBoardSignalDiscovery(t, db, 95301)
	require.NoError(t, db.Exec(`INSERT INTO board_signal_disposition (candidate_id, ignored_at)
		VALUES (?, now())`, candidateID).Error)
	require.NoError(t, db.Exec(`DELETE FROM board_signal_candidate WHERE id = ?`, candidateID).Error)
	var markerCount int
	require.NoError(t, db.Raw(`SELECT count(*) FROM board_signal_disposition WHERE candidate_id = ?`, candidateID).Scan(&markerCount).Error)
	require.Equal(t, 0, markerCount)
}
