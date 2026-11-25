<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: semantic-board -->

## Why

辅助标签 L2 语义去重（`auxlabel/auxiliary_label_service.go` 的 `sqlMergeMatcher`）把全部 active 辅助标签的 `merge_embedding` 向量加载进进程内存（26,713 条 × 2560 维 float64 ≈ **547MB**），每个 L1 未命中标签都对全集算一遍余弦——实测 **1.0 秒/标签**（`auxiliary label dedup gate slow` 已告警 2,050 次）；缓存 10 分钟 TTL 到期后重拉并解析 26,713 行向量文本，**单核 9.6 秒**。这是 2026-11-24 22:00 前后后端 RSS 冲到 2.6GB、CPU 283%、页面接口 4-5 秒的直接原因。

`semantic_labels.merge_embedding` 目前**没有任何向量索引**；而 pgvector 0.8.3 的 halfvec HNSW 支持到 4000 维，2560 维完全在范围内——`topic_tag_embeddings` 已有三条同构索引在跑。

## What Changes

- 为 `semantic_labels.merge_embedding` 建 **halfvec 表达式 HNSW 部分索引**（`WHERE label_type='auxiliary' AND status='active' AND merge_embedding IS NOT NULL`），新增 migration + 启动期维度守卫（沿用 `topic_tag_embeddings` 的 halfvec 索引模式，含 `maintenance_work_mem` / `max_parallel_maintenance_workers=0` 的 Docker `/dev/shm` 规避）。
- `sqlMergeMatcher` 由「进程内全量余弦」改为「数据库 ANN 召回（halfvec HNSW）+ 原列 float32 精确重排 + 既有口径裁决」（cosine ≥ 去重阈值、RefCount 大者优先、同 RefCount 取 ID 小者），对齐 `findSimilarByVector` 既有两段式范式。
- 删除 `packageAuxLabelCache` 的 `mergeEmbeddings` 向量缓存与 `auxMergeEmbeddingsTTL`（省 ~547MB 常驻内存，并消除 `applyCreate` 注释所述的 map 并发读写风险源）。
- L1 精确匹配由「每条标签线性扫 26,767 条」改为 `bySlug`（含别名 slug）/ `byAliasFold` / `byNormKey` 三个 map 索引。
- 归并路径的既有不变量保持不变：只 addAlias、不改 Label、不重算/不触碰 `embedding` 与 `merge_embedding` 列。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `auxiliary-label`: 新增「语义去重走数据库 ANN 索引」要求——L2 候选召回 MUST 通过 halfvec HNSW 索引完成并设置 `hnsw.ef_search`，阈值判定 MUST 基于原列精确距离（float32 重排），进程 SHALL NOT 持有全量向量副本；归并结果须与全量遍历一致（差异逐条归因）。

## Impact

- **代码**：`backend-go/internal/tagmanagement/service/auxlabel/auxiliary_label_service.go`（matcher / cache / L1）、`backend-go/internal/platform/database/postgres_migrations.go`（新 migration + 索引守卫）
- **数据库**：`semantic_labels` 新增 1 条 halfvec HNSW 部分索引；无表结构变更、无数据迁移
- **内存**：后端 RSS 预期从 2.6GB 峰值回落到 ~300MB
- **性能**：单标签 L2 去重 1s → ~10ms；消除 10 分钟一次的 9.6 秒全量重载
- **语义**：归并阈值、RefCount 优先级、alias 积累口径不变（验收含新旧实现对账）
- **不影响**：API 契约、前端、其他标签路径
