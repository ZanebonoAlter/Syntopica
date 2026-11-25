package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/notification"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// ── 日报缺档：缺口查询 + 异步补填端点（add-daily-report-gap-backfill tasks 2.4/2.5/2.6 / 4.2）──
//
// 覆盖：受理 / 拒绝（闸门 409、队列 409、窗口 400）/ 降级（缺口为空）与事件载荷契约。

// setupDailyReportGapTest installs an in-memory SQLite DB with the tables the
// two endpoints touch (ai_settings / 日报本体 / 补填运行留档 / 两条消费队列).
func setupDailyReportGapTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:dailyrepgap-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{},
		&models.TagJob{},
		&models.EmbeddingQueue{},
		&repository.BoardDailyReport{},
		&repository.DailyReportBackfillRun{},
	))

	prev := repository.Repo
	repository.InitRepository(db)
	t.Cleanup(func() { repository.Repo = prev })

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	return engine
}

// getDailyReportGaps issues the gap query and returns the decoded envelope.
func getDailyReportGaps(t *testing.T, engine *gin.Engine, query string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/daily-reports/gaps"+query, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	return w.Code, payload
}

// postBackfillGaps issues the backfill trigger and returns the decoded envelope.
func postBackfillGaps(t *testing.T, engine *gin.Engine, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	payloadBytes, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/daily-reports/backfill-gaps", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	return w.Code, payload
}

// localToday returns local midnight of today.
func localToday() time.Time {
	now := time.Now().In(time.Local)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// TestListDailyReportGaps_DefaultWindow: 缺口查询返回窗口边界与空缺口集合，
// 不写库、不触发生成；无运行记录时 last_run 缺省。
func TestListDailyReportGaps_DefaultWindow(t *testing.T) {
	engine := setupDailyReportGapTest(t)

	status, payload := getDailyReportGaps(t, engine, "")
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, payload["success"])

	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok, "data must be an object: %v", payload)
	today := localToday()
	require.Equal(t, today.AddDate(0, 0, -7).Format("2006-01-02"), data["window_start"])
	require.Equal(t, today.AddDate(0, 0, -1).Format("2006-01-02"), data["window_end"])
	require.Equal(t, float64(7), data["days"])
	require.Equal(t, float64(0), data["missing_days"])
	require.Equal(t, float64(0), data["total_missing"])
	require.NotContains(t, data, "last_run", "无运行记录时 last_run 缺省")
	gaps, ok := data["gaps"].([]interface{})
	require.True(t, ok, "gaps must be an array: %v", data)
	require.Empty(t, gaps)
}

// TestListDailyReportGaps_RetentionAboveDefaultDaysStaysAtSeven：保留窗口配得比
// 默认 days 大时，缺省查询仍只给 7 天窗口（review R-2 的口径锁：retention 是硬
// 上界、不是默认值；前端恒传 days=7，若缺省给满保留窗口，spec 与实现又会分叉）。
func TestListDailyReportGaps_RetentionAboveDefaultDaysStaysAtSeven(t *testing.T) {
	setupDailyReportGapTest(t)
	require.NoError(t, repository.Repo.DB().Create(&models.AISettings{
		Key: "tag_edge_retention_days", Value: "30",
	}).Error)

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	today := localToday()

	status, payload := getDailyReportGaps(t, engine, "")
	require.Equal(t, http.StatusOK, status)
	data := payload["data"].(map[string]interface{})
	require.Equal(t, float64(30), data["retention_days"])
	require.Equal(t, float64(7), data["days"], "retention=30 时缺省窗口仍是 7 天")
	require.Equal(t, today.AddDate(0, 0, -7).Format("2006-01-02"), data["window_start"])
}

// TestListDailyReportGaps_DaysOnlyNarrows: days 只能收窄，不能越过保留窗口下界。
func TestListDailyReportGaps_DaysOnlyNarrows(t *testing.T) {
	engine := setupDailyReportGapTest(t)
	today := localToday()

	status, payload := getDailyReportGaps(t, engine, "?days=30")
	require.Equal(t, http.StatusOK, status)
	data := payload["data"].(map[string]interface{})
	require.Equal(t, today.AddDate(0, 0, -7).Format("2006-01-02"), data["window_start"],
		"days=30 不得越过保留窗口下界（7 天）")
	require.Equal(t, today.AddDate(0, 0, -1).Format("2006-01-02"), data["window_end"])

	status, payload = getDailyReportGaps(t, engine, "?days=3")
	require.Equal(t, http.StatusOK, status)
	data = payload["data"].(map[string]interface{})
	require.Equal(t, today.AddDate(0, 0, -3).Format("2006-01-02"), data["window_start"],
		"days 只能收窄窗口")
	require.Equal(t, float64(3), data["days"])

	status, payload = getDailyReportGaps(t, engine, "?days=0")
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "invalid_days", payload["reason"])
}

// TestListDailyReportGaps_IncludesLastRun: 最近一次运行摘要随缺口响应给出。
func TestListDailyReportGaps_IncludesLastRun(t *testing.T) {
	engine := setupDailyReportGapTest(t)

	run := &repository.DailyReportBackfillRun{
		RunID:        "run-1",
		TriggerScope: repository.BackfillTriggerScopeManual,
		WindowStart:  localToday().AddDate(0, 0, -7),
		WindowEnd:    localToday().AddDate(0, 0, -1),
		StartedAt:    time.Now(),
	}
	require.NoError(t, repository.Repo.CreateBackfillRun(run))
	finished := time.Now()
	require.NoError(t, repository.Repo.FinalizeBackfillRun("run-1", repository.BackfillRunStatusSuccess,
		repository.JSON("[]"), 3, 0, 0, finished))

	status, payload := getDailyReportGaps(t, engine, "")
	require.Equal(t, http.StatusOK, status)
	data := payload["data"].(map[string]interface{})
	lastRun, ok := data["last_run"].(map[string]interface{})
	require.True(t, ok, "last_run must be present: %v", data)
	require.Equal(t, "run-1", lastRun["run_id"])
	require.Equal(t, repository.BackfillRunStatusSuccess, lastRun["status"])
	require.Equal(t, float64(3), lastRun["filled"])
	require.Equal(t, float64(0), lastRun["remaining"])
}

// TestTriggerBackfillDailyReportGaps_AlreadyRunning: 闸门被占 → 409 already_running，
// 不启动第二条流水线。
func TestTriggerBackfillDailyReportGaps_AlreadyRunning(t *testing.T) {
	engine := setupDailyReportGapTest(t)
	probe := &gateProbe{}
	probe.stub(t)

	status, payload := postBackfillGaps(t, engine, map[string]interface{}{})
	require.Equal(t, http.StatusConflict, status)
	require.Equal(t, false, payload["success"])
	require.Equal(t, "already_running", payload["reason"])
}

// TestTriggerBackfillDailyReportGaps_QueueNotDrained: 队列未排空 → 409，
// 说明补出的候选不全。
func TestTriggerBackfillDailyReportGaps_QueueNotDrained(t *testing.T) {
	t.Run("tag queue", func(t *testing.T) {
		engine := setupDailyReportGapTest(t)
		require.NoError(t, repository.Repo.DB().Create(&models.TagJob{
			ArticleID:   1,
			Status:      string(models.JobStatusPending),
			AvailableAt: time.Now(),
		}).Error)

		status, payload := postBackfillGaps(t, engine, map[string]interface{}{})
		require.Equal(t, http.StatusConflict, status)
		require.Equal(t, "tag_queue_not_empty", payload["reason"])
		msg, ok := payload["error"].(string)
		require.True(t, ok)
		require.Contains(t, msg, "队列")
	})

	t.Run("embedding queue", func(t *testing.T) {
		engine := setupDailyReportGapTest(t)
		require.NoError(t, repository.Repo.DB().Create(&models.EmbeddingQueue{
			TagID:  1,
			Status: models.EmbeddingQueueStatusPending,
		}).Error)

		status, payload := postBackfillGaps(t, engine, map[string]interface{}{})
		require.Equal(t, http.StatusConflict, status)
		require.Equal(t, "embedding_queue_not_empty", payload["reason"])
	})
}

// TestTriggerBackfillDailyReportGaps_RebuildWindowGuard: 超保留窗口的日期 → 400，
// 与 generate / TriggerNowWithDate 同措辞。
func TestTriggerBackfillDailyReportGaps_RebuildWindowGuard(t *testing.T) {
	engine := setupDailyReportGapTest(t)

	outside := localToday().AddDate(0, 0, -8) // 默认窗口 7 天
	status, payload := postBackfillGaps(t, engine, map[string]interface{}{
		"date": outside.Format("2006-01-02"),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "out_of_retention_window", payload["reason"])
	msg, ok := payload["error"].(string)
	require.True(t, ok)
	require.Contains(t, msg, "保留窗口")
}

// TestTriggerBackfillDailyReportGaps_TodayRejected: 今天由当日生成负责，
// 补填窗口不含今天。
func TestTriggerBackfillDailyReportGaps_TodayRejected(t *testing.T) {
	engine := setupDailyReportGapTest(t)

	status, payload := postBackfillGaps(t, engine, map[string]interface{}{
		"date": localToday().Format("2006-01-02"),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "out_of_backfill_window", payload["reason"])
}

// TestTriggerBackfillDailyReportGaps_ConflictingScope: date 与 days 互斥。
func TestTriggerBackfillDailyReportGaps_ConflictingScope(t *testing.T) {
	engine := setupDailyReportGapTest(t)

	status, payload := postBackfillGaps(t, engine, map[string]interface{}{
		"date": localToday().AddDate(0, 0, -2).Format("2006-01-02"),
		"days": 3,
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "conflicting_scope", payload["reason"])
}

// backfillRecorder captures the background run's terminal side effects:
// the WS terminal broadcast + the terminal notification call.
type backfillRecorder struct {
	mu           sync.Mutex
	doneCalls    int
	donePayload  map[string]interface{}
	notifyCalls  int
	notifyStatus string
	notifyCounts notification.BackfillCounts
	done         chan struct{}
}

func (r *backfillRecorder) stub(t *testing.T) {
	t.Helper()
	prevDone := broadcastBackfillDone
	prevNotify := notifyBackfillTerminal
	broadcastBackfillDone = func(jobID, runID, status string, filled, failed, remaining int, windowStart, windowEnd string, startedAt, finishedAt time.Time) {
		r.mu.Lock()
		r.doneCalls++
		r.donePayload = buildBackfillDoneMessage(jobID, runID, status, filled, failed, remaining, windowStart, windowEnd, startedAt, finishedAt)
		r.mu.Unlock()
		select {
		case r.done <- struct{}{}:
		default:
		}
	}
	notifyBackfillTerminal = func(status string, windowStart, windowEnd string, counts notification.BackfillCounts) {
		r.mu.Lock()
		r.notifyCalls++
		r.notifyStatus = status
		r.notifyCounts = counts
		r.mu.Unlock()
	}
	t.Cleanup(func() {
		broadcastBackfillDone = prevDone
		notifyBackfillTerminal = prevNotify
	})
}

// TestTriggerBackfillDailyReportGaps_Accepted: 受理即返回 job_id/run_id，
// 补填在后台跑；终态恰好一条通知 + 一条独立 WS 终态事件。
func TestTriggerBackfillDailyReportGaps_Accepted(t *testing.T) {
	engine := setupDailyReportGapTest(t)
	recorder := &backfillRecorder{done: make(chan struct{}, 1)}
	recorder.stub(t)

	status, payload := postBackfillGaps(t, engine, map[string]interface{}{})
	require.Equal(t, http.StatusOK, status, "%v", payload)
	require.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok, "data must be an object: %v", payload)
	require.NotEmpty(t, data["job_id"])
	require.NotEmpty(t, data["run_id"])
	require.Equal(t, "processing", data["status"])
	require.Equal(t, localToday().AddDate(0, 0, -7).Format("2006-01-02"), data["window_start"])
	require.Equal(t, localToday().AddDate(0, 0, -1).Format("2006-01-02"), data["window_end"])

	// 后台运行终态（最多等 5 秒）。
	select {
	case <-recorder.done:
	case <-time.After(5 * time.Second):
		t.Fatal("backfill run did not finish in time")
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	require.Equal(t, 1, recorder.notifyCalls, "每次运行恰好一条终态通知")
	require.Equal(t, 1, recorder.doneCalls, "每次运行恰好一条终态广播")
	require.Equal(t, "daily_report_backfill_done", recorder.donePayload["type"])
	require.Equal(t, recorder.notifyStatus, recorder.donePayload["status"])
}

// TestBuildBackfillProgressMessageCarriesDate: 补填的逐格进度复用
// daily_report_progress，并带 date 与 run_scope=backfill 附加字段；既有字段不变。
func TestBuildBackfillProgressMessageCarriesDate(t *testing.T) {
	msg := buildBackfillProgressMessage("job-9", service.GapBackfillProgress{
		RunID:      "run-9",
		Date:       "2026-11-20",
		BoardID:    42,
		BoardLabel: "算力",
		Status:     "completed",
		Filled:     3,
		Total:      5,
	})

	require.Equal(t, "daily_report_progress", msg["type"])
	require.Equal(t, "job-9", msg["job_id"])
	require.Equal(t, uint(42), msg["board_id"])
	require.Equal(t, "算力", msg["board_name"])
	require.Equal(t, 3, msg["saved"])
	require.Equal(t, "3/5", msg["progress"])
	require.Equal(t, "2026-11-20", msg["date"], "补填进度必须带报告日")
	require.Equal(t, "backfill", msg["run_scope"])
}

// TestBuildBackfillDoneMessageContract: 终态是独立事件类型（不复用
// daily_report_done），字段齐全。
func TestBuildBackfillDoneMessageContract(t *testing.T) {
	started := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	finished := started.Add(90 * time.Second)
	msg := buildBackfillDoneMessage("job-9", "run-9", "partial", 3, 2, 2,
		"2026-11-17", "2026-11-23", started, finished)

	require.Equal(t, "daily_report_backfill_done", msg["type"])
	require.Equal(t, "job-9", msg["job_id"])
	require.Equal(t, "run-9", msg["run_id"])
	require.Equal(t, "partial", msg["status"])
	require.Equal(t, 3, msg["filled"])
	require.Equal(t, 2, msg["failed"])
	require.Equal(t, 2, msg["remaining"])
	require.Equal(t, "2026-11-17", msg["window_start"])
	require.Equal(t, "2026-11-23", msg["window_end"])
	require.Equal(t, started.Format(time.RFC3339), msg["started_at"])
	require.Equal(t, finished.Format(time.RFC3339), msg["finished_at"])
	require.NotEmpty(t, msg["timestamp"])
}
