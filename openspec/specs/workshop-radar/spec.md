# workshop-radar Specification

## Purpose
叙事工坊的首屏：跨版块回答「今天该看谁」。把各版块当日事件密度、近三日变化与最新日报压成一张可扫读的列表，并在缺省（未选中版块）状态下取代表格式的标签池视图。

## Requirements

### Requirement: 工坊首屏为雷达
`/tags` 在未选中版块时 SHALL 渲染「今日雷达」视图，SHALL NOT 再渲染辅助标签池或组合标签池。雷达 SHALL 在同一屏内给出：当日需关注的版块列表、跨版块的「今日该读」条目、以及「今天可能升温」的版块提示。选中版块后 SHALL 进入版块态势页（`/tags/:boardId`）。

#### Scenario: 打开工坊首屏

- **WHEN** 用户从侧栏进入叙事工坊且未指定版块
- **THEN** 页面 SHALL 渲染雷达列表，按关注度降序排列各版块
- **AND** SHALL NOT 渲染辅助标签池/组合标签池

#### Scenario: 从雷达进入版块

- **WHEN** 用户点击雷达中的某一版块行
- **THEN** 路由 SHALL 变为 `/tags/:boardId` 并渲染该版块的矩阵视图

### Requirement: 版块行构成与分档
每个雷达行 SHALL 包含：版块名、当日事件线数、近三日与前三日的热度环比、该版块前 3 条泳道的 14 天事件线密度、最新日报标题（无日报时 SHALL 显示「今天暂无日报」）、以及进入版块的动作。

「关注度」SHALL 由 lane 热度派生（该版块当日 `lane_heat`，保留一位小数），SHALL NOT 由 `section` 数、`事件` 数或分区数加权得到。行 SHALL 按关注度分档：热点行带「热点」分档标记；关注度低于阈值的版块 SHALL 折叠为一行「常温」摘要，不逐行占据注意力。

分档阈值 SHALL 为可配置项，且 SHALL 按 `lane_heat` 的真实分布标定（标定依据记录在 design 或实现文档中），MUST NOT 直接沿用旧 `section` 计数量纲下的 50 / 15。分档数字 SHALL 在界面上可读（hover 或行内），MUST NOT 只以颜色表达。

小矩阵（14 日事件线密度）SHALL 逐格降级兼容（matrix-clue-fallback-and-rail-merge）：格子值 = 该泳道该日事件线数 > 0 用事件线数，否则按**该日分区数**兜底（`story_counts[i] > 0 ? story_counts[i] : counts[i]`；`counts[i]` = 该泳道该日分区数——雷达小矩阵实际可得的口径，控制台矩阵可用线索合计，两处各按可得口径兜底）。降级格子的 title/aria SHALL 明示降级（如「11/02 · 无事件线（按分区数 4 显示）」），SHALL NOT 静默混用两种量纲冒充事件线数；事件线数未知（旧后端缺 `story_counts`，整轴回退 `counts`）时沿用现状口径不加降级标注。降级只影响格子展示，MUST NOT 参与 `lane_heat`、行序或分档计算。

#### Scenario: 热点行带理由与密度

- **WHEN** 某版块当日 `lane_heat` 为全站最高且环比上升、且达到热点阈值
- **THEN** 该行 SHALL 排在最前、带「热点」分档标记，并展示最新日报标题与泳道事件密度条

#### Scenario: 常温版块折叠

- **WHEN** 某版块近 14 天没有任何事件（无事件线且无非综合稿事件）
- **THEN** 该版块 SHALL 只出现在「常温」摘要行中，不展开为独立雷达行

#### Scenario: 关注度不因分区数虚高

- **WHEN** 版块 A 当日 8 个 section 但只对应 1 个事件线（2 个来源），版块 B 当日 2 个 section 对应 2 个事件线（共 6 个来源）
- **THEN** B 的 `lane_heat` SHALL 高于 A，雷达排序 SHALL 把 B 排在 A 之前，MUST NOT 因 A 的 section 数更多而反转

#### Scenario: 小矩阵历史未回填日按分区数兜底

- **WHEN** 某泳道某日事件线数为 0（事件线回填未覆盖的历史日）而该日分区数为 4
- **THEN** 该格 SHALL 按 4 着档显示，title/aria SHALL 标注「无事件线（按分区数 4 显示）」
- **AND** SHALL NOT 显示空白或 0 冒充「无事件」，SHALL NOT 把 4 当作事件线数呈现
- **AND** 该兜底值 SHALL NOT 影响 `lane_heat`、行序（story 合计筛选键用原始 `story_counts`）与关注分档

### Requirement: 今日该读与升温提示

雷达右栏 SHALL 以**一块清单**呈现跨版块事件线：「今日该读」单清单按全局热度混排，上限 **8** 条（由 5 扩到 8：单清单吃掉原「今日新起」档的屏位，右栏总屏占不变；2026-11-09 用户拍板合并）。SHALL NOT 再渲染独立的「今天可能升温」「今日新起」分档卡片，同一事件线在右栏 SHALL 只出现一次。

「今日该读」SHALL 展示跨版块全局 Top N 事件线（每条带版块名），取自全部活跃版块的事件线集合并按全局热度混排；SHALL NOT 再以版块日报标题充当条目；SHALL NOT 按来源数设置硬门槛（热度公式本身含来源权重，多来源者自然靠前）；超出上限的候选 SHALL NOT 展示，也 SHALL NOT 附「另有 N 条」类剩余计数摘要（徽标化的单清单里，第 9+ 名与普通未上榜事件线同权，不构成需要单独交代的信息）。

分档信息 SHALL 以**行内徽标**表达，且只在存在时出现（无徽标 = 普通当日事件线）：

- `has_baseline = false`（近 3 日无任何热度证据）且当日有报道证据的条目 SHALL 带「新起」徽标，SHALL NOT 显示倍率（不按 0 倍或 ∞ 硬凑）；
- `has_baseline = true` 且 `ratio ≥ 2` 且当日有报道证据的条目 SHALL 带「升温 N×」徽标（N 为该事件线自身倍率，保留一位小数）；倍率 SHALL 挂在该事件线上、SHALL NOT 挂在版块名上、SHALL NOT 用版块合计热度判升温。

徽标化只是展示层收敛：升温判定与热度口径不变——`ratio` = 该事件线自身的锚点日 `story_heat` ÷ 其自身近 3 日基线（锚点日之前 3 个 CST 日终日均，不含锚点日）≥ 2，且该事件线在锚点日当天至少有一条报道证据（signal 落在锚点日 CST 日内）；判定单位（事件线自身）、阈值、热度来源（`story_signals` 派生，口径版本 `heat-v1-48h-hl24h`）与排序（原始热度降序 → 来源数降序 → 存活事件线 id 升序，与日报「今日重点」候选排序同一口径）全部不变。

「跨天延续建设中」脚注：当服务端判据 `cross_day_story_count` 为 0（当日没有任何一条事件线**既在锚点日之前 3 日基线窗口内有热度证据、又在锚点日当天有证据**）时，右栏 SHALL 在清单下方以一行脚注明示跨天延续能力尚在建设中，SHALL NOT 占用独立卡片；`cross_day_story_count > 0`（有跨天线但今天都没到 2 倍）时 SHALL NOT 显示任何升温空态（清单徽标已表达）；前端收到缺少该判据的旧响应时 SHALL 不出脚注，SHALL NOT 断言「没有跨天延续的事件线」。跨天延续事件线是否存在 SHALL 由服务端以「既有锚点日之前 3 日基线窗口内的证据、又有锚点日当天证据的事件线数」判定并随响应下发，SHALL NOT 只看「有基线」（锚点日之前就结束、只剩残热的旧线有基线但并未延续到今天），SHALL NOT 由前端凭条目渲染结果反推；本 spec 中「跨天延续的事件线」即按上述判据定义（可验证表述）——断档超出 3 日基线窗的线按「新起」徽标处理，属已知口径边界（见 design D3；`enable-story-cross-day-continuation` 层二 ≤1 天延续窗口落地后该边界收窄）。

「今日」SHALL 为**各版块自己的锚点日**（该版块**自己的最新日报日**）的 CST 日终，SHALL NOT 取自然今天；每个版块的事件线 SHALL 在**该版块自己的锚点日**同一时刻计算当日热度与近 3 日基线，SHALL NOT 因版块滞后而整板缺席。锚点日 SHALL 取该版块自己的最新日报日，SHALL NOT 受泳道轴（只覆盖锚定到活跃话题的 section）影响——最新一期日报未锚定到活跃话题的版块仍 SHALL 以自己的最新日报日测量并出条目。条目 SHALL 显式携带其所属版块的锚点日，滞后版块 SHALL 显示自己的日期，SHALL NOT 冒充全局锚点日的日期。跨版块混排时热度取自各自锚点日，存在「滞后版块少一天累积」的系统性偏差；该偏差 SHALL 视为已知口径并如实呈现，SHALL NOT 以人为补偿伪造同刻可比性。

清单 SHALL 只读，SHALL NOT 触发日报生成、事件线回填或标签匹配；条目有正文落点时 SHALL 可点击定位（跳该事件线落点的日报并定位正文，`?story=` 深链），无落点 SHALL 呈现为静态行并提示。事件身份回填状态 SHALL 以**板级判定**给出（只看该版块自己的报告线程带不带 `story_id`，SHALL NOT 依赖话题是否锚定到活跃话题——候选话题锚定的版块同样有真实事件线）：状态为「未回填」（有事件候选线程却都不带 `story_id`）的版块 SHALL NOT 产生任何条目，SHALL NOT 被呈现为「今天没有事件」，界面 SHALL 明示未回填版块数量并说明其未纳入排序；状态为「无可回填对象」（板内没有任何事件候选）的版块 SHALL NOT 计入未回填数。端点因超时降级（`data_delayed`）或单版块事件线计算失败导致该版块缺数据时，SHALL 明示本次事件线排序不完整。

#### Scenario: 今日该读按事件线跨版块混排

- **WHEN** 版块 A 有事件线「甲」当日热度 5.0、版块 B 有事件线「乙」当日热度 3.0
- **THEN** 「今日该读」SHALL 同时列出甲与乙，SHALL NOT 只保留热度更高的版块
- **AND** 每条 SHALL 带上其所属版块名与来源数，SHALL NOT 渲染日报标题作为条目

#### Scenario: 排序只按热度与来源数

- **WHEN** 两条事件线当日热度相同而来源数分别为 3 与 1
- **THEN** 来源数 3 者 SHALL 排在前
- **AND** 排序 SHALL NOT 受其所在版块的 `section` 数、`thread` 数或关注度影响

#### Scenario: 清单上限 8 且不再有独立新起档

- **WHEN** 当日跨版块候选事件线有 9 条，其中热度第 6/7/8 名为无基线的新起线
- **THEN** 「今日该读」SHALL 列出热度前 8 条（新起线按热度自然竞争入榜，进入者带「新起」徽标），第 9 名 SHALL NOT 展示
- **AND** 右栏 SHALL NOT 渲染独立的「今日新起」卡片，SHALL NOT 附「另有 N 条今日新起事件线」或「N 条均已列入」摘要行

#### Scenario: 今日新起单列且不显示倍率

- **WHEN** 某事件线当日热度 2.0 而其近 3 日基线为 0（近 3 日无任何热度证据）且当日有报道证据，该条进入「今日该读」
- **THEN** 该条目行内 SHALL 带「新起」徽标（无基线的分档信息单列在行内，不再单列卡片）
- **AND** SHALL NOT 显示倍率，SHALL NOT 以 0.0× 或 ∞ 冒充

#### Scenario: 升温判定

- **WHEN** 某事件线当日热度 4.0、其自身近 3 日基线 1.0、且当日有报道证据，该条进入「今日该读」
- **THEN** 该条目行内 SHALL 带「升温 4.0×」徽标
- **AND** 倍率 SHALL 挂在该事件线上，SHALL NOT 挂在其所属版块上

#### Scenario: 分区多但事件少不升温

- **WHEN** 某版块当日 8 个 section 但其事件线当日热度 1.2、自身基线 1.0
- **THEN** 该条目 SHALL NOT 带「升温」徽标，SHALL NOT 因 section 数 ≥ 3 而带徽标

#### Scenario: 无升温

- **WHEN** 当日存在跨天延续的事件线，但没有任何一条的热度达到自身近 3 日基线的 2 倍
- **THEN** 右栏 SHALL NOT 显示任何升温空态或脚注（清单徽标已表达升温与否）
- **AND** SHALL NOT 声称「没有跨天延续的事件线」——该措辞 SHALL 仅用于服务端判据为 0 的情形

#### Scenario: 无跨天延续事件线时的空态

- **WHEN** 当日没有任何事件线同时具备锚点日之前 3 日基线窗口内的证据与锚点日当天的证据（`cross_day_story_count == 0`）
- **THEN** 右栏 SHALL 在清单下方以一行脚注明示跨天延续能力尚在建设中
- **AND** 该脚注 SHALL NOT 占用独立卡片，SHALL NOT 以「暂未触发升温条件」表述该成因

#### Scenario: 旧响应缺跨天线计数时不冒充无跨天线

- **WHEN** 前端收到的响应缺少用于判定跨天延续事件线是否存在的服务端字段（`cross_day_story_count`）
- **THEN** 右栏 SHALL 不出「跨天延续建设中」脚注
- **AND** SHALL NOT 断言「没有跨天延续的事件线」——无法判定时 SHALL NOT 断言某一成因

#### Scenario: 今日新起与该读互斥

- **WHEN** 同一事件线既是当日热度 Top 8 又满足升温或新起判据
- **THEN** 该事件线 SHALL 只在「今日该读」清单中出现一次（行内徽标承载分档信息），徽标与清单条目 SHALL 保持一一对应
- **AND** SHALL NOT 在右栏任何位置重复渲染同一条事件线（单清单结构性互斥，不再有可重复的第二块）

#### Scenario: 新起全被该读覆盖时说明已列入该读

- **WHEN** 当日的无基线事件线全部进入「今日该读」Top 8
- **THEN** 每一条 SHALL 带「新起」徽标（徽标即「已列入该读」的行内表达），SHALL NOT 另出「N 条均已列入」说明行
- **AND** 右栏 SHALL NOT 因新起全被覆盖而渲染任何额外的覆盖说明或空档

#### Scenario: 新起超出列出上限时附剩余计数摘要

- **WHEN** 当日新起事件线热度排在第 9 名及以后（未进入 Top 8）
- **THEN** 这些条目 SHALL NOT 展示，也 SHALL NOT 附「另有 N 条今日新起事件线」剩余计数摘要（徽标化的单清单里，第 9+ 名与普通未上榜事件线同权）
- **AND** 未列出的条目 SHALL NOT 被表述为「新起事件线另有 N 条」——上榜与否由同一热度排序决定

#### Scenario: 该读不按来源数硬过滤

- **WHEN** 当日热度最高的 8 条事件线中有 4 条各自只有 1 个来源、第 1 条有 2 个来源
- **THEN** 「今日该读」SHALL 照常列出这 8 条，SHALL NOT 因来源数为 1 而剔除其中任何一条

#### Scenario: 今日按各版块自己的锚点日而非自然今天

- **WHEN** 版块 A 的锚点日为 2026-11-05、版块 B 的锚点日为 2026-11-06，而自然今天为 2026-11-07
- **THEN** 版块 A 的事件线 SHALL 以 2026-11-05 的 CST 日终计算当日热度与近 3 日基线，版块 B 的 SHALL 以 2026-11-06 的 CST 日终计算
- **AND** 界面 SHALL 标明各条目的锚点日，SHALL NOT 以 2026-11-07 的数据出条目
- **AND** 版块 A 的事件线 SHALL 仍出现在清单中，SHALL NOT 因其滞后于版块 B 而整板缺席

#### Scenario: 锚点日取该版块自己的最新日报日而非泳道轴末日

- **WHEN** 某版块最新一期日报的 section 未锚定到任何活跃话题（该版块没有活跃泳道，泳道轴末日为空或更早）
- **THEN** 该版块的事件线 SHALL 仍以**自己的最新日报日**的 CST 日终测量并出条目
- **AND** SHALL NOT 因泳道轴末日为空而整板缺席

#### Scenario: 滞后版块显示自己的日期

- **WHEN** 某条事件线所属版块的锚点日为 2026-11-05，而全局锚点日为 2026-11-06
- **THEN** 该条目 SHALL 显示 2026-11-05
- **AND** SHALL NOT 显示为 2026-11-06，SHALL NOT 冒充全局锚点日

#### Scenario: 未回填版块不冒充无事件

- **WHEN** 某活跃版块的事件身份未回填（板级判定：板内有事件候选线程却都不带 `story_id`）
- **THEN** 该版块 SHALL NOT 产生任何事件线条目
- **AND** 界面 SHALL 明示「事件身份未回填」并说明该版块未纳入排序，SHALL NOT 呈现为「今天没有事件」
- **AND** 板级判定 SHALL NOT 依赖话题是否锚定到活跃话题——候选话题锚定但事件身份已回填的版块 SHALL 照常出条目，SHALL NOT 被误判成「未回填」
- **AND** 无可回填对象（板内没有任何事件候选）的版块 SHALL NOT 计入未回填数

#### Scenario: 部分版块延迟时排序不完整明示

- **WHEN** 端点因超时把某版块降级为 `data_delayed`
- **THEN** 事件线清单 SHALL 标注本次排序不完整
- **AND** SHALL NOT 把缺该版块数据的清单呈现为完整排序

#### Scenario: 单版块事件线计算失败同样明示排序不完整

- **WHEN** 某版块的事件线计算失败（该版块条目缺席）
- **THEN** 事件线清单 SHALL 标注本次排序不完整，并把该版块 id 列入降级版块清单
- **AND** SHALL NOT 把该降级标成「整板数据延迟」，SHALL NOT 静默呈现为完整排序

### Requirement: 版块总览聚合端点
后端 SHALL 提供 `GET /api/semantic-boards/overview`，一次返回所有活跃版块的雷达行所需数据（版块 id/名称/标签数、当日分区数、近 3 日与前三日分区数、14 天分日分区数、最新日报的日期/标题/文章数/事件数），以及顶层的事件线清单段（锚点日、热度口径版本、今日该看 Top N（上限 8，条目自带 `has_baseline`/`ratio`/`has_today_evidence` 供行内徽标判定）、升温全量计数（`warming_count`，供报头）、跨天线判据（`cross_day_story_count`，供脚注）、板级回填状态标识（未回填 / 无可回填对象）与降级标记）。事件线清单段 SHALL NOT 再返回独立的升温/新起清单数组与去 Top 计数（`warming`/`new_stories`/`new_count`/`new_total_count` 随本变更移除——徽标信息随 `top_stories` 条目自带，独立清单与三态摘要失去消费方，留着即死契约）。端点 SHALL 只读，聚合口径 MUST 复用既有泳道动态、事件线热度与日报的实现，不得在前端或新服务中复制统计公式；跨版块归并与排序 SHALL 在后端完成，前端 SHALL NOT 另行合并或重排事件线。版块无数据时 SHALL 返回零值而非 404。事件线清单 SHALL 随同一次响应返回，SHALL NOT 依赖第二个请求。

#### Scenario: 一次请求渲染雷达

- **WHEN** 前端加载雷达
- **THEN** 系统 SHALL 以单次 `GET /api/semantic-boards/overview` 获取全部版块行数据与事件线清单
- **AND** SHALL NOT 为每个版块分别请求泳道动态与日报列表

#### Scenario: 无数据版块

- **WHEN** 某活跃版块近 14 天没有日报或分区
- **THEN** 端点 SHALL 返回该版块且各计数为 0，前端 SHALL 将其归入「常温」

#### Scenario: 事件线清单随行同响

- **WHEN** 端点返回至少一个可识别事件线的版块
- **THEN** 响应 SHALL 同时给出该版块事件线在锚点日的当日热度、来源数与近 3 日基线
- **AND** SHALL NOT 只给出版块合计而缺事件线粒度

#### Scenario: 徽标判定信息随条目自带

- **WHEN** 端点返回 `top_stories`（上限 8）
- **THEN** 每个条目 SHALL 自带 `has_baseline`、`ratio`、`has_today_evidence` 与锚点日/落点字段（行内「新起/升温」徽标与深链全部由条目自身字段派生）
- **AND** 响应 SHALL NOT 携带独立的 `warming`/`new_stories` 清单数组与 `new_count`/`new_total_count` 计数字段

### Requirement: 总览报头操作区归组
总览报头 SHALL 把「返回主页」入口与「全局治理」「新建版块」等页头动作放在同一操作区（右上角），返回入口 SHALL 紧邻「全局治理」左侧并与其呈现为同一档操作按钮（边框、字号、悬停表现一致；同档口径：min-height 2rem、字号 0.7rem、边框 `--color-border-medium`、hover/focus 边框与文字变 `--color-accent`）；标题区 SHALL NOT 另挂返回入口。入口 SHALL 保持可达：可点回主页且带可读的文案与 aria 标签。

#### Scenario: 返回主页入口与全局治理同组

- **WHEN** 用户打开叙事工坊总览
- **THEN** 「主页」入口 SHALL 出现在报头右上操作区内且紧邻「全局治理」左侧
- **AND** 「主页」与「全局治理」SHALL 呈现为同一档操作按钮
- **AND** 标题区 SHALL NOT 单独悬挂返回主页入口

### Requirement: 降级不得呈现为业务结论

在「版块总览聚合端点」之外，端点与前端 SHALL 满足下列降级诚实性契约：**「没取到」SHALL NOT 被渲染成「就是 0」**。

端点 SHALL 为每个版块行给出「本次是否取到」的判定：当端点因超时未完成该版块、或该版块泳道动态计算失败时，该行 SHALL 标 `data_unavailable=true`，且其计数类字段（`attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count`）SHALL 输出**未知**（`null`）而非 0；该行 SHALL NOT 参与关注度分档、行序与「N 个板块值得关注」计数。取数正常但确实没有数据的版块 SHALL 仍返回零值行且 SHALL NOT 标 `data_unavailable`。

前端 SHALL 把未取到的版块与真实零值区别呈现：报头标题 SHALL NOT 出现「0 个板块值得关注」这类由未取到数据推出的结论；当日线索合计与热点数在存在未取到版块时 SHALL 显示未知（—），SHALL NOT 把未取到的计数合计为 0 或据此断言「没有热点」；未取到的版块行 SHALL NOT 显示「关注强度 0.0」「0 篇文章 · 0 个事件」「无日报日期」等由未取到数据推出的呈现，SHALL 以「数据未取到」表达未知态，并 SHALL 保留进入版块与重试的可达路径。

#### Scenario: 全部版块未取到时不报「0 个板块值得关注」

- **WHEN** 端点因超时未取到任何版块的数据（全部行 `data_unavailable=true`）
- **THEN** 报头标题 SHALL 显示「本次数据未取全」类表述
- **AND** SHALL NOT 显示「0 个板块值得关注」
- **AND** 页面 SHALL NOT 渲染「今天没有需要看的版块」这一业务空态（未取到 ≠ 真的没有）
- **AND** SHALL 提供重试入口

#### Scenario: 部分版块未取到时不污染统计

- **WHEN** 部分版块 `data_unavailable=true`，其余版块取数正常
- **THEN** 报头 SHALL 标注本次数据不完整
- **AND** 当日线索合计与热点数 SHALL 显示未知（—），SHALL NOT 把未取到的版块计为 0
- **AND** 未取到的版块 SHALL NOT 参与关注度分档与行序，SHALL NOT 被计入「N 个板块值得关注」
- **AND** 行内 SHALL 以「数据未取到」表达，SHALL NOT 显示「关注强度 0.0」

#### Scenario: 未取到与真实零值可区分

- **WHEN** 端点未取到某版块的数据
- **THEN** 该行 SHALL 标 `data_unavailable=true`，且 `attention` / `today_clues` / `last3_clues` / `prev3_clues` / `report_article_count` / `report_event_count` SHALL 为未知
- **AND** SHALL NOT 输出 0 冒充该版块的业务结论

#### Scenario: 真实零值仍照旧

- **WHEN** 某活跃版块近 14 天没有日报或泳道，取数本身成功
- **THEN** 端点 SHALL 返回该版块的零值行且 SHALL NOT 标 `data_unavailable`
- **AND** 前端 SHALL 仍将其归入「常温」，SHALL NOT 标为「数据未取到」

#### Scenario: 计算失败与超时同标

- **WHEN** 某版块的泳道动态计算失败（而非端点超时）
- **THEN** 该行 SHALL 与超时未取到的版块一样标 `data_unavailable=true`
- **AND** SHALL NOT 只对超时路径标未取到、让计算失败路径继续以零值冒充业务结论

### Requirement: 滞后版块不占前排

主列表的行序 SHALL 把「锚点日落后于全局锚点日」的版块（滞后版块）从关注度排序里拆出来：滞后版块 SHALL NOT 参与关注度分档（热点 / 关注 / 常温）、SHALL NOT 凭旧数据的关注度占据前排名次；SHALL 逐行显示在主列表最底部，SHALL 保留自己锚点日的「数据至 MM/DD」中性标注，SHALL NOT 冒充全局锚点日。

「没取到」的版块（`data_unavailable=true`）SHALL 排在滞后版块之下，行序序号 SHALL 连续不重号。

报头的「今日线索」合计 SHALL 只计锚点日与全局锚点日一致的版块：滞后版块的线索数是它自己那个更早的锚点日的数，SHALL NOT 计入今日合计（旧数据不污染今日口径）。

#### Scenario: 滞后版块的旧日线索不进今日合计

- **WHEN** 某版块锚点日为 2026-11-07、当日线索 3 条，全局锚点日为 2026-11-18
- **THEN** 报头「今日线索」合计 SHALL NOT 包含这 3 条
- **AND** 该版块自己的行内线索数 SHALL 照旧如实显示

#### Scenario: 滞后版块不占前排名次

- **WHEN** 某版块锚点日为 2026-11-07、而全局锚点日为 2026-11-18，且其关注度高于其它若干版块
- **THEN** 该版块 SHALL NOT 出现在按关注度排列的前排名次
- **AND** SHALL NOT 挂「热点 / 关注 / 常温」分档徽标
- **AND** SHALL 逐行显示在主列表末尾，并标「数据至 11/07」
- **AND** 该版块 SHALL NOT 被折叠隐藏（仍可进版块）

#### Scenario: 未取到版块排在最底

- **WHEN** 某版块 `data_unavailable=true`
- **THEN** 该版块 SHALL 排在滞后版块之后（列表最底）
- **AND** 行序序号 SHALL 与其它行连续、不重号
