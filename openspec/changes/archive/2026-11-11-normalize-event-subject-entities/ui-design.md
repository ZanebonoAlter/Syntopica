<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# ui-design: 主体实体归一（none 档 · N/A）

## N/A Reason

本 change 的 ui-impact 声明为 `none`（见 `proposal.md` 头部 `<!-- ui-impact: none -->`）：交付物是**后端实体身份层 + 主体提及归一服务 + 影子/评测 CLI**，不新增页面、不新增面板/弹窗/导航项、不改既有交互模式，也不改任何既有界面的展示字段来源。

用户可见行为**零变化**：日报、雷达、事件线、标签面板、话题编排的读出路径全部不变（默认影子模式 `off`，实体层不被任何界面消费）。因此：

- 无界面入口 → 无「入口与入口变更」契约；
- 无 loading/empty/error/success 状态新增 → 无状态矩阵；
- 无新组件、无自由宽度、无 dialog 尺寸档选择 → 无布局契约与组件复用映射；
- 不制作原型（`ui-prototype: none`），不需要用户审批（`ui-approval: not-required`）。

唯一与「界面可见」沾边的地方是 `topic_tags.metadata.entity_subject` 这个**后端 metadata 键**（第一轮稳定短身份说明的落点）——它不是界面字段，当前后端也无任何读路径消费它（见 `design.md` D2 的「影子期只写不读」边界）。若后续 change 要把归一结果呈现在界面上（实体档案页、别名管理、误合复核队列），届时另开 change 并把 ui-impact 升到 `minor`/`major`，本 change 不预留界面契约。
