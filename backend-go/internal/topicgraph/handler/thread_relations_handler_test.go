package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// ── 线索相关事件端点 HTTP 语义（replace-lane-lifeline-with-topic-events 7.2）──
//
// PG testcontainer（与 repository 测试同款）：只断言参数解析、状态码与响应
// 骨架。覆盖 spec Scenario「非法天数回落并回显」「日报不存在」「无任何推荐」
// 「读请求不产生写」，加上 days 边界值清单（空/7/30/14/0/-1/365/abc）、
// `skipped_no_embedding` 透出与 `threads:{}` 的对象字面量。

func setupThreadRelationsHandlerTest(t *testing.T) (*gin.Engine, *repository.TopicGraphRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	engine := gin.New()
	RegisterThreadRelationRoutes(engine.Group("/api"))
	return engine, repository.Repo
}

func throDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

// throWorld carries the board/topic every seeded report hangs under.
type throWorld struct {
	repo    *repository.TopicGraphRepository
	db      *gorm.DB
	boardID uint
	topicID uint
}

func throSeedWorld(t *testing.T, repo *repository.TopicGraphRepository) throWorld {
	t.Helper()
	db := repo.DB()

	board := models.SemanticLabel{Label: "thr-board", Slug: "thr-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)

	topic := repository.BoardPersistentTopic{
		SemanticBoardID: board.ID,
		Label:           "线索相关事件泳道",
		Status:          repository.TopicStatusActive,
		FirstSeenDate:   repository.NormalizeReportDate(throDay(2026, 11, 17)),
		LastSeenDate:    repository.NormalizeReportDate(throDay(2026, 11, 17)),
	}
	require.NoError(t, db.Omit("embedding", "centroid").Create(&topic).Error)

	return throWorld{repo: repo, db: db, boardID: board.ID, topicID: topic.ID}
}

// report seeds one report on `day`.
func (w throWorld) report(t *testing.T, day time.Time) uint {
	t.Helper()
	report := repository.BoardDailyReport{
		SemanticBoardID: w.boardID,
		PeriodDate:      repository.NormalizeReportDate(day),
		Title:           "日报 " + day.Format("2006-01-02"),
		Status:          "completed",
	}
	require.NoError(t, w.db.Create(&report).Error)
	return report.ID
}

// thread seeds one topic-owned section + one thread. A nil vector leaves the
// embedding column NULL (the "no vector" path).
func (w throWorld) thread(t *testing.T, reportID uint, title string, vector []float64, articleIDs []uint) uint {
	t.Helper()
	section := repository.DailyReportSection{
		ReportID:          reportID,
		ClusterLabel:      "章节",
		Embedding:         repository.FloatsToPgVector([]float64{0}),
		PersistentTopicID: &w.topicID,
	}
	require.NoError(t, w.db.Create(&section).Error)

	arts, err := json.Marshal(articleIDs)
	require.NoError(t, err)
	thread := repository.DailyReportThread{
		ReportID:          reportID,
		SectionID:         section.ID,
		Title:             title,
		Summary:           "摘要：" + title,
		TagIDs:            repository.JSON("[]"),
		RelatedArticleIDs: repository.JSON(arts),
	}
	query := w.db
	if vector == nil {
		query = w.db.Omit("embedding")
	} else {
		thread.Embedding = repository.FloatsToPgVector(vector)
	}
	require.NoError(t, query.Create(&thread).Error)
	return thread.ID
}

// seedHappyPath: current report (11.17) with one thread + window source
// (11.15) carrying the identical vector and a source article.
func (w throWorld) seedHappyPath(t *testing.T) (reportID, threadID, sourceThreadID, articleID uint) {
	t.Helper()
	vec := []float64{1, 0}
	reportID = w.report(t, throDay(2026, 11, 17))
	threadID = w.thread(t, reportID, "当期线索", vec, nil)

	feed := models.Feed{Title: "示例来源", URL: "https://example.com/thr-feed"}
	require.NoError(t, w.db.Create(&feed).Error)
	pub := throDay(2026, 11, 15)
	article := models.Article{FeedID: feed.ID, Title: "来源文章", PubDate: &pub}
	require.NoError(t, w.db.Create(&article).Error)

	sourceReport := w.report(t, throDay(2026, 11, 15))
	sourceThreadID = w.thread(t, sourceReport, "窗口内来源", vec, []uint{article.ID})
	return reportID, threadID, sourceThreadID, article.ID
}

func throGet(t *testing.T, engine *gin.Engine, path string) (int, []byte, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	raw := w.Body.Bytes()
	var payload map[string]any
	if len(raw) > 0 {
		require.NoError(t, json.Unmarshal(raw, &payload))
	}
	return w.Code, raw, payload
}

func throData(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	require.Equal(t, true, payload["success"])
	data, ok := payload["data"].(map[string]any)
	require.True(t, ok, "响应 data 段必须是对象：%v", payload)
	return data
}

// ── 200：契约字段与分组键 ────────────────────────────────────────────────

func TestThreadRelationsHandler_OK(t *testing.T) {
	engine, repo := setupThreadRelationsHandlerTest(t)
	w := throSeedWorld(t, repo)
	reportID, threadID, sourceThreadID, articleID := w.seedHappyPath(t)

	code, _, payload := throGet(t, engine, fmt.Sprintf("/api/daily-reports/%d/thread-relations?days=7", reportID))
	require.Equal(t, http.StatusOK, code, "%v", payload)
	data := throData(t, payload)

	assert.Equal(t, float64(reportID), data["report_id"])
	assert.Equal(t, float64(7), data["days"])
	assert.Equal(t, "2026-11-10", data["window_start"])
	assert.Equal(t, "2026-11-16", data["window_end"])
	assert.Equal(t, repository.ThreadRelationStatusReady, data["status"])
	assert.Equal(t, float64(1), data["total"])
	assert.Equal(t, float64(0), data["skipped_no_embedding"])
	_, hasStale := data["stale_candidates"]
	assert.False(t, hasStale, "旧口径的 stale_candidates 字段必须彻底消失")

	threads, ok := data["threads"].(map[string]any)
	require.True(t, ok, "threads 必须是按 thread id 分组的对象")
	list, ok := threads[fmt.Sprintf("%d", threadID)].([]any)
	require.True(t, ok, "分组键是 thread id 的字符串（JSON 对象键）")
	require.Len(t, list, 1)
	item := list[0].(map[string]any)
	assert.Equal(t, float64(sourceThreadID), item["thread_id"])
	assert.Equal(t, "窗口内来源", item["title"])
	assert.Equal(t, "摘要：窗口内来源", item["summary"])
	assert.Equal(t, "2026-11-15", item["period_date"])
	assert.InDelta(t, 1.0, item["score"], 1e-12, "score 就是余弦相似度本身")
	assert.Equal(t, repository.ThreadRelationLevelStrong, item["level"])
	assert.Equal(t, false, item["confirmed"])

	sourceArticle, ok := item["source_article"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(articleID), sourceArticle["id"])
	assert.Equal(t, "来源文章", sourceArticle["title"])
	assert.Equal(t, "示例来源", sourceArticle["feed"])
	assert.NotNil(t, sourceArticle["published_at"])

	// 旧口径的字段一个都不许残留。
	for _, gone := range []string{"tag_id", "label", "description", "occurrences", "last_seen", "source_thread"} {
		_, exists := item[gone]
		assert.False(t, exists, "旧字段 %q 必须消失", gone)
	}
}

// ── 天数边界：只有 7/30 通过，其它回落 7 并回显（不报 400）─────────────────

func TestThreadRelationsHandler_DaysFallbackAndEcho(t *testing.T) {
	engine, repo := setupThreadRelationsHandlerTest(t)
	w := throSeedWorld(t, repo)
	reportID, _, _, _ := w.seedHappyPath(t)

	cases := map[string]float64{
		"":    7,
		"7":   7,
		"30":  30,
		"14":  7,
		"0":   7,
		"-1":  7,
		"365": 7,
		"abc": 7,
	}
	for raw, wantDays := range cases {
		path := fmt.Sprintf("/api/daily-reports/%d/thread-relations", reportID)
		if raw != "" {
			path += "?days=" + raw
		}
		code, _, payload := throGet(t, engine, path)
		require.Equal(t, http.StatusOK, code, "days=%q 不得报 400：%v", raw, payload)
		data := throData(t, payload)
		assert.Equal(t, wantDays, data["days"], "days=%q 应回显实际生效值", raw)
	}

	// 30 天档真的换了窗口（不是只改回显字段）。
	_, _, wide := throGet(t, engine, fmt.Sprintf("/api/daily-reports/%d/thread-relations?days=30", reportID))
	assert.Equal(t, "2026-10-18", throData(t, wide)["window_start"])
}

// ── 404 / 400：日报不存在 vs 参数非法 ────────────────────────────────────

func TestThreadRelationsHandler_ReportNotFound(t *testing.T) {
	engine, repo := setupThreadRelationsHandlerTest(t)
	w := throSeedWorld(t, repo)
	w.seedHappyPath(t)

	code, _, payload := throGet(t, engine, "/api/daily-reports/999999/thread-relations")
	assert.Equal(t, http.StatusNotFound, code)
	assert.Equal(t, false, payload["success"])

	// 非法 id 不是「不存在」，是参数错误（400）。
	code, _, _ = throGet(t, engine, "/api/daily-reports/abc/thread-relations")
	assert.Equal(t, http.StatusBadRequest, code)
}

// ── 无任何推荐：threads 必须是 {} 而非 null，status=empty ─────────────────

func TestThreadRelationsHandler_EmptyThreadsObject(t *testing.T) {
	engine, repo := setupThreadRelationsHandlerTest(t)
	w := throSeedWorld(t, repo)
	reportID := w.report(t, throDay(2026, 11, 17))

	code, raw, payload := throGet(t, engine, fmt.Sprintf("/api/daily-reports/%d/thread-relations", reportID))
	require.Equal(t, http.StatusOK, code, "%v", payload)
	data := throData(t, payload)
	assert.Equal(t, repository.ThreadRelationStatusEmpty, data["status"])
	assert.Equal(t, float64(0), data["total"])
	assert.Equal(t, float64(0), data["skipped_no_embedding"])

	threads, ok := data["threads"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, threads)
	assert.Contains(t, string(raw), `"threads":{}`, "空集必须是对象字面量，不是 null")
}

// ── partial：当期线索缺向量 → skipped_no_embedding 透出且 status=partial ──

func TestThreadRelationsHandler_SkippedNoEmbeddingSurfaced(t *testing.T) {
	engine, repo := setupThreadRelationsHandlerTest(t)
	w := throSeedWorld(t, repo)
	reportID, threadID, _, _ := w.seedHappyPath(t)
	skippedThreadID := w.thread(t, reportID, "缺向量的线索", nil, nil)

	code, _, payload := throGet(t, engine, fmt.Sprintf("/api/daily-reports/%d/thread-relations", reportID))
	require.Equal(t, http.StatusOK, code, "%v", payload)
	data := throData(t, payload)
	assert.Equal(t, float64(1), data["skipped_no_embedding"])
	assert.Equal(t, repository.ThreadRelationStatusPartial, data["status"])

	threads := data["threads"].(map[string]any)
	_, present := threads[fmt.Sprintf("%d", threadID)]
	assert.True(t, present)
	_, skippedPresent := threads[fmt.Sprintf("%d", skippedThreadID)]
	assert.False(t, skippedPresent, "缺向量的线索不得出现在 threads 里")
}

// ── 幂等：重复请求响应等价（读请求不产生写）───────────────────────────────

func TestThreadRelationsHandler_RepeatedRequestsEquivalent(t *testing.T) {
	engine, repo := setupThreadRelationsHandlerTest(t)
	w := throSeedWorld(t, repo)
	reportID, _, _, _ := w.seedHappyPath(t)
	path := fmt.Sprintf("/api/daily-reports/%d/thread-relations?days=7", reportID)

	codeA, rawA, _ := throGet(t, engine, path)
	codeB, rawB, _ := throGet(t, engine, path)
	assert.Equal(t, http.StatusOK, codeA)
	assert.Equal(t, codeA, codeB)
	assert.JSONEq(t, string(rawA), string(rawB))
	assert.Equal(t, string(rawA), string(rawB), "同一输入两次查询逐字节等价")
}
