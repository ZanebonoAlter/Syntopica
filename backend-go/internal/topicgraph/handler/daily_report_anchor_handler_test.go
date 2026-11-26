package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// ── PATCH /api/daily-reports/topics/:id/anchor（lane-manual-correction D3） ──
// HTTP 契约测试：请求体只接受 section_ids；自然语言输入 4xx；响应带
// topic_id / 来源 / 改写次数 / effective_from。

// assertVecEqual compares two pgvector literals element-wise: PostgreSQL
// normalizes the literal it stores ("[0,0,1]"), so the producer's format
// ("[0.000000,0.000000,1.000000]") never matches as a raw string.
func assertVecEqual(t *testing.T, expected, actual string) {
	t.Helper()
	parse := func(lit string) []float64 {
		lit = strings.TrimSpace(lit)
		lit = strings.TrimPrefix(lit, "[")
		lit = strings.TrimSuffix(lit, "]")
		if lit == "" {
			return nil
		}
		parts := strings.Split(lit, ",")
		out := make([]float64, 0, len(parts))
		for _, p := range parts {
			v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			require.NoError(t, err)
			out = append(out, v)
		}
		return out
	}
	want, got := parse(expected), parse(actual)
	require.Len(t, got, len(want))
	for i := range want {
		assert.InDelta(t, want[i], got[i], 1e-6, "vector element %d differs", i)
	}
}

func setupAnchorHandlerPGTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	r := gin.New()
	api := r.Group("/api")
	RegisterDailyReportRoutes(api)
	return r, db
}

func seedAnchorBoard(t *testing.T, db *gorm.DB, slug string) uint {
	t.Helper()
	board := models.SemanticLabel{
		Label:     slug,
		Slug:      slug,
		LabelType: "board",
		Status:    "active",
	}
	require.NoError(t, db.Create(&board).Error)
	return board.ID
}

func seedAnchorReport(t *testing.T, db *gorm.DB, boardID uint, day time.Time) uint {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      day,
		Title:           "Test Report",
		Status:          "completed",
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func seedAnchorTopicRow(t *testing.T, db *gorm.DB, boardID uint, label string, status string) repository.BoardPersistentTopic {
	t.Helper()
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Embedding:       repository.FloatsToPgVector([]float64{0, 0, 1}),
		Status:          status,
		Source:          repository.TopicSourceAuto,
		FirstSeenDate:   repository.NormalizeReportDate(time.Now()),
		LastSeenDate:    repository.NormalizeReportDate(time.Now()),
		HitCount:        1,
		ConsecutiveHits: 1,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic
}

func seedAnchorSectionRow(t *testing.T, db *gorm.DB, reportID uint, label string, topicID *uint) repository.DailyReportSection {
	t.Helper()
	sec := repository.DailyReportSection{
		ReportID:     reportID,
		ClusterLabel: label,
		ArticleCount: 1,
		Embedding:    repository.FloatsToPgVector([]float64{1, 0, 0}),
	}
	if topicID != nil {
		sec.PersistentTopicID = topicID
	}
	require.NoError(t, db.Create(&sec).Error)
	return sec
}

func patchAnchor(t *testing.T, r *gin.Engine, topicID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/daily-reports/topics/"+anchorIDStr(topicID)+"/anchor", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func anchorIDStr(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

// TestUpdateTopicAnchor_HandlerSuccess covers the success contract: the
// response carries topic_id / source / selected_sections / override_count /
// effective_from, and the fact layer really moved.
func TestUpdateTopicAnchor_HandlerSuccess(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())

	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	s1 := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)
	s2 := seedAnchorSectionRow(t, db, reportID, "s2", &topic.ID)

	w := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(s1.ID)+`,`+anchorIDStr(s2.ID)+`]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			TopicID       uint   `json:"topic_id"`
			Source        string `json:"source"`
			Selected      []uint `json:"selected_sections"`
			OverrideCount int    `json:"override_count"`
			EffectiveFrom string `json:"effective_from"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, topic.ID, resp.Data.TopicID)
	assert.Equal(t, repository.TopicAnchorSourceSelectedSections, resp.Data.Source)
	assert.Equal(t, []uint{s1.ID, s2.ID}, resp.Data.Selected)
	assert.Equal(t, 1, resp.Data.OverrideCount)
	assert.Equal(t, repository.AnchorEffectiveFromNextReport, resp.Data.EffectiveFrom)

	var reloaded repository.BoardPersistentTopic
	require.NoError(t, db.First(&reloaded, topic.ID).Error)
	assert.True(t, reloaded.CentroidPendingRecompute)
}

// TestUpdateTopicAnchor_RejectsNonSectionIDBodies covers the input variants:
// free text, a missing section_ids, an empty list and a non-array value are
// all 4xx — the 首义 has exactly one input mode.
func TestUpdateTopicAnchor_RejectsNonSectionIDBodies(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())

	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	bodies := map[string]string{
		"natural language": `{"description":"把这条泳道定义为 Claude Code 工具链"}`,
		"free text":        `{"section_ids":"把这条泳道定义为 Claude Code 工具链"}`,
		"missing":          `{}`,
		"empty list":       `{"section_ids":[]}`,
		"null":             `{"section_ids":null}`,
		"bare string":      `"把这条泳道定义为 Claude Code 工具链"`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			w := patchAnchor(t, r, topic.ID, body)
			assert.GreaterOrEqual(t, w.Code, 400)
			assert.Less(t, w.Code, 500)
		})
	}

	// Nothing was written by any of the rejected bodies.
	var reloaded repository.BoardPersistentTopic
	require.NoError(t, db.First(&reloaded, topic.ID).Error)
	assertVecEqual(t, repository.FloatsToPgVector([]float64{0, 0, 1}), reloaded.Embedding)
	assert.False(t, reloaded.CentroidPendingRecompute)

	var count int64
	require.NoError(t, db.Model(&repository.TopicAnchorOverride{}).Count(&count).Error)
	assert.Zero(t, count)

	// The node itself is untouched.
	var secRow repository.DailyReportSection
	require.NoError(t, db.First(&secRow, sec.ID).Error)
	require.NotNil(t, secRow.PersistentTopicID)
	assert.Equal(t, topic.ID, *secRow.PersistentTopicID)
}

// TestUpdateTopicAnchor_RejectsNodeOfAnotherTopic covers the validation
// contract on the HTTP surface: a node that does not belong to the lane is
// rejected with 400 and nothing is written.
func TestUpdateTopicAnchor_RejectsNodeOfAnotherTopic(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())

	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	otherTopic := seedAnchorTopicRow(t, db, boardID, "lane B", repository.TopicStatusActive)
	good := seedAnchorSectionRow(t, db, reportID, "good", &topic.ID)
	foreign := seedAnchorSectionRow(t, db, reportID, "foreign", &otherTopic.ID)

	w := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(good.ID)+`,`+anchorIDStr(foreign.ID)+`]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var reloaded repository.BoardPersistentTopic
	require.NoError(t, db.First(&reloaded, topic.ID).Error)
	assertVecEqual(t, repository.FloatsToPgVector([]float64{0, 0, 1}), reloaded.Embedding)
	assert.False(t, reloaded.CentroidPendingRecompute)
}

// TestUpdateTopicAnchor_UnknownTopicIs404 covers the precondition variant:
// a topic that does not exist answers 404 and writes nothing.
func TestUpdateTopicAnchor_UnknownTopicIs404(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "lane A", repository.TopicStatusActive)
	sec := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	w := patchAnchor(t, r, 999999, `{"section_ids":[`+anchorIDStr(sec.ID)+`]}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestUpdateTopicAnchor_ArchivedTopicIs409 covers the archived-lane guard.
func TestUpdateTopicAnchor_ArchivedTopicIs409(t *testing.T) {
	r, db := setupAnchorHandlerPGTest(t)
	boardID := seedAnchorBoard(t, db, "anchor-board")
	reportID := seedAnchorReport(t, db, boardID, time.Now())
	topic := seedAnchorTopicRow(t, db, boardID, "archived lane", repository.TopicStatusArchived)
	sec := seedAnchorSectionRow(t, db, reportID, "s1", &topic.ID)

	w := patchAnchor(t, r, topic.ID, `{"section_ids":[`+anchorIDStr(sec.ID)+`]}`)
	assert.Equal(t, http.StatusConflict, w.Code)
}
