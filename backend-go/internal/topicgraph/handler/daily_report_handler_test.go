package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	body, err := json.Marshal(map[string]string{"date": date})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/daily-reports/generate", bytes.NewReader(body))
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
			date := time.Now().In(time.Local).AddDate(0, 0, -tc.offsetDay)

			status, payload := postGenerateDailyReport(t, engine, date.Format("2006-01-02"))

			require.Equal(t, http.StatusOK, status, "in-window date must not be rejected: %v", payload)
			require.Equal(t, true, payload["success"])
			require.NotEqual(t, "out_of_retention_window", payload["reason"])
		})
	}
}
