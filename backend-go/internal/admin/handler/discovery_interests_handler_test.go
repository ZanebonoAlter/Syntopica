package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/admin/service"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// ── 兴趣记录列表 handler 契约（improve-discovery-recommendations High 1 修复）──
//
// GET /api/discovery/interests：200 + data 数组（前端 getInterests 直接 map），
// 分页元信息在顶层 pagination；board_label 来自 semantic_labels（NULL 板 = null），
// status 展示态 active|faded|legacy；created_at 降序。

func setupInterestHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.DiscoveryInterestEntry{}, &models.SemanticLabel{}, &models.DiscoveryRun{},
	))
	database.DB = db
	repository.InitRepository(db)
	return db
}

func decodeInterestBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

func TestInterestsHandlerListShapeAndOrder(t *testing.T) {
	db := setupInterestHandlerTestDB(t)
	board := models.SemanticLabel{Label: "体育", Slug: "s-h1", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)

	unmatched := models.DiscoveryInterestEntry{QueryText: "冷门主题", Status: "active"}
	require.NoError(t, db.Create(&unmatched).Error)
	matched := models.DiscoveryInterestEntry{QueryText: "足球战术", BoardID: &board.ID, Status: "active"}
	require.NoError(t, db.Create(&matched).Error)
	faded := models.DiscoveryInterestEntry{QueryText: "旧兴趣", Status: "inactive"}
	require.NoError(t, db.Create(&faded).Error)
	legacy := models.DiscoveryInterestEntry{QueryText: "迁移旧种子", Status: "legacy_inactive"}
	require.NoError(t, db.Create(&legacy).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/discovery/interests", nil)
	GetInterests(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeInterestBody(t, recorder)
	require.Equal(t, true, body["success"])
	data := body["data"].([]any)
	require.Len(t, data, 4, "全量 4 条（默认 page_size 30 内）")
	page := body["pagination"].(map[string]any)
	require.EqualValues(t, 1, page["page"])
	require.EqualValues(t, service.InterestListDefaultPageSize, page["per_page"])
	require.EqualValues(t, 4, page["total"])

	// created_at 降序：最后插入的 legacy 在最前。
	first := data[0].(map[string]any)
	require.Equal(t, "legacy", first["status"], "legacy_inactive → 展示态 legacy")
	require.Equal(t, "迁移旧种子", first["query_text"])

	byQuery := map[string]map[string]any{}
	for _, item := range data {
		m := item.(map[string]any)
		byQuery[m["query_text"].(string)] = m
	}
	require.Equal(t, "active", byQuery["足球战术"]["status"])
	require.Equal(t, float64(board.ID), byQuery["足球战术"]["board_id"])
	require.Equal(t, "体育", byQuery["足球战术"]["board_label"])
	require.Nil(t, byQuery["冷门主题"]["board_id"], "未匹配版块 board_id 为 null")
	require.Nil(t, byQuery["冷门主题"]["board_label"], "未匹配版块 board_label 为 null")
	require.Equal(t, "faded", byQuery["旧兴趣"]["status"], "inactive → 展示态 faded")
}

func TestInterestsHandlerPaginationCapsPageSize(t *testing.T) {
	db := setupInterestHandlerTestDB(t)
	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&models.DiscoveryInterestEntry{
			QueryText: fmt.Sprintf("q%d", i), Status: "active",
		}).Error)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/discovery/interests?page=2&page_size=500", nil)
	GetInterests(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := decodeInterestBody(t, recorder)
	page := body["pagination"].(map[string]any)
	require.EqualValues(t, service.InterestListMaxPageSize, page["per_page"], "page_size 上限 100")
	data := body["data"].([]any)
	require.Len(t, data, 0, "page_size 被截到 100，page=2 offset=100 越过全量 5 条 → 空页")
}
