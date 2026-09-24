package analysispause

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/testutil"
)

// setupTestDB initializes database.DB through the same testcontainer helper the
// rest of the codebase uses (testutil.SetupTestDB), exactly like the aisettings
// config-store tests: the pause switch is persisted in ai_settings.analysis_paused.
func setupTestDB(t *testing.T) {
	t.Helper()
	testutil.SetupTestDB(t)
}

// setHealthForTest drives the process-global aihealth snapshot from tests in
// this package, then schedules a reset back to the not-ready state so the
// global never leaks into a sibling test.
func setHealthForTest(t *testing.T, healthy bool) {
	t.Helper()
	now := time.Now()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{Healthy: healthy, CheckedAt: &now})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })
}

// resetHealthForTest puts aihealth into the not-ready (startup-race) state:
// CheckedAt==nil so Healthy() returns false.
func resetHealthForTest(t *testing.T) {
	t.Helper()
	aihealth.SetSnapshotForTest(aihealth.Snapshot{})
	t.Cleanup(func() { aihealth.SetSnapshotForTest(aihealth.Snapshot{}) })
}

// TestIsPaused covers the gate's original contract: after SetPaused(true) the
// gate reports paused, after SetPaused(false) it reports not paused. With the
// health gate wired in, IsPaused is asserted under a healthy model layer so
// this stays a pure user-switch round trip.
func TestIsPaused(t *testing.T) {
	setupTestDB(t)
	setHealthForTest(t, true)

	require.NoError(t, SetPaused(true))
	require.True(t, IsPaused())

	require.NoError(t, SetPaused(false))
	require.False(t, IsPaused())
}

// TestUserPaused_HealthIndependent asserts the core spec invariant: UserPaused
// tracks only the user switch and never the health state. The same persisted
// flag is read regardless of whether models are healthy.
func TestUserPaused_HealthIndependent(t *testing.T) {
	setupTestDB(t)

	require.NoError(t, SetPaused(false))
	for _, healthy := range []bool{true, false} {
		setHealthForTest(t, healthy)
		require.False(t, UserPaused(), "UserPaused must stay false regardless of health")
	}

	require.NoError(t, SetPaused(true))
	for _, healthy := range []bool{true, false} {
		setHealthForTest(t, healthy)
		require.True(t, UserPaused(), "UserPaused must stay true regardless of health")
	}
}

// TestIsPaused_TruthTable covers effective-pause = userPaused || !healthy,
// including the startup-race (not-ready) case which is treated as unhealthy.
func TestIsPaused_TruthTable(t *testing.T) {
	cases := []struct {
		name       string
		userPaused bool
		// healthState: "healthy", "unhealthy", or "not_ready"
		healthState  string
		wantIsPaused bool
	}{
		{"user_off+healthy", false, "healthy", false},
		{"user_off+unhealthy", false, "unhealthy", true},
		{"user_off+not_ready", false, "not_ready", true},
		{"user_on+healthy", true, "healthy", true},
		{"user_on+unhealthy", true, "unhealthy", true},
		{"user_on+not_ready", true, "not_ready", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			require.NoError(t, SetPaused(tc.userPaused))
			switch tc.healthState {
			case "healthy":
				setHealthForTest(t, true)
			case "unhealthy":
				setHealthForTest(t, false)
			case "not_ready":
				resetHealthForTest(t)
			}

			require.Equal(t, tc.wantIsPaused, IsPaused())
		})
	}
}

// TestPauseReason covers the reason string for every branch and the invariant
// that PauseReason is non-empty iff IsPaused is true.
func TestPauseReason(t *testing.T) {
	cases := []struct {
		name        string
		userPaused  bool
		healthState string
		wantReason  string
	}{
		{"user_off+healthy", false, "healthy", ""},
		{"user_off+unhealthy", false, "unhealthy", "model_unhealthy"},
		{"user_off+not_ready", false, "not_ready", "model_unhealthy"},
		{"user_on+healthy", true, "healthy", "user_paused"},
		{"user_on+unhealthy", true, "unhealthy", "user_paused+model_unhealthy"},
		{"user_on+not_ready", true, "not_ready", "user_paused+model_unhealthy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestDB(t)
			require.NoError(t, SetPaused(tc.userPaused))
			switch tc.healthState {
			case "healthy":
				setHealthForTest(t, true)
			case "unhealthy":
				setHealthForTest(t, false)
			case "not_ready":
				resetHealthForTest(t)
			}

			require.Equal(t, tc.wantReason, PauseReason())
			// Invariant: PauseReason != ""  <=>  IsPaused() == true.
			require.Equal(t, tc.wantReason != "", IsPaused())
		})
	}
}

// TestSetPausedRoundTrip verifies the persisted round trip through the
// aisettings store: engaging writes {"paused":true,"paused_at":...}, releasing
// writes {"paused":false} with no timestamp.
func TestSetPausedRoundTrip(t *testing.T) {
	setupTestDB(t)
	setHealthForTest(t, true)

	require.NoError(t, SetPaused(true))
	paused, pausedAt, err := aisettings.LoadAnalysisPausedConfig()
	require.NoError(t, err)
	require.True(t, paused)
	require.NotNil(t, pausedAt)

	require.NoError(t, SetPaused(false))
	paused, pausedAt, err = aisettings.LoadAnalysisPausedConfig()
	require.NoError(t, err)
	require.False(t, paused)
	require.Nil(t, pausedAt)
}

// TestPausedAt covers the timestamp surface: nil while not paused, a recent
// timestamp while paused, nil again after release.
func TestPausedAt(t *testing.T) {
	setupTestDB(t)

	require.Nil(t, PausedAt())

	require.NoError(t, SetPaused(true))
	at := PausedAt()
	require.NotNil(t, at)
	require.WithinDuration(t, time.Now().UTC(), *at, 2*time.Minute,
		"paused_at should be stamped near now")

	require.NoError(t, SetPaused(false))
	require.Nil(t, PausedAt())
}
