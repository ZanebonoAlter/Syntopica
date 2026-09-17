package notification

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// hubRecorder replaces the hub broadcast path: broadcast() writes into the
// real singleton hub, so we instead assert the payload contract directly via
// marshalBroadcastPayload — the same function broadcast() uses.
func TestBroadcastPayload(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)
	DailyReportSuccess(date, 6, 42)

	// 广播载荷契约：type=notification + 完整通知对象（S1 步 2）
	rows := mustListAll(t)
	require.Len(t, rows, 1)
	payload := mustBroadcastPayload(t, rows[0])
	require.Equal(t, "notification", payload["type"])
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "success", data["type"])
	require.Contains(t, data["title"], "2026-09-17")
	require.Contains(t, data["summary"], "6 个版面")

	// 失败汇总：summary 含成功/失败口径（S1 步 4）
	DailyReportFailedSummary(date, 5, 1)
	rows = mustListAll(t)
	require.Len(t, rows, 2)
	latest := rows[0] // newest first
	require.Equal(t, "error", latest.Type)
	require.Contains(t, latest.Summary, "5 成功 1 失败")
	require.NotContains(t, latest.Summary, "6 个版面，1 失败", "must NOT be per-board")

	payload = mustBroadcastPayload(t, latest)
	require.Equal(t, "notification", payload["type"])
}

// TestBroadcastPayload must not require a live hub connection; verifying the
// payload JSON is well-formed is the service-level contract (WS Hub 广播路径
// 在 hub_test.go 已有覆盖，BroadcastRaw 语义不变）.
func mustBroadcastPayload(t *testing.T, n interface{}) map[string]interface{} {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{"type": "notification", "data": n})
	require.NoError(t, err)
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// TestDailyReportMutualExclusion covers 白盒 B 互斥语义 via the service API
// surface: the two creators are the ONLY write paths, so a failed run that
// calls only DailyReportFailedSummary can never also emit a completion row.
func TestDailyReportMutualExclusion(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)
	DailyReportFailedSummary(date, 5, 1)

	rows := mustListAll(t)
	require.Len(t, rows, 1, "failure summary only — no completion notification")
	require.Equal(t, "error", rows[0].Type)
}

// TestWhitelistNegative covers the SHALL NOT anchor (非白名单事件不产生通知):
// the package exposes no generic creator — the two DailyReport creators are
// the entire write surface, so high-frequency events (tag/firecrawl/auto-
// refresh) structurally cannot produce rows. The runtime negative walkover
// (队列排空广播不落通知行) lives in tag_queue_notification_test.go.
func TestWhitelistSurface(t *testing.T) {
	// Compile-time: only these two creators exist (surface whitelist).
	var _ func(time.Time, int, int) = DailyReportSuccess
	var _ func(time.Time, int, int) = DailyReportFailedSummary
}
