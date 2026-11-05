package core

// speed-up-tag-embedding 任务 2.1/2.2（test-cases.md 步 4/11）：
// 4 worker 并发上限（并发 batch 执行 ≤ embeddingWorkerCount）；Stop 等
// goroutine 全部退出（workerGauge 归零），Stop 后 Start 不残留多 worker。

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// blockingBatchEmbedder 阻塞型 fake：每个 batch 调用进入后在 gate 释放前
// 挂住，用并发计数器观测同时进行的 batch 数。
type blockingBatchEmbedder struct {
	fakeBatchEmbedder
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	gate     chan struct{}
	once     sync.Once
}

func newBlockingBatchEmbedder(db *gorm.DB) *blockingBatchEmbedder {
	return &blockingBatchEmbedder{
		fakeBatchEmbedder: fakeBatchEmbedder{batchCalls: map[string][]int{}, db: db},
		gate:              make(chan struct{}),
	}
}

func (f *blockingBatchEmbedder) GenerateEmbeddingsBatch(ctx context.Context, tags []*models.TopicTag, embeddingType string) ([]*tagmodels.TopicTagEmbedding, error) {
	cur := f.inFlight.Add(1)
	for {
		max := f.maxSeen.Load()
		if cur <= max || f.maxSeen.CompareAndSwap(max, cur) {
			break
		}
	}
	defer f.inFlight.Add(-1)
	// 第 4 个并发 batch 到齐后放行全部（验证并发上限恰好为 4）。
	f.once.Do(func() {
		go func() {
			deadline := time.After(10 * time.Second)
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-deadline:
					close(f.gate)
					return
				case <-tick.C:
					if f.maxSeen.Load() >= int32(embeddingWorkerCount) {
						close(f.gate)
						return
					}
				}
			}
		}()
	})
	<-f.gate
	return f.fakeBatchEmbedder.GenerateEmbeddingsBatch(ctx, tags, embeddingType)
}

func TestEmbeddingQueueWorkerConcurrencyCap(t *testing.T) {
	svc, _ := newEmbeddingQueueTestService(t)
	blocking := newBlockingBatchEmbedder(repository.Repo.DB())
	svc.embedding = blocking

	origPaused := isAnalysisPaused
	isAnalysisPaused = func() bool { return false }
	t.Cleanup(func() { isAnalysisPaused = origPaused })

	// 100 任务 → claim 批 32/32/32/4，最多 4 个 worker 同时处理。
	for i := 0; i < 100; i++ {
		seedQueueTag(t, "并发标签")
	}

	svc.Start()
	defer svc.Stop()

	deadline := time.Now().Add(60 * time.Second)
	for {
		var pending int64
		repository.Repo.DB().Model(&models.EmbeddingQueue{}).
			Where("status IN ?", []string{models.EmbeddingQueueStatusPending, models.EmbeddingQueueStatusProcessing}).
			Count(&pending)
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queue not drained in time, pending+processing=%d", pending)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if got := blocking.maxSeen.Load(); got > int32(embeddingWorkerCount) {
		t.Fatalf("max concurrent batches = %d, want <= %d", got, embeddingWorkerCount)
	}
	if got := blocking.maxSeen.Load(); got != int32(embeddingWorkerCount) {
		t.Logf("note: max concurrent batches = %d (workers=%d)", got, embeddingWorkerCount)
	}
}

// waitWorkerGauge 轮询等待 worker 数达到期望（goroutine 调度启动有延迟，
// 直接断言会囥成竞态）。
func waitWorkerGauge(t *testing.T, svc *EmbeddingQueueService, want int32) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for svc.workerGauge.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("workerGauge = %d, want %d", svc.workerGauge.Load(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEmbeddingQueueStopStartNoResidualWorkers(t *testing.T) {
	svc, _ := newEmbeddingQueueTestService(t)

	origPaused := isAnalysisPaused
	isAnalysisPaused = func() bool { return false }
	t.Cleanup(func() { isAnalysisPaused = origPaused })

	svc.Start()
	waitWorkerGauge(t, svc, int32(embeddingWorkerCount))

	// Stop 必须等全部 goroutine 退出后才返回。
	svc.Stop()
	if got := svc.workerGauge.Load(); got != 0 {
		t.Fatalf("after Stop workerGauge = %d, want 0", got)
	}

	// Stop 后再 Start：worker 数回到期望，无残留叠加。
	svc.Start()
	waitWorkerGauge(t, svc, int32(embeddingWorkerCount))
	svc.Stop()
	if got := svc.workerGauge.Load(); got != 0 {
		t.Fatalf("after second Stop workerGauge = %d, want 0", got)
	}
}
