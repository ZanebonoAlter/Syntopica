# Review — deconflict-radar-warming-and-new-stories

- 日期：2026-11-07 ｜ 方式：独立只读审查（制品 7 份逐行 + 实现代码逐行 + 真接口证据复核）
- 归因说明：工作树含 162 个兄弟 change 的脏文件且全部未提交，`git diff` 无法按 change 切分。本审查按「本 change 特有标记」（`topIDs` 互斥、`CrossDayStoryCount`、`warmingEmptyKind`、`newStoryTotal`、三处文案）在共享文件内定位归属，逐标记核对其边界，未把兄弟 change 的改动计入。
- 结论速览：**5 条发现（High 1 / Medium 2 / Low 2）**。核心问题是 R-1：spec「新起全被该读覆盖时给摘要」在可达状态下无法满足——摘要分支是死代码，组件测试用后端不可能产生的载荷伪造了覆盖。其余三项用户拍板（诚实空态、该读不硬卡来源数、互斥本体）实现到位。

## 一、八项重点审查逐条结论

### 1. `cross_day_story_count` 判据 — 有问题（见 R-3）

判据本体 `HasBaseline && HasTodayEvidence`（`board_radar_repository.go:579`）对 brief 列出的三个边界：

- 昨日有证据 + 今日有 → 计入 ✓（`TestRadarStoryFeedCrossDayStoryCount` 第一子测：seed 昨日 + 今日信号，断言 == 2，实跑绿）。
- 昨日有证据 + 今日没有 → **不**计入 ✓（同测试的 stale 线种子只喂昨日信号，断言被排除；这正是 388 条残热旧线的防错报口径）。
- 5 天前有证据 + 今日有（断档超出 3 日基线窗）→ `HasBaseline=false` → 判为「今日新起」、**不计入** `cross_day_story_count`。**与 spec 措辞冲突**：spec.md:13/71 写的是「既在**锚点日之前**有热度证据、又在锚点日当天有证据」——断档线客观上满足这句措辞，但实现只在近 3 日基线窗内认「锚点日之前的证据」。若当日仅有这类线，空态会显示「当前没有跨天延续的事件线」，而 spec 的 WHEN（存在同时具备两侧证据的线）为真。test-cases.md:46 与 design.md:68 还把 spec 引作「有近 3 日基线热度的事件线数」——**该句在 spec 中不存在**，且恰好是被真库证伪的「只看基线」口径。详 R-3。

### 2. `new_count` 口径 — 通过（但摘要行永不出现，归 R-1）

- 赋值时机：`feed.NewCount = len(newStories)` 在 `board_radar_repository.go:645`，截断在 :649-650——**确在截断前**，`TestRadarStoryFeedNewCountNotAffectedByTrim`（new_count==3 而 len==3 上限）钉住。
- 四处口径：实现（去 Top 后完整数）＝ design D2（「当日有证据 ∧ 无基线 ∧ 不在 Top N 的完整计数（截断前）」）＝ spec.md:11（「以一行摘要给出完整计数」）＝ API 文档 daily-reports.md:315（「**未进 `top_stories`** 的新起事件线全量计数（非截断数；与 `new_stories` 同口径）」）＝ 前端摘要行（`newStoryTotal` = `newCount`，BoardRadarReadingRail.vue:231）。**逐字一致** ✓。
- 但同一后端函数保证 `len(new_stories)==0 ⟺ new_count==0`（截断上限 3≥1），前端摘要条件 `!newStories.length && newStoryTotal > 0`（rail:227）**永假**——见 R-1。

### 3. 互斥边界 — 通过

- `topIDs` 由截断后的 `feed.TopStories` 生成（:620-626），上限 `radarTopStoryLimit=5` 调大调小时 `topIDs` 始终等于「界面实际展示集」，互斥对齐成立——这正是 design D1 论证的去重必须对齐展示集，不随上限漂移。
- 升温不参与互斥：实现上天然不相交（升温要求 `HasBaseline`、新起要求 `!HasBaseline`，:633-638 的 if/continue 结构保证）；升温 ∩ 该读允许重叠已**写进 spec 正文**（spec.md:5 括号豁免句）与 Requirement 首段，「同一事件线两种视角」的论证成立且成为契约，不是只停留在 design。✓

### 4. 空态三态 — 通过（附 Low R-4）

- 归一：`cross_day_story_count` 缺失/非有限数 → `null`，`0` → `0`（boardRadar.ts:288-291，`Number.isFinite` 三元，非 `||`）✓。
- 派生：`crossDay === null || undefined → 'unknown'`；`> 0 → 'not-triggered'`；`0 → 'no-cross-day'`（useBoardRadarOverview.ts:108-113）——全前端**唯一**消费点（grep 全量确认），无 `??`/`||` 误用，`0` 不会被吞。✓
- `unknown` 回退「暂未触发升温条件。」：这是 spec.md:75 Scenario 明文要求（「SHALL 回退为『暂未触发升温条件』」），且该句在 unknown 态是可观察事实（升温清单确实为空）而非成因断言——比「没有跨天延续」保守，比空白诚实，可接受。若追求更中性可用「今日暂无升温条目。」，非必需。
- 缺陷：composable 注释把 `no-cross-day` 的判据写成「有基线的事件线数为 0」（useBoardRadarOverview.ts:103）——复述了本 change 自己证伪的只看基线口径，见 R-4。

### 5. 任务与实现的一致 — 有问题（见 R-5）

逐字比对 tasks 4.1-4.5、test-cases 全部文案与实现/测试：

| 出处 | 写法 | 实现 |
| --- | --- | --- |
| tasks.md:31（4.3） | 「**今日**没有新起的事件线。」 | 「**今天**没有新起的事件线。」（rail:238，组件测试 :393 同） |
| test-cases.md 4.2 空集行、6.1 B11 | 「**今日**没有新起的事件线。」 | 同上 |

其余升温文案（「当前没有跨天延续的事件线。」「跨天延续能力尚在建设中。」「暂未触发升温条件。」）与摘要行「另有 N 条今日新起事件线（未在今日该读中列出）。」四处逐字一致。R-5 记录用词不一致 + 一处失真的 spec 引用。

### 6. 验收证据强度 — 通过

- A-8 诚实 ✓：明确标注「未覆盖／未取得实测样本」，列出单测清单与命令，未把单测写成实测。
- A-7 真有 390×844 数据 ✓：报告给出三档视口各自的 `{diff, noOverflow, clippedCount}` JSON，非空泛声明。
- A-1～A-6 均为真 DOM 机械断言（testid 存在性、innerText 文案、DOM 提取 story_id 集合），与真接口 curl 数据互相印证，无「单测冒充实测」条目。
- `not-triggered`/`unknown` 两个空态分支真库造不出、仅单测覆盖——报告**未声称**对它们做过实测（A-2 只断言 no-cross-day 态），无虚报；但报告也未像 A-8 那样显式声明这两个分支未实测（属可补的完备性，不构成不诚实）。
- 先红后绿证据（/tmp 外科回退、四后端测试红→绿 + 前端 6 红→绿）方法可信、输出在案。✓

### 7. scope creep — 通过

按标记归因核查本 change 触点：后端仅 `CrossDayStoryCount` 字段+计数（:141-150/:577-580）、`topIDs` 互斥（:620-639）、`NewCount` 注释改写（:132-140）——热度公式与窗口（`heat-v1-48h-hl24h`）、排序键（heat desc → participants desc → story_id asc，:608-617）、`top_stories` 上限 5 / 升温上限 5 / 新起上限 3、`radarWarmingRatio=2`、事件线归属/去重算法均未触碰（proposal「不受影响」清单与代码现状一致）。前端仅归一字段、composable 两派生、rail 结构与文案、view 传参。`docs/reference/api/daily-reports.md` 的大 diff 中本 change 仅 :301-303/:311-316/:389-390 段，其余属兄弟 change。未发现越界改动。✓

### 8. 与 `enable-story-cross-day-continuation` 的回归风险 — 通过

- 文件不重叠 ✓：该 change 的 Impact/tasks 落在 `story_assign.go`/`story_fingerprint.go`/`lane_snapshot_repository.go`/`cmd/backfill-*`，grep 其 design/tasks 无 `board_radar_repository.go`、无本 change 前端文件（其 design.md:17/96 显式说雷达侧来源数「不需要改，也不该动」）。
- 衔接成立 ✓：该 change 的层二延续判据日期窗口 ≤1 天（以线最近证据日为基准），接上的跨天线在锚点日前 1 日必有证据 → 落在 3 日基线窗内 → `HasBaseline=true` 且当日有挂载证据 → `cross_day_story_count>0` → 本 change 空态自动切到 `not-triggered` 分支，与 design 风险表预判一致。断档 >3 天的情形在该 change 下会被判成**新线**（不接续），「客观跨天线被漏计」在身份层已被消除——R-3 的残余风险因此收窄为措辞层。

## 二、问题清单

### R-1 2026-11-07【High】spec「新起全被该读覆盖时给摘要」在可达状态下无法满足：摘要分支是死代码，组件测试用不可能载荷伪造覆盖

- **状态**：已修复（后端新增 new_total_count、前端三态渲染、伪造载荷用例删除，2026-11-07）
- **位置**：backend-go/internal/topicgraph/repository/board_radar_repository.go:627-650；front/app/features/tags/components/console/BoardRadarReadingRail.vue:227-238；openspec/changes/deconflict-radar-warming-and-new-stories/specs/workshop-radar/spec.md:11,93；front/app/features/tags/components/console/BoardRadarView.test.ts:381-389
- **事实**：后端同一函数内 `feed.NewCount = len(newStories)`（:645，截断前）与 `newStories = newStories[:radarNewStoryLimit]`（:649-650，上限 3≥1）保证 **`len(new_stories)==0 ⟺ new_count==0`**（normalizer 对 new_stories 逐项 map 无过滤，boardRadar.ts:284；handler 无再加工，grep 全仓 NewCount 仅 :645 一处赋值）。因此前端摘要条件 `v-if="!newStories.length && newStoryTotal > 0"`（rail:227）**对任何真实载荷永假**。「全被该读覆盖」的真实状态（后端测试 `TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered` 亲测：`assert.Zero(t, feed.NewCount)`）下界面渲染的是「今天没有新起的事件线。」（rail:238）——既无计数也无「未在今日该读中列出」说明，且该句在此状态下是**假话**（当日明明有新起线，只是全进了该读）。而 spec.md:11 要求「SHALL 以一行摘要给出完整计数与说明（『另有 N 条今日新起事件线（未在今日该读中列出）』）」、Scenario:93 THEN 要求「SHALL 显示一行摘要，计数为去 Top 后的完整条数」。组件测试 :383 用 `storyFeed({ newStories: [], newCount: 133 })` —— 一个后端不可能产生的组合 —— 让断言绿，tasks.md 场景映射表把该 Scenario 指到这条测试，scenario-trace 门禁因此假绿。
- **影响**：用户拍板项 2「全被覆盖时降为一行摘要」在任何可达状态下都未交付；归档后主 spec 将携带一条实现 provably 无法满足的 Scenario；测试覆盖是伪造的（钉住的是不可达输入上的行为）。
- **处置建议**：三选一并同步 spec/tasks/测试：① 全被覆盖态改为诚实文案（如「今日新起的事件线均已列入『今日该读』。」），spec Scenario 同步改写，组件测试改用真实可达载荷（全被覆盖 ⇒ newCount==0）；② 让摘要行服务于截断态（`newCount > newStories.length` 时附「另有 N−3 条」——当前真库 133 条只列 3 条、其余 130 条完全不可见，顺带修复）；③ 撤掉死代码分支与对应假测试，spec 删去摘要义务。当前实现三者都不是，属于「spec 说了、代码写了、但两者永远接不上」。

### R-2 2026-11-07【Medium】proposal「部署后影响」与实测行为矛盾且未更正；design/API 文档同源失真

- **状态**：已修复（proposal/design/api 文档按实测与实现更正：3 条目 + 「另有 130 条」摘要，2026-11-07）
- **位置**：openspec/changes/deconflict-radar-warming-and-new-stories/proposal.md:38；design.md:22；docs/reference/api/daily-reports.md:302；ui-verification/report.md（A-3/A-4 实测记录）
- **事实**：proposal「部署后影响」第 1 条预测：「当前真库数据下它会变成一行摘要：**『另有 133 条今日新起事件线（未在今日该读中列出）』**……右栏实际展示的条目数是 5 条（该读）+ **0 条（新起条目）** + 0 条（升温）」。而 ui-verification 实测：`new_stories = 5052/5110/5074` **三条条目在列**（A-3 逐条断言存在、A-4 集合互斥），无摘要行——预测的「一行摘要 + 0 条目」与实测「3 条目 + 无摘要」直接矛盾，报告未标注。design.md:22「摘要显示 133，与 new_stories 列表（同为去 Top 后取前 3）口径一致」描述的展示态（摘要行出现）同样不会发生。docs/reference/api/daily-reports.md:302「全部新起线都被 Top 覆盖时 `new_stories` 为空而 `new_count` 为 0，**界面据此给一行摘要**」——`new_count==0` 时界面实际渲染「今天没有新起的事件线。」（rail:238 的 `v-else-if="!newStories.length"`），不渲染摘要。
- **影响**：「部署后影响」是用户知情与验收的依据，与实际不符会误导本次验收结论与后续 change（尤其 enable-story-cross-day-continuation）对现状的假设；API 文档把 UI 行为写错。
- **处置建议**：三处文字改为实际行为：当前数据下列 3 条非 Top 新起、其余 130 条无计数入口；全被覆盖时显示「今天没有新起的事件线。」（若按 R-1 修实现则同步改回）。

### R-3 2026-11-07【Medium】cross_day_story_count 判据与 spec 措辞存在基线窗口差；两份制品引用了 spec 中不存在的句子且恰是已证伪口径

- **状态**：已修复（引用逐处对齐 spec 实文；spec 措辞收敛为判据本身 + design D3 记录已知口径边界，2026-11-07）
- **位置**：backend-go/internal/topicgraph/repository/board_radar_repository.go:141-150,577-580；backend-go/internal/topicgraph/repository/board_radar_stories.go:196-197；specs/workshop-radar/spec.md:13,71；test-cases.md:46；design.md:68；board_radar_stories_test.go:653-655
- **事实**：实现判据 `point.HasBaseline && point.HasTodayEvidence`（:579），其中 `HasBaseline = baseline > 0`、baseline = 锚点日**之前 3 天**均值（board_radar_stories.go:196-197）。spec.md:13/71 的措辞是「既在**锚点日之前**有热度证据、又在锚点日当天有证据」——锚点日前 >3 日断档后今天复现的线满足 spec 措辞、却被实现判为「今日新起」且不计入计数；当日仅有这类线时空态会说「当前没有跨天延续的事件线」，与该 Scenario 的 WHEN（存在同时具备两侧证据的线）矛盾。后端字段注释（:141-143「既在锚点日之前有热度证据、又在锚点日当天有证据」）同样超出实现实际检查的窗口。同时 test-cases.md:46 把 spec 锚引作「跨天延续事件线是否存在 SHALL 由服务端以『**有近 3 日基线热度的事件线数**』判定」、design.md:68 引作「D3 在 spec 里把判定输入写死为『有近 3 日基线热度的事件线数』」——**spec 中不存在该句**（spec 实文是「既有锚点日之前的证据、又有锚点日当天证据的事件线数」），且被引的这句恰好是 design D3 自己用真库证伪的「只看基线」口径（388 条错报）。board_radar_stories_test.go:653-655 测试头注释同样写「= 有近 3 日基线的事件线条数」而其断言验的是加当日证据的口径。
- **影响**：断档复活场景（>3 日）下空态措辞与 spec WHEN 冲突（低频但属契约缝隙；enable-story-cross-day-continuation 落地后该场景在身份层基本消失，风险收窄为措辞层）；引用失真更实际的风险是诱导后来者把后端「修」回只数基线的错口径——那是本 change 刚修掉的 388 条错报。
- **处置建议**：① spec.md:13/71 措辞收紧为「既在锚点日之前 3 日基线窗口内有热度证据、又在锚点日当天有证据」，并加一句「断档超出基线窗的线按『今日新起』处理」；② test-cases.md:46、design.md:68 的引用改为 spec 实文；③ 后端字段注释（:141）与测试头注释（:653）补上「近 3 日窗口内」限定。

### R-4 2026-11-07【Low】composable 注释复述被证伪的「有基线」口径

- **状态**：已修复（composable 注释改为真实判据，2026-11-07）
- **位置**：front/app/features/tags/composables/useBoardRadarOverview.ts:103
- **事实**：`warmingEmptyKind` 的 JSDoc 写「`no-cross-day`：服务端明确说『**有基线的事件线数**为 0』」；实际判据（后端 :579）是「有基线**且锚点日当天有证据**」的事件线数为 0——注释漏掉当日证据这半，正是 design D3 记录的、只看基线会错报 388 条的那个口径。
- **影响**：注释误导后来者；与前端的 normalizer 注释（boardRadar.ts:286-288，写对了）自相矛盾。
- **处置建议**：改为「既在近 3 日有基线、又在锚点日当天有证据的事件线数为 0」。

### R-5 2026-11-07【Low】「今日/今天」用字不一致；test-cases 一处 spec 引用非原文

- **状态**：已修复（统一为「今天」，与界面其它文案一致；spec 引用改为实文，2026-11-07）
- **位置**：tasks.md:31；test-cases.md（§4.2 空集行、§6.1 B11、§3 展示字段表）；BoardRadarReadingRail.vue:238；BoardRadarView.test.ts:393
- **事实**：tasks 4.3 与 test-cases 两处写「**今日**没有新起的事件线。」，实现与组件测试为「**今天**没有新起的事件线。」——档名是「今日新起」，同一张卡内今日/今天混用。另 test-cases §3 的 spec 锚「今日新起…SHALL 显示其来源数与热度」在主 spec 与 delta 中均无此句（最接近的是 Scenario「今日新起单列且不显示倍率」；rail 的 newStoryMeta 确实显示来源数与热度，行为本身无恙）。
- **影响**：纯文字一致性；spec 引用失真降低 trace 可信度。
- **处置建议**：实现文案统一为「今日没有新起的事件线。」（与档名一致），组件测试同步；test-cases §3 引用改为 spec 实文。

## 三、通过项明细（验了什么、结论如何）

1. **三档互斥本体**：`newStories` 排除截断后 topIDs（:620-639），排除仅作用于 `!HasBaseline` 分支；升温与该读重叠为 spec 明文豁免。后端 `TestRadarStoryFeedNewStoriesExcludeTopStories`（6 条无基线线，Top5 覆盖 5、新起只剩第 6 条）+ 真接口 DOM 提取（A-4 交集空）双重验证。**通过**。
2. **诚实空态（拍板项 1）**：`no-cross-day` 文案「当前没有跨天延续的事件线。＋跨天延续能力尚在建设中。」与 spec 逐字一致（rail:181-184），真接口 `cross_day_story_count: 0` 下实测渲染（A-2）；`not-triggered`/`unknown` 均回退旧文案，spec Scenario:63/75 两条措辞义务满足，组件测试三态文案互异断言在案。**通过**。
3. **该读不硬卡来源数（拍板项 3）**：无 `participants` 门槛代码；`TestRadarStoryFeedTopStoriesIgnoreParticipantFloor`（4 条单来源照常进 Top5）钉住；spec.md:7 显式写明「SHALL NOT 按来源数设置硬门槛」。**通过**。
4. **`new_count` 口径四处一致 + 截断前赋值**：见重点项 2。**通过**。
5. **归一三态与消费点**：见重点项 4，无 `??`/`||` 误用、`0`/`null` 全链路可区分。**通过**。
6. **降级安全**：旧后端（缺字段）→ `null` → `unknown` → 不断言成因；旧后端的 `new_count` 语义下摘要分支同样不触发（空 ⟺ 0 恒成立），无错误摘要风险；无 DDL/迁移/回填。**通过**。
7. **验收证据诚实度**：见重点项 6。**通过**。
8. **scope 与回归**：见重点项 7、8。**通过**。

## 四、核实记录

- 制品：proposal / ui-design / design / specs delta / tasks / test-cases / ui-verification 逐行读完；openspec validate --strict → `Change 'deconflict-radar-warming-and-new-stories' is valid`。
- 实现：`board_radar_repository.go`（finalizeRadarStories 全函数 :526-663）、`board_radar_stories.go`（RadarBoardStoryPoints :135-230，HasBaseline/baseline 计算）、`boardRadar.ts`（normalizer :258-296）、`useBoardRadarOverview.ts`（:85-133）、`BoardRadarReadingRail.vue`（:155-242 三档模板）、`BoardRadarView.vue` 传参、`board_radar_stories_test.go`（20 个测试，本 change 5 个逐个读）、`BoardRadarView.test.ts`（三态/摘要/互斥用例）。
- 死代码判定链：NewCount 唯一赋值点 grep 全仓确认（:645）；normalizer 无过滤；截断上限 3 为常量。组件测试 :383 载荷 `newStories: [] ∧ newCount: 133` 与后端不变式矛盾，为不可能状态。
- 兄弟 change 交互：`enable-story-cross-day-continuation` proposal/design/tasks 读了 Why/What/Impact，层二日期窗口 ≤1 天、文件清单无 radar 侧重叠。
- 环境注记：`.pi/extensions/tests/policy-decision.smoke.cjs` 当前因缺 `./.pdec.cjs` 模块无法运行（cwd 无关，两处跑法同错）——与本 change 无关的 harness 环境问题，如实记录，不计入本 change 发现。
- 未运行任何后端/前端测试（只读纪律；绿证据采信 ui-verification 报告内的红绿记录并与代码逐条互证）。
