package core

// speed-up-tag-embedding 任务 2.5（test-cases.md 步 8/9 + 白盒重试退避边界）：
// markFailed 指数退避（retry 1/2/3 → 30s/60s/120s 回 pending），第 4 次失败
// 终态 failed；claim 过滤 next_retry_at 未到期任务。

import (
	"testing"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

func seedRetryTask(t *testing.T, retryCount int) uint {
	t.Helper()
	_, taskID := seedQueueTag(t, "重试标签")
	db := repository.Repo.DB()
	if err := db.Model(&models.EmbeddingQueue{}).Where("id = ?", taskID).
		Update("retry_count", retryCount).Error; err != nil {
		t.Fatalf("set retry_count: %v", err)
	}
	return taskID
}

func loadRetryTask(t *testing.T, id uint) models.EmbeddingQueue {
	t.Helper()
	var task models.EmbeddingQueue
	if err := repository.Repo.DB().First(&task, id).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	return task
}

func TestMarkFailedBackoffSchedule(t *testing.T) {
	svc, _ := newEmbeddingQueueTestService(t)

	cases := []struct {
		retryBefore int
		wantStatus  string
		wantDelay   time.Duration
	}{
		{0, models.EmbeddingQueueStatusPending, 30 * time.Second},
		{1, models.EmbeddingQueueStatusPending, 60 * time.Second},
		{2, models.EmbeddingQueueStatusPending, 120 * time.Second},
		{3, models.EmbeddingQueueStatusFailed, 0}, // 第 4 次失败：终态
	}
	for _, c := range cases {
		taskID := seedRetryTask(t, c.retryBefore)
		before := time.Now()
		svc.markFailed(taskID, "boom")
		task := loadRetryTask(t, taskID)

		if task.Status != c.wantStatus {
			t.Fatalf("retry_before=%d status=%s, want %s", c.retryBefore, task.Status, c.wantStatus)
		}
		if task.RetryCount != c.retryBefore+1 {
			t.Fatalf("retry_before=%d retry_count=%d, want %d", c.retryBefore, task.RetryCount, c.retryBefore+1)
		}
		if c.wantStatus == models.EmbeddingQueueStatusPending {
			if task.NextRetryAt == nil {
				t.Fatalf("retry_before=%d: next_retry_at not set", c.retryBefore)
			}
			got := task.NextRetryAt.Sub(before)
			if got < c.wantDelay-time.Second || got > c.wantDelay+time.Second {
				t.Fatalf("retry_before=%d delay=%v, want ~%v", c.retryBefore, got, c.wantDelay)
			}
		} else if task.CompletedAt == nil {
			t.Fatal("terminal failure must set completed_at")
		}
	}
}

func TestClaimBatchFiltersUndueRetries(t *testing.T) {
	svc, _ := newEmbeddingQueueTestService(t)

	// 到期任务（next_retry_at 已过）可领取。
	dueID := seedRetryTask(t, 1)
	past := time.Now().Add(-time.Minute)
	if err := repository.Repo.DB().Model(&models.EmbeddingQueue{}).Where("id = ?", dueID).
		Update("next_retry_at", past).Error; err != nil {
		t.Fatalf("set next_retry_at: %v", err)
	}
	// 未到期任务（差很久）不领取。
	undueID := seedRetryTask(t, 1)
	future := time.Now().Add(10 * time.Minute)
	if err := repository.Repo.DB().Model(&models.EmbeddingQueue{}).Where("id = ?", undueID).
		Update("next_retry_at", future).Error; err != nil {
		t.Fatalf("set next_retry_at: %v", err)
	}

	tasks := svc.claimBatch(embeddingBatchMax)
	if len(tasks) != 1 || tasks[0].ID != dueID {
		t.Fatalf("claimed %+v, want only due task %d", tasks, dueID)
	}
}

func TestFailedTaskNotAutoReclaimed(t *testing.T) {
	svc, _ := newEmbeddingQueueTestService(t)

	taskID := seedRetryTask(t, embeddingMaxRetries) // 第 4 次失败 → failed
	svc.markFailed(taskID, "boom")

	tasks := svc.claimBatch(embeddingBatchMax)
	for _, task := range tasks {
		if task.ID == taskID {
			t.Fatal("terminal-failed task must not be claimable; only manual retry resets it")
		}
	}

	// 手动 RetryFailed 清 next_retry_at 后可再次领取。
	if _, err := svc.RetryFailed(); err != nil {
		t.Fatalf("RetryFailed: %v", err)
	}
	task := loadRetryTask(t, taskID)
	if task.Status != models.EmbeddingQueueStatusPending || task.NextRetryAt != nil {
		t.Fatalf("after RetryFailed: status=%s next_retry_at=%v, want pending/nil", task.Status, task.NextRetryAt)
	}
	tasks = svc.claimBatch(embeddingBatchMax)
	found := false
	for _, task := range tasks {
		if task.ID == taskID {
			found = true
		}
	}
	if !found {
		t.Fatal("task must be claimable again after manual retry")
	}
}
