package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/testutil"
)

// ── improve-discovery-recommendations 4.4：推荐生命周期与排除（design D5 / test-cases S5）──
//
// 覆盖：TTL/冷却配置校验、到期判据两端、暂时不看与撤销、长期排除与恢复（恢复只清字段）、
// 跨 qa/refresh source 隔离、发布前并发排除复查、失败不碰时间戳、历史四状态分类与
// 旧 dismissed 行只读。

// candidateIDForRoute 取路由对应的统一候选 id（4.3 起召回/生命周期都走 candidate）。
func candidateIDForRoute(t *testing.T, db *gorm.DB, routeID uint) uint {
	t.Helper()
	var c models.FeedCandidate
	require.NoError(t, db.Where("route_id = ?", routeID).First(&c).Error)
	return c.ID
}

// recForRoute 取指定路由的推荐行（测试断言用）。
func recForRoute(t *testing.T, db *gorm.DB, routeID uint, status string) models.FeedRecommendation {
	t.Helper()
	var rec models.FeedRecommendation
	require.NoError(t, db.Where("route_id = ? AND status = ?", routeID, status).First(&rec).Error)
	return rec
}

// ── 配置（D5：TTL=14、冷却=30，均 1–365，读取即校验）──

// TestLifecycleConfig_DefaultsPartialAndRanges：键缺失回默认；部分覆盖只动出现字段；
// 越界/类型错/坏 JSON 逐项报错；save/load roundtrip。
func TestLifecycleConfig_DefaultsPartialAndRanges(t *testing.T) {
	db := testutil.SetupTestDB(t)

	cfg, err := loadLifecycleConfig(db)
	require.NoError(t, err)
	require.Equal(t, DefaultDiscoveryLifecycleConfig(), cfg)
	require.Equal(t, 14, cfg.RecommendationTTLDays)
	require.Equal(t, 30, cfg.SnoozeDays)

	// 部分覆盖：只改 snooze_days，其余回默认。
	require.NoError(t, db.Create(&models.AISettings{
		Key: lifecycleConfigKey, Value: `{"snooze_days": 7}`,
	}).Error)
	cfg, err = loadLifecycleConfig(db)
	require.NoError(t, err)
	require.Equal(t, 7, cfg.SnoozeDays)
	require.Equal(t, 14, cfg.RecommendationTTLDays)

	// 越界逐项报错（含字段名）。
	require.ErrorContains(t, DiscoveryLifecycleConfig{RecommendationTTLDays: 0, SnoozeDays: 30}.Validate(), "recommendation_ttl_days")
	require.ErrorContains(t, DiscoveryLifecycleConfig{RecommendationTTLDays: 366, SnoozeDays: 30}.Validate(), "recommendation_ttl_days")
	require.ErrorContains(t, DiscoveryLifecycleConfig{RecommendationTTLDays: 14, SnoozeDays: 0}.Validate(), "snooze_days")
	require.ErrorContains(t, DiscoveryLifecycleConfig{RecommendationTTLDays: 14, SnoozeDays: 366}.Validate(), "snooze_days")
	require.NoError(t, DiscoveryLifecycleConfig{RecommendationTTLDays: 1, SnoozeDays: 365}.Validate())

	// 读取即校验：非法值不得静默回落默认。
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", lifecycleConfigKey).
		Update("value", `{"recommendation_ttl_days": 0, "snooze_days": 30}`).Error)
	_, err = loadLifecycleConfig(db)
	require.ErrorContains(t, err, "recommendation_ttl_days")

	// 类型非法。
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", lifecycleConfigKey).
		Update("value", `{"snooze_days": "30"}`).Error)
	_, err = loadLifecycleConfig(db)
	require.ErrorContains(t, err, "snooze_days")

	// roundtrip。
	require.NoError(t, saveLifecycleConfig(db, DiscoveryLifecycleConfig{RecommendationTTLDays: 3, SnoozeDays: 9}))
	cfg, err = loadLifecycleConfig(db)
	require.NoError(t, err)
	require.Equal(t, 3, cfg.RecommendationTTLDays)
	require.Equal(t, 9, cfg.SnoozeDays)
	require.Error(t, saveLifecycleConfig(db, DiscoveryLifecycleConfig{RecommendationTTLDays: 14, SnoozeDays: 0}), "非法配置不得落库")
}

// ── 到期（D5：now >= expires_at 排他；到期转历史，不写 dismissed_at）──

// TestLifecycle_ExpiryBoundaryAndDefaultList：发布写出 TTL 到期时刻；到期卡退出默认列表，
// 历史标 expired；回拨到未到期即回归；两边各走一步（now-1s 出局 / now+1s 保留）。
func TestLifecycle_ExpiryBoundaryAndDefaultList(t *testing.T) {
	db := testutil.SetupTestDB(t)
	r1, _, _ := setupRecFixture(t, db)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)

	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)
	rec := recForRoute(t, db, r1, "pending")
	require.NotNil(t, rec.ExpiresAt)
	require.NotNil(t, rec.LastSelectedAt)
	require.WithinDuration(t, time.Now().AddDate(0, 0, 14), *rec.ExpiresAt, 2*time.Minute, "expires_at = now + recommendation_ttl_days")

	// 未到期 → 在默认列表。
	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.True(t, hasRouteCard(cards, r1))

	// 到期（now-1s）→ 退出默认列表，历史标 expired，且绝不写 dismissed_at。
	require.NoError(t, db.Model(&models.FeedRecommendation{}).Where("id = ?", rec.ID).
		Update("expires_at", time.Now().Add(-1*time.Second)).Error)
	cards, err = svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.False(t, hasRouteCard(cards, r1), "到期卡退出默认列表")

	history, err := svc.GetRecommendationHistory(context.Background())
	require.NoError(t, err)
	entry := findHistory(history, rec.ID)
	require.NotNil(t, entry)
	require.Equal(t, HistoryStatusExpired, entry.Status)
	require.Nil(t, entry.SnoozedUntil, "自动过期不得携带「暂时不看/不感兴趣」语义")

	var after models.FeedRecommendation
	require.NoError(t, db.First(&after, rec.ID).Error)
	require.Nil(t, after.DismissedAt, "自动过期 ≠ 拒绝：不写 dismissed_at")
	require.Equal(t, "pending", after.Status, "到期只改展示去向，资格仍可恢复")

	// 回拨到未到期（now+1s）→ 重新出现在默认列表。
	require.NoError(t, db.Model(&models.FeedRecommendation{}).Where("id = ?", rec.ID).
		Update("expires_at", time.Now().Add(1*time.Second)).Error)
	cards, err = svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.True(t, hasRouteCard(cards, r1))
}

// ── 暂时不看（冷却权威 candidate_preferences，两端边界 + 撤销）──

// TestLifecycle_SnoozeBoundaryAndRevoke：暂时不看写 snoozed_until=now+30d 并返回到期时刻；
// 冷却中退出默认列表（推荐行状态不变、无 dismissed_at）；now-1s 恢复可见；撤销清字段后
// 重新可推、不产生新行。
func TestLifecycle_SnoozeBoundaryAndRevoke(t *testing.T) {
	db := testutil.SetupTestDB(t)
	r1, _, _ := setupRecFixture(t, db)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)

	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)
	rec := recForRoute(t, db, r1, "pending")
	candID := candidateIDForRoute(t, db, r1)

	until, err := svc.SnoozeRecommendation(context.Background(), rec.ID)
	require.NoError(t, err)
	require.NotNil(t, until)
	require.WithinDuration(t, time.Now().AddDate(0, 0, 30), *until, 2*time.Minute, "返回实际到期 = now + snooze_days")

	var pref models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", candID).First(&pref).Error)
	require.NotNil(t, pref.SnoozedUntil)
	require.Nil(t, pref.ExcludedAt, "暂时不看不动 excluded_at")

	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.False(t, hasRouteCard(cards, r1), "冷却中退出默认列表")

	// 推荐行本身不被改写（冷却权威在 candidate_preferences）。
	var mid models.FeedRecommendation
	require.NoError(t, db.First(&mid, rec.ID).Error)
	require.Equal(t, "pending", mid.Status)
	require.Nil(t, mid.DismissedAt)

	// 冷却到期（now-1s）→ 重新可见。
	require.NoError(t, db.Model(&models.CandidatePreference{}).Where("id = ?", pref.ID).
		Update("snoozed_until", time.Now().Add(-1*time.Second)).Error)
	cards, err = svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.True(t, hasRouteCard(cards, r1), "到达冷却结束可重新参与")

	// 再暂时不看 → 撤销（restore）只清字段，可再次被推，且不产生新推荐行。
	_, err = svc.SnoozeRecommendation(context.Background(), rec.ID)
	require.NoError(t, err)
	require.NoError(t, svc.RestoreRecommendation(context.Background(), rec.ID))
	var restored models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", candID).First(&restored).Error)
	require.Nil(t, restored.SnoozedUntil)
	require.Nil(t, restored.ExcludedAt)

	cards, err = svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.True(t, hasRouteCard(cards, r1))

	var cnt int64
	require.NoError(t, db.Model(&models.FeedRecommendation{}).Where("route_id = ?", r1).Count(&cnt).Error)
	require.EqualValues(t, 1, cnt, "撤销不产生新推荐行")
}

// ── 长期排除 / 恢复（跨 source 隔离；enabled 开关不解除排除；恢复不是订阅）──

// TestLifecycle_ExcludeCrossSourceAndEnabledToggle：长期排除后 ask 与 refresh 都不出该卡；
// 目录 enabled 开关（关→开）不得解除排除；恢复只清字段（不建订阅、不立即出卡、不新增行）。
func TestLifecycle_ExcludeCrossSourceAndEnabledToggle(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 2)
	// 全局行为画像（refresh 召回入口），与候选向量同模型同维。
	require.NoError(t, db.Create(&models.PreferenceVector{
		BoardID: nil, Source: PreferenceSourceBehavior,
		EmbeddingVec: floatsToPgVector([]float64{1, 0, 0}), Dimension: 3, Model: "test",
	}).Error)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)

	run, err := svc.runService().StartAsk(context.Background(), "首轮查询", "seed-key")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status)
	candID := ids[0]
	rec := recForRoute(t, db, routeIDForCandidate(t, db, candID), "pending")

	require.NoError(t, svc.ExcludeRecommendation(context.Background(), rec.ID))
	var pref models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", candID).First(&pref).Error)
	require.NotNil(t, pref.ExcludedAt)

	cards, err := svc.GetRecommendations(context.Background(), "pending")
	require.NoError(t, err)
	require.False(t, hasRouteCard(cards, routeIDForCandidate(t, db, ids[0])))
	require.True(t, hasRouteCard(cards, routeIDForCandidate(t, db, ids[1])), "其他候选不受影响")

	// 候选 enabled 开关往返不得解除排除（写路径互不相通）。
	off, on := false, true
	require.NoError(t, db.Model(&models.FeedCandidate{}).Where("id = ?", candID).
		Update("recommendation_enabled", &off).Error)
	require.NoError(t, db.Model(&models.FeedCandidate{}).Where("id = ?", candID).
		Update("recommendation_enabled", &on).Error)
	var afterToggle models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", candID).First(&afterToggle).Error)
	require.NotNil(t, afterToggle.ExcludedAt, "目录推荐开关不得解除长期排除")

	// 跨 source：ask（qa）与 refresh（manual_refresh）都被召回资格过滤拦截。
	run2, err := svc.runService().StartAsk(context.Background(), "排除后查询", "exclude-key")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run2.Status)
	require.False(t, runHasCandidate(t, db, run2.ID, candID), "排除后 ask 不出该卡")
	require.True(t, runHasCandidate(t, db, run2.ID, ids[1]))

	summary, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)
	require.False(t, runHasCandidate(t, db, summary.RunID, candID), "排除后 refresh 不出该卡")

	// 恢复：仅清字段（不建订阅、不立即出卡、不新增推荐行）。
	var feedsBefore, recsBefore, runsBefore int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&feedsBefore).Error)
	require.NoError(t, db.Model(&models.FeedRecommendation{}).Count(&recsBefore).Error)
	require.NoError(t, db.Model(&models.DiscoveryRun{}).Count(&runsBefore).Error)

	require.NoError(t, svc.RestoreRecommendation(context.Background(), rec.ID))
	var restored models.CandidatePreference
	require.NoError(t, db.Where("candidate_id = ?", candID).First(&restored).Error)
	require.Nil(t, restored.ExcludedAt)
	require.Nil(t, restored.SnoozedUntil)

	var feedsAfter, recsAfter, runsAfter int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&feedsAfter).Error)
	require.NoError(t, db.Model(&models.FeedRecommendation{}).Count(&recsAfter).Error)
	require.NoError(t, db.Model(&models.DiscoveryRun{}).Count(&runsAfter).Error)
	require.Equal(t, feedsBefore, feedsAfter, "恢复不建订阅")
	require.Equal(t, recsBefore, recsAfter, "恢复不立即生成新卡")
	require.Equal(t, runsBefore, runsAfter, "恢复不触发新 run")
}

// TestLifecycle_PublishRecheckBlocksConcurrentExclude：召回通过后、发布前用户刚排除
// （模拟并发窗口）→ 发布事务内复查 candidate_preferences 跳过该卡，不发布、run 仍成功。
func TestLifecycle_PublishRecheckBlocksConcurrentExclude(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 1)
	candID := ids[0]

	client := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "理由"}), nil
	}}
	router := newMockAIRouter(t, db, client, false)
	svc := NewDiscoveryRunService(db, router, nil)
	svc.recall = &fakeRecallBatcher{askFn: func(context.Context, string, []float64, int, string) ([]recallBatch, error) {
		// 模拟「召回已通过 → 用户排除」的并发窗口。
		require.NoError(t, db.Create(&models.CandidatePreference{
			CandidateID: candID, ExcludedAt: ptrTime(time.Now()),
		}).Error)
		return []recallBatch{{
			boardLabel: "全局", queryText: "q", source: RecommendationSourceQA, path: recallPathQuery,
			candidates: wrapCandidates([]candidateRow{{
				CandidateID: ids[0], Namespace: "ns0", Path: "/p0", Name: "Route0",
				Example: "/ns0/p0", UsableDirectly: true, Distance: 0.1,
			}}, recallPathQuery),
		}}, nil
	}}

	run, err := svc.StartAsk(context.Background(), "并发排除", "concurrent-exclude")
	require.NoError(t, err)
	require.Equal(t, "succeeded", run.Status, "被跳过不是发布失败（整轮仍成功）")
	require.Equal(t, 1, svc.lastPublish.Cooldown, "复查命中候选偏好阻断")
	require.Zero(t, svc.lastPublish.Inserted)

	var pending, items int64
	require.NoError(t, db.Model(&models.FeedRecommendation{}).Count(&pending).Error)
	require.NoError(t, db.Model(&models.DiscoveryRunItem{}).Count(&items).Error)
	require.Zero(t, pending, "被排除候选不发布 pending")
	require.Zero(t, items, "run 无该候选项快照")
}

// TestLifecycle_FailedRefreshKeepsTimestamps：刷新失败（精排协议异常）不碰既有 pending 的
// last_selected_at / expires_at，也不人为延期（已自然到期卡仍进历史）。
func TestLifecycle_FailedRefreshKeepsTimestamps(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ids := setupAskFixture(t, db, 1)

	okClient := &mockDiscoveryClient{chatFn: func(airouter.ChatRequest) (string, error) {
		return strictSelectionJSON([2]any{ids[0], "首轮"}), nil
	}}
	router := newMockAIRouter(t, db, okClient, false)
	svc := NewDiscoveryRunService(db, router, nil)
	_, err := svc.StartAsk(context.Background(), "首轮查询", "ok-key")
	require.NoError(t, err)
	rec := recForRoute(t, db, routeIDForCandidate(t, db, ids[0]), "pending")
	require.NotNil(t, rec.LastSelectedAt)
	require.NotNil(t, rec.ExpiresAt)

	// 第二轮：召回返回同候选，但精排返回不可解析内容 → 整轮失败、不发布。
	okClient.chatFn = func(airouter.ChatRequest) (string, error) { return "这不是 JSON", nil }
	svc.recall = &fakeRecallBatcher{refreshFn: func(context.Context) ([]recallBatch, error) {
		return []recallBatch{{
			boardLabel: "全局", source: RecommendationSourceManualRefresh, path: recallPathBehavior,
			candidates: wrapCandidates([]candidateRow{{
				CandidateID: ids[0], Namespace: "ns0", Path: "/p0", Name: "Route0",
				Example: "/ns0/p0", UsableDirectly: true, Distance: 0.1,
			}}, recallPathBehavior),
		}}, nil
	}}
	_, err = svc.Refresh(context.Background())
	require.Error(t, err)

	var after models.FeedRecommendation
	require.NoError(t, db.First(&after, rec.ID).Error)
	require.Equal(t, *rec.LastSelectedAt, *after.LastSelectedAt, "失败不刷新 last_selected_at")
	require.Equal(t, *rec.ExpiresAt, *after.ExpiresAt, "失败不人为延期")
	require.Equal(t, "pending", after.Status)
	require.Nil(t, after.DismissedAt)
}

// ── 历史视图四状态分类 + legacy/dismissed 只读 ──

// TestLifecycle_HistoryClassificationAndLegacyReadOnly：accepted/expired/snoozed/excluded
// 四状态分类；legacy 迁移行与旧 dismissed 行归 expired 且只读（restore 不复活）；
// 活跃未到期 pending 不进历史；snoozed 带到期时间，expired 不带。
func TestLifecycle_HistoryClassificationAndLegacyReadOnly(t *testing.T) {
	db := testutil.SetupTestDB(t)
	routeIDs := setupRunRoutesFixture(t, db, 6)
	svc := NewRecommendationService(db, nil, nil)
	now := time.Now()
	future := now.AddDate(0, 0, 7)
	past := now.Add(-1 * time.Hour)

	candAccepted, candExpired, candSnoozed, candExcluded, candLegacy, candActive := uint(11), uint(12), uint(13), uint(14), uint(15), uint(16)
	mk := func(routeID uint, cid uint, status string, expires *time.Time, reason string, dismissed *time.Time) models.FeedRecommendation {
		rec := models.FeedRecommendation{
			RouteID: routeID, CandidateID: &cid, Source: RecommendationSourceQA, Score: 1,
			LLMReason: reason, Status: status, RecommendationHash: fmt.Sprintf("%s-%d", status, cid),
			ExpiresAt: expires, LastSelectedAt: &now, DismissedAt: dismissed,
		}
		require.NoError(t, db.Create(&rec).Error)
		return rec
	}
	mk(routeIDs[0], candAccepted, "accepted", nil, "已订阅", nil)
	mk(routeIDs[1], candExpired, "pending", &past, "到期卡", nil)
	mk(routeIDs[2], candSnoozed, "pending", &future, "冷却卡", nil)
	mk(routeIDs[3], candExcluded, "pending", nil, "排除卡", nil)
	mk(routeIDs[4], candLegacy, "legacy", &past, "迁移行", nil)
	mk(routeIDs[5], candActive, "pending", &future, "活跃卡", nil)
	legacyDismissed := mk(routeIDs[4], candLegacy, "dismissed", &past, "旧拒绝", &past)

	require.NoError(t, db.Create(&models.CandidatePreference{CandidateID: candSnoozed, SnoozedUntil: &future}).Error)
	require.NoError(t, db.Create(&models.CandidatePreference{CandidateID: candExcluded, ExcludedAt: &now}).Error)

	history, err := svc.GetRecommendationHistory(context.Background())
	require.NoError(t, err)

	byStatus := map[string]int{}
	for _, h := range history {
		byStatus[h.Status]++
	}
	require.Equal(t, 1, byStatus[HistoryStatusAccepted])
	require.Equal(t, 3, byStatus[HistoryStatusExpired], "到期 pending + legacy + 旧 dismissed 三条")
	require.Equal(t, 1, byStatus[HistoryStatusSnoozed])
	require.Equal(t, 1, byStatus[HistoryStatusExcluded])
	require.Len(t, history, 6, "活跃未到期 pending 不进历史；同候选冷却/排除去重只留一条")

	for _, h := range history {
		switch h.Status {
		case HistoryStatusSnoozed:
			require.NotNil(t, h.SnoozedUntil, "snoozed 携带冷却到期时间")
		case HistoryStatusExpired:
			require.Nil(t, h.SnoozedUntil, "自动过期不得出现「不感兴趣」语义字段")
		}
		require.NotEmpty(t, h.Name, "历史条目带展示名（路由名）")
	}

	// 旧 dismissed / legacy 行只读：restore 不复活（仍是历史 expired，不产生新 pending）。
	for _, rec := range []models.FeedRecommendation{recForRoute(t, db, routeIDs[4], "legacy"), legacyDismissed} {
		require.NoError(t, svc.RestoreRecommendation(context.Background(), rec.ID))
	}
	history, err = svc.GetRecommendationHistory(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, countHistoryStatus(history, HistoryStatusExpired), "restore 不把旧历史行复活为可推荐")
	require.NoError(t, db.First(&legacyDismissed, legacyDismissed.ID).Error)
	require.Equal(t, "dismissed", legacyDismissed.Status, "旧 dismissed 行状态只读")
}

// TestLifecycle_HistoryRestoreHidesSnoozedEntry：恢复后该候选回到活跃 pending →
// 历史区不再显示（未被拒绝，只是恢复资格）。
func TestLifecycle_HistoryRestoreHidesSnoozedEntry(t *testing.T) {
	db := testutil.SetupTestDB(t)
	r1, _, _ := setupRecFixture(t, db)
	router, _ := newSelectAllMockRouter(t, db)
	svc := NewRecommendationService(db, router, nil)
	_, err := svc.RefreshRecommendations(context.Background())
	require.NoError(t, err)
	rec := recForRoute(t, db, r1, "pending")

	_, err = svc.SnoozeRecommendation(context.Background(), rec.ID)
	require.NoError(t, err)
	history, err := svc.GetRecommendationHistory(context.Background())
	require.NoError(t, err)
	require.NotNil(t, findHistory(history, rec.ID))

	require.NoError(t, svc.RestoreRecommendation(context.Background(), rec.ID))
	history, err = svc.GetRecommendationHistory(context.Background())
	require.NoError(t, err)
	require.Nil(t, findHistory(history, rec.ID), "恢复资格后回到活跃列表，不再是历史条目")
}

// ── 测试辅助 ──

func hasRouteCard(cards []RecommendationCard, routeID uint) bool {
	for _, c := range cards {
		if c.RouteID == routeID {
			return true
		}
	}
	return false
}

func findHistory(entries []RecommendationHistoryEntry, id uint) *RecommendationHistoryEntry {
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i]
		}
	}
	return nil
}

// runHasCandidate 断言指定 run 的项快照是否包含该候选（跨 source 隔离断言）。
func runHasCandidate(t *testing.T, db *gorm.DB, runID, candidateID uint) bool {
	t.Helper()
	var cnt int64
	require.NoError(t, db.Model(&models.DiscoveryRunItem{}).
		Where("run_id = ? AND candidate_id = ?", runID, candidateID).Count(&cnt).Error)
	return cnt > 0
}

func countHistoryStatus(entries []RecommendationHistoryEntry, status string) int {
	n := 0
	for _, e := range entries {
		if e.Status == status {
			n++
		}
	}
	return n
}
