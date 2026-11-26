package core

// speed-up-tag-embedding 任务 2.3（test-cases.md 步 5 + 白盒批量切批边界）：
// 批量处理下同 embedding_type 文本一次 provider 调用（identity/semantic 各一
// 次），而非每标签串行两次单条调用。

import (
	"context"
	"testing"

	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// fakeBatchEmbedder 记录批量调用次数与批量大小（按 embedding_type 分桶）。
type fakeBatchEmbedder struct {
	batchCalls map[string][]int // embedding_type → 每次 batch 的标签数
	saveFails  map[uint]bool    // tagID → SaveEmbedding 强制失败
	db         *gorm.DB
}

func (f *fakeBatchEmbedder) GenerateEmbeddingsBatch(_ context.Context, tags []*models.TopicTag, embeddingType string) ([]*tagmodels.TopicTagEmbedding, error) {
	f.batchCalls[embeddingType] = append(f.batchCalls[embeddingType], len(tags))
	out := make([]*tagmodels.TopicTagEmbedding, 0, len(tags))
	for _, tag := range tags {
		out = append(out, &tagmodels.TopicTagEmbedding{
			TopicTagID:    tag.ID,
			EmbeddingType: embeddingType,
			EmbeddingVec:  "[0.1,0.2]",
			Dimension:     2,
			Model:         "fake",
			TextHash:      hashText(embeddingType + "\n" + tag.Label),
		})
	}
	return out, nil
}

func (f *fakeBatchEmbedder) GenerateEmbeddingForText(_ context.Context, tagID uint, embeddingType string, text string) (*tagmodels.TopicTagEmbedding, []float64, error) {
	return &tagmodels.TopicTagEmbedding{
		TopicTagID:    tagID,
		EmbeddingType: embeddingType,
		EmbeddingVec:  "[0.1,0.2]",
		Dimension:     2,
		Model:         "fake",
		TextHash:      hashText(embeddingType + "\n" + text),
	}, []float64{0.1, 0.2}, nil
}

func (f *fakeBatchEmbedder) SaveEmbedding(embedding *tagmodels.TopicTagEmbedding) error {
	if f.saveFails[embedding.TopicTagID] {
		return errFakeSave
	}
	return f.db.Create(embedding).Error
}

var errFakeSave = &fakeErr{}

type fakeErr struct{}

func (*fakeErr) Error() string { return "fake save failure" }

func newEmbeddingQueueTestService(t *testing.T) (*EmbeddingQueueService, *fakeBatchEmbedder) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	svc := NewEmbeddingQueueService(nil)
	fake := &fakeBatchEmbedder{batchCalls: map[string][]int{}, db: db}
	svc.embedding = fake
	return svc, fake
}

func seedQueueTag(t *testing.T, label string) (uint, uint) {
	t.Helper()
	db := repository.Repo.DB()
	tag := models.TopicTag{Label: label, Category: "keyword", Kind: "topic", Slug: Slugify(label), Status: "active"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	task := models.EmbeddingQueue{TagID: tag.ID, Status: models.EmbeddingQueueStatusPending}
	if err := db.Create(&task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}
	return tag.ID, task.ID
}

// TestEmbeddingQueueBatchProviderCalls 步 5：10 任务 → identity/semantic 各
// 恰好 1 次批量调用（共 2 次），全部任务 completed。
func TestEmbeddingQueueBatchProviderCalls(t *testing.T) {
	svc, fake := newEmbeddingQueueTestService(t)

	var taskIDs []uint
	for i := 0; i < 10; i++ {
		_, taskID := seedQueueTag(t, "批量标签")
		taskIDs = append(taskIDs, taskID)
	}

	if !svc.processNext() {
		t.Fatal("expected a claimed batch")
	}

	for _, typ := range []string{EmbeddingTypeIdentity, EmbeddingTypeSemantic} {
		calls := fake.batchCalls[typ]
		if len(calls) != 1 {
			t.Fatalf("%s batch calls = %d, want 1 (batched)", typ, len(calls))
		}
		if calls[0] != 10 {
			t.Fatalf("%s batch size = %d, want 10", typ, calls[0])
		}
	}

	var completed int64
	repository.Repo.DB().Model(&models.EmbeddingQueue{}).
		Where("id IN ? AND status = ?", taskIDs, models.EmbeddingQueueStatusCompleted).Count(&completed)
	if completed != 10 {
		t.Fatalf("completed = %d, want 10", completed)
	}

	// 空队列再跑一轮：无新批量调用。
	if svc.processNext() {
		t.Fatal("expected no claimable batch on empty queue")
	}
	if len(fake.batchCalls[EmbeddingTypeIdentity]) != 1 {
		t.Fatalf("unexpected extra batch call after drain")
	}
}

// TestEmbeddingQueueBatchCap32 白盒边界：33 任务触发两批（32+1）。
func TestEmbeddingQueueBatchCap32(t *testing.T) {
	svc, fake := newEmbeddingQueueTestService(t)

	for i := 0; i < 33; i++ {
		seedQueueTag(t, "切批标签")
	}

	if !svc.processNext() {
		t.Fatal("expected first batch (32)")
	}
	if got := fake.batchCalls[EmbeddingTypeIdentity][0]; got != embeddingBatchMax {
		t.Fatalf("first batch size = %d, want %d", got, embeddingBatchMax)
	}
	if !svc.processNext() {
		t.Fatal("expected second batch (1)")
	}
	if got := fake.batchCalls[EmbeddingTypeIdentity][1]; got != 1 {
		t.Fatalf("second batch size = %d, want 1", got)
	}
}

// TestEmbeddingQueueBatchPartialSaveFailure 幂等变体：批内一条 SaveEmbedding
// 失败只影响该任务（markFailed 重试轨道），其余照常 completed。
func TestEmbeddingQueueBatchPartialSaveFailure(t *testing.T) {
	svc, fake := newEmbeddingQueueTestService(t)

	var keepTagID uint
	for i := 0; i < 3; i++ {
		tagID, _ := seedQueueTag(t, "部分失败标签")
		if i == 1 {
			keepTagID = tagID
		}
	}
	fake.saveFails = map[uint]bool{keepTagID: true}

	svc.processNext()

	var statuses []string
	repository.Repo.DB().Model(&models.EmbeddingQueue{}).Order("id ASC").Pluck("status", &statuses)
	if len(statuses) != 3 {
		t.Fatalf("task count = %d, want 3", len(statuses))
	}
	completed := 0
	retryPending := 0
	for _, s := range statuses {
		switch s {
		case models.EmbeddingQueueStatusCompleted:
			completed++
		case models.EmbeddingQueueStatusPending:
			retryPending++
		}
	}
	if completed != 2 || retryPending != 1 {
		t.Fatalf("completed=%d retryPending=%d, want 2/1", completed, retryPending)
	}
}
