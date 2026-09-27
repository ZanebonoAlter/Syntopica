package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/discovery/service"
	"syntopica-backend/internal/platform/database"
)

// GetPreferenceProfile GET /api/preference-profile — 兴趣画像（版块分组 top 标签/权重/来源/最后计算时间）。
func GetPreferenceProfile(c *gin.Context) {
	svc := service.NewPreferenceProfileService(database.DB)
	items, err := svc.GetProfile(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// RecomputePreferenceProfile POST /api/preference-profile/recompute — 手动触发重算（与 scheduler 同路径）。
func RecomputePreferenceProfile(c *gin.Context) {
	svc := service.NewPreferenceProfileService(database.DB)
	summary, err := svc.RecomputeAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
		"message": "preference profile recomputed",
	})
}
