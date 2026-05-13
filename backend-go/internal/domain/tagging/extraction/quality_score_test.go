package extraction

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"my-robot-backend/internal/domain/models"
	"my-robot-backend/internal/platform/database"
)

func setupQualityScoreTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	if err := database.DB.AutoMigrate(
		&models.Feed{},
		&models.Article{},
		&models.TopicTag{},
		&models.TopicTagEmbedding{},
		&models.ArticleTopicTag{},
		&models.TopicTagRelation{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestPercentileRankStableRange(t *testing.T) {
	values := map[uint]float64{1: 10, 2: 20, 3: 20, 4: 50}

	lowest := percentileRank(values, 1)
	middle := percentileRank(values, 2)
	highest := percentileRank(values, 4)

	if lowest < 0 || lowest > 1 {
		t.Fatalf("lowest percentile out of range: %f", lowest)
	}
	if middle < 0 || middle > 1 {
		t.Fatalf("middle percentile out of range: %f", middle)
	}
	if highest < 0 || highest > 1 {
		t.Fatalf("highest percentile out of range: %f", highest)
	}
	if !(lowest < middle && middle < highest) {
		t.Fatalf("expected ordered percentiles, got lowest=%f middle=%f highest=%f", lowest, middle, highest)
	}
}

func TestComputeAllQualityScoresDefaultsAndEmptyAssociations(t *testing.T) {
	setupQualityScoreTestDB(t)

	feed := models.Feed{Title: "Feed", URL: "https://example.com/feed"}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{
		FeedID: feed.ID,
		Title:  "Tagged article",
		Link:   "https://example.com/article-1",
	}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	normal := models.TopicTag{Slug: "quality-normal", Label: "Quality Normal", Category: models.TagCategoryKeyword, Status: "active"}
	empty := models.TopicTag{Slug: "quality-empty", Label: "Quality Empty", Category: models.TagCategoryKeyword, Status: "active"}
	if err := database.DB.Create(&normal).Error; err != nil {
		t.Fatalf("create normal tag: %v", err)
	}
	if err := database.DB.Create(&empty).Error; err != nil {
		t.Fatalf("create empty tag: %v", err)
	}

	link := models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: normal.ID, Score: 0.91, Source: "llm", UpdatedAt: time.Now()}
	if err := database.DB.Create(&link).Error; err != nil {
		t.Fatalf("create article tag link: %v", err)
	}

	if err := ComputeAllQualityScores(); err != nil {
		t.Fatalf("ComputeAllQualityScores: %v", err)
	}

	var refreshedNormal models.TopicTag
	if err := database.DB.First(&refreshedNormal, normal.ID).Error; err != nil {
		t.Fatalf("reload normal tag: %v", err)
	}
	var refreshedEmpty models.TopicTag
	if err := database.DB.First(&refreshedEmpty, empty.ID).Error; err != nil {
		t.Fatalf("reload empty tag: %v", err)
	}

	if refreshedEmpty.QualityScore != 0 {
		t.Fatalf("empty tag quality_score = %f, want 0", refreshedEmpty.QualityScore)
	}

	expected := 0.4*0.5 + 0.2*0.5 + 0.2*0.5 + 0.2*0.7
	if math.Abs(refreshedNormal.QualityScore-expected) > 0.0001 {
		t.Fatalf("normal tag quality_score = %f, want %f", refreshedNormal.QualityScore, expected)
	}
}
