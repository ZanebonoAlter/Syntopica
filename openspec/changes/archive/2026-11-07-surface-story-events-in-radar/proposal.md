<!-- complexity: medium -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, daily-report -->
<!-- doc-impact: flow, api -->

# Surface story events in radar

## Why

用户原话（2026-11-07）：

> 「叙事工坊首页也有个今日该看，我觉得这个不应该是日报，而是事件才对」
> 「今日升温这个应该指事件线」

两条硬证据（实现现状只读核查，细节见 design.md「背景与约束」节）：

1. **升温判定口径早就是事件线热度，标题却停在版块**。后端 `story_heat.go` 的 `boardHeatEvidence.compute(at)` 逐条算出的是 **story 级** 热度，`ComputeBoardStoryHeatByDay` 只是把它**求和成版块合计**；前端 `useBoardRadarOverview.ts` 再拿 `todayHeat / (last3Heat/3)` 与「当日事件线数 ≥3」判升温，并把倍率挂在**版块名**上（`BoardRadarReadingRail.vue` 渲染 `<span>{{ item.row.label }}</span><b>{{ item.ratio.toFixed(1) }}×</b>`）。数字口径已是事件线热度，呈现粒度还停在版块——读者看到「某版块 3.0×」却不知道是**哪条事件线**在升温，只能自己进版块翻日报去找。
2. **「今日该读」给的是日报标题，读者看不到今天到底发生了什么**。右栏逐条渲染 `row.reportTitle`（日报标题）+ 日期 + 文章数 + 事件数，点击跳该版块日报页。这是「哪个版块今天出了报」，不是「今天发生了哪几件事」；想知道具体是哪几条事件在升温，仍需点进日报翻正文。

用户已拍板两处口径：**Q1 今日该看换成事件线 → 跨版块全局 Top N（每条带版块名）**；**Q2 升温改成事件线级 → 无近 3 日基线的新事件线单列「今日新起」档，不硬凑倍率**。

## What Changes

- **「今日该读」改为事件线条目**：右栏该块不再渲染各版块日报标题，改为**跨版块全局 Top N 事件线**（默认 N=5）。每条显示版块名 + 事件线标题 + 来源数 + 热度；排序为「原始热度降序 → 来源数降序 → 存活事件线 id 升序」，与日报「今日重点」候选排序同一口径。版块行的「最新日报标题」入口保留不动。
- **「今天可能升温」改为事件线级**：判定落在**每条事件线自己**——锚点日 story_heat ≥ 2 × 它自己的近 3 日基线（锚点日前 3 个 CST 日终的日均，不含锚点日），且该事件线在锚点日当天有报道证据。倍率挂到事件线而不是版块。
- **新增「今日新起」档**：近 3 日基线为 0（近 3 日无任何热度证据）的新事件线单列一档，**不显示倍率**，不按 0 倍或 ∞ 倍硬凑。
- **「今日」口径钉死**：事件线热度与基线一律以雷达锚点日（各活跃版块最新日报日的最大值，无日报时回退今天）的 CST 日终为同一时刻计算，**不是自然今天**；跨版块混排只在这一时刻上做，避免滞后版块的旧事件被当成「今天」。
- **未回填版块保守降级**：`stories_backfilled=false` 的版块没有可识别事件线 → 不产生任何条目，也**不按 0/空态冒充无事件**；界面明示「N 个版块事件身份未回填，未纳入事件线排序」。端点超时降级（`data_delayed`）时明示「本次事件线排序不完整」。
- **点击去向可寻址**：后端为每条事件线解析出「锚点日之前最近一期含该事件线正文的日报日期」，条目跳转 `/tags/{boardId}/{reportDate}?story={storyId}`；日报阅读台读 `?story=` 后按既有正文 DOM 顺序定位首条对应 thread（展开/滚动/聚焦/高亮 2s）。**找不到目标不跳假位置**：本期正文无该事件线则不滚动、给提示；无正文落点的事件线不渲染为可点条目。
- **改动能力**：`workshop-radar`（主：雷达条目与聚合端点契约）、`daily-report-system`（`?story=` 深链定位契约）。无新表、无新列、无数据迁移。

## Impact

- **后端**：`backend-go/internal/topicgraph/repository/board_radar_repository.go`（overview 端点新增顶层事件线清单段、跨板归并与排序）、新增 `board_radar_stories.go`（按事件线拆分的明细，复用 `story_heat.go` 既有证据与热度算法，不重写公式）、`story_heat.go`（证据查询附带事件线标题字段，附加字段不改热度计算）、`story_models.go`（无字段变更）。既有 `ComputeBoardStoryHeatByDay` 的返回值语义与调用方不变。
- **前端**：`front/app/api/normalizers/boardRadar.ts`（新增 raw 字段与 camelCase 读模型）、`front/app/features/tags/composables/useBoardRadarOverview.ts`（`todayRead`/`warming` 改为消费事件线清单）、`front/app/features/tags/components/console/BoardRadarReadingRail.vue`（两块内容换数据源与呈现粒度，新增「今日新起」子档）、`front/app/features/tags/components/console/BoardRadarView.vue`（接线与去向）、`front/app/features/tags/components/daily-report/DailyReportDeskShell.vue` + `DailyReportReader.vue`（`?story=` 深链定位）。
- **API**：`GET /api/semantic-boards/overview` 响应新增顶层 `stories` 段（锚点日、热度口径版本、今日该看 Top N、今日升温、今日新起、未回填版块、降级标记）。**单次请求契约不变**，不新增端点；旧响应缺 `stories` 段时前端回退到既有版块级列表。
- **存储**：只扩展响应 JSON，无新增表/列/迁移，不回填历史。
- **性能**：overview 端点维持 5s 预算；每版块新增一次窄窗（锚点日前 4 天）证据读取 + 4 个日终的 per-story 热度计算，需实测确认不击穿预算；超时按既有 `data_delayed` 降级并标注排序不完整。
- **红线**：不改红线 28（矩阵/演进的事件线量纲）、不改热度来源（仍只从 `story_signals` 派生，`heat-v1-48h-hl24h`）、不改 lane_heat/关注度/行序口径。红线 29 的「今天可能升温」条款由**版块级改写为事件线级**（≥2× 阈值保留、热度来源不变、事件线数下限换成可验证的「锚点日当天有报道证据」），**实现阶段必须同步改写 `docs/reference/flow/semantic-board.md` 红线 29**（本 change 制品阶段不改 reference 文档）。红线 30 的未回填降级在事件线清单上从严执行。

## 部署后影响与需要的操作

- **部署后用户可见行为**：叙事工坊（`/tags`）右栏「今日该读」从「各版块最新日报标题」变成「跨版块全局 Top 5 事件线（带版块名、来源数、热度）」；「今天可能升温」从版块级倍率变成事件线级倍率（每条挂到具体事件线）；右栏多出「今日新起」一档（无近 3 日基线的新事件线，不显示倍率）。版块行的「最新日报标题」入口与 14 日小矩阵、关注度分档**全部不变**。
- **需要用户手动执行的操作**：无数据迁移、无需重建日报、无需清理缓存。若某版块的事件身份尚未回填（`cmd/backfill-stories` 未跑或失败），该版块**不会**出现在事件线清单里，界面会标注「事件身份未回填，未纳入事件线排序」——想让它上榜需手动跑既有回填 CLI（`cmd/backfill-stories`，幂等可重跑）。若想恢复旧行为，回滚部署即可（前端对缺 `stories` 段的旧响应有回退分支）。
- **旧数据如何降级**：历史日报、旧事件线、旧版块行一律不动；未回填版块按上条标注而非显示「0 个事件」；`heat_incomplete`（落后源）标记照旧透出，排序不完整时界面明示；点击事件线若该期正文已无对应 thread，不跳假位置、只给提示。
