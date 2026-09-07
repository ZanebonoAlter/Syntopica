# 语义标签 / 板块域（`tables/semantic-labels.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


### 5.1 semantic_labels（语义标签统一表）

辅助标签（`label_type=auxiliary`）和 SemanticBoard（`label_type=board`）共存于同一张表，通过 `label_type` 区分。

> 向量保留策略（2026-08-20 起）：`status='disabled'` 的行 `embedding` / `merge_embedding` 置 NULL（行本体与 aliases 保留），所有禁用路径（API 删除 board / DisableAuxiliaryLabel / 别名合并 / 批量软删 / 更新接口）同步置 NULL；重新启用由 backfill / llm_extract 重算向量。存量 disabled 向量已一次性清理（7.3 万行，~1.7 GB）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `label` | VARCHAR(160) | NOT NULL | 展示名称 |
| `slug` | VARCHAR(**160**) | NOT NULL; **单列唯一** `idx_semantic_labels_slug` | 稳定标识 |
| `embedding` | vector | —（运行时维度） | 语义向量（struct `*string`，列名 `embedding`） |
| `merge_embedding` | vector | —（运行时维度） | 合并去重用向量（struct `*string`，列名 `merge_embedding`） |
| `label_type` | VARCHAR(20) | NOT NULL; index `idx_semantic_labels_label_type` | 类型：`auxiliary` / `board` / `composite`（约定，无 CHECK） |
| `aliases` | JSONB | DEFAULT '[]'（serializer:json） | 别名列表 |
| `ref_count` | INTEGER | NOT NULL DEFAULT 0 | 引用计数（辅助标签被 tag 引用次数） |
| `description` | TEXT | — | 描述 |
| `display_order` | INTEGER | NOT NULL DEFAULT 0 | 显示排序 |
| `source` | VARCHAR(**50**) | NOT NULL DEFAULT 'llm_extract' | 来源：`llm_extract` / `llm_suggest` / `manual`（约定） |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'active'; index `idx_semantic_labels_status` | 状态：`active` / `disabled` |
| `protected` | BOOLEAN | NOT NULL DEFAULT false | 是否受保护（不可自动删除） |
| `enrichment_enabled` | BOOLEAN | NOT NULL DEFAULT false | 循环 B 增强开关（默认关，耗资源需先绑数据源） |
| `window_days` | INTEGER | NOT NULL DEFAULT 14 | 循环 B 实时详情窗口天数（范围 1-365） |
| `context_layers` | JSONB | DEFAULT '["week","month","year","all"]' | 解读员读取的分层粒度配置 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

> ⚠️ `slug` 约束是**单列 UNIQUE**（迁移 `20260521_0001` `UNIQUE(slug)`）。旧文档曾写「唯一约束 `(label_type, slug)`」，错误。
> ⚠️ `embedding` / `merge_embedding` 均为运行时维度向量列（旧文档曾写 `vector(1536)`，错误）。

### 5.2 topic_tag_semantic_labels（tag-辅助标签关联，中间表）

纯关联表，**无 `id` 列，无时间戳**，复合主键。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `topic_tag_id` | BIGINT | PK; NOT NULL | 关联 tag ID（逻辑关联，GORM 声明 OnDelete:CASCADE） |
| `semantic_label_id` | BIGINT | PK; NOT NULL | 关联辅助标签 ID |

主键：`(topic_tag_id, semantic_label_id)`。索引：`idx_topic_tag_semantic_labels_topic_tag_id`、`idx_topic_tag_semantic_labels_semantic_label_id`（迁移 `20260521_0001`）。

> ⚠️ 旧文档曾列 `id BIGSERIAL PK`：代码无 `id`，复合主键。

### 5.3 topic_tag_board_labels（tag-SemanticBoard 匹配结果，中间表）

复合主键，无 `id`。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `topic_tag_id` | BIGINT | PK; NOT NULL | 关联 tag ID |
| `semantic_board_id` | BIGINT | PK; NOT NULL | 关联 SemanticBoard ID |
| `score` | FLOAT | NOT NULL DEFAULT 0 | 匹配分数 |
| `match_reason` | TEXT | —（自由文本，无 size/CHECK） | 匹配原因说明（如 `direct_hit` / `hit_rate` / `max_sim` / `weighted`，仅约定非强制） |
| `downgraded` | BOOLEAN | NOT NULL DEFAULT false | 是否被降级（命中后人工/规则下调） |
| `direction_mismatch` | BOOLEAN | NOT NULL DEFAULT false | 方向不匹配标记 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

主键：`(topic_tag_id, semantic_board_id)`。索引：`idx_topic_tag_board_labels_topic_tag_id`、`idx_topic_tag_board_labels_semantic_board_id`（迁移 `20260521_0001`）。

> ⚠️ 旧文档曾列 `id BIGSERIAL PK` 且 `match_reason VARCHAR(20)`：代码无 `id`，`match_reason` 为 `type:text` 自由文本。

### 5.4 board_composition（board 构成，中间表）

纯关联表，**无 `id` 列，无时间戳**，复合主键。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `board_id` | BIGINT | PK; NOT NULL | 关联 board ID（`label_type=board`） |
| `auxiliary_label_id` | BIGINT | PK; NOT NULL | 挂载单元 ID：`label_type=auxiliary` 辅助标签 或 `label_type=composite` 组合标签（add-composite-labels 起列语义复用） |

主键：`(board_id, auxiliary_label_id)`。索引：`idx_board_composition_board_id`、`idx_board_composition_auxiliary_label_id`（迁移 `20260521_0001`）。

> ⚠️ 旧文档曾列 `id BIGSERIAL PK`：代码无 `id`，复合主键。


### 5.4.1 composite_components（组合标签组件序列，中间表）

add-composite-labels 引入。纯关联表，复合主键，组件按 `position` 有序（顺序决定组合方向，如「美国国债×收益率」≠「收益率×美国国债」的语义侧重）。组件删除时经 FK ON DELETE CASCADE 级联清理。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `composite_id` | BIGINT | PK; NOT NULL; FK→semantic_labels(id) ON DELETE CASCADE | 组合标签 ID（`label_type=composite`） |
| `component_label_id` | BIGINT | PK; NOT NULL; FK→semantic_labels(id) ON DELETE CASCADE | 组件辅助标签 ID（`label_type=auxiliary`，active） |
| `position` | INTEGER | NOT NULL | 组件序号（1 起，有序） |

主键：`(composite_id, component_label_id)`。表由 AutoMigrate 从 `models.CompositeComponent` 创建，迁移 `20260902_0001` 兜底确保 FK 约束（cascade ensure）并 seed 三个 ai_settings（`composite_label_dedupe_sim=0.95`、`semantic_board_match_direct_hit_score_factor=0.7`、`semantic_board_upgrade_composite_min_cooccurrence=10`）。

### 5.5 board_upgrade_suggestions（板块升级建议）

每次板块升级生成批次产出的建议，带生命周期（pending → confirmed / dismissed）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `batch_id` | VARCHAR(64) | NOT NULL; index | 生成批次 |
| `mode` | VARCHAR(32) | NOT NULL | 模式：split-board-upgrade-directions 起为四格键 `create:aux` / `create:composite` / `expand:aux` / `expand:composite`（旧值 discover_new/expand_existing 不再产生） |
| `decision` | VARCHAR(32) | NOT NULL | 决策：`create_new` / `merge_into_existing` / `compose`；`watch` 已退役（生成侧不再产生，存量 pending 行由迁移 `20260905_0002` 一次性 DELETE 清理，枚举值仅为存量行 DTO 兼容保留） |
| `board_label` | VARCHAR(160) | NOT NULL | 板块标签 |
| `description` | TEXT | — | 描述 |
| `target_board_id` | INTEGER | —（`*uint`，可空） | 目标板块 ID（merge_into_existing 与扩充方向 compose 建议挂载目标） |
| `auxiliary_label_ids` | JSONB | DEFAULT '[]'（serializer:json） | 辅助标签 ID 列表 |
| `confidence` | VARCHAR(16) | NOT NULL DEFAULT 'llm' | 置信度：`high` / `llm` |
| `evidence` | JSONB | —（serializer:json） | 证据快照 `{shortlist, margins, cotag_events, lane_briefs}`；compose 建议带 `{source, compose_cooccurrence, compose_window_days, compose_representative_titles}` |
| `status` | VARCHAR(16) | NOT NULL DEFAULT 'pending'; index `idx_board_upgrade_suggestions_status` | 状态：`pending` / `confirmed` / `dismissed` |
| `dismiss_reason` | TEXT | —（`*string`，可空） | 驳回原因 |
| `suggestion_hash` | VARCHAR(64) | NOT NULL | 稳定指纹 `(mode, decision, target_board_id, sorted_auxiliary_label_ids)` |
| `resolved_at` | TIMESTAMP | —（`*time.Time`） | 处理时间 |
| `resolved_by` | VARCHAR(50) | —（`*string`） | 处理人 |
| `created_at` | TIMESTAMP | — | 创建时间 |

**部分唯一索引（迁移 `20260717_0001`）**：`uq_board_upgrade_suggestions_hash UNIQUE(suggestion_hash) WHERE status='pending'`，保证同簇同决策不重复插入 pending 行；dismissed 行同 hash 在重新生成时用于冷却复查。

---


## 索引

### 语义标签 / 板块域索引（迁移 `20260521_0001`）

| 索引名 | 表 | 列 |
| -------- | ------ | ------ |
| `idx_semantic_labels_slug` | semantic_labels | UNIQUE `(slug)` |
| `idx_semantic_labels_label_type` | semantic_labels | `(label_type)` |
| `idx_semantic_labels_status` | semantic_labels | `(status)` |
| `idx_topic_tag_semantic_labels_topic_tag_id` | topic_tag_semantic_labels | `(topic_tag_id)` |
| `idx_topic_tag_semantic_labels_semantic_label_id` | topic_tag_semantic_labels | `(semantic_label_id)` |
| `idx_topic_tag_board_labels_topic_tag_id` | topic_tag_board_labels | `(topic_tag_id)` |
| `idx_topic_tag_board_labels_semantic_board_id` | topic_tag_board_labels | `(semantic_board_id)` |
| `idx_board_composition_board_id` | board_composition | `(board_id)` |
| `idx_board_composition_auxiliary_label_id` | board_composition | `(auxiliary_label_id)` |


## 域 ER 图

> 实线关系 = GORM 逻辑关联；标注「真实DB FK」的边 = DB 物理外键（共 6 条，全清单见 [_conventions.md](_conventions.md)）。外域邻居表仅标名，字段见其所属域文档。

```mermaid
erDiagram
    semantic_labels ||--o{ board_composition : "board side"
    semantic_labels ||--o{ board_daily_reports : "semantic_board_id"
    semantic_labels ||--o{ board_data_sources : "semantic_board_id"
    semantic_labels ||--o{ board_persistent_topics : "semantic_board_id"
    semantic_labels ||--o{ board_topic_watches : "semantic_board_id"
    semantic_labels ||--o{ board_upgrade_suggestions : "target_board_id (可空)"
    semantic_labels ||--o{ composite_components : "component side (FK CASCADE)" %% 真实DB FK
    semantic_labels ||--o{ composite_components : "composite side (FK CASCADE)" %% 真实DB FK
    semantic_labels ||--o{ cross_board_relation_runs : "source_board_id (逻辑)"
    semantic_labels ||--o{ cross_board_relations : "source_board_id / target_board_id (逻辑)"
    semantic_labels ||--o{ topic_tag_board_labels : "board side"
    semantic_labels ||--o{ topic_tag_semantic_labels : "auxiliary label side"
    topic_tags ||--o{ topic_tag_board_labels : "tag side"
    topic_tags ||--o{ topic_tag_board_labels : "topic_tag_id"
    topic_tags ||--o{ topic_tag_semantic_labels : "tag side"
    topic_tags ||--o{ topic_tag_semantic_labels : "topic_tag_id"

    board_composition {
        BIGINT board_id PK "复合主键 (board_id, auxiliary_label_id)"
        BIGINT auxiliary_label_id PK "无 id / 无时间戳"
    }
    board_daily_reports {
    }
    board_data_sources {
    }
    board_persistent_topics {
    }
    board_topic_watches {
    }
    board_upgrade_suggestions {
        SERIAL id PK
        VARCHAR status "pending|confirmed|dismissed"
    }
    composite_components {
    }
    cross_board_relation_runs {
    }
    cross_board_relations {
    }
    semantic_labels {
        SERIAL id PK
        VARCHAR status
    }
    topic_tag_board_labels {
        BIGINT topic_tag_id PK "复合主键"
        BIGINT semantic_board_id PK "复合主键（无 id）"
        BIGINT topic_tag_id PK "复合主键 (topic_tag_id, semantic_board_id)"
        BIGINT semantic_board_id PK "无 id"
    }
    topic_tag_semantic_labels {
        BIGINT topic_tag_id PK "复合主键"
        BIGINT semantic_label_id PK "复合主键（无 id）"
        BIGINT topic_tag_id PK "复合主键 (topic_tag_id, semantic_label_id)"
        BIGINT semantic_label_id PK "无 id / 无 created_at"
    }
    topic_tags {
    }
```

