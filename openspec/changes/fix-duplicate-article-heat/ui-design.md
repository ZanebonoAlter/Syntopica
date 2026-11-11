<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# ui-design: 重复文章副本导致的来源数/热度虚高（none 档 · N/A）

## N/A Reason

本 change 的 ui-impact 声明为 `none`（见 `proposal.md` 头部 `<!-- ui-impact: none -->`）：交付物是两类**后端口径修复**——关键字物化轨的日期穿透（补档时按报告日扫描）与参与者身份按内容身份去重（同一规范化链接的多个副本只算 1 个参与者），外加存量 signals/热度重算。

用户可见变化仅限**既有数值的语义修正**：雷达「今日续接」/事件线条目里的「N 个来源」与热度小数、板块关注度（lane_heat）、矩阵颜色分档所依据的参与者数——字段、组件、布局、入口、交互模式全部不变，前端不改任何文件。因此：

- 无新增/变更界面入口 → 无「入口与入口变更」契约；
- 无 loading/empty/error/success 状态新增 → 无状态矩阵；
- 无新组件、无宽度/尺寸档、无布局模式变化 → 无布局契约与组件复用映射；
- 不制作原型（`ui-prototype: none`），不需要用户审批（`ui-approval: not-required`）。

若后续要把「来源明细（哪些 feed 是同一篇副本）」做成可见面板，届时另开 change 并把 ui-impact 升到 `minor`，本 change 不预留界面契约。
