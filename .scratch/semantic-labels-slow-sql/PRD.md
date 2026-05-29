# PRD: semantic_labels 慢查询优化

> **状态**: 实施中
> **优先级**: 高（打标签流程性能瓶颈）
> **关联**: `auxiliary_label_service.go`, `models/semantic_label.go`

## 背景

打标签流程中 `semantic_labels` 表出现大量慢查询（6~16 秒/次），3 天内累计 16,055 次全表扫描。表行数约 10,770 行，每行含两个 vector(4096) 列，单次查询传输约 345 MB。

## 根因分析

### 问题 SQL

```sql
SELECT * FROM "semantic_labels" WHERE label_type = 'auxiliary' AND status = 'active'
```

### 调用链（N+1 模式）

```
tagArticle(article)
  └─ for each tag (2~5 per article):
       └─ AttachAuxiliaryLabels(tagID, labels)
            └─ for each label (1~3 per tag):
                 └─ ResolveAuxiliaryLabel(label)
                      └─ loadActiveAuxiliaryLabels()   ← 每次全表扫描
```

### 瓶颈因素

1. `SELECT *` 拉回 `embedding` (vector 4096) 和 `merge_embedding` (vector 4096) — 每行约 32 KB
2. 10,770 行 × 32 KB ≈ **345 MB/次**
3. 每篇文 N 个 tag × M 个 label = N×M 次重复查询
4. 仅有 `(label_type)` 和 `(status)` 单列索引，无复合索引

### 次要慢查询

向量搜索 `topic_tag_embeddings` 的 `<=>` 距离排序（5,073 次，1.5~2.5 秒/次），使用 HNSW 索引，优先级较低。

## 方案

### Level 2（实施）：SELECT 排除向量列

`loadActiveAuxiliaryLabels` 改为只查询业务所需字段（slug、aliases、label 等），向量列按需延迟加载。

- slug/alias 精确匹配（L1）不需要向量
- merge embedding 比较（L2）在精确匹配失败后才需要，可按 ID 单独查询
- 预期：单次查询数据量从 ~345 MB 降至 ~2 MB

### Level 3（备选）：复合索引 + 查询拆分

```sql
CREATE INDEX idx_semantic_labels_type_status ON semantic_labels(label_type, status);
```

将 slug 查询和 embedding 查询拆分为独立步骤，slug 精确匹配走索引。
