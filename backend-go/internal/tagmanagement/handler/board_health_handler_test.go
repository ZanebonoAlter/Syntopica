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
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

// 用例来源：openspec/changes/add-board-health-checkup/test-cases.md 主链路步
// 13/14（查询最新快照+趋势 / 空态 / 404）+ 变体「输入」（trend 白名单外 400）。
// 端点：GET /api/semantic-boards/:id/health-checkup?trend=N。轻量端点契约测试
// 用内存 SQLite（无 pgvector 依赖），走完整 RegisterSemanticBoardRoutes 注册路径。

func setupBoardHealthCheckupTest(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:board_health_checkup_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SemanticLabel{},
		&tagmodels.BoardHealthSnapshot{},
	))
	repository.InitRepository(db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	RegisterSemanticBoardRoutes(api)
	return db, r
}

func getHealthCheckup(r *gin.Engine, url string) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body
}

func TestGetBoardHealthCheckup(t *testing.T) {
	db, r := setupBoardHealthCheckupTest(t)

	// 活跃版块 + 3 条快照（升序分数 51/52/53）。
	board := models.SemanticLabel{Label: "AI前沿", Slug: "bh-handler-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)
	base := time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		score := 51.0 + float64(i)
		require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
			SemanticBoardID: board.ID, CheckedAt: base.AddDate(0, 0, i),
			Metrics:              tagmodels.BoardHealthMetrics{Board: tagmodels.BoardHealthBoardMetrics{ReasonDist: map[string]int{}}, Labels: []tagmodels.BoardHealthLabelMetrics{}},
			UncapturedCandidates: []tagmodels.BoardHealthCandidate{},
			LLMScore:             &score,
		}).Error)
	}

	t.Run("正常：latest + 趋势升序", func(t *testing.T) {
		w, body := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup", board.ID))
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]any)
		latest := data["latest"].(map[string]any)
		require.InDelta(t, 53.0, latest["llm_score"].(float64), 1e-9)
		trend := data["trend"].([]any)
		require.Len(t, trend, 3)
		first := trend[0].(map[string]any)
		last := trend[2].(map[string]any)
		require.Less(t, first["checked_at"].(string), last["checked_at"].(string), "ascending")
		require.InDelta(t, 51.0, first["llm_score"].(float64), 1e-9)
	})

	t.Run("trend=2 截最近两条", func(t *testing.T) {
		_, body := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup?trend=2", board.ID))
		trend := body["data"].(map[string]any)["trend"].([]any)
		require.Len(t, trend, 2)
		first := trend[0].(map[string]any)
		require.InDelta(t, 52.0, first["llm_score"].(float64), 1e-9, "newest N keeps 52/53")
	})

	t.Run("无快照返回空态", func(t *testing.T) {
		fresh := models.SemanticLabel{Label: "新能源", Slug: "bh-handler-fresh", LabelType: "board", Status: "active"}
		require.NoError(t, db.Create(&fresh).Error)
		w, body := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup", fresh.ID))
		require.Equal(t, http.StatusOK, w.Code, "empty state is not an error")
		data := body["data"].(map[string]any)
		require.Nil(t, data["latest"])
		require.Empty(t, data["trend"])
	})

	t.Run("disabled 版块返回空态非 404", func(t *testing.T) {
		disabled := models.SemanticLabel{Label: "停用", Slug: "bh-handler-disabled", LabelType: "board", Status: "disabled"}
		require.NoError(t, db.Create(&disabled).Error)
		w, body := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup", disabled.ID))
		require.Equal(t, http.StatusOK, w.Code)
		data := body["data"].(map[string]any)
		require.Nil(t, data["latest"])
		require.Empty(t, data["trend"])
	})

	t.Run("版块不存在 404", func(t *testing.T) {
		w, _ := getHealthCheckup(r, "/api/semantic-boards/999999/health-checkup")
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("trend 白名单外 400（0/91/负数/非整数）", func(t *testing.T) {
		for _, raw := range []string{"0", "91", "-3", "abc", "1.5"} {
			w, _ := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup?trend=%s", board.ID, raw))
			require.Equal(t, http.StatusBadRequest, w.Code, "trend=%s must be rejected", raw)
		}
	})

	t.Run("trend 边界 1 与 90 合法", func(t *testing.T) {
		for _, raw := range []string{"1", "90"} {
			w, _ := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup?trend=%s", board.ID, raw))
			require.Equal(t, http.StatusOK, w.Code, "trend=%s is inside the whitelist", raw)
		}
	})

	t.Run("端点只读：不产生新快照", func(t *testing.T) {
		var before, after int64
		require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&before).Error)
		getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup", board.ID))
		getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup?trend=90", board.ID))
		require.NoError(t, db.Model(&tagmodels.BoardHealthSnapshot{}).Count(&after).Error)
		require.Equal(t, before, after, "read-only endpoint must not write snapshots")
	})
}
