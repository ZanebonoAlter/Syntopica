package database_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── improve-discovery-recommendations 2.2/2.3：PG-only 迁移收尾的隔离库验证 ──
//
// 覆盖 test-cases.md S7（旧数据可恢复迁移）主链路 1/2：
//   - 带脏 seed 升级：旧 seed 标 legacy_inactive、不编造原查询、订阅计数不变；
//   - 迁移重跑：幂等无副本；
//   - pending 部分唯一索引：同 hash 第二条 pending 被拒、legacy 同 hash 多条允许。
//
// 迁移入口是 database.RunAutoMigrate 尾部内联的 runDiscoveryV2Backfill（PG-only），
// 测试直接调用真实入口 RunAutoMigrate 驱动。模拟「未迁移旧库」的方式：DROP 掉 golden
// schema 在空库上预建的部分唯一索引（哨兵复位）；旧全表唯一索引 idx_feed_recommendations_hash
// 在真实旧库存在、但那使同 hash 多行 pending 无法存在，故不重建（模拟历史非唯一/半迁移库）。

// discoveryRecRow 是 feed_recommendations 断言用的最小投影。
type discoveryRecRow struct {
	ID          uint
	Status      string
	ExpiresAt   *time.Time
	CandidateID *uint
	DismissedAt *time.Time
}

func discoveryRecByID(t *testing.T, db *gorm.DB, id uint) discoveryRecRow {
	t.Helper()
	var row discoveryRecRow
	require.NoError(t, db.Raw(
		`SELECT id, status, expires_at, candidate_id, dismissed_at FROM feed_recommendations WHERE id = ?`, id,
	).Scan(&row).Error)
	return row
}

func discoveryCount(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(query, args...).Scan(&n).Error)
	return n
}

// TestDiscoveryV2BackfillLegacyUpgradeAndIdempotentRerun 走完整迁移故事：
// 首迁（升级旧库）→ 全量断言 → 重跑计数不变 → 新系统写入的新 pending 不被迁移触碰。
func TestDiscoveryV2BackfillLegacyUpgradeAndIdempotentRerun(t *testing.T) {
	db := testutil.SetupTestDB(t)

	// 模拟未迁移旧库态：golden schema 在空库上预建了部分唯一索引（哨兵），先复位它，
	// 才能插入同 hash 多条 pending 的旧行（旧库不存在该索引）。
	require.NoError(t, db.Exec(`DROP INDEX IF EXISTS idx_feed_recommendations_hash_pending`).Error)

	// ── fixture：模拟升级前旧库 ──
	now := time.Now().UTC()
	board := models.SemanticLabel{Label: "迁移测试版块", Slug: "mig-test-board", LabelType: "board", Status: "active"}
	require.NoError(t, db.Create(&board).Error)

	routes := []models.RSSHubRoute{
		// namespace/path 大小写混合：stable_key 必须收敛为全小写。Parameters 是 jsonb 列的
		// 原始 JSON 字符串，必须给合法 JSON（空串会被 PG 拒绝）。
		{Namespace: "Blogs", Path: "User", Name: "用户博客", URL: "https://rsshub.local/blogs/user", ContentHash: "h-blogs", Parameters: "{}"},
		{Namespace: "news", Path: "Tech", Name: "科技新闻", ContentHash: "h-news", Parameters: "{}"},
		{Namespace: "media", Path: "Video/List", Name: "视频列表", ContentHash: "h-media", Parameters: "{}"},
	}
	for i := range routes {
		require.NoError(t, db.Create(&routes[i]).Error)
	}

	// 旧 seed 两条（全局桶 + 具体版块）+ behavior 一条（不得迁移）。
	seedGlobal := models.PreferenceVector{
		Source: "seed", EmbeddingVec: "[1,0,0]", Dimension: 3, Model: "mig-embed",
		TagWeights: models.MetadataMap{}, LastComputedAt: now,
	}
	seedBoard := models.PreferenceVector{
		BoardID: &board.ID, Source: "seed", EmbeddingVec: "[0,1,0]", Dimension: 3, Model: "mig-embed",
		TagWeights: models.MetadataMap{}, LastComputedAt: now,
	}
	behaviorVec := models.PreferenceVector{
		Source: "behavior", EmbeddingVec: "[0,0,1]", Dimension: 3, Model: "mig-embed",
		TagWeights: models.MetadataMap{}, LastComputedAt: now,
	}
	require.NoError(t, db.Create(&seedGlobal).Error)
	require.NoError(t, db.Create(&seedBoard).Error)
	require.NoError(t, db.Create(&behaviorVec).Error)

	// 旧推荐：同 hash 两条 pending（重复组）、一条独立 hash pending、一条 accepted、
	// 一条 30 天内 dismissed、一条 35 天前 dismissed。
	dupFirst := models.FeedRecommendation{RouteID: routes[0].ID, Source: "qa", Status: "pending", RecommendationHash: "mig-dup-hash", Score: 0.5}
	dupSecond := models.FeedRecommendation{RouteID: routes[0].ID, Source: "manual_refresh", Status: "pending", RecommendationHash: "mig-dup-hash", Score: 0.6}
	singlePending := models.FeedRecommendation{RouteID: routes[1].ID, Source: "qa", Status: "pending", RecommendationHash: "mig-single-hash", Score: 0.7}
	accepted := models.FeedRecommendation{RouteID: routes[2].ID, Source: "qa", Status: "accepted", RecommendationHash: "mig-acc-hash", Score: 0.9}
	dismissedRecentAt := now.Add(-5 * 24 * time.Hour)
	dismissedRecent := models.FeedRecommendation{RouteID: routes[1].ID, Source: "qa", Status: "dismissed", RecommendationHash: "mig-diss-new", DismissedAt: &dismissedRecentAt}
	dismissedOldAt := now.Add(-35 * 24 * time.Hour)
	dismissedOld := models.FeedRecommendation{RouteID: routes[2].ID, Source: "qa", Status: "dismissed", RecommendationHash: "mig-diss-old", DismissedAt: &dismissedOldAt}
	for _, rec := range []*models.FeedRecommendation{&dupFirst, &dupSecond, &singlePending, &accepted, &dismissedRecent, &dismissedOld} {
		require.NoError(t, db.Create(rec).Error)
	}
	require.Greater(t, dupSecond.ID, dupFirst.ID, "后插入的重复组行 id 更大，是去重时保留者")

	// 已有订阅：feeds 行数前后必须不变。
	feedA := models.Feed{Title: "已有订阅A", URL: "https://example.com/a.xml"}
	feedB := models.Feed{Title: "已有订阅B", URL: "https://example.com/b.xml"}
	require.NoError(t, db.Create(&feedA).Error)
	require.NoError(t, db.Create(&feedB).Error)
	feedsBefore := discoveryCount(t, db, `SELECT count(*) FROM feeds`)
	require.Equal(t, int64(2), feedsBefore)

	// ── 首次迁移（真实入口）──
	require.NoError(t, database.RunAutoMigrate(db))

	// b. 部分唯一索引已建。
	require.Equal(t, int64(1), discoveryCount(t, db,
		`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname='idx_feed_recommendations_hash_pending'`),
		"首迁后必须存在 pending 部分唯一索引")

	// c. 候选数=路由数且 stable_key 小写收敛。
	require.Equal(t, int64(len(routes)), discoveryCount(t, db, `SELECT count(*) FROM feed_candidates`),
		"候选数必须等于路由数")
	var stableKeys []string
	require.NoError(t, db.Raw(`SELECT stable_key FROM feed_candidates ORDER BY stable_key`).Scan(&stableKeys).Error)
	require.Equal(t, []string{"blogs/user", "media/video/list", "news/tech"}, stableKeys,
		"stable_key 必须是全小写 namespace/path")
	var candBlogs struct {
		Kind                  string
		RouteID               *uint
		RecommendationEnabled bool
		AccessScope           string
		Revision              int
	}
	require.NoError(t, db.Raw(
		`SELECT kind, route_id, recommendation_enabled, access_scope, revision FROM feed_candidates WHERE stable_key='blogs/user'`,
	).Scan(&candBlogs).Error)
	require.Equal(t, "rsshub", candBlogs.Kind)
	require.NotNil(t, candBlogs.RouteID)
	require.Equal(t, routes[0].ID, *candBlogs.RouteID)
	require.True(t, candBlogs.RecommendationEnabled)
	require.Equal(t, "public", candBlogs.AccessScope)
	require.Equal(t, 1, candBlogs.Revision)

	// a. 重复 hash 只剩一条 pending（保留 id 最大者）其余 legacy，且 legacy 带 expires_at。
	require.Equal(t, int64(1), discoveryCount(t, db,
		`SELECT count(*) FROM feed_recommendations WHERE recommendation_hash='mig-dup-hash' AND status='pending'`),
		"重复 hash 的 pending 只能剩一条")
	require.Equal(t, int64(1), discoveryCount(t, db,
		`SELECT count(*) FROM feed_recommendations WHERE recommendation_hash='mig-dup-hash' AND status='legacy'`),
		"重复组非最大 id 必须转 legacy")
	dupLegacyRow := discoveryRecByID(t, db, dupFirst.ID)
	require.Equal(t, "legacy", dupLegacyRow.Status)
	require.NotNil(t, dupLegacyRow.ExpiresAt, "转 legacy 的旧 pending 必须带 expires_at")
	dupSurvivor := discoveryRecByID(t, db, dupSecond.ID)
	require.Equal(t, "pending", dupSurvivor.Status)
	require.Greater(t, dupSurvivor.ID, dupLegacyRow.ID)

	// a. 旧 pending（独立 hash）全部转 legacy 且带 expires_at，不伪装成新精排结果。
	singleRow := discoveryRecByID(t, db, singlePending.ID)
	require.Equal(t, "legacy", singleRow.Status, "迁移前已存在的旧 pending 必须进 legacy 历史")
	require.NotNil(t, singleRow.ExpiresAt)

	// accepted / dismissed 状态保持。
	accRow := discoveryRecByID(t, db, accepted.ID)
	require.Equal(t, "accepted", accRow.Status)
	require.Nil(t, accRow.ExpiresAt)
	require.Equal(t, "dismissed", discoveryRecByID(t, db, dismissedRecent.ID).Status)
	require.Equal(t, "dismissed", discoveryRecByID(t, db, dismissedOld.ID).Status)

	// d. candidate_id 回填非空（全部旧推荐按 route 对应到候选）。
	require.Equal(t, int64(0), discoveryCount(t, db,
		`SELECT count(*) FROM feed_recommendations WHERE candidate_id IS NULL`),
		"全部旧推荐必须回填 candidate_id")
	require.NotNil(t, discoveryRecByID(t, db, dupFirst.ID).CandidateID)
	var dupCandID uint
	require.NoError(t, db.Raw(
		`SELECT candidate_id FROM feed_recommendations WHERE id = ?`, dupFirst.ID).Scan(&dupCandID).Error)
	var blogsCandID uint
	require.NoError(t, db.Raw(`SELECT id FROM feed_candidates WHERE stable_key='blogs/user'`).Scan(&blogsCandID).Error)
	require.Equal(t, blogsCandID, dupCandID, "candidate_id 必须指向该路由的候选")

	// e. seed 两条迁为 legacy_inactive（run_id NULL、不编造原查询），behavior 不迁移。
	require.Equal(t, int64(2), discoveryCount(t, db, `SELECT count(*) FROM discovery_interest_entries`),
		"只有 source=seed 的旧向量迁移")
	type entryRow struct {
		RunID     *uint
		QueryText string
		BoardID   *uint
		Status    string
		LegacyRef string
	}
	var entries []entryRow
	require.NoError(t, db.Raw(
		`SELECT run_id, query_text, board_id, status, legacy_ref FROM discovery_interest_entries ORDER BY id`).Scan(&entries).Error)
	require.Len(t, entries, 2)
	for _, e := range entries {
		require.Nil(t, e.RunID, "legacy 迁移行无原始 run，run_id 必须 NULL（NULL 不参与唯一约束）")
		require.Equal(t, "legacy_inactive", e.Status)
		require.Equal(t, "legacy seed（无原始查询）", e.QueryText, "不得编造原查询")
	}
	byRef := map[string]entryRow{}
	for _, e := range entries {
		byRef[e.LegacyRef] = e
	}
	globalRef := "preference_vectors:" + strconv.FormatUint(uint64(seedGlobal.ID), 10)
	boardRef := "preference_vectors:" + strconv.FormatUint(uint64(seedBoard.ID), 10)
	require.Contains(t, byRef, globalRef)
	require.Contains(t, byRef, boardRef)
	require.Nil(t, byRef[globalRef].BoardID, "全局桶 seed 迁移后 board_id 保持 NULL")
	require.NotNil(t, byRef[boardRef].BoardID)
	require.Equal(t, board.ID, *byRef[boardRef].BoardID, "版块 seed 迁移后 board_id 保持指向")
	require.NotContains(t, byRef, "preference_vectors:"+strconv.FormatUint(uint64(behaviorVec.ID), 10),
		"behavior 源不得被迁移（行为画像走 scheduler 重算通道）")

	// f. 30 天内 dismissed 推导剩余冷却；35 天前的不生成。
	require.Equal(t, int64(1), discoveryCount(t, db, `SELECT count(*) FROM candidate_preferences`),
		"只有 30 天内 dismissed 的候选生成冷却行")
	var prefRow struct {
		CandidateID  uint
		SnoozedUntil time.Time
		Note         string
	}
	require.NoError(t, db.Raw(
		`SELECT candidate_id, snoozed_until, note FROM candidate_preferences`).Scan(&prefRow).Error)
	newsCandID := discoveryCount(t, db, `SELECT id FROM feed_candidates WHERE stable_key='news/tech'`)
	require.Equal(t, newsCandID, int64(prefRow.CandidateID), "冷却行必须挂在近期 dismissed 路由的候选上")
	expectedSnooze := dismissedRecentAt.Add(30 * 24 * time.Hour)
	diff := prefRow.SnoozedUntil.Sub(expectedSnooze)
	require.LessOrEqual(t, diff.Abs(), 2*time.Minute, "snoozed_until = dismissed_at + 30 天")
	require.Equal(t, "legacy dismissed", prefRow.Note)
	mediaCandID := discoveryCount(t, db, `SELECT id FROM feed_candidates WHERE stable_key='media/video/list'`)
	require.Equal(t, int64(0), discoveryCount(t, db,
		`SELECT count(*) FROM candidate_preferences WHERE candidate_id = ?`, mediaCandID),
		"35 天前 dismissed 已自然到期，不生成冷却行")

	// feeds 行数不变。
	require.Equal(t, feedsBefore, discoveryCount(t, db, `SELECT count(*) FROM feeds`), "迁移不得触碰已有订阅")

	// ── 重跑：计数不变、无副本 ──
	type snap struct {
		candidates, recs, pending, legacy, entries, prefs, feeds int64
	}
	takeSnap := func() snap {
		return snap{
			candidates: discoveryCount(t, db, `SELECT count(*) FROM feed_candidates`),
			recs:       discoveryCount(t, db, `SELECT count(*) FROM feed_recommendations`),
			pending:    discoveryCount(t, db, `SELECT count(*) FROM feed_recommendations WHERE status='pending'`),
			legacy:     discoveryCount(t, db, `SELECT count(*) FROM feed_recommendations WHERE status='legacy'`),
			entries:    discoveryCount(t, db, `SELECT count(*) FROM discovery_interest_entries`),
			prefs:      discoveryCount(t, db, `SELECT count(*) FROM candidate_preferences`),
			feeds:      discoveryCount(t, db, `SELECT count(*) FROM feeds`),
		}
	}
	before := takeSnap()
	require.NoError(t, database.RunAutoMigrate(db))
	require.Equal(t, before, takeSnap(), "迁移重跑必须幂等：各表计数不变")
	require.Equal(t, "pending", discoveryRecByID(t, db, dupSecond.ID).Status,
		"重跑不得把首迁保留的 pending 又转 legacy")

	// ── 新系统上线后写入的新 pending 不受迁移影响（哨兵 + cutoff 保护）──
	require.NoError(t, db.Exec(`
		INSERT INTO feed_recommendations (route_id, source, score, status, recommendation_hash, created_at, updated_at)
		VALUES (?, 'qa', 0.8, 'pending', 'mig-new-system-hash', now(), now())`, routes[2].ID).Error)
	require.NoError(t, database.RunAutoMigrate(db))
	require.Equal(t, int64(2), discoveryCount(t, db,
		`SELECT count(*) FROM feed_recommendations WHERE status='pending'`),
		"新系统写入的 pending 必须在后续启动的迁移中保持 pending")
	require.Equal(t, "pending", discoveryRecByID(t, db, dupSecond.ID).Status)
}

// TestDiscoveryV2PendingPartialUniqueIndex 直接 SQL 断言索引契约：
// 同 hash 第二条 pending 违反 idx_feed_recommendations_hash_pending；legacy 同 hash 允许多条。
func TestDiscoveryV2PendingPartialUniqueIndex(t *testing.T) {
	db := testutil.SetupTestDB(t)
	// golden schema 已在空库上建好部分唯一索引；防御性重跑一次入口确保索引存在。
	require.NoError(t, database.RunAutoMigrate(db))

	ins := func(status, hash string) error {
		return db.Exec(`
			INSERT INTO feed_recommendations (route_id, source, score, status, recommendation_hash, created_at, updated_at)
			VALUES (1, 'qa', 0.5, ?, ?, now(), now())`, status, hash).Error
	}

	// 同 hash 两条 pending：第二条必须被部分唯一索引拒绝。
	require.NoError(t, ins("pending", "partial-idx-hash"))
	err := ins("pending", "partial-idx-hash")
	require.Error(t, err, "同 hash 第二条 pending 必须违反 idx_feed_recommendations_hash_pending")
	require.Contains(t, err.Error(), "idx_feed_recommendations_hash_pending")

	// legacy 历史行允许同 hash 多条（旧全表唯一约束已退役）。
	require.NoError(t, ins("legacy", "partial-idx-legacy"))
	require.NoError(t, ins("legacy", "partial-idx-legacy"))
	require.Equal(t, int64(2), discoveryCount(t, db,
		`SELECT count(*) FROM feed_recommendations WHERE recommendation_hash='partial-idx-legacy'`))
}
