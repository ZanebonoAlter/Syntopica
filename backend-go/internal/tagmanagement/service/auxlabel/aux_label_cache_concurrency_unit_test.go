package auxlabel

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/textutil"
)

// --- fix-aux-label-dedup-ann review R-1：L1 索引并发读的回归测试 ---
//
// 事故背景（2026-10-07 生产已发生）：applyCreate 在写锁内**原地**写 L1 三张 map
// （c.active.add(row)），而 planAuxiliaryLabels 曾在 loadActiveAuxiliaryLabels
// 返回后**无锁**读同一批 map（active.match(...)）。Go 的 map 并发读写是
// `fatal error: concurrent map read and map write`——不可 recover，进程直接退出。
// 触发面：core/tag_queue.go 以 concurrency=10 并行跑 AttachAuxiliaryLabels。
//
// 修法：L1 查找统一走 auxLabelCache.matchL1（读锁内查），applyCreate 的原地写
// 仍在写锁内。本测试是回归闸：写侧反复 applyCreate、读侧反复 matchL1。
//
// 回归强度说明（不依赖 -race 也能红）：Go 运行时自带的 map 并发访问检测会在
// 读写撞车时直接 fatal（fatal error: concurrent map read and map write）——
// 修复前这个用例会把测试进程打挂（与生产同款故障）；修复后必须稳定通过。
// `-race` 可用时另有一层影子内存告警。
//
// 对照说明：修复只加了锁，没改查找口径——同一 key 上 matchL1 与索引直查
// （auxLabelIndex.match）必须给出同一行；末尾断言逐条核对这一点。

func TestAuxLabelCacheConcurrentCreateAndMatchL1(t *testing.T) {
	const seedRows = 500
	const rounds = 5000

	seed := make([]models.SemanticLabel, 0, seedRows)
	for i := 0; i < seedRows; i++ {
		seed = append(seed, models.SemanticLabel{
			ID:    uint(i + 1),
			Label: fmt.Sprintf("既有标签%d", i),
			Slug:  fmt.Sprintf("seed-label-%d", i),
		})
	}
	cache := &auxLabelCache{active: newAuxLabelIndex(seed)}

	created := make([]models.SemanticLabel, rounds)
	var lookups, hits, misses int64
	var wg sync.WaitGroup
	writerDone := make(chan struct{})

	// 写侧：连续新建。每次 applyCreate 都原地写 L1 三张 map 与 active 列表。
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(writerDone)
		for i := 0; i < rounds; i++ {
			row := models.SemanticLabel{
				ID:    uint(seedRows + i + 1),
				Label: fmt.Sprintf("新建标签%d", i),
				Slug:  fmt.Sprintf("new-label-%d", i),
			}
			created[i] = row
			cache.applyCreate(&row)
		}
	}()

	// 读侧：写侧结束前一直查 L1（slug → alias fold → normalize key 三个口径）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-writerDone:
				return
			default:
			}
			n := atomic.AddInt64(&lookups, 1)
			i := n % seedRows
			label := fmt.Sprintf("既有标签%d", i)
			if _, hit := cache.matchL1(fmt.Sprintf("seed-label-%d", i), label, textutil.NormalizeLabelKey(label)); hit {
				atomic.AddInt64(&hits, 1)
				continue
			}
			atomic.AddInt64(&misses, 1)
		}
	}()

	wg.Wait()

	// 读侧必须真的读到了 map（否则这份回归是假绿）：种子行一直可命中。
	require.Positive(t, lookups, "reader must have performed L1 lookups")
	require.Positive(t, hits, "reader must have hit the L1 index")
	require.Zero(t, misses, "seeded rows must stay L1-matchable while the writer mutates the index")

	// 写侧全部落进索引：每条新建行都能被 L1 命中，且命中的就是它自己。
	for i := range created {
		row, hit := cache.matchL1(created[i].Slug, created[i].Label, textutil.NormalizeLabelKey(created[i].Label))
		require.True(t, hit, "created row must be L1-matchable: %s", created[i].Slug)
		require.Equal(t, created[i].ID, row.ID)
	}

	// 对照：matchL1 与索引直查口径一致（修复只加锁，不改查找语义）。
	cache.mu.RLock()
	direct, directHit := cache.active.match("seed-label-0", "既有标签0", textutil.NormalizeLabelKey("既有标签0"))
	cache.mu.RUnlock()
	locked, lockedHit := cache.matchL1("seed-label-0", "既有标签0", textutil.NormalizeLabelKey("既有标签0"))
	require.Equal(t, directHit, lockedHit)
	require.Equal(t, direct.ID, locked.ID)
}
