## MODIFIED Requirements

### Requirement: AICallLog 完整记录字段

`ai_call_logs` 表 SHALL 包含以下字段以满足 [`ai-logging.md`](../../../../../docs/reference/standard/backend/ai-logging.md) R2：`id`、`created_at`、`operation`（业务操作名）、`capability`、`route_name`、`provider_name`、`model`、`success`、`is_fallback`、`latency_ms`、`error_code`、`error_message`、`prompt`（完整 messages 文本）、`response_snippet`、`token_usage`（prompt/completion/total 的 jsonb）、`trace_id`、`session_id`（编排分组键）、`request_meta`。

`operation` SHALL NOT NULL。`prompt`、`token_usage`、`session_id` SHALL nullable（兼容历史数据与非编排的单次调用）。表 SHALL 在 `(session_id)` 上建索引、在 `(operation, created_at)` 上建复合索引。

建表与加列 SHALL 通过显式迁移完成（开发执行规范 §10），不依赖 gorm AutoMigrate。

chat 调用成功且 provider 响应携带 `usage` 块时，系统 SHALL 按 OpenAI 兼容接口标准键（`prompt_tokens`、`completion_tokens`、`total_tokens`）解析用量，并将 `ai_call_logs.token_usage` 落库为 `{"prompt":N,"completion":N,"total":N}` 的 jsonb。基础三键形状 SHALL 保持不变（消费方如 session 聚合按此形状读取）；当 usage 块携带思考用量细分（如 `completion_tokens_details.reasoning_tokens`）时，系统 SHALL 附加 `reasoning` 可选键落库，SHALL NOT 因此改变基础三键。provider 响应不携带 `usage` 块（无论成功或失败）时，`token_usage` SHALL 为 NULL，SHALL NOT 落库全零值。

**调用失败但 provider 响应携带 `usage` 块时**（如 HTTP 200 但 content 为空被归一化为 `empty_response`、结构化解析失败 `parse_error`），失败行 SHALL 落库该 `token_usage`——provider 侧已实际消耗的 token（含被思考烧掉的 reasoning token）不得因调用失败而从账目中消失；网络错误、HTTP 4xx/5xx 等无响应体的失败 SHALL 保持 NULL。

失败行的归因信息：当 provider 响应携带 `finish_reason`（如 `length`、`stop`）时，失败行 SHALL 将其记入 `response_snippet`（前缀标注），使"预算耗尽型空响应"（finish_reason=length）与"服务端真空响应"可区分；无响应体的失败 SHALL 不携带该前缀。

#### Scenario: 完整 prompt 落库

- **WHEN** airouter 执行一次 `Chat` 调用成功
- **THEN** 写入的 `ai_call_logs.prompt` SHALL 包含本次 system+user messages 的完整文本（超 20000 runes 时截断并标注 `[truncated]`），不得为空字符串或摘要

#### Scenario: token 用量记录

- **WHEN** chat 调用成功且 provider 响应包含 `usage` 块（键为 `prompt_tokens`/`completion_tokens`/`total_tokens`）
- **THEN** `ai_call_logs.token_usage` SHALL 为 `{"prompt":N,"completion":N,"total":N}` 且 `total` 大于 0，与响应 `usage` 块数值一致

#### Scenario: 失败但响应携带 usage 块落库

- **WHEN** chat 调用返回 HTTP 200 且响应包含 `usage` 块（`completion_tokens` = `reasoning_tokens` > 0），但 content 为空被归一化为 `empty_response` 失败
- **THEN** 失败行 `ai_call_logs.token_usage` SHALL 落库该用量（含 `reasoning` 可选键），`success=false`，账目可据此核算空思考烧掉的 token

#### Scenario: 失败行携带 finish_reason

- **WHEN** chat 调用返回 HTTP 200、`choices[0].finish_reason="length"` 且 content 为空
- **THEN** 失败行的 `response_snippet` SHALL 含 `finish_reason=length` 标注

#### Scenario: 无 usage 块不落全零

- **WHEN** chat 调用成功但 provider 响应不包含 `usage` 块，或调用因网络错误/HTTP 4xx/5xx 失败（无响应体 usage）
- **THEN** `ai_call_logs.token_usage` SHALL 为 NULL，SHALL NOT 写入 `{"prompt":0,"completion":0,"total":0}`

#### Scenario: 历史行回填

- **WHEN** 迁移执行后，历史已存在的 `ai_call_logs` 行
- **THEN** 其 `operation` SHALL 为 `unknown`（回填），`prompt`/`token_usage`/`session_id` SHALL 为 NULL，不报错

#### Scenario: operation 列约束

- **WHEN** 尝试插入 `operation=NULL` 或空字符串
- **THEN** 系统 SHALL 因 NOT NULL 约束拒绝（回填完成后）
