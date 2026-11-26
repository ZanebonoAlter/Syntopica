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

// 用例来源：board-health-triage-advice test-cases 主链路 A4 —— triage 随快照
// 透传；旧快照（metrics 无 triage 字段）按旧形态返回不报错。
func TestGetBoardHealthCheckupTriagePassthrough(t *testing.T) {
	db, r := setupBoardHealthCheckupTest(t)
	board := models.SemanticLabel{Label: "分诊透传板", Slug: "bh-handler-triage", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)

	// 新形态快照：metrics 携带 triage + 信号字段。
	score := 66.0
	newShape := tagmodels.BoardHealthMetrics{
		Board: tagmodels.BoardHealthBoardMetrics{
			MatchedTags: 1, ReasonDist: map[string]int{}, GhostMatches: 1,
			MagnetFlow: &tagmodels.BoardHealthMagnetFlow{FailedTags: 5, FlowTags: 0},
		},
		Labels: []tagmodels.BoardHealthLabelMetrics{{
			AuxID: 42, Label: "上海", Description: "上海知名商业综合体",
			LabelType: "auxiliary",
			Magnet:    &tagmodels.BoardHealthMagnetSignal{AllSamples: 400, FailedSamples: 40, Suspicious: true},
			Neighbors: []tagmodels.BoardHealthNeighbor{{AuxID: 9, Label: "举办地点", Cosine: 0.796}},
			DescFlags: []string{"此次"},
		}, {
			// 组合标签也进构成清单：label_type 必须逐条透出（前端据此标不可执行）。
			AuxID: 43, Label: "上海 × 展会", LabelType: "composite",
		}},
		Triage: []tagmodels.BoardHealthTriageItem{{
			AuxID: 42, Category: "rewrite", Evidence: "desc 预筛命中此次", Suggestion: "中国东部直辖市",
		}},
	}
	require.NoError(t, db.Create(&tagmodels.BoardHealthSnapshot{
		SemanticBoardID: board.ID, CheckedAt: time.Date(2026, 10, 29, 8, 0, 0, 0, time.Local),
		Metrics: newShape, UncapturedCandidates: []tagmodels.BoardHealthCandidate{}, LLMScore: &score,
	}).Error)

	// 旧形态快照：2026-10 之前的历史 jsonb 无 triage/信号字段（原样 SQL 植入）。
	require.NoError(t, db.Exec(`INSERT INTO board_health_snapshots
		(semantic_board_id, checked_at, metrics, uncaptured_candidates, llm_score, llm_comment, created_at)
		VALUES (?, ?, ?, '[]', 55, '旧评语', ?)`,
		board.ID, time.Date(2026, 9, 1, 8, 0, 0, 0, time.Local),
		`{"board":{"matched_tags":3,"reason_dist":{"direct_hit":3},"ghost_matches":0,"vector_missing":0},"labels":[{"aux_id":7,"label":"旧标签","samples":3,"cosine_mean":null,"cosine_median":null,"cosine_p25":null,"cosine_p75":null,"low_sim_ratio":null,"insufficient":true}]}`, time.Date(2026, 9, 1, 7, 0, 0, 0, time.Local)).Error)

	w, body := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup", board.ID))
	require.Equal(t, http.StatusOK, w.Code)
	data := body["data"].(map[string]any)
	latest := data["latest"].(map[string]any)

	// 新形态：triage 数组原样透出（category/evidence/suggestion）。
	metrics := latest["metrics"].(map[string]any)
	triage := metrics["triage"].([]any)
	require.Len(t, triage, 1)
	item := triage[0].(map[string]any)
	require.Equal(t, float64(42), item["aux_id"])
	require.Equal(t, "rewrite", item["category"])
	require.Equal(t, "desc 预筛命中此次", item["evidence"])
	require.Equal(t, "中国东部直辖市", item["suggestion"])
	// 信号字段随 labels 透出。
	labels := metrics["labels"].([]any)
	label := labels[0].(map[string]any)
	require.Equal(t, "上海知名商业综合体", label["description"])
	require.NotNil(t, label["magnet"])
	require.Equal(t, []any{"此次"}, label["desc_flags"])
	// 4.1 M1：label_type 逐条透出（auxiliary / composite）——前端据此把组合标签的
	// 「重写描述」标为不可执行（描述修复链只认 auxiliary）。
	require.Equal(t, "auxiliary", label["label_type"])
	require.Equal(t, "composite", labels[1].(map[string]any)["label_type"])
	flow := metrics["board"].(map[string]any)["magnet_flow"]
	require.NotNil(t, flow)
	require.Nil(t, flow.(map[string]any)["top_board"], "无可靠流向显式未知")

	// 历史快照（trend 第二点）不受影响；latest 缺 triage 字段时为 null。
	w2, body2 := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup?trend=2", board.ID))
	require.Equal(t, http.StatusOK, w2.Code)
	trend := body2["data"].(map[string]any)["trend"].([]any)
	require.Len(t, trend, 2)

	// 历史快照成为 latest 时：无 triage 字段 → 反序列化为零值 → JSON null，
	// 统计与评分正常返回（spec「历史快照缺分诊字段兼容」）。
	histBoard := models.SemanticLabel{Label: "旧板", Slug: "bh-handler-hist", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&histBoard).Error)
	require.NoError(t, db.Exec(`INSERT INTO board_health_snapshots
		(semantic_board_id, checked_at, metrics, uncaptured_candidates, llm_score, llm_comment, created_at)
		VALUES (?, ?, ?, '[]', 55, '旧评语', ?)`,
		histBoard.ID, time.Date(2026, 9, 1, 8, 0, 0, 0, time.Local),
		`{"board":{"matched_tags":3,"reason_dist":{"direct_hit":3},"ghost_matches":0,"vector_missing":0},"labels":[{"aux_id":7,"label":"旧标签","samples":3,"cosine_mean":null,"cosine_median":null,"cosine_p25":null,"cosine_p75":null,"low_sim_ratio":null,"insufficient":true}]}`, time.Date(2026, 9, 1, 7, 0, 0, 0, time.Local)).Error)
	_, body3 := getHealthCheckup(r, fmt.Sprintf("/api/semantic-boards/%d/health-checkup", histBoard.ID))
	histLatest := body3["data"].(map[string]any)["latest"].(map[string]any)
	require.InDelta(t, 55.0, histLatest["llm_score"].(float64), 1e-9, "旧评分正常返回")
	histMetrics := histLatest["metrics"].(map[string]any)
	require.Nil(t, histMetrics["triage"], "缺 triage 字段 → null，不报错")
	histLabel := histMetrics["labels"].([]any)[0].(map[string]any)
	require.Nil(t, histLabel["magnet"], "缺信号字段 → null")
}
