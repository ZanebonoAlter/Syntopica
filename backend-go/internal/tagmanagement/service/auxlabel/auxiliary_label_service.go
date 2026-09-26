package auxlabel

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/textutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/service/core"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"syntopica-backend/internal/tagmanagement/repository"
)

type AuxLabelGCMode string

const (
	AuxLabelGCModeDryRun      AuxLabelGCMode = "dry_run"
	AuxLabelGCModeDisable     AuxLabelGCMode = "disable"
	AuxLabelGCModeDelete      AuxLabelGCMode = "delete"
	AuxLabelGCModeRecalculate AuxLabelGCMode = "recalculate"
)

type AuxLabelGCRequest struct {
	Mode      AuxLabelGCMode `json:"mode"`
	GraceDays int            `json:"grace_days"`
}

type AuxLabelGCResult struct {
	EligibleCount  int                     `json:"eligible_count"`
	AffectedCount  int                     `json:"affected_count"`
	CorrectedCount int                     `json:"corrected_count,omitempty"`
	TotalCount     int                     `json:"total_count,omitempty"`
	Preview        []AuxLabelGCPreviewItem `json:"preview,omitempty"`
}

type AuxLabelGCPreviewItem struct {
	ID        uint   `json:"id"`
	Label     string `json:"label"`
	RefCount  int    `json:"ref_count"`
	CreatedAt string `json:"created_at"`
}

// defaultAuxLabelDedupeSim is the fallback L2 merge similarity threshold when
// ai_settings.auxiliary_label_dedupe_sim is missing or invalid. The effective
// threshold is loaded per L2 evaluation by loadAuxLabelDedupeSim (design D5).
const defaultAuxLabelDedupeSim = 0.95

var ensureVectorDimOnce sync.Once

type AuxiliaryLabelEmbeddingMode string

const (
	AuxiliaryLabelEmbeddingModeMerge   AuxiliaryLabelEmbeddingMode = "merge"
	AuxiliaryLabelEmbeddingModeStorage AuxiliaryLabelEmbeddingMode = "storage"
)

type AuxiliaryLabelEmbedder func(ctx context.Context, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error)

type mergeMatcherFunc func(ctx context.Context, db *gorm.DB, labels []models.SemanticLabel, mergePgVector string, mergeVector []float64) (*models.SemanticLabel, error)

const (
	auxActiveLabelsTTL    = 5 * time.Minute
	auxMergeEmbeddingsTTL = 10 * time.Minute
)

type auxLabelCache struct {
	mu sync.RWMutex

	// Active auxiliary labels cache
	activeLabels   []models.SemanticLabel
	activeLabelsAt time.Time

	// Merge embedding vectors cache
	mergeEmbeddings   map[uint][]float64
	mergeEmbeddingsAt time.Time
}

type AuxiliaryLabelService struct {
	db           *gorm.DB
	embedder     AuxiliaryLabelEmbedder
	mergeMatcher mergeMatcherFunc
	cache        *auxLabelCache
}

// package-level cache for AuxiliaryLabelService — shared across instances
var packageAuxLabelCache = &auxLabelCache{}

// InvalidateAuxLabelCache clears the package-level auxiliary label cache.
// Tests that mutate semantic_labels (or whose ResetTestData truncates tables)
// must call this so stale cached labels do not span test boundaries —
// ResetTestData clears the DB but not this in-memory cache.
func InvalidateAuxLabelCache() {
	packageAuxLabelCache.mu.Lock()
	packageAuxLabelCache.activeLabels = nil
	packageAuxLabelCache.mergeEmbeddings = nil
	packageAuxLabelCache.activeLabelsAt = time.Time{}
	packageAuxLabelCache.mergeEmbeddingsAt = time.Time{}
	packageAuxLabelCache.mu.Unlock()
}

func NewAuxiliaryLabelService(db *gorm.DB, embedder AuxiliaryLabelEmbedder) *AuxiliaryLabelService {
	if db == nil {
		db = repository.Repo.DB()
	}
	if embedder == nil {
		embedder = DefaultAuxiliaryLabelEmbedder
	}
	return &AuxiliaryLabelService{db: db, embedder: embedder, mergeMatcher: sqlMergeMatcher, cache: packageAuxLabelCache}
}

// vectorDimEnsurers holds callbacks registered by other packages to ensure their
// vector columns have the correct dimension. Called once at startup after the
// embedding dimension is determined.
var vectorDimEnsurers []func(dim int)

// RegisterVectorDimEnsurer registers a callback to be invoked at startup with the
// detected embedding dimension. Call from init() in domain packages that own vector columns.
func RegisterVectorDimEnsurer(fn func(dim int)) {
	vectorDimEnsurers = append(vectorDimEnsurers, fn)
}

// EnsureVectorDimensionOnce ensures all vector column dimensions match the embedder output.
// Called once at startup. Uses the global DB to avoid calling DDL inside a transaction.
func EnsureVectorDimensionOnce(ctx context.Context) {
	ensureVectorDimOnce.Do(func() {
		_, vector, err := DefaultAuxiliaryLabelEmbedder(ctx, "dimension-check", AuxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			logging.Warnf("Failed to determine embedding dimension: %v", err)
			return
		}
		dim := len(vector)
		if err := EnsureSemanticLabelVectorDimension(dim); err != nil {
			logging.Errorf("Failed to ensure semantic_labels.embedding vector dimension: %v; "+
				"INSERT into semantic_labels will fail until this is resolved", err)
		}
		if err := EnsureSemanticLabelMergeVectorDimension(dim); err != nil {
			logging.Errorf("Failed to ensure semantic_labels.merge_embedding vector dimension: %v; "+
				"INSERT into semantic_labels will fail until this is resolved", err)
		}
		for _, fn := range vectorDimEnsurers {
			fn(dim)
		}
	})
}

func (s *AuxiliaryLabelService) AttachAuxiliaryLabels(ctx context.Context, topicTagID uint, labels []core.AuxiliaryLabel) error {
	if topicTagID == 0 || len(labels) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txService := NewAuxiliaryLabelService(tx, s.embedder)
		for _, item := range labels {
			label, err := txService.ResolveAuxiliaryLabel(ctx, item.Label, item.Description)
			if err != nil {
				return err
			}
			link := tagmodels.TopicTagSemanticLabel{TopicTagID: topicTagID, SemanticLabelID: label.ID}
			res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", label.ID).UpdateColumn("ref_count", gorm.Expr("ref_count + 1")).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *AuxiliaryLabelService) ResolveAuxiliaryLabel(ctx context.Context, rawLabel, description string) (*models.SemanticLabel, error) {
	label := strings.TrimSpace(rawLabel)
	if label == "" {
		return nil, fmt.Errorf("auxiliary label must not be empty")
	}
	if _, generic := core.GenericAuxiliaryLabels[label]; generic {
		return nil, fmt.Errorf("auxiliary label %q is too generic", label)
	}
	slug := core.Slugify(label)
	if slug == "" {
		return nil, fmt.Errorf("auxiliary label slug is empty")
	}

	description = strings.TrimSpace(description)

	labels, err := s.loadActiveAuxiliaryLabels(ctx)
	if err != nil {
		return nil, err
	}

	gateStart := time.Now()
	// L1: exact match by slug, alias, or whitespace-insensitive normalize key.
	// The normalize key (strip all whitespace + lower) collapses text variants
	// like "SK 海力士" / "SK海力士" that Slugify misses (Slugify keeps a single
	// space). Same function as the one-shot dup-merge migration (design D5/D6).
	normalizeKey := textutil.NormalizeLabelKey(label)
	for _, existing := range labels {
		if existing.Slug == slug || semanticAliasesContain(existing.Aliases, label) ||
			textutil.NormalizeLabelKey(existing.Label) == normalizeKey {
			return &existing, nil
		}
	}

	// L2: merge embedding comparison (Go-side cosine for 2560-dim vectors — pgvector HNSW cannot index >2000 dim)
	mergePgVector, mergeVector, err := s.embedder(ctx, label, AuxiliaryLabelEmbeddingModeMerge)
	if err != nil {
		return nil, err
	}

	bestMatch, err := s.mergeMatcher(ctx, s.db, labels, mergePgVector, mergeVector)
	if err != nil {
		return nil, err
	}
	if elapsed := time.Since(gateStart); elapsed > 50*time.Millisecond {
		logging.Warnf("auxiliary label dedup gate slow: label=%q took=%s (>50ms), review L1/L2 (embedder/merge) performance", label, elapsed)
	}
	if bestMatch != nil {
		return s.addAlias(ctx, bestMatch, label)
	}

	// L3: create new — storage embedding from label+description, reuse L2 merge embedding
	storageInput := label
	if description != "" {
		storageInput = label + ": " + description
	}
	storagePgVector, _, err := s.embedder(ctx, storageInput, AuxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		return nil, err
	}

	created := models.SemanticLabel{
		Label:          label,
		Slug:           UniqueSemanticLabelSlug(s.db.WithContext(ctx), slug),
		LabelType:      "auxiliary",
		Source:         "llm_extract",
		Status:         "active",
		Embedding:      &storagePgVector,
		MergeEmbedding: &mergePgVector,
	}
	if description != "" {
		created.Description = description
	}
	if err := s.db.WithContext(ctx).Create(&created).Error; err != nil {
		return nil, err
	}
	s.invalidateOnCreate()
	return &created, nil
}

func (s *AuxiliaryLabelService) DisableAuxiliaryLabel(ctx context.Context, labelID uint) error {
	if labelID == 0 {
		return fmt.Errorf("auxiliary label id is required")
	}

	var label models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", labelID, "auxiliary").First(&label).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&label).Updates(map[string]any{"status": "disabled", "embedding": nil, "merge_embedding": nil}).Error; err != nil {
		return err
	}
	s.invalidateOnDisable()
	return nil
}

func (s *AuxiliaryLabelService) MergeAuxiliaryLabelAlias(ctx context.Context, sourceID uint, targetID uint) error {
	if sourceID == 0 || targetID == 0 {
		return fmt.Errorf("source and target auxiliary label ids are required")
	}
	if sourceID == targetID {
		return fmt.Errorf("source and target auxiliary label ids must be different")
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source models.SemanticLabel
		if err := tx.Where("id = ? AND label_type = ?", sourceID, "auxiliary").First(&source).Error; err != nil {
			return err
		}
		var target models.SemanticLabel
		if err := tx.Where("id = ? AND label_type = ?", targetID, "auxiliary").First(&target).Error; err != nil {
			return err
		}

		for _, alias := range append([]string{source.Label}, source.Aliases...) {
			if !strings.EqualFold(target.Label, alias) && !semanticAliasesContain(target.Aliases, alias) {
				target.Aliases = append(target.Aliases, alias)
			}
		}
		if err := tx.Save(&target).Error; err != nil {
			return err
		}

		var links []tagmodels.TopicTagSemanticLabel
		if err := tx.Where("semantic_label_id = ?", sourceID).Find(&links).Error; err != nil {
			return err
		}
		for _, link := range links {
			migrated := tagmodels.TopicTagSemanticLabel{TopicTagID: link.TopicTagID, SemanticLabelID: targetID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&migrated).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("semantic_label_id = ?", sourceID).Delete(&tagmodels.TopicTagSemanticLabel{}).Error; err != nil {
			return err
		}

		var targetRefCount int64
		if err := tx.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", targetID).Count(&targetRefCount).Error; err != nil {
			return err
		}
		var sourceRefCount int64
		if err := tx.Model(&tagmodels.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", sourceID).Count(&sourceRefCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", targetID).Update("ref_count", int(targetRefCount)).Error; err != nil {
			return err
		}
		return tx.Model(&models.SemanticLabel{}).Where("id = ?", sourceID).Updates(map[string]any{"ref_count": int(sourceRefCount), "status": "disabled", "embedding": nil, "merge_embedding": nil}).Error
	})
}

func (s *AuxiliaryLabelService) RemoveBoardComposition(ctx context.Context, boardID uint, auxiliaryLabelID uint) error {
	if boardID == 0 || auxiliaryLabelID == 0 {
		return fmt.Errorf("board and auxiliary label ids are required")
	}

	var board models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", boardID, "board").First(&board).Error; err != nil {
		return err
	}
	var auxiliary models.SemanticLabel
	if err := s.db.WithContext(ctx).Where("id = ? AND label_type = ?", auxiliaryLabelID, "auxiliary").First(&auxiliary).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("board_id = ? AND auxiliary_label_id = ?", boardID, auxiliaryLabelID).
			Delete(&tagmodels.BoardComposition{}).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`DELETE FROM topic_tag_board_labels
			 WHERE semantic_board_id = ?
			 AND EXISTS (
			   SELECT 1 FROM topic_tag_semantic_labels
			   WHERE topic_tag_semantic_labels.topic_tag_id = topic_tag_board_labels.topic_tag_id
			   AND topic_tag_semantic_labels.semantic_label_id = ?
			 )`, boardID, auxiliaryLabelID,
		).Error; err != nil {
			return err
		}
		return nil
	})
}

// RecountRefs recalculates ref_count for the given auxiliary label IDs by counting
// actual topic_tag_semantic_labels rows. This is self-healing: it corrects any
// accumulated drift in ref_count values.
func (s *AuxiliaryLabelService) RecountRefs(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		var count int64
		if err := s.db.WithContext(ctx).Model(&tagmodels.TopicTagSemanticLabel{}).
			Where("semantic_label_id = ?", id).Count(&count).Error; err != nil {
			return fmt.Errorf("recount refs for semantic_label %d: %w", id, err)
		}
		if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Where("id = ?", id).Update("ref_count", int(count)).Error; err != nil {
			return fmt.Errorf("update ref_count for semantic_label %d: %w", id, err)
		}
	}
	return nil
}

// GC performs garbage collection on auxiliary labels based on the requested mode.
func (s *AuxiliaryLabelService) GC(ctx context.Context, req AuxLabelGCRequest) (*AuxLabelGCResult, error) {
	switch req.Mode {
	case AuxLabelGCModeRecalculate:
		return s.gcRecalculate(ctx)
	case AuxLabelGCModeDryRun, AuxLabelGCModeDisable, AuxLabelGCModeDelete:
		return s.gcCleanup(ctx, req)
	default:
		return nil, fmt.Errorf("invalid mode %q, must be one of: dry_run, disable, delete, recalculate", req.Mode)
	}
}

func (s *AuxiliaryLabelService) gcRecalculate(ctx context.Context) (*AuxLabelGCResult, error) {
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Where("label_type = ?", "auxiliary").
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("query auxiliary labels: %w", err)
	}

	if len(ids) == 0 {
		return &AuxLabelGCResult{TotalCount: 0, CorrectedCount: 0}, nil
	}

	// Collect before-state for diff
	type refRow struct {
		ID       uint
		RefCount int
	}
	var before []refRow
	s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, ref_count").
		Where("id IN ?", ids).
		Find(&before)
	beforeCounts := make(map[uint]int, len(before))
	for _, r := range before {
		beforeCounts[r.ID] = r.RefCount
	}

	if err := s.RecountRefs(ctx, ids); err != nil {
		return nil, fmt.Errorf("recount refs: %w", err)
	}

	// Count how many actually changed
	var after []refRow
	s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, ref_count").
		Where("id IN ?", ids).
		Find(&after)
	corrected := 0
	for _, r := range after {
		if beforeCounts[r.ID] != r.RefCount {
			corrected++
		}
	}

	return &AuxLabelGCResult{TotalCount: len(ids), CorrectedCount: corrected}, nil
}

func (s *AuxiliaryLabelService) gcCleanup(ctx context.Context, req AuxLabelGCRequest) (*AuxLabelGCResult, error) {
	if req.GraceDays <= 0 {
		req.GraceDays = 1
	}

	cutoff := time.Now().AddDate(0, 0, -req.GraceDays)

	// Find eligible labels: active, unprotected, past grace period,
	// no topic_tag_semantic_labels AND not mounted on any board
	var eligible []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ? AND protected = false", "auxiliary", "active").
		Where("created_at < ?", cutoff).
		Where("id NOT IN (SELECT DISTINCT semantic_label_id FROM topic_tag_semantic_labels)").
		Where("id NOT IN (SELECT DISTINCT auxiliary_label_id FROM board_composition)").
		Find(&eligible).Error
	if err != nil {
		return nil, fmt.Errorf("query eligible labels: %w", err)
	}

	result := &AuxLabelGCResult{EligibleCount: len(eligible)}

	// Preview (first 20)
	previewLimit := 20
	if len(eligible) < previewLimit {
		previewLimit = len(eligible)
	}
	result.Preview = make([]AuxLabelGCPreviewItem, previewLimit)
	for i := 0; i < previewLimit; i++ {
		result.Preview[i] = AuxLabelGCPreviewItem{
			ID:        eligible[i].ID,
			Label:     eligible[i].Label,
			RefCount:  eligible[i].RefCount,
			CreatedAt: eligible[i].CreatedAt.Format(time.RFC3339),
		}
	}

	if req.Mode == AuxLabelGCModeDryRun || len(eligible) == 0 {
		return result, nil
	}

	ids := make([]uint, len(eligible))
	for i, l := range eligible {
		ids[i] = l.ID
	}

	if req.Mode == AuxLabelGCModeDisable {
		// Delete board_composition rows referencing these labels
		if err := s.db.WithContext(ctx).Where("auxiliary_label_id IN ?", ids).
			Delete(&tagmodels.BoardComposition{}).Error; err != nil {
			return nil, fmt.Errorf("delete board_composition: %w", err)
		}
		// Soft-delete: update status to disabled (vectors dropped — re-enable regenerates)
		if err := s.db.WithContext(ctx).Model(&models.SemanticLabel{}).
			Where("id IN ?", ids).
			Updates(map[string]any{"status": "disabled", "embedding": nil, "merge_embedding": nil}).Error; err != nil {
			return nil, fmt.Errorf("disable labels: %w", err)
		}
		result.AffectedCount = len(ids)
	} else { // delete
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).
			Delete(&models.SemanticLabel{}).Error; err != nil {
			return nil, fmt.Errorf("delete labels: %w", err)
		}
		result.AffectedCount = len(ids)
	}

	return result, nil
}

func (s *AuxiliaryLabelService) loadActiveAuxiliaryLabels(ctx context.Context) ([]models.SemanticLabel, error) {
	s.cache.mu.RLock()
	if s.cache.activeLabels != nil && time.Since(s.cache.activeLabelsAt) < auxActiveLabelsTTL {
		labels := s.cache.activeLabels
		s.cache.mu.RUnlock()
		return labels, nil
	}
	s.cache.mu.RUnlock()

	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	// Double-check after acquiring write lock
	if s.cache.activeLabels != nil && time.Since(s.cache.activeLabelsAt) < auxActiveLabelsTTL {
		return s.cache.activeLabels, nil
	}

	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Select("id, label, slug, label_type, aliases, ref_count, description, status, protected, source, display_order, created_at, updated_at").
		Where("label_type = ? AND status = ?", "auxiliary", "active").
		Find(&labels).Error
	if err != nil {
		return nil, err
	}
	s.cache.activeLabels = labels
	s.cache.activeLabelsAt = time.Now()
	return labels, nil
}

func (s *AuxiliaryLabelService) invalidateOnCreate() {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.activeLabels = nil
	s.cache.activeLabelsAt = time.Time{}
	s.cache.mergeEmbeddings = nil
	s.cache.mergeEmbeddingsAt = time.Time{}
}

func (s *AuxiliaryLabelService) invalidateOnDisable() {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.activeLabels = nil
	s.cache.activeLabelsAt = time.Time{}
	s.cache.mergeEmbeddings = nil
	s.cache.mergeEmbeddingsAt = time.Time{}
}

func (s *AuxiliaryLabelService) invalidateOnAliasChange() {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.activeLabels = nil
	s.cache.activeLabelsAt = time.Time{}
	// mergeEmbeddings preserved — vectors unchanged by alias changes
}

// loadAuxLabelDedupeSim reads the configurable L2 merge similarity threshold
// from ai_settings (key auxiliary_label_dedupe_sim). Falls back to
// defaultAuxLabelDedupeSim (0.95) when the row is missing or the value is not a
// valid float in (0,1]. Called once per L2 evaluation (only when L1 misses — the
// new-entity path), so a single DB read is acceptable (design D5).
func loadAuxLabelDedupeSim(db *gorm.DB) float64 {
	var setting models.AISettings
	if err := db.Where("key = ?", "auxiliary_label_dedupe_sim").First(&setting).Error; err != nil {
		return defaultAuxLabelDedupeSim
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(setting.Value), 64)
	if err != nil || v <= 0 || v > 1 {
		return defaultAuxLabelDedupeSim
	}
	return v
}

// sqlMergeMatcher loads merge embeddings from cache and computes cosine
// similarity in Go. Eliminates the 216 MB DB transfer on every call.
func sqlMergeMatcher(ctx context.Context, db *gorm.DB, labels []models.SemanticLabel, _ string, mergeVector []float64) (*models.SemanticLabel, error) {
	// Use package-level cache
	embeddings, err := packageAuxLabelCacheGetOrLoad(ctx, db)
	if err != nil {
		return nil, err
	}
	threshold := loadAuxLabelDedupeSim(db)

	var best *models.SemanticLabel
	for i := range labels {
		existingVec, ok := embeddings[labels[i].ID]
		if !ok {
			continue
		}
		sim, err := airouter.CosineSimilarity(mergeVector, existingVec)
		if err != nil || sim < threshold {
			continue
		}
		candidate := labels[i]
		if best == nil || candidate.RefCount > best.RefCount || (candidate.RefCount == best.RefCount && candidate.ID < best.ID) {
			best = &candidate
		}
	}
	return best, nil
}

// packageAuxLabelCacheGetOrLoad loads merge embeddings using the package-level cache.
// This is a standalone helper since sqlMergeMatcher is a package-level function (not a method).
func packageAuxLabelCacheGetOrLoad(ctx context.Context, db *gorm.DB) (map[uint][]float64, error) {
	packageAuxLabelCache.mu.RLock()
	if packageAuxLabelCache.mergeEmbeddings != nil && time.Since(packageAuxLabelCache.mergeEmbeddingsAt) < auxMergeEmbeddingsTTL {
		emb := packageAuxLabelCache.mergeEmbeddings
		packageAuxLabelCache.mu.RUnlock()
		return emb, nil
	}
	packageAuxLabelCache.mu.RUnlock()

	packageAuxLabelCache.mu.Lock()
	defer packageAuxLabelCache.mu.Unlock()
	if packageAuxLabelCache.mergeEmbeddings != nil && time.Since(packageAuxLabelCache.mergeEmbeddingsAt) < auxMergeEmbeddingsTTL {
		return packageAuxLabelCache.mergeEmbeddings, nil
	}

	type row struct {
		ID             uint
		MergeEmbedding *string
	}
	var rows []row
	if err := db.WithContext(ctx).Model(&models.SemanticLabel{}).
		Select("id, merge_embedding").
		Where("label_type = ? AND status = ? AND merge_embedding IS NOT NULL", "auxiliary", "active").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uint][]float64, len(rows))
	for _, r := range rows {
		if r.MergeEmbedding == nil || *r.MergeEmbedding == "" {
			continue
		}
		vec, err := ParsePgVector(*r.MergeEmbedding)
		if err != nil {
			continue
		}
		result[r.ID] = vec
	}
	packageAuxLabelCache.mergeEmbeddings = result
	packageAuxLabelCache.mergeEmbeddingsAt = time.Now()
	return result, nil
}

func (s *AuxiliaryLabelService) addAlias(ctx context.Context, label *models.SemanticLabel, alias string) (*models.SemanticLabel, error) {
	if !semanticAliasesContain(label.Aliases, alias) && !strings.EqualFold(label.Label, alias) {
		label.Aliases = append(label.Aliases, alias)
		if err := s.db.WithContext(ctx).Save(label).Error; err != nil {
			return nil, err
		}
		s.invalidateOnAliasChange()
	}
	return label, nil
}

func semanticAliasesContain(aliases []string, label string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(label)) || core.Slugify(alias) == core.Slugify(label) {
			return true
		}
	}
	return false
}

func DefaultAuxiliaryLabelEmbedder(ctx context.Context, input string, mode AuxiliaryLabelEmbeddingMode) (string, []float64, error) {
	opName := "auxiliary_label_storage_embedding"
	if mode == AuxiliaryLabelEmbeddingModeMerge {
		opName = "auxiliary_label_merge_embedding"
	}
	router := airouter.NewRouter()
	result, err := router.Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{input},
		Operation: "tagmanagement.auxlabel_embedding",
		Metadata: map[string]any{
			"operation": opName,
			"label":     input,
		},
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return "", nil, err
	}
	if result == nil || len(result.Embeddings) == 0 {
		return "", nil, fmt.Errorf("empty embedding result")
	}
	vector := result.Embeddings[0]
	return core.FloatsToPgVector(vector), vector, nil
}

func ParsePgVector(value string) ([]float64, error) {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if value == "" {
		return nil, fmt.Errorf("empty vector")
	}
	parts := strings.Split(value, ",")
	result := make([]float64, 0, len(parts))
	for _, part := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, nil
}

func UniqueSemanticLabelSlug(db *gorm.DB, base string) string {
	slug := base
	for i := 2; ; i++ {
		var count int64
		db.Model(&models.SemanticLabel{}).Where("slug = ?", slug).Count(&count)
		if count == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// ensureSemanticLabelVectorColumnDim checks if a vector column on semantic_labels
// matches the required dimension and alters it if not. column is a hardcoded column
// name ("embedding" / "merge_embedding"), never user input, so it is safe to splice
// into SQL.
//
// Should only be called at startup; DDL operations use a 5s lock timeout to avoid
// blocking if other connections hold table locks.
//
// On ALTER failure it diagnoses whether stale data is blocking the resize and returns
// an actionable error: pgvector cannot truncate existing vectors, so the caller must
// clear the rows before the dimension can change.
func ensureSemanticLabelVectorColumnDim(column string, dim int) error {
	db := repository.Repo.DB()
	if err := db.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
		logging.Warnf("Failed to set lock_timeout: %v", err)
	}

	var typeStr string
	if err := db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'semantic_labels' AND a.attname = ?`, column,
	).Row().Scan(&typeStr); err != nil {
		return nil // column may not exist yet, let migration handle it
	}

	expected := fmt.Sprintf("vector(%d)", dim)
	if typeStr == expected {
		return nil
	}

	logging.Infof("Altering semantic_labels.%s column from %s to %s", column, typeStr, expected)

	_ = db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS idx_semantic_labels_%s", column)).Error

	if err := db.Exec(fmt.Sprintf(
		"ALTER TABLE semantic_labels ALTER COLUMN %s TYPE %s", column, expected,
	)).Error; err != nil {
		var staleRows int64
		_ = db.Raw(fmt.Sprintf(
			"SELECT count(*) FROM semantic_labels WHERE %s IS NOT NULL", column,
		)).Scan(&staleRows).Error
		if staleRows > 0 {
			return fmt.Errorf(
				"alter semantic_labels.%s from %s to %s failed: %d row(s) contain stale vectors of the old dimension; "+
					"clear them (e.g. UPDATE semantic_labels SET %s = NULL) then restart: %w",
				column, typeStr, expected, staleRows, column, err,
			)
		}
		return fmt.Errorf("alter semantic_labels.%s column to %s: %w", column, expected, err)
	}

	return nil
}

// EnsureSemanticLabelVectorDimension checks if the semantic_labels.embedding column
// matches the required dimension and alters it if not.
func EnsureSemanticLabelVectorDimension(dim int) error {
	return ensureSemanticLabelVectorColumnDim("embedding", dim)
}

// EnsureSemanticLabelMergeVectorDimension checks if the semantic_labels.merge_embedding
// column matches the required dimension and alters it if not.
func EnsureSemanticLabelMergeVectorDimension(dim int) error {
	return ensureSemanticLabelVectorColumnDim("merge_embedding", dim)
}
