package daily_report

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
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

	boardName := dailyReportBoardName(boardID)
	broadcastProgress(jobID, "generating", boardID, boardName, 0, "0/1")

	report, sections, err := GenerateDailyReport(ctx, boardID, date)
	if err != nil {
		logging.Errorf("daily-report: generate failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, boardName, 0, "1/1")
		broadcastDone(jobID, 0, 1)
		return
	}
	if report == nil {
		broadcastProgress(jobID, "completed", boardID, boardName, 0, "1/1")
		broadcastDone(jobID, 0, 1)
		return
	}

	if err := SaveReport(report, sections); err != nil {
		logging.Errorf("daily-report: save failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, boardName, 0, "1/1")
		broadcastDone(jobID, 0, 1)
		return
	}

	broadcastProgress(jobID, "completed", boardID, boardName, 1, "1/1")
	broadcastDone(jobID, 1, 1)
}

func generateAllBoards(date time.Time, jobID string) {
	ctx, cancel := timeoutCtx(30 * time.Minute)
	defer cancel()

	boardIDs, err := CollectBoardIDsForDate(date)
	if err != nil {
		logging.Errorf("daily-report: collect boards failed: %v", err)
		broadcastProgress(jobID, "failed", 0, "All boards", 0, "0/0")
		broadcastDone(jobID, 0, 0)
		return
	}

	totalBoards := len(boardIDs)
	if totalBoards == 0 {
		broadcastDone(jobID, 0, 0)
		return
	}

	savedCount := 0
	for idx, boardID := range boardIDs {
		boardName := dailyReportBoardName(boardID)
		broadcastProgress(jobID, "generating", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx, totalBoards))

		report, sections, genErr := GenerateDailyReport(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate failed for board %d: %v", boardID, genErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			continue
		}
		if report == nil {
			broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			continue
		}

		if saveErr := SaveReport(report, sections); saveErr != nil {
			logging.Warnf("daily-report: save failed for board %d: %v", boardID, saveErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			continue
		}
		savedCount++
		broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
	}

	broadcastDone(jobID, savedCount, totalBoards)
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

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"reports": reports}})
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

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"report": report}})
}

// broadcastProgress sends a WebSocket progress message.
func broadcastProgress(jobID string, status string, boardID uint, boardName string, saved int, progress string) {
	msg := buildProgressMessage(jobID, status, boardID, boardName, saved, progress)
	data, _ := json.Marshal(msg)
	ws.GetHub().BroadcastRaw(data)
}

func broadcastDone(jobID string, totalSaved int, totalBoards int) {
	msg := buildDoneMessage(jobID, totalSaved, totalBoards)
	data, _ := json.Marshal(msg)
	ws.GetHub().BroadcastRaw(data)
}

func buildProgressMessage(jobID string, status string, boardID uint, boardName string, saved int, progress string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "daily_report_progress",
		"job_id":     jobID,
		"status":     status,
		"board_id":   boardID,
		"board_name": boardName,
		"saved":      saved,
		"progress":   progress,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
}

func buildDoneMessage(jobID string, totalSaved int, totalBoards int) map[string]interface{} {
	return map[string]interface{}{
		"type":         "daily_report_done",
		"job_id":       jobID,
		"total_saved":  totalSaved,
		"total_boards": totalBoards,
		"timestamp":    time.Now().Format(time.RFC3339),
	}
}

func dailyReportBoardName(boardID uint) string {
	if boardID == 0 {
		return "All boards"
	}
	var board models.SemanticLabel
	if err := database.DB.Select("label").Where("id = ?", boardID).First(&board).Error; err != nil {
		return fmt.Sprintf("Board #%d", boardID)
	}
	return board.Label
}

func timeoutCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
