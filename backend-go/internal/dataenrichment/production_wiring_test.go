package dataenrichment

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/platform/database"
)

// ── dbLifelineReader.GetTopicLifelineArchive（fix-board-analysis-material tasks 2.1 / M3.5）──
//
// 归档行选取：month 最新 2 期 + year 最新 1 期；month 在前（近期主体），year 压尾；
// 无记录返回空切片（不报错）。

func setupArchiveReaderDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:test-archive-reader?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database.DB = db
	if err := database.RunAutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedArchive(t *testing.T, db *gorm.DB, topicID uint, granularity, period, content string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO topic_lifeline_context (persistent_topic_id, granularity, period, content, as_of_date, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '2026-08-26', 'manual', datetime('now'), datetime('now'))`,
		topicID, granularity, period, content).Error; err != nil {
		t.Fatalf("seed archive row: %v", err)
	}
}

func TestGetTopicLifelineArchive_SelectionAndOrder(t *testing.T) {
	db := setupArchiveReaderDB(t)
	reader := NewDBLifelineReader(db)

	seedArchive(t, db, 1, "month", "2026-08", "八月主线")
	seedArchive(t, db, 1, "month", "2026-07", "七月主线")
	seedArchive(t, db, 1, "month", "2026-06", "六月不该入选")
	seedArchive(t, db, 1, "year", "2026", "年度背景")
	seedArchive(t, db, 1, "year", "2025", "去年不该入选")
	seedArchive(t, db, 1, "week", "2026-W35", "week 不属归档")

	rows, err := reader.GetTopicLifelineArchive(1)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows (2 month + 1 year), got %d: %+v", len(rows), rows)
	}
	wantOrder := []struct{ gran, period string }{
		{"month", "2026-08"}, {"month", "2026-07"}, {"year", "2026"},
	}
	for i, w := range wantOrder {
		if rows[i].Granularity != w.gran || rows[i].Period != w.period {
			t.Fatalf("row %d: want %s %s, got %s %s", i, w.gran, w.period, rows[i].Granularity, rows[i].Period)
		}
	}
}

func TestGetTopicLifelineArchive_NoRecords(t *testing.T) {
	db := setupArchiveReaderDB(t)
	reader := NewDBLifelineReader(db)

	rows, err := reader.GetTopicLifelineArchive(999)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("want 0 rows, got %d", len(rows))
	}
}
