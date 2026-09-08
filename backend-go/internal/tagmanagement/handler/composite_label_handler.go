package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

// compositeLabelEmbedder is a package-level seam mirroring
// semanticBoardLabelEmbedder so handler tests can stub the LLM call.
var compositeLabelEmbedder service.AuxiliaryLabelEmbedder = service.DefaultAuxiliaryLabelEmbedder

type compositeLabelHandler struct {
	composite *service.CompositeLabelService
}

type createCompositeLabelRequest struct {
	Label             string `json:"label"`
	Description       string `json:"description"`
	ComponentLabelIDs []uint `json:"component_label_ids"`
}

// compositeLabelCreateResponse extends the service result with the dedup
// outcome surfaced as a user-facing message (manual create that hits L1/L2
// returns the existing composite instead of an error).
type compositeLabelCreateResponse struct {
	ID          uint     `json:"id"`
	Label       string   `json:"label"`
	Slug        string   `json:"slug"`
	Aliases     []string `json:"aliases"`
	Status      string   `json:"status"`
	Source      string   `json:"source"`
	RefCount    int      `json:"ref_count"`
	Outcome     string   `json:"outcome"`
	ReusedLabel string   `json:"reused_label,omitempty"`
	Message     string   `json:"message"`
}

func registerCompositeLabelRoutes(rg *gin.RouterGroup) {
	handler := &compositeLabelHandler{
		composite: service.NewCompositeLabelService(repository.Repo.DB(), compositeLabelEmbedder),
	}
	group := rg.Group("/composite-labels")
	{
		group.GET("", handler.listCompositeLabels)
		group.GET("/component-options", handler.listComponentOptions)
		group.POST("", handler.createCompositeLabel)
		group.POST("/:id/disable", handler.disableCompositeLabel)
		group.POST("/:id/enable", handler.enableCompositeLabel)
	}
}

func (h *compositeLabelHandler) listComponentOptions(c *gin.Context) {
	limit := 0
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}
	boardID := parseUintQuery(c, "board_id")
	relatedAuxID := parseUintQuery(c, "related_to")
	items, err := h.composite.ListComponentOptions(c.Request.Context(), limit, boardID, relatedAuxID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items}})
}

func (h *compositeLabelHandler) listCompositeLabels(c *gin.Context) {
	items, err := h.composite.ListCompositeLabels(c.Request.Context(), c.Query("status"))
	if err != nil {
		respondError(c, http.StatusInternalServerError, err)
		return
	}
	respondOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *compositeLabelHandler) createCompositeLabel(c *gin.Context) {
	var req createCompositeLabelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}
	if len(req.ComponentLabelIDs) < service.CompositeMinComponents || len(req.ComponentLabelIDs) > service.CompositeMaxComponents {
		respondError(c, http.StatusBadRequest, errCompositeComponentCount(len(req.ComponentLabelIDs)))
		return
	}

	result, err := h.composite.CreateCompositeLabel(c.Request.Context(), req.Label, req.Description, req.ComponentLabelIDs, "manual")
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	resp := compositeLabelCreateResponse{
		ID:       result.Label.ID,
		Label:    result.Label.Label,
		Slug:     result.Label.Slug,
		Aliases:  result.Label.Aliases,
		Status:   result.Label.Status,
		Source:   result.Label.Source,
		RefCount: result.Label.RefCount,
		Outcome:  string(result.Outcome),
	}
	switch result.Outcome {
	case service.CompositeOutcomeReusedL1:
		resp.Message = "组件集合与既有组合标签完全一致，已复用既有组合（ref_count+1）"
	case service.CompositeOutcomeAliasL2:
		resp.ReusedLabel = result.Label.Label
		resp.Message = "与既有组合语义高度相似，已作为别名并入既有组合（ref_count+1）"
	default:
		resp.Message = "组合标签已创建"
	}
	respondOK(c, resp)
}

func (h *compositeLabelHandler) disableCompositeLabel(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.composite.DisableCompositeLabel(c.Request.Context(), id); err != nil {
		respondError(c, statusForCompositeError(err), err)
		return
	}
	respondOK(c, gin.H{"id": id, "status": "disabled", "message": "组合标签已禁用（向量已清除，组件与别名保留）"})
}

func (h *compositeLabelHandler) enableCompositeLabel(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.composite.EnableCompositeLabel(c.Request.Context(), id); err != nil {
		respondError(c, statusForCompositeError(err), err)
		return
	}
	respondOK(c, gin.H{"id": id, "status": "active", "message": "组合标签已启用（embedding 已重算）"})
}

func statusForCompositeError(err error) int {
	if err == gorm.ErrRecordNotFound {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func errCompositeComponentCount(n int) error {
	return fmt.Errorf("组合标签需要 %d-%d 个不同组件，收到 %d", service.CompositeMinComponents, service.CompositeMaxComponents, n)
}

// parseUintQuery 解析可选的数字 query 参数（缺省/非法返回 0=未提供语义）。
func parseUintQuery(c *gin.Context, key string) uint {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0
	}
	return uint(parsed)
}
