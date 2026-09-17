package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"syntopica-backend/internal/admin"
	appbootstrap "syntopica-backend/internal/app"
	"syntopica-backend/internal/dataenrichment"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/httpclient"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/middleware"
	"syntopica-backend/internal/platform/tracing"
	"syntopica-backend/internal/reader"
	taggingdomain "syntopica-backend/internal/tagmanagement"
	"syntopica-backend/internal/topicgraph"
)

func main() {
	if err := config.LoadConfig("./configs"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
	}

	if config.AppConfig != nil {
		logging.Init(
			config.AppConfig.Log.Level,
			logging.FileConfig{
				Enabled:    config.AppConfig.Log.File.Enabled,
				Path:       config.AppConfig.Log.File.Path,
				MaxSizeMB:  config.AppConfig.Log.File.MaxSizeMB,
				MaxBackups: config.AppConfig.Log.File.MaxBackups,
				MaxAgeDays: config.AppConfig.Log.File.MaxAgeDays,
				Compress:   config.AppConfig.Log.File.Compress,
			},
		)
		defer logging.Close()
	}

	if err := database.InitDB(config.AppConfig); err != nil {
		logging.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize feature repositories
	admin.InitRepository(database.DB)
	reader.InitRepository(database.DB)
	taggingdomain.InitRepository(database.DB)
	topicgraph.InitRepository(database.DB)

	// Wire data-enrichment domain (repo + cycle-A/B services + HTTP handler)
	// BEFORE SetupRoutes: handler.RegisterRoutes dereferences the handler
	// singleton and panics if Init hasn't run. StartRuntime is too late.
	dataenrichment.Init(database.DB)

	// Ensure semantic_labels.embedding vector dimension matches the embedder model.
	// Runs once at startup on the global DB (not inside any transaction) to avoid DDL lock contention.
	taggingdomain.EnsureVectorDimensionOnce(context.Background())

	traceCfg := tracing.DefaultConfig()
	if config.AppConfig != nil {
		traceCfg.SampleRatio = config.AppConfig.Tracing.SampleRatio
		traceCfg.InstrumentGORM = config.AppConfig.Tracing.InstrumentGORM
		traceCfg.InstrumentHTTP = config.AppConfig.Tracing.InstrumentHTTP
	}
	httpclient.SetInstrumentation(traceCfg.InstrumentHTTP)
	// 启动时把已保存的全局出站代理注入 httpclient，使 feed 抓取 / Firecrawl / LLM 等外部请求走代理。
	if cfg, _, err := aisettings.LoadProxyConfig(); err == nil {
		if u, ok := cfg["http_proxy_url"].(string); ok {
			if perr := httpclient.SetProxy(u); perr != nil {
				logging.Warnf("Invalid saved http_proxy_url %q ignored: %v", u, perr)
			} else if trimmed := strings.TrimSpace(u); trimmed != "" {
				logging.Infof("Outbound proxy enabled: %s", trimmed)
			}
		}
	}
	tp, err := tracing.InitTracerProvider(database.DB, traceCfg)
	if err != nil {
		logging.Warnf("Failed to initialize tracing: %v", err)
	} else {
		defer func() {
			// 关停时刷新遥测尾部：带 5s 超时（替换原 context.Background() 的可能无限等待）。
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := tp.Shutdown(ctx); err != nil {
				logging.Warnf("Failed to shutdown tracer: %v", err)
			}
		}()
	}

	if config.AppConfig != nil && config.AppConfig.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.Default()
	r.Use(otelgin.Middleware(tracing.ServiceName))
	if config.AppConfig != nil {
		r.Use(middleware.CORS(config.AppConfig))
	}
	r.Use(gin.Recovery())

	appbootstrap.SetupStaticFiles(r)
	appbootstrap.SetupRoutes(r)
	// In public read-only demo mode (DEMO_READ_ONLY=1) we skip all background
	// schedulers (RSS refresh, LLM daily reports, firecrawl crawling, etc.) so
	// they cannot mutate the sanitized snapshot or burn non-existent AI credits.
	// 关停信号处理总是注册：demo 模式（runtime 为 nil）也要优雅摘端口，只是跳过 Registry 步骤。
	var runtime *appbootstrap.Runtime
	if os.Getenv("DEMO_READ_ONLY") != "1" {
		runtime = appbootstrap.StartRuntime()
	}

	addr := fmt.Sprintf(":%s", config.AppConfig.Server.Port)
	logging.Infof("Server starting on %s", addr)
	logging.Infof("Environment: %s", config.AppConfig.Server.Mode)

	//nolint:gosec // G112：与 gin r.Run 的历史启动语义一致（本 change 只改关停协议，不动协议层超时）
	srv := &http.Server{Addr: addr, Handler: r}
	done := appbootstrap.SetupGracefulShutdown(runtime, srv)
	if err := appbootstrap.RunServer(srv, done); err != nil {
		// 端口占用等真实启动失败：保持 Fatalf（错误日志 + 非 0 退出码）行为不变。
		logging.Fatalf("Failed to start server: %v", err)
	}
}
