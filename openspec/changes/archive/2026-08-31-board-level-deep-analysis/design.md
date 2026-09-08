# Design: board-level-deep-analysis

## Context

见 `proposal.md` 的 Why。现有实现已经具备态势卡、新鲜度门、泳道下钻、lane 证据、append-only result、QA/review 与版块 API，但认知链把语义共现误当作因果系统：`board_interpret` 在研究前生成并锁定反转式命题，后续研究方向只验证命题，最终 `boardAnalyze` 强制论文式多层机制与系统重定位。生产样本中固定句式和抽象词高度重复，说明问题来自契约与编排，而非单次模型漂移。

SemanticBoard 的业务本质是持久概念分区：同一 section 可落入多个版块，同版块泳道只保证语义相关，不保证共同驱动或因果传导。因此默认入口必须先回答“发生了什么、是否有关”，不能预设“存在一个底层命题”。

## Goals / Non-Goals

**Goals:**

- 默认触发以低成本版块简报帮助用户识别变化、关系、不确定性和研究问题
- 深度调查仅由用户显式选择问题后启动，并通过多假设、零假设、支持与反证形成可撤回结论
- 分析方法按问题适配选择，事实阶段与作者修辞隔离
- 新旧结果可并存，既有 lane 引用、快照、QA/review、新鲜度门继续复用
- 前端先给人话结论和证据，再按需展开研究细节

**Non-Goals:**

- 自动替用户选择问题并连续启动调查
- 自动调度、泳道治理、专业结构化数据源与方法卡自动生产
- 重写或删除已生成的旧论文式版块报告
- 在本 change 内重构单泳道现有多形态 schema；只调整其方法注入、证据纪律与版块下钻入口

## Decisions

### D1 版块简报先判断关系，禁止预设统一系统

- 简报把每条泳道视为独立观察单位，只能在证据支持时建立跨泳道关系。
- 关系类型固定为：`common_driver`（可能共同驱动）、`possible_causal`（可能因果传导）、`divergent`（方向分化）、`context_only`（仅背景相关）、`unclear`（尚无法判断）。不得把同期出现直接标成因果。
- “未发现统一关系”“存在多个并行趋势”“材料充分但只支持普通解释”都是正常结果，不走 sparse，也不触发自动深挖。
- 备选否决：继续生成多个 thesis 让用户选择——候选仍然预设必须立论，只是把确认偏误延后给用户。

### D2 默认简报与显式调查拆成两条编排

```text
EnrichBoardBrief(boardID)
  freshness → situation cards → board_brief(单次 LLM、仅内部素材) → persist
                                      │
                               用户选择/自填问题
                                      ▼
InvestigateBoardQuestion(briefID, question)
  hypotheses → shared research loop → synthesis → persist child result
```

- “分析板块”只执行简报链，不调用 `web_search` / `fetch_page`，也不自动进入调查。
- 用户从简报研究问题点击“深入调查”或自填问题后，才执行调查链。
- 一份简报可派生多份调查；调查失败不影响父简报。
- 简报和调查都沿用异步执行。每次 trigger 返回唯一 `job_id` 与 `job_kind`，前端按 job_id 轮询；同一版块任一 brief/investigation 正在运行时，其他版块任务触发返回 409 并携带当前 job，避免同板块并发改写状态与 LLM 成本失控。不同版块仍可并行。
- 备选否决：一次点击先简报后自动调查——仍会替用户选题，且默认成本/等待时间不可控。

### D3 简报使用独立契约，不再复用 thesis/argument/depth

`board_brief` 的 `sectors` 载荷：

```json
{
  "scope": "board",
  "result_kind": "board_brief",
  "summary": "1-3 句人话概览",
  "observations": [{"id":"o1","lane_id":1,"statement":"...","basis":"...","as_of_date":"..."}],
  "relationships": [{"lane_ids":[1,2],"type":"unclear","explanation":"...","confidence":"low","evidence_refs":["o1"]}],
  "uncertainties": [{"question":"...","why_uncertain":"...","needed_evidence":"..."}],
  "research_questions": [{"id":"q1","question":"...","rationale":"...","related_lane_ids":[1,2]}],
  "lane_refs": [{"lane_id":1,"note":"..."}]
}
```

- 默认上限：关键观察 5 条、关系 6 条、研究问题 4 条；避免把态势卡换一种形式全量复述。
- `observations` 是“内部新闻记忆观察”，不是外部核验后的绝对事实；basis 必须能回到 lane/as_of。
- 简报 parser 校验 lane id 属于当前版块、关系引用存在、枚举合法；坏 JSON 可重试一次，仍失败时机械降级为按质量排序的观察清单，不制造关系和问题。

### D4 调查先生成竞争假设，研究后可全部否决

调查先根据“用户问题 + 父简报观察/未知项”选择 0-2 张方法卡，不读取尚未生成的候选假设；随后用选中方法的证据检查清单辅助生成 2-4 个互斥度尽可能高的假设：

- 必须含 `is_null=true` 的零假设（例如“这些变化没有统一机制/可由普通因素分别解释”）。
- 每个假设先声明 `support_needed`、`disconfirm_needed` 与 `scope`，此时不选赢家。
- 一个共享研究 agent 接收全部假设与证据需求，统一调用内部工具、`web_search`、`fetch_page`，避免按假设各跑一套循环造成重复查询。
- 研究计划至少尝试一次中性查询和一次反证/替代解释查询；工具不可用或检索不到时记录 gap，不伪造完成。
- 最终综合可修改、合并、拆分或推翻假设，状态枚举为 `supported | plausible | insufficient | weakened | refuted`；允许所有非零假设均为 `insufficient/refuted`。

调查载荷：

```json
{
  "scope":"board",
  "result_kind":"board_investigation",
  "parent_briefing_id":123,
  "question":{"id":"q1","text":"...","source":"generated|custom"},
  "hypotheses":[{
    "id":"h0","label":"...","is_null":true,"assessment":"plausible","confidence":"medium",
    "support_evidence":[],"counter_evidence":[],"gaps":[],"scope":"..."
  }],
  "conclusion":{"summary":"...","confidence":"medium","scope":"...","boundary":"..."},
  "evidence_chain":[],
  "lane_refs":[],
  "method_refs":[]
}
```

### D5 result 表继续复用，但增加明确种类与父子关系

- 保留 `analysis_scope=topic|board` 作粗粒度兼容；新增 `result_kind`：`topic_analysis | board_brief | board_investigation | legacy_board_analysis`。
- 新增 nullable `parent_result_id` 自关联与 nullable `question_key`；仅 `board_investigation` 必须指向同版块的 `board_brief` 并带 question_key。
- `question_key` 为规范化问题文本（trim、连续空白折叠）哈希；generated/custom 采用同一算法，原始问题文本仍完整保存在 sectors。`parent_result_id + question_key` 定义“同一问题重跑”，不把显示用 question id 当跨请求身份。
- 旧 topic 行回填 `topic_analysis`；现有论文式 board 行回填 `legacy_board_analysis`。旧 JSON 不改写。
- board review 只在同 kind 内比较：简报对比上一份简报；调查仅在同一父简报、同一 question_key 下重跑时比较，否则不自动跨问题 review。
- QA 继续按 result id 工作，简报、调查和 legacy 报告均保持不可变。
- 备选否决：为简报/调查新建两张结果表——会重复 QA、review、日志与列表能力。

### D6 参考角色迁移为分析方法卡，按需选 0-2 张

- 新表 `analysis_methods(id, name unique, title, summary, selection_meta jsonb, content text, enabled, created_at, updated_at, deleted_at)`；删除采用软删除，历史结果不因卡片删除失去引用语义。
- `selection_meta` 至少包含 `applicable_when[]`、`avoid_when[]`、`required_evidence[]`、`failure_modes[]`；`content` 只写操作步骤与检查清单，不承载人物口吻。
- 选择器先只读取 enabled 卡片的名称、摘要与 selection_meta，基于调查问题选 0-2 张；选中后才加载全文。没有适配方法时选 0 张，调查仍正常运行。
- 简报阶段禁止注入方法卡；调查的假设生成与最终综合可注入选中卡，研究 agent 只接收由方法卡派生的证据需求，不接收作者修辞文本。
- 结果中的 method_refs 固化 `{id,title,content_hash}`，实际注入正文与选择理由固化进 input_snapshot；卡片后续编辑、停用或软删除不改变历史调查的可回放内容。
- 新增 `/analysis-methods` CRUD。旧 `reference_roles` 表本期不删除；迁移时复制为 disabled legacy 方法记录，原始内容保留供用户人工整理。旧 API 可保留只读兼容别名一个版本，但不再参与 prompt。
- 《内部看美国·方法论画像》不得默认启用；若用户后续整理，应拆成有明确适用边界的方法卡，剔除金句、人格和固定句式。

### D7 证据纪律从“数量配额”改为“适配与对抗”

- 取消“深度层尽量覆盖 ≥3 类证据”的硬引导；证据类型数量不是质量代理，缺历史材料时不强行类比。
- 优先级：与问题直接相关的一手依据 > 可核查二手材料 > 背景新闻；每条证据标明它支持或削弱哪个假设。
- `source_type=news|web|page|lane` 与可选 `kind=quote|series|chart` 保留，lane 仍校验属于活跃集合。
- 历史类比、系统重定位、机制分层都是可选方法产物，不是结果必填字段。

### D8 前端采用“先摘要、后研究”的渐进展示

- `BoardEnrichmentPanel` 默认展示最新简报：summary → 关键观察 → 关系 → 不确定项 → 候选问题。
- 每个候选问题提供“深入调查”；同时提供自填问题入口。调查运行状态与父简报隔离。
- 调查卡先显示 question、conclusion、confidence、boundary；竞争假设、支持/反证、gaps 与证据链默认折叠展开。
- 同一内容不再以 argument.layers 和 depth.mechanism_layers 重复渲染。人话契约：摘要先说具体变化；抽象概念首次出现必须解释它对应的行为/指标；固定“不是…而是…”不得作为模板要求。
- legacy board 报告继续由旧组件只读渲染，并明确标为“旧版分析”。

### D9 API 保持版块根路径，显式区分简报与调查

- 现有 `POST /semantic-boards/:id/enrichment/analysis/trigger` 改为触发简报，202 响应带 `{job_id, job_kind:"board_brief", scope, target_id}`。
- 新增 `POST /semantic-boards/:id/enrichment/analysis/investigations/trigger`，body 为 `{briefing_result_id, question_id? , question?}`；202 响应带独立 job_id 与 `job_kind=board_investigation`。
- 新增按 `job_id` 查询的状态入口，返回 running/finished/error/result_id/job_kind；保留按 board 查询当前/最近任务的兼容状态入口，供重进页面恢复轮询。
- 同一 board 的 brief/investigation 共用 active key 串行；409 响应返回当前 job_id/job_kind，前端恢复该任务轮询而不是误把 investigation 当 brief。
- `GET .../analysis/results` 增加 `kind` 过滤；详情返回 result_kind、parent_result_id 与 question_key。
- 单泳道 trigger 保留可选 `prefill_lens`，来源改为简报观察/关系或调查问题，用户可修改。

### D10 态势卡、补全门、质量信号和 lane 引用复用当前实现

- 简报与调查都从同一份已补全态势卡出发；调查可按需下钻 lane，并能读取 month/year 历史背景记忆。
- 态势卡事实源沿用 `week → month → section_fingerprint → description → none`，保证 month 补全成果确实被消费；section fallback 必须含实质事实而不是“泳道名×篇数”。
- 分析前补全门只检查 month/year 的有数据周期：缺行（含首份）先建，已有行最后写于 72h 前则重算；week 退出检查集，近期事实由 14 天窗口承担。单次最多 40 次、串行、失败或预算溢出均降级，`as_of_date` 不得晚于当前时刻。
- 质量信号只决定卡片详略和观察排序，不能作为“关系成立”的证据。
- 版块无活跃泳道时拒绝；全部稀疏时简报仍可生成观察不足说明，但不自动产生调查问题。

### D11 review 与历史认知按产物种类隔离

- 新简报与上一份 `board_brief` 对比，记录新增观察、消失信号、关系置信变化；不得再比较 thesis。
- 调查 review 以 `parent_result_id + question_key` 识别同一问题重跑，记录假设状态与证据变化；不同问题不互比。
- 历史 applied review 只能作为“曾经哪里判断失误”的提醒，不能直接成为本次事实或预选结论。
- review 继续不回写 lifeline。

### D12 综合 JSON 只允许可证明的单一根终止符修复

- 正常路径仍先走 `ParseJSONResponse` 与完整结构校验；provider 的 nil/空白正文在 airouter 边界记 `empty_response` 失败并走 ordered fallback。
- 真实慢供应商 fallback 暴露一种窄故障：输出已包含完整 `hypotheses/conclusion/evidence_chain/lane_refs`，字符串与全部内部对象/数组均闭合，但只漏掉根对象最后一个 `}`。此时可在综合专用 parser 边界追加**恰好一个** `}`，随后仍执行同一份严格 schema/证据/lane 校验，并在 generation meta 留稳定 repair reason。
- 修复判据必须机械可证：去 fence 后从首个 `{` 开始；扫描过程中无括号错配、不停在字符串/转义内；扫描结束 delimiter stack 恰好只剩根 `{`；最后有效字符是 `]`；补全后能被 `encoding/json` 解析，且顶层包含 `lane_refs`。缺内部括号、字符串被截断、错配、尾随正文或缺顶层末字段均不得修复，继续既有一次纠错重试；两次仍失败保持 0 调查行。
- 不把该修复放进通用 airouter/OpenAI client，也不扩大为任意 JSON 自动补全，避免把真正截断的研究结论误当成功。

### D13 跨 provider 的 lane 证据别名归一后仍守住评估一致性

- 调查 evidence 的规范持久字段仍是字符串 `ref`。兼容 fallback 模型常见的 `lane_id` 数值别名，但仅在 `source_type=lane` 且 `ref` 为空时读取；值必须是正整数且位于 JSON 安全整数域，再转为十进制 `ref`，随后照常经过父简报 lane 白名单。若 `ref` 已存在则它优先，冲突/越界/非整数 `lane_id` 不得掩盖非法 `ref`。
- 证据逐项清洗与去重后，hypothesis 的 support/counter 引用只保留存活证据；同时以证据自身的 `supports/counters` 极性做确定性并集合并，避免同一响应两处冗余引用轻微漂移造成 UI 悬空。
- 最终状态必须与存活证据相容：`supported` 至少一条 support；`refuted`/`weakened` 至少一条 counter。清洗后若不满足，视为结构失败并进入既有一次纠错重试；第二次仍失败保持 0 行，不允许“证据链为空但已证实/已推翻”的调查落库。`insufficient` 与保守的 `plausible` 仍可在明确 gaps/boundary 下无直接证据。
- 前端零证据空态只陈述“没有通过核验、可展示的证据”，不得断言研究没有采到材料；原始工具调用和被剔除原因留在审计日志/input snapshot，不在 UI 猜测具体原因。

## Risks / Trade-offs

- [两阶段增加一次用户操作] → 候选问题直接给“深入调查”按钮并保留自填入口；换取用户选题权与成本可控
- [简报可能显得不够“深”] → 以关系、未知项和研究问题体现判断力；需要深度时显式调查，而不是默认堆术语
- [多假设增加 prompt 与 schema 复杂度] → 限 2-4 个假设、一个共享研究循环、固定 assessment 枚举
- [LLM 仍可能把共现误写成因果] → 关系枚举区分 possible/context/unclear，parser + 审计 prompt 要求依据，前端显示置信度
- [方法选择器误选] → 最多 2 张、允许 0 张、avoid_when 优先；method_refs 落结果便于追溯
- [旧 API 调用方收到新 schema] → 项目为同仓前后端，要求同批部署；result_kind 显式分支，旧结果继续可读
- [旧角色资料迁移后失效] → 原文非破坏保留为 disabled legacy，UI 明示需整理后才能启用
- [父子结果继续复用同表使 JSON 多形] → result_kind 为强分派字段，repository/API 必须按 kind 过滤，旧 scope 仅作兼容

## Migration Plan

1. 新增 `result_kind`、`parent_result_id`，按 `analysis_scope` 与旧 sectors 形状回填；不改写旧 sectors。
2. 新建 `analysis_methods`；将 `reference_roles` 复制为 disabled legacy 记录，保留旧表用于回滚，本期不删除用户内容。
3. 后端先具备新旧 result 解析能力，再切换 trigger 为简报并开放调查端点。
4. 前端同批切换到简报/调查工作台；历史列表将 `legacy_board_analysis` 交给旧组件。
5. 观察新简报与调查稳定后，后续 change 再决定是否移除 `/reference-roles` 兼容别名和旧表。
6. 回滚时恢复旧 trigger 路由和旧前端；新增列/表可保留为空，不影响旧 topic/board 行。
