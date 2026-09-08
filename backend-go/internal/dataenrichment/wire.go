package dataenrichment

import (
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/config"
)

// InitRepository initializes the dataenrichment repository singleton.
func InitRepository(db *gorm.DB) {
	repository.InitRepo(db)
}

// GetRepo returns the dataenrichment repository singleton.
func GetRepo() *repository.Repository {
	return repository.Repo
}

// Package-level service singletons built by Init. The runtime schedulers
// (registered in app/runtime.go) consume them via the getters below.
var (
	lifelineSvc *service.LifelineContextService
	topicLister ActiveTopicLister
)

// Init wires the full data-enrichment domain: repository singleton, cycle-A
// lifeline context service, cycle-B orchestrator, and the HTTP handler
// singleton.
//
// MUST be called before dataenrichment.RegisterRoutes, which means it runs in
// main.go BEFORE app.SetupRoutes — mirroring how the other domains call
// InitRepository in main.go prior to route registration. (app.StartRuntime only
// registers schedulers and runs AFTER SetupRoutes, so it is too late for the
// handler singleton that RegisterRoutes dereferences.)
func Init(db *gorm.DB) {
	InitRepository(db)
	repo := GetRepo()

	// Cycle A: lifeline context summary service (news-only, scheduled).
	lifelineSvc = service.NewLifelineContextService(
		airouter.NewRouter(),
		repo,
		NewTopicGraphSectionReader(db),
		CapabilityNews,
	)
	topicLister = NewDBTopicLister(db)

	// Cycle B: three-role orchestration (interpret → agent loop → analyze + review).
	boardConfigReader := NewDBBoardConfigReader(db)
	lifelineReader := NewDBLifelineReader(db)
	renderer := service.NewLifelineRenderer()
	boardLister := NewDBBoardLister(db)
	laneLister := NewDBLaneLister(db)
	laneDetailRenderer := service.NewRendererLaneDetailAdapter(lifelineReader, renderer)

	// web_search backend: Bocha client reads credentials dynamically on each
	// Search via a provider (DB(ui) > env > config.yaml > empty). Reading on
	// demand lets UI changes take effect without restart (mirrors Firecrawl
	// reading DB per job). When all sources are empty the provider returns "",
	// BochaWebSearcher.Search returns a "not configured" error, and
	// executeWebSearch degrades to an error JSON (same semantics as Noop).
	// NoopWebSearcher is retained only as a test stub.
	bochaProvider := service.BochaConfigProvider(func() (string, string) {
		// 1. DB (UI) first. enabled 缺省视为启用（仅显式 false 才跳过 DB）。
		if cfg, _, err := aisettings.LoadBochaConfig(); err == nil && cfg != nil {
			if v, ok := cfg["enabled"].(bool); !ok || v {
				if k, _ := cfg["api_key"].(string); strings.TrimSpace(k) != "" {
					ep, _ := cfg["endpoint"].(string)
					return k, ep
				}
			}
		}
		// 2. env / config.yaml 兑底。
		if c := config.AppConfig; c != nil {
			return c.Bocha.APIKey, c.Bocha.Endpoint
		}
		return "", ""
	})

	toolRegistry := service.NewRegistry(
		service.NewDefaultHTTPFetcher(),
		service.WithWebSearcher(service.NewBochaWebSearcher(bochaProvider)),
		service.WithPageFetcher(service.NewReaderPageFetcher()),
		service.WithBoardLister(boardLister),
		service.WithLaneLister(laneLister),
		service.WithLaneDetailRenderer(laneDetailRenderer),
		service.WithInternalContextSearcher(NewDBInternalContextSearcher(db)),
	)
	orchestrator := service.NewOrchestratorService(
		airouter.NewRouter(),
		repo,
		lifelineReader,
		renderer,
		toolRegistry,
		boardConfigReader,
		CapabilityAnalysis,
	)
	// D9 freshness gate: board/topic enrichment borrows cycle-A's refresh
	// methods (lifelineSvc) so pre-analysis catch-up works out of the box.
	orchestrator.SetFreshnessRefresher(lifelineSvc)
	// Board config gate (M5.1): EnrichBoard resolves enrichment_enabled by
	// board ID via a post-construction setter. Without this the first gate
	// 500s with "board config resolver not wired" in production while unit
	// tests pass (they wire the resolver themselves).
	orchestrator.SetBoardConfigResolver(service.NewDBBoardConfigResolver(db))

	// Cycle B (optional): FinGenius stock debate (submit → poll → distill → persist).
	fingeniusClient := service.NewFinGeniusHTTPClient()
	debateDistiller := service.NewDebateDistiller(airouter.NewRouter(), CapabilityAnalysis)
	debateSvc := service.NewDebateService(fingeniusClient, debateDistiller, repo)

	// Cycle B (阶段2b): report follow-up QA agent. Reuses the SAME tool registry as
	// the orchestrator so the exploration tools (list_boards/list_lanes/
	// get_lane_detail/web_search) are available; never writes to the result table.
	qaAgent := service.NewQAAgent(airouter.NewRouter(), toolRegistry, repo, CapabilityAnalysis)

	// HTTP handler singleton consumed by handler.RegisterRoutes.
	handler.InitHandler(repo, lifelineSvc, orchestrator, boardConfigReader, debateSvc, qaAgent, db)
}

// GetLifelineService returns the cycle-A service built by Init, for scheduler
// registration in app/runtime.go.
func GetLifelineService() *service.LifelineContextService {
	return lifelineSvc
}

// GetTopicLister returns the active-topic lister built by Init, for scheduler
// registration in app/runtime.go.
func GetTopicLister() ActiveTopicLister {
	return topicLister
}
