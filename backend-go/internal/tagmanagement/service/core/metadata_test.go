package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/testutil"
	"syntopica-backend/internal/tagmanagement/repository"
)

func setupTopicExtractionTestDB(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repository.InitRepository(db)
	database.DB = db // needed by airouter.NewStore() via NewTagExtractor()

	// Set up bridge factories (normally set by sub-package init(), but can't import due to cycles)
	AuxServiceFactory = func(db *gorm.DB, embedder interface{}) AuxService {
		return &noopAuxService{}
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

	vendor := models.AIVendor{Name: "tag-vendor", ProviderType: airouter.ProviderTypeOpenAICompatible, BaseURL: aiServer.URL, APIKey: "token", Enabled: true}
	if err := repository.Repo.DB().Create(&vendor).Error; err != nil {
		t.Fatalf("create vendor: %v", err)
	}
	model := models.AIModel{VendorID: vendor.ID, Name: "test-model", ModelKind: airouter.ModelKindLLM, Enabled: true, Status: airouter.ModelStatusOK}
	if err := repository.Repo.DB().Create(&model).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
	route := models.AIRoute{Name: airouter.DefaultRouteName, Capability: string(airouter.CapabilityTopicTagging), Enabled: true}
	if err := repository.Repo.DB().Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	if err := repository.Repo.DB().Create(&models.AIRouteModel{RouteID: route.ID, ModelID: model.ID, Priority: 1, Enabled: true}).Error; err != nil {
		t.Fatalf("create route model: %v", err)
	}

	feed := models.Feed{Title: "OpenAI Feed", URL: "https://example.com/feed-limit"}
	if err := repository.Repo.DB().Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{
		FeedID:      feed.ID,
		Title:       "OpenAI Anthropic Google Meta Microsoft NVIDIA PostgreSQL Kubernetes Redis Docker LangChain",
		Link:        "https://example.com/limit-tags",
		Description: strings.Repeat("OpenAI Anthropic Google Meta Microsoft NVIDIA PostgreSQL Kubernetes Redis Docker LangChain. ", 8),
	}
	if err := repository.Repo.DB().Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	if err := TagArticle(context.Background(), &article, feed.Title, "AI"); err != nil {
		t.Fatalf("tag article: %v", err)
	}

	var links []models.ArticleTopicTag
	if err := repository.Repo.DB().Where("article_id = ?", article.ID).Find(&links).Error; err != nil {
		t.Fatalf("load article tags: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("article tag count = %d, want 3", len(links))
	}

	var savedTags []models.TopicTag
	if err := repository.Repo.DB().Model(&models.TopicTag{}).
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

type noopAuxService struct{}

func (n *noopAuxService) AttachAuxiliaryLabels(_ context.Context, _ uint, _ []AuxiliaryLabel) error {
	return nil
}

func (n *noopAuxService) RecountRefs(_ context.Context, _ []uint) error {
	return nil
}
