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
| [tables/embeddings.md](tables/embeddings.md) | 向量域 | topic-graph |
| [tables/job-queues.md](tables/job-queues.md) | 任务队列域 | content-enrichment |
| [tables/daily-report-watch.md](tables/daily-report-watch.md) | 日报 / 持久话题 / Watch 域 | daily-report |
| [tables/data-enrichment.md](tables/data-enrichment.md) | 数据增强域 | data-enrichment |
| [tables/preference-discovery.md](tables/preference-discovery.md) | 用户行为与偏好发现域 | discovery |
| [tables/tracing.md](tables/tracing.md) | 链路追踪域 | — |
| [tables/deprecated-framework.md](tables/deprecated-framework.md) | 已废弃 / 预留 / 框架表 | — |

另：[DATA_LIFECYCLE.md](DATA_LIFECYCLE.md)（数据状态字段流转，独立于域文档）。

## 完整表清单（52 业务 + 5 废弃 + 1 框架，按域分组 = 归属速查表；2026-09-02 cross_board 两表此前漏录，本次补齐）

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
| `ai_providers` | AI 供应商 | `models.AIProvider` |
| `ai_routes` | AI 路由 | `models.AIRoute` |
| `ai_route_providers` | AI 路由-供应商绑定 | `models.AIRouteProvider` |
| `ai_call_logs` | AI 调用日志 | `models.AICallLog` |
| `ai_embedding_cache` | embedding 结果缓存 | `models.AIEmbeddingCache` |

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

#### 语义标签/板块 → [`tables/semantic-labels.md`](tables/semantic-labels.md)（flow: `semantic-board`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `semantic_labels` | 语义标签统一表（辅助标签 + SemanticBoard） | `models.SemanticLabel` |
| `topic_tag_semantic_labels` | tag-辅助标签关联（中间表） | `models.TopicTagSemanticLabel` |
| `topic_tag_board_labels` | tag-SemanticBoard 匹配结果（中间表） | `models.TopicTagBoardLabel` |
| `board_composition` | board 构成（中间表；挂载单元可为 aux 或 composite，`auxiliary_label_id` 列复用） | `models.BoardComposition` |
| `composite_components` | 组合标签组件序列（composite→auxiliary，position 有序） | `models.CompositeComponent` |
| `board_upgrade_suggestions` | 板块升级建议 | `models.BoardUpgradeSuggestion` |

#### 向量 → [`tables/embeddings.md`](tables/embeddings.md)（flow: `topic-graph`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `embedding_config` | 向量配置（键值对） | `models.EmbeddingConfig` |
| `embedding_queues` | 向量生成队列 | `models.EmbeddingQueue` |
| `merge_reembedding_queues` | 合并后重算向量队列 | `models.MergeReembeddingQueue` |

#### 任务队列 → [`tables/job-queues.md`](tables/job-queues.md)（flow: `content-enrichment`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `firecrawl_jobs` | Firecrawl 抓取任务 | `models.FirecrawlJob` |
| `tag_jobs` | 标签任务 | `models.TagJob` |

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

#### 数据增强 → [`tables/data-enrichment.md`](tables/data-enrichment.md)（flow: `data-enrichment`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `board_data_sources` | 板块数据源绑定 | `dataenrichment.BoardDataSource` |
| `topic_lifeline_context` | 话题分层新闻汇总上下文（循环 A） | `dataenrichment.TopicLifelineContext` |
| `topic_enrichment_result` | 数据增强结果快照（不可变） | `dataenrichment.TopicEnrichmentResult` |
| `topic_enrichment_review` | 数据增强认知演进反思 | `dataenrichment.TopicEnrichmentReview` |
| `stock_debate_result` | FinGenius 个股辩论结果 | `dataenrichment.StockDebateResult` |
| `topic_enrichment_qa` | 报告追问记录（多轮 append-only） | `dataenrichment.TopicEnrichmentQA` |
| `reference_roles` | 旧参考角色/方法论画像（已退役，只读兼容一版本） | `dataenrichment.ReferenceRole` |
| `analysis_methods` | 分析方法卡库（调查链按问题选卡注入） | `dataenrichment.AnalysisMethod` |
| `cross_board_relation_runs` | 跨板块关系生成批次 | `repository.CrossBoardRelationRun` |
| `cross_board_relations` | 跨板块证据关系 | `repository.CrossBoardRelation` |

#### 偏好/发现 → [`tables/preference-discovery.md`](tables/preference-discovery.md)（flow: `discovery`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `preference_vectors` | 偏好向量画像（按 SemanticBoard 聚合） | `models.PreferenceVector` |
| `rsshub_routes` | RSSHub 路由目录 | `models.RSSHubRoute` |
| `route_embeddings` | RSSHub 路由向量 | `models.RouteEmbedding` |
| `feed_recommendations` | 订阅源推荐卡片 | `models.FeedRecommendation` |
| `route_param_options` | 路由参数可选值字典 | `models.RouteParamOption` |

#### 用户行为 → [`tables/preference-discovery.md`](tables/preference-discovery.md)（flow: `discovery`）

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `reading_behaviors` | 阅读行为 | `models.ReadingBehavior` |

#### 追踪 → [`tables/tracing.md`](tables/tracing.md)

| 表名 | 说明 | 对应模型 |
| ------ | ------ | ---------- |
| `otel_spans` | OpenTelemetry 链路追踪 | `tracing.OtelSpan` |

### 废弃表（5 张，无对应 model，保留标注）

| 表名 | 说明 |
| ------ | ------ |
| `ai_summaries` | 旧版 Feed 级 AI 批量摘要（已废弃） |
| `ai_summary_feeds` | AI 摘要-Feed 关联（已废弃） |
| `ai_summary_topics` | AI 摘要-主题关联（已废弃） |
| `topic_analysis_jobs` | 主题分析任务队列（已废弃，无 migrator 注册） |
| `digest_configs` | Digest 推送配置（预留，已废弃） |

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
│   sections      │                                 │ ai_providers/   │
│  daily_report_  │                                 │  ai_routes/     │
│   threads       │                                 │  ai_route_      │
│  daily_report_  │                                 │  providers/     │
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

