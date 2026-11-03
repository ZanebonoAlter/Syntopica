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
//
// board-health-triage-advice seam A: the same pure-read round additionally
// computes three deterministic triage signals per composition label
// (magnet / pgvector neighbors / description word-root screen) plus the
// board-level failed-mount flow, feeds them into the SAME single LLM call
// (score + comment + notify + triage together), and persists the verdict
// list in metrics.triage — degraded rounds keep statistics and signals but
// drop score/comment/triage.
package boardhealth

import (
	"context"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ai_settings keys (design D8 + board-health-triage-advice D2). All optional —
// LoadConfig falls back to the defaults below on any missing/invalid value.
const (
	minSampleSizeKey    = "board_health_min_sample_size"
	alertThresholdKey   = "board_health_alert_threshold"
	notifyCooldownKey   = "board_health_notify_cooldown_days"
	memberCandidatesKey = "board_health_member_candidates_top"
	magnetGapKey        = "board_health_magnet_gap"
	magnetMinFailedKey  = "board_health_magnet_min_failed"
)

// Default configuration values (design D8; magnet defaults from
// board-health-triage-advice design D2 / Open Questions 参数化).
const (
	DefaultMinSampleSize       = 5
	DefaultAlertThreshold      = 60.0
	DefaultNotifyCooldownDays  = 3
	DefaultMemberCandidatesTop = 20
	DefaultMagnetGap           = 0.08
	DefaultMagnetMinFailed     = 5
)

// maxUncapturedCandidates caps the persisted candidate list (design D4 top 50).
const maxUncapturedCandidates = 50

// Config carries the six ai_settings-tunable knobs with defaults resolved.
type Config struct {
	MinSampleSize       int
	AlertThreshold      float64
	NotifyCooldownDays  int
	MemberCandidatesTop int
	// MagnetGap is the magnet-signal suspicion threshold: all_mean minus
	// failed_mean must reach it (design D2, >= semantics).
	MagnetGap float64
	// MagnetMinFailed is the minimum failed-subset sample count below which
	// the magnet signal is insufficient (never a suspicion verdict).
	MagnetMinFailed int
}

// LoadConfig reads the board-health keys from ai_settings, falling back to
// the design defaults on absent rows or invalid values (same pattern as
// board.LoadUpgradeConfig: defaults first, IN-query override).
func LoadConfig(ctx context.Context, db *gorm.DB) Config {
	config := Config{
		MinSampleSize:       DefaultMinSampleSize,
		AlertThreshold:      DefaultAlertThreshold,
		NotifyCooldownDays:  DefaultNotifyCooldownDays,
		MemberCandidatesTop: DefaultMemberCandidatesTop,
		MagnetGap:           DefaultMagnetGap,
		MagnetMinFailed:     DefaultMagnetMinFailed,
	}
	var settings []models.AISettings
	if err := db.WithContext(ctx).
		Where("key IN ?", []string{minSampleSizeKey, alertThresholdKey, notifyCooldownKey, memberCandidatesKey, magnetGapKey, magnetMinFailedKey}).
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
		case magnetGapKey:
			config.MagnetGap = parseFloatDefault(setting.Value, config.MagnetGap)
		case magnetMinFailedKey:
			config.MagnetMinFailed = parseIntDefault(setting.Value, config.MagnetMinFailed)
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
	// A-L1（review Low）：非有限值一律回退 fallback——NaN 与任何比较都为
	// false，会绕过下方 0..100 边界被静默当作阈值；±Inf 虽会被边界拦住，
	// 一并显式拒收。正常数值（含 0 与 100 边界）行为不变。
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || parsed > 100 {
		return fallback
	}
	return parsed
}
