package handler

import (
	"sync"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/tagmanagement/repository"
)

var embeddingQueueStatusService *embeddingQueueStatusReader
var embeddingQueueStatusOnce sync.Once

type embeddingQueueStatusReader struct {
	db *gorm.DB
}

func getEmbeddingQueueStatusReader() *embeddingQueueStatusReader {
	embeddingQueueStatusOnce.Do(func() {
		embeddingQueueStatusService = &embeddingQueueStatusReader{db: repository.Repo.DB()}
	})
	return embeddingQueueStatusService
}

// EmbeddingQueueStatusCounts is the embedding-queue counter snapshot carried by
// the merged GET /api/poll bundle.
//
// 队列清空判据与日报调度完全一致（night-window-alignment 约束 23）：
// pending + processing == 0 才算清空；failed 是终态、不计入活跃量
// （与标签队列同口径：failed 只做红色提示，不阻断生成）。
//
// 单位是**标签**（embedding_queues 行按 tag_id 排），与标签队列的**文章**
// 单位不同——展示时必须分别标注量纲，不能把两个数当成同一件事。
type EmbeddingQueueStatusCounts struct {
	Pending        int64 `json:"pending"`
	Processing     int64 `json:"processing"`
	Completed      int64 `json:"completed"`
	Failed         int64 `json:"failed"`
	Total          int64 `json:"total"`
	CompletedToday int64 `json:"completed_today"`
}

// EmbeddingQueueStatusSnapshot queries the current embedding-queue counters.
// Shared with the admin /api/poll bundle so both always agree on the same SQL
// and status-mapping semantics (the dedicated GET /api/embedding/queue/status
// endpoint keeps its map-shaped response and is left untouched).
func EmbeddingQueueStatusSnapshot() (EmbeddingQueueStatusCounts, error) {
	reader := getEmbeddingQueueStatusReader()

	type statusRow struct {
		Status string
		Count  int64
	}
	var rows []statusRow
	if err := reader.db.Model(&models.EmbeddingQueue{}).
		Select("status, count(*) as count").
		Group("status").
		Scan(&rows).Error; err != nil {
		return EmbeddingQueueStatusCounts{}, err
	}

	counts := EmbeddingQueueStatusCounts{}
	var total int64
	for _, r := range rows {
		total += r.Count
		switch r.Status {
		case models.EmbeddingQueueStatusPending:
			counts.Pending = r.Count
		case models.EmbeddingQueueStatusProcessing:
			counts.Processing = r.Count
		case models.EmbeddingQueueStatusCompleted:
			counts.Completed = r.Count
		case models.EmbeddingQueueStatusFailed:
			counts.Failed = r.Count
		}
	}
	counts.Total = total

	// CompletedToday = 今日入队且已完成的标签数（embedding 队列按批入队，
	// 「今日完成」= 今日这批的推进量）。服务器本地日界，与标签队列同约定。
	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if err := reader.db.Model(&models.EmbeddingQueue{}).
		Where("status = ? AND created_at >= ?", models.EmbeddingQueueStatusCompleted, startOfToday).
		Count(&counts.CompletedToday).Error; err != nil {
		return EmbeddingQueueStatusCounts{}, err
	}
	return counts, nil
}
