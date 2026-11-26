package search

// add-article-search tasks 2.3 — 关键词路径 repository（design D1/D2）。
//
// PG 路径：各层 LIKE（走 pg_trgm GIN 索引）+ similarity() ≥ 阈值，分层
// 查询、Go 侧聚合（实现选择：单条大 SQL 的每层 EXISTS/布尔列会引入随词数
// 变化的动态列名/占位符拼装，可读性差；分层查询各自结构固定、各自可独立
// 降级，命中明细在 Go 聚合——见汇报）。
//
// 归档口径（spec「归档文章默认含入搜索」）：include 默认不过滤 archived；
// active-only 加 archived = false。
//
// 非 PG（sqlite 测试库）：仅 L0/L1/L2 LIKE 路径（无 similarity，方言分支
// 风格与既有 article_handler 一致）。

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// 归档口径参数（spec：include | active-only，默认 include）。
const (
	ArchiveInclude    = "include"
	ArchiveActiveOnly = "active-only"
)

// 搜索行投影的 excerpt 源字段拉取量固定 2000 字符（见 articleSearchSelect
// 的 substr 上限与 ArticleSearchRow 注释；照 handler excerptsByArticleID
// 的有界源模式——content 平均 4KB，全量下发违反 article-list-projection
// 窄投影契约：正文类大字段不出列表，导语 ≤200 字符纯文本）。

// ArticleSearchRow 搜索结果行投影：articleListColumns 的窄投影镜像
// （article_handler.go）+ 有界 excerpt 源字段。不含 content/
// firecrawl_content/ai_content_summary 大字段（article-list-projection
// 契约沿用）；excerpt 的最终 ≤200 字符纯文本构建在 handler 层
// （buildExcerpt 属 reader/handler，本包 import 会成环——源字段模式照
// excerptSourceRow 先例）。Excerpt* 字段兼作 LIKE 命中字段归属判定的
// 输入（attributeField）。
type ArticleSearchRow struct {
	ID                 uint       `gorm:"column:id"`
	FeedID             uint       `gorm:"column:feed_id"`
	Title              string     `gorm:"column:title"`
	Link               string     `gorm:"column:link"`
	ImageURL           string     `gorm:"column:image_url"`
	PubDate            *time.Time `gorm:"column:pub_date"`
	Author             string     `gorm:"column:author"`
	Read               bool       `gorm:"column:read"`
	Favorite           bool       `gorm:"column:favorite"`
	Archived           bool       `gorm:"column:archived"`
	SummaryStatus      string     `gorm:"column:summary_status"`
	SummaryGeneratedAt *time.Time `gorm:"column:summary_generated_at"`
	FirecrawlStatus    string     `gorm:"column:firecrawl_status"`
	FirecrawlError     string     `gorm:"column:firecrawl_error"`
	FirecrawlCrawledAt *time.Time `gorm:"column:firecrawl_crawled_at"`
	CompletionError    string     `gorm:"column:completion_error"`
	CreatedAt          time.Time  `gorm:"column:created_at"`

	ExcerptDescription string `gorm:"column:excerpt_description"`
	ExcerptContent     string `gorm:"column:excerpt_content"`
	NoFirecrawlBody    int    `gorm:"column:no_firecrawl_body"`
}

// articleSearchSelect 与 ArticleSearchRow 一一对应的投影子句
// （列集 = articleListColumns + 有界 excerpt 源）。
const articleSearchSelect = "articles.id, articles.feed_id, articles.title, articles.link, articles.image_url, articles.pub_date, articles.author, articles.read, articles.favorite, articles.archived, articles.summary_status, articles.summary_generated_at, articles.firecrawl_status, articles.firecrawl_error, articles.firecrawl_crawled_at, articles.completion_error, articles.created_at, substr(articles.description, 1, 2000) AS excerpt_description, substr(articles.content, 1, 2000) AS excerpt_content, CASE WHEN articles.firecrawl_content = '' THEN 1 ELSE 0 END AS no_firecrawl_body"

// HitField 命中字段。
type HitField string

const (
	FieldTitle       HitField = "title"
	FieldDescription HitField = "description"
)

// HitDetail 单层单词的命中明细：层级（fusion 加权档）、最佳命中字段、
// 命中文本（hit_reasons 展示原文）、L3 原始 similarity 分。
type HitDetail struct {
	Level MatchLevel
	Field HitField
	Text  string
	// TrigramScore 仅 Level == LevelTrigram 时有意义。
	TrigramScore float64
}

// ArticleHit 一篇文章的关键词路径聚合命中：行投影 + 各层命中明细。
type ArticleHit struct {
	Article ArticleSearchRow
	Hits    []HitDetail
}

// SearchRepository 关键词路径数据访问。
type SearchRepository struct {
	db *gorm.DB
}

// NewSearchRepository builds a SearchRepository over the given DB
// （postgres 走 trigram 全层，sqlite 走 LIKE 子集）。
func NewSearchRepository(db *gorm.DB) *SearchRepository {
	return &SearchRepository{db: db}
}

// likeLayer 一个待执行的 LIKE 层描述。
type likeLayer struct {
	terms []MatchTerm
	level MatchLevel
}

// SearchKeyword executes the keyword path: L0 (when multi-word) / L1 / L2 LIKE layers
// plus the L3 trigram layer (PG only), aggregating hit details per article.
// LIKE core-layer failure returns an error（关键词路径是主力，静默吞错会让
// 「空结果」伪装成「无命中」）；L3 failure only warns and degrades（红线：
// 搜索关键词路径必须永远在线，增强层失败不得让搜索整体报错）。
func (r *SearchRepository) SearchKeyword(ctx context.Context, pq *PreparedQuery, archived string) ([]ArticleHit, error) {
	merged, err := r.SearchKeywordLiteral(ctx, pq, archived)
	if err != nil {
		return nil, err
	}

	// L3 trigram 兜底（PG only；sqlite 测试库无 pg_trgm，方言分支与
	// article_handler 的 Name() 判定风格一致）。注意：trigram 层会补入
	// 仅它命中的新文章——最终输出必须从 byID 汇总，不能直接返回 literal 切片。
	if r.db.Name() != "postgres" {
		return merged, nil
	}
	byID := make(map[uint]*ArticleHit, len(merged))
	for i := range merged {
		byID[merged[i].Article.ID] = &merged[i]
	}
	if err := r.mergeTrigramLayer(ctx, pq, archived, byID); err != nil {
		logging.Warnf("search: trigram layer failed, degraded to LIKE-only: %v", err)
	}

	out := make([]ArticleHit, 0, len(byID))
	for _, hit := range byID {
		out = append(out, *hit)
	}
	return out, nil
}

// SearchKeywordLiteral runs only the LIKE layers（L0 多词时 / L1 词典 / L2
// 整串）without the L3 trigram layer. Used by the expand path（tasks 2.7，
// spec：扩词只补跑 L1/L2 字面召回，不叠加 trigram 模糊层）。
func (r *SearchRepository) SearchKeywordLiteral(ctx context.Context, pq *PreparedQuery, archived string) ([]ArticleHit, error) {
	if pq == nil || pq.Raw == "" {
		return nil, fmt.Errorf("search: nil or empty prepared query")
	}

	layers := r.likeLayers(pq)
	merged := make(map[uint]*ArticleHit)
	for _, layer := range layers {
		rows, err := r.queryLikeAllTerms(ctx, layer.terms, archived)
		if err != nil {
			return nil, fmt.Errorf("search keyword layer %s: %w", layer.level, err)
		}
		for i := range rows {
			hit := merged[rows[i].ID]
			if hit == nil {
				hit = &ArticleHit{Article: rows[i]}
				merged[rows[i].ID] = hit
			}
			for _, term := range layer.terms {
				hit.Hits = append(hit.Hits, HitDetail{
					Level: layer.level,
					Field: attributeField(&hit.Article, term.Text),
					Text:  term.Text,
				})
			}
		}
	}

	out := make([]ArticleHit, 0, len(merged))
	for _, hit := range merged {
		out = append(out, *hit)
	}
	return out, nil
}

// likeLayers 组装 LIKE 层：L0 多词（单词与 L2 重合，跳过）、L1 词典
// （多词/单词档）、L2 整串。
func (r *SearchRepository) likeLayers(pq *PreparedQuery) []likeLayer {
	layers := make([]likeLayer, 0, 3)
	if len(pq.L0Terms) > 1 {
		layers = append(layers, likeLayer{terms: pq.L0Terms, level: LevelDictMulti})
	}
	if len(pq.L1Words) > 0 {
		level := LevelDictSingle
		if len(pq.L1Words) > 1 {
			level = LevelDictMulti
		}
		layers = append(layers, likeLayer{terms: pq.L1Words, level: level})
	}
	layers = append(layers, likeLayer{
		terms: []MatchTerm{{Text: pq.Raw, Escaped: escapeLikePattern(pq.Raw)}},
		level: LevelWhole,
	})
	return layers
}

// queryLikeAllTerms 一层 AND-(title|description) LIKE 查询：每个词必须命中
// title 或 description（词可落不同字段）。转义已由预处理完成，此处只拼
// ESCAPE '\' 子句（escapeLikePattern 模式，PG/SQLite 行为一致）。
func (r *SearchRepository) queryLikeAllTerms(ctx context.Context, terms []MatchTerm, archived string) ([]ArticleSearchRow, error) {
	conds := make([]string, len(terms))
	args := make([]interface{}, 0, len(terms)*2)
	for i, term := range terms {
		pattern := "%" + term.Escaped + "%"
		conds[i] = "(articles.title LIKE ? ESCAPE '\\' OR articles.description LIKE ? ESCAPE '\\')"
		args = append(args, pattern, pattern)
	}

	query := r.db.WithContext(ctx).Table("articles").
		Select(articleSearchSelect).
		Where(strings.Join(conds, " AND "), args...)
	if archived == ArchiveActiveOnly {
		query = query.Where("articles.archived = ?", false)
	}

	var rows []ArticleSearchRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// trigramRow L3 行：行投影 + 双字段 similarity 分。
type trigramRow struct {
	ArticleSearchRow
	TitleSim float64 `gorm:"column:title_sim"`
	DescSim  float64 `gorm:"column:desc_sim"` // 已停用（L3 title-only），保留列映射兼容既有行结构
}

// mergeTrigramLayer 执行 L3 并聚合进 merged：`%` 相似度算子按事务内
// pg_trgm.similarity_threshold 走 GIN 索引圈候选，similarity() 出显式分数，
// Go 侧再按配置阈值防御性复验（GUC 与显式阈值同值，双保险）。
//
// L3 仅作用于 title 列（2026-10-14 真库实测决策，add-article-search）：
// description 列的 GIN % recheck 需对数万候选行 detoast，单层 5.9~8.5s
// （树莓派 SD 卡 I/O），而三个实测查询的 desc-L3 独占召回（非 LIKE 层
// 已覆盖）均为 0 行——成本换零召回，砍掉；title-L3 走索引仅 ~95ms 且
// 连续短语场景有真实独占兜底价值（spec P50<1s 目标）。desc 的字面命中
// 由 L0-L2 LIKE 层覆盖（同样走 trgm 索引，无性能问题）。
func (r *SearchRepository) mergeTrigramLayer(ctx context.Context, pq *PreparedQuery, archived string, merged map[uint]*ArticleHit) error {
	var rows []trigramRow
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// set_config(..., true) 等价 SET LOCAL：作用域限本事务（连接池安全，
		// 不会污染池内其它连接）；% 算子按该阈值用 gin_trgm_ops 索引。
		if err := tx.Exec("SELECT set_config('pg_trgm.similarity_threshold', ?, true)",
			strconv.FormatFloat(pq.TrigramThreshold, 'f', -1, 64)).Error; err != nil {
			return err
		}
		query := tx.Table("articles").
			Select(articleSearchSelect+", similarity(articles.title, ?) AS title_sim", pq.Raw).
			Where("articles.title % ?", pq.Raw)
		if archived == ArchiveActiveOnly {
			query = query.Where("articles.archived = ?", false)
		}
		return query.Scan(&rows).Error
	})
	if err != nil {
		return err
	}

	for i := range rows {
		row := &rows[i]
		score, field := row.TitleSim, FieldTitle
		if score < pq.TrigramThreshold {
			continue // 防御性复验：% 算子已过滤，此处兜 float 精度
		}
		hit := merged[row.ID]
		if hit == nil {
			hit = &ArticleHit{Article: row.ArticleSearchRow}
			merged[row.ID] = hit
		}
		hit.Hits = append(hit.Hits, HitDetail{
			Level:        LevelTrigram,
			Field:        field,
			Text:         pq.Raw,
			TrigramScore: score,
		})
	}
	return nil
}

// attributeField 判定命中字段：title 全量在投影内可直接 strings.Contains
// （与 LIKE '%escaped%' ESCAPE '\' 对原文的语义等价）；description 只取了
// 有界前缀，词可能落在界外——届时回退 description（该层的 SQL 谓词已保证
// 词命中 title 或完整 description 之一）。
func attributeField(row *ArticleSearchRow, text string) HitField {
	if strings.Contains(row.Title, text) {
		return FieldTitle
	}
	return FieldDescription
}
