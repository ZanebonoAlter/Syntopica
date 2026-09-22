<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report, topic-graph, data-enrichment, ai-summary -->

## Why

打开日报展开一条话题泳道，第一眼看到的是今天的流水账（线索/文章明细）+ 7 天节点图（点一天才看一天），缺少"这条泳道最近整体在怎么走"的趋势视角；而趋势信息（近 14 天态势句）其实已经在板块内容 tab 的泳道动态卡片里，两处割裂。且态势句受 ≤100 字硬截断，复杂话题的概要经常被切在半空（实测"……会谈多次推迟，最终于 9 月"戛然而止），长内容无处可看。月/年维度的周期归档摘要（topic_lifeline_context，定时任务持续维护）已有基建但从未上过人看的 UI。

## What Changes

1. **结算产物长短两版**：泳道态势结算（每日日报后异步）单次 LLM 调用同时生成「短版 ≤100 字一句话 + 长版 ≤500 字成段叙述」；`topic_lane_snapshots` 新增可空长版列，卡片短版行为不变，日报趋势区消费长版。存量快照无长版，下个日报日自愈，不回填。
2. **聚合响应携带长版**：`GET /semantic-boards/:id/lane-dynamics` 的 `lane.snapshot` 增加长版字段，板块内容卡片继续只渲染短版。
3. **日报泳道趋势区**：日报阅读视图话题泳道展开体顶部新增「泳道趋势」区块（既有页面结构与交互模式内）：近 14 天 / 月 / 年 三档分段切换，展示对应概要全文（不再截断）与汇总截止信息；缺数据时按档位降级占位（与现有"态势待结算"同风格）。今天的 section 明细与 7 天节点图保留在下方。
4. **14 天逐日事件可展开**：趋势区 14 天档内提供逐日事件时间线的就地展开（复用板块内容卡片的"日期 → 当日事件"结构），让长版叙述与逐日事实互相对照。

## Capabilities

### New Capabilities

- `lane-trend-overview`: 日报话题泳道展开区的泳道趋势区块——14 天（长版态势）/ 月 / 年三档概要切换、缺数据降级、逐日事件就地展开。

### Modified Capabilities

- `board-lane-dynamics`: 结算产物从单句 100 字扩展为长短两版（同窗同素材一次生成）；`topic_lane_snapshots` 存储与 lane-dynamics 聚合响应相应扩展；卡片渲染短版的需求不变。

## Impact

- **后端 topicgraph**：`service/lane_snapshot.go`（prompt 改造、两版解析与各自 clamp、空输出守卫）、`repository/daily_report_models.go`（模型加列）、`repository/lane_snapshot_repository.go`（upsert/聚合带长版）。handler 无需改动（透传）。
- **数据库**：`topic_lane_snapshots` 加可空 text 列（AutoMigrate，无 FK 变更，无数据迁移）。
- **前端**：`api/laneDynamics.ts`（snapshot 增 detail 字段）、`features/tags/components/daily-report/` 新增趋势区组件并挂载进 `DailyReportTopicSection` 泳道体顶部；月/年复用 `api/boardEnrichment.ts` 既有 contexts 端点（`GET /persistent-topics/:id/enrichment/contexts?granularity=month|year`）。
- **AI**：lane snapshot 单次调用的输出预算上调（两版合计），capability 路由与 operation 不变（`daily_report.lane_snapshot` / digest_polish）。
- **文档**：`flow/daily-report.md`（结算产物）、`flow/topic-graph.md`、`docs/reference/api/`、`docs/reference/database/` 实现时同步。

## 部署后影响与需要的操作

部署后用户打开日报展开泳道，顶部多出「泳道趋势」区：14 天档首日可能仍无长版（存量快照只有短版，趋势区先以短版兜底并提示随下次日报结算升级）；月/年档展示 lifeline 归档的最新周期摘要，个别泳道无归档时显示占位文案。板块内容 tab 卡片外观不变。需要的操作：无强制项；可选——部署后手动触发一次日报生成即可让全部活跃泳道当日成立长版；月/年数据由既有定时任务维护，无需配置。旧数据不删除不回填，长版列为可空新列，降级路径完整。
