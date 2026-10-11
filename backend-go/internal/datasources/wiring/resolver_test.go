package wiring

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/testutil"
)

// TestDataSourcesKeyResolverChain covers the live key chains (bocha semantics)
// for BOTH keyed sources (COMTRADE_API_KEY / FRED_API_KEY):
// UI DB (enabled + key) wins > disabled DB value is skipped > absent DB row
// falls back to the boot-time static value (env/config.yaml via viper).
func TestDataSourcesKeyResolverChain(t *testing.T) {
	testutil.SetupTestDB(t)
	resolve := DataSourcesKeyResolver()

	// No DB row, no static config → "".
	require.Empty(t, resolve("COMTRADE_API_KEY"))

	// DB row enabled with key → returned live.
	require.NoError(t, aisettings.SaveComtradeConfig(map[string]interface{}{
		"api_key": "db-key-1234", "enabled": true,
	}, "test"))
	require.Equal(t, "db-key-1234", resolve("COMTRADE_API_KEY"))

	// Disabled DB row → skipped (falls back to static = "" here).
	require.NoError(t, aisettings.SaveComtradeConfig(map[string]interface{}{
		"api_key": "db-key-1234", "enabled": false,
	}, "test"))
	require.Empty(t, resolve("COMTRADE_API_KEY"))

	// Blank key → treated as unset.
	require.NoError(t, aisettings.SaveComtradeConfig(map[string]interface{}{
		"api_key": "   ", "enabled": true,
	}, "test"))
	require.Empty(t, resolve("COMTRADE_API_KEY"))

	// Unknown config key name → "" (keyed-source resolver by design).
	require.Empty(t, resolve("SOME_OTHER_KEY"))

	// FRED chain mirrors comtrade exactly (fred_config: api_key + enabled).
	require.Empty(t, resolve("FRED_API_KEY"))
	require.NoError(t, aisettings.SaveFredConfig(map[string]interface{}{
		"api_key": "fred-db-key-99", "enabled": true,
	}, "test"))
	require.Equal(t, "fred-db-key-99", resolve("FRED_API_KEY"))
	// The comtrade chain is untouched by the fred row (independent chains);
	// restore its DB row first (the blank-key step above left it blank).
	require.NoError(t, aisettings.SaveComtradeConfig(map[string]interface{}{
		"api_key": "db-key-1234", "enabled": true,
	}, "test"))
	require.Equal(t, "db-key-1234", resolve("COMTRADE_API_KEY"))
	require.Equal(t, "fred-db-key-99", resolve("FRED_API_KEY"))
	require.NoError(t, aisettings.SaveFredConfig(map[string]interface{}{
		"api_key": "fred-db-key-99", "enabled": false,
	}, "test"))
	require.Empty(t, resolve("FRED_API_KEY"))
	require.Equal(t, "db-key-1234", resolve("COMTRADE_API_KEY"))
}
