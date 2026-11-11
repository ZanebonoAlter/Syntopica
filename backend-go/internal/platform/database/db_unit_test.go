package database

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// TestPostgresMigrationsDocumentDimensionAgnosticEmbeddingColumn 护栏：
// 20260403_0003 描述必须写明「向量列存在且维度无关」，且迁移正文里不得再出现
// 硬编码维度的目标类型（vector(N)）。
//
// 为什么改：灾备恢复时若备份档不带 schema_migrations，整条版本迁移链会在有数据的
// 库上从零重放；目标类型写死 vector(4096) 而存量列是 vector(2560) 时，重放即维度
// 冲突，恢复直接失败（fix-disaster-recovery-channel design D2）。
func TestPostgresMigrationsDocumentDimensionAgnosticEmbeddingColumn(t *testing.T) {
	migrations := postgresMigrations()
	if len(migrations) < 3 {
		t.Fatalf("postgres migrations count = %d, want at least 3", len(migrations))
	}

	migration := mustFindMigration(t, migrations, "20260403_0003")
	if !strings.Contains(migration.Description, "向量列") {
		t.Fatalf("expected vector column description, got %q", migration.Description)
	}
	if !strings.Contains(migration.Description, "维度") {
		t.Fatalf("expected dimension-agnostic description, got %q", migration.Description)
	}

	source, err := os.ReadFile("postgres_migrations.go")
	if err != nil {
		t.Fatalf("read postgres_migrations.go: %v", err)
	}
	// 定位 20260403_0003 的迁移块：Version 行 → 下一个 Version 行（或文件尾）。
	lines := strings.Split(string(source), "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		if start == -1 {
			if strings.Contains(line, `"20260403_0003"`) {
				start = i
			}
			continue
		}
		if strings.Contains(line, "Version:") {
			end = i
			break
		}
	}
	if start == -1 {
		t.Fatal("migration 20260403_0003 not found in source")
	}
	block := strings.Join(lines[start:end], "\n")
	if regexp.MustCompile(`vector\(\s*\d+\s*\)`).MatchString(block) {
		t.Fatalf("migration 20260403_0003 must not hardcode a vector dimension:\n%s", block)
	}
	if !strings.Contains(block, "TYPE vector") {
		t.Fatalf("migration 20260403_0003 must alter to undimensioned vector:\n%s", block)
	}
}

func TestTopicTagAnalysisPayloadJSONExplicitlyStaysTextInModel(t *testing.T) {
	field, ok := reflect.TypeOf(tagmodels.TopicTagAnalysis{}).FieldByName("PayloadJSON")
	if !ok {
		t.Fatal("PayloadJSON field not found")
	}

	if !strings.Contains(field.Tag.Get("gorm"), "type:text") {
		t.Fatalf("expected PayloadJSON gorm tag to keep text storage, got %q", field.Tag.Get("gorm"))
	}
}

func TestSemanticLabelBoardSystemMigrationDocumentsSchemaCutover(t *testing.T) {
	migration := mustFindMigration(t, postgresMigrations(), "20260521_0001")
	if !strings.Contains(strings.ToLower(migration.Description), "semantic") {
		t.Fatalf("expected semantic migration description, got %q", migration.Description)
	}

	source, err := os.ReadFile("postgres_migrations.go")
	if err != nil {
		t.Fatalf("read postgres_migrations.go: %v", err)
	}
	joined := string(source)

	// 表与列由 AutoMigrate（migrator.go 注册 SemanticLabel/TopicTagSemanticLabel 等）负责；显式迁移只验证索引 / seed / 历史 DROP。
	mustContainAll(t, joined,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_semantic_labels_slug",
		"CREATE INDEX IF NOT EXISTS idx_semantic_labels_label_type",
		"CREATE INDEX IF NOT EXISTS idx_semantic_labels_status",
		"CREATE INDEX IF NOT EXISTS idx_narrative_boards_semantic_board_id",
		"CREATE INDEX IF NOT EXISTS idx_topic_tag_board_labels_topic_tag_id",
		"CREATE INDEX IF NOT EXISTS idx_topic_tag_board_labels_semantic_board_id",
		`{"semantic_board_match_sim_threshold", "0.6"`,
		`{"semantic_board_match_direct_hit_rate", "0.5"`,
		`{"semantic_board_match_direct_max_sim", "0.8"`,
		`{"semantic_board_match_weight_sim", "0.6"`,
		`{"semantic_board_match_weight_density", "0.4"`,
		`{"semantic_board_match_weighted_threshold", "0.6"`,
		`{"semantic_board_match_direct_hit_min_overlap", "2"`,
		`{"semantic_board_match_max_boards", "3"`,
		`{"semantic_board_upgrade_ref_count_threshold", "5"`,
		`{"semantic_board_upgrade_cluster_distance_threshold", "0.35"`,
		`{"semantic_board_upgrade_cotag_window_days", "30"`,
		`{"semantic_board_upgrade_cotag_top_n", "20"`,
		`{"semantic_board_upgrade_cotag_dedupe_sim_threshold", "0.85"`,
		`{"semantic_board_upgrade_cotag_hard_limit", "15"`,
	)
	mustContainAll(t, joined,
		"ALTER TABLE topic_tags DROP COLUMN IF EXISTS concept_id",
		"DROP TABLE IF EXISTS board_concepts CASCADE",
		"DROP TABLE IF EXISTS hierarchy_configs CASCADE",
		"DROP TABLE IF EXISTS hierarchy_pending_changes CASCADE",
		"DROP TABLE IF EXISTS hierarchy_anchor_signals CASCADE",
		"DROP TABLE IF EXISTS rebuild_jobs CASCADE",
		"DROP TABLE IF EXISTS abstract_tag_update_queues CASCADE",
		"DROP TABLE IF EXISTS adopt_narrower_queues CASCADE",
		"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS abstract_tag_id",
		"ALTER TABLE narrative_boards DROP COLUMN IF EXISTS board_concept_id",
	)

	for _, deprecatedKey := range []string{
		"semantic_board_match_min_score",
		"semantic_board_match_top_k",
		"semantic_board_upgrade_min_auxiliary_labels",
		"semantic_board_upgrade_min_tag_count",
	} {
		if strings.Contains(joined, deprecatedKey) {
			t.Fatalf("expected migration not to seed deprecated ai_settings key %q", deprecatedKey)
		}
	}
}

func TestDailyReportTimeMigrationSeedsDefaultValue(t *testing.T) {
	migration := mustFindMigration(t, postgresMigrations(), "20260614_0002")
	if !strings.Contains(strings.ToLower(migration.Description), "daily_report_time") {
		t.Fatalf("expected daily_report_time migration description, got %q", migration.Description)
	}

	source, err := os.ReadFile("postgres_migrations.go")
	if err != nil {
		t.Fatalf("read postgres_migrations.go: %v", err)
	}
	joined := string(source)

	mustContainAll(t, joined,
		`"daily_report_time"`,
		`"21:00"`,
		`日报生成时刻`,
	)
}

func mustFindMigration(t *testing.T, migrations []Migration, version string) Migration {
	t.Helper()

	for _, migration := range migrations {
		if migration.Version == version {
			return migration
		}
	}

	t.Fatalf("migration %s not found", version)
	return Migration{}
}

func mustContainAll(t *testing.T, text string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected text to contain %q", needle)
		}
	}
}

func TestWatchHitUniqueIndexMigrationRegistered(t *testing.T) {
	migration := mustFindMigration(t, postgresMigrations(), "20260630_0002")
	if !strings.Contains(strings.ToLower(migration.Description), "topic_watch_hits") {
		t.Fatalf("expected topic_watch_hits migration description, got %q", migration.Description)
	}

	source, err := os.ReadFile("postgres_migrations.go")
	if err != nil {
		t.Fatalf("read postgres_migrations.go: %v", err)
	}
	joined := string(source)

	mustContainAll(t, joined,
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_watch_section_report ON topic_watch_hits(watch_id, section_id, report_id)",
		"DROP INDEX IF EXISTS idx_watch_section_report;",
	)
}

func TestWatchHitFKCascadeMigrationRegistered(t *testing.T) {
	migration := mustFindMigration(t, postgresMigrations(), "20260801_0002")
	if !strings.Contains(strings.ToLower(migration.Description), "topic_watch_hits") {
		t.Fatalf("expected topic_watch_hits migration description, got %q", migration.Description)
	}

	source, err := os.ReadFile("postgres_migrations.go")
	if err != nil {
		t.Fatalf("read postgres_migrations.go: %v", err)
	}
	joined := string(source)

	// 清理孤儿 + 幂等加 FK + 长锁守卫（design D2/D3/D4）。
	mustContainAll(t, joined,
		"DELETE FROM topic_watch_hits WHERE watch_id NOT IN (SELECT id FROM board_topic_watches)",
		"ADD CONSTRAINT fk_topic_watch_hits_watch",
		"FOREIGN KEY (watch_id) REFERENCES board_topic_watches(id)",
		"ON DELETE CASCADE",
		`withLockTimeout(db, "5s"`,
		"IF NOT EXISTS",
	)
}

func TestQualityBreakdownMigrationRegistered(t *testing.T) {
	migration := mustFindMigration(t, postgresMigrations(), "20260625_0001")
	if !strings.Contains(strings.ToLower(migration.Description), "quality_breakdown") {
		t.Fatalf("expected quality_breakdown migration description, got %q", migration.Description)
	}
}

func mustNotContainAny(t *testing.T, text string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			t.Fatalf("expected text not to contain %q", needle)
		}
	}
}
