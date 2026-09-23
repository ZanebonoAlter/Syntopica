# 阶段2b交付报告：研究闭环（phase-2b-report）

> 2026-09-22，develop 主仓库直改（树上其他 change 脏改未触碰、未还原）。范围=tasks 3.3/3.4/3.5/4.1/4.3/4.4（research 侧）/4.5；阶段1 与 2a 合同的行为零改动（两处**纯增量**扩展见⑥①，备案请裁决）。发现侧 2a 交付未重做；真实 AI 验收（6.T5）按约不做，本批全 stub。

## ① 改动文件清单

**新增（service 层）**

| 文件 | 内容 |
| --- | --- |
| `service/signal_research.go` | 研究闭环：`GenerateSignalResearchSessionID`（board_signal_report_{id}_{hex8}）、cutoff 过滤包装层（`filterSignalToolResult`：四种期粒度「期结束≤cutoff 才保留」+期间不可解析剔除+observation_id 打标+filter_meta）、研究账本（calls/calcs/gaps+观测索引，appendix 唯一事实源）、`signalResearchPolicy`（toolLoopPolicy + toolLoopActionRunner：question 必填/四源白名单/calculate 动作/非法动作拦截不执行但计轮）、`buildSignalResearchRegistry`、研究 prompt（问题驱动+找反证+缺口披露）、`SignalResearchService.ResearchCandidate`（冻结快照→40轮 loop→收束分类→compose→保存） |
| `service/signal_calculation.go` | 受限计算解释器：difference/percent_change/mean，同系列/同单位/批准流量对（imports−exports）兼容校验，null→missing 不填 0，base≤0 拒绝，big.Rat 精确计算+half-up(away from zero) 4位去末尾0，calc_id 代码分配 |
| `service/signal_compose.go` | 成文：LLM 契约校验（四段唯一序/implication 五字段+direction 枚举/引用可解析/可见<3000 非空白字符/图表 0~3 张与兼容性）、有界重试≤3、appendix 代码生成、`SignalReportPayload`（sectors jsonb 形状） |
| `service/signal_research_test.go` | 16 用例：LP-1~7 + cutoff 过滤 + 50点不截 + allowedTools 钉住 + calculate 动作校验 + NR-1/2 |
| `service/signal_calculation_test.go` | 9 用例：CA-1~7 + 舍入格式表驱动 + 同期间同系列拒绝 |
| `service/signal_compose_test.go` | 9 用例：SV-1~7 + appendix 代码生成（伪造被忽略）+ 用户消息携带账本 |

**新增（handler 层）**

| 文件 | 内容 |
| --- | --- |
| `handler/signal_research.go` | `POST /signals/:candidateId/research`（404/400/409/200复用/202）、`GET /signal-reports`（游标分页+source_signal_id 归属校验）、`GET /signal-reports/:rid`（owner/kind 404 语义，序列化无 review 键）、`SignalResearchRunner` 接口 + post-construction 装配（SetSignalResearch[OnInstance]，与 2a 同款） |
| `handler/signal_research_test.go` | 9 用例：API-3~6、S2、JB-2/4（research 侧）、报告列表/详情契约、旧 review 路由保持 |

**增量编辑（共享文件，全部向后兼容）**

| 文件 | 改动 |
| --- | --- |
| `service/orchestrator.go` | ① 新增可选接口 `toolLoopActionRunner`（RunLoopAction）；② runToolLoop default 分支在**policy 实现该接口时**交由其处理动作，否则走原错误返回——investigation policy（未实现该接口）与 policy=nil 旧路径字节不变（不 fork 循环） |
| `handler/analysis_runner.go` | `SignalJobPatch` 增量补 `ResultID` 字段 + patchJob 应用 + 终态回写仅在 fn 返 0 时保留 patch 值（旧 Start 路径 fn 返回值优先，行为不变）。**⑥①裁决备案** |
| `handler/handler.go` | EnrichmentHandler 加 `signalResearch` 字段；board 分析组注册 3 条新路由 |
| `handler/board_enrichment_handler.go` | 新增 `parseBeforeID`/`parseListLimit` 两个列表参数 helper（parseBoardID 旁，纯新增） |
| `repository/signal_repository.go` | 新增 `GetSignalDiscoveryByID`（研究读取冻结 InputSnapshot 的必要读路径；纯新增查询，既有查询零改动） |
| `wire.go` | 增量装配 `signalResearch := service.NewSignalResearchService(router, CapabilityAnalysis, toolRegistry, repo)` + `handler.SetSignalResearchOnInstance(...)`（toolRegistry 为既有共享 registry，四源已注册，研究侧经 cutoff 包装层消费） |
| `openspec/changes/board-signal-reports/tasks.md` | 勾选 3.3/3.4/3.5/4.1/4.3/4.4/4.5（各带证据） |

## ② research/report API 契约（给阶段3 前端）

### 路由（挂现有 board 分析组，与 discovery 共享 202/409 互斥）

```
POST /api/semantic-boards/:id/enrichment/analysis/signals/:candidateId/research
GET  /api/semantic-boards/:id/enrichment/analysis/signal-reports?granularity=&period=&before_id=&limit=[&source_signal_id=]
GET  /api/semantic-boards/:id/enrichment/analysis/signal-reports/:rid
```

job 轮询沿用 `GET /api/enrichment/analysis-status?job_id=`。

### POST /signals/:candidateId/research

- 请求 body 可缺省：`{"regenerate": false}`（仅此一个语义开关；其余浏览器自带字段一律忽略——服务端快照权威，API-4）。非法 JSON → 400 无 job。
- **404**：候选不存在或 `candidate.semantic_board_id != :id`（跨板块不暴露存在性）。
- **400**：板块增强开关未开启（同旧 trigger 文案，含 `not enabled` 可机判前缀）。
- **409**（先于 200 复用判断，design §7 顺序）：同板块任一任务在跑（含自身候选重复点击），`data` 携 running job 完整身份（`job_kind` 可区分是谁在跑）。
- **200**（幂等复用）：无 running 且该候选已有成功报告且 `regenerate!=true` → `data = {status:"already_reported", result_id}`，零新 LLM。
- **202**：`data = {status:"started", job_id, job_kind:"board_signal_report", scope:"board", target_id, candidate_id, granularity, period}`。

### research job 状态机（board_signal_report 专用字段，旧 kind 输出零变化）

| 字段 | 时机 | 值 |
| --- | --- | --- |
| `candidate_id` / `granularity` / `period` | 启动即报 | 候选身份（派生「研究中」依赖 candidate_id patch） |
| `phase` | 运行中 | `research`（40轮研究 loop）→ `compose`（≤3次成文）；终态停留最后 phase |
| `outcome` | 终态 | `succeeded` / `failed`（预算耗尽**不是** failed——stop_reason 在 result 的 generation_meta 里） |
| `error_stage` | 仅 failed | `research` / `compose` / `save` |
| `result_id` | 仅 succeeded | 落库后的不可变报告 id（JB-2；经 SignalJobPatch.ResultID 上报） |

前端判定：`finished && outcome=="succeeded"` → 用 result_id 打开报告详情；`failed` → 错误态，候选回 `pending` 可重试；重启后 job_id 轮询 404 → 停止轮询提示可重试（候选/报告仍在）。

### GET /signal-reports（仅成功报告）

- 参数：`granularity`（month|year）、`period`（必填，形状随粒度）、`before_id`（结果 id 游标，排他）、`limit`（默认 20，>100 截断 100，非法 400）、`source_signal_id`（可选：过滤某候选的版本序列；候选不存在/跨板块 → 404）。
- 响应 `data`：数组，id 倒序。每行：`{id, analysis_scope, result_kind:"signal_report", semantic_board_id, granularity, period, source_signal_id, sectors, session_id, created_at}`。列表行**不含** tool_calls/input_snapshot（完整工具日志只在详情暴露）。

### GET /signal-reports/:rid（详情）

- owner（板块不匹配）/kind（非 signal_report）不匹配一律 404。
- 返回列表行字段 + `tool_calls`（完整原响应工具日志）+ `input_snapshot`（发现时冻结材料）。
- **整个响应无任何 review/judge/approved/digest 字段**（测试对响应体整体断言）。

### sectors payload（前端渲染合同）

```jsonc
{
  "schema_version": 2,
  "signal_snapshot": { "candidate_id", "discovery_id", "signal", "why_it_matters", "research_question",
                        "evidence_refs", "score", "rationale", "granularity", "period", "analysis_mode", "cutoff" },
  "report": {
    "title": "判断式标题",
    "sections": [ // 恰四段、按序唯一
      {"kind":"thesis","text":"…"}, {"kind":"facts","text":"…"}, {"kind":"causal","text":"…"},
      {"kind":"implication","text":"…","verdict":"…","direction":"up|down|diverge|conditional",
       "horizon":"…","trigger_condition":"…","self_doubt":"…"}
    ],
    "charts": [ {"chart_id","kind":"line|comparison","claim","refs":["c1:o1","k1"]} ]  // 0~3 张；折线已按期间升序排序
  },
  "appendix": { "calls": […], "calculations": […], "gaps": […] },   // 见③，代码生成
  "generation_meta": { "session_id","attempts","retries","decisions","source_calls","calculation_calls",
                        "stop_reason":"finished|budget_exhausted","analysis_mode","cutoff" }
}
```

**引用渲染约定（FE-7/FE-8）**：正文/图题中的 `[[data:c1:o2]]`、`[[calc:k1]]`、`[[news:<切片ID>]]` token 原样存储；展示数值由前端从 appendix 查表渲染（观测行有 value/unit/raw_value/missing_reason，计算行有 value（规范十进制字符串）/unit）——模型永远不提供展示数值。ref 形态即来源标记：`cN:oM`=原值观测、`kN`=代码计算、`[[news:]]`=新闻背景（展示数据来自 signal_snapshot.evidence_refs 对应的候选依据）。stop_reason=budget_exhausted 时前端应展示缺口提示（正文已按要求披露）。

## ③ loop/compose 内部结构说明

### research loop（3.3/3.4）

- **挂点**：共享 `runToolLoop`（orchestrator.go），maxLoops=40 经 `toolLoopParams`。`signalResearchPolicy` 实现 `toolLoopPolicy`（CheckCall：非四源工具→`tool_not_in_research_whitelist` 拦截、缺 question→`missing_question` 拦截；ObserveCall：真实执行后把 question+观测写入账本；CheckFinish：无机械配额，可随时收束）。新增**可选** `toolLoopActionRunner` 接口（type assertion，仅 default 分支咨询）：signal policy 借此在循环内处理 `calculate` 动作与一切非法动作（blocked 记录+反馈+计轮）——**不 fork 循环**，investigation policy 与无 policy 调用方字节不变。
- **预算**：总决策≤40 轮（maxLoops）；每轮至多一个动作 → 总执行≤40 机械保证；失败调用/被拒计算计执行，blocked（形状非法/重复）不计执行只计轮。收束分类机械判定：`FinalData≠""`→finished；`policy.lastStep==40`→budget_exhausted（以已有证据成文，正文披露缺口）；其余（LLM 错误/解析失败）→ failed；ctx 取消/超时→failed（不伪装）。
- **三防御**：/no_think 前缀、ResultFull 完整不截断、dedupKeyFor 重复拦截全部来自共享循环本体；测试逐项断言。
- **cutoff 过滤先于 agent**：`buildSignalResearchRegistry` 克隆四源工具包 Execute——保留观测打 `observation_id`（"c1:o2"），`filter_meta={cutoff,kept,dropped}` 附在响应顶层；期粒度→期结束时刻：EIA 周结日零点、JODI 次月 1 日零点、WDI 次年 1 月 1 日、Comtrade A/M 同规则；**期结束≤cutoff 才保留**，不可解析剔除。完整原响应存账本并回写 `result.tool_calls`（工具日志）；附录载筛选后全集（60 点用例证不截 50）。
- **账本**：`signalResearchLedger` 持 calls（call_id/question/tool/args/status/error/retrieved_at/last_modified/source_sha256/documents/observations 全集/filter_meta + 私有 rawResponse）、calcs（解释器结果）、gaps（取数失败/无覆盖）、obsIndex（observation_ref → 系列身份/单位/期间/流量/值）。这是 appendix 与 compose 引用校验的唯一事实源。

### calculate API（3.5）

- 模型提交 `{"action":"calculate","op","inputs":["c1:o1",…],"question"}`；policy 前置拦截：缺 question/带 value（模型不能提交计算值）/未知 op/空 inputs/重复请求 → blocked。合法请求进解释器：`runSignalCalculation(obsIndex, req, calcID, now)`。
- 兼容规则：difference=同系列不同时点 **或** 同源同期间批准流量对（imports−exports，保守起步）；percent_change=同源同系列同单位且 base>0；mean=同系列互异期间集合，任一缺期→missing（不称完整窗口）。跨源/混单位/库存减流量→rejected。null 输入→missing+原因，绝不填 0。
- 输出记录 `{calc_id, op, inputs, expression, value, unit, precision:4, status: ok|missing|rejected, reason?, computed_at}`；value 为规范十进制字符串（half-up away-from-zero 最多 4 位、去末尾 0）；percent_change 的 unit=`%`。

### compose（4.1）

- `SignalComposer.Compose`：≤3 次尝试（初次+2 重试，retries=attempts-1），每次失败把校验问题回注给下一稿；最终失败返回**最后一次校验错误**（不被后续 chat 错误掩盖），job 转 failed 无 result。
- 校验：四段唯一序；每段 text 非空；implication 五字段非空+direction 枚举；标题+正文+图题+后果段结构化字段合计 <3000 非空白 Unicode 字符（附录/UI 不计，2999/3000 边界有测试）；全部 `[[…]]` 引用可解析（观测在索引、计算 status=ok、news 在候选白名单）；charts 0~3 张、kind 枚举、claim 非空、refs 非空且可解析、同源同单位、折线同系列且≥2 非空点并按期间升序重排（null 断点保留不补零）、计算点期间取输入最大期。
- prompt 基线=sample-report.md 大白话：先说结论→发生了什么→意味着什么→接下来怎么看；标题即判断；禁行话替代解释；禁手写统计数值（只允许引用占位符）；竞争解释摆进 causal；自我质疑融入 implication；证据不足用 conditional 维持判断，不强迫方向。

## ④ 测试结果（命令+退出码+用例数）

| 命令（cwd=backend-go） | 退出码 | 说明 |
| --- | --- | --- |
| `go test -short ./internal/dataenrichment/... -count=1` | 0 | change-scope 判定影响包（本 change 路径全落 dataenrichment 域）；4 包 ok，含本批 43 个新测试函数与既有全量回归（旧 kind 状态输出、brief/investigation/topic/review 既有用例全绿） |
| `go test ./internal/dataenrichment/repository/ -run "TestSignal|TestBoardSignal" -count=1` | 0 | 隔离 PG（testcontainer）：新增 GetSignalDiscoveryByID 与既有约束/迁移用例共存（非 -short 真跑 PG） |
| `go test ./internal/dataenrichment/handler/ -short -run "TestSignalResearch|TestSignalReports" -count=1` ×3 连跑 | 0 | handler 新用例稳定性（9 用例 ×3 全绿） |
| `golangci-lint run ./...` | 0 | 0 issues |
| `go vet ./...` | 0 | — |
| `go build ./...` | 0 | — |

新用例计数：service 层 signal_research_test 16 + signal_calculation_test 9 + signal_compose_test 9 = 34；handler 层 signal_research_test 9；合计 **43**。全部 stub LLM/工具/store（不起真模型、不真实外网、无 SQLite 于 service/repository 层；handler 层按 testing.md 用内存 SQLite）。

## ⑤ 未做项/残余风险

- **真实 AI 验收（6.T5/OB-3）不做**（按 brief）。合成 stub 无法证明研究质量/轮次分布/证据相关性——真实样本验收留给人工阶段，届时需记录取数次数/gap/结论证据链。
- **6.T1 的 datasources/其他域**：change-scope 输出里的 reader/tagmanagement/topicgraph/datasources 包映射来自**树上其他 change 的预存脏改**（本批零改动这些路径），按约束只跑了 dataenrichment 域。
- **`DiscoverSignals`/`ResearchCandidate` 的 now 取真实时钟**（2a 已备案）：研究侧周期一致性校验用候选/批次自带字段，不受时钟影响；无新增时钟缝需求。
- **材料/工具结果的上下文容量**：40 轮工具结果全量进历史（design §4 明确不截工具历史），超大窗口（EIA weeks=12 × 全 flow）叠加多次调用可能逼近上下文上限——超限时按设计显式 failed（LLM 容量错误→research stage failed 可重试），不静默丢证据。首版未做 prompt 侧预算裁剪，与 2a 的 detect 同款待观察项。
- **`[[news:]]` 引用的展示数据**在 signal_snapshot.evidence_refs（切片 ID）中，切片标题/日期不在报告 payload 里——阶段3 若要 tooltip 展示切片内容，需从候选列表接口已有的证据数据取，或后续给 snapshot 增补（本批未加，避免扩 design §7 形状）。
- **research 不重跑 freshness**（design §1/§4 明确）；若发现材料里的泳道摘要已过期，报告如实基于快照成文——这是设计选择，不是缺陷。

## ⑥ 裁决请求

1. **SignalJobPatch 增量补 `ResultID` 字段（已实施，请追认）**：2a 交付的 `StartSignal` 把 fn 返回值硬编码为 0，job fn 无法像旧 `Start` 那样把 result_id 带上 job 状态；而 4.3/JB-2 要求「outcome=succeeded 时 result_id 存在」。2a 报告自述「StartSignal 的 launch fn 已支持返回 resultID」与代码现状不符（launch 支持、StartSignal 包装层丢弃）。本批选择最小增量：SignalJobPatch 加 ResultID（omitempty、旧 kind 从不调用 report、终态回写 fn 返回值仍优先）——StartSignal/RunningSignalReportForCandidate 等已交付行为零变化。若 controller 偏好其它方案（如改 StartSignal 签名为返回 (uint, error)），可一行回退本扩展再重接，请裁决。
2. **repository 新增 `GetSignalDiscoveryByID`**：研究读取冻结快照的必要读路径，纯新增查询（与既有 GetSignalCandidateByID 同款 First 主键查），未动阶段1 任何既有行为。纯增量，随本报告知会。
3. **cutoff 比较的边界口径（已在代码注释与测试固化）**：统一「观测期结束时刻 ≤ cutoff 才保留」。EIA 周结日按当日零点（phase-1 契约「period < cutoff」的等价保守形式，仅在 cutoff 恰为周结日零点时相差一例）；JODI/WDI/Comtrade 用排他期末（次月 1 日/次年 1 月 1 日零点），历史目标周期的整月数据在 cutoff=周期终点处恰好相等故保留。若 controller 希望 EIA 也改成严格小于的另一种边界表述，属一行改动。
4. **Comtrade 观测的规范计算值**：观测行含 qty_kg/net_wgt_kg/value_usd 三种量，计算索引取 value_usd（单位 USD）为该行规范值，其余量随观测原样进附录展示；WDI 观测无单位列，同系列计算不受影响、跨系列由 series 判定拒绝。此为 design §5 未覆盖的两处保守落地，请知会（如需禁用 Comtrade 参与计算可一行关掉）。
5. **409 与 200 复用的顺序**：按 design §7「先共享 board 互斥，无 running 且已有成功报告才 200」实现——「已有报告 + 他任务在跑」时返回 409 而非 200。若阶段3 前端希望这种场景回 200 静默复用，需放宽 design 原文，请裁决（当前实现未擅改）。
