# 全局实体关系图

本文档提供 RSS Reader 数据库的全局实体关系图，覆盖 38 张表、35 条 FK 约束，按 6 个业务域组织。

> 域级 ER 图使用 Mermaid `erDiagram` 语法，渲染依赖 GitHub/VSCode Mermaid 插件。全局概览图使用纯 ASCII 作为 fallback。

---

## 全局域级概览

```
┌─────────────────┐       ┌─────────────────────┐
│     Core        │       │    Topic Tags        │
│  ┌───────────┐  │  FK   │  ┌─────────────────┐ │
│  │ categories│──┼───────┼─→│   topic_tags     │ │  ← 12 incoming FKs (hub)
│  ├───────────┤  │       │  ├─────────────────┤ │
│  │   feeds   │  │       │  │ topic_tag_       │ │
│  ├───────────┤  │       │  │   embeddings     │ │
│  │ articles  │  │       │  ├─────────────────┤ │
│  ├───────────┤  │       │  │ topic_tag_       │ │
│  │ reading_  │  │       │  │   relations (★)  │─┼───┐
│  │  behaviors│  │       │  ├─────────────────┤ │   │
│  ├───────────┤  │       │  │ article_topic_   │ │   │
│  │ user_     │  │       │  │   tags           │ │   │
│  │ preferences│ │       │  ├─────────────────┤ │   │
│  ├───────────┤  │       │  │ embedding_queues │ │   │
│  │ firecrawl_│  │       │  ├─────────────────┤ │   │
│  │   jobs    │  │       │  │ merge_reembedding│ │   │
│  ├───────────┤  │       │  │   _queues        │ │   │
│  │ tag_jobs  │  │       │  ├─────────────────┤ │   │
│  ├───────────┤  │       │  │ topic_tag_       │ │   │
│  │ schema_   │  │       │  │   analyses       │ │   │
│  │ migrations│  │       │  ├─────────────────┤ │   │
│  └───────────┘  │       │  │ topic_analysis_  │ │   │
└─────────────────┘       │  │   cursors        │ │   │
                          │  ├─────────────────┤ │   │
                          │  │ topic_analysis_  │ │   │
                          │  │   jobs           │ │   │
                          │  └─────────────────┘ │   │
                          └─────────────────────┘   │
                                     ↑               │
                                     │ self-ref      │ FK (hierarchy)
┌─────────────────┐       ┌─────────────────────┐   │
│  AI Summaries   │       │    Hierarchy         │   │
│  ┌───────────┐  │       │  ┌─────────────────┐ │   │
│  │ai_summaries│ │  FK   │  │ hierarchy_config│ │   │
│  ├───────────┤  │───→───│  ├─────────────────┤ │   │
│  │ai_summary_ │ │ topic │  │ hierarchy_config│ │   │
│  │  feeds     │ │ tags  │  │   _versions     │ │   │
│  ├───────────┤  │       │  ├─────────────────┤ │   │
│  │ai_summary_ │ │       │  │ adopt_narrower_ │←┼───┘
│  │  topics    │─┼───→───┼──│   queues        │ │
│  └───────────┘  │       │  ├─────────────────┤ │
└─────────────────┘       │  │ multi_parent_   │ │
                          │  │   resolve_queues│ │
┌─────────────────┐       │  ├─────────────────┤ │
│   Narrative      │       │  │ abstract_tag_   │ │
│  ┌───────────┐  │  FK   │  │   update_queues │ │
│  │ narrative_ │──┼───→───┼──│                 │ │
│  │  boards    │  │ topic │  ├─────────────────┤ │
│  ├───────────┤  │ tags  │  │ hierarchy_      │ │
│  │ narrative_ │  │       │  │   pending_      │ │
│  │ summaries  │  │       │  │   changes       │ │
│  ├───────────┤  │       │  └─────────────────┘ │
│  │ board_    │  │       └─────────────────────┘
│  │ concepts  │  │
│  └───────────┘  │
└─────────────────┘

┌─────────────────┐
│ AI Infrastructure│
│  ┌───────────┐  │
│  │ai_providers│ │
│  ├───────────┤  │
│  │ ai_routes │  │
│  ├───────────┤  │
│  │ai_route_  │  │
│  │ providers │  │
│  ├───────────┤  │
│  │ai_call_   │  │
│  │  logs     │  │
│  ├───────────┤  │
│  │ai_settings│  │
│  ├───────────┤  │
│  │scheduler_ │  │
│  │  tasks    │  │
│  ├───────────┤  │
│  │otel_spans │  │
│  └───────────┘  │
└─────────────────┘
```

- 实线箭头 → 表示 FK 引用（源域引用目标域的表）
- `topic_tags` 是数据库枢纽，有 12 条入边（10 张表引用）
- `topic_tag_relations` 通过 `parent_id` 和 `child_id` 双线自引用 `topic_tags`
- 虚线边界表示域内的队列表与主表之间的逻辑归属

---

## 域级 ER 图

### Core（核心数据面）

```mermaid
erDiagram
    categories ||--o{ feeds : "category_id"
    categories ||--o{ user_preferences : "category_id"
    feeds ||--o{ articles : "feed_id"
    feeds ||--o{ reading_behaviors : "feed_id"
    feeds ||--o{ user_preferences : "feed_id"
    articles ||--o{ firecrawl_jobs : "article_id"
    articles ||--o{ tag_jobs : "article_id"
    articles ||--o{ reading_behaviors : "article_id"

    categories {
        SERIAL id PK
        VARCHAR name
        VARCHAR slug
        VARCHAR icon
        VARCHAR color
        TEXT description
    }
    feeds {
        SERIAL id PK
        INTEGER category_id FK
        VARCHAR title
        VARCHAR url
        INTEGER refresh_interval
        BOOLEAN firecrawl_enabled
        BOOLEAN article_summary_enabled
    }
    articles {
        SERIAL id PK
        INTEGER feed_id FK
        VARCHAR title
        TEXT content
        TEXT firecrawl_content
        TEXT ai_content_summary
        VARCHAR firecrawl_status
        VARCHAR summary_status
    }
    firecrawl_jobs {
        SERIAL id PK
        INTEGER article_id FK
        VARCHAR status
        INTEGER attempt_count
    }
    tag_jobs {
        SERIAL id PK
        INTEGER article_id FK
        VARCHAR status
        VARCHAR reason
    }
    reading_behaviors {
        SERIAL id PK
        INTEGER article_id FK
        INTEGER feed_id FK
        VARCHAR event_type
    }
    user_preferences {
        SERIAL id PK
        INTEGER feed_id FK
        INTEGER category_id FK
        FLOAT preference_score
    }
```

### Topic Tags（主题标签面）

```mermaid
erDiagram
    topic_tags ||--o{ topic_tag_embeddings : "topic_tag_id"
    topic_tags ||--o{ topic_tag_relations : "parent_id"
    topic_tags ||--o{ topic_tag_relations : "child_id"
    topic_tags ||--o{ article_topic_tags : "topic_tag_id"
    topic_tags ||--o{ embedding_queues : "tag_id"
    topic_tags ||--o{ merge_reembedding_queues : "source_tag_id"
    topic_tags ||--o{ merge_reembedding_queues : "target_tag_id"
    topic_tags ||--o{ topic_tag_analyses : "topic_tag_id"
    topic_tags ||--o{ topic_analysis_cursors : "topic_tag_id"
    topic_tags ||--o{ topic_analysis_jobs : "topic_tag_id"
    topic_tags ||--o{ topic_tags : "merged_into_id"
    articles ||--o{ article_topic_tags : "article_id"

    topic_tags {
        SERIAL id PK
        VARCHAR slug
        VARCHAR label
        VARCHAR category
        VARCHAR status
        INTEGER merged_into_id FK
    }
    topic_tag_embeddings {
        SERIAL id PK
        INTEGER topic_tag_id FK
        vector embedding
        INTEGER dimension
        VARCHAR model
    }
    topic_tag_relations {
        SERIAL id PK
        INTEGER parent_id FK
        INTEGER child_id FK
        VARCHAR relation_type
        FLOAT similarity_score
    }
    article_topic_tags {
        SERIAL id PK
        INTEGER article_id FK
        INTEGER topic_tag_id FK
        FLOAT score
    }
    embedding_queues {
        BIGSERIAL id PK
        BIGINT tag_id FK
        VARCHAR status
    }
    merge_reembedding_queues {
        BIGSERIAL id PK
        BIGINT source_tag_id FK
        BIGINT target_tag_id FK
        VARCHAR status
    }
    topic_tag_analyses {
        BIGSERIAL id PK
        BIGINT topic_tag_id FK
        VARCHAR analysis_type
        VARCHAR window_type
    }
    topic_analysis_cursors {
        BIGSERIAL id PK
        BIGINT topic_tag_id FK
        BIGINT last_summary_id
    }
    topic_analysis_jobs {
        VARCHAR id PK
        BIGINT topic_tag_id FK
        VARCHAR status
    }
```

### AI Summaries（AI 摘要面）

```mermaid
erDiagram
    feeds ||--o{ ai_summaries : "feed_id"
    categories ||--o{ ai_summaries : "category_id"
    ai_summaries ||--o{ ai_summary_topics : "summary_id"
    ai_summaries ||--o{ articles : "feed_summary_id"
    topic_tags ||--o{ ai_summary_topics : "topic_tag_id"

    ai_summaries {
        BIGSERIAL id PK
        BIGINT feed_id FK
        BIGINT category_id FK
        VARCHAR title
        TEXT summary
        TEXT key_points
        BIGINT article_count
    }
    ai_summary_feeds {
        BIGSERIAL id PK
        BIGINT summary_id
        BIGINT feed_id
        VARCHAR feed_title
    }
    ai_summary_topics {
        BIGSERIAL id PK
        BIGINT summary_id FK
        BIGINT topic_tag_id FK
        NUMERIC score
    }
```

### Narrative（叙事摘要面）

```mermaid
erDiagram
    topic_tags ||--o{ narrative_boards : "abstract_tag_id"
    board_concepts ||--o{ narrative_boards : "board_concept_id"
    topic_tags ||--o{ board_concepts : "concept_id"
    narrative_boards ||--o{ narrative_summaries : "board_id"

    narrative_boards {
        SERIAL id PK
        VARCHAR name
        TEXT description
        TEXT event_tag_ids "JSON array"
        TEXT abstract_tag_ids "JSON array"
        INTEGER abstract_tag_id FK
        INTEGER board_concept_id FK
        BOOLEAN is_system
    }
    narrative_summaries {
        BIGSERIAL id PK
        VARCHAR title
        TEXT summary
        VARCHAR status
        VARCHAR period
        INTEGER board_id FK
        TEXT related_tag_ids "JSON array"
        TEXT related_article_ids "JSON array"
    }
    board_concepts {
        SERIAL id PK
        VARCHAR name
        TEXT description
        vector embedding
        BOOLEAN is_active
    }
```

### Hierarchy（层级关系面）

```mermaid
erDiagram
    topic_tags ||--o{ adopt_narrower_queues : "abstract_tag_id"
    topic_tags ||--o{ multi_parent_resolve_queues : "child_tag_id"
    topic_tags ||--o{ abstract_tag_update_queues : "abstract_tag_id"
    topic_tags ||--o{ hierarchy_pending_changes : "tag_id"
    topic_tags ||--o{ hierarchy_pending_changes : "current_parent_id"

    hierarchy_config ||--o{ hierarchy_config_versions : "config_id"

    hierarchy_config {
        BIGSERIAL id PK
        JSONB templates
        BIGINT version
    }
    hierarchy_config_versions {
        BIGSERIAL id PK
        BIGINT config_id FK
        BIGINT version
        JSONB templates
        TEXT change_log
    }
    adopt_narrower_queues {
        SERIAL id PK
        BIGINT abstract_tag_id FK
        VARCHAR source
        VARCHAR status
    }
    multi_parent_resolve_queues {
        SERIAL id PK
        INTEGER child_tag_id FK
        VARCHAR source
        VARCHAR status
    }
    abstract_tag_update_queues {
        BIGSERIAL id PK
        BIGINT abstract_tag_id FK
        VARCHAR trigger_reason
        VARCHAR status
    }
    hierarchy_pending_changes {
        SERIAL id PK
        INTEGER tag_id FK
        VARCHAR change_type
        INTEGER current_parent_id FK
        VARCHAR status
    }
```

### AI Infrastructure（AI 基础设施）

```mermaid
erDiagram
    ai_routes ||--o{ ai_route_providers : "route_id"
    ai_providers ||--o{ ai_route_providers : "provider_id"

    ai_providers {
        SERIAL id PK
        VARCHAR name
        VARCHAR provider_type
        VARCHAR base_url
        VARCHAR model
        BOOLEAN enabled
    }
    ai_routes {
        SERIAL id PK
        VARCHAR name
        VARCHAR capability
        VARCHAR strategy
    }
    ai_route_providers {
        SERIAL id PK
        INTEGER route_id FK
        INTEGER provider_id FK
        INTEGER priority
    }
    ai_call_logs {
        SERIAL id PK
        VARCHAR capability
        VARCHAR route_name
        VARCHAR provider_name
        BOOLEAN success
        INTEGER latency_ms
    }
    ai_settings {
        SERIAL id PK
        VARCHAR key
        TEXT value
    }
    scheduler_tasks {
        SERIAL id PK
        VARCHAR name
        VARCHAR status
        INTEGER check_interval
    }
    otel_spans {
        BIGSERIAL id PK
        CHAR trace_id
        CHAR span_id
        VARCHAR name
        BIGINT kind
        BIGINT status_code
        BIGINT start_time_unix_nano
        BIGINT end_time_unix_nano
    }
```

---

## FK 引用矩阵

| source_table | fk_column | target_table | target_column | constraint_name |
|---|---|---|---|---|
| `feeds` | `category_id` | `categories` | `id` | `fk_categories_feeds` |
| `articles` | `feed_id` | `feeds` | `id` | `fk_feeds_articles` |
| `articles` | `feed_summary_id` | `ai_summaries` | `id` | `articles_feed_summary_id_fkey` |
| `ai_summaries` | `category_id` | `categories` | `id` | `fk_ai_summaries_category` |
| `ai_summaries` | `feed_id` | `feeds` | `id` | `fk_ai_summaries_feed` |
| `ai_summary_topics` | `summary_id` | `ai_summaries` | `id` | `fk_ai_summaries_summary_topics` |
| `ai_summary_topics` | `topic_tag_id` | `topic_tags` | `id` | `fk_ai_summary_topics_topic_tag` |
| `article_topic_tags` | `article_id` | `articles` | `id` | `fk_article_topic_tags_article` |
| `article_topic_tags` | `topic_tag_id` | `topic_tags` | `id` | `fk_article_topic_tags_topic_tag` |
| `topic_tags` | `merged_into_id` | `topic_tags` | `id` | `fk_topic_tags_merged_into` |
| `topic_tags` | `concept_id` | `board_concepts` | `id` | `topic_tags_concept_id_fkey` |
| `topic_tag_embeddings` | `topic_tag_id` | `topic_tags` | `id` | `fk_topic_tags_embedding` |
| `topic_tag_relations` | `parent_id` | `topic_tags` | `id` | `topic_tag_relations_parent_id_fkey` |
| `topic_tag_relations` | `child_id` | `topic_tags` | `id` | `topic_tag_relations_child_id_fkey` |
| `topic_analysis_jobs` | `topic_tag_id` | `topic_tags` | `id` | (FK inferred) |
| `ai_route_providers` | `route_id` | `ai_routes` | `id` | `fk_ai_routes_route_providers` |
| `ai_route_providers` | `provider_id` | `ai_providers` | `id` | `fk_ai_route_providers_provider` |
| `reading_behaviors` | `article_id` | `articles` | `id` | `fk_reading_behaviors_article` |
| `reading_behaviors` | `feed_id` | `feeds` | `id` | `fk_reading_behaviors_feed` |
| `user_preferences` | `feed_id` | `feeds` | `id` | `fk_user_preferences_feed` |
| `user_preferences` | `category_id` | `categories` | `id` | `fk_user_preferences_category` |
| `firecrawl_jobs` | `article_id` | `articles` | `id` | `fk_firecrawl_jobs_article` |
| `tag_jobs` | `article_id` | `articles` | `id` | `fk_tag_jobs_article` |
| `embedding_queues` | `tag_id` | `topic_tags` | `id` | `embedding_queue_tag_id_fkey` |
| `merge_reembedding_queues` | `source_tag_id` | `topic_tags` | `id` | `merge_reembedding_queues_source_tag_id_fkey` |
| `merge_reembedding_queues` | `target_tag_id` | `topic_tags` | `id` | `merge_reembedding_queues_target_tag_id_fkey` |
| `adopt_narrower_queues` | `abstract_tag_id` | `topic_tags` | `id` | `fk_adopt_narrower_queues_abstract_tag` |
| `multi_parent_resolve_queues` | `child_tag_id` | `topic_tags` | `id` | `fk_mprq_child_tag` |
| `abstract_tag_update_queues` | `abstract_tag_id` | `topic_tags` | `id` | `fk_abstract_tag_update_queues_abstract_tag` |
| `hierarchy_pending_changes` | `tag_id` | `topic_tags` | `id` | `hierarchy_pending_changes_tag_id_fkey` |
| `hierarchy_pending_changes` | `current_parent_id` | `topic_tags` | `id` | `hierarchy_pending_changes_current_parent_id_fkey` |
| `narrative_boards` | `abstract_tag_id` | `topic_tags` | `id` | `narrative_boards_abstract_tag_id_fkey` |
| `narrative_boards` | `board_concept_id` | `board_concepts` | `id` | `narrative_boards_board_concept_id_fkey` |
| `narrative_summaries` | `board_id` | `narrative_boards` | `id` | `fk_narrative_summaries_board` |

---

## 关系模式说明

### 桥接表（Many-to-Many）

- **`article_topic_tags`**：连接 `articles` ↔ `topic_tags`，桥接表 + 关联评分
- **`ai_summary_topics`**：连接 `ai_summaries` ↔ `topic_tags`
- **`ai_summary_feeds`**：连接 `ai_summaries` ↔ `feeds`（含快照字段）
- **`ai_route_providers`**：连接 `ai_routes` ↔ `ai_providers`，附带优先级

### 自引用（Self-Referential FK）

- **`topic_tags.merged_into_id`** → `topic_tags.id`：标签合并后指向目标标签
- **`topic_tag_relations.parent_id` / `child_id`** → `topic_tags.id`：层级父子关系

### 反规范化（Denormalized）

- **`ai_call_logs`**：存储 `route_name` 和 `provider_name`（冗余）以保留调用时的上下文快照，即使后续路由/供应商被修改或删除
- **`ai_summary_feeds`**：存储 `feed_title`、`feed_icon`、`feed_color` 摘要生成时的快照

### JSON-stored ID Lists（无 FK 约束的关系）

以下字段使用 JSON 数组存储关联 ID，不通过 FK 约束保证完整性：

- **`narrative_boards.event_tag_ids`** → `topic_tags.id`：关联的 event 标签
- **`narrative_boards.abstract_tag_ids`** → `topic_tags.id`：关联的抽象标签
- **`narrative_boards.prev_board_ids`** → `narrative_boards.id`：前日关联 Board
- **`narrative_summaries.parent_ids`** → `narrative_summaries.id`：父叙事
- **`narrative_summaries.related_tag_ids`** → `topic_tags.id`：关联标签
- **`narrative_summaries.related_article_ids`** → `articles.id`：关联文章
- **`ai_summaries.articles`** → `articles.id`：覆盖文章列表

---

## 更新日志

### 2026-05-14

- 初始版本：全局 ASCII 概览图、6 个业务域 Mermaid ER 图、35 行 FK 引用矩阵、关系模式说明

---

## 相关文档

- [数据库字段说明](DATABASE_FIELDS.md) — 38 张表的完整字段字典
- [数据生命周期](DATA_LIFECYCLE.md) — 6 条数据链路的状态字段流转
- [项目架构总览](../architecture/overview.md) — 系统架构全局视角
- [数据流](../architecture/data-flow.md) — 代码执行流和 API 调用链
