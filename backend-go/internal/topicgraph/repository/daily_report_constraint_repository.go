package repository

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// 辅助标签约束的仓储层（topic-lane-auxiliary-label-constraint tasks 1.3/4.1）。
//
// 三条不可偏离的约束（见 proposal.md「三条不可偏离的实现约束」与 design D1/D7）：
//   - 约束以向量为载体，仓储不存 tag id 集合（tag 边 7 天 GC 会让 id 集合自行失效）；
//   - margin / max_distance 可空（空 = 取 ai_settings 全局默认，design D4）；
//   - 判定输入 MUST NOT 取 daily_report_sections.cluster_tag_ids（实测 92.8% 悬空），
//     一律经 article_topic_tags 取有效 tag（悬空率 0%）。

// ErrLaneConstraintNotFound is returned when a constraint row is absent (the
// handler maps it to 404 on a delete-by-id / topic lookup miss).
var ErrLaneConstraintNotFound = errors.New("lane constraint not found")

// LaneConstraintInput is one constraint in a full-replacement request. The
// anchor vector is supplied by the service layer (airouter), never by the
// client. Margin / MaxDistance nil = take the global defaults.
type LaneConstraintInput struct {
	Term           string
	Mode           string
	Margin         *float64
	MaxDistance    *float64
	TermEmbedding  string
	EmbeddingModel string
}

// LaneLabelVector is one tag vector available for constraint judgement
// together with the vector space it belongs to. Judgement SKIPS labels whose
// Model differs from the anchor's (design D7).
type LaneLabelVector struct {
	TagID uint
	Model string
	// Vector is the parsed identity/semantic vector; nil when the stored
	// literal could not be parsed (treated as "no usable vector").
	Vector []float64
}

// GetTopic loads one persistent topic by id. Returns gorm.ErrRecordNotFound
// when absent so the handler can answer 404 (spec「话题不存在返回 404」).
func (r *TopicGraphRepository) GetTopic(topicID uint) (*BoardPersistentTopic, error) {
	var topic BoardPersistentTopic
	if err := r.db.First(&topic, topicID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}
	return &topic, nil
}

// ListTopicLaneConstraints returns one topic's constraints ordered by id
// (stable, creation order = the order the user bound them).
func (r *TopicGraphRepository) ListTopicLaneConstraints(topicID uint) ([]TopicLaneConstraint, error) {
	var rows []TopicLaneConstraint
	if err := r.db.Where("topic_id = ?", topicID).
		Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list topic %d lane constraints: %w", topicID, err)
	}
	return rows, nil
}

// ListTopicLaneConstraintsByTopicIDs batches the same read for the lane
// pipeline (one query per report instead of one per topic). Topics without
// constraints are absent from the map — the gate then passes them through with
// zero extra work (spec「无约束话题行为不变」).
func (r *TopicGraphRepository) ListTopicLaneConstraintsByTopicIDs(topicIDs []uint) (map[uint][]TopicLaneConstraint, error) {
	out := make(map[uint][]TopicLaneConstraint, len(topicIDs))
	if len(topicIDs) == 0 {
		return out, nil
	}
	var rows []TopicLaneConstraint
	if err := r.db.Where("topic_id IN ?", topicIDs).
		Order("topic_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list lane constraints for %d topics: %w", len(topicIDs), err)
	}
	for _, row := range rows {
		out[row.TopicID] = append(out[row.TopicID], row)
	}
	return out, nil
}

// ReplaceTopicLaneConstraints replaces a topic's whole constraint set inside
// ONE transaction (spec「批量替换幂等」/「非法载荷不部分写入」).
//
// 语义是**全量覆盖**：本次提交里没有的 term 被删除，已有的 term 原地更新
// （id / created_at 保留），新 term 插入。任一行非法（mode 非法、margin /
// max_distance 越界、term 重复）都会让整笔事务回滚——约束集合保持提交前的状态，
// 不会留下半套写入。非法判定由 DB 的 CHECK / 唯一索引兜底（服务层先做一次
// 快速校验给出可读错误，DB 约束是最后一道闸）。
func (r *TopicGraphRepository) ReplaceTopicLaneConstraints(topicID uint, inputs []LaneConstraintInput) error {
	if err := ValidateLaneConstraintInputs(inputs); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing []TopicLaneConstraint
		if err := tx.Where("topic_id = ?", topicID).Find(&existing).Error; err != nil {
			return fmt.Errorf("load existing lane constraints: %w", err)
		}
		byTerm := make(map[string]TopicLaneConstraint, len(existing))
		for _, row := range existing {
			byTerm[row.Term] = row
		}
		keep := make(map[uint]bool, len(inputs))
		for _, in := range inputs {
			if row, ok := byTerm[in.Term]; ok {
				keep[row.ID] = true
				updates := map[string]interface{}{
					"mode":         in.Mode,
					"margin":       in.Margin,
					"max_distance": in.MaxDistance,
					"updated_at":   gorm.Expr("now()"),
				}
				// The anchor only changes when the caller supplied a new one
				// (service reuses the stored anchor for an unchanged term — the
				// anchor is a pure function of term, and reusing it keeps the
				// vector space stable when the embedding route has degraded).
				if in.TermEmbedding != "" {
					updates["term_embedding"] = in.TermEmbedding
					updates["embedding_model"] = in.EmbeddingModel
				}
				if err := tx.Model(&TopicLaneConstraint{}).Where("id = ?", row.ID).
					Updates(updates).Error; err != nil {
					return fmt.Errorf("update lane constraint %d (%s): %w", row.ID, in.Term, err)
				}
				continue
			}
			row := TopicLaneConstraint{
				TopicID:        topicID,
				Term:           in.Term,
				TermEmbedding:  in.TermEmbedding,
				EmbeddingModel: in.EmbeddingModel,
				Mode:           in.Mode,
				Margin:         in.Margin,
				MaxDistance:    in.MaxDistance,
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("create lane constraint %q: %w", in.Term, err)
			}
		}
		// Drop the ones the caller omitted (deletion = omission).
		dropIDs := make([]uint, 0, len(existing))
		for _, row := range existing {
			if !keep[row.ID] {
				dropIDs = append(dropIDs, row.ID)
			}
		}
		if len(dropIDs) > 0 {
			if err := tx.Where("id IN ?", dropIDs).Delete(&TopicLaneConstraint{}).Error; err != nil {
				return fmt.Errorf("delete omitted lane constraints: %w", err)
			}
		}
		return nil
	})
}

// DeleteTopicLaneConstraint removes one constraint by (topic, id). Returns
// ErrLaneConstraintNotFound when the row does not belong to that topic.
func (r *TopicGraphRepository) DeleteTopicLaneConstraint(topicID, constraintID uint) error {
	res := r.db.Where("id = ? AND topic_id = ?", constraintID, topicID).
		Delete(&TopicLaneConstraint{})
	if res.Error != nil {
		return fmt.Errorf("delete lane constraint %d: %w", constraintID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrLaneConstraintNotFound
	}
	return nil
}

// validateLaneConstraintInputs mirrors the DB CHECK constraints in Go so the
// caller gets a readable error before touching the table. An empty mode is
// filled with "required" (the column default).
//
// Exported as ValidateLaneConstraintInputs because the service layer validates
// the very same rules before computing anchor vectors (一个真源，两处调用).
func ValidateLaneConstraintInputs(inputs []LaneConstraintInput) error {
	seen := make(map[string]bool, len(inputs))
	for i := range inputs {
		in := &inputs[i]
		in.Term = strings.TrimSpace(in.Term)
		if in.Term == "" {
			return fmt.Errorf("lane constraint #%d: term is required", i+1)
		}
		if len([]rune(in.Term)) > LaneConstraintTermMaxLen {
			return fmt.Errorf("lane constraint %q: term exceeds %d characters", in.Term, LaneConstraintTermMaxLen)
		}
		if seen[in.Term] {
			return fmt.Errorf("lane constraint %q: duplicate term in one request", in.Term)
		}
		seen[in.Term] = true
		if in.Mode == "" {
			in.Mode = LaneConstraintModeRequired
		}
		if in.Mode != LaneConstraintModeRequired && in.Mode != LaneConstraintModePreferred {
			return fmt.Errorf("lane constraint %q: invalid mode %q", in.Term, in.Mode)
		}
		if in.Margin != nil && (*in.Margin < 0 || *in.Margin > 1) {
			return fmt.Errorf("lane constraint %q: margin %v out of range [0,1]", in.Term, *in.Margin)
		}
		if in.MaxDistance != nil && (*in.MaxDistance < 0 || *in.MaxDistance > 1) {
			return fmt.Errorf("lane constraint %q: max_distance %v out of range [0,1]", in.Term, *in.MaxDistance)
		}
	}
	return nil
}

// liveTagVectorsJoin is the single predicate that decides whether a tag is
// still "live" for constraint judgement: it MUST be backed by at least one
// article_topic_tags edge (悬空率 0%). daily_report_sections.cluster_tag_ids
// is deliberately NOT used — it points at deleted rows for 92.8% of historical
// sections (14679/15820), so judging on it would silently no-op the gate.
const liveTagVectorsJoin = `JOIN topic_tags tt ON tt.id = e.topic_tag_id
	WHERE e.embedding_type = 'semantic'
	  AND EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = e.topic_tag_id)
	  AND e.topic_tag_id IN ?`

// ListLaneConstraintLabelVectors loads the semantic-track vector + model of
// each requested tag, keeping only tags that still exist AND still carry an
// article edge (see liveTagVectorsJoin). This is the gate's judgement input
// (tasks 4.1/4.2): dangling ids, deleted tags and GC'd edges simply vanish,
// so they can neither satisfy nor violate a constraint.
func (r *TopicGraphRepository) ListLaneConstraintLabelVectors(tagIDs []uint) (map[uint]LaneLabelVector, error) {
	out := make(map[uint]LaneLabelVector, len(tagIDs))
	if len(tagIDs) == 0 {
		return out, nil
	}
	type row struct {
		TopicTagID uint
		Embedding  string
		Model      string
	}
	var rows []row
	if err := r.db.Raw(`
		SELECT e.topic_tag_id AS topic_tag_id, e.embedding AS embedding, e.model AS model
		FROM topic_tag_embeddings e
		`+liveTagVectorsJoin, tagIDs).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list lane constraint label vectors: %w", err)
	}
	for _, rw := range rows {
		if rw.Embedding == "" {
			continue
		}
		vec, perr := repoParsePgVector(rw.Embedding)
		if perr != nil || len(vec) == 0 {
			continue
		}
		out[rw.TopicTagID] = LaneLabelVector{TagID: rw.TopicTagID, Model: rw.Model, Vector: vec}
	}
	return out, nil
}

// SampleLiveTagEmbeddingDimension returns the stored vector dimension of one
// live tag vector computed by the given model (0 when there is no sample yet).
// The constraint write path uses it to reject an anchor whose dimension does
// not match the existing space (spec「计算失败不落库」的维度不一致分支) — a
// mismatched anchor would silently never match a tag.
func (r *TopicGraphRepository) SampleLiveTagEmbeddingDimension(model string) (int, error) {
	var dims []int
	if err := r.db.Raw(`
		SELECT DISTINCT vector_dims(e.embedding) AS dim
		FROM topic_tag_embeddings e
		JOIN topic_tags tt ON tt.id = e.topic_tag_id
		WHERE e.embedding_type = 'semantic'
		  AND e.model = ?
		  AND e.embedding IS NOT NULL
		  AND EXISTS (SELECT 1 FROM article_topic_tags att WHERE att.topic_tag_id = e.topic_tag_id)
		LIMIT 2`, model).Scan(&dims).Error; err != nil {
		return 0, fmt.Errorf("sample tag embedding dimension for model %q: %w", model, err)
	}
	if len(dims) == 0 {
		return 0, nil
	}
	return dims[0], nil
}

// ResolveSectionValidTagIDs resolves each section's *effective* tag ids
// without touching cluster_tag_ids: section → threads.related_article_ids →
// article_topic_tags → existing topic_tags (0% dangling). Sections whose
// threads carry no article reference fall back to intersecting
// cluster_tag_ids with still-existing tags — a dangling id can never survive
// either path (tasks 4.1).
func (r *TopicGraphRepository) ResolveSectionValidTagIDs(sectionIDs []uint) (map[uint][]uint, error) {
	out := make(map[uint][]uint, len(sectionIDs))
	if len(sectionIDs) == 0 {
		return out, nil
	}
	type sectionTag struct {
		SectionID uint
		TagID     uint
	}
	var resolved []sectionTag
	if err := r.db.Raw(`
		SELECT DISTINCT t.section_id AS section_id, att.topic_tag_id AS tag_id
		FROM daily_report_threads t
		JOIN LATERAL jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(t.related_article_ids) = 'array'
			     THEN t.related_article_ids ELSE '[]'::jsonb END
		) AS a(article_id) ON true
		JOIN article_topic_tags att ON att.article_id = a.article_id::bigint
		JOIN topic_tags tg ON tg.id = att.topic_tag_id
		WHERE t.section_id IN ?
		ORDER BY t.section_id ASC, att.topic_tag_id ASC`, sectionIDs).Scan(&resolved).Error; err != nil {
		return nil, fmt.Errorf("resolve section tag ids via article_topic_tags: %w", err)
	}
	bySection := make(map[uint][]uint, len(sectionIDs))
	for _, row := range resolved {
		ids := bySection[row.SectionID]
		if len(ids) == 0 || ids[len(ids)-1] != row.TagID {
			bySection[row.SectionID] = append(ids, row.TagID)
		}
	}

	// Path 2 (fallback): intersect cluster_tag_ids with live tags. Only used
	// for sections that path 1 resolved to nothing (no thread article refs).
	type clusterRow struct {
		ID            uint
		ClusterTagIDs JSON
	}
	var pending []uint
	for _, id := range sectionIDs {
		if len(bySection[id]) == 0 {
			pending = append(pending, id)
		}
	}
	if len(pending) > 0 {
		var rows []clusterRow
		if err := r.db.Select("id, cluster_tag_ids").Where("id IN ?", pending).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("load section cluster tag ids: %w", err)
		}
		candidateIDs := make(map[uint][]uint, len(rows))
		all := make([]uint, 0, len(rows)*4)
		for _, row := range rows {
			ids := jsonUintIDs(row.ClusterTagIDs)
			candidateIDs[row.ID] = ids
			all = append(all, ids...)
		}
		live, err := r.existingTagIDs(all)
		if err != nil {
			return nil, err
		}
		for sectionID, ids := range candidateIDs {
			kept := make([]uint, 0, len(ids))
			for _, id := range ids {
				if live[id] {
					kept = append(kept, id)
				}
			}
			sort.Slice(kept, func(i, j int) bool { return kept[i] < kept[j] })
			bySection[sectionID] = kept
		}
	}

	for _, id := range sectionIDs {
		if ids, ok := bySection[id]; ok {
			out[id] = ids
		} else {
			out[id] = nil
		}
	}
	return out, nil
}

// existingTagIDs returns the subset of ids that still exist in topic_tags.
func (r *TopicGraphRepository) existingTagIDs(ids []uint) (map[uint]bool, error) {
	live := make(map[uint]bool, len(ids))
	if len(ids) == 0 {
		return live, nil
	}
	var found []uint
	if err := r.db.Table("topic_tags").Where("id IN ?", ids).
		Pluck("id", &found).Error; err != nil {
		return nil, fmt.Errorf("check tag existence: %w", err)
	}
	for _, id := range found {
		live[id] = true
	}
	return live, nil
}
