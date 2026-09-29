package aihealth

// Regression tests for duplicate process launches: a slow-starting local
// model (unreachable through the whole 45s poll window) used to be re-launched
// once per route sharing its vendor and once per reprobe (every resume
// click), stacking processes. Poll windows are cut short via a cancelling
// context instead of waiting out the real 45s.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
)

// neverReachableProbe builds a probe fake that is never reachable (model still
// loading) and counts probe calls.
func neverReachableProbe(counter *int) func(context.Context, models.AIVendor) (bool, string) {
	return func(context.Context, models.AIVendor) (bool, string) {
		if counter != nil {
			*counter++
		}
		return false, "connection refused"
	}
}

func countingLaunch(counter *int) func(string) error {
	return func(string) error { *counter++; return nil }
}

// Scenario: two routes whose primary models belong to the same slow-starting
// vendor — one probe run must launch that vendor exactly once (previously once
// per route). The two routes bind DIFFERENT models of the same vendor, so the
// dedup key really is the vendor, not the model (W11-2).
func TestRunStartupProbe_SharedVendor_LaunchedOncePerRun(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	v := seedVendor(t, db, "local-llm", "start-llm.bat", true)
	embModel := seedModel(t, db, v, "local-emb", "embedding", true)
	llmModel := seedModel(t, db, v, "local-llm-main", "llm", true)
	emb := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	llm := seedRoute(t, db, "default", string(airouter.CapabilitySummary), true)
	seedBinding(t, db, emb.ID, embModel.ID, 1)
	seedBinding(t, db, llm.ID, llmModel.ID, 1)

	useFakeProbe(t, neverReachableProbe(nil))
	launches := 0
	useFakeLaunch(t, countingLaunch(&launches))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	RunStartupProbe(ctx, store, true)

	require.Equal(t, 1, launches, "vendor shared by two routes must be launched once per run")
	snap := GetSnapshot()
	require.Len(t, snap.Routes, 2)
	for _, e := range snap.Routes {
		require.False(t, e.Reachable)
		require.Equal(t, "local-llm", e.Vendor)
	}
}

// Scenario: a reprobe (resume click) fired while the launched process is
// still loading must not spawn a second process — it keeps polling the one
// already started.
func TestRunStartupProbe_CooldownBlocksRelaunch(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	_, m := seedVendorModel(t, db, "local-llm", "llm", "start-llm.bat", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	seedBinding(t, db, er.ID, m.ID, 1)

	probeCalls := 0
	useFakeProbe(t, neverReachableProbe(&probeCalls))
	launches := 0
	useFakeLaunch(t, countingLaunch(&launches))

	oldCooldown := launchCooldown
	launchCooldown = time.Hour // second run stays inside the window
	t.Cleanup(func() { launchCooldown = oldCooldown })

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		RunStartupProbe(ctx, store, true)
		cancel()
	}

	require.Equal(t, 1, launches, "cooldown must suppress the reprobe launch")
	require.Greater(t, probeCalls, 1, "second run must still poll the warming-up provider")
}

// Scenario: after the cooldown expires, a still-unreachable vendor may be
// launched again (a genuinely dead process is recoverable on reprobe).
func TestRunStartupProbe_CooldownExpiry_Relaunches(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	_, m := seedVendorModel(t, db, "local-llm", "llm", "start-llm.bat", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	seedBinding(t, db, er.ID, m.ID, 1)

	useFakeProbe(t, neverReachableProbe(nil))
	launches := 0
	useFakeLaunch(t, countingLaunch(&launches))

	oldCooldown := launchCooldown
	launchCooldown = 50 * time.Millisecond
	t.Cleanup(func() { launchCooldown = oldCooldown })

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		RunStartupProbe(ctx, store, true)
		cancel()
		if i == 0 {
			time.Sleep(60 * time.Millisecond) // let the cooldown expire
		}
	}

	require.Equal(t, 2, launches, "after cooldown expiry a reprobe may launch again")
}

// Scenario: a reprobe fired while another probe is still running is skipped
// entirely — no concurrent probes, no extra launch.
func TestRunStartupProbe_ReentrantRunSkipped(t *testing.T) {
	resetSnapshot()
	db, store := setupTestDB(t)
	_, m := seedVendorModel(t, db, "local-llm", "llm", "start-llm.bat", true)
	er := seedRoute(t, db, "default", string(airouter.CapabilityEmbedding), true)
	seedBinding(t, db, er.ID, m.ID, 1)

	probeCalls := 0
	useFakeProbe(t, neverReachableProbe(&probeCalls))

	probeMu.Lock() // simulate an in-flight probe
	defer probeMu.Unlock()

	RunStartupProbe(context.Background(), store, true)

	require.Zero(t, probeCalls, "skipped reprobe must not probe anything")
	require.Nil(t, GetSnapshot().CheckedAt, "skipped reprobe must not touch the snapshot")
}
