package app

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"my-robot-backend/internal/app/runtimeinfo"
	"my-robot-backend/internal/domain/contentprocessing"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/domain/topicanalysis"
	"my-robot-backend/internal/domain/topicextraction"
	"my-robot-backend/internal/jobs"
	"my-robot-backend/internal/platform/database"
	"my-robot-backend/internal/platform/logging"
)

type Runtime struct {
	AutoRefresh            *jobs.AutoRefreshScheduler
	PreferenceUpdate       *jobs.PreferenceUpdateScheduler
	ContentCompletion      *jobs.ContentCompletionScheduler
	Firecrawl              *jobs.FirecrawlScheduler
	BlockedArticleRecovery *jobs.BlockedArticleRecoveryScheduler
	TagQualityScore        *jobs.TagQualityScoreScheduler
	NarrativeSummary       *jobs.NarrativeSummaryScheduler
	TagHierarchyCleanup    *jobs.TagHierarchyCleanupScheduler
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

	// Initialize hierarchy template manager
	if err := topicanalysis.GetHierarchyManager().LoadFromDB(); err != nil {
		logging.Warnf("Failed to load hierarchy templates: %v", err)
	} else {
		logging.Infoln("Hierarchy template manager initialized successfully")
	}

	// Start the tag queue worker for async article tagging
	if err := topicextraction.GetTagQueue().Start(); err != nil {
		logging.Warnf("Failed to start tag queue: %v", err)
	} else {
		logging.Infoln("Tag queue started successfully")
	}

	// Start the embedding queue worker for async embedding generation
	topicanalysis.StartEmbeddingQueueWorker()
	logging.Infoln("Embedding queue worker started successfully")
	topicanalysis.StartMergeReembeddingQueueWorker()
	logging.Infoln("Merge re-embedding queue worker started successfully")
	topicanalysis.StartAbstractTagUpdateQueueWorker()
	logging.Infoln("Abstract tag update queue worker started successfully")
	topicanalysis.StartAdoptNarrowerQueueWorker()
	logging.Infoln("Adopt narrower queue worker started successfully")

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

	crawlServiceURL := os.Getenv("CRAWL_SERVICE_URL")
	if crawlServiceURL == "" {
		crawlServiceURL = "http://localhost:11235"
	}
	contentprocessing.InitContentCompletionHandler(crawlServiceURL)

	runtime.ContentCompletion = jobs.NewContentCompletionScheduler(
		contentprocessing.GetContentCompletionService(),
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

	runtime.TagHierarchyCleanup = jobs.NewTagHierarchyCleanupScheduler(86400)
	if err := runtime.TagHierarchyCleanup.Start(); err != nil {
		logging.Warnf("Failed to start tag hierarchy cleanup scheduler: %v", err)
	} else {
		logging.Infoln("Tag hierarchy cleanup scheduler started successfully")
	}

	runtimeinfo.AutoRefreshSchedulerInterface = runtime.AutoRefresh
	runtimeinfo.PreferenceUpdateSchedulerInterface = runtime.PreferenceUpdate
	runtimeinfo.ContentCompletionSchedulerInterface = runtime.ContentCompletion
	runtimeinfo.FirecrawlSchedulerInterface = runtime.Firecrawl
	runtimeinfo.TagQualityScoreSchedulerInterface = runtime.TagQualityScore
	runtimeinfo.NarrativeSummarySchedulerInterface = runtime.NarrativeSummary
	runtimeinfo.TagHierarchyCleanupSchedulerInterface = runtime.TagHierarchyCleanup

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
			logging.Infoln("Stopping tag queue...")
			topicextraction.GetTagQueue().Stop()

			logging.Infoln("Stopping embedding queue worker...")
			topicanalysis.StopEmbeddingQueueWorker()

			logging.Infoln("Stopping merge re-embedding queue worker...")
			topicanalysis.StopMergeReembeddingQueueWorker()

			logging.Infoln("Stopping abstract tag update queue worker...")
			topicanalysis.StopAbstractTagUpdateQueueWorker()

			logging.Infoln("Stopping adopt narrower queue worker...")
			topicanalysis.StopAdoptNarrowerQueueWorker()

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

			if runtime.TagHierarchyCleanup != nil {
				logging.Infoln("Stopping tag hierarchy cleanup scheduler...")
				runtime.TagHierarchyCleanup.Stop()
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
