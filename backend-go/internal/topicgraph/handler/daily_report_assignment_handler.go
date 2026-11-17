package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// 人工归属变更（lane-manual-correction D4/D6）——一个端点承载两个方向：
//
//	PATCH /api/daily-reports/sections/:id/assignment  {"topic_id": null}   移除
//	PATCH /api/daily-reports/sections/:id/assignment  {"topic_id": 7}      加回
//	POST /api/daily-reports/topics/:id/detach-date    {"period_date": "2026-11-10"}
//
// 移除不需要指定去处（用户表达的是「它不该在这儿」，不是「它该在那儿」），
// 加回只接受当前无归属的节点（不提供改挂语义）。

// updateSectionAssignment handles PATCH /api/daily-reports/sections/:id/assignment.
//
// Body: {"topic_id": null} detaches the node from its lane; {"topic_id": 7}
// attaches it to lane 7. The two directions share the endpoint because they
// write the same snapshot columns with the same audit trail.
//
// Response: section_id / topic_id / previous_topic_id / action / changed.
func updateSectionAssignment(c *gin.Context) {
	sectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid section id"})
		return
	}

	target, err := parseAssignmentTopicID(c)
	if err != nil {
		return
	}

	result, err := repository.Repo.SetSectionTopicAssignment(uint(sectionID), target)
	if err != nil {
		logging.Errorf("change assignment of section %d: %v", sectionID, err)
		c.JSON(assignmentErrorStatus(err), gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// parseAssignmentTopicID reads the body's topic_id. The field is nullable and
// its absence is a different request from an explicit null, so it is decoded
// as a raw JSON value: null → 移除, a positive integer → 加回, anything else
// (string / negative / zero / missing) → 400.
func parseAssignmentTopicID(c *gin.Context) (*uint, error) {
	var req struct {
		TopicID json.RawMessage `json:"topic_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: topic_id must be a number or null"})
		return nil, err
	}
	raw := strings.TrimSpace(string(req.TopicID))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "topic_id is required"})
		return nil, errors.New("topic_id is required")
	}
	if raw == "null" {
		return nil, nil
	}
	var id int64
	if err := json.Unmarshal(req.TopicID, &id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "topic_id must be a number or null"})
		return nil, err
	}
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "topic_id must be a positive number"})
		return nil, errors.New("topic_id must be a positive number")
	}
	topicID := uint(id)
	return &topicID, nil
}

// detachTopicByDate handles POST /api/daily-reports/topics/:id/detach-date.
//
// Body: {"period_date": "2026-11-10"} — every node of that lane on that day
// is released in one transaction (each one following the single-detach口径:
// four snapshot columns NULL, lane_tier preserved, audit action=detach), and
// the board's relations are rebuilt once at the end.
//
// Response: topic_id / period_date / detached / section_ids.
func detachTopicByDate(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}

	var req struct {
		PeriodDate string `json:"period_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body: period_date must be YYYY-MM-DD"})
		return
	}
	day, err := time.Parse("2006-01-02", strings.TrimSpace(req.PeriodDate))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "period_date must be YYYY-MM-DD"})
		return
	}

	result, err := repository.Repo.DetachTopicSectionsByDate(topicID, day)
	if err != nil {
		logging.Errorf("detach topic %d on %s: %v", topicID, req.PeriodDate, err)
		c.JSON(assignmentErrorStatus(err), gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// assignmentErrorStatus maps a repository error onto an HTTP status:
//   - unknown section / unknown topic → 404
//   - archived target lane → 409
//   - nothing to detach / already assigned to another lane → 409
//   - 改挂 out of an active lane → 409（动态 candidate 泳道可改挂）
//   - target lane on another board → 400
//   - anything else (DB failure) → 500
func assignmentErrorStatus(err error) int {
	switch {
	case errors.Is(err, repository.ErrSectionNotFound),
		errors.Is(err, repository.ErrTopicNotFound):
		return http.StatusNotFound
	case errors.Is(err, repository.ErrTopicArchived),
		errors.Is(err, repository.ErrSectionAlreadyUnassigned),
		errors.Is(err, repository.ErrSectionAlreadyAssigned),
		errors.Is(err, repository.ErrReassignFromActiveLane):
		return http.StatusConflict
	case errors.Is(err, repository.ErrTopicOnOtherBoard):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
