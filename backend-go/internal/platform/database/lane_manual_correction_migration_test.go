package database_test

// lane-manual-correction-loop 的迁移验收（tasks 1.1-1.3）：
//
//	20261113_0001  board_persistent_topics.centroid_pending_recompute
//	20261113_0002  topic_anchor_overrides 表
//	20261113_0003  daily_report_sections 归属变更留痕三列
//	20261113_0004  section_assignment_overrides 表（review R-2 的 append-only 归属留痕）
//
// 四步都是加法（新列 / 新表）：幂等可重入、无 DROP / TRUNCATE / 既有列改类型、
// 不回填历史数据语义。

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Blank import registers the topicgraph models (board_persistent_topics /
	// daily_report_sections) for this test binary's AutoMigrate — the tables
	// these migrations touch are owned by the topicgraph domain, and
	// platform/database cannot reference that package statically (import
	// cycle). Same pattern as constraints_test.go.
	_ "syntopica-backend/internal/topicgraph/repository"
)

const (
	laneCorrectionPendingMigrationVersion   = "20261113_0001"
	laneCorrectionOverridesMigrationVersion = "20261113_0002"
	laneCorrectionTraceMigrationVersion     = "20261113_0003"
	laneCorrectionTrailMigrationVersion     = "20261113_0004"
)

// runLaneCorrectionMigration runs one migration's Up closure (mirrors the
// production in-transaction path).
func runLaneCorrectionMigration(t *testing.T, db *gorm.DB, version string) {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == version {
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
			return
		}
	}
	t.Fatalf("migration %s not found", version)
}

// rewindLaneCorrectionShape drops the three schema items back to their
// pre-migration shape so the ADD COLUMN / CREATE TABLE branches are really
// exercised (AutoMigrate may already have created the columns).
func rewindLaneCorrectionShape(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, stmt := range []string{
		`ALTER TABLE board_persistent_topics DROP COLUMN IF EXISTS centroid_pending_recompute`,
		`DROP TABLE IF EXISTS topic_anchor_overrides`,
		`ALTER TABLE daily_report_sections DROP COLUMN IF EXISTS manual_assignment_at`,
		`ALTER TABLE daily_report_sections DROP COLUMN IF EXISTS manual_assignment_from_topic_id`,
		`ALTER TABLE daily_report_sections DROP COLUMN IF EXISTS manual_assignment_action`,
		`DROP TABLE IF EXISTS section_assignment_overrides`,
	} {
		require.NoError(t, db.Exec(stmt).Error, stmt)
	}
}

func TestLaneManualCorrectionMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	rewindLaneCorrectionShape(t, db)

	// 迁移前的形状：列与表都不存在。
	require.EqualValues(t, 0, countColumns(t, db, "board_persistent_topics", "centroid_pending_recompute"))
	require.False(t, tableExistsRaw(t, db, "topic_anchor_overrides"), "topic_anchor_overrides must not exist before the migration")
	require.EqualValues(t, 0, countColumns(t, db, "daily_report_sections", "manual_assignment_at"))
	require.False(t, tableExistsRaw(t, db, "section_assignment_overrides"), "section_assignment_overrides must not exist before the migration")

	// ── 历史行：迁移前存在的行必须被安全带过（不被删、不被改语义）──
	require.NoError(t, db.Exec(`
		INSERT INTO board_persistent_topics
			(semantic_board_id, label, embedding, status, source, first_seen_date, last_seen_date, hit_count, consecutive_hits)
		VALUES (987701, 'legacy lane before migration', '[0,0,1]', 'active', 'auto', CURRENT_DATE, CURRENT_DATE, 3, 2)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO daily_report_sections (report_id, cluster_label, embedding, persistent_topic_id)
		VALUES (987701, 'legacy section before migration', '[1,0,0]', NULL)
	`).Error)

	// ── 执行三条迁移 ──
	runLaneCorrectionMigration(t, db, laneCorrectionPendingMigrationVersion)
	runLaneCorrectionMigration(t, db, laneCorrectionOverridesMigrationVersion)
	runLaneCorrectionMigration(t, db, laneCorrectionTraceMigrationVersion)
	runLaneCorrectionMigration(t, db, laneCorrectionTrailMigrationVersion)

	// ── 1.1 centroid_pending_recompute ──
	require.EqualValues(t, 1, countColumns(t, db, "board_persistent_topics", "centroid_pending_recompute"))
	dataType, nullable := columnMeta(t, db, "board_persistent_topics", "centroid_pending_recompute")
	require.Equal(t, "boolean", dataType)
	require.Equal(t, "NO", nullable, "标记列必须 NOT NULL")
	var columnDefault string
	require.NoError(t, db.Raw(`SELECT column_default FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'board_persistent_topics' AND column_name = 'centroid_pending_recompute'`).Row().Scan(&columnDefault))
	require.Contains(t, columnDefault, "false", "默认值必须是 false（历史行不触发重算）")

	// 历史行的标记为 false —— 不触发任何重算。
	var pendingCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM board_persistent_topics WHERE centroid_pending_recompute`).Scan(&pendingCount).Error)
	require.EqualValues(t, 0, pendingCount, "历史行的待重算标记必须为 false")
	var legacyHits int
	require.NoError(t, db.Raw(`SELECT hit_count FROM board_persistent_topics WHERE label = 'legacy lane before migration'`).Row().Scan(&legacyHits))
	require.Equal(t, 3, legacyHits, "迁移不得改写既有行的其它列")

	// ── 1.2 topic_anchor_overrides ──
	require.True(t, tableExistsRaw(t, db, "topic_anchor_overrides"), "topic_anchor_overrides must exist after the migration")
	for _, col := range []string{"id", "topic_id", "old_embedding", "new_embedding", "source", "input_summary", "operator", "created_at"} {
		require.EqualValues(t, 1, countColumns(t, db, "topic_anchor_overrides", col), "column %s must exist", col)
	}
	require.True(t, indexExistsRaw(t, db, "idx_topic_anchor_overrides_topic"), "topic_id index must exist")
	var indexDef string
	require.NoError(t, db.Raw(`SELECT pg_get_indexdef('idx_topic_anchor_overrides_topic'::regclass)`).Row().Scan(&indexDef))
	require.Contains(t, indexDef, "(topic_id)")
	// 留痕表是空的：迁移不回填任何历史记录。
	var overrideCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM topic_anchor_overrides`).Scan(&overrideCount).Error)
	require.Zero(t, overrideCount)

	// ── 1.3 daily_report_sections 留痕三列 ──
	for _, col := range []string{"manual_assignment_at", "manual_assignment_from_topic_id", "manual_assignment_action"} {
		require.EqualValues(t, 1, countColumns(t, db, "daily_report_sections", col), "column %s must exist", col)
		_, nullable := columnMeta(t, db, "daily_report_sections", col)
		require.Equal(t, "YES", nullable, "%s 必须可空（历史行不回填）", col)
	}
	var traceAt *string
	var traceTopic *int64
	var traceAction *string
	require.NoError(t, db.Raw(`SELECT manual_assignment_at::text, manual_assignment_from_topic_id, manual_assignment_action
		FROM daily_report_sections WHERE cluster_label = 'legacy section before migration'`).Row().Scan(&traceAt, &traceTopic, &traceAction))
	require.Nil(t, traceAt, "历史行的留痕时间必须为 NULL")
	require.Nil(t, traceTopic, "历史行的留痕话题 id 必须为 NULL")
	require.Nil(t, traceAction, "历史行的留痕动作必须为 NULL")

	// ── 1.4 section_assignment_overrides（append-only 归属留痕表）──
	require.True(t, tableExistsRaw(t, db, "section_assignment_overrides"), "section_assignment_overrides must exist after the migration")
	for _, col := range []string{"id", "section_id", "topic_id_from", "topic_id_to", "action", "operator", "created_at"} {
		require.EqualValues(t, 1, countColumns(t, db, "section_assignment_overrides", col), "column %s must exist", col)
	}
	require.True(t, indexExistsRaw(t, db, "idx_section_assignment_overrides_section"), "section_id index must exist")
	var trailIndexDef string
	require.NoError(t, db.Raw(`SELECT pg_get_indexdef('idx_section_assignment_overrides_section'::regclass)`).Row().Scan(&trailIndexDef))
	require.Contains(t, trailIndexDef, "(section_id)")
	// 留痕表不建外键：审计不随硬删消失。
	var trailFKCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.table_constraints
		WHERE table_schema = 'public' AND table_name = 'section_assignment_overrides' AND constraint_type = 'FOREIGN KEY'`).Scan(&trailFKCount).Error)
	require.Zero(t, trailFKCount, "留痕表不设外键")
	// 迁移不回填任何历史记录。
	var trailCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM section_assignment_overrides`).Scan(&trailCount).Error)
	require.Zero(t, trailCount)

	// ── 幂等重放：三条迁移再跑一遍不报错、结构不变 ──
	runLaneCorrectionMigration(t, db, laneCorrectionPendingMigrationVersion)
	runLaneCorrectionMigration(t, db, laneCorrectionOverridesMigrationVersion)
	runLaneCorrectionMigration(t, db, laneCorrectionTraceMigrationVersion)
	runLaneCorrectionMigration(t, db, laneCorrectionTrailMigrationVersion)
	require.EqualValues(t, 1, countColumns(t, db, "board_persistent_topics", "centroid_pending_recompute"))
	require.True(t, tableExistsRaw(t, db, "topic_anchor_overrides"))
	require.True(t, tableExistsRaw(t, db, "section_assignment_overrides"))
	var indexCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_topic_anchor_overrides_topic'`).Scan(&indexCount).Error)
	require.EqualValues(t, 1, indexCount)

	// cleanup：清种子行，别给共享容器留漂移。
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM daily_report_sections WHERE cluster_label = 'legacy section before migration'`).Error
		_ = db.Exec(`DELETE FROM board_persistent_topics WHERE label = 'legacy lane before migration'`).Error
	})
}

// countColumns returns how many columns the given table has with that name.
func countColumns(t *testing.T, db *gorm.DB, table, column string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`, table, column).Scan(&count).Error)
	return count
}
