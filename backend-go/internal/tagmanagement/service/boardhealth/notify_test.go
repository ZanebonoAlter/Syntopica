package boardhealth

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// 用例来源：test-cases.md 主链路步 9/10（异常通知 / 裁决不值得与冷却静默）+
// 变体「冷却期边界：恰第 3 天」+ 白盒「评分：边界阈值」。

func TestShouldNotify(t *testing.T) {
	config := Config{AlertThreshold: 60, NotifyCooldownDays: 3}
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.Local)
	lowScore := 52.0
	highScore := 75.0
	thresholdScore := 60.0

	t.Run("异常且裁决值得则通知", func(t *testing.T) {
		require.True(t, shouldNotify(&lowScore, true, config, nil, now))
	})
	t.Run("裁决不值得则静默", func(t *testing.T) {
		require.False(t, shouldNotify(&lowScore, false, config, nil, now))
	})
	t.Run("正常分数忽略 notify", func(t *testing.T) {
		require.False(t, shouldNotify(&highScore, true, config, nil, now))
	})
	t.Run("降级轮（score NULL）不通知", func(t *testing.T) {
		require.False(t, shouldNotify(nil, true, config, nil, now))
	})
	t.Run("评分恰=阈值不通知（严格小于）", func(t *testing.T) {
		require.False(t, shouldNotify(&thresholdScore, true, config, nil, now))
	})
	t.Run("冷却期内不重复通知", func(t *testing.T) {
		yesterday := now.Add(-24 * time.Hour)
		require.False(t, shouldNotify(&lowScore, true, config, &yesterday, now))
	})
	t.Run("恰第 3 天当天可再通知（严格 < 冷却）", func(t *testing.T) {
		threeDaysAgo := now.Add(-3 * 24 * time.Hour)
		require.True(t, shouldNotify(&lowScore, true, config, &threeDaysAgo, now))
	})
	t.Run("冷却边界差一秒仍在期内", func(t *testing.T) {
		justUnder := now.Add(-3*24*time.Hour + time.Second)
		require.False(t, shouldNotify(&lowScore, true, config, &justUnder, now))
	})
}

func TestNotifyIfAlertPaths(t *testing.T) {
	db := setupTestDB(t)
	board := createBoardRow(t, db, "新能源", "active")
	config := Config{AlertThreshold: 60, NotifyCooldownDays: 3}
	lowScore := 52.0
	ctx := context.Background()

	var notified []uint
	original := notifyBoardHealthAlert
	defer func() { notifyBoardHealthAlert = original }()
	notifyBoardHealthAlert = func(boardID uint, boardLabel string, score *float64, llmComment string) {
		notified = append(notified, boardID)
	}

	// 无历史通知 + 低分 + 值得 → 落通知。
	notifyIfAlert(ctx, db, config, &board, &lowScore, true, time.Now())
	require.Equal(t, []uint{board.ID}, notified)

	// 冷却期内（真实 notification 行作锚）→ 静默。
	require.NoError(t, db.Create(&models.Notification{
		Type: "error", Title: "版块体检预警 · 新能源", Summary: "体检评分 52。",
		LinkType: "board", LinkID: strconv.FormatUint(uint64(board.ID), 10),
	}).Error)
	notifyIfAlert(ctx, db, config, &board, &lowScore, true, time.Now())
	require.Len(t, notified, 1, "cooldown suppresses the second alert")

	// 冷却锚恰过期 → 可再通知。
	old := time.Now().Add(-4 * 24 * time.Hour)
	require.NoError(t, db.Model(&models.Notification{}).
		Where("link_type = ? AND link_id = ?", "board", strconv.FormatUint(uint64(board.ID), 10)).
		Update("created_at", old).Error)
	notifyIfAlert(ctx, db, config, &board, &lowScore, true, time.Now())
	require.Len(t, notified, 2, "expired cooldown allows a new alert")
}
