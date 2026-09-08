package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
)

// LifelineService abstracts the methods of service.LifelineContextService used by handlers.
// This allows tests to mock the LLM-dependent regenerate operation.
type LifelineService interface {
	RefreshGranularity(ctx context.Context, topicID uint, granularity string, now time.Time) error
	RefreshPeriod(ctx context.Context, topicID uint, granularity, period string, now time.Time) error
}

// Orchestrator abstracts the methods of service.OrchestratorService used by handlers.
// This allows tests to mock the LLM-dependent trigger operation.
type Orchestrator interface {
	EnrichTopic(ctx context.Context, topicID uint) (*service.EnrichmentOutput, error)
	EnrichTopicLens(ctx context.Context, topicID uint, prefillLens string) (*service.EnrichmentOutput, error)
	EnrichBoard(ctx context.Context, boardID uint) (*service.BoardEnrichmentOutput, error)
	BoardEnrichmentEnabled(ctx context.Context, boardID uint) error
	InvestigateBoardQuestion(ctx context.Context, boardID uint, parentBriefID uint, question service.BoardInvestigationQuestion) (*service.BoardInvestigationOutput, error)
	// Cross-board relation discovery (add-evidence-backed-cross-board-relations).
	RunRelationDiscovery(ctx context.Context, in service.RelationDiscoveryInput) (*service.RelationDiscoveryOutput, error)
	ReResolveRelation(ctx context.Context, boardID, relationID uint) (*service.RelationReResolveOutput, error)
}

// DebateRunner abstracts DebateService.RunDebate for handler testing.
type DebateRunner interface {
	RunDebate(ctx context.Context, resultID, topicID uint, sessionID string, symbols []service.DebateSymbol) ([]*repository.StockDebateResult, error)
}

// QARunner abstracts QAAgent.Ask for handler testing.
type QARunner interface {
	Ask(ctx context.Context, resultID uint, question string) (*service.QAAnswer, error)
}

// EnrichmentHandler serves the data enrichment CRUD API.
// Dependencies are injected via InitHandler (called from runtime.go).
type EnrichmentHandler struct {
	repo              *repository.Repository
	lifelineSvc       LifelineService
	orchestrator      Orchestrator
	boardConfigReader service.BoardConfigReader
	debateSvc         DebateRunner
	qaRunner          QARunner
	db                *gorm.DB
	analysis          *analysisRunner
}

var instance *EnrichmentHandler

// InitHandler stores the singleton handler. Call once at startup.
func InitHandler(
	repo *repository.Repository,
	lifelineSvc LifelineService,
	orchestrator Orchestrator,
	boardConfigReader service.BoardConfigReader,
	debateSvc DebateRunner,
	qaRunner QARunner,
	db *gorm.DB,
) {
	instance = &EnrichmentHandler{
		repo:              repo,
		lifelineSvc:       lifelineSvc,
		orchestrator:      orchestrator,
		boardConfigReader: boardConfigReader,
		debateSvc:         debateSvc,
		qaRunner:          qaRunner,
		db:                db,
		analysis:          newAnalysisRunner(),
	}
}

// NewHandler creates a standalone EnrichmentHandler for testing without touching the singleton.
func NewHandler(
	repo *repository.Repository,
	lifelineSvc LifelineService,
	orchestrator Orchestrator,
	boardConfigReader service.BoardConfigReader,
	debateSvc DebateRunner,
	qaRunner QARunner,
	db *gorm.DB,
) *EnrichmentHandler {
	return &EnrichmentHandler{
		repo:              repo,
		lifelineSvc:       lifelineSvc,
		orchestrator:      orchestrator,
		boardConfigReader: boardConfigReader,
		debateSvc:         debateSvc,
		qaRunner:          qaRunner,
		db:                db,
		analysis:          newAnalysisRunner(),
	}
}

// RegisterRoutes registers all data enrichment HTTP routes under rg using the
// package-level singleton handler.
func RegisterRoutes(rg *gin.RouterGroup) {
	if instance == nil {
		panic("handler.InitHandler must be called before RegisterRoutes")
	}
	instance.RegisterRoutes(rg)
}

// RegisterRoutes registers all routes on the given router group, accepting an
// explicit handler so tests can wire their own dependencies.
func (h *EnrichmentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	// Async analysis status poller (board + topic scopes, no topic prefix).
	rg.GET("/enrichment/analysis-status", h.getAnalysisStatus)

	// ── Topic enrichment (topic dimension) ──────────────────────────────────
	enrichment := rg.Group("/persistent-topics/:topicId/enrichment")

	// Table 1: topic_lifeline_context (period-archival model)
	contexts := enrichment.Group("/contexts")
	{
		contexts.GET("", h.listContexts)                               // ?granularity=week (opt)
		contexts.GET("/:granularity/:period", h.getContext)            // specific period
		contexts.PUT("/:granularity/:period", h.updateContext)         // edit specific period
		contexts.POST("/:granularity/regenerate", h.regenerateContext) // ?period=2026-W27
	}

	// Table 2: topic_enrichment_result
	results := enrichment.Group("/results")
	{
		results.GET("", h.listResults)
		results.GET("/:id", h.getResult)
		results.POST("/trigger", h.triggerEnrichment)

		// Stock debate (FinGenius)
		results.POST("/:id/debates", h.triggerDebate)
		results.GET("/:id/debates", h.listDebates)

		// Report follow-up Q&A (causal-analysis-agent 阶段2b)
		results.POST("/:id/qa", h.askQA)
		results.GET("/:id/qa", h.listQA)
	}

	// Table 4: topic_enrichment_qa sediment (manual pin; report stays immutable)
	qa := enrichment.Group("/qa")
	{
		qa.POST("/:id/sediment", h.sedimentQA)
	}

	// Table 3: topic_enrichment_review
	reviews := enrichment.Group("/reviews")
	{
		reviews.GET("", h.listReviews)
		reviews.POST("", h.createReview)
		reviews.PUT("/:id", h.updateReviewDeviation)
		reviews.POST("/:id/apply", h.applyReview)
	}

	// ── Board-level analysis (board dimension; board-level-deep-analysis D9) ─
	boardAnalysis := rg.Group("/semantic-boards/:id/enrichment/analysis")
	{
		boardAnalysis.POST("/trigger", h.triggerBoardEnrichment)
		boardAnalysis.POST("/investigations/trigger", h.triggerBoardInvestigation)
		boardAnalysis.GET("/results", h.listBoardResults)
		boardAnalysis.GET("/results/:rid", h.getBoardResult)

		// Board report follow-up Q&A (design D5: QA keeps working by result id;
		// all three board kinds incl. legacy — result stays immutable, QA rows
		// are independent append-only rows).
		boardAnalysis.POST("/relations/discover", h.triggerRelationDiscovery)
		boardAnalysis.GET("/relations", h.listBoardRelations)
		boardAnalysis.GET("/relations/:rid", h.getBoardRelation)
		boardAnalysis.POST("/relations/:rid/confirm", h.confirmBoardRelation)
		boardAnalysis.POST("/relations/:rid/dismiss", h.dismissBoardRelation)
		boardAnalysis.POST("/relations/:rid/re-resolve", h.reResolveBoardRelation)
		boardAnalysis.POST("/results/:rid/qa", h.askBoardQA)
		boardAnalysis.GET("/results/:rid/qa", h.listBoardQA)
		boardAnalysis.POST("/results/:rid/qa/:qid/sediment", h.sedimentBoardQA)
	}

	// ── Board data source bindings (board dimension) ────────────────────────
	dataSources := rg.Group("/semantic-boards/:id/data-sources")
	{
		dataSources.GET("", h.listDataSources)
		dataSources.PUT("", h.upsertDataSource)
		dataSources.DELETE("/:sourceType", h.deleteDataSource)
	}

	// ── Analysis methods (global method-card library; design D6) ───────────
	methods := rg.Group("/analysis-methods")
	{
		methods.GET("", h.listAnalysisMethods)
		methods.POST("", h.createAnalysisMethod)
		methods.GET("/:id", h.getAnalysisMethod)
		methods.PUT("/:id", h.updateAnalysisMethod)
		methods.PUT("/:id/enable", h.setAnalysisMethodEnabled)
		methods.DELETE("/:id", h.deleteAnalysisMethod)
	}

	// ── Legacy reference roles: one-version read-only compatibility ────────
	roles := rg.Group("/reference-roles")
	{
		roles.GET("", h.listReferenceRoles)
		roles.POST("", h.createReferenceRole)
		roles.GET("/:id", h.getReferenceRole)
		roles.PUT("/:id", h.updateReferenceRole)
		roles.DELETE("/:id", h.deleteReferenceRole)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// respondAccepted is the async-trigger envelope (D9): 202, not 200 — the job
// has been started, not completed.
func respondAccepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": data})
}

// respondErrorWithData keeps the {success,error} shape but attaches a data
// payload — 409s carry the currently running job's identity for recovery.
func respondErrorWithData(c *gin.Context, status int, msg string, data any) {
	c.JSON(status, gin.H{"success": false, "error": msg, "data": data})
}

func respondError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"success": false, "error": msg})
}

func parseTopicID(c *gin.Context) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param("topicId"), 10, 64)
	if err != nil || parsed == 0 {
		respondError(c, http.StatusBadRequest, "invalid topicId")
		return 0, false
	}
	return uint(parsed), true
}

func parseIDParam(c *gin.Context, name string) (uint, bool) {
	parsed, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || parsed == 0 {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("invalid %s", name))
		return 0, false
	}
	return uint(parsed), true
}

// ── Table 1: topic_lifeline_context ─────────────────────────────────────────

// listContexts returns all contexts for a topic, optionally filtered by ?granularity=.
// GET /persistent-topics/:topicId/enrichment/contexts[?granularity=week]
func (h *EnrichmentHandler) listContexts(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	granFilter := c.Query("granularity")
	var (
		list []repository.TopicLifelineContext
		err  error
	)

	if granFilter != "" {
		if !isValidGranularity(granFilter) {
			respondError(c, http.StatusBadRequest, fmt.Sprintf("invalid granularity filter: %s", granFilter))
			return
		}
		list, err = h.repo.ListTopicLifelineContextsByGranularity(c.Request.Context(), topicID, granFilter)
	} else {
		list, err = h.repo.ListTopicLifelineContextsByTopic(c.Request.Context(), topicID)
	}

	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, list)
}

// getContext returns a single context by granularity + period.
// GET /persistent-topics/:topicId/enrichment/contexts/:granularity/:period
func (h *EnrichmentHandler) getContext(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}
	granularity := c.Param("granularity")
	period := c.Param("period")
	if !isValidGranularity(granularity) {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("invalid granularity: %s", granularity))
		return
	}
	if period == "" {
		respondError(c, http.StatusBadRequest, "period is required")
		return
	}

	lc, err := h.repo.GetTopicLifelineContext(c.Request.Context(), topicID, granularity, period)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}

	respondOK(c, lc)
}

// updateContext allows manual editing of a context's content.
// PUT /persistent-topics/:topicId/enrichment/contexts/:granularity
// Body: { "content": "..." }
// updateContext allows manual editing of a context's content.
// PUT /persistent-topics/:topicId/enrichment/contexts/:granularity/:period
// Body: { "content": "..." }
func (h *EnrichmentHandler) updateContext(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}
	granularity := c.Param("granularity")
	period := c.Param("period")
	if !isValidGranularity(granularity) {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("invalid granularity: %s", granularity))
		return
	}
	if period == "" {
		respondError(c, http.StatusBadRequest, "period is required")
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "content is required")
		return
	}

	// Fetch existing context to get the ID.
	existing, err := h.repo.GetTopicLifelineContext(c.Request.Context(), topicID, granularity, period)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}

	existing.Content = req.Content
	existing.Source = "manual"

	if err := h.repo.UpsertTopicLifelineContext(c.Request.Context(), existing); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, existing)
}

// regenerateContext triggers manual re-generation for a granularity (and optional period).
// POST /persistent-topics/:topicId/enrichment/contexts/:granularity/regenerate?period=2026-W27
// If period is omitted, the current period is used (via RefreshGranularity).
func (h *EnrichmentHandler) regenerateContext(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}
	granularity := c.Param("granularity")
	if !isValidGranularity(granularity) {
		respondError(c, http.StatusBadRequest, fmt.Sprintf("invalid granularity: %s", granularity))
		return
	}

	now := time.Now()
	period := c.Query("period")

	var err error
	if period != "" {
		err = h.lifelineSvc.RefreshPeriod(c.Request.Context(), topicID, granularity, period, now)
	} else {
		err = h.lifelineSvc.RefreshGranularity(c.Request.Context(), topicID, granularity, now)
	}

	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("regenerate failed: %v", err))
		return
	}

	// Fetch the newly generated context.
	if period == "" {
		period = service.PeriodForGranularity(now, granularity)
	}
	lc, err := h.repo.GetTopicLifelineContext(c.Request.Context(), topicID, granularity, period)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("context regenerated but failed to fetch: %v", err))
		return
	}

	respondOK(c, lc)
}

// ── Table 2: topic_enrichment_result ────────────────────────────────────────

// listResults returns all enrichment results for a topic, newest first.
// GET /persistent-topics/:topicId/enrichment/results
func (h *EnrichmentHandler) listResults(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	results, err := h.repo.ListTopicEnrichmentResultsByTopic(c.Request.Context(), topicID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Return slim summaries with sectors / tool_calls overview / input_snapshot / session_id.
	type resultSummary struct {
		ID                  uint   `json:"id"`
		EvolutionAssessment string `json:"evolution_assessment"`
		Sectors             any    `json:"sectors"`
		ToolCallsCount      int    `json:"tool_calls_count"`
		SessionID           string `json:"session_id"`
		CreatedAt           any    `json:"created_at"`
	}
	items := make([]resultSummary, 0, len(results))
	for _, r := range results {
		sectors := tryParseJSON(r.Sectors)
		tcCount := countJSONArray(r.ToolCalls)
		items = append(items, resultSummary{
			ID:                  r.ID,
			EvolutionAssessment: truncateStr(r.EvolutionAssessment, 200),
			Sectors:             sectors,
			ToolCallsCount:      tcCount,
			SessionID:           r.SessionID,
			CreatedAt:           r.CreatedAt,
		})
	}

	respondOK(c, items)
}

// getResult returns a single enrichment result with full detail.
// GET /persistent-topics/:topicId/enrichment/results/:id
func (h *EnrichmentHandler) getResult(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}

	// Validate the result belongs to the requested topic (IDOR protection).
	if !repository.TopicIDMatches(result.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "result not found for this topic")
		return
	}

	respondOK(c, gin.H{
		"id":                   result.ID,
		"persistent_topic_id":  result.PersistentTopicID,
		"evolution_assessment": result.EvolutionAssessment,
		"sectors":              tryParseJSON(result.Sectors),
		"causal_chain":         result.CausalChain,
		"tool_calls":           tryParseJSON(result.ToolCalls),
		"input_snapshot":       tryParseJSON(result.InputSnapshot),
		"session_id":           result.SessionID,
		"created_at":           result.CreatedAt,
	})
}

// triggerEnrichment manually triggers cycle-B enrichment for the topic.
// POST /persistent-topics/:topicId/enrichment/results/trigger
func (h *EnrichmentHandler) triggerEnrichment(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	// Check enrichment_enabled via board config.
	cfg, err := h.boardConfigReader.GetBoardConfig(c.Request.Context(), topicID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !cfg.EnrichmentEnabled {
		respondError(c, http.StatusBadRequest, "enrichment is not enabled for this topic's board")
		return
	}

	// D8: optional {prefill_lens} body — drill-down entry from board reports.
	var body struct {
		PrefillLens string `json:"prefill_lens"`
	}
	_ = c.ShouldBindJSON(&body) // empty/absent body is fine

	// Trigger is fire-and-forget (fix-board-analysis-material 8.x): the run
	// survives client disconnects; poll /enrichment/analysis-status for result.
	st, err := h.analysis.Start(AnalysisScopeTopic, topicID, AnalysisJobKindTopic, analysisJobTimeout, func(ctx context.Context) (uint, error) {
		var output *service.EnrichmentOutput
		var err error
		if body.PrefillLens != "" {
			output, err = h.orchestrator.EnrichTopicLens(ctx, topicID, body.PrefillLens)
		} else {
			output, err = h.orchestrator.EnrichTopic(ctx, topicID)
		}
		if err != nil {
			return 0, err
		}
		return output.Result.ID, nil
	})
	if err != nil {
		var runErr *RunningJobError
		if errors.As(err, &runErr) {
			respondErrorWithData(c, http.StatusConflict, "topic analysis already running", runErr.Current)
			return
		}
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("enrichment failed: %v", err))
		return
	}
	respondAccepted(c, gin.H{
		"status":    "started",
		"job_id":    st.JobID,
		"job_kind":  st.JobKind,
		"scope":     AnalysisScopeTopic,
		"target_id": topicID,
	})
}

// ── Stock Debate (FinGenius) ───────────────────────────────────────────────

// triggerDebate starts a FinGenius debate for the given symbols.
// POST /persistent-topics/:topicId/enrichment/results/:id/debates
// Body: { "symbols": [{"code":"161129","name":"易方达原油","sector":"原油"}] }
// If symbols are omitted or empty, defaults to extracting from the result's sectors.symbols.
func (h *EnrichmentHandler) triggerDebate(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	resultID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// Validate the result belongs to this topic.
	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), resultID)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !repository.TopicIDMatches(result.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "result not found for this topic")
		return
	}

	var req struct {
		Symbols []service.DebateSymbol `json:"symbols"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Body may be empty or invalid; try default symbols extraction.
		req.Symbols = nil
	}

	// Default: extract symbols from result's sectors.
	symbols := req.Symbols
	if len(symbols) == 0 {
		symbols = extractSymbolsFromSectors(result.Sectors)
	}
	if len(symbols) == 0 {
		respondError(c, http.StatusBadRequest, "no symbols provided and none found in result sectors")
		return
	}

	// Generate a session ID for this debate run.
	sessionID := fmt.Sprintf("data_enrichment_debate_%d_%d", topicID, resultID)

	debateResults, err := h.debateSvc.RunDebate(c.Request.Context(), resultID, topicID, sessionID, symbols)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("debate failed: %v", err))
		return
	}

	respondOK(c, gin.H{"debates": debateResults})
}

// listDebates returns all debate results for a given enrichment result.
// GET /persistent-topics/:topicId/enrichment/results/:id/debates
func (h *EnrichmentHandler) listDebates(c *gin.Context) {
	resultID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	debates, err := h.repo.ListStockDebateResultsByResult(c.Request.Context(), resultID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, debates)
}

// ── Report follow-up Q&A (causal-analysis-agent 阶段2b) ────────────────────

// askQA runs one follow-up round against an immutable report and returns the
// answer. The report itself is never modified; each round appends a
// topic_enrichment_qa row (source="qa").
// POST /persistent-topics/:topicId/enrichment/results/:id/qa
// Body: { "question": "..." }
func (h *EnrichmentHandler) askQA(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	resultID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// IDOR protection: validate the result belongs to this topic.
	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), resultID)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !repository.TopicIDMatches(result.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "result not found for this topic")
		return
	}

	var req struct {
		Question string `json:"question" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "question is required")
		return
	}

	answer, err := h.qaRunner.Ask(c.Request.Context(), resultID, req.Question)
	if err != nil {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("qa failed: %v", err))
		return
	}

	respondOK(c, answer)
}

// listQA returns the multi-round follow-up history for a report, oldest first.
// GET /persistent-topics/:topicId/enrichment/results/:id/qa
// IDOR protection mirrors askQA: the result must belong to this topic
// (board-scope rows never match a topic id — use the board QA routes).
func (h *EnrichmentHandler) listQA(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	resultID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// IDOR protection: validate the result belongs to this topic.
	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), resultID)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !repository.TopicIDMatches(result.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "result not found for this topic")
		return
	}

	list, err := h.repo.ListTopicEnrichmentQAByResultID(c.Request.Context(), resultID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, list)
}

// sedimentQA pins a follow-up round as a durable note (sedimented=true).
// Only flips the flag on the qa row; the report (topic_enrichment_result) is
// never rewritten (业务约束#2: result 不可变).
// POST /persistent-topics/:topicId/enrichment/qa/:id/sediment
func (h *EnrichmentHandler) sedimentQA(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	qaID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// IDOR protection: qa has no PersistentTopicID, so validate via its result.
	qa, err := h.repo.GetTopicEnrichmentQAByID(c.Request.Context(), qaID)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	result, err := h.repo.GetTopicEnrichmentResultByID(c.Request.Context(), qa.TopicEnrichmentResultID)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !repository.TopicIDMatches(result.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "qa not found for this topic")
		return
	}

	if err := h.repo.MarkQASedimented(c.Request.Context(), qaID); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	qa, err = h.repo.GetTopicEnrichmentQAByID(c.Request.Context(), qaID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, qa)
}

// extractSymbolsFromSectors parses the sectors JSONB to extract symbols from each sector.
func extractSymbolsFromSectors(sectorsJSON json.RawMessage) []service.DebateSymbol {
	if len(sectorsJSON) == 0 {
		return nil
	}
	var sectors []map[string]any
	if err := json.Unmarshal(sectorsJSON, &sectors); err != nil {
		return nil
	}

	var symbols []service.DebateSymbol
	for _, s := range sectors {
		sectorName, _ := s["sector"].(string)
		symbolsRaw, _ := s["symbols"].([]any)
		for _, sym := range symbolsRaw {
			sm, ok := sym.(map[string]any)
			if !ok {
				continue
			}
			code, _ := sm["code"].(string)
			name, _ := sm["name"].(string)
			if code != "" {
				symbols = append(symbols, service.DebateSymbol{
					Code:   code,
					Name:   name,
					Sector: sectorName,
				})
			}
		}
	}
	return symbols
}

// ── Table 3: topic_enrichment_review ────────────────────────────────────────

// listReviews returns all reviews for a topic, newest first.
// GET /persistent-topics/:topicId/enrichment/reviews
func (h *EnrichmentHandler) listReviews(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	reviews, err := h.repo.ListTopicEnrichmentReviewsByTopic(c.Request.Context(), topicID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, reviews)
}

// updateReviewDeviation updates the deviation_summary of a review.
// PUT /persistent-topics/:topicId/enrichment/reviews/:id
// Body: { "deviation_summary": "..." }
func (h *EnrichmentHandler) updateReviewDeviation(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// IDOR protection: validate the review belongs to this topic before mutating.
	review, err := h.repo.GetTopicEnrichmentReviewByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !repository.TopicIDMatches(review.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "review not found for this topic")
		return
	}

	var req struct {
		DeviationSummary string `json:"deviation_summary" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "deviation_summary is required")
		return
	}

	if err := h.repo.UpdateTopicEnrichmentReviewDeviation(c.Request.Context(), id, req.DeviationSummary); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch the updated review.
	review, err = h.repo.GetTopicEnrichmentReviewByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, review)
}

// applyReview marks a review as applied (applied=true).
// Does NOT write back to table 1 (context) per design §4.3.
// POST /persistent-topics/:topicId/enrichment/reviews/:id/apply
func (h *EnrichmentHandler) applyReview(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	// IDOR protection: validate the review belongs to this topic before mutating.
	review, err := h.repo.GetTopicEnrichmentReviewByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !repository.TopicIDMatches(review.PersistentTopicID, topicID) {
		respondError(c, http.StatusNotFound, "review not found for this topic")
		return
	}

	if err := h.repo.ApplyTopicEnrichmentReview(c.Request.Context(), id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	review, err = h.repo.GetTopicEnrichmentReviewByID(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, review)
}

// createReview manually creates a review annotation.
// POST /persistent-topics/:topicId/enrichment/reviews
// Body: { "curr_result_id": N, "deviation_summary": "...", "prev_result_id": N? }
func (h *EnrichmentHandler) createReview(c *gin.Context) {
	topicID, ok := parseTopicID(c)
	if !ok {
		return
	}

	var req struct {
		CurrResultID     uint   `json:"curr_result_id" binding:"required"`
		DeviationSummary string `json:"deviation_summary" binding:"required"`
		PrevResultID     *uint  `json:"prev_result_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "curr_result_id and deviation_summary are required")
		return
	}

	review := &repository.TopicEnrichmentReview{
		PersistentTopicID: repository.TopicIDPtr(topicID),
		PrevResultID:      req.PrevResultID,
		CurrResultID:      req.CurrResultID,
		DeviationSummary:  req.DeviationSummary,
		Applied:           true,
		Source:            "manual",
	}

	if err := h.repo.CreateTopicEnrichmentReview(c.Request.Context(), review); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, review)
}

// ── Board data sources ──────────────────────────────────────────────────────

// listDataSources returns data sources bound to a board.
// GET /semantic-boards/:id/data-sources
func (h *EnrichmentHandler) listDataSources(c *gin.Context) {
	boardID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	list, err := h.repo.ListBoardDataSourcesByBoardID(c.Request.Context(), boardID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, list)
}

// upsertDataSource creates or updates a data source binding for a board.
// PUT /semantic-boards/:id/data-sources
// Body: { "source_type": "<registered source type>", "config": {...}, "enabled": true }
// Note: built-in financial source types (etf_quote/exchange_rate/gdelt_event)
// were removed; source_type is an extensible enum (spec "板块数据源绑定").
func (h *EnrichmentHandler) upsertDataSource(c *gin.Context) {
	boardID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req struct {
		SourceType string         `json:"source_type" binding:"required"`
		Config     map[string]any `json:"config"`
		Enabled    *bool          `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "source_type is required")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	ds := &repository.BoardDataSource{
		SemanticBoardID: boardID,
		SourceType:      req.SourceType,
		Config:          req.Config,
		Enabled:         enabled,
	}

	if err := h.repo.UpsertBoardDataSource(c.Request.Context(), ds); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	respondOK(c, ds)
}

// deleteDataSource removes a data source binding by source type.
// DELETE /semantic-boards/:id/data-sources/:sourceType
func (h *EnrichmentHandler) deleteDataSource(c *gin.Context) {
	boardID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	sourceType := c.Param("sourceType")

	ds, err := h.repo.GetBoardDataSourceByBoardAndType(c.Request.Context(), boardID, sourceType)
	if err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}

	if err := h.repo.DeleteBoardDataSource(c.Request.Context(), ds.ID); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, gin.H{"deleted": true})
}

// ── Private helpers ─────────────────────────────────────────────────────────

var validGranularities = map[string]bool{
	"week": true, "month": true, "year": true, "all": true,
}

func isValidGranularity(g string) bool {
	return validGranularities[g]
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func countJSONArray(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	count := 0
	for _, b := range raw {
		if b == '{' {
			count++
		}
	}
	return count
}

func tryParseJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
