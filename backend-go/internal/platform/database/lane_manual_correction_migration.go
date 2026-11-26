package database

import (
	"fmt"

	"gorm.io/gorm"
)

// Migrations for the lane-manual-correction-loop change (openspec/changes/
// lane-manual-correction-loop). Four additive schema steps, each idempotent:
//
//	20261113_0001  board_persistent_topics.centroid_pending_recompute  （tasks 1.1）
//	20261113_0002  topic_anchor_overrides 表                             （tasks 1.2）
//	20261113_0003  daily_report_sections 归属变更留痕三列                 （tasks 1.3）
//	20261113_0004  section_assignment_overrides 表（归属留痕 append-only） （review R-2）
//
// 四步全是加法（新列 / 新表），无 DROP / TRUNCATE / 既有列改类型；回滚不需要
// 数据备份（新列可空或带默认值，新表留库中无害）。
//
// 设计依据：change design.md「Migration Plan」第 1 步 + Decisions D2/D7；第 4 步
// 是 review R-2 的补齐（留痕从「覆盖写最近一次」升级为 append-only 完整历史）。

// laneManualCorrectionPendingMigration implements 20261113_0001
// (lane-manual-correction-loop task 1.1)：给 board_persistent_topics 加
// centroid_pending_recompute 标记列。
//
// 该列是「人工修正后强制重算质心」的持久标记：人工改首义 / 剔除节点 / 加回
// 节点把相关话题置位，质心刷新轮的处理范围 =「当日 touched 话题 ∪ 本列为
// true 的话题」（design D2）。没有这列，一条「当天没有新内容命中」的泳道会
// 永远等不到重算——人工改的事实落不下去。
//
// 历史行一律 false：迁移不做任何回填、不触发额外重算（红线：加法迁移、
// 不改既有数据语义）。
//
// 幂等：ADD COLUMN IF NOT EXISTS；AutoMigrate 先于版本化迁移运行（db.go 的
// RunAutoMigrate → RunMigrations），列可能已由 AutoMigrate 建出——补齐
// DEFAULT 与 NOT NULL 后再收口，保证两种启动顺序下约束一致。
func laneManualCorrectionPendingMigration() Migration {
	return Migration{
		Version:     "20261113_0001",
		Description: "lane-manual-correction-loop: add board_persistent_topics.centroid_pending_recompute (boolean NOT NULL DEFAULT false) — 人工修正后的强制重算标记；历史行默认 false，不触发重算",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_persistent_topics") {
				return nil // daily-report tables not registered on this deployment
			}
			if err := db.Exec(`ALTER TABLE board_persistent_topics
				ADD COLUMN IF NOT EXISTS centroid_pending_recompute boolean NOT NULL DEFAULT false`).Error; err != nil {
				return fmt.Errorf("add board_persistent_topics.centroid_pending_recompute: %w", err)
			}
			// AutoMigrate 可能已把该列建成可空形态——先补默认值再收口 NOT NULL
			// （ensureNotNullDefault 只在列可空时动手，幂等）。
			if err := ensureNotNullDefault(db, "board_persistent_topics", "centroid_pending_recompute", "false"); err != nil {
				return err
			}
			if err := db.Exec(`ALTER TABLE board_persistent_topics
				ALTER COLUMN centroid_pending_recompute SET DEFAULT false`).Error; err != nil {
				return fmt.Errorf("set board_persistent_topics.centroid_pending_recompute default: %w", err)
			}
			return nil
		},
	}
}

// laneManualCorrectionOverridesTableMigration implements 20261113_0002
// (lane-manual-correction-loop task 1.2)：建 topic_anchor_overrides 追加表
// （首义改写留痕，design D7）。
//
// 表结构与 design.md「Migration Plan」第 1 步一致：id / topic_id /
// old_embedding / new_embedding / source / input_summary / operator /
// created_at，topic_id 建索引。
//
// 两个 vector 列写成无维度限定的 vector（维度由运行时 embedding 模型决定，
// 与 board_persistent_topics.embedding / centroid 的既有口径一致）。
//
// topic_id 刻意不建外键：留痕是只追加的审计流水，硬删话题时不应连带抹掉
// 「这条泳道的定义面什么时候被谁改过」的历史（spec：留痕 SHALL 只追加，
// SHALL NOT 被删除）。没有外键也就没有级联删除的风险。
//
// 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。
func laneManualCorrectionOverridesTableMigration() Migration {
	return Migration{
		Version:     "20261113_0002",
		Description: "lane-manual-correction-loop: create topic_anchor_overrides (append-only 首义改写留痕表) + topic_id index; topic_id 不设外键，留痕不随话题删除而消失",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "board_persistent_topics") {
				return nil // daily-report tables not registered on this deployment
			}
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS topic_anchor_overrides (
					id            bigserial PRIMARY KEY,
					topic_id      bigint NOT NULL,
					old_embedding vector,
					new_embedding vector NOT NULL,
					source        varchar(32) NOT NULL,
					input_summary text NOT NULL DEFAULT '',
					operator      varchar(64) NOT NULL DEFAULT '',
					created_at    timestamptz NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create topic_anchor_overrides: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_topic_anchor_overrides_topic
				ON topic_anchor_overrides (topic_id)`).Error; err != nil {
				return fmt.Errorf("create idx_topic_anchor_overrides_topic: %w", err)
			}
			return nil
		},
	}
}

// laneManualCorrectionAssignmentOverridesTableMigration implements
// 20261113_0004（review R-2）：建 section_assignment_overrides 追加表
// （泳道归属变更留痕）。
//
// 表结构：id / section_id / topic_id_from / topic_id_to / action /
// operator / created_at，section_id 建索引。
//
// 语义：daily_report_sections 上的留痕三列是「最近一次人工归属变更」的快照，
// 会被下一次人工变更覆盖写；本表是 append-only 的完整历史，「移除 → 加回」
// 两步各自留一行，谁在什么时候把哪条内容挪进 / 挪出哪条泳道都能答。
//
// topic_id_from / topic_id_to 语义：
//   - detach：topic_id_from = 移除前所属话题，topic_id_to = NULL；
//   - attach：topic_id_from = NULL（此前无归属），topic_id_to = 目标话题。
//
// section_id 刻意不建外键：留痕是只追加的审计流水，硬删 section 时不应连带
// 抹掉「这条内容什么时候被谁挪走过」的历史（同 topic_anchor_overrides 的
// 口径：审计不随硬删消失）。
//
// 幂等：CREATE TABLE IF NOT EXISTS + CREATE INDEX IF NOT EXISTS。
func laneManualCorrectionAssignmentOverridesTableMigration() Migration {
	return Migration{
		Version:     "20261113_0004",
		Description: "lane-manual-correction-loop: create section_assignment_overrides (append-only 泳道归属变更留痕表) + section_id index; 不设外键，留痕不随 section 硬删而消失",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "daily_report_sections") {
				return nil // daily-report tables not registered on this deployment
			}
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS section_assignment_overrides (
					id            bigserial PRIMARY KEY,
					section_id    bigint NOT NULL,
					topic_id_from bigint NULL,
					topic_id_to   bigint NULL,
					action        varchar(16) NOT NULL,
					operator      varchar(64) NOT NULL DEFAULT '',
					created_at    timestamptz NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create section_assignment_overrides: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_section_assignment_overrides_section
				ON section_assignment_overrides (section_id)`).Error; err != nil {
				return fmt.Errorf("create idx_section_assignment_overrides_section: %w", err)
			}
			return nil
		},
	}
}

// laneManualCorrectionAssignmentTraceMigration implements 20261113_0003
// (lane-manual-correction-loop task 1.3)：给 daily_report_sections 加归属
// 变更留痕三列（design D7）。
//
//	manual_assignment_at             timestamptz NULL  变更时间
//	manual_assignment_from_topic_id  bigint      NULL  变更前的话题 id
//	manual_assignment_action         text        NULL  动作（detach / attach）
//
// 三列全部可空、无默认值：历史行保持 NULL（=「没有人工归属变更」），无需
// 回填、不改既有行语义。
//
// 幂等：ADD COLUMN IF NOT EXISTS × 3。
func laneManualCorrectionAssignmentTraceMigration() Migration {
	return Migration{
		Version:     "20261113_0003",
		Description: "lane-manual-correction-loop: add daily_report_sections assignment-change trace columns (manual_assignment_at timestamptz / manual_assignment_from_topic_id bigint / manual_assignment_action text) — 全部可空，历史行 NULL",
		Up: func(db *gorm.DB) error {
			if !tableExists(db, "daily_report_sections") {
				return nil // daily-report tables not registered on this deployment
			}
			stmts := []string{
				`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS manual_assignment_at timestamptz`,
				`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS manual_assignment_from_topic_id bigint`,
				`ALTER TABLE daily_report_sections ADD COLUMN IF NOT EXISTS manual_assignment_action text`,
			}
			for _, stmt := range stmts {
				if err := db.Exec(stmt).Error; err != nil {
					return fmt.Errorf("add daily_report_sections assignment trace column: %w", err)
				}
			}
			return nil
		},
	}
}
