package core

import (
	"sync"

	"syntopica-backend/internal/platform/logging"
)

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

// ---- Embedding queue worker lifecycle ----

var embQueueSvc *EmbeddingQueueService
var embQueueOnce sync.Once

func StartEmbeddingQueueWorker() {
	embQueueOnce.Do(func() { embQueueSvc = NewEmbeddingQueueService(logging.NewZapLogger()) })
	embQueueSvc.Start()
}

func StopEmbeddingQueueWorker() {
	embQueueOnce.Do(func() { embQueueSvc = NewEmbeddingQueueService(nil) })
	embQueueSvc.Stop()
}

// ---- Merge re-embedding queue worker lifecycle ----

var mergeQueueSvc *MergeReembeddingQueueService
var mergeQueueOnce sync.Once

func StartMergeReembeddingQueueWorker() {
	mergeQueueOnce.Do(func() { mergeQueueSvc = NewMergeReembeddingQueueService(nil) })
	mergeQueueSvc.Start()
}

func StopMergeReembeddingQueueWorker() {
	mergeQueueOnce.Do(func() { mergeQueueSvc = NewMergeReembeddingQueueService(nil) })
	mergeQueueSvc.Stop()
}
