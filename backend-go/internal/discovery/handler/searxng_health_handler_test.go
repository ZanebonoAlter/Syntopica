package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/testutil"
)

// ── SearXNG 健康探测端点测试（add-searxng-health-indicator 任务 3.1/3.2）──
// 四态判定经真实配置解析（ai_settings DB 现读）+ 真实 searxng.Search 探针
// （httptest 本地桩服务）；disabled 是正常态（200，不返回错误码）；探测不落
// ai_call_logs（非 AI 调用）。

func newSearxngHealthCtx(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/settings/searxng/health", nil)
	return ctx, recorder
}

// stubSearxngEndpoint 桩一个本地 SearXNG JSON API。
func stubSearxngEndpoint(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

type searxngHealthResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Status    string `json:"status"`
		LatencyMS int64  `json:"latency_ms"`
		Detail    string `json:"detail"`
	} `json:"data"`
}

// TestGetSearxngHealth_FourStates：disabled（未配置）/ unreachable（死端口）/
// degraded（200 空 results，2026-10-01 事故形态）/ ok（结果非空）。
func TestGetSearxngHealth_FourStates(t *testing.T) {
	t.Run("disabled_no_config_returns_200_not_error", func(t *testing.T) {
		testutil.SetupTestDB(t) // 未保存 searxng_config

		ctx, rec := newSearxngHealthCtx(t)
		GetSearxngHealth(ctx)

		require.Equal(t, http.StatusOK, rec.Code, "disabled 是正常态，SHALL NOT 返回错误码")
		var resp searxngHealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.True(t, resp.Success)
		assert.Equal(t, "disabled", resp.Data.Status)
		assert.NotEmpty(t, resp.Data.Detail)
	})

	t.Run("unreachable_dead_endpoint", func(t *testing.T) {
		testutil.SetupTestDB(t)
		require.NoError(t, aisettings.SaveSearxngConfig(map[string]interface{}{
			"endpoint": "http://127.0.0.1:1", "enabled": true,
		}, "probe test: unreachable"))

		ctx, rec := newSearxngHealthCtx(t)
		GetSearxngHealth(ctx)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp searxngHealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "unreachable", resp.Data.Status)
		assert.GreaterOrEqual(t, resp.Data.LatencyMS, int64(0))
	})

	t.Run("degraded_reachable_but_empty_results", func(t *testing.T) {
		db := testutil.SetupTestDB(t)
		endpoint := stubSearxngEndpoint(t, `{"results":[],"unresponsive_engines":["google"]}`)
		require.NoError(t, aisettings.SaveSearxngConfig(map[string]interface{}{
			"endpoint": endpoint, "enabled": true,
		}, "probe test: degraded"))

		ctx, rec := newSearxngHealthCtx(t)
		GetSearxngHealth(ctx)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp searxngHealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "degraded", resp.Data.Status, "引擎全挂 → degraded 而非 ok/unreachable")
		assert.Contains(t, resp.Data.Detail, "搜索无结果")

		// 任务 3.2：探测不走 airouter、不落 ai_call_logs（非 AI 调用）。
		var callLogs int64
		require.NoError(t, db.Model(&models.AICallLog{}).Count(&callLogs).Error)
		assert.Zero(t, callLogs, "探测 MUST NOT 落 ai_call_logs")
	})

	t.Run("ok_results_nonempty", func(t *testing.T) {
		testutil.SetupTestDB(t)
		endpoint := stubSearxngEndpoint(t, `{"results":[{"title":"connectivity probe","url":"https://example.com/p","content":"c"}]}`)
		require.NoError(t, aisettings.SaveSearxngConfig(map[string]interface{}{
			"endpoint": endpoint, "enabled": true,
		}, "probe test: ok"))

		ctx, rec := newSearxngHealthCtx(t)
		GetSearxngHealth(ctx)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp searxngHealthResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "ok", resp.Data.Status)
		assert.GreaterOrEqual(t, resp.Data.LatencyMS, int64(0))
		assert.True(t, strings.Contains(resp.Data.Detail, "结果"), "detail 带人话摘要")
	})
}
