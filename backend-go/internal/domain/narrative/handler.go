package narrative

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/domain/models"
)

var service = NewNarrativeService()

func RegisterNarrativeRoutes(rg *gin.RouterGroup) {
	group := rg.Group("/narratives")
	{
		group.GET("/timeline", getNarrativeTimeline)
		group.GET("/scopes", getNarrativeScopes)
		group.POST("/regenerate", regenerateNarratives)
		group.GET("", getNarratives)
		group.DELETE("", deleteNarratives)
		group.GET("/:id", getNarrative)
		group.GET("/:id/history", getNarrativeHistory)
	}

	boardGroup := rg.Group("/narratives/boards")
	{
		boardGroup.GET("/timeline", getBoardTimeline)
		boardGroup.GET("/:id", getBoardDetail)
		boardGroup.POST("/generate", triggerGenerate)
	}

}

func parseScopeParams(c *gin.Context) (scopeType string, categoryID *uint) {
	scopeType = c.DefaultQuery("scope_type", "")
	catIDStr := c.Query("category_id")
	if catIDStr != "" {
		if id, err := strconv.ParseUint(catIDStr, 10, 32); err == nil {
			uid := uint(id)
			categoryID = &uid
		}
	}
	return
}

func getNarrativeTimeline(c *gin.Context) {
	dateStr := c.Query("date")
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	daysStr := c.DefaultQuery("days", "7")
	days := 7
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	scopeType, categoryID := parseScopeParams(c)

	timeline, err := service.GetTimeline(date, days, scopeType, categoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": timeline})
}

func getNarratives(c *gin.Context) {
	boardIDStr := c.Query("board_id")
	if boardIDStr != "" {
		boardID, err := strconv.ParseUint(boardIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board_id"})
			return
		}

		narratives, err := service.GetByBoardID(uint(boardID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true, "data": narratives})
		return
	}

	dateStr := c.Query("date")
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	scopeType, categoryID := parseScopeParams(c)

	narratives, err := service.GetByDate(date, scopeType, categoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": narratives})
}

func deleteNarratives(c *gin.Context) {
	dateStr := c.Query("date")
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	scopeType, categoryID := parseScopeParams(c)

	deleted, err := service.DeleteByDate(date, scopeType, categoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deleted": deleted}})
}

func getNarrativeScopes(c *gin.Context) {
	dateStr := c.Query("date")
	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	daysStr := c.DefaultQuery("days", "7")
	days := 7
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	scopes, err := service.GetScopes(date, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": scopes})
}

func getNarrative(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	narrative, err := service.GetNarrativeTree(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "narrative not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": narrative})
}

func getNarrativeHistory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}

	history, err := service.GetNarrativeHistory(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": history})
}

type regenerateRequest struct {
	Date       string `json:"date"`
	ScopeType  string `json:"scope_type"`
	CategoryID *uint  `json:"category_id"`
}

func regenerateNarratives(c *gin.Context) {
	var req regenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request body"})
		return
	}

	var date time.Time
	if req.Date != "" {
		var err error
		date, err = time.ParseInLocation("2006-01-02", req.Date, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	var saved int
	var err error

	if req.ScopeType == models.NarrativeScopeTypeFeedCategory && req.CategoryID != nil {
		saved, err = service.RegenerateAndSaveForCategory(date, *req.CategoryID)
	} else {
		saved, err = service.RegenerateAndSave(date)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"saved": saved}})
}

func getBoardTimeline(c *gin.Context) {
	dateStr := c.Query("date")
	var anchorDate time.Time
	if dateStr != "" {
		var err error
		anchorDate, err = time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		anchorDate = time.Now()
	}

	daysStr := c.DefaultQuery("days", "7")
	days := 7
	if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
		days = d
	}

	scopeType, categoryID := parseScopeParams(c)

	startOfAnchor := time.Date(anchorDate.Year(), anchorDate.Month(), anchorDate.Day(), 0, 0, 0, 0, time.Local)
	rangeStart := startOfAnchor.AddDate(0, 0, -(days - 1))
	rangeEnd := startOfAnchor.Add(24 * time.Hour)

	timeline, err := service.GetBoardTimeline(rangeStart, rangeEnd, scopeType, categoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": timeline})
}

func getBoardDetail(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}

	detail, err := service.GetBoardDetail(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "board not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": detail})
}

func triggerGenerate(c *gin.Context) {
	var req struct {
		Date    string `json:"date" binding:"required"` // YYYY-MM-DD
		BoardID *uint  `json:"board_id"`               // nil = all boards
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "date is required (YYYY-MM-DD)"})
		return
	}

	date, err := time.ParseInLocation("2006-01-02", req.Date, time.Local)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
		return
	}

	var saved int
	if req.BoardID != nil {
		saved, err = service.RegenerateAndSaveForBoard(*req.BoardID, date)
	} else {
		saved, err = service.RegenerateAndSave(date)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"saved": saved}})
}
