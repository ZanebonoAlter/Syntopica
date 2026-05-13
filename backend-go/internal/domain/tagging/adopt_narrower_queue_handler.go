package tagging

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/domain/models"
)

var adoptNarrowerQueueService *AdoptNarrowerQueueService
var adoptNarrowerQueueServiceOnce sync.Once

func getAdoptNarrowerQueueService() *AdoptNarrowerQueueService {
	adoptNarrowerQueueServiceOnce.Do(func() {
		adoptNarrowerQueueService = NewAdoptNarrowerQueueService(nil)
	})
	return adoptNarrowerQueueService
}

func StartAdoptNarrowerQueueWorker() {
	getAdoptNarrowerQueueService().Start()
}

func StopAdoptNarrowerQueueWorker() {
	getAdoptNarrowerQueueService().Stop()
}

func GetAdoptNarrowerQueueStatus(c *gin.Context) {
	svc := getAdoptNarrowerQueueService()
	type statusRow struct {
		Status string
		Count  int64
	}
	var rows []statusRow
	if err := svc.db.Model(&models.AdoptNarrowerQueue{}).
		Select("status, count(*) as count").
		Group("status").
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	result := map[string]int64{
		"pending": 0, "processing": 0, "completed": 0, "failed": 0, "total": 0,
	}
	var total int64
	for _, r := range rows {
		result[r.Status] = r.Count
		total += r.Count
	}
	result["total"] = total
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func RetryAdoptNarrowerQueueFailed(c *gin.Context) {
	svc := getAdoptNarrowerQueueService()
	result := svc.db.Model(&models.AdoptNarrowerQueue{}).
		Where("status = ? AND retry_count < ?", models.AdoptNarrowerQueueStatusFailed, maxRetryCount).
		Updates(map[string]interface{}{
			"status":        models.AdoptNarrowerQueueStatusPending,
			"error_message": "",
			"started_at":    nil,
			"completed_at":  nil,
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": result.Error.Error()})
		return
	}
	var skippedCount int64
	svc.db.Model(&models.AdoptNarrowerQueue{}).
		Where("status = ? AND retry_count >= ?", models.AdoptNarrowerQueueStatusFailed, maxRetryCount).
		Count(&skippedCount)

	msg := "已重试 " + strconv.FormatInt(result.RowsAffected, 10) + " 个失败任务"
	if skippedCount > 0 {
		msg += "，" + strconv.FormatInt(skippedCount, 10) + " 个任务已达重试上限"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": msg,
	})
}

func RegisterAdoptNarrowerQueueRoutes(rg *gin.RouterGroup) {
	queue := rg.Group("/embedding/adopt-narrower")
	{
		queue.GET("/status", GetAdoptNarrowerQueueStatus)
		queue.POST("/retry", RetryAdoptNarrowerQueueFailed)
	}
}
