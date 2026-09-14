// Package aihealth probes the reachability of each AI route's primary provider
// at backend startup and keeps an in-memory snapshot that the analysis pause
// gate (and later the health API) reads to decide whether the model layer is
// ready. Optionally it can fire-and-forget a provider's start_command to bring
// a local model process up before re-probing. A background heartbeat timer
// (StartPeriodicReprobe) keeps probing regardless of the snapshot state, so a
// mid-session provider death (e.g. the AI host powering off) degrades the
// snapshot after degradeFailures consecutive failures.
package aihealth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// RouteHealth records the reachability of one route's primary (highest
// priority) provider at probe time.
type RouteHealth struct {
	RouteName         string
	Capability        string
	PrimaryProvider   string
	ModelKind         string
	Reachable         bool
	LaunchedByBackend bool
	LastCheckedAt     time.Time
	Error             string
}

// Snapshot is the process-local health snapshot. It is rebuilt on every probe
// and never persisted. CheckedAt==nil means "first probe not finished yet"
// (probing), during which Healthy is treated as false so analysis workers do
// not lease during the startup race.
type Snapshot struct {
	Healthy   bool
	CheckedAt *time.Time
	Routes    []RouteHealth
	AutoStart bool
}

const (
	probePollTimeout  = 45 * time.Second
	probePollInterval = 2 * time.Second
)

// reprobeInterval is the period of the background health heartbeat timer
// (StartPeriodicReprobe). The heartbeat re-probes unconditionally — healthy
// state included — so provider death is detected within a couple of ticks and
// recovery needs a single successful probe. Package-level so tests can shrink
// it.
var reprobeInterval = 60 * time.Second

// degradeFailures is how many consecutive unhealthy probe verdicts it takes to
// degrade a healthy snapshot to not healthy (debounce). Fast-refused failures
// (host powered off, latency ≈0) each count one heartbeat, so the worst-case
// degrade delay is ≈ degradeFailures × reprobeInterval. Timeout-shaped
// failures are already stretched by the per-provider probe timeout
// (provider.timeout_seconds, see airouter.TestConnection): a slow-but-alive
// server answers /models within its own call timeout and never counts as a
// failure, so no extra busy-tolerance window is needed on top. Package-level
// so tests can shrink it.
var degradeFailures = 2

var (
	snapshotMu sync.RWMutex
	// current starts not-ready (CheckedAt==nil) so Healthy() returns false
	// during the startup race window before the first probe completes.
	current = &Snapshot{}

	// probeMu serializes RunStartupProbe runs. Reprobe triggers (backend
	// startup + every analysis resume) that fire while a probe is already in
	// flight are skipped instead of piling up concurrent probes that could
	// each fire start_command.
	probeMu sync.Mutex

	// launchCooldown suppresses re-executing a provider's start_command within
	// this window after a successful launch. A slow-loading model stays
	// unreachable past the 45s poll window; without the cooldown every reprobe
	// (each resume click) would spawn another copy of the process. Package
	// level so tests can shrink it.
	launchCooldown = 10 * time.Minute

	launchMu     sync.Mutex
	lastLaunchAt = map[uint]time.Time{}

	// failStreak counts consecutive unhealthy probe verdicts observed while
	// the snapshot was healthy on entry — it is the degrade debounce. Guarded
	// by probeMu (only mutated inside runProbeLocked). Reset by resetSnapshot
	// so tests never inherit a previous test's streak.
	failStreak int

	// listRoutesMaxRetries / listRoutesRetryInterval tune the retry loop around
	// store.ListRoutes in RunStartupProbe. They are package-level so tests can
	// shrink them (see aihealth_test.go) to avoid real sleeps. Defaults: 3
	// attempts, ~2s backoff — survives transient DB hiccups (socket exhaustion
	// / port conflicts) at startup without welding the health gate shut on a
	// single error.
	listRoutesMaxRetries    = 3
	listRoutesRetryInterval = 2 * time.Second
)

// Healthy reports whether the AI model layer is ready. It returns false
// conservatively while the snapshot is not yet ready (CheckedAt==nil).
func Healthy() bool {
	snapshotMu.RLock()
	defer snapshotMu.RUnlock()
	if current == nil || current.CheckedAt == nil {
		return false
	}
	return current.Healthy
}

// GetSnapshot returns a copy of the current snapshot. When not yet ready it
// returns {Healthy:false, CheckedAt:nil}.
func GetSnapshot() Snapshot {
	snapshotMu.RLock()
	defer snapshotMu.RUnlock()
	if current == nil || current.CheckedAt == nil {
		return Snapshot{Healthy: false, CheckedAt: nil}
	}
	return *current
}

func setSnapshot(s Snapshot) {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	current = &s
}

// applyProbeVerdict writes the freshly computed probe verdict into the
// snapshot with degrade debounce: a healthy verdict always lands (streak
// resets), while an unhealthy verdict only degrades the snapshot after
// degradeFailures consecutive failures. Within the debounce window the
// snapshot keeps its previous overall Healthy=true verdict but still updates
// CheckedAt and the per-route details, so the health API shows the fresh
// (partially unreachable) details while the pause gate stays open. A snapshot
// that was already not healthy (or not yet ready) lands not-healthy directly —
// the debounce only protects the healthy→not-healthy transition. The caller
// MUST hold probeMu.
func applyProbeVerdict(healthy bool, checkedAt time.Time, entries []RouteHealth, autoStart bool) {
	prev := GetSnapshot()
	next := Snapshot{
		Healthy:   healthy,
		CheckedAt: &checkedAt,
		Routes:    entries,
		AutoStart: autoStart,
	}
	switch {
	case healthy:
		failStreak = 0
	case prev.CheckedAt != nil && prev.Healthy:
		failStreak++
		if failStreak < degradeFailures {
			next.Healthy = true
		}
	default:
		// Not-ready startup race or already not-healthy: no debounce meaning.
		failStreak = 0
	}
	setSnapshot(next)
}

// listRoutesWithRetry calls store.ListRoutes with a bounded retry/backoff so a
// single transient DB error (socket exhaustion / port conflict at startup)
// does not weld the health gate shut. It returns the routes on the first
// success, or the last error once all attempts are exhausted.
func listRoutesWithRetry(store *airouter.Store) ([]models.AIRoute, error) {
	var routes []models.AIRoute
	var lastErr error
	for attempt := 1; attempt <= listRoutesMaxRetries; attempt++ {
		routes, lastErr = store.ListRoutes()
		if lastErr == nil {
			return routes, nil
		}
		logging.Warnf("aihealth: list routes attempt %d/%d failed: %v", attempt, listRoutesMaxRetries, lastErr)
		if attempt < listRoutesMaxRetries {
			time.Sleep(listRoutesRetryInterval)
		}
	}
	return nil, lastErr
}

// SetSnapshotForTest replaces the in-memory snapshot. It is intended only for
// tests in other packages (e.g. analysispause) that need to drive Healthy()
// without running a real probe. Passing the zero Snapshot{...} restores the
// not-ready state (CheckedAt==nil → Healthy()==false). Production code must
// not call this.
func SetSnapshotForTest(s Snapshot) {
	setSnapshot(s)
}

// SetProbeFnForTest replaces the probe seam (probeFn) for tests in other
// packages (e.g. the analysis-pause handler test that drives a reprobe). It
// returns a cleanup func restoring the previous value. Production code must
// not call this.
func SetProbeFnForTest(fn func(context.Context, models.AIProvider) (bool, string)) func() {
	old := probeFn
	probeFn = fn
	return func() { probeFn = old }
}

// probeFn is the seam that turns a provider into a (reachable, error) verdict.
// Tests replace it to avoid real network. The default delegates to
// airouter.TestConnection (GET {base_url}/models, zero tokens).
var probeFn = defaultProbe

// launchFn is the seam that fire-and-forgets a start_command. The default is a
// cross-platform detached exec (see launch_windows.go / launch_unix.go).
var launchFn = defaultLaunch

func defaultProbe(ctx context.Context, p models.AIProvider) (bool, string) {
	_, err := airouter.TestConnection(ctx, p)
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

// recentlyLaunched reports whether this provider was successfully launched
// within the launch cooldown window (i.e. a process we started is presumably
// still warming up).
func recentlyLaunched(providerID uint) bool {
	launchMu.Lock()
	defer launchMu.Unlock()
	last, ok := lastLaunchAt[providerID]
	return ok && time.Since(last) < launchCooldown
}

// recordLaunch marks a successful launch of the provider's start_command so
// later probes within launchCooldown skip re-launching it. Called only after
// launchFn succeeds; concurrent runs are prevented by probeMu and within-run
// duplicates by the per-provider outcome cache, so check-then-record is safe.
func recordLaunch(providerID uint) {
	launchMu.Lock()
	defer launchMu.Unlock()
	lastLaunchAt[providerID] = time.Now()
}

// RunStartupProbe probes the primary provider of every enabled route that has
// at least one provider bound, optionally auto-launching unreachable local
// providers, then writes the result into the in-memory snapshot. It is meant
// to run once at backend startup and again on analysis resume. Runs are
// serialized: a reprobe fired while one is in flight returns immediately.
func RunStartupProbe(ctx context.Context, store *airouter.Store, autoStart bool) {
	if !probeMu.TryLock() {
		logging.Infof("aihealth: a probe is already in flight; skip this reprobe")
		return
	}
	defer probeMu.Unlock()
	runProbeLocked(ctx, store, autoStart)
}

// TryStartProbe starts one probe run asynchronously (like RunStartupProbe but
// without blocking the caller) and reports whether a probe actually started.
// It returns false when another probe is already in flight (skipped, no
// queuing). Used by the background reprobe timer and the manual reprobe API so
// both share the same in-flight semantics with the startup probe.
func TryStartProbe(ctx context.Context, store *airouter.Store, autoStart bool) bool {
	if !probeMu.TryLock() {
		return false
	}
	go func() {
		defer probeMu.Unlock()
		runProbeLocked(ctx, store, autoStart)
	}()
	return true
}

// StartPeriodicReprobe runs the background heartbeat timer: it re-probes at
// reprobeInterval unconditionally — healthy state included. The heartbeat is
// what detects a mid-session provider death (e.g. the AI host powering off):
// after degradeFailures consecutive failed probes the snapshot degrades to
// not healthy and the analysis pause gate closes; a single successful probe
// restores healthy. autoStartFn is re-evaluated on every tick so a mid-run
// change of the auto_start_models switch takes effect without restart. The
// timer stops when ctx is cancelled (backend shutdown).
func StartPeriodicReprobe(ctx context.Context, store *airouter.Store, autoStartFn func() bool) {
	ticker := time.NewTicker(reprobeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			TryStartProbe(ctx, store, autoStartFn())
		}
	}
}

// runProbeLocked executes one full probe pass. The caller MUST hold probeMu
// (acquired via RunStartupProbe or TryStartProbe).
func runProbeLocked(ctx context.Context, store *airouter.Store, autoStart bool) {
	now := time.Now()

	routes, listErr := listRoutesWithRetry(store)
	if listErr != nil {
		logging.Errorf("aihealth: list routes failed after %d attempts: %v", listRoutesMaxRetries, listErr)
		setSnapshot(Snapshot{Healthy: false, CheckedAt: &now, AutoStart: autoStart})
		return
	}

	// probeOutcome is the per-provider verdict reused by every route whose
	// primary is that provider, so a provider shared by several routes is
	// probed — and potentially launched — at most once per run.
	type probeOutcome struct {
		reachable bool
		errMsg    string
		launched  bool
	}
	outcomes := make(map[uint]probeOutcome)

	embeddingCap := string(airouter.CapabilityEmbedding)
	entries := make([]RouteHealth, 0)
	for _, route := range routes {
		if !route.Enabled || len(route.RouteProviders) == 0 {
			continue
		}
		// RouteProviders are preloaded ordered by priority ASC. Skip links that
		// are disabled (or whose provider is disabled), matching
		// LoadRouteWithProviders semantics, and take the first passing link as
		// the primary (highest precedence) provider. Only the primary is
		// probed; fallbacks are intentionally skipped (spec: 仅探主 provider).
		// A route whose links are all disabled produces no entry, same as a
		// route without any provider.
		var primary models.AIProvider
		found := false
		for _, link := range route.RouteProviders {
			if !link.Enabled || !link.Provider.Enabled {
				continue
			}
			primary = link.Provider
			found = true
			break
		}
		if !found {
			continue
		}

		outcome, cached := outcomes[primary.ID]
		if !cached {
			reachable, errMsg := probeFn(ctx, primary)
			launched := false
			if !reachable && autoStart && strings.TrimSpace(primary.StartCommand) != "" {
				if recentlyLaunched(primary.ID) {
					// A process we launched is still warming up (model loads can
					// outlast the 45s poll window). Do not spawn another copy —
					// just keep waiting on the one already started.
					logging.Infof("aihealth: provider %q launched within %s; skip re-launch, keep polling",
						primary.Name, launchCooldown)
					reachable, errMsg = pollProbe(ctx, primary, probePollTimeout, probePollInterval)
					launched = reachable
				} else if lerr := launchFn(primary.StartCommand); lerr != nil {
					logging.Errorf("aihealth: launch start_command for provider %q failed: %v", primary.Name, lerr)
					errMsg = fmt.Sprintf("launch failed: %v", lerr)
				} else {
					recordLaunch(primary.ID)
					reachable, errMsg = pollProbe(ctx, primary, probePollTimeout, probePollInterval)
					if reachable {
						launched = true
					}
				}
			}
			outcome = probeOutcome{reachable: reachable, errMsg: errMsg, launched: launched}
			outcomes[primary.ID] = outcome
		}

		entries = append(entries, RouteHealth{
			RouteName:         route.Name,
			Capability:        route.Capability,
			PrimaryProvider:   primary.Name,
			ModelKind:         primary.ModelKind,
			Reachable:         outcome.reachable,
			LaunchedByBackend: outcome.launched,
			LastCheckedAt:     time.Now(),
			Error:             outcome.errMsg,
		})
	}

	// Lenient health (spec 健康就绪判定宽松): at least one reachable
	// embedding-capability route AND at least one reachable non-embedding
	// (llm-class) route. Classification is by route capability, not provider
	// model_kind.
	healthy := hasReachableCapability(entries, embeddingCap) &&
		hasReachableCapabilityOtherThan(entries, embeddingCap)

	applyProbeVerdict(healthy, now, entries, autoStart)
}

func hasReachableCapability(entries []RouteHealth, want string) bool {
	for _, e := range entries {
		if e.Capability == want && e.Reachable {
			return true
		}
	}
	return false
}

func hasReachableCapabilityOtherThan(entries []RouteHealth, exclude string) bool {
	for _, e := range entries {
		if e.Capability != exclude && e.Reachable {
			return true
		}
	}
	return false
}

// pollProbe repeatedly probes the provider at interval until it is reachable or
// upTo elapses. The first probe runs immediately so a freshly-launched process
// that is already up resolves without waiting.
func pollProbe(ctx context.Context, p models.AIProvider, upTo, interval time.Duration) (bool, string) {
	pollCtx, cancel := context.WithTimeout(ctx, upTo)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr string
	for {
		reachable, errMsg := probeFn(pollCtx, p)
		lastErr = errMsg
		if reachable {
			return true, ""
		}
		select {
		case <-pollCtx.Done():
			return false, lastErr
		case <-ticker.C:
		}
	}
}
