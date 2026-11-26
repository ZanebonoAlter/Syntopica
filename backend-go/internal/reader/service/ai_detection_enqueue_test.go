package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/reader/repository"
)

// ============================================================================
// 判定入队辅助函数包级测试（add-ai-generated-article-detection task 3.3 / 3.4）
// ============================================================================

// 三个内容就绪节点共用的入队口径：开关打开才入队，开关关闭/feed 不存在不入队。
func TestEnqueueAIDetectionJobHonoursFeedSwitch(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)

	tests := []struct {
		name     string
		enabled  bool
		nilFeed  bool
		wantJobs int
	}{
		{name: "switch on: enqueued", enabled: true, wantJobs: 1},
		{name: "switch off: not enqueued", enabled: false, wantJobs: 0},
		{name: "missing feed counts as off", nilFeed: true, wantJobs: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feed := models.Feed{
				Title:              tt.name,
				URL:                "https://example.com/" + tt.name,
				AIDetectionEnabled: tt.enabled,
			}
			require.NoError(t, db.Create(&feed).Error)
			article := seedWorkerArticle(t, db, feed.ID, tt.name, "正文")

			var requestFeed *models.Feed
			if !tt.nilFeed {
				requestFeed = &feed
			}
			require.NoError(t, EnqueueAIDetectionJob(db, requestFeed, article.ID, AIDetectionReasonFirecrawlCompleted))

			var jobs []models.DetectionJob
			require.NoError(t, db.Where("article_id = ?", article.ID).Find(&jobs).Error)
			require.Len(t, jobs, tt.wantJobs)
			if tt.wantJobs == 1 {
				require.Equal(t, repository.DetectionJobReasonFirecrawlCompleted, jobs[0].Reason)
			}
		})
	}
}

// 判定入队幂等：同文章同来源重复入队不产生第二条未完成作业。
func TestEnqueueAIDetectionJobIsIdempotent(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	article := seedWorkerArticle(t, db, feed.ID, "idempotent", "正文")

	for i := 0; i < 3; i++ {
		require.NoError(t, EnqueueAIDetectionJob(db, &feed, article.ID, AIDetectionReasonArticleIngested))
	}

	var count int64
	require.NoError(t, db.Model(&models.DetectionJob{}).Where("article_id = ?", article.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// 入队失败（DB 未就绪）只是返回 error：调用方只记日志，不阻断主流程。
func TestEnqueueAIDetectionJobReportsUninitializedDB(t *testing.T) {
	feed := models.Feed{ID: 1, AIDetectionEnabled: true}
	require.Error(t, EnqueueAIDetectionJob(nil, &feed, 1, AIDetectionReasonArticleIngested))

	// 开关关闭时即便 DB 为空也不做任何事（提前返回，不报错）。
	disabled := models.Feed{ID: 1}
	require.NoError(t, EnqueueAIDetectionJob(nil, &disabled, 1, AIDetectionReasonArticleIngested))
}

// 显式重判入口：不受 feed 开关限制，入队 manual_recheck 且优先级插队。
func TestRequestAIDetectionRecheckEnqueuesForceJob(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	// 开关关闭的订阅源也允许用户显式重判（手动动作优先于自动入队口径）。
	require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed.ID).Update("ai_detection_enabled", false).Error)
	article := seedWorkerArticle(t, db, feed.ID, "recheck", "正文")

	require.NoError(t, RequestAIDetectionRecheck(db, article.ID))

	var jobs []models.DetectionJob
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.Equal(t, repository.DetectionJobReasonManualRecheck, jobs[0].Reason)
	require.True(t, repository.IsForceDetectionReason(jobs[0].Reason))
	require.Equal(t, aiDetectionRecheckPriority, jobs[0].Priority)
}

// 显式重判把既有的自动入队作业原地升级（不新建第二条）。
func TestRequestAIDetectionRecheckUpgradesPendingJob(t *testing.T) {
	db := setupAIDetectionWorkerTestDB(t)
	feed := seedWorkerFeed(t, db)
	article := seedWorkerArticle(t, db, feed.ID, "upgrade", "正文")

	require.NoError(t, EnqueueAIDetectionJob(db, &feed, article.ID, AIDetectionReasonArticleIngested))
	require.NoError(t, RequestAIDetectionRecheck(db, article.ID))

	var jobs []models.DetectionJob
	require.NoError(t, db.Where("article_id = ?", article.ID).Find(&jobs).Error)
	require.Len(t, jobs, 1)
	require.Equal(t, repository.DetectionJobReasonManualRecheck, jobs[0].Reason)
}

func TestRequestAIDetectionRecheckRejectsInvalidRequest(t *testing.T) {
	require.Error(t, RequestAIDetectionRecheck(nil, 0))
	require.Error(t, RequestAIDetectionRecheck(nil, 1))
}
