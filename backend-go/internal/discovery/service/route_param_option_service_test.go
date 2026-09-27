package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	discmodels "syntopica-backend/internal/discovery/models"
	"syntopica-backend/internal/platform/testutil"
)

// makeRouteForParamOption 创建一条 bare RSSHubRoute 供字典 FK 挂靠。
func makeRouteForParamOption(t *testing.T, db *gorm.DB, ns, path string) uint {
	t.Helper()
	r := discmodels.RSSHubRoute{Namespace: ns, Path: path, Name: ns + path, Parameters: "{}", Status: "ok"}
	require.NoError(t, db.Create(&r).Error)
	return r.ID
}

// TestRouteParamOptionBatchByRouteIDs：一次 IN 查询取多路由字典；空集合不发查询（T2）。
func TestRouteParamOptionBatchByRouteIDs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewRouteParamOptionService(db)
	r1 := makeRouteForParamOption(t, db, "nsa", "/realtime/:category?")
	r2 := makeRouteForParamOption(t, db, "nsb", "/user/:uid")

	require.NoError(t, db.Create(&discmodels.RouteParamOption{RouteID: r1, ParamName: "category", Value: "world", Label: "国际", Source: "manual"}).Error)
	require.NoError(t, db.Create(&discmodels.RouteParamOption{RouteID: r1, ParamName: "category", Value: "cn", Label: "国内", Source: "manual"}).Error)
	require.NoError(t, db.Create(&discmodels.RouteParamOption{RouteID: r2, ParamName: "uid", Value: "1", Label: "测试用户", Source: "scraped"}).Error)

	// 批量取 r1+r2：返回全部 3 条。
	opts, err := svc.ListByRouteIDs(context.Background(), []uint{r1, r2})
	require.NoError(t, err)
	require.Len(t, opts, 3)

	// 空 routeIDs：不发查询、不报错、返回 nil。
	got, err := svc.ListByRouteIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Nil(t, got)

	// 仅 r1：返回 2 条。
	opts1, err := svc.ListByRouteIDs(context.Background(), []uint{r1})
	require.NoError(t, err)
	require.Len(t, opts1, 2)
}

// TestGroupByRouteAndParam：扁平条目 → map[routeID]map[paramName][]ParamOption（T2 契约形状）。
func TestGroupByRouteAndParam(t *testing.T) {
	opts := []discmodels.RouteParamOption{
		{RouteID: 1, ParamName: "category", Value: "world", Label: "国际", Source: "manual"},
		{RouteID: 1, ParamName: "category", Value: "cn", Label: "国内", Source: "manual"},
		{RouteID: 1, ParamName: "lang", Value: "zh", Label: "中文", Source: "scraped"},
		{RouteID: 2, ParamName: "uid", Value: "1", Label: "用户1", Source: "manual"},
	}
	grouped := GroupByRouteAndParam(opts)
	require.Contains(t, grouped, uint(1))
	require.Contains(t, grouped, uint(2))
	require.Len(t, grouped[1]["category"], 2, "route1 category 有 2 个可选值")
	require.Len(t, grouped[1]["lang"], 1)
	require.Len(t, grouped[2]["uid"], 1)

	// 契约形状：ParamOption 只有 value/label/source。
	cat0 := grouped[1]["category"][0]
	require.Equal(t, "world", cat0.Value)
	require.Equal(t, "国际", cat0.Label)
	require.Equal(t, "manual", cat0.Source)
}

// TestGroupByRouteAndParamEmpty：空集合兜底返回非 nil 空 map（向后兼容 JSON {}）。
func TestGroupByRouteAndParamEmpty(t *testing.T) {
	grouped := GroupByRouteAndParam(nil)
	require.NotNil(t, grouped, "空集合返回非 nil map")
	require.Len(t, grouped, 0)
}

// TestRouteParamOptionCRUD：admin CRUD 全链路 + 空 source 走 DB DEFAULT manual。
func TestRouteParamOptionCRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewRouteParamOptionService(db)
	rid := makeRouteForParamOption(t, db, "nsc", "/list/:type?")

	// Create：空 source → DB DEFAULT manual。
	created, err := svc.Create(context.Background(), RouteParamOptionInput{
		RouteID: rid, ParamName: "type", Value: "hot", Label: "热门",
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "manual", created.Source, "空 source 走 DB DEFAULT manual")

	// Get。
	got, err := svc.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "hot", got.Value)

	// Update：改 label + source=scraped。
	updated, err := svc.Update(context.Background(), created.ID, RouteParamOptionInput{
		Value: "hot", Label: "热门榜", Source: "scraped",
	})
	require.NoError(t, err)
	require.Equal(t, "热门榜", updated.Label)
	require.Equal(t, "scraped", updated.Source)

	// List 按 route_id 过滤。
	listed, err := svc.List(context.Background(), &rid)
	require.NoError(t, err)
	require.Len(t, listed, 1)

	// Delete。
	require.NoError(t, svc.Delete(context.Background(), created.ID))
	_, err = svc.Get(context.Background(), created.ID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestRouteParamOptionRejectsLLMSource：source=llm 被拒（D5 铁律：永不接受 llm）。
func TestRouteParamOptionRejectsLLMSource(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewRouteParamOptionService(db)
	rid := makeRouteForParamOption(t, db, "nsd", "/x/:p?")

	_, err := svc.Create(context.Background(), RouteParamOptionInput{
		RouteID: rid, ParamName: "p", Value: "v", Source: "llm",
	})
	require.Error(t, err, "source=llm 必须被拒")

	// 先建一条合法的，再用 Update 尝试改成 llm → 也要被拒。
	created, err := svc.Create(context.Background(), RouteParamOptionInput{
		RouteID: rid, ParamName: "p", Value: "v", Source: "manual",
	})
	require.NoError(t, err)
	_, err = svc.Update(context.Background(), created.ID, RouteParamOptionInput{Source: "llm"})
	require.Error(t, err, "Update source=llm 必须被拒")
}

// TestRouteParamOptionCreateRequiresFields：route_id/param_name/value 必填。
func TestRouteParamOptionCreateRequiresFields(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewRouteParamOptionService(db)
	_, err := svc.Create(context.Background(), RouteParamOptionInput{ParamName: "p", Value: "v"})
	require.Error(t, err, "缺 route_id 必须报错")
}
