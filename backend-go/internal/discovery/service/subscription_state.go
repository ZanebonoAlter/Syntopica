package service

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"

	discmodels "syntopica-backend/internal/discovery/models"
	"syntopica-backend/internal/models"
)

// ── 来源订阅状态判定（fix-discovery-subscribed-hint，design D1–D3）──
//
// 判定规则：RSSHub 候选按「路由级固定前缀」推导——同一 namespace+path 模板下任意已填
// 参数的订阅都算该来源已订阅；原生 RSS 候选按规范化地址精确比对。全部纯读：不建
// 候选↔订阅关联、不写库（订阅真相始终是 feeds.url，design D1）。
//
// 该口径刻意与召回期资格过滤（discovery_recall.go:searchCandidates 仍按精确 feeds.url
// 去重）保持两套：展示层放宽到路由级提示，召回期不动（本 change Non-Goals）。
// 路由级命中只提示、不禁用订阅入口——是否重复由订阅动作按最终地址判定。

// routeFixedPrefix 取 route path 中首个 `:` 参数段之前的固定部分，末尾去 `/`：
// `/event/:category?` → `/event`，`/znews/:section?` → `/znews`，`/newsflash` → `/newsflash`，
// 裸 `/:id?` → 空前缀（订阅前缀即 `{base}/{namespace}`）。
//
// 参数段判定与 ParseRouteParameters 同口径：只有段首 `:` 才是参数段，段内冒号
// （如 `/foo:bar`）不算；`{regex}` 约束段永远跟在参数段之后，故首个 `:` 之前的
// 截断天然不受其影响（与 buildFeedURL 的 stripBraceConstraints 同一族路径语法）。
// namespace 参与路由身份语义，固定段本身只取决于 path，保留形参以对齐调用侧口径。
func routeFixedPrefix(namespace, path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] != ':' {
			continue
		}
		if i == 0 || path[i-1] == '/' {
			return strings.TrimRight(path[:i], "/")
		}
	}
	return strings.TrimRight(path, "/")
}

// routeSubscriptionPrefix 拼出候选的路由级订阅前缀：`{base}/{namespace}{fixedPath}`。
// base 尾部斜杠先去除（避免 `base//ns`）；path 为裸 `/:id?` 时前缀即 `{base}/{namespace}`。
func routeSubscriptionPrefix(baseURL, namespace, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + namespace + routeFixedPrefix(namespace, path)
}

// feedURLMatchesRoutePrefix 判定订阅地址是否落在路由前缀下（design D1）：
// 相等（可选参数缺省 = 裸路径）、`prefix+"/"`（参数以路径段形式填入）、
// `prefix+"?"`（参数以 query 形式挂在裸路径上）三种命中。
//
// MUST 带段边界：裸 HasPrefix 会让 `/zaobao/znews` 误命中 `/zaobao/znews2`。
// 空前缀（无 base/namespace 的退化输入）不匹配任何地址，避免通配一切。
func feedURLMatchesRoutePrefix(feedURL, prefix string) bool {
	if feedURL == "" || prefix == "" {
		return false
	}
	if feedURL == prefix {
		return true
	}
	return strings.HasPrefix(feedURL, prefix+"/") || strings.HasPrefix(feedURL, prefix+"?")
}

// SubscriptionIndex 是 feeds.url 的只读快照（design D3：一次装载、纯读匹配）。
// 字段不导出：装载后只经方法消费；测试可直接构造快照。
type SubscriptionIndex struct {
	urls []string
}

// LoadSubscriptionIndex 一次读取全部订阅地址（单用户量级几十~几百行；Go 侧做精确/
// 前缀匹配，不逐 view 拼 SQL、不用 LIKE，天然批量无 N+1）。查询失败向上返回：
// 调用方不得静默当作「未订阅」，那会伪造订阅状态。
func LoadSubscriptionIndex(ctx context.Context, db *gorm.DB) (*SubscriptionIndex, error) {
	var urls []string
	if err := db.WithContext(ctx).Model(&models.Feed{}).Order("url ASC").Pluck("url", &urls).Error; err != nil {
		return nil, err
	}
	return &SubscriptionIndex{urls: urls}, nil
}

// ExactMatches 返回与 url 精确相等的订阅地址（feeds.url 唯一，至多一条）。
// 无命中返回空切片（非 nil），调用方可直接序列化为 `[]`。
func (idx *SubscriptionIndex) ExactMatches(url string) []string {
	out := []string{}
	if idx == nil || url == "" {
		return out
	}
	for _, u := range idx.urls {
		if u == url {
			out = append(out, u)
		}
	}
	return out
}

// MatchRouteSubscriptions 在本响应的全部候选路由前缀 prefixes 内做订阅匹配：
// 返回 prefix → 命中该前缀的订阅地址（按 URL 字典序去重；无命中的 prefix 也返回空切片）。
//
// 最长前缀优先消歧（design D2）：同一地址命中多个前缀时只归入最长者——否则
// `/gov` 会抢占 `/gov/ndrc/fggz` 这类更具体路由的订阅。前缀集合必须一次给全
// （跨请求的极端不一致记入 design Risks）。
func (idx *SubscriptionIndex) MatchRouteSubscriptions(prefixes []string) map[string][]string {
	out := make(map[string][]string, len(prefixes))
	if idx == nil {
		return out
	}
	unique := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if _, ok := out[p]; ok {
			continue // 去重：同前缀只保留一个键
		}
		out[p] = []string{}
		unique = append(unique, p)
	}
	for _, u := range idx.urls {
		longest := ""
		for _, p := range unique {
			if !feedURLMatchesRoutePrefix(u, p) {
				continue
			}
			if len(p) > len(longest) {
				longest = p
			}
		}
		if longest != "" {
			out[longest] = append(out[longest], u)
		}
	}
	for p, urls := range out {
		out[p] = uniqueSorted(urls)
	}
	return out
}

// resolveCandidateSubscriptions 批量判定候选的已有订阅地址（返回 candidate_id → 地址，
// 无订阅为空切片）。rsshub 候选按路由级前缀（在本次调用的全部候选前缀内做最长前缀
// 优先），rss 候选按精确地址；route_id 缺失/路由缺失的 rsshub 候选保持未订阅。
// routes 为 route_id → 路由资料，调用方一次预载（禁 N+1）。
func resolveCandidateSubscriptions(
	index *SubscriptionIndex,
	baseURL string,
	cands []discmodels.FeedCandidate,
	routes map[uint]discmodels.RSSHubRoute,
) map[uint][]string {
	out := make(map[uint][]string, len(cands))
	prefixes := make([]string, 0, len(cands))
	prefixOf := make(map[uint]string, len(cands))
	for _, c := range cands {
		if c.Kind != "rsshub" || c.RouteID == nil {
			continue
		}
		route, ok := routes[*c.RouteID]
		if !ok {
			continue
		}
		p := routeSubscriptionPrefix(baseURL, route.Namespace, route.Path)
		prefixOf[c.ID] = p
		prefixes = append(prefixes, p)
	}
	matched := index.MatchRouteSubscriptions(prefixes)
	for _, c := range cands {
		urls := []string{}
		switch c.Kind {
		case "rsshub":
			if p, ok := prefixOf[c.ID]; ok {
				urls = matched[p]
			}
		case "rss":
			if c.FeedURL != nil {
				urls = index.ExactMatches(*c.FeedURL)
			}
		}
		if urls == nil {
			urls = []string{} // 空集合保持非 nil：JSON 为 [] 而非 null
		}
		out[c.ID] = urls
	}
	return out
}

// uniqueSorted 返回去重后的字典序切片；空输入返回空切片（非 nil）。
func uniqueSorted(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
