package aisettings

import (
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── 泳道当天热度：候选泳道权重（retire-story-adopt-event-metrics D8）──────────
//
// 键与取值校验落在共享配置层，避免「热度计算（topicgraph/repository）」与
// 「匹配规则配置面（tagmanagement/handler）」各自维护一份键名/域校验。
// 该键只进「当天线索热度」，SHALL NOT 参与任何话题生命周期判定。

// LaneHeatCandidateWeightKey is the online-overridable candidate weight.
const LaneHeatCandidateWeightKey = "lane_heat_candidate_weight"

// DefaultLaneHeatCandidateWeight is the documented default (design D8).
// The key is a product taste, not a technical constant — it is adjustable
// online and SHALL NOT feed any topic-lifecycle judgement.
const DefaultLaneHeatCandidateWeight = 0.5

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
