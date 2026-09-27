// Package admin is the public facade for the internal/admin feature module.
// It re-exports symbols from handler/, service/, scheduler/, and repository/
// sub-packages so that external consumers (internal/app, cmd/server) can
// reference everything as admin.X without knowing the internal layout.
package admin

import (
	"gorm.io/gorm"

	"syntopica-backend/internal/admin/handler"
	"syntopica-backend/internal/admin/repository"
	"syntopica-backend/internal/admin/scheduler"
	platformscheduler "syntopica-backend/internal/platform/scheduler"
)

// ============================================================================
// Repository wiring
// ============================================================================

// InitRepository delegates to the repository sub-package.
func InitRepository(db *gorm.DB) {
	repository.InitRepository(db)
}

// SetRegistry sets the global scheduler registry for handler access.
func SetRegistry(reg handler.SchedulerRegistry) {
	handler.Reg = reg
}

// ============================================================================
// Handler re-exports (HTTP handlers registered in internal/app/router.go)
// ============================================================================

// AI provider / route / settings handlers
var (
	ListProviders  = handler.ListProviders
	UpsertProvider = handler.UpsertProvider
	UpdateProvider = handler.UpdateProvider
	DeleteProvider = handler.DeleteProvider
	ListRoutes     = handler.ListRoutes
	UpdateRoute    = handler.UpdateRoute
	GetSettings    = handler.GetSettings
	SaveSettings   = handler.SaveSettings
	TestConnection = handler.TestConnection
)

// AI model health handlers (ai-model-health)
var (
	GetAIHealth        = handler.GetAIHealth
	SetAutoStartModels = handler.SetAutoStartModels
	ReprobeAIHealth    = handler.ReprobeAIHealth
)

// Scheduler handlers
var (
	GetSchedulersStatus         = handler.GetSchedulersStatus
	GetPollBundle               = handler.GetPollBundle
	GetSchedulerStatus          = handler.GetSchedulerStatus
	TriggerScheduler            = handler.TriggerScheduler
	ResetSchedulerStats         = handler.ResetSchedulerStats
	UpdateSchedulerInterval     = handler.UpdateSchedulerInterval
	UpdateSchedulerScheduleTime = handler.UpdateSchedulerScheduleTime
	GetTasksStatus              = handler.GetTasksStatus
)

// Analysis pause handlers (pause-analysis)
var (
	GetAnalysisPause = handler.GetAnalysisPause
	SetAnalysisPause = handler.SetAnalysisPause
)

// Notification handlers (add-notification-center)
var (
	ListNotifications        = handler.ListNotifications
	GetUnreadCount           = handler.GetUnreadCount
	MarkNotificationRead     = handler.MarkNotificationRead
	MarkAllNotificationsRead = handler.MarkAllNotificationsRead
	ClearNotifications       = handler.ClearNotifications
)

// AI call log handlers
var (
	ListCallLogs = handler.ListCallLogs
	GetSession   = handler.GetSession
)

// Reading behavior handlers
var (
	TrackReadingBehavior      = handler.TrackReadingBehavior
	BatchTrackReadingBehavior = handler.BatchTrackReadingBehavior
	GetReadingStats           = handler.GetReadingStats
)

// ============================================================================
// Comtrade settings handlers（comtrade_settings_handler 留 admin 域）
var (
	GetComtradeSettings  = handler.GetComtradeSettings
	SaveComtradeSettings = handler.SaveComtradeSettings
)

// Scheduler re-exports (types and constructors used in internal/app/runtime.go)
// ============================================================================

// SchedulerRegistry is the registry that manages named scheduler instances.
type SchedulerRegistry = platformscheduler.Registry

var (
	NewSchedulerRegistry = platformscheduler.NewRegistry
)

// BaseScheduler types and constructors for the factory pattern.
// Runtime uses scheduler.New(scheduler.Config{...}) directly.
var (
	NewBaseScheduler              = platformscheduler.New
	NewTaskPersistence            = platformscheduler.NewTaskPersistence
	NewTaskPersistenceWithNextRun = platformscheduler.NewTaskPersistenceWithNextRun
	NextDailyReportTime           = scheduler.NextDailyReportTime
	NextBoardUpgradeSuggestTime   = scheduler.NextBoardUpgradeSuggestTime
)

// DailyReportSchedulerWrapper for TriggerNowWithDate support.
type DailyReportScheduler = scheduler.DailyReportSchedulerWrapper

var (
	NewDailyReportSchedulerWrapper = scheduler.NewDailyReportSchedulerWrapper
)

// Job functions (for use in runtime.go when creating schedulers).
var (
	LogCleanupJob              = scheduler.LogCleanupJob
	AuxLabelCleanupJob         = scheduler.AuxLabelCleanupJob
	BlockedArticleRecoveryJob  = scheduler.BlockedArticleRecoveryJob
	PreferenceProfileUpdateJob = scheduler.PreferenceProfileUpdateJob
	RSSHubCatalogSyncJob       = scheduler.RSSHubCatalogSyncJob
	TagQualityScoreJob         = scheduler.TagQualityScoreJob
	AutoRefreshJob             = scheduler.AutoRefreshJob
	ContentCompletionJob       = scheduler.ContentCompletionJob
	DailyReportJob             = scheduler.DailyReportJob
	BoardUpgradeSuggestJob     = scheduler.BoardUpgradeSuggestJob
	FirecrawlJob               = scheduler.FirecrawlJob
	FirecrawlStatusEnricher    = scheduler.FirecrawlStatusEnricher

	// improve-discovery-recommendations 4.6：发现 v2 三个后台任务
	// （检查=维护类、回补=分析类由 runtime 包 PauseAware、运行维护=维护类）。
	CandidateAvailabilityCheckJob = scheduler.CandidateAvailabilityCheckJob
	CandidateEmbeddingBackfillJob = scheduler.CandidateEmbeddingBackfillJob
	DiscoveryRunMaintenanceJob    = scheduler.DiscoveryRunMaintenanceJob
)
