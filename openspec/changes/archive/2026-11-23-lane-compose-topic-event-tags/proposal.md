<!-- complexity: simple -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report -->
<!-- review: 见 review.md -->

# Proposal: 编排态节点标记——已确认关键线索 ★

## Why

新建泳道编排态（workshop-lane-compose）的画布节点是"内容裸点"：14 天窗口下点下只有 ≤5 字截断标题，30 天窗口连标题都关闭；要判断"这条是不是我关心的"，只能逐条 hover 读全文。

但用户其实已经在日报里回答过这个问题——`topic_key_event_markers`（`state='confirmed'`）就是人工确认"这条线索是关键"的主权信号。它当前完全不进编排态：选料现场看不到自己确认过什么，人工判断被降级成"重新读一遍标题"。

本 change 把这层信号以**纯只读投影**的方式带回选料现场：事件节点带 ★，卡里说清是哪条线索。

## What Changes

1. **事件节点星形**：某 section 下存在 `state='confirmed'` 的关键注记 → 该节点主体由圆点**换成星形**（布尔，不带条数）；星形外接半径沿用既有 `pointRadius`，不改变「大小 = 接近程度」。
2. **节点卡明细**：卡内日期行之后追加「★ 已确认关键线索」区块，逐条列出全部被确认线索的冻结快照标题。
3. **左右两栏同步**：编排态左栏候选成员条目（日期行右端）与右栏②「可补充的事件」条目（日期行首）显示 ★，同一字段、无独立开关。
4. **后端投影**：`compose-candidates` 每条 section 追加 `confirmed_key_events: [{ marker_id, thread_id?, title, confirmed_at? }]`，恒为数组（空则 `[]`）；一条 SQL 按 `source_section_id IN (...)` + `state='confirmed'` 批量取，不 join thread。
5. **隐藏两个工具条入口**（用户 2026-11-23 定档）：编排面工具条不再渲染「显示已有泳道（只读）」复选框与「历史待启用话题」按钮；`showAssigned` 恒 `false`（只读参照层不显示），历史名单的数据与 API 能力保留但无 UI 入口。
6. **修拖拽手感（用户 2026-11-23 反馈）**：拖拽时被拖节点/整组**本体跟随光标实时移动**（不是只动一个小 ghost），并**阻止拖拽顺带选中画布上的文本**（`pointerdown` 阻止默认 + 拖拽态 `user-select:none`）；松手/取消清空位移。

## Non-Goals

- **不做事件标签**（原 proposal 的第 2 层整体移出本期）：标签不跨天复用、7 天孤儿清理硬删，母文档实测「同话题标签交集有候选仅 1.6%」，当描述展示价值也撑不起一层界面。
- **不做"同事件虚线环"**：同上实测，画不出可见结果；"和已选像不像"已由点大小/深浅（接近度）承担。
- **不改任何写入路径**：不写 marker、不改日报 reader 的注记交互、不改状态机。
- **不参与任何计算**：不改选入/移出、接近度、质心、阈值、候选链资格。
- **不在右栏新增段落或开关**（母 spec 边界：右栏只承载三段）；仅在右栏②「可补充的事件」条目日期行首加一个 ★ 字符。
- **不做模糊补挂**：marker 来源指针为 NULL（重建后脱离快照）时不显示 ★，不按标题/日期/话题猜归属。
- **不删历史名单与只读参照的底层能力**：本 change 只隐藏两个 UI 入口——`LaneComposeHistoryDialog` 组件与 `showAssigned` 状态保留，未来恢复只需加回控件。

## UI 方案（minor 复用契约）

在既有画布形态（`rework-lane-compose-swimlane-canvas` 已定：行=泳道、点击=只看、拖拽=选入）上追加两个极小落点：

- 节点主体换星形（空心星 = 未选 / 实心星 + ✓ = 已选），外接半径沿用原 `pointRadius`，不放大、不参与接近度编码；
- 节点卡日期行之后一行「★ 已确认关键线索：<线索标题>」（多条逐条列出）；
- 左栏成员条目日期行右端、右栏②条目日期行首各一个小星。

几何基准沿用已批准的 `rework-lane-compose-swimlane-canvas/ui-prototype/states/02-editing.svg`；旧 `ui-prototype/`（基于已删除的话题带 + 勾选形态）标为 superseded，不参与验收、不作为几何基准。

## Capabilities

- `workshop-lane-compose`（modified：**新增 3 条 requirement** —— 已确认关键线索标记 / 归属与数据口径 / 拖拽期间的对象跟随与文本选择隔离；**修改 1 条既有 requirement** —— 「候选话题侧栏」删去历史入口与只读参照开关的断言）

## Impact

- **后端**：`internal/topicgraph/repository/key_event_markers.go`（新增批量查询）、`repository/daily_report_manual_topic.go`（DTO + 装配）。handler 不改（整体透传）。
- **前端**：`app/api/persistentTopics.ts`（类型 + 归一）、`components/compose/LaneComposeCanvas.vue`（星形节点 + 卡内区块）、`components/compose/LaneComposeCandidates.vue`（左栏 ★）、`components/compose/LaneComposeInspector.vue`（右栏② ★）、`components/console/BoardComposeOverlay.vue`（工具条隐藏两个入口）。
- **spec**：`openspec/specs/workshop-lane-compose/spec.md` 追加 requirement。
- **部署后影响**：
  - (a) 编排态里，确认过关键注记的事件节点会带 ★，卡内多一行明细；没有确认注记的节点与现在**完全一致**；工具条少两个控件（「显示已有泳道（只读）」与「历史待启用话题」），只读参照层不再出现。
  - (b) 需要用户手动执行的操作：无（只读投影，无数据迁移）。
  - (c) 旧数据降级：日报重建后注记与来源 section/thread 失联的（指针 NULL）不显示 ★——注记快照仍在日报侧保留，这是"不冒认"的设计意图，不是数据丢失。
