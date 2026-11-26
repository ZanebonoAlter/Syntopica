package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"syntopica-backend/internal/models"
	"syntopica-backend/internal/platform/airouter"
	"syntopica-backend/internal/platform/aisettings"
	"syntopica-backend/internal/reader/repository"
)

// ============================================================================
// 疑似 AI 生成判定服务（add-ai-generated-article-detection design D2/D3/D10/D11）
// ============================================================================
//
// 判定链路与标签打标链路彻底解耦：一篇一作业一次调用，只输出一个 0-1 置信分。
// 判定输入按「全文抓取正文 → 订阅源原始正文 → 订阅源摘要」回退（判定专用口径，与
// 展示层的 AIContentSummary→FirecrawlContent→Content→Description 优先级无关）。
// 样本截断为首段 + 尾段（按 rune 切，不按字节）。阈值与样本长度经 ai_settings 在线可调。
//
// 失败语义：判定失败（能力未绑定模型 / provider 全挂 / 置信分解析不出）一律不改文章
// 判定状态（保持「未判定」），失败原因进 ai_call_logs 审计（红线：成败都写）。

// 判定状态取值（design D14 命名契约）。权威定义在 models 包（前后端与投影端共用），
// 这里别名导出，供判定服务与域外调用方直接引用。
const (
	// AIDetectionStatusUnknown 未判定（存量文章与判定失败的文章都停在这里）。
	AIDetectionStatusUnknown = models.AIDetectionStatusUnknown
	// AIDetectionStatusSuspect 疑似 AI 生成（置信分达到阈值）。
	AIDetectionStatusSuspect = models.AIDetectionStatusSuspect
	// AIDetectionStatusNotAI 已确认非 AI（置信分低于阈值）。
	AIDetectionStatusNotAI = models.AIDetectionStatusNotAI
)

// 判定来源取值（design D14）。
const (
	// AIDetectionSourceLLM 自动判定。
	AIDetectionSourceLLM = models.AIDetectionSourceLLM
	// AIDetectionSourceManual 人工标记（人工结论优先，自动判定不得覆盖）。
	AIDetectionSourceManual = models.AIDetectionSourceManual
)

// AIDetectionOperation 是判定调用的 operation 名。它只进 ai_call_logs 审计日志，
// 不参与模型选择——路由按 capability 解析（见 airouter store_vendor.go）。
const AIDetectionOperation = "tagmanagement.ai_detection"

// 判定输入取用来源标记（可观测：审计 metadata 与判定结果都会带）。
const (
	AIDetectionInputFirecrawl   = "firecrawl"
	AIDetectionInputContent     = "content"
	AIDetectionInputDescription = "description"
)

// ErrAIDetectionManualOverride 表示「文章已有人工判定结论，本次自动判定被跳过」。
// 调用方（判定 worker）应用 errors.Is 判定它并按「跳过、非失败」处理。
var ErrAIDetectionManualOverride = errors.New("ai detection: manual conclusion takes precedence")

// ErrAIDetectionNoInput 表示文章没有任何可用于判定的文本。
var ErrAIDetectionNoInput = errors.New("ai detection: article has no detection input")

// AIDetectionResult 是一次判定的结论。
type AIDetectionResult struct {
	// Score 是 0-1 的置信分。
	Score float64
	// Status 是判定状态（suspect / not_ai）。
	Status string
	// Source 是判定来源（自动判定恒为 llm）。
	Source string
	// InputSource 是判定输入的取用来源（firecrawl / content / description）。
	InputSource string
	// Threshold 是本次判定采用的阈值（审计/排查用）。
	Threshold float64
	// DetectedAt 是判定时刻。
	DetectedAt time.Time
}

// AIDetectionChatRouter 是判定服务的 LLM 出口。生产实现是 *airouter.Router；
// 抽出接口是为了让包级测试能注入假实现（与 tagmanagement 的 ChatRouter 同款做法）。
type AIDetectionChatRouter interface {
	Chat(ctx context.Context, req airouter.ChatRequest) (*airouter.ChatResult, error)
	// LogLocalFailure 补写一条「本地判定失败」审计行（路由缺失 / 响应不可解析）。
	LogLocalFailure(ctx context.Context, req airouter.ChatRequest, failure airouter.LocalFailure)
}

// AIDetectionConfigProvider 读取判定参数（阈值 + 样本长度）。
// 生产实现是 aisettings.LoadAIDetectionConfig：每次判定现读，改后立即生效。
type AIDetectionConfigProvider func() (aisettings.AIDetectionConfig, error)

// AIDetectionService 是文章级「疑似 AI 生成」判定服务。
type AIDetectionService struct {
	mu     sync.Mutex
	router AIDetectionChatRouter
	config AIDetectionConfigProvider
}

// NewAIDetectionService 构造判定服务（生产用）。
func NewAIDetectionService() *AIDetectionService {
	return &AIDetectionService{config: aisettings.LoadAIDetectionConfig}
}

// NewAIDetectionServiceWithRouter 构造注入指定 LLM 出口与参数读取器的判定服务（测试用）。
func NewAIDetectionServiceWithRouter(router AIDetectionChatRouter, config AIDetectionConfigProvider) *AIDetectionService {
	return &AIDetectionService{router: router, config: config}
}

// resolveRouter 延迟构造 airouter.Router：Router 在构造时捕获当时的 database.DB，
// 早于 DB 初始化构造的单例会拿到空 store（审计静默丢失），故首次使用时再建。
func (s *AIDetectionService) resolveRouter() AIDetectionChatRouter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.router == nil {
		s.router = airouter.NewRouter()
	}
	return s.router
}

// loadConfig 读取判定参数。参数读取失败时回落到默认值并继续（配置读不出来不该让
// 判定链路整体瘫痪）；阈值取默认 0.7、样本取默认首 3000 + 尾 2000 runes。
func (s *AIDetectionService) loadConfig() aisettings.AIDetectionConfig {
	if s.config == nil {
		return aisettings.DefaultAIDetectionConfig()
	}
	cfg, err := s.config()
	if err != nil {
		return aisettings.DefaultAIDetectionConfig()
	}
	return cfg
}

// HasManualAIDetection 报告文章是否已带人工判定结论（来源=manual）。
//
// 「人工结论优先」（design D10）：文章已被人工标记或人工取消后，后续自动判定不得
// 覆盖该人工结论，除非用户显式触发重新判定（force=true）。这是判定 worker 与
// 人工标记入口共用的判定函数，导出供跨包复用。
func HasManualAIDetection(article *models.Article) bool {
	if article == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(article.AIDetectionSource), AIDetectionSourceManual)
}

// SelectAIDetectionInput 按「全文抓取正文 → 订阅源原始正文 → 订阅源摘要」的优先级取用
// 判定输入（design D3）。返回文本与取用来源标记；三个来源都为空时返回 ("", "")。
func SelectAIDetectionInput(article *models.Article) (string, string) {
	if article == nil {
		return "", ""
	}
	if text := strings.TrimSpace(article.FirecrawlContent); text != "" {
		return article.FirecrawlContent, AIDetectionInputFirecrawl
	}
	if text := strings.TrimSpace(article.Content); text != "" {
		return article.Content, AIDetectionInputContent
	}
	if text := strings.TrimSpace(article.Description); text != "" {
		return article.Description, AIDetectionInputDescription
	}
	return "", ""
}

// BuildAIDetectionSample 把判定输入截断为「首段 + 尾段」样本（design D3）。
//
// 按 rune 计数（不是字节）——中文正文按字节切会切出半个字符。样本长度恰为
// head+tail runes（不加省略标记），模型侧靠提示词说明「首尾拼接、中间省略」。
// 文本本身短于 head+tail 时原样返回。
func BuildAIDetectionSample(text string, head, tail int) string {
	if head <= 0 && tail <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= head+tail {
		return text
	}
	if head <= 0 {
		return string(runes[len(runes)-tail:])
	}
	if tail <= 0 {
		return string(runes[:head])
	}
	return string(runes[:head]) + string(runes[len(runes)-tail:])
}

// aiDetectionScorePattern 匹配响应里的浮点数字（允许小数与科学计数法）。
var aiDetectionScorePattern = regexp.MustCompile(`[-+]?\d*\.?\d+(?:[eE][-+]?\d+)?`)

// ParseAIDetectionScore 从模型响应里提取 0-1 的置信分。
//
// 容错口径：提示词要求模型只输出一个 0-1 的纯数字，但小模型常带前后缀
// （"0.86"、"置信分：0.86"、"0.86\n"）。先试整段解析（覆盖标准输出），再退化为
// 「取首个落在 0-1 区间的数字」。
//
// 解析失败 → (0, false)：判定失败（状态保持「未判定」，原因进审计），
// 不得静默当 0 分或当非疑似。
func ParseAIDetectionScore(content string) (float64, bool) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return 0, false
	}
	if value, err := strconv.ParseFloat(trimmed, 64); err == nil {
		if value >= 0 && value <= 1 {
			return value, true
		}
		return 0, false
	}
	for _, token := range aiDetectionScorePattern.FindAllString(trimmed, -1) {
		value, err := strconv.ParseFloat(token, 64)
		if err != nil {
			continue
		}
		if value >= 0 && value <= 1 {
			return value, true
		}
	}
	return 0, false
}

// DetectArticle 对一篇文章做「疑似 AI 生成」判定：一次 LLM 调用，产出一个 0-1 置信分。
//
// force=false 且文章已有人工结论时直接跳过（人工结论优先，design D10），返回
// ErrAIDetectionManualOverride；force=true 时覆盖人工结论（显式重判入口）。
//
// 本方法只判定、不落库（落库见 PersistAIDetectionResult）。判定失败时文章判定状态
// 保持「未判定」，失败原因进 ai_call_logs 审计。
func (s *AIDetectionService) DetectArticle(ctx context.Context, article *models.Article, force bool) (*AIDetectionResult, error) {
	if article == nil {
		return nil, errors.New("ai detection: article is nil")
	}
	if !force && HasManualAIDetection(article) {
		return nil, ErrAIDetectionManualOverride
	}

	cfg := s.loadConfig()

	text, inputSource := SelectAIDetectionInput(article)
	if text == "" {
		return nil, fmt.Errorf("%w: article %d", ErrAIDetectionNoInput, article.ID)
	}
	sample := BuildAIDetectionSample(text, cfg.SampleHead, cfg.SampleTail)
	if sample == "" {
		// 样本长度被配置成 0（head+tail 都为 0）：判定输入不可用，不得发空样本请求。
		return nil, fmt.Errorf("%w: article %d (sample length configured to 0)", ErrAIDetectionNoInput, article.ID)
	}

	req := s.buildRequest(article, sample, inputSource)
	router := s.resolveRouter()

	result, err := router.Chat(ctx, req)
	if err != nil {
		// 路由缺失 / 无可用绑定时 Router.Chat 早退、不写审计行——补一行，
		// 保证「能力未绑定模型 → 判定失败并记入 AI 调用审计」不是静默跳过。
		code := ""
		switch {
		case errors.Is(err, airouter.ErrRouteNotFound):
			code = "route_not_found"
		case errors.Is(err, airouter.ErrNoProviders):
			code = "no_providers"
		}
		if code != "" {
			router.LogLocalFailure(ctx, req, airouter.LocalFailure{
				Code:    code,
				Message: err.Error(),
			})
		}
		return nil, fmt.Errorf("ai detection: chat failed for article %d: %w", article.ID, err)
	}

	score, ok := ParseAIDetectionScore(result.Content)
	if !ok {
		// provider 侧成功但结论不可用：Router 已写过一行成功行，这里补一行失败行，
		// 否则审计里看不到「拿到响应却判不出分数」。
		router.LogLocalFailure(ctx, req, airouter.LocalFailure{
			Code:            "parse_error",
			Message:         fmt.Sprintf("ai detection: cannot parse 0-1 confidence score for article %d", article.ID),
			ResponseSnippet: result.Content,
			RouteName:       result.RouteName,
			ProviderName:    result.ProviderName,
			ModelName:       result.ModelName,
		})
		return nil, fmt.Errorf("ai detection: cannot parse confidence score for article %d", article.ID)
	}

	status := AIDetectionStatusNotAI
	if score >= cfg.Threshold {
		status = AIDetectionStatusSuspect
	}

	return &AIDetectionResult{
		Score:       score,
		Status:      status,
		Source:      AIDetectionSourceLLM,
		InputSource: inputSource,
		Threshold:   cfg.Threshold,
		DetectedAt:  time.Now().In(models.ShanghaiTZ),
	}, nil
}

// DetectAndPersist 判定 + 落库的一站式入口（判定 worker 用它即可）。
//
// 判定失败（含人工结论跳过）时不会改动文章行——判定状态保持原值（「未判定」）。
func (s *AIDetectionService) DetectAndPersist(ctx context.Context, article *models.Article, force bool) (*AIDetectionResult, error) {
	result, err := s.DetectArticle(ctx, article, force)
	if err != nil {
		return nil, err
	}
	if err := PersistAIDetectionResult(article, result); err != nil {
		return nil, err
	}
	return result, nil
}

// ApplyAIDetectionResult 把判定结论写入文章实体（不落库）。
func ApplyAIDetectionResult(article *models.Article, result *AIDetectionResult) {
	if article == nil || result == nil {
		return
	}
	article.AIDetectionStatus = result.Status
	article.AIDetectionScore = result.Score
	article.AIDetectionSource = result.Source
	detectedAt := result.DetectedAt
	if detectedAt.IsZero() {
		detectedAt = time.Now().In(models.ShanghaiTZ)
	}
	article.AIDetectionAt = &detectedAt
}

// PersistAIDetectionResult 把判定结论写入文章实体并落库。
//
// **只写判定四列**，不走整行 Save：判定与摘要补全 worker 都是「LLM 前加载文章 → LLM 后
// 写回」，整行 Save 会按加载时的快照回写全部列，后写者把对方在这段时间内写入的列
// （ai_content_summary / summary_status / read 等）静默回滚，且作业已 completed 不会重跑。
// 窄写四列让两个写者互不覆盖（review R-2）。
func PersistAIDetectionResult(article *models.Article, result *AIDetectionResult) error {
	if article == nil || result == nil {
		return errors.New("ai detection: article or result is nil")
	}
	if article.ID == 0 {
		return errors.New("ai detection: article id is zero")
	}
	if repository.Repo == nil || repository.Repo.DB() == nil {
		return errors.New("ai detection: repository is not initialized")
	}
	ApplyAIDetectionResult(article, result)
	return repository.Repo.DB().Model(&models.Article{}).
		Where("id = ?", article.ID).
		Updates(map[string]any{
			"ai_detection_status": article.AIDetectionStatus,
			"ai_detection_score":  article.AIDetectionScore,
			"ai_detection_source": article.AIDetectionSource,
			"ai_detection_at":     article.AIDetectionAt,
		}).Error
}

// buildRequest 构造判定用的 airouter 请求：Operation 非空（红线：空则报错不发请求）、
// Capability=ai_detection（决定路由与并发配额，与打标互不挤占）。
func (s *AIDetectionService) buildRequest(article *models.Article, sample, inputSource string) airouter.ChatRequest {
	temperature := 0.0
	return airouter.ChatRequest{
		Operation:  AIDetectionOperation,
		Capability: airouter.CapabilityAIDetection,
		Messages: []airouter.Message{
			{Role: "system", Content: aiDetectionSystemPrompt},
			{Role: "user", Content: buildAIDetectionUserPrompt(article.Title, sample)},
		},
		Temperature: &temperature,
		Metadata: map[string]any{
			"article_id":   article.ID,
			"feed_id":      article.FeedID,
			"input_source": inputSource,
			"sample_runes": len([]rune(sample)),
		},
	}
}

// aiDetectionSystemPrompt 约束模型只输出一个 0-1 的纯数字（design D2：小模型只打分，
// 不写解释；输出形态不稳会让解析成为主要失败源）。
const aiDetectionSystemPrompt = `你是「文章是否由 AI 生成」的判定器。给定一段文章正文样本，判断它是否像 AI 生成或 AI 洗稿的内容，并输出一个 0 到 1 之间的置信分。

输出规则（必须严格遵守）：
1. 只输出一行：一个 0 到 1 之间的纯数字，例如 0.86。
2. 不要输出解释、理由、标签、序号、标点、markdown 或任何多余文字。
3. 0 表示几乎确定是人工撰写；1 表示几乎确定由 AI 生成；中间值表示不确定程度。`

// buildAIDetectionUserPrompt 拼判定请求正文。标题供模型参考文风，正文样本是首尾拼接。
func buildAIDetectionUserPrompt(title, sample string) string {
	var b strings.Builder
	b.WriteString("请判定下面这篇文章的正文样本是否为 AI 生成。\n\n")
	if strings.TrimSpace(title) != "" {
		b.WriteString("标题：")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	b.WriteString("正文样本（首段与尾段拼接，中间内容已省略）：\n---\n")
	b.WriteString(sample)
	b.WriteString("\n---\n\n只输出一个 0 到 1 之间的数字。")
	return b.String()
}
