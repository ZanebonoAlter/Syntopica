package handler

import (
	"context"
	"encoding/json"
	"errors"
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
	"syntopica-backend/internal/platform/notification"
	"syntopica-backend/internal/platform/ws"
	tagging "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph/repository"
	"syntopica-backend/internal/topicgraph/service"
)

// RegisterDailyReportRoutes registers all daily report routes.
func RegisterDailyReportRoutes(api *gin.RouterGroup) {
	// POST /api/daily-reports/:id/rebucket — re-run intra-lane theme bucketing
	// for one bucket group (add-intra-lane-theme-bucketing).
	api.POST("/daily-reports/:id/rebucket", rebucketDailyReportSection)

	// POST /api/daily-reports/generate
	api.POST("/daily-reports/generate", triggerGenerateDailyReport)

	// GET /api/semantic-boards/overview — 今日雷达单次聚合（8.2）。
	api.GET("/semantic-boards/overview", getBoardRadarOverview)

	// GET /api/daily-reports/today — reading-desk queue: boards with a report today.
	api.GET("/daily-reports/today", listTodayReports)

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

	// 日报页边注（daily-report-margin-notes）：批注四端点 + 管理页列表
	RegisterMarginNoteRoutes(api)

	// 话题关键事件注记（add-topic-key-event-markers）：读/写/维护端点
	RegisterKeyEventRoutes(api)
}

// triggerGenerateDailyReport handles POST /api/daily-reports/generate
func triggerGenerateDailyReport(c *gin.Context) {
	var req struct {
		Date     string `json:"date"`
		BoardID  *uint  `json:"board_id"`
		BoardIDs []uint `json:"board_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Any field with an illegal type is rejected outright: a malformed
		// request SHALL NOT degrade to the all-boards scope. {"board_ids": {...}}
		// and {"board_id": "5"} used to fall through to the lenient branch below,
		// leaving BoardID nil → a full-site generation pipeline (one typo would
		// rebuild every board). A missing `date` is NOT a type error — it stays
		// on the lenient path.
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			field := typeErr.Field
			expected := "the declared field type"
			switch field {
			case "board_id":
				expected = "a number"
			case "board_ids":
				expected = "an array of numbers"
			case "date":
				expected = "a YYYY-MM-DD string"
			default:
				if field == "" {
					// Not a JSON object at the top level (array / scalar).
					field = "request body"
					expected = "a JSON object"
				}
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("invalid %s: expected %s, got %s", field, expected, typeErr.Value),
			})
			return
		}
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

	// Scope exclusivity (add-radar-pipeline-status D1): board_id (one board)
	// and board_ids (a board collection) are two different scopes of the same
	// endpoint — accepting both is ambiguous. Rejected BEFORE the rebuild
	// window guard and before the generation gate is touched, so a malformed
	// request never starts a pipeline nor occupies the pipeline slot.
	if req.BoardID != nil && len(req.BoardIDs) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "board_id 与 board_ids 不能同时指定：请二选一（单板或板块集合）",
			"reason":  "conflicting_board_scope",
		})
		return
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

	// Generation gate (guard-daily-report-generate D1/D2): mutual exclusion
	// with the scheduled pipeline — at most ONE generation pipeline
	// process-wide. Checked AFTER the rebuild-window guard so an out-of-window
	// date never touches the gate. Same wording as TriggerNowWithDate.
	if !tryAcquireGenerationGate() {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "日报生成正在执行中，请稍后再试。",
			"reason":  "already_running",
		})
		return
	}

	jobID := uuid.New().String()

	switch {
	case len(req.BoardIDs) > 0:
		// Multi-board collection scope (add-radar-pipeline-status D2/D3): the
		// requested set is generated as-is — NOT intersected with
		// CollectBoardIDsForDate, so a board with no content today completes
		// with 0 entries instead of being silently skipped.
		go generateSelectedBoards(date, dedupeBoardIDs(req.BoardIDs), jobID)
	case req.BoardID != nil:
		// Generate for single board
		go generateSingleBoard(*req.BoardID, date, jobID)
	default:
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

// notifyDailyReportTerminal is the single adjudication point for the
// daily-report terminal-state notification (test-cases 白盒 B):
// failedCount==0 → one completion notification; failedCount>0 → ONE failure
// summary (success/failed counts in the copy), mutually exclusive with the
// completion notification. Never more than one notification per run.
//
// 计数口径与 daily_report_done 终态消息同源（add-radar-pipeline-status D12）：
// totalBoards == WS 的 total_boards（**含**当日无内容的板块），
// successCount == total_saved、failedCount == failed_count、
// emptyCount == empty_count。当日无内容的板块计入总版面数，但既不进成功数、
// 也不进失败数——通知与 WS 说的永远是同一件事。
func notifyDailyReportTerminal(date time.Time, totalBoards, successCount, failedCount, emptyCount int) {
	notification.DailyReportTerminal(date, notification.DailyReportCounts{
		TotalBoards: totalBoards,
		Saved:       successCount,
		Failed:      failedCount,
		Empty:       emptyCount,
	})
}

// Generation gate seams (guard-daily-report-generate D1/D2): the manual
// endpoint shares the process-wide daily-report generation gate with the
// scheduled DailyReportJob (the gate itself lives in service/; the topicgraph
// root package re-exports it for the admin scheduler). Package-level vars so
// tests can stub the gate without side effects.
var (
	tryAcquireGenerationGate = service.TryAcquireDailyReportGeneration
	releaseGenerationGate    = service.ReleaseDailyReportGeneration
)

// generateReportEntry is the generation entry point used by the manual
// endpoint's worker goroutines. A package-level variable so tests can
// substitute a stub (same seam pattern as the scheduler's
// generateAndSaveReport): the real implementation drives the whole LLM
// pipeline.
var generateReportEntry = service.GenerateAndSaveReport

// dailyReportCounts is the terminal tally of one daily-report run, split into
// the three mutually exclusive board states:
//
//   - saved:  板块生成成功（有日报产出）
//   - failed: 板块生成失败（生成报错）
//   - empty:  板块当日无内容（report == nil）——既不是成功也不是失败，
//     与通知中心口径一致（DailyReportTerminal：无内容的板块不进成功数、
//     不进失败数，但计入总版面数）
//
// 三态互斥且穷尽：本次运行涉及的每个板块恰好落入其中一态，因此
// saved + failed + empty == total_boards 恒成立。终态 WS 消息直接带这三个
// 计数，消费方 SHALL NOT 用 total_boards - total_saved 反推失败数（那会把
// 「当日无内容」的板块错算成失败）。
type dailyReportCounts struct {
	saved  int
	failed int
	empty  int
}

func generateSingleBoard(boardID uint, date time.Time, jobID string) {
	// Gate release (guard-daily-report-generate): this goroutine owns the
	// generation gate acquired by the handler; deferred FIRST so every exit
	// path — success, failure, or panic unwind — releases it.
	defer releaseGenerationGate()

	ctx, cancel := timeoutCtx(10 * time.Minute)
	defer cancel()

	boardName := dailyReportBoardName(boardID)
	broadcastProgress(jobID, "generating", boardID, boardName, 0, "0/1")

	report, err := generateReportEntry(ctx, boardID, date)
	if err != nil {
		logging.Errorf("daily-report: generate/save failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, boardName, 0, "1/1")
		// 三态计数与终态通知同口径：失败 1（该板块真失败）· 无内容 0。
		broadcastDone(jobID, dailyReportCounts{failed: 1}, 1)
		// 单版面项目：失败 1 = 全部失败 → 1 条失败汇总（白盒 B3/B 边界）
		notifyDailyReportTerminal(date, 1, 0, 1, 0)
		return
	}
	if report == nil {
		broadcastProgress(jobID, "completed", boardID, boardName, 0, "1/1")
		// 版面无内容不算失败：终态完成（保存 0 条）——计入 empty，不计失败。
		broadcastDone(jobID, dailyReportCounts{empty: 1}, 1)
		// 版面无内容不算失败：终态完成（保存 0 条）；通知走成功类，且
		// 「共 1 个版面」含这个当日无内容的板块。
		notifyDailyReportTerminal(date, 1, 0, 0, 1)
		return
	}

	broadcastProgress(jobID, "completed", boardID, boardName, 1, "1/1")
	broadcastDone(jobID, dailyReportCounts{saved: 1}, 1)
	notifyDailyReportTerminal(date, 1, 1, 0, 0)
}

func generateAllBoards(date time.Time, jobID string) {
	// Gate release (guard-daily-report-generate): this goroutine owns the
	// generation gate acquired by the handler; deferred FIRST so every exit
	// path — success, failure, or panic unwind — releases it.
	defer releaseGenerationGate()

	ctx, cancel := timeoutCtx(30 * time.Minute)
	defer cancel()

	boardIDs, err := repository.Repo.CollectBoardIDsForDate(date)
	if err != nil {
		logging.Errorf("daily-report: collect boards failed: %v", err)
		broadcastProgress(jobID, "failed", 0, "All boards", 0, "0/0")
		// 收集阶段即失败：终态计数与终态通知同口径（通知把这次收集失败
		// 记作「共 1 个版面，0 成功 1 失败」）——记 1 个失败板块，
		// 恒等式 saved + failed + empty == total_boards 仍成立。
		broadcastDone(jobID, dailyReportCounts{failed: 1}, 1)
		// 收集阶段即失败：一次失败汇总（B3 同型，至多一条）；总版面数与
		// 终态消息的 total_boards 同为 1。
		notifyDailyReportTerminal(date, 1, 0, 1, 0)
		return
	}

	totalBoards := len(boardIDs)
	if totalBoards == 0 {
		// 当日无任何可生成板块：只广播终态，不发通知（无可通知的版面）。
		broadcastDone(jobID, dailyReportCounts{}, 0)
		return
	}

	savedCount := 0
	failedCount := 0
	emptyCount := 0
	for idx, boardID := range boardIDs {
		boardName := dailyReportBoardName(boardID)
		broadcastProgress(jobID, "generating", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx, totalBoards))

		report, genErr := generateReportEntry(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate/save failed for board %d: %v", boardID, genErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			failedCount++
			continue
		}
		if report == nil {
			broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			// 当日无内容：既不算成功也不算失败（与通知中心口径一致）
			emptyCount++
			continue
		}
		savedCount++
		broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
	}

	broadcastDone(jobID, dailyReportCounts{saved: savedCount, failed: failedCount, empty: emptyCount}, totalBoards)
	// 终态通知（白盒 B1/B2/B3）：failed==0 → 完成通知；≥1 → 一条失败汇总，互斥。
	// 计数与上面那次终态广播同源（三态：成功 / 失败 / 当日无内容；总版面数含无内容板块）。
	notifyDailyReportTerminal(date, totalBoards, savedCount, failedCount, emptyCount)
}

// dedupeBoardIDs collapses a requested board collection into a set (first
// occurrence wins, request order preserved). A repeated id would run the same
// board twice inside one pipeline: the report write is idempotent, but the LLM
// cost is paid twice and the board would be double-counted in the terminal
// counts.
func dedupeBoardIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

// generateSelectedBoards generates the daily report for an explicit board
// collection (add-radar-pipeline-status D2/D3/D4). Isomorphic to
// generateAllBoards: one goroutine, sequential per board, per-board progress
// broadcast, then ONE terminal broadcast + ONE terminal notification. The
// board set comes from the request and is NOT filtered by
// CollectBoardIDsForDate (a board without content today is not a failure —
// it completes with 0 entries, same as the single-board path).
func generateSelectedBoards(date time.Time, boardIDs []uint, jobID string) {
	// Gate release (guard-daily-report-generate): this goroutine owns the
	// generation gate acquired by the handler; deferred FIRST so every exit
	// path — success, failure, or panic unwind — releases it.
	defer releaseGenerationGate()

	ctx, cancel := timeoutCtx(30 * time.Minute)
	defer cancel()

	totalBoards := len(boardIDs)
	if totalBoards == 0 {
		broadcastDone(jobID, dailyReportCounts{}, 0)
		return
	}

	savedCount := 0
	failedCount := 0
	emptyCount := 0
	for idx, boardID := range boardIDs {
		boardName := dailyReportBoardName(boardID)
		broadcastProgress(jobID, "generating", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx, totalBoards))

		report, genErr := generateReportEntry(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate/save failed for board %d: %v", boardID, genErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			failedCount++
			continue
		}
		if report == nil {
			broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
			// 当日无内容：既不算成功也不算失败（与通知中心口径一致）
			emptyCount++
			continue
		}
		savedCount++
		broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards))
	}

	broadcastDone(jobID, dailyReportCounts{saved: savedCount, failed: failedCount, empty: emptyCount}, totalBoards)
	// 终态通知（白盒 B1/B2/B3）：failed==0 → 完成通知；≥1 → 一条失败汇总，互斥。
	// 计数与上面那次终态广播同源（三态：成功 / 失败 / 当日无内容；总版面数含无内容板块）。
	notifyDailyReportTerminal(date, totalBoards, savedCount, failedCount, emptyCount)
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

// listTodayReports handles GET /api/daily-reports/today.
func listTodayReports(c *gin.Context) {
	date := c.DefaultQuery("date", time.Now().Format("2006-01-02"))
	if _, err := time.Parse("2006-01-02", date); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid date"})
		return
	}
	rows, err := repository.Repo.ListTodayReportBriefs(date)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if rows == nil {
		rows = []repository.TodayReportBrief{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"date": date, "reports": rows}})
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

// rebucketDailyReportSection handles POST /api/daily-reports/:id/rebucket
// (add-intra-lane-theme-bucketing). Body: {"section_id": <id>}. It re-runs the
// intra-lane theme bucketing for that section's (report, cluster_index) group
// and returns the group's newest sections + the bucket count.
//
// Status mapping mirrors the spec:
//
//	404 report / section not found
//	409 report not completed, or the group changed mid-flight (retry)
//	400 watch-materialized group (owned by the watch track)
//	200 buckets=1 when the group cannot be split (no data changed)
func rebucketDailyReportSection(c *gin.Context) {
	reportID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid report id"})
		return
	}
	var req struct {
		SectionID uint `json:"section_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.SectionID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "section_id is required"})
		return
	}

	result, err := service.RebucketSectionGroup(c.Request.Context(), uint(reportID), req.SectionID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrRebucketReportNotFound),
			errors.Is(err, repository.ErrRebucketSectionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "report or section not found"})
		case errors.Is(err, repository.ErrRebucketReportNotDone),
			errors.Is(err, repository.ErrRebucketStaleGroup):
			c.JSON(http.StatusConflict, gin.H{"success": false, "error": "report is not ready for rebucketing"})
		case errors.Is(err, repository.ErrRebucketWatchGroup):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "watch-materialized groups cannot be rebucketed"})
		default:
			logging.Warnf("daily-report rebucket failed for report %d section %d: %v", reportID, req.SectionID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "rebucket failed"})
		}
		return
	}

	sections := result.Sections
	if sections == nil {
		sections = []repository.DailyReportSection{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"buckets":  result.Buckets,
		"sections": sections,
	}})
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
	// Optional confirmed key-event nodes (add-topic-key-event-markers):
	// normalized to a non-nil array so the reader can map directly.
	if resp.KeyEvents == nil {
		resp.KeyEvents = []repository.TopicKeyEventMarker{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"sections":   resp.Sections,
		"relations":  resp.Relations,
		"key_events": resp.KeyEvents,
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

// broadcastProgress sends a WebSocket progress message. Package-level var so
// tests can observe the broadcast stream without a live WebSocket client
// (same seam pattern as generateReportEntry).
var broadcastProgress = func(jobID string, status string, boardID uint, boardName string, saved int, progress string) {
	msg := buildProgressMessage(jobID, status, boardID, boardName, saved, progress)
	data, _ := json.Marshal(msg)
	ws.GetHub().BroadcastRaw(data)
}

// broadcastDone sends the terminal WebSocket message. Package-level var for
// the same reason as broadcastProgress: the "exactly one terminal broadcast per
// request" contract must be observable in tests.
//
// The terminal message carries the three-state tally (saved / failed / empty)
// so consumers never have to derive the failure count themselves (deriving it
// as total_boards - total_saved misclassifies "no content today" boards).
var broadcastDone = func(jobID string, counts dailyReportCounts, totalBoards int) {
	msg := buildDoneMessage(jobID, counts, totalBoards)
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

// buildDoneMessage builds the terminal WebSocket payload.
//
// The payload carries the three-state tally explicitly:
// total_saved (boards that produced a report), failed_count (boards whose
// generation failed) and empty_count (boards with no content that day).
// The three states are mutually exclusive and exhaustive over the boards in
// scope, so total_saved + failed_count + empty_count == total_boards always
// holds. failed_count / empty_count are additive fields: consumers that only
// know total_saved / total_boards keep working unchanged.
func buildDoneMessage(jobID string, counts dailyReportCounts, totalBoards int) map[string]interface{} {
	return map[string]interface{}{
		"type":         "daily_report_done",
		"job_id":       jobID,
		"total_saved":  counts.saved,
		"total_boards": totalBoards,
		"failed_count": counts.failed,
		"empty_count":  counts.empty,
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

// composeWindowMaxDays is the supported window length of the compose
// workbench (both ends included): an explicit from/to range may span at most
// 90 days.
const composeWindowMaxDays = 90

// parseComposeWindowParams parses the optional from/to window parameters of
// the compose-candidates endpoint. Both bounds are inclusive YYYY-MM-DD days
// (UTC). Giving only one of them, an inverted range or an over-long range is
// an error — the request is never silently corrected.
func parseComposeWindowParams(c *gin.Context) (*time.Time, *time.Time, error) {
	fromRaw := strings.TrimSpace(c.Query("from"))
	toRaw := strings.TrimSpace(c.Query("to"))
	if fromRaw == "" && toRaw == "" {
		return nil, nil, nil
	}
	if fromRaw == "" || toRaw == "" {
		return nil, nil, errors.New("from and to must be provided together")
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid from date %q: expected YYYY-MM-DD", fromRaw)
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid to date %q: expected YYYY-MM-DD", toRaw)
	}
	if from.After(to) {
		return nil, nil, errors.New("from must not be later than to")
	}
	if int(to.Sub(from)/(24*time.Hour))+1 > composeWindowMaxDays {
		return nil, nil, fmt.Errorf("date range must not exceed %d days", composeWindowMaxDays)
	}
	return &from, &to, nil
}

// composeCandidates handles GET
// /api/semantic-boards/:id/persistent-topics/compose-candidates?days=N&from=&to=.
// Returns the in-window sections that carry a usable embedding (parsed to a
// float slice), the window relations, the server-side candidate chains and
// the configured match_threshold, so the compose UI can compute aggregate
// anchors / outlier distances client-side in real time without re-deriving
// the chain semantics.
//
// days is the shortcut window (anchored to the board's latest completed
// report); from/to are an explicit inclusive range and override days.
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
	from, to, err := parseComposeWindowParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	// An explicit range must stay inside the fetchable window: the board's
	// latest completed report minus (composeWindowMaxDays−1) days. Older data
	// is not served, so the request is rejected instead of silently trimmed.
	// A `to` beyond the latest completed report is fine — that window simply
	// holds no later data.
	if from != nil && to != nil {
		latest, err := repository.Repo.LatestCompletedReportDate(uint(boardID))
		if err != nil {
			logging.Errorf("compose candidates: latest completed date board=%d: %v", boardID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load compose candidates"})
			return
		}
		if latest != nil {
			earliest := latest.AddDate(0, 0, -(composeWindowMaxDays - 1))
			if from.Before(earliest) {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": fmt.Sprintf(
					"from is outside the fetchable window (earliest %s)", earliest.UTC().Format("2006-01-02"))})
				return
			}
		}
	}

	resp, err := repository.Repo.GetComposeCandidates(repository.ComposeCandidatesQuery{
		BoardID: uint(boardID),
		Days:    days,
		From:    from,
		To:      to,
	})
	if err != nil {
		logging.Errorf("compose candidates: board=%d: %v", boardID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "failed to load compose candidates"})
		return
	}
	if resp.Sections == nil {
		resp.Sections = []repository.ComposeCandidateSection{}
	}
	if resp.Relations == nil {
		resp.Relations = []repository.ComposeRelation{}
	}
	if resp.CandidateChains == nil {
		resp.CandidateChains = []repository.ComposeCandidateChain{}
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
