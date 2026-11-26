package search

// add-article-search tasks 2.8 — articles 列表端点 search 过滤的 SQL
// 片段构造器（design D7：tsquery 分支替换为 D2 同款管线）。
//
// /api/search 走分层查询 + Go 聚合（repository.go），但列表端点的查询
// 结构（joins/分页/count）必须保持单条 SQL——此处把同一管线的各层
// 谓词 OR 组合为一条 WHERE 片段：L0 多词 AND-LIKE 组 / L1 词典词
// AND-LIKE 组 / L2 整串 LIKE /（PG）L3 similarity ≥ 阈值。
//
// OR 组合与分层查询的逐层 UNION 语义等价（每层组内仍保持该层的 AND
// 语义）。L3 用显式 similarity() >= 阈值而非 % 算子：% 受连接级 GUC
// pg_trgm.similarity_threshold 控制（分层路径靠事务内 set_config 设定，
// 此处无法包事务），显式分数谓词无 GUC 依赖、语义与分层路径一致
// （GIN 索引对 OR 组合中的 LIKE 臂仍可用 BitmapOr）。
//
// 归档口径由调用方保持（列表端点契约：默认排除归档——与搜索端点的
// 默认 include 互不影响）。

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

// likeCondition 单词命中 title 或 description 的谓词（转义已由预处理
// 完成，与 queryLikeAllTerms 同款子句）。
const likeCondition = "(articles.title LIKE ? ESCAPE '\\' OR articles.description LIKE ? ESCAPE '\\')"

// ListSearchWhere builds the WHERE fragment implementing the D2 pipeline
// for the articles LIST endpoint's postgres search filter. Returns
// ok=false when the query carries no usable condition（空/纯空白——调用方
// 不过滤，等同其它空过滤参数行为）。
func ListSearchWhere(ctx context.Context, db *gorm.DB, dict *DictCache, query string) (string, []interface{}, bool) {
	if strings.TrimSpace(query) == "" {
		return "", nil, false
	}
	pq, err := NewQueryPreprocessor(db, dict).Prepare(ctx, query)
	if err != nil {
		// Prepare 对非空输入仅可能因 ErrEmptyQuery（剥离空白后为空）失败
		return "", nil, false
	}

	var groups []string
	var args []interface{}

	// L0 多词 AND 组（单词时与 L2 重合，跳过——与 likeLayers 口径一致）
	if len(pq.L0Terms) > 1 {
		groups = append(groups, likeAndGroup(pq.L0Terms, &args))
	}
	// L1 词典词 AND 组
	if len(pq.L1Words) > 0 {
		groups = append(groups, likeAndGroup(pq.L1Words, &args))
	}
	// L2 整串
	whole := "%" + escapeLikePattern(pq.Raw) + "%"
	groups = append(groups, likeCondition)
	args = append(args, whole, whole)
	// L3 trigram（PG only；sqlite 列表分支走 handler 既有的整串 LIKE）。
	// 仅 title 列：真库实测 desc 列 recheck 单独 5.9~8.5s 且独占召回 0 行
	// （2026-10-14 决策，详见 repository.go mergeTrigramLayer 注释）。
	if db != nil && db.Name() == "postgres" {
		groups = append(groups, "similarity(articles.title, ?) >= ?")
		args = append(args, pq.Raw, pq.TrigramThreshold)
	}

	return "(" + strings.Join(groups, " OR ") + ")", args, true
}

// likeAndGroup builds one AND-layer group: every term must hit title or
// description（词可落不同字段）, appending the bound args.
func likeAndGroup(terms []MatchTerm, args *[]interface{}) string {
	conds := make([]string, len(terms))
	for i, term := range terms {
		pattern := "%" + term.Escaped + "%"
		conds[i] = likeCondition
		*args = append(*args, pattern, pattern)
	}
	return "(" + strings.Join(conds, " AND ") + ")"
}
