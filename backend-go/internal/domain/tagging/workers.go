package tagging

import "syntopica-backend/internal/platform/logging"

func StartAllWorkers() {
	if err := GetTagQueue().Start(); err != nil {
		logging.Warnf("Failed to start tag queue: %v", err)
	} else {
		logging.Infoln("Tag queue started successfully")
	}

	StartEmbeddingQueueWorker()
	logging.Infoln("Embedding queue worker started successfully")
	StartMergeReembeddingQueueWorker()
	logging.Infoln("Merge re-embedding queue worker started successfully")
}

func StopAllWorkers() {
	logging.Infoln("Stopping tag queue...")
	GetTagQueue().Stop()

	logging.Infoln("Stopping embedding queue worker...")
	StopEmbeddingQueueWorker()

	logging.Infoln("Stopping merge re-embedding queue worker...")
	StopMergeReembeddingQueueWorker()
}
