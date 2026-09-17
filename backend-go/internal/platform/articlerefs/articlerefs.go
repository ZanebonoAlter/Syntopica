// Package articlerefs maintains "references to an article row by ID" held in
// jsonb arrays. Deleting an articles row used to leave those references
// dangling, which is what made daily-report threads point at articles that no
// longer exist (the frontend then degrades to "文章 #<id>").
//
// Scope (verified 2026-09-17 against every jsonb column in the schema):
// daily_report_threads.related_article_ids is the ONLY local article-ID holder;
// other jsonb columns carry tag IDs, external URLs or free text, so they are out
// of scope by construction rather than by omission.
//
// Contract for callers (heal-dangling-article-refs D2): a delete path MUST call
// RewireArticleRefs (when a surviving equivalent row exists) or PruneArticleRefs
// (when it does not) BEFORE deleting the article rows, inside the same
// transaction, so the two writes commit or roll back together.
//
// Rewrites here preserve element order and drop duplicates (first occurrence
// wins): the daily-report reader slices the first ten references for display, so
// reordering would change what users see. Empty arrays are written as the
// literal [] — never as a JSON null scalar, which makes
// jsonb_array_elements_text raise SQLSTATE 22023 for every later query.
package articlerefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	// threadRefsTable / refsColumn name the single jsonb holder of local article
	// IDs. Kept as literals (not models) on purpose: this package stays free of
	// domain imports so platform and domain code can both depend on it.
	threadRefsTable = "daily_report_threads"
	refsColumn      = "related_article_ids"

	// DefaultBatchSize is PruneDanglingRefs' window when the caller passes a
	// non-positive batch.
	DefaultBatchSize = 500

	// articleIDChunk bounds the IN (...) lists of existence lookups. PostgreSQL
	// allows 65535 bind parameters, but small chunks keep the plans cheap.
	articleIDChunk = 1000
)

// errNilDB guards the entry points: a nil handle would otherwise panic deep
// inside GORM, which is harder to attribute than a returned error.
var errNilDB = errors.New("articlerefs: nil db")

// refsArraySQL renders the reference column as a jsonb array. Historical rows
// carry a JSON null scalar (or SQL NULL) instead of [], and feeding either into
// jsonb_array_elements_text raises SQLSTATE 22023 — so every read goes through
// this guard. The alias `t` is fixed by convention across the queries here.
const refsArraySQL = `CASE WHEN jsonb_typeof(t.` + refsColumn + `) = 'array' THEN t.` + refsColumn + ` ELSE '[]'::jsonb END`

// refRow is one thread row as loaded for maintenance: its primary key plus the
// raw jsonb value of the reference array.
type refRow struct {
	ID   uint
	Refs []byte
}

// parseRefIDs decodes a jsonb array of article IDs. Anything that is not a JSON
// array — the JSON null scalar, SQL NULL, or an unexpected scalar — reads as
// zero references and is reported through dropped so the caller rewrites the
// column: jsonb_array_elements_text cannot read such a value, so preserving it
// would keep breaking every query that touches the column. Elements that are not
// positive integers are dropped too (test-cases B6): they cannot reference an
// article row, so keeping one would only perpetuate a dangling reference.
// Numeric strings are accepted because historical writers were not consistent
// about them.
func parseRefIDs(raw []byte) (ids []uint, dropped bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, true
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(trimmed, &elements); err != nil {
		return nil, true
	}
	ids = make([]uint, 0, len(elements))
	for _, element := range elements {
		text := strings.TrimSpace(string(element))
		text = strings.Trim(text, `"`)
		id, err := strconv.ParseUint(text, 10, 64)
		if err != nil || id == 0 {
			dropped = true
			continue
		}
		ids = append(ids, uint(id))
	}
	return ids, dropped
}

// rewriteRefIDs maps ids through keep, preserving order and dropping duplicate
// results (first occurrence wins). keep returns ok=false for an element that
// must disappear. changed reports whether the resulting array differs from the
// input, which is what decides whether a row is written at all (idempotency:
// a clean row produces no UPDATE).
func rewriteRefIDs(ids []uint, keep func(uint) (uint, bool)) ([]uint, bool) {
	out := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	changed := false
	for _, id := range ids {
		next, ok := keep(id)
		if !ok {
			changed = true
			continue
		}
		if next != id {
			changed = true
		}
		if seen[next] {
			changed = true
			continue
		}
		seen[next] = true
		out = append(out, next)
	}
	return out, changed
}

// marshalRefIDs renders ids as a jsonb array literal, normalizing nil/empty to
// [] so the stored value is always readable by jsonb_array_elements_text.
func marshalRefIDs(ids []uint) (string, error) {
	if len(ids) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("marshal article refs: %w", err)
	}
	return string(payload), nil
}

// updateRefs writes one row's rewritten array. The ::jsonb cast keeps the bound
// parameter a plain string, so the driver never has to encode a Go slice.
func updateRefs(db *gorm.DB, id uint, ids []uint) error {
	payload, err := marshalRefIDs(ids)
	if err != nil {
		return err
	}
	if err := db.Exec(`UPDATE `+threadRefsTable+` SET `+refsColumn+` = ?::jsonb WHERE id = ?`, payload, id).Error; err != nil {
		return fmt.Errorf("update thread %d refs: %w", id, err)
	}
	return nil
}

// loadRefRowsContaining returns every thread whose reference array contains at
// least one of want. Elements are compared as text, so a dirty element can never
// raise a cast error. want must be non-empty.
func loadRefRowsContaining(db *gorm.DB, want []string) ([]refRow, error) {
	payload, err := json.Marshal(want)
	if err != nil {
		return nil, fmt.Errorf("marshal wanted ids: %w", err)
	}
	var rows []refRow
	query := `SELECT t.id, t.` + refsColumn + ` AS refs FROM ` + threadRefsTable + ` t
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements_text(` + refsArraySQL + `) AS e(elem)
			WHERE e.elem IN (SELECT jsonb_array_elements_text(?::jsonb))
		)
		ORDER BY t.id`
	if err := db.Raw(query, string(payload)).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load thread refs: %w", err)
	}
	return rows, nil
}

// loadRefRowsAfter returns up to limit thread rows with an id greater than
// after, ordered by id. It is the windowed scan the repair walks: the cursor is
// the last id seen, so newly inserted rows are never revisited by the same pass.
func loadRefRowsAfter(db *gorm.DB, after uint, limit int) ([]refRow, error) {
	var rows []refRow
	query := `SELECT t.id, t.` + refsColumn + ` AS refs FROM ` + threadRefsTable + ` t
		WHERE t.id > ? ORDER BY t.id LIMIT ?`
	if err := db.Raw(query, after, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("load thread refs after id %d: %w", after, err)
	}
	return rows, nil
}

// rewriteRows applies rewrite to every loaded row, updating only the rows whose
// array actually changes. droppedInvalid marks rows whose stored array carried
// unusable elements: those always need a write even when the rewrite itself is a
// no-op. Returns the number of updated rows.
func rewriteRows(db *gorm.DB, rows []refRow, rewrite func([]uint) ([]uint, bool)) (int64, error) {
	var updated int64
	for _, row := range rows {
		ids, hadInvalid := parseRefIDs(row.Refs)
		next, changed := rewrite(ids)
		if hadInvalid {
			changed = true
		}
		if !changed {
			continue
		}
		if err := updateRefs(db, row.ID, next); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

// idStrings renders ids as decimal strings for the jsonb element comparison
// (text matching avoids casting untrusted elements). Zero is skipped: it can
// never be an article primary key, and it cannot appear from parseRefIDs.
func idStrings(ids []uint) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, strconv.FormatUint(uint64(id), 10))
	}
	return out
}

// uniqueIDs deduplicates ids, dropping zeros, so existence lookups stay small.
func uniqueIDs(ids []uint) []uint {
	out := make([]uint, 0, len(ids))
	seen := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
