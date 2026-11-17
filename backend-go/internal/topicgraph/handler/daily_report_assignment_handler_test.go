package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── 人工归属变更端点（lane-manual-correction D4，故事 B1/B5 + C5） ─────────
// HTTP 契约测试（testcontainer PG）：移除成功 / 加回成功 / 跨 board 加回 4xx /
// 已归属 section 加回 4xx / 移除无归属 4xx / 输入非法 400 / 按日批量移除。

func setupAssignmentHandlerPGTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	r := gin.New()
	api := r.Group("/api")
	RegisterDailyReportRoutes(api)
	return r, db
}

func seedAssignmentBoard(t *testing.T, db *gorm.DB, slug string) uint {
	t.Helper()
	board := models.SemanticLabel{Label: slug, Slug: slug, LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	return board.ID
}

func seedAssignmentReport(t *testing.T, db *gorm.DB, boardID uint, day string) uint {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC).AddDate(0, 0, assignmentDayOffset(day)),
		Title:           "report " + day,
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func assignmentDayOffset(day string) int {
	parsed, err := time.Parse("2006-01-02", day)
	if err != nil {
		return 0
	}
	base := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	return int(parsed.Sub(base).Hours() / 24)
}

func seedAssignmentTopic(t *testing.T, db *gorm.DB, boardID uint, label string, status string) repository.BoardPersistentTopic {
	t.Helper()
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       repository.FloatsToPgVector([]float64{0, 0, 1}),
		Status:          status,
		Source:          repository.TopicSourceAuto,
		FirstSeenDate:   repository.NormalizeReportDate(time.Now()),
		LastSeenDate:    repository.NormalizeReportDate(time.Now()),
		HitCount:        3,
		ConsecutiveHits: 2,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

func seedAssignmentSection(t *testing.T, db *gorm.DB, reportID uint, label string, topicID *uint) repository.DailyReportSection {
	t.Helper()
	sec := repository.DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: label,
		ArticleCount: 1,
		Embedding:    repository.FloatsToPgVector([]float64{1, 0, 0}),
		LaneTier:     "l1_direct",
	}
	if topicID != nil {
		status := repository.TopicStatusActive
		sec.PersistentTopicID = topicID
		sec.TopicMatchDistance = 0.12
		sec.TopicMatchConfidence = repository.TopicConfAnchorHit
		sec.TopicStatusAtReport = &status
	}
	require.NoError(t, db.Create(&sec).Error)
	return sec
}

func patchAssignment(t *testing.T, r *gin.Engine, sectionID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch,
		"/api/daily-reports/sections/"+strconv.FormatUint(uint64(sectionID), 10)+"/assignment",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func postDetachDate(t *testing.T, r *gin.Engine, topicID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost,
		"/api/daily-reports/topics/"+strconv.FormatUint(uint64(topicID), 10)+"/detach-date",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestUpdateSectionAssignment_DetachSuccess covers 故事 B1/B2 over HTTP: the
// four snapshot columns become NULL, lane_tier survives and the audit columns
// record the detach with the topic the node came from.
func TestUpdateSectionAssignment_DetachSuccess(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", &topic.ID)

	w := patchAssignment(t, r, sec.ID, `{"topic_id":null}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			SectionID uint   `json:"section_id"`
			TopicID   *uint  `json:"topic_id"`
			Action    string `json:"action"`
			Changed   bool   `json:"changed"`
			BoardID   uint   `json:"board_id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, sec.ID, resp.Data.SectionID)
	assert.Nil(t, resp.Data.TopicID)
	assert.Equal(t, repository.ManualAssignmentActionDetach, resp.Data.Action)
	assert.True(t, resp.Data.Changed)
	assert.Equal(t, boardID, resp.Data.BoardID)

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	assert.Nil(t, reloaded.PersistentTopicID)
	assert.Empty(t, reloaded.TopicMatchConfidence)
	assert.Zero(t, reloaded.TopicMatchDistance)
	assert.Nil(t, reloaded.TopicStatusAtReport)
	assert.Equal(t, "l1_direct", reloaded.LaneTier)
	require.NotNil(t, reloaded.ManualAssignmentAt)
	require.NotNil(t, reloaded.ManualAssignmentFromTopicID)
	assert.Equal(t, topic.ID, *reloaded.ManualAssignmentFromTopicID)
	assert.Equal(t, repository.ManualAssignmentActionDetach, reloaded.ManualAssignmentAction)
}

// TestUpdateSectionAssignment_AttachSuccess covers 故事 B5 over HTTP: the
// target lane is written, the confidence becomes "manual", the distance is
// recomputed and the audit columns record the attach.
func TestUpdateSectionAssignment_AttachSuccess(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", nil)

	w := patchAssignment(t, r, sec.ID, fmt.Sprintf(`{"topic_id":%d}`, topic.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
	assert.Equal(t, repository.TopicConfManual, reloaded.TopicMatchConfidence)
	assert.InDelta(t, 1.0, reloaded.TopicMatchDistance, 1e-6, "距离按目标锚点实算")
	assert.Nil(t, reloaded.TopicStatusAtReport)
	assert.Equal(t, "l1_direct", reloaded.LaneTier)
	require.NotNil(t, reloaded.ManualAssignmentAt)
	assert.Nil(t, reloaded.ManualAssignmentFromTopicID)
	assert.Equal(t, repository.ManualAssignmentActionAttach, reloaded.ManualAssignmentAction)

	// 目标话题进入待重算队列（质心窗口重新纳入该节点）。
	var reloadedTopic repository.BoardPersistentTopic
	require.NoError(t, db.First(&reloadedTopic, topic.ID).Error)
	assert.True(t, reloadedTopic.CentroidPendingRecompute)
}

// TestUpdateSectionAssignment_RejectsCrossBoardAttach covers spec「加回目标
// 校验」: a target lane on another board is rejected and nothing is written.
func TestUpdateSectionAssignment_RejectsCrossBoardAttach(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	otherBoardID := seedAssignmentBoard(t, db, "assignment-other-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	foreign := seedAssignmentTopic(t, db, otherBoardID, "foreign lane", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", nil)

	w := patchAssignment(t, r, sec.ID, fmt.Sprintf(`{"topic_id":%d}`, foreign.ID))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	assert.Nil(t, reloaded.PersistentTopicID)
	assert.Empty(t, reloaded.TopicMatchConfidence)
	assert.Nil(t, reloaded.ManualAssignmentAt)
}

// TestUpdateSectionAssignment_RejectsReparent covers spec「加回不接受改挂」:
// an assigned node cannot be moved to another lane — it must be removed first.
func TestUpdateSectionAssignment_RejectsReparent(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)
	other := seedAssignmentTopic(t, db, boardID, "lane B", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", &topic.ID)

	w := patchAssignment(t, r, sec.ID, fmt.Sprintf(`{"topic_id":%d}`, other.ID))
	assert.Equal(t, http.StatusConflict, w.Code)

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
	assert.Equal(t, repository.TopicConfAnchorHit, reloaded.TopicMatchConfidence)
	assert.Nil(t, reloaded.ManualAssignmentAt)
}

// TestUpdateSectionAssignment_RejectsDetachWithoutAssignment covers the
// precondition「移除：section 当前归属某话题；否则 4xx」.
func TestUpdateSectionAssignment_RejectsDetachWithoutAssignment(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	sec := seedAssignmentSection(t, db, reportID, "free", nil)

	w := patchAssignment(t, r, sec.ID, `{"topic_id":null}`)
	assert.Equal(t, http.StatusConflict, w.Code)

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	assert.Nil(t, reloaded.PersistentTopicID)
	assert.Nil(t, reloaded.ManualAssignmentAt)
}

// TestUpdateSectionAssignment_RejectsInvalidTopicID covers the input variants
// of 变体走查「assignment topic_id 字符串/负数/0」: all 4xx, nothing written.
func TestUpdateSectionAssignment_RejectsInvalidTopicID(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", &topic.ID)

	bodies := map[string]string{
		"string":   `{"topic_id":"7"}`,
		"negative": `{"topic_id":-1}`,
		"zero":     `{"topic_id":0}`,
		"missing":  `{}`,
		"float":    `{"topic_id":1.5}`,
		"bool":     `{"topic_id":true}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			w := patchAssignment(t, r, sec.ID, body)
			assert.GreaterOrEqual(t, w.Code, 400)
			assert.Less(t, w.Code, 500)
		})
	}

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
	assert.Equal(t, repository.TopicConfAnchorHit, reloaded.TopicMatchConfidence)
	assert.Nil(t, reloaded.ManualAssignmentAt)
}

// TestUpdateSectionAssignment_AttachToSameLaneIsIdempotent covers the
// idempotency variant「section 已属目标时加回 → 幂等 200 不重复留痕」.
func TestUpdateSectionAssignment_AttachToSameLaneIsIdempotent(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", &topic.ID)

	w := patchAssignment(t, r, sec.ID, fmt.Sprintf(`{"topic_id":%d}`, topic.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Data struct {
			Changed bool `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Data.Changed, "幂等加回不写任何列")

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	assert.Equal(t, repository.TopicConfAnchorHit, reloaded.TopicMatchConfidence)
	assert.Nil(t, reloaded.ManualAssignmentAt)
}

// TestDetachTopicByDate_ReleasesThatDay covers 故事 C5 over HTTP: every node
// of that lane on that day is released, the other day is untouched.
func TestDetachTopicByDate_ReleasesThatDay(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	day1 := seedAssignmentReport(t, db, boardID, "2026-11-10")
	day2 := seedAssignmentReport(t, db, boardID, "2026-11-11")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)

	s1 := seedAssignmentSection(t, db, day1, "d1-a", &topic.ID)
	s2 := seedAssignmentSection(t, db, day1, "d1-b", &topic.ID)
	s3 := seedAssignmentSection(t, db, day2, "d2-a", &topic.ID)

	w := postDetachDate(t, r, topic.ID, `{"period_date":"2026-11-10"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TopicID    uint   `json:"topic_id"`
			PeriodDate string `json:"period_date"`
			Detached   int    `json:"detached"`
			SectionIDs []uint `json:"section_ids"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, topic.ID, resp.Data.TopicID)
	assert.Equal(t, "2026-11-10", resp.Data.PeriodDate)
	assert.Equal(t, 2, resp.Data.Detached)
	assert.Equal(t, []uint{s1.ID, s2.ID}, resp.Data.SectionIDs)

	for _, id := range []uint{s1.ID, s2.ID} {
		var reloaded repository.DailyReportSection
		require.NoError(t, db.First(&reloaded, id).Error)
		assert.Nil(t, reloaded.PersistentTopicID)
		assert.Equal(t, "l1_direct", reloaded.LaneTier)
		require.NotNil(t, reloaded.ManualAssignmentAt)
		assert.Equal(t, repository.ManualAssignmentActionDetach, reloaded.ManualAssignmentAction)
	}

	var otherDay repository.DailyReportSection
	require.NoError(t, db.First(&otherDay, s3.ID).Error)
	require.NotNil(t, otherDay.PersistentTopicID)
	assert.Equal(t, topic.ID, *otherDay.PersistentTopicID)
	assert.Nil(t, otherDay.ManualAssignmentAt)
}

// TestDetachTopicByDate_RejectsInvalidDate covers the input variant: a
// missing or malformed period_date is 4xx and nothing is written.
func TestDetachTopicByDate_RejectsInvalidDate(t *testing.T) {
	r, db := setupAssignmentHandlerPGTest(t)
	boardID := seedAssignmentBoard(t, db, "assignment-board")
	reportID := seedAssignmentReport(t, db, boardID, "2026-11-10")
	topic := seedAssignmentTopic(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAssignmentSection(t, db, reportID, "s1", &topic.ID)

	for _, body := range []string{`{}`, `{"period_date":"2026/11/10"}`, `{"period_date":"not-a-date"}`, `{"period_date":""}`} {
		w := postDetachDate(t, r, topic.ID, body)
		assert.GreaterOrEqual(t, w.Code, 400, body)
		assert.Less(t, w.Code, 500, body)
	}

	var reloaded repository.DailyReportSection
	require.NoError(t, db.First(&reloaded, sec.ID).Error)
	require.NotNil(t, reloaded.PersistentTopicID)
	assert.Equal(t, topic.ID, *reloaded.PersistentTopicID)
	assert.Nil(t, reloaded.ManualAssignmentAt)
}
