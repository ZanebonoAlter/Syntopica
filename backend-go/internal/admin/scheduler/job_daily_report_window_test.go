package scheduler

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/scheduler"
	topicgraphrepo "syntopica-backend/internal/topicgraph/repository"
)

// ── night-window-alignment D1-D9：日报队列感知生成（单版完整制）──
//
// 生成时机 = 不早于墙钟时刻（NextRun 保证）∧ tag_jobs 与 embedding_queues
// 双队列 pending+leased 双零；到兜底时刻强制生成；当日已存在不重复。
// 等待循环的外部依赖（队列计数/兜底时刻/复查周期）全部走包级缝，本文件
// 按白盒分支表逐支覆盖，不依赖真实 60s 等待与墙钟到点。

// atClock returns base's calendar day at HH:MM local.
func atClock(base time.Time, h, m int) time.Time {
	return time.Date(base.Year(), base.Month(), base.Day(), h, m, 0, 0, base.Location())
}

// stubQueueDrained swaps the drain seam for a scripted responder; the counter
// records how often the wait loop polled.
func stubQueueDrained(t *testing.T, respond func(check int) bool) *atomic.Int32 {
	t.Helper()
	var checks atomic.Int32
	previous := dailyReportQueueDrained
	dailyReportQueueDrained = func() (bool, error) {
		return respond(int(checks.Add(1))), nil
	}
	t.Cleanup(func() { dailyReportQueueDrained = previous })
	return &checks
}

// overrideDeadline pins the deadline seam relative to real time (negative =
// already past → force-generate immediately).
func overrideDeadline(t *testing.T, offset time.Duration) {
	t.Helper()
	previous := dailyReportDeadlineFn
	dailyReportDeadlineFn = func(now time.Time) time.Time { return now.Add(offset) }
	t.Cleanup(func() { dailyReportDeadlineFn = previous })
}

// seedTodayReportRow inserts one (board, today) report row — the "当天已存在"
// predicate is any-board for the date.
func seedTodayReportRow(t *testing.T, db *gorm.DB, boardID uint) {
	t.Helper()
	require.NoError(t, db.Create(&topicgraphrepo.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      topicgraphrepo.NormalizeReportDate(midnightLocal(time.Now())),
		Status:          "completed",
	}).Error)
}

// TestDailyReportWindow_QueueEmptyGeneratesImmediately（D1 + 白盒「双空首查」）：
// 队列提前清空时墙钟触发的定时路径首查即生成，不消耗复查周期。
func TestDailyReportWindow_QueueEmptyGeneratesImmediately(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())
	seedReportBoardsForDate(t, db, today, 7)
	stub := stubDailyReportGeneration(t, db)

	checks := stubQueueDrained(t, func(int) bool { return true })

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Data["report_count"])
	require.Len(t, stub.snapshot(), 1)
	require.EqualValues(t, 1, checks.Load(), "double-empty queues must generate on the first check")
	require.NotContains(t, result.Data, "skipped")
}

// TestDailyReportWindow_WaitsUntilQueuesDrain（D2，spec「墙钟时队列非空则等待
// 清空」）：队列前几次复查非空、之后清空 → 在复查点触发生成（不早于 21:00 由
// NextRun 保证，这里验证等待→清空→生成的衔接）。
func TestDailyReportWindow_WaitsUntilQueuesDrain(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())
	seedReportBoardsForDate(t, db, today, 7)
	stub := stubDailyReportGeneration(t, db)

	// 前 3 次复查非空（tag backlog），第 4 次起清空（22:10 清空场景的缩影）。
	// 兜底缝拨到未来，排除运行钟点落在窗口外的偶发，专验「等待→清空→生成」。
	checks := stubQueueDrained(t, func(n int) bool { return n > 3 })
	overrideDeadline(t, time.Hour)

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Data["report_count"], "generation must fire after the queue drains")
	require.Len(t, stub.snapshot(), 1)
	require.GreaterOrEqual(t, checks.Load(), int32(4), "wait loop must re-check until drained")
}

// TestDailyReportWindow_DeadlineForcesGeneration（D3，spec「队列持续非空则兜底
// 强制生成」）：队列到兜底仍非空 → 立即强制生成，不等复查周期。
func TestDailyReportWindow_DeadlineForcesGeneration(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())
	seedReportBoardsForDate(t, db, today, 7)
	stub := stubDailyReportGeneration(t, db)

	checks := stubQueueDrained(t, func(int) bool { return false }) // 队列一直非空
	overrideDeadline(t, -time.Second)                              // 兜底已过（23:30 场景）

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, result.Data["report_count"], "deadline must force generation")
	require.Len(t, stub.snapshot(), 1)
	require.EqualValues(t, 1, checks.Load(), "deadline exit must happen on the first check")
}

// TestDailyReportWindow_ExistingReportSkipsGeneration（D4，spec「当日报告已存在
// 时 SHALL NOT 重复生成」）：当日任意版面已有报告 → 定时路径进入即返回，不生成、
// 不通知。
func TestDailyReportWindow_ExistingReportSkipsGeneration(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())
	seedReportBoardsForDate(t, db, today, 7) // 有内容、队列空、未到兜底

	seedTodayReportRow(t, db, 7)

	stub := stubDailyReportGeneration(t, db)
	capture := &notifyCapture{}
	capture.stub(t)

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)
	require.Empty(t, stub.snapshot(), "existing report must not be regenerated")
	require.Equal(t, "already_exists", result.Data["skipped"])
	require.Zero(t, capture.successCalls())
	require.Zero(t, capture.failCalls())
}

// TestDailyReportWindow_ManualTriggerDuringWaitReturns409（D5）：等待循环持有
// isExecuting 期间手动 TriggerNowWithDate 命中 409 重入保护（既有同 job 不并发）。
func TestDailyReportWindow_ManualTriggerDuringWaitReturns409(t *testing.T) {
	base := scheduler.New(scheduler.Config{
		Name: "Daily Report",
		Job:  func(ctx context.Context) (*scheduler.JobResult, error) { return nil, nil },
	})
	wrapper := NewDailyReportSchedulerWrapper(base)

	// 模拟定时路径等待循环占住执行权。
	require.True(t, wrapper.TrySetExecuting())
	defer wrapper.ClearExecuting()

	res := wrapper.TriggerNowWithDate("")
	require.Equal(t, false, res["accepted"])
	require.Equal(t, false, res["started"])
	require.Equal(t, http.StatusConflict, res["status_code"])
	require.Equal(t, "already_running", res["reason"])
}

// TestNextDailyReportTime_RestartWithinWindowReentersWait（D6，spec「重启不丢失
// 调度」）：墙钟已过、兜底未到、当日未生成 → 下次触发在复查周期内（继续当日
// 队列感知等待），SHALL NOT 因重启退化为 24 小时。
func TestNextDailyReportTime_RestartWithinWindowReentersWait(t *testing.T) {
	db := setupDailyReportJobTest(t) // 无报告行 → 当日未生成；配置缺省 21:00/23:30
	_ = db

	now := atClock(time.Now(), 22, 0)
	next := NextDailyReportTime(now)

	deadline := atClock(now, 23, 30)
	require.False(t, next.After(deadline), "next run %v must stay inside today's window (deadline %v)", next, deadline)
	require.True(t, next.After(now), "next run must be the recheck cycle, not immediate")
	require.Equal(t, now.Add(dailyReportPollInterval), next)
}

// TestNextDailyReportTime_StartAfterDeadlineDefersToTomorrow（D7，spec「服务在
// 目标时刻后启动」）：兜底已过且当日未生成 → 顺延次日墙钟，SHALL NOT 启动即跑。
func TestNextDailyReportTime_StartAfterDeadlineDefersToTomorrow(t *testing.T) {
	db := setupDailyReportJobTest(t)
	_ = db

	now := atClock(time.Now(), 23, 50)
	next := NextDailyReportTime(now)

	require.Equal(t, atClock(now, 21, 0).Add(24*time.Hour), next, "must defer to tomorrow's wall clock")
}

// TestNextDailyReportTime_ExistingReportDefersToTomorrow（单版完整制）：当日已
// 生成 → 下次触发直接顺延明日（生成完成后调度循环不空转）。
func TestNextDailyReportTime_ExistingReportDefersToTomorrow(t *testing.T) {
	db := setupDailyReportJobTest(t)
	seedTodayReportRow(t, db, 7)

	now := atClock(time.Now(), 21, 5)
	require.Equal(t, atClock(now, 21, 0).Add(24*time.Hour), NextDailyReportTime(now))
}

// TestNextDailyReportTime_BeforeWallClockStartsTonight（D4 场景「服务在目标时刻
// 前启动」）：18:00 启动 → 当日 21:00 触发。
func TestNextDailyReportTime_BeforeWallClockStartsTonight(t *testing.T) {
	setupDailyReportJobTest(t)

	now := atClock(time.Now(), 18, 0)
	require.Equal(t, atClock(now, 21, 0), NextDailyReportTime(now))
}

// TestDailyReportWindow_DeadlineEarlierThanWallFallsBack（D8）：deadline 配置
// 20:00 早于墙钟 21:00 → 回退默认 23:30 + warn，墙钟保持 21:00。
func TestDailyReportWindow_DeadlineEarlierThanWallFallsBack(t *testing.T) {
	db := setupDailyReportJobTest(t)
	require.NoError(t, db.Create(&models.AISettings{Key: "daily_report_time", Value: "21:00"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "daily_report_deadline", Value: "20:00"}).Error)

	now := time.Now()
	wall, deadline := dailyReportWindow(now)
	require.Equal(t, atClock(now, 21, 0), wall)
	require.Equal(t, atClock(now, 23, 30), deadline, "earlier deadline must fall back to default 23:30")

	// 21:30（墙钟后、回退后的兜底前）仍在窗口内 → 重启续跑语义成立。
	inside := atClock(now, 21, 30)
	require.Equal(t, inside.Add(dailyReportPollInterval), NextDailyReportTime(inside))
}

// TestDailyReportWindow_ConfigDefaultsAndInvalidFallback（D9）：双 key 缺失 →
// 默认 21:00/23:30；"25:99"/"abc" 各自回退默认。
func TestDailyReportWindow_ConfigDefaultsAndInvalidFallback(t *testing.T) {
	db := setupDailyReportJobTest(t)
	now := time.Now()

	// 缺失：默认。
	wall, deadline := dailyReportWindow(now)
	require.Equal(t, atClock(now, 21, 0), wall)
	require.Equal(t, atClock(now, 23, 30), deadline)

	// 非法：各自回退默认（时间 "25:99"、兜底 "abc"）。
	require.NoError(t, db.Create(&models.AISettings{Key: "daily_report_time", Value: "25:99"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "daily_report_deadline", Value: "abc"}).Error)
	wall, deadline = dailyReportWindow(now)
	require.Equal(t, atClock(now, 21, 0), wall)
	require.Equal(t, atClock(now, 23, 30), deadline)
}

// TestDailyReportWindow_WallEqualsDeadlineIsLegal（白盒边界「墙钟=兜底合法，
// 等价单时刻」）：窗口塌缩为单点——窗口后的时点顺延明日；到点后的强制生成
// 分支由 DeadlineForcesGeneration 覆盖（强制逻辑只看 now ≥ deadline，不依赖
// 测试运行的真实钟点）。
func TestDailyReportWindow_WallEqualsDeadlineIsLegal(t *testing.T) {
	db := setupDailyReportJobTest(t)
	require.NoError(t, db.Create(&models.AISettings{Key: "daily_report_time", Value: "21:00"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: "daily_report_deadline", Value: "21:00"}).Error)

	now := time.Now()
	wall, deadline := dailyReportWindow(now)
	require.Equal(t, atClock(now, 21, 0), wall)
	require.Equal(t, wall, deadline, "equal wall/deadline is legal and collapses the window")

	// 塌缩窗口后的时点 → 顺延明日（不再当日触发）。
	late := atClock(now, 21, 30)
	require.Equal(t, atClock(late, 21, 0).Add(24*time.Hour), NextDailyReportTime(late))
}
