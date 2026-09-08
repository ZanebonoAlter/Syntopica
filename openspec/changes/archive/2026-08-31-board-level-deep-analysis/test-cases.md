# 白盒用例 · board-level-deep-analysis（修订版）

> 复杂档白盒用例文档（开发执行规范 §2）。2026-08-28 用户验收将主线修订为“版块简报→用户选题→多假设调查”。黑盒行为见 specs；本文只补内部边界、失败分支与兼容矩阵。

## M1 态势卡与补全门（保留当前实现回归）

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M1.1 | 有 week lifeline | 卡片优先使用 week 摘要 | facts_source=lifeline_week，不透传全文 |
| M1.2 | week 缺失但有 month lifeline | 卡片使用 month 摘要 | facts_source=lifeline_month，补全成果被消费 |
| M1.3 | 无 lifeline 但有近期 section 正文/摘要 | 降级实质事实指纹 | 不得只输出“泳道名×篇数” |
| M1.4 | 无 lifeline 且无可读 section | 再降级 description/none | 不报错、不伪造内容 |
| M1.5 | 多泳道质量排序/≥10 泳道 | 高质量在前且输入预算受控 | 质量只影响排序，不进入关系依据 |
| M1.6 | month/year 有数据周期缺行（含首份） | 串行补建后装配 | missing_period 被刷新；week 不检查 |
| M1.7 | month/year 已有行最后写入 >72h | 重算 | stale_row 被刷新，as_of 不晚于 now |
| M1.8 | 粒度无 section 数据 | 跳过 | action=skip_no_data，不建空档 |
| M1.9 | 补全需求 >40 | 超额降级 | action=budget_exhausted，分析继续 |
| M1.10 | 补全失败 | 用旧卡继续 | 日志含 topic/granularity/period/error |
| M1.11 | 同日二次触发 | 新鲜档不重复补 | 幂等 |

## M2 board_brief 生成

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M2.1 | 正常多泳道 | summary + observations + relationships + uncertainties + research_questions | 无 thesis/argument/depth 必填 |
| M2.2 | 泳道均有内容但无关系 | 正常 brief | relationships 可空或 unclear/context_only，不走 sparse |
| M2.3 | 多个方向分化 | 产 divergent/并行观察 | 不压成单一命题 |
| M2.4 | 全部无观察 | 素材不足 brief | research_questions 为空 |
| M2.5 | LLM 非法 JSON | 重试一次 | 第二次仍错走机械观察降级 |
| M2.6 | 机械降级 | 只列高质量观察 | 不造关系、不造研究问题 |
| M2.7 | observations >5 / relationships >6 / questions >4 | parser 截断或拒绝重试 | 上限稳定可测 |
| M2.8 | prompt snapshot | 无工具说明、无方法卡全文、无“X不是A而是B”要求 | 默认触发工具调用次数=0 |
| M2.9 | 旧 applied review 存在 | 只作为偏差提醒 | 不把旧 thesis 当本次事实 |

## M3 关系与引用校验

| # | 输入 | 期望 |
|---|---|---|
| M3.1 | 合法 common_driver/possible_causal/divergent/context_only/unclear | 保留 |
| M3.2 | 未知 relation type | 降级 unclear 或拒绝该条并留痕 |
| M3.3 | lane_ids 含非活跃 lane | 拒绝整条关系或清理幽灵 id，不污染其他条目 |
| M3.4 | evidence_refs 指向不存在 observation | 拒绝悬空 ref |
| M3.5 | possible_causal 无解释/低证据 | 降级 unclear，不能提升置信 |
| M3.6 | 质量分高但无关系证据 | 不建立关系 | 质量信号不得充当依据 |

## M4 假设生成

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M4.1 | generated question | 2-4 hypotheses | 含 question source=generated |
| M4.2 | custom question | 同一链路 | 保存原始文本，source=custom |
| M4.3 | 无零假设 | 重试；仍缺则机械补 H0 | 进入研究前 `is_null=true` 至少 1 个 |
| M4.4 | 所有假设只是宏大叙事同义改写 | 判无竞争性并重试 | H0/普通解释必须独立 |
| M4.5 | 假设缺 support_needed/disconfirm_needed/scope | 解析失败 | 不进入研究循环 |
| M4.6 | 5 个以上假设 | 截断或重试至 2-4 | 控制研究预算 |

## M5 共享研究循环

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M5.1 | 4 个假设 | 只启动一个共享 tool loop | 非每假设一套 loop |
| M5.2 | 研究计划 | 含基础事实、中性查询、支持与反证任务 | 不得所有查询都含某个结论词 |
| M5.3 | 非零假设 | 至少尝试一次反证/替代核查 | 无结果记 gap，不伪造 counter evidence |
| M5.4 | 同 tool+args 重复 | 拦截 | 既有三防御回归 |
| M5.5 | web_search 未配置 | 继续内部研究 | result 记录 external gap |
| M5.6 | fetch_page 失败 | 不把 snippet 当原文 quote | 证据质量降级 |
| M5.7 | 历史类比与问题无关 | 不检索或不纳入结论 | 无证据类型配额驱动 |

## M6 调查综合与持久化

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M6.1 | 某非零假设证据充分且反证弱 | supported/plausible | confidence 与证据质量匹配 |
| M6.2 | 支持和反证相当 | plausible/insufficient | 两边均保留 |
| M6.3 | 反证直接否定 | weakened/refuted | 不因初始排序保留赢家地位 |
| M6.4 | 所有非零假设不足 | H0 可最可信或 conclusion=insufficient | 不强选宏大结论 |
| M6.5 | 初始假设需拆分 | 最终可新增/拆分说明 | 保留初始与最终差异可追溯 |
| M6.6 | synthesize 中途失败 | 不落半成品 investigation | 父 brief 不受影响 |
| M6.7 | 正常落库 | parent 指向同 board brief 且 question_key 为规范化问题 hash | tool_calls/method_refs/input_snapshot 齐备 |
| M6.8 | parent 是别版块或 legacy | 拒绝 | DB/service 双层校验 |
| M6.9 | 一 brief 两问题 | 两 child rows | 互不覆盖 |

## M7 方法卡选择与隔离

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M7.1 | 空方法库 | brief/investigation 正常 | method_refs=[] |
| M7.2 | enabled 卡均 avoid | 选择 0 张 | 不强选最接近项 |
| M7.3 | 3 张适配 | 最多 2 张 | 记录理由 |
| M7.4 | 两张全文超预算 | 整卡舍弃低相关项 | 不截断卡片内部结构 |
| M7.5 | 生成 brief | 0 方法正文注入 | 即使库中有 enabled 卡 |
| M7.6 | 调查选中历史对照但无历史材料 | 标记方法不适用/gap | 不编造案例 |
| M7.7 | 方法含固定金句/模仿人格要求 | 该修辞不进入 prompt | 方法内容校验/清洗可追溯 |
| M7.8 | 旧 reference role 迁移 | 原文 hash 不变、disabled | 旧表仍可回滚读取 |
| M7.9 | 方法选择顺序 | 仅按问题+父简报选卡，再生成假设 | 不读取未生成的候选假设 |
| M7.10 | 已引用方法被删除 | 软删除且历史可回放 | method_refs 有 title/content_hash，input_snapshot 有注入正文 |

## M8 数据迁移与 kind 隔离

| # | 条件 | 期望 |
|---|---|---|
| M8.1 | 旧 analysis_scope=topic | result_kind=topic_analysis |
| M8.2 | 旧 analysis_scope=board 且 thesis/argument 形状 | result_kind=legacy_board_analysis |
| M8.3 | 新 brief | scope=board + kind=board_brief + parent null |
| M8.4 | 新 investigation | scope=board + kind=board_investigation + parent/question_key non-null |
| M8.5 | GET kind=board_brief | 不混入 investigation/legacy/topic |
| M8.6 | 简报 review | 只找上一 brief |
| M8.7 | 两个不同 question | 不互相 review |
| M8.8 | 同 parent+question_key 重跑 | generated/custom 同一规范化文本可比较 hypothesis assessment 变化 |

## M9 API 与前端

| # | 条件 | 期望 |
|---|---|---|
| M9.1 | POST 现有 analysis trigger | 立即返回唯一 job_id + kind=board_brief，不自动调查 |
| M9.2 | POST investigation generated question | 校验 brief/question id，返回 kind=board_investigation job |
| M9.3 | POST investigation custom question | 空白拒绝，合法文本保存并生成 question_key |
| M9.4 | 同 board 已有任一任务运行 | 409 携带当前 job_id/job_kind，不启动第二任务 |
| M9.5 | 客户端断连/重进 | 后台继续，按 job_id 或 board 当前任务恢复轮询 |
| M9.6 | investigation 完成 | UI 刷新 child result，不误报/选中新 brief |
| M9.7 | 简报无 research_questions | UI 正常，无“深入调查”空壳按钮 |
| M9.8 | 点击候选问题 | payload 含 briefing_result_id+question_id |
| M9.9 | 自填问题 | payload 含 briefing_result_id+question |
| M9.10 | 调查报告 | 首屏 conclusion；hypotheses/support/counter/gaps 折叠 |
| M9.11 | legacy result | 旧组件渲染+“旧版分析”标记 |
| M9.12 | lane 下钻 | prefill 为观察/问题，可编辑 |
| M9.13 | loading/error | job_kind 分派正确，brief 与 investigation 视觉状态隔离 |
| M9.14 | unmount 后 trigger/sync 响应迟到 | 不重建 timer、不 notify、不写任务态（view context disposed） |
| M9.15 | A trigger/sync 在途切 B | 不跟 A 的 job、不污染 B 任务态/列表/选中；回 A 由 scope sync 恢复 |
| M9.16 | A finished reload 在途切 B | loader 自身拒写旧板块列表/选中，外层不 select、无新增 notify、无 timer 重建 |
| M9.17 | A→B 切板块 | activate 同步置空 selectedTopicId + 清 topic 级展示 refs（无混合视图）；B topics 失败后不回填、无 topic 级请求/轮询 |
| M9.18 | 同 board topics 失败 | 当前视图 selected 置空 + 清 topic 级 refs + 停 topic/debate 轮询，error 置位，后续无 topic 级请求 |
| M9.19 | 同 board topics 成功刷新 | 合法选择保留、topic 级 refs 不清（由后续 loader 刷新） |
| M9.20 | A topics 失败响应迟到切 B 后到达 | epoch 守卫丢弃，不清 B 的列表/选择/展示 refs/loading、不写 error |
| M9.21 | investigation sectors 混入 argument/depth 旧字段（脏数据） | 视图忽略不渲染，首屏不铺连续长文（5.7 前端护栏） |
| M9.22 | 悬空证据引用 / 幽灵或非法非正整数 lane ref / prefill 超长 | 引用原样显示不崩 / 不渲染下钻入口不 emit / 截断 ≤400 rune（前端第二道护栏） |
| M9.23 | investigation trigger 400/404（父结果非法/不存在） | 明确报错、不启动轮询、释放 triggering（同步预检失败不盲转） |
| M9.24 | 无 result_kind 旧行选中 | 前端兑底 legacy 路由（旧组件+「旧版分析」标注，与后端 EffectiveResultKind 双保险） |
| M9.25 | board 档 detail：URL 板块属主正确但 result `analysis_scope=topic`（脏/历史行） | 404（与跨板块/不存在统一，不区分），响应体不泄漏行内容；列表不泄漏脏行、正常 board 档行 200 不误伤。落点 `handler/board_enrichment_handler_test.go` `TestBoardEnrichmentRoutes_DetailScopeMismatch` |
| M9.26 | topic 档 QA load(list)/ask/sediment：请求迟到时 URL topicId 与 result 实际归属不一致；board 档 result 借 topic 路由 | 三路由均 404 不泄漏行；sediment 经 qa→result 归属二跳校验。落点 `handler/handler_test.go` `TestAskQA_IDORProtection`/`TestListQA_IDORProtection`/`TestSedimentQA_IDORProtection`；board 档对照矩阵 `board_qa_handler_test.go` `TestBoardQA_OwnershipMatrix`（ask/list/sediment 跨 board、topic 档、同 board 换 result、不存在均 404，被拒组合零 sedimented 翻转） |

## M10 退场链与可观测性

| # | 条件 | 期望 |
|---|---|---|
| M10.1 | 新版默认 trigger | `board_interpret`/旧 board analyze 调用次数=0 |
| M10.2 | 简报 session | 只含 board_brief（及必要 review）Operation |
| M10.3 | 调查 session | hypothesize + tool_use + synthesize，可按 session 回放 |
| M10.4 | 方法选择 | input_snapshot 可查候选、选中、舍弃原因 |
| M10.5 | 旧报告 QA | 继续可用，不触发旧报告改写 |

## M11 airouter 空回复 fallback（真实 UI 根因修复）

> 真实 UI 调查根因：glm `board_synthesize` HTTP 200 但 assistant content=""，`Router.Chat` 旧逻辑记 success 且不 fallback，下游 parser 报 empty text。修复在 provider 边界统一规范化（空/白内容 → `ProviderError{Code:"empty_response"}`），覆盖所有 provider 类型，不在单个 client 特判。

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M11.1 | primary 空串/纯空白内容、backup 正常 | 记 primary 失败并继续 ordered fallback | ai_call_logs 主行 error_code=empty_response 且非 success；返回 backup 内容，UsedFallback=true/AttemptCount=2 |
| M11.2 | primary 返回 nil ChatResponse（无 err） | 不 panic、同样走 fallback | 同 M11.1 判据 |
| M11.3 | 所有 provider 均空 | 返回聚合错误 + nil result | 错误含各 provider 名与 “empty response”；日志无 success 行 |
| M11.4 | 非空内容含首尾空白 | 原样成功返回、不误判空 | 内容不 trim、UsedFallback=false/AttemptCount=1、日志 success |
| M11.5 | 空回复失败行日志契约 | prompt/session/operation 字段不回归 | 失败行 Prompt=`[user]\n…`、SessionID/Operation/Capability 与成功路径一致 |

## M12 board_synthesize 单一根终止符修复（真实 UI fallback hardening）

> 生产证据：qwen fallback 首次响应 2622 字，四个顶层字段均完整，词法 delimiter stack 只剩根 `{`；追加一个 `}` 后可被 `encoding/json` 完整解析。第二次响应有内部错配，必须继续拒绝。修复只位于调查综合 parser 边界，不放宽通用 JSON parser。

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M12.1 | clean JSON | 原样解析，不标 repair | attempts=1，repair_reason 空 |
| M12.2 | 完整 hypotheses/conclusion/evidence_chain/lane_refs 仅缺根 `}` | 只补一个 `}`，继续完整结构/证据/lane 校验 | attempts=1，不发纠错重试；generation meta `repair_reason=terminal_root_delimiter` |
| M12.3 | 缺两个以上 delimiter / 内部 `]}` 错配 | 不修复 | 第一次进入纠错重试；第二次仍坏则 error、0 调查行 |
| M12.4 | 字符串或 escape 未闭合 | 不修复 | 同 M12.3 |
| M12.5 | 尾随正文或缺顶层 `lane_refs` | 不修复 | 同 M12.3，禁止把半份调查当完整结果 |
| M12.6 | 单根修复后 schema/初始假设覆盖仍非法 | 不因 JSON 可解析而放宽结构校验 | 继续既有 invalid_structure 重试/失败 |

## M13 fallback lane evidence 字段归一与评估一致性

> 真实 result #12：qwythos 返回 2 条合法 lane evidence，但使用 `lane_id:5` 而非规范 `ref:"5"`；旧 parser 全部剔除后仍落 supported/refuted，形成“证据链空但结论确定”的悬空报告。修复以白名单和状态一致性为边界，不宽松接纳任意别名。

| # | 条件 | 期望 | 判据 |
|---|---|---|---|
| M13.1 | lane evidence 无 ref、lane_id 为白名单内安全正整数 | 归一为十进制 ref 并保留 | persisted evidence.ref 精确等于 lane_id；supports/counters 可在假设分区展示 |
| M13.2 | 显式 ref 合法且另有冲突 lane_id | ref 优先 | 不改写显式身份 |
| M13.3 | 显式 ref 非法/越白名单但 lane_id 合法 | 不用别名掩盖 | 证据剔除 |
| M13.4 | lane_id 为 0/负数/小数/≥2^53/非数字 | 拒绝别名 | 证据剔除且不溢出/回绕 |
| M13.5 | evidence 极性存在但 hypothesis 冗余引用漏项 | 确定性并集合并 | hypothesis support/counter 引用补齐为存活 evidence id，不重复 |
| M13.6 | supported 无存活 support，或 refuted/weakened 无存活 counter | 综合结构失败 | 首次纠错重试；第二次仍非法 error、0 行 |
| M13.7 | insufficient/plausible 无直接证据但 gaps/boundary 完整 | 允许保守落库 | 不因缺证据强造状态或机械改结论 |
| M13.8 | 前端 evidence_chain 空 | 中性空态 | 显示“没有通过核验、可展示的证据”，不写“没有采到材料” |

## 测试落点

| 模块 | 计划测试文件 | 层 |
|---|---|---|
| M1-M3 | `service/situation_cards_test.go`, `freshness_gate_test.go`, `board_brief_test.go` | 单测/DB集成 |
| M4-M6 | `service/board_investigation_test.go`, `board_investigation_research_test.go`, `enrich_board_test.go` | 单测/集成 |
| M7 | `service/analysis_methods_test.go`, `handler/analysis_method_handler_test.go`, database migration test | 单测/DB集成 |
| M8 | repository + migration + review tests | DB集成 |
| M9 | handler tests + Vue component tests；QA/详情守卫：`board_enrichment_handler_test.go` `TestBoardEnrichmentRoutes_DetailScopeMismatch`、`handler_test.go` `TestAskQA_IDORProtection`/`TestListQA_IDORProtection`/`TestSedimentQA_IDORProtection`、`board_qa_handler_test.go` `TestBoardQA_OwnershipMatrix` | API/unit |
| M10 | M10.1: `service/enrich_board_test.go` `TestEnrichBoard_TriggerChainNeverRunsLegacyWritePath`（三旧 Operation=0）；M10.2: `TestEnrichBoard_ReviewJudgeSkipsLegacyPrev` + `board_brief_review_test.go`；M10.3: `service/board_investigation_persist_test.go`（快照/tool_calls 留痕）；M10.4: 同前 + `method_sanitizer_test.go`；M10.5: `handler/board_legacy_read_compat_test.go`（`TestBoardLegacyReadCompatibility` 指纹不变 + `TestBoardLegacyQAAppendOnly`）+ `handler/board_qa_handler_test.go`（含 `TestListQA_IDORProtection` topic 档 IDOR） | 单测/DB集成 |
| M11 | `platform/airouter/router_test.go`：`TestRouterChatFallsBackOnEmptyResponse`（M11.1/M11.5，子用例 empty_string/whitespace_only）、`TestRouterChatNilResponseFallsBack`（M11.2）、`TestRouterChatAllProvidersEmptyReturnsError`（M11.3）、`TestRouterChatPaddedContentStillSucceeds`（M11.4） | 单测/DB集成 |
| M12 | `service/board_investigation_synthesis_test.go`：综合专用 terminal-root repair 纯函数矩阵 + `synthesizeBoardInvestigation` 首次修复不重试 / 非单根故障仍重试失败 | 单测 |
| M13 | `service/board_investigation_synthesis_test.go`：lane_id alias 安全边界/显式 ref 优先/极性引用并集/确定性 assessment 证据一致性重试；`front/app/features/tags/components/BoardInvestigationReport.test.ts`：零证据中性空态 | 单测/组件 |
