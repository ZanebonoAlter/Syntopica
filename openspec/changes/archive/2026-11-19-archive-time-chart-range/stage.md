<!-- stage: implement -->
<!-- constraint-domains: semantic-board -->

# stage — archive-time-chart-range

归档视图时间筛选改造：默认日历今天 + 90 天折线视窗（复用矩阵时间视窗手势）+ 全历史导航条 + 后端按日计数端点。

- 阶段：实现（制品 proposal / ui-design / specs / design / tasks 已就绪，`openspec validate` 通过）。
- 业务域：`semantic-board`（版块归档与文章匹配证据展示）。
- ui-impact: minor（复用既有交互契约，无新页面/导航/信息架构变化）；complexity: complex（新增端点 + 视窗/选区双层状态）。
