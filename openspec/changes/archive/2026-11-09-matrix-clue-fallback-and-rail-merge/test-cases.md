# Test Cases — matrix-clue-fallback-and-rail-merge

> 故事主线：**同一个「事件线数 > 0 才用事件线数」的降级规则，让历史未回填日不再空白（雷达小矩阵按分区数、控制台矩阵格与时间筛选折线按线索合计，且都诚实标注），右栏收敛为一块 8 条带徽标的「今日该读」清单，管线状态默认折叠、异常自曝。**

## ⓪ 继承与调整表（问句⓪：改契约了，旧资产逐行处置）

反查：`bash scripts/harness/test-assets.sh workshop-radar`（4 个历史 change）、`… workshop-board-console`、`… radar-pipeline-status`。本 change 改契约的 Requirement：workshop-radar「版块行构成与分档」「今日该读与升温提示」「版块总览聚合端点」、workshop-board-console「泳道热度口径」「观察窗与自由日期显示」、radar-pipeline-status「雷达总览页右栏底部常驻管线状态块」。

### workshop-radar（上一 change `deconflict-radar-warming-and-new-stories` / `surface-story-events-in-radar` 刚改过三档契约，本 change 再改）

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 今日新起单列且不显示倍率 | **改写**（名保留：卡片→行内徽标，「单列」语义改为徽标行内单列） | BoardRadarView.test.ts（radar-new-* 断言） | 改写：删卡片断言，改断言 `radar-story-<id>` 行内徽标 `[新起]` 且无倍率 |
| 今日新起与该读互斥 | **改写**（名保留：单清单结构性互斥 + 不重复条目断言） | board_radar_stories_test.go `TestRadarStoryFeedNewStoriesExcludeTopStories` | 删除（`new_stories` 字段移除，编译期无对象） |
| 新起全被该读覆盖时说明已列入该读 | **反向改写**（名保留：徽标即行内表达，SHALL NOT 另出说明行） | board_radar_stories_test.go `TestRadarStoryFeedNewStoriesSummaryCountWhenFullyCovered` + BoardRadarView.test.ts（radar-new-covered 断言） | 改写（断言改为徽标化新语义） |
| 新起超出列出上限时附剩余计数摘要 | **反向改写**（名保留：第 9+ 名不展示且 SHALL NOT 附摘要） | board_radar_stories_test.go `TestRadarStoryFeedNewCountNotAffectedByTrim` / `TestRadarStoryFeedNewTotalCount` + BoardRadarView.test.ts（radar-new-summary 断言） | 改写：删旧断言，改断言第 9 名不展示、无摘要行 |
| 升温判定 | **改写**（名保留：倍率从卡片挂到行内徽标，判据逐字不变） | BoardRadarView.test.ts（rd-warm 卡片断言） | 改写为徽标断言（`升温 4.0×` 挂事件线行） |
| 无升温 | **改写**（名保留：卡片删除后无空态可言） | BoardRadarView.test.ts（radar-no-warm「暂未触发升温条件」断言） | 改写：断言无升温卡片/无空态文案 |
| 无跨天延续事件线时的空态 | **改写**（名保留：卡片空态→一行脚注） | BoardRadarView.test.ts | 改写：断言脚注「跨天延续事件线建设中」且非卡片 |
| 旧响应缺跨天线计数时不冒充无跨天线 | **保留（改消费方）**：`crossDayStoryCount null` 归一断言不变；`WarmingEmptyKind` 删，脚注布尔 `crossDayBuilding` 承接 | boardRadar.test.ts + useBoardRadarOverview.test.ts | 保留归一断言；删 `warmingEmptyKind` 断言，新增 `crossDayBuilding` 三态 |
| 今日该读按事件线跨版块混排 / 排序只按热度与来源数 / 该读不按来源数硬过滤 / 今日按各版块自己的锚点日… / 锚点日取该版块自己的最新日报日… / 滞后版块显示自己的日期 / 分区多但事件少不升温 / 未回填版块不冒充无事件 / 部分版块延迟时排序不完整明示 / 单版块事件线计算失败同样明示排序不完整 / 打开工坊首屏 / 从雷达进入版块 / 热点行带理由与密度 / 常温版块折叠 / 关注度不因分区数虚高 / 一次请求渲染雷达 / 无数据版块 / 事件线清单随行同响 / 返回主页入口与全局治理同组 | **保留不动**（契约未变） | board_radar_stories_test.go / board_radar_repository_test.go / BoardRadarView.test.ts / useBoardRadarOverview.test.ts / BoardRadarMasthead.test.ts | 零改动 |

### workshop-board-console

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 矩阵格值是桶峰值 | **改写**（THEN 增线索合计兜底句） | observationLanes.test.ts（matrixCellView text 断言）+ laneDynamicsWindow.test.ts | 保留事件线主值断言；新增兜底/降级标注断言 |
| 同日分桶不冒充分区热度 / 颜色表达参与者数 / 多线格点名获胜线 / 来源同分稳定点名 / 无身份不伪造标题 / 空标题与零来源 / 断档可读 / 零线索可开 / 渲染矩阵 / 图例可见 | **保留不动** | board_radar_repository_test.go（同日分桶）/ ObservationMatrixGrid.test.ts / observationLanes.test.ts | 零改动（零线索可开另在新场景「真零事件不兜底成非零」加钉） |
| 拖选不重排 / 精确档回切 / 明确切独立档 / 档池分层失败 / 池未取到有提示可重试 / 池失败后切 365 档不残留误述 / 非法日期反馈 / 深链恢复 | **保留不动**（观察窗语义零改动） | laneConsoleView.test.ts / useConsoleSelection.test.ts | 零改动 |
| 首屏曲线即含历史 | **改写**（曲线口径句：线索合计→降级口径） | laneConsoleView.test.ts（buildPoolSeries 无 story_count 夹具，值恰不变） | 既有三用例期望值保留（现状兼容证明）；新增带 `story_count` 夹具的两条断言 |

### radar-pipeline-status

| 旧 Scenario | 处置 | 旧测试 | 动作 |
| --- | --- | --- | --- |
| 状态块常驻右栏底部 | **改写**（默认折叠 + 内容区改单清单表述） | BoardRadarView.test.ts（rd-rail-col 两段容器） | 改写：断言折叠摘要存在 + 展开交互 |
| 状态块展示五项事实 | **保留（先展开）** | BoardRadarStatusBlock.test.ts | 加「先展开」夹具（默认折叠后五项不在 DOM） |
| 首次对账未成功不呈现业务结论 / 对账失败保留旧值 / 总览取数失败不判未生成 | **保留不动**（R-17 语义不变；摘要行同样遵守） | BoardRadarStatusBlock.test.ts | 断言目标补摘要行变体（读取中/读取失败不判未生成） |
| 手动日报生成入口 / 双队列清空判据全部 Scenario | **保留不动**（两个 Requirement 零改动） | BoardRadarStatusBlock.test.ts | 零改动（展开态下断言，先展开） |

## 主链路表（问句①：节拍全）

| # | 步/动作 | 来源 Scenario | 期望 | 层 | 落点 |
| --- | --- | --- | --- | --- | --- |
| M1 | 后端聚合 9 条热度递减事件线 | 清单上限 8 且不再有独立新起档 | `top_stories` 恰 8 条；响应无 `warming`/`new_stories`/`new_count`/`new_total_count` | repository（隔离 PG） | board_radar_stories_test.go `TestRadarStoryFeedTopStoriesLimitEight` |
| M2 | 同上数据下 `warming_count` 全量计数 | 徽标判定信息随条目自带 | `warming_count` = 升温线全量（不截断）；条目自带 `has_baseline`/`ratio`/`has_today_evidence` | repository | board_radar_stories_test.go `TestRadarStoryFeedWarmingCountWithoutList` |
| M3 | 归一：raw `stories` 段（带四废字段）→ 读模型 | 徽标判定信息随条目自带 | 读模型不含 `warming`/`newStories`/`newCount`/`newTotalCount`；`topStories`/`warmingCount`/`crossDayStoryCount` 照常 | 纯函数单测 | boardRadar.test.ts |
| M4 | 雷达页挂载（storyMode） | 无基线条目带「新起」徽标…/升温徽标挂在事件线上 | 8 条单清单；无基线行带 `[新起]`；ratio≥2 行带 `[升温 N×]`；无 `radar-new-block`/升温卡片/「另有 N 条」摘要 | 组件 Vitest | BoardRadarView.test.ts |
| M5 | `cross_day_story_count=0` / `>0` / `null` 三态挂载 | 无跨天延续事件线时的空态/有跨天线…无空态/旧响应缺计数… | 0→脚注一行；>0→无任何空态；null→无脚注 | 组件 Vitest | BoardRadarView.test.ts + useBoardRadarOverview.test.ts |
| M6 | 点徽标条目（有落点） | （继承）深链 | 跳 `/tags/:board/:date?story=<id>`（不变，回归钉） | 组件 Vitest | BoardRadarView.test.ts（保留用例） |
| M7 | 雷达行 `storyCounts[i]=0` 且 `counts[i]=4` | 小矩阵历史未回填日按分区数兜底 | 该格 title=`… · 无事件线（按分区数 4 显示）`；正常格 title=`… · N 条事件线` | 组件 Vitest | BoardRadarView.test.ts |
| M8 | `dayCellMetrics`：raw `story_count` 0/正数/缺省 | 矩阵格值是桶峰值（含兜底） | `displayCount` 按 D2 公式；`degradedFromClue` 仅 0+线索>0 为真 | 纯函数单测 | laneDynamicsWindow.test.ts |
| M9 | `matrixCellView` 降级格/真零格/未知格 | 历史未回填日按线索合计兜底/真零事件不兜底成非零 | text=132+aria 明示降级；text=0 无尾注；`?` 不变 | 纯函数单测 | observationLanes.test.ts |
| M10 | `buildPoolSeries` 混合日（story 0+线索132 / story 3+线索132 / 无 story_count） | 曲线与矩阵格同口径 | 值分别 132 / 3 / 现状合计；断档 0、全未知 null 不变 | 纯函数单测 | laneConsoleView.test.ts |
| M11 | ObservationRangeControls 挂载 | 首屏曲线即含历史（口径句） | 标注「日事件线数」+「无事件线日按线索合计兜底」；TrendAxes 单位「事件线数（条）」 | 组件 Vitest | ObservationRangeControls.test.ts（文案断言新增） |
| M12 | 管线块正常态挂载 | 默认折叠成一行摘要 | 一行摘要；五项事实/生成入口不在 DOM | 组件 Vitest | BoardRadarStatusBlock.test.ts |
| M13 | 点击摘要行 → 展开 → 再点 → 收起 | 默认折叠成一行摘要 | 五项事实出现/消失；`aria-expanded` 随态 | 组件 Vitest | BoardRadarStatusBlock.test.ts |
| M14 | 异常态挂载（过点未出/失败任务/phase failed/积压≥100） | 异常态自动展开 | 自动展开（immediate）；异常消除回折叠 | 组件 Vitest | BoardRadarStatusBlock.test.ts |

负向节拍（SHALL NOT）：M1 无第 9 名与摘要字段｜M4 无升温卡片/新起卡片/摘要行/覆盖说明行，徽标不挂版块名｜M5 >0 无空态、null 无脚注｜M7 降级格不显示空白、不把分区数当事件线数呈现｜M9 真零不附降级说明｜M10 不混加两口径｜M12 折叠不渲染五项事实。外部依赖失败：旧响应缺段（M3 null 回退既有用例）、池未取到（继承用例零改动）。

## 五组变体走查（问句②）

- **输入**：空串/纯空白/分隔符——不适用（无用户自由文本输入；日期输入走既有 RDY 组用例，划除）。单 token/大小写/特殊字符——不适用（徽标/格值由服务端数值派生）。超长——事件线标题超长：既有 `storyTitle` 回退 `事件线 #ID` 与雷达行 title 全文用例保留，徽标不截断标题（ellipsis 由行内布局承接，划除专项）。
- **前置**：空集——storyMode 下 `topStories=[]` → 既有「今天没有可识别的事件线」空态保留；雷达无版块（空 boards）既有用例零改动。单元素——单条目带徽标（M4 子断言）。重复——同一 story 只出现一次（全局排序截 8 结构性保证；右栏不重复条目）。越界引用——第 9 名不展示（M1）；`counts[i]` 缺位（`?? 0` 兜底，白盒 B2）。部分满足——9 条中仅 1 条无基线（徽标只挂那一条）。
- **时间窗口**：边界两端——锚点日当天算不算：既有基线不含锚点日用例保留（零改动）；`isBeforeSchedule` 边界（恰好等于生成时刻 → 不算已过点，白盒 B5）。空窗口——近 3 日无证据 → 无基线 → 新起徽标（M4）；跨窗口——断档超基线窗的线按新起（继承口径，spec 已注明）。归一化——`ratio.toFixed(1)` 展示一位小数（4.1769→「升温 4.2×」）。
- **幂等**：重复执行——只读端点重复请求结果一致（radar 只读既有契约）；展开/收起重复点击状态往返（M13）。部分失败重试——对账失败保留旧值（继承用例）。并发——不适用（无新增写路径，划除）。
- **可用性（UI 必检前三）**：误输入反馈——不适用本次新增面（日期表单既有用例，划除）。空态——M5 三态 + storyMode 空清单既有空态。错误态——R-17 首次失败不渲染右栏内容（既有用例）；管线块读取失败（继承+摘要行变体）。加载态——雷达骨架屏既有用例；折叠摘要「读取中」不自业务结论（M12 子断言）。超长文本——划除（见输入组）。重复提交——生成入口防重复（继承，展开态下用例不动）。

## 展示字段盘点（问句⑤：改数据结构）

| 用户可见字段 | 来源语义 | spec 锚 |
| --- | --- | --- |
| 小矩阵格值（雷达行） | 事件线数；>0，否则分区数兜底（title 明示） | workshop-radar「版块行构成与分档」（本次 MODIFIED） |
| 小矩阵格子 title/aria | 正常「N 条事件线」/降级「无事件线（按分区数 N 显示）」 | 同上 |
| 控制台矩阵格值 + title/aria | 事件线数；>0，否则线索合计兜底（aria 明示） | workshop-board-console「泳道热度口径」（本次 MODIFIED） |
| 时间筛选曲线值 + 标注/单位 | 与矩阵格同口径；「日事件线数（无事件线日按线索合计兜底）」 | workshop-board-console「观察窗与自由日期显示」（本次 MODIFIED） |
| 「今日该读」条目徽标 | `新起`=!hasBaseline&&hasTodayEvidence；`升温 N×`=hasBaseline&&ratio≥2&&hasTodayEvidence | workshop-radar「今日该读与升温提示」（本次 MODIFIED） |
| 徽标图例行 | 「新起 = 近 3 日无基线；升温 = 达自身近 3 日基线 2 倍」 | 同上（SHALL 由徽标+图例表达分档信息） |
| 跨天线脚注 | `cross_day_story_count == 0` → 「跨天延续事件线建设中」 | 同上 |
| 管线摘要行 | 状态词 · 日报 x/y · 队列 n；异常词 accent | radar-pipeline-status「雷达总览页右栏底部常驻管线状态块」（本次 MODIFIED） |
| （删除）升温卡片/新起卡片/「另有 N 条」/「均已列入」 | — | 上述 Requirement 本次 MODIFIED 明确 SHALL NOT |

## 效果核对（问句④：真库量化）

- **触发原因**：降级展示依赖断言外因素（事件线回填覆盖面——真库只到 2026-10-30）。
- **方法**：`curl -s --noproxy '*' http://127.0.0.1:5100/api/semantic-boards/overview`（只读，无需部署即可统计）逐格比较 `lanes[].story_counts[i]` 与 `lanes[].counts[i]`。
- **量化结果（2026-11-09 实测，13 活跃版块 × 每板 top-3 泳道 × 14 日轴 2026-10-24～11-06）**：
  - 事件线 > 0 的格子：**156**（不降级，值与降级前逐格一致）；
  - 事件线 = 0 且分区数 > 0（**降级前空白/无信息**）：**122** → 降级规则下**全部**有兜底值（降级后空格子 0）；
  - 事件线 = 0 且分区数 = 0（真断档）：**100**，不标降级（`0` 是合法已知值，不当空白）；
  - 降级格的日期分布正好压在回填缺口上：10-24 **19**、10-25 **14**、10-26 **21**、10-27 **17**、10-28 **20**、10-29 **22**（六天合计 **113**，占 122 的 93%），10-30 之后仅零星 9 格（滞后版块 `刚果疫情新闻`/`卫生健康` 锚点日为 11-05）。
  - **口径备注**：任务 brief 引的用户目测「约 60 个格子」是按屏幕上可见的版块行/其中一段数的；本表按接口全量 top-3 泳道逐格统计，两者不矛盾，以接口口径为准。
  - 新旧端兼容实测：跑在 :5100 的旧构建仍下发 `warming`/`new_stories`/`new_count`/`new_total_count` 且 `top_stories` 只有 5 条 → 新前端 normalizer 把它们当可选字段忽略（不报错、不渲染幽灵条目），退化形态 = 5 条无徽标/脚注的清单（R5 已验）。
- **结论**：降级覆盖面与成因完全对上（回填缺口日集中兜底），真断档不误标；"降级后 0 空格子"由规则单测（`displayCount`/`laneCells` 六组边界）+ 上述逐格统计共同证明，无需等部署复测。

## 白盒附加（complex 档 MUST：分支表 + 边界值）

### 分支表

| 分支 | 输入 | 期望 | 覆盖 |
| --- | --- | --- | --- |
| displayCount: story>0 | story=3,total=132 | 3, degraded=false | M8/M10 |
| displayCount: story=0,total>0 | story=0,total=132 | 132, degraded=true | M8/M9/M10 |
| displayCount: story=0,total=0 | 0,0 | 0, degraded=false | M8/M9 |
| displayCount: story=null（raw 缺+全桶未知） | — | null（'?'/曲线跳过） | M8（既有用例保留） |
| displayCount: raw 缺+桶已知 | raw 缺,total=5 | 5（=现状 storyCount）, degraded=false | M8 兼容断言 |
| 小矩阵: story[i]=0,counts[i]=4 | — | 4,降级 title | M7 |
| 小矩阵: story[i]=0,counts[i]=0 | — | 0,正常 title（真断档） | 白盒 B1 |
| 小矩阵: counts[i] 缺位（长度不齐） | — | `?? 0` 不 NaN | 白盒 B2 |
| 徽标: !hasBaseline&&hasTodayEvidence | — | `新起` | M4 |
| 徽标: hasBaseline&&ratio≥2&&hasTodayEvidence | ratio=4.1769 | `升温 4.2×` | M4 |
| 徽标: hasBaseline&&ratio<2 | ratio=1.9 | 无徽标 | M4 子断言 |
| 徽标: !hasBaseline&&!hasTodayEvidence | — | 无徽标（残热不算新起） | 白盒 B3 |
| 脚注: cross=0 / >0 / null | — | 有/无/无 | M5 |
| 折叠: 初始/手动开/手动关/异常强开/异常消除 | — | 折叠/开/关/开/折叠 | M12–M14 |
| abnormal: 过点未出 | todayReady=false,isBeforeSchedule=false | 展开 | M14 |
| abnormal: 未到点未出 | isBeforeSchedule=true | 不展开（正常态） | 白盒 B4 |
| abnormal: 积压 99/100 | activeCount=99/100 | 不展开/展开 | 白盒 B5 |

### 边界值

- ratio=2.0 恰达阈值 → 升温徽标（≥2 含等号，继承升温判定语义）；1.99 → 无。
- `top_stories` 第 8/9 名 → 在榜/不在榜（M1）。
- 旧后端缺 `story_counts`（`storyCountsKnown=false`，整轴回退 `counts`）→ 走 `sections` 口径显示「N 个分区」，不标降级、不得写成「N 条事件线」（单测钉住；review R-5）。
- `isBeforeSchedule` 恰等生成时刻 → 未过点（B4/B5）。
- 队列 active 0 / 1 / 100：闸门（任一>0 禁生成）与展开（≥100）**两把尺子不同**——闸门不变，展开阈值仅 UI attention（B5 断言双尺解耦）。

### 不适用（划除留痕）

- 并发用例（无新增写路径）；用户自由文本输入用例（无新输入面）；k3 截图（minor 复用契约，机械断言为准）；agent-browser 端到端（交互均为组件层可机械断言的开合/徽标存在性，主链路 M1–M14 已全覆盖——人工豁免：部署后按效果核对节脚本在静态 :5100 复核）。
