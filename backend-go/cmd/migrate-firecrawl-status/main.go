package main

import (
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
)

func main() {
	if err := config.LoadConfig("./configs"); err != nil {
		logging.Warnf("Warning: Failed to load config: %v", err)
	}

	if err := database.InitDB(config.AppConfig); err != nil {
		logging.Fatalf("Failed to initialize database: %v", err)
	}

	// Update articles where feed has FirecrawlEnabled=false but article has FirecrawlStatus="pending"
	result := database.DB.Exec(`
		UPDATE articles 
		SET firecrawl_status = 'complete' 
		WHERE firecrawl_status = 'pending' 
		AND feed_id IN (SELECT id FROM feeds WHERE firecrawl_enabled = false)
	`)

	if result.Error != nil {
		logging.Fatalf("Migration failed: %v", result.Error)
	}

	logging.Infof("Updated %d articles: set firecrawl_status to 'complete' for feeds with FirecrawlEnabled=false", result.RowsAffected)

	// Show stats
	var stats struct {
		Pending    int64
		Complete   int64
		Processing int64
	}
	database.DB.Model(&models.Article{}).Where("firecrawl_status = ?", "pending").Count(&stats.Pending)
	database.DB.Model(&models.Article{}).Where("firecrawl_status = ?", "complete").Count(&stats.Complete)
	database.DB.Model(&models.Article{}).Where("firecrawl_status = ?", "processing").Count(&stats.Processing)

	logging.Infof("Current stats: pending=%d, complete=%d, processing=%d", stats.Pending, stats.Complete, stats.Processing)
}
