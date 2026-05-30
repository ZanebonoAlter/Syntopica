package topicgraph

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/database"
)

func setupTopicGraphTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:topic_graph_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Category{}, &models.Feed{}))
	require.NoError(t, db.AutoMigrate(&models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}, &models.TopicTagRelation{}))
	database.DB = db
}

func seedTopicGraphData(t *testing.T) {
	t.Helper()

	category := models.Category{Name: "AI", Slug: "ai", Color: "#3b6b87", Icon: "mdi:brain"}
	require.NoError(t, database.DB.Create(&category).Error)

	feedA := models.Feed{Title: "OpenAI Blog", URL: "https://example.com/openai", CategoryID: &category.ID, Icon: "mdi:robot", Color: "#3b6b87"}
	feedB := models.Feed{Title: "Latent Space", URL: "https://example.com/latent-space", CategoryID: &category.ID, Icon: "mdi:orbit", Color: "#bb6c37"}
	require.NoError(t, database.DB.Create(&feedA).Error)
	require.NoError(t, database.DB.Create(&feedB).Error)

	createdAt := time.Date(2026, 3, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	articles := []models.Article{
		{FeedID: feedA.ID, Title: "Runtime launch", Link: "https://example.com/runtime", CreatedAt: createdAt},
		{FeedID: feedA.ID, Title: "Toolchain memo", Link: "https://example.com/toolchain", CreatedAt: createdAt},
	}
	for i := range articles {
		require.NoError(t, database.DB.Create(&articles[i]).Error)
	}

	topicTags := []models.TopicTag{
		{Label: "AI Agent", Slug: "ai-agent", Category: models.TagCategoryKeyword, Kind: "keyword"},
		{Label: "OpenAI", Slug: "openai", Category: models.TagCategoryKeyword, Kind: "keyword"},
	}
	for i := range topicTags {
		require.NoError(t, database.DB.Create(&topicTags[i]).Error)
	}

	articleTopicTags := []models.ArticleTopicTag{
		{ArticleID: articles[0].ID, TopicTagID: topicTags[0].ID, Score: 1.0, Source: "llm"},
		{ArticleID: articles[0].ID, TopicTagID: topicTags[1].ID, Score: 0.8, Source: "llm"},
		{ArticleID: articles[1].ID, TopicTagID: topicTags[0].ID, Score: 0.9, Source: "llm"},
	}
	for i := range articleTopicTags {
		require.NoError(t, database.DB.Create(&articleTopicTags[i]).Error)
	}
}

func seedTopicArticlesData(t *testing.T) {
	t.Helper()

	topicTags := []models.TopicTag{
		{Label: "AI Agent", Slug: "ai-agent", Category: models.TagCategoryKeyword, Kind: "topic"},
		{Label: "OpenAI", Slug: "openai", Category: models.TagCategoryKeyword, Kind: "topic"},
		{Label: "GPT-5", Slug: "gpt-5", Category: models.TagCategoryKeyword, Kind: "topic"},
	}
	for i := range topicTags {
		require.NoError(t, database.DB.Create(&topicTags[i]).Error)
	}

	createdAt := time.Date(2026, 3, 11, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	articles := []models.Article{
		{Title: "AI Agent Article 1", Link: "https://example.com/ai-agent-1", CreatedAt: createdAt},
		{Title: "AI Agent Article 2", Link: "https://example.com/ai-agent-2", CreatedAt: createdAt.Add(1 * time.Hour)},
		{Title: "AI Agent Article 3", Link: "https://example.com/ai-agent-3", CreatedAt: createdAt.Add(2 * time.Hour)},
		{Title: "OpenAI Article 1", Link: "https://example.com/openai-1", CreatedAt: createdAt.Add(3 * time.Hour)},
		{Title: "GPT-5 Article 1", Link: "https://example.com/gpt5-1", CreatedAt: createdAt.Add(4 * time.Hour)},
	}
	for i := range articles {
		require.NoError(t, database.DB.Create(&articles[i]).Error)
	}

	articleTopicTags := []models.ArticleTopicTag{
		{ArticleID: articles[0].ID, TopicTagID: topicTags[0].ID, Score: 1.0},
		{ArticleID: articles[1].ID, TopicTagID: topicTags[0].ID, Score: 0.9},
		{ArticleID: articles[2].ID, TopicTagID: topicTags[0].ID, Score: 0.8},
		{ArticleID: articles[3].ID, TopicTagID: topicTags[1].ID, Score: 1.0},
		{ArticleID: articles[4].ID, TopicTagID: topicTags[2].ID, Score: 1.0},
	}
	for i := range articleTopicTags {
		require.NoError(t, database.DB.Create(&articleTopicTags[i]).Error)
	}
}

func TestGetTopicGraphReturnsNodesAndEdges(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicGraphData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "type", Value: "daily"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/daily?date=2026-03-11", http.NoBody)

	GetTopicGraph(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Nodes []map[string]any `json:"nodes"`
			Edges []map[string]any `json:"edges"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.NotEmpty(t, body.Data.Nodes)
	require.NotEmpty(t, body.Data.Edges)
	require.Contains(t, collectNodeLabels(body.Data.Nodes), "AI Agent")
	require.Contains(t, collectNodeLabels(body.Data.Nodes), "OpenAI")

	var persistedCount int64
	require.NoError(t, database.DB.Model(&models.TopicTag{}).Count(&persistedCount).Error)
	require.Equal(t, int64(2), persistedCount)
}

func TestGetTopicDetailReturnsHistoryAndArticles(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicGraphData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "ai-agent"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/ai-agent?type=daily&date=2026-03-11", http.NoBody)

	GetTopicDetail(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Topic struct {
				Slug  string `json:"slug"`
				Label string `json:"label"`
			} `json:"topic"`
			Articles    []map[string]any  `json:"articles"`
			History     []map[string]any  `json:"history"`
			SearchLinks map[string]string `json:"search_links"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, "ai-agent", body.Data.Topic.Slug)
	require.Equal(t, "AI Agent", body.Data.Topic.Label)
	require.NotEmpty(t, body.Data.Articles)
	require.NotEmpty(t, body.Data.History)
	firstArticle := body.Data.Articles[0]
	require.NotEmpty(t, firstArticle["title"])
	searchLinks, ok := body.Data.SearchLinks["youtube_live"]
	require.True(t, ok)
	require.NotEmpty(t, searchLinks)
}

func TestGetDigestsByArticleTagReturnsArticles(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicGraphData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "ai-agent"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/tag/ai-agent/digests?type=daily&date=2026-03-11", http.NoBody)

	GetDigestsByArticleTagHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Digests []map[string]any `json:"digests"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.NotEmpty(t, body.Data.Digests)

	firstDigest := body.Data.Digests[0]
	require.NotEmpty(t, firstDigest["title"])
	require.NotEmpty(t, firstDigest["feed_name"])
}

func TestGetTopicArticlesSuccess(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicArticlesData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "ai-agent"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/ai-agent/articles?page=1&page_size=10&type=daily&date=2026-03-11", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Articles []map[string]any `json:"articles"`
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.NotEmpty(t, body.Data.Articles)
	require.Equal(t, int64(3), body.Data.Total)
	require.Equal(t, 1, body.Data.Page)
	require.Equal(t, 10, body.Data.PageSize)

	firstArticle := body.Data.Articles[0]
	require.NotEmpty(t, firstArticle["id"])
	require.NotEmpty(t, firstArticle["title"])
}

func TestGetTopicArticlesPagination(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicArticlesData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "ai-agent"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/ai-agent/articles?page=1&page_size=2&type=daily&date=2026-03-11", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Articles []map[string]any `json:"articles"`
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Len(t, body.Data.Articles, 2)
	require.Equal(t, int64(3), body.Data.Total)
	require.Equal(t, 1, body.Data.Page)
	require.Equal(t, 2, body.Data.PageSize)
}

func TestGetTopicArticlesMissingSlug(t *testing.T) {
	setupTopicGraphTestDB(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: ""}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic//articles", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusBadRequest, recorder.Code)

	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.False(t, body.Success)
	require.NotEmpty(t, body.Error)
}

func TestGetTopicArticlesInvalidPage(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicArticlesData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "ai-agent"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/ai-agent/articles?page=-1&page_size=10&type=daily&date=2026-03-11", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Page int `json:"page"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, 1, body.Data.Page)
}

func TestGetTopicArticlesInvalidPageSize(t *testing.T) {
	setupTopicGraphTestDB(t)
	seedTopicArticlesData(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "ai-agent"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/ai-agent/articles?page=1&page_size=200&type=daily&date=2026-03-11", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			PageSize int `json:"page_size"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, 100, body.Data.PageSize)
}

func TestGetTopicArticlesTopicNotFound(t *testing.T) {
	setupTopicGraphTestDB(t)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "non-existent-topic"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/non-existent-topic/articles?page=1&page_size=10&type=daily&date=2026-03-11", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.False(t, body.Success)
	require.Contains(t, body.Error, "topic not found")
}

func TestGetTopicArticlesEmptyResult(t *testing.T) {
	setupTopicGraphTestDB(t)
	topicTag := models.TopicTag{Label: "Empty Topic", Slug: "empty-topic", Category: models.TagCategoryKeyword, Kind: "topic"}
	require.NoError(t, database.DB.Create(&topicTag).Error)
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "slug", Value: "empty-topic"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/topic-graph/topic/empty-topic/articles?page=1&page_size=10&type=daily&date=2026-03-11", http.NoBody)

	GetTopicArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Articles []map[string]any `json:"articles"`
			Total    int64            `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Empty(t, body.Data.Articles)
	require.Equal(t, int64(0), body.Data.Total)
}

func collectNodeLabels(nodes []map[string]any) []string {
	labels := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if label, ok := node["label"].(string); ok {
			labels = append(labels, label)
		}
	}
	return labels
}
