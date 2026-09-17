package scheduler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	adminrepo "syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/articlerefs"
	tagging "syntopica-backend/internal/tagmanagement"
	topicgraphrepo "syntopica-backend/internal/topicgraph/repository"
)

// ── offline-catchup 2.4/2.5：日报补档扫描 + 重建窗口守卫 ──
//
// SQLite 内存库同时接进 admin repository（tag_jobs 计数、ai_settings 读窗口）和
// topicgraph repository（板块收集、报告存在性）。生成入口 `generateAndSaveReport`
// 是包级变量，测试替换为写库 stub——真实实现会跑整条 LLM 流水线，单测无法承受。

// generatedCall records one (board, day) the stub was asked to generate.
type generatedCall struct {
	BoardID uint
	Date    time.Time
}

// generationStub is a deterministic stand-in for the LLM generation entry point.
type generationStub struct {
	mu    sync.Mutex
	calls []generatedCall
}

func (s *generationStub) snapshot() []generatedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]generatedCall(nil), s.calls...)
}

func setupDailyReportJobTest(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:dailyreport-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// Single connection: the job and its spawned goroutines share one in-memory
	// DB, and serializing writes avoids shared-cache table locks.
	sqlDB.SetMaxOpenConns(1)

	prevAdminRepo := adminrepo.Repo
	prevTopicRepo := topicgraphrepo.Repo
	adminrepo.InitRepository(db)
	topicgraphrepo.InitRepository(db)
	t.Cleanup(func() {
		adminrepo.Repo = prevAdminRepo
		topicgraphrepo.Repo = prevTopicRepo
	})

	require.NoError(t, db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.ArticleTopicTag{},
		&models.TopicTagBoardLabel{},
		&models.TagJob{},
		&models.AISettings{},
		&topicgraphrepo.BoardDailyReport{},
	))
	return db
}

// stubDailyReportGeneration swaps the generation entry point for a writer that
// records every request and persists a marker report row, so assertions can see
// both "was it regenerated" and "what exists in the DB".
func stubDailyReportGeneration(t *testing.T, db *gorm.DB) *generationStub {
	t.Helper()
	previous := generateAndSaveReport
	stub := &generationStub{}
	generateAndSaveReport = func(_ context.Context, boardID uint, date time.Time) (*topicgraphrepo.BoardDailyReport, error) {
		stub.mu.Lock()
		stub.calls = append(stub.calls, generatedCall{BoardID: boardID, Date: date})
		stub.mu.Unlock()

		report := topicgraphrepo.BoardDailyReport{
			SemanticBoardID: boardID,
			PeriodDate:      topicgraphrepo.NormalizeReportDate(date),
			Title:           fmt.Sprintf("generated board=%d date=%s", boardID, date.Format("2006-01-02")),
			Status:          "completed",
		}
		if err := db.Create(&report).Error; err != nil {
			return nil, err
		}
		return &report, nil
	}
	t.Cleanup(func() { generateAndSaveReport = previous })
	return stub
}

func midnightLocal(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// seedReportBoardsForDate makes the given boards count as "having content" on
// day: one article published that day carrying one active event tag mounted on
// each board — exactly the join CollectBoardIDsForDate walks.
func seedReportBoardsForDate(t *testing.T, db *gorm.DB, day time.Time, boardIDs ...uint) {
	t.Helper()
	suffix := day.Format("2006-01-02")

	feed := models.Feed{Title: "feed " + suffix, URL: fmt.Sprintf("https://example.com/feed/%s", suffix)}
	require.NoError(t, db.Create(&feed).Error)

	pubDate := midnightLocal(day).Add(10 * time.Hour)
	article := models.Article{
		FeedID:  feed.ID,
		Title:   "article " + suffix,
		Link:    fmt.Sprintf("https://example.com/article/%s", suffix),
		PubDate: &pubDate,
	}
	require.NoError(t, db.Create(&article).Error)

	tag := models.TopicTag{Slug: "tag-" + suffix, Label: "tag " + suffix, Category: models.TagCategoryEvent, Status: "active"}
	require.NoError(t, db.Create(&tag).Error)

	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tag.ID, Source: "llm"}).Error)

	for _, boardID := range boardIDs {
		require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: boardID}).Error)
	}
}

func seedExistingReport(t *testing.T, db *gorm.DB, boardID uint, day time.Time, title string) {
	t.Helper()
	require.NoError(t, db.Create(&topicgraphrepo.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      topicgraphrepo.NormalizeReportDate(day),
		Title:           title,
		Status:          "completed",
	}).Error)
}

// waitForWrapperIdle blocks until the wrapper's async run released its execution
// lock, so a triggered goroutine cannot outlive the test's cleanup.
func waitForWrapperIdle(t *testing.T, wrapper *DailyReportSchedulerWrapper) {
	t.Helper()
	require.Eventually(t, func() bool {
		return wrapper.GetStatus()["is_executing"] == false
	}, 5*time.Second, 5*time.Millisecond, "triggered daily report run must finish")
}

// Scenario「停机缺档次日自动补齐」: with the tagging queue drained, every missing
// (board, day) inside the window is rebuilt — and today is never rebuilt by the
// backfill (the main pass owns it).
func TestDailyReportJobBackfillsMissingReportsInWindow(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())

	missingDays := []time.Time{today.AddDate(0, 0, -1), today.AddDate(0, 0, -2), today.AddDate(0, 0, -3)}
	for _, day := range missingDays {
		seedReportBoardsForDate(t, db, day, 11, 22)
	}
	stub := stubDailyReportGeneration(t, db)

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)
	require.NotNil(t, result)

	require.EqualValues(t, 6, result.Data["backfilled_count"], "2 boards × 3 missing days")
	require.NotContains(t, result.Data, "backfill_skipped_reason", "empty queue must not defer")
	require.Contains(t, result.Summary, "backfilled 6")

	// Every backfill call lands inside the reported window and never on the
	// window's exclusive upper end (= today).
	windowEnd, err := time.ParseInLocation("2006-01-02", result.Data["backfill_window_end"].(string), time.Local)
	require.NoError(t, err)
	jobToday := windowEnd.AddDate(0, 0, 1)
	require.Equal(t, today.AddDate(0, 0, -1), windowEnd, "window excludes today")
	for _, call := range stub.snapshot() {
		callDay := midnightLocal(call.Date)
		require.False(t, callDay.After(windowEnd), "call %s outside window", callDay.Format("2006-01-02"))
		require.False(t, callDay.Equal(jobToday), "backfill must not regenerate today")
	}

	for _, day := range missingDays {
		for _, boardID := range []uint{11, 22} {
			exists, err := topicgraphrepo.Repo.ReportExistsForBoardDate(boardID, day)
			require.NoError(t, err)
			require.True(t, exists, "board %d %s must exist after backfill", boardID, day.Format("2006-01-02"))
		}
	}
	exists, err := topicgraphrepo.Repo.ReportExistsForBoardDate(11, today)
	require.NoError(t, err)
	require.False(t, exists, "backfill must not create today's report")
}

// Scenario「队列未清空顺延」: pending or leased tag jobs defer the whole backfill
// pass while today's report is generated as usual.
func TestDailyReportJobBackfillDefersUntilTagQueueDrains(t *testing.T) {
	cases := []struct {
		name   string
		status models.JobStatus
		key    string
	}{
		{name: "pending", status: models.JobStatusPending, key: "backfill_pending_jobs"},
		{name: "leased", status: models.JobStatusLeased, key: "backfill_leased_jobs"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDailyReportJobTest(t)
			today := midnightLocal(time.Now())

			seedReportBoardsForDate(t, db, today, 7) // today: main pass must still run
			gapDay := today.AddDate(0, 0, -2)
			seedReportBoardsForDate(t, db, gapDay, 7) // missing report, must stay missing

			require.NoError(t, db.Create(&models.TagJob{
				ArticleID:   1,
				Status:      string(tc.status),
				AvailableAt: time.Now(),
			}).Error)

			stub := stubDailyReportGeneration(t, db)

			result, err := DailyReportJob()(context.Background())
			require.NoError(t, err)

			require.Equal(t, "tag_queue_not_empty", result.Data["backfill_skipped_reason"])
			require.EqualValues(t, 1, result.Data[tc.key])
			require.EqualValues(t, 0, result.Data["backfilled_count"], "backfill must be skipped entirely")
			require.EqualValues(t, 1, result.Data["report_count"], "today's report is generated as usual")

			calls := stub.snapshot()
			require.Len(t, calls, 1, "only today was generated")
			require.Equal(t, today, midnightLocal(calls[0].Date))

			exists, err := topicgraphrepo.Repo.ReportExistsForBoardDate(7, gapDay)
			require.NoError(t, err)
			require.False(t, exists, "the gap day must be deferred, not half-filled")
		})
	}
}

// Scenario「只补缺不重建已有」: existing reports are never regenerated (that would
// burn a whole LLM pipeline run and replace a good report).
func TestDailyReportJobBackfillSkipsExistingReports(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())

	days := []time.Time{today.AddDate(0, 0, -1), today.AddDate(0, 0, -2), today.AddDate(0, 0, -3)}
	for _, day := range days {
		seedReportBoardsForDate(t, db, day, 5, 6)
		for _, boardID := range []uint{5, 6} {
			seedExistingReport(t, db, boardID, day, fmt.Sprintf("existing board=%d date=%s", boardID, day.Format("2006-01-02")))
		}
	}
	stub := stubDailyReportGeneration(t, db)

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)

	require.EqualValues(t, 0, result.Data["backfilled_count"])
	require.Empty(t, stub.snapshot(), "existing reports must not be regenerated")

	var titles []string
	require.NoError(t, db.Model(&topicgraphrepo.BoardDailyReport{}).Order("semantic_board_id").Pluck("title", &titles).Error)
	require.Len(t, titles, 6)
	for _, title := range titles {
		require.Contains(t, title, "existing", "pre-existing report content must survive untouched")
	}
}

// H2: the (semantic_board_id, period_date) unique index rejects a second row
// for the same board and day — the DB-level backstop behind SaveReport's
// find-then-create upsert now that the backfill scan is a second writer.
func TestBoardDailyReportUniqueIndexRejectsDuplicateBoardDate(t *testing.T) {
	db := setupDailyReportJobTest(t)
	day := midnightLocal(time.Now())

	seedExistingReport(t, db, 42, day, "first")

	err := db.Create(&topicgraphrepo.BoardDailyReport{
		SemanticBoardID: 42,
		PeriodDate:      topicgraphrepo.NormalizeReportDate(day),
		Title:           "duplicate",
		Status:          "completed",
	}).Error
	require.Error(t, err, "a second report for the same (board, day) must be rejected")
}

// M1: failed tag jobs are surfaced in JobResult but never block the backfill
// (their articles may simply have no edges yet — a permanently failing job must
// not freeze the catch-up forever).
func TestDailyReportJobBackfillReportsFailedJobsWithoutBlocking(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())

	gapDay := today.AddDate(0, 0, -2)
	seedReportBoardsForDate(t, db, gapDay, 9)
	require.NoError(t, db.Create(&models.TagJob{
		ArticleID:   1,
		Status:      string(models.JobStatusFailed),
		AvailableAt: time.Now(),
	}).Error)

	stub := stubDailyReportGeneration(t, db)

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err)

	require.EqualValues(t, 1, result.Data["backfill_failed_jobs"])
	require.NotContains(t, result.Data, "backfill_skipped_reason", "failed jobs must not defer the backfill")
	require.EqualValues(t, 1, result.Data["backfilled_count"])
	require.Len(t, stub.snapshot(), 1, "the missing gap day is still rebuilt")
}

// Scenario「调度器指定日期触发同口径」+ 边界「date == 下界放行」: the scheduler's
// TriggerNowWithDate uses the same window (and wording) as the HTTP handler, and
// the boundary day itself stays rebuildable.
func TestDailyReportJobRetentionGuardMatchesHandlerWindow(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())
	stub := stubDailyReportGeneration(t, db)

	cases := []struct {
		name         string
		setting      string // ai_settings tag_edge_retention_days; "" = key missing → default 7
		days         int
		wantRejected bool
	}{
		{name: "default window rejects day 8", setting: "", days: 8, wantRejected: true},
		{name: "default window allows boundary day 7", setting: "", days: 7, wantRejected: false},
		{name: "configured window rejects day 4", setting: "3", days: 4, wantRejected: true},
		{name: "configured window allows boundary day 3", setting: "3", days: 3, wantRejected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("key = ?", tagging.TagEdgeRetentionDaysKey).Delete(&models.AISettings{}).Error)
			if tc.setting != "" {
				require.NoError(t, db.Create(&models.AISettings{Key: tagging.TagEdgeRetentionDaysKey, Value: tc.setting}).Error)
			}

			target := today.AddDate(0, 0, -tc.days)
			wrapper := NewDailyReportSchedulerWrapper(New(Config{Name: "daily_report_guard_" + tc.name}))

			res := wrapper.TriggerNowWithDate(target.Format("2006-01-02"))

			if tc.wantRejected {
				require.Equal(t, false, res["accepted"])
				require.Equal(t, false, res["started"])
				require.Equal(t, "out_of_retention_window", res["reason"])
				require.Equal(t, http.StatusBadRequest, res["status_code"])
				message, ok := res["message"].(string)
				require.True(t, ok)
				require.Contains(t, message, "标签边")
				require.Contains(t, message, "拒绝重建")
				require.Empty(t, stub.snapshot(), "a rejected rebuild must not start generation")
				return
			}

			// Allowed: the guard must not be the thing that rejects it.
			accepted, ok := res["accepted"].(bool)
			require.True(t, ok)
			require.True(t, accepted, "boundary day is inside the window: %v", res["reason"])
			require.Equal(t, "manual_run_started", res["reason"])
			waitForWrapperIdle(t, wrapper)
		})
	}
}

// Scenario「巡检探针失败不影响 job」: the integrity probe reads
// daily_report_threads, which does not exist in every database the job can run
// against (this SQLite setup, or any environment whose schema lacks the table).
// The probe is observability only, so a failing check must leave the job a
// success — a false "dangling refs" repair trigger or a failed nightly run would
// both be worse than a missing log line (heal-dangling-article-refs D6).
func TestDailyReportJobSucceedsWhenDanglingRefProbeFails(t *testing.T) {
	db := setupDailyReportJobTest(t)
	today := midnightLocal(time.Now())
	seedReportBoardsForDate(t, db, today, 11)
	stubDailyReportGeneration(t, db)

	// The probe needs daily_report_threads and PostgreSQL's LATERAL join; this
	// SQLite schema has neither, so the probe must fail here — and that failure is
	// asserted directly rather than inferred, so a probe that silently degraded to
	// (0, nil) could not let this test pass for the wrong reason.
	require.False(t, db.Migrator().HasTable("daily_report_threads"),
		"this test needs the probe to fail: no thread table in the SQLite schema")
	_, probeErr := articlerefs.CountDanglingArticleRefs(db)
	require.Error(t, probeErr, "the integrity probe must fail on the SQLite schema, not return a silent zero")

	result, err := DailyReportJob()(context.Background())
	require.NoError(t, err, "a failing integrity probe must not fail the job")
	require.NotNil(t, result)
	require.EqualValues(t, 1, result.Data["report_count"], "the report pass still ran to completion")
}

// ── add-notification-center 白盒 B：定时路径终态通知判定口径 ──
//
// notification seam（notifyDailyReportSuccess / notifyDailyReportFailedSummary）
// 是包级变量，测试直接 stub 捕获判定结果，无需通知库。四分支：
// 全成功 / 部分真失败 / 空版面不误报 / 全部失败。互斥不变式（至多一条）由
// adjudicateDailyReportTerminal 单点保证，这里断言调用侧。

type notifyCapture struct {
	mu            sync.Mutex
	successDates  []time.Time
	successTotal  []int
	successSaved  []int
	failDates     []time.Time
	failSucceeded []int
	failFailed    []int
}

func (c *notifyCapture) stub(t *testing.T) {
	t.Helper()
	prevSuccess := notifyDailyReportSuccess
	prevFailed := notifyDailyReportFailedSummary
	notifyDailyReportSuccess = func(date time.Time, totalBoards, totalSaved int) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.successDates = append(c.successDates, date)
		c.successTotal = append(c.successTotal, totalBoards)
		c.successSaved = append(c.successSaved, totalSaved)
	}
	notifyDailyReportFailedSummary = func(date time.Time, successCount, failedCount int) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.failDates = append(c.failDates, date)
		c.failSucceeded = append(c.failSucceeded, successCount)
		c.failFailed = append(c.failFailed, failedCount)
	}
	t.Cleanup(func() {
		notifyDailyReportSuccess = prevSuccess
		notifyDailyReportFailedSummary = prevFailed
	})
}

func (c *notifyCapture) successCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.successDates)
}
func (c *notifyCapture) failCalls() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.failDates) }

// stubGenerationBehavior swaps the generation entry point for a behavior table:
// each boardID maps to "ok" (report) / "empty" (nil, nil) / "err" (error).
func stubGenerationBehavior(t *testing.T, behavior map[uint]string) {
	t.Helper()
	previous := generateAndSaveReport
	generateAndSaveReport = func(_ context.Context, boardID uint, date time.Time) (*topicgraphrepo.BoardDailyReport, error) {
		switch behavior[boardID] {
		case "err":
			return nil, fmt.Errorf("simulated LLM failure board=%d", boardID)
		case "empty":
			return nil, nil
		default:
			return &topicgraphrepo.BoardDailyReport{SemanticBoardID: boardID}, nil
		}
	}
	t.Cleanup(func() { generateAndSaveReport = previous })
}

func TestDailyReportTerminalNotificationAdjudication(t *testing.T) {
	today := midnightLocal(time.Now())

	t.Run("全成功→一条完成通知", func(t *testing.T) {
		db := setupDailyReportJobTest(t)
		seedReportBoardsForDate(t, db, today, 11, 22)
		stubGenerationBehavior(t, map[uint]string{11: "ok", 22: "ok"})
		cap := &notifyCapture{}
		cap.stub(t)

		_, err := DailyReportJob(today)(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, cap.successCalls(), "exactly one completion notification")
		require.Equal(t, 0, cap.failCalls(), "mutual exclusion: no failure summary")
	})

	t.Run("部分真失败→一条失败汇总", func(t *testing.T) {
		db := setupDailyReportJobTest(t)
		seedReportBoardsForDate(t, db, today, 11, 22, 33)
		stubGenerationBehavior(t, map[uint]string{11: "ok", 22: "err", 33: "ok"})
		cap := &notifyCapture{}
		cap.stub(t)

		_, err := DailyReportJob(today)(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, cap.failCalls(), "ONE failure summary, never per-board")
		require.Equal(t, 0, cap.successCalls(), "互斥：失败时不发完成通知")
		require.Equal(t, 2, cap.failSucceeded[0], "2 boards generated")
		require.Equal(t, 1, cap.failFailed[0], "1 real failure")
	})

	t.Run("空版面不误报为失败", func(t *testing.T) {
		db := setupDailyReportJobTest(t)
		seedReportBoardsForDate(t, db, today, 11, 22)
		stubGenerationBehavior(t, map[uint]string{11: "ok", 22: "empty"})
		cap := &notifyCapture{}
		cap.stub(t)

		_, err := DailyReportJob(today)(context.Background())
		require.NoError(t, err)
		require.Equal(t, 0, cap.failCalls(), "report==nil (当日无内容) must NOT count as failure")
		require.Equal(t, 1, cap.successCalls(), "terminal state is a completion")
	})

	t.Run("全部失败→一条失败汇总", func(t *testing.T) {
		db := setupDailyReportJobTest(t)
		seedReportBoardsForDate(t, db, today, 11, 22)
		stubGenerationBehavior(t, map[uint]string{11: "err", 22: "err"})
		cap := &notifyCapture{}
		cap.stub(t)

		_, err := DailyReportJob(today)(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, cap.failCalls(), "ONE failure summary for the run")
		require.Equal(t, 0, cap.successCalls())
		require.Equal(t, 0, cap.failSucceeded[0])
		require.Equal(t, 2, cap.failFailed[0])
	})
}
