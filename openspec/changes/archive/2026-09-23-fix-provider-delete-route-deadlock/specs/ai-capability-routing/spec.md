## ADDED Requirements

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
