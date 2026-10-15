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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/topicgraph/repository"
)

// ── key-event 注记端点 HTTP 语义（add-topic-key-event-markers 1.3）──────────
//
// PG testcontainer：动作走真实 SQL（行锁/唯一约束/CHECK）。覆盖 400/404/409、
// 跨域、伪造正文忽略、来源失效取消、topic 读端 confirmed-only 契约、人工动作
// 零 AI/研究调用（无推荐 run 行产生）。推荐状态派生（idle/done/failed 与
// running 存量降级）也在此验证。

func setupKeyEventHandlerTest(t *testing.T) (*gin.Engine, *repository.TopicGraphRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	r := gin.New()
	api := r.Group("/api")
	RegisterKeyEventRoutes(api)
	return r, repository.Repo
}

type keyEventWorld struct {
	boardID  uint
	topicID  uint
	reportID uint
	sectionA uint
	threadA  uint
	threadB  uint
}

func seedKeyEventWorld(t *testing.T, repo *repository.TopicGraphRepository) keyEventWorld {
	t.Helper()
	board := models.SemanticLabel{Label: "ke-board", Slug: "ke-board", LabelType: "board", Status: "active"}
	require.NoError(t, repo.DB().Create(&board).Error)
	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID, Label: "KE 泳道", Status: repository.TopicStatusActive,
		FirstSeenDate: repository.NormalizeReportDate(time.Now()),
		LastSeenDate:  repository.NormalizeReportDate(time.Now()),
	}
	require.NoError(t, repo.DB().Omit("embedding", "centroid").Create(&topic).Error)
	report := repository.BoardDailyReport{
		SemanticBoardID: board.ID, PeriodDate: repository.NormalizeReportDate(time.Now()),
		Status: "completed", Title: "t",
	}
	require.NoError(t, repo.DB().Create(&report).Error)
	section := repository.DailyReportSection{
		ReportID: report.ID, ClusterLabel: "章节", ArticleCount: 2,
		Embedding:         repository.FloatsToPgVector([]float64{0}),
		PersistentTopicID: &topic.ID,
	}
	require.NoError(t, repo.DB().Create(&section).Error)
	mkThread := func(title string, arts string) uint {
		th := repository.DailyReportThread{
			ReportID: report.ID, SectionID: section.ID, Title: title, Summary: "摘要 " + title,
			TagIDs: repository.JSON("[7]"), RelatedArticleIDs: repository.JSON(arts),
			Embedding: repository.FloatsToPgVector([]float64{0}),
		}
		require.NoError(t, repo.DB().Create(&th).Error)
		return th.ID
	}
	return keyEventWorld{
		boardID: board.ID, topicID: topic.ID, reportID: report.ID,
		sectionA: section.ID, threadA: mkThread("线索A", "[10,11]"), threadB: mkThread("线索B", "[12]"),
	}
}

func keyEventRequest(t *testing.T, engine *gin.Engine, method, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	var payload map[string]any
	if w.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	}
	return w.Code, payload
}

// ── PUT：确认 / 幂等 / 409 / 伪造正文忽略 ──────────────────────────────────

func TestKeyEventPut_MarkIdempotentAndRevisionConflict(t *testing.T) {
	engine, repo := setupKeyEventHandlerTest(t)
	w := seedKeyEventWorld(t, repo)

	code, resp := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "mark"})
	require.Equal(t, http.StatusOK, code, "%v", resp)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	assert.Equal(t, "confirmed", data["state"])
	assert.Equal(t, "线索A", data["title_snapshot"], "快照来自服务端 thread")

	// Repeat same target → idempotent 200, no new row.
	code, resp2 := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "mark"})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, data["id"], resp2["data"].(map[string]any)["id"])

	// State CHANGE with a stale revision → 409 carrying the latest state.
	stale := int(data["revision"].(float64)) + 5
	code, resp3 := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "dismiss", "expected_revision": stale})
	require.Equal(t, http.StatusConflict, code, "%v", resp3)
	assert.Equal(t, "revision_conflict", resp3["reason"])
	latest := resp3["data"].(map[string]any)
	assert.Equal(t, "confirmed", latest["state"], "409 附服务端最新状态")

	// Correct revision → dismiss succeeds.
	rev := int(latest["revision"].(float64))
	code, resp4 := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "dismiss", "expected_revision": rev})
	require.Equal(t, http.StatusOK, code, "%v", resp4)
	assert.Equal(t, "dismissed", resp4["data"].(map[string]any)["state"])
}

func TestKeyEventPut_ForgedBodyIgnored(t *testing.T) {
	engine, repo := setupKeyEventHandlerTest(t)
	w := seedKeyEventWorld(t, repo)

	// 伪造正文：客户端试图携带快照文案与 AI 理由 → 字段被忽略，快照仍取服务端来源。
	code, resp := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{
			"action": "mark",
			"title":  "伪造标题", "summary": "伪造摘要",
			"title_snapshot": "伪造快照", "recommendation_reason": "伪造AI理由",
			"state": "confirmed", "revision": 99,
		})
	require.Equal(t, http.StatusOK, code, "%v", resp)
	data := resp["data"].(map[string]any)
	assert.Equal(t, "线索A", data["title_snapshot"])
	assert.Equal(t, "摘要 线索A", data["summary_snapshot"])
	assert.Nil(t, data["recommendation_reason"], "AI 理由不来自客户端")

	var marker repository.TopicKeyEventMarker
	require.NoError(t, repo.DB().Where("source_thread_id = ?", w.threadA).First(&marker).Error)
	assert.Equal(t, "线索A", marker.TitleSnapshot)
	assert.Empty(t, marker.RecommendationReason)
}

func TestKeyEventPut_ValidationAndScope(t *testing.T) {
	engine, repo := setupKeyEventHandlerTest(t)
	w := seedKeyEventWorld(t, repo)
	otherTopic := repository.BoardPersistentTopic{
		SemanticBoardID: w.boardID, Label: "他泳道", Status: repository.TopicStatusActive,
		FirstSeenDate: repository.NormalizeReportDate(time.Now()),
		LastSeenDate:  repository.NormalizeReportDate(time.Now()),
	}
	require.NoError(t, repo.DB().Omit("embedding", "centroid").Create(&otherTopic).Error)

	put := func(topicID, threadID uint, body map[string]any) (int, map[string]any) {
		return keyEventRequest(t, engine, http.MethodPut,
			fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", topicID, threadID), body)
	}

	// Empty / whitespace / unknown actions → 400.
	for _, action := range []string{"", " ", "\t", "MARK", "confirm", "delete", "unmark"} {
		code, resp := put(w.topicID, w.threadA, map[string]any{"action": action})
		assert.Equal(t, http.StatusBadRequest, code, "action=%q: %v", action, resp)
	}
	// Non-JSON body → 400.
	req := httptest.NewRequest(http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Cross-topic thread → 404 (越界引用).
	code, _ := put(otherTopic.ID, w.threadA, map[string]any{"action": "mark"})
	assert.Equal(t, http.StatusNotFound, code)
	// Unknown topic/thread → 404.
	code, _ = put(999999, w.threadA, map[string]any{"action": "mark"})
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = put(w.topicID, 999999, map[string]any{"action": "mark"})
	assert.Equal(t, http.StatusNotFound, code)
	// Invalid ids → 400.
	code, _ = keyEventRequest(t, engine, http.MethodPut,
		"/api/daily-reports/topics/abc/key-events/threads/1", map[string]any{"action": "mark"})
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/x", w.topicID), map[string]any{"action": "mark"})
	assert.Equal(t, http.StatusBadRequest, code)

	// Orphan section (no topic) → 400 (无归属新标记).
	orphan := repository.DailyReportSection{
		ReportID: w.reportID, ClusterLabel: "无归属", ArticleCount: 0,
		Embedding: repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, repo.DB().Create(&orphan).Error)
	oth := repository.DailyReportThread{
		ReportID: w.reportID, SectionID: orphan.ID, Title: "无归属线索",
		Embedding: repository.FloatsToPgVector([]float64{0}),
	}
	require.NoError(t, repo.DB().Create(&oth).Error)
	code, _ = put(w.topicID, oth.ID, map[string]any{"action": "mark"})
	assert.Equal(t, http.StatusBadRequest, code)

	// Manual mark without any AI involvement: no recommendation run appears.
	var runs int64
	require.NoError(t, repo.DB().Model(&repository.KeyEventRecommendationRun{}).Count(&runs).Error)
	assert.Equal(t, int64(0), runs, "人工动作零 AI/研究调用")
}

// ── PATCH：来源失效取消 / 重新 mark 拒绝 ───────────────────────────────────

func TestKeyEventPatch_DetachedSourceUnmarkOnly(t *testing.T) {
	engine, repo := setupKeyEventHandlerTest(t)
	w := seedKeyEventWorld(t, repo)

	_, resp := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "mark"})
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	markerID := uint(data["id"].(float64))
	rev := int(data["revision"].(float64))

	// Simulate source replacement at the DB level (rebuild deleted the thread).
	require.NoError(t, repo.DB().Where("id = ?", w.threadA).Delete(&repository.DailyReportThread{}).Error)

	patch := func(action string, revision int) (int, map[string]any) {
		return keyEventRequest(t, engine, http.MethodPatch,
			fmt.Sprintf("/api/daily-reports/topics/%d/key-events/%d", w.topicID, markerID),
			map[string]any{"action": action, "revision": revision})
	}

	// 来源失效取消：unmark still works on the detached confirmed snapshot.
	code, resp2 := patch("unmark", rev)
	require.Equal(t, http.StatusOK, code, "%v", resp2)
	assert.Equal(t, "dismissed", resp2["data"].(map[string]any)["state"])

	// 重新 mark 凭旧来源 → 400.
	code, resp3 := patch("mark", rev+1)
	require.Equal(t, http.StatusBadRequest, code, "%v", resp3)

	// PATCH without revision → 400.
	code, _ = keyEventRequest(t, engine, http.MethodPatch,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/%d", w.topicID, markerID),
		map[string]any{"action": "unmark"})
	assert.Equal(t, http.StatusBadRequest, code)
	// PATCH unknown marker / cross-topic marker → 404.
	code, _ = keyEventRequest(t, engine, http.MethodPatch,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/999999", w.topicID),
		map[string]any{"action": "unmark", "revision": 1})
	assert.Equal(t, http.StatusNotFound, code)
}

// ── GET：topic 读端 confirmed-only / days 契约 / report 状态派生 ────────────

func TestKeyEventGet_TopicConfirmedOnly(t *testing.T) {
	engine, repo := setupKeyEventHandlerTest(t)
	w := seedKeyEventWorld(t, repo)

	// Mark A (confirmed) and dismiss B.
	_, respA := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "mark"})
	require.Equal(t, true, respA["success"])
	code, respB := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadB),
		map[string]any{"action": "dismiss"})
	require.Equal(t, http.StatusOK, code, "%v", respB)

	get := func(query string) (int, map[string]any) {
		return keyEventRequest(t, engine, http.MethodGet,
			fmt.Sprintf("/api/daily-reports/topics/%d/key-events%s", w.topicID, query), nil)
	}

	// Default: confirmed only, days defaults to 14.
	code, got := get("")
	require.Equal(t, http.StatusOK, code, "%v", got)
	markers := got["data"].(map[string]any)["markers"].([]any)
	require.Len(t, markers, 1)
	assert.Equal(t, "线索A", markers[0].(map[string]any)["title_snapshot"])

	// Explicit non-confirmed state → 400 (总览不承担待处理状态).
	for _, state := range []string{"suggested", "dismissed", "all", "SUGGESTED"} {
		code, resp := get("?state=" + state)
		assert.Equal(t, http.StatusBadRequest, code, "state=%s: %v", state, resp)
	}
	// days contract: 0 legal, non-numeric / negative → 400 (不误当全部).
	code, got0 := get("?days=0")
	require.Equal(t, http.StatusOK, code)
	assert.Len(t, got0["data"].(map[string]any)["markers"].([]any), 1)
	for _, days := range []string{"abc", "-1", "1.5", "999999999999999999999"} {
		code, resp := get("?days=" + days)
		assert.Equal(t, http.StatusBadRequest, code, "days=%s: %v", days, resp)
	}
	// Unknown topic → 404.
	code, _ = keyEventRequest(t, engine, http.MethodGet, "/api/daily-reports/topics/999999/key-events", nil)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestKeyEventGet_ReportStatusDerivation(t *testing.T) {
	engine, repo := setupKeyEventHandlerTest(t)
	w := seedKeyEventWorld(t, repo)

	getStatus := func() string {
		code, resp := keyEventRequest(t, engine, http.MethodGet,
			fmt.Sprintf("/api/daily-reports/%d/key-events", w.reportID), nil)
		require.Equal(t, http.StatusOK, code, "%v", resp)
		return resp["data"].(map[string]any)["status"].(string)
	}

	// No run row → idle.
	assert.Equal(t, "idle", getStatus())

	// Persisted running without a live in-process task (restart killed it)
	// → honest degradation to failed.
	require.NoError(t, repo.UpsertKeyEventRecommendationRun(w.reportID, w.boardID, "tok-a"))
	assert.Equal(t, "failed", getStatus(), "running 存量读时如实降级")

	// Terminal states pass through.
	require.NoError(t, repo.FinishKeyEventRecommendationRun(w.reportID, "tok-a", repository.KeyEventRunStateDone, 3, 2, ""))
	assert.Equal(t, "done", getStatus())
	require.NoError(t, repo.FinishKeyEventRecommendationRun(w.reportID, "tok-a", repository.KeyEventRunStateFailed, 3, 0, "boom"))
	assert.Equal(t, "failed", getStatus())

	// Markers returned alongside (attached threadA marker visible).
	_, resp := keyEventRequest(t, engine, http.MethodPut,
		fmt.Sprintf("/api/daily-reports/topics/%d/key-events/threads/%d", w.topicID, w.threadA),
		map[string]any{"action": "mark"})
	require.Equal(t, true, resp["success"])
	code, got := keyEventRequest(t, engine, http.MethodGet,
		fmt.Sprintf("/api/daily-reports/%d/key-events", w.reportID), nil)
	require.Equal(t, http.StatusOK, code)
	list := got["data"].(map[string]any)["markers"].([]any)
	require.Len(t, list, 1)
	assert.Equal(t, "confirmed", list[0].(map[string]any)["state"])

	// Unknown report → 404.
	code, _ = keyEventRequest(t, engine, http.MethodGet, "/api/daily-reports/999999/key-events", nil)
	assert.Equal(t, http.StatusNotFound, code)
	// Invalid report id → 400.
	code, _ = keyEventRequest(t, engine, http.MethodGet, "/api/daily-reports/abc/key-events", nil)
	assert.Equal(t, http.StatusBadRequest, code)
}
