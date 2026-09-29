## Purpose

AI 路由降级观测：基于 ai_call_logs 只读聚合的降级统计与事件流查询，供设置页观测面板回答「最近哪些调用发生了降级、为什么降级、落在哪个模型」。

## ADDED Requirements

### Requirement: 降级统计与事件流查询 API

系统 SHALL 提供 `GET /api/ai/observability/degradations?window=24h|7d&capability=<可选过滤>`：返回窗口内统计（调用总数、降级次数、失败次数）与降级事件流列表。数据源 SHALL 为既有 ai_call_logs（只读，不改其 schema），观测窗口 SHALL 受 ai_call_logs 保留期约束（默认 7 天清理，7d 窗口按现存日志计算）。

#### Scenario: 24h 窗口统计

- **WHEN** 调用 GET /api/ai/observability/degradations?window=24h
- **THEN** 返回 SHALL 含 { window, stats: { calls, degradations, failures }, events: [...] }，数值均来自窗口内 ai_call_logs 聚合

#### Scenario: 按 capability 过滤事件流

- **WHEN** 调用同一 API 带 capability=topic_tagging
- **THEN** events SHALL 只含该能力的事件，stats SHALL 保持全窗口口径

### Requirement: 降级事件字段与类型二分

每条降级事件 SHALL 携带：发生时间、capability、顺位迁移（from→to 序号，或全链失败标记）、生效模型名（降级后实际命中的模型；全链失败为最后尝试的模型）、事件类型与错误码。事件类型 SHALL 二分为 `structured`（结构化输出降级，见 structured-output-adaptation）与 `fallback`（供应商失败顺位迁移），SHALL NOT 混用单一字段。

#### Scenario: 结构化降级事件

- **WHEN** 某调用发生 json_schema→json_object 运行时降级
- **THEN** 事件 type SHALL 为 structured，code SHALL 记录降级方向（如 json_schema→json_object），model SHALL 为该次调用的模型

#### Scenario: 供应商失败顺位迁移事件

- **WHEN** 某能力主模型超时、顺位迁移到第二模型成功
- **THEN** 事件 type SHALL 为 fallback，from/to SHALL 为 0→1，model SHALL 为实际命中的第二模型，code SHALL 为底层错误（如 timeout）

#### Scenario: 全链失败事件

- **WHEN** 某能力全部绑定模型尝试失败
- **THEN** 事件 SHALL 携带全链失败标记（from/to 为空或 -1 语义），model 为最后尝试的模型

### Requirement: 观测面板界面契约

设置页运行状态（runtime-status）的 AI 健康 tab SHALL 在健康快照下方渲染降级观测块：统计行随 24h/7d 窗口切换刷新；事件流展示上述字段（时间/能力/顺位迁移/模型/类型标签/错误码）；每条事件提供「定位」操作跳转到能力路由分区对应能力行并高亮一次。观测块 SHALL NOT 提供任何写操作。

#### Scenario: 时间窗切换刷新统计

- **WHEN** 用户在观测块从 24h 切到 7d
- **THEN** 统计行与事件流 SHALL 按 7d 窗口重新加载

#### Scenario: 定位跳转能力路由

- **WHEN** 用户点击某条 fallback 事件（topic_tagging，顺位 0→1）的「定位」
- **THEN** 界面 SHALL 跳转到设置页能力路由分区，滚动到 topic_tagging 行并脉冲高亮一次

#### Scenario: 空窗口态

- **WHEN** 所选窗口内无任何降级事件
- **THEN** 观测块 SHALL 显示空态文案（如「窗口内无记录」），stats 计数照常返回
