# ai-vendor-model Specification

## Purpose
AI 供应商与模型两层注册表：vendor 承载协议/认证/端点/进程拉起配置，model 承载模型身份/类别/思考默认档/启用状态与清单刷新，取代原 ai_providers 单层实体。

## Requirements

### Requirement: Vendor 与 Model 两层实体

系统 SHALL 以两层实体管理 AI 接入：`ai_vendors`（供应商）承载 provider_type（openai_compatible / ollama）、base_url、api_key、timeout、思考方言（thinking_dialect）、结构化输出档位（structured_output）与本地启动命令（start_command）；`ai_models`（模型）挂属于唯一 vendor，承载模型名、model_kind（llm / embedding）、默认思考档（thinking_level，可空=用全局默认）、启用状态（enabled）与清单时间戳（last_seen_at）。系统 SHALL NOT 再以 `ai_providers` 单层实体承接新写入（存量迁移后该表停写废弃）。

#### Scenario: vendor 承载协议与端点

- **WHEN** 用户创建一个 llama.cpp 供应商（provider_type=openai_compatible、base_url=http://10.11.12.111:8080/v1）
- **THEN** 系统 SHALL 保存为一条 ai_vendors 记录，协议/端点/超时/方言/结构化输出档位均落在 vendor 上

#### Scenario: model 挂属 vendor 并声明类别

- **WHEN** 用户在该 vendor 下启用模型 qwythos（model_kind=llm、默认思考档=medium）
- **THEN** 系统 SHALL 保存为一条 ai_models 记录，携带 vendor 外键、类别与默认档

### Requirement: Vendor API Key 可空与显式清除

`openai_compatible` 类型的 vendor SHALL 允许空 API Key 配置并保存（支持 llama.cpp / vLLM 等无认证本地服务），SHALL NOT 强制要求非空；`ollama` 类型本就无需 key。client 层 SHALL 在 API Key 为空时省略 `Authorization` 请求头。系统 SHALL 以 `clear_api_key` 布尔字段支持显式清除已存 key：`clear_api_key=true` 时置空；缺省或为假且请求 `api_key` 为空时沿用已存值。

#### Scenario: 无 key vendor 创建成功

- **WHEN** 用户创建 provider_type=openai_compatible、api_key 为空的 vendor
- **THEN** 系统 SHALL 成功保存，SHALL NOT 返回「api_key is required」错误

#### Scenario: 无 key vendor 调用不带认证头

- **WHEN** 路由对某 api_key 为空的 vendor 发起 chat 请求
- **THEN** 发往该 vendor BaseURL 的 HTTP 请求 SHALL NOT 包含 `Authorization` 头

#### Scenario: 清除已存 key

- **WHEN** 更新已存 vendor，携带 clear_api_key=true
- **THEN** 系统 SHALL 将该 vendor 的 APIKey 置空；未携带清除标记且 api_key 为空时 SHALL 沿用原值

### Requirement: Vendor 本地启动命令（start_command）

vendor MAY 声明可选的 `start_command`（文本，可空）：有值 SHALL 表达「该 vendor 是需被托管的本地进程」（如 llama-server 启动行）；为空 SHALL 表达「外部已托管服务，不尝试拉起」。vendor 列表 API SHALL 返回 start_command 是否已配置（原文或 configured 布尔）；创建/更新 SHALL 接受并持久化该字段。拉起/探测/冷却语义由 ai-model-health 能力约束。

#### Scenario: 配置本地启动命令

- **WHEN** 用户为 llama.cpp vendor 填写 start_command=`llama-server -m qwythos.gguf --port 8080`
- **THEN** 系统 SHALL 保存该命令，该 vendor 被视为可托管本地进程

#### Scenario: 云端供应商留空

- **WHEN** 用户创建 DeepSeek 云端 vendor，不填 start_command
- **THEN** 系统 SHALL 保存空值，该 vendor 不被视为本地进程

### Requirement: 模型清单接口刷新与 stale 标记

系统 SHALL 提供按 vendor 触发的模型清单刷新：调用该 vendor 的 `GET {base_url}/models`，将返回的模型清单与库内该 vendor 的模型对账——新出现的模型 SHALL 新增入库（标记 last_seen_at，默认未启用）；已存在且再次可见的 SHALL 更新 last_seen_at；库内存在但清单未返回的 SHALL 标记 stale，SHALL NOT 静默删除。刷新失败（网络/超时/非 2xx）SHALL 保留旧清单不变并返回错误。

#### Scenario: 刷新发现新模型

- **WHEN** 用户对 DeepSeek vendor 点击刷新，上游 /models 返回了库内没有的 deepseek-chat
- **THEN** 系统 SHALL 新增该模型（默认未启用），界面清单出现带「新」标记的条目

#### Scenario: 上游消失的模型标 stale 不删除

- **WHEN** 刷新后上游清单不再包含已启用的 deepseek-reasoner
- **THEN** 系统 SHALL 将其标记 stale，SHALL NOT 删除该记录及其路由绑定；stale 模型 SHALL NOT 出现在绑定候选中

#### Scenario: 刷新失败保留旧清单

- **WHEN** 刷新请求超时或返回非 2xx
- **THEN** 系统 SHALL 返回明确错误，库内该 vendor 的模型清单（含 stale 标记）SHALL 保持刷新前状态

### Requirement: 模型配置约束

每个 model SHALL 声明 model_kind ∈ {llm, embedding}，与所属 vendor 的 provider_type 正交。仅本地服务类 vendor MAY 配置 embedding 模型（云端 vendor 的模型 SHALL 为 llm）。llm 模型 MAY 配置默认思考档（七档值之一）；embedding 模型 SHALL NOT 携带思考档。vendor 创建时按预设清单拉入的模型 SHALL 默认未启用，用户显式勾选后生效。

#### Scenario: 云端 vendor 不能配 embedding 模型

- **WHEN** 用户试图为云端 DeepSeek vendor 添加 model_kind=embedding 的模型
- **THEN** 系统 SHALL 拒绝并返回明确错误

#### Scenario: 新拉取模型默认未启用

- **WHEN** 新增 vendor 保存后自动拉取模型清单成功
- **THEN** 拉入的模型 SHALL 全部处于未启用状态，直至用户勾选启用

### Requirement: Vendor 删除级联与界面契约

系统 SHALL 允许删除被能力路由绑定的 vendor：删除 SHALL 在同一事务内解除其全部模型的路由绑定（ai_route_model 关联）、删除其全部模型、再删除 vendor 本身，SHALL NOT 因仍被路由引用而拒绝；删除响应 SHALL 告知解除的绑定数与删除的模型数。管理界面 SHALL NOT 因引用状态禁用删除入口，确认弹窗 SHALL 列明级联影响（N 模型 / M 绑定）。

#### Scenario: 删除被路由引用的 vendor

- **WHEN** 用户删除一个模型被 7 条能力路由绑定的 vendor
- **THEN** 系统 SHALL 事务内解除全部绑定、删除其模型与 vendor 本身，响应告知「删除 N 模型、解除 M 绑定」

#### Scenario: 删除确认弹窗列明级联

- **WHEN** 用户点击某 vendor 的删除按钮
- **THEN** 系统 SHALL 弹出统一确认弹窗，正文列明将一并删除的模型数与解除的绑定数，确认后才执行

### Requirement: 存量配置拆解迁移

系统 SHALL 以版本化迁移将存量 `ai_providers` 拆解为 vendor+model 两层：每行 provider 生成一条 vendor（携带协议/端点/key/超时/启动命令）与其单条 model（名称=provider.model，model_kind 与启用状态沿用）；`ai_route_providers` 绑定 SHALL 改挂对应 model_id。历史死字段（`ai_providers.metadata`、`ai_routes.strategy`、`ai_routes.priority`）SHALL NOT 迁移。存量 `enable_thinking` 布尔 SHALL 按 true→high / false→off 写入该模型的默认思考档。迁移 SHALL 幂等（重复执行不产生重复 vendor/model）。

#### Scenario: 三行存量拆解为两层

- **WHEN** 迁移执行时 ai_providers 存在 qwen（:8080）、qwen3-embedding（:8081）、deepseek-flash（云端）三行
- **THEN** 迁移后 SHALL 存在 3 条 vendor 与 3 条 model，原 10 条 ai_route_providers 绑定全部改挂到对应 model_id 且顺序保持

#### Scenario: 死字段不迁移

- **WHEN** 迁移完成
- **THEN** 新层表 SHALL NOT 存在 metadata / strategy / priority 对应字段或数据

#### Scenario: enable_thinking 布尔映射默认档

- **WHEN** 迁移时某 provider enable_thinking=true
- **THEN** 其拆出的模型默认思考档 SHALL 为 high；false 的 SHALL 为 off
