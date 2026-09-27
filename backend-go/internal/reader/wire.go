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
