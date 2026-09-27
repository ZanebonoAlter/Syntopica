
## completion_on_refresh 死字段与总结触发链路全图

【字段现状】models/feed.go: `ArticleSummaryEnabled`(article_summary_enabled, gorm default:false) / `CompletionOnRefresh`(completion_on_refresh, gorm default:true ←要改false) / `MaxCompletionRetries`(default:3) / `FirecrawlEnabled`(default:false) / `TaggingEnabled`(default:true)。

【触发链路·置状态点】
1. feed_service.go buildArticleFromEntry(~L316)：firecrawl+summary→SummaryStatus="incomplete"；非firecrawl+summary→"pending"（两分支都要加 && CompletionOnRefresh）
2. job_firecrawl.go L226 抓取成功：`if feed.ArticleSummaryEnabled { updates["summary_status"]="incomplete" }`（加条件）
3. job_firecrawl.go ~L195 终态失败降级RSS：`if feed.ArticleSummaryEnabled { Update summary_status incomplete }`（加条件）

【扫描点】
- content_completion_service.go ListReadyArticles(~L120)：JOIN feeds，`article_summary_enabled=true` + summary_status IN (incomplete,pending,stale) + firecrawl_status IN (completed,failed)，由 job_content_completion.go:25 每 tick 捞 50 篇（加 feeds.completion_on_refresh=true）
- repository.go ListArticlesForCompletion(L473)：唯一消费 completion_on_refresh 的查询，零调用方→删除
- repository.go ListArticlesIncomplete(L457)：条件 article_summary_enabled+非firecrawl，调用方待查
- job_blocked_article_recovery.go:51：按 article_summary_enabled=true 找 feed（对齐加闸门）

【手动总结】后端 POST /api/content-completion/articles/:id/complete 带 force，不检查 feed 开关；前端手动按钮显示条件 useArticleContentView.ts:120 `feed.articleSummaryEnabled===true`（保持不动即方案A）。

【素材降级链】content_completion_service.go L169：firecrawl_content > RSS description；打标签 article_tagger.go:347 与 topic_watch_repository.go:316 同为降级链（总结缺失fallback全文，不断链）。

【迁移】postgres_migrations.go 既有 Migration 框架（幂等 SQL 模式，参考 L894 icon_source backfill）。DB 实况：24/24 feed completion_on_refresh=true（历史遗产），7 feed 开 article_summary_enabled（全是 firecrawl feed），26 篇文章 summary_status IN ('','pending','incomplete','failed')，1天46篇总结。

【前端】EditFeedDialog.vue(293行, FeedLayoutShell.vue:655 调用)：有url/分类/总结开关/重试/删除，内部 ??false(L27)/??true(L39) 回退不一致bug；FeedDetailEditor.vue(settings入口 SettingsSectionFeeds.vue:130)：有分类/刷新间隔/最大文章数/firecrawl/打标签/"内容补全"(completion_on_refresh误导名L254)，缺 AI总结开关/重试/url/删除(L127已有删除按钮)。AddFeedDialog 不传两开关字段(Go零值false)。SettingsSectionFeeds.selectedFeedId 为本地 ref(L29)，深链需读 route.query.feed。删除能力：useGlobalSettings.ts:108 deleteFeed。

**引用**：backend-go/internal/models/feed.go、backend-go/internal/reader/service/feed_service.go:316、backend-go/internal/admin/scheduler/job_firecrawl.go:195、backend-go/internal/admin/scheduler/job_firecrawl.go:226、backend-go/internal/reader/service/content_completion_service.go:120、backend-go/internal/reader/repository/repository.go:457、backend-go/internal/reader/repository/repository.go:473、backend-go/internal/admin/scheduler/job_blocked_article_recovery.go:51、backend-go/internal/platform/database/postgres_migrations.go、front/app/components/dialog/EditFeedDialog.vue、front/app/features/settings/components/FeedDetailEditor.vue、front/app/features/settings/components/SettingsSectionFeeds.vue、front/app/features/shell/components/FeedLayoutShell.vue:655、front/app/features/articles/composables/useArticleContentView.ts:120

<!-- pinned 2026-09-20T12:44:39Z -->

## ListArticlesIncomplete 查实为死代码，随 ListArticlesForCompletion 一并删除

探索报告称 ListArticlesIncomplete（repository.go:454）"调用方待查"，实现档 grep 全仓（生产+测试）确认零调用方，与 ListArticlesForCompletion（:473）同为死代码。ItemQuery 类型（:560）仅被 ListArticlesForCompletion 引用，三者一并删除。job_blocked_article_recovery.go 的 L51 区域实为 STAT-05 告警计数（按 article_summary_enabled=true 统计 incomplete），加 AND completion_on_refresh=true 对齐闸门；恢复循环（waiting_for_firecrawl/blocked → pending）只关 firecrawl 不涉总结开关，不动。test-cases.md BR-1~BR-5 已收录。

<!-- pinned 2026-09-20T13:04:46Z -->
