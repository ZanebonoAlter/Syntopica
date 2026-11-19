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
	// 写入前的维度一致性校验（embedding-dimension-contract）：向量列不再带
	// 维度约束（unify-vector-column-dimension），列级校验已消失——写进一条错
	// 维度的向量比事后查询时报错更难排查，这里是唯一的写入侧防线。
	if embedding.Dimension > 0 && embedding.EmbeddingVec != "" {
		if err := airouter.ValidatePgVectorDimension(embedding.EmbeddingVec, embedding.Dimension); err != nil {
			logging.Errorf("Rejecting embedding write for tag %d (%s): %v", embedding.TopicTagID, embedding.EmbeddingType, err)
			return fmt.Errorf("embedding dimension check: %w", err)
		}
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

// ensureVectorDimension 让 topic_tag_embeddings.embedding 的**向量索引**与当前
// 运行时维度保持一致。列类型只在“还带着过时的维度限定”时才动。
//
// 三分支（unify-vector-column-dimension design D3/D4 + 缺口 B）：
//
//  1. 列已是无维度限定的 vector（目标形态）：**不执行 ALTER**。维度是运行时
//     属性，schema 不承载它——ALTER 只会把分叉源搬回 DDL。只按运行时维度
//     校验/重建 halfvec 索引（表达式内嵌维度，漂移必须重建）。
//  2. 列是 vector(dim) 且与运行时维度一致：列不动，只确保索引存在。
//  3. 列是 vector(其他N)：**迁移过渡期分支**——只有尚未跑统一迁移的旧库才会
//     落进来；保留原有的 DROP INDEX + ALTER + 重建纠正动作（统一迁移跑完后该
//     分支自然失效，因为列已经是裸 vector）。
//
// 索引策略（optimize-search-semantic-index design D4）：
//   - halfvec identity 部分 HNSW（database.TopicTagIdentityHalfvecIndexDDL）：
//     语义搜索标签召回的 ANN 索引，dim ≤ 4000（pgvector halfvec 上限）才建；
//     裸 vector 列上也能建（表达式自带维度）。
//   - float32 HNSW idx_topic_tag_embeddings_embedding：dim ≤ 2000（pgvector
//     float32 HNSW 硬上限）才建，且**只在线列带维度限定时可建**——裸 vector 列
//     建它会报 "column does not have dimensions"，且存活的 float32 索引会把列
//     钉在旧维度上。语义召回主路径走 halfvec 索引，float32 索引无查询在用。
//   - dim > 4000：两个索引都建不了，语义路回退旧的全表精确扫描（行为不中断）。
func ensureVectorDimension(dim int) error {
	db := repository.Repo.DB()

	typeStr, err := database.VectorColumnType(db, "topic_tag_embeddings", "embedding")
	if err != nil {
		return nil // 列不存在：让迁移处理
	}

	expected := fmt.Sprintf("vector(%d)", dim)
	switch typeStr {
	case "vector":
		// 分支 1：裸 vector —— 不 ALTER，只管索引。
		return ensureTopicTagVectorIndexes(db, dim, false)
	case expected:
		// 分支 2：列维度与运行时一致 —— 列不动，只确保索引存在。
		return ensureTopicTagVectorIndexes(db, dim, true)
	}

	// 分支 3：迁移过渡期 —— 列仍带过时的维度限定，保留原有纠正动作。
	logging.Infof("Altering embedding column from %s to %s", typeStr, expected)

	// Drop indexes first — they depend on the column type (float32 index on the
	// column, halfvec index on the cast expression with the old dimension).
	for _, indexName := range []string{"idx_topic_tag_embeddings_embedding", database.TopicTagIdentityHalfvecIndexName} {
		if err := db.Exec("DROP INDEX IF EXISTS " + indexName).Error; err != nil { //nolint:gosec // G202: 索引名来自包内常量/固定字面量
			logging.Warnf("Failed to drop vector index %s: %v", indexName, err)
		}
	}

	if err := db.Exec(fmt.Sprintf(
		"ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE %s", expected,
	)).Error; err != nil {
		return fmt.Errorf("alter embedding column to %s: %w", expected, err)
	}

	// 列已带维度限定 → float32 索引可建。
	return ensureTopicTagVectorIndexes(db, dim, true)
}

// ensureTopicTagVectorIndexes 确保 topic_tag_embeddings 上的向量索引与运行时维度
// 一致。
//
//   - halfvec identity 部分 HNSW：表达式内嵌维度 → 运行时维度变了必须重建；
//     dim ≤ 4000 才建，超限跳过并记日志（语义召回回退精确扫描）。
//   - float32 HNSW：dim ≤ 2000 才建，且只在线列带维度限定时可建——裸 vector 列
//     建它会报 "column does not have dimensions"；裸列上残留的同名索引会把列
//     钉在旧维度上（新维度向量写入直接被拒）→ 删除。
//
// columnHasFixedDimension：列类型是否带维度限定（vector(dim)）。
func ensureTopicTagVectorIndexes(db *gorm.DB, dim int, columnHasFixedDimension bool) error {
	// ── halfvec identity 索引（语义召回 ANN 路径）──
	if dim > 0 && dim <= 4000 {
		if err := ensureTopicTagHalfvecIndex(db, dim); err != nil {
			logging.Warnf("Failed to ensure halfvec identity HNSW index: %v", err)
		}
	} else {
		logging.Infof("Dimension %d exceeds pgvector halfvec HNSW limit (4000); skipping semantic identity index (semantic search falls back to exact scan)", dim)
	}

	// ── float32 HNSW 索引 ──
	const float32Index = "idx_topic_tag_embeddings_embedding"
	if dim > 2000 {
		logging.Infof("Dimension %d exceeds pgvector float32 HNSW limit (2000); skipping float32 vector index (aux-label dedup falls back to Go-side cosine via cache)", dim)
		return nil
	}
	if !columnHasFixedDimension {
		exists, err := database.VectorIndexExists(db, float32Index)
		if err != nil {
			logging.Warnf("Failed to probe float32 vector index %s: %v", float32Index, err)
			return nil
		}
		if exists {
			logging.Infof("Dropping legacy float32 vector index %s: a dimensionless column cannot host a float32 HNSW index and the stale index pins the column to its old dimension", float32Index)
			if err := db.Exec("DROP INDEX IF EXISTS " + float32Index).Error; err != nil { //nolint:gosec // G202: 索引名是包内常量
				logging.Warnf("Failed to drop float32 vector index %s: %v", float32Index, err)
			}
		}
		return nil
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS " + float32Index + //nolint:gosec // G202: 索引名是包内常量
		" ON topic_tag_embeddings USING hnsw (embedding vector_cosine_ops)").Error; err != nil {
		logging.Warnf("Failed to create float32 HNSW index: %v", err)
	}
	return nil
}

// ensureTopicTagHalfvecIndex 按运行时维度确保 halfvec identity 索引的表达式内嵌
// 维度正确——不符（或索引 invalid）则重建。
//
// 构建期 GUC 设在固定连接上（会话级 SET + RESET）：运行期不在事务里，SET LOCAL
// 无效；迁移路径（20261023_0001 / 20261117_0002）走事务内的 SET LOCAL。
func ensureTopicTagHalfvecIndex(db *gorm.DB, dim int) error {
	existingDim, valid := database.TopicTagIdentityHalfvecIndexState(db)
	if existingDim == dim && valid {
		return nil // 已是正确表达式且有效：不动
	}
	if existingDim != 0 {
		logging.Infof("Rebuilding halfvec identity HNSW index: embedded dimension %d != runtime dimension %d", existingDim, dim)
	}
	// Pin one connection for the session-level build GUCs (Connection does not
	// open a transaction, so plain CREATE INDEX is fine). Disable parallel
	// maintenance: Docker's default 64MB /dev/shm makes HNSW parallel builds
	// fail with SQLSTATE 53100 (seen in the production migration).
	return db.Connection(func(tx *gorm.DB) error {
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
		return database.EnsureTopicTagIdentityHalfvecIndex(tx, dim)
	})
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
