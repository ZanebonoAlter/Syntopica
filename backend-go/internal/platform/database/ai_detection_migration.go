package database

import (
	"fmt"

	"gorm.io/gorm"
)

// aiDetectionSchemaMigration 实现 20261120_0001（add-ai-generated-article-detection design D6/D1）：
//
//   - articles 新增四列：ai_detection_status（VARCHAR(20)，默认 'unknown' = 未判定）、
//     ai_detection_score（DOUBLE PRECISION，默认 0）、ai_detection_source（VARCHAR(20)，
//     默认 ” = 尚无来源）、ai_detection_at（TIMESTAMPTZ，可空 = 未判定时间）。
//   - feeds 新增 ai_detection_enabled（BOOLEAN，默认 false = 订阅源级判定开关关闭）。
//   - 新建 detection_jobs 作业表（一篇一作业、租约、重试上限；与 tag_jobs 同构但相互分离）。
//
// 幂等：全部 ADD COLUMN IF NOT EXISTS + CREATE TABLE IF NOT EXISTS + CREATE INDEX IF
// NOT EXISTS；重复执行不报错、不改存量行。
//
// 存量行语义：articles 的四列带 DEFAULT，存量文章判定状态一律为「未判定」
// （ai_detection_status='unknown'），界面不展示任何疑似标记；feeds 的新列默认关闭，
// 不产生任何判定作业。
//
// 表不存在（域模型未注册的最小部署）时跳过对应表，不中断启动。
func aiDetectionSchemaMigration() Migration {
	return Migration{
		Version:     "20261120_0001",
		Description: "add-ai-generated-article-detection: add articles.ai_detection_{status,score,source,at} + feeds.ai_detection_enabled columns and create the detection_jobs queue table (idempotent; defaults keep existing rows unjudged and all feeds disabled)",
		Up: func(db *gorm.DB) error {
			// Postgres-only DDL（BIGSERIAL / TIMESTAMPTZ / ALTER COLUMN DEFAULT）；
			// SQLite（模型单测）上直接跳过——不把方言差异变成迁移噪声。
			if db.Dialector == nil || db.Name() != "postgres" {
				return nil
			}
			if tableExists(db, "articles") {
				for _, stmt := range []string{
					`ALTER TABLE articles ADD COLUMN IF NOT EXISTS ai_detection_status VARCHAR(20) NOT NULL DEFAULT 'unknown'`,
					`ALTER TABLE articles ADD COLUMN IF NOT EXISTS ai_detection_score DOUBLE PRECISION NOT NULL DEFAULT 0`,
					`ALTER TABLE articles ADD COLUMN IF NOT EXISTS ai_detection_source VARCHAR(20) NOT NULL DEFAULT ''`,
					`ALTER TABLE articles ADD COLUMN IF NOT EXISTS ai_detection_at TIMESTAMPTZ NULL`,
				} {
					if err := db.Exec(stmt).Error; err != nil {
						return fmt.Errorf("add articles ai_detection column: %w", err)
					}
				}
			}

			if tableExists(db, "feeds") {
				if err := db.Exec(`ALTER TABLE feeds ADD COLUMN IF NOT EXISTS ai_detection_enabled BOOLEAN NOT NULL DEFAULT false`).Error; err != nil {
					return fmt.Errorf("add feeds.ai_detection_enabled: %w", err)
				}
			}

			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS detection_jobs (
					id               BIGSERIAL   PRIMARY KEY,
					article_id       BIGINT      NOT NULL,
					status           VARCHAR(20) NOT NULL DEFAULT 'pending',
					priority         INTEGER     NOT NULL DEFAULT 0,
					attempt_count    INTEGER     NOT NULL DEFAULT 0,
					max_attempts     INTEGER     NOT NULL DEFAULT 5,
					available_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
					leased_at        TIMESTAMPTZ NULL,
					lease_expires_at TIMESTAMPTZ NULL,
					last_error       TEXT        NOT NULL DEFAULT '',
					reason           VARCHAR(50) NOT NULL DEFAULT '',
					created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
					updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
				)`).Error; err != nil {
				return fmt.Errorf("create detection_jobs: %w", err)
			}
			for _, stmt := range []string{
				`CREATE INDEX IF NOT EXISTS idx_detection_jobs_article_id ON detection_jobs (article_id)`,
				`CREATE INDEX IF NOT EXISTS idx_detection_jobs_status ON detection_jobs (status)`,
				`CREATE INDEX IF NOT EXISTS idx_detection_jobs_available_at ON detection_jobs (available_at)`,
				`CREATE INDEX IF NOT EXISTS idx_detection_jobs_lease_expires_at ON detection_jobs (lease_expires_at)`,
				`CREATE INDEX IF NOT EXISTS idx_detection_jobs_created_at ON detection_jobs (created_at)`,
			} {
				if err := db.Exec(stmt).Error; err != nil {
					return fmt.Errorf("create detection_jobs index: %w", err)
				}
			}

			return nil
		},
	}
}
