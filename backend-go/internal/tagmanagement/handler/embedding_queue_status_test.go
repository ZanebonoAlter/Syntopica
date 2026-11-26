package handler

import (
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
)

// setupEmbeddingQueueStatusTestDB initializes the shared test DB and resets the
// embedding-queue status reader singleton so it binds to the test database
// (same pattern as tag_queue_status_test.go).
func setupEmbeddingQueueStatusTestDB(t *testing.T) *gorm.DB {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)

	embeddingQueueStatusService = nil
	embeddingQueueStatusOnce = sync.Once{}

	return db
}

func seedEmbeddingQueueAt(t *testing.T, db *gorm.DB, tagID uint, status string, createdAt time.Time) {
	t.Helper()
	row := models.EmbeddingQueue{
		TagID:  tagID,
		Status: status,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed embedding queue row: %v", err)
	}
	// created_at is auto-populated by GORM on insert; pin it explicitly to
	// exercise the today/yesterday boundary.
	if err := db.Model(&models.EmbeddingQueue{}).Where("id = ?", row.ID).
		Update("created_at", createdAt).Error; err != nil {
		t.Fatalf("set created_at: %v", err)
	}
}

// TestEmbeddingQueueStatusSnapshot_Counts pins the counter mapping: each
// status lands in its own bucket and total is the sum of all rows.
func TestEmbeddingQueueStatusSnapshot_Counts(t *testing.T) {
	db := setupEmbeddingQueueStatusTestDB(t)

	now := time.Now()

	seedEmbeddingQueueAt(t, db, 201, models.EmbeddingQueueStatusPending, now)
	seedEmbeddingQueueAt(t, db, 202, models.EmbeddingQueueStatusPending, now)
	seedEmbeddingQueueAt(t, db, 203, models.EmbeddingQueueStatusProcessing, now)
	seedEmbeddingQueueAt(t, db, 204, models.EmbeddingQueueStatusCompleted, now)
	seedEmbeddingQueueAt(t, db, 205, models.EmbeddingQueueStatusFailed, now)

	counts, err := EmbeddingQueueStatusSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if counts.Pending != 2 {
		t.Fatalf("pending = %d, want 2", counts.Pending)
	}
	if counts.Processing != 1 {
		t.Fatalf("processing = %d, want 1", counts.Processing)
	}
	if counts.Completed != 1 {
		t.Fatalf("completed = %d, want 1", counts.Completed)
	}
	if counts.Failed != 1 {
		t.Fatalf("failed = %d, want 1", counts.Failed)
	}
	if counts.Total != 5 {
		t.Fatalf("total = %d, want 5", counts.Total)
	}
}

// TestEmbeddingQueueStatusSnapshot_CompletedTodayBoundary covers the
// today/yesterday day boundary for completed_today (mirrors the tag-queue
// 白盒 C 边界): yesterday's completed rows count only toward cumulative
// completed, today's count toward both.
func TestEmbeddingQueueStatusSnapshot_CompletedTodayBoundary(t *testing.T) {
	db := setupEmbeddingQueueStatusTestDB(t)

	now := time.Now()
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := startOfToday.Add(-time.Hour)

	seedEmbeddingQueueAt(t, db, 301, models.EmbeddingQueueStatusCompleted, startOfToday)
	seedEmbeddingQueueAt(t, db, 302, models.EmbeddingQueueStatusCompleted, startOfToday.Add(time.Hour))
	seedEmbeddingQueueAt(t, db, 303, models.EmbeddingQueueStatusCompleted, yesterday)
	// non-completed rows must not leak into completed_today.
	seedEmbeddingQueueAt(t, db, 304, models.EmbeddingQueueStatusPending, startOfToday.Add(time.Hour))
	seedEmbeddingQueueAt(t, db, 305, models.EmbeddingQueueStatusFailed, yesterday)

	counts, err := EmbeddingQueueStatusSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if counts.CompletedToday != 2 {
		t.Fatalf("completed_today = %d, want 2 (today-completed only)", counts.CompletedToday)
	}
	if counts.Completed != 3 {
		t.Fatalf("completed = %d, want 3 (cumulative)", counts.Completed)
	}
}

// TestEmbeddingQueueStatusSnapshot_DrainPredicate pins the drain semantics
// shared with the daily-report scheduler (night-window-alignment 约束 23):
// only pending + processing are active; failed is a terminal state and must
// NOT keep the queue "not drained" (it is surfaced as a red hint instead).
func TestEmbeddingQueueStatusSnapshot_DrainPredicate(t *testing.T) {
	db := setupEmbeddingQueueStatusTestDB(t)

	now := time.Now()

	// failed-only queue: terminal → drain predicate satisfied.
	seedEmbeddingQueueAt(t, db, 401, models.EmbeddingQueueStatusFailed, now)
	counts, err := EmbeddingQueueStatusSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if counts.Pending+counts.Processing != 0 {
		t.Fatalf("failed-only queue must be drained, got pending=%d processing=%d",
			counts.Pending, counts.Processing)
	}
	if counts.Failed != 1 {
		t.Fatalf("failed = %d, want 1 (still reported as hint)", counts.Failed)
	}

	// one pending row → not drained.
	seedEmbeddingQueueAt(t, db, 402, models.EmbeddingQueueStatusPending, now)
	counts, err = EmbeddingQueueStatusSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if counts.Pending+counts.Processing == 0 {
		t.Fatal("queue with a pending row must not be drained")
	}
}

// TestEmbeddingQueueStatusSnapshot_ObservabilityFields pins the D5 fields:
// oldest_pending_age_seconds is the age of the oldest pending row (nil when
// none), throughput_per_min counts completions in the last 60s.
func TestEmbeddingQueueStatusSnapshot_ObservabilityFields(t *testing.T) {
	db := setupEmbeddingQueueStatusTestDB(t)

	// 空队列：oldest_pending_age_seconds 必须为 null（前端空态不炸），吞吐 0。
	counts, err := EmbeddingQueueStatusSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if counts.OldestPendingAgeSeconds != nil {
		t.Fatalf("oldest_pending_age_seconds = %v, want nil on empty queue", *counts.OldestPendingAgeSeconds)
	}
	if counts.ThroughputPerMin != 0 {
		t.Fatalf("throughput_per_min = %d, want 0", counts.ThroughputPerMin)
	}

	now := time.Now()
	seedEmbeddingQueueAt(t, db, 501, models.EmbeddingQueueStatusPending, now.Add(-90*time.Second))
	completed := models.EmbeddingQueue{TagID: 502, Status: models.EmbeddingQueueStatusCompleted, CompletedAt: &now}
	if err := db.Create(&completed).Error; err != nil {
		t.Fatalf("seed completed row: %v", err)
	}

	counts, err = EmbeddingQueueStatusSnapshot()
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if counts.OldestPendingAgeSeconds == nil {
		t.Fatal("oldest_pending_age_seconds must be set with a pending row")
	}
	if age := *counts.OldestPendingAgeSeconds; age < 80 || age > 120 {
		t.Fatalf("oldest_pending_age_seconds = %d, want ~90", age)
	}
	if counts.ThroughputPerMin != 1 {
		t.Fatalf("throughput_per_min = %d, want 1", counts.ThroughputPerMin)
	}
}
