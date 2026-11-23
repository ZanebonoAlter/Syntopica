package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// setupTagQueueRetryTestDB 装一份单连接内存 sqlite，按生产迁移 20261120_0002 的
// 同一份 DDL 建出部分唯一索引（sqlite 测试库由 AutoMigrate 建模、不走版本迁移），
// 并把队列状态读取器单例重绑到这份库上——RetryTagQueueFailed 走该单例。
func setupTagQueueRetryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("raw db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	repository.InitRepository(db)
	tagQueueStatusService = nil
	tagQueueStatusOnce = sync.Once{}

	if err := db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TagJob{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_jobs_article_active ON tag_jobs (article_id) WHERE status IN ('pending', 'leased')`).Error; err != nil {
		t.Fatalf("create partial unique index: %v", err)
	}
	return db
}

func seedRetryTagJob(t *testing.T, db *gorm.DB, articleID uint, status string, attemptCount int) models.TagJob {
	t.Helper()
	job := models.TagJob{
		ArticleID:    articleID,
		Status:       status,
		AttemptCount: attemptCount,
		MaxAttempts:  5,
		AvailableAt:  time.Now(),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("seed tag job (article %d, %s): %v", articleID, status, err)
	}
	return job
}

func postRetryTagQueue(t *testing.T) map[string]any {
	t.Helper()
	router := setupTagQueueStatusRouter()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tag-queue/retry", nil)
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("retry endpoint returned %d: %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func assertRetryTagJobStatus(t *testing.T, db *gorm.DB, jobID uint, want string) models.TagJob {
	t.Helper()
	var job models.TagJob
	if err := db.First(&job, jobID).Error; err != nil {
		t.Fatalf("reload tag job %d: %v", jobID, err)
	}
	if job.Status != want {
		t.Fatalf("job %d status = %q, want %q", jobID, job.Status, want)
	}
	return job
}

// TestRetryTagQueueFailedSkipsArticlesWithActiveJob covers review R-1: the
// partial unique index uq_tag_jobs_article_active makes a blind
// `failed → pending` batch UPDATE collide as soon as one article holds a failed
// row next to an active one (the normal shape after a failure followed by a
// re-enqueue) — the whole statement rolled back and the endpoint answered 500
// with zero rows retried. The retry must only flip failed rows of articles
// without an active job, and at most one row per article (an article can
// accumulate several failed rows, and flipping them all would collide with
// itself).
func TestRetryTagQueueFailedSkipsArticlesWithActiveJob(t *testing.T) {
	db := setupTagQueueRetryTestDB(t)

	const (
		conflictArticle = 7001 // failed + pending：重试必须跳过这一行
		pureArticle     = 7002 // 纯 failed：正常转 pending
		stackedArticle  = 7003 // 两条 failed：只翻最新一条
	)

	failedRow := seedRetryTagJob(t, db, conflictArticle, string(models.JobStatusFailed), 5)
	activeRow := seedRetryTagJob(t, db, conflictArticle, string(models.JobStatusPending), 0)
	pureRow := seedRetryTagJob(t, db, pureArticle, string(models.JobStatusFailed), 3)
	olderFailed := seedRetryTagJob(t, db, stackedArticle, string(models.JobStatusFailed), 1)
	newerFailed := seedRetryTagJob(t, db, stackedArticle, string(models.JobStatusFailed), 2)

	body := postRetryTagQueue(t)

	if got, want := body["message"], "已重试 2 个失败任务"; got != want {
		t.Fatalf("message = %v, want %q (only the two retryable rows may flip)", got, want)
	}

	// 冲突文章：failed 行原地保持 failed，活动作业不受影响；未完成作业仍只有一条。
	assertRetryTagJobStatus(t, db, failedRow.ID, string(models.JobStatusFailed))
	assertRetryTagJobStatus(t, db, activeRow.ID, string(models.JobStatusPending))

	// 纯 failed 文章：正常转 pending，且重试计数清零。
	reloaded := assertRetryTagJobStatus(t, db, pureRow.ID, string(models.JobStatusPending))
	if reloaded.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0 after retry", reloaded.AttemptCount)
	}

	// 多条 failed 的文章：只挑最新一条，旧行保持终态，避免自己撞唯一索引。
	assertRetryTagJobStatus(t, db, olderFailed.ID, string(models.JobStatusFailed))
	assertRetryTagJobStatus(t, db, newerFailed.ID, string(models.JobStatusPending))

	var activeCount int64
	if err := db.Model(&models.TagJob{}).
		Where("article_id = ? AND status IN ?", conflictArticle,
			[]string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Count(&activeCount).Error; err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active jobs for article %d = %d, want 1 (retry must not create a second active row)", conflictArticle, activeCount)
	}
}
