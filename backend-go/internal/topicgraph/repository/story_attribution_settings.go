package repository

import (
	"strings"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── 事件线在线生产停写开关（retire-story-adopt-event-metrics D1/D2）─────────
//
// 唯一在线写入入口是 service/daily_report_story_highlights.go:boundedReportAttribution；
// 它读本开关决定是否继续产生 board_stories / story_signals /
// daily_report_threads.story_id。开关走 ai_settings 布尔键（可在线改、可灰度、
// 回滚不需要发版也不需要数据修复）；实现代码与表一律保留。

const (
	// StoryAttributionEnabledKey is the ai_settings boolean key that gates the
	// online story attribution step.
	StoryAttributionEnabledKey = "story_attribution_enabled"

	// DefaultStoryAttributionEnabled keeps pre-change behavior when the key is
	// absent: an existing deployment that has never touched the key keeps
	// writing story identity exactly as before (rollback = delete the key or
	// set it back to true).
	DefaultStoryAttributionEnabled = true
)

// ParseStoryAttributionEnabled parses a stored boolean. The bool is false for
// unrecognized values — those fall back to the documented default with a
// warning instead of silently deciding the opposite of the operator's intent.
func ParseStoryAttributionEnabled(raw string) (enabled bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "t", "yes", "on":
		return true, true
	case "false", "0", "f", "no", "off":
		return false, true
	default:
		// Anything else (blank, a number, a typo) is a bad stored value: fall
		// back to the default instead of guessing the operator's intent.
		return DefaultStoryAttributionEnabled, false
	}
}

// StoryAttributionEnabled reads the stop-write switch: missing key → default
// (keep writing, behavior unchanged), bad stored value → warn and fall back to
// the default. A read error is also reported through the bool so the caller
// never blocks report generation on a settings lookup.
func (r *TopicGraphRepository) StoryAttributionEnabled() bool {
	var setting models.AISettings
	if err := r.db.Where("key = ?", StoryAttributionEnabledKey).First(&setting).Error; err != nil {
		return DefaultStoryAttributionEnabled
	}
	enabled, ok := ParseStoryAttributionEnabled(setting.Value)
	if !ok {
		logging.Warnf("daily-report: invalid %s=%q, falling back to %t (behavior unchanged)",
			StoryAttributionEnabledKey, setting.Value, DefaultStoryAttributionEnabled)
	}
	return enabled
}
