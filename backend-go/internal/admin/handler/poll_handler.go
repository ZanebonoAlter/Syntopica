package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"syntopica-backend/internal/platform/notification"
	tagging "syntopica-backend/internal/tagmanagement"
)

// GetPollBundle handles GET /api/poll — the single reconciliation endpoint for
// the frontend's resident status data (scheduler states + tag-queue counters +
// embedding-queue counters + unread count). One request instead of independent
// polling loops (openspec: client-poll-budget / 常驻状态数据由单一合并入口承载).
//
// Reuses the exact queries of the legacy endpoints (they stay registered
// for already-open tabs), so the server cost per poll is the sum of a few
// cheap reads — no new heavy work.
func GetPollBundle(c *gin.Context) {
	schedulers := make([]SchedulerStatusResponse, 0)
	for _, key := range orderedSchedulerNames() {
		s, ok := lookupScheduler(key)
		if !ok {
			continue
		}
		if status := safeGetStatus(s, schedulerLabel(s, key)); status != nil {
			enrichStatus(s, key, status)
			schedulers = append(schedulers, *status)
		}
	}

	queue, err := tagging.TagQueueStatusSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	// embedding 队列计数：日报自动生成要求 tag_jobs 与 embedding_queues
	// **双队列**清空（lane 分桶依赖 tag embedding），只给标签队列会让人误以为
	// 「队列已清空」→ 定时任务应该已经出报错了。单位是标签（非文章）。
	embeddingQueue, err := tagging.EmbeddingQueueStatusSnapshot()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	unread, err := notification.UnreadCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	payload := buildSchedulerStatusPayload(schedulers)
	// Design D4 shape: data carries the three sections; the scheduler block's
	// top-level semantics (analysis_paused / ai_healthy / routes) stay at the
	// response top level so the frontend distributor reads them unchanged.
	payload["data"] = gin.H{
		"schedulers":      payload["data"],
		"tag_queue":       queue,
		"embedding_queue": embeddingQueue,
		"notifications":   gin.H{"unread": unread},
	}
	payload["server_time"] = time.Now().Format(time.RFC3339)

	c.JSON(http.StatusOK, payload)
}
