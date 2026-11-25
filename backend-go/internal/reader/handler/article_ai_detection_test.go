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
	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/service"
)

// 用例来源：openspec/changes/add-ai-generated-article-detection
// tasks 4.7 + specs/ai-generated-article-detection「人工标记与人工结论优先」。

func putArticleUpdate(t *testing.T, articleID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "article_id", Value: fmt.Sprintf("%d", articleID)}}
	ctx.Request = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/articles/%d", articleID), bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateArticle(ctx)
	return recorder
}

func decodeArticleResponse(t *testing.T, recorder *httptest.ResponseRecorder) (bool, string, map[string]interface{}) {
	t.Helper()
	var body struct {
		Success bool                   `json:"success"`
		Error   string                 `json:"error"`
		Data    map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Success, body.Error, body.Data
}

func seedDetectionTestArticle(t *testing.T) models.Article {
	t.Helper()
	feed := models.Feed{Title: "Manual Mark Feed", URL: "https://example.com/manual-mark"}
	require.NoError(t, database.DB.Create(&feed).Error)
	article := models.Article{
		FeedID:    feed.ID,
		Title:     "待人工纠正的稿件",
		Link:      "https://example.com/manual-mark/article",
		CreatedAt: time.Now(),
	}
	require.NoError(t, database.DB.Create(&article).Error)
	return article
}

func reloadArticle(t *testing.T, articleID uint) models.Article {
	t.Helper()
	var article models.Article
	require.NoError(t, database.DB.Omit("tag_count", "relevance_score").First(&article, articleID).Error)
	return article
}

// TestUpdateArticle_ManualAIDetectionMark 覆盖 task 4.7 场景「人工标记写入来源=人工」：
// 状态与来源落到同一组判定字段，置信分按标记态取界值，判定时间被写入。
func TestUpdateArticle_ManualAIDetectionMark(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	article := seedDetectionTestArticle(t)

	recorder := putArticleUpdate(t, article.ID, `{"ai_detection_status":"suspect","ai_detection_source":"manual"}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	success, _, data := decodeArticleResponse(t, recorder)
	require.True(t, success)
	require.Equal(t, models.AIDetectionStatusSuspect, data["ai_detection_status"])
	require.Equal(t, models.AIDetectionSourceManual, data["ai_detection_source"])
	require.Equal(t, float64(1), data["ai_detection_score"])
	require.NotNil(t, data["ai_detection_at"])

	stored := reloadArticle(t, article.ID)
	require.Equal(t, models.AIDetectionStatusSuspect, stored.AIDetectionStatus)
	require.Equal(t, models.AIDetectionSourceManual, stored.AIDetectionSource)
	require.Equal(t, float64(1), stored.AIDetectionScore)
	require.NotNil(t, stored.AIDetectionAt)
	// 自动判定执行前的跳过判据（worker 用它判定「人工结论优先」）。
	require.True(t, service.HasManualAIDetection(&stored))

	// 人工取消（unknown）：展示态清空，不留上一轮结论的残留。
	recorder = putArticleUpdate(t, article.ID, `{"ai_detection_status":"unknown","ai_detection_source":"manual"}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	_, _, data = decodeArticleResponse(t, recorder)
	require.Equal(t, models.AIDetectionStatusUnknown, data["ai_detection_status"])
	require.Equal(t, float64(0), data["ai_detection_score"])
	require.Equal(t, "", data["ai_detection_source"])
	require.Nil(t, data["ai_detection_at"])

	stored = reloadArticle(t, article.ID)
	require.Equal(t, models.AIDetectionStatusUnknown, stored.AIDetectionStatus)
	require.Equal(t, float64(0), stored.AIDetectionScore)
	require.Empty(t, stored.AIDetectionSource)
	require.Nil(t, stored.AIDetectionAt)
	require.False(t, service.HasManualAIDetection(&stored))
}

// TestUpdateArticle_RejectsForgedDetectionSource 覆盖 task 4.7 的「source 强制
// manual，不接受客户端伪造 llm」与非法状态值：两条都 400 且不改数据库。
func TestUpdateArticle_RejectsForgedDetectionSource(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	article := seedDetectionTestArticle(t)

	for _, body := range []string{
		`{"ai_detection_status":"suspect","ai_detection_source":"llm"}`,
		`{"ai_detection_status":"definitely_ai"}`,
		`{"ai_detection_source":"manual"}`,
	} {
		recorder := putArticleUpdate(t, article.ID, body)
		require.Equal(t, http.StatusBadRequest, recorder.Code, "body=%s", body)
		success, errMsg, _ := decodeArticleResponse(t, recorder)
		require.False(t, success)
		require.NotEmpty(t, errMsg)
	}

	stored := reloadArticle(t, article.ID)
	require.Equal(t, models.AIDetectionStatusUnknown, models.NormalizeAIDetectionStatus(stored.AIDetectionStatus))
	require.Empty(t, stored.AIDetectionSource)
}

// TestUpdateArticle_UnknownAcceptsEmptySource 覆盖 review R-14：取消标记时把 source 传成
// 空串与省略同语义（旧实现一律 400，显式清空没有路径），都走「清空判定字段」分支；
// 空串 + 非 unknown 仍按非法来源拒绝（标记必须显式给 manual，或省略 source）。
func TestUpdateArticle_UnknownAcceptsEmptySource(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	article := seedDetectionTestArticle(t)

	marker := putArticleUpdate(t, article.ID, `{"ai_detection_status":"suspect","ai_detection_source":"manual"}`)
	require.Equal(t, http.StatusOK, marker.Code, marker.Body.String())

	recorder := putArticleUpdate(t, article.ID, `{"ai_detection_status":"unknown","ai_detection_source":""}`)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	_, _, data := decodeArticleResponse(t, recorder)
	require.Equal(t, models.AIDetectionStatusUnknown, data["ai_detection_status"])
	require.Equal(t, float64(0), data["ai_detection_score"])
	require.Equal(t, "", data["ai_detection_source"])
	require.Nil(t, data["ai_detection_at"])

	stored := reloadArticle(t, article.ID)
	require.Equal(t, models.AIDetectionStatusUnknown, models.NormalizeAIDetectionStatus(stored.AIDetectionStatus))
	require.Equal(t, float64(0), stored.AIDetectionScore)
	require.Empty(t, stored.AIDetectionSource)
	require.Nil(t, stored.AIDetectionAt)

	// 空串 + suspect：仍 400（旧口径不回退），且不得落库。
	rejected := putArticleUpdate(t, article.ID, `{"ai_detection_status":"suspect","ai_detection_source":""}`)
	require.Equal(t, http.StatusBadRequest, rejected.Code, rejected.Body.String())
	success, errMsg, _ := decodeArticleResponse(t, rejected)
	require.False(t, success)
	require.NotEmpty(t, errMsg)

	stored = reloadArticle(t, article.ID)
	require.Equal(t, models.AIDetectionStatusUnknown, models.NormalizeAIDetectionStatus(stored.AIDetectionStatus))
	require.Empty(t, stored.AIDetectionSource)
}

// TestRecheckAIDetection_EnqueuesForceJob 覆盖 task 4.7「重新判定入口可覆盖人工
// 结论」的入口侧：入队一条 manual_recheck 作业（判定执行时绕过人工结论跳过），
// 且不直接改写人工结论（结论由判定链路产出）。
func TestRecheckAIDetection_EnqueuesForceJob(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	require.NoError(t, database.DB.AutoMigrate(&models.DetectionJob{}))
	article := seedDetectionTestArticle(t)

	marker := putArticleUpdate(t, article.ID, `{"ai_detection_status":"suspect","ai_detection_source":"manual"}`)
	require.Equal(t, http.StatusOK, marker.Code, marker.Body.String())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "article_id", Value: fmt.Sprintf("%d", article.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/articles/%d/ai-detection/recheck", article.ID), http.NoBody)

	RecheckAIDetection(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			JobID     uint   `json:"job_id"`
			ArticleID uint   `json:"article_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.NotZero(t, body.Data.JobID)
	require.Equal(t, article.ID, body.Data.ArticleID)
	require.Equal(t, string(models.JobStatusPending), body.Data.Status)

	var job models.DetectionJob
	require.NoError(t, database.DB.First(&job, body.Data.JobID).Error)
	require.Equal(t, article.ID, job.ArticleID)
	require.Equal(t, repository.DetectionJobReasonManualRecheck, job.Reason)
	require.True(t, repository.IsForceDetectionReason(job.Reason), "force 语义由 reason 承载")

	// 入口只入队，不直接改人工结论。
	stored := reloadArticle(t, article.ID)
	require.Equal(t, models.AIDetectionStatusSuspect, stored.AIDetectionStatus)
	require.Equal(t, models.AIDetectionSourceManual, stored.AIDetectionSource)
}

// TestRecheckAIDetection_ArticleNotFound 覆盖入口的基本错误映射：不存在的文章 404。
func TestRecheckAIDetection_ArticleNotFound(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)
	require.NoError(t, database.DB.AutoMigrate(&models.DetectionJob{}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "article_id", Value: "987654"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/articles/987654/ai-detection/recheck", http.NoBody)

	RecheckAIDetection(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
}
