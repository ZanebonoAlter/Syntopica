<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 为纯后端调度编排变更（日报 job 尾部追加 lifeline 归档补齐 tail step），不触及任何前端路径、API 契约或用户可见界面——月/年报消费方 `listContexts` API 行为不变，仅数据及时性恢复。依据 proposal 头部 `ui-impact: none` 声明，无需 UI 设计契约与原型。
