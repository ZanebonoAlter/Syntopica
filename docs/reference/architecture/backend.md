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
│   │   ├── article/
│   │   ├── category/
│   │   ├── content/
│   │   ├── feed/
│   │   ├── models/
│   │   ├── narrative/
│   │   ├── preferences/
│   │   ├── tagging/
│   │   │   ├── analysis/
│   │   │   ├── extraction/
│   │   │   └── watched/
│   │   └── topicgraph/
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
- `category/`：分类 CRUD
- `feed/`：订阅 CRUD、刷新、OPML、RSS 解析
- `article/`：文章列表、详情、状态更新、统计
- `preferences/`：阅读行为记录与偏好分析
- `content/`：内容补全、Firecrawl 配置与抓取、文章正文处理
- `tagging/`：标签系统根包，共享类型和窗口工具、`StartAllWorkers`/`StopAllWorkers` 统一入口
  - `tagging/analysis/`：主题分析任务与分析结果 API、embedding 向量化、标签合并、辅助标签入库
  - `tagging/extraction/`：摘要/文章标签提取
  - `tagging/watched/`：关注标签管理
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

`feed` 和 `article` 是基础数据面。

- feed 刷新负责拉 RSS、去重、入库 article
- article 记录承接后续 Firecrawl、内容补全、摘要、主题分析
- feed 上的 `firecrawl_enabled`、`article_summary_enabled` 会直接影响文章入库后的状态初始化

### AI 与内容增强

这部分不再只是一个"AI 摘要开关"，而是两层叠加：

- `platform/airouter`：管理 provider 和 capability route
- `domain/content`：正文抓取、内容补全、Firecrawl 配置

### 主题图谱

主题标签能力统一在 `tagging/` 包下，按子包拆分：

- `tagging`（根包）：共享类型和窗口解析、`StartAllWorkers`/`StopAllWorkers` 统一入口
- `tagging/extraction`：从摘要/文章提取 Tag
- `tagging/analysis`：生成并查询 topic analysis，同时承担 embedding 向量化、Tag 合并（源 DELETE）、辅助标签入库（L1/L2/L3 三级匹配）
- `tagging/watched`：关注标签管理
- `topicgraph`：返回图谱节点边、详情、相关文章、相关 digest

#### 辅助标签入库三级匹配

`findOrCreateTag` 创建 Tag 后，辅助标签通过 `auxiliary_label_service.go` 入库：

1. **L1 slug/alias 精确匹配**：slug 或 aliases 中包含新标签 → 复用已有 auxiliary label，ref_count++
2. **L2 embedding ≥0.95 合并**：pgvector 余弦相似度 ≥0.95 → 合并到 ref_count 更大的一方，小方 label 加入大方 aliases
3. **L3 新建**：无匹配 → 创建 semantic_label(label_type=auxiliary) + 生成 embedding

禁用标签 (status=disabled) 不参与 L1/L2 匹配。

#### 标签创建流程（`findOrCreateTag`）

`findOrCreateTag` 采用简化的三级匹配，不再调用 LLM 判断：

1. 缓存命中 → 直接返回
2. Embedding 三级匹配：
   - **exact**：精确/别名匹配 → 复用已有 Tag，更新 label/source
   - **candidates**：相似候选 → 跳过 LLM 判断，直接 fall through 到创建
   - **no_match**：无匹配 → fall through 到创建
3. Fallback：slug+category 精确查找，否则创建新 Tag

Event Tag（`category=event`）在创建时跳过 `ensureTagEmbedding`，embedding 在描述+关键词生成后延迟入队。

#### Event 标签多行 Embedding

Event 标签采用多行 embedding 策略：

- **Title 行**：`embedding_type='semantic'`，文本 = label + description（不含文章上下文）
- **Keyword 行**：`embedding_type='event_keyword'`，每个关键词一行，由 LLM 从标签上下文提取 3-5 个关键实体/动作词，存储在 `metadata.event_keywords`

生成时序：`findOrCreateTag` 创建 Tag → `generateTagDescription` 生成描述+关键词 → 保存到 `metadata.event_keywords` → 入队 embedding queue → 队列 worker 生成 identity + semantic + 所有 event_keyword embedding。

#### Tag 合并（源 DELETE）

合并相似 Tag 时采用硬删除策略，不再使用 `status='merged'` 或 `status='inactive'`：

- `HardMergeTags(sourceID, targetID)` 迁移 article_topic_tags → 迁移 topic_tag_relations (children) → DELETE topic_tag_embeddings → DELETE topic_tags 源行
- AutoTagMerge 调度器基于 pgvector 余弦相似度 > 0.97 自动触发

#### SemanticBoard 匹配

Tag 入库后，`semantic_board_matching.go` 执行 Tag → SemanticBoard 匹配：

1. 读取 tag 的辅助标签和 active SemanticBoard composition
2. 直接命中：tag 的辅助标签 ∈ board 构成标签 → 直接挂载
3. 命中率 > 50% → 直接挂载
4. max_sim ≥ 0.8 → 直接挂载
5. 加权综合：0.6×max_sim + 0.4×hit_rate ≥ 阈值 → 挂载

默认最多 3 个 board，按匹配分排序。匹配结果写入 `topic_tag_board_labels`。

冷启动无 SemanticBoard 时不匹配、不报错。

#### SemanticBoard 升级建议

`semantic_board_upgrade.go` 实现两阶段升级：

1. 收集 ref_count ≥ 阈值的候选辅助标签
2. embedding 预聚类（average-link greedy，默认 cosine 距离阈值 0.35；可通过 `cluster_method` 配置切换回 centroid 模式）
3. 每簇补充 co-tag 事件上下文
4. LLM 判断：create_new / skip
5. 用户确认执行（支持前端 merge_into_existing 操作）

#### 回填队列

`semantic_board_backfill.go` 支持三种回填模式：

- all：所有 tag 重新匹配
- unassigned：只处理无归属的 tag
- board：只处理指定 board 的 tag

异步逐个执行，幂等覆盖。

依赖方向大致是：

```text
tagging (根包，含 topictypes 功能)
    ↑
    ├── topicgraph
    ├── tagging/analysis (含 embedding、tag merge、辅助标签入库 L1/L2/L3)
    ├── tagging/watched (关注标签)
    └── tagging/extraction -> tagging/analysis
```

### 叙事摘要

叙事摘要（`narrative/`）基于 SemanticBoard 派生每日 NarrativeBoard。

核心概念：

- **SemanticBoard**（`semantic_labels` 表，label_type=board）：全局共享的长期语义板块，不按 tag category 或 feed category 分表
- **NarrativeBoard**（`narrative_boards` 表）：每日叙事板实例，保留 scope_type/scope_category_id，从当日文章范围内属于该 SemanticBoard 的 event tags 派生

#### 生成流程

`GenerateAndSave(date)` 入口执行以下步骤：

1. `CollectSemanticBoardNarrativeInputs` — 按日期和 scope 收集每个 SemanticBoard 的 active event tags
2. 对每个有事件的 SemanticBoard，创建 NarrativeBoard（写入 semantic_board_id, event_tag_ids）
3. prev_board_ids 按 semantic_board_id + scope + 前一日续接
4. 后处理：`DeriveBoardConnections`、`runFeedbackFromTodayNarratives`、`cleanEmptyBoards`

#### 冷启动

无 SemanticBoard 时不生成任何 NarrativeBoard，不报错。用户需手动触发 LLM 升级建议创建第一批 SemanticBoard。

#### 多板块归属

topic_tag_board_labels 允许一个 tag 归属多个 SemanticBoard（默认最多 3 个），因此同一 event tag 及其文章可出现在多个 NarrativeBoard 中。

#### Board 叙事上下文

LLM 生成叙事摘要时，使用 SemanticBoard 的 label 和 description 作为 board context，不再使用 abstract tag 或 board_concepts。

#### 叙事域文件清单

```
narrative/
├── service.go           # 服务编排
├── handler.go           # REST API 路由
├── collector.go         # 数据采集
├── generator.go         # AI 叙事生成
├── board_creation.go    # Board 创建
├── board_generator.go   # Board 级叙事生成
├── board_narrative_generator.go  # Board 叙事生成
├── board_collector.go   # Board 数据收集
├── board_postprocess.go # Board 后处理
├── semantic_board_matching.go  # SemanticBoard 匹配
├── semantic_board_upgrade.go   # 语义板块升级建议
├── semantic_board_backfill.go # 语义板块回填
├── auxiliary_label_service.go # 辅助标签服务
├── board_handler.go     # SemanticBoard REST API
├── watched_narrative.go # 关注标签叙事
├── tag_feedback.go      # 叙事反馈到标签
└── *_test.go            # 测试
```

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
  - `topic_tags.quality_score`：按频率、共现、来源分散度、语义默认分得到的客观质量分
- 叙事板相关模型：`narrative_boards`、`semantic_labels`（label_type=board）
  - `narrative_boards.semantic_board_id`：关联持久化 SemanticBoard
  - `topic_tag_board_labels`：tag-SemanticBoard 匹配结果
  - `topic_tag_semantic_labels`：tag-辅助标签关联
  - `board_composition`：SemanticBoard 构成标签

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

- `/api/topic-tags`：关注标签、标签合并预览（由 `tagging/analysis` 包注册）
- `/api/embedding`：embedding 配置与队列管理（由 `tagging/analysis` 包注册）
- `/api/narratives`：叙事摘要时间线、列表、详情、历史、重新生成
- `/api/narratives/boards`：Board 时间线和详情
- `/api/semantic-boards`：SemanticBoard CRUD、升级建议、回填、匹配配置
- `/api/auxiliary-labels`：辅助标签池查询和治理
- `/api/tags/:id/auxiliary-labels`：tag 辅助标签查询
- `/api/tags/:id/semantic-boards`：tag 所属 SemanticBoard 查询

## 具体数据链路示例

下面这几条链路是当前代码里真实存在、而且最值得在阅读代码时重点跟的主线。

### 用例 1：自动刷新 feed -> 新文章入库

场景：用户给某个 feed 配了刷新间隔，或者手动触发 `/api/schedulers/auto_refresh/trigger`。

链路：

1. `internal/jobs/auto_refresh.go` 扫描 `refresh_interval > 0` 的 feed
2. 到点 feed 调用 `feed.FeedService.RefreshFeed`
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
4. `content.ContentCompletionService.CompleteArticle` 基于 Firecrawl 正文生成 `ai_content_summary`
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
- 再按用例追具体域：`feed` -> `content` -> `tagging` -> `topicgraph` -> `narrative`
- 叙事域能力可以按以下顺序跟：`narrative/service.go` → `narrative/collector.go` → `narrative/board_creation.go` → `narrative/concept_matcher.go` → `narrative/concept_service.go`
