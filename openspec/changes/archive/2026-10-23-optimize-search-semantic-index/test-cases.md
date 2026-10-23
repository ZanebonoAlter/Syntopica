# Test Cases: optimize-search-semantic-index

> 层选择：索引 DDL/维度解析 → Go 单测；语义召回正确性/去重/ANN 对账 → PG testcontainer 集成测试；真库性能 → 只读 EXPLAIN + API 连续抽查；无 UI 层（ui-impact none）。

## 1. 主链路故事表（语义路径从超时到命中）

| 步 | 动作 | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- |
| 1 | 迁移在空/现有表上执行 | 生成 `idx_topic_tag_embeddings_identity_halfvec_hnsw`，`indisvalid=true`，表达式为 `(embedding::halfvec(N))`、谓词 `embedding_type='identity'` | PG | semantic_pg_test.go |
| 2 | 查询向量 → ANN 候选 | `EXPLAIN` 显示 Index Scan（或至少候选查询 <100ms），候选 ID 非空 | PG/真库 | semantic_pg_test.go + 6.5 |
| 3 | 候选精确复算 + 阈值/top_n | 返回标签分数与旧暴力 cosine 一致，`min_cosine`/`top_n` 边界不变 | PG | 既有 boundary/topN 测试 |
| 4 | 同 tag 多 text_hash | 只返回一条 MatchedTag，分数取 max | PG | 新增 dedupe 测试 |
| 5 | 160 标签 ANN vs 暴力 | top-8 标签与分数完全一致 | PG | 新增 ANN 对账测试 |
| 6 | 维度 >4000 / ANN 失败 | 回退旧精确扫描并 warn，不 panic | 单测/代码审查 | semantic.go 分支 |

## 2. 变体走查（五组）

- **输入**：查询向量维度 = 列维度 / 小于 / 大于 4000；identity 行 0 / 1 / 14k；候选全部低于阈值；同 tag 多行重复。
- **前置**：索引存在且 valid / 索引缺失 / 索引 INVALID（迁移先 drop 再建）；pgvector <0.8（无 halfvec）→ ANN 失败回退；表不存在 → 迁移 no-op。
- **时间窗口**：无时间语义；迁移构建时间 64MB/256MB 两档已实测（73s/29s），不设测试断言避免 CI 波动。
- **幂等**：迁移重复执行（IF NOT EXISTS + invalid 检查）；索引已存在且 valid → 秒过；迁移失败 → 事务回滚、版本不记录、下次重试。
- **可用性**：语义路径 3s 超时/embedding 失败仍 skipped；索引缺失时回退旧扫描（可能超时）但不报错。

## 3. 白盒附加（分支表）

| 分支 | 边界值 | 用例 |
| --- | --- | --- |
| `vectorColumnDimension` | `vector(2560)` / `vector` / `vector(4000)` / `vector(4096)` / 空串 | database 单测 |
| `TopicTagIdentityHalfvecIndexDDL` | dim 2560 / 4000 | database 单测（名称/表达式/谓词） |
| `matchTagsANN` | 候选 0 / 1 / 128；同 tag 多行 | semantic_pg_test.go |
| `rankMatchedTags` | 分数 < min / = min / > min；超过 top_n | 既有 boundary/topN + 新增 |
| `matchTagsExact` 回退 | ANN 返回错误 / 维度 >4000 | 代码审查 + 构造错误注入（可白盒） |
| 迁移 invalid 索引 | 同名 INVALID 索引存在 | database 单测/手工核对 |
| `ensureVectorDimension` | dim 1024 / 2560 / 4000 / 4096 | 代码审查 + `go build`（不污染共享测试库） |

## 4. 效果核对（真库只读）

| 指标 | 现状（2026-10-23） | 预期 | 核对 |
| --- | --- | --- | --- |
| 标签余弦 SQL | 3.74s（Seq Scan + detoast） | <100ms（Index Scan） | `EXPLAIN (ANALYZE, BUFFERS)` 新候选查询 |
| 索引 | `tmp_hnsw_test` INVALID / 0 bytes | `idx_topic_tag_embeddings_identity_halfvec_hnsw` valid / ~114MB | `pg_index` + `pg_relation_size` |
| `/api/search` 语义 | `semantic.skipped=true`，tookMs ~3.0-3.6s | `skipped=false`，tookMs <3s（典型 ~1.5-2s） | 连续 5 次 `q=英伟达` 留痕 |
| 语义命中标签 | 无（skipped） | 返回命中标签列表（top_n≤8） | 响应 `semantic.tags` |

## 5. 展示字段盘点

无新增/修改用户可见字段；`semantic.tags`/`skipped` 契约不变，仅恢复其应有行为。无 UI 验收（ui-impact none）。
