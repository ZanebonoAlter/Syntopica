<!-- ui-impact: none -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## N/A Reason

纯后端判定逻辑变更（方向校验加全库最优豁免层），前端零代码改动：日报过滤、体检 ghost 统计、`BoardHealthPanel.vue` 等展示均消费同一 `direction_mismatch` flag，改源头全链路自动受益。用户可见的变化仅为数据层面（体检评分回升、幽灵计数下降、日报内容回归），不涉及任何 UI 结构/交互/状态变化，与 proposal 的 `ui-impact: none` 声明一致。
