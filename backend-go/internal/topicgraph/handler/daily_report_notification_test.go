package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/notification"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// TestDailyReportNotification covers the terminal-state adjudication
// (test-cases 白盒 B 分支表 + 边界值): one notification per run, mutually
// exclusive completion/failure, aggregated failure copy.
func TestDailyReportNotification(t *testing.T) {
	_ = testutil.SetupTestDB(t)

	date := time.Date(2026, 9, 17, 4, 0, 0, 0, time.Local)

	// B1：失败版面数 = 0 → 完成通知（success）
	notifyDailyReportTerminal(date, 6, 6, 0, 0)
	rows := mustListNotifications(t)
	require.Len(t, rows, 1)
	require.Equal(t, "success", rows[0].Type)
	require.Contains(t, rows[0].Title, "2026-09-17")
	require.Contains(t, rows[0].Summary, "6 个版面")

	// B2：6 版面 1 失败 → 仅 1 条失败汇总（含成功/失败口径），不逐版面
	notifyDailyReportTerminal(date, 6, 5, 1, 0)
	rows = mustListNotifications(t)
	require.Len(t, rows, 2, "one failure summary, never per-board rows")
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "5 成功 1 失败")
	require.NotContains(t, rows[0].Summary, "7 个版面", "total copy must be the run's total_boards=6, never a per-board count")

	// B3：全部失败（6/6 失败）→ 仅 1 条失败汇总
	notifyDailyReportTerminal(date, 6, 0, 6, 0)
	rows = mustListNotifications(t)
	require.Len(t, rows, 3)
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "0 成功 6 失败")

	// 边界：单版面项目失败 1 = 全部失败 → 1 条失败汇总（B 边界值）
	notifyDailyReportTerminal(date, 1, 0, 1, 0)
	rows = mustListNotifications(t)
	require.Len(t, rows, 4)
	require.Contains(t, rows[0].Summary, "0 成功 1 失败")

	// B4（口径变更 D12）：6 版面 = 4 成功 + 1 失败 + 1 当日无内容 → 失败汇总
	// 的「共 N 个版面」含当日无内容的板块（与 WS 的 total_boards 同口径），
	// 当日无内容的板块不进失败数。
	notifyDailyReportTerminal(date, 6, 4, 1, 1)
	rows = mustListNotifications(t)
	require.Len(t, rows, 5)
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "共 6 个版面")
	require.Contains(t, rows[0].Summary, "4 成功 1 失败")
	require.Contains(t, rows[0].Summary, "1 个当日无内容")

	// 失败汇总与完成通知互斥：失败分支只调 FailedSummary，不产生完成行
	// （notifyDailyReportTerminal 内部 if/else 已保证——上迹 5 次调用每次
	// 恰好新增 1 行即为互斥证据）。
	require.Len(t, rows, 5, "exactly one row per adjudication call")
}

// mustListNotifications reads the notifications table via the service List
// (newest-first), keeping the assertion independent of internal ordering.
func mustListNotifications(t *testing.T) []models.Notification {
	t.Helper()
	rows, _, err := notification.List(false, 100, 0)
	require.NoError(t, err)
	return rows
}

// ── add-topic-key-event-markers 2.1：推荐失败/迟到/空候选不改日报终态 ────────

// TestDailyReportNotification_KeyEventRecommendationNeverChangesTerminal
// drives generateSingleBoard with a generateReportEntry stub that mimics the
// real GenerateAndSaveReport composition: report saved → existing terminal
// work → recommendation spawned (failing / late / empty-candidates). The
// terminal notification must stay EXACTLY ONE success row and the report row
// must remain completed — the recommendation is best-effort by construction
// and can never emit a second terminal notification.
func TestDailyReportNotification_KeyEventRecommendationNeverChangesTerminal(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	board := models.SemanticLabel{Label: "ke-term", Slug: "ke-term", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	report := repository.BoardDailyReport{
		SemanticBoardID: board.ID, PeriodDate: repository.NormalizeReportDate(time.Now()),
		Status: "completed", Title: "t",
	}
	require.NoError(t, db.Create(&report).Error)
	// One topic-attributed thread so the failing chat path is actually
	// reached (zero candidates would short-circuit to done/0 — that is the
	// empty-candidate path, covered by the empty board at the end).
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID, Label: "通知泳道", Status: repository.TopicStatusActive,
		FirstSeenDate: repository.NormalizeReportDate(time.Now()),
		LastSeenDate:  repository.NormalizeReportDate(time.Now()),
	}
	require.NoError(t, db.Omit("embedding", "centroid").Create(&topic).Error)
	section := repository.DailyReportSection{
		ReportID: report.ID, ClusterLabel: "章节", ArticleCount: 1,
		Embedding: repository.FloatsToPgVector([]float64{0}), PersistentTopicID: &topic.ID,
	}
	require.NoError(t, db.Create(&section).Error)
	thread := repository.DailyReportThread{
		ReportID: report.ID, SectionID: section.ID, Title: "通知线索", Summary: "s",
		RelatedArticleIDs: repository.JSON("[42]"), Embedding: repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, db.Create(&thread).Error)

	// Recommendation chat hook: always fails (covers the failure path; empty
	// candidates is exercised by the zero-thread board below).
	origChat := service.SetKeyEventRecommendationChatForTest(func(ctx context.Context, sessionID, system, user string) (string, error) {
		return "", context.DeadlineExceeded
	})
	t.Cleanup(origChat)
	// Synchronous spawn: the stubbed generateReportEntry below runs inside
	// generateSingleBoard's worker goroutine; running the recommendation to
	// completion there keeps assertions race-free (the default detached
	// goroutine has the same red-line behavior — see service tests).
	restoreSpawn := service.SetKeyEventRecommendationSpawnForTest(func(boardID, reportID uint, sessionID string) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		service.RunKeyEventRecommendationSync(ctx, boardID, reportID, sessionID)
	})
	t.Cleanup(restoreSpawn)

	origEntry := generateReportEntry
	t.Cleanup(func() { generateReportEntry = origEntry })
	generateReportEntry = func(ctx context.Context, boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
		// Mirror the real pipeline's spawn AFTER the save + existing terminal
		// work (synchronous here so the test observes the final state).
		service.SpawnKeyEventRecommendationForReport(boardID, report.ID, "daily_report_test")
		return &report, nil
	}

	rowsBefore := len(mustListNotifications(t))
	generateSingleBoard(board.ID, report.PeriodDate, "job-ke")

	rows := mustListNotifications(t)
	require.Equal(t, rowsBefore+1, len(rows), "exactly one terminal notification, never a second")
	require.Equal(t, "success", rows[0].Type)

	// Report untouched; recommendation run honestly failed; zero markers.
	var after repository.BoardDailyReport
	require.NoError(t, db.First(&after, report.ID).Error)
	require.Equal(t, "completed", after.Status)
	run, err := repository.Repo.GetKeyEventRecommendationRun(report.ID)
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, repository.KeyEventRunStateFailed, run.State)
	var markers int64
	require.NoError(t, db.Model(&repository.TopicKeyEventMarker{}).Count(&markers).Error)
	require.Zero(t, markers)

	// Late landing after the terminal: a rebuild restarts the run row (new
	// token); the old task's save drops the batch — notification count still
	// unchanged and no marker appears.
	require.NoError(t, repository.Repo.UpsertKeyEventRecommendationRun(report.ID, board.ID, "tok-late"))
	rowsBefore2 := len(mustListNotifications(t))
	saved, err := repository.Repo.SaveSuggestedMarkers(board.ID, report.ID, "tok-late", []repository.SuggestedKeyEvent{
		{SourceThreadID: 12345, Reason: "迟到且无效", EvidenceIDs: []uint{1}},
	})
	require.NoError(t, err)
	require.Zero(t, saved)
	require.Len(t, mustListNotifications(t), rowsBefore2, "迟到推荐不产生任何通知")

	// Empty-candidate board: zero threads → done, no AI call, one success.
	emptyBoard := models.SemanticLabel{Label: "ke-empty", Slug: "ke-empty", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&emptyBoard).Error)
	emptyReport := repository.BoardDailyReport{
		SemanticBoardID: emptyBoard.ID, PeriodDate: repository.NormalizeReportDate(time.Now().AddDate(0, 0, -1)),
		Status: "completed", Title: "t2",
	}
	require.NoError(t, db.Create(&emptyReport).Error)
	generateReportEntry = func(ctx context.Context, boardID uint, date time.Time) (*repository.BoardDailyReport, error) {
		return &emptyReport, nil // no spawn path calls AI on empty candidates anyway
	}
	rowsBefore3 := len(mustListNotifications(t))
	generateSingleBoard(emptyBoard.ID, emptyReport.PeriodDate, "job-ke2")
	require.Equal(t, rowsBefore3+1, len(mustListNotifications(t)))
	require.Equal(t, "success", mustListNotifications(t)[0].Type)
}
