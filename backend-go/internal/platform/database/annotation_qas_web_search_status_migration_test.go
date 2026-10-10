package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Side-effect: register margin note models so AutoMigrate can create
	// annotation_qas for the migration target.
	_ "syntopica-backend/internal/topicgraph/repository"
)

// findMarginNotesWebSearchStatusMigration locates migration 20261007_0001's Up
// closure (add-searxng-health-indicator design D5: web_search_status text NULL).
func findMarginNotesWebSearchStatusMigration() func(*gorm.DB) error {
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261007_0001" {
			return m.Up
		}
	}
	return nil
}

// TestMarginNotesWebSearchStatusMigration exercises migration 20261007_0001
// against a testcontainer PG: nullable column added, historical rows keep NULL
// (no backfill — NULL 是历史数据语义), and idempotent on re-run.
func TestMarginNotesWebSearchStatusMigration(t *testing.T) {
	db := testutil.SetupTestDB(t)

	up := findMarginNotesWebSearchStatusMigration()
	require.NotNil(t, up, "migration 20261007_0001 must be registered")

	// annotation_qas must exist (AutoMigrate via model registration) — create
	// one historical row first so the no-backfill path is exercised. FK 要求
	// 先有父行 report_annotations（对齐 cited_web_sources 迁移测试范式）。
	require.NoError(t, db.Exec(`INSERT INTO report_annotations (report_id, section_id, quoted_text, created_at)
		VALUES (1, 0, '历史划词', now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO annotation_qas (annotation_id, question, answer, created_at)
		VALUES (1, '历史问题', '历史回答', now())`).Error)

	require.NoError(t, up(db), "first run must succeed")

	// Column exists and is nullable (旧行 NULL 保留，不回填).
	var nullCnt int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM annotation_qas WHERE web_search_status IS NULL`).Scan(&nullCnt).Error)
	require.EqualValues(t, 1, nullCnt, "historical row keeps NULL web_search_status (no backfill)")

	// 三态写入合法（disabled/ok/empty 都是普通 text，约束在应用层枚举）。
	require.NoError(t, db.Exec(`UPDATE annotation_qas SET web_search_status = 'empty' WHERE annotation_id = 1`).Error)
	var got string
	require.NoError(t, db.Raw(`SELECT web_search_status FROM annotation_qas WHERE annotation_id = 1`).Scan(&got).Error)
	require.Equal(t, "empty", got)

	// Idempotent: second run must succeed.
	require.NoError(t, up(db), "re-run must be idempotent")
}
