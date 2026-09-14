package service

import (
	"fmt"
	"time"

	"syntopica-backend/internal/topicgraph/repository"
)

// IsDateOutsideRebuildWindow reports whether date falls before the daily report
// rebuild window lower bound: date < today - retentionDays calendar days
// (offline-catchup design D6).
//
// Both sides are passed through repository.NormalizeReportDate first so the
// comparison is calendar-day based no matter what clock time the callers carry
// (the HTTP handler parses a bare YYYY-MM-DD, the scheduler passes time.Now()).
//
// The lower bound itself is INSIDE the window — a date exactly retentionDays
// days back is still rebuildable. That mirrors the edge GC's strictly-older
// deletion (`created_at < cutoff` keeps an edge created exactly at the cutoff,
// design D3): if the edges for that day survive, the day is rebuildable.
//
// retentionDays <= 0 means "no usable window": callers feed this from
// tagging.LoadTagEdgeRetentionDays, which never returns <= 0, so there is
// nothing to enforce and nothing is rejected.
func IsDateOutsideRebuildWindow(date, today time.Time, retentionDays int) bool {
	if retentionDays <= 0 {
		return false
	}
	lowerBound := repository.NormalizeReportDate(today).AddDate(0, 0, -retentionDays)
	return repository.NormalizeReportDate(date).Before(lowerBound)
}

// RebuildWindowRejectionMessage is the user-facing explanation shared by both
// rebuild entry points (POST /api/daily-reports/generate and the scheduler's
// TriggerNowWithDate) so the two can never drift (design D6: same wording).
func RebuildWindowRejectionMessage(retentionDays int) string {
	return fmt.Sprintf(
		"该日期早于标签边保留窗口（%d 天），标签边已按窗口回收、候选不全，拒绝重建。",
		retentionDays,
	)
}
