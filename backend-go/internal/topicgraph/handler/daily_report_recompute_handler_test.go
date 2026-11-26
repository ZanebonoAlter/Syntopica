package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── recompute-topic-centroids：存量质心一次性重算 ────────────────────────────
//
// 契约锚：spec Scenario「存量质心重算」（anchor-topic-centroid-with-embedding）。
//
// 跑在内存 SQLite 上（同包的 setupRebucketHandlerTest 同一套路）：端点是异步的
// （返回即受理），而本包其它用例会留下还在写库的日报生成 goroutine——共享
// testcontainer 的 SetupTestDB 复位（TRUNCATE）与它们并发会死锁（SQLSTATE
// 40P01）并把本用例判红。质心公式与 PG 语义由 repository 包的 testcontainer
// 用例兜底（TestComputeTopicCentroid_* / TestRecomputeTopicCentroids_*）。
//
// 覆盖三态（无参 / 带 board_id / 非法 board_id）+ 异步确实重算了质心。
// 每个用例都等到 goroutine 落库后才结束，避免异步任务溢到下一个用例。

// setupRecomputeHandlerTest wires the recompute endpoint against an in-memory
// SQLite database (the endpoint answers immediately; the work runs in a
// background goroutine).
func setupRecomputeHandlerTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:recompute-%s?mode=memory&cache=shared&_pragma=busy_timeout(5000)", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.AISettings{},
		&repository.BoardDailyReport{},
		&repository.DailyReportSection{},
		&repository.BoardPersistentTopic{},
	))
	repository.InitRepository(db)

	engine := gin.New()
	RegisterDailyReportRoutes(engine.Group("/api"))
	return engine, db
}

// postRecomputeCentroids issues the endpoint call and decodes the JSON body.
func postRecomputeCentroids(t *testing.T, engine *gin.Engine, query string) (int, map[string]interface{}) {
	t.Helper()
	url := "/api/daily-reports/recompute-topic-centroids"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodPost, url, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded))
	return w.Code, decoded
}

// seedRecomputeTopic creates an active topic whose 首义 is [1,0,0] and whose
// two hit sections are [0,1,0] — the blended centroid is [0.5,0.5,0].
func seedRecomputeTopic(t *testing.T, db *gorm.DB, boardID uint) uint {
	t.Helper()
	day := time.Now()
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           "recompute-topic",
		Status:          repository.TopicStatusActive,
		Source:          repository.TopicSourceAuto,
		Embedding:       repository.FloatsToPgVector([]float64{1, 0, 0}),
		FirstSeenDate:   day,
		LastSeenDate:    day,
		HitCount:        2,
	}
	require.NoError(t, db.Create(&topic).Error)

	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      day,
		Title:           "recompute report",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)

	for i := 0; i < 2; i++ {
		section := repository.DailyReportSection{
			ReportID:          report.ID,
			ClusterLabel:      "s",
			Embedding:         repository.FloatsToPgVector([]float64{0, 1, 0}),
			PersistentTopicID: &topic.ID,
		}
		require.NoError(t, db.Create(&section).Error)
	}
	return topic.ID
}

// topicCentroid reads the stored centroid of one topic ("" when NULL).
func topicCentroid(t *testing.T, db *gorm.DB, topicID uint) string {
	t.Helper()
	var topic repository.BoardPersistentTopic
	if err := db.First(&topic, topicID).Error; err != nil {
		return ""
	}
	return topic.Centroid
}

// waitForCentroid polls until the async goroutine has written the centroid
// (or the deadline expires).
func waitForCentroid(t *testing.T, db *gorm.DB, topicID uint) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if c := topicCentroid(t, db, topicID); c != "" {
			return c
		}
		time.Sleep(50 * time.Millisecond)
	}
	return ""
}

// parseCentroidVector parses a pgvector literal read back from the DB.
func parseCentroidVector(t *testing.T, s string) []float64 {
	t.Helper()
	trimmed := strings.TrimPrefix(strings.TrimSpace(s), "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	parts := strings.Split(trimmed, ",")
	out := make([]float64, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		require.NoError(t, err)
		out[i] = v
	}
	return out
}

// TestRecomputeTopicCentroids_AcceptsAllBoards: no board_id → the whole site
// is queued and the call returns immediately with status=processing.
func TestRecomputeTopicCentroids_AcceptsAllBoards(t *testing.T) {
	engine, db := setupRecomputeHandlerTest(t)
	topicID := seedRecomputeTopic(t, db, 1)

	status, payload := postRecomputeCentroids(t, engine, "")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok, "response must carry a data object: %v", payload)
	assert.Equal(t, "processing", data["status"])

	assert.NotEmpty(t, waitForCentroid(t, db, topicID),
		"the async run must have recomputed the topic centroid")
}

// TestRecomputeTopicCentroids_ScopedToBoard: board_id narrows the run to one
// board — the other board's topic keeps its (empty) centroid.
func TestRecomputeTopicCentroids_ScopedToBoard(t *testing.T) {
	engine, db := setupRecomputeHandlerTest(t)
	topicA := seedRecomputeTopic(t, db, 11)
	topicB := seedRecomputeTopic(t, db, 22)

	status, payload := postRecomputeCentroids(t, engine, "board_id=11")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok, "response must carry a data object: %v", payload)
	assert.Equal(t, "processing", data["status"])

	assert.NotEmpty(t, waitForCentroid(t, db, topicA),
		"the scoped board's topic must be recomputed")
	assert.Empty(t, topicCentroid(t, db, topicB),
		"a topic on another board must not be touched")
}

// TestRecomputeTopicCentroids_RejectsInvalidBoardID: a malformed board scope
// is rejected before anything is queued (no goroutine is spawned).
func TestRecomputeTopicCentroids_RejectsInvalidBoardID(t *testing.T) {
	engine, _ := setupRecomputeHandlerTest(t)

	for _, query := range []string{"board_id=abc", "board_id=0", "board_id=-1"} {
		status, payload := postRecomputeCentroids(t, engine, query)
		assert.Equal(t, http.StatusBadRequest, status, "query %q must be rejected", query)
		assert.Equal(t, false, payload["success"])
		assert.Equal(t, "invalid board_id", payload["error"])
	}
}

// TestRecomputeTopicCentroids_RewritesCentroids: the accepted call really
// recomputes the stored centroid with the anchor-blend formula.
func TestRecomputeTopicCentroids_RewritesCentroids(t *testing.T) {
	engine, db := setupRecomputeHandlerTest(t)
	topicID := seedRecomputeTopic(t, db, 1)

	assert.Empty(t, topicCentroid(t, db, topicID), "precondition: no centroid yet")

	status, _ := postRecomputeCentroids(t, engine, "")
	require.Equal(t, http.StatusOK, status)

	centroid := waitForCentroid(t, db, topicID)
	require.NotEmpty(t, centroid, "the async recompute must have written the centroid")
	assert.InDeltaSlice(t, []float64{0.5, 0.5, 0}, parseCentroidVector(t, centroid), 1e-6,
		"centroid = 0.5·首义[1,0,0] + 0.5·滚动均值[0,1,0]")
}
