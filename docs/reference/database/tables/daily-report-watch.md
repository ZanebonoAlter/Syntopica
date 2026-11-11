# 日报 / 持久话题 / Watch 域（`tables/daily-report-watch.md`）

> 真相源 = 代码（GORM struct + `postgres_migrations.go`），本文件是投影。全局约定（FK 真相 / 向量维度 / 枚举 / 唯一与 CHECK 约束）见 [_conventions.md](_conventions.md)；完整表清单与导航见 [_index.md](../_index.md)。


> 本域 10 张表由 `internal/topicgraph` 的 `RegisterModels` 注册（`init()`），部署若未引入该包则表不存在；迁移对它们用 `tableExists` 守卫，缺失时安全跳过。

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

一分区 = 一聚类；域型泳道的一个聚类可拆成 2-6 个主题方向（兄弟分区共享 `cluster_index`，用 `bucket_index`/`bucket_title` 区分）。承载归属持久话题的字段。

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
| `lane_tier` | VARCHAR(16) | —（迁移 `20260727_0001`） | 泳道归属标记：`l1_direct`（质心强挂直归属）/ `l2_llm`（弱区 LLM 留/换）/ `l3_new`（新开 candidate）；物化轨另有 `watch_keyword` / `watch_sentence`。NULL = 迁移前历史 section（旧流程产出，不回刷） |
| `bucket_title` | VARCHAR(200) | 可空（迁移 `20261024_0001`） | 泳道内主题分桶的方向名（add-intra-lane-theme-bucketing）。NULL = 未分桶/历史行；不参与任何跨天统计 |
| `bucket_index` | INTEGER | NOT NULL DEFAULT 0（迁移 `20261024_0001`） | 同一 `cluster_index` 内的桶序号（0 起、组内唯一）。未分桶 = 0；`(report_id, cluster_index)` **刻意无唯一约束**（兄弟桶共享） |
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
| `related_article_ids` | JSONB | 保序、去重（首个为准） | 关联文章 ID 列表（逻辑引用 `articles.id`，无 FK）。**写入时只允许存在的 id**：日报生成写库前过一次存在性校验；空引用写 `[]`，**不得**写 JSON `null`（`jsonb_array_elements_text` 遇 null 抛 22023）。**删除路径必须维护**：`articles` 行被删时同事务内改指保留条或剔除（详见 `flow/daily-report.md` 约束 21） |
| `embedding` | vector | —（运行时维度） | 线程向量 |
| `fit_distance` | FLOAT | —（`*float64`，**无 default**，刻意区分 nil 与 0.0） | 与所属分区的契合距离（nil = 无信号，0.0 = 完美契合） |
| `story_id` | BIGINT | —（`*uint` 可空；index `daily_report_threads_story_idx`；迁移 `20261105_0001` 建真实 FK → `board_stories(id) ON DELETE SET NULL`） | **历史列（retire-story-adopt-event-metrics 起停写）**：新生成/重建的日报恒 NULL，历史值只读保留且读路径不再对外返回；事件线链路已停写，该列只作回滚与存量核对依据 |
| `scope` | TEXT | NOT NULL DEFAULT 'single'; **CHECK** `chk_daily_report_threads_scope IN ('single','composite')`（迁移 `20261105_0001`） | 由 `tag_ids` 中 event 标签数推导：≥2 = composite（综合稿，永不建/挂事件线），否则 single |
| `is_development` | BOOLEAN | NOT NULL DEFAULT false（迁移 `20261105_0001`） | **历史列**：「同一事件线不同阶段（进展）」标记；跨期去重已退役，新报不再写该语义（默认 false），只读保留 |
| `story_manual` | BOOLEAN | NOT NULL DEFAULT false（迁移 `20261105_0001`） | **历史列**：新报恒 false；历史人工标记只读保留，自动归属已停写故不再有「跳过人工标记」路径 |
| `display_tier` | TEXT | NOT NULL DEFAULT 'body'; **CHECK** `chk_daily_report_threads_display_tier IN ('body','flash','hidden')`（迁移 `20261105_0001`） | 展示层载体：**新报恒 body**（跨期去重退役，不再产生 flash）；历史 flash/hidden 只读保留，读路径仍只回 body。身份/signals/热度永不读该列 |
| `created_at` | TIMESTAMP | — | 创建时间 |

> **已删列**：`status`、`prev_thread_id`（迁移 `20260603_0001`）。

> **`related_article_ids` 完整性（heal-dangling-article-refs）**：该数组是「按 ID 逻辑引用文章」的唯一 jsonb 持有者（全库 jsonb 列盘点结论）。契约：保序、去重、空写 `[]`、禁 JSON `null` 标量；删除 `articles` 行的三条路径（去重归并 `mergeDuplicateArticleGroup`、删订阅源级联 `DeleteFeedCascade`/`DeleteArticlesByFeed`、未来的删行原语）MUST 在同一事务内先调 `internal/platform/articlerefs` 维护再删行；日报写入前 MUST 对全部候选 id（含 Step7.5 watch 物化 thread）做一次存在性校验。存量悬空由迁移 `20260917_0002` 一次性修复（不可逆、幂等）；生成时快照计数不回填。

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

### 9.8 topic_lane_snapshots（泳道滚动态势快照）

每活跃泳道一行的滚动 14 天态势快照（每日日报生成后异步结算，upsert 覆盖；纯派生缓存，可随时重建）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `persistent_topic_id` | INTEGER | NOT NULL; UNIQUE `idx_topic_lane_snapshots_topic`; **FK** `persistent_topic_id → board_persistent_topics(id) ON DELETE CASCADE`（迁移 `20260910_0001`） | 所属持久话题（泳道） |
| `rolling_summary` | TEXT | NOT NULL | ≤100 字短版态势句 |
| `rolling_detail` | TEXT | 可空（模型层空串=缺失） | ≤500 字长版成段叙述，与短版同窗同素材同一次结算生成；纯派生缓存——每日报结算 upsert 覆盖，无迁移动作（AutoMigrate 加列）；存量旧行空=合法，下个结算周期自愈补齐、不回填 |
| `as_of_date` | DATE | NOT NULL | 汇总截止（=最新报告期） |
### 9.9 topic_key_event_markers（话题关键事件注记）

人工主权的关键事件注记行：AI 只写 `suggested`，`confirmed`/`dismissed` 只能由用户动作产生；同版块同来源指纹唯一（重建重跑幂等）。来源指针可空——日报重生成删 thread/section 时 DB FK SET NULL 断开引用，确认快照保留（失效来源注记）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `semantic_board_id` | INTEGER | NOT NULL; UNIQUE `idx_topic_key_events_board_source(semantic_board_id, source_key)`; **FK** `→ semantic_labels(id) ON DELETE CASCADE`（迁移 `20261012_0001`） | 所属版块（boards=semantic_labels 行） |
| `source_key` | VARCHAR(64) | NOT NULL; 同上复合唯一 | 来源指纹：SHA-256(board\|topic\|上海日期\|空白归一标题\|排序去重 tag+article ID)；任何漂移=新注记 |
| `persistent_topic_id` | INTEGER | 复合索引 `idx_topic_key_events_topic_state`; **FK** `→ board_persistent_topics(id) ON DELETE SET NULL` | 所属话题（话题删除→失效快照注记） |
| `source_report_id` | INTEGER | index; **FK** `→ board_daily_reports(id) ON DELETE SET NULL` | 来源日报（导航入口；SET NULL 保快照） |
| `source_section_id` | INTEGER | **FK** `→ daily_report_sections(id) ON DELETE SET NULL` | 来源分区（重建后失配→NULL=失效快照） |
| `source_thread_id` | INTEGER | **FK** `→ daily_report_threads(id) ON DELETE SET NULL` | 来源线索（thread 寻址动作的目标） |
| `source_period_date` | DATE | NOT NULL; 复合索引同上 | 来源报告期（轴定位；快照冻结） |
| `topic_label_snapshot` | VARCHAR(200) | — | 确认时话题名快照 |
| `title_snapshot` | TEXT | — | 确认时线索标题快照（展示不得重写新闻） |
| `summary_snapshot` | TEXT | — | 确认时线索摘要快照 |
| `tag_ids_snapshot` | JSONB | — | 标签 ID 快照数组 |
| `article_ids_snapshot` | JSONB | — | 依据文章 ID 快照数组 |
| `state` | VARCHAR(16) | NOT NULL DEFAULT 'suggested'; 复合索引同上; **CHECK** `ck_topic_key_event_markers_state IN ('suggested','confirmed','dismissed')` | 三态；AI 只写 suggested |
| `recommendation_reason` | TEXT | —（AI 行才有；手动标记为空） | 建议理由（服务端校验候选与证据归属后才落库） |
| `recommendation_basis` | JSONB | —（同上） | `{"evidence_article_ids":[...],"new_progress":bool}`；证据必须 ⊆ 线索文章 |
| `last_action` | VARCHAR(16) | — | 最近动作（mark/dismiss/unmark 审计） |
| `confirmed_at` | TIMESTAMP | —（可空） | 确认时刻（信号材料回溯标记用） |
| `decided_at` | TIMESTAMP | —（可空） | 最近人工决定时刻 |
| `revision` | INTEGER | NOT NULL DEFAULT 1 | 乐观锁版本；409 冲突返回最新行 |
| `created_at` / `updated_at` | TIMESTAMP | — | 创建 / 更新时间 |

> 幂等语义（add-topic-key-event-markers design D1）：PUT/PATCH 同目标同状态重复提交直接返回已保存行；状态变更必须携带最新 `revision`，冲突 409 附服务端最新行；重建重跑按 `source_key` 精确匹配重挂来源指针，失配即断开为独立快照注记（不伪造 section/关系）。

### 9.10 key_event_recommendation_runs（推荐任务状态）

每报告一行的推荐任务状态（重建覆盖 state/token，迟到任务凭 token 失配整批丢弃）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | SERIAL | PK | 主键 |
| `report_id` | INTEGER | NOT NULL; UNIQUE `idx_key_event_runs_report` | 目标日报（一报告一行） |
| `semantic_board_id` | INTEGER | NOT NULL; index | 所属版块 |
| `state` | VARCHAR(16) | NOT NULL DEFAULT 'running'; **CHECK** `ck_key_event_recommendation_runs_state IN ('running','done','failed')` | running/done/failed；进程重启后无内存任务对应的 running 行读取降级为 failed |
| `generation_token` | VARCHAR(64) | NOT NULL | 生成代 token（uuid）：重建换 token，迟到批次对不上即丢弃 |
| `candidate_count` | INTEGER | — | 本轮候选线索数（≤40） |
| `suggested_count` | INTEGER | — | 本轮实际写/刷新 suggested 行数 |
| `error` | TEXT | —（可空） | 失败原因 |
| `created_at` / `updated_at` | TIMESTAMP | — | 创建 / 更新时间 |

### 9.11 board_stories（事件线，add-event-story-heat；**只读保留**）

> ⚠️ **本表自 change `retire-story-adopt-event-metrics`（2026-11-11）起只读保留**：在线生产已硬停写（归属入口与其调用已摘除，无 `ai_settings` 开关；review R-1），重建历史报也不再重算派生计数（review R-2），回填 CLI 写模式拒绝，不删表不迁移；下列字段/索引描述为历史口径与回滚（`git revert`）依据。现行「今日重点」用当天同事件聚簇（不落库），见 `flow/daily-report.md` 红线 46-47。

跨天同一事件的聚合体（一条线贯穿多天的多个 `daily_report_threads`），承载结构化指纹与热度汇总计数；热度证据在 `story_signals`，热度由 signals 实时计算不落本表。挂在版块下（`board_id`），可选关联首次归属话题（`lane_id` 仅展示分组用，不参与身份判定）。迁移 `20261105_0001`。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `id` | BIGSERIAL | PK | 主键 |
| `board_id` | BIGINT | NOT NULL; index `board_stories_subject_idx` (priority 1)；迁移建真实 FK → `semantic_labels(id)`（boards 是 `label_type='board'` 的 SemanticLabel 行）ON DELETE CASCADE | 所属版块 |
| `lane_id` | BIGINT | —（可空；FK ON DELETE SET NULL） | 首次归属话题（board_persistent_topics.id）；可空——话题归档/删除不影响身份 |
| `title` | TEXT | NOT NULL | 事件线标题 |
| `subject` | TEXT | — | 主体原文（展示用，可能是「特朗普」也可能是「Trump」） |
| `subject_key` | TEXT | —; index `board_stories_subject_idx` (priority 2) | 归一化实体键（`person:trump` / `company:bytedance` / `org:pboc` / `product:gpt-5` / `other:*`）；主体归一化主判据 |
| `action` | TEXT | — | 动作（发布/宣布/涨价/回应/下架）；阶段判定与反向动作表输入 |
| `object` | TEXT | — | 对象（产品/政策/资产/事项） |
| `occurred_at` | TIMESTAMPTZ | —（可空） | 事件发生时间 |
| `status` | TEXT | NOT NULL DEFAULT 'active'; **CHECK** `chk_board_stories_status IN ('active','settled','merged')` | active（参与召回与热度）/ settled（已收束）/ merged（已合并，merged_into 指向存活者） |
| `first_seen_date` / `last_seen_date` | DATE | NOT NULL | 活跃区间（date 非时间戳），随新事件挂入滚动更新 |
| `thread_count` | INTEGER | NOT NULL DEFAULT 0 | 挂入事件数（composite 不计） |
| `article_count` | INTEGER | NOT NULL DEFAULT 0 | 关联文章数 |
| `participant_count` | INTEGER | NOT NULL DEFAULT 0 | 独立参与者数（按内容身份去重，feed 兜底）；**历史字段**（矩阵颜色通道已退役，不再有消费方） |
| `embedding` | vector | —（**无维度限定**，运行时维度） | 召回向量（title 级，只召回不判定） |
| `subject_embedding` | vector | —（**无维度限定**） | 主体归一化兑底向量（subject_key 的 embedding，cos ≥ 0.92）；仅为已建立事件线算一次 |
| `merged_into` | BIGINT | —（可空；FK → board_stories(id)）；index `board_stories_recall_idx` 为部分索引 `WHERE merged_into IS NULL AND status='active'` ON `(board_id, last_seen_date DESC)` | 指向合并后存活事件线；读取一跳解析（不允许两层链） |
| `origin` | TEXT | NOT NULL DEFAULT 'rule'; **CHECK** `chk_board_stories_origin IN ('rule','model','manual')` | rule（确定性判定）/ model（LLM 兕底，单分类器置信度 ≥0.75）/ manual（人工创建/合并，永不被自动判定覆盖） |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL DEFAULT now() | 创建 / 更新 |

### 9.12 story_signals（热度证据，add-event-story-heat；**只读保留**）

> ⚠️ **本表自 change `retire-story-adopt-event-metrics` 起只读保留**：不再有新写入（归属停写），历史行供回滚与存量核对；现行泳道热度不读本表（`heat-v2-lane-clues-1d` 按当天线索数现算）。

一行 = 一条报道证据在信源时间讨论过一条事件线；身份（事件归属）与热度（signals）分离，去重只作用于身份层。**参与者身份 = 内容身份优先、feed 兜底**：去重键 = `COALESCE(NULLIF(content_key,''), participant_key)`——同一篇内容（规范化链接相同）被多个 feed 各抓一份只算 1 个参与者，`participant_count` / 来源数 / 热度同源于此（fix-duplicate-article-heat）。热度只读本表计算（`story_heat = Σ 0.5^(Δt/24h)` 窗口 48h；`lane_heat` 72h/14d 独立衰减），不从日报派生。迁移 `20261105_0001`（+ `20261110_0001` 加 `content_key`）。**存量回填入口**：`cmd/backfill-stories --backfill-content-keys`（按 `articles.link` 规范化回填 `content_key` 并重算受影响事件线的参与者计数，幂等；迁移本身不做数据回填）。

| 字段名 | 类型 | 约束/默认/索引 | 用途 |
| -------- | ------ | ------ | ------ |
| `story_id` | BIGINT | NOT NULL; PK (1/2)；FK → board_stories(id) ON DELETE CASCADE；index `story_signals_story_idx` ON `(story_id, participant_key, observed_at DESC)` | 所属事件线 |
| `article_id` | BIGINT | NOT NULL; PK (2/2)；FK → articles(id) ON DELETE CASCADE | 报道文章（同一篇对同一条线只计一次证据） |
| `feed_id` | BIGINT | NOT NULL | 来源 feed |
| `participant_key` | TEXT | NOT NULL | 参与者键的 **feed 兜底值**（`feed:<id>`）；`content_key` 为空（内容身份不可判定）时按它去重，同参与者多篇只计一次 |
| `content_key` | TEXT | 可空（迁移 `20261110_0001` 新增）；index `story_signals_story_content_idx` ON `(story_id, content_key)` | 内容身份键 = `NormalizeArticleLink(articles.link)`（去 `#fragment`、去 `?query`、scheme/host 小写、path 原样；空 link → 空串 = 不可判定）；同一内容的多 feed 副本共用一个键，去重时优先于 `participant_key` |
| `kind` | TEXT | NOT NULL DEFAULT 'editorial'; **CHECK** `chk_story_signals_kind IN ('editorial','signal')` | editorial（信源报道，本期唯一写入值）/ signal（社媒等外部信号，预留位，写入路径未开放） |
| `observed_at` | TIMESTAMPTZ | NOT NULL; index `story_signals_time_idx` ON `(observed_at DESC)` | 信源时间 = `articles.pub_date`；热度衰减的 Δt 基准 |
| `observed_at_source` | TEXT | NOT NULL DEFAULT 'pub_date'; **CHECK** `chk_story_signals_observed_at_source IN ('pub_date','fallback')` | pub_date（信源时间，优先）/ fallback（文章缺 pub_date 时回退 created_at） |

> 写入幂等：复合主键 `(story_id, article_id)` 收敛（重复写入保留最新 observed_at）；归属变更时旧线 signal 删除、新线写入（`RecomputeSignalsForStory`）。证据不按 `archived` 过滤——归档只是轮转出 feed 活跃窗口（MaxArticles），不是撤回，归档报道照常计入热度（红线同 sourcestats 包「窗口查询不得过滤 archived」）；将来若需排除「撤回/恶意」须新增专用列，不得复用 `archived`。

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
| `idx_topic_key_events_board_source` | topic_key_event_markers | UNIQUE `(semantic_board_id, source_key)` | gorm tag（AutoMigrate） |
| `idx_topic_key_events_topic_state` | topic_key_event_markers | `(persistent_topic_id, state, source_period_date)` | gorm tag（AutoMigrate） |
| `idx_key_event_runs_report` | key_event_recommendation_runs | UNIQUE `(report_id)` | gorm tag（AutoMigrate） |
| `daily_report_threads_story_idx` | daily_report_threads | `(story_id)` | `20261105_0001` |
| `board_stories_recall_idx` | board_stories | 部分索引 `(board_id, last_seen_date DESC) WHERE merged_into IS NULL AND status='active'` | `20261105_0001` |
| `board_stories_subject_idx` | board_stories | `(board_id, subject_key)` | `20261105_0001` |
| `story_signals_time_idx` | story_signals | `(observed_at DESC)` | `20261105_0001` |
| `story_signals_story_idx` | story_signals | `(story_id, participant_key, observed_at DESC)` | `20261105_0001` |
| `story_signals_story_content_idx` | story_signals | `(story_id, content_key)` | `20261110_0001` |


## 域 ER 图

> 实线关系 = GORM 逻辑关联；标注「真实DB FK」的边 = DB 物理外键（共 13 条，全清单见 [_conventions.md](_conventions.md)）。外域邻居表仅标名，字段见其所属域文档。

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
    board_persistent_topics ||--|| topic_lane_snapshots : "persistent_topic_id (UNIQUE, OnDelete:CASCADE)" %% 每泳道一行滚动14天态势
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
    topic_lane_snapshots {
        SERIAL id PK
        VARCHAR rolling_summary "≤100字态势句"
        TEXT rolling_detail "≤500字长版叙述；空=缺失（存量/生成失败）"
        DATE as_of_date "汇总截止=最新报告期"
    }
    topic_watch_hits {
        SERIAL id PK
    }
```

