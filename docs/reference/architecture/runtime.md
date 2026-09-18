# 后端运行时与接口

## 先看启动主线

当前 Go 后端真实启动顺序在 `backend-go/cmd/server/main.go`，顺序如下：

1. `config.LoadConfig("./configs")` 读取配置
2. `database.InitDB(config.AppConfig)` 初始化 PostgreSQL、建表和索引
3. 各业务域 `InitRepository(database.DB)`：`admin`、`reader`、`taggingdomain`、`topicgraph`
4. `taggingdomain.EnsureVectorDimensionOnce()` 校正 `semantic_labels.embedding` 向量维度与 embedder 一致
5. `tracing.InitTracerProvider(database.DB, traceCfg)` 初始化 OpenTelemetry tracing
6. 根据配置切换 Gin `debug/release` 模式
7. 创建 `gin.Engine`，挂载 CORS 与 Recovery、otelgin 中间件
8. `app.SetupStaticFiles(r)` + `app.SetupRoutes(r)` 注册静态资源与 HTTP / WebSocket 路由
9. `app.StartRuntime()` 启动后台 scheduler 与内容补全服务（`DEMO_READ_ONLY=1` 时跳过，`runtime` 传 nil）
10. `app.SetupGracefulShutdown(runtime, srv)` 注册优雅退出（所有模式都注册）
11. `app.RunServer(srv, done)` 开始监听；收到 SIGTERM/SIGINT 并完成关停序列后 main 正常 return（不再 `os.Exit`，`tracer Shutdown` / `logging.Close` 等 defer 真实执行）

所以 `cmd/server` 现在只是薄入口，真正的运行时装配已经集中在 `internal/app/`。

## Runtime 里实际启动了什么

`backend-go/internal/app/runtime.go` 里定义的 `Runtime` 目前会启动 10 类后台任务：

- `AutoRefresh`：扫描到点 feed 并触发刷新
- `PreferenceProfileUpdate`：以 `reading_behaviors` 为权重源重算偏好向量画像（纯向量算术，零 LLM）
- `RSSHubCatalogSync`：同步自建 RSSHub 实例路由目录 + 增量可用性校验 + 新路由 embedding
- `ContentCompletion`：基于正文生成文章级摘要
- `Firecrawl`：抓取文章完整正文（readability 进程内主力，Firecrawl 仅在 readability 不合格时兜底 SPA 站点）
- `BlockedArticleRecovery`：恢复因 Firecrawl 配置变更等原因阻塞的文章
- `DailyReport`：基于活跃主题标签生成每日叙事摘要
- `TagQualityScore`：重算 `topic_tags.quality_score`
- `LogCleanup`：清理过期日志
- `AuxLabelCleanup`：清理辅助标签

此外还会启动以下异步队列 worker：

- `tagging.StartAllWorkers()`：统一启动标签相关 worker（标签打标队列、embedding 向量化队列、合并后 re-embedding 队列）

对应启动逻辑也都在 `StartRuntime()` 里，不存在额外的隐藏入口。

## 运行时共享状态怎么暴露

当前 runtime 采用 `SchedulerRegistry` 模式对外暴露调度器：

- `registry.Register(name, scheduler)`：注册调度器
- `registry.StartAll()`：启动所有已注册调度器
- `registry.StopAll()`：停止所有已注册调度器

调度器通过 `internal/admin/scheduler` 包的工厂模式创建，每个调度器只需一个 `JobFunc` 函数 + 一行注册代码。

## 启动参数与默认值

`StartRuntime()` 里目前写死了几组默认间隔：

- `auto_refresh`：60 秒检查一次
- `preference_profile_update`：3600 秒检查一次（偏好向量画像重算，零 LLM）
- `rsshub_catalog_sync`：86400 秒检查一次（每天一次，同步 RSSHub 路由目录）
- `content_completion`：60 秒检查一次
- `firecrawl`：300 秒检查一次
- `blocked_article_recovery`：3600 秒检查一次
- `tag_quality_score`：3600 秒检查一次
- `daily_report`：86400 秒检查一次（每天一次）
- `log_cleanup`：86400 秒检查一次（每天一次）
- `aux_label_cleanup`：3600 秒检查一次

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
- `/api/preference-profile`：偏好向量画像读取与手动重算
- `/api/discovery`：订阅源发现（RSSHub 目录同步 / 推荐卡片 / 问答）
- `/api/settings/rsshub`：RSSHub 实例配置读写

### AI 与内容处理 API

- `/api/ai/summarize`：单篇摘要
- `/api/ai/settings`：旧摘要设置读写兼容入口
- `/api/ai/providers`：AI provider 管理
- `/api/ai/routes`：AI capability route 管理
- `/api/content-completion`：文章级内容补全触发、状态与总览
- `/api/firecrawl`：单文章抓取、feed Firecrawl 开关、状态、配置保存

### 主题与 digest API

- `/api/embedding`：embedding 配置与队列管理
- `/api/topic-tags`：关注标签、标签合并预览
- ~~`/api/narratives`~~：已废弃（narrative 生成并入 daily_report；历史数据经 `/api/semantic-boards/:id/narratives` 只读）

### Scheduler API

统一入口在 `/api/schedulers`：

- `GET /api/schedulers/status`
- `GET /api/schedulers/:name/status`
- `POST /api/schedulers/:name/trigger`
- `POST /api/schedulers/:name/reset`
- `PUT /api/schedulers/:name/interval`

这组 API 现在统一覆盖：

- `auto_refresh`
- `preference_profile_update`
- `rsshub_catalog_sync`
- `content_completion`
- `firecrawl`
- `tag_quality_score`
- `daily_report`
- `log_cleanup`
- `aux_label_cleanup`
- `blocked_article_recovery`

另外保留一个兼容别名：

- `ai_summary` -> `content_completion`

但能力不是完全对称的：

- `auto_refresh`、`preference_profile_update`、`rsshub_catalog_sync`、`content_completion`、`firecrawl`、`tag_quality_score`、`daily_report`、`log_cleanup`、`aux_label_cleanup`、`blocked_article_recovery` 支持统一状态查询
- `auto_refresh`、`preference_profile_update`、`rsshub_catalog_sync`、`content_completion`、`firecrawl`、`tag_quality_score`、`daily_report`、`log_cleanup`、`aux_label_cleanup`、`blocked_article_recovery` 支持统一 trigger
- `auto_refresh`、`preference_profile_update`、`rsshub_catalog_sync`、`content_completion`、`firecrawl`、`tag_quality_score`、`daily_report`、`log_cleanup`、`aux_label_cleanup`、`blocked_article_recovery` 支持 `reset` / `interval`

## Scheduler 状态现在能看到什么

当前 scheduler 状态主要来自两层：

### 进程内状态

每个 scheduler 自己维护：

- 是否已启动
- 是否正在执行
- 下次运行时间
- 当前处理对象 / 最近处理对象（部分任务）

### 数据库存档状态

当前 10 个调度器（`auto_refresh`、`preference_profile_update`、`rsshub_catalog_sync`、`content_completion`、`firecrawl`、`blocked_article_recovery`、`daily_report`、`tag_quality_score`、`log_cleanup`、`aux_label_cleanup`）都通过 `NewTaskPersistence` 把最近一轮执行结果写进 `scheduler_tasks`，包含：

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
5. 前端随后就可以查 `/api/schedulers/status` 看到所有调度器状态

这个顺序意味着：即使某个 scheduler 启动失败，HTTP API 仍然会起来，只是状态接口会暴露失败结果或缺失的 runtime 引用。

### 链路 2：手动触发自动刷新

1. 前端请求 `POST /api/schedulers/auto_refresh/trigger`
2. `TriggerScheduler` 从全局注册表 `admin/handler.Reg`（`SchedulerRegistry`）取到 `auto_refresh` 调度器
3. 如果实现了 `TriggerNow()`（或 `TriggerNowWithDate()`），就直接返回是否接受、是否真的触发、拒绝原因
4. 调度器执行扫描，结果写回 `scheduler_tasks.last_execution_result`
5. 前端再查 `/api/schedulers/auto_refresh/status` 就能看到最近一轮扫描摘要

### 链路 3：内容补全状态更新

## 这次补上的闭环

当前运行时已经补齐了这些缺口：

- `/api/tasks/status` 不再是固定占位，而是聚合 `content_completion`、`firecrawl` 的实时工作量
- `ResetSchedulerStats` 会真实清空支持调度器的统计状态；其中持久化调度器会同步重置 `scheduler_tasks`
- `UpdateSchedulerInterval` 会真实更新运行中的调度器间隔，而不是只返回"重启后生效"文案
- `PreferenceProfileUpdateScheduler`（旧 `PreferenceUpdateScheduler` 随偏好分数体系废弃删除）与新 `RSSHubCatalogSyncScheduler` 已挂入统一 runtime registry，也能从 `/api/schedulers/*` 查询和触发
- 调度器对外使用 `content_completion` 作为规范名，同时继续兼容旧名 `ai_summary`

还保留的边界有一点：

- `admin/handler.Reg` 仍然是启动时由 `StartRuntime` 通过 `SetRegistry` 设置的全局变量，不是正式依赖注入容器；调度器对外使用 `content_completion` 作为规范名，同时通过持久化名 `ai_summary` 兼容旧前端

## 优雅退出怎么做

`SetupGracefulShutdown(runtime, srv)` 监听 `SIGINT` / `SIGTERM`，返回 `done` channel 供 `RunServer` 等待。收到信号后按**固定四步**执行（graceful-shutdown-hardening change 重构；实现见 `backend-go/internal/app/runtime.go`）：

1. **① `srv.Shutdown`（5s 上限）**：先关 listener——**端口立即释放**（不再等进程退出），同时排空在途 HTTP 请求；超时强制关闭（有界优先）
2. **② `Registry.StopAll(30s)`**（`runtime != nil` 时）：逐个停调度器，上限保持历史值；超时只记 warn
3. **③ 工作队列停止（外包 10s 上限）**：`tagging.StopAllWorkers()`（tag queue / embedding / merge-reembedding 三队列）在独立 goroutine 里跑，外层 `select` + 10s 兜底；超时只告警不重试（卡死的 worker 随进程退出，队列持久化保证可恢复）。`workers.go` 本体零改动
4. **④ 耗时汇总**：单行日志 `shutdown steps: http=…s registry=…s workers=…s total=…s`，一次关停即可看清各步构成

任一步 panic / 异常 / 超时都**不阻断后续步骤**（每步独立 recover，失败记 warn）；序列结束 `close(done)` → main 正常 return → defer 链（tracer flush、`logging.Close()`）真实执行。`DEMO_READ_ONLY=1`（`runtime == nil`）跳过 ② 但仍做 HTTP 摘端口与 ③；端口占用等真实启动错误仍走 `logging.Fatalf`（非 0 退出码）。

关停预算：http 5s + registry 30s + workers 10s，但**端口在第一步就释放**（通常 1~2s），容器编排的 SIGTERM 宽限期与 `scripts/dev/start-dev.sh stop` 的观察窗都只关心端口与进程存活。

## 读代码建议

如果你想顺着运行时看代码，建议按这个顺序：

1. `backend-go/cmd/server/main.go`
2. `backend-go/internal/app/router.go`
3. `backend-go/internal/app/runtime.go`
4. `backend-go/internal/admin/scheduler/base.go`
5. `backend-go/internal/admin/scheduler/registry.go`
6. `backend-go/internal/admin/scheduler/job_*.go`
再回到 `docs/reference/architecture/backend.md` 看业务分层，会比较容易把“启动装配”和“业务链路”对上。
