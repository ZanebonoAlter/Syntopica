# 后端架构

## 先说结论

这份文档只描述当前 `backend-go/` 已经落地的真实结构，不再沿用旧的预期分层。

当前后端可以直接按四层理解：

- `cmd/`：启动入口和辅助命令
- `internal/app/`：应用装配、路由注册、运行时启动与退出
- `internal/platform/`：数据库、配置、AI 路由、WebSocket、共享基础设施
- `internal/domain/` + `internal/jobs/`：业务域能力与后台调度执行壳

如果你发现文档和代码不一致，优先相信源码入口：`backend-go/cmd/server/main.go`、`backend-go/internal/app/router.go`、`backend-go/internal/app/runtime.go`。

## 技术栈

- Go 1.21
- Gin
- GORM
- PostgreSQL + pgvector
- Viper
- Gorilla WebSocket
- robfig/cron

## 当前真实入口

- 服务入口：`backend-go/cmd/server/main.go`
- 路由装配：`backend-go/internal/app/router.go`
- 运行时启动：`backend-go/internal/app/runtime.go`
- 运行时共享引用：`backend-go/internal/app/runtimeinfo/schedulers.go`
- 配置加载：`backend-go/internal/platform/config/config.go`
- 数据库初始化与表补丁：`backend-go/internal/platform/database/db.go`
- 配置文件：`backend-go/configs/config.yaml`

## 当前目录现实

```text
backend-go/
├── cmd/
│   ├── migrate-db/
│   ├── migrate-embedding-queue/
│   ├── migrate-tags/
│   ├── server/
│   └── test-embedding/
├── configs/
├── internal/
│   ├── app/
│   │   └── runtimeinfo/
│   ├── domain/
│   │   ├── aiadmin/
│   │   ├── articles/
│   │   ├── categories/
│   │   ├── contentprocessing/
│   │   ├── feeds/
│   │   ├── models/
│   │   ├── narrative/
│   │   ├── preferences/
│   │   ├── topicanalysis/
│   │   ├── topicextraction/
│   │   ├── topicgraph/
│   │   └── topictypes/
│   ├── jobs/
│   └── platform/
│       ├── ai/
│       ├── airouter/
│       ├── aisettings/
│       ├── config/
│       ├── database/
│       ├── logging/
│       ├── middleware/
│       ├── opennotebook/
│       ├── tracing/
│       └── ws/
```

## 分层职责

### `cmd/`

- `server/`：HTTP 服务真实入口
- `migrate-tags/`：主题标签相关迁移命令
- `migrate-db/`：数据库迁移命令
- `migrate-embedding-queue/`：embedding 队列迁移命令
- `test-embedding/`：embedding 联调入口

### `internal/app/`

这是应用壳层，负责把平台能力、业务域和后台任务接起来。

- `router.go`：注册 HTTP API 和 WebSocket 路由
- `runtime.go`：启动 scheduler、初始化内容补全服务、注册优雅退出
- `runtimeinfo/`：临时保存运行时实例，给 handler 查询状态或触发任务

这里要注意：`runtimeinfo` 还是过渡方案，它不是完整的 runtime container。

### `internal/platform/`

这是共享基础设施层，不承载具体业务语义。

- `config/`：读取 `configs/config.yaml`
- `database/`：初始化 PostgreSQL、建表、索引、字段补丁
- `logging/`：轻量日志门面，负责把 info/warn 与 error/fatal/panic 分流到 stdout / stderr
- `middleware/`：Gin 中间件，例如 CORS
- `ws/`：WebSocket hub，给前端推送异步任务状态
- `ai/`：AI 调用封装
- `airouter/`：AI provider、capability route、failover 路由
- `aisettings/`：兼容旧配置表的 AI / Firecrawl / Open Notebook 配置读写
- `opennotebook/`：Open Notebook 客户端能力

### `internal/domain/`

业务能力按域组织，handler 和 service 主要都放在域目录里。

- `aiadmin/`：AI provider 与 capability route 管理
- `categories/`：分类 CRUD
- `feeds/`：订阅 CRUD、刷新、OPML、RSS 解析
- `articles/`：文章列表、详情、状态更新、统计
- `preferences/`：阅读行为记录与偏好分析
- `contentprocessing/`：内容补全、Firecrawl 配置与抓取、文章正文处理
- `topictypes/`：主题图谱共享类型和窗口工具
- `topicextraction/`：摘要/文章标签提取
- `topicanalysis/`：主题分析任务与分析结果 API、embedding 向量化、标签合并、关注标签、抽象标签
- `topicgraph/`：主题图谱、主题详情、主题相关文章查询
- `models/`：共享 GORM 模型和部分格式化 helper
- `narrative/`：叙事摘要生成、Board 管理、BoardConcept 匹配、按日期查询、历史版本
  - 叙事域完整文件清单：
    ```
    narrative/
    ├── service.go           # 服务编排
    ├── handler.go           # REST API 路由
    ├── collector.go         # 数据采集
    ├── generator.go         # AI 叙事生成
    ├── board_creation.go    # Board 创建
    ├── board_generator.go   # Board 级叙事生成
    ├── board_narrative_generator.go  # Board 叙事生成（概念上下文）
    ├── board_collector.go   # Board 数据收集
    ├── board_merge.go       # Board 合并（部分废弃）
    ├── board_postprocess.go # Board 后处理
    ├── concept_service.go   # Board Concept CRUD
    ├── concept_handler.go   # Board Concept REST API
    ├── concept_embedding.go # 概念 embedding 生成
    ├── concept_matcher.go   # Embedding 匹配引擎
    ├── concept_suggestion.go # LLM 冷启动建议
    ├── watched_narrative.go # 关注标签叙事
    ├── tag_feedback.go      # 叙事反馈到标签
    └── *_test.go            # 测试
    ```

### `internal/jobs/`

这里是调度外壳，不放完整业务，只负责定时触发和运行状态记录。

- `auto_refresh.go`：扫描到点 feed 并异步触发刷新
- `content_completion.go`：对 `firecrawl completed + summary incomplete` 的文章做内容补全
- `firecrawl.go`：轮询待抓取文章并执行 Firecrawl
- `tag_quality_score.go`：每小时重算 `topic_tags.quality_score`，支持统一 scheduler 状态查询和手动触发
- `preference_update.go`：阅读偏好更新任务
- `blocked_article_recovery.go`：恢复因 Firecrawl 配置变更等原因阻塞的文章
- `narrative_summary.go`：基于活跃主题标签生成每日叙事摘要
- `handler.go`：scheduler 状态查询与手动触发 API

## 当前主要子系统

### 订阅与文章

`feeds` 和 `articles` 是基础数据面。

- feed 刷新负责拉 RSS、去重、入库 article
- article 记录承接后续 Firecrawl、内容补全、摘要、主题分析
- feed 上的 `firecrawl_enabled`、`article_summary_enabled` 会直接影响文章入库后的状态初始化

### AI 与内容增强

这部分不再只是一个"AI 摘要开关"，而是两层叠加：

- `platform/airouter`：管理 provider 和 capability route
- `domain/contentprocessing`：正文抓取、内容补全、Firecrawl 配置

### 主题图谱

主题能力已经拆成四个包：

- `topictypes`：共享类型和窗口解析
- `topicextraction`：从摘要/文章提取 topic tag
- `topicanalysis`：生成并查询 topic analysis，同时承担 embedding 向量化、标签合并、关注标签、抽象标签管理
- `topicgraph`：返回图谱节点边、详情、相关文章、相关 digest

当前 `topicanalysis` 里的抽象标签整理链路有三层保护，避免重复抽象标签和错误扁平化：

- `processAbstractJudgment` 在创建新 abstract tag 前，会先用临时 semantic embedding 做 shortlist，再让 LLM 判断是否应复用已有同概念 abstract tag
- `MatchAbstractTagHierarchy` 会遍历多个高相似 abstract 候选；高相似时优先判断“合并还是上下位关系”，而不是默认继续嵌套
- 新建或复用 abstract tag 并挂上子标签后，会异步执行 `adoptNarrowerAbstractChildren`，主动把更窄的已有 abstract tag 收养进来；如果候选已经有更具体的中间父节点，则保留中间层，只补更宽的父子关系

依赖方向大致是：

```text
topictypes
    ↑
    ├── topicgraph
    ├── topicanalysis (含 embedding、tag merge、watched tags、abstract tags)
    └── topicextraction -> topicanalysis
```

### 叙事摘要

叙事摘要（`narrative/`）基于活跃主题标签和抽象标签树生成每日叙事，支持双轨制 Board 创建。

叙事系统的核心概念是 Board（板块）和 BoardConcept（板块概念）：

- **Board**（`narrative_boards` 表）：每日生成的叙事分组容器，通过 `scope_type`/`scope_category_id` 控制作用域
- **BoardConcept**（`board_concepts` 表）：持久化的板块概念实体，跨天存在，通过 embedding 匹配接收小抽象树和未分类 event 标签

#### 双轨制 Board 创建

每日生成时走两条轨道：

- **热点板轨道**：大抽象树（≥6 节点，阈值可配置）→ 自动创建热点 Board（`is_system=true`），支持跨日延续（`prev_board_ids`）
- **概念板轨道**：小抽象树 + 未分类 event → embedding cosine similarity 匹配 BoardConcept → 创建概念 Board（`board_concept_id` 不为空）

未匹配的标签进入"未归类桶"，超过阈值时触发 LLM 建议新 BoardConcept。

#### 生成流程

`GenerateAndSave(date)` 入口执行以下步骤：

1. `GenerateAndSaveForAllCategories` — 逐分类双轨生成
2. `GenerateAndSaveGlobal` — 全局 embedding 匹配生成
3. `runFallbackAssociations` — 关联前日叙事
4. `DeriveBoardConnections` — 派生 Board 间连接
5. `runFeedbackFromTodayNarratives` — 叙事反馈到标签
6. `cleanEmptyBoards` — 清理无叙事关联的空 Board

#### Board Concept 管理

- LLM 冷启动：扫描所有 active abstract tags 建议初始概念列表
- 用户通过前端 `BoardConceptManager` 审阅/接受/拒绝/手动创建
- CRUD API：`/api/narratives/board-concepts`
- 概念 embedding 在创建/更新时自动生成

#### 关联叙事后处理

- 叙事反馈（`tag_feedback.go`）：检查叙事关联的 event 标签对，触发抽象标签创建
- 关注标签叙事（`watched_narrative.go`）：为关注标签生成维度总结（`period=watched_tag`）

#### 调度

- `NarrativeSummaryScheduler` 按配置间隔运行
- 手动触发：`POST /api/narratives/regenerate`（JSON body 含 date、scope_type、category_id）

## 数据模型重点

旧文档只写 feed/article 基础字段已经不够，当前后端的数据面至少包含这些正式能力。

### `feeds`

- `article_summary_enabled`
- `completion_on_refresh`
- `max_completion_retries`
- `firecrawl_enabled`
- `refresh_interval`
- `refresh_status`

### `articles`

- `image_url`
- `summary_status`
- `summary_generated_at`
- `ai_content_summary`
- `completion_attempts`
- `completion_error`
- `firecrawl_status`
- `firecrawl_content`
- `firecrawl_error`
- `firecrawl_crawled_at`

### 其他关键表/模型

- `ai_settings`：兼容旧配置存储
- `ai_providers` / `ai_routes` / `ai_route_providers`：AI 路由配置
- `scheduler_tasks`：scheduler 最近执行状态、耗时、错误、结果摘要
- 主题图谱相关模型：`topic_tags`、`topic_tag_analyses`、`topic_tag_embeddings` 等
  - `topic_tags.quality_score`：按频率、共现、来源分散度、语义默认分得到的客观质量分，普通标签先算，抽象标签再按 child 加权平均
- 叙事板相关模型：`narrative_boards`、`board_concepts`
  - `narrative_boards.board_concept_id`：关联持久化概念板
  - `narrative_boards.is_system`：区分系统生成的热点板和用户概念板
  - `board_concepts.embedding`：pgvector 向量列，用于概念匹配

## 真实 API 面

`internal/app/router.go` 当前已经注册这些主路由组：

- `/api/categories`
- `/api/feeds`
- `/api/articles`
- `/api/ai`
- `/api/schedulers`
- `/api/reading-behavior`
- `/api/user-preferences`
- `/api/content-completion`
- `/api/firecrawl`
- `/api/topic-graph`
- `/api/import-opml` / `/api/export-opml`
- `/ws`

其中 `topic-graph` 组下面还挂了 `analysis` 子路由，AI 管理则已经扩展到 provider 和 route 级别，而不是只有"摘要设置"一个入口。

此外还有以下独立注册的路由组：

- `/api/topic-tags`：关注标签、标签合并预览、抽象标签管理（由 `topicanalysis` 包注册）
- `/api/embedding`：embedding 配置与队列管理（由 `topicanalysis` 包注册）
- `/api/narratives`：叙事摘要时间线、列表、详情、历史、重新生成（由 `narrative` 包注册）
- `/api/narratives/boards`：Board 时间线和详情
- `/api/narratives/board-concepts`：板块概念 CRUD 和 LLM 建议（由 `narrative` 包注册）
- `/api/narratives/unclassified`：未分类标签桶

## 具体数据链路示例

下面这几条链路是当前代码里真实存在、而且最值得在阅读代码时重点跟的主线。

### 用例 1：自动刷新 feed -> 新文章入库

场景：用户给某个 feed 配了刷新间隔，或者手动触发 `/api/schedulers/auto_refresh/trigger`。

链路：

1. `internal/jobs/auto_refresh.go` 扫描 `refresh_interval > 0` 的 feed
2. 到点 feed 调用 `feeds.FeedService.RefreshFeed`
3. `RefreshFeed` 通过 RSS parser 拉取源站内容并更新 feed 元信息
4. 新 entry 去重后写入 `articles`
5. `buildArticleFromEntry` 按 feed 配置初始化文章状态：
   - 默认 `summary_status = complete`
   - 如果 feed 开启 `firecrawl_enabled`，则文章先标记 `firecrawl_status = pending`
   - 如果同时开启 `article_summary_enabled`，则文章再标记 `summary_status = incomplete`
6. `cleanupOldArticles` 按 `max_articles` 清理旧文章（收藏文章跳过）

这个链路的关键点是：feed 刷新不只是在“加文章”，它还会把后续 Firecrawl / 内容补全链路需要的状态位一起种进去。

### 用例 2：Firecrawl 抓正文 -> 内容补全生成文章摘要

场景：某个 feed 开启了 Firecrawl，前面的刷新流程已经把文章打成 `firecrawl_status = pending`。

链路：

1. `jobs.FirecrawlScheduler` 轮询待抓取文章
2. Firecrawl 成功后，文章被更新为：
   - `firecrawl_status = completed`
   - `firecrawl_content` 写入抓取正文
   - `summary_status = incomplete`
3. `jobs.ContentCompletionScheduler` 定时查询：
   - `articles.firecrawl_status = completed`
   - `articles.summary_status = incomplete`
   - `feeds.article_summary_enabled = true`
4. `contentprocessing.ContentCompletionService.CompleteArticle` 基于 Firecrawl 正文生成 `ai_content_summary`
5. 文章最终更新为完成态，并记录失败次数、错误信息、最近处理文章等状态
6. 前端可通过 `/api/content-completion/overview` 和 `/api/content-completion/articles/:article_id/status` 看到结果

这条链路说明：运行时对外现在用 `content_completion` 作为规范 scheduler 名字，但仍兼容旧别名 `ai_summary`；它对应的是“文章级内容补全”，不是 `ai_summaries` 表里的 feed 聚合摘要。

### Article 打标签时机

文章标签现在按以下规则运行：

1. 普通 refresh 新文章：入库后立即打标签（feed 未开启 Firecrawl 时）
2. 若 feed 开启了 `Firecrawl`：refresh 阶段先不打标签
	- Firecrawl 抓取完成后，写入 `tag_jobs` 队列，由 `TagQueue` worker 异步执行重新打标签
	- 若 feed 同时开启了 `自动补全`（`article_summary_enabled`），则由 ContentCompletion scheduler 在生成 `AIContentSummary` 后同样 enqueue `tag_jobs`
3. 前端文章详情支持手动打标签 / 重新打标签，接口为 `POST /api/articles/:article_id/tags`
	- 手动接口现在只 enqueue 队列并返回 `job_id`
	- `TagQueue` 完成后通过 WebSocket 广播 `tag_completed`
	- LLM 提示词明确要求最多返回 `8` 个标签，并按优先级从高到低排序；后端在写入 `article_topic_tags` 前也会只保留前 `8` 个，作为兜底
4. `TagQueue.Start()` 首次启动失败时不会阻塞应用；它会后台按 30 秒间隔重试最多 10 次

当前正文提取优先级为：

- `AIContentSummary`
- `FirecrawlContent`
- `Content`
- `Description`

## 当前边界上的真实问题

当前问题已经不是“目录乱”，而是这些边界还在过渡：

- `runtimeinfo` 仍是全局变量式共享引用，适合过渡，不适合长期扩展
- `domain/models` 仍是共享模型桶，后续还可以继续收敛 ownership
- `aisettings` 同时承担兼容旧配置和新配置落库，职责偏宽
- `runtimeinfo` 仍是全局变量式共享引用，但当前至少已经把实际启动的 scheduler 全部挂进统一入口
- `/api/tasks/status` 现在是聚合视图，不是通用任务编排系统；它反映的是 summary queue、内容补全、firecrawl 三类后台工作

## 推荐阅读顺序

- 先看 `docs/architecture/backend-runtime.md`
- 再看 `backend-go/cmd/server/main.go`
- 再看 `backend-go/internal/app/router.go`
- 再看 `backend-go/internal/app/runtime.go`
- 再按用例追具体域：`feeds` -> `contentprocessing` -> `topic*` -> `narrative`
- 叙事域能力可以按以下顺序跟：`narrative/service.go` → `narrative/collector.go` → `narrative/board_creation.go` → `narrative/concept_matcher.go` → `narrative/concept_service.go`
