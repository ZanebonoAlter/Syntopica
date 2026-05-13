package tagging

import (
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

var queueService *EmbeddingQueueService
var queueServiceOnce sync.Once

func getQueueService() *EmbeddingQueueService {
	queueServiceOnce.Do(func() {
		queueService = NewEmbeddingQueueService(nil)
	})
	return queueService
}

func GetEmbeddingQueueStatus(c *gin.Context) {
	status, err := getQueueService().GetStatus()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": status})
}

func GetEmbeddingQueueTasks(c *gin.Context) {
	status := c.Query("status")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	tasks, total, err := getQueueService().GetTasks(status, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"tasks": tasks,
			"total": total,
		},
	})
}

func RetryEmbeddingQueueFailed(c *gin.Context) {
	count, err := getQueueService().RetryFailed()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "已重试 " + strconv.FormatInt(count, 10) + " 个失败任务",
	})
}

func BackfillPersonMetadataHandler(c *gin.Context) {
	processed, err := BackfillPersonMetadata()
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"processed": processed}})
}

func RegisterEmbeddingQueueRoutes(rg *gin.RouterGroup) {
	queue := rg.Group("/embedding/queue")
	{
		queue.GET("/status", GetEmbeddingQueueStatus)
		queue.GET("/tasks", GetEmbeddingQueueTasks)
		queue.POST("/retry", RetryEmbeddingQueueFailed)
		queue.POST("/person-metadata/backfill", BackfillPersonMetadataHandler)
	}
}

func StartEmbeddingQueueWorker() {
	getQueueService().Start()
}

func StopEmbeddingQueueWorker() {
	getQueueService().Stop()
}
