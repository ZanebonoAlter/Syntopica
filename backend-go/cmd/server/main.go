package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	appbootstrap "syntopica-backend/internal/app"
	taggingdomain "syntopica-backend/internal/domain/tagging"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/platform/middleware"
	"syntopica-backend/internal/platform/tracing"
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

	if err := airouter.EnsureLegacySummaryConfigMigrated(); err != nil {
		logging.Warnf("Failed to migrate legacy AI summary config: %v", err)
	}

	// Ensure semantic_labels.embedding vector dimension matches the embedder model.
	// Runs once at startup on the global DB (not inside any transaction) to avoid DDL lock contention.
	taggingdomain.EnsureVectorDimensionOnce(context.Background())

	traceCfg := tracing.DefaultConfig()
	tp, err := tracing.InitTracerProvider(database.DB, traceCfg)
	if err != nil {
		logging.Warnf("Failed to initialize tracing: %v", err)
	} else {
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
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
	runtime := appbootstrap.StartRuntime()
	appbootstrap.SetupGracefulShutdown(runtime)

	addr := fmt.Sprintf(":%s", config.AppConfig.Server.Port)
	logging.Infof("Server starting on %s", addr)
	logging.Infof("Environment: %s", config.AppConfig.Server.Mode)

	if err := r.Run(addr); err != nil {
		logging.Fatalf("Failed to start server: %v", err)
	}
}
