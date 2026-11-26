package database

// fix-aux-label-dedup-ann tasks 1.1 — semantic_labels merge halfvec 部分索引的
// DDL 单一事实源单测（无 DB，`-short` 下也跑）：索引名、表达式（typmod 带维度）、
// opclass、WHERE 谓词。迁移与启动期守卫由 testcontainer 用例覆盖。

import (
	"strconv"
	"strings"
	"testing"
)

func TestSemanticLabelMergeHalfvecIndexDDL(t *testing.T) {
	for _, dim := range []int{2560, 4000} {
		ddl := SemanticLabelMergeHalfvecIndexDDL(dim)
		mustContain := []string{
			"CREATE INDEX IF NOT EXISTS " + SemanticLabelMergeHalfvecIndexName,
			"ON semantic_labels USING hnsw",
			"((merge_embedding::halfvec(" + strconv.Itoa(dim) + ")) halfvec_cosine_ops)",
			"WHERE label_type = 'auxiliary' AND status = 'active' AND merge_embedding IS NOT NULL",
		}
		for _, want := range mustContain {
			if !strings.Contains(ddl, want) {
				t.Fatalf("DDL(dim=%d) missing %q\ngot: %s", dim, want, ddl)
			}
		}
	}

	if name := SemanticLabelMergeHalfvecIndexName; name != "idx_semantic_labels_merge_halfvec_hnsw" {
		t.Fatalf("index name = %q（改名会让部署回滚脚本与文档失效）", name)
	}
	// 谓词是 DDL 与 L2 召回查询共用的单一事实源（逐字一致部分索引才生效）。
	if SemanticLabelMergeHalfvecIndexPredicate != "label_type = 'auxiliary' AND status = 'active' AND merge_embedding IS NOT NULL" {
		t.Fatalf("index predicate = %q", SemanticLabelMergeHalfvecIndexPredicate)
	}
}
