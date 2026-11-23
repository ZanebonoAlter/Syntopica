package database_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// findTagJobUniqueMigration locates migration 20261120_0002's Up closure
// (fix-feed-refresh-churn D4).
func findTagJobUniqueMigration(t *testing.T) func(*gorm.DB) error {
	t.Helper()
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261120_0002" {
			return m.Up
		}
	}
	t.Fatal("migration 20261120_0002 (tag_jobs partial unique index) must be registered in postgresMigrations()")
	return nil
}

// seedTagJob 直插一条 tag_jobs 行（状态可指定），返回其 id。
func seedTagJob(t *testing.T, db *gorm.DB, articleID uint, status string) uint {
	t.Helper()
	job := models.TagJob{
		ArticleID:   articleID,
		Status:      status,
		MaxAttempts: 5,
		AvailableAt: time.Now(),
	}
	require.NoError(t, db.Create(&job).Error, "seed tag job (%s, article %d)", status, articleID)
	return job.ID
}

// seedTagJobFull 直插一条带完整语义的 tag_jobs 行（用于断言合并语义），
// updated_at 显式钉住以控制「最新非空 reason」的归属。
func seedTagJobFull(t *testing.T, db *gorm.DB, articleID uint, status string, priority int, forceRetag bool, reason string, updatedAt time.Time) uint {
	t.Helper()
	job := models.TagJob{
		ArticleID:    articleID,
		Status:       status,
		Priority:     priority,
		ForceRetag:   forceRetag,
		Reason:       reason,
		MaxAttempts:  5,
		AvailableAt:  time.Now(),
	}
	require.NoError(t, db.Create(&job).Error, "seed tag job (%s, article %d)", status, articleID)
	require.NoError(t, db.Model(&models.TagJob{}).Where("id = ?", job.ID).Update("updated_at", updatedAt).Error)
	return job.ID
}

// TestTagJobUniqueMigrationCollapsesDuplicatesAndAddsIndex 覆盖迁移的三步：
// ① 存量同文章多条 pending/leased 先把语义（priority MAX / force_retag OR /
// reason 最新非空）合并进最小 id（审查 R-5），再删除其余行（否则建唯一索引会
// 直接失败）；② 建出部分唯一索引后，数据库自己拒绝第二条未完成作业，而终态行
// 不受限制；③ 幂等重跑（版本已记录后再次 Up）不得报错、不得改动数据。
func TestTagJobUniqueMigrationCollapsesDuplicatesAndAddsIndex(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findTagJobUniqueMigration(t)

	// Golden schema 已经跑过本迁移 → 先撤掉索引，才能种出迁移前的存量重复行。
	require.NoError(t, db.Exec(`DROP INDEX IF EXISTS uq_tag_jobs_article_active`).Error)

	const (
		dupArticle      = 9001 // 三条未完成 → 只留最小 id，语义合并进去
		singleArticle   = 9002 // 一条未完成 → 原样保留
		terminalArticle = 9003 // 两条终态 → 不清理
	)
	// 保留行语义最差（priority 0 / 无 force / 旧 reason），被删行更优——合并必须
	// 把 MAX(priority)、OR(force_retag) 和最新非空 reason 全部带进保留行（review R-5）。
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	dupKeep := seedTagJobFull(t, db, dupArticle, string(models.JobStatusPending), 0, false, "article_created", older)
	seedTagJobFull(t, db, dupArticle, string(models.JobStatusLeased), 5, true, "summary_completed", newer)
	seedTagJobFull(t, db, dupArticle, string(models.JobStatusPending), 2, false, "", older)
	single := seedTagJob(t, db, singleArticle, string(models.JobStatusPending))
	seedTagJob(t, db, terminalArticle, string(models.JobStatusCompleted))
	seedTagJob(t, db, terminalArticle, string(models.JobStatusFailed))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))

	// 合并语义：保留行拿到 MAX(priority) / OR(force_retag) / 最新非空 reason。
	var kept models.TagJob
	require.NoError(t, db.First(&kept, dupKeep).Error)
	require.Equal(t, 5, kept.Priority, "priority must merge with MAX across the collapsed rows")
	require.True(t, kept.ForceRetag, "force_retag must merge with OR (a force retag intent must not vanish)")
	require.Equal(t, "summary_completed", kept.Reason, "reason must come from the latest non-empty row")

	activeIDs := func(articleID uint) []uint {
		t.Helper()
		var ids []uint
		require.NoError(t, db.Raw(
			`SELECT id FROM tag_jobs WHERE article_id = ? AND status IN ('pending','leased') ORDER BY id`,
			articleID).Scan(&ids).Error)
		return ids
	}

	require.Equal(t, []uint{dupKeep}, activeIDs(dupArticle), "duplicate active rows must collapse onto the lowest id")
	require.Equal(t, []uint{single}, activeIDs(singleArticle), "the only active row must survive untouched")

	var terminalCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM tag_jobs WHERE article_id = ?`, terminalArticle).Scan(&terminalCount).Error)
	require.Equal(t, int64(2), terminalCount, "completed/failed rows are outside the partial index and must be preserved")

	var indexCount int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname = 'uq_tag_jobs_article_active'`).Scan(&indexCount).Error)
	require.Equal(t, int64(1), indexCount, "partial unique index must exist after the migration")

	// 数据库级兜底：同文章的第二条未完成行不再能被插入。
	dupErr := db.Exec(
		`INSERT INTO tag_jobs (article_id, status, max_attempts, available_at, created_at, updated_at)
		 VALUES (?, 'pending', 5, now(), now(), now())`, singleArticle).Error
	require.Error(t, dupErr, "a second active job for the same article must be rejected by the partial unique index")

	// 终态行不占索引位置：完成的文章照常重新入队。
	require.NoError(t, db.Exec(
		`INSERT INTO tag_jobs (article_id, status, max_attempts, available_at, created_at, updated_at)
		 VALUES (?, 'pending', 5, now(), now(), now())`, terminalArticle).Error)

	// 幂等：重复执行不报错，且不改变已收敛的数据（含合并后的语义）。
	require.NoError(t, up(db))
	require.Equal(t, []uint{dupKeep}, activeIDs(dupArticle))
	require.Equal(t, []uint{single}, activeIDs(singleArticle))
	reloaded := models.TagJob{}
	require.NoError(t, db.First(&reloaded, dupKeep).Error)
	require.Equal(t, 5, reloaded.Priority, "idempotent re-run must not reset the merged priority")
	require.True(t, reloaded.ForceRetag, "idempotent re-run must not reset the merged force_retag")
	require.Equal(t, "summary_completed", reloaded.Reason, "idempotent re-run must not reset the merged reason")
	require.Equal(t, int64(3), func() int64 {
		var n int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM tag_jobs WHERE article_id = ?`, terminalArticle).Scan(&n).Error)
		return n
	}(), "2 terminal + 1 re-enqueued pending row must survive the idempotent re-run")
}
