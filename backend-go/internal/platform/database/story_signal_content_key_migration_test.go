package database_test

// fix-duplicate-article-heat 的迁移验收：20261110_0001 给 story_signals 加
// content_key（内容身份键）列与 (story_id, content_key) 索引——幂等、可重入、
// **不回填数据**（回填由 cmd/backfill-stories --backfill-content-keys 负责）。

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

const storyContentKeyMigrationVersion = "20261110_0001"

// runMigrationByVersion 按版本号跑一次迁移的 Up（镜像生产路径的 in-tx 执行）。
func runMigrationByVersion(t *testing.T, db *gorm.DB, version string) {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == version {
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
			return
		}
	}
	t.Fatalf("migration %s not found", version)
}

// contentKeyColumnCount 返回 story_signals.content_key 的列数（0 = 列不存在）。
func contentKeyColumnCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'story_signals' AND column_name = 'content_key'`).Scan(&count).Error)
	return count
}

func TestStorySignalContentKeyMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	// 文章 / 版块表由注册的模型建出；story_signals 在本二进制里也由注册模型
	// 建出（AutoMigrate 先行路径），20261105_0001 负责补齐索引 / CHECK / FK。
	require.NoError(t, database.RunAutoMigrate(db))
	runMigrationByVersion(t, db, "20261105_0001")
	require.True(t, tableExistsRaw(t, db, "story_signals"), "story_signals must exist before the content-key migration")

	// 回到迁移前形状：列与索引都没有（AutoMigrate 路径可能已把列建出来，
	// 这里显式摘掉，才能真正验到 ADD COLUMN 分支）。
	require.NoError(t, db.Exec(`ALTER TABLE story_signals DROP COLUMN IF EXISTS content_key`).Error)
	require.EqualValues(t, 0, contentKeyColumnCount(t, db), "content_key must not exist before the migration")
	require.False(t, indexExistsRaw(t, db, "story_signals_story_content_idx"), "content index must not exist before the migration")

	// ── 执行本迁移 ──
	runMigrationByVersion(t, db, storyContentKeyMigrationVersion)

	// 列：存在、可空、text 类型（不给 size → 不会被 AutoMigrate 反复 ALTER）。
	require.EqualValues(t, 1, contentKeyColumnCount(t, db))
	dataType, nullable := columnMeta(t, db, "story_signals", "content_key")
	require.Equal(t, "text", dataType)
	require.Equal(t, "YES", nullable, "content_key must stay nullable (history rows are not backfilled by the migration)")

	// 索引：(story_id, content_key)。
	require.True(t, indexExistsRaw(t, db, "story_signals_story_content_idx"), "content index must exist after the migration")
	var indexDef string
	require.NoError(t, db.Raw(`SELECT pg_get_indexdef('story_signals_story_content_idx'::regclass)`).Row().Scan(&indexDef))
	require.Contains(t, indexDef, "(story_id, content_key)")

	// ── 幂等重放：列不重复、索引不重复、不报错 ──
	runMigrationByVersion(t, db, storyContentKeyMigrationVersion)
	require.EqualValues(t, 1, contentKeyColumnCount(t, db))
	var indexCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = 'story_signals_story_content_idx'`).Scan(&indexCount).Error)
	require.EqualValues(t, 1, indexCount)

	// ── 迁移不碰数据：存量行的 content_key 保持未回填（NULL）──
	require.NoError(t, db.Exec(`INSERT INTO articles (feed_id, title, link, summary_status, firecrawl_status)
		VALUES (987701, 'story-content-key-article', 'https://example.com/story-content-key', '', 'completed')`).Error)
	var articleID uint
	require.NoError(t, db.Raw(`SELECT id FROM articles WHERE title = 'story-content-key-article' LIMIT 1`).Scan(&articleID).Error)
	require.NotZero(t, articleID)
	// board_stories.board_id 外键指向 semantic_labels(id)：先种一个版块。
	require.NoError(t, db.Exec(`INSERT INTO semantic_labels (id, label, slug, label_type)
		VALUES (95621, 'story-content-key-board', 'story-content-key-board', 'board')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO board_stories (board_id, title, first_seen_date, last_seen_date)
		VALUES (95621, 'story-content-key-story', '2026-11-10', '2026-11-10')`).Error)
	var storyID uint
	require.NoError(t, db.Raw(`SELECT id FROM board_stories WHERE title = 'story-content-key-story' LIMIT 1`).Scan(&storyID).Error)
	require.NotZero(t, storyID)
	require.NoError(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, observed_at)
		VALUES (?, ?, 987701, 'feed:987701', '2026-11-10 09:00+08')`, storyID, articleID).Error)
	var contentKey *string
	require.NoError(t, db.Raw(`SELECT content_key FROM story_signals WHERE story_id = ? AND article_id = ?`, storyID, articleID).Scan(&contentKey).Error)
	require.Nil(t, contentKey, "the migration must not backfill content_key (cmd/backfill-stories owns that)")

	// cleanup：清种子行，别给共享容器留漂移。
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM story_signals WHERE story_id = ?`, storyID).Error
		_ = db.Exec(`DELETE FROM board_stories WHERE title = 'story-content-key-story'`).Error
		_ = db.Exec(`DELETE FROM articles WHERE title = 'story-content-key-article'`).Error
		_ = db.Exec(`DELETE FROM semantic_labels WHERE id = 95621`).Error
	})
}
