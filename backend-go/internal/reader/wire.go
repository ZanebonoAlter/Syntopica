package reader

import (
	"gorm.io/gorm"
	"syntopica-backend/internal/reader/handler"
	"syntopica-backend/internal/reader/repository"
	"syntopica-backend/internal/reader/service"
)

// ============================================================================
// Repository singleton (delegated to sub-package)
// ============================================================================

// Repo is the package-level repository singleton, initialized by InitRepository.
var Repo = repository.Repo

// InitRepository initializes the repository sub-package.
func InitRepository(db *gorm.DB) {
	repository.InitRepository(db)
	Repo = repository.Repo
}

// ============================================================================
// Re-exports from repository sub-package
// ============================================================================

// FirecrawlJobQueue is a re-export.
type FirecrawlJobQueue = repository.FirecrawlJobQueue

// NewFirecrawlJobQueue is a re-export.
var NewFirecrawlJobQueue = repository.NewFirecrawlJobQueue

// ============================================================================
// Re-exports from service sub-package
// ============================================================================

// ContentCompletionService is a re-export.
type ContentCompletionService = service.ContentCompletionService

// ContentCompletionArticleRef is a re-export.
type ContentCompletionArticleRef = service.ContentCompletionArticleRef

// ToArticleRef is a re-export.
var ToArticleRef = service.ToArticleRef

// FeedService is a re-export.
type FeedService = service.FeedService

// NewFeedService is a re-export.
var NewFeedService = service.NewFeedService

// IconStorageDir is a re-export.
var IconStorageDir = service.IconStorageDir

// FirecrawlConfig is a re-export.
type FirecrawlConfig = service.FirecrawlConfig

// NewFirecrawlService is a re-export.
var NewFirecrawlService = service.NewFirecrawlService

// Crawler is a re-export of the neutral content-crawler interface.
type Crawler = service.Crawler

// NewReadabilityCrawler is a re-export of the in-process readability crawler constructor.
var NewReadabilityCrawler = service.NewReadabilityCrawler

// NewFallbackCrawler is a re-export of the readability→firecrawl fallback chain constructor.
var NewFallbackCrawler = service.NewFallbackCrawler

// NewContentCompletionService is a re-export.
var NewContentCompletionService = service.NewContentCompletionService

// GetFirecrawlConfig is a re-export.
var GetFirecrawlConfig = service.GetFirecrawlConfig

// ============================================================================
// Re-exports from handler sub-package
// ============================================================================

// InitContentCompletionHandler is a re-export.
var InitContentCompletionHandler = handler.InitContentCompletionHandler

// SetSchedulerLookup is a re-export.
var SetSchedulerLookup = handler.SetSchedulerLookup

// GetContentCompletionService is a re-export.
var GetContentCompletionService = handler.GetContentCompletionService

// ============================================================================
// Cross-domain facade additions (decouple-backend-domains Batch 3):
// discovery (candidate check / recommendation accept) and dataenrichment
// (fetch_page) previously reached into reader/service directly.
// ============================================================================

type (
	// FeedCreateOptions is a re-export.
	FeedCreateOptions = service.FeedCreateOptions
	// ReadabilityCrawler is a re-export.
	ReadabilityCrawler = service.ReadabilityCrawler
)

var (
	NewFeedCreateService = service.NewFeedCreateService
	ParseFeedBody        = service.ParseFeedBody
)

// ============================================================================
// Re-exports for the AI-generated-article-detection domain
// (add-ai-generated-article-detection)
// ============================================================================
//
// 判定服务的对外入口：域外（判定 worker / 人工标记 handler）只 import 读者域根门面，
// 不碰 service/ 深路径（spec backend-package-boundaries）。

// AIDetectionService is a re-export.
type AIDetectionService = service.AIDetectionService

// DetectionJobQueue is a re-export: 疑似 AI 判定作业队列（与 tag_jobs 相互分离）。
type DetectionJobQueue = repository.DetectionJobQueue

// DetectionJobRequest is a re-export.
type DetectionJobRequest = repository.DetectionJobRequest

// NewDetectionJobQueue is a re-export.
var NewDetectionJobQueue = repository.NewDetectionJobQueue

// IsForceDetectionReason is a re-export: 作业是否为显式重判（manual_recheck）。
var IsForceDetectionReason = repository.IsForceDetectionReason

// EnqueueAIDetectionJob is a re-export: 统一入队辅助函数（判 feed.ai_detection_enabled）。
var EnqueueAIDetectionJob = service.EnqueueAIDetectionJob

// RequestAIDetectionRecheck is a re-export: 显式重新判定入口（绕过人工结论跳过）。
var RequestAIDetectionRecheck = service.RequestAIDetectionRecheck

// StartAIDetectionWorker is a re-export: 启动判定 worker（应用启动注册）。
var StartAIDetectionWorker = service.StartAIDetectionWorker

// StopAIDetectionWorker is a re-export: 停止判定 worker（优雅关停注册）。
var StopAIDetectionWorker = service.StopAIDetectionWorker

// AIDetectionResult is a re-export.
type AIDetectionResult = service.AIDetectionResult

// NewAIDetectionService is a re-export.
var NewAIDetectionService = service.NewAIDetectionService

// HasManualAIDetection is a re-export: reports whether the article already carries
// a manual conclusion (人工结论优先，自动判定不得覆盖).
var HasManualAIDetection = service.HasManualAIDetection

// SelectAIDetectionInput is a re-export: firecrawl_content → content → description.
var SelectAIDetectionInput = service.SelectAIDetectionInput

// BuildAIDetectionSample is a re-export: head + tail runes sample.
var BuildAIDetectionSample = service.BuildAIDetectionSample

// ParseAIDetectionScore is a re-export: extracts the 0-1 confidence score.
var ParseAIDetectionScore = service.ParseAIDetectionScore

// PersistAIDetectionResult is a re-export: writes the verdict onto the article row.
var PersistAIDetectionResult = service.PersistAIDetectionResult

// AIDetection 判定常量 re-export（design D14 命名契约）。
const (
	// AIDetectionStatusUnknown is a re-export: 未判定.
	AIDetectionStatusUnknown = service.AIDetectionStatusUnknown
	// AIDetectionStatusSuspect is a re-export: 疑似 AI 生成.
	AIDetectionStatusSuspect = service.AIDetectionStatusSuspect
	// AIDetectionStatusNotAI is a re-export: 已确认非 AI.
	AIDetectionStatusNotAI = service.AIDetectionStatusNotAI
	// AIDetectionSourceLLM is a re-export: 自动判定.
	AIDetectionSourceLLM = service.AIDetectionSourceLLM
	// AIDetectionSourceManual is a re-export: 人工标记.
	AIDetectionSourceManual = service.AIDetectionSourceManual
	// AIDetectionOperation is a re-export: 判定调用的 operation 名（进审计日志）。
	AIDetectionOperation = service.AIDetectionOperation

	// AIDetectionReasonFirecrawlCompleted is a re-export: 全文抓取成功后入队。
	AIDetectionReasonFirecrawlCompleted = service.AIDetectionReasonFirecrawlCompleted
	// AIDetectionReasonFirecrawlFailed is a re-export: 全文抓取终态失败降级后入队。
	AIDetectionReasonFirecrawlFailed = service.AIDetectionReasonFirecrawlFailed
	// AIDetectionReasonArticleIngested is a re-export: feed 未开全文抓取时入库即入队。
	AIDetectionReasonArticleIngested = service.AIDetectionReasonArticleIngested
	// AIDetectionReasonManualRecheck is a re-export: 显式重新判定入队。
	AIDetectionReasonManualRecheck = service.AIDetectionReasonManualRecheck
)

var (
	// ErrAIDetectionManualOverride is a re-export: 人工结论优先，自动判定被跳过。
	ErrAIDetectionManualOverride = service.ErrAIDetectionManualOverride
	// ErrAIDetectionNoInput is a re-export: 文章没有可用的判定输入。
	ErrAIDetectionNoInput = service.ErrAIDetectionNoInput
)
