# 数据库文档全局约定（`tables/_conventions.md`）

> 适用于 `tables/` 下全部域文档与 `_index.md`。本文件是全局事实唯一权威；域文档只写域内事实。

## 阅读约定（全局事实）

1. **表名**：GORM 未自定义 `NamingStrategy`，默认蛇形 + 复数；非默认表名由 struct 的 `TableName()` 显式指定。本文档每张表的真实表名以迁移 SQL 与 struct 为准。
2. **外键（FK）几乎不存在于 DB 层**：`gorm.Config` 设了 `DisableForeignKeyConstraintWhenMigrating: true`，且迁移 `20260601_0001` 主动 drop 了历史上全部 `fk_*`。**全库真实 DB FK 仅 3 处**：`topic_tags_merged_into_id_fkey`，完整 6 条清单见下方「DB 级外键（全库共 6 条）」。本文档中凡是写「关联 / FK」的地方，除特别注明外均为 GORM 逻辑关联，**不**对应 DB 级外键约束。
3. **向量列维度运行时决定**：除 `topic_tag_embeddings.embedding` 由迁移固定为 `vector(4096)` 外，其余向量列 gorm tag 仅声明 `type:vector`，实际维度由运行时 `embedding_config.embedding_dimension` / `ensureXxxEmbeddingDimension` 设置；HNSW 索引仅当维度 ≤ 2000 时创建。
4. **字段表列含义**：「类型」= DB 列类型；「约束/默认/索引」= NOT NULL / DEFAULT / 索引 / 唯一 / CHECK 等；「用途」= 业务含义。`string` 无 `size` 时 GORM 默认 256。
5. **枚举**：除非特别注明「+ CHECK」，枚举取值均为代码层约定，DB 层不强制。

---

## FK 真相（必读）

**本文档历史上把所有 GORM 关联都画成了「FK 约束」，这是系统性的误导。** 真相如下（代码权威）：

1. **GORM 全局关闭外键迁移**：`backend-go/internal/platform/database/db.go` 设置 `DisableForeignKeyConstraintWhenMigrating: true`。
2. **迁移 `20260601_0001` 主动 DROP 了历史上 `embedding_queues` / `merge_reembedding_queues` / `topic_tag_embeddings` / `topic_tag_relations` / `topic_tags` 的全部 `fk_*` 约束**（`postgres_migrations.go:472-520`）。
3. **全库真实存在的 DB 级外键共 6 条**（历史版本曾误写 1/2/3 条，2026-09 逐条核对迁移代码后修正；完整清单见下方「DB 级外键（全库共 6 条）」权威表）。
4. 其余所有「关联」都是 **GORM 应用层逻辑关联**（struct 的 `foreignKey` tag），DB 层**没有**对应的外键约束，`constraint_name` 形如 `fk_feeds_articles`、`fk_categories_feeds` **在数据库里并不存在**。
5. **级联删除（OnDelete:CASCADE）声明很不一致**：只有部分 GORM tag 声明了 `constraint:OnDelete:CASCADE`，很多关联没有任何级联声明。详见下方 [FK 引用矩阵](#fk-引用矩阵)。

> 结论：下文 Mermaid 图中的 `FK` 字样和关系连线，除特别标注外，**均表示 GORM 逻辑关联，不是 DB 物理外键**。ER 图描述的是「业务上的引用关系」，DB 完整性实际由应用层负责。

---

## 索引与约束总览（全局）

### pgvector 扩展（迁移 `20260403_0001`）

`CREATE EXTENSION IF NOT EXISTS vector`

### 全文检索（迁移 `20260417_0002`）

- ~~`articles.search_vector`（tsvector）+ GIN 索引 + 触发器~~（已删除，2026-08-20，零使用；列保留）

### 性能索引（迁移 `20260417_0001`）

| 索引名 | 表 | 列 |
| -------- | ------ | ------ |
| `idx_articles_read` | articles | `(read)` |
| `idx_articles_favorite` | articles | `(favorite)` |
| `idx_articles_feed_pub_date` | articles | `(feed_id, pub_date DESC)` |
| `idx_articles_feed_id_title` | articles | `(feed_id, title)` |
| `idx_article_topic_tags_article_id` | article_topic_tags | `(article_id)` |
| `idx_feeds_category_id` | feeds | `(category_id)` |

### 向量唯一索引

| 索引名 | 表 | 列 | 迁移 |
| -------- | ------ | ------ | ------ |
| `idx_topic_tag_embeddings_tag_type_hash` | topic_tag_embeddings | UNIQUE `(topic_tag_id, embedding_type, text_hash)` | `20260514_0001` |

> 注：`topic_tag_embeddings.embedding` 无独立 HNSW 迁移语句；运行时 HNSW 仅作用于 `daily_report_sections.embedding` 与 `board_persistent_topics.embedding`（dim ≤ 2000 才建）。

### 唯一约束（表级）

| 约束名 | 表 | 列 |
| -------- | ------ | ------ |
| `uq_section_relations_pair` | daily_report_section_relations | UNIQUE `(from_section_id, to_section_id, relation_type)` |
| `uq_board_upgrade_suggestions_hash` | board_upgrade_suggestions | 部分唯一 `(suggestion_hash) WHERE status='pending'` |

### CHECK 约束（迁移添加，DB 层强制）

| 约束名 | 表 | 表达式 | 迁移 |
| -------- | ------ | ------ | ------ |
| `chk_board_persistent_topics_status` | board_persistent_topics | `status IN ('candidate','active','archived')` | `20260619_0001` |
| `chk_board_persistent_topics_source` | board_persistent_topics | `source IN ('auto','manual')` | `20260702_0001` |
| `chk_board_topic_watches_status` | board_topic_watches | `status IN ('active','paused')` | `20260630_0001` |
| `chk_board_topic_watches_type` | board_topic_watches | `type IN ('label','keyword')` | `20260824_0002` |

### DB 级外键（全库共 6 条，权威清单）

| 约束名 | 表.列 → 引用 | ON DELETE | 迁移 |
| -------- | ------ | ------ | ------ |
| `topic_tags_merged_into_id_fkey` | `topic_tags.merged_into_id → topic_tags(id)` | CASCADE | `20260601_0001` |
| `fk_topic_watch_hits_watch` | `topic_watch_hits.watch_id → board_topic_watches(id)` | CASCADE | `20260801_0002` |
| `fk_topic_tag_embeddings_tag` | `topic_tag_embeddings.topic_tag_id → topic_tags(id)` | CASCADE | `20260820_0001` |
| `fk_board_topic_watches_topic` | `board_topic_watches.persistent_topic_id → board_persistent_topics(id)` | SET NULL | `20260825_0001` |
| `fk_topic_enrichment_result_parent_board` | `topic_enrichment_result(parent_result_id, semantic_board_id) → topic_enrichment_result(id, semantic_board_id)`（复合） | RESTRICT | `20260828_0001` |
| `fk_composite_components_composite` | `composite_components.composite_id → semantic_labels(id)` | CASCADE | `20260902_0001` |

> 其余所有表间关联均为 GORM 逻辑关联，**DB 层未强制**（`DisableForeignKeyConstraintWhenMigrating: true`）。

---

## 向量维度规则总述

| 表.列 | 维度来源 | 说明 |
| -------- | ------ | ------ |
| `topic_tag_embeddings.embedding` | **迁移固定 4096** | 唯一维度固定的向量列（迁移 `20260403_0003`） |
| `semantic_labels.embedding` | 运行时 | `auxlabel.EnsureVectorDimensionOnce` 按 `embedding_config.embedding_dimension` 设置 |
| `semantic_labels.merge_embedding` | 运行时 | 同上 |
| `daily_report_sections.embedding` | 运行时 | `ensureSectionEmbeddingDimension` |
| `daily_report_threads.embedding` | 运行时 | （随分区维度） |
| `board_persistent_topics.embedding` | 运行时 | `ensurePersistentTopicEmbeddingDimension` |
| `preference_vectors.embedding` | 运行时 | 偏好画像重算时按 `topic_tag_embeddings` semantic 轨维度写入（入库记 `dimension`/`model`，粗筛前校验一致） |
| `route_embeddings.embedding` | 运行时 | 路由向量，与偏好向量同空间（入库记 `dimension`/`model`） |

> HNSW 索引仅当维度 ≤ 2000 时创建（pgvector 限制）；> 2000 时跳过 HNSW，仅保留向量列。

---

## FK 引用矩阵

> **关键纠正**：本矩阵历史上的 30 行 `constraint_name`（如 `fk_feeds_articles`、`fk_categories_feeds`）**绝大多数在 DB 层并不存在**。下面拆成两部分——「真实 DB 级外键」与「GORM 应用层逻辑关联」。

### Part A — 真实 DB 级外键约束

即上方「DB 级外键（全库共 6 条，权威清单）」，不再重复维护两份。最早一条（`topic_tags_merged_into_id_fkey`，自引用，用于话题合并）由 `postgres_migrations.go` 显式 `ADD CONSTRAINT` 创建；迁移 `20260601_0001` 先 DROP 了所有历史 `fk_*` 后仅重建此 1 条，其余 5 条由后续版本迁移逐条补齐。

### Part B — GORM 应用层逻辑关联（foreignKey tag，DB 层未强制外键）

以下关联由 GORM struct 的 `foreignKey` tag 定义，是**业务引用关系**，**DB 层无对应外键约束**。按是否声明 `OnDelete:CASCADE` 分组。

#### B-1. 声明了 OnDelete:CASCADE 的逻辑关联（14 条）

> 即便声明了 CASCADE，也只是 GORM 在应用层操作时的语义约定；DB 层仍无物理 FK。

| source_table | column | target_table | target_column | gorm 关联定义出处 |
| --- | --- | --- | --- | --- |
| `feeds` | `category_id` | `categories` | `id` | `Category.Feeds` (category.go:17) |
| `articles` | `feed_id` | `feeds` | `id` | `Feed.Articles` (feed.go:29) |
| `firecrawl_jobs` | `article_id` | `articles` | `id` | `FirecrawlJob.Article` (job_queue.go:28) |
| `tag_jobs` | `article_id` | `articles` | `id` | `TagJob.Article` (job_queue.go:52) |
| `topic_tag_embeddings` | `topic_tag_id` | `topic_tags` | `id` | `TopicTagEmbedding.TopicTag` (topic_graph.go:130) |
| `article_topic_tags` | `article_id` | `articles` | `id` | `ArticleTopicTag.Article` (topic_graph.go:166) |
| `article_topic_tags` | `topic_tag_id` | `topic_tags` | `id` | `ArticleTopicTag.TopicTag` (topic_graph.go:167) |
| `topic_tag_semantic_labels` | `topic_tag_id` | `topic_tags` | `id` | `TopicTagSemanticLabel.TopicTag` (semantic_label.go:40) |
| `topic_tag_semantic_labels` | `semantic_label_id` | `semantic_labels` | `id` | `TopicTagSemanticLabel.SemanticLabel` (semantic_label.go:41) |
| `topic_tag_board_labels` | `topic_tag_id` | `topic_tags` | `id` | `TopicTagBoardLabel.TopicTag` (semantic_label.go:58) |
| `topic_tag_board_labels` | `semantic_board_id` | `semantic_labels` | `id` | `TopicTagBoardLabel.SemanticBoard` (semantic_label.go:59) |
| `board_composition` | `board_id` | `semantic_labels` | `id` | `BoardComposition.Board` (semantic_label.go:70) |
| `board_composition` | `auxiliary_label_id` | `semantic_labels` | `id` | `BoardComposition.AuxiliaryLabel` (semantic_label.go:71) |
| `topic_watch_hits` | `watch_id` | `board_topic_watches` | `id` | `BoardTopicWatch.Hits` (daily_report_models.go:413) |

#### B-2. 无 OnDelete 声明的逻辑关联（仅 GORM foreignKey，无级联）

| source_table | column | target_table | target_column | 备注 |
| --- | --- | --- | --- | --- |
| `reading_behaviors` | `article_id` | `articles` | `id` | 无 OnDelete |
| `reading_behaviors` | `feed_id` | `feeds` | `id` | 无 OnDelete |
| `user_preferences` | `feed_id` | `feeds` | `id` | 无 OnDelete |
| `user_preferences` | `category_id` | `categories` | `id` | 无 OnDelete |
| `embedding_queues` | `tag_id` | `topic_tags` | `id` | 历史 DB FK 已被迁移 DROP |
| `merge_reembedding_queues` | `source_tag_id` | `topic_tags` | `id` | 历史 DB FK 已被迁移 DROP |
| `merge_reembedding_queues` | `target_tag_id` | `topic_tags` | `id` | 历史 DB FK 已被迁移 DROP |
| `topic_tag_analyses` | `topic_tag_id` | `topic_tags` | `id` | 无 OnDelete |
| `topic_analysis_cursors` | `topic_tag_id` | `topic_tags` | `id` | 无 OnDelete |
| `ai_route_providers` | `route_id` | `ai_routes` | `id` | 无 OnDelete |
| `ai_route_providers` | `provider_id` | `ai_providers` | `id` | 无 OnDelete |
| `board_daily_reports` | `semantic_board_id` | `semantic_labels` | `id` | 无 OnDelete |
| `board_daily_reports` | `prev_report_id` | `board_daily_reports` | `id` | 自引用，可空，无 OnDelete |
| `daily_report_sections` | `report_id` | `board_daily_reports` | `id` | 无 OnDelete（`BoardDailyReport.Sections`） |
| `daily_report_sections` | `persistent_topic_id` | `board_persistent_topics` | `id` | 可空，无 OnDelete |
| `daily_report_threads` | `report_id` | `board_daily_reports` | `id` | 无 OnDelete |
| `daily_report_threads` | `section_id` | `daily_report_sections` | `id` | 无 OnDelete（`DailyReportSection.Threads`） |
| `daily_report_section_relations` | `from_section_id` | `daily_report_sections` | `id` | 多对多，无 OnDelete |
| `daily_report_section_relations` | `to_section_id` | `daily_report_sections` | `id` | 多对多，无 OnDelete |
| `board_persistent_topics` | `semantic_board_id` | `semantic_labels` | `id` | 无 OnDelete |
| `board_topic_watches` | `semantic_board_id` | `semantic_labels` | `id` | 无 OnDelete |
| `topic_watch_hits` | `section_id` | `daily_report_sections` | `id` | 无 OnDelete |
| `topic_watch_hits` | `report_id` | `board_daily_reports` | `id` | 无 OnDelete |
| `board_data_sources` | `semantic_board_id` | `semantic_labels` | `id` | 无 OnDelete |
| `topic_lifeline_context` | `persistent_topic_id` | `board_persistent_topics` | `id` | 无 OnDelete |
| `topic_enrichment_result` | `persistent_topic_id` | `board_persistent_topics` | `id` | 无 OnDelete |
| `topic_enrichment_review` | `persistent_topic_id` | `board_persistent_topics` | `id` | 无 OnDelete |
| `topic_enrichment_review` | `curr_result_id` | `topic_enrichment_result` | `id` | 无 OnDelete |
| `topic_enrichment_review` | `prev_result_id` | `topic_enrichment_result` | `id` | 可空，无 OnDelete |
| `stock_debate_result` | `topic_enrichment_result_id` | `topic_enrichment_result` | `id` | 无 OnDelete |
| `stock_debate_result` | `persistent_topic_id` | `board_persistent_topics` | `id` | 无 OnDelete |
| `board_upgrade_suggestions` | `target_board_id` | `semantic_labels` | `id` | 可空，无 OnDelete |

> **单向不对称说明**：部分关联的 CASCADE 仅在「父→子」切片侧声明（如 `Feed.Articles` 有 CASCADE，而反向的 `Article.Feed` 指针无 `constraint` tag）。由于 DB 层无物理 FK，这只影响 GORM 的应用层删除行为，不构成 DB 级不一致。

---

## 关系模式说明

### 桥接表（Many-to-Many，无 id，复合主键）

以下三张桥接表**均无 `id` 列**，使用复合主键（代码权威）：

- **`topic_tag_semantic_labels`**：连接 `topic_tags` ↔ `semantic_labels`（auxiliary）。复合主键 `(topic_tag_id, semantic_label_id)`，无 `created_at`，两端 `OnDelete:CASCADE`。
- **`topic_tag_board_labels`**：连接 `topic_tags` ↔ `semantic_labels`（board），含 `score` / `match_reason` / `downgraded` / `direction_mismatch`。复合主键 `(topic_tag_id, semantic_board_id)`，两端 `OnDelete:CASCADE`。
- **`board_composition`**：连接 `semantic_labels`（board）↔ `semantic_labels`（auxiliary）。复合主键 `(board_id, auxiliary_label_id)`，无时间戳列，两端 `OnDelete:CASCADE`。

> ⚠️ 旧版本文档曾把这三张表写成 `BIGSERIAL id PK`，与代码不符，已纠正。

### 带 id 的桥接/明细表

- **`article_topic_tags`**：连接 `articles` ↔ `topic_tags`，桥接表 + 关联评分，**有 `id`**，两端 `OnDelete:CASCADE`。
- **`ai_route_providers`**：连接 `ai_routes` ↔ `ai_providers`，附带 `priority`，**有 `id`**，无 OnDelete。
- **`daily_report_section_relations`**：连接 `daily_report_sections` ↔ `daily_report_sections`（跨日分区多对多），**有 `id`**，区分 `relation_type`（similarity / identity），唯一约束 `(from_section_id, to_section_id, relation_type)`，无 OnDelete。

### 自引用（Self-Referential）

- **`topic_tags.merged_into_id` → `topic_tags.id`**：话题合并，**唯一真实 DB FK**（`ON DELETE CASCADE`）。
- **`semantic_labels`**：辅助标签和 SemanticBoard 共存于同一张表，通过 `label_type` 区分（非外键自引用）。
- **`board_daily_reports.prev_report_id → board_daily_reports.id`**：前日报告链，可空，逻辑关联。

### 反规范化（Denormalized，无 FK 约束）

- **`ai_call_logs`**：存储 `route_name` 和 `provider_name`（冗余）以保留调用时的上下文快照，即使后续路由/供应商被修改或删除。
- **`board_upgrade_suggestions.auxiliary_label_ids`**（JSONB `[]uint`）：逻辑指向 `semantic_labels.id`（auxiliary），不保证完整性。
- **`daily_report_threads.tag_ids` / `related_article_ids`**（JSONB）：逻辑指向 `topic_tags.id` / `articles.id`，无 FK。

### JSON-stored ID Lists（无 FK 约束的关系）

以下字段使用 JSON 数组存储关联 ID，不通过 FK 约束保证完整性：


### 已废弃表（无对应 model）

以下表在代码中无 GORM model，不再 AutoMigrate，仅作历史结构保留：

- `ai_summaries` / `ai_summary_feeds` / `ai_summary_topics`：AI 摘要旧体系
- `topic_analysis_jobs`：旧分析任务表
- `digest_configs`：旧摘要配置表

---

## 相关文档

- [数据库域文档索引](../_index.md) — 52 张业务表按域分档的字段字典与 ER 图
- [数据生命周期](DATA_LIFECYCLE.md) — 数据链路的状态字段流转
- [项目架构总览](../architecture/overview.md) — 系统架构全局视角
- [业务流程](../flow/README.md) — 链路概要设计、函数调用链、前后端协作
- [数据库审计报告](../_audit/database-gaps.md) — 文档与代码差异的逐项核查记录
