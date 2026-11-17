package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// getTopicAnchor handles GET /api/daily-reports/topics/:id/anchor.
//
// Read side of the 首义 contract (lane-manual-correction-loop): the write
// endpoint (PATCH .../anchor) returns what the new anchor is, but the
// correction workbench also needs to show the **current** anchor when it
// opens — which source it came from (auto / manual) and, for a manual one,
// which nodes define it.
//
// Response:
//
//	{
//	  "topic_id": 7,
//	  "source": "auto" | "manual",
//	  "section_ids": [101, 109],   // 当前首义节点集；自动首义为空数组（不虚构）
//	  "override_count": 2,          // 人工改写次数
//	  "effective_from": "next_report" // 人工改写的生效时机；自动首义为空串
//	}
//
// Errors: unknown topic → 404. Read-only: this endpoint never writes.
func getTopicAnchor(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}

	state, err := repository.Repo.GetTopicAnchorState(topicID)
	if err != nil {
		if errors.Is(err, repository.ErrTopicNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "topic not found"})
			return
		}
		logging.Errorf("get topic %d anchor: %v", topicID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to read topic anchor"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": state})
}
