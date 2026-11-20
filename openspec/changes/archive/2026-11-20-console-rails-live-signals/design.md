# Design: 版块态势页左右轨改为实时信号

## Context

代码事实（见 `explore-findings.md`）：

- 左轨 `BoardRail.vue` 是 57 行模板：`boards`（`GET /semantic-boards`）直出，`tag_count` 作右列数字。
- 壳层 `useConsoleShellData.loadReportMeta()` **已经调用** `GET /semantic-boards/overview`，但只消费当前板块的 `report_date` / `report_title`，整行其余字段丢弃。
- 控制台读选中日走 `laneDayReading` → `GET /daily-reports/:id`；normalizer `normalizeReportDetail` 只留 `sections`，**丢掉了 `title` / `summary` / `highlights`**。
- 右轨顺序由 `openspec/specs/workshop-board-console/spec.md:164` 固定，改动需 delta。
- 三区栅格 `17rem | 1fr | 21rem`（`BoardConsoleBody.vue`），1440–1535 右轨收 18rem，<1200 右轨隐藏，<768 双轨全隐。
- 后端写库时 `title = highlights[0].Title`、`summary = highlights[0].Reason`（`report_story_highlights.go:78-81`）——**日报标题就是第 1 条重点**。

结论：**零后端改动**，本次全部是读模型重组 + 呈现层。

## Decisions

### D1 复用既有端点，不新增

| 需要的信息 | 来源 | 现状 |
| --- | --- | --- |
| 版块逐日线索轴（14 天） | `overview.boards[].clue_days` | 雷达页 `BoardRadarStory` 小矩阵已在用 |
| 版块锚点日 / 未取到 | `overview.boards[].anchor_date` / `data_unavailable` | 雷达页滞后与未取到分档已在用 |
| 当日日报点评 | `daily-reports/:id` → `report.highlights` | 后端下发，前端 normalizer 丢弃 |
| 当日分区 + 线索 | `daily-reports/:id` → `report.sections[].threads[]` | 控制台读链已取到（只用 `sections` 结构） |
| 分区归属泳道 | `sections[].persistent_topic.label` + `topic_status_at_report` | 同上 |
| 版块泳道当日线索数 | `lane-dynamics.lanes[].timeline[].clue_count` | 矩阵日格已在用 |

### D2 左轨共同轴重对齐（关键正确性要求）

**问题**：后端 `radarAxis(anchor)`（`board_radar_repository.go:501-510`）以**每个版块自己的锚点日**生成 14 天窗口：

```go
axis[i] = anchor.AddDate(0,0, i-13)   // anchor = 该版块自己的最新日报日
```

所以 13 行的「最后一格」各自代表不同日期。直接并列 = 假比较（一个停更 3 天的版块看起来仍然「刚刚有动静」）。

**修法**（纯前端，输入全在手边）：设全局锚点 `G` = overview 响应 `date`（各版块最新日报日的最大值），共同轴 = `[G-13 … G]`。对每个版块：

```
offset = daysBetween(G, board.anchor_date)     // 该版块落后全局多少天
board.clue_days[i] 对应日期 = board.anchor_date - 13 + i
该日期在共同轴上的位置     = i - offset
位置在 [0,14) 内 → 写入；否则 → 未取到（斜纹）
```

| 情形 | offset | 渲染 |
| --- | --- | --- |
| 当日有日报 | 0 | 14 格全部有值 |
| 落后 3 天 | 3 | 前 11 格有值，末 3 格斜纹（未取到） |
| 落后 ≥14 天 | ≥14 | 整行斜纹 |

于是「停更」（末几格斜纹）与「安静」（格子为 0）**在视觉上天然分开**，无需额外文字标注——这正好替代了被删除的辅助说明文字。

**边界**：落在版块自身窗口内、但当天没有日报的日子，`clue_days` 给 0（overview 既有口径，无 null 概念）。这是 overview/雷达页既有行为，本 change **沿用不修改**，也不在界面上把它说成「当天真的没有线索」——左轨不配任何数字文案。

### D3 左轨读模型在壳层组装，组件只渲染

`BoardRail.vue` 改为收 `rows: RailBoardRow[]`（纯视图模型），由 `useConsoleShellData` 暴露：

```ts
interface RailBoardRow {
  boardId: number
  label: string
  /** 共同轴 14 格（oldest → newest）；null = 该位未取到。 */
  cells: (number | null)[]
  /** 行内峰值（用于归一色阶）；0 = 整行无数据。 */
  peak: number
  /** 行整体强度系数（0.42–1，按 14 天总量分档）——安静板块自然压暗。 */
  intensity: number
  /** 治理数据退到 title。 */
  tagCount: number | null
  /** 滞后标记（供 title 与可访问文本）。 */
  anchorDate: string | null
}
```

`railClueAxis.ts` 提供纯函数：
- `buildRailBoardRows(boards, overviewRows, globalDate)` —— join（按 `boards` 顺序，**不**按 overview 的 attention 序）、共同轴重对齐、峰值与强度计算、null 语义。
- 全部未取到（`data_unavailable` / 缺 `clue_days`）→ 整行斜纹。

### D4 颜色归一复用现有纯函数

每行 14 格按**行内峰值**归一：`clueRowMax` → `clueDepth(count, rowMax)` → `clueMix(depth)`，来源 `observationLanes.ts`（矩阵格与雷达小矩阵同源）。不做第二套色阶。

行整体强度（跨行）：`0.42 + 0.58 × (行 14 天总量 / 全库最大行总量)`，作用在 `.row-axis` 的 opacity 上。形状不变，只压暗安静行。

### D5 「当日日报」的两层组装

`RailNowBrief.vue` 收已组装的 view model（组件内不发请求）：

```ts
interface NowBriefModel {
  /** 所跟日期（= 矩阵 selectedDate）。 */
  date: string | null
  /** 该日是否有日报。 */
  hasReport: boolean
  /** 点评层：日报 highlights（2–3 条，顺序 = 后端顺序）。 */
  reviews: NowReview[]
  /** 泳道层：当日日报里归属正式泳道的分区。 */
  lanes: NowLane[]
}
interface NowReview {
  title: string
  reason: string
  /** article_count 缺省 = null（不显示「N 篇来源」）。 */
  articleCount: number | null
  /** cluster_id 存在 = 可定位正文（点击跳转）。 */
  clusterId: number | null
}
interface NowLane {
  topicId: string
  label: string
  color: string
  /** 代表线索标题（日报 thread 标题或分区概述）；无 = 不出该行。 */
  sentence: string
}
```

组装规则（`nowBrief.ts` 纯函数）：

1. **点评层** = `report.highlights` 原样（不截断条数，后端就是 2–3 条）。
   - **不额外渲染 `report.title`**（= `highlights[0].title`）与 `report.summary`（= `highlights[0].reason`）——重复。
   - `article_count` 缺省 → 不显示「N 篇来源」（不补 0）。
2. **泳道层** = `report.sections` 过滤 `topic_status_at_report === 'active'` 且有 `persistent_topic`，每条取该分区**首条 thread 的 title**（无 thread 时退回 `cluster_label`）；两者皆无 → 该行不出。
   - **线索数**：取 `lane-dynamics` 该泳道在**选中日**的 `clue_count`（与矩阵日格同源同值）。取不到 → `null`（显示 `—`，不补 0）。**MUST NOT** 用日报分区 thread 数（含综合稿）——那是另一套量纲。
   - **排序**：按线索数降序（`null` 排末位）；并列时保持日报 sections 原序（`sort` 必须 stable）。日报原序是编辑叙事顺序，并列时用它兜底。
   - **归属校验**：泳道层条目 SHALL 只包含能在 `lane-dynamics.lanes` 中按 `topic_id` 找到的正式泳道（名单含 watch 关联泳道与矩阵判为休眠而折叠的行）——数值因而与矩阵日格同源同值；查不到该 topic 的分区不进右栏。注：休眠泳道在矩阵首屏折叠于「休眠泳道 N」，其格需展开后可见，这不影响数值同源。
   - candidate 分区不进右栏（矩阵下方「当日其他话题」已承载）。

### D6 跟随矩阵选中日（唯一日期源）

- 右轨无任何日期选择器。块标题写 `当日日报 · MM-DD`，日期取 `useConsoleSelection.selectedDate`。
- 选中日无日报 → 块内诚实空态（「这一天没有日报」），不显示上一日的内容、不回落到锚点日。
- 报告详情请求失败 → 该块局部降级（提示 + 重试），右轨其余块照常。

### D7 矩阵选中列淡底

`ObservationMatrixGrid` / `ObservationMatrixDayHeader`：选中列（`selectedDate` 对应的轴位置）整列加一层淡底 `background`（`color-mix(in srgb, var(--color-accent) 4%, transparent)` 量级），不描边、不改列头高亮、不动格子本身颜色。

- 只在**有分区的那天**（报日）铺底；无分区的历史日不铺（避免整列灰带盖过「— 无分区」的诚实表达）。
- 复用既有 `selectedDate`（唯一状态），MUST NOT 新增第二份选中态。
- 窄屏（`BoardNarrow*`）复用同源日期条，本 change 不改。

### D8 体检 brief 瘦身

`RailHealthBrief` `variant === 'brief'` 保留：徽章（评分 + 分档色）、`上次体检 MM/DD HH:mm`、`刷新`、错误/重试、stale 提示；移除评语首句。评语仍在 `full`（`BoardHealthDetail`）完整可见，信息不丢。右轨体检块由约 92px 降到约 28px。

### D9 不新增折叠

本次不引入「折叠/展开」交互。顺序重排 + 体检瘦身已把「当日日报」与「话题追踪」放进首屏（1440×900 下右轨可视高度约 858px：体检 28 + 当日日报 ~330 + 追踪 150 + 候选 130 + 来源 160 ≈ 798，余量靠右轨纵向滚动吸收）。

### D9b 两栏满高自适应

用户反馈原型「左右两侧太空、内容悬在顶部」。两条栏 SHALL 是**满高容器**（高度 = 视口高减顶栏），不是随内容收缩的浮条：

- **容器**：两栏各自 `height: 100%` + 纵向滚动（`.scroll-thin`）；带极淡底色（`rgba(232,227,218,.022)` 级别），与主舞台拉开一层，使栏体读作「面」。
- **右轨纵向分布**：右轨为纵向 flex；「当日日报」块 `flex: 1 1 auto`（内容顶部对齐、块内不滚），其余四块 `flex: 0 0 auto` 贴底。视口变高 → 空白落在「当日日报」块内（而非栏底悬空）；视口变矮 → 整栏滚动（块默认 `min-height: auto` 不会被压扁）。
- **左轨行高**：一行式行高弹性区间 24–40px（`flex: 1 1 auto` + `max-height: 40px`），栏高富余时均匀撑开，行内名称与条码垂直居中，行间一条极淡分隔线；栏高不足时整栏滚动。

注：生产页面当前的版块栏同样是「13 行置顶 + 下方留白」，本次不引入新内容填充留白（加口径说明/汇总数字都违反「左栏不配解释性文字与数字」的约束），改以栏体底色与分隔线让留白读作栏体的一部分。

### D10 动效约束

| 动效 | 实现 | 时长 | reduced-motion |
| --- | --- | --- | --- |
| 左轨 14 格入场点亮 | `@keyframes rail-cell-in` + `animation-delay: calc(var(--i) * 12ms)` | ≤ 420ms | 关 |
| 今日格脉动 | `rail-now-pulse`（opacity/box-shadow，不位移）；**仅选中行** | 2.2s infinite | 关 |
| 选中行左缘线 | `transform: scaleY(0) → 1`，`transform-origin: top` | 160ms | 关 |
| 当日日报行入场 | opacity 淡入 + 错峰 delay | 220ms | 关 |

禁止：13 行同时脉动、hover 位移（会与矩阵行 hover 冲突）、任何 `transition: all`。

### D11 测试策略

| 层 | 内容 |
| --- | --- |
| 纯函数单测 | `railClueAxis.test.ts`：共同轴重对齐（offset 0 / 3 / ≥14）、join 顺序不等于 overview 顺序、未取到整行斜纹、峰值与强度计算；`nowBrief.test.ts`：点评层不重复 title/summary、泳道层只取 active、有 thread 用 thread 否则用 cluster_label、两者皆无不占位、candidate 排除、顺序沿用日报序 |
| 组件测试 | `BoardRail.test.ts`（**新增**）：14 格数量、斜纹格、选中态、`title` 含 `tag_count`；`BoardContextRail.test.ts`：区块顺序断言；矩阵淡底断言（在既有矩阵测试内补） |
| 不做 | 不为动效写断言（视觉验收走 `ui-verification/report.md`）；不新增 e2e 用例 |

## Risks / Trade-offs

| 风险 | 处置 |
| --- | --- |
| 272px 一行式放不下「名称 + 14 格」 | 名称 `min-width: 0` + ellipsis，条码固定 14×7px + 1px gap ≈ 100px；行高 24px |
| 共同轴重对齐算错 → 假比较 | 纯函数单测锁 offset 0/3/≥14 三种情形 |
| `clue_days` 窗口内「无日报日 = 0」是 overview 既有口径 | 沿用不修改；左轨不配任何数字文案，不把 0 说成「无线索」 |
| 泳道层与矩阵泳道行重复 | 矩阵给密度、右栏给内容；右栏不显示条数，形状与语义都不同 |
| 泳道层句子来自日报 thread，可能很长 | 截断到 2 行（`-webkit-line-clamp`），完整文本进 `title` |
| `highlights` 为空 | 块内显示诚实空态（「本期暂无重点」，沿用日报阅读页同一措辞），MUST NOT 整块消失 |
| 右轨顺序改动违反现行 spec | 本 change 出 delta（MODIFIED Requirement） |
| overview 是 5s 预算端点 | 与既有 `loadReportMeta` 同一请求（不新增），失败时左轨退「全部未取到」形态，不阻塞矩阵 |

## Migration

无数据迁移、无配置项、无缓存失效。老后端（缺 `clue_days` / `today_clues`）时左轨整行斜纹；日报缺 `highlights` 字段时右栏该块显示空态，矩阵与追踪照常可用。
