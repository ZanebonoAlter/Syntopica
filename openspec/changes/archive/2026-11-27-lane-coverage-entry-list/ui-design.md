<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

## 入口与入口变更

| 入口 | 位置 | 变更 |
| --- | --- | --- |
| 「窗口内未进日报」折叠条 | `ObservationMatrixGrid`：`.omx-scroll`（横滚容器）**之外**、`.omx-matrix` 内、休眠条正下方 | **改为**原生 `<button>`（与休眠折叠条同款 `omx-group-summary` / `omx-group-btn`，`aria-expanded`）：chevron +「窗口内未进日报 · N」+ 泳道名清单（超长截断，title/aria 保留全文）+ 右侧「展开查看 / 收起」；**默认折叠**。泳道名在折叠态也常显（「不以缺席表达沉默」不变） |
| 点击折叠条 → 列表 | 紧随折叠条 | **新增**：`<ul data-testid="coverage-list">`，每项 = 泳道名 +「最近命中 MM-DD」/「无记录」+ 行尾 ⋯；是普通列表，**没有日期格、不是网格行** |
| 列表项 ⋯ 菜单 | `ObservationMatrixLaneRow` 的 `coverage` 分支（`cells=[]`，只有行头） | 不变：重命名 / 编辑泳道 / 归档泳道；重命名 = 行内输入框（Enter 保存 / Esc·失焦取消） |
| 覆盖清单失败行 | 与折叠条同一位置 | 行为不变（「窗口外泳道清单未取到」+「重试」），只跟着从 `.omx-scroll` 内挪到新位置 |
| 矩阵网格行 | `ObservationMatrixGrid` 的 `.omx-grid` | **移除**：覆盖组不再渲染任何 `ObservationMatrixLaneRow` 网格行与逐日「—」格 |

## 受影响状态

| 状态 | 影响 |
| --- | --- |
| loading | 不变：覆盖清单读取中不渲染折叠条与列表（不占位） |
| empty | 不变：清单成功且为空 → 折叠条与列表都不渲染（MUST NOT 显示「· 0」空组） |
| error | 不变：折叠条位置只出一行「窗口外泳道清单未取到 · 重试」；矩阵行 / 格 / 休眠组照常 |
| success · 折叠（默认） | 只有一条折叠条：▸ +「窗口内未进日报 · N」+ 名字清单 +「展开查看」，`aria-expanded="false"`；网格内没有这些泳道的行 |
| success · 展开 | ▾ +「收起」，`aria-expanded="true"`；折叠条下方出列表 N 项；项内名字是静态标签（不可选中、不触发行选中）；⋯ 可用；矩阵行与其他组不动 |
| 改名（列表项内） | 与矩阵行头一致：输入框替换名字，Enter 保存；失败保留输入并 toast；成功退出编辑并刷新名单与列表（展开态保持） |
| 编辑 / 归档（列表项 ⋯） | 与矩阵行头一致：走既有 `editLane` / `archiveLane`；MUST NOT 改变选中泳道或选中日 |

## 复用组件与布局模式

- 布局模式：**不变**（L1 版块态势固定视口栅格例外，矩阵舞台内部横滚）。折叠条与列表放在 `.omx-scroll` 之外：宽度 = 舞台可见宽（不随横滚漂移），也不受 `.omx-scroll` 的 `overflow-y: hidden` 裁切。
- 复用：`omx-group-summary` / `omx-group-btn` / `omx-group-title` / `omx-group-count` / `omx-group-note` / `omx-group-noterow` 样式族（Grid scoped，同组件内直接用，不抽共享 css）；`ObservationMatrixLaneRow` 的 `coverage` 分支 + `RowActionMenu`（菜单 Teleport 到 body，不怕滚动容器）+ 名称看全卡；`observationMatrixRows.css` 的 `.omx-lane` / `.omx-lane-name-static` / `.omx-lane-rename-input`；`Icon` chevron。
- **不新增**基础组件、色板、弹窗、浮层。列表只补三条 Grid 本地样式：`ul` reset、行间点线分隔、列表内 `.omx-lane` 修正（`position: static`、`cursor: default`——行头的 sticky/手型光标是为网格准备的，在列表里既无意义又误导）；列表项里新出现的可见标签「最近命中 MM-DD」（`.omx-lane-coverage-seen`）由 `ObservationMatrixLaneRow` 渲染，样式随元素放在 `observationMatrixRows.css`，与同形态的 `.omx-lane-dormant-tag` 并列（元素在哪、样式在哪）。
- 文案：只放状态结论与动作标签——折叠条「展开查看 / 收起」、项内「最近命中 MM-DD」「无记录」；MUST NOT 仿写休眠条右侧那句解释。
- 窄屏：L1 窄屏本就不渲染矩阵（既有降级），不新增窄屏形态。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| D1 | 窗口内未进日报的泳道：矩阵网格外的折叠条入口（常显名字），点击就地展开/收起一个无日期格的列表；列表项写「最近命中 MM-DD」 | `panel` | `dialog`, `drawer`, `page`, 默认展开, 网格行 / 日期格, 项内写「最近一次」 | `pinned`（用户 2026-11-27 选定方案 B「挪出网格，入口 + 列表」；「最近命中」为主线程核实 `last_seen_date` 语义后定） |
| D2 | 重命名：列表项 ⋯ →「重命名」→ 行内输入框（Enter 保存 / Esc·失焦取消） | `inline-edit` | `dialog`, `page` | `pinned`（沿用 lane-matrix-coverage-and-others-row D2） |

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 折叠条数量 N / 名字清单 / 列表项名字 | `api GET /semantic-boards/:id/topics`（既有，`useDailyReportsApi.listBoardTopics`） | 只取 `status === 'active'` 且不在 A 档行里的（`coverageLanes.ts`）；零新增接口 |
| 列表项「最近命中 MM-DD」/「无记录」 | 同上 `last_seen_date` | 话题最近一次被线索命中的日期（算法侧命中计数），不是进日报的日期；空串 → 「无记录」 |
| 改名结果 | `api PATCH /daily-reports/topics/:id`（既有） | 与矩阵行头同一条写入路径；成功后重取 A 档与覆盖清单 |

## Prototype

无需原型（minor 档：折叠条、列表、⋯ 菜单、就地改名全部是既有模式，本次只是换载体）。

## Acceptance

- 组件测试（真实路径）：
  - `front/app/features/tags/components/console/ObservationMatrixGrid.test.ts`：默认折叠只有条（无行无格）、点条展开 N 项（名 + 最近命中 / 无记录）、项 ⋯ 三项菜单且不 emit `selectLane`、失败行 + 重试、读取中 / 空不渲染；
  - `front/app/features/tags/components/console/ObservationMatrix.test.ts`：展开后才有列表项、错误行、空 / 读取中不渲染；
  - `front/app/features/tags/components/console/ObservationMatrixLaneRow.test.ts`：`coverage` 行头文案「最近命中 MM-DD」「无记录」、`cells=[]` 不渲染格；
  - `front/app/features/tags/components/console/BoardConsoleShell.integration.test.ts`：板块话题清单 → 折叠条名字 → 展开后列表；列表项 ⋯ → 重命名 → Enter 走 `updateTopic`。
- 命令：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrixGrid.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/ObservationMatrixLaneRow.test.ts app/features/tags/components/console/BoardConsoleShell.integration.test.ts --maxWorkers=2` → 全绿。
- 视觉：本次起因就是「看着突兀」，故加做 agent-browser 走查并在 `ui-verification/report.md` 建账——折叠态（1440×900 与 1920×1080 各一张：矩阵底部只有一条折叠条，没有整行「—」）、展开态一张（列表项 = 名 + 最近命中 + ⋯）、⋯ 菜单打开一张；列表不产生页面级横向溢出。
