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
