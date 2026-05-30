package analysis

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/domain/tagging"
)

type AnalysisHandler struct {
	service AnalysisService
}

// RegisterAnalysisRoutes 注册 topic analysis 相关路由
func RegisterAnalysisRoutes(router *gin.RouterGroup, service AnalysisService) {
	handler := &AnalysisHandler{service: service}

	group := router.Group("/analysis")
	{
		group.GET("", handler.GetAnalysisByQuery)
		group.GET("/status", handler.GetAnalysisStatusByQuery)
		group.POST("/rebuild", handler.RebuildAnalysisByQuery)
		group.POST("/retry", handler.RebuildAnalysisByQuery)
		group.GET("/:tagID/:analysisType", handler.GetAnalysis)
		group.POST("/:tagID/:analysisType/rebuild", handler.RebuildAnalysis)
		group.GET("/:tagID/:analysisType/status", handler.GetAnalysisStatus)
	}
}

func (h *AnalysisHandler) GetAnalysisByQuery(c *gin.Context) {
	tagID, analysisType, windowType, anchorDate, ok := parseQueryParams(c)
	if !ok {
		return
	}

	analysis, err := h.service.GetOrCreateAnalysis(tagID, analysisType, windowType, anchorDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": analysis})
}

func (h *AnalysisHandler) GetAnalysisStatusByQuery(c *gin.Context) {
	tagID, analysisType, windowType, anchorDate, ok := parseQueryParams(c)
	if !ok {
		return
	}

	status, progress, err := h.service.GetAnalysisStatus(tagID, analysisType, windowType, anchorDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": status, "progress": progress}})
}

func (h *AnalysisHandler) RebuildAnalysisByQuery(c *gin.Context) {
	tagID, analysisType, windowType, anchorDate, ok := parseQueryParams(c)
	if !ok {
		return
	}

	if err := h.service.RebuildAnalysis(tagID, analysisType, windowType, anchorDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	analysis, err := h.service.GetAnalysis(tagID, analysisType, windowType, anchorDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": analysis})
}

func (h *AnalysisHandler) GetAnalysis(c *gin.Context) {
	tagID, ok := parseTagIDParam(c)
	if !ok {
		return
	}

	analysisType := c.Param("analysisType")
	windowType, anchorDate, err := parseWindowAnchorFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, expected YYYY-MM-DD"})
		return
	}

	analysis, err := h.service.GetOrCreateAnalysis(tagID, analysisType, windowType, anchorDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": analysis})
}

func (h *AnalysisHandler) RebuildAnalysis(c *gin.Context) {
	tagID, ok := parseTagIDParam(c)
	if !ok {
		return
	}

	analysisType := c.Param("analysisType")
	windowType, anchorDate, err := parseWindowAnchorFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, expected YYYY-MM-DD"})
		return
	}

	if err := h.service.RebuildAnalysis(tagID, analysisType, windowType, anchorDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	analysis, err := h.service.GetAnalysis(tagID, analysisType, windowType, anchorDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": analysis})
}

func (h *AnalysisHandler) GetAnalysisStatus(c *gin.Context) {
	tagID, ok := parseTagIDParam(c)
	if !ok {
		return
	}

	analysisType := c.Param("analysisType")
	windowType, anchorDate, err := parseWindowAnchorFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, expected YYYY-MM-DD"})
		return
	}

	status, progress, err := h.service.GetAnalysisStatus(tagID, analysisType, windowType, anchorDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"status":   status,
			"progress": progress,
		},
	})
}

func parseTagIDParam(c *gin.Context) (uint64, bool) {
	tagIDStr := c.Param("tagID")
	tagID, err := strconv.ParseUint(tagIDStr, 10, 64)
	if err != nil || tagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid tagID"})
		return 0, false
	}
	return tagID, true
}

func parseQueryParams(c *gin.Context) (uint64, string, string, time.Time, bool) {
	tagIDStr := c.Query("tag_id")
	tagID, err := strconv.ParseUint(tagIDStr, 10, 64)
	if err != nil || tagID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid tag_id"})
		return 0, "", "", time.Time{}, false
	}

	analysisType := c.Query("analysis_type")
	if analysisType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "analysis_type is required"})
		return 0, "", "", time.Time{}, false
	}

	windowType, anchorDate, err := parseWindowAnchorFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid anchor date format, expected YYYY-MM-DD"})
		return 0, "", "", time.Time{}, false
	}

	return tagID, analysisType, windowType, anchorDate, true
}

func parseWindowAnchorFromRequest(c *gin.Context) (string, time.Time, error) {
	windowType := c.Query("windowType")
	if strings.TrimSpace(windowType) == "" {
		windowType = c.Query("window_type")
	}
	if strings.TrimSpace(windowType) == "" {
		windowType = c.Query("window")
	}
	windowType = normalizeWindowType(windowType)

	anchorDateRaw := c.Query("anchorDate")
	if strings.TrimSpace(anchorDateRaw) == "" {
		anchorDateRaw = c.Query("anchor_date")
	}
	if strings.TrimSpace(anchorDateRaw) == "" {
		anchorDateRaw = c.Query("date")
	}

	if c.Request != nil && c.Request.Method == http.MethodPost && strings.HasPrefix(c.ContentType(), "application/json") {
		var body struct {
			WindowType string `json:"windowType"`
			AnchorDate string `json:"anchorDate"`
		}
		if err := c.ShouldBindJSON(&body); err == nil {
			if strings.TrimSpace(body.WindowType) != "" {
				windowType = normalizeWindowType(body.WindowType)
			}
			if strings.TrimSpace(body.AnchorDate) != "" {
				anchorDateRaw = body.AnchorDate
			}
		} else if !errorsIsEOF(err) {
			return "", time.Time{}, err
		}
		if c.Request.Body != nil {
			c.Request.Body = http.NoBody
		}
	}

	anchorDate, err := tagging.ParseAnchorDate(anchorDateRaw)
	if err != nil {
		return "", time.Time{}, err
	}
	return windowType, anchorDate, nil
}

func errorsIsEOF(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "eof") || strings.Contains(strings.ToLower(err.Error()), "empty")
}
