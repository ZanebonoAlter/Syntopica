package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── 目录同步（3.4 修复：fetch 失败=失败、content_hash 补全 url/example、
// 候选联动 upsert 与人工隔离、gone 语义）──
//
// 前三个用例沿用 testcontainer PG（既有覆盖）；3.4 新增用例走 sqlite 内存库 + mock fetch，
// 只迁移本切片涉及的表（RSSHubRoute / FeedCandidate / Feed / CandidateAvailability），
// 不触碰 platform/database 迁移辖区。

// setupCatalogSyncSQLiteDB 内存库 + 本切片相关表。
func setupCatalogSyncSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:cs-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.RSSHubRoute{}, &models.FeedCandidate{}, &models.Feed{}, &models.CandidateAvailability{},
	))
	return db
}

// newSQLiteCatalogSyncService 构造 sqlite 下的同步服务（显式 baseURL，不读全局 ai_settings）。
func newSQLiteCatalogSyncService(t *testing.T, db *gorm.DB) *CatalogSyncService {
	t.Helper()
	return NewCatalogSyncService(db, "https://rsshub.test")
}

// mockFetchNamespace 固定返回一组路由（routes: {外层键: detail}）。
func mockFetchNamespace(t *testing.T, routes map[string]map[string]any) func(context.Context) (map[string]json.RawMessage, error) {
	t.Helper()
	return func(context.Context) (map[string]json.RawMessage, error) {
		return marshalNamespace(t, routes), nil
	}
}

// newsflashRoute 单路由 fixture（用于内容变更对比）。
func newsflashRoute(name, url, example string) map[string]map[string]any {
	return map[string]map[string]any{
		"36kr": {
			"/newsflashes": map[string]any{
				"path": "/36kr/newsflashes", "name": name, "url": url, "example": example,
				"description": "d", "parameters": map[string]any{},
			},
		},
	}
}

// legacyContentHash 3.4 之前的 hash 算法（不含 url/example），用于验证旧 hash 重同步收敛。
func legacyContentHash(rec routeRecord) string {
	raw := rec.Namespace + "|" + rec.Path + "|" + rec.Name + "|" + rec.Description + "|" + rec.ParametersJSON()
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])[:32]
}

// loadOnlyCandidate 取唯一候选（并断言只有一条）。
func loadOnlyCandidate(t *testing.T, db *gorm.DB, stableKey string) models.FeedCandidate {
	t.Helper()
	var cands []models.FeedCandidate
	require.NoError(t, db.Find(&cands).Error)
	require.Len(t, cands, 1)
	if stableKey != "" {
		require.Equal(t, stableKey, cands[0].StableKey)
	}
	return cands[0]
}

// marshalNamespace 构造 /api/namespace 的 mock 响应（ns → {routes: {path: detail}}）。
func marshalNamespace(t *testing.T, routes map[string]map[string]any) map[string]json.RawMessage {
	t.Helper()
	out := make(map[string]json.RawMessage)
	for ns, rs := range routes {
		body := map[string]any{"routes": rs}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		out[ns] = b
	}
	return out
}

// TestFlattenNamespace 验证嵌套 dict 展平（D2 解析）。
func TestFlattenNamespace(t *testing.T) {
	raw := marshalNamespace(t, map[string]map[string]any{
		"36kr": {
			"/newsflashes": map[string]any{
				"path": "/36kr/newsflashes", "name": "快讯", "url": "36kr.com",
				"example": "/36kr/newsflashes", "description": "36氪快讯",
				"parameters": map[string]any{},
			},
		},
		"bilibili": {
			"/user/dynamic/:uid": map[string]any{
				"path": "/bilibili/user/dynamic/:uid", "name": "用户动态",
				"example": "/bilibili/user/dynamic/1", "description": "B 站用户动态",
				"parameters": map[string]any{"uid": "用户 UID"},
			},
		},
	})
	recs, err := flattenNamespace(raw)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	byPath := make(map[string]routeRecord, len(recs))
	for _, r := range recs {
		byPath[r.Path] = r
	}
	require.Contains(t, byPath, "/36kr/newsflashes")
	require.Contains(t, byPath, "/bilibili/user/dynamic/:uid")
	require.Equal(t, "快讯", byPath["/36kr/newsflashes"].Name)
	require.Equal(t, `{"uid":"用户 UID"}`, byPath["/bilibili/user/dynamic/:uid"].ParametersJSON())
}

// TestCatalogSyncAllInsertAndParamMark：首次同步入库 + 参数标记（testcontainer）。
func TestCatalogSyncAllInsertAndParamMark(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := NewCatalogSyncService(db, "")
	svc.fetch = func(ctx context.Context) (map[string]json.RawMessage, error) {
		return marshalNamespace(t, map[string]map[string]any{
			"36kr": {
				"/newsflashes": map[string]any{
					"path": "/36kr/newsflashes", "name": "快讯",
					"description": "36氪快讯", "parameters": map[string]any{},
				},
			},
			"bilibili": {
				"/user/dynamic/:uid": map[string]any{
					"path": "/bilibili/user/dynamic/:uid", "name": "用户动态",
					"description": "B 站动态", "parameters": map[string]any{"uid": "用户 UID"},
				},
			},
		}), nil
	}

	summary, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, summary.Total)
	require.Equal(t, 2, summary.Inserted)

	var usable, requires models.RSSHubRoute
	require.NoError(t, db.Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").First(&usable).Error)
	require.True(t, usable.UsableDirectly, "零参数路由 usable_directly")
	require.False(t, usable.RequiresParameters)

	require.NoError(t, db.Where("namespace = ? AND path = ?", "bilibili", "/bilibili/user/dynamic/:uid").First(&requires).Error)
	require.True(t, requires.RequiresParameters, "必填参数路由 requires_parameters")
	require.False(t, requires.UsableDirectly)
}

// TestCatalogSyncAllIdempotent：二次同步内容不变 → 不产生变更。
func TestCatalogSyncAllIdempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	fetch := func(ctx context.Context) (map[string]json.RawMessage, error) {
		return marshalNamespace(t, map[string]map[string]any{
			"36kr": {"/newsflashes": map[string]any{"path": "/36kr/newsflashes", "name": "快讯", "description": "d"}},
		}), nil
	}
	svc := NewCatalogSyncService(db, "")
	svc.fetch = fetch

	_, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	s2, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, s2.Inserted, "幂等：无新增")
	require.Equal(t, 0, s2.Updated, "幂等：无变更")
}

// TestCatalogSyncAllMarksGone：目录消失的路由标 gone（不物理删除）。
// Medium 9 修复后，只有「合法非空目录中缺席」才标 gone；空载荷不再走到这里（见
// TestCatalogSyncEmptyPayloadFailsClosed）。
func TestCatalogSyncAllMarksGone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	full := func(ctx context.Context) (map[string]json.RawMessage, error) {
		return marshalNamespace(t, map[string]map[string]any{
			"36kr": {
				"/newsflashes": map[string]any{"path": "/36kr/newsflashes", "name": "快讯", "description": "d"},
				"/hot":         map[string]any{"path": "/36kr/hot", "name": "热榜", "description": "d"},
			},
		}), nil
	}
	// 第二轮：目录仍非空（/hot 还在），但 /newsflashes 消失 → 只标它 gone。
	withoutNewsflash := func(ctx context.Context) (map[string]json.RawMessage, error) {
		return marshalNamespace(t, map[string]map[string]any{
			"36kr": {"/hot": map[string]any{"path": "/36kr/hot", "name": "热榜", "description": "d"}},
		}), nil
	}
	svc := NewCatalogSyncService(db, "")
	svc.fetch = full
	_, err := svc.SyncAll(context.Background())
	require.NoError(t, err)

	svc.fetch = withoutNewsflash
	s2, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s2.Gone, "消失路由应标 gone")

	var gone models.RSSHubRoute
	require.NoError(t, db.Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").First(&gone).Error)
	require.Equal(t, "gone", gone.Status, "路由保留但状态 gone（不物理删除）")
}

// TestCatalogSyncEmptyPayloadFailsClosed：HTTP 200 + 合法 `{}`（全量展开 0 条）→ 整轮失败，
// 既有路由零 gone（Medium 9：空目录载荷不得把全部路由误判为上游删除）。
func TestCatalogSyncEmptyPayloadFailsClosed(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	seed := models.RSSHubRoute{Namespace: "36kr", Path: "/newsflashes", Name: "快讯", Parameters: "{}", ContentHash: "h", Status: "ok"}
	require.NoError(t, db.Create(&seed).Error)

	svc := newSQLiteCatalogSyncService(t, db)
	svc.fetch = func(context.Context) (map[string]json.RawMessage, error) { return map[string]json.RawMessage{}, nil }
	summary, err := svc.SyncAll(context.Background())
	require.ErrorContains(t, err, "empty namespace payload", "空载荷视为异常")
	require.Nil(t, summary)

	var gone int64
	require.NoError(t, db.Model(&models.RSSHubRoute{}).Where("status = ?", "gone").Count(&gone).Error)
	require.EqualValues(t, 0, gone, "空载荷绝不标 gone")
	var reloaded models.RSSHubRoute
	require.NoError(t, db.First(&reloaded, seed.ID).Error)
	require.Equal(t, "ok", reloaded.Status, "既有路由状态保持不变")
}

// TestCatalogSyncAllUnreachable：fetch 失败 → 返回错误且零 gone 标记（不再假装「无变化」成功）。
func TestCatalogSyncAllUnreachable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// 预置一行
	require.NoError(t, db.Create(&models.RSSHubRoute{Namespace: "x", Path: "/p", Name: "n", Parameters: "{}", ContentHash: "h", Status: "unknown"}).Error)
	svc := NewCatalogSyncService(db, "")
	svc.fetch = func(ctx context.Context) (map[string]json.RawMessage, error) {
		return nil, gorm.ErrInvalidDB // 模拟不可达
	}
	summary, err := svc.SyncAll(context.Background())
	require.Error(t, err, "拉不到不等于无变化：必须返回错误")
	require.ErrorIs(t, err, gorm.ErrInvalidDB, "错误须保留原因链")
	require.Nil(t, summary)
	var gone int64
	db.Model(&models.RSSHubRoute{}).Where("status = ?", "gone").Count(&gone)
	require.EqualValues(t, 0, gone, "拉取失败绝不标记 gone")
	var cnt int64
	db.Model(&models.RSSHubRoute{}).Count(&cnt)
	require.EqualValues(t, 1, cnt, "既有目录保持不变")
}

// ─────────────────────────────────────────────────────────────────────────────
// 3.4 新增覆盖（sqlite + mock fetch）：fetch 失败语义、content_hash 补全、
// 候选联动 upsert 与人工隔离、gone 语义与视图透出。
// ─────────────────────────────────────────────────────────────────────────────

// seedRouteAndCandidate 建一条既有路由 + 带人工值/停用/授权的候选，用于人工隔离断言。
func seedRouteAndCandidate(t *testing.T, db *gorm.DB, hash string) (models.RSSHubRoute, models.FeedCandidate) {
	t.Helper()
	route := models.RSSHubRoute{
		Namespace: "36kr", Path: "/36kr/newsflashes", Name: "旧上游名", URL: "https://36kr.com",
		Example: "/36kr/newsflashes", Description: "上游说明", Parameters: "{}",
		ContentHash: hash, Status: "ok",
	}
	require.NoError(t, db.Create(&route).Error)
	enabled := false
	cand := models.FeedCandidate{
		StableKey: "36kr//36kr/newsflashes", Kind: "rsshub", RouteID: &route.ID,
		ManualMetadata: models.MetadataMap{
			ManualFieldName: "人工名", ManualFieldDescription: "人工说明",
		},
		RecommendationEnabled: &enabled, AccessScope: "private_allowed", Revision: 7,
	}
	require.NoError(t, db.Create(&cand).Error)
	return route, cand
}

// TestCatalogSyncFetchFailureIsErrorAndTouchesNothing：fetch 失败 → 返回错误（含原因），
// 零 gone 标记、零候选写入、既有路由与候选（含人工列）逐字段不变。
func TestCatalogSyncFetchFailureIsErrorAndTouchesNothing(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	route, cand := seedRouteAndCandidate(t, db, "oldhash")

	svc := newSQLiteCatalogSyncService(t, db)
	svc.fetch = func(context.Context) (map[string]json.RawMessage, error) {
		return nil, errors.New("dial rsshub.test: connection refused")
	}
	summary, err := svc.SyncAll(context.Background())
	require.Error(t, err, "拉不到不得当「无变化」返回成功 summary")
	require.Nil(t, summary)
	require.Contains(t, err.Error(), "connection refused", "错误信息须含原因")

	var got models.RSSHubRoute
	require.NoError(t, db.First(&got, route.ID).Error)
	require.Equal(t, "ok", got.Status, "失败不得标 gone")
	require.Equal(t, "oldhash", got.ContentHash, "失败不得改 hash")
	require.Equal(t, "旧上游名", got.Name)

	after := loadOnlyCandidate(t, db, cand.StableKey)
	require.Equal(t, cand.ManualMetadata, after.ManualMetadata)
	require.Equal(t, *cand.RecommendationEnabled, *after.RecommendationEnabled)
	require.Equal(t, cand.AccessScope, after.AccessScope)
	require.Equal(t, cand.Revision, after.Revision)
}

// TestCatalogSyncNamespaceParseFailureIsWholeRoundFailure：任一 namespace 体或 route detail
// 解析失败 → 整轮失败（fail-closed），不标 gone、不写候选、不改既有行。
// 选择依据：拿不到完整目录时按「未出现在本轮列表」推导 gone 会把未取得的路由误判上游删除。
func TestCatalogSyncNamespaceParseFailureIsWholeRoundFailure(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]json.RawMessage
	}{
		{
			name: "namespace 体不是对象",
			raw:  map[string]json.RawMessage{"36kr": json.RawMessage(`"oops"`)},
		},
		{
			name: "route detail 字段类型异常",
			raw: map[string]json.RawMessage{
				"36kr": json.RawMessage(`{"routes":{"/newsflashes":{"path":123,"name":"n"}}}`),
			},
		},
		{
			name: "route detail 无 path",
			raw: map[string]json.RawMessage{
				"36kr": json.RawMessage(`{"routes":{"/newsflashes":{"name":"n"}}}`),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupCatalogSyncSQLiteDB(t)
			route, cand := seedRouteAndCandidate(t, db, "oldhash")

			svc := newSQLiteCatalogSyncService(t, db)
			svc.fetch = func(context.Context) (map[string]json.RawMessage, error) { return tc.raw, nil }
			summary, err := svc.SyncAll(context.Background())
			require.Error(t, err, "响应结构无法完整解析必须整轮失败")
			require.Nil(t, summary)

			var got models.RSSHubRoute
			require.NoError(t, db.First(&got, route.ID).Error)
			require.Equal(t, "ok", got.Status, "部分解析绝不标 gone（整轮放弃）")
			require.Equal(t, "oldhash", got.ContentHash, "未写任何行")
			after := loadOnlyCandidate(t, db, cand.StableKey)
			require.Equal(t, cand.Revision, after.Revision)
		})
	}
}

// TestRouteRecordContentHashCoversURLAndExample：白盒——hash 输入必须含 url 与 example，
// 任一变化都改变 hash（否则上游改了站点/示例后本系统永远用旧值）。
func TestRouteRecordContentHashCoversURLAndExample(t *testing.T) {
	base := routeRecord{
		Namespace: "36kr", Path: "/36kr/newsflashes", Name: "快讯",
		URL: "https://36kr.com", Example: "/36kr/newsflashes", Description: "d",
		parameters: map[string]any{},
	}
	changedURL := base
	changedURL.URL = "https://36kr.com/new"
	changedExample := base
	changedExample.Example = "/36kr/newsflashes/hot"

	require.NotEqual(t, base.contentHash(), changedURL.contentHash(), "url 变化必须改变 hash")
	require.NotEqual(t, base.contentHash(), changedExample.contentHash(), "example 变化必须改变 hash")
	require.Equal(t, base.contentHash(), base.contentHash(), "同输入 hash 稳定")

	// 旧算法（不含 url/example）与算法变更后的 hash 必然不同 → 旧行自然走 Update。
	legacy := legacyContentHash(base)
	require.NotEqual(t, legacy, base.contentHash())
}

// TestCatalogSyncUpdatesOnURLAndExampleChange：url/example 变化走 Update 并落库新值；
// 旧 hash 行重同步一次即收敛（第二轮无变更，无需迁移脚本）。
func TestCatalogSyncUpdatesOnURLAndExampleChange(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	svc := newSQLiteCatalogSyncService(t, db)
	svc.fetch = mockFetchNamespace(t, newsflashRoute("快讯", "https://36kr.com", "/36kr/newsflashes"))
	s1, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s1.Inserted)

	// 把既有行回写成旧算法 hash + 旧 url/example，模拟 3.4 之前的存量数据。
	rec := routeRecord{
		Namespace: "36kr", Path: "/36kr/newsflashes", Name: "快讯",
		URL: "https://36kr.com", Example: "/36kr/newsflashes", Description: "d",
		parameters: map[string]any{},
	}
	require.NoError(t, db.Model(&models.RSSHubRoute{}).
		Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").
		Updates(map[string]any{"content_hash": legacyContentHash(rec), "url": "https://old.example", "example": "/old"}).Error)

	// 上游改了 url / example。
	svc.fetch = mockFetchNamespace(t, newsflashRoute("快讯", "https://36kr.com/new", "/36kr/newsflashes/hot"))
	s2, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s2.Updated, "旧 hash 行重同步走 Update")
	require.Equal(t, 0, s2.Gone)

	var got models.RSSHubRoute
	require.NoError(t, db.Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").First(&got).Error)
	require.Equal(t, "https://36kr.com/new", got.URL, "url 变化必须落库")
	require.Equal(t, "/36kr/newsflashes/hot", got.Example, "example 变化必须落库")
	wantHash := routeRecord{
		Namespace: "36kr", Path: "/36kr/newsflashes", Name: "快讯",
		URL: "https://36kr.com/new", Example: "/36kr/newsflashes/hot", Description: "d",
		parameters: map[string]any{},
	}
	require.Equal(t, wantHash.contentHash(), got.ContentHash)

	// 收敛：内容未再变 → 无变更。
	s3, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, s3.Updated, "重同步一次即收敛")
	require.Equal(t, 0, s3.Inserted)
}

// TestCatalogSyncCreatesCandidateForNewRoute：新路由 → 建候选（kind=rsshub / public /
// enabled=true / revision=1 / stable_key 小写），重复同步幂等不重建。
func TestCatalogSyncCreatesCandidateForNewRoute(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	svc := newSQLiteCatalogSyncService(t, db)
	svc.fetch = func(context.Context) (map[string]json.RawMessage, error) {
		return marshalNamespace(t, map[string]map[string]any{
			"36Kr": {
				"/Newsflashes": map[string]any{
					"path": "/36Kr/Newsflashes", "name": "快讯", "url": "https://36kr.com",
					"description": "d", "parameters": map[string]any{},
				},
			},
		}), nil
	}
	s1, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s1.CandidatesCreated)
	require.Equal(t, 0, s1.CandidatesLinked)

	cand := loadOnlyCandidate(t, db, "36kr//36kr/newsflashes")
	require.Equal(t, "rsshub", cand.Kind)
	require.NotNil(t, cand.RouteID)
	require.Equal(t, "public", cand.AccessScope)
	require.NotNil(t, cand.RecommendationEnabled)
	require.True(t, *cand.RecommendationEnabled)
	require.Equal(t, uint(1), cand.Revision)
	require.Empty(t, cand.ManualMetadata)

	s2, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, s2.CandidatesCreated, "已存在候选不重建")
	require.Equal(t, 0, s2.CandidatesLinked, "已绑定同一 route_id 不产生写放大")
	require.Equal(t, 0, s2.Inserted)
}

// TestCatalogSyncLinksExistingCandidateWithoutTouchingManualFields：已存在候选（如导入的
// rsshub 条目 RouteID 为空）→ 同步只补 route_id 关联；manual/enabled/access/revision 逐字段不变。
func TestCatalogSyncLinksExistingCandidateWithoutTouchingManualFields(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	require.NoError(t, db.Create(&models.RSSHubRoute{
		Namespace: "36kr", Path: "/36kr/newsflashes", Name: "旧名", Parameters: "{}",
		ContentHash: "oldhash", Status: "ok",
	}).Error)
	enabled := false
	imported := models.FeedCandidate{
		StableKey: "36kr//36kr/newsflashes", Kind: "rsshub", RouteID: nil,
		ManualMetadata: models.MetadataMap{
			ManualFieldName: "人工名", ManualFieldDescription: "人工说明", ManualFieldLanguage: "zh", ManualFieldRegion: "CN",
		},
		RecommendationEnabled: &enabled, AccessScope: "private_pending", Revision: 7,
	}
	require.NoError(t, db.Create(&imported).Error)

	svc := newSQLiteCatalogSyncService(t, db)
	svc.fetch = mockFetchNamespace(t, newsflashRoute("新上游名", "https://36kr.com", "/36kr/newsflashes"))
	summary, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, summary.CandidatesLinked, "只补关联")
	require.Equal(t, 0, summary.CandidatesCreated)
	require.Equal(t, 1, summary.Updated, "上游名变化走 Update")

	var linked models.FeedCandidate
	require.NoError(t, db.First(&linked, imported.ID).Error)
	var route models.RSSHubRoute
	require.NoError(t, db.Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").First(&route).Error)
	require.NotNil(t, linked.RouteID)
	require.Equal(t, route.ID, *linked.RouteID)

	require.Equal(t, imported.ManualMetadata, linked.ManualMetadata, "人工说明不得被同步覆盖")
	require.Equal(t, "人工名", linked.ManualMetadata[ManualFieldName])
	require.Equal(t, "private_pending", linked.AccessScope, "授权范围不得被同步改写")
	require.Equal(t, uint(7), linked.Revision, "revision 不得被同步改写")
	require.False(t, *linked.RecommendationEnabled, "用户停用后同步不得复活")
}

// TestCatalogSyncMarkGoneKeepsCandidateAndViewExposesStatus：上游确认消失 → route.status=gone
// 保留行、候选保留（不物理删、订阅与人工资料照旧）；视图透出 route.status；路由复现恢复 unknown。
func TestCatalogSyncMarkGoneKeepsCandidateAndViewExposesStatus(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	svc := newSQLiteCatalogSyncService(t, db)
	// 第一轮就同时有 /newsflashes 与 /other（保活路由）：目录非空是本测试后续「确认消失」
	// 判定 gone 的前提（Medium 9 后空载荷会整轮失败）。
	svc.fetch = mockFetchNamespace(t, map[string]map[string]any{
		"36kr": {
			"/newsflashes": map[string]any{"path": "/36kr/newsflashes", "name": "快讯", "url": "https://36kr.com", "example": "/36kr/newsflashes", "description": "d", "parameters": map[string]any{}},
			"/other":       map[string]any{"path": "/36kr/other", "name": "其他", "description": "d"},
		},
	})
	_, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	var cand models.FeedCandidate
	require.NoError(t, db.Where("stable_key = ?", "36kr//36kr/newsflashes").First(&cand).Error)

	// 上游成功确认路由消失：本轮目录仍非空（/other 还在），/newsflashes 缺席 → 只标它 gone。
	svc.fetch = mockFetchNamespace(t, map[string]map[string]any{
		"36kr": {"/other": map[string]any{"path": "/36kr/other", "name": "其他", "description": "d"}},
	})
	s2, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s2.Gone)

	var route models.RSSHubRoute
	require.NoError(t, db.Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").First(&route).Error)
	require.Equal(t, "gone", route.Status, "保留行并标 gone，不物理删除")
	var after models.FeedCandidate
	require.NoError(t, db.First(&after, cand.ID).Error)
	require.Equal(t, cand.ID, after.ID, "候选保留不删（订阅照旧）")
	require.NotNil(t, after.RouteID)
	require.Equal(t, *cand.RecommendationEnabled, *after.RecommendationEnabled)

	// 视图透出上游下架信息：CandidateView.Route.Status（GetCandidate / ListCandidates 共用 buildViews）。
	viewSvc := NewCandidateCatalogService(db)
	view, err := viewSvc.GetCandidate(context.Background(), after.ID)
	require.NoError(t, err)
	require.NotNil(t, view.Route, "候选仍关联 route")
	require.Equal(t, "gone", view.Route.Status, "视图透出 route.status")

	// gone 幂等：重复同步不再计 gone。
	s3, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, s3.Gone)

	// 路由复现（与 /other 并存）→ 状态恢复 unknown，候选不动。
	svc.fetch = mockFetchNamespace(t, map[string]map[string]any{
		"36kr": {
			"/newsflashes": map[string]any{"path": "/36kr/newsflashes", "name": "快讯", "url": "https://36kr.com", "example": "/36kr/newsflashes", "description": "d", "parameters": map[string]any{}},
			"/other":       map[string]any{"path": "/36kr/other", "name": "其他", "description": "d"},
		},
	})
	s4, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s4.Updated, "复现需清 gone")
	require.NoError(t, db.First(&route, route.ID).Error)
	require.Equal(t, "unknown", route.Status)
	var candCount int64
	require.NoError(t, db.Model(&models.FeedCandidate{}).Count(&candCount).Error)
	require.EqualValues(t, 2, candCount, "gone 不清候选、复现也不重建（newsflashes + other）")
	var reappeared models.FeedCandidate
	require.NoError(t, db.First(&reappeared, cand.ID).Error)
	require.Equal(t, cand.Revision, reappeared.Revision)
}

// TestCatalogSyncContentChangeKeepsAvailabilityStatus（Low 17）：内容变更走 Save 全量写回时，
// 保留既有 status（broken/unknown），不得被初始零值 unknown 覆盖（否则可用性结论丢失）。
func TestCatalogSyncContentChangeKeepsAvailabilityStatus(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	svc := newSQLiteCatalogSyncService(t, db)
	svc.fetch = mockFetchNamespace(t, newsflashRoute("快讯 A", "https://36kr.com", "/36kr/newsflashes"))
	_, err := svc.SyncAll(context.Background())
	require.NoError(t, err)

	// 可用性检查判 broken（模拟 CheckAvailability 写入）。
	require.NoError(t, db.Model(&models.RSSHubRoute{}).
		Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").
		Update("status", "broken").Error)

	// 内容变更（name 变）→ 走 Save 全量写回，status 必须保留 broken。
	svc.fetch = mockFetchNamespace(t, newsflashRoute("快讯 B", "https://36kr.com", "/36kr/newsflashes"))
	s, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, s.Updated)
	var row models.RSSHubRoute
	require.NoError(t, db.Where("namespace = ? AND path = ?", "36kr", "/36kr/newsflashes").First(&row).Error)
	require.Equal(t, "快讯 B", row.Name)
	require.Equal(t, "broken", row.Status, "内容变更不得重置可用性结论")
}

// TestCatalogSyncLinksImportedCandidateOnUnchangedRoute：路由内容未变（本轮不写路由行）
// 也要补齐导入条目留空的 route_id 关联（design D8：导入的 rsshub 条目待同步绑定本地上游），
// 且只写关联列。第二轮无变化 → 零写放大。
func TestCatalogSyncLinksImportedCandidateOnUnchangedRoute(t *testing.T) {
	db := setupCatalogSyncSQLiteDB(t)
	svc := newSQLiteCatalogSyncService(t, db)
	fetch := mockFetchNamespace(t, newsflashRoute("快讯", "https://36kr.com", "/36kr/newsflashes"))
	svc.fetch = fetch
	_, err := svc.SyncAll(context.Background())
	require.NoError(t, err)

	// 模拟导入：候选存在但 RouteID 为空，人工字段与停用状态为导入/用户设定。
	cand := loadOnlyCandidate(t, db, "")
	enabled := false
	require.NoError(t, db.Model(&models.FeedCandidate{}).Where("id = ?", cand.ID).Updates(map[string]any{
		"route_id":               nil,
		"manual_metadata":        models.MetadataMap{ManualFieldName: "人工名"},
		"recommendation_enabled": enabled,
		"access_scope":           "private_pending",
		"revision":               5,
	}).Error)

	// 再次同步：路由内容一模一样 → 不写路由行，但候选必须绑上 route_id。
	s, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, s.Inserted)
	require.Equal(t, 0, s.Updated, "内容未变不写路由")
	require.Equal(t, 1, s.CandidatesLinked, "未变路由也要补齐关联")

	linked := loadOnlyCandidate(t, db, cand.StableKey)
	require.NotNil(t, linked.RouteID, "导入条目同步后绑定本地上游")
	require.Equal(t, uint(5), linked.Revision)
	require.Equal(t, "private_pending", linked.AccessScope)
	require.False(t, *linked.RecommendationEnabled)
	require.Equal(t, "人工名", linked.ManualMetadata[ManualFieldName])

	// 已绑定 → 第三轮零写放大。
	s3, err := svc.SyncAll(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, s3.CandidatesLinked)
	require.Equal(t, 0, s3.CandidatesCreated)
}
