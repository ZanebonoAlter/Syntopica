## Purpose

AI 能力路由系统：为每个 AI capability（summary、digest_polish、topic_tagging、embedding 等）维护独立的路由配置、provider 绑定与并发配额，使各业务流程通过其绑定的 capability 加载路由与 provider，互不干扰。

## Requirements

### Requirement: 能力与业务用途绑定

系统 SHALL 为每个 AI capability 维护与业务用途的唯一绑定：`summary` SHALL 驱动文章自动总结；`digest_polish` SHALL 驱动日报生成；`topic_tagging` SHALL 驱动事件标签提取与标签相关的语义操作；`embedding` SHALL 驱动向量嵌入；`open_notebook` SHALL 驱动日报页边注问答（margin notes QA，通用概念 + 当天文章上下文 RAG 作答与术语抽取）。每个业务流程 SHALL 仅通过其绑定的 capability 加载路由与 provider。

#### Scenario: 文章总结使用 summary 路由

- **WHEN** 文章自动总结流程（`summarizeContent`）调用 LLM
- **THEN** 系统 SHALL 通过 `summary` capability 加载路由与 provider

#### Scenario: 日报生成使用 digest_polish 路由

- **WHEN** 日报生成流程的任一 LLM 调用（聚类、要闻、叙事）执行
- **THEN** 系统 SHALL 通过 `digest_polish` capability 加载路由与 provider

#### Scenario: 页边注问答使用 open_notebook 路由

- **WHEN** 日报页边注批注的问答流程（回答生成与术语抽取）调用 LLM
- **THEN** 系统 SHALL 通过 `open_notebook` capability 加载路由与 provider，并按该 capability 的并发配额限流

### Requirement: 默认并发配额独立
系统 SHALL 为每个 capability 提供独立的默认并发上限信号量，可被路由级 `MaxConcurrency` 覆盖。不同 capability 的并发配额 SHALL 互不挤占。

#### Scenario: digest_polish 与 topic_tagging 并发隔离
- **WHEN** 日报生成与标签提取同时进行
- **THEN** `digest_polish` 与 `topic_tagging` SHALL 使用各自独立的信号量，一方占满配额时 SHALL NOT 阻塞另一方

### Requirement: 废弃的 article_completion
系统 SHALL NOT 定义 `article_completion` capability 常量，SHALL NOT 在任何 LLM 调用中使用 `article_completion` 作为 capability。

#### Scenario: 无 article_completion 调用
- **WHEN** 系统发起任何 LLM 调用
- **THEN** 该调用的 capability SHALL NOT 为 `article_completion`

### Requirement: Provider API Key 可空
系统 SHALL 允许 `openai_compatible` 类型的 provider 以空 API Key 配置并保存，以支持 llama.cpp / vLLM / LM Studio 等无认证的本地 OpenAI 兼容服务。系统 SHALL NOT 对 `openai_compatible` 类型强制要求非空 API Key；仅 `ollama` 类型本就无需 key。client 层 SHALL 在 API Key 为空时省略 `Authorization` 请求头。

#### Scenario: 无 key 的 openai_compatible provider 可创建
- **WHEN** 用户创建一个 `provider_type=openai_compatible`、`api_key` 为空的 provider
- **THEN** 系统 SHALL 成功保存该 provider，且 SHALL NOT 返回「api_key is required」错误

#### Scenario: 无 key provider 被 chat 调用时不发送认证头
- **WHEN** 路由对某 `api_key` 为空的 `openai_compatible` provider 发起 chat 请求
- **THEN** 发往该 provider BaseURL 的 HTTP 请求 SHALL NOT 包含 `Authorization` 头

#### Scenario: 前端不再强制校验 openai_compatible 的 key
- **WHEN** 用户在主模型或备用模型表单中保存一个 `provider_type=openai_compatible` 且 API Key 为空的配置
- **THEN** 前端 SHALL NOT 以「需要填写 API Key」阻断保存

### Requirement: 显式清除已保存的 API Key
系统 SHALL 提供显式清除已保存 API Key 的能力，通过 `clear_api_key` 布尔字段表达，消除「空串=沿用」语义下无法清空已存 key 的二义。当 `clear_api_key=true` 时，系统 SHALL 将该 provider 的 `APIKey` 置空；当 `clear_api_key` 为假或缺省且请求中的 `api_key` 为空时，系统 SHALL 沿用已保存的 key 不变。

#### Scenario: 清除已存 key
- **WHEN** 更新一个已存 provider，请求携带 `clear_api_key=true`
- **THEN** 系统 SHALL 将该 provider 的 `APIKey` 设为空字符串

#### Scenario: 不清除时沿用已存 key
- **WHEN** 更新一个已存 provider，请求未携带 `clear_api_key`（或为假）且 `api_key` 为空
- **THEN** 系统 SHALL 保持该 provider 原有的 `APIKey` 不变

#### Scenario: 前端提供清除密钥入口
- **WHEN** 用户编辑一个 `api_key_configured=true` 的备用 provider
- **THEN** 前端 SHALL 显示「清除密钥」操作入口；触发后 SHALL 以 `clear_api_key=true` 提交更新

### Requirement: enable_thinking 为输出清理标记（尽力而为）
`enable_thinking` 字段 SHALL 仅控制响应内容的 `<think>` 标签清理行为，SHALL NOT 声称在请求侧控制模型是否思考。`enable_thinking=true` 时系统 SHALL 对 chat 响应内容执行 `stripThinkTags`；`enable_thinking=false` 时 SHALL 跳过该清理。系统 SHALL NOT 向 provider 发送任何声称「关闭思考」的请求参数（包括但不限于 `reasoning_effort:"none"`）。

#### Scenario: 开启 thinking 时清理 think 标签
- **WHEN** 某 `enable_thinking=true` 的 provider 返回包含 `<think>...</think>` 的内容
- **THEN** 系统 SHALL 从返回给调用方的 `ChatResult.Content` 中剥离该标签

#### Scenario: 关闭 thinking 时保留原始内容
- **WHEN** 某 `enable_thinking=false` 的 provider 返回包含 `<think>...</think>` 的内容
- **THEN** 系统 SHALL 将原始内容（含标签）原样返回给调用方

#### Scenario: 不发送关闭思考参数
- **WHEN** 对任意 provider 发起 chat 请求
- **THEN** 请求 payload SHALL NOT 包含 `reasoning_effort` 字段（无论 `enable_thinking` 取值）

#### Scenario: DeepSeek 的独立 reasoning 字段无害
- **WHEN** provider 将思考内容放在独立的 `reasoning_content` 字段（如 DeepSeek）而非内嵌 `content`
- **THEN** 系统 SHALL 仅取 `content` 字段作为结果，`reasoning_content` SHALL 被忽略且不影响 `enable_thinking` 的清理行为

### Requirement: Provider 模型类型区分（model_kind）

每个 `AIProvider` SHALL 声明其模型功能类型 `model_kind`，取值 `llm` 或 `embedding`，默认 `llm`。`model_kind` 表达模型功能（LLM 推理 / 向量嵌入），与表达协议的 `provider_type`（openai_compatible / ollama）正交，互不替代。provider 列表 API（`GET /api/ai/providers`）SHALL 返回每个 provider 的 `model_kind`；provider 创建/更新（`POST /api/ai/providers`、`PUT /api/ai/providers/:id`）SHALL 接受并持久化 `model_kind`，缺省按 `llm`。

#### Scenario: 新建 provider 默认 llm

- **WHEN** 用户创建一个 provider，未传 model_kind
- **THEN** 系统 SHALL 保存 model_kind=llm

#### Scenario: 显式声明 embedding 类型

- **WHEN** 用户创建 provider，传 model_kind=embedding
- **THEN** 系统 SHALL 保存 model_kind=embedding

#### Scenario: 列表返回 model_kind

- **WHEN** 调用 GET /api/ai/providers
- **THEN** 每个 provider 对象 SHALL 包含 model_kind 字段（llm 或 embedding）

#### Scenario: model_kind 与 provider_type 正交

- **WHEN** 一个 provider_type=openai_compatible、model_kind=embedding 的本地 llama.cpp embedding 服务
- **THEN** 系统 SHALL 接受该组合，两者互不冲突

### Requirement: 路由绑定的模型类型约束

系统 SHALL 在路由绑定 provider 时按 capability 类别校验 provider 的 `model_kind`：`embedding` capability 路由 SHALL 只绑定 `model_kind=embedding` 的 provider；其余（llm 类）capability 路由 SHALL 只绑定 `model_kind=llm` 的 provider。违反时系统 SHALL 拒绝保存并返回明确错误（指明冲突的 provider 名与期望类型）。embedding 任务 SHALL 且仅 SHALL 使用 embedding 类型的模型。

#### Scenario: embedding 路由拒绝 llm provider

- **WHEN** 保存 embedding capability 路由，绑定的某 provider model_kind=llm
- **THEN** 系统 SHALL 拒绝保存，返回错误（如「embedding 路由不能挂 llm 模型『X』」），路由绑定不变

#### Scenario: llm 路由拒绝 embedding provider

- **WHEN** 保存 summary capability 路由，绑定的某 provider model_kind=embedding
- **THEN** 系统 SHALL 拒绝保存，返回错误

#### Scenario: 同类绑定放行

- **WHEN** 保存 embedding 路由绑定 model_kind=embedding 的 provider，或 summary 路由绑定 model_kind=llm 的 provider
- **THEN** 系统 SHALL 接受保存

#### Scenario: 存量迁移后冲突告警

- **WHEN** 版本化迁移执行后，存在「同一 provider 同时绑定在 embedding 路由与 llm 路由」的存量数据
- **THEN** 迁移 SHALL 以日志告警列出冲突 provider，且后续任何一次该路由的保存 SHALL 被类型校验拦截，直至用户手动修正

### Requirement: Provider 本地启动命令（start_command）

每个 `AIProvider` MAY 声明一个可选的本地启动命令 `start_command`（文本，可空）。`start_command` 有值 SHALL 表达「该 provider 是需被托管的本地进程」（如 llama.cpp 的 `llama-server` 启动行）；为空 SHALL 表达「该 provider 是外部已托管服务，不 attempts 拉起」。provider 列表 API SHALL 返回 `start_command` 是否已配置（不回显原文以避免无谓泄露时，至少返回 `start_command_configured` 布尔）；provider 创建/更新 SHALL 接受并持久化 `start_command`。

#### Scenario: 配置本地启动命令

- **WHEN** 用户为一个 llama.cpp provider 填写 start_command=`llama-server -m D:/models/qwen.gguf --port 8081`
- **THEN** 系统 SHALL 保存该命令，标记该 provider 为可托管本地进程

#### Scenario: 外部服务留空

- **WHEN** 用户为一个云端 OpenAI 兼容 provider 不填 start_command
- **THEN** 系统 SHALL 保存空值，该 provider 不被视为本地进程

#### Scenario: 列表回显配置态

- **WHEN** 调用 GET /api/ai/providers
- **THEN** 每个 provider 对象 SHALL 至少包含表示 start_command 是否已配置的字段（start_command 原文或 start_command_configured 布尔）

### Requirement: Provider 删除级联解绑

系统 SHALL 允许删除被能力线路引用的 provider：删除请求 SHALL 在同一事务内先解除该 provider 的全部线路关联（`ai_route_provider` 关联记录），再删除 provider 本身。系统 SHALL NOT 因 provider 仍被线路引用而拒绝删除请求。删除响应 SHALL 告知用户解除的线路关联数量。

#### Scenario: 删除挂在线路上的 provider

- **WHEN** 用户删除一个被一条或多条能力线路引用的 provider
- **THEN** 系统 SHALL 删除该 provider 及其在所有线路上的关联记录，返回成功响应并说明解除的关联数量

#### Scenario: 删除未被引用的 provider

- **WHEN** 用户删除一个没有任何线路引用的 provider
- **THEN** 系统 SHALL 直接删除该 provider，行为与级联解绑路径一致

### Requirement: 删除入口不被引用状态阻断

管理界面 SHALL NOT 因 provider 仍被线路引用而禁用或阻断其删除入口；当被删 provider 挂在线路上时，界面 SHALL 在删除确认前告知「将从所有线路解绑」的后果。

#### Scenario: 挂线路 provider 的删除入口与告知

- **WHEN** 用户查看一个被线路引用的 provider 的删除操作入口
- **THEN** 删除按钮 SHALL 可点击，且行内 SHALL 展示告知性文案（删除将自动从所有线路解绑）

#### Scenario: 删除确认弹窗

- **WHEN** 用户点击删除按钮
- **THEN** 系统 SHALL 弹出统一确认弹窗（非原生 confirm），确认后执行删除，取消则不发起任何请求

### Requirement: 被摘空的线路允许存在

线路因 provider 删除（级联解绑）而不再持有任何 provider 时，系统 SHALL 保留该线路及其配置。后续调用该能力的 LLM/嵌入时，系统 SHALL 走既有的「无可用 provider」错误路径，SHALL NOT 因线路为空而 panic 或产生不一致状态。

#### Scenario: 线路被摘空后调用该能力

- **WHEN** 某能力的全部 provider 均被删除（线路被摘空）后，业务流程调用该能力
- **THEN** 系统 SHALL 返回既有的无可用 provider 错误，错误信息可预期
