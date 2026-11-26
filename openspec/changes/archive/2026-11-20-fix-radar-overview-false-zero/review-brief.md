# review brief — fix-radar-overview-false-zero

## 被审对象

- change 目录：`openspec/changes/fix-radar-overview-false-zero/`
- 制品：`proposal.md` / `design.md` / `ui-design.md` / `specs/workshop-radar/spec.md` / `tasks.md`
- 代码 diff（工作树未提交）：后端 `backend-go/internal/topicgraph/**`（`board_radar_repository.go`、`lane_snapshot_repository.go`、`repository.go`、`board_radar_batch_test.go`）+ 前端 `front/app/features/tags/**` + `front/app/api/**` + 文档 `docs/reference/api/daily-reports.md`、`docs/reference/flow/semantic-board.md`

## 本 change 想解决什么（用户视角）

1. **假 0**：总览端点 5 秒预算超时后，把没算完的版块整行返回零值，前端渲染成「0 个板块值得关注」「热点 0」「今日线索 0」「关注强度 0.0」——用户以为今天全清零，实际是「这次没取到」。
2. **滞后污染排名**：「刚果疫情新闻」锚点日 11-07（全局 11-18），关注度 3.0 却占主列表第 10 位，排在关注度 2.5 的版块前面——用户 2026-11-19 当场要求「默认放到最下面，排名不要在前面污染」。

## 关键契约（review 时按这些逐条核）

- 后端：未取到（超时未完成 / 泳道动态计算失败）→ 行级 `data_unavailable=true`，且 `attention`/`today_clues`/`last3_clues`/`prev3_clues`/`report_article_count`/`report_event_count` 输出 `null`；**真实零值**（近 14 天无日报无泳道）仍是 0 且 `data_unavailable=false`。
- 后端：全部行未取到 → 顶层 `date` 为空串（不冒充今天）；真没日报 → 仍回退今天（既有行为不变）。
- 后端：性能改造**不得改动任何统计口径**（关注度公式、三窗线索、14 天轴、候选权重、分档阈值、排序 tie-break）。
- 前端：未取到的行不进 hot/warm/cold 分档、不计入「N 个板块值得关注」；热点数与当日线索合计在存在未取到版块时为未知（显示「—」）；行内显示「数据未取到」，不显示 0.0 / 0 篇文章 / 无日报日期；不得渲染「今天没有需要看的版块」业务空态。
- 前端：滞后版块（自身锚点日 ≠ 全局锚点日；锚点日缺失**不算**滞后）不进分档、不占前排名次，逐行沉底到末尾并保留「数据至 MM/DD」；未取到的行排在滞后行之下；序号跨段连续不重号。

## 请重点查（High 优先）

1. **有没有任何路径把「未知」写成 0**（后端 nil→0、前端 `?? 0` / `|| 0` / `num()` 兜底）。
2. **真实零值路径有没有被误伤**（无日报/无泳道的版块应仍输出 0、仍进常温）。
3. **口径有没有被性能改造带偏**：批量路径与单版块路径是否真的共用同一份聚合代码（而不是复制一份可能漂移的实现）；批量 anchor SQL 的 `status='completed'` 过滤是否与 `GetBoardReportAnchor` 逐字一致；最新日报摘要是 `DISTINCT ON`/窗口函数取「每版块最新一行」，是否与单版块 `ORDER BY period_date DESC LIMIT 1` 等价（注意 `GetLatestReportBrief` **不限 status**）。
4. **ctx 取消**：`context canceled` / `deadline exceeded` 是否被误判成「该版块真的没数据」而输出 0（应为未取到）。
5. **前端 null 安全性**：可空化后是否还有地方把 null 当 0（分档比较、行序、`toFixed`、排序比较里出 NaN）。
6. **排序与序号**：滞后/未取到两段的接号是否可能重号或跳号；`storyRows` 与视图渲染数组是否一致（别出现「行渲染了但没进 storyRows」或反之）。
7. **raw SQL 参数化**：新增的两条批量 SQL 是否用占位符绑定（不得字符串拼 SQL）。
8. **SQLite 兼容**：`ROW_NUMBER()` 窗口函数在内存 SQLite 测试下是否可用（既有 service 包测试跑 SQLite）。
9. **既有行为回归**：既有用例的**断言值**有没有被改动（只允许改断言写法，如指针解引用）；`data_delayed` 语义有没有被挪用。
10. **并发脏文件**：工作区有其它 active change 的未提交改动（`front/.../BoardRadarStory.vue` 的 `RowActionMenu`、`internal/tagmanagement/*`、`internal/discovery/service/*` 等）——本 change 的 diff 是否误伤/回退了它们。

## 输出要求

- 中文报告，按 **High / Medium / Low** 分类；每条给 `file:line` + 问题 + 建议改法。
- High = bug / 安全 / 清晰错误 / 契约违反（必改）；Medium = 合理但依赖上下文；Low = 误报可忽略。
- 「审毕无发现」也要显式写（写明审查范围）。
- **不要**直接改代码（只读审查）。
