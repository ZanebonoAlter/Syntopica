package datasources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupHandler(t *testing.T) *Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)
	if err := UpsertCatalog(db, func(string) string { return "" }); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return NewHandler(db)
}

func TestHandlerListCatalog(t *testing.T) {
	h := setupHandler(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/datasources", nil)
	h.List(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		DataSources []CatalogDTO `json:"data_sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.DataSources) != 4 {
		t.Fatalf("want 4 sources, got %d", len(resp.DataSources))
	}
	for _, s := range resp.DataSources {
		if s.Topics == nil {
			t.Fatalf("%s: topics must be an array (possibly empty), got nil", s.Code)
		}
		if s.Status != StatusEnabled && s.Status != StatusDisabled {
			t.Fatalf("%s: bad status %q", s.Code, s.Status)
		}
	}
	// Comtrade disabled + reason names the config key (M2).
	var comtrade *CatalogDTO
	for i := range resp.DataSources {
		if resp.DataSources[i].Code == "un_comtrade" {
			comtrade = &resp.DataSources[i]
		}
	}
	if comtrade == nil || comtrade.Status != StatusDisabled || !strings.Contains(comtrade.StatusReason, "COMTRADE_API_KEY") {
		t.Fatalf("comtrade must be disabled with config-key reason, got %+v", comtrade)
	}
}

func TestHandlerProbeUnknownSource(t *testing.T) {
	h := setupHandler(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "code", Value: "nope"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/datasources/nope/probe", nil)
	h.Probe(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown source: want 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INVALID_ARGUMENT") {
		t.Fatalf("want INVALID_ARGUMENT, got %s", rec.Body.String())
	}
}

func TestHandlerProbeNotWired(t *testing.T) {
	h := setupHandler(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "code", Value: "eia_wpsr"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/datasources/eia_wpsr/probe", nil)
	h.Probe(c)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("unwired probe: want 501, got %d", rec.Code)
	}
}

func TestHandlerProbeSuccess(t *testing.T) {
	h := setupHandler(t)
	at := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	h.RegisterProbe("eia_wpsr", func(ctx context.Context) (map[string]any, time.Time, error) {
		return map[string]any{"rows": 3}, at, nil
	})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "code", Value: "eia_wpsr"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/datasources/eia_wpsr/probe", nil)
	h.Probe(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"retrieved_at":"2026-09-19T12:00:00Z"`) {
		t.Fatalf("response must carry retrieved_at: %s", body)
	}
	if !strings.Contains(body, `"elapsed_ms"`) {
		t.Fatalf("response must carry elapsed_ms: %s", body)
	}
	// Probe must NOT mutate the catalog row (spec: 结果不写目录状态).
	h.db.Model(&DataSource{}).Where("code = ?", "eia_wpsr").Find(&[]DataSource{})
	var row DataSource
	if err := h.db.Where("code = ?", "eia_wpsr").First(&row).Error; err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if row.LastProbeAt != nil {
		t.Fatalf("probe must not write last_probe_at, got %v", row.LastProbeAt)
	}
}

func TestHandlerProbeErrorMapping(t *testing.T) {
	h := setupHandler(t)
	// SOURCE_UNAVAILABLE → 502 with error_code.
	h.RegisterProbe("eia_wpsr", func(ctx context.Context) (map[string]any, time.Time, error) {
		return nil, time.Time{}, UnavailableDetail("eia_wpsr", "网络请求失败", "timeout")
	})
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "code", Value: "eia_wpsr"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	h.Probe(c)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "SOURCE_UNAVAILABLE") {
		t.Fatalf("want 502 SOURCE_UNAVAILABLE, got %d %s", rec.Code, rec.Body.String())
	}

	// SCHEMA_CHANGED → 502 but a distinguishable error_code.
	h.RegisterProbe("eia_wpsr", func(ctx context.Context) (map[string]any, time.Time, error) {
		return nil, time.Time{}, SchemaChanged("eia_wpsr", "上游结构漂移")
	})
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Params = gin.Params{{Key: "code", Value: "eia_wpsr"}}
	c2.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	h.Probe(c2)
	if rec2.Code != http.StatusBadGateway || !strings.Contains(rec2.Body.String(), "SCHEMA_CHANGED") {
		t.Fatalf("want 502 SCHEMA_CHANGED, got %d %s", rec2.Code, rec2.Body.String())
	}

	// INVALID_ARGUMENT → 400.
	h.RegisterProbe("eia_wpsr", func(ctx context.Context) (map[string]any, time.Time, error) {
		return nil, time.Time{}, InvalidArg("eia_wpsr", "参数非法")
	})
	rec3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(rec3)
	c3.Params = gin.Params{{Key: "code", Value: "eia_wpsr"}}
	c3.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	h.Probe(c3)
	if rec3.Code != http.StatusBadRequest || !strings.Contains(rec3.Body.String(), "INVALID_ARGUMENT") {
		t.Fatalf("want 400 INVALID_ARGUMENT, got %d %s", rec3.Code, rec3.Body.String())
	}
}

// TestHandlerListLiveStatus: with a key resolver set, key-requiring rows get
// live status on every List (UI-configured keys flip the row without a
// restart); seeded boot-time snapshots do not leak through.
func TestHandlerListLiveStatus(t *testing.T) {
	h := setupHandler(t)
	key := "db-key-9999"
	h.SetKeyResolver(func(string) string { return key })

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	h.List(c)
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"un_comtrade","name"`) && !strings.Contains(body, `"status":"enabled"`) {
		t.Fatalf("live key must flip un_comtrade to enabled: %s", body)
	}
	for _, m := range []string{"设置界面", "即时生效"} {
		if strings.Contains(body, m) && strings.Contains(body, "未配置") {
			t.Fatalf("stale disabled reason leaked: %s", body)
		}
	}

	// Resolver back to "" → disabled again, reason names the config key.
	h.SetKeyResolver(func(string) string { return "" })
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	h.List(c2)
	body2 := rec2.Body.String()
	if !strings.Contains(body2, `"status":"disabled"`) || !strings.Contains(body2, "COMTRADE_API_KEY") {
		t.Fatalf("empty key must disable with reason naming COMTRADE_API_KEY: %s", body2)
	}
}
