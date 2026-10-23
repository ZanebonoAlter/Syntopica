# Design: optimize-search-semantic-index

## Context

现状见 proposal.md「Why」。关键代码事实：

- `backend-go/internal/reader/search/semantic.go` 的标签召回 SQL 对全部 identity 行做 `GROUP BY topic_tag_id + MAX(1 - (embedding <=> ?))`，无法走 ANN 索引；`topic_tag_embeddings` 的 identity 行有 14,658 条，`embedding` 是 `vector(2560)`，TOAST 总大小 713MB。
- `topic_tag_embeddings` 上唯一的 HNSW 索引 `tmp_hnsw_test` 为 INVALID；`tagmanagement/service/core/embedding.go` 的 `ensureVectorDimension` 明确写了「dim > 2000 跳过 HNSW」，因为 pgvector float32 HNSW 上限 2000。
- pgvector 0.8.3 的 `halfvec` 支持 HNSW/IVFFlat，维度上限 4000；临时表实测 halfvec HNSW 索引 114MB、构建 28.7s（maintenance_work_mem=256MB）、top-50 查询 15ms。
- 搜索语义路径整体 3s 超时（`SemanticSearcher.timeout`），embedding 短 query 非缓存 miss 平均 ~1.4s；SQL 3.7s 是超时主因。

## Goals / Non-Goals

**Goals**：让语义标签召回在 3s 预算内稳定完成；保持 `min_cosine + top_n` 双约束与 cosine 分数语义；维度变更时索引可自愈；部署可回滚（仅新增索引）。

**Non-Goals**：不改 API/UI 契约；不改 3s 超时与 skipped 降级；不换 embedding 模型/供应商；不重嵌标签；不做异步语义搜索（若后续实测仍被本地 embedding 队列拖慢，另开 change）。

## Decisions

### D1. 用 halfvec 部分 HNSW 索引绕开 2000 维限制

- **结论**：在 `topic_tag_embeddings` 上建表达式索引 `((embedding::halfvec(N)))`，opclass `halfvec_cosine_ops`，`WHERE embedding_type = 'identity'`；N 从列 `format_type` 解析，≤4000 才建。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go:searchSemanticHalfvecIndexMigration`
- **取舍**：不用 float32 HNSW（pgvector 硬上限 2000 维，2560 维建不了）；不用 IVFFlat（同样受 2000 维限制，且需要预训练 lists）；不对全部 embedding_type 建索引（语义搜索只查 identity，部分索引更小更快）。

### D2. ANN 候选 + 原始 float32 精确复算

- **结论**：先 `ORDER BY (embedding::halfvec(N)) <=> ?::halfvec(N) LIMIT 128` 取近似候选（`hnsw.ef_search=128`），再按候选 ID 用原始 `embedding <=> ?::vector` 精确复算 cosine；Go 侧按 tag 去重（同 tag 多 text_hash 取 max）、阈值过滤、降序取 top_n。
- **落点**：`backend-go/internal/reader/search/semantic.go:matchTagsANN`
- **取舍**：不直接把 halfvec 近似分当最终 cosine（量化误差可能让阈值边界标签翻面，违背 spec 的精确 cosine 契约）；不保留旧的全表 GROUP BY（3.7s，超时根因）；候选数不做无限大（128 足够覆盖 top_n=8 与多 text_hash 重复）。

### D3. 候选上限与 ef_search 固定为 128

- **结论**：`semanticCandidateLimit = 128`，`hnsw.ef_search = 128`；候选数远大于默认 top_n=8，且覆盖同 tag 多 text_hash 的重复行。
- **落点**：tasks.md `2.1`
- **取舍**：不用 top_n*2（重复行/阈值过滤可能把有效标签挤出候选）；不用 512（查询耗时与内存上升，收益递减）；参数先固定，后续可按真实召回率调整。

### D4. 维度变更守卫：drop 后按新维度重建

- **结论**：`ensureVectorDimension` 在 ALTER 列前 drop 旧 float32 索引与 halfvec 索引；dim ≤2000 重建旧 float32 HNSW；dim ≤4000 重建 halfvec identity 索引；>4000 跳过并记日志，语义路回退旧精确扫描。
- **落点**：`backend-go/internal/tagmanagement/service/core/embedding.go:ensureVectorDimension`
- **取舍**：不在 ALTER 后依赖迁移重建（迁移版本已记录，不会重跑）；不保留跨维度的旧索引（表达式依赖旧 typmod，Postgres 会拒绝/失效）。

### D5. 迁移在事务内构建，maintenance_work_mem 提升到 256MB

- **结论**：迁移 `20261023_0001` 默认 in-tx 路径；`SET LOCAL maintenance_work_mem='256MB'` 后 `CREATE INDEX IF NOT EXISTS`；构建失败整体回滚，下次启动重试。
- **落点**：`backend-go/internal/platform/database/postgres_migrations.go:searchSemanticHalfvecIndexMigration`
- **取舍**：不用 `CREATE INDEX CONCURRENTLY`（启动期无并发写入，in-tx 更简单且失败可原子回滚；实测 256MB 下 ~29s，64MB 下 ~73s）；不把索引构建放到 admin 命令（启动迁移是现有唯一 schema 事实源，避免漏建）。

### D6. 本次不改 embedding 供应商/超时

- **结论**：保持本地 Qwen3-Embedding-4B 与 3s 语义超时；本次只修 SQL。短 query 非缓存 miss ~1.4s，加 ANN/精确/文章召回后总耗时 ~1.6-2.0s，仍在 3s 内。
- **落点**：无代码落点：proposal.md「非目标」+ tasks.md `6.3` 真库核对，因为 SQL 3.7s 是超时主因、embedding 短 query 已在预算内；若实测仍被本地 tag 批量嵌入队列拖慢，另开 change 做异步语义/优先级。
- **取舍**：不把超时从 3s 调到 5s（会让慢查询更慢，且掩盖 SQL 根因）；不换远程 embedding（当前 `10.11.12.111:8081` 不可达，无法验证）。

### D7. 慢阶段日志

- **结论**：语义路径对 embedding / ANN 候选 / 精确复算 / 文章召回分别计时；超过阈值（embedding 1s、候选 200ms、精确 100ms、文章 200ms）记 warn，正常不刷日志。
- **落点**：`backend-go/internal/reader/search/semantic.go:Search`
- **取舍**：不逐次 info 打点（搜索频率高、日志噪声大）；不引入 metrics 新依赖（现有 logging 足够定位）。

## Risks / Trade-offs

- [迁移首次启动构建索引 ~29-73s] → 迁移在事务内，失败回滚并下次重试；部署时预留启动等待时间，构建完成后索引常驻。
- [halfvec 近似候选可能漏掉阈值边缘标签] → 候选 128 + ef_search 128，且对候选做精确复算；PG 测试用 160 个标签与暴力精确结果对账。
- [维度变更后索引失效] → `ensureVectorDimension` drop/重建；>4000 维回退旧扫描并记录。
- [索引缺失/旧 pgvector 不支持 halfvec] → ANN 查询报错时回退旧精确扫描并 warn；语义路仍受 3s 预算约束。
- [本地 tag 批量嵌入队列仍可能让 embedding 超过 3s] → 本次不解决；慢阶段日志会暴露，后续可做异步语义或 embedding 优先级。

## Migration Plan

1. 部署后首次启动自动执行 `20261023_0001`，构建索引（一次性 ~29-73s），健康检查可能延迟。
2. 回滚：`DROP INDEX IF EXISTS idx_topic_tag_embeddings_identity_halfvec_hnsw`；代码回退后旧 SQL 仍可用（只是慢），无数据损失。
3. 无需回填/重嵌；索引可从现有 embedding 列重建。

## Open Questions

无（embedding 队列优先级/异步语义作为后续独立 change，不在本次范围）。
