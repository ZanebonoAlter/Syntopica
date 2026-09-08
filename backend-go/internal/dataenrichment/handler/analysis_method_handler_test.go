package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

func TestAnalysisMethodRoutesCRUDSoftDeleteEnableAndLegacyCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	repo := repository.NewRepository(db)
	repository.SetRepo(repo)
	const prefix = "analysis-method-handler-test-"
	t.Cleanup(func() {
		_ = db.Unscoped().Where("name LIKE ?", prefix+"%").Delete(&repository.AnalysisMethod{}).Error
		_ = db.Unscoped().Where("name LIKE ?", prefix+"%").Delete(&repository.ReferenceRole{}).Error
	})
	require.NoError(t, db.Unscoped().Where("name LIKE ?", prefix+"%").Delete(&repository.AnalysisMethod{}).Error)
	require.NoError(t, db.Unscoped().Where("name LIKE ?", prefix+"%").Delete(&repository.ReferenceRole{}).Error)

	h := handler.NewHandler(repo, nil, nil, nil, nil, nil, db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h.RegisterRoutes(router.Group("/api"))
	do := func(method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp
	}

	w, resp := do(http.MethodPost, "/api/analysis-methods", map[string]any{
		"name": prefix + "causal", "title": "因果链检验", "summary": "检查替代解释", "content": "列出竞争假设",
		"selection_meta": map[string]any{
			"applicable_when": []string{"存在因果主张"}, "avoid_when": []string{"无可核查材料"},
			"required_evidence": []string{"时间序列"}, "failure_modes": []string{"相关当因果"},
		},
		"enabled": true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := resp["data"].(map[string]any)
	id := uint(data["id"].(float64))
	require.False(t, data["legacy"].(bool))
	require.Equal(t, "无可核查材料", data["selection_meta"].(map[string]any)["avoid_when"].([]any)[0])

	w, resp = do(http.MethodPut, fmt.Sprintf("/api/analysis-methods/%d", id), map[string]any{"summary": "已编辑"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "已编辑", resp["data"].(map[string]any)["summary"])

	w, resp = do(http.MethodPut, fmt.Sprintf("/api/analysis-methods/%d/enable", id), map[string]any{"enabled": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.False(t, resp["data"].(map[string]any)["enabled"].(bool))

	legacy := &repository.AnalysisMethod{Name: prefix + "legacy", Title: "旧画像", Content: "原文", Legacy: true, Enabled: false}
	require.NoError(t, repo.CreateAnalysisMethod(context.Background(), legacy))
	w, resp = do(http.MethodGet, "/api/analysis-methods", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := resp["data"].([]any)
	require.Len(t, items, 2)
	foundLegacy := false
	for _, item := range items {
		if item.(map[string]any)["id"].(float64) == float64(legacy.ID) {
			foundLegacy = item.(map[string]any)["legacy"].(bool)
		}
	}
	require.True(t, foundLegacy)

	w, _ = do(http.MethodDelete, fmt.Sprintf("/api/analysis-methods/%d", id), nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = do(http.MethodGet, fmt.Sprintf("/api/analysis-methods/%d", id), nil)
	require.Equal(t, http.StatusNotFound, w.Code)

	// Old API remains readable for one version, but every write is explicitly gone.
	role := &repository.ReferenceRole{Name: prefix + "role", Title: "兼容角色", Content: "旧原文", Enabled: true}
	require.NoError(t, repo.CreateReferenceRole(context.Background(), role))
	w, _ = do(http.MethodGet, "/api/reference-roles", nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = do(http.MethodGet, fmt.Sprintf("/api/reference-roles/%d", role.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/reference-roles", map[string]any{"name": "x", "content": "x"}},
		{http.MethodPut, fmt.Sprintf("/api/reference-roles/%d", role.ID), map[string]any{"enabled": false}},
		{http.MethodDelete, fmt.Sprintf("/api/reference-roles/%d", role.ID), nil},
	} {
		w, _ = do(tc.method, tc.path, tc.body)
		require.Equal(t, http.StatusGone, w.Code)
	}
	unchanged, err := repo.GetReferenceRoleByID(context.Background(), role.ID)
	require.NoError(t, err)
	require.True(t, unchanged.Enabled)
}
