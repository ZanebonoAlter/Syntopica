package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

// backfillRecordingEmbedder is a handler-package fake for the auxiliary-label
// embedder — records every call so tests can assert zero-LLM-call noops.
type backfillRecordingEmbedder struct {
	calls []string
	modes []service.AuxiliaryLabelEmbeddingMode
}

func (e *backfillRecordingEmbedder) embed(ctx context.Context, input string, mode service.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	e.calls = append(e.calls, input)
	e.modes = append(e.modes, mode)
	vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
	return service.FloatsToPgVector(vec), vec, nil
}

func setupAuxLabelBackfillRouter(t *testing.T) (*gorm.DB, *gin.Engine, *backfillRecordingEmbedder) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	embedder := &backfillRecordingEmbedder{}
	auxLabelServiceEmbedder = embedder.embed
	t.Cleanup(func() { auxLabelServiceEmbedder = nil })

	g := gin.New()
	api := g.Group("/api")
	RegisterSemanticBoardRoutes(api)
	return db, g, embedder
}

func postBackfill(t *testing.T, router *gin.Engine) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auxiliary-labels/backfill-embeddings", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusAccepted, recorder.Code, "endpoint must return 202 immediately")
	return decodeResponseBody(t, recorder)
}

// 主链路步7（handler 半）：健康库 → 202 {"pending":0}，零 LLM 调用、零写入。
func TestAuxLabelBackfillEndpointNoopHealthy(t *testing.T) {
	db, router, embedder := setupAuxLabelBackfillRouter(t)
	vec := service.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	healthy := models.SemanticLabel{
		Label: "OpenAI", Slug: "openai", LabelType: "auxiliary", Status: "active",
		Embedding: &vec, MergeEmbedding: &vec,
	}
	require.NoError(t, db.Create(&healthy).Error)

	body := postBackfill(t, router)

	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response envelope must carry data.pending")
	require.Equal(t, float64(0), data["pending"])
	require.Empty(t, embedder.calls, "healthy DB must trigger zero LLM calls (no goroutine spawned)")
}

// 主链路步8：死向量行 N 条 → 202 {"pending":N}，goroutine 异步补齐不阻塞响应。
func TestAuxLabelBackfillEndpointPendingCountAndAsyncFill(t *testing.T) {
	db, router, _ := setupAuxLabelBackfillRouter(t)
	vec := service.FloatsToPgVector(testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim))
	deads := []models.SemanticLabel{
		{Label: "高市早苗", Slug: "dead-1", LabelType: "auxiliary", Status: "active"},
		{Label: "日本央行", Slug: "dead-2", LabelType: "auxiliary", Status: "active"},
		{Label: "OpenAI", Slug: "dead-3", LabelType: "auxiliary", Status: "active"},
	}
	for i := range deads {
		require.NoError(t, db.Create(&deads[i]).Error)
	}
	healthy := models.SemanticLabel{
		Label: "特朗普", Slug: "healthy", LabelType: "auxiliary", Status: "active",
		Embedding: &vec, MergeEmbedding: &vec,
	}
	require.NoError(t, db.Create(&healthy).Error)

	body := postBackfill(t, router)

	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response envelope must carry data.pending")
	require.Equal(t, float64(3), data["pending"], "pending must equal dead-vector row count")

	// detached goroutine 异步补齐：Eventually 等待三行向量全部恢复（响应先于回填完成返回）
	require.Eventually(t, func() bool {
		var nullCount int64
		if err := db.Model(&models.SemanticLabel{}).
			Where("label_type = ? AND status = ?", "auxiliary", "active").
			Where("embedding IS NULL OR merge_embedding IS NULL").
			Count(&nullCount).Error; err != nil {
			return false
		}
		return nullCount == 0
	}, 5*time.Second, 50*time.Millisecond, "async backfill should rebuild all dead-vector rows")
}
