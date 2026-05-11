package narrative

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&models.NarrativeSummary{},
		&models.NarrativeBoard{},
		&models.TopicTag{},
		&models.TopicTagRelation{},
		&models.ArticleTopicTag{},
		&models.Article{},
		&models.Feed{},
		&models.Category{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	database.DB = db
	t.Cleanup(func() { database.DB = nil })
	return db
}

func TestResolveGlobalGeneration_NoPrevious(t *testing.T) {
	setupServiceTestDB(t)

	date := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	gen := resolveGlobalGeneration(date)
	if gen != 0 {
		t.Errorf("expected generation 0 with no previous, got %d", gen)
	}
}

func TestResolveGlobalGeneration_WithPrevious(t *testing.T) {
	db := setupServiceTestDB(t)

	yesterday := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	db.Create(&models.NarrativeSummary{
		Title:      "Prev Global",
		Summary:    "Yesterday",
		Status:     "continuing",
		Period:     "daily",
		PeriodDate: yesterday,
		Generation: 2,
		Source:     "ai",
		ScopeType:  models.NarrativeScopeTypeGlobal,
	})

	date := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	gen := resolveGlobalGeneration(date)
	if gen != 3 {
		t.Errorf("expected generation 3 (max_gen 2 + 1), got %d", gen)
	}
}

func seedCategory(t *testing.T, db *gorm.DB, id uint, name string) models.Category {
	t.Helper()
	cat := models.Category{ID: id, Name: name}
	if err := db.Create(&cat).Error; err != nil {
		t.Fatalf("seed category: %v", err)
	}
	return cat
}

func seedBoard(t *testing.T, db *gorm.DB, scopeType string, categoryID *uint, periodDate time.Time) uint {
	t.Helper()
	b := models.NarrativeBoard{
		PeriodDate:      periodDate,
		Name:            "test-board",
		ScopeType:       scopeType,
		ScopeCategoryID: categoryID,
	}
	if err := db.Create(&b).Error; err != nil {
		t.Fatalf("seed board: %v", err)
	}
	return b.ID
}

func seedSummary(t *testing.T, db *gorm.DB, boardID *uint, scopeType string, categoryID *uint, periodDate time.Time) {
	t.Helper()
	s := models.NarrativeSummary{
		Title:           "test-narrative",
		Summary:         "test",
		Status:          models.NarrativeStatusEmerging,
		Period:          "daily",
		PeriodDate:      periodDate,
		Source:          "ai",
		ScopeType:       scopeType,
		ScopeCategoryID: categoryID,
		BoardID:         boardID,
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("seed summary: %v", err)
	}
}

func TestGetScopes_Empty(t *testing.T) {
	setupServiceTestDB(t)
	svc := NewNarrativeService()

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	resp, err := svc.GetScopes(date, 7)
	if err != nil {
		t.Fatalf("GetScopes: %v", err)
	}
	if resp.GlobalCount != 0 {
		t.Errorf("expected 0 global boards, got %d", resp.GlobalCount)
	}
	if len(resp.Categories) != 0 {
		t.Errorf("expected 0 categories, got %d", len(resp.Categories))
	}
}

func TestGetScopes_CategoryBoards(t *testing.T) {
	db := setupServiceTestDB(t)
	svc := NewNarrativeService()

	catID := uint(1)
	seedCategory(t, db, catID, "Tech")
	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, date)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, date)

	resp, err := svc.GetScopes(date, 7)
	if err != nil {
		t.Fatalf("GetScopes: %v", err)
	}
	if len(resp.Categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(resp.Categories))
	}
	if resp.Categories[0].BoardCount != 2 {
		t.Errorf("expected board_count=2, got %d", resp.Categories[0].BoardCount)
	}
	if resp.Categories[0].CategoryName != "Tech" {
		t.Errorf("expected category_name=Tech, got %s", resp.Categories[0].CategoryName)
	}
}

func TestGetScopes_BoardsWithoutSummariesStillShow(t *testing.T) {
	db := setupServiceTestDB(t)
	svc := NewNarrativeService()

	catID := uint(5)
	seedCategory(t, db, catID, "Science")
	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, date)

	resp, err := svc.GetScopes(date, 7)
	if err != nil {
		t.Fatalf("GetScopes: %v", err)
	}
	if len(resp.Categories) != 1 {
		t.Fatalf("expected 1 category (board has no summaries), got %d", len(resp.Categories))
	}
	if resp.Categories[0].BoardCount != 1 {
		t.Errorf("expected board_count=1, got %d", resp.Categories[0].BoardCount)
	}
}

func TestGetScopes_GlobalCount(t *testing.T) {
	db := setupServiceTestDB(t)
	svc := NewNarrativeService()

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	seedBoard(t, db, models.NarrativeScopeTypeGlobal, nil, date)
	seedBoard(t, db, models.NarrativeScopeTypeGlobal, nil, date)

	resp, err := svc.GetScopes(date, 7)
	if err != nil {
		t.Fatalf("GetScopes: %v", err)
	}
	if resp.GlobalCount != 2 {
		t.Errorf("expected global_count=2, got %d", resp.GlobalCount)
	}
}

func TestGetScopes_DaysRange(t *testing.T) {
	db := setupServiceTestDB(t)
	svc := NewNarrativeService()

	catID := uint(10)
	seedCategory(t, db, catID, "Sports")
	anchor := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)

	withinRange := anchor.AddDate(0, 0, -5)
	outOfRange := anchor.AddDate(0, 0, -10)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, withinRange)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, outOfRange)

	resp, err := svc.GetScopes(anchor, 7)
	if err != nil {
		t.Fatalf("GetScopes: %v", err)
	}
	if len(resp.Categories) != 1 {
		t.Fatalf("expected 1 category (within 7-day range), got %d", len(resp.Categories))
	}
	if resp.Categories[0].BoardCount != 1 {
		t.Errorf("expected board_count=1, got %d", resp.Categories[0].BoardCount)
	}

	resp3, err := svc.GetScopes(anchor, 3)
	if err != nil {
		t.Fatalf("GetScopes days=3: %v", err)
	}
	if len(resp3.Categories) != 0 {
		t.Errorf("expected 0 categories for days=3 (board is 5 days old), got %d", len(resp3.Categories))
	}
}

func TestGetScopes_DefaultDays(t *testing.T) {
	setupServiceTestDB(t)
	svc := NewNarrativeService()

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	resp, err := svc.GetScopes(date, 0)
	if err != nil {
		t.Fatalf("GetScopes with days=0: %v", err)
	}
	if resp.Date != "2026-04-30" {
		t.Errorf("expected date 2026-04-30, got %s", resp.Date)
	}
}

func TestCleanEmptyBoards_NoEmpty(t *testing.T) {
	db := setupServiceTestDB(t)

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	catID := uint(1)
	boardID := seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, date)
	seedSummary(t, db, &boardID, models.NarrativeScopeTypeFeedCategory, &catID, date)

	cleanEmptyBoards(date, &catID)

	var count int64
	db.Model(&models.NarrativeBoard{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 board (not empty), got %d", count)
	}
}

func TestCleanEmptyBoards_RemovesOrphanedBoard(t *testing.T) {
	db := setupServiceTestDB(t)

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	catID := uint(1)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &catID, date)

	cleanEmptyBoards(date, &catID)

	var count int64
	db.Model(&models.NarrativeBoard{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 boards (empty one deleted), got %d", count)
	}
}

func TestCleanEmptyBoards_ScopedByCategory(t *testing.T) {
	db := setupServiceTestDB(t)

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	cat1 := uint(1)
	cat2 := uint(2)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &cat1, date)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, &cat2, date)

	cleanEmptyBoards(date, &cat1)

	var count int64
	db.Model(&models.NarrativeBoard{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 board (cat2 untouched), got %d", count)
	}
}

func TestCleanEmptyBoards_GlobalCleanup(t *testing.T) {
	db := setupServiceTestDB(t)

	date := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	seedBoard(t, db, models.NarrativeScopeTypeGlobal, nil, date)
	seedBoard(t, db, models.NarrativeScopeTypeFeedCategory, nil, date)

	cleanEmptyBoards(date, nil)

	var count int64
	db.Model(&models.NarrativeBoard{}).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 boards (all empty), got %d", count)
	}
}

func TestCleanEmptyBoards_DoesNotAffectOtherDates(t *testing.T) {
	db := setupServiceTestDB(t)

	today := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	yesterday := time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC)
	seedBoard(t, db, models.NarrativeScopeTypeGlobal, nil, yesterday)

	cleanEmptyBoards(today, nil)

	var count int64
	db.Model(&models.NarrativeBoard{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 board (different date), got %d", count)
	}
}
