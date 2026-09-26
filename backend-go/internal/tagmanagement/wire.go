package tagging

import (
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/tagmanagement/handler"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
	"syntopica-backend/internal/tagmanagement/service/board"
	"syntopica-backend/internal/tagmanagement/service/sourcestats"
	"syntopica-backend/internal/tagmanagement/service/watched"
)

// Register domain-owned models with AutoMigrate via the injection registry.
// Done here (domain root) instead of tagmanagement/models because
// platform/database's postgres_migrations.go imports tagmanagement/models
// (seed + dup-merge migration logic), which would close a cycle if
// tagmanagement/models also imported platform/database.
func init() {
	// Migration hooks (see database/postgres_migrations.go): the tag models
	// own the seed/dup-merge logic; database calls back via these hooks.
	database.EmbeddingConfigSeeder = func(db *gorm.DB, defaults []database.EmbeddingConfigDefault) error {
		conv := make([]struct {
			Key         string
			Value       string
			Description string
		}, len(defaults))
		for i, d := range defaults {
			conv[i] = struct {
				Key         string
				Value       string
				Description string
			}{d.Key, d.Value, d.Description}
		}
		return tagmodels.SeedEmbeddingConfigs(db, conv)
	}
	database.AuxLabelDupMerge = tagmodels.RunAuxLabelDupMerge
	database.RegisterModels(
		&tagmodels.BoardComposition{},
		&tagmodels.BoardUpgradeSuggestion{},
		&tagmodels.CompositeComponent{},
		&tagmodels.EmbeddingConfig{},
		&tagmodels.MergeReembeddingQueue{},
		&tagmodels.TagCategoryMeta{},
		&tagmodels.TopicTagAnalysis{},
		&tagmodels.TopicTagBoardLabel{},
		&tagmodels.TopicTagEmbedding{},
		&tagmodels.TopicTagSemanticLabel{},
		&tagmodels.TagMergeSuggestion{},
	)
}

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

// ============================================================================
// Cross-domain facade additions (decouple-backend-domains Batch 3):
// admin (poll_handler, board upgrade job, tag quality job) and reader
// (article_handler, feed_board_stats_handler) previously reached into
// tagmanagement/handler + service/{watched,sourcestats,board} subpackages
// directly. They now consume these root re-exports only.
// ============================================================================

// Tag queue status snapshot (was: admin → tagmanagement/handler deep path).
type TagQueueStatusCounts = handler.TagQueueStatusCounts

var TagQueueStatusSnapshot = handler.TagQueueStatusSnapshot

// Watched tags expansion (was: reader → tagmanagement/service/watched).
var GetWatchedTagIDsExpanded = watched.GetWatchedTagIDsExpanded

// Feed×board hit stats (was: reader → tagmanagement/service/sourcestats).
type FeedStat = sourcestats.FeedStat

var (
	FeedBoardHitStats = sourcestats.FeedBoardHitStats
	ParseWindow       = sourcestats.ParseWindow
)

// Quality scoring (was: admin job → tagmanagement/service).
var ComputeAllQualityScores = service.ComputeAllQualityScores

// Semantic board upgrade (was: admin job → tagmanagement/service/board).
type (
	UpgradeGenerateRequest = board.UpgradeGenerateRequest
)

const (
	UpgradeDirectionCreate = board.UpgradeDirectionCreate
	UpgradeSourceAux       = board.UpgradeSourceAux
	UpgradeSourceComposite = board.UpgradeSourceComposite
)

var (
	NewSemanticBoardUpgradeService    = board.NewSemanticBoardUpgradeService
	NewDefaultSemanticBoardUpgradeLLM = board.NewDefaultSemanticBoardUpgradeLLM
)
