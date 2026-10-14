package search

// add-article-search tasks 2.4 — 语义路径（design D3）。
//
// query embedding → 与 identity 型 topic_tag_embeddings 做 cosine
// （pgvector <=>）双重约束（cosine ≥ search_semantic_min_cosine ∧ 排名前
// search_semantic_top_n）→ 经 article_topic_tags 召回文章，文章语义分 =
// 命中标签的 max cosine；命中标签全量进 hit_reasons。
//
// 依赖注入：Embedder 窄接口隔离 airouter（测试注入 fake；生产 Router 由
// wire/构造注入，不 import 全局单例）。3s 超时在 Search 内部施加
// （ctx.WithTimeout 包住 embedding + 两段 SQL——调用方无需自管，见汇报）。
// embedding 失败/超时 → ErrSemanticSkipped 哨兵（不报错，调用方置
// semantic.skipped=true）。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/logging"
)

// semanticQueryOperation 是语义路径 query embedding 的 airouter 业务名
// （AI 调用记录红线：operation 必填、经 Router.Embed 留痕）。
// 注意：airouter 的 ai_embedding_cache 白名单（embeddingCacheOperations）
// 已登记本 operation（tasks 2.6 接线，design D3 查询 embedding 走缓存），
// 同查询重复搜索命中缓存免 provider 调用。
const semanticQueryOperation = "article_search_query"

// defaultSemanticTimeout 语义路径整体超时上限（spec「embedding 不可用降级」
// ：上限 3s）。
const defaultSemanticTimeout = 3 * time.Second

// ErrSemanticSkipped 语义路径降级哨兵：embedding 不可用/超时。语义路是
// 增强路径——调用方据此置 semantic.skipped=true 并正常返回关键词结果。
var ErrSemanticSkipped = errors.New("search: semantic path skipped")

// Embedder 语义查询向量的窄接口。
type Embedder interface {
	EmbedQuery(ctx context.Context, input string) ([]float64, error)
}

// AirouterEmbedder 把 airouter.Router.Embed 适配为 Embedder（生产实现，
// 由调用方构造注入 Router）。
type AirouterEmbedder struct {
	Router *airouter.Router
}

// EmbedQuery wraps Router.Embed with the search operation name; the result
// carries exactly one vector (single-query input).
func (a *AirouterEmbedder) EmbedQuery(ctx context.Context, input string) ([]float64, error) {
	if a == nil || a.Router == nil {
		return nil, errors.New("search: nil airouter router")
	}
	result, err := a.Router.Embed(ctx, airouter.EmbeddingRequest{
		Input:     []string{input},
		Operation: semanticQueryOperation,
	}, airouter.CapabilityEmbedding)
	if err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, errors.New("search: empty embedding result")
	}
	return result.Embeddings[0], nil
}

// MatchedTag 语义路径命中的标签（标签名 + cosine 分）。
type MatchedTag struct {
	TagID  uint
	Label  string
	Cosine float64
}

// SemanticHit 语义路径召回的文章：行投影 + 语义分（max cosine）+ 全量
// 命中标签。
type SemanticHit struct {
	Article ArticleSearchRow
	// Cosine 文章语义分 = 命中标签的 max cosine（design D3）。
	Cosine float64
	// Tag 最高分命中标签。
	Tag MatchedTag
	// MatchedTags 该文章命中的全部标签（design D3：全量进 hit_reasons）。
	MatchedTags []MatchedTag
}

// SemanticSearcher 语义路径执行器。loadConfig 注入点供测试替身；timeout
// 默认 3s，测试可白盒调短。
type SemanticSearcher struct {
	db         *gorm.DB
	embedder   Embedder
	loadConfig func(ctx context.Context) SemanticConfig
	timeout    time.Duration
}

// NewSemanticSearcher wires the production config source (ai_settings).
func NewSemanticSearcher(db *gorm.DB, embedder Embedder) *SemanticSearcher {
	return &SemanticSearcher{
		db:       db,
		embedder: embedder,
		loadConfig: func(ctx context.Context) SemanticConfig {
			return LoadSemanticConfig(ctx, db)
		},
		timeout: defaultSemanticTimeout,
	}
}

// tagCosineRow 标签 cosine 查询行。
type tagCosineRow struct {
	TagID  uint    `gorm:"column:tag_id"`
	Label  string  `gorm:"column:label"`
	Cosine float64 `gorm:"column:cosine"`
}

// semanticArticleRow 标签→文章召回行。
type semanticArticleRow struct {
	ArticleSearchRow
	TagID uint `gorm:"column:tag_id"`
}

// Search executes the semantic path. Returns (hits, matchedTags, err)：
//   - embedding 失败/超时 → (nil, nil, ErrSemanticSkipped)
//   - 零命中（含无 identity 行）→ (空, 空, nil)——区别于 skipped
//     （spec：语义零命中时 semantic 块为空标签列表，非服务不可用）
//   - archived 参数口径同关键词路径（include 默认含归档 / active-only）
func (s *SemanticSearcher) Search(ctx context.Context, query string, archived string) ([]SemanticHit, []MatchedTag, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	vec, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		logging.Warnf("search: semantic embedding unavailable, skipped: %v", err)
		return nil, nil, ErrSemanticSkipped
	}

	cfg := s.loadConfig(ctx)
	pgVec := floatsToPgVector(vec)

	// 1) 标签双约束：cosine ≥ MinCosine（HAVING）∧ 排名前 TopN（ORDER BY
	// cosine DESC LIMIT）。同一标签多 text_hash 行（重嵌过渡期）按
	// GROUP BY 取 max——排名与阈值都以聚合后的标签为单位。
	var tagRows []tagCosineRow
	tagSQL := `
SELECT e.topic_tag_id AS tag_id,
       t.label AS label,
       MAX(1 - (e.embedding <=> ?::vector)) AS cosine
FROM topic_tag_embeddings e
JOIN topic_tags t ON t.id = e.topic_tag_id
WHERE e.embedding_type = 'identity'
GROUP BY e.topic_tag_id, t.label
HAVING MAX(1 - (e.embedding <=> ?::vector)) >= ?
ORDER BY cosine DESC
LIMIT ?`
	if err := s.db.WithContext(ctx).Raw(tagSQL, pgVec, pgVec, cfg.MinCosine, cfg.TopN).Scan(&tagRows).Error; err != nil {
		return nil, nil, fmt.Errorf("search semantic tags: %w", err)
	}
	if len(tagRows) == 0 {
		return nil, nil, nil
	}

	matchedTags := make([]MatchedTag, len(tagRows))
	byTagID := make(map[uint]MatchedTag, len(tagRows))
	tagIDs := make([]uint, len(tagRows))
	for i, row := range tagRows {
		matchedTags[i] = MatchedTag(row)
		byTagID[row.TagID] = matchedTags[i]
		tagIDs[i] = row.TagID
	}

	// 2) 标签 → 文章召回（零写入，只读消费 topic_tag_embeddings 与
	// article_topic_tags——红线 12）。文章可能挂多个命中标签，Go 聚合。
	var articleRows []semanticArticleRow
	query2 := s.db.WithContext(ctx).Table("article_topic_tags att").
		Joins("JOIN articles ON articles.id = att.article_id").
		Select(articleSearchSelect+", att.topic_tag_id AS tag_id").
		Where("att.topic_tag_id IN ?", tagIDs)
	if archived == ArchiveActiveOnly {
		query2 = query2.Where("articles.archived = ?", false)
	}
	if err := query2.Scan(&articleRows).Error; err != nil {
		return nil, nil, fmt.Errorf("search semantic articles: %w", err)
	}

	byArticle := make(map[uint]*SemanticHit, len(articleRows))
	for i := range articleRows {
		row := &articleRows[i]
		tag := byTagID[row.TagID]
		hit := byArticle[row.ID]
		if hit == nil {
			hit = &SemanticHit{Article: row.ArticleSearchRow}
			byArticle[row.ID] = hit
		}
		hit.MatchedTags = append(hit.MatchedTags, tag)
		if tag.Cosine > hit.Cosine {
			hit.Cosine = tag.Cosine
			hit.Tag = tag
		}
	}

	out := make([]SemanticHit, 0, len(byArticle))
	for _, hit := range byArticle {
		out = append(out, *hit)
	}
	return out, matchedTags, nil
}

// floatsToPgVector converts a float64 slice to a pgvector literal
// [0.100000,0.200000]。模式复制自 tagmanagement 的 FloatsToPgVector——
// 跨域 service 深路径 depguard 禁止 import，本地实现同语义。
func floatsToPgVector(v []float64) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
