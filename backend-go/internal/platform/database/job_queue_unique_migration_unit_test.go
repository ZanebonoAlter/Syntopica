package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// TestJobQueueUniqueMigrationsRegistered 非 Docker 单元测试（testcontainers 不可用的
// 环境下也能跑）：两个「同文章至多一条未完成作业」的部分唯一索引迁移必须
//
//   - 各自恰好注册一次（重复注册会让 RunMigrations 对同一版本重放）；
//   - 版本排在 20261120_0001（建 detection_jobs 表）之后，保证建索引时表已存在；
//   - 在非 postgres 方言上是 no-op——sqlite 模型单测由 AutoMigrate 建模，不该凭空
//     多出部分唯一索引（测试夹具自己按生产 DDL 建）。
func TestJobQueueUniqueMigrationsRegistered(t *testing.T) {
	for _, version := range []string{"20261120_0002", "20261120_0003"} {
		count := 0
		for _, m := range postgresMigrations() {
			if m.Version == version {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("migration %s registered %d times, want exactly 1", version, count)
		}
		if version <= "20261120_0001" {
			t.Fatalf("migration %s must sort after 20261120_0001 (detection_jobs table creation)", version)
		}
	}
}

// TestJobQueueUniqueMigrationsSkipNonPostgres 守住方言守卫：sqlite 上两个 Up 都是
// no-op（不报错、不建索引），否则模型单测库会带上生产才有的约束。
func TestJobQueueUniqueMigrationsSkipNonPostgres(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table string
		index string
		up    func(*gorm.DB) error
	}{
		{"tag_jobs", "tag_jobs", "uq_tag_jobs_article_active", tagJobUniqueMigration().Up},
		{"detection_jobs", "detection_jobs", "uq_detection_jobs_article_active", detectionJobUniqueMigration().Up},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+tc.name+"-migration-guard?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatalf("open sqlite: %v", err)
			}
			if err := db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TagJob{}, &models.DetectionJob{}); err != nil {
				t.Fatalf("migrate: %v", err)
			}

			if err := tc.up(db); err != nil {
				t.Fatalf("Up on sqlite must be a no-op, got error: %v", err)
			}

			var count int64
			if err := db.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name = ?`, tc.index).Scan(&count).Error; err != nil {
				t.Fatalf("query sqlite_master: %v", err)
			}
			if count != 0 {
				t.Fatalf("index %s must not be created on sqlite (dialect guard)", tc.index)
			}
		})
	}
}
