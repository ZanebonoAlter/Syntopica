# Issue 01: L2 向量比较优化 — 排除向量列 + Go 侧 Cosine 计算

> **Status:** in_progress
> **Priority:** high
> **Component:** backend-go/internal/domain/tagging, backend-go/internal/platform/database

## 问题描述

`loadActiveAuxiliaryLabels()` 原先 `SELECT *` 拉取两个 vector(2560) 列，每行 ~33KB，
10K 行 ≈ 345 MB 传输，单次查询 6-16 秒。

L2 阶段原先把所有 merge_embedding 拉到 Go 做 cosine 循环——同样的 345 MB 传输瓶颈。

## 根因分析

pgvector HNSW 索引**无法用于 vector(2560)**：
- `vector` 类型 HNSW 限制 ≤ 2000 维
- `halfvec` cast 索引（≤ 4000 维）虽然能建，但 pgvector 查询优化器**不识别表达式索引**：
  `ORDER BY (col::halfvec) <=> ...` 仍然走 Seq Scan
- SQL `<=>` 全表扫描实测 3.6-4.5s，与 Go 侧等价

因此 2560 维度下 **SQL 推送 vs Go 计算 性能等价**，但 Go 侧只需加载 id + merge_embedding
两列（~10K × 20KB ≈ 200 MB vs 原来 SELECT * 的 345 MB），且避免 PostgreSQL 进程内计算开销。

## 修复方案

### 1. `loadActiveAuxiliaryLabels()` 排除向量列（已完成）

```go
Select("id, label, slug, label_type, aliases, ref_count, description, status, ...")
```

不再拉取 `embedding` 和 `merge_embedding`，10K 行从 ~345 MB 降到 ~5 MB。

### 2. L2 用独立查询只加载 `id + merge_embedding`（已完成）

```go
// sqlMergeMatcher: 只 SELECT id, merge_embedding WHERE id IN (...)
// 在 Go 侧做 cosine similarity 计算
```

- 避免全表 `SELECT *` 的 345 MB 传输
- 只加载 ~10K × 20KB = 200 MB（两列：uint + vector 字符串）
- Cosine 计算在 Go 侧，无 PostgreSQL 进程开销

### 3. 删除无效的 HNSW 索引代码（已完成）

- 删除 `createHNSWIndex()` 函数
- 删除迁移脚本中的 halfvec HNSW 索引定义
- `EnsureSemanticLabel*` 函数只负责列类型变更，不再尝试建索引

## 维度兼容性

| Embedding 模型 | 维度 | HNSW 可用 | 当前方案 |
|----------------|------|-----------|----------|
| 当前 4B 模型 | 2560 | ✗ | Go 侧 cosine ✓ |
| 8B 模型 | 4096 | ✗ | Go 侧 cosine（更慢） |
| ≤2000 维模型 | ≤2000 | ✓ | 可改回 SQL + HNSW |

## 后续事项

如果 8B 模型（4096 维）Go 侧计算也过慢，需要引入专用向量检索组件：
- **Milvus / Qdrant** — 专用向量数据库
- **Elasticsearch kNN** — 支持 4096 维 dense_vector
- **Redis Vector Search** — 轻量方案
- **降维** — Matryoshka / PCA 截断到 ≤2000 维

## 影响范围

- `auxiliary_label_service.go` — `sqlMergeMatcher` 改为 Go 侧计算；删除 `createHNSWIndex`
- `postgres_migrations.go` — 删除 HNSW 索引定义
