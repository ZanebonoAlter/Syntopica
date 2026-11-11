package repository

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── 泳道当天热度（retire-story-adopt-event-metrics D6/D7/D8）────────────────
//
// 唯一口径实现：泳道当天热度 = 该泳道当天线索数按话题状态加权；版块关注度 =
// 各泳道当天热度之和。只表达当天，不衰减、不跨天、不读任何事件身份。
//
// 与 story_heat.go（heat-v1-48h-hl24h，story_signals 证据衰减）完全独立的
// 版本号与实现：两套语义不共用函数，事后可按版本号区分读数来源。

const (
	// LaneHeatRuleVersion pins the same-day clue-count formula. It deliberately
	// replaces HeatRuleVersion (heat-v1-48h-hl24h) rather than versioning that
	// formula's lane half in place.
	LaneHeatRuleVersion = "heat-v2-lane-clues-1d"

	// LaneHeatCandidateWeightKey is the online-overridable candidate weight.
	LaneHeatCandidateWeightKey = "lane_heat_candidate_weight"

	// DefaultLaneHeatCandidateWeight is the documented default (design D8).
	// The key is a product taste, not a technical constant — it is adjustable
	// online and SHALL NOT feed any topic-lifecycle judgement.
	DefaultLaneHeatCandidateWeight = 0.5

	// activeLaneHeatWeight is the fixed weight of an active lane.
	activeLaneHeatWeight = 1.0
)

// ValidLaneHeatCandidateWeight enforces the documented domain: finite number in
// [0, 1]. 0 is legal and means "candidate lanes do not heat the board".
func ValidLaneHeatCandidateWeight(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}

// ParseLaneHeatCandidateWeight parses a stored/submitted weight value. The bool
// is false for anything outside the documented domain (bad stored values fall
// back to the default at the read point; bad submissions are rejected whole).
func ParseLaneHeatCandidateWeight(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || !ValidLaneHeatCandidateWeight(value) {
		return DefaultLaneHeatCandidateWeight, false
	}
	return value, true
}

// LoadLaneHeatCandidateWeight reads the online weight: missing → default,
// invalid stored value → warn and fall back to the default (never a silent 0,
// never a failed render).
func LoadLaneHeatCandidateWeight(db *gorm.DB) float64 {
	var setting models.AISettings
	if err := db.Where("key = ?", LaneHeatCandidateWeightKey).First(&setting).Error; err != nil {
		return DefaultLaneHeatCandidateWeight
	}
	value, ok := ParseLaneHeatCandidateWeight(setting.Value)
	if !ok {
		logging.Warnf("lane-heat: invalid %s=%q, falling back to %.2f",
			LaneHeatCandidateWeightKey, setting.Value, DefaultLaneHeatCandidateWeight)
		return DefaultLaneHeatCandidateWeight
	}
	return value
}

// ComputeLaneHeat is the ONE implementation of the same-day lane heat:
// active lanes count their clues at 1.0, candidate lanes at candidateWeight.
// The heat is a deterministic function of (status, clue count, weight): the
// same report data always yields the same value, and any historical day can be
// recomputed (nothing is persisted). Composite threads never reach this
// function — the clue query excludes them.
func ComputeLaneHeat(status string, clueCount int, candidateWeight float64) float64 {
	if clueCount <= 0 {
		return 0
	}
	weight := activeLaneHeatWeight
	if status == TopicStatusCandidate {
		weight = candidateWeight
		if !ValidLaneHeatCandidateWeight(weight) {
			weight = DefaultLaneHeatCandidateWeight
		}
	}
	return float64(clueCount) * weight
}

// ComputeBoardAttention sums the lanes' same-day heat and keeps one decimal
// (spec: 版块关注度 = Σ 泳道热度，保留一位小数).
func ComputeBoardAttention(laneHeats []float64) float64 {
	total := 0.0
	for _, heat := range laneHeats {
		total += heat
	}
	return math.Round(total*10) / 10
}

// CountLaneCluesByDate aggregates one board's clue (thread) count per topic
// (`daily_report_sections.persistent_topic_id`) for the given report day. It
// covers BOTH active and candidate topics: the candidate weight needs a real
// clue count, and candidates never appear in the lane set (design D7).
// Composite (综合稿) threads are excluded — a multi-event piece must not inflate
// any single lane's heat (spec: 综合稿的 thread SHALL NOT 计入线索数).
func (r *TopicGraphRepository) CountLaneCluesByDate(boardID uint, date time.Time) (map[uint]int, error) {
	day := NormalizeReportDate(date).Format(time.DateOnly)
	var rows []struct {
		TopicID uint `gorm:"column:topic_id"`
		Clues   int  `gorm:"column:clues"`
	}
	err := r.db.Table("daily_report_sections AS s").
		Select("s.persistent_topic_id AS topic_id, COUNT(t.id) AS clues").
		Joins("JOIN board_daily_reports AS r ON r.id = s.report_id").
		Joins("JOIN daily_report_threads AS t ON t.section_id = s.id").
		Where("r.semantic_board_id = ? AND r.period_date = ? AND s.persistent_topic_id IS NOT NULL", boardID, day).
		Where("COALESCE(t.scope, '') <> ?", ThreadScopeComposite).
		Group("s.persistent_topic_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("lane heat: clue counts board %d day %s: %w", boardID, day, err)
	}
	out := make(map[uint]int, len(rows))
	for _, row := range rows {
		out[row.TopicID] = row.Clues
	}
	return out, nil
}

// LaneHeatsByDate resolves the same-day heat of every topic that has clues on
// the anchor day, keyed by topic id. Topic status is read from the topics of
// this board only (archived topics are never heated — they are not lanes).
func (r *TopicGraphRepository) LaneHeatsByDate(boardID uint, date time.Time, candidateWeight float64) (map[uint]float64, error) {
	clues, err := r.CountLaneCluesByDate(boardID, date)
	if err != nil {
		return nil, err
	}
	out := make(map[uint]float64, len(clues))
	if len(clues) == 0 {
		return out, nil
	}
	ids := make([]uint, 0, len(clues))
	for id := range clues {
		ids = append(ids, id)
	}
	var topics []BoardPersistentTopic
	if err := r.db.Select("id", "status").Where("id IN ?", ids).Find(&topics).Error; err != nil {
		return nil, fmt.Errorf("lane heat: topic status: %w", err)
	}
	status := make(map[uint]string, len(topics))
	for _, topic := range topics {
		status[topic.ID] = topic.Status
	}
	for id, count := range clues {
		if status[id] == TopicStatusArchived {
			continue // 归档话题不是泳道，不参与当天热度
		}
		out[id] = ComputeLaneHeat(status[id], count, candidateWeight)
	}
	return out, nil
}
