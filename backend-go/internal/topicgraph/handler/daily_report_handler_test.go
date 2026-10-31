package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

func TestFilterVisibleTopics_HidesObservingCandidates(t *testing.T) {
	topics := []repository.BoardPersistentTopic{
		{ID: 1, Status: repository.TopicStatusActive, ConsecutiveHits: 0, HitCount: 1},
		{ID: 2, Status: repository.TopicStatusArchived, ConsecutiveHits: 0, HitCount: 1},
		{ID: 3, Status: repository.TopicStatusCandidate, ConsecutiveHits: 0, HitCount: 2}, // below threshold 3 (by hit_count)
		{ID: 4, Status: repository.TopicStatusCandidate, ConsecutiveHits: 0, HitCount: 3}, // meets threshold (by hit_count)
		{ID: 5, Status: repository.TopicStatusCandidate, ConsecutiveHits: 1, HitCount: 5}, // above threshold; cons kept low to prove hit_count is the gate
	}
	result := repository.FilterVisibleTopics(topics, 3)
	require.Len(t, result, 4)
	ids := make([]uint, len(result))
	for i, topic := range result {
		ids[i] = topic.ID
	}
	require.ElementsMatch(t, []uint{1, 2, 4, 5}, ids)
	// Verify the observing candidate is excluded
	for _, topic := range result {
		assert.NotEqual(t, uint(3), topic.ID, "observing candidate id=3 must not be visible")
	}
}

// TestFilterVisibleTopics_UsesHitCountNotConsecutive confirms the visibility
// gate is cumulative hit_count, NOT consecutive_hits: a candidate with high
// consecutive_hits but low hit_count is hidden, and one with low consecutive
// but high hit_count is shown.
func TestFilterVisibleTopics_UsesHitCountNotConsecutive(t *testing.T) {
	topics := []repository.BoardPersistentTopic{
		// high consecutive (5) but low hit_count (1) → hidden (underqualified by cumulative)
		{ID: 1, Status: repository.TopicStatusCandidate, ConsecutiveHits: 5, HitCount: 1},
		// low consecutive (0) but high hit_count (3) → shown (qualified by cumulative)
		{ID: 2, Status: repository.TopicStatusCandidate, ConsecutiveHits: 0, HitCount: 3},
	}
	result := repository.FilterVisibleTopics(topics, 3)
	require.Len(t, result, 1)
	require.Equal(t, uint(2), result[0].ID, "only cumulative hit_count>=threshold qualifies, regardless of consecutive")
}

func TestBuildDailyReportProgressMessageMatchesFrontendContract(t *testing.T) {
	msg := buildProgressMessage("job-1", "generating", 2849, "刚果（金）局势", 0, "0/1")

	require.Equal(t, "daily_report_progress", msg["type"])
	require.Equal(t, "job-1", msg["job_id"])
	require.Equal(t, uint(2849), msg["board_id"])
	require.Equal(t, "刚果（金）局势", msg["board_name"])
	require.Equal(t, "generating", msg["status"])
	require.Equal(t, 0, msg["saved"])
	require.Equal(t, "0/1", msg["progress"])
	require.NotEmpty(t, msg["timestamp"])
}

func TestBuildDailyReportDoneMessageMatchesFrontendContract(t *testing.T) {
	msg := buildDoneMessage("job-1", dailyReportCounts{saved: 1}, 1)

	require.Equal(t, "daily_report_done", msg["type"])
	require.Equal(t, "job-1", msg["job_id"])
	require.Equal(t, 1, msg["total_saved"])
	require.Equal(t, 1, msg["total_boards"])
	require.Equal(t, 0, msg["failed_count"])
	require.Equal(t, 0, msg["empty_count"])
	require.NotEmpty(t, msg["timestamp"])
}

// 终态消息自带三态计数：成功 / 真失败 / 当日无内容，恒等式
// total_saved + failed_count + empty_count == total_boards 恒成立。
func TestBuildDailyReportDoneMessageCarriesThreeStateCounts(t *testing.T) {
	msg := buildDoneMessage("job-2", dailyReportCounts{saved: 1, failed: 1, empty: 1}, 3)

	require.Equal(t, 1, msg["total_saved"], "成功板块数")
	require.Equal(t, 1, msg["failed_count"], "真失败板块数")
	require.Equal(t, 1, msg["empty_count"], "当日无内容的板块数")
	require.Equal(t, 3, msg["total_boards"])

	totalSaved, ok := msg["total_saved"].(int)
	require.True(t, ok)
	failed, ok := msg["failed_count"].(int)
	require.True(t, ok)
	empty, ok := msg["empty_count"].(int)
	require.True(t, ok)
	totalBoards, ok := msg["total_boards"].(int)
	require.True(t, ok)
	require.Equal(t, totalBoards, totalSaved+failed+empty, "三态恒等式：成功 + 失败 + 无内容 == 板块总数")
}

// ── offline-catchup 2.5：重建窗口守卫（HTTP 入口） ──
//
// 与调度器 TriggerNowWithDate 同口径：早于 tag_edge_retention_days 下界的日期
// 一律 4xx，消息说明标签边已按窗口回收；窗口内（含下界当天）照常异步触发。

func setupDailyReportGuardTest(t *testing.T, retentionSetting string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:dailyrepguard-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	// Install (never restore) the repository singleton, mirroring the other
	// handler tests: the success path spawns a worker goroutine that reads
	// repository.Repo, so the DB handle must stay valid for the whole binary.
	repository.InitRepository(db)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{},
		&repository.BoardDailyReport{},
	))
	if retentionSetting != "" {
		require.NoError(t, db.Create(&models.AISettings{Key: tagging.TagEdgeRetentionDaysKey, Value: retentionSetting}).Error)
	}

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	return engine
}

func postGenerateDailyReport(t *testing.T, engine *gin.Engine, date string) (int, map[string]interface{}) {
	t.Helper()
	return postGenerateDailyReportBody(t, engine, map[string]interface{}{"date": date})
}

// postGenerateDailyReportBody posts an arbitrary generate payload (with or
// without board_id) and decodes the JSON response.
func postGenerateDailyReportBody(t *testing.T, engine *gin.Engine, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	payloadBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/daily-reports/generate", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	return w.Code, payload
}

// Scenario「超窗日期拒绝重建」: 4xx + window wording, and the existing report of
// that day is left untouched (never overwritten by an empty rebuild).
func TestTriggerGenerateDailyReportRetentionGuardRejectsOutsideWindow(t *testing.T) {
	engine := setupDailyReportGuardTest(t, "")

	today := time.Now().In(time.Local)
	outside := today.AddDate(0, 0, -8) // default window is 7 days

	seeded := repository.BoardDailyReport{
		SemanticBoardID: 42,
		PeriodDate:      repository.NormalizeReportDate(outside),
		Title:           "pre-existing good report",
		Status:          "completed",
	}
	require.NoError(t, repository.Repo.DB().Create(&seeded).Error)

	status, payload := postGenerateDailyReport(t, engine, outside.Format("2006-01-02"))

	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, false, payload["success"])
	require.Equal(t, "out_of_retention_window", payload["reason"])
	message, ok := payload["error"].(string)
	require.True(t, ok, "error must carry the user-facing explanation: %v", payload)
	require.Contains(t, message, "保留窗口")
	require.Contains(t, message, "标签边")
	require.Contains(t, message, "拒绝重建")

	var stored repository.BoardDailyReport
	require.NoError(t, repository.Repo.DB().First(&stored, seeded.ID).Error)
	require.Equal(t, "pre-existing good report", stored.Title, "rejected rebuild must not touch the existing report")
}

// Scenario「窗口内日期正常重建」+ 边界「date == 下界放行」: the guard is not what
// rejects in-window dates, including the boundary day itself.
func TestTriggerGenerateDailyReportRetentionGuardAllowsInsideWindow(t *testing.T) {
	cases := []struct {
		name      string
		setting   string
		offsetDay int
	}{
		{name: "default window inside", setting: "", offsetDay: 3},
		{name: "default window boundary", setting: "", offsetDay: 7},
		{name: "configured window inside", setting: "3", offsetDay: 1},
		{name: "configured window boundary", setting: "3", offsetDay: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := setupDailyReportGuardTest(t, tc.setting)
			// 受理即异步占真闸，连续子测试会相互 409——装恒真 seam 隔离
			// （闸互斥本身由 gate/409 专门用例覆盖）。
			stubGenerationGatePermissive(t)
			date := time.Now().In(time.Local).AddDate(0, 0, -tc.offsetDay)

			status, payload := postGenerateDailyReport(t, engine, date.Format("2006-01-02"))

			require.Equal(t, http.StatusOK, status, "in-window date must not be rejected: %v", payload)
			require.Equal(t, true, payload["success"])
			require.NotEqual(t, "out_of_retention_window", payload["reason"])
		})
	}
}

// ── guard-daily-report-generate 3.1：手动端点接域闸 ──
//
// 契约锚：spec Scenario「生成进行中重复手动触发」「管线结束后闸释放」「手动
// 触发与定时调度互斥不产生重复通知」。闸与调度 DailyReportJob 同一把
// （service/generation_gate.go）；测试经包级 seam 桩接，不碰真 LLM 管线。

// stubGenerationGatePermissive installs an always-acquire gate stub and a
// no-op release, for in-window acceptance cases: consecutive requests never
// race on the real gate (their worker goroutines may still be holding it).
func stubGenerationGatePermissive(t *testing.T) {
	t.Helper()
	prevAcquire, prevRelease := tryAcquireGenerationGate, releaseGenerationGate
	tryAcquireGenerationGate = func() bool { return true }
	releaseGenerationGate = func() {}
	t.Cleanup(func() {
		tryAcquireGenerationGate, releaseGenerationGate = prevAcquire, prevRelease
	})
}

// gateProbe counts acquire attempts/releases while refusing acquisition:
// simulates "a pipeline (manual or scheduled) is already running".
type gateProbe struct {
	acquireAttempts int32
	releaseCalls    int32
}

func (p *gateProbe) stub(t *testing.T) {
	t.Helper()
	prevAcquire, prevRelease := tryAcquireGenerationGate, releaseGenerationGate
	tryAcquireGenerationGate = func() bool {
		atomic.AddInt32(&p.acquireAttempts, 1)
		return false
	}
	releaseGenerationGate = func() { atomic.AddInt32(&p.releaseCalls, 1) }
	t.Cleanup(func() {
		tryAcquireGenerationGate, releaseGenerationGate = prevAcquire, prevRelease
	})
}

// genEntryCounter swaps the generation entry point for a counting no-op stub.
// Returns the call counter; the rejected-path cases assert it stays zero.
func genEntryCounter(t *testing.T) *int32 {
	t.Helper()
	var calls int32
	prev := generateReportEntry
	generateReportEntry = func(_ context.Context, _ uint, _ time.Time) (*repository.BoardDailyReport, error) {
		atomic.AddInt32(&calls, 1)
		return nil, nil
	}
	t.Cleanup(func() { generateReportEntry = prev })
	return &calls
}

// Scenario「生成进行中重复手动触发」（全板/单板各一）：闸被占时 409
// already_running，不启第二条管线（生成入口未被调），无终态广播面（未拿闸
// 无可释放）。文案与 TriggerNowWithDate 同口径。
func TestTriggerGenerateDailyReportAlreadyRunning(t *testing.T) {
	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{name: "all boards", body: map[string]interface{}{"date": time.Now().Format("2006-01-02")}},
		{name: "single board", body: map[string]interface{}{"date": time.Now().Format("2006-01-02"), "board_id": 7}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := setupDailyReportGuardTest(t, "")
			probe := &gateProbe{}
			probe.stub(t)
			genCalls := genEntryCounter(t)

			status, payload := postGenerateDailyReportBody(t, engine, tc.body)

			require.Equal(t, http.StatusConflict, status)
			require.Equal(t, false, payload["success"])
			require.Equal(t, "already_running", payload["reason"])
			msg, ok := payload["error"].(string)
			require.True(t, ok, "error must be a string: %v", payload)
			require.Equal(t, "日报生成正在执行中，请稍后再试。", msg)

			require.EqualValues(t, 1, atomic.LoadInt32(&probe.acquireAttempts), "gate checked exactly once")
			require.Zero(t, atomic.LoadInt32(&probe.releaseCalls), "rejected trigger never owned the gate")
			require.Zero(t, atomic.LoadInt32(genCalls), "second pipeline must NOT start")
		})
	}
}

// 白盒（闸校验顺序）：rebuild-window 400 优先于 already_running 409——超窗
// 日期直接拒绝，不尝试占闸。
func TestTriggerGenerateDailyReportRebuildWindowCheckedBeforeGate(t *testing.T) {
	engine := setupDailyReportGuardTest(t, "")
	probe := &gateProbe{}
	probe.stub(t)
	genCalls := genEntryCounter(t)

	outside := time.Now().In(time.Local).AddDate(0, 0, -8) // default window is 7 days
	status, payload := postGenerateDailyReport(t, engine, outside.Format("2006-01-02"))

	require.Equal(t, http.StatusBadRequest, status, "out-of-window rejection wins over the gate: %v", payload)
	require.Equal(t, "out_of_retention_window", payload["reason"])
	require.Zero(t, atomic.LoadInt32(&probe.acquireAttempts), "the gate must not even be tried for an out-of-window date")
	require.Zero(t, atomic.LoadInt32(genCalls))
}

// Scenario「管线结束后闸释放」成功态：受理后 worker goroutine 结束时 defer
// release 必须真释放真闸——下一个请求不再 409。用真闸（非 seam）验证端到端
// defer 挂点；轮询等待异步 goroutine 退出，避免时序脆断。
func TestTriggerGenerateDailyReportReleasesGateAfterRun(t *testing.T) {
	engine := setupDailyReportGuardTest(t, "")

	// 真闸（setup 未动 seam，但显式指回 service 真函数，防未来 setup 改动）。
	prevAcquire, prevRelease := tryAcquireGenerationGate, releaseGenerationGate
	tryAcquireGenerationGate = service.TryAcquireDailyReportGeneration
	releaseGenerationGate = service.ReleaseDailyReportGeneration
	t.Cleanup(func() {
		tryAcquireGenerationGate, releaseGenerationGate = prevAcquire, prevRelease
		service.ReleaseDailyReportGeneration() // 兜底归位
	})
	service.ReleaseDailyReportGeneration()

	// 单板路径 + 无副作用快速 stub：goroutine 秒退，闸即刻释放。
	var genCalls int32
	prevEntry := generateReportEntry
	generateReportEntry = func(_ context.Context, _ uint, _ time.Time) (*repository.BoardDailyReport, error) {
		atomic.AddInt32(&genCalls, 1)
		return nil, nil
	}
	t.Cleanup(func() { generateReportEntry = prevEntry })

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{
		"date":     time.Now().Format("2006-01-02"),
		"board_id": 42,
	})
	require.Equal(t, http.StatusOK, status, "accepted while idle: %v", payload)
	require.Equal(t, true, payload["success"])

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if service.TryAcquireDailyReportGeneration() {
			service.ReleaseDailyReportGeneration()
			require.EqualValues(t, 1, atomic.LoadInt32(&genCalls), "exactly one pipeline ran")
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("gate was not released within 5s after the accepted run — defer release missing or stuck")
}

// Scenario「管线结束后闸释放」panic 态：worker goroutine 生成入口 panic 时
// defer release 在 unwind 中执行，闸不泄漏。测试侧 recover 收 panic 防进程崩。
func TestGenerateSingleBoardReleasesGateOnPanic(t *testing.T) {
	engine := setupDailyReportGuardTest(t, "") // repository.Repo for board name lookup
	_ = engine

	// 模拟 handler 已占闸，随后管线 panic。
	require.True(t, service.TryAcquireDailyReportGeneration(), "setup: gate must be free")
	t.Cleanup(service.ReleaseDailyReportGeneration)

	prevEntry := generateReportEntry
	generateReportEntry = func(_ context.Context, _ uint, _ time.Time) (*repository.BoardDailyReport, error) {
		panic("simulated pipeline crash")
	}
	t.Cleanup(func() { generateReportEntry = prevEntry })

	func() {
		defer func() { _ = recover() }() // 收 panic：被测点是 defer release 的 unwind 执行
		generateSingleBoard(42, time.Now(), "job-panic")
	}()

	require.True(t, service.TryAcquireDailyReportGeneration(), "gate must be released after panic unwind — defer FIRST discipline")
	service.ReleaseDailyReportGeneration()
}

// ── 重新分桶 API（add-intra-lane-theme-bucketing tasks 4.5）────────────────────

// setupRebucketHandlerTest builds a minimal SQLite world for the rebucket
// endpoint's gate paths (404 / 409 / 400). The SQLite dialect skips the
// Postgres advisory lock (LockDailyReportSave), so the gates are exercised
// without a container; the write path itself is covered by the PG service tests.
func setupRebucketHandlerTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:rebucket-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	repository.InitRepository(db)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{},
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.DailyReportThread{},
	))

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	return engine
}

func postRebucket(t *testing.T, engine *gin.Engine, reportID uint, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/daily-reports/%d/rebucket", reportID), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	return w.Code, decoded
}

// TestRebucketHandler covers tasks 4.5 / Scenarios 拒绝非完成态日报、拒绝 watch 物化组.
func TestRebucketHandler(t *testing.T) {
	t.Run("rejects a non-completed report with 409", func(t *testing.T) {
		engine := setupRebucketHandlerTest(t)
		db := repository.Repo.DB()

		report := repository.BoardDailyReport{SemanticBoardID: 1, PeriodDate: time.Now(), Title: "t", Status: "generating"}
		require.NoError(t, db.Create(&report).Error)
		section := repository.DailyReportSection{ReportID: report.ID, ClusterIndex: 0, ClusterLabel: "s",
			ClusterTagIDs: []byte("[]"), Embedding: repository.FloatsToPgVector([]float64{0})}
		require.NoError(t, db.Create(&section).Error)

		status, payload := postRebucket(t, engine, report.ID, map[string]interface{}{"section_id": section.ID})
		assert.Equal(t, http.StatusConflict, status)
		assert.Equal(t, false, payload["success"])
	})

	t.Run("rejects a watch-materialized group with 400", func(t *testing.T) {
		engine := setupRebucketHandlerTest(t)
		db := repository.Repo.DB()

		report := repository.BoardDailyReport{SemanticBoardID: 1, PeriodDate: time.Now(), Title: "t", Status: "completed"}
		require.NoError(t, db.Create(&report).Error)
		section := repository.DailyReportSection{ReportID: report.ID, ClusterIndex: 0, ClusterLabel: "s",
			ClusterTagIDs: []byte("[]"), Embedding: repository.FloatsToPgVector([]float64{0}), LaneTier: "watch_sentence"}
		require.NoError(t, db.Create(&section).Error)

		status, payload := postRebucket(t, engine, report.ID, map[string]interface{}{"section_id": section.ID})
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, false, payload["success"])
	})

	t.Run("unknown report or section is 404", func(t *testing.T) {
		engine := setupRebucketHandlerTest(t)
		db := repository.Repo.DB()

		status, _ := postRebucket(t, engine, 999, map[string]interface{}{"section_id": 1})
		assert.Equal(t, http.StatusNotFound, status)

		report := repository.BoardDailyReport{SemanticBoardID: 1, PeriodDate: time.Now(), Title: "t", Status: "completed"}
		require.NoError(t, db.Create(&report).Error)
		status, _ = postRebucket(t, engine, report.ID, map[string]interface{}{"section_id": 4242})
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("missing section_id is 400", func(t *testing.T) {
		engine := setupRebucketHandlerTest(t)
		status, _ := postRebucket(t, engine, 1, map[string]interface{}{})
		assert.Equal(t, http.StatusBadRequest, status)
	})
}

// ── add-radar-pipeline-status：board_ids 集合作用域 ──────────────────────────
//
// 契约锚：spec Scenario「触发多板块集合生成」「多板块集合共用一个执行闸与一次
// 终态」「集合内单板失败不影响其余板块」「单板块与集合参数互斥」。
//
// 作用域三态（design D1/D2）：
//
//	缺省 / 空数组 → 全量（当日有内容的板块）
//	board_id      → 单板
//	board_ids     → 请求集合（不与当日内容求交：当日无内容的板走"生成 0 篇即完成"）
//
// 观察手段：生成入口 / 闸 / WS 广播三处包级 seam（与既有 generateReportEntry、
// tryAcquireGenerationGate 同款），终态通知读真实 notifications 行。

// setupBoardScopeTest builds a SQLite world where BOTH scopes are observable:
// the all-boards collection path resolves to the seeded board, and the terminal
// notification row is readable (the notification writer resolves the DB through
// the shared database.DB handle).
//
// The four collection tables are created by raw DDL instead of AutoMigrate:
// CollectBoardIDsForDate only reads a handful of columns and the models carry
// pgvector columns SQLite cannot migrate.
func setupBoardScopeTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:boardscope-%s?mode=memory&cache=shared", name)), &gorm.Config{})
	require.NoError(t, err)
	repository.InitRepository(db)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{},
		&models.Notification{},
	))

	// The notification writer resolves the DB through database.DB.
	prevDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = prevDB })

	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS topic_tags (id INTEGER PRIMARY KEY, status TEXT, category TEXT)`,
		`CREATE TABLE IF NOT EXISTS article_topic_tags (id INTEGER PRIMARY KEY, topic_tag_id INTEGER, article_id INTEGER)`,
		`CREATE TABLE IF NOT EXISTS articles (id INTEGER PRIMARY KEY, pub_date DATETIME)`,
		`CREATE TABLE IF NOT EXISTS topic_tag_board_labels (id INTEGER PRIMARY KEY, topic_tag_id INTEGER, semantic_board_id INTEGER)`,
		`CREATE TABLE IF NOT EXISTS semantic_labels (id INTEGER PRIMARY KEY, label TEXT)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	// The in-memory DB is shared per test name (and survives a -count>1 rerun):
	// start from an empty mailbox so the terminal-notification assertions
	// count this run's rows only.
	require.NoError(t, db.Exec(`DELETE FROM notifications`).Error)

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	return engine
}

// seedBoardScopeDay seeds one article carrying one active event tag bound to
// boardID on the given date — the all-boards collection then resolves to
// exactly [boardID].
func seedBoardScopeDay(t *testing.T, boardID uint, date time.Time) {
	t.Helper()
	db := repository.Repo.DB()
	require.NoError(t, db.Exec(`INSERT OR REPLACE INTO topic_tags (id, status, category) VALUES (1, 'active', 'event')`).Error)
	require.NoError(t, db.Exec(`INSERT OR REPLACE INTO articles (id, pub_date) VALUES (1, ?)`, date).Error)
	require.NoError(t, db.Exec(`INSERT OR REPLACE INTO article_topic_tags (id, topic_tag_id, article_id) VALUES (1, 1, 1)`).Error)
	require.NoError(t, db.Exec(`INSERT OR REPLACE INTO topic_tag_board_labels (id, topic_tag_id, semantic_board_id) VALUES (1, 1, ?)`, boardID).Error)
}

// seedBoardLabels gives the boards a readable name (the pipeline resolves the
// display name through semantic_labels).
func seedBoardLabels(t *testing.T, ids ...uint) {
	t.Helper()
	db := repository.Repo.DB()
	for _, id := range ids {
		require.NoError(t, db.Exec(`INSERT OR REPLACE INTO semantic_labels (id, label) VALUES (?, ?)`, id, fmt.Sprintf("板块 %d", id)).Error)
	}
}

// generationRecorder stubs the generation entry point and records the boards
// the pipeline asked for (order preserved) plus the boards that must fail.
type generationRecorder struct {
	mu       sync.Mutex
	boards   []uint
	failures map[uint]bool
	empties  map[uint]bool
}

func newGenerationRecorder(failing ...uint) *generationRecorder {
	rec := &generationRecorder{failures: map[uint]bool{}, empties: map[uint]bool{}}
	for _, id := range failing {
		rec.failures[id] = true
	}
	return rec
}

// withEmpty marks boards as "当日无内容"：stub 返回 (nil, nil)——第三种终态
// （既不是成功也不是失败，与通知中心口径一致）。
func (r *generationRecorder) withEmpty(ids ...uint) *generationRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		r.empties[id] = true
	}
	return r
}

func (r *generationRecorder) stub(t *testing.T) {
	t.Helper()
	prev := generateReportEntry
	generateReportEntry = func(_ context.Context, boardID uint, _ time.Time) (*repository.BoardDailyReport, error) {
		r.mu.Lock()
		r.boards = append(r.boards, boardID)
		fail := r.failures[boardID]
		empty := r.empties[boardID]
		r.mu.Unlock()
		if fail {
			return nil, errors.New("simulated generation failure")
		}
		if empty {
			return nil, nil
		}
		return &repository.BoardDailyReport{SemanticBoardID: boardID}, nil
	}
	t.Cleanup(func() { generateReportEntry = prev })
}

// requested returns the boards the pipeline asked for, in call order.
func (r *generationRecorder) requested() []uint {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]uint, len(r.boards))
	copy(out, r.boards)
	return out
}

// progressRecord / doneRecord mirror the WebSocket payload fields the
// terminal-broadcast contract is about.
type progressRecord struct {
	jobID    string
	status   string
	boardID  uint
	progress string
}

type doneRecord struct {
	jobID       string
	totalSaved  int
	totalBoards int
	failedCount int
	emptyCount  int
}

// broadcastRecorder captures the daily-report WebSocket messages so the "one
// terminal broadcast per request" contract is observable without a live
// WebSocket client.
type broadcastRecorder struct {
	mu       sync.Mutex
	progress []progressRecord
	done     []doneRecord
}

func (r *broadcastRecorder) stub(t *testing.T) {
	t.Helper()
	prevProgress, prevDone := broadcastProgress, broadcastDone
	broadcastProgress = func(jobID string, status string, boardID uint, boardName string, saved int, progress string) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.progress = append(r.progress, progressRecord{jobID: jobID, status: status, boardID: boardID, progress: progress})
	}
	broadcastDone = func(jobID string, counts dailyReportCounts, totalBoards int) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.done = append(r.done, doneRecord{
			jobID:       jobID,
			totalSaved:  counts.saved,
			totalBoards: totalBoards,
			failedCount: counts.failed,
			emptyCount:  counts.empty,
		})
	}
	t.Cleanup(func() { broadcastProgress, broadcastDone = prevProgress, prevDone })
}

// doneFor returns the terminal broadcasts of one job (a leftover pipeline of
// an earlier test carries a different job_id and is filtered out).
func (r *broadcastRecorder) doneFor(jobID string) []doneRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []doneRecord{}
	for _, d := range r.done {
		if d.jobID == jobID {
			out = append(out, d)
		}
	}
	return out
}

// progressFor returns the per-board progress broadcasts of one job.
func (r *broadcastRecorder) progressFor(jobID string) []progressRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []progressRecord{}
	for _, p := range r.progress {
		if p.jobID == jobID {
			out = append(out, p)
		}
	}
	return out
}

// gateCounter is a permissive generation-gate stub that counts acquisitions
// and releases — the "one request = one gate" contract must be observable.
type gateCounter struct {
	acquire int32
	release int32
}

func (g *gateCounter) stub(t *testing.T) {
	t.Helper()
	prevAcquire, prevRelease := tryAcquireGenerationGate, releaseGenerationGate
	tryAcquireGenerationGate = func() bool {
		atomic.AddInt32(&g.acquire, 1)
		return true
	}
	releaseGenerationGate = func() { atomic.AddInt32(&g.release, 1) }
	t.Cleanup(func() {
		tryAcquireGenerationGate, releaseGenerationGate = prevAcquire, prevRelease
	})
}

func (g *gateCounter) acquired() int { return int(atomic.LoadInt32(&g.acquire)) }
func (g *gateCounter) released() int { return int(atomic.LoadInt32(&g.release)) }

// waitForPipelineEnd blocks until the pipeline goroutine finished: the deferred
// gate release runs right after the terminal notification write, so a release
// count of 1 proves the notification is already on disk (and no DB access
// races with the assertions that follow).
func waitForPipelineEnd(t *testing.T, gate *gateCounter) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if gate.released() >= 1 {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("pipeline did not finish within 10s — defer release missing or the pipeline is stuck")
}

// notificationsForDate returns the terminal notification rows whose title
// carries the given YYYY-MM-DD date — isolates a test's own rows from a
// leftover pipeline goroutine of an earlier test (different date).
func notificationsForDate(t *testing.T, date string) []models.Notification {
	t.Helper()
	rows := mustListNotifications(t)
	out := make([]models.Notification, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(row.Title, date) {
			out = append(out, row)
		}
	}
	return out
}

// boardScopeDate is the date the scope tests generate for: yesterday — inside
// the rebuild retention window, and distinct from the "today" used by the
// earlier tests so a leftover pipeline cannot pollute their notifications.
func boardScopeDate() time.Time {
	return time.Now().In(time.Local).AddDate(0, 0, -1)
}

// Scenario「触发多板块集合生成」+ 分支表 6 行：作用域选择矩阵。缺省/空数组 →
// 全量（当日有内容的板块）；board_id → 单板；board_ids → 请求集合（不与当日
// 内容求交，design D2）；board_id + board_ids → 400 且不取闸、不启管线。
func TestTriggerGenerateDailyReportBoardScope(t *testing.T) {
	const seededBoard = uint(777)

	cases := []struct {
		name         string
		body         map[string]interface{}
		wantBoards   []uint
		wantTotal    int
		wantConflict bool
	}{
		{name: "缺省 → 全量", body: map[string]interface{}{}, wantBoards: []uint{seededBoard}, wantTotal: 1},
		{name: "空数组 → 全量", body: map[string]interface{}{"board_ids": []int{}}, wantBoards: []uint{seededBoard}, wantTotal: 1},
		{name: "board_id → 单板", body: map[string]interface{}{"board_id": 5}, wantBoards: []uint{5}, wantTotal: 1},
		{name: "board_ids → 集合", body: map[string]interface{}{"board_ids": []int{3, 7, 9}}, wantBoards: []uint{3, 7, 9}, wantTotal: 3},
		{name: "board_id 与 board_ids 互斥 → 400", body: map[string]interface{}{"board_id": 5, "board_ids": []int{3}}, wantConflict: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := setupBoardScopeTest(t)
			scopeDate := boardScopeDate()
			dateStr := scopeDate.Format("2006-01-02")
			seedBoardScopeDay(t, seededBoard, scopeDate)
			seedBoardLabels(t, seededBoard, 3, 5, 7, 9)

			gate := &gateCounter{}
			gate.stub(t)
			gen := newGenerationRecorder()
			gen.stub(t)
			bcasts := &broadcastRecorder{}
			bcasts.stub(t)

			body := map[string]interface{}{"date": dateStr}
			for k, v := range tc.body {
				body[k] = v
			}

			status, payload := postGenerateDailyReportBody(t, engine, body)

			if tc.wantConflict {
				require.Equal(t, http.StatusBadRequest, status, "互斥参数必须 400: %v", payload)
				require.Equal(t, false, payload["success"])
				require.Equal(t, "conflicting_board_scope", payload["reason"])
				require.Zero(t, gate.acquired(), "互斥 400 绝不取闸")
				require.Empty(t, gen.requested(), "互斥 400 绝不启动管线")
				require.Empty(t, bcasts.done, "互斥 400 不产生任何终态广播")
				require.Empty(t, notificationsForDate(t, dateStr), "互斥 400 不产生任何终态通知")
				return
			}

			require.Equal(t, http.StatusOK, status, "作用域请求必须受理: %v", payload)
			data, ok := payload["data"].(map[string]interface{})
			require.True(t, ok, "data must be an object: %v", payload)
			jobID, ok := data["job_id"].(string)
			require.True(t, ok, "job_id must be a string: %v", data)
			require.Equal(t, "processing", data["status"])

			waitForPipelineEnd(t, gate)

			require.Equal(t, tc.wantBoards, gen.requested(), "生成作用域必须与请求一致")
			done := bcasts.doneFor(jobID)
			require.Len(t, done, 1, "一个请求 = 一次终态广播")
			require.Equal(t, tc.wantTotal, done[0].totalBoards)
			require.Len(t, notificationsForDate(t, dateStr), 1, "一个请求 = 一条终态通知")
		})
	}
}

// Scenario「多板块集合共用一个执行闸与一次终态」：一次请求 = 一个闸 = 一个
// job_id = 一次终态广播 = 一条终态通知。
func TestTriggerGenerateDailyReportBoardScopeSharesGateAndTerminal(t *testing.T) {
	engine := setupBoardScopeTest(t)
	scopeDate := boardScopeDate()
	dateStr := scopeDate.Format("2006-01-02")
	seedBoardScopeDay(t, 777, scopeDate)
	seedBoardLabels(t, 777, 3, 7, 9)

	gate := &gateCounter{}
	gate.stub(t)
	gen := newGenerationRecorder()
	gen.stub(t)
	bcasts := &broadcastRecorder{}
	bcasts.stub(t)

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{
		"date":      dateStr,
		"board_ids": []int{3, 7, 9},
	})
	require.Equal(t, http.StatusOK, status, "collection scope must be accepted: %v", payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	jobID, ok := data["job_id"].(string)
	require.True(t, ok)

	require.Equal(t, 1, gate.acquired(), "一个请求 = 一次取闸")
	waitForPipelineEnd(t, gate)
	require.Equal(t, 1, gate.released(), "一个请求 = 一次释放（管线结束即释放）")

	require.Equal(t, []uint{3, 7, 9}, gen.requested(), "只生成请求集合内的板块")
	done := bcasts.doneFor(jobID)
	require.Len(t, done, 1, "集合共用一个 job_id = 一次终态广播")
	require.Equal(t, 3, done[0].totalBoards)
	require.Equal(t, 3, done[0].totalSaved)

	rows := notificationsForDate(t, dateStr)
	require.Len(t, rows, 1, "集合共用一条终态通知")
	require.Equal(t, "success", rows[0].Type)
	require.Contains(t, rows[0].Summary, fmt.Sprintf("共 %d 个版面", done[0].totalBoards),
		"通知的总版面数必须与终态广播的 total_boards 一致")
	require.Contains(t, rows[0].Summary, fmt.Sprintf("%d 个版面已产出日报", done[0].totalSaved),
		"通知的成功数是产出日报的板块数，与终态广播的 total_saved 一致")
}

// Scenario「集合内单板失败不影响其余板块」：#7 失败，#3/#9 照常完成；终态
// 广播与终态通知各一条，通知含成功 + 失败计数。
func TestTriggerGenerateDailyReportBoardScopePartialFailure(t *testing.T) {
	engine := setupBoardScopeTest(t)
	scopeDate := boardScopeDate()
	dateStr := scopeDate.Format("2006-01-02")
	seedBoardScopeDay(t, 777, scopeDate)
	seedBoardLabels(t, 777, 3, 7, 9)

	gate := &gateCounter{}
	gate.stub(t)
	gen := newGenerationRecorder(7) // #7 fails
	gen.stub(t)
	bcasts := &broadcastRecorder{}
	bcasts.stub(t)

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{
		"date":      dateStr,
		"board_ids": []int{3, 7, 9},
	})
	require.Equal(t, http.StatusOK, status, "partial failure must not fail the request: %v", payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	jobID, ok := data["job_id"].(string)
	require.True(t, ok)

	waitForPipelineEnd(t, gate)

	require.Equal(t, []uint{3, 7, 9}, gen.requested(), "失败板不中断其余板：三个板都被尝试")
	done := bcasts.doneFor(jobID)
	require.Len(t, done, 1, "部分失败仍然只有一次终态广播")
	require.Equal(t, 3, done[0].totalBoards)
	require.Equal(t, 2, done[0].totalSaved, "成功 2 板")

	progress := bcasts.progressFor(jobID)
	failedBoards := map[uint]bool{}
	completedBoards := map[uint]bool{}
	for _, p := range progress {
		switch p.status {
		case "failed":
			failedBoards[p.boardID] = true
		case "completed":
			completedBoards[p.boardID] = true
		}
	}
	require.True(t, failedBoards[7], "#7 必须报 failed")
	require.True(t, completedBoards[3] && completedBoards[9], "#3/#9 必须报 completed")

	rows := notificationsForDate(t, dateStr)
	require.Len(t, rows, 1, "部分失败仍然只有一条终态通知")
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "2 成功 1 失败", "终态通知必须同时体现成功与失败计数")
}

// 终态计数三态恒等式：total_saved + failed_count + empty_count == total_boards。
// 「当日无内容」的板块（report == nil）计入 empty_count——既不算成功也不算
// 失败，与终态通知口径（成功/失败计数）一致。
func TestTerminalBroadcastCountsThreeStates(t *testing.T) {
	engine := setupBoardScopeTest(t)
	scopeDate := boardScopeDate()
	dateStr := scopeDate.Format("2006-01-02")
	seedBoardScopeDay(t, 777, scopeDate)
	seedBoardLabels(t, 777, 3, 7, 9)

	gate := &gateCounter{}
	gate.stub(t)
	gen := newGenerationRecorder(9) // #9 真失败
	gen.withEmpty(7)                // #7 当日无内容（report == nil）
	gen.stub(t)
	bcasts := &broadcastRecorder{}
	bcasts.stub(t)

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{
		"date":      dateStr,
		"board_ids": []int{3, 7, 9},
	})
	require.Equal(t, http.StatusOK, status, "集合生成必须受理: %v", payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	jobID, ok := data["job_id"].(string)
	require.True(t, ok)

	waitForPipelineEnd(t, gate)

	done := bcasts.doneFor(jobID)
	require.Len(t, done, 1, "一个请求 = 一次终态广播")
	require.Equal(t, 3, done[0].totalBoards)
	require.Equal(t, 1, done[0].totalSaved, "只有 #3 产出日报")
	require.Equal(t, 1, done[0].failedCount, "只有 #9 真失败")
	require.Equal(t, 1, done[0].emptyCount, "#7 当日无内容：计入 empty，不计入失败")
	require.Equal(t, done[0].totalBoards, done[0].totalSaved+done[0].failedCount+done[0].emptyCount,
		"三态恒等式：total_saved + failed_count + empty_count == total_boards")

	// 终态通知与终态广播同口径：无内容的板块不进失败数，且「共 N 个版面」
	// 的 N == 终态广播的 total_boards（含当日无内容的板块）。
	rows := notificationsForDate(t, dateStr)
	require.Len(t, rows, 1, "一个请求 = 一条终态通知")
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, "1 成功 1 失败", "「当日无内容」的板块不算失败")
	require.Contains(t, rows[0].Summary, fmt.Sprintf("共 %d 个版面", done[0].totalBoards),
		"通知的总版面数必须与终态广播的 total_boards 一致（含当日无内容的板块）")
	require.Contains(t, rows[0].Summary, fmt.Sprintf("%d 个当日无内容", done[0].emptyCount),
		"通知必须显式给出当日无内容的板块数")
	require.NotContains(t, rows[0].Summary, "2 成功", "当日无内容的板块不计成功")
}

// 单板块路径的「当日无内容」：终态 empty_count = 1、failed_count = 0，
// 恒等式成立；终态通知是完成通知（不是失败汇总）。
func TestGenerateSingleBoardEmptyIsNotFailure(t *testing.T) {
	engine := setupBoardScopeTest(t)
	scopeDate := boardScopeDate()
	dateStr := scopeDate.Format("2006-01-02")
	seedBoardScopeDay(t, 777, scopeDate)
	seedBoardLabels(t, 777, 5)

	gate := &gateCounter{}
	gate.stub(t)
	gen := newGenerationRecorder()
	gen.withEmpty(5)
	gen.stub(t)
	bcasts := &broadcastRecorder{}
	bcasts.stub(t)

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{
		"date":     dateStr,
		"board_id": 5,
	})
	require.Equal(t, http.StatusOK, status, "单板生成必须受理: %v", payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	jobID, ok := data["job_id"].(string)
	require.True(t, ok)

	waitForPipelineEnd(t, gate)

	done := bcasts.doneFor(jobID)
	require.Len(t, done, 1, "一个请求 = 一次终态广播")
	require.Equal(t, 1, done[0].totalBoards)
	require.Equal(t, 0, done[0].totalSaved, "无内容的板块不产出日报")
	require.Equal(t, 0, done[0].failedCount, "当日无内容不算失败")
	require.Equal(t, 1, done[0].emptyCount, "当日无内容计入 empty")
	require.Equal(t, done[0].totalBoards, done[0].totalSaved+done[0].failedCount+done[0].emptyCount,
		"三态恒等式：total_saved + failed_count + empty_count == total_boards")

	rows := notificationsForDate(t, dateStr)
	require.Len(t, rows, 1, "一个请求 = 一条终态通知")
	require.Equal(t, "success", rows[0].Type, "无内容的板块不产生失败汇总通知")
	require.Contains(t, rows[0].Summary, fmt.Sprintf("共 %d 个版面", done[0].totalBoards),
		"通知的总版面数必须与终态广播的 total_boards 一致")
	require.Contains(t, rows[0].Summary, fmt.Sprintf("%d 个当日无内容", done[0].emptyCount),
		"通知必须显式给出当日无内容的板块数")
}

// 收集阶段失败的早退路径：终态广播与终态通知同口径——广播记
// total_boards=1 / failed_count=1，通知读作「共 1 个版面，0 成功 1 失败」
// （两条通道对"总版面数"说的是同一件事）。
func TestCollectFailureNotificationMatchesTerminalBroadcast(t *testing.T) {
	engine := setupBoardScopeTest(t)
	scopeDate := boardScopeDate()
	dateStr := scopeDate.Format("2006-01-02")

	// 让板块收集阶段直接失败（缺表 → SQL 报错）
	require.NoError(t, repository.Repo.DB().Exec(`DROP TABLE articles`).Error)

	gate := &gateCounter{}
	gate.stub(t)
	gen := newGenerationRecorder()
	gen.stub(t)
	bcasts := &broadcastRecorder{}
	bcasts.stub(t)

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{"date": dateStr})
	require.Equal(t, http.StatusOK, status, "全量生成必须受理: %v", payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	jobID, ok := data["job_id"].(string)
	require.True(t, ok)

	waitForPipelineEnd(t, gate)

	done := bcasts.doneFor(jobID)
	require.Len(t, done, 1, "一个请求 = 一次终态广播")
	require.Equal(t, 1, done[0].totalBoards)
	require.Equal(t, 0, done[0].totalSaved)
	require.Equal(t, 1, done[0].failedCount)
	require.Equal(t, 0, done[0].emptyCount)

	rows := notificationsForDate(t, dateStr)
	require.Len(t, rows, 1, "一个请求 = 一条终态通知")
	require.Equal(t, "error", rows[0].Type)
	require.Contains(t, rows[0].Summary, fmt.Sprintf("共 %d 个版面", done[0].totalBoards),
		"通知的总版面数必须与终态广播的 total_boards 一致")
	require.Contains(t, rows[0].Summary, fmt.Sprintf("%d 成功 %d 失败", done[0].totalSaved, done[0].failedCount),
		"通知的成功/失败数必须与终态广播的 total_saved/failed_count 一致")
}

// Scenario「集合内重复板块折叠」：board_ids: [3, 7, 3] → 只为 #3 与 #7 各
// 生成一次（#3 不重复生成），终态计数不重复计入 #3。
func TestTriggerGenerateDailyReportBoardScopeDedupesRepeatedBoards(t *testing.T) {
	engine := setupBoardScopeTest(t)
	scopeDate := boardScopeDate()
	dateStr := scopeDate.Format("2006-01-02")
	seedBoardScopeDay(t, 777, scopeDate)
	seedBoardLabels(t, 777, 3, 7)

	gate := &gateCounter{}
	gate.stub(t)
	gen := newGenerationRecorder()
	gen.stub(t)
	bcasts := &broadcastRecorder{}
	bcasts.stub(t)

	status, payload := postGenerateDailyReportBody(t, engine, map[string]interface{}{
		"date":      dateStr,
		"board_ids": []int{3, 7, 3},
	})
	require.Equal(t, http.StatusOK, status, "含重复 id 的集合必须被受理: %v", payload)
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	jobID, ok := data["job_id"].(string)
	require.True(t, ok)

	waitForPipelineEnd(t, gate)

	require.Equal(t, []uint{3, 7}, gen.requested(), "重复 id 折叠：#3 只生成一次，且保留首次出现顺序")

	done := bcasts.doneFor(jobID)
	require.Len(t, done, 1, "一个请求 = 一次终态广播")
	require.Equal(t, 2, done[0].totalBoards, "终态计数不重复计入 #3")
	require.Equal(t, 2, done[0].totalSaved)

	perBoard := map[uint]int{}
	for _, p := range bcasts.progressFor(jobID) {
		perBoard[p.boardID]++
	}
	require.Equal(t, 2, perBoard[3], "#3 只被生成一轮（generating + completed）")
	require.Equal(t, 2, perBoard[7], "#7 被生成一轮（generating + completed）")

	require.Len(t, notificationsForDate(t, dateStr), 1, "一个请求 = 一条终态通知")
}

// Scenario「集合字段类型非法不降级为全量」+「单板块字段类型非法不降级为全量」：
// board_ids 传对象 / board_id 传字符串 → HTTP 400；闸未被获取、不启动管线、
// 无终态广播、无终态通知（畸形请求 SHALL NOT 触发全站生成管线）。
func TestTriggerGenerateDailyReportRejectsInvalidFieldTypes(t *testing.T) {
	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{name: "board_ids 传对象", body: map[string]interface{}{"board_ids": map[string]interface{}{"id": 3}}},
		{name: "board_id 传字符串", body: map[string]interface{}{"board_id": "5"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := setupBoardScopeTest(t)
			scopeDate := boardScopeDate()
			dateStr := scopeDate.Format("2006-01-02")
			seedBoardScopeDay(t, 777, scopeDate)
			seedBoardLabels(t, 777, 3, 5)

			gate := &gateCounter{}
			gate.stub(t)
			gen := newGenerationRecorder()
			gen.stub(t)
			bcasts := &broadcastRecorder{}
			bcasts.stub(t)

			body := map[string]interface{}{"date": dateStr}
			for k, v := range tc.body {
				body[k] = v
			}

			status, payload := postGenerateDailyReportBody(t, engine, body)

			require.Equal(t, http.StatusBadRequest, status, "类型非法必须 400: %v", payload)
			require.Equal(t, false, payload["success"])
			require.Zero(t, gate.acquired(), "类型非法绝不取闸")
			require.Empty(t, gen.requested(), "类型非法绝不启动管线")
			require.Empty(t, bcasts.done, "类型非法不产生任何终态广播")
			require.Empty(t, notificationsForDate(t, dateStr), "类型非法不产生任何终态通知")
		})
	}
}
