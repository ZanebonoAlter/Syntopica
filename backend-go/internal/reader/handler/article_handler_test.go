package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
	"syntopica-backend/internal/platform/logging"
	"syntopica-backend/internal/reader/repository"
	tagging "syntopica-backend/internal/tagmanagement"
	tagmodels "syntopica-backend/internal/tagmanagement/models"
)

func setupArticlesHandlerTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:articles_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Category{}, &models.Feed{}, &models.Article{}, &models.TopicTag{}, &models.ArticleTopicTag{}))
	database.DB = db
	repository.InitRepository(database.DB)
	tagging.InitRepository(database.DB)
}

func TestGetArticleReturnsArticleTags(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	category := models.Category{Name: "AI", Slug: "ai", Color: "#3b6b87", Icon: "mdi:brain"}
	require.NoError(t, database.DB.Create(&category).Error)

	feed := models.Feed{Title: "OpenAI Blog", URL: "https://example.com/openai", CategoryID: &category.ID}
	require.NoError(t, database.DB.Create(&feed).Error)

	article := models.Article{
		FeedID:    feed.ID,
		Title:     "GPT-5 agent runtime",
		Link:      "https://example.com/gpt5-agent-runtime",
		CreatedAt: time.Date(2026, 3, 22, 9, 0, 0, 0, models.ShanghaiTZ),
	}
	require.NoError(t, database.DB.Create(&article).Error)

	topicTag := models.TopicTag{Label: "AI Agent", Slug: "ai-agent", Category: models.TagCategoryKeyword, Kind: "keyword", Icon: "mdi:robot"}
	require.NoError(t, database.DB.Create(&topicTag).Error)
	require.NoError(t, database.DB.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: topicTag.ID, Score: 0.92, Source: "llm"}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "article_id", Value: fmt.Sprintf("%d", article.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/articles/%d", article.ID), http.NoBody)

	GetArticle(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			ID   uint `json:"id"`
			Tags []struct {
				Slug     string  `json:"slug"`
				Label    string  `json:"label"`
				Category string  `json:"category"`
				Score    float64 `json:"score"`
				Icon     string  `json:"icon"`
			} `json:"tags"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, article.ID, body.Data.ID)
	require.Len(t, body.Data.Tags, 1)
	require.Equal(t, "ai-agent", body.Data.Tags[0].Slug)
	require.Equal(t, "AI Agent", body.Data.Tags[0].Label)
	require.Equal(t, models.TagCategoryKeyword, body.Data.Tags[0].Category)
	require.Equal(t, 0.92, body.Data.Tags[0].Score)
	require.Equal(t, "mdi:robot", body.Data.Tags[0].Icon)
}

func TestGetArticlesReturnsTagCount(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "OpenAI Blog", URL: "https://example.com/openai"}
	require.NoError(t, database.DB.Create(&feed).Error)

	article := models.Article{
		FeedID:    feed.ID,
		Title:     "Runtime launch",
		Link:      "https://example.com/runtime",
		CreatedAt: time.Now(),
	}
	require.NoError(t, database.DB.Create(&article).Error)

	tagA := models.TopicTag{Label: "AI Agent", Slug: "ai-agent", Category: models.TagCategoryKeyword, Kind: "keyword"}
	tagB := models.TopicTag{Label: "OpenAI", Slug: "openai", Category: models.TagCategoryKeyword, Kind: "keyword"}
	require.NoError(t, database.DB.Create(&tagA).Error)
	require.NoError(t, database.DB.Create(&tagB).Error)
	require.NoError(t, database.DB.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tagA.ID, Score: 1, Source: "llm"}).Error)
	require.NoError(t, database.DB.Create(&models.ArticleTopicTag{ArticleID: article.ID, TopicTagID: tagB.ID, Score: 0.8, Source: "llm"}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/articles", http.NoBody)

	GetArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID       uint `json:"id"`
			TagCount int  `json:"tag_count"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.NotEmpty(t, body.Data)
	require.Equal(t, article.ID, body.Data[0].ID)
	require.Equal(t, 2, body.Data[0].TagCount)
}

func TestRetagArticleReturnsUpdatedTags(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "OpenAI Feed", URL: "https://example.com/openai"}
	require.NoError(t, database.DB.Create(&feed).Error)

	article := models.Article{
		FeedID:           feed.ID,
		Title:            "Daily brief",
		Link:             "https://example.com/daily-brief",
		Description:      "Old short description",
		AIContentSummary: "OpenAI launched a new AI agent workflow.",
		CreatedAt:        time.Now(),
	}
	require.NoError(t, database.DB.Create(&article).Error)

	require.NoError(t, database.DB.AutoMigrate(&models.TagJob{}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "article_id", Value: fmt.Sprintf("%d", article.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/articles/%d/tags", article.ID), http.NoBody)

	RetagArticleHandler(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			JobID     uint   `json:"job_id"`
			ArticleID uint   `json:"article_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.NotZero(t, body.Data.JobID)
	require.Equal(t, article.ID, body.Data.ArticleID)
	require.Equal(t, "pending", body.Data.Status)

	var job models.TagJob
	require.NoError(t, database.DB.First(&job, body.Data.JobID).Error)
	require.Equal(t, article.ID, job.ArticleID)
	require.True(t, job.ForceRetag)
}

func TestRetagArticleWithExistingLeasedJob(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "Leased Feed", URL: "https://example.com/leased"}
	require.NoError(t, database.DB.Create(&feed).Error)

	article := models.Article{
		FeedID:           feed.ID,
		Title:            "Leased article",
		Link:             "https://example.com/leased-article",
		AIContentSummary: "Summary for leased test.",
		CreatedAt:        time.Now(),
	}
	require.NoError(t, database.DB.Create(&article).Error)
	require.NoError(t, database.DB.AutoMigrate(&models.TagJob{}))

	// Simulate an existing leased job — worker has already claimed it.
	now := time.Now()
	leasedJob := models.TagJob{
		ArticleID:            article.ID,
		Status:               string(models.JobStatusLeased),
		Priority:             0,
		AttemptCount:         1,
		MaxAttempts:          5,
		AvailableAt:          now,
		LeasedAt:             &now,
		LeaseExpiresAt:       nil,
		FeedNameSnapshot:     feed.Title,
		CategoryNameSnapshot: "",
		ForceRetag:           false,
		Reason:               "auto",
	}
	require.NoError(t, database.DB.Create(&leasedJob).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "article_id", Value: fmt.Sprintf("%d", article.ID)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/articles/%d/tags", article.ID), http.NoBody)

	RetagArticleHandler(ctx)

	// Should succeed (200) and return the existing leased job, not a 500.
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			JobID     uint   `json:"job_id"`
			ArticleID uint   `json:"article_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, leasedJob.ID, body.Data.JobID)
	require.Equal(t, article.ID, body.Data.ArticleID)
	require.Equal(t, string(models.JobStatusLeased), body.Data.Status)

	// Verify the existing job was updated with ForceRetag=true.
	var updated models.TagJob
	require.NoError(t, database.DB.First(&updated, leasedJob.ID).Error)
	require.True(t, updated.ForceRetag)
}

func TestGetArticles_Projection(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	require.NoError(t, database.DB.AutoMigrate(&models.TopicTagRelation{}, &tagmodels.TopicTagBoardLabel{}, &tagmodels.TopicTagSemanticLabel{}))
	gin.SetMode(gin.TestMode)

	category := models.Category{Name: "Projection", Slug: "projection", Color: "#3b6b87", Icon: "mdi:brain"}
	require.NoError(t, database.DB.Create(&category).Error)

	feed := models.Feed{Title: "Projection Feed", URL: "https://example.com/projection", CategoryID: &category.ID}
	require.NoError(t, database.DB.Create(&feed).Error)

	bigHTML := "<p>" + strings.Repeat("这是正文内容 ", 500) + "</p>"
	activeArticle := models.Article{
		FeedID:           feed.ID,
		Title:            "Active article",
		Link:             "https://example.com/projection/active",
		Description:      "<p>Lede for active article</p>",
		Content:          bigHTML,
		FirecrawlContent: bigHTML,
		AIContentSummary: bigHTML,
		PubDate:          ptrTime2(time.Date(2026, 3, 22, 9, 0, 0, 0, models.ShanghaiTZ)),
		CreatedAt:        time.Now(),
	}
	require.NoError(t, database.DB.Create(&activeArticle).Error)

	archivedArticle := models.Article{
		FeedID:      feed.ID,
		Title:       "Archived article",
		Link:        "https://example.com/projection/archived",
		Description: "<p>Lede for archived article</p>",
		Content:     bigHTML,
		Archived:    true,
		PubDate:     ptrTime2(time.Date(2026, 3, 21, 9, 0, 0, 0, models.ShanghaiTZ)),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, database.DB.Create(&archivedArticle).Error)

	tag := models.TopicTag{Label: "AI Agent", Slug: "ai-agent-projection", Category: models.TagCategoryKeyword, Kind: "keyword"}
	require.NoError(t, database.DB.Create(&tag).Error)
	require.NoError(t, database.DB.Create(&models.ArticleTopicTag{ArticleID: activeArticle.ID, TopicTagID: tag.ID, Score: 1, Source: "llm"}).Error)
	require.NoError(t, database.DB.Create(&models.ArticleTopicTag{ArticleID: archivedArticle.ID, TopicTagID: tag.ID, Score: 1, Source: "llm"}).Error)

	boardID := uint(7)
	auxLabelID := uint(9)
	require.NoError(t, database.DB.Create(&tagmodels.TopicTagBoardLabel{TopicTagID: tag.ID, SemanticBoardID: boardID}).Error)
	require.NoError(t, database.DB.Create(&tagmodels.TopicTagSemanticLabel{TopicTagID: tag.ID, SemanticLabelID: auxLabelID}).Error)

	expectedKeys := map[string]struct{}{
		"id": {}, "feed_id": {}, "category_id": {}, "title": {}, "link": {}, "image_url": {},
		"pub_date": {}, "author": {}, "read": {}, "favorite": {}, "archived": {},
		"summary_status": {}, "summary_generated_at": {}, "firecrawl_status": {},
		"firecrawl_error": {}, "firecrawl_crawled_at": {}, "completion_error": {},
		"created_at": {}, "tag_count": {}, "relevance_score": {}, "excerpt": {},
	}
	forbiddenKeys := []string{"content", "description", "firecrawl_content", "ai_content_summary"}

	cases := []struct {
		name  string
		query string
	}{
		{"no filter", ""},
		{"feed filter", fmt.Sprintf("feed_id=%d", feed.ID)},
		{"category filter", fmt.Sprintf("category_id=%d", category.ID)},
		{"watched relevance", fmt.Sprintf("watched_tag_ids=%d&sort_by=relevance", tag.ID)},
		{"watched date", fmt.Sprintf("watched_tag_ids=%d", tag.ID)},
		{"concept filter", fmt.Sprintf("concept_id=%d", boardID)},
		{"auxiliary filter", fmt.Sprintf("auxiliary_label_id=%d", auxLabelID)},
		{"archived view", "archived=true"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := "/api/articles"
			if tc.query != "" {
				target += "?" + tc.query
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, target, http.NoBody)

			GetArticles(ctx)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

			var body struct {
				Success bool                     `json:"success"`
				Data    []map[string]interface{} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.True(t, body.Success)
			require.NotEmpty(t, body.Data, "branch %q should return rows", tc.query)

			for i, item := range body.Data {
				actualKeys := make(map[string]struct{}, len(item))
				for k := range item {
					actualKeys[k] = struct{}{}
				}
				assert.Equal(t, expectedKeys, actualKeys, "item %d key set mismatch", i)
				for _, forbidden := range forbiddenKeys {
					_, ok := item[forbidden]
					assert.False(t, ok, "item %d must not carry %q", i, forbidden)
				}
			}

			switch tc.name {
			case "no filter":
				assert.Equal(t, "Lede for active article", body.Data[0]["excerpt"])
			case "archived view":
				assert.Equal(t, "Lede for archived article", body.Data[0]["excerpt"])
			case "watched relevance":
				_, ok := body.Data[0]["relevance_score"]
				assert.True(t, ok, "relevance branch must expose relevance_score")
			}
		})
	}
}

func TestGetArticles_ExcerptDedupeSuppression(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "Dedupe Feed", URL: "https://example.com/dedupe"}
	require.NoError(t, database.DB.Create(&feed).Error)

	bodyHTML := "<p>" + strings.Repeat("重复正文段落 ", 30) + "</p>"
	repeatedArticle := models.Article{
		FeedID:      feed.ID,
		Title:       "Description repeats content",
		Link:        "https://example.com/dedupe/repeated",
		Description: bodyHTML,
		Content:     bodyHTML,
		PubDate:     ptrTime2(time.Date(2026, 3, 22, 9, 0, 0, 0, models.ShanghaiTZ)),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, database.DB.Create(&repeatedArticle).Error)

	distinctArticle := models.Article{
		FeedID:      feed.ID,
		Title:       "Distinct description",
		Link:        "https://example.com/dedupe/distinct",
		Description: "<p>A genuinely different lede for the distinct article</p>",
		Content:     bodyHTML,
		PubDate:     ptrTime2(time.Date(2026, 3, 21, 9, 0, 0, 0, models.ShanghaiTZ)),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, database.DB.Create(&distinctArticle).Error)

	shortPostHTML := "<p>如题，是不是因为我的邮箱注册过 facebook ？</p>"
	shortFallbackArticle := models.Article{
		FeedID:      feed.ID,
		Title:       "Empty description, short content post",
		Link:        "https://example.com/dedupe/short-fallback",
		Description: "",
		Content:     shortPostHTML,
		PubDate:     ptrTime2(time.Date(2026, 3, 20, 9, 0, 0, 0, models.ShanghaiTZ)),
		CreatedAt:   time.Now(),
	}
	require.NoError(t, database.DB.Create(&shortFallbackArticle).Error)

	firecrawlBackedArticle := models.Article{
		FeedID:           feed.ID,
		Title:            "Empty description with Firecrawl body",
		Link:             "https://example.com/dedupe/short-fallback-firecrawl",
		Description:      "",
		Content:          shortPostHTML,
		FirecrawlContent: "<p>" + strings.Repeat("Firecrawl 正文 ", 20) + "</p>",
		PubDate:          ptrTime2(time.Date(2026, 3, 19, 9, 0, 0, 0, models.ShanghaiTZ)),
		CreatedAt:        time.Now(),
	}
	require.NoError(t, database.DB.Create(&firecrawlBackedArticle).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/articles", http.NoBody)

	GetArticles(ctx)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var body struct {
		Success bool `json:"success"`
		Data    []struct {
			ID      uint   `json:"id"`
			Excerpt string `json:"excerpt"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Success)

	excerpts := make(map[uint]string, len(body.Data))
	for _, item := range body.Data {
		excerpts[item.ID] = item.Excerpt
	}

	require.Contains(t, excerpts, repeatedArticle.ID)
	require.Contains(t, excerpts, distinctArticle.ID)
	require.Contains(t, excerpts, shortFallbackArticle.ID)
	require.Contains(t, excerpts, firecrawlBackedArticle.ID)
	require.Equal(t, "", excerpts[repeatedArticle.ID], "lede repeating the body must be suppressed")
	require.Equal(t, "A genuinely different lede for the distinct article", excerpts[distinctArticle.ID])
	require.Equal(t, "", excerpts[shortFallbackArticle.ID], "content-fallback lede without a Firecrawl body must be suppressed (short post)")
	require.Equal(t, "如题，是不是因为我的邮箱注册过 facebook ？", excerpts[firecrawlBackedArticle.ID], "Firecrawl body keeps the content-fallback lede")
}

func TestExcerpt(t *testing.T) {
	longHTML := "<div><p>" + strings.Repeat("<b>深度阅读内容</b>", 3000) + "</p></div>"
	require.Greater(t, len(longHTML), 50000)

	substringLede := strings.Repeat("导语片段", 10) // 40 runes after cleaning
	substringBody := "<p>" + substringLede + strings.Repeat("正文继续", 20) + "</p>"
	distinctLede := strings.Repeat("导语", 25) // 50 runes
	distinctBody := strings.Repeat("正文", 25)

	cases := []struct {
		name            string
		description     string
		content         string
		noFirecrawlBody bool
		want            string
	}{
		{"empty source", "", "", false, ""},
		{"whitespace only", "  \t\n ", "", false, ""},
		{"full-width spaces only", "　　", "", false, ""},
		{"empty tags only", "<p></p>", "", false, ""},
		{"image only", "<img src='x'>", "", false, ""},
		{"symbols only", "，，，。", "", false, ""},
		{"plain text", "abc", "", false, "abc"},
		{"inline tags stripped", "<p>Hello <b>world</b></p>", "", false, "Hello world"},
		{"entities decoded", "a&amp;b&lt;c", "", false, "a&b<c"},
		{"199 runes kept", strings.Repeat("a", 199), "", false, strings.Repeat("a", 199)},
		{"200 runes kept", strings.Repeat("a", 200), "", false, strings.Repeat("a", 200)},
		{"201 runes truncated", strings.Repeat("a", 201), "", false, strings.Repeat("a", 200)},
		{"script block removed", "<script>alert(1)</script>text", "", false, "text"},
		{"style block removed", "<style>p{color:red}</style>ok", "", false, "ok"},
		// Fallback ledes: with a Firecrawl body the displayed text may differ,
		// so the lede survives; without one the reading page displays `content`
		// and the guard hides the lede regardless of length.
		{"falls back to content with firecrawl body", "", "<p>from content</p>", false, "from content"},
		{"falls back when description has no text, no firecrawl body", "<p></p>", "<p>from content</p>", true, ""},
		{"empty description falls back to long content, no firecrawl body", "", strings.Repeat("正文内容", 20), true, ""},
		{"empty description short post, no firecrawl body", "", "<p>如题，是不是因为我的邮箱注册过 facebook ？</p>", true, ""},
		{"empty description short post, with firecrawl body", "", "<p>如题，是不是因为我的邮箱注册过 facebook ？</p>", false, "如题，是不是因为我的邮箱注册过 facebook ？"},
		{"description equals content (long html)", longHTML, longHTML, false, ""},
		{"description equals content short", "abc", "abc", false, ""},
		{"same text different markup", "<p>abc</p>", "abc", false, ""},
		{"description substring of content", substringLede, substringBody, false, ""},
		{"distinct long lede kept", distinctLede, distinctBody, false, distinctLede},
		{"short distinct lede kept", "短导语", longHTML, false, "短导语"},
		{"short lede contained by content kept", "短导语", "<p>短导语" + strings.Repeat("正文", 20) + "</p>", false, "短导语"},
		{"long lede kept when content empty", distinctLede, "", false, distinctLede},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, buildExcerpt(tc.description, tc.content, tc.noFirecrawlBody))
		})
	}

	t.Run("long html truncated and tag free", func(t *testing.T) {
		got := buildExcerpt(longHTML, "", false)
		require.LessOrEqual(t, len([]rune(got)), 200)
		require.NotContains(t, got, "<")
		require.NotContains(t, got, ">")
	})
}

func TestPerPageOverLimit(t *testing.T) {
	setupArticlesHandlerTestDB(t)
	gin.SetMode(gin.TestMode)

	feed := models.Feed{Title: "Pagination Feed", URL: "https://example.com/pagination"}
	require.NoError(t, database.DB.Create(&feed).Error)

	articles := make([]models.Article, 0, 105)
	for i := 0; i < 105; i++ {
		articles = append(articles, models.Article{
			FeedID:    feed.ID,
			Title:     fmt.Sprintf("Pagination article %d", i),
			Link:      fmt.Sprintf("https://example.com/pagination/%d", i),
			PubDate:   ptrTime2(time.Now().Add(-time.Duration(i) * time.Minute)),
			CreatedAt: time.Now(),
		})
	}
	require.NoError(t, database.DB.Create(&articles).Error)

	var buf bytes.Buffer
	logging.SetWriters(&buf, &buf)
	defer logging.ResetWriters()

	cases := []struct {
		name        string
		query       string
		wantPerPage int
		wantRows    int
		wantWarn    string
	}{
		{"per_page=10000 clamped", "per_page=10000", 100, 100, "per_page=10000"},
		{"per_page=101 clamped", "per_page=101", 100, 100, "per_page=101"},
		{"per_page=100 boundary", "per_page=100", 100, 100, ""},
		{"per_page=20 default", "per_page=20", 20, 20, ""},
		{"per_page=0 falls back to default", "per_page=0", 20, 20, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/articles?"+tc.query, http.NoBody)

			GetArticles(ctx)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

			var body struct {
				Success    bool                     `json:"success"`
				Data       []map[string]interface{} `json:"data"`
				Pagination struct {
					PerPage int `json:"per_page"`
				} `json:"pagination"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
			require.True(t, body.Success)
			require.Equal(t, tc.wantPerPage, body.Pagination.PerPage)
			require.Len(t, body.Data, tc.wantRows)

			warnCount := strings.Count(buf.String(), "level=WARN")
			if tc.wantWarn == "" {
				require.Equal(t, 0, warnCount, "unexpected WARN: %s", buf.String())
			} else {
				require.Equal(t, 1, warnCount, "expected exactly one WARN: %s", buf.String())
				require.Contains(t, buf.String(), tc.wantWarn)
			}
		})
	}
}

func ptrTime2(t time.Time) *time.Time { return &t }
