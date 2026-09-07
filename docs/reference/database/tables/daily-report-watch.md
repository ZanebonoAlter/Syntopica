# 日报 / 持久话题 / Watch 域（`tables/daily-report-watch.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


> 本域 7 张表由 `internal/topicgraph` 的 `RegisterModels` 注册（`init()`），部署若未引入该包则表不存在；迁移对它们用 `tableExists` 守卫，缺失时安全跳过。

### 9.1 board_daily_reports（板块日报主表）

每天每板块一条日报。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `semantic_board_id` | INTEGER | NOT NULL; index `idx_board_daily_reports_semantic_board_id` | 所属语义板块 |
| `period_date` | DATE | NOT NULL | 周期日期 |
| `title` | VARCHAR(256) | — | 日报标题 |
| `summary` | VARCHAR(256) | — | 日报摘要 |
| `highlights` | JSONB | — | 要点 |
| `dynamics` | TEXT | — | 动态 |
| `article_count` | INTEGER | — | 文章数 |
| `event_tag_count` | INTEGER | — | 事件标签数 |
| `cluster_count` | INTEGER | — | 聚类数 |
| `status` | VARCHAR(20) | DEFAULT 'generating' | 状态（`generating` / 完成） |
| `raw_clusters` | JSONB | — | 原始聚类 |
| `prev_report_id` | INTEGER | —（`*uint`） | 前一日报告 ID |
| `generation_prompt_version` | VARCHAR(20) | — | 生成 prompt 版本 |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

`Sections` 关联为逻辑关联（无 OnDelete）。

### 9.2 daily_report_sections（日报分区）

一分区 = 一聚类。承载归属持久话题的字段。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `report_id` | INTEGER | NOT NULL; index `idx_daily_report_sections_report_id` | 所属日报 |
| `cluster_index` | INTEGER | — | 聚类序号 |
| `cluster_label` | VARCHAR(200) | — | 聚类标签 |
| `cluster_tag_ids` | JSONB | — | 聚类标签 ID 列表 |
| `article_count` | INTEGER | — | 文章数 |
| `best_tier` | INTEGER | DEFAULT 0 | 最佳层级 |
| `avg_score` | FLOAT | DEFAULT 0 | 平均分 |
| `quality_breakdown` | JSONB | —（迁移 `20260625_0001`） | 质量分解 |
| `embedding` | vector | —（运行时维度，迁移 `20260601_0002`） | 聚类向量 |
| `persistent_topic_id` | INTEGER | —（`*uint`，**刻意无 NOT NULL**）; index | 归属持久话题 ID（容忍回填窗口） |
| `topic_match_distance` | FLOAT | — | 归属匹配距离 |
| `topic_match_confidence` | VARCHAR(20) | — | 归属置信度：`anchor_hit` / `auto_new` / `unmatched` / **`manual`** |
| `topic_status_at_report` | VARCHAR(20) | —（`*string`，可空，迁移 `20260627_0001`） | 报告生成时话题状态快照（`candidate` / `active` / NULL） |
| `lane_tier` | VARCHAR(16) | —（迁移 `20260727_0001`） | 泳道归属标记：`l1_direct`（质心强挂直归属）/ `l2_llm`（弱区 LLM 留/换）/ `l3_new`（新开 candidate）。NULL = 迁移前历史 section（旧流程产出，不回刷） |
| `created_at` | TIMESTAMP | — | 创建时间 |

> HNSW 索引 `idx_daily_report_sections_embedding`：运行时由 `ensureSectionEmbeddingDimension` 创建（dim ≤ 2000 才建）。
> **瞬态字段（`gorm:"-"`，非持久化）**：`PersistentTopic`、`MatchedTopicID`。
> **已删列**：`threads`（JSONB，迁移 `20260529_0003` 迁移到独立表后 drop）、`prev_section_id`、`status`（迁移 `20260603_0001`）。

### 9.3 daily_report_threads（日报叙事线程）

从分区独立出来的叙事线程（迁移 `20260529_0002` 从 sections.threads JSONB 迁出）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `report_id` | INTEGER | NOT NULL; index `idx_daily_report_threads_report_id` | 所属日报 |
| `section_id` | INTEGER | NOT NULL; index `idx_daily_report_threads_section_id` | 所属分区 |
| `title` | VARCHAR(256) | — | 线程标题 |
| `summary` | VARCHAR(256) | — | 线程摘要 |
| `tag_ids` | JSONB | — | 关联标签 ID 列表 |
| `confidence` | FLOAT | DEFAULT 0 | 置信度 |
| `related_article_ids` | JSONB | — | 关联文章 ID 列表 |
| `embedding` | vector | —（运行时维度） | 线程向量 |
| `fit_distance` | FLOAT | —（`*float64`，**无 default**，刻意区分 nil 与 0.0） | 与所属分区的契合距离（nil = 无信号，0.0 = 完美契合） |
| `created_at` | TIMESTAMP | — | 创建时间 |

> **已删列**：`status`、`prev_thread_id`（迁移 `20260603_0001`）。

### 9.4 daily_report_section_relations（跨日分区关系，多对多）

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `from_section_id` | INTEGER | NOT NULL; index `idx_section_relations_from` | 起点分区 |
| `to_section_id` | INTEGER | NOT NULL; index `idx_section_relations_to` | 终点分区 |
| `distance` | FLOAT | NOT NULL | 距离 |
| `relation_type` | VARCHAR(20) | NOT NULL DEFAULT 'similarity'; index `idx_section_relations_type` | 关系：`similarity`（匈牙利时间线匹配）/ `identity`（持久话题连续性） |
| `created_at` | TIMESTAMP | — | 创建时间 |

**唯一约束（迁移 `20260620_0001`，三列宽化）**：`uq_section_relations_pair UNIQUE(from_section_id, to_section_id, relation_type)` —— 允许同一分区对的 identity 边与 similarity 边并存。

### 9.5 board_persistent_topics（板块持久叙事话题）

板块内持久叙事框架。一个板块 N 个话题，每个 section 归属一个话题（1:N）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `semantic_board_id` | INTEGER | NOT NULL; 复合索引 `idx_persistent_topics_board_status(semantic_board_id, status)` priority:1 | 所属语义板块 |
| `label` | VARCHAR(200) | NOT NULL | 持久叙事标题 |
| `description` | TEXT | — | 描述 |
| `embedding` | vector | —（运行时维度） | 归属匹配与历史回刷聚类向量 |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'candidate'; 复合索引同上 priority:2; **CHECK** `chk_board_persistent_topics_status (status IN ('candidate','active','archived'))` | 状态：`candidate`（观察中）/ `active`（连续命中后晋升）/ `archived`（衰减归档） |
| `source` | VARCHAR(10) | NOT NULL DEFAULT 'auto'; **CHECK** `chk_board_persistent_topics_source (source IN ('auto','manual'))` | 来源：`auto`（算法聚类）/ `manual`（用户手动建泳道，绕过 candidate 直接 active） |
| `first_seen_date` | DATE | NOT NULL | 首次命中日期 |
| `last_seen_date` | DATE | NOT NULL | 最近命中日期 |
| `hit_count` | INTEGER | NOT NULL DEFAULT 1 | 总命中数 |
| `consecutive_hits` | INTEGER | NOT NULL DEFAULT 0 | 连续命中天数 |
| `centroid` | vector | —（`default:NULL`，迁移 `20260727_0001`；运行时维度） | 近 `persistent_topic_centroid_window`（默认 30）条 section embedding 的均权平均，作为 lane 分桶与归属的**匹配锚点**（取代旧首义向量 `embedding`）；NULL 时运行时退化首义 |
| `is_vacuum` | BOOLEAN | NOT NULL DEFAULT false（迁移 `20260727_0001`） | 吸尘器标记：`strong/(strong+mid) < persistent_topic_vacuum_ratio`（默认 0.20）则 true——质心过宽、沾边 tag 都被吸成最近邻，挂到它的 tag 从 L1 降级 L2 交 LLM 裁决 |
| `vacuum_strong` | INTEGER | NOT NULL DEFAULT 0（迁移 `20260727_0001`） | 近 `persistent_topic_vacuum_window`（默认 7 天）归属该 topic 且 `topic_match_distance < 0.18` 的 section 计数（吸尘器统计快照） |
| `vacuum_mid` | INTEGER | NOT NULL DEFAULT 0（迁移 `20260727_0001`） | 近窗口归属该 topic 且 distance ∈ [0.18, 0.30] 的 section 计数（吸尘器统计快照） |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

> HNSW 索引 `idx_board_persistent_topics_embedding`：运行时由 `ensurePersistentTopicEmbeddingDimension` 创建（dim ≤ 2000 才建）。
>
> **质心维度**：`centroid` 同 `embedding` 用无维度 `vector` 声明，运行时由 `ensurePersistentTopicEmbeddingDimension` 同步维度。迁移 `20260727_0001` 离线用 pgvector `avg(vector)` 回填近 30 条 section 均权平均；回填失败（pgvector 版本不支持）则留 NULL，运行时 `ComputeTopicCentroid` 退化首义向量，不阻断。

**归属与生命周期（算法语义）**：`daily_report_sections` 通过 `persistent_topic_id` / `topic_match_distance` / `topic_match_confidence` / `lane_tier` 记录归属。归属由**泳道分桶**（lane-driven）决定，非旧双重确认 AND-gate：当天 tag 按到 topic **质心**的余弦距离分 L1/L2/L3 三桶（见 `flow/daily-report.md`）。`topic_match_confidence` 四态：`anchor_hit`（L1 直挂或 L2 LLM 留/换命中既有 topic）/ `auto_new`（L3 或 L2 换/新→新开 candidate）/ `unmatched`（section 无 embedding，无法分桶）/ `manual`（用户手动建泳道覆盖归属，非算法三态）。`lane_tier` 记录归属来源泳道（`l1_direct`/`l2_llm`/`l3_new`），历史 section（迁移前）为 NULL 视为旧流程数据、不回刷。`manual` 态由手动建泳道事务写入，前端独立样式区分。`topic_status_at_report` 与归属在同事务写入，不随后续状态回填，历史数据统一 NULL。

**排序与窗口边界**：可锚定话题选择器（`ListAnchorableTopicsByBoard`）选出全部 active 及 `last_seen_date` 在 `persistent_topic_candidate_decay_window`（默认 7 天）内的 candidate，按 `last_seen_date DESC, hit_count DESC, id ASC` 排序，candidate 最多保留 `persistent_topic_candidate_prompt_limit`（默认 20）条。`candidate_decay_window` 仅用于 prompt 卫生过滤，不触发任何状态变更；所有 status → archived 仅由用户在话题管理界面手动操作。

**候选展示门槛**：`consecutive_hits < upgrade_threshold`（默认 3）的 candidate（"observing"）在话题管理 UI 中隐藏，但仍持久化并参与可锚定集合。达门槛后自动可见。

**一次性清理迁移 `20260628_0001`**：幂等硬删所有 `status=candidate AND consecutive_hits < upgrade_threshold` 的历史 candidate。删除采用 `DeleteTopic` 语义：先将引用 section 的 `persistent_topic_id` / `topic_match_distance` / `topic_match_confidence` / `topic_status_at_report` 置 NULL（section 内容保留，仍可渲染为"其他动态"独立节点），再硬删 candidate 行，最后按 board 重建 relations 以清除指向已删 topic 的 identity/similarity 边。不可逆但 section 内容完整保留。第二次执行是 no-op。

### 9.6 board_topic_watches（用户声明的话题 Watch 标签）

版块上用户声明的 Watch 标签。与持久话题刻意独立：无共享 FK、无共享生命周期；命中始终是只读覆盖层，不影响任何 topic 状态。`type=label` 是日报结束时的 AI 单信号检测；`type=keyword` 是 threads 标题+摘要的确定性文本匹配，并在创建时回扫近 14 天历史日报。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `semantic_board_id` | INTEGER | NOT NULL; index | 所属语义版块 |
| `label` | VARCHAR(200) | NOT NULL | label 关注文本或 keyword 表达式 |
| `type` | VARCHAR(10) | NOT NULL DEFAULT 'label'; **CHECK** `chk_board_topic_watches_type (type IN ('label','keyword'))`（迁移 `20260824_0002`） | 命中判定轨：`label`（AI）/ `keyword`（文本） |
| `status` | VARCHAR(20) | NOT NULL DEFAULT 'active'; **CHECK** `chk_board_topic_watches_status (status IN ('active','paused'))` | 状态：`active` / `paused` |
| `created_at` | TIMESTAMP | — | 创建时间 |
| `updated_at` | TIMESTAMP | — | 更新时间 |

`Hits` 关联声明 `constraint:OnDelete:CASCADE`（GORM 层）。

### 9.7 topic_watch_hits（Watch 命中记录）

label 类 AI 或 keyword 类文本匹配得到的 Watch 与日报分区的匹配。只读覆盖层，**不得**改变任何 section 的 `persistent_topic_id` 或任何 topic 状态；keyword 的 `reason` 固定为「含关键字『…』」。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `watch_id` | INTEGER | NOT NULL; 复合唯一 `idx_watch_section_report`; **FK** `fk_topic_watch_hits_watch → board_topic_watches(id) ON DELETE CASCADE`（迁移 `20260801_0002`） | 关联 Watch ID |
| `section_id` | INTEGER | NOT NULL; 复合唯一同上 | 命中分区 |
| `report_id` | INTEGER | NOT NULL; 复合唯一同上 | 所属日报 |
| `period_date` | DATE | NOT NULL | 周期日期 |
| `reason` | TEXT | — | 命中理由 |
| `created_at` | TIMESTAMP | — | 创建时间 |

复合唯一索引：`idx_watch_section_report (watch_id, section_id, report_id)`（gorm tag + 迁移 `20260630_0002` 双重声明）。

---


## 索引

### 叙事域索引（迁移 `20260420_0001` / `20260430_0001`）

| 索引名 | 表 | 列 |
| -------- | ------ | ------ |

### 日报 / 持久话题 / Watch 域索引

| 索引名 | 表 | 列 | 迁移 |
| -------- | ------ | ------ | ------ |
| `idx_board_daily_reports_semantic_board_id` | board_daily_reports | `(semantic_board_id)` | `20260526_0001` |
| `idx_daily_report_sections_report_id` | daily_report_sections | `(report_id)` | `20260526_0001` |
| `idx_daily_report_threads_report_id` | daily_report_threads | `(report_id)` | `20260529_0001` |
| `idx_daily_report_threads_section_id` | daily_report_threads | `(section_id)` | `20260529_0001` |
| `idx_section_relations_from` | daily_report_section_relations | `(from_section_id)` | gorm tag |
| `idx_section_relations_to` | daily_report_section_relations | `(to_section_id)` | gorm tag |
| `idx_section_relations_type` | daily_report_section_relations | `(relation_type)` | `20260619_0001` |
| `idx_persistent_topics_board_status` | board_persistent_topics | `(semantic_board_id, status)` | `20260619_0001` |
| `idx_watch_section_report` | topic_watch_hits | UNIQUE `(watch_id, section_id, report_id)` | `20260630_0002` |
| `idx_board_upgrade_suggestions_status` | board_upgrade_suggestions | `(status)` | `20260717_0001` |


## 域 ER 图

> 实线关系 = GORM 逻辑关联；标注「真实DB FK」的边 = DB 物理外键（共 6 条，全清单见 [_conventions.md](_conventions.md)）。外域邻居表仅标名，字段见其所属域文档。

```mermaid
erDiagram
    board_daily_reports ||--o{ board_daily_reports : "prev_report_id (自引用, 可空)"
    board_daily_reports ||--o{ daily_report_sections : "report_id"
    board_daily_reports ||--o{ topic_watch_hits : "report_id"
    board_persistent_topics ||--o{ daily_report_sections : "persistent_topic_id (可空)"
    board_persistent_topics ||--o{ stock_debate_result : "persistent_topic_id"
    board_persistent_topics ||--o{ topic_enrichment_result : "persistent_topic_id"
    board_persistent_topics ||--o{ topic_enrichment_review : "persistent_topic_id"
    board_persistent_topics ||--o{ topic_lifeline_context : "persistent_topic_id"
    board_topic_watches ||--o{ topic_watch_hits : "watch_id (OnDelete:CASCADE)" %% 真实DB FK
    daily_report_sections ||--o{ daily_report_section_relations : "from_section_id"
    daily_report_sections ||--o{ daily_report_section_relations : "to_section_id"
    daily_report_sections ||--o{ daily_report_threads : "section_id"
    daily_report_sections ||--o{ topic_watch_hits : "section_id"
    semantic_labels ||--o{ board_daily_reports : "semantic_board_id"
    semantic_labels ||--o{ board_persistent_topics : "semantic_board_id"
    semantic_labels ||--o{ board_topic_watches : "semantic_board_id"

    board_daily_reports {
        SERIAL id PK
        VARCHAR status
    }
    board_persistent_topics {
        SERIAL id PK
        VARCHAR status "candidate|active|archived"
    }
    board_topic_watches {
        SERIAL id PK
        VARCHAR status "active|paused"
    }
    daily_report_section_relations {
        SERIAL id PK
    }
    daily_report_sections {
        SERIAL id PK
    }
    daily_report_threads {
        SERIAL id PK
    }
    semantic_labels {
    }
    stock_debate_result {
    }
    topic_enrichment_result {
    }
    topic_enrichment_review {
    }
    topic_lifeline_context {
    }
    topic_watch_hits {
        SERIAL id PK
    }
```

