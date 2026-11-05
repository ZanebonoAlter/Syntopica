package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/tracing"

	"go.opentelemetry.io/otel"
	"gorm.io/gorm"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
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

// MatchThreshold is the minimum cosine similarity for a tag pair to be
// considered a merge candidate. Pairs below this threshold are ignored.
var MatchThreshold = 0.92

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
	router *airouter.Router
}

// NewEmbeddingService creates a new embedding service
func NewEmbeddingService() *EmbeddingService {
	return &EmbeddingService{
		router: airouter.NewRouter(),
	}
}

// GenerateEmbedding generates an embedding for a tag's text representation
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, tag *models.TopicTag, embeddingType string, opts ...EmbeddingTextOptions) (*tagmodels.TopicTagEmbedding, []float64, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "EmbeddingService.GenerateEmbedding")
	defer span.End()
	text := buildTagEmbeddingText(tag, embeddingType, opts...)
	textHash := hashText(embeddingType + "\n" + text)

	// Use router with failover to generate embedding
	req := airouter.EmbeddingRequest{
		Input:     []string{text},
		Operation: "tagmanagement.embedding",
		Metadata: map[string]any{
			"tag_id":    tag.ID,
			"tag_label": tag.Label,
			"category":  tag.Category,
		},
	}
	result, err := s.router.Embed(ctx, req, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, nil, ErrEmbeddingFailed
	}

	pgVecStr := FloatsToPgVector(result.Embeddings[0])

	embedding := &tagmodels.TopicTagEmbedding{
		TopicTagID:    tag.ID,
		EmbeddingType: embeddingType,
		EmbeddingVec:  pgVecStr,
		Dimension:     result.Dimensions,
		Model:         result.Model,
		TextHash:      textHash,
	}

	return embedding, result.Embeddings[0], nil
}

func (s *EmbeddingService) GenerateEmbeddingForText(ctx context.Context, tagID uint, embeddingType string, text string) (*tagmodels.TopicTagEmbedding, []float64, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "EmbeddingService.GenerateEmbeddingForText")
	defer span.End()
	textHash := hashText(embeddingType + "\n" + text)

	req := airouter.EmbeddingRequest{
		Input:     []string{text},
		Operation: "tagmanagement.embedding",
		Metadata:  map[string]any{"tag_id": tagID},
	}
	result, err := s.router.Embed(ctx, req, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, nil, ErrEmbeddingFailed
	}

	pgVecStr := FloatsToPgVector(result.Embeddings[0])

	embedding := &tagmodels.TopicTagEmbedding{
		TopicTagID:    tagID,
		EmbeddingType: embeddingType,
		EmbeddingVec:  pgVecStr,
		Dimension:     result.Dimensions,
		Model:         result.Model,
		TextHash:      textHash,
	}

	return embedding, result.Embeddings[0], nil
}

// GenerateEmbeddingsBatch embeds the given tags' texts of one embedding type
// in a single provider call (speed-up-tag-embedding D2/D3: 同类型攞批 ≤32).
// Returned embeddings are in the same order as tags. Router.Embed handles the
// per-item cache lookup/write for multi-input requests.
func (s *EmbeddingService) GenerateEmbeddingsBatch(ctx context.Context, tags []*models.TopicTag, embeddingType string) ([]*tagmodels.TopicTagEmbedding, error) {
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "EmbeddingService.GenerateEmbeddingsBatch")
	defer span.End()
	if len(tags) == 0 {
		return nil, nil
	}

	texts := make([]string, len(tags))
	for i, tag := range tags {
		texts[i] = buildTagEmbeddingText(tag, embeddingType)
	}

	req := airouter.EmbeddingRequest{
		Input:     texts,
		Operation: "tagmanagement.embedding",
		Metadata: map[string]any{
			"batch":          true,
			"batch_size":     len(tags),
			"embedding_type": embeddingType,
		},
	}
	result, err := s.router.Embed(ctx, req, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingFailed, err)
	}
	if len(result.Embeddings) < len(texts) {
		return nil, fmt.Errorf("%w: provider returned %d embeddings for %d inputs", ErrEmbeddingFailed, len(result.Embeddings), len(texts))
	}

	embs := make([]*tagmodels.TopicTagEmbedding, len(tags))
	for i, tag := range tags {
		if len(result.Embeddings[i]) == 0 {
			return nil, fmt.Errorf("%w: empty embedding for tag %d", ErrEmbeddingFailed, tag.ID)
		}
		text := texts[i]
		embs[i] = &tagmodels.TopicTagEmbedding{
			TopicTagID:    tag.ID,
			EmbeddingType: embeddingType,
			EmbeddingVec:  FloatsToPgVector(result.Embeddings[i]),
			Dimension:     result.Dimensions,
			Model:         result.Model,
			TextHash:      hashText(embeddingType + "\n" + text),
		}
	}
	return embs, nil
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
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "EmbeddingService.FindSimilarTags")
	defer span.End()
	_, rawFloats, err := s.GenerateEmbedding(ctx, tag, embeddingType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	pgVecStr := FloatsToPgVector(rawFloats)

	candidates, err := s.findSimilarByVector(pgVecStr, category, embeddingType, limit, len(rawFloats))
	if err != nil {
		return nil, err
	}
	return candidates, nil
}

// similarCandidatePool / similarEfSearch tune the two-stage similarity search:
// HNSW candidate-pool size and hnsw.ef_search (must be ≥ pool so the index
// returns a full pool; pgvector default ef_search=40 would truncate LIMIT 50).
const (
	similarCandidatePool = 50
	similarEfSearch      = 120
)

// findSimilarByVector is the two-stage similarity search behind
// FindSimilarTags (speed-up-tag-embedding design D1):
//
//  1. candidate pool: ORDER BY embedding::halfvec(N) <=> $1::halfvec(N)
//     LIMIT 50 — takes the partial halfvec HNSW index per embedding_type
//     (typmod MUST carry the dimension; a bare ::halfvec cast never uses the
//     index — production EXPLAIN verified).
//  2. rerank: within the candidate pool, exact float32 `embedding <=> $1::vector`
//     ordering — identical ranking semantics to the old full-table scan, so
//     the cosine verification口径 / 0.92 / 0.95 / 0.8 thresholds are unchanged.
//
// hnsw.ef_search is set with SET LOCAL inside the query transaction so it
// resets when the tx ends and never pollutes pooled connections. When dim is
// unknown/over 4000 (no halfvec index possible) or the pool already covers
// everything, stage 2 alone produces the exact same result (seq scan fallback,
// slow but correct — pinned by test-cases typmod-fallback variant).
func (s *EmbeddingService) findSimilarByVector(pgVecStr, category, embeddingType string, limit, dim int) ([]TagCandidate, error) {
	type simRow struct {
		TagID    uint    `gorm:"column:tag_id"`
		Distance float64 `gorm:"column:distance"`
	}
	var rows []simRow

	pool := similarCandidatePool
	if limit > pool {
		pool = limit
	}

	// Exact float32 rerank over a candidate id set (stage 2). Also used as the
	// no-index fallback when the candidate ids cover the whole filtered table.
	rerank := func(tx *gorm.DB, ids []uint) error {
		q := `
			SELECT t.id AS tag_id, e.embedding <=> ?::vector AS distance
			FROM topic_tag_embeddings e
			JOIN topic_tags t ON t.id = e.topic_tag_id
			WHERE t.category = ?
			  AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
			  AND e.embedding IS NOT NULL
			  AND e.embedding_type = ?
			  AND e.topic_tag_id IN ?
			ORDER BY e.embedding <=> ?::vector
			LIMIT ?
		`
		return tx.Raw(q, pgVecStr, category, embeddingType, ids, pgVecStr, limit).Scan(&rows).Error
	}

	err := repository.Repo.DB().Transaction(func(tx *gorm.DB) error {
		if dim <= 0 || dim > 4000 {
			// No halfvec HNSW possible (>4000 dims): exact full scan, semantics
			// identical to the pre-change query.
			q := `
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
			return tx.Raw(q, pgVecStr, category, embeddingType, pgVecStr, limit).Scan(&rows).Error
		}
		// SET LOCAL scopes ef_search to this transaction — pooled connections
		// are unaffected once the tx commits/rolls back.
		if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", similarEfSearch)).Error; err != nil {
			return fmt.Errorf("set hnsw.ef_search: %w", err)
		}
		candSQL := fmt.Sprintf(`
			SELECT e.topic_tag_id AS tag_id
			FROM topic_tag_embeddings e
			JOIN topic_tags t ON t.id = e.topic_tag_id
			WHERE t.category = ?
			  AND (t.status = 'active' OR t.status = '' OR t.status IS NULL)
			  AND e.embedding IS NOT NULL
			  AND e.embedding_type = ?
			ORDER BY e.embedding::halfvec(%d) <=> ?::halfvec(%d)
			LIMIT ?
		`, dim, dim)
		var candIDs []uint
		if err := tx.Raw(candSQL, category, embeddingType, pgVecStr, pool).Scan(&candIDs).Error; err != nil {
			return fmt.Errorf("failed to query similar tag candidates: %w", err)
		}
		if len(candIDs) == 0 {
			return nil
		}
		return rerank(tx, candIDs)
	})
	if err != nil {
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
	if err := repository.Repo.DB().Where("id IN ?", tagIDs).Find(&tags).Error; err != nil {
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
	ctx, span := otel.Tracer(tracing.ServiceName).Start(ctx, "EmbeddingService.TagMatch")
	defer span.End()
	slug := Slugify(label)
	logging.Infof("TagMatch: start label=%q slug=%q category=%s threshold=%.2f", label, slug, category, MatchThreshold)
	var existingTag models.TopicTag
	err := repository.Repo.DB().Scopes(activeTagFilter).Where("slug = ? AND category = ?", slug, category).First(&existingTag).Error
	if err == nil {
		logging.Infof("TagMatch: label=%q category=%s result=exact reason=slug existingID=%d existingLabel=%q", label, category, existingTag.ID, existingTag.Label)
		return &TagMatchResult{
			MatchType:   "exact",
			ExistingTag: &existingTag,
			Similarity:  1.0,
		}, nil
	}

	if aliases != "" {
		aliasScanFallback := repository.Repo.DB().Name() != "postgres"
		if repository.Repo.DB().Name() == "postgres" {
			var aliasMatch models.TopicTag
			aliasSQL := `category = ? AND aliases IS NOT NULL AND aliases != '' AND EXISTS (
				SELECT 1 FROM jsonb_array_elements_text(aliases::jsonb) AS alias(value)
				WHERE LOWER(alias.value) = LOWER(?)
			)`
			if err := repository.Repo.DB().Scopes(activeTagFilter).Where(aliasSQL, category, label).First(&aliasMatch).Error; err == nil {
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
			if err := repository.Repo.DB().Scopes(activeTagFilter).Where("category = ?", category).Find(&aliasTags).Error; err == nil {
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
		if c.Similarity >= MatchThreshold {
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
	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "EmbeddingService.FindSimilarTagsAmongSet")
	defer span.End()
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
	if err := repository.Repo.DB().Raw(query, tagIDs, tagIDs, 1.0-threshold).Scan(&rows).Error; err != nil {
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

var DefaultMergeReembeddingQueueFactory = func() mergeReembeddingEnqueuer {
	return NewMergeReembeddingQueueService(nil)
}

var MergeReembeddingQueueFactory = DefaultMergeReembeddingQueueFactory

// EnqueueMergeReembedding enqueues a merge re-embedding task for the given source and target tags.
func EnqueueMergeReembedding(sourceTagID, targetTagID uint) error {
	return MergeReembeddingQueueFactory().Enqueue(sourceTagID, targetTagID)
}

// MergeTags hard-deletes sourceTag after migrating all references to targetTag.
func MergeTags(sourceTagID, targetTagID uint) error {
	if sourceTagID == targetTagID {
		return fmt.Errorf("cannot merge tag into itself (id=%d)", sourceTagID)
	}

	if err := HardMergeTags(repository.Repo.DB(), sourceTagID, targetTagID); err != nil {
		return err
	}

	if err := MergeReembeddingQueueFactory().Enqueue(sourceTagID, targetTagID); err != nil {
		return fmt.Errorf("enqueue merge re-embedding task: %w", err)
	}

	return nil
}

// SaveEmbedding saves or updates a tag's embedding in the database.
// If the actual vector dimension differs from the column definition, it alters the column type.
func (s *EmbeddingService) SaveEmbedding(embedding *tagmodels.TopicTagEmbedding) error {
	if embedding.Dimension > 0 && embedding.EmbeddingVec != "" {
		if err := ensureVectorDimension(embedding.Dimension); err != nil {
			return fmt.Errorf("ensure vector dimension %d: %w", embedding.Dimension, err)
		}
	}

	var tag models.TopicTag
	if err := repository.Repo.DB().Select("id").First(&tag, embedding.TopicTagID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTopicTagNotFound
		}
		return fmt.Errorf("load topic tag %d: %w", embedding.TopicTagID, err)
	}

	// Clean up stale embeddings: same tag + type but different text_hash
	repository.Repo.DB().Where(
		"topic_tag_id = ? AND embedding_type = ? AND text_hash != ?",
		embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash,
	).Delete(&tagmodels.TopicTagEmbedding{})

	var existing tagmodels.TopicTagEmbedding
	err := repository.Repo.DB().Where("topic_tag_id = ? AND embedding_type = ? AND text_hash = ?", embedding.TopicTagID, embedding.EmbeddingType, embedding.TextHash).First(&existing).Error

	if err == nil {
		embedding.ID = existing.ID
		return repository.Repo.DB().Save(embedding).Error
	}

	return repository.Repo.DB().Create(embedding).Error
}

// ensureVectorDimension checks if the embedding column matches the required dimension
// and alters it (plus the indexes) if not. Drops indexes before ALTER, recreates after.
//
// Index policy (optimize-search-semantic-index design D4):
//   - float32 HNSW idx_topic_tag_embeddings_embedding：dim ≤ 2000（pgvector float32
//     HNSW 硬上限）才建；aux-label 去重另有 Go 侧余弦缓存兜底（sqlMergeMatcher）。
//   - halfvec identity 部分 HNSW（database.TopicTagIdentityHalfvecIndexDDL）：语义搜索
//     标签召回的 ANN 索引，dim ≤ 4000（pgvector halfvec 上限）才建。
//   - dim > 4000：两个索引都建不了，语义路回退旧的全表精确扫描（行为不中断）。
//
// 两个索引的表达式都内嵌列维度（typmod），维度变更后旧索引的表达式与新列类型
// 不再匹配，所以必须先 drop 再 ALTER，否则 Postgres 报错/留下失效索引。
func ensureVectorDimension(dim int) error {
	var typeStr string
	if err := repository.Repo.DB().Raw(`
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

	// Drop indexes first — they depend on the column type (float32 index on the
	// column, halfvec index on the cast expression with the old dimension).
	for _, indexName := range []string{"idx_topic_tag_embeddings_embedding", database.TopicTagIdentityHalfvecIndexName} {
		if err := repository.Repo.DB().Exec("DROP INDEX IF EXISTS " + indexName).Error; err != nil { //nolint:gosec // G202: 索引名来自包内常量/固定字面量
			logging.Warnf("Failed to drop vector index %s: %v", indexName, err)
		}
	}

	if err := repository.Repo.DB().Exec(fmt.Sprintf(
		"ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE %s", expected,
	)).Error; err != nil {
		return fmt.Errorf("alter embedding column to %s: %w", expected, err)
	}

	// Recreate the float32 index — HNSW supports max 2000 dimensions.
	if dim <= 2000 {
		if err := repository.Repo.DB().Exec(
			"CREATE INDEX idx_topic_tag_embeddings_embedding ON topic_tag_embeddings USING hnsw (embedding vector_cosine_ops)",
		).Error; err != nil {
			logging.Warnf("Failed to recreate HNSW index: %v", err)
		}
	} else {
		// No float32 DB-side vector index (>2000 dim exceeds pgvector HNSW limit).
		// aux-label dedup uses Go-side cosine over the process-cached embeddings
		// (sqlMergeMatcher), so queries still work without an ANN index here.
		logging.Infof("Dimension %d exceeds pgvector float32 HNSW limit (2000); skipping float32 vector index (aux-label dedup falls back to Go-side cosine via cache)", dim)
	}

	// Recreate the halfvec identity index used by semantic search tag recall
	// (design D1/D4). Same DDL as migration 20261023_0001 — single source of truth.
	if dim > 0 && dim <= 4000 {
		// Pin one connection for the session-level build GUCs (Connection does not
		// open a transaction, so plain CREATE INDEX is fine). Disable parallel
		// maintenance: Docker's default 64MB /dev/shm makes HNSW parallel builds
		// fail with SQLSTATE 53100 (seen in the production migration).
		if err := repository.Repo.DB().Connection(func(tx *gorm.DB) error {
			// Register RESET before the first SET: if the first SET itself fails,
			// the pinned connection still returns to the pool clean.
			defer func() {
				_ = tx.Exec("RESET maintenance_work_mem").Error
				_ = tx.Exec("RESET max_parallel_maintenance_workers").Error
			}()
			if err := tx.Exec("SET maintenance_work_mem = '256MB'").Error; err != nil {
				return err
			}
			if err := tx.Exec("SET max_parallel_maintenance_workers = 0").Error; err != nil {
				return err
			}
			return tx.Exec(database.TopicTagIdentityHalfvecIndexDDL(dim)).Error
		}); err != nil {
			logging.Warnf("Failed to recreate halfvec identity HNSW index: %v", err)
		}
	} else {
		logging.Infof("Dimension %d exceeds pgvector halfvec HNSW limit (4000); skipping semantic identity index (semantic search falls back to exact scan)", dim)
	}

	return nil
}

// GetEmbedding retrieves the embedding for a tag
func (s *EmbeddingService) GetEmbedding(tagID uint, embeddingType string) (*tagmodels.TopicTagEmbedding, error) {
	var embedding tagmodels.TopicTagEmbedding
	err := repository.Repo.DB().Where("topic_tag_id = ? AND embedding_type = ?", tagID, embeddingType).First(&embedding).Error
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
	repository.Repo.DB().Raw(query, tagID, limit).Scan(&titles)
	return titles
}

func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// floatsToPgVector converts a float64 slice to pgvector string format: [0.1,0.2,0.3]
func FloatsToPgVector(v []float64) string {
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
