# Tasks: board-level-deep-analysis

> 2026-08-28 用户验收推翻了原“自动立命题→支持性检索→论文式长文”主线。本清单保留可复用的已完成基础设施，并以未完成任务跟踪“版块简报→用户选题→多假设调查”的修订实现；旧任务历史由 git 保留。

## 0. 已完成且继续复用的基础设施

- [x] 0.1 版块 scope result、semantic_board_id、nullable topic id、lane source_type/kind 与旧枚举兼容已落地；已有数据库与 repository 测试可作回归基线
- [x] 0.2 态势卡（week→month→实质 section 指纹）、month/year 补全门（缺行首建/72h 重算/40 次上限）、质量排序与失败降级已由现有实现落地；已有 service/DB 测试可作回归基线
- [x] 0.3 版块触发/结果 API、历史列表、lane 下钻、单泳道 prefill_lens 与 legacy 报告组件已落地；新实现须增量演进而非删除兼容路径
- [x] 0.4 append-only result、QA 追问、review 隔离与 ai_call_logs SessionID 链路已存在；简报和调查继续复用
- [x] 0.5 修订版白盒用例已记录于 `test-cases.md`；实现时按 M1-M10 用例先行

## 1. 结果种类与父子数据模型

- [x] 1.1 先新增迁移测试：旧 topic/board 行分别回填 `topic_analysis`/`legacy_board_analysis`，调查 parent 必须指向同版块 brief且带 question_key，回滚不改旧 sectors；验证 `go test ./internal/platform/database -run 'ResultKind|BoardInvestigationParent|QuestionKey'` 先红后绿
- [x] 1.2 新增 `result_kind`、nullable `parent_result_id` 与 nullable `question_key` 迁移/model/约束；question_key 由 trim+空白折叠后的问题文本 hash 生成，旧 JSON 原样保留；验证 1.1 测试通过并抽查 legacy 行数量不变
- [x] 1.3 repository 增加按 kind 查询、简报父子调查写入与同版块父结果校验；验证 repository 单测覆盖 kind 隔离、跨版块 parent 拒绝、一 brief 多 investigation
- [x] 1.4 result 序列化增加 kind/parent，保持无新字段的历史行可读；验证 handler/repository 兼容测试通过

## 2. 分析方法卡库

- [x] 2.1 先写方法选择与阶段隔离单测：0 张可正常调查、最多 2 张、avoid_when 优先、简报 prompt 零方法正文、超预算按整卡舍弃；验证 `go test ./internal/dataenrichment/service -run 'AnalysisMethod|BoardBriefNoMethod'` 先红后绿
- [x] 2.2 新建 `analysis_methods` model/repository/migration，字段含 summary/selection_meta/content/enabled/deleted_at；实现 `/analysis-methods` CRUD/启停/软删除；验证 repository + handler 创建/更新/停用/软删除测试通过
- [x] 2.3 将旧 `reference_roles` 非破坏复制为 disabled legacy 方法记录，保留旧表与原文；《内部看美国》升级后不得自动注入；验证迁移测试比较原文 hash、enabled=false、旧表仍存在
- [x] 2.4 实现方法选择器：仅按用户问题+父简报元数据选 0-2 张，再用选中方法辅助生成假设；选中后加载正文先过固定修辞清洗（金句/强制“不是…而是…”句式/人格口吻模仿等指令行确定性剔除，否定性边界说明保留），method_refs 固化 id/title/content_hash（原始 Content 字节，部分清洗不变）；清洗行数/原因码/整卡舍弃与实际注入正文留痕随调查写入 input_snapshot/ai_call_logs；选择理由、正文与预算舍弃同样写入；验证 2.1 与“方法删除后历史可回放”测试通过（4.5/4.6 落库链路验收：`board_investigation_persist_test.go` 断言快照含候选/理由/机码/实际注入正文，方法软删后历史可回放、被过滤原文与超限正文不落快照）
- [x] 2.5 移除旧参考角色对 board/topic interpret/tool/analyze 的全局注入，保留只读兼容 API 一个版本；验证 grep 仅兼容层引用旧注入函数，简报、调查及单泳道 prompt snapshot 均不含作者画像
- [x] 2.6 前端设置页由“参考角色”改为“分析方法”，表单覆盖适用/禁用/证据/失败模式，legacy 项默认停用并提示人工整理；验证组件单测覆盖 CRUD、启停和 legacy 提示

## 3. 版块简报链

- [x] 3.1 先写 `board_brief` parser/prompt 用例：无统一关系、并行趋势、全 sparse、幽灵 lane、非法关系枚举、坏 JSON 重试与机械降级、输出数量上限；验证 `go test ./internal/dataenrichment/service -run 'BoardBrief'` 先红后绿
- [x] 3.2 新增 `data_enrichment.board_brief` Operation 与简报 schema，输入仅为补齐后的态势卡 + 同 kind 历史 review，禁止 web/fetch 工具和方法卡全文；验证 prompt snapshot 与 3.1 测试通过
- [x] 3.3 将现有版块 trigger 编排改为 freshness→cards→brief→persist，不再自动运行 board_interpret/研究循环/论文 analyze；失败重试后机械降级只产观察，不造关系；验证 mock 断言默认触发 LLM/工具调用次数符合契约
- [x] 3.4 实现简报关系校验与 lane 白名单清理，质量信号只影响排序不充当关系证据；验证 context_only/possible_causal/unclear 边界测试通过
- [x] 3.5 review judge 按 `board_brief` 比较 observations/relationships/uncertainties，跳过 legacy thesis；验证第二份简报 review 测试且 lifeline 前后快照一致

## 4. 用户选题与多假设调查链

- [x] 4.1 先写 hypothesis parser 用例：2-4 假设、零假设必有、全宏大假设自动重试/补 H0、question generated/custom、所有假设可证据不足；验证 `go test ./internal/dataenrichment/service -run 'BoardHypothesis'` 先红后绿
- [x] 4.2 先写研究计划用例：单一共享循环、至少一个中性查询与一个反证/替代查询、重复工具拦截、外部工具失败记录 gap、不得只用结论词搜索；验证 `go test ./internal/dataenrichment/service -run 'BoardInvestigationResearch'` 先红后绿
- [x] 4.3 按“方法选择→假设生成”顺序实现 `data_enrichment.board_hypothesize`：方法只基于父简报+问题选择，随后产竞争假设与证据需求而不预选赢家；验证 4.1、无选择循环与方法引用快照测试通过
- [x] 4.4 实现一个共享 investigation tool loop，统一服务全部假设并区分 support/counter/gap；内部 lane 优先但允许按问题调用 web_search/fetch_page；验证 4.2 与 max_loops 测试通过
- [x] 4.5 实现 `data_enrichment.board_synthesize`：assessment 五态、可改写/合并/推翻假设、允许 H0 最可信或全部 insufficient；取消固定机制层/历史类比/system_reframe；验证 synthesis schema 与反证保留测试通过（`board_investigation_synthesis_test.go`：五态枚举/quote 机械核对/method 不作 source/幽灵 lane/悬空极性/同向新闻 high 降级/无 argument/depth）
- [x] 4.6 持久化 `board_investigation` 子结果、tool_calls、method_refs 与 input_snapshot；中途失败不得留半成品；验证一 brief 多调查、跨版块 parent 拒绝、原子性测试通过（`board_investigation_persist_test.go` PG 集成：全链落库/0-LLM 预检/同题重跑/synth 失败 0 行/父简报不变/软删方法可回放/舍弃机码不泄原文；review 4.7 留待后续）
- [x] 4.7 调查 review 仅比较相同 `parent_result_id + question_key` 的重跑；generated/custom 问题使用同一规范化 hash，不同问题不互比；验证 review kind/question 隔离测试通过（`board_investigation_review_test.go` 投影纯函数 + `board_investigation_review_chain_test.go` PG 集成：首份 0 judge/同链重跑 judge+review 行/generated-custom 空白变体同 key/不同问题-parent-版块及 legacy-brief 夹行不污染/judge false 不写/chat-parse 失败 non-fatal 当前行仍在/恶意超长 affected_context 强制空/父-当前-lifeline 不可变；repository 严格链查询见 `result_kind_repository_test.go` TestGetPrevBoardInvestigationByQuestion）
- [x] 4.8 调整单泳道 prompt：去掉旧作者画像全局注入和“至少三类证据”配额，不引入 board hypotheses schema，冲突材料留在既有 evidence/boundary；验证 orchestrator prompt 与旧结果解析回归测试通过
- [x] 4.9 真实 UI hardening：综合响应仅缺根对象最后一个 `}` 时做可证明的单终止符修复并记录 generation repair reason；内部截断/错配/未闭字符串/尾随正文/缺 lane_refs 继续严格拒绝。M12 红→绿、service race/lint/vet、独立 review 均通过；真实 job `65f647365f3ec53e89dca411` 在 16m15s 成功落 result #12（该轮 fallback JSON 本身合法，`attempts=1`、repair_reason 空，说明正常路径未被修复逻辑误伤）
- [x] 4.10 fallback evidence hardening：lane evidence 在 ref 缺失时安全归一 `lane_id`，仍过父简报白名单；清洗后合并证据极性与 hypothesis 引用，supported/refuted/weakened 不得失去全部对应证据。M13 红→绿、独立 review 无 Critical/Important；真实 job `c37454d16c0baeb841420ae5`（result #13，18m53s 落库）验证：glm 空→empty_response fallback qwen，qwen 原始响应 5 条 lane 证据全部无 ref 只带 lane_id（response_snippet 存证），持久化 ref 全为十进制且全∈父简报白名单，evidence_chain=5、h1 weakened 持反证 e4/e5、h3 supported 持支持引用；UI 证据/反证可展示且合法 lane 可下钻（详 7.5）

## 5. API 与前端认知工作台

- [x] 5.1 先写 handler/API 契约测试：原 trigger 返回 board_brief job、新 investigation trigger 支持 generated/custom 问题、job_id/job_kind 轮询、同 board 跨 kind 409、防断连、kind 过滤、父结果校验与旧详情兼容；验证目标 handler 包测试先红后绿（`handler/board_investigation_api_test.go` + 更新 `board_enrichment_handler_test.go`/`handler_test.go`/`analysis_runner_test.go`，`go test ./internal/dataenrichment/handler/` 含 -race 全绿）
- [x] 5.2 扩展 analysis runner：任务记录唯一 job_id/job_kind，按 job_id 查询，active key 仍按 board 串行；409 返回当前任务身份，按 board 状态入口保留用于重进恢复；验证 runner 并发/完成/错误/进程内恢复测试通过（`RunningJobError` 携完整身份、`StatusByJobID` 终态可查、`Status(scope,id)` 返最新 job）
- [x] 5.3 实现 `POST .../analysis/investigations/trigger` 与 `GET .../results?kind=`，现有 trigger 切换简报语义并返回 board_brief job；验证 5.1 通过（trigger 一律 202；status 支持 `?job_id=` 精确查，未知 404；409 响应 data 携当前 job 身份）
- [x] 5.4 先写 `BoardBriefReport` 组件测试：summary/观察/关系/不确定项/0-4 问题、无关系正常态、lane 下钻、深入调查 payload、自填问题；验证 Windows cmd 目标 unit 测试先红后绿（`BoardBriefReport.test.ts` 17 例：各区块渲染/无关系/0问题/lane emit prefill/generated+custom payload/空白+2000 rune 边界/all_sparse/degraded）
- [x] 5.5 实现简报主视图与按 job_id 的状态恢复；board_investigation 完成不得误刷新为新 brief，“分析板块”不再出现自动论文分析文案；验证组件测试和 API mock 通过（`useBoardEnrichment.test.ts` 14 例：202/409恢复/按job_id/brief完成选择/investigation不冒充/重进恢复/error+404停止/瞬时错误续轮/unmount清timer/legacy选择；`BoardEnrichmentPanel.test.ts` 10 例：简报主视图默认/legacy标注/investigation占位不冒充/kind标签/在跑文案；`client.test.ts` 4 例：409 data+status/普通错误/成功extras/网络异常）。review 修复（同勾追加）：导出 `stopBoardPoll`，panel bootstrap 开头先停旧 timer 清 activeBoardJob/triggering；轮询改串行 setTimeout（仍 3s）+ generation/boardId/jobId 身份守卫，迟到响应丢弃（不 stop/notify/重拉/选中旧板块）；`syncBoardAnalysisStatus` idle/finished 且未入轮询时清 triggering（409 无 data 兼底不卡按钮，finished 不误当 running）；历史下拉补 aria-label="历史报告"（`useBoardEnrichment.test.ts` +5 例切板块隔离/迟到不杀新job/串行无并发/409 兑底×2；`BoardEnrichmentPanel.test.ts` +2 例 bootstrap 停轮询顺序/aria-label）；二次 review 修复（同勾追加）：composable 新增 activateBoardContext/deactivateBoardContext 统一 board view context（activeBoardId+viewEpoch+disposed），panel bootstrap 最前 activate（停旧 poll+epoch++）、onUnmounted deactivate；trigger/sync/loadBoardAnalysisResults（含 boardResults/selected 写点）+bootstrap 链 loadTopics/loadDataSources 全部迟到守卫（dispose/切板块/epoch 变化→静默丢弃，不 start poll/不 toast/不写 refs，loading 重置同守卫防提前清新板块在途态）（`useBoardEnrichment.test.ts` +5 例 deferred 迟到：unmount后trigger202/sync running、A trigger/sync 迟到切B、A finished reload 迟到切B；`BoardEnrichmentPanel.test.ts` bootstrap 顺序断言改 activateBoardContext）；最终 review 修复（同勾追加，5.6+ 不做）：composable 新增 `syncTopicAnalysisStatus(topicId)` topic 档重进恢复——捕获 board epoch+topicId，await 后失配（卸载/切板块/换 topic）静默丢弃，旧 board 迟到 status 不写入新 board；running → 恢复 triggering+startTopicPoll，idle/finished/查询失败且 topicPollCtx 为空才清 triggering（不误停别 topic 新 poll）；panel bootstrap 改 loadTopics 先行 await、board 三件套同步启动，selectedTopicId 确定后才 sync 当前 topic（同 board 刷新/切走再回/首次进入 running topic 均恢复，无 topic 不调）；trigger topic 409 识别 `res.status===409||error` 文案（后端冲突体携 job 身份），仍按 scope 入口恢复轮询、终态文案/重拉保持，不重构 job_id poll（`useBoardEnrichment.test.ts` +8 例：sync running 恢复+终态重拉/切A→B→A恢复/sync迟到切B不启动/finished不清误当running/idle骨架清/trigger并发不清/unmount迟到不重建timer/409携身份恢复；`BoardEnrichmentPanel.test.ts` +1 例 bootstrap sync 接线+无topic不调）；收尾修复（同勾追加，5.6+ 不做）：真实链路集成测试 `BoardEnrichmentPanel.bootstrap.test.ts` 补 `listDataSources` mock（此前 boardLoads Promise.all 拒绝→末尾 sync 永不执行+2 unhandled rejection）、40 拍微任务 flush 改 `vi.waitFor` eventually（fake timers 自动推进虚拟时钟反复排空，不拍数）；`BoardEnrichmentPanel.test.ts` 时序修复：Critical 重排后 sync 在 `await boardLoads` 之后，单个 nextTick 不再够排空，改宏任务屏障 `setTimeout 0`（mock 均立即 resolve，一次宏任务边界确定性跑完整条 bootstrap 微任务链），且 beforeEach 重置模块级 `selectedTopicId`（旧时序下 sync 接线例首断言即挂、其置 null 分支从未执行而掩盖跨测试泄漏，修复后暴露，消除顺序耦合）；关键断言未弱化：loadAll<sync 顺序、bootstrap 结束 running poll 每 3s 持续、finished→「增强完成」+重拉结果表+停轮询、无 topic 不启动轮询；终审修复（同勾追加，5.6+ 不做）：loadTopics 失败语义与切板块清场——activateBoardContext 仅 boardId 真实变化时同步置空 selectedTopicId+clearTopicDisplayRefs（表1/2/3/详情/QA/辩论+loading/error 态，加载期间无混合视图）；loadTopics 当前视图失败→停 topic/debate 轮询+selected 置空+清 refs+topics=[]+error 置位（bootstrap 读 null 自然跳过 loadAll/sync，面板按无 topic 分支）；同 board 成功刷新合法选择保留、refs 不清由后续 loader 刷新；失败响应迟到已被 epoch 守卫丢弃不误清新板块（`useBoardEnrichment.test.ts` +4 例：A→B 切板块清场/B 失败不回填、同 board 失败清场停轮询、同 board 成功保留选择、A 失败迟到切 B 守卫丢弃；test-cases.md 登记 M9.17-M9.20）
- [x] 5.6 先写 `BoardInvestigationReport` 测试：首屏有限结论、支持/反证/gap 分区、hypothesis assessment、证据展开、lane 下钻；验证目标 unit 测试先红后绿（`BoardInvestigationReport.test.ts` 22 例：空态/loading/首屏 question+来源标签+conclusion 四字段+assessment 摘要/首屏不铺证据细节（quote/机构/lane_note 不进首屏）/sectors 塞入 argument+depth 旧字段忽略/五态中文标签/H0 标记+最可信/全 insufficient 正常态（无 role=alert）/support-counter-gap 三分区独立展开/悬空证据引用原样显示不崩/证据展开 quote+institution+date+web、page URL 可点（target=_blank noopener）/空证据链直白文案/lane 下钻 prefill=具体调查问题+lane_note+quote+反证假设说明（含评估态）/幽灵 lane 与非法非正整数 ref 不渲染入口不 emit/prefill ≤400 rune 截断/custom 来源「自填」标签/method_refs 留痕/parent_briefing_id 溯源）
- [x] 5.7 实现调查渐进展示，移除新结果的 argument/depth 重复长文；legacy 结果继续路由旧组件并显示“旧版分析”；验证新旧三种 result_kind 分派测试通过（`selectedResultView` 三 kind 路由：board_brief→BoardBriefReport / board_investigation→BoardInvestigationReport / 其余（含无 kind 旧行）→legacy 横幅+BoardAnalysisReport；`BoardEnrichmentPanel.test.ts` 22 例含：主视图默认简报、legacy 标注、investigation 选中渲染调查报告不冒充、历史下拉 kind 标签、job_kind 在跑文案分流、investigate 事件接线、investigationRunning 透传/不误传；`useBoardEnrichment.test.ts` 54 例含：investigation 完成 reload+选中 result_id 调查行但 latestBoardBrief 仍认 board_brief、默认选中最新 board_brief、仅 legacy 不选中、无 kind 兜底、triggerBoardInvestigation 202 按 job_id/custom payload/409 恢复（跨 kind）/400、404 不轮询/在途切板块迟到守卫）；Windows cmd 目标 6 文件 147 例全绿 + typecheck 通过）
- [x] 5.8 下钻 prefill 改为具体 observation/question/evidence note 且允许修改；验证前端 payload 与后端透传测试通过（brief observation/relationship/研究问题/lane_refs 与 investigation evidence 统一走 `handleDrillLane`：校验 lane 属当前板块 topics → 选中泳道 + 展开聚焦区 + prefill 写入可编辑 textarea + 滚动定位 `#focus-analysis`，不自动触发；`BoardEnrichmentPanel.test.ts` 下钻三例：observation prefill 写入不自动触发/investigation prefill 写入后用户改值 → `triggerEnrichment(101, 改后 lens)` 精确透传/幽灵 lane notify 警告且不误选不展开；`BoardBriefReport.test.ts` 各 lane 引用 prefill=statement/explanation/note/question；后端透传 `TestTopicTrigger_PrefillLens`（handler，prefill_lens→EnrichTopicLens）绿）
- [x] 5.9 调查零证据空态文案已改为“没有通过核验、可展示的证据”，不再断言研究没有采到材料；组件红→绿，Windows 22/22 + typecheck、WSL lint 通过，独立 review 无 Critical/Important，证据展开/URL/lane emit 回归不变

## 6. 旧行为退场与兼容

- [x] 6.1 停止新调用 `board_interpret`、旧 board agent directions 与 boardAnalyze 论文分支，但暂留 parser/组件用于 legacy 读取；验证新 trigger 调用链测试中旧 Operation 次数为 0（`service/enrich_board_test.go` `TestEnrichBoard_TriggerChainNeverRunsLegacyWritePath`：`data_enrichment.board_interpret`/`tool_use`/`analyze` 三 Operation 计数=0、恰好 1 次 board_brief、落库仅 board_brief 行；`grep boardInterpret enrich_board.go` 仅剩注释残留，无调用点；本轮 `go test ./internal/dataenrichment/service -run 'TriggerChainNeverRunsLegacyWritePath|ReviewJudgeSkipsLegacyPrev'` 绿）
- [x] 6.2 历史 `legacy_board_analysis` 列表/详情/QA/渲染回归；验证 fixture 覆盖 thesis/argument/depth 旧 JSON，无数据改写（`handler/board_legacy_read_compat_test.go` `TestBoardLegacyReadCompatibility`：详情 sectors 五字段 thesis/candidates/argument/depth/lane_refs 原样透传、读取路径前后 `legacyFingerprint` 字节指纹不变+行数不变、跨板块 404、kind 过滤；`TestBoardLegacyQAAppendOnly` QA append-only；`board_qa_handler_test.go` `TestBoardQA_AllKindsCanAskAndList`/`LegacyAskListSedimentImmutable`/`OwnershipMatrix`/`TestListQA_IDORProtection`（topic 档 listQA 跨 topic 404、board 档 result 借 topic 路由 404）；前端 legacy 分派渲染见 5.7 `selectedResultView`；本轮 handler 包目标测试 `-race` 绿；QA guard hardening：`board_enrichment_handler_test.go` `TestBoardEnrichmentRoutes_DetailScopeMismatch`（脏 scope 行 detail 404 不泄漏、列表不泄漏、正常行 200 对照）+ `handler_test.go` `TestAskQA_IDORProtection`/`TestListQA_IDORProtection`/`TestSedimentQA_IDORProtection`（topic 档 QA 迟到切 topic 均拒 404）+ `board_qa_handler_test.go` `TestBoardQA_OwnershipMatrix`（同 board 换 result sediment 404、被拒组合零 flag 翻转）；映射登记 test-cases.md M9.25/M9.26；本轮前端 Windows `nuxi typecheck` exit 0、WSL `pnpm lint` 0 error（5 warning 均非本 change 文件））
- [x] 6.3 删除默认启用的作者画像 seed 行为或将后续 seed 迁移为 disabled legacy；验证全新数据库初始化后不会有 enabled 作者风格 profile（新增 `20260831_0001` `referenceRoleSeedRetireMigration`：pristine seed 行按身份钉死（content+title 冻结字节）翻 disabled，用户已编辑 content/title 的同名行不覆盖仍 enabled；`database/reference_role_seed_retire_migration_test.go` `TestReferenceRoleSeedRetireMigration` 覆盖 fresh（seed 重开后再跑仍 disabled）/edited（用户行不翻转）/idempotent（重跑状态不变）三态 + 升级后 `analysis_methods` 无 enabled legacy 记录；`TestAnalysisMethodLegacyCopyMigrationPreservesBytesAndIsIdempotent` 原文字节保留幂等；本轮非 -short PG 集成绿）

## 7. 测试

- [x] 7.1 后端影响包：`cd backend-go && go test ./internal/dataenrichment/... ./internal/platform/database/...`，全部通过（2026-08-31 本轮无 `-short`，Docker PostgreSQL 18.3 真库；dataenrichment/handler/repository/service/database 五组均 `ok`）
- [x] 7.2 后端静态门禁：`cd backend-go && golangci-lint run ./... && go vet ./... && go build ./...`，全部退出 0（golangci-lint 0 issues）
- [x] 7.3 前端 lint：`cd front && pnpm lint`，本轮退出 0（0 errors；5 warnings 均来自本 change 外的存量文件）
- [x] 7.4 前端 Windows 验证：`cmd.exe /C "cd /d D:\project\Syntopica\front && pnpm exec nuxi typecheck && pnpm test:unit && pnpm build"`，单条 `&&` 链退出 0（typecheck 通过；62 files / 739 tests 全绿；client 1214 modules、Nitro build 成功）
- [x] 7.5 UI 手动验收（真实 Chrome·opencli·board 1974 全链走完，2026-08-31）：①生成简报——job `ecff5800…` 214s 落 result #11（`ui-brief.png`），未自动触发调查；②选择问题→深入调查——q1「深入调查」真实点击，job `c37454d16c0baeb841420ae5` 18m53s 落 result #13；③查看反证——h1（被削弱）展开，「反证 2：e4/e5 泳道事实」+支持 1+缺口 1 分区齐全（`ui-investigation.png`）；④证据详情——e4 展开显示机构/日期/逐字 quote/支持 h3·反对 h1h2（`ui-investigation-evidence.png`）；⑤lane 下钻——调查证据 chip「泳道 #1204·下钻核查」→聚焦分析展开、textarea 预填（问题+两假设反证说明+证据摘录）、可编辑、不自动触发（`ui-investigation-lane-prefill.png`）；⑥打开 legacy——result #5「旧版分析」正常只读渲染+QA（`ui-legacy.png`）；⑦无强制反转标题/无 argument/depth 长文（首屏断言+sectors 键集核验）。截图均经独立视觉复检：`ui-investigation.png`（报告顶部：标题/问题/来源/当前判断/置信范围边界）、`ui-investigation-counter.png`（h1 三分区）、`ui-investigation-evidence.png`（e4 详情）、`ui-investigation-lane-prefill.png`（预填区）全 PASS
  - 真实UI发现/修复证据（2026-08-31，airouter 根因修复，不勾选 7.5）：真实 UI 调查确认 glm-5.3-flash `board_synthesize` 两次 HTTP 200（latency 358973/389148ms）但 assistant content=""，`Router.Chat` 旧逻辑 `callErr==nil` 即记 success 且直接消费 `chatResp.Content` → 不 fallback、下游 parser 报 “empty text”。修复：`internal/platform/airouter/router.go` 在 provider 边界统一规范化（`isEmptyChatResponse`：nil 响应或 `TrimSpace(Content)==""` → `ProviderError{Code:"empty_response", Retryable:true, Message:"provider returned empty response content"}`，进入既有失败日志/attemptErrors/ordered fallback 分支；不先记 success、不在 OpenAI client 特判、覆盖所有 provider 类型；非空内容含首尾空白原样返回不误伤）。验证：`router_test.go` 新增 4 组用例（空串/纯空白 fallback+日志契约、nil 响应不 panic 且 fallback、全空聚合错误含 provider 名+empty response 且零 success 行、非空白内容不误判）红→绿；`go test ./internal/platform/airouter -count=1`、`go test -race`、`golangci-lint run ./internal/platform/airouter/...`（0 issues）、`go vet` 全绿；映射 test-cases.md M11。UI 手动验收本身仍未执行，保持未勾

## 8. 文档

<!-- doc-impact: flow api database architecture standard configuration deployment -->

- [x] 8.1 已更新 `docs/reference/flow/data-enrichment.md`：默认简报、显式调查、多假设/反证、方法卡阶段隔离、原子落库/分链 review、旧行为退场与 23 条业务约束；术语与 specs 对账完成
- [x] 8.2 已更新 `docs/reference/api/dataenrichment.md` 与 API 索引：result_kind/parent/question_key、调查 trigger、kind filter、board QA、analysis-methods CRUD 与旧 reference endpoint 写 410；请求字段/状态码/响应字段已逐项对照 handler
- [x] 8.3 已更新 `docs/reference/database/DATABASE_FIELDS.md`：result_kind/owner 形状、parent 复合 FK+trigger、analysis_methods、三次迁移、legacy 原字节与 seed 用户编辑保护；仅向上无 Down 的回滚边界已写明
- [x] 8.4 已更新 `docs/reference/architecture/map.md`，并同步 `standard/backend/ai-logging.md`、`configuration.md`、`deployment.md`：双入口及真实 symbol、`empty_response` fallback、慢供应商 600 秒操作与旧数据降级均已对账
- [x] 8.5 运行 `bash scripts/doc-impact.sh verify openspec/changes/board-level-deep-analysis` 与 `bash scripts/check-standards.sh`，期望均退出 0
  - 归档前实测（2026-08-31）：本 change doc-impact verify 退出 0；check-standards.sh 125 通过/0 失败（fix-board-analysis-material 的疑似遗漏系跨 change 脏区误报，已按 §11.2 加 doc-impact-excuse 豁免；optimize-pg-storage 检查项本轮 [OK]）

## 9. 验证

- [x] 9.1 `openspec validate board-level-deep-analysis --strict`，本轮通过：change 与三份 capability delta 全部有效
- [x] 9.2 `bash scripts/scenario-trace.sh openspec/changes/board-level-deep-analysis`，M12/M13 新增综合与证据一致性边界后重新通过：81 个 Scenario 全部映射到实际测试文件（自动测试 81 / 人工留痕 0）
- [x] 9.3 已按当前脚本权威用法运行 `bash scripts/change-scope.sh --json`（脚本自收集 89 个 staged/unstaged/untracked 路径，旧的文件名参数形态会报未知参数）：机械建议 dataenrichment short test + 全量 vet + frontend lint；platform 测试按人工复核补充 airouter/database 影响包。唯一无法判定路径 `backend-go/$null` 为本 change 外既有脏文件，未触碰
- [x] 9.4 本轮（M12/M13 后）全量重跑：后端 `go test -count=1 ./internal/dataenrichment/... ./internal/platform/database/... ./internal/platform/airouter/...` 全 ok（含真 PG，55s）；`golangci-lint run ./...` 0 issues、`go vet ./...`、`go build ./...` 全 exit 0；前端 WSL `pnpm lint` exit 0（0 errors/5 存量 warning），Windows cmd `nuxi typecheck && test:unit && build` 链 exit 0（58s，stderr 为 happy-dom/网络存量噪音）
- [x] 9.5 部署前数据库终检（2026-08-31）：kind 分布 board_brief 1 / board_investigation 2 / legacy 7 / topic 3；scope-owner 形状违规 0；孤儿调查（父非同板块 brief）0；三个迁移均已登记。`inv_no_evidence=1` 为 M13 前历史快照 #12（result 不可变不回写，#13 为其同题重跑且证据链 5 条，已文档化降级路径）；`reference_roles` 剩 1 行 enabled 为用户编辑过的 inside-america-v2，按 seed 保护策略有意保留且已无 prompt 调用方；`analysis_methods` legacy 副本 1 行 disabled。无需用户清理或重新生成旧数据

### Scenario → 测试文件映射（scenario-trace 对账表，81/81 自动测试）

| Scenario | 测试文件 |
| --- | --- |
| 方法卡声明适用边界 | backend-go/internal/dataenrichment/handler/analysis_method_handler_test.go, backend-go/internal/dataenrichment/service/analysis_methods_test.go |
| 停用即时生效 | backend-go/internal/dataenrichment/repository/analysis_method_repository_test.go, backend-go/internal/dataenrichment/handler/analysis_method_handler_test.go |
| 手工创建因果检验方法 | front/app/features/settings/components/AnalysisMethodPanel.test.ts, backend-go/internal/dataenrichment/handler/analysis_method_handler_test.go |
| 没有适配方法 | backend-go/internal/dataenrichment/service/board_investigation_test.go |
| 多张方法同时可用 | backend-go/internal/dataenrichment/service/analysis_methods_test.go, backend-go/internal/dataenrichment/service/board_investigation_test.go |
| 方法删除后历史仍可追溯 | backend-go/internal/dataenrichment/service/board_investigation_persist_test.go, backend-go/internal/dataenrichment/repository/analysis_method_repository_test.go |
| 简报不受方法卡文风影响 | backend-go/internal/dataenrichment/service/board_brief_test.go |
| 方法只影响调查步骤 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go, backend-go/internal/dataenrichment/service/board_brief_test.go |
| 方法卡不进入证据链 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 固定修辞被拒绝 | backend-go/internal/dataenrichment/service/method_sanitizer_test.go, backend-go/internal/dataenrichment/service/analysis_methods_test.go |
| 两张方法正文超限 | backend-go/internal/dataenrichment/service/analysis_methods_test.go, backend-go/internal/dataenrichment/service/board_investigation_test.go |
| 迁移内部看美国画像 | backend-go/internal/platform/database/analysis_method_migration_test.go, front/app/features/settings/components/AnalysisMethodPanel.test.ts |
| 空方法库行为正常 | backend-go/internal/dataenrichment/service/board_investigation_test.go, backend-go/internal/dataenrichment/service/board_brief_test.go |
| 版块未开启增强时拒绝 | backend-go/internal/dataenrichment/handler/board_enrichment_handler_test.go, backend-go/internal/dataenrichment/service/enrich_board_test.go |
| 默认触发只生成简报 | backend-go/internal/dataenrichment/service/enrich_board_test.go |
| 多次触发保留独立简报 | backend-go/internal/dataenrichment/service/enrich_board_test.go |
| 简报触发立即返回 | backend-go/internal/dataenrichment/handler/analysis_runner_test.go, front/app/features/tags/composables/useBoardEnrichment.test.ts |
| 调查状态不冒充简报状态 | backend-go/internal/dataenrichment/handler/board_investigation_api_test.go, front/app/features/tags/composables/useBoardEnrichment.test.ts |
| 同版块任务防重入 | backend-go/internal/dataenrichment/handler/analysis_runner_test.go, backend-go/internal/dataenrichment/handler/board_investigation_api_test.go |
| 有素材但没有统一关系 | backend-go/internal/dataenrichment/service/board_brief_test.go |
| 存在多个并行趋势 | backend-go/internal/dataenrichment/service/board_brief_test.go |
| 全部素材稀薄 | backend-go/internal/dataenrichment/service/board_brief_test.go, backend-go/internal/dataenrichment/service/enrich_board_test.go |
| 仅语义相关 | backend-go/internal/dataenrichment/service/board_brief_test.go |
| 因果证据不足 | backend-go/internal/dataenrichment/service/board_brief_test.go |
| 关系引用幽灵泳道 | backend-go/internal/dataenrichment/service/board_brief_test.go, backend-go/internal/dataenrichment/service/evidence_test.go |
| 用户选择候选问题 | backend-go/internal/dataenrichment/handler/board_investigation_api_test.go, front/app/features/tags/components/BoardBriefReport.test.ts |
| 用户自填问题 | backend-go/internal/dataenrichment/handler/board_investigation_api_test.go, front/app/features/tags/components/BoardBriefReport.test.ts |
| 没有值得调查的问题 | backend-go/internal/dataenrichment/service/board_brief_test.go, front/app/features/tags/components/BoardBriefReport.test.ts |
| 零假设不可缺席 | backend-go/internal/dataenrichment/service/board_investigation_test.go |
| 候选假设都很宏大 | backend-go/internal/dataenrichment/service/board_investigation_test.go |
| 搜索词不得只有既定结论 | backend-go/internal/dataenrichment/service/board_investigation_research_test.go |
| 只有同向材料 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 外部工具不可用 | backend-go/internal/dataenrichment/service/board_investigation_research_test.go |
| 零假设最符合材料 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go, front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 所有假设证据不足 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go, front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 初始假设被研究推翻 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go, front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 完整综合只缺根对象右大括号 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 综合响应存在内部截断或括号错配 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| fallback 使用 lane_id 表示合法泳道证据 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 显式非法 ref 不被 lane_id 掩盖 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 清洗后确定性评估失去全部对应证据 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 零证据报告不猜测研究过程 | front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 支持与反证分开显示 | front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 避免重复机制长文 | front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 一份简报派生多份调查 | backend-go/internal/dataenrichment/service/board_investigation_persist_test.go, backend-go/internal/dataenrichment/repository/result_kind_repository_test.go |
| 跨版块父结果被拒绝 | backend-go/internal/dataenrichment/service/board_investigation_persist_test.go, backend-go/internal/dataenrichment/handler/board_investigation_api_test.go |
| 泳道多时简报素材受控 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 低质量泳道不被静默删除 | backend-go/internal/dataenrichment/service/situation_cards_test.go |
| 从简报观察下钻 | front/app/features/tags/components/BoardBriefReport.test.ts |
| 从调查证据下钻 | front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 第二份简报自动对比 | backend-go/internal/dataenrichment/service/enrich_board_test.go, backend-go/internal/dataenrichment/service/board_brief_review_test.go |
| 不同问题调查不互比 | backend-go/internal/dataenrichment/service/board_investigation_review_chain_test.go |
| 查看旧报告 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts, backend-go/internal/dataenrichment/handler/board_legacy_read_compat_test.go |
| 消费分层上下文 | backend-go/internal/dataenrichment/service/orchestrator_test.go |
| 解读员领域自适应 | backend-go/internal/dataenrichment/service/orchestrator_test.go, backend-go/internal/dataenrichment/service/orchestrator_internal_test.go |
| 分析员产出深度层而非走向预测 | backend-go/internal/dataenrichment/service/orchestrator_internal_test.go, backend-go/internal/dataenrichment/service/orchestrator_test.go |
| 死循环防御 | backend-go/internal/dataenrichment/service/orchestrator_test.go, backend-go/internal/dataenrichment/service/board_investigation_research_test.go |
| 下钻问题可修改且可推翻 | backend-go/internal/dataenrichment/handler/board_enrichment_handler_test.go, front/app/features/tags/components/BoardBriefReport.test.ts |
| 单泳道不继承作者画像 | backend-go/internal/dataenrichment/service/orchestrator_internal_test.go, backend-go/internal/dataenrichment/service/analysis_methods_test.go |
| 半自动判断避免噪音 | backend-go/internal/dataenrichment/service/orchestrator_test.go, backend-go/internal/dataenrichment/service/board_investigation_review_chain_test.go |
| 兑现度结算 | backend-go/internal/dataenrichment/service/enrich_board_test.go, backend-go/internal/dataenrichment/handler/board_legacy_read_compat_test.go |
| 用户手动批注 | backend-go/internal/dataenrichment/handler/handler_test.go |
| result kind 隔离 | backend-go/internal/dataenrichment/repository/result_kind_repository_test.go, backend-go/internal/dataenrichment/service/enrich_board_test.go |
| 周期筛选翻历史 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts, backend-go/internal/dataenrichment/handler/handler_test.go |
| 证据链 tooltip 不跳转 | front/app/features/tags/components/CausalAnalysisReport.test.ts, front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 兑现度复盘可见 | front/app/features/tags/components/BoardEnrichmentPanel.test.ts, backend-go/internal/dataenrichment/service/board_investigation_review_chain_test.go |
| 契约为侦探墙铺路 | front/app/features/tags/components/BoardBriefReport.test.ts, front/app/features/tags/components/BoardInvestigationReport.test.ts, backend-go/internal/dataenrichment/handler/board_result_serialization_test.go |
| 简报到调查由用户确认 | front/app/features/tags/components/BoardBriefReport.test.ts, backend-go/internal/dataenrichment/service/enrich_board_test.go |
| lane 类证据下钻 | backend-go/internal/dataenrichment/service/evidence_test.go, front/app/features/tags/components/BoardInvestigationReport.test.ts |
| 旧枚举不受影响 | backend-go/internal/dataenrichment/service/evidence_test.go |
| kind 可选兼容 | backend-go/internal/dataenrichment/service/evidence_test.go |
| 截断档分析前重算 | backend-go/internal/dataenrichment/service/freshness_gate_test.go, backend-go/internal/dataenrichment/service/board_investigation_freshness_test.go |
| 无记录首建 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 无数据周期跳过 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 限额溢出降级 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 补齐幂等 | backend-go/internal/dataenrichment/service/freshness_gate_test.go |
| 补齐失败降级 | backend-go/internal/dataenrichment/service/freshness_gate_test.go, backend-go/internal/dataenrichment/service/board_investigation_freshness_test.go |
| 直接证据少于三类 | backend-go/internal/dataenrichment/service/orchestrator_internal_test.go, backend-go/internal/dataenrichment/service/board_brief_test.go |
| 支持与反证并列保存 | backend-go/internal/dataenrichment/service/board_investigation_persist_test.go, backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
| 单泳道不扩 schema | backend-go/internal/dataenrichment/service/orchestrator_internal_test.go, backend-go/internal/dataenrichment/service/orchestrator_test.go |
| 条件不足诚实降级 | backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go |
