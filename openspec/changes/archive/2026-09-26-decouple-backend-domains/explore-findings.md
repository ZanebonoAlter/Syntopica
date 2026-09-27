
## 解耦迁移的机械事实清单

decouple-backend-domains 实现阶段的关键代码事实（探索阶段已核实）：

**调度器搬家（Batch 1）**：框架四文件 `admin/scheduler/{base.go,registry.go,pause.go,persistence.go}`（JobFunc 定义在 base.go:16）；job_*.go 留 admin 只改 import；SchedulerTask 模型随走；消费方：dataenrichment/scheduler_jobs.go、app/runtime.go(resetStaleStates)、platform/analysispause+articlerefs 测试

**discovery 拆域（Batch 2）**：迁移文件=service 的 discovery_*/candidate_*/recommendation_*/availability/catalog_*/route_param_option_service/rsshub_config/lifecycle_config + handler 的 discovery_handler/candidate_catalog_handler/candidate_check_handler/route_param_option_handler + models 的 discovery_run/discovery_candidate/candidate_availability 全部；路由在 admin/routes.go:61 `rg.Group("/discovery")` 起的整块；job_discovery_v2/job_rsshub_catalog_sync/job_preference_profile_update 留 admin/scheduler；preference_profile_service+PreferenceVector 留 admin；争议文件 discovery_v2_switch/seed_policy* 按 grep 引用面定

**models 下放（Batch 3）**：每域建 internal/<domain>/models/ 子包防 root↔service 循环。域内清单：discovery 11 个、tagmanagement 11 个（BoardComposition/BoardUpgradeSuggestion/CompositeComponent/EmbeddingConfig/MergeReembeddingQueue/TagCategoryMeta/TagMergeSuggestion/TopicTagAnalysis/TopicTagBoardLabel/TopicTagEmbedding/TopicTagSemanticLabel）、reader FeedStats、topicgraph TopicAnalysisCursor、admin PreferenceVector、platform/scheduler SchedulerTask。留 models 白名单 18 项：12 真共享（AISettings/Article/ArticleTopicTag/Category/EmbeddingQueue/Feed/FirecrawlJob/ReadingBehavior/SemanticLabel/TagJob/TopicTag/TopicTagRelation）+ 6 跨层（AIProvider/AIRoute/AIRouteProvider/AICallLog/AIEmbeddingCache/Notification——platform/airouter 与 platform/notification 是真实消费方，grep 单域标记是因 platform 被过滤）

**门面收敛**：tagmanagement/wire.go 已有门面需补 TagQueueStatusSnapshot(handler 深路径→admin/handler/poll_handler.go 用)/GetWatchedTagIDsExpanded(service/watched→reader/handler/article_handler:111)/FeedBoardHitStats+ParseWindow(service/sourcestats→reader/handler/feed_board_stats_handler)；reader 现有 root 调用点：firecrawl_handler/article_handler 用 NewTagJobQueue+TagJobRequest+FeedCategoryName（wire 已 re-export）；admin→reader 深路径在 job_content_completion 等 5 文件

**depguard 注意**：module 名 syntopica-backend；跨域深路径 grep 基线（2026-09-26 复测）：生产边 9 条共 14 文件——admin→tagmanagement 3、admin→reader 2、admin→topicgraph 1、admin→datasources 1、reader→tagmanagement 2、dataenrichment→{admin,reader,datasources} 各 1、datasources→dataenrichment 2；topicgraph→tagmanagement 边为 0（初版记 4 系口径漂移）；跨域测试文件 9 个——admin→{tagmanagement,reader,topicgraph} 各 2、reader→{tagmanagement,topicgraph} 各 1、dataenrichment→topicgraph 1；reader/repository/article_refs_test.go:18 有 blank import topicgraph/repository 需随迁更新

<!-- pinned 2026-09-25T14:37:29Z -->

## AutoMigrate 模型注册必须走 RegisterModels 注入（域 models 下放的唯一合法路径）

Batch 1 实施时发现：platform/database/migrator.go 的 RunAutoMigrate 静态 allModels 清单与「模型下放到各域」天然冲突——域包（如 platform/scheduler、后续 discovery/models、tagmanagement/models）被 migrator import 会与「域→database（DB 访问，全仓 67 文件）」成环（Batch 1 实锤：scheduler→analysispause→aihealth→airouter→database + migrator→scheduler = import cycle）。

正确模式（已就位）：migrator.go 已有 extraModels + database.RegisterModels(models ...any) 注入机制（topicgraph/repository/daily_report_register_models.go 是先例：init() 里注册 BoardDailyReport 等 8 模型）。

Batch 2/3 迁模型时：各域 models 子包建 register 文件 init() 调 database.RegisterModels(&X{}...)，同时从 migrator.go 静态清单删对应行。测试侧同理：sqlite 测试的 RunAutoMigrate 依赖 RegisterModels 副作用的传递 import 链，链断后需在测试文件显式 blank import（先例：dataenrichment/service/orchestrator_test.go 补 `_ "syntopica-backend/internal/topicgraph/repository"`；platform/articlerefs/helpers_test.go 原有先例）。

其他 Batch 1 落地事实：SchedulerTask 定义在 platform/scheduler/task.go（含 ToDict + 私有 CST 格式化 helper，从 models/utils.go 复制）；persistence.go 直接用 database.DB（等价原 repository.Repo.DB()，同源）；pause_test.go 留在 admin/scheduler（它测 PauseAware×具体 job 集成，用 job_firecrawl.go 的非导出 firecrawlJobWithCrawler，归 job 域）；admin/scheduler 包内 job 文件引用框架符号一律加 scheduler. 前缀（import platform/scheduler）；admin/wire.go 框架 re-export 用别名 platformscheduler；scheduler_handler.go 用别名 pssched（参数名 scheduler 遮蔽）。

<!-- pinned 2026-09-26T03:56:40Z -->
