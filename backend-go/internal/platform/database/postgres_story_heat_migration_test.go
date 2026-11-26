package database_test

// add-event-story-heat tasks 1.3：迁移 20261105_0001 的三条验收——
// ① 新表与索引存在（含复合主键、部分索引、无维度 vector）；
// ② 存量库执行后 daily_report_threads 行数不变；
// ③ 三列默认值正确（scope='single'、is_development=false、story_id=NULL）；
// 外加幂等重放与 CHECK/FK 约束形状。
//
// 三个用例共享一个 testcontainer（包内约定）：Fresh 先 DROP 两张新表再跑
// Up（覆盖裸重放路径的 CREATE TABLE 分支），cleanup 里用 RunAutoMigrate +
// Up 重建回 golden 形状；Legacy 摘掉 daily_report_threads 三列模拟存量库。

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

const storyHeatMigrationVersion = "20261105_0001"

func storyHeatMigration(t *testing.T) database.Migration {
	t.Helper()
	for _, migration := range database.ExportedPostgresMigrations() {
		if migration.Version == storyHeatMigrationVersion {
			return migration
		}
	}
	t.Fatalf("migration %s not found", storyHeatMigrationVersion)
	return database.Migration{}
}

func runStoryHeatMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	migration := storyHeatMigration(t)
	require.Nil(t, migration.Down, "migration framework has no Down executor; migration must stay forward-only")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return migration.Up(tx) }))
}

// storyHeatIndexes 是 20261105_0001 拥有的五个索引（design §6.1 命名）。
var storyHeatIndexes = []string{
	"board_stories_recall_idx",
	"board_stories_subject_idx",
	"story_signals_time_idx",
	"story_signals_story_idx",
	"daily_report_threads_story_idx",
}

// storyHeatChecks 是迁移固化的命名 CHECK（ensure 块按名幂等追加）。
var storyHeatChecks = []string{
	"chk_board_stories_status",
	"chk_board_stories_origin",
	"chk_story_signals_kind",
	"chk_story_signals_observed_at_source",
	"chk_daily_report_threads_scope",
	"chk_daily_report_threads_display_tier",
}

// storyHeatFKs 是迁移所有的外键（AutoMigrate 不建 FK，迁移是唯一属主）。
var storyHeatFKs = []string{
	"fk_board_stories_board",
	"fk_board_stories_lane",
	"fk_board_stories_merged_into",
	"fk_story_signals_story",
	"fk_story_signals_article",
	"fk_daily_report_threads_story",
}

// seedStoryHeatBoard 插入一条最小 board（semantic_labels label_type='board'）
// 供 board_stories 的 FK 与默认值断言使用，返回其 id。
func seedStoryHeatBoard(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO semantic_labels (id, label, slug, label_type)
		VALUES (?, 'story-heat-mig-board', ?, 'board')`, boardID, fmt.Sprintf("story-heat-mig-board-%d", boardID)).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM semantic_labels WHERE id = ?`, boardID).Error
	})
	return boardID
}

// TestStoryHeatMigrationFreshReplay 覆盖 CREATE TABLE 分支：先 DROP 两张新表
// （CASCADE 会连带摘掉指向它们的 FK），Up 必须凭自身 DDL 原样重建。
func TestStoryHeatMigrationFreshReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	// 摘表回到「无事件线」状态；story_signals 先走（它引用 board_stories）。
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS story_signals`).Error)
	require.NoError(t, db.Exec(`DROP TABLE IF EXISTS board_stories CASCADE`).Error)

	runStoryHeatMigration(t, db)

	// 两张表回来了。
	for _, table := range []string{"board_stories", "story_signals"} {
		require.True(t, tableExistsRaw(t, db, table), "table %s must exist after Up", table)
	}

	// 五个索引齐备；recall 是部分索引（predicate 带 WHERE）。
	for _, index := range storyHeatIndexes {
		require.True(t, indexExistsRaw(t, db, index), "index %s must exist after Up", index)
	}
	var recallDef string
	require.NoError(t, db.Raw(`SELECT pg_get_indexdef('board_stories_recall_idx'::regclass)`).Row().Scan(&recallDef))
	require.Contains(t, recallDef, "WHERE", "recall index must stay partial")

	// 复合主键 (story_id, article_id)。
	pkCols := primaryKeyColumns(t, db, "story_signals")
	require.Equal(t, []string{"story_id", "article_id"}, pkCols)

	// 红线：embedding / subject_embedding 必须无维度限定（format_type='vector'）。
	for _, column := range []string{"embedding", "subject_embedding"} {
		require.Equal(t, "vector", columnFormatType(t, db, "board_stories", column),
			"board_stories.%s must be dimension-less vector", column)
	}

	// 默认值：插入不写 status/origin，落库 active/rule。
	boardID := seedStoryHeatBoard(t, db, 95601)
	require.NoError(t, db.Exec(`INSERT INTO board_stories (board_id, title, first_seen_date, last_seen_date)
		VALUES (?, 'story-heat-mig-story', '2026-11-04', '2026-11-04')`, boardID).Error)
	var storyID uint
	require.NoError(t, db.Raw(`SELECT id FROM board_stories WHERE board_id = ? AND title = 'story-heat-mig-story'`, boardID).Scan(&storyID).Error)
	require.NotZero(t, storyID)
	var status, origin string
	require.NoError(t, db.Raw(`SELECT status, origin FROM board_stories WHERE id = ?`, storyID).Row().Scan(&status, &origin))
	require.Equal(t, "active", status)
	require.Equal(t, "rule", origin)

	// CHECK 拒绝非法值。
	require.Error(t, db.Exec(`INSERT INTO board_stories (board_id, title, first_seen_date, last_seen_date, status)
		VALUES (?, 'x', '2026-11-04', '2026-11-04', 'weird')`, boardID).Error, "illegal status must be rejected")
	require.Error(t, db.Exec(`INSERT INTO board_stories (board_id, title, first_seen_date, last_seen_date, origin)
		VALUES (?, 'x', '2026-11-04', '2026-11-04', 'ghost')`, boardID).Error, "illegal origin must be rejected")

	// story_signals：合法行落库；kind / observed_at_source CHECK 生效；
	// 复合主键挡重复证据；文章 FK 挡悬空 article_id。
	require.NoError(t, db.Exec(`INSERT INTO articles (feed_id, title, link, summary_status, firecrawl_status)
		VALUES (987601, 'story-heat-mig-article', 'https://example.com/story-heat-mig', '', 'completed')`).Error)
	var articleID uint
	require.NoError(t, db.Raw(`SELECT id FROM articles WHERE title = 'story-heat-mig-article' LIMIT 1`).Scan(&articleID).Error)
	require.NotZero(t, articleID)
	require.NoError(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, observed_at)
		VALUES (?, ?, 987601, 'feed:987601', '2026-11-04 09:00+08')`, storyID, articleID).Error)
	require.Error(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, observed_at)
		VALUES (?, ?, 987601, 'feed:987601', '2026-11-04 09:00+08')`, storyID, articleID).Error,
		"duplicate (story_id, article_id) evidence must be rejected by the composite PK")
	require.Error(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, kind, observed_at)
		VALUES (?, ?, 987601, 'feed:987601', 'rumor', '2026-11-04 09:00+08')`, storyID, articleID).Error,
		"illegal kind must be rejected")
	require.Error(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, observed_at_source, observed_at)
		VALUES (?, ?, 987601, 'feed:987601', 'guessed', '2026-11-04 09:00+08')`, storyID, articleID).Error,
		"illegal observed_at_source must be rejected")
	require.Error(t, db.Exec(`INSERT INTO story_signals (story_id, article_id, feed_id, participant_key, observed_at)
		VALUES (?, 999999999, 987601, 'feed:987601', '2026-11-04 09:00+08')`, storyID).Error,
		"dangling article_id must be rejected by the FK")

	// cleanup：清种子行，再重建回 canonical golden 形状（AutoMigrate 先建裸
	// 表 + Up 补索引/命名 CHECK/FK），保证共享容器对后续测试无漂移。
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM articles WHERE title = 'story-heat-mig-article'`).Error
		_ = db.Exec(`DROP TABLE IF EXISTS story_signals`).Error
		_ = db.Exec(`DROP TABLE IF EXISTS board_stories CASCADE`).Error
		require.NoError(t, database.RunAutoMigrate(db))
		runStoryHeatMigration(t, db)
	})
}

// TestStoryHeatMigrationLegacyThreadsUnchanged 模拟存量库：摘掉三列 → 落
// 旧行状事件 → Up 加列。行数必须不变，旧行三列取默认值（story_id=NULL、
// scope='single'、is_development=false），新写事件不写 scope 时也取默认值。
func TestStoryHeatMigrationLegacyThreadsUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	// 摘列（连带摘掉列上的索引与依赖约束），回到迁移前形状。
	for _, column := range []string{"story_id", "scope", "is_development", "story_manual", "display_tier"} {
		require.NoError(t, db.Exec(`ALTER TABLE daily_report_threads DROP COLUMN IF EXISTS `+column).Error)
	}

	// 种子：board → report → section → 两条存量事件。
	boardID := seedStoryHeatBoard(t, db, 95611)
	require.NoError(t, db.Exec(`INSERT INTO board_daily_reports (semantic_board_id, period_date, title)
		VALUES (?, '2026-11-04', 'story-heat-mig-report')`, boardID).Error)
	var reportID uint
	require.NoError(t, db.Raw(`SELECT id FROM board_daily_reports WHERE semantic_board_id = ? AND title = 'story-heat-mig-report'`, boardID).Scan(&reportID).Error)
	require.NotZero(t, reportID)
	require.NoError(t, db.Exec(`INSERT INTO daily_report_sections (report_id, cluster_index)
		VALUES (?, 0)`, reportID).Error)
	var sectionID uint
	require.NoError(t, db.Raw(`SELECT id FROM daily_report_sections WHERE report_id = ? LIMIT 1`, reportID).Scan(&sectionID).Error)
	require.NotZero(t, sectionID)
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (report_id, section_id, title)
		VALUES (?, ?, 'story-heat-mig-thread-1'), (?, ?, 'story-heat-mig-thread-2')`,
		reportID, sectionID, reportID, sectionID).Error)
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM daily_report_threads WHERE report_id = ?`, reportID).Error
		_ = db.Exec(`DELETE FROM daily_report_sections WHERE report_id = ?`, reportID).Error
		_ = db.Exec(`DELETE FROM board_daily_reports WHERE id = ?`, reportID).Error
	})

	var before int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM daily_report_threads`).Scan(&before).Error)

	runStoryHeatMigration(t, db)

	// ② 行数不变（加列不触碰行）。
	var after int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM daily_report_threads`).Scan(&after).Error)
	require.Equal(t, before, after, "thread row count must be unchanged")

	// ③ 存量行默认值：story_id=NULL、scope='single'、is_development=false、
	// story_manual=false（人工标记位不因迁移翻转）、display_tier='body'
	// （存量事件一律未降级，展示层从正文起步）。
	type threadDefaults struct {
		StoryID       *uint
		Scope         *string
		IsDevelopment *bool
		StoryManual   *bool
		DisplayTier   *string
	}
	var rows []threadDefaults
	require.NoError(t, db.Raw(`SELECT story_id, scope, is_development, story_manual, display_tier FROM daily_report_threads
		WHERE report_id = ? ORDER BY id`, reportID).Scan(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Nil(t, row.StoryID, "legacy thread keeps story_id NULL (no backfill)")
		require.NotNil(t, row.Scope)
		require.Equal(t, "single", *row.Scope)
		require.NotNil(t, row.IsDevelopment)
		require.False(t, *row.IsDevelopment)
		require.NotNil(t, row.StoryManual)
		require.False(t, *row.StoryManual)
		require.NotNil(t, row.DisplayTier)
		require.Equal(t, "body", *row.DisplayTier)
	}

	// 重放路径的四列均为 NOT NULL（ADD COLUMN 自带 + ensureNotNullDefault）。
	for _, column := range []string{"scope", "is_development", "story_manual", "display_tier"} {
		_, nullable := columnMeta(t, db, "daily_report_threads", column)
		require.Equal(t, "NO", nullable, "daily_report_threads.%s must be NOT NULL after Up", column)
	}

	// 迁移后新写事件（不写 scope）也落默认值——验证 ADD COLUMN 的 DEFAULT。
	require.NoError(t, db.Exec(`INSERT INTO daily_report_threads (report_id, section_id, title)
		VALUES (?, ?, 'story-heat-mig-thread-3')`, reportID, sectionID).Error)
	var scope, displayTier string
	var isDev, isManual bool
	var storyID *uint
	require.NoError(t, db.Raw(`SELECT story_id, scope, is_development, story_manual, display_tier FROM daily_report_threads
		WHERE report_id = ? AND title = 'story-heat-mig-thread-3'`, reportID).Row().Scan(&storyID, &scope, &isDev, &isManual, &displayTier))
	require.Nil(t, storyID)
	require.Equal(t, "single", scope)
	require.False(t, isDev)
	require.False(t, isManual)
	require.Equal(t, "body", displayTier)

	// scope CHECK 对旧行同样生效（ensure 块已补约束）。
	require.Error(t, db.Exec(`UPDATE daily_report_threads SET scope = 'weird'
		WHERE report_id = ? AND title = 'story-heat-mig-thread-3'`, reportID).Error,
		"illegal scope must be rejected by the CHECK")
	// display_tier CHECK：body/flash/hidden 之外的值一律拒绝。
	require.NoError(t, db.Exec(`UPDATE daily_report_threads SET display_tier = 'flash'
		WHERE report_id = ? AND title = 'story-heat-mig-thread-3'`, reportID).Error,
		"flash is a legal tier")
	require.Error(t, db.Exec(`UPDATE daily_report_threads SET display_tier = 'sidebar'
		WHERE report_id = ? AND title = 'story-heat-mig-thread-3'`, reportID).Error,
		"illegal display_tier must be rejected by the CHECK")

	// 行数核对后清理：四列已在 Up 中恢复，golden 形状无损。
}

// TestStoryHeatMigrationIdempotentReplay 在 golden 形状上重复跑 Up 两次：
// 不报错、命名约束与索引各只存在一份。
func TestStoryHeatMigrationIdempotentReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	runStoryHeatMigration(t, db)
	runStoryHeatMigration(t, db) // 重放：不报错、不重复建

	// 命名 CHECK 各一份。
	for _, check := range storyHeatChecks {
		var count int
		require.NoError(t, db.Raw(`SELECT count(*) FROM pg_constraint WHERE conname = ?`, check).Scan(&count).Error)
		require.Equal(t, 1, count, "check %s must exist exactly once", check)
	}
	// FK 各一份。
	for _, fk := range storyHeatFKs {
		var count int
		require.NoError(t, db.Raw(`SELECT count(*) FROM pg_constraint WHERE conname = ?`, fk).Scan(&count).Error)
		require.Equal(t, 1, count, "fk %s must exist exactly once", fk)
	}
	// 索引各一份。
	for _, index := range storyHeatIndexes {
		var count int
		require.NoError(t, db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = ?`, index).Scan(&count).Error)
		require.Equal(t, 1, count, "index %s must exist exactly once", index)
	}
	// golden 路径（AutoMigrate 建表）下复合主键仍由模型 tag 产出。
	require.Equal(t, []string{"story_id", "article_id"}, primaryKeyColumns(t, db, "story_signals"))
	// golden 路径下 vector 列同样无维度（模型 tag type:vector）。
	for _, column := range []string{"embedding", "subject_embedding"} {
		require.Equal(t, "vector", columnFormatType(t, db, "board_stories", column))
	}

	// golden 路径（AutoMigrate 先建可空列）的 NOT NULL 由 ensureNotNullDefault
	// 固化：scope / is_development / story_manual / display_tier / board_stories.title 均不可空。
	for _, c := range []struct{ table, column string }{
		{"daily_report_threads", "scope"},
		{"daily_report_threads", "is_development"},
		{"daily_report_threads", "story_manual"},
		{"daily_report_threads", "display_tier"},
		{"board_stories", "title"},
	} {
		_, nullable := columnMeta(t, db, c.table, c.column)
		require.Equal(t, "NO", nullable, "%s.%s must be NOT NULL on the golden path", c.table, c.column)
	}
	// 约束列一律 text（防 size tag 回潮 → AutoMigrate 反复 ALTER + 丢 NOT NULL）。
	for _, c := range []struct{ table, column string }{
		{"board_stories", "status"},
		{"board_stories", "origin"},
		{"story_signals", "kind"},
		{"story_signals", "observed_at_source"},
		{"daily_report_threads", "scope"},
		{"daily_report_threads", "display_tier"},
	} {
		dataType, _ := columnMeta(t, db, c.table, c.column)
		require.Equal(t, "text", dataType, "%s.%s must be text (model tag must stay size-less)", c.table, c.column)
	}
}

// ── 原始 catalog 断言助手 ──────────────────────────────────────────────────

func tableExistsRaw(t *testing.T, db *gorm.DB, table string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.Raw(`SELECT to_regclass(?) IS NOT NULL`, "public."+table).Scan(&exists).Error)
	return exists
}

// columnMeta 返回列的 (data_type, is_nullable)，来自 information_schema。
func columnMeta(t *testing.T, db *gorm.DB, table, column string) (string, string) {
	t.Helper()
	var dataType, nullable string
	require.NoError(t, db.Raw(`SELECT data_type, is_nullable FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`, table, column).Row().Scan(&dataType, &nullable))
	return dataType, nullable
}

func indexExistsRaw(t *testing.T, db *gorm.DB, index string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, db.Raw(`SELECT count(*) > 0 FROM pg_indexes WHERE indexname = ?`, index).Scan(&exists).Error)
	return exists
}

// primaryKeyColumns 返回表的主键列（按 attnum 升序）。
func primaryKeyColumns(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()
	rows, err := db.Raw(`
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = to_regclass('public.` + table + `') AND i.indisprimary
		ORDER BY a.attnum`).Rows()
	require.NoError(t, err)
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var col string
		require.NoError(t, rows.Scan(&col))
		cols = append(cols, col)
	}
	require.NoError(t, rows.Err())
	return cols
}

// columnFormatType 返回列的 format_type（vector 无维度时为 'vector'）。
func columnFormatType(t *testing.T, db *gorm.DB, table, column string) string {
	t.Helper()
	var typeStr string
	require.NoError(t, db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = ? AND a.attname = ?`, table, column).Row().Scan(&typeStr))
	return typeStr
}
