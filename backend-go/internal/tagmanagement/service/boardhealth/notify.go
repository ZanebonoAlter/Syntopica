package boardhealth

import (
	"context"
	"strconv"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/notification"
)

// notification link contract: LinkType="board" + LinkID=<board id decimal> —
// the notification center jumps straight to the board page.
const notifyLinkType = "board"

// notifyBoardHealthAlert is the seam over notification.BoardHealthAlert so
// unit tests can observe notification writes without the real mailbox.
var notifyBoardHealthAlert = notification.BoardHealthAlert

// lastBoardAlertAt returns the most recent board-health alert notification
// time for the board, or nil when none exists (cooldown anchor, design D6).
func lastBoardAlertAt(ctx context.Context, db *gorm.DB, boardID uint) (*time.Time, error) {
	var notif models.Notification
	err := db.WithContext(ctx).
		Where("link_type = ? AND link_id = ?", notifyLinkType, strconv.FormatUint(uint64(boardID), 10)).
		Order("created_at DESC").
		First(&notif).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	createdAt := notif.CreatedAt
	return &createdAt, nil
}

// shouldNotify adjudicates the alert decision (design D6):
//   - degraded rounds (score NULL) never notify — alerts only fire on rounds
//     that actually have a score;
//   - score >= AlertThreshold never notifies (strictly-below semantics: a
//     score exactly on the line is not an alert);
//   - the LLM notify verdict must be true;
//   - a prior alert inside NotifyCooldownDays suppresses (strict <: exactly
//     cooldown days later a new alert may fire again).
func shouldNotify(score *float64, llmNotify bool, config Config, lastAlert *time.Time, now time.Time) bool {
	if score == nil {
		return false
	}
	if *score >= config.AlertThreshold {
		return false
	}
	if !llmNotify {
		return false
	}
	if lastAlert != nil && now.Sub(*lastAlert) < time.Duration(config.NotifyCooldownDays)*24*time.Hour {
		return false
	}
	return true
}

// notifyIfAlert evaluates the alert path for one persisted snapshot and
// creates the notification when warranted. Failures are logged only (the
// snapshot is already durable; a mailbox hiccup must not fail the round).
func notifyIfAlert(ctx context.Context, db *gorm.DB, config Config, board *models.SemanticLabel, score *float64, llmNotify bool, now time.Time) {
	if !shouldNotify(score, llmNotify, config, nil, now) {
		return
	}
	lastAlert, err := lastBoardAlertAt(ctx, db, board.ID)
	if err != nil {
		logging.Warnf("[board-health] cooldown query failed for board %d, skipping alert this round: %v", board.ID, err)
		return
	}
	if !shouldNotify(score, llmNotify, config, lastAlert, now) {
		return
	}
	var comment string
	if board.Description != "" {
		comment = board.Description
	}
	notifyBoardHealthAlert(board.ID, board.Label, score, comment)
}
