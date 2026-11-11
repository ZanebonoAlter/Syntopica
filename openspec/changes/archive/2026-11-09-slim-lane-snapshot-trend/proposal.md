<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: daily-report -->

## Why

泳道趋势「近 14 天」档的长版叙述（≤1000 字）实为逐日大事记复述（实测多条泳道长版打满 1000 字上限，按日期逐条排事件），与同区块已有的「逐日事件」展开完全职责撞车；短版态势句则是「定性一句 + 事件罗列」，事件部分与 section 正文重复，且无量级数据支撑、讲不出趋势。趋势区的定位应是「大致形势 + 走向」（演进叙事：阶段划分、转折驱动、量级佐证），事件明细完全交给逐日时间节点。顺带收益：单版纯文本输出使结算 token 预算下降、解析降级链消失。

## What Changes

- **结算产物收敛为单版演进叙事态势句**：`daily_report.lane_snapshot` 从「JSON 双字段（summary ≤200 字 + detail ≤1000 字成段叙述）」改为纯文本单段（150-200 字），内容为演进叙事——阶段划分（带大致日期锚）→ 每阶段驱动/转折（事件仅作转折标记，不展开不罗列）→ 当前落点与局势判断 → 量级佐证。现长短两版均被取代，全泳道只此一段。
- **素材新增量级信号段（后端算好，零 AI 成本）**：每日线索量 + 近 7 天 / 上个 7 天合计对比，作为 prompt 输入的一部分，让趋势判断有据可依、不编数字。
- **prompt 定位重写**：按阶段组织而非按日期罗列；只描述已发生走向（SHALL NOT 预测）；自由成段不固定模板；禁止逐日流水账。
- **解析路径简化**：弃 JSON 双字段解析与「JSON 坏→整段截 200 当短版」降级分支，改为纯文本 trim + rune 截断 + 空值守卫（空输出仍报错跳过，下个日报日自愈）。
- **MaxTokens 1536 → 500**：单版 200 字中文的输出预算。
- **`rolling_detail` 停止产出**：结算固定写空串；列保留不做 migration；存量长版随下个日报日结算覆盖自愈，不回填（沿用既有「空=合法」约定）。
- **前端 14 天档**：主视觉直接渲染单版态势句；删「长版优先 / 长版缺失回退短版 / 『长版随下次日报结算生成』提示」整条降级链。
- **不动**：月/年档（enrichment contexts 归档摘要）、板块内容 tab 泳道动态卡片（本就只渲染短版）、逐日事件就地展开（事件明细职责归它）。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `board-lane-dynamics`: 「日报后滚动态势结算」Requirement 从长短两版（200/1000 字、JSON 双字段、两版同窗同素材）改为单版演进叙事态势句（150-200 字纯文本）+ 素材量级信号段 + 输出预算与解析路径变更；存量长版缺失/为空的合法性与自愈约定不变。
- `lane-trend-overview`: 「14 天档内容与降级」Requirement 从「长版优先、长版缺失回退短版+提示」改为「单版态势句直接渲染」，降级分支删除；快照缺失占位、月/年档、逐日事件行为不变。

## Impact

- 后端：`backend-go/internal/topicgraph/service/lane_snapshot.go`（system prompt / 素材构造 / 解析 / 结算写入）、量级信号计算（基于 `ListLaneSnapshotMaterial` 素材行派生或仓储补一次计数查询，design 定）；`lane_snapshot_test.go` 解析与结算用例重写。
- 前端：`front/app/features/tags/components/daily-report/LaneTrendOverview.vue`（14 天档渲染分支收敛）、`LaneTrendOverview.test.ts`（降级用例删改）。
- 数据：`topic_lane_snapshots.rolling_detail` 存量数据不迁移不清理，新结算写空自愈；无 schema 变更。
- 消费方核对：板块内容 tab 泳道动态卡片与 console 泳道演进带读 `rolling_summary`（演进带在存量 detail 非空时仍透出旧长文至该泳道下个日报日自愈，无功能破坏）；日报阅读视图趋势区只读 `rolling_summary`。
- 部署后影响：下个日报日起，14 天档趋势区从「千字长文+短版」变为单段演进叙事；存量长版在其泳道下个日报日结算前仍按新渲染逻辑不再展示（前端不再读 detail）。
