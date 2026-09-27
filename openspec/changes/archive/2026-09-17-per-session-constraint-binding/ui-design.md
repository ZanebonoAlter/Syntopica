<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 只改 `.pi/extensions/constraint-injection.ts`（pi 扩展，harness 工具链）与其 smoke 用例：把进程级全局的档位绑定状态改为按 `sessionId` 隔离。不改任何前端页面、组件、接口契约或交互。

用户可见的唯一差别是**注入内容不再串味**（本会话只拿到自己 change 的约束），属于 harness 行为修正，不是 UI 变更。
