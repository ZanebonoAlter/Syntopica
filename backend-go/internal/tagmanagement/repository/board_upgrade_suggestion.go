package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
)

// BoardUpgradeSuggestionRepository persists semantic-board upgrade suggestions
// and drives their lifecycle (pending → confirmed / dismissed). See
// design.md D3 of change board-discovery-expansion.
type BoardUpgradeSuggestionRepository struct {
	db *gorm.DB
}

func NewBoardUpgradeSuggestionRepository(db *gorm.DB) *BoardUpgradeSuggestionRepository {
	return &BoardUpgradeSuggestionRepository{db: db}
}

// InsertPending persists a suggestion row with status='pending'. The partial
// unique index uq_board_upgrade_suggestions_hash ON (suggestion_hash) WHERE
// status='pending' enforces idempotent generation: a second insert with the
// same pending hash is a no-op (ON CONFLICT DO NOTHING), returning
// inserted=false. This makes re-running the generator a safe no-op for an
// unchanged cluster (spec: 建议生成幂等).
func (r *BoardUpgradeSuggestionRepository) InsertPending(ctx context.Context, sug *models.BoardUpgradeSuggestion) (bool, error) {
	sug.Status = "pending"
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		// Target the partial unique index whose predicate is status='pending'.
		Columns:     []clause.Column{{Name: "suggestion_hash"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "status = 'pending'"}}},
		DoNothing:   true,
	}).Create(sug)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// MarkConfirmed transitions a pending suggestion to confirmed, recording
// resolved_at=now. It MUST run inside the upgrade-execute transaction (tx) so a
// board_composition write failure rolls the suggestion state back unchanged
// (spec: confirm 联动). Only affects rows currently pending (idempotent against
// a double-confirm of an already-resolved suggestion).
func (r *BoardUpgradeSuggestionRepository) MarkConfirmed(tx *gorm.DB, id uint) error {
	return tx.Model(&models.BoardUpgradeSuggestion{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(map[string]interface{}{
			"status":      "confirmed",
			"resolved_at": time.Now(),
		}).Error
}

// CountDismissedInCooldown returns how many dismissed suggestions share the
// given hash and were resolved within the last cooldownDays (spec: dismissed
// 冷却期). A positive count means the hash is in cooldown and re-generation
// must be skipped. resolved_at is the cooldown start (set when the row was
// dismissed). The partial unique index allows many non-pending rows per hash,
// so this counts across all statuses='dismissed'.
func (r *BoardUpgradeSuggestionRepository) CountDismissedInCooldown(ctx context.Context, hash string, cooldownDays int) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.BoardUpgradeSuggestion{}).
		Where("suggestion_hash = ? AND status = ? AND resolved_at >= NOW() - (? * INTERVAL '1 day')", hash, "dismissed", cooldownDays).
		Count(&count).Error
	return count, err
}

// List returns persisted suggestions filtered by status/decision (spec: 建议查询
// API 读持久化表). Filtering rules:
//   - status==""   → no status filter (handler supplies the default "pending")
//   - decision=="" → default list excludes watch (decision <> 'watch')
//   - decision=="watch" → observation pool only
//   - decision=<other>  → exact decision match
//
// Ordering: high-confidence first, then newest (created_at DESC). This puts the
// most actionable, highest-signal suggestions at the top of the panel.
func (r *BoardUpgradeSuggestionRepository) List(ctx context.Context, status, decision string) ([]models.BoardUpgradeSuggestion, error) {
	q := r.db.WithContext(ctx).Model(&models.BoardUpgradeSuggestion{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	switch decision {
	case "":
		q = q.Where("decision <> ?", "watch")
	case "watch":
		q = q.Where("decision = ?", "watch")
	default:
		q = q.Where("decision = ?", decision)
	}
	var rows []models.BoardUpgradeSuggestion
	err := q.Order("CASE WHEN confidence = 'high' THEN 0 ELSE 1 END, created_at DESC").Find(&rows).Error
	return rows, err
}

// MarkDismissed transitions a pending suggestion to dismissed, recording
// resolved_at=now, resolved_by="manual" and the optional reason (spec: 建议
// dismiss 与 confirm 联动). Only pending rows are affected; an already-resolved
// suggestion is a no-op (idempotent against a double-dismiss).
func (r *BoardUpgradeSuggestionRepository) MarkDismissed(ctx context.Context, id uint, reason string) error {
	updates := map[string]interface{}{
		"status":      "dismissed",
		"resolved_at": time.Now(),
		"resolved_by": "manual",
	}
	if reason != "" {
		updates["dismiss_reason"] = reason
	}
	return r.db.WithContext(ctx).Model(&models.BoardUpgradeSuggestion{}).
		Where("id = ? AND status = ?", id, "pending").
		Updates(updates).Error
}
