package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/safefetch"
)

// ── 可用性检查 handler 契约（improve-discovery-recommendations 3.3，design D7 / spec C6）──
//
// 200（检查失败也是 200，状态与错误码在 data 里）、404 not_found、403 forbidden
// （未授权私有来源，零请求）、400 invalid id；候选视图携带 availability/last_checked_at。

const handlerTestRSSBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Handler Feed</title><link>https://example.com</link>
<description>d</description><item><title>i1</title><link>https://example.com/1</link></item></channel></rss>`

func newCandidateCheckGinContext(id string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	ctx.Params = gin.Params{{Key: "id", Value: id}}
	return ctx, recorder
}

// TestCandidateHandlerCheckSuccessAndViewAvailability 200：返回 availability/last_checked_at/
// next_check_at；候选列表与详情同步携带 status/last_checked_at（无记录 = unknown）。
func TestCandidateHandlerCheckSuccessAndViewAvailability(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)

	calls := 0
	restore := service.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*service.CandidateFetchResult, error) {
		calls++
		return &service.CandidateFetchResult{Result: &safefetch.Result{StatusCode: 200, Body: []byte(handlerTestRSSBody)}}, nil
	})
	defer restore()

	enabled := true
	url := "https://handler.example.com/feed"
	cand := models.FeedCandidate{
		StableKey: "rss:handler-check", Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Handler Check"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)

	// 检查前：视图 availability=unknown（未验证），last_checked_at 为 null。
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", cand.ID)}}
	GetCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	before := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Equal(t, "unknown", before["availability"])
	require.Nil(t, before["last_checked_at"])

	// 同步检查：200 + data 形状。
	ctx, recorder = newCandidateCheckGinContext(fmt.Sprintf("%d", cand.ID))
	CheckCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, calls)
	body := decodeCandidateBody(t, recorder)
	require.Equal(t, true, body["success"])
	data := body["data"].(map[string]any)
	require.Equal(t, "ok", data["availability"])
	require.NotNil(t, data["last_checked_at"])
	require.NotNil(t, data["next_check_at"])
	require.Equal(t, "", data["last_error_code"])
	require.NotContains(t, recorder.Body.String(), url, "检查响应不得回显完整端点地址")

	// 详情与列表都带上可用性状态。
	ctx, recorder = newCandidateCheckGinContext(fmt.Sprintf("%d", cand.ID))
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetCandidate(ctx)
	after := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Equal(t, "ok", after["availability"])
	require.NotNil(t, after["last_checked_at"])

	gin.SetMode(gin.TestMode)
	recorder = httptest.NewRecorder()
	listCtx, _ := gin.CreateTestContext(recorder)
	listCtx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ListCandidates(listCtx)
	require.Equal(t, http.StatusOK, recorder.Code)
	listData := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	items := listData["items"].([]any)
	require.Len(t, items, 1)
	item := items[0].(map[string]any)
	require.Equal(t, "ok", item["availability"])
	require.NotNil(t, item["last_checked_at"])
}

// TestCandidateHandlerCheckFailureIsNotAPIError：源检查失败仍 200，状态与错误码在 data。
func TestCandidateHandlerCheckFailureIsNotAPIError(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)
	restore := service.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*service.CandidateFetchResult, error) {
		return nil, fmt.Errorf("%w (after %s)", safefetch.ErrTimeout, safefetch.DefaultTimeout)
	})
	defer restore()

	enabled := true
	url := "https://handler.example.com/timeout"
	cand := models.FeedCandidate{
		StableKey: "rss:handler-timeout", Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Timeout"},
		RecommendationEnabled: &enabled, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)

	ctx, recorder := newCandidateCheckGinContext(fmt.Sprintf("%d", cand.ID))
	CheckCandidate(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	data := decodeCandidateBody(t, recorder)["data"].(map[string]any)
	require.Equal(t, "unknown", data["availability"])
	require.Equal(t, "network_error", data["last_error_code"])
}

// TestCandidateHandlerCheckForbiddenAndNotFound：未授权私有来源 403 forbidden（零请求）、
// 不存在候选 404 not_found、非法 id 400 validation。
func TestCandidateHandlerCheckForbiddenAndNotFound(t *testing.T) {
	db := setupCandidateCatalogHandlerTestDB(t)

	calls := 0
	restore := service.SetCandidateCheckFetcher(func(_ context.Context, _ string, _ safefetch.Options) (*service.CandidateFetchResult, error) {
		calls++
		return &service.CandidateFetchResult{Result: &safefetch.Result{StatusCode: 200, Body: []byte(handlerTestRSSBody)}}, nil
	})
	defer restore()

	enabled := true
	url := "https://10.0.0.9/private"
	privateCand := models.FeedCandidate{
		StableKey: "rss:handler-private", Kind: "rss", FeedURL: &url, CanonicalKey: url,
		ManualMetadata:        models.MetadataMap{service.ManualFieldName: "Private"},
		RecommendationEnabled: &enabled, AccessScope: "private_pending", Revision: 1,
	}
	require.NoError(t, db.Create(&privateCand).Error)

	ctx, recorder := newCandidateCheckGinContext(fmt.Sprintf("%d", privateCand.ID))
	CheckCandidate(ctx)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	body := decodeCandidateBody(t, recorder)
	require.Equal(t, false, body["success"])
	require.Equal(t, "forbidden", body["code"])
	require.Equal(t, 0, calls, "private_pending 不得发起任何请求")
	require.NotContains(t, recorder.Body.String(), "10.0.0.9", "错误不得回显私有地址")

	ctx, recorder = newCandidateCheckGinContext("9999")
	CheckCandidate(ctx)
	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, "not_found", decodeCandidateBody(t, recorder)["code"])

	ctx, recorder = newCandidateCheckGinContext("not-a-number")
	CheckCandidate(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &parsed))
	require.Equal(t, "validation", parsed["code"])
}
