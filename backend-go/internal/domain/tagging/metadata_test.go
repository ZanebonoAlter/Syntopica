package tagging

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
)

func setupTopicExtractionTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	database.DB = db
	if err := database.DB.AutoMigrate(
		&models.Feed{},
		&models.TopicTag{},
		&models.Article{},
		&models.ArticleTopicTag{},
		&models.AIProvider{},
		&models.AIRoute{},
		&models.AIRouteProvider{},
		&models.AICallLog{},
	); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
}

func TestBackfillArticleTagsOnlyTagsUntaggedArticles(t *testing.T) {
	setupTopicExtractionTestDB(t)

	feed := models.Feed{Title: "OpenAI Feed", URL: "https://example.com/feed"}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	preTagged := models.Article{FeedID: feed.ID, Title: "OpenAI note", Link: "https://example.com/pre", Description: "OpenAI update"}
	untagged := models.Article{FeedID: feed.ID, Title: "AI agent runtime", Link: "https://example.com/new", Description: "OpenAI agentic runtime"}
	if err := database.DB.Create(&preTagged).Error; err != nil {
		t.Fatalf("create pre-tagged article: %v", err)
	}
	if err := database.DB.Create(&untagged).Error; err != nil {
		t.Fatalf("create untagged article: %v", err)
	}

	existingTag := models.TopicTag{Label: "Existing", Slug: "existing", Category: models.TagCategoryKeyword, Kind: "keyword"}
	if err := database.DB.Create(&existingTag).Error; err != nil {
		t.Fatalf("create existing tag: %v", err)
	}
	if err := database.DB.Create(&models.ArticleTopicTag{ArticleID: preTagged.ID, TopicTagID: existingTag.ID, Score: 1, Source: "manual"}).Error; err != nil {
		t.Fatalf("create existing article tag: %v", err)
	}

	articles := []models.Article{preTagged, untagged}
	if err := BackfillArticleTags(context.Background(), articles, feed.Title, ""); err != nil {
		t.Fatalf("backfill article tags: %v", err)
	}

	var preTaggedLinks []models.ArticleTopicTag
	if err := database.DB.Where("article_id = ?", preTagged.ID).Find(&preTaggedLinks).Error; err != nil {
		t.Fatalf("load pre-tagged links: %v", err)
	}
	if len(preTaggedLinks) != 1 {
		t.Fatalf("pre-tagged link count = %d, want 1", len(preTaggedLinks))
	}

	var untaggedLinks []models.ArticleTopicTag
	if err := database.DB.Where("article_id = ?", untagged.ID).Find(&untaggedLinks).Error; err != nil {
		t.Fatalf("load untagged links: %v", err)
	}
	if len(untaggedLinks) == 0 {
		t.Fatal("expected untagged article to receive backfilled tags")
	}
}

func TestLimitArticleTagsKeepsTopFiveInOrder(t *testing.T) {
	tags := make([]TopicTag, 0, 10)
	for i := 0; i < 10; i++ {
		tags = append(tags, TopicTag{
			Label:    fmt.Sprintf("Tag %d", i),
			Slug:     fmt.Sprintf("tag-%d", i),
			Category: "keyword",
			Score:    float64(10 - i),
		})
	}

	limited := limitArticleTags(tags)

	if len(limited) != 5 {
		t.Fatalf("limited tag count = %d, want 5", len(limited))
	}
	for i, tag := range limited {
		want := fmt.Sprintf("Tag %d", i)
		if tag.Label != want {
			t.Fatalf("tag at index %d = %q, want %q", i, tag.Label, want)
		}
	}
}

func TestTagArticleStoresOnlyTopKeywordTags(t *testing.T) {
	setupTopicExtractionTestDB(t)

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "只负责从新闻摘要中提取 event 和 person") {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"tags\":[]}"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[{\"label\":\"Tag 0\",\"category\":\"keyword\",\"description\":\"测试标签0\"},{\"label\":\"Tag 1\",\"category\":\"keyword\",\"description\":\"测试标签1\"},{\"label\":\"Tag 2\",\"category\":\"keyword\",\"description\":\"测试标签2\"},{\"label\":\"Tag 3\",\"category\":\"keyword\",\"description\":\"测试标签3\"},{\"label\":\"Tag 4\",\"category\":\"keyword\",\"description\":\"测试标签4\"},{\"label\":\"Tag 5\",\"category\":\"keyword\",\"description\":\"测试标签5\"},{\"label\":\"Tag 6\",\"category\":\"keyword\",\"description\":\"测试标签6\"},{\"label\":\"Tag 7\",\"category\":\"keyword\",\"description\":\"测试标签7\"},{\"label\":\"Tag 8\",\"category\":\"keyword\",\"description\":\"测试标签8\"},{\"label\":\"Tag 9\",\"category\":\"keyword\",\"description\":\"测试标签9\"}]"}}]}`))
	}))
	defer aiServer.Close()

	provider := models.AIProvider{Name: "tag-primary", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Model: "test-model", Enabled: true}
	if err := database.DB.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityTopicTagging), Enabled: true, Strategy: "ordered_failover"}
	if err := database.DB.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := database.DB.Create(&models.AIRouteProvider{RouteID: route.ID, ProviderID: provider.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route provider: %v", err)
	}

	feed := models.Feed{Title: "OpenAI Feed", URL: "https://example.com/feed-limit"}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{
		FeedID:      feed.ID,
		Title:       "OpenAI Anthropic Google Meta Microsoft NVIDIA PostgreSQL Kubernetes Redis Docker LangChain",
		Link:        "https://example.com/limit-tags",
		Description: strings.Repeat("OpenAI Anthropic Google Meta Microsoft NVIDIA PostgreSQL Kubernetes Redis Docker LangChain. ", 8),
	}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	if err := TagArticle(context.Background(), &article, feed.Title, "AI"); err != nil {
		t.Fatalf("tag article: %v", err)
	}

	var links []models.ArticleTopicTag
	if err := database.DB.Where("article_id = ?", article.ID).Find(&links).Error; err != nil {
		t.Fatalf("load article tags: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("article tag count = %d, want 3", len(links))
	}

	var savedTags []models.TopicTag
	if err := database.DB.Model(&models.TopicTag{}).
		Joins("JOIN article_topic_tags ON article_topic_tags.topic_tag_id = topic_tags.id").
		Where("article_topic_tags.article_id = ?", article.ID).
		Order("article_topic_tags.id ASC").
		Find(&savedTags).Error; err != nil {
		t.Fatalf("load saved tags: %v", err)
	}
	if len(savedTags) != 3 {
		t.Fatalf("saved tag count = %d, want 3", len(savedTags))
	}
	if savedTags[0].Label != "Tag 0" || savedTags[2].Label != "Tag 2" {
		t.Fatalf("saved tag order = %q ... %q, want Tag 0 ... Tag 2", savedTags[0].Label, savedTags[2].Label)
	}
}
