// Package notification implements the persistent notification mailbox
// (add-notification-center). The whitelist is daily-report terminal states
// only — the exported surface intentionally exposes no generic Create, so
// high-frequency process events (tagging, firecrawl, auto-refresh) cannot
// produce rows (spec: 通知白名单——只日报生成终态).
//
// Delivery contract: write to the notifications table first, then broadcast
// the unified `notification` WS event. Offline coverage is free because the
// row persists (落库即触达). Write/broadcast failures are fail-open: they are
// logged and never block the calling business flow.
package notification

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/ws"
)

// MaxNotifications is the hard mailbox cap. Eviction runs inside the write
// path (not a cron): oldest read rows go first, oldest unread only when there
// are no read rows at all (test-cases 白盒 A).
const MaxNotifications = 500

// DailyReportCounts 是日报终态的**三态计数**，与 daily_report_done 终态消息
// 的字段一一对应（add-radar-pipeline-status D12）：
//
//   - TotalBoards: 本次运行涉及的板块总数（== WS 的 total_boards，**含**当日无内容的板块）
//   - Saved:       产出日报的板块数（== WS 的 total_saved）
//   - Failed:      生成报错、真正失败的板块数（== WS 的 failed_count）
//   - Empty:       当日无内容（report == nil）的板块数（== WS 的 empty_count）
//
// 三态互斥且穷尽：Saved + Failed + Empty == TotalBoards 恒成立。当日无内容的
// 板块既不算成功也不算失败——它计入总版面数，但不进成功数、不进失败数。
type DailyReportCounts struct {
	TotalBoards int
	Saved       int
	Failed      int
	Empty       int
}

// DailyReportTerminal 按三态口径写**一条**日报终态通知并广播它：
// Failed>0 → 一条失败汇总（type=error）；Failed==0 → 一条完成通知（type=success）。
// 两条通道互斥：一次运行至多一条终态通知（spec 白盒 B）。
//
// 文案与 daily_report_done 终态消息同源：「共 N 个版面」的 N 恒等于
// TotalBoards（含当日无内容的板块），成功数 == Saved、失败数 == Failed；
// 当日无内容的板块以「其中 M 个当日无内容」显式给出，且不进失败数。
// Fail-open: errors are logged, not returned.
func DailyReportTerminal(date time.Time, counts DailyReportCounts) {
	kind := "success"
	title := fmt.Sprintf("日报已生成 · %s", date.Format("2006-01-02"))
	summary := fmt.Sprintf("共 %d 个版面，%d 个版面已产出日报", counts.TotalBoards, counts.Saved)
	if counts.Failed > 0 {
		kind = "error"
		title = fmt.Sprintf("日报生成有失败 · %s", date.Format("2006-01-02"))
		summary = fmt.Sprintf("共 %d 个版面，%d 成功 %d 失败", counts.TotalBoards, counts.Saved, counts.Failed)
	}
	if counts.Empty > 0 {
		summary += fmt.Sprintf("，其中 %d 个当日无内容", counts.Empty)
	}
	createAndBroadcast(kind, title, summary+"。", "daily-report", date.Format("2006-01-02"))
}

// DailyReportSuccess creates the daily-report completion notification
// (type=success) and broadcasts it. Fail-open: errors are logged, not returned.
//
// totalBoards 是本次运行涉及的板块总数（**含**当日无内容的板块），totalSaved
// 是**产出日报的板块数**（不是条目数——旧文案写「保存 N 条条目」是误导）。
// 定时调度路径仍走本函数（无当日无内容计数）；手动 / 集合路径走
// DailyReportTerminal（三态口径，文案带当日无内容计数）。
func DailyReportSuccess(date time.Time, totalBoards, totalSaved int) {
	DailyReportTerminal(date, DailyReportCounts{TotalBoards: totalBoards, Saved: totalSaved})
}

// DailyReportFailedSummary creates ONE aggregated failure notification for a
// daily-report run with at least one failed board (never per-board), and
// broadcasts it. Mutually exclusive with DailyReportSuccess: when this fires,
// no completion notification is produced for the same run (spec 白盒 B).
//
// 本函数只有「成功数 / 失败数」两个计数，总版面数按 successCount+failedCount
// 推导（不含当日无内容的板块）——定时调度路径的旧口径。手动 / 集合路径走
// DailyReportTerminal，那里的总版面数与 WS 的 total_boards 恒等。
func DailyReportFailedSummary(date time.Time, successCount, failedCount int) {
	DailyReportTerminal(date, DailyReportCounts{
		TotalBoards: successCount + failedCount,
		Saved:       successCount,
		Failed:      failedCount,
	})
}

// BoardHealthAlert creates a board-health warning notification for a board
// whose checkup score fell below the alert threshold and whose LLM notify
// verdict was positive (add-board-health-checkup design D6). Reuses the
// "error" tier — the notification center renders a binary success/error
// style, and a health alert is user-attention-worthy, not celebratory.
// Fail-open like every mailbox writer: errors are logged, not returned.
// Callers own the cooldown: only boardhealth.notifyIfAlert may call this,
// after its cooldown query — do NOT call from anywhere else.
func BoardHealthAlert(boardID uint, boardLabel string, score *float64, llmComment string) {
	if score == nil {
		return // defensive: degraded rounds never alert
	}
	title := fmt.Sprintf("版块体检预警 · %s", boardLabel)
	comment := llmComment
	if comment == "" {
		comment = "体检评分低于告警线，建议检查构成标签质量。"
	}
	if len(comment) > 200 {
		comment = comment[:200] + "…"
	}
	summary := fmt.Sprintf("体检评分 %.0f。%s", *score, comment)
	createAndBroadcast("error", title, summary, "board", strconv.FormatUint(uint64(boardID), 10))
}

// createAndBroadcast persists the row, evicts over the cap, then broadcasts.
// Every failure path logs and returns — the mailbox must never break the
// business flow that reports into it.
func createAndBroadcast(kind, title, summary, linkType, linkID string) {
	notif, err := insertAndEvict(kind, title, summary, linkType, linkID)
	if err != nil {
		logging.Warnf("notification: write failed (fail-open): %v", err)
		return
	}
	broadcast(notif)
}

// insertAndEvict writes the row, then enforces the cap in the same path:
// DELETE keeps the newest MaxNotifications rows under the deletion-priority
// order (oldest read first, then oldest unread — test-cases 白盒 A1-A3).
func insertAndEvict(kind, title, summary, linkType, linkID string) (*models.Notification, error) {
	db := database.DB
	if db == nil {
		return nil, fmt.Errorf("notification: database not initialized")
	}

	notif := &models.Notification{
		Type:     kind,
		Title:    title,
		Summary:  summary,
		LinkType: linkType,
		LinkID:   linkID,
	}
	if err := db.Create(notif).Error; err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}

	if err := evictOverCap(db); err != nil {
		// Eviction failure is non-fatal for this write: the row is already in.
		logging.Warnf("notification: eviction failed (fail-open): %v", err)
	}
	return notif, nil
}

// evictOverCap deletes rows beyond the newest MaxNotifications under the
// deletion-priority order: oldest read rows evicted first (is_read DESC puts
// read rows first), then oldest unread (created_at ASC). One constraint
// DELETE — no row locks needed, single-user low-frequency writes (白盒 A4 划除).
func evictOverCap(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Notification{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count notifications: %w", err)
	}
	excess := count - MaxNotifications
	if excess <= 0 {
		return nil
	}
	// is_read DESC → read rows rank first for deletion; created_at ASC →
	// oldest first within each group; id ASC as deterministic tiebreaker.
	result := db.Exec(
		"DELETE FROM notifications WHERE id IN ("+
			"SELECT id FROM notifications ORDER BY is_read DESC, created_at ASC, id ASC LIMIT ?)",
		excess,
	)
	if result.Error != nil {
		return fmt.Errorf("evict notifications: %w", result.Error)
	}
	return nil
}

// broadcast pushes the unified `notification` event with the full payload.
func broadcast(notif *models.Notification) {
	data, err := json.Marshal(map[string]interface{}{
		"type": "notification",
		"data": notif,
	})
	if err != nil {
		logging.Warnf("notification: marshal broadcast payload (fail-open): %v", err)
		return
	}
	ws.GetHub().BroadcastRaw(data)
}

// List returns notifications newest-first. unreadOnly filters to unread rows.
func List(unreadOnly bool, limit, offset int) ([]models.Notification, int64, error) {
	db := database.DB
	if db == nil {
		return nil, 0, fmt.Errorf("notification: database not initialized")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := db.Model(&models.Notification{})
	if unreadOnly {
		query = query.Where("is_read = ?", false)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}
	var rows []models.Notification
	if err := query.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list notifications: %w", err)
	}
	return rows, total, nil
}

// UnreadCount returns the number of unread notifications.
func UnreadCount() (int64, error) {
	db := database.DB
	if db == nil {
		return 0, fmt.Errorf("notification: database not initialized")
	}
	var count int64
	if err := db.Model(&models.Notification{}).Where("is_read = ?", false).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return count, nil
}

// MarkRead marks one notification read. Idempotent: already-read rows are a
// no-op (test-cases D4). Returns the affected row; not found → nil, nil.
func MarkRead(id uint) (*models.Notification, error) {
	db := database.DB
	if db == nil {
		return nil, fmt.Errorf("notification: database not initialized")
	}
	var notif models.Notification
	if err := db.First(&notif, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("load notification: %w", err)
	}
	if notif.IsRead {
		return &notif, nil // idempotent no-op
	}
	now := time.Now()
	if err := db.Model(&notif).Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error; err != nil {
		return nil, fmt.Errorf("mark read: %w", err)
	}
	notif.IsRead = true
	notif.ReadAt = &now
	return &notif, nil
}

// MarkAllRead marks every unread row read. Second call affects 0 rows
// (test-cases V9). Returns the number of rows actually updated.
func MarkAllRead() (int64, error) {
	db := database.DB
	if db == nil {
		return 0, fmt.Errorf("notification: database not initialized")
	}
	result := db.Model(&models.Notification{}).
		Where("is_read = ?", false).
		Updates(map[string]interface{}{"is_read": true, "read_at": time.Now()})
	if result.Error != nil {
		return 0, fmt.Errorf("mark all read: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ClearAll removes every notification row.
func ClearAll() (int64, error) {
	db := database.DB
	if db == nil {
		return 0, fmt.Errorf("notification: database not initialized")
	}
	result := db.Where("1 = 1").Delete(&models.Notification{})
	if result.Error != nil {
		return 0, fmt.Errorf("clear notifications: %w", result.Error)
	}
	return result.RowsAffected, nil
}
