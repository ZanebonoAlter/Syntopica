// Package boardhealth implements the board health checkup
// (add-board-health-checkup): a daily pure-read statistics pass over every
// active board that runs after the daily report pipeline completes.
//
// The measurement rules (口径) have a single authority: this package —
// same discipline as sourcestats. Two levels per board:
//
//   - 版块级（口径①，全部匹配行）: matched tag count, match_reason
//     distribution, ghost matches (direction_mismatch), vector missing
//     count.
//   - 标签级（口径②修正，贴合度分布）: per composition label, cosine
//     distribution recomputed from the label's own embedding × EVERY
//     mounted tag's identity embedding — no aux-count filtering (the
//     original single-aux pure-sample口径 was retired 2026-10-08 after
//     real-data evidence showed single-aux tags almost never reach the
//     mounted set — see design D4). The low-sim line is relative: P25 of
//     the sufficient labels' cosine means within the board. Samples below
//     MinSampleSize (or a label missing its own embedding) mark the label
//     insufficient with null distribution fields. A mounted tag missing
//     its identity embedding is skipped for every label and counted into
//     VectorMissing (never zero-filled).
//
// The checkup never writes matching results and never creates upgrade
// suggestions; its only write is the board_health_snapshots row plus an
// optional LLM-adjudicated alert notification (cooled down per board).
package boardhealth

import (
	"context"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ai_settings keys (design D8). All optional — LoadConfig falls back to the
// defaults below on any missing/invalid value.
const (
	minSampleSizeKey    = "board_health_min_sample_size"
	alertThresholdKey   = "board_health_alert_threshold"
	notifyCooldownKey   = "board_health_notify_cooldown_days"
	memberCandidatesKey = "board_health_member_candidates_top"
)

// Default configuration values (design D8).
const (
	DefaultMinSampleSize       = 5
	DefaultAlertThreshold      = 60.0
	DefaultNotifyCooldownDays  = 3
	DefaultMemberCandidatesTop = 20
)

// maxUncapturedCandidates caps the persisted candidate list (design D4 top 50).
const maxUncapturedCandidates = 50

// Config carries the four ai_settings-tunable knobs with defaults resolved.
type Config struct {
	MinSampleSize       int
	AlertThreshold      float64
	NotifyCooldownDays  int
	MemberCandidatesTop int
}

// LoadConfig reads the four board-health keys from ai_settings, falling back
// to the design defaults on absent rows or invalid values (same pattern as
// board.LoadUpgradeConfig: defaults first, IN-query override).
func LoadConfig(ctx context.Context, db *gorm.DB) Config {
	config := Config{
		MinSampleSize:       DefaultMinSampleSize,
		AlertThreshold:      DefaultAlertThreshold,
		NotifyCooldownDays:  DefaultNotifyCooldownDays,
		MemberCandidatesTop: DefaultMemberCandidatesTop,
	}
	var settings []models.AISettings
	if err := db.WithContext(ctx).
		Where("key IN ?", []string{minSampleSizeKey, alertThresholdKey, notifyCooldownKey, memberCandidatesKey}).
		Find(&settings).Error; err != nil {
		return config
	}
	for _, setting := range settings {
		switch setting.Key {
		case minSampleSizeKey:
			config.MinSampleSize = parseIntDefault(setting.Value, config.MinSampleSize)
		case alertThresholdKey:
			config.AlertThreshold = parseFloatDefault(setting.Value, config.AlertThreshold)
		case notifyCooldownKey:
			config.NotifyCooldownDays = parseIntDefault(setting.Value, config.NotifyCooldownDays)
		case memberCandidatesKey:
			config.MemberCandidatesTop = parseIntDefault(setting.Value, config.MemberCandidatesTop)
		}
	}
	return config
}

func parseIntDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseFloatDefault(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || parsed > 100 {
		return fallback
	}
	return parsed
}
