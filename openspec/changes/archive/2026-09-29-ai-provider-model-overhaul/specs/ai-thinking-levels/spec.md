## Purpose

AI 思考档七档体系：值域、四级解析优先级、按场景（capability 绑定）覆盖分配、按供应商方言映射为请求参数，以及档位可用性钳制，取代原 enable_thinking 布尔的二值语义。

## ADDED Requirements

### Requirement: 七档值域与全局默认

系统 SHALL 定义思考档值域为固定七档：`off / minimal / low / medium / high / xhigh / max`，全局默认 `medium`。值域 SHALL NOT 在未走跨域语义变更流程的情况下扩展或改名。

#### Scenario: 未配置任何档位时请求用全局默认

- **WHEN** 某次 chat 调用的模型未配默认档、capability 绑定未覆盖、请求未指定
- **THEN** 系统 SHALL 以 medium 作为该次调用的生效思考档

### Requirement: 四级解析优先级

单次调用的生效思考档 SHALL 按固定优先级解析：请求级（调用方显式指定）> capability 绑定级（路由绑定项携带的覆盖档）> 模型默认档（ai_models.thinking_level）> 全局默认（medium）。高优先级为空时 SHALL 逐级回落，SHALL NOT 跳级。

#### Scenario: 绑定覆盖压过模型默认

- **GIVEN** 模型 qwythos 默认档 medium，open_notebook 路由绑定项覆盖为 high
- **WHEN** open_notebook 流程发起调用
- **THEN** 生效思考档 SHALL 为 high（绑定级 > 模型默认）

#### Scenario: 绑定未覆盖时用模型默认

- **GIVEN** 模型 deepseek-flash 默认档 high，summary 路由绑定项未携带覆盖档
- **WHEN** summary 流程发起调用
- **THEN** 生效思考档 SHALL 为 high（模型默认）

### Requirement: 场景级思考档覆盖（capability 绑定项）

能力路由的每条模型绑定项 MAY 携带思考档覆盖值（空=继承模型默认）。覆盖值 SHALL 在绑定保存时校验为合法七档值；绑定保存与覆盖设置 SHALL 是同一管理界面的原子操作（改覆盖即改绑定）。embedding 类绑定 SHALL NOT 携带覆盖值。

#### Scenario: 绑定处按场景调档

- **WHEN** 用户在 topic_tagging 路由的绑定项上把思考档从「继承」改为「低」并保存
- **THEN** 该绑定项 SHALL 携带 low 覆盖，后续 topic_tagging 调用生效档为 low；其他能力绑定同一模型的调用不受影响

#### Scenario: 非法档位被拒绝

- **WHEN** 保存路由时某绑定项携带 thinking_level=ultra
- **THEN** 系统 SHALL 拒绝保存并返回明确错误

### Requirement: 方言到请求参数的映射

系统 SHALL 按 vendor 声明的思考方言（thinking_dialect）把生效档位翻译为对应请求参数：`qwen-chat-template` → `chat_template_kwargs.enable_thinking`（并按该方言支持区间钳制档位）；`deepseek` → DeepSeek `thinking` 参数；`reasoning-effort` → `reasoning_effort` 参数；`none` → SHALL NOT 发送任何思考控制参数。方言映射表 SHALL 在 vendor 创建时由预设或用户选择固化，运行时按 vendor 取用。

#### Scenario: qwen 方言映射与档位钳制

- **GIVEN** vendor 方言为 qwen-chat-template（方言支持区间 off..high），生效档 xhigh
- **WHEN** 构造 chat 请求
- **THEN** 请求 SHALL 以钳制后的 high 档映射 chat_template_kwargs.enable_thinking，SHALL NOT 发送 xhigh 原值

#### Scenario: none 方言不发思考参数

- **WHEN** 某本地服务 vendor 方言为 none，生效档 medium
- **THEN** 请求 payload SHALL NOT 包含任何思考控制字段

### Requirement: xhigh/max 可用性钳制

模型 MAY 声明支持的档位上限（默认最高 high）。未声明支持 xhigh/max 的模型，其绑定处的档位候选 SHALL NOT 暴露 xhigh/max，运行时解析结果 SHALL 钳制到其支持上限；声明支持的模型 SHALL 全档暴露。

#### Scenario: 未声明模型钳制到 high

- **GIVEN** 模型 qwythos 未声明支持 xhigh/max，绑定覆盖尝试 xhigh
- **WHEN** 保存绑定或发起调用
- **THEN** 系统 SHALL 以 high 生效（保存时钳制候选或运行时钳制取其一，最终生效不超过 high）

#### Scenario: 声明支持的模型全档可选

- **WHEN** 用户为声明支持 max 的云端模型选择覆盖档 max
- **THEN** 绑定处 SHALL 可选并保存 max，调用按 max 档映射方言参数

### Requirement: 输出清理跟随思考档

思考档 SHALL 同时控制响应内容的 `<think>` 标签清理：生效档为 off 时系统 SHALL 保留响应原始内容（含 `<think>` 标签，如有）；生效档为非 off 时 SHALL 从返回给调用方的内容中剥离 `<think>...</think>`。将思考内容放在独立字段（如 DeepSeek `reasoning_content`）的响应 SHALL 仅取 content 字段，清理行为不受影响。

#### Scenario: 非 off 档清理 think 标签

- **WHEN** 生效档为 high 的调用返回包含 `<think>...</think>` 的内容
- **THEN** 返回给调用方的内容 SHALL 不含该标签

#### Scenario: off 档保留原始内容

- **WHEN** 生效档为 off 的调用返回包含 `<think>...</think>` 的内容
- **THEN** 系统 SHALL 将原始内容（含标签）原样返回给调用方
