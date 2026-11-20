<!-- ui-impact: minor -->
<!-- ui-approval: not-required -->
<!-- ui-prototype: none -->

# UI 设计（minor）

在既有叙事工坊总览页（`/tags`）的页面结构与交互模式内，增加「数据未取全」这一**状态表达**，不新增页面、面板、弹窗、导航步骤或布局模式。

## UI 影响等级

minor —— 只改既有组件的状态文案与数值呈现（报头标题、报头统计、版块行的数值与状态行），沿用既有栅格、列结构、样式体系与操作入口。

## 页面入口

- 入口 URL：`/tags`（叙事工坊总览，未选中版块时的雷达视图）。
- 触发元素：无新增入口；「重试」复用报头下方既有错误条里的重试按钮（`data-testid="radar-retry"`）。

## 用户主任务

1. 主任务：一眼看出「今天该看哪些版块、关注度多高、3 日变化如何」。
2. 次任务：数据不可用时知道「这次没取到」而不是「今天全清零」，并能一键重试。

## 交互状态

| 状态 | 触发 | 本次呈现 |
| --- | --- | --- |
| loading | 首次取数中 | 不变（既有骨架屏） |
| success（完整） | 全部版块取到 | 不变（今日数字照旧） |
| success（部分未取到） | 部分版块 `data_unavailable=true` | 报头标题改为「本次数据未取全」并保留已知关注版块数；当日线索合计显示「—」；未取到的版块行显示「数据未取到」，不参与分档，排在列表最底 |
| error（全部未取到） | 全部版块 `data_unavailable=true` | 报头标题「本次数据未取全，请重试」；统计区显示「—」；错误条出现并带既有「重试」按钮；**不得**渲染「今天没有需要看的版块」这一业务空态 |
| empty（成功且真无数据） | 取数成功且全部版块无日报/无泳道 | 不变（「今天没有需要看的版块」）—— 与「未取到」严格区分 |
| success（部分滞后） | 某版块锚点日 ≠ 全局锚点日 | 该版块**不进前排**（不参与关注度分档），逐行沉底到列表最底部，保留「数据至 MM/DD」徽标 |

## 布局类型

沿用既有 `workspace` 布局（`BoardRadarView.vue` 的 `AppPageShell mode="workspace"`）与版块行栅格 `rd-grid`（6 列：序 / 关注板块·最新日报 / 泳道活跃 / 关注强度 / 3 日变化 / 进入）。本次**不改列结构、不改断点、不改列宽**。

## 组件复用

| 组件 | 复用点 |
| --- | --- |
| `BoardRadarMasthead.vue` | 报头标题（`headline`）、副注（`sub`）、统计区（`statsVisible` / `todayClueTotal`）——只加「未取全」分支，不加新元素 |
| `BoardRadarStory.vue` | 版块行：关注强度数值、3 日变化、日报摘要行——只换文案/取值，不动栅格与操作按钮 |
| `BoardRadarView.vue` | 既有 `.rd-error` 错误条（`role="alert"` + 既有「重试」按钮）复用为「本次数据未取全」提示条，不新增提示条组件；主列表分段渲染（关注 → 常温折叠 → 滞后 → 未取到），沿用 `BoardRadarCold` 的 `rank-offset` 接号机制 |
| `BoardRadarCold.vue` | 既有常温折叠段与其分段标题样式——滞后段/未取到段沿用同一套分组标题样式与接号逻辑，不新造组件 |
| `boardRadarHelpers.ts` | 新增「未取到」判定 helper，沿用既有 `trendOf` / `isDelayed` 的 null-safe 风格 |
| 小矩阵 14 格 | 沿用既有 `cell.count === null` 的「未知格」表达（`data-known="false"`），本次只让未取到的版块整轴进入未知态 |

## 响应式范围

沿用现有断点与栅格行为，不新增/调整断点。未取到态只改文字与数值，不影响窄屏折叠与滚动。

## 原型路径

无（minor 档：不新增页面/面板/弹窗/布局模式，不产出 `ui-prototype/`）。

## 审批状态

not-required（minor 档，复用既有组件与既有交互模式，不改信息架构）。

## Interaction Decisions

| ID | 决策 | 模式 | forbidden | 状态 |
| --- | --- | --- | --- | --- |
| UI-1 | 「本次数据未取全」以页内报头文案 + 页内错误条 + 行内状态文字表达，就地重试 | page | dialog | pinned |
| UI-2 | 未取到的版块行保留行位与栅格，只把数值换成「—」、状态换成「数据未取到」，不折叠、不隐藏、不弹窗解释 | page | dialog | pinned |
| UI-3 | 重试入口复用既有错误条内的按钮，不新增工具栏按钮、不新增浮层 | page | drawer | pinned |
| UI-4 | 滞后版块逐行沉底到列表末尾（仍可见、保留「数据至 MM/DD」），不折叠进常温段、不隐藏 | page | dialog | pinned |

## Data Provenance

| 数据项 | source | 说明 |
| --- | --- | --- |
| 报头标题「本次数据未取全」 | api /semantic-boards/overview | 由行级 `data_unavailable` 派生（存在任一未取到版块）；**不是**由 `attention` 求和派生 |
| 报头标题「N 个板块值得关注」 | 派生 attention 分档公式 | 仅统计**取到**的版块（`attention` 非空且 ≥ 阈值）；未取到的版块不计入 |
| 报头统计「活跃板块 N」 | api /semantic-boards/overview | 活跃版块总数（取到与否都算在册），但「热点 N」只统计取到的版块 |
| 报头统计「热点 N / —」 | 派生 attention 分档公式 | 只统计 `attention` 已知的版块；存在未取到版块时显示「—」，不据此断言「没有热点」 |
| 报头统计「今日线索 N / —」 | 派生 Σ today_clues | 任一版块 `today_clues` 未知 → 合计显示「—」，不显示 0 |
| 版块行「关注强度 X.X / —」 | api /semantic-boards/overview | `attention`；未取到时为未知 → 显示「—」 |
| 版块行「数据未取到」 | api /semantic-boards/overview | 行级 `data_unavailable=true` |
| 版块行「关注强度 · 热点/常温/…」 | 派生 attention 分档公式 | 未取到的版块不参与分档，不挂分档徽标 |
| 版块行「3 日变化」 | 派生 last3/prev3 环比公式 | 沿用既有 `trendOf`；任一侧未知 → 「—」 |
| 版块行「N 篇文章 · M 个事件」 | api /semantic-boards/overview | 未取到 → 不显示数字，改「数据未取到」；真实无日报 → 照旧「0 篇文章 · 0 个事件」 |
| 版块行日报日期与标题 | api /semantic-boards/overview | `report_date` / `report_title`；未取到 → 不显示「无日报日期」这种伪空态 |
| 版块行小矩阵 14 格 | api /semantic-boards/overview | `clue_days` / `lanes[].clue_counts`；未取到 → 整轴未知（`data-known="false"`），不画 0 格 |
| 主列表行序 | 派生 分档公式 + 锚点日判定 | 顺序 = 今日数据版块（按关注度降序）→ 常温折叠段 → 滞后版块（锚点日 ≠ 全局锚点日，逐行）→ 未取到版块；滞后与未取到均不占前排位次，序号连续 |

## 明确不做

- 不做「自动重试」：后端超时后不再发第二次取数，本 change 不引入任何自动重试机制。
- 不做「降级时回退显示上次成功的数据」：不引入前端缓存兜底或旧值回放，未取到就是未取到。
- 不做新的提示条/弹窗/抽屉组件：不新增「数据未取全」专用浮层，不新增 toast。
- 不改版块行的列结构、列宽、栅格与排序键。
- 不把滞后版块隐藏、不折叠、不合并进「常温」段（仍逐行可见，只是沉底）。
- 不做滞后版块的「历史时间窗回看」入口（沉底行不进版块后仍按既有路径进版块态势页）。
- 不改右栏「当天线索最多的泳道」清单与管线状态块的既有呈现（本次只保证主列表与报头不说假话）。
- 不做「当日日报尚未生成（21:00 之前）」的专属页面或面板——日报尚未生成时按各版块自己的最新日报日呈现，属既有行为，本次不动。

## Acceptance（验收映射）

| 验收面 | 载体 | 文件路径 |
| --- | --- | --- |
| 未取到态派生与报头文案（全部/部分未取到、partial 拐角、今日线索合计只计新鲜行） | 组件/组合式测试 | `front/app/features/tags/composables/useBoardRadarOverview.test.ts` |
| 版块行渲染（未取到 → 「数据未取到」+「—」；日报信息未取到 → 不说「无日报日期」；滞后行不自带分档徽标） | 组件测试 | `front/app/features/tags/components/console/BoardRadarStory.test.ts` |
| 四段行序与接号（关注 → 常温 → 滞后 → 未取到）、未取全提示条复用既有 `.rd-error`、业务空态不误渲染 | 组件测试 | `front/app/features/tags/components/console/BoardRadarView.test.ts` |
| 报头统计在未取全时显示「—」 | 组件测试 | `front/app/features/tags/components/console/BoardRadarMasthead.test.ts` |
| 协议归一（`data_unavailable` → `dataUnavailable`，计数缺字段/未取到 → null） | 归一化测试 | `front/app/api/normalizers/boardRadar.test.ts` |
| 滞后判定、关注度展示与 3 日变化 | 工具函数测试 | `front/app/features/tags/components/console/boardRadarHelpers.test.ts` |
| 真机页面核对（滞后沉底、序号连续、报头统计、截图存证） | agent-browser 真机验证 | `openspec/changes/fix-radar-overview-false-zero/ui-verification/report.md`（判定条目 A-1 / A-2） |
