package auxlabel

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// --- fix-aux-label-idle-tx-cache design D5：testcontainer 真库效果回归 ---
//
// 改造前基线（生产健康检查实测）：attach 在单事务内逐标签调外部 AI API，
// pg_stat_activity 观察 idle in transaction 4–40 秒（事务持 xmin 阻塞 vacuum）。
// 本测试用 2s 延迟的批量 embedder 模拟慢 LLM：若回归为「事务内调 embedder」，
// 慢调用必然把会话留在 idle in transaction ≥2s，轮询必命中；两阶段实现下
// 延迟发生在事务外，全程不应出现超过 500ms 的 idle in transaction 会话。
//
// 说明：阶段二短事务在语句之间存在微秒级 idle in transaction 窗口（任何
// 多语句事务的固有形态），因此断言阈值取 500ms——改造前的 4–40s 病灶必中，
// 正常亚秒级纯写事务不会误伤。

func TestAttachIdleTxNoneDuringSlowEmbedder(t *testing.T) {
	db := setupAuxiliaryLabelTestDB(t)
	tag := models.TopicTag{Label: "慢嵌入标签", Slug: "slow-embed", Category: "event", Status: "active"}
	require.NoError(t, db.Create(&tag).Error)

	slowBatch := func(ctx context.Context, inputs []string, mode AuxiliaryLabelEmbeddingMode) ([]string, [][]float64, error) {
		time.Sleep(2 * time.Second) // 模拟外部 AI API 往返；两阶段下发生在事务外
		pgVectors := make([]string, len(inputs))
		vectors := make([][]float64, len(inputs))
		for i := range inputs {
			padded := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
			pgVectors[i], vectors[i] = core.FloatsToPgVector(padded), padded
		}
		return pgVectors, vectors, nil
	}
	service := NewAuxiliaryLabelService(db, (&recordingAuxiliaryEmbedder{}).embed)
	service.SetBatchEmbedder(slowBatch)

	done := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		done <- service.AttachAuxiliaryLabels(context.Background(), tag.ID, []core.AuxiliaryLabel{
			{Label: "慢标签甲", Description: "描述甲"},
			{Label: "慢标签乙", Description: "描述乙"},
			{Label: "慢标签丙", Description: "描述丙"},
		})
	}()

	// 轮询 pg_stat_activity：任何 >500ms 的 idle in transaction 会话都算命中。
	// 3 标签 × 2 批 × 2s ≈ 4s 观察窗口（旧实现的病灶窗口是整段 attach 时长）。
	var sightings []string
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case err := <-done:
			require.NoError(t, err)
			require.Empty(t, sightings, "idle in transaction sessions observed: %v", sightings)
			var labels int64
			require.NoError(t, db.Model(&models.SemanticLabel{}).Where("label_type = ?", "auxiliary").Count(&labels).Error)
			require.Equal(t, int64(3), labels)
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("attach did not finish within 30s")
		}
		var rows []struct {
			PID        int
			State      string
			AgeSeconds float64
		}
		if err := db.Raw(`
			SELECT pid, state, EXTRACT(EPOCH FROM (now() - state_change)) AS age_seconds
			FROM pg_stat_activity
			WHERE state = 'idle in transaction'
			  AND now() - state_change > interval '500 milliseconds'`).Scan(&rows).Error; err != nil {
			t.Fatalf("poll pg_stat_activity: %v", err)
		}
		for _, r := range rows {
			sightings = append(sightings, time.Now().Format(time.RFC3339)+
				" pid="+strconv.Itoa(r.PID)+" age="+time.Duration(r.AgeSeconds*float64(time.Second)).String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
