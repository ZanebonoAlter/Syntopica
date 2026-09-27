package aihealth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

// setupTestDB builds an in-memory SQLite DB with the AI tables and returns a
// ready-to-use airouter.Store. Mirrors the airouter package's own test setup.
func setupTestDB(t *testing.T) (*gorm.DB, *airouter.Store) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{}, &models.AIProvider{}, &models.AIRoute{},
		&models.AIRouteProvider{}, &models.AICallLog{},
	))
	database.DB = db
	return db, airouter.NewStore(db)
}

func seedProvider(t *testing.T, db *gorm.DB, name, kind, startCmd string, enabled bool) models.AIProvider {
	t.Helper()
	p := models.AIProvider{
		Name:         name,
		ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL:      "http://127.0.0.1:8081/v1",
		Model:        name + "-model",
		APIKey:       "k",
		Enabled:      enabled,
		ModelKind:    kind,
		StartCommand: startCmd,
	}
	require.NoError(t, db.Create(&p).Error)
	return p
}

func seedRoute(t *testing.T, db *gorm.DB, name, capability string, enabled bool) models.AIRoute {
	t.Helper()
	r := models.AIRoute{Name: name, Capability: capability, Enabled: enabled, Strategy: "ordered_failover"}
	require.NoError(t, db.Create(&r).Error)
	return r
}

func seedBinding(t *testing.T, db *gorm.DB, routeID, providerID uint, priority int) {
	t.Helper()
	seedBindingFlag(t, db, routeID, providerID, priority, true)
}

// seedBindingFlag is seedBinding with an explicit link Enabled flag, for
// scenarios where a higher-priority link is disabled.
func seedBindingFlag(t *testing.T, db *gorm.DB, routeID, providerID uint, priority int, enabled bool) {
	t.Helper()
	require.NoError(t, db.Create(&models.AIRouteProvider{
		RouteID: routeID, ProviderID: providerID, Priority: priority, Enabled: enabled,
	}).Error)
}

func resetSnapshot() {
	snapshotMu.Lock()
	current = &Snapshot{}
	snapshotMu.Unlock()
	// Degrade-debounce streak is per-test too: a leftover streak from a
	// previous test would make its first failing probe degrade immediately
	// (or hold a debounce window the test does not expect).
	failStreak = 0
	// Launch bookkeeping is per-test too: provider IDs restart at 1 in every
	// test's fresh SQLite DB, so a leftover cooldown entry would suppress the
	// next test's launch.
	launchMu.Lock()
	clear(lastLaunchAt)
	launchMu.Unlock()
}

func useFakeProbe(t *testing.T, fn func(context.Context, models.AIProvider) (bool, string)) {
	t.Helper()
	old := probeFn
	probeFn = fn
	t.Cleanup(func() { probeFn = old })
}

func useFakeLaunch(t *testing.T, fn func(string) error) {
	t.Helper()
	old := launchFn
	launchFn = fn
	t.Cleanup(func() { launchFn = old })
}

// reachableByName builds a probe fake that marks exactly the named providers
// reachable; every other provider is reported unreachable.
func reachableByName(reachable ...string) func(context.Context, models.AIProvider) (bool, string) {
	set := make(map[string]bool, len(reachable))
	for _, n := range reachable {
		set[n] = true
	}
	return func(_ context.Context, p models.AIProvider) (bool, string) {
		if set[p.Name] {
			return true, ""
		}
		return false, "connection refused"
	}
}

// entryByCapability indexes snapshot routes by capability. Within a single
// test each capability is seeded with exactly one route, so this avoids the
// ambiguity of multiple routes sharing the name "default".
func entryByCapability(s Snapshot) map[string]RouteHealth {
	out := make(map[string]RouteHealth, len(s.Routes))
	for _, r := range s.Routes {
		out[r.Capability] = r
	}
	return out
}

// --- Scenario: 启动时探测每条路由主 provider / 两类各通一条即健康 ---

func TestRunStartupProbe_AllReachable_Healthy(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	// Before first probe: not ready -> Healthy() == false.
	require.False(t, Healthy())
	require.Nil(t, GetSnapshot().CheckedAt)

	useFakeProbe(t, reachableByName("emb-main", "llm-main"))

	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	require.True(t, snap.Healthy)
	require.NotNil(t, snap.CheckedAt)
	require.False(t, snap.AutoStart)
	require.Len(t, snap.Routes, 2)

	byCap := entryByCapability(snap)
	require.True(t, byCap[string(airouter.CapabilityEmbedding)].Reachable)
	require.True(t, byCap[string(airouter.CapabilitySummary)].Reachable)

	// After probe, package Healthy() mirrors snapshot.
	require.True(t, Healthy())
}

// --- Scenario: 仅探主 provider 不探 fallback ---

func TestRunStartupProbe_OnlyPrimaryProbed_FallbackSkipped(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	primary := seedProvider(t, db, "emb-primary", "embedding", "", true)
	fallback := seedProvider(t, db, "emb-fallback", "embedding", "", true)
	llm := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	// primary is priority 1 (highest), fallback priority 2.
	seedBinding(t, db, er.ID, primary.ID, 1)
	seedBinding(t, db, er.ID, fallback.ID, 2)
	seedBinding(t, db, sr.ID, llm.ID, 1)

	var probed []string
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		probed = append(probed, p.Name)
		return true, ""
	})

	RunStartupProbe(context.Background(), store, false)

	require.NotContains(t, probed, "emb-fallback", "fallback provider must not be probed")
	require.Contains(t, probed, "emb-primary")

	snap := GetSnapshot()
	byCap := entryByCapability(snap)
	// Only the primary provider name is recorded for the embedding route.
	require.Equal(t, "emb-primary", byCap[string(airouter.CapabilityEmbedding)].PrimaryProvider)
}

// --- Scenario: priority=1 的 link 被禁用时，跳过并探下一个 enabled link ---

func TestRunStartupProbe_DisabledPrimarySkipped_NextEnabledProbed(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	disabled := seedProvider(t, db, "emb-disabled", "embedding", "", true) // provider enabled, link disabled
	active := seedProvider(t, db, "emb-active", "embedding", "", true)
	llm := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	// The disabled link has higher priority; the enabled link must be picked.
	seedBindingFlag(t, db, er.ID, disabled.ID, 1, false)
	seedBindingFlag(t, db, er.ID, active.ID, 2, true)
	seedBinding(t, db, sr.ID, llm.ID, 1)

	var probed []string
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		probed = append(probed, p.Name)
		return true, ""
	})

	RunStartupProbe(context.Background(), store, false)

	require.NotContains(t, probed, "emb-disabled", "disabled link/provider must not be probed")
	require.Contains(t, probed, "emb-active")

	snap := GetSnapshot()
	byCap := entryByCapability(snap)
	require.Equal(t, "emb-active", byCap[string(airouter.CapabilityEmbedding)].PrimaryProvider)
}

// --- Scenario: 无 provider 的路由跳过 ---

func TestRunStartupProbe_RouteWithoutProvidersSkipped(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	seedRoute(t, db, "lonely", string(airouter.CapabilitySummary), true) // enabled, no bindings
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	seedBinding(t, db, er.ID, emb.ID, 1)

	useFakeProbe(t, reachableByName("emb-main"))

	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	for _, r := range snap.Routes {
		require.NotEqual(t, "lonely", r.RouteName, "route without providers must not produce an entry")
	}
}

// --- Scenario: 缺 embedding 则不健康 ---

func TestRunStartupProbe_MissingEmbedding_NotHealthy(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	useFakeProbe(t, reachableByName("llm-main"))

	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	require.False(t, snap.Healthy, "no reachable embedding route -> not healthy")
}

// --- Scenario: 缺 llm 则不健康 ---

func TestRunStartupProbe_MissingLLM_NotHealthy(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	seedBinding(t, db, er.ID, emb.ID, 1)

	useFakeProbe(t, reachableByName("emb-main"))

	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	require.False(t, snap.Healthy, "no reachable llm route -> not healthy")
}

// --- Scenario: 未配置任何 enabled+provider 路由不健康 ---

func TestRunStartupProbe_NoRoutes_NotHealthy(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	seedRoute(t, db, "default", string(airouter.CapabilitySummary), false) // disabled

	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	require.False(t, snap.Healthy)
	require.Empty(t, snap.Routes)
	require.NotNil(t, snap.CheckedAt, "probe completes (not probing) even with no routes")
}

// --- Scenario: 健康判定宽松（digest_polish 不通不推翻整体）---

func TestRunStartupProbe_LenientHealth_NonCriticalDown(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	dp := seedProvider(t, db, "dp-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	dr := seedRoute(t, db, "default", string(airouter.CapabilityDigestPolish), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)
	seedBinding(t, db, dr.ID, dp.ID, 1)

	// emb + summary reachable; digest_polish NOT reachable.
	useFakeProbe(t, reachableByName("emb-main", "llm-main"))

	RunStartupProbe(context.Background(), store, false)

	snap := GetSnapshot()
	require.True(t, snap.Healthy, "digest_polish down must not break overall health")
	// disambiguate the two "default" routes by provider name.
	var dpEntry RouteHealth
	for _, e := range snap.Routes {
		if e.PrimaryProvider == "dp-main" {
			dpEntry = e
		}
	}
	require.False(t, dpEntry.Reachable, "digest_polish entry still recorded as down")
}

// --- Scenario: 总开关关时不拉起 ---

func TestRunStartupProbe_AutoStartOff_NoLaunch(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	// embedding primary unreachable, has start_command, but autoStart=false.
	emb := seedProvider(t, db, "emb-local", "embedding", "llama-server --port 8081", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	launchCalled := 0
	useFakeProbe(t, reachableByName("llm-main")) // emb NOT reachable
	useFakeLaunch(t, func(string) error { launchCalled++; return nil })

	RunStartupProbe(context.Background(), store, false)

	require.Equal(t, 0, launchCalled, "autoStart=false must not launch")
	snap := GetSnapshot()
	var embEntry RouteHealth
	for _, e := range snap.Routes {
		if e.PrimaryProvider == "emb-local" {
			embEntry = e
		}
	}
	require.False(t, embEntry.Reachable)
	require.False(t, embEntry.LaunchedByBackend)
}

// --- Scenario: 无 start_command 的 provider 不被拉起（autoStart=true）---

func TestRunStartupProbe_AutoStartOn_NoStartCommand_NoLaunch(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-remote", "embedding", "", true) // no start_command
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	launchCalled := 0
	useFakeProbe(t, reachableByName("llm-main")) // emb NOT reachable
	useFakeLaunch(t, func(string) error { launchCalled++; return nil })

	RunStartupProbe(context.Background(), store, true)

	require.Equal(t, 0, launchCalled, "provider without start_command must not be launched")
}

// --- Scenario: 总开关开且不通时拉起并复测 ---

func TestRunStartupProbe_AutoStartOn_Unreachable_LaunchAndReprobe(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-local", "embedding", "llama-server --port 8081", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	embUp := false // flips to reachable only AFTER launch runs.
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		if p.Name == "emb-local" {
			if embUp {
				return true, ""
			}
			return false, "connection refused"
		}
		return true, "" // llm reachable
	})
	launchCalls := 0
	useFakeLaunch(t, func(string) error {
		launchCalls++
		embUp = true // simulate the local process coming up
		return nil
	})

	RunStartupProbe(context.Background(), store, true)

	require.Equal(t, 1, launchCalls, "launch invoked exactly once")
	snap := GetSnapshot()
	require.True(t, snap.Healthy)
	require.True(t, snap.AutoStart)
	var embEntry RouteHealth
	for _, e := range snap.Routes {
		if e.PrimaryProvider == "emb-local" {
			embEntry = e
		}
	}
	require.True(t, embEntry.Reachable, "after launch+reprobe embedding becomes reachable")
	require.True(t, embEntry.LaunchedByBackend, "entry flagged as launched by backend")
}

// --- Scenario: 已可达则不重复拉起 ---

func TestRunStartupProbe_Reachable_NoLaunch(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-local", "embedding", "llama-server --port 8081", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	launchCalled := 0
	useFakeProbe(t, reachableByName("emb-main", "emb-local", "llm-main")) // emb already reachable
	useFakeLaunch(t, func(string) error { launchCalled++; return nil })

	RunStartupProbe(context.Background(), store, true)

	require.Equal(t, 0, launchCalled, "already reachable must not launch")
	snap := GetSnapshot()
	for _, e := range snap.Routes {
		if e.PrimaryProvider == "emb-local" {
			require.False(t, e.LaunchedByBackend)
		}
	}
}

// --- Scenario: 进程生命周期不被托管（不 Wait、不记 PID）---
// Structural: launchFn is fire-and-forget; RouteHealth carries no PID field.
// This test asserts the fake launch is called once and returns immediately
// without blocking RunStartupProbe.

func TestRunStartupProbe_LaunchIsFireAndForget(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-local", "embedding", "llama-server --port 8081", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	embUp := false
	useFakeProbe(t, func(ctx context.Context, p models.AIProvider) (bool, string) {
		if p.Name == "emb-local" {
			if embUp {
				return true, ""
			}
			return false, "down"
		}
		return true, ""
	})
	done := make(chan struct{})
	useFakeLaunch(t, func(string) error {
		embUp = true
		close(done)
		return nil
	})

	start := time.Now()
	RunStartupProbe(context.Background(), store, true)
	elapsed := time.Since(start)

	// launch returned immediately (no Wait); RunStartupProbe must not block on it.
	select {
	case <-done:
	default:
		t.Fatal("launch was not invoked")
	}
	require.Less(t, elapsed, 5*time.Second, "RunStartupProbe must not wait on the launched process")

	// No PID is stored anywhere in the snapshot (RouteHealth has no PID field by design).
	snap := GetSnapshot()
	for _, e := range snap.Routes {
		_ = e // struct has no PID member; lifecycle not managed.
	}
}

// --- Scenario: 启动竞态期分析不跑 / 检测完成后快照就绪 ---

func TestHealthy_NotReadyThenReady(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	emb := seedProvider(t, db, "emb-main", "embedding", "", true)
	sum := seedProvider(t, db, "llm-main", "llm", "", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	sr := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, er.ID, emb.ID, 1)
	seedBinding(t, db, sr.ID, sum.ID, 1)

	// Not ready: Healthy() false, CheckedAt nil.
	require.False(t, Healthy())
	require.Nil(t, GetSnapshot().CheckedAt)

	useFakeProbe(t, reachableByName("emb-main", "llm-main"))
	RunStartupProbe(context.Background(), store, false)

	require.NotNil(t, GetSnapshot().CheckedAt)
	require.True(t, Healthy())

	// Reset back to not-ready simulates a fresh boot; Healthy() must fall to false again.
	resetSnapshot()
	require.False(t, Healthy())
	require.Nil(t, GetSnapshot().CheckedAt)
}

// --- pollProbe timeout branch (fast): probe stays unreachable -> returns false ---

func TestPollProbe_TimesOutWhenNeverReachable(t *testing.T) {
	resetSnapshot()
	useFakeProbe(t, func(context.Context, models.AIProvider) (bool, string) {
		return false, "still down"
	})
	reachable, errMsg := pollProbe(context.Background(), models.AIProvider{Name: "x"},
		60*time.Millisecond, 20*time.Millisecond)
	require.False(t, reachable)
	require.NotEmpty(t, errMsg)
}

// --- Scenario: ListRoutes 瞬态失败时重试（不一次焊死健康门） ---

// closeUnderlyingDB closes the gorm.DB's underlying *sql.DB so every later
// query (incl. store.ListRoutes) errors. Simulates a transient DB connection
// failure (socket exhaustion / port conflict) hitting the startup probe.
func closeUnderlyingDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// useFastListRoutesRetries swaps the package retry knobs to tiny values for a
// fast test and restores them on cleanup.
func useFastListRoutesRetries(t *testing.T, retries int, interval time.Duration) {
	t.Helper()
	oldRetries, oldInterval := listRoutesMaxRetries, listRoutesRetryInterval
	listRoutesMaxRetries = retries
	listRoutesRetryInterval = interval
	t.Cleanup(func() {
		listRoutesMaxRetries = oldRetries
		listRoutesRetryInterval = oldInterval
	})
}

// TestRunStartupProbe_ListRoutesFails_Retries verifies a ListRoutes error is
// retried (listRoutesMaxRetries attempts with listRoutesRetryInterval backoff)
// rather than welding the snapshot to unhealthy on the first transient error.
func TestRunStartupProbe_ListRoutesFails_Retries(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	closeUnderlyingDB(t, db) // force ListRoutes to error on every attempt

	// 2 attempts -> 1 backoff sleep of 10ms between them (~10ms total).
	useFastListRoutesRetries(t, 2, 10*time.Millisecond)

	start := time.Now()
	RunStartupProbe(context.Background(), store, false)
	elapsed := time.Since(start)

	// Retried: elapsed must reflect the single inter-attempt backoff sleep.
	require.GreaterOrEqual(t, elapsed, 9*time.Millisecond,
		"ListRoutes failure must be retried (expected ~1 backoff sleep)")
	require.Less(t, elapsed, 1*time.Second, "retry backoff must stay bounded")

	snap := GetSnapshot()
	require.False(t, snap.Healthy, "persistent ListRoutes failure -> not healthy")
	require.NotNil(t, snap.CheckedAt, "probe still completes (stamped not-healthy snapshot)")
}

// TestRunStartupProbe_ListRoutesFails_NoRetryWhenMaxOne verifies the retry
// knob is honored: with retries=1 a single failure is NOT followed by any
// backoff sleep, so the probe returns near-instantly (contrast with the
// retries=2 test above).
func TestRunStartupProbe_ListRoutesFails_NoRetryWhenMaxOne(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	closeUnderlyingDB(t, db)

	useFastListRoutesRetries(t, 1, 10*time.Millisecond)

	start := time.Now()
	RunStartupProbe(context.Background(), store, false)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 9*time.Millisecond,
		"retries=1 must not sleep between attempts (no backoff)")
	require.False(t, GetSnapshot().Healthy)
}

// --- pollProbe success branch: probe flips reachable on retry ---

func TestPollProbe_ReachableOnRetry(t *testing.T) {
	resetSnapshot()
	attempts := 0
	useFakeProbe(t, func(context.Context, models.AIProvider) (bool, string) {
		attempts++
		if attempts >= 2 {
			return true, ""
		}
		return false, "warming up"
	})
	reachable, _ := pollProbe(context.Background(), models.AIProvider{Name: "x"},
		2*time.Second, 10*time.Millisecond)
	require.True(t, reachable)
}
