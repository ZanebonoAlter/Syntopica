// Package related implements the related-events capability
// (enable-story-cross-day-continuation): event tags compared by a deterministic
// text representation of name + description + current auxiliary labels, cached
// as an isolated derived vector index and queried read-only per board.
//
// Boundaries fixed by design.md:
//   - D1/D2: the comparison unit is the event tag's own text. subject_key,
//     entity IDs, fingerprint trust and thread vectors are NEVER inputs; the
//     representation is versioned (RepresentationVersion) and never silently
//     truncated (over-budget text is rejected, not shortened).
//   - D3: related_event_embeddings is a discardable cache keyed by
//     (tag, representation version, space key). No merge edges, no identity,
//     no alias writes; the legacy tag/aux/story vectors are untouched.
//   - D4: the index is built only by an explicit refresh (CLI/related service);
//     the GET path never calls a model and never writes.
//   - D5: per-source, same-board, 30-day window, threshold, top-10 stable
//     ordering, no transitive expansion.
package related

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/logging"
)

const (
	// RepresentationVersion is part of the cache key: any change to the text
	// layout below MUST bump it so stale vectors stop participating.
	RepresentationVersion = "related-event-v1"

	// ThresholdKey is the online-overridable similarity threshold.
	ThresholdKey = "related_event_similarity_threshold"

	// DefaultThreshold is the documented default (design D6). The 2026-11-10
	// historical check (S3, board 1980) ran at the original 0.82 start value and
	// showed it was too loose for this representation (68% clearly relevant vs
	// 100% at >=0.86), so the default was raised to 0.86 per user decision. It is
	// still a measured default, not a calibrated guarantee.
	DefaultThreshold = 0.86

	// WindowDays is the candidate window counted backwards from the request
	// start (label created_at, NOT the event's occurrence date).
	WindowDays = 30

	// MaxResults truncates the display list per source. Candidates are never
	// truncated before the threshold filter.
	MaxResults = 10

	// GenerationKey stores the explicit index generation (D3: swapping provider
	// weights in place is not auto-detectable, so an operator must bump the
	// generation and rebuild instead of pretending the old vectors still fit).
	GenerationKey = "related_event_index_generation"

	// DefaultGeneration is the generation used when the setting is absent.
	DefaultGeneration = "g1"

	// Operation is the airouter operation name for index-refresh embeddings.
	// It is intentionally absent from airouter's embedding-cache allowlist:
	// every refresh is a real audited call, never a generic-cache hit.
	Operation = "tagmanagement.related_event.embedding"

	// maxInputRunes is the conservative per-text provider budget. Exceeding it
	// is a recorded failure (invalid_source), never a silent truncation that
	// would produce a second, undeclared representation.
	maxInputRunes = 8000

	// boundaryEpsilon keeps the documented inclusive threshold boundary robust
	// against float32 vector storage: a pair constructed exactly at the
	// threshold must be included, not dropped by ~1e-7 rounding inside pgvector.
	// The tolerance is far below the 0.01 step the UI edits, so it never changes
	// a user-visible decision.
	boundaryEpsilon = 1e-6
)

var (
	// ErrNotIndexable marks a source whose text cannot be built (blank name).
	ErrNotIndexable = errors.New("related-events: source text is not indexable")
	// ErrTextTooLong marks text that exceeds the provider input budget.
	ErrTextTooLong = errors.New("related-events: source text exceeds the input budget")
	// ErrBoardNotFound / ErrTagNotInBoard are mapped by handlers to 404.
	ErrBoardNotFound = errors.New("semantic board not found")
	ErrTagNotInBoard = errors.New("event tag is not an active member of the board")
	// ErrRouteUnavailable means no usable embedding route is configured.
	ErrRouteUnavailable = errors.New("related-events: no usable embedding route")
)

// NormalizeField trims and collapses every run of Unicode whitespace (spaces,
// tabs, ideographic spaces) into a single space. It is the ONE normalization
// used for names, descriptions and auxiliary labels, so equal inputs always
// produce the same text hash.
func NormalizeField(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalizeAuxLabels normalizes, de-duplicates and sorts auxiliary labels by
// code point. Order and duplicates in the source rows therefore never change
// the representation (spec: 文本表示稳定且辅助标签可缺失).
func normalizeAuxLabels(labels []string) []string {
	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, raw := range labels {
		norm := NormalizeField(raw)
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	sort.Strings(out)
	return out
}

// BuildRepresentation renders the v1 text: fixed field names, one field per
// block, empty blocks omitted. Auxiliary labels enhance the text but are never
// required — a tag with no aux labels still uses name + description.
func BuildRepresentation(label, description string, auxLabels []string) (string, error) {
	name := NormalizeField(label)
	if name == "" {
		return "", ErrNotIndexable
	}

	blocks := []string{"名称\n" + name}
	if desc := NormalizeField(description); desc != "" {
		blocks = append(blocks, "描述\n"+desc)
	}
	if aux := normalizeAuxLabels(auxLabels); len(aux) > 0 {
		blocks = append(blocks, "辅助标签\n"+strings.Join(aux, "\n"))
	}

	text := strings.Join(blocks, "\n\n")
	if len([]rune(text)) > maxInputRunes {
		return "", ErrTextTooLong
	}
	return text, nil
}

// HashText is the text signature persisted next to the cached vector. A row
// whose hash differs from the tag's current text is stale and MUST NOT take
// part in similarity computation.
func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Space identifies the embedding model space: provider (vendor) + model +
// explicit index generation. Vectors from different spaces are never compared,
// even when the dimensions happen to match.
type Space struct {
	Provider   string
	Model      string
	Generation string
}

// Key renders the space into the cache-table key form.
func (s Space) Key() string {
	return fmt.Sprintf("provider=%s|model=%s|gen=%s", s.Provider, s.Model, s.Generation)
}

// ResolveSpace resolves the CURRENT primary embedding binding from the
// configured route (a DB read — never a model call). Refresh writes rows under
// this space; the read-only query uses the same value, so a route change makes
// old rows invisible until an explicit refresh rewrites them.
func ResolveSpace(ctx context.Context, db *gorm.DB, generation string) (Space, error) {
	if strings.TrimSpace(generation) == "" {
		generation = DefaultGeneration
	}
	type row struct {
		ProviderName string `gorm:"column:provider_name"`
		ModelName    string `gorm:"column:model_name"`
	}
	var found row
	err := db.WithContext(ctx).Raw(`
		SELECT v.name AS provider_name, m.name AS model_name
		FROM ai_routes r
		JOIN ai_route_models rm ON rm.route_id = r.id AND rm.enabled = true
		JOIN ai_models m ON m.id = rm.model_id AND m.enabled = true AND m.status = 'ok'
		JOIN ai_vendors v ON v.id = m.vendor_id AND v.enabled = true
		WHERE r.capability = ? AND r.enabled = true
		ORDER BY CASE WHEN r.name = 'default' THEN 0 ELSE 1 END, r.id ASC, rm.priority ASC
		LIMIT 1`, string("embedding")).Scan(&found).Error
	if err != nil {
		return Space{}, fmt.Errorf("resolve embedding space: %w", err)
	}
	if found.ProviderName == "" || found.ModelName == "" {
		return Space{}, ErrRouteUnavailable
	}
	return Space{Provider: found.ProviderName, Model: found.ModelName, Generation: generation}, nil
}

// LoadGeneration reads the explicit index generation (default: g1). A missing
// or blank setting falls back to the default; the operator bumps it through the
// refresh CLI when provider weights are replaced in place.
func LoadGeneration(db *gorm.DB) string {
	var setting models.AISettings
	if err := db.Where("key = ?", GenerationKey).First(&setting).Error; err != nil {
		return DefaultGeneration
	}
	if gen := strings.TrimSpace(setting.Value); gen != "" {
		return gen
	}
	return DefaultGeneration
}

// SetGeneration persists the index generation (refresh CLI only).
func SetGeneration(db *gorm.DB, generation string) error {
	generation = strings.TrimSpace(generation)
	if generation == "" {
		return errors.New("related-events: generation must not be empty")
	}
	setting := models.AISettings{Key: GenerationKey, Value: generation, Description: "Related events index generation"}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "description", "updated_at"}),
	}).Create(&setting).Error
}

// LoadThreshold reads the online threshold: missing → default; invalid stored
// value → warn once per read and fall back to the default (never a silent 0).
func LoadThreshold(db *gorm.DB) float64 {
	var setting models.AISettings
	if err := db.Where("key = ?", ThresholdKey).First(&setting).Error; err != nil {
		return DefaultThreshold
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(setting.Value), 64)
	if err != nil || !validThreshold(value) {
		logging.Warnf("related-events: invalid %s=%q, falling back to %.2f", ThresholdKey, setting.Value, DefaultThreshold)
		return DefaultThreshold
	}
	return value
}

// validThreshold enforces the documented domain: finite number in [0, 1].
func validThreshold(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}
