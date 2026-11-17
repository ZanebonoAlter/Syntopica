package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// 首义来源（读侧口径）。
//
// 与写侧常量分开：写侧记录的是**输入模式**（"selected_sections"——首义由选定节点范围定义），
// 读侧回答的是「当前首义是怎么来的、对应哪些节点」：
//   - TopicAnchorSourceAuto：没有人工改写记录 → 首义是创建话题时写入的（无节点集）；
//   - TopicAnchorSourceManual：有人工改写记录 → 首义是人工选定的那组节点（节点集可查）。
//
// 「自动首义无节点集」是契约硬约束（lane-correction-surface「自动首义不虚构节点列表」）：
// 自动首义只有一条向量，没有对应的节点列表，界面不得替它编一个。
const (
	// TopicAnchorSourceAuto —— 首义来自创建话题时写入，无节点集。
	TopicAnchorSourceAuto = "auto"
	// TopicAnchorSourceManual —— 首义来自人工选定的节点范围，节点集见 SectionIDs。
	TopicAnchorSourceManual = "manual"
)

// TopicAnchorState is the read-side view of a topic's current 首义.
//
// 前端泳道修正工作面用它展示「当前首义来源」（自动 / 人工 + 节点数）并决定
// 首义选择池的边界（窗口内节点 + 当前首义集，含窗口外节点）。
type TopicAnchorState struct {
	TopicID uint `json:"topic_id"`
	// Source is "auto" or "manual"（见上方常量）。
	Source string `json:"source"`
	// SectionIDs is the current anchor node set. Empty for an auto anchor:
	// an auto anchor is a single vector, it has no node list to show.
	SectionIDs []uint `json:"section_ids"`
	// OverrideCount is how many times the topic's 首义 has been rewritten by hand.
	OverrideCount int `json:"override_count"`
	// EffectiveFrom marks when the current anchor took effect on the matching
	// side. "next_report" for a manual rewrite (the centroid is recomputed by
	// the next daily-report round); empty for an auto anchor (already in use).
	EffectiveFrom string `json:"effective_from"`
}

// GetTopicAnchorState reads the topic's current 首义 state.
//
// Read-only: it never writes and never recomputes anything. The audit trail
// (`topic_anchor_overrides`, append-only) is the source of truth for the
// manual case — the latest row carries the chosen section ids in
// `input_summary`; the auto case has no row and therefore no node set.
func (r *TopicGraphRepository) GetTopicAnchorState(topicID uint) (*TopicAnchorState, error) {
	var topic BoardPersistentTopic
	if err := r.db.Select("id").First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopicNotFound
		}
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}

	var count int64
	if err := r.db.Model(&TopicAnchorOverride{}).Where("topic_id = ?", topicID).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count anchor overrides for topic %d: %w", topicID, err)
	}

	state := &TopicAnchorState{
		TopicID:       topicID,
		Source:        TopicAnchorSourceAuto,
		SectionIDs:    []uint{},
		OverrideCount: int(count),
	}
	if count == 0 {
		return state, nil
	}

	var latest TopicAnchorOverride
	if err := r.db.Where("topic_id = ?", topicID).
		Order("created_at DESC, id DESC").
		First(&latest).Error; err != nil {
		return nil, fmt.Errorf("load latest anchor override for topic %d: %w", topicID, err)
	}
	state.Source = TopicAnchorSourceManual
	state.SectionIDs = parseAnchorInputSummary(latest.InputSummary)
	state.EffectiveFrom = AnchorEffectiveFromNextReport
	return state, nil
}

// parseAnchorInputSummary reads the audit row's「输入摘要」(the JSON array of
// chosen section ids) back into a list. A malformed summary yields an empty
// set rather than a fabricated one — an unreadable trail must not turn into
// an invented node list.
func parseAnchorInputSummary(raw string) []uint {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []uint{}
	}
	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		return []uint{}
	}
	return dedupeSectionIDs(ids)
}
