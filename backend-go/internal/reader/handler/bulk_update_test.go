package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// bulk-update scope 契约测试（fix-bulk-markall-all-scope）。
// 复现测试先行：all:true 现状 400（bug），实现加入 all scope 后转绿。

func setupBulkUpdateTest(t *testing.T) {
	t.Helper()
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
}

func seedUnreadArticles(t *testing.T, n int) {
	t.Helper()
	category := models.Category{Name: "AI", Slug: "ai-bulk", Color: "#3b6b87", Icon: "mdi:brain"}
	require.NoError(t, database.DB.Create(&category).Error)
	feed := models.Feed{Title: "Bulk Feed", URL: "https://example.com/bulk", CategoryID: &category.ID}
	require.NoError(t, database.DB.Create(&feed).Error)
	for i := 0; i < n; i++ {
		a := models.Article{
			FeedID:    feed.ID,
			Title:     "bulk article",
			Link:      "https://example.com/bulk/" + time.Now().Format("150405.000000000") + "/" + string(rune('a'+i)),
			CreatedAt: time.Date(2026, 9, 18, 10, 0, 0, 0, models.ShanghaiTZ),
		}
		require.NoError(t, database.DB.Create(&a).Error)
	}
}

func performBulk(t *testing.T, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/articles/bulk-update", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	BulkUpdateArticles(ctx)
	return recorder
}

func countUnread(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, database.DB.Model(&models.Article{}).Where("read = ?", false).Count(&n).Error)
	return n
}

// [复现] all:true → 现状 400（bug）：全站标已读无 scope 被拒。修复后应 200 且全表置已读。
func TestBulkUpdateAllScope(t *testing.T) {
	setupBulkUpdateTest(t)
	seedUnreadArticles(t, 3)

	rec := performBulk(t, map[string]interface{}{"read": true, "all": true})

	// 修复前：400（Must specify a scope）——红轮证据；修复后：200
	if rec.Code != http.StatusOK {
		t.Fatalf("RED (bug 复现): all=true 现状被拒 code=%d body=%s", rec.Code, rec.Body.String())
	}
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, int64(0), countUnread(t), "全站 3 篇应全部置已读")
}

// [契约] all 与其他 scope 互斥 → 400
func TestBulkUpdateAllConflictWithFeedScope(t *testing.T) {
	setupBulkUpdateTest(t)
	seedUnreadArticles(t, 2)

	feedID := uint(1)
	rec := performBulk(t, map[string]interface{}{"read": true, "all": true, "feed_id": feedID})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "all cannot be combined")
}

// [契约保留] 无 scope 且无 all → 400 文案不变（硬化语义不放松）
func TestBulkUpdateNoScopeStillRejected(t *testing.T) {
	setupBulkUpdateTest(t)
	seedUnreadArticles(t, 2)

	rec := performBulk(t, map[string]interface{}{"read": true})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "Must specify a scope")
}

// [契约保留] 仅 scope 无更新字段 → 400 "At least one field"
func TestBulkUpdateScopeWithoutField(t *testing.T) {
	setupBulkUpdateTest(t)
	seedUnreadArticles(t, 2)

	rec := performBulk(t, map[string]interface{}{"all": true})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "At least one field")
}
