## Purpose

在全局 header 常驻展示标签队列的分析进度：复用现有 tag-queue status 计数与 WS 打标事件驱动，让用户在任何页面都能看到"分析进行到哪了"，空闲自动隐藏、失败变红可点。

## ADDED Requirements

### Requirement: 进度芯片常驻展示
系统 SHALL 在全局 header（铃铛左侧）展示标签队列进度芯片：内容为"分析中 n/total"（n=已完成计数，total=本轮总任务数）与迷你进度条，数据来源于 `GET /api/tag-queue/status` 计数与 WS `tag_completed`/`tag_failed` 事件驱动刷新。

#### Scenario: 批量打标进行中展示进度
- **WHEN** 队列存在 pending/leased 任务且用户在任何页面
- **THEN** 芯片可见并随事件实时推进（如"分析中 3/40"）

#### Scenario: WS 事件驱动计数刷新
- **WHEN** 收到 tag_completed 事件
- **THEN** 芯片计数即时更新，无需整页刷新或轮询等待

### Requirement: 空闲自动隐藏
队列既无活跃任务（pending+leased==0）也无 failed 任务时芯片 SHALL 不渲染（自动隐藏），不占用 header 空间；仅剩 failed 任务时 SHALL 保持可见（呈失败态，可点去处理）。

#### Scenario: 队列排空后芯片消失
- **WHEN** 最后一个任务 completed 且无新增入队且无 failed 任务
- **THEN** 芯片从 header 消失

#### Scenario: 活跃归零但仍有失败时保持可见
- **WHEN** 队列 pending+leased==0 但存在 failed 任务
- **THEN** 芯片保持可见并呈失败态（可点击跳队列区处理）

#### Scenario: 新任务入队芯片重现
- **WHEN** 队列空闲（芯片隐藏）状态下有新文章入队打标
- **THEN** 芯片重新出现并展示新计数

### Requirement: 失败态变红可点击
队列存在 failed 任务时芯片 SHALL 呈失败态（红色边框/底色，文案如"n 个失败"），点击行为与正常态一致。

#### Scenario: 失败态视觉区分
- **WHEN** 队列有 2 个 failed 任务且仍有活跃任务
- **THEN** 芯片呈红色失败态，进度条填充满

### Requirement: 点击跳转设置页队列区
点击芯片 SHALL 跳转设置页队列区（复用既有 TagQueuePanel），不新做面板。

#### Scenario: 点击芯片跳转
- **WHEN** 用户点击进行中的芯片
- **THEN** 路由跳转至 /settings 队列区，可见任务列表/重试/重打标操作

### Requirement: 队列计数展示语义
队列状态展示（芯片与 TagQueuePanel 状态区）SHALL 以活跃量（pending+leased）为主展示；completed 计数展示"今日完成"语义而非累计总量（配合队列行每日清理，累计口径不再有意义）。

#### Scenario: 面板计数语义
- **WHEN** 昨日有 1000 条 completed 且今日完成 20 条、活跃 3 条
- **THEN** 活跃量显示 3、"今日完成"显示今天日期内 completed 计数，不展示含历史的累计 total
