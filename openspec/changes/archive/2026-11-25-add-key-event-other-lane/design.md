# Design: 关键事件时间视野「其他」兜底泳道

## Context

见 `proposal.md` — Why。实现前必须知道的三条现状事实（均已核对代码）：

1. 行集合由 `lane-dynamics` 决定：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:661-668` 只接受 `status=active ∪ 被 watch 且未归档`、且窗口内有日报分区的话题（`:705-712`「沉寂不展示」）。候选话题只进 `candidates` 栏，永不进 `lanes`。
2. 事件响应不丢：`ListBoardKeyEventMarkers`（`backend-go/internal/topicgraph/repository/key_event_markers.go:635-649`）只按 board + `state=confirmed` + 日期窗过滤，**不按话题状态过滤**；`persistent_topic_id` 可空（FK `ON DELETE SET NULL`），normalizer 保留 `topicId: null`。
3. 丢弃发生在视图层两处：`KeyEventTimelineView.vue:228-242` 的 `rowTopicIds` 过滤；`useBoardKeyEvents.ts:32-40` 的 `buildKeyEventIndex` 跳过 `null`。所有读数（视图头当日条数、行头计数、整窗空态）都建立在这份过滤后的数据上。

## Goals / Non-Goals

**Goals**

- 「不属于任何已显示泳道」的 confirmed 事件有唯一、稳定、可读的落点（一行，不是 N 行）。
- 读数与事件同源：有几个节点，头部就读几条，绝不出现「行里有、头上 0」。
- 改动面限在前端两文件 + 测试；接口、DTO、表结构、`lane-dynamics` 口径全部不动。

**Non-Goals**

- 不把 candidate/archived 话题提升为正式泳道（会污染矩阵行序、休眠判定与 `lane-dynamics` 语义）。
- 不接「当日其他话题」（`dayOtherTopics.ts`）作为数据源：那是**所选那一天的线索**，跨日没有数据，而本行需要 7/14/30/365 全轴的确认事件。
- 不改右栏、不改矩阵视图、不改事件写入口（本视图仍只读）。
- 不做「其他」行的事件分组子标题（按话题分组）——第一条版本只保证事件可见与可读，事件卡已有所属话题名。

## Decisions

### D1. 「其他」用前端伪行承载，不加后端 lane

- **结论**：在前端合成一个伪行（`topicId = '__other__'`），把未归入行集合的事件塞进去；不修改 `lane-dynamics` 的 lane 集合。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：后端加 lane 的方案被否决——`WindowLane.topicId` 会进 `selectedLane` / 矩阵行序 / 休眠判定 / `lane-dynamics` 排序，一个非话题的伪 lane 要在 5 处做特例；而且事件数据本来就在前端手里，后端加 lane 也拿不到更全的事件。

### D2. 未归属事件用纯函数分组，不动按话题索引

- **结论**：保留 `buildKeyEventIndex`（`topicId=null` 不进话题索引的语义是对的），另加 `groupUnassignedEvents(events, assignedTopicIds)`：返回 `date → events[]`，收录 `topicId === null` **或** 不在 `assignedTopicIds` 的事件，保持接口序。
- **落点**：`front/app/features/tags/composables/useBoardKeyEvents.ts`
- **取舍**：改 `buildKeyEventIndex` 让 null 用一个哨兵 key 进索引会让「topicId → 事件」的类型与所有既有调用方都要处理哨兵；独立纯函数边界更清楚，也便于单测（无需挂载组件）。

### D3. 行集合统一为「可见投影」列表，读数与布局都从它派生

- **结论**：新增 `otherProjection`（伪 `ObservationLaneRow` + 事件 `byDate`）与 `visibleProjections = [...activeProjections, ...renderedDormant, ...otherProjections]`；把 `pendingGroups`、`rowCardEventMap`、`rowLayouts` 测量、行渲染四处原本写死 `[...activeProjections, ...renderedDormant]` 的循环统一改用 `visibleProjections`。`windowEventCount` 与 `projectionByTopicId` 纳入全部休眠行 + 「其他」行（不受折叠影响）。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：只给「其他」行单独写一套布局/卡片/测量逻辑会被否——两条标签带、引线树、卡片避让、行高自适应都会重复实现一遍且极易与正式行不同步（review R-23 已记录过一次「按行传矩形」的坑）。

### D4. 视图头当日条数改由「全量事件」派生

- **结论**：`dayEventsByDate` 不再按 `rowTopicIds` 过滤，而是对 `eventsByDate`（全量、含 null）按 `axisSet` 过滤。等值关系：任一 confirmed 事件要么属于某个持久化行，要么属于「其他」，因此该口径 == 「全部泳道（含其他）当日条数」。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：保留旧过滤再额外加 `others` 分支也能得到同一结果，但会留下第二套「谁算事件」的口径；合成一处后读数天然不会漏。

### D5. 「其他」行不参与选中与折叠

- **结论**：行头不绑定任何点击（本视图行头现在本就不 emit `selectLane`，`KeyEventTimelineView.vue:170-172` 注释即契约）；节点/标签点击沿用 `emit('selectDate')`。该行不进入 `dormantRows`、不受 `expandedDormant` 影响，位置固定在休眠组之后。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：让「其他」行走休眠折叠（作为特殊休眠行）被否决——用户不点展开就仍然看不到，与本次目的相反；写入 `selectedLane='__other__'` 被否决——矩阵里没有对应行，会制造 spec 明令禁止的「无对应行的死选择」。

### D6. 无事件不渲染，空态与错误态不造假

- **结论**：`otherProjections` 仅在 `otherEventsByDate` 轴内非空时产生一行。`emptyState === 'no-lanes'` 时若「其他」有事件，照常渲染行（说明行保留）；`eventsLoading` / 首错 / stale 分别沿用既有三态，不用空「其他」行冒充。
- **落点**：`front/app/features/tags/components/console/KeyEventTimelineView.vue`
- **取舍**：常驻一行并显示「0 条」被否决——正式泳道都只在有分区时成行，兜底行更不该制造空行噪声（spec Scenario「无兜底事件不占行」）。

## Risks / Trade-offs

- **[伪行 ID 泄漏到外部状态]** → 常量 `OTHER_ROW_TOPIC_ID` 只在本组件内使用；行头不绑定 `selectLane`，节点点击只写日期；单测断言不会 emit `selectLane`。
- **[同名事件跨话题混淆]**（两个候选话题各自有同名标题） → 标签仍显示标题（与正式行一致），事件卡显示 `topicLabelSnapshot`；单测覆盖两条不同话题同名事件都在「其他」行内且各自独立。
- **[读数口径回归]**（改动 `dayEventsByDate` 后旧测试断言「候选事件不计数」） → 这些断言正是本 change 要改的行为，测试按新 spec 改写并加交叉断言（行内节点数 == 头部条数）。
- **[与其他 in-progress change 冲突]** `key-event-day-focus-cards`（stage: implement）也 MODIFIED 同一个 Requirement 并新增了读数 Requirement → 本 change 的 delta 已把它的两段正文与两个 Scenario **一并纳入**（合并文本），并同步把它 delta 里的「候选事件 MUST NOT 呈现」改成「不单独占行、事件收进『其他』兜底行」；两 change 无论谁先归档都不会丢掉对方的契约。**归档顺序仍建议**：先归档 `key-event-day-focus-cards`（它的 delta 覆盖当前代码形态更完整），再归档本 change 重放合并文本。
- **[长窗性能]** 兜底行的事件数可能等于全窗未归属事件总数 → 与正式行同一套渲染（标签/卡片/测量），无额外请求；实测窗口内数据量在几十条以内。

## Migration Plan

纯前端展示层变更，无数据迁移、无接口兼容问题。回滚 = 还原两个前端文件；已确认事件与后端数据不受影响。

## Open Questions

（无）
