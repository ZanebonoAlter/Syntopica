package database_test

// add-article-search tasks 1.1 + 1.2 — migration A (20261013_0001: pg_trgm
// extension + GIN trgm indexes on articles.title/description) and migration B
// (20261013_0002: drop the retired tsvector apparatus — search_vector column,
// articles_search_vector_trigger, articles_search_vector_update(),
// idx_articles_search_vector).
//
// Asserted here per test-cases.md 故事 S4 主链路步 3/4 plus the 变体 walk
// 「幂等：迁移重跑 IF NOT EXISTS / IF EXISTS 全幂等」. All SQL runs inside the
// isolated testcontainer (DSN red line: never touches a real database).
//
// Gate mechanics (verified empirically): RunMigrations reads the destructive
// gate from config.AppConfig, NOT from the env — so testutil's golden-schema
// build leaves the gate CLOSED and migration B self-skips there (column stays;
// production default). These tests control the gate via config.AppConfig
// directly, mirroring destructive_guard_test.go's runMigrationsWithGate.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// applyPgTrgmChain runs the full production migration chain (AutoMigrate +
// RunMigrations) on the isolated container with the destructive gate held in
// the requested state. On a fresh container this applies everything from
// 20260403_0001 up; on a shared one only pending versions run.
func applyPgTrgmChain(t *testing.T, allowDestructive bool) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	config.AppConfig = &config.Config{Database: config.DatabaseConfig{AllowDestructiveMigrations: allowDestructive}}
	require.NoError(t, database.RunMigrations(db))
}

// rerunSearchMigrations re-executes exactly migrations A+B by clearing their
// recorded versions and re-running the chain with the gate open (versions are
// recorded even when B self-skips, per db-migration-safety).
func rerunSearchMigrations(t *testing.T) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec(
		`DELETE FROM schema_migrations WHERE version IN ('20261013_0001','20261013_0002')`).Error)
	config.AppConfig = &config.Config{Database: config.DatabaseConfig{AllowDestructiveMigrations: true}}
	require.NoError(t, database.RunMigrations(db))
}

// countRows is the shared scalar helper for the catalog probes below.
func countRows(t *testing.T, query string, args ...interface{}) int {
	t.Helper()
	db := testutil.OpenTestDB(t)
	var n int
	require.NoError(t, db.Raw(query, args...).Scan(&n).Error)
	return n
}

// TestPgTrgmMigrationInstallsExtensionAndIndexes covers test-cases.md 故事 S4
// 步 3 (migration A, task 1.1): after the chain runs, pg_trgm is installed,
// both GIN trgm indexes exist on articles, show_trgm produces multiple
// trigrams for a CJK phrase, and — gate still closed — the destructive
// migration B self-skipped, leaving search_vector in place (production-safe
// default, mirrored from destructive_guard_test.go).
func TestPgTrgmMigrationInstallsExtensionAndIndexes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	applyPgTrgmChain(t, false /* allowDestructive: production default */)

	require.Equal(t, 1, countRows(t, `SELECT COUNT(*) FROM pg_extension WHERE extname = 'pg_trgm'`),
		"pg_trgm extension must be installed")

	for _, idx := range []string{"idx_articles_title_trgm", "idx_articles_description_trgm"} {
		require.Equal(t, 1, countRows(
			t, `SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'articles' AND indexname = ?`, idx),
			"%s must exist on articles", idx)
		def := countRows(t,
			`SELECT COUNT(*) FROM pg_indexes WHERE indexname = ? AND indexdef LIKE '%USING gin%' AND indexdef LIKE '%gin_trgm_ops%'`, idx)
		require.Equal(t, 1, def, "%s must be a gin_trgm_ops GIN index", idx)
	}

	var trigrams int
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Raw(`SELECT array_length(show_trgm('特斯拉电池'), 1)`).Scan(&trigrams).Error)
	require.Greater(t, trigrams, 3, "show_trgm('特斯拉电池') must yield multiple trigrams, got %d", trigrams)

	// Production default: gate closed → migration B self-skipped and recorded;
	// the retired column survives until the gate is opened.
	require.Equal(t, 1, countRows(t,
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'articles' AND column_name = 'search_vector'`),
		"with the gate closed migration B must self-skip (search_vector column stays)")
}

// TestSearchVectorDropMigrationRemovesColumnTriggerIndex covers test-cases.md
// 故事 S4 步 4 (migration B, task 1.2): with the destructive gate open, the
// tsvector apparatus is fully gone — column, trigger, function, index. The
// trigger/function/index are recreated first so the drop path is exercised
// against the worst-case legacy state (a pre-20260820 database still carries
// all four objects), and the migration re-runs once more to prove idempotency
// (变体「幂等：迁移重跑」).
func TestSearchVectorDropMigrationRemovesColumnTriggerIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	// Baseline chain (any prior gate state): search_vector column exists from
	// 20260417_0002; trigger/index may already be gone via 20260820_0001.
	applyPgTrgmChain(t, false /* allowDestructive */)
	require.Equal(t, 1, countRows(t,
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'articles' AND column_name = 'search_vector'`),
		"precondition: search_vector column exists before the gated drop")

	// Recreate the full legacy apparatus so migration B has something to drop
	// in every slot (mirrors the 20260417_0002 definitions).
	db := testutil.OpenTestDB(t)
	require.NoError(t, db.Exec(`CREATE OR REPLACE FUNCTION articles_search_vector_update() RETURNS trigger AS $$
				BEGIN
					NEW.search_vector :=
						setweight(to_tsvector('simple', COALESCE(NEW.title, '')), 'A') ||
						setweight(to_tsvector('simple', COALESCE(NEW.description, '')), 'B');
					RETURN NEW;
				END;
				$$ LANGUAGE plpgsql`).Error, "recreate legacy trigger function")
	require.NoError(t, db.Exec(`DROP TRIGGER IF EXISTS articles_search_vector_trigger ON articles`).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER articles_search_vector_trigger
				BEFORE INSERT OR UPDATE OF title, description ON articles
				FOR EACH ROW EXECUTE FUNCTION articles_search_vector_update()`).Error, "recreate legacy trigger")
	require.NoError(t, db.Exec(`CREATE INDEX IF NOT EXISTS idx_articles_search_vector ON articles USING GIN (search_vector)`).Error,
		"recreate legacy GIN index")

	// Gate open + re-run migrations A/B.
	rerunSearchMigrations(t)

	require.Equal(t, 0, countRows(t,
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'articles' AND column_name = 'search_vector'`),
		"search_vector column must be dropped")
	require.Equal(t, 0, countRows(t,
		`SELECT COUNT(*) FROM pg_trigger WHERE tgname = 'articles_search_vector_trigger'`),
		"articles_search_vector_trigger must be dropped")
	require.Equal(t, 0, countRows(t,
		`SELECT COUNT(*) FROM pg_proc WHERE proname = 'articles_search_vector_update'`),
		"articles_search_vector_update() must be dropped")
	require.Equal(t, 0, countRows(t,
		`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'articles' AND indexname = 'idx_articles_search_vector'`),
		"idx_articles_search_vector must be dropped")

	// Migration A survives the re-run (IF NOT EXISTS) and B re-run is a no-op.
	require.Equal(t, 1, countRows(t, `SELECT COUNT(*) FROM pg_extension WHERE extname = 'pg_trgm'`),
		"pg_trgm must still be installed after migration B")
	rerunSearchMigrations(t)
	require.Equal(t, 0, countRows(t,
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'articles' AND column_name = 'search_vector'`),
		"idempotent re-run: column stays dropped")
}
