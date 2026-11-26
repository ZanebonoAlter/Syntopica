<!-- complexity: complex -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report -->

## Why

今日重点（头条）的聚簇候选按线索 `scope <> composite` 一刀切排除综合稿线索，但 `scope` 由「线索内 event 标签数 ≥2」推导——同一事件的多表述标签（例：2026-11-12 黄仁勋婚外情，17 个高度相似的 event 标签聚合出 20 篇文章的单线索）会被误判为 composite 而整体出局。当天候选池只剩 1 篇文章的小簇，头条被「豆包接入新模型」（1 篇）占据，真正的 20 篇大热点进不了重点。聚簇本身按 related-event 向量相似度分簇、自带防混编能力，入口级 scope 排除既多余又误伤大热点。

## What Changes

- **放宽聚簇候选入口**：今日重点聚簇输入不再按线索 scope 排除——composite 线索的 event 标签与 single 线索同权参与聚簇；跨事件混编标签由聚簇相似度门槛天然隔离（自成小簇、不并入单事件簇）。
- **不动**：`scope` 列及其推导规则、「composite 不计入事件数 / 不建立事件线」的既有语义、聚簇三级降级链（向量聚簇 → 语义向量兜底 → 主体近似）、候选排序（簇内内容身份去重文章数 DESC → 簇内标签数 DESC → 代表标签 id ASC，Top 8）、LLM 白名单挑选与「模型不输出数字」约束、簇快照字段（cluster_id / article_count / cluster_mode / data_as_of / research_at）。
- **顺手还债**：主 spec「事件线热度先筛重点候选」requirement 仍是事件线热度时代的已退役口径（H/P 热度公式、事件线存活 id），与本 change 对齐为现行聚簇口径（REMOVED 旧 + ADDED 新）；flow 文档对应节同步。

## Capabilities

- **Modified**: `daily-report-system`
  - REMOVED「事件线热度先筛重点候选」（Reason: 事件线热度链路已随 retire-story-adopt-event-metrics 退役，现行实现为聚簇候选口径，spec 未同步）
  - ADDED「今日重点聚簇候选」（现行口径 + 本 change 的 composite 放宽）

## Impact

- **后端**：`backend-go/internal/topicgraph/repository/report_event_clusters.go`（`ListReportEventThreads` 的 scope 过滤放宽）；`service/daily_report_event_cluster.go` / `service/daily_report_story_highlights.go` 逻辑不变，输入集合扩大。
- **行为影响**：新报告与**重建的历史报告**重点候选池扩容——大热点单事件多标签簇可进 Top 8 白名单；全部线索为 composite 的日报不再走标签回退。已生成报告的 highlights 快照不回填，重建按新口径重算（预期内）。
- **效果依赖**：LLM 仍从 ≤8 白名单自由挑 ≤3 条，头条是否为大热点取决于候选排序（文章数为第一键，大簇排首位）+ LLM 选择；效果核对以「候选白名单第一簇」为服务端可断言锚点。
- **文档**：`docs/reference/flow/daily-report.md`「今日重点的选题与热度」节更新候选入口口径。
- **数据**：无迁移、无新列。
