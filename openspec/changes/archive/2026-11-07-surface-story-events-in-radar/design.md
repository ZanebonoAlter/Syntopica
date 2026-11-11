# Design — 雷达右栏改事件线

## 背景与约束（现状事实，只列解释方案所需的）

**现状（只读核查，行号为实现时快照）**

| 事实 | 位置 |
|---|---|
| 「今日该读」逐条渲染 `row.reportTitle`（日报标题）+ 日期 + 文章数 + 事件数，点击跳该版块日报页 | `front/app/features/tags/components/console/BoardRadarReadingRail.vue` |
| 「今日该读」派生 = 有 `reportTitle` 的版块按 `attention` 降序取前 5 | `front/app/features/tags/composables/useBoardRadarOverview.ts` |
| 「今天可能升温」派生 = 版块级：`todayHeat / (last3Heat/3) ≥ 2` 且 `todayStories ≥ 3`，倍率挂在版块名上 | `front/app/features/tags/composables/useBoardRadarOverview.ts` |
| 热度本来就是 **story 级**：`boardHeatEvidence.compute(at)` 返回 `map[storyID]float64`；`ComputeBoardStoryHeatByDay` 只是把它**求和成版块合计** | `backend-go/internal/topicgraph/repository/story_heat.go` |
| 雷达行按版块**并发**构建，整体 5s 预算；超时行降级为 `data_delayed=true` + 零值 | `backend-go/internal/topicgraph/repository/board_radar_repository.go:GetBoardRadarOverview`、`backend-go/internal/topicgraph/handler/board_radar_handler.go:getBoardRadarOverview` |
| 每版块的 14 天轴锚点 = 该版块自己的最新日报日（`buildRadarRow` 的 `anchor` → `radarAxis`）；overview 顶层 `date` = 各版块最新日报日的最大值（`finalizeRadar`） | 同上 |
| 「今日重点」的候选排序已固定为 `heat desc → participant_count desc → story_id asc` | `backend-go/internal/topicgraph/repository/report_story_highlights.go:rankReportStoryCandidates` |
| 刊头「今日重点」点击定位到正文 thread 是**组件内状态**（`locateRequest` + `locateNonce`），**无 URL 参数** | `front/app/features/tags/components/DailyReportReader.vue:handleLocateHighlight` |
| 事件身份未回填的判定 = **板级**三态：雷达侧自算（该版块 14 天窗口内 thread 带不带 `story_id`，**不依赖话题锚定状态**）；lane-dynamics 的 `stories_backfilled` 样本只覆盖 active ∪ watch-linked 话题锚定的 section，只能管版块行的「事件数上界」标注 | `backend-go/internal/topicgraph/repository/board_radar_stories.go:radarBoardStoryIdentityState`（版块行上界口径仍在 `lane_snapshot_repository.go:loadLaneStoryAggregates`） |

**约束（红线，不可放宽）**

- 红线 28：矩阵/演进的量纲必须是事件线；`section` 数、桶数、`thread` 条数 MUST NOT 参与任何热度或排序计算。
- 红线 29：行序 = `lane_heat` 降序 → 全窗事件线数降序 → 泳道 id 升序；关注度 = Σ `lane_heat`；热度只从 `story_signals` 派生（`heat-v1-48h-hl24h`）。其中「今天可能升温」条款本 change 由**版块级改写为事件线级**（见 D6、D11）。
- 红线 30：事件身份未回填（板级判定 = 未回填）时 MUST NOT 按 0/空态冒充无事件，MUST 标注「事件身份未回填」；「无可回填对象」的版块 MUST NOT 计入未回填数。
- 术语契约（`docs/reference/flow/daily-report.md` §事件线归属与跨期去重）：事件 = thread（某天一次发生）；事件线 = `board_stories`（跨天同一事件）；话题 = 泳道；分区（section）只是展示容器，身份/计数/热度都不得引用它。

## 目标与非目标

**Goals**

- G1 右栏两块内容以**事件线**为条目单位；「今日该读」跨版块全局 Top N，每条带版块名。
- G2 升温判定落在事件线自身（自己的当日热度 vs 自己的近 3 日基线），无基线的新事件线单列「今日新起」且不显示倍率。
- G3 「今日」口径唯一且可解释：**每版块用自己最新日报日**（与泳道轴解耦），条目显式带该日期；跨版块排序用同一排序键，但**不同瞬时的热度不假装完全可比**（滞后版块少一天累积的已知偏差写进文档，不做人为补偿）。
- G4 未回填 / 超时 / 落后源三类降级都**诚实标注**，不冒充「今天没有事件」、不冒充完整排序。
- G5 事件线可点击直达日报正文对应 thread，且找不到目标时不跳假位置。

**Non-Goals**

- 不新建事件线详情页、不新建路由、不改事件线身份/归属/去重算法、不改热度公式与窗口、不改 `lane_heat`/关注度/行序口径、不改日报生成时序。
- 不做「事件线时间轴」「事件线详情」等后续路线（见姊妹 change `surface-story-heat-in-report-top` design §9 的阶段 2/3/4）。
- 不回填历史事件身份（`cmd/backfill-stories` 仍是用户手动动作）。

## Decisions

### D1. 跨版块归并落在后端 overview 端点的顶层 `stories` 段，前端不合并

- **结论**：`GET /api/semantic-boards/overview` 的响应新增顶层 `stories` 段（锚点日、热度口径版本、今日该看 Top N、今日升温清单、今日新起清单、未回填版块标识、降级标记）。跨版块归并、排序、截断全部在后端 `GetBoardRadarOverview` 内完成；前端只按段渲染，不做二次合并或重排。**不新增第二个端点**，事件线清单随同一次响应返回。
- **落点**：`backend-go/internal/topicgraph/repository/board_radar_repository.go:GetBoardRadarOverview`
- **取舍**：备选「每版块行内带 `stories` 数组、前端合并」——被否：排序键与破题是统计口径，`workshop-radar` 端点契约明令不得在前端复制统计公式，且前端合并无法统一处理未回填板与超时降级。备选「新增独立端点 `/semantic-boards/stories`」——被否：雷达首屏是单次请求契约（Scenario「一次请求渲染雷达」），多一个请求就多一个失败面与一轮 5s 预算。

### D2. 「今日」= 各版块自己的锚点日（该版块最新日报日），条目显式带日期；全局 R 只用于同刻测量与「滞后」提示

- **结论**：事件线的当日热度与近 3 日基线以**该版块自己的锚点日 A_b** 的 CST 日终 23:59:59 为锚点计算——A_b = **该版块自己的最新日报日**（`row.ReportDate`），**与泳道轴解耦**：泳道轴末日（`row.AnchorDate` = `dynamicsAnchor(dynamics)`，只覆盖 active ∪ watch-linked 话题锚定的 section）是**版块行既有字段，语义不变**，只管版块行的 14 天轴；最新一期日报没锚定到活跃话题的版块，其轴末日可能为空或更早，但事件线仍以自己的最新日报日测量并出条目（否则该版块整板缺席）。**每条目显式带该版块的锚点日**，滞后版块显示自己的日期而不是冒充 R 日。顶层 `date`（= `finalizeRadar` 的各板最大值 R）保持不变，只用于页头与「有 N 个版块数据滞后」的提示。
- **落点**：`backend-go/internal/topicgraph/repository/board_radar_repository.go:buildRadarRow`（`row.ReportDate` → 事件线测量时刻；`anchor` → `row.AnchorDate` = 版块行泳道轴末日，语义不变）、`radarBoardStoryPoints`（A_b → `radarAnchorInstant(row.ReportDate)`）、`finalizeRadar`（全局 `date`）
- **取舍**：备选「沿用泳道轴末日 `dynamicsAnchor(dynamics)` 当测量时刻」——被否（**真库证据推翻**：轴末日只覆盖锚定到活跃话题的 section；候选话题锚定的版块轴末日为空 → 该版块事件线整板缺席，而它的日报与事件线都真实存在）。备选「全局锚点日 R 统一测量」——被否（**真库证据推翻**：13 个活跃版块中 11 个在 2026-11-06、2 个在 2026-11-05，只差 1 天；若统一按 R=11-06 测量，这 2 个版块当日无证据 → 事件线清单里**整板缺席**，而现「今日该读」是**会显示**它们的（带 11-05 日期）。换过去等于白丢两个版块的全部事件，收益只是跨板热度「同刻可比」，代价是可见内容倒退）。备选「用自然今天」——被否：日报夜间生成、自然今天常无数据，事件线清单会恒为空；红线 30 的诚实降级要求「今天」有确定含义。
- **已知偏差（必须写进文档，不假装完全可比）**：滞后版块的条目在其**自身日期**日终测量，48h 窗内少累积约一天 → 其热度上限天然略低于同日的领先版块。这是**系统性偏向新鲜**的已知偏差，幅度 ≈1 天累积（滞后 1 天时约 0.5 个数量级内的递减），**不通过人为补偿修正**（补偿会引入伪精度）。若某版块滞后 ≥3 天，视为数据延迟 → 不纳入全局 Top N 排序，并在响应 `degraded_board_ids` 标注。


### D3. per-story 明细走「新增函数 + 复用包内私有热度算法」，不改既有热度函数的返回值语义

- **结论**：新增 `backend-go/internal/topicgraph/repository/board_radar_stories.go`，暴露 `RadarBoardStoryPoints(boardID uint, anchor time.Time) ([]RadarStoryPoint, error)`。其内部**只调用既有 `loadBoardHeatEvidence` + `boardHeatEvidence.compute(at)`**：对 `E0=A_b 日终`、`E1=A_b-1 日终`、`E2=A_b-2 日终`、`E3=A_b-3 日终` 各调一次 `compute`，即得四条事件线级日热度；基线 = `mean(heat(E1), heat(E2), heat(E3))`（**不含 E0 本身**）。`ComputeBoardStoryHeatByDay` 的签名与返回值语义**完全不变**（仍求和成版块合计），既有调用方与测试零改动。证据窗口 = `[E0-5d, E0]`（`heat(E3)` 的 48h 窗下界是 `E3-48h = E0-5d`），比既有 14 天轴那次加载更窄。
- **落点**：`backend-go/internal/topicgraph/repository/story_heat.go:compute`
- **取舍**：备选「把 `ComputeBoardStoryHeatByDay` 的返回值改成 per-story map」——被否：该函数被雷达行与既有测试按版块合计消费，改返回值会把「按板求和」这一步推给每个调用方，正是红线 29 要防的口径复制。备选「写一套雷达专用热度公式」——被否：复制指数衰减/窗口/参与者去重即复制热度口径，违反红线 28/29 的唯一口径纪律。

### D4. 标题与来源数取自同一份证据流，不额外开查询

- **结论**：`storyHeatIdentity` 增一个 `Title string` 字段，`loadBoardHeatEvidence` 的 `board_stories` 查询附带 `title`（附加字段，不改热度计算、不改归并解析）；事件线标题取**存活身份**的 `title`（经 `evidence.survivors` 一跳解析，与 `highlightStorySurvivors` 同纪律：坏链/环保守丢弃；**多跳链只剔除链上前驱，不得连坐最终存活者**——存活者本身是干净身份，丢了就是凭空消失一条事件线）。来源数 = R 日终 48h 窗内该事件线去重 `participant_key` 数（与 `rankReportStoryCandidates` 同法：每个参与者只计窗口内最后一次 `observed_at`）。
- **落点**：`backend-go/internal/topicgraph/repository/story_heat.go:loadBoardHeatEvidence`
- **取舍**：无备选：标题与来源数都已在这条证据流内，另开查询会挤占 5s 预算；另起标题查询还会与归并解析产生两套身份口径。

### D5. 排序键统一为「原始热度降序 → 来源数降序 → 存活事件线 id 升序」

- **结论**：三个清单（今日该看 / 今日升温 / 今日新起）共用同一排序键，跨版块混排时也用它破同分。`story_id` 是 `board_stories` 的全局主键，跨版块可比。排序前**不四舍五入**（与 `rankReportStoryCandidates` 一致），展示时才保留一位小数。
- **落点**：`backend-go/internal/topicgraph/repository/report_story_highlights.go:rankReportStoryCandidates`
- **取舍**：无备选：红线 29 要求热度口径唯一，同一条事件线在日报「今日重点」与雷达清单里若排序不同，读者会得到互相矛盾的名次。

### D6. 升温判定落在事件线自身；无近 3 日基线者单列「今日新起」且不显示倍率

- **结论**：
  - 升温档 = 该事件线 `heat(R) ≥ 2 × baseline`，`baseline = mean(heat(R-1), heat(R-2), heat(R-3))`，**且**该事件线在 R 当日（CST 00:00–23:59）至少有一条 `story_signals` 证据（「今日升温」是对今天的断言，必须今天有报道）。倍率 = `heat(R) / baseline`，保留一位小数。
  - 今日新起档 = `baseline == 0`（近 3 日无任何热度证据）且 `heat(R) > 0` 且 R 当日有证据；**不显示倍率**，不显示 0.0×、不显示 ∞。
  - 两档各自按 D5 的排序键排序，档内不混排；「今日该看」Top N 取全部 `heat(R) > 0` 的事件线（含升温/新起条目）。
- **落点**：Scenario 「升温判定」
- **取舍**：备选「基线含 R 自身（沿用版块级 `last3_heat/3` 的窗口）」——被否：含 R 时分母恒 > 0（有当日热度必有基线），用户拍板的「今日新起」档不可达；且会把「前两天翻倍」误判为今日升温。备选「保留版块级升温清单」——被否：用户明确「今日升温这个应该指事件线」，版块级合计正是用户指出的错位来源。备选「不要求当日证据」——被否：仅靠前一日报道的衰减余温也能凑够 2 倍，那不是「今天在升温」。

### D7. 未回填版块不参与排序，且显式标注「事件身份未回填」

- **结论**：事件身份回填状态按**板级三态**判定（只看该版块自己的报告线程带不带 `story_id`，**不依赖话题是否锚定到活跃话题**）：① **已回填**（`backfilled`）→ 照常出条目；② **未回填**（`unbackfilled`：有事件候选线程却都不带 `story_id`）→ **不产生任何条目**，也不进入「今天没有事件」的表述，响应 `stories` 段携带 `unbackfilled_board_ids` / `unbackfilled_board_count`，界面在该区块上方明示「N 个版块事件身份未回填，未纳入事件线排序」；③ **无可回填对象**（`no_candidate`：板内没有任何事件候选）→ **不计入未回填数**（本来就没有事件身份可回填），也不产条目，响应另带 `no_story_board_ids` / `no_story_board_count` 作诊断。回填是用户手动动作（`cmd/backfill-stories`，幂等可重跑），本 change 不自动回填。
- **落点**：`backend-go/internal/topicgraph/repository/board_radar_stories.go:radarBoardStoryIdentityState`（版块行的 `stories_backfilled` 上界口径仍在 `lane_snapshot_repository.go:loadLaneStoryAggregates`）
- **取舍**：备选「按非综合稿事件数上界参与排序」——被否：上界不是事件线身份，会产出既不可点击定位、也无可验证来源数的假条目，正是红线 30 禁止的「冒充」。备选「静默不显示」——被否：读者会误以为这些版块今天没有事件。

### D8. 端点超时降级 → 事件线清单标 `partial` 并列出降级版块

- **结论**：沿用既有 5s 预算与 `data_delayed` 降级（未完成的版块行归零 + `data_delayed=true`）。这些版块的事件线**不参与**全局排序，且响应把 `stories.partial` 置 `true`、`degraded_board_ids` 列出降级版块 id；界面标注「部分版块数据延迟，本次事件线排序不完整」。**不得**把缺数据的清单呈现为完整排序。**单版块事件线计算失败**（`RadarBoardStoryPoints` / `RadarStoryReportDates` 报错）走同一诚实降级：该板不参与排序 + `partial=true` + 进 `degraded_board_ids`；但它**不是** `data_delayed`（那是「整板数据延迟」语义，不得复用——会把「计算失败」误标成超时降级）。
- **落点**：`backend-go/internal/topicgraph/repository/board_radar_repository.go:GetBoardRadarOverview`
- **取舍**：无备选：静默截断等于把不完整的排序说成完整的，与红线 30 的诚实降级同一纪律。

### D9. 点击去向 = `?story=<id>` 深链；后端先解析落点报告日期，找不到目标不跳、给提示

- **结论**：
  1. 后端为清单里的每条事件线解析**落点报告日期**：该版块在 R 之前、最近一期「含该事件线 body thread（`scope='single'`、`display_tier='body'`、状态已完成）」的日报 `period_date`。解析按存活身份一跳归一（合并前的旧 id 同样命中）。
  2. 条目携带 `report_date`；点击跳 `/tags/{boardId}/{reportDate}?story={storyId}`。**无 `report_date` 的条目不渲染为可点**（静态行 + 「该事件线暂无正文落点」提示）。
  3. 日报阅读台读 `?story=`：参数在详情就绪后按**当前正文 DOM 顺序**定位该事件线第一条 thread（展开 → 滚动 → 聚焦 → 高亮 2s），复用刊头「今日重点」定位的同一实现。**本期正文无该事件线时**：不滚动、不跳近似位置，提示「本期正文暂无对应事件」。翻页/切版块时该参数被清掉（`DailyReportDeskShell.replaceDate` / `openTodayBoard` 只带 `lane` 查询参数，天然不续传），不在别的报告重复定位；非法参数值忽略且不报错。
- **落点**：`front/app/features/tags/components/DailyReportReader.vue:handleLocateHighlight`
- **取舍**：备选「链接到版块最新日报日、不校验该线是否在正文」——被否：会把读者送到不含该事件线的日报，制造「跳到假位置」的错觉，与刚归档 change 的「本期正文暂无对应事件」降级纪律冲突。备选「沿用组件内 `locateRequest`，不给 URL 参数」——被否：跨页无法寻址，雷达 → 日报这一跳拿不到目标。

### D10. 前端：右栏两块换数据源；旧响应（无 `stories` 段）回退既有版块级列表

- **结论**：`useBoardRadarOverview` 暴露 `topStories` / `warmingStories` / `newStories` 三个派生（来自响应 `stories` 段），`BoardRadarReadingRail` 的两块改渲染这三组；报头「N 处出现升温」计数改用服务端给的 `warming_count`（全量计数，不是截断后的 5）。**兼容分支**：响应缺 `stories` 段（旧后端）时回退到既有版块级日报标题列表与版块级升温清单，不渲染成空态。
- **落点**：`front/app/features/tags/composables/useBoardRadarOverview.ts`
- **取舍**：备选「直接清掉旧版块级派生」——被否：前后端分批发布时右栏会整块空白，用户看到的是「今天没有事件」这种业务结论（旧响应 ≠ 没有事件）。备选「前端算全量升温计数」——被否：截断后的清单算不出全量计数，报头会少报。

### D11. 红线 29 的「今天可能升温」条款由版块级改写为事件线级，实现阶段同步改写 flow 文档

- **结论**：本 change 只改**呈现粒度与判定主体**，不改热度来源、不改阈值数字（≥2× 保留）、不改 `lane_heat`/关注度/行序口径。版块级「当日事件线数 ≥ 3」这一**下限**在事件线级不可原样搬用（单条事件线凑不到 3 个事件），替换为可验证的「R 当日有报道证据」下界——**收紧而非放宽**。实现阶段必须同步改写 `docs/reference/flow/semantic-board.md` 红线 29 的升温条款（及雷达右栏口径描述），否则文档与实现分叉。
- **落点**：`docs/reference/flow/semantic-board.md`
- **取舍**：无备选：红线句是 constraint-injection 的注入源，文档不改就会持续把实现拉回版块级口径。

### D12. 清单条数上限：今日该看 5、今日升温 5、今日新起 3

- **结论**：右栏宽度维持既有 `clamp(17rem, 22vw, 29rem)`，条数上限固定为 5 / 5 / 3。截断在**后端**完成（`stories` 段直接只给上限条数），前端只渲染收到的条数、不再二次截断。上限是常量，实施时可按实际观感调整（调整只改常量，不改口径）。
- **落点**：`backend-go/internal/topicgraph/repository/board_radar_repository.go:GetBoardRadarOverview`
- **取舍**：备选「不限条数、全量渲染」——被否：右栏会变成长列表，需要新增折叠/滚动容器，超出 minor 复用契约（不改既有布局与交互模式）。

## 数据形状（wire 契约）

```jsonc
GET /api/semantic-boards/overview
{
  "date": "2026-11-06",              // 既有字段，= 各版块最新日报日的最大值（页头用）
  "boards": [ /* 既有 RadarBoardRow，逐字段不变；每行自带 anchor_date = 该版块自己的锚点日 */ ],
  "stories": {                        // 新增段（缺失时前端回退旧版块级列表）
    "anchor_date": "2026-11-06",     // 全局锚点日 R（= date）；仅用于「有 N 个版块滞后」提示
    "heat_rule_version": "heat-v1-48h-hl24h",
    "top_stories":   [ /* RadarStoryItem，跨版块混排，Top 5 */ ],
    "warming":       [ /* RadarStoryItem，事件线级升温，Top 5 */ ],
    "new_stories":   [ /* RadarStoryItem，无近 3 日基线的新事件线，Top 3 */ ],
    "warming_count": 6,               // 全量升温条数（报头计数用，非截断数）
    "new_count": 2,
    "unbackfilled_board_ids": [12],   // 板级判定=未回填、未纳入排序的版块
    "unbackfilled_board_count": 1,
    "no_story_board_ids": [77],       // 板级判定=无可回填对象（不计入未回填数）
    "no_story_board_count": 1,
    "lagging_board_ids": [3122],      // 自身最新日报日 < 全局 R 的版块（数据滞后，非缺失）
    "partial": false,                 // 存在降级版块（超时或计算失败）→ 排序不完整
    "degraded_board_ids": []
  }
}

// RadarStoryItem
{
  "board_id": 1974, "board_label": "中东地缘政治与美伊关系",
  "story_id": 4211, "story_title": "美伊博弈升级",
  "participants": 4,                 // 该版块锚点日日终 48h 窗去重来源数
  "heat": 5.0,                       // 该版块锚点日日终 story_heat（原始精度，展示保留一位小数）
  "baseline_heat": 1.2,              // 该版块近 3 日（不含锚点日）日均
  "ratio": 4.2,                      // heat / baseline_heat；无基线时为 0 且不展示
  "has_baseline": true,
  "has_today_evidence": true,
  "anchor_date": "2026-11-05",       // 该版块自己的锚点日（滞后版块 ≠ 全局 R，界面必须显示）
  "report_date": "2026-11-06"        // 落点报告日期；缺省 = 无正文落点，不可点
}
```

**后端计算链（每版块，随该版块的并发 goroutine 一起跑）**

```text
GetBoardRadarOverview(ctx)
  ├─ 活跃版块列表 + 标签数（既有）
  ├─ R = MAX(period_date) over 活跃版块日报（既有：finalizeRadar 的全局锚点，仅用于页头与滞后提示）
  ├─ 每版块并发（共用 5s 预算）：
  │    buildRadarRow(boardID)                     // 既有：日报摘要、泳道、14 天轴、热度
  │    RadarBoardStoryPoints(boardID, A_b)         // 新增：per-story 明细（D3）；A_b = 该版块自己的锚点日
  │      ├─ loadBoardHeatEvidence(boardID, E0-5d, E0)
  │      ├─ evidence.compute(Ek) × 4              // E0..E3 的 per-story 热度
  │      ├─ participants（E0 48h 窗去重）/ has_today_evidence（A_b 当日 CST 日内）
  │      └─ baseline = mean(E1,E2,E3)；ratio = heat(E0)/baseline
  │    RadarStoryReportDates(boardID, A_b, 候选事件线) // 新增：落点报告日期（D9-1）
  └─ 归并（扇出完成后，D1）
       ├─ 丢弃 heat ≤ 0 与降级版块的条目
       ├─ 排序 heat desc → participants desc → story_id asc（D5）
       ├─ top_stories = 前 5；warming = ratio ≥ 2 且当日有证据 前 5；
       │  new_stories = 无基线且当日有证据 前 3（D6、D12）
       ├─ unbackfilled_*：板级判定=未回填的版块；no_story_*：无可回填对象（D7）
       └─ partial / degraded_board_ids：data_delayed 或事件线计算失败的版块（D8）
```

**预算影响**：每版块新增一次窄窗证据读取（5 天窗口，小于既有 14 天轴那次）+ 4 次 `compute`（既有是 14 次但窗口更大）+ 一次落点报告日期查询。`compute` 是对信号集的线性扫描，新增部分 ≈ 既有热度开销的四分之一量级。风险与实测要求见「风险与取舍」节。

## 降级与空态矩阵

| 情形 | 今日该看 | 今日升温 / 今日新起 | 标注 |
|---|---|---|---|
| 正常 | 跨版块 Top 5，每条带版块名/来源数/热度 | 升温带自身倍率与理由句；新起不带倍率 | 锚点日随报头展示 |
| 该版块事件身份未回填（板级判定） | 该版块无条目 | 该版块无条目 | 「N 个版块事件身份未回填，未纳入事件线排序」 |
| 该版块无可回填对象（板级判定） | 该版块无条目 | 该版块无条目 | 不计入未回填数、不作「今天没有事件」表述 |
| 端点超时（`data_delayed`） | 降级版块条目缺席 | 同左 | 「部分版块数据延迟，本次事件线排序不完整」 |
| 单版块事件线计算失败 | 该版块条目缺席 | 同左 | 「本次事件线排序不完整」（不进 `data_delayed`） |
| 滞后版块（自身最新日报日 < 全局 R） | 照常出条目，带自己的锚点日 | 同左 | 「N 个版块数据滞后（各条目按自己的锚点日计）」 |
| 落后源（`heat_incomplete`） | 条目照出 | 同左 | 既有不完整标记照旧透出 |
| 无任何可识别事件线 | 「今天没有可识别的事件线。」 | 「暂未触发升温条件。」/「今天没有新起的事件线。」 | 不得写成「今天没有事件」（未回填时是别的情况） |
| 事件线无正文落点 | 静态行，不可点 | 同左 | 「该事件线暂无正文落点」 |
| 旧后端响应（无 `stories` 段） | 回退版块级日报标题列表 | 回退版块级升温清单 | 不渲染成业务空态 |
| 首次取数失败 | 不渲染右栏内容（既有 R-17 纪律） | 同左 | 既有错误条 + 重试 |

## 风险与取舍

| 风险 | 影响 | 缓解 |
|---|---|---|
| 5s 预算被新增的 per-story 计算击穿 | 版块行降级为 `data_delayed`，事件线清单变成 `partial` | 证据窗口收窄到 5 天；per-story 计算放在既有版块 goroutine 内共享同一 deadline；实施时用隔离 PG 实测最坏版块耗时，超预算则按 D8 降级（不延长预算、不放宽降级语义） |
| 「今日」按各版块自己的锚点日 A_b 测量 | 滞后版块的热度少一天累积，跨板排序偏向新鲜 | **已知系统性偏差，写进文档不假装完全可比**；条目显式带 A_b 日期让读者自行判断；滞后 ≥3 天的版块视为数据延迟、不纳入 Top N 并标注（不做人为补偿——补偿会引入伪精度） |
| 事件线级升温比版块级敏感（基线不含 R） | 条目变多、噪声上升 | 追加「R 当日有报道证据」下界；档内按同一排序键取前 5；用户可按档位读，不做逐条判定 |
| 「今日新起」把「沉寂后重现」也算成新起 | 档位语义略宽 | 本 change 按用户拍板口径实现（无近 3 日基线即入档）；`board_stories.first_seen_date` 可用于更窄定义，属后续增量，不在本 change 自行改口径 |
| `?story=` 深链在正文已被重建/覆盖时失效 | 读者点了却定位不到 | 后端先解析落点报告日期（降低失效概率）；失效时提示「本期正文暂无对应事件」且不滚动，不跳假位置 |
| 前端回退分支与新分支并存 | 两套渲染路径 | 回退只在响应缺 `stories` 段时启用；实施后确认新后端始终带该段，再评估是否移除回退 |

## 迁移与部署

- 无 DDL、无数据迁移、无历史回填；响应 JSON 扩展，旧客户端忽略新字段即可。
- 部署顺序：后端先带 `stories` 段上线，前端再切换渲染；前端保留旧响应回退分支，任意顺序均不出空白右栏。
- 回滚：回滚前端即恢复版块级列表；回滚后端则前端走回退分支（需后端响应缺 `stories` 段时前端确实回退——实施时须覆盖该用例）。
- 旧数据降级：历史日报、旧事件线、旧版块行不变；未回填版块按 D7 标注；`heat_incomplete` 照旧。

## 待确认问题（Open Questions，不阻塞实现，但需用户/主线程确认）

1. **条数上限 5 / 5 / 3 是否合意**（D12）：右栏宽度固定，条数只影响首屏密度；实施时可按实际观感调整常量。
2. **块标题文案**：用户口述为「今日该看」「今日升温」，实现里是「今日该读」「今天可能升温」。本 change 按「不改标题文案」实现，若用户希望统一口径需另行拍板。
3. **红线 29 文案改写**（D11）需在实现阶段同步落 `docs/reference/flow/semantic-board.md`；本 change 制品阶段不动 reference 文档。
