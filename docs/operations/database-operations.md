<<<<<<< Updated upstream:docs/operations/database.md
# 数据库说明

## 当前数据库

主分支仅支持 PostgreSQL 数据库驱动。SQLite 驱动已归档到 `sqlite` 独立分支，主分支不再维护。

| 驱动 | 用途 | 默认连接 |
|------|------|----------|
| `postgres` | 生产/开发使用，支持 pgvector 向量检索 | `host=postgres user=postgres password=postgres dbname=syntopica port=5432 sslmode=disable TimeZone=Asia/Shanghai` |

## 初始化方式

后端启动时自动执行版本化迁移。核心逻辑在：

- `backend-go/internal/platform/database/db.go` — 入口，连接 PostgreSQL 并执行迁移
- `backend-go/internal/platform/database/migrator.go` — 版本化迁移框架（`schema_migrations` 追踪表）
- `backend-go/internal/platform/database/postgres_migrations.go` — PostgreSQL 迁移定义
- `backend-go/internal/platform/database/bootstrap_postgres.go` — Postgres schema 引导：AutoMigrate + 索引
- `backend-go/internal/platform/database/connect_postgres.go` — PostgreSQL 连接与连接池配置

> 从 SQLite 迁移到 PostgreSQL 的操作步骤见 [PostgreSQL 迁移操作手册](./postgres-migration.md)。

## 迁移版本记录

| 序号 | 版本号 | 说明 |
|------|--------|------|
| 1 | `20260403_0001` | 启用 pgvector 扩展（`CREATE EXTENSION IF NOT EXISTS vector`） |
| 2 | `20260403_0002` | 创建全部基础表结构（GORM AutoMigrate 21 个模型表 + 列类型调整 + 10 个性能索引） |
| 3 | `20260403_0003` | 为 `topic_tag_embeddings` 表添加 `embedding vector(1536)` 列 |
| 4 | `20260413_0001` | 为 `topic_tag_embeddings.embedding` 创建 HNSW 向量索引 |
| 5 | `20260413_0002` | 创建 `embedding_config` 表并写入默认配置 |
| 6 | `20260413_0003` | 为 `topic_tags` 增加 `status`、`merged_into_id` 字段与索引 |
| 7 | `20260413_0004` | 创建 `embedding_queues` 表 |
| 8 | `20260413_0005` | 创建 `merge_reembedding_queues` 表 |
| 9 | `20260414_0001` | 为 `topic_tags` 增加 `description` 字段 |
| 10 | `20260414_0002` | 创建 `topic_tag_relations` 表 |
| 11 | `20260414_0003` | 为 `articles` 增加 `feed_summary_id`、`feed_summary_generated_at` 及对应索引 |
| 12 | `20260415_0001` | 为 `topic_tags` 增加 `is_watched`、`watched_at` 字段 |

迁移记录写入 `schema_migrations` 表，每个版本只会执行一次。

## 当前核心表（共 29 张）

### 数据核心表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `categories` | 分类 | `models.Category` |
| `feeds` | 订阅源 | `models.Feed` |
| `articles` | 文章 | `models.Article` |
| `article_topic_tags` | 文章-主题关联 | `models.ArticleTopicTag` |

### 调度与配置表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `scheduler_tasks` | 调度任务状态 | `models.SchedulerTask` |
| `ai_settings` | AI 配置（键值对） | `models.AISettings` |
| `ai_providers` | AI 供应商 | `models.AIProvider` |
| `ai_routes` | AI 路由 | `models.AIRoute` |
| `ai_route_providers` | AI 路由-供应商绑定 | `models.AIRouteProvider` |
| `ai_call_logs` | AI 调用日志 | `models.AICallLog` |
| `embedding_config` | 向量配置 | `models.EmbeddingConfig` |

### 用户行为表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `reading_behaviors` | 阅读行为 | `models.ReadingBehavior` |
| `user_preferences` | 用户偏好 | `models.UserPreference` |

### 主题标签表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `topic_tags` | 主题标签主表 | `models.TopicTag` |
| `topic_tag_embeddings` | 主题标签向量 | `models.TopicTagEmbedding` |
| `topic_tag_analyses` | 主题分析快照 | `models.TopicTagAnalysis` |
| `topic_analysis_cursors` | 主题分析游标 | `models.TopicAnalysisCursor` |
| `topic_analysis_jobs` | 主题分析任务队列 | `topicanalysis.topicAnalysisJobRecord` |
| `topic_tag_relations` | 主题标签层级关系 | `models.TopicTagRelation` |
| `narrative_summaries` | 叙事摘要 | `models.NarrativeSummary` |

### 任务队列表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `firecrawl_jobs` | Firecrawl 抓取任务 | `models.FirecrawlJob` |
| `tag_jobs` | 标签任务 | `models.TagJob` |
| `embedding_queues` | 向量生成队列 | `models.EmbeddingQueue` |
| `merge_reembedding_queues` | 合并后重算向量队列 | `models.MergeReembeddingQueue` |

### 系统表

| 表名 | 说明 |
|------|------|
| `schema_migrations` | 迁移版本追踪 |

## Topic 相关表说明

数据库里以 `topic_` 开头的表，主要服务于 Topic Graph、热点标签、主题分析这条链路。它们不是孤立功能，而是围绕 `topic_tags` 这份主题主数据逐层展开。

### `topic_tags`

- 主题标签主表，存放系统里所有主题实体
- 一条记录代表一个可复用的 topic/tag，核心字段包括：
  - `slug`：稳定标识，供接口和关联表引用
  - `label`：展示名称
  - `category`：标签分类，当前主要是 `event`、`person`、`keyword`
  - `aliases`：别名列表，便于复用已有标签
  - `description`：LLM 生成的标签描述
  - `is_canonical`：是否为规范标签
  - `source`：标签来源，如 `llm`、`heuristic`、`manual`
  - `status`：标签状态（`active`/`merged`）
  - `merged_into_id`：合并目标标签 ID
  - `is_watched`：是否为用户关注标签
  - `quality_score`：质量评分
- 主要职责：
  - 为 `article_topic_tags` 提供统一的标签字典
  - 作为 Topic Graph 节点和热点标签列表的数据来源
  - 作为 topic analysis、embedding 的主键锚点

### `topic_tag_embeddings`

- `topic_tags` 的向量扩展表，按 `topic_tag_id` 一对一保存 embedding
- 主要字段：
  - `vector`：旧版 JSON 文本向量（已废弃，保留兼容）
  - `embedding`：pgvector `vector(1536)` 列（当前主用）
  - `dimension`：向量维度
  - `model`：生成 embedding 的模型
  - `text_hash`：由标签文本生成的哈希，用于判断是否需要重算
- 带 HNSW 向量索引，支持快速余弦相似度搜索
- 主要职责：
  - 在打标签时做相似标签匹配，尽量复用已有 topic
  - 支撑"高相似直接复用 / 低相似新建 / 中间区间再判断"的匹配策略

### `topic_tag_analyses`

- 主题分析结果快照表，保存某个 topic 在某个时间窗上的分析结果
- 唯一键是：`topic_tag_id + analysis_type + window_type + anchor_date`
- 主要字段：
  - `analysis_type`：分析类型，例如事件、人物、关键词视角
  - `window_type`：时间窗，如 `daily`、`weekly`
  - `anchor_date`：锚点日期
  - `summary_count`：本次分析覆盖的摘要数量
  - `payload_json`：分析结果 JSON
  - `source`：结果来源，可能是 `ai` 或 `heuristic`
  - `version`：分析版本号

### `topic_analysis_cursors`

- topic analysis 的增量更新游标表
- 唯一键是：`topic_tag_id + analysis_type + window_type`
- 主要字段：
  - `last_summary_id`：上次分析已处理到的最新 summary ID
  - `last_updated_at`：上次刷新时间

### `topic_analysis_jobs`

- 主题分析任务队列表的持久化镜像
- 这张表不是最终分析结果表，而是运行时 job 状态表
- 主键是字符串 ID，不是自增序列

### `topic_tag_relations`

- 主题标签层级关系表，记录抽象标签与子标签的映射
- 唯一键是：`parent_id + child_id`
- `relation_type`：关系类型（`abstract`/`synonym`/`related`）

### `narrative_summaries`

- 叙事摘要表，记录事件线索的演进过程
- 状态包括：`emerging`（新兴）、`continuing`（持续）、`splitting`（分裂）、`merging`（合并）、`ending`（结束）

## 与 Topic 相关但不以 `topic_` 开头的表

### `article_topic_tags`

- 文章与 topic 的关联表，也是当前文章标签的事实来源
- Topic Graph 的图节点、热点列表、digest 聚合标签、文章详情 tags，当前都直接或间接依赖这张表

### `embedding_config`

- 向量系统的配置键值对表，存储相似度阈值、模型参数等

### `embedding_queues`

- 向量生成任务队列，记录待生成 embedding 的标签

### `merge_reembedding_queues`

- 标签合并后的向量重算队列，记录源标签和目标标签

## 数据链路

1. 文章或摘要被打标签，先写入 `topic_tags`
2. 文章标签关系写入 `article_topic_tags`
3. 若启用 embedding，相似性结果写入或读取 `topic_tag_embeddings`
4. Topic Graph 页面主要消费 `topic_tags + article_topic_tags`
5. Topic Analysis 则基于 `article_topic_tags` 生成结果，落到 `topic_tag_analyses`
6. 增量刷新状态记录在 `topic_analysis_cursors`
7. 标签层级关系记录在 `topic_tag_relations`
8. 叙事演进记录在 `narrative_summaries`

## Topic Analysis 详细链路

### 入口：前端先查结果，不是先入队

1. Topic Graph 页面打开某个 topic
2. 前端先请求 `/api/topic-graph/analysis`
3. 后端查询 `topic_tag_analyses`
4. 如果查到了快照，直接返回 `payload_json`
5. 如果没查到，前端再请求 `/api/topic-graph/analysis/status`
6. 若状态是 `missing`，前端会调用 `/api/topic-graph/analysis/rebuild` 主动入队

### 入队：创建 `topic_analysis_jobs`

当用户触发 rebuild 后：

1. 创建一个 `AnalysisJob`，job 的去重键是 `topic_tag_id + analysis_type + window_type + anchor_date`
2. 同一组键如果已有 pending/processing job，不会重复插入
3. 若使用内存队列，job 会同步保存到 `topic_analysis_jobs` 表
4. 若配置了 `REDIS_URL`，job 改存 Redis

### 执行与构建

1. Worker 取出优先级最高的 job
2. 通过 `article_topic_tags` 反查关联文章
3. 检查 `topic_analysis_cursors` 判断是否需要重算
4. 需要重算时调用 AI 生成分析，否则复用旧快照
5. 结果写入 `topic_tag_analyses`

### 前端状态感知

1. 先拉 `/api/topic-graph/analysis`
2. 没数据时再拉 `/api/topic-graph/analysis/status`
3. 如果状态是 `pending/processing`，前端每约 1.8 秒轮询
4. 如果状态是 `missing`，前端调用 `rebuild`
5. 如果状态变成 `ready`，前端再次拉取正式分析结果

## 当前 schema 特点

项目使用版本化迁移框架，流程：

1. 启动时初始化数据库连接
2. `ensureSchemaMigrationsTable()` 创建 `schema_migrations` 追踪表
3. 按版本号顺序执行 `postgres_migrations.go` 中的迁移
4. 每个迁移在事务中执行，完成后记录版本号
5. 迁移完成后，`digest.Migrate()` 补充 digest 相关表

数据库演进是"GORM AutoMigrate + 手写 SQL 迁移 + 独立子系统迁移"三种方式并存。

## 常用命令

```bash
cd backend-go
go run cmd/server/main.go            # 启动后端（自动执行迁移）
go run cmd/migrate-tags/main.go       # 标签数据迁移
go run cmd/migrate-db/main.go         # SQLite → PostgreSQL 数据迁移
```

## 说明

- 当前项目以 Go 后端为主
- 文档以当前 checkout 里的真实数据库逻辑为准
- 所有表的详细字段说明见 [DATABASE_FIELDS.md](../reference/database/DATABASE_FIELDS.md)
=======
# 数据库说明

## 当前数据库

主分支仅支持 PostgreSQL 数据库驱动。SQLite 驱动已归档到 `sqlite` 独立分支，主分支不再维护。

| 驱动 | 用途 | 默认连接 |
|------|------|----------|
| `postgres` | 生产/开发使用，支持 pgvector 向量检索 | `host=postgres user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai` |

## 初始化方式

后端启动时自动执行版本化迁移。核心逻辑在：

- `backend-go/internal/platform/database/db.go` — 入口，连接 PostgreSQL 并执行迁移
- `backend-go/internal/platform/database/migrator.go` — 版本化迁移框架（`schema_migrations` 追踪表）
- `backend-go/internal/platform/database/postgres_migrations.go` — PostgreSQL 迁移定义
- `backend-go/internal/platform/database/bootstrap_postgres.go` — Postgres schema 引导：AutoMigrate + 索引
- `backend-go/internal/platform/database/connect_postgres.go` — PostgreSQL 连接与连接池配置

> 从 SQLite 迁移到 PostgreSQL 的操作步骤见 [PostgreSQL 迁移操作手册](./postgres-migration.md)。

## 迁移版本记录

| 序号 | 版本号 | 说明 |
|------|--------|------|
| 1 | `20260403_0001` | 启用 pgvector 扩展（`CREATE EXTENSION IF NOT EXISTS vector`） |
| 2 | `20260403_0002` | 创建全部基础表结构（GORM AutoMigrate 21 个模型表 + 列类型调整 + 10 个性能索引） |
| 3 | `20260403_0003` | 为 `topic_tag_embeddings` 表添加 `embedding vector(1536)` 列 |
| 4 | `20260413_0001` | 为 `topic_tag_embeddings.embedding` 创建 HNSW 向量索引 |
| 5 | `20260413_0002` | 创建 `embedding_config` 表并写入默认配置 |
| 6 | `20260413_0003` | 为 `topic_tags` 增加 `status`、`merged_into_id` 字段与索引 |
| 7 | `20260413_0004` | 创建 `embedding_queues` 表 |
| 8 | `20260413_0005` | 创建 `merge_reembedding_queues` 表 |
| 9 | `20260414_0001` | 为 `topic_tags` 增加 `description` 字段 |
| 10 | `20260414_0002` | 创建 `topic_tag_relations` 表 |
| 11 | `20260414_0003` | 为 `articles` 增加 `feed_summary_id`、`feed_summary_generated_at` 及对应索引 |
| 12 | `20260415_0001` | 为 `topic_tags` 增加 `is_watched`、`watched_at` 字段 |

迁移记录写入 `schema_migrations` 表，每个版本只会执行一次。

## 当前核心表（共 29 张）

### 数据核心表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `categories` | 分类 | `models.Category` |
| `feeds` | 订阅源 | `models.Feed` |
| `articles` | 文章 | `models.Article` |
| `article_topic_tags` | 文章-主题关联 | `models.ArticleTopicTag` |

### 调度与配置表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `scheduler_tasks` | 调度任务状态 | `models.SchedulerTask` |
| `ai_settings` | AI 配置（键值对） | `models.AISettings` |
| `ai_providers` | AI 供应商 | `models.AIProvider` |
| `ai_routes` | AI 路由 | `models.AIRoute` |
| `ai_route_providers` | AI 路由-供应商绑定 | `models.AIRouteProvider` |
| `ai_call_logs` | AI 调用日志 | `models.AICallLog` |
| `embedding_config` | 向量配置 | `models.EmbeddingConfig` |

### 用户行为表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `reading_behaviors` | 阅读行为 | `models.ReadingBehavior` |
| `user_preferences` | 用户偏好 | `models.UserPreference` |

### 主题标签表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `topic_tags` | 主题标签主表 | `models.TopicTag` |
| `topic_tag_embeddings` | 主题标签向量 | `models.TopicTagEmbedding` |
| `topic_tag_analyses` | 主题分析快照 | `models.TopicTagAnalysis` |
| `topic_analysis_cursors` | 主题分析游标 | `models.TopicAnalysisCursor` |
| `topic_analysis_jobs` | 主题分析任务队列 | `topicanalysis.topicAnalysisJobRecord` |
| `topic_tag_relations` | 主题标签层级关系 | `models.TopicTagRelation` |
| `narrative_summaries` | 叙事摘要 | `models.NarrativeSummary` |

### 任务队列表

| 表名 | 说明 | 模型位置 |
|------|------|----------|
| `firecrawl_jobs` | Firecrawl 抓取任务 | `models.FirecrawlJob` |
| `tag_jobs` | 标签任务 | `models.TagJob` |
| `embedding_queues` | 向量生成队列 | `models.EmbeddingQueue` |
| `merge_reembedding_queues` | 合并后重算向量队列 | `models.MergeReembeddingQueue` |

### 系统表

| 表名 | 说明 |
|------|------|
| `schema_migrations` | 迁移版本追踪 |

## Topic 相关表说明

数据库里以 `topic_` 开头的表，主要服务于 Topic Graph、热点标签、主题分析这条链路。它们不是孤立功能，而是围绕 `topic_tags` 这份主题主数据逐层展开。

### `topic_tags`

- 主题标签主表，存放系统里所有主题实体
- 一条记录代表一个可复用的 topic/tag，核心字段包括：
  - `slug`：稳定标识，供接口和关联表引用
  - `label`：展示名称
  - `category`：标签分类，当前主要是 `event`、`person`、`keyword`
  - `aliases`：别名列表，便于复用已有标签
  - `description`：LLM 生成的标签描述
  - `is_canonical`：是否为规范标签
  - `source`：标签来源，如 `llm`、`heuristic`、`manual`
  - `status`：标签状态（`active`/`merged`）
  - `merged_into_id`：合并目标标签 ID
  - `is_watched`：是否为用户关注标签
  - `quality_score`：质量评分
- 主要职责：
  - 为 `article_topic_tags` 提供统一的标签字典
  - 作为 Topic Graph 节点和热点标签列表的数据来源
  - 作为 topic analysis、embedding 的主键锚点

### `topic_tag_embeddings`

- `topic_tags` 的向量扩展表，按 `topic_tag_id` 一对一保存 embedding
- 主要字段：
  - `vector`：旧版 JSON 文本向量（已废弃，保留兼容）
  - `embedding`：pgvector `vector(1536)` 列（当前主用）
  - `dimension`：向量维度
  - `model`：生成 embedding 的模型
  - `text_hash`：由标签文本生成的哈希，用于判断是否需要重算
- 带 HNSW 向量索引，支持快速余弦相似度搜索
- 主要职责：
  - 在打标签时做相似标签匹配，尽量复用已有 topic
  - 支撑"高相似直接复用 / 低相似新建 / 中间区间再判断"的匹配策略

### `topic_tag_analyses`

- 主题分析结果快照表，保存某个 topic 在某个时间窗上的分析结果
- 唯一键是：`topic_tag_id + analysis_type + window_type + anchor_date`
- 主要字段：
  - `analysis_type`：分析类型，例如事件、人物、关键词视角
  - `window_type`：时间窗，如 `daily`、`weekly`
  - `anchor_date`：锚点日期
  - `summary_count`：本次分析覆盖的摘要数量
  - `payload_json`：分析结果 JSON
  - `source`：结果来源，可能是 `ai` 或 `heuristic`
  - `version`：分析版本号

### `topic_analysis_cursors`

- topic analysis 的增量更新游标表
- 唯一键是：`topic_tag_id + analysis_type + window_type`
- 主要字段：
  - `last_summary_id`：上次分析已处理到的最新 summary ID
  - `last_updated_at`：上次刷新时间

### `topic_analysis_jobs`

- 主题分析任务队列表的持久化镜像
- 这张表不是最终分析结果表，而是运行时 job 状态表
- 主键是字符串 ID，不是自增序列

### `topic_tag_relations`

- 主题标签层级关系表，记录抽象标签与子标签的映射
- 唯一键是：`parent_id + child_id`
- `relation_type`：关系类型（`abstract`/`synonym`/`related`）

### `narrative_summaries`

- 叙事摘要表，记录事件线索的演进过程
- 状态包括：`emerging`（新兴）、`continuing`（持续）、`splitting`（分裂）、`merging`（合并）、`ending`（结束）

## 与 Topic 相关但不以 `topic_` 开头的表

### `article_topic_tags`

- 文章与 topic 的关联表，也是当前文章标签的事实来源
- Topic Graph 的图节点、热点列表、digest 聚合标签、文章详情 tags，当前都直接或间接依赖这张表

### `embedding_config`

- 向量系统的配置键值对表，存储相似度阈值、模型参数等

### `embedding_queues`

- 向量生成任务队列，记录待生成 embedding 的标签

### `merge_reembedding_queues`

- 标签合并后的向量重算队列，记录源标签和目标标签

## 数据链路

1. 文章或摘要被打标签，先写入 `topic_tags`
2. 文章标签关系写入 `article_topic_tags`
3. 若启用 embedding，相似性结果写入或读取 `topic_tag_embeddings`
4. Topic Graph 页面主要消费 `topic_tags + article_topic_tags`
5. Topic Analysis 则基于 `article_topic_tags` 生成结果，落到 `topic_tag_analyses`
6. 增量刷新状态记录在 `topic_analysis_cursors`
7. 标签层级关系记录在 `topic_tag_relations`
8. 叙事演进记录在 `narrative_summaries`

## Topic Analysis 详细链路

### 入口：前端先查结果，不是先入队

1. Topic Graph 页面打开某个 topic
2. 前端先请求 `/api/topic-graph/analysis`
3. 后端查询 `topic_tag_analyses`
4. 如果查到了快照，直接返回 `payload_json`
5. 如果没查到，前端再请求 `/api/topic-graph/analysis/status`
6. 若状态是 `missing`，前端会调用 `/api/topic-graph/analysis/rebuild` 主动入队

### 入队：创建 `topic_analysis_jobs`

当用户触发 rebuild 后：

1. 创建一个 `AnalysisJob`，job 的去重键是 `topic_tag_id + analysis_type + window_type + anchor_date`
2. 同一组键如果已有 pending/processing job，不会重复插入
3. 若使用内存队列，job 会同步保存到 `topic_analysis_jobs` 表
4. 若配置了 `REDIS_URL`，job 改存 Redis

### 执行与构建

1. Worker 取出优先级最高的 job
2. 通过 `article_topic_tags` 反查关联文章
3. 检查 `topic_analysis_cursors` 判断是否需要重算
4. 需要重算时调用 AI 生成分析，否则复用旧快照
5. 结果写入 `topic_tag_analyses`

### 前端状态感知

1. 先拉 `/api/topic-graph/analysis`
2. 没数据时再拉 `/api/topic-graph/analysis/status`
3. 如果状态是 `pending/processing`，前端每约 1.8 秒轮询
4. 如果状态是 `missing`，前端调用 `rebuild`
5. 如果状态变成 `ready`，前端再次拉取正式分析结果

## 当前 schema 特点

项目使用版本化迁移框架，流程：

1. 启动时初始化数据库连接
2. `ensureSchemaMigrationsTable()` 创建 `schema_migrations` 追踪表
3. 按版本号顺序执行 `postgres_migrations.go` 中的迁移
4. 每个迁移在事务中执行，完成后记录版本号
5. 迁移完成后，`digest.Migrate()` 补充 digest 相关表

数据库演进是"GORM AutoMigrate + 手写 SQL 迁移 + 独立子系统迁移"三种方式并存。

## 常用命令

```bash
cd backend-go
go run cmd/server/main.go            # 启动后端（自动执行迁移）
go run cmd/migrate-tags/main.go       # 标签数据迁移
go run cmd/migrate-db/main.go         # SQLite → PostgreSQL 数据迁移
```

## 说明

- 当前项目以 Go 后端为主
- 文档以当前 checkout 里的真实数据库逻辑为准
- 所有表的详细字段说明见 [DATABASE_FIELDS.md](../database/DATABASE_FIELDS.md)
---
# PostgreSQL 迁移操作手册

从 SQLite 迁移到 PostgreSQL + pgvector 的完整操作指南。

## 前置条件

- Docker & Docker Compose
- Go 1.22+
- 现有 SQLite 数据库文件（如 `backend-go/rss_reader.db`）

## 架构概述

迁移采用**停机一次性切换**，不做长期双写：

1. Docker Compose 启动 PostgreSQL + pgvector 容器
2. Go 后端首次连接时自动创建全部表结构（版本化迁移）
3. 运行数据迁移工具，将 SQLite 数据导入 PostgreSQL
4. 验证数据完整性后，日常运行切到 PostgreSQL

## 第一步：启动 PostgreSQL 容器

```bash
docker compose up -d
```

容器启动后：

- PostgreSQL 监听 `localhost:5432`（可通过 `POSTGRES_PORT` 环境变量覆盖）
- 默认数据库名 `rss_reader`，用户 `postgres`，密码 `postgres`
- 首次初始化时自动执行 `docker/postgres/init/01-enable-pgvector.sql`，启用 pgvector 扩展
- 数据持久化在 `./data` 目录

确认容器健康：

```bash
docker compose ps
# 状态应为 healthy
```

也可手动连接确认：

```bash
docker exec -it zanebono-rssreader-pgvector psql -U postgres -d rss_reader -c "SELECT extname FROM pg_extension WHERE extname = 'vector';"
```

## 第二步：配置后端连接

编辑 `backend-go/configs/config.yaml`：

```yaml
database:
  driver: "postgres"
  dsn: "host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai"
```

或通过环境变量覆盖（不修改配置文件）：

```bash
export DATABASE_DRIVER=postgres
export DATABASE_DSN="host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable TimeZone=Asia/Shanghai"
```

## 第三步：启动后端，自动建表

```bash
cd backend-go
go run cmd/server/main.go
```

首次连接 PostgreSQL 时，后端自动执行**版本化迁移**，顺序如下：

| 序号 | 版本号 | 说明 |
|------|--------|------|
| 1 | `20260403_0001` | 启用 pgvector 扩展（`CREATE EXTENSION IF NOT EXISTS vector`） |
| 2 | `20260403_0002` | 创建全部基础表结构（GORM AutoMigrate 21 个模型表 + 列类型调整 + 10 个性能索引） |
| 3 | `20260403_0003` | 为 `topic_tag_embeddings` 表添加 `embedding vector(1536)` 列 |
| 4 | `20260413_0001` | 为 `topic_tag_embeddings.embedding` 创建 HNSW 向量索引 |
| 5 | `20260413_0002` | 创建 `embedding_config` 表并写入默认配置 |
| 6 | `20260413_0003` | 为 `topic_tags` 增加 `status`、`merged_into_id` 字段与索引 |
| 7 | `20260413_0004` | 创建 `embedding_queues` 表 |
| 8 | `20260413_0005` | 创建 `merge_reembedding_queues` 表 |
| 9 | `20260414_0001` | 为 `topic_tags` 增加 `description` 字段 |
| 10 | `20260414_0002` | 创建 `topic_tag_relations` 表 |
| 11 | `20260414_0003` | 为 `articles` 增加 `feed_summary_id`、`feed_summary_generated_at` 及对应索引 |
| 12 | `20260415_0001` | 为 `topic_tags` 增加 `is_watched`、`watched_at` 字段 |

迁移记录写入 `schema_migrations` 表，每个版本只会执行一次。

迁移完成后，还会依次执行：

- `digest.Migrate()` — 创建 `digest_configs` 表并插入默认配置
- `airouter.EnsureLegacySummaryConfigMigrated()` — 将旧版 `ai_settings` 键值对迁移到新的 AI Provider/Route 表
- `tracing.InitTracerProvider()` — 创建 `otel_spans` 追踪表

启动后观察日志，确认无报错即可。此时表结构已就绪，但表中无数据。

> **注意：** 如果是全新部署（无历史数据需要迁移），到这一步就完成了，可以跳过第四步和第五步。

## 第四步：执行数据迁移

停掉后端服务（Ctrl+C），然后运行数据迁移工具。

### 4.1 预检（Dry Run）

检查 SQLite 源数据概况，不写入任何数据：

```bash
cd backend-go
go run cmd/migrate-db/main.go \
  --mode dry-run \
  --sqlite-path ./rss_reader.db \
  --postgres-dsn "host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable"
```

输出示例：

```
Mode: dry-run
Import order:
- categories: 5 rows
- feeds: 12 rows
- articles: 2340 rows
- ai_summaries: 180 rows
...
```

确认表和行数符合预期。

### 4.2 执行导入

```bash
go run cmd/migrate-db/main.go \
  --mode execute \
  --force \
  --sqlite-path ./rss_reader.db \
  --postgres-dsn "host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable"
```

`--force` 标志是必需的，因为执行模式会先清空目标表再导入。

执行流程：

1. 在目标库运行 PostgreSQL 迁移（确保表结构存在）
2. 解析 SQLite 和 PostgreSQL 两端都存在的表
3. 按外键依赖顺序（categories → feeds → articles → ... → topic_tag_embeddings）逐表导入
4. `topic_tag_embeddings.vector` 字段自动从 SQLite 的 JSON 文本转换为 PostgreSQL 的 `vector(1536)` 类型
5. 导入完成后重置所有序列（`setval()`）
6. 自动运行验证

### 4.3 单独验证

如需单独验证数据完整性：

```bash
go run cmd/migrate-db/main.go \
  --mode verify-only \
  --sqlite-path ./rss_reader.db \
  --postgres-dsn "host=127.0.0.1 user=postgres password=postgres dbname=rss_reader port=5432 sslmode=disable"
```

验证内容：

| 检查项 | 说明 |
|--------|------|
| 行数对比 | 每张表 SQLite 行数 vs PostgreSQL 行数 |
| 序列状态 | 每个序列的 `nextval` 应大于表中最大 ID |
| 抽样比对 | 每张表抽取最多 10 行，逐字段比对源和目标值 |
| 向量校验 | 抽样比对 embedding 向量转换的正确性 |

## 第五步：确认运行

```bash
cd backend-go
go run cmd/server/main.go
```

启动后检查：

- 日志中无迁移报错
- 前端能正常加载订阅源和文章
- Topic 分析功能正常

## 回滚方案

如果迁移后发现问题，可以回退到 SQLite：

1. 停掉后端
2. 修改 `config.yaml` 中的 `driver` 为 `sqlite`，`dsn` 改回 SQLite 文件路径
3. 原始 SQLite 文件未被修改，直接重启即可
4. PostgreSQL 容器可停止或保留用于事后分析

```bash
docker compose down
```

> SQLite 数据库文件在迁移过程中只读不写，始终保留完整备份。

## 迁移涉及的 25 张表

按导入顺序：

| 表名 | 说明 |
|------|------|
| `categories` | 分类 |
| `feeds` | 订阅源 |
| `articles` | 文章 |
| `ai_summaries` | AI 摘要 |
| `ai_summary_feeds` | 摘要关联的订阅源 |
| `scheduler_tasks` | 调度任务 |
| `ai_settings` | AI 配置（键值对） |
| `ai_providers` | AI 供应商 |
| `ai_routes` | AI 路由 |
| `ai_route_providers` | AI 路由-供应商绑定 |
| `ai_call_logs` | AI 调用日志 |
| `reading_behaviors` | 阅读行为 |
| `user_preferences` | 用户偏好 |
| `topic_tags` | 主题标签 |
| `topic_tag_embeddings` | 主题标签向量（vector 类型转换） |
| `topic_tag_analyses` | 主题分析 |
| `topic_analysis_cursors` | 主题分析游标 |
| `ai_summary_topics` | 摘要-主题关联 |
| `article_topic_tags` | 文章-主题关联 |
| `firecrawl_jobs` | Firecrawl 抓取任务 |
| `tag_jobs` | 标签任务 |
| `digest_configs` | 摘要配置（可选） |
| `topic_analysis_jobs` | 主题分析任务（可选） |

> 迁移后新增的表（如 `topic_tag_relations`、`embedding_config`、`embedding_queues`、`merge_reembedding_queues`、`narrative_summaries`）不涉及 SQLite 数据导入，由版本化迁移自动创建。

## 关键代码位置

| 文件 | 职责 |
|------|------|
| `backend-go/internal/platform/database/db.go` | 数据库初始化入口，连接 PostgreSQL |
| `backend-go/internal/platform/database/migrator.go` | 版本化迁移框架，`schema_migrations` 追踪表 |
| `backend-go/internal/platform/database/postgres_migrations.go` | 12 个 PostgreSQL 迁移注册 |
| `backend-go/internal/platform/database/bootstrap_postgres.go` | Postgres schema 引导：AutoMigrate + 索引 |
| `backend-go/internal/platform/database/connect_postgres.go` | PostgreSQL 连接与连接池配置 |
| `backend-go/internal/platform/database/datamigrate/types.go` | 数据迁移类型定义和表规格 |
| `backend-go/cmd/migrate-db/main.go` | 数据迁移 CLI 工具入口 |
| `docker-compose.yml` | PostgreSQL 容器定义 |

## 注意事项

- **停机操作**：数据迁移期间后端必须停机，否则会出现数据不一致
- **不可重复执行**：`execute` 模式会先 TRUNCATE 目标表，重复运行会清空已有数据
- **pgvector 版本**：容器镜像为 `pgvector/pgvector:pg18-trixie`，基于 PostgreSQL 18
- **连接池**：PostgreSQL 连接池参数可在 `config.yaml` 中配置（`max_idle_conns`、`max_open_conns`、`conn_max_lifetime_minutes`、`conn_max_idle_time_minutes`）
- **环境变量优先**：`DATABASE_DRIVER` 和 `DATABASE_DSN` 环境变量会覆盖配置文件
- **容器名称**：docker-compose 中 PostgreSQL 容器名为 `zanebono-rssreader-pgvector`

>>>>>>> Stashed changes:docs/operations/database-operations.md
