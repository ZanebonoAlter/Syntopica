package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/websearch"
)

// ── 网页搜索设置端点测试（switch-margin-note-search-to-deepseek 任务 3.4）──
// 全部经 handler 层接缝（webSearchResolveCredentials / webSearchProbeHealth）
// mock：不打网络、不连库。四态一律 200（disabled 是「未配置」不是故障，
// unreachable 的原因在 error_kind 里），面板据 status 与 error_kind 出文案。

func newWebSearchCtx(t *testing.T, path string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	return ctx, recorder
}

// stubWebSearchCredentials 替换凭据解析接缝（返回值由用例决定）。
func stubWebSearchCredentials(t *testing.T, creds websearch.Credentials, err error) {
	t.Helper()
	orig := webSearchResolveCredentials
	webSearchResolveCredentials = func(context.Context) (websearch.Credentials, error) {
		return creds, err
	}
	t.Cleanup(func() { webSearchResolveCredentials = orig })
}

// stubWebSearchProbe 替换探测接缝（返回值由用例决定）。
func stubWebSearchProbe(t *testing.T, health websearch.Health, err error) {
	t.Helper()
	orig := webSearchProbeHealth
	webSearchProbeHealth = func(context.Context) (websearch.Health, error) {
		return health, err
	}
	t.Cleanup(func() { webSearchProbeHealth = orig })
}

type webSearchHealthResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Status    string `json:"status"`
		LatencyMS int64  `json:"latency_ms"`
		Detail    string `json:"detail"`
		ErrorKind string `json:"error_kind"`
	} `json:"data"`
}

// TestGetWebSearchHealth_FourStates 覆盖四态 + error_kind 透出：ok / degraded /
// unreachable(auth) / unreachable(quota) / disabled，全部 HTTP 200。
func TestGetWebSearchHealth_FourStates(t *testing.T) {
	cases := []struct {
		name     string
		health   websearch.Health
		wantKind string
	}{
		{
			name:   "ok",
			health: websearch.Health{Status: websearch.HealthStatusOK, LatencyMS: 462, Detail: "搜索正常，返回 10 条结果"},
		},
		{
			name:   "degraded",
			health: websearch.Health{Status: websearch.HealthStatusDegraded, LatencyMS: 380, Detail: "服务可达但搜索无结果（搜索服务侧可能异常）"},
		},
		{
			name: "unreachable_auth",
			health: websearch.Health{
				Status:    websearch.HealthStatusUnreachable,
				LatencyMS: 120,
				Detail:    "鉴权失败：DeepSeek 密钥无效或无权访问搜索接口",
				ErrorKind: websearch.KindAuth,
			},
			wantKind: "auth",
		},
		{
			name: "unreachable_quota",
			health: websearch.Health{
				Status:    websearch.HealthStatusUnreachable,
				LatencyMS: 135,
				Detail:    "余额或额度不足：DeepSeek 账户余额或套餐额度已耗尽",
				ErrorKind: websearch.KindQuota,
			},
			wantKind: "quota",
		},
		{
			name:   "disabled",
			health: websearch.Health{Status: websearch.HealthStatusDisabled, Detail: "未关联可用的 DeepSeek 供应商（联网参考已关闭，问答不受影响）"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubWebSearchProbe(t, tc.health, nil)

			ctx, rec := newWebSearchCtx(t, "/api/settings/web-search/health")
			GetWebSearchHealth(ctx)

			require.Equal(t, http.StatusOK, rec.Code, "四态一律 200：状态在 body 里，不靠错误码")
			var resp webSearchHealthResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.True(t, resp.Success)
			assert.Equal(t, tc.health.Status, resp.Data.Status)
			assert.Equal(t, tc.health.LatencyMS, resp.Data.LatencyMS)
			assert.NotEmpty(t, resp.Data.Detail)
			assert.Equal(t, tc.wantKind, resp.Data.ErrorKind)
		})
	}
}

// TestGetWebSearchHealth_ProbeErrorFallsBackTo500：websearch.ProbeHealth 的 error
// 恒为 nil（失败形态全部编码进 Health，review R-7），这里只兜底「探测自身不可
// 执行」的极端情形——不得把它当主分支用。
func TestGetWebSearchHealth_ProbeErrorFallsBackTo500(t *testing.T) {
	stubWebSearchProbe(t, websearch.Health{}, errors.New("probe unavailable"))

	ctx, rec := newWebSearchCtx(t, "/api/settings/web-search/health")
	GetWebSearchHealth(ctx)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.NotEmpty(t, resp.Error)
}

type webSearchSettingsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		VendorConfigured bool   `json:"vendor_configured"`
		VendorName       string `json:"vendor_name"`
	} `json:"data"`
}

// TestGetWebSearchSettings_Configured：已关联供应商 → configured + 名称（无密钥字样）。
func TestGetWebSearchSettings_Configured(t *testing.T) {
	stubWebSearchCredentials(t, websearch.Credentials{
		VendorID: 7, VendorName: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", APIKey: "sk-secret",
	}, nil)

	ctx, rec := newWebSearchCtx(t, "/api/settings/web-search")
	GetWebSearchSettings(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp webSearchSettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.True(t, resp.Data.VendorConfigured)
	assert.Equal(t, "DeepSeek", resp.Data.VendorName)
	assert.NotContains(t, rec.Body.String(), "sk-secret", "MUST NOT 透出密钥")
	assert.NotContains(t, rec.Body.String(), "api.deepseek.com", "MUST NOT 透出 endpoint")
}

// TestGetWebSearchSettings_NotConfigured：未关联 → 200 空态（面板显示去 AI 路由的
// 指引，SHALL NOT 返回错误码）。
func TestGetWebSearchSettings_NotConfigured(t *testing.T) {
	stubWebSearchCredentials(t, websearch.Credentials{}, websearch.ErrNotConfigured)

	ctx, rec := newWebSearchCtx(t, "/api/settings/web-search")
	GetWebSearchSettings(ctx)

	require.Equal(t, http.StatusOK, rec.Code, "未配置是正常态，SHALL NOT 返回错误码")
	var resp webSearchSettingsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.False(t, resp.Data.VendorConfigured)
	assert.Empty(t, resp.Data.VendorName)
}

// TestGetWebSearchSettings_QueryFailureReturns500：真实查询失败（非未配置）→ 500，
// 前端走行内错误 + 重试，不谎报「未关联」。
func TestGetWebSearchSettings_QueryFailureReturns500(t *testing.T) {
	stubWebSearchCredentials(t, websearch.Credentials{}, errors.New("load ai_vendors: connection refused"))

	ctx, rec := newWebSearchCtx(t, "/api/settings/web-search")
	GetWebSearchSettings(ctx)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	assert.NotEmpty(t, resp.Error)
}
