package database

import (
	"fmt"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// Migrations for the topic-lane-auxiliary-label-constraint change
// (openspec/changes/topic-lane-auxiliary-label-constraint). One additive step:
//
//	20261124_0001  board_persistent_topic_constraints 表 + margin/max_distance 全局默认（tasks 1.1）
//
// 纯加法（新表），不动任何既有表结构（design D9：`board_persistent_topics` /
// `daily_report_sections` 列结构保持不变）；回滚 = DROP TABLE，代码侧 git revert。
//
// 为什么约束要独立成表而不是加列：约束是集合（一条话题可绑多条），塞不进单行；
// 且存量话题无约束行时闸门天然直通（零额外 LLM 调用），加列会带来无谓的迁移面。

// topicLaneConstraintMigration implements 20261124_0001
// (topic-lane-auxiliary-label-constraint task 1.1 / design D1/D4/D9).
//
// 表结构：约束以**向量**为载体（`term` 用户短词 + `term_embedding` 锚向量 +
// `embedding_model` 向量空间标识），不存 tag id 集合——`article_topic_tags` 边按
// `tag_edge_retention_days`（默认 7 天）回收、孤立 tag 随之清理，任何基于 tag id
// 的约束会在 7 天后自行失效，向量则与 GC 无关（design D1）。
//
// `term_embedding` 写成**无维度限定的 vector**：维度由运行时 embedding 模型的
// 实际输出决定，不属于 schema 契约（与 `unify-vector-column-dimension` 的终态
// 一致；带维度限定的列会在换模型时逼出全表 ALTER）。
//
// `margin` / `max_distance` **可空**：空 = 取 `ai_settings` 的全局默认
// （`persistent_topic_constraint_margin_default` / `..._max_distance_default`），
// 非空 = 该约束的显式覆写（design D4）。这样改全局默认对存量约束立即生效，
// 与既有 `persistent_topic_lane_l1_threshold` 的在线可调口径一致。
//
// 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS + 逐条
// check-existing-then-create 的 ai_settings 播种；表缺席时（域模型未注册的最小
// 部署）整段跳过。
func topicLaneConstraintMigration() Migration {
	return Migration{
		Version:     "20261124_0001",
		Description: "topic-lane-auxiliary-label-constraint: create board_persistent_topic_constraints (term + dimensionless vector anchor + embedding_model + mode required/preferred + nullable margin/max_distance overrides, CHECK/UNIQUE/FK CASCADE) and seed the margin/max_distance global defaults",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_persistent_topics") {
				return nil // daily-report tables not registered on this deployment
			}
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS board_persistent_topic_constraints (
					id             bigserial PRIMARY KEY,
					topic_id       bigint NOT NULL,
					term           varchar(100) NOT NULL,
					term_embedding vector NOT NULL,
					embedding_model varchar(100) NOT NULL,
					mode           varchar(16) NOT NULL DEFAULT 'required',
					margin         numeric NULL,
					max_distance   numeric NULL,
					created_at     timestamptz NOT NULL DEFAULT now(),
					updated_at     timestamptz NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create board_persistent_topic_constraints: %w", err)
			}

			// CHECK / 唯一约束用 DROP-then-ADD 的 DO 块保持幂等（AutoMigrate 不会建
			// 这些约束，但重复执行本迁移或手工预建表时不能炸）。
			statements := []string{
				`ALTER TABLE board_persistent_topic_constraints
					DROP CONSTRAINT IF EXISTS ck_board_persistent_topic_constraints_mode`,
				`ALTER TABLE board_persistent_topic_constraints
					ADD CONSTRAINT ck_board_persistent_topic_constraints_mode
					CHECK (mode IN ('required','preferred'))`,
				`ALTER TABLE board_persistent_topic_constraints
					DROP CONSTRAINT IF EXISTS ck_board_persistent_topic_constraints_margin`,
				`ALTER TABLE board_persistent_topic_constraints
					ADD CONSTRAINT ck_board_persistent_topic_constraints_margin
					CHECK (margin IS NULL OR (margin >= 0 AND margin <= 1))`,
				`ALTER TABLE board_persistent_topic_constraints
					DROP CONSTRAINT IF EXISTS ck_board_persistent_topic_constraints_max_distance`,
				`ALTER TABLE board_persistent_topic_constraints
					ADD CONSTRAINT ck_board_persistent_topic_constraints_max_distance
					CHECK (max_distance IS NULL OR (max_distance >= 0 AND max_distance <= 1))`,
			}
			for _, stmt := range statements {
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("constraint CHECK: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
			}

			// 同一话题内同一短词唯一（spec「同一话题内短词重复被拒」）。
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_board_persistent_topic_constraints_topic_term
					ON board_persistent_topic_constraints (topic_id, term)`).Error; err != nil {
					return fmt.Errorf("create uq_board_persistent_topic_constraints_topic_term: %w", err)
				}
				if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_board_persistent_topic_constraints_topic
					ON board_persistent_topic_constraints (topic_id)`).Error; err != nil {
					return fmt.Errorf("create idx_board_persistent_topic_constraints_topic: %w", err)
				}
				return nil
			}); err != nil {
				return err
			}

			// 话题删除级联清理约束（spec「话题删除级联清理约束」）。
			if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
				if err := tx.Exec(`DO $$ BEGIN
					IF NOT EXISTS (
						SELECT 1 FROM information_schema.table_constraints
						WHERE constraint_name = 'fk_board_persistent_topic_constraints_topic'
						  AND table_name = 'board_persistent_topic_constraints'
					) THEN
						ALTER TABLE board_persistent_topic_constraints
							ADD CONSTRAINT fk_board_persistent_topic_constraints_topic
							FOREIGN KEY (topic_id) REFERENCES board_persistent_topics(id)
							ON DELETE CASCADE;
					END IF;
				END $$`).Error; err != nil {
					return fmt.Errorf("add fk_board_persistent_topic_constraints_topic: %w", err)
				}
				return nil
			}); err != nil {
				return err
			}

			// margin / max_distance 全局默认（design D4；值来自本地空间实测：
			// margin=0.05 时正样本放行 64.4%、两类负样本误判均 0%；margin=0.10 时
			// 正样本掉到 25.2%。具体取值由效果核对环节标定，改这里只影响默认）。
			constraintDefaults := []struct {
				Key, Value, Description string
			}{
				{"persistent_topic_constraint_margin_default", "0.05", "辅助标签约束的相对偏好严格度默认值：tag 到约束锚 i 的距离须比其它锚近至少此值（约束行内 margin 为空时取本值）"},
				{"persistent_topic_constraint_max_distance_default", "0.55", "辅助标签约束的绝对距离上限默认值：超过此距离的 tag 不参与相对偏好判定（约束行内 max_distance 为空时取本值）"},
			}
			for _, d := range constraintDefaults {
				var existing models.AISettings
				if err := db.Where("key = ?", d.Key).First(&existing).Error; err == nil {
					continue
				}
				if err := db.Create(&models.AISettings{
					Key: d.Key, Value: d.Value, Description: d.Description,
				}).Error; err != nil {
					return fmt.Errorf("seed ai_settings key %s: %w", d.Key, err)
				}
			}
			return nil
		},
	}
}
