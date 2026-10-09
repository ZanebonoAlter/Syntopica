package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/tagmanagement/service/boardhealth"
)

// Trend series whitelist: default 7 points, hard bounds [1, 90] (design D9).
// Values outside the range (0, 91, negatives, non-integers) → 400 — never a
// silent fallback (same discipline as sourcestats.ParseWindow).
const (
	defaultTrendPoints = 7
	minTrendPoints     = 1
	maxTrendPoints     = 90
)

// parseTrendParam resolves ?trend=N against the whitelist.
func parseTrendParam(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultTrendPoints, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid trend: must be an integer in [%d, %d]", minTrendPoints, maxTrendPoints)
	}
	if n < minTrendPoints || n > maxTrendPoints {
		return 0, fmt.Errorf("invalid trend %d: must be in [%d, %d]", n, minTrendPoints, maxTrendPoints)
	}
	return n, nil
}

// getBoardHealthCheckup — GET /api/semantic-boards/:id/health-checkup?trend=N
// (spec: 体检数据查询端点只读). Returns the latest snapshot plus the trend
// series (ascending checked_at). Read-only: no writes, no checkup trigger.
// Invalid trend → 400; board id absent from semantic_labels → 404;
// disabled / non-board → empty state (latest=null, trend=[]), NOT an error.
func (h *semanticBoardHandler) getBoardHealthCheckup(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}

	trendN, err := parseTrendParam(c.Query("trend"))
	if err != nil {
		respondError(c, http.StatusBadRequest, err)
		return
	}

	latest, trend, err := boardhealth.HealthCheckupView(c.Request.Context(), h.db, id, trendN)
	if err != nil {
		if errors.Is(err, boardhealth.ErrBoardNotFound) {
			respondError(c, http.StatusNotFound, err)
			return
		}
		respondError(c, http.StatusInternalServerError, err)
		return
	}

	respondOK(c, gin.H{"latest": latest, "trend": trend})
}
