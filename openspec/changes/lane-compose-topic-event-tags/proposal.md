# Proposal: 泳道编排态候选话题补充信息——已确认关键注记高亮 + 事件标签列表

<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report, reading -->

## Why

新建泳道编排态（workshop-lane-compose）中，候选话题目前只有标题 + 成员数，是"裸"的：识别倾向只能靠逐条看成员。用户已通过日报关键事件注记（topic_key_event_markers，confirmed）明确表达过对部分话题的倾向，且事件标签（category='event' 的 topic_tags）已在数据中存在，但编排态完全不透出。

## What Changes

给候选话题（persistent topic 维度）补充两层**纯展示**信息：

1. **已确认关键注记标记（★）**：候选话题附 `confirmedKeyEventCount` + 注记标题列表；画布话题带标题、左侧候选栏条目显示 ★ 徽标，hover 展示注记明细。人工主权信号的视觉呈现，与自动聚合信息（事件标签）在视觉上区分。
2. **事件标签列表**：服务端在候选链响应中按话题聚合"窗口内成员携带的 event 类标签，按出现条数降序"；画布话题带标题行内联 top 2 + 溢出计数（`+N`），hover 浮层显示全量（标签 ×次数）；左侧候选栏条目显示精简一行。

## Non-Goals

- 不做事件标签点击过滤/高亮成员散点（②过滤透镜）——本期纯展示，跑一阵再评估。
- 不改质心推荐、不参与选择计算（红线：推荐不是置信度）。
- 不在右栏 Inspector 出现（spec 既有边界：不得出现关联事件清单）。
- 不使用 related-events 端点 / 跨天关联（本轮探索结论：先做窗口内标签聚合）。
- 不改 topic_key_event_markers 任何写入路径。

## UI 方案（用户已选：混合 C）

行内放 top1-2 事件标签 + 溢出计数（即视性），hover 放全量事件标签 + 已确认关键注记明细（详情深度）。原型见 `ui-prototype/`，**须用户明确批准原型后才进实现**。

## Impact

- 后端：候选链端点（persistentTopics compose candidates）响应新增话题级聚合字段。
- 前端：`LaneComposeCanvas.vue` 话题带、`ComposeSidebar.vue`（编排态用法）、`useLaneCompose.ts` 类型。
- 部署后影响：编排态界面新增展示信息；无数据迁移、无旧数据降级问题（无关键注记/无事件标签的话题不显示对应区域）。
