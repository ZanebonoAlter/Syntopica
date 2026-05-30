package app

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"syntopica-backend/internal/app/runtimeinfo"
	"syntopica-backend/internal/domain/content"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/domain/tagging"
	"syntopica-backend/internal/jobs"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

type Runtime struct {
	AutoRefresh            *jobs.AutoRefreshScheduler
	PreferenceUpdate       *jobs.PreferenceUpdateScheduler
	ContentCompletion      *jobs.ContentCompletionScheduler
	Firecrawl              *jobs.FirecrawlScheduler
	BlockedArticleRecovery *jobs.BlockedArticleRecoveryScheduler
	TagQualityScore        *jobs.TagQualityScoreScheduler
	NarrativeSummary       *jobs.NarrativeSummaryScheduler
	DailyReport            *jobs.DailyReportScheduler
	LogCleanup             *jobs.LogCleanupScheduler
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
	runtime := &Runtime{}

	resetStaleStates()

	tagging.StartAllWorkers()

	runtime.AutoRefresh = jobs.NewAutoRefreshScheduler(60)
	if err := runtime.AutoRefresh.Start(); err != nil {
		logging.Warnf("Failed to start auto-refresh scheduler: %v", err)
	} else {
		logging.Infoln("Auto-refresh scheduler started successfully")
	}

	preferenceUpdateInterval := 1800
	runtime.PreferenceUpdate = jobs.NewPreferenceUpdateScheduler(preferenceUpdateInterval)
	if err := runtime.PreferenceUpdate.Start(); err != nil {
		logging.Warnf("Failed to start preference update scheduler: %v", err)
	} else {
		logging.Infoln("Preference update scheduler started successfully")
	}

	runtime.Firecrawl = jobs.NewFirecrawlScheduler()
	if err := runtime.Firecrawl.Start(); err != nil {
		logging.Warnf("Failed to start firecrawl scheduler: %v", err)
	} else {
		logging.Infoln("Firecrawl scheduler started successfully")
	}

	content.InitContentCompletionHandler()

	runtime.ContentCompletion = jobs.NewContentCompletionScheduler(
		content.GetContentCompletionService(),
		60,
	)
	if err := runtime.ContentCompletion.Start(); err != nil {
		logging.Warnf("Failed to start content completion scheduler: %v", err)
	} else {
		logging.Infoln("Content completion scheduler started successfully")
	}

	// STAT-04: Blocked article recovery scheduler (hourly)
	runtime.BlockedArticleRecovery = jobs.NewBlockedArticleRecoveryScheduler(3600)
	if err := runtime.BlockedArticleRecovery.Start(); err != nil {
		logging.Warnf("Failed to start blocked article recovery scheduler: %v", err)
	} else {
		logging.Infoln("Blocked article recovery scheduler started successfully")
	}

	runtime.TagQualityScore = jobs.NewTagQualityScoreScheduler(3600)
	if err := runtime.TagQualityScore.Start(); err != nil {
		logging.Warnf("Failed to start tag quality score scheduler: %v", err)
	} else {
		logging.Infoln("Tag quality score scheduler started successfully")
	}

	runtime.NarrativeSummary = jobs.NewNarrativeSummaryScheduler(86400)
	if err := runtime.NarrativeSummary.Start(); err != nil {
		logging.Warnf("Failed to start narrative summary scheduler: %v", err)
	} else {
		logging.Infoln("Narrative summary scheduler started successfully")
	}

	runtime.LogCleanup = jobs.NewLogCleanupScheduler(86400)
	if err := runtime.LogCleanup.Start(); err != nil {
		logging.Warnf("Failed to start log cleanup scheduler: %v", err)
	} else {
		logging.Infoln("Log cleanup scheduler started successfully")
	}

	runtime.DailyReport = jobs.NewDailyReportScheduler(86400)
	if err := runtime.DailyReport.Start(); err != nil {
		logging.Warnf("Failed to start daily report scheduler: %v", err)
	} else {
		logging.Infoln("Daily report scheduler started successfully")
	}

	runtimeinfo.AutoRefreshSchedulerInterface = runtime.AutoRefresh
	runtimeinfo.PreferenceUpdateSchedulerInterface = runtime.PreferenceUpdate
	runtimeinfo.ContentCompletionSchedulerInterface = runtime.ContentCompletion
	runtimeinfo.FirecrawlSchedulerInterface = runtime.Firecrawl
	runtimeinfo.TagQualityScoreSchedulerInterface = runtime.TagQualityScore
	runtimeinfo.NarrativeSummarySchedulerInterface = runtime.NarrativeSummary
	runtimeinfo.LogCleanupSchedulerInterface = runtime.LogCleanup
	runtimeinfo.DailyReportSchedulerInterface = runtime.DailyReport

	return runtime
}

func SetupGracefulShutdown(runtime *Runtime) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logging.Infof("Received signal: %v, shutting down gracefully...", sig)

		done := make(chan struct{})
		go func() {
			tagging.StopAllWorkers()

			if runtime.AutoRefresh != nil {
				logging.Infoln("Stopping auto-refresh scheduler...")
				runtime.AutoRefresh.Stop()
			}

			if runtime.PreferenceUpdate != nil {
				logging.Infoln("Stopping preference update scheduler...")
				runtime.PreferenceUpdate.Stop()
			}

			if runtime.ContentCompletion != nil {
				logging.Infoln("Stopping content completion scheduler...")
				runtime.ContentCompletion.Stop()
			}

			if runtime.Firecrawl != nil {
				logging.Infoln("Stopping firecrawl scheduler...")
				runtime.Firecrawl.Stop()
			}

			if runtime.BlockedArticleRecovery != nil {
				logging.Infoln("Stopping blocked article recovery scheduler...")
				runtime.BlockedArticleRecovery.Stop()
			}

			if runtime.TagQualityScore != nil {
				logging.Infoln("Stopping tag quality score scheduler...")
				runtime.TagQualityScore.Stop()
			}

			if runtime.NarrativeSummary != nil {
				logging.Infoln("Stopping narrative summary scheduler...")
				runtime.NarrativeSummary.Stop()
			}

			if runtime.LogCleanup != nil {
				logging.Infoln("Stopping log cleanup scheduler...")
				runtime.LogCleanup.Stop()
			}

			if runtime.DailyReport != nil {
				logging.Infoln("Stopping daily report scheduler...")
				runtime.DailyReport.Stop()
			}

			close(done)
		}()

		select {
		case <-done:
			logging.Infoln("Graceful shutdown completed")
		case <-time.After(30 * time.Second):
			logging.Warnln("Graceful shutdown timed out after 30s, forcing exit")
		}
		os.Exit(0)
	}()
}
