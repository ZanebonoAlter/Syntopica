package main

import (
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

	db := database.DB
	if db == nil {
		logging.Errorln("Database connection is nil")
	}

	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS embedding_queues (
			id BIGSERIAL PRIMARY KEY,
			tag_id BIGINT NOT NULL REFERENCES topic_tags(id) ON DELETE CASCADE,
			status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
			error_message TEXT,
			retry_count INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			started_at TIMESTAMP,
			completed_at TIMESTAMP
		)
	`).Error; err != nil {
		logging.Fatalf("Failed to create embedding_queues table: %v", err)
	}
	logging.Infoln("✅ embedding_queues table created (or already exists)")

	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_embedding_queues_status ON embedding_queues(status)
	`).Error; err != nil {
		logging.Fatalf("Failed to create idx_embedding_queues_status index: %v", err)
	}
	logging.Infoln("✅ idx_embedding_queues_status index created (or already exists)")

	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_embedding_queues_tag_id ON embedding_queues(tag_id)
	`).Error; err != nil {
		logging.Fatalf("Failed to create idx_embedding_queues_tag_id index: %v", err)
	}
	logging.Infoln("✅ idx_embedding_queues_tag_id index created (or already exists)")

	logging.Infoln("Embedding queue migration completed successfully")
}
