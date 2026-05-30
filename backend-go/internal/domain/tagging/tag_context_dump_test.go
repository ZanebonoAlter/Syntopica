package tagging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/domain/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/config"
	"syntopica-backend/internal/platform/database"
)

func TestDumpTagContextForArticle74426(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real DB test in short mode")
	}

	if err := config.LoadConfig("./../../../configs"); err != nil {
		t.Logf("config load warning: %v", err)
	}
	if err := database.InitDB(config.AppConfig); err != nil {
		t.Fatalf("failed to connect DB: %v", err)
	}

	var article models.Article
	if err := database.DB.First(&article, 74551).Error; err != nil {
		t.Fatalf("article 74551 not found: %v", err)
	}

	var feed models.Feed
	if err := database.DB.Preload("Category").First(&feed, article.FeedID).Error; err != nil {
		t.Fatalf("feed %d not found: %v", article.FeedID, err)
	}

	var existingTags []struct {
		TagID    uint
		Label    string
		Category string
		Score    float64
		Source   string
	}
	database.DB.Raw(`
		SELECT att.topic_tag_id, tt.label, tt.category, att.score, att.source
		FROM article_topic_tags att
		JOIN topic_tags tt ON tt.id = att.topic_tag_id
		WHERE att.article_id = ?
		ORDER BY att.score DESC
	`, 74551).Scan(&existingTags)

	feedName := feed.Title
	categoryName := FeedCategoryName(feed)
	summary := buildArticleSummary(article)

	input := ExtractionInput{
		Title:        article.Title,
		Summary:      summary,
		FeedName:     feedName,
		CategoryName: categoryName,
		ArticleID:    &article.ID,
	}

	articleContext := ""
	if article.Title != "" {
		articleContext = article.Title
	}
	articleSummaryForCtx := buildArticleSummary(article)
	if articleSummaryForCtx != "" {
		if articleContext != "" {
			articleContext += ". "
		}
		runes := []rune(articleSummaryForCtx)
		if len(runes) > 800 {
			articleSummaryForCtx = string(runes[:800])
		}
		articleContext += articleSummaryForCtx
	}

	userPrompt := fmt.Sprintf(`请从以下新闻摘要中提取标签：

标题: %s
来源: %s
分类: %s

摘要内容:
%s

请返回JSON数组格式的标签列表。`, input.Title, input.FeedName, input.CategoryName, input.Summary)

	sep := strings.Repeat("=", 80)
	line := strings.Repeat("-", 78)

	var sb strings.Builder
	sb.WriteString(sep + "\n")
	sb.WriteString(" 打 Tag 上下文完整 Dump — Article 74551 (真实数据库)\n")
	sb.WriteString(sep + "\n\n")

	sb.WriteString("## 1. Article 记录\n\n")
	fmt.Fprintf(&sb, "  ID:                    %d\n", article.ID)
	fmt.Fprintf(&sb, "  FeedID:                %d\n", article.FeedID)
	fmt.Fprintf(&sb, "  Title:                 %s\n", article.Title)
	fmt.Fprintf(&sb, "  Author:                %s\n", article.Author)
	fmt.Fprintf(&sb, "  Link:                  %s\n", article.Link)
	fmt.Fprintf(&sb, "  SummaryStatus:         %s\n", article.SummaryStatus)
	fmt.Fprintf(&sb, "  FirecrawlStatus:       %s\n", article.FirecrawlStatus)
	fmt.Fprintf(&sb, "  Description 长度:      %d\n", len(article.Description))
	fmt.Fprintf(&sb, "  Content 长度:          %d\n", len(article.Content))
	fmt.Fprintf(&sb, "  AIContentSummary 长度: %d\n", len(article.AIContentSummary))
	fmt.Fprintf(&sb, "  FirecrawlContent 长度: %d\n", len(article.FirecrawlContent))
	fmt.Fprintf(&sb, "\n  Description 内容:\n  %s\n  %s\n  %s\n",
		line, padLines(article.Description, 2), line)
	fmt.Fprintf(&sb, "\n  Content 内容 (前500字符):\n  %s\n  %s\n  %s\n",
		line, truncateAndPad(article.Content, 500, 2), line)

	sb.WriteString("\n## 2. Feed / Category\n\n")
	fmt.Fprintf(&sb, "  Feed ID:         %d\n", feed.ID)
	fmt.Fprintf(&sb, "  Feed Title:      %s\n", feed.Title)
	fmt.Fprintf(&sb, "  Feed CategoryID: %v\n", feed.CategoryID)
	if feed.Category != nil {
		fmt.Fprintf(&sb, "  Category.Name:   %s\n", feed.Category.Name)
		fmt.Fprintf(&sb, "  Category.Slug:   %s\n", feed.Category.Slug)
	} else {
		sb.WriteString("  Category:        (nil — feed 未关联分类)\n")
	}

	sb.WriteString("\n## 3. Handler → Enqueue (TagJobRequest)\n\n")
	fmt.Fprintf(&sb, "  ArticleID:    %d\n", article.ID)
	fmt.Fprintf(&sb, "  FeedName:     %s\n", feedName)
	fmt.Fprintf(&sb, "  CategoryName: %q\n", categoryName)
	fmt.Fprintf(&sb, "  ForceRetag:   true\n")
	fmt.Fprintf(&sb, "  Reason:       manual_api_trigger\n")
	sb.WriteString("\n## 4. buildArticleSummary\n\n")
	sb.WriteString("  优先级:\n")
	sb.WriteString("    1. AIContentSummary（如有）\n")
	sb.WriteString("    2. FirecrawlContent（如 1 为空）\n")
	sb.WriteString("    3. Content（如 1、2 为空）\n")
	sb.WriteString("    4. Description（如 1、2、3 为空）\n")
	fmt.Fprintf(&sb, "\n  实际命中: AIContentSummary\n")
	fmt.Fprintf(&sb, "  Summary 长度: %d 字节 / %d rune\n", len(summary), len([]rune(summary)))
	sb.WriteString("\n## 5. ExtractionInput (传给 ExtractTags)\n\n")
	fmt.Fprintf(&sb, "  Title:        %s\n", input.Title)
	fmt.Fprintf(&sb, "  FeedName:     %s\n", input.FeedName)
	fmt.Fprintf(&sb, "  CategoryName: %q\n", input.CategoryName)
	fmt.Fprintf(&sb, "  ArticleID:    %d\n", *input.ArticleID)
	fmt.Fprintf(&sb, "  Summary 长度: %d\n", len(input.Summary))
	sb.WriteString("\n## 6. 最终发给 LLM 的 User Prompt\n\n")
	sb.WriteString("  " + line + "\n")
	for _, l := range strings.Split(userPrompt, "\n") {
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("  " + line + "\n")

	sb.WriteString("\n## 7. Event/Person System Prompt\n\n")
	sb.WriteString("  " + line + "\n")
	for _, l := range strings.Split(buildEventPersonPrompt(), "\n") {
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("  " + line + "\n")

	sb.WriteString("\n## 7b. Keyword System Prompt\n\n")
	sb.WriteString("  " + line + "\n")
	for _, l := range strings.Split(buildKeywordPrompt(), "\n") {
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("  " + line + "\n")

	fmt.Fprintf(&sb, "\n## 8. articleContext (传给 findOrCreateTag, 截断上限 800 rune)\n\n")
	fmt.Fprintf(&sb, "  长度: %d 字符 / %d rune\n", len(articleContext), len([]rune(articleContext)))
	sb.WriteString("  " + line + "\n")
	for _, l := range strings.Split(truncateByRune(articleContext, 1000), "\n") {
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("  " + line + "\n")

	sb.WriteString("\n## 9. 当前已有标签\n\n")
	if len(existingTags) == 0 {
		sb.WriteString("  (无标签)\n")
	} else {
		sb.WriteString("  TagID   | Label        | Category | Score | Source\n")
		sb.WriteString("  " + line + "\n")
		for _, tag := range existingTags {
			sb.WriteString(fmt.Sprintf("  %-7d | %-12s | %-8s | %5.1f | %s\n",
				tag.TagID, tag.Label, tag.Category, tag.Score, tag.Source))
		}
	}

	sb.WriteString("\n## 10. 实时调用 AI 提取\n\n")
	extractor := NewTagExtractor()
	candidates, extractErr := extractor.ExtractTags(context.Background(), input)
	if extractErr != nil {
		fmt.Fprintf(&sb, "  ExtractTags 错误: %v\n", extractErr)
	} else {
		fmt.Fprintf(&sb, "  返回标签数: %d, Source: %s\n", len(candidates.Tags), candidates.Source)
		if len(candidates.Skipped) > 0 {
			fmt.Fprintf(&sb, "  Skipped: %v\n", candidates.Skipped)
		}
		if len(candidates.Errors) > 0 {
			fmt.Fprintf(&sb, "  Errors: %v\n", candidates.Errors)
		}
		for i, tag := range candidates.Tags {
			fmt.Fprintf(&sb, "  Tag[%d]: label=%q category=%s score=%.2f\n", i, tag.Label, tag.Category, tag.Score)
		}
	}

	outputPath := filepath.Join("D:\\project\\my-robot", "tag_context_dump_74551.txt")
	if err := os.WriteFile(outputPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write dump file: %v", err)
	}
	t.Logf("Written to: %s", outputPath)
}

func padLines(s string, indent int) string {
	prefix := strings.Repeat(" ", indent)
	return strings.Join(strings.Split(s, "\n"), "\n"+prefix)
}

func truncateAndPad(s string, maxRunes int, indent int) string {
	prefix := strings.Repeat(" ", indent)
	runes := []rune(s)
	if len(runes) > maxRunes {
		s = string(runes[:maxRunes]) + "..."
	}
	return strings.Join(strings.Split(s, "\n"), "\n"+prefix)
}

func truncateByRune(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return s
}

func TestOllamaRawResponseForTagExtraction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real Ollama test in short mode")
	}

	if err := config.LoadConfig("./../../../configs"); err != nil {
		t.Logf("config load warning: %v", err)
	}
	if err := database.InitDB(config.AppConfig); err != nil {
		t.Fatalf("failed to connect DB: %v", err)
	}

	var route models.AIRoute
	if err := database.DB.Preload("RouteProviders.Provider").
		Where("capability = ? AND enabled = ?", "topic_tagging", true).
		First(&route).Error; err != nil {
		t.Fatalf("no enabled topic_tagging route found: %v", err)
	}
	var provider models.AIProvider
	found := false
	for _, rp := range route.RouteProviders {
		if rp.Enabled && rp.Provider.Enabled {
			provider = rp.Provider
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no enabled provider in topic_tagging route")
	}

	t.Logf("Provider: name=%s type=%s model=%s base_url=%s", provider.Name, provider.ProviderType, provider.Model, provider.BaseURL)

	var article models.Article
	articleID := os.Getenv("TEST_ARTICLE_ID")
	if articleID == "" {
		articleID = "74426"
	}
	if err := database.DB.First(&article, articleID).Error; err != nil {
		t.Fatalf("article %s not found: %v", articleID, err)
	}
	t.Logf("Testing article %d: %s", article.ID, article.Title)

	var feed models.Feed
	if err := database.DB.Preload("Category").First(&feed, article.FeedID).Error; err != nil {
		t.Fatalf("feed %d not found: %v", article.FeedID, err)
	}

	summary := buildArticleSummary(article)
	systemPrompt := buildEventPersonPrompt()
	userPrompt := buildExtractionUserPrompt(ExtractionInput{
		Title:        article.Title,
		Summary:      summary,
		FeedName:     feed.Title,
		CategoryName: FeedCategoryName(feed),
	})

	schema := eventPersonExtractionSchema()
	temperature := 0.2
	maxTokens := 1024

	payload := map[string]any{
		"model":            provider.Model,
		"messages":         []airouter.Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		"temperature":      temperature,
		"max_tokens":       maxTokens,
		"reasoning_effort": "none",
		"format":           schema,
	}

	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")

	var sb strings.Builder
	sep := strings.Repeat("=", 80)
	sb.WriteString(sep + "\n")
	sb.WriteString(" Ollama Raw Response Dump — Tag Extraction\n")
	sb.WriteString(sep + "\n\n")

	sb.WriteString("## Provider Info\n\n")
	fmt.Fprintf(&sb, "  Name:      %s\n", provider.Name)
	fmt.Fprintf(&sb, "  Type:      %s\n", provider.ProviderType)
	fmt.Fprintf(&sb, "  Model:     %s\n", provider.Model)
	fmt.Fprintf(&sb, "  BaseURL:   %s\n", provider.BaseURL)
	fmt.Fprintf(&sb, "  Timeout:   %ds\n", provider.TimeoutSeconds)
	sb.WriteString("\n## Request Payload\n\n")
	sb.WriteString("  " + strings.Repeat("-", 76) + "\n")
	for _, l := range strings.Split(string(payloadJSON), "\n") {
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("  " + strings.Repeat("-", 76) + "\n")

	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	fmt.Fprintf(&sb, "\n## Endpoint\n\n  %s\n", endpoint)
	bodyReader := bytes.NewReader(payloadJSON)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	start := time.Now()
	resp, err := (&http.Client{}).Do(httpReq)
	latency := time.Since(start)
	if err != nil {
		fmt.Fprintf(&sb, "\n## Error\n\n  %v\n", err)
		writeDump(t, sb.String())
		return
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(resp.Body)

	fmt.Fprintf(&sb, "\n## Response (latency: %v)\n\n", latency)
	fmt.Fprintf(&sb, "  HTTP Status: %d %s\n", resp.StatusCode, resp.Status)
	fmt.Fprintf(&sb, "  Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Fprintf(&sb, "  Body Length: %d bytes\n", len(responseBody))
	sb.WriteString("\n## Response Body (Raw)\n\n")
	sb.WriteString("  " + strings.Repeat("-", 76) + "\n")
	rawIndent := jsonIndentOrRaw(responseBody)
	for _, l := range strings.Split(rawIndent, "\n") {
		sb.WriteString("  " + l + "\n")
	}
	sb.WriteString("  " + strings.Repeat("-", 76) + "\n")

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Model           string   `json:"model"`
		Done            bool     `json:"done"`
		Duration        *float64 `json:"total_duration"`
		EvalCount       *int     `json:"eval_count"`
		PromptEvalCount *int     `json:"prompt_eval_count"`
		Error           *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	parseErr := json.Unmarshal(responseBody, &parsed)

	sb.WriteString("\n## Parsed Fields\n\n")
	if parseErr != nil {
		fmt.Fprintf(&sb, "  Parse error: %v\n", parseErr)
	} else {
		fmt.Fprintf(&sb, "  Model:             %s\n", parsed.Model)
		fmt.Fprintf(&sb, "  Done:              %v\n", parsed.Done)
		fmt.Fprintf(&sb, "  Choices count:     %d\n", len(parsed.Choices))
		if parsed.Duration != nil {
			fmt.Fprintf(&sb, "  Total Duration:    %.0f ns (%.2f s)\n", *parsed.Duration, float64(*parsed.Duration)/1e9)
		}
		if parsed.EvalCount != nil {
			fmt.Fprintf(&sb, "  Eval Count:        %d\n", *parsed.EvalCount)
		}
		if parsed.PromptEvalCount != nil {
			fmt.Fprintf(&sb, "  Prompt Eval Count: %d\n", *parsed.PromptEvalCount)
		}
		if parsed.Error != nil {
			fmt.Fprintf(&sb, "  Error:             %s (%s)\n", parsed.Error.Message, parsed.Error.Type)
		}
		for i, choice := range parsed.Choices {
			fmt.Fprintf(&sb, "\n  Choice[%d]:\n", i)
			fmt.Fprintf(&sb, "    FinishReason: %s\n", choice.FinishReason)
			fmt.Fprintf(&sb, "    Content length: %d\n", len(choice.Message.Content))
			sb.WriteString("    Content:\n")
			contentIndent := jsonIndentOrRaw([]byte(choice.Message.Content))
			for _, l := range strings.Split(contentIndent, "\n") {
				sb.WriteString("      " + l + "\n")
			}
		}
	}

	sb.WriteString("\n## After branch parsers\n\n")
	if len(parsed.Choices) > 0 {
		tags, err := parseEventPersonTags(parsed.Choices[0].Message.Content)
		if err != nil {
			tags, err = parseKeywordTags(parsed.Choices[0].Message.Content)
		}
		if err != nil {
			fmt.Fprintf(&sb, "  Parse error: %v\n", err)
		} else {
			fmt.Fprintf(&sb, "  Extracted %d tags:\n", len(tags))
			for i, tag := range tags {
				sb.WriteString(fmt.Sprintf("    [%d] label=%q category=%s aliases=%v\n",
					i, tag.Label, tag.Category, tag.Aliases))
			}
		}
	}

	writeDump(t, sb.String())
}

func TestRealExtractTagsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real flow test in short mode")
	}

	if err := config.LoadConfig("./../../../configs"); err != nil {
		t.Logf("config load warning: %v", err)
	}
	if err := database.InitDB(config.AppConfig); err != nil {
		t.Fatalf("failed to connect DB: %v", err)
	}

	articleID := os.Getenv("TEST_ARTICLE_ID")
	if articleID == "" {
		articleID = "74426"
	}

	var article models.Article
	if err := database.DB.First(&article, articleID).Error; err != nil {
		t.Fatalf("article %s not found: %v", articleID, err)
	}

	var feed models.Feed
	if err := database.DB.Preload("Category").First(&feed, article.FeedID).Error; err != nil {
		t.Fatalf("feed %d not found: %v", article.FeedID, err)
	}

	feedName := feed.Title
	categoryName := FeedCategoryName(feed)
	summary := buildArticleSummary(article)

	t.Logf("Article %d: %s", article.ID, article.Title)
	t.Logf("AIContentSummary len=%d, FirecrawlContent len=%d", len(article.AIContentSummary), len(article.FirecrawlContent))
	t.Logf("buildArticleSummary len=%d (maxSummaryRunesForTagging=%d)", len(summary), maxSummaryRunesForTagging)

	input := ExtractionInput{
		Title:        article.Title,
		Summary:      summary,
		FeedName:     feedName,
		CategoryName: categoryName,
		ArticleID:    &article.ID,
	}

	extractor := NewTagExtractor()
	result, err := extractor.ExtractTags(context.Background(), input)

	var sb strings.Builder
	sep := strings.Repeat("=", 80)
	sb.WriteString(sep + "\n")
	fmt.Fprintf(&sb, " Real ExtractTags Flow — Article %s\n", articleID)
	sb.WriteString(sep + "\n\n")

	sb.WriteString("## Input\n\n")
	fmt.Fprintf(&sb, "  Title:   %s\n", input.Title)
	fmt.Fprintf(&sb, "  Feed:    %s\n", input.FeedName)
	fmt.Fprintf(&sb, "  Category:%s\n", input.CategoryName)
	fmt.Fprintf(&sb, "  Summary len: %d / %d runes\n", len(input.Summary), len([]rune(input.Summary)))
	fmt.Fprintf(&sb, "  Summary first 200 chars: %s\n", truncateByRune(input.Summary, 200))
	sb.WriteString("\n## ExtractTags Result\n\n")
	fmt.Fprintf(&sb, "  Error: %v\n", err)
	if result != nil {
		fmt.Fprintf(&sb, "  Source: %s\n", result.Source)
		fmt.Fprintf(&sb, "  Tags count: %d\n", len(result.Tags))
		fmt.Fprintf(&sb, "  Skipped: %v\n", result.Skipped)
		fmt.Fprintf(&sb, "  Errors: %v\n", result.Errors)
		for i, tag := range result.Tags {
			sb.WriteString(fmt.Sprintf("  Tag[%d]: label=%q category=%s score=%.2f isNew=%v matchedTo=%d\n",
				i, tag.Label, tag.Category, tag.Score, tag.IsNew, tag.MatchedTo))
		}
	} else {
		sb.WriteString("  Result: nil\n")
	}

	writeDump(t, sb.String())
}

func jsonIndentOrRaw(data []byte) string {
	var buf bytes.Buffer
	if json.Indent(&buf, data, "  ", "  ") == nil {
		return buf.String()
	}
	return string(data)
}

func writeDump(t *testing.T, content string) {
	outputPath := filepath.Join("D:\\project\\my-robot", "ollama_raw_response_dump.txt")
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}
	t.Logf("Written to: %s", outputPath)
}
