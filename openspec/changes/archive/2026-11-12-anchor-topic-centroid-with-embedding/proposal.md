<!-- complexity: simple -->
<!-- ui-impact: none -->
<!-- constraint-domains: topic-graph, daily-report -->

## Why

持久化话题的匹配锚点（centroid）目前是纯滚动窗口：最近 `centroid_window`（默认 30）条命中 section 的等权平均。话题创建时的定义向量（首义 embedding，`board_persistent_topics.embedding`）只在 section 数 <2 时作为退化备胎，滑出窗口后影响力彻底归零。宽口径话题因此漂移成"泛新闻磁铁"：manual 话题「日本内外产业政策」（id 1009，2026-07-04 创建，累计命中 79 次）当前质心距创建时所选 section 平均 0.214、距最近杂闻命中仅 0.084，2026-11-11 社会新闻「陈杰父亲赴日寻人」以 0.166 < L1 阈值 0.18 直连挂入（`l1_direct` 不过 LLM）。漂移自增强：质心漂向杂闻 → 杂闻最近锚点更近 → 更多杂闻进窗口 → 继续漂。

## What Changes

- **质心公式改为首义+滚动加权混合**：`centroid = α·首义 embedding + (1-α)·最近 centroid_window 条命中 section 等权均值`，α 为新配置项 `persistent_topic_centroid_anchor_weight`，默认 **0.5**。所有话题（manual 与 auto）统一生效。
- **新配置项**：`persistent_topic_centroid_anchor_weight`（embedding_config，float，范围 [0,1]，默认 0.5；解析失败回退默认并告警，与现有 persistent_topic_* key 惯例一致）。
- **存量一次性重算**：上线时把全部带质心话题（现库 1921 个）按新公式重算一次，立刻纠正漂移（含 1009）；此后每次日报命中后的质心刷新自动走新公式。
- **退化路径不变**：可解析 section <2 仍退化为首义向量。
- **spec 行为变更**：`persistent-topic` 能力的「PersistentTopic 质心表示」Requirement 从「质心随内容全量漂移」改为「有界漂移（首义锚定 + 内容自适应）」，配置项清单增加新 key。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `persistent-topic`：修改「PersistentTopic 质心表示」Requirement（锚点公式、漂移上界语义、存量重算），扩充「配置项」Requirement（新增 anchor weight key）。

## Impact

- **代码**：`backend-go/internal/topicgraph/repository/daily_report_topic_repository.go`（`ComputeTopicCentroid` 混合逻辑、`PersistentTopicConfig` 新字段、`LoadPersistentTopicConfig` 解析）；一次性重算入口（触发方式见 design）；纯函数单测 `daily_report_centroid_test.go`。
- **行为影响**：话题质心整体向首义回拉，边缘匹配（原 0.12~0.18 区间的杂闻 tag）预计从 L1 直连降级为 L2 LLM 裁决——错挂减少、L2 调用量上升；话题演化速度变慢（滚动侧只占 50% 权重）。
- **不做**：不改 L1/L2 阈值本身；不改 lane 分桶逻辑；不改首义 embedding 的生成方式；不做"重置首义"运营操作（留待后续）。
