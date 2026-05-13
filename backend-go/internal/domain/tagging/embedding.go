package tagging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/airouter"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"

	"gorm.io/gorm"
)

const (
	EmbeddingTypeIdentity = "identity"
	EmbeddingTypeSemantic = "semantic"
)

var (
	ErrNoEmbeddingProvider = errors.New("no embedding provider configured")
	ErrEmbeddingFailed     = errors.New("failed to generate embedding")
	ErrTopicTagNotFound    = errors.New("topic tag not found")
)

// EmbeddingMatchThresholds defines similarity thresholds for tag matching
type EmbeddingMatchThresholds struct {
	// High similarity - auto-reuse existing tag
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
var CategoryThresholdOverrides = map[string]EmbeddingMatchThresholds{
	"keyword": {
		HighSimilarity: 0.90,
		LowSimilarity:  0.78,
	},
}

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

// FindSimilarTags finds existing tags with similar embeddings using pgvector SQL
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
		logging.Infof("TagMatch: label=%q category=%s result=exact reason=high_similarity existingID=%d existingLabel=%q similarity=%.4f", label, category, top.Tag.ID, top.Tag.Label, top.Similarity)
		return &TagMatchResult{
			MatchType:   "exact",
			ExistingTag: top.Tag,
			Similarity:  top.Similarity,
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

// FindSimilarAbstractTags finds existing abstract tags with similar embeddings using pgvector SQL.
// Only considers tags where source = 'abstract'.
// Excludes tags that already have a parent-child relation with tagID (in either direction).
func (s *EmbeddingService) FindSimilarAbstractTags(ctx context.Context, tagID uint, category string, limit int) ([]TagCandidate, error) {
	var existing models.TopicTagEmbedding
	if err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagID, EmbeddingTypeSemantic).First(&existing).Error; err != nil {
		if err2 := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", tagID, EmbeddingTypeIdentity).First(&existing).Error; err2 != nil {
			return nil, fmt.Errorf("no embedding for tag %d: %w", tagID, err)
		}
	}

	pgVecStr := existing.EmbeddingVec

	type simRow struct {
		TagID    uint    `gorm:"column:tag_id"`
		Distance float64 `gorm:"column:distance"`
	}
	var rows []simRow
	args := []interface{}{pgVecStr, tagID, tagID, tagID}
	sqlQuery := `
		SELECT t.id AS tag_id, e.embedding <=> ?::vector AS distance
		FROM topic_tag_embeddings e
		JOIN topic_tags t ON t.id = e.topic_tag_id
		WHERE t.source = 'abstract'
		  AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
		  AND t.id != ?
		  AND e.embedding IS NOT NULL
		  AND e.embedding_type = 'semantic'
		  AND NOT EXISTS (
		    SELECT 1 FROM topic_tag_relations r
		    WHERE (r.parent_id = ? AND r.child_id = t.id)
		       OR (r.child_id = ? AND r.parent_id = t.id)
		  )
	`
	if category != "" {
		sqlQuery += "  AND t.category = ?\n"
		args = append(args, category)
	}
	sqlQuery += `
		ORDER BY e.embedding <=> ?::vector
	`
	args = append(args, pgVecStr)
	if limit > 0 {
		sqlQuery += "	LIMIT ?\n"
		args = append(args, limit)
	}
	if err := database.DB.Raw(sqlQuery, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query similar abstract tags: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	tagIDs := make([]uint, len(rows))
	for i, r := range rows {
		tagIDs[i] = r.TagID
	}
	var tags []models.TopicTag
	if err := database.DB.Where("id IN ?", tagIDs).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("failed to load abstract tags: %w", err)
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
		candidates = append(candidates, TagCandidate{
			Tag:        t,
			Similarity: 1.0 - r.Distance,
		})
	}

	return candidates, nil
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

// MergeTags merges sourceTag into targetTag within a database transaction.
// 1. Update all article_topic_tags from source to target (dedup conflicts)
// 2. Set source tag status='merged', merged_into_id=target.ID
// 3. Delete source tag's embedding (stale after merge)
// 4. Recalculate target tag's feed_count
func MergeTags(sourceTagID, targetTagID uint) error {
	if sourceTagID == targetTagID {
		return fmt.Errorf("cannot merge tag into itself (id=%d)", sourceTagID)
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Step 1: Migrate article_topic_tags references
		// Find all source links first
		var sourceLinks []models.ArticleTopicTag
		if err := tx.Where("topic_tag_id = ?", sourceTagID).Find(&sourceLinks).Error; err != nil {
			return fmt.Errorf("find source article_topic_tags: %w", err)
		}

		for _, link := range sourceLinks {
			// Check if target already has a link for this article
			var existingCount int64
			if err := tx.Model(&models.ArticleTopicTag{}).
				Where("article_id = ? AND topic_tag_id = ?", link.ArticleID, targetTagID).
				Count(&existingCount).Error; err != nil {
				return fmt.Errorf("check existing article_topic_tag for article %d: %w", link.ArticleID, err)
			}

			if existingCount > 0 {
				// Target already covers this article — delete the source link
				if err := tx.Delete(&link).Error; err != nil {
					return fmt.Errorf("delete duplicate article_topic_tag %d: %w", link.ID, err)
				}
			} else {
				// No conflict — update the source link to point to target
				if err := tx.Model(&link).Update("topic_tag_id", targetTagID).Error; err != nil {
					return fmt.Errorf("update article_topic_tag %d to target: %w", link.ID, err)
				}
			}
		}

		// Step 3: Mark source tag as merged
		if err := tx.Model(&models.TopicTag{}).
			Where("id = ?", sourceTagID).
			Updates(map[string]interface{}{
				"status":         "merged",
				"merged_into_id": targetTagID,
			}).Error; err != nil {
			return fmt.Errorf("mark source tag as merged: %w", err)
		}

		// Step 4: Migrate topic_tag_relations for abstract hierarchy
		if err := migrateTagRelations(tx, sourceTagID, targetTagID); err != nil {
			return fmt.Errorf("migrate tag relations: %w", err)
		}

		// Step 5: Delete source tag's embedding (stale after merge)
		if err := tx.Where("topic_tag_id = ?", sourceTagID).Delete(&models.TopicTagEmbedding{}).Error; err != nil {
			return fmt.Errorf("delete source tag embedding: %w", err)
		}

		// Step 6: Recalculate target tag's feed_count
		if err := tx.Model(&models.TopicTag{}).
			Where("id = ?", targetTagID).
			Update("feed_count", tx.Model(&models.ArticleTopicTag{}).
				Select("COUNT(DISTINCT a.feed_id)").
				Joins("JOIN articles a ON a.id = article_topic_tags.article_id").
				Where("article_topic_tags.topic_tag_id = ?", targetTagID),
			).Error; err != nil {
			return fmt.Errorf("recalculate target feed_count: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if err := mergeReembeddingQueueFactory().Enqueue(sourceTagID, targetTagID); err != nil {
		return fmt.Errorf("enqueue merge re-embedding task: %w", err)
	}

	go enqueueAbstractTagUpdateIfTargetIsAbstract(targetTagID, "tag_merged")

	return nil
}

// migrateTagRelations migrates all topic_tag_relations involving sourceTagID to targetTagID.
// Handles both parent and child roles, deduplicates, and removes self-references.
func migrateTagRelations(tx *gorm.DB, sourceTagID, targetTagID uint) error {
	var targetTag models.TopicTag
	if err := tx.First(&targetTag, targetTagID).Error; err != nil {
		return fmt.Errorf("load target tag: %w", err)
	}
	isAbstractTarget := targetTag.Kind == "abstract" || targetTag.Source == "abstract"

	// Source as parent: relations where parent_id = source
	var parentRelations []models.TopicTagRelation
	if err := tx.Where("parent_id = ?", sourceTagID).Find(&parentRelations).Error; err != nil {
		return fmt.Errorf("find source parent relations: %w", err)
	}
	for _, rel := range parentRelations {
		childID := rel.ChildID
		if childID == targetTagID {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete self-referencing parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		if !isAbstractTarget {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete non-transferable parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		hasCycle, err := wouldCreateCycle(tx, targetTagID, childID)
		if err != nil {
			return fmt.Errorf("cycle check parent relation %d: %w", rel.ID, err)
		}
		if !hasCycle {
			hasCycle, err = wouldCreateCycle(tx, childID, targetTagID)
			if err != nil {
				return fmt.Errorf("reverse cycle check parent relation %d: %w", rel.ID, err)
			}
		}
		if hasCycle {
			logging.Warnf("skipping parent relation %d→%d: would create cycle with target %d", rel.ParentID, childID, targetTagID)
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete cyclic parent relation %d: %w", rel.ID, err)
			}
			continue
		}
		var existing int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", targetTagID, childID).
			Count(&existing)
		if existing > 0 {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete duplicate parent relation %d: %w", rel.ID, err)
			}
		} else {
			if err := tx.Model(&rel).Update("parent_id", targetTagID).Error; err != nil {
				return fmt.Errorf("migrate parent relation %d: %w", rel.ID, err)
			}
		}
	}

	// Source as child: relations where child_id = source
	var childRelations []models.TopicTagRelation
	if err := tx.Where("child_id = ?", sourceTagID).Find(&childRelations).Error; err != nil {
		return fmt.Errorf("find source child relations: %w", err)
	}
	for _, rel := range childRelations {
		parentID := rel.ParentID
		if parentID == targetTagID {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete self-referencing child relation %d: %w", rel.ID, err)
			}
			continue
		}
		if !isAbstractTarget {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete non-transferable child relation %d: %w", rel.ID, err)
			}
			continue
		}
		hasCycle, err := wouldCreateCycle(tx, parentID, targetTagID)
		if err != nil {
			return fmt.Errorf("cycle check child relation %d: %w", rel.ID, err)
		}
		if !hasCycle {
			hasCycle, err = wouldCreateCycle(tx, targetTagID, parentID)
			if err != nil {
				return fmt.Errorf("reverse cycle check child relation %d: %w", rel.ID, err)
			}
		}
		if hasCycle {
			logging.Warnf("skipping child relation %d→%d: would create cycle with target %d", parentID, rel.ChildID, targetTagID)
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete cyclic child relation %d: %w", rel.ID, err)
			}
			continue
		}
		var existing int64
		tx.Model(&models.TopicTagRelation{}).
			Where("parent_id = ? AND child_id = ?", parentID, targetTagID).
			Count(&existing)
		if existing > 0 {
			if err := tx.Delete(&rel).Error; err != nil {
				return fmt.Errorf("delete duplicate child relation %d: %w", rel.ID, err)
			}
		} else {
			if err := tx.Model(&rel).Update("child_id", targetTagID).Error; err != nil {
				return fmt.Errorf("migrate child relation %d: %w", rel.ID, err)
			}
		}
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

	var existing models.TopicTagEmbedding
	err := database.DB.Where("topic_tag_id = ? AND embedding_type = ?", embedding.TopicTagID, embedding.EmbeddingType).First(&existing).Error

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

	if embeddingType == EmbeddingTypeSemantic && tag.Category == "event" {
		for _, o := range opts {
			if len(o.ContextTitles) > 0 {
				text += ". 相关报道: " + strings.Join(o.ContextTitles, "；")
				break
			}
		}
	}

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

func enqueueAbstractTagUpdateIfTargetIsAbstract(targetTagID uint, reason string) {
	var tag models.TopicTag
	if err := database.DB.First(&tag, targetTagID).Error; err != nil {
		return
	}
	if tag.Source != "abstract" {
		return
	}
	EnqueueAbstractTagUpdate(targetTagID, reason)
}
