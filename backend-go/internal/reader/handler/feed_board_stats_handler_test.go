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
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
)

// 用例来源：openspec/changes/archive/2026-09-18-add-source-board-hit-rate/test-cases.md B4
//（源视角端点契约 7 条）。端点：GET /api/feeds/board-hit-stats?window=7。

func setupBoardHitStatsTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:board_hit_stats_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
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
	database.DB = db
	repository.InitRepository(db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	feeds := api.Group("/feeds")
	// 与 routes.go 相同的注册方式：静态段与 /:feed_id 同级共存。
	feeds.GET("/board-hit-stats", GetBoardHitStats)
	feeds.GET("/:feed_id", GetFeed)
	return db, r
}

func seedBHFeed(t *testing.T, db *gorm.DB, title string, taggingEnabled bool) models.Feed {
	t.Helper()
	feed := models.Feed{Title: title, URL: fmt.Sprintf("https://example.com/%s-%d.xml", title, time.Now().UnixNano()), TaggingEnabled: taggingEnabled}
	require.NoError(t, db.Create(&feed).Error)
	if !taggingEnabled {
		// default:true 标签会覆盖零值 false，关打标源需显式 UPDATE。
		require.NoError(t, db.Model(&models.Feed{}).Where("id = ?", feed.ID).UpdateColumn("tagging_enabled", false).Error)
	}
	return feed
}

func seedBHHitFixture(t *testing.T, db *gorm.DB) (models.SemanticLabel, models.TopicTag) {
	t.Helper()
	board := models.SemanticLabel{Label: "P", Slug: fmt.Sprintf("p-%d", time.Now().UnixNano()), LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	tag := models.TopicTag{Slug: fmt.Sprintf("t-%d", time.Now().UnixNano()), Label: "t", Status: "active", IsCanonical: true}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: board.ID, Score: 0.9}).Error)
	return board, tag
}

func seedBHArticle(t *testing.T, db *gorm.DB, feedID uint, taggedTagID *uint) models.Article {
	t.Helper()
	n := time.Now().UnixNano()
	article := models.Article{
		FeedID:    feedID,
		Title:     fmt.Sprintf("a-%d", n),
		Link:      fmt.Sprintf("https://example.com/a-%d", n),
		CreatedAt: time.Now().Add(-time.Hour),
	}
	require.NoError(t, db.Create(&article).Error)
	if taggedTagID != nil {
		require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: *taggedTagID, Score: 0.8, Source: "llm"}).Error)
	}
	return article
}

func callBoardHitStats(r *gin.Engine, query string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/feeds/board-hit-stats"+query, nil)
	r.ServeHTTP(rec, req)
	return rec
}

func decodeItems(t *testing.T, rec *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	var body struct {
		Success bool                     `json:"success"`
		Data    map[string][]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.True(t, body.Success)
	items := body.Data["items"]
	out := make([]map[string]interface{}, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		require.True(t, ok)
		out = append(out, m)
	}
	return out
}

// TC-B4-01：库内 3 个源 → success=true，data.items 长度 3。
func TestB4_01_ReturnsAllFeeds(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	for _, title := range []string{"a", "b", "c"} {
		seedBHFeed(t, db, title, true)
	}

	rec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	items := decodeItems(t, rec)
	require.Len(t, items, 3)
}

// TC-B4-02：每条记录含全字段。
func TestB4_02_AllFieldsPresent(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	seedBHFeed(t, db, "a", true)

	rec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code)
	items := decodeItems(t, rec)
	require.Len(t, items, 1)
	for _, key := range []string{
		"feed_id", "title", "tagging_enabled", "articles", "in_board",
		"tagged_no_board", "untagged_pending", "untagged_settled", "hit_rate", "boards",
	} {
		require.Contains(t, items[0], key, key)
	}
}

// TC-B4-03：源窗口内 67 篇、命中 56 → hit_rate == 56/67（浮点误差内），
// boards 为数组（可为空）。
func TestB4_03_HitRateIsRational(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	feed := seedBHFeed(t, db, "busy", true)
	_, tag := seedBHHitFixture(t, db)
	for i := 0; i < 67; i++ {
		if i < 56 {
			seedBHArticle(t, db, feed.ID, &tag.ID)
		} else {
			seedBHArticle(t, db, feed.ID, nil)
		}
	}

	rec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code)
	for _, item := range decodeItems(t, rec) {
		if item["feed_id"].(float64) == float64(feed.ID) {
			require.Equal(t, float64(67), item["articles"])
			require.Equal(t, float64(56), item["in_board"])
			require.InDelta(t, 56.0/67.0, item["hit_rate"], 1e-9)
			require.IsType(t, []interface{}{}, item["boards"])
		}
	}
}

// TC-B4-04：tagging_enabled=false 的源照常返回且标记为 false。
func TestB4_04_TaggingDisabledFeedReturnedAsIs(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	seedBHFeed(t, db, "off", false)

	rec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code)
	items := decodeItems(t, rec)
	require.Len(t, items, 1)
	require.Equal(t, false, items[0]["tagging_enabled"])
}

// TC-B4-05：连续调用两次同一窗口 → 两响应一致；tag_jobs 行数与 feeds 配置
// 不变（只读无副作用）。
func TestB4_05_ReadOnlyNoSideEffects(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	feed := seedBHFeed(t, db, "a", true)
	_, tag := seedBHHitFixture(t, db)
	article := seedBHArticle(t, db, feed.ID, nil)
	seedBHArticle(t, db, feed.ID, &tag.ID)
	require.NoError(t, db.Create(&models.TagJob{ArticleID: article.ID, Status: "pending", AvailableAt: time.Now(), MaxAttempts: 5}).Error)

	var tagJobsBefore, feedsBefore int64
	require.NoError(t, db.Model(&models.TagJob{}).Count(&tagJobsBefore).Error)
	require.NoError(t, db.Model(&models.Feed{}).Count(&feedsBefore).Error)

	first := callBoardHitStats(r, "?window=7")
	second := callBoardHitStats(r, "?window=7")

	require.Equal(t, first.Body.String(), second.Body.String())

	var tagJobsAfter, feedsAfter int64
	require.NoError(t, db.Model(&models.TagJob{}).Count(&tagJobsAfter).Error)
	require.NoError(t, db.Model(&models.Feed{}).Count(&feedsAfter).Error)
	require.Equal(t, tagJobsBefore, tagJobsAfter)
	require.Equal(t, feedsBefore, feedsAfter)
}

// TC-B4-06：库内无任何文章 → items 含全部源且 articles=0（不是空数组）。
func TestB4_06_NoArticlesStillAllFeeds(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	seedBHFeed(t, db, "a", true)
	seedBHFeed(t, db, "b", true)

	rec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusOK, rec.Code)
	items := decodeItems(t, rec)
	require.Len(t, items, 2)
	for _, item := range items {
		require.Equal(t, float64(0), item["articles"])
		require.Equal(t, float64(0), item["hit_rate"])
	}
}

// TC-B4-07：端点与既有 GET /api/feeds/:feed_id 路由共存，/board-hit-stats
// 不被当作 feed_id。
func TestB4_07_RouteCoexistenceWithFeedID(t *testing.T) {
	db, r := setupBoardHitStatsTest(t)
	feed := seedBHFeed(t, db, "a", true)

	statsRec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusOK, statsRec.Code, statsRec.Body.String())
	require.True(t, len(decodeItems(t, statsRec)) > 0)

	// 静态段没有被 :feed_id 吞掉的前提下，参数段路由照常工作。
	feedRec := httptest.NewRecorder()
	r.ServeHTTP(feedRec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/feeds/%d", feed.ID), nil))
	require.Equal(t, http.StatusOK, feedRec.Code, feedRec.Body.String())
	require.Contains(t, feedRec.Body.String(), `"title":"a"`)
}

// 非法 window → 400，不静默回退默认值（口径硬约束 ③；对应 TC-B3-04 端点侧）。
func TestB4_InvalidWindowReturns400(t *testing.T) {
	_, r := setupBoardHitStatsTest(t)
	for _, q := range []string{"?window=14", "?window=0", "?window=abc", "?window=-7"} {
		rec := callBoardHitStats(r, q)
		require.Equal(t, http.StatusBadRequest, rec.Code, q)
		require.Contains(t, rec.Body.String(), "success\":false")
	}
}

// 聚合失败 → 500：把全局仓储指到一个未迁移的库即可触发 SQL 错误。
func TestB4_AggregationFailureReturns500(t *testing.T) {
	_, r := setupBoardHitStatsTest(t)
	empty, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:broken_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	repository.InitRepository(empty)
	defer repository.InitRepository(database.DB)

	rec := callBoardHitStats(r, "?window=7")
	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}
