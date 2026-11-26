package core

// unify-vector-column-dimension / embedding-dimension-contract 的维度契约用例。
//
// 覆盖 spec `embedding-dimension-contract` 的六个 Scenario：
//
//	维度一致时写入成功 / 维度不一致时拒绝写入 / 维度变化后索引重建 /
//	超过索引维度上限时跳过并记日志 / 维度取自模型实际输出 / 配置表不持有维度键
//
// 全部跑在 testcontainer 隔离库内（🛑 不碰真库）：建索引 / ALTER 列类型都只发生
// 在一次性容器里，测试结束清理。

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// seedDimensionTag 建一条最小可用的 topic_tags 行并返回其 id
// （topic_tag_embeddings.topic_tag_id 有 FK，必须指向真实标签）。
func seedDimensionTag(t *testing.T, db *gorm.DB, slug string) uint {
	t.Helper()
	var id uint
	require.NoError(t, db.Raw(`
		INSERT INTO topic_tags (slug, label, category, aliases, description, created_at, updated_at)
		VALUES (?, ?, 'keyword', '[]', '', now(), now()) RETURNING id`, slug, slug).Scan(&id).Error)
	require.NotZero(t, id)
	return id
}

// dropProductionHalfvecIndex 清理生产 halfvec identity 索引——它带维度约束
// （表达式内嵌维度），留着会让后续用例写入不同维度的向量时撞上
// "expected N dimensions, not M"。
func dropProductionHalfvecIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	t.Cleanup(func() {
		_ = db.Exec("DROP INDEX IF EXISTS " + database.TopicTagIdentityHalfvecIndexName).Error
	})
}

// TestEmbeddingDimensionConsistentWriteSucceeds 覆盖场景「维度一致时写入成功」：
// 向量实际维度与随值留存的 Dimension 字段一致时，写入成功且落库的维度与向量
// 实际维度相同。
func TestEmbeddingDimensionConsistentWriteSucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)
	dropProductionHalfvecIndex(t, db)
	service := NewEmbeddingService()

	tagID := seedDimensionTag(t, db, "dim-consistent-tag")
	const dim = 8
	require.NoError(t, service.SaveEmbedding(&tagmodels.TopicTagEmbedding{
		TopicTagID:    tagID,
		EmbeddingType: EmbeddingTypeIdentity,
		EmbeddingVec:  makeValidVector(dim),
		Dimension:     dim,
		Model:         "test-model",
		TextHash:      "dim-consistent",
	}))

	var stored tagmodels.TopicTagEmbedding
	require.NoError(t, db.Where("topic_tag_id = ? AND embedding_type = ?", tagID, EmbeddingTypeIdentity).
		First(&stored).Error)
	assert.Equal(t, dim, stored.Dimension, "落库的维度必须等于随值留存的维度字段")

	var actualDim int
	require.NoError(t, db.Raw("SELECT vector_dims(embedding) FROM topic_tag_embeddings WHERE id = ?", stored.ID).
		Scan(&actualDim).Error)
	assert.Equal(t, dim, actualDim, "落库向量的实际维度必须与声明维度一致")
}

// TestEmbeddingDimensionMismatchRejected 覆盖场景「维度不一致时拒绝写入」：
// 向量实际维度与随值留存的 Dimension 字段不一致时，本次写入被拒绝（错误信息
// 同时带期望维度与实际维度），且不落库。
func TestEmbeddingDimensionMismatchRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)
	dropProductionHalfvecIndex(t, db)
	service := NewEmbeddingService()

	tagID := seedDimensionTag(t, db, "dim-mismatch-tag")
	const (
		declaredDim = 16
		actualDim   = 8
	)
	err := service.SaveEmbedding(&tagmodels.TopicTagEmbedding{
		TopicTagID:    tagID,
		EmbeddingType: EmbeddingTypeIdentity,
		EmbeddingVec:  makeValidVector(actualDim),
		Dimension:     declaredDim,
		Model:         "test-model",
		TextHash:      "dim-mismatch",
	})
	require.Error(t, err, "维度不一致的向量必须被拒绝写入")
	assert.Contains(t, err.Error(), "dimension mismatch")
	assert.Contains(t, err.Error(), "16", "错误信息必须带期望维度")
	assert.Contains(t, err.Error(), "8", "错误信息必须带实际维度")

	var count int64
	require.NoError(t, db.Model(&tagmodels.TopicTagEmbedding{}).
		Where("topic_tag_id = ?", tagID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "被拒绝的写入不得落库")
}

// TestEmbeddingDimensionComesFromModelOutput 覆盖场景「维度取自模型实际输出」：
// 落库的维度等于向量本身的实际维度（模型输出的长度），并且**配置表里的维度键
// 不能覆盖它**——这里刻意塞一条 dimension=999 的配置行，维度仍必须跟着模型输出
// 走。
func TestEmbeddingDimensionComesFromModelOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)
	dropProductionHalfvecIndex(t, db)
	service := NewEmbeddingService()

	// 敌意配置：配置表里出现维度键（真库已被迁移 20260614_0001 删除）。
	require.NoError(t, db.Exec(`
		INSERT INTO embedding_config (key, value, description, created_at, updated_at)
		VALUES ('embedding_dimension', '999', 'hostile dimension key', now(), now())
		ON CONFLICT (key) DO UPDATE SET value = '999'`).Error)

	tagID := seedDimensionTag(t, db, "dim-from-model-tag")
	const modelDim = 8
	require.NoError(t, service.SaveEmbedding(&tagmodels.TopicTagEmbedding{
		TopicTagID:    tagID,
		EmbeddingType: EmbeddingTypeIdentity,
		EmbeddingVec:  makeValidVector(modelDim),
		Dimension:     modelDim,
		Model:         "test-model",
		TextHash:      "dim-from-model",
	}))

	var stored tagmodels.TopicTagEmbedding
	require.NoError(t, db.Where("topic_tag_id = ? AND embedding_type = ?", tagID, EmbeddingTypeIdentity).
		First(&stored).Error)
	assert.Equal(t, modelDim, stored.Dimension, "维度必须取自模型输出，不得被配置表覆盖")

	var actualDim int
	require.NoError(t, db.Raw("SELECT vector_dims(embedding) FROM topic_tag_embeddings WHERE id = ?", stored.ID).
		Scan(&actualDim).Error)
	assert.Equal(t, modelDim, actualDim)
}

// TestEmbeddingConfigHoldsNoDimensionKey 覆盖场景「配置表不持有维度键」：
// 迁移链跑完后配置表里不得有指定向量维度的键；且向量列的类型声明里也不得携带
// 维度约束（schema 不承载维度）。
func TestEmbeddingConfigHoldsNoDimensionKey(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)

	var cfgRows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM embedding_config WHERE key = 'embedding_dimension'`).
		Scan(&cfgRows).Error)
	assert.Equal(t, int64(0), cfgRows, "配置表不得持有向量维度键（维度是运行时属性）")

	var typedColumns []string
	require.NoError(t, db.Raw(`
		SELECT c.table_name || '.' || c.column_name
		FROM information_schema.columns c
		JOIN pg_catalog.pg_class cls ON cls.relname = c.table_name
		JOIN pg_catalog.pg_namespace ns ON ns.oid = cls.relnamespace AND ns.nspname = 'public'
		JOIN pg_catalog.pg_attribute a ON a.attrelid = cls.oid AND a.attname = c.column_name
		WHERE c.table_schema = 'public' AND c.udt_name = 'vector' AND a.atttypmod <> -1
	`).Scan(&typedColumns).Error)
	assert.Empty(t, typedColumns, "向量列的类型声明不得携带维度约束（atttypmod 必须为 -1）")
}

// TestVectorIndexRebuiltOnDimensionChange 覆盖场景「维度变化后索引重建」：
// halfvec identity 索引的表达式内嵌维度必须跟着运行时维度走——维度变了就重建
// （CREATE INDEX IF NOT EXISTS 不会纠正既有表达式）。
func TestVectorIndexRebuiltOnDimensionChange(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)
	dropProductionHalfvecIndex(t, db)

	// 索引是 identity 部分索引：先清空 identity 行，任意维度的索引都能建。
	require.NoError(t, db.Exec("DELETE FROM topic_tag_embeddings").Error)

	require.NoError(t, ensureVectorDimension(16))
	assert.Equal(t, 16, database.TopicTagIdentityHalfvecIndexDimension(db),
		"索引表达式内嵌的维度必须等于运行时维度")

	// 运行时维度变了 → 索引必须重建为新维度。
	require.NoError(t, ensureVectorDimension(32))
	assert.Equal(t, 32, database.TopicTagIdentityHalfvecIndexDimension(db),
		"维度变化后索引必须重建为新维度")

	var valid bool
	require.NoError(t, db.Raw(`
		SELECT i.indisvalid FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = ?`, database.TopicTagIdentityHalfvecIndexName).Scan(&valid).Error)
	assert.True(t, valid, "重建后的索引必须有效")
}

// TestVectorIndexSkippedAboveHalfvecLimit 覆盖场景「超过索引维度上限时跳过并记日志」：
// 运行时维度超过 halfvec 上限（4000）时跳过建索引（语义召回回退精确扫描），
// 且不留下一条会把列钉在旧维度上的旧索引。
func TestVectorIndexSkippedAboveHalfvecLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)
	dropProductionHalfvecIndex(t, db)

	require.NoError(t, db.Exec("DELETE FROM topic_tag_embeddings").Error)
	require.NoError(t, ensureVectorDimension(5000))

	exists, err := database.VectorIndexExists(db, database.TopicTagIdentityHalfvecIndexName)
	require.NoError(t, err)
	assert.False(t, exists, "超过 halfvec 上限时不得建索引（也不得留下旧维度的索引）")
}

// TestVectorDimensionColumnStaysDimensionless 覆盖 design D3：列已是无维度限定
// 的 vector 时，运行期纠正器**不得**把列改回带维度限定的类型（否则分叉源
// 从 DDL 搬回运行时，等于原地复发）。
func TestVectorDimensionColumnStaysDimensionless(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := setupEmbeddingTestDB(t)
	dropProductionHalfvecIndex(t, db)

	var typeStr string
	require.NoError(t, db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'topic_tag_embeddings' AND a.attname = 'embedding'`).Row().Scan(&typeStr))
	require.Equal(t, "vector", typeStr, "前置条件：列已是无维度限定的 vector")

	require.NoError(t, ensureVectorDimension(64))

	require.NoError(t, db.Raw(`
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = 'topic_tag_embeddings' AND a.attname = 'embedding'`).Row().Scan(&typeStr))
	assert.Equal(t, "vector", typeStr, "无维度限定的列不得被运行期纠正器改回 vector(N)")
}
