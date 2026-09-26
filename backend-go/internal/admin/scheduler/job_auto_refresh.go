package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/scheduler"
	"syntopica-backend/internal/platform/ws"
	feed "syntopica-backend/internal/reader"
)

// staleRefreshTimeout is the duration after which a "refreshing" feed is considered stale.
const staleRefreshTimeout = 2 * time.Minute

// maxConcurrentRefreshes caps the number of feeds refreshed in parallel.
// A slow/hung feed occupies only one slot, leaving the rest available so
// a single bad feed cannot starve or stall refreshes of other feeds.
const maxConcurrentRefreshes = 8

// refreshSemaphore limits concurrent feed refreshes globally.
var refreshSemaphore = make(chan struct{}, maxConcurrentRefreshes)

// AutoRefreshSummary contains the result metrics for an auto-refresh cycle.
type AutoRefreshSummary struct {
	TriggerSource          string `json:"trigger_source"`
	StartedAt              string `json:"started_at"`
	FinishedAt             string `json:"finished_at"`
	ScannedFeeds           int    `json:"scanned_feeds"`
	DueFeeds               int    `json:"due_feeds"`
	TriggeredFeeds         int    `json:"triggered_feeds"`
	AlreadyRefreshingFeeds int    `json:"already_refreshing_feeds"`
	StaleResetFeeds        int    `json:"stale_reset_feeds"`
	Reason                 string `json:"reason"`
}

// AutoRefreshJob runs a feed refresh cycle: scans feeds, checks which need
// refresh, resets stale "refreshing" feeds, and triggers async refreshes.
func AutoRefreshJob(ctx context.Context) (*scheduler.JobResult, error) {
	startTime := time.Now()
	feedService := feed.NewFeedService()

	var feeds []models.Feed
	if err := repository.Repo.DB().Where("refresh_interval > 0").Find(&feeds).Error; err != nil {
		return nil, fmt.Errorf("error querying feeds: %w", err)
	}

	summary := &AutoRefreshSummary{
		TriggerSource: "scheduled",
		StartedAt:     startTime.Format(time.RFC3339),
		ScannedFeeds:  len(feeds),
	}

	now := time.Now()
	summary.StaleResetFeeds = resetStaleRefreshingFeeds(now)

	for _, f := range feeds {
		if !needsRefresh(&f, now) {
			continue
		}
		summary.DueFeeds++
		if f.RefreshStatus == "refreshing" {
			summary.AlreadyRefreshingFeeds++
			continue
		}
		markFeedRefreshing(f.ID)
		go func(feedID uint) {
			refreshFeedAsync(ctx, feedID, feedService) // nosec G118 -- ctx is scheduler-scoped, not request-scoped
		}(f.ID)
		summary.TriggeredFeeds++
	}

	if summary.TriggeredFeeds > 0 {
		go broadcastRefreshCompletion(startTime, summary)
	}

	summary.FinishedAt = time.Now().Format(time.RFC3339)
	summary.Reason = autoRefreshJobReason(summary)

	return &scheduler.JobResult{
		Data: map[string]interface{}{
			"trigger_source":           summary.TriggerSource,
			"scanned_feeds":            summary.ScannedFeeds,
			"due_feeds":                summary.DueFeeds,
			"triggered_feeds":          summary.TriggeredFeeds,
			"already_refreshing_feeds": summary.AlreadyRefreshingFeeds,
			"stale_reset_feeds":        summary.StaleResetFeeds,
			"reason":                   summary.Reason,
		},
		Summary: fmt.Sprintf("triggered %d feeds (scanned=%d, due=%d, stale_reset=%d)",
			summary.TriggeredFeeds, summary.ScannedFeeds, summary.DueFeeds, summary.StaleResetFeeds),
	}, nil
}

func needsRefresh(feed *models.Feed, now time.Time) bool {
	if feed.LastRefreshAt == nil {
		return true
	}
	timeSinceRefresh := now.Sub(*feed.LastRefreshAt)
	interval := time.Duration(feed.RefreshInterval) * time.Minute
	return timeSinceRefresh >= interval
}

func resetStaleRefreshingFeeds(now time.Time) int {
	cutoff := now.Add(-staleRefreshTimeout)

	var staleFeeds []models.Feed
	if err := repository.Repo.DB().Model(&models.Feed{}).
		Where("refresh_status = ? AND last_refresh_at < ?", "refreshing", cutoff).
		Find(&staleFeeds).Error; err != nil {
		logging.Errorf("Error querying stale feeds: %v", err)
		return 0
	}

	for _, feed := range staleFeeds {
		if feed.LastRefreshAt != nil {
			staleDuration := now.Sub(*feed.LastRefreshAt)
			logging.Warnf("Feed %d stuck for %.1f minutes, resetting", feed.ID, staleDuration.Minutes())
		}
	}

	if len(staleFeeds) == 0 {
		return 0
	}

	result := repository.Repo.DB().Model(&models.Feed{}).
		Where("refresh_status = ? AND last_refresh_at < ?", "refreshing", cutoff).
		Updates(map[string]interface{}{
			"refresh_status": "idle",
			"refresh_error":  "stale refreshing state reset after 5 minutes",
		})
	return int(result.RowsAffected)
}

func markFeedRefreshing(feedID uint) {
	repository.Repo.DB().Model(&models.Feed{}).Where("id = ?", feedID).Updates(map[string]interface{}{
		"refresh_status": "refreshing",
		"refresh_error":  "",
	})
}

func refreshFeedAsync(ctx context.Context, feedID uint, feedService *feed.FeedService) {
	// Acquire a slot so that one slow/hung feed cannot exhaust upstream
	// capacity or starve refreshes of other feeds.
	refreshSemaphore <- struct{}{}
	defer func() { <-refreshSemaphore }()

	defer func() {
		if r := recover(); r != nil {
			logging.Errorf("PANIC in refreshFeedAsync for feed %d: %v", feedID, r)
			resetFeedStatus(feedID, fmt.Sprintf("panic: %v", r))
		}
	}()

	if err := feedService.RefreshFeed(ctx, feedID); err != nil {
		logging.Errorf("Error refreshing feed %d: %v", feedID, err)
		resetFeedStatus(feedID, err.Error())
	}
}

func resetFeedStatus(feedID uint, errMsg string) {
	now := time.Now().In(models.ShanghaiTZ)
	repository.Repo.DB().Model(&models.Feed{}).Where("id = ? AND refresh_status = ?", feedID, "refreshing").Updates(map[string]interface{}{
		"refresh_status":  "error",
		"refresh_error":   errMsg,
		"last_refresh_at": &now,
	})
}

func broadcastRefreshCompletion(startTime time.Time, summary *AutoRefreshSummary) {
	duration := time.Since(startTime).Seconds()
	msg := ws.AutoRefreshCompleteMessage{
		Type:            "auto_refresh_complete",
		TriggeredFeeds:  summary.TriggeredFeeds,
		StaleResetFeeds: summary.StaleResetFeeds,
		DurationSeconds: duration,
		Timestamp:       time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		logging.Warnf("Auto-refresh completion message marshal failed: %v", err)
	} else {
		ws.GetHub().BroadcastRaw(data)
	}
}

func autoRefreshJobReason(summary *AutoRefreshSummary) string {
	switch {
	case summary.ScannedFeeds == 0:
		return "no_feeds_enabled"
	case summary.StaleResetFeeds > 0 && summary.TriggeredFeeds > 0:
		return "stale_reset_and_feeds_triggered"
	case summary.StaleResetFeeds > 0:
		return "stale_reset"
	case summary.TriggeredFeeds > 0:
		return "feeds_triggered"
	case summary.DueFeeds == 0:
		return "no_feeds_due"
	case summary.AlreadyRefreshingFeeds > 0:
		return "all_due_feeds_already_refreshing"
	default:
		return "scan_complete"
	}
}
