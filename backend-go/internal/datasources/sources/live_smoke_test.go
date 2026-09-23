package sources

import (
	"context"
	"os"
	"testing"
	"time"

	"syntopica-backend/internal/datasources"
)

// TestLiveSmoke performs real-network fetches against the three anonymous
// sources (EIA / JODI / WDI) plus Comtrade when a key is configured.
// Opt-in only: DATASOURCES_LIVE=1 (mirrors the energy-mcp test_e2e_live
// pattern). Assertions are structural + non-empty + provenance metadata —
// never specific upstream numbers (they change every period).
func TestLiveSmoke(t *testing.T) {
	if os.Getenv("DATASOURCES_LIVE") != "1" {
		t.Skip("live smoke is opt-in: set DATASOURCES_LIVE=1 (Comtrade additionally needs COMTRADE_API_KEY)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Whitelist the four official hosts explicitly.
	var hosts []string
	for _, d := range datasources.Catalog() {
		hosts = append(hosts, d.HostAllowlist...)
	}
	fetch := datasources.NewFetcher(hosts, datasources.DefaultFetchBudget())
	cache := datasources.NewTTLCache(time.Minute)

	t.Run("eia", func(t *testing.T) {
		eia := NewEIA(fetch, cache)
		res, err := eia.Fetch(ctx, "stocks")
		if err != nil {
			t.Fatalf("eia live: %v", err)
		}
		assertLiveShape(t, res)
		if res["observations"].([]map[string]any)[0]["unit"] != "MMbbl" {
			t.Fatal("eia stocks unit must be MMbbl")
		}
	})
	t.Run("jodi", func(t *testing.T) {
		jodi := NewJODI(fetch, cache)
		res, err := jodi.Fetch(ctx, "US", "production", "", "")
		if err != nil {
			t.Fatalf("jodi live: %v", err)
		}
		assertLiveShape(t, res)
		obs := res["observations"].([]map[string]any)
		if len(obs) == 0 {
			t.Fatal("jodi live: expected at least one observation")
		}
	})
	t.Run("wdi", func(t *testing.T) {
		wdi := NewWDI(fetch, cache)
		res, err := wdi.Fetch(ctx, "NE.EXP.GNFS.ZS", []string{"CHN"}, 2022, 2024)
		if err != nil {
			t.Fatalf("wdi live: %v", err)
		}
		assertLiveShape(t, res)
		if len(res["observations"].([]WDIObservation)) == 0 {
			t.Fatal("wdi live: expected observations")
		}
	})
	t.Run("comtrade", func(t *testing.T) {
		key := os.Getenv("COMTRADE_API_KEY")
		if key == "" {
			t.Skip("COMTRADE_API_KEY not set in this environment; key path already covered by unit tests")
		}
		c := NewComtrade(fetch, cache, func() string { return key })
		res, err := c.Fetch(ctx, ComtradeParams{
			FreqCode: "A", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "2024",
		})
		if err != nil {
			t.Fatalf("comtrade live: %v", err)
		}
		assertLiveShape(t, res)
		if res["no_data"].(bool) {
			t.Fatal("comtrade annual 2024 should have data")
		}
	})
}

func assertLiveShape(t *testing.T, res map[string]any) {
	t.Helper()
	if res["retrieved_at"] == "" || res["source_sha256"] == "" {
		t.Fatalf("provenance metadata missing: retrieved_at/sha256")
	}
	if _, ok := res["observations"]; !ok {
		t.Fatalf("observations key missing in live result: %v", res["source"])
	}
}
