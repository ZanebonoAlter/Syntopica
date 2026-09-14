// Package analysispause_test contains the end-to-end compose proof for the AI
// model health gate (openspec change ai-model-health-gate, task 3.6):
//
//	probe 通过 ⇒ aihealth.Healthy()==true ⇒ analysispause.IsPaused()==false ⇒
//	scheduler.PauseAware 放行（job 实际执行）
//	probe 失败 ⇒ aihealth.Healthy()==false ⇒ analysispause.IsPaused()==true ⇒
//	scheduler.PauseAware 跳过（job 不跑，摘要含 model_unhealthy）
//
// It lives in the external test package so the test binary can import
// aihealth/airouter/scheduler without creating an import cycle with
// analysispause's production import of aihealth.
//
// NOTE on the probe seam: aihealth.probeFn is an unexported package variable
// with no exported setter, so an external test package cannot swap in a fake
// probe. Instead these tests drive RunStartupProbe through its REAL default
// probe (airouter.TestConnection) against local httptest mock provider
// endpoints — exactly the "mock provider 端点" wording of task 3.6 — which
// exercises TestConnection itself and requires no product-code change.
package analysispause_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/admin/scheduler"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/analysispause"
	"syntopica-backend/internal/platform/testutil"
)

// resetHealthForTest forces the process-global aihealth snapshot back to the
// not-ready state and restores it on cleanup, so the snapshot never leaks into
// a sibling test (tests in this package and in gate_test.go all share the
// process-global).
func resetHealthForTest(t *testing.T) {
	t.Helper()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })
}

// mockProviderServer returns an httptest server answering GET /models with a
// valid OpenAI-style model list — enough for airouter.TestConnection (the real
// probe) to report reachable.
func mockProviderServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// closedProviderURL returns a base URL that refuses connections: a server is
// started and immediately closed, so the port is guaranteed not listening and
// TestConnection fails fast with connection refused.
func closedProviderURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	return srv.URL
}

// seedEnabledRoute upserts an enabled provider of the given model_kind bound as
// the primary (priority 1) link of an enabled route with the given capability.
// UpsertProvider/UpsertRoute are the production paths, so link priority and the
// model_kind binding check are exercised for real.
func seedEnabledRoute(t *testing.T, store *airouter.Store, capability, providerName, baseURL, modelKind string) {
	t.Helper()
	p := models.AIProvider{
		Name:         providerName,
		ProviderType: airouter.ProviderTypeOpenAICompatible,
		BaseURL:      baseURL,
		Model:        providerName + "-model",
		APIKey:       "test-key",
		Enabled:      true,
		ModelKind:    modelKind,
	}
	require.NoError(t, store.UpsertProvider(&p))
	require.NoError(t, store.UpsertRoute(&models.AIRoute{
		Name:       "default",
		Capability: capability,
		Enabled:    true,
	}, []uint{p.ID}))
}

// TestHealthGate_ProbeComposesWithPauseGate is the happy-path compose proof:
// the startup race (snapshot not ready) pauses analysis; after RunStartupProbe
// finds both an embedding and an llm route reachable (real HTTP against local
// mock endpoints), the gate opens and PauseAware passes the job through.
func TestHealthGate_ProbeComposesWithPauseGate(t *testing.T) {
	resetHealthForTest(t)
	db := testutil.SetupTestDB(t)
	store := airouter.NewStore(db)

	// analysis_paused is not seeded, so the user switch defaults to false.
	require.False(t, analysispause.UserPaused())

	seedEnabledRoute(t, store, string(airouter.CapabilityEmbedding), "emb-test", mockProviderServer(t).URL, "embedding")
	seedEnabledRoute(t, store, string(airouter.CapabilitySummary), "llm-test", mockProviderServer(t).URL, "llm")

	// Startup race: before the first probe completes the snapshot is not ready,
	// so Healthy()==false and the effective pause is ON (fail-closed).
	require.False(t, aihealth.Healthy())
	require.True(t, analysispause.IsPaused())

	// Probe both providers (both reachable) and let the verdict land in the
	// in-memory snapshot.
	aihealth.RunStartupProbe(context.Background(), store, false)

	require.True(t, aihealth.Healthy())
	require.False(t, analysispause.IsPaused())
	require.Equal(t, "", analysispause.PauseReason())

	// PauseAware must run the wrapped job unchanged.
	var ran int32
	result, err := scheduler.PauseAware(func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&ran, 1)
		return &scheduler.JobResult{Summary: "real job ran"}, nil
	})(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 1, atomic.LoadInt32(&ran), "job must run when the health gate is open")
	require.Equal(t, "real job ran", result.Summary)
}

// TestHealthGate_ProbeUnhealthy_PauseAwareSkips is the closed-gate compose
// proof: with both providers unreachable, RunStartupProbe marks the snapshot
// unhealthy, IsPaused flips to true with reason model_unhealthy, and
// PauseAware skips the wrapped job.
func TestHealthGate_ProbeUnhealthy_PauseAwareSkips(t *testing.T) {
	resetHealthForTest(t)
	db := testutil.SetupTestDB(t)
	store := airouter.NewStore(db)

	require.False(t, analysispause.UserPaused())

	unreachable := closedProviderURL(t)
	seedEnabledRoute(t, store, string(airouter.CapabilityEmbedding), "emb-test", unreachable, "embedding")
	seedEnabledRoute(t, store, string(airouter.CapabilitySummary), "llm-test", unreachable, "llm")

	aihealth.RunStartupProbe(context.Background(), store, false)

	require.False(t, aihealth.Healthy())
	require.True(t, analysispause.IsPaused())
	require.Equal(t, "model_unhealthy", analysispause.PauseReason())

	// PauseAware must skip the wrapped job: it never runs, the result is a
	// benign success whose summary carries model_unhealthy.
	var ran int32
	result, err := scheduler.PauseAware(func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&ran, 1)
		return &scheduler.JobResult{Summary: "real job ran"}, nil
	})(context.Background())

	require.NoError(t, err, "skipped result must be a success (err=nil)")
	require.EqualValues(t, 0, atomic.LoadInt32(&ran), "real job must NOT run while the health gate is closed")
	require.NotNil(t, result)
	require.Contains(t, result.Summary, "skipped")
	require.Contains(t, result.Summary, "model_unhealthy", "summary should flag the health reason")
	require.Equal(t, "paused", result.Data["skipped"])
}

// TestHealthGate_HeartbeatDegrade_PauseAwareSkips is the heartbeat-degrade
// compose proof (change ai-health-heartbeat-reprobe): a healthy snapshot
// whose providers die mid-session (real connection refused after the mock
// servers shut down) stays healthy through ONE failing probe (debounce),
// degrades on the second, and only then does the pause gate close and
// PauseAware skip. This mirrors the 常驻 topology: PC powers off mid-run.
func TestHealthGate_HeartbeatDegrade_PauseAwareSkips(t *testing.T) {
	resetHealthForTest(t)
	db := testutil.SetupTestDB(t)
	store := airouter.NewStore(db)

	require.False(t, analysispause.UserPaused())

	// Two live mock servers establish a healthy snapshot.
	seedEnabledRoute(t, store, string(airouter.CapabilityEmbedding), "emb-test", mockProviderServer(t).URL, "embedding")
	seedEnabledRoute(t, store, string(airouter.CapabilitySummary), "llm-test", mockProviderServer(t).URL, "llm")

	aihealth.RunStartupProbe(context.Background(), store, false)
	require.True(t, aihealth.Healthy(), "baseline: both mock providers reachable")
	require.False(t, analysispause.IsPaused())

	// Kill both endpoints: connection refused from here on (真实秒拒型失败).
	killProviders(t, store)

	// Failure #1: debounce window keeps the overall verdict healthy.
	aihealth.RunStartupProbe(context.Background(), store, false)
	require.True(t, aihealth.Healthy(), "first failing probe must stay inside the debounce window")
	require.False(t, analysispause.IsPaused(), "pause gate must stay open during the debounce window")

	// Failure #2: degrades -> gate closes -> PauseAware skips.
	aihealth.RunStartupProbe(context.Background(), store, false)
	require.False(t, aihealth.Healthy(), "second consecutive failing probe must degrade")
	require.True(t, analysispause.IsPaused())
	require.Equal(t, "model_unhealthy", analysispause.PauseReason())

	var ran int32
	result, err := scheduler.PauseAware(func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&ran, 1)
		return &scheduler.JobResult{Summary: "real job ran"}, nil
	})(context.Background())

	require.NoError(t, err)
	require.EqualValues(t, 0, atomic.LoadInt32(&ran), "job must NOT run after heartbeat degrade")
	require.Contains(t, result.Summary, "model_unhealthy")
	require.Equal(t, "paused", result.Data["skipped"])
}

// killProviders shuts down every enabled provider endpoint by pointing its
// base_url at a guaranteed-dead port (server started then closed), so the next
// real probe fails fast with connection refused — the actual failure shape of
// a powered-off AI host.
func killProviders(t *testing.T, store *airouter.Store) {
	t.Helper()
	dead := closedProviderURL(t)
	providers, err := store.ListProviders()
	require.NoError(t, err)
	for _, p := range providers {
		p.BaseURL = dead
		require.NoError(t, store.UpsertProvider(&p))
	}
}

// TestHealthGate_SlowProviderWithinTimeout_Healthy pins the busy-tolerance
// compose proof: a provider whose /models answers slowly (but well within its
// probe timeout) counts as reachable, NOT as a failure — the debounce logic
// never even sees it. 慢而活着的服务器不计失败（忙容忍由 provider 超时提供）.
func TestHealthGate_SlowProviderWithinTimeout_Healthy(t *testing.T) {
	resetHealthForTest(t)
	db := testutil.SetupTestDB(t)
	store := airouter.NewStore(db)

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(300 * time.Millisecond) // slow but alive; probe timeout defaults to 15s
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	t.Cleanup(slow.Close)

	seedEnabledRoute(t, store, string(airouter.CapabilityEmbedding), "emb-slow", slow.URL, "embedding")
	seedEnabledRoute(t, store, string(airouter.CapabilitySummary), "llm-slow", slow.URL, "llm")

	// Two consecutive probes against the slow-but-alive endpoints: both must
	// count as successes (no failures, no degrade) — even back-to-back.
	aihealth.RunStartupProbe(context.Background(), store, false)
	require.True(t, aihealth.Healthy(), "slow-but-alive provider within timeout counts as reachable")
	require.False(t, analysispause.IsPaused())
	aihealth.RunStartupProbe(context.Background(), store, false)
	require.True(t, aihealth.Healthy(), "repeated slow-but-alive probes must never degrade")
}

// TestHealthGate_EmbeddingUp_LLMDown_NotHealthy pins the lenient-health
// boundary in the compose chain: embedding reachable alone is NOT healthy
// (缺 llm → Healthy=false), so the pause gate stays closed.
func TestHealthGate_EmbeddingUp_LLMDown_NotHealthy(t *testing.T) {
	resetHealthForTest(t)
	db := testutil.SetupTestDB(t)
	store := airouter.NewStore(db)

	require.False(t, analysispause.UserPaused())

	seedEnabledRoute(t, store, string(airouter.CapabilityEmbedding), "emb-test", mockProviderServer(t).URL, "embedding")
	seedEnabledRoute(t, store, string(airouter.CapabilitySummary), "llm-test", closedProviderURL(t), "llm")

	aihealth.RunStartupProbe(context.Background(), store, false)

	snap := aihealth.GetSnapshot()
	require.False(t, snap.Healthy, "missing a reachable llm route -> not healthy")
	require.False(t, aihealth.Healthy())
	require.True(t, analysispause.IsPaused())

	var ran int32
	result, err := scheduler.PauseAware(func(ctx context.Context) (*scheduler.JobResult, error) {
		atomic.AddInt32(&ran, 1)
		return &scheduler.JobResult{Summary: "real job ran"}, nil
	})(context.Background())

	require.NoError(t, err, "skipped result must be a success (err=nil)")
	require.EqualValues(t, 0, atomic.LoadInt32(&ran), "real job must NOT run while the health gate is closed")
	require.Contains(t, result.Summary, "model_unhealthy")
	require.Equal(t, "paused", result.Data["skipped"])
}
