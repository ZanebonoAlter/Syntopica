package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrRelationStateConflict is returned by lifecycle transitions when the row
// is not in the required source state (spec: illegal/duplicate transitions
// return a conflict while staying idempotent — a retry of an already-applied
// transition reports the conflict instead of mutating again).
var ErrRelationStateConflict = errors.New("cross-board relation state conflict")

// InsertOpenRelation persists a relation with an open status (unresolved or
// proposed). The partial unique index
// uq_cross_board_relations_open ON (suggestion_hash) WHERE status IN
// ('unresolved','proposed') enforces idempotent discovery: a second insert
// with the same open hash is a no-op returning inserted=false (spec: 重复发现
// 同一建议).
func (r *Repository) InsertOpenRelation(ctx context.Context, rel *CrossBoardRelation) (bool, error) {
	if rel.Status != RelationStatusUnresolved && rel.Status != RelationStatusProposed {
		return false, fmt.Errorf("insert open relation: status %q is not open", rel.Status)
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		// Target the partial unique index whose predicate covers open statuses.
		Columns:     []clause.Column{{Name: "suggestion_hash"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "status IN ('unresolved','proposed')"}}},
		DoNothing:   true,
	}).Create(rel)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// CountDismissedRelationsInCooldown counts dismissed rows sharing the hash and
// resolved within the last cooldownDays (spec: 驳回建议进入冷却). A positive
// count means re-discovery for this hash must be skipped.
func (r *Repository) CountDismissedRelationsInCooldown(ctx context.Context, hash string, cooldownDays int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&CrossBoardRelation{}).
		Where("suggestion_hash = ? AND status = ? AND dismissed_at >= NOW() - (? * INTERVAL '1 day')", hash, RelationStatusDismissed, cooldownDays).
		Count(&count).Error
	return count, err
}

// GetCrossBoardRelationByID loads one relation row.
func (r *Repository) GetCrossBoardRelationByID(ctx context.Context, id uint) (*CrossBoardRelation, error) {
	var rel CrossBoardRelation
	if err := r.db.WithContext(ctx).First(&rel, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &rel, nil
}

// CrossBoardRelationFilter scopes ListCrossBoardRelations. BoardID matches
// either side of the relation; Statuses is an exact IN filter (empty = no
// status filter).
type CrossBoardRelationFilter struct {
	BoardID  *uint
	Statuses []string
	Limit    int
}

// ListCrossBoardRelations returns relations for the filter, newest first.
func (r *Repository) ListCrossBoardRelations(ctx context.Context, f CrossBoardRelationFilter) ([]CrossBoardRelation, error) {
	q := r.db.WithContext(ctx).Model(&CrossBoardRelation{})
	if f.BoardID != nil {
		q = q.Where("source_board_id = ? OR target_board_id = ?", *f.BoardID, *f.BoardID)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("status IN ?", f.Statuses)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	var rows []CrossBoardRelation
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ConfirmCrossBoardRelation atomically transitions proposed → confirmed inside
// a transaction that first re-validates the target board still exists and is
// a live board label (spec: 确认前重验目标/证据/有效期). ttl sets the mandatory
// expiry; resolvedBy records the operator. A row not in proposed (or a target
// board that vanished) yields ErrRelationStateConflict with zero writes.
func (r *Repository) ConfirmCrossBoardRelation(ctx context.Context, id uint, resolvedBy string, ttl time.Duration) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rel CrossBoardRelation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&rel, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}
		if rel.Status != RelationStatusProposed {
			return fmt.Errorf("%w: confirm requires proposed, got %s (id=%d)", ErrRelationStateConflict, rel.Status, id)
		}
		if rel.TargetBoardID == nil {
			return fmt.Errorf("%w: confirm requires a resolved target board (id=%d)", ErrRelationStateConflict, id)
		}
		// Re-validate the target board is still a live board label at confirm
		// time (the resolver ran earlier; boards may have been deleted since).
		var boardLive bool
		if err := tx.Raw(`SELECT EXISTS (
			SELECT 1 FROM semantic_labels
			WHERE id = ? AND label_type = 'board')`, *rel.TargetBoardID).Scan(&boardLive).Error; err != nil {
			return err
		}
		if !boardLive {
			return fmt.Errorf("%w: target board %d no longer exists (id=%d)", ErrRelationStateConflict, *rel.TargetBoardID, id)
		}
		now := time.Now()
		expires := now.Add(ttl)
		return tx.Model(&CrossBoardRelation{}).Where("id = ?", id).Updates(map[string]any{
			"status":       RelationStatusConfirmed,
			"confirmed_at": now,
			"expires_at":   expires,
			"resolved_by":  resolvedBy,
			"updated_at":   now,
		}).Error
	})
}

// DismissCrossBoardRelation transitions proposed/unresolved → dismissed with
// a reason (spec: 用户驳回，进入冷却). Dismissing is allowed from unresolved as
// well: noise control for unresolvable suggestions relies on the same hash
// cooldown, otherwise garbage candidates would re-appear forever.
func (r *Repository) DismissCrossBoardRelation(ctx context.Context, id uint, reason, resolvedBy string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&CrossBoardRelation{}).
		Where("id = ? AND status IN ?", id, []string{RelationStatusProposed, RelationStatusUnresolved}).
		Updates(map[string]any{
			"status":         RelationStatusDismissed,
			"dismissed_at":   now,
			"dismiss_reason": reason,
			"resolved_by":    resolvedBy,
			"updated_at":     now,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: dismiss requires proposed/unresolved (id=%d)", ErrRelationStateConflict, id)
	}
	return nil
}

// ReResolveCrossBoardRelation replaces the mapping snapshot and (optionally)
// the bound target of an unresolved relation, and may promote it to proposed
// when the new resolution plus verdict qualify (status must stay unresolved
// when newStatus is unresolved; only unresolved rows are eligible).
func (r *Repository) ReResolveCrossBoardRelation(ctx context.Context, id uint, newStatus string, targetBoardID, targetLaneID *uint, mappingSnapshot []byte) error {
	if newStatus != RelationStatusUnresolved && newStatus != RelationStatusProposed {
		return fmt.Errorf("re-resolve: illegal new status %q", newStatus)
	}
	now := time.Now()
	updates := map[string]any{
		"status":           newStatus,
		"target_board_id":  targetBoardID,
		"target_lane_id":   targetLaneID,
		"mapping_snapshot": mappingSnapshot,
		"updated_at":       now,
	}
	res := r.db.WithContext(ctx).Model(&CrossBoardRelation{}).
		Where("id = ? AND status = ?", id, RelationStatusUnresolved).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: re-resolve requires unresolved (id=%d)", ErrRelationStateConflict, id)
	}
	return nil
}

// ListActiveConfirmedRelationsForBoard returns confirmed, unexpired relations
// touching the board on either side, ordered deterministically
// (quality_grade DESC, confirmed_at DESC, id ASC — spec: 关系数量超过预算的
// 稳定排序). Expiry is enforced read-time: expires_at > NOW().
func (r *Repository) ListActiveConfirmedRelationsForBoard(ctx context.Context, boardID uint) ([]CrossBoardRelation, error) {
	var rows []CrossBoardRelation
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at > NOW() AND (source_board_id = ? OR target_board_id = ?)",
			RelationStatusConfirmed, boardID, boardID).
		Order("CASE quality_grade WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END DESC, confirmed_at DESC, id ASC").
		Find(&rows).Error
	return rows, err
}

// ExpireConfirmedRelations batch-marks confirmed relations whose expires_at
// has passed as expired (maintenance task; idempotent, returns rows touched).
// Read paths already treat them as expired — this only persists the state.
func (r *Repository) ExpireConfirmedRelations(ctx context.Context) (int64, error) {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&CrossBoardRelation{}).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at <= NOW()", RelationStatusConfirmed).
		Updates(map[string]any{"status": RelationStatusExpired, "expired_at": now, "updated_at": now})
	return res.RowsAffected, res.Error
}

// ── Runs ──────────────────────────────────────────────────────────────────────

// CreateRelationRun persists a new discovery run.
func (r *Repository) CreateRelationRun(ctx context.Context, run *CrossBoardRelationRun) error {
	return r.db.WithContext(ctx).Create(run).Error
}

// UpdateRelationRunStatus stores the terminal status plus error text for a run.
func (r *Repository) UpdateRelationRunStatus(ctx context.Context, id uint, status, runErr string) error {
	return r.db.WithContext(ctx).Model(&CrossBoardRelationRun{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "error": runErr, "updated_at": time.Now()}).Error
}

// GetRelationRunByID loads one run row.
func (r *Repository) GetRelationRunByID(ctx context.Context, id uint) (*CrossBoardRelationRun, error) {
	var run CrossBoardRelationRun
	if err := r.db.WithContext(ctx).First(&run, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &run, nil
}
