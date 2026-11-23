## ADDED Requirements

### Requirement: 版块级关键事件批量读

系统 SHALL 提供按版块批量读取确认关键事件的端点 `GET /api/semantic-boards/:id/key-events`，供版块态势页一次取全观察窗内的事件，MUST NOT 要求前端逐话题请求。

该端点 SHALL 接受窗口参数 `days`（正整数，必填或具明确默认值），返回该版块在窗口内**全部 `state='confirmed'`** 的注记，每条至少包含：事件日期（`source_period_date`）、所属话题 ID 与话题标签快照、标题快照、摘要快照、注记 ID。响应 SHALL 允许前端按「话题 + 日期」聚合。

端点 MUST NOT 返回 `suggested` 或 `dismissed` 注记。窗口外事件 MUST NOT 返回。无匹配事件时 SHALL 返回成功与空数组，MUST NOT 返回错误。非法的 `id` 或 `days` SHALL 返回 4xx 并说明参数问题。

本端点 SHALL 只读，MUST NOT 创建、确认或修改任何注记。既有「按单个话题读」端点与注记写端点的语义 MUST NOT 因本端点而改变。

#### Scenario: 一次取全窗口事件

- **WHEN** 前端以 `days=14` 请求某版块的关键事件
- **THEN** 系统 SHALL 一次返回该版块 14 天窗口内全部 confirmed 注记，前端无需再按话题逐个请求

#### Scenario: 建议态不出现

- **WHEN** 某话题在窗口内同时存在 confirmed 与 suggested 注记
- **THEN** 响应 SHALL 只包含 confirmed 注记

#### Scenario: 窗口外事件不返回

- **WHEN** 某 confirmed 注记的 `source_period_date` 早于窗口起点
- **THEN** 响应 MUST NOT 包含该注记

#### Scenario: 空结果

- **WHEN** 窗口内没有任何 confirmed 注记
- **THEN** 端点 SHALL 返回 2xx 与空数组，MUST NOT 返回错误，MUST NOT 用建议态补足

#### Scenario: 非法参数

- **WHEN** `days` 为非数字、负数或 0
- **THEN** 端点 SHALL 返回 4xx 并指明 `days` 不合法
