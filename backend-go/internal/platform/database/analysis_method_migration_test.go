package database_test

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Register data-enrichment models for AutoMigrate.
	_ "syntopica-backend/internal/dataenrichment/repository"
)

const analysisMethodMigrationVersion = "20260828_0002"

func TestAnalysisMethodLegacyCopyMigrationPreservesBytesAndIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	const prefix = "analysis-method-migration-test-"
	t.Cleanup(func() {
		_ = db.Unscoped().Exec("DELETE FROM analysis_methods WHERE name LIKE ?", prefix+"%").Error
		_ = db.Unscoped().Exec("DELETE FROM reference_roles WHERE name LIKE ?", prefix+"%").Error
	})
	require.NoError(t, db.Unscoped().Exec("DELETE FROM analysis_methods WHERE name LIKE ?", prefix+"%").Error)
	require.NoError(t, db.Unscoped().Exec("DELETE FROM reference_roles WHERE name LIKE ?", prefix+"%").Error)

	legacyContent := "第一行\r\n第二行\n原文末尾空格  \n"
	require.NoError(t, db.Exec(`INSERT INTO reference_roles (name,title,content,enabled,created_at,updated_at)
		VALUES (?, '旧画像', ?, true, now(), now())`, prefix+"copy", legacyContent).Error)
	// A user-owned method with the same name must win forever; migration must not overwrite it.
	require.NoError(t, db.Exec(`INSERT INTO analysis_methods
		(name,title,summary,selection_meta,content,enabled,legacy,created_at,updated_at)
		VALUES (?, '用户标题', '用户摘要', '{}'::jsonb, '用户已编辑正文', true, false, now(), now())`, prefix+"conflict").Error)
	require.NoError(t, db.Exec(`INSERT INTO reference_roles (name,title,content,enabled,created_at,updated_at)
		VALUES (?, '冲突旧画像', '不应覆盖', true, now(), now())`, prefix+"conflict").Error)

	var migration *database.Migration
	for i := range database.ExportedPostgresMigrations() {
		m := database.ExportedPostgresMigrations()[i]
		if m.Version == analysisMethodMigrationVersion {
			migration = &m
			break
		}
	}
	require.NotNil(t, migration)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
	}

	type methodRow struct {
		Content string
		Enabled bool
		Legacy  bool
	}
	var copied methodRow
	require.NoError(t, db.Raw(`SELECT content,enabled,legacy FROM analysis_methods WHERE name = ?`, prefix+"copy").Scan(&copied).Error)
	require.Equal(t, legacyContent, copied.Content)
	require.Equal(t, sha256.Sum256([]byte(legacyContent)), sha256.Sum256([]byte(copied.Content)))
	require.False(t, copied.Enabled)
	require.True(t, copied.Legacy)

	var conflict methodRow
	require.NoError(t, db.Raw(`SELECT content,enabled,legacy FROM analysis_methods WHERE name = ?`, prefix+"conflict").Scan(&conflict).Error)
	require.Equal(t, "用户已编辑正文", conflict.Content)
	require.True(t, conflict.Enabled)
	require.False(t, conflict.Legacy)

	var copiedCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM analysis_methods WHERE name = ?`, prefix+"copy").Scan(&copiedCount).Error)
	require.Equal(t, int64(1), copiedCount)
	var oldTable *string
	require.NoError(t, db.Raw(`SELECT to_regclass('public.reference_roles')::text`).Scan(&oldTable).Error)
	require.NotNil(t, oldTable)
	var oldCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM reference_roles WHERE name LIKE ?`, prefix+"%").Scan(&oldCount).Error)
	require.Equal(t, int64(2), oldCount)
}
