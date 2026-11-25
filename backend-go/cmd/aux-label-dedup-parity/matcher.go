package main

// L2 召回 matcher 的**对账侧复刻**（review R-6）：服务侧实现在
// internal/tagmanagement/service/auxlabel/auxiliary_label_service.go 的
// sqlMergeMatcher 链路上（mergeRecallSQL / queryMergeCandidates / pickMergeTarget）。
//
// 为什么复刻而不是调用：服务侧不为了一个一次性运维工具新开生产导出
// （那会把对账专用代码永久挂在服务包里）。代价是**两处实现必须同步**——
// 改服务侧召回口径时 MUST 同步本文件，否则对账结论失真。
//
// 口径（与服务侧一致，逐条）：
//   - 召回 WHERE 与部分索引谓词逐字一致（database.SemanticLabelMergeHalfvecIndexPredicate）；
//   - halfvec(<dim>) typmod 带运行时维度（裸 ::halfvec 不走索引）；
//   - 精排用原列 merge_embedding <=> ?::vector（float32 精确距离），
//     halfvec 近似距离不参与阈值判断；
//   - 裁决：cosine ≥ 阈值、RefCount 降序、同 RefCount 取 ID 小者；
//   - 候选池被填满且全部在阈值内 → 用更宽的池 + 更高的 ef_search 重查一次。

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// mergeRecallErrorHint 给召回失败补上维度类错误的处置指引：运维工具最容易在
// “换 embedding 模型”那个窗口被跑起来，而那正是维度不符唯一会发生的场景。
// 服务侧有同款包装（auxlabel 包的 wrapMergeRecallError）；工具侧不共享内部符号，
// 这里按公开契约（database.SemanticLabelMergeDimensionCleanupSQL）复刻指引文案。
func mergeRecallErrorHint(err error, dim int) error {
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "dimension") {
		return err
	}
	return fmt.Errorf("%w; if this is a vector dimension mismatch, semantic_labels still holds merge vectors of another dimension — clear them: %s then rebuild the vectors via POST /api/auxiliary-labels/backfill-embeddings",
		err, fmt.Sprintf(database.SemanticLabelMergeDimensionCleanupSQL, dim))
}

// mergeCandidateProjection is the metadata column set handed to the Go-side
// adjudication — the vectors themselves are never selected.
const mergeCandidateProjection = "id, label, slug, label_type, aliases, ref_count, description, status, protected, source, display_order, created_at, updated_at"

// mergeCandidate is one recalled row plus its exact float32 cosine distance.
type mergeCandidate struct {
	Row      models.SemanticLabel
	Distance float64
}

// matchMergeCandidate resolves the L2 merge target exactly as the live matcher
// does (same SQL, same transaction scoping, same adjudication).
func matchMergeCandidate(ctx context.Context, db *gorm.DB, mergePgVector string, mergeVector []float64) (*models.SemanticLabel, error) {
	dim := len(mergeVector)
	if dim == 0 || strings.TrimSpace(mergePgVector) == "" {
		return nil, fmt.Errorf("auxiliary label merge matcher: empty merge vector")
	}
	threshold := loadDedupeThreshold(db)

	var best *models.SemanticLabel
	recall := func(tx *gorm.DB) error {
		if err := setMergeEfSearch(tx, dim, mergeEfSearch); err != nil {
			return err
		}
		candidates, err := queryMergeCandidates(tx, mergePgVector, dim, mergeCandidateLimit)
		if err != nil {
			return fmt.Errorf("recall merge candidates: %w", mergeRecallErrorHint(err, dim))
		}
		best = pickMergeTarget(candidates, threshold)
		if !mergeRecallSaturated(candidates, mergeCandidateLimit, threshold) {
			return nil
		}
		// Every candidate in the pool is a near-duplicate: the ANN pool was
		// truncated, so a higher-RefCount target may sit beyond it. Re-query
		// once with a wider pool (and matching ef_search) before deciding.
		if err := setMergeEfSearch(tx, dim, mergeEfSearchWide); err != nil {
			return err
		}
		wide, err := queryMergeCandidates(tx, mergePgVector, dim, mergeCandidateLimitWide)
		if err != nil {
			return fmt.Errorf("recall merge candidates (wide): %w", mergeRecallErrorHint(err, dim))
		}
		best = pickMergeTarget(wide, threshold)
		return nil
	}

	if mergeHandleInTransaction(db) {
		if err := recall(db); err != nil {
			return nil, err
		}
		return best, nil
	}
	if err := db.WithContext(ctx).Transaction(recall); err != nil {
		return nil, err
	}
	return best, nil
}

// setMergeEfSearch sets hnsw.ef_search for the recall transaction. dims above
// halfvec's 4000 limit take the exact-scan path — no HNSW index is consulted
// there, so the GUC is pointless (and touching it outside a transaction would
// just warn).
func setMergeEfSearch(tx *gorm.DB, dim, efSearch int) error {
	if dim <= 0 || dim > 4000 {
		return nil
	}
	if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)).Error; err != nil {
		return fmt.Errorf("set hnsw.ef_search: %w", err)
	}
	return nil
}

// mergeHandleInTransaction reports whether db already runs inside a transaction.
func mergeHandleInTransaction(db *gorm.DB) bool {
	if db == nil || db.Statement == nil {
		return false
	}
	_, ok := db.Statement.ConnPool.(*sql.Tx)
	return ok
}

// mergeRecallSQL builds the stage-1 ANN recall statement for one dimension: the
// WHERE clause is the partial index predicate **verbatim** (otherwise the planner
// will not use the partial index) and the cast carries the dimension typmod.
func mergeRecallSQL(dim int) string {
	if dim <= 0 || dim > 4000 {
		return fmt.Sprintf(`
		SELECT id
		FROM semantic_labels
		WHERE %s
		ORDER BY merge_embedding <=> ?::vector
		LIMIT ?
	`, database.SemanticLabelMergeHalfvecIndexPredicate)
	}
	return fmt.Sprintf(`
		SELECT id
		FROM semantic_labels
		WHERE %s
		ORDER BY merge_embedding::halfvec(%d) <=> ?::halfvec(%d)
		LIMIT ?
	`, database.SemanticLabelMergeHalfvecIndexPredicate, dim, dim)
}

// queryMergeCandidates runs the two-stage recall inside the caller's
// transaction: (1) ANN candidates by halfvec distance, (2) exact float32 rerank
// on the original vector column so the threshold口径 keeps full precision.
func queryMergeCandidates(tx *gorm.DB, mergePgVector string, dim, limit int) ([]mergeCandidate, error) {
	var ids []uint
	if err := tx.Raw(mergeRecallSQL(dim), mergePgVector, limit).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Exact float32 rerank on the original column (≤limit rows, no index).
	type exactRow struct {
		ID       uint
		Distance float64
	}
	var exact []exactRow
	if err := tx.Raw(`
		SELECT id, merge_embedding <=> ?::vector AS distance
		FROM semantic_labels
		WHERE id IN ?
		ORDER BY merge_embedding <=> ?::vector
	`, mergePgVector, ids, mergePgVector).Scan(&exact).Error; err != nil {
		return nil, err
	}
	if len(exact) == 0 {
		return nil, nil
	}

	// Load the metadata needed for adjudication (vectors excluded).
	distanceByID := make(map[uint]float64, len(exact))
	order := make([]uint, len(exact))
	for i, r := range exact {
		distanceByID[r.ID] = r.Distance
		order[i] = r.ID
	}
	var rows []models.SemanticLabel
	if err := tx.Select(mergeCandidateProjection).Where("id IN ?", order).Find(&rows).Error; err != nil {
		return nil, err
	}
	rowByID := make(map[uint]models.SemanticLabel, len(rows))
	for _, row := range rows {
		rowByID[row.ID] = row
	}
	out := make([]mergeCandidate, 0, len(order))
	for _, id := range order {
		row, ok := rowByID[id]
		if !ok {
			continue
		}
		out = append(out, mergeCandidate{Row: row, Distance: distanceByID[id]})
	}
	return out, nil
}

// pickMergeTarget applies the legacy adjudication口径: cosine = 1 - distance,
// keep candidates at or above threshold, highest RefCount wins, ties go to the
// smaller ID.
func pickMergeTarget(candidates []mergeCandidate, threshold float64) *models.SemanticLabel {
	var best *models.SemanticLabel
	for i := range candidates {
		candidate := candidates[i].Row
		if 1-candidates[i].Distance < threshold {
			continue
		}
		if best == nil || candidate.RefCount > best.RefCount || (candidate.RefCount == best.RefCount && candidate.ID < best.ID) {
			copy := candidate
			best = &copy
		}
	}
	return best
}

// mergeRecallSaturated reports whether the recall pool was filled (LIMIT hit)
// with rows that are ALL at or above the merge threshold — the one case where a
// truncated ANN pool could hide a better (higher RefCount) target.
func mergeRecallSaturated(candidates []mergeCandidate, limit int, threshold float64) bool {
	if len(candidates) < limit {
		return false
	}
	for i := range candidates {
		if 1-candidates[i].Distance < threshold {
			return false
		}
	}
	return true
}
