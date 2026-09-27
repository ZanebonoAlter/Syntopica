## Purpose

Refresh 操作并行化改造，包括后端 feed refresh 并发限流和前端页面加载两波并行。

## Requirements

### Requirement: 后端 refresh-all 并发执行
`refreshAllFeedsWorker` SHALL 改为 `sync.WaitGroup` + `chan struct{}(cap=3)` semaphore 并发调度。每个 feed 的错误 SHALL 独立捕获，不影响其他 feed 的执行。单个 feed 的刷新逻辑 SHALL 保持不变。

#### Scenario: 5 feeds 并发刷新
- **WHEN** refresh-all 触发，有 5 个 feed 需要刷新
- **THEN** 系统 SHALL 以 semaphore=3 限流并发执行，最多同时刷新 3 个 feed

#### Scenario: 单个 feed 刷新失败不影响其他
- **WHEN** feed #2 刷新过程中发生错误
- **THEN** 系统 SHALL 记录 feed #2 的错误，继续并发刷新 feed #1/#3/#4/#5

#### Scenario: semaphore 限流
- **WHEN** 已有 3 个 feed 正在刷新
- **THEN** 第 4 个 feed SHALL 等待某个刷新完成后才能开始

### Requirement: 前端页面加载两波并行
`FeedLayoutShell.vue` 的 `onMounted` SHALL 改为两波 `Promise.all`：
- **第一波**：`fetchFeeds()` + `loadWatchedTags()`（两者无依赖关系）
- **第二波**：`loadArticles()` + `fetchGlobalUnreadCount()`（可能依赖 feeds 列表）

#### Scenario: 首次加载页面
- **WHEN** 用户打开应用页面
- **THEN** 第一波 SHALL 同时发起 feeds 和 watched tags 请求，完成后第二波 SHALL 同时发起 articles 和 unread count 请求

#### Scenario: 第一波部分失败
- **WHEN** `fetchFeeds()` 成功但 `loadWatchedTags()` 失败
- **THEN** 第二波 SHALL 仍然执行（使用已获取的 feeds），watched tags 错误 SHALL 被独立捕获

### Requirement: 定时刷新后列表更新为按需重取

前端自动刷新（`useAutoRefresh` 触发的 feed 刷新）完成后 MUST 只重取当前视图所需的列表数据（当前筛选条件下的当前页）；MUST NOT 以 `per_page: 10000` 或任何等价的全量参数重拉文章列表，MUST NOT 用整表替换的方式丢失选中行与滚动位置。

#### Scenario: 单 feed 刷新完成

- **WHEN** 某 feed 的定时刷新完成
- **THEN** 前端 SHALL 以当前筛选 + 当前页参数重取列表，请求的 `per_page` SHALL ≤ 100
- **AND** 列表选中行与滚动位置 SHALL 保持不变

#### Scenario: 不发起全量重拉

- **WHEN** 任意刷新流程完成
- **THEN** SHALL NOT 出现 `per_page=10000`（或任何 > 100）的文章列表请求

### Requirement: 定时刷新单调度器错峰触发

多个 feed 的自动刷新 MUST 由单一调度器按各自 due 时间依次触发，同一时刻至多一个 feed 刷新在执行；MUST NOT 为每个 feed 建立同相位、同周期的独立定时器（避免同一分钟批量触发）。

#### Scenario: 多 feed 同周期

- **WHEN** 23 个 feed 的刷新间隔同为 60 分钟且上次刷新时刻相近
- **THEN** 刷新 SHALL 依次串行执行（同一分钟至多 1 个刷新），MUST NOT 出现 20 个以上刷新同分钟并发

#### Scenario: 刷新过程中用户切换视图

- **WHEN** 调度器正在刷新某个 feed，用户切换筛选或分页
- **THEN** 刷新 SHALL 继续执行完毕，切换后的列表请求 SHALL 不被刷新阻塞（各自独立发起）

#### Scenario: 刷新失败不终止调度

- **WHEN** 某个 feed 的刷新请求失败
- **THEN** 调度器 SHALL 记录该失败并继续按 due 时间调度后续 feed，MUST NOT 停止全部定时刷新
