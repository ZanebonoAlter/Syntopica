package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// 用例来源：openspec/changes/archive/2026-09-18-add-source-board-hit-rate/test-cases.md B5
//（板块视角端点契约 8 条）。端点：GET /api/semantic-boards/:id/source-breakdown?window=7。
// 用内存 SQLite（无 pgvector 依赖的轻量端点契约测试），走完整
// RegisterSemanticBoardRoutes 注册路径。

func setupBoardSourceBreakdownTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:board_source_breakdown_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.ArticleTopicTag{},
		&models.SemanticLabel{},
		&models.TopicTagBoardLabel{},
		&models.TagJob{},
	))
	repository.InitRepository(db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterSemanticBoardRoutes(api)
	return db, r
}

func seedBSBoard(t *testing.T, db *gorm.DB, label, slug, labelType, status string) models.SemanticLabel {
	t.Helper()
	row := models.SemanticLabel{Label: label, Slug: fmt.Sprintf("%s-%d", slug, time.Now().UnixNano()), LabelType: labelType, Status: status}
	require.NoError(t, db.Create(&row).Error)
	return row
}

func seedBSFeed(t *testing.T, db *gorm.DB, title string) models.Feed {
	t.Helper()
	feed := models.Feed{Title: title, URL: fmt.Sprintf("https://example.com/%s-%d.xml", title, time.Now().UnixNano())}
	require.NoError(t, db.Create(&feed).Error)
	return feed
}

// seedBSSourceTag 建一个挂到指定板块的标签链（topic_tag → topic_tag_board_labels）。
func seedBSSourceTag(t *testing.T, db *gorm.DB, boardID uint) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Slug: fmt.Sprintf("t-%d", time.Now().UnixNano()), Label: "t", Status: "active", IsCanonical: true}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: boardID, Score: 0.9}).Error)
	return tag
}

func seedBSArticle(t *testing.T, db *gorm.DB, feedID uint, taggedTagIDs ...uint) models.Article {
	t.Helper()
	n := time.Now().UnixNano()
	article := models.Article{
		FeedID:    feedID,
		Title:     fmt.Sprintf("a-%d", n),
		Link:      fmt.Sprintf("https://example.com/a-%d", n),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	require.NoError(t, db.Create(&article).Error)
	for _, tagID := range taggedTagIDs {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tagID, Score: 0.8, Source: "llm"}).Error)
	}
	return article
}

func callBreakdown(r *gin.Engine, boardID uint, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/source-breakdown%s", boardID, query), nil)
	r.ServeHTTP(rec, req)
	return rec
}

func decodeBreakdown(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, true, body["success"], rec.Body.String())
	data, ok := body["data"].(map[string]interface{})
	require.True(t, ok, rec.Body.String())
	return data
}

func breakdownSources(t *testing.T, data map[string]interface{}) []map[string]interface{} {
	t.Helper()
	rawSources, ok := data["sources"].([]interface{})
	require.True(t, ok, "sources must be an array")
	out := make([]map[string]interface{}, 0, len(rawSources))
	for _, s := range rawSources {
		m, ok := s.(map[string]interface{})
		require.True(t, ok)
		out = append(out, m)
	}
	return out
}

// TC-B5-01：板块窗口内命中若干篇、3 个源 → total_articles = Σ sources[].articles，
// source_count = 3。
func TestB5_01_SourcesSumEqualsTotal(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	board := seedBSBoard(t, db, "P", "p", "board", "active")
	tag := seedBSSourceTag(t, db, board.ID)
	total := 0
	for i, title := range []string{"f1", "f2", "f3"} {
		feed := seedBSFeed(t, db, title)
		for j := 0; j <= i; j++ { // 1 + 2 + 3 = 6 篇
			seedBSArticle(t, db, feed.ID, tag.ID)
			total++
		}
	}

	rec := callBreakdown(r, board.ID, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	data := decodeBreakdown(t, rec)
	require.Equal(t, float64(total), data["total_articles"])
	require.Equal(t, float64(3), data["source_count"])

	sum := int64(0)
	for _, s := range breakdownSources(t, data) {
		sum += int64(s["articles"].(float64))
	}
	require.Equal(t, int64(total), sum)
}

// TC-B5-02：每条来源含 feed_id/title/articles/share/feed_articles/feed_hit_rate，
// 且 share == articles / total_articles。
func TestB5_02_SourceFieldsAndShare(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	board := seedBSBoard(t, db, "P", "p", "board", "active")
	tag := seedBSSourceTag(t, db, board.ID)
	f1 := seedBSFeed(t, db, "f1")
	f2 := seedBSFeed(t, db, "f2")
	for i := 0; i < 3; i++ {
		seedBSArticle(t, db, f1.ID, tag.ID)
	}
	seedBSArticle(t, db, f2.ID, tag.ID)

	data := decodeBreakdown(t, callBreakdown(r, board.ID, "?window=7"))
	require.Equal(t, float64(4), data["total_articles"])
	sources := breakdownSources(t, data)
	require.Len(t, sources, 2)
	var total float64 = data["total_articles"].(float64)
	for _, s := range sources {
		for _, key := range []string{"feed_id", "title", "articles", "share", "feed_articles", "feed_hit_rate"} {
			require.Contains(t, s, key, key)
		}
		require.InDelta(t, s["articles"].(float64)/total, s["share"], 1e-9)
	}
	// 默认按篇数降序：f1(3) 在前。
	require.Equal(t, "f1", sources[0]["title"])
	require.Equal(t, float64(3), sources[0]["articles"])
}

// TC-B5-03：源在板块内 12 篇，窗口内总量 162、其中 71 篇命中（12 篇在本板块 +
// 59 篇在另一板块）→ feed_articles=162、feed_hit_rate ≈ 71/162 ≈ 0.44。
func TestB5_03_ContextColumnsFeedArticlesAndHitRate(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	boardP := seedBSBoard(t, db, "P", "p", "board", "active")
	boardQ := seedBSBoard(t, db, "Q", "q", "board", "active")
	tagP := seedBSSourceTag(t, db, boardP.ID)
	tagQ := seedBSSourceTag(t, db, boardQ.ID)
	feed := seedBSFeed(t, db, "occasional")

	for i := 0; i < 12; i++ {
		seedBSArticle(t, db, feed.ID, tagP.ID)
	}
	for i := 0; i < 59; i++ {
		seedBSArticle(t, db, feed.ID, tagQ.ID)
	}
	for i := 0; i < 91; i++ { // 162 - 71 篇无标签
		seedBSArticle(t, db, feed.ID)
	}

	data := decodeBreakdown(t, callBreakdown(r, boardP.ID, "?window=7"))
	sources := breakdownSources(t, data)
	require.Len(t, sources, 1)
	s := sources[0]
	require.Equal(t, float64(12), s["articles"])
	require.Equal(t, float64(162), s["feed_articles"])
	require.InDelta(t, 71.0/162.0, s["feed_hit_rate"], 1e-9)
}

// TC-B5-04：请求不存在的板块 id → 404。
func TestB5_04_MissingBoardIs404(t *testing.T) {
	_, r := setupBoardSourceBreakdownTest(t)
	rec := callBreakdown(r, 999999, "?window=7")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TC-B5-05：请求 label_type='auxiliary' 的 id → 404（非 board 类型）。
func TestB5_05_AuxiliaryLabelIs404(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	aux := seedBSBoard(t, db, "aux", "aux", "auxiliary", "active")

	rec := callBreakdown(r, aux.ID, "?window=7")
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TC-B5-06：板块窗口内无命中文章 → 200、total_articles=0、source_count=0、
// sources=[]（空数组非 null，正常结果非错误）。
func TestB5_06_EmptyBoardIs200WithZeros(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	board := seedBSBoard(t, db, "empty", "empty", "board", "active")

	rec := callBreakdown(r, board.ID, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	data := decodeBreakdown(t, rec)
	require.Equal(t, float64(0), data["total_articles"])
	require.Equal(t, float64(0), data["source_count"])
	require.NotNil(t, data["sources"])
	require.Empty(t, data["sources"])
	// sources 序列化为 [] 而非 null。
	require.Contains(t, rec.Body.String(), `"sources":[]`)
}

// TC-B5-07：window=abc → 400（不静默回退）。
func TestB5_07_InvalidWindowIs400(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	board := seedBSBoard(t, db, "P", "p", "board", "active")
	for _, q := range []string{"?window=abc", "?window=14", "?window=-7", "?window=0"} {
		rec := callBreakdown(r, board.ID, q)
		require.Equal(t, http.StatusBadRequest, rec.Code, q)
	}
}

// TC-B5-08：同一文章命中板块 P 与 Q → 在两个板块的来源统计中各计 1（各自去重）。
func TestB5_08_SameArticleCountedInBothBoards(t *testing.T) {
	db, r := setupBoardSourceBreakdownTest(t)
	boardP := seedBSBoard(t, db, "P", "p", "board", "active")
	boardQ := seedBSBoard(t, db, "Q", "q", "board", "active")
	tagP := seedBSSourceTag(t, db, boardP.ID)
	tagQ := seedBSSourceTag(t, db, boardQ.ID)
	feed := seedBSFeed(t, db, "f")
	seedBSArticle(t, db, feed.ID, tagP.ID, tagQ.ID) // 一篇文章、两个标签、两板块

	dataP := decodeBreakdown(t, callBreakdown(r, boardP.ID, "?window=7"))
	dataQ := decodeBreakdown(t, callBreakdown(r, boardQ.ID, "?window=7"))
	require.Equal(t, float64(1), dataP["total_articles"])
	require.Equal(t, float64(1), dataQ["total_articles"])
	require.Len(t, breakdownSources(t, dataP), 1)
	require.Len(t, breakdownSources(t, dataQ), 1)
}
