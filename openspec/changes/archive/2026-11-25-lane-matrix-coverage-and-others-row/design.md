# lane-matrix-coverage-and-others-row — 实施设计

## 1. 问题与边界

用户看版三个问题，根因与归属：

| 问题 | 根因 | 归属层 |
| --- | --- | --- |
| 新建泳道在矩阵里看不到 | `lane-dynamics` 只为「窗口内有锚定 section」的话题生成行（「沉寂不展示」），窗口外/新建的 active 泳道整行缺席 | 前端补齐（不改 API 语义） |
| 没法改泳道名 | 唯一入口在修正工作面标题（点文字才变输入框），矩阵行 ⋯ 没有 | 前端（复用既有 PATCH） |
| 其他话题要一天一天点 | 「当日其他话题」只按所选日报详情渲染，跨日无读数 | 后端补每日聚合 + 前端一行 |

**不做**：不改 `lanes` 返回集合（雷达/候选/右轨名单不变）、不改话题状态机与归属、不加页面/弹窗/依赖/迁移。

## 2. 数据流

```
GET /semantic-boards/:id/topics ──┐
                                  ├─> buildOutsideWindowLanes(rows, topics) ─> ObservationMatrix 覆盖组
GET /lane-dynamics?days=N ────────┘                                          （行头 ⋯：改名/编辑/归档）

GET /lane-dynamics?days=N
  └─ other_topics[] ─> normalizeLaneDynamicsWindow ─> LaneDynamicsWindowData.otherTopics
        └─> ObservationMatrix「其他话题（未入泳道）」整行
              └─ 点格 → selectOthersDay(date) → selectedDate + othersOpen=true
```

## 3. 后端：`other_topics` 逐日聚合

### 3.1 契约

```jsonc
{
  "window_days": 14,
  "anchor_date": "2026-11-24",
  "lanes": [ /* 不变 */ ],
  "other_topics": [               // 窗口内每个 completed 日报日一条，date 升序；含 0
    { "date": "2026-11-11", "sections": 2, "clues": 5 },
    { "date": "2026-11-12", "sections": 0, "clues": 0 }
  ]
}
```

- `sections` = 该日「未入泳道」section 数；`clues` = 这些 section 的非综合稿 thread 数之和。
- 「未入泳道」= `persistent_topic_id IS NULL` **或** 不属于该板块 `active ∪ watch 关联`（且**非归档**）的话题集合（与前端 `dayOtherTopics` 的 A 名单排除一致；归档话题即使仍被 watch 指向也不在泳道集合里，其分区计入）。
- 只用 `status = completed` 的日报（与泳道时间线同边界）；无日报日不出条目。
- 旧客户端忽略该字段；旧后端不下发时前端不渲染整行。

### 3.2 实现点

- `LaneDynamicsResponse` 增加 `OtherTopics []LaneDynamicsOtherDay`（json `other_topics`），空数组非 `null`（与 lanes/candidates 空态契约一致）；无锚点时返回 `[]`。
- `computeBoardLaneDynamics` 在完成 lane 聚合后调用一次新查询 `listOtherTopicsWindow(boardID, from, anchor, laneTopicIDs)`：
  - 先由 `level.topicByID` + `watchLinked` 收集**全部** active/watch 话题 ID（不只是本窗有 section 的），作为排除集；
  - SQL 一次聚合（PG，仓库测试用 testcontainer PG）：
    ```sql
    SELECT r.period_date AS date,
           COUNT(DISTINCT s.id) AS sections,
           COALESCE(SUM(CASE WHEN t.id IS NOT NULL AND COALESCE(t.scope, '') <> 'composite' THEN 1 ELSE 0 END), 0) AS clues
    FROM daily_report_sections s
    JOIN board_daily_reports r ON r.id = s.report_id
    LEFT JOIN daily_report_threads t ON t.section_id = s.id
    WHERE r.semantic_board_id = ? AND r.status = 'completed'
      AND r.period_date >= ? AND r.period_date <= ?
      AND (s.persistent_topic_id IS NULL OR s.persistent_topic_id NOT IN ?)
    GROUP BY r.period_date
    ORDER BY r.period_date ASC
    ```
    排除集为空时不加 `NOT IN` 条件（避免 `NOT IN ()` 语法差异）；
  - 为「有日报但 0 个其他分区」的日子补 `{sections:0, clues:0}`：用同窗口 `SELECT DISTINCT period_date`（completed）作为日期骨架，SQL 结果填充。两次小查询或一次左连接骨架都可，取简单实现。
  - 失败策略：与 lane 主查询一致——返回错误（端点 500），不静默 0；前端保持旧数据（stale 语义）。**不**降级为「0 条其他话题」（会造假）。

### 3.3 为什么不在前端算

前端只有所选日的完整报告详情；用日报列表 `cluster_count` 只能得到「分区数」，拿不到非综合稿线索数，且需第二套排除逻辑。后端一次聚合与矩阵格同口径、零前端推算，符合「口径代码只有一份」。

## 4. 前端

### 4.1 覆盖组（窗口内未进日报）

- 新纯模块 `coverageLanes.ts`：
  ```ts
  export interface CoverageLaneRow { topicId: string, label: string, lastSeenDate: string | null }
  export function buildCoverageLanes(rows: ObservationLaneRow[], topics: BoardTopicListItem[]): CoverageLaneRow[]
  ```
  过滤 `status === 'active'`、`String(id)` 不在 rows、按 `last_seen_date` 降序 → label → id；`last_seen_date` 空串 → null（显示「无记录」）。
- `useConsoleShellData` 新增 `boardTopics / topicsLoading / topicsError / loadBoardTopics`（seq 防晚到、切板复位、失败保留旧值？——覆盖清单失败时保留旧值会撒谎成旧状态；采用「失败清空 + error」更诚实：组退化为一行重试提示。选择：失败清空并标 error）。
- 壳层 `renameLane(topicId, label)`：
  - `Number(topicId)` 校验 → `updateTopic(id, {label: trimmed})` → 失败 `notifyError` 返回 false；成功 `notifySuccess('泳道名已更新')` + `Promise.all([load(), loadBoardTopics()])` 返回 true。
- `consoleStageBinding` 透传 `coverageLanes / coverageLoading / coverageError / renameLane / retryCoverage`。
- `ObservationMatrixGrid`：
  - 活跃行 → 其他话题整行 → 当日其他话题组 → 休眠组 → **覆盖组**；
  - 覆盖组 summary：`窗口内未进日报 · N` + 名字列表 + 「最近一次 MM-DD」；默认展开（无折叠态，行数少；若未来多可再加折叠）；
  - 覆盖行复用 `ObservationMatrixLaneRow` 的 `dormant` 样式 + `未进日报` 标签（新增 prop `coverage?: boolean` 或复用 `dormant` + `tag` prop）；
  - 格的投影：覆盖行的 `topicId` 不在 A 档索引，`resolveLaneDay` 对档窗内日期返回 `gap`（正确显示「—」）；若可见轴延伸进 365 池且池里确有该泳道数据，则按既有语义显示真实值（池未取到仍为 `?`），不另造数。无需特判。但 `project()` 的 `rowMax` 全 0 → 不着色。
- 覆盖行名字为**静态标签**（不起 `selectLane`：该泳道不在本窗名单，选了也无可读内容，反而会清掉其他话题阅读槽）；⋯ 菜单保留。
- 覆盖清单失败：底部一行 `.omx-inline` + 重试按钮（emit `retryCoverage`）。

### 4.2 行头重命名

- `ObservationMatrixLaneRow` 菜单加 `{ key: 'rename', label: '重命名' }`（第一项）。
- 行内状态：`editing / draft / saving / error`；编辑时名字按钮换成 `<input>`（自动 focus+select），Enter 调 `props.renameLane?.(topicId, draft)`；`false` → 保留编辑 + `notifyError`；`true` → 退出编辑（名单刷新由壳层做）。Esc / blur 取消（saving 中不取消）。
- `renameLane` 作为可选函数 prop 透传（`ObservationMatrixLaneRow` ← `ObservationMatrixGrid` ← `ObservationMatrix` ← `BoardConsoleStage`/`BoardWideWorkface` ← binding）；缺省时不显示「重命名」项（老调用/测试兼容）。

### 4.3 其他话题整行

- normalizer：`LaneDynamicsOtherDay { date, sections, clues }`；`LaneDynamicsWindowData.otherTopics: Record<string, LaneDynamicsOtherDay>`（无字段 → 空对象 + 标记 `otherTopicsAvailable: boolean` 以区分「旧后端」与「有字段但空」）。
- `ObservationMatrixOtherRow.vue`（多根，风格同 lane row）：
  - 行头：`其他话题` + 小标「未入泳道」；title「未进入任何泳道的分区线索数」；
  - 逐日格：`otherTopics[date]` 存在 → 文本 `clues`（0 合法，可点）、title `${date} · ${sections} 个分区 · ${clues} 条线索`；不存在 → `—` 禁用：若 `date < otherTopicsSince`（档窗起点）title 写「档窗外，其他话题读数未取到」，否则写「无日报」；
  - 整行只在 `otherTopicsAvailable === true && Object.keys(otherTopics).length > 0` 时渲染（无任何 completed 日报时后端给空数组 → 隐藏，不显示整行「—」）；
  - 色深：整行 `rowMax = max(clues)` → `clueDepth`，与泳道一致；
  - 选中日列淡底：`is-col-selected` 与泳道格一致（有数据日）。
  - 点击：emit `selectOthersDay(date)`。
- `ObservationMatrixGrid` 在 active 行后渲染该行（仅在 `otherTopicsAvailable === true`）。
- 壳层接线 `selectOthersDayFromStage(date)`：`other.clearOther()` + `selection.selectedDate.value = date` + `other.othersOpen.value = true`（受控写同一份状态，不新增 selectedDate）。

## 5. 边界与诚实降级

| 边界 | 处理 |
| --- | --- |
| 覆盖清单读取失败 | 底部一行提示 + 重试；不冒「无覆盖」 |
| 覆盖泳道 last_seen_date 缺失 | 显示「无记录」 |
| 窗口内无日报（empty no-reports） | 覆盖组仍可显示（新建泳道可能确实没进过任何日报）；矩阵空态行照旧 |
| 覆盖行点格 | 全为「—」禁用，给出「无分区」提示（复用既有格子禁用提示） |
| 旧后端无 `other_topics` | 整行隐藏；不显示空壳 |
| 无任何 completed 日报（`other_topics: []`） | 整行隐藏（不显示全「—」） |
| 可见轴拖到 365 池区段 | `other_topics` 只覆盖档窗；档窗外格「—」+ title「档窗外，其他话题读数未取到」，不谎称无日报、不补 0 |
| `other_topics` 无某日报日（0 其他话题） | 后端补 0 条目；若字段存在某日但前端轴上没有 → 不渲染 |
| 改名空值/失败 | 保持编辑态 + 如实报错；不改本地显示名 |
| 改名与覆盖组 | 覆盖组 label 来自 topics 清单，成功改名后一并刷新 |
| 窄屏 | 矩阵本就不渲染，本次无新增窄屏形态 |

## 6. 测试策略（用例先行）

- 后端（repository 集成，testcontainer PG）：
  1. 未入泳道 0 值日仍出条目；
  2. 归属 active 的分区不计入、candidate/archived/null 计入；
  3. 综合稿只计入 thread 总数不计 clue（造 1 clue + 1 composite → clues=1）；
  4. 非 completed 日报整体排除；
  5. 无锚点 → `other_topics = []`。
- 前端：
  1. `coverageLanes.test.ts`：active 过滤、去重、last_seen 排序/缺失；
  2. `laneDynamicsWindow.test.ts`（既有）：normalizer 三态（缺字段/空数组/有值）+ 键值；
  3. `ObservationMatrix.test.ts`：覆盖组默认展开、失败提示与重试、其他话题行点击 emit；
  4. `ObservationMatrixGrid.test.ts`：其他话题行逐日格（0/—/可点）、旧响应隐藏、休眠 summary 名字；
  5. `ObservationMatrixLaneRow.test.ts`：菜单项、Enter 保存、Esc 取消、空名、失败保留。

## 7. 文档影响

- `docs/reference/flow/semantic-board.md`：矩阵新增覆盖组/其他话题行的业务约束与入口。
- `docs/reference/flow/daily-report.md`：lane-dynamics 的 `other_topics` 口径（如该文档登记了端点字段）。
- `docs/reference/api/semantic-boards.md`：响应字段 `other_topics` 与场景。
- 变更溯源按 §12 在 flow 文档追加本 change 链接。
