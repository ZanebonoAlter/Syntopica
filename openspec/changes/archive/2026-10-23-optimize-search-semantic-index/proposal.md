<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: reading, topic-graph -->

# Proposal: optimize-search-semantic-index

## Why

站内搜索的语义路径长期降级（`semantic.skipped=true`，前端提示「语义匹配暂不可用」）。真库实测（2026-10-23）：

- 语义路径的标签余弦 SQL 在 14,658 条 identity 向量（2560 维）上做全表逐行 `<=>`，**3.74 秒**（`EXPLAIN ANALYZE`：Seq Scan + detoast，shared read 33,906 buffers）；
- 3 秒语义预算被这条 SQL 独占，embedding 还没算完就超时；即使 embedding 命中缓存（1ms）也照样跳过；
- 没有可用的向量索引：`topic_tag_embeddings` 上唯一的 HNSW 索引 `tmp_hnsw_test` 是 **INVALID**，且 pgvector 的 float32 HNSW 硬上限 2000 维，2560 维根本建不了；
- 搜索查询 embedding 本身实测非缓存 miss 平均 ~1.4s（本地 Qwen3-Embedding-4B，短 query），在 3s 预算内；真正的超时来源是 SQL，不是 embedding 调用。

结论：这是**语义召回 SQL 没有索引**导致的超时，不是 embedding 服务不可用。修掉 SQL 后，语义路径才能在 3s 内完成，搜索结果恢复语义命中。

## What Changes

- **新增 halfvec HNSW 部分索引**（迁移 `20261023_0001`）：`topic_tag_embeddings` 上对 `(embedding::halfvec(N))` 建 `halfvec_cosine_ops` HNSW，`WHERE embedding_type = 'identity'`；N 从列 `format_type` 动态解析（当前 2560，≤4000 才建）。pgvector 的 halfvec 索引上限 4000 维，绕开 float32 的 2000 维限制。
- **语义标签召回改 ANN 候选 + 精确复算**：先走索引取 top 128 近似候选（`ORDER BY (embedding::halfvec(N)) <=> ?::halfvec(N) LIMIT 128`，`hnsw.ef_search=128`），再对候选按原始 float32 `<=>` 精确复算 cosine，最后在 Go 侧按 tag 去重、阈值过滤、降序取 top_n。双约束（`min_cosine` + `top_n`）与返回分数语义保持不变。
- **维度变更守卫**：`ensureVectorDimension` 在 ALTER 列维度前 drop 旧索引，维度 ≤4000 时重建 halfvec identity 索引；>4000 时跳过并记录日志（语义路回退旧精确扫描）。
- **慢阶段日志**：语义路径 embedding / ANN 候选 / 精确复算 / 文章召回分别计时，超过阈值记 warn，方便下次定位。
- **非目标**：不改 API 参数/响应契约；不改 3s 语义超时与 skipped 降级语义；不改 embedding 供应商/模型（本地 4B 模型短 query ~1.4s，已在预算内）；不重嵌标签向量、不改维度。

## Capabilities

### New Capabilities

（无；本 change 为纯性能修复，语义召回的行为契约不变，`.openspec.yaml` 声明 `skip_specs: true`）

### Modified Capabilities

（无）

## Impact

- **数据模型**：新增一个部分 HNSW 索引（additive，可 DROP 回滚）；无列变更、无数据回填。
- **代码**：`backend-go/internal/platform/database/postgres_migrations.go`（迁移 + 索引 DDL 单一事实源）、`backend-go/internal/reader/search/semantic.go`（ANN 候选 + 精确复算 + 计时日志）、`backend-go/internal/tagmanagement/service/core/embedding.go`（维度守卫 drop/重建）、对应测试。
- **行为**：语义路径从「3.7s SQL 超时 → 跳过」变为「ANN 候选 ~15ms + 精确复算 ~10ms + 文章召回 ~50ms，总耗时回到 embedding 主导的 ~1.5s」；3s 内正常返回语义命中。
- **部署**：首次启动迁移会构建索引，实测临时表 256MB 维护内存下 **~29s**（64MB 下 ~73s）；迁移在事务内执行，失败整体回滚，下次启动重试。
- **文档**：`docs/reference/database/tables/topic-tags.md`、`docs/reference/database/tables/_conventions.md`、`docs/reference/flow/reading.md`。
