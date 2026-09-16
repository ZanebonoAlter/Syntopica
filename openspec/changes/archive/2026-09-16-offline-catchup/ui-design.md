<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

本 change 是纯后端行为变更（归档策略、边回收 job、日报补档与 API 守卫），无用户可见界面结构、文案或交互变化。前端 `generateDailyReport` 调用超窗时会收到后端 4xx 错误并走既有错误提示链路，无需前端改动。依据 proposal 的 `ui-impact: none` 声明，不制作原型。
