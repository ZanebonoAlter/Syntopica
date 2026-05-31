package tagging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"

	"gorm.io/gorm"
)

const (
	EmbeddingTypeIdentity     = "identity"
	EmbeddingTypeSemantic     = "semantic"
	EmbeddingTypeEventKeyword = "event_keyword"
)

var (
	ErrNoEmbeddingProvider = errors.New("no embedding provider configured")
	ErrEmbeddingFailed     = errors.New("failed to generate embedding")
	ErrTopicTagNotFound    = errors.New("topic tag not found")
)

// EmbeddingMatchThresholds defines similarity thresholds for tag matching
type EmbeddingMatchThresholds struct {
	// High similarity - include as candidate (no longer auto-reuse)
	HighSimilarity float64
	// Low similarity - auto-create new tag
	LowSimilarity float64
	// Middle band - requires AI judgment
	// Tags with similarity between LowSimilarity and HighSimilarity need AI decision
}

// DefaultThresholds provides sensible defaults for matching
var DefaultThresholds = EmbeddingMatchThresholds{
	HighSimilarity: 0.97, // Auto-reuse if similarity >= 0.97
	LowSimilarity:  0.78, // Auto-create if similarity < 0.78
}

// CategoryThresholdOverrides defines per-category threshold adjustments.
// Keys are category names; the corresponding HighSimilarity overrides the default
// when TagMatch processes a tag of that category.
var CategoryThresholdOverrides = map[string]EmbeddingMatchThresholds{}

// ThresholdsForCategory returns the effective thresholds for a given category,
// falling back to DefaultThresholds when no override is configured.
func ThresholdsForCategory(category string) EmbeddingMatchThresholds {
	if override, ok := CategoryThresholdOverrides[category]; ok {
		return override
	}
	return DefaultThresholds
}

// TagMatchResult represents a tag match result
type TagMatchResult struct {
	MatchType   string // "exact", "candidates", "no_match"
	ExistingTag *models.TopicTag
	Similarity  float64
	Candidates  []TagCandidate
}

// TagCandidate represents a candidate tag for AI judgment
type TagCandidate struct {
	Tag        *models.TopicTag
	Similarity float64
	DateRange  string
}

// EmbeddingService handles embedding generation and similarity matching
type EmbeddingService struct {
	router     *airouter.Router
	thresholds EmbeddingMatchThresholds
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService() *EmbeddingService {
	thresholds := DefaultThresholds
	configService := NewEmbeddingConfigService()
	if loaded, err := configService.LoadThresholds(); err == nil {
		thresholds = loaded
	}

	return &EmbeddingService{
		router:     airouter.NewRouter(),
		thresholds: thresholds,
	}
}

// NewEmbeddingServiceWithThresholds creates a service with custom thresholds
func NewEmbeddingServiceWithThresholds(thresholds EmbeddingMatchThresholds) *EmbeddingService {
	return &EmbeddingService{
		router:     airouter.NewRouter(),
		thresholds: thresholds,
	}
}

// GetThresholds returns the configured match thresholds for this service.
func (s *EmbeddingService) GetThresholds() EmbeddingMatchThresholds {
	return s.thresholds
}

// GenerateEmbedding generates an embedding for a tag's text representation
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, tag *models.TopicTag, embeddingType string, opts ...EmbeddingTextOptions) (*models.TopicTagEmbedding, error) {
	text := buildTagEmbeddingText(tag, embeddingType, opts...)
	textHash := hashText(embeddingType + "\n" + text)

	// Use router with failover to generate embedding
	req := airouter.EmbeddingRequest{
		Input: []string{text},
		Metadata: map[string]any{
			"tag_id":    tag.ID,
			"tag_label": tag.Label,
			"category":  tag.Category,
		},
	}
	result, err := s.router.Embed(ctx, req, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, ErrEmbeddingFailed
	}

	// Store embedding as JSON (legacy) and as pgvector format
	vectorJSON, err := json.Marshal(result.Embeddings[0])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding: %w", err)
	}

	// Build pgvector format string: [0.1,0.2,0.3,...]
	pgVecStr := floatsToPgVector(result.Embeddings[0])

	embedding := &models.TopicTagEmbedding{
		TopicTagID:    tag.ID,
		EmbeddingType: embeddingType,
		Vector:        string(vectorJSON),
		EmbeddingVec:  pgVecStr,
		Dimension:     result.Dimensions,
		Model:         result.Model,
		TextHash:      textHash,
	}

	return embedding, nil
}

func (s *EmbeddingService) GenerateEmbeddingForText(ctx context.Context, tagID uint, embeddingType string, text string) (*models.TopicTagEmbedding, error) {
	textHash := hashText(embeddingType + "\n" + text)

	req := airouter.EmbeddingRequest{
		Input:    []string{text},
		Metadata: map[string]any{"tag_id": tagID},
	}
	result, err := s.router.Embed(ctx, req, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, ErrEmbeddingFailed
	}

	vectorJSON, err := json.Marshal(result.Embeddings[0])
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding: %w", err)
	}

	pgVecStr := floatsToPgVector(result.Embeddings[0])

	embedding := &models.TopicTagEmbedding{
		TopicTagID:    tagID,
		EmbeddingType: embeddingType,
		Vector:        string(vectorJSON),
		EmbeddingVec:  pgVecStr,
		Dimension:     result.Dimensions,
		Model:         result.Model,
		TextHash:      textHash,
	}

	return embedding, nil
}

func getEventKeywords(tag *models.TopicTag) []string {
	if tag.Metadata == nil {
		return nil
	}
	raw, ok := tag.Metadata["event_keywords"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

func (s *EmbeddingService) FindSimilarTags(ctx context.Context, tag *models.TopicTag, category string, limit int, embeddingType string) ([]TagCandidate, error) {
	embedding, err := s.GenerateEmbedding(ctx, tag, embeddingType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	// Build pgvector format for the query vector
	var vector []float64
	if err := json.Unmarshal([]byte(embedding.Vector), &vector); err != nil {
		return nil, fmt.Errorf("failed to parse embedding vector: %w", err)
	}
	pgVecStr := floatsToPgVector(vector)

	// Use pgvector SQL cosine distance (<=>) for similarity search
	// Filter out merged tags (only match active tags)
	type simRow struct {
		TagID    uint    `gorm:"column:tag_id"`
		Distance float64 `gorm:"column:distance"`
	}
	var rows []simRow
	query := `
		SELECT t.id AS tag_id, e.embedding <=> ?::vector AS distance
		FROM topic_tag_embeddings e
		JOIN topic_tags t ON t.id = e.topic_tag_id
		WHERE t.category = ?
		  AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
		  AND e.embedding IS NOT NULL
		  AND e.embedding_type = ?
		ORDER BY e.embedding <=> ?::vector
		LIMIT ?
	`
	if err := database.DB.Raw(query, pgVecStr, category, embeddingType, pgVecStr, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query similar tags: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Load tags and compute similarity (1 - distance)
	tagIDs := make([]uint, len(rows))
	for i, r := range rows {
		tagIDs[i] = r.TagID
	}
	var tags []models.TopicTag
	if err := database.DB.Where("id IN ?", tagIDs).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to load tags: %w", err)
	}
	tagMap := make(map[uint]*models.TopicTag, len(tags))
	for i := range tags {
		tagMap[tags[i].ID] = &tags[i]
	}

	candidates := make([]TagCandidate, 0, len(rows))
	for _, r := range rows {
		t, ok := tagMap[r.TagID]
		if !ok {
			continue
		}
		similarity := 1.0 - r.Distance
		candidates = append(candidates, TagCandidate{
			Tag:        t,
			Similarity: similarity,
		})
	}

	return candidates, nil
}

// TagMatch decides how to handle a candidate tag
func (s *EmbeddingService) TagMatch(ctx context.Context, label, category string, aliases string) (*TagMatchResult, error) {
	slug := Slugify(label)
	thresholds := ThresholdsForCategory(category)
	logging.Infof("TagMatch: start label=%q slug=%q category=%s low=%.2f high=%.2f", label, slug, category, thresholds.LowSimilarity, thresholds.HighSimilarity)
	var existingTag models.TopicTag
	err := database.DB.Scopes(activeTagFilter).Where("slug = ? AND category = ?", slug, category).First(&existingTag).Error
	if err == nil {
		logging.Infof("TagMatch: label=%q category=%s result=exact reason=slug existingID=%d existingLabel=%q", label, category, existingTag.ID, existingTag.Label)
		return &TagMatchResult{
			MatchType:   "exact",
			ExistingTag: &existingTag,
			Similarity:  1.0,
		}, nil
	}

	if aliases != "" {
		aliasScanFallback := database.DB.Name() != "postgres"
		if database.DB.Name() == "postgres" {
			var aliasMatch models.TopicTag
			aliasSQL := `category = ? AND aliases IS NOT NULL AND aliases != '' AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(aliases::jsonb) AS alias(value)
				WHERE LOWER(alias.value) = LOWER(?)
			)`
			if err := database.DB.Scopes(activeTagFilter).Where(aliasSQL, category, label).First(&aliasMatch).Error; err == nil {
				logging.Infof("TagMatch: label=%q category=%s result=exact reason=alias existingID=%d existingLabel=%q", label, category, aliasMatch.ID, aliasMatch.Label)
				return &TagMatchResult{
					MatchType:   "exact",
					ExistingTag: &aliasMatch,
					Similarity:  1.0,
				}, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				logging.Warnf("TagMatch: label=%q category=%s alias SQL match failed, falling back to in-memory alias scan: %v", label, category, err)
				aliasScanFallback = true
			}
		}

		if aliasScanFallback {
			var aliasTags []models.TopicTag
			if err := database.DB.Scopes(activeTagFilter).Where("category = ?", category).Find(&aliasTags).Error; err == nil {
				for _, t := range aliasTags {
					if containsAlias(t.Aliases, label) {
						logging.Infof("TagMatch: label=%q category=%s result=exact reason=alias existingID=%d existingLabel=%q", label, category, t.ID, t.Label)
						return &TagMatchResult{
							MatchType:   "exact",
							ExistingTag: &t,
							Similarity:  1.0,
						}, nil
					}
				}
			}
		}
	}

	candidate := &models.TopicTag{
		Label:    label,
		Category: category,
		Aliases:  aliases,
	}

	embType := EmbeddingTypeSemantic
	candidates, err := s.FindSimilarTags(ctx, candidate, category, 20, embType)
	if err != nil {
		logging.Warnf("TagMatch: label=%q category=%s similarity search failed, result=no_match err=%v", label, category, err)
		return &TagMatchResult{
			MatchType: "no_match",
		}, nil
	}

	logging.Infof("TagMatch: label=%q category=%s similarity search returned totalCandidates=%d bestSimilarity=%.4f", label, category, len(candidates), bestSimilarity(candidates))

	var validCandidates []TagCandidate
	for _, c := range candidates {
		if c.Similarity >= thresholds.LowSimilarity {
			validCandidates = append(validCandidates, c)
		}
	}

	if len(validCandidates) == 0 {
		logging.Infof("TagMatch: label=%q category=%s result=no_match reason=below_low_similarity bestSimilarity=%.4f", label, category, bestSimilarity(candidates))
		return &TagMatchResult{
			MatchType:  "no_match",
			Similarity: bestSimilarity(candidates),
		}, nil
	}

	if validCandidates[0].Similarity >= thresholds.HighSimilarity {
		top := validCandidates[0]
		logging.Infof("TagMatch: label=%q category=%s result=candidates reason=high_similarity_downgraded existingID=%d existingLabel=%q similarity=%.4f", label, category, top.Tag.ID, top.Tag.Label, top.Similarity)
		return &TagMatchResult{
			MatchType:  "candidates",
			Similarity: top.Similarity,
			Candidates: validCandidates,
		}, nil
	}

	logging.Infof("TagMatch: label=%q category=%s result=candidates validCandidates=%d topSimilarity=%.4f topLabels=%s", label, category, len(validCandidates), validCandidates[0].Similarity, matchCandidateLabels(validCandidates))

	return &TagMatchResult{
		MatchType:  "candidates",
		Similarity: validCandidates[0].Similarity,
		Candidates: validCandidates,
	}, nil
}

// FindSimilarTagsAmongSet finds pairs of tags within a set whose embedding
// cosine similarity meets the given threshold. Uses pgvector pairwise distance.
func (s *EmbeddingService) FindSimilarTagsAmongSet(ctx context.Context, tagIDs []uint, threshold float64) ([]SimilarityEdge, error) {
	if len(tagIDs) < 2 {
		return nil, nil
	}

	type pairRow struct {
		TagAID   uint    `gorm:"column:tag_a_id"`
		TagBID   uint    `gorm:"column:tag_b_id"`
		Distance float64 `gorm:"column:distance"`
	}
	var rows []pairRow
	query := `
		SELECT a.topic_tag_id AS tag_a_id,
		       b.topic_tag_id AS tag_b_id,
		       a.embedding <=> b.embedding AS distance
		FROM topic_tag_embeddings a
		JOIN topic_tag_embeddings b
		  ON a.topic_tag_id < b.topic_tag_id
		  AND a.embedding_type = b.embedding_type
		WHERE a.embedding_type = 'semantic'
		  AND a.topic_tag_id IN ?
		  AND b.topic_tag_id IN ?
		  AND a.embedding IS NOT NULL
		  AND b.embedding IS NOT NULL
		  AND a.embedding <=> b.embedding < ?
	`
	if err := database.DB.Raw(query, tagIDs, tagIDs, 1.0-threshold).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("pairwise similarity query: %w", err)
	}

	edges := make([]SimilarityEdge, 0, len(rows))
	for _, r := range rows {
		edges = append(edges, SimilarityEdge{
			TagAID:     r.TagAID,
			TagBID:     r.TagBID,
			Similarity: 1.0 - r.Distance,
		})
	}
	return edges, nil
}

func bestSimilarity(candidates []TagCandidate) float64 {
	if len(candidates) == 0 {
		return 0
	}
	return candidates[0].Similarity
}

func matchCandidateLabels(candidates []TagCandidate) string {
	labels := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.Tag == nil {
			continue
		}
		labels = append(labels, c.Tag.Label)
	}
	return strings.Join(labels, ", ")
}

// activeTagFilter returns a GORM scope that filters to only active (non-merged) tags.
// Used by query functions to exclude merged tags from match candidates.
// Includes empty-string status check for rows created before the migration ran.
func activeTagFilter(db *gorm.DB) *gorm.DB {
	return db.Where("status = ? OR status = ? OR status IS NULL", "active", "")
}

type mergeReembeddingEnqueuer interface {
	Enqueue(sourceTagID, targetTagID uint) error
}

var defaultMergeReembeddingQueueFactory = func() mergeReembeddingEnqueuer {
	return NewMergeReembeddingQueueService(nil)
}

var mergeReembeddingQueueFactory = defaultMergeReembeddingQueueFactory

// MergeTags hard-deletes sourceTag after migrating all references to targetTag.
func MergeTags(sourceTagID, targetTagID uint) error {
	if sourceTagID == targetTagID {
		return fmt.Errorf("cannot merge tag into itself (id=%d)", sourceTagID)
	}

	if err := HardMergeTags(database.DB, sourceTagID, targetTagID); err != nil {
		return err
	}

	if err := mergeReembeddingQueueFactory().Enqueue(sourceTagID, targetTagID); err != nil {
		return fmt.Errorf("enqueue merge re-embedding task: %w", err)
	}

	return nil
}

// DeleteTagEmbedding removes the embedding row for a given tag ID.
// Used after establishing parent-child relationships to prevent child tags
// from appearing in future embedding similarity matches.
func DeleteTagEmbedding(tagID uint) error {
	if tagID == 0 {
		return nil
	}
	return database.DB.Where("topic_tag_id = ?", tagID).Delete(&models.TopicTagEmbedding{}).Error
}

// SaveEmbedding saves or updates a tag's embedding in the database.
// If the actual vector dimension differs from the column definition, it alters the column type.
func (s *EmbeddingService) SaveEmbedding(embedding *models.TopicTagEmbedding) error {
	if embedding.Dimension > 0 && embedding.EmbeddingVec != "" {
		if err := ensureVectorDimension(embedding.Dimension); err != nil {
			return fmt.Errorf("ensure vector dimension %d: %w", embedding.Dimension, err)
		}
	}

	var tag models.TopicTag
	if err := database.DB.Select("id").First(&tag, embedding.TopicTagID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTopicTagNotFound
		}
		return fmt.Errorf("load topic tag %d: %w", embedding.TopicTagID, err)
	}

	// Clean up stale embeddings: same tag + type but different text_hash
	database.DB.Where(
		"topic_tag_id = ? AND embedding_type = ? AND text_hash != ?",
		embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash,
	).Delete(&models.TopicTagEmbedding{})

	var existing models.TopicTagEmbedding
	err := database.DB.Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash).First(&existing).Error

	if err == nil {
		embedding.ID = existing.ID
		return database.DB.Save(embedding).Error
	}

	return database.DB.Create(embedding).Error
}

// ensureVectorDimension checks if the embedding column matches the required dimension
// and alters it (plus the index) if not. Drops index before ALTER, recreates after.
// For dimensions > 2000, uses IVFFlat instead of HNSW (HNSW limit is 2000).
func ensureVectorDimension(dim int) error {
	var typeStr string
	if err := database.DB.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'topic_tag_embeddings' AND a.attname = 'embedding'
	`).Row().Scan(&typeStr); err != nil {
		return nil
	}

	expected := fmt.Sprintf("vector(%d)", dim)
	if typeStr == expected {
		return nil
	}

	logging.Infof("Altering embedding column from %s to %s", typeStr, expected)

	// Drop index first — it depends on the column type
	_ = database.DB.Exec("DROP INDEX IF EXISTS idx_topic_tag_embeddings_embedding").Error

	if err := database.DB.Exec(fmt.Sprintf(
		"ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE %s", expected,
	)).Error; err != nil {
		return fmt.Errorf("alter embedding column to %s: %w", expected, err)
	}

	// Recreate index — HNSW supports max 2000 dimensions
	if dim <= 2000 {
		if err := database.DB.Exec(
			"CREATE INDEX idx_topic_tag_embeddings_embedding ON topic_tag_embeddings USING hnsw (embedding vector_cosine_ops)",
		).Error; err != nil {
			logging.Warnf("Failed to recreate HNSW index: %v", err)
		}
	} else {
		logging.Infof("Dimension %d exceeds HNSW limit (2000), skipping vector index", dim)
	}

	return nil
}

// GetEmbedding retrieves the embedding for a tag
func (s *EmbeddingService) GetEmbedding(tagID uint, embeddingType string) (*models.TopicTagEmbedding, error) {
	var embedding models.TopicTagEmbedding
	err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagID, embeddingType).First(&embedding).Error
	if err != nil {
		return nil, err
	}
	return &embedding, nil
}

type EmbeddingTextOptions struct {
	ContextTitles []string
}

// BuildTextForEmbedding creates the text representation for embedding
func buildTagEmbeddingText(tag *models.TopicTag, embeddingType string, opts ...EmbeddingTextOptions) string {
	text := tag.Label

	if embeddingType == EmbeddingTypeSemantic && tag.Description != "" {
		text += ". " + tag.Description
	}

	if tag.Aliases != "" {
		var aliases []string
		if err := json.Unmarshal([]byte(tag.Aliases), &aliases); err == nil {
			for _, alias := range aliases {
				text += " " + alias
			}
		} else {
			text += " " + tag.Aliases
		}
	}

	text += " " + tag.Category

	return text
}

func GetTagContextTitles(tagID uint, limit int) []string {
	var titles []string
	query := `
		SELECT title FROM (
			SELECT DISTINCT a.title, MAX(a.created_at) AS created_at
			FROM article_topic_tags att
			JOIN articles a ON a.id = att.article_id
			WHERE att.topic_tag_id = ?
			GROUP BY a.title
		) sub
		ORDER BY sub.created_at DESC
		LIMIT ?
	`
	database.DB.Raw(query, tagID, limit).Scan(&titles)
	return titles
}

func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// floatsToPgVector converts a float64 slice to pgvector string format: [0.1,0.2,0.3]
func floatsToPgVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func containsAlias(aliasesJSON, label string) bool {
	if aliasesJSON == "" {
		return false
	}

	var aliases []string
	if err := json.Unmarshal([]byte(aliasesJSON), &aliases); err != nil {
		// Try comma-separated
		aliases = splitByComma(aliasesJSON)
	}

	labelLower := lower(label)
	for _, alias := range aliases {
		if lower(alias) == labelLower {
			return true
		}
	}
	return false
}

func splitByComma(s string) []string {
	var result []string
	current := ""
	for _, r := range s {
		if r == ',' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func lower(s string) string {
	return strings.ToLower(s)
}
