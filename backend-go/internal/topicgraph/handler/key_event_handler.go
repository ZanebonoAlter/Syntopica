package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// ── 关键事件注记 HTTP 端点（add-topic-key-event-markers design D2）──────────
//
//	GET    /api/daily-reports/:report_id/key-events
//	       → 当期来源映射（按 source_thread_id 关联）及推荐状态 idle/running/done/failed
//	GET    /api/daily-reports/topics/:id/key-events?days=14&state=confirmed
//	       → 7/14/30/全部窗口（days=0 全部）；topic 读端默认且只允许 confirmed，
//	         显式非 confirmed 的 state → 400（日报读端才允许待处理状态）
//	GET    /api/semantic-boards/:id/key-events?days=14
//	       → 版块级批量读（add-key-event-matrix-view design D3）：窗口内全部
//	         confirmed 注记扁平列表，前端按 topic+date 聚合；只读、无写动作；
//	         days 严格正整数（0 → 400，与 topic 端点 days=0=全部的旧契约不同）
//	PUT    /api/daily-reports/topics/:id/key-events/threads/:thread_id
//	       → body {action: mark|dismiss, expected_revision?: int}；服务端解析来源，
//	         不接受客户端文案；同目标幂等，旧 revision → 409 附最新状态
//	PATCH  /api/daily-reports/topics/:id/key-events/:marker_id
//	       → body {action: mark|dismiss|unmark, revision: int}
//
// 人工动作零 AI / 零研究调用：本文件只读写注记表与读推荐状态，不触发任何
// LLM 调用、不启动信号发现/研究任务。

// RegisterKeyEventRoutes wires the key-event endpoints under /api.
// NOTE: the report-scoped route uses gin's existing ":id" wildcard name —
// gin forbids mixing param names at the same path position
// (/api/daily-reports/:id already exists).
func RegisterKeyEventRoutes(api *gin.RouterGroup) {
	api.GET("/daily-reports/:id/key-events", getReportKeyEventMarkers)
	api.GET("/daily-reports/topics/:id/key-events", listTopicKeyEventMarkers)
	api.PUT("/daily-reports/topics/:id/key-events/threads/:thread_id", putThreadKeyEvent)
	api.PATCH("/daily-reports/topics/:id/key-events/:marker_id", patchKeyEventMarker)
	// 版块级批量读（add-key-event-matrix-view design D3）：矩阵级关键事件视图一次
	// 取全观察窗内 confirmed 注记；与 /semantic-boards/:id/... 同前缀（param 名同为 id）。
	api.GET("/semantic-boards/:id/key-events", listBoardKeyEventMarkers)
}

// keyEventStatusFromRun derives the report's recommendation status:
// idle (no run row) / running (live in-process) / done / failed. A persisted
// "running" row whose task is NOT alive in this process means a restart
// killed it — degrade honestly to failed (design D3: 重启不自动恢复付费调用).
func keyEventStatusFromRun(run *repository.KeyEventRecommendationRun) string {
	if run == nil {
		return "idle"
	}
	if run.State == repository.KeyEventRunStateRunning && !service.IsKeyEventRecommendationActive(run.ReportID) {
		return "failed"
	}
	return run.State
}

// getReportKeyEventMarkers handles GET /api/daily-reports/:id/key-events.
func getReportKeyEventMarkers(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report id"})
		return
	}
	report, err := repository.Repo.GetReportByID(uint(reportID))
	if err != nil || report == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "report not found"})
		return
	}
	markers, err := repository.Repo.ListReportKeyEventMarkers(uint(reportID))
	if err != nil {
		logging.Errorf("list report key events: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list key events"})
		return
	}
	run, err := repository.Repo.GetKeyEventRecommendationRun(uint(reportID))
	if err != nil {
		logging.Errorf("load key event run: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load recommendation state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"status":  keyEventStatusFromRun(run),
		"markers": markers,
	}})
}

// parseKeyEventDays validates the days query param: absent → default 14;
// present → non-negative integer (0 = all time); anything else → 400 (不误当
// 全部，test-cases S4).
func parseKeyEventDays(c *gin.Context) (int, bool) {
	raw := c.Query("days")
	if raw == "" {
		return 14, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "days must be a non-negative integer"})
		return 0, false
	}
	return days, true
}

// listTopicKeyEventMarkers handles GET /api/daily-reports/topics/:id/key-events.
func listTopicKeyEventMarkers(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}
	// Topic read endpoint: default AND only confirmed (spec: 总览不承担待处理
	// 状态；日报读端经 report 端点拿 suggested).
	if state := c.Query("state"); state != "" && state != repository.KeyEventStateConfirmed {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "topic read endpoint only supports state=confirmed"})
		return
	}
	days, ok := parseKeyEventDays(c)
	if !ok {
		return
	}
	var topic repository.BoardPersistentTopic
	if err := repository.Repo.DB().First(&topic, topicID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "topic not found"})
		return
	}
	markers, err := repository.Repo.ListTopicKeyEventMarkers(topicID, days, repository.KeyEventStateConfirmed)
	if err != nil {
		logging.Errorf("list topic key events: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list key events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"topic_id": topicID,
		"days":     days,
		"markers":  markers,
	}})
}

// parseBoardKeyEventDays validates the days query param for the BOARD-level
// batch read: absent → default 14; present → STRICTLY positive integer.
// Unlike the per-topic endpoint (parseKeyEventDays, where 0 = all time is a
// legacy contract), this endpoint's delta spec (topic-key-event-markers
// 「非法参数」 scenario) requires 0 to be a 4xx — 矩阵级视图永远在有限观察窗内
// 取数，不存在「全部时间」档。
func parseBoardKeyEventDays(c *gin.Context) (int, bool) {
	raw := c.Query("days")
	if raw == "" {
		return 14, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "days must be a positive integer"})
		return 0, false
	}
	return days, true
}

// listBoardKeyEventMarkers handles GET /api/semantic-boards/:id/key-events?days=N
// (add-key-event-matrix-view design D3): one-shot confirmed-only read for the
// board-level key-event timeline view. Flattened markers (frontend aggregates
// by topic+date); suggested/dismissed never enter the payload. days is a
// strictly positive window (default 14; 0/negative/non-numeric → 400). Read-only.
func listBoardKeyEventMarkers(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}
	days, ok := parseBoardKeyEventDays(c)
	if !ok {
		return
	}
	var board models.SemanticLabel
	if err := repository.Repo.DB().
		Where("id = ? AND label_type = ?", boardID, "board").
		First(&board).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "board not found"})
		return
	}
	today := time.Now().In(models.ShanghaiTZ)
	to := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, models.ShanghaiTZ)
	from := to.AddDate(0, 0, -(days - 1))
	markers, err := repository.Repo.ListBoardKeyEventMarkers(uint(boardID), from, to)
	if err != nil {
		logging.Errorf("list board key events: board=%d: %v", boardID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list key events"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"markers": markers,
	}})
}

// keyEventActionBody is the shared request body of PUT/PATCH. The client can
// ONLY express an action and the revision it read — never snapshot text
// (伪造快照拒绝: unknown fields are ignored, snapshots are server-derived).
type keyEventActionBody struct {
	Action           string `json:"action"`
	ExpectedRevision *int   `json:"expected_revision"`
	Revision         *int   `json:"revision"`
}

// putThreadKeyEvent handles PUT /api/daily-reports/topics/:id/key-events/threads/:thread_id.
func putThreadKeyEvent(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}
	threadID, err := strconv.ParseUint(c.Param("thread_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid thread id"})
		return
	}
	var body keyEventActionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	// 人工动作零 AI：直接走注记表，不触发任何推荐/研究调用。
	marker, err := repository.Repo.ApplyThreadKeyEvent(topicID, uint(threadID), body.Action, body.ExpectedRevision)
	keyEventRespondAction(c, marker, err)
}

// patchKeyEventMarker handles PATCH /api/daily-reports/topics/:id/key-events/:marker_id.
func patchKeyEventMarker(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}
	markerID, err := strconv.ParseUint(c.Param("marker_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid marker id"})
		return
	}
	var body keyEventActionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	marker, err := repository.Repo.ApplyMarkerKeyEvent(topicID, uint(markerID), body.Action, body.Revision)
	keyEventRespondAction(c, marker, err)
}

// keyEventRespondAction maps KeyEventActionError to 400/404/409 (with the
// latest marker on 409 for client re-read) and success to the envelope.
func keyEventRespondAction(c *gin.Context, marker *repository.TopicKeyEventMarker, err error) {
	if err != nil {
		if kea, ok := err.(*repository.KeyEventActionError); ok {
			body := gin.H{"success": false, "error": kea.Message, "reason": kea.Reason}
			if kea.Marker != nil {
				body["data"] = kea.Marker
			}
			c.JSON(kea.HTTPStatus, body)
			return
		}
		logging.Errorf("key event action: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "key event action failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": marker})
}
