package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/testutil"
)

// newBochaCtx builds a gin test context bound to the given JSON request body
// (nil → GET-style, no body). Returns ctx + its recorder.
func newBochaCtx(t *testing.T, method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/api/settings/bocha", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

// TestGetBochaSettings_Empty covers the absent-config default: GET returns
// api_key_configured=false, empty hint, the default endpoint, enabled=true.
func TestGetBochaSettings_Empty(t *testing.T) {
	testutil.SetupTestDB(t)

	ctx, rec := newBochaCtx(t, http.MethodGet, "")
	GetBochaSettings(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			APIKeyConfigured bool   `json:"api_key_configured"`
			APIKeyHint       string `json:"api_key_hint"`
			Endpoint         string `json:"endpoint"`
			Enabled          bool   `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Data.APIKeyConfigured)
	require.Empty(t, resp.Data.APIKeyHint)
	require.Equal(t, defaultBochaEndpoint, resp.Data.Endpoint)
	require.True(t, resp.Data.Enabled) // absent enabled defaults to true
}

// TestGetBochaSettings_MasksKey verifies the GET handler never echoes the full
// key: it returns a "configured" flag + last-4 hint only (secret safety).
func TestGetBochaSettings_MasksKey(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, aisettings.SaveBochaConfig(map[string]interface{}{
		"api_key":  "sk-bocha-secret-9988",
		"endpoint": "https://api.bochaai.com/v1/web-search",
		"enabled":  true,
	}, "test"))

	ctx, rec := newBochaCtx(t, http.MethodGet, "")
	GetBochaSettings(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			APIKeyConfigured bool   `json:"api_key_configured"`
			APIKeyHint       string `json:"api_key_hint"`
			Endpoint         string `json:"endpoint"`
			Enabled          bool   `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Data.APIKeyConfigured)
	require.Equal(t, "9988", resp.Data.APIKeyHint)
	// Full key must never appear in the response.
	require.NotContains(t, rec.Body.String(), "sk-bocha-secret-9988")
	require.True(t, resp.Data.Enabled)
}

// TestSaveBochaSettings_EmptyKeyPreserves covers the "api_key empty = don't
// change" contract: after saving a key, a POST with an empty api_key must keep
// the existing key intact (prevents form-roundtrip blanks from wiping it).
func TestSaveBochaSettings_EmptyKeyPreserves(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, aisettings.SaveBochaConfig(map[string]interface{}{
		"api_key": "sk-original-AAAA",
	}, "seed"))

	// POST with blank api_key (e.g. user edited only the endpoint).
	ctx, rec := newBochaCtx(t, http.MethodPost, `{"api_key":"","endpoint":"https://api.bochaai.com/v1/web-search","enabled":true}`)
	SaveBochaSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	cfg, _, err := aisettings.LoadBochaConfig()
	require.NoError(t, err)
	require.Equal(t, "sk-original-AAAA", cfg["api_key"], "empty api_key must preserve the existing key")
}

// TestSaveBochaSettings_NewKeyOverwrites covers the overwrite path: a non-empty
// api_key replaces whatever was stored.
func TestSaveBochaSettings_NewKeyOverwrites(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, aisettings.SaveBochaConfig(map[string]interface{}{
		"api_key": "sk-old",
	}, "seed"))

	ctx, rec := newBochaCtx(t, http.MethodPost, `{"api_key":"sk-new-BBBB","endpoint":"https://api.bochaai.com/v1/web-search","enabled":true}`)
	SaveBochaSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	cfg, _, err := aisettings.LoadBochaConfig()
	require.NoError(t, err)
	require.Equal(t, "sk-new-BBBB", cfg["api_key"])
}

// TestSaveBochaSettings_EnabledToggleWithoutKey covers enabled toggling while
// the key is preserved (enabled nil → unchanged; false → disabled).
func TestSaveBochaSettings_EnabledToggleWithoutKey(t *testing.T) {
	testutil.SetupTestDB(t)
	require.NoError(t, aisettings.SaveBochaConfig(map[string]interface{}{
		"api_key": "sk-stays",
		"enabled": true,
	}, "seed"))

	// Toggle enabled off, no api_key, no endpoint.
	ctx, rec := newBochaCtx(t, http.MethodPost, `{"enabled":false}`)
	SaveBochaSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	cfg, _, err := aisettings.LoadBochaConfig()
	require.NoError(t, err)
	require.Equal(t, "sk-stays", cfg["api_key"], "key preserved when only enabled toggled")
	require.Equal(t, false, cfg["enabled"])
}
