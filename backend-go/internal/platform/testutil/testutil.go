package testutil

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// ── Safety contract ──────────────────────────────────────────────────────────
//
// This package connects ONLY to a throwaway Postgres started in an isolated
// Docker container via testcontainers-go. It has NO default DSN and reads NO
// environment variable that could redirect it at the developer's docker-compose
// Postgres (which is the production database). An earlier revision connected to
// the production database and truncated it — that path no longer exists.
//
// Container cleanup is delegated entirely to the Testcontainers Ryuk sidecar
// (this package intentionally never calls TerminateContainer). If the Ryuk image
// cannot be pulled — default `testcontainers/ryuk:<version>` — the container is
// created but never reclaimed, and the test still passes: the leak is silent.
// Image list, symptoms and cleanup: docs/reference/standard/backend/testing.md

const (
	// pgImage is the throwaway container image. Same image as production
	// (docker-compose.pg.yml) so pgvector behavior matches, but started in its
	// own container that is destroyed when the test process exits.
	pgImage = "pgvector/pgvector:pg18-trixie"

	testDBName = "syntopica"
	testDBUser = "postgres"
	testDBPass = "postgres"
)

var (
	startOnce sync.Once // starts the container + opens the connection, once per process
	resetMu   sync.Mutex
	cachedDB  *gorm.DB
	startErr  error
	cachedDSN string // container connection string; used to reopen the pool after a schema rebuild

	// Golden-schema state. migrateOnce builds the schema once per process;
	// every later SetupTestDB call resets via ResetTestData (fast path).
	migrateOnce        sync.Once // runs runTestMigrations + takeSeedSnapshot once
	goldenSchemaErr    error     // captures first-build error, surfaced on every call
	migrationsRunCount int64     // test-only observable: how often runTestMigrations ran
	setupCallCount     int64     // distinguishes first SetupTestDB call from later ones
	seedSnapshotMu     sync.Mutex
	aiSettingsSeed     []models.AISettings
	embeddingCfgSeed   []models.EmbeddingConfig

	// goldenVectorColumns snapshots vector-typed columns at golden-build so
	// ResetTestData can re-ALTER them back if a test mutated the dimension
	// (some tests do: ALTER COLUMN embedding TYPE vector(N)). typeDecl is the
	// format_type() string captured at golden build (e.g. "vector").
	goldenVectorColumns []vectorColumnInfo
)

// vectorColumnInfo records a vector-typed column's golden-build type so
// ResetTestData can undo dimension mutations tests make via ALTER COLUMN.
type vectorColumnInfo struct {
	table    string
	column   string
	typeDecl string // pg_catalog.format_type output, e.g. "vector"
}

// OpenTestDB returns a *gorm.DB connected to a throwaway pgvector Postgres
// running in an isolated Docker container (started via testcontainers-go).
//
// The container is started once per test process and reused by every test in
// the process. The Testcontainers Ryuk sidecar terminates it on process exit,
// so no manual cleanup is needed.
//
// The returned connection NEVER points at the production database: there is no
// default DSN and no environment-variable override. This is deliberate.
func OpenTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	startOnce.Do(func() {
		startErr = startContainerAndConnect()
	})

	if startErr != nil {
		t.Fatalf("start test database container (requires Docker running): %v\n"+
			"hint: start Docker Desktop / daemon", startErr)
	}

	return cachedDB
}

// startContainerAndConnect starts the isolated pgvector container and opens a
// cached *gorm.DB against it. Runs exactly once per process via startOnce.
func startContainerAndConnect() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	ctr, err := tcpostgres.Run(ctx,
		pgImage,
		tcpostgres.WithDatabase(testDBName),
		tcpostgres.WithUsername(testDBUser),
		tcpostgres.WithPassword(testDBPass),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return fmt.Errorf("run postgres container: %w", err)
	}
	// Container cleanup is delegated to the Testcontainers Ryuk sidecar, which
	// runs on process exit (including crashes). We intentionally do NOT call
	// TerminateContainer here: the container is process-singleton and must
	// outlive each individual test function.

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("get container connection string: %w", err)
	}

	// Verbatim copy of production (database.InitDB): AutoMigrate must NOT create
	// FK constraints, because the versioned migrations are the source of truth
	// for which FKs exist. Production drops several FKs on purpose (e.g.
	// fk_topic_tag_relations_*, fk_merge_reembedding_queues_*) so that
	// MergeTags/HardMergeTags can hard-delete a tag still referenced from those
	// tables. With the default gorm.Config{} AutoMigrate would CREATE those FKs
	// and never drop them, leaving the test DB stricter than production and
	// breaking those code paths for no real reason.
	db, err := openGorm(connStr)
	if err != nil {
		return fmt.Errorf("gorm open: %w", err)
	}

	cachedDSN = connStr
	cachedDB = db
	return nil
}

// openGorm opens a *gorm.DB against dsn using the same config production's
// InitDB uses (SlowLogger, no FK constraints during migrate). ReimportTestDB
// calls it to open a fresh pool after rebuilding the schema.
//
// The SlowLogger mirrors production: it swallows gorm.ErrRecordNotFound (e.g.
// the check-then-create existence probes inside the embedding_config seed
// migrations) instead of surfacing them as warnings like GORM's default logger.
func openGorm(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   database.NewSlowLogger(200 * time.Millisecond),
	})
}

// SetupTestDB is the single entry point for integration tests.
// It:
//  1. Skips when running with -short flag.
//  2. Starts (or reuses) the isolated pgvector container.
//  3. On the FIRST call of the process, builds the golden schema once
//     (runTestMigrations) and snapshots migration seed rows.
//  4. On every LATER call, resets via the fast path (ResetTestData) instead of
//     rebuilding the schema — this is the ~6x speedup.
//  5. Sets database.DB for production code compatibility.
//
// Every integration test should start with: db := testutil.SetupTestDB(t)
func SetupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("requires Postgres (testcontainers)")
	}

	db := OpenTestDB(t)

	// Build the golden schema (migrations + seed snapshot) exactly once per
	// process. Later SetupTestDB calls skip this and reset via ResetTestData.
	migrateOnce.Do(func() {
		if err := runTestMigrations(t, db); err != nil {
			goldenSchemaErr = fmt.Errorf("build golden schema: %w", err)
			return
		}
		if err := takeSeedSnapshot(db); err != nil {
			goldenSchemaErr = fmt.Errorf("snapshot seed: %w", err)
			return
		}
		if err := snapshotVectorColumns(db); err != nil {
			goldenSchemaErr = fmt.Errorf("snapshot vector columns: %w", err)
			return
		}
	})
	if goldenSchemaErr != nil {
		t.Fatalf("%v", goldenSchemaErr)
	}

	// First call: the schema was just built and is already in the fresh
	// post-migration state — return it without a redundant truncate. Every
	// later call resets via the fast path.
	if atomic.AddInt64(&setupCallCount, 1) == 1 {
		database.DB = db
		return db
	}
	return ResetTestData(t, db)
}

// ReimportTestDB rebuilds the isolated test schema and reruns the production
// migration path. Regression tests can call it explicitly to restore the same
// schema and seed data that a fresh production database receives.
func ReimportTestDB(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()

	resetMu.Lock()
	defer resetMu.Unlock()

	if err := db.Exec("DROP SCHEMA IF EXISTS public CASCADE").Error; err != nil {
		t.Fatalf("drop test schema: %v", err)
	}
	if err := db.Exec("CREATE SCHEMA public").Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	if err := runTestMigrations(t, db); err != nil {
		t.Fatalf("import production database state: %v", err)
	}

	// Reopen the connection pool. Rebuilding the schema (DROP SCHEMA public
	// CASCADE) recreates the pgvector `vector` type with a NEW oid each cycle
	// (measured: 16387 -> 17280 -> 18173 across three rebuilds). The pool we
	// just used still holds server-side prepared statements that reference the
	// OLD oid, so any subsequent vector-column query fails with either
	// `cache lookup failed for type <old-oid>` or `cached plan must not change
	// result type`. Closing this pool and opening a fresh one gives connections
	// whose prepared-statement cache matches the rebuilt catalog — exactly what
	// a production restart does (new connection -> fresh catalog view).
	freshDB, err := openGorm(cachedDSN)
	if err != nil {
		t.Fatalf("reopen test database after schema rebuild: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	cachedDB = freshDB
	database.DB = freshDB
	return freshDB
}

// TruncateAllTables truncates all user tables using CASCADE. This is the reset
// helper for tests that deliberately need an empty database without seed data.
func TruncateAllTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	var tables []string
	if err := db.Raw(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'",
	).Scan(&tables).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}

	if len(tables) == 0 {
		return
	}

	if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", strings.Join(tables, ", "))).Error; err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

// snapshotVectorColumns records the golden-build type declaration of every
// vector-typed column in the public schema. ResetTestData re-ALTERs columns
// back to these declarations to undo any dimension mutation a test made
// (e.g. ALTER COLUMN embedding TYPE vector(2560)).
func snapshotVectorColumns(db *gorm.DB) error {
	goldenVectorColumns = nil
	type row struct {
		Table    string
		Column   string
		TypeDecl string
	}
	var rows []row
	if err := db.Raw(`
		SELECT c.table_name AS "table", c.column_name AS "column",
		       pg_catalog.format_type(a.atttypid, a.atttypmod) AS "type_decl"
		FROM information_schema.columns c
		JOIN pg_catalog.pg_class cls ON cls.relname = c.table_name
		JOIN pg_catalog.pg_namespace ns ON ns.oid = cls.relnamespace AND ns.nspname = 'public'
		JOIN pg_catalog.pg_attribute a ON a.attrelid = cls.oid AND a.attname = c.column_name
		WHERE c.table_schema = 'public' AND c.udt_name = 'vector'
	`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("snapshot vector columns: %w", err)
	}
	for _, r := range rows {
		goldenVectorColumns = append(goldenVectorColumns, vectorColumnInfo{
			table: r.Table, column: r.Column, typeDecl: r.TypeDecl,
		})
	}
	return nil
}

// takeSeedSnapshot reads the current seed rows of ai_settings/embedding_config
// from the golden schema into package-level slices. MUST be called immediately
// after runTestMigrations, before any test mutates those tables. Content comes
// from the DB at runtime — there is no hardcoded seed copy in testutil.
func takeSeedSnapshot(db *gorm.DB) error {
	seedSnapshotMu.Lock()
	defer seedSnapshotMu.Unlock()

	aiSettingsSeed = nil
	embeddingCfgSeed = nil
	if err := db.Find(&aiSettingsSeed).Error; err != nil {
		return fmt.Errorf("snapshot ai_settings: %w", err)
	}
	if err := db.Find(&embeddingCfgSeed).Error; err != nil {
		return fmt.Errorf("snapshot embedding_config: %w", err)
	}
	return nil
}

// ResetTestData resets the golden schema to the "fresh production startup"
// state for the next test. It:
//  1. TRUNCATEs all business tables (RESTART IDENTITY CASCADE, skipping
//     schema_migrations) — clears data and resets serial sequences.
//  2. Re-ALTERs every vector-typed column back to its golden-build type
//     declaration — undoes dimension mutations tests made via ALTER COLUMN.
//  3. Restores migration seed rows (ai_settings, embedding_config) from the
//     golden-schema snapshot, then advances their sequences past the restored
//     explicit-id rows.
//  4. Reopens the connection pool — fresh prepared statements against the
//     stable catalog (the golden schema never drops the vector extension, so
//     the vector type OID is stable; reopening only clears plans invalidated
//     by step 2's ALTERs, exactly mirroring what a production restart does).
//
// Returns the fresh *gorm.DB (the caller MUST use the returned handle; the
// passed db's pool is closed). TruncateAllTables (no restore, no reopen) is
// retained for callers that deliberately want a truly empty DB.
func ResetTestData(t *testing.T, db *gorm.DB) *gorm.DB {
	t.Helper()

	seedSnapshotMu.Lock()
	defer seedSnapshotMu.Unlock()

	if aiSettingsSeed == nil || embeddingCfgSeed == nil {
		t.Fatal("ResetTestData: seed snapshot not taken; golden schema must be built first")
	}

	// 1. Truncate all base tables except schema_migrations.
	var tables []string
	if err := db.Raw(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		AND table_name <> 'schema_migrations'`).Scan(&tables).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(tables) > 0 {
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE",
			strings.Join(tables, ", "))).Error; err != nil {
			t.Fatalf("truncate tables: %v", err)
		}
	}

	// 2. Restore vector column dimensions to the golden-build declarations
	//    (undoes test mutations like ALTER COLUMN embedding TYPE vector(N)).
	//    Tables are empty after the truncate, so the USING cast is trivial.
	for _, vc := range goldenVectorColumns {
		if err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s",
			vc.table, vc.column, vc.typeDecl, vc.column, vc.typeDecl)).Error; err != nil {
			t.Fatalf("restore vector column %s.%s to %s: %v", vc.table, vc.column, vc.typeDecl, err)
		}
	}

	// 3. Restore migration seed rows from the golden-schema snapshot.
	if len(aiSettingsSeed) > 0 {
		if err := db.Create(&aiSettingsSeed).Error; err != nil {
			t.Fatalf("restore ai_settings seed: %v", err)
		}
	}
	if len(embeddingCfgSeed) > 0 {
		if err := db.Create(&embeddingCfgSeed).Error; err != nil {
			t.Fatalf("restore embedding_config seed: %v", err)
		}
	}
	// Advance serial sequences past the restored explicit-id seed rows.
	for _, tbl := range []string{"ai_settings", "embedding_config"} {
		if err := db.Exec(fmt.Sprintf(
			"SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE((SELECT MAX(id) FROM %s), 0))",
			tbl, tbl)).Error; err != nil {
			t.Fatalf("advance %s id sequence: %v", tbl, err)
		}
	}

	// 4. Reopen the connection pool. Steps above (especially the ALTERs) can
	//    invalidate server-side prepared statements cached on the current pool
	//    (cached plan must not change result type). A fresh pool gives clean
	//    statements against the stable catalog. Mirrors ReimportTestDB's reopen.
	freshDB, err := openGorm(cachedDSN)
	if err != nil {
		t.Fatalf("reopen test database after reset: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
	cachedDB = freshDB
	database.DB = freshDB
	return freshDB
}

// TestEmbeddingDim is the vector dimension enforced by the production schema
// (semantic_labels.embedding/merge_embedding and topic_tag_embeddings.embedding
// are vector(4096)).
const TestEmbeddingDim = 4096

// PadVector zero-pads vec to dim. Integration tests construct small semantic
// vectors (e.g. []float64{1,0,0}) for deterministic cosine geometry, but the
// Postgres vector(4096) column rejects shorter vectors. PadVector bridges the
// gap without obscuring the test's geometric intent at call sites. The padding
// zeros do not change cosine similarity between two padded vectors.
func PadVector(vec []float64, dim int) []float64 {
	if len(vec) >= dim {
		return vec
	}
	out := make([]float64, dim)
	copy(out, vec)
	return out
}

// runTestMigrations mirrors the production database initialization path
// (database.InitDB): the same two phases production runs on every startup —
//   - Phase 1: database.RunAutoMigrate (sync all model tables/columns)
//   - Phase 2: database.RunMigrations (versioned migrations — the source of
//     truth for FKs/indexes/triggers and for the ai_settings/embedding_config
//     seed rows tests rely on).
//
// This is intentionally a verbatim copy of production: the testcontainer schema
// and seed data are exactly what production serves. Earlier revisions duplicated
// parts of this by hand (manual embedding column DDL, manual ai_settings /
// embedding_config seeds) — those duplicated production and drifted, so they
// were removed in favor of running the real migration path.
//
// t enables destructive migrations (MIGRATIONS_ALLOW_DESTRUCTIVE=1) so the test
// golden schema matches production + historical data cleanup. Production never
// sets this env (destructive migrations self-skip); see db-migration-safety.
func runTestMigrations(t *testing.T, db *gorm.DB) error {
	t.Setenv("MIGRATIONS_ALLOW_DESTRUCTIVE", "1")
	atomic.AddInt64(&migrationsRunCount, 1)
	// Enable pgvector extension (mirrors the first production migration).
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return fmt.Errorf("enable pgvector: %w", err)
	}

	// Phase 1: AutoMigrate — same as database.InitDB phase 1.
	if err := database.RunAutoMigrate(db); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	// Phase 2: versioned migrations — same as database.InitDB phase 2. This is
	// the source of truth for FKs/indexes/triggers and the seed data (semantic
	// board / event cluster config) that production and tests both depend on.
	if err := database.RunMigrations(db); err != nil {
		return fmt.Errorf("run versioned migrations: %w", err)
	}

	return nil
}
