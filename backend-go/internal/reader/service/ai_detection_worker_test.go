package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
)

// ============================================================================
// 判定 worker 包级测试（add-ai-generated-article-detection task 3.2 / 3.4）
// ============================================================================

func setupAIDetectionWorkerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.DB = db
	repository.InitRepository(db)
	require.NoError(t, db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.DetectionJob{}))
	return db
}

// newAIDetectionWorkerForTest 装一个注入了假 LLM 出口的 worker（固定阈值 0.7）。
func newAIDetectionWorkerForTest(db *gorm.DB, router AIDetectionChatRouter) *AIDetectionWorker {
	svc := NewAIDetectionServiceWithRouter(router, func() (aisettings.AIDetectionConfig, error) {
		return aisettings.AIDetectionConfig{Threshold: 0.7, SampleHead: 3000, SampleTail: 2000}, nil
	})
	return newAIDetectionWorker(repository.NewDetectionJobQueue(db), svc)
}

func seedWorkerFeed(t *testing.T, db *gorm.DB) models.Feed {
	t.Helper()

	feed := models.Feed{
		Title:              "Worker Feed",
		URL:                fmt.Sprintf("https://example.com/%s", t.Name()),
		AIDetectionEnabled: true,
	}
	require.NoError(t, db.Create(&feed).Error)
	return feed
}

func seedWorkerArticle(t *testing.T, db *gorm.DB, feedID uint, title, content string) models.Article {
	t.Helper()

	article := models.Article{
		FeedID:  feedID,
		Title:   title,
		Link:    fmt.Sprintf("https://example.com/%s/%s", t.Name(), title),
		Content: content,
	}
	require.NoError(t, db.Create(&article).Error)
	return article
}

func seedWorkerJob(t *testing.T, db *gorm.DB, articleID uint, reason string) models.DetectionJob {
	t.Helper()

	job := models.DetectionJob{
		ArticleID:    articleID,
		Status:       string(models.JobStatusPending),
		AttemptCount: 0,
		MaxAttempts:  3,
		AvailableAt:  time.Now(),
		Reason:       reason,
	}
	require.NoError(t, db.Create(&job).Error)
	return job
}

// 一篇文章只发起一次判定调用：一批 3 篇 → 3 次独立调用，每篇各自落库。
func TestAIDetectionWorkerJudgesEachArticleWithOneCall(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	router := &fakeAIDetectionRouter{content: "0.86"}
	worker := newAIDetectionWorkerForTest(db, router)

	articles := []models.Article{
		seedWorkerArticle(t, db, feed.ID, "one", "正文一"),
		seedWorkerArticle(t, db, feed.ID, "two", "正文二"),
		seedWorkerArticle(t, db, feed.ID, "three", "正文三"),
	}
	for _, article := range articles {
		seedWorkerJob(t, db, article.ID, repository.DetectionJobReasonArticleIngested)
	}

	worker.processBatch()

	require.Len(t, router.requests, len(articles), "每篇文章必须各自发起一次判定调用")
	seen := map[any]bool{}
	for _, req := range router.requests {
		require.Equal(t, AIDetectionOperation, req.Operation)
		require.Equal(t, airouter.CapabilityAIDetection, req.Capability)
		seen[req.Metadata["article_id"]] = true
	}
	require.Len(t, seen, len(articles), "判定调用必须对应不同文章，不得攒批合并")

	for _, article := range articles {
		var got models.Article
		require.NoError(t, db.First(&got, article.ID).Error)
		require.Equal(t, models.AIDetectionStatusSuspect, got.AIDetectionStatus)
		require.InDelta(t, 0.86, got.AIDetectionScore, 0.0001)
		require.Equal(t, models.AIDetectionSourceLLM, got.AIDetectionSource)
		require.NotNil(t, got.AIDetectionAt)
	}

	var jobs []models.DetectionJob
	require.NoError(t, db.Find(&jobs).Error)
	require.Len(t, jobs, len(articles))
	for _, job := range jobs {
		require.Equal(t, string(models.JobStatusCompleted), job.Status)
	}
}

// 人工结论优先：已人工标记/取消的文章不调 LLM、不覆盖结论，作业直接完成。
func TestAIDetectionWorkerSkipsManualConclusion(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	router := &fakeAIDetectionRouter{content: "0.99"}
	worker := newAIDetectionWorkerForTest(db, router)

	article := seedWorkerArticle(t, db, feed.ID, "manual-cancelled", "正文")
	// 人工取消：来源 manual、状态 not_ai（人工结论优先于自动判定）。
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", article.ID).Updates(map[string]any{
		"ai_detection_status": models.AIDetectionStatusNotAI,
		"ai_detection_source": models.AIDetectionSourceManual,
		"ai_detection_score":  0.1,
	}).Error)
	job := seedWorkerJob(t, db, article.ID, repository.DetectionJobReasonArticleIngested)

	worker.processBatch()

	require.Empty(t, router.requests, "人工结论在手时不得发起自动判定调用")

	var got models.Article
	require.NoError(t, db.First(&got, article.ID).Error)
	require.Equal(t, models.AIDetectionStatusNotAI, got.AIDetectionStatus, "人工结论不得被自动结论覆盖")
	require.Equal(t, models.AIDetectionSourceManual, got.AIDetectionSource)
	require.InDelta(t, 0.1, got.AIDetectionScore, 0.0001)

	var gotJob models.DetectionJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, string(models.JobStatusCompleted), gotJob.Status, "跳过不是失败，作业应收尾为完成")
}

// 显式重判（manual_recheck）绕过人工结论跳过，并按自动结论覆盖。
func TestAIDetectionWorkerForceRecheckOverridesManualConclusion(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	router := &fakeAIDetectionRouter{content: "0.2"}
	worker := newAIDetectionWorkerForTest(db, router)

	article := seedWorkerArticle(t, db, feed.ID, "manual-suspect", "正文")
	require.NoError(t, db.Model(&models.Article{}).Where("id = ?", article.ID).Updates(map[string]any{
		"ai_detection_status": models.AIDetectionStatusSuspect,
		"ai_detection_source": models.AIDetectionSourceManual,
		"ai_detection_score":  0.95,
	}).Error)
	seedWorkerJob(t, db, article.ID, repository.DetectionJobReasonManualRecheck)

	worker.processBatch()

	require.Len(t, router.requests, 1, "显式重判必须真的发起一次判定调用")

	var got models.Article
	require.NoError(t, db.First(&got, article.ID).Error)
	require.Equal(t, models.AIDetectionStatusNotAI, got.AIDetectionStatus)
	require.Equal(t, models.AIDetectionSourceLLM, got.AIDetectionSource, "显式重判后来源应更新为自动判定")
	require.InDelta(t, 0.2, got.AIDetectionScore, 0.0001)
}

// 判定失败：作业按重试上限终止为 failed，文章保持未判定，且不触碰打标链路。
func TestAIDetectionWorkerStopsRetryingAtMaxAttempts(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	router := &fakeAIDetectionRouter{err: fmt.Errorf("simulated provider failure")}
	worker := newAIDetectionWorkerForTest(db, router)

	article := seedWorkerArticle(t, db, feed.ID, "failing", "正文")
	job := models.DetectionJob{
		ArticleID:    article.ID,
		Status:       string(models.JobStatusPending),
		AttemptCount: 0,
		MaxAttempts:  1,
		AvailableAt:  time.Now(),
		Reason:       repository.DetectionJobReasonFirecrawlCompleted,
	}
	require.NoError(t, db.Create(&job).Error)

	worker.processBatch()

	require.Len(t, router.requests, 1)

	var gotJob models.DetectionJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, string(models.JobStatusFailed), gotJob.Status)
	require.NotEmpty(t, gotJob.LastError)

	var got models.Article
	require.NoError(t, db.First(&got, article.ID).Error)
	require.Equal(t, models.AIDetectionStatusUnknown, models.NormalizeAIDetectionStatus(got.AIDetectionStatus),
		"判定失败必须让文章保持未判定")
	require.Zero(t, got.AIDetectionScore)
	require.Empty(t, got.AIDetectionSource)
}

// 租约过期后可被重新领取：worker 直接接手过期 leased 作业。
func TestAIDetectionWorkerReclaimsExpiredLease(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	router := &fakeAIDetectionRouter{content: "0.91"}
	worker := newAIDetectionWorkerForTest(db, router)

	article := seedWorkerArticle(t, db, feed.ID, "expired", "正文")
	past := time.Now().Add(-time.Hour)
	job := models.DetectionJob{
		ArticleID:      article.ID,
		Status:         string(models.JobStatusLeased),
		AttemptCount:   1,
		MaxAttempts:    5,
		AvailableAt:    past,
		LeasedAt:       &past,
		LeaseExpiresAt: &past,
		Reason:         repository.DetectionJobReasonArticleIngested,
	}
	require.NoError(t, db.Create(&job).Error)

	worker.processBatch()

	require.Len(t, router.requests, 1)

	var gotJob models.DetectionJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, string(models.JobStatusCompleted), gotJob.Status)
	require.Equal(t, 2, gotJob.AttemptCount)

	var got models.Article
	require.NoError(t, db.First(&got, article.ID).Error)
	require.Equal(t, models.AIDetectionStatusSuspect, got.AIDetectionStatus)
}

// 无可判定文本（正文/摘要全空）时不发空样本请求，作业留在重试队列。
func TestAIDetectionWorkerSkipsArticlesWithoutInput(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	router := &fakeAIDetectionRouter{content: "0.9"}
	worker := newAIDetectionWorkerForTest(db, router)

	article := models.Article{FeedID: feed.ID, Title: "empty", Link: fmt.Sprintf("https://example.com/%s/empty", t.Name())}
	require.NoError(t, db.Create(&article).Error)
	job := seedWorkerJob(t, db, article.ID, repository.DetectionJobReasonArticleIngested)

	worker.processBatch()

	require.Empty(t, router.requests, "没有判定输入时不得发起空样本请求")

	var gotJob models.DetectionJob
	require.NoError(t, db.First(&gotJob, job.ID).Error)
	require.Equal(t, string(models.JobStatusPending), gotJob.Status)
	require.NotEmpty(t, gotJob.LastError)

	var got models.Article
	require.NoError(t, db.First(&got, article.ID).Error)
	require.Equal(t, models.AIDetectionStatusUnknown, models.NormalizeAIDetectionStatus(got.AIDetectionStatus))
}
