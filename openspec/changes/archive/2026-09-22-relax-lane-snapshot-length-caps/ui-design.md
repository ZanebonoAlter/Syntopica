# UI Design: relax-lane-snapshot-length-caps

<!-- ui-impact: none -->

**N/A**：本 change 仅放宽后端生成侧字数上限（常量 + prompt + max_tokens），不改任何 UI 结构、交互或样式。前端泳道趋势区（LaneTrendOverview）本就按"全文展示不截断"设计（区块内自然滚动），新上限 200/1000 字在该滚动容器内正常容纳，无需 ui-prototype。

## N/A Reason

- 无新 UI 面、无交互/布局/样式变更，ui-impact: none；字数上限放宽只改变既有文本容器内的内容长度，容器行为（自然滚动）不变。
- 前端展示层零改动（本 change 无任何 front/ 代码变更），无需双视口验收。
