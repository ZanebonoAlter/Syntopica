# 数据库字段说明文档

本文档详细说明了 RSS Reader 项目中所有数据库表（35 张）的字段用途、数据流向和工作流程。

---

## 完整表清单

| 表名 | 说明 | 对应模型 |
|------|------|----------|
| `categories` | 分类 | `models.Category` |
| `feeds` | 订阅源 | `models.Feed` |
| `articles` | 文章 | `models.Article` |
| `scheduler_tasks` | 调度任务状态 | `models.SchedulerTask` |
| `ai_settings` | AI 配置（键值对） | `models.AISettings` |
| `ai_providers` | AI 供应商 | `models.AIProvider` |
| `ai_routes` | AI 路由 | `models.AIRoute` |
| `ai_route_providers` | AI 路由-供应商绑定 | `models.AIRouteProvider` |
| `ai_call_logs` | AI 调用日志 | `models.AICallLog` |
| `ai_summaries` | AI 批量摘要 | `models.AISummary`（已废弃） |
| `ai_summary_feeds` | AI 摘要-Feed 关联 | `models.AISummaryFeed`（已废弃） |
| `ai_summary_topics` | AI 摘要-主题关联 | `models.AISummaryTopic`（已废弃） |
| `reading_behaviors` | 阅读行为 | `models.ReadingBehavior` |
| `user_preferences` | 用户偏好 | `models.UserPreference` |
| `topic_tags` | 主题标签 | `models.TopicTag` |
| `topic_tag_embeddings` | 主题标签向量 | `models.TopicTagEmbedding` |
| `topic_tag_analyses` | 主题分析快照 | `models.TopicTagAnalysis` |
| `topic_analysis_cursors` | 主题分析游标 | `models.TopicAnalysisCursor` |
| `topic_analysis_jobs` | 主题分析任务队列 | `topicanalysis.topicAnalysisJobRecord` |
| `article_topic_tags` | 文章-主题关联 | `models.ArticleTopicTag` |
| `embedding_config` | 向量配置 | `models.EmbeddingConfig` |
| `embedding_queues` | 向量生成队列 | `models.EmbeddingQueue` |
| `merge_reembedding_queues` | 合并后重算向量队列 | `models.MergeReembeddingQueue` |
| `semantic_labels` | 语义标签（辅助标签+SemanticBoard 统一表） | `models.SemanticLabel` |
| `topic_tag_semantic_labels` | tag-辅助标签关联 | `models.TopicTagSemanticLabel` |
| `topic_tag_board_labels` | tag-SemanticBoard 匹配结果 | `models.TopicTagBoardLabel` |
| `board_composition` | board 构成 | `models.BoardComposition` |
| `firecrawl_jobs` | Firecrawl 抓取任务 | `models.FirecrawlJob` |
| `tag_jobs` | 标签任务 | `models.TagJob` |
| `narrative_summaries` | 叙事摘要 | `models.NarrativeSummary` |
| `narrative_boards` | 叙事板块 | `models.NarrativeBoard` |
| `otel_spans` | OpenTelemetry 链路追踪 | `tracing.OtelSpan` |
| `digest_configs` | Digest 推送配置 | （已废弃） |
| `schema_migrations` | 迁移版本追踪 | （框架管理） |

---

## 核心表结构

### 1. articles（文章表）

存储 RSS 文章的核心数据。

#### 基础字段

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `feed_id` | INTEGER NOT NULL | 所属订阅源 ID |
| `title` | VARCHAR(500) NOT NULL | 文章标题 |
| `description` | TEXT | 文章描述 |
| `content` | TEXT | RSS 原始内容（HTML 片段） |
| `link` | VARCHAR(1000) | 文章链接 |
| `image_url` | VARCHAR(1000) | 封面图 |
| `pub_date` | TIMESTAMP | 发布时间 |
| `author` | VARCHAR(200) | 作者 |
| `read` | BOOLEAN DEFAULT false | 是否已读 |
| `favorite` | BOOLEAN DEFAULT false | 是否收藏 |
| `created_at` | TIMESTAMP | 创建时间 |

#### 内容相关字段

| 字段名 | 类型 | 用途 | 来源 | 格式 |
|--------|------|------|------|------|
| `content` | TEXT | RSS 原始内容（HTML 片段） | RSS Feed 解析 | HTML |
| `firecrawl_content` | TEXT | Firecrawl 抓取的完整网页内容 | Firecrawl Scheduler | Markdown |
| `ai_content_summary` | TEXT | AI 生成的优化总结内容 | AI Summary Scheduler | Markdown |

#### AI 总结状态字段

| 字段名 | 类型 | 用途 | 可选值 |
|--------|------|------|--------|
| `summary_status` | VARCHAR(20) DEFAULT 'complete' | AI 总结状态 | `incomplete` / `pending` / `complete` / `failed` |
| `summary_generated_at` | TIMESTAMP | AI 总结生成时间 | — |
| `summary_processing_started_at` | TIMESTAMP | AI 总结开始处理时间 | — |
| `completion_attempts` | INTEGER DEFAULT 0 | AI 总结重试次数 | — |
| `completion_error` | TEXT | AI 总结错误信息 | — |

#### Firecrawl 状态字段

| 字段名 | 类型 | 用途 | 可选值 |
|--------|------|------|--------|
| `firecrawl_status` | VARCHAR(20) DEFAULT 'pending' | Firecrawl 抓取状态 | `pending` / `processing` / `completed` / `failed` |
| `firecrawl_error` | TEXT | Firecrawl 抓取错误信息 | — |
| `firecrawl_crawled_at` | TIMESTAMP | Firecrawl 抓取时间 | — |

#### Feed 摘要关联字段

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `feed_summary_id` | BIGINT | 关联的 AI 批量摘要 ID（FK → `ai_summaries.id`） |
| `feed_summary_generated_at` | TIMESTAMP | Feed 摘要生成时间 |

#### 虚拟字段（计算列）

| 字段名 | 用途 |
|--------|------|
| `tag_count` | 文章标签数量（SQL 计算字段） |
| `relevance_score` | 相关度评分（SQL 计算字段） |

---

### 2. feeds（订阅源表）

存储 RSS 订阅源配置。

#### 基础字段

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `title` | VARCHAR(200) NOT NULL | 订阅源标题 |
| `description` | TEXT | 描述 |
| `url` | VARCHAR(500) UNIQUE NOT NULL | RSS URL |
| `category_id` | INTEGER | 所属分类 ID |
| `icon` | VARCHAR(1000) DEFAULT 'rss' | 图标 |
| `color` | VARCHAR(20) DEFAULT '#8b5cf6' | 颜色 |
| `last_updated` | TIMESTAMP | 最后更新时间 |
| `created_at` | TIMESTAMP | 创建时间 |
| `max_articles` | INTEGER DEFAULT 100 | 最大文章数 |
| `refresh_interval` | INTEGER DEFAULT 60 | 刷新间隔（秒） |
| `refresh_status` | VARCHAR(20) DEFAULT 'idle' | 刷新状态 |
| `refresh_error` | TEXT | 刷新错误信息 |
| `last_refresh_at` | TIMESTAMP | 最后刷新时间 |

#### 功能开关字段

| 字段名 | 类型 | 用途 | 说明 |
|--------|------|------|------|
| `ai_summary_enabled` | BOOLEAN DEFAULT true | 是否启用 Feed 级 AI 批量摘要 | 跨文章聚合总结 |
| `article_summary_enabled` | BOOLEAN DEFAULT false | 是否启用文章级 AI 总结 | 依赖 Firecrawl 先抓取完整内容 |
| `completion_on_refresh` | BOOLEAN DEFAULT true | 刷新时是否自动触发内容补全 | — |
| `max_completion_retries` | INTEGER DEFAULT 3 | AI 总结最大重试次数 | — |
| `firecrawl_enabled` | BOOLEAN DEFAULT false | 是否启用 Firecrawl 抓取完整内容 | 需要全局配置 Firecrawl API |

---

### 3. categories（分类表）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `name` | VARCHAR(100) UNIQUE NOT NULL | 分类名称 |
| `slug` | VARCHAR(50) UNIQUE | URL 友好标识 |
| `icon` | VARCHAR(50) DEFAULT 'folder' | 图标 |
| `color` | VARCHAR(20) DEFAULT '#6366f1' | 颜色 |
| `description` | TEXT | 描述 |
| `created_at` | TIMESTAMP | 创建时间 |

---

### 4. scheduler_tasks（调度任务表）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `name` | VARCHAR(50) UNIQUE NOT NULL | 任务名称 |
| `description` | VARCHAR(200) | 任务描述 |
| `check_interval` | INTEGER DEFAULT 60 NOT NULL | 检查间隔（秒） |
| `last_execution_time` | TIMESTAMP | 上次执行时间 |
| `next_execution_time` | TIMESTAMP | 下次执行时间 |
| `status` | VARCHAR(20) DEFAULT 'idle' | 状态 |
| `last_error` | TEXT | 最近错误 |
| `last_error_time` | TIMESTAMP | 最近错误时间 |
| `total_executions` | INTEGER DEFAULT 0 | 总执行次数 |
| `successful_executions` | INTEGER DEFAULT 0 | 成功次数 |
| `failed_executions` | INTEGER DEFAULT 0 | 失败次数 |
| `consecutive_failures` | INTEGER DEFAULT 0 | 连续失败次数 |
| `last_execution_duration` | FLOAT | 上次执行耗时（秒） |
| `last_execution_result` | TEXT | 上次执行结果 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

| 任务名 | 描述 | 执行间隔 |
|--------|------|----------|
| `auto_refresh` | 自动刷新 RSS 订阅源 | 60 秒 |
| `ai_summary` | AI 智能总结文章内容（基于 Firecrawl）| 3600 秒（1小时）|

---

### 6. AI 配置相关表

#### ai_settings（AI 配置键值对）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `key` | VARCHAR(100) UNIQUE NOT NULL | 配置键 |
| `value` | TEXT | JSON 值 |
| `description` | VARCHAR(200) | 说明 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

#### ai_providers（AI 供应商）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `name` | VARCHAR(100) UNIQUE NOT NULL | 供应商名称 |
| `provider_type` | VARCHAR(50) DEFAULT 'openai_compatible' | 供应商类型 |
| `base_url` | VARCHAR(500) NOT NULL | API 地址 |
| `api_key` | TEXT NOT NULL | API 密钥 |
| `model` | VARCHAR(100) NOT NULL | 模型名称 |
| `enabled` | BOOLEAN DEFAULT true | 是否启用 |
| `timeout_seconds` | INTEGER DEFAULT 120 | 超时时间 |
| `max_tokens` | INTEGER | 最大 token 数 |
| `temperature` | FLOAT | 温度参数 |
| `metadata` | TEXT | 扩展元数据 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

#### ai_routes（AI 路由）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `name` | VARCHAR(100) NOT NULL | 路由名称 |
| `capability` | VARCHAR(50) NOT NULL | 能力标识 |
| `enabled` | BOOLEAN DEFAULT true | 是否启用 |
| `strategy` | VARCHAR(50) DEFAULT 'ordered_failover' | 路由策略 |
| `description` | VARCHAR(255) | 描述 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(capability, name)`

#### ai_route_providers（AI 路由-供应商绑定）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `route_id` | INTEGER NOT NULL | 路由 ID |
| `provider_id` | INTEGER NOT NULL | 供应商 ID |
| `priority` | INTEGER DEFAULT 100 | 优先级（数值越小越高） |
| `enabled` | BOOLEAN DEFAULT true | 是否启用 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(route_id, provider_id)`

#### ai_call_logs（AI 调用日志）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `capability` | VARCHAR(50) NOT NULL | 能力标识 |
| `route_name` | VARCHAR(100) NOT NULL | 路由名称 |
| `provider_name` | VARCHAR(100) NOT NULL | 供应商名称 |
| `success` | BOOLEAN NOT NULL | 是否成功 |
| `is_fallback` | BOOLEAN DEFAULT false | 是否为降级调用 |
| `latency_ms` | INTEGER | 延迟（毫秒） |
| `error_code` | VARCHAR(100) | 错误码 |
| `error_message` | TEXT | 错误信息 |
| `request_meta` | TEXT | 请求元数据 |
| `created_at` | TIMESTAMP | 创建时间 |

---

### 6.5. AI 摘要关联表

以下三张表用于存储 Feed 级 AI 批量摘要，通过 `articles.feed_summary_id` 将文章关联到摘要。

#### ai_summaries（AI 批量摘要）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `feed_id` | BIGINT | 所属 Feed ID（FK → `feeds.id`） |
| `category_id` | BIGINT | 所属分类 ID（FK → `categories.id`） |
| `title` | VARCHAR(200) NOT NULL | 摘要标题 |
| `summary` | TEXT NOT NULL | 摘要正文 |
| `key_points` | TEXT | 关键要点（JSON） |
| `articles` | TEXT | 覆盖的文章 ID 列表（JSON 数组） |
| `article_count` | BIGINT DEFAULT 0 | 覆盖文章数 |
| `time_range` | BIGINT DEFAULT 180 | 时间范围（分钟） |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

#### ai_summary_feeds（AI 摘要-Feed 关联）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `summary_id` | BIGINT NOT NULL | 摘要 ID |
| `feed_id` | BIGINT NOT NULL | Feed ID |
| `feed_title` | VARCHAR(200) | Feed 标题快照 |
| `feed_icon` | VARCHAR(1000) | Feed 图标快照 |
| `feed_color` | VARCHAR(20) | Feed 颜色快照 |
| `article_count` | BIGINT DEFAULT 0 | 该 Feed 覆盖文章数 |
| `created_at` | TIMESTAMP | 创建时间 |

#### ai_summary_topics（AI 摘要-主题关联）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `summary_id` | BIGINT NOT NULL | 摘要 ID（FK → `ai_summaries.id`） |
| `topic_tag_id` | BIGINT NOT NULL | 标签 ID（FK → `topic_tags.id`, ON DELETE CASCADE） |
| `score` | NUMERIC DEFAULT 0 | 相关度评分 |
| `source` | VARCHAR(20) DEFAULT 'llm' | 来源（`llm`） |
| `created_at` | TIMESTAMP | 创建时间 |

---

### 7. 主题标签相关表

#### topic_tags（主题标签主表）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `slug` | VARCHAR(120) NOT NULL | 稳定标识 |
| `label` | VARCHAR(160) NOT NULL | 展示名称 |
| `category` | VARCHAR(20) DEFAULT 'keyword' | 标签分类（`event`/`person`/`keyword`） |
| `icon` | VARCHAR(100) | Iconify 图标 ID |
| `aliases` | TEXT | 别名列表（JSON 数组） |
| `description` | TEXT | LLM 生成的标签描述 |
| `is_canonical` | BOOLEAN DEFAULT false | 是否为规范标签 |
| `source` | VARCHAR(20) DEFAULT 'llm' | 标签来源（`llm`/`heuristic`/`manual`） |
| `feed_count` | INTEGER DEFAULT 0 | 引用此标签的不重复 Feed 数 |
| `status` | VARCHAR(20) DEFAULT 'active' | 状态（`active`/`merged`） |
| `merged_into_id` | INTEGER REFERENCES topic_tags(id) | 合并目标标签 ID |
| `is_watched` | BOOLEAN DEFAULT false | 是否为用户关注标签 |
| `watched_at` | TIMESTAMP | 关注时间 |
| `quality_score` | FLOAT DEFAULT 0 | 质量评分 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |
| `kind` | VARCHAR(20) DEFAULT 'keyword' | 已废弃，映射到 `category` |
| `concept_id` | INTEGER | 已废弃，不再使用 |

唯一约束：`(category, slug)`

唯一约束（topic_tag_semantic_labels）：`(topic_tag_id, semantic_label_id)`

唯一约束（topic_tag_board_labels）：`(topic_tag_id, semantic_board_id)`

唯一约束（board_composition）：`(board_id, auxiliary_label_id)`

#### topic_tag_embeddings（主题标签向量）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `topic_tag_id` | INTEGER NOT NULL | 关联标签 ID |
| `embedding_type` | VARCHAR(20) NOT NULL DEFAULT 'identity' | 嵌入类型：`identity`（标签名）、`semantic`（语义描述）、`event_keyword`（事件标签关键词） |
| `vector` | TEXT NOT NULL | 已废弃：旧版 JSON 文本向量 |
| `embedding` | vector(1536) | pgvector 向量列 |
| `dimension` | INTEGER NOT NULL | 向量维度（如 1536） |
| `model` | VARCHAR(50) NOT NULL | 生成模型名称 |
| `text_hash` | VARCHAR(64) | 标签文本哈希，参与唯一约束，同一 tag+type 可有多行（不同 text_hash） |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`idx_topic_tag_embeddings_tag_type_hash (topic_tag_id, embedding_type, text_hash)`

HNSW 索引：`idx_topic_tag_embeddings_embedding USING hnsw (embedding vector_cosine_ops)`

#### topic_tag_analyses（主题分析结果快照）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `topic_tag_id` | BIGINT | 关联标签 ID |
| `analysis_type` | VARCHAR | 分析类型（`event`/`person`/`keyword`） |
| `window_type` | VARCHAR | 时间窗（`daily`/`weekly`） |
| `anchor_date` | TIMESTAMP | 锚点日期 |
| `summary_count` | INTEGER | 覆盖的摘要数量 |
| `payload_json` | TEXT | 分析结果 JSON |
| `source` | VARCHAR | 来源（`ai`/`heuristic`/`cached`） |
| `version` | INTEGER | 分析版本号 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(topic_tag_id, analysis_type, window_type, anchor_date)`

#### topic_analysis_cursors（主题分析游标）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `topic_tag_id` | BIGINT | 关联标签 ID |
| `analysis_type` | VARCHAR | 分析类型 |
| `window_type` | VARCHAR | 时间窗 |
| `last_summary_id` | BIGINT | 上次分析已处理到的最大 summary ID |
| `last_updated_at` | TIMESTAMP | 上次刷新时间 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(topic_tag_id, analysis_type, window_type)`

#### topic_analysis_jobs（主题分析任务队列）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | VARCHAR(64) PK | 主键（字符串 ID） |
| `topic_tag_id` | BIGINT | 分析目标标签 ID |
| `analysis_type` | VARCHAR(32) | 分析类型 |
| `window_type` | VARCHAR(32) | 时间窗 |
| `anchor_date` | TIMESTAMP | 锚点日期 |
| `priority` | INTEGER | 优先级（数值越小越高） |
| `status` | VARCHAR(32) | 任务状态 |
| `retry_count` | INTEGER | 重试次数（最多 3 次） |
| `error_message` | TEXT | 失败信息 |
| `progress` | INTEGER | 运行进度 |
| `created_at` | TIMESTAMP | 创建时间 |
| `started_at` | TIMESTAMP | 开始时间 |
| `completed_at` | TIMESTAMP | 完成时间 |

#### article_topic_tags（文章-主题关联）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `article_id` | INTEGER NOT NULL | 文章 ID |
| `topic_tag_id` | INTEGER NOT NULL | 标签 ID |
| `score` | FLOAT DEFAULT 0 | 相关度评分 |
| `source` | VARCHAR(20) DEFAULT 'llm' | 来源 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(article_id, topic_tag_id)`

---

### 8. 语义标签相关表（已废弃：层级关系相关表）

> 旧版层级体系（hierarchy_config、adopt_narrower_queues、multi_parent_resolve_queues、abstract_tag_update_queues、hierarchy_pending_changes）已在语义标签/板块体系重构中移除。相关功能由 semantic_labels 的 label_type=auxiliary/board 和 topic_tag_semantic_labels 替代。

---

---

### 9. 向量相关表

#### embedding_config（向量配置）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `key` | VARCHAR(100) UNIQUE NOT NULL | 配置键 |
| `value` | TEXT NOT NULL | 配置值 |
| `description` | VARCHAR(200) | 说明 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

默认配置项：

| key | 默认值 | 说明 |
|-----|--------|------|
| `high_similarity_threshold` | `0.97` | 高相似度阈值，自动复用已有标签 |
| `low_similarity_threshold` | `0.78` | 低相似度阈值，自动创建新标签 |
| `embedding_model` | （空） | 覆盖 embedding 模型名 |
| `embedding_dimension` | `1536` | 向量维度 |
| `narrative_board_embedding_threshold` | `0.7` | 板块概念 embedding 匹配阈值（已废弃，由 semantic_board_match_* 替代） |
| `narrative_board_hotspot_threshold` | `6` | 热点板抽象树节点数阈值（已废弃） |

#### embedding_queues（向量生成队列）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `tag_id` | BIGINT NOT NULL REFERENCES topic_tags(id) | 关联标签 ID |
| `status` | VARCHAR(20) DEFAULT 'pending' | 状态 |
| `error_message` | TEXT | 错误信息 |
| `retry_count` | INTEGER DEFAULT 0 | 重试次数 |
| `created_at` | TIMESTAMP | 创建时间 |
| `started_at` | TIMESTAMP | 开始时间 |
| `completed_at` | TIMESTAMP | 完成时间 |

#### merge_reembedding_queues（合并后重算向量队列）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `source_tag_id` | BIGINT NOT NULL REFERENCES topic_tags(id) | 源标签 ID |
| `target_tag_id` | BIGINT NOT NULL REFERENCES topic_tags(id) | 目标标签 ID |
| `status` | VARCHAR(20) DEFAULT 'pending' | 状态 |
| `error_message` | TEXT | 错误信息 |
| `retry_count` | INTEGER DEFAULT 0 | 重试次数 |
| `created_at` | TIMESTAMP | 创建时间 |
| `started_at` | TIMESTAMP | 开始时间 |
| `completed_at` | TIMESTAMP | 完成时间 |

---

### 10. 任务队列表

#### firecrawl_jobs（Firecrawl 抓取任务）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `article_id` | INTEGER NOT NULL | 关联文章 ID |
| `status` | VARCHAR(20) DEFAULT 'pending' | 状态 |
| `priority` | INTEGER DEFAULT 0 | 优先级 |
| `attempt_count` | INTEGER DEFAULT 0 | 尝试次数 |
| `max_attempts` | INTEGER DEFAULT 5 | 最大尝试次数 |
| `available_at` | TIMESTAMP NOT NULL | 可执行时间 |
| `leased_at` | TIMESTAMP | 租约获取时间 |
| `lease_expires_at` | TIMESTAMP | 租约过期时间 |
| `last_error` | TEXT | 最近错误 |
| `url_snapshot` | VARCHAR(1000) | URL 快照 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

#### tag_jobs（标签任务）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `article_id` | INTEGER NOT NULL | 关联文章 ID |
| `status` | VARCHAR(20) DEFAULT 'pending' | 状态 |
| `priority` | INTEGER DEFAULT 0 | 优先级 |
| `attempt_count` | INTEGER DEFAULT 0 | 尝试次数 |
| `max_attempts` | INTEGER DEFAULT 5 | 最大尝试次数 |
| `available_at` | TIMESTAMP NOT NULL | 可执行时间 |
| `leased_at` | TIMESTAMP | 租约获取时间 |
| `lease_expires_at` | TIMESTAMP | 租约过期时间 |
| `last_error` | TEXT | 最近错误 |
| `feed_name_snapshot` | VARCHAR(200) | Feed 名称快照 |
| `category_name_snapshot` | VARCHAR(100) | 分类名称快照 |
| `force_retag` | BOOLEAN DEFAULT false | 是否强制重新打标签 |
| `reason` | VARCHAR(50) | 入队原因 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

---

### 11. narrative_summaries（叙事摘要表）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `title` | VARCHAR(300) NOT NULL | 叙事标题 |
| `summary` | TEXT NOT NULL | 叙事内容 |
| `status` | VARCHAR(20) NOT NULL | 状态（`emerging`/`continuing`/`splitting`/`merging`/`ending`） |
| `period` | VARCHAR(20) DEFAULT 'daily' | 周期（`daily` / `watched_tag`） |
| `period_date` | TIMESTAMP NOT NULL | 周期日期 |
| `generation` | INTEGER DEFAULT 0 | 代际 |
| `parent_ids` | TEXT | 父叙事 ID 列表 |
| `related_tag_ids` | TEXT | 关联标签 ID 列表 |
| `related_article_ids` | TEXT | 关联文章 ID 列表 |
| `source` | VARCHAR(20) DEFAULT 'ai' | 来源 |
| `scope_type` | VARCHAR(20) NOT NULL DEFAULT 'global' | 作用域类型（global / feed_category） |
| `scope_category_id` | INTEGER | 分类 ID（索引 idx_narrative_scope） |
| `scope_label` | VARCHAR(100) | 分类名称 |
| `board_id` | INTEGER | 所属 Board ID（索引，FK→narrative_boards.id） |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

---

### 12. narrative_boards（叙事板块表）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `period_date` | TIMESTAMP NOT NULL | 周期日期（索引 idx_narrative_boards_period） |
| `name` | VARCHAR(300) NOT NULL | 板块名称 |
| `description` | TEXT | 板块描述 |
| `scope_type` | VARCHAR(20) NOT NULL DEFAULT 'global' | 作用域类型（global / feed_category） |
| `scope_category_id` | INTEGER | 分类 ID（索引 idx_narrative_boards_scope） |
| `scope_label` | VARCHAR(100) | 分类名称 |
| `event_tag_ids` | TEXT | 关联 event 标签 ID 列表（JSON 数组） |
| `prev_board_ids` | TEXT | 前日关联 Board ID 列表（JSON 数组，用于跨日延续） |
| `semantic_board_id` | INTEGER | 关联的 SemanticBoard ID（索引，FK → `semantic_labels.id`） |
| `is_system` | BOOLEAN NOT NULL DEFAULT false | 是否为系统自动生成 |
| `created_at` | TIMESTAMP | 创建时间 |

### 13. 语义标签相关表

#### semantic_labels（语义标签统一表）

辅助标签和 SemanticBoard 共存于同一张表，通过 `label_type` 字段区分。

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `label` | VARCHAR(160) NOT NULL | 展示名称 |
| `slug` | VARCHAR(120) NOT NULL | 稳定标识 |
| `embedding` | vector(1536) | pgvector 语义向量 |
| `label_type` | VARCHAR(20) NOT NULL | 类型：`auxiliary`（辅助标签）/ `board`（SemanticBoard） |
| `aliases` | JSONB DEFAULT '[]' | 别名列表 |
| `ref_count` | INTEGER DEFAULT 0 | 引用计数（辅助标签被 tag 引用次数） |
| `description` | TEXT | 描述 |
| `display_order` | INTEGER DEFAULT 0 | 显示排序 |
| `source` | VARCHAR(20) DEFAULT 'llm_extract' | 来源：`llm_extract`（LLM 提取）/ `llm_suggest`（LLM 建议升级）/ `manual`（手动创建） |
| `status` | VARCHAR(20) DEFAULT 'active' | 状态：`active` / `disabled` |
| `protected` | BOOLEAN DEFAULT false | 是否受保护（不可自动删除） |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(label_type, slug)`

#### topic_tag_semantic_labels（tag-辅助标签关联）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `topic_tag_id` | BIGINT NOT NULL | 关联 tag ID（FK → `topic_tags.id`） |
| `semantic_label_id` | BIGINT NOT NULL | 关联辅助标签 ID（FK → `semantic_labels.id`） |

唯一约束：`(topic_tag_id, semantic_label_id)`

#### topic_tag_board_labels（tag-SemanticBoard 匹配结果）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `topic_tag_id` | BIGINT NOT NULL | 关联 tag ID（FK → `topic_tags.id`） |
| `semantic_board_id` | BIGINT NOT NULL | 关联 SemanticBoard ID（FK → `semantic_labels.id`） |
| `score` | FLOAT NOT NULL | 匹配分数 |
| `match_reason` | VARCHAR(20) | 匹配原因：`direct_hit` / `hit_rate` / `max_sim` / `weighted` |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

唯一约束：`(topic_tag_id, semantic_board_id)`

#### board_composition（board 构成）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `board_id` | BIGINT NOT NULL | 关联 board ID（FK → `semantic_labels.id`，label_type=board） |
| `auxiliary_label_id` | BIGINT NOT NULL | 关联辅助标签 ID（FK → `semantic_labels.id`，label_type=auxiliary） |

唯一约束：`(board_id, auxiliary_label_id)`

---

### 14. 其他表

#### reading_behaviors（阅读行为）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `article_id` | INTEGER NOT NULL | 文章 ID |
| `feed_id` | INTEGER | 订阅源 ID |
| `category_id` | INTEGER | 分类 ID |
| `session_id` | VARCHAR(100) | 会话 ID |
| `event_type` | VARCHAR(20) | 事件类型 |
| `scroll_depth` | INTEGER DEFAULT 0 | 滚动深度 |
| `reading_time` | INTEGER DEFAULT 0 | 阅读时间 |
| `created_at` | TIMESTAMP | 创建时间 |

#### user_preferences（用户偏好）

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | SERIAL PK | 主键 |
| `feed_id` | INTEGER | 订阅源 ID |
| `category_id` | INTEGER | 分类 ID |
| `preference_score` | FLOAT DEFAULT 0 | 偏好评分 |
| `avg_reading_time` | INTEGER DEFAULT 0 | 平均阅读时间 |
| `interaction_count` | INTEGER DEFAULT 0 | 交互次数 |
| `scroll_depth_avg` | FLOAT DEFAULT 0 | 平均滚动深度 |
| `last_interaction_at` | TIMESTAMP | 最后交互时间 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

#### otel_spans（OpenTelemetry 链路追踪）

存储 GORM Span Exporter 导出的 OpenTelemetry span 数据。通过自定义 exporter 落库，支持 trace 查询和统计 API。

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `trace_id` | CHAR(32) NOT NULL | 追踪 ID |
| `span_id` | CHAR(16) NOT NULL | Span ID |
| `parent_span_id` | CHAR(16) DEFAULT '' | 父 Span ID |
| `trace_state` | TEXT DEFAULT '' | W3C trace state |
| `name` | VARCHAR(255) NOT NULL | Span 名称 |
| `kind` | BIGINT DEFAULT 1 | Span 类型（Internal=1, Server=2, Client=3, Producer=4, Consumer=5） |
| `status_code` | BIGINT DEFAULT 0 | 状态码（0=Unset, 1=Error, 2=OK） |
| `status_message` | TEXT DEFAULT '' | 状态信息 |
| `start_time_unix_nano` | BIGINT NOT NULL | 开始时间（Unix 纳秒） |
| `end_time_unix_nano` | BIGINT NOT NULL | 结束时间（Unix 纳秒） |
| `duration_ms` | BIGINT DEFAULT 0 | 持续时间（毫秒） |
| `service_name` | VARCHAR(100) DEFAULT 'rss-reader-backend' | 服务名称 |
| `service_version` | VARCHAR(50) DEFAULT '' | 服务版本 |
| `resource_attributes` | TEXT DEFAULT '{}' | 资源属性（JSON） |
| `scope_name` | VARCHAR(100) DEFAULT '' | Scope 名称 |
| `scope_version` | VARCHAR(50) DEFAULT '' | Scope 版本 |
| `attributes` | TEXT DEFAULT '{}' | Span 属性（JSON） |
| `events` | TEXT DEFAULT '[]' | Span 事件（JSON） |
| `links` | TEXT DEFAULT '[]' | Span 链接（JSON） |
| `created_at` | TIMESTAMP | 创建时间 |

---

### 15. 已废弃/预留表

以下表当前无 Go 代码引用，数据为空（0 行），可能为旧版功能遗留或预留功能。

#### ai_summaries / ai_summary_feeds / ai_summary_topics

这三张表对应旧版 Feed 级 AI 批量摘要功能。字段说明见 §6.5 "AI 摘要关联表"。

**状态**：当前无 Go 代码引用（`ai_summaries` 等模型已不存在于 `internal/domain/models/`），数据库中可能存有旧数据。`articles.feed_summary_id` 仍然指向 `ai_summaries.id`。

#### digest_configs（Digest 推送配置）

预留的 Digest 日报/周报推送配置表。

| 字段名 | 类型 | 用途 |
|--------|------|------|
| `id` | BIGSERIAL PK | 主键 |
| `daily_enabled` | BOOLEAN DEFAULT false | 是否启用日报 |
| `daily_time` | VARCHAR(5) DEFAULT '09:00' | 日报推送时间 |
| `weekly_enabled` | BOOLEAN DEFAULT false | 是否启用周报 |
| `weekly_day` | BIGINT DEFAULT 1 | 周报推送日（1=周一） |
| `weekly_time` | VARCHAR(5) DEFAULT '09:00' | 周报推送时间 |
| `feishu_enabled` | BOOLEAN DEFAULT false | 是否启用飞书推送 |
| `feishu_webhook_url` | TEXT | 飞书 Webhook URL |
| `feishu_push_summary` | BOOLEAN DEFAULT true | 飞书推送摘要 |
| `feishu_push_details` | BOOLEAN DEFAULT false | 飞书推送详情 |
| `obsidian_enabled` | BOOLEAN DEFAULT false | 是否启用 Obsidian 导出 |
| `obsidian_vault_path` | TEXT | Obsidian Vault 路径 |
| `obsidian_daily_digest` | BOOLEAN DEFAULT true | Obsidian 日报导出 |
| `obsidian_weekly_digest` | BOOLEAN DEFAULT true | Obsidian 周报导出 |
| `created_at` | TIMESTAMP | 创建时间 |
| `updated_at` | TIMESTAMP | 更新时间 |

**状态**：当前无 Go 代码引用，数据库中 0 行数据，标记为预留功能。

---

## 字段用途说明

### 三个内容字段的区别

#### `content`（RSS 原始内容）

- **来源**：RSS Feed 解析
- **格式**：HTML 片段
- **特点**：可能不完整，可能包含 HTML 标签
- **用途**：作为基础内容展示

#### `firecrawl_content`（完整网页内容）

- **来源**：Firecrawl 抓取
- **格式**：Markdown
- **特点**：完整网页内容，过滤了广告和导航栏
- **用途**：作为 AI 总结的输入源，不对用户直接展示

#### `ai_content_summary`（AI 优化总结）

- **来源**：AI 生成
- **格式**：Markdown
- **特点**：保留核心内容，移除冗余
- **用途**：前端默认展示的内容

---

## 数据库索引清单

### 基线索引（迁移 `20260403_0002` 创建）

| 索引名 | 表 | 列 |
|--------|------|------|
| `idx_articles_feed_created_at` | articles | `(feed_id, created_at DESC)` |
| `idx_articles_pub_date` | articles | `(pub_date)` |
| `idx_article_topic_tags_topic_article` | article_topic_tags | `(topic_tag_id, article_id)` |
| `idx_reading_behaviors_feed_created_at` | reading_behaviors | `(feed_id, created_at DESC)` |
| `idx_firecrawl_jobs_status_available_at` | firecrawl_jobs | `(status, available_at)` |
| `idx_firecrawl_jobs_lease_expires_at` | firecrawl_jobs | `(lease_expires_at)` |
| `idx_tag_jobs_status_available_at` | tag_jobs | `(status, available_at)` |
| `idx_tag_jobs_lease_expires_at` | tag_jobs | `(lease_expires_at)` |

### 向量索引（迁移 `20260413_0001` 创建）

| 索引名 | 表 | 类型 |
|--------|------|------|
| `idx_topic_tag_embeddings_embedding` | topic_tag_embeddings | HNSW `(embedding vector_cosine_ops)` |
| `idx_topic_tag_embeddings_tag_type_hash` | topic_tag_embeddings | UNIQUE `(topic_tag_id, embedding_type, text_hash)` |

### 迁移补充索引

| 索引名 | 表 | 迁移版本 |
|--------|------|----------|
| `idx_topic_tags_status` | topic_tags | `20260413_0003` |
| `idx_topic_tags_merged_into_id` | topic_tags | `20260413_0003` |
| `idx_topic_tag_embeddings_tag_type_hash` | topic_tag_embeddings | `20260514_0001`（替代旧 `idx_topic_tag_embeddings_tag_type`） |

---

## 更新日志

### 2026-05-22

- 语义标签/板块体系重构：移除 hierarchy、board_concepts、topic_tag_relations 相关表定义
- 新增 semantic_labels、topic_tag_semantic_labels、topic_tag_board_labels、board_composition 四张表
- narrative_boards 新增 semantic_board_id 字段，移除 abstract_tag_id 和 board_concept_id
- topic_tags.concept_id 标记为已废弃
- embedding_config 新增 semantic_board_match_* 和 semantic_board_upgrade_* 配置

### 2026-05-14

- 更新 `topic_tag_embeddings`：新增 `embedding_type` 字段（`identity`/`semantic`/`event_keyword`），更新 `text_hash` 描述
- 唯一约束从 `(topic_tag_id, embedding_type)` 改为 `(topic_tag_id, embedding_type, text_hash)`，支持同一标签多行关键词嵌入
- 新增 `event_keyword` 嵌入类型，用于事件标签关键词向量化
- 迁移 `20260514_0001`：`idx_topic_tag_embeddings_tag_type` → `idx_topic_tag_embeddings_tag_type_hash`

### 2026-05-14

- 补齐缺失表覆盖：新增 §6.5 AI 摘要关联表、§8 层级关系相关表、§15 已废弃/预留表
- 新增 `otel_spans`（§14）、`hierarchy_pending_changes`（§8）、`digest_configs`（§15）字段说明
- 补充 `articles.feed_summary_id`、`articles.feed_summary_generated_at` 字段
- 补充 `topic_tags.concept_id` FK 字段
- 完整表清单扩充至 38 张（原 29 张）
- 迁移"工作流程"、"状态流转图"、"配置要求"章节至 `DATA_LIFECYCLE.md`
- 更新章节编号（§9 向量相关表、§10 任务队列表、§11-§13 叙事相关表、§14 其他表、§15 已废弃/预留表）
- 修正"相关文档"交叉引用路径，新增指向 `ER_DIAGRAM.md` 和 `DATA_LIFECYCLE.md` 的链接

### 2026-05-01

- 新增 `narrative_boards`（叙事板块）和 `board_concepts`（板块概念）两个完整表的字段说明
- 新增 `narrative_summaries` 的 `scope_type`、`scope_category_id`、`scope_label`、`board_id` 字段
- 新增 `ai_settings` 的 `narrative_board_embedding_threshold` 和 `narrative_board_hotspot_threshold` 配置项
- `narrative_summaries.period` 新增 `watched_tag` 值

### 2026-04-16

- 全面重写文档，覆盖所有 29 张表的完整字段说明
- 新增 AI Provider/Route 相关表（`ai_providers`、`ai_routes`、`ai_route_providers`、`ai_call_logs`）
- 新增向量相关表（`embedding_config`、`embedding_queues`、`merge_reembedding_queues`）
- 新增任务队列表（`firecrawl_jobs`、`tag_jobs`、`topic_analysis_jobs`）
- 新增 `topic_tag_relations`（标签层级关系）
- 新增 `narrative_summaries`（叙事摘要）
- 新增 `topic_tags` 的 `status`、`merged_into_id`、`description`、`is_watched`、`watched_at`、`quality_score` 字段
- 新增 `articles` 的 `summary_processing_started_at` 字段
- 新增索引清单章节
- 更新 Summary 状态流转说明（默认值为 `complete` 而非 `incomplete`）

### 2026-03-05

- 将 `content_completion` 任务重命名为 `ai_summary`
- 创建了本文档

---

## 相关文档

- [全局实体关系图](ER_DIAGRAM.md) — 35 张表的 FK 关系图（ASCII + Mermaid）和约束矩阵
- [数据生命周期](DATA_LIFECYCLE.md) — 6 条数据链路的状态字段流转说明
- [数据流](../reference/architecture/data-flow.md) — 代码执行流、API 调用链、前端 store 交互
- [数据库运维说明](../operations/database.md) — 数据库运维说明
- [PostgreSQL 迁移手册](../operations/postgres-migration.md) — PostgreSQL 迁移手册
- [AGENTS.md](../../AGENTS.md) — 项目开发指南
