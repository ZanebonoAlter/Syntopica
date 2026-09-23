package dataenrichment

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"syntopica-backend/internal/dataenrichment/handler"
	"syntopica-backend/internal/dataenrichment/repository"
	"syntopica-backend/internal/dataenrichment/service"
	wiring "syntopica-backend/internal/datasources/wiring"
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
	// Research data source tools (change integrate-research-data-sources):
	// registered into the shared registry for future research-conversation
	// flows. NOT part of any existing flow's allowedTools — enrichment/QA tool
	// surfaces are unchanged (spec「注入后现有工具面不变」).
	toolRegistry.Register(wiring.BuildTools(wiring.ComtradeKeyResolver())...)
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

	// Signal discovery (board-signal-reports 2a): period material → detect →
	// atomic batch save. Holds NO tool registry — discovery makes zero
	// data-source calls, zero calculations, zero compose steps.
	signalDiscovery := service.NewSignalDiscoveryService(
		airouter.NewRouter(),
		CapabilityAnalysis,
		service.NewSignalMaterialBuilder(db, lifelineReader),
		repo,
	)
	// 发现侧能力前置（board-signal-reports tasks 3.7 发现半边）：四源目录能力
	// 文本由 wiring 从 Catalog() 渲染，经 setter 注入 detector——service 包不
	// 得 import datasources/wiring（会成环），仿 SetFreshnessRefresher 先例由
	// 本处外部注入；空文本=旧 prompt 字节不变。注入知识≠授权取数：发现阶段
	// 依然零取数。
	signalDiscovery.SetSourceCapabilityText(wiring.SourceCapabilityText())

	// Signal deep research (board-signal-reports 2b): frozen candidate →
	// question-driven 40-round research loop（四源 cutoff 过滤白名单，仅此链
	// 授权）→ bounded compose → immutable signal_report. Candidates/批次从
	// repository 读，报告经 repository 校验链写入。
	signalResearch := service.NewSignalResearchService(
		airouter.NewRouter(),
		CapabilityAnalysis,
		toolRegistry,
		repo,
	)
	// 研究进展持久化（board-signal-reports tasks 4.7「断了不能白跑」）：每轮
	// 滚动 upsert 进展行，超时/失败保留 abandoned、成功标 superseded。
	signalResearch.SetSignalProgressStore(repo)
	// 源可用性现算（review M2 / design §10.4）：四源「当前不可用」标注不再在
	// 启动期烘焙（BuildTools 只装静态目录元数据），由 live resolver 在研究
	// 克隆期现算——UI 保存 key 后研究 prompt 即时生效、不用重启；probe 判定与
	// datasources.StatusFor 同源（RequiresKey+空 key）。
	signalResearch.SetToolAvailabilityProbe(wiring.UnavailableNoticeProbe(wiring.ComtradeKeyResolver()))

	// HTTP handler singleton consumed by handler.RegisterRoutes.
	handler.InitHandler(repo, lifelineSvc, orchestrator, boardConfigReader, debateSvc, qaAgent, db)
	handler.SetSignalDiscoveryOnInstance(signalDiscovery)
	handler.SetSignalResearchOnInstance(signalResearch)
}

// GetLifelineService returns the cycle-A service built by Init, for scheduler
// registration in app/runtime.go.
func GetLifelineService() *service.LifelineContextService {
	return lifelineSvc
}

// SweepOrphanedSignalResearchProgress converges progress rows left status=
// running by a dead process（board-signal-reports tasks 4.10 孤儿进展收敛；
// design §10.4）。启动接线在 app.StartRuntime——resetStaleStates 同类时点：
// 调度器/worker 未起、HTTP 未监听，内存必然无活研究 job，残留 running 行都是
// 孤儿。调用方契约：失败仅记日志不阻塞启动；幂等（无 running 行时 0 行）。
func SweepOrphanedSignalResearchProgress(ctx context.Context) (int64, error) {
	repo := GetRepo()
	if repo == nil {
		return 0, fmt.Errorf("dataenrichment repository not initialized")
	}
	return repo.SweepOrphanedSignalResearchProgress(ctx)
}

// GetTopicLister returns the active-topic lister built by Init, for scheduler
// registration in app/runtime.go.
func GetTopicLister() ActiveTopicLister {
	return topicLister
}
