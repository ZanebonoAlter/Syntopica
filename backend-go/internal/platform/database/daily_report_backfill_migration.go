package database

import (
	"fmt"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// dailyReportBackfillBudgetMinutesKey is the ai_settings key driving the
// whole-round budget of one gap-backfill run (minutes, default 60). Seeded by
// the 20261125_0001 migration so the budget is tunable online.
const dailyReportBackfillBudgetMinutesKey = "daily_report_backfill_budget_minutes"

// dailyReportBackfillRunsMigration implements 20261125_0001
// (add-daily-report-gap-backfill design D4 / tasks 1.1):
//
//	daily_report_backfill_runs —— 一次「日报缺档补填」运行的留档。
//
// 为什么独立成表而不是塞进 scheduler_tasks.last_execution_result：
// ① 手动补填不走调度任务，塞进去的留档会丢；
// ② 调度任务结果结构随调度器变动，补填留档需要稳定的逐日明细 + 终态四态；
// ③ 缺口查询要能读「最近一次运行摘要」，这需要一条按 started_at 排序的独立行。
//
// schema 由本版本化迁移创建，模型**不注册 AutoMigrate**（SQL 是 schema 唯一
// 权威，对齐页边注三表的既有做法）——逐日明细是 jsonb 数组契约、status 是受约束
// 的四态枚举，这些不该由 AutoMigrate 猜。
//
// 表结构：
//
//	id             bigserial 主键
//	run_id         varchar(64) 一次运行的唯一标识（也是 WS 终态消息里的 run_id）
//	trigger_scope  varchar(20) 触发来源：manual（手动端点）/ scheduled（调度器）
//	window_start   date        补填窗口下界（含）
//	window_end     date        补填窗口上界（含）
//	status         varchar(24) 终态：success / partial / failed / budget_exhausted
//	               （运行中为 running，收尾时改写）
//	daily_details  jsonb       逐日明细数组：[{date,filled,failed,remaining}]
//	filled_count   int         补出篇数
//	failed_count   int         失败格数
//	remaining_count int        未完成缺口数（预算耗尽时 > 0）
//	started_at     timestamptz 开始时间
//	finished_at    timestamptz 结束时间（运行中为 NULL）
//	created_at     timestamptz 建行时间
//
// 幂等：CREATE TABLE/INDEX IF NOT EXISTS + CHECK 约束按名 DROP-then-ADD。
// 表缺席（日报域模型未注册的最小部署）时整段跳过，与其它可选表迁移同口径。
func dailyReportBackfillRunsMigration() Migration {
	return Migration{
		Version:     "20261125_0001",
		Description: "add-daily-report-gap-backfill: create daily_report_backfill_runs (gap-backfill run ledger: window bounds, per-day jsonb detail, four-state terminal status, filled/failed/remaining counters, started/finished timestamps).",
		Up: func(db *gorm.DB) error {
			// 可选表守卫：日报域模型未注册（最小部署）时跳过，不炸启动。
			if !tableExists(db, "board_daily_reports") {
				return nil
			}
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS daily_report_backfill_runs (
					id              bigserial PRIMARY KEY,
					run_id          varchar(64)  NOT NULL,
					trigger_scope   varchar(20)  NOT NULL DEFAULT 'manual',
					window_start    date         NOT NULL,
					window_end      date         NOT NULL,
					status          varchar(24)  NOT NULL DEFAULT 'running',
					daily_details   jsonb        NOT NULL DEFAULT '[]'::jsonb,
					filled_count    int          NOT NULL DEFAULT 0,
					failed_count    int          NOT NULL DEFAULT 0,
					remaining_count int          NOT NULL DEFAULT 0,
					started_at      timestamptz  NOT NULL DEFAULT now(),
					finished_at     timestamptz  NULL,
					created_at      timestamptz  NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create daily_report_backfill_runs: %w", err)
			}
			if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_daily_report_backfill_runs_run_id
				ON daily_report_backfill_runs (run_id)`).Error; err != nil {
				return fmt.Errorf("create uq_daily_report_backfill_runs_run_id: %w", err)
			}
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_daily_report_backfill_runs_started
				ON daily_report_backfill_runs (started_at DESC)`).Error; err != nil {
				return fmt.Errorf("create idx_daily_report_backfill_runs_started: %w", err)
			}
			// 终态四态 + 运行中态由 CHECK 约束固化（幂等：先 DROP 再 ADD）。
			statements := []string{
				`ALTER TABLE daily_report_backfill_runs
					DROP CONSTRAINT IF EXISTS ck_daily_report_backfill_runs_status`,
				`ALTER TABLE daily_report_backfill_runs
					ADD CONSTRAINT ck_daily_report_backfill_runs_status
					CHECK (status IN ('running','success','partial','failed','budget_exhausted'))`,
				`ALTER TABLE daily_report_backfill_runs
					DROP CONSTRAINT IF EXISTS ck_daily_report_backfill_runs_trigger_scope`,
				`ALTER TABLE daily_report_backfill_runs
					ADD CONSTRAINT ck_daily_report_backfill_runs_trigger_scope
					CHECK (trigger_scope IN ('manual','scheduled'))`,
				`ALTER TABLE daily_report_backfill_runs
					DROP CONSTRAINT IF EXISTS ck_daily_report_backfill_runs_window`,
				`ALTER TABLE daily_report_backfill_runs
					ADD CONSTRAINT ck_daily_report_backfill_runs_window
					CHECK (window_end >= window_start)`,
			}
			for _, stmt := range statements {
				if err := withLockTimeout(db, "5s", func(tx *gorm.DB) error {
					if err := tx.Exec(stmt).Error; err != nil {
						return fmt.Errorf("daily_report_backfill_runs constraint: %w", err)
					}
					return nil
				}); err != nil {
					return err
				}
			}

			// 播种整轮预算的 ai_settings 默认值（60 分钟）——用户可在设置里在线
			// 调整整轮补填预算（键：daily_report_backfill_budget_minutes）。
			// 已存在则跳过（幂等）。
			if tableExists(db, "ai_settings") {
				var existing models.AISettings
				if err := db.Where("key = ?", dailyReportBackfillBudgetMinutesKey).First(&existing).Error; err != nil {
					if err := db.Create(&models.AISettings{
						Key:         dailyReportBackfillBudgetMinutesKey,
						Value:       "60",
						Description: "日报缺档补填整轮预算（分钟）",
					}).Error; err != nil {
						return fmt.Errorf("seed ai_settings.%s: %w", dailyReportBackfillBudgetMinutesKey, err)
					}
				}
			}
			return nil
		},
	}
}
