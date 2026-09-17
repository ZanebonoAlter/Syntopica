package articlerefs

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// NormalizeThreadRefs rewrites every non-array reference value to [] — both the
// JSON null scalar and SQL NULL found in historical rows. Without this, later
// jsonb_array_elements_text calls raise SQLSTATE 22023 (and a naive query then
// fails as a whole, which reads like "zero matches" instead of an error).
// Returns the number of rows written; already-normalized data yields 0.
func NormalizeThreadRefs(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, errNilDB
	}
	result := db.Exec(`UPDATE ` + threadRefsTable + ` SET ` + refsColumn + ` = '[]'::jsonb
		WHERE jsonb_typeof(` + refsColumn + `) IS DISTINCT FROM 'array'`)
	if result.Error != nil {
		return 0, fmt.Errorf("normalize thread refs: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// PruneDanglingRefs walks the thread table in id windows and drops every
// reference that has no live articles row, reporting how many rows it rewrote
// and how many dangling references it removed.
//
// It is the one-shot repair for references that were orphaned before the delete
// paths maintained them (heal-dangling-article-refs D5). Historical orphans are
// pruned rather than pointed at a survivor: the (feed_id, link) merge recorded no
// loser→keeper mapping and the loser row is gone, so the target cannot be
// inferred; the equivalent keeper is normally already in the same array.
//
// Idempotent: a second pass loads the same rows but changes nothing, because a
// reference now has a live row behind it. batch <= 0 uses DefaultBatchSize.
func PruneDanglingRefs(db *gorm.DB, batch int) (rowsTouched int64, refsRemoved int64, err error) {
	if db == nil {
		return 0, 0, errNilDB
	}
	if batch <= 0 {
		batch = DefaultBatchSize
	}
	var after uint
	for {
		rows, loadErr := loadRefRowsAfter(db, after, batch)
		if loadErr != nil {
			return rowsTouched, refsRemoved, loadErr
		}
		if len(rows) == 0 {
			return rowsTouched, refsRemoved, nil
		}
		after = rows[len(rows)-1].ID

		// One existence lookup per window instead of one per row: resolve every
		// referenced ID, then decide per row which elements to drop.
		parsed := make([][]uint, len(rows))
		invalid := make([]bool, len(rows))
		candidates := make([]uint, 0, len(rows)*4)
		for i, row := range rows {
			ids, hadInvalid := parseRefIDs(row.Refs)
			parsed[i], invalid[i] = ids, hadInvalid
			candidates = append(candidates, ids...)
		}
		existing, existErr := ExistingArticleIDs(db, candidates)
		if existErr != nil {
			return rowsTouched, refsRemoved, existErr
		}

		for i, row := range rows {
			drop := make(map[uint]bool)
			removed := 0
			for _, id := range parsed[i] {
				if !existing[id] {
					drop[id] = true
					removed++
				}
			}
			current := parsed[i]
			next, changed := rewriteRefIDs(current, func(id uint) (uint, bool) {
				if drop[id] {
					return 0, false
				}
				return id, true
			})
			if invalid[i] {
				changed = true
			}
			if !changed {
				continue
			}
			if updateErr := updateRefs(db, row.ID, next); updateErr != nil {
				return rowsTouched, refsRemoved, updateErr
			}
			rowsTouched++
			refsRemoved += int64(removed)
		}
	}
}

// CountDanglingArticleRefs reports how many thread references point at a missing
// article row. Read-only probe for the daily-report job's integrity log; it never
// deletes anything (an unknown deleter must stay visible, heal-dangling-article-refs D6).
func CountDanglingArticleRefs(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, errNilDB
	}
	var count int64
	query := `SELECT COUNT(*) FROM ` + threadRefsTable + ` t
		CROSS JOIN LATERAL jsonb_array_elements_text(` + refsArraySQL + `) AS e(elem)
		WHERE NOT EXISTS (SELECT 1 FROM articles a WHERE a.id::text = e.elem)`
	if err := db.Raw(query).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count dangling article refs: %w", err)
	}
	return count, nil
}

// ExistingArticleIDs returns the subset of ids that still have an articles row.
// Callers use it before writing references (the daily-report write path) or
// while repairing existing ones. ids are looked up in chunks of articleIDChunk;
// the placeholder list is built explicitly so no driver has to encode a Go slice.
func ExistingArticleIDs(db *gorm.DB, ids []uint) (map[uint]bool, error) {
	if db == nil {
		return nil, errNilDB
	}
	unique := uniqueIDs(ids)
	existing := make(map[uint]bool, len(unique))
	for start := 0; start < len(unique); start += articleIDChunk {
		end := start + articleIDChunk
		if end > len(unique) {
			end = len(unique)
		}
		chunk := unique[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]interface{}, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		var found []uint
		query := "SELECT id FROM articles WHERE id IN (" + placeholders + ")"
		if err := db.Raw(query, args...).Scan(&found).Error; err != nil {
			return nil, fmt.Errorf("check article existence: %w", err)
		}
		for _, id := range found {
			existing[id] = true
		}
	}
	return existing, nil
}
