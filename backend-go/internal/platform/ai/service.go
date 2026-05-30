package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AIService struct {
	BaseURL string
	APIKey  string
	Model   string
	client  *http.Client
}

type AISummaryRequest struct {
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Language string `json:"language"`
}

type AISummaryResponse struct {
	OneSentence string   `json:"one_sentence"`
	KeyPoints   []string `json:"key_points"`
	Takeaways   []string `json:"takeaways"`
	Tags        []string `json:"tags"`
	Markdown    string   `json:"markdown"`
}

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func NewAIService(baseURL, apiKey, model string) *AIService {
	return &AIService{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (s *AIService) SummarizeArticle(title, content, language string) (*AISummaryResponse, error) {
	systemPrompt := s.GetSystemPrompt(language)
	userContent := s.PrepareArticleContent(title, content)

	req := openAIRequest{
		Model: s.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		Temperature: 0.3,
		MaxTokens:   16000,
	}

	resp, err := s.callOpenAI(req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("AI API error: %s", resp.Error.Message)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	summaryText := cleanSummaryMarkdown(resp.Choices[0].Message.Content)
	summary := ParseSummaryMarkdown(summaryText)
	summary.Markdown = summaryText
	return summary, nil
}

func (s *AIService) GetSystemPrompt(language string) string {
	if language == "zh" {
		return `你是一名中文编辑，负责把抓取到的网页正文整理成适合 RSS 阅读器展示的 Markdown 成稿。

目标：
1. 尽量完整保留文章主体信息，不要写成过短摘要。
2. 删除非正文噪音，如导航、菜单、登录提示、广告、推荐阅读、版权尾注、Cookie 提示、分享按钮文案、重复页脚。
3. 在不改变原意的前提下，重组杂乱段落，让版式清晰、适合连续阅读。

输出要求：
1. 必须输出简体中文 Markdown。
2. 必须以 "# 原文标题" 开头。
3. 紧接着输出 "## 导读" 小节，提供 3-5 条项目符号，快速说明这篇文章讲了什么。
4. 然后输出 "## 正文整理" 小节。
5. 在 "## 正文整理" 中，尽量按原文主题顺序保留内容；如果原文有明显分节，使用 "###" 小标题重建结构。
6. 原文中的列表、引用、表格、链接、日期、数字、专有名词、代码名、产品名，能保留就保留。
7. 如果原文是教程、公告、发布说明、评测或访谈，要保留其原有层次，不要强行改成新闻快讯。
8. 如果原文存在明显的关键信息汇总，文末追加 "## 关键信息" 小节，用 3-6 条项目符号提炼最重要的结论、变更或影响。
9. 不要输出“作为 AI”“根据提示”等说明。
10. 不要使用代码块包裹整篇结果，只输出 Markdown 正文。

排版要求：
- 段落不要过长，必要时拆段。
- 小节标题要克制，不要为了排版制造空洞标题。
- 如果原文结构本来很清楚，就尽量贴近原文结构。
- 如果原文结构混乱，优先保证信息完整，其次再优化阅读顺序。`
	}

	return `你将抓取到的网页内容重写为适合阅读的 polished 版本。
仅返回 Markdown。

规则：
1. 以 '# <文章标题>' 开头。
2. 在顶部附近添加一个简短的要点摘要。
3. 正文使用 Markdown 格式，标题清晰。
4. 保留有用的列表、引用、表格、链接、日期、名称、数字和产品术语。
5. 删除广告、导航文本、Cookie 提示、重复的页脚文本和明显的样板内容。
6. 如果原文结构混乱，将其重新组织成更清晰的文章，同时保留原始事实。
7. 不要提及提示。不要用代码块包裹输出。`
}

func (s *AIService) PrepareArticleContent(title, content string) string {
	maxContentLength := 80000
	if len(content) > maxContentLength {
		content = content[:maxContentLength] + "..."
	}

	return fmt.Sprintf("Title: %s\n\nSource content in Markdown:\n%s", title, content)
}

func ParseSummaryMarkdown(responseText string) *AISummaryResponse {
	summary := &AISummaryResponse{
		KeyPoints: make([]string, 0),
		Takeaways: make([]string, 0),
		Tags:      make([]string, 0),
		Markdown:  responseText,
	}

	plain := markdownToPlainText(responseText)
	if plain != "" {
		if len(plain) > 180 {
			summary.OneSentence = plain[:180]
		} else {
			summary.OneSentence = plain
		}
	}

	return summary
}

func cleanSummaryMarkdown(input string) string {
	text := strings.TrimSpace(input)
	text = strings.TrimPrefix(text, "```markdown")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}

func markdownToPlainText(input string) string {
	replacer := strings.NewReplacer(
		"#", " ",
		"*", " ",
		"`", " ",
		">", " ",
		"|", " ",
	)
	plain := replacer.Replace(input)
	return strings.Join(strings.Fields(plain), " ")
}

func (s *AIService) callOpenAI(req openAIRequest) (*openAIResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", s.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.APIKey)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, err
	}

	return &openAIResp, nil
}

func (s *AIService) TestConnection() error {
	req := openAIRequest{
		Model:     s.Model,
		Messages:  []openAIMessage{{Role: "user", Content: "Hi"}},
		MaxTokens: 10,
	}

	resp, err := s.callOpenAI(req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("API error: %s", resp.Error.Message)
	}
	return nil
}
