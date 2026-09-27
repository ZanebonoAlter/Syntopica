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

// DailyReportSuccess creates the daily-report completion notification
// (type=success) and broadcasts it. Fail-open: errors are logged, not returned.
func DailyReportSuccess(date time.Time, totalBoards, totalSaved int) {
	title := fmt.Sprintf("日报已生成 · %s", date.Format("2006-01-02"))
	summary := fmt.Sprintf("共 %d 个版面，保存 %d 条条目。", totalBoards, totalSaved)
	createAndBroadcast("success", title, summary, "daily-report", date.Format("2006-01-02"))
}

// DailyReportFailedSummary creates ONE aggregated failure notification for a
// daily-report run with at least one failed board (never per-board), and
// broadcasts it. Mutually exclusive with DailyReportSuccess: when this fires,
// no completion notification is produced for the same run (spec 白盒 B).
func DailyReportFailedSummary(date time.Time, successCount, failedCount int) {
	title := fmt.Sprintf("日报生成有失败 · %s", date.Format("2006-01-02"))
	summary := fmt.Sprintf("共 %d 个版面，%d 成功 %d 失败。", successCount+failedCount, successCount, failedCount)
	createAndBroadcast("error", title, summary, "daily-report", date.Format("2006-01-02"))
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
