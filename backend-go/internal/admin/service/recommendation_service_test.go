package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// setupRecFixture 构造推荐所需最小数据：全局桶偏好向量 + 3 条路由（ok/broken/ok）+
// 各自候选向量（4.3 召回读 candidate_embeddings）。所有向量同向（distance≈0），
// 靠 status 验证粗筛排除规则。
func setupRecFixture(t *testing.T, db *gorm.DB) (r1, r2, r3 uint) {
	t.Helper()
	vec := padVec([]float64{1, 0, 0})
	routes := []models.RSSHubRoute{
		{Namespace: "nsa", Path: "/a", Name: "A", Example: "/nsa/a", UsableDirectly: true, Status: "ok", Parameters: "{}"},
		{Namespace: "nsb", Path: "/b", Name: "B", Example: "/nsb/b", UsableDirectly: true, Status: "broken", Parameters: "{}"},
		{Namespace: "nsc", Path: "/c", Name: "C", Example: "/nsc/c", UsableDirectly: true, Status: "ok", Parameters: "{}"},
	}
	for i := range routes {
		require.NoError(t, db.Create(&routes[i]).Error)
	}
	r1, r2, r3 = routes[0].ID, routes[1].ID, routes[2].ID
	for _, rid := range []uint{r1, r2, r3} {
		require.NoError(t, db.Create(&models.RouteEmbedding{
			RouteID: rid, EmbeddingVec: floatsToPgVector(vec), Dimension: testutil.TestEmbeddingDim, Model: "test",
		}).Error)
	}
	for _, r := range routes {
		setupCandidateVector(t, db, r.ID, r.Namespace, r.Path, "test", vec)
	}
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceBehavior, EmbeddingVec: floatsToPgVector(vec),
		Dimension: testutil.TestEmbeddingDim, Model: "test",
	}).Error)
	return
}

// 4.1 起 RefreshRecommendations 走 run 严格精排（精排未选中的候选不发布），
// 旧回归网测试统一注入「全选」mock router 保持原断言语义。

// TestRecommendationRefreshExcludesBroken：粗筛排除 status=broken 的路由（D4/D5）。
func TestRecommendationRefreshExcludesBroken(t *testing.T) {
	db := testutil.SetupTestDB(t)
	r1, r2, r3 := setupRecFixture(t, db)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)

	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)

	var recs []models.FeedRecommendation
	require.NoError(t, db.Where("status = ?", "pending").Find(&recs).Error)
	routeIDs := make(map[uint]bool)
	for _, r := range recs {
		routeIDs[r.RouteID] = true
	}
	require.True(t, routeIDs[r1], "ok 路由 r1 应被推荐")
	require.True(t, routeIDs[r3], "ok 路由 r3 应被推荐")
	require.False(t, routeIDs[r2], "broken 路由 r2 应被排除")
}

// TestRecommendationAcceptCreatesFeed：接受 usable_directly 推荐 → 共享建源服务安全验证
// （4.5 起注入 mock 抓取）后创建 feed + 标 accepted。
func TestRecommendationAcceptCreatesFeed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	mockSubscriptionFetch(t, 200, acceptTestValidRSS, nil)
	r1, _, _ := setupRecFixture(t, db)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)

	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)

	var rec models.FeedRecommendation
	require.NoError(t, db.Where("route_id = ? AND status = ?", r1, "pending").First(&rec).Error)

	feed, err := svc.AcceptRecommendation(context.Background(), rec.ID, nil, nil)
	require.NoError(t, err)
	require.NotZero(t, feed.ID)
	require.Equal(t, DefaultRSSHubBaseURL+"/nsa/a", feed.URL)

	var after models.FeedRecommendation
	require.NoError(t, db.First(&after, rec.ID).Error)
	require.Equal(t, "accepted", after.Status)
	require.NotNil(t, after.AcceptedFeedID)
}

// TestRecommendationCardIncludesRouteStatus：卡片暴露路由 status，供前端标「未验证/broken」（spec feed-discovery）。
func TestRecommendationCardIncludesRouteStatus(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_, _, _ = setupRecFixture(t, db)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)

	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)

	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.NotEmpty(t, cards)
	for _, c := range cards {
		require.Equal(t, "ok", c.RouteStatus, "fixture 路由 status=ok 应透传到卡片")
	}
}

// TestBuildFeedURLEscapesParams：requires_parameters 路由，用户填的参数值含特殊字符要 path-escape（M2）。
func TestBuildFeedURLEscapesParams(t *testing.T) {
	r := &models.RSSHubRoute{
		Namespace: "bilibili", Path: "/user/dynamic/:uid",
		RequiresParameters: true, UsableDirectly: false,
	}
	u := buildFeedURL(r, map[string]string{"uid": "a b/c"}, DefaultRSSHubBaseURL)
	require.NotContains(t, u, " ", "空格应转义")
	require.Contains(t, u, "a%20b", "空格转 %20")
	require.Contains(t, u, "%2F", "/ 应转义防 path 注入")
}

// 2026-09 用户实测：zaobao/realtime/:section?（usable_directly + example 缺省 china）
// 旧逻辑 usableDirectly 短路返回 example，用户填的 section 被无视、地址纹丝不动。
func TestBuildFeedURLUsableDirectlyWithParams(t *testing.T) {
	r := &models.RSSHubRoute{
		Namespace: "zaobao", Path: "/realtime/:section?",
		Example: "/zaobao/realtime/china", UsableDirectly: true,
	}
	require.Equal(t,
		DefaultRSSHubBaseURL+"/zaobao/realtime/singapore",
		buildFeedURL(r, map[string]string{"section": "singapore"}, DefaultRSSHubBaseURL),
		"填了参数必须替换模板，不得短路到 example")
	require.Equal(t,
		DefaultRSSHubBaseURL+"/zaobao/realtime/china",
		buildFeedURL(r, nil, DefaultRSSHubBaseURL),
		"未填参保持 example 缺省形态")
	require.Equal(t,
		DefaultRSSHubBaseURL+"/zaobao/realtime/china",
		buildFeedURL(r, map[string]string{"section": "  "}, DefaultRSSHubBaseURL),
		"空白值视作未提供，不影响 example 缺省")
}

// 可选参数替换后不得残留尾部 `?`（旧逻辑 :name 替换后 val? 残留，URL 带裸 ? 可致实例 404）。
func TestBuildFeedURLNoQuestionMarkResidue(t *testing.T) {
	r := &models.RSSHubRoute{Namespace: "zaobao", Path: "/realtime/:section?"}
	u := buildFeedURL(r, map[string]string{"section": "singapore"}, DefaultRSSHubBaseURL)
	require.Equal(t, DefaultRSSHubBaseURL+"/zaobao/realtime/singapore", u)
	require.NotContains(t, u, "?", "替换后的值不得残留可选标记 ?")
}

// {regex} 约束在替换前剥离：约束片段不得混入替换结果。
func TestBuildFeedURLStripsBraceConstraintBeforeSubstitution(t *testing.T) {
	r := &models.RSSHubRoute{Namespace: "test", Path: "/list/:keyword{.+}?"}
	u := buildFeedURL(r, map[string]string{"keyword": "ai"}, DefaultRSSHubBaseURL)
	require.Equal(t, DefaultRSSHubBaseURL+"/test/list/ai", u)
}
