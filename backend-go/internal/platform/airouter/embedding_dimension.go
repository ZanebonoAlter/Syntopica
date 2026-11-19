package airouter

// 向量维度契约（openspec capability `embedding-dimension-contract`）：
//
// 维度是**运行时属性**——由 embedding 模型的实际输出决定，逐行随值留存。
// 向量列的类型声明不再承载维度（unify-vector-column-dimension 把存量列统一成
// 无维度限定的 `vector`），列级约束随之消失，写入前的维度一致性校验就成了
// 唯一的维度防线：一条错维度的向量写进去，比事后查询时报错更难排查。
//
// 本文件是各写入路径共用的校验实现（platform 层——tagmanagement / discovery /
// topicgraph 三个域都依赖它，不引入跨域依赖）。

import (
	"fmt"
	"strings"
)

// ValidateEmbeddingDimension 校验向量的实际维度与随值留存的维度字段一致。
//
// dim ≤ 0 表示维度未知（provider 未回传维度）——没有可比对的目标，不构成
// 不一致，放行。其余情况维度不符即返回错误（调用方拒绝本次写入并记错误日志）。
func ValidateEmbeddingDimension(vec []float64, dim int) error {
	if dim <= 0 {
		return nil
	}
	if len(vec) != dim {
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d", dim, len(vec))
	}
	return nil
}

// PgVectorDimension 返回 pgvector 文本表达式的元素个数（= 向量维度）。
// 非 "[a,b,c]" 形态或空表达式返回错误。
func PgVectorDimension(pgVector string) (int, error) {
	s := strings.TrimSpace(pgVector)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return 0, fmt.Errorf("malformed pgvector literal")
	}
	body := strings.TrimSpace(s[1 : len(s)-1])
	if body == "" {
		return 0, fmt.Errorf("empty pgvector literal")
	}
	return len(strings.Split(body, ",")), nil
}

// ValidatePgVectorDimension 校验 pgvector 文本表达式的实际维度与随值留存的维度
// 字段一致（dim ≤ 0 = 维度未知，放行；表达式畸形则报错——畸形向量本来也写不进库）。
func ValidatePgVectorDimension(pgVector string, dim int) error {
	if dim <= 0 {
		return nil
	}
	actual, err := PgVectorDimension(pgVector)
	if err != nil {
		return fmt.Errorf("embedding dimension check: %w", err)
	}
	if actual != dim {
		return fmt.Errorf("embedding dimension mismatch: expected %d, got %d", dim, actual)
	}
	return nil
}
