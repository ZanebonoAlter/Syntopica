package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// TestRecommendationCardParamOptionsPopulated：有字典的路由 → 卡片 param_options 按 param_name 分组注入（D7）。
func TestRecommendationCardParamOptionsPopulated(t *testing.T) {
	db := testutil.SetupTestDB(t)
	r1, _, _ := setupRecFixture(t, db)

	// 给 r1 的 category 参数录 2 个可选值（manual）。
	require.NoError(t, db.Create(&models.RouteParamOption{RouteID: r1, ParamName: "category", Value: "world", Label: "国际", Source: "manual"}).Error)
	require.NoError(t, db.Create(&models.RouteParamOption{RouteID: r1, ParamName: "category", Value: "cn", Label: "国内", Source: "manual"}).Error)

	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)
	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)

	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)

	var r1card *RecommendationCard
	for i := range cards {
		if cards[i].RouteID == r1 {
			r1card = &cards[i]
		}
	}
	require.NotNil(t, r1card, "r1 应有推荐卡片")
	require.NotNil(t, r1card.ParamOptions, "param_options 必为非 nil map")
	require.Contains(t, r1card.ParamOptions, "category")
	require.Len(t, r1card.ParamOptions["category"], 2, "category 有 2 个可选值")

	// 契约形状：ParamOption 只含 value/label/source，source 不含 llm。
	opt := r1card.ParamOptions["category"][0]
	require.Equal(t, "world", opt.Value)
	require.Equal(t, "国际", opt.Label)
	require.Equal(t, "manual", opt.Source)
}

// TestRecommendationCardParamOptionsEmptyMapWhenNoDictionary：无字典数据的路由 →
// 卡片 ParamOptions 为非 nil 空 map（JSON 序列化为 {}，向后兼容契约）。
func TestRecommendationCardParamOptionsEmptyMapWhenNoDictionary(t *testing.T) {
	db := testutil.SetupTestDB(t)
	_, _, r3 := setupRecFixture(t, db)
	// 不给任何路由录字典。

	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)
	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)

	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.NotEmpty(t, cards)

	for i := range cards {
		// 每张卡（含无字典的 r3）param_options 必为非 nil、空 → JSON {}。
		require.NotNil(t, cards[i].ParamOptions, "card route=%d param_options 必非 nil", cards[i].RouteID)
		require.Empty(t, cards[i].ParamOptions)
		// 序列化契约：{} 而非 null。
		b, err := json.Marshal(cards[i].ParamOptions)
		require.NoError(t, err)
		require.Equal(t, "{}", string(b), "无字典时 param_options 序列化为 {}")
	}

	// r3 明确存在且空字典。
	var r3card *RecommendationCard
	for i := range cards {
		if cards[i].RouteID == r3 {
			r3card = &cards[i]
		}
	}
	require.NotNil(t, r3card)
}

// TestRecommendationCardParamOptionsEmptyOnEmptyResult：无推荐卡片时也不 panic、返回空切片。
func TestRecommendationCardParamOptionsEmptyOnEmptyResult(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewRecommendationService(db, nil, nil)
	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.Empty(t, cards)
}
