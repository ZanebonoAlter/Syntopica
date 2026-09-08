package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// TestCompositeComponentsMigration exercises migration 20260902_0001
// (add-composite-labels) against a testcontainer PG in the production upgrade
// scenario: AutoMigrate creates composite_components from the model, the
// migration belt-and-braces ensures the FK ON DELETE CASCADE and seeds the
// three ai_settings knobs. Verifies table shape, cascade delete, natural
// compatibility of label_type='composite' (no enum CHECK), and the seeds.
//
// Docker required. Skipped under -short.
func TestCompositeComponentsMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM semantic_labels WHERE slug IN ('cmp-mig-composite', 'cmp-mig-aux-a', 'cmp-mig-aux-b')`).Error
		_ = db.Exec(`DELETE FROM ai_settings WHERE key IN ('composite_label_dedupe_sim','semantic_board_match_direct_hit_score_factor','semantic_board_upgrade_composite_min_cooccurrence')`).Error
	})

	// Locate migration 20260902_0001's Up closure and run it in-tx (mirrors the
	// production in-transaction path).
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260902_0001" {
			up = m.Up
			break
		}
	}
	require.NotNil(t, up, "migration 20260902_0001 not found in list")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))

	// 1. Table shape: PK columns exist with expected names (ordered).
	var pkCols []struct {
		ColumnName string
	}
	require.NoError(t, db.Raw(`
		SELECT kcu.column_name
		FROM information_schema.key_column_usage kcu
		JOIN information_schema.table_constraints tc
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_name = kcu.table_name
		WHERE tc.table_name = 'composite_components' AND tc.constraint_type = 'PRIMARY KEY'
		ORDER BY kcu.ordinal_position
	`).Scan(&pkCols).Error)
	require.Len(t, pkCols, 2, "composite_components PK should be (composite_id, component_label_id)")
	require.Equal(t, "composite_id", pkCols[0].ColumnName)
	require.Equal(t, "component_label_id", pkCols[1].ColumnName)

	// 2. FK ON DELETE CASCADE on composite_id → semantic_labels(id).
	var fkCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE constraint_name = 'fk_composite_components_composite'
		  AND table_name = 'composite_components' AND constraint_type = 'FOREIGN KEY'
	`).Scan(&fkCount).Error)
	require.Equal(t, int64(1), fkCount, "FK fk_composite_components_composite must exist")

	// 3. label_type='composite' is naturally accepted (no enum CHECK blocks it).
	seed := func(label, slug, labelType string) *models.SemanticLabel {
		row := &models.SemanticLabel{Label: label, Slug: slug, LabelType: labelType, Status: "active", Source: "manual"}
		require.NoError(t, db.Create(row).Error)
		return row
	}
	auxA := seed("美国国债", "cmp-mig-aux-a", "auxiliary")
	auxB := seed("收益率", "cmp-mig-aux-b", "auxiliary")
	composite := seed("美债收益率", "cmp-mig-composite", "composite")

	// 4. Ordered components persist and cascade-delete with the composite row.
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: composite.ID, ComponentLabelID: auxA.ID, Position: 1}).Error)
	require.NoError(t, db.Create(&models.CompositeComponent{CompositeID: composite.ID, ComponentLabelID: auxB.ID, Position: 2}).Error)
	var comps []models.CompositeComponent
	require.NoError(t, db.Where("composite_id = ?", composite.ID).Order("position").Find(&comps).Error)
	require.Len(t, comps, 2)
	require.Equal(t, 1, comps[0].Position)
	require.Equal(t, 2, comps[1].Position)

	require.NoError(t, db.Delete(&models.SemanticLabel{}, composite.ID).Error)
	var remain int64
	require.NoError(t, db.Model(&models.CompositeComponent{}).Where("composite_id = ?", composite.ID).Count(&remain).Error)
	require.Equal(t, int64(0), remain, "composite_components must cascade-delete with the composite label row")

	// 5. Seeds present with documented defaults.
	for key, want := range map[string]string{
		"composite_label_dedupe_sim":                        "0.95",
		"semantic_board_match_direct_hit_score_factor":      "0.7",
		"semantic_board_upgrade_composite_min_cooccurrence": "10",
	} {
		var setting models.AISettings
		require.NoError(t, db.Where("key = ?", key).First(&setting).Error, "seed %s must exist", key)
		require.Equal(t, want, setting.Value, "seed %s default value", key)
	}
}
