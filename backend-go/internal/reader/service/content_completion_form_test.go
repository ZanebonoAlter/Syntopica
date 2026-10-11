package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/database"
)

// setupFormMarkFixture provisions the AI provider/route fixtures plus one
// feed/article pair whose summary completes via the router path against a
// fake AI server that replies with the given summary content.
func setupFormMarkFixture(t *testing.T, replyContent string) models.Article {
	t.Helper()

	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"choices":[{"message":{"content":%q}}]}`, replyContent)))
	}))
	t.Cleanup(aiServer.Close)

	seedSummaryRouteFixture(t, aiServer.URL)

	feed := models.Feed{Title: "Feed", URL: fmt.Sprintf("https://example.com/%s", t.Name()), ArticleSummaryEnabled: true, FirecrawlEnabled: true, MaxCompletionRetries: 2}
	if err := database.DB.Create(&feed).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}

	article := models.Article{FeedID: feed.ID, Title: "Form mark", Link: fmt.Sprintf("https://example.com/%s", t.Name()), FirecrawlStatus: "completed", FirecrawlContent: "body", SummaryStatus: "incomplete"}
	if err := database.DB.Create(&article).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}
	return article
}

func TestCompleteArticleStoresAggregateFormMark(t *testing.T) {
	setupServicesTestDB(t)

	article := setupFormMarkFixture(t, "<!-- form: aggregate -->\n# 科技周刊\n\n## 导读\n- 本期内容")

	service := NewContentCompletionService()
	if err := service.CompleteArticleWithMetadata(context.Background(), article.ID, false, nil); err != nil {
		t.Fatalf("complete article: %v", err)
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if refreshed.ContentForm != "aggregate" {
		t.Fatalf("content_form = %q, want aggregate", refreshed.ContentForm)
	}
	wantSummary := "# 科技周刊\n\n## 导读\n- 本期内容"
	if refreshed.AIContentSummary != wantSummary {
		t.Fatalf("ai_content_summary = %q, want %q (mark line must be stripped)", refreshed.AIContentSummary, wantSummary)
	}
}

func TestCompleteArticleStoresMonoFormMark(t *testing.T) {
	setupServicesTestDB(t)

	article := setupFormMarkFixture(t, "<!-- form: mono -->\n# .NET 教程\n\n## 正文整理\n- 单主题内容")

	service := NewContentCompletionService()
	if err := service.CompleteArticleWithMetadata(context.Background(), article.ID, false, nil); err != nil {
		t.Fatalf("complete article: %v", err)
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if refreshed.ContentForm != "mono" {
		t.Fatalf("content_form = %q, want mono", refreshed.ContentForm)
	}
	wantSummary := "# .NET 教程\n\n## 正文整理\n- 单主题内容"
	if refreshed.AIContentSummary != wantSummary {
		t.Fatalf("ai_content_summary = %q, want %q (mark line must be stripped)", refreshed.AIContentSummary, wantSummary)
	}
}

func TestCompleteArticleWithoutFormMarkKeepsSummaryAndEmptyForm(t *testing.T) {
	setupServicesTestDB(t)

	rawSummary := "# 无标记文章\n\n## 导读\n- 摘要原文入库"
	article := setupFormMarkFixture(t, rawSummary)

	service := NewContentCompletionService()
	if err := service.CompleteArticleWithMetadata(context.Background(), article.ID, false, nil); err != nil {
		t.Fatalf("complete article: %v", err)
	}

	var refreshed models.Article
	if err := database.DB.First(&refreshed, article.ID).Error; err != nil {
		t.Fatalf("reload article: %v", err)
	}
	if refreshed.ContentForm != "" {
		t.Fatalf("content_form = %q, want empty", refreshed.ContentForm)
	}
	if refreshed.AIContentSummary != rawSummary {
		t.Fatalf("ai_content_summary = %q, want %q (summary stored unchanged)", refreshed.AIContentSummary, rawSummary)
	}
}
