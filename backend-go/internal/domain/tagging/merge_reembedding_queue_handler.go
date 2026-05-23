package tagging

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

var mergeQueueService *MergeReembeddingQueueService
var mergeQueueServiceOnce sync.Once

func getMergeReembeddingQueueService() *MergeReembeddingQueueService {
	mergeQueueServiceOnce.Do(func() {
		mergeQueueService = NewMergeReembeddingQueueService(nil)
	})
	return mergeQueueService
}

func GetMergeReembeddingQueueStatus(c *gin.Context) {
	status, err := getMergeReembeddingQueueService().GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": status})
}

func GetMergeReembeddingQueueTasks(c *gin.Context) {
	status := c.Query("status")
	if status != "" && !isValidMergeReembeddingStatus(status) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid status"})
		return
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 || limit > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "limit must be between 1 and 200"})
		return
	}

	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "offset must be zero or greater"})
		return
	}

	tasks, total, err := getMergeReembeddingQueueService().GetTasks(status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tasks": tasks,
			"total": total,
		},
	})
}

func RetryMergeReembeddingQueueFailed(c *gin.Context) {
	count, err := getMergeReembeddingQueueService().RetryFailed()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已重试 " + strconv.FormatInt(count, 10) + " 个失败任务",
	})
}

func RegisterMergeReembeddingQueueRoutes(rg *gin.RouterGroup) {
	queue := rg.Group("/embedding/merge-reembedding")
	{
		queue.GET("/status", GetMergeReembeddingQueueStatus)
		queue.GET("/tasks", GetMergeReembeddingQueueTasks)
		queue.POST("/retry", RetryMergeReembeddingQueueFailed)
	}
}

func StartMergeReembeddingQueueWorker() {
	getMergeReembeddingQueueService().Start()
}

func StopMergeReembeddingQueueWorker() {
	getMergeReembeddingQueueService().Stop()
}

func isValidMergeReembeddingStatus(status string) bool {
	switch status {
	case "pending", "processing", "completed", "failed":
		return true
	default:
		return false
	}
}
