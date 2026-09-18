package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/tagmanagement/service/sourcestats"
)

// GetBoardHitStats — GET /api/feeds/board-hit-stats?window=7 (spec: 按订阅源
// 聚合端点). Read-only: one batched aggregation over all feeds, no writes, no
// tagging/matching side effects. Invalid window → 400 (never a silent
// fallback to the default); aggregation failure → 500.
func GetBoardHitStats(c *gin.Context) {
	windowDays, err := sourcestats.ParseWindow(c.Query("window"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	stats, err := sourcestats.FeedBoardHitStats(c.Request.Context(), repository.Repo.DB(), windowDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items": stats,
		},
	})
}
