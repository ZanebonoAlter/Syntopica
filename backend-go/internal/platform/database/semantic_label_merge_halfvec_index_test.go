package database_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
)

// fix-aux-label-dedup-ann tasks 1.2 / 1.3 — semantic_labels.merge_embedding 的
// halfvec 部分 HNSW 索引：迁移（空表跳过 / 有数据建成 / 幂等）与启动期守卫
// （不存在 / 一致 / 不一致 / invalid / 超上限）。

// seedAuxMergeVector 插入一行 active auxiliary 标签，merge_embedding 为 dim 维
// 常量向量（裸 vector 列不限维度）。
func seedAuxMergeVector(t *testing.T, db *gorm.DB, slug string, dim int) uint {
	t.Helper()
	vec := "[" + strings.TrimSuffix(strings.Repeat("0.1,", dim), ",") + "]"
	row := models.SemanticLabel{
		Label: slug, Slug: slug, LabelType: "auxiliary", Status: "active",
		Embedding: &vec, MergeEmbedding: &vec,
	}
	require.NoError(t, db.Create(&row).Error)
	return row.ID
}

func mergeHalfvecIndexOID(t *testing.T, db *gorm.DB) (oid int64, exists bool) {
	t.Helper()
	// 索引不存在时 to_regclass 为 NULL → COALESCE 落 0。
	require.NoError(t, db.Raw(`SELECT COALESCE(CAST(to_regclass('public.`+database.SemanticLabelMergeHalfvecIndexName+`') AS int), 0)`).Scan(&oid).Error)
	return oid, oid != 0
}

func dropMergeHalfvecIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`DROP INDEX IF EXISTS `+database.SemanticLabelMergeHalfvecIndexName).Error)
}

// TestSemanticLabelMergeHalfvecIndexMigration：空表（采样不到维度）跳过；有存量
// active auxiliary 向量则按采样维度建索引（非谓词行的维度不参与）；重复执行幂等
// （IF NOT EXISTS，不重建、oid 不变）。
func TestSemanticLabelMergeHalfvecIndexMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	require.NoError(t, db.Exec(`DELETE FROM semantic_labels`).Error)
	dropMergeHalfvecIndex(t, db)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM semantic_labels`)
		db.Exec(`DROP INDEX IF EXISTS ` + database.SemanticLabelMergeHalfvecIndexName)
	})

	up := findMigrationUp(t, "20261124_0002")

	// 表空 → 采样不到维度 → 跳过（不猜维度，等启动期守卫按运行时维度建）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
	_, exists := mergeHalfvecIndexOID(t, db)
	assert.False(t, exists, "空表时不得猜维度建索引")

	// 有存量数据 → 从谓词行采样维度 → 建索引；非谓词行（board，别的维度）不参与采样。
	boardVec := "[" + strings.TrimSuffix(strings.Repeat("0.2,", 32), ",") + "]"
	require.NoError(t, db.Create(&models.SemanticLabel{
		Label: "看板", Slug: "board-probe", LabelType: "board", Status: "active", MergeEmbedding: &boardVec,
	}).Error)
	seedAuxMergeVector(t, db, "vd-aux-merge", 64)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
	oid, exists := mergeHalfvecIndexOID(t, db)
	require.True(t, exists, "有 active auxiliary 向量时必须建索引")
	assert.Equal(t, 64, database.SemanticLabelMergeHalfvecIndexDimension(db), "裸 vector 列的索引维度必须取自谓词行存量数据")
	dim, valid := database.SemanticLabelMergeHalfvecIndexState(db)
	assert.True(t, valid, "新建的 halfvec 索引必须有效")

	// 重复执行 → 幂等：不重建（oid 不变）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }))
	oidAgain, _ := mergeHalfvecIndexOID(t, db)
	assert.Equal(t, oid, oidAgain, "重复执行迁移不得重建已存在且维度一致的索引")
	assert.Equal(t, dim, database.SemanticLabelMergeHalfvecIndexDimension(db))
}

// TestEnsureSemanticLabelMergeHalfvecIndex：启动期守卫四条路径 + 超上限跳过。
func TestEnsureSemanticLabelMergeHalfvecIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	require.NoError(t, db.Exec(`DELETE FROM semantic_labels`).Error)
	dropMergeHalfvecIndex(t, db)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM semantic_labels`)
		db.Exec(`DROP INDEX IF EXISTS ` + database.SemanticLabelMergeHalfvecIndexName)
	})
	seedAuxMergeVector(t, db, "guard-aux-merge", 64)

	// 不存在 → 创建（表上有 64 维行，索引按 64 建）。
	require.NoError(t, database.EnsureSemanticLabelMergeHalfvecIndex(db, 64))
	oidCreated, exists := mergeHalfvecIndexOID(t, db)
	require.True(t, exists, "守卫必须创建缺失的索引")
	assert.Equal(t, 64, database.SemanticLabelMergeHalfvecIndexDimension(db))

	// 已存在且维度一致 → 不动（oid 不变）。
	require.NoError(t, database.EnsureSemanticLabelMergeHalfvecIndex(db, 64))
	oidSame, _ := mergeHalfvecIndexOID(t, db)
	assert.Equal(t, oidCreated, oidSame, "维度一致时不得重建索引")

	// 维度不一致 → 按新维度重建（oid 变化）。
	// 必须先清掉旧维度行：pgvector 在写入时即校验表达式索引的 typmod，旧维度索引
	// 存在时根本不接受新维度行的变更（部署侧顺序同此：守卫先重建索引，再写入新维度
	// 嵌入）——这也正是 design D2「列维度过渡不得被旧索引卡死」的原因。
	require.NoError(t, db.Exec(`DELETE FROM semantic_labels`).Error)
	require.NoError(t, database.EnsureSemanticLabelMergeHalfvecIndex(db, 32))
	oidRebuilt, _ := mergeHalfvecIndexOID(t, db)
	assert.NotEqual(t, oidCreated, oidRebuilt, "运行时维度变化后必须重建索引")
	assert.Equal(t, 32, database.SemanticLabelMergeHalfvecIndexDimension(db))

	// invalid（历史构建失败残留）→ 重建为有效索引（重建后新维度行可写入）。
	require.NoError(t, db.Exec(`UPDATE pg_index SET indisvalid = false, indisready = false
		WHERE indexrelid = to_regclass('public.`+database.SemanticLabelMergeHalfvecIndexName+`')`).Error)
	require.NoError(t, database.EnsureSemanticLabelMergeHalfvecIndex(db, 32))
	oidHealed, _ := mergeHalfvecIndexOID(t, db)
	assert.NotEqual(t, oidRebuilt, oidHealed, "invalid 索引必须被重建")
	_, valid := database.SemanticLabelMergeHalfvecIndexState(db)
	assert.True(t, valid, "重建后的索引必须有效")
	seedAuxMergeVector(t, db, "guard-aux-merge-32", 32)

	// dim 超出 halfvec 上限 → 跳过（不报错、不动既有索引）。
	require.NoError(t, database.EnsureSemanticLabelMergeHalfvecIndex(db, 4096))
	oidSkipped, _ := mergeHalfvecIndexOID(t, db)
	assert.Equal(t, oidHealed, oidSkipped, "超上限时不得动既有索引")
}

// TestEnsureSemanticLabelMergeHalfvecIndexStaleDimension：存量旧维度行未清理时，
// 守卫的索引重建必然失败（cast 报 22P02）——同一原因也让召回整条报错、整批 attach
// 失败。守卫因此返回 error（调用方只 WARN、不阻断启动），并已在日志里给出清理 SQL
// 与 backfill 端点指引（review R-2）。
func TestEnsureSemanticLabelMergeHalfvecIndexStaleDimension(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	require.NoError(t, db.Exec(`DELETE FROM semantic_labels`).Error)
	dropMergeHalfvecIndex(t, db)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM semantic_labels`)
		db.Exec(`DROP INDEX IF EXISTS ` + database.SemanticLabelMergeHalfvecIndexName)
	})
	// 存量行是 64 维，运行时维度是 32：索引表达式 merge_embedding::halfvec(32)
	// 在这一行上必然报错。
	seedAuxMergeVector(t, db, "stale-aux-merge", 64)

	err := database.EnsureSemanticLabelMergeHalfvecIndex(db, 32)
	require.Error(t, err, "旧维度行未清理时索引重建必须失败（召回同样会报 22P02），不得静默成功")
	_, exists := mergeHalfvecIndexOID(t, db)
	assert.False(t, exists, "重建失败时不得留下半成品索引")

	// 清理旧维度行后重建成功（部署处置顺序：先清旧向量，再让守卫建索引）。
	require.NoError(t, db.Exec(`UPDATE semantic_labels SET merge_embedding = NULL WHERE id = (SELECT id FROM semantic_labels LIMIT 1)`).Error)
	seedAuxMergeVector(t, db, "fresh-aux-merge", 32)
	require.NoError(t, database.EnsureSemanticLabelMergeHalfvecIndex(db, 32))
	oid, exists := mergeHalfvecIndexOID(t, db)
	require.True(t, exists, "旧维度行清理后必须能建成索引")
	assert.Equal(t, 32, database.SemanticLabelMergeHalfvecIndexDimension(db))
	assert.NotZero(t, oid)
}

// 清理 SQL 常量必须与文档口径一致（运维照着执行）。
func TestSemanticLabelMergeDimensionCleanupSQL(t *testing.T) {
	sql := fmt.Sprintf(database.SemanticLabelMergeDimensionCleanupSQL, 2560)
	assert.Contains(t, sql, "UPDATE semantic_labels SET merge_embedding = NULL")
	assert.Contains(t, sql, "label_type='auxiliary'")
	assert.Contains(t, sql, "status='active'")
	assert.Contains(t, sql, "vector_dims(merge_embedding) <> 2560")
}
