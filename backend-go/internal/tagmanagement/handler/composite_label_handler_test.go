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
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

// setupCompositeRouter wires a gin engine with all semantic-board + composite
// routes over a fresh testcontainer DB, stubbing the composite embedder seam
// (it is read at registration time, so it must be set before Register*Routes).
func setupCompositeRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	g := gin.New()
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	compositeLabelEmbedder = func(ctx context.Context, input string, mode service.AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
		vec := testutil.PadVector([]float64{1, 0, 0}, testutil.TestEmbeddingDim)
		return service.FloatsToPgVector(vec), vec, nil
	}
	t.Cleanup(func() {
		compositeLabelEmbedder = service.DefaultAuxiliaryLabelEmbedder
	})

	api := g.Group("/api")
	RegisterSemanticBoardRoutes(api)
	return db, g
}

type compositeCreateBody struct {
	ID      uint   `json:"id"`
	Label   string `json:"label"`
	Outcome string `json:"outcome"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

type compositeListComponent struct {
	Label    string `json:"label"`
	Position int    `json:"position"`
}

type compositeListBody struct {
	Total int `json:"total"`
	Items []struct {
		Label      string                   `json:"label"`
		Status     string                   `json:"status"`
		RefCount   int                      `json:"ref_count"`
		Components []compositeListComponent `json:"components"`
	} `json:"items"`
}

func decodeCompositeCreateBody(t *testing.T, raw string) compositeCreateBody {
	t.Helper()
	var envelope struct {
		Data compositeCreateBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &envelope))
	return envelope.Data
}

func decodeCompositeListBody(t *testing.T, rec *httptest.ResponseRecorder) compositeListBody {
	t.Helper()
	var envelope struct {
		Data compositeListBody `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func TestCompositeLabelHandlerCreateListAndDedupe(t *testing.T) {
	db, router := setupCompositeRouter(t)

	auxA := createHandlerSemanticLabel(t, db, "美国国债", "clh-us-treasury", "auxiliary", "active", 8, []float64{1, 0, 0})
	auxB := createHandlerSemanticLabel(t, db, "收益率", "clh-yield", "auxiliary", "active", 5, []float64{0, 1, 0})

	// 1. Create — happy path.
	resp := performJSON(t, router, http.MethodPost, "/api/composite-labels", map[string]any{
		"label":               "美债收益率",
		"description":         "美国国债与收益率的组合",
		"component_label_ids": []uint{auxA.ID, auxB.ID},
	})
	require.Equal(t, http.StatusOK, resp.Code)
	body := decodeCompositeCreateBody(t, resp.Body.String())
	require.Equal(t, "created", body.Outcome)
	require.Equal(t, "manual", body.Source)
	require.Equal(t, "组合标签已创建", body.Message)

	// 2. List shows ordered components.
	listResp := performJSON(t, router, http.MethodGet, "/api/composite-labels", nil)
	require.Equal(t, http.StatusOK, listResp.Code)
	list := decodeCompositeListBody(t, listResp)
	require.Equal(t, 1, list.Total)
	require.Equal(t, "美债收益率", list.Items[0].Label)
	require.Len(t, list.Items[0].Components, 2)
	require.Equal(t, "美国国债", list.Items[0].Components[0].Label)
	require.Equal(t, 1, list.Items[0].Components[0].Position)
	require.Equal(t, "收益率", list.Items[0].Components[1].Label)
	require.Equal(t, 2, list.Items[0].Components[1].Position)

	// 3. Dedup reuse — same component set returns the existing composite, not an error.
	resp2 := performJSON(t, router, http.MethodPost, "/api/composite-labels", map[string]any{
		"label":               "美国国债收益率",
		"component_label_ids": []uint{auxB.ID, auxA.ID},
	})
	require.Equal(t, http.StatusOK, resp2.Code)
	body2 := decodeCompositeCreateBody(t, resp2.Body.String())
	require.Equal(t, "reused_l1", body2.Outcome)
	require.Equal(t, body.ID, body2.ID, "dedupe hit must return the existing composite id")
	require.Contains(t, body2.Message, "复用")

	// 4. Component-count validation → 400.
	bad := performJSON(t, router, http.MethodPost, "/api/composite-labels", map[string]any{
		"label":               "单个组件",
		"component_label_ids": []uint{auxA.ID},
	})
	require.Equal(t, http.StatusBadRequest, bad.Code)

	// 5. Non-auxiliary component → 400 (board id refused by service validation).
	board := createHandlerSemanticLabel(t, db, "AI Board", "clh-board", "board", "active", 0, nil)
	badType := performJSON(t, router, http.MethodPost, "/api/composite-labels", map[string]any{
		"label":               "类型错误",
		"component_label_ids": []uint{auxA.ID, board.ID},
	})
	require.Equal(t, http.StatusBadRequest, badType.Code)
}

func TestCompositeLabelHandlerDisableEnableAndNotFound(t *testing.T) {
	db, router := setupCompositeRouter(t)

	auxA := createHandlerSemanticLabel(t, db, "中国", "clh-cn", "auxiliary", "active", 8, []float64{1, 0, 0})
	auxB := createHandlerSemanticLabel(t, db, "CPI", "clh-cpi", "auxiliary", "active", 5, []float64{0, 1, 0})

	resp := performJSON(t, router, http.MethodPost, "/api/composite-labels", map[string]any{
		"label":               "中国CPI",
		"component_label_ids": []uint{auxA.ID, auxB.ID},
	})
	require.Equal(t, http.StatusOK, resp.Code)
	created := decodeCompositeCreateBody(t, resp.Body.String())

	// Disable → 200; active filter no longer returns it.
	disableResp := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/composite-labels/%d/disable", created.ID), nil)
	require.Equal(t, http.StatusOK, disableResp.Code)
	activeList := performJSON(t, router, http.MethodGet, "/api/composite-labels?status=active", nil)
	require.Equal(t, http.StatusOK, activeList.Code)
	require.Equal(t, 0, decodeCompositeListBody(t, activeList).Total)

	// Disabled row keeps NULL vector (red line) while components survive.
	var reloaded models.SemanticLabel
	require.NoError(t, db.Where("id = ?", created.ID).First(&reloaded).Error)
	require.Equal(t, "disabled", reloaded.Status)
	require.Nil(t, reloaded.Embedding)

	// Enable → 200 (embedder re-runs).
	enableResp := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/composite-labels/%d/enable", created.ID), nil)
	require.Equal(t, http.StatusOK, enableResp.Code)

	// Unknown id → 404.
	notFound := performJSON(t, router, http.MethodPost, "/api/composite-labels/999999/disable", nil)
	require.Equal(t, http.StatusNotFound, notFound.Code)
}

// TestCompositeLabelHandlerComponentOptions（S12 主链路步1）：推荐排序
// 数据源端到端——board_count 优先 + ref_count 次之 + 挂载版块名列表。
func TestCompositeLabelHandlerComponentOptions(t *testing.T) {
	db, router := setupCompositeRouter(t)

	board := createHandlerSemanticLabel(t, db, "宏观版块", "coh-macro", "board", "active", 0, nil)
	auxMounted := createHandlerSemanticLabel(t, db, "美国国债", "coh-treasury", "auxiliary", "active", 3, []float64{1, 0, 0})
	createHandlerSemanticLabel(t, db, "热门通用", "coh-hot", "auxiliary", "active", 20, []float64{0, 1, 0})
	createHandlerSemanticLabel(t, db, "冷门通用", "coh-cold", "auxiliary", "active", 1, []float64{0, 0, 1})
	require.NoError(t, db.Create(&models.BoardComposition{BoardID: board.ID, AuxiliaryLabelID: auxMounted.ID}).Error)

	resp := performJSON(t, router, http.MethodGet, "/api/composite-labels/component-options", nil)
	require.Equal(t, http.StatusOK, resp.Code)

	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				ID            uint   `json:"id"`
				Label         string `json:"label"`
				RefCount      int    `json:"ref_count"`
				BoardCount    int    `json:"board_count"`
				MountedBoards []struct {
					ID    uint   `json:"id"`
					Label string `json:"label"`
				} `json:"mounted_boards"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Body.String()), &parsed))
	require.True(t, parsed.Success)
	require.NotEmpty(t, parsed.Data.Items)

	labels := make([]string, 0, len(parsed.Data.Items))
	for _, it := range parsed.Data.Items {
		labels = append(labels, it.Label)
	}
	// 美国国债(1 版块挂载, ref 3) > 热门通用(0 挂载, ref 20) > 冷门通用(0 挂载, ref 1)
	require.Equal(t, []string{"美国国债", "热门通用", "冷门通用"}, labels)
	require.Equal(t, 1, parsed.Data.Items[0].BoardCount)
	require.Len(t, parsed.Data.Items[0].MountedBoards, 1)
	require.Equal(t, "宏观版块", parsed.Data.Items[0].MountedBoards[0].Label)
}
