## ADDED Requirements

### Requirement: 无调度器的运行模式返回空集合而非错误

在调度器未注册/未启动的运行模式（如只读 demo、`DEMO_READ_ONLY=1`）下，调度器状态类端点（至少 `/api/schedulers/status`、`/api/tasks/status`）SHALL 返回 HTTP 200 与结构合法的空集合（`data: []` 或等价空结构），MUST NOT 以 5xx 表达「无调度器」。前端常驻轮询这些端点时 MUST NOT 收到 5xx。

#### Scenario: 只读模式返回 200 空集合

- **WHEN** 在只读 demo 模式（调度器未注册）下请求 `/api/schedulers/status`
- **THEN** 响应 SHALL 为 HTTP 200，且响应体 SHALL 为可被前端解析的空调度器列表

#### Scenario: 任务队列状态同样可用

- **WHEN** 在只读 demo 模式（任务账本未初始化）下请求 `/api/tasks/status`
- **THEN** 响应 SHALL 为 HTTP 200，且响应体 SHALL 表示「无活跃任务、队列为空」

#### Scenario: 读模式不影响生产模式语义

- **WHEN** 在调度器正常注册的运行模式下请求同一端点
- **THEN** 响应 SHALL 仍为既有结构（含真实调度器列表与 `analysis_paused`/`ai_healthy` 等顶层字段）
