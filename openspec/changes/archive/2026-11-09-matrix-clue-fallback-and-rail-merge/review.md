# Review — matrix-clue-fallback-and-rail-merge

> 审查类型：独立只读 review（类型 A —— openspec change 制品 ↔ 实现 diff 一致性 / 红线符合性）。
> 审查快照：**2026-11-09 14:36–14:38**（工作树状态）。审查期间实现侧在 14:32:05–14:36:44 有并发改动落地
> （`boardRadar.ts` / `BoardRadarStory.vue` / `BoardRadarStatusBlock.vue` / `board_radar_repository.go` /
> `board_radar_stories_test.go` 及三个前端测试文件），
> 本报告全部结论均按最终快照复核过一遍；R-2 记录的就是这轮并发改动中被修掉的那条。
> 纪律：全程只读（只写本文件），未改实现、未改其它制品、未 commit、未跑重建/生产库/全量测试。

## 总评

实现侧把用户拍板的四项需求都落到了代码里，且落点比制品描述的更细：雷达小矩阵是**逐格三态**（事件线 / 分区数降级 / 旧后端分区口径）而不是两态，
控制台矩阵格与顶部折线共用 `dayCellMetrics.displayCount` 单一派生，右栏确实只剩一块 8 条清单 + 行内徽标，
管线块确实折叠成一行摘要并带异常自动展开。spec 三份 delta 覆盖的主 spec Requirement 全部是**同名整条 MODIFIED**，
归档同步后不会残留旧 Scenario（已逐条比对主 spec 的 18 条 Scenario 与 delta 的 19 条，delta 是超集）。

没有发现能阻断归档的缺陷（**0 High**）。发现 **3 Medium**（其中 R-2 在审查期间已被实现侧修掉，现记「已修复」）+ **6 Low**，
其中最值得先修的是 **R-1**：本 change 的立项动机之一就是「删掉死契约」，但**客户端契约文档 `docs/reference/api/daily-reports.md` 里还留着
被删 UI 的死契约**（三档空态文案），等于死契约只清了一半。

## 一、8 个重点项逐条结论

### 1. 降级规则是否精确且不误导 —— **通过**

- **规则与边界**：`BoardRadarStory.vue:54-66` 的 `laneCells` 是 `storyCounts[i] > 0 ? storyCounts[i] : counts[i]`，与 design D1、
  spec delta（`specs/workshop-radar/spec.md:12`）、红线 28 逐字一致。
- **真断档**：事件线 0 且分区数 0 → 不标降级、显示「0 条事件线」（`degraded = story === 0 && sectionCount > 0`，
  `BoardRadarStory.vue:61`）。零是合法已知值，没有被当成空白 —— 与 `dayCellMetrics` 那侧
  `displayCount = storyCount === null ? null : storyCount > 0 ? storyCount : (total > 0 ? total : (hasUnknown ? null : 0))`
  （`front/app/api/normalizers/laneDynamicsWindow.ts:205-209`）的「真零 vs 未知」三分法一致。
- **长度不一致**：`laneCells` 以 `storyCounts` 的长度迭代，`sections[i] ?? 0` 兜底缺位（不产 NaN，对应 test-cases 白盒 B2）。
  后端两侧数组恒等长（`board_radar_repository.go:400-410`，`counts`/`storyCounts` 均 `make([]int, LaneDynamicsDefaultDays=14)`），
  所以「storyCounts 比 counts 短 → 小矩阵少渲染几格、与 14 日轴表头错位」当前**不可达**，属可接受的残余风险。
- **静默混用路径：已消除**（这是本轮实现侧 14:32 的并发修复）。`boardRadar.ts:237` 新增 `storyCountsKnown`，
  `BoardRadarStory.vue:52/60/76-83` 落成三态口径 `story | degraded | sections`：旧后端缺 `story_counts` 时
  显示「N 个分区」而不是冒充「N 条事件线」。于是**任何一格的 title/aria 都必然能区分口径**：
  「N 条事件线」/「无事件线（按分区数 N 显示）」/「N 个分区」三选一，无第四条路径。

### 2. 两处口径是否真的同源 —— **通过（差异是写明的，不是含糊带过）**

- **规则同构、口径不同**：雷达小矩阵兜底用**分区数**、控制台矩阵/折线兜底用**线索合计** —— 量纲确实不同，但两处的**判定规则**是同一条
  （「事件线数 > 0 用事件线数，否则按各自实际可得的量兜底」）。雷达行不下发逐日线索合计
  （`buildRadarRow` 只给 `counts`/`storyCounts`，`board_radar_repository.go:400-423`），这是数据面约束而非实现偷懒。
- **写在哪里**（不是含糊）：design D1 首段 + 风险表 R1、`specs/workshop-radar/spec.md:12`、
  `specs/workshop-board-console/spec.md:9,65`、红线 28（`docs/reference/flow/semantic-board.md:411`）、
  `docs/reference/api/daily-reports.md`、ui-verification A-2 —— 六处都明写「雷达=分区数 / 控制台=线索合计，各自按可得口径」。
- **用户可区分**：两处降级文案互不相同（雷达「按分区数 N 显示」/ 控制台「按线索合计兜底显示」），
  且控制台曲线标注与 Y 轴单位已同步改口径（`ObservationRangeControls.vue:236`「无事件线日按线索合计兜底 · 非文章数」、
  `TrendAxes.vue:13`「事件线数（条）」）—— **不存在「以为同步了其实没同步」的静默路径**。

### 3. 后端字段删除是否留下死契约 —— **基本通过（1 处残留，见 R-1）**

- ① **spec delta**：明确写了「SHALL NOT 再返回独立的升温/新起清单数组与去 Top 计数」（`specs/workshop-radar/spec.md:171,194`）✓
- ② **API 文档**：字段表已删 `warming`/`new_stories`/`new_count`/`new_total_count` 四行，并在 :298 留了移除说明 ✓；
  **但 :348 的散文段仍在要求渲染三档空态文案 → R-1**
- ③ **前端 normalizer**：不再读取旧字段（`normalizeRadarStoryFeed` 的返回体无 `warming`/`newStories`/`newCount`/`newTotalCount`）；
  `boardRadar.ts:112-115` 只保留**可选类型声明**并注明「旧后端仍可能下发，前端不再消费——留着只为响应类型兼容」，属有意留痕不是消费 ✓
- ④ **测试**：旧断言已删（`TestRadarStoryFeedNewStoriesExcludeTopStories` 等 4 个），改写为「raw 携带旧字段 → 归一结果不含它们」
  （`front/app/api/normalizers/boardRadar.test.ts:251-258,348-350`）✓
- ⑤ **其它 change 的依赖**：主 spec `openspec/specs/workshop-radar/spec.md:52,107,113-114,119-120,137-138` 仍含旧三态契约，
  但本 change 用 `## MODIFIED Requirements` **整条替换同名 Requirement「今日该读与升温提示」**，delta 的 19 条 Scenario 覆盖主 spec 的 18 条（超集），
  归档同步后旧 Scenario 整体消失，不会残留 ✓

### 4. 徽标语义是否守恒 —— **通过（后端计数口径见 R-2，已在审查期间修好）**

- **逐字守恒**：前端 `isNewStory = !hasBaseline && hasTodayEvidence`（`BoardRadarReadingRail.vue:77-79`）、
  `isWarmingStory = hasBaseline && hasTodayEvidence && ratio >= 2`（:81-83）—— 与被删卡片的判据逐字相同，只换展示位置 ✓
- **红线 29 未被破坏**：无基线时 `ratio` 为 0 且**不渲染倍率**（渲染分支只在 `isWarmingStory` 为真时进入），
  没有以 0.0× 或 ∞ 硬凑 ✓；徽标互斥（`hasBaseline` 二值 + `v-if`/`v-else-if`）✓
- **后端全量计数**：`board_radar_repository.go:622` 现为
  `item.HasBaseline && item.Ratio >= radarWarmingRatio && item.HasTodayEvidence` —— 与前端徽标、API 文档（:303）、spec delta（:47）、
  结构体注释（:136-138）四者同口径（**这条是审查期间 14:34:37 修掉的，见 R-2**）。
- **退化情形**：当前真库 `cross_day_story_count = 0`，8 条全是「新起」，徽标无区分度 —— 实现侧已在
  `ui-verification/report.md:97-99` 如实记录，设计层未处理，见 R-7。

### 5. 管线折叠是否藏了异常 —— **有 1 个交互缺陷（R-3）+ 1 个表达缺口（R-4）**

- **摘要行确实暴露异常词**：`summaryState`（`BoardRadarStatusBlock.vue:225-234`）给出
  读取中 / 队列有失败 / 生成失败 / 读取失败 / 日报未生成 / 队列积压 / 管线正常；`summaryReport` 给「日报 x/y」；
  `summaryQueue` 给「队列 n(+m)」。折叠态不藏异常信号 ✓
- **异常自动展开**：`watch(abnormal, …, { immediate: true })`（:218）覆盖挂载即异常 ✓；
  `abnormal` 的四个判据（phase failed / 队列失败任务 / 过生成时刻未出日报 / 活跃量 ≥ 100）与 spec delta 逐条对齐 ✓
- **但**：取数窗口（`overviewLoading`）会把「过点未出日报」这条异常临时撤销，且手动态不粘滞 → R-3；
  失败任务的**条数**只在展开态可见 → R-4。

### 6. 红线 30 是否被合并削弱 —— **通过**

- 三条提示全部在位、同属 `rd-rail-note` 同一视觉层级、且排在清单**之前**：
  `radar-unbackfilled`（`BoardRadarReadingRail.vue:100-102`「N 个版块事件身份未回填，未纳入事件线排序」）、
  `radar-partial`（:104-106「…本次事件线排序不完整」—— `partial` 时**明示了「排序不完整」**）、
  `radar-lagging`（:108-110「N 个版块数据滞后（各条目按自己的锚点日计）」）✓
- 合并没有让任何一条降级到别的层级或被脚注吞掉（跨天延续脚注 `radar-crossday-note` 是另一条独立信息）✓

### 7. 验收证据强度 —— **基本通过（2 处证据缺口，见 R-8）**

- A-1～A-10 都有可复现的断言输出（DOM 计数 / aria 原文 / 接口字段 / 视口尺寸 / 像素高度），
  且**没有把单测覆盖写成实测通过**：A-2 明确标注「pass（单测覆盖）」，A-7 的异常自动展开也明说「由单测覆盖」✓
- 缺口：A-8 的三条提示里只有「滞后」有正向实测，未回填/排序不完整只有反向证据；A-5 用的两个选择器在新代码里已不存在（断言空转）→ R-8

### 8. 回归风险（`top_stories` 5→8）—— **通过**

- **日报「今日重点」不受影响**：它走 `ReportStoryCandidates` / `rankReportStoryCandidates`
  （`backend-go/internal/topicgraph/repository/report_story_highlights.go:152-201`），自己的 top-8 截断在 prompt 集环节，
  **不共用** `radarTopStoryLimit` ✓
- **主 spec 里的「5 条」已随 delta 改成「8 条」**（`specs/workshop-radar/spec.md:128`）✓
- **既有断言**：唯一受影响的是 `TestRadarStoryFeedTopStoriesIgnoreParticipantFloor`（造 5 条候选 → 5 ≤ 8 全部入榜，仍成立），
  但断言文案残留旧语义 → R-9；`TestRadarOverviewStoryFeedMergesBoardsAndSorts` 已更新为「候选 6 条 ≤ 上限 8」✓

## 二、问题清单

### R-1 2026-11-09【Medium】客户端契约文档残留已删 UI 的死契约（三档空态文案）
- **状态**：已修复
- **处置**：`docs/reference/api/daily-reports.md:348` 改写为新契约——单清单为空→「今天没有可识别的事件线。」；`cross_day_story_count == 0`→一行脚注「跨天延续事件线建设中。」；`> 0`→无任何空态；已删除的独立升温/新起卡片与「暂未触发升温条件。」「N 条均已列入今日该读」「今天没有新起的事件线。」三态摘要明写 SHALL NOT 再渲染（「新起/升温 N×」只在存在时以行内徽标出现）；SHALL NOT 写成「今天没有事件」保留。全文复查 `new_stories`/`new_count`/`new_total_count`/「暂未触发升温条件」/「今日新起」/「另有」——其余命中均为**移除说明或禁止性表述**（:298 / :304 / :315），无残留死契约；另同步 :301 徽标条目措辞（残热旧线标注，见 R-7）。
- **位置**：`docs/reference/api/daily-reports.md:348`
- **事实**：该行仍是旧三档契约：
  > `- **排序不完整 ≠ 无事件**：三档为空时分别渲染「今天没有可识别的事件线。」「暂未触发升温条件。」与新起档三态文案（截断摘要 / 「N 条均已列入今日该读」 / 「今天没有新起的事件线。」，见上）…`

  但本 change 已删除这些 UI：组件里不存在「暂未触发升温条件」（storyMode 分支无升温卡片，只有旧响应回退分支 `!storyMode` 的 `rd-warm-block`，
  `BoardRadarReadingRail.vue:183`）、不存在「N 条均已列入今日该读」、不存在「今天没有新起的事件线」；
  spec delta 明文 SHALL NOT（`specs/workshop-radar/spec.md:40,71,117,123`）。
  本 change 的 tasks 7.1 只写了「字段表删四行 + JSON 示例同步」（`tasks.md:50`），**散文段漏改**。
- **影响**：本 change 的立项动机之一是「删掉死契约」，现在死契约只清了一半 —— 字段清了、散文契约没清。
  `docs/reference/api/` 是客户端面向的契约文档，归档后读者据此实现会重新做出被明确删除的三块 UI/摘要行；
  且该文档与归档后的 spec delta 直接矛盾（同一件事两处写法相反）。
- **处置建议**：把 `:348` 这一条改写为新契约：清单为空 → 「今天没有可识别的事件线。」；`cross_day_story_count == 0` → 一行脚注；
  `> 0` → 无任何空态；`partial`/未回填/滞后 → 三条 `rd-rail-note` 明示「排序不完整」。删掉「暂未触发升温条件」与新起三态文案的表述。

### R-2 2026-11-09【Medium】后端 `warming_count` 缺「当日有证据」下界（报头计数与徽标判据不同源）
- **状态**：已修复
- **位置**：`backend-go/internal/topicgraph/repository/board_radar_repository.go:622`（判据）；同文件 `:136-138`（结构体注释）、`:617-621`（计数注释）
- **事实**：审查快照 14:33 时该行为 `if item.HasBaseline && item.Ratio >= radarWarmingRatio {`，**没有** `HasTodayEvidence`；
  而同文件结构体注释写「whose own heat reached 2× their own 3-day baseline **with today evidence**」、
  计数注释写「（自身基线 ratio ≥ 2 **且当日有证据**）」、API 文档 `docs/reference/api/daily-reports.md:303` 写
  「`ratio = heat / baseline_heat ≥ 2` **且** `has_today_evidence = true`」、spec delta `specs/workshop-radar/spec.md:47` 同样要求当日有报道证据、
  前端徽标 `BoardRadarReadingRail.vue:82` 也带 `item.hasTodayEvidence` —— 只有实现没带。
  **审查期间（2026-11-09 14:34:37）实现侧已改为**
  `if item.HasBaseline && item.Ratio >= radarWarmingRatio && item.HasTodayEvidence {`（判定位于 `finalizeRadarStories`，
  `board_radar_repository.go:536` 起的函数体内），与四处口径一致，本条随之关闭。
  14:36:44 又补了白盒回归用例 `TestFinalizeRadarStoriesWarmingCountRequiresTodayEvidence`
  （`backend-go/internal/topicgraph/repository/board_radar_stories_test.go:711-735`）：
  「有基线 + ratio≥2 + 当日无证据」不计入、「有基线 + ratio≥2 + 当日有证据」计入、「无基线」不计入 —— 该判据已被钉住。
- **影响**（修复前的实际风险，已消除）：报头「升温 N」（`BoardRadarMasthead.vue:65` 渲染 `warmingCount`）与行内「升温 N×」徽标会各说各话
  ——计数可能 > 页面上可见的徽标数。按当前热度公式（48h 窗 / 24h 半衰期、基线 = 锚点日前 3 个日终日均）可推：
  无当日证据时 `heat₀ ≤ 0.5 × heat₁` 且 `baseline ≥ heat₁/3` ⇒ `ratio ≤ 1.5 < 2`，所以当时**尚不可达**，属潜伏缺陷；
  但热度公式/基线窗一旦调整就会静默失配。
- **处置建议**：已修复且已补白盒回归用例，无需再动。建议把「无当日证据 ⇒ ratio ≤ 1.5 < 2」这条推导
  写进 design D4 或 `docs/reference/flow/daily-report.md` 的热度口径段，说明「当日有证据」下界当前是**冗余但显式**的护栏
  （公式一旦改动即成为唯一判据），避免后人误以为它只是文案装饰而顺手删掉。

### R-3 2026-11-09【Medium】折叠状态机把「异常状态变化」直接当「折叠状态」：取数窗口内异常被临时撤销 → 收起再展开抖动；用户手动态不粘滞
- **状态**：已修复
- **处置**：状态机拆成三个量（`front/app/features/tags/components/console/BoardRadarStatusBlock.vue`）——① **判据粘滞**：`overdueMissing` latch 记住「过点未出日报」的上一次已知判定，取数窗口（`overviewLoading`/`overviewError`）与 `!pollReady` 期间不重算、不判「异常消除」（加载中是未知不是消除），`abnormal` 在窗口内不翻转 → 不再收起再展开；② **自动态**：`autoCollapsed` 跟随 `abnormal`；③ **用户手动态**：`manualCollapsed`（null = 跟随自动态），手动开合后**粘滞**（正常态不自动收回），直到**异常态发生真实变迁**——变迁时手动意图作废、交还控制权给自动态（否则「用户手动收起 + 真实异常出现」不会自动展开，违反 spec「异常态 SHALL 自动展开」；「异常消除后 SHALL 回到折叠摘要」也永远轮不到）。渲染值 `collapsed = manualCollapsed ?? autoCollapsed`。**先红后绿**（新增 4 条用例）：① 取数窗口内异常判据变「未知」不翻转 ② 用户手动展开后正常态不自动收回 ③ 用户手动收起后真实异常出现仍自动展开 ④ 异常消除回折叠摘要。
- **位置**：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue:207-222`（`abnormal` + `watch`）、
  `front/app/features/tags/composables/useBoardRadarOverview.ts:38-54`（`load()` 期间 `loading = true`）、
  `front/app/features/tags/components/console/BoardRadarView.vue:58,65,71-72`（`load()` 的三个触发点）
- **事实**：
  ```ts
  const abnormal = computed(() => {
    if (phase.value === 'failed') return true
    if (failedCount.value > 0 || embeddingFailedCount.value > 0) return true
    if (pollReady.value && !overviewLoading.value && !overviewError.value
      && !todayReady.value && !isBeforeSchedule.value) return true
    return activeCount.value >= RADAR_QUEUE_ATTENTION || embeddingActiveCount.value >= RADAR_QUEUE_ATTENTION
  })
  const collapsed = ref(true)
  watch(abnormal, (isAbnormal) => { collapsed.value = !isAbnormal }, { immediate: true })
  ```
  「过点未出日报」这一支被 `!overviewLoading.value` 屏蔽；而 `overviewLoading` 就是雷达总览的 `loading`
  （`useBoardRadarOverview.ts:39` 置 true、`:53` 置 false），在**每次** `load()` 期间为真——挂载、重试按钮、
  手动生成日报后（`BoardRadarView.vue:71-72` `handleGenerated`）、新建版块后（`:65`）都会走 `load()`。
  于是：异常成立 → 用户点「重试」/新建版块 → `loading=true` → `abnormal` 变 false → `watch` 把 `collapsed` 写成 true（**自动收起**）→
  请求回来且日报仍未出 → `abnormal` 又为 true → `watch` 把 `collapsed` 写成 false（**重新展开**）——一次「收起→展开」抖动，
  而这期间**什么都没被解决**，只是从「已知异常」变成「未知」。
  这与同文件其它地方的 R-17 纪律不一致：`summaryState`（:229）与 `todayText` 都在 `overviewLoading` 时返回「读取中」而不是业务结论。
- **影响**：可见的 UI 抖动（用户刚展开就被重置）；语义上违反 spec delta「异常消除后 SHALL 回到折叠摘要」（加载期不是「消除」，是「未知」）；
  另外 `watch` 无条件覆盖 `collapsed`，任何一次异常状态跳变（含队列积压跨过 100 又回落）都会丢弃用户手动展开/收起的选择，
  与 spec delta 同句里的「用户手动展开/收起随时可用」相抵。
- **处置建议**：把「用户意图」与「自动态」解耦——例如 `const manual = ref<boolean | null>(null)`、
  `toggleCollapsed()` 写 `manual.value = !collapsed.value`，渲染用 `collapsed = manual.value ?? !abnormal.value`；
  并把「过点未出日报」这条异常在取数窗口内保持粘滞（用上一次判定的结果，不用 `overviewLoading` 直接屏蔽）。

### R-4 2026-11-09【Low】折叠摘要行不暴露失败任务条数
- **状态**：已修复
- **处置**：折叠摘要状态词改为「队列有失败 2+1」——折叠态即带失败词与条数（`failedCountText`）；两条队列量纲不同（标签队列=文章、Embedding 队列=标签），按**分段写法**呈现并**不相加**（与摘要队列段 `队列 40+3` 同款约定，见 R-6），`title` 注明「量纲不同，不相加」。新增用例钉住折叠态摘要含 `2+1` 且不含相加后的「队列有失败 3」。
- **位置**：`front/app/features/tags/components/console/BoardRadarStatusBlock.vue:225-234`（`summaryState`）、`:364-366`（`radar-status-queue-failed`）
- **事实**：`summaryState` 在失败时只给状态词「队列有失败」，**不带条数**；带条数的「N 个失败」/「Embedding 队列 N 个失败」
  两个节点在 `<template v-if="!collapsed">`（:319）内部，折叠态不渲染。而 spec delta 与 design 风险表 R4 都要求
  「摘要行本身 SHALL 呈现异常词与计数（折叠 SHALL NOT 藏异常信号）」。
- **影响**：折叠态下用户看得到「有失败」但看不到「几个失败」，与 R4 的缓解措辞（异常词**与计数**）有出入。
  失败态会自动展开，条数一跳即见，故影响有限。
- **处置建议**：在 `summaryState` 的「队列有失败」分支带上条数（如「队列有失败 3」），或在 `summaryQueue` 段附失败计数。

### R-5 2026-11-09【Low】design D1 与 test-cases 边界值未回写第三种格子口径（`sections` 模式）
- **状态**：已修复
- **处置**：`design.md` D1 诚实原则改为**三态**（`story` / `degraded` / `sections`）——旧后端缺 `story_counts`（`storyCountsKnown=false`，整轴回退 `counts`）走 `sections` 口径「N 个分区」，既不冒充事件线数也不加降级标注；「旧响应」条同步改写为「normalizer 回退并置 `storyCountsKnown=false`，展示层据此分流」。`test-cases.md` 边界值改为「旧后端缺 `story_counts` → 走 `sections` 口径显示「N 个分区」，不标降级、不得写成「N 条事件线」」。spec 无需改（与实现相容）。
- **位置**：`openspec/changes/matrix-clue-fallback-and-rail-merge/design.md:9-10`；`openspec/changes/matrix-clue-fallback-and-rail-merge/test-cases.md:125`
- **事实**：design D1「诚实原则」只写两态：「正常格 title = `MM/DD · N 条事件线`；降级格 title/aria = `MM/DD · 无事件线（按分区数 N 显示）`」；
  「旧响应」条写「`degraded` 恒 false（count>0 时同值；count=0 时 counts=0）——不产生「按分区数 0 显示」噪声」。
  但实现侧 14:32 的并发修复已落成**三态**（`BoardRadarStory.vue:52` `type LaneCellMode = 'story' | 'degraded' | 'sections'`），
  旧后端缺 `story_counts` 时显示「N 个分区」（`:81`）。同时 test-cases 边界值 `:125` 仍写
  「`counts[i] === storyCounts[i] === 0`（旧后端整轴回退）→ 不触发降级标注（同值判定）」——旧后端现在显示的是「N 个分区」，不是「N 条事件线」。
  spec delta 侧的措辞「事件线数未知（旧后端缺 `story_counts`，整轴回退 `counts`）时**沿用现状口径**不加降级标注」
  （`specs/workshop-radar/spec.md:12`）与实现是相容的（「N 个分区」正是本 change 之前的现状措辞），故 **spec 无需改，design/test-cases 需要回写**。
- **影响**：制品与实现的口径清单不一致；按 design D1 实现会退回两态（把旧后端的分区数写成事件线数），正是本次要消灭的静默混用。
- **处置建议**：design D1 补第三态（`sections`：旧后端缺 `story_counts` → 「N 个分区」），
  test-cases `:125` 边界值改为「旧后端整轴回退 → 按分区措辞显示，不标降级」。

### R-6 2026-11-09【Low】spec delta 与 flow 文档写「队列 n」，实现是「队列 40+3」（两条队列量纲不同，不得相加）
- **状态**：已修复
- **处置**：`specs/radar-pipeline-status/spec.md` 正文与「默认折叠成一行摘要」Scenario 改写为「队列 {标签队列文章数}[+{Embedding 队列标签数}]」并明写两条队列**量纲不同、SHALL 分段呈现、SHALL NOT 相加成一个数**；`docs/reference/flow/semantic-board.md` L0 雷达段同步改写（并补「异常词与计数在摘要行就可见」）。`design.md` D5 摘要行补队列量纲与失败条数分段定义（`队列 40+3` / `队列有失败 2+1`）。
- **位置**：`openspec/changes/matrix-clue-fallback-and-rail-merge/specs/radar-pipeline-status/spec.md:8`；
  `docs/reference/flow/semantic-board.md:26`；实现 `front/app/features/tags/components/console/BoardRadarStatusBlock.vue:245-254`
- **事实**：spec delta 与 flow 文档都写「正常态只占一行，形如「状态词 · 日报 x/y · **队列 n**」」；
  实现按 14:32 的并发修复改成 `队列 40+3`（标签队列待打标**文章**数 + Embedding 队列待向量化**标签**数，**不相加**），
  实现注释写「按 design D5 分段呈现 `队列 40+3`，MUST NOT 相加成一个数（matrix-clue R-3）」——但 design D5 并未规定这种分段格式，
  反而与 spec delta 一样写的是「队列 n」。
- **影响**：归档后 spec 主文件会写「队列 n」，等于授权把两条不同量纲的队列加成一个数 —— 正是实现侧刚刚修掉的错；
  同时实现注释引用了一个并不支持它的 design 依据。
- **处置建议**：把 spec delta 与 `docs/reference/flow/semantic-board.md:26` 的「队列 n」改成
  「队列 <标签队列文章数>[+<Embedding 队列标签数>]（两者量纲不同，不相加）」；design D5 补同一条格式定义。

### R-7 2026-11-09【Low】徽标在当前真库退化为零区分度；残热旧线无任何标记
- **状态**：已修复
- **处置**（两半分别处置）：① **徽标零区分度** = 已知**数据缺口**（真库 `cross_day_story_count = 0` → 8 条全「新起」），**设计接受**：`design.md` D3 记明「不因此隐藏徽标」——红线 29 要求分档以行内徽标表达，隐藏会让分档失去行内表达；区分度随 `enable-story-cross-day-continuation` 落地自然恢复，图例行已写明判据。② **残热旧线无标记** = 缺口**不会随跨天线能力落地而自动消失**（残热线仍无当日证据、仍无徽标），故**实现**标记：`BoardRadarReadingRail.vue` 条目 meta 行对 `hasTodayEvidence = false` 的条目补「今日无报道证据」（复用 `.rd-read-meta` + 既有 `--color-warning` token，**不新增徽标档位、不改判据**），新增用例钉住「两徽标都不出 + meta 行有标记」。同步 `ui-design.md` §3（复用契约）与 `docs/reference/api/daily-reports.md:301`（徽标条目措辞）。
- **位置**：`front/app/features/tags/components/console/BoardRadarReadingRail.vue:77-83,126-141`；
  `openspec/changes/matrix-clue-fallback-and-rail-merge/ui-verification/report.md:45,97-99`
- **事实**：真库 `cross_day_story_count = 0`，实测 `radar-badge-new-*` = 8、`radar-badge-warm-*` = 0
  （`ui-verification/report.md:45`）——8 行挂同一个徽标，徽标不携带任何区分信息；实现侧已在 report:97-99 记为「观察（非缺陷）」，
  但 **design D3 未处理该退化情形**。另：`has_today_evidence = false` 的条目（锚点日只剩 48h 残热的旧线）既不出「新起」也不出「升温」，
  行内 meta 只有「热度 X · 截至 日期」，与「普通当日事件线」在视觉上不可区分（design D3 记了「两徽标都不出现」，但没有展示兜底）。
- **影响**：不违反任何现行 SHALL；属展示层信息缺口——用户既看不出「为什么全是新起」，也看不出某条线今天其实没有报道证据。
- **处置建议**：design 层补一条退化处理（例如：清单内全部条目同档时不渲染徽标、改为在图例行说明「当前全部为新起」；
  残热线在 meta 行加「今日无报道证据」）。等 `enable-story-cross-day-continuation` 落地后徽标自然恢复区分度，但残热线的标记缺口不会自动消失。

### R-8 2026-11-09【Low】验收证据强度不足：A-8 只有反向证据、A-5 的选择器已不存在（断言空转）
- **状态**：已修复
- **处置**：`ui-verification/report.md` A-5 依据从「选择器查询恒 false」改为「**源码位置 + 渲染树**」双依据——`rd-warm-block` 仅存在于 `v-if="!storyMode"` 的旧响应回退分支（storyMode=true 下不渲染）、`rd-new-block`/`radar-new-summary` 组件源码 `grep` 零命中、渲染树无对应节点、右栏整块文本不含「今天可能升温」「今日新起」「另有」「均已列入」「今天没有新起的事件线」。A-8 明确区分**正向实测**（滞后提示，DOM 实测）与**反向证据**（未回填 / 排序不完整，真库为 0/false），并写明两条的正向路径由 `BoardRadarView.test.ts` 的未回填 / partial / 滞后渲染用例覆盖。
- **位置**：`openspec/changes/matrix-clue-fallback-and-rail-merge/ui-verification/report.md:51`（A-5）、`:61-63`（A-8）
- **事实**：A-8 标题为「滞后/未回填/排序不完整提示保留【pass】」，依据是
  「`lagging_board_ids = [3122, 246030]` → 右栏提示「2 个版块数据滞后」在位；`partial = false`、`unbackfilled_board_count = 0`、
  `no_story_board_count = 0` 时不出提示（无假阳性）」——三条提示里**只有「滞后」有正向实测**，未回填与排序不完整两条在真库上
  分别是 0 与 false，只有反向证据（不出现）。A-5 用「DOM 查询 `.rd-new-block` = **false**、`[data-testid="radar-new-summary"]` = **false**」
  证明卡片已删，但这两个选择器在新代码里**已经完全不存在**（组件内已无 `rd-new-block` 类与 `radar-new-summary` 属性），
  查询恒为 false，属空转证据。
- **影响**：验收强度不足（不构成缺陷——这两条路径有组件单测覆盖，见 `tasks.md` 3.3），
  但 A-8 标【pass】而只验到一条提示，容易被读成「三条都实测过」。
- **处置建议**：A-8 注明未回填/partial 的正向路径由哪个单测覆盖（或用可造 `partial=true` 的夹具补一条 DOM 断言）；
  A-5 改用「组件源码中不存在该 class / 该 data-testid」或「渲染快照无对应节点」作为证据，避免空转查询。

### R-9 2026-11-09【Low】后端测试断言文案残留旧上限语义
- **状态**：已修复
- **处置**：`board_radar_stories_test.go:629` 断言文案改为「造 5 条候选 → 5 条全部入榜（上限 8，未触顶）」——断言值不变（5 条），文案不再暗示上限仍是 5。
- **位置**：`backend-go/internal/topicgraph/repository/board_radar_stories_test.go:629`
- **事实**：`require.Len(t, feed.TopStories, 5, "5 条全进该读")` —— 该用例造 5 条候选，5 ≤ 8 全部入榜，**断言仍成立**；
  但文案读起来像「上限仍是 5」，与 `radarTopStoryLimit = 8`（`board_radar_repository.go:174`）不符，
  将来真出上限回归时这条文案会误导排查方向。
- **影响**：低——不影响正确性，但削弱回归可读性。
- **处置建议**：文案改为「造 5 条候选 → 5 条全部入榜（上限 8，未触顶）」。

## 三、R 编号清单与等级分布

| 编号 | 等级 | 状态 | 一句话 | 位置 |
| --- | --- | --- | --- | --- |
| R-1 | Medium | 已修复 | API 契约文档残留已删 UI 的三档空态死契约 | `docs/reference/api/daily-reports.md:348` |
| R-2 | Medium | **已修复** | 后端 `warming_count` 缺「当日有证据」下界（审查期间 14:34:37 已修） | `backend-go/internal/topicgraph/repository/board_radar_repository.go:622` |
| R-3 | Medium | 已修复 | 取数窗口内异常被临时撤销 → 收起再展开抖动；手动态不粘滞 | `front/app/features/tags/components/console/BoardRadarStatusBlock.vue:207-222` |
| R-4 | Low | 已修复 | 折叠摘要不暴露失败任务条数 | `BoardRadarStatusBlock.vue:225-234,364-366` |
| R-5 | Low | 已修复 | design D1 / test-cases 边界值未回写第三种格子口径 | `design.md:9-10`、`test-cases.md:125` |
| R-6 | Low | 已修复 | spec/flow 写「队列 n」，实现是「队列 40+3」 | `specs/radar-pipeline-status/spec.md:8`、`docs/reference/flow/semantic-board.md:26` |
| R-7 | Low | 已修复 | 徽标退化为零区分度；残热旧线无标记 | `BoardRadarReadingRail.vue:77-83`、`ui-verification/report.md:45,97-99` |
| R-8 | Low | 已修复 | A-8 只有反向证据；A-5 选择器空转 | `ui-verification/report.md:51,61-63` |
| R-9 | Low | 已修复 | 后端测试断言文案残留旧上限语义 | `backend-go/internal/topicgraph/repository/board_radar_stories_test.go:629` |

等级分布：**High 0 / Medium 3（其中 1 条已在审查期间修掉）/ Low 6**。无阻断归档的 High 项。

**处置状态（2026-11-09 修复轮）**：9 条全部处置完毕——R-1 / R-3～R-9 = **已修复**，R-2 = 审查期间已修复。逐条修法见各条目下的「**处置**」行。

> **并发写入说明（留痕）**：本修复轮期间工作树有**并发写者**（同 change 的其它会话）同时落地部分条目——R-1（`docs/reference/api/daily-reports.md`）、R-5（`design.md` D1 / `test-cases.md`）、R-6（spec delta / `semantic-board.md`）、R-7 文档部分（D3 已知退化）、R-8（`ui-verification/report.md` A-5/A-8）、R-9（后端断言文案）由并发写者先落地；本轮在其结果之上做了**合并与再校准**：R-3 状态机按本轮新增用例重校（真实异常变迁时手动意图作废）、R-4 失败条数改分段写法（两条队列不相加）、R-7 残热旧线标记实现落地（并发写者只记了「设计接受」）、R-1 的徽标条目措辞随 R-7 同步。

## 四、核实记录（只读命令）

```
$ openspec validate matrix-clue-fallback-and-rail-merge --strict
Change 'matrix-clue-fallback-and-rail-merge' is valid        (rc=0)

$ openspec validate --all --strict | grep matrix-clue
✓ change/matrix-clue-fallback-and-rail-merge
（树内另有 52 项失败，均为兄弟 change/主 spec 的既有问题，与本 change 无关）

$ git status --porcelain
（工作树含大量兄弟 change 的脏文件；本 change 的实现落点经逐文件读代码 + mtime 比对确认归属：
 front/app/features/tags/components/console/BoardRadar{Story,ReadingRail,StatusBlock,View}.vue
 front/app/api/normalizers/{boardRadar,laneDynamicsWindow}.ts
 front/app/features/tags/components/console/{laneConsoleView,observationLanes}.ts
 front/app/features/tags/composables/useBoardRadarOverview.ts
 backend-go/internal/topicgraph/repository/board_radar_{repository,stories_test}.go
 docs/reference/api/daily-reports.md、docs/reference/flow/semantic-board.md）

$ 关键事实比对（只读 grep/read）
- 主 spec workshop-radar 的 Requirement「今日该读与升温提示」= 主 spec:44-170，delta 整条 MODIFIED，
  Scenario 数 18 → 19（delta 为超集，无 Scenario 丢失）→ 归档同步不残留旧三态契约
- 后端 RadarStoryFeed 已无 Warming/NewStories/NewCount/NewTotalCount 字段与产出路径；
  radarTopStoryLimit = 8（board_radar_repository.go:174）；日报「今日重点」走 report_story_highlights.go 独立上限，不共用
- 前端 normalizeRadarStoryFeed 不读旧四字段；BoardRadarRawStoryFeed 仅保留可选类型声明（boardRadar.ts:112-115）
- dayCellMetrics.displayCount（laneDynamicsWindow.ts:205-210）被 matrixCellView（observationLanes.ts:214-231）
  与 buildPoolSeries/poolDayTotal（laneConsoleView.ts:163-184）共用 → 矩阵格与折线同源 ✓
- 徽标判据：BoardRadarReadingRail.vue:77-83 与 API 文档:303、spec delta:47 三处同口径 ✓
- 红线 30 三条提示：BoardRadarReadingRail.vue:100-110，同 class（rd-rail-note）、同一视觉层级 ✓
```

未执行（按纪律）：重建、全量测试、生产库写入。仅运行 `openspec validate`（只读校验）与 `review-trace.sh`（只读对账）。

## 五、最该先修的一条

**先修 R-1**（`docs/reference/api/daily-reports.md:348`）。

理由：本 change 的立项动机之一就是「删掉三块独立卡片与剩余计数摘要、不留死契约」，而它在**客户端契约文档**里只清了一半 ——
字段表清了，散文契约还在，而且写的是「SHALL 渲染『暂未触发升温条件。』与新起档三态文案」。
这有三重代价：(a) 归档后 spec 主文件与 API 文档对同一件事给出相反指令，后续实现者按 API 文档做会把被删除的 UI 做回来；
(b) 它是**唯一一条会让「死契约」在归档后仍然活着**的问题（其余 R-4/R-5/R-6/R-9 都是措辞/格式级漂移），
与本次改动的主题直接冲突；(c) 修复成本最低 —— 改一段散文，无代码、无测试、无数据风险。
R-3 排第二（可见 UI 抖动，且与 spec「异常消除后 SHALL 回到折叠摘要」的语义确有出入），其余为 Low。
