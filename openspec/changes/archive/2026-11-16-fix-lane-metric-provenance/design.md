# Design：泳道/矩阵数字口径修正（线索合计口径收敛 + 伴生数字对齐 + 计数去歧义）

<!-- complexity: complex -->
<!-- ui-impact: minor -->
<!-- constraint-domains: semantic-board, topic-graph, daily-report -->

## Context

同一次 `lane-dynamics` 响应里并存两套数，用户看到的是互相打架的数字：

```
矩阵格大字：   3
tooltip：     "3 条线索 · 1 个主题桶（最多一天桶 5 条）"   ← 同一句里 3 和 5
点开当日：    5 条线索条目（综合稿无任何标记）
```

根因不是矩阵算错，而是**伴生数字没跟着收敛**：

- `day.clue_count` = 当日线索合计（**排除**综合稿）→ 矩阵格大字 / 演进节点 / 池曲线 / `lane_heat` 输入；
- `sections[].thread_count` = 该桶 thread 总数（**包含**综合稿）→ 被 tooltip 当成「一天桶 N 条」露出来。

矩阵口径本身是对的（`docs/reference/flow/semantic-board.md` 红线 28：section 数、桶数、thread 条数、事件数 MUST NOT 参与任何热度或排序计算）。缺的是两件事：① tooltip 把另一口径的数当「辅助说明」露出来；② 「综合稿不计入」的说明随事件线退役后，界面上再没有任何一行告诉用户矩阵数排除了综合稿。

除这条主线外，本轮溯源同时确认了同一族真缺陷：两个「全部 M」不是同一把尺子量出来的、窗口内外成员集合不对称、「最新日报」两个锚点差一天、距离恰为 0 时字段被 JSON 省略、「接近度」两套算法同名同标签、`topic-graph.md` 写的候选门槛与代码实际判据不符。

**证据定位**（实现前逐条核对，结论写进各 Decision 的「落点」）：

| 现象 | 现状位置 | 现状行为 |
| --- | --- | --- |
| tooltip 伴生数字 | `front/app/features/tags/components/console/observationLanes.ts:contentLabel` | `（最多一天桶 ${metrics.peak} 条）`，`peak` 来自 `thread_count`（含综合稿） |
| 矩阵格回退值 | `front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics` + `front/app/features/tags/composables/laneMatrixModel.ts:buildLaneRow` | 回退到 `thread_count ?? events+folded`（含综合稿） |
| 「当日其他话题」合计 | `front/app/features/tags/components/console/dayOtherTopics.ts:buildOtherTopics` | `rows.reduce(… threadCount)`，即 `threads.length`（含综合稿），注释却写「与矩阵主行格值同口径」 |
| 两个「全部 M」 | `useLaneCorrection.ts:totalMemberCount`（加法）vs `LaneAnchorScope.vue:poolCountText`（去重并集） | 同一屏两个不同数 |
| 窗口内外不对称 | `daily_report_manual_topic.go`：`sections` 要求 `embedding IS NOT NULL`，`early_members` 不要求 | 「窗口 N」与「全部 M」不是同一集合的子集关系 |
| 「最新日报」锚点 | `lane_snapshot_repository.go:GetBoardReportAnchor`（不筛 status）vs `daily_report_manual_topic.go:LatestCompletedReportDate`（`status='completed'`） | 有未完成日报时两个窗口末日差一天 |
| 距离 0 丢失 | `daily_report_repository.go:SectionTimelineNode.TopicMatchDistance`、`daily_report_models.go:DailyReportSection.TopicMatchDistance` | `float64 + omitempty` → 恰为 0 时字段消失，前端接近度变 null |
| 文档口径错 | `docs/reference/flow/topic-graph.md` 红线 2/正文 63 行 | 写候选门槛是 `consecutive_hits`，代码实际用 `hit_count >= upgrade_threshold`（代码两处自洽） |

## Goals

1. **口径主线**：矩阵格 tooltip、「当日其他话题」合计、前端回退路径、池曲线文案四处统一到「非综合稿线索数」，并让「综合稿不计入」在界面上有一行常驻说明。
2. **同屏唯一**：两个「全部 M」同定义、窗口内外成员集合对称、两个窗口末日同锚。
3. **数字可辨**：接近度区分「归属快照」与「实时估算」；`topic_match_distance` 为 0 时不被 JSON 省略。
4. **口径写死**：`lane_heat` 的 NULL scope 判据在代码注释与 `docs/reference/flow/` 落成显式条款；`topic-graph.md` 的候选门槛判据改对。
5. **可验证**：complex 档三制品齐全 + 白盒分支表/边界值清单；后端 bug 修复先写复现测试再修。

## Non-Goals

1. **不实现 `unify-lane-workbench`**（画布统一 + 时间视窗）：画布图例、「大小=接近度」/成员半径不符、新建泳道编排侧（画布节点卡 / 右栏「距离 x.xxx」）的同向统一，均归该 change；本 change 不碰 `front/app/features/tags/components/console/LaneCorrectionCanvas.vue`。
2. **不改矩阵行序/色深算法**：`lane_heat` → 全窗线索合计 → topic id 的行序与行内归一色深已经是现行口径，本 change 只修它们的**输入**（线索数口径）与**伴生文案**。
3. **不改事件线退役结论**：story 量纲已随 `retire-story-adopt-event-metrics` 退役，本 change 只把残留文案（「事件线」「兜底」）清干净，不复活任何 story 字段。
4. **不动日报生成管线与话题生命周期**：`planLifecycle` 命中计数、候选转正/归档门禁、泳道结算提示词一字不改。
5. **不重写话题成员集合的定义**：「全部成员」仍等于本话题成员 section 总数；本 change 只让窗口内外两侧用**同一过滤判据**（向量可用性），不改「谁算成员」的归属语义。

## Decisions

### D1. 桶级综合稿数由 lane-dynamics 响应下发，不新开端点
- **结论**: `LaneDynamicsTimelineSection` 增加 `composite_count`（该桶综合稿 thread 数）；`listLaneSectionThreads` 已在 select `scope`，只需在同一次扫描里累加 composite 计数，零额外查询。
- **落点**: `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:LaneDynamicsTimelineSection` + `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:listLaneSectionThreads`
- **取舍**: 被否方案①「前端从日报详情 `threads[].scope` 自己数」——矩阵格来自 lane-dynamics 聚合响应，日报详情是按选中日懒加载的另一条链路，首屏矩阵拿不到；被否方案②「响应下发每条 thread 的 scope」——带宽与语义都过重（每桶最多 5 条标题 + 折叠数，thread 明细不下发）。

### D2. 前端「桶线索数」只有一条公式：`thread_count - composite_count`，不可数即未知
- **结论**: 归一层新增「桶线索数」派生：两个字段都已知 → `thread_count - composite_count`；任一未知 → 该桶线索数 `null`（未知，不造 0）。`dayCellMetrics` 的 `total`/`peak` 与格值回退一律改吃这条派生值；`thread_count` 与 `events+folded` 两条「含综合稿」旧回退从线索数计算中彻底退出（`fallbackClueCount` 字段删除）。
- **落点**: `front/app/api/normalizers/laneDynamicsWindow.ts:laneSectionClueCount` + `front/app/api/normalizers/laneDynamicsWindow.ts:dayCellMetrics` + `front/app/features/tags/composables/laneMatrixModel.ts:buildLaneRow` + `front/app/features/tags/components/console/observationLanes.ts:buildObservationRows` + `front/app/features/tags/components/console/laneChronicleMetrics.ts:formatDayTotal`
  - `dayCellMetrics` 同时给出 `clueCountSource`（`backend` / `buckets` / `none`）：`clue_count` 是权威完整值时（`backend`）就算各桶明细有未知也照常展示完整值；只有旧响应回退各桶合计（`buckets`）且有桶未知时才写成下界 `≥N`；全不可数（`none`）显示 `?`。这条区分是「诚实降级」在编年计数上的具体形态——不能把权威值降级成 `≥N`，也不能把下界冒充完整值。
- **取舍**: 被否方案①「保留 `thread_count` 旧回退」——违反 ADDED requirement「MUST NOT 用含综合稿的 thread 总数冒充线索数」；被否方案②「旧响应缺 `composite_count` 时按「该桶无综合稿」乐观假设」——需要假设历史数据没有综合稿，而综合稿是 2026-11 起常态存在的产物，假设即造假。代价见 Risks R1（旧后端错配时格值显示 `?`，这是规格要求的诚实降级，不是缺陷）。

### D3. 矩阵格 title/aria 同口径 + 差额显式说明，桶峰值降级为「各桶线索数」
- **结论**: 格子的 title/aria = 「当日线索合计 · N 个主题桶（各桶 X / Y 条；另有 M 条综合稿未计入）」：线索条数只出现格值口径（`clueCount`）与各桶非综合稿条数；综合稿以**差额**形式显式说明，不再出现「最多一天桶 N 条」这种由含综合稿 `thread_count` 推出的数。
- **落点**: `front/app/features/tags/components/console/observationLanes.ts:contentLabel` + `front/app/features/tags/components/console/observationLanes.ts:matrixCellView` + `front/app/features/tags/components/console/observationLanes.ts:DayCellMetrics`
- **取舍**: 被否方案①「保留桶峰值但改用非综合稿口径」——仍是另一个口径的线索数，且与格值重复，违反「同一句里不得出现两个不同口径的线索数」的精神；被否方案②「只删桶峰值、不补差额」——用户仍看不到「为什么详情里比格值多几条」，差额必须显式。

### D4. 常驻口径说明行独立成行，不并入图例
- **结论**: 矩阵区头部新增一行只读说明（复用 muted 小字样式，非强调色、不可交互）：「数字 = 当日线索合计（各主题桶非综合稿线索数之和；综合稿不计入，避免一条稿给多条泳道各灌热度）」；既有图例行（`'—' 无分区` / `'?' 未取到`）保持不变。
- **落点**: `front/app/features/tags/components/console/ObservationMatrix.vue`（`omx-head` 区域新增 `data-testid="matrix-basis-note"` 行）
- **取舍**: 被否方案①「把口径说明并进既有图例行」——图例行讲的是三态符号，口径说明讲的是数字定义，两件事挤一行会让两套语义互相稀释；被否方案②「放在矩阵下方/折叠进说明」——规格要求「常驻」。

### D5. 当日工作面的综合稿标记走日报详情已带的 `scope`，不新增接口
- **结论**: `daily_report_threads.scope`（`'composite'` / `'single'`，后端已随 thread DTO 下发）经前端 API 层透传到 `LaneDayThread.scope`，编年工作面的综合稿条目加只读徽标「综合稿」；徽标文案明确「不计入线索合计」，不得暗示被计入。
- **落点**: `front/app/api/dailyReports.ts:DailyReportThread` + `front/app/api/normalizers/laneDayReading.ts:normalizeThread` + `front/app/features/tags/components/console/LaneChronicleThread.vue`
- **取舍**: 被否方案①「前端按 `tag_ids` 里 event 标签数 ≥2 自行判定综合稿」——前端拿不到 event 标签的 `category`，且会把判据复制到第二处；被否方案②「不标记、只在口径说明里提一句」——规格要求条目级可辨识。

### D6. 「当日其他话题」组的线索合计改非综合稿口径，注释按实际口径重写
- **结论**: 每行的 `threadCount` 改记**非综合稿**线索数（行内 thread 按 `scope` 判定），并新增 `compositeCount`；组计数 = 各行非综合稿线索数之和；文件头注释里「与矩阵主行格值同口径」改为写明「非综合稿线索数合计，与矩阵日格同一口径」。
- **落点**: `front/app/features/tags/components/console/dayOtherTopics.ts:toRow` + `front/app/features/tags/components/console/dayOtherTopics.ts:buildOtherTopics` + `front/app/api/normalizers/laneDayReading.ts:LaneDayReportSection`
- **取舍**: 被否方案①「只改组计数、行计数保留 thread 总数」——组内每行标着「N 条线索」，行数加起来等于组数，两个口径会再次同屏打架；被否方案②「组计数保留旧口径并加注」——直接违反「SHALL 与矩阵格同口径」。

### D7. 池曲线只改单位文案与注释，不动组件结构/交互/props
- **结论**: `ObservationRangeControls.vue` 的 caption 改为「365 日池 · 已返回泳道的日线索数」/「线索数（非文章数）· 综合稿不计入」，`series` prop 的注释与文件头注释删掉「事件线数 > 0 用事件线数，否则线索合计兜底」的降级描述；组件结构、事件、props/emits 一律不动。
- **落点**: `front/app/features/tags/components/console/ObservationRangeControls.vue`（caption 段 + `series` prop 注释 + 文件头注释）
- **取舍**: 被否方案①「曲线改画非综合稿线索数」——`laneConsoleView.ts:poolDayTotal` 已经用 `metrics.clueCount`（非综合稿口径），曲线值本身无需改，只需把仍在讲「事件线/兜底」的文案改掉。

### D8. 「全部 M」唯一实现 = 成员 section 总数（窗口内 + 窗口外，加法），两处同源
- **结论**: `useLaneCorrection.ts:totalMemberCount`（成员 section 总数，加法）作为唯一「全部 M」；左栏徽标与首义范围面板都消费它（面板新增 `total-count` prop，未传时回退旧的池内计数以兼容旧调用）；面板的「窗口内 N」也改用窗口内**成员**数（`memberNodes.length`），避免首义集里已被移出的节点把两处计数拉成两套算法。
- **落点**: `front/app/features/tags/composables/useLaneCorrection.ts:totalMemberCount` + `front/app/features/tags/components/console/LaneAnchorScope.vue:poolCountText` + `front/app/features/tags/components/console/BoardComposeOverlay.vue`（把 `correction.totalMemberCount.value` 透传给 `LaneAnchorScope`）
- **取舍**: 被否方案①「面板继续用池长度（去重并集）」——规格明令两个「全部」不得用两套算法，且去重并集会把「已移出但仍在首义集」的节点算成成员；被否方案②「面板池裁成纯成员集」——已移出但仍在人工首义集里的节点必须留在选择池里（用户要能取消它的首义身份），裁池是行为变更，越界。

### D9. 窗口内外成员集合对称：两侧同用「向量可用」判据
- **结论**: `getComposeEarlyMembers` 与 `GetComposeCandidates` 的 sections 查询用同一过滤判据——都要求 `ds.embedding IS NOT NULL`，且都跳过向量解析失败/维度为零的行；**且两集合按窗口 `[cutoff, upper]` 互补切分**：窗口内取 `[cutoff, upper]`，窗口外取 `period_date < cutoff OR period_date > upper`（显式 `from`/`to` 且 `to < 最新 completed 日` 时上界之后仍可能有人，只取「更早」会让这些成员两侧都不出现——review R-3）。因此「窗口 N + 窗口外数 = 全部 M」恒成立，且「窗口 N ⊆ 全部 M」。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:getComposeEarlyMembers` + `backend-go/internal/topicgraph/repository/daily_report_manual_topic.go:GetComposeCandidates`
- **取舍**: 被否方案①「sections 侧放开向量要求」——编排画布的质心/推荐计算吃向量，放开后画布要处理空向量，属行为变更且越界到 `unify-lane-workbench` 的地盘；被否方案②「early_members 保留无向量行、只在前端计数时过滤」——前端无法分辨「无向量」与「未取到」，计数与展示会再次分叉。

### D10. 「最新日报」锚点统一为「最新 completed 日报日」，并由响应显式下发
- **结论**: 后端 `GetBoardReportAnchor` 加 `status='completed'` 过滤；lane-dynamics 响应新增 `anchor_date`（= 该 completed 锚点日），前端 `useBoardConsole.ts` 的矩阵轴末日优先取它，回退顺序 `anchor_date` → 壳层注入的 `reportAnchor` → A 响应最新日。这样矩阵窗口末日 = 泳道工作面窗口末日 = 「最新 completed 日报日」。
- **落点**: `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:GetBoardReportAnchor` + `backend-go/internal/topicgraph/repository/lane_snapshot_repository.go:LaneDynamicsResponse` + `front/app/api/laneDynamics.ts:LaneDynamicsResponse` + `front/app/api/normalizers/laneDynamicsWindow.ts:normalizeLaneDynamicsWindow` + `front/app/features/tags/composables/useBoardConsole.ts:anchorDate`
- **取舍**: 被否方案①「改 `GetLatestReportBrief`（雷达概览）也筛 completed」——雷达概览的 `report_date` 是跨板展示用的「最近日报」，不在本 change 辖区，改它会波及雷达页与其它消费者；被否方案②「前端继续用壳层注入的 `reportAnchor`」——它来自雷达概览的「任意状态最新日报」，正是差一天的来源。

### D11. `topic_match_distance` 为 0 时不得被 JSON 省略
- **结论**: `SectionTimelineNode.TopicMatchDistance`（生命线响应）与 `DailyReportSection.TopicMatchDistance`（日报详情）去掉 `omitempty`，0 值照常下发；前端 `useLaneCorrection` 的接近度换算因此能得 1.00 而不是空。
- **落点**: `backend-go/internal/topicgraph/repository/daily_report_repository.go:SectionTimelineNode` + `backend-go/internal/topicgraph/repository/daily_report_models.go:DailyReportSection`
- **取舍**: 被否方案①「改成 `*float64` 指针」——`DailyReportSection` 是 GORM 模型（列映射依赖字段类型），改指针会牵动写入路径；被否方案②「只在生命线响应里去 `omitempty`」——规格与 brief 点名两处，且日报详情里同一个字段省略 0 同样是「合法值被当缺数据」。

### D12. 接近度区分「归属快照」与「实时估算」
- **结论**: `LaneCorrectionNode` 增加 `proximityKind: 'snapshot' | 'estimate'`；成员行/更早成员/已移出/只读参照/首义节点 = `snapshot`（`1 − topic_match_distance`，入库即冻结），关联推荐行 = `estimate`（对当前成员质心实时算）。`LaneMemberList.vue` 成员行显示「接近度 0.91（快照）」，推荐行显示「估算接近度 0.72」，两者不同名不同形。
- **落点**: `front/app/features/tags/composables/useLaneCorrection.ts:LaneCorrectionNode` + `front/app/features/tags/composables/useLaneCorrection.ts:recommendations` + `front/app/features/tags/components/console/LaneMemberList.vue:proximityText`
- **取舍**: 被否方案①「统一成同一种算法（推荐也用快照距离）」——候选节点本来就没有归属快照距离，只能实算；被否方案②「同名字段、不同角标颜色」——规格要求「不得同名同形、无法区分来源」，仅靠颜色不满足可辨识要求（无障碍下颜色不可靠）。

### D13. `lane_heat` 的 scope 判据显式确认并注释化
- **结论**: `CountLaneCluesByDate` 的 `COALESCE(t.scope, '') <> 'composite'` 的语义是「**只排除综合稿**」：不是综合稿（含 scope 为 NULL / 空串的防御性情形）一律计入。**核实结果**：当前 schema 下 `daily_report_threads.scope` 是 NOT NULL + `CHECK (scope IN ('single','composite'))`（迁移 `ensureNotNullDefault` + CHECK 确保块），因此「历史行 scope 为 NULL」在当前 schema 下**不可达**——proposal 的「历史 NULL scope」前提不成立，COALESCE 是防御性写法。该判据的真实意图（只排除综合稿、不因「判不出来」丢行）写进函数注释，并在 `docs/reference/flow/semantic-board.md` 落成条款。
- **落点**: `backend-go/internal/topicgraph/repository/lane_heat_repository.go:CountLaneCluesByDate` + `backend-go/internal/topicgraph/repository/lane_heat_repository_test.go:TestLaneHeatScopeJudgmentExcludeCompositeOnly` + `docs/reference/flow/semantic-board.md`（矩阵/热度口径节）
- **取舍**: 被否方案①「改成 `t.scope IS DISTINCT FROM 'composite' AND t.scope IS NOT NULL`」——语义等价（列 NOT NULL），但把「防御性宽口径」收窄成「判不出来就不算」，一旦列约束被放开就会让历史日热度凭空掉到 0；被否方案②「顺手回填历史 scope」——派生数据不得由迁移回填（历史快照不得回填的红线）；被否方案③「写一条构造 NULL scope 的集成测试钉住」——NOT NULL + CHECK 使该数据无法构造（实测报 `null value in column "scope" ... violates not-null constraint`），改为断言 schema 事实 + 可达行为（single 计入 / composite 排除）。

### D14. 文档口径修正：`topic-graph.md` 候选门槛改 `hit_count >= upgrade_threshold`
- **结论**: `docs/reference/flow/topic-graph.md` 正文与红线 2 中「`consecutive_hits` 达阈值」改为「`hit_count >= upgrade_threshold`」（代码两处自洽：`CanActivate = Status==candidate && HitCount >= UpgradeThreshold`；可见门槛同样按 `HitCount` 过滤）；`consecutive_hits` 的真实用途（连续命中计数、中断归零）保留说明。
- **落点**: `docs/reference/flow/topic-graph.md`（正文 63 行附近 + 「业务约束与不变量」红线 2）
- **取舍**: 被否方案①「改代码去读 `consecutive_hits`」——会改变候选可见性与转正资格的现行语义（红线 2 已写「代码两处自洽」），且属行为变更不在本 change 辖区；被否方案②「只在文档加一句「以代码为准」」——不可判定，验收措辞必须封闭式。

### D15. 后端 bug 修复先写复现测试再修
- **结论**: 本 change 修的四处后端行为（桶级 `composite_count`、completed 锚点、成员集合对称、distance 0 不省略）各自先写一条会失败的复现测试，再改实现转绿；复现测试与最终断言同文件、同函数名，便于日后回归。
- **落点**: `backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` + `backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` + `backend-go/internal/topicgraph/repository/daily_report_repository_test.go` + `backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go`
- **取舍**: 无备选（complex 档 + 后端 bug 修复的硬性纪律：不可豁免）。

## Risks

| # | 风险 | 影响 | 缓解 |
| --- | --- | --- | --- |
| R1 | 前后端部署错配：前端先上（后端无 `composite_count`）→ 矩阵格全 `?` | 首屏矩阵数字不可读 | 后端与前端同仓同部署；`?` 是规格要求的诚实降级（不冒充）。已在 design 记为 Open Question Q1，如用户不接受需改为「后端先上」的发布顺序约束 |
| R2 | 「窗口内 N」改为成员口径后，首义面板的列表条数可能多于头部计数（首义集含已移出节点） | 用户可能困惑 | 面板头部计数写「窗口内 N · 全部 M」（成员口径），节点身份标（成员/更早成员/未选入）已逐行标明来源；Q2 留痕 |
| R3 | `GetBoardReportAnchor` 改 completed 口径后，泳道结算提示词的素材窗口末日随之变化（未完成日报不再当锚） | 未完成日报当天不再进入结算窗口 | 未完成日报本就没有可结算的完整分区；结算失败不阻塞日报生成（既有约束）。若用户希望结算仍锚未完成日报，需另开 change |
| R4 | `lane_heat` 判据文档化后，未来有人「顺手」把 scope 判据收窄（排除 NULL / 只认 single） | 历史日热度掉 0 | D13 的注释 + flow 条款双写；`lane_heat_repository_test.go` 补断言钉住「只排除综合稿」+ schema 事实（scope 列 NOT NULL） |
| R5 | 共享工作树里其他 change 正在改同批文件（`docs/reference/flow/*.md`、api 文档） | 合并冲突 / 覆盖他人改动 | 只增改本 change 相关小节，不动他人段落；收口前 `git diff` 逐文件核对；不 commit |
| R6 | 后端 testcontainer PG 在树莓派上启动慢 | 验证耗时 | 每个后端包只跑一次（`go test ./internal/topicgraph/...`），不并行开多个容器 |

## Migration Plan

1. **无 DDL、无数据迁移**：本 change 不改表结构、不回填历史数据（`daily_report_threads.scope` / `daily_report_sections.topic_match_distance` 均为存量列，读口径变化）。
2. **发布顺序**：后端（新增 `composite_count` / `anchor_date` 字段 + completed 锚点 + 成员集合对称 + distance 不省略）与前端（口径收敛 + 文案 + 计数同定义）同仓同部署；若分开发布，**后端先上**（见 Risks R1）。
3. **历史数据降级**：历史日报的 `scope` 为 NULL 的 thread 仍按非综合稿计入（与既有 `lane_heat` 一致，D13）；历史 section 的 `topic_match_distance` 为 0 或 NULL 的行，生命线响应分别下发 `0` 或缺省（NULL 在 DB 层仍是 NULL，指针型字段保持缺省语义）。
4. **回滚**：全部改动为代码级，回滚 = `git revert` 本 change 的提交；无残留数据副作用。
5. **旧数据如何降级给用户的说明**（AGENTS.md 完工汇报义务）：矩阵格数字口径变化后，**同一屏不再出现两个口径的线索数**；综合稿条目在工作面带「综合稿」标记且不计入数字；历史日的数字不回溯改写（历史快照不得回填）。

## Open Questions

| # | 问题 | 最保守读法（当前实现） | 待拍板点 |
| --- | --- | --- | --- |
| Q1 | 旧后端（无 `composite_count`）与新前端错配时，矩阵格是否接受全 `?` | 接受（规格明令不得用含综合稿的 thread 总数冒充线索数；`?` 是诚实降级） | 若用户不接受，改为「后端先上」的发布顺序硬约束，而非放宽口径 |
| Q2 | 首义范围面板的「窗口内 N」按成员口径（而非选择池条数）展示，选择池可能多出「已移出但仍在首义集」的节点 | 按成员口径（与左栏同尺），节点行逐条标身份 | 若用户认为池条数更重要，需另设一个明确命名的计数（不得复用「全部 M」） |
| Q3 | 矩阵轴末日改由 lane-dynamics 响应 `anchor_date` 决定，是否接受新增该响应字段 | 接受（单一口径源头，避免前端从两个来源猜） | 若用户要求零新增字段，退回方案为「前端按 completed 口径自行推导」，但会与后端锚点产生第二处推导逻辑 |
| Q4 | 结算快照 `as_of` 的单调性（review R-8）：锚点改 completed 后，最新报告处于 generating/failed 时锚点会回退到上一个 completed 日，`UpsertLaneSnapshot` 可能以更早的 `as_of` 覆盖已有快照 | 本 change 不改结算路径（结算失败/回退不阻塞日报是既有红线；`as_of` 单调性属结算纪律，需独立设计与 PG 集成用例） | 若用户要求本轮一并修，需要在 upsert 前比对既有 `as_of` 并跳过降级写入（新增 PG 用例 + 结算语义说明） |
| Q5 | `topic_match_distance` 为 0 与 DB NULL 在 JSON 里仍不可分（review R-7）：改可空指针会牵动 GORM 模型写入路径与日报徽标的锚定分档语义 | 只去 `omitempty`（0 照常下发，NULL 仍是 0），本 change 的验收锚（生命线响应含 `0`、泳道工作面接近度 1.00）已满足 | 若要让「0 = 完美锚定」与「无距离」在日报阅读面也分流，需另开 change 把字段改可空并调整 `topicAnchorTier` 分档 |
