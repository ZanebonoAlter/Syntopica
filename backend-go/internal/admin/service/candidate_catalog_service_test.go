package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// ── 候选源库 S9（feed-candidate-catalog spec：Candidate Catalog Is Not a Subscription List）──
//
// sqlite 内存库只迁移本切片相关表（FeedCandidate/Feed/RSSHubRoute/RouteParamOption/CandidateAvailability），
// 不触碰 platform/database 迁移辖区。candidate_preferences 刻意不建表：开关切换
// 用例在无该表的环境下运行通过，即证明服务不依赖/不触碰它（排除/冷却是另一权威）。
// CandidateAvailability 供候选视图的可用性字段（3.3/D7）读取：无记录 = unknown。

func setupCandidateCatalogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.FeedCandidate{}, &models.Feed{}, &models.RSSHubRoute{}, &models.RouteParamOption{},
		&models.CandidateAvailability{},
	))
	return db
}

func candidateStr(v string) *string { return &v }
func candidateBool(v bool) *bool    { return &v }

// countRows 统计表行数（candidate_preferences 不存在时返回 -1，用于断言「不触碰」）。
func countRows(t *testing.T, db *gorm.DB, model any, table string) int64 {
	t.Helper()
	if !db.Migrator().HasTable(table) {
		return -1
	}
	var n int64
	require.NoError(t, db.Table(table).Count(&n).Error)
	return n
}

// TestCandidateCatalogCreatePersistsWithoutSubscription S9-1 手动入库再订阅：
// 保存有效候选 → 条目出现且「尚未订阅」；feeds 表零行（入库 ≠ 订阅，spec C1）。
func TestCandidateCatalogCreatePersistsWithoutSubscription(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	view, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name:        "Example Feed",
		FeedURL:     "https://Example.com/feed?sort=desc",
		Description: "an example feed",
		Language:    "zh",
		Region:      "CN",
	})
	require.NoError(t, err)

	// 落库断言。
	require.EqualValues(t, 1, countRows(t, db, &models.FeedCandidate{}, "feed_candidates"))
	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, view.ID).Error)
	require.Equal(t, "rss", stored.Kind)
	require.True(t, strings.HasPrefix(stored.StableKey, "rss:"))
	require.NotContains(t, stored.StableKey, "example.com") // 稳定键为哈希，不泄露原 URL 明文
	require.Equal(t, "https://example.com/feed?sort=desc", stored.CanonicalKey)
	require.NotNil(t, stored.FeedURL)
	require.Equal(t, "https://example.com/feed?sort=desc", *stored.FeedURL)
	require.Equal(t, "Example Feed", stored.ManualMetadata[ManualFieldName])
	require.NotNil(t, stored.RecommendationEnabled)
	require.True(t, *stored.RecommendationEnabled)
	require.Equal(t, "public", stored.AccessScope)
	require.EqualValues(t, 1, stored.Revision)

	// 视图断言：有效字段 + 尚未订阅 + 无 Feed 副作用。
	require.Equal(t, "Example Feed", view.Name)
	require.Equal(t, "an example feed", view.Description)
	require.Equal(t, "zh", view.Language)
	require.Equal(t, "CN", view.Region)
	require.False(t, view.Subscribed)
	require.Nil(t, view.Route)
	require.EqualValues(t, 0, countRows(t, db, &models.Feed{}, "feeds"))
}

// TestCandidateCatalogCreateRecommendationEnabled 创建即落「参与推荐」开关（spec C1：记录含推荐启停）：
// 显式 false → 库内 false 且视图 false（条目仍入库，只是不进推荐池）；缺省/nil → true。
func TestCandidateCatalogCreateRecommendationEnabled(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	// 显式 false。
	off, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Paused", FeedURL: "https://example.com/paused",
		RecommendationEnabled: candidateBool(false),
	})
	require.NoError(t, err)
	require.NotNil(t, off.RecommendationEnabled)
	require.False(t, *off.RecommendationEnabled)
	var storedOff models.FeedCandidate
	require.NoError(t, db.First(&storedOff, off.ID).Error)
	require.NotNil(t, storedOff.RecommendationEnabled)
	require.False(t, *storedOff.RecommendationEnabled)

	// 显式 true。
	on, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "On", FeedURL: "https://example.com/on",
		RecommendationEnabled: candidateBool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, on.RecommendationEnabled)
	require.True(t, *on.RecommendationEnabled)

	// 缺省 nil = true。
	def, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Default", FeedURL: "https://example.com/default",
	})
	require.NoError(t, err)
	require.NotNil(t, def.RecommendationEnabled)
	require.True(t, *def.RecommendationEnabled)
	var storedDef models.FeedCandidate
	require.NoError(t, db.First(&storedDef, def.ID).Error)
	require.NotNil(t, storedDef.RecommendationEnabled)
	require.True(t, *storedDef.RecommendationEnabled)

	// 三行均入库（开关只改状态，不影响入库）。
	require.EqualValues(t, 3, countRows(t, db, &models.FeedCandidate{}, "feed_candidates"))
}

// TestCandidateCatalogCreateRejectsRSSHubKind rsshub 候选由同步/迁移产生：手动新增拒绝且零写入。
func TestCandidateCatalogCreateRejectsRSSHubKind(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)

	_, err := svc.CreateCandidate(context.Background(), CandidateCreateInput{
		Kind: "rsshub", Name: "x", FeedURL: "https://example.com/feed",
	})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeValidation, ce.Code)
	require.EqualValues(t, 0, countRows(t, db, &models.FeedCandidate{}, "feed_candidates"))
}

// TestCandidateCatalogCreateInvalidInputs S9-2a 输入校验：非法输入给字段错误且不写入
// （空白名称 / 非法 URL / 非 http scheme / userinfo / 超长字段 / URL 超长）。
func TestCandidateCatalogCreateInvalidInputs(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	longName := strings.Repeat("名", 201) // 201 runes > 200
	longURL := "https://example.com/" + strings.Repeat("a", 501)
	cases := []struct {
		name string
		in   CandidateCreateInput
	}{
		{"blank name", CandidateCreateInput{Name: "   ", FeedURL: "https://example.com/feed"}},
		{"empty name", CandidateCreateInput{Name: "", FeedURL: "https://example.com/feed"}},
		{"empty url", CandidateCreateInput{Name: "ok", FeedURL: "  "}},
		{"bad scheme", CandidateCreateInput{Name: "ok", FeedURL: "ftp://example.com/feed"}},
		{"userinfo", CandidateCreateInput{Name: "ok", FeedURL: "https://user:pass@example.com/feed"}},
		{"no host", CandidateCreateInput{Name: "ok", FeedURL: "https:///feed"}},
		{"invalid port", CandidateCreateInput{Name: "ok", FeedURL: "https://example.com:99999/feed"}},
		{"name too long", CandidateCreateInput{Name: longName, FeedURL: "https://example.com/feed"}},
		{"url too long", CandidateCreateInput{Name: "ok", FeedURL: longURL}},
		{"description too long", CandidateCreateInput{
			Name: "ok", FeedURL: "https://example.com/feed", Description: strings.Repeat("d", 4001)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateCandidate(ctx, tc.in)
			require.Error(t, err)
			var ce *CandidateError
			require.ErrorAs(t, err, &ce, tc.name)
			require.Equal(t, CandidateErrorCodeValidation, ce.Code)
		})
	}
	require.EqualValues(t, 0, countRows(t, db, &models.FeedCandidate{}, "feed_candidates"))
}

// TestCandidateCatalogCreateDuplicateVariants S9-2b 同源重复：scheme/host 大小写、默认端口、
// fragment 变体规范化后身份相同 → conflict 且携带已有条目 ID，不建第二条。
func TestCandidateCatalogCreateDuplicateVariants(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	first, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Example", FeedURL: "https://example.com/feed",
	})
	require.NoError(t, err)

	variants := []string{
		"https://example.com/feed",             // 完全相同
		"HTTPS://EXAMPLE.COM/feed",             // scheme/host 大小写
		"https://example.com:443/feed",         // 默认端口
		"  https://example.com/feed#fragment ", // fragment + 首尾空白
	}
	for i, raw := range variants {
		_, err := svc.CreateCandidate(ctx, CandidateCreateInput{Name: "Dup", FeedURL: raw})
		require.Error(t, err, "variant %d", i)
		var ce *CandidateError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, CandidateErrorCodeConflict, ce.Code)
		require.Equal(t, first.ID, ce.ExistingID, "conflict 应携带已有条目 ID")
	}
	require.EqualValues(t, 1, countRows(t, db, &models.FeedCandidate{}, "feed_candidates"))
}

// TestCandidateCatalogUpdateToggle S9-3a 停用与订阅独立：开关只改 enabled + revision，
// 不触碰 candidate_preferences（表不存在也不报错）；feeds 不受影响。
func TestCandidateCatalogUpdateToggle(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	created, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Toggle", FeedURL: "https://example.com/toggle",
	})
	require.NoError(t, err)

	view, err := svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{
		RecommendationEnabled: candidateBool(false),
		Revision:              created.Revision,
	})
	require.NoError(t, err)
	require.NotNil(t, view.RecommendationEnabled)
	require.False(t, *view.RecommendationEnabled)
	require.EqualValues(t, created.Revision+1, view.Revision)

	// manual 元数据与其它字段保持不变。
	require.Equal(t, "Toggle", view.Name)
	require.Equal(t, "https://example.com/toggle", view.CanonicalKey)

	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, created.ID).Error)
	require.NotNil(t, stored.RecommendationEnabled)
	require.False(t, *stored.RecommendationEnabled)
	require.EqualValues(t, 2, stored.Revision)
	require.Equal(t, "Toggle", stored.ManualMetadata[ManualFieldName])

	// 不触碰 candidate_preferences：该表未迁移（不存在）→ -1；feeds 仍零行。
	require.EqualValues(t, -1, countRows(t, db, nil, "candidate_preferences"))
	require.EqualValues(t, 0, countRows(t, db, &models.Feed{}, "feeds"))
}

// TestCandidateCatalogUpdateRevisionConflict 乐观锁：旧 revision 更新返回 conflict，
// 库内值不被覆盖（spec：开关失败恢复原值语义的服务端保障）。
func TestCandidateCatalogUpdateRevisionConflict(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	created, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Lock", FeedURL: "https://example.com/lock",
	})
	require.NoError(t, err)

	// 传入过期 revision=5（实际为 1）→ conflict。
	_, err = svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{
		RecommendationEnabled: candidateBool(false), Revision: 5,
	})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeConflict, ce.Code)

	// 库内未被覆盖：enabled 仍 true、revision 仍 1。
	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, created.ID).Error)
	require.True(t, *stored.RecommendationEnabled)
	require.EqualValues(t, 1, stored.Revision)

	// revision=0（缺省）= 不校验版本，仍成功并自增。
	view, err := svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{
		RecommendationEnabled: candidateBool(false),
	})
	require.NoError(t, err)
	require.False(t, *view.RecommendationEnabled)
	require.EqualValues(t, 2, view.Revision)
}

// TestCandidateCatalogUpdateMetadata 编辑 manual 四字段：覆盖生效；空串删键回退；
// rss 候选清空名称拒绝（无上游可回退）；不存在的 id 返回 not_found。
func TestCandidateCatalogUpdateMetadata(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	created, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name:        "Origin",
		FeedURL:     "https://example.com/meta",
		Description: "old description",
		Language:    "en",
	})
	require.NoError(t, err)

	// 覆盖 name/description，清空 language（删键）。
	view, err := svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{
		Name:        candidateStr("Renamed"),
		Description: candidateStr("new description"),
		Language:    candidateStr(""),
		Revision:    created.Revision,
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed", view.Name)
	require.Equal(t, "new description", view.Description)
	require.Equal(t, "", view.Language) // 删键后无上游 → 空
	require.NotContains(t, view.ManualMetadata, ManualFieldLanguage)

	// 非法覆盖：空白名称 / 超长 description。
	_, err = svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{Name: candidateStr("  ")})
	require.Error(t, err)
	var vce *CandidateError
	require.ErrorAs(t, err, &vce)
	require.Equal(t, CandidateErrorCodeValidation, vce.Code)

	_, err = svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{Description: candidateStr(strings.Repeat("x", 4001))})
	require.Error(t, err)

	// rss 候选清空名称拒绝。
	_, err = svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{Name: candidateStr("")})
	require.Error(t, err)
	require.ErrorAs(t, err, &vce)
	require.Equal(t, CandidateErrorCodeValidation, vce.Code)

	// not found。
	_, err = svc.UpdateCandidate(ctx, 9999, CandidateUpdateInput{Name: candidateStr("x")})
	require.Error(t, err)
	require.ErrorAs(t, err, &vce)
	require.Equal(t, CandidateErrorCodeNotFound, vce.Code)
}

// TestCandidateCatalogUpdateFeedURLSuccess 原生 RSS 地址可编辑（spec feed-candidate-catalog：
// 「手动新增和编辑原生 RSS」）：地址规范化后写入 feed_url/canonical_key，稳定身份随之重算，
// revision+1；订阅状态按新地址重算（feeds.url 匹配）。
func TestCandidateCatalogUpdateFeedURLSuccess(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	created, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Editable", FeedURL: "https://example.com/old.xml",
	})
	require.NoError(t, err)
	require.False(t, created.Subscribed)

	// 新地址已经是订阅源（feeds.url = 规范化后的新地址）→ 编辑后订阅状态应随之翻转。
	newURL := "https://feeds.example.org/new?sort=desc"
	require.NoError(t, db.Create(&models.Feed{Title: "sub", URL: newURL}).Error)

	view, err := svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{
		FeedURL:  candidateStr("https://FEEDS.example.org:443/new?sort=desc#frag"),
		Revision: created.Revision,
	})
	require.NoError(t, err)
	require.EqualValues(t, created.Revision+1, view.Revision)
	require.NotNil(t, view.FeedURL)
	require.Equal(t, newURL, *view.FeedURL) // scheme/host 小写、默认端口与 fragment 已归一
	require.Equal(t, newURL, view.CanonicalKey)
	require.True(t, view.Subscribed) // 订阅判定跟着新地址走

	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, created.ID).Error)
	expectedKey, kerr := BuildRSSStableKey(newURL)
	require.NoError(t, kerr)
	require.Equal(t, expectedKey, stored.StableKey)                      // 稳定身份随地址重算
	require.Equal(t, "Editable", stored.ManualMetadata[ManualFieldName]) // 人工字段不受地址编辑影响
	require.EqualValues(t, 2, stored.Revision)

	// 空地址 = validation（地址为 rss 候选身份的一部分，不允许清空）。
	_, err = svc.UpdateCandidate(ctx, created.ID, CandidateUpdateInput{FeedURL: candidateStr("  ")})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeValidation, ce.Code)
}

// TestCandidateCatalogUpdateFeedURLConflict 改地址撞已有候选 → conflict 携 existing_id 且零写入
// （同一有效地址不得出现两条候选）；归一化等价变体同样命中冲突。
func TestCandidateCatalogUpdateFeedURLConflict(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	first, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "First", FeedURL: "https://example.com/first.xml",
	})
	require.NoError(t, err)
	second, err := svc.CreateCandidate(ctx, CandidateCreateInput{
		Name: "Second", FeedURL: "https://example.com/second.xml",
	})
	require.NoError(t, err)

	// 精确地址 + 归一化等价变体（大小写/默认端口/fragment）都命中同一冲突。
	for _, raw := range []string{
		"https://example.com/first.xml",
		"HTTPS://EXAMPLE.COM:443/first.xml#fragment",
	} {
		_, err = svc.UpdateCandidate(ctx, second.ID, CandidateUpdateInput{FeedURL: candidateStr(raw)})
		require.Error(t, err)
		var ce *CandidateError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, CandidateErrorCodeConflict, ce.Code)
		require.Equal(t, first.ID, ce.ExistingID)
	}

	// 零写入：目标条目地址与 revision 均未变。
	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, second.ID).Error)
	require.NotNil(t, stored.FeedURL)
	require.Equal(t, "https://example.com/second.xml", *stored.FeedURL)
	require.EqualValues(t, 1, stored.Revision)
	require.EqualValues(t, 2, countRows(t, db, &models.FeedCandidate{}, "feed_candidates"))
}

// TestCandidateCatalogUpdateFeedURLRejectsRSSHub rsshub 路由地址只读：传 feed_url 恒 validation，
// 不读上游表、不改行。
func TestCandidateCatalogUpdateFeedURLRejectsRSSHub(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	rsshubCand := models.FeedCandidate{
		StableKey: "test/ns/:id", Kind: "rsshub", CanonicalKey: "",
		ManualMetadata: models.MetadataMap{}, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&rsshubCand).Error)

	_, err := svc.UpdateCandidate(ctx, rsshubCand.ID, CandidateUpdateInput{
		FeedURL: candidateStr("https://example.com/hijack.xml"),
	})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeValidation, ce.Code)
	require.Contains(t, ce.Message, "read-only")

	var stored models.FeedCandidate
	require.NoError(t, db.First(&stored, rsshubCand.ID).Error)
	require.Nil(t, stored.FeedURL)
	require.EqualValues(t, 1, stored.Revision)
}

// TestCandidateCatalogGetRSSHubPreloadRoute rsshub 候选单条：预载上游 Route 原始资料，
// 人工覆盖优先、清空回退上游（EffectiveMetadata）；rss 候选 Route 为 nil。
func TestCandidateCatalogGetRSSHubPreloadRoute(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	route := models.RSSHubRoute{
		Namespace: "test", Path: "/ns/:id", Name: "Upstream Name",
		Description: "upstream description", Example: "/test/ns/1",
	}
	require.NoError(t, db.Create(&route).Error)
	rsshubCand := models.FeedCandidate{
		StableKey: "test/ns/:id", Kind: "rsshub", RouteID: &route.ID,
		CanonicalKey: "", ManualMetadata: models.MetadataMap{
			ManualFieldDescription: "manual description",
		},
		AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&rsshubCand).Error)

	view, err := svc.GetCandidate(ctx, rsshubCand.ID)
	require.NoError(t, err)
	require.NotNil(t, view.Route)
	require.Equal(t, route.ID, view.Route.ID)
	require.Equal(t, "upstream description", view.Route.Description)
	// 有效字段：名称回退上游、描述取人工覆盖。
	require.Equal(t, "Upstream Name", view.Name)
	require.Equal(t, "manual description", view.Description)
	require.False(t, view.Subscribed)

	// 清空人工描述（rsshub 允许）→ 回退上游。
	cleared, err := svc.UpdateCandidate(ctx, rsshubCand.ID, CandidateUpdateInput{
		Description: candidateStr(""),
		Revision:    rsshubCand.Revision,
	})
	require.NoError(t, err)
	require.Equal(t, "upstream description", cleared.Description)

	// not found。
	_, err = svc.GetCandidate(ctx, 4242)
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeNotFound, ce.Code)
}

// TestCandidateCatalogListPagination 分页：默认 30、上限 100 截断、page 翻页、total 正确。
func TestCandidateCatalogListPagination(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	// 种子：150 条（i 编码进 URL 保证稳定键唯一）。
	for i := 0; i < 150; i++ {
		cand := models.FeedCandidate{
			StableKey: fmt.Sprintf("rss:seed-%03d", i), Kind: "rss",
			FeedURL:      candidateStr(fmt.Sprintf("https://example.com/seed/%03d", i)),
			CanonicalKey: fmt.Sprintf("https://example.com/seed/%03d", i),
			ManualMetadata: models.MetadataMap{
				ManualFieldName: fmt.Sprintf("Seed %03d", i),
			},
			AccessScope: "public", Revision: 1,
		}
		require.NoError(t, db.Create(&cand).Error)
	}

	// 默认 page_size=30。
	res, err := svc.ListCandidates(ctx, CandidateListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 150, res.Total)
	require.Len(t, res.Items, 30)

	// page_size=500 → 截断为 100。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{PageSize: 500})
	require.NoError(t, err)
	require.Len(t, res.Items, 100)
	require.EqualValues(t, 150, res.Total)

	// page_size=100 翻页：page=2 取剩余 50。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{PageSize: 100, Page: 2})
	require.NoError(t, err)
	require.Len(t, res.Items, 50)

	// 非法 kind → validation。
	_, err = svc.ListCandidates(ctx, CandidateListQuery{Kind: "bogus"})
	require.Error(t, err)
	var ce *CandidateError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, CandidateErrorCodeValidation, ce.Code)
}

// TestCandidateCatalogListKeywordAndFilters 关键词命中 name/description/feed_url（含
// rsshub 上游名称）；kind / recommendation_enabled 筛选；subscribed 布尔按 feeds.url 精确匹配。
func TestCandidateCatalogListKeywordAndFilters(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	disabled := false
	seed := []models.FeedCandidate{
		{
			StableKey: "rss:kw-1", Kind: "rss",
			FeedURL: candidateStr("https://ai.example.com/feed"), CanonicalKey: "https://ai.example.com/feed",
			ManualMetadata:        models.MetadataMap{ManualFieldName: "AI Weekly", ManualFieldDescription: "机器学习周报"},
			RecommendationEnabled: candidateBool(false), AccessScope: "public", Revision: 1,
		},
		{
			StableKey: "rss:kw-2", Kind: "rss",
			FeedURL: candidateStr("https://news.example.com/rss"), CanonicalKey: "https://news.example.com/rss",
			ManualMetadata: models.MetadataMap{ManualFieldName: "Tech News", ManualFieldDescription: "科技新闻"},
			AccessScope:    "public", Revision: 1,
		},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}
	route := models.RSSHubRoute{Namespace: "kw", Path: "/hub", Name: "HubUpstreamName", Description: "hub desc"}
	require.NoError(t, db.Create(&route).Error)
	hub := models.FeedCandidate{
		StableKey: "kw/hub", Kind: "rsshub", RouteID: &route.ID,
		ManualMetadata: models.MetadataMap{}, AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&hub).Error)
	// 一条已订阅：feeds.url 与候选 feed_url 精确匹配。
	require.NoError(t, db.Create(&models.Feed{Title: "AI Weekly", URL: "https://ai.example.com/feed"}).Error)

	// 关键词命中 manual name。
	res, err := svc.ListCandidates(ctx, CandidateListQuery{Keyword: "ai weekly"})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, seed[0].ID, res.Items[0].ID)
	require.True(t, res.Items[0].Subscribed) // feeds.url 精确匹配

	// 关键词命中 manual description。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{Keyword: "科技新闻"})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, seed[1].ID, res.Items[0].ID)
	require.False(t, res.Items[0].Subscribed)

	// 关键词命中 feed_url。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{Keyword: "news.example.com"})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)

	// 关键词命中 rsshub 上游名称（经 LEFT JOIN）。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{Keyword: "hubupstreamname"})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, hub.ID, res.Items[0].ID)
	require.NotNil(t, res.Items[0].Route)

	// LIKE 通配符按字面匹配，不放大结果。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{Keyword: "%"})
	require.NoError(t, err)
	require.EqualValues(t, 0, res.Total)

	// kind 筛选。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{Kind: "rsshub"})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, "rsshub", res.Items[0].Kind)

	// recommendation_enabled=false 筛选。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{RecommendationEnabled: &disabled})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, seed[0].ID, res.Items[0].ID)

	// 空关键词（纯空白）= 不过滤。
	res, err = svc.ListCandidates(ctx, CandidateListQuery{Keyword: "   "})
	require.NoError(t, err)
	require.EqualValues(t, 3, res.Total)
}

// TestCandidateCatalogKeywordTruncatedTo500Runes 关键词 ≤500 runes 截断（design D9）：
// 超长关键词截断后仍按前缀命中，不报错。
func TestCandidateCatalogKeywordTruncatedTo500Runes(t *testing.T) {
	db := setupCandidateCatalogTestDB(t)
	svc := NewCandidateCatalogService(db)
	ctx := context.Background()

	cand := models.FeedCandidate{
		StableKey: "rss:kw-long", Kind: "rss",
		FeedURL:      candidateStr("https://example.com/long"),
		CanonicalKey: "https://example.com/long",
		ManualMetadata: models.MetadataMap{
			ManualFieldName: strings.Repeat("长", 600),
		},
		AccessScope: "public", Revision: 1,
	}
	require.NoError(t, db.Create(&cand).Error)

	res, err := svc.ListCandidates(ctx, CandidateListQuery{Keyword: strings.Repeat("长", 600)})
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total) // 截断到 500 runes 后仍命中 600-rune 名称
}
