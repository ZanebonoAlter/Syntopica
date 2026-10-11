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
