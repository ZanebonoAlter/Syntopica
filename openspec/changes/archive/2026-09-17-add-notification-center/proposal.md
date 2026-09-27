<!-- complexity: complex -->
<!-- ui-impact: major -->
<!-- constraint-domains: scheduler, content-enrichment, daily-report, ai-summary -->

## Why

当前的异步任务推送（标签完成/失败、Firecrawl 抓取、日报生成、自动刷新）全部经 WS `BroadcastRaw` 即时广播，**发完即丢、不落库**：页面没开着的事件（如凌晨定时日报生成完）永远看不到，页面开着但切走的事件也随组件卸载丢失。现有 `useNotify` toast 只有 3~5 秒瞬时提示，无历史、无未读。需要右上角铃铛通知中心：后端落库 + 推送，前端常驻接收，跨刷新可见。

## What Changes

- **标签队列进度芯片常驻展示**（2026-xx 对话新增需求，与通知中心同批落地）：header 复用现有 `GET /tag-queue/status` 计数 + WS `tag_completed`/`tag_failed` 事件驱动，展示紧凑进度芯片（如「分析中 3/40」+ 进度条）；队列空闲时自动隐藏，有失败任务时变红可点击；点击跳设置页队列区（复用现有 TagQueuePanel，不新做面板）。
- **新增通知持久化层**：后端新增 notifications 表（PostgreSQL），写入即触达——页面没开着时发生的任务也能在下次打开页面时看到（用户已确认"落库即覆盖"）。
- **新增通知事件接入点（白名单：只日报）**：在日报生成链路（`job_daily_report.go` / `daily_report_handler.go`）的**终态节点**写入通知并经 WS 广播统一 `notification` 事件——①完成（`daily_report_done` 处，含总数）②失败（当次任务存在失败版面时发**一条**汇总，不逐版面发）。抓取（Firecrawl）、自动刷新、单篇打标**不进通知**（高频过程类，逐篇通知=认知负担，2026-xx 对话裁决：白名单只日报）；打标队列排空汇总为候选需求（触发条件=队列排空即汇总，已记录），本期不实现；任务开始通知不做。
- **新增通知 API**：未读数查询、分页列表、标记已读（单条/全部），单用户无鉴权。
- **前端通知中心**：AppHeader 右上角新增铃铛按钮（未读角标）+ 点击展开下拉通知面板（复用 unified-dialog 浮层契约），列表内查看 + 标已读。
- **通知保留**：按条数上限（500 条），写入时淘汰最旧（用户已确认，不做定时清理任务）。
- **队列行保留策略（2026-xx 对话新增）**：扩展 log_cleanup 定时清理——三张队列表的 completed 行保留 1 天（每天重置，字面语义）、failed 行保留 30 天（保 TagQueuePanel 重试能力）；embedding_queues completed 保留期由 30 天收紧为 1 天。存量的 ~1.9w tag_jobs / ~7.2w embedding / ~2.6k firecrawl completed 行由清理任务上线后自动淘汰，无需手工 SQL；队列面板「队列长度」展示语义改为活跃量（pending+leased）。
- WS 事件总线与各业务任务的核心逻辑不变；toast（useNotify）行为不变。

## Capabilities

### New Capabilities
- `notification-center`: 异步任务通知的持久化、推送、查询与前端铃铛面板——覆盖哪些任务节点产生通知（白名单=日报生成终态：完成/失败）、通知数据结构与保留策略（条数上限淘汰）、API 契约（未读数/列表/标已读）、WS 事件格式、前端面板交互（角标/下拉/标已读）。
- `tag-queue-progress-chip`: 标签队列分析进度的 header 常驻展示——复用现有 tag-queue status API 与 WS 事件驱动的进度芯片（pending/processing 计数、空闲自动隐藏、失败态变红、点击跳设置页队列区），不新增后端接口。

### Modified Capabilities
- `log-cleanup`: 队列表行保留策略收紧——①既有「embedding_queues completed 保留 30 天」Requirement 修改为保留 1 天（30 天窗口对 ~2400 行/天的生成量形同虚设，实测 72,482 行堆积）；②新增 Requirement：log_cleanup 同时清理 tag_jobs / firecrawl_jobs 的 completed 行（保留 1 天）与 failed 行（保留 30 天，保面板重试能力）。前端队列面板的计数展示语义同步改为活跃量。

## Impact

- **后端**：`backend-go/internal/platform/ws/hub.go` 旁新增 notification 模块（model/repository/service/handler，含 GORM 迁移表）；日报链路终态节点（`topicgraph/handler/daily_report_handler.go` done/failed 广播处）增加通知写入调用；`internal/app/router.go` 注册通知 API。标签/抓取/自动刷新链路**不改动**。
- **前端**：`front/app/components/ui/` 或 `common/` 新增 NotificationBell + 通知面板组件与 TagQueueProgressChip 组件；`AppHeaderView.vue` header-right 插入铃铛与进度芯片；新增 `useNotifications` store（未读数轮询/WS 订阅/标已读）与 `useTagQueueProgress` composable（status 计数 + WS 事件驱动）；`utils/eventTypes.ts` 增加 `notification` 事件常量；`app/api/` 新增 notifications API client。
- **数据库**：新增 notifications 表迁移（单用户，无需用户外键）；条数上限淘汰在写入路径内完成，无新定时任务；队列行清理复用现有 log_cleanup 定时任务，存量 completed 行自动淘汰、无需手工 SQL。
- **既有行为**：任务进度类 WS 事件（progress）与页面内展示完全不变；notification 事件为新增事件类型，不与现有事件冲突。
- **部署后操作**：见 tasks 验证节——启动后自动建表迁移，无手工数据迁移；旧数据无降级问题（纯新增）。
