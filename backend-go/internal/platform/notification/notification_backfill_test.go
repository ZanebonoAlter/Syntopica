package notification

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/testutil"
)

// ── 日报缺档补填终态通知（add-daily-report-gap-backfill task 2.3 / 4.1）──
//
// 契约：每次运行**恰好一条**终态通知（四态），成功也发；零缺口不发；与每日
// 生成终态互不覆盖、互不抑制（两次独立运行）。

// TestDailyReportBackfillTerminalFourStates: 四态各产生一条通知，类型与文案
// 口径正确（补出篇数 / 失败数 / 剩余缺口数）。
func TestDailyReportBackfillTerminalFourStates(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	cases := []struct {
		status   string
		counts   BackfillCounts
		wantKind string
	}{
		{
			status:   BackfillStatusSuccess,
			counts:   BackfillCounts{TotalCells: 5, Filled: 5},
			wantKind: "success",
		},
		{
			status:   BackfillStatusPartial,
			counts:   BackfillCounts{TotalCells: 5, Filled: 3, Failed: 2, Remaining: 2},
			wantKind: "error",
		},
		{
			status:   BackfillStatusFailed,
			counts:   BackfillCounts{TotalCells: 3, Filled: 0, Failed: 3, Remaining: 3},
			wantKind: "error",
		},
		{
			status:   BackfillStatusBudgetExhausted,
			counts:   BackfillCounts{TotalCells: 6, Filled: 4, Remaining: 2},
			wantKind: "error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			before := len(mustListAll(t))
			DailyReportBackfillTerminal(tc.status, "2026-11-17", "2026-11-23", tc.counts)
			rows := mustListAll(t)
			require.Len(t, rows, before+1, "每次运行恰好一条终态通知")

			latest := rows[0]
			require.Equal(t, tc.wantKind, latest.Type)
			require.Contains(t, latest.Title, "2026-11-17")
			require.Contains(t, latest.Summary, "2026-11-23", "文案含补填窗口范围")
			require.Contains(t, latest.Summary, "补出")
			require.Contains(t, latest.Summary, "失败")
			require.Contains(t, latest.Summary, "剩余")
		})
	}
}

// TestDailyReportBackfillTerminalZeroGaps: 零缺口不产生通知。
func TestDailyReportBackfillTerminalZeroGaps(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	before := len(mustListAll(t))
	DailyReportBackfillTerminal(BackfillStatusSuccess, "2026-11-17", "2026-11-23", BackfillCounts{})
	require.Len(t, mustListAll(t), before, "零缺口不产生通知")

	DailyReportBackfillTerminal(BackfillStatusSuccess, "2026-11-17", "2026-11-23", BackfillCounts{TotalCells: 0, Filled: 0})
	require.Len(t, mustListAll(t), before, "零缺口不产生通知")
}

// TestDailyReportBackfillTerminalIndependentFromDailyReport: 补填终态与每日
// 生成终态是两次独立运行——两条通知都保留，互不覆盖或抑制。
func TestDailyReportBackfillTerminalIndependentFromDailyReport(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 11, 23, 4, 0, 0, 0, time.Local)
	DailyReportTerminal(date, DailyReportCounts{TotalBoards: 3, Saved: 3})
	require.Len(t, mustListAll(t), 1)

	DailyReportBackfillTerminal(BackfillStatusPartial, "2026-11-17", "2026-11-23",
		BackfillCounts{TotalCells: 5, Filled: 3, Failed: 2, Remaining: 2})
	rows := mustListAll(t)
	require.Len(t, rows, 2, "生成终态与补填终态两条通知都保留")

	// 最新一条是补填的（补填后发生）。
	require.Contains(t, rows[0].Title, "补填")
	require.Contains(t, rows[1].Title, "日报已生成")
}
