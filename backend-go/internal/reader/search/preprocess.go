package search

// add-article-search tasks 2.2 — 查询预处理四层管线（design D2，spec
// 「查询预处理四层管线」Requirement）。
//
//	L0 空白拆分（unicode.IsSpace，含全角 U+3000/tab）→ 多词 AND
//	L1 标签词典正向最大匹配切词 → 逐词 AND LIKE（title/description）
//	L2 整串子串 LIKE
//	L3 similarity() ≥ 阈值（repository 层，PG only）
//
// 本文件只产出各层描述（PreparedQuery），SQL 组装在 repository.go，
// 层级加权在 fusion.go。

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/logging"
)

// maxQueryRunes 查询截断上限（spec「特殊字符与超长查询」：>200 字符截断
// + warn 不报错；rune 计非字节）。
const maxQueryRunes = 200

// ErrEmptyQuery 空/纯空白/剥离空白后无有效字符查询的哨兵错误：调用方
// （handler，tasks 2.6）转 400 且不产生任何召回动作
// （spec「空与纯空白查询拒绝」）。
var ErrEmptyQuery = errors.New("search: query is empty or whitespace-only")

// MatchTerm 一个查询词的两种形态：Text 供 hit_reasons 展示与 Go 侧字段
// 归属判定；Escaped 供 SQL LIKE 拼接（% _ \ 已转义，配 ESCAPE '\' 子句）。
type MatchTerm struct {
	Text    string
	Escaped string
}

// PreparedQuery 四层管线的输出：携带各层信息供 repository 组 SQL、
// fusion 层加权（层级判定见 fusion.go MatchLevel）。
//
// 层级归属约定（design D4 只定义四档系数，L0 的归档为实现决策）：
//   - L2 整串命中       → LevelWhole（系数 1.0）
//   - L0 多词 / L1 多词 → LevelDictMulti（系数 0.85）：L0 多词与词典多词
//     的匹配语义相同（全部词出现、位置可分散），归同档
//   - L1 单词           → LevelDictSingle（系数 0.6，补偿单词放宽召回）
//   - L3               → LevelTrigram（原始 similarity 作层级分）
type PreparedQuery struct {
	// Raw 截断并去首尾空白后的查询串：L2 整串 LIKE 与 L3 trigram 的输入
	// （未转义——trigram 参数需要原文）。
	Raw string
	// L0Terms 空白拆分词。len == 1 时与 L2 语义重合（整串即该词），
	// repository 跳过 L0 层避免重复谓词。
	L0Terms []MatchTerm
	// L1Words 词典切出的词；词典未加载/加载失败/无命中时为空（L1 自动跳过）。
	L1Words []MatchTerm
	// TrigramThreshold L3 阈值（配置非法时 loader 已回退默认）。
	TrigramThreshold float64
}

// QueryPreprocessor 四层管线执行器。TrigramThreshold 注入点供函数单测
// 替换（免 DB）；生产经 NewQueryPreprocessor 接 ai_settings。
type QueryPreprocessor struct {
	dict                 *DictCache
	loadTrigramThreshold func(ctx context.Context) float64
}

// NewQueryPreprocessor wires the production config source (ai_settings via
// LoadTrigramThreshold) and the shared dictionary cache.
func NewQueryPreprocessor(db *gorm.DB, dict *DictCache) *QueryPreprocessor {
	return &QueryPreprocessor{
		dict: dict,
		loadTrigramThreshold: func(ctx context.Context) float64 {
			return LoadTrigramThreshold(ctx, db)
		},
	}
}

// Prepare runs the shared prefix of the four-layer pipeline (validate,
// truncate, split, dict-segment, threshold). L3's similarity evaluation is
// deferred to the repository (PG only); the threshold rides on the result.
func (p *QueryPreprocessor) Prepare(ctx context.Context, query string) (*PreparedQuery, error) {
	// TrimSpace 按 unicode.IsSpace 剥离（含全角 U+3000 与 tab）。
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, ErrEmptyQuery
	}
	if runes := []rune(trimmed); len(runes) > maxQueryRunes {
		logging.Warnf("search: query exceeds %d runes (%d); truncated", maxQueryRunes, len(runes))
		trimmed = strings.TrimSpace(string(runes[:maxQueryRunes]))
	}

	pq := &PreparedQuery{Raw: trimmed}

	// L0：空白拆分（strings.Fields 同样按 unicode.IsSpace 切）。
	for _, term := range strings.Fields(trimmed) {
		pq.L0Terms = append(pq.L0Terms, MatchTerm{Text: term, Escaped: escapeLikePattern(term)})
	}

	// L1：词典正向最大匹配。加载失败静默降级（红线：搜索关键词路径必须
	// 永远在线，词典缺席只是 L1 空切，L0/L2/L3 照常）。
	if p.dict != nil {
		if err := p.dict.EnsureLoaded(ctx); err != nil {
			logging.Warnf("search: dict load failed, L1 skipped: %v", err)
		} else {
			for _, w := range p.dict.Segment(trimmed) {
				pq.L1Words = append(pq.L1Words, MatchTerm{Text: w, Escaped: escapeLikePattern(w)})
			}
		}
	}

	pq.TrigramThreshold = p.loadTrigramThreshold(ctx)
	return pq, nil
}

// escapeLikePattern 转义 LIKE 通配符（%/_/反斜杠），配合 ESCAPE '\' 子句在
// PostgreSQL 与 SQLite 上行为一致。模式复制自 internal/discovery/service
// （candidate_catalog_service.go escapeLikePattern，discovery 包私有且跨域
// 深路径 import 被 depguard 禁止，此处本地实现；change add-article-search
// tasks 2.2 明示「复制进 search 包勿 import」）。
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
