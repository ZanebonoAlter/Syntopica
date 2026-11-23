<!-- stage: verify -->
<!-- constraint-domains: semantic-board, daily-report -->

# lane-compose-topic-event-tags — 阶段载体

- 当前阶段：**design**（proposal / ui-design / specs / design 已落盘；tasks 落盘后进 apply）。
- **范围已收窄（2026-11-21 用户拍板）**：只做「已确认关键线索 ★」一层。
  - 保留：事件节点 ★ 角标（布尔）、节点卡逐条明细、左栏成员条目 ★。
  - 砍掉：事件标签列表、同事件虚线环（实测同话题标签交集有候选仅 1.6%，母文档约束 55 已禁）。
- **旧原型作废**：`ui-prototype/index.html` + `screenshots/` 基于已删除的「话题带 + 勾选」画布，不参与验收、不作为几何基准；几何基准 = `rework-lane-compose-swimlane-canvas/ui-prototype/states/02-editing.svg`。
- **追加范围（2026-11-23 用户定档）**：隐藏工具条两个入口——「显示已有泳道（只读）」与「历史待启用话题」；`showAssigned` 恒 false，历史组件/composable 保留但不可达。
- 数据口径：后端 `confirmed_key_events` 按 `source_section_id` 单查（不 join thread）；空数组不给 null；指针 NULL 的脱离快照不显示、不模糊补挂。
- 前端落点：`LaneComposeCanvas.vue`（角标 + 卡内）、`LaneComposeCandidates.vue`（左栏 ★）、`persistentTopics.ts`（类型/归一）。
- 用户拍板记录（2026-11-21 对话）：节点**换成星形**（不是圆点 + 小角标）；星形不带数字；卡内列全部；左栏与右栏②都补 ★（左栏日期行右端 / 右栏日期行首）。
- 约束域声明与 `proposal.md` 头部并集一致；实现期发现新域在本文件头部补（只增不减）。
