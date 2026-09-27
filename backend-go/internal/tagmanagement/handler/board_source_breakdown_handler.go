package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/tagmanagement/service/sourcestats"
)

// getBoardSourceBreakdown — GET /api/semantic-boards/:id/source-breakdown?window=7
// (spec: 按板块聚合端点). Read-only: no writes, no tagging/matching side
// effects. Invalid window → 400; board missing or not label_type='board' →
// 404; aggregation failure → 500 (design D4).
func (h *semanticBoardHandler) getBoardSourceBreakdown(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	windowDays, err := sourcestats.ParseWindow(c.Query("window"))
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	breakdown, err := sourcestats.BoardSourceBreakdown(c.Request.Context(), h.db, id, windowDays)
	if err != nil {
		if errors.Is(err, sourcestats.ErrBoardNotFound) {
			respondError(c, http.StatusNotFound, err)
			return
		}
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondOK(c, breakdown)
}
