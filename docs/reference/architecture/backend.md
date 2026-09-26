# 后端架构

## 先说结论

这份文档只描述当前 `backend-go/` 已经落地的真实结构，不再沿用旧的预期分层。

当前后端可以直接按四层理解：

- `cmd/server/`：启动入口
- `internal/app/`：应用装配、路由注册、运行时启动与退出
- `internal/platform/`：数据库、配置、AI 路由、WebSocket、共享基础设施
- 业务域：`internal/reader/`、`internal/tagmanagement/`、`internal/topicgraph/`、`internal/discovery/`、`internal/dataenrichment/`、`internal/datasources/`、`internal/admin/`、`internal/models/`（共享模型白名单）
- 域间依赖只走 root 门面包（`internal/<domain>` 根包 re-export），跨域深路径（handler/service/repository/models）由 `.golangci.yml` depguard 编译期拦截（spec `backend-package-boundaries`）

如果你发现文档和代码不一致，优先相信源码入口：`backend-go/cmd/server/main.go`、`backend-go/internal/app/router.go`、`backend-go/internal/app/runtime.go`。

## 技术栈

- Go 1.25
- Gin
- GORM
- PostgreSQL + pgvector
- Viper
- Gorilla WebSocket
- internal/platform/scheduler（自研调度器工厂 + Interval；域内 job 定义 `job_*.go` 留在各域，admin/scheduler 持有 9+ 个任务编排）

## 实时通信基础设施

### SSE（Server-Sent Events）

项目使用 SSE 推送后台长任务的实时进度。适用场景：单向进度推送（不需要双向通信）。

**后端实现模式（Gin）：**

```go
// Handler 示例
c.Header("Content-Type", "text/event-stream")
c.Header("Cache-Control", "no-cache")
c.Header("Connection", "keep-alive")
ch := getProgressChannel() // 全局单例 channel
c.Stream(func(w io.Writer) bool {
    if msg, ok := <-ch; ok {
        c.SSEvent("progress", msg)
        return true
    }
    return false
})
```

**前端消费模式：**

```ts
const es = new EventSource('/api/topic-tags/merge-preview/scan/stream')
es.onmessage = (e) => { progress.value = JSON.parse(e.data) }
es.onerror = () => es.close() // 扫描完成或出错时自动断开
```

**使用约定：**

- SSE 端点路径：`*/stream`，与触发任务的同级 `POST` 端点配对（如 `POST /scan` + `GET /scan/stream`）
- 消息格式：JSON `{ status, ...progress_fields }`，status 取值 `scanning` / `done` / `error`
- 连接管理：任务完成后服务端关闭 channel，客户端 `onerror` 时清理
- 并发保护：同类型任务同时只允许一个（用 `atomic.Bool` 保护）

**当前使用 SSE 的功能：**

- 标签合并全量扫描进度（`GET /api/topic-tags/merge-preview/scan/stream`）

### WebSocket

项目已有 WebSocket 基础设施（`/ws`），用于推送文章处理等实时通知。SSE 用于单向进度推送场景，WebSocket 用于需要双向通信或广播的场景。

## 当前真实入口

- 服务入口：`backend-go/cmd/server/main.go`
- 路由装配：`backend-go/internal/app/router.go`
- 运行时启动：`backend-go/internal/app/runtime.go`
- 配置加载：`backend-go/internal/platform/config/config.go`
- 数据库初始化与表补丁：`backend-go/internal/platform/database/db.go`
- 配置文件：`backend-go/configs/config.yaml`

## 当前目录现实

```text
backend-go/
├── cmd/
│   └── server/
├── configs/
├── internal/
│   ├── app/                       # 装配层（路由注册、运行时启动，可引任意域）
│   ├── models/                    # 共享 GORM 模型白名单（18 项 + FeedStats 例外，
│   │                              #   唯一权威登记处=.golangci.yml 白名单注释）
│   ├── admin/                     # 管理后台域（AI 运维/scheduler 编排/偏好设置）
│   │   ├── handler/               # AI/scheduler/poll/preferences API
│   │   ├── repository/
│   │   ├── scheduler/             # 域内 job 定义 job_*.go（框架在 platform/scheduler）
│   │   ├── routes.go / wire.go
│   ├── discovery/                 # 订阅发现域（feed-discovery，自 admin 迁出）
│   │   ├── handler/               # /api/discovery/* + 发现相关 settings 路由
│   │   ├── service/               # 候选/召回/推荐/可用性/目录/偏好画像
│   │   ├── models/                # 域独占模型（DiscoveryRun/FeedCandidate 等 12 个）
│   │   ├── routes.go / wire.go    # root 门面（跨域调用唯一入口）
│   ├── reader/                    # 订阅与文章域
│   │   ├── handler/               # feed/article/firecrawl/OPML API
│   │   ├── repository/
│   │   └── service/               # RSS 解析、内容补全、Firecrawl
│   ├── tagmanagement/             # 标签系统域
│   │   ├── handler/               # board/tag/merge/embedding API
│   │   ├── repository/
│   │   ├── models/                # 域独占模型（BoardComposition/EmbeddingConfig 等 11 个）
│   │   └── service/               # core/auxlabel/board/merge/watched/sourcestats
│   ├── topicgraph/                # 主题图谱域
│   │   ├── handler/               # daily_report/watch/margin_notes API
│   │   ├── repository/
│   │   ├── models/                # 域独占模型（TopicAnalysisCursor）
│   │   └── service/
│   ├── dataenrichment/            # 数据增强域（board 分析/信号/论证）
│   ├── datasources/               # 外部数据源域（EIA/JODi/WDI/Comtrade）
│   └── platform/                  # 共享基础设施
│       ├── airouter/              # AI provider/capability/failover 路由
│       ├── aisettings/            # AI/Firecrawl 配置读写
│       ├── scheduler/             # 调度器框架（BaseScheduler/Registry/PauseAware/
│       │                          #   SchedulerTask，自 admin/scheduler 迁出）
│       ├── analysispause/         # 分析暂停总闸
│       ├── aihealth/ articlerefs/ config/ database/ httpclient/ imageproxy/
│       ├── jsonutil/ logging/ middleware/ notification/ safefetch/
│       ├── searxng/ testutil/ textutil/ tracing/ ws/
```

每个业务域统一遵循三层结构：

```
internal/<domain>/
├── routes.go        # 路由注册（由 app/router.go 调用）
├── wire.go          # 单例初始化 + 外部调用 re-export
├── handler/         # Gin handler（package handler）
├── service/         # 业务逻辑（package service）
└── repository/      # 数据访问（package repository）
```

- 根包只放 `routes.go` 和 `wire.go`，不放 handler 文件。
- `wire.go` re-export 外部包需要的类型和函数，调用方只需 import 根包。
- `routes.go` import `handler/` 子包，将路由连接到 handler 函数。

## 分层职责

### `cmd/`

- `server/`：HTTP 服务真实入口

### `internal/app/`

应用壳层，负责把平台能力、业务域和调度器接起来。

- `router.go`：注册 HTTP API 和 WebSocket 路由（调用各域的 `RegisterRoutes`）
- `runtime.go`：启动 scheduler、初始化服务、注册优雅退出

### `internal/platform/`

这是共享基础设施层，不承载具体业务语义。

- `config/`：读取 `configs/config.yaml`
- `database/`：初始化 PostgreSQL、建表、索引、字段补丁
- `logging/`：轻量日志门面，负责把 info/warn 与 error/fatal/panic 分流到 stdout / stderr
- `middleware/`：Gin 中间件，例如 CORS
- `ws/`：WebSocket hub，给前端推送异步任务状态
- `airouter/`：AI provider、capability route、failover 路由
  - capability 绑定：`summary` → 文章自动总结、`digest_polish` → 日报生成、`topic_tagging` → 标签提取、`embedding` → 向量嵌入
  - `article_completion` 已废弃，前端面板不再显示；数据库残留行不影响运行
- `aisettings/`：兼容旧配置表的 AI / Firecrawl 配置读写
- `jsonutil/`：JSON 工具函数
- `testutil/`：测试辅助工具
- `tracing/`：OpenTelemetry tracing

### `internal/platform/scheduler/`

调度器框架（横切基础设施，decouple-backend-domains 迁自 admin/scheduler）：

- `base.go`：`BaseScheduler`、`JobFunc`、`Config`、`JobResult` 类型
- `registry.go`：调度器注册表（`Registry`/`NewRegistry`）
- `pause.go`：`PauseAware`（分析暂停闸门包装）
- `persistence.go`：可选的 `SchedulerTask` DB 状态持久化
- `task.go`：`SchedulerTask` 模型（AutoMigrate 经 `database.RegisterModels` 自注册）

### `internal/admin/`

管理后台域：AI 运维、调度任务编排、偏好设置。

- `handler/`：AI provider 管理、scheduler 状态/手动触发、poll 聚合、comtrade 设置
- `repository/`：管理域数据访问
- `scheduler/`：域内 job 定义 `job_*.go`（auto_refresh/daily_report/firecrawl/discovery_v2 等，
  import `internal/platform/scheduler` 框架；discovery 相关 job 经 discovery root 门面调用）
- `routes.go` + `wire.go`：路由与 re-export（discovery 域路由已迁 `internal/discovery/routes.go`）

### `internal/discovery/`

订阅发现域（feed-discovery，decouple-backend-domains 自 admin 整体迁出；对外 API `/api/discovery/*` 不变）：

- `handler/`：发现/推荐/候选目录/路由参数字典/偏好画像 API（含 `/api/settings` 的 rsshub/proxy/bocha/searxng）
- `service/`：候选召回、精排推荐、可用性检查、RSSHub 目录同步、偏好画像、seed 策略
- `models/`：域独占模型（DiscoveryRun、FeedCandidate、FeedRecommendation、RSSHubRoute、PreferenceVector 等 12 个，AutoMigrate 自注册）
- `routes.go` + `wire.go`：路由注册 + root 门面（admin jobs / app runtime 的唯一跨域入口）

### `internal/reader/`

订阅与文章域：RSS 订阅、文章管理、内容补全、Firecrawl 抓取。

- `handler/`：feed、article、category、content_completion、firecrawl、OPML API
- `service/`：RSS 解析、Feed 管理、内容补全、Firecrawl 服务
- `repository/`：文章/Feed 数据访问、Firecrawl 任务队列

### `internal/tagmanagement/`

标签系统域：主题标签、语义板、辅助标签、合并、embedding。

- `handler/`：board CRUD/match/upgrade、tag 管理、embedding 队列、合并 re-embedding
- `service/core/`：标签提取、co-tag 扩展、元数据标注
- `service/auxlabel/`：辅助标签管理
- `service/board/`：语义板匹配与概念
- `service/merge/`：标签合并
- `service/watched/`：关注标签
- `repository/`：标签域数据访问

### `internal/topicgraph/`

主题图谱域：每日报告。

- `handler/`：daily_report API
- `service/`：日报生成（LLM 调用、匹配、合并）
- `repository/`：图谱域数据访问

### `internal/models/`

共享 GORM 模型和部分格式化 helper。所有域共用。

## 当前主要子系统

### 订阅与文章（`reader` 域）

`reader` 域负责 RSS 订阅、文章管理、内容补全和 Firecrawl 抓取。

- Feed 刷新负责拉 RSS、去重、入库 Article
- Article 记录承接后续 Firecrawl、内容补全、摘要、标签分析
- Feed 上的 `firecrawl_enabled`、`article_summary_enabled` 会直接影响文章入库后的状态初始化
- Firecrawl 抓取正文 → 内容补全生成文章摘要

### AI 与管理后台（`admin` 域）

`admin` 域负责 AI 配置管理和后台调度任务。

- `platform/airouter`：管理 provider 和 capability route
- 调度器采用 `BaseScheduler` + `JobFunc` 工厂模式：新增调度任务只需写一个 `JobFunc` 函数 + 一行注册
- 9 个调度任务：`auto_refresh`、`aux_label_cleanup`、`blocked_article_recovery`、`content_completion`、`daily_report`、`firecrawl`、`log_cleanup`、`preference_update`、`tag_quality_score`

### 标签系统（`tagmanagement` 域）

标签能力统一在 `tagmanagement` 域下：

- `service/core/`：主题标签提取、co-tag 扩展、元数据标注
- `service/auxlabel/`：辅助标签管理与 GC
- `service/board/`：语义板匹配与概念
- `service/merge/`：标签合并
- `service/watched/`：关注标签管理

### 主题图谱（`topicgraph` 域）

每日报告生成 + PersistentTopic 生命周期管理。PersistentTopic 身份系统（candidate / active / archived 叙事状态）独立于规划中的 topic-watch 注意力系统——身份系统表示叙事演进阶段，watch 表示用户主动关注，本 change 只做身份侧。

- `tagging/analysis`：生成并查询 topic analysis，同时承担 embedding 向量化、Tag 合并（源 DELETE）、辅助标签入库（L1/L2/L3 三级匹配）
- `tagging/watched`：关注标签管理

#### 辅助标签入库三级匹配（`tagmanagement/service/auxlabel`）

`findOrCreateTag` 创建 Tag 后，辅助标签通过 `auxiliary_label_service.go` 入库：

1. **L1 slug/alias 精确匹配**：slug 或 aliases 中包含新标签 → 复用已有 auxiliary label，ref_count++
2. **L2 embedding ≥0.95 合并**：pgvector 余弦相似度 ≥0.95 → 合并到 ref_count 更大的一方，小方 label 加入大方 aliases
3. **L3 新建**：无匹配 → 创建 semantic_label(label_type=auxiliary) + 生成 embedding

禁用标签 (status=disabled) 不参与 L1/L2 匹配。

#### 标签创建流程（`tagmanagement/service/core/findOrCreateTag`）

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

#### Tag 合并（`tagmanagement/service/merge`）

合并相似 Tag 时采用硬删除策略，不再使用 `status='merged'` 或 `status='inactive'`：

- `HardMergeTags(sourceID, targetID)` 迁移 article_topic_tags → 迁移 topic_tag_relations (children) → DELETE topic_tag_embeddings → DELETE topic_tags 源行
- `tag_quality_score` 调度器定期重算标签质量分

#### SemanticBoard 匹配（`tagmanagement/service/board`）

Tag 入库后，`semantic_board_matching.go` 执行 Tag → SemanticBoard 匹配：

1. 读取 tag 的辅助标签和 active SemanticBoard composition
2. 直接命中：tag 的辅助标签 ∈ board 构成标签 → 直接挂载
3. 命中率 > 50% → 直接挂载
4. max_sim ≥ 0.8 → 直接挂载
5. 加权综合：0.6×max_sim + 0.4×hit_rate ≥ 阈值 → 挂载

默认最多 3 个 board，按匹配分排序。匹配结果写入 `topic_tag_board_labels`。

冷启动无 SemanticBoard 时不匹配、不报错。

#### SemanticBoard 升级建议（`tagmanagement/service/board`）

`semantic_board_upgrade.go` 实现两阶段升级：

1. 收集 ref_count ≥ 阈值的候选辅助标签
2. embedding 预聚类（average-link greedy，默认 cosine 距离阈值 0.35；可通过 `cluster_method` 配置切换回 centroid 模式）
3. 每簇补充 co-tag 事件上下文
4. LLM 判断：create_new / skip
5. 用户确认执行（支持前端 merge_into_existing 操作）

#### 回填队列（`tagmanagement/service/board`）

`semantic_board_backfill.go` 支持三种回填模式：

- all：所有 tag 重新匹配
- unassigned：只处理无归属的 tag
- board：只处理指定 board 的 tag

异步逐个执行，幂等覆盖。

依赖方向大致是：

```text
tagmanagement (根包，routes + wire)
    ↑
    ├── topicgraph
    ├── service/board (SemanticBoard 匹配、升级、回填)
    ├── service/auxlabel (辅助标签 GC、L1/L2/L3 匹配)
    ├── service/merge (Tag 合并)
    ├── service/watched (关注标签)
    └── service/core (标签提取、co-tag 扩展)
```

### 叙事摘要（`topicgraph` 域）

叙事摘要基于 SemanticBoard 派生每日 NarrativeBoard。核心概念不变，相关代码位于 `topicgraph` 域。

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

#### 叙事域文件清单（`topicgraph/service/`）

```
topicgraph/service/
├── daily_report_llm.go       # AI 叙事生成
├── daily_report_merge.go     # Board 合并
├── daily_report_matching.go  # 数据采集与匹配
├── graph_service.go          # 图谱查询
└── *_test.go                 # 测试
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

#### Feed 图标状态机（`icon` + `icon_source`）

`icon` 只承载值（iconify id 或图片 URL），`icon_source` 承载来源语义，三态流转：

```
fallback ──refresh──▶ auto      系统抓 RSS <image> / 站点 /favicon.ico
   │                    │
   └── 用户编辑 ──▶ custom ★冻结 永不被 RefreshFeed 覆盖
```

- **`fallback`**：系统兜底（`mdi:rss`），RefreshFeed 会尝试升级到 auto
- **`auto`**：系统自动抓取的 URL，可被刷新换更优图
- **`custom`**：用户显式设定，RefreshFeed 永不覆盖（硬契约）

favicon 获取走 RSS channel link（`parsed.Link`，站点首页）的 host 拼 `/favicon.ico`，**不依赖 Google s2**（国内被墙）。`resolveFeedIcon` 纯函数承载状态机逻辑（`feed_service.go`），便于单测。

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
- `/api/import-opml` / `/api/export-opml`
- `/ws`

AI 管理则已经扩展到 provider 和 route 级别，而不是只有"摘要设置"一个入口。

此外还有以下独立注册的路由组：

- `/api/topic-tags`：关注标签、标签合并预览（由 `tagmanagement` 域注册）
- `/api/embedding`：embedding 配置与队列管理（由 `tagmanagement` 域注册）
- ~~`/api/narratives`~~（**已废弃**）：narrative 生成管线下线，生成能力并入 `daily_report`；历史 `narrative_summaries`/`narrative_boards` 数据只读，经 `/api/semantic-boards/:id/narratives` 访问
- `/api/semantic-boards`：SemanticBoard CRUD、升级建议、回填、匹配配置
- `/api/auxiliary-labels`：辅助标签池查询和治理
- `/api/tags/:id/auxiliary-labels`：tag 辅助标签查询
- `/api/tags/:id/semantic-boards`：tag 所属 SemanticBoard 查询
- `/icons`：本地化 feed 图标的静态服务（存 `storage.icon_dir`，默认 `data/icons`；带 `X-Content-Type-Options: nosniff` + `Content-Security-Policy: sandbox` 响应头）

## 具体数据链路示例

> 逐条业务链路的详细设计已迁至 [`flow/`](../flow/README.md)。本节给追代码时的索引：

| 链路 | flow 文档 | 代码入口 |
| ------ | ---------- | ---------- |
| 自动刷新 feed → 新文章入库（状态位预埋） | [`flow/scheduler.md`](../flow/scheduler.md) | `admin/scheduler/job_auto_refresh.go` → `reader/service.FeedService.RefreshFeed` → `buildArticleFromEntry` |
| Firecrawl 抓正文 → 内容补全生成摘要 | [`flow/content-enrichment.md`](../flow/content-enrichment.md) | `job_firecrawl.go` → `job_content_completion.go` → `ContentCompletionService.CompleteArticle` |
| Article 打标签时机 / 正文提取优先级 | [`flow/reading.md`](../flow/reading.md) | `tag_jobs` 队列 → `TagQueue` worker；手动 `POST /api/articles/:id/tags` |

## 当前边界上的已知问题

- `models/` 仍是共享模型桶，后续可以继续收敛 ownership
- `aisettings` 同时承担兼容旧配置和新配置落库，职责偏宽
- 部分域仍使用全局单例（`database.DB`、`repository.Repo`）和函数变量桥接（`wire.go` 中的 re-export）
- `scheduler/` 包缺少单元测试（工厂模式迁移时旧测试被删除）

## 推荐阅读顺序

- 先看 `docs/reference/architecture/runtime.md`
- 再看 `backend-go/cmd/server/main.go`
- 再看 `backend-go/internal/app/router.go`
- 再看 `backend-go/internal/app/runtime.go`（调度器注册逻辑）
- 再按域追具体包：`reader` → `admin` → `tagmanagement` → `topicgraph`
- 每个域的入口都是 `routes.go` → `handler/` → `service/` → `repository/`
