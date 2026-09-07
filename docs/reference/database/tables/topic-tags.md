# 主题标签域（`tables/topic-tags.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


### 4.1 topic_tags（主题标签主表）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `slug` | VARCHAR(120) | NOT NULL; 复合索引 `idx_topic_tags_category_slug(category, slug)` | 稳定标识 |
| `label` | VARCHAR(160) | NOT NULL | 展示名称 |
| `category` | VARCHAR(20) | NOT NULL DEFAULT 'keyword'; 复合索引同上 | 分类：`event` / `person` / `keyword` |
| `icon` | VARCHAR(100) | — | Iconify 图标 ID |
| `aliases` | TEXT | — | 别名列表（JSON 数组） |
| `description` | TEXT | — | LLM 生成的标签描述 |
| `is_canonical` | BOOLEAN | DEFAULT false | 是否为规范标签 |
| `source` | VARCHAR(20) | DEFAULT 'llm' | 来源：`llm` / `heuristic` / `manual` |
| `feed_count` | INTEGER | DEFAULT 0 | 引用此标签的不重复 Feed 数；打标路径不增量维护，由 TagQualityScoreJob 周期对账重算 |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'active'; index | 状态：`active` / `merged` |
| `merged_into_id` | INTEGER | index | 合并目标标签 ID（**唯一真实 DB FK**：`topic_tags_merged_into_id_fkey ... ON DELETE CASCADE`） |
| `is_watched` | BOOLEAN | DEFAULT false | 是否为用户关注标签 |
| `watched_at` | TIMESTAMP | —（`*time.Time`） | 关注时间 |
| `quality_score` | FLOAT | DEFAULT 0 | 质量评分 |
| `metadata` | JSONB | DEFAULT '{}'（serializer:json） | 扩展元数据 |
| `kind` | VARCHAR(20) | DEFAULT 'keyword' | **已废弃**，映射到 `category`，保留兼容 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

> ⚠️ `(category, slug)` 是**普通复合索引**（gorm tag `index:idx_topic_tags_category_slug`，**非 unique**），不强制唯一。
> ⚠️ 旧文档曾列 `concept_id`：迁移 `20260522_0001` 已 `DROP COLUMN`，struct 无此字段，请勿引用。

### 4.2 topic_tag_embeddings（主题标签向量）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `topic_tag_id` | INTEGER | NOT NULL; 复合唯一 `idx_topic_tag_embeddings_tag_type_hash`; **FK `fk_topic_tag_embeddings_tag` → topic_tags.id ON DELETE CASCADE（迁移 `20260820_0001`）** | 关联标签 ID |
| `embedding_type` | VARCHAR(20) | NOT NULL DEFAULT 'identity'; 复合唯一同上 | 嵌入类型：`identity` / `semantic` / `event_keyword` |
| `embedding` | **vector(4096)** | — | pgvector 向量列（**迁移 `20260403_0003` 固定 4096 维**；struct 字段名 `embedding_vec`，列名 `embedding`） |
| `dimension` | INTEGER | NOT NULL | 向量维度 |
| `model` | VARCHAR(50) | NOT NULL | 生成模型名称 |
| `text_hash` | VARCHAR(64) | 复合唯一同上 | 标签文本哈希，参与唯一约束（同一 tag+type 可有多行不同 text_hash） |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

唯一约束：`idx_topic_tag_embeddings_tag_type_hash (topic_tag_id, embedding_type, text_hash)`（gorm tag + 迁移 `20260514_0001` 双重声明）。

> ⚠️ `topic_tag_embeddings.embedding` 是唯一维度固定的向量列（4096）。旧文档曾写 `vector(1536)`，错误。
> ⚠️ 旧文档曾列 `vector`（TEXT 旧版 JSON 向量）：迁移 `20260601_0001b` 已 `DROP COLUMN`，请勿引用。
> `TopicTagEmbedding.TopicTag` 声明了 `constraint:OnDelete:CASCADE`（GORM 层）；DB 层 FK 由迁移 `20260820_0001` 补齐（此前仅 GORM 声明、DB 无约束，删 tag 后向量残留成孤儿，2026-08-20 已清理 25.6 万孤儿行）。

### 4.3 topic_tag_analyses（主题分析快照）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | BIGSERIAL | PK | 主键 |
| `topic_tag_id` | BIGINT | 复合唯一 `idx_tag_analysis_date` | 关联标签 ID |
| `analysis_type` | string(256) | 复合唯一同上 | 分析类型：`event` / `person` / `keyword` |
| `window_type` | string(256) | 复合唯一同上 | 时间窗：`daily` / `weekly` |
| `anchor_date` | TIMESTAMP | 复合唯一同上 | 锚点日期 |
| `article_count` | INTEGER | — | 覆盖的文章数量 |
| `payload_json` | TEXT | — | 分析结果 JSON |
| `source` | string(256) | — | 来源：`ai` / `heuristic` / `cached` |
| `version` | INTEGER | — | 分析版本号 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

唯一约束：`idx_tag_analysis_date (topic_tag_id, analysis_type, window_type, anchor_date)`。

> ⚠️ 旧文档曾列 `summary_count`：真实列名是 **`article_count`**。

### 4.4 topic_analysis_cursors（主题分析游标）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | BIGSERIAL | PK | 主键 |
| `topic_tag_id` | BIGINT | 复合唯一 `idx_cursor_tag_type_window` | 关联标签 ID |
| `analysis_type` | string(256) | 复合唯一同上 | 分析类型 |
| `window_type` | string(256) | 复合唯一同上 | 时间窗 |
| `last_article_id` | BIGINT | — | 上次分析已处理到的最大文章 ID |
| `last_updated_at` | TIMESTAMP | — | 上次刷新时间 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

唯一约束：`idx_cursor_tag_type_window (topic_tag_id, analysis_type, window_type)`。

> ⚠️ 旧文档曾列 `last_summary_id`：真实列名是 **`last_article_id`**。

### 4.5 article_topic_tags（文章-主题关联）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `article_id` | INTEGER | NOT NULL; index `idx_article_topic_tag_article`; 复合唯一 `idx_article_topic_tags_link` | 文章 ID |
| `topic_tag_id` | INTEGER | NOT NULL; index `idx_article_topic_tag_topic`; 复合唯一同上 | 标签 ID |
| `score` | FLOAT | DEFAULT 0 | 相关度评分 |
| `source` | VARCHAR(20) | DEFAULT 'llm' | 来源 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

唯一约束：`idx_article_topic_tags_link (article_id, topic_tag_id)`。两端关联均声明 `constraint:OnDelete:CASCADE`（GORM 层）。

> 标签任务写入关联前会在短事务内以 `FOR KEY SHARE` 锁定对应文章。若 Feed 清理已删除该文章，则跳过关联写入并正常完成任务。
> 单列索引：`idx_article_topic_tags_article_id(article_id)`（迁移 `20260417_0001`）。

### 4.6 topic_tag_relations（标签层级关系）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `parent_id` | INTEGER | NOT NULL; 复合唯一 `idx_tag_relation_pair` | 父标签 ID（逻辑关联 `topic_tags.id`，无 DB FK） |
| `child_id` | INTEGER | NOT NULL; 复合唯一同上 | 子标签 ID |
| `relation_type` | VARCHAR(20) | NOT NULL DEFAULT 'abstract' | 关系类型：`abstract` / `synonym` / `related` |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

唯一约束：`idx_tag_relation_pair (parent_id, child_id)`。两端关联均为逻辑关联（无 OnDelete）。

### 4.7 tag_merge_suggestions（标签合并建议）

记录一对相似标签供人工合并决策。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `new_tag_id` | INTEGER | NOT NULL; 复合唯一 `idx_tag_merge_suggestion_pair` | 新标签 ID |
| `existing_tag_id` | INTEGER | NOT NULL; 复合唯一同上 | 既有标签 ID |
| `new_label` | VARCHAR(160) | NOT NULL | 新标签名快照 |
| `existing_label` | VARCHAR(160) | NOT NULL | 既有标签名快照 |
| `category` | VARCHAR(20) | NOT NULL | 分类 |
| `similarity` | FLOAT | NOT NULL; 复合索引 `idx_tag_merge_suggestion_status_sim(status, similarity)` | 相似度 |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'pending'; 复合索引同上 | 状态：`pending` / `merged` / `dismissed` |
| `source` | VARCHAR(20) | NOT NULL DEFAULT 'incremental' | 来源：`incremental` / `full_scan` |
| `llm_verdict` | TEXT | — | LLM 判定文本 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

---


## 域 ER 图

> 实线关系 = GORM 逻辑关联；标注「真实DB FK」的边 = DB 物理外键（共 6 条，全清单见 [_conventions.md](_conventions.md)）。外域邻居表仅标名，字段见其所属域文档。

```mermaid
erDiagram
    articles ||--o{ article_topic_tags : "article_id"
    topic_tags ||--o{ ai_summary_topics : "topic_tag_id (已废弃)"
    topic_tags ||--o{ article_topic_tags : "topic_tag_id"
    topic_tags ||--o{ embedding_queues : "tag_id"
    topic_tags ||--o{ merge_reembedding_queues : "source_tag_id"
    topic_tags ||--o{ merge_reembedding_queues : "target_tag_id"
    topic_tags ||--o{ topic_analysis_cursors : "topic_tag_id"
    topic_tags ||--o{ topic_tag_analyses : "topic_tag_id"
    topic_tags ||--o{ topic_tag_board_labels : "tag side"
    topic_tags ||--o{ topic_tag_board_labels : "topic_tag_id"
    topic_tags ||--o{ topic_tag_embeddings : "topic_tag_id" %% 真实DB FK
    topic_tags ||--o{ topic_tag_semantic_labels : "tag side"
    topic_tags ||--o{ topic_tag_semantic_labels : "topic_tag_id"

    ai_summary_topics {
    }
    article_topic_tags {
        SERIAL id PK
        INTEGER article_id FK
        INTEGER topic_tag_id FK
    }
    articles {
    }
    embedding_queues {
    }
    merge_reembedding_queues {
    }
    topic_analysis_cursors {
        BIGSERIAL id PK
        BIGINT topic_tag_id FK
    }
    topic_tag_analyses {
        BIGSERIAL id PK
        BIGINT topic_tag_id FK
    }
    topic_tag_board_labels {
    }
    topic_tag_embeddings {
        SERIAL id PK
        INTEGER topic_tag_id FK
    }
    topic_tag_semantic_labels {
    }
    topic_tags {
        SERIAL id PK
        VARCHAR status
        INTEGER merged_into_id "自引用→topic_tags.id（唯一真实 DB FK）"
    }
```

