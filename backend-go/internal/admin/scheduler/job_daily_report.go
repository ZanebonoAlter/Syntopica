package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	repository "syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/articlerefs"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/notification"
	"syntopica-backend/internal/platform/ws"
	tagging "syntopica-backend/internal/tagmanagement"
	daily_report "syntopica-backend/internal/topicgraph"
	topicgraphrepo "syntopica-backend/internal/topicgraph/repository"
)

// generateAndSaveReport is the daily report generation entry point used by both
// the main pass and the backfill scan. A package-level variable so tests can
// substitute a stub: the real implementation drives the whole LLM pipeline.
var generateAndSaveReport = daily_report.GenerateAndSaveReport

// Notification seams (add-notification-center, 白盒 B): package-level vars so
// tests can capture adjudication calls without a notification database
// (same seam as generateAndSaveReport).
var (
	notifyDailyReportSuccess       = notification.DailyReportSuccess
	notifyDailyReportFailedSummary = notification.DailyReportFailedSummary
)

// adjudicateDailyReportTerminal is the single adjudication point for the
// scheduled daily-report terminal-state notification (白盒 B):
// failedCount>0 → ONE failure summary (success/failed counts in the copy);
// failedCount==0 → one completion notification. Mutually exclusive — never
// more than one notification per run. 成功口径 = 版面产出报告的计数；空版面
// (report==nil) 算成功侧、不算失败（对齐 handler 路径口径：版面无内容≠失败）。
func adjudicateDailyReportTerminal(date time.Time, totalBoards, successCount, failedCount int) {
	if failedCount > 0 {
		notifyDailyReportFailedSummary(date, successCount, failedCount)
		return
	}
	notifyDailyReportSuccess(date, totalBoards, successCount)
}

// backfillScanTimeout bounds the backfill pass on its own budget. The main pass
// owns the job's 30-minute context for today's reports; a shared budget would
// leave the backfill of up to retentionDays × boards reports with whatever is
// left (usually nothing), silently failing every generation.
const backfillScanTimeout = 30 * time.Minute

// NextDailyReportTime computes the next wall-clock time for the daily report.
// It reads the configured HH:MM from AISettings (default "21:00") and returns
// today at that time if it hasn't passed yet, otherwise tomorrow at that time.
func NextDailyReportTime(now time.Time) time.Time {
	hhmm, err := aisettings.LoadDailyReportTimeConfig()
	if err != nil {
		logging.Warnf("daily_report: failed to load time config, using default: %v", err)
		hhmm = "21:00"
	}

	h, m := 21, 0
	_, scanErr := fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	if scanErr != nil {
		logging.Warnf("daily_report: failed to parse time %q, using default 21:00: %v", hhmm, scanErr)
		h, m = 21, 0
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if now.Before(today) {
		return today
	}
	return today.Add(24 * time.Hour)
}

// DailyReportJob generates daily reports for all active semantic boards.
// When no targetDate is provided (nil), it reports on the current local time.
func DailyReportJob(targetDate ...time.Time) JobFunc {
	return func(ctx context.Context) (*JobResult, error) {
		startTime := time.Now()

		date := time.Now().In(time.Local)
		if len(targetDate) > 0 {
			date = targetDate[0]
		}

		boardIDs, err := daily_report.CollectBoardIDsForDate(date)
		if err != nil {
			// 收集阶段即失败：一条失败汇总（fail-open，不改变作业本身的错误返回）
			notifyDailyReportFailedSummary(date, 0, 1)
			return nil, fmt.Errorf("failed to collect board IDs: %w", err)
		}

		// baseCtx survives the main-pass timeout so the backfill gets its own
		// budget instead of whatever the main pass left (< design D5).
		baseCtx := ctx
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()

		reportCount := 0
		failedCount := 0
		for _, boardID := range boardIDs {
			report, genErr := generateAndSaveReport(ctx, boardID, date)
			if genErr != nil {
				logging.Warnf("daily-report: generate/save failed for board %d: %v", boardID, genErr)
				failedCount++
				continue
			}
			if report == nil {
				continue // 当日无内容正常返回 (nil, nil)：空版面不算失败（对齐 handler 口径），计入成功侧
			}
			reportCount++
		}

		// Broadcast completion
		msg := map[string]interface{}{
			"type":         "daily_report_complete",
			"report_count": reportCount,
			"date":         date.Format("2006-01-02"),
			"timestamp":    time.Now().Format(time.RFC3339),
		}
		data, _ := json.Marshal(msg)
		ws.GetHub().BroadcastRaw(data)

		// Terminal-state notification (add-notification-center, 白盒 B): the
		// scheduled run is the 定时日报 users miss overnight — notify here.
		// The backfill sweep below is deliberately silent (no per-day spam).
		// failed 口径 = genErr != nil 的版面数（非 totalBoards-reportCount：
		// 空版面不是失败）。失败汇总与完成通知互斥（adjudicateDailyReportTerminal）。
		if totalBoards := len(boardIDs); totalBoards > 0 {
			adjudicateDailyReportTerminal(date, totalBoards, reportCount, failedCount)
		}

		resultData := map[string]interface{}{
			"report_count":   reportCount,
			"trigger_source": "scheduled",
			"started_at":     startTime.Format(time.RFC3339),
			"finished_at":    time.Now().Format(time.RFC3339),
		}
		summary := fmt.Sprintf("generated %d reports for %s", reportCount, date.Format("2006-01-02"))

		// Backfill is a scheduled-only follow-up: a manual TriggerNowWithDate is a
		// deliberate single-date rebuild, not a sweep (design D5).
		backfilled := 0
		if len(targetDate) == 0 {
			count, backfillData := backfillMissingReports(baseCtx, date)
			backfilled = count
			for key, value := range backfillData {
				resultData[key] = value
			}
			summary = fmt.Sprintf("%s (backfilled %d)", summary, backfilled)
		}
		// Set last so the scheduled merge above cannot be overwritten by it.
		resultData["backfilled_count"] = backfilled

		// Article references dangle when a delete path runs against an already
		// written report, or when a path nobody wired runs at all. Read-only
		// probe: it never deletes, so an unknown deleter stays visible instead of
		// being silently papered over (heal-dangling-article-refs D6).
		if dangling, danglingErr := articlerefs.CountDanglingArticleRefs(repository.Repo.DB()); danglingErr != nil {
			logging.Warnf("daily-report: dangling article ref check failed: %v", danglingErr)
		} else if dangling > 0 {
			logging.Warnf("daily-report: dangling article refs=%d (threads reference deleted articles; repair runs on the next startup migration)", dangling)
		} else {
			logging.Infof("daily-report: dangling article refs=0")
		}

		return &JobResult{
			Data:    resultData,
			Summary: summary,
		}, nil
	}
}

// backfillMissingReports rebuilds the reports missing inside the retention
// window (offline-catchup design D5).
//
// Precondition: the tagging queue must be drained. While tag_jobs still holds
// pending/leased rows the resumed drain has not attached the missing edges yet,
// so a backfill now would write reports with incomplete candidates. Skipping
// the whole pass costs at most one day — the missing days are still inside the
// window and the next run retries (顺延不丢).
//
// The scan covers [today-retentionDays, today-1] inclusive: today is already
// produced by the main pass, and the window is the same
// tag_edge_retention_days key the edge GC uses, so "an edge exists" and
// "the day is rebuildable" can never drift apart.
func backfillMissingReports(ctx context.Context, today time.Time) (int, map[string]interface{}) {
	data := map[string]interface{}{}

	pending, leased, failed, err := countUnfinishedTagJobs()
	if err != nil {
		logging.Warnf("daily-report: backfill queue check failed: %v; skipping backfill this round", err)
		data["backfill_skipped_reason"] = "tag_queue_check_failed"
		return 0, data
	}
	// failed jobs do NOT block the backfill (a permanently failing article must
	// not freeze the catch-up forever), but their edges are missing, so the
	// candidate set may be incomplete — report the count and say so.
	data["backfill_failed_jobs"] = failed
	if failed > 0 {
		logging.Warnf("daily-report: %d tag job(s) failed; their articles may lack edges, so this backfill round's candidates may be incomplete", failed)
	}
	if pending+leased > 0 {
		logging.Warnf("daily-report: backfill deferred — tagging queue not drained (pending=%d leased=%d); retrying next run",
			pending, leased)
		data["backfill_skipped_reason"] = "tag_queue_not_empty"
		data["backfill_pending_jobs"] = pending
		data["backfill_leased_jobs"] = leased
		return 0, data
	}

	retentionDays := tagging.LoadTagEdgeRetentionDays(repository.Repo.DB())

	ctx, cancel := context.WithTimeout(ctx, backfillScanTimeout)
	defer cancel()

	backfilled := 0
	start := today.AddDate(0, 0, -retentionDays)
	end := today.AddDate(0, 0, -1)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		// Budget guard: the scan shares the job context, and each day can cost
		// several LLM pipeline runs. Once the budget is gone there is no point
		// starting another day — defer the rest to the next run (顺延不丢).
		if ctx.Err() != nil {
			remainingDays := int(end.Sub(day).Hours()/24) + 1
			logging.Warnf("daily-report: backfill budget exhausted after %s; %d day(s) remaining deferred to the next run",
				day.Format("2006-01-02"), remainingDays)
			break
		}
		boardIDs, collectErr := daily_report.CollectBoardIDsForDate(day)
		if collectErr != nil {
			logging.Warnf("daily-report: backfill board collection failed for %s: %v", day.Format("2006-01-02"), collectErr)
			continue
		}
		for _, boardID := range boardIDs {
			exists, existsErr := topicgraphrepo.Repo.ReportExistsForBoardDate(boardID, day)
			if existsErr != nil {
				logging.Warnf("daily-report: backfill existence check failed for board %d on %s: %v",
					boardID, day.Format("2006-01-02"), existsErr)
				continue
			}
			if exists {
				continue // 只补缺不重建已有：重生成是整条 LLM 流水线且会覆盖既有报告
			}
			report, genErr := generateAndSaveReport(ctx, boardID, day)
			if genErr != nil {
				logging.Warnf("daily-report: backfill failed for board %d on %s: %v",
					boardID, day.Format("2006-01-02"), genErr)
				continue // 单板块失败不阻塞兄弟板块
			}
			if report == nil {
				continue
			}
			backfilled++
		}
	}

	data["backfilled_count"] = backfilled
	data["backfill_window_start"] = start.Format("2006-01-02")
	data["backfill_window_end"] = end.Format("2006-01-02")
	return backfilled, data
}

// countUnfinishedTagJobs counts the tag_jobs rows awaiting processing: pending
// (queued) and leased (in flight), plus failed (terminally broken). Any
// pending/leased row means the drain has not completed yet; failed is reported
// separately because it never drains and must not block the catch-up.
func countUnfinishedTagJobs() (pending, leased, failed int64, err error) {
	db := repository.Repo.DB()
	if err := db.Model(&models.TagJob{}).
		Where("status = ?", models.JobStatusPending).
		Count(&pending).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("count pending tag jobs: %w", err)
	}
	if err := db.Model(&models.TagJob{}).
		Where("status = ?", models.JobStatusLeased).
		Count(&leased).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("count leased tag jobs: %w", err)
	}
	if err := db.Model(&models.TagJob{}).
		Where("status = ?", models.JobStatusFailed).
		Count(&failed).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("count failed tag jobs: %w", err)
	}
	return pending, leased, failed, nil
}

// DailyReportSchedulerWrapper wraps BaseScheduler to add TriggerNowWithDate.
type DailyReportSchedulerWrapper struct {
	*BaseScheduler
	targetDateFn func() time.Time // default target date (today), overridable
}

// NewDailyReportSchedulerWrapper creates a DailyReportSchedulerWrapper that
// embeds BaseScheduler and adds TriggerNowWithDate support.
func NewDailyReportSchedulerWrapper(bs *BaseScheduler) *DailyReportSchedulerWrapper {
	return &DailyReportSchedulerWrapper{
		BaseScheduler: bs,
		targetDateFn:  func() time.Time { return time.Now().In(time.Local) },
	}
}

// TriggerNowWithDate triggers daily report generation for a specific date.
// This is accessed by the handler via type assertion on the Scheduler interface.
func (d *DailyReportSchedulerWrapper) TriggerNowWithDate(dateStr string) map[string]interface{} {
	if !d.TrySetExecuting() {
		return map[string]interface{}{
			"accepted":    false,
			"started":     false,
			"reason":      "already_running",
			"message":     "日报生成正在执行中，请稍后再试。",
			"status_code": http.StatusConflict,
		}
	}

	targetDate := d.targetDateFn()
	if dateStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			d.ClearExecuting()
			return map[string]interface{}{
				"accepted":    false,
				"started":     false,
				"reason":      "invalid_date",
				"message":     "日期格式无效，请使用 YYYY-MM-DD。",
				"status_code": http.StatusBadRequest,
			}
		}
		targetDate = parsed

		// Rebuild window guard (design D6): a date older than the retention
		// window has had its edges reclaimed, so rebuilding it would write an
		// empty-candidate report over a possibly good existing one. Same
		// predicate + wording as POST /api/daily-reports/generate.
		retentionDays := tagging.LoadTagEdgeRetentionDays(repository.Repo.DB())
		if daily_report.IsDateOutsideRebuildWindow(targetDate, time.Now(), retentionDays) {
			d.ClearExecuting()
			return map[string]interface{}{
				"accepted":    false,
				"started":     false,
				"reason":      "out_of_retention_window",
				"message":     daily_report.RebuildWindowRejectionMessage(retentionDays),
				"status_code": http.StatusBadRequest,
			}
		}
	}

	go func() {
		defer func() {
			d.ClearExecuting()
			if r := recover(); r != nil {
				logging.Errorf("PANIC in manual daily-report trigger: %v", r)
			}
		}()
		d.executeWithDate(targetDate)
	}()

	return map[string]interface{}{
		"accepted": true,
		"started":  true,
		"reason":   "manual_run_started",
		"message":  fmt.Sprintf("日报生成已经开始运行（目标日期: %s）。", targetDate.Format("2006-01-02")),
	}
}

func (d *DailyReportSchedulerWrapper) executeWithDate(targetDate time.Time) {
	job := DailyReportJob(targetDate)
	result, err := job(context.Background())
	if err != nil {
		logging.Errorf("Daily report job failed: %v", err)
	} else if result != nil {
		logging.Infof("Daily report: %s", result.Summary)
	}
}
