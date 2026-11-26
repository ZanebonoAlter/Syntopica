package repository

import (
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 人工修正的留痕历史（review R-2）。
//
// 两张 append-only 表各有一条读出口：
//
//	topic_anchor_overrides      —— 首义改写留痕（每改一次追加一行）
//	section_assignment_overrides —— 泳道归属变更留痕（移除 / 加回各追加一行）
//
// 两条留痕的写入方都在**同一个事务**里跟事实列一起落库（append-only：既有行
// 既不改也不删），因此「移除 → 加回」两步各自留一行，「这条内容为什么被挪
// 走」在加回之后依然答得出来。
//
// 读出口一律按时间升序（created_at ASC, id ASC）：同毫秒追加的行靠 id 兜底
// 排序，不会出现「同一秒内顺序随机」。

// vectorDigestHead is how many leading components a vector digest carries.
// The digest is a **summary**, not the vector: a 768-dim embedding would blow
// up every history response, while the leading components + norm are enough
// to tell two anchors apart.
const vectorDigestHead = 8

// VectorDigest is a compact summary of one embedding.
type VectorDigest struct {
	// Dimension is the vector length (e.g. 768).
	Dimension int `json:"dimension"`
	// Head carries the first few components, rounded to 6 decimals.
	Head []float64 `json:"head"`
	// Norm is the L2 norm of the whole vector.
	Norm float64 `json:"norm"`
}

// AnchorOverrideHistoryEntry is one row of a topic's 首义 rewrite history.
type AnchorOverrideHistoryEntry struct {
	ID uint `json:"id"`
	// TopicID is the rewritten topic.
	TopicID uint `json:"topic_id"`
	// Source is the input mode ("selected_sections").
	Source string `json:"source"`
	// SectionIDs are the chosen nodes of that rewrite (from the row's input
	// summary). Empty when the summary is unreadable — an unreadable trail
	// must not turn into an invented node list.
	SectionIDs []uint `json:"section_ids"`
	// OldVector / NewVector summarize the vector before / after the rewrite.
	// OldVector is nil when the topic had no usable vector yet.
	OldVector *VectorDigest `json:"old_vector"`
	NewVector *VectorDigest `json:"new_vector"`
	// Operator recorded who made the change (single-user deployment → "user").
	Operator string `json:"operator"`
	// CreatedAt is when the rewrite happened.
	CreatedAt time.Time `json:"created_at"`
}

// SectionAssignmentHistoryEntry is one row of a node's lane membership history.
type SectionAssignmentHistoryEntry struct {
	ID uint `json:"id"`
	// SectionID is the moved node.
	SectionID uint `json:"section_id"`
	// TopicIDFrom is the lane the node came from (NULL = 此前无归属).
	TopicIDFrom *uint `json:"topic_id_from"`
	// TopicIDTo is the lane the node went to (NULL = 移除).
	TopicIDTo *uint `json:"topic_id_to"`
	// Action is "detach" or "attach".
	Action string `json:"action"`
	// Operator recorded who made the change.
	Operator string `json:"operator"`
	// CreatedAt is when the change happened.
	CreatedAt time.Time `json:"created_at"`
}

// appendAssignmentOverride appends one immutable row to the lane-membership
// trail. It MUST be called inside the same transaction as the fact-column
// write: either both land or neither does (留痕与归属变更同生共死).
func (r *TopicGraphRepository) appendAssignmentOverride(tx *gorm.DB, sectionID uint, topicIDFrom, topicIDTo *uint, action string) error {
	row := SectionAssignmentOverride{
		SectionID:   sectionID,
		TopicIDFrom: topicIDFrom,
		TopicIDTo:   topicIDTo,
		Action:      action,
		Operator:    AnchorOperatorUser,
	}
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("append assignment override for section %d: %w", sectionID, err)
	}
	return nil
}

// ListTopicAnchorOverrideHistory returns every manual 首义 rewrite of a topic,
// oldest first.
//
// Read-only. The trail outlives the topic: a hard-deleted topic still has its
// rows, so this returns them instead of 404 (the audit must not vanish with
// the data it audits).
func (r *TopicGraphRepository) ListTopicAnchorOverrideHistory(topicID uint) ([]AnchorOverrideHistoryEntry, error) {
	var rows []TopicAnchorOverride
	if err := r.db.Where("topic_id = ?", topicID).
		Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list anchor override history for topic %d: %w", topicID, err)
	}

	entries := make([]AnchorOverrideHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, AnchorOverrideHistoryEntry{
			ID:         row.ID,
			TopicID:    row.TopicID,
			Source:     row.Source,
			SectionIDs: parseAnchorInputSummary(row.InputSummary),
			OldVector:  vectorDigestOf(row.OldEmbedding),
			NewVector:  vectorDigestOf(&row.NewEmbedding),
			Operator:   row.Operator,
			CreatedAt:  row.CreatedAt,
		})
	}
	if len(entries) == 0 {
		// No trail: an unknown topic is a 404, a known one is an empty history.
		var count int64
		if err := r.db.Model(&BoardPersistentTopic{}).Where("id = ?", topicID).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("count topic %d: %w", topicID, err)
		}
		if count == 0 {
			return nil, ErrTopicNotFound
		}
	}
	return entries, nil
}

// ListSectionAssignmentHistory returns every manual lane-membership change of
// one node, oldest first.
//
// Read-only. Same rule as the anchor history: the trail outlives the node, so
// a hard-deleted node still yields its rows; an unknown node without a trail
// is a 404.
func (r *TopicGraphRepository) ListSectionAssignmentHistory(sectionID uint) ([]SectionAssignmentHistoryEntry, error) {
	var rows []SectionAssignmentOverride
	if err := r.db.Where("section_id = ?", sectionID).
		Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list assignment history for section %d: %w", sectionID, err)
	}

	entries := make([]SectionAssignmentHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, SectionAssignmentHistoryEntry(row))
	}
	if len(entries) == 0 {
		var count int64
		if err := r.db.Model(&DailyReportSection{}).Where("id = ?", sectionID).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("count section %d: %w", sectionID, err)
		}
		if count == 0 {
			return nil, ErrSectionNotFound
		}
	}
	return entries, nil
}

// vectorDigestOf summarizes a stored pgvector string. A nil / empty /
// unparsable vector yields nil — the digest layer must never invent a vector.
func vectorDigestOf(raw *string) *VectorDigest {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	vec, err := repoParsePgVector(trimmed)
	if err != nil || len(vec) == 0 {
		return nil
	}
	head := vec
	if len(head) > vectorDigestHead {
		head = head[:vectorDigestHead]
	}
	rounded := make([]float64, len(head))
	norm := 0.0
	for i, v := range head {
		rounded[i] = math.Round(v*1e6) / 1e6
	}
	for _, v := range vec {
		norm += v * v
	}
	return &VectorDigest{
		Dimension: len(vec),
		Head:      rounded,
		Norm:      math.Round(math.Sqrt(norm)*1e6) / 1e6,
	}
}
