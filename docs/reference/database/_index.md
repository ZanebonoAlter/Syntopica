# 数据库文档索引（`docs/reference/database/`）

> **真相源 = 代码**（GORM struct + `postgres_migrations.go`），本目录是投影。全局事实（FK 真相 / 向量维度 / 唯一与 CHECK / FK 引用矩阵）唯一权威在 [`tables/_conventions.md`](tables/_conventions.md)。

## 域文档导航

| 域文档 | 覆盖 | flow 域对照 |
| ------ | ------ | ------ |
| [tables/content.md](tables/content.md) | 内容域 | content-enrichment |
| [tables/scheduling-config.md](tables/scheduling-config.md) | 调度与配置域 | scheduler |
| [tables/ai-routing.md](tables/ai-routing.md) | AI 路由域 | ai-summary |
| [tables/topic-tags.md](tables/topic-tags.md) | 主题标签域 | topic-graph |
| [tables/semantic-labels.md](tables/semantic-labels.md) | 语义标签 / 板块域 | semantic-board |
| [tables/entity-subjects.md](tables/entity-subjects.md) | 实体主体身份层（身份 / 名称证据 / aux 溯源） | topic-graph, semantic-board |
| [tables/research-data-sources.md](tables/research-data-sources.md) | 研究数据源域 | research-data-sources |
| [tables/embeddings.md](tables/embeddings.md) | 向量域 | topic-graph |
| [tables/job-queues.md](tables/job-queues.md) | 任务队列域 | content-enrichment |
| [tables/daily-report-watch.md](tables/daily-report-watch.md) | 日报 / 持久话题 / Watch 域 | daily-report |
| [tables/margin-notes.md](tables/margin-notes.md) | 页边注域（批注锚点 / 问答轮 / 术语库） | daily-report |
| [tables/data-enrichment.md](tables/data-enrichment.md) | 数据增强域 | data-enrichment |
| [tables/preference-discovery.md](tables/preference-discovery.md) | 用户行为与偏好发现域 | discovery |
| [tables/tracing.md](tables/tracing.md) | 链路追踪域 | — |
| [tables/deprecated-framework.md](tables/deprecated-framework.md) | 已废弃 / 预留 / 框架表 | — |

另：[DATA_LIFECYCLE.md](DATA_LIFECYCLE.md)（数据状态字段流转，独立于域文档）、[RESTORE.md](RESTORE.md)（灾备恢复流程：备份档恢复契约 + 可照做步骤 + 已知失败模式 + 实测耗时）、[RESTORE_DRILL.md](RESTORE_DRILL.md)（恢复演练留痕：演练时间/档来源/count 校验/迁移对账/耗时）。

## 完整表清单（真库 87 张表 = **85 业务 + 1 残留废弃（`user_preferences`）+ 1 框架（`schema_migrations`）**；下方另列 7 张「真库已无、仅考古快照」的历史 / 已删表 —— 分组清单共 94 行，按域分组 = 归属速查表；2026-09-02 cross_board 两表补齐；2026-09-24 页边注三表补齐；2026-10 ai-provider-model-overhaul 三张两层表补齐、ai_providers/ai_route_providers 停写废弃；2026-10-29 board-health-triage-advice 描述修复任务/条目两表补齐；2026-11 add-event-story-heat 事件线/热度证据两表补齐；2026-11 normalize-event-subject-entities 实体主体身份层三表补齐；2026-11-19 docs-sync-audit-fixes 索引回填 16 张（主题标签 1 / 语义标签 1 / 日报 3 / 数据增强 2 / discovery 7 / 研究数据源 1 / 残留废弃 1）、把已 DROP 的 reference_roles / analysis_methods 移入废弃节，计数改真库实测口径；2026-11-25 add-daily-report-gap-backfill 缺档补填留档表 1 张补齐）

#### 内容 → [`tables/content.md`](tables/content.md)（flow: `content-enrichment`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `categories` | 分类 | `models.Category` |
| `feeds` | 订阅源 | `models.Feed` |
| `articles` | 文章 | `models.Article` |

#### 调度 → [`tables/scheduling-config.md`](tables/scheduling-config.md)（flow: `scheduler`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `scheduler_tasks` | 调度任务状态 | `models.SchedulerTask` |
| `ai_settings` | AI 配置（键值对） | `models.AISettings` |

#### AI 路由 → [`tables/ai-routing.md`](tables/ai-routing.md)（flow: `ai-summary`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `ai_vendors` | AI 供应商（协议/认证/端点/方言/结构化档） | `models.AIVendor` |
| `ai_models` | AI 模型（唯一挂 vendor，七档思考默认档） | `models.AIModel` |
| `ai_route_models` | AI 路由-模型绑定（场景思考覆盖档） | `models.AIRouteModel` |
| `ai_routes` | AI 能力路由 | `models.AIRoute` |
| `ai_call_logs` | AI 调用日志 | `models.AICallLog` |
| `ai_embedding_cache` | embedding 结果缓存 | `models.AIEmbeddingCache` |
| `ai_providers` | AI 供应商（⛔ 停写废弃，迁移 `20261005_0001` 拆解为 vendor+model） | `models.AIProvider`（Deprecated） |
| `ai_route_providers` | AI 路由-供应商绑定（⛔ 停写废弃，改挂 ai_route_models） | `models.AIRouteProvider`（Deprecated） |

#### 主题标签 → [`tables/topic-tags.md`](tables/topic-tags.md)（flow: `topic-graph`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `topic_tags` | 主题标签主表 | `models.TopicTag` |
| `topic_tag_embeddings` | 主题标签向量 | `models.TopicTagEmbedding` |
| `topic_tag_analyses` | 主题分析快照 | `models.TopicTagAnalysis` |
| `topic_analysis_cursors` | 主题分析游标 | `models.TopicAnalysisCursor` |
| `article_topic_tags` | 文章-主题关联 | `models.ArticleTopicTag` |
| `topic_tag_relations` | 标签层级关系 | `models.TopicTagRelation` |
| `tag_merge_suggestions` | 标签合并建议 | `models.TagMergeSuggestion` |
| `tag_category_meta` | 标签分类默认展示属性（AutoMigrate 建表、四列 text 无主键；**当前无读写路径、库内 0 行**，展示属性实际由代码常量提供） | `tagmanagement/models.TagCategoryMeta` |

#### 语义标签/板块 → [`tables/semantic-labels.md`](tables/semantic-labels.md)（flow: `semantic-board`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `semantic_labels` | 语义标签统一表（辅助标签 + SemanticBoard） | `models.SemanticLabel` |
| `topic_tag_semantic_labels` | tag-辅助标签关联（中间表） | `models.TopicTagSemanticLabel` |
| `topic_tag_board_labels` | tag-SemanticBoard 匹配结果（中间表） | `models.TopicTagBoardLabel` |
| `board_composition` | board 构成（中间表；挂载单元可为 aux 或 composite，`auxiliary_label_id` 列复用） | `models.BoardComposition` |
| `composite_components` | 组合标签组件序列（composite→auxiliary，position 有序） | `models.CompositeComponent` |
| `board_upgrade_suggestions` | 板块升级建议 | `models.BoardUpgradeSuggestion` |
| `auxiliary_description_repair_jobs` | 共享描述修复任务（幂等键 + 冻结范围计数 + 租约列，无 DB FK） | `tagmanagement/models.AuxiliaryDescriptionRepairJob` |
| `auxiliary_description_repair_items` | 描述修复任务条目（冻结的 topic_tag 逐项进度，无 DB FK） | `tagmanagement/models.AuxiliaryDescriptionRepairItem` |
| `board_health_snapshots` | 版块体检快照（每轮每活跃版块一行；同日多条不去重、历史不覆盖） | `tagmanagement/models.BoardHealthSnapshot` |

#### 实体主体身份层 → [`tables/entity-subjects.md`](tables/entity-subjects.md)（flow: `topic-graph`, `semantic-board`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `entity_registry` | 实体身份行（稳定 ID / entity_key / 规范名无唯一约束 / 稳定短身份说明 / 两套检索向量及空间三元组；**无类型列**） | `tagmanagement/models.EntityRegistry` |
| `entity_mentions` | 名称与证据行（提及原文 + 规范化提及键 + 来源标签/文章；叫法允许歧义） | `tagmanagement/models.EntityMention` |
| `entity_aux_links` | 辅助标签溯源行（aux 硬删只掉本行、绝不级联删实体；GC 置 disabled 时读取态过滤） | `tagmanagement/models.EntityAuxLink` |

#### 向量 → [`tables/embeddings.md`](tables/embeddings.md)（flow: `topic-graph`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `embedding_config` | 向量配置（键值对） | `models.EmbeddingConfig` |
| `embedding_queues` | 向量生成队列 | `models.EmbeddingQueue` |
| `merge_reembedding_queues` | 合并后重算向量队列 | `models.MergeReembeddingQueue` |
| `related_event_embeddings` | 相关事件派生向量缓存（可丢弃索引，`related-events` 能力） | `tagmanagement/models.RelatedEventEmbedding` |

#### 任务队列 → [`tables/job-queues.md`](tables/job-queues.md)（flow: `content-enrichment`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `firecrawl_jobs` | Firecrawl 抓取任务 | `models.FirecrawlJob` |
| `tag_jobs` | 标签任务 | `models.TagJob` |
| `detection_jobs` | 疑似 AI 判定任务（与 `tag_jobs` 同构且完全独立；同文章至多一条未完成作业的部分唯一索引） | `models.DetectionJob` |

#### 通知 → [`tables/job-queues.md`](tables/job-queues.md)（flow: `daily-report`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `notifications` | 通知信箱（白名单=日报终态；上限 500 行写入路径内淘汰） | `models.Notification` |

#### 日报/持久话题/Watch → [`tables/daily-report-watch.md`](tables/daily-report-watch.md)（flow: `daily-report`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `board_daily_reports` | 板块日报主表 | `topicgraph.BoardDailyReport` |
| `daily_report_sections` | 日报分区 | `topicgraph.DailyReportSection` |
| `daily_report_threads` | 日报叙事线程 | `topicgraph.DailyReportThread` |
| `daily_report_section_relations` | 跨日分区关系 | `topicgraph.SectionRelation` |
| `board_persistent_topics` | 板块持久叙事话题 | `topicgraph.BoardPersistentTopic` |
| `board_topic_watches` | 用户声明的话题 Watch 标签 | `topicgraph.BoardTopicWatch` |
| `topic_watch_hits` | Watch 命中记录 | `topicgraph.TopicWatchHit` |
| `topic_key_event_markers` | 话题关键事件注记（人工主权三态 + 来源指纹幂等） | `topicgraph.TopicKeyEventMarker` |
| `key_event_recommendation_runs` | 关键事件推荐任务状态（每报告一行） | `topicgraph.KeyEventRecommendationRun` |
| `board_stories` | 事件线（跨天同一事件聚合，结构化指纹 + 合并链；add-event-story-heat）。**只读保留**：change `retire-story-adopt-event-metrics` 起停写，不删表不迁移 | `topicgraph.BoardStory` |
| `story_signals` | 热度证据（参与者×文章×信源时间，复合主键；add-event-story-heat）。**只读保留**：停写后无新行，现行泳道热度不读本表 | `topicgraph.StorySignal` |
| `topic_lane_snapshots` | 泳道滚动态势快照（每活跃泳道一行，日报后 upsert 覆盖；纯派生缓存） | `topicgraph.TopicLaneSnapshot` |
| `topic_anchor_overrides` | 首义改写留痕（append-only，每次人工重设一行；**刻意无 FK**） | `topicgraph.TopicAnchorOverride` |
| `section_assignment_overrides` | 泳道归属变更留痕（append-only，`detach`/`attach` 各一行；**刻意无 FK**） | `topicgraph.SectionAssignmentOverride` |
| `daily_report_backfill_runs` | 缺档补填运行留档（窗口、逐日明细 jsonb、四态终态、补出/失败/剩余计数；add-daily-report-gap-backfill） | `topicgraph.DailyReportBackfillRun` |

#### 页边注 → [`tables/margin-notes.md`](tables/margin-notes.md)（flow: `daily-report`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `report_annotations` | 批注锚点（划词 + 归属 + 偏移线索） | `topicgraph.ReportAnnotation` |
| `annotation_qas` | 问答轮（引用/网络来源/术语 jsonb 数组契约） | `topicgraph.AnnotationQA` |
| `term_notes` | 术语库（归一化唯一键 + P2 预留 embedding） | `topicgraph.TermNote` |

#### 数据增强 → [`tables/data-enrichment.md`](tables/data-enrichment.md)（flow: `data-enrichment`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `board_data_sources` | 板块数据源绑定 | `dataenrichment.BoardDataSource` |
| `topic_lifeline_context` | 话题分层新闻汇总上下文（循环 A） | `dataenrichment.TopicLifelineContext` |
| `topic_enrichment_result` | 数据增强结果快照（不可变） | `dataenrichment.TopicEnrichmentResult` |
| `topic_enrichment_review` | 数据增强认知演进反思 | `dataenrichment.TopicEnrichmentReview` |
| `stock_debate_result` | FinGenius 个股辩论结果 | `dataenrichment.StockDebateResult` |
| `topic_enrichment_qa` | 报告追问记录（多轮 append-only） | `dataenrichment.TopicEnrichmentQA` |
| `cross_board_relation_runs` | 跨板块关系生成批次 | `repository.CrossBoardRelationRun` |
| `cross_board_relations` | 跨板块证据关系 | `repository.CrossBoardRelation` |
| `board_signal_discovery` | 板块信号发现批次（两阶段人工流程，board-signal-reports） | `repository.BoardSignalDiscovery` |
| `board_signal_candidate` | 板块信号候选（不可变，复合 FK 钉死与批次一致） | `repository.BoardSignalCandidate` |
| `board_signal_research_progress` | 研究进展滚动快照（同 job 一行：`running` / `abandoned` / `superseded`，中断可查） | `repository.BoardSignalResearchProgress` |
| `board_signal_disposition` | 候选忽略标记（有行即忽略、恢复即删行；与 runner 临界区串行） | `repository.BoardSignalDisposition` |

#### 偏好/发现 → [`tables/preference-discovery.md`](tables/preference-discovery.md)（flow: `discovery`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `preference_vectors` | 偏好向量画像（按 SemanticBoard 聚合） | `models.PreferenceVector` |
| `rsshub_routes` | RSSHub 路由目录 | `models.RSSHubRoute` |
| `route_embeddings` | RSSHub 路由向量 | `models.RouteEmbedding` |
| `feed_recommendations` | 订阅源推荐卡片 | `models.FeedRecommendation` |
| `route_param_options` | 路由参数可选值字典 | `models.RouteParamOption` |
| `feed_candidates` | 候选源库（入库 ≠ 订阅；`stable_key` 跨来源稳定身份，UNIQUE） | `models.FeedCandidate` |
| `candidate_embeddings` | 候选介绍向量（UNIQUE(candidate_id, model, dimension)，指纹变才重嵌） | `models.CandidateEmbedding` |
| `candidate_availability` | 候选端点可用性状态机（unknown/ok/broken/requires_parameters；无记录=未验证） | `models.CandidateAvailability` |
| `candidate_preferences` | 候选推荐生命周期（accepted/expired/snoozed/excluded/restored，UNIQUE(candidate_id)） | `models.CandidatePreference` |
| `discovery_runs` | 发现运行（ask/refresh 全 run 化，`request_key` 幂等 UNIQUE） | `models.DiscoveryRun` |
| `discovery_run_items` | 本轮精排选中清单（原子发布证据，UNIQUE(run_id, candidate_id)） | `models.DiscoveryRunItem` |
| `discovery_interest_entries` | 问答兴趣条目（逐条独立一行，不合成单条平均向量） | `models.DiscoveryInterestEntry` |

#### 研究数据源 → [`tables/research-data-sources.md`](tables/research-data-sources.md)（flow: `research-data-sources`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `data_sources` | 研究数据源目录（启动幂等 upsert seed；观测值不落库，取数仅内存 TTL 缓存） | `datasources.DataSource` |

#### 用户行为 → [`tables/preference-discovery.md`](tables/preference-discovery.md)（flow: `discovery`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `reading_behaviors` | 阅读行为 | `models.ReadingBehavior` |

#### 追踪 → [`tables/tracing.md`](tables/tracing.md)

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `otel_spans` | OpenTelemetry 链路追踪 | `tracing.OtelSpan` |

### 废弃 / 已删表（8 张，均无对应 model）

**真库只残留 1 张**（`user_preferences`，DROP 迁移受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫）；其余 7 张在真库**已不存在**，仅作历史结构 / 考古快照保留（不可照本表清单建库）。

| 表名 | 说明 |
| ------ | ------ |
| `user_preferences` | 旧偏好表（**已废弃、无 model，真库可能残留**：`DROP TABLE` 迁移受 `MIGRATIONS_ALLOW_DESTRUCTIVE` 守卫。残留行有 DB 级 FK `NO ACTION` 指向 `feeds` / `categories`，删源 / 删分类路径按表存在性探测清引用，见 [_conventions.md](tables/_conventions.md)（B-2）；ER 关系见 [content.md](tables/content.md)） |
| `ai_summaries` | 旧版 Feed 级 AI 批量摘要（已废弃，真库无此表） |
| `ai_summary_feeds` | AI 摘要-Feed 关联（已废弃，真库无此表） |
| `ai_summary_topics` | AI 摘要-主题关联（已废弃，真库无此表） |
| `topic_analysis_jobs` | 主题分析任务队列（已废弃，无 migrator 注册，真库无此表） |
| `digest_configs` | Digest 推送配置（预留，已废弃，真库无此表） |
| `reference_roles` | 旧参考角色 / 方法论画像（**已 DROP**：迁移 `20260926_0001` restructure-settings-navigation 删表；真库无此表，结构快照留在迁移注释里） |
| `analysis_methods` | 分析方法卡库（**已 DROP**：同上迁移 `20260926_0001`；真库无此表） |

### 框架表

| 表名 | 说明 |
| ------ | ------ |
| `schema_migrations` | 迁移版本追踪（框架管理，不计入业务表） |

---


---

## 全局域级概览

```
┌─────────────────┐       ┌─────────────────────┐
│     Core        │       │    Topic Tags        │
│  ┌───────────┐  │ (GORM │  ┌─────────────────┐ │  ← 逻辑引用中心 (hub)
│  │ categories│──┼──逻辑─┼─→│   topic_tags     │ │
│  ├───────────┤  │ 关联) │  ├─────────────────┤ │
│  │   feeds   │  │       │  │ topic_tag_       │ │
│  ├───────────┤  │       │  │   embeddings     │ │
│  │ articles  │  │       │  ├─────────────────┤ │
│  ├───────────┤  │       │  │ article_topic_   │ │
│  │ reading_  │  │       │  │   tags           │ │
│  │  behaviors│  │       │  ├─────────────────┤ │
│  ├───────────┤  │       │  │ embedding_queues │ │
│  │ user_     │  │       │  ├─────────────────┤ │
│  │ preferences│ │       │  │ merge_reembedding│ │
│  ├───────────┤  │       │  │   _queues        │ │
│  │ firecrawl_│  │       │  ├─────────────────┤ │
│  │   jobs    │  │       │  │ topic_tag_       │ │
│  ├───────────┤  │       │  │   analyses       │ │
│  │ tag_jobs  │  │       │  ├─────────────────┤ │
│  └───────────┘  │       │  │ topic_analysis_  │ │
└─────────────────┘       │  │   cursors        │ │
                          │  ├─────────────────┤ │
┌─────────────────┐       │  │ topic_tag_       │  ┌─────────────────┐
│ Semantic Label  │ (GORM │  │   semantic_      │  │ Data Enrichment │
│  ┌───────────┐  │ 逻辑) │  │   labels         │  │  board_data_    │
│  │semantic_  │──┼───────┤  ├─────────────────┤ │  │   sources       │
│  │  labels   │  │       │  │ topic_tag_board_ │ │  ├───────────────┤ │
│  ├───────────┤  │       │  │   labels         │ │  │ topic_lifeline_ │ │
│  │ board_    │  │       │  ├─────────────────┤ │  │   context       │ │
│  │composition│  │       │  │ board_upgrade_   │ │  ├───────────────┤ │
│  └───────────┘  │       │  │   suggestions    │ │  │ topic_enrichment│ │
└────────┬────────┘       │  └─────────────────┘ │  │  _result/review │ │
         │ (board)        └─────────────────────┘  ├───────────────┤ │
         │                                           │ stock_debate_  │ │
┌────────▼────────┐                                 │   result       │ │
│ Daily Report /  │                                 └───────────────┘ │
│ Persistent Topic│                                 └─────────────────┘
│  board_daily_   │
│   reports       │                                 ┌─────────────────┐
│  daily_report_  │                                 │ AI Infra        │
│   sections      │                                 │ ai_vendors/     │
│  daily_report_  │                                 │  ai_models/     │
│   threads       │                                 │  ai_route_      │
│  daily_report_  │                                 │  models/        │
│ section_relations                                  │  ai_call_logs/  │
│  board_persistent│                                 │  ai_settings/   │
│   topics        │                                 │  scheduler_tasks│
│  board_topic_   │                                 │  otel_spans     │
│   watches       │                                 └─────────────────┘
│  topic_watch_   │
│   hits          │
└─────────────────┘
```

- 实线箭头 → 表示 **GORM 逻辑引用**（源表字段指向目标表 `id`）；除 `topic_tags.merged_into_id` 外均无 DB 级 FK。
- `semantic_labels` 是语义标签中心表，辅助标签（`label_type=auxiliary`）、SemanticBoard（`label_type=board`）与组合标签（`label_type=composite`，add-composite-labels）共存于此表。
- `topic_tags` 通过 `topic_tag_semantic_labels` 和 `topic_tag_board_labels` 两张桥接表与 `semantic_labels` 关联。
- `board_daily_reports` / `board_persistent_topics` / `board_topic_watches` / `board_data_sources` 均通过 `semantic_board_id` 逻辑引用 `semantic_labels`。
- 「AI Summaries」域（`ai_summaries` 等）已废弃（无对应 model，见下文该域说明）。

---

