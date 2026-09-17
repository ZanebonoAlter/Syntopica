package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/notification"
	"syntopica-backend/internal/platform/testutil"
)

// setupNotificationHandlerDB wires the shared testcontainer PG (the
// notification service uses PG-specific constraint DELETE for eviction, so
// the established in-memory sqlite handler pattern does not apply).
func setupNotificationHandlerDB(t *testing.T) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	// handlers read database.DB via the notification service global
	require.NotNil(t, database.DB)
}

type handlerCall struct {
	recorder *httptest.ResponseRecorder
}

func performRequest(handler gin.HandlerFunc, method, path string, params gin.Params) *handlerCall {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, nil)
	ctx.Params = params
	handler(ctx)
	return &handlerCall{recorder: recorder}
}

func (c *handlerCall) requireOK(t *testing.T) map[string]interface{} {
	t.Helper()
	require.Equal(t, http.StatusOK, c.recorder.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(c.recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
	return body
}

func dataOf(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, ok := body["data"].(map[string]interface{})
	require.True(t, ok, "data must be an object")
	return data
}

// TestNotificationAPI_UnreadCountAndList covers S1 步 5 + V3/V4/V5:
// empty table → 0 + empty list (no error); single element pagination;
// out-of-range offset.
func TestNotificationAPI_UnreadCountAndList(t *testing.T) {
	setupNotificationHandlerDB(t)

	// V3：空表 → 0 + 空列表，不报错
	rec := performRequest(GetUnreadCount, http.MethodGet, "/", nil)
	body := rec.requireOK(t)
	require.Equal(t, float64(0), dataOf(t, body)["unread"])

	rec = performRequest(ListNotifications, http.MethodGet, "/?limit=20&offset=0", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(0), dataOf(t, body)["total"])

	// V4：单元素分页
	notification.DailyReportSuccess(time.Now(), 1, 1)
	rec = performRequest(ListNotifications, http.MethodGet, "/?limit=20&offset=0", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(1), dataOf(t, body)["total"])
	require.Len(t, dataOf(t, body)["notifications"].([]interface{}), 1)

	// V5：越界 offset → 空列表不报错
	rec = performRequest(ListNotifications, http.MethodGet, "/?limit=20&offset=999", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(1), dataOf(t, body)["total"])
	require.Len(t, dataOf(t, body)["notifications"].([]interface{}), 0)
}

// TestNotificationAPI_MarkRead covers D3/D4/V6: single read, idempotent
// repeat, unknown id → 404.
func TestNotificationAPI_MarkRead(t *testing.T) {
	setupNotificationHandlerDB(t)

	notification.DailyReportSuccess(time.Now(), 1, 1)
	notification.DailyReportSuccess(time.Now(), 1, 1)
	rows, _, err := notification.List(false, 100, 0)
	require.NoError(t, err)
	id := rows[len(rows)-1].ID // oldest

	// D3：单条标已读
	rec := performRequest(MarkNotificationRead, http.MethodPost, "/", gin.Params{{Key: "id", Value: uintToString(id)}})
	body := rec.requireOK(t)
	require.Equal(t, true, dataOf(t, body)["is_read"])

	// D4：重复标已读 → no-op 不报错
	rec = performRequest(MarkNotificationRead, http.MethodPost, "/", gin.Params{{Key: "id", Value: uintToString(id)}})
	_ = rec.requireOK(t)

	// V6：越界引用 → 404，不误标他行
	rec = performRequest(MarkNotificationRead, http.MethodPost, "/", gin.Params{{Key: "id", Value: "99999"}})
	require.Equal(t, http.StatusNotFound, rec.recorder.Code)

	// 另一条仍未读
	rec = performRequest(GetUnreadCount, http.MethodGet, "/", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(1), dataOf(t, body)["unread"])
}

// TestNotificationAPI_MarkAllAndClear covers S1 步 7 + V9: mark-all sets all
// read, second call affects 0; clear-all empties the table.
func TestNotificationAPI_MarkAllAndClear(t *testing.T) {
	setupNotificationHandlerDB(t)

	notification.DailyReportSuccess(time.Now(), 1, 1)
	notification.DailyReportFailedSummary(time.Now(), 5, 1)

	// V9：全部标已读，第二次 0 行
	rec := performRequest(MarkAllNotificationsRead, http.MethodPost, "/", nil)
	body := rec.requireOK(t)
	require.Equal(t, float64(2), dataOf(t, body)["affected"])

	rec = performRequest(MarkAllNotificationsRead, http.MethodPost, "/", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(0), dataOf(t, body)["affected"])

	// 清空全部
	rec = performRequest(ClearNotifications, http.MethodDelete, "/", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(2), dataOf(t, body)["affected"])

	rec = performRequest(GetUnreadCount, http.MethodGet, "/", nil)
	body = rec.requireOK(t)
	require.Equal(t, float64(0), dataOf(t, body)["unread"])
}

// uintToString formats a notification id for the :id route param.
func uintToString(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
