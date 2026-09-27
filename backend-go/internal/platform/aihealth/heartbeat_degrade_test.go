package aihealth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

// 心跳降级状态机（test-cases.md 白盒附加分支表）的行为测试。探测一律经
// RunStartupProbe / TryStartProbe 驱动（与心跳 tick 共用同一入口与
// failStreak 域），节奏类的 ticker 行为由 reprobe_test.go 覆盖。

// probeReachableHook lets tests flip the shared fake probe installed by
// seedHealthyPair between reachable / refused without re-registering it.
var probeReachableHook func(bool)

// seedHealthyPair seeds one embedding + one llm route, installs a flippable
// fake probe (reachable by default), and returns the store.
func seedHealthyPair(t *testing.T) *airouter.Store {
	t.Helper()
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	reachable := true
	probeReachableHook = func(v bool) { reachable = v }
	t.Cleanup(func() { probeReachableHook = nil })

	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		if reachable {
			return true, ""
		}
		return false, "connection refused"
	})
	return store
}

func degradeProbeToFailure(t *testing.T) {
	t.Helper()
	require.NotNil(t, probeReachableHook, "fake probe hook not installed; call seedHealthyPair first")
	probeReachableHook(false)
}

func restoreProbeToSuccess(t *testing.T) {
	t.Helper()
	require.NotNil(t, probeReachableHook, "fake probe hook not installed; call seedHealthyPair first")
	probeReachableHook(true)
}

func allRoutesReachable(s Snapshot) bool {
	for _, r := range s.Routes {
		if !r.Reachable {
			return false
		}
	}
	return true
}

// 分支表行 3→4：healthy 下首次探测失败 → 去抖窗内保持 Healthy=true，
// 但明细（Routes/CheckedAt）照实更新为最新一次探测。
func TestHeartbeatDegrade_FirstFailureHoldsHealthy_UpdatesDetails(t *testing.T) {
	store := seedHealthyPair(t)

	RunStartupProbe(context.Background(), store, false)
	require.True(t, Healthy(), "baseline probe must establish healthy")

	degradeProbeToFailure(t)
	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	require.True(t, snap.Healthy, "first failure must stay healthy (debounce window)")
	require.NotNil(t, snap.CheckedAt, "CheckedAt must advance even inside the debounce window")
	require.False(t, allRoutesReachable(snap), "per-route details must reflect the failing probe")
	for _, r := range snap.Routes {
		require.NotEmpty(t, r.Error, "failing route entry must carry the error")
	}
}

// 分支表行 5：healthy 下连续第 2 次探测失败 → 降级 not healthy。
func TestHeartbeatDegrade_TwoConsecutiveFailures_Degrades(t *testing.T) {
	store := seedHealthyPair(t)

	RunStartupProbe(context.Background(), store, false)
	require.True(t, Healthy())

	degradeProbeToFailure(t)
	RunStartupProbe(context.Background(), store, false)
	require.True(t, Healthy(), "failure #1 must not degrade yet (streak=1 < 2)")

	RunStartupProbe(context.Background(), store, false)
	require.False(t, Healthy(), "failure #2 must degrade the snapshot (streak=2)")
	require.False(t, GetSnapshot().Healthy)
}

// 边界：成功重置 streak —— fail/ok/fail 后再 fail 才降级，
// 防历史失败残留导致单次失败即降级。
func TestHeartbeatDegrade_SuccessResetsStreak(t *testing.T) {
	store := seedHealthyPair(t)

	RunStartupProbe(context.Background(), store, false) // ok
	require.True(t, Healthy())

	degradeProbeToFailure(t)
	RunStartupProbe(context.Background(), store, false) // fail (#1)
	require.True(t, Healthy(), "first failure stays in debounce window")

	restoreProbeToSuccess(t)
	RunStartupProbe(context.Background(), store, false) // ok -> streak reset
	require.True(t, Healthy())

	degradeProbeToFailure(t)
	RunStartupProbe(context.Background(), store, false) // fail (streak back to 1)
	require.True(t, Healthy(), "post-reset first failure must NOT degrade (streak restarted)")

	RunStartupProbe(context.Background(), store, false) // fail (streak=2)
	require.False(t, Healthy(), "second consecutive failure after reset degrades")
}

// 跨触发源累计：心跳 TryStartProbe 失败 ×1 + 手动 RunStartupProbe 失败 ×1
// → 降级（failStreak 属 probeMu 域，不区分触发源）。
func TestHeartbeatDegrade_CrossTriggerSourceAccumulates(t *testing.T) {
	store := seedHealthyPair(t)

	RunStartupProbe(context.Background(), store, false)
	require.True(t, Healthy())

	degradeProbeToFailure(t)
	started := TryStartProbe(context.Background(), store, false)
	require.True(t, started, "no probe in flight -> heartbeat tick must start one")
	// 等异步探测写完：明细翻红但整体仍 healthy（去抖窗）。
	waitForSnapshot(t, func(s Snapshot) bool { return s.CheckedAt != nil && !allRoutesReachable(s) })
	require.True(t, Healthy(), "heartbeat failure #1 stays healthy (debounce)")

	RunStartupProbe(context.Background(), store, false) // 手动重探补上第 2 次失败
	require.False(t, Healthy(), "failures from different triggers must share the streak")
}

// 分支表行 7：降级后单次探通即恢复（升级路径保持现状语义）。
func TestHeartbeatRecover_SingleSuccessAfterDegrade(t *testing.T) {
	store := seedHealthyPair(t)

	RunStartupProbe(context.Background(), store, false)
	require.True(t, Healthy())

	degradeProbeToFailure(t)
	RunStartupProbe(context.Background(), store, false)
	RunStartupProbe(context.Background(), store, false)
	require.False(t, Healthy())

	restoreProbeToSuccess(t)
	RunStartupProbe(context.Background(), store, false)
	require.True(t, Healthy(), "a single successful probe must restore healthy")
	require.True(t, allRoutesReachable(GetSnapshot()), "restored snapshot routes must all be reachable")
}

// 分支表行 2：not-ready（CheckedAt=nil，启动竞态）首次探测失败 → 不走去抖
// 直接 not healthy（fail-closed）。
func TestHeartbeat_NotReadyFirstFail_NoDebounce(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		return false, "connection refused"
	})

	require.False(t, Healthy(), "not-ready snapshot must read as not healthy")
	RunStartupProbe(context.Background(), store, false)
	require.False(t, Healthy(), "first failing probe from not-ready must land not-healthy immediately (no debounce)")
}
