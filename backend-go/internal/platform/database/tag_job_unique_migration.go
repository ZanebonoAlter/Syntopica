package database

import (
	"fmt"

	"gorm.io/gorm"
)

// tagJobUniqueMigration 实现 20261120_0002（fix-feed-refresh-churn design D4）：
// tag_jobs 加 (article_id) WHERE status IN ('pending','leased') 部分唯一索引，
// 把「同文章至多一条未完成作业」从应用层 check-then-insert 约定升级为数据库级约束。
//
// 为什么需要：Enqueue 在默认 read committed 隔离下是事务内 check-then-insert，两条
// 并发 firecrawl 回调在同一秒各插一条（文章 178599 于 18:59:31 产生 2 条 pending
// 实证）。应用层的「先查后插」无论怎么写都关不上这个窗口，只有索引能关。
//
// 建索引前先清存量重复：同 article_id 的多条 pending/leased 只保留最小 id（最早入队
// 那条），其余删除——否则 CREATE UNIQUE INDEX 会因存量数据直接失败、连带启动失败。
// 删除前先把较新行的语义合并进保留行（审查 R-5）：priority 取 MAX、force_retag 取
// OR（否则一次 force 重打标的意图会随被删行丢失）、reason 取最新非空行（同一
// updated_at 时取较大 id）。
//
// 生产库已执行过本迁移（版本 20261120_0002 已记录），因此上述合并语义只对全新环境
// （新部署 / 测试库重建）生效；已跑过旧版迁移的库不会回补。
//
// 幂等：UPDATE 与 DELETE 收敛后重复执行无行可改/可删；CREATE INDEX IF NOT EXISTS
// 重复执行 no-op。表在千行级，普通建索引即可——版本化迁移本就在事务内跑，
// CONCURRENTLY 反而不可用。
//
// 非 postgres 方言（sqlite 模型单测）直接跳过：部分唯一索引是 PG/SQLite 共通语法，
// 但 sqlite 测试库由 AutoMigrate 建模、不走版本迁移，无需在此重复建。
func tagJobUniqueMigration() Migration {
	return Migration{
		Version:     "20261120_0002",
		Description: "fix-feed-refresh-churn: collapse duplicate active tag_jobs rows (keep the lowest id per article, merging priority=MAX / force_retag=OR / reason=latest non-empty into it) and add the partial unique index uq_tag_jobs_article_active ON tag_jobs (article_id) WHERE status IN ('pending','leased') so concurrent enqueues cannot double-insert (idempotent)",
		Up: func(db *gorm.DB) error {
			if db.Dialector == nil || db.Name() != "postgres" {
				return nil
			}
			if !tableExists(db, "tag_jobs") {
				return nil
			}
			// 先把待删行的语义合并进保留行（MIN(id)），再删它们。
			if err := db.Exec(`
				WITH active AS (
					SELECT id, article_id, priority, force_retag, reason, updated_at
					FROM tag_jobs
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
					       BOOL_OR(a.force_retag) AS force_retag,
					       (SELECT a2.reason FROM active a2
					         WHERE a2.article_id = k.article_id AND a2.reason <> ''
					         ORDER BY a2.updated_at DESC, a2.id DESC
					         LIMIT 1) AS reason
					FROM keep k
					JOIN active a ON a.article_id = k.article_id
					GROUP BY k.keep_id, k.article_id
				)
				UPDATE tag_jobs t
				SET priority = m.priority,
				    force_retag = m.force_retag,
				    reason = COALESCE(m.reason, t.reason)
				FROM merged m
				WHERE t.id = m.keep_id`).Error; err != nil {
				return fmt.Errorf("merge duplicate active tag_jobs into the kept row: %w", err)
			}
			if err := db.Exec(`
				DELETE FROM tag_jobs
				WHERE status IN ('pending', 'leased')
				  AND id NOT IN (
					SELECT MIN(id) FROM tag_jobs
					WHERE status IN ('pending', 'leased')
					GROUP BY article_id
				  )`).Error; err != nil {
				return fmt.Errorf("collapse duplicate active tag_jobs: %w", err)
			}
			if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_jobs_article_active ON tag_jobs (article_id) WHERE status IN ('pending', 'leased')`).Error; err != nil {
				return fmt.Errorf("create uq_tag_jobs_article_active: %w", err)
			}
			return nil
		},
	}
}
