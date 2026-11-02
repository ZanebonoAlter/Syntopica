package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// getBoardRadarOverview handles GET /api/semantic-boards/overview (spec 8.2).
// Read-only single shot: active boards + today/3d/14d section density +
// latest report brief. The repository reuses lane-dynamics and the report
// table; the 5s budget degrades late rows to zero + data_delayed.
func getBoardRadarOverview(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	overview, err := repository.Repo.GetBoardRadarOverview(ctx)
	if err != nil {
		logging.Errorf("get board radar overview: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to get board overview"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": overview})
}
