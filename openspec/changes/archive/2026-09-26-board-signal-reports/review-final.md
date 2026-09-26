# board-signal-reports 一次性完整审查报告（glm-5.3 high）

> 审查日期 2026-09-22，branch develop，fresh-context 只读审查。审查对象为阶段 0-4 交付的本 change 全部文件（以各 phase 报告①节清单与实际文件比对核实：后端 datasources/dataenrichment/database、前端 boardSignals.ts + Signal* 组件/composable、docs/reference 增量、EIA 归档 fixture）；工作树上其他 change 的预存脏改（tool_registry.go、wire.go 预存段、front/docs 其他文件、openspec/changes/connect-dsh-energy-data-sources 删除项等）已逐一甄别剔除，不计入本报告。审查方法：静态精读全部新增源码/测试/迁移/文档 + 只读运行目标测试验证（见文末核实记录）。

## A. Spec→实现完整性矩阵（每 Requirement 一行：判定/证据/缺口）

### specs/board-signal-reports/spec.md（ADDED，10 个 Requirement）

| Requirement | 判定 | 证据（file:symbol / 测试） | 缺口 |
| --- | --- | --- | --- |
| 周期材料与固定候选快照 | 已实现+有测试 | `service/signal_material.go:ParseSignalPeriod/SignalCutoff/SignalAnalysisMode/AssembleSignalMaterial`（Asia/Shanghai 半开区间、未来拒绝、retrospective、全状态泳道+归属 gap）；`handler/signal_discovery.go:triggerSignalDiscovery`（400 无 job）；`signal_material_test.go` PC-1~PC-5；`signal_discovery_test.go:TestSignalDiscoveryTrigger_InvalidPeriodRejectedWithoutJob`（7 例非法+idle 验证）；研究侧 `signal_research.go:ResearchCandidate` 只读 `discovery.InputSnapshot` 并做周期一致性拒绝 | 「迟些点击仍用原快照」主要由结构保证（研究唯一材料来源=冻结快照）+ PC-5 冻结测试，无独立端到端用例；可接受 |
| 信号发现与人工研究边界 | 已实现+有测试 | `service/signal_detect.go:SignalDetector.Detect`（≤2 尝试、整响应作废、白名单剔除、score≥6）；`SignalDiscoveryService` 无 registry 字段（编译级零工具面）；`signal_detect_test.go` SD-1~5；`handler/signal_discovery_test.go:TestSignalDiscovery_EndToEndDiscoveredPath`（stub LLM 恰 1 次调用=取数/计算/compose 全 0 的机制证据）、`TestSignalDiscovery_NoSignalKeepsOldCandidates`、`TestSignalDiscoveryJob_FailedOutcomeCarriesStage` | THEN「调用均为 0」的断言形态是「detect 恰 1 次 LLM+服务结构上无取数路径」的组合，判定充分 |
| 候选持久化与展示来源 | 已实现+有测试 | `repository/signal_repository.go:CreateSignalDiscoveryBatch`（单事务+同批去重+owner/period stamping）；`ListSignalCandidatesByPeriod`（游标倒序）；`signal_candidate_test.go` 8 用例（隔离 PG：原子保存/回滚无半批/直写 FK 拒绝/零候选批/去重/分页）；派生状态 `handler/signal_discovery.go:deriveSignalCandidateStatus` + `analysis_runner.go:RunningSignalReportForCandidate`，`TestSignalList_DerivedStatus`（live job→researching、job 结束回落，JB-3） | 无 |
| 单候选人工触发与幂等 | 已实现+有测试 | `handler/signal_research.go:triggerSignalResearch`（404/400/409 先行/200 already_reported/202 regenerate）；`signal_research_test.go:TestSignalResearch_CrossBoardCandidateAndReport404 / ConcurrentDuplicateAndIdempotentReuse / RegenerateStartsNewJob / OnlySelectedCandidateRuns / ClientSuppliedFieldsIgnored`；`TestSignalResearch_NR_OldReviewRoutesStillRegistered` | 无 |
| 四十轮问题驱动研究 | 已实现+有测试 | `service/signal_research.go`（maxLoops=40 经 toolLoopParams；policy 三动作；`signalResearchPolicy.lastStep` 收束分类）；`orchestrator.go:runToolLoop`+新增 `toolLoopActionRunner` 可选钩子（default 分支，旧路径字节不变）；`signal_research_test.go` LP-1~LP-7（17 轮 finish / 40 轮 budget_exhausted 且第 41 次不发生 / 20+20 混合 / 40 轮非法 blocked 收束 / 源失败 gap 成文 / 取消与 LLM 断裂 failed 不伪装 / question 必填+去重+/no_think+ResultFull） | 无 |
| 代码计算与公式证据 | 已实现+有测试 | `service/signal_calculation.go:runSignalCalculation`（三算子、big.Rat、half-up 4 位去尾 0、null→missing、base≤0 拒、兼容白名单）；policy 层 `RunLoopAction`（value 禁交/未知 op/空 inputs/duplicate_calculation 拦截）；`signal_calculation_test.go` CA-1~7 + 舍入表驱动（−1 MMbbl、−0.2381%、2800 Mb/d 实测） | **WDI 维度缺口**：见 E-M1（Period 键不匹配使 WDI 的 difference/mean 全域被误拒；CA 用例无 WDI 覆盖） |
| 报告论证与条件判断 | 已实现+有测试 | `service/signal_compose.go:validateSignalReportDraft`（四段唯一序/五字段/direction 枚举/可见 <3000/引用可解析/图表 0~3 与兼容）；`Compose` ≤3 尝试回注；`signal_compose_test.go` SV-1~7（2999 过/恰 3000 拒、conditional 合法、三稿耗尽 attempts=3 且错误为最后一次校验原因） | 无 |
| 观测与计算引用附录 | 已实现+有测试 | `[[data:]]/[[calc:]]/[[news:]]` 由 `resolveSignalRef` 校验、数值前端按附录查表渲染（`signalReport.ts:resolveDataRef/resolveCalcRef` raw_value 优先、缺失保真）；appendix 代码生成（draft 伪造 appendix/generation_meta 被忽略，`TestSignalCompose_AppendixIsCodeGenerated`）；60 观测不截 50 点（`TestSignalResearch_SixtyObservationsNotCutToFifty`）；完整原响应回写 tool_calls（`TestSignalResearch_CutoffFilterBeforeAgent`） | tool_calls 记录 Outcome 对类型化源错误误标 ok（见 E-M2，不影响附录账本正确性） |
| 成功快照与无评审输出 | 已实现+有测试 | `models.go:ResultKindSignalReport`（不入 isBoardResultKind）；`repository.go:validateResultShape` signal_report 分支；迁移 `20260922_0001`（CHECK 带 NULL-aware 守卫+复合 FK+部分索引）；`signal_report_test.go` 4 用例+`board_signal_migration_test.go` 3 用例（隔离 PG 直写拒绝、旧行 NULL 不回填、幂等）；NR 断言（operation 白名单 stub 默认分支即红、响应体无 review/judge/approved/digest 词） | 无 |
| 失败恢复与周期查询 | 已实现+有测试 | job 内存态+404 语义（`getAnalysisStatus` job_id 未知→404；`useSignalWorkbench.test.ts` FE-12 停轮询重拉）；失败保留候选（`TestSignalResearch_FailedJobCarriesErrorStageAndNoResultID`，无 result_id 键断言）；分页周期隔离（`TestSignalList_PaginationAndValidation`、`TestListSignalReportResultsIsKindAndPeriodIsolated`、`TestSignalReports_ListAndDetail`） | 无 |

### specs/data-enrichment/spec.md（MODIFIED，2 个）

| Requirement | 判定 | 证据 | 缺口 |
| --- | --- | --- | --- |
| 板块 tab「认知工作台」界面 | 已实现+有测试 | `BoardEnrichmentPanel.vue`（信号主视图替换+旧 brief/investigation/legacy/绑定入口卸载，新闻背景/聚焦分析/FinGenius 保留）；`SignalCandidateList.vue`（FE-1 只 emit discover、score 不渲染）+16 用例；`SignalReportView.vue`+27 用例（引用/图表/附录/安全渲染/事后回顾/无 review DOM）；`BoardEnrichmentPanel.test.ts` FE-11 断言 | 旧组件文件未物理删除（有裁决请求备案，DOM 断言保证不在工作台）；不阻塞 |
| 仅手动触发（不挂日报管线） | 已实现+有测试 | 发现/研究全人工、共享 409、等待不持锁（`TestSignalDiscoveryTrigger_RunningBriefConflicts`、`TestSignalResearch_OldBriefRunningBlocksResearch`）；主数据零污染：`SignalResearchStore` 三方法接口（编译级无 lifeline/review 写面）+ NR-2 测试；旧 API 语义隔离（`TestListSignalReportResultsIsKindAndPeriodIsolated` legacy kind 查询报错） | 无 |

### specs/research-data-sources/spec.md（ADDED+MODIFIED，2 个）

| Requirement | 判定 | 证据 | 缺口 |
| --- | --- | --- | --- |
| 显式历史窗口与原参数兼容（ADDED） | 已实现+有测试 | `eia.go:validateEiaWeeks/fetchWindow/fetchEditionForWeek`（1~12 发网前拒、归档版次探测、版次错配显式失败、缓存键 `eia:table1:weeks=N`）；`jodi.go:validateJodiWindow/fetchYearsWindow`（1~5、month 互斥、本年 404 回退一次去重、历史年 gap 不级联、非 404 整呼失败）；缺省行为零变化由 `TestEiaFetchWeeksWindowAssemblesArchivedEditions`（缺省成对形状断言）与既有 JODI 8 用例回归钉住 | 无 |
| 工具适配不改现有工具面（MODIFIED） | 已实现+有测试 | `signalResearchToolNames` 四源白名单仅新 loop（`TestSignalResearch_AllowedToolsPinnedAndLegacyUnchanged`：逐字钉住+`explorationToolNames` 逐字不变+`buildAgentAllowedTools` 不含四源+web_search CheckCall 即拦）；wiring `TestResearchToolsToolSurfaceUnchanged`（四工具名+weeks/years 不入 required） | 无 |

**A 判定分布：14/14 Requirement【已实现+有测试】；0 未实现/偏离。** 唯一能力级缺口为 E-M1（WDI 计算维度）。

## B. 红线合规（10 条逐项）

| # | 红线 | 判定 | 证据（file:symbol） |
| --- | --- | --- | --- |
| 1 | 两阶段人工边界：发现只产候选、无自动衔接、detector 不持 registry | **PASS** | `service/signal_detect.go`：`SignalDetector{airouter,capability}`、`SignalDiscoveryService{detector,builder,repo,now}`——均无 registry 字段（编译级）；discovery job fn 只 DiscoverSignals 后恒返 0 无 result；无任何 discovery→research 调用路径；FE-1 双层断言（组件零 research 出口+composable discover 链零 research 调用） |
| 2 | 40 轮/40 执行预算、每轮一动作、失败计执行、非法/重复不执行但计轮、stop_reason 三分类 | **PASS** | `orchestrator.go:runToolLoop`（maxLoops=40 循环每轮恰一动作）；`signal_research.go:RunLoopAction/CheckCall`（blocked 不执行、markStep 计轮）；LP-2（40 收束 budget_exhausted、第 41 次 `researchCalls==40` 钉死、耗尽仍 compose 恰 1 次落库）；LP-3（混合 40 执行、被拒计算计入）；LP-4（40 轮非法零执行有限收束）；LP-6（取消/LLM 断裂→failed，含「取消或超时」文案，不伪装 budget） |
| 3 | 零评审链：新报告不调 judge/不读 digest/不写 review；旧 review 不破坏 | **PASS** | stub 路由默认分支对 signal_research/compose 之外 operation 直接报错（NR-1 机械断言）；`SignalResearchStore` 接口无 review/lifeline 写方法（编译级）；列表+详情响应体整体断言无 review/judge/approved/digest；`TestSignalResearch_NR_OldReviewRoutesStillRegistered`+既有 review 用例回归绿 |
| 4 | 受限计算：仅三算子、模型不能交 value、单位兼容、base≤0 拒、null 不填 0、4 位 half-up、无任意脚本 | **PASS** | `signal_calculation.go`（白名单 switch、big.Rat、`formatSignalDecimal` half-up away-from-zero 去尾 0）；policy `value_forbidden` 拦截（测试覆盖）；CA-1~7+舍入表（2.00005→2.0001、−1/3→−0.3333） |
| 5 | cutoff：快照冻结、工具结果进 agent 前过滤、附录不截 50 点 | **PASS** | `ResearchCandidate` 只读 `InputSnapshot`+周期一致性拒绝（不重新装配，无 freshness 入口）；`buildSignalResearchRegistry` 包装 Execute→`filterSignalToolResult`（期结束≤cutoff、不可解析剔除、observation_id 打标）先于 agent 历史；`TestSignalResearch_CutoffFilterBeforeAgent`（历史不含 09-25、日志含完整原响应）+60 点不截。注记：EIA 周结日结束时刻取当日零点，与 phase-1「period<cutoff」仅在 cutoff 恰为周结日零点时相差一例（保守方向，2b ⑥③已备案） |
| 6 | 持久化：原子无半批、owner/周期复合 FK、候选不可变、空批次不清旧、running 不持久化 | **PASS** | `CreateSignalDiscoveryBatch` 单事务；迁移复合 FK `fk_board_signal_candidate_discovery`/`fk_topic_enrichment_result_signal_candidate`（直写 SQL 被拒实测）；候选无任何 Update 路径（grep 零命中）；`TestCreateSignalDiscoveryBatchZeroCandidates`/`TestSignalDiscovery_NoSignalKeepsOldCandidates`；状态派生只查 live job，无 running 列 |
| 7 | 幂等互斥：同 board 202/409、已有报告 200 复用、regenerate 才重做、检查与创建在锁下 | **PASS** | `triggerSignalResearch`：Status 前置 409 → 200 already_reported → StartSignal；`analysis_runner.go:launch` 在 `r.mu` 下做槽位冲突检查（检查/创建竞态由锁内原子裁决关闭，RunningJobError→409）；`TestSignalResearch_ConcurrentDuplicateAndIdempotentReuse`（409 携 job_kind、复用零新调用）、`RegenerateStartsNewJob` |
| 8 | 工具面不变量：四源仅 signal_research 白名单、旧 allowedTools 零变化、EIA/JODI 缺省零变化 | **PASS** | `signalResearchToolNames` 逐字钉住；`orchestrator.go:1559 explorationToolNames` 未改动且不含四源；`buildAgentAllowedTools` 前置四源断言；`TestEiaToolWeeksPassThrough`（缺省不传窗）/`TestJodiToolYearsPassThrough`/`TestEiaFetchWeeksWindowAssemblesArchivedEditions` 缺省成对形状+既有 JODI 单月用例回归 |
| 9 | 周期：Asia/Shanghai、未来 400 无 job、半开区间、retrospective 标记 | **PASS** | `models.ShanghaiTZ`（FixedZone +8）；`ParseSignalPeriod` 半开 From/To+未来拒绝；handler 在 StartSignal 之前校验（400 且 board 状态 idle 实测）；`SignalAnalysisMode`→批次 analysis_mode→snapshot/generation_meta→前端「事后回顾」标记（`isRetrospectiveReport`） |
| 10 | 报告契约：四段唯一序、<3000 非空白、引用可解析代码渲染、图表 0~3、attempts≤3、appendix 代码生成不可覆写 | **PASS** | `validateSignalReportDraft`（恰四段按序、`visible>=3000` 拒含恰 3000 边界、`resolveSignalRef` 全 token 校验、charts ≤3+兼容+折线升序重排 null 保留）；`signalComposeMaxAttempts=3`、retries=attempts−1；draft 无 appendix 字段（解析层丢弃）+`TestSignalCompose_AppendixIsCodeGenerated` |

**红线 10/10 PASS（B5 带已备案边界口径注记）。**

## C. 测试证据抽查（每个抽查测试：实质/薄弱/证据）

| # | 测试 | 判定 | 证据 |
| --- | --- | --- | --- |
| 1 | S1 达标只生成候选：`TestSignalDiscovery_EndToEndDiscoveredPath`（handler） | **实质** | 种子泳道/日报/切片+真实 `SignalDiscoveryService`+stub LLM；断言 `llm.calls==1`（detect 恰一次=取数/计算/compose 0 的机制证据）、job 无 result_id 键、候选行 evidence_refs 指向切片 id、无材料板块零 LLM 直落 no_signal |
| 2 | S2 单候选：`TestSignalResearch_OnlySelectedCandidateRuns` | **实质** | 双候选点击 A：stub.calls==1 且 lastCand==A；列表派生 A=reported（latest_result_id 对得上）/B=pending |
| 3 | S3 四十轮收束：`TestSignalResearch_FortyRoundsBudgetExhaustedNotFailure` | **实质** | 40 次 call_tool 决策；断言 `researchCalls==40`（第 41 轮不发生，stub 越界即错双保险）、stop_reason=budget_exhausted、SourceCalls=40、compose 恰 1 次、result 落库、compose prompt 含 budget_exhausted 披露指令 |
| 4 | S4 同单位差值/百分比：`TestSignalCalc_DifferenceSameSeriesAdjacentPeriods`+`PercentChangeHalfUpFourDigits` | **实质** | 断言 Value=="-1"/Unit=="MMbbl"/Expression=="c1:o2 - c1:o1"；−0.2381% 精确断言；配套舍入表驱动（2.00005→2.0001 等 10 例） |
| 5 | S5 直接可读无 review：`TestSignalResearch_NR_NoReviewNoNewsPollution`+`TestSignalReports_ListAndDetail` | **实质** | stub 默认 operation 分支即错（judge/digest 一旦发生即红）；详情响应体小写全文不含 review/judge/approved/digest 四词断言；input_snapshot 透传断言 |
| 6 | PG 直写拒绝：`TestSignalReportDBConstraintsRejectIllegalShapes`+`TestCreateSignalDiscoveryBatchAtomicSave`（DB-1 段） | **实质** | `testutil.SetupTestDB`（testcontainer 隔离 PG，golden schema 含迁移后约束，非 SQLite）；8 种非法形状 raw SQL INSERT 逐一 require.Error（未知 kind/缺粒度/缺周期/月 13/topic scope/legacy 带周期列/缺 source/跨板块候选/错周期候选）；合法行对照 require.NoError |
| 7 | cutoff 过滤：`TestSignalResearch_CutoffFilterBeforeAgent` | **实质** | 08-07 保留+09-25 剔除；第 2 轮 user 消息断言含 `c1:o1`/不含 09-25（agent 历史只见筛选集）；附录 kept/dropped filter_meta；工具日志 ResultFull 含被剔除观测（完整原响应） |
| 8 | job404 停轮询：`useSignalWorkbench.test.ts` FE-12（fake timers） | **实质** | 404 后断言不再发后续轮询、错误提示、重拉候选/报告列表（12 用例全绿实测复跑） |
| 9 | 事务回滚无半批：`TestCreateSignalDiscoveryBatchRejectsInvalidWithoutHalfBatch` | **实质** | 主键冲突构造第二批第二行失败→断言批次行 0、候选行 0（半批不可能） |
| 10 | 旧库迁移并存：`TestBoardSignalMigrationLegacyRowsKeepNullColumns` | **实质** | 先 DROP 三列→插 legacy 行→跑迁移→旧行三列 NULL+sectors JSONEq 原样；legacy 形状迁移后仍可插入 |

repository/database 测试全部经 `testutil.SetupTestDB` 隔离 testcontainer PostgreSQL（`-short` skip 标记存在、无 SQLite import；本次审查实际运行全绿，见核实记录）。**未发现空跑/恒真/被 skip 冒充的用例。**

## D. 一致性三角（后端 handler ↔ front/app/api/boardSignals.ts ↔ docs/reference/api/board-signals.md）

逐点核对结果：
- 派生状态枚举 `pending|researching|reported`：handler `SignalCandidateStatus*` ↔ TS `SignalCandidateStatus` ↔ doc GET /signals 字段表 —— **一致**。
- 候选行 13 字段（id/discovery_id/granularity/period/signal/why_it_matters/research_question/evidence_refs/score/rationale/discovery_created_at/status/latest_result_id 可空）：三方 —— **一致**。
- job 字段（job_id/job_kind/scope/target_id/running/started_at/finished/error/result_id + phase/outcome/error_stage/granularity/period/discovery_id/candidate_id/candidate_count 全 omitempty；discovery 永无 result_id、research 仅 succeeded 有、no_signal 省略 candidate_count）：`AnalysisStatus` ↔ `SignalAnalysisJobStatus`（全可选） ↔ doc 时机表 —— **一致**。
- research 响应：200 `already_reported{result_id}` / 202 `started{...,candidate_id}` / 409 data=running job 身份 / 404 / 400：三方 —— **一致**。
- 报告行/详情（列表含 sectors 不含 tool_calls/input_snapshot；详情追加两者；granularity/period/source_signal_id 非空、semantic_board_id 可空）：`serializeSignalReportSummary/Detail` ↔ `SignalReportRow/SignalReportDetail` ↔ doc —— **一致**。
- sectors payload（schema_version=2 五块；sections 四段 implication 五字段+direction 枚举；charts kind/refs；appendix calls/calculations/gaps 字段；generation_meta 9 字段含 stop_reason 枚举）：`signal_compose.go` 类型 ↔ TS 接口 ↔ doc —— **一致**。
- 引用渲染约定（token 原样存储、数值前端按附录渲染、raw_value 源精度、missing 不转 0）：实现 ↔ doc ↔ `signalReport.ts` —— **一致**。

**不一致清单（仅 1 条，Low）：**
1. doc `board-signals.md` research 状态表将「板块未开启 400」与「body 非法 400」合并列为第一判定步（标注"按此顺序判定"），实现实际顺序为 body 400 → 候选归属 404 → 开关 400 → 409 → 200 → 202（`handler/signal_research.go:triggerSignalResearch`）：跨板块候选落在未开启板块时返回 404 而非 400。design §7 顺序（归属/开关→互斥→复用）与实现一致，仅 doc 表述有歧义。

## E. 发现清单（High/Medium/Low）

**High：0 条。**

### Medium

**M1. WDI 观测期间键不匹配：wb_wdi 的 difference/mean 全域被错误拒绝（承诺能力降级+测试空白）**
- 位置：`backend-go/internal/dataenrichment/service/signal_research.go` `signalCalcObservationFromMap` wb_wdi 分支（`entry.Period, _ = obs["period"].(string)`）；对照 `datasources/sources/wdi.go` `WDIObservation`（JSON 键为 `year`，无 `period`）。
- 问题：WDI 观测入计算索引后 `Period` 恒为空串。后果：① 同指标不同年的 `difference` 命中「同一系列同一期间，差值无意义」被误拒；② 任意多年 `mean` 在第二个输入即触发「期间集合含重复期间 ""」被误拒；③ WDI 折线图期间排序退化为输入原序（sortKey 全 ""）；④ compose 账本视图里 WDI 行期间显示为空。`percent_change` 不比较期间仍可用。过滤层（`signalObservationPeriodEnd` 读 `year`）不受影响，cutoff 过滤正确。
- 复现/证据：静态比对两文件键名即可复现；`signal_calculation_test.go` 全部用例仅用 EIA/JODI fixture，无任何 WDI 计算路径用例（这也是它未被发现的直接原因）。红线4/Requirement「代码计算与公式证据」对四源之一的能力未兑现（以"拒绝"而非"错误数值"形式呈现，故不评 High）。
- 建议修法：wb_wdi 分支 `entry.Period` 读 `obs["year"]`（一行），补一条 WDI 同系列不同年 difference/mean 用例 + 一条 WDI 混入 EIA 拒绝用例。

**M2. 工具日志 Outcome 对类型化源错误误标 ok（持久化日志保真缺口）**
- 位置：`backend-go/internal/dataenrichment/service/orchestrator.go` `toolResultErrorText`（只识别顶层 `"error"` 字符串键）vs `datasources/wiring/tools.go` `marshalResult`（类型化错误序列化为 `{"error_code","message","detail"}`，无 `"error"` 键）；信号链账本判错用的是 `signal_research.go:signalSourceErrorText`（能识别 `error_code`，附录 `status=error` 正确）。
- 问题：研究循环里真实失败的源调用（如 SOURCE_UNAVAILABLE/INVALID_ARGUMENT 经 wiring 类型化输出）在 `result.tool_calls` 记录上 `Outcome="ok"`，与附录账本 `status="error"` 及 gap 记录相矛盾——OB-2「完整账本」的持久化侧字段失真（完整原响应仍在 ResultFull 可追溯，故非 High）。LP-5 断言了附录 status 而未断言记录 Outcome，因此未暴露。
- 复现/证据：读 `runToolLoop` Outcome 判定与 wiring 错误帧形状；用 stub 返回 `{"error_code":...}` 帧跑一次研究，检查落库 `tool_calls` 记录。
- 建议修法：在信号研究包装层把类型化错误帧归一（附 `"error"` 键）或在循环 Outcome 判定处兼查 `error_code`（注意后者为共享路径，需回归 investigation 用例；前者改动面更小）。

### Low

**L1. `selectSignalBackgroundSummary` 不按 granularity 过滤**
- 位置：`service/signal_material.go:205`（对照 `selectSignalPeriodSummary:192` 有 `r.Granularity == granularity` 检查）。
- 问题：泳道归档含 month/year 两档时，背景选择仅按 Period 字符串比较；多数场景字符串序自然排除异粒度（year"2026" vs month"2026-08"），极端组合下可选到异粒度摘要作背景（输出带 Granularity 字段可辨，非静默冒充）。`TestSelectSignalBackgroundSummaryNeverPastCutoff` 仅用同粒度 fixture。
- 建议：补 `r.Granularity == granularity` 条件（一行）+ 混粒度用例。

**L2. doc research 判定顺序与实现存在一处置换（见 D-1）**
- 位置：`docs/reference/api/board-signals.md` POST /signals/:candidateId/research 状态表；实现 `handler/signal_research.go`。建议 doc 表拆开「body 非法 400」与「开关未开启 400」的先后。

**L3. `listSignalCandidates` 内联重复实现 before_id/limit 解析**
- 位置：`handler/signal_discovery.go:222-247` vs `board_enrichment_handler.go:271 parseBeforeID/parseListLimit`（2b 为报告端点新增的共享 helper，候选列表未复用）。行为当前等价，纯漂移风险。建议候选列表改用共享 helper。

**L4. 候选列表派生状态 N+1 查询**
- 位置：`handler/signal_discovery.go:deriveSignalCandidateStatus`（每行 2 次查询：live job 查询+最新报告查询），limit≤100 时单请求最多 200 次往返。单实例单用户产品可接受；如列表变慢可批量取 `source_signal_id IN (...)` 的最新报告。

**L5. EIA cutoff 边界口径备案（非缺陷，收尾确认项）**
- 位置：`signal_research.go:signalObservationPeriodEnd`（EIA 周结日结束时刻=当日零点）vs phase-1 契约「period < cutoff」。仅当 cutoff 恰为周结日零点整时相差一例，方向保守（少保留），已在 phase-2b ⑥③备案。收尾时确认即可，无需改动。

## 总结

**整体判定：可进入收尾；建议先完成 M1/M2 两处定向修复（均为小改动）再归档。**

- A：14/14 Requirement 全部【已实现+有测试】，无未实现/根本性偏离。
- B：红线 10/10 PASS（B5 有已备案的边界口径注记，不构成违反）。
- C：抽查 10 个关键测试全部实质，repository/database 确认隔离 PG（非 SQLite）；本审查实测复跑后端 5 个目标命令+前端 3 个目标命令全绿。
- D：一致性三角基本全对齐，仅 1 处 doc 判定顺序表述歧义（L2）。
- E：High 0 / Medium 2 / Low 5。最重要的三条：**M1（WDI difference/mean 被全域误拒——四源之一承诺能力未兑现且零测试覆盖）**、**M2（工具日志 Outcome 把失败源调用标 ok——持久化账本字段失真）**、**L1（背景摘要不按粒度过滤——材料保真小缺口）**。

修复优先级：M1（一行修+补 WDI 计算用例）→ M2（错误帧归一+补记录 Outcome 断言）→ L2（doc 一行）→ L1/L3（顺手小修）→ L4/L5（收尾确认即可，可不改）。另外提醒：tasks 6.T4/6.T5/8.4~8.6 人工验收项尚未执行（阶段报告已如实标注），属收尾必做项而非代码缺陷。

---

### 核实记录（本审查实际执行的只读命令与结果）

| 命令（cwd=backend-go / front） | 结果 |
| --- | --- |
| `go test ./internal/dataenrichment/service/ -short -run "Signal" -count=1` | ok（detect/material/research/calculation/compose 全量） |
| `go test ./internal/dataenrichment/handler/ -short -run "Signal" -count=1` | ok（discovery+research+list） |
| `go test ./internal/datasources/... -short -count=1` | ok（3 包：datasources/sources/wiring） |
| `go vet ./internal/dataenrichment/... ./internal/datasources/...` | 无输出（0 问题） |
| `go test ./internal/dataenrichment/repository/ -run "TestCreateSignalDiscoveryBatch\|TestListSignalCandidatesByPeriod\|TestSignalEvidenceRefHelpers\|TestCreateSignalReportResult\|TestSignalReportDBConstraints\|TestSignalReportVersions\|TestListSignalReportResults" -count=1` | ok（隔离 testcontainer PG，5.8s） |
| `go test ./internal/platform/database/ -run "TestBoardSignal" -count=1` | ok（隔离 PG，11.6s） |
| `pnpm test:unit app/features/tags/composables/useSignalWorkbench.test.ts --maxWorkers=2` | 12/12 passed |
| `pnpm test:unit app/features/tags/components/SignalCandidateList.test.ts app/features/tags/components/SignalReportView.test.ts --maxWorkers=2` | 43/43 passed |

除本报告文件外零写入；未执行任何改变状态的命令。预存脏改甄别：datasources 目录整体未跟踪（含预存 change 与本 change 增量混存），本报告以「本 change 声明的改动面（eia weeks/jodi years/wiring 新参数）」为审查口径逐文件精读核对；wire.go/orchestrator.go/handler.go/models.go/repository.go/board_enrichment_handler.go 的 diff 逐 hunk 核对均为本 change 增量（wire.go 中 `toolRegistry.Register(wiring.BuildTools(...))` 一段为预存 change 注释所注内容，本 change 仅追加 signalDiscovery/signalResearch 装配段）。
