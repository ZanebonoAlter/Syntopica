package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// laneDynamicsDefaultDays is the default timeline/snapshot window for the
// lane-dynamics endpoint (spec: days 参数默认 14).
const laneDynamicsDefaultDays = repository.LaneDynamicsDefaultDays

// getBoardLaneDynamics handles GET /api/semantic-boards/:id/lane-dynamics?days=14.
// Single-shot aggregate for the 板块内容 lane-dynamics view (design D4):
// lanes (active ∪ watch-linked, section_count_14d DESC, snapshot or null,
// per-day timeline with folded thread titles) + candidates (visible-gate +
// latest-section hint) + has_reports for the empty-state split. Read-only.
func getBoardLaneDynamics(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}
	days := laneDynamicsDefaultDays
	if v := c.Query("days"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil && n > 0 {
			days = n
		}
	}
	resp, err := repository.Repo.GetBoardLaneDynamics(uint(boardID), days)
	if err != nil {
		logging.Errorf("get board lane dynamics: board=%d: %v", boardID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to get lane dynamics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}
