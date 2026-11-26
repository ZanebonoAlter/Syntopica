<!-- domain: topic-graph, daily-report -->

## Purpose

日报 section embedding 的内容化生成与历史回刷：section 向量基于所聚 tag 的事实信息（label/description/代表文章摘录）而非 cluster_label 标题文本，打破「lane 命中 section 标题冻结 → 质心永不漂移 → 同域 tag 无限吸附」的回声闭环。

## Requirements

### Requirement: Section embedding 内容化生成

系统生成日报 section 时，section 的 embedding SHALL 基于**该 section 实际聚合的内容**生成，而非 `cluster_label` 标题文本。embedding 文本 SHALL 按以下规则组装（确定性，不含 LLM 生成产物）：

1. 主体：该 section `cluster_tag_ids` 内每个 tag 依次拼接 `label`、`description`（非空时，前缀「：」）、`ArticleContext` 代表文章摘录（非空时，前缀「；代表文章：」，单 tag 截断 100 runes）；
2. 各 tag 片段以换行连接，总长截断 480 runes（按 embedding 网关单条输入 512 token 上限校准，2026-08-22 实测 545 token 文本被拒）；
3. 兜底链：section 无 tags 时 SHALL 依次退化为 thread 标题拼接、`cluster_label` 文本。

生成时机 SHALL 保持 threads 收集之后、同日合并之前（embedding 基于合并前的 tag 分组）。

#### Scenario: 内容化 embedding 正常生成

- **GIVEN** 某 section 聚合 3 个 tag，各有 label/description，其中 1 个有 ArticleContext
- **WHEN** 编排流水线执行到 section embedding 生成步骤
- **THEN** 系统 SHALL 将 3 个 tag 的 label+description+代表文章摘录拼接文本送入全局 embedding 模型（CapabilityEmbedding），结果写入该 section 的 embedding 列

#### Scenario: 空 tags 兜底

- **WHEN** 某 section 的 cluster_tag_ids 为空但存在 threads
- **THEN** 系统 SHALL 以 thread 标题拼接文本生成 embedding；若 threads 亦为空，SHALL 以 cluster_label 文本生成

#### Scenario: 截断上限

- **GIVEN** 某 section 聚合 10 个 tag，拼接文本总长 5000 runes
- **WHEN** 组装 embedding 文本
- **THEN** 系统 SHALL 将文本截断至 480 runes 后送嵌入

### Requirement: Section embedding 历史回刷

现有 `POST /api/daily-reports/backfill-embeddings` 端点 SHALL 扩展支持两种模式，回刷文本 SHALL 与流水线内容化规则一致（从 DB 反查 tag label/description/代表文章上下文组装）：

- **补缺模式（默认）**：对 `embedding IS NULL` 的 section 按内容化规则生成 embedding（替换旧 cluster_label 口径）；
- **重算模式（`recompute=true`）**：对范围内全部 section（含已有 embedding）重算，范围由可选 `board_id`（缺省全部 board）与 `since_days`（缺省 30，0 表示不限）限定。

两种模式完成后 SHALL 对受影响 topic 重算 centroid，再重建相关 board 的跨日 section 关系。单个 section 的嵌入失败 SHALL 跳过并计数，不阻塞整体回刷。

#### Scenario: 重算模式回刷指定板块

- **WHEN** 调用 `POST /api/daily-reports/backfill-embeddings?recompute=true&board_id=5&since_days=30`
- **THEN** 系统 SHALL 对 board 5 近 30 天的全部 sections 按内容规则重算 embedding，刷新相关 topic centroid 并重建 board 5 的跨日关系，日志返回统计

#### Scenario: 补缺模式口径统一

- **WHEN** 调用 `POST /api/daily-reports/backfill-embeddings`（无参数）
- **THEN** 系统 SHALL 对无 embedding 的 sections 按内容化规则（而非 cluster_label 文本）补齐，后续流程同重算模式

#### Scenario: 单条嵌入失败不阻塞

- **WHEN** 回刷过程中某 section 的 embedding 调用失败
- **THEN** 系统 SHALL 跳过该 section（保留原 embedding）并继续处理其余 sections，统计中跳过计数 +1
