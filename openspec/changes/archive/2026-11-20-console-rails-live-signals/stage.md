<!-- stage: implement -->
<!-- constraint-domains: semantic-board, daily-report -->

# stage · console-rails-live-signals

当前阶段：implement（proposal / design / specs delta / ui-design / ui-prototype 已落；原型 2026-11-19 经用户明确批准，`ui-approval: approved`）。

实现期口径要点（来自 design.md D1–D11）：
- 左轨条码 = `overview.boards[].clue_days`，**共同轴重对齐**（`位置 = i - offset`，offset = 全局锚点 − 该版块锚点），落在共同轴外 = 未取到（斜纹），MUST NOT 补 0。
- 右轨「当日日报」跟随**矩阵选中日**（唯一日期源），右轨内 MUST NOT 有日期选择器。
- 点评层 = `report.highlights`（2–3 条），MUST NOT 重复展示 `report.title` / `report.summary`（后端派生自首条重点）。
- 泳道层 = 报告 active 分区 ∩ lane-dynamics 正式泳道，句子取 `threads[0].title`（无则 `cluster_label`），线索数取当日 `clue_count`，**按线索数降序**（并列保持日报原序）。
- 两栏满高自适应；右轨「当日日报」块撑开剩余高度。


实现期若发现新的业务域（例如读链改动涉及 reading），在头部 `constraint-domains` 追加，只增不减。
