package handler

import (
	"encoding/json"
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

// ── compose-candidates window validation + response shape ──
//
// Whitebox branch table D1–D11 of
// openspec/changes/rework-workshop-into-radar-and-desk/test-cases-lane-compose-implementation.md §3.2.
//
// The endpoint talks raw SQL (date window + relation join), so these cases run
// against the testcontainer PostgreSQL — SQLite would not exercise the real
// query semantics.

func setupComposeCandidatesHandlerTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	r := gin.New()
	RegisterDailyReportRoutes(r.Group("/api"))
	return r, db
}

func composeDay(t *testing.T, s string) time.Time {
	t.Helper()
	day, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return day
}

func seedComposeBoard(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	board := models.SemanticLabel{Label: "compose-board", Slug: "compose-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	return board.ID
}

func seedComposeReport(t *testing.T, db *gorm.DB, boardID uint, day string, status string) uint {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: boardID,
		PeriodDate:      composeDay(t, day),
		Title:           "compose report " + day,
		Status:          status,
	}
	require.NoError(t, db.Create(&report).Error)
	return report.ID
}

func seedComposeTopic(t *testing.T, db *gorm.DB, boardID uint, label string, status string) uint {
	t.Helper()
	day := composeDay(t, "2026-10-01")
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: boardID,
		Label:           label,
		Status:          status,
		Source:          repository.TopicSourceAuto,
		Embedding:       repository.FloatsToPgVector([]float64{0.1, 0.2}),
		FirstSeenDate:   day,
		LastSeenDate:    day,
		HitCount:        2,
	}
	require.NoError(t, db.Create(&topic).Error)
	return topic.ID
}

func seedComposeSection(t *testing.T, db *gorm.DB, reportID uint, label string, topicID *uint) uint {
	t.Helper()
	section := repository.DailyReportSection{
		ReportID:             reportID,
		ClusterLabel:         label,
		ArticleCount:         1,
		Embedding:            repository.FloatsToPgVector([]float64{0.3, 0.4}),
		PersistentTopicID:    topicID,
		TopicMatchConfidence: repository.TopicConfAnchorHit,
	}
	require.NoError(t, db.Create(&section).Error)
	return section.ID
}

func seedComposeRelation(t *testing.T, db *gorm.DB, fromID, toID uint, relationType string, distance float64) {
	t.Helper()
	rel := repository.SectionRelation{
		FromSectionID: fromID,
		ToSectionID:   toID,
		Distance:      distance,
		RelationType:  relationType,
	}
	require.NoError(t, db.Create(&rel).Error)
}

// composeFixture is the standard board used by the window cases:
//
//	candidate topic T1: S1(10-27) — S2(10-28) — S3(10-30)   → one chain, span 3 days
//	active    topic T2: S4(10-29)                           → no chain (not candidate)
//	cross-topic similarity edge S3 — S4                     → must not merge the topics
type composeFixture struct {
	boardID        uint
	candidate      uint
	active         uint
	s1, s2, s3, s4 uint
}

func seedComposeFixture(t *testing.T, db *gorm.DB) composeFixture {
	t.Helper()
	boardID := seedComposeBoard(t, db)
	candidateID := seedComposeTopic(t, db, boardID, "以色列议会选举", repository.TopicStatusCandidate)
	activeID := seedComposeTopic(t, db, boardID, "中东局势", repository.TopicStatusActive)

	r27 := seedComposeReport(t, db, boardID, "2026-10-27", "completed")
	r28 := seedComposeReport(t, db, boardID, "2026-10-28", "completed")
	r29 := seedComposeReport(t, db, boardID, "2026-10-29", "completed")
	r30 := seedComposeReport(t, db, boardID, "2026-10-30", "completed")

	s1 := seedComposeSection(t, db, r27, "s1", &candidateID)
	s2 := seedComposeSection(t, db, r28, "s2", &candidateID)
	s3 := seedComposeSection(t, db, r30, "s3", &candidateID)
	s4 := seedComposeSection(t, db, r29, "s4", &activeID)

	seedComposeRelation(t, db, s1, s2, "identity", 0.05)
	seedComposeRelation(t, db, s2, s3, "identity", 0.07)
	seedComposeRelation(t, db, s3, s4, "similarity", 0.44)

	return composeFixture{
		boardID: boardID, candidate: candidateID, active: activeID,
		s1: s1, s2: s2, s3: s3, s4: s4,
	}
}

type composeCandidatesPayload struct {
	Sections []struct {
		ID                uint   `json:"id"`
		ReportID          uint   `json:"report_id"`
		PeriodDate        string `json:"period_date"`
		ClusterLabel      string `json:"cluster_label"`
		PersistentTopicID *uint  `json:"persistent_topic_id"`
	} `json:"sections"`
	Relations []struct {
		FromID       uint    `json:"from_id"`
		ToID         uint    `json:"to_id"`
		Distance     float64 `json:"distance"`
		RelationType string  `json:"relation_type"`
	} `json:"relations"`
	CandidateChains []struct {
		Key         string `json:"key"`
		TopicID     uint   `json:"topic_id"`
		TopicLabel  string `json:"topic_label"`
		TopicStatus string `json:"topic_status"`
		SectionIDs  []uint `json:"section_ids"`
		FirstDate   string `json:"first_date"`
		LastDate    string `json:"last_date"`
		SpanDays    int    `json:"span_days"`
	} `json:"candidate_chains"`
	MatchThreshold float64 `json:"match_threshold"`
	Window         struct {
		From                *string `json:"from"`
		To                  *string `json:"to"`
		Days                int     `json:"days"`
		LatestCompletedDate *string `json:"latest_completed_date"`
	} `json:"window"`
}

func callComposeCandidates(t *testing.T, r *gin.Engine, boardID uint, query string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/semantic-boards/" + itoa(boardID) + "/persistent-topics/compose-candidates"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

func decodeComposePayload(t *testing.T, w *httptest.ResponseRecorder) composeCandidatesPayload {
	t.Helper()
	var resp struct {
		Success bool                     `json:"success"`
		Data    composeCandidatesPayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	return resp.Data
}

// D1 — only `from` given → 400.
func TestComposeCandidates_D1OnlyFrom(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "from=2026-10-01")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "together")
}

// D2 — only `to` given → 400.
func TestComposeCandidates_D2OnlyTo(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "to=2026-10-30")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "together")
}

// D3 — inverted range → 400.
func TestComposeCandidates_D3InvertedRange(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "from=2026-10-30&to=2026-10-01")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "from must not be later than to")
}

// D4 — range longer than the 90-day support window → 400 (never silently
// trimmed to 90 days).
func TestComposeCandidates_D4RangeTooLong(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "from=2026-01-01&to=2026-10-30")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "90 days")
}

// D5 — a start date outside the fetchable window (latest completed report
// − 89 days) → 400.
func TestComposeCandidates_D5StartOutsideWindow(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	// The latest completed report is 2026-10-30 → the earliest fetchable
	// start is 2026-08-02. The range itself stays inside the 90-day support
	// window (81 days), so the "outside the window" rule is what rejects it.
	w := callComposeCandidates(t, r, fx.boardID, "from=2026-08-01&to=2026-10-20")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "outside the fetchable window")

	// The boundary itself is accepted.
	w = callComposeCandidates(t, r, fx.boardID, "from=2026-08-02&to=2026-10-20")
	assert.Equal(t, http.StatusOK, w.Code)
}

// D6 — `to` beyond the latest completed report is allowed (that window simply
// holds no later data).
func TestComposeCandidates_D6ToAfterLatestReport(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "from=2026-10-20&to=2026-12-31")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)
	require.NotNil(t, data.Window.To)
	assert.Equal(t, "2026-12-31", *data.Window.To)
	require.NotNil(t, data.Window.LatestCompletedDate)
	assert.Equal(t, "2026-10-30", *data.Window.LatestCompletedDate)
}

// D7 — malformed date → 400.
func TestComposeCandidates_D7InvalidDateFormat(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "from=2026-13-01&to=2026-10-30")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid from date")

	w = callComposeCandidates(t, r, fx.boardID, "from=2026-10-01&to=2026-10-32")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid to date")
}

// D8 — explicit range → 200 with the window echoed back.
func TestComposeCandidates_D8ExplicitRange(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "from=2026-10-01&to=2026-10-30")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	require.NotNil(t, data.Window.From)
	require.NotNil(t, data.Window.To)
	assert.Equal(t, "2026-10-01", *data.Window.From)
	assert.Equal(t, "2026-10-30", *data.Window.To)
	assert.Equal(t, 30, data.Window.Days)
	require.NotNil(t, data.Window.LatestCompletedDate)
	assert.Equal(t, "2026-10-30", *data.Window.LatestCompletedDate)

	// Sections: all four are inside the window.
	require.Len(t, data.Sections, 4)
	assert.Equal(t, "s1", data.Sections[0].ClusterLabel)

	// Relations: the three in-window edges (identity + similarity), verbatim.
	require.Len(t, data.Relations, 3)
	types := map[string]int{}
	for _, rel := range data.Relations {
		types[rel.RelationType]++
	}
	assert.Equal(t, 2, types["identity"])
	assert.Equal(t, 1, types["similarity"])

	// Candidate chains: one chain (the candidate topic), the active topic and
	// the cross-topic edge do not create one.
	require.Len(t, data.CandidateChains, 1)
	chain := data.CandidateChains[0]
	assert.Equal(t, fx.candidate, chain.TopicID)
	assert.Equal(t, repository.TopicStatusCandidate, chain.TopicStatus)
	assert.Equal(t, "以色列议会选举", chain.TopicLabel)
	assert.Equal(t, []uint{fx.s1, fx.s2, fx.s3}, chain.SectionIDs)
	assert.Equal(t, "2026-10-27", chain.FirstDate)
	assert.Equal(t, "2026-10-30", chain.LastDate)
	assert.Equal(t, 3, chain.SpanDays)
	assert.Equal(t, "以色列议会选举", chain.TopicLabel)
	assert.InDelta(t, 0.30, data.MatchThreshold, 1e-9)
}

// D9 — the days shortcut keeps its legacy semantics: the window is anchored
// to the latest completed report and covers `days` report days (both ends
// included).
func TestComposeCandidates_D9DaysShortcut(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "days=30")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	require.NotNil(t, data.Window.From)
	require.NotNil(t, data.Window.To)
	assert.Equal(t, "2026-10-01", *data.Window.From, "30 report days ending at the latest completed report")
	assert.Equal(t, "2026-10-30", *data.Window.To)
	assert.Equal(t, 30, data.Window.Days)
	require.Len(t, data.Sections, 4)
	require.Len(t, data.CandidateChains, 1)
}

// D10 — days above the 90-day cap is still trimmed to 90 (only the days
// shortcut is trimmed; an explicit range is rejected instead).
func TestComposeCandidates_D10DaysCap(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "days=365")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)
	assert.Equal(t, 90, data.Window.Days)
	require.NotNil(t, data.Window.From)
	assert.Equal(t, "2026-08-02", *data.Window.From)
	require.NotNil(t, data.Window.To)
	assert.Equal(t, "2026-10-30", *data.Window.To)
}

// D11 — a board without any completed report answers 200 with empty
// collections (never a 404).
func TestComposeCandidates_D11NoCompletedReport(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	boardID := seedComposeBoard(t, db)
	// A report that is still generating is not a completed report.
	seedComposeReport(t, db, boardID, "2026-10-30", "generating")

	w := callComposeCandidates(t, r, boardID, "days=30")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	assert.Empty(t, data.Sections)
	assert.Empty(t, data.Relations)
	assert.Empty(t, data.CandidateChains)
	assert.Nil(t, data.Window.LatestCompletedDate)
	assert.Nil(t, data.Window.From)
	assert.Nil(t, data.Window.To)
}

// D12 — an explicit from/to range wins over a simultaneously given days
// shortcut: the days parameter is ignored completely (contract §1.1 —
// "from/to 给出时 days 被忽略").
func TestComposeCandidates_D12ExplicitRangeOverridesDays(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	// days=7 would cover 2026-10-24 … 2026-10-30 (s1 on 10-27 included);
	// the explicit range 10-28 … 10-30 excludes it.
	w := callComposeCandidates(t, r, fx.boardID, "days=7&from=2026-10-28&to=2026-10-30")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	// The window echoes the explicit range, not the days shortcut.
	require.NotNil(t, data.Window.From)
	require.NotNil(t, data.Window.To)
	assert.Equal(t, "2026-10-28", *data.Window.From)
	assert.Equal(t, "2026-10-30", *data.Window.To)
	assert.Equal(t, 3, data.Window.Days, "days must be ignored — the effective span is to-from+1")

	// Only the sections inside the explicit range are returned: s1 (10-27)
	// sits inside the days=7 window but outside the explicit range.
	labels := make([]string, 0, len(data.Sections))
	for _, s := range data.Sections {
		labels = append(labels, s.ClusterLabel)
	}
	assert.Equal(t, []string{"s2", "s4", "s3"}, labels)
	assert.NotContains(t, labels, "s1")

	// The relation with the out-of-window endpoint (s1 — s2) is dropped as
	// well: relations are the edges whose both endpoints are visible.
	require.Len(t, data.Relations, 2)

	// The candidate chain is recomputed on the effective window: the
	// candidate topic's visible run (10-28 … 10-30) spans 2 days only, so it
	// no longer qualifies.
	assert.Empty(t, data.CandidateChains)
}

// D13 — the days<=0 ("approximately all history") mode: no day count, both
// window bounds open (null), and the full history is served (not bounded by
// the 90-day window).
func TestComposeCandidates_D13AllHistoryWindow(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	boardID := seedComposeBoard(t, db)
	topicID := seedComposeTopic(t, db, boardID, "候选话题", repository.TopicStatusCandidate)

	recentReport := seedComposeReport(t, db, boardID, "2026-10-30", "completed")
	// Far outside the 90-day window — only the all-history mode reaches it.
	oldReport := seedComposeReport(t, db, boardID, "2025-06-01", "completed")
	recentSection := seedComposeSection(t, db, recentReport, "recent", &topicID)
	oldSection := seedComposeSection(t, db, oldReport, "old", &topicID)

	w := callComposeCandidates(t, r, boardID, "days=0")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	// No day count, both bounds open — null, not "" and not absent.
	assert.Equal(t, 0, data.Window.Days)
	assert.Nil(t, data.Window.From)
	assert.Nil(t, data.Window.To)
	// The board does have a latest completed report — the field reports it.
	require.NotNil(t, data.Window.LatestCompletedDate)
	assert.Equal(t, "2026-10-30", *data.Window.LatestCompletedDate)

	// The full history is served, including the section far outside the
	// 90-day window.
	labels := make([]string, 0, len(data.Sections))
	for _, s := range data.Sections {
		labels = append(labels, s.ClusterLabel)
	}
	assert.Len(t, data.Sections, 2)
	assert.Contains(t, labels, "recent")
	assert.Contains(t, labels, "old")

	// Contrast: the 90-day shortcut only sees the recent section — the
	// all-history mode really is a different window.
	w = callComposeCandidates(t, r, boardID, "days=90")
	require.Equal(t, http.StatusOK, w.Code)
	data = decodeComposePayload(t, w)
	require.Len(t, data.Sections, 1)
	assert.Equal(t, "recent", data.Sections[0].ClusterLabel)
	assert.Equal(t, uint(recentSection), data.Sections[0].ID)
	assert.NotEqual(t, uint(oldSection), data.Sections[0].ID)
}
