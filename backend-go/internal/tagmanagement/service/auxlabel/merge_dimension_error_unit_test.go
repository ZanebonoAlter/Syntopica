package auxlabel

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// --- fix-aux-label-dedup-ann review R-2：换模型维度切换窗口的召回错误可诊断化 ---
//
// 场景：换 embedding 模型后，semantic_labels 里新旧维度的 merge 向量并存。召回
// SQL 自带 cast（merge_embedding::halfvec(<dim>)），旧维度行会让**整条查询**报
// pgvector 22P02 —— 不是「跳过该行」，也不是「退化成顺序扫描」，而是整批 attach
// 失败。所以这个错误必须自带处置指引（清理 SQL + backfill 端点），不能以裸 SQL
// 错误冒出去。

// 维度类错误（22P02 cast / 42883 距离算子）必须被包装成可诊断错误。
func TestWrapMergeRecallErrorDiagnosesDimensionMismatch(t *testing.T) {
	castErr := &pgconn.PgError{Code: "22P02", Message: "expected 2560 dimensions, not 1536"}
	wrapped := wrapMergeRecallError(fmt.Errorf("recall merge candidates: %w", castErr), 2560)
	require.Error(t, wrapped)
	require.Contains(t, wrapped.Error(), "expected 2560 dimensions, not 1536", "原始 pgvector 消息必须保留")
	require.Contains(t, wrapped.Error(), "dimension mismatch")
	require.Contains(t, wrapped.Error(), "UPDATE semantic_labels SET merge_embedding = NULL")
	require.Contains(t, wrapped.Error(), "vector_dims(merge_embedding) <> 2560", "清理 SQL 必须带上运行时维度")
	require.Contains(t, wrapped.Error(), "backfill-embeddings", "必须指向既有 backfill 端点")

	// 距离算子的维度不符（42883）同样可诊断。
	opErr := &pgconn.PgError{Code: "42883", Message: "different vector dimensions 2560 and 1536"}
	require.Contains(t, wrapMergeRecallError(opErr, 2560).Error(), "UPDATE semantic_labels SET merge_embedding = NULL")

	// 非维度类错误原样透传（不添无关处置建议）。
	other := errors.New("connection reset by peer")
	require.Equal(t, other, wrapMergeRecallError(other, 2560))
	require.NoError(t, wrapMergeRecallError(nil, 2560))
}

// 判定口径：只有「维度类」错误才算（22P02 / 42883 + 消息含 dimension）。
func TestIsMergeDimensionMismatch(t *testing.T) {
	require.True(t, isMergeDimensionMismatch(&pgconn.PgError{Code: "22P02", Message: "expected 8 dimensions, not 3"}))
	require.True(t, isMergeDimensionMismatch(&pgconn.PgError{Code: "42883", Message: "different vector dimensions 8 and 3"}))
	require.False(t, isMergeDimensionMismatch(&pgconn.PgError{Code: "22P02", Message: "invalid input syntax for type vector"}))
	require.False(t, isMergeDimensionMismatch(&pgconn.PgError{Code: "23505", Message: "duplicate key value"}))
	require.False(t, isMergeDimensionMismatch(errors.New("expected 8 dimensions, not 3")))
	require.False(t, isMergeDimensionMismatch(nil))
}
