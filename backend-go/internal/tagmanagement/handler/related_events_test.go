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
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service/core"
	"syntopica-backend/internal/tagmanagement/service/related"
)

// 用例来源：openspec/changes/enable-story-cross-day-continuation/test-cases.md
// 故事 S1（端点契约：排序/来源/覆盖态/404）与 S2（阈值保存、非法拒绝、整体不部分落库）。
// 端点：GET /api/semantic-boards/:id/tags/:tagId/related-events 与既有
// GET/PUT /api/semantic-boards/matching-config。SQL/向量路径走 testutil 临时 PG。

func setupRelatedEventsHandlerTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	vendor := models.AIVendor{Name: "stub-vendor", ProviderType: "openai_compatible", Enabled: true}
	require.NoError(t, db.Create(&vendor).Error)
	model := models.AIModel{VendorID: vendor.ID, Name: "stub-model", ModelKind: "embedding", Enabled: true, Status: "ok"}
	require.NoError(t, db.Create(&model).Error)
	route := models.AIRoute{Name: "default", Capability: "embedding", Enabled: true}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, db.Create(&models.AIRouteModel{RouteID: route.ID, ModelID: model.ID, Priority: 1, Enabled: true}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterSemanticBoardRoutes(api)
	return db, r
}

func seedRelatedBoard(t *testing.T, db *gorm.DB, label string) *models.SemanticLabel {
	t.Helper()
	board := &models.SemanticLabel{
		Label:     label,
		Slug:      fmt.Sprintf("related-handler-board-%d", time.Now().UnixNano()),
		LabelType: "board",
		Status:    "active",
	}
	require.NoError(t, db.Create(board).Error)
	return board
}

func seedRelatedTag(t *testing.T, db *gorm.DB, label string) *models.TopicTag {
	t.Helper()
	tag := &models.TopicTag{
		Label:       label,
		Description: label + " 的描述",
		Slug:        fmt.Sprintf("related-handler-tag-%d-%d", time.Now().UnixNano(), len(label)),
		Category:    models.TagCategoryEvent,
		Status:      "active",
		IsCanonical: true,
		CreatedAt:   time.Now().UTC().Add(-time.Hour),
	}
	require.NoError(t, db.Create(tag).Error)
	return tag
}

func seedRelatedMount(t *testing.T, db *gorm.DB, boardID, tagID uint) {
	t.Helper()
	require.NoError(t, db.Create(&tagmodels.TopicTagBoardLabel{
		TopicTagID:      tagID,
		SemanticBoardID: boardID,
		Score:           0.9,
		MatchReason:     "direct_hit",
	}).Error)
}

func seedRelatedVector(t *testing.T, db *gorm.DB, tag *models.TopicTag, vector []float64) {
	t.Helper()
	space, err := related.ResolveSpace(t.Context(), db, related.DefaultGeneration)
	require.NoError(t, err)
	text, err := related.BuildRepresentation(tag.Label, tag.Description, nil)
	require.NoError(t, err)
	require.NoError(t, db.Create(&tagmodels.RelatedEventEmbedding{
		TopicTagID:            tag.ID,
		RepresentationVersion: related.RepresentationVersion,
		SpaceKey:              space.Key(),
		TextHash:              related.HashText(text),
		Dimension:             len(vector),
		EmbeddingVec:          core.FloatsToPgVector(vector),
	}).Error)
}

func doJSON(t *testing.T, r *gin.Engine, method, url string, payload any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var bodyReader *bytes.Reader
	if payload == nil {
		bodyReader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(raw)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, url, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestRelatedEventsEndpoint(t *testing.T) {
	db, r := setupRelatedEventsHandlerTest(t)
	board := seedRelatedBoard(t, db, "AI 版块")

	source := seedRelatedTag(t, db, "源事件")
	near := seedRelatedTag(t, db, "近邻事件")
	far := seedRelatedTag(t, db, "无关事件")
	for _, tag := range []*models.TopicTag{source, near, far} {
		seedRelatedMount(t, db, board.ID, tag.ID)
	}

	feed := &models.Feed{Title: "来源", URL: fmt.Sprintf("https://example.com/%d", time.Now().UnixNano())}
	require.NoError(t, db.Create(feed).Error)
	pub := time.Now().UTC().Add(-2 * time.Hour)
	article := &models.Article{FeedID: feed.ID, Title: "近邻事件的报道", PubDate: &pub}
	require.NoError(t, db.Create(article).Error)
	require.NoError(t, db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: near.ID}).Error)

	seedRelatedVector(t, db, source, []float64{1, 0})
	seedRelatedVector(t, db, near, []float64{0.95, 0.3122498999199199})
	seedRelatedVector(t, db, far, []float64{0, 1})

	t.Run("返回排序、来源、覆盖数量与实际阈值", func(t *testing.T) {
		w, body := doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/tags/%d/related-events", board.ID, source.ID), nil)
		require.Equal(t, http.StatusOK, w.Code)
		require.True(t, body["success"].(bool))
		data := body["data"].(map[string]any)
		require.Equal(t, related.StatusReady, data["status"])
		require.InDelta(t, related.DefaultThreshold, data["threshold"].(float64), 1e-9)
		require.Equal(t, float64(related.WindowDays), data["window_days"])
		require.Equal(t, related.RepresentationVersion, data["representation_version"])
		require.Equal(t, float64(2), data["candidate_count"])
		require.Equal(t, float64(2), data["indexed_count"])
		require.NotEmpty(t, data["window_start"])
		require.NotEmpty(t, data["window_end"])

		items := data["items"].([]any)
		require.Len(t, items, 1, "只有达阈值的近邻返回")
		item := items[0].(map[string]any)
		require.Equal(t, float64(near.ID), item["tag_id"])
		require.Equal(t, near.Label, item["label"])
		require.Equal(t, near.Description, item["description"])
		require.InDelta(t, 0.95, item["similarity"].(float64), 1e-3)
		articleDTO := item["source_article"].(map[string]any)
		require.Equal(t, float64(article.ID), articleDTO["id"])
		require.Equal(t, article.Title, articleDTO["title"])
	})

	t.Run("缺索引如实报 not_ready，不伪装成空集", func(t *testing.T) {
		unindexed := seedRelatedTag(t, db, "未索引事件")
		seedRelatedMount(t, db, board.ID, unindexed.ID)
		w, body := doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/tags/%d/related-events", board.ID, unindexed.ID), nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]any)
		require.Equal(t, related.StatusNotReady, data["status"])
		require.Equal(t, related.ReasonSourceNotIndexed, data["reason"])
		require.Empty(t, data["items"].([]any))
	})

	t.Run("版块不存在 / 标签不属本版块 → 404", func(t *testing.T) {
		w, _ := doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/semantic-boards/999999/tags/%d/related-events", source.ID), nil)
		require.Equal(t, http.StatusNotFound, w.Code)

		otherBoard := seedRelatedBoard(t, db, "别的版块")
		w, _ = doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/tags/%d/related-events", otherBoard.ID, source.ID), nil)
		require.Equal(t, http.StatusNotFound, w.Code)

		w, _ = doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/tags/999999/related-events", board.ID), nil)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("非法参数 → 400", func(t *testing.T) {
		w, _ := doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/semantic-boards/%d/tags/abc/related-events", board.ID), nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestMatchingConfigRelatedEventThreshold(t *testing.T) {
	db, r := setupRelatedEventsHandlerTest(t)

	t.Run("缺项返回默认 0.86", func(t *testing.T) {
		w, body := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]any)
		require.InDelta(t, related.DefaultThreshold, data[related.ThresholdKey].(float64), 1e-9)
	})

	t.Run("合法保存后重读生效，其他匹配参数不变", func(t *testing.T) {
		_, before := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
		beforeData := before["data"].(map[string]any)

		w, _ := doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{
			related.ThresholdKey: 0.9,
		})
		require.Equal(t, http.StatusOK, w.Code)

		_, after := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
		afterData := after["data"].(map[string]any)
		require.InDelta(t, 0.9, afterData[related.ThresholdKey].(float64), 1e-9)
		require.Equal(t, beforeData["semantic_board_match_sim_threshold"], afterData["semantic_board_match_sim_threshold"])
	})

	t.Run("边界 0 与 1 合法", func(t *testing.T) {
		for _, value := range []float64{0, 1} {
			w, _ := doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{related.ThresholdKey: value})
			require.Equal(t, http.StatusOK, w.Code)
			_, body := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
			require.InDelta(t, value, body["data"].(map[string]any)[related.ThresholdKey].(float64), 1e-9)
		}
	})

	t.Run("非法值拒绝且不覆盖原值", func(t *testing.T) {
		require.NoError(t, db.Where("key = ?", related.ThresholdKey).Delete(&models.AISettings{}).Error)
		_, _ = doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{related.ThresholdKey: 0.75})

		cases := []struct {
			name    string
			payload map[string]any
		}{
			{name: "空白", payload: map[string]any{related.ThresholdKey: "  "}},
			{name: "范围外", payload: map[string]any{related.ThresholdKey: 1.5}},
			{name: "负数", payload: map[string]any{related.ThresholdKey: -0.1}},
			{name: "NaN 字符串", payload: map[string]any{related.ThresholdKey: "NaN"}},
			{name: "Infinity 字符串", payload: map[string]any{related.ThresholdKey: "Inf"}},
			{name: "非数字", payload: map[string]any{related.ThresholdKey: "abc"}},
		}
		for _, tc := range cases {
			w, body := doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", tc.payload)
			require.Equal(t, http.StatusBadRequest, w.Code, tc.name)
			require.False(t, body["success"].(bool), tc.name)
			_, cfg := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
			require.InDelta(t, 0.75, cfg["data"].(map[string]any)[related.ThresholdKey].(float64), 1e-9, tc.name+" 不得覆盖原值")
		}
	})

	t.Run("多字段 PUT 整体失败不部分落库", func(t *testing.T) {
		_, cfgBefore := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
		before := cfgBefore["data"].(map[string]any)

		w, _ := doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{
			related.ThresholdKey:                 0.5,
			"semantic_board_match_sim_threshold": 1.7,
		})
		require.Equal(t, http.StatusBadRequest, w.Code)

		_, cfgAfter := doJSON(t, r, http.MethodGet, "/api/semantic-boards/matching-config", nil)
		after := cfgAfter["data"].(map[string]any)
		require.Equal(t, before[related.ThresholdKey], after[related.ThresholdKey])
		require.Equal(t, before["semantic_board_match_sim_threshold"], after["semantic_board_match_sim_threshold"])
	})

	t.Run("白名单外键拒绝", func(t *testing.T) {
		w, _ := doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{"related_event_unknown": 0.5})
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestRelatedEventsHonoursUpdatedThreshold(t *testing.T) {
	db, r := setupRelatedEventsHandlerTest(t)
	board := seedRelatedBoard(t, db, "AI 版块")
	source := seedRelatedTag(t, db, "源事件")
	near := seedRelatedTag(t, db, "近邻事件")
	seedRelatedMount(t, db, board.ID, source.ID)
	seedRelatedMount(t, db, board.ID, near.ID)
	seedRelatedVector(t, db, source, []float64{1, 0})
	seedRelatedVector(t, db, near, []float64{0.9, 0.4358898943540674})

	url := fmt.Sprintf("/api/semantic-boards/%d/tags/%d/related-events", board.ID, source.ID)

	_, body := doJSON(t, r, http.MethodGet, url, nil)
	require.Len(t, body["data"].(map[string]any)["items"].([]any), 1, "默认 0.86 下近邻（0.90）达标")

	// 调高阈值到 0.95：同一缓存（不重算向量）下结果收紧，返回实际阈值。
	w, _ := doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{related.ThresholdKey: 0.95})
	require.Equal(t, http.StatusOK, w.Code)
	_, body = doJSON(t, r, http.MethodGet, url, nil)
	data := body["data"].(map[string]any)
	require.InDelta(t, 0.95, data["threshold"].(float64), 1e-9)
	require.Empty(t, data["items"].([]any))

	// 调低到 0.5：重新达标。
	_, _ = doJSON(t, r, http.MethodPut, "/api/semantic-boards/matching-config", map[string]any{related.ThresholdKey: 0.5})
	_, body = doJSON(t, r, http.MethodGet, url, nil)
	data = body["data"].(map[string]any)
	require.InDelta(t, 0.5, data["threshold"].(float64), 1e-9)
	require.Len(t, data["items"].([]any), 1)
}
