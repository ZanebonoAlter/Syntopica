// Package service is a thin facade that re-exports symbols from its
// domain-focused sub-packages (core, board, merge, auxlabel).
// External consumers (handler/, wire.go) continue to import
// "syntopica-backend/internal/tagmanagement/service" and reference
// service.Xxx without knowing the internal split.
package service

import (
	"syntopica-backend/internal/tagmanagement/service/auxlabel"
	"syntopica-backend/internal/tagmanagement/service/board"
	"syntopica-backend/internal/tagmanagement/service/core"
	"syntopica-backend/internal/tagmanagement/service/merge"
)

// InvalidateMatchCache invalidates the semantic-board match cache (board
// auxiliaries/composites/embeddings). Handlers that write board_composition
// directly must call this — the caches have no TTL.
func InvalidateMatchCache() {
	board.InvalidateMatchCache()
}

// ============================================================================
// Type aliases from core/ (merged tagging + embedding)
// ============================================================================

type (
	TopicTag                     = core.TopicTag
	AggregatedTopicTag           = core.AggregatedTopicTag
	ExtractedTag                 = core.ExtractedTag
	ExtractionInput              = core.ExtractionInput
	TopicTagSummary              = core.TopicTagSummary
	PendingArticle               = core.PendingArticle
	GetTopicArticlesParams       = core.GetTopicArticlesParams
	TagResolutionRequest         = core.TagResolutionRequest
	TagResolutionResponse        = core.TagResolutionResponse
	SimilarTagInfo               = core.SimilarTagInfo
	SimilarityEdge               = core.SimilarityEdge
	TagMatchResult               = core.TagMatchResult
	TagCandidate                 = core.TagCandidate
	AuxiliaryLabel               = core.AuxiliaryLabel
	EmbeddingService             = core.EmbeddingService
	EmbeddingQueueService        = core.EmbeddingQueueService
	EmbeddingConfigService       = core.EmbeddingConfigService
	MergeReembeddingQueueService = core.MergeReembeddingQueueService
)

// ============================================================================
// Function / variable re-exports from core/
// ============================================================================

var (
	ParseAnchorDate         = core.ParseAnchorDate
	ResolveWindow           = core.ResolveWindow
	StartAllWorkers         = core.StartAllWorkers
	StopAllWorkers          = core.StopAllWorkers
	Slugify                 = core.Slugify
	UniqueSemanticLabelSlug = auxlabel.UniqueSemanticLabelSlug
	FloatsToPgVector        = core.FloatsToPgVector
)

var (
	FeedCategoryName          = core.FeedCategoryName
	GetArticleTags            = core.GetArticleTags
	CleanupOrphanedTags       = core.CleanupOrphanedTags
	NormalizeDisplayCategory  = core.NormalizeDisplayCategory
	RegisterVectorDimEnsurer  = auxlabel.RegisterVectorDimEnsurer
	EnsureVectorDimensionOnce = auxlabel.EnsureVectorDimensionOnce
)

var (
	ComputeAllQualityScores             = core.ComputeAllQualityScores
	NewEmbeddingService                 = core.NewEmbeddingService
	NewEmbeddingQueueService            = core.NewEmbeddingQueueService
	NewEmbeddingConfigService           = core.NewEmbeddingConfigService
	NewMergeReembeddingQueueService     = core.NewMergeReembeddingQueueService
	MergeReembeddingQueueFactory        = core.MergeReembeddingQueueFactory
	DefaultMergeReembeddingQueueFactory = core.DefaultMergeReembeddingQueueFactory
	BackfillPersonMetadata              = core.BackfillPersonMetadata
	EnqueueMergeReembedding             = core.EnqueueMergeReembedding
	MergeTags                           = core.MergeTags
)

// ============================================================================
// Type aliases from board/
// ============================================================================

type (
	SemanticBoardMatchResult           = board.SemanticBoardMatchResult
	SemanticBoardMatchConfig           = board.SemanticBoardMatchConfig
	SemanticBoardBackfillRequest       = board.SemanticBoardBackfillRequest
	SemanticBoardBackfillService       = board.SemanticBoardBackfillService
	SemanticBoardUpgradeService        = board.SemanticBoardUpgradeService
	UpgradeGenerateRequest             = board.UpgradeGenerateRequest
	SemanticBoardUpgradeCandidate      = board.SemanticBoardUpgradeCandidate
	SemanticBoardUpgradeCluster        = board.SemanticBoardUpgradeCluster
	SemanticBoardUpgradeConfig         = board.SemanticBoardUpgradeConfig
	SemanticBoardUpgradeDecision       = board.SemanticBoardUpgradeDecision
	SemanticBoardUpgradeSuggestion     = board.SemanticBoardUpgradeSuggestion
	SemanticBoardUpgradeLLM            = board.SemanticBoardUpgradeLLM
	ConfirmSemanticBoardUpgradeRequest = board.ConfirmSemanticBoardUpgradeRequest
	BoardAuxiliaryLabel                = board.BoardAuxiliaryLabel
	MatchDetailPair                    = board.MatchDetailPair
)

var (
	NewSemanticBoardMatchingService       = board.NewSemanticBoardMatchingService
	NewSemanticBoardBackfillService       = board.NewSemanticBoardBackfillService
	NewSemanticBoardUpgradeService        = board.NewSemanticBoardUpgradeService
	NewDefaultSemanticBoardUpgradeLLM     = board.NewDefaultSemanticBoardUpgradeLLM
	ComputeMatchDetail                    = board.ComputeMatchDetail
	CosineSimilarity                      = board.CosineSimilarity
	BuildSemanticBoardUpgradeSystemPrompt = board.BuildSemanticBoardUpgradeSystemPrompt
	ParsePgVector                         = auxlabel.ParsePgVector
)

// InvalidateMatchingConfigCache clears the cached matching config so the next
// LoadConfig call reads fresh values from the database.
func InvalidateMatchingConfigCache() {
	board.InvalidateMatchingConfigCache()
}

// ============================================================================
// Type / function re-exports from merge/
// ============================================================================

var (
	HardMergeTags = core.HardMergeTags
)

// ============================================================================
// Type / function re-exports from auxlabel/
// ============================================================================

type (
	AuxLabelGCMode              = auxlabel.AuxLabelGCMode
	AuxLabelGCRequest           = auxlabel.AuxLabelGCRequest
	AuxiliaryLabelService       = auxlabel.AuxiliaryLabelService
	AuxiliaryLabelEmbeddingMode = auxlabel.AuxiliaryLabelEmbeddingMode
	AuxiliaryLabelEmbedder      = auxlabel.AuxiliaryLabelEmbedder

	// Composite label lifecycle (add-composite-labels).
	CompositeLabelService      = auxlabel.CompositeLabelService
	CompositeLabelCreateResult = auxlabel.CompositeLabelCreateResult
	CompositeCreateOutcome     = auxlabel.CompositeCreateOutcome
	CompositeLabelView         = auxlabel.CompositeLabelView
	CompositeComponentView     = auxlabel.CompositeComponentView
	ComponentOptionView        = auxlabel.ComponentOptionView
	MountedBoardRef            = auxlabel.MountedBoardRef
)

const (
	SemanticBoardUpgradeDecisionCreateNew = board.SemanticBoardUpgradeDecisionCreateNew

	AuxLabelGCModeDryRun      = auxlabel.AuxLabelGCModeDryRun
	AuxLabelGCModeDisable     = auxlabel.AuxLabelGCModeDisable
	AuxLabelGCModeDelete      = auxlabel.AuxLabelGCModeDelete
	AuxLabelGCModeRecalculate = auxlabel.AuxLabelGCModeRecalculate

	CompositeMinComponents   = auxlabel.CompositeMinComponents
	CompositeMaxComponents   = auxlabel.CompositeMaxComponents
	CompositeOutcomeCreated  = auxlabel.CompositeOutcomeCreated
	CompositeOutcomeReusedL1 = auxlabel.CompositeOutcomeReusedL1
	CompositeOutcomeAliasL2  = auxlabel.CompositeOutcomeAliasL2
)

var NewCompositeLabelService = auxlabel.NewCompositeLabelService

// ============================================================================
// Function re-exports from merge/
// ============================================================================

type (
	ScanProgress     = merge.ScanProgress
	EvaluateProgress = merge.EvaluateProgress
)

var (
	StartFullScan              = merge.StartFullScan
	StartEvaluation            = merge.StartEvaluation
	WaitForScanChannel         = merge.WaitForScanChannel
	WaitForEvaluateChannel     = merge.WaitForEvaluateChannel
	IsScanRunning              = merge.IsScanRunning
	IsEvaluateRunning          = merge.IsEvaluateRunning
	CancelEvaluation           = merge.CancelEvaluation
	GetScanProgressChannel     = merge.GetScanProgressChannel
	GetEvaluateProgressChannel = merge.GetEvaluateProgressChannel
)

var (
	NewAuxiliaryLabelService           = auxlabel.NewAuxiliaryLabelService
	DefaultAuxiliaryLabelEmbedder      = auxlabel.DefaultAuxiliaryLabelEmbedder
	ValidateActiveAuxiliaryLabels      = board.ValidateActiveAuxiliaryLabels
	UniqueUintSlice                    = board.UniqueUintSlice
	AuxiliaryLabelEmbeddingModeStorage = auxlabel.AuxiliaryLabelEmbeddingModeStorage
)

const (
	UpgradeDirectionCreate = board.UpgradeDirectionCreate
	UpgradeDirectionExpand = board.UpgradeDirectionExpand
	UpgradeSourceAux       = board.UpgradeSourceAux
	UpgradeSourceComposite = board.UpgradeSourceComposite
)
