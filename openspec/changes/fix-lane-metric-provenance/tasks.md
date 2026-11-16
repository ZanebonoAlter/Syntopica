# Tasks：泳道/矩阵数字口径修正（线索合计口径收敛 + 伴生数字对齐 + 计数去歧义）

> 六个交付故事（见 test-cases.md）：S1 矩阵格数字口径、S2 伴生数字与常驻说明、S3 观察窗曲线同口径、S4 成员计数与接近度同尺、S5 两个窗口末日一致、S6 泳道卡片/演进带回归。
> 复选框即执行清单：后端 bug 修复**先写复现测试再修**（complex 档不可豁免）；验证方式写在同一行。
> 本 change 不碰 `front/app/features/tags/components/console/LaneCorrectionCanvas.vue`（画布归 `unify-lane-workbench`）。

## 1. 后端：桶级综合稿数、completed 锚点、成员集合对称、距离 0 不省略

- [x] 1.1 复现测试先行（先红）：`backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go` 新增 `TestLaneDynamicsSectionCompositeCount`——同桶 5 条线索（2 条 `scope='composite'`）断言响应该 section 的 `thread_count` = 5、`composite_count` = 2，且该日 `clue_count` 仍 = 3（综合稿不计）。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestLaneDynamicsSectionCompositeCount` → 失败（`composite_count` 字段不存在）。 → PASS（先红：`composite_count` 字段不存在，断言失败；1.2 后转绿）
- [x] 1.2 实现桶级综合稿数：`backend-go/internal/topicgraph/repository/lane_snapshot_repository.go` 的 `LaneDynamicsTimelineSection` 增加 `CompositeCount int json:"composite_count"`；`listLaneSectionThreads` 在既有 select `scope` 的同一次扫描里累加 composite 计数（`laneSectionThreads` 加 `Composite` 字段）；`GetBoardLaneDynamics` 组装 section 时回填。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestLaneDynamicsSectionCompositeCount` → PASS。 → PASS（`cd backend-go && go test ./internal/topicgraph/repository -run TestLaneDynamicsSectionCompositeCount` → PASS）
- [x] 1.3 复现测试先行（先红）：同文件新增 `TestLaneDynamicsAnchorIgnoresUnfinishedReport`——版块有 completed(2026-11-10) 与 generating(2026-11-11) 两份日报，断言窗口末日 = completed 日、generating 报告的分区不出现在时间线、响应 `anchor_date` = completed 日。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestLaneDynamicsAnchorIgnoresUnfinishedReport` → 失败（锚点取了 generating 日）。 → PASS（先红：锚点取了 generating 日、generating 分区进了时间线；1.4 后转绿）
- [x] 1.4 实现 completed 锚点：`lane_snapshot_repository.go` 的 `GetBoardReportAnchor` 增加 `status = 'completed'` 过滤；`listLaneWindowSections` 增加 `r.status = 'completed'` 过滤（未完成日报的分区不进窗口内容）；`LaneDynamicsResponse` 增加 `AnchorDate string json:"anchor_date"`（无日报时为空串）。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestLaneDynamicsAnchorIgnoresUnfinishedReport` → PASS。 → PASS（`... -run TestLaneDynamicsAnchorIgnoresUnfinishedReport` → PASS；顺带把候选 hint 查询也统一到 completed 口径）
- [x] 1.5 复现测试先行（先红）：`backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go` 新增 `TestComposeCandidatesMemberSetsUseSameVectorFilter`——同话题 3 个窗口内成员（有向量）+ 2 个窗口外成员（1 有向量、1 embedding 为 NULL），断言 `early_members` 只含那 1 个有向量的窗口外成员，且「窗口 N + 更早数 = 全部 M」。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestComposeCandidatesMemberSetsUseSameVectorFilter` → 失败（NULL 向量成员被计入 `early_members`）。 → PASS（先红：embedding 为 NULL 的窗口外成员被计入 `early_members`）
- [x] 1.6 实现成员集合对称：`daily_report_manual_topic.go` 的 `getComposeEarlyMembers` 与 `GetComposeCandidates` 的 sections 查询同判据（都要求 `ds.embedding IS NOT NULL`，且都跳过向量解析失败/长度为 0 的行）；`ComposeEarlyMember.Embedding` 的注释从「解析失败为 null」改为「无可用向量的成员不入集」。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestComposeCandidatesMemberSetsUseSameVectorFilter` → PASS。 → PASS（`... -run TestComposeCandidatesMemberSetsUseSameVectorFilter` → PASS；两侧同判据，窗口 N + 更早数 = 全部 M）
- [x] 1.7 复现测试先行（先红）：`backend-go/internal/topicgraph/repository/daily_report_repository_test.go` 新增 `TestTopicLifelineKeepsZeroMatchDistance`——某 section 的 `topic_match_distance` 恰为 0，断言生命线响应 JSON 含 `"topic_match_distance":0`（不被省略）。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestTopicLifelineKeepsZeroMatchDistance` → 失败（字段被 `omitempty` 省略）。 → PASS（先红：`topic_match_distance` 被 `omitempty` 省略，JSON 里找不到该键）
- [x] 1.8 实现距离 0 不省略：`daily_report_repository.go` 的 `SectionTimelineNode.TopicMatchDistance` 与 `daily_report_models.go` 的 `DailyReportSection.TopicMatchDistance` 去掉 `omitempty`（`float64` 保留，0 值照常下发）。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestTopicLifelineKeepsZeroMatchDistance` → PASS。 → PASS（`... -run TestTopicLifelineKeepsZeroMatchDistance` → PASS，响应含 `"topic_match_distance":0`）
- [x] 1.9 `lane_heat` 的 NULL scope 判据注释化：`backend-go/internal/topicgraph/repository/lane_heat_repository.go` 的 `CountLaneCluesByDate` 写明「`COALESCE(t.scope,'') <> 'composite'` 有意让历史 NULL scope 的 thread 按非综合稿计入（`scope` 列默认 `'single'`，历史行为 NULL 即非综合稿；综合稿是 2026-11 之后的产物）」，并在 `lane_heat_repository_test.go` 补一条「scope 为 NULL 的 thread 计入线索数」的断言钉住该判据。验证：`cd backend-go && go test ./internal/topicgraph/repository -run TestLaneHeat` → PASS。 → PASS（`... -run TestLaneHeatScopeJudgmentExcludeCompositeOnly` → PASS；**前提修正**：`scope` 列 NOT NULL + CHECK → 「历史 NULL scope」在当前 schema 不可达，改为断言「只排除综合稿」+ schema 事实，见 design D13）

## 2. 前端：矩阵格口径（tooltip / 常驻说明 / 回退 / 曲线文案）

- [x] 2.1 协议字段：`front/app/api/laneDynamics.ts` 的 `LaneTimelineSection` 增加 `composite_count?: number`、`LaneDynamicsResponse` 增加 `anchor_date?: string`（均标注「缺省 = 旧后端」）。验证：`cd front && pnpm exec nuxi typecheck` → 通过（无未用导入/类型错误）。 → PASS（`cd front && pnpm exec nuxi typecheck` → 通过）
- [x] 2.2 归一层的桶线索数唯一公式：`front/app/api/normalizers/laneDynamicsWindow.ts` 新增桶线索数派生（`thread_count - composite_count`，任一缺失 → `null` 未知）；`WindowSection` 增加 `compositeCount` 与派生后的桶线索数；`dayCellMetrics` 的 `total`/`peak` 改吃派生值并新增 `compositeCount`；删除 `fallbackClueCount` 及其「events+folded 回退」语义；`anchorDate` 优先取 `anchor_date`。验证：`cd front && pnpm test:unit app/api/normalizers/laneDynamicsWindow.test.ts --maxWorkers=2` → PASS。 → PASS（`cd front && pnpm test:unit app/api/normalizers/laneDynamicsWindow.test.ts --maxWorkers=2` → 11 passed）
- [x] 2.3 前端回退路径不冒充线索数：`front/app/features/tags/composables/laneMatrixModel.ts` 的 `buildLaneRow` 中 `clueCounts` 回退改用 2.2 的同一派生公式（不得再用含综合稿的 `thread_count`/`events+folded`）。验证：`cd front && pnpm test:unit app/features/tags/composables/laneMatrixModel.test.ts --maxWorkers=2` → PASS（本 change 新建该测试文件）。 → PASS（`... pnpm test:unit app/features/tags/composables/laneMatrixModel.test.ts --maxWorkers=2` → 7 passed；本 change 新建该测试文件）
- [x] 2.4 同行行模型同口径：`front/app/features/tags/components/console/observationLanes.ts` 的 `buildObservationRows` 当日线索合计改吃派生值；`contentLabel` 改为同口径文案（当日线索合计 + 主题桶数 + 各桶非综合稿条数 + 「另有 N 条综合稿未计入」差额说明），删除「最多一天桶 N 条」。验证：`cd front && pnpm test:unit app/features/tags/components/console/observationLanes.test.ts --maxWorkers=2` → PASS。 → PASS（`... pnpm test:unit app/features/tags/components/console/observationLanes.test.ts --maxWorkers=2` → 全绿）
- [x] 2.5 常驻口径说明行：`front/app/features/tags/components/console/ObservationMatrix.vue` 矩阵区头部新增一行只读说明「数字 = 当日线索合计（各主题桶非综合稿线索数之和；综合稿不计入，避免一条稿给多条泳道各灌热度）」（`data-testid="matrix-basis-note"`，复用 muted 小字样式、不可交互）。验证：`cd front && pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts --maxWorkers=2` → PASS。 → PASS（`... pnpm test:unit app/features/tags/components/console/ObservationMatrix.test.ts --maxWorkers=2` → 26 passed）
- [x] 2.6 池曲线单位文案：`front/app/features/tags/components/console/ObservationRangeControls.vue` 的 caption 改为「365 日池 · 已返回泳道的日线索数」/「线索数（非文章数）· 综合稿不计入」，`series` prop 注释与文件头注释删掉「事件线数 > 0 用事件线数，否则线索合计兜底」的降级描述；**不动**该组件的结构、交互、props/emits。验证：`cd front && pnpm test:unit app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2` → PASS（断言文案含「线索数（非文章数）」且全文无「事件线」「兜底」）。 → PASS（`... pnpm test:unit app/features/tags/components/console/ObservationRangeControls.test.ts --maxWorkers=2` → 全绿；组件结构/交互/props 未动，只改 caption + Y 轴单位 + 注释）
- [x] 2.7 矩阵轴末日对齐：`front/app/features/tags/composables/useBoardConsole.ts` 的 `anchorDate` 优先取 lane-dynamics 响应的 `anchor_date`（回退顺序：壳层注入 `reportAnchor` → A 响应最新日 → 今天）。验证：`cd front && pnpm test:unit app/features/tags/composables/useBoardConsole.observation.test.ts --maxWorkers=2` → PASS。 → PASS（`... pnpm test:unit app/features/tags/composables/useBoardConsole.observation.test.ts --maxWorkers=2` → 27 passed）
- [x] 2.8 thread `scope` 透传：`front/app/api/dailyReports.ts` 的 `DailyReportThread` 增加 `scope?: string`；`front/app/api/normalizers/laneDayReading.ts` 的 `LaneDayThread` 增加 `scope` 并在 `normalizeThread` 透传（`'composite'` / `'single'`）。验证：`cd front && pnpm test:unit app/api/normalizers/laneDayReading.test.ts --maxWorkers=2` → PASS（若该文件不存在则并入 2.9 的用例文件断言）。 → PASS（`... pnpm test:unit app/api/normalizers/laneDayReading.test.ts --maxWorkers=2` → 全绿）
- [x] 2.9 「当日其他话题」线索合计改非综合稿口径：`front/app/features/tags/components/console/dayOtherTopics.ts` 的 `toRow` 行计数改记非综合稿线索数（并记 `compositeCount`），组计数 = 各行非综合稿之和；文件头注释里「与矩阵主行格值同口径」改写为「非综合稿线索数合计，与矩阵日格同一口径」。验证：`cd front && pnpm test:unit app/features/tags/components/console/dayOtherTopics.test.ts --maxWorkers=2` → PASS。 → PASS（`... pnpm test:unit app/features/tags/components/console/dayOtherTopics.test.ts --maxWorkers=2` → 22 passed）
- [x] 2.10 当日工作面综合稿标记：`front/app/features/tags/components/console/LaneChronicleThread.vue` 对 `thread.scope === 'composite'` 的条目渲染只读徽标「综合稿（不计入线索合计）」，普通条目不带。验证：`cd front && pnpm test:unit app/features/tags/components/console/LaneChronicle.test.ts --maxWorkers=2` → PASS。 → PASS（`... pnpm test:unit app/features/tags/components/console/LaneChronicle.test.ts --maxWorkers=2` → 29 passed）

## 3. 前端：泳道工作面（两个「全部 M」同尺、接近度可辨）

- [x] 3.1 接近度来源标注：`front/app/features/tags/composables/useLaneCorrection.ts` 的 `LaneCorrectionNode` 增加 `proximityKind: 'snapshot' | 'estimate'`（成员/更早成员/已移出/只读参照/首义节点 = `snapshot`，`recommendations` = `estimate`）。验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts --maxWorkers=2` → PASS。 → PASS（`... pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts --maxWorkers=2` → 全绿）
- [x] 3.2 两处「全部 M」同定义：`useLaneCorrection.ts` 的 `totalMemberCount` 注释写明「唯一「全部 M」= 本话题成员 section 总数（窗口内 + 窗口外）」；`front/app/features/tags/components/console/LaneMemberList.vue` 的徽标与 `front/app/features/tags/components/console/LaneAnchorScope.vue` 的 `poolCountText` 消费同一个数（`LaneAnchorScope` 新增 `total-count` / `window-count` props，未传时回退旧的池内计数以兼容旧调用）；`front/app/features/tags/components/console/BoardComposeOverlay.vue` 把 `correction.totalMemberCount.value` 透传给 `LaneAnchorScope`。验证：`cd front && pnpm test:unit app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneAnchorScope.test.ts --maxWorkers=2` → PASS（夹具：窗口内 11 + 窗口外 97 → 两处「全部」同为 108）。 → PASS（`... pnpm test:unit app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneAnchorScope.test.ts --maxWorkers=2` → 全绿；面板新增 `window-count` / `total-count` props，旧调用回退池计数）
- [x] 3.3 接近度标签可辨：`LaneMemberList.vue` 的 `proximityText` 对快照节点输出「接近度 x.xx（快照）」、对估算节点输出「估算接近度 x.xx」；成员行补上接近度展示（此前只有更早成员卡与推荐行有）。验证：`cd front && pnpm test:unit app/features/tags/components/console/LaneMemberList.test.ts --maxWorkers=2` → PASS（断言两类节点文本不同名不同形）。 → PASS（同上命令 → 全绿；成员行「接近度 x.xx（快照）」vs 推荐行「估算接近度 x.xx」）

## 4. 文档口径修正（代码之外的写死）

- [x] 4.1 `docs/reference/flow/topic-graph.md`：正文与「业务约束与不变量」红线 2 中候选门槛由 `consecutive_hits` 改写为 `hit_count >= upgrade_threshold`（代码两处自洽：`CanActivate = Status==candidate && HitCount >= UpgradeThreshold` 与话题管理可见门槛）；`consecutive_hits` 的真实用途（连续命中计数、中断归零）保留说明。验证：`grep -n "hit_count >= upgrade_threshold" docs/reference/flow/topic-graph.md` → 命中 ≥1 行；`grep -n "consecutive_hits < upgrade_threshold" docs/reference/flow/topic-graph.md` → 0 命中。 → PASS（`grep -n "hit_count >= upgrade_threshold" docs/reference/flow/topic-graph.md` 命中；`grep -n "consecutive_hits < upgrade_threshold" docs/reference/flow/topic-graph.md` → 0 命中；正文 63 行段 + Mermaid 节点 + 红线 2 三处同改）
- [x] 4.2 `docs/reference/flow/semantic-board.md`：矩阵/热度口径节补两条——矩阵日格 = 当日各主题桶**非综合稿**线索数之和（综合稿不计）、`lane_heat` 线索计数对历史 NULL scope 按非综合稿计入（`COALESCE(t.scope,'') <> 'composite'` 有意为之）。验证：`grep -n "非综合稿" docs/reference/flow/semantic-board.md` → 命中 ≥1 行。 → PASS（`grep -n "composite_count" docs/reference/flow/semantic-board.md` 命中；红线 28 追加 2026-11-16 口径收敛五点）
- [x] 4.3 `docs/reference/api/daily-reports.md` 与 `docs/reference/api/semantic-boards.md`：补 lane-dynamics 响应的 `sections[].composite_count` 与 `anchor_date` 两个字段说明（只增本 change 相关小节，不动他人段落）。验证：`grep -n "composite_count\|anchor_date" docs/reference/api/daily-reports.md docs/reference/api/semantic-boards.md` → 命中 ≥1 行。 → PASS（`docs/reference/api/daily-reports.md` 补 lane-dynamics `anchor_date` / `sections[].composite_count` / 锚点 completed 口径、compose-candidates 成员集合对称、lifeline `topic_match_distance` 恒下发；**`docs/reference/api/semantic-boards.md` 为假警报**：该文件不含 lane-dynamics / clue_count / topic_match_distance 任何章节（grep 0 命中），相关端点契约都在 daily-reports.md → 不改）

## 5. 测试

- [x] 5.1 后端复现测试与实现同步落定：1.1/1.3/1.5/1.7 四条复现测试均先红后绿，1.9 补 NULL scope 判据断言。验证：`cd backend-go && go test ./internal/topicgraph/...` → 全绿（无 FAIL；只跑影响包，不跑 `go test ./...`）。 → PASS（`cd backend-go && go test ./internal/topicgraph/...` → repository/handler/service 三包全绿，无 FAIL；只跑影响包，未跑全量）
- [x] 5.2 前端回归测试补齐：2.2–2.10 与 3.1–3.3 各自落断言（含旧资产改断言：`observationLanes.test.ts` 的 MX-2/MX-5/MX-7 改为非综合稿口径、`laneDynamicsWindow.test.ts` 的三级回退改为派生公式、`ObservationMatrix.test.ts` 补常驻说明行断言）。验证：`cd front && pnpm test:unit app/api/normalizers/laneDynamicsWindow.test.ts app/features/tags/components/console/observationLanes.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneAnchorScope.test.ts app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/composables/laneMatrixModel.test.ts app/features/tags/composables/useBoardConsole.observation.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts app/features/tags/components/console/LaneChronicle.test.ts --maxWorkers=2` → 全绿（不跑全量 `pnpm test:unit`）。 → PASS（69 个受影响前端测试文件 1091 用例全绿：`pnpm test:unit <69 文件> --maxWorkers=2`；含旧资产改断言：observationLanes MX-2/MX-7、laneDynamicsWindow 三级回退、ObservationMatrix 常驻说明、laneConsoleView/useConsoleSelection/BoardMatrixView/BoardLaneEvolution/useBoardConsole 的夹具补 `composite_count`）
- [x] 5.3 效果核对：按 test-cases.md 的「效果核对执行记录」逐条实跑并回填数字——S1 真库 thread `scope` 分布与某泳道当日合计复算、S2 人工看页面三处（常驻说明/悬停文案/工作面徽标）、S3 真库抽 4 日曲线 vs 格值比对、S4 真库成员总数比对、S5 真库非 completed 日报样本核查。验证：test-cases.md 的效果核对表六行均已填入数字与结论，不留「待回填」字样。 → PASS（真库只读统计已回填 test-cases.md「效果核对执行记录」：全库 14610 thread 中综合稿 179 条（1.2%）；样本版块 15220 当日 18 thread → 12 非综合稿线索）

## 6. Review

- [x] 6.1 派子线程聚焦 review（审查制品 + diff），意见按 High/Medium/Low 处置后 review.md 建账（重点查：前后端是否各写了一份等价口径、旧资产改断言是否漏改、tooltip 文案是否仍有第二个口径的线索数、成员集合两侧判据是否真的同一）。验证：`bash scripts/harness/review-trace.sh openspec/changes/fix-lane-metric-provenance` → 通过；`review.md` 存在且每条意见有 High/Medium/Low 处置结论。 → PASS（首轮全量 review 子线程 30 分钟超时无产出，留痕 run `5dafec45`；改派窄口径 review 子线程 `60caea01` 得 8 条意见 R-1～R-8：**已修复 4 条**（R-1 下界标注、R-2 结算素材补 completed 过滤、R-3 窗口外成员取上界之后、R-4 未知桶不列「各桶」明细、R-6 负差值不抹 0 —— 共 5 条）、**已驳回 2 条**（R-5 日级综合稿合计需后端另开字段、R-7 距离 0 与 NULL 的日报阅读面分流属另一辖区，记 Open Question Q5）、**转挂 1 条**（R-8 结算快照 `as_of` 单调性，记 Risks R3 + Open Question Q4）。`review-trace.sh` → `✓ 通过：review.md 建账 8 条`）

## 7. 文档
<!-- doc-impact: flow, api -->

> 业务域（`constraint-domains` 口径，见 proposal.md）：daily-report / semantic-board / topic-graph；
> 文档目录轴（`doc-impact` 口径，机器可读）：flow（业务红线与不变量）+ api（响应字段契约）。

- [x] 7.1 按 4.1–4.3 更新 `docs/reference/flow/topic-graph.md`、`docs/reference/flow/semantic-board.md`、`docs/reference/api/daily-reports.md`、`docs/reference/api/semantic-boards.md`（只增改本 change 相关小节，不动他人段落、不回退他人改动；归档时按 §12 补 flow 变更溯源链接）。验证：`bash scripts/harness/doc-impact.sh verify openspec/changes/fix-lane-metric-provenance` → 通过。 → 完成（verify 通过：声明 flow, api ／文件 4 个；实际落点：`flow/topic-graph.md` 候选门槛改 `hit_count`、`flow/semantic-board.md` 矩阵口径改非综合稿线索合计、`api/daily-reports.md` 补 `composite_count` + 锚点 completed 口径 + `topic_match_distance` 恒下发；`api/semantic-boards.md` 无需改——lane-dynamics 段落在 `api/daily-reports.md`）

## 8. 验证

- [x] 8.1 跑本 change 的门禁与影响范围验证：openspec strict、scenario-trace、check-tasks-tail、doc-impact verify，以及 §1–§5 各条定点 go test / vitest；把实际命令与结果写进本节执行记录表（含失败项处置）。验证：逐条命令 + 期望结果；本轮不跑全量测试、不连真库写入、不执行 git 提交。 → PASS（命令与结果见 8.2；未跑全量前端测试、未连真库写入、未执行 git 提交）

### 8.1 定点验证命令与期望结果

| 命令 | 期望结果 |
| --- | --- |
| `cd backend-go && go test ./internal/topicgraph/repository -run 'TestLaneDynamics\|TestComposeCandidates\|TestTopicLifeline\|TestLaneHeat'` | PASS（无 FAIL） |
| `cd backend-go && go test ./internal/topicgraph/handler -run TestComposeCandidates` | PASS（无 FAIL） |
| `cd backend-go && go test ./internal/topicgraph/...` | PASS（无 FAIL；只跑影响包） |
| `cd front && pnpm test:unit app/api/normalizers/laneDynamicsWindow.test.ts app/features/tags/components/console/observationLanes.test.ts app/features/tags/components/console/ObservationMatrix.test.ts app/features/tags/components/console/dayOtherTopics.test.ts app/features/tags/components/console/LaneMemberList.test.ts app/features/tags/components/console/LaneAnchorScope.test.ts app/features/tags/components/console/ObservationRangeControls.test.ts app/features/tags/components/console/LaneChronicle.test.ts app/features/tags/composables/useLaneCorrection.test.ts app/features/tags/composables/laneMatrixModel.test.ts app/features/tags/composables/useBoardConsole.observation.test.ts --maxWorkers=2` | PASS（无 FAIL；参数不带 `--`） |
| `cd front && pnpm exec nuxi typecheck` | 通过（无类型错误） |
| `cd front && pnpm lint` | 通过（无新增 lint 错误） |
| `openspec validate fix-lane-metric-provenance --strict` | `Change 'fix-lane-metric-provenance' is valid` |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/fix-lane-metric-provenance` | `✓ tasks.md 尾三节与 doc-impact 声明标记齐全` |
| `bash scripts/harness/scenario-trace.sh openspec/changes/fix-lane-metric-provenance` | 通过（Scenario 与测试文件映射齐） |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-lane-metric-provenance` | 通过 |
| `bash scripts/harness/review-trace.sh openspec/changes/fix-lane-metric-provenance` | 通过（`review.md` 已建账） |

### 8.2 实跑执行记录（2026-11-16，实现线程）

| 命令 | 结果 |
| --- | --- |
| `cd backend-go && go test ./internal/topicgraph/...` | ✅ `internal/topicgraph/handler` 32.7s ok · `internal/topicgraph/repository` 113.1s ok · `internal/topicgraph/service` 49.8s ok（新增 6 条复现/钉住用例全绿：桶级 composite_count、completed 锚点、成员集合对称、显式窗口成员总数、距离 0 不省略、scope 判据；只跑影响包，未跑 `go test ./...`） |
| `cd backend-go && go build ./... && go vet ./internal/topicgraph/...` | ✅ 通过（无输出） |
| `cd backend-go && golangci-lint run ./internal/topicgraph/...` | ✅ `0 issues.`（首轮报 1 处 gofmt——注释里的 `''` 被 gofmt 折成弯引号，改写为「空串」字面后复跑通过） |
| `cd front && pnpm test:unit <69 个受影响测试文件> --maxWorkers=2` | ✅ `Test Files 69 passed (69)` / `Tests 1093 passed (1093)`（含本 change 新建的 `laneMatrixModel.test.ts` 与 review 修复新增的 MX-9 / 下界断言；未跑全量 `pnpm test:unit`） |
| `cd front && pnpm exec nuxi typecheck` | ✅ 通过（无类型错误） |
| `cd front && pnpm lint` | ✅ `✖ 8 problems (0 errors, 8 warnings)` —— 8 条 warning 全在**本 change 未触碰**的文件（`ArticleCardView.test.ts` / `useArticleContentView.ts` / `DailyReportTopicSection.test.ts` / `detective-wall/utils.test.ts` / `schedulerMeta.test.ts`），无新增 error |
| `openspec validate fix-lane-metric-provenance --strict` | ✅ `Change 'fix-lane-metric-provenance' is valid` |
| `bash scripts/harness/check-tasks-tail.sh openspec/changes/fix-lane-metric-provenance` | ✅ `✓ tasks.md 尾三节与 doc-impact 声明标记齐全` |
| `bash scripts/harness/scenario-trace.sh openspec/changes/fix-lane-metric-provenance` | ✅ `通过：26 个 Scenario 映射齐全（自动测试 26 / 人工留痕 0）` |
| `bash scripts/harness/doc-impact.sh verify openspec/changes/fix-lane-metric-provenance` | ✅ `通过（声明:flow, api 文件:3 个 启发式:ownership）` |
| 真库只读核对（`psql … SELECT`，无 DDL/DML） | ✅ 综合稿占比 179/14610 = 1.2%；样本版块 15220 当日 18 thread → 12 非综合稿线索（数字已回填 test-cases.md 效果核对表） |

### 8.3 Scenario 映射表

| Scenario | 测试文件 |
| --- | --- |
| 同日分桶不冒充分区热度 | front/app/features/tags/components/console/observationLanes.test.ts |
| 矩阵格值是当日线索合计 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 综合稿不计入线索数 | backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 前端回退不冒充线索数 | front/app/api/normalizers/laneDynamicsWindow.test.ts, front/app/features/tags/composables/laneMatrixModel.test.ts |
| 色深表达行内相对强度 | front/app/features/tags/components/console/observationLanes.test.ts |
| 同屏不出现两个口径的线索数 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 当日工作面标出综合稿 | front/app/features/tags/components/console/LaneChronicle.test.ts |
| 常驻口径说明可见 | front/app/features/tags/components/console/ObservationMatrix.test.ts |
| 拖选不重排 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 精确档回切 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 明确切独立档 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 档池分层失败 | front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 首屏曲线即含历史 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 曲线与矩阵格同口径 | front/app/features/tags/components/console/ObservationRangeControls.test.ts, front/app/features/tags/components/console/laneConsoleView.test.ts |
| 池未取到有提示可重试 | front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 池失败后切 365 档不残留误述 | front/app/features/tags/components/console/BoardConsoleShellSelection.test.ts |
| 非法日期反馈 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 深链恢复 | front/app/features/tags/composables/useBoardConsole.observation.test.ts |
| 同屏只有一个全部 M | front/app/features/tags/composables/useLaneCorrection.test.ts, front/app/features/tags/components/console/LaneMemberList.test.ts |
| 窗口内外集合对称 | backend-go/internal/topicgraph/repository/daily_report_manual_topic_test.go |
| 接近度区分快照与估算 | front/app/features/tags/composables/useLaneCorrection.test.ts, front/app/features/tags/components/console/LaneMemberList.test.ts |
| 距离为零不被丢弃 | backend-go/internal/topicgraph/repository/daily_report_repository_test.go |
| 两个窗口末日一致 | backend-go/internal/topicgraph/handler/compose_candidates_handler_test.go, backend-go/internal/topicgraph/repository/lane_snapshot_repository_test.go |
| 发展时间线结构 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 一句话态势展示 | front/app/features/tags/components/console/ObservationLaneEvolution.test.ts |
| 单日多事件 | front/app/features/tags/components/console/LaneChronicle.test.ts |
