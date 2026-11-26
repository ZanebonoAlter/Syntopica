package search

// add-article-search tasks 2.4 — 语义路径（design D3）。
//
// query embedding → 与 identity 型 topic_tag_embeddings 做 cosine
// （pgvector <=>）双重约束（cosine ≥ search_semantic_min_cosine ∧ 排名前
// search_semantic_top_n）→ 经 article_topic_tags 召回文章，文章语义分 =
// 命中标签的 max cosine；命中标签全量进 hit_reasons。
//
// 标签召回分两段（optimize-search-semantic-index design D2/D3）：先走
// halfvec 部分 HNSW 索引取 LIMIT 128 近似候选，再按候选 ID 用原始 float32
// 精确复算 cosine（近似分只决定谁进候选，绝不当最终分数）；维度超 halfvec
// 上限 4000 或 ANN 出错时回退旧的全表精确扫描（行为不中断，代价是慢）。
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
	"sort"
	"strconv"
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

// 标签召回 ANN 参数（design D3）：候选数远大于默认 top_n=8，且能覆盖同 tag
// 多 text_hash 行与被阈值过滤掉的候选；ef_search 与候选数同档（先固定，后续
// 可按实测召回率调）。候选下限 128；TopN 被配置调大时按 4 倍跟随（见
// matchTagsANN），不会把合法的大 TopN 悄悄截断。
const (
	semanticCandidateLimit = 128
	// semanticMaxCandidates 候选上限：pgvector hnsw.ef_search 合法域 1..1000，
	// 候选数超过 1000 时 ef_search 会越界报错；1000 也足够覆盖任何合理 top_n。
	semanticMaxCandidates = 1000
	// semanticMaxIndexedDim halfvec HNSW 支持的维度上限（pgvector 0.8 为
	// 1..4000）。超过则索引建不了（迁移 20261023_0001 同样跳过），语义路退回
	// 旧的全表精确扫描。
	semanticMaxIndexedDim = 4000
)

// 分阶段慢日志阈值（design D7）：正常不刷日志，超阈值才 warn，用来定位
// 下次「语义又慢了」到底慢在 embedding / 候选 / 复算 / 文章召回哪一段。
const (
	semanticEmbeddingWarnThreshold = 1 * time.Second
	semanticCandidateWarnThreshold = 200 * time.Millisecond
	semanticRecomputeWarnThreshold = 100 * time.Millisecond
	semanticArticlesWarnThreshold  = 200 * time.Millisecond
)

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

// tagCosineRow 精确回退路径的标签 cosine 查询行。
type tagCosineRow struct {
	TagID  uint    `gorm:"column:tag_id"`
	Label  string  `gorm:"column:label"`
	Cosine float64 `gorm:"column:cosine"`
}

// tagCandidateRow ANN 候选行（design D2 第一段：只取 ID，不带向量）。
type tagCandidateRow struct {
	ID    uint `gorm:"column:id"`
	TagID uint `gorm:"column:tag_id"`
}

// tagRecomputeRow 候选精确复算行（design D2 第二段：原始 float32 cosine）。
type tagRecomputeRow struct {
	ID     uint    `gorm:"column:id"`
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

	embedStart := time.Now()
	vec, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		logging.Warnf("search: semantic embedding unavailable, skipped: %v", err)
		return nil, nil, ErrSemanticSkipped
	}
	if elapsed := time.Since(embedStart); elapsed > semanticEmbeddingWarnThreshold {
		logging.Warnf("search: semantic embedding slow: %s (query runes %d)", elapsed, len([]rune(query)))
	}

	cfg := s.loadConfig(ctx)

	// 1) 标签双约束：cosine ≥ MinCosine ∧ 排名前 TopN。同一标签多 text_hash
	// 行（重嵌过渡期）按 tag 取 max——排名与阈值都以聚合后的标签为单位。
	// matchTags 内部选路：ANN 候选 + 精确复算，或旧全表精确回退。
	matchedTags, err := s.matchTags(ctx, vec, cfg)
	if err != nil {
		return nil, nil, err
	}
	if len(matchedTags) == 0 {
		return nil, nil, nil
	}

	byTagID := make(map[uint]MatchedTag, len(matchedTags))
	tagIDs := make([]uint, len(matchedTags))
	for i, tag := range matchedTags {
		byTagID[tag.TagID] = tag
		tagIDs[i] = tag.TagID
	}

	// 2) 标签 → 文章召回（零写入，只读消费 topic_tag_embeddings 与
	// article_topic_tags——红线 12）。文章可能挂多个命中标签，Go 聚合。
	articlesStart := time.Now()
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
	if elapsed := time.Since(articlesStart); elapsed > semanticArticlesWarnThreshold {
		logging.Warnf("search: semantic article recall slow: %s (%d tags, %d rows)", elapsed, len(tagIDs), len(articleRows))
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

// matchTags 标签召回选路（design D2/D4）：维度可建 halfvec 索引时走 ANN
// 候选 + 精确复算；维度超限或 ANN 出错时回退旧的全表精确扫描——索引缺失
// 只意味着慢，不能变成语义不可用。
func (s *SemanticSearcher) matchTags(ctx context.Context, vec []float64, cfg SemanticConfig) ([]MatchedTag, error) {
	if len(vec) == 0 {
		return nil, nil
	}
	pgVec := floatsToPgVector(vec)
	if len(vec) > semanticMaxIndexedDim {
		return s.matchTagsExact(ctx, pgVec, cfg)
	}
	tags, err := s.matchTagsANN(ctx, vec, pgVec, cfg)
	if err != nil {
		logging.Warnf("search: semantic ANN candidates unavailable (dim %d), falling back to exact scan: %v", len(vec), err)
		return s.matchTagsExact(ctx, pgVec, cfg)
	}
	return tags, nil
}

// matchTagsANN 走 halfvec 部分 HNSW 索引取近似候选（design D2 第一段），再按
// 候选 ID 用原始 float32 `<=>` 精确复算 cosine（第二段）。近似分只决定谁进
// 候选，绝不当最终分数：量化误差可能让阈值边界标签翻面，违背 spec 的精确
// cosine 契约。
func (s *SemanticSearcher) matchTagsANN(ctx context.Context, vec []float64, pgVec string, cfg SemanticConfig) ([]MatchedTag, error) {
	// 列维度是运行时配置（当前 2560），表达式类型随之变化，必须按 len(vec)
	// 拼 halfvec(N)；索引 DDL（database.TopicTagIdentityHalfvecIndexDDL）用
	// 同一表达式，两者维度一致才能命中索引。
	halfvecType := fmt.Sprintf("halfvec(%d)", len(vec))
	// 候选上限以 128 为下限；TopN 被配置调大时按 4 倍跟随，避免把
	// search_semantic_top_n > 128 的合法配置悄悄截断。上限 1000：pgvector
	// 的 hnsw.ef_search 合法域是 1..1000，超限会让 set_config 报错并静默
	// 回退全表扫描（反而复现本 change 要治的超时）；同时限制 IN 参数规模。
	candidateLimit := semanticCandidateLimit
	if cfg.TopN > 0 && cfg.TopN*4 > candidateLimit {
		candidateLimit = cfg.TopN * 4
	}
	if candidateLimit > semanticMaxCandidates {
		candidateLimit = semanticMaxCandidates
	}
	candidateSQL := fmt.Sprintf(`
SELECT e.id AS id, e.topic_tag_id AS tag_id
FROM topic_tag_embeddings e
WHERE e.embedding_type = 'identity'
ORDER BY (e.embedding::%s) <=> ?::%s
LIMIT ?`, halfvecType, halfvecType) //nolint:gosec // G201: halfvecType 由 len(vec) 整数格式化而来，无注入面
	recomputeSQL := `
SELECT e.id AS id, e.topic_tag_id AS tag_id, t.label AS label,
       1 - (e.embedding <=> ?::vector) AS cosine
FROM topic_tag_embeddings e
JOIN topic_tags t ON t.id = e.topic_tag_id
WHERE e.id IN ?`

	var candidates []tagCandidateRow
	var rows []tagRecomputeRow
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// hnsw.ef_search 是本地 GUC（set_config is_local=true），须在同事务内
		// 设置，事务结束自动复位，不泄漏到连接池。
		if err := tx.Exec(`SELECT set_config('hnsw.ef_search', ?, true)`, strconv.Itoa(candidateLimit)).Error; err != nil {
			return fmt.Errorf("set hnsw.ef_search: %w", err)
		}

		candidatesStart := time.Now()
		if err := tx.Raw(candidateSQL, pgVec, candidateLimit).Scan(&candidates).Error; err != nil {
			return fmt.Errorf("semantic ANN candidates: %w", err)
		}
		if elapsed := time.Since(candidatesStart); elapsed > semanticCandidateWarnThreshold {
			logging.Warnf("search: semantic ANN candidates slow: %s (%d candidates)", elapsed, len(candidates))
		}
		if len(candidates) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(candidates))
		seen := make(map[uint]struct{}, len(candidates))
		for _, candidate := range candidates {
			if _, ok := seen[candidate.ID]; ok {
				continue
			}
			seen[candidate.ID] = struct{}{}
			ids = append(ids, candidate.ID)
		}

		recomputeStart := time.Now()
		if err := tx.Raw(recomputeSQL, pgVec, ids).Scan(&rows).Error; err != nil {
			return fmt.Errorf("semantic tag cosine recompute: %w", err)
		}
		if elapsed := time.Since(recomputeStart); elapsed > semanticRecomputeWarnThreshold {
			logging.Warnf("search: semantic tag cosine recompute slow: %s (%d candidates)", elapsed, len(ids))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return rankMatchedTags(rows, cfg), nil
}

// rankMatchedTags 把候选的精确 cosine 归并成标签级结果（design D2）：同 tag
// 多 text_hash 行取 max、cosine ≥ MinCosine 过滤、cosine 降序（同分按 tagID
// 升序稳定）、截断 TopN。
func rankMatchedTags(rows []tagRecomputeRow, cfg SemanticConfig) []MatchedTag {
	best := make(map[uint]MatchedTag, len(rows))
	for _, row := range rows {
		// 用 !(>=) 而不是 <：NaN cosine（零范数 embedding 脏数据）也会被
		// 排除，与旧 SQL HAVING MAX(...) >= ? 的 NULL 语义一致。
		if !(row.Cosine >= cfg.MinCosine) {
			continue
		}
		if existing, ok := best[row.TagID]; ok && existing.Cosine >= row.Cosine {
			continue
		}
		best[row.TagID] = MatchedTag{TagID: row.TagID, Label: row.Label, Cosine: row.Cosine}
	}
	tags := make([]MatchedTag, 0, len(best))
	for _, tag := range best {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Cosine != tags[j].Cosine {
			return tags[i].Cosine > tags[j].Cosine
		}
		return tags[i].TagID < tags[j].TagID
	})
	if cfg.TopN > 0 && len(tags) > cfg.TopN {
		tags = tags[:cfg.TopN]
	}
	return tags
}

// matchTagsExact 旧全表精确扫描（design D2 回退路径）：维度超 halfvec 上限
// 或 ANN 查询失败时保持语义行为不变（同 tag 取 max、阈值、top_n 都由 SQL
// 聚合完成）。真库 14k 行实测 3.7s，可能吃满 3s 预算——所以只作兜底。
func (s *SemanticSearcher) matchTagsExact(ctx context.Context, pgVec string, cfg SemanticConfig) ([]MatchedTag, error) {
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
		return nil, fmt.Errorf("search semantic tags: %w", err)
	}
	matchedTags := make([]MatchedTag, len(tagRows))
	for i, row := range tagRows {
		matchedTags[i] = MatchedTag(row)
	}
	return matchedTags, nil
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
