# UI Design — 矩阵降级兼容 + 右栏合并（minor）

> ui-impact: **minor** —— 全部复用既有 token / 组件骨架 / 视觉语言，无新页面、无新路由、无布局模式变更，无原型无审批。本文件是复用契约清单。

## 复用的类与 token（零新增色值 / 零新增断点）

### 1. 雷达小矩阵逐格降级（BoardRadarStory.vue）

- 格子视觉**完全复用**既有 `.rd-mini-cell` / `.b1..b4` 档位（`color-mix(in srgb, var(--color-accent) N%, var(--color-bg-base))`）——降级格与正常格同着档逻辑（值换成分区数后照常走 `laneLevel(count, laneMax)`），**不新增**降级专属色相（title/aria 承担区分，颜色不承载第二语义，红线 28「颜色 SHALL NOT 是唯一信息载体」）。
- 降级标注只在 title/aria：`MM/DD · 无事件线（按分区数 N 显示）`；正常格维持 `MM/DD · N 条事件线`。
- `.rd-mini-cells` 的 14 列 grid、`.rd-mini-lane` 行结构、`@media` 断点全部不动。

### 2. 控制台矩阵格值 + 时间筛选折线（observationLanes / laneConsoleView / ObservationRangeControls / TrendAxes）

- 矩阵格视觉复用既有 `.mx-cell` 着档与 `PARTICIPANT_BUCKET_EDGES` 颜色通道——降级只改格内数字与 title/aria 尾注（`N 条线索（0 条事件线，降级显示）`），不改颜色口径（颜色仍是当日最热事件线来源数分档）。
- `ObservationRangeControls` 曲线标注两行文案改口径：第一行「365 日池 · 已返回泳道的日事件线数」、第二行「无事件线日按线索合计兜底 · 非文章数」；`TrendAxes` 单位标签改「事件线数（条）」（`LaneTrendChart` 既有 `unit-label="事件线数（条）"` 同款，两处轴文字统一）。刷选交互、手柄、键盘语义零改动。

### 3. 右栏合并（BoardRadarReadingRail.vue）

- **结构收敛**：`今日该读` 一个 `rd-rail-block`（保留 `.rd-rule-title` + `READING LIST` 标签）；删除 `rd-warm-block`（升温卡片）与 `rd-new-block`（新起卡片）两节及其样式类。
- **行内徽标**：复用右栏既有排版语言新增最小样式 `.rd-badge`——
  - 字号 `0.58rem`、`letter-spacing 0.04em`、边框 `1px solid color-mix(in srgb, var(--color-accent) 45%, transparent)`、文字 `--color-accent`、圆角 3px、padding `0 0.3rem`；「升温」徽标同款加 `background: color-mix(in srgb, var(--color-accent) 10%, transparent)` 提一档存在感（与 `.rd-warm-block` 既有强调强度同源，不新造色值）。
  - 徽标排在条目标题行内（`rd-read-title` 之后），`新起` / `升温 3.2×` 两态；无徽标即普通当日事件线，不加占位。
- **残热旧线标记**（review R-7）：条目 meta 行（既有 `.rd-read-meta`，`font-size 0.62rem`）补一句「今日无报道证据」，颜色取既有 `--color-warning` token（零新增色值）；只对 `has_today_evidence = false` 的条目渲染。**不是徽标档位**——徽标仍只在「新起 / 升温 N×」两态出现，判据不变。
- **脚注**：跨天线为 0 时一行 `rd-rail-note` 同款脚注「跨天延续事件线建设中」（复用 note 的 muted 排版，不占卡片）；徽标图例一行 `rd-warm-note` 同款说明（新起 = 近 3 日无基线；升温 = 达自身近 3 日基线 2 倍）。
- **保留不动**：滞后/未回填/排序不完整三条 `rd-rail-note`、`?story=` 深链按钮态（`canOpen`）、旧响应回退的版块级清单渲染（storyMode=false 分支原样）。
- 8 条上限由后端截断（`top_stories` ≤ 8），前端不另设屏高逻辑；右栏总屏占与旧「5 条 + 新起 3 条」持平。

### 4. 管线状态块折叠（BoardRadarStatusBlock.vue）

- **折叠态** = 一行摘要：状态词 · 日报 x/y · 队列 n（+embedding m）。复用 `.rs-rule-title`（标题「管线状态 / PIPELINE」常驻）+ 新增最小 `.rs-summary` 行（`font-size 0.66rem`、`--color-text-muted`，异常词用 `--color-accent`，复用 `.rs-dot` 圆点标记异常）。摘要行即展开/收起按钮（同一 `<button>`，`aria-expanded` 随态）。
- **展开态** = 既有五项事实 + 进度条 + 手动生成入口，全部零改动（`rs-facts` / `rs-progress` / `rs-picker` 原样）。
- **状态机**：默认折叠；异常（日报已过生成时刻未生成 / 队列失败任务 >0 / 生成请求 failed / 标签或 embedding 队列活跃量 ≥ 100）自动展开（含挂载时即异常）；异常消除回折叠；用户手动展开/收起随时可用。折叠摘要行本身呈现异常词与计数——折叠不藏异常信号。

## 无原型声明

minor 档：无 `ui-prototype/`、无 ui-approval；上述复用契约即视觉验收依据（机械断言：徽标存在性按 data-testid、折叠态不渲染五项事实、降级格 title 文案）。

## 验收映射

### 组件测试映射

| 断言 | 测试文件 |
| --- | --- |
| 矩阵逐格降级值、降级格 title/aria 区分、`laneCounts` 上限 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| 折线与矩阵同口径（事件线 > 0 用事件线，否则线索口径）、`dayCellMetrics` 与 `poolDayTotal` | front/app/features/tags/components/console/laneConsoleView.test.ts |
| `storyCount`/`threadCount` 缺省回退与 null 不造 0 | front/app/api/normalizers/laneDynamicsWindow.test.ts |
| 单清单 8 条、行内徽标（新起 / 升温 N×）、跨天延续脚注、被删卡片不渲染 | front/app/features/tags/components/console/BoardRadarView.test.ts |
| `top_stories` 上限 8、徽标判定字段、`new_stories` 等被删字段不再下发 | backend-go/internal/topicgraph/repository/board_radar_stories_test.go |
| 折叠态只渲染一行摘要、异常自展开、`aria-expanded` 随态 | front/app/features/tags/components/console/BoardRadarStatusBlock.test.ts |

### agent-browser 实物验证

证据账本：`openspec/changes/matrix-clue-fallback-and-rail-merge/ui-verification/report.md`（A-1～A-10 十条机械断言，静态 :5100，1440×900 与 390×844 双档，截图 `rail-1440.png` / `rail-390.png`）。

---

## 归档后补记 — 雷达小矩阵色阶语义修订（2026-11-09 缺陷修复）

> 归档后补记（允许），不改本 change 的 spec 正文。本节**修订**上文第 1 节「不新增降级专属色」的决策：
> 该决策导致用户报「矩阵格子的颜色没修复」——事件线数与分区数是**两种量纲**，共用一条色阶时
> ① 代理值与已确证值同色同档、② 代理值稀释已确证格对比度、③ 行内事件线稀疏时代理格独占
> 最深实心档（零事件线的一天被画成最热的一天）。实测线上 378 格中 122 格走降级路径。

修订后的复用契约（仍零新增色值 / 零新增断点）：

- **已确证事件线格**：`.b1..b4` 不变（基准 = 该泳道**事件线数**最大值；旧后端 sections 口径下
  `storyCounts = counts`，该基准 = 行内分区数最大值，整行同量纲、档位与改动前一致）。
- **降级格（代理值）**：新增 `.d1..d4` **独立色阶**（基准 = 该泳道**分区数**最大值）+ `.is-degraded`
  形态标记；色强取 `color-mix(in srgb, var(--color-accent) 6% / 18% / 36% / 58%, var(--color-bg-base))`
  （**恒不与 `.b*` 任何一档同色**，且封顶在 `.b3` 之下——最深实心档只留给已确证的事件线峰值）。
- **形态标记**：`.is-degraded` 叠 `repeating-linear-gradient(45deg, …)` 斜纹（形状差异 = 第二信息通道，
  颜色不是唯一信息载体）；色相不新增（沿用 `--color-accent`，与控制台矩阵「单一色相只改 mix 比例」
  的既有语言一致）。
- **表头图例**：`.rd-labels` 标题改为「泳道活跃 · 每格事件线数（斜纹格按分区数兜底）」。
- 不变式：只改展示层，降级值仍 MUST NOT 进 `lane_heat` / 行序 / 关注分档；已确证格颜色零变化
  （378 格逐格对照：变化 122 格 = 恰为全部降级格，已确证格变化 0）。
- 证据与机械断言：`ui-verification/report.md` 的 B-1～B-5 节 + 截图 `matrix-color-{1440,390}.png`。
