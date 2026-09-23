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

func newComtradeCtx(t *testing.T, method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/api/settings/comtrade", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

// TestComtradeSettings_Roundtrip: empty default → save key (masked echo) →
// empty-string save keeps the key (no accidental wipe) → disable via pointer.
func TestComtradeSettings_Roundtrip(t *testing.T) {
	testutil.SetupTestDB(t)

	// Empty default: not configured, enabled=true.
	ctx, rec := newComtradeCtx(t, http.MethodGet, "")
	GetComtradeSettings(ctx)
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
	ctx, rec = newComtradeCtx(t, http.MethodPost, `{"api_key":"0b3d-test-key-abcd"}`)
	SaveComtradeSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	// GET back: configured + last-4 hint only (never the full key).
	ctx, rec = newComtradeCtx(t, http.MethodGet, "")
	GetComtradeSettings(ctx)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Data.APIKeyConfigured)
	require.Equal(t, "abcd", resp.Data.APIKeyHint)
	require.NotContains(t, rec.Body.String(), "0b3d-test-key-abcd")

	// Empty api_key must NOT wipe the stored key (form-resubmission safety).
	ctx, rec = newComtradeCtx(t, http.MethodPost, `{"api_key":""}`)
	SaveComtradeSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	ctx, rec = newComtradeCtx(t, http.MethodGet, "")
	GetComtradeSettings(ctx)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Data.APIKeyConfigured)

	// Disable via pointer → resolver chain must skip the DB value.
	disable := false
	body, _ := json.Marshal(map[string]any{"enabled": &disable})
	ctx, rec = newComtradeCtx(t, http.MethodPost, string(body))
	SaveComtradeSettings(ctx)
	require.Equal(t, http.StatusOK, rec.Code)
	stored, _, err := aisettings.LoadComtradeConfig()
	require.NoError(t, err)
	require.Equal(t, false, stored["enabled"])
	require.Equal(t, "0b3d-test-key-abcd", stored["api_key"]) // key preserved
}
