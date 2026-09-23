// Package wiring assembles the research data sources domain: fetchers over
// the budgeted HTTP layer, probe endpoints, and the agent-loop Tool adapters.
// It lives in its own package to break the import cycle
// datasources→sources→datasources (sources depends on the core primitives in
// datasources; the wiring depends on both).
package wiring

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/datasources"
	"syntopica-backend/internal/datasources/sources"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
)

// cacheTTL follows the energy-mcp field calibration (15 minutes, successes
// only).
const cacheTTL = 900 * time.Second

// newStack builds the shared fetcher/cache and the four source fetchers.
// resolve supplies the Comtrade key live (UI-configured value wins without
// restart).
func newStack(resolve datasources.KeyResolver) (*sources.EIA, *sources.JODI, *sources.WDI, *sources.Comtrade) {
	hosts := make([]string, 0, 4)
	for _, d := range datasources.Catalog() {
		hosts = append(hosts, d.HostAllowlist...)
	}
	fetch := datasources.NewFetcher(hosts, datasources.DefaultFetchBudget())
	cache := datasources.NewTTLCache(cacheTTL)
	return sources.NewEIA(fetch, cache),
		sources.NewJODI(fetch, cache),
		sources.NewWDI(fetch, cache),
		sources.NewComtrade(fetch, cache, func() string { return resolve("COMTRADE_API_KEY") })
}

// BuildTools assembles the fetcher stack and returns the four agent-loop
// tools (used by the dataenrichment wiring; registered but not in any default
// allowedTools list). Descriptions carry only STATIC catalog metadata; the
// 「当前不可用」availability notice is computed per research run from the live
// resolver (UnavailableNoticeProbe + service clone), never baked here
// (design §10.4：不用启动快照). resolve is still live for the Comtrade
// fetcher's per-call key lookup.
func BuildTools(resolve datasources.KeyResolver) []*service.Tool {
	e, j, w, c := newStack(resolve)
	return ResearchTools(e, j, w, c)
}

// RegisterRoutes mounts GET /api/datasources + POST /api/datasources/:code/probe
// with all four probes wired, using the global database.DB (domain
// RegisterRoutes convention).
func RegisterRoutes(api *gin.RouterGroup) *datasources.Handler {
	resolve := ComtradeKeyResolver()
	eia, jodi, wdi, comtrade := newStack(resolve)

	h := datasources.NewHandler(gormDB())
	h.SetKeyResolver(resolve)
	h.RegisterProbe("eia_wpsr", func(ctx context.Context) (map[string]any, time.Time, error) {
		res, err := eia.Fetch(ctx, "stocks")
		return probeSummary(res, err, "rows", "period")
	})
	h.RegisterProbe("jodi_oil_primary", func(ctx context.Context) (map[string]any, time.Time, error) {
		res, err := jodi.Fetch(ctx, "US", "production", "", "")
		return probeSummary(res, err, "rows", "period")
	})
	h.RegisterProbe("wb_wdi", func(ctx context.Context) (map[string]any, time.Time, error) {
		res, err := wdi.Fetch(ctx, "NE.EXP.GNFS.ZS", []string{"CHN"}, 2022, 2024)
		return probeSummary(res, err, "rows", "lastupdated")
	})
	h.RegisterProbe("un_comtrade", func(ctx context.Context) (map[string]any, time.Time, error) {
		res, err := comtrade.Fetch(ctx, sources.ComtradeParams{
			FreqCode: "A", Reporter: 156, CmdCode: "2709", FlowCode: "M", Period: "2024",
		})
		return probeSummary(res, err, "rows", "count")
	})

	ds := api.Group("/datasources")
	{
		ds.GET("", h.List)
		ds.POST("/:code/probe", h.Probe)
	}
	return h
}

// ComtradeKeyResolver returns the live key chain (bocha_config semantics):
// UI DB (comtrade_config: api_key + enabled, dynamic) > env/config.yaml
// (merged by viper at boot). Key-requiring sources re-resolve per call.
func ComtradeKeyResolver() datasources.KeyResolver {
	return func(configKeyName string) string {
		if configKeyName != "COMTRADE_API_KEY" {
			return ""
		}
		if cfg, _, err := aisettings.LoadComtradeConfig(); err == nil && cfg != nil {
			enabled := true
			if v, ok := cfg["enabled"].(bool); ok {
				enabled = v
			}
			if k, ok := cfg["api_key"].(string); ok && enabled && strings.TrimSpace(k) != "" {
				return strings.TrimSpace(k)
			}
		}
		if config.AppConfig != nil {
			return strings.TrimSpace(config.AppConfig.Comtrade.APIKey)
		}
		return ""
	}
}

// gormDB is the production global; kept as a var for test seams.
var gormDB = func() *gorm.DB { return database.DB }

// probeSummary extracts a compact summary (rows + one key metric) and the
// upstream retrieval time from a fetch result.
func probeSummary(res map[string]any, err error, rowsKey, metricKey string) (map[string]any, time.Time, error) {
	if err != nil {
		return nil, time.Time{}, err
	}
	summary := map[string]any{}
	if rows, ok := res["observations"]; ok {
		if b, err := json.Marshal(rows); err == nil {
			var arr []any
			if json.Unmarshal(b, &arr) == nil {
				summary["rows"] = len(arr)
			}
		}
	}
	if v, ok := res[metricKey]; ok {
		summary[metricKey] = v
	}
	var retrievedAt time.Time
	if s, ok := res["retrieved_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			retrievedAt = t
		}
	}
	return summary, retrievedAt, nil
}
