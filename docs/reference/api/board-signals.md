# 板块信号解读报告 Board Signals（board-signal-reports）

> 数据增强工作台主视图（2026-09-22 起）：两阶段人工流程——「发现信号」只识别并持久化候选，用户逐条点击「深入研究」才发起 40 轮预算研究，产出一篇无评审、数字可核查的解读报告。链路与业务约束见 [flow/data-enrichment.md](../flow/data-enrichment.md) §板块信号解读报告；表结构见 [database/tables/data-enrichment.md](../database/tables/data-enrichment.md)。旧版块简报/调查端点保持兼容（见 [dataenrichment.md](dataenrichment.md)），新工作台不调用。
>
> 通用约定（响应信封、错误格式）见 [_conventions.md](_conventions.md)。所有路由挂在现有 board 分析组下，与旧 brief/investigation **共享同板块 202/409 互斥**；job 轮询沿用 `GET /api/enrichment/analysis-status?job_id=`（未知 job_id → 404）。

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| POST | `/api/semantic-boards/:id/enrichment/analysis/signal-discoveries` | 触发信号发现（202 job 信封） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/signals` | 候选列表（周期内，派生状态 + 进展摘要） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/signals/:candidateId/research-progress` | 候选最近研究进展全量（含 ledger 账本） |
| POST | `/api/semantic-boards/:id/enrichment/analysis/signals/:candidateId/research` | 触发单候选深入研究（202/200 复用/409/404） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/signal-reports` | 信号报告列表（仅成功报告） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/signal-reports/:rid` | 信号报告详情（含工具日志/输入快照，无 review 字段） |

> 周期参数贯穿全部端点：`granularity` 固定 `month|year`（week/all 拒绝），`period` 形状随粒度（`YYYY-MM` 真实月份 / `YYYY` 2000~2100）；不晚于当前周期（业务时区 Asia/Shanghai）。历史周期合法，报告 `analysis_mode=retrospective`（事后回顾 · 本次取得的数据版本）。

---

## POST /signal-discoveries

触发一次信号发现。同步预检全部无副作用（不占 job 槽）：

| 状态 | 条件 |
| ------ | ------ |
| `400` | 周期非法/未来（`ParseSignalPeriod` 错误原文即消息）；板块未开启 `enrichment_enabled`（含 `not enabled` 可机判前缀） |
| `409` | 同板块任一任务在跑（与旧 brief/investigation/另一信号任务共享互斥），`data` 携当前任务完整身份（`job_kind` 可区分是谁在跑） |
| `202` | 已受理，discovery job 后台执行 |

**请求体**

```json
{ "granularity": "month", "period": "2026-08" }
```

**响应示例（202）**

```json
{
  "success": true,
  "data": {
    "status": "started",
    "job_id": "a1b2c3d4e5f6a7b8c9d4e5f6",
    "job_kind": "board_signal_discovery",
    "scope": "board",
    "target_id": 9,
    "granularity": "month",
    "period": "2026-08"
  }
}
```

job 内部：`prepare`（周期材料装配，历史期只取当时归属板块切片，无法还原归属记 gap）→ `detect`（≤2 次尝试；候选 `score` 门槛 6、证据仅限本次白名单、悬空引用剔除后无有效依据的信号丢弃）→ 原子保存批次+候选。**发现零取数/零计算/零成文**（detector 无工具注册表）。

---

## GET /signals

某板块某周期的候选列表，按批次/候选 id 倒序。空新批次不清旧记录（只查候选表）；空态=空数组。

**查询参数**

| 参数 | 必填 | 说明 |
|------|------|------|
| `granularity` | 是 | `month\|year`，非法 400 |
| `period` | 是 | 形状随粒度，不匹配 400 |
| `before_id` | 否 | 候选 id 游标（排他），非法 400 |
| `limit` | 否 | 默认 20，>100 截断为 100，非法 400 |
| `visibility` | 否 | `visible\|ignored\|all`（redesign-enrichment-research-workbench）：按忽略标记筛选；**省略 = all（旧客户端兼容语义）**，新工作台显式传 visible；非法 400 |

**响应示例**

```json
{
  "success": true,
  "data": [
    {
      "id": 3,
      "discovery_id": 2,
      "granularity": "month",
      "period": "2026-08",
      "signal": "美国商业原油库存连续三周超季节性去库",
      "why_it_matters": "……",
      "research_question": "……",
      "evidence_refs": ["31", "32"],
      "score": 8,
      "rationale": "……",
      "discovery_created_at": "2026-09-01T03:05:00Z",
      "status": "pending",
      "latest_result_id": null,
      "last_research_progress": {
        "rounds_done": 8,
        "source_calls": 9,
        "calculation_calls": 2,
        "status": "abandoned",
        "stop_reason": "timeout",
        "updated_at": "2026-09-22T10:00:00Z"
      }
    }
  ]
}
```

| 字段 | 说明 |
|------|------|
| `signal` / `why_it_matters` / `research_question` / `rationale` | detect 校验后原文（候选不可变） |
| `evidence_refs` | 新闻切片 id 白名单（发现批次 input_snapshot 内） |
| `score` | 1~10 整数（prompt 打分，≥6 才入选；展示层可忽略） |
| `discovery_created_at` | 发现时间来自批次（非候选） |
| `status` | **服务端派生，前端只读**：`pending`（待研究）/ `researching`（该候选有 live research job；不持久化，重启不残留）/ `reported`（已有成功报告） |
| `latest_result_id` | 仅 `reported` 有值，其余 `null`（可直接打开报告） |
| `last_research_progress` | 上次研究进展摘要（tasks 4.7「断了不能白跑」）：`rounds_done`/`source_calls`/`calculation_calls` 计数 + `status`（`running\|abandoned\|superseded`）+ `stop_reason`（仅失败行：`timeout`/error_stage/`failed`/`orphaned_by_restart`〔启动收敛的孤儿行，tasks 4.10〕，其余 `null`）+ `updated_at`；候选从未研究 → `null`。完整进展（含 ledger 账本）走进展端点 |
| `ignored_at` | 忽略标记时间（独立处理标记表 `board_signal_disposition`，redesign-enrichment-research-workbench D1）；未忽略 `null`。visible 视图恒为 null，ignored 视图非 null |

---

## GET /signals/:candidateId

候选详情（redesign-enrichment-research-workbench 2.2）：列表字段全量 + 忽略标记 + 派生状态/最新报告 + 最近研究进展 + 冻结边界 + 相关话题。只加载当前项（分页外选中项用）；**零 LLM**。

- 跨板块/不存在 → 404（不泄露存在性）。
- `cutoff`/`analysis_mode`/`discovery_created_at` 来自批次（冻结边界）；`related_topics` 由候选依据对冻结 input_snapshot 机械解析（SectionID→LaneID）并经当前话题表存在性/归属校验，归档话题照常返回；`related_gaps` 如实列出无法验证的引用/已删除或迁移话题（不猜回填）。
- `related_topics[]` 行：`{topic_id, label（当前名）, frozen_label, frozen_status, section_ids[]}`。

---

## PUT /signals/:candidateId/disposition

候选忽略/恢复（redesign-enrichment-research-workbench 2.2/D1）：幂等、零 LLM，不改候选内容/score/快照/报告。

**body** `{"ignored": boolean}`（必填；字符串布尔、未知字段（score/signal 等）一律 400——本端点永远改不了候选内容）。

| 状态 | 语义 |
|------|------|
| 200 | `{candidate_id, ignored, ignored_at}`（恢复时 ignored_at=null） |
| 400 | 缺 ignored / 类型错 / 未知字段 |
| 404 | 候选不存在或跨板块 |
| 409 | **同板块任一任务在跑**（discovery/research/旧 brief/investigation 共享互斥；运行中的忽略不可达，body 携 running 身份） |

幂等语义：重复忽略保留首次 `ignored_at`；恢复=删除标记行（未忽略时恢复也成功）。已忽略候选的旧报告 GET 不受限；新研究/显式重研 409（reason=`candidate_ignored`）要求先恢复，但无 running 且已有报告的默认 research 仍 200 幂等返回原 result（零新 LLM）。

---

## GET /signals/:candidateId/research-progress

候选最近一次研究的完整进展（tasks 4.7）：`job_id`/候选身份/周期 + 轮次与取数/计算计数 + **`ledger`**（全量账本 jsonb：`calls`/`calculations`/`gaps` 三段，与报告 appendix 同源结构）+ `status`/`stop_reason`/`error` + 时间戳。研究超时/失败后进展行保留（`status=abandoned`），重启后仍可查（进展在 DB，不随内存 job 蒸发）——**进程重启**（含被杀 job）的残留 `running` 行在后端启动时收敛为 `abandoned`+`stop_reason=orphaned_by_restart`（tasks 4.10），本端点不会再看到永久 `running`；成功落库报告后 `status=superseded` 归档保留（不删、不覆盖报告账本）。本端点只读，不触发任何新研究。

| 状态 | 条件 |
| ------ | ------ |
| `404` | 候选不存在或跨板块（不暴露存在性） |
| `200` `data=null` | 候选存在但从未研究过 |
| `200` | 返回最近进展全量行 |

---

## POST /signals/:candidateId/research

触发单候选深入研究。一次研究只服务一条候选、最多生成一篇报告；多候选中点一条只研究那一条。

**请求体**（可缺省；浏览器自带 `cutoff`/`granularity`/`period`/`signal` 等字段一律忽略——服务端冻结快照权威）

```json
{ "regenerate": false }
```

| 状态 | 条件（按此顺序判定） |
| ------ | ------ |
| `400` | body 非法 JSON（无 job） |
| `404` | 候选不存在或 `candidate.semantic_board_id != :id`（跨板块不暴露存在性） |
| `400` | 板块未开启 `enrichment_enabled` |
| `409` | **先于 200 复用判断**：同板块任一任务在跑（含自身候选重复点击），`data` 携 running job 完整身份 |
| `200` | 无 running 且该候选已有成功报告且 `regenerate != true` → 幂等复用，零新 LLM |
| `202` | 否则启动新 research job |

```json
// 200 复用
{ "success": true, "data": { "status": "already_reported", "result_id": 12 } }
// 202
{ "success": true, "data": { "status": "started", "job_id": "…", "job_kind": "board_signal_report",
  "scope": "board", "target_id": 9, "candidate_id": 3, "granularity": "month", "period": "2026-08" } }
```

研究链：读取候选冻结快照（不重新装配、不偷换最新材料，不重跑 freshness）→ 40 轮问题驱动 loop（七源取数 + `calculate` 合计 ≤40 次执行，可提前收束；add-fast-frequency-datasources 扩 yahoo_chart/fred_series/gdelt_doc 三快频源，报告头「数据截至」支持日度期值）→ `compose` ≤3 次尝试 → 成功保存不可变报告。失败 job 可重试（重试仍需用户点击，无自动重试）。研究 job 超时独立为 **150 分钟**（与 40 轮预算对齐：实测单轮 108~380s；发现保持 30 分钟），超时/失败时已完成轮次的进展（轮次计数+账本）持久化到 `board_signal_research_progress` 保留可查（见上节），不随 job 终止丢失。

---

## job 状态字段时机表（`GET /api/enrichment/analysis-status?job_id=`）

signal job 专用字段（全部 `omitempty`，旧 kind 输出零变化）：

### discovery job（`job_kind=board_signal_discovery`）

| 字段 | 出现时机 | 值 |
| ------ | ------ | ------ |
| `phase` | 运行中 | `prepare` → `detect`；终态停留最后 phase |
| `outcome` | 终态 | `discovered`（候选 ≥1）/ `no_signal`（0 条，正常完成）/ `failed` |
| `error_stage` | 仅 failed 且 stage 可知 | `prepare` / `detect` / `save` |
| `discovery_id` | 终态 | 批次 id |
| `candidate_count` | 终态 | 保存候选数（`no_signal` 时为 0，键省略） |
| `result_id` | **永不出现** | 发现不带报告 id、不启动研究 |

前端判定：`finished && outcome=="no_signal"` → 安静空态不清列表；`failed` → 错误态可重试；`discovered` → 刷新候选列表。

### research job（`job_kind=board_signal_report`）

| 字段 | 出现时机 | 值 |
| ------ | ------ | ------ |
| `candidate_id` / `granularity` / `period` | 启动即报 | 候选身份（派生「研究中」依赖 candidate_id） |
| `phase` | 运行中 | `research`（40 轮 loop）→ `compose`（≤3 次成文）；终态停留最后 phase |
| `outcome` | 终态 | `succeeded` / `failed`（**预算耗尽不是 failed**——`stop_reason` 在报告 `generation_meta` 里） |
| `error_stage` | 仅 failed | `research` / `compose` / `save` |
| `result_id` | **仅 succeeded** | 落库后的不可变报告 id |

**重启恢复语义**：job 表在内存（单实例单用户），进程重启后原 job_id 轮询返回 404——前端应停止轮询、提示可重试并重拉列表；候选派生状态自动回落 `pending`（不残留「研究中」），成功报告仍可查。同板块 409 冲突体携 running job 身份，前端按其 `job_kind` 恢复对应轮询（`board_signal_report`/`board_signal_discovery` 接管；旧 brief/investigation 仅提示）。

---

## GET /signal-reports

某板块某周期的成功报告列表（id 倒序）。

**查询参数**：`granularity`、`period`（必填，同上）、`before_id`（结果 id 游标，排他）、`limit`（默认 20 最大 100）、`source_signal_id`（可选：过滤某候选的版本序列；候选不存在/跨板块 → `404`）。

**响应行**（列表**不含** `tool_calls`/`input_snapshot`，完整工具日志只在详情）：

```json
{
  "id": 12,
  "analysis_scope": "board",
  "result_kind": "signal_report",
  "semantic_board_id": 9,
  "granularity": "month",
  "period": "2026-08",
  "source_signal_id": 3,
  "sectors": { "schema_version": 2, "…": "见下节" },
  "session_id": "board_signal_report_3_ab12cd34",
  "created_at": "2026-09-01T03:20:00Z"
}
```

## GET /signal-reports/:rid

报告详情：返回列表行全部字段 + `tool_calls`（完整原响应工具日志，含被 cutoff 剔除的观测）+ `input_snapshot`（发现时冻结材料）。owner（板块不匹配）或 kind（非 `signal_report`）不匹配一律 `404`，不泄漏存在性。**整个响应无任何 review/judge/approved/digest 字段**（新链无评审）。报告不可变：重新研究（`regenerate=true`）成功后追加新版本行，旧行不动。

---

## sectors payload（`schema_version=2` 存量 / `schema_version=3` 新产物）

新报告一律 schema_version=3（redesign-enrichment-research-workbench 3.1/D4）：sections 携受控块；`text` 为代码生成的兼容四段文本（旧 reader 可读，不重复计入篇幅预算）；顶层新增 `data_freshness` 机械分源时效。schema2 存量载荷不重编、不改写，按原四段文本兼容阅读；未知/损坏载荷显式兼容错误，不伪装空报告、不触发 AI 修复。

```jsonc
{
  "schema_version": 3,
  "signal_snapshot": {
    "candidate_id": 3, "discovery_id": 2,
    "signal": "…", "why_it_matters": "…", "research_question": "…",
    "evidence_refs": ["31", "32"], "score": 8, "rationale": "…",
    "granularity": "month", "period": "2026-08",
    "analysis_mode": "current",        // current | retrospective（历史期事后回顾）
    "cutoff": "2026-09-01T03:05:00Z"   // 数据截止边界
  },
  "report": {
    "title": "判断式标题",
    "sections": [                       // 恰四段、按序唯一
      { "kind": "thesis", "text": "…", "blocks": [ // schema3 受控块（三种平面类型，不嵌套）
        // {"type":"paragraph","text":"…（引用占位符）"}
        // {"type":"evidence_table","title":"关键观测","rows":[{"label":"同口径观测组","refs":["c1:o1","c1:o2"]}]}
        // {"type":"chart","chart_id":"ch1"}
        // 规则：thesis/implication 只放单 paragraph；表/图只置于 facts/causal；
        // 行内同口径（同单位）；行 label 不得复制账本数值；chart 块挂已定义
        // chart_id 且每图最多挂一处；未知块类型/未知字段整稿拒绝回注
      ] },
      { "kind": "facts", "text": "…（代码生成兼容文本）", "blocks": [ /* … */ ] },
      { "kind": "causal", "text": "…", "blocks": [ /* … */ ] },
      { "kind": "implication", "text": "…", "verdict": "…",
        "direction": "up|down|diverge|conditional",
        "horizon": "…", "trigger_condition": "…", "self_doubt": "…" }
    ],
    "charts": [                         // 0~3 张；0 图合法（无可绘制序列 + gap）
      { "chart_id": "chart-1", "kind": "line|comparison", "claim": "…",
        "refs": ["c1:o1", "k1"] }       // 折线已按期间升序排序；null 断点不补零
    ]
  },
  "appendix": {                         // 代码生成，LLM 不得生成/覆写
    "calls": [ /* call_id/question/tool/args/status/error/retrieved_at/last_modified/source_sha256/documents/observations 全集/filter_meta */ ],
    "calculations": [ /* 见下节计算记录 */ ],
    "gaps": [ /* 取数失败/无覆盖等诚实缺口 */ ]
  },
  "generation_meta": {
    "session_id": "board_signal_report_3_ab12cd34",
    "attempts": 1, "retries": 0,        // 成文尝试/重试
    "decisions": 17,                    // 总决策轮数（≤40）
    "source_calls": 16,                 // 真实取数执行次数（含失败）
    "calculation_calls": 0,             // 计算执行次数
    "stop_reason": "finished",          // finished | budget_exhausted（耗尽≠失败，正文披露缺口）
    "analysis_mode": "current", "cutoff": "2026-09-01T03:05:00Z"
  },
  "data_freshness": {                   // 仅 schema3：机械分源时效（代码生成，非模型产物）
    "sources": [ { "tool": "eia_wpsr_table1", "latest": "2026-07" },
                 { "tool": "yahoo_finance", "latest": "2026-10-01" } ],
    "cutoff": "2026-09-01T03:05:00Z",  // 研究时点（材料边界），不混报告生成时间
    "generated_at": "2026-10-02T03:05:00Z",
    "has_observations": true            // 零观测 → 如实 false，不用 cutoff 冒充数据期
  }
}
```

**引用渲染约定**：正文/图题中的 `[[data:c1:o2]]`（原值观测）、`[[calc:k1]]`（代码计算）、`[[news:<切片ID>]]`（新闻背景）token 原样存储；展示数值由前端从 appendix 查表渲染（阅读层短格式，原值/raw 在核查区逐项保留）——观测行有 `value`/`unit`/`raw_value`/`missing_reason`，计算行有 `value`（规范十进制字符串）/`unit`。**模型永不提供展示数值**。篇幅预算：标题+段落+证据表标题/行标签+图题+后果段结构化字段合计 <3000 非空白字符（兼容 text 不重复计数，附录/UI 不计）。

> schema3 时效口径：不再强制 facts 重复全局「数据截至」句（系统已生成分源时效）；模型若写了期值仍校验不得编造。schema2 的旧 facts 文字原样保留，读取层补分源提示并注明「各源最大期」口径。

## GET /history（旧研究只读列表）

按板块列出可验证归属的旧研究（redesign-enrichment-research-workbench 5.1）：`result_kind ∈ board_brief | board_investigation | legacy_board_analysis | topic_analysis`（topic scope 经所属话题归属校验，含已归档话题）；**绝不混入 signal_report**。读取零 LLM，增强开关关闭不阻止。

| 参数 | 必填 | 说明 |
|------|------|------|
| `kind` | 否 | 按上述四种过滤；非法 400 |
| `topic_id` | 否 | 按所属话题过滤；非法 400 |
| `before_id` | 否 | 结果 id 排他游标 |
| `limit` | 否 | 默认 20，上限 100 |

行：`{id, result_kind, analysis_scope, persistent_topic_id, semantic_board_id, created_at, session_id}`（不带 sectors 大载荷）。详情/QA/辩论/review 复用既有 GET；跨板块不泄露；单条坏载荷前端降级不吞列表。

## 计算字段（appendix.calculations[]）

| 字段 | 说明 |
|------|------|
| `calc_id` | 代码分配（k1、k2…），模型不可指定 |
| `op` | `difference` / `percent_change` / `mean`（白名单三算子，无脚本/URL/任意代码） |
| `inputs` | 观测引用数组（仅本次已取得且通过 cutoff 的原始观测；forward/计算结果作输入拒绝） |
| `expression` | 人类可读公式 |
| `value` | 规范十进制字符串（`big.Rat` 精确计算，half-up 最多 4 位、去末尾 0）；percent_change 的 `unit`=`%` |
| `unit` | 原生单位（不跨源/跨类换算；同系列/同单位/批准流量对 imports−exports 才允许） |
| `precision` | 固定 4 |
| `status` | `ok` / `missing`（任一输入 null——不填 0、带原因）/ `rejected`（混单位/跨源/库存减流量/base≤0 等） |
| `reason` | status ≠ ok 时的原因 |
| `computed_at` | 计算时间 |

> 真实执行计数以 `generation_meta` 为准（`source_calls`/`calculation_calls`），不信模型自报；研究进行中的 job 状态只有 `phase`，无实时计数。
