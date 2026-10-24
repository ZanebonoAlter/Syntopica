package board

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/models"
)

// 用例来源：fix-digest-tag-leak tasks 1.1 / test-cases 故事 A-8 ——
// DirectionRelativeMargin（ai_settings key
// semantic_board_match_direction_relative_margin）三配置用例 + 白盒变体
// （非数字回退默认、负数与 0 同义关闭）。LoadConfig 走包级缓存，每个子测试
// 先 InvalidateBoardCache 隔离。
func TestLoadConfig(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	svc := NewSemanticBoardMatchingService(db)
	ctx := context.Background()
	const marginKey = "semantic_board_match_direction_relative_margin"

	t.Run("no margin row uses default 0.15", func(t *testing.T) {
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.15, config.DirectionRelativeMargin, 1e-9)
	})

	t.Run("custom margin 0.18", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AISettings{Key: marginKey, Value: "0.18"}).Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.18, config.DirectionRelativeMargin, 1e-9)
	})

	t.Run("margin zero disables relative gate", func(t *testing.T) {
		require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", marginKey).Update("value", "0").Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.0, config.DirectionRelativeMargin, 1e-9)
	})

	t.Run("non-numeric falls back to default", func(t *testing.T) {
		require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", marginKey).Update("value", "abc").Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.15, config.DirectionRelativeMargin, 1e-9)
	})

	t.Run("negative converges to zero (gate off)", func(t *testing.T) {
		require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", marginKey).Update("value", "-0.2").Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.0, config.DirectionRelativeMargin, 1e-9)
	})
}

// TestConfigDirectionFloor 覆盖 fix-board-ghost-false-positive task 2.1：
// DirectionFloor（ai_settings key semantic_board_match_direction_floor）三用例
// ——默认 0（豁免无下限）/ 自定义值生效 / 越界（>1 或负数或非数字）拒绝回退默认。
func TestConfigDirectionFloor(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	svc := NewSemanticBoardMatchingService(db)
	ctx := context.Background()
	const floorKey = "semantic_board_match_direction_floor"

	t.Run("no floor row uses default 0 (off)", func(t *testing.T) {
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.0, config.DirectionFloor, 1e-9)
	})

	t.Run("custom floor 0.55", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AISettings{Key: floorKey, Value: "0.55"}).Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.55, config.DirectionFloor, 1e-9)
	})

	t.Run("out-of-range value rejected falls back to default 0", func(t *testing.T) {
		for _, invalid := range []string{"1.5", "-0.2", "abc"} {
			require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", floorKey).Update("value", invalid).Error)
			InvalidateBoardCache()
			config := svc.LoadConfig(ctx)
			require.InDelta(t, 0.0, config.DirectionFloor, 1e-9, "value %q should fall back to 0", invalid)
		}
	})
}

// TestConfigDirectionGapExemptMargin 覆盖 fix-board-direction-gap-exempt
// task 2.1：DirectionGapExemptMargin（ai_settings key
// semantic_board_match_direction_gap_exempt_margin）三用例——默认 0.10 /
// 自定义值生效 / 非法（非数字或>1）拒绝回退默认；负值归 0（豁免关闭，
// 回退并列最优口径）。用例来源 spec「DirectionGapExemptMargin
// configurable」三个 Scenario + parse 语义白盒变体。
func TestConfigDirectionGapExemptMargin(t *testing.T) {
	db := setupSemanticBoardMatchingTestDB(t)
	svc := NewSemanticBoardMatchingService(db)
	ctx := context.Background()
	const marginKey = "semantic_board_match_direction_gap_exempt_margin"

	t.Run("no margin row uses default 0.10", func(t *testing.T) {
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.10, config.DirectionGapExemptMargin, 1e-9)
	})

	t.Run("custom margin 0.15", func(t *testing.T) {
		require.NoError(t, db.Create(&models.AISettings{Key: marginKey, Value: "0.15"}).Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.15, config.DirectionGapExemptMargin, 1e-9)
	})

	t.Run("upper bound 1.0 accepted", func(t *testing.T) {
		// review L5：parse 语义 [0,1] 闭区间，1.0 为合法上界不是越界。
		require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", marginKey).Update("value", "1.0").Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 1.0, config.DirectionGapExemptMargin, 1e-9)
	})

	t.Run("invalid value rejected falls back to default 0.10", func(t *testing.T) {
		for _, invalid := range []string{"1.5", "abc"} {
			require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", marginKey).Update("value", invalid).Error)
			InvalidateBoardCache()
			config := svc.LoadConfig(ctx)
			require.InDelta(t, 0.10, config.DirectionGapExemptMargin, 1e-9, "value %q should fall back to 0.10", invalid)
		}
	})

	t.Run("negative converges to zero (exemption off)", func(t *testing.T) {
		require.NoError(t, db.Model(&models.AISettings{}).Where("key = ?", marginKey).Update("value", "-0.2").Error)
		InvalidateBoardCache()
		config := svc.LoadConfig(ctx)
		require.InDelta(t, 0.0, config.DirectionGapExemptMargin, 1e-9)
	})
}
