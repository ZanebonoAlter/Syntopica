# ai-logging Specification

## Purpose
TBD - created by archiving change ai-call-logging-schema. Update Purpose after archive.

## Requirements

### Requirement: AICallLog 完整记录字段

`ai_call_logs` 表 SHALL 包含以下字段以满足 [`ai-logging.md`](../../../../../docs/reference/standard/backend/ai-logging.md) R2：`id`、`created_at`、`operation`（业务操作名）、`capability`、`route_name`、`provider_name`、`model`、`success`、`is_fallback`、`latency_ms`、`error_code`、`error_message`、`prompt`（完整 messages 文本）、`response_snippet`、`token_usage`（prompt/completion/total 的 jsonb）、`trace_id`、`session_id`（编排分组键）、`request_meta`。

`operation` SHALL NOT NULL。`prompt`、`token_usage`、`session_id` SHALL nullable（兼容历史数据与非编排的单次调用）。表 SHALL 在 `(session_id)` 上建索引、在 `(operation, created_at)` 上建复合索引。

建表与加列 SHALL 通过显式迁移完成（开发执行规范 §10），不依赖 gorm AutoMigrate。

chat 调用成功且 provider 响应携带 `usage` 块时，系统 SHALL 按 OpenAI 兼容接口标准键（`prompt_tokens`、`completion_tokens`、`total_tokens`）解析用量，并将 `ai_call_logs.token_usage` 落库为 `{"prompt":N,"completion":N,"total":N}` 的 jsonb。存储 jsonb 形状 SHALL 保持不变（消费方如 session 聚合按此形状读取）。provider 响应不携带 `usage` 块或调用失败时，`token_usage` SHALL 为 NULL，SHALL NOT 落库全零值。

#### Scenario: 完整 prompt 落库

- **WHEN** airouter 执行一次 `Chat` 调用成功
- **THEN** 写入的 `ai_call_logs.prompt` SHALL 包含本次 system+user messages 的完整文本（超 20000 runes 时截断并标注 `[truncated]`），不得为空字符串或摘要

#### Scenario: token 用量记录

- **WHEN** chat 调用成功且 provider 响应包含 `usage` 块（键为 `prompt_tokens`/`completion_tokens`/`total_tokens`）
- **THEN** `ai_call_logs.token_usage` SHALL 为 `{"prompt":N,"completion":N,"total":N}` 且 `total` 大于 0，与响应 `usage` 块数值一致

#### Scenario: 无 usage 块不落全零

- **WHEN** chat 调用成功但 provider 响应不包含 `usage` 块，或调用失败
- **THEN** `ai_call_logs.token_usage` SHALL 为 NULL，SHALL NOT 写入 `{"prompt":0,"completion":0,"total":0}`

#### Scenario: 历史行回填

- **WHEN** 迁移执行后，历史已存在的 `ai_call_logs` 行
- **THEN** 其 `operation` SHALL 为 `unknown`（回填），`prompt`/`token_usage`/`session_id` SHALL 为 NULL，不报错

#### Scenario: operation 列约束

- **WHEN** 尝试插入 `operation=NULL` 或空字符串
- **THEN** 系统 SHALL 因 NOT NULL 约束拒绝（回填完成后）

### Requirement: airouter 强制 Operation 传参

`Router.Chat` / `Router.Embed` 的入参 SHALL 包含 `Operation` 字段。`Operation` 为空时，`Router.Chat` SHALL 返回错误、不执行调用、不入库日志。这是把"必记 operation"从开发者自觉提升为强制约束。

`SessionID` 字段 SHALL 可选（非编排的单次调用允许为空）；编排类调用（日报管线、agent loop）的多次调用 SHALL 共享同一个 SessionID。

#### Scenario: 漏填 Operation 被拒

- **WHEN** 调用 `Router.Chat` 时 `req.Operation` 为空
- **THEN** SHALL 返回 error，不发起 provider 调用，不写 AICallLog

#### Scenario: 编排 session 串联

- **WHEN** 一次日报生成（含 cluster_tags/highlights/threads/merge/embedding 多次调用）
- **THEN** 这些调用写入的 `ai_call_logs.session_id` SHALL 全部相同（由编排入口生成并通过 context 传递）

### Requirement: 现有 AI 调用补齐 Operation 与 SessionID

日报管线与 topic_watch 的所有 airouter 调用 SHALL 传语义化的 `Operation` 值：`daily_report.cluster_tags`、`daily_report.highlights`、`daily_report.threads`、`daily_report.merge_arbitration`、`section.embedding`、`topic_watch.evaluate`。

日报编排入口 SHALL 生成统一 SessionID（格式 `daily_report_{board_id}_{uuid8}`；用 board_id 因 report.ID 在 SaveReport 后才填充，而 SessionID 须在 LLM 调用前注入）并通过 context 传给当期所有调用。

#### Scenario: 日报调用可按编排重建

- **WHEN** 查询 `GET /api/ai/call-logs?session_id=daily_report_42_<uuid>`
- **THEN** SHALL 返回该日报生成期间的全部 LLM 调用（cluster_tags + highlights + threads + merge + embedding），按 created_at 正序

### Requirement: AI 调用日志查询 API

系统 SHALL 暴露 `GET /api/ai/call-logs`，支持查询参数 `operation`、`session_id`、`capability`、`from`、`to`、`limit`（默认 50，上限 200）、`offset`。响应 SHALL 为 `gin.H{"success":true,"data":[...]}`，每条记录包含 id/operation/session_id/capability/provider_name/model/success/latency_ms/token_usage/prompt（截断预览）/response_snippet/created_at。

#### Scenario: 按 session 回放编排

- **WHEN** `GET /api/ai/call-logs?session_id=<某编排id>`
- **THEN** SHALL 返回该 session 下全部调用，按 created_at 正序，用于回放一次编排的全过程

#### Scenario: 分页上限

- **WHEN** `GET /api/ai/call-logs?limit=500`
- **THEN** SHALL 被截断为 200 条（上限），不报错

### Requirement: embedding 调用 token 用量记录

airouter 的 embedding 调用（`Router.Embed`）SHALL 与 chat 调用同等记录 token 用量：provider 响应携带 `usage` 块且调用成功时，`ai_call_logs.token_usage` SHALL 为 `{"prompt":N,"completion":N,"total":N}` 的 jsonb（按 OpenAI 兼容标准键解析）；响应无 `usage` 块或调用失败时 SHALL 为 NULL。命中本地 embedding 缓存（`ai_embedding_cache`）直接返回、未发起 provider 调用时不产生用量，`token_usage` SHALL 为 NULL。

#### Scenario: embedding 成功调用记录用量

- **WHEN** `Router.Embed` 调用 provider 成功且响应包含 `usage` 块
- **THEN** 写入的 `ai_call_logs.token_usage` SHALL 为非零的 `{"prompt":N,"completion":N,"total":N}`，与响应 `usage` 块数值一致

#### Scenario: embedding 缓存命中不记用量

- **WHEN** embedding 请求命中本地缓存（未发起 provider 调用）
- **THEN** 对应 `ai_call_logs` 行的 `token_usage` SHALL 为 NULL

#### Scenario: session 聚合涵盖 embedding 用量

- **WHEN** 按同一 `session_id` 聚合（如 `GET /api/ai/sessions/:session_id`）
- **THEN** `summary.total_tokens` SHALL 同时累加该 session 下 chat 与 embedding 调用的用量
