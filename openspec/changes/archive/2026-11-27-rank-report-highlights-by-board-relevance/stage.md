<!-- stage: implement -->
<!-- constraint-domains: daily-report -->

# stage — rank-report-highlights-by-board-relevance

业务域：`daily-report`（今日重点选取与排序；泳道分带 lane_tier 的定义与不变量都在 `docs/reference/flow/daily-report.md`）。

> 本文件是 change 的**阶段载体**（非 schema 制品）。头部 `<!-- stage: -->` 是 constraint-injection extension 的进度真相源；`constraint-domains` 与 `proposal.md` 头部的声明取并集，本文件只增不减。

## 探索结论去哪了

两轮探索的事实（链路、可用信号、15 期历史回放、改动会碰的测试与规格）落在 change 根的 `explore-findings.md`，实现与审查阶段自动注入。
