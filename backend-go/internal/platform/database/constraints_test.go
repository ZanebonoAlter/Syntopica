package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"syntopica-backend/internal/platform/testutil"

	// Blank import registers scheduler-owned models (scheduler_tasks, task.go
	// init → database.RegisterModels) for this test binary's AutoMigrate. The
	// information_schema assertions below need scheduler_tasks to exist in the
	// golden schema; RunAutoMigrate's table set depends on these init() side
	// effects (platform/database cannot reference the package statically —
	// import cycle, see task.go:11).
	_ "syntopica-backend/internal/platform/scheduler"
)

// This integration test guards against "constraint vacuum": after the model-tag
// cleanup migration (20260723_0001), the NOT NULL / DEFAULT constraints that used
// to live in GORM tags must be materialized in the DB via explicit migrations.
// If someone removes the model tag but forgets the migration, a fresh DB would
// create nullable columns with no default — this test fails loudly in that case.
//
// It runs against a throwaway testcontainer (via testutil.SetupTestDB), so Docker
// must be available. It is skipped under -short.

// columnConstraint describes the expected DB-level constraint for one column.
type columnConstraint struct {
	table   string
	column  string
	notNull bool   // expect is_nullable='NO'
	def     string // expect column_default to contain this substring ("" = don't check default)
}

func TestModelTagConstraints_MaterializedInDB(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	cases := []columnConstraint{
		// ── ai_models.go ──
		{"scheduler_tasks", "name", true, ""},
		{"scheduler_tasks", "check_interval", true, "60"},
		{"scheduler_tasks", "status", false, "idle"},
		{"ai_settings", "key", true, ""},
		{"ai_providers", "name", true, ""},
		{"ai_providers", "provider_type", true, "openai_compatible"},
		{"ai_providers", "enabled", true, "true"},
		{"ai_providers", "timeout_seconds", true, "120"},
		{"ai_providers", "enable_thinking", true, "false"},
		{"ai_routes", "name", true, ""},
		{"ai_routes", "capability", true, ""},
		{"ai_routes", "enabled", true, "true"},
		// ai_routes.priority/strategy 死列已随 vendor/model 两层化移除（全新库不再建列）。
		{"ai_routes", "max_concurrency", true, "0"},
		{"ai_route_providers", "route_id", true, ""},
		{"ai_route_providers", "provider_id", true, ""},
		{"ai_route_providers", "priority", true, "100"},
		{"ai_route_providers", "enabled", true, "true"},
		{"ai_call_logs", "capability", true, ""},
		{"ai_call_logs", "route_name", true, ""},
		{"ai_call_logs", "provider_name", true, ""},
		{"ai_call_logs", "success", true, ""},
		{"ai_call_logs", "is_fallback", true, "false"},

		// ── topic_graph.go ──
		{"topic_tags", "slug", true, ""},
		{"topic_tags", "label", true, ""},
		{"topic_tags", "category", true, "keyword"},
		{"topic_tags", "status", true, "active"},
		{"topic_tags", "is_canonical", false, "false"},
		{"topic_tags", "source", false, "llm"},
		{"topic_tags", "feed_count", false, "0"},
		{"topic_tags", "is_watched", false, "false"},
		{"topic_tags", "quality_score", false, "0"},
		{"topic_tags", "kind", false, "keyword"},
		{"topic_tag_embeddings", "topic_tag_id", true, ""},
		{"topic_tag_embeddings", "embedding_type", true, "identity"},
		{"topic_tag_embeddings", "dimension", true, ""},
		{"topic_tag_embeddings", "model", true, ""},
		{"tag_merge_suggestions", "new_tag_id", true, ""},
		{"tag_merge_suggestions", "existing_tag_id", true, ""},
		{"tag_merge_suggestions", "new_label", true, ""},
		{"tag_merge_suggestions", "existing_label", true, ""},
		{"tag_merge_suggestions", "category", true, ""},
		{"tag_merge_suggestions", "similarity", true, ""},
		{"tag_merge_suggestions", "status", true, "pending"},
		{"tag_merge_suggestions", "source", true, "incremental"},
		{"article_topic_tags", "article_id", true, ""},
		{"article_topic_tags", "topic_tag_id", true, ""},
		{"article_topic_tags", "score", false, "0"},
		{"article_topic_tags", "source", false, "llm"},

		// ── semantic_label.go ──
		{"semantic_labels", "label", true, ""},
		{"semantic_labels", "slug", true, ""},
		{"semantic_labels", "label_type", true, ""},
		{"semantic_labels", "ref_count", true, "0"},
		{"semantic_labels", "display_order", true, "0"},
		{"semantic_labels", "source", true, "llm_extract"},
		{"semantic_labels", "status", true, "active"},
		{"semantic_labels", "protected", true, "false"},
		{"semantic_labels", "enrichment_enabled", true, "false"},
		{"semantic_labels", "window_days", true, "14"},
		{"semantic_labels", "aliases", false, "["},           // default '[]'::jsonb
		{"semantic_labels", "context_layers", false, "week"}, // default array contains "week"
		{"topic_tag_board_labels", "score", true, "0"},
		{"topic_tag_board_labels", "downgraded", true, "false"},
		{"topic_tag_board_labels", "direction_mismatch", true, "false"},
	}

	type row struct {
		IsNullable    string  `gorm:"column:is_nullable"`
		ColumnDefault *string `gorm:"column:column_default"`
	}

	for _, c := range cases {
		var r row
		err := db.Raw(
			`SELECT is_nullable, column_default FROM information_schema.columns
			 WHERE table_schema='public' AND table_name=? AND column_name=?`,
			c.table, c.column,
		).Row().Scan(&r.IsNullable, &r.ColumnDefault)
		require.NoError(t, err, "lookup %s.%s", c.table, c.column)

		if c.notNull {
			require.Equal(t, "NO", r.IsNullable,
				"%s.%s should be NOT NULL (was driven by model tag; must now be materialized by migration 20260723_0001)",
				c.table, c.column)
		}
		if c.def != "" {
			require.NotNil(t, r.ColumnDefault, "%s.%s should have a DEFAULT", c.table, c.column)
			require.Contains(t, *r.ColumnDefault, c.def,
				"%s.%s default should contain %q, got %v", c.table, c.column, c.def, r.ColumnDefault)
		}
	}
}
