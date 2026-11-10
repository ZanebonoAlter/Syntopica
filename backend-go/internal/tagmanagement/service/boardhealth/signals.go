package boardhealth

import (
	"context"
	"regexp"

	"gorm.io/gorm"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"
)

// Three-disease triage signals (board-health-triage-advice design D1/D2,
// seam A task 1.2). Pure read, same round as the affinity statistics — the
// signals reuse the matchedRows + identity embeddings already loaded by
// collectBoardStatistics, plus one pgvector top-K query per composition
// label and one flow aggregation per board. No matching recomputation, no
// writes (红线 20/22: 口径唯一实现于 boardhealth，体检零写).
//
// 磁铁信号口径（fix-board-health-magnet-signal D1/D2，2026-11-09 反转）：
// 判定用「失败子集均值显著高于健康子集均值」（语义：磁铁把跑题的挂载吸了
// 过来），不再是「失败子集均值显著低于全体均值」——旧口径与语义相反，会把
// 核心主体系统性点成名。全体均值（all 侧）保留但退出判定。

// neighborTopK is the pgvector top-K of the neighbor signal (spec: top-3).
const neighborTopK = 3

// descEventPattern is the fixed article-style word-root screen for the
// description-pollution prefilter (病C 显性事件词根). Closed set from the
// proposal (此次/本次/该事件/报道/针对/期间/近日) — deliberately a fixed
// literal alternation, never assembled from user input (test-cases 变体:
// 正则元字符注入不适用).
var descEventPattern = regexp.MustCompile(`此次|本次|该事件|报道|针对|期间|近日`)

// screenDescription returns the deduplicated hit word roots of the fixed
// event pattern, or nil when the description is empty or hits nothing
// (spec: description 为空不标记). Pure function.
func screenDescription(desc string) []string {
	if desc == "" {
		return nil
	}
	matches := descEventPattern.FindAllString(desc, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	flags := make([]string, 0, len(matches))
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			flags = append(flags, match)
		}
	}
	return flags
}

// computeMagnetSignals fills every composition label's Magnet field from the
// same-round in-memory data, grouping the board's mounted rows by
// direction_mismatch (fix-board-health-magnet-signal design D1):
//   - healthy side (good) = direction_mismatch=false mounts; failed side =
//     direction_mismatch=true mounts; all side = both (口径 identical to the
//     affinity distribution — 不筛样本);
//   - suspicious = failed_mean - good_mean >= MagnetGap AND good samples
//     >= MinSampleSize AND failed samples >= MagnetMinFailed;
//   - 语义：「磁铁」= 把跑题的挂载吸过来 —— 失败侧的贴合均值必须**高于**
//     健康侧。旧口径（all_mean - failed_mean）与语义相反，2026-11-09 真库
//     实证：失败挂载按定义就与本板方向不符、与核心主体天然不相似，旧口径
//     于是「越是核心主体 gap 越大、越像污染源」，全板 41/47 成立、信号
//     失去区分度；
//   - either side short on samples, or the label's own embedding missing →
//     Insufficient, never a suspicion verdict (样本不足/向量缺失不硬凑);
//   - the all side is still computed（展示 + 历史快照兼容）but MUST NOT take
//     part in the verdict (all_mean ≡ cosine_mean, 见 models 注释);
//   - 每个信号都带 caliber 口径标记（"healthy_subset" = 本轮新口径；空串 =
//     旧口径/历史快照），让消费方能区分「旧快照」与「新快照但健康侧不可算」
//     ——good_mean 为 nil 无法承担这个区分（design D12 / review R-4）;
//   - no per-label attribution of which tag was "attracted" by which label
//     (红线: 不做单标签吸引归因 — the failed subset is the same board-level
//     rows for every label, only the cosine values differ).
func computeMagnetSignals(labels []tagmodels.BoardHealthLabelMetrics, auxEmb map[uint][]float64, rows []matchedRow, tagEmb map[uint][]float64, config Config) {
	allVectors := make([][]float64, 0, len(rows))
	goodVectors := make([][]float64, 0, len(rows))
	failedVectors := make([][]float64, 0)
	for _, row := range rows {
		vector, ok := tagEmb[row.TopicTagID]
		if !ok {
			continue // tag-side vector missing: invisible to both sides (never zero-filled)
		}
		allVectors = append(allVectors, vector)
		if row.DirectionMismatch {
			failedVectors = append(failedVectors, vector)
		} else {
			goodVectors = append(goodVectors, vector)
		}
	}
	for i := range labels {
		entry := &labels[i]
		signal := &tagmodels.BoardHealthMagnetSignal{
			AllSamples:    len(allVectors),
			GoodSamples:   len(goodVectors),
			FailedSamples: len(failedVectors),
			// 口径标记无条件写：caliber 标记的是**磁铁口径**（本轮用哪个公式
			// 判定），与两侧样本是否充足无关——健康侧样本不足（insufficient）
			// 时照样写新口径，消费方才能区分「旧快照」与「新快照但健康侧不可
			// 算」（design D12 / review R-4）。
			Caliber: tagmodels.BoardHealthMagnetCaliberHealthySubset,
		}
		entry.Magnet = signal
		labelVector, ok := auxEmb[entry.AuxID]
		if !ok {
			signal.Insufficient = true // 缺自身向量：两侧均值不可算，信号不足
			continue
		}
		if len(allVectors) > 0 {
			mean := meanCosine(labelVector, allVectors)
			signal.AllMean = &mean
		}
		if len(goodVectors) > 0 {
			mean := meanCosine(labelVector, goodVectors)
			signal.GoodMean = &mean
		}
		if len(failedVectors) > 0 {
			mean := meanCosine(labelVector, failedVectors)
			signal.FailedMean = &mean
		}
		// 两侧各自达标才判：健康侧不足说明「相对谁更高」无从谈起。
		signal.Insufficient = signal.GoodSamples < config.MinSampleSize ||
			signal.FailedSamples < config.MagnetMinFailed
		signal.Suspicious = !signal.Insufficient &&
			signal.GoodMean != nil && signal.FailedMean != nil &&
			*signal.FailedMean-*signal.GoodMean >= config.MagnetGap
	}
}

// meanCosine averages the cosine of one vector against every other.
func meanCosine(vector []float64, others [][]float64) float64 {
	sum := 0.0
	for _, other := range others {
		sum += cosineSim(vector, other)
	}
	return sum / float64(len(others))
}

// loadNeighborsForLabels fills each composition label's Neighbors with its
// top-K nearest ACTIVE auxiliary labels (pgvector cosine, excluding self).
// A label without its own embedding keeps nil neighbors — the signal is
// explicitly insufficient, never fabricated (spec: 向量缺失信号不硬算);
// a computed-but-empty list means the active-aux space holds no other label.
func loadNeighborsForLabels(ctx context.Context, db *gorm.DB, labels []tagmodels.BoardHealthLabelMetrics, auxEmb map[uint][]float64) error {
	for i := range labels {
		entry := &labels[i]
		vector, ok := auxEmb[entry.AuxID]
		if !ok {
			continue
		}
		neighbors, err := queryAuxNeighbors(ctx, db, entry.AuxID, vector, neighborTopK)
		if err != nil {
			return err
		}
		entry.Neighbors = neighbors
	}
	return nil
}

// queryAuxNeighbors runs the pgvector top-K nearest query over the active
// auxiliary-label space (design D1: per-label exact scan acceptable for the
// nightly round; explicit column aliases guard against the 2026-10-07
// pgvector scan-returns-nothing regression).
func queryAuxNeighbors(ctx context.Context, db *gorm.DB, auxID uint, vector []float64, k int) ([]tagmodels.BoardHealthNeighbor, error) {
	queryVec := core.FloatsToPgVector(vector)
	var rows []struct {
		AuxID  uint    `gorm:"column:aux_id"`
		Label  string  `gorm:"column:label"`
		Cosine float64 `gorm:"column:cosine"`
	}
	err := db.WithContext(ctx).Raw(`
		SELECT l.id AS aux_id,
		       l.label AS label,
		       1 - (l.embedding <=> ?::vector) AS cosine
		FROM semantic_labels l
		WHERE l.label_type = 'auxiliary'
		  AND l.status = 'active'
		  AND l.embedding IS NOT NULL
		  AND l.id <> ?
		ORDER BY l.embedding <=> ?::vector
		LIMIT ?`, queryVec, auxID, queryVec, k).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	neighbors := make([]tagmodels.BoardHealthNeighbor, 0, len(rows))
	for _, row := range rows {
		neighbors = append(neighbors, tagmodels.BoardHealthNeighbor{
			AuxID:  row.AuxID,
			Label:  row.Label,
			Cosine: row.Cosine,
		})
	}
	return neighbors, nil
}

// failedTagIDs extracts the tag ids of the board's direction_mismatch rows.
func failedTagIDs(rows []matchedRow) []uint {
	ids := make([]uint, 0)
	for _, row := range rows {
		if row.DirectionMismatch {
			ids = append(ids, row.TopicTagID)
		}
	}
	return ids
}

// loadFailedFlow aggregates the board-level magnet flow evidence (design
// D2): among the board's direction_mismatch mounts, where else are those
// tags VALIDLY mounted (direction_mismatch=false on another ACTIVE board)?
// The top-1 destination is the remove-evidence "主要流向".
//   - a failed tag whose other mounts are themselves direction_mismatch is
//     NOT reliable flow (test-cases 白盒: 失败对手挂载不作可靠流向);
//   - failed mounts with no valid destination → FlowTags=0 and TopBoard
//     nil (显式未知, not a fabricated target);
//   - no failed mounts at all → nil (JSON null).
func loadFailedFlow(ctx context.Context, db *gorm.DB, boardID uint, failedIDs []uint) (*tagmodels.BoardHealthMagnetFlow, error) {
	if len(failedIDs) == 0 {
		return nil, nil
	}
	flow := &tagmodels.BoardHealthMagnetFlow{FailedTags: len(failedIDs)}
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(DISTINCT ttbl.topic_tag_id)
		FROM topic_tag_board_labels ttbl
		JOIN semantic_labels l
		     ON l.id = ttbl.semantic_board_id
		    AND l.label_type = 'board'
		    AND l.status = 'active'
		WHERE ttbl.topic_tag_id IN ?
		  AND ttbl.semantic_board_id <> ?
		  AND COALESCE(ttbl.direction_mismatch, false) = false`,
		failedIDs, boardID).Scan(&flow.FlowTags).Error; err != nil {
		return nil, err
	}
	if flow.FlowTags == 0 {
		return flow, nil // explicit unknown: no failed tag has a reliable destination
	}
	var top struct {
		BoardLabel string `gorm:"column:board_label"`
		FlowTags   int    `gorm:"column:flow_tags"`
	}
	err := db.WithContext(ctx).Raw(`
		SELECT l.label AS board_label,
		       COUNT(DISTINCT ttbl.topic_tag_id) AS flow_tags
		FROM topic_tag_board_labels ttbl
		JOIN semantic_labels l
		     ON l.id = ttbl.semantic_board_id
		    AND l.label_type = 'board'
		    AND l.status = 'active'
		WHERE ttbl.topic_tag_id IN ?
		  AND ttbl.semantic_board_id <> ?
		  AND COALESCE(ttbl.direction_mismatch, false) = false
		GROUP BY ttbl.semantic_board_id, l.label
		ORDER BY flow_tags DESC, ttbl.semantic_board_id ASC
		LIMIT 1`, failedIDs, boardID).Scan(&top).Error
	if err != nil {
		return nil, err
	}
	if top.BoardLabel != "" {
		flow.TopBoard = &top.BoardLabel
		flow.TopTags = top.FlowTags
	}
	return flow, nil
}
