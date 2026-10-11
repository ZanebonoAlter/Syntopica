package sources

import (
	"context"
	"os"
	"testing"
	"time"

	"syntopica-backend/internal/datasources"
)

// TestLiveSmoke performs real-network fetches against the five anonymous
// sources (EIA / JODI / WDI / Yahoo / GDELT) plus Comtrade/FRED when a key
// is configured.
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
	// Fast-frequency sources (add-fast-frequency-datasources tasks 1.6):
	// yahoo/gdelt are anonymous and always run; fred runs only when a key is
	// configured (same opt-in pattern as comtrade). Assertions stay
	// structural (rows/shape/latest period) — never specific numbers.
	t.Run("yahoo", func(t *testing.T) {
		y := NewYahoo(fetch, cache)
		res, err := y.Fetch(ctx, "CL=F", "1mo")
		if err != nil {
			t.Fatalf("yahoo live: %v", err)
		}
		assertLiveShape(t, res)
		obs := res["observations"].([]map[string]any)
		if len(obs) < 10 {
			t.Fatalf("yahoo live: ~21 trading days expected in 1mo, got %d rows", len(obs))
		}
		t.Logf("yahoo live: %d rows, latest period=%v currency=%v", len(obs), obs[len(obs)-1]["period"], res["currency"])
	})
	t.Run("gdelt", func(t *testing.T) {
		// GDELT 匿名档限流 1 次/5 秒（官方 429 文案：one every 5 seconds）。
		// 先节流等待，仍 429 则 skip——限流不是接口死亡信号（2026-10-02 实测
		// 匿名活）；生产语义 = SOURCE_UNAVAILABLE 显式错误帧记 gap，不阻断。
		time.Sleep(6 * time.Second)
		g := NewGdelt(fetch, cache)
		res, err := g.Fetch(ctx, "iran oil", "timelinevol", "7d")
		if err != nil {
			if se, ok := datasources.AsSourceError(err); ok && se.StatusCode == 429 {
				t.Skipf("gdelt rate-limited (1 req/5s free tier); service alive, not a death signal")
			}
			t.Fatalf("gdelt live: %v", err)
		}
		assertLiveShape(t, res)
		if res["no_data"].(bool) {
			t.Fatal("gdelt live: 'iran oil' timeline should have points")
		}
		obs := res["observations"].([]map[string]any)
		t.Logf("gdelt live: %d points, latest=%v", res["rows"], obs[len(obs)-1]["period"])
	})
	t.Run("fred", func(t *testing.T) {
		key := os.Getenv("FRED_API_KEY")
		if key == "" {
			t.Skip("FRED_API_KEY not set in this environment; key path already covered by unit tests")
		}
		fr := NewFred(fetch, cache, func() string { return key })
		res, err := fr.Fetch(ctx, "DCOILWTICO", "", "")
		if err != nil {
			t.Fatalf("fred live: %v", err)
		}
		assertLiveShape(t, res)
		if res["no_data"].(bool) {
			t.Fatal("fred live: DCOILWTICO last year should have data")
		}
		obs := res["observations"].([]map[string]any)
		t.Logf("fred live: %d rows, latest=%v units=%v frequency=%v", res["rows"], obs[len(obs)-1]["period"], res["units"], res["frequency"])
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
