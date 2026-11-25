package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/topicgraph/repository"
)

// ── 日报缺档补填编排（add-daily-report-gap-backfill task 2.2 / 4.1）──
//
// 覆盖：只补空、旧到新、单格失败不阻断、预算耗尽留档顺延、中断重跑幂等、
// 零缺口不发通知的编排前提（通知本身在 notification 包测）。

// setupBackfillServiceTest installs an in-memory SQLite DB with the two tables
// the orchestration touches (日报本体 + 补填运行留档) as the repository
// singleton. The previous singleton is restored on cleanup.
func setupBackfillServiceTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:backfillsvc-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&repository.BoardDailyReport{},
		&repository.DailyReportBackfillRun{},
	))

	prev := repository.Repo
	repository.InitRepository(db)
	t.Cleanup(func() {
		repository.Repo = prev
	})
	return db
}

// stubBackfillGaps replaces the gap scan with a fixed cell list.
func stubBackfillGaps(t *testing.T, gaps []repository.DailyReportGap) {
	t.Helper()
	prev := backfillListGaps
	backfillListGaps = func(_, _ time.Time) ([]repository.DailyReportGap, error) {
		return gaps, nil
	}
	t.Cleanup(func() { backfillListGaps = prev })
}

// backfillCall records one generation attempt.
type backfillCall struct {
	boardID uint
	date    string
}

// stubBackfillGenerate replaces the generation entry with a recorder. onCall may
// advance a fake clock (budget tests) and/or write the report (idempotency
// tests); the returned report marks the cell as filled.
func stubBackfillGenerate(t *testing.T, calls *[]backfillCall, onCall func(boardID uint, date time.Time) (*repository.BoardDailyReport, error)) {
	t.Helper()
	prev := backfillGenerateReport
	backfillGenerateReport = func(_ context.Context, boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
		*calls = append(*calls, backfillCall{boardID: boardID, date: date.Format("2006-01-02")})
		if onCall != nil {
			return onCall(boardID, date)
		}
		return &repository.BoardDailyReport{SemanticBoardID: boardID, PeriodDate: repository.NormalizeReportDate(date)}, nil
	}
	t.Cleanup(func() { backfillGenerateReport = prev })
}

// stubBackfillNow replaces the clock seam (budget tests).
func stubBackfillNow(t *testing.T, now *time.Time) {
	t.Helper()
	prev := backfillNow
	backfillNow = func() time.Time { return *now }
	t.Cleanup(func() { backfillNow = prev })
}

func backfillTestWindow() (time.Time, time.Time) {
	today := time.Now().In(time.Local)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	return today.AddDate(0, 0, -7), today.AddDate(0, 0, -1)
}

// TestRunGapBackfill_OnlyFillsEmpty: 只补空——已有报告的格子跳过，绝不重建。
func TestRunGapBackfill_OnlyFillsEmpty(t *testing.T) {
	db := setupBackfillServiceTest(t)
	windowStart, windowEnd := backfillTestWindow()

	day1 := windowStart
	gaps := []repository.DailyReportGap{
		{Date: day1.Format("2006-01-02"), BoardID: 11, BoardLabel: "板 A"},
		{Date: day1.Format("2006-01-02"), BoardID: 22, BoardLabel: "板 B"},
	}
	stubBackfillGaps(t, gaps)

	// 板 B 在 day1 已有报告 → 只补板 A。
	require.NoError(t, db.Create(&repository.BoardDailyReport{
		SemanticBoardID: 22,
		PeriodDate:      repository.NormalizeReportDate(day1),
		Status:          "completed",
	}).Error)

	var calls []backfillCall
	stubBackfillGenerate(t, &calls, nil)

	result, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	require.NoError(t, err)
	require.Len(t, calls, 1, "已有报告的格子不得重建")
	require.Equal(t, uint(11), calls[0].boardID)
	require.Equal(t, 1, result.Filled)
	require.Equal(t, 1, result.Skipped)
	require.Equal(t, 0, result.Remaining)
	require.Equal(t, repository.BackfillRunStatusSuccess, result.Status)
}

// TestRunGapBackfill_OldestFirst: 旧到新——按报告日升序处理（prev_report_id 链的前提）。
func TestRunGapBackfill_OldestFirst(t *testing.T) {
	setupBackfillServiceTest(t)
	windowStart, windowEnd := backfillTestWindow()

	day1 := windowStart
	day2 := windowStart.AddDate(0, 0, 1)
	day3 := windowStart.AddDate(0, 0, 2)
	gaps := []repository.DailyReportGap{
		{Date: day3.Format("2006-01-02"), BoardID: 3},
		{Date: day1.Format("2006-01-02"), BoardID: 1},
		{Date: day2.Format("2006-01-02"), BoardID: 2},
	}
	stubBackfillGaps(t, gaps)

	var calls []backfillCall
	stubBackfillGenerate(t, &calls, nil)

	_, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	require.NoError(t, err)
	require.Len(t, calls, 3)
	require.Equal(t, day1.Format("2006-01-02"), calls[0].date)
	require.Equal(t, day2.Format("2006-01-02"), calls[1].date)
	require.Equal(t, day3.Format("2006-01-02"), calls[2].date)
}

// TestRunGapBackfill_SingleFailureDoesNotBlock: 单格失败不阻断兄弟格。
func TestRunGapBackfill_SingleFailureDoesNotBlock(t *testing.T) {
	setupBackfillServiceTest(t)
	windowStart, windowEnd := backfillTestWindow()
	day := windowStart.Format("2006-01-02")

	gaps := []repository.DailyReportGap{
		{Date: day, BoardID: 1},
		{Date: day, BoardID: 2},
	}
	stubBackfillGaps(t, gaps)

	var calls []backfillCall
	stubBackfillGenerate(t, &calls, func(boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
		if boardID == 1 {
			return nil, fmt.Errorf("boom")
		}
		return &repository.BoardDailyReport{SemanticBoardID: boardID, PeriodDate: repository.NormalizeReportDate(date)}, nil
	})

	result, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	require.NoError(t, err)
	require.Len(t, calls, 2, "失败的那格不阻断兄弟格")
	require.Equal(t, 1, result.Filled)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, repository.BackfillRunStatusPartial, result.Status)
	require.Equal(t, 1, result.Remaining, "失败的格子仍是缺口")
}

// TestRunGapBackfill_BudgetExhaustedKeepsCompleted: 预算耗尽即停，已完成的保留，
// 剩余缺口写进运行结果（顺延不丢）。
func TestRunGapBackfill_BudgetExhaustedKeepsCompleted(t *testing.T) {
	db := setupBackfillServiceTest(t)
	windowStart, windowEnd := backfillTestWindow()
	day := windowStart.Format("2006-01-02")

	gaps := []repository.DailyReportGap{
		{Date: day, BoardID: 1},
		{Date: day, BoardID: 2},
		{Date: day, BoardID: 3},
	}
	stubBackfillGaps(t, gaps)

	// 假时钟：每次生成推进 2 分钟；预算 3 分钟 → 第 3 格前耗尽。
	clock := time.Now().In(time.Local)
	stubBackfillNow(t, &clock)

	var calls []backfillCall
	stubBackfillGenerate(t, &calls, func(boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
		clock = clock.Add(2 * time.Minute)
		return &repository.BoardDailyReport{SemanticBoardID: boardID, PeriodDate: repository.NormalizeReportDate(date)}, nil
	})

	result, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		Budget:      3 * time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, calls, 2, "预算耗尽后不再启动新的生成")
	require.Equal(t, 2, result.Filled)
	require.Equal(t, 1, result.Remaining, "剩余缺口被记下来")
	require.Equal(t, repository.BackfillRunStatusBudgetExhausted, result.Status)
	// 逐日明细也要如实记剩余（review R-3）：没走到的那格所属日期的 remaining
	// 不能是 0，否则与运行级 remaining_count 对不上。
	require.Len(t, result.Details, 1)
	require.Equal(t, 2, result.Details[0].Filled)
	require.Equal(t, 1, result.Details[0].Remaining, "未跑到的格子计入该日 remaining")

	// 运行留档：终态 + 计数落库（下一轮据此续跑）。
	var run repository.DailyReportBackfillRun
	require.NoError(t, db.Where("run_id = ?", result.RunID).First(&run).Error)
	require.Equal(t, repository.BackfillRunStatusBudgetExhausted, run.Status)
	require.Equal(t, 2, run.FilledCount)
	require.Equal(t, 1, run.RemainingCount)
	require.NotNil(t, run.FinishedAt)
}

// TestRunGapBackfill_Idempotent: 中断重跑幂等——只补仍缺的格子，已补出的不重建。
func TestRunGapBackfill_Idempotent(t *testing.T) {
	db := setupBackfillServiceTest(t)
	windowStart, windowEnd := backfillTestWindow()
	day := windowStart.Format("2006-01-02")

	gaps := []repository.DailyReportGap{
		{Date: day, BoardID: 1},
		{Date: day, BoardID: 2},
	}
	stubBackfillGaps(t, gaps)

	var calls []backfillCall
	stubBackfillGenerate(t, &calls, func(boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
		// 真的落一份报告（模拟生成成功），第二轮就该被跳过。
		require.NoError(t, db.Create(&repository.BoardDailyReport{
			SemanticBoardID: boardID,
			PeriodDate:      repository.NormalizeReportDate(date),
			Status:          "completed",
		}).Error)
		return &repository.BoardDailyReport{SemanticBoardID: boardID, PeriodDate: repository.NormalizeReportDate(date)}, nil
	})

	first, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	require.NoError(t, err)
	require.Equal(t, 2, first.Filled)

	second, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	require.NoError(t, err)
	require.Len(t, calls, 2, "第二轮不得重建已补出的报告")
	require.Equal(t, 0, second.Filled)
	require.Equal(t, 2, second.Skipped)
	require.Equal(t, repository.BackfillRunStatusSuccess, second.Status)
}

// TestRunGapBackfill_ZeroGaps: 零缺口 → 无格子被处理，终态 success，计数全 0。
func TestRunGapBackfill_ZeroGaps(t *testing.T) {
	db := setupBackfillServiceTest(t)
	windowStart, windowEnd := backfillTestWindow()
	stubBackfillGaps(t, nil)

	var calls []backfillCall
	stubBackfillGenerate(t, &calls, nil)

	result, err := RunGapBackfill(context.Background(), GapBackfillOptions{
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	})
	require.NoError(t, err)
	require.Empty(t, calls)
	require.Equal(t, 0, result.TotalCells)
	require.Equal(t, repository.BackfillRunStatusSuccess, result.Status)

	// 运行留档仍然写一行（终态 success），但没有任何格子被处理。
	var count int64
	require.NoError(t, db.Model(&repository.DailyReportBackfillRun{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// TestLoadBackfillBudget: 预算从 ai_settings 读，缺行/非法值回退默认 60 分钟。
func TestLoadBackfillBudget(t *testing.T) {
	db := setupBackfillServiceTest(t)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS ai_settings (key TEXT PRIMARY KEY, value TEXT)`).Error)

	require.Equal(t, DefaultBackfillBudgetMinutes*time.Minute, LoadBackfillBudget(db),
		"缺行时用默认 60 分钟")

	require.NoError(t, db.Exec(`INSERT INTO ai_settings (key, value) VALUES (?, ?)`, BackfillBudgetMinutesKey, "15").Error)
	require.Equal(t, 15*time.Minute, LoadBackfillBudget(db), "ai_settings 可调")

	require.NoError(t, db.Exec(`UPDATE ai_settings SET value = ? WHERE key = ?`, "not-a-number", BackfillBudgetMinutesKey).Error)
	require.Equal(t, DefaultBackfillBudgetMinutes*time.Minute, LoadBackfillBudget(db),
		"非法值回退默认")
}
