package boardhealth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// 用例来源：tasks 3.3 / design D8 —— 6 配置项均可缺省带默认值，非法值回退；
// 磁铁两项为 board-health-triage-advice D2（tasks 1.1）。
func TestLoadConfigDefaultsAndOverride(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// 缺省：无任何行 → 全默认。
	config := LoadConfig(ctx, db)
	require.Equal(t, 5, config.MinSampleSize)
	require.InDelta(t, 60.0, config.AlertThreshold, 1e-9)
	require.Equal(t, 3, config.NotifyCooldownDays)
	require.Equal(t, 20, config.MemberCandidatesTop)
	require.InDelta(t, 0.08, config.MagnetGap, 1e-9)
	require.Equal(t, 5, config.MagnetMinFailed)
	require.InDelta(t, 0.8, config.MaxActionableRatio, 1e-9)

	// 合法覆盖。
	require.NoError(t, db.Create(&models.AISettings{Key: minSampleSizeKey, Value: "8"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: alertThresholdKey, Value: "45"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: notifyCooldownKey, Value: "7"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: memberCandidatesKey, Value: "30"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: magnetGapKey, Value: "0.15"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: magnetMinFailedKey, Value: "8"}).Error)
	require.NoError(t, db.Create(&models.AISettings{Key: maxActionableRatioKey, Value: "0.6"}).Error)
	config = LoadConfig(ctx, db)
	require.Equal(t, 8, config.MinSampleSize)
	require.InDelta(t, 45.0, config.AlertThreshold, 1e-9)
	require.Equal(t, 7, config.NotifyCooldownDays)
	require.Equal(t, 30, config.MemberCandidatesTop)
	require.InDelta(t, 0.15, config.MagnetGap, 1e-9)
	require.Equal(t, 8, config.MagnetMinFailed)
	require.InDelta(t, 0.6, config.MaxActionableRatio, 1e-9)

	// 非法值回退默认（负数 / 非数字 / 越界 / 零）。
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", minSampleSizeKey).Update("value", "-1").Error)
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", alertThresholdKey).Update("value", "abc").Error)
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", notifyCooldownKey).Update("value", "0").Error)
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", memberCandidatesKey).Update("value", "-5").Error)
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", magnetGapKey).Update("value", "abc").Error)
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", magnetMinFailedKey).Update("value", "0").Error)
	require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", maxActionableRatioKey).Update("value", "1.5").Error)
	config = LoadConfig(ctx, db)
	require.Equal(t, 5, config.MinSampleSize, "negative falls back")
	require.InDelta(t, 60.0, config.AlertThreshold, 1e-9, "non-numeric falls back")
	require.Equal(t, 3, config.NotifyCooldownDays, "zero falls back")
	require.Equal(t, 20, config.MemberCandidatesTop, "negative falls back")
	require.InDelta(t, 0.08, config.MagnetGap, 1e-9, "non-numeric falls back")
	require.Equal(t, 5, config.MagnetMinFailed, "zero falls back")
	require.InDelta(t, 0.8, config.MaxActionableRatio, 1e-9, "占比阈值 >1 非法 → 回退默认")
}
