# 话题数据增强 Data Enrichment

> 涵盖话题维度的生命周期上下文（lifeline_context）、增强结果（enrichment_result）、复盘评审（enrichment_review）、个股多空辩论（stock_debate），板块维度的数据源绑定（board_data_source）与简报/调查分析（board_brief / board_investigation），以及全局分析方法卡库（analysis_methods，旧 reference_roles 只读兼容）。
>
> 通用约定（响应信封、错误格式）见 [_conventions.md](_conventions.md)。

| 方法 | 路径 | 说明 |
| ------ | ------ | ------ |
| GET | `/api/persistent-topics/:topicId/enrichment/contexts` | 列出话题全部周期归档上下文（可按粒度过滤） |
| GET | `/api/persistent-topics/:topicId/enrichment/contexts/:granularity/:period` | 取指定粒度+周期的单条上下文 |
| PUT | `/api/persistent-topics/:topicId/enrichment/contexts/:granularity/:period` | 手动编辑某周期上下文内容 |
| POST | `/api/persistent-topics/:topicId/enrichment/contexts/:granularity/regenerate` | 重新生成某粒度上下文 |
| GET | `/api/persistent-topics/:topicId/enrichment/results` | 列出话题的增强结果（精简摘要） |
| GET | `/api/persistent-topics/:topicId/enrichment/results/:id` | 取单条增强结果（完整详情） |
| POST | `/api/persistent-topics/:topicId/enrichment/results/trigger` | 触发该话题的增强计算（异步，202 job 信封） |
| POST | `/api/persistent-topics/:topicId/enrichment/results/:id/debates` | 触发指定结果的多空辩论 |
| GET | `/api/persistent-topics/:topicId/enrichment/results/:id/debates` | 列出指定结果的辩论历史 |
| POST | `/api/persistent-topics/:topicId/enrichment/results/:id/qa` | 对指定结果发起一轮报告追问 |
| GET | `/api/persistent-topics/:topicId/enrichment/results/:id/qa` | 列出指定结果的多轮追问历史 |
| POST | `/api/persistent-topics/:topicId/enrichment/qa/:id/sediment` | 将某轮追问沉淀为持久笔记 |
| GET | `/api/persistent-topics/:topicId/enrichment/reviews` | 列出话题的复盘评审 |
| POST | `/api/persistent-topics/:topicId/enrichment/reviews` | 新建复盘评审 |
| PUT | `/api/persistent-topics/:topicId/enrichment/reviews/:id` | 更新评审的偏离摘要 |
| POST | `/api/persistent-topics/:topicId/enrichment/reviews/:id/apply` | 将评审标记为已应用 |
| GET | `/api/semantic-boards/:id/data-sources` | 列出板块绑定的数据源 |
| PUT | `/api/semantic-boards/:id/data-sources` | 新增/更新（upsert）板块数据源绑定 |
| DELETE | `/api/semantic-boards/:id/data-sources/:sourceType` | 删除指定类型的数据源绑定 |
| POST | `/api/semantic-boards/:id/enrichment/analysis/trigger` | 触发版块简报（202 job 信封） |
| POST | `/api/semantic-boards/:id/enrichment/analysis/investigations/trigger` | 对父简报某问题触发深入调查（202） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/results` | 版块档历史列表（可选 `?kind=` 过滤） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/results/:rid` | 单份版块档 result 详情 |
| POST | `/api/semantic-boards/:id/enrichment/analysis/results/:rid/qa` | 对板块档 result（三 kind 均可）发起追问 |
| GET | `/api/semantic-boards/:id/enrichment/analysis/results/:rid/qa` | 列出板块档 result 的追问历史 |
| POST | `/api/semantic-boards/:id/enrichment/analysis/results/:rid/qa/:qid/sediment` | 沉淀板块档某轮追问 |
| GET | `/api/enrichment/analysis-status` | 异步分析任务状态轮询（board + topic 两个 scope） |
| POST | `/api/semantic-boards/:id/enrichment/analysis/relations/discover` | 从简报观察/研究问题触发跨版块关系发现（202 job 信封） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/relations` | 跨版块关系列表（双侧匹配；`?status=` 逗号分隔过滤） |
| GET | `/api/semantic-boards/:id/enrichment/analysis/relations/:rid` | 关系详情（含 mapping_snapshot/gaps/run 审计） |
| POST | `/api/semantic-boards/:id/enrichment/analysis/relations/:rid/confirm` | 确认 proposed 关系（TTL 内生效） |
| POST | `/api/semantic-boards/:id/enrichment/analysis/relations/:rid/dismiss` | 驳回 proposed/unresolved 关系（reason 必填，进冷却） |
| POST | `/api/semantic-boards/:id/enrichment/analysis/relations/:rid/re-resolve` | 重解析 unresolved 关系目标 |
| GET / POST | `/api/analysis-methods` | 方法卡列表 / 创建 |
| GET / PUT / DELETE | `/api/analysis-methods/:id` | 方法卡详情 / 局部更新 / 软删除 |
| PUT | `/api/analysis-methods/:id/enable` | 方法卡启停 |
| GET | `/api/reference-roles`、`/api/reference-roles/:id` | 旧参考角色只读兼容（一版本） |
| POST / PUT / DELETE | `/api/reference-roles*` | 已退役，一律 410 |

> **路径参数命名**：本模块话题维度统一用 `:topicId`（驼峰，区别于其它模块常用的 `:topic_id` 下划线风格）；板块维度用 `:id`，与 board CRUD 一致。`:granularity` 取值固定为 `week` / `month` / `year` / `all`。

---

## 生命周期上下文 Contexts

### GET /api/persistent-topics/:topicId/enrichment/contexts

列出某话题的全部周期归档上下文。

**查询参数**

| 参数 | 必填 | 说明 |
|------|------|------|
| `granularity` | 否 | 按粒度过滤，取值 `week` / `month` / `year` / `all`；不传则返回全部粒度 |

**响应示例**

```json
{
  "success": true,
  "data": [
    {
      "id": 12,
      "persistent_topic_id": 7,
      "granularity": "week",
      "period": "2026-W27",
      "content": "本周话题演化摘要……",
      "source": "auto",
      "created_at": "2026-07-07T00:00:00Z",
      "updated_at": "2026-07-07T00:00:00Z"
    }
  ]
}
```

非法 `granularity` 返回 `400`。

### GET /api/persistent-topics/:topicId/enrichment/contexts/:granularity/:period

取指定粒度 + 周期的单条上下文。`period` 为必填路径段（如 `2026-W27`、`2026-07`、`2026`、`all`）。不存在时返回 `404`。

### PUT /api/persistent-topics/:topicId/enrichment/contexts/:granularity/:period

手动编辑某周期上下文的正文内容。更新后 `source` 会被置为 `manual`。

**请求体**

```json
{ "content": "手工修订后的上下文正文……" }
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `content` | 是 | 上下文正文，字符串 |

`content` 缺失返回 `400`；目标记录不存在返回 `404`。响应为更新后的上下文对象。

### POST /api/persistent-topics/:topicId/enrichment/contexts/:granularity/regenerate

重新生成某粒度的上下文。

**查询参数**

| 参数 | 必填 | 说明 |
|------|------|------|
| `period` | 否 | 指定周期（如 `2026-W27`）；不传则刷新该粒度当前周期（`RefreshGranularity`） |

响应为重新生成后的上下文对象。生成失败返回 `500`。

---

## 增强结果 Results

### GET /api/persistent-topics/:topicId/enrichment/results

列出该话题的增强结果（最新优先），返回精简摘要。

**响应示例**

```json
{
  "success": true,
  "data": [
    {
      "id": 101,
      "evolution_assessment": "话题持续升温，资金向龙头集中……",
      "sectors": [{ "sector": "原油", "symbols": [...] }],
      "tool_calls_count": 8,
      "session_id": "sess_abc123",
      "created_at": "2026-07-07T10:00:00Z"
    }
  ]
}
```

> `evolution_assessment` 超过 200 字符会被截断并补 `...`。
>
> **`sectors` 实际形状**（causal-analysis-agent 起，本字段名沿用但语义已改为分析产物）：`{form, lens, analysis}`，按 `form` 多态。`form ∈ event_chain|theme_vein|single_point|structural|sparse`（五形态）。`analysis` 为按形态的分层体（事实层 + 见解层）；**非 sparse 形态还含 `depth` 深度层**（`system_reframe`/`mechanism_layers`/`historical_analogy`/`regime_shift`(可空)/`boundary`(非空)/`evidence_chain`，`evidence_chain.source_type ∈ news|web|page`）；`sparse` 形态只有 `notice`+`summary`，无 depth。旧 result（无 depth）降级渲染不崩。完整 TS 镜像见 `front/app/api/boardEnrichment.ts` 的 `AnalyzeOutput` 联合。

### GET /api/persistent-topics/:topicId/enrichment/results/:id

取单条增强结果的完整详情。带 IDOR 校验：若结果不属于该 `topicId` 返回 `404`。

**响应示例**

```json
{
  "success": true,
  "data": {
    "id": 101,
    "persistent_topic_id": 7,
    "evolution_assessment": "完整评估正文……",
    "sectors": [{ "sector": "原油", "symbols": [...] }],
    "causal_chain": "因果链描述……",
    "tool_calls": [...],
    "input_snapshot": {...},
    "session_id": "sess_abc123",
    "created_at": "2026-07-07T10:00:00Z"
  }
}
```

### POST /api/persistent-topics/:topicId/enrichment/results/trigger

触发该话题的增强计算（cycle-B，含可选 `{prefill_lens}` 下钻预填，见文末）。触发前会校验所属板块是否启用增强（`enrichment_enabled`），未启用返回 `400`。**异步**：立即返回 `202` job 信封，分析在后台跑完落库；同 topic 已有任务在跑返回 `409`（`data` 携当前任务身份）。

**响应示例（202）**

```json
{
  "success": true,
  "data": {
    "status": "started",
    "job_id": "a1b2c3d4e5f6a7b8c9d4e5f6",
    "job_kind": "topic_analysis",
    "scope": "topic",
    "target_id": 101
  }
}
```

之后轮询 `GET /api/enrichment/analysis-status?job_id=`（见下）拿 `running/finished/error/result_id`。增强启动失败返回 `500`。

### POST /api/persistent-topics/:topicId/enrichment/results/:id/debates

对指定增强结果触发个股多空辩论。带 IDOR 校验。

**请求体**（可选）

```json
{
  "symbols": [
    { "code": "161129", "name": "易方达原油", "sector": "原油" }
  ]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `symbols` | 否 | 辩论标的列表；缺省或为空时自动从结果 `sectors.symbols` 提取 |

当 `symbols` 为空且结果内无可用标的时返回 `400`。

**响应示例**

```json
{
  "success": true,
  "data": { "debates": [ { /* StockDebateResult */ } ] }
}
```

### GET /api/persistent-topics/:topicId/enrichment/results/:id/debates

列出指定结果的历史辩论记录。

---

## 报告追问 Q&A

报告（`topic_enrichment_result`）生成后保持**不可变**（业务约束：result 不可变）。用户可对同一报告发起多轮追问：每轮复用增强 agent 的工具循环（`list_boards` / `list_lanes` / `get_lane_detail` / `web_search`），把报告快照植入系统提示，产出答案并 append 一行 `topic_enrichment_qa`（`source="qa"`）。报告本身从不被改写。

### POST /api/persistent-topics/:topicId/enrichment/results/:id/qa

对指定增强结果发起一轮报告追问。带 IDOR 校验（结果必须属于 `:topicId`）。

**请求体**

```json
{
  "question": "油价还会涨吗？"
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `question` | 是 | 追问问题，字符串 |

`question` 缺失返回 `400`。

**响应示例**

```json
{
  "success": true,
  "data": {
    "answer": "油价短期承压（推演有据）……",
    "tool_calls": [],
    "refs": [ { "source_type": "tool", "ref": "list_boards" } ]
  }
}
```

| 字段 | 说明 |
|------|------|
| `answer` | 追问答案文本 |
| `tool_calls` | 本轮工具调用记录 |
| `refs` | 引用来源（双类：新闻来自报告上下文 + 工具来自本轮） |

追问内部 LLM 调用走 Operation `data_enrichment.qa_tool_use`，session_id 为 `data_enrichment_qa_{resultID}_{hex8}`（每次询问唯一）。

### GET /api/persistent-topics/:topicId/enrichment/results/:id/qa

列出指定结果的多轮追问历史，按 `created_at` 升序（最旧优先）。

**响应示例**

```json
{
  "success": true,
  "data": [ { /* TopicEnrichmentQA */ } ]
}
```

### POST /api/persistent-topics/:topicId/enrichment/qa/:id/sediment

将某轮追问标记为已沉淀（`sedimented=true`）——用户手动 pin 为持久笔记。仅翻转 qa 行的 flag，**报告（result）永不重写**。带 IDOR 校验（经 qa → result → topic 间接校验归属）。

**响应示例**

```json
{
  "success": true,
  "data": { /* 更新后的 TopicEnrichmentQA，sedimented=true */ }
}
```

---

## 复盘评审 Reviews

### GET /api/persistent-topics/:topicId/enrichment/reviews

列出该话题的复盘评审（最新优先）。

### POST /api/persistent-topics/:topicId/enrichment/reviews

手动新建一条评审标注。新建的评审 `applied=true`、`source="manual"`。

**请求体**

```json
{
  "curr_result_id": 102,
  "deviation_summary": "本期与上期的偏离说明……",
  "prev_result_id": 101
}
```

| 字段 | 必填 | 说明 |
| ------ | ------ | ------ |
| `curr_result_id` | 是 | 当前结果 ID（uint） |
| `deviation_summary` | 是 | 偏离摘要，字符串 |
| `prev_result_id` | 否 | 上一期结果 ID（uint 指针，可空） |

`curr_result_id` / `deviation_summary` 缺失返回 `400`。响应为新建的评审对象。

### PUT /api/persistent-topics/:topicId/enrichment/reviews/:id

更新某条评审的偏离摘要。

**请求体**

```json
{ "deviation_summary": "修订后的偏离说明……" }
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `deviation_summary` | 是 | 偏离摘要，字符串 |

缺失返回 `400`。响应为更新后的评审对象。

### POST /api/persistent-topics/:topicId/enrichment/reviews/:id/apply

将评审标记为已应用（`applied=true`）。按设计不回写到上下文表（lifeline_context）。响应为更新后的评审对象。

---

## 板块数据源 Data Sources

### GET /api/semantic-boards/:id/data-sources

列出某板块绑定的全部数据源。

### PUT /api/semantic-boards/:id/data-sources

新增或更新（upsert）某板块的一个数据源绑定。`enabled` 缺省为 `true`。

**请求体**

```json
{
  "source_type": "custom_research",
  "config": { "note": "板块级参数" },
  "enabled": true
}
```

| 字段 | 必填 | 说明 |
| ------ | ------ | ------ |
| `source_type` | 是 | 数据源类型标识（受 `ValidateSourceType` 校验）。**内置金融源 `etf_quote`/`exchange_rate`/`gdelt_event` 已移除**，传入返回 `400`（unknown source_type）；枚举可扩展（`repository.RegisterSourceType` 运行时注册）。`web_search`/`fetch_page`/内部导航为 always-on，无需绑定 |
| `config` | 否 | 数据源配置，任意 JSON 对象 |
| `enabled` | 否 | 是否启用，布尔；不传按 `true` 处理 |

`source_type` 缺失返回 `400`。响应为写入后的数据源对象。

### DELETE /api/semantic-boards/:id/data-sources/:sourceType

删除指定类型的数据源绑定。目标不存在返回 `404`。

**响应示例**

```json
{ "success": true, "data": { "deleted": true } }
```


## 板块简报与问题调查 Board Analysis（board-level-deep-analysis）

> 默认触发产**版块简报**（`result_kind=board_brief`：单次 LLM，纯事实观察/关系/不确定项/可选题）；用户对简报中某问题显式触发**深入调查**（`result_kind=board_investigation`：方法卡→多假设含 H0→共享研究循环→五态综合）。存量旧论文式分析回填为 `legacy_board_analysis` 只读兼容。链路与约束见 [flow/data-enrichment.md](../flow/data-enrichment.md) §版块级简报与问题调查。

### POST /api/semantic-boards/:id/enrichment/analysis/trigger

触发版块简报。需板块开启 `enrichment_enabled`（未启用返回 `400`，含中文提示）；装配前自动跑 month/year 补全门（失败降级不阻塞）。**异步**：`202` job 信封，`job_kind=board_brief`；同板块任一 job（简报或调查）在跑时返回 `409`，`data` 携当前任务身份（前端据此恢复轮询）。

**响应示例（202）**

```json
{
  "success": true,
  "data": {
    "status": "started",
    "job_id": "a1b2c3d4e5f6a7b8c9d4e5f6",
    "job_kind": "board_brief",
    "scope": "board",
    "target_id": 9
  }
}
```

**响应示例（409，同板块在跑）**

```json
{
  "success": false,
  "error": "board analysis already running",
  "data": { "job_id": "0f1e2d3c4b5a69788796a5b4", "job_kind": "board_investigation", "scope": "board", "target_id": 9, "running": true, "started_at": "2026-08-31T09:00:00Z", "finished": false }
}
```

### POST /api/semantic-boards/:id/enrichment/analysis/investigations/trigger

对父简报某问题触发深入调查。**同步预检**（trim/枚举/父存在/同板块/kind=board_brief）失败返回 `400`/`404` 且 0 后台调用；合法则 `202`（`job_kind=board_investigation`，独立 `job_id`）后台调 `InvestigateBoardQuestion`。同板块互斥与 409 同上。

**请求体**

```json
{ "briefing_result_id": 205, "question_id": "q1" }
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `briefing_result_id` | 是 | 父简报 result ID（uint）；不存在/不属于本板块 → `404`；kind 非 board_brief → `400` |
| `question_id` | 否（二选一） | 父简报 `research_questions` 候选 id（`source=generated`，文本以父简报为准；解析不出文本 → `400`） |
| `question` | 否（二选一） | 用户自填问题（`source=custom`）；与 `question_id` 同时传时 `question_id` 优先 |

两者都缺 → `400`。`question_key` 由服务端按规范化问题文本（trim+空白折叠）SHA-256 计算，generated/custom 同算法；同父同题重跑允许（多行 append-only）。

### 跨版块关系发现（add-evidence-backed-cross-board-relations）

**POST /api/semantic-boards/:id/enrichment/analysis/relations/discover**

从父简报的观察/研究问题出发触发发现。同步预检：父简报存在且属本板块（否则 404）、kind=board_brief（否则 400）、`source_kind` ∈ `observation|question` 且 `source_key` 在父简报对应列表中（否则 400）——**source 文本永远不出现在客户端，服务端从父简报 sectors 重取**。合法 → `202`（`job_kind=relation_discovery`，`scope=relation`）；同 source 重复触发 → `409` 且 data 携当前任务身份（按其 job_id 恢复轮询）。

```json
// 请求
{ "briefing_result_id": 205, "source_kind": "observation", "source_key": "o1" }
// 202
{ "success": true, "data": { "status": "started", "job_id": "rel_...", "job_kind": "relation_discovery", "scope": "relation", "target_id": 9 } }
```

**关系生命周期五态**：`unresolved`（目标未解析/证据不足，可 re-resolve / dismiss）→ `proposed`（resolved+supported 待裁决，可 confirm / dismiss）→ `confirmed`（用户确认，TTL 内被简报消费；过期转 `expired`）；`dismissed`（驳回，同 hash 进 `dismiss_cooldown_days` 冷却）。confirm 仅 proposed 可调（否则 409，事务内重验目标版块存在）；dismiss 仅 proposed/unresolved 可调（reason 必填）；re-resolve 仅 unresolved 可调（否则 409）。列表按 status 过滤（非法值 400），匹配该版块的任一侧（source/target）；详情附 `mapping_snapshot`、`gaps`、`run`（发现轨迹审计：trigger_kind、budget_snapshot、error）。

**简报消费字段**：confirmed 未过期关系由服务端机械装配进新简报 `sectors.cross_board_relations[]`（`{relation_id, other_board_id, direction: outgoing|incoming, relation_type, claim, quality_grade, confirmed_at, expires_at, evidence_url, evidence_quote}`），LLM 不生成该字段、简报生成不联网；旧简报缺字段读取降级为空。

### GET /api/semantic-boards/:id/enrichment/analysis/results

版块档历史列表（scope=board，最新在前）。可选 `?kind=` 过滤：`board_brief` / `board_investigation` / `legacy_board_analysis`；缺省返回全部版块档；非法 kind → `400`。行结构同详情（`serializeBoardResult`）。

### GET /api/semantic-boards/:id/enrichment/analysis/results/:rid

单份版块档 result 详情。请求不属于该板块的 rid、或行 `analysis_scope` ≠ board（脏 scope 行）→ `404`，不泄漏存在性。

**响应示例**

```json
{
  "success": true,
  "data": {
    "id": 206,
    "analysis_scope": "board",
    "result_kind": "board_investigation",
    "parent_result_id": 205,
    "question_key": "6a1b2c3d4e5f...（64-hex SHA-256）",
    "sectors": { "scope": "board", "result_kind": "board_investigation", "question": { "id": "q1", "text": "...", "source": "generated" }, "hypotheses": [...], "conclusion": {...}, "evidence_chain": [...], "lane_refs": [...], "method_refs": [...] },
    "tool_calls": [...],
    "input_snapshot": {...},
    "session_id": "data_enrichment_board_9_ab12cd34",
    "created_at": "2026-08-31T09:12:00Z"
  }
}
```

> `sectors` 按 `result_kind` 多态：`board_brief` 载 `{summary, observations, relationships, uncertainties, research_questions, lane_refs, degraded?, retry_reason?}`；`board_investigation` 载 `{question, hypotheses[](五态 assessment+支持/反证/gaps), conclusion, evidence_chain, lane_refs, method_refs, retry_reason?}`；`legacy_board_analysis` 原样透传 v1 五字段 `{thesis, candidates, argument, depth, lane_refs}`。

### 板块档报告追问 QA（三 kind 均可）

`POST /api/semantic-boards/:id/enrichment/analysis/results/:rid/qa`（发起一轮，body `{question}`）、`GET .../results/:rid/qa`（多轮历史，最旧优先）、`POST .../results/:rid/qa/:qid/sediment`（沉淀仅翻 `sedimented` flag）——与话题档 QA 同机制（result 不可变、每轮 append 一行 `topic_enrichment_qa`）；简报/调查/legacy 三种 board kind 均可追问。所有权校验：result 必须属于路径板块且 `analysis_scope=board`，sediment 额外校验 qa 行属于该 rid；跨板块/跨 result/scope 不符/不存在统一 `404`。追问内部走 Operation `data_enrichment.qa_tool_use`。

### GET /api/enrichment/analysis-status

异步分析任务状态轮询（board + topic 两个 scope 共用，无前缀）。两种查询方式：

| 查询参数 | 行为 |
|------|------|
| `?job_id=` | 精确查一个 job（含已完成的）；未知 job_id → `404` |
| `?scope=board\|topic&id=` | 当前/最近任务（重进恢复）；无任务返回 idle 骨架 `{scope, target_id, running:false, finished:false}`（进程重启后同此） |

**响应示例（运行中）**

```json
{
  "success": true,
  "data": {
    "job_id": "a1b2c3d4e5f6a7b8c9d4e5f6",
    "job_kind": "board_brief",
    "scope": "board",
    "target_id": 9,
    "running": true,
    "started_at": "2026-08-31T09:00:00Z",
    "finished": false
  }
}
```

`job_kind` 枚举：`topic_analysis` / `board_brief` / `board_investigation`。完成后 `finished=true` 且带 `result_id`；失败带 `error`。单次后台上限 30 分钟；内存 job 表，进程重启即空闲态。

## 分析方法卡 Analysis Methods（方法库，board-level-deep-analysis）

> 全局方法卡库（`analysis_methods` 表）：声明适用/禁用/证据/失败模式边界，仅在调查链按问题选中 0-2 张注入（经清洗）；简报/事实阶段永不注入。创建默认 `disabled`；`legacy=true` 项为旧参考角色迁移（需人工整理后启用）。设置页「分析方法」section 即此 API。

### GET /api/analysis-methods

方法卡列表（当前返回完整卡，含 `content`；设置页无需再逐条请求详情）。**响应示例**

```json
{
  "success": true,
  "data": [
    { "id": 1, "name": "inside-america", "title": "内部看美国·方法论画像（v2）", "summary": "...", "selection_meta": { "applicable_when": [...], "avoid_when": [...], "required_evidence": [...], "failure_modes": [...] }, "content": "...", "enabled": false, "legacy": true, "created_at": "2026-08-28T00:00:00Z", "updated_at": "2026-08-28T00:00:00Z" }
  ]
}
```

### POST /api/analysis-methods

创建方法卡。body：`{ "name": "...", "title"?: "...", "summary"?: "...", "selection_meta"?: {...四数组}, "content": "...", "enabled"?: false }`——`name` + `content` 必填（缺 → `400`）；重名 → `409`；默认 `enabled=false`。

### GET /api/analysis-methods/:id

单条详情（含 `content` 正文）；不存在或已软删除 → `404`。

### PUT /api/analysis-methods/:id

局部更新（`name`/`title`/`summary`/`selection_meta`/`content`/`enabled` 任意子集）；传空 `name`/`content` → `400`；改名为已存在名 → `409`。

### PUT /api/analysis-methods/:id/enable

启停。body `{ "enabled": true }` 必填（缺 → `400`）；即时生效（后端每次调查现查 enabled 卡）。

### DELETE /api/analysis-methods/:id

软删除（`deleted_at`）；响应 `{ "deleted": <id> }`。历史调查的 `method_refs`（含 content_hash）仍可追溯。

## 参考角色 Reference Roles（旧，只读兼容一版本）

> 旧方法论画像库（`reference_roles` 表）已退役：所有 prompt 不再注入。GET 保留一版本供迁移查看；写 API 一律 `410`，指向 `/analysis-methods`（旧角色已由迁移按原文字节复制为 disabled legacy 方法卡）。设置页旧面板已下架。

### GET /api/reference-roles、GET /api/reference-roles/:id

旧角色列表/详情，只读；不存在 → `404`。

### POST / PUT / DELETE /api/reference-roles*

一律 `410 Gone`：`reference roles are read-only; use /analysis-methods`。

## 单泳道 trigger 扩展：prefill_lens

`POST /api/persistent-topics/:topicId/enrichment/results/trigger` 可选 body `{ "prefill_lens": "..." }`——版块简报观察/关系/研究问题/lane 引用与调查证据下钻时预填视角，后端 `EnrichTopicLens` 用它覆盖默认 lens 候选（candidates[0]）；空/缺省走原逻辑。预填写入可编辑输入框、允许修改、不自动触发。
