## MODIFIED Requirements

### Requirement: Provider 模型类型区分（model_kind）

每个模型（ai_models）SHALL 声明其功能类型 `model_kind`，取值 `llm` 或 `embedding`，默认 `llm`。`model_kind` 表达模型功能（LLM 推理 / 向量嵌入），与表达协议的 vendor `provider_type`（openai_compatible / ollama）正交，互不替代。vendor 模型清单 API SHALL 返回每个模型的 `model_kind`；模型创建/更新 SHALL 接受并持久化 `model_kind`，缺省按 `llm`（约束细则见 ai-vendor-model「模型配置约束」）。

#### Scenario: 新建 provider 默认 llm

- **WHEN** 刷新或新建一个模型，未指定 model_kind
- **THEN** 系统 SHALL 保存 model_kind=llm

#### Scenario: 列表返回 model_kind

- **WHEN** 调用 vendor 模型清单 API（列表/刷新结果）
- **THEN** 每个模型对象 SHALL 包含 model_kind 字段（llm 或 embedding）

#### Scenario: 显式声明 embedding 类型

- **WHEN** 用户为本地 vendor 的模型选择 model_kind=embedding
- **THEN** 系统 SHALL 保存 model_kind=embedding

#### Scenario: model_kind 与 provider_type 正交

- **WHEN** 一个 provider_type=openai_compatible 的本地 vendor 下存在 model_kind=embedding 的模型（llama.cpp embedding 服务）
- **THEN** 系统 SHALL 接受该组合，两者互不冲突

### Requirement: 路由绑定的模型类型约束

能力路由的绑定对象 SHALL 为模型（ai_models），绑定关系由 `ai_route_model` 关联承载，并维护顺位顺序。系统 SHALL 在路由保存（UpsertRoute）时按 capability 类别校验绑定模型的 `model_kind`：`embedding` capability 路由 SHALL 只绑定 `model_kind=embedding` 的模型；其余（llm 类）capability 路由 SHALL 只绑定 `model_kind=llm` 的模型。违反时系统 SHALL 拒绝保存并返回明确错误（指明冲突的模型名与期望类型）。未启用（enabled=false）与 stale 的模型 SHALL NOT 出现在绑定候选中。

#### Scenario: embedding 路由拒绝 llm provider

- **WHEN** 保存 embedding capability 路由，绑定的某模型 model_kind=llm
- **THEN** 系统 SHALL 拒绝保存，返回错误（如「embedding 路由不能挂 llm 模型『X』」），路由绑定不变

#### Scenario: llm 路由拒绝 embedding provider

- **WHEN** 保存 summary capability 路由，绑定的某模型 model_kind=embedding
- **THEN** 系统 SHALL 拒绝保存并返回错误

#### Scenario: 同类绑定放行

- **WHEN** 保存 embedding 路由绑定 model_kind=embedding 的模型，或 summary 路由绑定 model_kind=llm 的模型
- **THEN** 系统 SHALL 接受保存

#### Scenario: 存量迁移后冲突告警

- **WHEN** 版本化迁移执行后，存在「同一模型同时绑定在 embedding 路由与 llm 路由」的存量数据
- **THEN** 迁移 SHALL 以日志告警列出冲突模型，且后续任何一次该路由的保存 SHALL 被类型校验拦截，直至用户手动修正

#### Scenario: stale 与未启用模型不可绑定

- **WHEN** 路由绑定候选列表加载，某模型处于 stale 或 enabled=false
- **THEN** 该模型 SHALL NOT 出现在候选中；直接提交绑定该模型的保存请求 SHALL 被拒绝

### Requirement: 被摘空的线路允许存在

线路因模型随 vendor 删除（级联解绑）或模型单独停用/摘除而不再持有任何绑定时，系统 SHALL 保留该线路及其配置。后续调用该能力的 LLM/嵌入时，系统 SHALL 走既有的「无可用模型」错误路径，SHALL NOT 因线路为空而 panic 或产生不一致状态。

#### Scenario: 线路被摘空后调用该能力

- **WHEN** 某能力的全部绑定均被删除（线路被摘空）后，业务流程调用该能力
- **THEN** 系统 SHALL 返回既有的无可用模型错误，错误信息可预期

## ADDED Requirements

### Requirement: 绑定项可携带思考档覆盖

`ai_route_model` 绑定项 MAY 携带 `thinking_level` 覆盖值（空=继承模型默认档），实现按场景分配思考等级；覆盖语义与解析优先级由 ai-thinking-levels 能力约束。绑定项的保存（增删、顺位调整、覆盖档设置）SHALL 在同一路由保存操作内原子完成。

#### Scenario: 顺位调整与覆盖档同时保存

- **WHEN** 用户在能力路由界面调整某能力绑定顺序并把首位的覆盖档设为 high 后点击保存
- **THEN** 系统 SHALL 在一次 UpsertRoute 中同时持久化新顺序与覆盖值

## REMOVED Requirements

### Requirement: Provider API Key 可空
**Reason**: key 的承载实体从 provider 上移到 vendor（两层拆解）。
**Migration**: 行为原样迁移至 ai-vendor-model「Vendor API Key 可空与显式清除」，验收以新 capability 为准。

### Requirement: 显式清除已保存的 API Key
**Reason**: 同上，key 管理（含 clear_api_key）归属 vendor 实体契约。
**Migration**: 行为原样迁移至 ai-vendor-model「Vendor API Key 可空与显式清除」。

### Requirement: enable_thinking 为输出清理标记（尽力而为）
**Reason**: 布尔二值思考语义被七档思考体系取代（enable_thinking 曾语义翻转导致全表重置的事故源）。
**Migration**: 存量按 true→high / false→off 迁入模型默认档（见 ai-vendor-model「存量配置拆解迁移」）；请求侧档位→参数映射与输出清理跟随语义见 ai-thinking-levels。

### Requirement: Provider 本地启动命令（start_command）
**Reason**: 进程拉起配置归属 vendor（同一 llama.cpp 进程可服务多个模型）。
**Migration**: 字段契约见 ai-vendor-model「Vendor 本地启动命令」；拉起/探测/冷却语义见 ai-model-health 修订后的同名能力。

### Requirement: Provider 删除级联解绑
**Reason**: 删除级联的对象变为 vendor（级联其全部模型与绑定）。
**Migration**: 级联行为与界面契约迁移至 ai-vendor-model「Vendor 删除级联与界面契约」。

### Requirement: 删除入口不被引用状态阻断
**Reason**: 同上，UI 契约随删除级联一并迁移。
**Migration**: 迁移至 ai-vendor-model「Vendor 删除级联与界面契约」（确认弹窗列明级联影响）。
