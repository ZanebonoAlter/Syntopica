## MODIFIED Requirements

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

## ADDED Requirements

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
