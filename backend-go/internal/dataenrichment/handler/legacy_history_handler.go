package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/dataenrichment/repository"
)

// ── 旧研究历史只读列表（redesign-enrichment-research-workbench D2 GET /history）
//
// GET /semantic-boards/:id/enrichment/analysis/history
//
//	[&kind=&topic_id=&before_id=&limit=]
//
// 按 board 或所属 topic JOIN 汇总可验证归属的旧 result（board_brief /
// board_investigation / legacy_board_analysis / topic_analysis，含已归档话
// 题），id DESC 排他游标分页。读取零 LLM、零编排调用：不做增强开关预检——
// 开关关闭不阻止只读访问（spec「历史分页与开关无关」）。signal_report 永不
// 混入（新报告走 GET /signal-reports）；详情复用既有 GET /results/:rid 等，
// 不在此新增历史详情 API。
const (
	legacyHistoryDefaultLimit = 20
	legacyHistoryMaxLimit     = 100
)

// listLegacyHistory handles GET /history.
func (h *EnrichmentHandler) listLegacyHistory(c *gin.Context) {
	boardID, ok := parseBoardID(c)
	if !ok {
		return
	}
	kind := strings.TrimSpace(c.Query("kind"))
	if kind != "" && !repository.ValidLegacyHistoryKind(kind) {
		respondError(c, http.StatusBadRequest,
			"kind must be board_brief|board_investigation|legacy_board_analysis|topic_analysis")
		return
	}
	var topicID uint
	if raw := strings.TrimSpace(c.Query("topic_id")); raw != "" {
		v, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || v == 0 {
			respondError(c, http.StatusBadRequest, "invalid topic_id")
			return
		}
		topicID = uint(v)
	}
	beforeID, ok := parseBeforeID(c)
	if !ok {
		return
	}
	limit, ok := parseListLimit(c, legacyHistoryDefaultLimit, legacyHistoryMaxLimit)
	if !ok {
		return
	}
	rows, err := h.repo.ListLegacyResults(c.Request.Context(), boardID, kind, topicID, beforeID, limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []repository.LegacyHistoryRow{}
	}
	respondOK(c, rows)
}
