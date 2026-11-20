<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: topic-graph, daily-report, semantic-board -->

# 雷达总览不再把「没取到」显示成 0

## Why

叙事工坊总览（`/tags`）的数据来自单次 `GET /api/semantic-boards/overview`。端点有 5 秒硬预算，超时后会把**所有未算完的版块整行换成零值**并标 `data_delayed=true`（`board_radar_repository.go` 超时分支）。前端把这批零值当业务结论渲染，于是报头出现「0 个板块值得关注」「热点 0」「今日线索 0」，每行出现「关注强度 0.0」「0 篇文章 · 0 个事件」——**用户看到的是「今天全部清零」，实际是「这次没取到」**。

实测（2026-11-19，13 个活跃版块，连续 13 次调用）：3 次撞 5 秒预算（5.00s / 5.23s / 10.47s），其余 0.7–2.5s。撞线那几次的响应与用户截图逐字段一致（`report_date=""`、`attention=0`、`today_clues=0`、全行 `data_delayed=true`、顶层 `date` 回退到今天）。

需要澄清的一点：**「当天日报还没生成」本身不是问题**。各版块锚点日取自己的最新日报日（`workshop-radar` spec:68），11-19 无日报时页面本应显示 11-18 的真实数据（实测 13 板中 11 板有非零关注度，最高 16.5）。真正的问题是**超时降级把「没取到」写成了「就是 0」**。

同一处还有第二个缺陷：泳道动态计算失败的版块（`degraded`）返回的也是零值行，但连 `data_delayed` 都不标，前端无从区分「真的是 0」与「算不出来」。

## What Changes

**后端（`backend-go/internal/topicgraph/`）**

1. **未取到 ≠ 0**：把「未取到」从零值改成**可空 + 显式标记**。
   - `RadarBoardRow` 的计数类字段（`attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count`）改为可空（JSON `null` = 未知），正常行仍输出数字，协议对正常消费者无感。
   - 新增行级字段 `data_unavailable`：端点未取到该版块（超时未完成、泳道动态计算失败）时为 `true`。这同时补上了「计算失败」路径此前缺失的标记。
   - 正常空态版块（近 14 天无日报/无泳道）**不**标 `data_unavailable`，仍是完整的零值行（沿用 spec「无数据版块返回零值」）。
2. **治本：消掉超时这个触发器**。当前每版块 ~8 条串行 SQL、跨版块全表扫的候选 hint 查询（实测 ~237ms）使端点墙钟骑在 5 秒预算线上，DB 一忙就全体降级。本次按实测执行计划做查询/索引优化，**只改性能，不改任何统计口径**（关注度、线索数、分档阈值一律不变）。
3. **降级语义收敛**：超时未取到与计算失败两条路径统一标 `data_unavailable`，并保证「未取到」的版块**不参与**关注度分档（避免污染前端分档）。

**前端（`front/app/features/tags/`）**

4. 报头：有版块未取到时，标题**不再**说「0 个板块值得关注」，改「本次数据未取全」；全部未取到时改「本次数据未取全，请重试」。
5. 报头统计：当日线索合计在存在未取到版块时显示「—」（未知），不显示 0。
6. 版块行：未取到的行显示「数据未取到」与「—」，不显示「0 篇文章 · 0 个事件」「关注强度 0.0」；不再显示「数据延迟 · 截至 日期未提供」这种自相矛盾的文案。
7. 未取到的版块不参与关注度分档（不进热点/常温分档），单独以「数据未取到」状态呈现。

**滞后版块沉底（用户 2026-11-19 当场决定）**

8. **滞后版块默认逐行沉底到主列表最底部**：锚点日落后于全局锚点日的版块（活库实例：「刚果疫情新闻」锚点日 `2026-11-07`、全局 `2026-11-18`）当前按关注度混在前排（第 10 位，排在关注度 2.5 的版块之前），旧数据在前排占位、污染排名。改为：滞后版块**不参与关注度分档、不占前排位次**，逐行显示在列表最底部，保留既有「数据至 MM/DD」中性徽标；序号连续不重号。
9. **未取到的版块排在滞后版块之下（最底）**：行内显示「数据未取到」，不占任何名次分档。

**不改动**：关注度口径与阈值、泳道动态口径、日报生成与锚点语义、数据模型、任何统计数字（正常路径下响应数值与今天完全一致）。

## Capabilities

### Modified Capabilities
- `workshop-radar`：总览聚合端点的降级语义（未取到 ≠ 0）、报头与版块行在数据不完整时的呈现契约。

## Impact

- **后端**：`backend-go/internal/topicgraph/repository/board_radar_repository.go`（行结构 + 超时/降级分支）、必要时 `lane_snapshot_repository.go` 的慢查询、`backend-go/internal/platform/database/` 下的索引迁移（若需要）、`backend-go/internal/topicgraph/handler/board_radar_handler.go`（预算常量）。
- **前端**：`front/app/features/tags/composables/useBoardRadarOverview.ts`、`front/app/features/tags/components/console/boardRadarHelpers.ts`、`BoardRadarMasthead.vue`、`BoardRadarStory.vue`、`BoardRadarView.vue`、`BoardRadarCold.vue`（分段与接号）、`front/app/api/normalizers/boardRadar.ts` 与 `front/app/api/semanticBoards.ts`（类型可空化）。
- **协议**：`GET /api/semantic-boards/overview` 响应**新增** `data_unavailable`，且未取到时计数类字段输出 `null`。同仓同版本发布，前端归一化层已有「缺字段 → null」的既有通道。
- **文档**：`docs/reference/api/daily-reports.md`（端点契约段）、`openspec/specs/workshop-radar/spec.md`（delta）。
- **数据**：无模型变更、无数据迁移。若新增索引，走既有迁移机制（只加索引，不动数据）。
