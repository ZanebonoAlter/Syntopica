package narrative

import (
	"encoding/json"
	"fmt"
	"strings"
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
		&models.SemanticLabel{},
		&models.TopicTagBoardLabel{},
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
	cat := models.Category{ID: id, Name: name, Slug: fmt.Sprintf("category-%d", id)}
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

func TestCollectSemanticBoardNarrativeInputs_ColdStartNoBoard(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	tag := seedNarrativeEventTag(t, db, "霍尔木兹海峡", "hormuz")
	seedNarrativeArticleForTag(t, db, tag.ID, nil, date)

	inputs, err := CollectSemanticBoardNarrativeInputs(date, models.NarrativeScopeTypeGlobal, nil)
	if err != nil {
		t.Fatalf("CollectSemanticBoardNarrativeInputs: %v", err)
	}
	if len(inputs) != 0 {
		t.Fatalf("expected no boards during cold start, got %d", len(inputs))
	}
}

func TestCollectSemanticBoardNarrativeInputs_CategoryScope(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	catA := uint(5)
	catB := uint(6)
	seedCategory(t, db, catA, "Tech")
	seedCategory(t, db, catB, "Energy")
	board := seedSemanticBoard(t, db, "AI与机器学习", "ai-board")
	tagA := seedNarrativeEventTag(t, db, "OpenAI", "openai-event")
	tagB := seedNarrativeEventTag(t, db, "油价上涨", "oil-price")
	seedNarrativeArticleForTag(t, db, tagA.ID, &catA, date)
	seedNarrativeArticleForTag(t, db, tagB.ID, &catB, date)
	seedTagBoardLabel(t, db, tagA.ID, board.ID)
	seedTagBoardLabel(t, db, tagB.ID, board.ID)

	inputs, err := CollectSemanticBoardNarrativeInputs(date, models.NarrativeScopeTypeFeedCategory, &catA)
	if err != nil {
		t.Fatalf("CollectSemanticBoardNarrativeInputs: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected 1 semantic board input, got %d", len(inputs))
	}
	if inputs[0].Board.ID != board.ID {
		t.Fatalf("expected board %d, got %d", board.ID, inputs[0].Board.ID)
	}
	if len(inputs[0].EventTags) != 1 || inputs[0].EventTags[0].ID != tagA.ID {
		t.Fatalf("expected only category tag %d, got %+v", tagA.ID, inputs[0].EventTags)
	}
}

func TestCollectSemanticBoardNarrativeInputs_GlobalScope(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	catA := uint(5)
	catB := uint(6)
	seedCategory(t, db, catA, "Tech")
	seedCategory(t, db, catB, "Energy")
	board := seedSemanticBoard(t, db, "能源安全", "energy-security")
	tagA := seedNarrativeEventTag(t, db, "霍尔木兹海峡", "hormuz")
	tagB := seedNarrativeEventTag(t, db, "油轮保险", "tanker-insurance")
	seedNarrativeArticleForTag(t, db, tagA.ID, &catA, date)
	seedNarrativeArticleForTag(t, db, tagB.ID, &catB, date)
	seedTagBoardLabel(t, db, tagA.ID, board.ID)
	seedTagBoardLabel(t, db, tagB.ID, board.ID)

	inputs, err := CollectSemanticBoardNarrativeInputs(date, models.NarrativeScopeTypeGlobal, nil)
	if err != nil {
		t.Fatalf("CollectSemanticBoardNarrativeInputs: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected 1 semantic board input, got %d", len(inputs))
	}
	if len(inputs[0].EventTags) != 2 {
		t.Fatalf("expected both global event tags, got %+v", inputs[0].EventTags)
	}
}

func TestCollectSemanticBoardNarrativeInputs_AllowsDuplicateEventTagAcrossBoards(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	geopolitics := seedSemanticBoard(t, db, "地缘政治", "geopolitics")
	energy := seedSemanticBoard(t, db, "能源安全", "energy-security")
	tag := seedNarrativeEventTag(t, db, "霍尔木兹海峡", "hormuz")
	seedNarrativeArticleForTag(t, db, tag.ID, nil, date)
	seedTagBoardLabel(t, db, tag.ID, geopolitics.ID)
	seedTagBoardLabel(t, db, tag.ID, energy.ID)

	inputs, err := CollectSemanticBoardNarrativeInputs(date, models.NarrativeScopeTypeGlobal, nil)
	if err != nil {
		t.Fatalf("CollectSemanticBoardNarrativeInputs: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected 2 board inputs, got %d", len(inputs))
	}
	for _, input := range inputs {
		if len(input.EventTags) != 1 || input.EventTags[0].ID != tag.ID {
			t.Fatalf("expected duplicate tag %d in every board, got %+v", tag.ID, inputs)
		}
	}
}

func TestCreateBoardFromSemanticBoard_WritesSemanticScopeAndPrev(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	catID := uint(5)
	board := seedSemanticBoard(t, db, "AI与机器学习", "ai-board")
	otherBoard := seedSemanticBoard(t, db, "能源安全", "energy-board")
	tag := seedNarrativeEventTag(t, db, "OpenAI", "openai-event")
	yesterday := date.AddDate(0, 0, -1)
	twoDaysAgo := date.AddDate(0, 0, -2)
	previous := models.NarrativeBoard{
		PeriodDate:      yesterday,
		Name:            board.Label,
		ScopeType:       models.NarrativeScopeTypeFeedCategory,
		ScopeCategoryID: &catID,
		SemanticBoardID: &board.ID,
	}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatalf("create previous board: %v", err)
	}
	ignored := []models.NarrativeBoard{
		{PeriodDate: twoDaysAgo, Name: board.Label, ScopeType: models.NarrativeScopeTypeFeedCategory, ScopeCategoryID: &catID, SemanticBoardID: &board.ID},
		{PeriodDate: yesterday, Name: board.Label, ScopeType: models.NarrativeScopeTypeGlobal, SemanticBoardID: &board.ID},
		{PeriodDate: yesterday, Name: otherBoard.Label, ScopeType: models.NarrativeScopeTypeFeedCategory, ScopeCategoryID: &catID, SemanticBoardID: &otherBoard.ID},
	}
	if err := db.Create(&ignored).Error; err != nil {
		t.Fatalf("create ignored previous boards: %v", err)
	}

	prevIDs := matchPreviousSemanticBoard(board.ID, date, models.NarrativeScopeTypeFeedCategory, &catID)
	created, err := createBoardFromSemanticBoard(SemanticBoardNarrativeInput{
		Board:        board,
		EventTags:    []TagInput{{ID: tag.ID, Label: tag.Label}},
		PrevBoardIDs: prevIDs,
	}, date, ScopeSaveOpts{ScopeType: models.NarrativeScopeTypeFeedCategory, CategoryID: &catID, Label: "Tech"})
	if err != nil {
		t.Fatalf("createBoardFromSemanticBoard: %v", err)
	}
	if created.SemanticBoardID == nil || *created.SemanticBoardID != board.ID {
		t.Fatalf("expected semantic_board_id %d, got %v", board.ID, created.SemanticBoardID)
	}
	if created.ScopeType != models.NarrativeScopeTypeFeedCategory || created.ScopeCategoryID == nil || *created.ScopeCategoryID != catID {
		t.Fatalf("unexpected scope: type=%s category=%v", created.ScopeType, created.ScopeCategoryID)
	}
	if created.ScopeLabel != "Tech" {
		t.Fatalf("expected scope label Tech, got %q", created.ScopeLabel)
	}
	var eventIDs []uint
	if err := json.Unmarshal([]byte(created.EventTagIDs), &eventIDs); err != nil {
		t.Fatalf("unmarshal event ids: %v", err)
	}
	if len(eventIDs) != 1 || eventIDs[0] != tag.ID {
		t.Fatalf("expected event tag ids [%d], got %v", tag.ID, eventIDs)
	}

	var createdPrevIDs []uint
	if err := json.Unmarshal([]byte(created.PrevBoardIDs), &createdPrevIDs); err != nil {
		t.Fatalf("unmarshal prev ids: %v", err)
	}
	if len(createdPrevIDs) != 1 || createdPrevIDs[0] != previous.ID {
		t.Fatalf("expected prev board id %d, got %v", previous.ID, createdPrevIDs)
	}
}

func TestCreateBoardFromSemanticBoard_MatchesPreviousGlobalScope(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	catID := uint(5)
	board := seedSemanticBoard(t, db, "能源安全", "energy-security")
	tag := seedNarrativeEventTag(t, db, "霍尔木兹海峡", "hormuz")
	yesterday := date.AddDate(0, 0, -1)
	previous := models.NarrativeBoard{PeriodDate: yesterday, Name: board.Label, ScopeType: models.NarrativeScopeTypeGlobal, SemanticBoardID: &board.ID}
	ignored := models.NarrativeBoard{PeriodDate: yesterday, Name: board.Label, ScopeType: models.NarrativeScopeTypeFeedCategory, ScopeCategoryID: &catID, SemanticBoardID: &board.ID}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatalf("create previous global board: %v", err)
	}
	if err := db.Create(&ignored).Error; err != nil {
		t.Fatalf("create ignored category board: %v", err)
	}

	prevIDs := matchPreviousSemanticBoard(board.ID, date, models.NarrativeScopeTypeGlobal, nil)
	created, err := createBoardFromSemanticBoard(SemanticBoardNarrativeInput{
		Board:        board,
		EventTags:    []TagInput{{ID: tag.ID, Label: tag.Label}},
		PrevBoardIDs: prevIDs,
	}, date, ScopeSaveOpts{ScopeType: models.NarrativeScopeTypeGlobal, Label: "Global"})
	if err != nil {
		t.Fatalf("createBoardFromSemanticBoard: %v", err)
	}

	var createdPrevIDs []uint
	if err := json.Unmarshal([]byte(created.PrevBoardIDs), &createdPrevIDs); err != nil {
		t.Fatalf("unmarshal prev ids: %v", err)
	}
	if len(createdPrevIDs) != 1 || createdPrevIDs[0] != previous.ID {
		t.Fatalf("expected global prev board id %d, got %v", previous.ID, createdPrevIDs)
	}
}

func TestSaveNarrativesWithBoard_CategoryScopeFiltersArticleIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	date := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	catA := uint(5)
	catB := uint(6)
	seedCategory(t, db, catA, "Tech")
	seedCategory(t, db, catB, "Energy")
	tag := seedNarrativeEventTag(t, db, "OpenAI", "openai-event")
	articleA := seedNarrativeArticleForTag(t, db, tag.ID, &catA, date)
	seedNarrativeArticleForTag(t, db, tag.ID, &catB, date)
	board := models.NarrativeBoard{
		PeriodDate:      date,
		Name:            "AI与机器学习",
		ScopeType:       models.NarrativeScopeTypeFeedCategory,
		ScopeCategoryID: &catA,
		EventTagIDs:     fmt.Sprintf("[%d]", tag.ID),
	}
	if err := db.Create(&board).Error; err != nil {
		t.Fatalf("create board: %v", err)
	}

	saved, err := saveNarrativesWithBoard([]NarrativeOutput{{
		Title:         "AI叙事",
		Summary:       "AI 摘要",
		Status:        models.NarrativeStatusEmerging,
		RelatedTagIDs: []uint{tag.ID},
	}}, board, date, &ScopeSaveOpts{ScopeType: models.NarrativeScopeTypeFeedCategory, CategoryID: &catA, Label: "Tech"})
	if err != nil {
		t.Fatalf("saveNarrativesWithBoard: %v", err)
	}
	if saved != 1 {
		t.Fatalf("expected saved=1, got %d", saved)
	}

	var summary models.NarrativeSummary
	if err := db.First(&summary).Error; err != nil {
		t.Fatalf("load summary: %v", err)
	}
	var articleIDs []uint64
	if err := json.Unmarshal([]byte(summary.RelatedArticleIDs), &articleIDs); err != nil {
		t.Fatalf("unmarshal article ids: %v", err)
	}
	if len(articleIDs) != 1 || articleIDs[0] != uint64(articleA.ID) {
		t.Fatalf("expected only category article %d, got %v", articleA.ID, articleIDs)
	}
}

func TestBuildBoardNarrativePrompt_UsesSemanticBoardContext(t *testing.T) {
	prompt := buildBoardNarrativePrompt(BoardNarrativeContext{
		Board:              models.NarrativeBoard{Name: "旧看板名", Description: "旧描述"},
		SemanticBoardLabel: "AI与机器学习",
		SemanticBoardDesc:  "追踪 AI 基础模型、应用和产业链变化",
		EventTags:          []TagInput{{ID: 1, Label: "OpenAI", ArticleCount: 2}},
	})

	if !strings.Contains(prompt, "SemanticBoard: AI与机器学习") {
		t.Fatalf("prompt missing semantic board label: %s", prompt)
	}
	if !strings.Contains(prompt, "SemanticBoard 描述: 追踪 AI 基础模型、应用和产业链变化") {
		t.Fatalf("prompt missing semantic board description: %s", prompt)
	}
	if strings.Contains(prompt, "旧看板名") || strings.Contains(prompt, "旧描述") {
		t.Fatalf("prompt should prefer semantic board context over old board fields: %s", prompt)
	}
}

func TestBuildBoardNarrativePrompt_FallsBackToBoardContext(t *testing.T) {
	prompt := buildBoardNarrativePrompt(BoardNarrativeContext{
		Board:     models.NarrativeBoard{Name: "旧看板名", Description: "旧描述"},
		EventTags: []TagInput{{ID: 1, Label: "OpenAI", ArticleCount: 2}},
	})

	if !strings.Contains(prompt, "SemanticBoard: 旧看板名") {
		t.Fatalf("prompt missing fallback board name: %s", prompt)
	}
	if !strings.Contains(prompt, "SemanticBoard 描述: 旧描述") {
		t.Fatalf("prompt missing fallback board description: %s", prompt)
	}
}

func seedSemanticBoard(t *testing.T, db *gorm.DB, label string, slug string) models.SemanticLabel {
	t.Helper()
	board := models.SemanticLabel{Label: label, Slug: slug, LabelType: "board", Status: "active", Description: label + " description"}
	if err := db.Create(&board).Error; err != nil {
		t.Fatalf("seed semantic board: %v", err)
	}
	return board
}

func seedNarrativeEventTag(t *testing.T, db *gorm.DB, label string, slug string) models.TopicTag {
	t.Helper()
	tag := models.TopicTag{Label: label, Slug: slug, Category: models.TagCategoryEvent, Status: "active", Source: "llm"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("seed event tag: %v", err)
	}
	return tag
}

func seedNarrativeArticleForTag(t *testing.T, db *gorm.DB, topicTagID uint, categoryID *uint, date time.Time) models.Article {
	t.Helper()
	categoryKey := uint(0)
	if categoryID != nil {
		categoryKey = *categoryID
	}
	feed := models.Feed{Title: fmt.Sprintf("Feed %d-%d", topicTagID, categoryKey), URL: fmt.Sprintf("https://example.com/feed/%d/%d", topicTagID, categoryKey), CategoryID: categoryID}
	if err := db.Create(&feed).Error; err != nil {
		t.Fatalf("seed feed: %v", err)
	}
	pubDate := date.Add(10 * time.Hour)
	article := models.Article{FeedID: feed.ID, Title: fmt.Sprintf("Article %d-%d", topicTagID, categoryKey), Link: fmt.Sprintf("https://example.com/article/%d/%d", topicTagID, categoryKey), PubDate: &pubDate}
	if err := db.Create(&article).Error; err != nil {
		t.Fatalf("seed article: %v", err)
	}
	if err := db.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: topicTagID}).Error; err != nil {
		t.Fatalf("seed article tag: %v", err)
	}
	return article
}

func seedTagBoardLabel(t *testing.T, db *gorm.DB, topicTagID uint, boardID uint) {
	t.Helper()
	if err := db.Create(&models.TopicTagBoardLabel{TopicTagID: topicTagID, SemanticBoardID: boardID, Score: 1, MatchReason: "test"}).Error; err != nil {
		t.Fatalf("seed tag board label: %v", err)
	}
}
