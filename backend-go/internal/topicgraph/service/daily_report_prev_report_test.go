package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/topicgraph/repository"
)

// TestFindPreviousReportBrief_NearestEarlierReport 锁住「前一日引用」的语义
// （review R-1）。
//
// 背景：GORM 的 First() 自带 `ORDER BY <主键> ASC`，不显式排序时拿到的是**最早**
// 一份历史报告。补填按旧→新串行正是为了让 prev_report_id 指向紧邻的前一日，
// 主键序会把它指向史前第一份，链断在起点。
func TestFindPreviousReportBrief_NearestEarlierReport(t *testing.T) {
	db := setupBackfillServiceTest(t)

	reportDay := func(offsetDays int) time.Time {
		return repository.NormalizeReportDate(time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local).
			AddDate(0, 0, offsetDays))
	}
	mustCreate := func(boardID uint, date time.Time, status string) {
		require.NoError(t, db.Create(&repository.BoardDailyReport{
			SemanticBoardID: boardID,
			PeriodDate:      date,
			Status:          status,
		}).Error)
	}

	// 同板三份历史：最早（-6）、两天前（-2）、昨天（-1）。
	mustCreate(1, reportDay(-6), "completed")
	mustCreate(1, reportDay(-2), "completed")
	mustCreate(1, reportDay(-1), "completed")
	// 兄弟板同日报告不应被选中。
	mustCreate(2, reportDay(-1), "completed")

	prev := findPreviousReportBrief(1, reportDay(0))
	require.NotNil(t, prev)
	require.Equal(t, reportDay(-1).Format("2006-01-02"),
		prev.PeriodDate.Format("2006-01-02"),
		"prev_report_id 必须指向紧邻的前一日，而不是主键最早的那份")
	require.Equal(t, uint(1), prev.SemanticBoardID)

	// 未完成的报告不参与前一日引用（与既有 status=completed 过滤一致）。
	mustCreate(3, reportDay(-3), "completed")
	mustCreate(3, reportDay(-1), "running")
	prev = findPreviousReportBrief(3, reportDay(0))
	require.NotNil(t, prev)
	require.Equal(t, reportDay(-3).Format("2006-01-02"), prev.PeriodDate.Format("2006-01-02"))

	// 无更早报告 → nil（首日不编造前一份）。
	require.Nil(t, findPreviousReportBrief(99, reportDay(0)))
}
