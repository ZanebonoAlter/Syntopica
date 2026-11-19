<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是 harness 工具链内部改造（`constraint-injection` 扩展的注入形态与进度标识机制），**不触及任何用户可见界面**：不新增/修改页面、组件、路由、交互模式或界面文字。注入内容出现在**模型上下文**而非用户界面，用户在 UI 上感知不到任何差异。

依据：proposal.md 头部的 ui-impact 声明为 none（与本文件 marker 一致），且 Impact 节已列明「无前端/后端业务代码改动，不动 `front/` 与 `backend-go/`」。

- 无需原型。
- 验收映射：N/A（验收走 smoke test + 归档对账脚本，见 tasks.md 验证节）。
