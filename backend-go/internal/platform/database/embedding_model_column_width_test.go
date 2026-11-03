package database_test

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	discmodels "syntopica-backend/internal/discovery/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

// 回归护栏：存 embedding 模型名的列宽 MUST NOT 窄于模型注册表 ai_models.name
// ——模型名在注册表里放得下，在任何落库处就必须放得下
// （fix-embedding-model-column-width design D1）。
//
// 背景：模型名取自 ai_models.name（VARCHAR(100)），而五张 embedding 表的
// model 列停在 VARCHAR(50)。本地 llama.cpp embedding 模型注册时「名字」直接
// 填 GGUF 文件绝对路径（61 字符）→ 超限 11 字符 → PostgreSQL 拒绝 INSERT
// （SQLSTATE 22001）→ 整条 embedding 任务失败。缓存命中时返回的 cached.Model
// 是同一个值，所以主路由是远端短名模型也挡不住。

// longEmbeddingModelName 是本地 llama.cpp embedding 模型的「名字」——注册时
// 直接填 GGUF 文件绝对路径（61 字符）。
const longEmbeddingModelName = "/home/zanebono/.syntopica/models/Qwen3-Embedding-4B-Q6_K.gguf"

// shortEmbeddingModelName 是远端 embedding 模型的短名——迁移前的存量行形态
// （≤50 字符，旧列宽装得下的那种）。
const shortEmbeddingModelName = "qwen3-embedding"

// boundaryEmbeddingModelName 是**恰好 50 字符**（旧列宽 VARCHAR(50) 的上限）
// 的存量值——贴着旧边界。旧的用例用 15 字符短名，对截断完全不敏感（任何
// 截到 ≤50 的实现都发现不了）；贴着边界的值才能让「会截断的实现」露馅。
var boundaryEmbeddingModelName = strings.Repeat("m", 50)

// overBoundaryEmbeddingModelName 是 55 字符的值——落在 (50, 100) 区间里。
// 「USING left(model,50)」这类截断实现只有对这种值才会露馅：迁移前它装不进
// VARCHAR(50)，但装得进仍窄于目标宽度的 VARCHAR(60)，而后者仍会触发加宽
// ALTER——于是截断实现会把它砍到 50 字符，被本用例逐字节断言抓住。
var overBoundaryEmbeddingModelName = strings.Repeat("w", 55)

// embeddingModelWidthTables 列出存 embedding 模型名的五张表——与迁移
// 20261103_0001 的辖区一致（design D2：一次修五张，不只修触发报错的那张）。
//
// 这份清单是「测试覆盖清单」；迁移自己的清单由 export_test.go 导出，两者必须
// 相等——否则「迁移改了表、测试没覆盖」这种漂移就没人拦了。
var embeddingModelWidthTables = []string{
	"topic_tag_embeddings",
	"candidate_embeddings",
	"preference_vectors",
	"route_embeddings",
	"discovery_interest_entries",
}

// embeddingModelRowSeed 描述「在某张表里造一行 model=… 的向量行」：seed 插入
// 一行并返回回找该行的 (where, arg)。probeID 用于生成互不冲突的业务键
// （候选/路由的唯一索引都参与，偏好向量的 (board_id, source) 也参与）。
// vectorText 非空时把该向量写进 embedding 列（空串 = 向量列留 NULL）。
type embeddingModelRowSeed struct {
	table string
	seed  func(t *testing.T, db *gorm.DB, modelName string, probeID int, vectorText string) (where string, arg any)
}

// embeddingModelRowSeeds 是五张表各自的造行方式。向量列默认留 NULL——大部分用例
// 守的是「model 列宽」这一条契约，不该为了塞向量去构造几千维大向量；
// 只有守「向量本身原样保留」的那一行才带真实向量。
var embeddingModelRowSeeds = []embeddingModelRowSeed{
	{
		table: "topic_tag_embeddings",
		seed: func(t *testing.T, db *gorm.DB, modelName string, probeID int, vectorText string) (string, any) {
			slug := fmt.Sprintf("embwidth-probe-tag-%d", probeID)
			require.NoError(t, db.Exec(`INSERT INTO topic_tags (slug, label, category, status, created_at, updated_at)
				VALUES (?, 'embwidth probe tag', 'keyword', 'active', now(), now())`, slug).Error)
			var tagID uint
			require.NoError(t, db.Raw(`SELECT id FROM topic_tags WHERE slug = ?`, slug).Row().Scan(&tagID))
			hash := fmt.Sprintf("embwidth-probe-hash-%d", probeID)
			if vectorText != "" {
				dim := embeddingVectorDimension(t, db)
				require.NoError(t, db.Exec(`INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, embedding, dimension, model, text_hash, created_at, updated_at)
					VALUES (?, 'identity', ?::vector, ?, ?, ?, now(), now())`, tagID, vectorText, dim, modelName, hash).Error)
				return `text_hash = ?`, hash
			}
			require.NoError(t, db.Exec(`INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, dimension, model, text_hash, created_at, updated_at)
				VALUES (?, 'identity', 2560, ?, ?, now(), now())`, tagID, modelName, hash).Error)
			return `text_hash = ?`, hash
		},
	},
	{
		table: "candidate_embeddings",
		seed: func(t *testing.T, db *gorm.DB, modelName string, probeID int, _ string) (string, any) {
			candidateID := 900000 + probeID
			require.NoError(t, db.Exec(`INSERT INTO candidate_embeddings (candidate_id, model, dimension, text_hash, created_at, updated_at)
				VALUES (?, ?, 2560, ?, now(), now())`, candidateID, modelName, fmt.Sprintf("embwidth-probe-hash-%d", probeID)).Error)
			return `candidate_id = ?`, candidateID
		},
	},
	{
		table: "preference_vectors",
		seed: func(t *testing.T, db *gorm.DB, modelName string, probeID int, _ string) (string, any) {
			source := fmt.Sprintf("embwidth-probe-src-%d", probeID)
			require.NoError(t, db.Exec(`INSERT INTO preference_vectors (board_id, source, dimension, model, last_computed_at, created_at, updated_at)
				VALUES (NULL, ?, 2560, ?, now(), now(), now())`, source, modelName).Error)
			return `source = ?`, source
		},
	},
	{
		table: "route_embeddings",
		seed: func(t *testing.T, db *gorm.DB, modelName string, probeID int, _ string) (string, any) {
			routeID := 900000 + probeID
			require.NoError(t, db.Exec(`INSERT INTO route_embeddings (route_id, dimension, model, text_hash, created_at, updated_at)
				VALUES (?, 2560, ?, ?, now(), now())`, routeID, modelName, fmt.Sprintf("embwidth-probe-hash-%d", probeID)).Error)
			return `route_id = ?`, routeID
		},
	},
	{
		table: "discovery_interest_entries",
		seed: func(t *testing.T, db *gorm.DB, modelName string, probeID int, _ string) (string, any) {
			query := fmt.Sprintf("embwidth-probe-query-%d", probeID)
			require.NoError(t, db.Exec(`INSERT INTO discovery_interest_entries (run_id, query_text, dimension, model, created_at, updated_at)
				VALUES (NULL, ?, 2560, ?, now(), now())`, query, modelName).Error)
			return `query_text = ?`, query
		},
	},
}

// findEmbeddingModelColumnWidthMigration 定位迁移 20261103_0001 的 Up 闭包。
func findEmbeddingModelColumnWidthMigration(t *testing.T) func(*gorm.DB) error {
	t.Helper()
	return findMigrationUp(t, "20261103_0001")
}

// findEmbeddingModelNotNullMigration 定位迁移 20261103_0002 的 Up 闭包。
func findEmbeddingModelNotNullMigration(t *testing.T) func(*gorm.DB) error {
	t.Helper()
	return findMigrationUp(t, "20261103_0002")
}

// findMigrationUp 按版本号从生产迁移清单里取 Up 闭包。
func findMigrationUp(t *testing.T, version string) func(*gorm.DB) error {
	t.Helper()
	for _, m := range database.ExportedPostgresMigrations() {
		if m.Version == version {
			return m.Up
		}
	}
	t.Fatalf("migration %s must be registered", version)
	return nil
}

// embeddingColumnWidth 读 information_schema 里某列的 character_maximum_length
// （非 varchar 列返回 -1，便于断言信息一眼定位）。
func embeddingColumnWidth(t *testing.T, db *gorm.DB, table, column string) int {
	t.Helper()
	var width int
	require.NoError(t, db.Raw(`SELECT COALESCE(character_maximum_length, -1) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`, table, column).Row().Scan(&width),
		"读取 %s.%s 的 character_maximum_length 失败", table, column)
	return width
}

// embeddingColumnIsNullable 读某列是否可空（is_nullable = 'YES'）。
func embeddingColumnIsNullable(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()
	var nullable string
	require.NoError(t, db.Raw(`SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`, table, column).Row().Scan(&nullable),
		"读取 %s.%s 的 is_nullable 失败", table, column)
	return nullable == "YES"
}

// embeddingModelColumnsInDB 列出**全库**所有 column_name='model' 且有长度上限
// 的列（character_maximum_length IS NOT NULL）——数据驱动的不变式输入：将来
// 谁新增一张 xxx.model VARCHAR(50)，这个集合里就会多出一行窄列，用例即红。
func embeddingModelColumnsInDB(t *testing.T, db *gorm.DB) map[string]int {
	t.Helper()
	rows, err := db.Raw(`SELECT table_name, character_maximum_length FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name = 'model' AND character_maximum_length IS NOT NULL
		ORDER BY table_name`).Rows()
	require.NoError(t, err, "读全库 model 列清单失败")
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var table string
		var width int
		require.NoError(t, rows.Scan(&table, &width), "扫描 model 列行失败")
		out[table] = width
	}
	require.NoError(t, rows.Err(), "遍历全库 model 列清单失败")
	return out
}

// embeddingTableRowCount 统计表行数（迁移前后比对存量行是否丢失）。
func embeddingTableRowCount(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM `+table).Scan(&n).Error, "统计 %s 行数失败", table)
	return n
}

// gormFieldSize 读模型定义里某字段的 gorm size tag——「gorm tag 与 DDL 一致」
// 的事实源只能是模型定义本身（DDL 是它的产物）。
func gormFieldSize(t *testing.T, db *gorm.DB, model any, field string) int {
	t.Helper()
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(model), "解析模型 %T 失败", model)
	f := stmt.Schema.LookUpField(field)
	require.NotNil(t, f, "模型 %T 没有字段 %s", model, field)
	raw, ok := f.TagSettings["SIZE"]
	require.True(t, ok, "模型 %T 的字段 %s 缺 size tag", model, field)
	size, err := strconv.Atoi(raw)
	require.NoError(t, err, "模型 %T 的字段 %s 的 size tag 不是整数：%q", model, field, raw)
	return size
}

// embeddingVectorDimension 读 topic_tag_embeddings.embedding 的实际维度
// （pgvector 类型串 "vector(2560)" → 2560；"vector"（未限定维度）→ 0）。
// 运行时读、不写死：黄金 schema 是 vector(4096)，生产是 vector(2560)。
func embeddingVectorDimension(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var typeStr string
	require.NoError(t, db.Raw(`
SELECT format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relname = 'topic_tag_embeddings' AND a.attname = 'embedding'
`).Row().Scan(&typeStr), "读 topic_tag_embeddings.embedding 的列类型失败")
	if dim := parseVectorDim(typeStr); dim > 0 {
		return dim
	}
	// 列未限定维度：halfvec 索引表达式会约束插入向量的维度，按索引里的维度走。
	var indexDef string
	if err := db.Raw(`SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = ?`,
		"idx_topic_tag_embeddings_identity_halfvec_hnsw").Row().Scan(&indexDef); err == nil {
		if dim := parseHalfvecDim(indexDef); dim > 0 {
			return dim
		}
	}
	return testutil.TestEmbeddingDim
}

// parseVectorDim 从 pgvector 类型串解析维度："vector(2560)" → 2560；
// "vector"（未限定）/ 非法 → 0。
func parseVectorDim(typeStr string) int {
	s := strings.TrimSpace(typeStr)
	const prefix = "vector("
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, ")") {
		return 0
	}
	dim, err := strconv.Atoi(strings.TrimSuffix(s[len(prefix):], ")"))
	if err != nil || dim <= 0 {
		return 0
	}
	return dim
}

// parseHalfvecDim 从索引定义里解析 halfvec(N) 的维度。
func parseHalfvecDim(indexDef string) int {
	const marker = "halfvec("
	i := strings.Index(indexDef, marker)
	if i < 0 {
		return 0
	}
	rest := indexDef[i+len(marker):]
	j := strings.Index(rest, ")")
	if j <= 0 {
		return 0
	}
	dim, err := strconv.Atoi(rest[:j])
	if err != nil || dim <= 0 {
		return 0
	}
	return dim
}

// pgVectorText 把 float64 向量格式化成 pgvector 的文本表示 "[0.1,0.2,...]"。
func pgVectorText(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(v, 'g', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// indexOID 读某索引的 OID。索引被重建时 OID 会变（PG 实测：对已是
// varchar(100) 的列再执行 ALTER COLUMN model TYPE VARCHAR(100)，表体不重写
// 但该列上的索引会被重建 → 索引 OID 变化）——用它断言「迁移没有白重建索引」。
func indexOID(t *testing.T, db *gorm.DB, indexName string) string {
	t.Helper()
	var oid string
	require.NoError(t, db.Raw(`SELECT c.oid::text FROM pg_class c
		WHERE c.relname = ? AND c.relkind = 'i'`, indexName).Row().Scan(&oid),
		"读索引 %s 的 OID 失败", indexName)
	return oid
}

// cleanupEmbeddingModelProbe 复位共享的进程级 testcontainer：删掉本用例造的
// 探针行（不做 DROP/TRUNCATE——不动别的用例的数据），并把 model 列宽拉回
// 100（迁移后的状态），让后续用例看到的必须是迁移后的 schema。
func cleanupEmbeddingModelProbe(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, stmt := range []string{
		`DELETE FROM topic_tag_embeddings WHERE text_hash LIKE 'embwidth-probe-hash-%'`,
		`DELETE FROM topic_tags WHERE slug LIKE 'embwidth-probe-%'`, // 级联删 topic_tag_embeddings
		`DELETE FROM candidate_embeddings WHERE candidate_id BETWEEN 900000 AND 900099`,
		`DELETE FROM preference_vectors WHERE source LIKE 'embwidth-probe-src-%'`,
		`DELETE FROM route_embeddings WHERE route_id BETWEEN 900000 AND 900099`,
		`DELETE FROM discovery_interest_entries WHERE query_text LIKE 'embwidth-probe-query-%'`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Logf("清理探针行失败（%v）：%s", err, stmt)
		}
	}
	for _, table := range embeddingModelWidthTables {
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ALTER COLUMN model TYPE VARCHAR(100)", table)).Error; err != nil {
			t.Errorf("复位 %s.model 到 VARCHAR(100) 失败：%v", table, err)
			continue
		}
		assert.Equal(t, 100, embeddingColumnWidth(t, db, table, "model"),
			"复位后 %s.model 的列宽必须是 100（迁移后的状态）", table)
	}
}

// TestEmbeddingModelColumnWidthMigration 跑迁移 20261103_0001 的完整链路：
// 制造「迁移前」状态（窄列 VARCHAR(50)）→ 复现长模型名写不进去（SQLSTATE
// 22001）→ 插入存量行（短名 + 恰好 50 字符的边界值 + 一行真实向量）→ 跑迁移
// → 列宽变 100、存量行/向量逐字节原样保留、长模型名写得进去 → 重复执行幂等
// 无错、且不会重建索引。
//
// 覆盖场景：长模型名写入成功（候选/偏好/路由/兴趣条目四表各一行）、列宽不窄于
// 模型注册表、既有向量不受影响、加宽不截断。
//
// Docker required. Skipped under -short.
func TestEmbeddingModelColumnWidthMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	up := findEmbeddingModelColumnWidthMigration(t)

	require.Greater(t, len(longEmbeddingModelName), 50,
		"长模型名必须超过旧列宽 50，否则测不出 22001 超限")
	require.LessOrEqual(t, len(longEmbeddingModelName), 100,
		"长模型名必须放得进新列宽 100")
	require.Len(t, boundaryEmbeddingModelName, 50,
		"边界存量值必须恰好 50 字符（贴着旧列宽上限）")
	require.Len(t, overBoundaryEmbeddingModelName, 55,
		"越界存量值必须 55 字符（>50 且 ≤100）")

	// 复位：共享 testcontainer 上只动自己造的探针行与列宽，不 DROP/TRUNCATE。
	t.Cleanup(func() { cleanupEmbeddingModelProbe(t, db) })

	// ① 制造「迁移前」状态：五张表的 model 列回窄到 VARCHAR(50)。
	for _, table := range embeddingModelWidthTables {
		require.NoError(t, db.Exec(fmt.Sprintf("ALTER TABLE %s ALTER COLUMN model TYPE VARCHAR(50)", table)).Error,
			"回窄 %s.model 到 VARCHAR(50)（造迁移前状态）失败", table)
		require.Equal(t, 50, embeddingColumnWidth(t, db, table, "model"),
			"%s.model 应处于迁移前的窄列状态", table)
	}

	// ② 迁移前：长模型名写不进去——复现生产的 SQLSTATE 22001
	//    （topic_tag_embeddings 是本 change 的触发表，用它最贴真）。
	require.NoError(t, db.Exec(`INSERT INTO topic_tags (slug, label, category, status, created_at, updated_at)
		VALUES ('embwidth-probe-tag-fail', 'embwidth probe tag', 'keyword', 'active', now(), now())`).Error)
	var failTagID uint
	require.NoError(t, db.Raw(`SELECT id FROM topic_tags WHERE slug = 'embwidth-probe-tag-fail'`).Row().Scan(&failTagID))
	err := db.Exec(`INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, dimension, model, text_hash, created_at, updated_at)
		VALUES (?, 'identity', 2560, ?, 'embwidth-probe-hash-fail', now(), now())`, failTagID, longEmbeddingModelName).Error
	require.Error(t, err, "迁移前：61 字符的模型名必须被 varchar(50) 拒绝（生产报的就是这个错）")
	assert.Contains(t, err.Error(), "character varying(50)",
		"迁移前的报错应是列宽超限：%v", err)

	// ③ 存量行：五张表各插两行「迁移前」世界里能存在的行——
	//    短名（≤50）与恰好 50 字符的边界值（贴着旧列宽上限）。
	//    topic_tag_embeddings 另插一行带**真实向量**的存量行（守「向量本身
	//    原样保留」——加宽不得动向量）。
	vectorDim := embeddingVectorDimension(t, db)
	vectorText := pgVectorText(testutil.PadVector([]float64{0.1, 0.2, 0.3}, vectorDim))
	type seededRow struct {
		table string
		where string
		arg   any
		model string
	}
	var pre []seededRow
	for _, spec := range embeddingModelRowSeeds {
		for i, modelName := range []string{shortEmbeddingModelName, boundaryEmbeddingModelName} {
			// probeID 1 = 短名，probeID 2 = 边界值：业务键必须互不相同
			// （route_embeddings 的唯一键是 UNIQUE(route_id) 单列）。
			where, arg := spec.seed(t, db, modelName, i+1, "")
			pre = append(pre, seededRow{table: spec.table, where: where, arg: arg, model: modelName})
		}
	}
	vectorWhere, vectorArg := embeddingModelRowSeeds[0].seed(t, db, boundaryEmbeddingModelName, 5, vectorText)
	var vectorBefore sql.NullString
	require.NoError(t, db.Raw(`SELECT embedding::text FROM topic_tag_embeddings WHERE `+vectorWhere, vectorArg).Row().Scan(&vectorBefore),
		"读迁移前的向量文本失败")

	before := map[string]int64{}
	for _, table := range embeddingModelWidthTables {
		before[table] = embeddingTableRowCount(t, db, table)
	}

	// ④ 跑迁移（事务内调用，与生产路径一致：生产迁移默认走 in-tx）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"迁移 20261103_0001 在存量有行的窄列表上必须成功")

	// ⑤ 五张表的 model 列宽 = 100。
	for _, table := range embeddingModelWidthTables {
		assert.Equal(t, 100, embeddingColumnWidth(t, db, table, "model"),
			"%s.model 的列宽必须是 100（与 ai_models.name 对齐）", table)
	}

	// ⑥ 存量行原样保留、行数不变（加宽不截断、不丢行）。
	for _, table := range embeddingModelWidthTables {
		assert.Equal(t, before[table], embeddingTableRowCount(t, db, table),
			"%s 的行数不得因迁移变化", table)
	}
	for _, row := range pre {
		var model string
		require.NoError(t, db.Raw(`SELECT model FROM `+row.table+` WHERE `+row.where, row.arg).Row().Scan(&model),
			"回读 %s 的存量行失败", row.table)
		assert.Equal(t, row.model, model,
			"%s 的存量行 model 值必须逐字节原样保留（加宽不得截断）", row.table)
	}

	// ⑥-b 向量本身原样保留：迁移前后的 embedding::text 逐字节相等。
	var vectorAfter sql.NullString
	require.NoError(t, db.Raw(`SELECT embedding::text FROM topic_tag_embeddings WHERE `+vectorWhere, vectorArg).Row().Scan(&vectorAfter),
		"读迁移后的向量文本失败")
	require.True(t, vectorAfter.Valid, "带真实向量的存量行在迁移后不得丢向量")
	assert.Equal(t, vectorBefore.String, vectorAfter.String,
		"topic_tag_embeddings 的存量向量必须逐字节原样保留（加宽不得动向量）")

	// ⑦ 鉴别场景：造一个「装不进旧宽度、但装得进仍窄于目标宽度的列」的值
	//    （55 字符 + 列宽收到 VARCHAR(60)），再跑一次迁移——此时条件判定会
	//    命中「需要加宽」而真的 ALTER。「USING left(model,50)」这类截断实现
	//    会把 55 字符砍成 50，本断言即红；正确实现必须原样保留。
	routeSeed := embeddingModelRowSeeds[3]
	where, arg := routeSeed.seed(t, db, overBoundaryEmbeddingModelName, 4, "")
	require.NoError(t, db.Exec(`ALTER TABLE route_embeddings ALTER COLUMN model TYPE VARCHAR(60)`).Error,
		"把 route_embeddings.model 收到 VARCHAR(60)（仍窄于目标宽度）失败")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"迁移 20261103_0001 在「仍窄于目标宽度」的列上必须成功")
	assert.Equal(t, 100, embeddingColumnWidth(t, db, "route_embeddings", "model"),
		"route_embeddings.model 加宽后必须是 100")
	var model55 string
	require.NoError(t, db.Raw(`SELECT model FROM route_embeddings WHERE `+where, arg).Row().Scan(&model55),
		"回读 route_embeddings 的 55 字符存量行失败")
	assert.Equal(t, overBoundaryEmbeddingModelName, model55,
		"55 字符的存量值必须逐字节原样保留（截断实现会把它砍到 50 —— 必然红）")
	// 同一轮里另外四张表已是目标宽度：迁移必须完全不动它们。
	for _, table := range []string{"topic_tag_embeddings", "candidate_embeddings", "preference_vectors", "discovery_interest_entries"} {
		assert.Equal(t, 100, embeddingColumnWidth(t, db, table, "model"),
			"%s.model 已是目标宽度，重跑迁移后必须仍是 100", table)
	}

	// ⑧ 迁移后：61 字符的长模型名写得进去——本次修的正是这个。
	for _, spec := range embeddingModelRowSeeds {
		where, arg := spec.seed(t, db, longEmbeddingModelName, 3, "")
		var model string
		require.NoError(t, db.Raw(`SELECT model FROM `+spec.table+` WHERE `+where, arg).Row().Scan(&model),
			"回读 %s 的长模型名行失败", spec.table)
		assert.Equal(t, longEmbeddingModelName, model,
			"%s 必须能存下 61 字符的模型名（本次修复的正是这个）", spec.table)
	}

	// ⑨ 幂等：列宽已达目标时重跑是**完全 no-op**——不报错、列宽不变、
	//    61 字符的值不被改写、该列上的索引不被重建（索引 OID 不变）。
	oidBefore := indexOID(t, db, "idx_candidate_embedding_uniq")
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"重复执行迁移必须无错（幂等）")
	for _, table := range embeddingModelWidthTables {
		assert.Equal(t, 100, embeddingColumnWidth(t, db, table, "model"),
			"%s.model 重复执行后列宽仍是 100", table)
	}
	var longAfter string
	require.NoError(t, db.Raw(`SELECT model FROM topic_tag_embeddings WHERE text_hash = ?`, "embwidth-probe-hash-3").Row().Scan(&longAfter),
		"回读 topic_tag_embeddings 的长模型名行失败")
	assert.Equal(t, longEmbeddingModelName, longAfter,
		"61 字符的模型名在重复执行迁移后必须逐字节不变（截断实现会露馅）")
	assert.Equal(t, oidBefore, indexOID(t, db, "idx_candidate_embedding_uniq"),
		"列宽已是目标时重跑迁移不得重建该列上的索引（索引 OID 不得变化）")
}

// TestEmbeddingModelColumnWidthNoopWhenAtTargetWidth 守住 20261103_0001 的
// 「真 no-op」：列宽已达目标时迁移**完全不碰表**——不 ALTER、不取锁。
// 探针：另开一条连接持有 candidate_embeddings 的 ACCESS EXCLUSIVE 锁。
// 正确实现只读 catalog（information_schema / to_regclass 不锁用户表）→
// 立刻返回；无条件 ALTER 的实现会卡在这把锁上，被 lock_timeout(5s) 打断
// 而报错——这条用例即红。
//
// Docker required. Skipped under -short.
func TestEmbeddingModelColumnWidthNoopWhenAtTargetWidth(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	up := findEmbeddingModelColumnWidthMigration(t)

	t.Cleanup(func() { cleanupEmbeddingModelProbe(t, db) })

	// 黄金 schema 的状态：列宽已经是目标宽度。
	require.Equal(t, 100, embeddingColumnWidth(t, db, "candidate_embeddings", "model"),
		"candidate_embeddings.model 应当已是目标宽度 100")

	// 探针锁：另一条连接（连接池里的另一条连接）独占 candidate_embeddings。
	lockTx := db.Begin()
	require.NoError(t, lockTx.Error, "开第二条连接失败")
	require.NoError(t, lockTx.Exec(`LOCK TABLE candidate_embeddings IN ACCESS EXCLUSIVE MODE`).Error,
		"持有 candidate_embeddings 的 ACCESS EXCLUSIVE 锁失败")
	defer func() { _ = lockTx.Rollback().Error }()

	// 列宽已达目标 → 迁移不得去 ALTER 这张表（否则会卡在上面的锁上）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"列宽已是目标宽度时迁移必须完全不碰表（不取锁、不 ALTER）")
}

// TestEmbeddingModelColumnWidthNotNullMigration 守住 topic_tag_embeddings.model
// 的 NOT NULL 约束：20260723_0001 用显式迁移把它固化成 NOT NULL，本 change 的上一版实现
// 却因为 gorm tag 改宽触发 AutoMigrate 的 ALTER COLUMN，被 gorm 的 postgres
// driver 顺带 DROP NOT NULL 弄丢了（生产库已是可空）。20261103_0002 负责恢复。
//
// Docker required. Skipped under -short.
func TestEmbeddingModelColumnWidthNotNullMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)
	up := findEmbeddingModelNotNullMigration(t)

	// 复原保险：无论本用例成败，共享 schema 都必须回到「NOT NULL 由显式迁移
	// 固化」的黄金状态（否则后面的 constraints 用例会红），探针行也不能留。
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM topic_tag_embeddings WHERE text_hash = 'embwidth-probe-hash-notnull'`,
			`DELETE FROM topic_tags WHERE slug = 'embwidth-probe-tag-notnull'`,
			`UPDATE topic_tag_embeddings SET model = '' WHERE model IS NULL`,
		} {
			if err := db.Exec(stmt).Error; err != nil {
				t.Errorf("清理探针行失败（%v）：%s", err, stmt)
			}
		}
		if err := db.Exec(`ALTER TABLE topic_tag_embeddings ALTER COLUMN model SET NOT NULL`).Error; err != nil {
			t.Errorf("复原 topic_tag_embeddings.model 的 NOT NULL 失败：%v", err)
		}
	})

	// ① 复现事故现场：AutoMigrate 的 ALTER COLUMN 把 NOT NULL 静默丢了。
	require.NoError(t, db.Exec(`ALTER TABLE topic_tag_embeddings ALTER COLUMN model DROP NOT NULL`).Error,
		"模拟 AutoMigrate 丢掉 NOT NULL 失败")
	require.True(t, embeddingColumnIsNullable(t, db, "topic_tag_embeddings", "model"),
		"造出的事故现场必须是可空状态")

	// ② 造一行 model 为 NULL 的存量行（迁移必须能自愈：回填 + 补约束）。
	require.NoError(t, db.Exec(`INSERT INTO topic_tags (slug, label, category, status, created_at, updated_at)
		VALUES ('embwidth-probe-tag-notnull', 'embwidth probe tag', 'keyword', 'active', now(), now())`).Error)
	var tagID uint
	require.NoError(t, db.Raw(`SELECT id FROM topic_tags WHERE slug = 'embwidth-probe-tag-notnull'`).Row().Scan(&tagID))
	require.NoError(t, db.Exec(`INSERT INTO topic_tag_embeddings (topic_tag_id, embedding_type, dimension, model, text_hash, created_at, updated_at)
		VALUES (?, 'identity', 2560, NULL, 'embwidth-probe-hash-notnull', now(), now())`, tagID).Error)

	// ③ 跑迁移 20261103_0002。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"迁移 20261103_0002 必须成功")

	// ④ NOT NULL 恢复。
	assert.False(t, embeddingColumnIsNullable(t, db, "topic_tag_embeddings", "model"),
		"迁移后 topic_tag_embeddings.model 必须恢复 NOT NULL")

	// ⑤ NULL 行被回填（不是丢行）。
	var model sql.NullString
	require.NoError(t, db.Raw(`SELECT model FROM topic_tag_embeddings WHERE text_hash = 'embwidth-probe-hash-notnull'`).Row().Scan(&model),
		"回读被回填的行失败")
	require.True(t, model.Valid, "迁移后该行的 model 不得仍是 NULL")
	assert.Equal(t, "", model.String, "NULL model 必须被回填成空串（与 20260723_0001 同口径）")

	// ⑥ 幂等：重复执行无错、约束仍在。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return up(tx) }),
		"重复执行迁移 20261103_0002 必须无错（幂等）")
	assert.False(t, embeddingColumnIsNullable(t, db, "topic_tag_embeddings", "model"),
		"重复执行后 topic_tag_embeddings.model 仍必须是 NOT NULL")
}

// TestEmbeddingModelColumnWidthInvariant 守住列宽不变式：存 embedding 模型名的
// 列宽 MUST NOT 窄于模型注册表 ai_models.name 的列宽。断言是**数据驱动**的
// ——扫全库所有 column_name='model' 的列逐张比对，将来谁把列改窄、或新增一张
// 窄列的 embedding 表，这个用例即红。
//
// 同时断言 GORM 模型定义的 size tag 与 DDL 实际列宽一致——tag 与 DDL 漂移
// 意味着下次启动 AutoMigrate 会把列改回窄列，等于没修（design D3）。
//
// Docker required. Skipped under -short.
func TestEmbeddingModelColumnWidthInvariant(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires testcontainer Postgres")
	}
	db := testutil.SetupTestDB(t)

	registryWidth := embeddingColumnWidth(t, db, "ai_models", "name")
	require.Positive(t, registryWidth, "ai_models.name 必须有列宽（模型注册表是列宽的事实源）")

	// ① 测试覆盖清单 MUST 等于迁移清单——防「迁移改了表、测试没覆盖」的漂移。
	migrationTables := database.ExportedEmbeddingModelNameTables()
	require.ElementsMatch(t, migrationTables, embeddingModelWidthTables,
		"测试覆盖清单必须与迁移清单一致（export_test.go 导出的是迁移自己的清单）")

	// ② 全库所有 model 列（有长度上限的）宽度 ≥ 注册表 name 列宽（数据驱动）。
	all := embeddingModelColumnsInDB(t, db)
	require.NotEmpty(t, all, "全库应当存在 model 列，不变式才有意义")
	for table, width := range all {
		assert.GreaterOrEqual(t, width, registryWidth,
			"%s.model 的列宽(%d) 不得窄于 ai_models.name 的列宽(%d)", table, width, registryWidth)
	}

	// ③ 迁移管辖的每张表都必须落在全库 model 列集合内。
	for _, table := range migrationTables {
		width, ok := all[table]
		require.True(t, ok, "迁移管辖的 %s 必须出现在全库 model 列集合内（实际集合：%v）", table, all)
		assert.GreaterOrEqual(t, width, registryWidth,
			"%s.model 的列宽(%d) 不得窄于 ai_models.name 的列宽(%d)", table, width, registryWidth)
	}

	// ④ GORM 模型定义与 DDL 一致：声明的 size 必须等于 DB 实际列宽。
	for _, c := range []struct {
		table string
		model any
	}{
		{"topic_tag_embeddings", &tagmodels.TopicTagEmbedding{}},
		{"candidate_embeddings", &discmodels.CandidateEmbedding{}},
		{"preference_vectors", &discmodels.PreferenceVector{}},
		{"route_embeddings", &discmodels.RouteEmbedding{}},
		{"discovery_interest_entries", &discmodels.DiscoveryInterestEntry{}},
	} {
		declared := gormFieldSize(t, db, c.model, "Model")
		actual := embeddingColumnWidth(t, db, c.table, "model")
		assert.Equal(t, declared, actual,
			"%s：gorm tag 声明 size=%d 与 DDL 实际列宽(%d) 不一致（漂移会让 AutoMigrate 把列改回窄列）",
			c.table, declared, actual)
	}
}
