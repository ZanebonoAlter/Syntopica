package search

// add-search-sort-modes tasks 1.1 — 搜索结果排序模式（design D1~D4；
// spec「搜索结果排序模式」）。
//
// 排序发生在 RRF 融合之后、offset 分页之前：Fuse 保持纯 RRF 职责，
// 本文件只对已融合榜单做投影排序（design D3）。两种模式返回同一结果集
// （只改变顺序，不过滤、不重算分数）。

import (
	"sort"
	"time"
)

// SortMode 结果排序模式（spec：date 默认｜relevance）。
type SortMode string

const (
	// SortByDate 按 Asia/Shanghai 日历日倒序分组，同日内按融合相关度。
	SortByDate SortMode = "date"
	// SortByRelevance 保持现有 RRF 融合相关度顺序（旧行为）。
	SortByRelevance SortMode = "relevance"
)

// searchSortZone 排序日界固定 UTC+8（design D2：Asia/Shanghai 无夏令时，
// 用固定偏移，不依赖宿主机 tzdata）。
var searchSortZone = time.FixedZone("CST", 8*60*60)

// ParseSortMode 解析 sort_by 参数：relevance 显式切换；缺省/空串/非法值
// 静默回退 date（枚举类回退惯例，不报错——spec「非法排序值回退默认」）。
func ParseSortMode(raw string) SortMode {
	if SortMode(raw) == SortByRelevance {
		return SortByRelevance
	}
	return SortByDate
}

// sortDayKey 时间的 UTC+8 日历日键：同一天返回同值，跨日单调递增
// （y*10000+m*100+d，跨月/跨年仍有序）。零值时间自然得到最小键，
// 排到最后（防御路径）。
func sortDayKey(t time.Time) int64 {
	y, m, d := t.In(searchSortZone).Date()
	return int64(y)*10000 + int64(m)*100 + int64(d)
}

// effectiveTime 排序实际时间：pub_date 为空回退 created_at
// （spec「空发布时间回退创建时间」）；两者皆零值时为零值时间。
func effectiveTime(row ArticleSearchRow) time.Time {
	if row.PubDate != nil {
		return *row.PubDate
	}
	return row.CreatedAt
}

// sortKey 单篇文章的预计算排序键（避免比较器内重复时区转换）。
type sortKey struct {
	day int64
	at  time.Time
}

// SortFused 对融合榜单排序：relevance 原样返回（引用与顺序不变）；
// date 复制后按「日倒序 → RRF 分 → 关键词分 → 语义分 → 精确时间倒序 →
// ArticleID 升序」排序（design D4，全序确定保证分页稳定）。入参不被修改；
// rows 缺失的 ID 用零值时间参与比较，自然排最后。
func SortFused(fused []FusedResult, rows map[uint]ArticleSearchRow, mode SortMode) []FusedResult {
	if mode != SortByDate {
		return fused
	}

	out := make([]FusedResult, len(fused))
	copy(out, fused)

	keys := make(map[uint]sortKey, len(out))
	for _, fr := range out {
		at := effectiveTime(rows[fr.ArticleID])
		keys[fr.ArticleID] = sortKey{day: sortDayKey(at), at: at}
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := keys[out[i].ArticleID], keys[out[j].ArticleID]
		if a.day != b.day {
			return a.day > b.day
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].KeywordScore != out[j].KeywordScore {
			return out[i].KeywordScore > out[j].KeywordScore
		}
		if out[i].SemanticScore != out[j].SemanticScore {
			return out[i].SemanticScore > out[j].SemanticScore
		}
		if !a.at.Equal(b.at) {
			return a.at.After(b.at)
		}
		return out[i].ArticleID < out[j].ArticleID
	})
	return out
}
