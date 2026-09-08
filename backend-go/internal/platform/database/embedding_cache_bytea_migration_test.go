package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// Integration tests for the pre-AutoMigrate converter (optimize-pg-storage):
// ai_embedding_cache.embedding jsonb → bytea. Runs the real production
// startup order against a legacy schema (legacy jsonb table → RunAutoMigrate
// → RunMigrations) — this also empirically guards that AutoMigrate does not
// fail startup on the jsonb/bytea mismatch. Throwaway testcontainer, Docker
// required, skipped under -short.
//
// The conversion is non-destructive (legacy rows are re-encoded, not
// truncated), so no MIGRATIONS_ALLOW_DESTRUCTIVE gate is involved.

// setupLegacyCacheDB creates the pre-change jsonb table with legacy rows,
// then runs the production startup migration sequence. The destructive gate
// is left CLOSED (production default) on purpose: the converter must not
// depend on it.
func setupLegacyCacheDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenTestDB(t)

	// The testcontainer is a process-wide singleton shared by every test in
	// the package: drop leftovers from a previous test so CREATE TABLE is
	// deterministic.
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS ai_embedding_cache").Error)

	require.NoError(t, db.Exec(`CREATE TABLE ai_embedding_cache (
		cache_key varchar(64) PRIMARY KEY,
		model varchar(100),
		operation varchar(80),
		embedding jsonb,
		dimensions int,
		input_preview varchar(200),
		created_at timestamptz
	)`).Error)
	// Two convertible rows (float32-exact values so assertions stay strict)
	// plus one NULL row that must survive as NULL.
	require.NoError(t, db.Exec(`INSERT INTO ai_embedding_cache (cache_key, model, operation, embedding, dimensions, input_preview, created_at)
		VALUES ('legacy-1', 'm', 'tagmanagement.embedding', '[[0.5,0.75]]', 2, 'x', now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO ai_embedding_cache (cache_key, model, operation, embedding, dimensions, input_preview, created_at)
		VALUES ('legacy-2', 'm', 'tagmanagement.embedding', '[[0.25,0.5],[1.5,-0.5]]', 2, 'y', now())`).Error)
	require.NoError(t, db.Exec(`INSERT INTO ai_embedding_cache (cache_key, model, operation, embedding, dimensions, input_preview, created_at)
		VALUES ('legacy-null', 'm', 'tagmanagement.embedding', NULL, 2, 'z', now())`).Error)

	require.NoError(t, db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error)
	require.NoError(t, database.RunAutoMigrate(db))
	require.NoError(t, database.RunMigrations(db))
	return db
}

func cacheEmbeddingType(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var dataType string
	require.NoError(t, db.Raw(`SELECT a.atttypid::regtype::text
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'ai_embedding_cache' AND a.attname = 'embedding'`).Scan(&dataType).Error)
	return dataType
}

// TestEmbeddingCacheByteaConversionPreservesRows: legacy jsonb rows survive
// the column switch, re-encoded to the float32 LE binary codec.
func TestEmbeddingCacheByteaConversionPreservesRows(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: needs Docker testcontainer")
	}
	db := setupLegacyCacheDB(t)

	require.Equal(t, "bytea", cacheEmbeddingType(t, db))

	var rows []models.AIEmbeddingCache
	require.NoError(t, db.Order("cache_key").Find(&rows).Error)
	require.Len(t, rows, 3)

	require.Equal(t, "legacy-1", rows[0].CacheKey)
	vectors, err := models.DecodeEmbeddingVectors(rows[0].Embedding)
	require.NoError(t, err)
	require.Equal(t, [][]float64{{0.5, 0.75}}, vectors)

	require.Equal(t, "legacy-2", rows[1].CacheKey)
	vectors, err = models.DecodeEmbeddingVectors(rows[1].Embedding)
	require.NoError(t, err)
	require.Equal(t, [][]float64{{0.25, 0.5}, {1.5, -0.5}}, vectors)

	require.Equal(t, "legacy-null", rows[2].CacheKey)
	require.Nil(t, rows[2].Embedding)
}

// TestEmbeddingCacheByteaConversionIdempotent: re-running the full startup
// migration sequence on an already-converted DB is a no-op.
func TestEmbeddingCacheByteaConversionIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: needs Docker testcontainer")
	}
	db := setupLegacyCacheDB(t)

	require.NoError(t, database.RunAutoMigrate(db))
	require.NoError(t, database.RunMigrations(db))

	require.Equal(t, "bytea", cacheEmbeddingType(t, db))
	var count int64
	require.NoError(t, db.Raw("SELECT count(*) FROM ai_embedding_cache").Scan(&count).Error)
	require.Equal(t, int64(3), count)

	// Converted payload still decodes after the second pass.
	var rec models.AIEmbeddingCache
	require.NoError(t, db.First(&rec, "cache_key = ?", "legacy-1").Error)
	vectors, err := models.DecodeEmbeddingVectors(rec.Embedding)
	require.NoError(t, err)
	require.Equal(t, [][]float64{{0.5, 0.75}}, vectors)
}
