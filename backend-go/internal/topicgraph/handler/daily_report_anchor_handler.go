package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// updateTopicAnchor handles PATCH /api/daily-reports/topics/:id/anchor.
//
// Body: {"section_ids": [101, 105, 110]} — the chosen lane nodes that define
// this lane's 首义. The server aggregates their embeddings (mean pooling, the
// same 口径 as the manual-topic creation path) and writes the result as the
// topic's 首义 vector.
//
// Free text is NOT an accepted input (lane-manual-correction D3): a
// natural-language description is rejected with 400 and never vectorized —
// the 首义 is defined by "which nodes define this lane", which the canvas
// shows.
//
// Response: topic_id / source / selected_sections / override_count /
// effective_from. effective_from is always "next_report": the fact layer
// changes now, the centroid (and therefore the matching) moves on the next
// daily report (D1 — no "recompute now" endpoint exists).
func updateTopicAnchor(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}

	var req struct {
		SectionIDs []uint `json:"section_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// A body carrying free text (or anything else that is not a list of
		// section ids) lands here — the 首义 has exactly one input mode.
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "invalid body: section_ids must be an array of section ids",
		})
		return
	}
	if len(req.SectionIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "section_ids is required"})
		return
	}

	result, err := repository.Repo.SetTopicAnchorFromSections(topicID, req.SectionIDs, repository.AnchorOperatorUser)
	if err != nil {
		logging.Errorf("set topic %d anchor: %v", topicID, err)
		c.JSON(anchorErrorStatus(err), gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// anchorErrorStatus maps a repository error onto an HTTP status:
//   - unknown topic → 404
//   - archived topic → 409 (archived lanes are not anchorable)
//   - anything else (invalid node range, empty selection, DB failure) → 400
func anchorErrorStatus(err error) int {
	switch {
	case errors.Is(err, repository.ErrTopicNotFound):
		return http.StatusNotFound
	case errors.Is(err, repository.ErrTopicArchived):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
