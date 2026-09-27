package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/notification"
	"syntopica-backend/internal/platform/testutil"
)

// TestDailyReportNotification covers the terminal-state adjudication
// (test-cases 白盒 B 分支表 + 边界值): one notification per run, mutually
// exclusive completion/failure, aggregated failure copy.
func TestDailyReportNotification(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)

	// B1：失败版面数 = 0 → 完成通知（success）
	notifyDailyReportTerminal(date, 6, 6, 0)
	rows := mustListNotifications(t)
	require.Len(t, rows, 1)
	require.Equal(t, "success", rows[0].Type)
	require.Contains(t, rows[0].Title, "2026-09-17")
	require.Contains(t, rows[0].Summary, "6 个版面")

	// B2：6 版面 1 失败 → 仅 1 条失败汇总（含成功/失败口径），不逐版面
	notifyDailyReportTerminal(date, 6, 5, 1)
	rows = mustListNotifications(t)
	require.Len(t, rows, 2, "one failure summary, never per-board rows")
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "5 成功 1 失败")
	require.NotContains(t, rows[0].Summary, "7 个版面", "total copy must be success+failed=6")

	// B3：全部失败（6/6 失败）→ 仅 1 条失败汇总
	notifyDailyReportTerminal(date, 6, 0, 6)
	rows = mustListNotifications(t)
	require.Len(t, rows, 3)
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "0 成功 6 失败")

	// 边界：单版面项目失败 1 = 全部失败 → 1 条失败汇总（B 边界值）
	notifyDailyReportTerminal(date, 1, 0, 1)
	rows = mustListNotifications(t)
	require.Len(t, rows, 4)
	require.Contains(t, rows[0].Summary, "0 成功 1 失败")

	// 失败汇总与完成通知互斥：失败分支只调 FailedSummary，不产生完成行
	// （notifyDailyReportTerminal 内部 if/else 已保证——上迹 4 次调用每次
	// 恰好新增 1 行即为互斥证据）。
	require.Len(t, rows, 4, "exactly one row per adjudication call")
}

// mustListNotifications reads the notifications table via the service List
// (newest-first), keeping the assertion independent of internal ordering.
func mustListNotifications(t *testing.T) []models.Notification {
	t.Helper()
	rows, _, err := notification.List(false, 100, 0)
	require.NoError(t, err)
	return rows
}
