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

// seedComposeSectionWithDistance seeds a section carrying the anchor distance
// snapshot (the 「接近度」 source: 1 − topic_match_distance).
func seedComposeSectionWithDistance(t *testing.T, db *gorm.DB, reportID uint, label string, topicID *uint, distance float64) uint {
	t.Helper()
	section := repository.DailyReportSection{
		ReportID:             reportID,
		ClusterLabel:         label,
		ArticleCount:         1,
		Embedding:            repository.FloatsToPgVector([]float64{0.3, 0.4}),
		PersistentTopicID:    topicID,
		TopicMatchDistance:   distance,
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
		ClueCount         int    `json:"clue_count"`
		// ★ 只读投影：恒为数组，无标记是 []（不是 null / 缺字段）。
		ConfirmedKeyEvents []struct {
			MarkerID    uint    `json:"marker_id"`
			ThreadID    *uint   `json:"thread_id"`
			Title       string  `json:"title"`
			ConfirmedAt *string `json:"confirmed_at"`
		} `json:"confirmed_key_events"`
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
	// EarlyMembers is a pointer: nil = the field is absent from the response
	// (the request did not ask for the full member set).
	EarlyMembers *[]struct {
		ID                 uint     `json:"id"`
		ReportID           uint     `json:"report_id"`
		PeriodDate         string   `json:"period_date"`
		ClusterLabel       string   `json:"cluster_label"`
		PersistentTopicID  *uint    `json:"persistent_topic_id"`
		TopicMatchDistance *float64 `json:"topic_match_distance"`
	} `json:"early_members"`
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

// TestComposeCandidatesAndLaneDynamicsShareWindowEnd（fix-lane-metric-provenance
// 1.3/1.4 + persistent-topic delta「两个窗口末日一致」）：版块有一份未完成日报
// （period_date 比最新 completed 晚一天）时，compose-candidates 窗口末日与
// lane-dynamics 窗口锚点必须同一天（= 最新 completed 日报日），不得相差一天。
func TestComposeCandidatesAndLaneDynamicsShareWindowEnd(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	boardID := seedComposeBoard(t, db)

	// 最新 completed 日报 10-28；未完成日报 10-29（晚一天）。
	completed := seedComposeReport(t, db, boardID, "2026-10-28", "completed")
	seedComposeReport(t, db, boardID, "2026-10-29", "generating")
	topicID := seedComposeTopic(t, db, boardID, "锚点话题", repository.TopicStatusActive)
	seedComposeSection(t, db, completed, "已完成分区", &topicID)

	// 泳道动态窗口末日 = 响应 anchor_date。
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/semantic-boards/"+itoa(boardID)+"/lane-dynamics?days=14", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var dynamics struct {
		Success bool `json:"success"`
		Data    struct {
			AnchorDate string `json:"anchor_date"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &dynamics))
	require.True(t, dynamics.Success)
	assert.Equal(t, "2026-10-28", dynamics.Data.AnchorDate, "lane-dynamics 锚点只认 completed 日报")

	// compose-candidates 窗口末日 = 同一份 completed 日报。
	payload := decodeComposePayload(t, callComposeCandidates(t, r, boardID, "days=14"))
	require.NotNil(t, payload.Window.To)
	assert.Equal(t, "2026-10-28", *payload.Window.To)
	require.NotNil(t, payload.Window.LatestCompletedDate)
	assert.Equal(t, "2026-10-28", *payload.Window.LatestCompletedDate)
	assert.Equal(t, dynamics.Data.AnchorDate, *payload.Window.LatestCompletedDate, "两个窗口末日必须同一天")
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

// D4 — range longer than the 365-day support window → 400 (never silently
// trimmed; unify-lane-workbench OQ-2：上限由 90 放宽到 365，三处同步)。
func TestComposeCandidates_D4RangeTooLong(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	// 366 天（含首末）→ 拒绝并明说上限。
	w := callComposeCandidates(t, r, fx.boardID, "from=2025-10-30&to=2026-10-30")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "365 days")

	// 365 天边界本身可用（task 2.3：364/365 日窗口通过、366 日拒绝）。
	w = callComposeCandidates(t, r, fx.boardID, "from=2025-10-31&to=2026-10-30")
	assert.Equal(t, http.StatusOK, w.Code)
}

// D5 — a start date outside the fetchable window (latest completed report
// − 364 days) → 400.
func TestComposeCandidates_D5StartOutsideWindow(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	// The latest completed report is 2026-10-30 → the earliest fetchable
	// start is 2025-10-31 (latest − 364). The range itself stays inside the
	// 365-day support window, so the "outside the window" rule is what
	// rejects it.
	w := callComposeCandidates(t, r, fx.boardID, "from=2025-10-30&to=2026-10-20")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "outside the fetchable window")

	// The boundary itself is accepted.
	w = callComposeCandidates(t, r, fx.boardID, "from=2025-10-31&to=2026-10-20")
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

// D10 — days above the 365-day cap is still trimmed to 365 (only the days
// shortcut is trimmed; an explicit range is rejected instead). OQ-2：上限
// 由 90 放宽到 365（用户 2026-11-16 裁决，新增泳道可看一年真实可选池）。
func TestComposeCandidates_D10DaysCap(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "days=365")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)
	assert.Equal(t, 365, data.Window.Days)
	require.NotNil(t, data.Window.From)
	assert.Equal(t, "2025-10-31", *data.Window.From)
	require.NotNil(t, data.Window.To)
	assert.Equal(t, "2026-10-30", *data.Window.To)

	// days over the cap is trimmed to 365 (legacy shortcut semantics).
	w = callComposeCandidates(t, r, fx.boardID, "days=999")
	require.Equal(t, http.StatusOK, w.Code)
	data = decodeComposePayload(t, w)
	assert.Equal(t, 365, data.Window.Days)
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

// ── 泳道修正模式：全量成员（all_members_topic_id） ──
//
// The lane correction workbench lists the topic's **whole** member set: the
// in-window members stay in `sections` (the canvas draws them), the members
// whose report period precedes the window's first day are appended as
// `early_members` (the left column groups them by month). Without the
// parameter the compose workbench response must stay untouched.

// D14 — all_members_topic_id → early_members holds the out-of-window members
// of that topic (ascending by period date), the in-window members stay in
// sections, and members of other topics never leak in.
func TestComposeCandidates_D14AllMembersEarlyMembers(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	// Two older reports (outside the days=7 window 2026-10-24 … 2026-10-30).
	older := seedComposeReport(t, db, fx.boardID, "2026-10-20", "completed")
	oldest := seedComposeReport(t, db, fx.boardID, "2026-10-05", "completed")
	earlyNewer := seedComposeSectionWithDistance(t, db, older, "early-newer", &fx.candidate, 0.21)
	earlyOlder := seedComposeSectionWithDistance(t, db, oldest, "early-older", &fx.candidate, 0.34)
	// A member of the other topic: must not appear in the early members.
	seedComposeSectionWithDistance(t, db, older, "early-other-topic", &fx.active, 0.12)

	w := callComposeCandidates(t, r, fx.boardID, "days=7&all_members_topic_id="+itoa(fx.candidate))
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	require.NotNil(t, data.EarlyMembers, "the full member set must be returned when the parameter is given")
	early := *data.EarlyMembers
	require.Len(t, early, 2, "only the out-of-window members of that topic")

	// Ascending by report date.
	assert.Equal(t, uint(earlyOlder), early[0].ID)
	assert.Equal(t, "early-older", early[0].ClusterLabel)
	assert.Equal(t, "2026-10-05", early[0].PeriodDate[:10])
	require.NotNil(t, early[0].TopicMatchDistance)
	assert.InDelta(t, 0.34, *early[0].TopicMatchDistance, 1e-9)
	require.NotNil(t, early[0].PersistentTopicID)
	assert.Equal(t, fx.candidate, *early[0].PersistentTopicID)

	assert.Equal(t, uint(earlyNewer), early[1].ID)
	assert.Equal(t, "early-newer", early[1].ClusterLabel)
	assert.Equal(t, "2026-10-20", early[1].PeriodDate[:10])

	// The in-window members are untouched: the window still holds the three
	// sections of the candidate topic plus the one of the active topic.
	require.Len(t, data.Sections, 4)
	labels := make([]string, 0, len(data.Sections))
	for _, s := range data.Sections {
		labels = append(labels, s.ClusterLabel)
	}
	assert.ElementsMatch(t, []string{"s1", "s2", "s3", "s4"}, labels)
}

// D15 — without the parameter the response carries no early_members field at
// all (the compose workbench is byte-identical to before).
func TestComposeCandidates_D15NoAllMembersParamUnchanged(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	older := seedComposeReport(t, db, fx.boardID, "2026-10-20", "completed")
	seedComposeSectionWithDistance(t, db, older, "early-older", &fx.candidate, 0.34)

	w := callComposeCandidates(t, r, fx.boardID, "days=7")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "early_members",
		"without all_members_topic_id the field must be absent, not an empty array")

	data := decodeComposePayload(t, w)
	assert.Nil(t, data.EarlyMembers)
	require.Len(t, data.Sections, 4)
}

// D16 — a malformed parameter → 400; a topic of another board → 404.
func TestComposeCandidates_D16AllMembersParamValidation(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "days=7&all_members_topic_id=abc")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid all_members_topic_id")

	w = callComposeCandidates(t, r, fx.boardID, "days=7&all_members_topic_id=0")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// A topic that does not exist at all.
	w = callComposeCandidates(t, r, fx.boardID, "days=7&all_members_topic_id=999999")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "topic not found on this board")
}

// D17 — a topic whose members are all inside the window → early_members is
// present and empty (the client tells "no earlier members" apart from "this
// build does not know the full member set").
func TestComposeCandidates_D17EarlyMembersEmptyArray(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "days=30&all_members_topic_id="+itoa(fx.candidate))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"early_members":[]`)

	data := decodeComposePayload(t, w)
	require.NotNil(t, data.EarlyMembers)
	assert.Empty(t, *data.EarlyMembers)
}

// ── 编排态 ★ 只读投影（add-lane-compose-topic-event-tags）──

// CC-K1 — confirmed markers are projected per section; thread-deleted snapshots
// (source_thread_id NULL) still attach via source_section_id; suggested never
// enters the payload.
func TestComposeCandidates_ConfirmedKeyEventsPerSection(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	seedComposeThread(t, db, fx.s1, fx.s1, "线索一", "single")
	var thread repository.DailyReportThread
	require.NoError(t, db.Where("section_id = ? AND title = ?", fx.s1, "线索一").First(&thread).Error)

	now := time.Now()
	require.NoError(t, db.Create(&repository.TopicKeyEventMarker{
		SemanticBoardID:  fx.boardID,
		SourceKey:        "k-thread",
		SourceSectionID:  &fx.s1,
		SourceThreadID:   &thread.ID,
		SourcePeriodDate: composeDay(t, "2026-10-27"),
		TitleSnapshot:    "确认线索一",
		State:            repository.KeyEventStateConfirmed,
		LastAction:       repository.KeyEventActionMark,
		ConfirmedAt:      &now,
	}).Error)
	// thread 已删（source_thread_id NULL）→ 仍靠 section 指针挂上。
	require.NoError(t, db.Create(&repository.TopicKeyEventMarker{
		SemanticBoardID:  fx.boardID,
		SourceKey:        "k-detached-thread",
		SourceSectionID:  &fx.s1,
		SourcePeriodDate: composeDay(t, "2026-10-27"),
		TitleSnapshot:    "thread 已删的确认快照",
		State:            repository.KeyEventStateConfirmed,
		LastAction:       repository.KeyEventActionMark,
	}).Error)
	// suggested 不进投影。
	require.NoError(t, db.Create(&repository.TopicKeyEventMarker{
		SemanticBoardID:  fx.boardID,
		SourceKey:        "k-suggested",
		SourceSectionID:  &fx.s2,
		SourcePeriodDate: composeDay(t, "2026-10-28"),
		TitleSnapshot:    "AI 建议",
		State:            repository.KeyEventStateSuggested,
	}).Error)

	w := callComposeCandidates(t, r, fx.boardID, "days=30")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)

	s1Index := -1
	for i := range data.Sections {
		if data.Sections[i].ID == fx.s1 {
			s1Index = i
		}
	}
	require.NotEqual(t, -1, s1Index)
	s1 := data.Sections[s1Index]
	require.Len(t, s1.ConfirmedKeyEvents, 2)
	assert.Equal(t, "确认线索一", s1.ConfirmedKeyEvents[0].Title)
	require.NotNil(t, s1.ConfirmedKeyEvents[0].ThreadID, "有线程的标记带 thread_id")
	require.NotNil(t, s1.ConfirmedKeyEvents[0].ConfirmedAt)
	assert.Equal(t, "thread 已删的确认快照", s1.ConfirmedKeyEvents[1].Title)
	assert.Nil(t, s1.ConfirmedKeyEvents[1].ThreadID, "thread 已删 → thread_id 省略")

	// 没有 marker 的 section 必须序列化成 []（不是 null / 缺字段）。
	foundEmpty := false
	for _, sec := range data.Sections {
		if sec.ID == fx.s2 {
			foundEmpty = true
			assert.Empty(t, sec.ConfirmedKeyEvents)
		}
	}
	assert.True(t, foundEmpty)
	assert.Contains(t, w.Body.String(), `"confirmed_key_events":[]`)
}

// CC-K2 — no markers at all → every section is [] (known "no marker", not null).
func TestComposeCandidates_ConfirmedKeyEventsEmptyArray(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	w := callComposeCandidates(t, r, fx.boardID, "days=30")
	require.Equal(t, http.StatusOK, w.Code)
	data := decodeComposePayload(t, w)
	require.NotEmpty(t, data.Sections)
	for _, sec := range data.Sections {
		assert.NotNil(t, sec.ConfirmedKeyEvents)
		assert.Empty(t, sec.ConfirmedKeyEvents)
	}
	assert.NotContains(t, w.Body.String(), `"confirmed_key_events":null`)
}

// ── unify-lane-workbench：section 线索数（clue_count）与轻量取数 ──

// seedComposeThread seeds one thread of a section with the given scope
// ("single" counts as a clue, "composite" 综合稿 does not).
func seedComposeThread(t *testing.T, db *gorm.DB, reportID, sectionID uint, title, scope string) {
	t.Helper()
	thread := repository.DailyReportThread{
		ReportID:  reportID,
		SectionID: sectionID,
		Title:     title,
		Scope:     scope,
	}
	require.NoError(t, db.Omit("Embedding").Create(&thread).Error)
}

// CC1 — every section carries clue_count (non-composite thread count);
// 0 must serialize for sections with no threads; composite threads are
// excluded from the count (unify-lane-workbench D2 / test-cases E14).
func TestComposeCandidates_ClueCountExcludesComposite(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	// s1: two clue threads + one composite → clue_count = 2.
	seedComposeThread(t, db, fx.s1, fx.s1, "线索一", "single")
	seedComposeThread(t, db, fx.s1, fx.s1, "线索二", "")
	seedComposeThread(t, db, fx.s1, fx.s1, "综合稿", "composite")
	// s2: one clue → 1; s3: none → 0 (must still serialize); s4: only composite → 0.
	seedComposeThread(t, db, fx.s2, fx.s2, "线索", "single")
	seedComposeThread(t, db, fx.s4, fx.s4, "综合稿", "composite")

	w := callComposeCandidates(t, r, fx.boardID, "days=30")
	require.Equal(t, http.StatusOK, w.Code)

	// JSON 层断言：clue_count 必序列化、0 不省略（不是 Go 结构体零值丢失）。
	assert.Contains(t, w.Body.String(), `"clue_count":0`)

	data := decodeComposePayload(t, w)
	require.Len(t, data.Sections, 4)
	byLabel := map[string]int{}
	for _, s := range data.Sections {
		byLabel[s.ClusterLabel] = s.ClueCount
	}
	assert.Equal(t, 2, byLabel["s1"], "composite threads are excluded from clue_count")
	assert.Equal(t, 1, byLabel["s2"])
	assert.Equal(t, 0, byLabel["s3"], "a section with no threads has a known zero count")
	assert.Equal(t, 0, byLabel["s4"], "a section with only composite threads counts 0 clues")
}

// CC2 — omit_embeddings=1 drops the per-section vectors (轻量取数, OQ-2)：
// the curve only needs period_date + persistent_topic_id + clue_count; the
// plain response keeps the vectors byte-for-byte.
func TestComposeCandidates_OmitEmbeddingsLightweight(t *testing.T) {
	r, db := setupComposeCandidatesHandlerTest(t)
	fx := seedComposeFixture(t, db)

	light := callComposeCandidates(t, r, fx.boardID, "days=30&omit_embeddings=1")
	require.Equal(t, http.StatusOK, light.Code)
	assert.NotContains(t, light.Body.String(), `"embedding"`, "lightweight path must not ship vectors")
	data := decodeComposePayload(t, light)
	require.Len(t, data.Sections, 4)
	for _, s := range data.Sections {
		assert.Equal(t, 0, s.ClueCount, "clue_count still serializes in the lightweight response")
	}

	plain := callComposeCandidates(t, r, fx.boardID, "days=30")
	require.Equal(t, http.StatusOK, plain.Code)
	assert.Contains(t, plain.Body.String(), `"embedding"`, "the canvas path keeps shipping vectors")
}
