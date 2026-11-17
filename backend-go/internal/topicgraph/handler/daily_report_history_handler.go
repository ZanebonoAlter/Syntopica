package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
)

// 人工修正留痕的只读查询端点（review R-2）。留痕本身是 append-only 的两张
// 表，写侧已落地；这里只提供「查得到」的能力：
//
//	GET /api/daily-reports/topics/:id/anchor/history     首义改写历史（时间升序）
//	GET /api/daily-reports/sections/:id/assignment/history  归属变更历史（时间升序）
//
// 两个端点都是纯读：不写任何列、不重算任何派生值。

// getTopicAnchorHistory handles GET /api/daily-reports/topics/:id/anchor/history.
//
// Response:
//
//	{
//	  "topic_id": 7,
//	  "overrides": [
//	    {
//	      "id": 1,
//	      "topic_id": 7,
//	      "source": "selected_sections",
//	      "section_ids": [101, 105, 110],
//	      "old_vector": {"dimension": 768, "head": [...], "norm": 1.02},
//	      "new_vector": {"dimension": 768, "head": [...], "norm": 1.04},
//	      "operator": "user",
//	      "created_at": "2026-11-15T09:00:00Z"
//	    }
//	  ]
//	}
//
// Rows are ordered oldest first (created_at ASC, id ASC). `old_vector` is null
// when the topic had no usable vector before the first rewrite.
//
// Errors: unknown topic → 404. Read-only: never writes.
func getTopicAnchorHistory(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}

	entries, err := repository.Repo.ListTopicAnchorOverrideHistory(topicID)
	if err != nil {
		if errors.Is(err, repository.ErrTopicNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "topic not found"})
			return
		}
		logging.Errorf("get topic %d anchor history: %v", topicID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to read topic anchor history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"topic_id":  topicID,
		"overrides": entries,
	}})
}

// getSectionAssignmentHistory handles GET
// /api/daily-reports/sections/:id/assignment/history.
//
// Response:
//
//	{
//	  "section_id": 501,
//	  "entries": [
//	    {"id": 1, "section_id": 501, "topic_id_from": 8, "topic_id_to": null,
//	     "action": "detach", "operator": "user", "created_at": "..."},
//	    {"id": 2, "section_id": 501, "topic_id_from": null, "topic_id_to": 8,
//	     "action": "attach", "operator": "user", "created_at": "..."}
//	  ]
//	}
//
// Rows are ordered oldest first, so a 「移除 → 加回」 pair shows the detach
// first and the attach second.
//
// Errors: unknown section → 404. Read-only: never writes.
func getSectionAssignmentHistory(c *gin.Context) {
	sectionID, err := parseSectionID(c)
	if err != nil {
		return
	}

	entries, err := repository.Repo.ListSectionAssignmentHistory(sectionID)
	if err != nil {
		if errors.Is(err, repository.ErrSectionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "section not found"})
			return
		}
		logging.Errorf("get section %d assignment history: %v", sectionID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to read section assignment history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"section_id": sectionID,
		"entries":    entries,
	}})
}

// parseSectionID reads the :id path parameter as a section id.
func parseSectionID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid section id"})
		return 0, err
	}
	return uint(id), nil
}
