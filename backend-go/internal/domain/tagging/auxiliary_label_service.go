package tagging

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const auxiliaryLabelMergeThreshold = 0.95

var ensureVectorDimOnce sync.Once

type auxiliaryLabelEmbeddingMode string

const (
	auxiliaryLabelEmbeddingModeMerge   auxiliaryLabelEmbeddingMode = "merge"
	auxiliaryLabelEmbeddingModeStorage auxiliaryLabelEmbeddingMode = "storage"
)

type auxiliaryLabelEmbedder func(ctx context.Context, input string, mode auxiliaryLabelEmbeddingMode) (string, []float64, error)

type AuxiliaryLabelService struct {
	db       *gorm.DB
	embedder auxiliaryLabelEmbedder
}

func NewAuxiliaryLabelService(db *gorm.DB, embedder auxiliaryLabelEmbedder) *AuxiliaryLabelService {
	if db == nil {
		db = database.DB
	}
	if embedder == nil {
		embedder = defaultAuxiliaryLabelEmbedder
	}
	return &AuxiliaryLabelService{db: db, embedder: embedder}
}

// EnsureVectorDimensionOnce ensures the semantic_labels.embedding and merge_embedding
// column dimensions match the embedder output. Called once on first label creation.
// Uses the global DB to avoid calling DDL inside a transaction.
func EnsureVectorDimensionOnce(ctx context.Context) {
	ensureVectorDimOnce.Do(func() {
		_, vector, err := defaultAuxiliaryLabelEmbedder(ctx, "dimension-check", auxiliaryLabelEmbeddingModeStorage)
		if err != nil {
			logging.Warnf("Failed to determine embedding dimension: %v", err)
			return
		}
		dim := len(vector)
		if err := EnsureSemanticLabelVectorDimension(dim); err != nil {
			logging.Warnf("Failed to ensure embedding vector dimension: %v", err)
		}
		if err := EnsureSemanticLabelMergeVectorDimension(dim); err != nil {
			logging.Warnf("Failed to ensure merge_embedding vector dimension: %v", err)
		}
	})
}

func (s *AuxiliaryLabelService) AttachAuxiliaryLabels(ctx context.Context, topicTagID uint, labels []AuxiliaryLabel) error {
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
			link := models.TopicTagSemanticLabel{TopicTagID: topicTagID, SemanticLabelID: label.ID}
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
	if _, generic := genericAuxiliaryLabels[label]; generic {
		return nil, fmt.Errorf("auxiliary label %q is too generic", label)
	}
	slug := Slugify(label)
	if slug == "" {
		return nil, fmt.Errorf("auxiliary label slug is empty")
	}

	description = strings.TrimSpace(description)

	labels, err := s.loadActiveAuxiliaryLabels(ctx)
	if err != nil {
		return nil, err
	}

	// L1: exact match by slug or alias
	for _, existing := range labels {
		if existing.Slug == slug || semanticAliasesContain(existing.Aliases, label) {
			return &existing, nil
		}
	}

	// L2: merge embedding comparison using label-only embedding vs MergeEmbedding
	mergePgVector, mergeVector, err := s.embedder(ctx, label, auxiliaryLabelEmbeddingModeMerge)
	if err != nil {
		return nil, err
	}
	var bestMatch *models.SemanticLabel
	for _, existing := range labels {
		if existing.MergeEmbedding == nil || *existing.MergeEmbedding == "" {
			continue
		}
		existingVec, err := parsePgVector(*existing.MergeEmbedding)
		if err != nil {
			continue
		}
		sim, err := airouter.CosineSimilarity(mergeVector, existingVec)
		if err == nil && sim >= auxiliaryLabelMergeThreshold {
			candidate := existing
			if bestMatch == nil || candidate.RefCount > bestMatch.RefCount || (candidate.RefCount == bestMatch.RefCount && candidate.ID < bestMatch.ID) {
				bestMatch = &candidate
			}
		}
	}
	if bestMatch != nil {
		return s.addAlias(ctx, bestMatch, label)
	}

	// L3: create new — storage embedding from label+description, reuse L2 merge embedding
	storageInput := label
	if description != "" {
		storageInput = label + ": " + description
	}
	storagePgVector, _, err := s.embedder(ctx, storageInput, auxiliaryLabelEmbeddingModeStorage)
	if err != nil {
		return nil, err
	}

	created := models.SemanticLabel{
		Label:          label,
		Slug:           uniqueSemanticLabelSlug(s.db.WithContext(ctx), slug),
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
	return s.db.WithContext(ctx).Model(&label).Update("status", "disabled").Error
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

		var links []models.TopicTagSemanticLabel
		if err := tx.Where("semantic_label_id = ?", sourceID).Find(&links).Error; err != nil {
			return err
		}
		for _, link := range links {
			migrated := models.TopicTagSemanticLabel{TopicTagID: link.TopicTagID, SemanticLabelID: targetID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&migrated).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("semantic_label_id = ?", sourceID).Delete(&models.TopicTagSemanticLabel{}).Error; err != nil {
			return err
		}

		var targetRefCount int64
		if err := tx.Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", targetID).Count(&targetRefCount).Error; err != nil {
			return err
		}
		var sourceRefCount int64
		if err := tx.Model(&models.TopicTagSemanticLabel{}).Where("semantic_label_id = ?", sourceID).Count(&sourceRefCount).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.SemanticLabel{}).Where("id = ?", targetID).Update("ref_count", int(targetRefCount)).Error; err != nil {
			return err
		}
		return tx.Model(&models.SemanticLabel{}).Where("id = ?", sourceID).Updates(map[string]any{"ref_count": int(sourceRefCount), "status": "disabled"}).Error
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
	return s.db.WithContext(ctx).Where("board_id = ? AND auxiliary_label_id = ?", boardID, auxiliaryLabelID).Delete(&models.BoardComposition{}).Error
}

func (s *AuxiliaryLabelService) loadActiveAuxiliaryLabels(ctx context.Context) ([]models.SemanticLabel, error) {
	var labels []models.SemanticLabel
	err := s.db.WithContext(ctx).
		Where("label_type = ? AND status = ?", "auxiliary", "active").
		Find(&labels).Error
	return labels, err
}

func (s *AuxiliaryLabelService) addAlias(ctx context.Context, label *models.SemanticLabel, alias string) (*models.SemanticLabel, error) {
	if !semanticAliasesContain(label.Aliases, alias) && !strings.EqualFold(label.Label, alias) {
		label.Aliases = append(label.Aliases, alias)
		if err := s.db.WithContext(ctx).Save(label).Error; err != nil {
			return nil, err
		}
	}
	return label, nil
}

func semanticAliasesContain(aliases []string, label string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(label)) || Slugify(alias) == Slugify(label) {
			return true
		}
	}
	return false
}

func defaultAuxiliaryLabelEmbedder(ctx context.Context, input string, mode auxiliaryLabelEmbeddingMode) (string, []float64, error) {
	opName := "auxiliary_label_storage_embedding"
	if mode == auxiliaryLabelEmbeddingModeMerge {
		opName = "auxiliary_label_merge_embedding"
	}
	router := airouter.NewRouter()
	result, err := router.Embed(ctx, airouter.EmbeddingRequest{
		Input: []string{input},
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
	return floatsToPgVector(vector), vector, nil
}

func parsePgVector(value string) ([]float64, error) {
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

func uniqueSemanticLabelSlug(db *gorm.DB, base string) string {
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

// EnsureSemanticLabelVectorDimension checks if the semantic_labels.embedding column
// matches the required dimension and alters it (plus recreates the index) if not.
// For dimensions > 2000, skips HNSW index (HNSW limit is 2000).
// Should only be called at startup; DDL operations use a 5s lock timeout to avoid
// blocking if other connections hold table locks.
func EnsureSemanticLabelVectorDimension(dim int) error {
	// Set lock timeout to prevent infinite blocking on DDL
	if err := database.DB.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
		logging.Warnf("Failed to set lock_timeout: %v", err)
	}

	var typeStr string
	if err := database.DB.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'semantic_labels' AND a.attname = 'embedding'
	`).Row().Scan(&typeStr); err != nil {
		return nil // column may not exist yet, let migration handle it
	}

	expected := fmt.Sprintf("vector(%d)", dim)
	if typeStr == expected {
		return nil
	}

	logging.Infof("Altering semantic_labels.embedding column from %s to %s", typeStr, expected)

	// Drop index first — it depends on the column type
	_ = database.DB.Exec("DROP INDEX IF EXISTS idx_semantic_labels_embedding").Error

	if err := database.DB.Exec(fmt.Sprintf(
		"ALTER TABLE semantic_labels ALTER COLUMN embedding TYPE %s", expected,
	)).Error; err != nil {
		return fmt.Errorf("alter semantic_labels.embedding column to %s: %w", expected, err)
	}

	// Recreate index — HNSW supports max 2000 dimensions
	if dim <= 2000 {
		if err := database.DB.Exec(
			"CREATE INDEX idx_semantic_labels_embedding ON semantic_labels USING hnsw (embedding vector_cosine_ops)",
		).Error; err != nil {
			logging.Warnf("Failed to create HNSW index on semantic_labels.embedding: %v", err)
		}
	} else {
		logging.Infof("Dimension %d exceeds HNSW limit (2000), skipping vector index on semantic_labels", dim)
	}

	return nil
}

// EnsureSemanticLabelMergeVectorDimension checks if the semantic_labels.merge_embedding
// column matches the required dimension and alters it if not. Same HNSW limit as embedding.
func EnsureSemanticLabelMergeVectorDimension(dim int) error {
	if err := database.DB.Exec("SET LOCAL lock_timeout = '5s'").Error; err != nil {
		logging.Warnf("Failed to set lock_timeout: %v", err)
	}

	var typeStr string
	if err := database.DB.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'semantic_labels' AND a.attname = 'merge_embedding'
	`).Row().Scan(&typeStr); err != nil {
		return nil // column may not exist yet, let migration handle it
	}

	expected := fmt.Sprintf("vector(%d)", dim)
	if typeStr == expected {
		return nil
	}

	logging.Infof("Altering semantic_labels.merge_embedding column from %s to %s", typeStr, expected)

	_ = database.DB.Exec("DROP INDEX IF EXISTS idx_semantic_labels_merge_embedding").Error

	if err := database.DB.Exec(fmt.Sprintf(
		"ALTER TABLE semantic_labels ALTER COLUMN merge_embedding TYPE %s", expected,
	)).Error; err != nil {
		return fmt.Errorf("alter semantic_labels.merge_embedding column to %s: %w", expected, err)
	}

	if dim <= 2000 {
		if err := database.DB.Exec(
			"CREATE INDEX idx_semantic_labels_merge_embedding ON semantic_labels USING hnsw (merge_embedding vector_cosine_ops)",
		).Error; err != nil {
			logging.Warnf("Failed to create HNSW index on semantic_labels.merge_embedding: %v", err)
		}
	} else {
		logging.Infof("Dimension %d exceeds HNSW limit (2000), skipping vector index on semantic_labels.merge_embedding", dim)
	}

	return nil
}
