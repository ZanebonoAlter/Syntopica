package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// 本文件覆盖 db-migration-safety 的「版本迁移闭包 SHALL 幂等可重复执行」要求，
// 重点是 fix-disaster-recovery-channel 追加的向量列约束：
//
//	涉及向量列类型（vector / halfvec）的迁移 SHALL NOT 把维度硬编码进目标类型，
//	目标类型 SHALL 为无维度限定的 vector。
//
// 为什么这条约束与灾备恢复直接相关：备份档恢复时，若档内没有 schema_migrations，
// 整条版本迁移链会在**有数据的库**上从零重放（DDL 落在存量数据上）。迁移
// 20260403_0003 此前把目标类型写死成 vector(4096)，而现网存量列是 vector(2560)
// ——重放即维度冲突，恢复直接失败。改成无维度限定的 vector 后，去掉 typmod 是
// 放宽而非收紧约束，任意维度的存量数据都能安全 ALTER。
//
// 所有用例名统一以 TestVectorDimension 开头：门禁用
// `go test ./internal/platform/database/... -run VectorDimension` 跑本文件，
// 命名保持一致可让该命令覆盖本文件映射到的全部场景。

// vectorColumnType 返回列的实际类型声明（format_type 带 typmod，
// 例如 "vector" / "vector(2560)"）。
func vectorColumnType(t *testing.T, db *gorm.DB, table, column string) string {
	t.Helper()
	var typ string
	require.NoError(t, db.Raw(`
		SELECT pg_catalog.format_type(a.atttypid, a.atttypmod)
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = ? AND a.attname = ?
		  AND a.attnum > 0 AND NOT a.attisdropped`, table, column).Scan(&typ).Error)
	require.NotEmpty(t, typ, "column %s.%s not found", table, column)
	return typ
}

// columnIsNullable 报告列是否可空（attnotnull = false 即可空）。
func columnIsNullable(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var notNull bool
	require.NoError(t, db.Raw(`
		SELECT a.attnotnull
		FROM pg_catalog.pg_attribute a
		JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relname = ? AND a.attname = ?
		  AND a.attnum > 0 AND NOT a.attisdropped`, table, column).Scan(&notNull).Error)
	return !notNull
}

// seedTopicTag 建一条最小可用的 topic_tags 行并返回其 id
// （topic_tag_embeddings.topic_tag_id 有 FK，必须指向真实标签）。
func seedTopicTag(t *testing.T, db *gorm.DB, slug string) uint {
	t.Helper()
	var id uint
	require.NoError(t, db.Raw(`
		INSERT INTO topic_tags (slug, label, category, aliases, description, created_at, updated_at)
		VALUES (?, ?, 'keyword', '[]', '', now(), now()) RETURNING id`, slug, slug).Scan(&id).Error)
	require.NotZero(t, id, "topic_tags insert must return an id")
	return id
}

// TestVectorDimensionMigrationPreservesStoredDimension 覆盖场景
// 「向量列维度迁移不因存量维度不同而失败」：
// 在一个向量列已含非空数据、且其维度与迁移目标不同的库上重放迁移，
// 迁移必须成功，列类型变成无维度限定的 vector，存量数据维度保持不变。
func TestVectorDimensionMigrationPreservesStoredDimension(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	// 造「存量维度 ≠ 目标」的形态：列类型 vector(2560) + 非空 2560 维数据。
	require.NoError(t, db.Exec(`DELETE FROM topic_tag_embeddings`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(2560)`).Error)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM topic_tag_embeddings`)
		db.Exec(`ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector`)
	})

	const storedDim = 2560
	tagID := seedTopicTag(t, db, "vd-probe-tag-1")
	require.NoError(t, db.Exec(`
		INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, embedding, dimension, model, text_hash, created_at, updated_at)
		VALUES (?, 'identity', array_fill(0.1::real, ARRAY[2560])::vector, 2560, 'probe-model', 'probe-hash-1', now(), now()),
		       (?, 'identity', array_fill(0.2::real, ARRAY[2560])::vector, 2560, 'probe-model', 'probe-hash-2', now(), now())`,
		tagID, tagID).Error)

	var beforeDims []int
	require.NoError(t, db.Raw(`SELECT vector_dims(embedding) FROM topic_tag_embeddings ORDER BY id`).Scan(&beforeDims).Error)
	require.Len(t, beforeDims, 2)
	require.Equal(t, storedDim, beforeDims[0])

	up := findMigrationUp(t, "20260403_0003")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"维度 ≠ 目标时重放向量列类型迁移必须成功（目标类型无维度限定）")

	// 列类型：无维度限定（不再是 vector(N)）
	assert.Equal(t, "vector", vectorColumnType(t, db, "topic_tag_embeddings", "embedding"),
		"迁移后列类型必须是无维度限定的 vector")

	// 存量数据：维度不变、行数不变
	var afterDims []int
	require.NoError(t, db.Raw(`SELECT vector_dims(embedding) FROM topic_tag_embeddings ORDER BY id`).Scan(&afterDims).Error)
	assert.Equal(t, beforeDims, afterDims, "存量向量维度必须保持不变")
	var rows int64
	require.NoError(t, db.Table("topic_tag_embeddings").Count(&rows).Error)
	assert.Equal(t, int64(2), rows, "存量行数必须保持不变")
}

// TestVectorDimensionMigrationReplayKeepsUndimensionedColumnType 覆盖场景
// 「向量列类型迁移重复执行不改变列类型」：
// 列已经是无维度限定的 vector 时重放迁移，迁移成功且列类型不变。
func TestVectorDimensionMigrationReplayKeepsUndimensionedColumnType(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	before := vectorColumnType(t, db, "topic_tag_embeddings", "embedding")
	require.Equal(t, "vector", before, "前置条件：列已是无维度限定的 vector")

	up := findMigrationUp(t, "20260403_0003")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"重复执行向量列类型迁移必须幂等成功")

	assert.Equal(t, before, vectorColumnType(t, db, "topic_tag_embeddings", "embedding"),
		"重复执行不得改变列类型")
}

// TestVectorDimensionMigrationSetNotNullReplayIsIdempotent 覆盖场景
// 「SET NOT NULL 迁移重复执行不报错」：
// ai_call_logs.operation 列已是 NOT NULL 时重复执行 20260704_0001 的 SET NOT NULL 逻辑，
// 不报错（幂等返回成功）。
func TestVectorDimensionMigrationSetNotNullReplayIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	up := findMigrationUp(t, "20260704_0001")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
	require.False(t, columnIsNullable(t, db, "ai_call_logs", "operation"),
		"前置条件：ai_call_logs.operation 已是 NOT NULL")

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"对已 NOT NULL 的列重复执行 SET NOT NULL 逻辑必须不报错")
	assert.False(t, columnIsNullable(t, db, "ai_call_logs", "operation"))
}

// TestVectorDimensionMigrationConstrainRespectsNotNullFalse 覆盖场景
// 「constrain helper 尊重 notNull=false 声明」：
// 重放 20260723_0001 时，cols 声明 notNull=false 的列（semantic_labels.context_layers / aliases）
// 只被 SET DEFAULT + 回填，不得被 SET NOT NULL。
func TestVectorDimensionMigrationConstrainRespectsNotNullFalse(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	up := findMigrationUp(t, "20260723_0001")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))

	for _, column := range []string{"context_layers", "aliases"} {
		assert.True(t, columnIsNullable(t, db, "semantic_labels", column),
			"semantic_labels.%s 声明 notNull=false，重放后必须保持可空", column)
	}
}

// TestVectorDimensionMigrationChainReplayOnPopulatedDBIsIdempotent 覆盖场景
// 「重复执行 InitDB 不产生迁移错误」，并直接对应灾备场景：
// 备份档里没有 schema_migrations 时，恢复后整条版本迁移链会在**有数据的库**上从零重放。
// 这里清空迁移历史后重跑全量迁移链，断言所有闭包都成功（不因非幂等 DDL 报错）。
func TestVectorDimensionMigrationChainReplayOnPopulatedDBIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	// 造「有数据的库」：向量列存量数据 + 迁移历史被清空（模拟历史备份档无 schema_migrations）。
	require.NoError(t, db.Exec(`DELETE FROM topic_tag_embeddings`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE topic_tag_embeddings ALTER COLUMN embedding TYPE vector(2560)`).Error)
	tagID := seedTopicTag(t, db, "vd-probe-tag-2")
	require.NoError(t, db.Exec(`
		INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, embedding, dimension, model, text_hash, created_at, updated_at)
		VALUES (?, 'identity', array_fill(0.3::real, ARRAY[2560])::vector, 2560, 'probe-model', 'probe-hash-3', now(), now())`,
		tagID).Error)
	require.NoError(t, db.Exec(`DELETE FROM schema_migrations`).Error)

	// 重放整条迁移链（AutoMigrate + 全量版本迁移 = 生产 InitDB 的两个阶段）。
	require.NoError(t, database.RunAutoMigrate(db))
	require.NoError(t, database.RunMigrations(db),
		"迁移历史缺失时整条版本迁移链必须在有数据的库上重放成功")

	// 重放后：列类型被收敛到无维度限定，存量数据维度不变。
	assert.Equal(t, "vector", vectorColumnType(t, db, "topic_tag_embeddings", "embedding"))
	var dims []int
	require.NoError(t, db.Raw(`SELECT vector_dims(embedding) FROM topic_tag_embeddings ORDER BY id`).Scan(&dims).Error)
	require.NotEmpty(t, dims)
	for _, d := range dims {
		assert.Equal(t, 2560, d, "重放后存量向量维度必须保持不变")
	}

	// 复原测试容器 schema（重放会改表结构：DROP 过期的表、加约束），避免污染同进程其它用例。
	t.Cleanup(func() {
		testutil.ReimportTestDB(t, db)
	})
}
