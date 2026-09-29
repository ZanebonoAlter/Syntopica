# structured-output-adaptation Specification

## Purpose
结构化输出能力适配：vendor 按档位声明结构化输出能力，请求按档构造 payload，运行时 400 降级链与解析纠错兜底，治好「写死 json_schema 导致 provider 连续 400 且兜底从未生效」的出血点。

## Requirements

### Requirement: vendor 结构化输出能力档位

每个 vendor SHALL 声明结构化输出档位 structured_output ∈ {`json_schema`, `json_object`, `none`}：`json_schema` = 支持 response_format json_schema（带 schema 校验）；`json_object` = 仅支持 response_format json_object（schema 需注入 prompt）；`none` = 不支持任何 response_format（纯文本 + schema 注入 prompt）。档位 SHALL 在 vendor 创建/更新时可配（预设携带默认值），运行时读 vendor 档位构造请求。

#### Scenario: DeepSeek 预设携带宽松档位

- **WHEN** 用户以 DeepSeek 预设创建 vendor
- **THEN** 该 vendor 的 structured_output SHALL 默认为 json_object（该厂商 json_schema 支持不完整的历史实测结论固化为预设默认）

#### Scenario: 档位可改

- **WHEN** 用户编辑 vendor 把结构化输出从 json_object 改为 json_schema
- **THEN** 系统 SHALL 持久化新档位，后续请求按 json_schema 构造

### Requirement: payload 按档位构造

需要结构化输出（调用方携带 JSON schema）的请求，系统 SHALL 按 vendor 档位构造：`json_schema` 档 SHALL 发送 response_format={type:json_schema, json_schema:{schema}}；`json_object` 档 SHALL 发送 response_format={type:json_object} 并把 schema 以指令文本注入 prompt；`none` 档 SHALL NOT 发送 response_format，schema 以指令文本注入 prompt。三档的注入文本 SHALL 明确要求「只输出符合给定结构的 JSON」。

#### Scenario: json_schema 档携带 schema

- **WHEN** 对 json_schema 档 vendor 发起携带 schema 的结构化调用
- **THEN** 请求 payload SHALL 包含 response_format.type=json_schema 与完整 schema 定义

#### Scenario: json_object 档 schema 入 prompt

- **WHEN** 对 json_object 档 vendor 发起同一调用
- **THEN** 请求 SHALL 包含 response_format.type=json_object，prompt SHALL 追加 schema 指令文本，payload SHALL NOT 携带 json_schema 结构

#### Scenario: none 档纯文本

- **WHEN** 对 none 档 vendor 发起同一调用
- **THEN** 请求 payload SHALL NOT 包含任何 response_format 字段，prompt SHALL 追加 schema 指令文本

### Requirement: 400 运行时降级链

结构化调用以 json_schema 档发出并收到 4xx 拒绝（如 provider 不识别 schema 格式）时，系统 SHALL 在同一次调用内降级重试：先降为 json_object + schema 注入 prompt 重试一次；再被 4xx 拒绝时降为纯文本（无 response_format）+ schema 注入重试一次；仍失败 SHALL 按该 provider 尝试失败处理（走 provider 列表顺序降级或聚合错误）。降级链 SHALL NOT 持久化修改 vendor 档位（运行时行为，档位变化只能由用户显式编辑）。每次降级 SHALL 产生降级事件（供 ai-route-observability 记录）。

#### Scenario: json_schema 400 后降级成功

- **WHEN** 某 vendor 档位为 json_schema 的调用返回 400（不识别 schema），降级为 json_object 重试成功
- **THEN** 该次调用 SHALL 以 json_object 响应内容作为成功结果，vendor 档位保持 json_schema 不变，并记录一条结构化降级事件

#### Scenario: 降级链穷尽后走 provider 降级

- **WHEN** 某调用三档全部尝试均失败（或非 4xx 的失败）
- **THEN** 系统 SHALL 按既有 provider 顺序降级到下一个绑定模型，全部失败以 errors.Join 聚合返回

#### Scenario: 降级不持久化档位

- **WHEN** 运行时降级链触发过 json_object 成功
- **THEN** 后续对同一 vendor 的新调用 SHALL 仍按其声明的 json_schema 档发起（首试档不变）

### Requirement: 结构化响应解析纠错

对携带 schema 的结构化调用，系统 SHALL 对响应内容做 JSON 解析；解析失败时 SHALL 执行一次纠错提取（截取首个平衡的 JSON 对象块再解析）；纠错后仍失败 SHALL 视为该次尝试失败（走降级链或 provider 降级），SHALL NOT 把非 JSON 原文当成功结果返回给调用方。

#### Scenario: 响应带包裹文本时纠错提取

- **WHEN** 模型返回「以下是结果：{...json...}」这类带包裹的文本
- **THEN** 系统 SHALL 提取花括号平衡块解析成功，调用以结构化结果成功

#### Scenario: 纠错失败按尝试失败处理

- **WHEN** 响应内容不含任何可提取的 JSON 结构
- **THEN** 该次尝试 SHALL 判为失败进入降级链，SHALL NOT 返回原文冒充结构化结果
