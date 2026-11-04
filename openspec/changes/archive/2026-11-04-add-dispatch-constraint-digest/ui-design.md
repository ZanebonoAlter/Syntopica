<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 仅新增 harness 取数脚本、修订规范文档、给事实库事件追加两个字段，不改任何前端页面、组件、路由或交互，无用户可见界面变化。

## Acceptance

以 smoke 脚本对账「红线提取与扩展实现逐字节一致」「路径反查命中/无命中/多域」三类行为；规范引导的落地效果以 `harness-retro` 的「派发带域比例」观察窗口回检，不以脚本退出 0 宣称主线程已实际注入。
