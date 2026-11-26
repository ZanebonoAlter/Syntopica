# Design — 矩阵降级兼容 + 右栏合并

> 决策落点速查：D1 雷达小矩阵逐格降级｜D2 折线/矩阵格共用口径（displayCount）｜D3 右栏单清单 + 徽标判据｜D4 top 上限 8 与响应字段去留｜D5 管线折叠状态机｜D6 不变式清单。

## D1 雷达小矩阵逐格降级（BoardRadarStory.vue）

- **口径事实**（先查清再定）：`RadarLane.counts[i] = len(day.Sections)` = 该泳道该日**分区数**（`board_radar_repository.go` buildRadarRow：`counts[i] = len(day.Sections)`）；`storyCounts[i] = day.StoryCount`（回填模式下 = 当日去重存活事件线数，历史未回填日为 0）。雷达行内**没有**逐日线索合计（lane-dynamics timeline 不进雷达行），实际可得的兜底口径就是分区数。故：**雷达小矩阵降级用分区数，控制台矩阵/折线用线索合计，各自按可得口径，文档与 spec 均写明**。
- **规则**：`cell[i] = storyCounts[i] > 0 ? storyCounts[i] : counts[i]`；`degraded[i] = storyCounts[i] === 0 && counts[i] > 0`（0 分区兜底还是 0 = 真断档，不标降级）。
- **诚实原则**（三态，review R-5）：正常格 title = `MM/DD · N 条事件线`；降级格 title/aria = `MM/DD · 无事件线（按分区数 N 显示）`；旧后端缺 `story_counts` 时走 `sections` 口径 = `MM/DD · N 个分区`（沿用本 change 之前的现状措辞，既不标降级也不冒充事件线数）。颜色档位照常用降级值算 `laneLevel(count, laneMax)`（展示层），不新增降级专属色。
- **旧响应**：normalizer 在 `story_counts` 缺省时整轴回退 `counts` 并置 `storyCountsKnown=false`，展示层据此走 `sections` 口径（「N 个分区」）；`degraded` 仅对「已知事件线数为 0」成立，旧响应恒 false（不产生「按分区数 0 显示」噪声）。
- **不变式**：兜底值不进 `lane_heat`、不进行序（后端「前 3 泳道」筛选键用原始 `StoryCounts` 合计）、不进关注分档。

## D2 矩阵格值与时间筛选折线同口径（dayCellMetrics）

- **现状**：`dayCellMetrics.total` = 各桶 `threadCount ?? fallbackClueCount` 之和（线索合计）；`storyCount` = `day.storyCount ?? (全未知 ? null : total)`。矩阵格 text 用 `storyCount`，折线 `poolDayTotal` 用 `total` —— 两处口径不一致。
- **新增派生字段**（不改既有字段语义，演进带 `knownDayTotal` 仍用 `storyCount`）：
  ```ts
  displayCount = storyCount === null
    ? null
    : storyCount > 0
      ? storyCount
      : (total > 0 ? total : (hasUnknown ? null : 0))
  degradedFromClue = storyCount === 0 && total > 0
  ```
  - `storyCount > 0` → 事件线数（有未知桶也不影响，主值已知）；
  - `storyCount === 0`（已回填版块历史未回填日）→ `total` 线索合计兜底（`total === 0` 且无未知桶 → 0，真零事件合法）；
  - `storyCount === 0` 且**线索全未知**（`total === 0 && hasUnknown`）→ `null`：后端说「事件线 0」但线索数不可得，无法区分「真零」与「未回填空白」，归未知比造 0 诚实（格显示 `?`、曲线跳过该日）；
  - `storyCount === null`（旧响应缺 raw 且全桶未知）→ null，**不造 0**；旧响应缺 raw 但桶已知时 `storyCount` 已回退 `total`，`displayCount` 与现状值相同。
- **消费方**：
  - `matrixCellView`（observationLanes.ts）：`text = displayCount ?? '?'`；title/aria 首段在降级时改 `N 条线索（0 条事件线，降级显示）`，非降级维持 `N 个事件线`。颜色通道（maxParticipantCount）与点名不动。
  - `poolDayTotal`（laneConsoleView.ts）：逐泳道求和改用 `displayCount`；`displayCount === null` 的桶跳过（对应原 `peak === null` 跳过语义，扩展为「日值不可知即跳过」）；`knownBuckets` 计数随之改挂 `displayCount !== null`（content 日必有 ≥1 分区，全未知日 `knownBuckets === 0 → null` 的守卫语义不变）。
- **文案**：`ObservationRangeControls` 曲线标注改「365 日池 · 已返回泳道的日事件线数」+「无事件线日按线索合计兜底 · 非文章数」；`TrendAxes` 默认 `unitLabel` 改「事件线数（条）」（与 `LaneTrendChart` 既有 `unit-label="事件线数（条）"` 统一，两处轴文字同口径）。
- **不变式**：`displayCount` 不进 `clueTotal`（行序排序键仍用原始线索轴）、不进休眠判定（`sectionTotal`）、不进演进带（`knownDayTotal` = `storyCount`，红线 28「演进/趋势节点 = 当日事件线数」在演进带保持原样——本 change 只同步矩阵格与顶部时间视窗图两处，演进带口径不变是有意为之，见风险 R3）。

## D3 右栏单清单 + 徽标（BoardRadarReadingRail.vue / useBoardRadarOverview.ts）

- **徽标判据**（与被删卡片的判定逐字相同，只换位置）：
  - `新起`：`!hasBaseline && hasTodayEvidence`（当日有证据 ∧ 近 3 日无基线）；
  - `升温 N×`：`hasBaseline && ratio >= 2 && hasTodayEvidence`（N = `ratio.toFixed(1)`）；
  - 无徽标 = 普通当日事件线。两判据互斥（`hasBaseline` 二值），不叠加。
  - 若无证据线挤进 Top 8（锚点日残热），两徽标都不出现——与后端三档判定一致，不新设判据。
- **已知退化（review R-7）**：当前真库 `cross_day_story_count = 0`，Top 8 全部满足「新起」判据 → 徽标暂为零区分度。展示口径**不因此隐藏徽标**（红线 29：分档 SHALL 以行内徽标表达，隐藏会让读者无法判断某条是否真的新起），退化说明留在图例/脚注，等 `enable-story-cross-day-continuation` 造出真实跨天线后区分度自然恢复。
- **残热旧线标记（review R-7）**：锚点日只剩 48h 残热、`has_today_evidence = false` 的旧线既不出「新起」也不出「升温」，与「普通当日事件线」在行内不可区分。该缺口**不会随跨天线能力落地而自动消失**（残热线仍无当日证据、仍无徽标），故条目 meta 行补一句**「今日无报道证据」**标记（复用 `.rd-read-meta` 排版 + 既有 `--color-warning` token，**不新增徽标档位、不改判据**）——把「今天没有报道证据」如实说出来。
- **删除**：「今天可能升温」卡片（storyMode 分支）、「今日新起」卡片、「另有 N 条」摘要、「N 条均已列入」说明、「今天没有新起的事件线」空态、`rd-warm-note` 升温规则句（并入徽标图例一行：新起 = 近 3 日无基线；升温 = 达自身近 3 日基线 2 倍）。
- **脚注**：`crossDayStoryCount === 0` → 一行 `rd-rail-note`「跨天延续事件线建设中」（不占卡片）；`> 0` 无任何空态；`null`（旧响应缺字段）不出脚注。`WarmingEmptyKind` 三态类型退役（其 no-cross-day 分支由脚注布尔承接）。
- **旧响应回退（storyMode=false）不动**：版块级该读清单 + 版块级升温清单原样保留（兼容路径，本 change 不扩不删）。
- **保留**：未回填/部分降级/滞后三条提示、`?story=` 深链（`openStory`）、无落点静态行、`canOpen` 判据、报头 `warmingCount`（服务端全量计数）。

## D4 top 上限 8 与响应字段去留（board_radar_repository.go）

- **8 的理由**：5（原该读）+ 3（原新起列出上限）= 8，右栏屏占不变；新起线热度排序下几乎必然进 Top 8（真库线索:事件线 ≈ 1.04~1.08、实测新起 = 该读第 6/7/8 名），单清单吃掉冗余分档。
- **字段去留**（详见 tasks/test-cases 的清单表）：
  - **保留**：`top_stories`（≤8，条目自带 `has_baseline`/`ratio`/`has_today_evidence`/`anchor_date`/`report_date`）、`warming_count`（报头「N 处出现升温」）、`cross_day_story_count`（脚注判据）、`unbackfilled_*`/`no_story_*`/`lagging_board_ids`/`partial`/`degraded_board_ids`（红线 30 标记全保）。
  - **删除**：`warming` 数组、`new_stories` 数组、`new_count`、`new_total_count`、常量 `radarWarmingLimit`/`radarNewStoryLimit`、去 Top 互斥逻辑（`topIDs`）、`newTotal` 计数。`WarmingCount` 改为循环内纯计数（不再截断）。
  - **互斥契约退役理由**：单清单下不存在第二块可重复；「该读不重复展示同一事件线」由单清单结构性保证（全局排序后截 8，无重复 id）。三态新起摘要（另有 N 条/均已列入/没有新起）失去消费方，留字段即死契约，spec 同步删除对应 Scenario。
- **兼容**：新旧前端 × 新旧后端四个象限都不炸——旧前端忽略新字段少；新前端对缺字段（`cross_day_story_count` null、`top_stories` 条目缺 `has_baseline`）按 boolean false 降级（不出徽标/脚注）。

## D5 管线状态块折叠状态机（BoardRadarStatusBlock.vue）

- **状态**（review R-3 修订，**三个量**：判据粘滞 / 自动态 / 用户手动态）：`autoCollapsed` 跟随 `abnormal`（默认折叠），`manualCollapsed = null` 时生效；渲染值 `collapsed = manualCollapsed ?? autoCollapsed`。用户手动开合后 `manualCollapsed` **粘滞**（手动展开后正常态不自动收回，直到用户自己收起），直到**异常态发生真实变迁**——变迁时手动意图作废、交还控制权给自动态（否则「用户手动收起 + 真实异常出现」不会自动展开，违反 spec「异常态 SHALL 自动展开」；「异常消除后 SHALL 回到折叠摘要」也永远轮不到）。即：异常 → 展开（含挂载即异常）、异常消除 → 回折叠摘要，都由自动态接管。
- **取数窗口粘滞**：`abnormal` 的「过点未出日报」支路改由 `overdueMissing` 记住**上一次已知判定**——`!pollReady` / `overviewLoading` / `overviewError` 期间不重算、不判「异常消除」（加载中不是消除，是未知，与 R-17 同纪律），避免挂载/重试/生成后 `load()` 触发「收起→展开」抖动。
- **abnormal 判据**（任一为真）：
  1. `phase === 'failed'`（生成请求失败）；
  2. `failedCount > 0 || embeddingFailedCount > 0`（队列失败任务，红色提示同源）；
  3. `pollReady && !todayReady && !isBeforeSchedule`（已过生成时刻仍未出当日日报；未到点不算异常）——**只在数据已知时判定**（取数窗口/取数失败期间沿用上一次判定，见上），否则异常会被临时撤销；
  4. `activeCount >= 100 || embeddingActiveCount >= 100`（明显积压阈值，UI attention 常量，非业务闸门——生成闸门仍是「任一活跃即禁用」，不受此阈值影响）。
- **摘要行**：`{状态词} · 日报 {todayReportCount}/{boardTotal} · 队列 {activeCount}{embeddingActive>0 ? '+'+m : ''}`（两条队列量纲不同——文章 / 标签——SHALL 分段呈现、SHALL NOT 相加成一个数，review R-6）；状态词优先级：读取中（`!pollReady`）→ 有失败（**带条数**，同样按分段写法「队列有失败 2+1」：标签队列失败文章数 + Embedding 队列失败标签数，折叠不得藏异常计数，review R-4）→ 日报未生成 → 队列积压 → 正常。对账未就绪时整行「管线状态读取中」（R-17：不把未知当业务结论）；总览读取失败时日报段显示「读取失败」。摘要行是 `<button>`（展开/收起），带 `aria-expanded`，`data-testid="radar-status-summary"`。
- **渲染**：折叠态 `v-if` 不渲染五项事实/进度条/生成入口（非 v-show——折叠就是不在 DOM）；展开态全部复用既有模板与类。五项事实、手动生成入口、双队列闸门三个 Requirement 的其余契约零改动。

## D6 不变式清单（评审对照用）

1. 热度公式/窗口/排序键不变：heat desc → participants desc → story id asc；`heat-v1-48h-hl24h`。
2. 锚点日契约不变：各版块自己的最新日报日、条目自带锚点日、滞后如实呈现。
3. 红线 30 标记全保留：unbackfilled（不产条目）/no_story/lagging/partial+degraded。
4. 事件线身份/归属/去重算法零触碰（`enable-story-cross-day-continuation` 辖区）。
5. `?story=` 深链、滞后/未回填/排序不完整提示零触碰。
6. R-17：首次取数失败不出业务空态；管线块读取中语义不变。
7. 生成闸门不变：双队列任一活跃即禁用手动生成（折叠只是视觉态，不参与闸门）。

## 风险表

| # | 风险 | 缓解 |
| --- | --- | --- |
| R1 | 两套兜底口径（雷达=分区数、控制台=线索合计）被误读为不一致 | spec/flow/API 文档三处都写明「各自按实际可得口径」；title/aria 明示兜底来源；同一条「事件线数 > 0 用事件线数」的规则本体一致 |
| R2 | 测试继承面大（三档契约刚在上一 change 改过，本 change 再改） | test-cases ⓪ 继承与调整表逐行列旧断言处置；先写红测试再实现；退役 Scenario 的测试同步删除不留死断言 |
| R3 | 演进带（knownDayTotal）仍用原始事件线数，与矩阵格降级值不同（同页两种数字） | 有意为之：演进带是单泳道近窗视图（档内日多已回填），降级需求集中在 365 池历史段与矩阵格；spec 已写明演进带口径不变；`enable-story-cross-day-continuation` 落地后回填覆盖扩大，差异自然收敛 |
| R4 | 折叠态藏住异常（用户不展开看不到积压） | 摘要行本身带异常词与计数；异常自动展开；阈值 100 的 attention 常量在 design 留档 |
| R5 | 新旧端混布（新前端 + 旧后端）下徽标/脚注缺失被当 bug | normalizer 对缺字段按 null/false 降级（不出徽标不出脚注，不报错）；proposal 部署节写明同批发布拿完整形态 |
| R6 | 删除响应字段后旧测试断言编译失败 | 字段删除是编译期可见的，测试同步删改（test-cases 表逐行处置）；gofmt/vet/lint 兜底 |
