package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/config"
)

// TestDefaultBoardConfigRelationAutoDiscoveryOff pins the per-board default:
// automatic relation discovery is opt-in (spec: automatic discovery is off by default),
// and old board rows without the column read as false via the resolver's
// COALESCE (verified against a real DB in the repository integration suite).
func TestDefaultBoardConfigRelationAutoDiscoveryOff(t *testing.T) {
	cfg := DefaultBoardConfig()
	require.False(t, cfg.RelationAutoDiscoveryEnabled)
	require.False(t, cfg.EnrichmentEnabled)
	require.Equal(t, 14, cfg.WindowDays)
	require.NoError(t, cfg.Validate())
}

// TestEffectiveCrossBoardRelationConfigDefaults proves the global budget
// fallback: when AppConfig was never loaded (nil) or the section is zero-valued
// (old config.yaml without a cross_board_rel block), every budget adopts its
// default instead of staying at an accidental zero (a zero budget would
// silently disable discovery).
func TestEffectiveCrossBoardRelationConfigDefaults(t *testing.T) {
	saved := config.AppConfig
	t.Cleanup(func() { config.AppConfig = saved })

	config.AppConfig = nil
	got := config.EffectiveCrossBoardRelationConfig()
	require.Equal(t, 3, got.AutoMaxSourcesPerBrief)
	require.Equal(t, 4, got.MaxSearchesPerRun)
	require.Equal(t, 2, got.MaxFetchesPerRun)
	require.Equal(t, 6, got.MaxLoopsPerRun)
	require.Equal(t, 300, got.RunTimeoutSeconds)
	require.InDelta(t, 0.62, got.ResolveThreshold, 1e-9)
	require.InDelta(t, 0.08, got.ResolveMargin, 1e-9)
	require.Equal(t, 14, got.DismissCooldownDays)
	require.Equal(t, 720, got.ConfirmedTTLHours)
	require.Equal(t, 3, got.BriefMaxRelations)
	require.Equal(t, 1200, got.BriefMaxRelationRunes)

	// Zero-valued section (old yaml) → same defaults field by field.
	config.AppConfig = &config.Config{}
	got2 := config.EffectiveCrossBoardRelationConfig()
	require.Equal(t, got, got2)

	// Explicit values survive untouched.
	config.AppConfig = &config.Config{}
	config.AppConfig.CrossBoardRel = config.CrossBoardRelationConfig{
		AutoMaxSourcesPerBrief: 1, MaxSearchesPerRun: 9, MaxFetchesPerRun: 8,
		MaxLoopsPerRun: 7, RunTimeoutSeconds: 60, ResolveThreshold: 0.7, ResolveMargin: 0.2,
		DismissCooldownDays: 5, ConfirmedTTLHours: 24, BriefMaxRelations: 1, BriefMaxRelationRunes: 100,
	}
	got3 := config.EffectiveCrossBoardRelationConfig()
	require.Equal(t, 1, got3.AutoMaxSourcesPerBrief)
	require.Equal(t, 9, got3.MaxSearchesPerRun)
	require.Equal(t, 0.7, got3.ResolveThreshold)
	require.Equal(t, 5, got3.DismissCooldownDays)
}
