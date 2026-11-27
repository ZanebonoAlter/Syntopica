package repository

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

// ── 泳道优先召回（lane-first-recall）──────────────────────────────────────────
//
// 日报候选收集原本只有「标签 → 版块」一条通道：已挂载标签（主查询）+ 未挂载
// 标签的 `Limit(50)` 随机现场匹配。两条都拿**版块**当召回单元，而版块级判据对
// 单篇新事件结构性失效——`weighted` 区间的 board score 恒为 0.700（2026-11-26：
// `英伟达暂停东南亚芯片交付` 与 `索尼公布PS+会免名单` 逐位同分），质量分级因此
// 只能靠 `kept >= 10` 的整批开关抖动。
//
// 本文件提供第二条通道：以**泳道**（board_persistent_topics）为召回单元，在当日
// 全量 event 标签池里按语义距离 ANN 召回。泳道级距离是可分的（15220 两条泳道
// Top-10 全为真事件 0.076–0.161，噪声从 0.174 才出现），所以这条通道能救回
// 「属于某条已有泳道、当天只有 1 篇文章」的新事件。
//
// 红线：
//   - 复用 `topic_tag_embeddings` 上现成的 semantic halfvec HNSW 索引；查询表达式
//     必须与索引一致（`(embedding)::halfvec(<dim>) <=> $1::halfvec(<dim>)`，typmod
//     必须带维度——bare `::halfvec` 不会走索引）。ANN 只负责扩容候选，最终裁决与
//     截断一律用原列 float32 精确距离（同 semantic-board 红线 38 的范式）。
//   - 只读：不写 `topic_tag_board_labels`（挂载是永久事实，召回是单日行为），
//     不写话题/关系表。
//   - 「当天没有专属新闻的泳道」空着是正确结果，不得放宽距离凑数（design 边界）。

const (
	// laneRecallTopKKey / laneRecallMaxDistanceKey are the ai_settings keys
	// seeded by migration 20261126_0001 (lane-first-recall task 1.1).
	laneRecallTopKKey        = "lane_recall_top_k"
	laneRecallMaxDistanceKey = "lane_recall_max_distance"

	// laneRecallHalfvecMaxDim is pgvector's halfvec ceiling. Above it the CAST
	// (and therefore the existing semantic HNSW index) is unusable — the lane
	// is skipped rather than falling back to a full-table float32 scan.
	laneRecallHalfvecMaxDim = 4000
	// laneRecallTopKCeiling bounds an operator-supplied top_k so a typo
	// (e.g. 10000) cannot turn one lane into a report-wide tag dump.
	laneRecallTopKCeiling = 64
	// laneRecallPoolFloor/Factor size the ANN candidate pool. The pool must be
	// wider than top_k: the precise re-rank (stage 2) may reorder neighbours and
	// drop entries past max_distance, so a pool == top_k would truncate before
	// the decision is made.
	laneRecallPoolFloor  = 16
	laneRecallPoolFactor = 4
	// laneRecallEfSearch only matters when the planner actually chooses the HNSW
	// index; with today's per-day pool sizes it does not (see the caveat on
	// laneRecallAnnSQLTemplate). Kept so the ANN stage stays correct if it does.
	laneRecallEfSearch = 200
)

// LaneRecallConfig carries the two runtime-tunable lane-recall parameters
// (lane-first-recall design D2). Same online-override 口径 as the lane
// thresholds: read from ai_settings on every report run, fall back to the
// calibrated defaults on missing/invalid values, never fail the pipeline.
type LaneRecallConfig struct {
	TopK        int     // lane_recall_top_k: max tags one lane may recall per day
	MaxDistance float64 // lane_recall_max_distance: tag→lane-anchor cosine distance ceiling
}

// DefaultLaneRecallConfig returns the calibrated defaults (design D2: K=8 is
// the recall knee, τ=0.16 gives ~90% precision at ~5 tags/board/day).
func DefaultLaneRecallConfig() LaneRecallConfig {
	return LaneRecallConfig{TopK: 8, MaxDistance: 0.16}
}

// parseLaneRecallConfig maps raw ai_settings values onto a validated config.
// Pure: missing keys, non-numeric strings and out-of-range values all fall back
// to the default for that field independently, with a warn log — a malformed
// row must never abort a daily-report run.
func parseLaneRecallConfig(values map[string]string) LaneRecallConfig {
	cfg := DefaultLaneRecallConfig()
	if raw, ok := values[laneRecallTopKKey]; ok {
		v, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && v > 0 && v <= laneRecallTopKCeiling {
			cfg.TopK = v
		} else {
			logging.Warnf("lane-recall: invalid %s %q; using default %d", laneRecallTopKKey, raw, cfg.TopK)
		}
	}
	if raw, ok := values[laneRecallMaxDistanceKey]; ok {
		v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err == nil && v > 0 && v <= 1 {
			cfg.MaxDistance = v
		} else {
			logging.Warnf("lane-recall: invalid %s %q; using default %v", laneRecallMaxDistanceKey, raw, cfg.MaxDistance)
		}
	}
	return cfg
}

// LoadLaneRecallConfig reads the lane-recall keys from ai_settings. Reading is
// best-effort: a missing row keeps the default, an unreadable table keeps the
// default too (the report pipeline treats this channel as optional).
func LoadLaneRecallConfig(db *gorm.DB) LaneRecallConfig {
	if db == nil {
		return DefaultLaneRecallConfig()
	}
	var rows []models.AISettings
	if err := db.Where("key IN ?", []string{laneRecallTopKKey, laneRecallMaxDistanceKey}).Find(&rows).Error; err != nil {
		logging.Warnf("lane-recall: read ai_settings failed (using defaults): %v", err)
		return DefaultLaneRecallConfig()
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}
	return parseLaneRecallConfig(values)
}

// laneRecallAnnSQLTemplate is stage 1 of the recall: the candidate id pool,
// ordered by the expression the semantic HNSW index is built on. The typmod MUST
// carry the dimension (`::halfvec(<dim>)`) so the expression matches
// `idx_topic_tag_embeddings_semantic_halfvec_hnsw` exactly.
//
// Measured caveat (review R-11): on production a per-day pool is only a few
// thousand rows, so EXPLAIN picks a Sort over the filtered set in all three
// plan modes (custom / generic after 6 calls / generic) and the HNSW index is
// never used — SET LOCAL hnsw.ef_search is therefore inert in practice. The
// expression is kept because it becomes index-eligible once the pool grows;
// do NOT claim "index is hit" in docs or comments.
const laneRecallAnnSQLTemplate = `
	SELECT e.topic_tag_id AS tag_id
	FROM topic_tag_embeddings e
	JOIN topic_tags t ON t.id = e.topic_tag_id
	WHERE t.status = 'active'
	  AND t.category = ?
	  AND e.embedding_type = 'semantic'
	  AND e.embedding IS NOT NULL
	  AND EXISTS (
		SELECT 1
		FROM article_topic_tags att
		JOIN articles a ON a.id = att.article_id
		WHERE att.topic_tag_id = t.id
		  AND a.pub_date >= ? AND a.pub_date < ?
	  )
	  AND NOT EXISTS (
		SELECT 1 FROM topic_tag_board_labels bl
		WHERE bl.topic_tag_id = t.id
		  AND bl.semantic_board_id = ?
		  AND COALESCE(bl.direction_mismatch, false)
	  )
	ORDER BY e.embedding::halfvec(%d) <=> ?::halfvec(%d)
	LIMIT ?
`

// laneRecallExactSQL is stage 2: the same pool re-scored on the original float32
// column. The ANN stage only widens the candidate set; admission and ordering
// are decided here.
const laneRecallExactSQL = `
	SELECT e.topic_tag_id AS tag_id, e.embedding <=> ?::vector AS distance
	FROM topic_tag_embeddings e
	JOIN topic_tags t ON t.id = e.topic_tag_id
	WHERE t.status = 'active'
	  AND t.category = ?
	  AND e.embedding_type = 'semantic'
	  AND e.embedding IS NOT NULL
	  AND e.topic_tag_id IN ?
	  AND EXISTS (
		SELECT 1
		FROM article_topic_tags att
		JOIN articles a ON a.id = att.article_id
		WHERE att.topic_tag_id = t.id
		  AND a.pub_date >= ? AND a.pub_date < ?
	  )
	  AND NOT EXISTS (
		SELECT 1 FROM topic_tag_board_labels bl
		WHERE bl.topic_tag_id = t.id
		  AND bl.semantic_board_id = ?
		  AND COALESCE(bl.direction_mismatch, false)
	  )
	ORDER BY e.embedding <=> ?::vector
`

// LaneRecalledTag is one tag recalled through the lane channel, carrying the
// lane that witnessed it and the **precise** (original float32 column) cosine
// distance that decided admission.
type LaneRecalledTag struct {
	TagID    uint
	LaneID   uint
	Distance float64
}

// recallDecision applies the day's admission rule to a candidate list: keep the
// nearest at most topK entries whose distance is strictly below maxDistance.
// Pure so the two truncations (distance ceiling, top-K) are unit-testable without
// Postgres; it sorts internally so a caller cannot break the early exit by
// handing over an unsorted slice.
//
// "Empty is a correct answer": a lane whose nearest neighbour is at or beyond
// maxDistance recalls nothing — 71 of 122 measured lane-days (58%) end up here
// and that is the intended behaviour (design 边界: 不得放宽距离凑数).
func recallDecision(cands []LaneRecalledTag, topK int, maxDistance float64) []LaneRecalledTag {
	if topK <= 0 || maxDistance <= 0 || len(cands) == 0 {
		return nil
	}
	sorted := make([]LaneRecalledTag, len(cands))
	copy(sorted, cands)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Distance != sorted[j].Distance {
			return sorted[i].Distance < sorted[j].Distance
		}
		return sorted[i].TagID < sorted[j].TagID
	})
	out := make([]LaneRecalledTag, 0, topK)
	for _, c := range sorted {
		if c.Distance >= maxDistance {
			break
		}
		if len(out) >= topK {
			break
		}
		out = append(out, c)
	}
	return out
}

// mergeNearestLaneRecall folds one lane's admitted hits into the board-level
// map, keeping the nearest witnessing lane per tag (a tag may sit inside
// several lanes' radius). The final lane assignment is re-decided downstream by
// the L1/L2 bucketing, so the recall layer only needs one witness.
func mergeNearestLaneRecall(dst map[uint]LaneRecalledTag, hits []LaneRecalledTag) {
	for _, hit := range hits {
		if prev, ok := dst[hit.TagID]; !ok || hit.Distance < prev.Distance {
			dst[hit.TagID] = hit
		}
	}
}

// laneRecallAnchor picks the vector a lane is recalled against: the centroid
// (mean of recent sections) when present, degrading to the 首义 embedding —
// exactly the anchor the L1/L2 bucketing uses (topicAnchorVec), so "recalled by
// this lane" and "bucketed into this lane" speak about the same point.
// nil = the lane has no usable vector and must be skipped.
func laneRecallAnchor(centroid, embedding string) []float64 {
	if v, err := repoParsePgVector(centroid); err == nil && len(v) > 0 {
		return v
	}
	if v, err := repoParsePgVector(embedding); err == nil && len(v) > 0 {
		return v
	}
	return nil
}

// laneRecallAnchorUsable rejects anchors the halfvec index cannot serve. A
// dimension mismatch against the tag vector space surfaces here as an
// unusable/over-long vector; a non-finite component (a corrupted pgvector text
// literal) would poison every distance comparison, so it is rejected too.
func laneRecallAnchorUsable(anchor []float64) bool {
	if len(anchor) == 0 || len(anchor) > laneRecallHalfvecMaxDim {
		return false
	}
	for _, v := range anchor {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// RecallTagsByLane recalls the day's top tags for one lane: up to topK tags
// whose precise distance to the lane anchor is below maxDistance. Empty result
// is a valid answer (see recallDecision).
//
// Two-stage query (ANN recall + exact adjudication), mirroring
// core.findSimilarByVector:
//  1. candidate pool ordered by the indexed halfvec expression — hits
//     idx_topic_tag_embeddings_semantic_halfvec_hnsw;
//  2. exact float32 `embedding <=> $1::vector` distances over that pool, so the
//     admitted set is decided (and cut) by full-precision distances.
func (r *TopicGraphRepository) RecallTagsByLane(laneID uint, day time.Time, topK int, maxDistance float64) ([]LaneRecalledTag, error) {
	if laneID == 0 {
		return nil, nil
	}
	var lane BoardPersistentTopic
	if err := r.db.Select("id", "status", "embedding", "centroid", "semantic_board_id").
		Where("id = ?", laneID).Take(&lane).Error; err != nil {
		return nil, fmt.Errorf("load lane %d: %w", laneID, err)
	}
	if lane.Status != TopicStatusActive {
		return nil, nil
	}
	anchor := laneRecallAnchor(lane.Centroid, lane.Embedding)
	if !laneRecallAnchorUsable(anchor) {
		logging.Warnf("lane-recall: lane %d has no usable vector (dim=%d), skipped", laneID, len(anchor))
		return nil, nil
	}
	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	hits, err := r.recallLaneCandidates(laneID, lane.SemanticBoardID, anchor, startOfDay, startOfDay.Add(24*time.Hour), topK)
	if err != nil {
		return nil, err
	}
	return recallDecision(hits, topK, maxDistance), nil
}

// RecallTagsForBoard runs the lane channel for every active lane of a board and
// merges the results ("same tag witnessed by several lanes → nearest lane
// wins"). Per-lane failures (missing vector, ANN error) are logged and skipped:
// the lane channel is an enrichment and never blocks the report.
func (r *TopicGraphRepository) RecallTagsForBoard(boardID uint, day time.Time, cfg LaneRecallConfig) (map[uint]LaneRecalledTag, error) {
	if boardID == 0 {
		return nil, nil
	}
	if cfg.TopK <= 0 || cfg.MaxDistance <= 0 {
		cfg = DefaultLaneRecallConfig()
	}
	var lanes []BoardPersistentTopic
	if err := r.db.Select("id", "status", "embedding", "centroid").
		Where("semantic_board_id = ? AND status = ?", boardID, TopicStatusActive).
		Order("id ASC").Find(&lanes).Error; err != nil {
		return nil, fmt.Errorf("list active lanes for board %d: %w", boardID, err)
	}
	if len(lanes) == 0 {
		return nil, nil
	}

	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	recalled := make(map[uint]LaneRecalledTag)
	skipped, failed, empty := 0, 0, 0
	for _, lane := range lanes {
		anchor := laneRecallAnchor(lane.Centroid, lane.Embedding)
		if !laneRecallAnchorUsable(anchor) {
			skipped++
			logging.Warnf("lane-recall: board %d lane %d has no usable vector (dim=%d), skipped", boardID, lane.ID, len(anchor))
			continue
		}
		pool := cfg.TopK * laneRecallPoolFactor
		if pool < laneRecallPoolFloor {
			pool = laneRecallPoolFloor
		}
		cands, err := r.recallLaneCandidates(lane.ID, boardID, anchor, startOfDay, endOfDay, pool)
		if err != nil {
			failed++
			logging.Warnf("lane-recall: board %d lane %d recall failed, skipped: %v", boardID, lane.ID, err)
			continue
		}
		admitted := recallDecision(cands, cfg.TopK, cfg.MaxDistance)
		if len(admitted) == 0 {
			empty++
		}
		mergeNearestLaneRecall(recalled, admitted)
	}
	logging.Infof("lane-recall: board %d lanes=%d skipped_no_vector=%d failed=%d empty=%d recalled_tags=%d top_k=%d max_distance=%v",
		boardID, len(lanes), skipped, failed, empty, len(recalled), cfg.TopK, cfg.MaxDistance)
	return recalled, nil
}

// recallLaneCandidates is the two-stage ANN + exact query behind
// RecallTagsByLane: stage 1 returns an id pool ordered by the indexed halfvec
// distance, stage 2 re-scores that pool with the original float32 column and
// returns distance-sorted hits (no truncation — recallDecision decides).
func (r *TopicGraphRepository) recallLaneCandidates(laneID, boardID uint, anchor []float64, startOfDay, endOfDay time.Time, pool int) ([]LaneRecalledTag, error) {
	if pool <= 0 {
		pool = laneRecallPoolFloor
	}
	dim := len(anchor)
	pgVec := FloatsToPgVector(anchor)

	var hits []LaneRecalledTag
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// SET LOCAL scopes ef_search to this transaction: pooled connections are
		// unaffected once the tx ends.
		if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", laneRecallEfSearch)).Error; err != nil {
			return fmt.Errorf("set hnsw.ef_search: %w", err)
		}
		// Stage 1 — ANN recall over the day's full event tag pool (NOT limited to
		// tags mounted on this board: 86% of a day's new event tags carry no mount
		// at all, and the board-channel fallback only ever saw a random 50 of them).
		candSQL := fmt.Sprintf(laneRecallAnnSQLTemplate, dim, dim)
		var candIDs []uint
		if err := tx.Raw(candSQL, models.TagCategoryEvent, startOfDay, endOfDay, boardID, pgVec, pool).Scan(&candIDs).Error; err != nil {
			return fmt.Errorf("query lane recall candidates: %w", err)
		}
		if len(candIDs) == 0 {
			return nil
		}
		// Stage 2 — exact float32 adjudication over the pool. Same filters are
		// re-applied so the decision never sees a row the pool filters excluded.
		exactSQL := laneRecallExactSQL
		var rows []struct {
			TagID    uint
			Distance float64
		}
		if err := tx.Raw(exactSQL, pgVec, models.TagCategoryEvent, candIDs, startOfDay, endOfDay, boardID, pgVec).Scan(&rows).Error; err != nil {
			return fmt.Errorf("rerank lane recall candidates: %w", err)
		}
		hits = make([]LaneRecalledTag, 0, len(rows))
		for _, row := range rows {
			hits = append(hits, LaneRecalledTag{TagID: row.TagID, LaneID: laneID, Distance: row.Distance})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// SortLaneRecallHits orders lane hits deterministically (distance asc, tag id
// asc). The orchestrator appends lane-recalled tags in this order so a report
// built from the same inputs keeps the same tag order.
func SortLaneRecallHits(hits []LaneRecalledTag) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Distance != hits[j].Distance {
			return hits[i].Distance < hits[j].Distance
		}
		return hits[i].TagID < hits[j].TagID
	})
}
