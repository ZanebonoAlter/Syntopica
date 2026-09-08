package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"syntopica-backend/internal/admin"
	"syntopica-backend/internal/admin/scheduler"
	"syntopica-backend/internal/dataenrichment"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/aihealth"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	content "syntopica-backend/internal/reader"
	tagging "syntopica-backend/internal/tagmanagement"
)

type Runtime struct {
	Registry *admin.SchedulerRegistry
}

func resetStaleStates() {
	resetCount := 0

	result := database.DB.Model(&models.SchedulerTask{}).
		Where("status = ?", "running").
		Updates(map[string]interface{}{
			"status":     "idle",
			"last_error": "reset on startup: previous process terminated unexpectedly",
		})
	resetCount += int(result.RowsAffected)

	if resetCount > 0 {
		logging.Infof("Reset %d scheduler task(s) stuck in 'running' state", resetCount)
	}

	feedResult := database.DB.Model(&models.Feed{}).
		Where("refresh_status = ?", "refreshing").
		Updates(map[string]interface{}{
			"refresh_status": "idle",
			"refresh_error":  "reset on startup: previous process terminated unexpectedly",
		})
	if feedResult.RowsAffected > 0 {
		logging.Infof("Reset %d feed(s) stuck in 'refreshing' state", feedResult.RowsAffected)
	}

	articleResult := database.DB.Model(&models.Article{}).
		Where("firecrawl_status = ?", "processing").
		Updates(map[string]interface{}{
			"firecrawl_status": "pending",
			"firecrawl_error":  "reset on startup: previous process terminated unexpectedly",
		})
	if articleResult.RowsAffected > 0 {
		logging.Infof("Reset %d article(s) stuck in 'processing' firecrawl state", articleResult.RowsAffected)
	}

	jobResult := database.DB.Model(&models.FirecrawlJob{}).
		Where("status = ?", "leased").
		Updates(map[string]interface{}{
			"status":           "pending",
			"leased_at":        nil,
			"lease_expires_at": nil,
		})
	if jobResult.RowsAffected > 0 {
		logging.Infof("Reset %d firecrawl job(s) stuck in 'leased' state", jobResult.RowsAffected)
	}

	tagJobResult := database.DB.Model(&models.TagJob{}).
		Where("status = ?", "leased").
		Updates(map[string]interface{}{
			"status":           "pending",
			"leased_at":        nil,
			"lease_expires_at": nil,
		})
	if tagJobResult.RowsAffected > 0 {
		logging.Infof("Reset %d tag job(s) stuck in 'leased' state", tagJobResult.RowsAffected)
	}
}

func StartRuntime() *Runtime {
	resetStaleStates()

	// Fire the one-shot AI model health probe asynchronously so it never blocks
	// startup. The in-memory snapshot starts not-ready (Healthy()==false), so
	// workers/IsPaused treat the startup-race window as paused until the probe
	// completes and flips the snapshot to its real verdict (spec: 启动竞态期分析不跑).
	autoStart, _ := aisettings.LoadAutoStartModelsConfig()
	go aihealth.RunStartupProbe(context.Background(), airouter.NewStore(database.DB), autoStart)
	// Background self-heal timer: while the snapshot is not healthy it
	// re-probes at reprobeInterval (default 60s) until the model layer comes up
	// (slow-loading local models can outlast the startup probe's poll window),
	// then idles. autoStart is re-read per tick so a mid-run switch change
	// takes effect without restart.
	go aihealth.StartPeriodicReprobe(context.Background(), airouter.NewStore(database.DB), func() bool {
		v, _ := aisettings.LoadAutoStartModelsConfig()
		return v
	})

	tagging.StartAllWorkers()

	registry := admin.NewSchedulerRegistry()

	// Register each scheduler using the BaseScheduler factory pattern.
	// Each scheduler is configured with its JobFunc, interval, startup delay,
	// and optional TaskPersistence for DB state tracking.

	registry.Register("log_cleanup", scheduler.New(scheduler.Config{
		Name:         "Log Cleanup",
		Description:  "Clean up expired ai_call_logs and otel_spans rows",
		Interval:     86400 * time.Second,
		StartupDelay: 5 * time.Minute,
		Job:          admin.LogCleanupJob,
		Persistence: admin.NewTaskPersistence("log_cleanup",
			"清理过期的 AI 调用日志和追踪数据"),
	}))

	registry.Register("aux_label_cleanup", scheduler.New(scheduler.Config{
		Name:         "Aux Label Cleanup",
		Description:  "Clean up auxiliary labels with no active topic_tag references",
		Interval:     3600 * time.Second,
		StartupDelay: 10 * time.Minute,
		Job:          admin.AuxLabelCleanupJob,
		Persistence: admin.NewTaskPersistence("aux_label_cleanup",
			"清理无活跃标签引用的辅助标签"),
	}))

	registry.Register("blocked_article_recovery", scheduler.New(scheduler.Config{
		Name:        "Blocked Article Recovery",
		Description: "Recover articles stuck in blocked state",
		Interval:    3600 * time.Second,
		Job:         admin.BlockedArticleRecoveryJob,
		Persistence: admin.NewTaskPersistence("blocked_article_recovery",
			"恢复被阻塞的文章"),
	}))

	// Medium schedulers: with SchedulerTask DB persistence

	// preference-vector-feed-discovery: 偏好向量画像重算（D1，零 LLM/embedding）。
	registry.Register("preference_profile_update", scheduler.New(scheduler.Config{
		Name:        "Preference Profile Update",
		Description: "重算偏好向量画像（行为加权标签向量质心，按版块分桶）",
		Interval:    3600 * time.Second,
		Job:         admin.PreferenceProfileUpdateJob,
		Persistence: admin.NewTaskPersistence("preference_profile_update",
			"重算偏好向量画像"),
	}))

	// preference-vector-feed-discovery: RSSHub 路由目录同步（D2/D8，自建实例 /api/namespace）。
	registry.Register("rsshub_catalog_sync", scheduler.New(scheduler.Config{
		Name:        "RSSHub Catalog Sync",
		Description: "同步 RSSHub 路由目录（自建实例 /api/namespace）",
		Interval:    24 * time.Hour,
		Job:         admin.RSSHubCatalogSyncJob,
		Persistence: admin.NewTaskPersistence("rsshub_catalog_sync",
			"同步 RSSHub 路由目录"),
	}))

	registry.Register("tag_quality_score", scheduler.New(scheduler.Config{
		Name:        "Tag Quality Score",
		Description: "Recompute persistent quality scores for topic tags",
		Interval:    3600 * time.Second,
		Job:         scheduler.PauseAware(admin.TagQualityScoreJob),
		Persistence: admin.NewTaskPersistence("tag_quality_score",
			"Recompute persistent quality scores for topic tags"),
	}))

	registry.Register("auto_refresh", scheduler.New(scheduler.Config{
		Name:        "Auto Refresh",
		Description: "Auto-refresh RSS feeds",
		Interval:    60 * time.Second,
		Job:         admin.AutoRefreshJob,
		Persistence: admin.NewTaskPersistence("auto_refresh",
			"Auto-refresh RSS feeds"),
	}))

	// Complex schedulers
	content.InitContentCompletionHandler()
	registry.Register("content_completion", scheduler.New(scheduler.Config{
		Name:        "Content Completion",
		Description: "Complete article content and generate article summaries",
		TaskName:    "ai_summary",
		Aliases:     []string{"ai_summary"},
		Interval:    60 * time.Second,
		Job:         scheduler.PauseAware(admin.ContentCompletionJob(content.GetContentCompletionService())),
		Persistence: admin.NewTaskPersistence("ai_summary",
			"Complete article content and generate article summaries"),
	}))

	// DailyReport: wrapped with TriggerNowWithDate support
	dailyReportNextRunFn := scheduler.NextDailyReportTime
	dailyReportBase := scheduler.New(scheduler.Config{
		Name:        "Daily Report",
		Description: "Generate daily reports for all active semantic boards",
		NextRun:     dailyReportNextRunFn,
		Job:         scheduler.PauseAware(admin.DailyReportJob()), // uses current time at each execution
		Persistence: admin.NewTaskPersistenceWithNextRun("daily_report",
			"Generate daily reports for all active semantic boards",
			dailyReportNextRunFn),
	})
	dailyReportWrapper := admin.NewDailyReportSchedulerWrapper(dailyReportBase)
	registry.Register("daily_report", dailyReportWrapper)

	// BoardUpgradeSuggest: daily discover_new generation + watch GC (design D4).
	// Loosely coupled fixed-time trigger (default 06:30), not chained to the
	// daily report. Manual trigger via POST /upgrade-suggestions/generate.
	boardUpgradeNextRunFn := scheduler.NextBoardUpgradeSuggestTime
	registry.Register("relation_expire", scheduler.New(scheduler.Config{
		Name:         "Cross-Board Relation Expire",
		Description:  "Batch-mark expired confirmed cross-board relations",
		Interval:     3600 * time.Second,
		StartupDelay: 30 * time.Minute,
		Job:          dataenrichment.RelationExpireJob(dataenrichment.GetRepo()),
		Persistence: admin.NewTaskPersistence("relation_expire",
			"批量标记过期的已确认跨版块关系"),
	}))

	registry.Register("board_upgrade_suggest", scheduler.New(scheduler.Config{
		Name:        "Board Upgrade Suggest",
		Description: "每日生成版块升级建议 + 观察池 watch GC",
		NextRun:     boardUpgradeNextRunFn,
		Job:         scheduler.PauseAware(admin.BoardUpgradeSuggestJob()),
		Persistence: admin.NewTaskPersistenceWithNextRun("board_upgrade_suggest",
			"每日生成版块升级建议(discover_new)+观察池GC",
			boardUpgradeNextRunFn),
	}))

	// Firecrawl: with custom status enricher
	firecrawlQueue := content.NewFirecrawlJobQueue(database.DB)
	registry.Register("firecrawl", scheduler.New(scheduler.Config{
		Name:         "Firecrawl Crawler",
		Description:  "Auto-crawl full content for articles",
		Interval:     300 * time.Second,
		StartupDelay: 0,
		Job:          scheduler.PauseAware(admin.FirecrawlJob(firecrawlQueue, "scheduled")),
		StatusDetail: admin.FirecrawlStatusEnricher(),
		Persistence: admin.NewTaskPersistence("firecrawl",
			"自动爬取文章全文"),
	}))

	// ── Lifeline context schedulers (cycle A) ──────────────────────────────────
	// The repository, cycle-A service, cycle-B orchestrator, and HTTP handler
	// singleton are all wired by dataenrichment.Init in main.go (which runs
	// BEFORE SetupRoutes). StartRuntime only registers the schedulers below.
	lifelineSvc := dataenrichment.GetLifelineService()
	lister := dataenrichment.GetTopicLister()

	// Weekly lifeline: DISABLED (fix-board-analysis-material 7.3). Near-
	// horizon memory is served by the 14-day section window injected with
	// every analysis; long-horizon memory by month/year archives maintained
	// by the monthly/yearly jobs + the analysis-time completeness gate. The
	// weekly fleet-wide heal was also a burst risk (hundreds of LLM calls).
	// Registration stays commented for reference; existing week rows remain
	// consumable via the situation-card chain.
	_ = dataenrichment.NextWeeklyLifelineTime // keep helper referenced
	// registry.Register("lifeline_weekly", scheduler.New(scheduler.Config{
	// 	Name:        "Lifeline Weekly Refresh",
	// 	Description: "每周一刷新所有活跃话题的周度新闻汇总（循环A，含历史回填）",
	// 	NextRun:     dataenrichment.NextWeeklyLifelineTime,
	// 	Job:         scheduler.PauseAware(dataenrichment.WeeklyLifelineJob(lifelineSvc, lister)),
	// 	Persistence: admin.NewTaskPersistenceWithNextRun("lifeline_weekly",
	// 		"每周一刷新所有活跃话题的周度新闻汇总上下文", dataenrichment.NextWeeklyLifelineTime),
	// }))

	// Monthly lifeline: every 1st of month 03:30 Asia/Shanghai.
	monthlyNextRun := dataenrichment.NextMonthlyLifelineTime
	registry.Register("lifeline_monthly", scheduler.New(scheduler.Config{
		Name:        "Lifeline Monthly Refresh",
		Description: "每月1号刷新所有活跃话题的月度新闻汇总（循环A，含历史回填）",
		NextRun:     monthlyNextRun,
		Job:         scheduler.PauseAware(dataenrichment.MonthlyLifelineJob(lifelineSvc, lister)),
		Persistence: admin.NewTaskPersistenceWithNextRun("lifeline_monthly",
			"每月1号刷新所有活跃话题的月度新闻汇总上下文", monthlyNextRun),
	}))

	// Yearly lifeline: every Jan 1 04:00 Asia/Shanghai.
	yearlyNextRun := dataenrichment.NextYearlyLifelineTime
	registry.Register("lifeline_yearly", scheduler.New(scheduler.Config{
		Name:        "Lifeline Yearly Refresh",
		Description: "每年1月1号刷新所有活跃话题的年度新闻汇总（循环A，含历史回填）",
		NextRun:     yearlyNextRun,
		Job:         scheduler.PauseAware(dataenrichment.YearlyLifelineJob(lifelineSvc, lister)),
		Persistence: admin.NewTaskPersistenceWithNextRun("lifeline_yearly",
			"每年1月1号刷新所有活跃话题的年度新闻汇总上下文", yearlyNextRun),
	}))

	// Set global registry for handler access
	admin.SetRegistry(registry)
	content.SetSchedulerLookup(func(name string) interface{} {
		s, _ := registry.Get(name)
		return s
	})

	registry.StartAll()

	return &Runtime{Registry: registry}
}

func SetupGracefulShutdown(runtime *Runtime) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logging.Infof("Received signal: %v, shutting down gracefully...", sig)

		tagging.StopAllWorkers()

		if runtime.Registry != nil {
			runtime.Registry.StopAll(30 * time.Second)
		}

		logging.Infoln("Graceful shutdown completed")
		os.Exit(0)
	}()
}
