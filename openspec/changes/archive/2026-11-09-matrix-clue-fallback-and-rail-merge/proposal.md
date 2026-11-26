<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board -->
<!-- doc-impact: api, flow -->

# 矩阵降级兼容 + 右栏合并（单清单徽标化 + 管线折叠）

## Why

用户看真实界面后拍板的四件事（2026-11-09，真库实测为证）：

1. **雷达小矩阵大片空白**。事件线回填只做到 2026-10-30，10-24～10-29 六天（约 60 个格子）`story_counts[i] = 0` 但那些天明明有线索（132/73/118/123/131/118 条）。现状 `lane.storyCounts ?? lane.counts` 只在**数据缺省**（旧后端）才回退——数据存在但为 0 时不回退，格子全空。用户拍板：**逐格降级**（事件线数 > 0 用事件线数，否则兜底显示），且降级必须可辨（title/aria 明示，不得静默冒充事件线数）。
2. **控制台矩阵的时间筛选折线图与格子口径不一致**。`buildPoolSeries` 用 `dayCellMetrics.total`（线索合计），矩阵格子主值是事件线数——两处数字对不上。用户明确要求同步：折线与格子用**同一条降级规则**。
3. **右栏四块冗余**。「今天可能升温」当前真库**恒空**（跨天线 0 条，见 `enable-story-cross-day-continuation`）；「今日新起」实测 = 「今日该读」第 6/7/8 名（线索:事件线 ≈ 1.04~1.08，热度排序下新起几乎必然挤进 Top 5），纯冗余。用户拍板：**一块清单「今日该读」扩到 8 条**（吃掉新起的位置），新起/升温改为**行内徽标**（`[新起]` / `[升温 3.2×]`），删掉三块独立卡片/摘要行。
4. **管线状态块常驻展开太占地方**。默认折叠成一行摘要，异常态自动展开。

## What Changes

- **雷达小矩阵逐格降级**（`BoardRadarStory.vue`）：格子值 = `storyCounts[i] > 0 ? storyCounts[i] : counts[i]`。已查清 `RadarLane.counts[i] = len(day.Sections)` = **分区数**（`board_radar_repository.go` buildRadarRow）——雷达小矩阵降级用分区数兜底（控制台矩阵可得线索合计，两边各自用实际可得的口径，文档写明）；降级格 title/aria 标注「无事件线（按分区数 N 显示）」。降级只影响展示，不参与 lane_heat/排序/分档。
- **控制台矩阵格值与时间筛选折线图同口径降级**（`dayCellMetrics` / `matrixCellView` / `poolDayTotal`）：共用规则 = 该日事件线数 > 0 用事件线数，否则**当日线索合计**（各桶 `thread_count ?? events+folded` 之和）兜底；`story_count` 为 null（旧响应/全未知）维持现状（`'?'` / null，不造 0）。降级格 title/aria 明示「0 条事件线，按线索合计兜底」。`ObservationRangeControls` 曲线标注与 `TrendAxes` Y 轴单位文案同步改口径。
- **右栏合并**（`BoardRadarReadingRail.vue` / `useBoardRadarOverview.ts`）：
  - 「今日该读」单清单扩到 8 条；行内徽标：`[新起]`（当日有证据 ∧ 无基线）、`[升温 N×]`（有基线 ∧ 倍率 ≥2 ∧ 当日有证据）——判据与被删卡片逐字相同，只换展示位置；无徽标 = 普通当日事件线。
  - 删除「今日新起」独立卡片、「另有 N 条」摘要行、「N 条均已列入」说明行、「今天可能升温」独立卡片。
  - 升温空态收敛为脚注：只在 `cross_day_story_count == 0`（服务端判据）时一行「跨天延续建设中」；有跨天线但没升温时不显示任何空态（徽标已表达）；旧响应缺字段不出脚注。
  - 保留：滞后/未回填/排序不完整提示（红线 30）、`?story=` 深链、点击定位。
- **后端响应精简**（`board_radar_repository.go`）：`top_stories` 上限 5→8（8 = 原 5 该读 + 3 新起的屏位）；`warming` / `new_stories` 数组与 `new_count` / `new_total_count` 字段**移除**（徽标判定所需 `has_baseline` / `ratio` 随 `top_stories` 条目自带；`warming_count` 保留供报头计数；`cross_day_story_count` 保留供脚注判定）。
- **管线状态块折叠**（`BoardRadarStatusBlock.vue`）：默认折叠成一行摘要（状态词 · 日报 x/y · 队列 n），点击展开五项事实与手动生成入口；异常态（日报已过生成时刻未生成 / 队列失败任务 / 生成请求失败 / 队列明显积压）自动展开，异常消除回折叠。

不动的事：热度公式/窗口/排序键（heat desc → participants desc → story id asc）、事件线身份/归属/去重算法（`enable-story-cross-day-continuation` 辖区）、锚点日契约、`?story=` 深链、R-17 未知态语义。

## Capabilities

### New Capabilities

（无——只修订既有能力。）

### Modified Capabilities

- `workshop-radar`：
  - MODIFIED「版块行构成与分档」：小矩阵逐格降级规则 + 诚实标注（新 Scenario）。
  - MODIFIED「今日该读与升温提示」：大改写——单清单 + 行内徽标 + 跨天线脚注，删除新起/升温独立分档与三态摘要契约；锚点日/未回填/排序不完整契约全部保留。
  - MODIFIED「版块总览聚合端点」：Top N 上限 8；清单段字段去留。
- `workshop-board-console`：
  - MODIFIED「泳道热度口径（2026-10-27 矩阵阶段修订）」：矩阵日格展示降级规则（事件线数 > 0 用事件线数，否则线索合计兜底，title/aria 明示；热度/行序/休眠输入不变）。
  - MODIFIED「观察窗与自由日期显示（2026-10-27 用户裁决 A 路线）」：顶部时间视窗图与矩阵格同口径；曲线标注文案同步。
- `radar-pipeline-status`：
  - MODIFIED「雷达总览页右栏底部常驻管线状态块」：默认折叠一行摘要 + 异常自动展开（其余 Requirement 不动）。

## Impact

- **后端**：`backend-go/internal/topicgraph/repository/board_radar_repository.go`（`radarTopStoryLimit` 5→8；删 `warming`/`new_stories` 产出与 `radarWarmingLimit`/`radarNewStoryLimit`；`WarmingCount` 改纯计数）；测试 `board_radar_stories_test.go`（新上限测试 + 退役契约测试删除/改写）。
- **前端**：
  - `front/app/api/normalizers/laneDynamicsWindow.ts`（`DayCellMetrics` 增 `displayCount`/`degradedFromClue`）；
  - `front/app/features/tags/components/console/observationLanes.ts`（`matrixCellView` 格值与 title 用降级口径）；
  - `front/app/features/tags/components/console/laneConsoleView.ts`（`poolDayTotal` 用 `displayCount`）；
  - `front/app/features/tags/components/console/ObservationRangeControls.vue` + `TrendAxes.vue`（口径文案）；
  - `front/app/features/tags/components/console/BoardRadarStory.vue`（小矩阵逐格降级 + title）；
  - `front/app/features/tags/components/console/BoardRadarReadingRail.vue`（结构重排：单清单 + 徽标 + 脚注）；
  - `front/app/features/tags/composables/useBoardRadarOverview.ts`（删三档派生，增脚注判据）；
  - `front/app/api/normalizers/boardRadar.ts`（读模型删 `warming`/`newStories`/`newCount`/`newTotalCount`）;
  - `front/app/features/tags/components/console/BoardRadarStatusBlock.vue`（折叠状态机）。
- **文档**（doc-impact: api, flow）：`docs/reference/api/daily-reports.md`（overview `stories` 段：上限 8、字段表去留、示例）；`docs/reference/flow/semantic-board.md`（L0 雷达段、红线 28/29 展示层降级条款、代码入口、变更溯源）。

## 部署后影响与需要的操作

- **无需数据迁移、无需回填、无需清缓存**：纯展示层与响应字段精简，不动表结构。
- **需要的操作**：同批部署前后端（前端读模型删了 `warming`/`new_stories` 消费，旧前端 + 新后端会少三块右栏内容但无错误；新前端 + 旧后端时徽标/脚注按字段缺失降级，不出错误文案——两个方向都不炸，但同批发布才拿到完整新形态）。
- **用户可见变化**：
  1. 旧数据（事件线未回填的历史日子）从**空白**变为**兜底数字**——雷达小矩阵显示分区数（title 注明降级）、控制台矩阵格与时间筛选折线图显示线索合计（title 注明降级）；已回填日子完全不变。
  2. 雷达右栏从四块收敛为一块「今日该读」（8 条 + 行内徽标）+ 管线状态折叠行；「今天可能升温」卡片与「今日新起」卡片不再出现。
  3. 管线状态块默认只占一行；异常时自动展开。
- **旧数据降级**：`story_counts` 缺省（更旧后端）沿用整轴 `counts` 回退现状；`cross_day_story_count` 缺省不出脚注。

## Risks / Non-Goals

- **非目标**：不修事件线回填覆盖面（那是 `cmd/backfill-stories` 运维 + `enable-story-cross-day-continuation` 判定的事）；不改热度公式与排序键；不新增徽标的阈值判定（升温 ≥2× 沿用红线 29）。
- 风险与缓解见 design.md 风险表（口径混读、测试继承面大、折叠态藏异常等）。
