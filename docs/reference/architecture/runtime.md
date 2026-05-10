# 后端运行时与接口

## 先看启动主线

当前 Go 后端真实启动顺序在 `backend-go/cmd/server/main.go`，顺序如下：

1. `config.LoadConfig("./configs")` 读取配置
2. `database.InitDB(config.AppConfig)` 初始化 PostgreSQL、建表和索引
3. `digest.Migrate()` 执行 digest 相关迁移
4. `airouter.EnsureLegacySummaryConfigMigrated()` 把旧摘要配置迁到 AI route 体系
5. `tracing.InitTracerProvider(database.DB, traceCfg)` 初始化 OpenTelemetry tracing
6. 根据配置切换 Gin `debug/release` 模式
7. 创建 `gin.Engine`，挂载 CORS 与 Recovery、otelgin 中间件
8. `app.SetupRoutes(r)` 注册 HTTP 与 WebSocket 路由
9. `app.StartRuntime()` 启动后台 scheduler 与内容补全服务
10. `app.SetupGracefulShutdown(runtime)` 注册优雅退出
11. `r.Run(:port)` 开始监听

所以 `cmd/server` 现在只是薄入口，真正的运行时装配已经集中在 `internal/app/`。

## Runtime 里实际启动了什么

`backend-go/internal/app/runtime.go` 里定义的 `Runtime` 目前会启动 10 类后台任务：

- `AutoRefresh`：扫描到点 feed 并触发刷新
- `PreferenceUpdate`：更新阅读偏好
- `ContentCompletion`：基于 Firecrawl 正文生成文章级摘要
- `Firecrawl`：抓取文章完整正文
- `BlockedArticleRecovery`：恢复因 Firecrawl 配置变更等原因阻塞的文章
- `AutoTagMerge`：基于 embedding 相似度自动合并相似标签
- `TagQualityScore`：重算 `topic_tags.quality_score`
- `NarrativeSummary`：基于活跃主题标签生成每日叙事摘要

此外还会启动以下异步队列 worker：

- `topicextraction.GetTagQueue().Start()`：文章标签异步打标队列
- `topicanalysis.StartEmbeddingQueueWorker()`：标签 embedding 向量化队列
- `topicanalysis.StartMergeReembeddingQueueWorker()`：合并后 re-embedding 队列

对应启动逻辑也都在 `StartRuntime()` 里，不存在额外的隐藏入口。

## 运行时共享状态怎么暴露

当前 runtime 不是通过依赖注入容器对外暴露，而是把运行中的 scheduler 引用写进 `backend-go/internal/app/runtimeinfo/schedulers.go`。

现在真正挂进去的是这些：

- `AutoRefreshSchedulerInterface`
- `PreferenceUpdateSchedulerInterface`
- `AISummarySchedulerInterface`
- `FirecrawlSchedulerInterface`
- `AutoTagMergeSchedulerInterface`
- `TagQualityScoreSchedulerInterface`
- `NarrativeSummarySchedulerInterface`

这里仍有一个命名差异要讲清楚：

- `AISummarySchedulerInterface` 这个名字对应的是 `ContentCompletionScheduler`，也就是文章级内容补全任务，不是 feed 级 `auto_summary`

## 启动参数与默认值

`StartRuntime()` 里目前写死了几组默认间隔：

- `auto_refresh`：60 秒检查一次
- `preference_update`：1800 秒检查一次
- `content_completion`：60 秒检查一次
- `firecrawl`：轮询
- `blocked_article_recovery`：3600 秒检查一次
- `tag_quality_score`：3600 秒检查一次
- `narrative_summary`：86400 秒检查一次（每天一次）

同时内容补全服务会先读取 `CRAWL_SERVICE_URL`：

- 有环境变量就用环境变量
- 没有就回落到 `http://localhost:11235`

## 当前路由面

`backend-go/internal/app/router.go` 目前把后端接口分成这些入口。

### 基础入口

- `GET /`：API 概览
- `GET /health`：健康检查
- `GET /api/tasks/status`：聚合 summary queue、content completion、firecrawl 的实时任务概览
- `GET /ws`：WebSocket 连接入口

### 核心业务 API

- `/api/categories`：分类 CRUD
- `/api/feeds`：订阅 CRUD、单 feed 刷新、批量刷新、feed 预览抓取
- `/api/articles`：文章列表、详情、统计、单条/批量状态更新
- `/api/reading-behavior`：阅读行为上报与统计
- `/api/user-preferences`：偏好查询与手动更新

### AI 与内容处理 API

- `/api/ai/summarize`：单篇摘要
- `/api/ai/settings`：旧摘要设置读写兼容入口
- `/api/ai/providers`：AI provider 管理
- `/api/ai/routes`：AI capability route 管理
- `/api/content-completion`：文章级内容补全触发、状态与总览
- `/api/firecrawl`：单文章抓取、feed Firecrawl 开关、状态、配置保存

### 主题与 digest API

- `/api/topic-graph`：图谱、topic 详情、按分类聚合、topic 相关文章、相关 digest、pending 文章
- `/api/topic-graph/analysis`：topic analysis 查询、状态、重建
- `/api/embedding`：embedding 配置与队列管理
- `/api/topic-tags`：关注标签、标签合并预览、抽象标签管理
- `/api/narratives`：叙事摘要列表、详情、历史

### Scheduler API

统一入口在 `/api/schedulers`：

- `GET /api/schedulers/status`
- `GET /api/schedulers/:name/status`
- `POST /api/schedulers/:name/trigger`
- `POST /api/schedulers/:name/reset`
- `PUT /api/schedulers/:name/interval`

这组 API 现在统一覆盖：

- `auto_refresh`
- `preference_update`
- `content_completion`
- `firecrawl`
- `tag_quality_score`
- `narrative_summary`
- `tag_hierarchy_cleanup`

另外保留一个兼容别名：

- `ai_summary` -> `content_completion`

但能力不是完全对称的：

- `auto_refresh`、`preference_update`、`content_completion`、`firecrawl`、`tag_quality_score`、`narrative_summary`、`tag_hierarchy_cleanup` 支持统一状态查询
- `auto_refresh`、`preference_update`、`content_completion`、`firecrawl`、`tag_quality_score`、`narrative_summary`、`tag_hierarchy_cleanup` 支持统一 trigger
- `auto_refresh`、`preference_update`、`content_completion`、`firecrawl`、`tag_quality_score`、`narrative_summary`、`tag_hierarchy_cleanup` 支持 `reset` / `interval`

## Scheduler 状态现在能看到什么

当前 scheduler 状态主要来自两层：

### 进程内状态

每个 scheduler 自己维护：

- 是否已启动
- 是否正在执行
- 下次运行时间
- 当前处理对象 / 最近处理对象（部分任务）

### 数据库存档状态

`auto_refresh`、`content_completion`、`tag_quality_score`、`narrative_summary`、`tag_hierarchy_cleanup` 会把最近一轮执行结果写进 `scheduler_tasks`，包含：

- `last_execution_time`
- `next_execution_time`
- `last_execution_duration`
- `last_error`
- `total_executions`
- `successful_executions`
- `failed_executions`
- `last_execution_result`

其中 `last_execution_result` 不是统一 schema，而是各 scheduler 自己的摘要 JSON。

## 几条关键运行时链路

### 链路 1：服务启动到定时任务进入运行态

1. 进程启动后完成配置和数据库初始化
2. `SetupRoutes` 先把所有 HTTP / WS 接口挂到 Gin
3. `StartRuntime` 再启动 scheduler
4. 每个 scheduler 在 `Start()` 阶段会初始化或修复自己的 `scheduler_tasks` 记录
5. 前端随后就可以查 `/api/schedulers/status`，digest 兼容状态仍可从 `/api/digest/status` 读取

这个顺序意味着：即使某个 scheduler 启动失败，HTTP API 仍然会起来，只是状态接口会暴露失败结果或缺失的 runtime 引用。

### 链路 2：手动触发自动刷新

1. 前端请求 `POST /api/schedulers/auto_refresh/trigger`
2. `jobs.TriggerScheduler` 从 `runtimeinfo` 取到 `AutoRefreshScheduler`
3. 如果实现了 `TriggerNow()`，就直接返回是否接受、是否真的触发、拒绝原因
4. `AutoRefreshScheduler` 执行扫描，结果写回 `scheduler_tasks.last_execution_result`
5. 前端再查 `/api/schedulers/auto_refresh/status` 就能看到最近一轮扫描摘要

### 链路 3：内容补全状态更新

## 这次补上的闭环

当前运行时已经补齐了这些缺口：

- `/api/tasks/status` 不再是固定占位，而是聚合 `content_completion`、`firecrawl` 的实时工作量
- `ResetSchedulerStats` 会真实清空支持调度器的统计状态；其中持久化调度器会同步重置 `scheduler_tasks`
- `UpdateSchedulerInterval` 会真实更新运行中的调度器间隔，而不是只返回"重启后生效"文案
- `PreferenceUpdateScheduler` 已挂入统一 runtime registry，也能从 `/api/schedulers/*` 查询和触发
- 调度器对外使用 `content_completion` 作为规范名，同时继续兼容旧名 `ai_summary`

还保留的边界主要有两点：

- `runtimeinfo` 仍然是全局引用式共享，不是正式依赖注入容器
- `AISummarySchedulerInterface` 这个变量名还没重命名，只是在 API 层做了规范名映射

## 优雅退出怎么做

`SetupGracefulShutdown(runtime)` 监听：

- `SIGINT`
- `SIGTERM`

收到信号后会按顺序停止：

- TagQueue
- EmbeddingQueueWorker
- MergeReembeddingQueueWorker
- AutoRefresh
- PreferenceUpdate
- ContentCompletion
- Firecrawl
- BlockedArticleRecovery
- AutoTagMerge
- TagQualityScore
- NarrativeSummary

最后等待 30 秒超时后 `os.Exit(0)`。当前没有额外的 HTTP server drain 或任务持久化恢复逻辑，所以更准确的说法是“基础优雅退出”，不是复杂的停机编排。

## 读代码建议

如果你想顺着运行时看代码，建议按这个顺序：

1. `backend-go/cmd/server/main.go`
2. `backend-go/internal/app/router.go`
3. `backend-go/internal/app/runtime.go`
4. `backend-go/internal/jobs/handler.go`
5. `backend-go/internal/jobs/content_completion.go`
6. `backend-go/internal/jobs/narrative_summary.go`
7. `backend-go/internal/jobs/tag_hierarchy_cleanup.go`
再回到 `docs/architecture/backend-go.md` 看业务分层，会比较容易把“启动装配”和“业务链路”对上。
