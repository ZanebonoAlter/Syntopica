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
	"syntopica-backend/internal/admin/service"
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

// Preference profile handlers (preference-vector-feed-discovery)
var (
	GetPreferenceProfile       = handler.GetPreferenceProfile
	RecomputePreferenceProfile = handler.RecomputePreferenceProfile
)

// Discovery handlers (preference-vector-feed-discovery)
var (
	SyncCatalog            = handler.SyncCatalog
	GetCatalogStatus       = handler.GetCatalogStatus
	GetRecommendations     = handler.GetRecommendations
	RefreshRecommendations = handler.RefreshRecommendations
	AcceptRecommendation   = handler.AcceptRecommendation
	DismissRecommendation  = handler.DismissRecommendation
	ExcludeRecommendation  = handler.ExcludeRecommendation
	RestoreRecommendation  = handler.RestoreRecommendation
	Ask                    = handler.Ask
	GetDiscoveryRun        = handler.GetDiscoveryRun
	GetInterests           = handler.GetInterests
	GetRSSHubSettings      = handler.GetRSSHubSettings
	SaveRSSHubSettings     = handler.SaveRSSHubSettings
	GetProxySettings       = handler.GetProxySettings
	SaveProxySettings      = handler.SaveProxySettings
	GetBochaSettings       = handler.GetBochaSettings
	SaveBochaSettings      = handler.SaveBochaSettings
)

// Route param option dictionary handlers (feed-param-options)
var (
	ListRouteParamOptions  = handler.ListRouteParamOptions
	CreateRouteParamOption = handler.CreateRouteParamOption
	UpdateRouteParamOption = handler.UpdateRouteParamOption
	DeleteRouteParamOption = handler.DeleteRouteParamOption
)

// Candidate catalog handlers (improve-discovery-recommendations)
var (
	ListCandidates   = handler.ListCandidates
	CreateCandidate  = handler.CreateCandidate
	GetCandidate     = handler.GetCandidate
	UpdateCandidate  = handler.UpdateCandidate
	ExportCandidates = handler.ExportCandidates
	// 可用性检查（3.3，design D7）
	CheckCandidate = handler.CheckCandidate
	// 私网访问授权确认（Medium 7，design D9）
	ConfirmCandidateAccess = handler.ConfirmCandidateAccess
	// 目录导入导出（3.2，design D8）
	PreviewCatalogImport = handler.PreviewCatalogImport
	ConfirmCatalogImport = handler.ConfirmCatalogImport
)

// ============================================================================
// Scheduler re-exports (types and constructors used in internal/app/runtime.go)
// ============================================================================

// SchedulerRegistry is the registry that manages named scheduler instances.
type SchedulerRegistry = scheduler.Registry

var (
	NewSchedulerRegistry = scheduler.NewRegistry
)

// BaseScheduler types and constructors for the factory pattern.
// Runtime uses scheduler.New(scheduler.Config{...}) directly.
var (
	NewBaseScheduler              = scheduler.New
	NewTaskPersistence            = scheduler.NewTaskPersistence
	NewTaskPersistenceWithNextRun = scheduler.NewTaskPersistenceWithNextRun
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

// improve-discovery-recommendations 4.6：discovery_v2 开关读取（runtime 决定是否
// 注册三个后台任务；服务入口用同一读取函数拒绝已关闭的检查/回补调用）。
var DiscoveryV2Enabled = service.LoadDiscoveryV2Enabled
