package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"

	"syntopica-backend/internal/domain/tagging"
	"syntopica-backend/internal/platform/airouter"
)

type AIAnalyzer interface {
	Analyze(ctx context.Context, params AnalysisParams) (*AnalysisResult, error)
	GetPromptTemplate(analysisType string) string
	ParseResponse(response string, analysisType string) (*AnalysisResult, error)
}

type AnalysisParams struct {
	TopicTagID   uint64
	TopicLabel   string
	AnalysisType string
	WindowType   string
	AnchorDate   time.Time
	Summaries    []SummaryInfo
}

type SummaryInfo struct {
	ArticleID    uint64 `json:"article_id,omitempty"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	FeedName     string `json:"feed_name,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

type TimelineEvent struct {
	Date     string        `json:"date"`
	Title    string        `json:"title"`
	Summary  string        `json:"summary"`
	Sources  []EventSource `json:"sources,omitempty"`
	Entities []EntityBrief `json:"entities,omitempty"`
}

type EventSource struct {
	ArticleID uint64 `json:"article_id,omitempty"`
	Title     string `json:"title,omitempty"`
}

type EntityBrief struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type PersonProfile struct {
	Name       string `json:"name,omitempty"`
	Role       string `json:"role,omitempty"`
	Background string `json:"background,omitempty"`
}

type Appearance struct {
	Date         string `json:"date,omitempty"`
	Scene        string `json:"scene,omitempty"`
	Quote        string `json:"quote,omitempty"`
	ArticleID    uint64 `json:"article_id,omitempty"`
	ArticleTitle string `json:"article_title,omitempty"`
}

type TrendPoint struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type RelatedTopic struct {
	Topic string  `json:"topic,omitempty"`
	Score float64 `json:"score,omitempty"`
}

type CoOccurrence struct {
	Keyword string  `json:"keyword"`
	Score   float64 `json:"score,omitempty"`
}

type ContextExample struct {
	Text   string `json:"text,omitempty"`
	Source string `json:"source,omitempty"`
	Date   string `json:"date,omitempty"`
}

type AnalysisResult struct {
	Timeline        []TimelineEvent  `json:"timeline,omitempty"`
	Profile         *PersonProfile   `json:"profile,omitempty"`
	Appearances     []Appearance     `json:"appearances,omitempty"`
	TrendData       []TrendPoint     `json:"trend_data,omitempty"`
	RelatedTopics   []RelatedTopic   `json:"related_topics,omitempty"`
	CoOccurrence    []CoOccurrence   `json:"co_occurrence,omitempty"`
	ContextExamples []ContextExample `json:"context_examples,omitempty"`
	Summary         string           `json:"summary"`
}

type aiPromptInput struct {
	TopicLabel string
	WindowType string
	AnchorDate string
	Summaries  string
}

type AIAnalysisService struct {
	router         *airouter.Router
	logger         *zap.Logger
	maxTokens      int
	temperature    float64
	requestTimeout time.Duration
	maxRetries     int
}

const eventPromptTemplate = `你是一个新闻事件分析专家。请分析以下与"{{.TopicLabel}}"相关的新闻摘要，提取关键事件时间线。

时间窗口: {{.WindowType}} ({{.AnchorDate}})
相关摘要:
{{.Summaries}}

请输出JSON格式:
{
  "timeline": [
    {
      "date": "YYYY-MM-DD",
      "title": "事件标题",
      "summary": "事件摘要",
      "sources": [{"article_id": 1, "title": "文章标题"}]
    }
  ],
  "key_moments": ["关键节点1", "关键节点2"],
  "related_entities": [{"name": "实体名", "type": "person|org"}],
  "summary": "整体事件总结"
}`

const personPromptTemplate = `你是一个新闻人物分析专家。请分析以下与"{{.TopicLabel}}"相关的新闻摘要，生成人物档案和关键出现记录。

时间窗口: {{.WindowType}} ({{.AnchorDate}})
相关摘要:
{{.Summaries}}

请输出JSON格式:
{
  "profile": {
    "name": "人物名称",
    "role": "身份角色",
    "background": "背景简介"
  },
  "appearances": [
    {
      "date": "YYYY-MM-DD",
      "scene": "出现事件",
      "quote": "核心观点",
      "article_id": 1,
      "article_title": "文章标题"
    }
  ],
  "related_topics": [{"topic": "关联主题", "score": 0.8}],
  "summary": "人物动态总结"
}`

const keywordPromptTemplate = `你是一个新闻关键词分析专家。请分析以下与"{{.TopicLabel}}"相关的新闻摘要，提取趋势、共现和上下文。

时间窗口: {{.WindowType}} ({{.AnchorDate}})
相关摘要:
{{.Summaries}}

请输出JSON格式:
{
  "trend_data": [
    {"date": "YYYY-MM-DD", "count": 3}
  ],
  "related_topics": [{"topic": "相关主题", "score": 0.7}],
  "co_occurrence": [{"keyword": "共现关键词", "score": 0.6}],
  "context_examples": [{"text": "上下文片段", "source": "文章标题", "date": "YYYY-MM-DD"}],
  "summary": "趋势总结"
}`

func NewAIAnalysisService(logger *zap.Logger) *AIAnalysisService {
	if logger == nil {
		logger = zap.NewNop()
	}

	maxTokens := parseEnvInt("TOPIC_ANALYSIS_MAX_TOKENS", 2000)
	temperature := parseEnvFloat("TOPIC_ANALYSIS_TEMPERATURE", 0.2)
	timeoutSeconds := parseEnvInt("TOPIC_ANALYSIS_TIMEOUT_SECONDS", 90)
	maxRetries := parseEnvInt("TOPIC_ANALYSIS_RETRY_COUNT", 3)

	return &AIAnalysisService{
		router:         airouter.NewRouter(),
		logger:         logger,
		maxTokens:      maxTokens,
		temperature:    temperature,
		requestTimeout: time.Duration(timeoutSeconds) * time.Second,
		maxRetries:     maxInt(maxRetries, 1),
	}
}

func (s *AIAnalysisService) Analyze(ctx context.Context, params AnalysisParams) (*AnalysisResult, error) {
	if err := validateAnalysisParams(params.AnalysisType, params.WindowType); err != nil {
		return nil, err
	}

	if len(params.Summaries) == 0 {
		return &AnalysisResult{Summary: "暂无可分析内容"}, nil
	}

	prompt, err := s.buildPrompt(params)
	if err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var response string
	for attempt := 1; attempt <= s.maxRetries; attempt++ {
		reqCtx, cancel := context.WithTimeout(ctx, s.requestTimeout)
		response, err = s.callModel(reqCtx, params, prompt, attempt)
		cancel()
		if err == nil {
			break
		}
		s.logger.Warn("topic analysis ai call failed",
			zap.Int("attempt", attempt),
			zap.Int("max_retries", s.maxRetries),
			zap.Uint64("topic_tag_id", params.TopicTagID),
			zap.String("analysis_type", params.AnalysisType),
			zap.Error(err),
		)
		if attempt < s.maxRetries {
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("topic analysis ai request failed after retries: %w", err)
	}

	result, err := s.ParseResponse(response, params.AnalysisType)
	if err != nil {
		return nil, fmt.Errorf("parse topic analysis response failed: %w", err)
	}
	return result, nil
}

func (s *AIAnalysisService) GetPromptTemplate(analysisType string) string {
	switch normalizeAnalysisType(analysisType) {
	case AnalysisTypePerson:
		return personPromptTemplate
	case AnalysisTypeKeyword:
		return keywordPromptTemplate
	default:
		return eventPromptTemplate
	}
}

func (s *AIAnalysisService) ParseResponse(response string, analysisType string) (*AnalysisResult, error) {
	raw := extractJSONObject(response)
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("empty ai response")
	}

	result := &AnalysisResult{}
	switch normalizeAnalysisType(analysisType) {
	case AnalysisTypeEvent:
		var payload struct {
			Timeline        []TimelineEvent `json:"timeline"`
			RelatedEntities []EntityBrief   `json:"related_entities"`
			Summary         string          `json:"summary"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
		for i := range payload.Timeline {
			if len(payload.Timeline[i].Entities) == 0 {
				payload.Timeline[i].Entities = payload.RelatedEntities
			}
		}
		result.Timeline = payload.Timeline
		result.Summary = payload.Summary
	case AnalysisTypePerson:
		var payload struct {
			Profile       *PersonProfile `json:"profile"`
			Appearances   []Appearance   `json:"appearances"`
			RelatedTopics []RelatedTopic `json:"related_topics"`
			Summary       string         `json:"summary"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
		result.Profile = payload.Profile
		result.Appearances = payload.Appearances
		result.RelatedTopics = payload.RelatedTopics
		result.Summary = payload.Summary
	case AnalysisTypeKeyword:
		var payload struct {
			TrendData       []TrendPoint     `json:"trend_data"`
			RelatedTopics   []RelatedTopic   `json:"related_topics"`
			CoOccurrence    []CoOccurrence   `json:"co_occurrence"`
			ContextExamples []ContextExample `json:"context_examples"`
			Summary         string           `json:"summary"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, err
		}
		result.TrendData = payload.TrendData
		result.RelatedTopics = payload.RelatedTopics
		result.CoOccurrence = payload.CoOccurrence
		result.ContextExamples = payload.ContextExamples
		result.Summary = payload.Summary
	default:
		return nil, fmt.Errorf("unsupported analysis type: %s", analysisType)
	}

	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = "分析已完成"
	}
	return result, nil
}

func (s *AIAnalysisService) buildPrompt(params AnalysisParams) (string, error) {
	tmplText := s.GetPromptTemplate(params.AnalysisType)
	tmpl, err := template.New("topic_analysis_prompt").Parse(tmplText)
	if err != nil {
		return "", err
	}

	lines := make([]string, 0, len(params.Summaries))
	for idx, summary := range params.Summaries {
		title := strings.TrimSpace(summary.Title)
		if len([]rune(title)) > 120 {
			title = string([]rune(title)[:120])
		}
		content := strings.TrimSpace(summary.Summary)
		if len([]rune(content)) > 300 {
			content = string([]rune(content)[:300])
		}
		line := fmt.Sprintf("%d. [%s] %s\n%s", idx+1, summary.CreatedAt, title, content)
		lines = append(lines, line)
	}

	input := aiPromptInput{
		TopicLabel: strings.TrimSpace(params.TopicLabel),
		WindowType: normalizeWindowType(params.WindowType),
		AnchorDate: normalizeQueueAnchorDate(params.AnchorDate).In(tagging.TopicGraphCST).Format("2006-01-02"),
		Summaries:  strings.Join(lines, "\n\n"),
	}

	var builder strings.Builder
	if err := tmpl.Execute(&builder, input); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func (s *AIAnalysisService) callModel(ctx context.Context, params AnalysisParams, prompt string, attempt int) (string, error) {
	maxTokens := s.maxTokens
	temperature := s.temperature

	start := time.Now()
	resp, err := s.router.Chat(ctx, airouter.ChatRequest{
		Capability: airouter.CapabilityTopicTagging,
		Messages: []airouter.Message{
			{Role: "system", Content: "你是新闻主题分析助手。只输出合法 JSON，不要额外解释。"},
			{Role: "user", Content: prompt},
		},
		MaxTokens:   &maxTokens,
		Temperature: &temperature,
		Metadata: map[string]any{
			"topic_tag_id":  params.TopicTagID,
			"topic_label":   params.TopicLabel,
			"analysis_type": params.AnalysisType,
			"window_type":   params.WindowType,
			"anchor_date":   normalizeQueueAnchorDate(params.AnchorDate).Format("2006-01-02"),
			"summary_count": len(params.Summaries),
			"attempt":       attempt,
			"source":        "topic_graph_analysis",
		},
	})

	latency := time.Since(start)
	if err != nil {
		s.logger.Error("topic analysis ai call error",
			zap.Uint64("topic_tag_id", params.TopicTagID),
			zap.String("analysis_type", params.AnalysisType),
			zap.Duration("latency", latency),
			zap.Error(err),
		)
		return "", err
	}

	s.logger.Info("topic analysis ai call completed",
		zap.Uint64("topic_tag_id", params.TopicTagID),
		zap.String("analysis_type", params.AnalysisType),
		zap.Duration("latency", latency),
	)
	return resp.Content, nil
}

func parseEnvInt(key string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func parseEnvFloat(key string, defaultValue float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func extractJSONObject(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start < 0 || end <= start {
		return trimmed
	}
	return trimmed[start : end+1]
}
