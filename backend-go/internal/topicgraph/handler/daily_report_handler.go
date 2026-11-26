package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
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

	// GET /api/daily-reports/gaps — 窗口内「有候选内容但没有报告」的缺口清单。
	// 静态路径必须注册在通配 /daily-reports/:id 之前（gin 同层静态/通配共存
	// 依赖注册顺序，先静态后通配才不会落到 :id 分支）。
	api.GET("/daily-reports/gaps", listDailyReportGaps)

	// POST /api/daily-reports/backfill-gaps — 异步补填：受理即返回 job_id，
	// 服务端后台旧到新串行补空（只补缺、不重建已有）。
	api.POST("/daily-reports/backfill-gaps", triggerBackfillDailyReportGaps)

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
	// PATCH /api/daily-reports/topics/:id/anchor — manual re-anchoring of a
	// lane's 首义 from a chosen node range (lane-manual-correction D3). The
	// body carries section_ids only; free text is rejected.
	api.PATCH("/daily-reports/topics/:id/anchor", updateTopicAnchor)
	// GET /api/daily-reports/topics/:id/anchor — read side of the same
	// contract: the current 首义 source (auto / manual) and, for a manual
	// one, the node set that defines it (lane-manual-correction-loop).
	api.GET("/daily-reports/topics/:id/anchor", getTopicAnchor)
	// GET /api/daily-reports/topics/:id/anchor/history — the full append-only
	// 首义 rewrite trail, oldest first (review R-2).
	api.GET("/daily-reports/topics/:id/anchor/history", getTopicAnchorHistory)
	// GET /api/daily-reports/topics/:id/constraints — 辅助标签约束列表（只回元信息，
	// 不回锤向量）与全量覆盖式替换（topic-lane-auxiliary-label-constraint）。
	api.GET("/daily-reports/topics/:id/constraints", getTopicLaneConstraints)
	api.PUT("/daily-reports/topics/:id/constraints", putTopicLaneConstraints)
	// PATCH /api/daily-reports/sections/:id/assignment — manual lane
	// membership change (lane-manual-correction D4): {"topic_id": null}
	// removes the node from its lane, {"topic_id": N} attaches it to lane N.
	api.PATCH("/daily-reports/sections/:id/assignment", updateSectionAssignment)
	// GET /api/daily-reports/sections/:id/assignment/history — the full
	// append-only lane-membership trail, oldest first (review R-2).
	api.GET("/daily-reports/sections/:id/assignment/history", getSectionAssignmentHistory)
	// POST /api/daily-reports/topics/:id/detach-date — release every node of
	// that lane on one day ({"period_date": "YYYY-MM-DD"}).
	api.POST("/daily-reports/topics/:id/detach-date", detachTopicByDate)
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

	// POST /api/daily-reports/recompute-topic-centroids — one-off recompute of
	// every topic centroid with the current formula (首义锚定 + 滚动均值混合).
	// Optional ?board_id scopes to one board.
	api.POST("/daily-reports/recompute-topic-centroids", triggerRecomputeTopicCentroids)

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

	// 线索相关事件推荐（replace-lane-lifeline-with-topic-events）：一次返回
	// 整份日报所有线索的相关事件，只读；取代话题泳道折线的「之前讲过什么」入口。
	RegisterThreadRelationRoutes(api)
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

// boardHealthCheckupTimeout 是手动日报产出后异步体检的整轮预算（design D4）。
// 生成 goroutine 的 ctx 此时已结束（不得复用），体检自派生 30 分钟预算。
const boardHealthCheckupTimeout = 30 * time.Minute

// triggerBoardHealthCheckup 是体检调用的包级 seam（测试可替换）：真实实现
// 走生产 LLM 体检入口，scope = 本次产出日报的板块集合。
var triggerBoardHealthCheckup = func(scope []uint) {
	ctx, cancel := context.WithTimeout(context.Background(), boardHealthCheckupTimeout)
	defer cancel()
	tagging.RunBoardHealthCheckupWithOptions(ctx, repository.Repo.DB(), tagging.BoardHealthOptions{BoardIDs: scope})
}

// launchBoardHealthCheckup 异步触发体检：goroutine 边界 recover，任何 panic
// 只记日志，绝不影响 HTTP 响应与日报 job（design D3）。新鲜度守卫在体检
// 入口内（默认 6h），重复触发零成本。
func launchBoardHealthCheckup(scope []uint) {
	if len(scope) == 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logging.Errorf("daily-report: board-health checkup panicked (report untouched): %v", r)
			}
		}()
		triggerBoardHealthCheckup(scope)
	}()
}

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

	dateKey := date.Format("2006-01-02")
	boardName := dailyReportBoardName(boardID)
	broadcastProgress(jobID, "generating", boardID, boardName, 0, "0/1", dateKey, dailyReportRunScopeGenerate)

	report, err := generateReportEntry(ctx, boardID, date)
	if err != nil {
		logging.Errorf("daily-report: generate/save failed for board %d: %v", boardID, err)
		broadcastProgress(jobID, "failed", boardID, boardName, 0, "1/1", dateKey, dailyReportRunScopeGenerate)
		// 三态计数与终态通知同口径：失败 1（该板块真失败）· 无内容 0。
		broadcastDone(jobID, dailyReportCounts{failed: 1}, 1)
		// 单版面项目：失败 1 = 全部失败 → 1 条失败汇总（白盒 B3/B 边界）
		notifyDailyReportTerminal(date, 1, 0, 1, 0)
		return
	}
	if report == nil {
		broadcastProgress(jobID, "completed", boardID, boardName, 0, "1/1", dateKey, dailyReportRunScopeGenerate)
		// 版面无内容不算失败：终态完成（保存 0 条）——计入 empty，不计失败。
		broadcastDone(jobID, dailyReportCounts{empty: 1}, 1)
		// 版面无内容不算失败：终态完成（保存 0 条）；通知走成功类，且
		// 「共 1 个版面」含这个当日无内容的板块。
		notifyDailyReportTerminal(date, 1, 0, 0, 1)
		return
	}

	broadcastProgress(jobID, "completed", boardID, boardName, 1, "1/1", dateKey, dailyReportRunScopeGenerate)
	broadcastDone(jobID, dailyReportCounts{saved: 1}, 1)
	notifyDailyReportTerminal(date, 1, 1, 0, 0)
	// 产出日报 → 体检该板块（异步、范围限本板）。体检是体检依据的产出面：
	// 用户手动补出日报后必须能看到新依据（fix-board-health-triage-degrade D3）。
	launchBoardHealthCheckup([]uint{boardID})
}

func generateAllBoards(date time.Time, jobID string) {
	// Gate release (guard-daily-report-generate): this goroutine owns the
	// generation gate acquired by the handler; deferred FIRST so every exit
	// path — success, failure, or panic unwind — releases it.
	defer releaseGenerationGate()

	ctx, cancel := timeoutCtx(30 * time.Minute)
	defer cancel()

	dateKey := date.Format("2006-01-02")

	boardIDs, err := repository.Repo.CollectBoardIDsForDate(date)
	if err != nil {
		logging.Errorf("daily-report: collect boards failed: %v", err)
		broadcastProgress(jobID, "failed", 0, "All boards", 0, "0/0", dateKey, dailyReportRunScopeGenerate)
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
		broadcastProgress(jobID, "generating", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx, totalBoards), dateKey, dailyReportRunScopeGenerate)

		report, genErr := generateReportEntry(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate/save failed for board %d: %v", boardID, genErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards), dateKey, dailyReportRunScopeGenerate)
			failedCount++
			continue
		}
		if report == nil {
			broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards), dateKey, dailyReportRunScopeGenerate)
			// 当日无内容：既不算成功也不算失败（与通知中心口径一致）
			emptyCount++
			continue
		}
		savedCount++
		broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards), dateKey, dailyReportRunScopeGenerate)
	}

	broadcastDone(jobID, dailyReportCounts{saved: savedCount, failed: failedCount, empty: emptyCount}, totalBoards)
	// 终态通知（白盒 B1/B2/B3）：failed==0 → 完成通知；≥1 → 一条失败汇总，互斥。
	// 计数与上面那次终态广播同源（三态：成功 / 失败 / 当日无内容；总版面数含无内容板块）。
	notifyDailyReportTerminal(date, totalBoards, savedCount, failedCount, emptyCount)
	// 产出日报 → 体检本次板块集合（异步）。
	if savedCount > 0 {
		launchBoardHealthCheckup(boardIDs)
	}
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

	dateKey := date.Format("2006-01-02")

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
		broadcastProgress(jobID, "generating", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx, totalBoards), dateKey, dailyReportRunScopeGenerate)

		report, genErr := generateReportEntry(ctx, boardID, date)
		if genErr != nil {
			logging.Warnf("daily-report: generate/save failed for board %d: %v", boardID, genErr)
			broadcastProgress(jobID, "failed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards), dateKey, dailyReportRunScopeGenerate)
			failedCount++
			continue
		}
		if report == nil {
			broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards), dateKey, dailyReportRunScopeGenerate)
			// 当日无内容：既不算成功也不算失败（与通知中心口径一致）
			emptyCount++
			continue
		}
		savedCount++
		broadcastProgress(jobID, "completed", boardID, boardName, savedCount, fmt.Sprintf("%d/%d", idx+1, totalBoards), dateKey, dailyReportRunScopeGenerate)
	}

	broadcastDone(jobID, dailyReportCounts{saved: savedCount, failed: failedCount, empty: emptyCount}, totalBoards)
	// 终态通知（白盒 B1/B2/B3）：failed==0 → 完成通知；≥1 → 一条失败汇总，互斥。
	// 计数与上面那次终态广播同源（三态：成功 / 失败 / 当日无内容；总版面数含无内容板块）。
	notifyDailyReportTerminal(date, totalBoards, savedCount, failedCount, emptyCount)
	// 产出日报 → 体检本次板块集合（异步）。
	if savedCount > 0 {
		launchBoardHealthCheckup(boardIDs)
	}
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

// 日报运行的来源标记（run_scope）。generate = 当日生成（定时 / 手动），
// backfill = 缺档补填。补填的逐格进度复用 daily_report_progress 事件类型，
// 终态走独立事件类型（见 buildBackfillDoneMessage）。
const (
	dailyReportRunScopeGenerate = "generate"
	dailyReportRunScopeBackfill = "backfill"
)

// broadcastProgress sends a WebSocket progress message. Package-level var so
// tests can observe the broadcast stream without a live WebSocket client
// (same seam pattern as generateReportEntry).
var broadcastProgress = func(jobID string, status string, boardID uint, boardName string, saved int, progress string, date string, runScope string) {
	msg := buildProgressMessage(jobID, status, boardID, boardName, saved, progress, date, runScope)
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

// buildProgressMessage builds the per-board progress payload.
//
// date 与 run_scope 是附加字段（add-daily-report-gap-backfill D10）：
//   - date：本次进度对应的**报告日**（YYYY-MM-DD）。补填是多天串行的，
//     没有它前端无法把进度归到具体那一天。
//   - run_scope：本次运行的来源（generate / backfill）。补填的逐格进度复用
//     本事件类型（旧消费方忽略附加字段即可），但补填的**终态**走独立事件
//     daily_report_backfill_done——daily_report_done 被现有消费方无 job_id
//     过滤地消费，复用会把补填终态误认成「本次今日生成」的结果。
//
// 既有字段（type / job_id / status / board_id / board_name / saved /
// progress / timestamp）一个不动，旧消费方保持兼容。
func buildProgressMessage(jobID string, status string, boardID uint, boardName string, saved int, progress string, date string, runScope string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "daily_report_progress",
		"job_id":     jobID,
		"status":     status,
		"board_id":   boardID,
		"board_name": boardName,
		"saved":      saved,
		"progress":   progress,
		"date":       date,
		"run_scope":  runScope,
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

// ═══════════════════════════════════════════════════════════════════════
// 日报缺档：缺口查询 + 异步补填（add-daily-report-gap-backfill）
// ═══════════════════════════════════════════════════════════════════════

// backfillRunTimeout 是补填后台 goroutine 的硬上限。整轮预算由
// ai_settings（daily_report_backfill_budget_minutes，默认 60 分钟）控制，
// 这个超时只是「预算之外也别永远挂着」的兜底：在途的那一格生成允许比预算
// 多跑一会儿（预算只拦「不再启动新的生成」）。
const backfillRunTimeout = 2 * time.Hour

// 补填相关的包级 seam（与 generateReportEntry / broadcastProgress 同一模式）：
// 测试替换它们即可观察受理 / 进度 / 终态 / 通知，不碰真 LLM 管线。
var (
	// broadcastBackfillProgress 逐格进度（复用 daily_report_progress 事件类型，
	// 带 date 与 run_scope=backfill 附加字段）。
	broadcastBackfillProgress = func(jobID string, p service.GapBackfillProgress) {
		msg := buildBackfillProgressMessage(jobID, p)
		data, _ := json.Marshal(msg)
		ws.GetHub().BroadcastRaw(data)
	}
	// broadcastBackfillDone 终态（独立事件类型 daily_report_backfill_done）。
	broadcastBackfillDone = func(jobID, runID, status string, filled, failed, remaining int, windowStart, windowEnd string, startedAt, finishedAt time.Time) {
		msg := buildBackfillDoneMessage(jobID, runID, status, filled, failed, remaining, windowStart, windowEnd, startedAt, finishedAt)
		data, _ := json.Marshal(msg)
		ws.GetHub().BroadcastRaw(data)
	}
	// notifyBackfillTerminal 终态通知入口（每次运行恰好一条，四态）。
	notifyBackfillTerminal = notification.DailyReportBackfillTerminal
)

// buildBackfillProgressMessage builds one gap-backfill cell's progress payload.
//
// 事件类型复用 daily_report_progress（旧消费方忽略附加字段即可），附加 date
// （本次进度对应的报告日）与 run_scope=backfill（区分补填与当日生成）。
func buildBackfillProgressMessage(jobID string, p service.GapBackfillProgress) map[string]interface{} {
	return buildProgressMessage(jobID, p.Status, p.BoardID, p.BoardLabel,
		p.Filled, fmt.Sprintf("%d/%d", p.Filled, p.Total), p.Date, dailyReportRunScopeBackfill)
}

// buildBackfillDoneMessage builds the gap-backfill terminal payload.
//
// **独立事件类型**（不复用 daily_report_done）：daily_report_done 被现有消费方
// （useDailyReportProgress / useRadarPipelineStatus 的 done 分支）无 job_id
// 过滤地消费，复用会把补填终态误认成「本次今日生成」的结果。
//
// 字段：type / job_id / run_id / status / filled / failed / remaining /
// window_start / window_end / started_at / finished_at / timestamp。
func buildBackfillDoneMessage(jobID, runID, status string, filled, failed, remaining int, windowStart, windowEnd string, startedAt, finishedAt time.Time) map[string]interface{} {
	return map[string]interface{}{
		"type":         "daily_report_backfill_done",
		"job_id":       jobID,
		"run_id":       runID,
		"status":       status,
		"filled":       filled,
		"failed":       failed,
		"remaining":    remaining,
		"window_start": windowStart,
		"window_end":   windowEnd,
		"started_at":   startedAt.Format(time.RFC3339),
		"finished_at":  finishedAt.Format(time.RFC3339),
		"timestamp":    time.Now().Format(time.RFC3339),
	}
}

// dailyReportGapDefaultDays is the default lookback of the gap window (days).
// 上限受 tag_edge_retention_days 约束（保留窗口是硬上界）。缺口查询与补填
// 端点共用这个默认值——「补齐」的范围必须与用户看到的缺口清单同口径。
const dailyReportGapDefaultDays = 7

// localDayStart truncates a timestamp to local midnight.
func localDayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// dailyReportGapWindow resolves the gap/backfill window: [today - days, today - 1].
// today 不在窗口内（今天由当日生成负责）；days 只能收窄，不能越过保留窗口下界
// （effectiveDays = min(days, retentionDays)）。
func dailyReportGapWindow(days int, retentionDays int, now time.Time) (time.Time, time.Time, int) {
	effective := days
	if effective <= 0 {
		effective = retentionDays
	}
	if retentionDays > 0 && effective > retentionDays {
		effective = retentionDays
	}
	if effective <= 0 {
		effective = 1
	}
	today := localDayStart(now)
	return today.AddDate(0, 0, -effective), today.AddDate(0, 0, -1), effective
}

// listDailyReportGaps handles GET /api/daily-reports/gaps.
//
// 只读端点：返回窗口内「该板块当天有候选内容、但没有报告」的格子。候选口径
// 与生成 Step1 一致（active + event + NOT direction_mismatch + 跨板排他），
// 不使用宽松口径——否则会把生成时必然被剔的板块算成缺口，补填反复空跑烧 LLM。
//
// 查询不写库、不触发任何生成。
func listDailyReportGaps(c *gin.Context) {
	retentionDays := tagging.LoadTagEdgeRetentionDays(repository.Repo.DB())

	days := dailyReportGapDefaultDays
	if v := c.Query("days"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "invalid days, expected a positive integer",
				"reason":  "invalid_days",
			})
			return
		}
		days = parsed
	}

	now := time.Now()
	windowStart, windowEnd, effectiveDays := dailyReportGapWindow(days, retentionDays, now)

	gaps, err := service.ListDailyReportGaps(windowStart, windowEnd)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if gaps == nil {
		gaps = []repository.DailyReportGap{}
	}

	// 按日分组（升序）：missing_days = 有缺口的日期数，total_missing = 格子数。
	byDate := map[string][]repository.DailyReportGap{}
	dateOrder := []string{}
	for _, g := range gaps {
		if _, ok := byDate[g.Date]; !ok {
			dateOrder = append(dateOrder, g.Date)
		}
		byDate[g.Date] = append(byDate[g.Date], g)
	}
	sort.Strings(dateOrder)

	lastRun, err := repository.Repo.LatestBackfillRun()
	if err != nil {
		logging.Warnf("daily-report: load latest backfill run failed: %v", err)
	}

	payload := gin.H{
		"window_start":   windowStart.Format("2006-01-02"),
		"window_end":     windowEnd.Format("2006-01-02"),
		"days":           effectiveDays,
		"retention_days": retentionDays,
		"gaps":           gaps,
		"missing_days":   len(dateOrder),
		"total_missing":  len(gaps),
	}
	if lastRun != nil {
		payload["last_run"] = backfillRunSummary(lastRun)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": payload})
}

// backfillRunSummary projects a run ledger row into the gaps response's
// last_run field (no run record → the field is absent).
func backfillRunSummary(run *repository.DailyReportBackfillRun) gin.H {
	summary := gin.H{
		"run_id":       run.RunID,
		"status":       run.Status,
		"filled":       run.FilledCount,
		"failed":       run.FailedCount,
		"remaining":    run.RemainingCount,
		"window_start": run.WindowStart.Format("2006-01-02"),
		"window_end":   run.WindowEnd.Format("2006-01-02"),
		"started_at":   run.StartedAt.Format(time.RFC3339),
	}
	if run.FinishedAt != nil {
		summary["finished_at"] = run.FinishedAt.Format(time.RFC3339)
	}
	return summary
}

// triggerBackfillDailyReportGaps handles POST /api/daily-reports/backfill-gaps.
//
// 受理即返回 job_id / run_id，补填在服务端后台旧到新串行执行（HTTP 线程不
// 等待补齐完成）。
//
// 三条前置先判后跑（顺序固定，与 generate 端点同口径）：
//
//	① 窗口守卫（400）——超保留窗口的日期拒绝；今天由当日生成负责，不在补填窗口内；
//	② 打标队列门槛（409）——队列未排空时补出的候选不全；
//	③ 日报生成闸门（409）——与定时任务 / 手动生成共用同一把域闸。
//
// 请求体（均可选）：{"date": "YYYY-MM-DD", "days": N, "board_id": N}。
// date 与 days 互斥（一个指定单日，一个收窄窗口）。
func triggerBackfillDailyReportGaps(c *gin.Context) {
	var req struct {
		Date    string `json:"date"`
		Days    *int   `json:"days"`
		BoardID *uint  `json:"board_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// 类型错误直接拒（与 generate 端点同款）；其余（空 body / 非法 JSON）
		// 走宽松路径 = 全窗口补填。
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			expected := "the declared field type"
			switch typeErr.Field {
			case "date":
				expected = "a YYYY-MM-DD string"
			case "days":
				expected = "a number"
			case "board_id":
				expected = "a number"
			default:
				if typeErr.Field == "" {
					typeErr.Field = "request body"
					expected = "a JSON object"
				}
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("invalid %s: expected %s, got %s", typeErr.Field, expected, typeErr.Value),
			})
			return
		}
		req.Date = ""
		req.Days = nil
		req.BoardID = nil
	}

	if req.Date != "" && req.Days != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "date 与 days 不能同时指定：一个指定单日，一个收窄窗口。",
			"reason":  "conflicting_scope",
		})
		return
	}

	now := time.Now()
	retentionDays := tagging.LoadTagEdgeRetentionDays(repository.Repo.DB())

	var windowStart, windowEnd time.Time
	if req.Date != "" {
		parsed, err := time.ParseInLocation("2006-01-02", req.Date, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "invalid date format, use YYYY-MM-DD",
				"reason":  "invalid_date",
			})
			return
		}
		// 重建窗口守卫：与 generate / TriggerNowWithDate 同谓词同文案。
		if service.IsDateOutsideRebuildWindow(parsed, now, retentionDays) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   service.RebuildWindowRejectionMessage(retentionDays),
				"reason":  "out_of_retention_window",
			})
			return
		}
		// 今天由当日日报生成负责，补填窗口不含今天（也不含未来）。
		if !parsed.Before(localDayStart(now)) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "补填窗口不含今天与未来日期（今天由当日日报生成负责）。",
				"reason":  "out_of_backfill_window",
			})
			return
		}
		windowStart = localDayStart(parsed)
		windowEnd = localDayStart(parsed)
	} else {
		// 默认天数与缺口查询一致（7 天，上限受保留窗口约束）——「补齐」的
		// 范围必须与用户看到的缺口清单同口径，否则会补出用户看不见的日子。
		days := dailyReportGapDefaultDays
		if req.Days != nil {
			if *req.Days <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"error":   "invalid days, expected a positive integer",
					"reason":  "invalid_days",
				})
				return
			}
			days = *req.Days
		}
		windowStart, windowEnd, _ = dailyReportGapWindow(days, retentionDays, now)
	}

	// 队列门槛（409）：打标队列未排空时补出的候选不全（与调度器补档同口径）。
	// 双队列（tag_jobs + embedding_queues）语义同 useRadarPipelineStatus：
	// 任一未清空即不得生成；计数不可得按「未知」处理（不当作已清空）。
	drained, queueReason, queueErr := backfillQueuesDrained()
	if queueErr != nil || !drained {
		reason := queueReason
		if reason == "" {
			reason = "queue_check_failed"
		}
		logging.Warnf("daily-report: backfill rejected (reason=%s): %v", reason, queueErr)
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "打标/向量化队列未排空，补出的候选不全，请稍后再试。",
			"reason":  reason,
		})
		return
	}

	// 日报生成闸门（409）：与定时任务 / 手动生成共用同一把域闸，整轮持有。
	if !tryAcquireGenerationGate() {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error":   "日报生成正在执行中，请稍后再试。",
			"reason":  "already_running",
		})
		return
	}

	jobID := uuid.New().String()
	runID := uuid.New().String()

	opts := service.GapBackfillOptions{
		WindowStart:  windowStart,
		WindowEnd:    windowEnd,
		TriggerScope: repository.BackfillTriggerScopeManual,
		RunID:        runID,
		Progress: func(p service.GapBackfillProgress) {
			broadcastBackfillProgress(jobID, p)
		},
	}
	if req.BoardID != nil {
		opts.BoardIDs = []uint{*req.BoardID}
	}

	go runGapBackfill(jobID, runID, opts)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"job_id":       jobID,
			"run_id":       runID,
			"status":       "processing",
			"window_start": windowStart.Format("2006-01-02"),
			"window_end":   windowEnd.Format("2006-01-02"),
		},
	})
}

// backfillQueuesDrained reports whether both consumer queues are drained:
// tag_jobs has no pending/leased rows AND embedding_queues has no
// pending/processing rows.
//
// 与调度器的 countQueuesDrained（internal/admin/scheduler/job_daily_report.go）
// 是同一把尺子的调度器侧孪生实现——跨域不可 import（topicgraph → admin 会成环），
// 故按同一口径复刻于此：failed 的打标作业**不**计入活跃量（它永远不会排空，
// 不能因此永久冻结补填），但计数不可得时按「未知」处理（不当作已清空）。
func backfillQueuesDrained() (bool, string, error) {
	db := repository.Repo.DB()
	if db == nil {
		return false, "queue_check_failed", fmt.Errorf("backfill queue check: repository not initialized")
	}
	var tagBacklog int64
	if err := db.Model(&models.TagJob{}).
		Where("status IN ?", []string{string(models.JobStatusPending), string(models.JobStatusLeased)}).
		Count(&tagBacklog).Error; err != nil {
		return false, "queue_check_failed", fmt.Errorf("count tag job backlog: %w", err)
	}
	var embeddingBacklog int64
	if err := db.Model(&models.EmbeddingQueue{}).
		Where("status IN ?", []string{models.EmbeddingQueueStatusPending, models.EmbeddingQueueStatusProcessing}).
		Count(&embeddingBacklog).Error; err != nil {
		return false, "queue_check_failed", fmt.Errorf("count embedding queue backlog: %w", err)
	}
	if tagBacklog > 0 {
		return false, "tag_queue_not_empty", nil
	}
	if embeddingBacklog > 0 {
		return false, "embedding_queue_not_empty", nil
	}
	return true, "", nil
}

// runGapBackfill is the background worker of one accepted backfill run.
//
// 闸门释放挂在最前（defer FIRST）：成功 / 失败 / panic 展开都释放。
// 终态：恰好一条通知（零缺口由通知入口自己守）+ 一条独立 WS 终态消息。
func runGapBackfill(jobID, runID string, opts service.GapBackfillOptions) {
	defer releaseGenerationGate()

	ctx, cancel := context.WithTimeout(context.Background(), backfillRunTimeout)
	defer cancel()

	windowStart := opts.WindowStart.Format("2006-01-02")
	windowEnd := opts.WindowEnd.Format("2006-01-02")

	result, err := service.RunGapBackfill(ctx, opts)
	if err != nil {
		// 缺口扫描失败 = 这次运行根本没开始（没有格子被处理）：不发通知，
		// 但仍广播终态消息，前端不会一直等。
		logging.Errorf("daily-report: gap backfill failed: %v", err)
		now := time.Now()
		broadcastBackfillDone(jobID, runID, "failed", 0, 0, 0, windowStart, windowEnd, now, now)
		return
	}

	counts := notification.BackfillCounts{
		TotalCells: result.TotalCells,
		Filled:     result.Filled,
		Failed:     result.Failed,
		Remaining:  result.Remaining,
	}
	// 每次运行恰好一条终态通知（成功也发；零缺口不发——由通知入口守）。
	notifyBackfillTerminal(result.Status, windowStart, windowEnd, counts)
	broadcastBackfillDone(jobID, result.RunID, result.Status,
		result.Filled, result.Failed, result.Remaining,
		windowStart, windowEnd, result.StartedAt, result.FinishedAt)
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

// triggerRecomputeTopicCentroids handles POST
// /api/daily-reports/recompute-topic-centroids?board_id=<optional>.
//
// One-off correction of the existing topic centroids after the anchor-blend
// formula (首义锚定 + 滚动均值混合) replaced the pure rolling mean: a topic
// whose centroid drifted over months is pulled back towards its creation-time
// definition. Optional ?board_id scopes the run to one board; without it every
// board is processed.
//
// Async by design (same shape as triggerBackfillEmbeddings): the response
// returns immediately with status=processing while the work runs in a
// background goroutine, so a full-site run never blocks the HTTP call.
// Idempotent — rerunning it with the same data yields the same centroids.
func triggerRecomputeTopicCentroids(c *gin.Context) {
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

	// WithTimeout only after validation: the 400 path never spawns the
	// goroutine, so the context must not be created before it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	go func() {
		// cancel must live inside the worker goroutine: a handler-scoped
		// `defer cancel()` fires the moment this handler returns (right after
		// the "processing" JSON) and would kill the run immediately.
		defer cancel()
		updated, failed, err := repository.Repo.RecomputeTopicCentroids(ctx, boardID)
		if err != nil {
			logging.Errorf("daily-report: recompute topic centroids failed: %v", err)
			return
		}
		logging.Infof("daily-report: recompute topic centroids complete: %d topics updated, %d failed", updated, failed)
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"status": "processing",
		},
	})
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
// Body: {"label": "...", "section_ids": [1,2,3], "constraints": [{"term":"以色列","mode":"required"}]}
// Creates a new persistent topic with source=manual from the selected sections'
// mean embedding, then reassigns those sections to the new topic.
//
// `constraints` 可选（topic-lane-auxiliary-label-constraint）：提供时与建话题**同事务**
// 写入（锚向量在开事务前算好）；任一约束非法或锚算不出 → 整个建话题拒绝，
// SHALL NOT 留下「有泳道、无约束」的半成品。
func createManualTopic(c *gin.Context) {
	boardID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid board id"})
		return
	}

	var req struct {
		Label       string                          `json:"label"`
		SectionIDs  []uint                          `json:"section_ids"`
		Constraints []service.LaneConstraintRequest `json:"constraints"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid body"})
		return
	}
	if req.Label == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "label is required"})
		return
	}

	// 锚向量必须在开事务前算完（embedding 是网络调用，不能进 DB 事务）。
	// 失败分三类：载荷非法 400 / 锚算不出 503（带可读原因）/ 其余 500。
	constraints, err := service.PrepareManualLaneConstraints(c.Request.Context(), req.Constraints)
	if err != nil {
		respondLaneConstraintError(c, err,
			fmt.Sprintf("prepare manual lane constraints: board=%d label=%q", boardID, req.Label),
			"failed to compute constraint anchors")
		return
	}

	topic, skipped, err := repository.Repo.CreateManualTopicWithConstraints(uint(boardID), req.Label, req.SectionIDs, constraints)
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
// 365 days (unify-lane-workbench OQ-2，2026-11-16 用户裁决由 90 放宽；与
// repository.ComposeWindowMaxDays 同源，三处上限之一，另两处：报错文案引用
// 本常量、前端 useLaneCompose.validateCustomRange)。
const composeWindowMaxDays = repository.ComposeWindowMaxDays

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
//
// all_members_topic_id is the lane-correction mode: when given, the members
// of that topic whose report period precedes the window's first day are
// appended as early_members (数据全量、展示分层 — the compose canvas draws
// the window only, the left column lists the rest). Omitting the parameter
// keeps the compose workbench response unchanged (no early_members field at
// all).
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
	allMembersTopicID, status, msg := parseAllMembersTopicID(c, uint(boardID))
	if status != 0 {
		c.JSON(status, gin.H{"success": false, "error": msg})
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
		BoardID:           uint(boardID),
		Days:              days,
		From:              from,
		To:                to,
		AllMembersTopicID: allMembersTopicID,
		// 轻量取数（unify-lane-workbench OQ-2）：曲线路径省略每条 section 的
		// embedding 向量（每条 10,244 B），只留日期/归属/clue_count。
		OmitEmbeddings: c.Query("omit_embeddings") == "1",
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
	if resp.EarlyMembers != nil && *resp.EarlyMembers == nil {
		empty := []repository.ComposeEarlyMember{}
		resp.EarlyMembers = &empty
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// parseAllMembersTopicID parses the optional all_members_topic_id parameter of
// the compose-candidates endpoint (the lane-correction 「全量成员」 mode).
//
// The parameter must name a topic of the requested board: a topic of another
// board is rejected (404) instead of answering with an empty member list that
// would read as "this topic has no earlier members".
//
// The second return value is the HTTP status to answer with (0 = no error);
// the third is the error message.
func parseAllMembersTopicID(c *gin.Context, boardID uint) (*uint, int, string) {
	raw := strings.TrimSpace(c.Query("all_members_topic_id"))
	if raw == "" {
		return nil, 0, ""
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 {
		return nil, http.StatusBadRequest, "invalid all_members_topic_id"
	}
	topicID := uint(parsed)
	owner, ok, err := repository.Repo.PersistentTopicBoard(topicID)
	if err != nil {
		logging.Errorf("compose candidates: load topic %d: %v", topicID, err)
		return nil, http.StatusInternalServerError, "failed to load the topic of all_members_topic_id"
	}
	if !ok || owner != boardID {
		return nil, http.StatusNotFound, "topic not found on this board"
	}
	return &topicID, 0, ""
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
