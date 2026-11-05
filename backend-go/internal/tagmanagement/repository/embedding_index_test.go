package repository

// speed-up-tag-embedding 任务 1.1（test-cases.md 步 1）：镜像
// scripts/db/tag-embedding-halfvec-hnsw-20261104.sql 的 DDL 在 testcontainer
// 隔离库执行——三类 embedding_type 各得一个 valid 的 partial halfvec HNSW
// 索引；DROP INDEX IF EXISTS tmp_hnsw_test 清理坏索引尸体的语句合法且幂等
//（真库上由用户在低负载窗口手动执行，见 change 红线）。

import (
	"fmt"
	"testing"

	"syntopica-backend/internal/platform/testutil"
)

func TestTagEmbeddingHalfvecIndexesDDL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	InitRepository(db)

	dim := 2560
	types := []string{"semantic", "identity", "event_keyword"}
	for _, typ := range types {
		name := fmt.Sprintf("idx_tte_%s_halfvec_hnsw_ddl_test", typ)
		ddl := fmt.Sprintf(
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON topic_tag_embeddings USING hnsw ((embedding::halfvec(%d)) halfvec_cosine_ops) WHERE embedding_type = '%s' AND embedding IS NOT NULL",
			name, dim, typ)
		// CREATE INDEX CONCURRENTLY 不能在事务里跑：用 Connection 的自动提交路径。
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("create %s index: %v", typ, err)
		}
		t.Cleanup(func() {
			_ = db.Exec("DROP INDEX IF EXISTS " + name).Error
		})
	}
	// 清理坏索引尸体的语句（SQL 文件同款）必须可执行且幂等。
	if err := db.Exec("DROP INDEX CONCURRENTLY IF EXISTS tmp_hnsw_test").Error; err != nil {
		t.Fatalf("drop tmp_hnsw_test: %v", err)
	}

	var count int
	if err := db.Raw(`
		SELECT count(*)
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		WHERE i.indrelid = to_regclass('public.topic_tag_embeddings')
		  AND c.relname LIKE '%halfvec_hnsw_ddl_test'
		  AND i.indisvalid`).Scan(&count).Error; err != nil {
		t.Fatalf("probe index validity: %v", err)
	}
	if count != 3 {
		t.Fatalf("valid halfvec HNSW indexes = %d, want 3", count)
	}

	var ghost int
	if err := db.Raw(`
		SELECT count(*) FROM pg_class WHERE relname = 'tmp_hnsw_test'`).Scan(&ghost).Error; err != nil {
		t.Fatalf("probe tmp_hnsw_test: %v", err)
	}
	if ghost != 0 {
		t.Fatalf("tmp_hnsw_test still present after cleanup DDL")
	}
}
