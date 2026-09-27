package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/safefetch"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/service"
)

// ── CreateFeed handler 迁移到共享建源服务后的回归（4.5）──
//
// 建源统一走 service.CreateFeedWithVerification：地址规范化 → 安全抓取 + RSS 解析
// 验证 → 短事务按规范化 URL 去重/建源。handler 只做参数解析与错误映射。

const handlerValidRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>H</title><link>https://example.com</link>
<description>d</description><item><title>i</title><link>https://example.com/1</link></item></channel></rss>`

func setupCreateFeedHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:create_feed_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Feed{}))
	database.DB = db
	repository.InitRepository(database.DB)
	return db
}

// mockHandlerFetch 注入可控抓取，返回调用计数。
func mockHandlerFetch(t *testing.T, status int, body string, err error) *int {
	t.Helper()
	calls := 0
	restore := service.SetSubscriptionFetcher(func(_ context.Context, rawURL string, _ safefetch.Options) (*safefetch.Result, error) {
		calls++
		if err != nil {
			return nil, err
		}
		return &safefetch.Result{StatusCode: status, Body: []byte(body), FinalURL: rawURL}, nil
	})
	t.Cleanup(restore)
	return &calls
}

func callCreateFeed(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/feeds", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	CreateFeed(ctx)
	return rec
}

// 迁移后正常建源：规范化地址 + 安全验证通过 → 201 且落库。
func TestCreateFeedHandlerCreatesVerifiedFeed(t *testing.T) {
	db := setupCreateFeedHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	calls := mockHandlerFetch(t, 200, handlerValidRSS, nil)

	rec := callCreateFeed(t, `{"url":"HTTPS://Feeds.Example.com:443/tech.xml","title":"Tech"}`)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, 1, *calls)

	var feed models.Feed
	require.NoError(t, db.Where("url = ?", "https://feeds.example.com/tech.xml").First(&feed).Error)
	require.Equal(t, "Tech", feed.Title)

	// 同地址重复建源 → 409（复用既有 Feed，不再建第二个）。
	dup := callCreateFeed(t, `{"url":"https://feeds.example.com/tech.xml"}`)
	require.Equal(t, http.StatusConflict, dup.Code, dup.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// HTTP 200 但非 RSS → 400，不建源。
func TestCreateFeedHandlerRejectsNonRSSBody(t *testing.T) {
	db := setupCreateFeedHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	mockHandlerFetch(t, 200, "<html><body>nope</body></html>", nil)

	rec := callCreateFeed(t, `{"url":"https://example.com/page"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "not a parseable RSS/Atom")

	var count int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

// 非法/超长地址 → 400，且不发起抓取。
func TestCreateFeedHandlerRejectsInvalidURLLocally(t *testing.T) {
	setupCreateFeedHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	calls := mockHandlerFetch(t, 200, handlerValidRSS, nil)

	for _, body := range []string{
		`{"url":"ftp://example.com/feed"}`,
		`{"url":"https://example.com/"` + strings.Repeat("a", 600) + `"}`,
	} {
		rec := callCreateFeed(t, body)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	require.Equal(t, 0, *calls, "非法地址须在抓取前就地拒绝")
}

// 验证失败错误经 handler 回传为 400 且可读。
func TestCreateFeedHandlerMapsVerificationError(t *testing.T) {
	setupCreateFeedHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	mockHandlerFetch(t, 0, "", safefetch.ErrTimeout)

	rec := callCreateFeed(t, `{"url":"https://example.com/slow"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.False(t, body.Success)
	require.Contains(t, body.Error, "feed verification failed")
}
