## Context

> **2026-10-30 新建泳道修订优先（原型待审）**：本域以 [最新阶段合同](ui-design-lane-compose-draft.md) 和 [workshop-lane-compose delta](specs/workshop-lane-compose/spec.md) 为准。左候选按同ID原关系连通片段、首末日期差至少3天筛选（非连续3天）；点击整组基础成员，不自动补相似、不截20条。7/14/30/90天＋自选起止共窗；补充推荐取当前有效成员均值、原余弦阈值，非active优先、active只读次级。原图点选仅查看、全边保留、人工补漏无算法门禁；旧“连续多天启用/top6邻接即推荐/默认质量卡”的编排提案不再是本入口合同。管理可见与启用仍服务端累计门槛，不为与左候选同列表而放宽；旧质心与快照不变。只更新制品，不作产品实施批准，也不影响其他域已批布局。2026-10-31 追加「勾选与相近度」修订（仍仅原型）：组与每个成员改用原生复选框并支持半选、取消整组、清空已选、取消本次，取消不吞组外补选与手写名；已有active泳道默认不画（30天默认38点/9条两端可见原边），需上下文时显式开启只读参照，取消聚焦/查看已有归属不再自动打开；待选推荐按真实余弦距离映射点大小与填色深浅，已选另用实心＋✓独立标识。候选跨3天门槛、日期共窗、成员均值与原阈值0.3一律不变。

现有 `/tags` 是单路由 + 五个 `contentTab` 互斥面板，全部状态（选中版块、tab、泳道、日期）挤在 `useTagsPage` 一个 composable 里；`BoardThreadBrowser.vue`（3395 行）同时承载 DAG 浏览、编排（compose）、合并与回填。可复用的既有资产：`lane-dynamics` 聚合端点（泳道 + 候选 + 逐日 section）、`section-timeline`（sections + relations，编排态依赖）、`health-checkup`、`source-breakdown`、`board articles`（含 filtered_tags 证据）、`BoardDailyReportTimeline` 内部 reader（页边注 / watch 定位 / 深链）。约束：`layout.md` 只有 reader/contained/workspace/split 四模式；ui-impact 为 major，需双视口验收；窄屏 767.98 断点与 375 基线是硬红线。

## Goals / Non-Goals

**Goals**
- 把「一个页面塞五份文档」拆成「雷达 → 版块态势 → 日报阅读台 → 文章抽屉」四级脊，路由承载层级。
- 同一批 section 数据只渲染一次（矩阵），编排态复用同一批数据的另一投影。
- 治理从 tab 变为右轨常驻 + 抽屉，且不阻塞主视图。
- 编排态保留现有 compose 的全部语义（采纳/确认启用/连续天数），只换选料界面。

**Non-Goals**
- 不改匹配算法、相似度阈值、日报生成流水线。
- 不改 `lane-dynamics` / `section-timeline` / `health-checkup` / `source-breakdown` 的聚合口径与响应（只新增 overview 与 articles 字段）。
- 不做挂墙大屏 / 轮播 / kiosk。
- 不在本期做「把漏掉的文章补进日报」的写动作。

## Decisions

### D1. 两级路由承载层级

- **结论**：`/tags`（雷达）与 `/tags/:boardId`（版块态势）+ `/tags/:boardId/:date`（阅读台）三层路由；原 `?board=&report=&annotation=` 深链继续可用（映射到对应路由并保留定位参数）。
- **落点**：`front/app/pages/tags.vue`
- **取舍**：否决「单路由 + 内部层级状态」——层级无法收藏/回退，且首次加载要等 JS 才决定看哪层，雷达的"打开即见"卖点会丢。

### D2. 矩阵复用 lane-dynamics，不新增数据源

- **结论**：矩阵、演进带、section 抽屉全部由既有 `GET /semantic-boards/:id/lane-dynamics?days=N` 响应驱动（lanes[].timeline[].sections 即格子与事件）；休眠分组是前端展示规则（窗口内 ≤2 个 section 的活跃泳道）。
- **落点**：Scenario「默认展示泳道动态区」
- **取舍**：否决新增 matrix 专用端点——同一批数据多一份口径维护成本，且与「态势句随日报结算」的既有语义会分叉。

### D3. 治理右轨与抽屉复用现有面板内部分块

- **结论**：右轨只用 `BoardHealthPanel` 的评分/趋势/候选区与 `BoardSourcePanel` 的 top-N；完整质量表、组合区、候选全量、来源表与版块设置搬进抽屉。`BoardHealthPanel` 拆为「信号区 + 明细区」两段导出，原组件退化为抽屉内容。
- **落点**：`front/app/features/tags/components/BoardHealthPanel.vue`
- **取舍**：否决重写治理面板——体检口径/分档/降级语义刚在 `add-board-health-checkup` 定稿，重写会把口径错误再引一遍。

### D4. 编排态复用 useInlineCompose 的纯逻辑

- **结论**：编排态新增画布与候选侧栏组件，但采纳/确认启用/质量指标（成员、平均距离、离群、预勾上限 20）继续调用 `useInlineCompose`（内部 `composeReport.ts` 的 cosineDistance / aggregatePreview / outlierFlags）；候选顺序与「连续 N 天」判定沿用其现有字段，不另算。
- **落点**：`front/app/features/tags/composables/useInlineCompose.ts`
- **取舍**：否决照抄一份画布专用逻辑——两套距离口径必然漂移，且现有单测已覆盖边界。
- **与旧 compose 的差异（2026-10 对账）**：画布窗口取 **14 天**（spec Scenario，旧工坊 30 天语义让位）；「采纳」先并入该候选 own 的全部 section、再补相似待确认（spec 场景，旧实现只勾相似待确认）。两处差异均有单测锁定。

### D5. 归档的 in_report 由日报分区线索派生

- **结论**：`in_report` = 文章 id 是否出现在查询窗口内任一分区 thread 的 `related_article_ids`；不改表、不加冗余列。端点新增 `report_status` 过滤与 `in_report` 字段，口径实现在 service 层一次，前端只展示。
- **落点**：Scenario「标记未入日报」
- **取舍**：否决给文章表加 `in_report` 列——日报重生成会使其失效，需要回填任务；派生查询配合窗口限制（默认 6 份日报）足够。

### D6. 阅读台抽壳，不改 reader 内部

- **结论**：`BoardDailyReportTimeline` 的阅读层从 `position: fixed` 浮层改为路由页面容器；内部 reader（分区映射、页边注、watch 命中定位、深链闪现、lifeline 缓存）原样保留，仅通过插槽/容器 props 接收四区布局。
- **落点**：`front/app/features/tags/components/BoardDailyReportTimeline.vue`
- **取舍**：否决重写 reader——页边注与深链是长期演进出来的能力，重写等于把 `daily-report-margin-notes` / 深链契约全部重做。

### D7. 雷达用一个聚合端点

- **结论**：新增 `GET /api/semantic-boards/overview`，一次返回各活跃版块的当日/近 3 日/14 天分区数与最新日报摘要；实现复用 lane-dynamics 与日报列表的 service 方法。
- **落点**：8.2
- **取舍**：否决前端并行 9 路请求——首屏抖动会直接毁掉「打开即见」；也否决给版块列表端点加字段——列表是配置面，不该背统计。

### D8. 固定视口栅格作为 layout 例外登记

- **结论**：≥1440px 的雷达/版块态势使用固定视口栅格（无页面级纵向滚动，仅舞台内滚动），在 `layout.md` 的「需要自由宽度」条款下登记为例外；1440px 以下回落 workspace + 页面滚动；768–1199 右轨收进顶栏治理入口。
- **落点**：`docs/reference/standard/frontend/layout.md`
- **取舍**：否决硬套 workspace（页面滚动）——那样「一屏看全」的核心卖点不存在；也否决新造第五种 layout mode——四模式是 review 基线，加档要先改契约本身。

### D9. 窄屏线性化，不做矩阵横滚

- **结论**：<768px 只保留「雷达列表 → 版块摘要（泳道列表 + 每行 14 天迷你条）→ 日报阅读台」；矩阵与编排态在窄屏不渲染。
- **落点**：7.4
- **取舍**：否决矩阵横向滚动——滚到第 3 天就看不见第 10 天，"一眼"价值归零；也否决窄屏做缩略矩阵——375px 下无法同时保证可读与可点。

### D10. 四阶段落地，路由切换是回滚点

- **结论**：阶段①骨架+矩阵（只读）→②阅读台与归档→③治理抽屉与编排态→④雷达+overview+清理旧 tab。每阶段可独立发布；回滚只需把 `/tags` 路由指回旧 `TagsPage`，旧组件保留到阶段④结束再删。
- **落点**：1.1
- **取舍**：否决一次性大爆炸合并——`BoardThreadBrowser` 与 `BoardDailyReportTimeline` 体量大，一次性改动无法定位回归来源。

### D11. 获批矩阵采用观察窗，日期拖选不重排成员（2026-10-27用户裁决）

用户对实施清单的结构化问答明确选择“观察窗：名单稳定”。本阶段沿已批准scope-cleanup原型，复用真实lane-dynamics独立7/14/30/365查询档决定名单/原排序/休眠分组依据；真实365日数据池供观察日期裁切。拖选/输入观察日期只更新当前这批泳道的显示区间，不把起止日自动转换成另一套成员排行请求。

这不是静态原型数据入产品，也不是重建所选时期全部历史话题。独立档、观察区间、当前选中泳道/日分别管理；切板块的请求竞态、锚点/范围元信息、按ID读取摘要等由实施前字段与用例合同落实。必要只读元字段可补，但不为了横轴自由选择新增from/to统计模式、不改排序算法、candidate资格、人工status或历史section快照。“休眠”只是展示分组，不发生状态写入。

2026-10-27父复核明确：sourceDays只由独立档按钮/明确URL days变更；A为名单、source全轴前端clueTotal降序→sectionTotal降序→ID升序、休眠及候选的唯一基准。B365只补A同ID历史，拖至恰7/14/30/365日也不切base。A成功B失败保留A可读/历史未取到不补0、不换名单或强改视窗；A首次失败B成功不冒成功，旧A刷新失败stale可读；source365档池一次请求。缓存仅当前板块四档共用365、列表1份、报告14份/文章20份、Map FIFO，失败不入成功缓存，切板块/卸载释放，当前阅读不因逐出消失或回顶。

同日用户明确选择“保留真实研究入口，关系后议”：矩阵顶栏第三入口定名“研究工作台”，承接`/tags/:boardId/research`的BoardEnrichmentPanel，不改研究页；返回上下文由矩阵保存。不是恢复旧关系图/关键事件，A-56/R-11…R-16旧责任不洗绿。

详细清单与未采用B路线见 `implementation-matrix-checklist.md`；本决策覆盖更早矩阵日期交互的歧义，不改变其他待审页面。实施前置合同（组件映射/状态分离/字段锚/互斥分工）见 `implementation-matrix-stage.md`，阶段用例账本（主链路/继承表/白盒分支）见 `test-cases-matrix-stage.md`（均为 8B.2 文档产出，未改产品代码）。

### D12. 日报日驱动的日期轴 + 当日其他话题（2026-10-28 用户批准 matrix-day-others 增量）

- **结论**：矩阵的日期可选择性改由**板块真实报日索引**决定（列头只对报日可选，与日格/演进节点写同一个 `selectedDate`），不再依赖正式泳道存在；正式行之后加同级独立的「当日其他话题」「休眠泳道」两个全宽折叠组，其他组成员 = 所选日报详情 sections 中按稳定 ID/话题身份排除正式与休眠泳道后的分区，只在所选日标线索数，行仅展示当日原题与当时状态快照，下读复用既有线索标题/逐字摘要/相关文章能力。范围排出所选日时回窗内最近报日，无报日诚实空态，不跳今天或 active 泳道日。
- **落点**：`front/app/features/tags/composables/useLaneDayReading.ts`（报日索引 + 报告优先读取，与编年共缓存/双 epoch）、`front/app/api/normalizers/laneDayReading.ts`（补 `topic_status_at_report`/`persistent_topic_id` 字段归一）、`components/console/dayOtherTopics.ts`（新纯模型）、`components/console/laneConsoleView.ts` + `useConsoleSelection.ts`（选中日三级校正）、`components/console/ObservationMatrix.vue` / `ObservationMatrixGrid.vue` / 新 `DayOtherTopicsGroup.vue`、新 `OtherTopicReading.vue`、`BoardNarrowSummary.vue`、`BoardConsoleShell.vue` / `BoardConsoleStage.vue` / `LaneChronicle.vue`（store 上提注入）。
- **取舍**：否决另建「报告读取器」——同一日报详情/文章缓存必须有边界且只一份（索引 1/板块、报告 14、文章 20，继承 CA 组），否则违背已交付缓存契约；否决用候选池/当前 topic 状态补行——组只读所选日报真实分区，状态只取 `topic_status_at_report` 快照（历史不篡改）；否决新增 `date=` URL 参数或独立日历——保持既有 `days/start/end/lane` 深链与一次性返回上下文，选日仍为显示态；否决给其他话题另写摘要组件——复用 `LaneChronicleThread`/`LaneRelatedArticles`（同一 store）。
- **边界**：报日索引失败只影响列头选择（提示+重试，不阻塞矩阵/编年）；报告详情失败=错误态+重试（其他话题没有聚合标题可降级，不伪造）；0 行与无详情/无日报分列；某泳道当日未取到只追加说明不重复归类；单条/组合/候选/生命周期/生成/回填零改动、零写请求。
- **前置合同**：`implementation-matrix-day-others.md`（字段来源/状态模型/单-多 seam/独占辖区/门禁）；用例账本 `test-cases-matrix-day-others.md`（S11 冷启动+成熟不回归、五组变体、继承与调整、RDY/OTH/ORD/GRP/RDC/NAR 白盒）。本增量不改 reader/结构化 spec 以外的其他页面，不恢复旧被否方案，旧失败/未覆盖账目不洗绿。

## Risks / Trade-offs

- [雷达 9 版块聚合可能慢] → overview 端点内部并发取数并设 5s 超时；超时版块返回零值并标「数据延迟」，不阻塞整屏。
- [4K 下密度过高、字号偏小] → 根字号 `clamp(15px,.86vw,30px)`；1440/1920 双视口验收 + 4K 150% 实测截图。
- [既有测试大量断言五个 tab 结构] → 阶段①同时更新受影响测试；不删测试，改为断言新宿主（矩阵/右轨）。
- [归档 in_report 查询在长窗口下变慢] → 默认窗口 6 份日报，索引走 `daily_reports(period_date)` + `report_sections(report_id)`；超过 30 天窗口时改异步计数。
- [reader 抽壳引入回归] → 保持 reader 内部零改动，用既有页边注/深链测试作为回归门禁。
- [窄屏只剩三屏，功能落差被感知为「缺功能」] → 窄屏在编排入口给出明确提示，并在设置页提供「用宽屏运行」的说明文案。

## Migration Plan

1. 阶段①上线：新增路由与矩阵，`/tags` 仍可由旧页兜底（路由未切换）。
2. 阶段②：阅读台切换为路由页，深链 `?report=&annotation=` 保持可用。
3. 阶段③：治理抽屉与编排态切换；旧 tab 入口隐藏但组件保留。
4. 阶段④：路由默认切到雷达；删除旧 `contentTab` 分支与退役面板；`docs/reference/flow/` 同步。
5. 回滚：任一阶段出问题只回退对应路由映射；数据层无迁移，回滚无需修数据。

## Open Questions

无（窄屏方案、关系图归属、全局参数去向已在提案与 UI 设计中定稿）。
