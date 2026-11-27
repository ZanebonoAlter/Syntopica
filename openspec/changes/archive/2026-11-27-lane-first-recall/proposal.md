<!-- complexity: complex -->
<!-- diagrams: full(双通道收集的先后与去重、五道闸的先后顺序（排他→direction_mismatch→质量分区→L2→L3 防污染）、以及 gap=0 短路的准确位置——这三处都是「分支位置决定结论」的结构，纯文字读三遍也容易错位；11-26 事故本身就是因为读者（包括作者）以为二元开关只有两端，忽略了中间没有入口) -->
<!-- ui-impact: none -->
<!-- constraint-domains: daily-report, semantic-board, topic-graph -->

## Why

2026-11-26「全球科技巨头动态」日报只收了 10 篇文章（前一日 33 篇、再前一日 65 篇），当天所有英伟达算力管制新闻（`英伟达成立国家安全与出口管制监督委员会`、`英伟达暂停东南亚芯片交付`、`英伟达暂停向东南亚四国发货`、`标普全球评级将英伟达列入负面观察名单`、`英伟达逼近死亡交叉`）一条未进；用户配了辅助标签约束的泳道「黄仁勋风波及英伟达股价、产品发布」当天零 section。

全链路排查（详见 design「归因证据」）确认这不是辅助标签约束（该泳道两条约束均为 `preferred`，永不拦截；且约束只有「拦」的能力，没有「拉」新闻的能力），而是**召回层的两个结构性缺口**：

1. **质量分级把新事件整批丢弃**：`filterTagsByQuality` 是二元硬开关——高质量标签（`composite_hit`/`direct_hit`/`hit_rate`/`max_sim`）≥ 10 个时，所有 `weighted`（单篇文章产出的新事件天然落在这里）整批丢弃。该版块自 11-19 起长期处于这一侧，当天 24 个 `weighted` 全灭。11 月整月走势显示该开关天天在 7~52 之间抖动，行为不可预期。
2. **未挂载标签的现场匹配被随机截断**：`collectBoardTags` 的 fallback 对「未挂任何版块」的 event 标签用 `Limit(50)` 且**没有 `ORDER BY`**。11-26 有 363 个未挂载 event 标签（占新出 event 标签 61%），只随机取 50 个尝试匹配；算力相关标签排 116/162/290 名，从未被考虑。11-25 的 `美国收紧AI晶片出口管制` 排在 460 个中的第 277 名，同样从未被匹配，那篇《美国收紧AI晶片出口管制 韩国迎数据中心投资机遇》在 11-25 日报也没进。

判据实测（12 个有 active 泳道的版块、2 天、61 条泳道、307 对人工核对样本）证明：**以版块为召回单元时任何分数/距离判据都切不开**（`weighted` 区间 board score 全为 0.70；标签到最近泳道的绝对距离 quality p75=0.204 vs weighted p50=0.239，重叠严重）；**以泳道为召回单元则干净可分**——每泳道 Top-8、距离上限 0.15 时精度约 95%，且「当天没有专属新闻的泳道」会自动空着而不是硬拉邻座内容（1974 版块「以黎冲突升级」rank1 距离 0.241，此前正是靠 section 级 0.18 门槛把海湾/美伊/俄乌内容硬塞进来）。

## What Changes

- **新增泳道优先召回**（`lane-first-recall`）：每天日报生成时，对每条 active 泳道用 ANN 在当前**全量 event 标签池**（不限于已挂载本版块的标签）里召回 Top-K（默认 8）且距离 < 上限（默认 0.16）的标签，作为独立召回通道并入候选集。召回用 `topic_tag_embeddings` 上现成的 semantic halfvec HNSW 索引，61 条 active 泳道 = 61 次 ANN 查询/天。
- **泳道证据替代版块统计证据**：经泳道召回得到的标签直接放行，不参与 `filterTagsByQuality` 的 `kept >= 10` 二元开关判定——它已经有比统计证据更强的判据（离某条泳道的语义距离）。
- **两个参数进 `ai_settings` 在线覆盖**：`lane_recall_top_k`（默认 8）、`lane_recall_max_distance`（默认 0.16），与 `persistent_topic_lane_l1_threshold` 同模式。
- **L3 防污染闸**：经泳道召回进来的标签，若 L2 裁决为 `new`，直接丢弃、**不得进入 L3 自建新泳道**——弱证据不许自己开分店。
- **保留既有约束**：跨版块排他（`semantic_board_report_exclusive_gap`）、`direction_mismatch` 过滤、辅助标签约束闸门、去重与 L1/L2/L3 聚类全部行为不变；泳道召回只加一条 ingress。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `daily-report-system`：候选标签收集新增泳道召回通道（召回单元从版块变为泳道）、两个可调参数（`lane_recall_top_k` / `lane_recall_max_distance`）、“当天无专属新闻的泳道保持空白”约束；质量分级对「有泳道证据」的标签放行；L2 判 `new` 的泳道召回标签不进入 L3。

## Impact

- 代码：`backend-go/internal/topicgraph/repository/daily_report_topic_repository.go`（新增 ANN 召回查询）、`backend-go/internal/topicgraph/service/daily_report_orchestrator.go`（`collectBoardTags` 并入泳道召回、`filterTagsByQuality` 认泳道证据）、`backend-go/internal/topicgraph/service/daily_report_lane.go`（L2 判 new 的泳道标签不回 L3）、`backend-go/internal/platform/database/`（两个 ai_settings 播种迁移）
- 配置：`ai_settings` 新增 2 个 key（可缺省，默认值生效）
- 数据：无表结构变更、无回填；`topic_tag_board_labels` 不变（泳道召回不写挂载记录）
- **用户可见行为**：日报会重新收进「属于某条已有泳道但当天只有 1 篇文章」的新事件；当天没有专属新闻的泳道保持空白而不再被邻座内容填充。量级实测约 +5 标签/版块/天，单条泳道当天最多 +8 条新线索。
- **不改变**：标签→版块的永久挂载判定（2026-10-04 非农事故防线保持原样）；已生成的存量日报不重建。
