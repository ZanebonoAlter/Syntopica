package database

import (
	"fmt"

	"gorm.io/gorm"
)

// detectionJobUniqueMigration 实现 20261120_0003（add-ai-generated-article-detection
// task 3.5，与 fix-feed-refresh-churn D4 同型加固）：detection_jobs 加
// (article_id) WHERE status IN ('pending','leased') 部分唯一索引。
//
// Enqueue 同样是事务内 check-then-insert（骨架对齐 tag_jobs）：三个内容就绪节点
// （抓取完成 / 抓取失败降级 / 入库即判定）可能在同一秒落到同一篇文章上，仅靠应用层
// 先查后插关不掉窗口。tag_jobs 已有并发双插实证（178599），本表同型，一次加固。
//
// 建索引前先清存量重复：同 article_id 的多条 pending/leased 只保留最小 id，否则
// CREATE UNIQUE INDEX 会因存量数据失败、连带启动失败。删除前先把较新行的语义合并
// 进保留行（审查 R-5）：priority 取 MAX、reason 取最新非空行（同一 updated_at 时取
// 较大 id）；detection_jobs 无 force_retag 列，该项跳过。
//
// 生产库已执行过本迁移（版本 20261120_0003 已记录），因此上述合并语义只对全新环境
// （新部署 / 测试库重建）生效；已跑过旧版迁移的库不会回补。
//
// 幂等：UPDATE 与 DELETE 收敛后重复执行无行可改/可删；CREATE INDEX IF NOT EXISTS
// 重复执行 no-op。表由 20261120_0001 / AutoMigrate 建出——表不存在（最小部署未注册
// 模型）时跳过。非 postgres 方言（sqlite 模型单测）直接跳过，与同批迁移一致。
func detectionJobUniqueMigration() Migration {
	return Migration{
		Version:     "20261120_0003",
		Description: "add-ai-generated-article-detection: collapse duplicate active detection_jobs rows (keep the lowest id per article, merging priority=MAX / reason=latest non-empty into it) and add the partial unique index uq_detection_jobs_article_active ON detection_jobs (article_id) WHERE status IN ('pending','leased') so concurrent enqueues cannot double-insert (idempotent)",
		Up: func(db *gorm.DB) error {
			if db.Dialector == nil || db.Name() != "postgres" {
				return nil
			}
			if !tableExists(db, "detection_jobs") {
				return nil
			}
			// 先把待删行的语义合并进保留行（MIN(id)），再删它们。
			if err := db.Exec(`
				WITH active AS (
					SELECT id, article_id, priority, reason, updated_at
					FROM detection_jobs
					WHERE status IN ('pending', 'leased')
				),
				keep AS (
					SELECT article_id, MIN(id) AS keep_id
					FROM active
					GROUP BY article_id
				),
				merged AS (
					SELECT k.keep_id,
					       MAX(a.priority) AS priority,
					       (SELECT a2.reason FROM active a2
					         WHERE a2.article_id = k.article_id AND a2.reason <> ''
					         ORDER BY a2.updated_at DESC, a2.id DESC
					         LIMIT 1) AS reason
					FROM keep k
					JOIN active a ON a.article_id = k.article_id
					GROUP BY k.keep_id, k.article_id
				)
				UPDATE detection_jobs t
				SET priority = m.priority,
				    reason = COALESCE(m.reason, t.reason)
				FROM merged m
				WHERE t.id = m.keep_id`).Error; err != nil {
				return fmt.Errorf("merge duplicate active detection_jobs into the kept row: %w", err)
			}
			if err := db.Exec(`
				DELETE FROM detection_jobs
				WHERE status IN ('pending', 'leased')
				  AND id NOT IN (
					SELECT MIN(id) FROM detection_jobs
					WHERE status IN ('pending', 'leased')
					GROUP BY article_id
				  )`).Error; err != nil {
				return fmt.Errorf("collapse duplicate active detection_jobs: %w", err)
			}
			if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_detection_jobs_article_active ON detection_jobs (article_id) WHERE status IN ('pending', 'leased')`).Error; err != nil {
				return fmt.Errorf("create uq_detection_jobs_article_active: %w", err)
			}
			return nil
		},
	}
}
