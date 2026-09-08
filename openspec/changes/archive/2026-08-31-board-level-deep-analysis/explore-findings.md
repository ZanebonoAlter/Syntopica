
## 内部看美国方法论画像 v2（7期全文验证完成）

7 期全文转写（yt-dlp -f 30280 音频 + faster-whisper medium GPU，脚本 yt-dlp/asr/batch_transcribe.py 支持断点续跑，转写稿在 yt-dlp/transcripts/*.txt，ASR 系繁体+错字）经 3 路子代理精读交叉验证，产出两份文档更新：(1) 内部看美国-分析基因.md 补「六期全文验证」节——7 基因中 ①②④⑦ 全稳（①深化为认知归因批判、④编号化、⑦数字 delta 化），③历史机制呈两副面孔（立自己框架=纵深背书 / 拆别人框架=证伪弹药），⑤边界限定不对称（只护选定轴），⑥范式转折分化（产业类弱化/辩论类变形为转折审计）；(2) inside-america-methodology-profile.md 升 v2——新增辩论流水线（钢人先行→概念考古→举证责任门→自然实验）、思想背叛叙事、机制保质期、比喻对决、物理隐喻系统、双向排异、金句工程、情绪阀门等模式，并给出跨议题稳定/变形清单。该文档即 tasks.md 2.3「手工录入首份参考角色」的内容源，v2 可直接录入（约 3.1KB，远低于注入上限 4k 字符）。

**引用**：docs/research/board-analysis-reference-role/inside-america-methodology-profile.md、docs/research/yt-dlp/transcripts/、docs/research/内部看美国-分析基因.md

<!-- pinned 2026-08-25T16:36:02Z -->

## 新鲜度门3天阈值+证据多样性纪律落档

用户新增两项需求已落 change（2026-08-26 讨论定案）：(1) D9 素材新鲜度门——EnrichBoard/EnrichTopic 装配前对本板块活跃泳道 week/month/year 三档 lifeline 查 as_of_date 滞后 >3 天者同步补齐（复用循环 A 汇总函数，从 job 抽出为服务方法）；关键坑：month 档月中滞后是常态，补齐必须把 as_of_date 推进到「本次增量处理位置」而非周期边界，否则每次分析重复补齐死循环；all 档不查；补齐串行限流、失败降级不阻塞+结构化日志；落 tasks 3.6 + spec delta 3 场景（滞后补齐/幂等/失败降级）。(2) D10 证据多样性纪律——analyze prompt 约束证据链覆盖≥3 类（数据序列/报告文献/历史对照/新闻网页）+ 一手源检索引导，source_type 枚举不动；落 tasks 3.7 + spec delta 2 场景。后置讨论：专业数据源结构化集成（API 序列/PDF 文献库）另开 change，本期不做。

<!-- pinned 2026-08-26T02:37:04Z -->

## 版块分析刻板文风的 prompt 与调用链根因

真实链路是：态势卡 → boardInterpret 先生成并选定命题 → boardResearchDirections 围绕已选命题生成“命题机制/钩子核实/历史对照”方向 → runBoardAgentLoop 做支持性检索 → boardAnalyze 以“命题已定”写论文式长文。boardInterpretPrompt 明文要求 thesis 必须是「X 不是 A，而是 B」；启用的《内部看美国·方法论画像》又把「不是X而是Y」、概念重命名、钩子升维、底层规律、块尾金句列为稳定硬核，并完整注入 interpret 与 final analyze。最终 prompt 强制 3-5 层递进机制、system_reframe、历史类比、depth；UI 又连续渲染 argument.layers 与内容高度重叠的 depth.mechanism_layers。当前没有“普通解释已足够/无结构性反转”的非 sparse 出口，也没有反证检索、竞争假设或研究后改写/推翻 thesis 的阶段，因此形成先立戏剧性命题、再搜证据支撑的确认偏误闭环。生产库 7 份 board 报告合计出现「不是」55 次、「而是」73 次，平均句长约 67 个中文字符，59 句超过 80 字；说明用户感受是系统性产物，不是单次模型漂移。建议后续设计讨论区分“方法卡”与“作者文风”，改为证据先行、多假设（含朴素/零假设）、反证检索、研究后可撤回 thesis，并让历史类比/system_reframe/深层机制按素材适配而非一律强制。

**引用**：backend-go/internal/dataenrichment/service/board_interpret.go:boardInterpretPrompt、backend-go/internal/dataenrichment/service/enrich_board.go:EnrichBoard、backend-go/internal/dataenrichment/service/enrich_board.go:boardResearchDirections、backend-go/internal/dataenrichment/service/board_analysis.go:boardAnalyzePrompt、backend-go/internal/dataenrichment/service/board_analysis.go:boardAgentLoopSystemPrompt、backend-go/internal/platform/database/seeddata/inside-america-methodology-profile.md、front/app/features/tags/components/BoardAnalysisReport.vue:BoardAnalysisReport

<!-- pinned 2026-08-28T15:12:43Z -->

## 重新定位：语义版块不是天然因果系统

重新对照 flow 后发现更上游的产品建模问题：SemanticBoard 的原始职责是把散装 event 标签组织成持久概念分区，section 还能同时挂 1-3 个版块；同板块泳道仅表示语义相关，不保证共同驱动、因果关系或能被一个命题串联。当前 board-level 分析却预设“这个板块作为一个系统怎么了”，强制把最多 12 条泳道织成单一传导链，这是把分类容器误当因果系统。更合理的产品分层是：①默认版块简报/态势扫描：事实变化、重要泳道、关系类型（共同驱动/因果/分化/仅相关/暂无关系）、未知项；②只对选中的可研究问题做深度调查：多假设含零假设，支持+反证检索，研究后允许撤回/拆分命题；③单泳道聚焦下钻。深度应由证据质量、替代解释和边界处理体现，而不是固定层数、宏大系统重定位或历史类比。参考角色应降级为按问题选择的方法卡，不能参与事实抽取，也不应把作者修辞全局注入。

**引用**：docs/reference/flow/semantic-board.md、docs/reference/flow/data-enrichment.md、backend-go/internal/dataenrichment/service/enrich_board.go:EnrichBoard、backend-go/internal/dataenrichment/service/board_interpret.go:boardInterpretPrompt、backend-go/internal/dataenrichment/service/board_analysis.go:boardAnalyzePrompt

<!-- pinned 2026-08-28T15:17:45Z -->

## 重设计必须兼容现有补全门与异步 runner

交叉审查确认两个既有集成点不能按旧 board-level 设计回退：① `fix-board-analysis-material` 已把 freshness gate 改成 month/year 有数据周期补全（缺行含首份先建、UpdatedAt>72h 重算、week 退出检查、单次 40 调用上限、失败降级），态势卡事实源为 week→month→实质 section 指纹→description→none；新版简报/调查必须直接复用。② 当前分析 trigger 已经由 analysisRunner 异步执行，但 key 仅 `(scope,targetID)`、状态无 job id/kind。拆成 brief/investigation 后需扩展唯一 job_id/job_kind；同一 board 两类任务仍串行，409 返回当前任务身份，前端按 job_id 轮询，避免调查完成被误当成新简报。另以规范化问题文本 hash 的 question_key 识别同父简报同题重跑；方法卡必须先按问题+父简报选择，再参与假设生成，避免选择循环。

**引用**：backend-go/internal/dataenrichment/service/freshness_gate.go:ensureLaneFreshness、backend-go/internal/dataenrichment/handler/analysis_runner.go:analysisRunner、backend-go/internal/dataenrichment/handler/board_enrichment_handler.go:triggerBoardEnrichment、front/app/features/tags/composables/useBoardEnrichment.ts:startBoardPoll、openspec/changes/fix-board-analysis-material/specs/data-enrichment/spec.md、openspec/changes/fix-board-analysis-material/specs/board-level-analysis/spec.md

<!-- pinned 2026-08-28T15:59:58Z -->

## 分析方法卡后端基础与 4.x 接口

tasks 2.1/2.2/2.3/2.5 已实现：analysis_methods 由 repository.AnalysisMethod + 强类型 AnalysisMethodSelectionMeta 承载（applicable_when/avoid_when/required_evidence/failure_modes，JSONB，soft delete，legacy）。Repository 提供 ListEnabledAnalysisMethodSummaries（不加载 Content）与 GetAnalysisMethodsByIDs（按请求 ID 顺序加载全文），供后续 2.4/4.x 选择器使用。service.AssembleSelectedAnalysisMethods 是纯函数：输入已按相关性排序的选中卡，最多 2 张，按 rune 总预算整卡舍弃，返回 Prompt、MethodRefs{id,title,content_hash}、Dropped{reason}；AnalysisMethodContentHash 对原始 content 字节做稳定 SHA-256。当前未接 LLM、未写 investigation input_snapshot/ai_call_logs。迁移 20260828_0002 将 reference_roles ON CONFLICT DO NOTHING 复制为 enabled=false/legacy=true，不覆盖同名用户方法；旧 GET API 保留，写 API 410；所有 topic/board prompt caller 已移除旧画像注入。

**引用**：backend-go/internal/dataenrichment/repository/models.go:AnalysisMethod、backend-go/internal/dataenrichment/repository/repository.go:ListEnabledAnalysisMethodSummaries、backend-go/internal/dataenrichment/repository/repository.go:GetAnalysisMethodsByIDs、backend-go/internal/dataenrichment/service/analysis_methods.go:AssembleSelectedAnalysisMethods、backend-go/internal/platform/database/postgres_migrations.go:analysisMethodLegacyCopyMigration、backend-go/internal/dataenrichment/handler/analysis_method_handler.go

<!-- pinned 2026-08-29T03:22:52Z -->

## 调查综合+持久化落地（4.5/4.6/2.4）与 5.x 入口契约

tasks 4.5/4.6/2.4 已实现并落库验收（2026-08-30）。供 5.x handler/analysis runner 接线：

**同步入口**：`OrchestratorService.InvestigateBoardQuestion(ctx, boardID uint, parentBriefID uint, question service.BoardInvestigationQuestion) (*service.BoardInvestigationOutput, error)`（board_investigation_synthesis.go）。预检失败（停用板块/跨board/legacy/不存在 parent/父 sectors 非法/问题非法）= error 且 0 LLM，可直接映射 4xx；synth/中途失败 = error 且 0 行（父简报不动）。成功返回 `BoardInvestigationOutput{Result}`（kind=board_investigation、parent_result_id、question_key 已落库）。同 brief 同题重跑允许（多行 append-only）。内部不跑 freshness/cards、不改写父 brief、不挂 review（4.7 未做）。session=`data_enrichment_board_{boardID}_{uuid8}` 全链共享；LLM 顺序 board_method_select（有 enabled 卡才调）→ board_hypothesize → data_enrichment.tool_use×N（一个共享研究 loop，N=研究决策数，纪律要求 ≥1 neutral + 每非零假设 1 counter + 1 finish）→ board_synthesize（重试最多 1）。

**question 契约**：`{ID string(显示用，generated 取简报候选 id), Text string, Source "generated"|"custom"}`；Normalize 只 trim 两端（原文含中间空白完整进 sectors.question.text）；question_key=repository.ComputeQuestionKey（trim+空白折叠 hash，generated/custom 同算法）。

**sectors 新 schema**（board_investigation）：scope/result_kind/parent_briefing_id/question{id,text,source}/hypotheses[{id,label,is_null,derived_from[],assessment(supported|plausible|insufficient|weakened|refuted),confidence(low|medium|high),scope,support_evidence[],counter_evidence[],gaps[]}]/conclusion{summary,confidence,scope,boundary}/evidence_chain[{id,source_type(news|web|page|lane),ref,url,quote,institution,date,kind,lane_note,supports[],counters[]}]/lane_refs[]/method_refs[{id,title,content_hash}]/retry_reason(稳定码)。无 thesis/argument/depth/system_reframe。tool_calls=共享研究完整有序记录（含 purpose/hypothesis_ids/outcome/result_full）。input_snapshot：parent_brief_id/parent_sectors(原始jsonb)/parent_projection/question/question_key/lane_whitelist(父简报快照推导)/methods(候选+选择+机码)/method_prompt(实际注入清洗后正文)/method_cards(逐卡清洗注入trace)/method_refs/evidence_needs/initial_hypotheses(含稳定retry码)/research{coverage,gaps,final_data,loops}/synthesis{attempts,retry_reason}。

**机械护栏**：高置信 supported 必须有 web/page 可核查证据（quote 保守 substring 核对工具 ResultFull）且 research 做过 counter（本假设或 derived_from 初始假设），否则降级 medium+gap+boundary 注记；method 绝不作 source_type；幽灵 lane（父简报白名单外）剔除。

**review Minor 已修**：generateBoardHypotheses 的 RetryReason 改稳定码（chat_error/parse_error/invalid_structure/no_null_hypothesis），synthesize 同款（chat_error/parse_error/invalid_structure）；完整 err 只进日志与重试 prompt（ai_call_logs）。

**测试锚点**：单测 board_investigation_synthesis_test.go（package service，15 用例）；PG 集成 board_investigation_persist_test.go（package service_test，7 用例，复用 mockAirRouter/mockBoardResolver/noopFreshnessRefresher，工具 stub=invLaneRenderer+invWebSearcher，链路响应 addInvChain/addInvChainWithSelector）。ai-logging.md 已登记 board_synthesize。

<!-- pinned 2026-08-30T04:57:15Z -->

## 8.x 文档对账事实清单（flow/api/database/map 四文档改法）

8.1-8.4 文档对账所需全部事实已核实（未开始写文档，git 零改动）：

【flow/data-enrichment.md 需改点】①「版块级深度分析」节（~L137-150）仍写旧自动论文链（board_interpret→runBoardAgentLoop→boardAnalyze 五字段 sectors）→ 改为双链：EnrichBoard=简报（enrichment_enabled 门槛→活跃泳道→补全门→态势卡→board_brief 单次 LLM 不联网不注方法全文→persist；坏 JSON 重试一次→机械降级只产观察）+ InvestigateBoardQuestion(boardID, parentBriefID, question)=调查（board_method_select 选 0-2 卡→board_hypothesize 2-4 假设必含 H0→单一共享研究循环复用 data_enrichment.tool_use（中性+反证查询、gap 记录）→board_synthesize 五态 assessment supported|plausible|insufficient|weakened|refuted）。②约束#14 改 result_kind 制（topic_analysis|board_brief|board_investigation|legacy_board_analysis，scope-owner 互斥、board_investigation 必挂同版块 board_brief 父+question_key=规范化问题文本 SHA-256 hex64，generated/custom 同 key）；review 按 kind：简报只比上一份简报、调查只比 parent_result_id+question_key 相同重跑、legacy 只读。③约束#18「≥3 类证据」→ 证据适配与反证纪律（orchestrator.go L1035-1039 已改直接相关性优先、反证保留；board_analysis.go 的 ≥3 类 prompt 仅 legacy 文件残留无调用方）。④代码入口节：board_interpret.go 标注 legacy-only；补 board_brief.go/board_investigation*.go/method_sanitizer.go/board_brief_review.go/board_investigation_review.go；前端 BoardBriefReport.vue/BoardInvestigationReport.vue/useBoardEnrichment.ts、设置页 SettingsSectionAnalysisMethods（settings.vue sectionKey 'analysis-methods'，旧 ReferenceRolePanel 不再挂载）。⑤Operation 表补 4 个：board_brief/board_method_select/board_hypothesize/board_synthesize（board_investigation_research.go L46 复用 tool_use）。⑥REST 表：trigger 改 202+{job_id,job_kind:board_brief,scope,target_id}；新 investigations/trigger；results?kind=；QA 三条 board 路由；analysis-status?job_id=。⑦变更溯源补 v2 行（change 相对链接 openspec/changes/board-level-deep-analysis 未归档）。

【api/dataenrichment.md 需改点】trigger 旧示例（200+result+review_generated+freshness_report）→202 job 信封（board+topic 都是）；409 体 {success:false,error,data:当前job身份(AnalysisStatus)}；GET /enrichment/analysis-status?job_id=（未知 404）/scope=board|topic&id=（无任务返 idle 骨架）；investigations/trigger body {briefing_result_id 必填, question_id?→generated（文本以父简报 research_questions 为准，qid 解析不出 400）, question?→custom}，同步预检 400/404 且 0 后台调用；results?kind=（非法 kind 400）行结构 serializeBoardResult={id,analysis_scope,result_kind,parent_result_id,question_key,sectors,tool_calls,input_snapshot,session_id,created_at}；board QA POST/GET /results/:rid/qa + /qa/:qid/sediment（三 kind 均可，append-only，跨板块/scope-mismatch 统一 404）；analysis-methods CRUD（POST name+content 必填 400、重名 409、默认 disabled；PUT /:id 局部更新；PUT /:id/enable {enabled} 必填 400；DELETE 软删除返 {deleted:id}）；reference-roles GET+GET/:id 保留、POST/PUT/DELETE 一律 410 指向 /analysis-methods。prefill_lens 语义=可修改预填（EnrichTopicLens），drill 从 brief observation/relationship/question/investigation evidence。_index.md dataenrichment 行补路由前缀。

【DATABASE_FIELDS.md 需改点】表清单缺 reference_roles 行、需加 analysis_methods 行（现列 47 行 → 49；首行"43 张"/节头"48 张/45 张"本就漂移，统一改 49）；§10.3 补 result_kind VARCHAR(32) NOT NULL DEFAULT 'topic_analysis' + CHECK chk_topic_enrichment_result_kind、parent_result_id BIGINT（复合 FK fk_topic_enrichment_result_parent_board REFERENCES (id,semantic_board_id) ON DELETE RESTRICT + 触发器 trg_validate_topic_enrichment_result_parent：investigation 父必须同版块 board_brief、有子调查的 brief 不得改 kind/board）、question_key VARCHAR(64)（CHECK ^[0-9a-f]{64}$）、形状约束 chk_..._parent_shape（topic_analysis: topic owner+无父无key；brief/legacy: board owner+无父无key；investigation: board owner+父+key）、索引 idx_..._board_kind_id/idx_..._parent_question_id（后者 partial WHERE parent_result_id IS NOT NULL）、迁移 20260828_0001（回填：board→legacy_board_analysis、topic→topic_analysis；mixed owner 行拒绝迁移）；§10.7 reference_roles 更新（20260831_0001 按 name+seeded title+frozen content 字节钉死翻 disabled，用户编辑行不动）；新增 §10.8 analysis_methods（name unique/title/summary/selection_meta jsonb{applicable_when,avoid_when,required_evidence,failure_modes}/content/enabled 默认 false/legacy bool/deleted_at 软删索引/created_at/updated_at；20260828_0002 从 reference_roles 按字节复制 ON CONFLICT(name) DO NOTHING 不覆盖用户编辑）。topic_enrichment_qa 注明 board 三 kind 共用（按 result_id 归属）。

【map.md】数据富化行后端入口改：enrich_board.go（简报编排）/board_brief.go/board_investigation{,_research,_synthesis}.go（调查链）/analysis_methods.go+method_sanitizer.go（方法卡）/freshness_gate.go/situation_cards.go/board_interpret.go+board_analysis.go（legacy-only）；前端 BoardBriefReport.vue/BoardInvestigationReport.vue/BoardAnalysisReport.vue(legacy)/useBoardEnrichment.ts、设置页 AnalysisMethodPanel.vue。

【已就绪无需改】ai-logging.md 清单/SessionID 规则/Capability 映射均已含 4 新 operation 与调查 session 规则（与代码一致，仅资料来源日期行可不动）；configuration.md 数据增强节不受影响（无新配置项，board_brief/investigation 复用 data_enrichment_analysis capability；方法卡经 UI 管理非配置文件）——configuration 若 doc-impact verify 需要，可仅在数据增强节加一句"版块简报/调查复用 data_enrichment_analysis 路由，无新增配置"；deployment.md 无需改（无新环境变量/服务；迁移自动执行）。

<!-- pinned 2026-08-31T04:37:12Z -->

## 空AI回复未触发fallback

真实UI调查在 provider timeout=600s 时，glm-5.3-flash 的 board_synthesize 两次 HTTP 均成功（latency 358973/389148ms）但 assistant content 长度=0；Router.Chat 当前只看 callErr==nil 就记录 success 并返回空 Content，导致 service parser 报 empty text，既有 qwen fallback 未执行。应在 Router.Chat provider 边界把 nil/空白 ChatResponse 规范化为 ProviderError(code=empty_response,retryable=true)，再走统一失败日志与 ordered fallback；当前 ChatResponse 只有 Content/Usage，无 native tool_calls，故空文本没有合法语义。

**引用**：backend-go/internal/platform/airouter/router.go:Router.Chat、backend-go/internal/platform/airouter/openai_compatible.go:ChatResponse、backend-go/internal/platform/airouter/router_test.go

<!-- pinned 2026-08-31T09:10:55Z -->

## 调查综合单根终止符故障边界

真实 job a20b6a89c62af13c52d8d4b5：glm board_synthesize 两次 HTTP 200 空 content，经 Router empty_response 正确 fallback；qwen 响应日志 1533364（2622字）词法 delimiter stack 仅剩根 `{`，补恰好一个 `}` 后可解析为完整 hypotheses(4)/conclusion四字段/evidence_chain(5)/lane_refs(6)；日志1534148（3412字）在内部 position1177括号错配，不可修。M12 因此只在 board synthesis parser 做单根终止符 repair：无错配/未闭字符串、栈仅根{、尾字符]、补}后encoding/json成功且顶层lane_refs为array；其余继续一次纠错重试/失败0行。成功 repair 记 input_snapshot.synthesis.repair_reason=terminal_root_delimiter。qwen provider max_tokens=3000，glm timeout=600。

**引用**：backend-go/internal/dataenrichment/service/board_investigation_synthesis.go:parseBoardSynthesisJSONResponse、backend-go/internal/dataenrichment/service/board_investigation_synthesis.go:synthesizeBoardInvestigation、backend-go/internal/dataenrichment/service/board_investigation_synthesis_test.go:TestParseBoardSynthesisJSONResponse_TerminalRootRepairBoundary

<!-- pinned 2026-08-31T11:04:52Z -->

## fallback lane证据字段漂移

真实成功 result #12 / session data_enrichment_board_1974_961016c1：glm board_synthesize 360921ms 后空content→empty_response fallback；qwen返回合法JSON但两条 lane evidence 用 `lane_id:5` 而非规范 `ref:"5"`。旧 parser 因 ref空将两条全剔除，并清空全部 hypothesis refs，却仍落 h1=refuted、h2/h3=supported，形成 evidence_chain=[] 的悬空确定性结论。M13 修复：仅 source_type=lane 且显式ref为空时接受 float64安全正整数(<2^53) lane_id，转十进制ref后仍过父简报白名单；显式ref永远优先。存活证据supports/counters按first-seen并入hypothesis refs；supported无support、refuted/weakened无counter升级为结构失败走一次纠错重试，第二次仍败0行。前端空证据文案改为不归因的“没有通过核验、可展示的证据”。旧result不可变不回写，可从父简报显式重跑。

**引用**：backend-go/internal/dataenrichment/service/board_investigation_synthesis.go:laneIDAliasRef、backend-go/internal/dataenrichment/service/board_investigation_synthesis.go:mergeEvidencePolarityIntoHypotheses、backend-go/internal/dataenrichment/service/board_investigation_synthesis.go:checkDefinitiveAssessmentConsistency、front/app/features/tags/components/BoardInvestigationReport.vue

<!-- pinned 2026-08-31T12:39:56Z -->
