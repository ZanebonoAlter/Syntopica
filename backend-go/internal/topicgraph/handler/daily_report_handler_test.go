package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
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
	msg := buildDoneMessage("job-1", 1, 1)

	require.Equal(t, "daily_report_done", msg["type"])
	require.Equal(t, "job-1", msg["job_id"])
	require.Equal(t, 1, msg["total_saved"])
	require.Equal(t, 1, msg["total_boards"])
	require.NotEmpty(t, msg["timestamp"])
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
