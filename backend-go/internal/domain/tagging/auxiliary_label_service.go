package tagging

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const auxiliaryLabelMergeThreshold = 0.95

type auxiliaryLabelEmbedder func(ctx context.Context, label string) (string, []float64, error)

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

func (s *AuxiliaryLabelService) AttachAuxiliaryLabels(ctx context.Context, topicTagID uint, labels []string) error {
	if topicTagID == 0 || len(labels) == 0 {
		return nil
	}
	normalized, err := normalizeAuxiliaryLabels(labels)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txService := NewAuxiliaryLabelService(tx, s.embedder)
		for _, raw := range normalized {
			label, err := txService.ResolveAuxiliaryLabel(ctx, raw)
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

func (s *AuxiliaryLabelService) ResolveAuxiliaryLabel(ctx context.Context, rawLabel string) (*models.SemanticLabel, error) {
	label := strings.TrimSpace(rawLabel)
	if err := validateAuxiliaryLabels([]string{label, label + "-context", label + "-anchor"}); err != nil {
		return nil, err
	}
	slug := Slugify(label)
	if slug == "" {
		return nil, fmt.Errorf("auxiliary label slug is empty")
	}

	labels, err := s.loadActiveAuxiliaryLabels(ctx)
	if err != nil {
		return nil, err
	}
	for _, existing := range labels {
		if existing.Slug == slug || semanticAliasesContain(existing.Aliases, label) {
			return &existing, nil
		}
	}

	pgVector, vector, err := s.embedder(ctx, label)
	if err != nil {
		return nil, err
	}
	var bestMatch *models.SemanticLabel
	for _, existing := range labels {
		if existing.Embedding == nil || *existing.Embedding == "" {
			continue
		}
		existingVec, err := parsePgVector(*existing.Embedding)
		if err != nil {
			continue
		}
		sim, err := airouter.CosineSimilarity(vector, existingVec)
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

	created := models.SemanticLabel{
		Label:     label,
		Slug:      uniqueSemanticLabelSlug(s.db.WithContext(ctx), slug),
		LabelType: "auxiliary",
		Source:    "llm_extract",
		Status:    "active",
		Embedding: &pgVector,
	}
	if err := s.db.WithContext(ctx).Create(&created).Error; err != nil {
		return nil, err
	}
	return &created, nil
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

func validateAuxiliaryLabels(labels []string) error {
	if len(labels) < 3 || len(labels) > 5 {
		return fmt.Errorf("auxiliary labels must contain 3-5 labels")
	}
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			return fmt.Errorf("auxiliary label must not be empty")
		}
		if _, generic := genericAuxiliaryLabels[label]; generic {
			return fmt.Errorf("auxiliary label %q is too generic", label)
		}
	}
	return nil
}

func defaultAuxiliaryLabelEmbedder(ctx context.Context, label string) (string, []float64, error) {
	router := airouter.NewRouter()
	result, err := router.Embed(ctx, airouter.EmbeddingRequest{
		Input: []string{label},
		Metadata: map[string]any{
			"operation": "auxiliary_label_embedding",
			"label":     label,
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

func semanticAliasesContain(aliases []string, label string) bool {
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias), strings.TrimSpace(label)) || Slugify(alias) == Slugify(label) {
			return true
		}
	}
	return false
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
