package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

func setupEmbeddingQueueOrphanTest(t *testing.T) *EmbeddingQueueService {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	return NewEmbeddingQueueService(nil)
}

// ── worker 兜底：tag 已删除的任务在 processNext 中被直接删除，不进 failed ──
func TestEmbeddingQueueProcessNext_DropsTaskForMissingTag(t *testing.T) {
	s := setupEmbeddingQueueOrphanTest(t)

	orphan := models.EmbeddingQueue{TagID: 99999, Status: models.EmbeddingQueueStatusPending}
	require.NoError(t, s.db.Create(&orphan).Error)

	s.processNext()

	var count int64
	require.NoError(t, s.db.Model(&models.EmbeddingQueue{}).Where("id = ?", orphan.ID).Count(&count).Error)
	assert.Zero(t, count, "orphan task should be deleted, not marked failed")
}

// ── RetryFailed：孤儿 failed 任务被清掉，正常 failed 任务被重置为 pending ──
func TestEmbeddingQueueRetryFailed_DropsOrphansAndResetsRealFailures(t *testing.T) {
	s := setupEmbeddingQueueOrphanTest(t)

	tag := models.TopicTag{Slug: "orphan-retry", Label: "orphan-retry", Category: "keyword", Source: "llm", Status: "active"}
	require.NoError(t, s.db.Create(&tag).Error)

	orphanFailed := models.EmbeddingQueue{TagID: 88888, Status: models.EmbeddingQueueStatusFailed}
	realFailed := models.EmbeddingQueue{TagID: tag.ID, Status: models.EmbeddingQueueStatusFailed}
	require.NoError(t, s.db.Create(&orphanFailed).Error)
	require.NoError(t, s.db.Create(&realFailed).Error)

	retried, err := s.RetryFailed()
	require.NoError(t, err)
	assert.EqualValues(t, 1, retried, "only the real failure should be reset to pending")

	var orphanCount int64
	require.NoError(t, s.db.Model(&models.EmbeddingQueue{}).Where("id = ?", orphanFailed.ID).Count(&orphanCount).Error)
	assert.Zero(t, orphanCount, "orphan failed task should be deleted on retry")

	var real models.EmbeddingQueue
	require.NoError(t, s.db.Where("id = ?", realFailed.ID).First(&real).Error)
	assert.Equal(t, models.EmbeddingQueueStatusPending, real.Status)
	assert.Empty(t, real.ErrorMessage)
}

// ── merge 队列同款：target tag 不存在 → processNext 直接删任务 ──
func TestMergeReembeddingProcessNext_DropsTaskForMissingTargetTag(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	s := NewMergeReembeddingQueueService(nil)

	orphan := tagmodels.MergeReembeddingQueue{TargetTagID: 77777, Status: tagmodels.MergeReembeddingQueueStatusPending}
	require.NoError(t, db.Create(&orphan).Error)

	s.processNext()

	var count int64
	require.NoError(t, db.Model(&tagmodels.MergeReembeddingQueue{}).Where("id = ?", orphan.ID).Count(&count).Error)
	assert.Zero(t, count, "orphan merge task should be deleted, not marked failed")
}

// ── merge 队列同款：RetryFailed 先清孤儿再重置 ──
func TestMergeReembeddingRetryFailed_DropsOrphansAndResetsRealFailures(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	s := NewMergeReembeddingQueueService(nil)

	target := models.TopicTag{Slug: "merge-orphan-retry", Label: "merge-orphan-retry", Category: "keyword", Source: "llm", Status: "active"}
	require.NoError(t, db.Create(&target).Error)

	orphanFailed := tagmodels.MergeReembeddingQueue{TargetTagID: 66666, Status: tagmodels.MergeReembeddingQueueStatusFailed}
	realFailed := tagmodels.MergeReembeddingQueue{TargetTagID: target.ID, Status: tagmodels.MergeReembeddingQueueStatusFailed}
	require.NoError(t, db.Create(&orphanFailed).Error)
	require.NoError(t, db.Create(&realFailed).Error)

	retried, err := s.RetryFailed()
	require.NoError(t, err)
	assert.EqualValues(t, 1, retried)

	var orphanCount int64
	require.NoError(t, db.Model(&tagmodels.MergeReembeddingQueue{}).Where("id = ?", orphanFailed.ID).Count(&orphanCount).Error)
	assert.Zero(t, orphanCount)

	var real tagmodels.MergeReembeddingQueue
	require.NoError(t, db.Where("id = ?", realFailed.ID).First(&real).Error)
	assert.Equal(t, tagmodels.MergeReembeddingQueueStatusPending, real.Status)
}
