# 矩阵/追踪阶段实施合同（8B.2 产出，本文件只做映射，不改代码）

> 批准基准：`ui-prototype/revision-2026-10-27-matrix-scope-cleanup/standalone.html`（1,571,048 字节，SHA256 `ad22103a7351c5c925291feb5bfa2344441e237913bb870c1c6a91a213becc77`），用户 2026-10-27「可以，我批准了」；阶段合同 `ui-design-matrix-draft.md`（头部 approved）+ `ui-approval-matrix-stage.md`；观察窗 = 用户选定 A 路线「名单稳定」（design D11）。
> **2026-10-27 父复核修订**：纠正观察窗 §2.1（观察日期不推导档位，原型 auto-swap 不入产品）；P1–P4 已裁落账 §7；P2 研究入口命名与 P4 有界缓存同步本文件。本文件是 8B.2 的字段/用例/分工合同：把已批准 UI 映射到产品组件/composable/API/路由，列状态模型、互斥辖区、验收落点。**不含任何产品代码改动**；离线样本 JSON、原型常数、调试壳一律不得进产品。

## 1. 真实读取边界（已核实的代码事实，产品数据流只能建立在这些之上）

| # | 事实 | 源码锚点 |
| --- | --- | --- |
| F1 | `GET /semantic-boards/:id/lane-dynamics?days=N`：窗口锚定板块 `MAX(period_date)` 往回 N 天（含端点）；lanes = 本板块非归档话题 ∩（active ∪ active watch 关联）；**窗内零分区的泳道不返回**；后端排序 = 窗内不同命中日 DESC → 最近命中 DESC → id ASC（前端另行按线索合计重排）；timeline 按日期倒序（新在前） | `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardLaneDynamics`（356 起） |
| F2 | 每 section 只带 `section_id/label/thread_count/events(前5条标题)/folded_count`，**无 thread id、无 summary**；截断常量 `LaneTimelineEventsPerSection = 5` | 同上 `LaneDynamicsTimelineSection`（230–238）、常量 27 行 |
| F3 | `topic_lane_snapshots` 每话题一行最新滚动态势（summary + as_of），**没有历史日摘要档案**；`snapshot=null` = 待结算（合法态） | 同上 `GetLaneSnapshotsByTopicIDs`（193 起）；`flow/daily-report.md` §泳道态势结算 |
| F4 | `GET /semantic-boards/:id/daily-reports?days=N`（默认 7、无上限钳制、锚定 now()）返回报告列表（`id/period_date/title/...`）；`GET /daily-reports/:id` 预载全部 sections（cluster_index ASC）及其**全量 threads（不截断）**，thread 含 `id/report_id/section_id/title/summary/related_article_ids` | `handler/daily_report_handler.go:listBoardDailyReports`（295）、`repository/daily_report_repository.go:GetReportByID`（396） |
| F5 | 产品已有「日 → 完整线索」生产级 join：列表按 `period_date` 找 report id → 详情按 `section_id` 精确匹配 → threads | `front/app/features/tags/components/console/SectionDrawer.vue:loadClues`（56–90） |
| F6 | 文章详情唯一正源 = `GET /articles/:id`（列表端点不带正文类大字段是红线，详情端点即为其唯一来源） | `front/app/api/articles.ts:getArticle`；`flow/reading.md` 约束 9 |
| F7 | 原型的「同 section 内唯一 exact 标题匹配」只是离线包适配（lane 事件是裸字符串）；**产品恒久关联必须走稳定 report/section/thread ID**（F5 模式），不得搬标题匹配 | `ui-design-matrix-draft.md` FB8 节；`implementation-matrix-checklist.md` 项 4 |
| F8 | watch 管理面板现款即纯 `boardId` 作用域；`persistentTopicId` 只作关联定位 | `front/app/features/tags/components/topic-watch/WatchManagePanel.vue:props`（20–22，`listWatches(props.boardId)`） |
| F9 | 总览已有全局治理单点（新建板块/扩充/回填/规则/合并/标签池六项） | `front/app/features/tags/components/console/BoardRadarGovernance.vue`（由 BoardRadarView 挂载） |
| F10 | 前端 lane-dynamics DTO 沿用 snake_case 原样透出（api 模块内既定约定，组件经 `LaneMatrixRow` 消费） | `front/app/api/laneDynamics.ts` 文件头注释 |
| F11 | 现产品行序 = 前端 `clueTotal → sectionTotal → topicId`（父 2026-10-27 确认保持，不得换后端 distinctDays 序） | `front/app/features/tags/composables/useBoardConsole.ts:buildLaneRows`（117–121） |
| F12 | `/tags/:boardId/research` 真实路由渲染 `BoardEnrichmentPanel`（研究工作台），非旧匈牙利关系图；P2 命名据此纠正 | `front/app/pages/tags/[boardId]/research.vue`（父实读） |

**API 改动结论：后端零改动、零新端点、零新参数。** 观察窗不新增 from/to（用户已裁决）；标题/摘要/相关文章全部复用 F4/F5/F6 既有链路。新增的历史读取仅为「365池 = 同一端点days=365的按需调用」；sourceDays=365时档/池共用一次请求，不发第二次。

## 2. 状态模型与数据流（观察窗 A 路线）

### 2.1 五路状态显式分离（观察窗 A 路线；2026-10-27 父复核纠正：观察日期不推导档位）

> **与原型的差异（必读）**：原型 `MatrixRange.context` 在观察范围恰好命中某档区间时自动切 base、否则提升 365 池为 base——**产品不得复制此行为**，那会在拖日期时换名单，违背用户最后 D11 裁决。产品里 base 永远 = sourceDays 档响应（A 通道），与观察范围无关。

| 状态 | 含义 | 变更时机 | 是否触发请求 |
 | --- | --- | --- | --- |
| `sourceDays`（档） | 7/14/30/365 四档之一；其 A 响应是**名单、排序、休眠、候选的唯一基准** | **仅**独立快捷档按钮 / 明确 URL `days=` 变更；拖选/输入/缩放/键盘一律不改 | 是（A 通道；=365 时与池共用同一响应，但仍是明确档选择，不是日期推导） |
| 观察范围 `{start,end}` | 显示裁切区间，UTC+8 含起止日；默认 = 当前档响应自身的 `[anchor-days+1, anchor]` | 刷选/双端拖/平移/缩放/键盘/文本输入/URL `start,end` | 否（**无论恰好多少天（含恰 7/14/30/365 日）都不改 sourceDays、不换 base、不把 365 池提为名单**） |
| `selectedLaneId` | 选中泳道（`?lane=` 既有语义不变） | 点行/点泳道/URL | 否 |
| `selectedDate` | 选中观察日（默认 = 所选泳道在可见区间内**最后一个有分区日**） | 点日格/演进节点/编年前后导航 | 触发「当日完整线索」按需读取（F5 join，带缓存） |
| 请求 epoch | 每板块×每通道（A=当前档、B=365 池）独立递增序号；切板块/切档使旧响应作废 | load() 时 `++seq` | — |

- **A/B 职责（唯一基准规则）**：A（sourceDays 档响应）始终决定名单、行序、休眠分组、候选清单；B（365 池响应）仅提供 A 名单内这些 `topic_id` 的**历史日内容**（观察范围超出档窗时的显示补充），MUST NOT 参与名单/排序/休眠/候选计算。
- **排序与休眠只在 source 档全轴计算**：行序 = 前端现有 `clueTotal → sectionTotal → topicId`（`useBoardConsole.ts` buildLaneRows 现行实现，父已确认保持，**不得换成后端 distinctDays 返回序**）；`clueTotal/sectionTotal` 必须按 source 档响应自身全轴（非观察轴、不因 B 合并 timeline 被重算）；休眠 = source 档全窗分区 ≤2。显示裁切只作用于 `timeline`（`day.date ∈ [start,end]`），**排序/休眠/成员不随裁切重算、不写任何 status**。三轴职责分明：base 轴（source 全轴，管名单/排序/休眠）｜观察轴（可见区间，管显示）｜全年池（B，仅补历史内容）。
- 可见轴 = `buildDayAxis(end, end-start+1)`（现有纯函数复用）；「今天」高亮按 Asia/Shanghai 日历日（`todayInShanghai` 复用），与数据锚点（A/B 同锚 = 板块 MAX(period_date)）分别表达。
- 范围标注：常态标「名单/排序/休眠按 N 日档」（N=sourceDays）；观察范围超出档窗、由 B 补内容的日期区段另标「历史内容由 365 日池补充（不改名单）」。
- URL 深链：`?lane=` 保留，新增 `days/start/end` query（`days` 仅接受四档值，是**显式档变更**，不因 start/end 推导；`start/end` 须 `YYYY-MM-DD` 且 `start≤end` 且落在池范围内，非法/越界 → 忽略该项并给出明确文案，**原视窗保持不变**，文案语义对齐原型 `MatrixRange.validate`：缺项/格式/顺序/池外四类）。
- 切板块：清空 range（回档默认）、selectedDate、编年展开态、休眠展开、选中泳道回退（沿用 `useBoardConsole` 现有清场 + 新增 range 相关字段）。

### 2.2 请求与缓存（P4 裁决：有界缓存，技术决定不再问用户）

- A 通道：`getLaneDynamics(boardId, sourceDays)`；B 通道：`getLaneDynamics(boardId, 365)`（仅当 sourceDays ≠ 365 且需要池内容时按需拉取；`sourceDays=365` 时同一响应复用，A/B 仅发**一次**请求）。
- 缓存作用域与上限（简单 Map 先入先出即可，禁引入通用缓存框架/新依赖）：
  - lane-dynamics 源缓存：**当前板块限 4 档**（7/14/30/365，365 池共用去重）；切板块失效；组件卸载释放。
  - 报告列表索引：仅当前板块 1 份；报告详情最多 **14** 份；文章详情最多 **20** 份；同样板块作用域 + 卸载释放。
  - 稳定性约束：当前展开对象（正在读的编年/摘要/文章）不因逐出而消失或回顶；失败响应不入成功缓存；成功才入缓存。
- 失败态分层（与既有语义对齐，A/B 独立分层）：A 成功 B 失败 → A 全量可读，超出档窗的历史日标「未取到」不补 0、不换名单、不强制改视窗；A 失败 B 成功 → **不得用 B 冒充档成功**，按 A 首次失败=错误态 / 刷新失败=staleNotice 保留旧 A 区分；矩阵/演进/编年共用 A，不各自发请求。
- 当日完整线索：`useLaneDayReading`（新，见 §3）持有 report 列表缓存 + report 详情缓存（上限见上） + section→threads join；article 详情按 id 缓存（同一文章跨 thread 复用，语义对齐阅读台 thread 文章缓存故事）。

## 3. 批准原型 → 产品落点映射

### 3.1 组件与 composable

| 原型区域（scope-cleanup） | 产品落点 | 动作 | 说明 |
| --- | --- | --- | --- |
| 矩阵壳/三区骨架 | `console/BoardConsoleShell.vue` | 改造 | 头部重排：视图 seg（矩阵/追踪｜归档）+ **最新日报主按钮**（`打开日报 · MM/DD`，唯一 latest 入口）+ 板块管理 + 新建泳道；撤窗口 `<select>`（FB6「不用 select」）→ 挂 `ObservationRangeControls`；**撤「更多」菜单**（全局六项唯一入口=总览 BoardRadarGovernance，F9）；日格点击改「选日 + 下方就地下读」（不再开 SectionDrawer） |
| 时间视窗（brush+快捷档+输入+平移缩放+键盘） | 新 `console/ObservationRangeControls.vue` + 纯函数 `console/observationRange.ts` | 新增 | SVG brush：365 池「已返回泳道日线索合计」曲线 + 选区/双端手柄（slider 语义、方向键逐日/Shift 逐周）+ 精确输入 + 平移/缩放按钮 + 快捷档；轴在绘图区外（原型 `trendFrame` 同构）；无新依赖（对齐 BoardHealthSignal 的内联 SVG 风格） |
| 矩阵（日格=桶峰值、去行尾总数、断档 '—'/0 可开） | `console/BoardMatrixView.vue` + `composables/useBoardConsole.ts` | 改造 | `buildLaneRow` 增 `peakCounts[date]=max(thread_count)`（缺省回退 `events.length+folded_count`，再缺=未知显示 `?`，不补桶数）；格值/aria/title 改「桶峰值 X 条 · N 个主题桶 · 当日合计 M 条」；**删行尾合计列**；无分区='—' 禁用，有分区 0 线索='0' 可开；列 >31 时容器内横向滚动（min-width 按列数），不做页面级横滚；footnote 常态标「名单/排序/休眠按 N 日档」，超档窗补内容的日期另标「历史由 365 池补充（不改名单）」 |
| 演进带（点径=日合计、无最密三日、>31 日趋势图+折叠逐日） | `console/BoardLaneEvolution.vue` + 新 `console/LaneTrendChart.vue` | 改造+新增 | 删「最密集三天」「今日事件」块（FB9 收敛；编年默认选最近内容日承接「看最近」）；节点点径 `min(24, 6+√total×5)`、节点数字=当日合计；标签阈值 ≥3 且可见日 ≤31；>31 日渲染 `LaneTrendChart`（日期 X 轴 + 线索合计 Y 轴 + 0 基线 + 真实比例刻度，点带 title）+「展开逐日索引」折叠；态势句改 `最近结算态势 · as_of（非本视窗总结）` 折叠 details，正文=快照逐字（`snapshot=null` = 待结算占位、时间线照常） |
| 当日编年（标题目录/单条就地真实摘要/相关文章按需/前后有内容日） | 新 `console/LaneChronicle.vue` + 新 `composables/useLaneDayReading.ts` | 新增 | 数据= A名单内所选ID的当日sections（A档内容/B补历史）经F5稳定ID join读取的全量threads；默认标题收束、同一时刻仅一条展开（点另一条收束前一条）；相关文章按钮按需展开（`related_article_ids` → `getArticle` 缓存，站内阅读走壳层既有 `ArticlePreviewModal`）；前后「有内容日」就地切换（跳过无分区日）；页脚「已返回 N 桶 · M 条标题 · 另有 K 条折叠」；caption 三指标（桶峰值/当日合计/主题桶数）+「打开 N 日报」动作（selectedDate≠最新报告日时保留**带完整日期**的历史日报动作，FB9 红线：不得误当重复 latest 删掉）；**SectionDrawer 的 loadClues 逻辑迁移进 useLaneDayReading** |
| `SectionDrawer`（32rem overlay） | `console/SectionDrawer.vue` | 退役于本路径（文件保留） | 矩阵日格不再开抽屉；组件与测试文件保留至 8.4 阶段④清理（不删文件、不动 BoardThreadBrowser）；其「打开日报/看归档」义务迁入编年 caption |
| 右轨（健康→构成候选→追踪→来源） | `console/BoardGovernanceRail.vue`（+`BoardHealthSignal.vue` 小改） | 改造 | 区块顺序与内容对齐原型 `contextRail`；健康 = 真实 `llm_score`/`checked_at`/评语首句两行预览 + 「最新快照 · 非所选历史评分」标注 + 完整依据入口（开治理抽屉 quality 页签，复用）；构成候选 top3 真实名称/实际清单数 +「高频共现 ≠ 必须收编」措辞；话题追踪 = 真实 watch 名称 + active/paused + **唯一管理入口（boardId）**（F8 已合规）+ 话题候选分列（人工激活，不自动转正）；来源 = 独立 7/30/90 窗切换 + 总量/来源数 + 贡献条 + top 行 +「独立统计，不随时间视窗」标注；**删 gov-advanced 全局六项**（唯一入口总览）与 gov-entries 中重复入口（研究/设置/话题管理 → 板块管理菜单） |
| 板块管理（设置/话题管理/研究归一区） | `BoardConsoleShell.vue` 头部新「板块管理」菜单（复用 `console-more` 的 details 菜单样式） | 新增（薄壳） | 只挂既有真实入口：数据增强设置（治理抽屉 settings）、话题管理（TopicManageDialog）、研究工作台（`/tags/:boardId/research` 路由）；不新增功能 |
| 追踪管理（板块作用域） | `topic-watch/WatchManagePanel.vue` | 复用不动 | 现款即 boardId-only（F8）；关注记录 `persistentTopicId` 只用于右轨「对应泳道」定位链接 |
| 窄屏（泳道摘要+迷你条+内联编年+来源内联） | `console/BoardNarrowSummary.vue` | 改造 | 迷你条仍分区数口径；挂 `LaneChronicle`（同一摘要/编年实现，桌面手机共用单一口径）；不横滚矩阵（既有约定不变） |
| 旧能力承接（研究入口） | `BoardConsoleShell.vue` 头部 seg 第三钮「研究工作台」 | 新增（仅入口） | **P2 裁决（2026-10-27 用户结构化问答）**：定名「研究工作台」，走真实既有 `/tags/:boardId/research`（实渲染 `BoardEnrichmentPanel`，非旧匈牙利关系图）；**不得称「关系/全景」、不得声称旧关系已恢复**；关系/关键事件按既有待办另议（A-56/R-11..R-16 不洗绿）；进研究台不改 foreign 研究页，返回上下文由矩阵自身保存（技术方案实现批自定）；见 §7 P2 落账 |

### 3.2 不动清单（foreign/他域，本阶段禁改）

`TagsPage.vue`、`BoardThreadBrowser.vue`（研究域复用）、`DailyReportReader.vue`/`BoardDailyReportTimeline.vue`/`daily-report/**`（阅读台域）、`BoardRail.vue`（仅样式不动，行为沿用）、后端 `lane_snapshot_repository.go`、`lane_dynamics_handler.go`、`daily_report_*`（bucketing/slim-lane 归属）、`boardSignals/boardEnrichment/Signal*`（研究域）、`useLaneTrendData.ts`（阅读台宿主，14 天档语义不同）、总览簇 `BoardRadar*`（8A 已交付，仅只读引用 BoardRadarGovernance 既有事实）。若实现期发现必须碰上述文件，停工列准确 hunk + 理由交父协调，不得覆盖。

## 4. 展示字段盘点（问句⑤：新字段必须有语义锚）

| 展示字段 | 来源（真实链路） | spec 锚 |
| --- | --- | --- |
| 日格数字（桶峰值） | A名单内同ID的显示日内容（A/B）`timeline[].sections[].thread_count` 当日 max（缺省回退 events+folded，未知='?'） | workshop-board-console「矩阵视图」MODIFIED |
| 日格 aria/title（桶峰值/桶数/合计三指标） | 同上派生 | 同上 |
| 演进节点数字与点径 | 当日各桶 `thread_count` 之和（`min(24,6+5√total)`） | workshop-board-console「选中泳道演进带」MODIFIED |
| 趋势图 Y 值（顶图/泳道图） | 顶图=池内**已返回泳道**逐日合计之和；泳道图=选中泳道逐日合计 | 「观察窗与自由日期显示」+「演进带」 |
| 编年标题列表 | A名单内同ID的显示日sections（A/B）的**全量 threads**（经 F5 join 读取，稳定 ID） | workshop-board-console「当日线索工作面」+ board-lane-dynamics「矩阵观察窗读取边界」 |
| 单条摘要 | `daily_report_threads.summary`（详情端点，按 thread id） | 同上 |
| 相关文章/导语 | `related_article_ids` → `GET /articles/:id`（按需、缓存） | 同上 |
| 态势句与 as_of | `topic_lane_snapshots` 最新滚动快照逐字（非历史日摘要） | board-lane-dynamics「泳道卡片构成」MODIFIED（既有锚，措辞补「非本视窗总结」标注） |
| 观察范围标注（档基准/池补内容） | sourceDays 常态 + B 补内容区段派生 | 「观察窗与自由日期显示」 |
| 健康评分/日期/评语首句 | `health-checkup` latest `llm_score/checked_at/llm_comment` | workshop-board-console「治理右轨」MODIFIED（既有锚，补最新快照标注） |
| 范围标注/休眠/排序文案 | source 档全轴派生（≤2 休眠、clueTotal→sectionTotal→ID），纯展示 | 「休眠泳道折叠」（既有锚）+ 观察窗锚 |

## 5. 互斥实现辖区与依赖（8B.3 派发切分；单 writer 或 ≤2 并行时按组串行）

| 组 | 辖界（文件） | 依赖 | 建议档位 |
| --- | --- | --- | --- |
| G1 数据层 | `useBoardConsole.ts`（双通道/epoch/缓存/观察范围/query）+ `observationRange.ts`（新纯函数） | 无 | implementer（zai-coding-cn/glm-5.3，默认档） |
| G2 视窗控件 | `ObservationRangeControls.vue` + `LaneTrendChart.vue` | G1（纯函数可先行并行） | implementer |
| G3 矩阵+演进 | `BoardMatrixView.vue`、`BoardLaneEvolution.vue` | G1 | implementer（与 G2 可并行，不同文件） |
| G4 当日编年 | `useLaneDayReading.ts`、`LaneChronicle.vue`、SectionDrawer 路径退役 | G1 | implementer |
| G5 壳层与右轨收敛 | `BoardConsoleShell.vue`、`BoardGovernanceRail.vue`、`BoardHealthSignal.vue`（小改）、`BoardNarrowSummary.vue` | G1–G4 全部（串行收口） | implementer（主线程验收后一次收口） |
| G6 测试与验证 | 各组随组落单测；G5 后跑定向 vitest + eslint；agent-browser 主故事留父/验证批 | 各组 | implementer + 父 |

- 冲突面：`BoardConsoleShell.vue` 只允许 G5 写；`useBoardConsole.ts` 只允许 G1 写——其余组经 props/events 消费。
- 测试文件与源同目录随组新增/修改（清单见 test-cases-matrix-stage.md 落点列）。
- 不跑全量：`cd front && pnpm test:unit <受影响文件...> --maxWorkers=2`；`pnpm exec eslint <受影响 ts 文件>`；typecheck/build/浏览器留 8B.4 收口批。

## 6. 验收命令与落点（8B.3/8B.4 判据，详见 test-cases-matrix-stage.md）

- 命令（每批）：`cd front && pnpm test:unit app/features/tags/composables/useBoardConsole.test.ts app/features/tags/components/console/<本组组件>.test.ts --maxWorkers=2`（参数不带 `--`）；`pnpm exec eslint <受影响 TS 文件>`。
- 后端：零改动 → 不跑 Go 包测试（`lane_snapshot_repository_test.go` 仅作回归网，若共享树该文件被他人改动则跳过不判红）。
- 浏览器主故事（8B.4）：静态托管 `:5100`，1440×900/1920×1080 + 375×667/390×844 双主题，机械断言（弹层 elementFromPoint/点击后到达状态/ellipsis/逐状态长内容）——计划见 test-cases-matrix-stage.md §真实浏览器。
- 深链回归：`/tags/:boardId?lane=&days=&start=&end=` 刷新恢复；旧 `?board=&report=&annotation=` 映射不受影响（现有用例照跑）。

## 7. 父裁决落账（2026-10-27 复核批；原 P1–P4 待决项全部已裁，后续代码批按此执行）

- **P1（已准，本批已执行）**：ui-design.md 历史残留矩阵措辞（密集三日/密集日/30日可动视口待审等，共 12 处 hunk：原行 6 段补纠正句、18、35、59、69、72、73、83、93、95、133、151）已改为批准形态（点径日合计/内联工作面/观察窗 A 路线/365 列容器内横滚/批准状态句）；原型文件与 hash 未动。
- **P2（已裁）**：用户结构化问答明确选「保留真实研究入口，关系后议」。顶栏第三入口定名「研究工作台」，承接真实 `/tags/:boardId/research`（实渲染 `BoardEnrichmentPanel`）；不得称「关系/全景」或声称旧关系已恢复；旧关系/关键事件按既有待办另议，A-56/R-11..R-16 责任不洗绿；不改 foreign 研究页，返回上下文由矩阵自身保存。已同步 ui-design.md 准入段、ui-approval-matrix-stage.md、ui-design-matrix-draft.md。
- **P3（已准）**：本阶段只撤 console「更多」菜单与右轨全局六项的**重复入口/宿主代码**；总览 `BoardRadarGovernance` 对应六项真实功能全部保留可达（含确认/取消流程），不动总览视觉、不删业务 API；S9 继承表中 A-36/A-57 标「改语义（入口路径变总览）」，旧历史结果不改绿，新版阶段真实复验另记。
- **P4（已裁，技术决定）**：有界缓存——lane-dynamics 源仅当前板块 4 档（365 池共用去重），切板块失效、卸载释放；报告详情 ≤14 份、文章详情 ≤20 份、报告列表索引仅当前板块 1 份；简单 Map FIFO/既有工具，禁引入通用缓存框架/新依赖；当前展开对象不因逐出消失/回顶，失败不入成功缓存；白盒边界见 test-cases-matrix-stage.md CA 组（0/恰上限/超 1 逐出/跨板块释放/当前读项稳定）。

## 8. 部署影响预告（8B.4 交付时向用户汇报的底稿）

纯前端静态发布（`bash scripts/dev/deploy-frontend.sh`）：矩阵/追踪页换为观察窗版面；无数据迁移/重生成/配置；旧数据不回填——「休眠/排序」仍是展示派生（source 档全轴），`topic_lane_snapshots` 只读消费；后端零改动不重启。用户可见变化：日格数字语义从分区数改为桶峰值、行尾总数消失、点格改为下方就地读、时间窗口由下拉改为刷选/输入（拖选永不换名单）、头部第三入口为「研究工作台」、右轨重排且全局治理回总览。

## 9. G1 数据层交接（2026-10-27 实现批回填；不改上文批准语义）

**已交付**（useBoardConsole 旧导出全量兼容，纯模型层拆至 `composables/laneMatrixModel.ts` 并自 useBoardConsole 原样 re-export，公共导入路径不变）：

- `components/console/observationRange.ts`：日期/范围纯函数（严格 YYYY-MM-DD 真实日历日、validate 四类文案、池边界钳制、平移/缩放/键盘端点、档默认区间、`parseTierDaysValue`、`SOURCE_TIER_DAYS/POOL_DAYS/DEFAULT_SOURCE_DAYS` 常量）。**无任何从日期推导档位的 API**（OX-1 结构保证）。
- `api/normalizers/laneDynamicsWindow.ts` + `api/laneDynamicsWindow.ts`：新数据面 API 边界（snake_case→camelCase、数字 ID→string；`dayCellMetrics` 三口径 peak/total/sectionCount + hasUnknown；失败错误体逐字段透传，不改 shared ApiResponse）。`LaneDynamicsWindowPayload = { window, legacy }`，legacy 原始响应同引用携带（F10 兼容，G5 收口后移除）。
- `composables/laneDynamicsChannels.ts`：通道 store（每板块 ≤4 档成功缓存 Map FIFO、同 (board,days) 在途去重含 force 合乘、失败不入缓存不逐出成功项、`invalidateBoard`/`dispose` 代数隔离——迟到响应标 `discarded` 不回填）。
- `composables/useObservationWindow.ts`：观察窗状态机（observation 范围、B 池 needsPool 自动按需 + `ensurePool/retryPool`、`resolveLaneDay(topicId:string,date)` 三态 content(tier|pool)/gap/unavailable、days/start/end query 双向同步、daysNotice/rangeNotice）。由 useBoardConsole 组合并整包透出。
- `composables/useBoardConsole.ts`：A 通道经通道 store 接线（`load()`=force 刷新语义不变；`setSourceDays`/`setWindow`=切档走缓存命中路径）；`sourceWindow`（camelCase A 数据）、`anchorDate`（deps.reportAnchor 注入优先，G5 壳层接 `latestReportDate`）、`visibleAxis`、`sourceDays` 别名透出；切板块失效旧板块缓存并复位观察窗；scope dispose 释放缓存。

**G2–G5 接线示例**（含 2026-10-27 R-22/R-23 复审修正）：

```ts
const console = useBoardConsole(boardIdRef, { route, router, reportAnchor: () => latestReportDate.value })
// 拖选中预览：只更新显示，不写 URL 不动 committedRange（校验失败→ rangeNotice，原范围保持）
console.applyObservationRange(start, end, { writeQuery: false })
// 松手提交（默认 writeQuery）：与 URL 已反映范围（committedRange）不同才写——
// 预览后同值提交恰写一次正确 start/end（保留 days/lane/report/annotation），重复提交幂等
console.applyObservationRange(start, end)
console.committedRange.value   // URL 已反映的已提交范围（深链恢复/切档/切板块时同步重置），G5 返回状态可用
// 单日内容：消费新数据面字符串 ID（sourceWindow 泳道的 topicId，API 边界已归一）
console.resolveLaneDay(lane.topicId, date)  // {kind:'content',source:'tier'|'pool',day} | {kind:'gap'} | {kind:'unavailable'}
dayCellMetrics(day)                          // 桶峰值/合计/桶数/未知（api/normalizers/laneDynamicsWindow）
console.poolLoading / poolError / poolStaleNotice / needsPool   // 「历史由 365 池补充」标注与未取到态
console.daysNotice   // 非法 days 文案：初始非法=回退默认 14；已有合法档=「保持 N 日档」（R-23）
```

**G3 注意（字符串 ID 边界纪律）**：新数据面（`sourceWindow`/`poolWindow`/`resolveLaneDay`/`dayCellMetrics`）的 ID 与字段已由 API 边界（normalizer）归一为字符串 ID + camelCase——组件/行模型应直接消费 `sourceWindow.lanes`（topicId 即 string），**不得在组件层散落 `String(id)`/raw snake_case 映射**；legacy `rows`（数字 topicId）仅过渡兼容（未迁移消费者用，G5 收口后移除）。

**本批未做**（后续组辖区）：G2 视窗控件、G3 矩阵/演进组件、G4 useLaneDayReading（报告列表 1 份/详情 14/文章 20 缓存契约留给该组，本批未实现）、G5 壳层（`reportAnchor` 注入、`daysNotice/rangeNotice` 渲染、`setWindow`→`setSourceDays` 换接）。.vue typecheck/build 留 G5/8B.4 集中。

**验证**（2026-10-27 实跑；R-22/R-23 复审批后复跑）：`pnpm test:unit`（--maxWorkers=2，参数不带 `--`）useBoardConsole 旧 22 用例零改动照跑 + observation 18（含 R-22 预览→同值提交恰一次/深链恢复提交/R-23 外部非法 days 保持当前档 3 项新用例）+ observationRange 16 + channels 9 + normalizer 9 + 消费组件/中间件 33；`pnpm exec eslint` 受影响 TS 文件 exit 0。基线核验：85 份中仅 useBoardConsole.ts 为本批变更，其余逐字节一致。

## 10. G2 视窗控件交接（2026-10-27 实现批回填；不改上文批准语义）

**已交付**（全部为本组新文件，零改动 G1/G3–G5 辖区文件）：

- `components/console/ObservationRangeControls.vue`（418 行）：时间视窗受控纯 UI。props `bounds: DateBounds|null`（池界，null=整组禁用占位）/`range: ObservationRange|null`（已提交观察范围，受控回流）/`sourceDays: number`（档按钮激活态）/`series: LaneTrendPoint[]|null`（365 池日合计曲线，null=未取到仍可刷选）/`loading`/`notice`（透传 G1 rangeNotice）。events：`preview(range)`（拖动中 → 父调 `applyObservationRange(start,end,{writeQuery:false})`）、`commit(range)`（松手/表单/键盘/平移缩放按钮 → 父默认提交；一次手势恰一次，R-22 同值松手语义由父幂等收口）、`source-change(days: 7|14|30|365)`（**仅独立档按钮**；拖/输成恰档宽不发，OX-1）。刷选四模式（空白反向合法/选区平移/双端手柄/键盘 ±1·Shift±7·Home·End）走 `rangeBrush.ts` 纯状态机 + 绘图区真实 rect 映射；pointercancel/Esc 恢复起始选区不提交；指针事件监听挂绘图区 SVG 自身（真浏览器 setPointerCapture 重定向，真机触摸未验）。
- `components/console/RangeDateForm.vue`（160 行）：YYYY-MM-DD 严格文本表单（G1 `validateObservationRange` 四类文案，非法不 emit `commit` 只 emit `issue` 文案）+ 平移/缩放按钮（离散动作直接 commit）；字段随 range 回流实时同步（原型 paint 同构）。
- `components/console/LaneTrendChart.vue`（218 行）：纯 series 趋势图（EV-6 长窗形态）。props `series: LaneTrendPoint[]`（`{date, value: number|null}`，升序由上层保证）/`selectedDate`（受控高亮）/`loading`/`label`（宿主口径文案）/`emptyText`。event `select-date(date)`（仅已知值日含已知 0；未知日无点不可点）。null 不补 0 不桥接（分段折线）、已知 0 画基线、全未知不伪造量级；点带 title 读日期/实际数、不抢 Tab（键盘选日归 G3 既有导航）；Y 刻度 max/⌊max/2⌋/0、X 首/中/末（≤5 日全标）。
- `components/console/TrendAxes.vue`（130 行）：brush 与泳道图共享外置轴（Y 线索合计刻度 + X 日期刻度在绘图区外，指针映射不被轴像素挤歪；两图视觉/口径一致）。
- 纯 helper：`console/laneTrendSeries.ts`（series 类型/分段/刻度/坐标纯函数）、`console/rangeBrush.ts`（刷选手势纯状态机 + 指针映射）、`console/brushCurve.ts`（bounds+series → brush 视图模型）。

**G3/G5 接线示例**：

```ts
// G5 壳层（BoardConsoleShell，撤旧窗口 <select> 后）：
<ObservationRangeControls
  :bounds="console.poolBounds.value"          // G1 useObservationWindow 透出
  :range="console.observation.value"           // 受控回流（preview/commit 都经它生效）
  :source-days="console.sourceDays.value"
  :series="poolSeries"                          // 由 poolWindow/sourceWindow(365) 派生日合计 series；未取到传 null
  :loading="console.poolLoading.value"          // 365 档时 A 响应即池，loading 用 A 的
  :notice="console.rangeNotice.value ?? console.daysNotice.value"
  @preview="r => console.applyObservationRange(r.start, r.end, { writeQuery: false })"
  @commit="r => console.applyObservationRange(r.start, r.end)"
  @source-change="d => console.setSourceDays(d)"  // 明确切档（范围重置为档默认，G1 onTierSwitch）
/>
// G3 演进带（BoardLaneEvolution，可见日 >31 时）：
<LaneTrendChart
  :series="visibleAxis.map(date => ({ date, value: dayTotalOf(date) }))"  // 上层派生，null=未知
  :selected-date="selectedDate" label="所选泳道 · 所示日期内日合计（非矩阵桶峰值）"
  @select-date="d => (selectedDate = d)"
/>
```

- **四档按钮归属**：已内聚在 ObservationRangeControls 头部（`source-change` 唯一出口）。G5 收壳时移除 BoardConsoleShell 旧窗口档控件（`MATRIX_WINDOW_OPTIONS` select/档钮），G3/G5 不再画第二组；`MATRIX_WINDOW_OPTIONS` 常量本体留给 G5 处置。
- **series 派生口径**（G5）：365 档 → `sourceWindow` 自身；其他档跨窗历史 → `poolWindow`；逐日值 = 该日全部 A 名单同 ID 桶线索合计（`dayCellMetrics(day).total` 口径，A/B 双索引经 `resolveLaneDay`），未知日（池未就绪/断档）给 `null` 不补 0。组件不自行请求；要完整背景曲线可早调 `ensurePool()`。
- 池曲线 series 的 X 轴与刷选共用 `bounds` 全轴（含两端 index 0..364），与 anchor/today 无关（不 Date.now 推业务期）。

**本批未做**：G3/G5 集成（产品仍未挂载两组件）、.vue typecheck/build（留 G5/8B.4 集中）、真实鼠标/触摸/双主题/布局验收（8B.4）；未跑浏览器与全量命令。

**验证**（2026-10-27 实跑）：`pnpm test:unit`（--maxWorkers=2，参数不带 `--`）ObservationRangeControls 25 + LaneTrendChart 12 + laneTrendSeries 10 + rangeBrush 10 = 57 新用例全绿；G1 回归 observationRange 16 + useBoardConsole.observation 19 照跑绿（G1 源码零改动）；`pnpm exec eslint` 本组 7 个 .ts 文件 exit 0（**.vue 不在 eslint 配置覆盖内，未称 lint 通过**；模板由 vitest vue 编译链验证可编译渲染）。

## 11. G3 矩阵+演进交接（2026-10-28 实现批回填；不改上文批准语义）

**已交付**（全部为本组新文件，零改动 G1/G2/G4/G5/后端辖区；旧 `BoardMatrixView`/`BoardLaneEvolution` 保留未动，G5 收口时替换挂载）：

- `console/observationLanes.ts`（229 行）：G3 纯模型，**source 统计与观察日投影两半分开**。source 半：`buildObservationRows(source, sourceAxis)` → `ObservationLaneRow`（topicId 字符串、clueTotal/sectionTotal 按 A 档全轴、dormant=分区 ≤2 含等号；行序 clueTotal 降序→sectionTotal 降序→topicId 数值升序，`compareTopicId` 数字串按数值比）+ `splitObservationRows`；名单/行序/休眠唯一基准，与观察轴/365 池无关，`source.candidates` 不被触碰。投影半：`projectMatrixCell`/`matrixCellView`（三指标 aria「桶峰值 X 条 · N 个主题桶 · 当日合计 M 条」；content 峰值缺省回退、再缺 '?' 仍可开；gap '—' 禁用；unavailable '?' 禁用）、`projectEvolutionNodes`（total=日合计、`knownDayTotal` 全未知为 null 不补 0、`firstThreadTitle` 首线索标题、showLabel ≥3）、`evolutionDotSize`（min(24, 6+√total×5)）、`evolutionSeriesValue`（content=已知合计、gap=已知 0、unavailable=null）。
- `console/ObservationMatrix.vue`（298 行）+ `console/ObservationMatrixGrid.vue`（352 行）：矩阵父壳（头部说明/档基准 footnote/数据截止/「查看所选日 ↓」gotoSelected + loading 骨架/首错重试/stale 提示条/无日报/无泳道五态分层）+ 网格子块（行×日格、休眠折叠、日格三态、容器内横滚 >31 列、标签列 sticky、无行尾合计列）。
- `console/ObservationLaneEvolution.vue`（241 行）+ `LaneStancePanel.vue`（98 行）+ `LaneEvolutionNodes.vue`（182 行）+ `LaneEvolutionDayIndex.vue`（129 行）：演进组合层（状态分层/头部口径/today 合计）+ 态势面板（`<details>` 默认折叠「最近结算态势 · asOf（非本视窗总结）」、正文快照逐字、旧 detail 可展开全文、snapshot=null 待结算占位）+ 短窗 ≤31 逐日节点带（点径/数字=日合计、≥3 首线索标签、gap 空心禁用、unavailable '?'）+ 长窗 >31 G2 LaneTrendChart + 折叠逐日索引。**G2 图对已知值日（含 gap 已知 0）发 select-date，演进组合层守 content 门槛**（gap 点击只提示不 emit）。

**公共 props/emits**（G5 接线面；两个父组件均为受控组件，无自有网络/日期/选中状态）：

- `ObservationMatrix`：props `axis: string[]`（可见观察轴=`console.visibleAxis`）、`activeRows/dormantRows: ObservationLaneRow[]`（G5 用 `buildObservationRows(console.sourceWindow.value, console.axis.value)` + `splitObservationRows` 派生）、`selectedLaneId: string|null`、`selectedDate: string|null`（与 G4 编年共用一份，G5 统一持有）、`today`、`sourceDays`、`anchorDate: string|null`、`resolveDay: (topicId, date) => LaneDayResolution`（直接传 `console.resolveLaneDay`）、`expandedDormant`、`loading/error/staleNotice/emptyState/dataAsOf`（沿 G1 既有状态）。emits：`selectLane[topicId:string]`（写字符串 ID）、`selectDate[{topicId:string,date:string}]`（点有分区格：G5 更新 selectedDate + 下方编年就地下读，不开 modal 不强滚）、`toggleDormant`、`retry`、`gotoSelected`（「查看所选日 ↓」显式导航，滚动由 G5 决定）、`generate`（无日报空态既有入口意图）。
- `ObservationLaneEvolution`：props `lane: ObservationLaneRow|null`（选中行）、`axis`、`today`、`selectedDate`、`resolveDay`、`loading/error/staleNotice`、`sourceDays`（0=不展示档说明）。emits：`selectDate[date:string]`（仅 content 日）、`retry`。

**DOM 焦点锚**（G5/8B.4 滚动与焦点定位用）：日格/泳道行均为原生 button——`[data-testid="matrix-lane-{topicId}"][data-lane-id]`、`[data-testid="matrix-cell-{topicId}-{date}"][data-lane-id][data-date]`、`[data-testid="evolution-node-{date}"][data-date]`、`evolution-index-day-{date}`；禁用格/节点（gap/unavailable）`disabled` 且 aria 带「无分区/未取到」（断档可读不走 toast）。testid 命名与旧 BoardMatrixView/BoardLaneEvolution 保持同构（matrix-cell-/matrix-lane-/dormant-toggle/lane-evolution-*），G5 换挂载时选择器基本平移。

**G5 接线示例**：

```ts
const rows = computed(() => buildObservationRows(console.sourceWindow.value, console.axis.value))
// 保持响应式：不能在 setup 时一次性解构数组，否则初次请求/切档后名单不会更新。
const rowGroups = computed(() => splitObservationRows(rows.value))
const activeRows = computed(() => rowGroups.value.active)
const dormantRows = computed(() => rowGroups.value.dormant)
```
```html
<ObservationMatrix
  :axis="console.visibleAxis.value" :active-rows="activeRows" :dormant-rows="dormantRows"
  :selected-lane-id="selectedLaneRow?.topicId ?? null" :selected-date="selectedDate"
  :today="console.today.value" :source-days="console.sourceDays.value" :anchor-date="console.anchorDate.value"
  :resolve-day="console.resolveLaneDay" :expanded-dormant="console.expandedDormant.value"
  :loading="console.loading.value" :error="console.error.value" :stale-notice="console.staleNotice.value"
  :empty-state="console.emptyState.value" :data-as-of="console.anchorDate.value"
  @select-lane="id => selectLaneByString(id)" @select-date="p => { selectLaneByString(p.topicId); selectedDate = p.date }"
  @toggle-dormant="console.toggleDormant" @retry="console.load" @goto-selected="scrollToChronicle" @generate="…既有入口…"
/>
<ObservationLaneEvolution
  :lane="selectedLaneRow" :axis="console.visibleAxis.value" :today="console.today.value"
  :selected-date="selectedDate" :resolve-day="console.resolveLaneDay" :source-days="console.sourceDays.value"
  :loading="console.loading.value" :error="console.error.value" :stale-notice="console.staleNotice.value"
  @select-date="d => (selectedDate = d)" @retry="console.load"
/>
```
（`selectedLaneRow`：G1 `selectedLane` 是 legacy 数字 ID 行；G5 按 `rows.find(r => r.topicId === String(console.selectedLaneId.value))` 取 G3 行，或换用字符串选中态后统一。）

**本批未做**：G5 壳层挂载（产品页面仍挂旧 BoardMatrixView/BoardLaneEvolution，行为不变）、.vue typecheck/build（留 G5/8B.4）、真实布局/双主题/触摸验收（8B.4）、窄屏（BoardNarrowSummary 由 G5 接）；矩阵/演进不渲染第二组档位控件（唯一出口仍是 G2 ObservationRangeControls 的 `source-change`）。

**验证**（2026-10-28 实跑，父复核后拆分复跑）：`pnpm test:unit`（--maxWorkers=2，参数不带 `--`）observationLanes 12 + ObservationMatrix 17 + ObservationLaneEvolution 11 = 40 用例全绿（拆分前后同 40 项义务零删改；期间父线程独立复跑同三文件 3 files/40 tests 全绿）；`pnpm exec eslint` 本组 4 个 .ts 文件 exit 0 无警告（**.vue 不在 eslint 配置覆盖内，未称 lint 通过**；模板由 vitest vue 编译链验证可编译渲染）。拆分后各 .vue 均 ≤352 行/≤9979B（规范上限 ~500 行/~15KB）。

## 12. G4 当日编年交接（2026-10-28 实现批回填；不改上文批准语义）

**已交付**（全部为本组新文件，零改动 G1/G2/G3/G5/Shell/右轨/SectionDrawer/旧 reader/后端/原型辖区；SectionDrawer 文件保留未动，退役挂载留 G5）：

- `api/normalizers/laneDayReading.ts` + `api/laneDayReading.ts`：本域 API 边界。既有 dailyReports/articles 协议零改动；客户端归一 camelCase + 字符串 ID（reportId/sectionId/threadId/articleId），thread `fit_distance→fitDistance|null`、`related_article_ids→relatedArticleIds`。join 纯函数 `buildLaneDayJoinedModel`（全部命中=ready、≥1 桶未命中=degraded + missingSectionCount/foldedTotal）/`buildLaneDayFallbackModel`（no-report/error 仍给 base 降级视图，不借别日）。**报告索引 lookback 覆盖算法 `reportIndexDaysFor(requiredStart, today) = max(365, 含两端日历日跨度)`**：ListReports 锚今天、lane 池锚 anchor（已实读 daily_report_repository.go:427-440 核实），停更板块 days 随 anchor/today 差额扩大（例 anchor 2026-10-24 / today 2026-10-28 → 369），池首报告不漏；不新增 from-to、不套四档校验。共享 API 缺口：`/articles/:id` 响应无来源名字段（仅 feed_id），相关文章元信息只展示标题/时间，未造来源名。
- `composables/laneDayReadingCache.ts`：有界缓存 helper（简单 Map FIFO、同键在途合乘、失败/undefined 不入成功缓存不逐出成功项、force 绕过缓存、重写刷新插入位）。
- `composables/useLaneDayReading.ts`：编年读取状态。缓存契约：报告列表索引仅当前板块 1 份（days 需求扩大后整体替换、已有更大覆盖不重复取）、报告详情 ≤14、文章 ≤20；epoch seq 守卫（切板块/切日/卸载晚到丢弃，切板块经 resetForBoard 全清）；跨上下文先清显示再加载（新日期标题下不挂旧日摘要），仅同上下文刷新失败保留旧数据标 staleNotice；分区不匹配按桶降级 base 标题+折叠数、force 重读绕过索引与详情缓存；相关文章按需（`relatedEntries` 独立于缓存逐出、单项重试、`getArticlePayload` 给 G5 复用完整 payload）；切板块 watch 失效 + onScopeDispose 卸载释放。
- `console/LaneChronicle.vue`（386 行/14.2KB）+ `LaneChronicleHeader.vue`（caption 三指标/前后有内容日导航/打开日报/看归档）+ `LaneChronicleThread.vue`（标题默认收束、单条互斥展开真实 summary 逐字、置信/关联篇数在展开后次级区）+ `LaneRelatedArticles.vue`（按需展开、总数始终真实、单项失败局部重试、文章主点击 emit；ids 空给空文案）。

**公开接口（G5 接线面）**：`LaneChronicle` 为受控组件，props `boardId:number / lane:ObservationLaneRow|null / selectedDate:string|null / axis:string[] / today:string / anchorDate:string|null / resolveDay:LaneDayResolver`；emits `selectDate[date]`（前后有内容日，G5 更新 selectedDate）、`openReport[{date, reportId:string|null}]`（G5 走 `/tags/:boardId/:date` 既有路由）、`viewArchive[date]`（G5 切归档视图带日期）、`openArticle[articleId:string]`（G5 复用站内 ArticlePreviewModal/原文阅读；如需完整 payload 调 reading.getArticlePayload）。组件内部自建 useLaneDayReading 实例（boardId 切换自动失效），G5 无需额外接线；DOM 锚 `lane-chronicle / lane-chronicle-{prev,next,metrics,open-report,archive,stale,error,no-report,degraded,retry,refresh,loading,gap,unavailable,footer} / lane-chronicle-section-{sectionId} / lane-chronicle-thread-{threadId}(-toggle-) / lane-chronicle-related-{threadId}(-toggle-) / lane-chronicle-article-{articleId}(-retry-|loading-)`。窄屏 BoardNarrowSummary 挂同一组件即同口径。

**G5 注意**：①selectedDate 与矩阵/演进共用一份（G3 事件 `select-date`）；编年默认选日（CH-1 可见区间最后有分区日）由 G5 持有 selectedDate 时实现，LaneChronicle 自身只跟随。②latest 日报主入口在壳层头部（FB9），编年 caption 的「打开 N 日报」带完整日期是历史日动作，勿当重复删除。③若 G5 需要绝对最新日报元信息（含 0 泳道），用壳层已有列表/`getTodayReports` 口径，不要拿本组索引或 active 行最近日期反推「板块无报告」（本组索引覆盖边界见 reportIndexDaysFor）。

**本批未做**：G5 挂载（产品页面未接，8B.3 不勾）、.vue typecheck/build（留 G5/8B.4 集中）、真实浏览器/双主题/布局验收（8B.4）、reading_behaviors 与打标链路零触碰。

**验证**（2026-10-28 实跑）：`pnpm test:unit`（--maxWorkers=2，参数不带 `--`）normalizer 7 + façade 5 + cache 7 + useLaneDayReading 18（RD-1..5/CA-3..8/停更 lookback/跨上下文/stale）+ LaneChronicle 19（CH-2..10/同名不同 ID/0 线索/错误重试/gap-unavailable 区分/只读零写）= 56 用例 5 文件全绿；`pnpm exec eslint` 本组 9 个 .ts 文件 exit 0 无警告（.vue 不在 eslint 覆盖内，未称 lint 通过；模板由 vitest vue 编译链验证）。各 .vue ≤386 行/≤14.2KB。

**R-24/R-25/R-26 修复批（2026-10-28，父复核退回后定向修；修复记录详见 review.md 对应条目）**：
- R-24 上下文严格 gate：`useLaneDayReading` 上下文增加 `sectionKey`（当日 sections 派生）+ `suspendDisplay()`（离开可读上下文失效在途显示、不动成功缓存）；`LaneChronicle` display gate（lane/date/sections 全一致才进页头/横幅/正文/动作，动作不得旧 reportId 配新日期），四类横幅拆出 `LaneChronicleBanners.vue`；watch source 纳入 sectionKey。
- R-25：列表未命中 period_date 改中性「未匹配到当日日报详情」+刷新，不直称无日报。
- R-26：默认只显 `当日合计 N 条`，桶峰值/桶数/口径进 `<details>` 计数口径（键盘可展开、默认折叠）；hasUnknown 全未知标 `?`、部分未知标 `≥已知部分`，不补 0。
- 修复后验证：`pnpm test:unit app/features/tags/composables/useLaneDayReading.test.ts app/features/tags/components/console/LaneChronicle.test.ts --maxWorkers=2` → 45/45 绿（含 5+2 条 R24 负向/隔离用例、R26 折叠展开/未知标注用例、R25 文案用例）；eslint 同批 3 个 TS 文件 exit 0。拆分后 LaneChronicle.vue 386 行/14.4KB、LaneChronicleBanners.vue 83 行、LaneChronicleHeader.vue 220 行/5.9KB。

## 13. G5 壳层/右轨/手机收口交接（2026-10-28 实现批回填；不改上文批准语义）

**已交付（G5 新增/收口；`BoardConsoleShell.vue` 由单一巨型壳拆为职责文件，均 ≤500 行/≤15KB）**：

- `console/BoardConsoleShell.vue`（493 行/15,045B）：编排层。唯一 `useBoardConsole` 状态源 + `useConsoleSelection`（选中日/名单投影）+ `useConsoleShellData`（版块/日报锚/watch）+ `useConsoleReturnContext`（返回上下文）+ `useConsoleRail`（健康/构成/来源右轨）+ 壳级弹层/刷新编排（不写全局 html 根字号，角色字号由组件局部/token 承担）。
- `console/BoardConsoleBody.vue`（267 行）：三区骨架（`BoardConsoleHeader` / `BoardRail` / `BoardConsoleStage` / `BoardContextRail`），宽屏填满 workspace、1440 压缩右轨、768–1199 收右轨、<768 单栏；`boardMissing` 空态也在此。
- `console/BoardConsoleHeader.vue`（467 行）：面包屑 + 视图 seg（矩阵/追踪｜文章归档｜研究工作台）+ 唯一最新日报主按钮 + 「板块管理」菜单（设置/构成治理/话题管理/研究工作台）+ 新建泳道 + 主题；无旧窗口 select、无「更多」菜单、无全局六项。
- `console/BoardConsoleStage.vue`（236 行）：受控主舞台（归档 / 空态提示 / G2 视窗控件 / G3 矩阵+演进 / G4 编年 / 窄屏摘要），`查看所选日` 与手机「下方编年」本地平滑滚动，不另建日期状态。
- `console/BoardNarrowSummary.vue`（389 行）：窄屏全部可选泳道（活跃 + 休眠折叠）、分区数迷你条（未知虚线不补 0）、选中态、下方编年入口、本板块上下文入口。
- `console/BoardContextRail.vue`（361 行）/ `RailHealthBrief.vue`（352 行）/ `RailSourcesBrief.vue`（302 行）：右轨顺序 健康→构成候选→话题追踪→来源；健康 = 最新 `llm_score`/体检时间/评语首句（两行封顶）+「最新快照 · 非所选历史评分」+ 完整依据；stale 保留旧快照；构成候选 top3 真实名称/清单数；watch active/paused + 唯一 boardId 管理；来源独立 7/30/90 窗 + 贡献条 +「独立统计，不随时间视窗」。
- `console/BoardWideWorkface.vue`（265 行）：单级 1040px（AppDialog xl）工作面；`context` 右轨同源全量 + `health` 完整评语/计数/趋势 + `quality/composites/candidates/sources/settings` 复用既有真实面板与删除/收编/移除处理函数，不套第二层 32rem 抽屉。
- 新辅助：`useConsoleShellData.ts`（186 行）、`useConsoleSelection.ts`、`useConsoleReturnContext.ts`（370 行）、`useConsoleRail.ts`、`useConsoleRefresh.ts`（32 行）、`useConsoleWorkface.ts`（41 行）、`restoreScroll.ts`（89 行）、`RangeHandleHits.vue`（52 行）、`usePlotWidth.ts`（45 行）、`useReadingSettledSignal.ts`（47 行）、`rangeBrush.ts`（166 行）、`consoleStageTypes.ts`、`api/normalizers/consoleRail.ts`（health/source snake_case → camelCase + 字符串 ID，供新右轨/工作面；旧管理面板仍用 raw DTO，只在壳层桥接）。
- 旧文件保留：`BoardGovernanceRail.vue`/`BoardGovernanceDrawer.vue`/`SectionDrawer.vue`/旧 `BoardMatrixView.vue`/`BoardLaneEvolution.vue` 及其测试未删；产品挂载已切到新路径（SectionDrawer 退出日格路径）。
- `BoardRail.vue` 新增 `allowAddBoard`（默认 false）：新建版块唯一入口回总览 `BoardRadarGovernance`；`BoardConsoleShell` 不再传。

**真实入口/作用域/旧功能新家**：

| 旧入口/能力 | 新位置 | 作用域/说明 |
| --- | --- | --- |
| 顶部窗口 `<select>` 7/14/30/全部 | `ObservationRangeControls`（头部唯一档按钮 + 刷选/输入/平移缩放） | 仅独立档按钮或 URL `days` 改名单；自由范围恰 7/14/30/365 不切档 |
| 最新日报（原回退 `latestDataDate`） | 顶部「打开日报 · MM/DD」唯一主入口 | 版块独立 report 元信息（`/semantic-boards/overview` 的 `report_date`；失败退回 `days=365` 列表）；0 active 泳道也有；列表空/今天无报告不推断「从无历史」 |
| 矩阵日格点击 → SectionDrawer | 点格选中日 + 下方 `LaneChronicle` 就地下读 | 不开抽屉/不强滚；日格仍是桶峰值，编年全量 thread 经日报详情稳定 ID join |
| 演进带（最密集三日/今日事件） | `ObservationLaneEvolution`（点径=日合计、态势折叠 asOf、>31 日趋势图） | 最密集三日/今日事件块退役 |
| 右轨旧 `BoardGovernanceRail` | `BoardContextRail` + `BoardWideWorkface` | 健康→构成候选→追踪→来源；全局六项/新建版块不在本壳 |
| 右轨「治理抽屉」32rem | `BoardWideWorkface`（AppDialog xl 1040） | 支持窄屏进入同一内容面；不因父级 media 隐藏 |
| 追踪管理 | `WatchManagePanel`（不变） | 参数仅 boardId；watch 的 `persistentTopicId` 仅用于定位当前泳道 |
| 全局治理六项/新建版块 | 总览 `BoardRadarGovernance`（不变） | 版块页反向断言菜单/按钮不存在 |
| 研究工作台 | 顶部 seg 第三入口 + 板块管理菜单 → `/tags/:boardId/research`（`BoardEnrichmentPanel`） | 真实路由；不是旧匈牙利关系图；研究页不动 |
| 删除版块 | `BoardWideWorkface` settings → 既有 `BoardSettingsForm` 删除确认 | 删除版块 ≠ 归档话题；不新写删除算法 |
| 归档 | 顶部 seg「文章归档」→ 既有 `BoardArchiveView` | 保留初始日期；切回矩阵不重取泳道 |
| 文章阅读 | `LaneChronicle` 文章主点击 → 既有 `ArticlePreviewModal`（`getArticle`） | 不加导语预览层；关闭恢复触发焦点；不标已读、不动 reading_behaviors |
| 返回上下文 | `consoleReturnContext`（sessionStorage，一次） | 阅读台 `focus=report&lane=` 与研究工作台往返恢复 泳道/日期/days/start/end/滚动；URL 显式项优先；不携带 report/annotation |

**真实字段来源（G5 消费）**：

| 展示 | 来源 | 边界 |
| --- | --- | --- |
| 最新日报日期/标题 | `GET /semantic-boards/overview` → `RadarBoard.reportDate/reportTitle`（normalizer 已 camelCase） | 0 active 泳道仍有；overview 失败才退回 `GET /semantic-boards/:id/daily-reports?days=365`；两源都失败给「元信息未取到，不代表没有历史日报」 |
| 名单/行序/休眠/候选 | A 档 `lane-dynamics` 响应（`sourceWindow`） | G3 `buildConsoleRows`；B 365 池不参与 |
| 日格/演进/编年 | G3 `resolveLaneDay` 三态 + G4 `useLaneDayReading` | 未知 `?`/断档 `—`/未取到不补 0；0 线索合法 |
| 编年标题/摘要/相关文章 | `GET /daily-reports/:id` 全量 threads + `GET /articles/:id` | 稳定 section/thread ID；文章列表不取正文大字段 |
| 编年历史日报 | `/tags/:boardId/:date` 既有阅读路由 | `LaneChronicleHeader` 「打开所选日日报」保留完整日期 |
| 健康 | `GET /semantic-boards/:id/health-checkup` → `latest.llm_score/checked_at/llm_comment` | `normalizeRailHealth` → camelCase；stale 保留旧快照；无快照不伪 0 |
| 构成候选 | health `uncaptured_candidates` | 辅助标签实体；高频共现 ≠ 必须收编；完整列表仍在 workface candidates |
| 话题候选 | `lane-dynamics.candidates`（`sourceWindow.candidates`） | 仅人工激活资格；入口为现有编排/话题管理 |
| 话题追踪 | `GET /semantic-boards/:id/topic-watches` | 现有 TopicWatch camelCase DTO；管理 action 无 lane/topic 参数 |
| 来源 | `GET /semantic-boards/:id/source-breakdown?window=7|30|90`（`useBoardSources`） | 只读复用 sourcestats 口径；`normalizeRailSourceBreakdown` 只做 camelCase/字符串 ID；独立统计不随时间视窗 |
| 归档 | `GET /semantic-boards/:id/articles`（既有 `BoardArchiveView`） | 列表不取正文大字段 |

**验证（2026-10-28 实跑；只跑受影响文件，`--maxWorkers=2`，参数不带 `--`）**：

- **R27–R33 定向修复批（2026-10-28）**：9 文件 / **120 tests 全绿**（8 文件批 119：`BoardConsoleShell` 35 + `BoardContextRail` 16 + `BoardWideWorkface` 3 + `BoardNarrowSummary` 4 + `useBoardConsole.observation` 21 + `useBoardConsole` 22 + `BoardSettingsForm` 6 + `BoardArchiveView` 12；另 `DailyReportGenerateDialog` 1）。焦点 watcher 补回后再跑 shell/rail/workface **3 文件 / 54 tests 全绿**。
- 前次 G1–G4 回归网 24 文件 / 308 tests 仍保留未改为本批验收；本批只跑实际受影响目标，未重跑全量/全 308。旧 4 文件 42 tests / 旧管理面板 3 文件 26 tests 的历史结果未因本批改变。
- `pnpm exec eslint` 本轮受影响 **TS** 文件 exit 0（含 `useConsoleRefresh.ts`/`useConsoleReturnContext.ts`/新增测试）；`.vue` 不在仓库 eslint 配置覆盖内，未称 lint 通过。
- `pnpm exec nuxi typecheck`（末尾集中一次，未与测试/构建并行）：本组 G1–G5 错误清零；仅余 **7 条他域旧债 `SignalReportView.test.ts`** `possibly undefined`（按 brief 不擅改），退出码仍为 1。
- 类型修正披露：为满足本组 typecheck，G2/G3/G4 仅有类型层面最小修正（`LaneChronicle` null 收窄、`LaneTrendChart`/`brushCurve` noUncheckedIndexedAccess、`ObservationRangeControls` 手势局部变量、`observationLanes` bucket 字面量类型、四个测试的必要非空断言）；不改行为/口径。

**R27–R33 定向修复（2026-10-28，待父复核）**：

- R27：删除 `BoardConsoleShell` 全局 `html.fontSize` 写入/恢复及生命周期导入；角色字号保留局部 rem/token，不做整页倍乘。负向用例挂载/卸载 html 字号保持 `17px`；§13 不再声称该公式来自 ui-design。
- R28：`useConsoleShellData` 元信息刷新保留同板块最后成功值（失败 `reportMetaStale`，从未成功才 unavailable）；`useConsoleRefresh` 串行「元信息→A」；`DailyReportGenerateDialog` 真实 `done` 上升沿 emit `completed`；completed/cancel/顶栏刷新同一链。用例覆盖新日期头部+源 axis、0 active 新头部、关闭刷新、失败保留旧值→重试恢复。
- R29：Stage 接 `needsPool/poolError/poolStaleNotice/retryPool` 独立提示与重试；`ensurePool` 在途防重、成功才清错、失败保留旧池标陈旧；重试期间提示保留/按钮禁用。用例断言只重发 365、A/14 日档/选中泳道不变、成功恢复历史格。
- R30：返回恢复改成无写回填 observation/lane → 合成单次完整 query（days/start/end/lane，删 focus）并 await router.replace；`selectLane` 增加 `writeQuery:false`。异步 fake router 用例只提交 1 次且四键齐全；显式/非法 URL 优先用例保留。
- R31：按 `scrollHeight > clientHeight` 选唯一真实滚动宿主（stage 或 window），不再 `stage.scrollTop ?? window.scrollY`；焦点统一 `focus({ preventScroll: true })`。用例：windowY=120 存/还、桌面 stageY=80 存；1280/390 实页留 S9。
- R32：仅精准改设置/归档两处旧指引为「今日雷达（总览）的版块治理 / 当前板块板块管理 → 构成与治理」，补文案/反旧词断言，不重绘页面。
- R33：右轨「话题候选 / 人工激活」改 `openTopics`，经 Body/Workface 接既有本板块 `TopicManageDialog`（boardId），不再开新建泳道编排；顶栏新建泳道原入口保留。用例只验证对话框落点/board scope，不真激活。

**R34–R39 定向修复（2026-10-28，待父复核/实页）**：

- R34：新增 `console/restoreScroll.ts`（有限帧、默认 180 帧）并在 `useConsoleReturnContext` 接入；按 stage/window 唯一宿主应用，内容增高后重试到目标；用户 wheel/touch/pointer/key、上下文变化、切板块、scope dispose 均取消，焦点 `preventScroll`，无固定 sleep/空白占位。负向用例：clamp 假宿主 302→382 恢复、取消、上下文变化、内容终短耗尽；shell R31 window 恢复保留。
- R35：`useObservationWindow.onTierSwitch` 增加 `writeQuery/restoreQuery`；用户按钮/返回恢复标记 `suppressQueryRestore`，忽略 router 尚未提交的旧 start/end；外部 URL `days` 走 `fromUrl`（保留显式范围）。用例：异步 router + 365/14 缓存立即命中，sourceDays/默认 observation/visibleAxis/UI range/URL 一致；外部显式 start/end 不被删；OX-1/R22/R30 回归保留。
- R36：`ObservationMatrixGrid` 以 `axis.length>31` 分 `is-scrollable`；≤31 单屏 `minmax(0,1fr)` + 两行 `MM/DD`（title/aria 完整日期），>31 才启用内部横滚/sticky 短日期；未改 spec/test-cases。用例 30/31/32/365 分支。
- R37：`ObservationRangeControls` 新增 24px 透明 `brush-hit-start/end` overlay，起上/止下各半、短窗/单日不互盖；`handleHitStyle` 用 min/max 夹端点，坐标仍走真实 plot rect；SVG 键盘/aria 与 pointer 取消保留。用例 24px/上下分区/端点/overlay 拖动映射；真实 elementFromPoint/手指留 K3。
- R38：`AppDialog` 新增默认关闭的 `semanticDialog/dialogLabel`（role/aria-modal/label/tabindex + 初始 focus preventScroll）；`ArticlePreviewModal` 新增 `dialogSemantics` 并透传，仅 matrix Shell 启用；90vw/搜索高亮/MutationObserver/iframe 高度/阅读行为未改，关闭焦点仍由 Shell。测试覆盖默认/opt-in 与转发，原高亮/高度测试照跑。
- R39：`LaneChronicleHeader` kicker 拆分系统 `.lch-kicker-system`（保留 uppercase）与业务 `.lch-lane-label`（原大小写/ellipsis）；用例断言 `Agent 架构重构…` 原样。真实 computed text-transform 留 K3。

新增共享接口：`AppDialog` 的 `semanticDialog?: boolean`、`dialogLabel?: string`；`ArticlePreviewModal` 的 `dialogSemantics?: boolean`；均默认 false，只有 matrix reader 启用。

**本批验证（2026-10-28；实际受影响 9 文件 / 174 tests）**：`restoreScroll` 4 + `useBoardConsole.observation` 23 + `BoardConsoleShell` 36 + `ObservationMatrix` 18 + `rangeBrush` 12 + `ObservationRangeControls` 27 + `AppDialog` 15 + `ArticlePreviewModal` 13 + `LaneChronicle` 26 = **174 tests 全绿**。`pnpm exec eslint` 上述 TS 文件 exit 0；`pnpm exec nuxi typecheck` 仅余 SignalReportView.test.ts 7 条旧债（exit 1 如实）。未 build/浏览器/dev/真实业务 GET/SQL 写/生成/部署/提交/归档；旧 308 未重跑；真实 1440/1280/390 滚动、≤31 几何、elementFromPoint/手指、computed uppercase 留父构建后 K3。8B.3/8B.4 未勾。

**R34/R35/R37/R40 第二轮返修（2026-10-28，待父复核/实页）**：

- R34：180 帧不再承担“内容就绪”。`LaneChronicle` 在 display 就绪/非 loading 或 gap/unavailable 终态 emit `readingSettled{laneKey,date,status}`（`useReadingSettledSignal.ts`），经 Stage/Body 到 Shell 的 `readingSettled` ref，接入 `useConsoleReturnContext`。pending 期间不排布局帧、不耗 attempts；同 lane/date 信号到达后才开始有限恢复；自然完成/耗尽 `onDone` 摘 window 监听，用户意图/上下文/切板块/卸载取消。shell 负向：延迟详情期间 250 帧机会内无恢复帧；详情就绪后 302 clamp、增高 382 再恢复；R31 stage/window 唯一宿主保留。
- R35：shell 负向改为手动 deferred 导航：旧 `days=30/start/end` 未提交、缓存续体已跑时先断言 UI/轴已用 365 默认且 URL 仍旧；释放后 URL 变 `days=365` 无 start/end，UI/轴一致。自由范围/OX-1/R22/R30 与外部显式 range 保留。
- R37/R40：`RangeHandleHits.vue` 按真实像素生成命中区：≥24px 两独立区；<24px 合并为 24+间距区，按最近可见端解析；单日同 x 按上/下区分；`BrushGesture.grabOffset` 保持首动偏移。`usePlotWidth.ts` 读真实 SVG 宽；父层用真实 plot rect 解析端点。用例覆盖 375 等效 7/14 中点、外缘 6px+1px 首动、单日上/下、两池边界、非重叠拖动映射，未缩小 24px 合同。
- 尺寸（行/字节）：Shell 493/15,045；Header 467/12,565；Body 267/7,312；Stage 236/7,293；useConsoleReturnContext 370/12,926；restoreScroll 89/2,777；rangeBrush 172/6,433；RangeHandleHits 52/1,358；usePlotWidth 45/1,318；useReadingSettledSignal 47/1,707；ObservationRangeControls 454/14,812；ObservationMatrixGrid 384/11,073；LaneChronicle 399/14,947；LaneChronicleHeader 241/6,433；AppDialog 208/5,390；ArticlePreviewModal 226/8,211。均 ≤500 行/15KB。

**本批第二轮验证（2026-10-28；实际受影响 9 文件 / 181 tests）**：`restoreScroll` 5 + `useBoardConsole.observation` 23 + `BoardConsoleShell` 37 + `ObservationMatrix` 18 + `rangeBrush` 14 + `ObservationRangeControls` 29 + `AppDialog` 15 + `ArticlePreviewModal` 13 + `LaneChronicle` 27 = **181 tests 全绿**。`pnpm exec eslint` 上述 TS 文件 exit 0（`.vue` 不在 eslint 覆盖内）；`pnpm exec nuxi typecheck` 仅余 `SignalReportView.test.ts` 7 条他域旧债（exit 1 如实）。未 build/浏览器/dev/真实业务 GET/SQL 写/生成/部署/提交/归档；未重跑旧 308/全量；真实 1440/1280/390 滚动、≤31 几何、elementFromPoint/手指、computed uppercase 留父构建后 K3。

**第三轮生产接线补齐（2026-10-28，待父复核/实页）**：R34 pending 建立即挂用户意图监听（布局复用、stopRestore 完整摘除）；shell 负向覆盖 pending+wheel 取消后 resolve 不强拉，无意图对照仍 302→382。R37/R40 `usePlotWidth` 用 `watch(plotRef,{flush:'post'})` 处理 SVG 晚到/消失/替换并重连 RO；Controls 负向从 bounds/range=null 晚到渲染真实 hit 层，`usePlotWidth.test` 覆盖出现/消失。本轮实际跑 `BoardConsoleShell` 38 + `ObservationRangeControls` 30 + `usePlotWidth` 1 = **3 files / 69 tests 全绿**；未重跑 9 files/181/旧 308。`pnpm exec eslint` 本轮 TS exit 0；`nuxi typecheck` 仅余 SignalReportView 7 条旧债（exit 1）。

**run2 N1/N2/N3 收口（2026-10-28，待父定向实页）**：

- N1：`buildHitRegions` 先算端点夹取后的实际 24px 区间；只要实际相交（不再用原始 gap<24 假定）就合并为一个 overlap 区，交 nearest 解析，DOM 不再由 end 抢占。rangeBrush 15 tests 覆盖左夹/右夹重叠带与旧短窗/单日/边界/非重叠回归。
- N2：375 极短窗 1px≈1.15 日的量化与触摸无法精确选单线如实记入 ui-navigation，不硬改映射、不造 UI。
- N3：loaded `thread.summary=''` 展开「暂无摘要记录」组件断言；未取到报告不冒该空态；test-cases 变体 7 措辞同步，不改业务契约。
- 第24步容量补证：`useLaneDayReading`（20）+ `laneDayReadingCache`（7）+ `rangeBrush`（15）+ `LaneChronicle`（28）= **4 files / 70 tests passed exit 0**，记录 `run2/cache-layer-supplement.md`；明确为 mock/组件层证据，不伪称浏览器做过 14/20 大样本，第24步仍 PARTIAL。
- 文档：`flow/semantic-board.md` 更新 L1/观察窗/矩阵/演进/编年/右轨/缓存当前入口，`architecture/ui-navigation.md` 补研究工作台真实路由/健康来源/N2 触控限制；未改红线/生命周期/旧快照/原型/总览/其它 change。
- 门禁：`doc-impact.sh verify ... --base HEAD` 通过；`check-standards.sh --change rework-workshop-into-radar-and-desk` **250 通过 / 0 失败 exit 0**。

**未验/风险**：

- 未 build、未起 dev server、未做浏览器/真机触摸/双主题/双视口实页验收；1440/1920/2560/375/390 的最终机械断言留父/8B.4。
- 未连真实后端/数据库；所有数据接线以 mock 组件测试证明（无 POST/生成/回填/收编/激活/删除执行）。
- 已知外域 `SignalReportView.test.ts` 7 条类型债未洗绿；旧根 53 pass/1 fail/2 未覆盖与关系/归档/老投诉欠账不因本批改变。
- `.vue` 无 eslint 覆盖；模板由 vitest vue 编译链验证可编译渲染。8B.3 待父综合验收后勾，8B.4 需真实构建/浏览器证据才勾，本线程未勾。
