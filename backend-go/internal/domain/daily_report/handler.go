package daily_report

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/ws"
)

// RegisterDailyReportRoutes registers all daily report routes.
func RegisterDailyReportRoutes(api *gin.RouterGroup) {
	// POST /api/daily-reports/generate
	api.POST("/daily-reports/generate", triggerGenerateDailyReport)

	// GET /api/daily-reports/:id
	api.GET("/daily-reports/:id", getDailyReport)

	// GET /api/semantic-boards/:id/daily-reports
	api.GET("/semantic-boards/:id/daily-reports", listBoardDailyReports)
}

// triggerGenerateDailyReport handles POST /api/daily-reports/generate
func triggerGenerateDailyReport(c *gin.Context) {
	var req struct {
		Date    string `json:"date"`
		BoardID *uint  `json:"board_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Date is optional, defaults to today
		req.Date = ""
	}

	var date time.Time
	if req.Date != "" {
		parsed, err := time.ParseInLocation("2006-01-02", req.Date, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date format, use YYYY-MM-DD"})
			return
		}
		date = parsed
	} else {
		date = time.Now()
	}

	jobID := uuid.New().String()

	if req.BoardID != nil {
		// Generate for single board
		go generateSingleBoard(*req.BoardID, date, jobID)
	} else {
		// Generate for all boards
		go generateAllBoards(date, jobID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"job_id": jobID,
			"status": "processing",
		},
	})
}

func generateSingleBoard(boardID uint, date time.Time, jobID string) {
	ctx, cancel := timeoutCtx(10 * time.Minute)
	defer cancel()

	broadcastProgress(jobID, "processing", boardID, 0, 0)

	report, sections, err := GenerateDailyReport(ctx, boardID, date)
	if err != nil {
		logging.Errorf("daily-report: generate failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, 0, 0)
		return
	}
	if report == nil {
		broadcastProgress(jobID, "completed", boardID, 0, 0)
		return
	}

	if err := SaveReport(report, sections); err != nil {
		logging.Errorf("daily-report: save failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, 0, 0)
		return
	}

	broadcastProgress(jobID, "completed", boardID, report.ID, len(sections))
}

func generateAllBoards(date time.Time, jobID string) {
	ctx, cancel := timeoutCtx(30 * time.Minute)
	defer cancel()

	boardIDs, err := CollectBoardIDsForDate(date)
	if err != nil {
		logging.Errorf("daily-report: collect boards failed: %v", err)
		broadcastProgress(jobID, "failed", 0, 0, 0)
		return
	}

	if len(boardIDs) == 0 {
		broadcastProgress(jobID, "completed", 0, 0, 0)
		return
	}

	completed := 0
	for _, boardID := range boardIDs {
		broadcastProgress(jobID, "processing", boardID, 0, completed)

		report, sections, genErr := GenerateDailyReport(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate failed for board %d: %v", boardID, genErr)
			continue
		}
		if report == nil {
			completed++
			continue
		}

		if saveErr := SaveReport(report, sections); saveErr != nil {
			logging.Warnf("daily-report: save failed for board %d: %v", boardID, saveErr)
			continue
		}
		completed++
	}

	broadcastProgress(jobID, "completed", 0, 0, completed)
}

// listBoardDailyReports handles GET /api/semantic-boards/:id/daily-reports
func listBoardDailyReports(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}

	daysStr := c.DefaultQuery("days", "7")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 7
	}

	reports, err := ListReports(uint(boardID), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if reports == nil {
		reports = []ReportListItem{}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": reports})
}

// getDailyReport handles GET /api/daily-reports/:id
func getDailyReport(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report id"})
		return
	}

	report, err := GetReportByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "report not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

// broadcastProgress sends a WebSocket progress message.
func broadcastProgress(jobID string, status string, boardID uint, reportID uint, completed int) {
	msg := map[string]interface{}{
		"type":     "daily_report_progress",
		"job_id":   jobID,
		"status":   status,
		"board_id": boardID,
		"report_id": reportID,
		"completed": completed,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	data, _ := json.Marshal(msg)
	ws.GetHub().BroadcastRaw(data)
}

func timeoutCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
