package database

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"syntopica-backend/internal/models"
)

// Migrations for the lane-first-recall change
// (openspec/changes/lane-first-recall). One additive step:
//
//	20261126_0001  ai_settings 播种 lane_recall_top_k / lane_recall_max_distance（tasks 1.1）
//
// 纯数据播种：不建表、不改列、不回填。回滚 = 删这两行 ai_settings（代码侧
// git revert 后参数失效，读取端缺失即回退默认值——所以回滚不需要数据修复）。
//
// 为什么进 ai_settings 而不是写死常量：阈值是**标签级**余弦距离量纲，换
// embedding 模型后必须重标（design Risks）。与 `persistent_topic_lane_l1_threshold`
// 同模式——在线可调、无需重启、无需发布。
//
// 幂等：`ON CONFLICT (key) DO NOTHING`。既有的 check-existing-then-create 种子
// 在并发首启时可能撞唯一约束（两个实例同时 First 到空、同时 Create），
// ON CONFLICT 把这一窗口也关掉；语义与「不覆盖用户已改值」一致。
func laneRecallConfigMigration() Migration {
	return Migration{
		Version:     "20261126_0001",
		Description: "lane-first-recall: seed ai_settings lane_recall_top_k (8) and lane_recall_max_distance (0.16), idempotent and non-overriding",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "ai_settings") {
				return nil // ai_settings not registered on this deployment
			}
			// 默认值来自 12 版块 / 61 泳道 / 122 lane-day / 307 对人工样本的标定
			// （design D2）：K=8 是召回量拐点（8→12 只多 22%，8→6 掉 15%），
			// τ=0.16 精度约 90%、净新增约 62 对/天/12 版块（≈5 标签/版块/天）；
			// τ=0.20 会把 GTA6 / 仁王3 / 米哈游一并放进来。
			seeds := []struct {
				Key, Value, Description string
			}{
				{"lane_recall_top_k", "8", "泳道优先召回：每条 active 泳道当天在 event 标签池里按语义距离召回的最多标签数（缺失/非法回退 8）"},
				{"lane_recall_max_distance", "0.16", "泳道优先召回：标签到泳道锚点的余弦距离上限，超过则不收（缺失/非法回退 0.16）"},
			}
			for _, seed := range seeds {
				row := models.AISettings{Key: seed.Key, Value: seed.Value, Description: seed.Description}
				if err := db.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "key"}},
					DoNothing: true,
				}).Create(&row).Error; err != nil {
					return fmt.Errorf("seed ai_settings key %s: %w", seed.Key, err)
				}
			}
			return nil
		},
	}
}
