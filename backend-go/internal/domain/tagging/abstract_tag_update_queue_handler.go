package tagging

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"my-robot-backend/internal/domain/models"
)

var abstractTagUpdateQueueService *AbstractTagUpdateQueueService
var abstractTagUpdateQueueServiceOnce sync.Once

func getAbstractTagUpdateQueueService() *AbstractTagUpdateQueueService {
	abstractTagUpdateQueueServiceOnce.Do(func() {
		abstractTagUpdateQueueService = NewAbstractTagUpdateQueueService(nil)
	})
	return abstractTagUpdateQueueService
}

func StartAbstractTagUpdateQueueWorker() {
	getAbstractTagUpdateQueueService().Start()
}

func StopAbstractTagUpdateQueueWorker() {
	getAbstractTagUpdateQueueService().Stop()
}

func GetAbstractTagUpdateQueueStatus(c *gin.Context) {
	svc := getAbstractTagUpdateQueueService()
	type statusRow struct {
		Status string
		Count  int64
	}
	var rows []statusRow
	if err := svc.db.Model(&models.AbstractTagUpdateQueue{}).
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

const maxRetryCount = 5

func RetryAbstractTagUpdateQueueFailed(c *gin.Context) {
	svc := getAbstractTagUpdateQueueService()
	result := svc.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("status = ? AND retry_count < ?", models.AbstractTagUpdateQueueStatusFailed, maxRetryCount).
		Updates(map[string]interface{}{
			"status":        models.AbstractTagUpdateQueueStatusPending,
			"error_message": "",
			"started_at":    nil,
			"completed_at":  nil,
			"retry_count":   0,
		})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": result.Error.Error()})
		return
	}
	var skippedCount int64
	svc.db.Model(&models.AbstractTagUpdateQueue{}).
		Where("status = ? AND retry_count >= ?", models.AbstractTagUpdateQueueStatusFailed, maxRetryCount).
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

func RegisterAbstractTagUpdateQueueRoutes(rg *gin.RouterGroup) {
	queue := rg.Group("/embedding/abstract-tag-update")
	{
		queue.GET("/status", GetAbstractTagUpdateQueueStatus)
		queue.POST("/retry", RetryAbstractTagUpdateQueueFailed)
	}
}
