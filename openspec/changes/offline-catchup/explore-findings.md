
## offline-catchup 实现锚点与契约

openspec change offline-catchup（停机恢复补全）已验证的代码锚点与实现契约：

**删边段现状**（任务 2.3 移除对象）：`backend-go/internal/reader/service/feed_service.go` L197-215——affectedTagIDs pluck（L197-200）→ 删 ArticleTopicTag（L204）→ `tagging.CleanupOrphanedTags(affectedTagIDs)`（L215）。其余归档行为（behaviors 删/search_vector NULL/翻标志/窗口计数）不动。

**关键符号**：
- `CleanupOrphanedTags(tagIDs []uint)`：`internal/tagmanagement/service/core/article_tagger.go:364`，经 tagmanagement 包根 re-export（feed_service 以 `tagging.CleanupOrphanedTags` 调用）。EdgeGC 新函数照此 re-export 模式。
- `AuxLabelCleanupJob`：`internal/admin/scheduler/job_aux_label_cleanup.go` 全文 ~28 行，调 `tagging.NewAuxiliaryLabelService(repository.Repo.DB(), nil).GC(ctx, AuxLabelGCRequest{Mode: Disable, GraceDays: 1})`。边 GC 在此之后串第二步。
- `DailyReportJob`：`internal/admin/scheduler/job_daily_report.go`（164 行），L61 逐板 `daily_report.GenerateAndSaveReport(ctx, boardID, date)`；L111 `DailyReportSchedulerWrapper.TriggerNowWithDate(dateStr)`（补档扫描挂 job 尾部、守卫加在 wrapper + handler）。
- `GenerateAndSaveReport(ctx, boardID uint, date time.Time)`：`internal/topicgraph/service/daily_report_watch.go:47`（幂等 upsert，SaveReport 按 (semantic_board_id, period_date) 查存在性——daily_report_repository.go:190-192）。
- tag_jobs 状态枚举：`internal/models/job_queue.go:8-11` JobStatusPending/Leased/Completed/Failed；队列 repo `internal/tagmanagement/repository/tag_job_queue.go`，service `internal/tagmanagement/service/core/tag_queue.go`。
- ai_settings 读取模式：`models.AISettings{Key,Value string}`（ai_models.go:56），scheduler job 直查先例 `job_board_upgrade_suggest.go`；`AdminRepository.GetAISettings(key)`（admin/repository/repository.go:96）。

**共享配置契约**（三处同口径：EdgeGC 窗口 / 补档扫描窗 / 守卫下界）：键 `tag_edge_retention_days`，默认 7，缺失/非法/≤0 回退默认+warn。建议 tagmanagement 导出 `LoadTagEdgeRetentionDays(db) int` 类 helper 供 tagmanagement/admin/scheduler/topicgraph/handler 三方调用（tagmanagement 不 import admin，用 db 直查 models.AISettings）。

**边界口径**（test-cases.md 已定）：删边条件 `created_at < 本地当天零点 − N 天`（日历天口径，D=today-N 当天全保留）；守卫 `date < today−N 天（日历天）` 拒绝（date==下界放行）；补档扫描 (today-N, today) 不含今天；队列 pending/leased 任一存在即跳过补档顺延；failed 不阻断只计数告警。

**测试 harness**：reader/service 与 tagmanagement service 层测试用 SQLite（`setupCleanupTestDB` 模式：setupFeedsTestDB + tagging.InitRepository + AutoMigrate 补表）；scheduler job 测试先例 `job_firecrawl_test.go`/`pause_test.go`；handler 用 httptest。既有测试 `TestCleanupOldArticlesClearsDerivedData`（feed_service_cleanup_test.go:219）断言旧契约需反转（见 test-cases.md 继承与调整表）。

**并发态势**：本次目标文件全部干净；脏文件属其他 5 个 active change（ai-health-heartbeat-reprobe / improve-discovery-recommendations / expand-upgrade-days-window 等），不碰。

<!-- pinned 2026-09-14T01:47:23Z -->

## M5-B: EdgeGC 仅回收已归档文章的边

review M5-B（用户拍板）落地：`backend-go/internal/tagmanagement/service/core/edge_gc.go` 新增包级 const `archivedArticleEdgePredicate = "article_id IN (SELECT id FROM articles WHERE archived = ?)"`，在 EdgeGC 的**两处**谓词同时挂 `.Where(..., true)`——受影响 tag 的 Pluck（原 L96-101）与 Delete（原 L107）。语义：未归档文章的边永不回收，归档后才进入 N 天倒计时（活跃文章在分析面，阅读页标签角标/过滤直接消费边）。`countOrphanedTags`/`CleanupOrphanedTags` 谓词不动（全部边不论归档位），故存活的自归档外边会保住 tag。测试侧 `edge_gc_test.go` 新增 `seedArchivedArticle` 助手（seedArticle + Update archived=true），删除类用例文章全部改已归档；新增 `TestEdgeGCKeepsEdgesOfUnarchivedArticles`（含已归档对照边，证明 GC 真跑了）与 `TestEdgeGCUnarchivedEdgesDoNotAffectSharedTag`（混合边，AffectedTags/OrphanedTags 计数不被未归档边污染，tag 存活）。变异验证：把 const 改成 `"1 = ?"` 时两条新用例全 FAIL（断言有效）。连带 fixture 调整：`internal/admin/scheduler/pause_test.go` 的 `TestAuxLabelCleanupEdgeGCRunsWhileAnalysisPaused` 文章补 `Archived: true`（否则新契约下 edge_deleted_count 期望 1 实得 0）。

**引用**：backend-go/internal/tagmanagement/service/core/edge_gc.go:archivedArticleEdgePredicate、backend-go/internal/tagmanagement/service/core/edge_gc.go:EdgeGC、backend-go/internal/tagmanagement/service/core/edge_gc_test.go:TestEdgeGCKeepsEdgesOfUnarchivedArticles、backend-go/internal/tagmanagement/service/core/edge_gc_test.go:seedArchivedArticle、backend-go/internal/admin/scheduler/pause_test.go:TestAuxLabelCleanupEdgeGCRunsWhileAnalysisPaused

<!-- pinned 2026-09-14T15:39:50Z -->
