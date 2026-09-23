package wiring

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/testutil"
)

// TestComtradeKeyResolverChain covers the live key chain (bocha semantics):
// UI DB (enabled + key) wins > disabled DB value is skipped > absent DB row
// falls back to the boot-time static value (env/config.yaml via viper).
func TestComtradeKeyResolverChain(t *testing.T) {
	testutil.SetupTestDB(t)
	resolve := ComtradeKeyResolver()

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

	// Unknown config key name → "" (single-key resolver by design).
	require.Empty(t, resolve("SOME_OTHER_KEY"))
}
