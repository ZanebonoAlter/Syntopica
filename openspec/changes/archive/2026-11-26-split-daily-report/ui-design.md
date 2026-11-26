<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是**纯文档拆分**，零 UI 变更：

- 只改 `openspec/specs/` 的文件边界（1 个巨 spec 拆 3 份 + 2 个散件归位），不产生界面元素
- 不触碰 `front/`、`backend-go/`，零代码改动
- `daily-report-interface` 会装进 12 条前端契约（报纸布局、空态、全屏层、状态块），但**契约文本逐字搬运**——字段、交互、文案、层级全部不变
- 之所以不是 `minor`：minor 档要求「复用契约 + 挂验收证据」，而本次没有任何 UI 行为需要验收。契约从文件 A 搬到文件 B 不构成 UI 变更（与 `merge-topic-specs` 同口径）
