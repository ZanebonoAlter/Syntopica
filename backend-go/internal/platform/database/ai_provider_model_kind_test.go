package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// TestAIProviderModelKindBackfillMigration exercises migration 20260802_0001
// end-to-end against a testcontainer PG in the production first-run scenario:
// AutoMigrate adds ai_providers.model_kind (DEFAULT 'llm'), so every existing
// provider starts at 'llm'. The migration must backfill providers that are
// exclusively on an embedding route to 'embedding', leave llm-route providers
// alone, and warn (without flipping) providers bound to BOTH embedding and llm
// routes. This is the testing.md hard constraint: "schema 迁移要在 testcontainer
// PG + 历史数据下测".
//
// Docker required. Skipped under -short.
func TestAIProviderModelKindBackfillMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))

	// Hermetic cleanup against the shared process-singleton container: clear any
	// leftover fixture rows from a prior run (no FK enforcement under the test
	// config, so delete order is free; clean dependents first anyway).
	require.NoError(t, db.Exec(`DELETE FROM ai_route_providers WHERE provider_id IN (9101, 9102, 9103)`).Error)
	require.NoError(t, db.Exec(`DELETE FROM ai_routes WHERE id IN (9101, 9102)`).Error)
	require.NoError(t, db.Exec(`DELETE FROM ai_providers WHERE id IN (9101, 9102, 9103)`).Error)

	// Seed pre-migration state. AutoMigrate added model_kind with DEFAULT 'llm';
	// all three providers start at 'llm' (the production first-run state).
	//   pEmb  (9101): embedding route only  → must flip to embedding
	//   pLLM  (9102): summary (llm) route only → stays llm
	//   pBoth (9103): on BOTH routes         → conflict, warned, stays llm
	pEmb := models.AIProvider{ID: 9101, Name: "mk-mig-emb", ProviderType: "openai_compatible", BaseURL: "https://e/v1", Model: "emb", ModelKind: "llm", Enabled: true, TimeoutSeconds: 120}
	pLLM := models.AIProvider{ID: 9102, Name: "mk-mig-llm", ProviderType: "openai_compatible", BaseURL: "https://l/v1", Model: "gpt", ModelKind: "llm", Enabled: true, TimeoutSeconds: 120}
	pBoth := models.AIProvider{ID: 9103, Name: "mk-mig-both", ProviderType: "openai_compatible", BaseURL: "https://b/v1", Model: "mix", ModelKind: "llm", Enabled: true, TimeoutSeconds: 120}
	require.NoError(t, db.Create(&pEmb).Error)
	require.NoError(t, db.Create(&pLLM).Error)
	require.NoError(t, db.Create(&pBoth).Error)

	embRoute := models.AIRoute{ID: 9101, Name: "default", Capability: "embedding", Enabled: true, MaxConcurrency: 1}
	sumRoute := models.AIRoute{ID: 9102, Name: "default", Capability: "summary", Enabled: true, MaxConcurrency: 1}
	require.NoError(t, db.Create(&embRoute).Error)
	require.NoError(t, db.Create(&sumRoute).Error)

	// pEmb on embedding route; pLLM on summary route; pBoth on BOTH routes.
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: 9101, ProviderID: 9101, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: 9102, ProviderID: 9102, Priority: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: 9101, ProviderID: 9103, Priority: 2, Enabled: true}).Error)
	require.NoError(t, db.Create(&models.AIRouteProvider{RouteID: 9102, ProviderID: 9103, Priority: 3, Enabled: true}).Error)

	// Locate migration 20260802_0001's Up closure and run it in-tx (mirrors the
	// production default in-transaction path).
	var up func(*gorm.DB) error
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == "20260802_0001" {
			up = m.Up
			break
		}
	}
	require.NotNil(t, up, "migration 20260802_0001 not found in list")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"migration Up must succeed (backfill embedding-exclusive providers)")

	// Assertions:
	//   pEmb  → embedding (flipped: embedding-exclusive)
	//   pLLM  → llm (untouched)
	//   pBoth → llm (conflict: on both routes, left at default, warned only)
	var emb, llm, both models.AIProvider
	require.NoError(t, db.First(&emb, 9101).Error)
	require.NoError(t, db.First(&llm, 9102).Error)
	require.NoError(t, db.First(&both, 9103).Error)
	require.Equal(t, "embedding", emb.ModelKind, "embedding-exclusive provider must be backfilled to embedding")
	require.Equal(t, "llm", llm.ModelKind, "llm-route-only provider must stay llm")
	require.Equal(t, "llm", both.ModelKind, "conflict provider must stay llm (warned, not auto-flipped)")

	// Idempotency: re-running Up is a no-op (only flips llm→embedding; pEmb is
	// already embedding, so it is skipped).
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"migration must be idempotent")
	require.NoError(t, db.First(&emb, 9101).Error)
	require.NoError(t, db.First(&llm, 9102).Error)
	require.NoError(t, db.First(&both, 9103).Error)
	require.Equal(t, "embedding", emb.ModelKind)
	require.Equal(t, "llm", llm.ModelKind)
	require.Equal(t, "llm", both.ModelKind)
}
