package handler

import (
	"bytes"
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
	tagging "syntopica-backend/internal/tagmanagement"
)

func setupFeedHandlerRefreshTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:feed_refresh_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Category{}, &models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}))
	database.DB = db
	repository.InitRepository(database.DB)
	tagging.InitRepository(database.DB)
}

// TestUpdateFeedRefreshIntervalBug 端到端复现：
// 前端 updateFeedSetting 只带被改的单个字段（如 color），
// 不含 refresh_interval。后端 UpdateFeed 用 `req.RefreshInterval >= 0`
// 判断是否更新——但 Go 的 int 零值=0，0>=0 恒真，于是把 refresh_interval
// 误写成 0，导致该 feed 被 AutoRefreshJob 的 `WHERE refresh_interval > 0` 永久排除。
//
// 对照组：显式传 refresh_interval=30 → 正确更新为 30。
// 复现组：只改 color → refresh_interval 应保持 30，实际被误写成 0（bug）。
func TestUpdateFeedRefreshIntervalBug(t *testing.T) {
	setupFeedHandlerRefreshTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "少数派", URL: "https://sspai.com/feed", RefreshInterval: 60}
	require.NoError(t, database.DB.Create(&feed).Error)

	callUpdate := func(body string) {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Params = gin.Params{{Key: "feed_id", Value: fmt.Sprintf("%d", feed.ID)}}
		ctx.Request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/feeds/%d", feed.ID), bytes.NewBufferString(body))
		UpdateFeed(ctx)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	reload := func() models.Feed {
		var f models.Feed
		require.NoError(t, database.DB.First(&f, feed.ID).Error)
		return f
	}

	// 对照组：显式传 refresh_interval=30 → 正确更新
	callUpdate(`{"refresh_interval":30,"color":"#111111"}`)
	require.Equal(t, 30, reload().RefreshInterval, "显式传值时应正确更新为 30")

	// 修复验证：只改 color，不带 refresh_interval
	// 期望：refresh_interval 保持 30（不被零值误写）
	callUpdate(`{"color":"#222222"}`)
	after := reload()
	t.Logf("只改color后 refresh_interval: 期望=30, 实际=%d", after.RefreshInterval)

	require.Equal(t, 30, after.RefreshInterval, "修复后：不带 refresh_interval 的更新应保持原值 30")
}

// TestUpdateFeedCategoryPresence 验证 category_id 用 presence 检查：
// 显式传 null → 置空分类；不带该字段 → 保持不变（不被误改）。
func TestUpdateFeedCategoryPresence(t *testing.T) {
	setupFeedHandlerRefreshTestDB(t)
	gin.SetMode(gin.TestMode)

	cat := models.Category{Name: "科技", Slug: "tech", Color: "#abc", Icon: "folder"}
	require.NoError(t, database.DB.Create(&cat).Error)

	feed := models.Feed{Title: "少数派", URL: "https://sspai.com/feed", CategoryID: &cat.ID}
	require.NoError(t, database.DB.Create(&feed).Error)

	callUpdate := func(body string) {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Params = gin.Params{{Key: "feed_id", Value: fmt.Sprintf("%d", feed.ID)}}
		ctx.Request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/feeds/%d", feed.ID), bytes.NewBufferString(body))
		UpdateFeed(ctx)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	reload := func() models.Feed {
		var f models.Feed
		require.NoError(t, database.DB.First(&f, feed.ID).Error)
		return f
	}

	// 对照：不带 category_id 的更新不应改动分类
	callUpdate(`{"max_articles":200}`)
	kept := reload()
	require.NotNil(t, kept.CategoryID, "不带 category_id 时分类应保持不变")
	require.Equal(t, cat.ID, *kept.CategoryID)

	// 显式传 null → 置空分类
	callUpdate(`{"category_id":null}`)
	require.Nil(t, reload().CategoryID, "显式传 null 应置空分类")
}

// TestUpdateFeedAIDetectionEnabled 回归（add-ai-generated-article-detection 5.6 联调发现）：
// 前端订阅源详情的「AI 生成检测」toggle 只发 ai_detection_enabled 这一个字段，
// 后端 UpdateFeedRequest 若不含该字段，请求仍回 200 但开关不落库（界面刷新后弹回），
// 判定链路的 feed 级闸门于是永远关着。这里断言双向都能写进库、不带该字段时不被改动。
func TestUpdateFeedAIDetectionEnabled(t *testing.T) {
	setupFeedHandlerRefreshTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "掘金人工智能", URL: "https://juejin.cn/ai/feed"}
	require.NoError(t, database.DB.Create(&feed).Error)
	require.False(t, feed.AIDetectionEnabled, "默认必须关闭（design D4：逐源人工开启）")

	callUpdate := func(body string) {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Params = gin.Params{{Key: "feed_id", Value: fmt.Sprintf("%d", feed.ID)}}
		ctx.Request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/feeds/%d", feed.ID), bytes.NewBufferString(body))
		UpdateFeed(ctx)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	reload := func() models.Feed {
		var f models.Feed
		require.NoError(t, database.DB.First(&f, feed.ID).Error)
		return f
	}

	callUpdate(`{"ai_detection_enabled":true}`)
	require.True(t, reload().AIDetectionEnabled, "开关必须真的落库（否则 feed 级闸门永久关闭）")

	// 不带该字段的其它更新不得把开关改回默认
	callUpdate(`{"color":"#333333"}`)
	require.True(t, reload().AIDetectionEnabled, "未提供 ai_detection_enabled 时应保持 true")

	callUpdate(`{"ai_detection_enabled":false}`)
	require.False(t, reload().AIDetectionEnabled, "关闭开关必须落库")
}
