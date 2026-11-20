<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- ui-approval: approved -->
<!-- constraint-domains: semantic-board, daily-report -->

# Proposal: 版块态势页左右轨改为实时信号

> **原型已批准**（2026-11-19，用户在对话中明确通过）：左轨一行式名称 + 14 格条码、右轨「当日日报」两层、矩阵选中列淡底、两栏满高自适应。
> 用户追加决策：**各泳道按当日线索数降序**（线索数取 `lane-dynamics` 当日 `clue_count`，与矩阵日格同源同值）。

## Why

版块态势页 `/tags/:boardId`（矩阵/追踪）的两条常驻栏现在都在回答「这个版块**配置得怎么样**」，没有回答用户每天打开页面最想知道的「这个版块**最近在发生什么**」。

1. **左轨的数字没有含义**。每行右侧的 `409 / 1515 / 141` 是 `tag_count` = `topic_tag_board_labels` 行数（全时段标签挂载数，含 `direction_mismatch` 幽灵行），与矩阵里的「当日线索数」不同源、不同量纲、不同时间窗。用户实测反馈「数字意味不明」——它既不随今天变化，也无法横向比较。
2. **左轨没有形状**。13 行「名称 + 孤立数字」，眼睛没有可比较的东西，纯文字列单薄。而本产品已有成熟语言（主矩阵的密度格、雷达页每板块 14 日小矩阵、顶部时间视窗曲线），左轨却没有继承任何一种。
3. **右轨四块同构**。体检 / 构成候选 / 话题追踪 / 来源构成四块都是「标题 + 竖排列表 + 查看全部」，形状一样、层级一样，读起来就是堆列表；其中三块是治理属性。
4. **「正在追踪」被挤到折叠位**。体检测评语占三行，把真正高频使用的「话题追踪」推到第三块，1200px 高度的笔记本上要滚。
5. **没有「最近在发生什么」这一层**。当日日报的编辑点评（`highlights`）、日报分区（`sections`）与线索（`threads`）后端全部下发，控制台也已把选中日日报详情拉到手（`laneDayReading` → `GET /daily-reports/:id`），只是 normalizer `normalizeReportDetail` 只保留了 `sections`，把 `title` / `summary` / `highlights` 丢了。

## What Changes

### 1. 左轨（版块栏）：文字列表 → 逐日线索条码

每个版块一行，两段：**名称**（省略号截断）+ **14 格逐日线索条码**（右侧）。

| 元素 | 口径 |
| --- | --- |
| 条码 | 该版块**逐日非综合稿线索数**（`GET /semantic-boards/overview` 的 `clue_days`，14 天），颜色按**行内峰值**归一分档（复用 `observationLanes.ts` 的 `clueDepth` / `clueMix`，与矩阵格、雷达小矩阵同一套算法） |
| 数字 | **删除**。当日线索数不单独占位（用户裁决：画蛇添足） |
| 辅助说明 | **删除**（「数字 = 当日线索数 · 每格 = 一天」这类解释性文字不进界面） |
| 标签挂载数 | 退到行的 `title`（治理数据仍可查） |

行序保持 `display_order`（不因活跃度重排）；活跃度差异只用视觉强调表达：行整体强度按 14 天总量分档，安静板块自然压暗。

**共同轴对齐（新增的诚实性要求）**：后端 `radarAxis(anchor)` 用**每个版块自己的锚点日**生成 14 天窗口（`board_radar_repository.go:433,501-510`），因此 13 行的「最后一格」各自代表不同日期，直接并列即假比较。前端 SHALL 按**全局锚点日**（overview 响应 `date`）把每行的 `clue_days` 重对齐到共同轴 `[date-13 … date]`：该版块自身窗口覆盖不到的位置标**未取到（斜纹占位）**，MUST NOT 补 0。于是「停更」与「安静」在视觉上天然分开，无需额外文字标注。

### 2. 右轨：顺序重排 + 体检瘦身 + 「当日日报」块

新顺序：**版块体检（一行状态条）→ 当日日报（新增）→ 话题追踪 → 构成候选 → 来源构成**。

- 体检 `brief` 压缩为**一行状态条**：真实 `llm_score` 徽章 + 上次体检时间 + 刷新。评语首句移到 `full` 详情（右轨常驻不再展示评语）。
- 新增「当日日报」（`RailNowBrief.vue`），**跟随矩阵选中日**（唯一日期源），两层：
  1. **点评层**：日报 `highlights`（后端 LLM 生成的 2–3 条当日要闻），每条 = 标题 + `reason` 截断 + 「N 篇来源」（`article_count` 缺省时不显示）。
     **MUST NOT 单独再显示日报标题**——后端 `FinalizeReportHighlights` 写库时 `title = highlights[0].Title`、`summary = highlights[0].Reason`（`report_story_highlights.go:78-81`），标题就是第 1 条重点，重复展示即重复。
  2. **泳道层**：当日日报里归属于正式泳道（`topic_status_at_report = active`）的分区，每条 = 泳道色点 + 泳道名 + **该分区的一句代表线索**（日报 thread 标题 / 分区概述）。**不显示条数**——「多少条」是矩阵的活，右栏只讲「说了什么」；两套线索量纲同屏出现即口径不统一。

  candidate 分区（「其他动态」）SHALL NOT 进右栏（矩阵下方的「当日其他话题」折叠组已承载）。

### 3. 日期来源唯一

右轨**不提供任何日期选择器**。日期只有一个来源：壳层 `useConsoleSelection.selectedDate`（矩阵列头 / 日格 / 编年三处入口共用）。右栏的「当日日报」跟随它，块标题写明所跟日期。

### 4. 中间舞台：选中列整列淡底

矩阵当前只靠**列头**的琥珀高亮标示选中日（`ObservationMatrixDayHeader.vue` 的 `is-selected`）。本次 SHALL 给选中列整列加一层淡底（`background`，不加边框、不改列头），使「右栏在讲哪天」在主舞台一眼可见。选中态仍是同一 `selectedDate`，MUST NOT 新增第二份选中态。

### 5. 骨架（版式骨架 + 加载骨架）

- 版式骨架：右轨不再四块同构——体检是一行状态条、当日日报是「点评行 + 泳道行」、追踪是带状态点的点行、来源是构成条。
- 加载骨架：左轨行骨架 = 名称灰条 + 14 格灰格（不再是 4 条纯灰条）；「当日日报」骨架 = 2 条点评灰条 + 2 条泳道灰条。不新增共享 skeleton 组件（本仓库无统一 skeleton，沿用各组件自写 + `rail-pulse` 动画语言）。

### 6. 动效（克制、可关）

- 左轨：进页面 14 格逐列点亮（stagger ≤ 40ms/格，总时长 ≤ 420ms）；**只有当前选中行**的当日格脉动；选中行左缘 1px 强调线由上而下生长（`scaleY` 160ms）。13 行全在闪 = 视觉噪声，明确禁止。
- 「当日日报」：点评行与泳道行入场错峰淡入（不做位移动画）。
- `prefers-reduced-motion: reduce` 下全部关停（只留状态差异）。

## Capabilities

### Modified Capabilities

- `workshop-board-console`：
  - 版块态势页骨架 Requirement 的「版块栏」补充信号表达契约（左轨条码口径 + 共同轴对齐 + 未取到语义）。
  - 治理右轨与治理抽屉 Requirement：区块顺序改为「体检（一行状态条）→ 当日日报 → 话题追踪 → 构成候选 → 来源构成」；体检 brief 不再常驻评语首句；新增「当日日报」模块的两层口径与降级链；右轨不得出现日期选择器。

## Impact

**前端**

| 文件 | 改动 |
| --- | --- |
| `front/app/features/tags/components/console/BoardRail.vue` | 模板/样式重写：名称 + 14 格条码；props 新增 `rows`（纯视图模型） |
| `front/app/features/tags/components/console/RailNowBrief.vue` | **新增**：「当日日报」模块 |
| `front/app/features/tags/components/console/BoardContextRail.vue` | 区块顺序重排 + 挂载 `RailNowBrief` |
| `front/app/features/tags/components/console/RailHealthBrief.vue` | `brief` 变体瘦身为一行状态条（评语移入 `full`） |
| `front/app/features/tags/components/console/ObservationMatrixGrid.vue` / `ObservationMatrixDayHeader.vue` / `observationMatrixRows.css` | 选中列整列淡底 |
| `front/app/api/normalizers/laneDayReading.ts` | `normalizeReportDetail` 补 `title` / `summary` / `highlights` 字段（纯前端补字段，零后端改动） |
| `front/app/features/tags/components/console/railClueAxis.ts` | **新增**纯函数：overview 行 → 左轨行视图模型（含共同轴重对齐 + null/斜纹/滞后判定） |
| `front/app/features/tags/components/console/nowBrief.ts` | **新增**纯函数：日报详情 → 「当日日报」两层视图模型（点评层 + 泳道层） |
| `front/app/features/tags/components/console/railContext.ts` / `useConsoleShellRail.ts` / `BoardConsoleBody.vue` / `consoleStageBinding.ts` | 传参接线 |
| `front/app/features/tags/components/console/BoardRail.test.ts` | **新增**（当前无该测试）：共同轴对齐 / 斜纹 / 选中四态 |
| `front/app/features/tags/components/console/BoardContextRail.test.ts` | 顺序断言更新（现有测试会因顺序变化失败） |

**后端**：无改动。`overview` 与 `daily-reports/:id` 的字段均已存在。

**文档**
- `docs/reference/flow/semantic-board.md`：左轨条码口径与共同轴对齐规则；右轨「当日日报」节。
- `docs/reference/flow/daily-report.md`：`highlights` 与 `title`/`summary` 的派生关系（title/summary 取首条重点）作为右栏不得重复展示的依据。
- `docs/reference/api/daily-reports.md`：`GET /daily-reports/:id` 节补「L1 版块态势页右轨消费方」。

**风险**
- 左轨 17rem（272px）：一行式 = 名称（省略号）+ 14 格 × 7px + 间距 ≈ 名称 150px / 条码 100px，可容下；行高 24px，13 行 ≈ 312px。
- `GET /semantic-boards/overview` 是 5s 预算聚合端点：左轨与壳层共用的同一次请求（**不新增请求**），失败时左轨退「全部未取到」形态，版块列表与矩阵不受影响。
- 「当日日报」依赖选中日日报详情：控制台已有该请求（`laneDayReading`），本次只是把被丢弃的字段接上，**零新增请求**；该请求失败时右栏该块局部降级，其余块照常。
