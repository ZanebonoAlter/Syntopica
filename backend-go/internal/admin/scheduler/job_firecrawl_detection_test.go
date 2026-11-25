package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	content "syntopica-backend/internal/reader"
)

// ============================================================================
// Firecrawl 入队点包级测试（add-ai-generated-article-detection task 3.3 ① ②）
// ============================================================================

// setupFirecrawlDetectionTest 复用 firecrawl 作业测试的装配，补上 detection_jobs
// 表，并给订阅源打开判定开关。
func setupFirecrawlDetectionTest(t *testing.T, switchOn bool) (*gorm.DB, models.Feed) {
	t.Helper()

	db := setupFirecrawlJobTest(t)
	require.NoError(t, db.AutoMigrate(&models.DetectionJob{}))

	queue := content.NewFirecrawlJobQueue(db)
	feed, _ := seedFirecrawlArticles(t, db, queue, 1, false)
	require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed.ID).
		Update("ai_detection_enabled", switchOn).Error)
	return db, feed
}

func detectionJobsOf(t *testing.T, db *gorm.DB, articleID uint) []models.DetectionJob {
	t.Helper()

	var jobs []models.DetectionJob
	require.NoError(t, db.Where("article_id = ?", articleID).Find(&jobs).Error)
	return jobs
}

// 全文抓取成功完成：入队一条 firecrawl_completed 判定作业（输入用全文）。
func TestFirecrawlCompletionEnqueuesAIDetection(t *testing.T) {
	db, _ := setupFirecrawlDetectionTest(t, true)
	queue := content.NewFirecrawlJobQueue(db)

	_, err := firecrawlJobWithCrawler(queue, "detection-success", func(*content.FirecrawlConfig) content.Crawler {
		return newFakeCrawler(time.Millisecond, nil)
	})(context.Background())
	require.NoError(t, err)

	var article models.Article
	require.NoError(t, db.First(&article).Error)
	jobs := detectionJobsOf(t, db, article.ID)
	require.Len(t, jobs, 1)
	require.Equal(t, content.AIDetectionReasonFirecrawlCompleted, jobs[0].Reason)
	require.Equal(t, string(models.JobStatusPending), jobs[0].Status)
}

// 全文抓取终态失败降级：仍然入队判定（输入回退 RSS 正文），reason 为 firecrawl_failed。
func TestFirecrawlTerminalFailureEnqueuesAIDetection(t *testing.T) {
	db, _ := setupFirecrawlDetectionTest(t, true)
	queue := content.NewFirecrawlJobQueue(db)

	// 让 claim 后的 attempt_count 达到上限 → terminal 降级分支。
	require.NoError(t, db.Model(&models.FirecrawlJob{}).Where("article_id IS NOT NULL").
		Update("attempt_count", 4).Error)

	_, err := firecrawlJobWithCrawler(queue, "detection-failure", func(*content.FirecrawlConfig) content.Crawler {
		return newFakeCrawler(time.Millisecond, func(string) bool { return true })
	})(context.Background())
	require.NoError(t, err)

	var article models.Article
	require.NoError(t, db.First(&article).Error)
	require.Equal(t, "failed", article.FirecrawlStatus)

	jobs := detectionJobsOf(t, db, article.ID)
	require.Len(t, jobs, 1)
	require.Equal(t, content.AIDetectionReasonFirecrawlFailed, jobs[0].Reason)
}

// 订阅源开关关闭：抓取完成与失败降级都不产生判定作业。
func TestFirecrawlSwitchOffSkipsAIDetection(t *testing.T) {
	tests := []struct {
		name   string
		fail   bool
		bump   bool
	}{
		{name: "success path", fail: false},
		{name: "terminal failure path", fail: true, bump: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := setupFirecrawlDetectionTest(t, false)
			queue := content.NewFirecrawlJobQueue(db)

			if tt.bump {
				require.NoError(t, db.Model(&models.FirecrawlJob{}).Where("article_id IS NOT NULL").
					Update("attempt_count", 4).Error)
			}

			_, err := firecrawlJobWithCrawler(queue, "detection-off", func(*content.FirecrawlConfig) content.Crawler {
				return newFakeCrawler(time.Millisecond, func(string) bool { return tt.fail })
			})(context.Background())
			require.NoError(t, err)

			var article models.Article
			require.NoError(t, db.First(&article).Error)
			require.Empty(t, detectionJobsOf(t, db, article.ID))
		})
	}
}
