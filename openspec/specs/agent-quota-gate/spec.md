<!-- domain: harness -->

# agent-quota-gate Specification

## Purpose
TBD - created by archiving change agent-quota-gate. Update Purpose after archive.
## Requirements
### Requirement: 派发前拦截 Agent 工具调用

扩展 SHALL 注册 `tool_call` 事件钩子，且仅在 `event.toolName === "Agent"` 时介入额度检查；其他工具的调用 MUST NOT 产生任何网络请求或阻断行为。

#### Scenario: 非 Agent 工具直接放行

- **WHEN** 主线程发起 `bash`/`read`/`edit` 等非 Agent 工具调用
- **THEN** 钩子不发起额度查询、不阻断、不改写参数

#### Scenario: Agent 工具触发额度检查

- **WHEN** 主线程发起 `Agent` 工具调用（含 `run_in_background` 或前台）
- **THEN** 钩子解析目标 provider 并执行额度判定后才决定放行或阻断

### Requirement: 目标 provider 解析

扩展 SHALL 按以下优先级解析 Agent 调用的目标 provider：`model` 参数为 `provider/modelId` 全称时取该 provider；省略 `model` 时取当前会话默认 provider；`model` 为裸 modelId 时按 pi 的字母序规则解析并在结果中标注风险。API key 与 baseUrl MUST 通过 `ctx.modelRegistry.getProviderAuth()` 获取，MUST NOT 直接读取 `auth.json`。

#### Scenario: 全称 model 解析

- **WHEN** Agent 调用的 `model` 为 `zai-coding-cn/glm-5.2`
- **THEN** 目标 provider 为 `zai-coding-cn`，查询 GLM 额度端点

#### Scenario: 省略 model 走默认 provider

- **WHEN** Agent 调用未传 `model` 参数
- **THEN** 目标 provider 为会话默认 provider（`ctx.model` 的 provider）

#### Scenario: fuzzy 名标注风险

- **WHEN** Agent 调用的 `model` 为裸 `glm-5.2`（无 provider 前缀）
- **THEN** 按字母序解析到对应 provider，且当解析结果非默认供应商时在日志/reason 中提示该写法可能落到非预期供应商

### Requirement: 供应商额度查询适配器

扩展 SHALL 内置 zai-coding-cn、kimi-coding、deepseek 三个适配器，分别调用各自额度端点并归一化为统一状态（`ok`/`low`/`exhausted`/`unknown` + 中文摘要 + 重置时间提示）。无适配器的 provider（如 opencode-go）SHALL 放行，且每会话至多提示一次。

#### Scenario: GLM 双窗口判定

- **WHEN** GLM 端点返回 5h 窗口用量 95%、每周用量 40%（两条 TOKENS_LIMIT）
- **THEN** 状态为 `low`，摘要包含两个窗口的剩余百分比与 5h 窗口重置时间

#### Scenario: GLM 的 TIME_LIMIT 不参与判定

- **WHEN** GLM 端点返回一条 `TIME_LIMIT`（MCP 用量，percentage=100）+ 一条 `TOKENS_LIMIT`（5h 窗口，percentage=0）
- **THEN** 判定只基于 TOKENS_LIMIT，状态为 `ok`；MCP 用量即使 100% 也不影响派发

#### Scenario: DeepSeek 余额判定

- **WHEN** DeepSeek 端点返回 `is_available=false` 或总余额低于配置阈值
- **THEN** 状态为 `exhausted`

#### Scenario: opencode-go 无适配器放行

- **WHEN** Agent 调用目标 provider 为 `opencode-go`
- **THEN** 放行该调用，且同会话内不再重复提示该 provider 未接入

### Requirement: 额度不足时阻断派发

当判定状态为 `low` 或 `exhausted` 时，扩展 SHALL 返回 `{ block: true, reason }` 阻断该次 Agent 调用；reason MUST 为中文，包含：目标 provider、各窗口/余额剩余情况、最近的重置时间、建议动作（换模型或等待重置）。

#### Scenario: 阻断携带可操作 reason

- **WHEN** kimi-coding 周窗口剩余 5%（低于阈值）
- **THEN** Agent 调用被阻断，reason 说明 kimi-coding 额度不足、重置时间，并建议改用其他有额度的 provider 重试

#### Scenario: 额度充足放行

- **WHEN** 目标 provider 各窗口剩余均不低于阈值
- **THEN** 不阻断，Agent 调用正常执行

### Requirement: 缓存与失败降级

额度查询结果 SHALL 按 provider 内存缓存（默认 TTL 3 分钟，可经环境变量调整），同一 provider 的并发查询 MUST 去重（共享 in-flight 请求）。查询失败、超时（上限 5 秒）或返回非预期格式时 MUST 判定为 `unknown` 并放行，MUST NOT 因额度接口故障阻断派发。

#### Scenario: 缓存命中零网络开销

- **WHEN** 3 分钟内对同一 provider 连续派发两次子线程
- **THEN** 第二次判定直接使用缓存结果，不发起 HTTP 请求

#### Scenario: 额度接口超时 fail-open

- **WHEN** GLM 端点 5 秒内未响应
- **THEN** 该次判定为 `unknown`，Agent 调用放行

### Requirement: 阈值可配置

5h/周窗口百分比阈值（默认 10%）与 DeepSeek 余额阈值（默认 1 元）SHALL 集中在扩展顶部常量定义，支持环境变量覆盖。

#### Scenario: 环境变量覆盖阈值

- **WHEN** 设置环境变量将窗口阈值调整为 20%
- **THEN** 窗口剩余 15% 的 provider 被判定为 `low` 并阻断

