package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/datasources"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── fred settings（add-fast-frequency-datasources tasks 4.1；语义与 comtrade
// 逐条同款：脱敏回显 / 空串保留 / enabled 指针覆盖 / 保存即时回写目录状态）──

func newFredCtx(t *testing.T, method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/api/settings/fred", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

// TestFredSettings_Roundtrip: empty default → save key (masked echo, never the
// full key) → empty-string save keeps the key → disable via pointer keeps the
// key stored.
func TestFredSettings_Roundtrip(t *testing.T) {
	testutil.SetupTestDB(t)

	// Empty default: not configured, enabled=true.
	ctx, rec := newFredCtx(t, http.MethodGet, "")
	GetFredSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			APIKeyConfigured bool   `json:"api_key_configured"`
			APIKeyHint       string `json:"api_key_hint"`
			Enabled          bool   `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Data.APIKeyConfigured)
	require.Empty(t, resp.Data.APIKeyHint)
	require.True(t, resp.Data.Enabled)

	// Save a key.
	ctx, rec = newFredCtx(t, http.MethodPost, `{"api_key":"fed-test-key-9911"}`)
	SaveFredSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	// GET back: configured + last-4 hint only (never the full key).
	ctx, rec = newFredCtx(t, http.MethodGet, "")
	GetFredSettings(ctx)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Data.APIKeyConfigured)
	require.Equal(t, "9911", resp.Data.APIKeyHint)
	require.NotContains(t, rec.Body.String(), "fed-test-key-9911")

	// Empty api_key must NOT wipe the stored key (form-resubmission safety).
	ctx, rec = newFredCtx(t, http.MethodPost, `{"api_key":""}`)
	SaveFredSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	ctx, rec = newFredCtx(t, http.MethodGet, "")
	GetFredSettings(ctx)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Data.APIKeyConfigured)

	// Disable via pointer → resolver chain must skip the DB value, key kept.
	disable := false
	body, _ := json.Marshal(map[string]any{"enabled": &disable})
	ctx, rec = newFredCtx(t, http.MethodPost, string(body))
	SaveFredSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	stored, _, err := aisettings.LoadFredConfig()
	require.NoError(t, err)
	require.Equal(t, false, stored["enabled"])
	require.Equal(t, "fed-test-key-9911", stored["api_key"]) // key preserved
}

// TestFredSettings_ComtradeRowIndependent: the fred row never reads/writes the
// comtrade row (independent ai_settings keys).
func TestFredSettings_ComtradeRowIndependent(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, aisettings.SaveComtradeConfig(map[string]any{
		"api_key": "comtrade-key", "enabled": true,
	}, "test"))
	ctx, rec := newFredCtx(t, http.MethodPost, `{"api_key":"fred-key"}`)
	SaveFredSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	comtrade, _, err := aisettings.LoadComtradeConfig()
	require.NoError(t, err)
	require.Equal(t, "comtrade-key", comtrade["api_key"])

	fred, _, err := aisettings.LoadFredConfig()
	require.NoError(t, err)
	require.Equal(t, "fred-key", fred["api_key"])
}

// TestFredSettings_SaveFlipsCatalogStatusWithoutRestart: 界面保存 key →
// data_sources 目录 fred status 立即翻转 enabled（live resolver 现算回写，
// 无重启；spec scenario「界面保存 key 即时生效」）。
func TestFredSettings_SaveFlipsCatalogStatusWithoutRestart(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, datasources.UpsertCatalog(database.DB, func(string) string { return "" }))
	rows, err := datasources.ListCatalog(database.DB)
	require.NoError(t, err)
	for _, r := range rows {
		if r.Code == "fred" {
			require.Equal(t, datasources.StatusDisabled, r.Status, "seed with no key must be disabled")
		}
	}

	ctx, rec := newFredCtx(t, http.MethodPost, `{"api_key":"fed-flip-key"}`)
	SaveFredSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	rows, err = datasources.ListCatalog(database.DB)
	require.NoError(t, err)
	for _, r := range rows {
		if r.Code == "fred" {
			require.Equal(t, datasources.StatusEnabled, r.Status, "saved key must flip status live")
			require.Empty(t, r.StatusReason)
		}
	}
}
