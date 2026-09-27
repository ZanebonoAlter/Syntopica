package aisettings

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// ── night-window-alignment D5：daily_report_deadline ──
//
// 两把新 key 走既有 AISettings key/value 通道：缺失/非法回退默认 + warn（不做
// 前端配置界面），deadline 早于 daily_report_time 回退默认 23:30。

// seedRawSetting writes a raw ai_settings row, bypassing Save validation so
// tests can seed invalid values. Upsert: the golden schema seeds
// daily_report_time already.
func seedRawSetting(t *testing.T, key, value string) {
	t.Helper()
	var settings models.AISettings
	err := database.DB.Where("key = ?", key).First(&settings).Error
	if err == nil {
		settings.Value = value
		require.NoError(t, database.DB.Save(&settings).Error)
		return
	}
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, database.DB.Create(&models.AISettings{Key: key, Value: value}).Error)
}

// deleteSetting removes one key so tests can assert true-missing behavior.
func deleteSetting(t *testing.T, key string) {
	t.Helper()
	require.NoError(t, database.DB.Where("key = ?", key).Delete(&models.AISettings{}).Error)
}

// TestLoadDailyReportWindowConfig_MissingKeys（D9 默认时刻）：双 key 缺失 →
// 默认墙钟 21:00 + 默认兜底 23:30。
func TestLoadDailyReportWindowConfig_MissingKeys(t *testing.T) {
	testutil.SetupTestDB(t)
	// golden schema 种子含 daily_report_time=21:00；真「缺失」场景先删双 key。
	deleteSetting(t, dailyReportTimeKey)
	deleteSetting(t, dailyReportDeadlineKey)

	timeStr, deadlineStr, err := LoadDailyReportWindowConfig()
	require.NoError(t, err)
	require.Equal(t, "21:00", timeStr)
	require.Equal(t, "23:30", deadlineStr)
}

// TestLoadDailyReportWindowConfig_InvalidValuesFallBack（D9 非法格式）：
// "25:99" / "abc" 各自回退默认并记警告。
func TestLoadDailyReportWindowConfig_InvalidValuesFallBack(t *testing.T) {
	testutil.SetupTestDB(t)
	seedRawSetting(t, dailyReportTimeKey, "25:99")
	seedRawSetting(t, dailyReportDeadlineKey, "abc")

	timeStr, deadlineStr, err := LoadDailyReportWindowConfig()
	require.NoError(t, err)
	require.Equal(t, "21:00", timeStr)
	require.Equal(t, "23:30", deadlineStr)
}

// TestLoadDailyReportWindowConfig_DeadlineBeforeTimeFallsBack（D8 兜底早于墙钟
// 的非法关系）：deadline 回退默认 23:30 + warn，墙钟保持 21:00。
func TestLoadDailyReportWindowConfig_DeadlineBeforeTimeFallsBack(t *testing.T) {
	testutil.SetupTestDB(t)
	seedRawSetting(t, dailyReportTimeKey, "21:00")
	seedRawSetting(t, dailyReportDeadlineKey, "20:00")

	timeStr, deadlineStr, err := LoadDailyReportWindowConfig()
	require.NoError(t, err)
	require.Equal(t, "21:00", timeStr, "wall clock must stay configured")
	require.Equal(t, "23:30", deadlineStr, "deadline earlier than wall clock falls back to default")
}

// TestLoadDailyReportWindowConfig_NormalAndUpdateEffective（正常配置 + 更新生效）：
// 合法值原样返回；Save 更新后下一次 Load 即生效（墙钟循环每个周期现读）。
func TestLoadDailyReportWindowConfig_NormalAndUpdateEffective(t *testing.T) {
	testutil.SetupTestDB(t)

	require.NoError(t, SaveDailyReportTimeConfig("20:30"))
	require.NoError(t, SaveDailyReportDeadlineConfig("23:00"))

	timeStr, deadlineStr, err := LoadDailyReportWindowConfig()
	require.NoError(t, err)
	require.Equal(t, "20:30", timeStr)
	require.Equal(t, "23:00", deadlineStr)

	// 更新生效：deadline 改成 22:15 后即读即得。
	require.NoError(t, SaveDailyReportDeadlineConfig("22:15"))
	timeStr, deadlineStr, err = LoadDailyReportWindowConfig()
	require.NoError(t, err)
	require.Equal(t, "20:30", timeStr)
	require.Equal(t, "22:15", deadlineStr)
}

// TestSaveDailyReportDeadlineConfig_RejectsInvalid：Save 侧拒绝非法格式（与
// daily_report_time 同口径）。
func TestSaveDailyReportDeadlineConfig_RejectsInvalid(t *testing.T) {
	testutil.SetupTestDB(t)

	require.Error(t, SaveDailyReportDeadlineConfig("24:00"))
	require.Error(t, SaveDailyReportDeadlineConfig("abc"))
	require.Error(t, SaveDailyReportDeadlineConfig("9:00"))
}
