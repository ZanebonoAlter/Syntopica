# client-poll-budget Specification

## Purpose
前端常驻轮询的预算契约：把「调度器状态 / 标签队列计数 / 未读计数」这类常驻状态数据的刷新合并为单一对账入口，并给间隔、退避与页面可见性定硬边界，避免单页每秒级并发轮询白占带宽与连接。

## Requirements

### Requirement: 常驻状态数据由单一合并入口承载

前端对常驻状态类数据（调度器状态、标签队列计数、通知未读计数）的周期性对账 SHALL 通过单一批量端点完成一次请求；MUST NOT 为三类数据各自建立独立的周期性请求。服务端 SHALL 保留既有的分项端点（避免已打开的旧标签页请求 404）。

#### Scenario: 一次请求拿到三类计数

- **WHEN** 前端触发一次常驻对账
- **THEN** SHALL 只发起一个 HTTP 请求，其响应 SHALL 同时包含调度器状态、标签队列计数与未读计数

#### Scenario: 合并后旧端点仍可用

- **WHEN** 旧标签页（未更新 bundle）请求 `/api/schedulers/status`、`/api/tag-queue/status`、`/api/notifications/unread-count`
- **THEN** 三个端点 SHALL 各自继续返回既有结构的响应（HTTP 200）

### Requirement: 轮询间隔下限与退避

常驻对账间隔 SHALL 自适应且不短于下限：空闲态 ≥ 30 秒、有近期用户操作反馈时 ≥ 15 秒、仅当某项确实处于执行中（热态）时可短至 ≥ 5 秒；热态结束后 SHALL 回到空闲间隔。单标签页常驻对账频次 SHALL ≤ 4 次/分钟。

#### Scenario: 空闲态低频

- **WHEN** 无任务执行、无近期触发（超过 20 秒）
- **THEN** 相邻两次对账间隔 SHALL ≥ 30 秒

#### Scenario: 热态高频有界

- **WHEN** 某调度器处于执行中
- **THEN** 对账间隔 SHALL ≥ 5 秒，且执行结束后 SHALL 回到 ≥ 30 秒

#### Scenario: 单页频次上限

- **WHEN** 页面持续打开 1 分钟（任意状态组合）
- **THEN** 常驻对账请求数 SHALL ≤ 4

### Requirement: 页面隐藏时暂停轮询

常驻对账 SHALL 在页面不可见（`document.hidden`）时暂停，并在页面恢复可见时立即执行一次对账；MUST NOT 在后台标签页持续轮询。

#### Scenario: 后台标签页无请求

- **WHEN** 页面切到后台并保持 10 分钟
- **THEN** 期间 SHALL NOT 产生常驻对账请求

#### Scenario: 恢复可见立即对账

- **WHEN** 页面从后台切回前台
- **THEN** 前端 SHALL 立即发起一次对账请求（不等待下一个间隔）

### Requirement: 对账失败不清空既有数值

合并对账请求失败时，前端 SHALL 保留各类计数的上一次已知值并退避重试；MUST NOT 因单次失败把角标、芯片计数或状态指示清零或转为错误态。

#### Scenario: 单次失败保留旧值

- **WHEN** 合并对账请求返回 5xx 或网络错误
- **THEN** 未读角标与队列芯片 SHALL 继续显示上一次的值，且下一次对账 SHALL 按退避后的间隔进行
