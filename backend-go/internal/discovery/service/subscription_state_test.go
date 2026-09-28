package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	discmodels "syntopica-backend/internal/discovery/models"
	"syntopica-backend/internal/models"
)

// ── fix-discovery-subscribed-hint 1.2（design D1/D2/D6）：订阅状态判定边界 ──
//
// 纯函数零 IO，SQLite 免 DB；仅 LoadSubscriptionIndex 用内存 SQLite 验证一次
// SELECT url FROM feeds。

// TestRouteFixedPrefix 覆盖路径模板族：无参、尾部可选参、必填参、regex 约束段、
// 裸 /:id?（前缀为空）、段内冒号非参数段。
func TestRouteFixedPrefix(t *testing.T) {
	cases := []struct {
		name      string
		namespace string
		path      string
		want      string
	}{
		{"无参路径", "zaobao", "/newsflash", "/newsflash"},
		{"尾部可选参", "zaobao", "/event/:category?", "/event"},
		{"尾部可选参（另一形态）", "zaobao", "/znews/:section?", "/znews"},
		{"必填参", "zaobao", "/news/:category/:locale", "/news"},
		{"regex 约束段", "gov", "/fggz/:category{.+}?", "/fggz"},
		{"裸可选参", "test", "/:id?", ""},
		{"裸必填参", "test", "/:id", ""},
		{"根路径", "test", "/", ""},
		{"空路径", "test", "", ""},
		{"尾斜杠去除", "test", "/newsflash/", "/newsflash"},
		{"段内冒号非参数段", "test", "/foo:bar/baz", "/foo:bar/baz"},
		{"段内冒号后才是参数段", "test", "/foo:bar/:id", "/foo:bar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, routeFixedPrefix(tc.namespace, tc.path))
		})
	}
}

// TestRouteSubscriptionPrefix 前缀拼装：base 尾斜杠去除、裸参路由前缀落到 namespace。
func TestRouteSubscriptionPrefix(t *testing.T) {
	require.Equal(t,
		"https://rsshub.app/zaobao/znews",
		routeSubscriptionPrefix("https://rsshub.app", "zaobao", "/znews/:section?"))
	require.Equal(t,
		"https://rsshub.app/zaobao/znews",
		routeSubscriptionPrefix("https://rsshub.app/", "zaobao", "/znews/:section?"),
		"base 尾斜杠去除，不产生双斜杠")
	require.Equal(t,
		"http://47.110.71.194:1200/test",
		routeSubscriptionPrefix("http://47.110.71.194:1200", "test", "/:id?"),
		"裸参路由前缀 = base/namespace")
}

// TestFeedURLMatchesRoutePrefix 三种命中形态 + 段边界反例 + 退化输入。
func TestFeedURLMatchesRoutePrefix(t *testing.T) {
	const prefix = "https://rsshub.app/zaobao/znews"
	cases := []struct {
		name    string
		feedURL string
		prefix  string
		want    bool
	}{
		{"相等（可选参缺省 = 裸路径）", prefix, prefix, true},
		{"尾斜杠形态", prefix + "/", prefix, true},
		{"子段（参数以路径段填入）", prefix + "/world", prefix, true},
		{"更深子路径按段边界命中", prefix + "/world/extra", prefix, true},
		{"query 形态（参数挂裸路径）", prefix + "?section=world", prefix, true},
		{"段边界反例：/znews2 不命中 /znews", prefix + "2", prefix, false},
		{"段边界反例：更长路径段碰前缀", "https://rsshub.app/zaobao/znews2/world", prefix, false},
		{"短串不命中", "https://rsshub.app/zaobao/znew", prefix, false},
		{"同 path 异 host 不命中", "https://example.com/zaobao/znews", prefix, false},
		{"空前缀不匹配任何地址", prefix, "", false},
		{"空地址不命中", "", prefix, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, feedURLMatchesRoutePrefix(tc.feedURL, tc.prefix))
		})
	}
}

// TestSubscriptionIndexExactMatches 原生精确匹配：同 path 异 host、尾斜杠、空地址都不命中。
func TestSubscriptionIndexExactMatches(t *testing.T) {
	idx := &SubscriptionIndex{urls: []string{
		"https://rsshub.app/zaobao/znews/world",
		"https://example.com/feed.xml",
	}}
	require.Equal(t, []string{"https://example.com/feed.xml"}, idx.ExactMatches("https://example.com/feed.xml"))
	require.Equal(t, []string{}, idx.ExactMatches("https://other.example.com/feed.xml"), "同 path 异 host 不误伤")
	require.Equal(t, []string{}, idx.ExactMatches("https://rsshub.app/zaobao/znews/world/"), "精确口径不做尾斜杠模糊")
	require.Equal(t, []string{}, idx.ExactMatches(""), "空地址不命中")
}

// TestSubscriptionIndexMatchRouteSubscriptions 最长前缀优先消歧 + 结果排序去重 +
// 无命中的前缀也返回空切片（非 nil）。
func TestSubscriptionIndexMatchRouteSubscriptions(t *testing.T) {
	idx := &SubscriptionIndex{urls: []string{
		"https://rsshub.app/gov/x",
		"https://rsshub.app/gov/ndrc/fggz/energy",
		"https://rsshub.app/zaobao/znews/world",
	}}
	got := idx.MatchRouteSubscriptions([]string{
		"https://rsshub.app/gov",
		"https://rsshub.app/gov/ndrc/fggz",
		"https://rsshub.app/zaobao/znews",
		"https://rsshub.app/gov", // 重复前缀只保留一个键
		"https://rsshub.app/unmatched",
	})
	require.Equal(t, []string{"https://rsshub.app/gov/x"}, got["https://rsshub.app/gov"],
		"被更长前缀抢占的地址不再归入短前缀")
	require.Equal(t, []string{"https://rsshub.app/gov/ndrc/fggz/energy"}, got["https://rsshub.app/gov/ndrc/fggz"],
		"同一地址命中多个前缀时只算最长者")
	require.Equal(t, []string{"https://rsshub.app/zaobao/znews/world"}, got["https://rsshub.app/zaobao/znews"])
	require.NotNil(t, got["https://rsshub.app/unmatched"])
	require.Equal(t, []string{}, got["https://rsshub.app/unmatched"], "无命中的前缀返回空切片")
	require.Len(t, got, 4, "重复前缀去重后只保留 4 个键")

	require.Empty(t, idx.MatchRouteSubscriptions(nil), "空前缀集合返回空 map")
	emptyIdx := (&SubscriptionIndex{}).MatchRouteSubscriptions([]string{"https://rsshub.app/x"})
	require.NotNil(t, emptyIdx["https://rsshub.app/x"])
	require.Empty(t, emptyIdx["https://rsshub.app/x"], "空索引不产生命中（前缀键仍返回空切片）")

	// 排序去重：乱序输入快照 → 结果按 URL 字典序。
	unsorted := &SubscriptionIndex{urls: []string{
		"https://rsshub.app/zaobao/znews/world",
		"https://rsshub.app/zaobao/znews/china",
	}}
	sorted := unsorted.MatchRouteSubscriptions([]string{"https://rsshub.app/zaobao/znews"})
	require.Equal(t, []string{
		"https://rsshub.app/zaobao/znews/china",
		"https://rsshub.app/zaobao/znews/world",
	}, sorted["https://rsshub.app/zaobao/znews"], "结果按 URL 字典序稳定排序")
}

// TestLoadSubscriptionIndex 一次 SELECT url FROM feeds：装载全部订阅地址。
func TestLoadSubscriptionIndex(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	require.NoError(t, db.Create(&models.Feed{Title: "已订", URL: "https://rsshub.app/zaobao/znews/world"}).Error)
	require.NoError(t, db.Create(&models.Feed{Title: "另一条", URL: "https://example.com/feed.xml"}).Error)

	idx, err := LoadSubscriptionIndex(context.Background(), db)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"https://rsshub.app/zaobao/znews/world",
		"https://example.com/feed.xml",
	}, idx.urls)
}

// TestResolveCandidateSubscriptions 批量判定：rsshub 路由级（example 不同也命中）、
// 原生精确（同 path 异 host 不误伤）、route_id 缺失保持未订阅、空集合非 nil。
func TestResolveCandidateSubscriptions(t *testing.T) {
	const base = DefaultRSSHubBaseURL
	idx := &SubscriptionIndex{urls: []string{
		base + "/zaobao/znews/world",
		"https://example.com/native/feed.xml",
	}}
	subscribedRouteID, unusedRouteID := uint(1), uint(2)
	routes := map[uint]discmodels.RSSHubRoute{
		subscribedRouteID: {ID: subscribedRouteID, Namespace: "zaobao", Path: "/znews/:section?"},
		unusedRouteID:     {ID: unusedRouteID, Namespace: "other", Path: "/feed"},
	}
	cands := []discmodels.FeedCandidate{
		{ID: 11, Kind: "rsshub", RouteID: &subscribedRouteID},
		{ID: 12, Kind: "rsshub", RouteID: &unusedRouteID},
		{ID: 13, Kind: "rss", FeedURL: candidateStr("https://example.com/native/feed.xml")},
		{ID: 14, Kind: "rss", FeedURL: candidateStr("https://other.example.com/native/feed.xml")},
		{ID: 15, Kind: "rsshub", RouteID: nil},
	}
	got := resolveCandidateSubscriptions(idx, base, cands, routes)
	require.Equal(t, []string{base + "/zaobao/znews/world"}, got[11], "路由级命中：example 不同也认")
	require.Equal(t, []string{}, got[12], "未订阅 rsshub 为空切片")
	require.Equal(t, []string{"https://example.com/native/feed.xml"}, got[13], "原生精确命中")
	require.Equal(t, []string{}, got[14], "同 path 异 host 不命中")
	require.Equal(t, []string{}, got[15], "route_id 缺失保持未订阅")
	for id, urls := range got {
		require.NotNil(t, urls, "candidate %d 的订阅地址切片必须非 nil（JSON 为 []）", id)
	}
}
