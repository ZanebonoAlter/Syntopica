package tagging

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupDateContextTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	database.DB = db

	if err := db.AutoMigrate(&models.TopicTag{}, &models.TopicTagEmbedding{}); err != nil {
		t.Fatalf("migrate test tables: %v", err)
	}

	db.AutoMigrate(&models.Article{}, &models.Feed{}, &models.ArticleTopicTag{})

	return db
}

func TestBuildExtractionUserPrompt_WithPubDate(t *testing.T) {
	input := ExtractionInput{
		Title:        "特朗普访华",
		Summary:      "特朗普抵达北京进行访问",
		FeedName:     "News Feed",
		CategoryName: "Politics",
		PubDate:      "2025-05-10",
	}

	prompt := buildExtractionUserPrompt(input)

	if !strings.Contains(prompt, "发布日期: 2025-05-10") {
		t.Error("prompt should contain publication date line")
	}
	if !strings.Contains(prompt, "特朗普访华") {
		t.Error("prompt should contain title")
	}
}

func TestBuildExtractionUserPrompt_EmptyPubDate(t *testing.T) {
	input := ExtractionInput{
		Title:        "特朗普访华",
		Summary:      "特朗普抵达北京进行访问",
		FeedName:     "News Feed",
		CategoryName: "Politics",
		PubDate:      "",
	}

	prompt := buildExtractionUserPrompt(input)

	if strings.Contains(prompt, "发布日期") {
		t.Error("prompt should NOT contain 发布日期 when PubDate is empty")
	}
}

func TestFormatPubDate_Valid(t *testing.T) {
	tm := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)
	result := formatPubDate(&tm)
	if result != "2025-05-10" {
		t.Errorf("expected '2025-05-10', got %q", result)
	}
}

func TestFormatPubDate_Nil(t *testing.T) {
	result := formatPubDate(nil)
	if result != "" {
		t.Errorf("expected empty string for nil PubDate, got %q", result)
	}
}

func TestLoadTagDateRanges_SingleArticle(t *testing.T) {
	db := setupDateContextTestDB(t)

	db.AutoMigrate(&models.Article{})

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com"}
	db.Create(&feed)

	tm := time.Date(2025, 5, 10, 12, 0, 0, 0, time.UTC)
	article := models.Article{
		FeedID:  feed.ID,
		Title:   "Test Article",
		Link:    "https://example.com/1",
		PubDate: &tm,
	}
	db.Create(&article)

	tag := models.TopicTag{
		Slug: "test-tag", Label: "Test Tag", Category: "event", Status: "active",
	}
	db.Create(&tag)

	db.Create(&models.ArticleTopicTag{
		ArticleID:  article.ID,
		TopicTagID: tag.ID,
	})

	ranges := loadTagDateRanges([]uint{tag.ID})
	if ranges == nil {
		t.Fatal("expected non-nil date ranges")
	}
	val, ok := ranges[tag.ID]
	if !ok {
		t.Fatal("expected date range for tag")
	}
	if !strings.Contains(val, "2025-05-10") {
		t.Errorf("expected date range containing 2025-05-10, got %q", val)
	}
}

func TestLoadTagDateRanges_MultipleArticles(t *testing.T) {
	db := setupDateContextTestDB(t)

	db.AutoMigrate(&models.Article{})

	feed := models.Feed{Title: "Test Feed", URL: "https://example.com"}
	db.Create(&feed)

	tm1 := time.Date(2025, 5, 8, 12, 0, 0, 0, time.UTC)
	tm2 := time.Date(2025, 5, 12, 12, 0, 0, 0, time.UTC)
	a1 := models.Article{
		FeedID:  feed.ID,
		Title:   "Article 1",
		Link:    "https://example.com/1",
		PubDate: &tm1,
	}
	db.Create(&a1)
	a2 := models.Article{
		FeedID:  feed.ID,
		Title:   "Article 2",
		Link:    "https://example.com/2",
		PubDate: &tm2,
	}
	db.Create(&a2)

	tag := models.TopicTag{
		Slug: "test-tag", Label: "Test Tag", Category: "event", Status: "active",
	}
	db.Create(&tag)

	db.Create(&models.ArticleTopicTag{ArticleID: a1.ID, TopicTagID: tag.ID})
	db.Create(&models.ArticleTopicTag{ArticleID: a2.ID, TopicTagID: tag.ID})

	ranges := loadTagDateRanges([]uint{tag.ID})
	val, _ := ranges[tag.ID]

	if !strings.Contains(val, "最早文章: 2025-05-08") {
		t.Errorf("expected 最早文章: 2025-05-08, got %q", val)
	}
	if !strings.Contains(val, "最新: 2025-05-12") {
		t.Errorf("expected 最新: 2025-05-12, got %q", val)
	}
}

func TestLoadTagDateRanges_EmptyTagIDs(t *testing.T) {
	_ = setupDateContextTestDB(t)
	ranges := loadTagDateRanges(nil)
	if ranges != nil {
		t.Error("expected nil for empty tag IDs")
	}
	ranges = loadTagDateRanges([]uint{})
	if ranges != nil {
		t.Error("expected nil for empty tag IDs slice")
	}
}

func TestBuildCandidateList_IncludesDateRange(t *testing.T) {
	tag := &models.TopicTag{
		ID:    1,
		Label: "Test Tag",
	}

	candidates := []TagCandidate{
		{Tag: tag, Similarity: 0.95, DateRange: "(文章日期: 2025-05-10)"},
	}

	result := buildCandidateList(candidates)
	if !strings.Contains(result, "文章日期: 2025-05-10") {
		t.Errorf("candidate list should include date range, got: %s", result)
	}
}

func TestBuildCandidateList_NoDateRange(t *testing.T) {
	tag := &models.TopicTag{
		ID:    1,
		Label: "Test Tag",
	}

	candidates := []TagCandidate{
		{Tag: tag, Similarity: 0.95},
	}

	result := buildCandidateList(candidates)
	if strings.Contains(result, "文章日期") || strings.Contains(result, "最早文章") {
		t.Error("candidate list should NOT contain date info when DateRange is empty")
	}
}
