package aihealth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

// waitForSnapshot polls GetSnapshot until pred holds or the deadline passes.
func waitForSnapshot(t *testing.T, pred func(Snapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pred(GetSnapshot()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("snapshot condition not met in time")
}

// --- TryStartProbe: 无探测 in-flight 时启动异步探测 ---

func TestTryStartProbe_NoProbeInFlight_StartsAndUpdatesSnapshot(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	useFakeProbe(t, reachableByName("emb-main", "llm-main"))

	started := TryStartProbe(context.Background(), store, false)
	require.True(t, started, "no probe in flight -> must start one")

	waitForSnapshot(t, func(s Snapshot) bool { return s.CheckedAt != nil })
	snap := GetSnapshot()
	require.True(t, snap.Healthy, "async probe must update snapshot to healthy")
}

// --- TryStartProbe: 探测 in-flight 时返回 false（跳过，不并发） ---

func TestTryStartProbe_ProbeInFlight_Skipped(t *testing.T) {
	resetSnapshot()

	// Occupy the global probe mutex to simulate an in-flight probe.
	probeMu.Lock()
	defer probeMu.Unlock()

	started := TryStartProbe(context.Background(), nil, false)
	require.False(t, started, "probe in flight -> must be skipped (no concurrent probe)")
}

// --- StartPeriodicReprobe: 不健康时按间隔重探，探通自愈 ---

func TestStartPeriodicReprobe_RetriesWhileUnhealthy_SelfHeals(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	probeCalls := 0
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		probeCalls++
		if probeCalls <= 1 {
			return false, "model still loading"
		}
		return true, ""
	})

	oldInterval := reprobeInterval
	reprobeInterval = 20 * time.Millisecond
	defer func() { reprobeInterval = oldInterval }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go StartPeriodicReprobe(ctx, store, func() bool { return false })

	// Unhealthy -> keeps re-probing until a probe succeeds and flips snapshot.
	waitForSnapshot(t, func(s Snapshot) bool { return s.CheckedAt != nil && s.Healthy })
	require.GreaterOrEqual(t, probeCalls, 2, "unhealthy state must trigger repeated probes")
}

// --- StartPeriodicReprobe: 健康态持续心跳（反转旧"健康即停"契约） ---

func TestStartPeriodicReprobe_HeartbeatContinuesWhenHealthy(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	probeCalls := 0
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		probeCalls++
		return true, ""
	})

	oldInterval := reprobeInterval
	reprobeInterval = 20 * time.Millisecond
	defer func() { reprobeInterval = oldInterval }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go StartPeriodicReprobe(ctx, store, func() bool { return false })

	// First tick flips the snapshot healthy.
	waitForSnapshot(t, func(s Snapshot) bool { return s.CheckedAt != nil && s.Healthy })

	// Healthy -> the heartbeat must KEEP probing (spec: 健康态心跳持续探测).
	// A short observation window spanning several ticks must see the count
	// grow; the old contract asserted the opposite (timer idles when healthy).
	countAtHealthy := probeCalls
	deadline := time.Now().Add(2 * time.Second)
	for probeCalls <= countAtHealthy && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.Greater(t, probeCalls, countAtHealthy, "heartbeat must keep probing while snapshot is healthy")
}

// --- StartPeriodicReprobe: 探测 in-flight 时 tick 跳过（不并发探测） ---

func TestStartPeriodicReprobe_InFlightTickSkipped(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	slow := make(chan struct{})
	probeCalls := 0
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		probeCalls++
		if probeCalls == 1 {
			<-slow // hold the first probe in flight
		}
		return false, "down"
	})

	oldInterval := reprobeInterval
	reprobeInterval = 10 * time.Millisecond
	defer func() { reprobeInterval = oldInterval }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go StartPeriodicReprobe(ctx, store, func() bool { return false })

	// Wait until the first probe is in flight.
	deadline := time.Now().Add(2 * time.Second)
	for probeCalls < 1 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, probeCalls, 1, "first probe must start")

	// Several ticks while in flight: no concurrent probe may start.
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, 1, probeCalls, "ticks while probe in flight must be skipped (no concurrency)")

	close(slow)

	// After the in-flight probe finishes, the next tick may start a new probe.
	deadline2 := time.Now().Add(2 * time.Second)
	for probeCalls < 2 && time.Now().Before(deadline2) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, probeCalls, 2, "re-probing must resume after the in-flight probe completes")
}

// --- StartPeriodicReprobe: ctx 取消后停止 ---

func TestStartPeriodicReprobe_CtxCancelStops(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	// One route + provider so every probe pass actually calls probeFn (always
	// failing -> snapshot stays unhealthy -> timer keeps ticking).
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	seedBinding(t, db, er.ID, emb.ID, 1)

	probeCalls := 0
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		probeCalls++
		return false, "down"
	})

	oldInterval := reprobeInterval
	reprobeInterval = 10 * time.Millisecond
	defer func() { reprobeInterval = oldInterval }()

	ctx, cancel := context.WithCancel(context.Background())
	go StartPeriodicReprobe(ctx, store, func() bool { return false })

	deadline := time.Now().Add(2 * time.Second)
	for probeCalls < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	require.GreaterOrEqual(t, probeCalls, 2, "reprobe must be ticking")

	cancel()
	time.Sleep(60 * time.Millisecond)
	afterCancel := probeCalls
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, afterCancel, probeCalls, "no probes after ctx cancel")
}
