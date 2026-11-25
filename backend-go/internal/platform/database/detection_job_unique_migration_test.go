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

// findDetectionJobUniqueMigration locates migration 20261120_0003's Up closure
// (add-ai-generated-article-detection task 3.5, same shape as fix-feed-refresh-churn D4).
func findDetectionJobUniqueMigration(t *testing.T) func(*gorm.DB) error {
	t.Helper()
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20261120_0003" {
			return m.Up
		}
	}
	t.Fatal("migration 20261120_0003 (detection_jobs partial unique index) must be registered in postgresMigrations()")
	return nil
}

// seedDetectionJob 直插一条 detection_jobs 行（状态可指定），返回其 id。
func seedDetectionJob(t *testing.T, db *gorm.DB, articleID uint, status string) uint {
	t.Helper()
	job := models.DetectionJob{
		ArticleID:   articleID,
		Status:      status,
		MaxAttempts: 5,
		AvailableAt: time.Now(),
	}
	require.NoError(t, db.Create(&job).Error, "seed detection job (%s, article %d)", status, articleID)
	return job.ID
}

// seedDetectionJobFull 直插一条带完整语义的 detection_jobs 行（用于断言合并语义），
// updated_at 显式钉住以控制「最新非空 reason」的归属。
func seedDetectionJobFull(t *testing.T, db *gorm.DB, articleID uint, status string, priority int, reason string, updatedAt time.Time) uint {
	t.Helper()
	job := models.DetectionJob{
		ArticleID:   articleID,
		Status:      status,
		Priority:    priority,
		Reason:      reason,
		MaxAttempts: 5,
		AvailableAt: time.Now(),
	}
	require.NoError(t, db.Create(&job).Error, "seed detection job (%s, article %d)", status, articleID)
	require.NoError(t, db.Model(&models.DetectionJob{}).Where("id = ?", job.ID).Update("updated_at", updatedAt).Error)
	return job.ID
}

// TestDetectionJobUniqueMigrationCollapsesDuplicatesAndAddsIndex 覆盖迁移的三步：
// ① 存量同文章多条 pending/leased 先把语义（priority MAX / reason 最新非空）合并
// 进最小 id（审查 R-5），再删除其余行；② 建出部分唯一索引后数据库自己拒绝第二条
// 未完成作业，终态行不受限制；③ 幂等重跑不得报错、不得改动数据。
func TestDetectionJobUniqueMigrationCollapsesDuplicatesAndAddsIndex(t *testing.T) {
	db := testutil.SetupTestDB(t)
	up := findDetectionJobUniqueMigration(t)

	// Golden schema 已经跑过本迁移 → 先撤掉索引，才能种出迁移前的存量重复行。
	require.NoError(t, db.Exec(`DROP INDEX IF EXISTS uq_detection_jobs_article_active`).Error)

	const (
		dupArticle      = 9101
		singleArticle   = 9102
		terminalArticle = 9103
	)
	// 保留行语义最差（priority 0 / 旧 reason），被删行更优——合并必须把
	// MAX(priority) 与最新非空 reason 带进保留行（review R-5）。
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	dupKeep := seedDetectionJobFull(t, db, dupArticle, string(models.JobStatusPending), 0, "article_ingested", older)
	seedDetectionJobFull(t, db, dupArticle, string(models.JobStatusLeased), 10, "manual_recheck", newer)
	seedDetectionJobFull(t, db, dupArticle, string(models.JobStatusPending), 3, "", older)
	single := seedDetectionJob(t, db, singleArticle, string(models.JobStatusPending))
	seedDetectionJob(t, db, terminalArticle, string(models.JobStatusCompleted))
	seedDetectionJob(t, db, terminalArticle, string(models.JobStatusFailed))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))

	// 合并语义：保留行拿到 MAX(priority) / 最新非空 reason。
	var kept models.DetectionJob
	require.NoError(t, db.First(&kept, dupKeep).Error)
	require.Equal(t, 10, kept.Priority, "priority must merge with MAX across the collapsed rows")
	require.Equal(t, "manual_recheck", kept.Reason, "reason must come from the latest non-empty row")

	activeIDs := func(articleID uint) []uint {
		t.Helper()
		var ids []uint
		require.NoError(t, db.Raw(
			`SELECT id FROM detection_jobs WHERE article_id = ? AND status IN ('pending','leased') ORDER BY id`,
			articleID).Scan(&ids).Error)
		return ids
	}

	require.Equal(t, []uint{dupKeep}, activeIDs(dupArticle), "duplicate active rows must collapse onto the lowest id")
	require.Equal(t, []uint{single}, activeIDs(singleArticle), "the only active row must survive untouched")

	var terminalCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM detection_jobs WHERE article_id = ?`, terminalArticle).Scan(&terminalCount).Error)
	require.Equal(t, int64(2), terminalCount, "completed/failed rows are outside the partial index and must be preserved")

	var indexCount int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname = 'uq_detection_jobs_article_active'`).Scan(&indexCount).Error)
	require.Equal(t, int64(1), indexCount, "partial unique index must exist after the migration")

	// 数据库级兜底：同文章的第二条未完成行不再能被插入。
	dupErr := db.Exec(
		`INSERT INTO detection_jobs (article_id, status, max_attempts, available_at, created_at, updated_at)
		 VALUES (?, 'pending', 5, now(), now(), now())`, singleArticle).Error
	require.Error(t, dupErr, "a second active job for the same article must be rejected by the partial unique index")

	// 终态行不占索引位置：完成的文章照常重新入队。
	require.NoError(t, db.Exec(
		`INSERT INTO detection_jobs (article_id, status, max_attempts, available_at, created_at, updated_at)
		 VALUES (?, 'pending', 5, now(), now(), now())`, terminalArticle).Error)

	// 幂等：重复执行不报错，且不改变已收敛的数据（含合并后的语义）。
	require.NoError(t, up(db))
	require.Equal(t, []uint{dupKeep}, activeIDs(dupArticle))
	require.Equal(t, []uint{single}, activeIDs(singleArticle))
	reloaded := models.DetectionJob{}
	require.NoError(t, db.First(&reloaded, dupKeep).Error)
	require.Equal(t, 10, reloaded.Priority, "idempotent re-run must not reset the merged priority")
	require.Equal(t, "manual_recheck", reloaded.Reason, "idempotent re-run must not reset the merged reason")

	var total int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM detection_jobs WHERE article_id = ?`, terminalArticle).Scan(&total).Error)
	require.Equal(t, int64(3), total, "2 terminal + 1 re-enqueued pending row must survive the idempotent re-run")
}
