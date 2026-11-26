package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
)

// TestTagJobUniqueMigrationRegistered 守住 20261120_0002 的接线（fix-feed-refresh-churn
// D4）：版本恰好注册一次（重复注册会让 RunMigrations 重放同一迁移），且排在
// 20261120_0001 之后。不依赖 Docker，归档门禁与净环境都能跑。
func TestTagJobUniqueMigrationRegistered(t *testing.T) {
	version := "20261120_0002"
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
		t.Fatalf("migration %s must sort after 20261120_0001", version)
	}
}

// TestTagJobUniqueMigrationSkipsNonPostgres 守住方言守卫：sqlite 上 Up 是 no-op
// （不报错、不建索引），否则模型单测库会带上生产才有的约束。
func TestTagJobUniqueMigrationSkipsNonPostgres(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tag-job-unique-migration-guard?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Feed{}, &models.Article{}, &models.TagJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := tagJobUniqueMigration().Up(db); err != nil {
		t.Fatalf("Up on sqlite must be a no-op, got error: %v", err)
	}

	var count int64
	if err := db.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name = 'uq_tag_jobs_article_active'`).Scan(&count).Error; err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 0 {
		t.Fatalf("index uq_tag_jobs_article_active must not be created on sqlite (dialect guard)")
	}
}
