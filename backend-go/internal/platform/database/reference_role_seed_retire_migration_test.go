package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"

	// Register data-enrichment models for AutoMigrate.
	_ "syntopica-backend/internal/dataenrichment/repository"
)

// ── 20260831_0001 seed 画像退役（board-level-deep-analysis tasks 6.3）────────
//
// testcontainer PG 驱动三个 Up 闭包的升级序列（seed → legacy copy → retire）：
//   - 原始 seed（未被用户动过）升级后 disabled，原文字节保留，幂等
//   - 用户已编辑 content / title 的同名行不被覆盖（仍 enabled，内容原样）
//   - 不同 name 但相同 content 的行不受影响
//   - 升级序列结束时：本测试所辖 reference_roles 行无 enabled，复制到
//     analysis_methods 的 legacy 记录也是 disabled（fresh DB 语义）
//
// 历史不篡改：20260826_0002 迁移本体不改，seed 行继续由它插入，retire
// 只按 name+title+冻结原文 字节匹配关掉开关。

func findMigrationUp(t *testing.T, version string) func(*gorm.DB) error {
	t.Helper()
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == version {
			return m.Up
		}
	}
	t.Fatalf("migration %s not found in list", version)
	return nil
}

func TestReferenceRoleSeedRetireMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	const prefix = "seed-retire-test-"
	const seedName = "inside-america-v2"
	// Hermetic cleanup on the shared process-singleton container.
	require.NoError(t, db.Exec(`DELETE FROM analysis_methods WHERE name LIKE ? OR name = ?`, prefix+"%", seedName).Error)
	require.NoError(t, db.Exec(`DELETE FROM reference_roles WHERE name LIKE ? OR name = ?`, prefix+"%", seedName).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM analysis_methods WHERE name LIKE ? OR name = ?`, prefix+"%", seedName).Error
		_ = db.Exec(`DELETE FROM reference_roles WHERE name LIKE ? OR name = ?`, prefix+"%", seedName).Error
	})

	seedUp := findMigrationUp(t, "20260826_0002")
	copyUp := findMigrationUp(t, "20260828_0002")
	retireUp := findMigrationUp(t, "20260831_0001")

	// 1. 原始 seed 路径：seed 插入（enabled=true，与历史部署一致）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return seedUp(tx) }))
	var seedContent string
	var seedEnabled bool
	require.NoError(t, db.Raw(`SELECT content, enabled FROM reference_roles WHERE name = ?`, seedName).
		Row().Scan(&seedContent, &seedEnabled))
	require.NotEmpty(t, seedContent, "seed migration must have inserted the frozen profile")
	require.True(t, seedEnabled, "the 20260826_0002 seed starts enabled (historical fact)")

	// 用户编辑过的同名/同内容变体（不会被 retire 触碰）。
	require.NoError(t, db.Exec(`INSERT INTO reference_roles (name,title,content,enabled,created_at,updated_at)
		VALUES (?, '内部看美国·方法论画像（v2）', '用户改写后的正文', true, now(), now())`, prefix+"edited-content").Error)
	require.NoError(t, db.Exec(`INSERT INTO reference_roles (name,title,content,enabled,created_at,updated_at)
		VALUES (?, '用户自己的新标题', ?, true, now(), now())`, prefix+"edited-title", seedContent).Error)

	// 2. legacy copy 先行（升级序列 20260828_0002），再 retire。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return copyUp(tx) }))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return retireUp(tx) }))

	// 3. 原始 seed 已 disabled 且原文字节保留。
	var after struct {
		Content string
		Enabled bool
	}
	require.NoError(t, db.Raw(`SELECT content, enabled FROM reference_roles WHERE name = ?`, seedName).
		Scan(&after).Error)
	require.False(t, after.Enabled, "pristine seed must be disabled by the retire migration")
	require.Equal(t, seedContent, after.Content, "seed content bytes must be preserved")

	// 4. 用户编辑行不被覆盖：content 改写仍 enabled，title 改写仍 enabled。
	var editedContentEnabled, editedTitleEnabled bool
	require.NoError(t, db.Raw(`SELECT enabled FROM reference_roles WHERE name = ?`, prefix+"edited-content").Scan(&editedContentEnabled).Error)
	require.True(t, editedContentEnabled, "user-edited content row must not be flipped")
	require.NoError(t, db.Raw(`SELECT enabled FROM reference_roles WHERE name = ?`, prefix+"edited-title").Scan(&editedTitleEnabled).Error)
	require.True(t, editedTitleEnabled, "user-edited title row must not be flipped")
	var editedContent string
	require.NoError(t, db.Raw(`SELECT content FROM reference_roles WHERE name = ?`, prefix+"edited-content").Scan(&editedContent).Error)
	require.Equal(t, "用户改写后的正文", editedContent)

	// 5. 复制到 analysis_methods 的 legacy 记录无 enabled 作者画像。
	var legacyEnabled int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM analysis_methods WHERE legacy AND enabled`).Scan(&legacyEnabled).Error)
	require.Zero(t, legacyEnabled, "no legacy analysis_method may be enabled")
	var copiedEnabled bool
	require.NoError(t, db.Raw(`SELECT enabled FROM analysis_methods WHERE name = ?`, seedName).Scan(&copiedEnabled).Error)
	require.False(t, copiedEnabled)

	// 6. 幂等：再跑一次 retire，状态不变、原文仍保留。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return retireUp(tx) }))
	require.NoError(t, db.Raw(`SELECT content, enabled FROM reference_roles WHERE name = ?`, seedName).Scan(&after).Error)
	require.False(t, after.Enabled)
	require.Equal(t, seedContent, after.Content)

	// 7. fresh DB 语义：把 seed 重新打开（模拟从零初始化到 20260826_0002 的
	// 库再升级）后，再跑一次 retire，seed 行不亮 enabled（用户编辑行不受影响，
	// 它们是用户内容，且无 prompt caller 读这张表）。
	require.NoError(t, db.Exec(`UPDATE reference_roles SET enabled = true WHERE name = ?`, seedName).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return retireUp(tx) }))
	var seedEnabledAfter int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM reference_roles WHERE name = ? AND enabled`, seedName).
		Scan(&seedEnabledAfter).Error)
	require.Zero(t, seedEnabledAfter, "re-enabled pristine seed must be retired again on re-run")
	// 用户编辑行始终未被 retire 改动（回归兼证明：身份匹配只认冻结字节）。
	require.NoError(t, db.Raw(`SELECT enabled FROM reference_roles WHERE name = ?`, prefix+"edited-content").Scan(&editedContentEnabled).Error)
	require.True(t, editedContentEnabled)
}
