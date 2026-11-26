package database

// optimize-search-semantic-index tasks 1.1 — halfvec identity HNSW 索引的
// DDL 单一事实源与列维度解析单测（无 DB，`-short` 下也跑）。
//
// 覆盖 test-cases.md 白盒「vectorColumnDimension」「TopicTagIdentityHalfvecIndexDDL」
// 两行：维度解析的各形态、DDL 的名称/表达式/opclass/谓词。迁移本身（读列
// format_type → 建索引 / skip）由 testcontainer 的既有语义测试间接覆盖。

import (
	"strconv"
	"strings"
	"testing"
)

func TestVectorColumnDimension(t *testing.T) {
	cases := []struct {
		name    string
		typeStr string
		want    int
	}{
		{name: "当前运行时维度", typeStr: "vector(2560)", want: 2560},
		{name: "halfvec 上限 4000", typeStr: "vector(4000)", want: 4000},
		{name: "超 halfvec 上限（迁移跳过）", typeStr: "vector(4096)", want: 4096},
		{name: "小维度", typeStr: "vector(1024)", want: 1024},
		{name: "未限定维度", typeStr: "vector", want: 0},
		{name: "空串", typeStr: "", want: 0},
		{name: "空括号", typeStr: "vector()", want: 0},
		{name: "非数字维度", typeStr: "vector(abc)", want: 0},
		{name: "零维度", typeStr: "vector(0)", want: 0},
		{name: "负维度", typeStr: "vector(-8)", want: 0},
		{name: "别的向量类型", typeStr: "halfvec(2560)", want: 0},
		{name: "多余空白", typeStr: "  vector(1536)  ", want: 1536},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := vectorColumnDimension(tc.typeStr); got != tc.want {
				t.Fatalf("vectorColumnDimension(%q) = %d, want %d", tc.typeStr, got, tc.want)
			}
		})
	}
}

func TestTopicTagIdentityHalfvecIndexDDL(t *testing.T) {
	for _, dim := range []int{2560, 4000} {
		ddl := TopicTagIdentityHalfvecIndexDDL(dim)
		mustContain := []string{
			"CREATE INDEX IF NOT EXISTS " + TopicTagIdentityHalfvecIndexName,
			"ON topic_tag_embeddings USING hnsw",
			"((embedding::halfvec(" + strconv.Itoa(dim) + ")) halfvec_cosine_ops)",
			"WHERE embedding_type = 'identity'",
		}
		for _, want := range mustContain {
			if !strings.Contains(ddl, want) {
				t.Fatalf("DDL(dim=%d) missing %q\ngot: %s", dim, want, ddl)
			}
		}
	}

	if name := TopicTagIdentityHalfvecIndexName; name != "idx_topic_tag_embeddings_identity_halfvec_hnsw" {
		t.Fatalf("index name = %q（改名会让部署回滚脚本与文档失效）", name)
	}
}
