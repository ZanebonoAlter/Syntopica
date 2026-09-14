package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── Medium 5 / 6 / 11：并发与共享状态 ──
//
// 这些用例走 testcontainer PG（testutil.SetupTestDB，-short 下跳过）：request_key 唯一
// 冲突后重查复用、advisory lock 串行化 refresh（PG 路径）、markRecommendationAccepted
// 条件更新。sqlite 无法复现部分唯一索引与并发语义，故不用内存库。

// TestEnsureRun_ConcurrentSameRequestKeyReuse：并发同 request_key 新建 → 一方撞唯一索引后
// 重查复用，两者返回同一 run，不把冲突当 500（Medium 5）。
func TestEnsureRun_ConcurrentSameRequestKeyReuse(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewDiscoveryRunService(db, nil, nil)
	const key = "concurrent-request-key"

	const n = 8
	ids := make([]uint, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			run, _, err := svc.ensureRun(context.Background(), DiscoveryRunKindAsk, "查询", key)
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = run.ID
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "并发同 key 不得返回错误（第 %d 个）", i)
	}
	for i := 1; i < n; i++ {
		require.Equal(t, ids[0], ids[i], "并发同 key 必须收敛到同一 run")
	}
	var count int64
	require.NoError(t, db.Model(&models.DiscoveryRun{}).Where("request_key = ?", key).Count(&count).Error)
	require.EqualValues(t, 1, count, "request_key 唯一：只落一行")
}

// TestEnsureRefreshRun_ConcurrentSingleRunning：并发调用 ensureRefreshRun（PG advisory lock
// 串行化）→ 只产生一个 running refresh run，其余复用（Medium 5 防双跑）。
func TestEnsureRefreshRun_ConcurrentSingleRunning(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewDiscoveryRunService(db, nil, nil)

	const n = 6
	runIDs := make([]uint, n)
	reused := make([]bool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			run, re, err := svc.ensureRefreshRun(context.Background())
			if err != nil {
				errs[i] = err
				return
			}
			runIDs[i], reused[i] = run.ID, re
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "并发 ensureRefreshRun 不得报错（第 %d 个）", i)
	}
	for i := 1; i < n; i++ {
		require.Equal(t, runIDs[0], runIDs[i], "全部收敛到同一 refresh run")
	}
	var running int64
	require.NoError(t, db.Model(&models.DiscoveryRun{}).
		Where("kind = ? AND status = ?", DiscoveryRunKindRefresh, "running").Count(&running).Error)
	require.EqualValues(t, 1, running, "只允许一个 running refresh run")
}

// TestIsUniqueViolation 覆盖 PG 与 sqlite 两类唯一约束错误文案（保底匹配）。
func TestIsUniqueViolation(t *testing.T) {
	require.True(t, isUniqueViolation(errors.New("ERROR: duplicate key value violates unique constraint \"idx_discovery_runs_request_key\" (SQLSTATE 23505)")))
	require.True(t, isUniqueViolation(errors.New("UNIQUE constraint failed: discovery_runs.request_key")))
	require.False(t, isUniqueViolation(nil))
	require.False(t, isUniqueViolation(errors.New("connection refused")))
}
