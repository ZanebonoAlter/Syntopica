package aisettings

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/testutil"
)

func TestParseValidHHMM(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"00:00", true},
		{"09:30", true},
		{"12:00", true},
		{"21:00", true},
		{"23:59", true},
		{"24:00", false},
		{"25:99", false},
		{"abc", false},
		{"", false},
		{"1:30", false},  // must be zero-padded
		{"09:60", false}, // minutes > 59
		{"09:5", false},  // must be zero-padded
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := hhmmPattern.MatchString(tt.input)
			if result != tt.valid {
				t.Errorf("hhmmPattern.MatchString(%q) = %v, want %v", tt.input, result, tt.valid)
			}
		})
	}
}

func TestDefaultDailyReportTime(t *testing.T) {
	if defaultDailyReportTime != "21:00" {
		t.Errorf("defaultDailyReportTime = %q, want %q", defaultDailyReportTime, "21:00")
	}
}

func TestProxyConfigKey(t *testing.T) {
	if proxyConfigKey != "http_proxy_config" {
		t.Errorf("proxyConfigKey = %q, want %q", proxyConfigKey, "http_proxy_config")
	}
}

func TestDefaultRSSHubDocBase(t *testing.T) {
	if defaultRSSHubDocBase != "https://docs.rsshub.app" {
		t.Errorf("defaultRSSHubDocBase = %q, want %q", defaultRSSHubDocBase, "https://docs.rsshub.app")
	}
	if rsshubDocBaseKey != "rsshub_doc_base" {
		t.Errorf("rsshubDocBaseKey = %q, want %q", rsshubDocBaseKey, "rsshub_doc_base")
	}
}

// TestLoadAnalysisPausedConfig_DefaultFalse covers the fail-open default: when
// the analysis_paused key is absent from ai_settings, Load must return
// (false, nil, nil) — analysis is NOT paused.
func TestLoadAnalysisPausedConfig_DefaultFalse(t *testing.T) {
	testutil.SetupTestDB(t)

	paused, pausedAt, err := LoadAnalysisPausedConfig()
	require.NoError(t, err)
	require.False(t, paused)
	require.Nil(t, pausedAt)
}

// TestSaveAndLoadAnalysisPausedConfig covers the store round trip: engaging
// (true) stamps paused_at, releasing (false) clears it again.
func TestSaveAndLoadAnalysisPausedConfig(t *testing.T) {
	testutil.SetupTestDB(t)

	// Engage: paused=true, paused_at stamped to now (UTC, RFC3339).
	require.NoError(t, SaveAnalysisPausedConfig(true))
	paused, pausedAt, err := LoadAnalysisPausedConfig()
	require.NoError(t, err)
	require.True(t, paused)
	require.NotNil(t, pausedAt)

	// Release: paused=false, paused_at cleared.
	require.NoError(t, SaveAnalysisPausedConfig(false))
	paused, pausedAt, err = LoadAnalysisPausedConfig()
	require.NoError(t, err)
	require.False(t, paused)
	require.Nil(t, pausedAt)
}

// TestLoadAutoStartModelsConfig_DefaultFalse covers the safe default: when
// the auto_start_models key is absent from ai_settings, Load returns
// (false, nil) — the backend must not auto-launch local model processes by
// default.
func TestLoadAutoStartModelsConfig_DefaultFalse(t *testing.T) {
	testutil.SetupTestDB(t)

	enabled, err := LoadAutoStartModelsConfig()
	require.NoError(t, err)
	require.False(t, enabled)
}

// TestSaveAndLoadAutoStartModelsConfig covers the store round trip for both
// on and off.
func TestSaveAndLoadAutoStartModelsConfig(t *testing.T) {
	testutil.SetupTestDB(t)

	require.NoError(t, SaveAutoStartModelsConfig(true))
	enabled, err := LoadAutoStartModelsConfig()
	require.NoError(t, err)
	require.True(t, enabled)

	require.NoError(t, SaveAutoStartModelsConfig(false))
	enabled, err = LoadAutoStartModelsConfig()
	require.NoError(t, err)
	require.False(t, enabled)
}

func TestIsValidHTTPURL(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"https://docs.rsshub.app", true},
		{"http://example.com", true},
		{"https://docs.rsshub.app/routes", true},
		{"ftp://example.com", false},
		{"example.com", false},
		{"", false},
		{"not a url", false},
		{"javascript:alert(1)", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isValidHTTPURL(tt.input); got != tt.valid {
				t.Errorf("isValidHTTPURL(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

// TestBochaConfigKey covers the jsonb key constant for the Bocha web-search
// backend (data-enrichment web_search, interface-configurable key).
func TestBochaConfigKey(t *testing.T) {
	if bochaConfigKey != "bocha_config" {
		t.Errorf("bochaConfigKey = %q, want %q", bochaConfigKey, "bocha_config")
	}
}

// TestLoadBochaConfig_DefaultEmpty covers the absent-key default: when
// bocha_config is missing from ai_settings, Load returns an empty map + nil
// settings + nil error (provider then falls back to env/config.yaml).
func TestLoadBochaConfig_DefaultEmpty(t *testing.T) {
	testutil.SetupTestDB(t)

	cfg, settings, err := LoadBochaConfig()
	require.NoError(t, err)
	require.Empty(t, cfg)
	require.Nil(t, settings)
}

// TestSaveAndLoadBochaConfig covers the store round trip: after Save writes the
// {api_key, endpoint, enabled} shape, Load reads the same fields back.
func TestSaveAndLoadBochaConfig(t *testing.T) {
	testutil.SetupTestDB(t)

	require.NoError(t, SaveBochaConfig(map[string]interface{}{
		"api_key":  "sk-bocha-123456",
		"endpoint": "https://api.bochaai.com/v1/web-search",
		"enabled":  true,
	}, "Bocha web-search configuration"))

	cfg, settings, err := LoadBochaConfig()
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, bochaConfigKey, settings.Key)
	require.Equal(t, "sk-bocha-123456", cfg["api_key"])
	require.Equal(t, "https://api.bochaai.com/v1/web-search", cfg["endpoint"])
	require.Equal(t, true, cfg["enabled"])
}

// TestSearxngConfigKey covers the jsonb key constant for the margin-note QA
// SearXNG backend (design D7).
func TestSearxngConfigKey(t *testing.T) {
	if searxngConfigKey != "searxng_config" {
		t.Errorf("searxngConfigKey = %q, want %q", searxngConfigKey, "searxng_config")
	}
}

// TestLoadSearxngConfig_DefaultEmpty covers the absent-key default: empty map
// → caller falls back to env/config.yaml, both empty = disabled (silent degrade).
func TestLoadSearxngConfig_DefaultEmpty(t *testing.T) {
	testutil.SetupTestDB(t)

	cfg, settings, err := LoadSearxngConfig()
	require.NoError(t, err)
	require.Empty(t, cfg)
	require.Nil(t, settings)
}

// TestSaveAndLoadSearxngConfig covers the store round trip for the
// {endpoint, enabled} shape.
func TestSaveAndLoadSearxngConfig(t *testing.T) {
	testutil.SetupTestDB(t)

	require.NoError(t, SaveSearxngConfig(map[string]interface{}{
		"endpoint": "http://localhost:8889",
		"enabled":  true,
	}, "SearXNG local search configuration"))

	cfg, settings, err := LoadSearxngConfig()
	require.NoError(t, err)
	require.NotNil(t, settings)
	require.Equal(t, searxngConfigKey, settings.Key)
	require.Equal(t, "http://localhost:8889", cfg["endpoint"])
	require.Equal(t, true, cfg["enabled"])
}
