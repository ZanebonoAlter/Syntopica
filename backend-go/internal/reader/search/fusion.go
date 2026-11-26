package search

// add-article-search tasks 2.5 — RRF 融合 + 命中层级加权（design D4）。
//
// 基础 RRF：score = Σ 1/(60 + rank_i)，关键词榜与语义榜（与预留的扩展榜）
// 各自排名后求和。关键词榜内部排序分 = 字段权重（title 1.0 / description
// 0.7）× 层级系数（整串 1.0 / 词典多词 0.85 / 词典单词 0.6 / trigram 原始
// similarity）——补偿单词 AND 放宽召回的精度损失（多词命中 > 单词命中）。

import "sort"

// MatchLevel 命中层级（design D4 四档系数；L0 多词归 DictMulti 见
// preprocess.go 的层级归属约定）。
type MatchLevel string

const (
	// LevelWhole L2 整串子串命中（系数 1.0）。
	LevelWhole MatchLevel = "whole"
	// LevelDictMulti L0 多词 / L1 词典多词 AND 命中（系数 0.85）。
	LevelDictMulti MatchLevel = "dict-multi"
	// LevelDictSingle L1 词典单词命中（系数 0.6）。
	LevelDictSingle MatchLevel = "dict-single"
	// LevelTrigram L3 trigram 命中（无固定系数，用原始 similarity）。
	LevelTrigram MatchLevel = "trigram"
)

// 字段权重与层级系数（design D4 数值表；test-cases「命中层级加权排序断言」）。
const (
	fieldWeightTitle       = 1.0
	fieldWeightDescription = 0.7
	coefWhole              = 1.0
	coefDictMulti          = 0.85
	coefDictSingle         = 0.6
	rrfK                   = 60
)

// HitReasonType 命中解释类型（spec「混合搜索聚合端点」：keyword|semantic|expand）。
const (
	ReasonKeyword  = "keyword"
	ReasonSemantic = "semantic"
	ReasonExpand   = "expand"
)

// HitReason 单条命中解释（响应契约 hit_reasons 元素：{type, text, score}）。
type HitReason struct {
	Type  string  `json:"type"`
	Text  string  `json:"text"`
	Score float64 `json:"score"`
}

// RankEntry 单路榜单条目：Score 决定该路名次（RRF 的 rank 来源），Reasons
// 为该路对该文章的命中解释。
type RankEntry struct {
	ArticleID uint
	Score     float64
	Reasons   []HitReason
}

// FuseInput 三路榜单输入；Expand 榜预留（tasks 2.7 LLM 扩展，type=expand）。
type FuseInput struct {
	Keyword  []RankEntry
	Semantic []RankEntry
	Expand   []RankEntry
}

// FusedResult 融合排序输出项。
type FusedResult struct {
	ArticleID uint
	// Score RRF 融合分 Σ 1/(60 + rank_i)。
	Score float64
	// KeywordScore / SemanticScore 各路榜内最高分（并列时的次级排序键）。
	KeywordScore  float64
	SemanticScore float64
	// HitReasons 同文章多路命中合并后的聚合解释（type+text 去重保最高分）。
	HitReasons []HitReason
}

// KeywordScore 计算一篇文章关键词榜内部排序分：各命中明细取
// 字段权重 × 层级系数（trigram 用原始 similarity）的最大值。
func KeywordScore(details []HitDetail) float64 {
	best := 0.0
	for _, d := range details {
		if w := hitWeight(d); w > best {
			best = w
		}
	}
	return best
}

// hitWeight 单条命中的加权分（design D4）。
func hitWeight(d HitDetail) float64 {
	field := fieldWeightTitle
	if d.Field == FieldDescription {
		field = fieldWeightDescription
	}
	switch d.Level {
	case LevelWhole:
		return field * coefWhole
	case LevelDictMulti:
		return field * coefDictMulti
	case LevelDictSingle:
		return field * coefDictSingle
	case LevelTrigram:
		return field * d.TrigramScore
	default:
		return 0
	}
}

// KeywordRankEntries 把关键词路径命中（repository 输出）转为关键词榜单：
// Score = KeywordScore(hits)；Reasons 逐命中明细展开（keyword 型）。
// 返回已按 Score 排名的榜单（榜单即名次，Fuse 会再稳定排一次，同序无害）。
func KeywordRankEntries(hits []ArticleHit) []RankEntry {
	entries := make([]RankEntry, 0, len(hits))
	for _, h := range hits {
		reasons := make([]HitReason, 0, len(h.Hits))
		for _, d := range h.Hits {
			reasons = append(reasons, HitReason{Type: ReasonKeyword, Text: d.Text, Score: hitWeight(d)})
		}
		entries = append(entries, RankEntry{
			ArticleID: h.Article.ID,
			Score:     KeywordScore(h.Hits),
			Reasons:   reasons,
		})
	}
	return sortedRankEntries(entries)
}

// SemanticRankEntries 把语义路径命中转为语义榜：Score = 文章语义分
// （max cosine）；Reasons 携带全量命中标签（design D3：命中标签全量进
// hit_reasons）。返回已按 Score 排名的榜单。
func SemanticRankEntries(hits []SemanticHit) []RankEntry {
	entries := make([]RankEntry, 0, len(hits))
	for _, h := range hits {
		reasons := make([]HitReason, 0, len(h.MatchedTags))
		for _, tag := range h.MatchedTags {
			reasons = append(reasons, HitReason{Type: ReasonSemantic, Text: tag.Label, Score: tag.Cosine})
		}
		entries = append(entries, RankEntry{
			ArticleID: h.Article.ID,
			Score:     h.Cosine,
			Reasons:   reasons,
		})
	}
	return sortedRankEntries(entries)
}

// Fuse 三路 RRF 融合。每路独立按 Score 排名（降序，平分按 ArticleID 升序
// 保证确定性），命中的文章累计 1/(60+rank)；同文章多路命中合并进同一
// FusedResult 的 HitReasons（type+text 去重、保留最高分）。最终排序：
// 融合分降序 → 关键词分降序 → 语义分降序 → ArticleID 升序。
func Fuse(input FuseInput) []FusedResult {
	acc := make(map[uint]*FusedResult)
	get := func(id uint) *FusedResult {
		a := acc[id]
		if a == nil {
			a = &FusedResult{ArticleID: id}
			acc[id] = a
		}
		return a
	}

	addList := func(entries []RankEntry, keyword, semantic bool) {
		sorted := sortedRankEntries(entries)
		for i, e := range sorted {
			rank := i + 1 // 1-indexed rank
			a := get(e.ArticleID)
			a.Score += 1 / float64(rrfK+rank)
			if keyword && e.Score > a.KeywordScore {
				a.KeywordScore = e.Score
			}
			if semantic && e.Score > a.SemanticScore {
				a.SemanticScore = e.Score
			}
			for _, r := range e.Reasons {
				mergeReason(a, r)
			}
		}
	}
	addList(input.Keyword, true, false)
	addList(input.Semantic, false, true)
	addList(input.Expand, false, false)

	out := make([]FusedResult, 0, len(acc))
	for _, a := range acc {
		out = append(out, *a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].KeywordScore != out[j].KeywordScore {
			return out[i].KeywordScore > out[j].KeywordScore
		}
		if out[i].SemanticScore != out[j].SemanticScore {
			return out[i].SemanticScore > out[j].SemanticScore
		}
		return out[i].ArticleID < out[j].ArticleID
	})
	return out
}

// sortedRankEntries 按 Score 降序、ArticleID 升序返回副本（不改入参顺序）。
func sortedRankEntries(entries []RankEntry) []RankEntry {
	sorted := make([]RankEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		return sorted[i].ArticleID < sorted[j].ArticleID
	})
	return sorted
}

// mergeReason 并入一条命中解释：同 (type, text) 已存在则保留最高分，
// 否则追加（同文章多路命中聚合，spec hit_reasons 契约）。
func mergeReason(a *FusedResult, r HitReason) {
	for i := range a.HitReasons {
		existing := &a.HitReasons[i]
		if existing.Type == r.Type && existing.Text == r.Text {
			if r.Score > existing.Score {
				existing.Score = r.Score
			}
			return
		}
	}
	a.HitReasons = append(a.HitReasons, r)
}
