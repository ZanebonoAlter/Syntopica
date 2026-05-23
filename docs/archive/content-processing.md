# 内容处理链路

## 这份文档讲什么

这里专门说明后端当前的“正文增强 + 文章级 AI 内容整理”链路，也就是：

- feed 刷新后文章怎样进入待处理状态
- Firecrawl 怎样抓正文
- 内容补全怎样基于正文生成 `ai_content_summary`
- 前端现在能通过哪些接口看到状态

这份文档只讲文章级内容处理链路，不涉及其他子系统。

## 当前真实链路

```text
Feed refresh
  -> article created
  -> firecrawl_status = pending (if feed.firecrawl_enabled)
  -> Firecrawl scheduler fetches full content
  -> firecrawl_status = completed
  -> summary_status = incomplete (if feed.article_summary_enabled)
  -> ContentCompletion scheduler summarizes article body
  -> ai_content_summary written back to articles
  -> summary_status = complete
```

一句话概括：当前内容处理链路的核心对象始终是 `articles` 表，Firecrawl 和内容补全都在给 article 补字段。

## 当前关键模块

### 后端入口

- Feed 刷新：`backend-go/internal/domain/feeds/service.go`
- 内容补全 handler：`backend-go/internal/domain/contentprocessing/content_completion_handler.go`
- 内容补全 service：`backend-go/internal/domain/contentprocessing/content_completion_service.go`
- Firecrawl handler：`backend-go/internal/domain/contentprocessing/firecrawl_handler.go`
- Firecrawl 配置读取：`backend-go/internal/domain/contentprocessing/firecrawl_config.go`
- Firecrawl service：`backend-go/internal/domain/contentprocessing/firecrawl_service.go`

### 调度器

- 自动刷新：`backend-go/internal/jobs/auto_refresh.go`
- 内容补全：`backend-go/internal/jobs/content_completion.go`
- Firecrawl：`backend-go/internal/jobs/firecrawl.go`

### 前端相关位置

- 编辑 feed：`front/app/features/shell/components/FeedLayoutShell.vue`
- 文章正文视图：`front/app/features/articles/components/ArticleContentView.vue`
- 内容补全 composable：`front/app/features/articles/composables/useContentCompletion.ts`
- 内容来源切换工具：`front/app/utils/articleContentSource.ts`

## 关键状态字段

内容处理链路主要依赖这些字段。

### feed 级开关

- `feeds.firecrawl_enabled`：该 feed 的文章是否进入 Firecrawl 队列
- `feeds.article_summary_enabled`：该 feed 的文章是否允许生成 `ai_content_summary`
- `feeds.max_completion_retries`：内容补全最大重试次数
- `feeds.completion_on_refresh`：当前模型里已有该字段，但现有主链路仍主要依赖 scheduler 扫描状态位，而不是直接在刷新后同步执行

### article 级状态

- `articles.firecrawl_status`：`pending` / `processing` / `completed` / `failed`
- `articles.firecrawl_content`：Firecrawl 抓回来的 markdown 正文
- `articles.firecrawl_error`：抓取失败原因
- `articles.firecrawl_crawled_at`：最近一次抓取完成时间
- `articles.summary_status`：内容补全状态，常见值有 `incomplete` / `pending` / `complete` / `failed`
- `articles.ai_content_summary`：文章级 AI 内容整理结果
- `articles.completion_attempts`：已尝试次数
- `articles.completion_error`：最近一次内容补全失败原因
- `articles.summary_generated_at`：内容补全生成时间

## 具体用例 1：feed 刷新后如何把文章送进内容处理链路

场景：某个 feed 开启了自动刷新，并且用户给这个 feed 打开了 Firecrawl 和文章级 AI 总结。

链路：

1. `auto_refresh` 调度器扫描到点 feed
2. `feeds.FeedService.RefreshFeed(feedID)` 拉 RSS 并解析 entry
3. 新文章通过 `buildArticleFromEntry()` 写入数据库
4. 文章初始状态按 feed 开关写入：
   - 默认 `summary_status = complete`
   - 如果 `feed.firecrawl_enabled = true`，则改成 `firecrawl_status = pending`
   - 如果同时 `feed.article_summary_enabled = true`，则再改成 `summary_status = incomplete`

这一步很关键：内容处理链路不是通过单独队列表驱动，而是通过 article 上的状态字段进入后续 scheduler 扫描范围。

## 具体用例 2：Firecrawl 定时抓正文

场景：文章已经被标记为 `firecrawl_status = pending`。

链路：

1. `jobs.FirecrawlScheduler` 每 300 秒轮询一次
2. 先读取 `GetFirecrawlConfig()`，配置来自 `aisettings`，为空时会尝试兼容旧 `summary_config.firecrawl`
3. 只查询满足以下条件的文章：
   - `feeds.firecrawl_enabled = true`
   - `articles.firecrawl_status = pending`
4. 每次最多取 50 篇，当前实现是单线程串行抓取，`concurrency = 1`
5. 开始抓取前先把文章改成 `firecrawl_status = processing`
6. `FirecrawlService.ScrapePage()` 请求远端 `/v1/scrape`，返回 markdown/html
7. 成功后更新文章：
   - `firecrawl_status = completed`
   - `firecrawl_content = markdown`
   - `firecrawl_crawled_at = now`
   - 如果 `feed.article_summary_enabled = true`，再把 `summary_status = incomplete`
8. 失败则写：
   - `firecrawl_status = failed`
   - `firecrawl_error = err`

同时这条链路会通过 `platform/ws` 广播 `firecrawl_progress`，前端可以看到当前批次的抓取进度。

## 具体用例 3：内容补全定时生成文章级 AI 摘要

场景：Firecrawl 已经成功抓到正文，且该 feed 开启了文章级内容整理。

链路：

1. `jobs.ContentCompletionScheduler` 每 60 分钟检查一次
2. 它只查询满足以下条件的文章：
   - `articles.firecrawl_status = completed`
   - `articles.summary_status = incomplete`
   - `feeds.article_summary_enabled = true`
3. 执行 `completionService.CompleteArticle(article.ID)`
4. `CompleteArticleWithForce()` 会依次校验：
   - article 存在
   - feed 存在
   - feed 开启 `article_summary_enabled`
   - `firecrawl_status == completed`
   - 没超过 `max_completion_retries`（非 force 情况）
5. 开始处理时先把 article 改成：
   - `summary_status = pending`
   - `completion_attempts += 1`
   - `completion_error = ""`
6. 内容来源优先使用 `firecrawl_content`
7. AI 调用优先走 `airouter.CapabilityArticleCompletion`，如果没有路由配置，再退回直接 AI service
8. 成功后写回：
   - `ai_content_summary`
   - `summary_status = complete`
   - `summary_generated_at`
9. 失败后写回：
   - `completion_error`
   - 若达到上限则 `summary_status = failed`
   - 否则回退成 `summary_status = incomplete`

这里的核心区别是：这条链路生成的是 article 级整理结果，写在 `articles.ai_content_summary`，不是 `ai_summaries` 表里的 feed 聚合摘要。

## 具体用例 4：手动补全与手动抓取

除了定时任务，当前也支持手动触发。

### 手动抓单篇正文

- 接口：`POST /api/firecrawl/article/:id`
- 要求：该 article 所属 feed 必须开启 `firecrawl_enabled`，且全局 Firecrawl 配置也必须启用
- 成功后立即写回 `firecrawl_content`，并把 `summary_status` 设为 `incomplete`

### 手动补单篇文章内容摘要

- 接口：`POST /api/content-completion/articles/:article_id/complete`
- 支持 body：`{ "force": true|false }`
- `force=true` 时会忽略已完成态，重新生成摘要

### 手动补整条 feed 的文章

- 接口：`POST /api/content-completion/feeds/:feed_id/complete-all`
- 现在会扫描该 feed 下 `summary_status in ('incomplete', 'failed')` 的文章，并逐条强制补全

## 当前状态接口怎么看

### scheduler 视角

前端可以通过 `GET /api/schedulers/status` 看这些调度器：

- `auto_refresh`
- `content_completion`
- `firecrawl`
- `preference_update`
- `digest`
- `blocked_article_recovery`
- `tag_quality_score`
- `narrative_summary`

其中 `content_completion` 是文章级内容补全 scheduler，不是 feed 聚合摘要任务。

为了兼容旧前端和旧调用，后端目前仍接受 `ai_summary` 作为 `content_completion` 的别名。

`blocked_article_recovery` 调度器会定期恢复卡在 `processing` 状态的文章，将其重置为可重新处理的状态。

### 内容补全视角

`GET /api/content-completion/overview` 会返回更贴近文章处理链路的总览：

- `pending_count`
- `processing_count`
- `completed_count`
- `failed_count`
- `blocked_count`
- `ai_configured`
- `blocked_reasons`

其中 `blocked_reasons` 会细分：

- `waiting_for_firecrawl_count`
- `feed_disabled_count`
- `ai_unconfigured_count`
- `ready_but_missing_content_count`

### 单文章视角

`GET /api/content-completion/articles/:article_id/status` 会返回：

- `summary_status`
- `attempts`
- `error`
- `summary_generated_at`
- `ai_content_summary`
- `firecrawl_content`
- `firecrawl_status`
- `firecrawl_error`
- `firecrawl_crawled_at`

这也是前端文章详情页切换“原始内容 / Firecrawl 内容”与显示内容补全状态时的主要来源。

## 当前实现的真实限制

这里要把现状写清楚，避免误解成“链路已经完全闭环”。

- Firecrawl scheduler 当前是串行处理，不是高并发抓取
- runtime 内部变量名仍沿用 `AISummarySchedulerInterface`，只是 API 层已经统一对外使用 `content_completion`
- `completion_on_refresh` 虽然存在于 feed 模型，但主链路仍然是“刷新先打状态，再由 scheduler 异步消费”
- 内容补全依赖 `firecrawl_content`，如果正文为空，文章会一直留在待处理/失败回退路径里
- scheduler 统一接口只覆盖运行时状态，不等于完整任务审计系统

## 推荐阅读顺序

- 先看 `backend-go/internal/domain/feeds/service.go`
- 再看 `backend-go/internal/jobs/firecrawl.go`
- 再看 `backend-go/internal/domain/contentprocessing/content_completion_service.go`
- 再看 `backend-go/internal/jobs/content_completion.go`
- 最后对照 `front/app/features/articles/components/ArticleContentView.vue` 看前端如何消费这些字段
