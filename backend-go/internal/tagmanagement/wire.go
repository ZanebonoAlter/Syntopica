package tagging

import (
	"context"
	"fmt"
	"sync"

	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/tagmanagement/handler"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
	"syntopica-backend/internal/tagmanagement/repository"
	"syntopica-backend/internal/tagmanagement/service"
	"syntopica-backend/internal/tagmanagement/service/board"
	"syntopica-backend/internal/tagmanagement/service/boardhealth"
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
		&tagmodels.AuxiliaryDescriptionRepairItem{},
		&tagmodels.AuxiliaryDescriptionRepairJob{},
		&tagmodels.BoardComposition{},
		&tagmodels.BoardHealthSnapshot{},
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
	// RegisterAuxiliaryDescriptionRepairRoutes registers the four
	// description-repair resources (board-health-triage-advice tasks
	// 2.2/2.5/2.6). The service is injected so the routes and the background
	// worker share ONE instance.
	RegisterAuxiliaryDescriptionRepairRoutes = handler.RegisterAuxiliaryDescriptionRepairRoutes
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

// Embedding queue status snapshot (admin → tagmanagement/handler deep path).
// 日报生成的双队列清空判据需要第二个队列的计数，与标签队列同口径。
type EmbeddingQueueStatusCounts = handler.EmbeddingQueueStatusCounts

var EmbeddingQueueStatusSnapshot = handler.EmbeddingQueueStatusSnapshot

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

// Board health checkup (add-board-health-checkup): the scheduler's daily-report
// tail step consumes these root re-exports only (admin-boundary: deep imports
// of tagmanagement subpackages are forbidden from admin).
type BoardHealthLLM = boardhealth.BoardHealthLLM

var (
	RunBoardHealthCheckup    = boardhealth.RunCheckup
	NewDefaultBoardHealthLLM = boardhealth.NewDefaultBoardHealthLLM
)

// RunBoardHealthCheckupWrapped runs a full checkup round with the production
// airouter-backed LLM — the exact entry the daily-report tail step calls.
func RunBoardHealthCheckupWrapped(ctx context.Context, db *gorm.DB) {
	boardhealth.RunCheckup(ctx, db, boardhealth.NewDefaultBoardHealthLLM())
}

// ============================================================================
// 辅助标签描述修复链生命周期（board-health-triage-advice tasks 2.6）
//
// 修复链只有一个服务实例与一个后台执行器：HTTP 路由与后台 worker 必须用
// **同一个** service（否则 worker 建的 task 与用户看到的 task 会分属两个
// 门面）。初始化是显式的（没有 sync.Once 懒全局）：失败必须把错误交回调用方
// （app 侧 Fatalf），不能吞成 nil 服务或让路由凭空消失。
// ============================================================================

var (
	descriptionRepairMu       sync.Mutex
	descriptionRepairService  *service.AuxRepairService
	descriptionRepairExecutor *service.AuxRepairExecutor
)

// InitDescriptionRepairChain creates the repair service AND the background
// executor over the given DB and keeps them as the domain's singletons.
//
// It must run AFTER the database is ready (AutoMigrate + migrations) and
// BEFORE the routes are registered. The failure is returned — never cached
// as a nil singleton.
func InitDescriptionRepairChain(db *gorm.DB) error {
	svc, err := service.NewAuxRepairService(db)
	if err != nil {
		return fmt.Errorf("init description-repair service: %w", err)
	}
	exec, err := service.NewAuxRepairExecutor(svc, service.DefaultAuxRepairExecutorConfig())
	if err != nil {
		return fmt.Errorf("init description-repair executor: %w", err)
	}
	InstallDescriptionRepairChain(svc, exec)
	return nil
}

// InstallDescriptionRepairChain installs an ALREADY BUILT repair service and
// executor as the domain's singletons. It is the dependency seam of the
// lifecycle: the production path builds them with the production
// collaborators (InitDescriptionRepairChain), the tests build them with stubs
// (a stub embedder — the tests must never call a real AI provider) and
// install them through the SAME wiring path.
//
// The installed service is the one the routes use — the HTTP surface and the
// background worker always share one instance.
//
// ORDERING CONTRACT (the caller must respect it):
//   - AFTER the database is ready (AutoMigrate + migrations) — the chain
//     writes and reads repair tables;
//   - BEFORE the routes are registered — the routes capture the service
//     instance at registration time;
//   - BEFORE the worker is started — the worker uses the same instance.
//
// Replacing an ALREADY RUNNING chain is NOT supported: the old router would
// keep the old service while the new worker used the new one (two doors to
// the same task). To swap the chain, stop the old worker first (and rebuild
// the host that registered the routes). This function deliberately does not
// implement hot swapping.
func InstallDescriptionRepairChain(svc *service.AuxRepairService, exec *service.AuxRepairExecutor) {
	descriptionRepairMu.Lock()
	defer descriptionRepairMu.Unlock()
	descriptionRepairService = svc
	descriptionRepairExecutor = exec
}

// DescriptionRepairService returns the shared repair service (nil when the
// chain was not initialized).
func DescriptionRepairService() *service.AuxRepairService {
	descriptionRepairMu.Lock()
	defer descriptionRepairMu.Unlock()
	return descriptionRepairService
}

// StartDescriptionRepairExecutor starts the repair worker on the given
// context. The context MUST be the APPLICATION lifetime — never the context
// of the HTTP request that created the task (closing the page must not cancel
// a confirmed task).
func StartDescriptionRepairExecutor(ctx context.Context) error {
	descriptionRepairMu.Lock()
	exec := descriptionRepairExecutor
	descriptionRepairMu.Unlock()
	if exec == nil {
		return fmt.Errorf("description-repair chain is not initialized")
	}
	return exec.Start(ctx)
}

// DescriptionRepairExecutorRunning reports whether the repair worker is
// running (observability + tests).
func DescriptionRepairExecutorRunning() bool {
	descriptionRepairMu.Lock()
	exec := descriptionRepairExecutor
	descriptionRepairMu.Unlock()
	if exec == nil {
		return false
	}
	return exec.Running()
}

// StopDescriptionRepairExecutor cancels the worker and joins it (idempotent).
// It is called during the graceful shutdown, BEFORE the database is released.
func StopDescriptionRepairExecutor() {
	descriptionRepairMu.Lock()
	exec := descriptionRepairExecutor
	descriptionRepairMu.Unlock()
	if exec == nil {
		return
	}
	exec.Stop()
}
