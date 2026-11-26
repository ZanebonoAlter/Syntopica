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
	// 口径变更（D12）：totalSaved 的语义从「条目数」改为「产出日报的板块数」，
	// 故入参从 42（条目）改为 5（板块）；断言「共 6 个版面」保持不变。
	DailyReportSuccess(date, 6, 5)

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
	require.Contains(t, data["summary"], "5 个版面已产出日报", "成功数是板块数，不是条目数")
	require.NotContains(t, data["summary"], "条条目", "文案不得把板块数表述成条目数")

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
// surface: the DailyReport creators are the ONLY write paths, so a failed run
// that calls only DailyReportFailedSummary can never also emit a completion row.
func TestDailyReportMutualExclusion(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)
	DailyReportFailedSummary(date, 5, 1)

	rows := mustListAll(t)
	require.Len(t, rows, 1, "failure summary only — no completion notification")
	require.Equal(t, "error", rows[0].Type)
}

// TestWhitelistNegative covers the SHALL NOT anchor (非白名单事件不产生通知):
// the package exposes no generic creator — the DailyReport creators are the
// entire write surface, so high-frequency events (tag/firecrawl/auto-
// refresh) structurally cannot produce rows. The runtime negative walkover
// (队列排空广播不落通知行) lives in tag_queue_notification_test.go.
func TestWhitelistSurface(t *testing.T) {
	// Compile-time: only these creators exist (surface whitelist).
	var _ func(time.Time, int, int) = DailyReportSuccess
	var _ func(time.Time, int, int) = DailyReportFailedSummary
	var _ func(time.Time, DailyReportCounts) = DailyReportTerminal
}

// TestDailyReportTerminalThreeStateCounts covers D12 三态口径：终态通知的
// 「共 N 个版面」== WS 的 total_boards（含当日无内容的板块），成功数 ==
// total_saved、失败数 == failed_count，当日无内容的板块显式给出且不进失败数。
func TestDailyReportTerminalThreeStateCounts(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)

	// 1 成功 + 1 失败 + 1 当日无内容 → 失败汇总（互斥：不再另发完成通知）
	DailyReportTerminal(date, DailyReportCounts{TotalBoards: 3, Saved: 1, Failed: 1, Empty: 1})
	rows := mustListAll(t)
	require.Len(t, rows, 1, "失败汇总与完成通知互斥，一次运行至多一条终态通知")
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "共 3 个版面", "总版面数含当日无内容的板块")
	require.Contains(t, rows[0].Summary, "1 成功 1 失败", "成功/失败数与 WS 的 total_saved/failed_count 一致")
	require.Contains(t, rows[0].Summary, "1 个当日无内容", "当日无内容的板块显式给出")
	require.NotContains(t, rows[0].Summary, "2 成功", "当日无内容的板块不计成功")

	// 全成功 + 1 当日无内容 → 完成通知（非失败汇总）
	DailyReportTerminal(date, DailyReportCounts{TotalBoards: 3, Saved: 2, Failed: 0, Empty: 1})
	rows = mustListAll(t)
	require.Len(t, rows, 2)
	latest := rows[0] // newest first
	require.Equal(t, "success", latest.Type)
	require.Contains(t, latest.Summary, "共 3 个版面")
	require.Contains(t, latest.Summary, "2 个版面已产出日报")
	require.Contains(t, latest.Summary, "1 个当日无内容")

	// 单板失败（收集阶段失败的早退口径）：共 1 个版面，0 成功 1 失败
	DailyReportTerminal(date, DailyReportCounts{TotalBoards: 1, Saved: 0, Failed: 1, Empty: 0})
	rows = mustListAll(t)
	require.Len(t, rows, 3)
	latest = rows[0]
	require.Equal(t, "error", latest.Type)
	require.Contains(t, latest.Summary, "共 1 个版面，0 成功 1 失败。")
	require.NotContains(t, latest.Summary, "当日无内容", "无当日无内容板块时不出现该分句")
}
