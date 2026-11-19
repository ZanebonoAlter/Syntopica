# 向量域（`tables/embeddings.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


### 6.1 embedding_config（向量配置，键值对）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `key` | VARCHAR(100) | UNIQUE NOT NULL; index | 配置键 |
| `value` | TEXT | NOT NULL | 配置值 |
| `description` | VARCHAR(200) | — | 说明 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

> ⚠️ **默认配置项已变更**：`high_similarity_threshold` / `low_similarity_threshold` / `embedding_dimension` / `embedding_model` 曾由迁移 `20260413_0002` seed，但已被迁移 `20260614_0001` **DELETE 清除**，现由运行时代码管理，不再 seed。`narrative_board_embedding_threshold` / `narrative_board_hotspot_threshold` 亦已删（迁移 `20260522_0001`，已废弃）。
> 当前仍 seed 的键：`event_cluster_kw_min_overlap`、`event_cluster_sem_threshold`（迁移 `20260514_0002`）。

### 6.2 embedding_queues（向量生成队列）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | BIGSERIAL | PK | 主键 |
| `tag_id` | BIGINT | NOT NULL; index | 关联标签 ID（逻辑关联 `topic_tags.id`，无 DB FK） |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'pending'; index | 状态：`pending` / `processing` / `completed` / `failed` |
| `error_message` | TEXT | — | 错误信息 |
| `retry_count` | INTEGER | DEFAULT 0 | 重试次数 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `started_at` | TIMESTAMP | —（`*time.Time`） | 开始时间 |
| `completed_at` | TIMESTAMP | —（`*time.Time`） | 完成时间 |

> 关联为逻辑关联（无 OnDelete，DB FK 已 drop）。
> 保留策略（2026-08-20 起）：`status='completed'` 且 `created_at` 早于 30 天的行由 `job_log_cleanup` 周期清理（部分索引 `idx_embedding_queues_completed_created` 支撑，迁移 `20260820_0002`）；`pending`/`processing`/`failed` 不受影响。

### 6.3 merge_reembedding_queues（合并后重算向量队列）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | BIGSERIAL | PK | 主键 |
| `source_tag_id` | BIGINT | NOT NULL; index | 源标签 ID（逻辑关联，无 DB FK） |
| `target_tag_id` | BIGINT | NOT NULL; index | 目标标签 ID（逻辑关联，无 DB FK） |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'pending'; index | 状态：`pending` / `processing` / `completed` / `failed` |
| `error_message` | TEXT | — | 错误信息 |
| `retry_count` | INTEGER | DEFAULT 0 | 重试次数 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `started_at` | TIMESTAMP | —（`*time.Time`） | 开始时间 |
| `completed_at` | TIMESTAMP | —（`*time.Time`） | 完成时间 |

### 6.4 related_event_embeddings（相关事件派生向量缓存）

> 能力 `related-events`（enable-story-cross-day-continuation）。**可丢弃索引**：每行都能由标签当前文本重新生成；删表只会让相关事件回到 `not_ready`，不影响任何业务身份。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| ------- | ------ | ------ | ------ |
| `topic_tag_id` | BIGINT | PK 组成；无 DB FK（派生缓存，随标签清理留孤儿行亦无害） | 事件标签（`topic_tags.id`） |
| `representation_version` | VARCHAR(40) | PK 组成 | 文本表示版本（当前 `related-event-v1`），布局变更必须换版本 |
| `space_key` | VARCHAR(220) | PK 组成 | 模型空间：`provider=<vendor>|model=<model>|gen=<generation>`（同维不同模型不混算） |
| `text_hash` | VARCHAR(64) | index | 文本签名（sha256）；与当前标签文本不一致的旧行**不参与**查询 |
| `dimension` | BIGINT | — | 向量维度（与源向量维度不一致的候选不算就绪） |
| `embedding` | VECTOR | — | 表示向量（**无维度限定**：列类型不承载维度；维度由实际路由模型决定，逐行存于 `dimension` 列，写入前校验两者一致——`related/refresh.go` 的 `validateVector`） |

> 向量维度契约（全库）：维度是运行时属性，schema 不承载（详见 [`_conventions.md`](_conventions.md)「向量维度规则总述」）。配置表 `embedding_config` 不持有任何维度键（迁移 `20260614_0001` 已删 `embedding_dimension`）。
| `created_at` / `updated_at` | TIMESTAMPTZ | — | 写入/刷新时间 |

- 主键 `(topic_tag_id, representation_version, space_key)`：三条失效轴 = 文本变化（`text_hash` 不匹配即失效）、表示版本升级、模型空间/显式 generation 变化。
- **唯一写入口**：`go run ./cmd/related-events --refresh --board <id>`（`internal/tagmanagement/service/related`）；GET `/semantic-boards/:id/tags/:tagId/related-events` 只读，不调模型、不写这张表。
- 不存合并关系、不存身份、不存相关边；相关对在查询时现算，因此**没有** `topic_tag_relations` 那类读端展开副作用。
- 不含 ai_embedding_cache 复用（operation `tagmanagement.related_event.embedding` 不在缓存白名单），每次刷新都是真实调用的审计行。

---


## 域 ER 图

> 实线关系 = GORM 逻辑关联；标注「真实DB FK」的边 = DB 物理外键（共 13 条，全清单见 [_conventions.md](_conventions.md)）。外域邻居表仅标名，字段见其所属域文档。

```mermaid
erDiagram
    topic_tags ||--o{ embedding_queues : "tag_id"
    topic_tags ||--o{ merge_reembedding_queues : "source_tag_id"
    topic_tags ||--o{ merge_reembedding_queues : "target_tag_id"
    topic_tags ||--o{ related_event_embeddings : "topic_tag_id"

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
    related_event_embeddings {
        BIGINT topic_tag_id PK
        VARCHAR representation_version PK
        VARCHAR space_key PK
        VARCHAR text_hash
        VECTOR embedding
    }
    topic_tags {
    }
```

