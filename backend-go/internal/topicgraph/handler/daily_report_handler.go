package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/ws"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// RegisterDailyReportRoutes registers all daily report routes.
func RegisterDailyReportRoutes(api *gin.RouterGroup) {
	// POST /api/daily-reports/generate
	api.POST("/daily-reports/generate", triggerGenerateDailyReport)

	// GET /api/daily-reports/:id
	api.GET("/daily-reports/:id", getDailyReport)

	// GET /api/semantic-boards/:id/daily-reports
	api.GET("/semantic-boards/:id/daily-reports", listBoardDailyReports)

	// GET /api/semantic-boards/:id/section-timeline
	api.GET("/semantic-boards/:id/section-timeline", getBoardSectionTimeline)

	// GET /api/semantic-boards/:id/topics — every topic on a board (incl. archived
	// and orphans) with section counts, for the management UI.
	api.GET("/semantic-boards/:id/topics", listBoardTopics)

	// GET /api/semantic-boards/:id/lane-dynamics — 泳道动态聚合端点
	// (overview-lane-dynamics design D4): lanes + timeline + candidates +
	// has_reports in one shot. Read-only.
	api.GET("/semantic-boards/:id/lane-dynamics", getBoardLaneDynamics)

	// GET /api/daily-reports/sections/:id/lifecycle
	api.GET("/daily-reports/sections/:id/lifecycle", getSectionLifecycle)

	// GET /api/daily-reports/topics/:id/lifeline — all sections of one persistent
	// topic (no day limit), aggregated by persistent_topic_id.
	api.GET("/daily-reports/topics/:id/lifeline", getTopicLifeline)

	// PATCH /api/daily-reports/topics/:id — manual rename / archive / reactivate.
	api.PATCH("/daily-reports/topics/:id", updateTopic)
	// DELETE /api/daily-reports/topics/:id — hard-delete (irreversible; clears
	// the topic assignment on sections, sections themselves are kept).
	api.DELETE("/daily-reports/topics/:id", deleteTopic)
	// POST /api/daily-reports/topics/:id/merge — merge source topics into :id.
	api.POST("/daily-reports/topics/:id/merge", mergeTopic)
	// POST /api/daily-reports/topics/:id/split — carve sections into a new topic.
	api.POST("/daily-reports/topics/:id/split", splitTopic)

	// POST /api/daily-reports/backfill-embeddings
	api.POST("/daily-reports/backfill-embeddings", triggerBackfillEmbeddings)

	// POST /api/daily-reports/backfill-relations
	api.POST("/daily-reports/backfill-relations", triggerBackfillRelations)

	// POST /api/daily-reports/backfill-topics — reconstruct persistent topics
	// from historical sections. Optional ?board_id scopes to one board.
	api.POST("/daily-reports/backfill-topics", triggerBackfillTopics)

	// POST /api/semantic-boards/:id/persistent-topics/manual
	// Create a manual persistent topic (user-curated lane) from selected sections.
	api.POST("/semantic-boards/:id/persistent-topics/manual", createManualTopic)

	// GET /api/semantic-boards/:id/persistent-topics/compose-candidates
	// Load the candidate sections (with embeddings) for the manual-compose UI.
	api.GET("/semantic-boards/:id/persistent-topics/compose-candidates", composeCandidates)

	// POST /api/semantic-boards/:id/persistent-topics/embed-query
	// Embed a natural-language query so the compose UI can rank candidates by
	// cosine similarity. Uses the same global model as section embeddings.
	api.POST("/semantic-boards/:id/persistent-topics/embed-query", embedQuery)

	// Topic watch routes
	RegisterTopicWatchRoutes(api)
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

	// Rebuild window guard (offline-catchup design D6): a date older than the
	// tag edge retention window has had its edges reclaimed, so its candidate set
	// is incomplete — rebuilding would overwrite a possibly good existing report
	// with an empty one (same-day rebuild is a full replace). Shares the
	// predicate + wording with the scheduler's TriggerNowWithDate.
	retentionDays := tagging.LoadTagEdgeRetentionDays(repository.Repo.DB())
	if service.IsDateOutsideRebuildWindow(date, time.Now(), retentionDays) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   service.RebuildWindowRejectionMessage(retentionDays),
			"reason":  "out_of_retention_window",
		})
		return
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

	report, err := service.GenerateAndSaveReport(ctx, boardID, date)
	if err != nil {
		logging.Errorf("daily-report: generate/save failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, boardName, 0, "1/1")
		broadcastDone(jobID, 0, 1)
		return
	}
	if report == nil {
		broadcastProgress(jobID, "completed", boardID, boardName, 0, "1/1")
		broadcastDone(jobID, 0, 1)
		return
	}

	broadcastProgress(jobID, "completed", boardID, boardName, 1, "1/1")
	broadcastDone(jobID, 1, 1)
}

func generateAllBoards(date time.Time, jobID string) {
	ctx, cancel := timeoutCtx(30 * time.Minute)
	defer cancel()

	boardIDs, err := repository.Repo.CollectBoardIDsForDate(date)
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

		report, genErr := service.GenerateAndSaveReport(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate/save failed for board %d: %v", boardID, genErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			continue
		}
		if report == nil {
			broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
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

	reports, err := repository.Repo.ListReports(uint(boardID), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	if reports == nil {
		reports = []repository.ReportListItem{}
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

	report, err := repository.Repo.GetReportByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "report not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"report": report}})
}

// getBoardSectionTimeline handles GET /api/semantic-boards/:id/section-timeline
func getBoardSectionTimeline(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}
	// days 缺省或 <=0 → 全部历史；显式传 7/14/30 则按天窗口。
	daysStr := c.Query("days")
	days := 0
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = d
		}
	}

	resp, err := repository.Repo.GetBoardSectionTimeline(uint(boardID), days)
	if err != nil {
		logging.Errorf("get board section timeline: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to get section timeline"})
		return
	}
	if resp.Sections == nil {
		resp.Sections = []repository.SectionTimelineNode{}
	}
	if resp.Relations == nil {
		resp.Relations = []repository.SectionRelationResult{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"sections":  resp.Sections,
		"relations": resp.Relations,
	}})
}

// listBoardTopics handles GET /api/semantic-boards/:id/topics.
// Returns every persistent topic on the board (active, candidate, AND archived,
// plus orphans that no section references) with a per-topic section_count for
// the management UI. Unlike section-timeline, this is not windowed — it exists
// precisely so anomalous topics produced by backfill bugs are visible and
// manageable.
func listBoardTopics(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}
	topics, err := repository.Repo.ListTopicsByBoardAll(uint(boardID))
	if err != nil {
		logging.Errorf("list board topics: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to list topics"})
		return
	}
	// Aggregate section counts per topic in one query (left-joined topics show 0).
	countMap, err := repository.Repo.CountSectionsByTopic()
	if err != nil {
		logging.Errorf("list board topics: count sections: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to count sections"})
		return
	}
	upgradeThreshold := repository.LoadPersistentTopicConfig(repository.Repo.DB()).UpgradeThreshold
	// Filter out observing candidates (hit_count < upgrade_threshold).
	topics = repository.FilterVisibleTopics(topics, upgradeThreshold)
	type topicListItem struct {
		repository.BoardPersistentTopic
		SectionCount int    `json:"section_count"`
		Color        string `json:"color"`
		CanActivate  bool   `json:"can_activate"`
	}
	items := make([]topicListItem, 0, len(topics))
	for _, t := range topics {
		items = append(items, topicListItem{
			BoardPersistentTopic: t,
			SectionCount:         countMap[t.ID],
			Color:                repository.PersistentTopicColor(t.ID),
			CanActivate:          t.Status == repository.TopicStatusCandidate && t.HitCount >= upgradeThreshold,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"topics": items}})
}

// getSectionLifecycle handles GET /api/daily-reports/sections/:id/lifecycle
func getSectionLifecycle(c *gin.Context) {
	sectionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid section id"})
		return
	}

	resp, err := repository.Repo.GetSectionLifecycle(uint(sectionID))
	if err != nil {
		logging.Errorf("get section lifecycle: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to get section lifecycle"})
		return
	}
	if resp.Sections == nil {
		resp.Sections = []repository.SectionTimelineNode{}
	}
	if resp.Relations == nil {
		resp.Relations = []repository.SectionRelationResult{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"sections":  resp.Sections,
		"relations": resp.Relations,
	}})
}

// getTopicLifeline handles GET /api/daily-reports/topics/:id/lifeline.
// Returns all sections of a persistent topic aggregated by topic id, with the
// internal relations among them. Unlike section lifecycle (embedding-graph
// based), this is identity-key based and survives label drift.
func getTopicLifeline(c *gin.Context) {
	topicIDStr := c.Param("id")
	topicID, err := strconv.ParseUint(topicIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid topic id"})
		return
	}

	resp, err := repository.Repo.GetTopicLifeline(uint(topicID))
	if err != nil {
		logging.Errorf("get topic lifeline: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to get topic lifeline"})
		return
	}
	if resp.Sections == nil {
		resp.Sections = []repository.SectionTimelineNode{}
	}
	if resp.Relations == nil {
		resp.Relations = []repository.SectionRelationResult{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"sections":  resp.Sections,
		"relations": resp.Relations,
	}})
}

// parseTopicID extracts and validates the :id path param as a topic id. Writes
// the error response when invalid, so callers return immediately on err.
func parseTopicID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid topic id"})
		return 0, err
	}
	return uint(id), nil
}

// updateTopic handles PATCH /api/daily-reports/topics/:id.
// Body: {"label": "...", "status": "active|archived"} — either field optional;
// omit a field to leave it unchanged. status is restricted to active/archived.
func updateTopic(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}
	var req struct {
		Label  *string `json:"label"`
		Status *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	topic, err := repository.Repo.UpdateTopic(topicID, req.Label, req.Status)
	if err != nil {
		logging.Errorf("update topic %d: %v", topicID, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": topic})
}

// mergeTopic handles POST /api/daily-reports/topics/:id/merge.
// Body: {"source_topic_ids": [2, 3]} — every section on those topics is
// reassigned to :id and the sources are archived.
func mergeTopic(c *gin.Context) {
	targetID, err := parseTopicID(c)
	if err != nil {
		return
	}
	var req struct {
		SourceTopicIDs []uint `json:"source_topic_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	topic, err := repository.Repo.MergeTopics(targetID, req.SourceTopicIDs)
	if err != nil {
		logging.Errorf("merge into topic %d: %v", targetID, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": topic})
}

// splitTopic handles POST /api/daily-reports/topics/:id/split.
// Body: {"section_ids": [10, 11], "label": "..."} — the listed sections are
// carved out of :id into a freshly created topic.
func splitTopic(c *gin.Context) {
	sourceID, err := parseTopicID(c)
	if err != nil {
		return
	}
	var req struct {
		SectionIDs []uint `json:"section_ids"`
		Label      string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	topic, err := repository.Repo.SplitTopic(sourceID, req.SectionIDs, req.Label)
	if err != nil {
		logging.Errorf("split topic %d: %v", sourceID, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": topic})
}

// deleteTopic handles DELETE /api/daily-reports/topics/:id.
// Hard-deletes the topic and unlinks its sections (sections keep their content,
// only the topic assignment is cleared). This is irreversible; the reversible
// path is PATCH with status=archived.
func deleteTopic(c *gin.Context) {
	topicID, err := parseTopicID(c)
	if err != nil {
		return
	}
	if err := repository.Repo.DeleteTopic(topicID); err != nil {
		logging.Errorf("delete topic %d: %v", topicID, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
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

// triggerBackfillEmbeddings handles POST /api/daily-reports/backfill-embeddings
// Query: recompute (bool, default false), board_id (optional), since_days
// (optional, recompute only; default 30, 0 = unlimited).
func triggerBackfillEmbeddings(c *gin.Context) {
	recompute := c.Query("recompute") == "true"
	var boardID *uint
	if v := c.Query("board_id"); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board_id"})
			return
		}
		uid := uint(id)
		boardID = &uid
	}
	sinceDays := 30
	if v := c.Query("since_days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid since_days"})
			return
		}
		sinceDays = n
	}

	// WithTimeout only after validation: early-return paths (400) never spawn
	// the goroutine, so the context must not be created before them.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	go func() {
		// cancel must live inside the worker goroutine: a handler-scoped
		// `defer cancel()` fires the moment this handler returns (right after
		// the "processing" JSON), canceling the async backfill's first embed
		// call (observed: `Post ...: context canceled`).
		defer cancel()
		embedded, skipped, matched, err := repository.Repo.BackfillSectionEmbeddings(ctx, recompute, boardID, sinceDays)
		if err != nil {
			logging.Errorf("daily-report: backfill failed: %v", err)
			return
		}
		logging.Infof("daily-report: backfill complete: %d sections embedded, %d skipped, %d sections matched", embedded, skipped, matched)
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"status": "processing",
		},
	})
}

// triggerBackfillRelations handles POST /api/daily-reports/backfill-relations
func triggerBackfillRelations(c *gin.Context) {
	boardIDStr := c.Query("board_id")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if boardIDStr != "" {
		boardID, err := strconv.ParseUint(boardIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid board_id"})
			return
		}
		go func() {
			<-ctx.Done() // wait for response to be sent
			rebuilt, err := repository.Repo.BackfillRelations(uint(boardID))
			if err != nil {
				logging.Errorf("daily-report: backfill relations for board %d failed: %v", boardID, err)
				return
			}
			logging.Infof("daily-report: backfill relations for board %d complete: %d relations rebuilt", boardID, rebuilt)
		}()
	} else {
		go func() {
			<-ctx.Done()
			results, err := repository.Repo.BackfillAllRelations()
			if err != nil {
				logging.Errorf("daily-report: backfill all relations failed: %v", err)
				return
			}
			for bid, cnt := range results {
				logging.Infof("daily-report: board %d: %d relations rebuilt", bid, cnt)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "processing"}})
}

// triggerBackfillTopics handles POST /api/daily-reports/backfill-topics.
// Reconstructs persistent topics from historical sections lacking a topic
// assignment. Optional ?board_id scopes the run to one board; without it, all
// boards with unassigned sections are processed.
func triggerBackfillTopics(c *gin.Context) {
	boardIDStr := c.Query("board_id")
	_, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if boardIDStr != "" {
		boardID, err := strconv.ParseUint(boardIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid board_id"})
			return
		}
		go func() {
			created, err := repository.Repo.BackfillPersistentTopics(uint(boardID))
			if err != nil {
				logging.Errorf("daily-report: backfill topics for board %d failed: %v", boardID, err)
				return
			}
			logging.Infof("daily-report: backfill topics for board %d complete: %d topics created", boardID, created)
		}()
	} else {
		go func() {
			results, err := repository.Repo.BackfillAllPersistentTopics()
			if err != nil {
				logging.Errorf("daily-report: backfill all topics failed: %v", err)
				return
			}
			for bid, cnt := range results {
				logging.Infof("daily-report: board %d: %d topics created", bid, cnt)
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "processing"}})
}

func dailyReportBoardName(boardID uint) string {
	if boardID == 0 {
		return "All boards"
	}
	var board models.SemanticLabel
	if err := repository.Repo.DB().Select("label").Where("id = ?", boardID).First(&board).Error; err != nil {
		return fmt.Sprintf("Board #%d", boardID)
	}
	return board.Label
}

func timeoutCtx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// createManualTopic handles POST /api/semantic-boards/:id/persistent-topics/manual.
// Body: {"label": "...", "section_ids": [1,2,3]}
// Creates a new persistent topic with source=manual from the selected sections'
// mean embedding, then reassigns those sections to the new topic.
func createManualTopic(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}

	var req struct {
		Label      string `json:"label"`
		SectionIDs []uint `json:"section_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	if req.Label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "label is required"})
		return
	}

	topic, skipped, err := repository.Repo.CreateManualTopic(uint(boardID), req.Label, req.SectionIDs)
	if err != nil {
		logging.Errorf("create manual topic: board=%d label=%q: %v", boardID, req.Label, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	resp := gin.H{
		"success": true,
		"data": gin.H{
			"topic":   topic,
			"skipped": skipped,
		},
	}
	if len(skipped) > 0 {
		resp["message"] = fmt.Sprintf("%d 条 section 因无向量被跳过", len(skipped))
	}
	c.JSON(http.StatusOK, resp)
}

// composeCandidates handles GET
// /api/semantic-boards/:id/persistent-topics/compose-candidates?days=N.
// Returns the in-window sections that carry a usable embedding (parsed to a
// float slice) plus the configured match_threshold, so the compose UI can
// compute aggregate anchors / outlier distances client-side in real time.
func composeCandidates(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}
	days := 0
	if daysStr := c.Query("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil {
			days = d
		}
	}
	resp, err := repository.Repo.GetComposeCandidates(uint(boardID), days)
	if err != nil {
		logging.Errorf("compose candidates: board=%d: %v", boardID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load compose candidates"})
		return
	}
	if resp.Sections == nil {
		resp.Sections = []repository.ComposeCandidateSection{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// embedText encodes a natural-language query with the global CapabilityEmbedding
// model — the same model used for section embeddings, so cosine similarity
// between a query vector and candidate embeddings is meaningful. Package-level
// variable so tests can inject a mock (real model calls can't run in unit tests);
// mirrors the embedFunc injection pattern in daily_report_thread_fit.go.
var embedText = func(ctx context.Context, query string) ([]float64, error) {
	result, err := airouter.NewRouter().Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{query},
		Operation: "section.embedding",
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("embed query: empty result")
	}
	return result.Embeddings[0], nil
}

// embedQuery handles POST
// /api/semantic-boards/:id/persistent-topics/embed-query.
// Body: {"query": "..."} → {"embedding": [...]}
//
// Powers the compose-UI candidate search: the frontend embeds the user's
// natural-language query here, then ranks candidate sections by cosine
// similarity to this vector (and, once candidates are selected, by similarity
// to the selected set's aggregate anchor). The embedding model is global
// (CapabilityEmbedding), so :id is a route placeholder only — it keeps the
// endpoint grouped with the compose-candidates/manual siblings in the route tree.
func embedQuery(c *gin.Context) {
	var req struct {
		Query string `json:"query"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "query is required"})
		return
	}

	ctx, cancel := timeoutCtx(30 * time.Second)
	defer cancel()
	vec, err := embedText(ctx, req.Query)
	if err != nil {
		logging.Errorf("embed query: board=%s query=%q: %v", c.Param("id"), req.Query, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to embed query"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"embedding": vec}})
}
