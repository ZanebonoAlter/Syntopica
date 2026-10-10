package searxng

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/logging"
)

// swapResolveEndpoint stubs the endpoint resolution hook（同包测试范式：
// searchHTTPClient 先例），脚本化「已配置/未配置」两态。
func swapResolveEndpoint(t *testing.T, endpoint string, ok bool) {
	t.Helper()
	original := resolveConfiguredEndpoint
	resolveConfiguredEndpoint = func() (string, bool) { return endpoint, ok }
	t.Cleanup(func() { resolveConfiguredEndpoint = original })
}

// newSearxngStubServer builds a local SearXNG JSON API stub: body is the raw
// JSON payload returned for every /search request.
func newSearxngStubServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestProbeHealth_FourStates 覆盖面板四态判定（add-searxng-health-indicator
// design D2）：disabled（预检不过）/ unreachable（HTTP 层错误，含死端口）/
// degraded（HTTP 200 但 results 空——2026-10-01 事故形态）/ ok（结果非空）。
func TestProbeHealth_FourStates(t *testing.T) {
	t.Run("disabled_unconfigured", func(t *testing.T) {
		swapResolveEndpoint(t, "", false)
		res := ProbeHealth(context.Background())
		assert.Equal(t, HealthStatusDisabled, res.Status)
		assert.NotEmpty(t, res.Detail)
	})

	t.Run("unreachable_dead_endpoint", func(t *testing.T) {
		// 端口 1 本地几乎必然 connection refused（HTTP 层错误 → unreachable）。
		swapResolveEndpoint(t, "http://127.0.0.1:1", true)
		res := ProbeHealth(context.Background())
		assert.Equal(t, HealthStatusUnreachable, res.Status)
		assert.GreaterOrEqual(t, res.LatencyMS, int64(0))
		assert.Contains(t, res.Detail, "无法连接")
	})

	t.Run("degraded_http_ok_but_no_results", func(t *testing.T) {
		srv := newSearxngStubServer(t, `{"results":[],"unresponsive_engines":["google","bing"]}`)
		swapResolveEndpoint(t, srv.URL, true)
		res := ProbeHealth(context.Background())
		assert.Equal(t, HealthStatusDegraded, res.Status, "进程活引擎死 → degraded 而非 ok/unreachable")
		assert.Contains(t, res.Detail, "搜索无结果")
	})

	t.Run("ok_results_nonempty", func(t *testing.T) {
		srv := newSearxngStubServer(t, `{"results":[{"title":"connectivity probe","url":"https://example.com/probe","content":"c"}]}`)
		swapResolveEndpoint(t, srv.URL, true)
		res := ProbeHealth(context.Background())
		assert.Equal(t, HealthStatusOK, res.Status)
		assert.GreaterOrEqual(t, res.LatencyMS, int64(0), "耗时字段始终带出")
		assert.Contains(t, res.Detail, "1 条结果")
	})
}

// TestProbeHealth_LogPrefixSameAsQAChain（任务 3.2）：探测日志前缀与 QA 降级
// 链路同族可 grep——非 ok 态必须落一条「margin-notes: health probe」WARN，
// 排障时与 QA 搜索失败 WARN 一起检索。
func TestProbeHealth_LogPrefixSameAsQAChain(t *testing.T) {
	var captured strings.Builder
	logging.SetWriters(&captured, &captured)
	defer logging.ResetWriters()

	swapResolveEndpoint(t, "", false)
	_ = ProbeHealth(context.Background())
	require.Contains(t, captured.String(), "margin-notes: health probe", "disabled 探测须落可 grep 的 WARN")

	captured.Reset()
	srv := newSearxngStubServer(t, `{"results":[]}`)
	swapResolveEndpoint(t, srv.URL, true)
	_ = ProbeHealth(context.Background())
	require.Contains(t, captured.String(), "margin-notes: health probe", "degraded 探测须落可 grep 的 WARN")
}

// TestResolveConfiguredEndpoint_SingleSourceOfTruth：导出的解析入口即内部
// hook 默认实现（面板探测与 QA 链路 marginNoteSearxngEndpoint 共享同一路径，
// 两侧判活口径必然一致——「面板绿了链路还红」结构性不可能）。
func TestResolveConfiguredEndpoint_SingleSourceOfTruth(t *testing.T) {
	swapResolveEndpoint(t, "http://shared.example:8889", true)
	ep, ok := ResolveConfiguredEndpoint()
	require.True(t, ok)
	assert.Equal(t, "http://shared.example:8889", ep)
}
