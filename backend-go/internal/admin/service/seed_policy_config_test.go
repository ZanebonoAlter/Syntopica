package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
)

// ── seed policy 配置持久化测试（ai_settings JSON blob 模式，PG testcontainer）──

// TestSeedPolicyConfigMissingKeyFallsBackDefault：键缺失 → 全默认且合法。
func TestSeedPolicyConfigMissingKeyFallsBackDefault(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cfg, err := loadSeedPolicyConfig(db)
	require.NoError(t, err)
	require.Equal(t, DefaultSeedPolicyConfig(), cfg)
}

// TestSeedPolicyConfigPartialOverride：只覆盖出现字段，其余回默认。
func TestSeedPolicyConfigPartialOverride(t *testing.T) {
	db := testutil.SetupTestDB(t)
	require.NoError(t, db.Create(&models.AISettings{
		Key: seedPolicyConfigKey, Value: `{"interest_max_entries": 8, "seed_match_similarity": 0.85}`,
	}).Error)
	cfg, err := loadSeedPolicyConfig(db)
	require.NoError(t, err)
	def := DefaultSeedPolicyConfig()
	require.Equal(t, 8, cfg.MaxEntries, "出现的字段生效")
	require.InDelta(t, 0.85, cfg.MatchSimilarity, 1e-9)
	require.Equal(t, def.WindowDays, cfg.WindowDays, "未出现字段回默认")
	require.Equal(t, def.SeedBudget, cfg.SeedBudget)
	require.Equal(t, def.MatchMargin, cfg.MatchMargin)
}

// TestSeedPolicyConfigInvalidRejected：非法值（超域/类型错/坏 JSON）→ 带字段名的错误。
func TestSeedPolicyConfigInvalidRejected(t *testing.T) {
	db := testutil.SetupTestDB(t)
	bad := []string{
		`{"interest_window_days": 400}`,
		`{"interest_half_life_days": 7, "interest_window_days": 5}`, // half_life > window
		`{"seed_candidate_budget": 99}`,
		`{"interest_max_entries": "many"}`, // 类型非法
		`{not-json`,
	}
	for _, v := range bad {
		require.NoError(t, db.Where("key = ?", seedPolicyConfigKey).Delete(&models.AISettings{}).Error)
		require.NoError(t, db.Create(&models.AISettings{Key: seedPolicyConfigKey, Value: v}).Error)
		_, err := loadSeedPolicyConfig(db)
		require.Error(t, err, "value=%s 必须拒绝", v)
	}
}

// TestSeedPolicyConfigSaveRoundtrip：合法配置校验后落库、读回一致；非法配置拒绝保存。
func TestSeedPolicyConfigSaveRoundtrip(t *testing.T) {
	db := testutil.SetupTestDB(t)
	custom := DefaultSeedPolicyConfig()
	custom.WindowDays = 60
	custom.MaxEntries = 10
	custom.HalfLifeDays = 14
	custom.MaturityArticles = 50
	custom.SeedBudget = 6
	custom.MatchSimilarity = 0.8
	custom.MatchMargin = 0.05

	require.NoError(t, saveSeedPolicyConfig(db, custom))
	got, err := loadSeedPolicyConfig(db)
	require.NoError(t, err)
	require.Equal(t, custom, got)

	invalid := custom
	invalid.WindowDays = 0
	require.Error(t, saveSeedPolicyConfig(db, invalid), "非法配置不得落库")
	// 保存失败不得污染已存值。
	got, err = loadSeedPolicyConfig(db)
	require.NoError(t, err)
	require.Equal(t, custom, got)
	_ = context.Background()
}
