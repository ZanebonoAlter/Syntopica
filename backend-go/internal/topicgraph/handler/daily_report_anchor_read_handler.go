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
//	  "override_count": 2,          // 首义留痕条数（含手动建泳道的建话题留痕）
//	  "effective_from": "next_report", // 人工留痕的生效时机；自动首义为空串
//	  "anchor_origin": {            // 首义来源节点（可考 / 不可考）
//	    "resolved": true,
//	    "section_id": 101,
//	    "title": "…",
//	    "period_date": "2026-11-10"
//	  },
//	  "centroid": {                 // 滚动窗口口径（只读）
//	    "window_size": 30,
//	    "anchor_weight": 0.5,
//	    "section_ids": […],
//	    "members": [{"section_id": …, "title": "…", "period_date": "…"}],
//	    "degraded": false
//	  }
//	}
//
// anchor_origin.resolved=false 时，section_id / title / period_date 一律为
// null（来源不可考，绝不用「最早成员」顶替）。
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
