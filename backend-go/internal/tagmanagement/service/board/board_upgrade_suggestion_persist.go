package board

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"syntopica-backend/internal/models"
)

// GenerateAndPersist runs one generation pass (mode) and persists the non-skip
// suggestions as pending rows, returning counts:
//   - inserted:         new pending rows written this run
//   - skipped:          idempotent duplicates (same hash already pending)
//   - cooldownBlocked:  same hash dismissed within the cooldown window
//
// It wraps the existing GenerateSuggestions (unchanged) with the persistence
// gate (design.md D3): skip decisions are never persisted; non-skip suggestions
// are deduplicated via the partial unique index and cooled-down via the
// dismissed-resolved_at record.
func (s *SemanticBoardUpgradeService) GenerateAndPersist(ctx context.Context, req UpgradeGenerateRequest) (inserted, skipped, cooldownBlocked int, err error) {
	if err := req.Validate(); err != nil {
		return 0, 0, 0, err
	}
	mode := req.ModeKey()
	suggestions, _, err := s.GenerateSuggestions(ctx, req)
	if err != nil {
		return 0, 0, 0, err
	}
	batchID := buildBatchID(mode)
	cooldownDays := s.LoadDismissCooldownDays(ctx)
	for _, sug := range suggestions {
		if sug.Decision == SemanticBoardUpgradeDecisionSkip {
			continue // skip decisions are never persisted (spec)
		}
		hash := ComputeSuggestionHash(mode, string(sug.Decision), sug.TargetBoardID, sug.AuxiliaryLabelIDs)
		// Cooldown gate: a hash dismissed within the cooldown window is blocked
		// from re-generation (spec: dismissed 冷却期).
		blocked, coolErr := s.suggestionRepo.CountDismissedInCooldown(ctx, hash, cooldownDays)
		if coolErr != nil {
			return inserted, skipped, cooldownBlocked, coolErr
		}
		if blocked > 0 {
			cooldownBlocked++
			continue
		}
		model := &models.BoardUpgradeSuggestion{
			BatchID:           batchID,
			Mode:              mode,
			Decision:          string(sug.Decision),
			BoardLabel:        sug.BoardLabel,
			Description:       sug.Description,
			TargetBoardID:     sug.TargetBoardID,
			AuxiliaryLabelIDs: sug.AuxiliaryLabelIDs,
			Confidence:        suggestionConfidence(sug.Confidence),
			Evidence:          sug.Evidence,
			Status:            "pending",
			SuggestionHash:    hash,
		}
		ok, insErr := s.suggestionRepo.InsertPending(ctx, model)
		if insErr != nil {
			return inserted, skipped, cooldownBlocked, insErr
		}
		if ok {
			inserted++
		} else {
			skipped++ // same hash already pending → idempotent no-op
		}
	}

	return inserted, skipped, cooldownBlocked, nil
}

// ComputeSuggestionHash returns a stable 32-hex-char fingerprint of
// (mode, decision, target_board_id, sorted auxiliary_label_ids). The partial
// unique index uq_board_upgrade_suggestions_hash keys on this for idempotent
// generation and dismissed-cooldown tracking (design.md D3).
//
// targetBoardID nil → "0"; auxiliary_label_ids are sorted ascending and joined
// by ",". sha256 of the concatenation (pipe-delimited), first 32 hex chars.
func ComputeSuggestionHash(mode, decision string, targetBoardID *uint, auxIDs []uint) string {
	sorted := UniqueUintSlice(auxIDs)
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	target := "0"
	if targetBoardID != nil {
		target = strconv.FormatUint(uint64(*targetBoardID), 10)
	}
	raw := mode + "|" + decision + "|" + target + "|" + strings.Join(parts, ",")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}

// buildBatchID mints a per-run batch identifier shared by all suggestions of one
// generation: UTC timestamp + mode (e.g. "20260718T063000Z-discover_new").
func buildBatchID(mode string) string {
	return time.Now().UTC().Format("20060102T150405Z") + "-" + mode
}

// suggestionConfidence normalizes the algorithm-produced confidence: "high"
// stays, anything else (incl. "" from the LLM path) defaults to "llm" (spec §4.3).
func suggestionConfidence(c string) string {
	if c == "high" {
		return "high"
	}
	return "llm"
}

// LoadDismissCooldownDays reads the dismissed-suggestion cooldown window from
// ai_settings key semantic_board_upgrade_suggestion_dismiss_cooldown_days
// (default 14). Mirrors the LoadUpgradeConfig pattern (spec: dismissed 冷却期).
func (s *SemanticBoardUpgradeService) LoadDismissCooldownDays(ctx context.Context) int {
	const defaultDays = 14
	var setting models.AISettings
	if err := s.db.WithContext(ctx).Where("key = ?", "semantic_board_upgrade_suggestion_dismiss_cooldown_days").First(&setting).Error; err != nil {
		return defaultDays // missing/invalid → default; not worth failing the run
	}
	return parseSemanticBoardUpgradeInt(setting.Value, defaultDays)
}
