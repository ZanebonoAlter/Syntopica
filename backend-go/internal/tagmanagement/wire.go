package tagging

import (
	"gorm.io/gorm"

	"syntopica-backend/internal/tagmanagement/handler"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
)

// ============================================================================
// Package-level wiring
// ============================================================================

var Repo *repository.TagManagementRepository

func InitRepository(db *gorm.DB) {
	repository.InitRepository(db)
	Repo = repository.Repo
}

func NewTagManagementRepository(db *gorm.DB) *repository.TagManagementRepository {
	return repository.NewTagManagementRepository(db)
}

// ============================================================================
// Type aliases from service/
// ============================================================================

type (
	TopicTag                 = service.TopicTag
	AggregatedTopicTag       = service.AggregatedTopicTag
	ExtractedTag             = service.ExtractedTag
	ExtractionInput          = service.ExtractionInput
	TopicTagSummary          = service.TopicTagSummary
	PendingArticle           = service.PendingArticle
	GetTopicArticlesParams   = service.GetTopicArticlesParams
	TagResolutionRequest     = service.TagResolutionRequest
	TagResolutionResponse    = service.TagResolutionResponse
	SimilarTagInfo           = service.SimilarTagInfo
	SimilarityEdge           = service.SimilarityEdge
	TagMatchResult           = service.TagMatchResult
	TagCandidate             = service.TagCandidate
	SemanticBoardMatchResult = service.SemanticBoardMatchResult
	AuxLabelGCMode           = service.AuxLabelGCMode
	AuxLabelGCRequest        = service.AuxLabelGCRequest
	EdgeGCRequest            = service.EdgeGCRequest
	EdgeGCResult             = service.EdgeGCResult
)

const (
	AuxLabelGCModeDryRun      = service.AuxLabelGCModeDryRun
	AuxLabelGCModeDisable     = service.AuxLabelGCModeDisable
	AuxLabelGCModeDelete      = service.AuxLabelGCModeDelete
	AuxLabelGCModeRecalculate = service.AuxLabelGCModeRecalculate

	DefaultTagEdgeRetentionDays = service.DefaultTagEdgeRetentionDays
	TagEdgeRetentionDaysKey     = service.TagEdgeRetentionDaysKey
)

// ============================================================================
// Function / variable re-exports
// ============================================================================

var (
	ParseAnchorDate = service.ParseAnchorDate
	ResolveWindow   = service.ResolveWindow
	StartAllWorkers = service.StartAllWorkers
	StopAllWorkers  = service.StopAllWorkers
)

var (
	FeedCategoryName          = service.FeedCategoryName
	GetArticleTags            = service.GetArticleTags
	CleanupOrphanedTags       = service.CleanupOrphanedTags
	NormalizeDisplayCategory  = service.NormalizeDisplayCategory
	RegisterVectorDimEnsurer  = service.RegisterVectorDimEnsurer
	EnsureVectorDimensionOnce = service.EnsureVectorDimensionOnce
)

var (
	EdgeGC                   = service.EdgeGC
	LoadTagEdgeRetentionDays = service.LoadTagEdgeRetentionDays
)

var (
	NewTagJobQueue = repository.NewTagJobQueue
)

// Re-export TagJobRequest from repository
type TagJobRequest = repository.TagJobRequest

var (
	NewAuxiliaryLabelService        = service.NewAuxiliaryLabelService
	NewSemanticBoardMatchingService = service.NewSemanticBoardMatchingService
	MatchTier                       = handler.MatchTier
)

// ============================================================================
// Handler route registrations
// ============================================================================

var (
	RegisterWatchedTagsRoutes           = handler.RegisterWatchedTagsRoutes
	RegisterTagManagementRoutes         = handler.RegisterTagManagementRoutes
	RegisterTagMergePreviewRoutes       = handler.RegisterTagMergePreviewRoutes
	RegisterTagQueueRoutes              = handler.RegisterTagQueueRoutes
	RegisterSemanticBoardRoutes         = handler.RegisterSemanticBoardRoutes
	RegisterEmbeddingQueueRoutes        = handler.RegisterEmbeddingQueueRoutes
	RegisterMergeReembeddingQueueRoutes = handler.RegisterMergeReembeddingQueueRoutes
)
