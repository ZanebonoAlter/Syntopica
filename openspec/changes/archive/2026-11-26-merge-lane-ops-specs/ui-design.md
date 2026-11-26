<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯文档结构重构**，零 UI 变更：

- 只改 `openspec/specs/` 的文件边界（6 → 2），不产生界面元素
- 不触碰 `front/`、`backend-go/`，零代码改动
- 搬运的条款含大量界面契约（雷达首屏、档案视图、动态卡片、趋势区块），但**契约文本逐字不改**——布局、字段、跳转联动全部原样
- 之所以不是 `minor`：minor 档要求「复用契约 + 挂验收证据」，本次没有任何新 UI 行为需要验收（与 `merge-topic-specs` / `split-daily-report` 同口径）
