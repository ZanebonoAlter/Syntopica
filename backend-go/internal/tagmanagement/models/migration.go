package models

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
)

// SeedEmbeddingConfigs inserts default embedding_config rows that do not exist
// yet. Wired into database.EmbeddingConfigSeeder by the tagmanagement root
// (production) and platform/testutil (golden schema) — moved verbatim from
// platform/database/postgres_migrations.go in decouple-backend-domains to
// break the database⇄tagmanagement/models import cycle.
func SeedEmbeddingConfigs(db *gorm.DB, defaults []struct {
	Key         string
	Value       string
	Description string
}) error {
	for _, d := range defaults {
		var existing EmbeddingConfig
		if err := db.Where("key = ?", d.Key).First(&existing).Error; err != nil {
			if err := db.Create(&EmbeddingConfig{Key: d.Key, Value: d.Value, Description: d.Description}).Error; err != nil {
				logging.Warnf("Warning: failed to seed embedding_config key %s: %v", d.Key, err)
			}
		}
	}
	return nil
}

// RunAuxLabelDupMerge performs a one-shot deduplication of active auxiliary
// labels whose normalizeKey is identical (text-variant duplicates like
// "SK 海力士" / "SK海力士").
//
// For each group: the label with the highest ref_count (ties broken by smallest
// id) is the primary; all others are merged into it by moving aliases,
// topic_tag_semantic_labels, and board_composition references, then disabling
// the source. This mirrors the runtime MergeAuxiliaryLabelAlias semantics.
//
// Idempotent: after a successful run no group will have count>1 (disabled
// labels are excluded from the grouping query).
func RunAuxLabelDupMerge(db *gorm.DB) error {
	// Query active auxiliary labels.
	type auxRow struct {
		ID       uint
		Label    string
		RefCount int
	}
	var allRows []auxRow
	if err := db.Model(&models.SemanticLabel{}).
		Select("id, label, ref_count").
		Where("label_type = ? AND status = ?", "auxiliary", "active").
		Order("id ASC").
		Find(&allRows).Error; err != nil {
		return fmt.Errorf("dup-merge: query active auxiliary labels: %w", err)
	}

	// Group by normalizeKey.
	groups := make(map[string][]auxRow)
	for _, r := range allRows {
		nk := textutil.NormalizeLabelKey(r.Label)
		groups[nk] = append(groups[nk], r)
	}

	var mergeCount int
	for nk, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Primary = highest ref_count, ties broken by smallest id.
		primary := group[0]
		for i := 1; i < len(group); i++ {
			if group[i].RefCount > primary.RefCount ||
				(group[i].RefCount == primary.RefCount && group[i].ID < primary.ID) {
				primary = group[i]
			}
		}

		for _, secondary := range group {
			if secondary.ID == primary.ID {
				continue
			}

			logging.Infof("Dup-merge: normalize_key=%q: merging source=%d(%q, ref=%d) → target=%d(%q, ref=%d)",
				nk, secondary.ID, secondary.Label, secondary.RefCount, primary.ID, primary.Label, primary.RefCount)

			if err := mergeOneAuxLabelDup(db, secondary.ID, primary.ID); err != nil {
				return fmt.Errorf("dup-merge: merge source=%d into target=%d: %w", secondary.ID, primary.ID, err)
			}
			mergeCount++
		}

		logging.Infof("Dup-merge: normalized %d duplicates into primary %q (id=%d)", len(group)-1, primary.Label, primary.ID)
	}

	logging.Infof("Dup-merge: complete — %d auxiliary labels merged across all groups", mergeCount)

	// Cache invalidation is handled by the database-side hook wrapper
	// (database.InvalidateBoardCache) after this function returns.

	return nil
}

// mergeOneAuxLabelDup merges a single source auxiliary label into a target.
// Mirrors MergeAuxiliaryLabelAlias semantics but uses direct DB operations
// (the service method lives in a package that would create a circular import).
func mergeOneAuxLabelDup(db *gorm.DB, sourceID, targetID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var source, target models.SemanticLabel
		if err := tx.Where("id = ? AND label_type = ?", sourceID, "auxiliary").First(&source).Error; err != nil {
			return fmt.Errorf("load source: %w", err)
		}
		if err := tx.Where("id = ? AND label_type = ?", targetID, "auxiliary").First(&target).Error; err != nil {
			return fmt.Errorf("load target: %w", err)
		}

		// Merge aliases: source label + source aliases → target aliases (dedup).
		aliasSet := make(map[string]bool)
		for _, a := range target.Aliases {
			aliasSet[strings.ToLower(strings.TrimSpace(a))] = true
		}
		for _, a := range append([]string{source.Label}, source.Aliases...) {
			key := strings.ToLower(strings.TrimSpace(a))
			if !aliasSet[key] && !strings.EqualFold(target.Label, a) {
				target.Aliases = append(target.Aliases, a)
				aliasSet[key] = true
			}
		}
		if err := tx.Save(&target).Error; err != nil {
			return fmt.Errorf("save target aliases: %w", err)
		}

		// Migrate topic_tag_semantic_labels: source → target, ON CONFLICT DO NOTHING.
		var links []TopicTagSemanticLabel
		if err := tx.Where("semantic_label_id = ?", sourceID).Find(&links).Error; err != nil {
			return fmt.Errorf("load source links: %w", err)
		}
		for _, link := range links {
			migrated := TopicTagSemanticLabel{TopicTagID: link.TopicTagID, SemanticLabelID: targetID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&migrated).Error; err != nil {
				return fmt.Errorf("migrate link topic_tag=%d: %w", link.TopicTagID, err)
			}
		}
		if err := tx.Where("semantic_label_id = ?", sourceID).Delete(&TopicTagSemanticLabel{}).Error; err != nil {
			return fmt.Errorf("delete source links: %w", err)
		}

		// Migrate board_composition: source → target, ON CONFLICT DO NOTHING.
		type boardCompRow struct {
			BoardID          uint `gorm:"column:board_id"`
			AuxiliaryLabelID uint `gorm:"column:auxiliary_label_id"`
		}
		var comps []boardCompRow
		if err := tx.Table("board_composition").Where("auxiliary_label_id = ?", sourceID).Find(&comps).Error; err != nil {
			return fmt.Errorf("load source board_composition: %w", err)
		}
		for _, comp := range comps {
			if err := tx.Exec(`
				INSERT INTO board_composition (board_id, auxiliary_label_id)
				VALUES (?, ?) ON CONFLICT DO NOTHING
			`, comp.BoardID, targetID).Error; err != nil {
				return fmt.Errorf("migrate board_composition board=%d: %w", comp.BoardID, err)
			}
		}
		if err := tx.Where("auxiliary_label_id = ?", sourceID).Delete(&BoardComposition{}).Error; err != nil {
			return fmt.Errorf("delete source board_composition: %w", err)
		}

		// Recalculate ref_counts.
		var targetRefCount int64
		if err := tx.Model(&TopicTagSemanticLabel{}).Where("semantic_label_id = ?", targetID).Count(&targetRefCount).Error; err != nil {
			return fmt.Errorf("count target refs: %w", err)
		}
		var sourceRefCount int64
		if err := tx.Model(&TopicTagSemanticLabel{}).Where("semantic_label_id = ?", sourceID).Count(&sourceRefCount).Error; err != nil {
			return fmt.Errorf("count source refs: %w", err)
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", targetID).Update("ref_count", int(targetRefCount)).Error; err != nil {
			return fmt.Errorf("update target ref_count: %w", err)
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", sourceID).Updates(map[string]any{
			"ref_count":       int(sourceRefCount),
			"status":          "disabled",
			"embedding":       nil,
			"merge_embedding": nil,
		}).Error; err != nil {
			return fmt.Errorf("disable source: %w", err)
		}

		return nil
	})
}
