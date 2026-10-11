package auxlabel

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// 任务 3.3（design D3 + D5 testcontainer）：并发双 attach 同名新标签 →
// slug 唯一索引冲突冒泡、无重复行。
//
// 确定性设计（避免时序抖动）：
//  1. 栅栏：共享批量 embedder 的前两次调用（两个 attach 的 merge 批）互相等齐，
//     保证双方都在阶段一完成 embedding 决策之后才可能进入阶段二；
//  2. 表锁：辅助事务先持 semantic_labels 的 SHARE ROW EXCLUSIVE 锁（阻塞一切
//     INSERT/UPDATE，不阻塞普通 SELECT），让两个阶段二的 INSERT 都排队等锁——
//     双方都已在锁前完成 slug 查询（UniqueSemanticLabelSlug 的 SELECT 不受该锁
//     阻塞，且此时无已提交行，双双规划同一 slug）；
//  3. 释放：一个 INSERT 获胜提交，另一个撞唯一索引报错冒泡，其事务整体回滚。
//
// 该窗口（规划与写入之间的 TOCTOU）与改造前事务内「查后插入」完全一致
// （design D3：不加 ON CONFLICT 重试，冲突冒泡由打标 job 记 warn 跳过）。
func TestAttachConcurrentSameLabelSlugConflictBubbles(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tagA := models.TopicTag{Label: "并发标签甲", Slug: "concurrent-a", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tagA).Error)
	tagB := models.TopicTag{Label: "并发标签乙", Slug: "concurrent-b", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tagB).Error)

	// 栅栏：前两次调用（两路 merge 批）到齐后放行；后续调用（storage 批）直接通过。
	var arrivals int64
	gate := &sync.WaitGroup{}
	gate.Add(2)
	gatedBatch := func(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
		if n := atomic.AddInt64(&arrivals, 1); n <= 2 {
			gate.Done()
		}
		gate.Wait()
		pgVectors := make([]string, len(inputs))
		vectors := make([][]float64, len(inputs))
		for i := range inputs {
			padded := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
			pgVectors[i], vectors[i] = core.FloatsToPgVector(padded), padded
		}
		return pgVectors, vectors, nil
	}

	// 辅助事务持表写锁，钉住「双方都完成规划」的时点。
	lockTx := db.Begin()
	require.NoError(t, lockTx.Exec("LOCK TABLE semantic_labels IN SHARE ROW EXCLUSIVE MODE").Error)
	t.Cleanup(func() {
		_ = lockTx.Rollback().Error
	})

	labels := []core.AuxiliaryLabel{{Label: "同名新标签", Description: "并发创建"}}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, tag := range []*models.TopicTag{&tagA, &tagB} {
		wg.Add(1)
		go func(idx int, tagID uint) {
			defer wg.Done()
			svc := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
			svc.SetBatchEmbedder(gatedBatch)
			errs[idx] = svc.AttachAuxiliaryLabels(context.Background(), tagID, labels)
		}(i, tag.ID)
	}

	// 等双方都卡在 INSERT（阶段一已完成）后放锁，让竞争展开。
	time.Sleep(800 * time.Millisecond)
	require.NoError(t, lockTx.Rollback().Error)
	wg.Wait()

	// 恰好一路成功、一路冲突冒泡。
	var failures, successes int
	for _, err := range errs {
		if err == nil {
			successes++
		} else {
			failures++
			require.True(t, strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique"),
				"loser error must be the slug unique-index violation, got: %v", err)
		}
	}
	require.Equal(t, 1, successes, "exactly one attach must win the race: errs=%v", errs)
	require.Equal(t, 1, failures, "exactly one attach must hit the unique index: errs=%v", errs)

	// 无重复行：同名 slug 仅一行；败者零 link，胜者一条。
	var rows []models.SemanticLabel
	require.NoError(t, db.Where("slug = ?", core.Slugify("同名新标签")).Find(&rows).Error)
	require.Len(t, rows, 1, "no duplicate rows may survive the race")
	winnerID := rows[0].ID
	require.Equal(t, 1, rows[0].RefCount)

	var links []tagmodels.TopicTagSemanticLabel
	require.NoError(t, db.Where("semantic_label_id = ?", winnerID).Find(&links).Error)
	require.Len(t, links, 1, "loser's link must roll back; winner keeps exactly one")
}
